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
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// MockPromptContainer implements Container interface for prompt handler tests
type MockPromptContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	promptService        *svcmocks.MockPromptServiceInterface
	resourceUsageService *MockResourceUsageServiceForHandlers
	embeddingService     *svcmocks.MockEmbeddingServiceInterface
	authService          *svcmocks.MockAuthServiceInterface
	teamService          *svcmocks.MockTeamServiceInterface
}

// Only override methods that return non-nil mocks
func (m *MockPromptContainer) PromptService() services.PromptServiceInterface {
	return m.promptService
}

func (m *MockPromptContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return m.resourceUsageService
}

func (m *MockPromptContainer) EmbeddingService() services.EmbeddingServiceInterface {
	return m.embeddingService
}

func (m *MockPromptContainer) AuthService() services.AuthServiceInterface {
	return m.authService
}

func (m *MockPromptContainer) TeamService() services.TeamServiceInterface {
	return m.teamService
}

func newMockPromptContainer(t *testing.T) *MockPromptContainer {
	return &MockPromptContainer{
		promptService:        svcmocks.NewMockPromptServiceInterface(t),
		resourceUsageService: &MockResourceUsageServiceForHandlers{},
		embeddingService:     svcmocks.NewMockEmbeddingServiceInterface(t),
		authService:          svcmocks.NewMockAuthServiceInterface(t),
		teamService:          svcmocks.NewMockTeamServiceInterface(t),
	}
}

// setupPromptDefaultTeamMock sets up auth and team service mocks for getUserDefaultTeamID
func setupPromptDefaultTeamMock(container *MockPromptContainer) {
	userID := "user-123"
	defaultTeamID := "550e8400-e29b-41d4-a716-446655440000"
	container.authService.On("GetUserByID", mock.Anything, userID).
		Return(&models.User{ID: userID, DefaultTeamID: &defaultTeamID}, nil).Maybe()
	container.teamService.On("IsUserMemberOfTeam", mock.Anything, userID, defaultTeamID).
		Return(true, nil).Maybe()
	container.teamService.On("GetTeam", mock.Anything, userID, defaultTeamID).
		Return(&models.Team{ID: defaultTeamID, Name: "Test Team"}, nil).Maybe()
}

func createTestServer(container *MockPromptContainer) *Server {
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
	r.Route("/api/v1/{team_id}/prompts", func(r chi.Router) {
		r.Use(srv.teamValidationMiddleware()) // Validate team_id from URL and team access
		r.Get("/", srv.handleListPrompts)
		r.Post("/", srv.handleCreatePrompt)
		r.Get("/labels", srv.handleGetPromptLabels)
		r.Get("/{slug}", srv.handleGetPrompt)
		r.Put("/{slug}", srv.handleUpdatePrompt)
		r.Delete("/{slug}", srv.handleDeletePrompt)
		r.Get("/{slug}/placeholders", srv.handleGetPromptPlaceholders)
		r.Post("/{slug}/render", srv.handleRenderPrompt)
	})

	return srv
}

