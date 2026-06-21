package server

import (
	"context"
	"encoding/json"
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
	"github.com/vibexp/vibexp/internal/specconformance"
)

// MockAgentExecutionContainer implements Container interface for execution handler tests
type MockAgentExecutionContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	agentExecutionRepo      *MockAgentExecutionRepository
	agentExecutionEventRepo *MockAgentExecutionEventRepository
	agentRepo               *MockAgentRepository
}

// MockAgentExecutionRepository mocks the AgentExecutionRepository interface
type MockAgentExecutionRepository struct {
	mock.Mock
}

func (m *MockAgentExecutionRepository) Create(ctx context.Context, execution *models.AgentExecution) error {
	args := m.Called(ctx, execution)
	return args.Error(0)
}

func (m *MockAgentExecutionRepository) GetByID(
	ctx context.Context,
	userID, executionID string,
) (*models.AgentExecution, error) {
	args := m.Called(ctx, userID, executionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AgentExecution), args.Error(1)
}

func (m *MockAgentExecutionRepository) List(
	ctx context.Context,
	userID string,
	filters repositories.AgentExecutionFilters,
) ([]models.AgentExecution, int, error) {
	args := m.Called(ctx, userID, filters)
	return args.Get(0).([]models.AgentExecution), args.Int(1), args.Error(2)
}

func (m *MockAgentExecutionRepository) Update(ctx context.Context, execution *models.AgentExecution) error {
	args := m.Called(ctx, execution)
	return args.Error(0)
}

func (m *MockAgentExecutionRepository) GetByAgentID(
	ctx context.Context,
	userID, agentID string,
	filters repositories.AgentExecutionFilters,
) ([]models.AgentExecution, int, error) {
	args := m.Called(ctx, userID, agentID, filters)
	return args.Get(0).([]models.AgentExecution), args.Int(1), args.Error(2)
}

func (m *MockAgentExecutionRepository) GetByTaskID(
	ctx context.Context,
	userID, taskID string,
) (*models.AgentExecution, error) {
	args := m.Called(ctx, userID, taskID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AgentExecution), args.Error(1)
}

func (m *MockAgentExecutionRepository) UpdateTaskInfo(
	ctx context.Context,
	executionID, taskID, contextID, currentState string,
) error {
	args := m.Called(ctx, executionID, taskID, contextID, currentState)
	return args.Error(0)
}

func (m *MockAgentExecutionRepository) UpdateArtifacts(
	ctx context.Context,
	executionID string,
	artifacts []map[string]interface{},
) error {
	args := m.Called(ctx, executionID, artifacts)
	return args.Error(0)
}

func (m *MockAgentExecutionRepository) GetByConversationID(
	ctx context.Context,
	userID, conversationID string,
	limit int,
	before *time.Time,
) ([]models.AgentExecution, bool, int, error) {
	args := m.Called(ctx, userID, conversationID, limit, before)
	return args.Get(0).([]models.AgentExecution), args.Bool(1), args.Int(2), args.Error(3)
}

func (m *MockAgentExecutionRepository) ListConversations(
	ctx context.Context,
	userID, agentID string,
	page, limit int,
) ([]models.ConversationSummary, int, error) {
	args := m.Called(ctx, userID, agentID, page, limit)
	return args.Get(0).([]models.ConversationSummary), args.Int(1), args.Error(2)
}

func (m *MockAgentExecutionRepository) GetFirstExecutionInConversation(
	ctx context.Context,
	userID, conversationID string,
) (*models.AgentExecution, error) {
	args := m.Called(ctx, userID, conversationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AgentExecution), args.Error(1)
}

func (m *MockAgentExecutionRepository) UpdateConversationID(
	ctx context.Context,
	executionID, conversationID string,
) error {
	args := m.Called(ctx, executionID, conversationID)
	return args.Error(0)
}

func (m *MockAgentExecutionRepository) UpdateStatus(ctx context.Context, executionID, status string) error {
	args := m.Called(ctx, executionID, status)
	return args.Error(0)
}

// MockAgentRepository mocks the AgentRepository interface
type MockAgentRepository struct {
	mock.Mock
}

