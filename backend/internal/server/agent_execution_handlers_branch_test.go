package server

// Branch coverage for agent_execution_handlers.go (issue #361): parameter
// fallback and empty-result paths of handleCursorBasedPolling and
// handlePageBasedPagination, plus the error/tenancy branches of
// handleGetExecutionStatus, handleGetConversationExecutions and
// handleListAgentConversations that the existing integration tests leave
// uncovered. Reuses the mock container and server from
// agent_execution_handlers_integration_test.go.

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// branchTestExecutionID is the fixed execution id used by the branch tests.
const branchTestExecutionID = "exec-1"

// Fixed identifiers shared by every branch test in this file.
const (
	branchTestUserID  = "user-123"
	branchTestTeamID  = "team-123"
	branchTestAgentID = "agent-1"
)

// newExecutionWithAgentMocks wires GetByID for the branch-test execution (in
// the given status) and its team-scoped agent lookup.
func newExecutionWithAgentMocks(mockContainer *MockAgentExecutionContainer, status string) {
	execution := &models.AgentExecution{
		ID:        branchTestExecutionID,
		AgentID:   branchTestAgentID,
		UserID:    branchTestUserID,
		Status:    status,
		StartedAt: time.Now().Add(-time.Minute),
	}
	agent := &models.Agent{
		ID: branchTestAgentID, UserID: branchTestUserID, TeamID: branchTestTeamID, Name: "Test Agent",
	}

	mockContainer.agentExecutionRepo.On("GetByID", mock.Anything, branchTestUserID, branchTestExecutionID).
		Return(execution, nil)
	mockContainer.agentRepo.On("GetByID", mock.Anything, branchTestUserID, branchTestTeamID, branchTestAgentID).
		Return(agent, nil)
}

// TestHandleGetExecutionEvents_CursorInvalidSinceFallsBackToZero: a
// non-numeric ?since= still selects cursor mode but polls from sequence 0.
func TestHandleGetExecutionEvents_CursorInvalidSinceFallsBackToZero(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()
	executionID, userID, teamID := branchTestExecutionID, branchTestUserID, branchTestTeamID

	newExecutionWithAgentMocks(mockContainer, "completed")

	// Empty result set: the cursor must stay at the fallback value 0.
	mockContainer.agentExecutionEventRepo.On("ListAfterSequence", mock.Anything, executionID, 0).
		Return([]models.AgentExecutionEvent{}, nil)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/executions/"+executionID+"/events?since=not-a-number", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, float64(0), response["next_sequence"], "empty poll must not advance the cursor")
	assert.Equal(t, false, response["has_more"], "completed execution has no more events")
	assert.Empty(t, response["events"].([]interface{}))

	mockContainer.agentExecutionEventRepo.AssertExpectations(t)
}

// TestHandleGetExecutionEvents_CursorNegativeSinceFallsBackToZero: a negative
// cursor is rejected by the >= 0 guard and polls from 0.
func TestHandleGetExecutionEvents_CursorNegativeSinceFallsBackToZero(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()
	executionID, userID, teamID := branchTestExecutionID, branchTestUserID, branchTestTeamID

	newExecutionWithAgentMocks(mockContainer, "pending")

	mockContainer.agentExecutionEventRepo.On("ListAfterSequence", mock.Anything, executionID, 0).
		Return([]models.AgentExecutionEvent{}, nil)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/executions/"+executionID+"/events?since=-5", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, true, response["has_more"], "pending execution keeps polling open")

	mockContainer.agentExecutionEventRepo.AssertExpectations(t)
}

// TestHandleGetExecutionEvents_CursorListError: event-repository failure in
// cursor mode surfaces as a 500 problem document.
func TestHandleGetExecutionEvents_CursorListError(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()
	executionID, userID, teamID := branchTestExecutionID, branchTestUserID, branchTestTeamID

	newExecutionWithAgentMocks(mockContainer, "running")

	mockContainer.agentExecutionEventRepo.On("ListAfterSequence", mock.Anything, executionID, 3).
		Return(nil, errors.New("database error"))

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/executions/"+executionID+"/events?since=3", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "INTERNAL_ERROR", response["code"])
	assert.Equal(t, "Failed to retrieve events", response["detail"])

	mockContainer.agentExecutionEventRepo.AssertExpectations(t)
}

