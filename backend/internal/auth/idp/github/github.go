// Package github constructs an idp.IdentityProvider that authenticates
// against GitHub. GitHub is OAuth2 but NOT OIDC — it exposes no
// userinfo/ID-token surface — so this adapter implements the interface
// directly: it exchanges the authorization code at GitHub's token endpoint,
// then calls the GitHub REST API (/user and /user/emails) to populate
// idp.Claims. GitHub specifics never leak past idp.Claims.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/auth/idp"
)

// GitHub OAuth2 / REST endpoints. They are vars (not consts) so unit tests
// can point them at httptest servers.
var (
	authorizeURL = "https://github.com/login/oauth/authorize"
	exchangeURL  = "https://github.com/login/oauth/access_token"
	apiBaseURL   = "https://api.github.com"
)

// defaultScopes request read access to the user's profile and email
// addresses. "user:email" is required to read the verified primary email
// via /user/emails when the public profile email is unset.
var defaultScopes = []string{"read:user", "user:email"}

// httpTimeout is the timeout applied to outbound calls to GitHub.
const httpTimeout = 15 * time.Second

// Config holds the credentials required to construct the GitHub provider.
type Config struct {
	// ClientID is the GitHub OAuth App client ID (GITHUB_CLIENT_ID).
	ClientID string
	// ClientSecret is the GitHub OAuth App client secret (GITHUB_CLIENT_SECRET).
	ClientSecret string
	// RedirectURL is the callback URL registered with the OAuth App
	// (GITHUB_REDIRECT_URI).
	RedirectURL string
	// Scopes overrides defaultScopes when non-empty.
	Scopes []string
}

// provider implements idp.IdentityProvider against GitHub's OAuth2 + REST API.
type provider struct {
	clientID     string
	clientSecret string
	redirectURL  string
	scopes       []string
	httpClient   *http.Client
}

// New builds a GitHub-backed IdentityProvider. Unlike OIDC providers it does
// no network discovery, so construction never fails on connectivity — it only
// validates that credentials are present.
func New(cfg Config) (idp.IdentityProvider, error) {
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("github: client ID is required")
	}
	if cfg.ClientSecret == "" {
		return nil, fmt.Errorf("github: client secret is required")
	}
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = defaultScopes
	}
	return &provider{
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		redirectURL:  cfg.RedirectURL,
		scopes:       scopes,
		httpClient:   &http.Client{Timeout: httpTimeout},
	}, nil
}

// Name returns the canonical provider identifier.
func (p *provider) Name() idp.ProviderName { return idp.ProviderGitHub }

// AuthorizeURL builds the GitHub authorization-code URL. If redirectURI is
// non-empty it overrides the configured RedirectURL. The provider hint is
// accepted for interface compliance but unused — GitHub has a single OAuth
// surface and does not route by connection.
func (p *provider) AuthorizeURL(state, redirectURI, _ string) string {
	redirect := p.redirectURL
	if redirectURI != "" {
		redirect = redirectURI
	}
	q := url.Values{}
	q.Set("client_id", p.clientID)
	q.Set("redirect_uri", redirect)
	q.Set("scope", strings.Join(p.scopes, " "))
	q.Set("state", state)
	return authorizeURL + "?" + q.Encode()
}

// githubTokenResponse is the JSON body returned by the token endpoint when
// the request sets Accept: application/json. GitHub returns HTTP 200 even on
// failure, signalling the error via the `error` field.
type githubTokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// githubUser is the subset of GET /user we consume.
type githubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Email     string `json:"email"`
}