func (m *MockAgentRepository) Create(ctx context.Context, agent *models.Agent) error {
	args := m.Called(ctx, agent)
	return args.Error(0)
}

func (m *MockAgentRepository) GetByID(ctx context.Context, userID, teamID, agentID string) (*models.Agent, error) {
	args := m.Called(ctx, userID, teamID, agentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Agent), args.Error(1)
}

func (m *MockAgentRepository) GetByIDCrossTeam(ctx context.Context, userID, agentID string) (*models.Agent, error) {
	args := m.Called(ctx, userID, agentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Agent), args.Error(1)
}

func (m *MockAgentRepository) List(
	ctx context.Context,
	userID string,
	filters repositories.AgentFilters,
) ([]models.Agent, int, error) {
	args := m.Called(ctx, userID, filters)
	return args.Get(0).([]models.Agent), args.Int(1), args.Error(2)
}

func (m *MockAgentRepository) Update(ctx context.Context, agent *models.Agent) error {
	args := m.Called(ctx, agent)
	return args.Error(0)
}

func (m *MockAgentRepository) Delete(ctx context.Context, userID, teamID, agentID string) error {
	args := m.Called(ctx, userID, teamID, agentID)
	return args.Error(0)
}

func (m *MockAgentRepository) GetStats(ctx context.Context, userID, teamID string) (*models.AgentStatsResponse, error) {
	args := m.Called(ctx, userID, teamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AgentStatsResponse), args.Error(1)
}

func (m *MockAgentRepository) UpdateExecutionStats(
	ctx context.Context, agentID string, success bool, duration int,
) error {
	args := m.Called(ctx, agentID, success, duration)
	return args.Error(0)
}

func (m *MockAgentRepository) GetNamesByIDsCrossTeam(
	ctx context.Context, userID string, ids []string,
) (map[string]string, error) {
	args := m.Called(ctx, userID, ids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]string), args.Error(1)
}

// MockAgentExecutionEventRepository mocks the AgentExecutionEventRepository interface
type MockAgentExecutionEventRepository struct {
	mock.Mock
}

func (m *MockAgentExecutionEventRepository) Create(ctx context.Context, event *models.AgentExecutionEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockAgentExecutionEventRepository) GetByID(
	ctx context.Context,
	eventID string,
) (*models.AgentExecutionEvent, error) {
	args := m.Called(ctx, eventID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AgentExecutionEvent), args.Error(1)
}

func (m *MockAgentExecutionEventRepository) ListByExecutionID(
	ctx context.Context,
	executionID string,
	limit, offset int,
) ([]models.AgentExecutionEvent, int, error) {
	args := m.Called(ctx, executionID, limit, offset)
	return args.Get(0).([]models.AgentExecutionEvent), args.Int(1), args.Error(2)
}

func (m *MockAgentExecutionEventRepository) ListAfterSequence(
	ctx context.Context,
	executionID string,
	afterSequence int,
) ([]models.AgentExecutionEvent, error) {
	args := m.Called(ctx, executionID, afterSequence)
	if args.Get(0) == nil {
		return []models.AgentExecutionEvent{}, args.Error(1)
	}
	return args.Get(0).([]models.AgentExecutionEvent), args.Error(1)
}

func (m *MockAgentExecutionEventRepository) GetLatestByExecutionID(
	ctx context.Context,
	executionID string,
) (*models.AgentExecutionEvent, error) {
	args := m.Called(ctx, executionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AgentExecutionEvent), args.Error(1)
}

func (m *MockAgentExecutionEventRepository) CountByExecutionID(ctx context.Context, executionID string) (int, error) {
	args := m.Called(ctx, executionID)
	return args.Int(0), args.Error(1)
}

// Container interface stub implementations
func (m *MockAgentExecutionContainer) AgentExecutionRepository() repositories.AgentExecutionRepository {
	return m.agentExecutionRepo
}

func (m *MockAgentExecutionContainer) AgentExecutionEventRepository() repositories.AgentExecutionEventRepository {
	return m.agentExecutionEventRepo
}

func (m *MockAgentExecutionContainer) AgentRepository() repositories.AgentRepository {
	return m.agentRepo
}

