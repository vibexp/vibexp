package oauthserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// auditCapture keeps every event it is given.
type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (c *auditCapture) Record(_ context.Context, e audit.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *auditCapture) only(t *testing.T) audit.Event {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	require.Len(t, c.events, 1, "exactly one audit event")
	return c.events[0]
}

// nameTestClient gives the harness's client a display name.
func nameTestClient(t *testing.T, h *testHarness, name string) {
	t.Helper()
	clients, ok := h.svc.clients.(*memClientRepo)
	require.True(t, ok)
	c, err := clients.GetByID(context.Background(), h.clientID)
	require.NoError(t, err)
	c.ClientName = name
	require.NoError(t, clients.Create(context.Background(), c))
}

func TestAudit_ClientRegistration(t *testing.T) {
	rec := &auditCapture{}
	h := newTestHarness(t, withAudit(rec))
	defer h.close()

	res := h.register(t, `{"redirect_uris":["https://app.example.test/cb"],"client_name":"Claude Code"}`)
	require.Equal(t, http.StatusCreated, res.status)
	clientID, _ := jsonBody(t, res)["client_id"].(string)

	e := rec.only(t)
	assert.Equal(t, models.AuditOpAuthMCPClientRegistered, e.Operation)
	assert.Equal(t, models.AuditSourceMCP, e.Source)
	require.NotNil(t, e.Actor)
	assert.Nil(t, e.Actor.UserID, "registration is anonymous")
	assert.Equal(t, models.AuditActorAnonymous, e.Actor.Type)
	assert.Equal(t, models.AuditResourceTypeOAuthClient, e.ResourceType)
	assert.Equal(t, clientID, e.ResourceID)
	assert.Equal(t, "Claude Code", e.ResourceName)
	assert.Equal(t, "Claude Code", e.Metadata["client_name"])
	assert.Equal(t, []string{"https://app.example.test/cb"}, e.Metadata["redirect_uris"])
}

func TestAudit_RejectedRegistrationRecordsNothing(t *testing.T) {
	rec := &auditCapture{}
	h := newTestHarness(t, withAudit(rec))
	defer h.close()

	res := h.register(t, `{"redirect_uris":[]}`)
	require.Equal(t, http.StatusBadRequest, res.status)
	assert.Empty(t, rec.events)
}

func TestAudit_ConsentDecisions(t *testing.T) {
	for action, op := range map[string]models.AuditOperation{
		consentApprove: models.AuditOpAuthMCPConsentGranted,
		consentDeny:    models.AuditOpAuthMCPConsentDenied,
	} {
		t.Run(action, func(t *testing.T) {
			rec := &auditCapture{}
			h := newTestHarness(t, withAudit(rec))
			defer h.close()
			nameTestClient(t, h, "Cursor")

			loginID := h.authorizeToConsent(t, pkceChallenge())
			redirectTo := h.decideConsent(t, loginID, action)
			require.NotEmpty(t, redirectTo)

			e := rec.only(t)
			assert.Equal(t, op, e.Operation)
			require.NotNil(t, e.Actor)
			require.NotNil(t, e.Actor.UserID, "the actor is the login session's user")
			assert.Equal(t, testUserID, *e.Actor.UserID)
			assert.Equal(t, h.clientID, e.ResourceID)
			assert.Equal(t, "Cursor", e.Metadata["client_name"])
			assert.Equal(t, "Cursor", e.ResourceName)
			assert.Contains(t, e.Metadata, "scopes")

			// No secret of the flow reaches the event: not the authorization
			// code the redirect carries, nor the consent CSRF token.
			doc, err := json.Marshal(e)
			require.NoError(t, err)
			if code := strings.SplitN(redirectTo, "code=", 2); len(code) == 2 {
				assert.NotContains(t, string(doc), strings.SplitN(code[1], "&", 2)[0])
			}
			assert.NotContains(t, string(doc), h.svc.signConsent(loginID))
		})
	}
}

func TestAudit_UnnamedClientFallsBackToItsID(t *testing.T) {
	rec := &auditCapture{}
	h := newTestHarness(t, withAudit(rec))
	defer h.close()

	h.decideConsent(t, h.authorizeToConsent(t, pkceChallenge()), consentApprove)

	assert.Equal(t, h.clientID, rec.only(t).Metadata["client_name"])
}

// panickyRecorder panics: it must not change the consent response.
type panickyRecorder struct{}

func (panickyRecorder) Record(context.Context, audit.Event) { panic("audit down") }

func TestAudit_PanickingRecorderNeverChangesTheConsentResponse(t *testing.T) {
	h := newTestHarness(t, withAudit(panickyRecorder{}))
	defer h.close()

	redirectTo := h.decideConsent(t, h.authorizeToConsent(t, pkceChallenge()), consentApprove)

	assert.NotEmpty(t, queryParam(t, redirectTo, "code"), "the code is still issued")
}