// TestHandleGetExecutionEvents_PageInvalidParamsFallBackToDefaults: page=0 and
// an out-of-range limit fall back to page 1 / limit 50 (offset 0).
func TestHandleGetExecutionEvents_PageInvalidParamsFallBackToDefaults(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()
	executionID, userID, teamID := branchTestExecutionID, branchTestUserID, branchTestTeamID

	newExecutionWithAgentMocks(mockContainer, "completed")

	mockContainer.agentExecutionEventRepo.On("ListByExecutionID", mock.Anything, executionID, 50, 0).
		Return([]models.AgentExecutionEvent{}, 0, nil)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/executions/"+executionID+"/events?page=0&limit=101", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, float64(1), response["page"])
	assert.Equal(t, float64(50), response["per_page"])
	assert.Equal(t, float64(0), response["total_pages"])
	assert.Empty(t, response["events"].([]interface{}))

	mockContainer.agentExecutionEventRepo.AssertExpectations(t)
}

// TestHandleGetExecutionEvents_PageListError: event-repository failure in
// page mode surfaces as a 500 problem document.
func TestHandleGetExecutionEvents_PageListError(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()
	executionID, userID, teamID := branchTestExecutionID, branchTestUserID, branchTestTeamID

	newExecutionWithAgentMocks(mockContainer, "completed")

	mockContainer.agentExecutionEventRepo.On("ListByExecutionID", mock.Anything, executionID, 50, 0).
		Return([]models.AgentExecutionEvent{}, 0, errors.New("database error"))

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/executions/"+executionID+"/events", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "Failed to retrieve execution events", response["detail"])

	mockContainer.agentExecutionEventRepo.AssertExpectations(t)
}

// TestHandleGetExecutionStatus_InternalError: a non-sentinel repository error
// must map to 500, not leak as a 404.
func TestHandleGetExecutionStatus_InternalError(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()
	executionID, userID, teamID := branchTestExecutionID, branchTestUserID, branchTestTeamID

	mockContainer.agentExecutionRepo.On("GetByID", mock.Anything, userID, executionID).
		Return((*models.AgentExecution)(nil), errors.New("connection reset"))

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/executions/"+executionID+"/status", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "INTERNAL_ERROR", response["code"])

	mockContainer.agentExecutionRepo.AssertExpectations(t)
}

// TestHandleGetConversationExecutions_RepositoryError: repository failure
// surfaces as a 500 problem document.
func TestHandleGetConversationExecutions_RepositoryError(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()
	conversationID, userID, teamID := "conv-1", "user-123", "team-123"

	mockContainer.agentExecutionRepo.On(
		"GetByConversationID", mock.Anything, userID, conversationID, 50, (*time.Time)(nil),
	).Return(([]models.AgentExecution)(nil), false, 0, errors.New("database error"))

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/conversations/"+conversationID+"/executions", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "Failed to retrieve conversation", response["detail"])

	mockContainer.agentExecutionRepo.AssertExpectations(t)
}

// TestHandleGetConversationExecutions_EmptyConversation: no executions means
// the team-ownership check has nothing to verify and the handler returns an
// empty (non-null) list.
func TestHandleGetConversationExecutions_EmptyConversation(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()
	conversationID, userID, teamID := "conv-empty", "user-123", "team-123"

	mockContainer.agentExecutionRepo.On(
		"GetByConversationID", mock.Anything, userID, conversationID, 50, (*time.Time)(nil),
	).Return([]models.AgentExecution{}, false, 0, nil)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/conversations/"+conversationID+"/executions", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, float64(0), response["total_count"])
	assert.Equal(t, float64(0), response["count"])
	assert.Empty(t, response["executions"].([]interface{}))

	// With no executions there is no agent to team-check.
	mockContainer.agentRepo.AssertNotCalled(t, "GetByID",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockContainer.agentExecutionRepo.AssertExpectations(t)
}

// TestHandleGetConversationExecutions_CrossTeamNotFound: a conversation whose
// agent is not in the requested team must 404 to avoid information leakage.
func TestHandleGetConversationExecutions_CrossTeamNotFound(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()
	conversationID, userID, teamID := "conv-1", "user-123", "team-b"

	executions := []models.AgentExecution{{
		ID: "exec-1", AgentID: "agent-1", UserID: userID, Status: "completed",
		ConversationID: &conversationID, StartedAt: time.Now(),
	}}

	mockContainer.agentExecutionRepo.On(
		"GetByConversationID", mock.Anything, userID, conversationID, 50, (*time.Time)(nil),
	).Return(executions, false, 1, nil)
	mockContainer.agentRepo.On("GetByID", mock.Anything, userID, teamID, "agent-1").
		Return((*models.Agent)(nil), repositories.ErrAgentNotFound)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/conversations/"+conversationID+"/executions", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "Conversation not found", response["detail"])

	mockContainer.agentRepo.AssertExpectations(t)
}

// TestHandleGetConversationExecutions_QueryParams: a valid limit and a valid
// RFC 3339 `before` reach the repository; out-of-range/invalid values fall
// back to limit 50 / nil before.
func TestHandleGetConversationExecutions_QueryParams(t *testing.T) {
	before := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)

	tests := []struct {
		name          string
		query         string
		expectedLimit int
		expectBefore  *time.Time
	}{
		{
			name:          "valid limit and before are propagated",
			query:         "?limit=25&before=2026-01-02T15:04:05Z",
			expectedLimit: 25,
			expectBefore:  &before,
		},
		{
			name:          "out-of-range limit and invalid before fall back",
			query:         "?limit=500&before=yesterday",
			expectedLimit: 50,
			expectBefore:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockAgentExecutionContainer()
			conversationID, userID, teamID := "conv-1", "user-123", "team-123"

			mockContainer.agentExecutionRepo.On(
				"GetByConversationID", mock.Anything, userID, conversationID, tt.expectedLimit,
				mock.MatchedBy(func(got *time.Time) bool {
					if tt.expectBefore == nil {
						return got == nil
					}
					return got != nil && got.Equal(*tt.expectBefore)
				}),
			).Return([]models.AgentExecution{}, false, 0, nil)

			srv := createTestExecutionServer(mockContainer)
			req := makeExecutionAuthenticatedRequest(
				"/api/v1/"+teamID+"/agents/conversations/"+conversationID+"/executions"+tt.query, userID)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)
			mockContainer.agentExecutionRepo.AssertExpectations(t)
		})
	}
}

