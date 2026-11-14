package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/testutils/fakeoidc"
)

const (
	testIssuer   = "http://localhost:9003"
	testClientID = "e2e-oidc"
	testSecret   = "e2e-oidc-client-credential"
)

// newTestServer serves the issuer and its control API.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	handler, err := newHandler(fakeoidc.Options{
		Issuer: testIssuer, ClientID: testClientID, ClientSecret: testSecret,
		Subject: "default-sub", Email: "default@vibexp.test",
	})
	require.NoError(t, err)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// do sends a request with body (form-encoded when it is url.Values, raw JSON
// when it is a string, marshalled otherwise) and returns the status and the
// decoded JSON answer, if any.
func do(t *testing.T, method, target string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	contentType := ""
	switch b := body.(type) {
	case nil:
	case url.Values:
		reader, contentType = strings.NewReader(b.Encode()), "application/x-www-form-urlencoded"
	case string:
		reader, contentType = strings.NewReader(b), "application/json"
	default:
		raw, err := json.Marshal(b)
		require.NoError(t, err)
		reader, contentType = strings.NewReader(string(raw)), "application/json"
	}
	req, err := http.NewRequestWithContext(context.Background(), method, target, reader)
	require.NoError(t, err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	// Error answers carry a JSON body under a text/plain content type, as
	// http.Error writes them, so decode whatever parses.
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		out = nil
	}
	return resp.StatusCode, out
}

// authorize runs the authorization endpoint and returns its JSON answer.
func authorize(t *testing.T, base string) map[string]any {
	t.Helper()
	q := url.Values{"client_id": {testClientID}, "redirect_uri": {"http://localhost:8080/cb"}, "state": {"st"}}
	status, body := do(t, http.MethodGet, base+"/authorize?"+q.Encode(), nil)
	require.Equal(t, http.StatusOK, status)
	return body
}

// token calls the token endpoint with form.
func token(t *testing.T, base string, form url.Values) (int, map[string]any) {
	t.Helper()
	form.Set("client_id", testClientID)
	form.Set("client_secret", testSecret)
	return do(t, http.MethodPost, base+"/token", form)
}

// signIn runs a whole sign-in and returns the ID token's claims and the
// refresh token.
func signIn(t *testing.T, base string) (map[string]any, string) {
	t.Helper()
	code, ok := authorize(t, base)["code"].(string)
	require.True(t, ok, "authorize issued a code")
	status, tokens := token(t, base, url.Values{"grant_type": {"authorization_code"}, "code": {code}})
	require.Equal(t, http.StatusOK, status)
	refresh, _ := tokens["refresh_token"].(string)
	idToken, _ := tokens["id_token"].(string)
	return claimsOf(t, idToken), refresh
}

// claimsOf decodes an ID token's payload; fakeoidc's own tests and the
// identity-provider clients' tests cover its signature.
func claimsOf(t *testing.T, idToken string) map[string]any {
	t.Helper()
	parts := strings.Split(idToken, ".")
	require.Len(t, parts, 3)
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var claims map[string]any
	require.NoError(t, json.Unmarshal(raw, &claims))
	return claims
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(t)
	status, _ := do(t, http.MethodGet, srv.URL+"/healthz", nil)
	assert.Equal(t, http.StatusOK, status)
}

func TestDiscovery_AdvertisesTheIssuerFlag(t *testing.T) {
	srv := newTestServer(t)
	status, doc := do(t, http.MethodGet, srv.URL+"/.well-known/openid-configuration", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, testIssuer, doc["issuer"])
	assert.Equal(t, testIssuer+"/token", doc["token_endpoint"])
}

func TestControlUser_ScriptsTheNextSignIn(t *testing.T) {
	srv := newTestServer(t)
	status, _ := do(t, http.MethodPost, srv.URL+"/_control/user", map[string]any{
		"sub": "sso-1", "email": "sso-1@vibexp.test", "claims": map[string]any{"groups": []string{"vibexp-admins"}},
	})
	require.Equal(t, http.StatusNoContent, status)

	claims, _ := signIn(t, srv.URL)
	assert.Equal(t, "sso-1", claims["sub"])
	assert.Equal(t, "sso-1@vibexp.test", claims["email"])
	assert.Equal(t, []any{"vibexp-admins"}, claims["groups"])
	assert.Equal(t, testIssuer, claims["iss"])
}