func newMockAgentExecutionContainer() *MockAgentExecutionContainer {
	return &MockAgentExecutionContainer{
		agentExecutionRepo:      &MockAgentExecutionRepository{},
		agentExecutionEventRepo: &MockAgentExecutionEventRepository{},
		agentRepo:               &MockAgentRepository{},
	}
}

func createTestExecutionServer(container *MockAgentExecutionContainer) *Server {
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

	// Register execution monitoring routes with team scoping
	r.Route("/api/v1/{team_id}/agents/executions", func(r chi.Router) {
		r.Get("/{id}/status", srv.handleGetExecutionStatus)
		r.Get("/{id}/events", srv.handleGetExecutionEvents)
	})

	r.Route("/api/v1/{team_id}/agents/conversations", func(r chi.Router) {
		r.Get("/{conversation_id}/executions", srv.handleGetConversationExecutions)
	})

	r.Route("/api/v1/{team_id}/agents", func(r chi.Router) {
		r.Get("/{id}/conversations", srv.handleListAgentConversations)
	})

	return srv
}

//nolint:unparam // userID parameter kept for test flexibility, may vary in future tests
func makeExecutionAuthenticatedRequest(path string, userID string) *http.Request {
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, userID))
	return req
}

// TestHandleGetExecutionStatus_Running tests get execution status for running execution
func TestHandleGetExecutionStatus_Running(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()

	executionID := "exec-running-123"
	userID := "user-123"
	teamID := "team-123"
	agentID := "agent-1"
	currentState := "working"

	expectedExecution := &models.AgentExecution{
		ID:           executionID,
		AgentID:      agentID,
		UserID:       userID,
		Status:       "running",
		CurrentState: &currentState,
		Input:        map[string]interface{}{"task": "test task"},
		StartedAt:    time.Now().Add(-5 * time.Minute),
		Artifacts:    []map[string]interface{}{},
	}

	expectedAgent := &models.Agent{
		ID:     agentID,
		UserID: userID,
		TeamID: teamID,
		Name:   "Test Agent",
	}

	mockContainer.agentExecutionRepo.On("GetByID", mock.Anything, userID, executionID).
		Return(expectedExecution, nil)
	mockContainer.agentRepo.On("GetByID", mock.Anything, userID, teamID, agentID).
		Return(expectedAgent, nil)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest("/api/v1/"+teamID+"/agents/executions/"+executionID+"/status", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.AgentExecution
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedExecution.ID, response.ID)
	assert.Equal(t, expectedExecution.Status, response.Status)
	assert.Equal(t, *expectedExecution.CurrentState, *response.CurrentState)

	mockContainer.agentExecutionRepo.AssertExpectations(t)
	mockContainer.agentRepo.AssertExpectations(t)
}

// TestHandleGetExecutionStatus_Completed tests get execution status for completed execution
func TestHandleGetExecutionStatus_Completed(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()

	executionID := "exec-completed-123"
	userID := "user-123"
	teamID := "team-123"
	agentID := "agent-1"
	currentState := "completed"
	endedAt := time.Now()

	expectedExecution := &models.AgentExecution{
		ID:           executionID,
		AgentID:      agentID,
		UserID:       userID,
		Status:       "completed",
		CurrentState: &currentState,
		Input:        map[string]interface{}{"task": "completed task"},
		StartedAt:    time.Now().Add(-10 * time.Minute),
		EndedAt:      &endedAt,
		Duration:     func() *int { d := 600000; return &d }(), // 10 minutes in ms
		Artifacts: []map[string]interface{}{
			{"id": "art-1", "name": "result.json", "type": "json"},
		},
	}

	expectedAgent := &models.Agent{
		ID:     agentID,
		UserID: userID,
		TeamID: teamID,
		Name:   "Test Agent",
	}

	mockContainer.agentExecutionRepo.On("GetByID", mock.Anything, userID, executionID).
		Return(expectedExecution, nil)
	mockContainer.agentRepo.On("GetByID", mock.Anything, userID, teamID, agentID).
		Return(expectedAgent, nil)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest("/api/v1/"+teamID+"/agents/executions/"+executionID+"/status", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.AgentExecution
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedExecution.ID, response.ID)
	assert.Equal(t, expectedExecution.Status, response.Status)
	assert.Equal(t, "completed", *response.CurrentState)
	assert.NotNil(t, response.EndedAt)
	assert.Len(t, response.Artifacts, 1)

	mockContainer.agentExecutionRepo.AssertExpectations(t)
	mockContainer.agentRepo.AssertExpectations(t)
}

