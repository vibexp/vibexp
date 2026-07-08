package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// Helper to add chi URL params to request context
func addURLParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for key, value := range params {
		rctx.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// TestHandleGetArtifact_Success_WithMockedService tests successful artifact retrieval
func TestHandleGetArtifact_Success_WithMockedService(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)

	expectedArtifact := &models.Artifact{
		ID:          "art-1",
		ProjectID:   "550e8400-e29b-41d4-a716-446655440000",
		Slug:        "test-slug",
		Title:       "Test Artifact",
		Content:     "Test Content",
		Description: "Test Description",
		UserID:      "user-123",
		Type:        "general",
		Status:      "active",
		Metadata:    map[string]interface{}{"key": "value"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockArtifactService.On(
		"GetArtifactByProjectIDAndSlug",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"test-slug",
	).Return(expectedArtifact, nil)

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock: mockArtifactService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/test-project/test-slug"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id":    "550e8400-e29b-41d4-a716-446655440000",
		"project_id": "550e8400-e29b-41d4-a716-446655440000",
		"slug":       "test-slug",
	})
	rr := httptest.NewRecorder()

	srv.handleGetArtifact(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response models.Artifact
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "art-1", response.ID)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", response.ProjectID)
	assert.Equal(t, "test-slug", response.Slug)
	assert.Equal(t, "Test Artifact", response.Title)
	assert.Equal(t, "Test Content", response.Content)

	mockArtifactService.AssertExpectations(t)
}

// TestHandleGetArtifact_NotFound tests artifact not found scenario
func TestHandleGetArtifact_NotFound(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)

	mockArtifactService.On(
		"GetArtifactByProjectIDAndSlug",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"non-existent",
	).Return((*models.Artifact)(nil), errors.New("artifact not found"))

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock: mockArtifactService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/test-project/non-existent"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id":    "550e8400-e29b-41d4-a716-446655440000",
		"project_id": "550e8400-e29b-41d4-a716-446655440000",
		"slug":       "non-existent",
	})
	rr := httptest.NewRecorder()

	srv.handleGetArtifact(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	mockArtifactService.AssertExpectations(t)
}

// TestHandleGetArtifactsByProject_Success tests listing artifacts by project
func TestHandleGetArtifactsByProject_Success(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)

	teamID := "550e8400-e29b-41d4-a716-446655440000"
	expectedArtifacts := []models.Artifact{
		{ID: "art-1", ProjectID: "550e8400-e29b-41d4-a716-446655440000", Slug: "slug-1", Title: "Artifact 1", UserID: "user-123", Type: "general", Status: "active", Metadata: map[string]interface{}{}},
		{ID: "art-2", ProjectID: "550e8400-e29b-41d4-a716-446655440000", Slug: "slug-2", Title: "Artifact 2", UserID: "user-123", Type: "general", Status: "active", Metadata: map[string]interface{}{}},
	}

	expectedResponse := &models.ArtifactListResponse{
		Artifacts:  expectedArtifacts,
		TotalCount: 2,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}

	mockArtifactService.On("ListArtifacts", "user-123", mock.MatchedBy(func(filters services.ArtifactFilters) bool {
		return filters.ProjectID == "550e8400-e29b-41d4-a716-446655440000" && filters.TeamID == teamID
	})).Return(expectedResponse, nil)

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock: mockArtifactService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/test-project"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id":    "550e8400-e29b-41d4-a716-446655440000",
		"project_id": "550e8400-e29b-41d4-a716-446655440000",
	})
	rr := httptest.NewRecorder()

	srv.handleListArtifactsByProject(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	var response models.ArtifactListResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, 2, response.TotalCount)
	assert.Len(t, response.Artifacts, 2)

	mockArtifactService.AssertExpectations(t)
}

