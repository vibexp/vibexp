package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// mockActivityService is a simple mock for ActivityService
type mockActivityService struct{}

func (m *mockActivityService) RecordActivity(
	ctx context.Context, userID string, req activities.CreateActivityRequest,
) (*activities.Activity, error) {
	return &activities.Activity{}, nil
}

func (m *mockActivityService) RecordAuthActivity(
	ctx context.Context, userID string, activityType string, sessionID *string,
	metadata map[string]interface{}, sourceIP *string, userAgent *string,
) error {
	return nil
}

func (m *mockActivityService) RecordResourceActivity(
	ctx context.Context, userID string, activityType string, entityType string,
	entityID *string, description string, metadata map[string]interface{},
) error {
	return nil
}

func (m *mockActivityService) RecordClaudeCodeActivity(
	ctx context.Context, userID string, sessionID string, toolName *string,
	hookEventName string, metadata map[string]interface{},
) error {
	return nil
}

func (m *mockActivityService) GetActivities(
	ctx context.Context, filters activities.ActivityFilters,
) (*activities.ActivityListResponse, error) {
	return &activities.ActivityListResponse{}, nil
}

func (m *mockActivityService) GetActivityByID(
	ctx context.Context, userID string, activityID string,
) (*activities.Activity, error) {
	return nil, nil
}

func (m *mockActivityService) GetActivityStats(
	ctx context.Context, userID string,
) (*activities.ActivityStatsResponse, error) {
	return &activities.ActivityStatsResponse{}, nil
}

func (m *mockActivityService) GetAllTypes() *activities.ActivityTypesResponse {
	return &activities.ActivityTypesResponse{}
}

func (m *mockActivityService) DeleteActivity(ctx context.Context, activityID string) error {
	return nil
}

func (m *mockActivityService) GetActivityTypes() []string {
	return []string{}
}

func (m *mockActivityService) GetEntityTypes() []string {
	return []string{}
}

func (m *mockActivityService) RunRetentionJob(_ context.Context) error {
	return nil
}

// MockMemoryContainer implements Container interface for memory handler tests
type MockMemoryContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	memoryService        *svcmocks.MockMemoryServiceInterface
	resourceUsageService *MockResourceUsageServiceForHandlers
	embeddingService     *svcmocks.MockEmbeddingServiceInterface
	activityService      *mockActivityService
	authService          *svcmocks.MockAuthServiceInterface
	teamService          *svcmocks.MockTeamServiceInterface
	projectRepository    *repomocks.MockProjectRepository
}

func (m *MockMemoryContainer) MemoryService() services.MemoryServiceInterface {
	return m.memoryService
}

func (m *MockMemoryContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return m.resourceUsageService
}

func (m *MockMemoryContainer) EmbeddingService() services.EmbeddingServiceInterface {
	return m.embeddingService
}

func (m *MockMemoryContainer) ActivityService() activities.ActivityService {
	return m.activityService
}

func (m *MockMemoryContainer) AuthService() services.AuthServiceInterface {
	return m.authService
}

func (m *MockMemoryContainer) TeamService() services.TeamServiceInterface {
	return m.teamService
}

func (m *MockMemoryContainer) ProjectRepository() repositories.ProjectRepository {
	return m.projectRepository
}

func newMockMemoryContainer(t *testing.T) *MockMemoryContainer {
	return &MockMemoryContainer{
		memoryService:        svcmocks.NewMockMemoryServiceInterface(t),
		resourceUsageService: &MockResourceUsageServiceForHandlers{},
		embeddingService:     svcmocks.NewMockEmbeddingServiceInterface(t),
		activityService:      &mockActivityService{},
		authService:          svcmocks.NewMockAuthServiceInterface(t),
		teamService:          svcmocks.NewMockTeamServiceInterface(t),
		projectRepository:    repomocks.NewMockProjectRepository(t),
	}
}

