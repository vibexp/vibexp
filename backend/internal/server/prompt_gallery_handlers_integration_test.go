package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// MockPromptGalleryContainer implements Container interface for prompt gallery handler tests
type MockPromptGalleryContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	promptGalleryService *svcmocks.MockPromptGalleryServiceInterface
}

func (m *MockPromptGalleryContainer) PromptGalleryService() services.PromptGalleryServiceInterface {
	return m.promptGalleryService
}

func newMockPromptGalleryContainer(t *testing.T) *MockPromptGalleryContainer {
	return &MockPromptGalleryContainer{
		promptGalleryService: svcmocks.NewMockPromptGalleryServiceInterface(t),
	}
}

func createTestGalleryServer(container *MockPromptGalleryContainer) *Server {
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

	// Register gallery routes with middleware matching production setup
	r.Route("/api/v1/prompt-gallery", func(r chi.Router) {
		// Public GET endpoints - optional auth (matches production)
		r.Group(func(r chi.Router) {
			r.Use(srv.optionalAuthMiddleware)
			r.Get("/categories", srv.handleGetPromptGalleryCategories)
			r.Get("/prompts", srv.handleListPromptGalleryPrompts)
			r.Get("/prompts/{id}", srv.handleGetPromptGalleryPrompt)
		})

		// Protected POST endpoints - usage tracking requires authentication.
		// In production this group sits inside the flexibleAuth-wrapped route
		// group; the handler itself rejects requests with no user in context.
		r.Group(func(r chi.Router) {
			r.Post("/prompts/{id}/use", srv.handleTrackPromptGalleryUsage)
		})
	})

	return srv
}

func makeUnauthenticatedRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestGetPromptGalleryCategories_Success tests successful retrieval of gallery categories
func TestGetPromptGalleryCategories_Success(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedCategories := []models.PromptGalleryCategory{
		{Category: "development", Count: 10},
		{Category: "marketing", Count: 5},
		{Category: "writing", Count: 8},
	}

	mockContainer.promptGalleryService.On("GetCategories").
		Return(expectedCategories, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/categories")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []models.PromptGalleryCategory
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 3)
	assert.Equal(t, "development", response[0].Category)
	assert.Equal(t, 10, response[0].Count)

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestGetPromptGalleryCategories_EmptyList tests successful retrieval with no categories
func TestGetPromptGalleryCategories_EmptyList(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	mockContainer.promptGalleryService.On("GetCategories").
		Return([]models.PromptGalleryCategory{}, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/categories")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []models.PromptGalleryCategory
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 0)

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestGetPromptGalleryCategories_ServiceError tests handling of service errors
func TestGetPromptGalleryCategories_ServiceError(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	mockContainer.promptGalleryService.On("GetCategories").
		Return(([]models.PromptGalleryCategory)(nil), errors.New("database error"))

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/categories")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "INTERNAL_ERROR", response["code"])
	assert.Equal(t, "Failed to get categories", response["detail"])

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestListPromptGalleryPrompts_Success tests successful listing of gallery prompts
func TestListPromptGalleryPrompts_Success(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts: []models.PromptGalleryTemplate{
			{
				ID:          "gallery-1",
				Title:       "Test Prompt 1",
				Description: "A test gallery prompt",
				Content:     "This is test content",
				Category:    "development",
				Tags:        json.RawMessage(`["golang", "testing"]`),
			},
			{
				ID:          "gallery-2",
				Title:       "Test Prompt 2",
				Description: "Another test prompt",
				Content:     "More test content",
				Category:    "writing",
				Tags:        json.RawMessage(`["blog", "article"]`),
			},
		},
		TotalCount: 2,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}

	mockContainer.promptGalleryService.On("ListPrompts", "", "", ([]string)(nil), 1, 20).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PromptGalleryListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response.Prompts, 2)
	assert.Equal(t, 2, response.TotalCount)
	assert.Equal(t, 1, response.Page)

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestListPromptGalleryPrompts_WithCategoryFilter tests listing with category filter
func TestListPromptGalleryPrompts_WithCategoryFilter(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts: []models.PromptGalleryTemplate{
			{
				ID:       "gallery-1",
				Title:    "Dev Prompt",
				Category: "development",
			},
		},
		TotalCount: 1,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}

	mockContainer.promptGalleryService.On("ListPrompts", "development", "", ([]string)(nil), 1, 20).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts?category=development")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PromptGalleryListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response.Prompts, 1)
	assert.Equal(t, "development", response.Prompts[0].Category)

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestListPromptGalleryPrompts_WithSearchFilter tests listing with search filter
func TestListPromptGalleryPrompts_WithSearchFilter(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts: []models.PromptGalleryTemplate{
			{
				ID:          "gallery-1",
				Title:       "Testing Guide",
				Description: "A guide for testing",
			},
		},
		TotalCount: 1,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}

	mockContainer.promptGalleryService.On("ListPrompts", "", "testing", ([]string)(nil), 1, 20).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts?search=testing")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PromptGalleryListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response.Prompts, 1)

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestListPromptGalleryPrompts_WithTagsFilter tests listing with tags filter
func TestListPromptGalleryPrompts_WithTagsFilter(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts: []models.PromptGalleryTemplate{
			{
				ID:    "gallery-1",
				Title: "Golang Testing",
				Tags:  json.RawMessage(`["golang", "testing"]`),
			},
		},
		TotalCount: 1,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}

	mockContainer.promptGalleryService.On("ListPrompts", "", "", []string{"golang", "testing"}, 1, 20).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts?tags=golang,testing")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PromptGalleryListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response.Prompts, 1)

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestListPromptGalleryPrompts_WithPagination tests listing with pagination
func TestListPromptGalleryPrompts_WithPagination(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts:    []models.PromptGalleryTemplate{},
		TotalCount: 45,
		Page:       2,
		PerPage:    10,
		TotalPages: 5,
	}

	mockContainer.promptGalleryService.On("ListPrompts", "", "", ([]string)(nil), 2, 10).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts?page=2&limit=10")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PromptGalleryListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 2, response.Page)
	assert.Equal(t, 10, response.PerPage)
	assert.Equal(t, 45, response.TotalCount)
	assert.Equal(t, 5, response.TotalPages)

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestListPromptGalleryPrompts_EmptyResult tests listing with no results
func TestListPromptGalleryPrompts_EmptyResult(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts:    []models.PromptGalleryTemplate{},
		TotalCount: 0,
		Page:       1,
		PerPage:    20,
		TotalPages: 0,
	}

	mockContainer.promptGalleryService.On("ListPrompts", "", "", ([]string)(nil), 1, 20).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PromptGalleryListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response.Prompts, 0)
	assert.Equal(t, 0, response.TotalCount)

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestListPromptGalleryPrompts_ServiceError tests handling of service errors
func TestListPromptGalleryPrompts_ServiceError(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	mockContainer.promptGalleryService.On("ListPrompts", "", "", ([]string)(nil), 1, 20).
		Return((*models.PromptGalleryListResponse)(nil), errors.New("database error"))

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "INTERNAL_ERROR", response["code"])
	assert.Equal(t, "Failed to list prompts", response["detail"])

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestGetPromptGalleryPrompt_Success tests successful retrieval of a single gallery prompt
func TestGetPromptGalleryPrompt_Success(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedPrompt := &models.PromptGalleryTemplate{
		ID:          "gallery-123",
		Title:       "Test Prompt",
		Description: "A detailed test prompt",
		Content:     "This is the prompt content",
		Category:    "development",
		Tags:        json.RawMessage(`["golang", "api"]`),
		Metadata:    json.RawMessage(`{"author": "VibeXP Team", "version": "1.0"}`),
	}

	mockContainer.promptGalleryService.On("GetPromptByID", "gallery-123").
		Return(expectedPrompt, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts/gallery-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PromptGalleryTemplate
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "gallery-123", response.ID)
	assert.Equal(t, "Test Prompt", response.Title)
	assert.Equal(t, "development", response.Category)

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestGetPromptGalleryPrompt_NotFound tests retrieval of non-existent prompt
func TestGetPromptGalleryPrompt_NotFound(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	// Use valid UUID format but non-existent ID
	nonExistentID := "550e8400-e29b-41d4-a716-446655440000"
	// Production shape: PromptGalleryService.GetPromptByID wraps the
	// repository sentinel ("failed to get prompt: %w"); the old
	// string-equality handler only matched the bare sentinel text, so this
	// shape pins errors.Is.
	mockContainer.promptGalleryService.On("GetPromptByID", nonExistentID).
		Return((*models.PromptGalleryTemplate)(nil), fmt.Errorf("failed to get prompt: %w", repositories.ErrPromptNotFound))

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts/"+nonExistentID)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])
	assert.Equal(t, "Prompt not found", response["detail"])

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestGetPromptGalleryPrompt_MissingID tests retrieval without prompt ID
func TestGetPromptGalleryPrompt_MissingID(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts/")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	// This will be handled by the router and return 404 or similar
	assert.NotEqual(t, http.StatusOK, w.Code)
}

// TestGetPromptGalleryPrompt_ServiceError tests handling of service errors
func TestGetPromptGalleryPrompt_ServiceError(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	mockContainer.promptGalleryService.On("GetPromptByID", "gallery-123").
		Return((*models.PromptGalleryTemplate)(nil), errors.New("database error"))

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts/gallery-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "INTERNAL_ERROR", response["code"])

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestTrackPromptGalleryUsage_Success tests successful usage tracking
func TestTrackPromptGalleryUsage_Success(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	req := &models.PromptGalleryUsageRequest{
		PromptID: "gallery-123",
	}

	mockContainer.promptGalleryService.On("TrackPromptUsage", "user-123", req).
		Return(nil)

	srv := createTestGalleryServer(mockContainer)
	httpReq := makeAuthenticatedRequest("POST", "/api/v1/prompt-gallery/prompts/gallery-123/use", nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Usage tracked successfully", response["message"])

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestTrackPromptGalleryUsage_Unauthorized tests usage tracking without authentication
func TestTrackPromptGalleryUsage_Unauthorized(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("POST", "/api/v1/prompt-gallery/prompts/gallery-123/use")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	// The handler rejects requests with no user in context (AUTH_INVALID).
	assert.Equal(t, "AUTH_INVALID", response["code"])

	// Verify no service call was made
	mockContainer.promptGalleryService.AssertNotCalled(t, "TrackPromptUsage")
}

// TestTrackPromptGalleryUsage_PromptNotFound tests tracking usage of non-existent prompt
func TestTrackPromptGalleryUsage_PromptNotFound(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	// Use valid UUID format but non-existent ID
	nonExistentID := "550e8400-e29b-41d4-a716-446655440000"
	req := &models.PromptGalleryUsageRequest{
		PromptID: nonExistentID,
	}

	// Production shape: PromptGalleryService.TrackPromptUsage wraps the
	// repository sentinel ("failed to verify prompt: %w").
	mockContainer.promptGalleryService.On("TrackPromptUsage", "user-123", req).
		Return(fmt.Errorf("failed to verify prompt: %w", repositories.ErrPromptNotFound))

	srv := createTestGalleryServer(mockContainer)
	httpReq := makeAuthenticatedRequest("POST", "/api/v1/prompt-gallery/prompts/"+nonExistentID+"/use", nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, httpReq)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])
	assert.Equal(t, "Prompt not found", response["detail"])

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestTrackPromptGalleryUsage_MissingID tests tracking usage without prompt ID
func TestTrackPromptGalleryUsage_MissingID(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	srv := createTestGalleryServer(mockContainer)
	req := makeAuthenticatedRequest("POST", "/api/v1/prompt-gallery/prompts//use", nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	// This will be handled by the router and return 404 or similar
	assert.NotEqual(t, http.StatusOK, w.Code)
}

// TestTrackPromptGalleryUsage_ServiceError tests handling of service errors during tracking
func TestTrackPromptGalleryUsage_ServiceError(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	req := &models.PromptGalleryUsageRequest{
		PromptID: "gallery-123",
	}

	mockContainer.promptGalleryService.On("TrackPromptUsage", "user-123", req).
		Return(errors.New("database error"))

	srv := createTestGalleryServer(mockContainer)
	httpReq := makeAuthenticatedRequest("POST", "/api/v1/prompt-gallery/prompts/gallery-123/use", nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, httpReq)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "INTERNAL_ERROR", response["code"])
	assert.Equal(t, "Failed to track usage", response["detail"])

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestListPromptGalleryPrompts_NoAuthRequired tests that listing prompts doesn't require authentication
func TestListPromptGalleryPrompts_NoAuthRequired(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts: []models.PromptGalleryTemplate{
			{ID: "gallery-1", Title: "Public Prompt"},
		},
		TotalCount: 1,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}

	mockContainer.promptGalleryService.On("ListPrompts", "", "", ([]string)(nil), 1, 20).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	// Create request without authentication context
	req := httptest.NewRequest("GET", "/api/v1/prompt-gallery/prompts", nil)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestGetPromptGalleryPrompt_NoAuthRequired tests that getting a prompt doesn't require authentication
func TestGetPromptGalleryPrompt_NoAuthRequired(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedPrompt := &models.PromptGalleryTemplate{
		ID:    "gallery-123",
		Title: "Public Prompt",
	}

	mockContainer.promptGalleryService.On("GetPromptByID", "gallery-123").
		Return(expectedPrompt, nil)

	srv := createTestGalleryServer(mockContainer)
	// Create request without authentication context
	req := httptest.NewRequest("GET", "/api/v1/prompt-gallery/prompts/gallery-123", nil)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestGetPromptGalleryCategories_NoAuthRequired tests that getting categories doesn't require authentication
func TestGetPromptGalleryCategories_NoAuthRequired(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedCategories := []models.PromptGalleryCategory{
		{Category: "development", Count: 5},
	}

	mockContainer.promptGalleryService.On("GetCategories").
		Return(expectedCategories, nil)

	srv := createTestGalleryServer(mockContainer)
	// Create request without authentication context
	req := httptest.NewRequest("GET", "/api/v1/prompt-gallery/categories", nil)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestListPromptGalleryPrompts_CombinedFilters tests listing with multiple filters combined
func TestListPromptGalleryPrompts_CombinedFilters(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts: []models.PromptGalleryTemplate{
			{
				ID:       "gallery-1",
				Title:    "Golang API Testing",
				Category: "development",
				Tags:     json.RawMessage(`["golang", "api"]`),
			},
		},
		TotalCount: 1,
		Page:       1,
		PerPage:    10,
		TotalPages: 1,
	}

	mockContainer.promptGalleryService.On("ListPrompts", "development", "api", []string{"golang"}, 1, 10).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest(
		"GET",
		"/api/v1/prompt-gallery/prompts?category=development&search=api&tags=golang&limit=10",
	)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PromptGalleryListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response.Prompts, 1)
	assert.Equal(t, "development", response.Prompts[0].Category)

	mockContainer.promptGalleryService.AssertExpectations(t)
}

// testPaginationDefaults is a helper to test pagination default behavior
func testPaginationDefaults(t *testing.T, queryParams string, expectedPage, expectedLimit int) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts:    []models.PromptGalleryTemplate{},
		TotalCount: 0,
		Page:       expectedPage,
		PerPage:    expectedLimit,
		TotalPages: 0,
	}

	mockContainer.promptGalleryService.On("ListPrompts", "", "", ([]string)(nil), expectedPage, expectedLimit).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts?"+queryParams)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestListPromptGalleryPrompts_InvalidPagination tests listing with invalid pagination parameters
func TestListPromptGalleryPrompts_InvalidPagination(t *testing.T) {
	tests := []struct {
		name          string
		queryParams   string
		expectedPage  int
		expectedLimit int
	}{
		{name: "Negative page defaults to 1", queryParams: "page=-1", expectedPage: 1, expectedLimit: 20},
		{name: "Zero page defaults to 1", queryParams: "page=0", expectedPage: 1, expectedLimit: 20},
		{name: "Invalid page string defaults to 1", queryParams: "page=invalid", expectedPage: 1, expectedLimit: 20},
		{name: "Negative limit defaults to 20", queryParams: "limit=-1", expectedPage: 1, expectedLimit: 20},
		{name: "Zero limit defaults to 20", queryParams: "limit=0", expectedPage: 1, expectedLimit: 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testPaginationDefaults(t, tt.queryParams, tt.expectedPage, tt.expectedLimit)
		})
	}
}

// TestListPromptGalleryPrompts_TagsWithWhitespace tests that tags with whitespace are properly trimmed
func TestListPromptGalleryPrompts_TagsWithWhitespace(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts:    []models.PromptGalleryTemplate{},
		TotalCount: 0,
		Page:       1,
		PerPage:    20,
		TotalPages: 0,
	}

	// Tags should be trimmed: "golang ", " testing", " api " -> "golang", "testing", "api"
	mockContainer.promptGalleryService.On("ListPrompts", "", "", []string{"golang", "testing", "api"}, 1, 20).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	// URL encode the query parameter to avoid malformed HTTP request
	req := httptest.NewRequest("GET", "/api/v1/prompt-gallery/prompts?tags=golang+%2C+testing%2C+api+", nil)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestListPromptGalleryPrompts_MaxLimitEnforcement tests that the maximum limit
// is enforced to prevent resource exhaustion
func TestListPromptGalleryPrompts_MaxLimitEnforcement(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts:    []models.PromptGalleryTemplate{},
		TotalCount: 0,
		Page:       1,
		PerPage:    100, // Maximum enforced limit
		TotalPages: 0,
	}

	// Requesting limit=999999 should be capped at 100
	mockContainer.promptGalleryService.On("ListPrompts", "", "", ([]string)(nil), 1, 100).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts?limit=999999")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestGetPromptGalleryPrompt_InvalidUUIDFormat tests that invalid UUID format is rejected
func TestGetPromptGalleryPrompt_InvalidUUIDFormat(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts/invalid-uuid-format")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Invalid prompt ID format", response["detail"])
}

// TestTrackPromptGalleryUsage_InvalidUUIDFormat tests that invalid UUID format is rejected for usage tracking
func TestTrackPromptGalleryUsage_InvalidUUIDFormat(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	srv := createTestGalleryServer(mockContainer)
	// Create authenticated request
	req := httptest.NewRequest("POST", "/api/v1/prompt-gallery/prompts/malicious-input/use", nil)
	req.Header.Set("Content-Type", "application/json")
	// Add user context
	ctx := context.WithValue(req.Context(), contextKeyUserID, "user-123")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Invalid prompt ID format", response["detail"])
}

// TestListPromptGalleryPrompts_SQLInjectionAttempt tests protection against SQL injection
func TestListPromptGalleryPrompts_SQLInjectionAttempt(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts:    []models.PromptGalleryTemplate{},
		TotalCount: 0,
		Page:       1,
		PerPage:    20,
		TotalPages: 0,
	}

	// SQL injection attempt should be sanitized (truncated to max length)
	// URL decoding will convert %3B to semicolon
	sqlInjection := "' DROP TABLE prompts; --"
	mockContainer.promptGalleryService.On("ListPrompts", "", sqlInjection, ([]string)(nil), 1, 20).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts?search='+DROP+TABLE+prompts%3B+--")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// The service layer receives the input, but it should be parameterized at DB level
	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestListPromptGalleryPrompts_XSSAttempt tests that XSS payloads are handled safely
func TestListPromptGalleryPrompts_XSSAttempt(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts:    []models.PromptGalleryTemplate{},
		TotalCount: 0,
		Page:       1,
		PerPage:    20,
		TotalPages: 0,
	}

	// XSS attempt
	xssPayload := "<script>alert('xss')</script>"
	mockContainer.promptGalleryService.On("ListPrompts", "", xssPayload, ([]string)(nil), 1, 20).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts?search=%3Cscript%3Ealert('xss')%3C/script%3E")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Response should be properly encoded JSON (not executable)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestListPromptGalleryPrompts_ExcessivelyLongQuery tests that long queries are truncated
func TestListPromptGalleryPrompts_ExcessivelyLongQuery(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts:    []models.PromptGalleryTemplate{},
		TotalCount: 0,
		Page:       1,
		PerPage:    20,
		TotalPages: 0,
	}

	// Create a search query longer than max (255 chars)
	longQuery := strings.Repeat("a", 300)
	// Should be truncated to 255 characters
	truncatedQuery := strings.Repeat("a", 255)

	mockContainer.promptGalleryService.On("ListPrompts", "", truncatedQuery, ([]string)(nil), 1, 20).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts?search="+longQuery)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockContainer.promptGalleryService.AssertExpectations(t)
}

// TestListPromptGalleryPrompts_ExcessiveTags tests that excessive tags are limited
func TestListPromptGalleryPrompts_ExcessiveTags(t *testing.T) {
	mockContainer := newMockPromptGalleryContainer(t)

	expectedResponse := &models.PromptGalleryListResponse{
		Prompts:    []models.PromptGalleryTemplate{},
		TotalCount: 0,
		Page:       1,
		PerPage:    20,
		TotalPages: 0,
	}

	// Provide 15 tags, but only first 10 should be used
	expectedTags := []string{"tag1", "tag2", "tag3", "tag4", "tag5", "tag6", "tag7", "tag8", "tag9", "tag10"}

	mockContainer.promptGalleryService.On("ListPrompts", "", "", expectedTags, 1, 20).
		Return(expectedResponse, nil)

	srv := createTestGalleryServer(mockContainer)
	// 15 tags provided, only first 10 should be used
	tagsQuery := "tags=tag1,tag2,tag3,tag4,tag5,tag6,tag7,tag8,tag9,tag10,tag11,tag12,tag13,tag14,tag15"
	req := makeUnauthenticatedRequest("GET", "/api/v1/prompt-gallery/prompts?"+tagsQuery)
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockContainer.promptGalleryService.AssertExpectations(t)
}
