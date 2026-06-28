package oauthserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (h *testHarness) register(t *testing.T, reqBody string) httpResult {
	t.Helper()
	resp, err := h.client.Post(h.server.URL+RegisterPath, "application/json", strings.NewReader(reqBody))
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	return readResult(t, resp)
}

func TestRegister_PublicClientHappyPath(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	res := h.register(t, `{"redirect_uris":["https://app.example.test/cb"],"client_name":"My App"}`)
	require.Equal(t, http.StatusCreated, res.status)
	body := jsonBody(t, res)
	assert.NotEmpty(t, body["client_id"], "client_id must be issued")
	assert.Equal(t, "none", body["token_endpoint_auth_method"])
	assert.Equal(t, "My App", body["client_name"])
	assert.ElementsMatch(t, []any{"authorization_code", "refresh_token"}, body["grant_types"])
}

func TestRegister_RejectsBadMetadata(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantCode string
	}{
		{"missing redirect_uris", `{"client_name":"x"}`, "invalid_redirect_uri"},
		{"non-absolute redirect", `{"redirect_uris":["/relative"]}`, "invalid_redirect_uri"},
		{"http non-loopback redirect", `{"redirect_uris":["http://evil.example.test/cb"]}`, "invalid_redirect_uri"},
		{"redirect with fragment", `{"redirect_uris":["https://app.example.test/cb#frag"]}`, "invalid_redirect_uri"},
		{"confidential auth method", `{"redirect_uris":["https://app.example.test/cb"],"token_endpoint_auth_method":"client_secret_basic"}`, "invalid_client_metadata"},
		{"unsupported grant", `{"redirect_uris":["https://app.example.test/cb"],"grant_types":["implicit"]}`, "invalid_client_metadata"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHarness(t)
			defer h.close()
			res := h.register(t, tc.body)
			assert.Equal(t, http.StatusBadRequest, res.status)
			assert.Equal(t, tc.wantCode, jsonBody(t, res)["error"])
		})
	}
}

func TestRegister_AllowsLoopbackHTTP(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	res := h.register(t, `{"redirect_uris":["http://127.0.0.1:1234/cb"]}`)
	require.Equal(t, http.StatusCreated, res.status)
	assert.NotEmpty(t, jsonBody(t, res)["client_id"])
}