func TestControlClaims_ChangesWhatARefreshMints(t *testing.T) {
	srv := newTestServer(t)
	do(t, http.MethodPost, srv.URL+"/_control/claims", map[string]any{"name": "groups", "value": []string{"vibexp-admins"}})
	_, refresh := signIn(t, srv.URL)

	status, _ := do(t, http.MethodPost, srv.URL+"/_control/claims", map[string]any{"name": "groups", "value": []string{}})
	require.Equal(t, http.StatusNoContent, status)

	status, tokens := token(t, srv.URL, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
	require.Equal(t, http.StatusOK, status)
	idToken, _ := tokens["id_token"].(string)
	assert.Equal(t, []any{}, claimsOf(t, idToken)["groups"])
}

func TestControlRefresh_InvalidGrantRevokesUntilReset(t *testing.T) {
	srv := newTestServer(t)
	_, refresh := signIn(t, srv.URL)

	status, _ := do(t, http.MethodPost, srv.URL+"/_control/refresh", map[string]any{"error": "invalid_grant"})
	require.Equal(t, http.StatusNoContent, status)
	status, body := token(t, srv.URL, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "invalid_grant", body["error"])

	status, _ = do(t, http.MethodPost, srv.URL+"/_control/reset", nil)
	require.Equal(t, http.StatusNoContent, status)
	_, refresh = signIn(t, srv.URL)
	status, _ = token(t, srv.URL, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
	assert.Equal(t, http.StatusOK, status)
}

func TestControlStats_CountsRefreshes(t *testing.T) {
	srv := newTestServer(t)
	_, refresh := signIn(t, srv.URL)
	_, before := do(t, http.MethodGet, srv.URL+"/_control/stats", nil)
	assert.InDelta(t, 0, before["refresh_calls"], 0)

	status, _ := token(t, srv.URL, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
	require.Equal(t, http.StatusOK, status)

	_, after := do(t, http.MethodGet, srv.URL+"/_control/stats", nil)
	assert.InDelta(t, 1, after["refresh_calls"], 0)
}

func TestControlRefresh_Status(t *testing.T) {
	srv := newTestServer(t)
	_, refresh := signIn(t, srv.URL)
	do(t, http.MethodPost, srv.URL+"/_control/refresh", map[string]any{"status": http.StatusServiceUnavailable})

	status, _ := token(t, srv.URL, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}})
	assert.Equal(t, http.StatusServiceUnavailable, status)
}

func TestControlAuthorizeError_RefusesUntilCleared(t *testing.T) {
	srv := newTestServer(t)
	status, _ := do(t, http.MethodPost, srv.URL+"/_control/authorize-error", map[string]any{"error": "access_denied"})
	require.Equal(t, http.StatusNoContent, status)
	assert.Equal(t, map[string]any{"error": "access_denied", "state": "st"}, authorize(t, srv.URL))

	do(t, http.MethodPost, srv.URL+"/_control/authorize-error", map[string]any{"error": ""})
	assert.NotEmpty(t, authorize(t, srv.URL)["code"])
}

func TestControlReset_RestoresTheFlagsUser(t *testing.T) {
	srv := newTestServer(t)
	do(t, http.MethodPost, srv.URL+"/_control/user", map[string]any{
		"sub": "other", "email": "other@vibexp.test", "claims": map[string]any{"groups": []string{"g"}},
	})
	do(t, http.MethodPost, srv.URL+"/_control/reset", nil)

	claims, _ := signIn(t, srv.URL)
	assert.Equal(t, "default-sub", claims["sub"])
	assert.NotContains(t, claims, "groups")
}

func TestControl_RejectsBadRequests(t *testing.T) {
	srv := newTestServer(t)
	tests := []struct {
		name, path string
		body       any
	}{
		{"user without sub", "/_control/user", map[string]any{"email": "a@vibexp.test"}},
		{"user without email", "/_control/user", map[string]any{"sub": "a"}},
		{"claim without name", "/_control/claims", map[string]any{"value": "x"}},
		{"unsupported refresh error", "/_control/refresh", map[string]any{"error": "server_error"}},
		{"malformed body", "/_control/claims", "{"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := do(t, http.MethodPost, srv.URL+tt.path, tt.body)
			assert.Equal(t, http.StatusBadRequest, status)
		})
	}
	status, _ := do(t, http.MethodGet, srv.URL+"/_control/reset", nil)
	assert.Equal(t, http.StatusNotFound, status, "a control route is POST only; anything else reaches the issuer")
}
