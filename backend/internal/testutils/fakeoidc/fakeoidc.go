// Package fakeoidc is an in-process OpenID Connect issuer for tests: discovery,
// JWKS, an authorization endpoint and a token endpoint that mints a signed ID
// token for a fixed subject. It counts discovery requests, so a test can prove
// when an identity provider was (re)discovered, and can hang discovery to
// simulate an unreachable issuer.
//
// A code issued by the authorization endpoint remembers the request's PKCE
// challenge and nonce: the token endpoint then requires the matching
// code_verifier (RFC 7636, S256 only) and puts that nonce in the ID token, so
// tests run against an issuer that actually enforces both. A code the issuer
// never issued keeps the original behaviour: no PKCE check and no nonce.
//
// The token endpoint also serves the refresh_token grant the way a rotating
// issuer with reuse detection does: every refresh spends the presented refresh
// token and issues a new one, and presenting a spent token revokes every token
// the issuer has issued (invalid_grant from then on). It counts refresh calls,
// so a test can prove how often a session was refreshed.
package fakeoidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
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
	unverified  atomic.Bool
	release     chan struct{}

	refreshCalls  atomic.Int64
	refreshStatus atomic.Int32
	noRefreshID   atomic.Bool
	forgeIDToken  atomic.Bool

	mu              sync.Mutex
	grants          map[string]grant
	nonceOverride   *string
	extraClaims     map[string]any
	scopesSupported []string
	// liveRefresh and spentRefresh are the refresh tokens issued and not yet
	// used, and those already used; revoked is set once a spent one is
	// presented again.
	liveRefresh  map[string]bool
	spentRefresh map[string]bool
	revoked      bool
}

// grant is what the authorization endpoint recorded for one issued code.
type grant struct {
	challenge, method, nonce string
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
		grants: map[string]grant{}, liveRefresh: map[string]bool{}, spentRefresh: map[string]bool{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", i.handleDiscovery)
	mux.HandleFunc("/jwks", i.handleJWKS)
	mux.HandleFunc("/authorize", i.handleAuthorize)
	mux.HandleFunc("/token", i.handleToken)
	mux.HandleFunc("/userinfo", i.handleUserInfo)
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

// SetEmailVerified sets the email_verified claim of the ID tokens minted from
// now on. It is true until changed.
func (i *Issuer) SetEmailVerified(verified bool) { i.unverified.Store(!verified) }

// SetNonceOverride makes every ID token minted from now on carry nonce instead
// of the one recorded at authorization; an empty nonce omits the claim. It
// simulates a replayed or injected ID token.
func (i *Issuer) SetNonceOverride(nonce string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.nonceOverride = &nonce
}

// SetClaim makes every ID token minted from now on carry the claim name with
// value; a nil value removes it again.
func (i *Issuer) SetClaim(name string, value any) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.extraClaims == nil {
		i.extraClaims = map[string]any{}
	}
	if value == nil {
		delete(i.extraClaims, name)
		return
	}
	i.extraClaims[name] = value
}

// SetScopesSupported sets the scopes_supported list of the discovery
// document; nil omits it. It affects clients discovered from now on.
func (i *Issuer) SetScopesSupported(scopes []string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.scopesSupported = scopes
}

// RefreshCalls reports how many refresh_token grants the token endpoint has
// served, failed ones included.
func (i *Issuer) RefreshCalls() int64 { return i.refreshCalls.Load() }

// SetRefreshStatus makes every refresh_token grant from now on answer status
// (a 5xx simulates an issuer outage); 0 restores normal answers.
func (i *Issuer) SetRefreshStatus(status int) {
	i.refreshStatus.Store(int32(status)) // #nosec G115 -- an HTTP status fits
}

// SetRefreshIDToken sets whether a refresh response carries an ID token. It
// does until changed; without one a client falls back to userinfo.
func (i *Issuer) SetRefreshIDToken(include bool) { i.noRefreshID.Store(!include) }

// SetForgeIDTokens makes every ID token minted from now on signed by a key
// the issuer does not publish, so its signature does not verify.
func (i *Issuer) SetForgeIDTokens(forge bool) { i.forgeIDToken.Store(forge) }

// IssueRefreshToken issues a live refresh token, as a sign-in would, so a test
// can store a session without running the authorization flow.
func (i *Issuer) IssueRefreshToken() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.issueRefreshLocked()
}

// issueRefreshLocked mints and records a new refresh token; i.mu must be held.
func (i *Issuer) issueRefreshLocked() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	token := "rt-" + hex.EncodeToString(b)
	i.liveRefresh[token] = true
	return token
}

// rotate spends token and returns its successor, or false when token is not a
// live refresh token. Presenting a spent token revokes every token issued.
func (i *Issuer) rotate(token string) (string, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.spentRefresh[token] {
		i.revoked = true
	}
	if i.revoked || !i.liveRefresh[token] {
		return "", false
	}
	delete(i.liveRefresh, token)
	i.spentRefresh[token] = true
	return i.issueRefreshLocked(), true
}

