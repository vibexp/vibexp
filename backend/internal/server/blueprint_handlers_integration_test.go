package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// MockBlueprintContainer implements container.Container interface for blueprint tests
type MockBlueprintContainer struct {
	BaseMockContainer        // Embed base container for default nil implementations
	BlueprintServiceMock     services.BlueprintServiceInterface
	ResourceUsageServiceMock services.ResourceUsageServiceInterface
	EmbeddingServiceMock     services.EmbeddingServiceInterface
	AuthServiceMock          services.AuthServiceInterface
	TeamServiceMock          services.TeamServiceInterface
	APIKeyServiceMock        services.APIKeyServiceInterface
}

func (m *MockBlueprintContainer) BlueprintService() services.BlueprintServiceInterface {
	return m.BlueprintServiceMock
}

func (m *MockBlueprintContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return m.ResourceUsageServiceMock
}

func (m *MockBlueprintContainer) EmbeddingService() services.EmbeddingServiceInterface {
	return m.EmbeddingServiceMock
}

// Implement all other required container methods (returning nil for unused services)
func (m *MockBlueprintContainer) AuthService() services.AuthServiceInterface {
	return m.AuthServiceMock
}
func (m *MockBlueprintContainer) APIKeyService() services.APIKeyServiceInterface {
	return m.APIKeyServiceMock
}
func (m *MockBlueprintContainer) TeamService() services.TeamServiceInterface {
	return m.TeamServiceMock
}

// Helper function to create authenticated request with user context
//
//nolint:unparam // userID parameter kept for consistency with similar test helpers
func createBlueprintAuthenticatedRequest(method, path, body, userID string) *http.Request {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	// Add fake API key for authentication (will be validated by mock)
	req.Header.Set("Authorization", "Bearer vxk_test_fake_key_for_testing")
	ctx := context.WithValue(req.Context(), contextKeyUserID, userID)
	return req.WithContext(ctx)
}

// createListSpecTestServer creates a test server with blueprint mocks for list tests
func createListSpecTestServer(t *testing.T, mockSvc *servicesmocks.MockBlueprintServiceInterface) *Server {
	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation (default: allow access)
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockSvc,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer
	return srv
}

