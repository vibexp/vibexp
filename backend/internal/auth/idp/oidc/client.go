// Package oidc provides a generic, provider-agnostic OpenID Connect client
// that satisfies the idp.IdentityProvider interface.
package oidc

import (
	"context"
	"fmt"
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
	}, nil
}

// Name returns the canonical provider identifier.
func (c *Client) Name() idp.ProviderName { return c.name }

// AuthorizeURL builds the authorization-code-flow URL. If redirectURI is
// non-empty it overrides the default RedirectURL configured at construction.
// The provider parameter is accepted for interface compliance but ignored by
// the generic OIDC client — provider routing is handled by wrapper
// implementations (e.g. the WorkOS provider).
func (c *Client) AuthorizeURL(state, redirectURI, _ string) string {
	cfg := *c.oauth2Config
	if redirectURI != "" {
		cfg.RedirectURL = redirectURI
	}
	return cfg.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// ExchangeCode exchanges an authorization code for tokens and the verified
// user claims. The ID token signature is verified against the provider's
// JWKS; if no ID token is returned it falls back to the userinfo endpoint.
func (c *Client) ExchangeCode(
	ctx context.Context, code, redirectURI string,
) (*idp.Tokens, *idp.Claims, error) {
	cfg := *c.oauth2Config
	if redirectURI != "" {
		cfg.RedirectURL = redirectURI
	}

	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, nil, fmt.Errorf("oidc: exchange code: %w", err)
	}

	tokens := &idp.Tokens{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
	}

	claims, err := c.claimsFromToken(ctx, &cfg, token)
	if err != nil {
		return nil, nil, err
	}
	if rawIDToken, ok := token.Extra("id_token").(string); ok {
		tokens.IDToken = rawIDToken
	}

	return tokens, claims, nil
}

// Refresh requests a new access token using the provided refresh token.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*idp.Tokens, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("oidc: refresh token is required")
	}

	src := c.oauth2Config.TokenSource(ctx, &oauth2.Token{
		RefreshToken: refreshToken,
		Expiry:       time.Now().Add(-time.Minute),
	})
	token, err := src.Token()
	if err != nil {
		return nil, fmt.Errorf("oidc: refresh token: %w", err)
	}

	tokens := &idp.Tokens{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
	}
	if rawIDToken, ok := token.Extra("id_token").(string); ok {
		tokens.IDToken = rawIDToken
	}
	return tokens, nil
}

func (c *Client) claimsFromToken(
	ctx context.Context, cfg *oauth2.Config, token *oauth2.Token,
) (*idp.Claims, error) {
	if rawIDToken, ok := token.Extra("id_token").(string); ok && rawIDToken != "" {
		idToken, err := c.verifier.Verify(ctx, rawIDToken)
		if err != nil {
			return nil, fmt.Errorf("oidc: verify id token: %w", err)
		}
		var raw struct {
			Sub           string `json:"sub"`
			Email         string `json:"email"`
			EmailVerified bool   `json:"email_verified"`
			Name          string `json:"name"`
			Picture       string `json:"picture"`
		}
		if err := idToken.Claims(&raw); err != nil {
			return nil, fmt.Errorf("oidc: decode id token claims: %w", err)
		}
		if raw.Sub == "" {
			return nil, fmt.Errorf("oidc: id token missing subject")
		}
		return &idp.Claims{
			Subject:       raw.Sub,
			Email:         raw.Email,
			EmailVerified: raw.EmailVerified,
			Name:          raw.Name,
			Picture:       raw.Picture,
		}, nil
	}

	userInfo, err := c.provider.UserInfo(ctx, cfg.TokenSource(ctx, token))
	if err != nil {
		return nil, fmt.Errorf("oidc: fetch userinfo: %w", err)
	}
	var raw struct {
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := userInfo.Claims(&raw); err != nil {
		return nil, fmt.Errorf("oidc: decode userinfo claims: %w", err)
	}
	if userInfo.Subject == "" {
		return nil, fmt.Errorf("oidc: userinfo missing subject")
	}
	return &idp.Claims{
		Subject:       userInfo.Subject,
		Email:         userInfo.Email,
		EmailVerified: raw.EmailVerified,
		Name:          raw.Name,
		Picture:       raw.Picture,
	}, nil
}
