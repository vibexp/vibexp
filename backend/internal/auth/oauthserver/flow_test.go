package oauthserver

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	gojose "github.com/go-jose/go-jose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/mcptoken"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
)

const (
	testIssuer          = "https://as.example.test"
	testResourceURI     = "https://mcp.example.test/mcp"
	testRedirectURI     = "https://client.example.test/callback"
	testFrontendBaseURL = "https://app.example.test"
	testVerifier        = "verifier-abcdefghijklmnopqrstuvwxyz-0123456789-XYZ"
)

// httpResult is a fully-consumed HTTP response: the body is read and closed by
// the request helpers, so tests never leak a response body.
type httpResult struct {
	status int
	header http.Header
	body   []byte
}

type testHarness struct {
	svc      *Service
	server   *httptest.Server
	client   *http.Client
	clientID string
}

func newTestHarness(t *testing.T) *testHarness {
	t.Helper()
	encKey := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	clients := newMemClientRepo()
	svc := NewService(
		Config{
			Issuer:              testIssuer,
			ResourceURI:         testResourceURI,
			FrontendBaseURL:     testFrontendBaseURL,
			AccessTokenTTL:      15 * time.Minute,
			RefreshTokenTTL:     24 * time.Hour,
			AuthCodeTTL:         10 * time.Minute,
			KeyRotationInterval: 720 * time.Hour,
		},
		encKey,
		clients,
		newMemRequestRepo(), newMemRequestRepo(), newMemRequestRepo(), newMemRequestRepo(),
		newMemSigningKeyRepo(),
		newMemLoginSessionRepo(),
		slog.New(slog.DiscardHandler),
	)
	require.NoError(t, svc.keys.EnsureActiveKey(context.Background()))

	clientID := registerTestClient(t, clients)
	return &testHarness{
		svc:    svc,
		server: httptest.NewServer(testMux(svc)),
		client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		clientID: clientID,
	}
}

// testUserID is the app user the test attach middleware binds to a login session.
const testUserID = "user-1"

// testUserHeader carries the simulated authenticated user id into the test attach
// handler (see injectTestUser).
const testUserHeader = "X-Test-User"

// injectTestUser simulates the standard /api auth middleware for the consent
// attach endpoint: when an X-Test-User header is present it puts that user id in
// the context (as flexibleAuthMiddleware does after authenticating a vx_session);
// otherwise it leaves the context unauthenticated so ConsentAttach's own 401 guard
// is exercised.
func injectTestUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if uid := r.Header.Get(testUserHeader); uid != "" {
			r = r.WithContext(context.WithValue(r.Context(), contextkeys.UserID, uid))
		}
		next.ServeHTTP(w, r)
	})
}

func registerTestClient(t *testing.T, clients *memClientRepo) string {
	t.Helper()
	const id = "test-client"
	require.NoError(t, clients.Create(context.Background(), &models.OAuthClient{
		ID:                      id,
		RedirectURIs:            []string{testRedirectURI},
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		Audience:                []string{testResourceURI},
		Public:                  true,
		TokenEndpointAuthMethod: "none",
		CreatedAt:               time.Now(),
	}))
	return id
}

func testMux(svc *Service) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+AuthorizePath, svc.Authorize)
	mux.HandleFunc("GET "+ConsentAPIPath, svc.ConsentDetails)
	mux.HandleFunc("POST "+ConsentAPIPath, svc.ConsentDecision)
	mux.Handle("POST "+ConsentAttachPath, injectTestUser(http.HandlerFunc(svc.ConsentAttach)))
	mux.HandleFunc("POST "+TokenPath, svc.Token)
	mux.HandleFunc("POST "+RegisterPath, svc.Register)
	mux.HandleFunc("GET "+JWKSPath, svc.JWKS)
	mux.HandleFunc("GET "+MetadataPath, svc.Metadata)
	return mux
}

func (h *testHarness) close() { h.server.Close() }