// TestHandleCreateBlueprint_Success tests successful blueprint creation
//
//nolint:funlen // Comprehensive test with mocked service
func TestHandleCreateBlueprint_Success(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	mockResourceService := servicesmocks.NewMockResourceUsageServiceInterface(t)
	mockAuthService := servicesmocks.NewMockAuthServiceInterface(t)
	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	expectedBlueprint := &models.Blueprint{
		ID:          "spec-new",
		ProjectID:   "550e8400-e29b-41d4-a716-446655440000",
		Slug:        "550e8400-e29b-41d4-a716-446655440000",
		Title:       "New Blueprint",
		Content:     "New Spec Content",
		Description: "New Spec Description",
		UserID:      "user-123",
		Type:        "general",
		Status:      "active",
		Metadata:    map[string]interface{}{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", "550e8400-e29b-41d4-a716-446655440000").
		Return(true, nil)

	mockResourceService.On("CheckResourceLimit", mock.Anything, "user-123", "blueprint").
		Return(true, nil)

	mockBlueprintService.On(
		"CreateBlueprint",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		mock.MatchedBy(func(req *models.CreateBlueprintRequest) bool {
			return req.ProjectID == "550e8400-e29b-41d4-a716-446655440000" &&
				req.Slug == "550e8400-e29b-41d4-a716-446655440000" &&
				req.Title == "New Blueprint" &&
				req.Content == "New Spec Content"
		})).Return(expectedBlueprint, nil)

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock:     mockBlueprintService,
		ResourceUsageServiceMock: mockResourceService,
		AuthServiceMock:          mockAuthService,
		TeamServiceMock:          mockTeamService,
		APIKeyServiceMock:        mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	reqBody := `{
		"project_id": "550e8400-e29b-41d4-a716-446655440000",
		"slug": "550e8400-e29b-41d4-a716-446655440000",
		"title": "New Blueprint",
		"content": "New Spec Content",
		"description": "New Spec Description",
		"type": "general",
		"status": "active"
	}`

	req := createBlueprintAuthenticatedRequest(
		"POST",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints",
		reqBody,
		"user-123",
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	// Debug output
	if rr.Code != http.StatusCreated {
		t.Logf("Actual status code: %d, Body: %s", rr.Code, rr.Body.String())
	}

	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response models.Blueprint
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "spec-new", response.ID)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", response.Slug)
	assert.Equal(t, "New Blueprint", response.Title)

	specconformance.AssertConformsToSpec(t, req, rr)
	mockBlueprintService.AssertExpectations(t)
	mockResourceService.AssertExpectations(t)
}

// TestHandleCreateBlueprint_PathErrors covers the #339 error mappings: a
// traversal-invalid path surfaces as 400, and a duplicate (project_id, path)
// conflict surfaces as 409.
func TestHandleCreateBlueprint_PathErrors(t *testing.T) {
	cases := []struct {
		name       string
		svcErr     error
		wantStatus int
	}{
		{"invalid path -> 400", services.ErrInvalidBlueprintPath, http.StatusBadRequest},
		{
			"duplicate path -> 409",
			errors.New("blueprint with path 'a.md' already exists in this project"),
			http.StatusConflict,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
			mockResourceService := servicesmocks.NewMockResourceUsageServiceInterface(t)
			mockAuthService := servicesmocks.NewMockAuthServiceInterface(t)
			mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
			mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

			mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
				Return(&models.APIKey{ID: "api-key-123", UserID: "user-123"}, nil)
			mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", "550e8400-e29b-41d4-a716-446655440000").
				Return(true, nil)
			mockResourceService.On("CheckResourceLimit", mock.Anything, "user-123", "blueprint").
				Return(true, nil)
			mockBlueprintService.On("CreateBlueprint", "user-123", "550e8400-e29b-41d4-a716-446655440000",
				mock.Anything).Return(nil, tc.svcErr)

			srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
			srv.container = &MockBlueprintContainer{
				BlueprintServiceMock:     mockBlueprintService,
				ResourceUsageServiceMock: mockResourceService,
				AuthServiceMock:          mockAuthService,
				TeamServiceMock:          mockTeamService,
				APIKeyServiceMock:        mockAPIKeyService,
			}

			reqBody := `{"project_id":"550e8400-e29b-41d4-a716-446655440000","slug":"a",` +
				`"title":"A","content":"c","path":"../escape.md"}`
			req := createBlueprintAuthenticatedRequest(
				"POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints", reqBody, "user-123")
			rr := httptest.NewRecorder()
			srv.router.ServeHTTP(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code, "body: %s", rr.Body.String())
			mockBlueprintService.AssertExpectations(t)
		})
	}
}

// TestHandleCreateBlueprint_ValidationError tests validation errors
//
//nolint:funlen // Comprehensive table-driven test
func TestHandleCreateBlueprint_ValidationError(t *testing.T) {
	srv, _, _ := setupTestServerForBlueprint(t, nil)

	testCases := []struct {
		name           string
		body           string
		expectedStatus int
	}{
		{
			name:           "Missing slug",
			body:           `{"title": "Test", "content": "Content"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Missing title",
			body:           `{"project_id": "550e8400-e29b-41d4-a716-446655440000", "slug": "test", "content": "Content"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Missing content",
			body:           `{"project_id": "550e8400-e29b-41d4-a716-446655440000", "slug": "test", "title": "Test"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Slug too long",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", "slug": "` +
				strings.Repeat("a", 256) + `", "title": "Test", "content": "Content"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Title too long",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test", "title": "` + strings.Repeat("a", 256) + `", "content": "Content"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Invalid type",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test", "title": "Test", "content": "Content", "type": "invalid"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Invalid status",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test", "title": "Test", "content": "Content", "status": "invalid"}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := createBlueprintAuthenticatedRequest(
				"POST",
				"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints",
				tc.body,
				"user-123",
			)
			rr := httptest.NewRecorder()

			srv.router.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
		})
	}
}

// TestHandleCreateBlueprint_ResourceLimitExceeded tests resource limit exceeded
func TestHandleCreateBlueprint_ResourceLimitExceeded(t *testing.T) {
	srv, _, mockResourceService := setupTestServerForBlueprint(t, func(
		specLibSvc *servicesmocks.MockBlueprintServiceInterface,
		resourceSvc *servicesmocks.MockResourceUsageServiceInterface,
		teamSvc *servicesmocks.MockTeamServiceInterface,
	) {
		resourceSvc.On("CheckResourceLimit", mock.Anything, "user-123", "blueprint").
			Return(false, nil)
		teamSvc.On("IsUserMemberOfTeam", mock.Anything, "user-123", "550e8400-e29b-41d4-a716-446655440000").
			Return(true, nil)
	})

	reqBody := `{
		"project_id": "550e8400-e29b-41d4-a716-446655440000",
		"slug": "test-slug",
		"title": "New Blueprint",
		"content": "New Spec Content"
	}`

	req := createBlueprintAuthenticatedRequest(
		"POST",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints",
		reqBody,
		"user-123",
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	mockResourceService.AssertExpectations(t)
}

// TestHandleCreateBlueprint_ServiceError tests service errors
func TestHandleCreateBlueprint_ServiceError(t *testing.T) {
	srv, mockBlueprintService, _ := setupTestServerForBlueprint(t, func(
		specLibSvc *servicesmocks.MockBlueprintServiceInterface,
		resourceSvc *servicesmocks.MockResourceUsageServiceInterface,
		teamSvc *servicesmocks.MockTeamServiceInterface,
	) {
		resourceSvc.On("CheckResourceLimit", mock.Anything, "user-123", "blueprint").
			Return(true, nil)
		specLibSvc.On("CreateBlueprint", "user-123", "550e8400-e29b-41d4-a716-446655440000", mock.Anything).
			Return((*models.Blueprint)(nil), errors.New("blueprint already exists"))
		teamSvc.On("IsUserMemberOfTeam", mock.Anything, "user-123", "550e8400-e29b-41d4-a716-446655440000").
			Return(true, nil)
	})

	reqBody := `{
		"project_id": "550e8400-e29b-41d4-a716-446655440000",
		"slug": "existing-spec",
		"title": "Existing Spec",
		"content": "Content"
	}`

	req := createBlueprintAuthenticatedRequest(
		"POST",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints",
		reqBody,
		"user-123",
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code)
	mockBlueprintService.AssertExpectations(t)
}

// setupTestServerForBlueprint creates a test server with mocked services for blueprint tests
func setupTestServerForBlueprint(
	t *testing.T,
	setupMocks func(
		*servicesmocks.MockBlueprintServiceInterface,
		*servicesmocks.MockResourceUsageServiceInterface,
		*servicesmocks.MockTeamServiceInterface,
	),
) (
	*Server,
	*servicesmocks.MockBlueprintServiceInterface,
	*servicesmocks.MockResourceUsageServiceInterface,
) {
	t.Helper()
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	mockResourceService := servicesmocks.NewMockResourceUsageServiceInterface(t)
	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication for all tests (Bearer token in request)
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation for all tests (default: allow access)
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	if setupMocks != nil {
		setupMocks(mockBlueprintService, mockResourceService, mockTeamService)
	}

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock:     mockBlueprintService,
		ResourceUsageServiceMock: mockResourceService,
		TeamServiceMock:          mockTeamService,
		APIKeyServiceMock:        mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	return srv, mockBlueprintService, mockResourceService
}

// TestHandleCreateBlueprint_ValidTypes tests valid blueprint types
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestHandleCreateBlueprint_ValidTypes(t *testing.T) {
	testCases := []struct {
		name    string
		body    string
		subtype string
	}{
		{
			name: "Valid general type",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test-spec", "title": "Test", "content": "Content", "type": "general"}`,
		},
		{
			name: "Valid claude-code type",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test-spec", "title": "Test", "content": "Content", "type": "claude-code"}`,
		},
		{
			name: "Claude-code with sub-agents",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test-spec", "title": "Test", "content": "Content", ` +
				`"type": "claude-code", "subtype": "sub-agents", "metadata": {"model": "inherit"}}`,
			subtype: "sub-agents",
		},
		{
			name: "Claude-code with skills",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test-spec", "title": "Test", "content": "Content", ` +
				`"type": "claude-code", "subtype": "skills"}`,
			subtype: "skills",
		},
		{
			name: "Claude-code with slash-commands",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test-spec", "title": "Test", "content": "Content", ` +
				`"type": "claude-code", "subtype": "slash-commands"}`,
			subtype: "slash-commands",
		},
		{
			name: "Claude-code with others",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test-spec", "title": "Test", "content": "Content", ` +
				`"type": "claude-code", "subtype": "others"}`,
			subtype: "others",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srv, mockSvc, mockRes := setupTestServerForBlueprint(t, func(
				mockSvc *servicesmocks.MockBlueprintServiceInterface,
				mockRes *servicesmocks.MockResourceUsageServiceInterface,
				mockTeam *servicesmocks.MockTeamServiceInterface,
			) {
				mockTeam.On("IsUserMemberOfTeam", mock.Anything, "user-123", "550e8400-e29b-41d4-a716-446655440000").
					Return(true, nil)

				mockRes.On("CheckResourceLimit", mock.Anything, "user-123", "blueprint").Return(true, nil)
				now := time.Now()
				createdSpec := &models.Blueprint{
					ID: "spec-123", ProjectID: "shared", UserID: "user-123", CreatedAt: now, UpdatedAt: now,
				}
				mockSvc.On(
					"CreateBlueprint",
					"user-123",
					"550e8400-e29b-41d4-a716-446655440000",
					mock.Anything,
				).Return(createdSpec, nil)
			})

			req := createBlueprintAuthenticatedRequest(
				"POST",
				"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints",
				tc.body,
				"user-123",
			)
			rr := httptest.NewRecorder()
			srv.router.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusCreated, rr.Code)
			mockSvc.AssertExpectations(t)
			mockRes.AssertExpectations(t)
		})
	}
}

