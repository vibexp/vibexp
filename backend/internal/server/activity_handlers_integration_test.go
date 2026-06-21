package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// MockActivityService is a mock implementation of activities.ActivityService
type MockActivityService struct {
	mock.Mock
}

func (m *MockActivityService) RecordActivity(
	ctx context.Context, userID string, req activities.CreateActivityRequest,
) (*activities.Activity, error) {
	args := m.Called(ctx, userID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activities.Activity), args.Error(1)
}

func (m *MockActivityService) RecordAuthActivity(
	ctx context.Context, userID string, activityType string, sessionID *string,
	metadata map[string]interface{}, sourceIP *string, userAgent *string,
) error {
	args := m.Called(ctx, userID, activityType, sessionID, metadata, sourceIP, userAgent)
	return args.Error(0)
}

func (m *MockActivityService) RecordResourceActivity(
	ctx context.Context, userID string, activityType string, entityType string,
	entityID *string, description string, metadata map[string]interface{},
) error {
	args := m.Called(ctx, userID, activityType, entityType, entityID, description, metadata)
	return args.Error(0)
}

func (m *MockActivityService) RecordClaudeCodeActivity(
	ctx context.Context, userID string, sessionID string, toolName *string,
	hookEventName string, metadata map[string]interface{},
) error {
	args := m.Called(ctx, userID, sessionID, toolName, hookEventName, metadata)
	return args.Error(0)
}

func (m *MockActivityService) GetActivities(
	ctx context.Context, filters activities.ActivityFilters,
) (*activities.ActivityListResponse, error) {
	args := m.Called(ctx, filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activities.ActivityListResponse), args.Error(1)
}

func (m *MockActivityService) GetActivityByID(
	ctx context.Context, userID string, activityID string,
) (*activities.Activity, error) {
	args := m.Called(ctx, userID, activityID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activities.Activity), args.Error(1)
}

func (m *MockActivityService) GetActivityStats(
	ctx context.Context, userID string,
) (*activities.ActivityStatsResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activities.ActivityStatsResponse), args.Error(1)
}

func (m *MockActivityService) DeleteActivity(ctx context.Context, activityID string) error {
	args := m.Called(ctx, activityID)
	return args.Error(0)
}

func (m *MockActivityService) GetActivityTypes() []string {
	args := m.Called()
	return args.Get(0).([]string)
}

func (m *MockActivityService) GetEntityTypes() []string {
	args := m.Called()
	return args.Get(0).([]string)
}

func (m *MockActivityService) GetAllTypes() *activities.ActivityTypesResponse {
	args := m.Called()
	return args.Get(0).(*activities.ActivityTypesResponse)
}

func (m *MockActivityService) RunRetentionJob(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// MockActivityContainer implements Container interface for activity handler tests
type MockActivityContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	activityService *MockActivityService
}

// Only override methods that return non-nil mocks
func (m *MockActivityContainer) ActivityService() activities.ActivityService {
	return m.activityService
}

func newMockActivityContainer(_ *testing.T) *MockActivityContainer {
	return &MockActivityContainer{
		activityService: &MockActivityService{},
	}
}