// TestHandleListAgentConversations_AgentNotFound: an agent outside the team
// 404s before any conversation listing happens.
func TestHandleListAgentConversations_AgentNotFound(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()
	agentID, userID, teamID := "agent-missing", "user-123", "team-123"

	mockContainer.agentRepo.On("GetByID", mock.Anything, userID, teamID, agentID).
		Return((*models.Agent)(nil), repositories.ErrAgentNotFound)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/"+agentID+"/conversations", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "Agent not found", response["detail"])

	mockContainer.agentExecutionRepo.AssertNotCalled(t, "ListConversations",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockContainer.agentRepo.AssertExpectations(t)
}

// TestHandleListAgentConversations_RepositoryError: listing failure surfaces
// as a 500 problem document.
func TestHandleListAgentConversations_RepositoryError(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()
	agentID, userID, teamID := "agent-1", "user-123", "team-123"

	agent := &models.Agent{ID: agentID, UserID: userID, TeamID: teamID, Name: "Test Agent"}
	mockContainer.agentRepo.On("GetByID", mock.Anything, userID, teamID, agentID).
		Return(agent, nil)
	mockContainer.agentExecutionRepo.On("ListConversations", mock.Anything, userID, agentID, 1, 20).
		Return(([]models.ConversationSummary)(nil), 0, errors.New("database error"))

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/"+agentID+"/conversations", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "Failed to retrieve conversations", response["detail"])

	mockContainer.agentExecutionRepo.AssertExpectations(t)
}

// TestHandleListAgentConversations_PaginationParams: valid page/limit reach the
// repository; an out-of-range limit falls back to the default 20.
func TestHandleListAgentConversations_PaginationParams(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		expectedPage  int
		expectedLimit int
	}{
		{name: "valid page and limit are propagated", query: "?page=3&limit=5", expectedPage: 3, expectedLimit: 5},
		{name: "out-of-range values fall back to defaults", query: "?page=-1&limit=250", expectedPage: 1, expectedLimit: 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockAgentExecutionContainer()
			agentID, userID, teamID := "agent-1", "user-123", "team-123"

			agent := &models.Agent{ID: agentID, UserID: userID, TeamID: teamID, Name: "Test Agent"}
			mockContainer.agentRepo.On("GetByID", mock.Anything, userID, teamID, agentID).
				Return(agent, nil)
			mockContainer.agentExecutionRepo.On(
				"ListConversations", mock.Anything, userID, agentID, tt.expectedPage, tt.expectedLimit,
			).Return([]models.ConversationSummary{}, 0, nil)

			srv := createTestExecutionServer(mockContainer)
			req := makeExecutionAuthenticatedRequest(
				"/api/v1/"+teamID+"/agents/"+agentID+"/conversations"+tt.query, userID)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			var response models.ConversationListResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, tt.expectedPage, response.Page)
			assert.Equal(t, tt.expectedLimit, response.PerPage)

			mockContainer.agentExecutionRepo.AssertExpectations(t)
		})
	}
}
