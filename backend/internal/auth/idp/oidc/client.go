// Package oidc provides a generic, provider-agnostic OpenID Connect client
// that satisfies the idp.IdentityProvider interface.
package oidc

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/vibexp/vibexp/internal/auth/idp"
)

// Config holds the configuration required to construct a generic OIDC
// client. IssuerURL is used for OIDC discovery and must point at an
// OIDC-compliant issuer (e.g. https://accounts.google.com).
type Config struct {
	Name         idp.ProviderName
	IssuerURL    string
	ClientID     string
	ClientSecret string
	Scopes       []string
	RedirectURL  string
	// AccessTypeOffline adds Google's access_type=offline parameter to the
	// authorization request. It is Google-specific (other issuers may reject
	// it as unknown), so only the google package sets it.
	AccessTypeOffline bool
}

// Validate ensures the configuration is internally consistent before
// attempting issuer discovery.
func (c Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("oidc: provider name is required")
	}
	if c.IssuerURL == "" {
		return fmt.Errorf("oidc: issuer URL is required")
	}
	if c.ClientID == "" {
		return fmt.Errorf("oidc: client ID is required")
	}
	if c.ClientSecret == "" {
		return fmt.Errorf("oidc: client secret is required")
	}
	return nil
}

// Client is a generic OIDC identity provider built on top of
// github.com/coreos/go-oidc/v3 and golang.org/x/oauth2.
type Client struct {
	name         idp.ProviderName
	provider     *oidc.Provider
	verifier     *oidc.IDTokenVerifier
	oauth2Config *oauth2.Config
	// accessTypeOffline mirrors Config.AccessTypeOffline.
	accessTypeOffline bool
	// revocation is the issuer's RFC 7009 revocation endpoint, read from
	// discovery; its zero value means the issuer advertises none.
	revocation revocationEndpoint
}

// New builds a new OIDC Client. It performs OIDC discovery against
// IssuerURL, so it requires network access at construction time.
func New(ctx context.Context, cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc: discover issuer %q: %w", cfg.IssuerURL, err)
	}

	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "email", "profile"}
		// Ask for a refresh token where the issuer says it grants one, so the
		// session can be re-checked against the issuer (#1647). Google is
		// asked through access_type=offline instead and rejects offline_access.
		if !cfg.AccessTypeOffline && supportsOfflineAccess(provider) {
			scopes = append(scopes, oidc.ScopeOfflineAccess)
		}
	}

	return &Client{
		name:     cfg.Name,
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		oauth2Config: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  cfg.RedirectURL,
			Scopes:       scopes,
		},
		accessTypeOffline: cfg.AccessTypeOffline,
		revocation:        discoverRevocation(provider),
	}, nil
}

// supportsOfflineAccess reports whether the issuer's discovery document lists
// the offline_access scope in scopes_supported. An unreadable or absent list
// reads as no.
func supportsOfflineAccess(provider *oidc.Provider) bool {
	var meta struct {
		ScopesSupported []string `json:"scopes_supported"`
	}
	if err := provider.Claims(&meta); err != nil {
		return false
	}
	return slices.Contains(meta.ScopesSupported, oidc.ScopeOfflineAccess)
}

// revocationEndpoint is where and how the client revokes a token (RFC 7009).
type revocationEndpoint struct {
	// url is the revocation_endpoint of the discovery document; empty when the
	// issuer advertises none.
	url string
	// post sends the client credentials in the form body (client_secret_post)
	// instead of HTTP Basic (client_secret_basic).
	post bool
}

// Client authentication methods of revocation_endpoint_auth_methods_supported.
const (
	authMethodBasic = "client_secret_basic"
	authMethodPost  = "client_secret_post"
)

// discoverRevocation reads the revocation endpoint from the issuer's discovery
// document. The client authenticates with client_secret_basic, the default of
// RFC 8414, unless the issuer lists client_secret_post and not basic among
// revocation_endpoint_auth_methods_supported. An unreadable document reads as
// no endpoint.
func discoverRevocation(provider *oidc.Provider) revocationEndpoint {
	var meta struct {
		Endpoint    string   `json:"revocation_endpoint"`
		AuthMethods []string `json:"revocation_endpoint_auth_methods_supported"`
	}
	if err := provider.Claims(&meta); err != nil || meta.Endpoint == "" {
		return revocationEndpoint{}
	}
	post := slices.Contains(meta.AuthMethods, authMethodPost) && !slices.Contains(meta.AuthMethods, authMethodBasic)
	return revocationEndpoint{url: meta.Endpoint, post: post}
}

// Name returns the canonical provider identifier.
func (c *Client) Name() idp.ProviderName { return c.name }

