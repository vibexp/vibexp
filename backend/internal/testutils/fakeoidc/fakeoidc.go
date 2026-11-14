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
// so a test can prove how often a session was refreshed. Each refresh token
// belongs to the user it was issued for, and a refresh mints that user's ID
// token, so several users can hold sessions with one issuer.
//
// A revocation endpoint (RFC 7009) can be advertised in discovery with
// SetRevocationEndpoint. It authenticates the client (HTTP Basic or form
// credentials), revokes a live refresh token, records every request and can be
// made to fail or hang, so a test can prove when and how a token was revoked.
//
// New runs the issuer for a Go test; NewHandler builds it for a long-running
// process such as the e2e stack's cmd/oidc-test-issuer, which scripts it
// through SetUser, SetClaims, SetAuthorizeError, RevokeRefreshTokens and Reset.
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
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	key          *rsa.PrivateKey
	clientID     string
	clientSecret string
	// initial is the user New or NewHandler was given; Reset restores it.
	initial identity

	discoveries atomic.Int64
	hang        atomic.Bool
	unverified  atomic.Bool
	release     chan struct{}

	refreshCalls   atomic.Int64
	refreshStatus  atomic.Int32
	jwksStatus     atomic.Int32
	userInfoStatus atomic.Int32
	noRefreshID    atomic.Bool
	forgeIDToken   atomic.Bool

	revocationEnabled atomic.Bool
	revocationStatus  atomic.Int32
	revocationHang    atomic.Bool

	mu              sync.Mutex
	user            identity
	authorizeError  string
	grants          map[string]grant
	nonceOverride   *string
	extraClaims     map[string]any
	scopesSupported []string
	// liveRefresh holds the refresh tokens issued and not yet used, with the
	// user each was issued for, and spentRefresh those already used; revoked
	// is set once a spent one is presented again, or by RevokeRefreshTokens.
	liveRefresh  map[string]identity
	spentRefresh map[string]bool
	revoked      bool
	// revocationAuthMethods is the revocation_endpoint_auth_methods_supported
	// list of discovery (nil omits it); revocations are the requests the
	// revocation endpoint received.
	revocationAuthMethods []string
	revocations           []Revocation
}

// Revocation is one request the revocation endpoint received.
type Revocation struct {
	// Token and TokenTypeHint are the form's token and token_type_hint.
	Token, TokenTypeHint string
	// BasicAuth reports whether the client authenticated with HTTP Basic
	// (client_secret_basic) rather than form credentials (client_secret_post).
	BasicAuth bool
	// Authenticated reports whether the client credentials were correct.
	Authenticated bool
}

// identity is the user an ID token is minted for.
type identity struct {
	subject, email string
}

// grant is what the authorization endpoint recorded for one issued code.
type grant struct {
	challenge, method, nonce string
	user                     identity
}

// Options configures an issuer built with NewHandler.
type Options struct {
	// Issuer is the issuer URL the handler is served at: discovery advertises
	// its endpoints under it and ID tokens carry it as iss.
	Issuer string
	// ClientID and ClientSecret are the only client credentials accepted.
	ClientID, ClientSecret string
	// Subject and Email are the user ID tokens are minted for until SetUser
	// changes it.
	Subject, Email string
}

// New starts an issuer that accepts clientID/clientSecret and signs ID tokens
// for subject and email. It is closed when the test ends.
func New(t testing.TB, clientID, clientSecret, subject, email string) *Issuer {
	t.Helper()
	i, handler, err := newIssuer(Options{
		ClientID: clientID, ClientSecret: clientSecret, Subject: subject, Email: email,
	})
	if err != nil {
		t.Fatalf("fakeoidc: %v", err)
	}
	server := httptest.NewServer(handler)
	i.URL = server.URL
	t.Cleanup(func() {
		close(i.release)
		server.Close()
	})
	return i
}