func pkceChallenge() string {
	sum := sha256.Sum256([]byte(testVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// startAuthorize drives GET /authorize and returns the login id presented at the
// SPA consent page. The login session is USER-LESS at this point: the AS never
// authenticates anyone itself. PKCE uses S256 (the only method the AS allows).
func (h *testHarness) startAuthorize(t *testing.T, challenge string) string {
	t.Helper()
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {h.clientID},
		"redirect_uri":          {testRedirectURI},
		"state":                 {"state-abcdefgh"},
		"resource":              {testResourceURI},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	consentLoc := h.requireRedirect(t, h.get(t, AuthorizePath+"?"+q.Encode()))
	require.Contains(t, consentLoc, ConsentPagePath, "authorize must redirect to the SPA consent page")
	return queryParam(t, consentLoc, "login")
}

// attachUser binds an app user to the login session via the authenticated attach
// endpoint, signing the CSRF token bound to the login id. An empty userID omits
// the simulated session so the endpoint's 401 path can be exercised.
func (h *testHarness) attachUser(t *testing.T, loginID, userID string) httpResult {
	t.Helper()
	return h.attachWithCSRF(t, loginID, userID, h.svc.signConsent(loginID))
}

// attachWithCSRF posts to the attach endpoint with an explicit CSRF token so a
// tampered token can be exercised.
func (h *testHarness) attachWithCSRF(t *testing.T, loginID, userID, csrf string) httpResult {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"login": loginID})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, h.server.URL+ConsentAttachPath, bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	if userID != "" {
		req.Header.Set(testUserHeader, userID)
	}
	resp, err := h.client.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	return readResult(t, resp)
}

// authorizeToConsent drives authorize -> SPA consent redirect -> app-user attach,
// returning the login id ready for a consent decision (mirrors the SPA: the user
// is logged into the app and now bound to the user-less login session).
func (h *testHarness) authorizeToConsent(t *testing.T, challenge string) string {
	t.Helper()
	loginID := h.startAuthorize(t, challenge)
	res := h.attachUser(t, loginID, testUserID)
	require.Equal(t, http.StatusOK, res.status, "attaching the app user must succeed")
	return loginID
}

// runAuthorizeToCode drives authorize -> upstream callback -> consent approval and
// returns the issued authorization code.
func (h *testHarness) runAuthorizeToCode(t *testing.T, challenge string) string {
	t.Helper()
	loginID := h.authorizeToConsent(t, challenge)
	clientRedirect := h.decideConsent(t, loginID, "approve")
	return queryParam(t, clientRedirect, "code")
}

// decideConsent posts an approve/deny decision to the JSON consent API and
// returns the captured client redirect URL (redirect_to).
func (h *testHarness) decideConsent(t *testing.T, loginID, action string) string {
	t.Helper()
	res := h.postConsentJSON(t, map[string]string{
		"login":  loginID,
		"csrf":   h.svc.signConsent(loginID),
		"action": action,
	})
	require.Equal(t, http.StatusOK, res.status, "consent decision must return 200 JSON")
	assert.Equal(t, "no-store", res.header.Get("Cache-Control"),
		"the code-bearing decision response must not be cached")
	redirectTo, _ := jsonBody(t, res)["redirect_to"].(string)
	require.NotEmpty(t, redirectTo, "consent decision must return a redirect_to")
	return redirectTo
}

func (h *testHarness) exchangeCode(t *testing.T, code, verifier string) httpResult {
	t.Helper()
	return h.postForm(t, TokenPath, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"client_id":     {h.clientID},
		"code_verifier": {verifier},
	})
}

func (h *testHarness) refresh(t *testing.T, refreshToken string) httpResult {
	t.Helper()
	return h.postForm(t, TokenPath, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {h.clientID},
	})
}

func TestAuthorizationCodeFlow_IssuesAudienceBoundJWT(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	code := h.runAuthorizeToCode(t, pkceChallenge())
	res := h.exchangeCode(t, code, testVerifier)

	require.Equal(t, http.StatusOK, res.status, "token endpoint should succeed")
	body := jsonBody(t, res)
	accessToken, _ := body["access_token"].(string)
	require.NotEmpty(t, accessToken, "access_token must be present")
	assert.Equal(t, "bearer", body["token_type"])
	assert.NotEmpty(t, body["refresh_token"], "refresh_token must be issued")

	claims := h.verifyJWTAgainstJWKS(t, accessToken)
	assert.Equal(t, testIssuer, claims["iss"])
	assert.Equal(t, "user-1", claims["sub"])
	assert.Contains(t, audienceStrings(claims["aud"]), testResourceURI,
		"token must be audience-bound to the MCP resource URI (RFC 8707)")
}