// AuthorizeURL builds the authorization-code-flow URL. If redirectURI is
// non-empty it overrides the default RedirectURL configured at construction.
// The provider parameter is accepted for interface compliance but ignored by
// the generic OIDC client — provider routing is handled by wrapper
// implementations (e.g. the Google provider). A non-empty flow.CodeVerifier
// adds its PKCE S256 challenge and a non-empty flow.Nonce adds the OIDC nonce;
// access_type=offline is sent only when the client was built with
// Config.AccessTypeOffline.
func (c *Client) AuthorizeURL(state, redirectURI, _ string, flow idp.FlowParams) string {
	cfg := *c.oauth2Config
	if redirectURI != "" {
		cfg.RedirectURL = redirectURI
	}
	var opts []oauth2.AuthCodeOption
	if flow.CodeVerifier != "" {
		opts = append(opts, oauth2.S256ChallengeOption(flow.CodeVerifier))
	}
	if flow.Nonce != "" {
		opts = append(opts, oidc.Nonce(flow.Nonce))
	}
	if c.accessTypeOffline {
		opts = append(opts, oauth2.AccessTypeOffline)
	}
	return cfg.AuthCodeURL(state, opts...)
}

// ExchangeCode exchanges an authorization code for tokens and the verified
// user claims. The ID token signature is verified against the provider's
// JWKS and, when flow.Nonce is set, its nonce must equal it (else
// idp.ErrNonceMismatch); if no ID token is returned it falls back to the
// userinfo endpoint, where there is no nonce to check. A non-empty
// flow.CodeVerifier is sent as the PKCE code_verifier.
func (c *Client) ExchangeCode(
	ctx context.Context, code, redirectURI string, flow idp.FlowParams,
) (*idp.Tokens, *idp.Claims, error) {
	cfg := *c.oauth2Config
	if redirectURI != "" {
		cfg.RedirectURL = redirectURI
	}

	var opts []oauth2.AuthCodeOption
	if flow.CodeVerifier != "" {
		opts = append(opts, oauth2.VerifierOption(flow.CodeVerifier))
	}
	token, err := cfg.Exchange(ctx, code, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("oidc: exchange code: %w", err)
	}

	tokens := &idp.Tokens{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
	}

	claims, err := c.claimsFromToken(ctx, &cfg, token, flow.Nonce)
	if err != nil {
		return nil, nil, err
	}
	if rawIDToken, ok := token.Extra("id_token").(string); ok {
		tokens.IDToken = rawIDToken
	}

	return tokens, claims, nil
}

// Refresh requests a new access token using the provided refresh token and
// returns the user's current claims. An ID token in the refresh response is
// verified like the sign-in one (signature, issuer, audience, expiry) but
// without a nonce, which OpenID Connect Core §12.2 leaves out of a refresh; a
// response without one falls back to userinfo.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*idp.Tokens, *idp.Claims, error) {
	if refreshToken == "" {
		return nil, nil, fmt.Errorf("oidc: refresh token is required")
	}

	src := c.oauth2Config.TokenSource(ctx, &oauth2.Token{
		RefreshToken: refreshToken,
		Expiry:       time.Now().Add(-time.Minute),
	})
	token, err := src.Token()
	if err != nil {
		return nil, nil, fmt.Errorf("oidc: refresh token: %w", err)
	}

	tokens := &idp.Tokens{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
	}
	claims, err := c.claimsFromToken(ctx, c.oauth2Config, token, "")
	if err != nil {
		return nil, nil, err
	}
	if rawIDToken, ok := token.Extra("id_token").(string); ok {
		tokens.IDToken = rawIDToken
	}
	return tokens, claims, nil
}

