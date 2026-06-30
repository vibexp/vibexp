package oauthserver

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// baseAuthorizeQuery is a valid S256 authorize request; callers tweak one field
// to exercise a specific validation path.
func baseAuthorizeQuery(clientID string) url.Values {
	return url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {testRedirectURI},
		"state":                 {"state-abcdefgh"},
		"resource":              {testResourceURI},
		"code_challenge":        {pkceChallenge()},
		"code_challenge_method": {"S256"},
	}
}

// authorizeToCodeWithQuery drives authorize -> consent attach -> approve with a
// caller-supplied authorize query and returns the issued authorization code.
func (h *testHarness) authorizeToCodeWithQuery(t *testing.T, q url.Values) string {
	t.Helper()
	consentLoc := h.requireRedirect(t, h.get(t, AuthorizePath+"?"+q.Encode()))
	require.Contains(t, consentLoc, ConsentPagePath, "authorize must redirect to the SPA consent page")
	loginID := queryParam(t, consentLoc, "login")
	require.Equal(t, http.StatusOK, h.attachUser(t, loginID, testUserID).status)
	return queryParam(t, h.decideConsent(t, loginID, "approve"), "code")
}

// TestAuthorize_RejectsForeignResource covers the request-side of RFC 8707: a
// `resource` other than the AS's own resource URI is rejected before login, so a
// client can never obtain a token audience-bound to someone else's server.
func TestAuthorize_RejectsForeignResource(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	q := baseAuthorizeQuery(h.clientID)
	q.Set("resource", "https://attacker.example.test/mcp")
	loc := h.requireRedirect(t, h.get(t, AuthorizePath+"?"+q.Encode()))

	assert.Contains(t, loc, "error=invalid_request", "a foreign resource must be rejected")
	assert.NotContains(t, loc, "code=", "no code on a rejected resource")
	assert.NotContains(t, loc, ConsentPagePath, "a rejected resource never reaches the consent gate")
}

// TestAuthorize_RejectsWhenAnyResourceIsForeign: with multiple `resource`
// indicators, every one must match — a single foreign value fails the request.
func TestAuthorize_RejectsWhenAnyResourceIsForeign(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	q := baseAuthorizeQuery(h.clientID)
	q["resource"] = []string{testResourceURI, "https://attacker.example.test/mcp"}
	loc := h.requireRedirect(t, h.get(t, AuthorizePath+"?"+q.Encode()))

	assert.Contains(t, loc, "error=invalid_request")
	assert.NotContains(t, loc, ConsentPagePath)
}

// TestAuthorize_OmittedResourceBindsToDefaultAudience: omitting `resource`
// altogether is allowed (RFC 8707 makes it optional) and the issued token is
// still bound to the AS's configured resource URI.
func TestAuthorize_OmittedResourceBindsToDefaultAudience(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	q := baseAuthorizeQuery(h.clientID)
	q.Del("resource")
	code := h.authorizeToCodeWithQuery(t, q)

	res := h.exchangeCode(t, code, testVerifier)
	require.Equal(t, http.StatusOK, res.status)
	accessToken, _ := jsonBody(t, res)["access_token"].(string)
	require.NotEmpty(t, accessToken)

	claims := h.verifyJWTAgainstJWKS(t, accessToken)
	assert.Contains(t, audienceStrings(claims["aud"]), testResourceURI,
		"an absent resource indicator still binds the token to the AS's resource URI")
}
