package google

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

func TestNew_Validation(t *testing.T) {
	_, err := New(context.Background(), Config{ClientSecret: "test-client-secret"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client ID is required")

	_, err = New(context.Background(), Config{ClientID: "test-client-id"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client secret is required")
}

// TestNew_WrapsOIDCWithGoogleName confirms the wrapper discovers the issuer
// and reports the stable idp.ProviderGoogle name. A minimal OIDC discovery
// document is enough for go-oidc's NewProvider to succeed.
func TestNew_WrapsOIDCWithGoogleName(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		doc := map[string]any{
			"issuer":                 srv.URL,
			"authorization_endpoint": srv.URL + "/o/oauth2/v2/auth",
			"token_endpoint":         srv.URL + "/token",
			"jwks_uri":               srv.URL + "/jwks",
			"userinfo_endpoint":      srv.URL + "/userinfo",
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(doc); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)

	orig := issuerURL
	issuerURL = srv.URL
	t.Cleanup(func() { issuerURL = orig })

	p, err := New(context.Background(), Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://app.example.com/api/v1/auth/callback",
	})
	require.NoError(t, err)
	assert.Equal(t, idp.ProviderGoogle, p.Name())

	// The wrapper must produce a usable Google authorization URL.
	authURL := p.AuthorizeURL("state-1", "", "", idp.FlowParams{})
	assert.Contains(t, authURL, srv.URL+"/o/oauth2/v2/auth")
	assert.Contains(t, authURL, "state=state-1")
}

// TestNew_NameAndIssuerOverride confirms a DB-managed Google provider reports
// its slug and that the issuer override is used for discovery.
func TestNew_NameAndIssuerOverride(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 srv.URL,
			"authorization_endpoint": srv.URL + "/auth",
			"token_endpoint":         srv.URL + "/token",
			"jwks_uri":               srv.URL + "/jwks",
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)

	p, err := New(context.Background(), Config{
		Name:         "google-workspace",
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		IssuerURL:    srv.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, idp.ProviderName("google-workspace"), p.Name())
	assert.Contains(t, p.AuthorizeURL("s", "", "", idp.FlowParams{}), srv.URL+"/auth")
}

// TestAuthorizeURL_OfflineAccessPKCEAndNonce pins that Google keeps
// access_type=offline (the generic OIDC client no longer sends it) alongside
// the PKCE challenge and the nonce.
func TestAuthorizeURL_OfflineAccessPKCEAndNonce(t *testing.T) {
	iss := fakeoidc.New(t, "test-client-id", "test-client-secret", "sub", "user@example.com")
	p, err := New(context.Background(), Config{
		ClientID: "test-client-id", ClientSecret: "test-client-secret", IssuerURL: iss.URL,
	})
	require.NoError(t, err)

	verifier := oauth2.GenerateVerifier()
	u, err := url.Parse(p.AuthorizeURL("s", "", "", idp.FlowParams{CodeVerifier: verifier, Nonce: "n-1"}))
	require.NoError(t, err)
	q := u.Query()
	assert.Equal(t, "offline", q.Get("access_type"))
	assert.Equal(t, oauth2.S256ChallengeFromVerifier(verifier), q.Get("code_challenge"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.Equal(t, "n-1", q.Get("nonce"))
}

// TestAuthorizeURL_NeverOfflineAccessScope pins that Google is asked for a
// refresh token through access_type=offline only: even an issuer whose
// discovery lists offline_access does not get the scope (#1647), which Google
// rejects.
func TestAuthorizeURL_NeverOfflineAccessScope(t *testing.T) {
	iss := fakeoidc.New(t, "test-client-id", "test-client-secret", "sub", "user@example.com")
	iss.SetScopesSupported([]string{"openid", "email", "profile", "offline_access"})
	p, err := New(context.Background(), Config{
		ClientID: "test-client-id", ClientSecret: "test-client-secret", IssuerURL: iss.URL,
	})
	require.NoError(t, err)

	u, err := url.Parse(p.AuthorizeURL("s", "", "", idp.FlowParams{}))
	require.NoError(t, err)
	assert.Equal(t, "offline", u.Query().Get("access_type"))
	assert.Equal(t, "openid email profile", u.Query().Get("scope"))
}
