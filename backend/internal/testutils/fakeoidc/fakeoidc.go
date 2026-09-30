// Package fakeoidc is an in-process OpenID Connect issuer for tests: discovery,
// JWKS and a token endpoint that mints a signed ID token for a fixed subject. It
// counts discovery requests, so a test can prove when an identity provider was
// (re)discovered, and can hang discovery to simulate an unreachable issuer.
package fakeoidc

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const keyID = "fakeoidc-key"

// Issuer is a running fake OIDC issuer.
type Issuer struct {
	// URL is the issuer URL (discovery lives under it).
	URL string

	server       *httptest.Server
	key          *rsa.PrivateKey
	clientID     string
	clientSecret string
	subject      string
	email        string

	discoveries atomic.Int64
	hang        atomic.Bool
	release     chan struct{}
}

// New starts an issuer that accepts clientID/clientSecret and signs ID tokens
// for subject and email. It is closed when the test ends.
func New(t testing.TB, clientID, clientSecret, subject, email string) *Issuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("fakeoidc: generate key: %v", err)
	}
	i := &Issuer{
		key: key, clientID: clientID, clientSecret: clientSecret,
		subject: subject, email: email, release: make(chan struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", i.handleDiscovery)
	mux.HandleFunc("/jwks", i.handleJWKS)
	mux.HandleFunc("/token", i.handleToken)
	i.server = httptest.NewServer(mux)
	i.URL = i.server.URL
	t.Cleanup(func() {
		close(i.release)
		i.server.Close()
	})
	return i
}

// Discoveries reports how many discovery requests the issuer has served (or
// started serving, when hanging).
func (i *Issuer) Discoveries() int64 { return i.discoveries.Load() }

// SetHang makes discovery block until the request is cancelled (true) or answer
// normally again (false).
func (i *Issuer) SetHang(hang bool) { i.hang.Store(hang) }

func (i *Issuer) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	i.discoveries.Add(1)
	if i.hang.Load() {
		select {
		case <-r.Context().Done():
		case <-i.release:
		}
		return
	}
	writeJSON(w, map[string]any{
		"issuer":                                i.URL,
		"authorization_endpoint":                i.URL + "/authorize",
		"token_endpoint":                        i.URL + "/token",
		"jwks_uri":                              i.URL + "/jwks",
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (i *Issuer) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"keys": []map[string]any{{
		"kty": "RSA", "alg": "RS256", "use": "sig", "kid": keyID,
		"n": base64.RawURLEncoding.EncodeToString(i.key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(i.key.E)).Bytes()),
	}}})
}

func (i *Issuer) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, secret := r.FormValue("client_id"), r.FormValue("client_secret")
	if id == "" {
		id, secret, _ = r.BasicAuth()
	}
	if id != i.clientID || secret != i.clientSecret {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": i.URL, "sub": i.subject, "aud": i.clientID,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		"email": i.email, "email_verified": true, "name": "Fake User",
	})
	tok.Header["kid"] = keyID
	signed, err := tok.SignedString(i.key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"access_token": "fake-access", "refresh_token": "fake-refresh",
		"token_type": "Bearer", "expires_in": 3600, "id_token": signed,
	})
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
