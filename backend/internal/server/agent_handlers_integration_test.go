package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// MockAgentContainer implements Container interface for agent handler tests
type MockAgentContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	agentService           *svcmocks.MockAgentServiceInterface
	resourceUsageService   *MockResourceUsageServiceForHandlers
	agentInvocationService *svcmocks.MockAgentInvocationServiceInterface
	authService            *svcmocks.MockAuthServiceInterface
	teamService            *svcmocks.MockTeamServiceInterface
}

func (m *MockAgentContainer) AgentService() services.AgentServiceInterface {
	return m.agentService
}

func (m *MockAgentContainer) AuthService() services.AuthServiceInterface {
	return m.authService
}

func (m *MockAgentContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return m.resourceUsageService
}

func (m *MockAgentContainer) AgentInvocationService() services.AgentInvocationServiceInterface {
	return m.agentInvocationService
}

func (m *MockAgentContainer) TeamService() services.TeamServiceInterface {
	return m.teamService
}

func newMockAgentContainer(t *testing.T) *MockAgentContainer {
	return &MockAgentContainer{
		agentService:           svcmocks.NewMockAgentServiceInterface(t),
		resourceUsageService:   &MockResourceUsageServiceForHandlers{},
		agentInvocationService: svcmocks.NewMockAgentInvocationServiceInterface(t),
		authService:            svcmocks.NewMockAuthServiceInterface(t),
		teamService:            svcmocks.NewMockTeamServiceInterface(t),
	}
}

// setupDefaultTeamMock sets up the auth service mock to return a user with a default team
func setupDefaultTeamMock(mockContainer *MockAgentContainer, userID, teamID string) {
	defaultTeamID := teamID
	mockContainer.authService.On("GetUserByID", mock.Anything, userID).
		Return(&models.User{
			ID:            userID,
			DefaultTeamID: &defaultTeamID,
		}, nil).Maybe()
	mockContainer.teamService.On("IsUserMemberOfTeam", mock.Anything, userID, teamID).
		Return(true, nil).Maybe()
	mockContainer.teamService.On("GetTeam", mock.Anything, userID, teamID).
		Return(&models.Team{
			ID:   teamID,
			Name: "Test Team",
		}, nil).Maybe()
}