// RevokeRefreshToken revokes refreshToken at the issuer's revocation endpoint
// (RFC 7009) with token_type_hint=refresh_token, authenticating as the client.
// It returns idp.ErrRevocationUnsupported when discovery advertised no
// endpoint, and an error for any answer but 200 OK. ctx bounds the call; an
// *http.Client in ctx under oauth2.HTTPClient is used, as for the token
// endpoint.
func (c *Client) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	if c.revocation.url == "" {
		return idp.ErrRevocationUnsupported
	}
	if refreshToken == "" {
		return fmt.Errorf("oidc: refresh token is required")
	}
	form := url.Values{"token": {refreshToken}, "token_type_hint": {"refresh_token"}}
	if c.revocation.post {
		form.Set("client_id", c.oauth2Config.ClientID)
		form.Set("client_secret", c.oauth2Config.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.revocation.url, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("oidc: build revocation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if !c.revocation.post {
		// RFC 6749 §2.3.1: the credentials are form-encoded before Basic.
		req.SetBasicAuth(url.QueryEscape(c.oauth2Config.ClientID), url.QueryEscape(c.oauth2Config.ClientSecret))
	}
	resp, err := httpClient(ctx).Do(req)
	if err != nil {
		return fmt.Errorf("oidc: revoke token: %w: %w", idp.ErrProviderUnavailable, err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxRevocationErrorBody))
	closeErr := resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("oidc: revoke token: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err = errors.Join(readErr, closeErr); err != nil {
		return fmt.Errorf("oidc: revoke token: read response: %w", err)
	}
	return nil
}

// maxRevocationErrorBody caps how much of a revocation response is read, and
// quoted in the error of a failed one.
const maxRevocationErrorBody = 512

// httpClient returns the *http.Client carried in ctx under oauth2.HTTPClient,
// or http.DefaultClient.
func httpClient(ctx context.Context) *http.Client {
	if hc, ok := ctx.Value(oauth2.HTTPClient).(*http.Client); ok && hc != nil {
		return hc
	}
	return http.DefaultClient
}

// claimsFromToken reads the claims from the verified ID token, or from userinfo
// when the token response carried none.
func (c *Client) claimsFromToken(
	ctx context.Context, cfg *oauth2.Config, token *oauth2.Token, nonce string,
) (*idp.Claims, error) {
	if rawIDToken, ok := token.Extra("id_token").(string); ok && rawIDToken != "" {
		return c.idTokenClaims(ctx, rawIDToken, nonce)
	}
	return c.userInfoClaims(ctx, cfg, token)
}

// idTokenClaims verifies rawIDToken, checks its nonce and returns its claims.
func (c *Client) idTokenClaims(ctx context.Context, rawIDToken, nonce string) (*idp.Claims, error) {
	idToken, err := c.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		// go-oidc reports a failure to download the issuer's keys as
		// "fetching keys ...": the issuer is unavailable, the token unjudged.
		if strings.Contains(err.Error(), "fetching keys") {
			return nil, fmt.Errorf("oidc: verify id token: %w: %w", idp.ErrProviderUnavailable, err)
		}
		return nil, fmt.Errorf("oidc: verify id token: %w", err)
	}
	if err = checkNonce(idToken.Nonce, nonce); err != nil {
		return nil, err
	}
	var raw struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err = idToken.Claims(&raw); err != nil {
		return nil, fmt.Errorf("oidc: decode id token claims: %w", err)
	}
	if raw.Sub == "" {
		return nil, fmt.Errorf("oidc: id token missing subject")
	}
	var all map[string]any
	if err = idToken.Claims(&all); err != nil {
		return nil, fmt.Errorf("oidc: decode id token claims: %w", err)
	}
	return &idp.Claims{
		Subject:       raw.Sub,
		Email:         raw.Email,
		EmailVerified: raw.EmailVerified,
		Name:          raw.Name,
		Picture:       raw.Picture,
		Raw:           all,
	}, nil
}

// userInfoClaims returns the claims of the userinfo endpoint.
func (c *Client) userInfoClaims(
	ctx context.Context, cfg *oauth2.Config, token *oauth2.Token,
) (*idp.Claims, error) {
	userInfo, err := c.provider.UserInfo(ctx, cfg.TokenSource(ctx, token))
	if err != nil {
		if userInfoUnavailable(err) {
			return nil, fmt.Errorf("oidc: fetch userinfo: %w: %w", idp.ErrProviderUnavailable, err)
		}
		return nil, fmt.Errorf("oidc: fetch userinfo: %w", err)
	}
	var raw struct {
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err = userInfo.Claims(&raw); err != nil {
		return nil, fmt.Errorf("oidc: decode userinfo claims: %w", err)
	}
	if userInfo.Subject == "" {
		return nil, fmt.Errorf("oidc: userinfo missing subject")
	}
	var all map[string]any
	if err = userInfo.Claims(&all); err != nil {
		return nil, fmt.Errorf("oidc: decode userinfo claims: %w", err)
	}
	return &idp.Claims{
		Subject:       userInfo.Subject,
		Email:         userInfo.Email,
		EmailVerified: raw.EmailVerified,
		Name:          raw.Name,
		Picture:       raw.Picture,
		Raw:           all,
	}, nil
}

// userInfoStatus matches the "<status>: <body>" error go-oidc returns for a
// non-200 userinfo answer whose status says the issuer could not answer: a
// 5xx, 429 Too Many Requests or 408 Request Timeout.
var userInfoStatus = regexp.MustCompile(`^(5\d\d|429|408) `)

// userInfoUnavailable reports whether a userinfo failure is the issuer being
// unreachable or failing rather than an answer about the token (a 401 for a
// revoked access token is the latter).
func userInfoUnavailable(err error) bool {
	var urlErr *url.Error
	return errors.As(err, &urlErr) || userInfoStatus.MatchString(err.Error())
}

// checkNonce enforces OpenID Connect Core §3.1.3.7: when the authorization
// request carried a nonce, the ID token must echo exactly that value. An empty
// expected nonce (no nonce was sent) checks nothing.
func checkNonce(got, want string) error {
	if want == "" {
		return nil
	}
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return fmt.Errorf("oidc: %w", idp.ErrNonceMismatch)
	}
	return nil
}
