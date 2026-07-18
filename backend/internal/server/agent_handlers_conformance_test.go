package server

// Behavioral spec-conformance tests for the four agent operations cleared from
// the payload-coverage ledger (#1714, issue #361): list executions, execute,
// preview-card, and update credentials. Every test drives the real router,
// asserts observable behavior (status, body fields, service arguments), and
// validates the recorded response against openapi.yaml right after the status
// assertion via specconformance.AssertConformsToSpec.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const conformanceTeamID = "550e8400-e29b-41d4-a716-446655440000"

// makeAgentRawRequest builds an authenticated request from a raw body string,
// for malformed-JSON paths where a marshaled struct cannot express the input.
func makeAgentRawRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))
}

// --- GET /api/v1/{team_id}/agents/{id}/executions ---

func TestHandleListAgentExecutions_Success(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	endedAt := time.Now()

	executions := []models.AgentExecution{
		{
			ID: "exec-1", AgentID: "agent-1", UserID: "user-123", Status: "success",
			Input: map[string]interface{}{"task": "first"}, StartedAt: time.Now().Add(-time.Hour),
			EndedAt: &endedAt, Version: 1,
		},
		{
			ID: "exec-2", AgentID: "agent-1", UserID: "user-123", Status: "running",
			StartedAt: time.Now().Add(-time.Minute), Version: 1,
		},
	}

	// Defaults must reach the service: page 1, limit 10, agent scoped, no filters.
	mockContainer.agentService.On(
		"ListExecutions", mock.Anything, "user-123",
		mock.MatchedBy(func(f services.AgentExecutionFilters) bool {
			return f.AgentID != nil && *f.AgentID == "agent-1" &&
				f.Status == nil && f.DateFrom == nil && f.DateTo == nil &&
				f.Page == 1 && f.Limit == 10
		}),
	).Return(executions, 12, nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest(
		"GET", "/api/v1/"+conformanceTeamID+"/agents/agent-1/executions", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, float64(12), response["total_count"])
	assert.Equal(t, float64(1), response["page"])
	assert.Equal(t, float64(10), response["per_page"])
	assert.Equal(t, float64(2), response["total_pages"]) // ceil(12/10)
	assert.Len(t, response["executions"].([]interface{}), 2)

	mockContainer.agentService.AssertExpectations(t)
}

func TestHandleListAgentExecutions_FilterAndPaginationParams(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	mockContainer.agentService.On(
		"ListExecutions", mock.Anything, "user-123",
		mock.MatchedBy(func(f services.AgentExecutionFilters) bool {
			return f.AgentID != nil && *f.AgentID == "agent-1" &&
				f.Status != nil && *f.Status == "error" &&
				f.DateFrom != nil && *f.DateFrom == "2026-01-01" &&
				f.DateTo != nil && *f.DateTo == "2026-02-01" &&
				f.Page == 3 && f.Limit == 5
		}),
	).Return([]models.AgentExecution{}, 0, nil)

	srv := createTestAgentServer(mockContainer)
	url := "/api/v1/" + conformanceTeamID +
		"/agents/agent-1/executions?status=error&date_from=2026-01-01&date_to=2026-02-01&page=3&limit=5"
	req := makeAgentAuthenticatedRequest("GET", url, nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, float64(0), response["total_count"])
	assert.Equal(t, float64(3), response["page"])
	assert.Equal(t, float64(5), response["per_page"])
	assert.Empty(t, response["executions"].([]interface{}))

	mockContainer.agentService.AssertExpectations(t)
}

func TestHandleListAgentExecutions_InvalidPaginationFallsBackToDefaults(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	// page=0 and limit>100 are out of range and must fall back to 1/10.
	mockContainer.agentService.On(
		"ListExecutions", mock.Anything, "user-123",
		mock.MatchedBy(func(f services.AgentExecutionFilters) bool {
			return f.Page == 1 && f.Limit == 10
		}),
	).Return([]models.AgentExecution{}, 0, nil)

	srv := createTestAgentServer(mockContainer)
	url := "/api/v1/" + conformanceTeamID + "/agents/agent-1/executions?page=0&limit=500"
	req := makeAgentAuthenticatedRequest("GET", url, nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
	mockContainer.agentService.AssertExpectations(t)
}

func TestHandleListAgentExecutions_ServiceError(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	mockContainer.agentService.On("ListExecutions", mock.Anything, "user-123", mock.Anything).
		Return(([]models.AgentExecution)(nil), 0, errors.New("database error"))

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest(
		"GET", "/api/v1/"+conformanceTeamID+"/agents/agent-1/executions", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "INTERNAL_ERROR", response["code"])
	assert.Equal(t, "Failed to list executions", response["detail"])

	mockContainer.agentService.AssertExpectations(t)
}

// --- POST /api/v1/{team_id}/agents/{id}/execute ---

func TestHandleExecuteAgent_Success(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	endedAt := time.Now()

	execution := &models.AgentExecution{
		ID: "exec-1", AgentID: "agent-1", UserID: "user-123", Status: "success",
		Input:     map[string]interface{}{"prompt": "hello"},
		StartedAt: time.Now().Add(-time.Minute), EndedAt: &endedAt, Version: 1,
	}

	// No conversation_id in the body → the service must receive a nil pointer
	// (a new conversation is started).
	mockContainer.agentInvocationService.On(
		"InvokeAgent", mock.Anything, "user-123", "agent-1",
		map[string]interface{}{"prompt": "hello"}, (*string)(nil),
	).Return(execution, nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest(
		"POST", "/api/v1/"+conformanceTeamID+"/agents/agent-1/execute",
		map[string]interface{}{"input": map[string]interface{}{"prompt": "hello"}}, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response models.AgentExecution
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "exec-1", response.ID)
	assert.Equal(t, "agent-1", response.AgentID)
	assert.Equal(t, "success", response.Status)

	mockContainer.agentInvocationService.AssertExpectations(t)
}

func TestHandleExecuteAgent_ContinuesConversation(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	conversationID := "conv-9"

	execution := &models.AgentExecution{
		ID: "exec-2", AgentID: "agent-1", UserID: "user-123", Status: "pending",
		ConversationID: &conversationID, StartedAt: time.Now(), Version: 1,
	}

	mockContainer.agentInvocationService.On(
		"InvokeAgent", mock.Anything, "user-123", "agent-1",
		map[string]interface{}{"prompt": "again"},
		mock.MatchedBy(func(id *string) bool { return id != nil && *id == "conv-9" }),
	).Return(execution, nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest(
		"POST", "/api/v1/"+conformanceTeamID+"/agents/agent-1/execute",
		map[string]interface{}{
			"input":           map[string]interface{}{"prompt": "again"},
			"conversation_id": "conv-9",
		}, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response models.AgentExecution
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.NotNil(t, response.ConversationID)
	assert.Equal(t, "conv-9", *response.ConversationID)

	mockContainer.agentInvocationService.AssertExpectations(t)
}

func TestHandleExecuteAgent_InvalidJSON(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentRawRequest(
		"POST", "/api/v1/"+conformanceTeamID+"/agents/agent-1/execute", `{"input":`)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	// The strict invocation mock proves the service was never reached.
	mockContainer.agentInvocationService.AssertNotCalled(t, "InvokeAgent",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestHandleExecuteAgent_ErrorMapping(t *testing.T) {
	tests := []struct {
		name           string
		serviceErr     error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "agent not found maps to 404",
			serviceErr:     errors.New("agent not found"),
			expectedStatus: http.StatusNotFound,
			expectedCode:   "RESOURCE_NOT_FOUND",
		},
		{
			name:           "inactive agent maps to 400",
			serviceErr:     errors.New("agent is not active"),
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "unknown failure maps to 500",
			serviceErr:     errors.New("a2a transport exploded"),
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockAgentContainer(t)
			mockContainer.agentInvocationService.On(
				"InvokeAgent", mock.Anything, "user-123", "agent-1", mock.Anything, (*string)(nil),
			).Return((*models.AgentExecution)(nil), tt.serviceErr)

			srv := createTestAgentServer(mockContainer)
			req := makeAgentAuthenticatedRequest(
				"POST", "/api/v1/"+conformanceTeamID+"/agents/agent-1/execute",
				map[string]interface{}{"input": map[string]interface{}{}}, "user-123")
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			if tt.expectedCode != "" {
				var response map[string]interface{}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, tt.expectedCode, response["code"])
			}

			mockContainer.agentInvocationService.AssertExpectations(t)
		})
	}
}

// --- POST /api/v1/{team_id}/agents/preview-card ---

func TestHandlePreviewAgentCard_Success(t *testing.T) {
	mockContainer := newMockAgentContainer(t)
	cardURL := "https://example.com/.well-known/agent-card.json"

	card := &models.AgentCard{
		Name:        "Code Reviewer Agent",
		Description: "Reviews pull requests",
		Version:     "1.0.0",
		Capabilities: a2a.AgentCapabilities{
			Streaming: true,
		},
	}

	// Preview happens before the agent exists: the fetch must be unauthenticated.
	mockContainer.cardFetcher.On("FetchAgentCard", mock.Anything, cardURL, (map[string]string)(nil)).
		Return(card, nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest(
		"POST", "/api/v1/"+conformanceTeamID+"/agents/preview-card",
		map[string]interface{}{"card_url": cardURL}, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "Code Reviewer Agent", response["name"])
	assert.Equal(t, "1.0.0", response["version"])
	capabilities, ok := response["capabilities"].(map[string]interface{})
	require.True(t, ok, "capabilities must be an object")
	assert.Equal(t, true, capabilities["streaming"])

	mockContainer.cardFetcher.AssertExpectations(t)
}

func TestHandlePreviewAgentCard_MissingCardURL(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentAuthenticatedRequest(
		"POST", "/api/v1/"+conformanceTeamID+"/agents/preview-card",
		map[string]interface{}{}, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "VALIDATION_FAILED", response["code"])
	assert.Contains(t, response["detail"], "Agent card URL is required")

	mockContainer.cardFetcher.AssertNotCalled(t, "FetchAgentCard",
		mock.Anything, mock.Anything, mock.Anything)
}

func TestHandlePreviewAgentCard_FetchErrorMapping(t *testing.T) {
	tests := []struct {
		name           string
		fetchErr       error
		expectedStatus int
	}{
		{
			name:           "card host 404 maps to 400",
			fetchErr:       errors.New("agent card not found at https://example.com/card"),
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "unreachable host maps to 502",
			fetchErr:       errors.New("network error: connection refused"),
			expectedStatus: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockAgentContainer(t)
			cardURL := "https://example.com/card"
			mockContainer.cardFetcher.On("FetchAgentCard", mock.Anything, cardURL, (map[string]string)(nil)).
				Return((*models.AgentCard)(nil), tt.fetchErr)

			srv := createTestAgentServer(mockContainer)
			req := makeAgentAuthenticatedRequest(
				"POST", "/api/v1/"+conformanceTeamID+"/agents/preview-card",
				map[string]interface{}{"card_url": cardURL}, "user-123")
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			mockContainer.cardFetcher.AssertExpectations(t)
		})
	}
}

// --- PUT /api/v1/{team_id}/agents/{id}/credentials ---

func TestHandleUpdateAgentCredentials_Success(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	mockContainer.agentService.On(
		"UpdateAgentCredentials", mock.Anything, "user-123", conformanceTeamID, "agent-1",
		mock.MatchedBy(func(req *models.UpdateAgentCredentialsRequest) bool {
			cred, ok := req.Credentials["api_key"]
			return ok && cred.Type == "apiKey" && cred.Value == "sk-new-1234567890"
		}),
	).Return(nil)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentRawRequest(
		"PUT", "/api/v1/"+conformanceTeamID+"/agents/agent-1/credentials",
		`{"credentials":{"api_key":{"type":"apiKey","value":"sk-new-1234567890"}}}`)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
	assert.Empty(t, w.Body.Bytes(), "204 response must have no body")

	mockContainer.agentService.AssertExpectations(t)
}

func TestHandleUpdateAgentCredentials_UnsupportedTypeRejected(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentRawRequest(
		"PUT", "/api/v1/"+conformanceTeamID+"/agents/agent-1/credentials",
		`{"credentials":{"oauth_token":{"type":"oauth2","value":"tok"}}}`)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "VALIDATION_FAILED", response["code"])
	assert.Contains(t, response["detail"], "oauth2")

	mockContainer.agentService.AssertNotCalled(t, "UpdateAgentCredentials",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestHandleUpdateAgentCredentials_InvalidJSON(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentRawRequest(
		"PUT", "/api/v1/"+conformanceTeamID+"/agents/agent-1/credentials",
		`{"credentials": not-json}`)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
}

func TestHandleUpdateAgentCredentials_NotFound(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	// Production shape: the service wraps the repository sentinel.
	mockContainer.agentService.On(
		"UpdateAgentCredentials", mock.Anything, "user-123", conformanceTeamID, "missing", mock.Anything,
	).Return(fmt.Errorf("failed to get agent: %w", repositories.ErrAgentNotFound))

	srv := createTestAgentServer(mockContainer)
	req := makeAgentRawRequest(
		"PUT", "/api/v1/"+conformanceTeamID+"/agents/missing/credentials",
		`{"credentials":{"api_key":{"type":"apiKey","value":"sk-1"}}}`)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])

	mockContainer.agentService.AssertExpectations(t)
}

// TestHandleUpdateAgentCredentials_PermissionDeniedIsForbidden covers the authz
// branch prompt_agent_rbac_handlers_test.go does not: writing credentials is an
// agent mutation gated by services.AuthorizationService, and a denial must
// surface as a 403 problem document — not fall through to a 500.
func TestHandleUpdateAgentCredentials_PermissionDeniedIsForbidden(t *testing.T) {
	mockContainer := newMockAgentContainer(t)

	mockContainer.agentService.On(
		"UpdateAgentCredentials", mock.Anything, "user-123", conformanceTeamID, "agent-1", mock.Anything,
	).Return(services.ErrPermissionDenied)

	srv := createTestAgentServer(mockContainer)
	req := makeAgentRawRequest(
		"PUT", "/api/v1/"+conformanceTeamID+"/agents/agent-1/credentials",
		`{"credentials":{"api_key":{"type":"apiKey","value":"sk-1"}}}`)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code, "denial must be 403, not 500: %s", w.Body.String())
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), `"code":"FORBIDDEN"`)

	mockContainer.agentService.AssertExpectations(t)
}