// TestHandleCreateArtifact_Success tests successful artifact creation
//
//nolint:funlen // Comprehensive test with mocked service
func TestHandleCreateArtifact_Success(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)
	mockResourceService := servicesmocks.NewMockResourceUsageServiceInterface(t)

	expectedArtifact := &models.Artifact{
		ID:          "art-new",
		ProjectID:   "550e8400-e29b-41d4-a716-446655440000",
		Slug:        "new-slug",
		Title:       "New Artifact",
		Content:     "New Content",
		Description: "New Description",
		UserID:      "user-123",
		Type:        "general",
		Status:      "active",
		Metadata:    map[string]interface{}{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Mock resource limit check
	mockResourceService.On("CheckResourceLimit", mock.Anything, "user-123", "artifact").
		Return(true, nil)

	// Note: team_id now comes from URL parameter (validated by middleware), no longer from user's default team
	teamID := "550e8400-e29b-41d4-a716-446655440000"

	mockArtifactService.On(
		"CreateArtifact",
		"user-123",
		teamID,
		mock.MatchedBy(func(req *models.CreateArtifactRequest) bool {
			return req.ProjectID == "550e8400-e29b-41d4-a716-446655440000" &&
				req.Slug == "new-slug" &&
				req.Title == "New Artifact" &&
				req.Content == "New Content"
		}),
	).Return(expectedArtifact, nil)

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock:      mockArtifactService,
		ResourceUsageServiceMock: mockResourceService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	reqBody := `{
		"project_id": "550e8400-e29b-41d4-a716-446655440000",
		"slug": "new-slug",
		"title": "New Artifact",
		"content": "New Content",
		"description": "New Description",
		"type": "general",
		"status": "active"
	}`

	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts"
	req := createAuthenticatedRequest("POST", url, reqBody, "user-123")
	req = addURLParams(req, map[string]string{"team_id": "550e8400-e29b-41d4-a716-446655440000"})
	rr := httptest.NewRecorder()

	srv.handleCreateArtifact(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	var response models.Artifact
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "art-new", response.ID)
	assert.Equal(t, "new-slug", response.Slug)
	assert.Equal(t, "New Artifact", response.Title)

	mockArtifactService.AssertExpectations(t)
	mockResourceService.AssertExpectations(t)
}

// TestHandleCreateArtifact_InvalidType verifies the handler rejects a type that
// is not one of the team's types (the TypeService-backed validation, #1846).
func TestHandleCreateArtifact_InvalidType(t *testing.T) {
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	mockType := servicesmocks.NewMockTypeServiceInterface(t)
	mockType.EXPECT().ValidateType(mock.Anything, teamID, "artifacts", "nonexistent").
		Return(false, nil)

	mockContainer := &MockArtifactContainer{TypeServiceMock: mockType}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	reqBody := `{
		"project_id": "550e8400-e29b-41d4-a716-446655440000",
		"slug": "new-slug",
		"title": "New Artifact",
		"content": "New Content",
		"type": "nonexistent"
	}`
	url := "/api/v1/" + teamID + "/artifacts"
	req := createAuthenticatedRequest("POST", url, reqBody, "user-123")
	req = addURLParams(req, map[string]string{"team_id": teamID})
	rr := httptest.NewRecorder()

	srv.handleCreateArtifact(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestHandleCreateArtifact_ValidationError tests validation errors
//
//nolint:funlen // Test function requires comprehensive setup for validation scenarios
func TestHandleCreateArtifact_ValidationError(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)
	mockResourceService := servicesmocks.NewMockResourceUsageServiceInterface(t)

	// Mock resource limit check - always allow
	mockResourceService.On("CheckResourceLimit", mock.Anything, "user-123", "artifact").
		Return(true, nil).Maybe()

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock:      mockArtifactService,
		ResourceUsageServiceMock: mockResourceService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	testCases := []struct {
		name           string
		body           string
		expectedStatus int
	}{
		{
			name: "Missing slug",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"title": "Test", "content": "Content"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Missing title",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test-slug", "content": "Content"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Missing content",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test-slug", "title": "Test"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Title too long",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test-slug", "title": "` + strings.Repeat("a", 256) + `", ` +
				`"content": "Content"}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts"
			req := createAuthenticatedRequest("POST", url, tc.body, "user-123")
			req = addURLParams(req, map[string]string{"team_id": "550e8400-e29b-41d4-a716-446655440000"})
			rr := httptest.NewRecorder()

			srv.handleCreateArtifact(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
		})
	}
}

// TestHandleCreateArtifact_ResourceLimitExceeded tests resource limit exceeded
func TestHandleCreateArtifact_ResourceLimitExceeded(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)
	mockResourceService := servicesmocks.NewMockResourceUsageServiceInterface(t)

	// Mock resource limit check - limit exceeded
	mockResourceService.On("CheckResourceLimit", mock.Anything, "user-123", "artifact").
		Return(false, nil)

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock:      mockArtifactService,
		ResourceUsageServiceMock: mockResourceService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	reqBody := `{
		"slug": "new-slug",
		"project_id": "550e8400-e29b-41d4-a716-446655440000", "slug": "test-slug", "title": "New Artifact",
		"content": "New Content"
	}`

	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts"
	req := createAuthenticatedRequest("POST", url, reqBody, "user-123")
	req = addURLParams(req, map[string]string{"team_id": "550e8400-e29b-41d4-a716-446655440000"})
	rr := httptest.NewRecorder()

	srv.handleCreateArtifact(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	mockResourceService.AssertExpectations(t)
}

// TestHandleUpdateArtifact_Success tests successful artifact update
func TestHandleUpdateArtifact_Success(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)
	mockResourceService := servicesmocks.NewMockResourceUsageServiceInterface(t)

	updatedArtifact := &models.Artifact{
		ID:        "art-1",
		ProjectID: "550e8400-e29b-41d4-a716-446655440000",
		Slug:      "test-slug",
		Title:     "Updated Title",
		Content:   "Updated Content",
		UserID:    "user-123",
		Type:      "general",
		Status:    "active",
		Metadata:  map[string]interface{}{},
		UpdatedAt: time.Now(),
	}

	// Mock resource limit check
	mockResourceService.On("CheckResourceLimit", mock.Anything, "user-123", "artifact").
		Return(true, nil)

	mockArtifactService.On(
		"UpdateArtifactByProjectIDAndSlug",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"test-slug",
		mock.MatchedBy(func(req *models.UpdateArtifactRequest) bool {
			return req.Title != nil && *req.Title == "Updated Title"
		})).Return(updatedArtifact, nil)

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock:      mockArtifactService,
		ResourceUsageServiceMock: mockResourceService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	reqBody := `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
		`"slug": "test-slug", "title": "Updated Title", "content": "Updated Content"}`
	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/test-project/test-slug"
	req := createAuthenticatedRequest("PUT", url, reqBody, "user-123")
	req = addURLParams(req, map[string]string{
		"team_id":    "550e8400-e29b-41d4-a716-446655440000",
		"project_id": "550e8400-e29b-41d4-a716-446655440000",
		"slug":       "test-slug",
	})
	rr := httptest.NewRecorder()

	srv.handleUpdateArtifact(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	var response models.Artifact
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "Updated Title", response.Title)
	mockArtifactService.AssertExpectations(t)
	mockResourceService.AssertExpectations(t)
}

// TestHandleUpdateArtifact_NotFound tests update on non-existent artifact
func TestHandleUpdateArtifact_NotFound(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)
	mockResourceService := servicesmocks.NewMockResourceUsageServiceInterface(t)

	// Mock resource limit check
	mockResourceService.On("CheckResourceLimit", mock.Anything, "user-123", "artifact").
		Return(true, nil)

	mockArtifactService.On(
		"UpdateArtifactByProjectIDAndSlug",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"non-existent",
		mock.Anything,
	).Return((*models.Artifact)(nil), errors.New("artifact not found"))

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock:      mockArtifactService,
		ResourceUsageServiceMock: mockResourceService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	reqBody := `{"project_id": "550e8400-e29b-41d4-a716-446655440000", "slug": "test-slug", "title": "Updated Title"}`
	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/test-project/non-existent"
	req := createAuthenticatedRequest("PUT", url, reqBody, "user-123")
	req = addURLParams(req, map[string]string{
		"team_id":    "550e8400-e29b-41d4-a716-446655440000",
		"project_id": "550e8400-e29b-41d4-a716-446655440000",
		"slug":       "non-existent",
	})
	rr := httptest.NewRecorder()

	srv.handleUpdateArtifact(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	mockArtifactService.AssertExpectations(t)
}

// TestHandleDeleteArtifact_Success tests successful artifact deletion
func TestHandleDeleteArtifact_Success(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)
	mockEmbeddingService := servicesmocks.NewMockEmbeddingServiceInterface(t)

	existingArtifact := &models.Artifact{
		ID:        "art-1",
		ProjectID: "550e8400-e29b-41d4-a716-446655440000",
		Slug:      "test-slug",
		Title:     "Test Artifact",
		UserID:    "user-123",
	}

	mockArtifactService.On(
		"GetArtifactByProjectIDAndSlug",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"test-slug",
	).Return(existingArtifact, nil)

	mockArtifactService.On(
		"DeleteArtifactByProjectIDAndSlug",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"test-slug",
	).Return(nil)

	// Mock embedding deletion (optional, can fail gracefully)
	mockEmbeddingService.On("DeleteEmbeddingsByEntity", "artifact", "art-1").
		Return(nil)

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock:  mockArtifactService,
		EmbeddingServiceMock: mockEmbeddingService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/test-project/test-slug"
	req := createAuthenticatedRequest("DELETE", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id":    "550e8400-e29b-41d4-a716-446655440000",
		"project_id": "550e8400-e29b-41d4-a716-446655440000",
		"slug":       "test-slug",
	})
	rr := httptest.NewRecorder()

	srv.handleDeleteArtifact(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	mockArtifactService.AssertExpectations(t)
}

// TestHandleDeleteArtifact_NotFound tests deleting non-existent artifact
func TestHandleDeleteArtifact_NotFound(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)

	mockArtifactService.On(
		"GetArtifactByProjectIDAndSlug",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"non-existent",
	).Return((*models.Artifact)(nil), errors.New("artifact not found"))

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock: mockArtifactService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/test-project/non-existent"
	req := createAuthenticatedRequest("DELETE", url, "", "user-123")
	req = addURLParams(req, map[string]string{
		"team_id":    "550e8400-e29b-41d4-a716-446655440000",
		"project_id": "550e8400-e29b-41d4-a716-446655440000",
		"slug":       "non-existent",
	})
	rr := httptest.NewRecorder()

	srv.handleDeleteArtifact(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	mockArtifactService.AssertExpectations(t)
}

// TestHandleGetArtifactStats_Success tests getting artifact statistics
func TestHandleGetArtifactStats_Success(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)

	expectedStats := &models.ArtifactStatsResponse{
		TotalProjects:  5,
		TotalArtifacts: 25,
		AddedThisWeek:  3,
		TotalByType: map[string]int{
			"general":         10,
			"work_reports":    8,
			"static_contexts": 7,
		},
		TotalByStatus: map[string]int{
			"active":   20,
			"draft":    3,
			"archived": 5,
		},
	}

	// Note: team_id comes from URL path (validated by middleware)
	teamID := "550e8400-e29b-41d4-a716-446655440000"

	mockArtifactService.On("GetArtifactStats", "user-123", teamID).
		Return(expectedStats, nil)

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock: mockArtifactService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/stats"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	req = addURLParams(req, map[string]string{"team_id": "550e8400-e29b-41d4-a716-446655440000"})
	rr := httptest.NewRecorder()

	srv.handleGetArtifactStats(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.ArtifactStatsResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, 5, response.TotalProjects)
	assert.Equal(t, 25, response.TotalArtifacts)
	assert.Equal(t, 3, response.AddedThisWeek)
	assert.Equal(t, 10, response.TotalByType["general"])
	assert.Equal(t, 20, response.TotalByStatus["active"])

	mockArtifactService.AssertExpectations(t)
}

// TestHandleGetArtifactStats_ServiceError tests stats service error
func TestHandleGetArtifactStats_ServiceError(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)

	// Note: team_id comes from URL path (validated by middleware)
	teamID := "550e8400-e29b-41d4-a716-446655440000"

	mockArtifactService.On("GetArtifactStats", "user-123", teamID).
		Return((*models.ArtifactStatsResponse)(nil), errors.New("database error"))

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock: mockArtifactService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/stats"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	req = addURLParams(req, map[string]string{"team_id": "550e8400-e29b-41d4-a716-446655440000"})
	rr := httptest.NewRecorder()

	srv.handleGetArtifactStats(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	mockArtifactService.AssertExpectations(t)
}

// NOTE: TestHandleGetArtifactStats_MissingTeamID removed
// Reason: team_id is now a required URL path parameter (not query string), validated by middleware
// The route is /api/v1/{team_id}/artifacts/stats, so team_id cannot be missing

// NOTE: TestHandleGetArtifactStats_InvalidTeamID removed
// Reason: team_id validation is now handled by artifactTeamValidationMiddleware
// This test would need to test the full middleware stack, not just the handler

// NOTE: TestHandleGetArtifactStats_UnauthorizedTeamAccess removed
// Reason: team access validation is now handled by artifactTeamValidationMiddleware
// This test would need to test the full middleware stack, not just the handler

// TestHandleGetArtifactProjects_Success tests getting artifact projects