// NewHandler builds an issuer served at opts.Issuer and returns it with its
// HTTP handler, for a process that serves the handler itself.
func NewHandler(opts Options) (*Issuer, http.Handler, error) {
	i, handler, err := newIssuer(opts)
	if err != nil {
		return nil, nil, err
	}
	i.URL = opts.Issuer
	return i, handler, nil
}

// newIssuer builds an issuer and its routes; the caller sets URL.
func newIssuer(opts Options) (*Issuer, http.Handler, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}
	user := identity{subject: opts.Subject, email: opts.Email}
	i := &Issuer{
		key: key, clientID: opts.ClientID, clientSecret: opts.ClientSecret,
		initial: user, user: user, release: make(chan struct{}),
		grants: map[string]grant{}, liveRefresh: map[string]identity{}, spentRefresh: map[string]bool{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", i.handleDiscovery)
	mux.HandleFunc("/jwks", i.handleJWKS)
	mux.HandleFunc("/authorize", i.handleAuthorize)
	mux.HandleFunc("/token", i.handleToken)
	mux.HandleFunc("/userinfo", i.handleUserInfo)
	mux.HandleFunc("/revoke", i.handleRevoke)
	return i, mux, nil
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

// SetClaims replaces every claim SetClaim added with claims; nil or empty
// removes them all.
func (i *Issuer) SetClaims(claims map[string]any) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.extraClaims = map[string]any{}
	for name, value := range claims {
		if value != nil {
			i.extraClaims[name] = value
		}
	}
}

// SetUser makes the sign-ins authorized from now on mint ID tokens for subject
// and email. A refresh keeps minting for the user its token was issued for.
func (i *Issuer) SetUser(subject, email string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.user = identity{subject: subject, email: email}
}

// SetAuthorizeError makes the authorization endpoint refuse every request from
// now on with the RFC 6749 error code (e.g. access_denied) instead of issuing
// a code; an empty code restores normal answers.
func (i *Issuer) SetAuthorizeError(code string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.authorizeError = code
}

// RevokeRefreshTokens revokes every refresh token issued so far and from now
// on, as reuse detection does: every refresh_token grant answers invalid_grant
// until Reset.
func (i *Issuer) RevokeRefreshTokens() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.revoked = true
}

// Reset puts back what SetUser, SetClaim(s), SetAuthorizeError,
// RevokeRefreshTokens and SetRefreshStatus changed, and forgets every code and
// refresh token issued, so the next script starts from a fresh issuer.
func (i *Issuer) Reset() {
	i.refreshStatus.Store(0)
	i.mu.Lock()
	defer i.mu.Unlock()
	i.user = i.initial
	i.authorizeError = ""
	i.extraClaims = nil
	i.grants = map[string]grant{}
	i.liveRefresh = map[string]identity{}
	i.spentRefresh = map[string]bool{}
	i.revoked = false
}

// SetScopesSupported sets the scopes_supported list of the discovery
// document; nil omits it. It affects clients discovered from now on.
func (i *Issuer) SetScopesSupported(scopes []string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.scopesSupported = scopes
}

// SetRevocationEndpoint sets whether discovery advertises the revocation
// endpoint. It does not until changed, and it affects clients discovered from
// now on.
func (i *Issuer) SetRevocationEndpoint(enabled bool) { i.revocationEnabled.Store(enabled) }

// SetRevocationAuthMethods sets the revocation_endpoint_auth_methods_supported
// list of discovery; nil omits it. It affects clients discovered from now on.
func (i *Issuer) SetRevocationAuthMethods(methods []string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.revocationAuthMethods = methods
}

// SetRevocationStatus makes the revocation endpoint answer status from now on
// (a 5xx simulates an issuer outage); 0 restores normal answers.
func (i *Issuer) SetRevocationStatus(status int) {
	i.revocationStatus.Store(int32(status)) // #nosec G115 -- an HTTP status fits
}

// SetRevocationHang makes the revocation endpoint block until the request is
// cancelled (true) or answer normally again (false).
func (i *Issuer) SetRevocationHang(hang bool) { i.revocationHang.Store(hang) }