// TestHandleCreateBlueprint_InvalidType tests invalid blueprint type rejection
func TestHandleCreateBlueprint_InvalidType(t *testing.T) {
	srv, _, _ := setupTestServerForBlueprint(t, nil)

	body := `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
		`"slug": "test-spec", "title": "Test", "content": "Content", "type": "invalid-type"}`
	req := createBlueprintAuthenticatedRequest(
		"POST",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints",
		body,
		"user-123",
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	bodyBytes, err := io.ReadAll(rr.Body)
	assert.NoError(t, err)
	assert.Contains(t, string(bodyBytes), "Type must be one of: general, claude-code, claude, cursor, codex")
}

// TestHandleGetBlueprint_Success tests successful blueprint retrieval
//
//nolint:funlen // Test function requires comprehensive setup
func TestHandleGetBlueprint_Success(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)

	expectedBlueprint := &models.Blueprint{
		ID:          "spec-1",
		ProjectID:   "550e8400-e29b-41d4-a716-446655440000",
		Slug:        "test-spec",
		Title:       "Test Blueprint",
		Content:     "Test Spec Content",
		Description: "Test Description",
		UserID:      "user-123",
		Type:        "general",
		Status:      "active",
		Metadata:    map[string]interface{}{"key": "value"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	mockBlueprintService.On(
		"GetBlueprintByProjectIDAndSlugInTeam",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"550e8400-e29b-41d4-a716-446655440000",
		"test-spec",
	).Return(expectedBlueprint, nil)

	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockBlueprintService,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	req := createBlueprintAuthenticatedRequest(
		"GET",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints/550e8400-e29b-41d4-a716-446655440000/test-spec",
		"",
		"user-123",
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response models.Blueprint
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "spec-1", response.ID)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", response.ProjectID)
	assert.Equal(t, "test-spec", response.Slug)
	assert.Equal(t, "Test Blueprint", response.Title)
	assert.Equal(t, "Test Spec Content", response.Content)

	specconformance.AssertConformsToSpec(t, req, rr)
	mockBlueprintService.AssertExpectations(t)
}

// TestHandleGetBlueprint_NotFound tests blueprint not found scenario
func TestHandleGetBlueprint_NotFound(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)

	mockBlueprintService.On(
		"GetBlueprintByProjectIDAndSlugInTeam",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"550e8400-e29b-41d4-a716-446655440000",
		"non-existent",
	).Return((*models.Blueprint)(nil), errors.New("blueprint not found"))

	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockBlueprintService,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	req := createBlueprintAuthenticatedRequest(
		"GET",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints/550e8400-e29b-41d4-a716-446655440000/non-existent",
		"",
		"user-123",
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	mockBlueprintService.AssertExpectations(t)
}

// TestHandleListBlueprints_Success tests listing spec libraries
//
//nolint:funlen
func TestHandleListBlueprints_Success(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)

	expectedSpecLibraries := []models.Blueprint{
		{ID: "spec-1", ProjectID: "550e8400-e29b-41d4-a716-446655440000", Slug: "spec-1", Title: "Blueprint 1"},
		{ID: "spec-2", ProjectID: "550e8400-e29b-41d4-a716-446655440000", Slug: "spec-2", Title: "Blueprint 2"},
	}

	expectedResponse := &models.BlueprintListResponse{
		Blueprints: expectedSpecLibraries,
		TotalCount: 2,
		Page:       1,
		PerPage:    10,
		TotalPages: 1,
	}

	mockBlueprintService.On(
		"ListBlueprints",
		"user-123",
		mock.MatchedBy(func(filters services.BlueprintFilters) bool {
			return filters.Page == 1 && filters.Limit == 10
		})).Return(expectedResponse, nil)

	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockBlueprintService,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	req := createBlueprintAuthenticatedRequest(
		"GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints", "", "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response models.BlueprintListResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, 2, response.TotalCount)
	assert.Len(t, response.Blueprints, 2)

	mockBlueprintService.AssertExpectations(t)
}

// TestHandleListBlueprints_EmptyResultConformsToSpec guards the issue #121 fix:
// the empty blueprints list must serialize under the wire field name the spec
// documents (`blueprints`). It reproduces the exact body the frontend crashed
// on ({"blueprints":[],"total_count":0,...}) and validates it against the spec,
// so a future spec/backend field-name divergence fails here instead of in E2E.
func TestHandleListBlueprints_EmptyResultConformsToSpec(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)

	mockBlueprintService.On(
		"ListBlueprints",
		"user-123",
		mock.MatchedBy(func(filters services.BlueprintFilters) bool {
			return filters.Page == 1 && filters.Limit == 10
		})).Return(&models.BlueprintListResponse{
		Blueprints: []models.Blueprint{},
		TotalCount: 0,
		Page:       1,
		PerPage:    20,
		TotalPages: 0,
	}, nil)

	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{ID: "api-key-123", UserID: "user-123"}, nil)
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockBlueprintService,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	req := createBlueprintAuthenticatedRequest(
		"GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints", "", "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)
	specconformance.AssertConformsToSpec(t, req, rr)

	assert.Equal(t, http.StatusOK, rr.Code)
	// The wire field must be `blueprints` (spec name aligned with the backend).
	assert.JSONEq(t,
		`{"blueprints":[],"total_count":0,"page":1,"per_page":20,"total_pages":0}`,
		rr.Body.String())

	mockBlueprintService.AssertExpectations(t)
}

// TestHandleListBlueprints_WithFilters tests listing with query filters
//
//nolint:funlen // Comprehensive test with multiple filter combinations
func TestHandleListBlueprints_WithFilters(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)

	expectedResponse := &models.BlueprintListResponse{
		Blueprints: []models.Blueprint{},
		TotalCount: 0,
		Page:       2,
		PerPage:    10,
		TotalPages: 0,
	}

	mockBlueprintService.On(
		"ListBlueprints",
		"user-123",
		mock.MatchedBy(func(filters services.BlueprintFilters) bool {
			return filters.ProjectID == "my-project" &&
				filters.Status == "active" &&
				filters.Type == "general" &&
				filters.Page == 2 &&
				filters.Limit == 10
		})).Return(expectedResponse, nil)

	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockBlueprintService,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	req := createBlueprintAuthenticatedRequest(
		"GET",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints"+
			"?project_id=my-project&status=active&type=general&page=2&limit=10",
		"",
		"user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	mockBlueprintService.AssertExpectations(t)
}

// TestHandleListBlueprints_ServiceError tests list service error
func TestHandleListBlueprints_ServiceError(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)

	mockBlueprintService.On("ListBlueprints", "user-123", mock.Anything).
		Return((*models.BlueprintListResponse)(nil), errors.New("database error"))

	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockBlueprintService,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	req := createBlueprintAuthenticatedRequest(
		"GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints", "", "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	mockBlueprintService.AssertExpectations(t)
}

// TestHandleUpdateBlueprint_Success tests successful blueprint update
//
//nolint:funlen // Test function requires comprehensive setup
func TestHandleUpdateBlueprint_Success(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	mockResourceService := servicesmocks.NewMockResourceUsageServiceInterface(t)
	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	updatedBlueprint := &models.Blueprint{
		ID:        "spec-1",
		ProjectID: "550e8400-e29b-41d4-a716-446655440000",
		Slug:      "test-spec",
		Title:     "Updated Title",
		Content:   "Updated Content",
		UserID:    "user-123",
		Type:      "general",
		Status:    "active",
		UpdatedAt: time.Now(),
	}

	mockResourceService.On("CheckResourceLimit", mock.Anything, "user-123", "blueprint").
		Return(true, nil)

	mockBlueprintService.On(
		"UpdateBlueprintByProjectIDAndSlugInTeam",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"550e8400-e29b-41d4-a716-446655440000",
		"test-spec",
		mock.MatchedBy(func(req *models.UpdateBlueprintRequest) bool {
			return req.Title != nil && *req.Title == "Updated Title"
		})).Return(updatedBlueprint, nil)

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock:     mockBlueprintService,
		ResourceUsageServiceMock: mockResourceService,
		TeamServiceMock:          mockTeamService,
		APIKeyServiceMock:        mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	reqBody := `{"title": "Updated Title", "content": "Updated Content"}`
	req := createBlueprintAuthenticatedRequest(
		"PUT",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints/550e8400-e29b-41d4-a716-446655440000/test-spec",
		reqBody,
		"user-123",
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response models.Blueprint
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "Updated Title", response.Title)
	specconformance.AssertConformsToSpec(t, req, rr)
	mockBlueprintService.AssertExpectations(t)
	mockResourceService.AssertExpectations(t)
}

// TestHandleUpdateBlueprint_NotFound tests update on non-existent blueprint
func TestHandleUpdateBlueprint_NotFound(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	mockResourceService := servicesmocks.NewMockResourceUsageServiceInterface(t)
	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockResourceService.On("CheckResourceLimit", mock.Anything, "user-123", "blueprint").
		Return(true, nil)

	mockBlueprintService.On(
		"UpdateBlueprintByProjectIDAndSlugInTeam",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"550e8400-e29b-41d4-a716-446655440000",
		"non-existent",
		mock.Anything).Return((*models.Blueprint)(nil), errors.New("blueprint not found"))

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock:     mockBlueprintService,
		ResourceUsageServiceMock: mockResourceService,
		TeamServiceMock:          mockTeamService,
		APIKeyServiceMock:        mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	reqBody := `{"title": "Updated Title"}`
	req := createBlueprintAuthenticatedRequest(
		"PUT",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints/550e8400-e29b-41d4-a716-446655440000/non-existent",
		reqBody,
		"user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	mockBlueprintService.AssertExpectations(t)
}

// TestHandleUpdateBlueprint_InvalidPath maps a traversal-invalid path service
// error to 400 (#339).
func TestHandleUpdateBlueprint_InvalidPath(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	mockResourceService := servicesmocks.NewMockResourceUsageServiceInterface(t)
	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{ID: "api-key-123", UserID: "user-123"}, nil)
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()
	mockResourceService.On("CheckResourceLimit", mock.Anything, "user-123", "blueprint").
		Return(true, nil)
	mockBlueprintService.On("UpdateBlueprintByProjectIDAndSlugInTeam",
		"user-123", "550e8400-e29b-41d4-a716-446655440000",
		"550e8400-e29b-41d4-a716-446655440000", "s",
		mock.Anything).Return((*models.Blueprint)(nil), services.ErrInvalidBlueprintPath)

	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
	srv.container = &MockBlueprintContainer{
		BlueprintServiceMock:     mockBlueprintService,
		ResourceUsageServiceMock: mockResourceService,
		TeamServiceMock:          mockTeamService,
		APIKeyServiceMock:        mockAPIKeyService,
	}

	req := createBlueprintAuthenticatedRequest(
		"PUT",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints/550e8400-e29b-41d4-a716-446655440000/s",
		`{"path":"../escape.md"}`, "user-123")
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code, "body: %s", rr.Body.String())
	mockBlueprintService.AssertExpectations(t)
}

// TestHandleUpdateBlueprint_ValidationError tests update validation errors
//
//nolint:funlen // Comprehensive table-driven test
func TestHandleUpdateBlueprint_ValidationError(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	mockResourceService := servicesmocks.NewMockResourceUsageServiceInterface(t)
	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockResourceService.On("CheckResourceLimit", mock.Anything, "user-123", "blueprint").
		Return(true, nil)

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock:     mockBlueprintService,
		ResourceUsageServiceMock: mockResourceService,
		TeamServiceMock:          mockTeamService,
		APIKeyServiceMock:        mockAPIKeyService,
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
			name:           "Invalid JSON",
			body:           `{invalid json}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Title too long",
			body:           `{"title": "` + strings.Repeat("a", 256) + `"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid type",
			body:           `{"type": "invalid"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid status",
			body:           `{"status": "invalid"}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := createBlueprintAuthenticatedRequest(
				"PUT",
				"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints/550e8400-e29b-41d4-a716-446655440000/test-spec",
				tc.body,
				"user-123")
			rr := httptest.NewRecorder()

			srv.router.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
		})
	}
}

// TestHandleDeleteBlueprint_Success tests successful blueprint deletion
//
//nolint:funlen
func TestHandleDeleteBlueprint_Success(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	mockEmbeddingService := servicesmocks.NewMockEmbeddingServiceInterface(t)
	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	existingBlueprint := &models.Blueprint{
		ID:        "spec-1",
		ProjectID: "550e8400-e29b-41d4-a716-446655440000",
		Slug:      "test-spec",
		Title:     "Test Blueprint",
		UserID:    "user-123",
	}

	mockBlueprintService.On(
		"GetBlueprintByProjectIDAndSlugInTeam",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"550e8400-e29b-41d4-a716-446655440000",
		"test-spec",
	).Return(existingBlueprint, nil)

	mockBlueprintService.On(
		"DeleteBlueprintByProjectIDAndSlug",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"550e8400-e29b-41d4-a716-446655440000",
		"test-spec",
	).Return(nil)

	mockEmbeddingService.On("DeleteEmbeddingsByEntity", "blueprint", "spec-1").
		Return(nil)

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockBlueprintService,
		EmbeddingServiceMock: mockEmbeddingService,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	req := createBlueprintAuthenticatedRequest(
		"DELETE",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints/550e8400-e29b-41d4-a716-446655440000/test-spec",
		"",
		"user-123",
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)
	mockBlueprintService.AssertExpectations(t)
}

// TestHandleDeleteBlueprint_NotFound tests deleting non-existent blueprint
func TestHandleDeleteBlueprint_NotFound(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)

	mockBlueprintService.On(
		"GetBlueprintByProjectIDAndSlugInTeam",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"550e8400-e29b-41d4-a716-446655440000",
		"non-existent",
	).Return((*models.Blueprint)(nil), errors.New("blueprint not found"))

	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockBlueprintService,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	req := createBlueprintAuthenticatedRequest(
		"DELETE",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints/550e8400-e29b-41d4-a716-446655440000/non-existent",
		"",
		"user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	mockBlueprintService.AssertExpectations(t)
}

// TestHandleDeleteBlueprint_ServiceError tests delete service error
//
//nolint:funlen
func TestHandleDeleteBlueprint_ServiceError(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)

	existingBlueprint := &models.Blueprint{
		ID:        "spec-1",
		ProjectID: "550e8400-e29b-41d4-a716-446655440000",
		Slug:      "test-spec",
		Title:     "Test Blueprint",
		UserID:    "user-123",
	}

	mockBlueprintService.On(
		"GetBlueprintByProjectIDAndSlugInTeam",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"550e8400-e29b-41d4-a716-446655440000",
		"test-spec",
	).Return(existingBlueprint, nil)

	mockBlueprintService.On(
		"DeleteBlueprintByProjectIDAndSlug",
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		"550e8400-e29b-41d4-a716-446655440000",
		"test-spec",
	).Return(errors.New("database error"))

	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockBlueprintService,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	req := createBlueprintAuthenticatedRequest(
		"DELETE",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints/550e8400-e29b-41d4-a716-446655440000/test-spec",
		"",
		"user-123",
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	mockBlueprintService.AssertExpectations(t)
}

// TestHandleGetBlueprintStats_Success tests getting blueprint statistics
//
//nolint:funlen
func TestHandleGetBlueprintStats_Success(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)

	expectedStats := &models.BlueprintStatsResponse{
		TotalProjects:   5,
		TotalBlueprints: 25,
		AddedThisWeek:   3,
		TotalByType: map[string]int{
			"general": 25,
		},
		TotalByStatus: map[string]int{
			"active":  20,
			"expired": 5,
		},
	}

	mockBlueprintService.On("GetBlueprintStats", "user-123").
		Return(expectedStats, nil)

	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockBlueprintService,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	req := createBlueprintAuthenticatedRequest(
		"GET",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints/stats",
		"",
		"user-123",
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response models.BlueprintStatsResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, 5, response.TotalProjects)
	assert.Equal(t, 25, response.TotalBlueprints)
	assert.Equal(t, 3, response.AddedThisWeek)
	assert.Equal(t, 25, response.TotalByType["general"])
	assert.Equal(t, 20, response.TotalByStatus["active"])

	specconformance.AssertConformsToSpec(t, req, rr)
	mockBlueprintService.AssertExpectations(t)
}

// TestHandleGetBlueprintStats_ServiceError tests stats service error
func TestHandleGetBlueprintStats_ServiceError(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)

	mockBlueprintService.On("GetBlueprintStats", "user-123").
		Return((*models.BlueprintStatsResponse)(nil), errors.New("database error"))

	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockBlueprintService,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	req := createBlueprintAuthenticatedRequest(
		"GET",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints/stats",
		"",
		"user-123",
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	mockBlueprintService.AssertExpectations(t)
}

// TestHandleCreateBlueprint_SubtypeValidation tests subtype validation logic
func TestHandleCreateBlueprint_SubtypeValidation(t *testing.T) {
	testCases := []struct {
		name           string
		body           string
		expectedStatus int
		errorMessage   string
	}{
		{
			name: "Subtype not allowed when type is general",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "t", "title": "T", "content": "C", "type": "general", "subtype": "sub-agents"}`,
			expectedStatus: http.StatusBadRequest,
			errorMessage:   "Subtype cannot be set for type 'general'",
		},
		{
			name: "Invalid subtype value with claude-code type",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "t", "title": "T", "content": "C", "type": "claude-code", "subtype": "invalid"}`,
			expectedStatus: http.StatusBadRequest,
			errorMessage:   "Invalid subtype for type 'claude-code'",
		},
		{
			name: "Valid request without subtype",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "t", "title": "T", "content": "C", "type": "general"}`,
			expectedStatus: http.StatusCreated,
			errorMessage:   "",
		},
		{
			name: "Sub-agents without model metadata",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "t", "title": "T", "content": "C", "type": "claude-code", "subtype": "sub-agents"}`,
			expectedStatus: http.StatusBadRequest,
			errorMessage:   "Sub-agents subtype requires 'model' metadata field",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runSubtypeValidationTest(t, tc.body, tc.expectedStatus, tc.errorMessage)
		})
	}
}

// runSubtypeValidationTest is a helper for subtype validation tests
func runSubtypeValidationTest(t *testing.T, body string, expectedStatus int, errorMessage string) {
	t.Helper()
	srv, mockBlueprintService, mockResourceUsageService := setupTestServerForBlueprint(t,
		func(specLibSvc *servicesmocks.MockBlueprintServiceInterface,
			resourceUsageSvc *servicesmocks.MockResourceUsageServiceInterface,
			mockTeam *servicesmocks.MockTeamServiceInterface) {
			// Only set up CheckResourceLimit expectation for success cases
			if expectedStatus == http.StatusCreated {
				mockTeam.On("IsUserMemberOfTeam", mock.Anything, "user-123", "550e8400-e29b-41d4-a716-446655440000").
					Return(true, nil)

				resourceUsageSvc.On("CheckResourceLimit", mock.Anything, "user-123", "blueprint").
					Return(true, nil)
				specLibSvc.On("CreateBlueprint", "user-123", "550e8400-e29b-41d4-a716-446655440000", mock.Anything).
					Return(&models.Blueprint{ID: "spec-1"}, nil)
			}
		})

	req := createBlueprintAuthenticatedRequest(
		"POST",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints",
		body,
		"user-123",
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, expectedStatus, rr.Code)

	if errorMessage != "" {
		var errResp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		assert.Contains(t, errResp["detail"], errorMessage)
	}

	mockBlueprintService.AssertExpectations(t)
	mockResourceUsageService.AssertExpectations(t)
}

// TestHandleUpdateBlueprint_SubtypeValidationOnTypeChange tests subtype validation on type change
func TestHandleUpdateBlueprint_SubtypeValidationOnTypeChange(t *testing.T) {
	srv, mockBlueprintService, mockResourceUsageService := setupTestServerForBlueprint(t,
		func(specLibSvc *servicesmocks.MockBlueprintServiceInterface,
			resourceUsageSvc *servicesmocks.MockResourceUsageServiceInterface,
			mockTeam *servicesmocks.MockTeamServiceInterface) {
			// Resource limit check happens before validation
			resourceUsageSvc.On("CheckResourceLimit", mock.Anything, "user-123", "blueprint").
				Return(true, nil)
			// No service expectations because validation should fail before service call
		})

	body := `{"type": "general", "subtype": "sub-agents"}`
	req := createBlueprintAuthenticatedRequest(
		"PUT",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints/550e8400-e29b-41d4-a716-446655440000/test-spec",
		body,
		"user-123",
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var errResp map[string]interface{}
	err := json.Unmarshal(rr.Body.Bytes(), &errResp)
	assert.NoError(t, err)
	assert.Contains(t, errResp["detail"], "Subtype cannot be set for type 'general'")

	mockBlueprintService.AssertExpectations(t)
	mockResourceUsageService.AssertExpectations(t)
}

// TestHandleListBlueprintsByProject_Success tests successful blueprint listing by project
func TestHandleListBlueprintsByProject_Success(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	projectID := "550e8400-e29b-41d4-a716-446655440000"
	teamID := "550e8400-e29b-41d4-a716-446655440001"

	expectedResponse := &models.BlueprintListResponse{
		Blueprints: []models.Blueprint{{
			ID: "spec-1", ProjectID: projectID, Slug: "spec-1", Title: "Blueprint 1",
			Content: "Content 1", UserID: "user-123", Type: "general", Status: "active",
		}},
		TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
	}

	mockBlueprintService.On("ListBlueprints", "user-123",
		mock.MatchedBy(func(filters services.BlueprintFilters) bool {
			return filters.ProjectID == projectID && filters.TeamID == teamID
		})).Return(expectedResponse, nil)

	srv := createListSpecTestServer(t, mockBlueprintService)
	reqPath := "/api/v1/" + teamID + "/blueprints/" + projectID
	req := createBlueprintAuthenticatedRequest("GET", reqPath, "", "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var response models.BlueprintListResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 1, response.TotalCount)
	assert.Len(t, response.Blueprints, 1)
	assert.Equal(t, "spec-1", response.Blueprints[0].ID)
	specconformance.AssertConformsToSpec(t, req, rr)
	mockBlueprintService.AssertExpectations(t)
}

// TestHandleListBlueprintsByProject_InvalidProjectID tests listing with invalid project ID format
func TestHandleListBlueprintsByProject_InvalidProjectID(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockBlueprintService,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	teamID := "550e8400-e29b-41d4-a716-446655440001"
	reqPath := "/api/v1/" + teamID + "/blueprints/invalid-project-id"
	req := createBlueprintAuthenticatedRequest("GET", reqPath, "", "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)

	var errResp map[string]interface{}
	err := json.Unmarshal(rr.Body.Bytes(), &errResp)
	assert.NoError(t, err)
	assert.Contains(t, errResp["detail"], "Invalid project_id format")
}

// TestHandleListBlueprintsByProject_ServiceError tests listing with service error
func TestHandleListBlueprintsByProject_ServiceError(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)

	mockBlueprintService.On("ListBlueprints", "user-123", mock.Anything).
		Return((*models.BlueprintListResponse)(nil), errors.New("database error"))

	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockBlueprintService,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	projectID := "550e8400-e29b-41d4-a716-446655440000"
	teamID := "550e8400-e29b-41d4-a716-446655440001"
	reqPath := "/api/v1/" + teamID + "/blueprints/" + projectID
	req := createBlueprintAuthenticatedRequest("GET", reqPath, "", "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	var errResp map[string]interface{}
	err := json.Unmarshal(rr.Body.Bytes(), &errResp)
	assert.NoError(t, err)
	assert.Contains(t, errResp["detail"], "Failed to list blueprints")

	mockBlueprintService.AssertExpectations(t)
}

// TestHandleListBlueprintsByProject_EmptyResult tests listing with no results
//
//nolint:funlen
func TestHandleListBlueprintsByProject_EmptyResult(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)

	projectID := "550e8400-e29b-41d4-a716-446655440000"
	teamID := "550e8400-e29b-41d4-a716-446655440001"

	expectedResponse := &models.BlueprintListResponse{
		Blueprints: []models.Blueprint{},
		TotalCount: 0,
		Page:       1,
		PerPage:    20,
		TotalPages: 0,
	}

	mockBlueprintService.On(
		"ListBlueprints",
		"user-123",
		mock.MatchedBy(func(filters services.BlueprintFilters) bool {
			return filters.ProjectID == projectID && filters.TeamID == teamID
		})).Return(expectedResponse, nil)

	mockTeamService := servicesmocks.NewMockTeamServiceInterface(t)
	mockAPIKeyService := servicesmocks.NewMockAPIKeyServiceInterface(t)

	// Mock API key authentication
	mockAPIKeyService.On("ValidateAPIKey", mock.Anything, "vxk_test_fake_key_for_testing").
		Return(&models.APIKey{
			ID:     "api-key-123",
			UserID: "user-123",
		}, nil)

	// Mock team membership validation
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, "user-123", mock.AnythingOfType("string")).
		Return(true, nil).Maybe()

	mockContainer := &MockBlueprintContainer{
		BlueprintServiceMock: mockBlueprintService,
		TeamServiceMock:      mockTeamService,
		APIKeyServiceMock:    mockAPIKeyService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	reqPath := "/api/v1/" + teamID + "/blueprints/" + projectID
	req := createBlueprintAuthenticatedRequest("GET", reqPath, "", "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.BlueprintListResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 0, response.TotalCount)
	assert.Len(t, response.Blueprints, 0)

	mockBlueprintService.AssertExpectations(t)
}

// TestHandleListBlueprintsByProject_WithPagination tests listing with pagination parameters
func TestHandleListBlueprintsByProject_WithPagination(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	projectID := "550e8400-e29b-41d4-a716-446655440000"
	teamID := "550e8400-e29b-41d4-a716-446655440001"

	expectedResponse := &models.BlueprintListResponse{
		Blueprints: []models.Blueprint{{
			ID: "spec-2", ProjectID: projectID, Slug: "spec-2", Title: "Blueprint 2",
			UserID: "user-123", Type: "general", Status: "active",
		}},
		TotalCount: 15, Page: 2, PerPage: 10, TotalPages: 2,
	}

	mockBlueprintService.On("ListBlueprints", "user-123",
		mock.MatchedBy(func(filters services.BlueprintFilters) bool {
			return filters.ProjectID == projectID && filters.TeamID == teamID &&
				filters.Page == 2 && filters.Limit == 10
		})).Return(expectedResponse, nil)

	srv := createListSpecTestServer(t, mockBlueprintService)
	reqPath := "/api/v1/" + teamID + "/blueprints/" + projectID + "?page=2&limit=10"
	req := createBlueprintAuthenticatedRequest("GET", reqPath, "", "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var response models.BlueprintListResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 15, response.TotalCount)
	assert.Equal(t, 2, response.Page)
	assert.Equal(t, 10, response.PerPage)
	mockBlueprintService.AssertExpectations(t)
}

// TestHandleListBlueprintsByProject_WithFilters tests listing with type and status filters
func TestHandleListBlueprintsByProject_WithFilters(t *testing.T) {
	mockBlueprintService := servicesmocks.NewMockBlueprintServiceInterface(t)
	projectID := "550e8400-e29b-41d4-a716-446655440000"
	teamID := "550e8400-e29b-41d4-a716-446655440001"

	expectedResponse := &models.BlueprintListResponse{
		Blueprints: []models.Blueprint{{
			ID: "spec-3", ProjectID: projectID, Slug: "spec-3", Title: "Blueprint 3",
			UserID: "user-123", Type: "claude-code", Status: "active",
		}},
		TotalCount: 1, Page: 1, PerPage: 20, TotalPages: 1,
	}

	mockBlueprintService.On("ListBlueprints", "user-123",
		mock.MatchedBy(func(filters services.BlueprintFilters) bool {
			return filters.ProjectID == projectID && filters.TeamID == teamID &&
				filters.Type == "claude-code" && filters.Status == "active"
		})).Return(expectedResponse, nil)

	srv := createListSpecTestServer(t, mockBlueprintService)
	reqPath := "/api/v1/" + teamID + "/blueprints/" + projectID +
		"?type=claude-code&status=active"
	req := createBlueprintAuthenticatedRequest("GET", reqPath, "", "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var response models.BlueprintListResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, 1, response.TotalCount)
	assert.Equal(t, "claude-code", response.Blueprints[0].Type)
	mockBlueprintService.AssertExpectations(t)
}

// TestHandleCreateBlueprint_NewTypes tests creating blueprints with new types (claude, cursor, codex)
//
//nolint:funlen // Test function requires comprehensive setup and assertions
func TestHandleCreateBlueprint_NewTypes(t *testing.T) {
	testCases := []struct {
		name    string
		body    string
		subtype string
	}{
		{
			name: "Valid claude type with claude-md subtype",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "claude-md-spec", "title": "Claude.md", "content": "# Claude.md", ` +
				`"type": "claude", "subtype": "claude-md"}`,
			subtype: "claude-md",
		},
		{
			name: "Valid cursor type with skills subtype",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "cursor-skills", "title": "Cursor Skills", "content": "Skills content", ` +
				`"type": "cursor", "subtype": "skills"}`,
			subtype: "skills",
		},
		{
			name: "Valid cursor type with agents subtype",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "cursor-agents", "title": "Cursor Agents", "content": "Agents content", ` +
				`"type": "cursor", "subtype": "agents"}`,
			subtype: "agents",
		},
		{
			name: "Valid cursor type with commands subtype",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "cursor-commands", "title": "Cursor Commands", ` +
				`"content": "Commands content", "type": "cursor", "subtype": "commands"}`,
			subtype: "commands",
		},
		{
			name: "Valid cursor type with rules subtype",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "cursor-rules", "title": "Cursor Rules", "content": "Rules content", ` +
				`"type": "cursor", "subtype": "rules"}`,
			subtype: "rules",
		},
		{
			name: "Valid cursor type with cursor-md subtype",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "cursor-md", "title": "Cursor.md", "content": "# Cursor.md", "type": "cursor", "subtype": "cursor-md"}`,
			subtype: "cursor-md",
		},
		{
			name: "Valid codex type with rules subtype",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "codex-rules", "title": "Codex Rules", "content": "Rules content", "type": "codex", "subtype": "rules"}`,
			subtype: "rules",
		},
		{
			name: "Valid codex type with skills subtype",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "codex-skills", "title": "Codex Skills", "content": "Skills content", ` +
				`"type": "codex", "subtype": "skills"}`,
			subtype: "skills",
		},
		{
			name: "Valid codex type with agents-md subtype",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "codex-agents-md", "title": "Codex AGENTS.md", "content": "# AGENTS.md", ` +
				`"type": "codex", "subtype": "agents-md"}`,
			subtype: "agents-md",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srv, mockSvc, mockRes := setupTestServerForBlueprint(t, func(
				mockSvc *servicesmocks.MockBlueprintServiceInterface,
				mockRes *servicesmocks.MockResourceUsageServiceInterface,
				mockTeam *servicesmocks.MockTeamServiceInterface,
			) {
				mockTeam.On("IsUserMemberOfTeam", mock.Anything, "user-123", "550e8400-e29b-41d4-a716-446655440000").
					Return(true, nil)

				mockRes.On("CheckResourceLimit", mock.Anything, "user-123", "blueprint").Return(true, nil)
				now := time.Now()
				createdSpec := &models.Blueprint{
					ID: "spec-123", ProjectID: "shared", UserID: "user-123", CreatedAt: now, UpdatedAt: now,
				}
				mockSvc.On(
					"CreateBlueprint",
					"user-123",
					"550e8400-e29b-41d4-a716-446655440000",
					mock.Anything,
				).Return(createdSpec, nil)
			})

			req := createBlueprintAuthenticatedRequest(
				"POST",
				"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints",
				tc.body,
				"user-123",
			)
			rr := httptest.NewRecorder()
			srv.router.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusCreated, rr.Code)
			mockSvc.AssertExpectations(t)
			mockRes.AssertExpectations(t)
		})
	}
}

