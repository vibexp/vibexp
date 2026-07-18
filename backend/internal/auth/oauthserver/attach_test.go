package oauthserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubAccessPolicy is a ConsentAccessChecker that allows every user except those
// listed in denied, or fails outright when err is set.
type stubAccessPolicy struct {
	denied map[string]bool
	err    error
	calls  int
}

func (p *stubAccessPolicy) AllowUser(_ context.Context, userID string) (bool, error) {
	p.calls++
	if p.err != nil {
		return false, p.err
	}
	return !p.denied[userID], nil
}

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

// TestConsentAttach_DeniesUserNotOnAllowlist is the point of #217: a user removed
// from the access allowlist still holds a valid web session until its TTL expires,
// and must not be able to authorize a NEW MCP client in that window. The denial
// must leave the session user-less so no authorization code can ever be issued.
func TestConsentAttach_DeniesUserNotOnAllowlist(t *testing.T) {
	policy := &stubAccessPolicy{denied: map[string]bool{testUserID: true}}
	h := newTestHarness(t, withAccessPolicy(policy))
	defer h.close()

	loginID := h.startAuthorize(t, pkceChallenge())
	res := h.attachUser(t, loginID, testUserID)

	require.Equal(t, http.StatusForbidden, res.status, "a denied user must get 403")
	assert.Equal(t, 1, policy.calls, "the access policy must be consulted")
	assert.Equal(t, "access_restricted", jsonBody(t, res)["code"],
		"the denial must carry the stable access_restricted code the SPA branches on")

	// The user must NOT be bound...
	_, body := h.consentDetails(t, loginID)
	assert.Equal(t, false, body["authenticated"], "a denied attach must not bind the user")

	// ...so no authorization code is issuable for this login session.
	decision := h.postConsentJSON(t, map[string]string{
		"login":  loginID,
		"csrf":   h.svc.signConsent(loginID),
		"action": "approve",
	})
	assert.Equal(t, http.StatusBadRequest, decision.status,
		"a denied user must never reach code issuance")
}

// TestConsentAttach_AllowsUserOnAllowlist proves the re-check is a gate, not a
// wall: a permitted user's consent flow is unchanged.
func TestConsentAttach_AllowsUserOnAllowlist(t *testing.T) {
	policy := &stubAccessPolicy{denied: map[string]bool{"someone-else": true}}
	h := newTestHarness(t, withAccessPolicy(policy))
	defer h.close()

	loginID := h.startAuthorize(t, pkceChallenge())
	res := h.attachUser(t, loginID, testUserID)

	require.Equal(t, http.StatusOK, res.status)
	assert.Equal(t, 1, policy.calls, "the access policy must be consulted")
	assert.Equal(t, true, jsonBody(t, res)["authenticated"])

	status, body := h.consentDetails(t, loginID)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, true, body["authenticated"], "an allowed user must be bound as before")
}

// TestConsentAttach_FailsClosedWhenPolicyErrors proves an undecidable policy (e.g.
// the user lookup fails) never widens access: the user is not bound.
func TestConsentAttach_FailsClosedWhenPolicyErrors(t *testing.T) {
	policy := &stubAccessPolicy{err: errors.New("lookup exploded")}
	h := newTestHarness(t, withAccessPolicy(policy))
	defer h.close()

	loginID := h.startAuthorize(t, pkceChallenge())
	res := h.attachUser(t, loginID, testUserID)

	require.Equal(t, http.StatusInternalServerError, res.status)
	assert.NotEmpty(t, jsonBody(t, res)["error"], "the failure must not leak internals")

	_, body := h.consentDetails(t, loginID)
	assert.Equal(t, false, body["authenticated"], "an undecidable policy must not bind the user")
}

// TestConsentAttach_NoPolicyLeavesFlowUnchanged pins the open-instance default:
// with no allowlist configured the AS is built without a policy and attach behaves
// exactly as it did before #217.
func TestConsentAttach_NoPolicyLeavesFlowUnchanged(t *testing.T) {
	h := newTestHarness(t) // no policy — an unconfigured allowlist
	defer h.close()

	loginID := h.startAuthorize(t, pkceChallenge())
	require.Equal(t, http.StatusOK, h.attachUser(t, loginID, testUserID).status)

	_, body := h.consentDetails(t, loginID)
	assert.Equal(t, true, body["authenticated"])
}
