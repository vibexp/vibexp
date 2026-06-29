package oauthserver

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// consentDetails fetches the JSON consent details for a login id.
func (h *testHarness) consentDetails(t *testing.T, loginID string) (int, map[string]any) {
	t.Helper()
	res := h.get(t, ConsentAPIPath+"?login="+url.QueryEscape(loginID))
	return res.status, jsonBody(t, res)
}

// TestAuthorize_CreatesUserlessSession proves /authorize never authenticates: it
// redirects to the SPA consent page and the login session carries no user, so
// ConsentDetails reports authenticated:false (the needs-login signal).
func TestAuthorize_CreatesUserlessSession(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	loginID := h.startAuthorize(t, pkceChallenge())
	require.NotEmpty(t, loginID)

	status, body := h.consentDetails(t, loginID)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, false, body["authenticated"], "a user-less session must report needs-login")
	assert.NotEmpty(t, body["csrf"], "needs-login response must carry the CSRF token for attach")
	assert.Nil(t, body["client_name"], "no approval-screen details leak before a user is bound")
}

// TestConsentAttach_BindsAuthenticatedUser proves the attach endpoint binds the
// logged-in app user to the session, after which ConsentDetails returns the
// approval-screen contents.
func TestConsentAttach_BindsAuthenticatedUser(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	loginID := h.startAuthorize(t, pkceChallenge())
	res := h.attachUser(t, loginID, testUserID)
	require.Equal(t, http.StatusOK, res.status)
	assert.Equal(t, "no-store", res.header.Get("Cache-Control"))
	assert.Equal(t, true, jsonBody(t, res)["authenticated"])

	status, body := h.consentDetails(t, loginID)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, true, body["authenticated"], "after attach the session must be authenticated")
	assert.Equal(t, h.clientID, body["client_name"])
}

// TestConsentAttach_RequiresSession proves an unauthenticated caller (no
// vx_session) cannot bind a user: the endpoint returns 401.
func TestConsentAttach_RequiresSession(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	loginID := h.startAuthorize(t, pkceChallenge())
	res := h.attachUser(t, loginID, "") // no simulated session
	assert.Equal(t, http.StatusUnauthorized, res.status, "attach without a session must be 401")
	assert.NotEmpty(t, jsonBody(t, res)["error"])

	// The session stays user-less, so consent still gates on login.
	_, body := h.consentDetails(t, loginID)
	assert.Equal(t, false, body["authenticated"], "a rejected attach must not bind a user")
}

// TestConsentAttach_RejectsBadCSRF proves a tampered CSRF token is rejected even
// for an authenticated caller.
func TestConsentAttach_RejectsBadCSRF(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	loginID := h.startAuthorize(t, pkceChallenge())
	res := h.attachWithCSRF(t, loginID, testUserID, "not-the-right-token")
	assert.Equal(t, http.StatusBadRequest, res.status)
	assert.NotEmpty(t, jsonBody(t, res)["error"])
}

// TestConsentAttach_RejectsUnknownLogin proves attaching to an expired/unknown
// login id yields a 4xx JSON error (no leak).
func TestConsentAttach_RejectsUnknownLogin(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	res := h.attachUser(t, "does-not-exist", testUserID)
	assert.Equal(t, http.StatusBadRequest, res.status)
	assert.NotEmpty(t, jsonBody(t, res)["error"])
}

// TestConsentAttach_RejectsRebindToDifferentUser proves a login session bound to
// one user cannot be hijacked by another authenticated caller.
func TestConsentAttach_RejectsRebindToDifferentUser(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	loginID := h.startAuthorize(t, pkceChallenge())
	require.Equal(t, http.StatusOK, h.attachUser(t, loginID, testUserID).status)

	res := h.attachUser(t, loginID, "intruder")
	assert.Equal(t, http.StatusConflict, res.status, "rebinding to a different user must be rejected")
}

// TestConsentDecision_RequiresAttachedUser proves a consent decision cannot
// proceed on a user-less session: a signed-out flow can never reach issuance.
func TestConsentDecision_RequiresAttachedUser(t *testing.T) {
	h := newTestHarness(t)
	defer h.close()

	loginID := h.startAuthorize(t, pkceChallenge())
	res := h.postConsentJSON(t, map[string]string{
		"login":  loginID,
		"csrf":   h.svc.signConsent(loginID),
		"action": "approve",
	})
	assert.Equal(t, http.StatusBadRequest, res.status, "consent must not proceed without an attached user")
}