func createMemoryTestServer(container *MockMemoryContainer) *Server {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	// Initialize router manually for testing
	r := chi.NewRouter()

	srv := &Server{
		port:      "8080",
		container: container,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	// Register routes manually (simplified version for testing with team_id)
	r.Route("/api/v1/{team_id}/memories", func(r chi.Router) {
		r.Post("/", srv.handleCreateMemory)
		r.Get("/", srv.handleListMemories)
		r.Get("/search", srv.handleSearchMemoriesByMetadata)
		r.Get("/{id}", srv.handleGetMemory)
		r.Put("/{id}", srv.handleUpdateMemory)
		r.Delete("/{id}", srv.handleDeleteMemory)
	})

	return srv
}

//nolint:unparam // userID parameter is used with different values in different test scenarios
func makeMemoryAuthenticatedRequest(method, path string, body interface{}, userID string) *http.Request {
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

// addRouteParams adds chi URL parameters to the request context
func addRouteParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for key, value := range params {
		rctx.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

const testHandlerProjectID = "550e8400-e29b-41d4-a716-446655440003"

// TestHandleCreateMemory_Success tests successful memory creation with mocked service
func TestHandleCreateMemory_Success(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	now := time.Now()
	expectedMemory := &models.Memory{
		ID:        "memory-123",
		UserID:    "test-user-123",
		ProjectID: testHandlerProjectID,
		Text:      "Test memory text",
		Metadata:  map[string]interface{}{"category": "work"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Mock project repository: project belongs to team-123
	mockContainer.projectRepository.On("GetByID", mock.Anything, "test-user-123", testHandlerProjectID).
		Return(&models.Project{
			ID:     testHandlerProjectID,
			UserID: "test-user-123",
			TeamID: "team-123",
		}, nil)

	// Mock service expectations (team validation is done by middleware, not tested here)
	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "test-user-123", "memory").
		Return(true, nil)
	mockContainer.memoryService.On(
		"CreateMemory",
		"test-user-123",
		"team-123",
		mock.MatchedBy(func(req *models.CreateMemoryRequest) bool {
			return req != nil && req.ProjectID == testHandlerProjectID &&
				req.Text == "Test memory text" &&
				req.Metadata != nil && req.Metadata["category"] == "work"
		}),
	).Return(expectedMemory, nil)

	srv := createMemoryTestServer(mockContainer)
	reqBody := map[string]interface{}{
		"project_id": testHandlerProjectID,
		"text":       "Test memory text",
		"metadata":   map[string]interface{}{"category": "work"},
	}
	req := makeMemoryAuthenticatedRequest("POST", "/api/v1/team-123/memories", reqBody, "test-user-123")
	req = addRouteParams(req, map[string]string{"team_id": "team-123"})
	w := httptest.NewRecorder()

	srv.handleCreateMemory(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response models.Memory
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, expectedMemory.ID, response.ID)
	assert.Equal(t, expectedMemory.Text, response.Text)
	assert.Equal(t, testHandlerProjectID, response.ProjectID)
	assert.Equal(t, "work", response.Metadata["category"])

	mockContainer.memoryService.AssertExpectations(t)
	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestHandleCreateMemory_ValidationError tests memory creation with invalid input
func TestHandleCreateMemory_ValidationError(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)
	srv := createMemoryTestServer(mockContainer)

	tests := []struct {
		name    string
		payload string
	}{
		{"empty text", `{"project_id":"550e8400-e29b-41d4-a716-446655440003","text":"","metadata":{"category":"work"}}`},
		{"invalid JSON", `{invalid json}`},
		{"missing project_id", `{"text":"Some text"}`},
		{"invalid project_id format", `{"project_id":"not-a-uuid","text":"Some text"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/team-123/memories", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "test-user-123"))
			req = addRouteParams(req, map[string]string{"team_id": "team-123"})

			w := httptest.NewRecorder()
			srv.handleCreateMemory(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestHandleCreateMemory_ResourceLimitExceeded tests memory creation when resource limit is exceeded
func TestHandleCreateMemory_ResourceLimitExceeded(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	// Mock project repository: project belongs to the correct team
	mockContainer.projectRepository.On("GetByID", mock.Anything, "test-user-123", testHandlerProjectID).
		Return(&models.Project{
			ID:     testHandlerProjectID,
			UserID: "test-user-123",
			TeamID: "team-123",
		}, nil)

	// Mock resource limit exceeded
	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "test-user-123", "memory").
		Return(false, nil)

	srv := createMemoryTestServer(mockContainer)
	reqBody := map[string]interface{}{
		"project_id": testHandlerProjectID,
		"text":       "Test memory text",
	}
	req := makeMemoryAuthenticatedRequest("POST", "/api/v1/team-123/memories", reqBody, "test-user-123")
	req = addRouteParams(req, map[string]string{"team_id": "team-123"})
	w := httptest.NewRecorder()

	srv.handleCreateMemory(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestHandleGetMemory_Success tests successful memory retrieval with mocked service
func TestHandleGetMemory_Success(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	now := time.Now()
	expectedMemory := &models.Memory{
		ID:        "memory-123",
		UserID:    "test-user-123",
		Text:      "Test memory text",
		Metadata:  map[string]interface{}{"category": "work"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockContainer.memoryService.On("GetMemory", "test-user-123", mock.Anything, "memory-123").Return(expectedMemory, nil)

	srv := createMemoryTestServer(mockContainer)
	req := makeMemoryAuthenticatedRequest("GET", "/api/v1/team-123/memories/memory-123", nil, "test-user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": "team-123",
		"id":      "memory-123",
	})
	w := httptest.NewRecorder()

	srv.handleGetMemory(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.Memory
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, expectedMemory.ID, response.ID)
	assert.Equal(t, expectedMemory.Text, response.Text)

	mockContainer.memoryService.AssertExpectations(t)
}

// TestHandleGetMemory_NotFound tests memory retrieval when memory doesn't exist
func TestHandleGetMemory_NotFound(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	mockContainer.memoryService.On("GetMemory", "test-user-123", mock.Anything, "nonexistent-123").
		Return(nil, repositories.ErrMemoryNotFound)

	srv := createMemoryTestServer(mockContainer)
	req := makeMemoryAuthenticatedRequest("GET", "/api/v1/team-123/memories/nonexistent-123", nil, "test-user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": "team-123",
		"id":      "nonexistent-123",
	})
	w := httptest.NewRecorder()

	srv.handleGetMemory(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestHandleListMemories_Success tests successful memory listing with mocked service
func TestHandleListMemories_Success(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	now := time.Now()
	expectedResponse := &models.MemoryListResponse{
		Memories: []models.Memory{
			{
				ID:        "memory-1",
				UserID:    "test-user-123",
				Text:      "Memory 1",
				Metadata:  map[string]interface{}{"category": "work"},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:        "memory-2",
				UserID:    "test-user-123",
				Text:      "Memory 2",
				Metadata:  map[string]interface{}{"category": "personal"},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		TotalCount: 2,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}

	mockContainer.memoryService.On(
		"ListMemories",
		"test-user-123",
		mock.MatchedBy(func(filters services.MemoryFilters) bool {
			return filters.Page == 1 && filters.Limit == 10
		}),
	).Return(expectedResponse, nil)

	srv := createMemoryTestServer(mockContainer)
	req := makeMemoryAuthenticatedRequest("GET", "/api/v1/team-123/memories", nil, "test-user-123")
	req = addRouteParams(req, map[string]string{"team_id": "team-123"})
	w := httptest.NewRecorder()

	srv.handleListMemories(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.MemoryListResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, 2, response.TotalCount)
	assert.Len(t, response.Memories, 2)

	mockContainer.memoryService.AssertExpectations(t)
}

// TestHandleListMemories_WithFilters tests memory listing with pagination
func TestHandleListMemories_WithFilters(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	expectedResponse := &models.MemoryListResponse{
		Memories:   []models.Memory{},
		TotalCount: 0,
		Page:       2,
		PerPage:    10,
		TotalPages: 0,
	}

	mockContainer.memoryService.On(
		"ListMemories",
		"test-user-123",
		mock.MatchedBy(func(filters services.MemoryFilters) bool {
			return filters.Page == 2 && filters.Limit == 10
		}),
	).Return(expectedResponse, nil)

	srv := createMemoryTestServer(mockContainer)
	url := "/api/v1/team-123/memories?page=2&limit=10"
	req := makeMemoryAuthenticatedRequest("GET", url, nil, "test-user-123")
	req = addRouteParams(req, map[string]string{"team_id": "team-123"})
	w := httptest.NewRecorder()

	srv.handleListMemories(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.MemoryListResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, 2, response.Page)
	assert.Equal(t, 10, response.PerPage)

	mockContainer.memoryService.AssertExpectations(t)
}

// TestHandleUpdateMemory_Success tests successful memory update with mocked service
func TestHandleUpdateMemory_Success(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	now := time.Now()
	updatedMemory := &models.Memory{
		ID:        "memory-123",
		UserID:    "test-user-123",
		Text:      "Updated memory text",
		Metadata:  map[string]interface{}{"category": "work"},
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}

	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "test-user-123", "memory").
		Return(true, nil)
	mockContainer.memoryService.On(
		"UpdateMemory",
		"test-user-123",
		mock.Anything,
		"memory-123",
		mock.MatchedBy(func(req *models.UpdateMemoryRequest) bool {
			return req != nil && req.Text != nil && *req.Text == "Updated memory text"
		}),
	).Return(updatedMemory, nil)

	srv := createMemoryTestServer(mockContainer)
	reqBody := map[string]interface{}{
		"text": "Updated memory text",
	}
	req := makeMemoryAuthenticatedRequest("PUT", "/api/v1/team-123/memories/memory-123", reqBody, "test-user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": "team-123",
		"id":      "memory-123",
	})
	w := httptest.NewRecorder()

	srv.handleUpdateMemory(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.Memory
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, "Updated memory text", response.Text)

	mockContainer.memoryService.AssertExpectations(t)
	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestHandleUpdateMemory_NotFound tests memory update when memory doesn't exist
func TestHandleUpdateMemory_NotFound(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "test-user-123", "memory").
		Return(true, nil)
	mockContainer.memoryService.On("UpdateMemory", "test-user-123", mock.Anything, "nonexistent-123", mock.Anything).
		Return(nil, repositories.ErrMemoryNotFound)

	srv := createMemoryTestServer(mockContainer)
	reqBody := map[string]interface{}{
		"text": "Updated text",
	}
	req := makeMemoryAuthenticatedRequest("PUT", "/api/v1/team-123/memories/nonexistent-123", reqBody, "test-user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": "team-123",
		"id":      "nonexistent-123",
	})
	w := httptest.NewRecorder()

	srv.handleUpdateMemory(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestHandleDeleteMemory_Success tests successful memory deletion with mocked service
func TestHandleDeleteMemory_Success(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	mockContainer.memoryService.On("DeleteMemory", "test-user-123", mock.Anything, "memory-123").Return(nil)
	mockContainer.embeddingService.On("DeleteEmbeddingsByEntity", "memory", "memory-123").
		Return(nil)

	srv := createMemoryTestServer(mockContainer)
	req := makeMemoryAuthenticatedRequest("DELETE", "/api/v1/team-123/memories/memory-123", nil, "test-user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": "team-123",
		"id":      "memory-123",
	})
	w := httptest.NewRecorder()

	srv.handleDeleteMemory(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	mockContainer.memoryService.AssertExpectations(t)
	mockContainer.embeddingService.AssertExpectations(t)
}

// TestHandleDeleteMemory_NotFound tests memory deletion when memory doesn't exist
func TestHandleDeleteMemory_NotFound(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	mockContainer.memoryService.On("DeleteMemory", "test-user-123", mock.Anything, "nonexistent-123").
		Return(repositories.ErrMemoryNotFound)

	srv := createMemoryTestServer(mockContainer)
	req := makeMemoryAuthenticatedRequest("DELETE", "/api/v1/team-123/memories/nonexistent-123", nil, "test-user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": "team-123",
		"id":      "nonexistent-123",
	})
	w := httptest.NewRecorder()

	srv.handleDeleteMemory(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestHandleSearchMemoriesByMetadata_Success tests successful metadata search with mocked service
func TestHandleSearchMemoriesByMetadata_Success(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	now := time.Now()
	expectedResponse := &models.MemoryListResponse{
		Memories: []models.Memory{
			{
				ID:        "memory-1",
				UserID:    "test-user-123",
				Text:      "Memory with metadata",
				Metadata:  map[string]interface{}{"category": "work"},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		TotalCount: 1,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}

	mockContainer.memoryService.On(
		"SearchMemoriesByMetadata",
		"test-user-123",
		"category",
		"work",
		mock.MatchedBy(func(filters services.MemoryFilters) bool {
			return filters.Page == 1 && filters.Limit == 10
		}),
	).Return(expectedResponse, nil)

	srv := createMemoryTestServer(mockContainer)
	req := makeMemoryAuthenticatedRequest(
		"GET",
		"/api/v1/team-123/memories/search?metadata_key=category&metadata_value=work",
		nil,
		"test-user-123",
	)
	req = addRouteParams(req, map[string]string{"team_id": "team-123"})
	w := httptest.NewRecorder()

	srv.handleSearchMemoriesByMetadata(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.MemoryListResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, 1, response.TotalCount)
	assert.Equal(t, "work", response.Memories[0].Metadata["category"])

	mockContainer.memoryService.AssertExpectations(t)
}

// TestHandleSearchMemoriesByMetadata_MissingParameters tests metadata search with missing required params
func TestHandleSearchMemoriesByMetadata_MissingParameters(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)
	srv := createMemoryTestServer(mockContainer)

	tests := []struct {
		name  string
		query string
	}{
		{"missing both params", ""},
		{"missing value", "?metadata_key=category"},
		{"missing key", "?metadata_value=work"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := makeMemoryAuthenticatedRequest("GET", "/api/v1/team-123/memories/search"+tt.query, nil, "test-user-123")
			req = addRouteParams(req, map[string]string{"team_id": "team-123"})
			w := httptest.NewRecorder()
			srv.handleSearchMemoriesByMetadata(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestHandleSearchMemoriesByMetadata_WithPagination tests metadata search with pagination
func TestHandleSearchMemoriesByMetadata_WithPagination(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	expectedResponse := &models.MemoryListResponse{
		Memories:   []models.Memory{},
		TotalCount: 0,
		Page:       2,
		PerPage:    10,
		TotalPages: 0,
	}

	mockContainer.memoryService.On(
		"SearchMemoriesByMetadata",
		"test-user-123",
		"category",
		"work",
		mock.MatchedBy(func(filters services.MemoryFilters) bool {
			return filters.Page == 2 && filters.Limit == 10
		}),
	).Return(expectedResponse, nil)

	srv := createMemoryTestServer(mockContainer)
	url := "/api/v1/team-123/memories/search?metadata_key=category&metadata_value=work&page=2&limit=10"
	req := makeMemoryAuthenticatedRequest("GET", url, nil, "test-user-123")
	req = addRouteParams(req, map[string]string{"team_id": "team-123"})
	w := httptest.NewRecorder()

	srv.handleSearchMemoriesByMetadata(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.MemoryListResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, 2, response.Page)
	assert.Equal(t, 10, response.PerPage)

	mockContainer.memoryService.AssertExpectations(t)
}

// TestHandleListMemories_SortBy tests list memories with valid sort_by parameters
func TestHandleListMemories_SortBy(t *testing.T) {
	validSortFields := []string{"text", "updated_at", "created_at"}

	for _, sortField := range validSortFields {
		t.Run("sort_by="+sortField, func(t *testing.T) {
			mockContainer := newMockMemoryContainer(t)

			mockContainer.memoryService.On(
				"ListMemories",
				"test-user-123",
				mock.MatchedBy(func(filters services.MemoryFilters) bool {
					return filters.SortBy == sortField && filters.SortOrder == "asc"
				}),
			).Return(&models.MemoryListResponse{
				Memories:   []models.Memory{},
				TotalCount: 0,
				Page:       1,
				PerPage:    10,
				TotalPages: 0,
			}, nil)

			srv := createMemoryTestServer(mockContainer)
			url := "/api/v1/team-123/memories?sort_by=" + sortField + "&sort_order=asc"
			req := makeMemoryAuthenticatedRequest("GET", url, nil, "test-user-123")
			req = addRouteParams(req, map[string]string{"team_id": "team-123"})
			w := httptest.NewRecorder()

			srv.handleListMemories(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "sort_by=%s should return 200", sortField)
			mockContainer.memoryService.AssertExpectations(t)
		})
	}
}

// TestHandleListMemories_InvalidSortBy tests list memories with invalid sort_by returns 400
func TestHandleListMemories_InvalidSortBy(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	srv := createMemoryTestServer(mockContainer)
	url := "/api/v1/team-123/memories?sort_by=invalid_column"
	req := makeMemoryAuthenticatedRequest("GET", url, nil, "test-user-123")
	req = addRouteParams(req, map[string]string{"team_id": "team-123"})
	w := httptest.NewRecorder()

	srv.handleListMemories(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Contains(t, response["detail"], "invalid sort_by value")
}

// TestHandleListMemories_DefaultSort tests list memories with no sort params uses default ordering
func TestHandleListMemories_DefaultSort(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	mockContainer.memoryService.On(
		"ListMemories",
		"test-user-123",
		mock.MatchedBy(func(filters services.MemoryFilters) bool {
			return filters.SortBy == "" && filters.SortOrder == ""
		}),
	).Return(&models.MemoryListResponse{
		Memories:   []models.Memory{},
		TotalCount: 0,
		Page:       1,
		PerPage:    10,
		TotalPages: 0,
	}, nil)

	srv := createMemoryTestServer(mockContainer)
	req := makeMemoryAuthenticatedRequest("GET", "/api/v1/team-123/memories", nil, "test-user-123")
	req = addRouteParams(req, map[string]string{"team_id": "team-123"})
	w := httptest.NewRecorder()

	srv.handleListMemories(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockContainer.memoryService.AssertExpectations(t)
}

// TestHandleListMemories_SortOrderDesc tests list memories with desc sort order
func TestHandleListMemories_SortOrderDesc(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	mockContainer.memoryService.On(
		"ListMemories",
		"test-user-123",
		mock.MatchedBy(func(filters services.MemoryFilters) bool {
			return filters.SortBy == "text" && filters.SortOrder == "desc"
		}),
	).Return(&models.MemoryListResponse{
		Memories:   []models.Memory{},
		TotalCount: 0,
		Page:       1,
		PerPage:    10,
		TotalPages: 0,
	}, nil)

	srv := createMemoryTestServer(mockContainer)
	url := "/api/v1/team-123/memories?sort_by=text&sort_order=desc"
	req := makeMemoryAuthenticatedRequest("GET", url, nil, "test-user-123")
	req = addRouteParams(req, map[string]string{"team_id": "team-123"})
	w := httptest.NewRecorder()

	srv.handleListMemories(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockContainer.memoryService.AssertExpectations(t)
}

// TestHandleCreateMemory_CrossTeamProjectOwnership tests that creating a memory
// with a project_id belonging to a different team is rejected.
func TestHandleCreateMemory_CrossTeamProjectOwnership(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	// Project exists but belongs to a different team
	mockContainer.projectRepository.On("GetByID", mock.Anything, "test-user-123", testHandlerProjectID).
		Return(&models.Project{
			ID:     testHandlerProjectID,
			UserID: "test-user-123",
			TeamID: "other-team-999", // different team!
		}, nil)

	srv := createMemoryTestServer(mockContainer)
	reqBody := map[string]interface{}{
		"project_id": testHandlerProjectID,
		"text":       "Test memory text",
	}
	req := makeMemoryAuthenticatedRequest("POST", "/api/v1/team-123/memories", reqBody, "test-user-123")
	req = addRouteParams(req, map[string]string{"team_id": "team-123"})
	w := httptest.NewRecorder()

	srv.handleCreateMemory(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Contains(t, fmt.Sprintf("%v", response["detail"]), "does not belong to this team")
}

// TestHandleCreateMemory_NonExistentProject tests that creating a memory
// with a non-existent project_id returns 404.
func TestHandleCreateMemory_NonExistentProject(t *testing.T) {
	mockContainer := newMockMemoryContainer(t)

	// Project does not exist
	mockContainer.projectRepository.On("GetByID", mock.Anything, "test-user-123", testHandlerProjectID).
		Return(nil, fmt.Errorf("project not found for repository: id=%s", testHandlerProjectID))

	srv := createMemoryTestServer(mockContainer)
	reqBody := map[string]interface{}{
		"project_id": testHandlerProjectID,
		"text":       "Test memory text",
	}
	req := makeMemoryAuthenticatedRequest("POST", "/api/v1/team-123/memories", reqBody, "test-user-123")
	req = addRouteParams(req, map[string]string{"team_id": "team-123"})
	w := httptest.NewRecorder()

	srv.handleCreateMemory(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestHandleUpdateMemory_OnlyProjectID tests that updating a memory with only
// project_id set is allowed (validates C4 fix for the "at least one field" guard).
func TestHandleUpdateMemory_OnlyProjectID(t *testing.T) {
	const otherProjectID = "660e8400-e29b-41d4-a716-446655440099"
	mockContainer := newMockMemoryContainer(t)

	// Mock project ownership validation succeeds
	mockContainer.projectRepository.On("GetByID", mock.Anything, "test-user-123", otherProjectID).
		Return(&models.Project{
			ID:     otherProjectID,
			UserID: "test-user-123",
			TeamID: "team-123",
		}, nil)

	now := time.Now()
	updatedMemory := &models.Memory{
		ID:        "memory-123",
		UserID:    "test-user-123",
		ProjectID: otherProjectID,
		Text:      "Existing text",
		Metadata:  map[string]interface{}{},
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}

	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "test-user-123", "memory").
		Return(true, nil)
	mockContainer.memoryService.On(
		"UpdateMemory",
		"test-user-123",
		mock.Anything,
		"memory-123",
		mock.MatchedBy(func(req *models.UpdateMemoryRequest) bool {
			return req != nil && req.ProjectID != nil && *req.ProjectID == otherProjectID
		}),
	).Return(updatedMemory, nil)

	srv := createMemoryTestServer(mockContainer)
	reqBody := map[string]interface{}{
		"project_id": otherProjectID,
	}
	req := makeMemoryAuthenticatedRequest("PUT", "/api/v1/team-123/memories/memory-123", reqBody, "test-user-123")
	req = addRouteParams(req, map[string]string{
		"team_id": "team-123",
		"id":      "memory-123",
	})
	w := httptest.NewRecorder()

	srv.handleUpdateMemory(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockContainer.memoryService.AssertExpectations(t)
	mockContainer.projectRepository.AssertExpectations(t)
}