// TestHandleCreateBlueprint_InvalidTypeSubtypeCombinations tests invalid type-subtype combinations
//
//nolint:funlen // Test function requires comprehensive test cases
func TestHandleCreateBlueprint_InvalidTypeSubtypeCombinations(t *testing.T) {
	testCases := []struct {
		name          string
		body          string
		expectedError string
	}{
		{
			name: "Claude type with invalid subtype (skills)",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test-spec", "title": "Test", "content": "Content", "type": "claude", "subtype": "skills"}`,
			expectedError: "Invalid subtype for type 'claude'",
		},
		{
			name: "Cursor type with invalid subtype (claude-md)",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test-spec", "title": "Test", "content": "Content", "type": "cursor", "subtype": "claude-md"}`,
			expectedError: "Invalid subtype for type 'cursor'",
		},
		{
			name: "Codex type with invalid subtype (agents)",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test-spec", "title": "Test", "content": "Content", "type": "codex", "subtype": "agents"}`,
			expectedError: "Invalid subtype for type 'codex'",
		},
		{
			name: "General type with subtype (should fail)",
			body: `{"project_id": "550e8400-e29b-41d4-a716-446655440000", ` +
				`"slug": "test-spec", "title": "Test", "content": "Content", "type": "general", "subtype": "skills"}`,
			expectedError: "Subtype cannot be set for type 'general'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _, _ := setupTestServerForBlueprint(t, nil)

			req := createBlueprintAuthenticatedRequest(
				"POST",
				"/api/v1/550e8400-e29b-41d4-a716-446655440000/blueprints",
				tc.body,
				"user-123",
			)
			rr := httptest.NewRecorder()
			srv.router.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
			bodyBytes, err := io.ReadAll(rr.Body)
			assert.NoError(t, err)
			assert.Contains(t, string(bodyBytes), tc.expectedError)
		})
	}
}