func createTestAgentServer(container *MockAgentContainer) *Server {
	cfg := &config.Config{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during test

	// Initialize router manually for testing
	r := chi.NewRouter()

	srv := &Server{
		port:      "8080",
		container: container,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	// Register routes manually (simplified version for testing)
	r.Route("/api/v1/{team_id}/agents", func(r chi.Router) {
		r.Get("/", srv.handleListAgents)
		r.Post("/", srv.handleCreateAgent)
		r.Post("/preview-card", srv.handlePreviewAgentCard)
		r.Get("/stats", srv.handleGetAgentStats)
		r.Get("/{id}", srv.handleGetAgent)
		r.Put("/{id}", srv.handleUpdateAgent)
		r.Delete("/{id}", srv.handleDeleteAgent)
		r.Post("/{id}/executions", srv.handleStartAgentExecution)
		r.Get("/{id}/execute", srv.handleExecuteAgent)
		r.Post("/{id}/execute", srv.handleExecuteAgent)
		r.Get("/{id}/executions", srv.handleListAgentExecutions)
		r.Get("/{id}/conversations", srv.handleListAgentConversations)
		r.Put("/{id}/credentials", srv.handleUpdateAgentCredentials)
		r.Put("/executions/{execution_id}", srv.handleCompleteAgentExecution)
		r.Get("/executions/{execution_id}", srv.handleGetAgentExecution)
		r.Get("/executions/{id}/status", srv.handleGetExecutionStatus)
		r.Get("/executions/{id}/events", srv.handleGetExecutionEvents)
		r.Get("/conversations/{conversation_id}/executions", srv.handleGetConversationExecutions)
	})

	return srv
}

func makeAgentAuthenticatedRequest(method, path string, body interface{}, userID string) *http.Request {
	var reqBody []byte
	var err error

	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			panic(err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, userID))

	return req
}

// Helper function to convert string to *string
func stringPtr(s string) *string {
	return &s
}

// Helper function to convert time.Time to *time.Time
func timePtr(t time.Time) *time.Time {
	return &t
}

// TestHandleListAgents_Success tests successful list agents with mocked service
func TestHandleListAgents_Success(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	teamID := "550e8400-e29b-41d4-a716-446655440000"

	// Mock expectations
	mockContainer.agentService.On(
		"ListAgents",
		mock.Anything,
		"user-123",
		mock.MatchedBy(func(filters services.AgentFilters) bool {
			return filters.TeamID == teamID && filters.Page == 1 && filters.Limit == 10
		}),
	).Return(&models.AgentListResponse{
		Agents: []models.Agent{
			{
				ID:          "agent-1",
				Name:        "Test Agent",
				Description: "A test agent",
				CardURL:     stringPtr("http://example.com/agent.json"),
				Status:      "active",
				UserID:      "user-123",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		},
		TotalCount: 1,
		Page:       1,
		PerPage:    10,
		TotalPages: 1,
	}, nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("GET", "/api/v1/"+teamID+"/agents", nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.AgentListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, 1, response.TotalCount)
	assert.Equal(t, 1, response.Page)
	assert.Equal(t, 10, response.PerPage)
	assert.Len(t, response.Agents, 1)

	agent := response.Agents[0]
	assert.Equal(t, "agent-1", agent.ID)
	assert.Equal(t, "Test Agent", agent.Name)
	assert.Equal(t, "active", agent.Status)

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleListAgents_WithFilters tests list agents with filters
func TestHandleListAgents_WithFilters(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	mockContainer.agentService.On(
		"ListAgents",
		mock.Anything,
		"user-123",
		mock.MatchedBy(func(filters services.AgentFilters) bool {
			return filters.Status == "active" && filters.Search == "test" &&
				filters.Page == 2 && filters.Limit == 5 && filters.TeamID == teamID
		}),
	).Return(&models.AgentListResponse{
		Agents:     []models.Agent{},
		TotalCount: 0,
		Page:       2,
		PerPage:    5,
		TotalPages: 0,
	}, nil)

	srv := createTestAgentServer(mockContainer)
	url := "/api/v1/" + teamID + "/agents?status=active&search=test&page=2&limit=5"
	req := makeAgentAuthenticatedRequest("GET", url, nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.AgentListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, 0, response.TotalCount)
	assert.Equal(t, 2, response.Page)

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleListAgents_ServiceError tests list agents when service returns error
func TestHandleListAgents_ServiceError(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	// Use a different user ID to demonstrate parameter variation
	userID := "user-456"
	teamID := "550e8400-e29b-41d4-a716-446655440456"
	setupDefaultTeamMock(mockContainer, userID, teamID)

	mockContainer.agentService.On("ListAgents", mock.Anything, userID, mock.Anything).
		Return((*models.AgentListResponse)(nil), errors.New("database error"))

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("GET", "/api/v1/"+teamID+"/agents", nil, userID)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "INTERNAL_ERROR", response["code"])
	assert.Equal(t, "Failed to list agents", response["detail"])

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleGetAgent_Success tests successful get agent by ID
func TestHandleGetAgent_Success(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	expectedAgent := &models.Agent{
		ID:          "agent-1",
		Name:        "Test Agent",
		Description: "A test agent",
		CardURL:     stringPtr("http://example.com/agent.json"),
		Status:      "active",
		UserID:      "user-123",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockContainer.agentService.On("GetAgentByID", mock.Anything, "user-123", teamID, "agent-1").
		Return(expectedAgent, nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("GET", "/api/v1/"+teamID+"/agents/agent-1", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.Agent
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedAgent.ID, response.ID)
	assert.Equal(t, expectedAgent.Name, response.Name)
	assert.Equal(t, expectedAgent.Status, response.Status)

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleGetAgent_NotFound tests get agent when agent not found
func TestHandleGetAgent_NotFound(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	// Production shape: AgentService.GetAgentByID wraps the repository
	// sentinel ("failed to get agent: %w"); the old string-equality handler
	// only matched the bare sentinel text, so this shape pins errors.Is.
	mockContainer.agentService.On("GetAgentByID", mock.Anything, "user-123", teamID, "non-existent").
		Return((*models.Agent)(nil), fmt.Errorf("failed to get agent: %w", repositories.ErrAgentNotFound))

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("GET", "/api/v1/"+teamID+"/agents/non-existent", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])
	assert.Equal(t, "Agent not found", response["detail"])

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleCreateAgent_Success tests successful agent creation
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestHandleCreateAgent_Success(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	reqBody := &models.CreateAgentRequest{
		Name:        "New Agent",
		Description: "A new agent",
		CardURL:     "http://example.com/agent.json",
		Status:      "active",
	}

	expectedAgent := &models.Agent{
		ID:          "agent-new",
		Name:        reqBody.Name,
		Description: reqBody.Description,
		CardURL:     stringPtr(reqBody.CardURL),
		Status:      reqBody.Status,
		UserID:      "user-123",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Mock resource limit check
	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "user-123", "agent").
		Return(true, nil)

	// Mock getUserDefaultTeamID - setup default team mock
	setupDefaultTeamMock(mockContainer, "user-123", "team-123")

	mockContainer.agentService.On(
		"CreateAgent",
		mock.Anything,
		"user-123",
		"team-123",
		mock.MatchedBy(func(req *models.CreateAgentRequest) bool {
			return req.Name == "New Agent" && req.CardURL == "http://example.com/agent.json"
		}),
	).Return(expectedAgent, nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("POST", "/api/v1/team-123/agents", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response models.Agent
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedAgent.ID, response.ID)
	assert.Equal(t, expectedAgent.Name, response.Name)
	assert.Equal(t, expectedAgent.CardURL, response.CardURL)

	mockContainer.agentService.AssertExpectations(t)
	mockContainer.resourceUsageService.AssertExpectations(t)
	mockContainer.authService.AssertExpectations(t)
}

// TestHandleCreateAgent_NameConflict guards the create-side 409 path now keyed on
// errors.Is(ErrAgentNameConflict) instead of a substring match (#1704). The mock
// returns the service-wrapped error production emits.
func TestHandleCreateAgent_NameConflict(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	reqBody := &models.CreateAgentRequest{
		Name:    "Duplicate Agent",
		CardURL: "http://example.com/agent.json",
		Status:  "active",
	}

	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "user-123", "agent").
		Return(true, nil)
	setupDefaultTeamMock(mockContainer, "user-123", "team-123")
	mockContainer.agentService.On("CreateAgent", mock.Anything, "user-123", "team-123", mock.Anything).
		Return((*models.Agent)(nil), fmt.Errorf("failed to create agent: %w", repositories.ErrAgentNameConflict))

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("POST", "/api/v1/team-123/agents", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusConflict, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "RESOURCE_EXISTS", response["code"])

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleCreateAgent_ValidationError tests create agent with validation errors
func TestHandleCreateAgent_ValidationError(t *testing.T) {
	tests := []struct {
		name          string
		reqBody       *models.CreateAgentRequest
		expectedError string
	}{
		{
			name: "Missing card_url",
			reqBody: &models.CreateAgentRequest{
				Name: "Test Agent",
			},
			expectedError: "Agent card URL is required",
		},
		{
			name: "Empty card_url",
			reqBody: &models.CreateAgentRequest{
				Name:    "Test Agent",
				CardURL: "",
			},
			expectedError: "Agent card URL is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockAgentContainer(t)
			setupDefaultTeamMock(mockContainer, "user-123", "team-123")
			srv := createTestAgentServer(mockContainer)

			req := makeAgentAuthenticatedRequest("POST", "/api/v1/team-123/agents", tt.reqBody, "user-123")
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)
			specconformance.AssertConformsToSpec(t, req, w)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			// RFC 9457 error format
			assert.Equal(t, "VALIDATION_FAILED", response["code"])
			assert.Equal(t, 400.0, response["status"])
			assert.Contains(t, response["detail"], tt.expectedError)
			assert.NotEmpty(t, response["timestamp"])
			// request_id is present (may be empty if middleware not invoked in test)
			assert.Contains(t, response, "request_id")
		})
	}
}

// TestHandleCreateAgent_ResourceLimitExceeded tests create agent when resource limit exceeded
func TestHandleCreateAgent_ResourceLimitExceeded(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	reqBody := &models.CreateAgentRequest{
		Name:    "New Agent",
		CardURL: "http://example.com/agent.json",
	}

	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "user-123", "agent").
		Return(false, nil)

	setupDefaultTeamMock(mockContainer, "user-123", "team-123")

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("POST", "/api/v1/team-123/agents", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_LIMIT_EXCEEDED", response["code"])

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestHandleUpdateAgent_Success tests successful agent update
func TestHandleUpdateAgent_Success(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	newName := "Updated Agent"
	newStatus := "paused"
	reqBody := &models.UpdateAgentRequest{
		Name:   &newName,
		Status: &newStatus,
	}

	updatedAgent := &models.Agent{
		ID:          "agent-1",
		Name:        newName,
		Description: "A test agent",
		CardURL:     stringPtr("http://example.com/agent.json"),
		Status:      newStatus,
		UserID:      "user-123",
		CreatedAt:   time.Now().Add(-24 * time.Hour),
		UpdatedAt:   time.Now(),
	}

	mockContainer.agentService.On("UpdateAgent", mock.Anything, "user-123", teamID, "agent-1", mock.Anything).
		Return(updatedAgent, nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("PUT", "/api/v1/"+teamID+"/agents/agent-1", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.Agent
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, updatedAgent.Name, response.Name)
	assert.Equal(t, updatedAgent.Status, response.Status)

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleUpdateAgent_EmptyDescription is the regression guard for #1704
// item 1: an empty description used to write two stacked 400 bodies, then
// let the update proceed and append a third 200 body to the committed 400
// response. It must produce exactly one 400 document and never reach the
// service (the strict mock fails on an unexpected UpdateAgent call).
func TestHandleUpdateAgent_EmptyDescription(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest(
		"PUT", "/api/v1/"+teamID+"/agents/agent-1",
		map[string]any{"description": ""}, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	dec := json.NewDecoder(bytes.NewReader(w.Body.Bytes()))
	var first map[string]any
	assert.NoError(t, dec.Decode(&first), "400 body must be valid JSON")
	assert.False(t, dec.More(), "response must contain exactly one JSON document, got stacked bodies")
}

// TestHandleUpdateAgent_NotFound tests update non-existent agent
func TestHandleUpdateAgent_NotFound(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	newName := "Updated Agent"
	reqBody := &models.UpdateAgentRequest{
		Name: &newName,
	}

	// Production shape: AgentService.UpdateAgent's not-found path comes from
	// its initial GetByID, wrapped as "failed to get agent: %w".
	mockContainer.agentService.On("UpdateAgent", mock.Anything, "user-123", teamID, "non-existent", mock.Anything).
		Return((*models.Agent)(nil), fmt.Errorf("failed to get agent: %w", repositories.ErrAgentNotFound))

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("PUT", "/api/v1/"+teamID+"/agents/non-existent", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleUpdateAgent_NameConflict is the #1704 regression guard for the 409
// branch: a duplicate-name update must return 409, not 500. The mock returns the
// SERVICE-WRAPPED error ("failed to update agent: %w") the way production does —
// the old exact-string compare against the unwrapped message never matched, so
// the handler fell through to 500. errors.Is sees through the wrapping.
func TestHandleUpdateAgent_NameConflict(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	newName := "Duplicate Agent"
	reqBody := &models.UpdateAgentRequest{Name: &newName}

	mockContainer.agentService.On("UpdateAgent", mock.Anything, "user-123", teamID, "agent-1", mock.Anything).
		Return((*models.Agent)(nil), fmt.Errorf("failed to update agent: %w", repositories.ErrAgentNameConflict))

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("PUT", "/api/v1/"+teamID+"/agents/agent-1", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusConflict, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "RESOURCE_EXISTS", response["code"])

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleDeleteAgent_Success tests successful agent deletion
func TestHandleDeleteAgent_Success(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	existingAgent := &models.Agent{
		ID:          "agent-1",
		Name:        "Test Agent",
		Description: "A test agent",
		CardURL:     stringPtr("http://example.com/agent.json"),
		Status:      "active",
		UserID:      "user-123",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockContainer.agentService.On("GetAgentByID", mock.Anything, "user-123", teamID, "agent-1").
		Return(existingAgent, nil)

	mockContainer.agentService.On("DeleteAgent", mock.Anything, "user-123", teamID, "agent-1").
		Return(nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("DELETE", "/api/v1/"+teamID+"/agents/agent-1", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNoContent, w.Code)

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleDeleteAgent_NotFound tests delete non-existent agent
func TestHandleDeleteAgent_NotFound(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	// Production shape: the delete handler resolves the agent via
	// AgentService.GetAgentByID, which wraps as "failed to get agent: %w".
	mockContainer.agentService.On("GetAgentByID", mock.Anything, "user-123", teamID, "non-existent").
		Return((*models.Agent)(nil), fmt.Errorf("failed to get agent: %w", repositories.ErrAgentNotFound))

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("DELETE", "/api/v1/"+teamID+"/agents/non-existent", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleGetAgentStats_Success tests successful get agent stats
func TestHandleGetAgentStats_Success(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	expectedStats := &models.AgentStatsResponse{
		TotalAgents:  10,
		ActiveAgents: 7,
		PausedAgents: 2,
		ErrorAgents:  1,
		TotalRuns:    150,
	}

	mockContainer.agentService.On("GetAgentStats", mock.Anything, "user-123", teamID).
		Return(expectedStats, nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("GET", "/api/v1/"+teamID+"/agents/stats", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.AgentStatsResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedStats.TotalAgents, response.TotalAgents)
	assert.Equal(t, expectedStats.ActiveAgents, response.ActiveAgents)
	assert.Equal(t, expectedStats.PausedAgents, response.PausedAgents)
	assert.Equal(t, expectedStats.ErrorAgents, response.ErrorAgents)
	assert.Equal(t, expectedStats.TotalRuns, response.TotalRuns)

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleGetAgentStats_ServiceError tests get agent stats when service returns error
func TestHandleGetAgentStats_ServiceError(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	mockContainer.agentService.On("GetAgentStats", mock.Anything, "user-123", teamID).
		Return((*models.AgentStatsResponse)(nil), errors.New("database error"))

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("GET", "/api/v1/"+teamID+"/agents/stats", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "INTERNAL_ERROR", response["code"])
	assert.Equal(t, "Failed to get agent stats", response["detail"])

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleStartAgentExecution_Success tests successful agent execution start
func TestHandleStartAgentExecution_Success(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	reqBody := &models.CreateAgentExecutionRequest{
		Input: map[string]interface{}{
			"task": "test task",
		},
	}

	expectedExecution := &models.AgentExecution{
		ID:        "execution-1",
		AgentID:   "agent-1",
		UserID:    "user-123",
		Status:    "running",
		Input:     reqBody.Input,
		StartedAt: time.Now(),
		EndedAt:   timePtr(time.Now()),
	}

	mockContainer.agentService.On(
		"StartExecution",
		mock.Anything,
		"user-123",
		teamID,
		"agent-1",
		mock.MatchedBy(func(req *models.CreateAgentExecutionRequest) bool {
			return req.AgentID == "agent-1"
		}),
	).Return(expectedExecution, nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("POST", "/api/v1/"+teamID+"/agents/agent-1/executions", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response models.AgentExecution
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedExecution.ID, response.ID)
	assert.Equal(t, expectedExecution.AgentID, response.AgentID)
	assert.Equal(t, expectedExecution.Status, response.Status)

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleStartAgentExecution_AgentNotFound tests start execution when agent not found
func TestHandleStartAgentExecution_AgentNotFound(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	reqBody := &models.CreateAgentExecutionRequest{
		Input: map[string]interface{}{},
	}

	mockContainer.agentService.On("StartExecution", mock.Anything, "user-123", teamID, "non-existent", mock.Anything).
		Return((*models.AgentExecution)(nil), fmt.Errorf("failed to get agent: %w", repositories.ErrAgentNotFound))

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("POST", "/api/v1/"+teamID+"/agents/non-existent/executions", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])
	assert.Equal(t, "Agent not found", response["detail"])

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleCompleteAgentExecution_Success tests successful agent execution completion
func TestHandleCompleteAgentExecution_Success(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	reqBody := &models.UpdateAgentExecutionRequest{
		Status: "success",
		Output: map[string]interface{}{
			"result": "completed successfully",
		},
	}

	expectedExecution := &models.AgentExecution{
		ID:      "execution-1",
		AgentID: "agent-1",
		UserID:  "user-123",
		Status:  "success",
		// Output handled via task completion,
		StartedAt: time.Now().Add(-5 * time.Minute),
		EndedAt:   timePtr(time.Now()),
	}

	mockContainer.agentService.On(
		"CompleteExecution",
		mock.Anything,
		"user-123",
		"execution-1",
		mock.MatchedBy(func(req *models.UpdateAgentExecutionRequest) bool {
			return req.Status == "success"
		}),
	).Return(expectedExecution, nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("PUT", "/api/v1/"+teamID+"/agents/executions/execution-1", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.AgentExecution
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedExecution.ID, response.ID)
	assert.Equal(t, expectedExecution.Status, response.Status)

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleCompleteAgentExecution_WithError tests execution completion with error status
func TestHandleCompleteAgentExecution_WithError(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	errorMsg := "execution failed"
	reqBody := &models.UpdateAgentExecutionRequest{
		Status: "error",
		Error:  &errorMsg,
	}

	expectedExecution := &models.AgentExecution{
		ID:        "execution-1",
		AgentID:   "agent-1",
		UserID:    "user-123",
		Status:    "error",
		Error:     &errorMsg,
		StartedAt: time.Now().Add(-5 * time.Minute),
		EndedAt:   timePtr(time.Now()),
	}

	mockContainer.agentService.On(
		"CompleteExecution",
		mock.Anything,
		"user-123",
		"execution-1",
		mock.MatchedBy(func(req *models.UpdateAgentExecutionRequest) bool {
			return req.Status == "error"
		}),
	).Return(expectedExecution, nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("PUT", "/api/v1/"+teamID+"/agents/executions/execution-1", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.AgentExecution
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedExecution.Status, response.Status)
	assert.NotNil(t, response.Error)

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleCompleteAgentExecution_ValidationError tests validation errors
func TestHandleCompleteAgentExecution_ValidationError(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	reqBody := &models.UpdateAgentExecutionRequest{
		Status: "invalid_status",
	}

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("PUT", "/api/v1/"+teamID+"/agents/executions/execution-1", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "VALIDATION_FAILED", response["code"])
}

// TestHandleCompleteAgentExecution_NotFound tests complete execution when execution not found
func TestHandleCompleteAgentExecution_NotFound(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	reqBody := &models.UpdateAgentExecutionRequest{
		Status: "success",
	}

	mockContainer.agentService.On("CompleteExecution", mock.Anything, "user-123", "non-existent", mock.Anything).
		Return(
			(*models.AgentExecution)(nil),
			fmt.Errorf("failed to get execution: %w", repositories.ErrAgentExecutionNotFound),
		)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("PUT", "/api/v1/"+teamID+"/agents/executions/non-existent", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])
	assert.Equal(t, "Execution not found", response["detail"])

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleGetAgentExecution_Success tests successful get agent execution
func TestHandleGetAgentExecution_Success(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	expectedExecution := &models.AgentExecution{
		ID:        "execution-1",
		AgentID:   "agent-1",
		UserID:    "user-123",
		Status:    "success",
		Input:     map[string]interface{}{"task": "test"},
		StartedAt: time.Now().Add(-10 * time.Minute),
		EndedAt:   timePtr(time.Now()),
	}

	mockContainer.agentService.On("GetExecution", mock.Anything, "user-123", "execution-1").
		Return(expectedExecution, nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("GET", "/api/v1/"+teamID+"/agents/executions/execution-1", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.AgentExecution
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedExecution.ID, response.ID)
	assert.Equal(t, expectedExecution.AgentID, response.AgentID)
	assert.Equal(t, expectedExecution.Status, response.Status)

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleGetAgentExecution_NotFound tests get execution when execution not found
func TestHandleGetAgentExecution_NotFound(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	setupDefaultTeamMock(mockContainer, "user-123", teamID)

	mockContainer.agentService.On("GetExecution", mock.Anything, "user-123", "non-existent").
		Return(
			(*models.AgentExecution)(nil),
			fmt.Errorf("failed to get execution: %w", repositories.ErrAgentExecutionNotFound),
		)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("GET", "/api/v1/"+teamID+"/agents/executions/non-existent", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])
	assert.Equal(t, "Execution not found", response["detail"])

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleListAgents_SortBy tests list agents with valid sort_by parameters
func TestHandleListAgents_SortBy(t *testing.T) {
	validSortFields := []string{"name", "status", "updated_at", "created_at", "last_run", "success_rate"}

	for _, sortField := range validSortFields {
		t.Run("sort_by="+sortField, func(t *testing.T) {
			mockContainer := newMockAgentContainer(t)
			teamID := "550e8400-e29b-41d4-a716-446655440000"

			mockContainer.agentService.On(
				"ListAgents",
				mock.Anything,
				"user-123",
				mock.MatchedBy(func(filters services.AgentFilters) bool {
					return filters.SortBy == sortField && filters.SortOrder == "asc" && filters.TeamID == teamID
				}),
			).Return(&models.AgentListResponse{
				Agents:     []models.Agent{},
				TotalCount: 0,
				Page:       1,
				PerPage:    10,
				TotalPages: 0,
			}, nil)

			srv := createTestAgentServer(mockContainer)
			url := "/api/v1/" + teamID + "/agents?sort_by=" + sortField + "&sort_order=asc"
			req := makeAgentAuthenticatedRequest("GET", url, nil, "user-123")
			w := httptest.NewRecorder()

			srv.router.ServeHTTP(w, req)
			specconformance.AssertConformsToSpec(t, req, w)

			assert.Equal(t, http.StatusOK, w.Code, "sort_by=%s should return 200", sortField)
			mockContainer.agentService.AssertExpectations(t)
		})
	}
}

// TestHandleListAgents_InvalidSortBy tests list agents with invalid sort_by returns 400
func TestHandleListAgents_InvalidSortBy(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"

	srv := createTestAgentServer(mockContainer)
	url := "/api/v1/" + teamID + "/agents?sort_by=invalid_column"
	req := makeAgentAuthenticatedRequest("GET", url, nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response["detail"], "invalid sort_by value")
}

// TestHandleListAgents_DefaultSort tests list agents with no sort params uses default ordering
func TestHandleListAgents_DefaultSort(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"

	mockContainer.agentService.On(
		"ListAgents",
		mock.Anything,
		"user-123",
		mock.MatchedBy(func(filters services.AgentFilters) bool {
			return filters.SortBy == "" && filters.SortOrder == "" && filters.TeamID == teamID
		}),
	).Return(&models.AgentListResponse{
		Agents:     []models.Agent{},
		TotalCount: 0,
		Page:       1,
		PerPage:    10,
		TotalPages: 0,
	}, nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest("GET", "/api/v1/"+teamID+"/agents", nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)
	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleListAgents_SortOrderDesc tests list agents with desc sort order
func TestHandleListAgents_SortOrderDesc(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"

	mockContainer.agentService.On(
		"ListAgents",
		mock.Anything,
		"user-123",
		mock.MatchedBy(func(filters services.AgentFilters) bool {
			return filters.SortBy == "name" && filters.SortOrder == "desc" && filters.TeamID == teamID
		}),
	).Return(&models.AgentListResponse{
		Agents:     []models.Agent{},
		TotalCount: 0,
		Page:       1,
		PerPage:    10,
		TotalPages: 0,
	}, nil)

	srv := createTestAgentServer(mockContainer)
	url := "/api/v1/" + teamID + "/agents?sort_by=name&sort_order=desc"
	req := makeAgentAuthenticatedRequest("GET", url, nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)
	mockContainer.agentService.AssertExpectations(t)
}