// TestHandleGetExecutionStatus_Failed tests get execution status for failed execution
func TestHandleGetExecutionStatus_Failed(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()

	executionID := "exec-failed-123"
	userID := "user-123"
	teamID := "team-123"
	agentID := "agent-1"
	currentState := "failed"
	errorMsg := "Task execution failed: timeout"
	endedAt := time.Now()

	expectedExecution := &models.AgentExecution{
		ID:           executionID,
		AgentID:      agentID,
		UserID:       userID,
		Status:       "failed",
		CurrentState: &currentState,
		Error:        &errorMsg,
		Input:        map[string]interface{}{"task": "failing task"},
		StartedAt:    time.Now().Add(-3 * time.Minute),
		EndedAt:      &endedAt,
	}

	expectedAgent := &models.Agent{
		ID:     agentID,
		UserID: userID,
		TeamID: teamID,
		Name:   "Test Agent",
	}

	mockContainer.agentExecutionRepo.On("GetByID", mock.Anything, userID, executionID).
		Return(expectedExecution, nil)
	mockContainer.agentRepo.On("GetByID", mock.Anything, userID, teamID, agentID).
		Return(expectedAgent, nil)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest("/api/v1/"+teamID+"/agents/executions/"+executionID+"/status", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.AgentExecution
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedExecution.ID, response.ID)
	assert.Equal(t, "failed", response.Status)
	assert.NotNil(t, response.Error)
	assert.Contains(t, *response.Error, "timeout")

	mockContainer.agentExecutionRepo.AssertExpectations(t)
	mockContainer.agentRepo.AssertExpectations(t)
}

// TestHandleGetExecutionStatus_NotFound tests get execution status when execution not found
func TestHandleGetExecutionStatus_NotFound(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()

	executionID := "exec-nonexistent"
	userID := "user-123"
	teamID := "team-123"

	mockContainer.agentExecutionRepo.On("GetByID", mock.Anything, userID, executionID).
		Return((*models.AgentExecution)(nil), repositories.ErrAgentExecutionNotFound)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest("/api/v1/"+teamID+"/agents/executions/"+executionID+"/status", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])
	assert.Equal(t, "Execution not found", response["detail"])

	mockContainer.agentExecutionRepo.AssertExpectations(t)
}

// TestHandleGetExecutionStatus_Unauthorized tests get execution status for different user's execution
func TestHandleGetExecutionStatus_Unauthorized(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()

	executionID := "exec-other-user"
	userID := "user-123"
	teamID := "team-123"

	// Simulate unauthorized access by returning not found error
	mockContainer.agentExecutionRepo.On("GetByID", mock.Anything, userID, executionID).
		Return((*models.AgentExecution)(nil), repositories.ErrAgentExecutionNotFound)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest("/api/v1/"+teamID+"/agents/executions/"+executionID+"/status", userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNotFound, w.Code)

	mockContainer.agentExecutionRepo.AssertExpectations(t)
}

