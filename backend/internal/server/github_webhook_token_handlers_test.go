package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const (
	webhookTestRoutingSegment = "test-routing-segment"
	webhookTestSecret         = "the-teams-own-webhook-secret"
	webhookTestAppCfg         = "cfg-team-a"
	webhookTestTeamID         = "team-a"
	webhookTestPayload        = `{"action":"created","installation":{"id":4242}}`
)

type mockWebhookContainer struct {
	BaseMockContainer
	mock.Mock
	appConfigSvc *svcmocks.MockGitHubAppConfigServiceInterface
	githubSvc    *svcmocks.MockGitHubAppServiceInterface
	webhookRepo  *repomocks.MockWebhookEventRepository
}

func (m *mockWebhookContainer) GitHubAppConfigService() services.GitHubAppConfigServiceInterface {
	return m.appConfigSvc
}

func (m *mockWebhookContainer) GitHubAppService() services.GitHubAppServiceInterface {
	return m.githubSvc
}

func (m *mockWebhookContainer) WebhookEventRepository() repositories.WebhookEventRepository {
	return m.webhookRepo
}

func newMockWebhookContainer(t *testing.T) *mockWebhookContainer {
	return &mockWebhookContainer{
		appConfigSvc: svcmocks.NewMockGitHubAppConfigServiceInterface(t),
		githubSvc:    svcmocks.NewMockGitHubAppServiceInterface(t),
		webhookRepo:  repomocks.NewMockWebhookEventRepository(t),
	}
}

func newWebhookTestServer(t *testing.T, c *mockWebhookContainer) *Server {
	t.Helper()
	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: c,
		logger:    slog.New(slog.DiscardHandler),
		config:    &config.Config{},
		router:    r,
	}
	r.Post("/api/v1/webhooks/github/{token}", srv.handleGitHubWebhookByToken)
	return srv
}

