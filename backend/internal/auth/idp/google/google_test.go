package google

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
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
	authURL := p.AuthorizeURL("state-1", "", "")
	assert.Contains(t, authURL, srv.URL+"/o/oauth2/v2/auth")
	assert.Contains(t, authURL, "state=state-1")
}