func createTestActivityServer(container *MockActivityContainer) *Server {
	cfg := &config.Config{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during test

	// Initialize router manually for testing
	r := chi.NewRouter()

	srv := &Server{
		port:            "8080",
		container:       container,
		activityService: container.activityService,
		logger:          logger,
		config:          cfg,
		router:          r,
	}

	// Register routes manually (simplified version for testing)
	r.Route("/api/v1/activities", func(r chi.Router) {
		r.Get("/", srv.handleActivitiesGet)
		r.Post("/", srv.handleActivityPost)
		r.Get("/stats", srv.handleActivitiesStatsGet)
		r.Get("/types", srv.handleActivitiesTypesGet)
		r.Get("/entity-types", srv.handleActivitiesEntityTypesGet)
		r.Get("/{id}", srv.handleActivityGet)
	})
	// Register internal job route without OIDC middleware (middleware is bypassed in unit tests).
	r.Post("/internal/jobs/activities/retention", srv.handleActivityRetentionJob)

	return srv
}

func makeAuthenticatedActivityRequest(method, path string, body interface{}, userID string) *http.Request {
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

// TestHandleGetActivities_Success tests successful retrieval of activities with mocked service
func TestHandleGetActivities_Success(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"
	activityType := "auth_login"
	entityType := "user"

	// Mock expectations
	mockContainer.activityService.On(
		"GetActivities", mock.Anything,
		mock.MatchedBy(func(filters activities.ActivityFilters) bool {
			return *filters.UserID == userID &&
				*filters.ActivityType == activityType &&
				*filters.EntityType == entityType &&
				filters.Limit == 25
		})).Return(&activities.ActivityListResponse{
		Activities: []activities.Activity{
			{
				ID:           "activity-1",
				UserID:       userID,
				ActivityType: activityType,
				EntityType:   entityType,
				Description:  "User logged in",
				CreatedAt:    time.Now(),
			},
		},
		TotalCount: 1,
		Page:       1,
		PerPage:    25,
		TotalPages: 1,
	}, nil)

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest(
		"GET", "/api/v1/activities?activity_type=auth_login&entity_type=user",
		nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "Activities retrieved successfully", response["message"])
	assert.NotNil(t, response["data"])

	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(1), data["total_count"])
	assert.Equal(t, float64(1), data["page"])

	activities := data["activities"].([]interface{})
	assert.Len(t, activities, 1)

	activity := activities[0].(map[string]interface{})
	assert.Equal(t, "activity-1", activity["id"])
	assert.Equal(t, activityType, activity["activity_type"])

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleGetActivities_WithPagination tests activities retrieval with pagination
func TestHandleGetActivities_WithPagination(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"

	mockContainer.activityService.On(
		"GetActivities", mock.Anything,
		mock.MatchedBy(func(filters activities.ActivityFilters) bool {
			return *filters.UserID == userID && filters.Limit == 10 && filters.Offset == 10
		})).Return(&activities.ActivityListResponse{
		Activities: []activities.Activity{},
		TotalCount: 25,
		Page:       2,
		PerPage:    10,
		TotalPages: 3,
	}, nil)

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("GET", "/api/v1/activities?page=2&limit=10", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(25), data["total_count"])
	assert.Equal(t, float64(2), data["page"])
	assert.Equal(t, float64(3), data["total_pages"])

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleGetActivities_WithEntityIDFilter tests activities retrieval filtered by entity_id
func TestHandleGetActivities_WithEntityIDFilter(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"
	entityID := "entity-456"

	mockContainer.activityService.On(
		"GetActivities", mock.Anything,
		mock.MatchedBy(func(filters activities.ActivityFilters) bool {
			return *filters.UserID == userID && *filters.EntityID == entityID
		})).Return(&activities.ActivityListResponse{
		Activities: []activities.Activity{
			{
				ID:           "activity-1",
				UserID:       userID,
				ActivityType: "prompt_created",
				EntityType:   "prompt",
				EntityID:     &entityID,
				Description:  "Prompt created",
				CreatedAt:    time.Now(),
			},
		},
		TotalCount: 1,
		Page:       1,
		PerPage:    25,
		TotalPages: 1,
	}, nil)

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("GET", "/api/v1/activities?entity_id=entity-456", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	data := response["data"].(map[string]interface{})
	activities := data["activities"].([]interface{})
	assert.Len(t, activities, 1)

	activity := activities[0].(map[string]interface{})
	assert.Equal(t, "prompt_created", activity["activity_type"])
	assert.Equal(t, entityID, activity["entity_id"])

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleGetActivities_ServiceError tests error handling when service fails
func TestHandleGetActivities_ServiceError(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"

	mockContainer.activityService.On("GetActivities", mock.Anything, mock.Anything).
		Return((*activities.ActivityListResponse)(nil), errors.New("database error"))

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("GET", "/api/v1/activities", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleGetActivityByID_Success tests successful retrieval of a single activity
func TestHandleGetActivityByID_Success(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"
	activityID := "activity-456"

	expectedActivity := &activities.Activity{
		ID:           activityID,
		UserID:       userID,
		ActivityType: "auth_login",
		EntityType:   "user",
		Description:  "User logged in successfully",
		CreatedAt:    time.Now(),
	}

	mockContainer.activityService.On("GetActivityByID", mock.Anything, userID, activityID).
		Return(expectedActivity, nil)

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("GET", "/api/v1/activities/activity-456", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "Activity retrieved successfully", response["message"])

	data := response["data"].(map[string]interface{})
	assert.Equal(t, activityID, data["id"])
	assert.Equal(t, "auth_login", data["activity_type"])

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleGetActivityByID_NotFound tests retrieval of non-existent activity
func TestHandleGetActivityByID_NotFound(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"
	activityID := "non-existent"

	mockContainer.activityService.On("GetActivityByID", mock.Anything, userID, activityID).
		Return((*activities.Activity)(nil), repositories.ErrActivityNotFound)

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("GET", "/api/v1/activities/non-existent", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleGetActivityStats_Success tests successful retrieval of activity stats
func TestHandleGetActivityStats_Success(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"

	expectedStats := &activities.ActivityStatsResponse{
		TotalActivities:    100,
		ActivitiesToday:    10,
		ActivitiesThisWeek: 50,
		TopActivityTypes: []activities.ActivityTypeCount{
			{ActivityType: "auth_login", Count: 30},
			{ActivityType: "prompt_created", Count: 20},
		},
		TopEntityTypes: []activities.EntityTypeCount{
			{EntityType: "user", Count: 40},
			{EntityType: "prompt", Count: 30},
		},
		RecentActivities: []activities.Activity{
			{
				ID:           "activity-1",
				UserID:       userID,
				ActivityType: "auth_login",
				EntityType:   "user",
				Description:  "User logged in",
				CreatedAt:    time.Now(),
			},
		},
		ActivitiesByDateWeek: []activities.ActivityCountByDate{
			{Date: "2024-01-01", Count: 10},
			{Date: "2024-01-02", Count: 15},
		},
	}

	mockContainer.activityService.On("GetActivityStats", mock.Anything, userID).
		Return(expectedStats, nil)

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("GET", "/api/v1/activities/stats", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "Activity statistics retrieved successfully", response["message"])

	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(100), data["total_activities"])
	assert.Equal(t, float64(10), data["activities_today"])
	assert.Equal(t, float64(50), data["activities_this_week"])

	topActivityTypes := data["top_activity_types"].([]interface{})
	assert.Len(t, topActivityTypes, 2)

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleGetActivityStats_ServiceError tests error handling for stats retrieval
func TestHandleGetActivityStats_ServiceError(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"

	mockContainer.activityService.On("GetActivityStats", mock.Anything, userID).
		Return((*activities.ActivityStatsResponse)(nil), errors.New("database error"))

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("GET", "/api/v1/activities/stats", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleCreateActivity_Success tests successful activity creation
func TestHandleCreateActivity_Success(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"
	activityType := "prompt_created"
	entityType := "prompt"
	entityID := "prompt-456"

	reqBody := activities.CreateActivityRequest{
		ActivityType: activityType,
		EntityType:   entityType,
		EntityID:     &entityID,
		Description:  "Test prompt created",
	}

	expectedActivity := &activities.Activity{
		ID:           "activity-new",
		UserID:       userID,
		ActivityType: activityType,
		EntityType:   entityType,
		EntityID:     &entityID,
		Description:  "Test prompt created",
		CreatedAt:    time.Now(),
	}

	mockContainer.activityService.On(
		"RecordActivity", mock.Anything, userID,
		mock.MatchedBy(func(req activities.CreateActivityRequest) bool {
			return req.ActivityType == activityType &&
				req.EntityType == entityType &&
				req.Description == "Test prompt created"
		})).Return(expectedActivity, nil)

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("POST", "/api/v1/activities", reqBody, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "Activity created successfully", response["message"])

	data := response["data"].(map[string]interface{})
	assert.Equal(t, "activity-new", data["id"])
	assert.Equal(t, activityType, data["activity_type"])
	assert.Equal(t, entityType, data["entity_type"])

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleCreateActivity_ValidationError tests validation errors
func TestHandleCreateActivity_ValidationError(t *testing.T) {
	tests := []struct {
		name          string
		reqBody       activities.CreateActivityRequest
		expectedError string
	}{
		{
			name: "Missing activity type",
			reqBody: activities.CreateActivityRequest{
				EntityType:  "user",
				Description: "Test",
			},
			expectedError: "activity type is required",
		},
		{
			name: "Missing entity type",
			reqBody: activities.CreateActivityRequest{
				ActivityType: "auth_login",
				Description:  "Test",
			},
			expectedError: "entity type is required",
		},
		{
			name: "Missing description",
			reqBody: activities.CreateActivityRequest{
				ActivityType: "auth_login",
				EntityType:   "user",
			},
			expectedError: "description is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockActivityContainer(t)
			srv := createTestActivityServer(mockContainer)

			req := makeAuthenticatedActivityRequest("POST", "/api/v1/activities", tt.reqBody, "user-123")
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			// RFC 9457 error format
			assert.Equal(t, "BAD_REQUEST", response["code"])
			assert.Equal(t, 400.0, response["status"])
			assert.Contains(t, response["detail"], tt.expectedError)
			assert.NotEmpty(t, response["timestamp"])
			// request_id is present (may be empty if middleware not invoked in test)
			assert.Contains(t, response, "request_id")
		})
	}
}

// TestHandleCreateActivity_ServiceError tests error handling during creation
func TestHandleCreateActivity_ServiceError(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"

	reqBody := activities.CreateActivityRequest{
		ActivityType: "auth_login",
		EntityType:   "user",
		Description:  "Test activity",
	}

	mockContainer.activityService.On("RecordActivity", mock.Anything, userID, mock.Anything).
		Return((*activities.Activity)(nil), errors.New("database error"))

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("POST", "/api/v1/activities", reqBody, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleGetActivityTypes_Success tests successful retrieval of activity types
func TestHandleGetActivityTypes_Success(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"

	expectedResponse := &activities.ActivityTypesResponse{
		ActivityTypes: []string{
			"auth_login",
			"auth_logout",
			"prompt_created",
			"prompt_updated",
		},
		EntityTypes: []string{
			"user",
			"prompt",
			"session",
		},
	}

	mockContainer.activityService.On("GetAllTypes").
		Return(expectedResponse)

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("GET", "/api/v1/activities/types", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "Activity and entity types retrieved successfully", response["message"])

	data := response["data"].(map[string]interface{})
	activityTypes := data["activity_types"].([]interface{})
	assert.Len(t, activityTypes, 4)
	assert.Equal(t, "auth_login", activityTypes[0])

	entityTypes := data["entity_types"].([]interface{})
	assert.Len(t, entityTypes, 3)
	assert.Equal(t, "user", entityTypes[0])

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleGetEntityTypes_Success tests successful retrieval of entity types
func TestHandleGetEntityTypes_Success(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"

	expectedEntityTypes := []string{
		"user",
		"api_key",
		"prompt",
		"artifact",
		"session",
		"agent",
		"memory",
		"system",
	}

	mockContainer.activityService.On("GetEntityTypes").
		Return(expectedEntityTypes)

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("GET", "/api/v1/activities/entity-types", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "Activity entity types retrieved successfully", response["message"])

	data := response["data"].([]interface{})
	assert.Len(t, data, 8)
	assert.Equal(t, "user", data[0])
	assert.Equal(t, "api_key", data[1])

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleGetActivities_WithSearchFilter tests activities retrieval with search filter
func TestHandleGetActivities_WithSearchFilter(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"
	searchTerm := "login"

	mockContainer.activityService.On(
		"GetActivities", mock.Anything,
		mock.MatchedBy(func(filters activities.ActivityFilters) bool {
			return *filters.UserID == userID && *filters.Search == searchTerm
		})).Return(&activities.ActivityListResponse{
		Activities: []activities.Activity{
			{
				ID:           "activity-1",
				UserID:       userID,
				ActivityType: "auth_login",
				EntityType:   "user",
				Description:  "User login successful",
				CreatedAt:    time.Now(),
			},
		},
		TotalCount: 1,
		Page:       1,
		PerPage:    25,
		TotalPages: 1,
	}, nil)

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("GET", "/api/v1/activities?search=login", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	data := response["data"].(map[string]interface{})
	activities := data["activities"].([]interface{})
	assert.Len(t, activities, 1)

	activity := activities[0].(map[string]interface{})
	assert.Contains(t, activity["description"], "login")

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleGetActivities_WithSessionIDFilter tests activities retrieval filtered by session_id
func TestHandleGetActivities_WithSessionIDFilter(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"
	sessionID := "session-789"

	mockContainer.activityService.On(
		"GetActivities", mock.Anything,
		mock.MatchedBy(func(filters activities.ActivityFilters) bool {
			return *filters.UserID == userID && *filters.SessionID == sessionID
		})).Return(&activities.ActivityListResponse{
		Activities: []activities.Activity{
			{
				ID:           "activity-1",
				UserID:       userID,
				ActivityType: "claude_code_session",
				EntityType:   "session",
				SessionID:    &sessionID,
				Description:  "Claude Code session activity",
				CreatedAt:    time.Now(),
			},
		},
		TotalCount: 1,
		Page:       1,
		PerPage:    25,
		TotalPages: 1,
	}, nil)

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("GET", "/api/v1/activities?session_id=session-789", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	data := response["data"].(map[string]interface{})
	activities := data["activities"].([]interface{})
	assert.Len(t, activities, 1)

	activity := activities[0].(map[string]interface{})
	assert.Equal(t, sessionID, activity["session_id"])

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleCreateActivity_WithMetadata tests activity creation with metadata
func TestHandleCreateActivity_WithMetadata(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"

	metadata := map[string]interface{}{
		"source":  "api",
		"version": "1.0",
	}

	reqBody := activities.CreateActivityRequest{
		ActivityType: "prompt_created",
		EntityType:   "prompt",
		Description:  "Prompt created via API",
		Metadata:     metadata,
	}

	expectedActivity := &activities.Activity{
		ID:           "activity-new",
		UserID:       userID,
		ActivityType: "prompt_created",
		EntityType:   "prompt",
		Description:  "Prompt created via API",
		Metadata:     metadata,
		CreatedAt:    time.Now(),
	}

	mockContainer.activityService.On(
		"RecordActivity", mock.Anything, userID,
		mock.MatchedBy(func(req activities.CreateActivityRequest) bool {
			return req.Metadata != nil && req.Metadata["source"] == "api"
		})).Return(expectedActivity, nil)

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("POST", "/api/v1/activities", reqBody, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	data := response["data"].(map[string]interface{})
	activityMetadata := data["metadata"].(map[string]interface{})
	assert.Equal(t, "api", activityMetadata["source"])

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleGetActivities_EmptyResult tests activities retrieval with no results
func TestHandleGetActivities_EmptyResult(t *testing.T) {
	mockContainer := newMockActivityContainer(t)

	userID := "user-123"

	mockContainer.activityService.On("GetActivities", mock.Anything, mock.Anything).
		Return(&activities.ActivityListResponse{
			Activities: []activities.Activity{},
			TotalCount: 0,
			Page:       1,
			PerPage:    25,
			TotalPages: 0,
		}, nil)

	srv := createTestActivityServer(mockContainer)
	req := makeAuthenticatedActivityRequest("GET", "/api/v1/activities", nil, userID)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(0), data["total_count"])

	activities := data["activities"].([]interface{})
	assert.Len(t, activities, 0)

	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleActivityRetentionJob_Success verifies 200 response when RunRetentionJob succeeds.
func TestHandleActivityRetentionJob_Success(t *testing.T) {
	mockContainer := newMockActivityContainer(t)
	mockContainer.activityService.On("RunRetentionJob", mock.Anything).Return(nil).Once()

	srv := createTestActivityServer(mockContainer)
	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/activities/retention", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK", w.Body.String())
	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleActivityRetentionJob_ServiceError verifies 500 response when RunRetentionJob fails.
func TestHandleActivityRetentionJob_ServiceError(t *testing.T) {
	mockContainer := newMockActivityContainer(t)
	mockContainer.activityService.On("RunRetentionJob", mock.Anything).
		Return(errors.New("db unavailable")).Once()

	srv := createTestActivityServer(mockContainer)
	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/activities/retention", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockContainer.activityService.AssertExpectations(t)
}

// TestHandleActivityRetentionJob_InvokedOnce verifies RunRetentionJob is called exactly once.
func TestHandleActivityRetentionJob_InvokedOnce(t *testing.T) {
	mockContainer := newMockActivityContainer(t)
	mockContainer.activityService.On("RunRetentionJob", mock.Anything).Return(nil).Once()

	srv := createTestActivityServer(mockContainer)
	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/activities/retention", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	mockContainer.activityService.AssertNumberOfCalls(t, "RunRetentionJob", 1)
	assert.Equal(t, http.StatusOK, w.Code)
}