// signWebhook produces the header GitHub would send for this body and secret.
func signWebhook(body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func webhookRequest(token, body, signature string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/"+token, bytes.NewReader([]byte(body)))
	if signature != "" {
		req.Header.Set(githubWebhookSignatureHeader, signature)
	}
	req.Header.Set("X-GitHub-Event", "installation")
	req.Header.Set("X-GitHub-Delivery", "delivery-1")
	return req
}

func validTarget() *services.WebhookDeliveryTarget {
	return &services.WebhookDeliveryTarget{
		AppConfigID:   webhookTestAppCfg,
		TeamID:        webhookTestTeamID,
		WebhookSecret: webhookTestSecret,
	}
}

func TestWebhookByToken_ValidDeliveryIsProcessed(t *testing.T) {
	c := newMockWebhookContainer(t)
	c.appConfigSvc.EXPECT().ResolveWebhookToken(mock.Anything, webhookTestRoutingSegment).
		Return(validTarget(), nil)
	c.webhookRepo.EXPECT().IsProcessed(mock.Anything, "delivery-1").Return(false, nil)
	// Dispatch must be scoped to the App the delivery arrived for.
	c.githubSvc.EXPECT().
		HandleWebhookEvent(mock.Anything, webhookTestAppCfg, "installation", int64(4242), "created").
		Return(nil)
	c.webhookRepo.EXPECT().
		MarkProcessed(mock.Anything, "delivery-1", "installation", mock.Anything).Return(nil)

	req := webhookRequest(webhookTestRoutingSegment, webhookTestPayload,
		signWebhook(webhookTestPayload, webhookTestSecret))
	w := httptest.NewRecorder()
	newWebhookTestServer(t, c).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
}

// The load-bearing security assertion: with a bad signature the body must never
// be parsed. The payload here is malformed JSON, so if parsing happened first we
// would get a parse error instead of a signature error — and an attacker would
// have influenced processing before authentication.
func TestWebhookByToken_BadSignatureNeverParsesBody(t *testing.T) {
	c := newMockWebhookContainer(t)
	c.appConfigSvc.EXPECT().ResolveWebhookToken(mock.Anything, webhookTestRoutingSegment).
		Return(validTarget(), nil)

	const malformed = `{"action":`
	w := httptest.NewRecorder()
	newWebhookTestServer(t, c).ServeHTTP(w,
		webhookRequest(webhookTestRoutingSegment, malformed, signWebhook(malformed, "the-wrong-secret")))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid signature")
	assert.NotContains(t, w.Body.String(), "Invalid payload",
		"a bad signature must be reported before the body is parsed")
	// IsProcessed / HandleWebhookEvent are never EXPECTed: mockery fails the test
	// if an unverified delivery reached dedup or dispatch.
}

// An unknown token must cost one indexed lookup and nothing else — no HMAC, no
// body read. The endpoint is public and unauthenticated, so a wrong token has
// to be cheap.
func TestWebhookByToken_UnknownTokenIs404WithNoDetail(t *testing.T) {
	c := newMockWebhookContainer(t)
	c.appConfigSvc.EXPECT().ResolveWebhookToken(mock.Anything, webhookTestRoutingSegment).
		Return(nil, services.ErrGitHubAppNotConfigured)

	w := httptest.NewRecorder()
	newWebhookTestServer(t, c).ServeHTTP(w,
		webhookRequest(webhookTestRoutingSegment, webhookTestPayload,
			signWebhook(webhookTestPayload, webhookTestSecret)))

	assert.Equal(t, http.StatusNotFound, w.Code)
	body := w.Body.String()
	// The response must not reveal whether the token merely does not exist.
	assert.NotContains(t, body, webhookTestRoutingSegment)
	assert.NotContains(t, strings.ToLower(body), "token")
}

// A malformed token is rejected on charset alone, before any database work.
func TestWebhookByToken_MalformedTokenRejectedBeforeLookup(t *testing.T) {
	for _, name := range []string{"bad charset", "over length"} {
		t.Run(name, func(t *testing.T) {
			pathSegment := "has.dots.and+plus"
			if name == "over length" {
				pathSegment = strings.Repeat("a", 65)
			}

			// ResolveWebhookToken is never EXPECTed: mockery fails the test if a
			// malformed token reached the database.
			c := newMockWebhookContainer(t)

			w := httptest.NewRecorder()
			newWebhookTestServer(t, c).ServeHTTP(w,
				webhookRequest(pathSegment, webhookTestPayload, "sha256=deadbeef"))

			assert.Equal(t, http.StatusNotFound, w.Code)
		})
	}
}

func TestWebhookByToken_MissingSignatureRejected(t *testing.T) {
	c := newMockWebhookContainer(t)
	c.appConfigSvc.EXPECT().ResolveWebhookToken(mock.Anything, webhookTestRoutingSegment).
		Return(validTarget(), nil)

	w := httptest.NewRecorder()
	newWebhookTestServer(t, c).ServeHTTP(w,
		webhookRequest(webhookTestRoutingSegment, webhookTestPayload, ""))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Missing signature")
}

func TestWebhookByToken_DuplicateDeliveryIsDeduped(t *testing.T) {
	c := newMockWebhookContainer(t)
	c.appConfigSvc.EXPECT().ResolveWebhookToken(mock.Anything, webhookTestRoutingSegment).
		Return(validTarget(), nil)
	c.webhookRepo.EXPECT().IsProcessed(mock.Anything, "delivery-1").Return(true, nil)
	// HandleWebhookEvent is never EXPECTed — a replayed delivery must not
	// re-dispatch.

	w := httptest.NewRecorder()
	newWebhookTestServer(t, c).ServeHTTP(w,
		webhookRequest(webhookTestRoutingSegment, webhookTestPayload,
			signWebhook(webhookTestPayload, webhookTestSecret)))

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestWebhookByToken_OversizedBodyRejected(t *testing.T) {
	c := newMockWebhookContainer(t)
	c.appConfigSvc.EXPECT().ResolveWebhookToken(mock.Anything, webhookTestRoutingSegment).
		Return(validTarget(), nil)

	huge := `{"action":"` + strings.Repeat("x", int(maxWebhookBodyBytes)+1) + `"}`
	w := httptest.NewRecorder()
	newWebhookTestServer(t, c).ServeHTTP(w,
		webhookRequest(webhookTestRoutingSegment, huge, signWebhook(huge, webhookTestSecret)))

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// Two teams can hold the same numeric installation id (each installed its own
// App on the same org). A delivery for team A's App must dispatch with A's App
// config id, so the service resolves A's installation and never B's.
func TestWebhookByToken_DeliveryIsScopedToTheOwningApp(t *testing.T) {
	c := newMockWebhookContainer(t)
	c.appConfigSvc.EXPECT().ResolveWebhookToken(mock.Anything, "token-for-team-b").
		Return(&services.WebhookDeliveryTarget{
			AppConfigID:   "cfg-team-b",
			TeamID:        "team-b",
			WebhookSecret: "team-b-secret",
		}, nil)
	c.webhookRepo.EXPECT().IsProcessed(mock.Anything, "delivery-1").Return(false, nil)
	// Same installation id as team A would use — only the App config id differs,
	// and that is what must reach the service.
	c.githubSvc.EXPECT().
		HandleWebhookEvent(mock.Anything, "cfg-team-b", "installation", int64(4242), "created").
		Return(nil)
	c.webhookRepo.EXPECT().
		MarkProcessed(mock.Anything, "delivery-1", "installation", mock.Anything).Return(nil)

	w := httptest.NewRecorder()
	newWebhookTestServer(t, c).ServeHTTP(w,
		webhookRequest("token-for-team-b", webhookTestPayload,
			signWebhook(webhookTestPayload, "team-b-secret")))

	assert.Equal(t, http.StatusOK, w.Code)
}

// An App config that somehow stored no webhook secret must reject everything,
// not accept anything.
func TestWebhookByToken_EmptySecretRejectsAll(t *testing.T) {
	c := newMockWebhookContainer(t)
	c.appConfigSvc.EXPECT().ResolveWebhookToken(mock.Anything, webhookTestRoutingSegment).
		Return(&services.WebhookDeliveryTarget{
			AppConfigID: webhookTestAppCfg, TeamID: webhookTestTeamID, WebhookSecret: "",
		}, nil)

	w := httptest.NewRecorder()
	newWebhookTestServer(t, c).ServeHTTP(w,
		webhookRequest(webhookTestRoutingSegment, webhookTestPayload, signWebhook(webhookTestPayload, "")))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// The routing token grants the ability to submit signature-checked deliveries,
// so it is a credential sitting in a URL path. Access logs are widely readable
// and long-lived — exactly where it must not land.
func TestRedactSensitivePath(t *testing.T) {
	assert.Equal(t, "/api/v1/webhooks/github/[redacted]",
		apierrors.RedactSensitivePath("/api/v1/webhooks/github/"+webhookTestRoutingSegment))
	assert.NotContains(t,
		apierrors.RedactSensitivePath("/api/v1/webhooks/github/"+webhookTestRoutingSegment), webhookTestRoutingSegment)

	// Everything else is logged verbatim; redaction must not blunt the logs.
	for _, path := range []string{
		"/api/v1/webhooks/github",
		"/api/v1/team-1/prompts",
		"/healthz",
	} {
		assert.Equal(t, path, apierrors.RedactSensitivePath(path))
	}
}

func TestVerifyGitHubSignature(t *testing.T) {
	body := []byte(webhookTestPayload)

	assert.True(t, verifyGitHubSignature(body,
		signWebhook(webhookTestPayload, webhookTestSecret), webhookTestSecret))
	assert.False(t, verifyGitHubSignature(body,
		signWebhook(webhookTestPayload, "other-secret"), webhookTestSecret))
	assert.False(t, verifyGitHubSignature(body, "", webhookTestSecret))
	// A signature without the algorithm prefix must not be compared raw.
	assert.False(t, verifyGitHubSignature(body,
		strings.TrimPrefix(signWebhook(webhookTestPayload, webhookTestSecret), "sha256="),
		webhookTestSecret))
	// An empty stored secret rejects even a signature computed with one.
	assert.False(t, verifyGitHubSignature(body, signWebhook(webhookTestPayload, ""), ""))
}