func setupCursorBasedPollingMocks(
	mockContainer *MockAgentExecutionContainer,
	executionID, userID, teamID string,
) {
	currentState := "working"
	agentID := "agent-1"

	execution := &models.AgentExecution{
		ID:           executionID,
		AgentID:      agentID,
		UserID:       userID,
		Status:       "running",
		CurrentState: &currentState,
		StartedAt:    time.Now().Add(-5 * time.Minute),
	}

	agent := &models.Agent{
		ID:     agentID,
		UserID: userID,
		TeamID: teamID,
		Name:   "Test Agent",
	}

	events := []models.AgentExecutionEvent{
		{
			ID:             "event-3",
			ExecutionID:    executionID,
			EventType:      "status-update",
			EventData:      map[string]interface{}{"state": "working"},
			SequenceNumber: 3,
			ReceivedAt:     time.Now().Add(-2 * time.Minute),
		},
		{
			ID:             "event-4",
			ExecutionID:    executionID,
			EventType:      "task",
			EventData:      map[string]interface{}{"message": "Processing data"},
			SequenceNumber: 4,
			ReceivedAt:     time.Now().Add(-1 * time.Minute),
		},
	}

	mockContainer.agentExecutionRepo.On("GetByID", mock.Anything, userID, executionID).
		Return(execution, nil)
	mockContainer.agentRepo.On("GetByID", mock.Anything, userID, teamID, agentID).
		Return(agent, nil)

	mockContainer.agentExecutionEventRepo.On("ListAfterSequence", mock.Anything, executionID, 2).
		Return(events, nil)
}

// TestHandleGetExecutionEvents_CursorBasedPolling tests cursor-based event polling
func TestHandleGetExecutionEvents_CursorBasedPolling(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()

	executionID := "exec-123"
	userID := "user-123"
	teamID := "team-123"

	setupCursorBasedPollingMocks(mockContainer, executionID, userID, teamID)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/executions/"+executionID+"/events?since=2",
		userID,
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, executionID, response["execution_id"])
	assert.Equal(t, "running", response["status"])
	assert.Equal(t, true, response["has_more"])
	// Cursor is the LAST returned sequence (4), not lastSeq+1: the repo filters
	// `sequence_number > since`, so re-polling with this value returns event 5
	// rather than skipping it (#1704).
	assert.Equal(t, float64(4), response["next_sequence"])

	responseEvents := response["events"].([]interface{})
	assert.Len(t, responseEvents, 2)

	mockContainer.agentExecutionRepo.AssertExpectations(t)
	mockContainer.agentExecutionEventRepo.AssertExpectations(t)
	mockContainer.agentRepo.AssertExpectations(t)
}

// TestHandleGetExecutionEvents_CursorNoGap is the #1704 regression guard: feeding
// the returned next_sequence back as `since` must return the very next event, not
// skip it. With the old `lastSeq+1` cursor and the repo's `sequence_number > since`
// filter, event lastSeq+1 was silently dropped.
func TestHandleGetExecutionEvents_CursorNoGap(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()
	executionID := "exec-123"
	userID := "user-123"
	teamID := "team-123"

	// First poll (since=2) returns events 3 and 4 → cursor 4.
	setupCursorBasedPollingMocks(mockContainer, executionID, userID, teamID)
	// Re-polling with that cursor (since=4) must return event 5: the repo filters
	// `sequence_number > 4`, so a lastSeq+1 cursor would have skipped it (#1704).
	mockContainer.agentExecutionEventRepo.On("ListAfterSequence", mock.Anything, executionID, 4).
		Return([]models.AgentExecutionEvent{{
			ID: "event-5", ExecutionID: executionID, EventType: "task",
			EventData: map[string]interface{}{"message": "step 5"}, SequenceNumber: 5, ReceivedAt: time.Now(),
		}}, nil)

	srv := createTestExecutionServer(mockContainer)
	poll := func(since int) map[string]interface{} {
		req := makeExecutionAuthenticatedRequest(
			fmt.Sprintf("/api/v1/%s/agents/executions/%s/events?since=%d", teamID, executionID, since), userID)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		specconformance.AssertConformsToSpec(t, req, w)
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}

	first := poll(2)
	require.Equal(t, float64(4), first["next_sequence"], "cursor must be last returned seq, not lastSeq+1")
	second := poll(int(first["next_sequence"].(float64)))
	events := second["events"].([]interface{})
	require.Len(t, events, 1, "re-polling with the cursor must return event 5, not skip it")
	assert.Equal(t, float64(5), events[0].(map[string]interface{})["sequence_number"])
}

