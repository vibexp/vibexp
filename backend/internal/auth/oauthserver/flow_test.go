package oauthserver

import (
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

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/models"
)

const (
	testIssuer      = "https://as.example.test"
	testResourceURI = "https://mcp.example.test/mcp"
	testRedirectURI = "https://client.example.test/callback"
	testVerifier    = "verifier-abcdefghijklmnopqrstuvwxyz-0123456789-XYZ"
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
	registry := idp.NewRegistry(&fakeProvider{
		name:   idp.ProviderName("fake"),
		claims: &idp.Claims{Subject: "upstream-sub", Email: "u@example.test", Name: "U"},
	})
	svc := NewService(
		Config{
			Issuer:              testIssuer,
			ResourceURI:         testResourceURI,
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
		registry,
		&fakeProvisioner{user: &models.User{ID: "user-1"}},
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
	mux.HandleFunc("GET "+IDPCallbackPath, svc.IdPCallback)
	mux.HandleFunc("GET "+ConsentPath, svc.ConsentForm)
	mux.HandleFunc("POST "+ConsentPath, svc.ConsentSubmit)
	mux.HandleFunc("POST "+TokenPath, svc.Token)
	mux.HandleFunc("POST "+RegisterPath, svc.Register)
	mux.HandleFunc("GET "+JWKSPath, svc.JWKS)
	mux.HandleFunc("GET "+MetadataPath, svc.Metadata)
	return mux
}

func (h *testHarness) close() { h.server.Close() }

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// runAuthorizeToCode drives authorize -> upstream callback -> consent and returns
// the issued authorization code.
func (h *testHarness) runAuthorizeToCode(t *testing.T, challengeMethod, challenge string) string {
	t.Helper()
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {h.clientID},
		"redirect_uri":          {testRedirectURI},
		"state":                 {"state-abcdefgh"},
		"resource":              {testResourceURI},
		"code_challenge":        {challenge},
		"code_challenge_method": {challengeMethod},
	}
	loginID := queryParam(t, h.requireRedirect(t, h.get(t, AuthorizePath+"?"+q.Encode())), "state")

	cb := IDPCallbackPath + "?code=upstream-code&state=" + url.QueryEscape(loginID)
	consentLoc := h.requireRedirect(t, h.get(t, cb))
	require.Contains(t, consentLoc, ConsentPath)
	loginFromConsent := queryParam(t, consentLoc, "login")

	form := url.Values{
		"login":  {loginFromConsent},
		"csrf":   {h.svc.signConsent(loginFromConsent)},
		"action": {"approve"},
	}
	clientRedirect := h.requireRedirect(t, h.postForm(t, ConsentPath, form))
	return queryParam(t, clientRedirect, "code")
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

	code := h.runAuthorizeToCode(t, "S256", pkceChallenge(testVerifier))
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

	code := h.runAuthorizeToCode(t, "S256", pkceChallenge(testVerifier))
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

	code := h.runAuthorizeToCode(t, "S256", pkceChallenge(testVerifier))
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