// githubEmail is one entry from GET /user/emails.
type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// ExchangeCode exchanges the authorization code for an access token, then
// loads the user profile and verified primary email to build idp.Claims.
func (p *provider) ExchangeCode(
	ctx context.Context, code, redirectURI string,
) (*idp.Tokens, *idp.Claims, error) {
	redirect := p.redirectURL
	if redirectURI != "" {
		redirect = redirectURI
	}

	accessToken, err := p.exchange(ctx, code, redirect)
	if err != nil {
		return nil, nil, err
	}

	user, err := p.fetchUser(ctx, accessToken)
	if err != nil {
		return nil, nil, err
	}
	if user.ID == 0 {
		return nil, nil, fmt.Errorf("github: user response missing id")
	}

	email, emailVerified := user.Email, false
	primary, verified, err := p.fetchPrimaryEmail(ctx, accessToken)
	if err != nil {
		return nil, nil, err
	}
	if primary != "" {
		email, emailVerified = primary, verified
	}

	name := user.Name
	if name == "" {
		name = user.Login
	}

	// GitHub OAuth App tokens do not expire and are not refreshable. Leave
	// ExpiresAt as the zero value so the session layer treats the access
	// token as long-lived (the refresh path is never exercised — see Refresh).
	tokens := &idp.Tokens{AccessToken: accessToken}
	claims := &idp.Claims{
		Subject:       strconv.FormatInt(user.ID, 10),
		Email:         email,
		EmailVerified: emailVerified,
		Name:          name,
		Picture:       user.AvatarURL,
	}
	return tokens, claims, nil
}

// Refresh is unsupported: GitHub OAuth App access tokens are long-lived and
// GitHub does not issue refresh tokens for them.
func (p *provider) Refresh(_ context.Context, _ string) (*idp.Tokens, error) {
	return nil, fmt.Errorf("github: token refresh is not supported")
}

func (p *provider) exchange(ctx context.Context, code, redirect string) (string, error) {
	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("code", code)
	if redirect != "" {
		form.Set("redirect_uri", redirect)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, exchangeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("github: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github: post token request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: github close response body: %v\n", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("github: read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github: token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tr githubTokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("github: decode token response: %w", err)
	}
	if tr.Error != "" {
		return "", fmt.Errorf("github: token exchange failed: %s: %s", tr.Error, tr.ErrorDescription)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("github: token response missing access_token")
	}
	return tr.AccessToken, nil
}

func (p *provider) fetchUser(ctx context.Context, accessToken string) (*githubUser, error) {
	body, status, err := p.apiGet(ctx, "/user", accessToken)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("github: /user returned %d: %s", status, string(body))
	}
	var u githubUser
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("github: decode user response: %w", err)
	}
	return &u, nil
}

// fetchPrimaryEmail returns the user's primary verified email. It prefers the
// primary+verified address, falling back to the first verified one. When no
// verified email exists it returns ("", false, nil) so the caller can fall
// back to the public profile email.
func (p *provider) fetchPrimaryEmail(ctx context.Context, accessToken string) (string, bool, error) {
	body, status, err := p.apiGet(ctx, "/user/emails", accessToken)
	if err != nil {
		return "", false, err
	}
	if status != http.StatusOK {
		return "", false, fmt.Errorf("github: /user/emails returned %d: %s", status, string(body))
	}
	var emails []githubEmail
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", false, fmt.Errorf("github: decode emails response: %w", err)
	}
	var firstVerified string
	for _, e := range emails {
		if !e.Verified {
			continue
		}
		if e.Primary {
			return e.Email, true, nil
		}
		if firstVerified == "" {
			firstVerified = e.Email
		}
	}
	if firstVerified != "" {
		return firstVerified, true, nil
	}
	return "", false, nil
}

// apiGet performs an authenticated GET against the GitHub REST API and returns
// the response body and status code. The URL is built from the package-level
// apiBaseURL and a constant path, so the request target is never
// caller-controlled.
func (p *provider) apiGet(ctx context.Context, path, accessToken string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBaseURL+path, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("github: build %s request: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("github: get %s: %w", path, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: github close response body: %v\n", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("github: read response body: %w", err)
	}
	return body, resp.StatusCode, nil
}