func setupPageBasedPaginationMocks(
	mockContainer *MockAgentExecutionContainer,
	executionID, userID, teamID string,
) (*models.AgentExecution, []models.AgentExecutionEvent) {
	currentState := "completed"
	agentID := "agent-1"

	execution := &models.AgentExecution{
		ID:           executionID,
		AgentID:      agentID,
		UserID:       userID,
		Status:       "completed",
		CurrentState: &currentState,
		StartedAt:    time.Now().Add(-10 * time.Minute),
	}

	agent := &models.Agent{
		ID:     agentID,
		UserID: userID,
		TeamID: teamID,
		Name:   "Test Agent",
	}

	events := []models.AgentExecutionEvent{
		{
			ID:             "event-1",
			ExecutionID:    executionID,
			EventType:      "task",
			EventData:      map[string]interface{}{"message": "Task started"},
			SequenceNumber: 1,
			ReceivedAt:     time.Now().Add(-10 * time.Minute),
		},
		{
			ID:             "event-2",
			ExecutionID:    executionID,
			EventType:      "status-update",
			EventData:      map[string]interface{}{"state": "completed"},
			SequenceNumber: 2,
			ReceivedAt:     time.Now().Add(-5 * time.Minute),
		},
	}

	mockContainer.agentExecutionRepo.On("GetByID", mock.Anything, userID, executionID).
		Return(execution, nil)
	mockContainer.agentRepo.On("GetByID", mock.Anything, userID, teamID, agentID).
		Return(agent, nil)

	mockContainer.agentExecutionEventRepo.On("ListByExecutionID", mock.Anything, executionID, 50, 0).
		Return(events, 2, nil)

	return execution, events
}

// TestHandleGetExecutionEvents_PageBasedPagination tests page-based event pagination
func TestHandleGetExecutionEvents_PageBasedPagination(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()

	executionID := "exec-123"
	userID := "user-123"
	teamID := "team-123"

	setupPageBasedPaginationMocks(mockContainer, executionID, userID, teamID)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/executions/"+executionID+"/events",
		userID,
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, float64(2), response["total_count"])
	assert.Equal(t, float64(1), response["page"])
	assert.Equal(t, float64(50), response["per_page"])
	assert.Equal(t, float64(1), response["total_pages"])

	responseEvents := response["events"].([]interface{})
	assert.Len(t, responseEvents, 2)

	mockContainer.agentExecutionRepo.AssertExpectations(t)
	mockContainer.agentExecutionEventRepo.AssertExpectations(t)
	mockContainer.agentRepo.AssertExpectations(t)
}

// TestHandleGetExecutionEvents_EmptyEvents tests getting events when none exist
func TestHandleGetExecutionEvents_EmptyEvents(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()

	executionID := "exec-no-events"
	userID := "user-123"
	teamID := "team-123"
	agentID := "agent-1"
	currentState := "pending"

	execution := &models.AgentExecution{
		ID:           executionID,
		AgentID:      agentID,
		UserID:       userID,
		Status:       "pending",
		CurrentState: &currentState,
		StartedAt:    time.Now(),
	}

	agent := &models.Agent{
		ID:     agentID,
		UserID: userID,
		TeamID: teamID,
		Name:   "Test Agent",
	}

	mockContainer.agentExecutionRepo.On("GetByID", mock.Anything, userID, executionID).
		Return(execution, nil)
	mockContainer.agentRepo.On("GetByID", mock.Anything, userID, teamID, agentID).
		Return(agent, nil)

	mockContainer.agentExecutionEventRepo.On("ListByExecutionID", mock.Anything, executionID, 50, 0).
		Return([]models.AgentExecutionEvent{}, 0, nil)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/executions/"+executionID+"/events",
		userID,
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, float64(0), response["total_count"])
	responseEvents := response["events"].([]interface{})
	assert.Len(t, responseEvents, 0)

	mockContainer.agentExecutionRepo.AssertExpectations(t)
	mockContainer.agentExecutionEventRepo.AssertExpectations(t)
	mockContainer.agentRepo.AssertExpectations(t)
}

// TestHandleGetExecutionEvents_NotFound tests getting events for non-existent execution
func TestHandleGetExecutionEvents_NotFound(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()

	executionID := "exec-nonexistent"
	userID := "user-123"
	teamID := "team-123"

	mockContainer.agentExecutionRepo.On("GetByID", mock.Anything, userID, executionID).
		Return((*models.AgentExecution)(nil), repositories.ErrAgentExecutionNotFound)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/executions/"+executionID+"/events",
		userID,
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])
	assert.Equal(t, "Execution not found", response["detail"])

	mockContainer.agentExecutionRepo.AssertExpectations(t)
}

