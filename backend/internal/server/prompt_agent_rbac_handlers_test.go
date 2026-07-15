package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// The prompt and agent handlers must turn services.ErrPermissionDenied into a 403.
//
// This is the bug class that already shipped twice in this epic (#222/PR #233 and
// #235/PR #239): these handlers map errors by matching substrings of the message,
// and ErrPermissionDenied's text matches none of their branches — so without an
// errors.Is branch ordered before that chain, a denial renders as a 500. The
// service-level RBAC tests cannot see this; only a request through the handler can.

const (
	praUserID = "user-caller"
	praTeamID = "team-123"
)

// MockPromptAgentContainer overrides only the prompt and agent services.
type MockPromptAgentContainer struct {
	BaseMockContainer
	promptService        services.PromptServiceInterface
	agentService         services.AgentServiceInterface
	resourceUsageService services.ResourceUsageServiceInterface
}

func (m *MockPromptAgentContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return m.resourceUsageService
}

func (m *MockPromptAgentContainer) PromptService() services.PromptServiceInterface {
	return m.promptService
}

func (m *MockPromptAgentContainer) AgentService() services.AgentServiceInterface {
	return m.agentService
}

// createTestPromptAgentServer wires the mutating prompt/agent routes. The team
// validation middleware is deliberately not mounted: it enforces tenancy, while
// the role decision under test happens in the service.
func createTestPromptAgentServer(
	t *testing.T, prompt services.PromptServiceInterface, agent services.AgentServiceInterface,
) *Server {
	t.Helper()

	// Both create paths check a resource limit before reaching the service.
	usage := svcmocks.NewMockResourceUsageServiceInterface(t)
	usage.EXPECT().CheckResourceLimit(mock.Anything, mock.Anything, mock.Anything).
		Return(true, nil).Maybe()

	r := chi.NewRouter()
	srv := &Server{
		port: "8080",
		container: &MockPromptAgentContainer{
			promptService: prompt, agentService: agent, resourceUsageService: usage,
		},
		logger: slog.New(slog.DiscardHandler),
		config: &config.Config{},
		router: r,
	}

	r.Route("/api/v1/{team_id}/prompts", func(r chi.Router) {
		r.Post("/", srv.handleCreatePrompt)
		r.Put("/{slug}", srv.handleUpdatePrompt)
	})
	r.Route("/api/v1/{team_id}/agents", func(r chi.Router) {
		r.Post("/", srv.handleCreateAgent)
		r.Put("/{id}", srv.handleUpdateAgent)
		r.Delete("/{id}", srv.handleDeleteAgent)
	})

	return srv
}

func praRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, praUserID))
}

// assertForbidden pins the full RFC 9457 contract, not just the status.
func assertForbidden(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusForbidden, w.Code, "denial must be 403, not 500: %s", w.Body.String())
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), `"code":"FORBIDDEN"`)
}

func TestPromptHandlers_PermissionDeniedIsForbidden(t *testing.T) {
	base := "/api/v1/" + praTeamID + "/prompts"

	t.Run("create", func(t *testing.T) {
		prompt := svcmocks.NewMockPromptServiceInterface(t)
		prompt.EXPECT().CreatePrompt(praUserID, praTeamID, mock.Anything).
			Return(nil, services.ErrPermissionDenied).Once()

		srv := createTestPromptAgentServer(t, prompt, svcmocks.NewMockAgentServiceInterface(t))
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, praRequest(http.MethodPost, base, `{"name":"P","slug":"p","body":"b","project_id":"proj-1"}`))

		assertForbidden(t, w)
	})

	t.Run("update", func(t *testing.T) {
		prompt := svcmocks.NewMockPromptServiceInterface(t)
		prompt.EXPECT().UpdatePromptBySlug(praUserID, praTeamID, "p", mock.Anything).
			Return(nil, services.ErrPermissionDenied).Once()

		srv := createTestPromptAgentServer(t, prompt, svcmocks.NewMockAgentServiceInterface(t))
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, praRequest(http.MethodPut, base+"/p", `{"body":"edited"}`))

		assertForbidden(t, w)
	})
}

func TestAgentHandlers_PermissionDeniedIsForbidden(t *testing.T) {
	base := "/api/v1/" + praTeamID + "/agents"

	t.Run("create", func(t *testing.T) {
		agent := svcmocks.NewMockAgentServiceInterface(t)
		agent.EXPECT().CreateAgent(mock.Anything, praUserID, praTeamID, mock.Anything).
			Return(nil, services.ErrPermissionDenied).Once()

		srv := createTestPromptAgentServer(t, svcmocks.NewMockPromptServiceInterface(t), agent)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, praRequest(http.MethodPost, base,
			`{"name":"A","card_url":"https://example.com/card"}`))

		assertForbidden(t, w)
	})

	t.Run("update", func(t *testing.T) {
		agent := svcmocks.NewMockAgentServiceInterface(t)
		agent.EXPECT().UpdateAgent(mock.Anything, praUserID, praTeamID, "agent-1", mock.Anything).
			Return(nil, services.ErrPermissionDenied).Once()

		srv := createTestPromptAgentServer(t, svcmocks.NewMockPromptServiceInterface(t), agent)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, praRequest(http.MethodPut, base+"/agent-1", `{"name":"renamed"}`))

		assertForbidden(t, w)
	})

	// Delete is the own-vs-any case: the handler loads the agent first, so the
	// denial arrives from the second call and must not be mistaken for not-found.
	t.Run("delete", func(t *testing.T) {
		agent := svcmocks.NewMockAgentServiceInterface(t)
		agent.EXPECT().GetAgentByID(mock.Anything, praUserID, praTeamID, "agent-1").
			Return(&models.Agent{ID: "agent-1", UserID: "user-other", TeamID: praTeamID}, nil).Once()
		agent.EXPECT().DeleteAgent(mock.Anything, praUserID, praTeamID, "agent-1").
			Return(services.ErrPermissionDenied).Once()

		srv := createTestPromptAgentServer(t, svcmocks.NewMockPromptServiceInterface(t), agent)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, praRequest(http.MethodDelete, base+"/agent-1", ""))

		assertForbidden(t, w)
	})
}
