package workos

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
)

func TestNew_MissingClientID(t *testing.T) {
	_, err := New(context.Background(), Config{
		ClientID:    "",
		APIKey:      "test-api-key",
		RedirectURI: "http://localhost/callback",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client id and api key are required")
}

func TestNew_MissingAPIKey(t *testing.T) {
	_, err := New(context.Background(), Config{
		ClientID:    "client-id",
		APIKey:      "",
		RedirectURI: "http://localhost/callback",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client id and api key are required")
}

func TestProviderWorkOSConstant(t *testing.T) {
	// Ensure the constant is stable — it is persisted in the DB
	assert.Equal(t, idp.ProviderName("workos"), idp.ProviderWorkOS)
}

// TestNew_WithRealWorkOS is skipped unless credentials are available in env.
// It is here as documentation of expected behaviour.
func TestNew_WithRealWorkOS_Skipped(t *testing.T) {
	t.Skip("Integration test requires real WorkOS credentials — skip in unit tests")
}

// stubBaseProvider is a minimal idp.IdentityProvider used to exercise the
// AuthKit provider wrapper without requiring network access.
type stubBaseProvider struct{}

func (stubBaseProvider) Name() idp.ProviderName { return idp.ProviderWorkOS }
func (stubBaseProvider) AuthorizeURL(state, redirectURI, _ string) string {
	return "https://api.workos.com/user_management/authorize?state=" + state
}
func (stubBaseProvider) ExchangeCode(_ context.Context, _, _ string) (*idp.Tokens, *idp.Claims, error) {
	return nil, nil, nil
}
func (stubBaseProvider) Refresh(_ context.Context, _ string) (*idp.Tokens, error) {
	return nil, nil
}

func TestAuthorizeURL_AppendsAuthKitProvider(t *testing.T) {
	p := &provider{IdentityProvider: stubBaseProvider{}}
	got := p.AuthorizeURL("test-state", "http://cb", "")
	assert.Contains(t, got, "provider=GoogleOAuth",
		"single-click Google flow requires provider=GoogleOAuth in the authorize URL")
	assert.Contains(t, got, "state=test-state")
}

func TestAuthorizeURL_EmptyBaseReturnsEmpty(t *testing.T) {
	p := &provider{IdentityProvider: emptyURLProvider{}}
	assert.Equal(t, "", p.AuthorizeURL("s", "", ""))
}

func TestAuthorizeURL_AppendsRequestedProvider(t *testing.T) {
	p := &provider{IdentityProvider: stubBaseProvider{}}
	got := p.AuthorizeURL("test-state", "http://cb", "GitHubOAuth")
	assert.Contains(t, got, "provider=GitHubOAuth",
		"explicit provider hint must appear in the authorize URL")
	assert.NotContains(t, got, "provider=GoogleOAuth",
		"GitHubOAuth hint must replace the default GoogleOAuth")
	assert.Contains(t, got, "state=test-state")
}

func TestAuthorizeURL_DefaultsToGoogleOAuth(t *testing.T) {
	p := &provider{IdentityProvider: stubBaseProvider{}}
	got := p.AuthorizeURL("test-state", "http://cb", "")
	assert.Contains(t, got, "provider=GoogleOAuth",
		"empty provider hint must fall back to GoogleOAuth")
	assert.Contains(t, got, "state=test-state")
}

type emptyURLProvider struct{ stubBaseProvider }

func (emptyURLProvider) AuthorizeURL(_, _, _ string) string { return "" }

// newTestProvider builds a provider whose ExchangeCode is wired to the given
// httptest server's URL by overriding the package-level tokenEndpoint.
func newTestProvider(client *http.Client) *provider {
	return &provider{
		IdentityProvider: stubBaseProvider{},
		clientID:         "client_test",
		clientSecret:     "sk_test",
		httpClient:       client,
	}
}

func TestExchangeCode_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "client_test", r.PostForm.Get("client_id"))
		assert.Equal(t, "sk_test", r.PostForm.Get("client_secret"))
		assert.Equal(t, "authorization_code", r.PostForm.Get("grant_type"))
		assert.Equal(t, "auth-code-123", r.PostForm.Get("code"))
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		_, werr := w.Write([]byte(`{
			"access_token": "wat_abc",
			"refresh_token": "wrt_xyz",
			"user": {
				"id": "user_01ABC",
				"email": "alice@example.com",
				"email_verified": true,
				"first_name": "Alice",
				"last_name": "Smith",
				"profile_picture_url": "https://workos.com/avatar.png"
			}
		}`))
		require.NoError(t, werr)
	}))
	defer srv.Close()

	// Override the package-level token endpoint via test seam.
	originalEndpoint := authenticateURL
	authenticateURL = srv.URL
	defer func() { authenticateURL = originalEndpoint }()

	p := newTestProvider(srv.Client())
	tokens, claims, err := p.ExchangeCode(context.Background(), "auth-code-123", "http://cb")
	require.NoError(t, err)
	require.NotNil(t, tokens)
	require.NotNil(t, claims)

	assert.Equal(t, "wat_abc", tokens.AccessToken)
	assert.Equal(t, "wrt_xyz", tokens.RefreshToken)
	assert.True(t, tokens.ExpiresAt.After(time.Now()))

	assert.Equal(t, "user_01ABC", claims.Subject)
	assert.Equal(t, "alice@example.com", claims.Email)
	assert.True(t, claims.EmailVerified)
	assert.Equal(t, "Alice Smith", claims.Name)
	assert.Equal(t, "https://workos.com/avatar.png", claims.Picture)
}

func TestExchangeCode_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, werr := w.Write([]byte(`{"error":"invalid_grant"}`))
		require.NoError(t, werr)
	}))
	defer srv.Close()

	originalEndpoint := authenticateURL
	authenticateURL = srv.URL
	defer func() { authenticateURL = originalEndpoint }()

	p := newTestProvider(srv.Client())
	_, _, err := p.ExchangeCode(context.Background(), "bad-code", "http://cb")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}

func TestExchangeCode_MissingUserID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, werr := w.Write([]byte(`{"access_token":"wat","user":{"email":"a@b.c"}}`))
		require.NoError(t, werr)
	}))
	defer srv.Close()

	originalEndpoint := authenticateURL
	authenticateURL = srv.URL
	defer func() { authenticateURL = originalEndpoint }()

	p := newTestProvider(srv.Client())
	_, _, err := p.ExchangeCode(context.Background(), "code", "http://cb")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user.id")
}

func TestFullName(t *testing.T) {
	assert.Equal(t, "Alice Smith", fullName("Alice", "Smith"))
	assert.Equal(t, "Alice", fullName("Alice", ""))
	assert.Equal(t, "Smith", fullName("", "Smith"))
	assert.Equal(t, "", fullName("", ""))
	assert.Equal(t, "Alice Smith", fullName("  Alice  ", "  Smith  "))
}
