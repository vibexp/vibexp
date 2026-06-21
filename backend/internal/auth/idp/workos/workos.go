// Package workos constructs an idp.IdentityProvider that authenticates
// against WorkOS AuthKit. It bypasses the generic OIDC userinfo flow because
// WorkOS does not expose a userinfo_endpoint — instead, the token-exchange
// response carries a `user` object with the profile claims.
package workos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/auth/idp/oidc"
)

// issuerURLBase is the WorkOS AuthKit OIDC issuer base.
// The full issuer URL is issuerURLBase + "/" + clientID, e.g.
// https://api.workos.com/user_management/client_01JXYZ
const issuerURLBase = "https://api.workos.com/user_management"

// authenticateURL is the WorkOS authenticate endpoint used to exchange the
// authorization code for tokens. WorkOS returns the user profile inline
// in this response, not via a separate userinfo endpoint.
// It is a var (not a const) so unit tests can point at httptest servers.
var authenticateURL = "https://api.workos.com/user_management/authenticate"

// googleProvider is the value passed as ?provider= in the authorize URL
// so WorkOS routes directly to Google OAuth, bypassing the AuthKit hosted
// sign-in chooser. Users see a single "Continue with Google" click that
// goes straight to Google. Without a provider param WorkOS returns
// "invalid-connection-selector"; with "authkit" it shows its own chooser.
// To support GitHub/Microsoft/SSO buttons later, surface separate buttons
// that hit /auth/login?provider=GitHubOAuth (etc.) and pass the value
// through here.
const googleProvider = "GoogleOAuth"

// httpTimeout is the timeout applied to outbound calls to WorkOS APIs.
const httpTimeout = 15 * time.Second

// Config holds the configuration used to build a WorkOS-backed IdentityProvider.
type Config struct {
	// ClientID is the WorkOS client ID (WORKOS_CLIENT_ID).
	ClientID string
	// APIKey is the WorkOS API key, used as the OIDC client secret (WORKOS_API_KEY).
	APIKey string
	// RedirectURI is the callback URI registered with WorkOS (WORKOS_REDIRECT_URI).
	RedirectURI string
}

// provider implements idp.IdentityProvider against WorkOS AuthKit.
// AuthorizeURL delegates to the embedded OIDC client and appends
// ?provider=GoogleOAuth so WorkOS routes the user straight to Google
// (no AuthKit chooser page). ExchangeCode and Refresh both call WorkOS's
// /user_management/authenticate endpoint directly and read claims from
// the inline `user` payload — WorkOS does not expose an OIDC userinfo
// endpoint, so the generic OIDC client cannot complete this flow.
type provider struct {
	idp.IdentityProvider
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

// AuthorizeURL appends ?provider=<providerHint> so WorkOS routes the user
// directly to the requested OAuth connection. If providerHint is empty,
// it falls back to googleProvider ("GoogleOAuth") to preserve the existing
// single-click Google sign-in behaviour.
func (p *provider) AuthorizeURL(state, redirectURI, providerHint string) string {
	base := p.IdentityProvider.AuthorizeURL(state, redirectURI, "")
	if base == "" {
		return ""
	}
	effective := providerHint
	if effective == "" {
		effective = googleProvider
	}
	sep := "&"
	if !strings.Contains(base, "?") {
		sep = "?"
	}
	return base + sep + "provider=" + url.QueryEscape(effective)
}

// workOSUser is the inline user payload returned by WorkOS's authenticate endpoint.
type workOSUser struct {
	ID                string `json:"id"`
	Email             string `json:"email"`
	EmailVerified     bool   `json:"email_verified"`
	FirstName         string `json:"first_name"`
	LastName          string `json:"last_name"`
	ProfilePictureURL string `json:"profile_picture_url"`
}

// workOSTokenResponse is the body returned by POST /user_management/authenticate.
// ExpiresIn is the access-token lifetime in seconds; WorkOS does not always
// return it, in which case we fall back to defaultAccessTokenTTL.
type workOSTokenResponse struct {
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token"`
	ExpiresIn    int64      `json:"expires_in,omitempty"`
	User         workOSUser `json:"user"`
}

// defaultAccessTokenTTL is the fallback access-token lifetime when WorkOS
// does not include expires_in in its authenticate response. WorkOS access
// tokens currently default to 1 hour. Documented at:
// https://workos.com/docs/reference/user-management/authentication/refresh
const defaultAccessTokenTTL = time.Hour

// computeExpiresAt returns the access-token expiry timestamp, preferring
// the WorkOS-supplied expires_in value when present.
func computeExpiresAt(expiresIn int64) time.Time {
	if expiresIn > 0 {
		return time.Now().Add(time.Duration(expiresIn) * time.Second)
	}
	return time.Now().Add(defaultAccessTokenTTL)
}

// ExchangeCode posts the authorization code to WorkOS's authenticate endpoint
// and returns the tokens + claims extracted from the inline `user` object.
func (p *provider) ExchangeCode(
	ctx context.Context, code, _ string,
) (*idp.Tokens, *idp.Claims, error) {
	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)

	tr, err := p.postAuthenticate(ctx, form)
	if err != nil {
		return nil, nil, err
	}
	if tr.User.ID == "" {
		return nil, nil, fmt.Errorf("workos: token response missing user.id")
	}

	tokens := &idp.Tokens{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    computeExpiresAt(tr.ExpiresIn),
	}
	claims := &idp.Claims{
		Subject:       tr.User.ID,
		Email:         tr.User.Email,
		EmailVerified: tr.User.EmailVerified,
		Name:          fullName(tr.User.FirstName, tr.User.LastName),
		Picture:       tr.User.ProfilePictureURL,
	}
	return tokens, claims, nil
}