func TestAuthorizationCodeFlow_RejectsWrongPKCEVerifier(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	code := h.runAuthorizeToCode(t, pkceChallenge())
	res := h.exchangeCode(t, code, "wrong-verifier-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	assert.Equal(t, http.StatusBadRequest, res.status)
	assert.Equal(t, "invalid_grant", jsonBody(t, res)["error"], "S256 PKCE mismatch must be rejected")
}

func TestAuthorize_RejectsPlainPKCE(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {h.clientID},
		"redirect_uri":          {testRedirectURI},
		"state":                 {"state-abcdefgh"},
		"code_challenge":        {"plainchallengevalue1234567890abcdefghij"},
		"code_challenge_method": {"plain"},
	}
	loc := h.requireRedirect(t, h.get(t, AuthorizePath+"?"+q.Encode()))
	assert.Contains(t, loc, "error=", "plain PKCE must be rejected")
	assert.NotContains(t, loc, "code=", "no authorization code on plain PKCE")
}

func TestAuthorize_RequiresPKCE(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	q := url.Values{
		"response_type": {"code"},
		"client_id":     {h.clientID},
		"redirect_uri":  {testRedirectURI},
		"state":         {"state-abcdefgh"},
	}
	loc := h.requireRedirect(t, h.get(t, AuthorizePath+"?"+q.Encode()))
	assert.Contains(t, loc, "error=", "missing PKCE must be rejected")
}

func TestRefreshToken_RotatesAndDetectsReuse(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	code := h.runAuthorizeToCode(t, pkceChallenge())
	firstRefresh, _ := jsonBody(t, h.exchangeCode(t, code, testVerifier))["refresh_token"].(string)
	require.NotEmpty(t, firstRefresh)

	// First refresh succeeds and rotates the token.
	res2 := h.refresh(t, firstRefresh)
	require.Equal(t, http.StatusOK, res2.status)
	secondRefresh, _ := jsonBody(t, res2)["refresh_token"].(string)
	require.NotEmpty(t, secondRefresh)
	assert.NotEqual(t, firstRefresh, secondRefresh, "refresh token must rotate")

	// Replaying the first (now rotated) refresh token is detected as reuse.
	res3 := h.refresh(t, firstRefresh)
	assert.Equal(t, http.StatusBadRequest, res3.status)
	assert.Equal(t, "invalid_grant", jsonBody(t, res3)["error"], "reused refresh token must be rejected")

	// Reuse detection revokes the whole family: the rotated token is now dead too.
	res4 := h.refresh(t, secondRefresh)
	assert.Equal(t, http.StatusBadRequest, res4.status,
		"the rotated refresh token must be revoked after reuse is detected")
}

func TestMetadata_AdvertisesS256AndEndpoints(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	res := h.get(t, MetadataPath)
	require.Equal(t, http.StatusOK, res.status)
	md := jsonBody(t, res)
	assert.Equal(t, testIssuer, md["issuer"])
	assert.Equal(t, testIssuer+TokenPath, md["token_endpoint"])
	assert.Equal(t, testIssuer+RegisterPath, md["registration_endpoint"])
	assert.Contains(t, audienceStrings(md["code_challenge_methods_supported"]), "S256")
	// jwks_uri must be the path the verifier (authkit) fetches: <issuer>/oauth2/jwks.json.
	assert.Equal(t, testIssuer+JWKSPath, md["jwks_uri"])
	assert.Contains(t, audienceStrings(md["scopes_supported"]), ScopeMCP)
}

// mcpEchoResolver mimics the MCP resource server's user resolution: the embedded
// AS mints sub = internal user ID, so resolving "by user ID" returns the subject
// itself (as a found user's id would). It echoes the subject so the test can
// assert which user an MCP call would run as.
type mcpEchoResolver struct{}

func (mcpEchoResolver) ResolveUserID(_ context.Context, _, subject string) (string, error) {
	return subject, nil
}