// Revocations returns the requests the revocation endpoint has received, in
// order, failed ones included.
func (i *Issuer) Revocations() []Revocation {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]Revocation(nil), i.revocations...)
}

// RefreshCalls reports how many refresh_token grants the token endpoint has
// served, failed ones included.
func (i *Issuer) RefreshCalls() int64 { return i.refreshCalls.Load() }

// SetRefreshStatus makes every refresh_token grant from now on answer status
// (a 5xx simulates an issuer outage); 0 restores normal answers.
func (i *Issuer) SetRefreshStatus(status int) {
	i.refreshStatus.Store(int32(status)) // #nosec G115 -- an HTTP status fits
}

// SetJWKSStatus makes the keys endpoint answer status from now on; 0 restores
// normal answers. A client caches the keys, so it only bites before the first
// verification.
func (i *Issuer) SetJWKSStatus(status int) {
	i.jwksStatus.Store(int32(status)) // #nosec G115 -- an HTTP status fits
}

// SetUserInfoStatus makes the userinfo endpoint answer status from now on; 0
// restores normal answers.
func (i *Issuer) SetUserInfoStatus(status int) {
	i.userInfoStatus.Store(int32(status)) // #nosec G115 -- an HTTP status fits
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
	return i.issueRefreshLocked(i.user)
}

// issueRefreshLocked mints and records a new refresh token for user; i.mu must
// be held.
func (i *Issuer) issueRefreshLocked(user identity) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	token := "rt-" + hex.EncodeToString(b)
	i.liveRefresh[token] = user
	return token
}

// rotate spends token and returns its successor and the user it was issued
// for, or false when token is not a live refresh token. Presenting a spent
// token revokes every token issued.
func (i *Issuer) rotate(token string) (string, identity, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.spentRefresh[token] {
		i.revoked = true
	}
	user, live := i.liveRefresh[token]
	if i.revoked || !live {
		return "", identity{}, false
	}
	delete(i.liveRefresh, token)
	i.spentRefresh[token] = true
	return i.issueRefreshLocked(user), user, true
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
// challenge, nonce and the current user. Where a real issuer would redirect the
// browser to redirect_uri with the code and state, this one answers them as
// JSON: tests drive the callback themselves, and echoing a caller-supplied URL
// as a redirect would be an open redirect. A refusal set by SetAuthorizeError
// is answered the same way, as {"error","state"}.
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
	i.mu.Lock()
	refusal, user := i.authorizeError, i.user
	i.mu.Unlock()
	if refusal != "" {
		writeJSON(w, map[string]string{"error": refusal, "state": q.Get("state")})
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
		user: user,
	}
	i.mu.Unlock()
	writeJSON(w, map[string]string{"code": code, "state": q.Get("state")})
}

// redeem returns the nonce to put in code's ID token, the user it was issued
// for and a fresh refresh token for that user, or false when the code was
// issued with a PKCE challenge that verifier does not meet. A code is consumed
// only when redeemed: golang.org/x/oauth2 retries a failed exchange with the
// other client-auth style, and that retry must fail the same way. A code the
// issuer never issued redeems with no nonce, for the current user.
func (i *Issuer) redeem(code, verifier string) (redeemed, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	g, issued := i.grants[code]
	if issued && g.challenge != "" {
		sum := sha256.Sum256([]byte(verifier))
		want := base64.RawURLEncoding.EncodeToString(sum[:])
		if g.method != "S256" || verifier == "" || subtle.ConstantTimeCompare([]byte(want), []byte(g.challenge)) != 1 {
			return redeemed{}, false
		}
	}
	delete(i.grants, code)
	if !issued {
		g.user = i.user
	}
	out := redeemed{nonce: g.nonce, user: g.user, refreshToken: i.issueRefreshLocked(g.user)}
	if i.nonceOverride != nil {
		out.nonce = *i.nonceOverride
	}
	return out, true
}