// Authorize runs authURL (an authorization URL pointing at a fake issuer)
// without a browser and returns the code the issuer issued for it.
func Authorize(t testing.TB, authURL string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, authURL, nil)
	if err != nil {
		t.Fatalf("fakeoidc: authorize: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fakeoidc: authorize: %v", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("fakeoidc: authorize: close body: %v", cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fakeoidc: authorize: status %d", resp.StatusCode)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("fakeoidc: authorize: decode: %v", err)
	}
	return body.Code
}

// handleAuthorize issues a fresh code for the request, remembering its PKCE
// challenge and nonce. Where a real issuer would redirect the browser to
// redirect_uri with the code and state, this one answers them as JSON: tests
// drive the callback themselves, and echoing a caller-supplied URL as a
// redirect would be an open redirect.
func (i *Issuer) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("client_id") != i.clientID {
		http.Error(w, "unknown client_id", http.StatusBadRequest)
		return
	}
	if q.Get("redirect_uri") == "" {
		http.Error(w, "missing redirect_uri", http.StatusBadRequest)
		return
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	code := hex.EncodeToString(b)
	i.mu.Lock()
	i.grants[code] = grant{
		challenge: q.Get("code_challenge"), method: q.Get("code_challenge_method"), nonce: q.Get("nonce"),
	}
	i.mu.Unlock()
	writeJSON(w, map[string]string{"code": code, "state": q.Get("state")})
}

// redeem returns the nonce to put in code's ID token, or false when the code
// was issued with a PKCE challenge that verifier does not meet. A code is
// consumed only when redeemed: golang.org/x/oauth2 retries a failed exchange
// with the other client-auth style, and that retry must fail the same way. A
// code the issuer never issued redeems with no nonce.
func (i *Issuer) redeem(code, verifier string) (string, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	g, issued := i.grants[code]
	if issued && g.challenge != "" {
		sum := sha256.Sum256([]byte(verifier))
		want := base64.RawURLEncoding.EncodeToString(sum[:])
		if g.method != "S256" || verifier == "" || subtle.ConstantTimeCompare([]byte(want), []byte(g.challenge)) != 1 {
			return "", false
		}
	}
	delete(i.grants, code)
	if i.nonceOverride != nil {
		return *i.nonceOverride, true
	}
	return g.nonce, true
}

func (i *Issuer) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	i.discoveries.Add(1)
	if i.hang.Load() {
		select {
		case <-r.Context().Done():
		case <-i.release:
		}
		return
	}
	doc := map[string]any{
		"issuer":                                i.URL,
		"authorization_endpoint":                i.URL + "/authorize",
		"token_endpoint":                        i.URL + "/token",
		"userinfo_endpoint":                     i.URL + "/userinfo",
		"jwks_uri":                              i.URL + "/jwks",
		"id_token_signing_alg_values_supported": []string{"RS256"},
	}
	i.mu.Lock()
	if i.scopesSupported != nil {
		doc["scopes_supported"] = i.scopesSupported
	}
	i.mu.Unlock()
	writeJSON(w, doc)
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
	if r.FormValue("grant_type") == "refresh_token" {
		i.handleRefresh(w, r.FormValue("refresh_token"))
		return
	}
	nonce, ok := i.redeem(r.FormValue("code"), r.FormValue("code_verifier"))
	if !ok {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	signed, err := i.mintIDToken(nonce)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	i.mu.Lock()
	i.liveRefresh["fake-refresh"] = true
	i.mu.Unlock()
	writeJSON(w, map[string]any{
		"access_token": "fake-access", "refresh_token": "fake-refresh",
		"token_type": "Bearer", "expires_in": 3600, "id_token": signed,
	})
}

// handleRefresh serves the refresh_token grant: it rotates token or answers
// invalid_grant, and answers the configured status instead when one is set.
func (i *Issuer) handleRefresh(w http.ResponseWriter, token string) {
	i.refreshCalls.Add(1)
	if status := int(i.refreshStatus.Load()); status != 0 {
		http.Error(w, `{"error":"temporarily_unavailable"}`, status)
		return
	}
	next, ok := i.rotate(token)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	body := map[string]any{
		"access_token": "fake-access-" + next, "refresh_token": next,
		"token_type": "Bearer", "expires_in": 3600,
	}
	if !i.noRefreshID.Load() {
		signed, err := i.mintIDToken("")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		body["id_token"] = signed
	}
	writeJSON(w, body)
}

// mintIDToken signs an ID token for the issuer's subject, with nonce when it
// is not empty and the claims SetClaim added.
func (i *Issuer) mintIDToken(nonce string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": i.URL, "sub": i.subject, "aud": i.clientID,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		"email": i.email, "email_verified": !i.unverified.Load(), "name": "Fake User",
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	i.mu.Lock()
	for name, value := range i.extraClaims {
		claims[name] = value
	}
	i.mu.Unlock()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = keyID
	key := i.key
	if i.forgeIDToken.Load() {
		forged, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return "", err
		}
		key = forged
	}
	return tok.SignedString(key)
}

// handleUserInfo answers the userinfo endpoint for the issuer's subject.
func (i *Issuer) handleUserInfo(w http.ResponseWriter, _ *http.Request) {
	sub := i.subject
	i.mu.Lock()
	if v, ok := i.extraClaims["sub"].(string); ok {
		sub = v
	}
	i.mu.Unlock()
	writeJSON(w, map[string]any{
		"sub": sub, "email": i.email, "email_verified": !i.unverified.Load(), "name": "Fake User",
	})
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
