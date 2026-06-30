package oauthserver

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oauthErrorCodes is the set of RFC 6749 token-endpoint error codes; failures
// must surface one of these (never an ad-hoc string or internal detail).
var oauthErrorCodes = map[string]struct{}{
	"invalid_request":        {},
	"invalid_client":         {},
	"invalid_grant":          {},
	"unauthorized_client":    {},
	"unsupported_grant_type": {},
	"invalid_scope":          {},
}

func requireOAuthError(t *testing.T, res httpResult, wantCode string) {
	t.Helper()
	require.GreaterOrEqual(t, res.status, http.StatusBadRequest, "token failure must be a 4xx")
	code, _ := jsonBody(t, res)["error"].(string)
	require.NotEmpty(t, code, "an OAuth error must carry an error code")
	_, known := oauthErrorCodes[code]
	assert.True(t, known, "error %q must be a standard RFC 6749 code", code)
	if wantCode != "" {
		assert.Equal(t, wantCode, code)
	}
}

func TestToken_RejectsUnknownCode(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	res := h.exchangeCode(t, "code-that-was-never-issued", testVerifier)
	requireOAuthError(t, res, "invalid_grant")
}

// TestToken_DetectsCodeReuse: an authorization code is single-use. The second
// exchange of the same code must fail (and fosite revokes anything issued).
func TestToken_DetectsCodeReuse(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	code := h.runAuthorizeToCode(t, pkceChallenge())
	require.Equal(t, http.StatusOK, h.exchangeCode(t, code, testVerifier).status)

	res := h.exchangeCode(t, code, testVerifier)
	requireOAuthError(t, res, "invalid_grant")
}

func TestToken_RejectsMissingPKCEVerifier(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	code := h.runAuthorizeToCode(t, pkceChallenge())
	res := h.postForm(t, url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {testRedirectURI},
		"client_id":    {h.clientID},
		// code_verifier intentionally omitted
	})
	requireOAuthError(t, res, "invalid_grant")
}

func TestToken_RejectsRedirectURIMismatch(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	code := h.runAuthorizeToCode(t, pkceChallenge())
	res := h.postForm(t, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"https://client.example.test/not-the-one-used"},
		"client_id":     {h.clientID},
		"code_verifier": {testVerifier},
	})
	requireOAuthError(t, res, "")
}

func TestToken_RejectsUnknownClient(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	code := h.runAuthorizeToCode(t, pkceChallenge())
	res := h.postForm(t, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"client_id":     {"client-that-was-never-registered"},
		"code_verifier": {testVerifier},
	})
	requireOAuthError(t, res, "")
}

// TestToken_RejectsUnsupportedGrantType: the composed handler set is OAuth 2.1
// core only (authorization_code + refresh_token); client_credentials must not be
// accepted.
func TestToken_RejectsUnsupportedGrantType(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	res := h.postForm(t, url.Values{
		"grant_type": {"client_credentials"},
		"client_id":  {h.clientID},
	})
	requireOAuthError(t, res, "")
}

// TestToken_DoesNotLeakDebugDetail verifies SendDebugMessagesToClients is off:
// an error response must not echo internal library names or source locations.
func TestToken_DoesNotLeakDebugDetail(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	res := h.exchangeCode(t, "code-that-was-never-issued", testVerifier)
	body := strings.ToLower(string(res.body))
	assert.NotContains(t, body, "fosite", "internal library name must not leak to clients")
	assert.NotContains(t, body, ".go:", "source locations must not leak to clients")
}