// postAuthenticate POSTs the form to WorkOS's authenticate endpoint and
// returns the decoded response. Shared by ExchangeCode and Refresh.
func (p *provider) postAuthenticate(
	ctx context.Context, form url.Values,
) (*workOSTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authenticateURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("workos: build authenticate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("workos: post authenticate request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: workos close response body: %v\n", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("workos: read authenticate response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("workos: authenticate endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tr workOSTokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("workos: decode authenticate response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("workos: authenticate response missing access_token")
	}
	return &tr, nil
}

// Refresh exchanges a refresh token for a new access token using the
// WorkOS authenticate endpoint. The user payload returned is ignored —
// callers only need the new tokens for cookie rotation.
func (p *provider) Refresh(ctx context.Context, refreshToken string) (*idp.Tokens, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("workos: refresh token is required")
	}

	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	tr, err := p.postAuthenticate(ctx, form)
	if err != nil {
		return nil, err
	}
	return &idp.Tokens{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    computeExpiresAt(tr.ExpiresIn),
	}, nil
}

func fullName(first, last string) string {
	first = strings.TrimSpace(first)
	last = strings.TrimSpace(last)
	switch {
	case first != "" && last != "":
		return first + " " + last
	case first != "":
		return first
	default:
		return last
	}
}

// New builds an IdentityProvider that authenticates against WorkOS AuthKit.
// It performs OIDC discovery at construction time, so it requires network access.
func New(ctx context.Context, cfg Config) (idp.IdentityProvider, error) {
	if cfg.ClientID == "" || cfg.APIKey == "" {
		return nil, fmt.Errorf("idp/workos: client id and api key are required")
	}

	issuerURL := fmt.Sprintf("%s/%s", issuerURLBase, cfg.ClientID)

	base, err := oidc.New(ctx, oidc.Config{
		Name:         idp.ProviderWorkOS,
		IssuerURL:    issuerURL,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.APIKey,
		RedirectURL:  cfg.RedirectURI,
		Scopes:       []string{"openid", "email", "profile", "offline_access"},
	})
	if err != nil {
		return nil, err
	}

	return &provider{
		IdentityProvider: base,
		clientID:         cfg.ClientID,
		clientSecret:     cfg.APIKey,
		httpClient:       &http.Client{Timeout: httpTimeout},
	}, nil
}