// redeemed is what a code exchange answers with.
type redeemed struct {
	nonce, refreshToken string
	user                identity
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
	if i.revocationEnabled.Load() {
		doc["revocation_endpoint"] = i.URL + "/revoke"
	}
	i.mu.Lock()
	if i.scopesSupported != nil {
		doc["scopes_supported"] = i.scopesSupported
	}
	if i.revocationAuthMethods != nil {
		doc["revocation_endpoint_auth_methods_supported"] = i.revocationAuthMethods
	}
	i.mu.Unlock()
	writeJSON(w, doc)
}

func (i *Issuer) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	if status := int(i.jwksStatus.Load()); status != 0 {
		http.Error(w, "keys unavailable", status)
		return
	}
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
	got, ok := i.redeem(r.FormValue("code"), r.FormValue("code_verifier"))
	if !ok {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	signed, err := i.mintIDToken(got.user, got.nonce)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"access_token": "fake-access", "refresh_token": got.refreshToken,
		"token_type": "Bearer", "expires_in": 3600, "id_token": signed,
	})
}

// handleRevoke serves the revocation endpoint (RFC 7009): it records the
// request, authenticates the client, and revokes the token when it is a live
// refresh token. Like a real issuer it answers 200 for a token it does not
// know (§2.2), and 401 invalid_client for bad credentials.
func (i *Issuer) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, secret, basic := r.BasicAuth()
	if basic {
		id, secret = formDecoded(id), formDecoded(secret)
	} else {
		id, secret = r.PostFormValue("client_id"), r.PostFormValue("client_secret")
	}
	authenticated := id == i.clientID && secret == i.clientSecret
	token := r.PostFormValue("token")
	i.mu.Lock()
	i.revocations = append(i.revocations, Revocation{
		Token: token, TokenTypeHint: r.PostFormValue("token_type_hint"), BasicAuth: basic, Authenticated: authenticated,
	})
	i.mu.Unlock()

	if i.revocationHang.Load() {
		select {
		case <-r.Context().Done():
		case <-i.release:
		}
		return
	}
	if status := int(i.revocationStatus.Load()); status != 0 {
		http.Error(w, `{"error":"temporarily_unavailable"}`, status)
		return
	}
	if !authenticated {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	}
	i.mu.Lock()
	delete(i.liveRefresh, token)
	i.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// formDecoded undoes the form encoding RFC 6749 §2.3.1 applies to Basic
// credentials; a malformed value decodes to "", which matches no client.
func formDecoded(v string) string {
	decoded, err := url.QueryUnescape(v)
	if err != nil {
		return ""
	}
	return decoded
}

// handleRefresh serves the refresh_token grant: it rotates token or answers
// invalid_grant, and answers the configured status instead when one is set.
func (i *Issuer) handleRefresh(w http.ResponseWriter, token string) {
	i.refreshCalls.Add(1)
	if status := int(i.refreshStatus.Load()); status != 0 {
		http.Error(w, `{"error":"temporarily_unavailable"}`, status)
		return
	}
	next, user, ok := i.rotate(token)
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
		signed, err := i.mintIDToken(user, "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		body["id_token"] = signed
	}
	writeJSON(w, body)
}

// mintIDToken signs an ID token for user, with nonce when it is not empty and
// the claims SetClaim added.
func (i *Issuer) mintIDToken(user identity, nonce string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": i.URL, "sub": user.subject, "aud": i.clientID,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		"email": user.email, "email_verified": !i.unverified.Load(), "name": "Fake User",
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

// handleUserInfo answers the userinfo endpoint for the current user.
func (i *Issuer) handleUserInfo(w http.ResponseWriter, _ *http.Request) {
	if status := int(i.userInfoStatus.Load()); status != 0 {
		http.Error(w, "userinfo unavailable", status)
		return
	}
	i.mu.Lock()
	user := i.user
	if v, ok := i.extraClaims["sub"].(string); ok {
		user.subject = v
	}
	i.mu.Unlock()
	writeJSON(w, map[string]any{
		"sub": user.subject, "email": user.email, "email_verified": !i.unverified.Load(), "name": "Fake User",
	})
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