// TestHandleGetExecutionEvents_Unauthorized tests getting events for unauthorized execution
func TestHandleGetExecutionEvents_Unauthorized(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()

	executionID := "exec-other-user"
	userID := "user-123"
	teamID := "team-123"

	mockContainer.agentExecutionRepo.On("GetByID", mock.Anything, userID, executionID).
		Return((*models.AgentExecution)(nil), repositories.ErrAgentExecutionNotFound)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/executions/"+executionID+"/events",
		userID,
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusNotFound, w.Code)

	mockContainer.agentExecutionRepo.AssertExpectations(t)
}

// TestHandleGetExecutionEvents_WithPagination tests page-based pagination with custom page and limit
//
//nolint:funlen // Test requires comprehensive setup and assertions
func TestHandleGetExecutionEvents_WithPagination(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()

	executionID := "exec-paginated"
	userID := "user-123"
	teamID := "team-123"
	agentID := "agent-1"
	currentState := "completed"

	execution := &models.AgentExecution{
		ID:           executionID,
		AgentID:      agentID,
		UserID:       userID,
		Status:       "completed",
		CurrentState: &currentState,
		StartedAt:    time.Now().Add(-1 * time.Hour),
	}

	agent := &models.Agent{
		ID:     agentID,
		UserID: userID,
		TeamID: teamID,
		Name:   "Test Agent",
	}

	events := []models.AgentExecutionEvent{
		{
			ID:             "event-11",
			ExecutionID:    executionID,
			EventType:      "task",
			EventData:      map[string]interface{}{"message": "Event 11"},
			SequenceNumber: 11,
			ReceivedAt:     time.Now().Add(-30 * time.Minute),
		},
	}

	mockContainer.agentExecutionRepo.On("GetByID", mock.Anything, userID, executionID).
		Return(execution, nil)
	mockContainer.agentRepo.On("GetByID", mock.Anything, userID, teamID, agentID).
		Return(agent, nil)

	// Page 2 with limit 10, offset = (2-1) * 10 = 10
	mockContainer.agentExecutionEventRepo.On("ListByExecutionID", mock.Anything, executionID, 10, 10).
		Return(events, 25, nil)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/executions/"+executionID+"/events?page=2&limit=10",
		userID,
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, float64(25), response["total_count"])
	assert.Equal(t, float64(2), response["page"])
	assert.Equal(t, float64(10), response["per_page"])
	assert.Equal(t, float64(3), response["total_pages"]) // ceil(25/10) = 3

	mockContainer.agentExecutionRepo.AssertExpectations(t)
	mockContainer.agentExecutionEventRepo.AssertExpectations(t)
	mockContainer.agentRepo.AssertExpectations(t)
}

// TestHandleGetConversationExecutions_Success tests getting executions by conversation ID
//
//nolint:funlen // Test requires comprehensive setup and assertions
func TestHandleGetConversationExecutions_Success(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()

	conversationID := "conv-123"
	userID := "user-123"
	teamID := "team-123"
	agentID := "agent-1"

	executions := []models.AgentExecution{
		{
			ID:             "exec-1",
			AgentID:        agentID,
			UserID:         userID,
			Status:         "completed",
			ConversationID: &conversationID,
			StartedAt:      time.Now().Add(-10 * time.Minute),
		},
		{
			ID:             "exec-2",
			AgentID:        agentID,
			UserID:         userID,
			Status:         "running",
			ConversationID: &conversationID,
			StartedAt:      time.Now().Add(-2 * time.Minute),
		},
	}

	agent := &models.Agent{
		ID:     agentID,
		UserID: userID,
		TeamID: teamID,
		Name:   "Test Agent",
	}

	mockContainer.agentExecutionRepo.On(
		"GetByConversationID",
		mock.Anything,
		userID,
		conversationID,
		50,
		(*time.Time)(nil),
	).Return(executions, false, 2, nil)
	mockContainer.agentRepo.On("GetByID", mock.Anything, userID, teamID, agentID).
		Return(agent, nil)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/conversations/"+conversationID+"/executions",
		userID,
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, conversationID, response["conversation_id"])
	assert.Equal(t, float64(2), response["total_count"])
	assert.Equal(t, float64(2), response["count"])
	assert.Equal(t, false, response["has_more"])

	responseExecs := response["executions"].([]interface{})
	assert.Len(t, responseExecs, 2)

	mockContainer.agentExecutionRepo.AssertExpectations(t)
	mockContainer.agentRepo.AssertExpectations(t)
}