//nolint:unparam // userID parameter is used with different values in different test scenarios
func makeAuthenticatedRequest(method, path string, body interface{}, userID string) *http.Request {
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

// TestHandleListPrompts_Success_WithMockedService tests successful list prompts with mocked service
func TestHandleListPrompts_Success_WithMockedService(t *testing.T) {
	mockContainer := newMockPromptContainer(t)

	mockContainer.teamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", "550e8400-e29b-41d4-a716-446655440000").
		Return(true, nil)

	mockContainer.promptService.On("ListPrompts", "user-123", mock.MatchedBy(func(filters services.PromptFilters) bool {
		return filters.UserID == "user-123" && filters.Page == 1 && filters.Limit == 10
	})).Return(&models.PromptListResponse{
		Prompts: []models.Prompt{
			{
				ID:          "prompt-1",
				Name:        "Test Prompt",
				Slug:        "test-prompt",
				Description: "A test prompt",
				Body:        "This is a test prompt body",
				Status:      "published",
				UserID:      "user-123",
				TeamID:      "550e8400-e29b-41d4-a716-446655440000",
				ProjectID:   "550e8400-e29b-41d4-a716-446655440099",
				// Labels nil on purpose: serializes as null (spec: nullable).
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Version:   1,
			},
		},
		TotalCount: 1,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}, nil)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts", nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "Prompts retrieved successfully", response["message"])

	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(1), data["total_count"])
	assert.Equal(t, float64(1), data["page"])
	assert.Equal(t, float64(20), data["per_page"])
	assert.Equal(t, float64(1), data["total_pages"])

	prompts := data["prompts"].([]interface{})
	assert.Len(t, prompts, 1)

	prompt := prompts[0].(map[string]interface{})
	assert.Equal(t, "prompt-1", prompt["id"])
	assert.Equal(t, "Test Prompt", prompt["name"])

	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleListPrompts_WithFilters tests list prompts with filters
func TestHandleListPrompts_WithFilters(t *testing.T) {
	mockContainer := newMockPromptContainer(t)

	mockContainer.teamService.On(
		"IsUserMemberOfTeam", mock.Anything, "user-123", "550e8400-e29b-41d4-a716-446655440000",
	).Return(true, nil)

	mockContainer.promptService.On("ListPrompts", "user-123", mock.MatchedBy(func(filters services.PromptFilters) bool {
		return filters.Status == "published" && filters.Search == "test" && filters.Page == 2 && filters.Limit == 10 &&
			filters.MCPExpose != nil && *filters.MCPExpose &&
			filters.IsShared != nil && !*filters.IsShared
	})).Return(&models.PromptListResponse{
		Prompts:    []models.Prompt{},
		TotalCount: 0,
		Page:       2,
		PerPage:    10,
		TotalPages: 0,
	}, nil)

	srv := createTestServer(mockContainer)
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	url := "/api/v1/" + teamID + "/prompts?status=published&search=test&page=2&limit=10&mcp_expose=true&shared=false"
	req := makeAuthenticatedRequest("GET", url, nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(0), data["total_count"])
	assert.Equal(t, float64(2), data["page"])

	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleGetPromptLabels_Success tests successful label retrieval and pins
// the wire shape: the legacy {status, message, data: {labels}} envelope.
func TestHandleGetPromptLabels_Success(t *testing.T) {
	mockContainer := newMockPromptContainer(t)

	teamID := "550e8400-e29b-41d4-a716-446655440000"
	mockContainer.teamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", teamID).
		Return(true, nil)

	mockContainer.promptService.On("GetUserLabels", "user-123").
		Return([]string{"code-review", "documentation"}, nil)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest("GET", "/api/v1/"+teamID+"/prompts/labels", nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)
	specconformance.AssertConformsToSpec(t, req, w)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, "Labels retrieved successfully", response["message"])

	data := response["data"].(map[string]interface{})
	labels := data["labels"].([]interface{})
	assert.Equal(t, []interface{}{"code-review", "documentation"}, labels)

	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleListPrompts_ServiceError tests list prompts when service returns error
func TestHandleListPrompts_ServiceError(t *testing.T) {
	mockContainer := newMockPromptContainer(t)

	mockContainer.teamService.On(
		"IsUserMemberOfTeam", mock.Anything, "user-123", "550e8400-e29b-41d4-a716-446655440000",
	).Return(true, nil)

	mockContainer.promptService.On("ListPrompts", "user-123", mock.Anything).
		Return((*models.PromptListResponse)(nil), errors.New("database error"))

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts", nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "INTERNAL_ERROR", response["code"])
	assert.Equal(t, "Failed to list prompts", response["detail"])

	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleGetPrompt_Success tests successful get prompt by slug
func TestHandleGetPrompt_Success(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	setupPromptDefaultTeamMock(mockContainer)

	expectedPrompt := &models.Prompt{
		ID:          "prompt-1",
		Name:        "Test Prompt",
		Slug:        "test-slug",
		Description: "A test prompt",
		Body:        "This is a test prompt body",
		Status:      "published",
		UserID:      "user-123",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockContainer.promptService.On("GetPromptBySlug", "user-123", mock.Anything, "test-slug").
		Return(expectedPrompt, nil)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest(
		"GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug", nil, "user-123",
	)
	w := httptest.NewRecorder()

	// We need to use ServeHTTP to get proper routing
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.Prompt
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedPrompt.ID, response.ID)
	assert.Equal(t, expectedPrompt.Name, response.Name)
	assert.Equal(t, expectedPrompt.Slug, response.Slug)

	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleGetPrompt_NotFound tests get prompt when prompt not found
func TestHandleGetPrompt_NotFound(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	setupPromptDefaultTeamMock(mockContainer)

	mockContainer.promptService.On("GetPromptBySlug", "user-123", mock.Anything, "non-existent").
		Return((*models.Prompt)(nil), repositories.ErrPromptNotFound)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest(
		"GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/non-existent", nil, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])
	assert.Equal(t, "Prompt not found", response["detail"])

	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleCreatePrompt_Success tests successful prompt creation
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestHandleCreatePrompt_Success(t *testing.T) {
	mockContainer := newMockPromptContainer(t)

	reqBody := &models.CreatePromptRequest{
		Name:        "New Prompt",
		Slug:        "new-prompt",
		Description: "A new prompt",
		Body:        "This is a new prompt body",
		Status:      "draft",
		ProjectID:   "project-123",
	}

	expectedPrompt := &models.Prompt{
		ID:          "prompt-new",
		Name:        reqBody.Name,
		Slug:        reqBody.Slug,
		Description: reqBody.Description,
		Body:        reqBody.Body,
		Status:      reqBody.Status,
		ProjectID:   reqBody.ProjectID,
		UserID:      "user-123",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Mock team access validation
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	mockContainer.teamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", teamID).
		Return(true, nil)

	// Mock resource limit check
	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "user-123", "prompt").
		Return(true, nil)

	mockContainer.promptService.On(
		"CreatePrompt",
		"user-123",
		teamID,
		mock.MatchedBy(func(req *models.CreatePromptRequest) bool {
			return req.Name == "New Prompt" && req.Slug == "new-prompt" && req.ProjectID == "project-123"
		}),
	).Return(expectedPrompt, nil)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest("POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response models.Prompt
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedPrompt.ID, response.ID)
	assert.Equal(t, expectedPrompt.Name, response.Name)
	assert.Equal(t, expectedPrompt.Slug, response.Slug)

	mockContainer.promptService.AssertExpectations(t)
	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestHandleCreatePrompt_ValidationError tests create prompt with validation errors
func TestHandleCreatePrompt_ValidationError(t *testing.T) {
	tests := []struct {
		name          string
		reqBody       *models.CreatePromptRequest
		expectedError string
	}{
		{
			name: "Missing name",
			reqBody: &models.CreatePromptRequest{
				Slug: "test",
				Body: "test body",
			},
			expectedError: "Name is required",
		},
		{
			name: "Missing slug",
			reqBody: &models.CreatePromptRequest{
				Name: "Test",
				Body: "test body",
			},
			expectedError: "Slug is required",
		},
		{
			name: "Missing body",
			reqBody: &models.CreatePromptRequest{
				Name: "Test",
				Slug: "test",
			},
			expectedError: "Body is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockPromptContainer(t)

			// Mock team access validation
			mockContainer.teamService.On(
				"IsUserMemberOfTeam", mock.Anything, "user-123", "550e8400-e29b-41d4-a716-446655440000",
			).Return(true, nil)

			srv := createTestServer(mockContainer)

			req := makeAuthenticatedRequest(
				"POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts", tt.reqBody, "user-123",
			)
			w := httptest.NewRecorder()

			srv.router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			assert.Equal(t, "VALIDATION_FAILED", response["code"])
			assert.Equal(t, tt.expectedError, response["detail"])
		})
	}
}

// TestHandleCreatePrompt_ResourceLimitExceeded tests create prompt when resource limit exceeded
func TestHandleCreatePrompt_ResourceLimitExceeded(t *testing.T) {
	mockContainer := newMockPromptContainer(t)

	reqBody := &models.CreatePromptRequest{
		Name:      "New Prompt",
		Slug:      "new-prompt",
		Body:      "This is a new prompt body",
		ProjectID: "project-123",
	}

	// Mock team access validation
	mockContainer.teamService.On(
		"IsUserMemberOfTeam", mock.Anything, "user-123", "550e8400-e29b-41d4-a716-446655440000",
	).Return(true, nil)

	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "user-123", "prompt").
		Return(false, nil)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest("POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_LIMIT_EXCEEDED", response["code"])

	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestHandleCreatePrompt_Conflict tests create prompt with duplicate slug
func TestHandleCreatePrompt_Conflict(t *testing.T) {
	mockContainer := newMockPromptContainer(t)

	reqBody := &models.CreatePromptRequest{
		Name:      "Duplicate Prompt",
		Slug:      "duplicate",
		Body:      "This is a duplicate prompt",
		ProjectID: "project-123",
	}

	// Mock team access validation
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	mockContainer.teamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", teamID).
		Return(true, nil)

	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "user-123", "prompt").
		Return(true, nil)

	mockContainer.promptService.On("CreatePrompt", "user-123", teamID, mock.Anything).
		Return((*models.Prompt)(nil), errors.New("prompt with slug 'duplicate' already exists for this user"))

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest("POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_EXISTS", response["code"])

	mockContainer.promptService.AssertExpectations(t)
	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestHandleUpdatePrompt_Success tests successful prompt update
func TestHandleUpdatePrompt_Success(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	setupPromptDefaultTeamMock(mockContainer)

	newName := "Updated Prompt"
	newStatus := "published"
	reqBody := &models.UpdatePromptRequest{
		Name:   &newName,
		Status: &newStatus,
	}

	updatedPrompt := &models.Prompt{
		ID:          "prompt-1",
		Name:        newName,
		Slug:        "test-slug",
		Description: "A test prompt",
		Body:        "This is a test prompt body",
		Status:      newStatus,
		UserID:      "user-123",
		CreatedAt:   time.Now().Add(-24 * time.Hour),
		UpdatedAt:   time.Now(),
	}

	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "user-123", "prompt").
		Return(true, nil)

	mockContainer.promptService.On("UpdatePromptBySlug", "user-123", mock.Anything, "test-slug", mock.Anything).
		Return(updatedPrompt, nil)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest(
		"PUT", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug", reqBody, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.Prompt
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, updatedPrompt.Name, response.Name)
	assert.Equal(t, updatedPrompt.Status, response.Status)

	mockContainer.promptService.AssertExpectations(t)
	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestHandleUpdatePrompt_NotFound tests update non-existent prompt
func TestHandleUpdatePrompt_NotFound(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	setupPromptDefaultTeamMock(mockContainer)

	newName := "Updated Prompt"
	reqBody := &models.UpdatePromptRequest{
		Name: &newName,
	}

	mockContainer.resourceUsageService.On("CheckResourceLimit", mock.Anything, "user-123", "prompt").
		Return(true, nil)

	mockContainer.promptService.On("UpdatePromptBySlug", "user-123", mock.Anything, "non-existent", mock.Anything).
		Return((*models.Prompt)(nil), repositories.ErrPromptNotFound)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest(
		"PUT", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/non-existent", reqBody, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])

	mockContainer.promptService.AssertExpectations(t)
	mockContainer.resourceUsageService.AssertExpectations(t)
}

// TestHandleDeletePrompt_Success tests successful prompt deletion
func TestHandleDeletePrompt_Success(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	setupPromptDefaultTeamMock(mockContainer)

	existingPrompt := &models.Prompt{
		ID:          "prompt-1",
		Name:        "Test Prompt",
		Slug:        "test-slug",
		Description: "A test prompt",
		Body:        "This is a test prompt body",
		Status:      "published",
		UserID:      "user-123",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockContainer.promptService.On("GetPromptBySlug", "user-123", mock.Anything, "test-slug").
		Return(existingPrompt, nil)

	mockContainer.promptService.On("DeletePromptBySlug", "user-123", mock.Anything, "test-slug").
		Return(nil)

	mockContainer.embeddingService.On("DeleteEmbeddingsByEntity", "prompt", "prompt-1").
		Return(nil)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest(
		"DELETE", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug", nil, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	mockContainer.promptService.AssertExpectations(t)
	mockContainer.embeddingService.AssertExpectations(t)
}

// TestHandleDeletePrompt_NotFound tests delete non-existent prompt
func TestHandleDeletePrompt_NotFound(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	setupPromptDefaultTeamMock(mockContainer)

	mockContainer.promptService.On("GetPromptBySlug", "user-123", mock.Anything, "non-existent").
		Return((*models.Prompt)(nil), repositories.ErrPromptNotFound)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest(
		"DELETE", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/non-existent", nil, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])

	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleGetPromptPlaceholders_Success tests successful get prompt placeholders
func TestHandleGetPromptPlaceholders_Success(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	setupPromptDefaultTeamMock(mockContainer)

	expectedPlaceholders := []string{"name", "age", "location"}

	mockContainer.promptService.On("GetPromptPlaceholders", "user-123", mock.Anything, "test-slug").
		Return(expectedPlaceholders, nil)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest(
		"GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug/placeholders", nil, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	placeholders := response["placeholders"].([]interface{})
	assert.Len(t, placeholders, 3)
	assert.Equal(t, "name", placeholders[0])
	assert.Equal(t, "age", placeholders[1])
	assert.Equal(t, "location", placeholders[2])

	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleGetPromptPlaceholders_NotFound tests get placeholders for non-existent prompt
func TestHandleGetPromptPlaceholders_NotFound(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	setupPromptDefaultTeamMock(mockContainer)

	mockContainer.promptService.On("GetPromptPlaceholders", "user-123", mock.Anything, "non-existent").
		Return(([]string)(nil), repositories.ErrPromptNotFound)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest(
		"GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/non-existent/placeholders", nil, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])

	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleRenderPrompt_Success tests successful prompt rendering
func TestHandleRenderPrompt_Success(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	setupPromptDefaultTeamMock(mockContainer)

	reqBody := &models.RenderPromptRequest{
		Placeholders: map[string]string{
			"name":     "John Doe",
			"age":      "30",
			"location": "New York",
		},
	}

	expectedResponse := &models.RenderPromptResponse{
		RenderedBody:   "Hello John Doe, you are 30 years old from New York",
		ReferencesUsed: nil,
	}

	mockContainer.promptService.On("RenderPrompt", "user-123", mock.Anything, "test-slug", reqBody.Placeholders).
		Return(expectedResponse, nil)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest(
		"POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug/render", reqBody, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.RenderPromptResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedResponse.RenderedBody, response.RenderedBody)
	// Check that references used is empty (nil or empty slice)
	assert.Empty(t, response.ReferencesUsed)

	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleRenderPrompt_WithReferences tests prompt rendering with references
func TestHandleRenderPrompt_WithReferences(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	setupPromptDefaultTeamMock(mockContainer)

	reqBody := &models.RenderPromptRequest{
		Placeholders: map[string]string{
			"topic": "Go programming",
		},
	}

	expectedResponse := &models.RenderPromptResponse{
		RenderedBody:   "Let's talk about Go programming. It's a compiled language.",
		ReferencesUsed: []string{"ref-slug-1", "ref-slug-2"},
	}

	mockContainer.promptService.On("RenderPrompt", "user-123", mock.Anything, "test-slug", reqBody.Placeholders).
		Return(expectedResponse, nil)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest(
		"POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug/render", reqBody, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.RenderPromptResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedResponse.RenderedBody, response.RenderedBody)
	assert.Len(t, response.ReferencesUsed, 2)
	assert.Contains(t, response.ReferencesUsed, "ref-slug-1")
	assert.Contains(t, response.ReferencesUsed, "ref-slug-2")

	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleRenderPrompt_NotFound tests render non-existent prompt
func TestHandleRenderPrompt_NotFound(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	setupPromptDefaultTeamMock(mockContainer)

	reqBody := &models.RenderPromptRequest{
		Placeholders: map[string]string{},
	}

	mockContainer.promptService.On("RenderPrompt", "user-123", mock.Anything, "non-existent", reqBody.Placeholders).
		Return((*models.RenderPromptResponse)(nil), repositories.ErrPromptNotFound)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest(
		"POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/non-existent/render", reqBody, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])

	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleRenderPrompt_RenderError tests render error scenarios
func TestHandleRenderPrompt_RenderError(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	setupPromptDefaultTeamMock(mockContainer)

	reqBody := &models.RenderPromptRequest{
		Placeholders: map[string]string{},
	}

	mockContainer.promptService.On("RenderPrompt", "user-123", mock.Anything, "test-slug", reqBody.Placeholders).
		Return((*models.RenderPromptResponse)(nil), errors.New("missing required placeholder: name"))

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest(
		"POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug/render", reqBody, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "render_error", response["code"])

	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleListPrompts_EmptyResult tests list prompts with no results
func TestHandleListPrompts_EmptyResult(t *testing.T) {
	mockContainer := newMockPromptContainer(t)

	mockContainer.teamService.On(
		"IsUserMemberOfTeam", mock.Anything, "user-123", "550e8400-e29b-41d4-a716-446655440000",
	).Return(true, nil)

	mockContainer.promptService.On("ListPrompts", "user-123", mock.Anything).
		Return(&models.PromptListResponse{
			Prompts:    []models.Prompt{},
			TotalCount: 0,
			Page:       1,
			PerPage:    20,
			TotalPages: 0,
		}, nil)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts", nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	data := response["data"].(map[string]interface{})
	assert.Equal(t, float64(0), data["total_count"])

	prompts := data["prompts"].([]interface{})
	assert.Len(t, prompts, 0)

	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleListPrompts_SortBy tests list prompts with valid sort_by parameters
func TestHandleListPrompts_SortBy(t *testing.T) {
	validSortFields := []string{"name", "status", "updated_at", "created_at"}

	for _, sortField := range validSortFields {
		t.Run("sort_by="+sortField, func(t *testing.T) {
			mockContainer := newMockPromptContainer(t)
			teamID := "550e8400-e29b-41d4-a716-446655440000"

			mockContainer.teamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", teamID).
				Return(true, nil)

			mockContainer.promptService.On("ListPrompts", "user-123", mock.MatchedBy(func(filters services.PromptFilters) bool {
				return filters.SortBy == sortField && filters.SortOrder == "asc" && filters.TeamID == teamID
			})).Return(&models.PromptListResponse{
				Prompts:    []models.Prompt{},
				TotalCount: 0,
				Page:       1,
				PerPage:    20,
				TotalPages: 0,
			}, nil)

			srv := createTestServer(mockContainer)
			url := "/api/v1/" + teamID + "/prompts?sort_by=" + sortField + "&sort_order=asc"
			req := makeAuthenticatedRequest("GET", url, nil, "user-123")
			w := httptest.NewRecorder()

			srv.router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "sort_by=%s should return 200", sortField)
			mockContainer.promptService.AssertExpectations(t)
		})
	}
}

// TestHandleListPrompts_InvalidSortBy tests list prompts with invalid sort_by returns 400
func TestHandleListPrompts_InvalidSortBy(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"

	mockContainer.teamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", teamID).
		Return(true, nil)

	srv := createTestServer(mockContainer)
	url := "/api/v1/" + teamID + "/prompts?sort_by=invalid_column"
	req := makeAuthenticatedRequest("GET", url, nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response["detail"], "invalid sort_by value")
}

// TestHandleListPrompts_DefaultSort tests list prompts with no sort params uses default ordering
func TestHandleListPrompts_DefaultSort(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"

	mockContainer.teamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", teamID).
		Return(true, nil)

	mockContainer.promptService.On("ListPrompts", "user-123", mock.MatchedBy(func(filters services.PromptFilters) bool {
		return filters.SortBy == "" && filters.SortOrder == "" && filters.TeamID == teamID
	})).Return(&models.PromptListResponse{
		Prompts:    []models.Prompt{},
		TotalCount: 0,
		Page:       1,
		PerPage:    20,
		TotalPages: 0,
	}, nil)

	srv := createTestServer(mockContainer)
	req := makeAuthenticatedRequest("GET", "/api/v1/"+teamID+"/prompts", nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockContainer.promptService.AssertExpectations(t)
}

// TestHandleListPrompts_SortOrderDesc tests list prompts with desc sort order
func TestHandleListPrompts_SortOrderDesc(t *testing.T) {
	mockContainer := newMockPromptContainer(t)
	teamID := "550e8400-e29b-41d4-a716-446655440000"

	mockContainer.teamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", teamID).
		Return(true, nil)

	mockContainer.promptService.On("ListPrompts", "user-123", mock.MatchedBy(func(filters services.PromptFilters) bool {
		return filters.SortBy == "name" && filters.SortOrder == "desc" && filters.TeamID == teamID
	})).Return(&models.PromptListResponse{
		Prompts:    []models.Prompt{},
		TotalCount: 0,
		Page:       1,
		PerPage:    20,
		TotalPages: 0,
	}, nil)

	srv := createTestServer(mockContainer)
	url := "/api/v1/" + teamID + "/prompts?sort_by=name&sort_order=desc"
	req := makeAuthenticatedRequest("GET", url, nil, "user-123")
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockContainer.promptService.AssertExpectations(t)
}

// TODO: Fix context/auth setup for integration tests
// For now, we have good test coverage with:
// - Repository tests (TestPromptReferenceRepository_*)
// - Service tests (TestPromptService_GetPromptDependencies*)
// - Handler unauthorized tests (TestGetPromptDependencies_Unauthorized)
// - Frontend service tests
/*
func TestHandleGetPromptDependencies_Success(t *testing.T) {
	mockContainer := &MockPromptContainer{
		promptService:        svcmocks.NewMockPromptServiceInterface(t),
		resourceUsageService: &MockResourceUsageServiceForHandlers{},
		embeddingService:     svcmocks.NewMockEmbeddingServiceInterface(t),
	}

	cfg := &config.Config{}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	srv := &Server{
		router:    chi.NewRouter(),
		port:      "8080",
		container: mockContainer,
		config:    cfg,
		logger:    logger,
	}
	srv.setupRoutes()

	userID := "user-123"
	slug := "test-prompt"

	expectedDeps := &models.PromptDependenciesResponse{
		UsedBy: []models.PromptDependencyInfo{
			{ID: "dep-1", Slug: "dependent-1", Name: "Dependent Prompt 1"},
			{ID: "dep-2", Slug: "dependent-2", Name: "Dependent Prompt 2"},
		},
		Uses: []models.PromptDependencyInfo{
			{ID: "ref-1", Slug: "referenced-1", Name: "Referenced Prompt 1"},
		},
	}

	mockContainer.promptService.On("GetPromptDependenciesBySlug", userID, slug).
		Return(expectedDeps, nil)

	req, err := http.NewRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/"+slug+"/dependencies", nil)
	assert.NoError(t, err)

	ctx := context.WithValue(req.Context(), contextKeyUserID, userID)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.PromptDependenciesResponse
	err = json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify response structure
	assert.Len(t, response.UsedBy, 2)
	assert.Len(t, response.Uses, 1)
	assert.Equal(t, "dependent-1", response.UsedBy[0].Slug)
	assert.Equal(t, "Dependent Prompt 1", response.UsedBy[0].Name)
	assert.Equal(t, "referenced-1", response.Uses[0].Slug)

	mockContainer.promptService.AssertExpectations(t)
}

func TestHandleGetPromptDependencies_EmptyArrays(t *testing.T) {
	mockContainer := &MockPromptContainer{
		promptService:        svcmocks.NewMockPromptServiceInterface(t),
		resourceUsageService: &MockResourceUsageServiceForHandlers{},
		embeddingService:     svcmocks.NewMockEmbeddingServiceInterface(t),
	}

	cfg := &config.Config{}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	srv := &Server{
		router:    chi.NewRouter(),
		port:      "8080",
		container: mockContainer,
		config:    cfg,
		logger:    logger,
	}
	srv.setupRoutes()

	userID := "user-123"
	slug := "test-prompt"

	// Test that empty arrays are returned, not null
	expectedDeps := &models.PromptDependenciesResponse{
		UsedBy: []models.PromptDependencyInfo{},
		Uses:   []models.PromptDependencyInfo{},
	}

	mockContainer.promptService.On("GetPromptDependenciesBySlug", userID, slug).
		Return(expectedDeps, nil)

	req, err := http.NewRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/"+slug+"/dependencies", nil)
	assert.NoError(t, err)

	ctx := context.WithValue(req.Context(), contextKeyUserID, userID)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.PromptDependenciesResponse
	err = json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify empty arrays are returned, not null
	assert.NotNil(t, response.UsedBy)
	assert.NotNil(t, response.Uses)
	assert.Len(t, response.UsedBy, 0)
	assert.Len(t, response.Uses, 0)

	// Verify JSON structure contains empty arrays
	assert.Contains(t, rr.Body.String(), `"used_by":[]`)
	assert.Contains(t, rr.Body.String(), `"uses":[]`)

	mockContainer.promptService.AssertExpectations(t)
}

func TestHandleGetPromptDependencies_PromptNotFound(t *testing.T) {
	mockContainer := &MockPromptContainer{
		promptService:        svcmocks.NewMockPromptServiceInterface(t),
		resourceUsageService: &MockResourceUsageServiceForHandlers{},
		embeddingService:     svcmocks.NewMockEmbeddingServiceInterface(t),
	}

	cfg := &config.Config{}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	srv := &Server{
		router:    chi.NewRouter(),
		port:      "8080",
		container: mockContainer,
		config:    cfg,
		logger:    logger,
	}
	srv.setupRoutes()

	userID := "user-123"
	slug := "non-existent"

	mockContainer.promptService.On("GetPromptDependenciesBySlug", userID, slug).
		Return(nil, repositories.ErrPromptNotFound)

	req, err := http.NewRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/"+slug+"/dependencies", nil)
	assert.NoError(t, err)

	ctx := context.WithValue(req.Context(), contextKeyUserID, userID)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)

	var response map[string]interface{}
	err = json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, "RESOURCE_NOT_FOUND", response["code"])
	assert.Contains(t, response["detail"], "Prompt not found")

	mockContainer.promptService.AssertExpectations(t)
}
*/