// TestMCPVerifierAcceptsASMintedToken is the end-to-end proof of issue #32: a JWT
// minted by the embedded Authorization Server (sub = internal user ID, aud = MCP
// resource, iss = AS issuer) passes the MCP resource server's verifier
// (mcptoken/authkit), which fetches JWKS from the AS's published
// /oauth2/jwks.json, and resolves to the correct internal users.id. The AS issuer
// must be the live server URL so the verifier's JWKS fetch and iss check both
// resolve against it; handlers are registered after the server starts.
func TestMCPVerifierAcceptsASMintedToken(t *testing.T) {
	encKey := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	clients := newMemClientRepo()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	svc := NewService(
		Config{
			Issuer:              server.URL,
			ResourceURI:         testResourceURI,
			FrontendBaseURL:     testFrontendBaseURL,
			AccessTokenTTL:      15 * time.Minute,
			RefreshTokenTTL:     24 * time.Hour,
			AuthCodeTTL:         10 * time.Minute,
			KeyRotationInterval: 720 * time.Hour,
		},
		encKey,
		clients,
		newMemRequestRepo(), newMemRequestRepo(), newMemRequestRepo(), newMemRequestRepo(),
		newMemSigningKeyRepo(),
		newMemLoginSessionRepo(),
		slog.New(slog.DiscardHandler),
	)
	require.NoError(t, svc.keys.EnsureActiveKey(context.Background()))
	mux.HandleFunc("GET "+AuthorizePath, svc.Authorize)
	mux.HandleFunc("GET "+ConsentAPIPath, svc.ConsentDetails)
	mux.HandleFunc("POST "+ConsentAPIPath, svc.ConsentDecision)
	mux.Handle("POST "+ConsentAttachPath, injectTestUser(http.HandlerFunc(svc.ConsentAttach)))
	mux.HandleFunc("POST "+TokenPath, svc.Token)
	mux.HandleFunc("GET "+JWKSPath, svc.JWKS)

	h := &testHarness{
		svc:    svc,
		server: server,
		client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		clientID: registerTestClient(t, clients),
	}

	code := h.runAuthorizeToCode(t, pkceChallenge())
	tokenRes := h.exchangeCode(t, code, testVerifier)
	require.Equal(t, http.StatusOK, tokenRes.status)
	accessToken, ok := jsonBody(t, tokenRes)["access_token"].(string)
	require.True(t, ok)
	require.NotEmpty(t, accessToken)

	// Verify the token exactly as the MCP resource server does.
	verifier, err := mcptoken.New(context.Background(), server.URL, testResourceURI, mcpEchoResolver{})
	require.NoError(t, err)

	info, err := verifier.Verify(context.Background(), accessToken,
		httptest.NewRequest(http.MethodGet, "/mcp/v1/common", nil))
	require.NoError(t, err, "an AS-minted token must pass the MCP resource server's verifier")
	assert.Equal(t, "user-1", info.UserID, "the MCP call must run as the token's subject user (users.id)")
}

// --- HTTP + assertion helpers ---

func (h *testHarness) get(t *testing.T, path string) httpResult {
	t.Helper()
	resp, err := h.client.Get(h.server.URL + path)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	return readResult(t, resp)
}

func (h *testHarness) postForm(t *testing.T, path string, form url.Values) httpResult {
	t.Helper()
	resp, err := h.client.PostForm(h.server.URL+path, form)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	return readResult(t, resp)
}

// postConsentJSON posts a JSON body to the consent decision endpoint.
func (h *testHarness) postConsentJSON(t *testing.T, payload any) httpResult {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	resp, err := h.client.Post(h.server.URL+ConsentAPIPath, "application/json", bytes.NewReader(data))
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	return readResult(t, resp)
}

// readResult reads the (already open) response; the caller closes the body.
func readResult(t *testing.T, resp *http.Response) httpResult {
	t.Helper()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return httpResult{status: resp.StatusCode, header: resp.Header, body: data}
}

func (h *testHarness) requireRedirect(t *testing.T, res httpResult) string {
	t.Helper()
	require.GreaterOrEqual(t, res.status, http.StatusMultipleChoices, "expected a redirect")
	require.Less(t, res.status, http.StatusBadRequest, "expected a redirect")
	return res.header.Get("Location")
}

func (h *testHarness) verifyJWTAgainstJWKS(t *testing.T, token string) map[string]any {
	t.Helper()
	var set gojose.JSONWebKeySet
	require.NoError(t, json.Unmarshal(h.get(t, JWKSPath).body, &set))

	sig, err := gojose.ParseSigned(token)
	require.NoError(t, err)
	require.Len(t, sig.Signatures, 1)
	kid := sig.Signatures[0].Header.KeyID
	require.NotEmpty(t, kid, "JWT must carry a kid header")

	keys := set.Key(kid)
	require.Len(t, keys, 1, "JWKS must contain the signing key by kid")
	pub, ok := keys[0].Key.(*rsa.PublicKey)
	require.True(t, ok)

	payload, err := sig.Verify(pub)
	require.NoError(t, err, "token must validate against the published JWKS")

	var claims map[string]any
	require.NoError(t, json.Unmarshal(payload, &claims))
	return claims
}

func queryParam(t *testing.T, rawURL, key string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return u.Query().Get(key)
}

func jsonBody(t *testing.T, res httpResult) map[string]any {
	t.Helper()
	out := map[string]any{}
	if len(strings.TrimSpace(string(res.body))) > 0 {
		require.NoError(t, json.Unmarshal(res.body, &out))
	}
	return out
}

func audienceStrings(aud any) []string {
	switch v := aud.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, a := range v {
			if s, ok := a.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