// TestHandleListAgentConversations_Success tests listing conversations for an agent
//
//nolint:funlen // Test requires comprehensive setup and assertions
func TestHandleListAgentConversations_Success(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()

	agentID := "agent-123"
	userID := "user-123"
	teamID := "team-123"

	conversations := []models.ConversationSummary{
		{
			ConversationID: "conv-1",
			AgentID:        agentID,
			MessageCount:   5,
			FirstMessage:   "Hello",
			LastMessage:    "Goodbye",
			StartedAt:      time.Now().Add(-24 * time.Hour),
			LastActivityAt: time.Now().Add(-1 * time.Hour),
			LastStatus:     "completed",
		},
		{
			ConversationID: "conv-2",
			AgentID:        agentID,
			MessageCount:   3,
			FirstMessage:   "Hi",
			LastMessage:    "Thanks",
			StartedAt:      time.Now().Add(-12 * time.Hour),
			LastActivityAt: time.Now().Add(-30 * time.Minute),
			LastStatus:     "running",
		},
	}

	agent := &models.Agent{
		ID:     agentID,
		UserID: userID,
		TeamID: teamID,
		Name:   "Test Agent",
	}

	mockContainer.agentRepo.On("GetByID", mock.Anything, userID, teamID, agentID).
		Return(agent, nil)
	mockContainer.agentExecutionRepo.On("ListConversations", mock.Anything, userID, agentID, 1, 20).
		Return(conversations, 2, nil)

	srv := createTestExecutionServer(mockContainer)
	req := makeExecutionAuthenticatedRequest(
		"/api/v1/"+teamID+"/agents/"+agentID+"/conversations",
		userID,
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ConversationListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, 2, response.TotalCount)
	assert.Equal(t, 1, response.Page)
	assert.Equal(t, 20, response.PerPage)
	assert.Equal(t, 1, response.TotalPages)
	assert.Len(t, response.Conversations, 2)

	mockContainer.agentExecutionRepo.AssertExpectations(t)
	mockContainer.agentRepo.AssertExpectations(t)
}

// TestHandleGetExecutionStatus_CrossTeamAccess tests that accessing an execution from wrong team returns 404
func TestHandleGetExecutionStatus_CrossTeamAccess(t *testing.T) {
	mockContainer := newMockAgentExecutionContainer()

	executionID := "exec-team-a"
	userID := "user-123"
	requestedTeamID := "team-b" // User requests with team B

	agentID := "agent-1"
	currentState := "running"

	expectedExecution := &models.AgentExecution{
		ID:           executionID,
		AgentID:      agentID,
		UserID:       userID,
		Status:       "running",
		CurrentState: &currentState,
		StartedAt:    time.Now(),
	}

	// Execution exists
	mockContainer.agentExecutionRepo.On("GetByID", mock.Anything, userID, executionID).
		Return(expectedExecution, nil)

	// But agent is not found in team B (returns error)
	mockContainer.agentRepo.On("GetByID", mock.Anything, userID, requestedTeamID, agentID).
		Return((*models.Agent)(nil), repositories.ErrAgentNotFound)

	srv := createTestExecutionServer(mockContainer)
	url := "/api/v1/" + requestedTeamID + "/agents/executions/" + executionID + "/status"
	req := makeExecutionAuthenticatedRequest(url, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	// Should return 404 to prevent information leakage
	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])
	assert.Equal(t, "Execution not found", response["detail"])

	mockContainer.agentExecutionRepo.AssertExpectations(t)
	mockContainer.agentRepo.AssertExpectations(t)
}
