package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
)

// fakeIssuer hosts the minimum OIDC discovery + JWKS + token endpoints
// required to exercise the OIDC client in tests.
type fakeIssuer struct {
	server      *httptest.Server
	key         *rsa.PrivateKey
	keyID       string
	clients     map[string]string // clientID -> clientSecret
	subject     string
	omitIDToken bool // when true, /token returns no id_token (forces userinfo fallback)
}

const (
	testClientID     = "test-client"
	testClientSecret = "test-secret"
)

func newFakeIssuer(t *testing.T, subject string) *fakeIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	f := &fakeIssuer{
		key:     key,
		keyID:   "test-key",
		clients: map[string]string{testClientID: testClientSecret},
		subject: subject,
	}

	mux := http.NewServeMux()
	f.server = httptest.NewServer(mux)
	mux.HandleFunc("/.well-known/openid-configuration", f.handleDiscovery)
	mux.HandleFunc("/jwks", f.handleJWKS)
	mux.HandleFunc("/token", f.handleToken)
	mux.HandleFunc("/userinfo", f.handleUserInfo)

	return f
}

func (f *fakeIssuer) close() { f.server.Close() }

func (f *fakeIssuer) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	body := map[string]any{
		"issuer":                                f.server.URL,
		"authorization_endpoint":                f.server.URL + "/authorize",
		"token_endpoint":                        f.server.URL + "/token",
		"jwks_uri":                              f.server.URL + "/jwks",
		"userinfo_endpoint":                     f.server.URL + "/userinfo",
		"id_token_signing_alg_values_supported": []string{"RS256"},
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (f *fakeIssuer) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	n := base64.RawURLEncoding.EncodeToString(f.key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(f.key.E)).Bytes())
	body := map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA", "alg": "RS256", "use": "sig",
			"kid": f.keyID, "n": n, "e": e,
		}},
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (f *fakeIssuer) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cid := r.FormValue("client_id")
	csec := r.FormValue("client_secret")
	if cid == "" {
		cid, csec, _ = r.BasicAuth()
	}
	if expected, ok := f.clients[cid]; !ok || expected != csec {
		http.Error(w, "invalid_client", http.StatusUnauthorized)
		return
	}
	body := map[string]any{
		"access_token":  "fake-access",
		"refresh_token": "fake-refresh",
		"token_type":    "Bearer",
		"expires_in":    3600,
	}
	if !f.omitIDToken {
		idToken, err := f.signIDToken(cid, "test@example.com", "Test User", "https://example.com/p.png")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		body["id_token"] = idToken
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (f *fakeIssuer) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	body := map[string]any{
		"sub":            f.subject,
		"email":          "test@example.com",
		"email_verified": true,
		"name":           "Test User",
		"picture":        "https://example.com/p.png",
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (f *fakeIssuer) signIDToken(clientID, email, name, picture string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":            f.server.URL,
		"sub":            f.subject,
		"aud":            clientID,
		"iat":            now.Unix(),
		"exp":            now.Add(time.Hour).Unix(),
		"email":          email,
		"email_verified": true,
		"name":           name,
		"picture":        picture,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = f.keyID
	return tok.SignedString(f.key)
}

func TestConfig_Validate(t *testing.T) {
	t.Run("rejects empty fields", func(t *testing.T) {
		require.Error(t, Config{}.Validate())
	})
	t.Run("accepts a complete config", func(t *testing.T) {
		require.NoError(t, Config{
			Name:         idp.ProviderGoogle,
			IssuerURL:    "https://accounts.google.com",
			ClientID:     "id",
			ClientSecret: "secret",
		}.Validate())
	})
}

func TestNew_DiscoversIssuer(t *testing.T) {
	f := newFakeIssuer(t, "subject-1")
	defer f.close()

	c, err := New(context.Background(), Config{
		Name:         idp.ProviderGoogle,
		IssuerURL:    f.server.URL,
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURL:  "http://localhost/callback",
	})
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, idp.ProviderGoogle, c.Name())
}

func TestClient_AuthorizeURL_ContainsState(t *testing.T) {
	f := newFakeIssuer(t, "subject-1")
	defer f.close()

	c, err := New(context.Background(), Config{
		Name:         idp.ProviderGoogle,
		IssuerURL:    f.server.URL,
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURL:  "http://default/callback",
	})
	require.NoError(t, err)

	url := c.AuthorizeURL("xyz", "", "")
	assert.Contains(t, url, "state=xyz")
	assert.Contains(t, url, "client_id=test-client")
	assert.Contains(t, url, "http%3A%2F%2Fdefault%2Fcallback")

	// override redirect URI
	url = c.AuthorizeURL("abc", "http://override/callback", "")
	assert.Contains(t, url, "http%3A%2F%2Foverride%2Fcallback")
}

func TestClient_ExchangeCode_VerifiesIDToken(t *testing.T) {
	f := newFakeIssuer(t, "subject-42")
	defer f.close()

	c, err := New(context.Background(), Config{
		Name:         idp.ProviderGoogle,
		IssuerURL:    f.server.URL,
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURL:  "http://localhost/callback",
	})
	require.NoError(t, err)

	tokens, claims, err := c.ExchangeCode(context.Background(), "any-code", "")
	require.NoError(t, err)
	require.NotNil(t, tokens)
	require.NotNil(t, claims)

	assert.Equal(t, "fake-access", tokens.AccessToken)
	assert.Equal(t, "fake-refresh", tokens.RefreshToken)
	assert.NotEmpty(t, tokens.IDToken)

	assert.Equal(t, "subject-42", claims.Subject)
	assert.Equal(t, "test@example.com", claims.Email)
	assert.True(t, claims.EmailVerified)
	assert.Equal(t, "Test User", claims.Name)
	assert.Equal(t, "https://example.com/p.png", claims.Picture)
}

// TestClient_ExchangeCode_FallsBackToUserInfo covers the path where the
// token response omits id_token. The OIDC client must then call the
// userinfo endpoint to obtain the canonical claims.
func TestClient_ExchangeCode_FallsBackToUserInfo(t *testing.T) {
	f := newFakeIssuer(t, "subject-userinfo")
	f.omitIDToken = true
	defer f.close()

	c, err := New(context.Background(), Config{
		Name:         idp.ProviderGoogle,
		IssuerURL:    f.server.URL,
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURL:  "http://localhost/callback",
	})
	require.NoError(t, err)

	tokens, claims, err := c.ExchangeCode(context.Background(), "any-code", "")
	require.NoError(t, err)
	require.NotNil(t, tokens)
	require.NotNil(t, claims)

	assert.Empty(t, tokens.IDToken, "id_token should not be present on userinfo fallback")
	assert.Equal(t, "subject-userinfo", claims.Subject)
	assert.Equal(t, "test@example.com", claims.Email)
	assert.True(t, claims.EmailVerified)
	assert.Equal(t, "Test User", claims.Name)
}

func TestClient_ExchangeCode_RejectsBadCredentials(t *testing.T) {
	f := newFakeIssuer(t, "subject-1")
	defer f.close()

	c, err := New(context.Background(), Config{
		Name:         idp.ProviderGoogle,
		IssuerURL:    f.server.URL,
		ClientID:     "test-client",
		ClientSecret: "wrong-secret",
		RedirectURL:  "http://localhost/callback",
	})
	require.NoError(t, err)

	_, _, err = c.ExchangeCode(context.Background(), "any-code", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exchange code")
}

func TestClient_Refresh_RequiresToken(t *testing.T) {
	f := newFakeIssuer(t, "subject-1")
	defer f.close()

	c, err := New(context.Background(), Config{
		Name:         idp.ProviderGoogle,
		IssuerURL:    f.server.URL,
		ClientID:     "test-client",
		ClientSecret: "test-secret",
	})
	require.NoError(t, err)

	_, err = c.Refresh(context.Background(), "")
	require.Error(t, err)
}

func TestNew_FailsOnUnreachableIssuer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	_, err := New(context.Background(), Config{
		Name:         idp.ProviderGoogle,
		IssuerURL:    srv.URL,
		ClientID:     "x",
		ClientSecret: "y",
	})
	require.Error(t, err)
}

func TestNew_ProviderRequiresName(t *testing.T) {
	_, err := New(context.Background(), Config{
		IssuerURL:    "https://example.invalid",
		ClientID:     "x",
		ClientSecret: "y",
	})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "provider name"), "got: %v", err)
}
