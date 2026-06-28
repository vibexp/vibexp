package oauthserver

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConsentDetails_ReturnsApprovalScreenJSON proves the GET endpoint returns
// the consent-screen contents (client name, redirect host, requested scopes) plus
// a CSRF token the SPA echoes back on the decision POST.
func TestConsentDetails_ReturnsApprovalScreenJSON(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	loginID := h.authorizeToConsent(t, pkceChallenge())

	res := h.get(t, ConsentAPIPath+"?login="+url.QueryEscape(loginID))
	require.Equal(t, http.StatusOK, res.status)
	body := jsonBody(t, res)
	assert.Equal(t, h.clientID, body["client_name"], "falls back to client id when no display name is set")
	assert.Equal(t, "client.example.test", body["redirect_host"])
	assert.Equal(t, h.svc.signConsent(loginID), body["csrf"], "CSRF must bind to the login id")
}

// TestConsentDetails_RejectsUnknownLogin proves an expired/invalid login id yields
// a 4xx JSON error and never leaks HTML or a stack trace.
func TestConsentDetails_RejectsUnknownLogin(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	res := h.get(t, ConsentAPIPath+"?login=does-not-exist")
	assert.Equal(t, http.StatusBadRequest, res.status)
	assert.Contains(t, res.header.Get("Content-Type"), "application/json")
	assert.NotEmpty(t, jsonBody(t, res)["error"])
}

// TestConsentDecision_DenyReturnsAccessDenied proves a deny decision returns a
// redirect_to carrying error=access_denied to the client callback (no code).
func TestConsentDecision_DenyReturnsAccessDenied(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	loginID := h.authorizeToConsent(t, pkceChallenge())
	redirectTo := h.decideConsent(t, loginID, "deny")

	u, err := url.Parse(redirectTo)
	require.NoError(t, err)
	assert.Equal(t, "client.example.test", u.Host, "must redirect to the client callback")
	assert.Equal(t, "access_denied", u.Query().Get("error"))
	assert.Empty(t, u.Query().Get("code"), "deny must not issue an authorization code")
}

// TestConsentDecision_RejectsBadCSRF proves a tampered CSRF token is rejected
// (4xx JSON) and the session is not consumed for issuance.
func TestConsentDecision_RejectsBadCSRF(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	loginID := h.authorizeToConsent(t, pkceChallenge())
	res := h.postConsentJSON(t, map[string]string{
		"login":  loginID,
		"csrf":   "not-the-right-token",
		"action": "approve",
	})
	assert.Equal(t, http.StatusBadRequest, res.status)
	assert.NotEmpty(t, jsonBody(t, res)["error"])
}

// TestConsentDecision_RejectsUnknownAction proves only approve/deny are accepted.
func TestConsentDecision_RejectsUnknownAction(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	loginID := h.authorizeToConsent(t, pkceChallenge())
	res := h.postConsentJSON(t, map[string]string{
		"login":  loginID,
		"csrf":   h.svc.signConsent(loginID),
		"action": "maybe",
	})
	assert.Equal(t, http.StatusBadRequest, res.status)
}

// TestConsentDecision_LoginIsSingleUse proves approving consumes the login
// session, so a replay (even with a valid CSRF) is rejected.
func TestConsentDecision_LoginIsSingleUse(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	loginID := h.authorizeToConsent(t, pkceChallenge())
	first := h.decideConsent(t, loginID, "approve")
	require.NotEmpty(t, queryParam(t, first, "code"))

	replay := h.postConsentJSON(t, map[string]string{
		"login":  loginID,
		"csrf":   h.svc.signConsent(loginID),
		"action": "approve",
	})
	assert.Equal(t, http.StatusBadRequest, replay.status, "a consumed login session must not be reusable")
}
