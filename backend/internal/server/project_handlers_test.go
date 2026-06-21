package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

func TestProjectHandlers_ListProjectsRequiresTeamID(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{
			"List without team_id in URL - Unauthorized (auth runs first)",
			"/api/v1/projects",
			http.StatusUnauthorized,
		},
		{
			"List with team_id in URL - Unauthorized",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/projects",
			http.StatusUnauthorized,
		},
		{
			"List with invalid team_id in URL - Unauthorized (auth runs first)",
			"/api/v1/invalid/projects",
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v, body: %v",
					status, tt.expected, rr.Body.String())
			}
		})
	}
}

// TestCreateProject_Unauthorized tests project creation without auth
func TestCreateProject_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{"name": "Test Project", "slug": "test-project"}`
	req, err := http.NewRequest("POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestCreateProject_InvalidJSON tests project creation with invalid JSON
func TestCreateProject_InvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{invalid json}`
	req, err := http.NewRequest("POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should be 401 Unauthorized (no valid auth) or 400 Bad Request (invalid JSON)
	assert.True(t, rr.Code == http.StatusUnauthorized || rr.Code == http.StatusBadRequest,
		"Expected 401 or 400, got %d", rr.Code)
}

// TestCreateProject_MissingName tests project creation without name
func TestCreateProject_MissingName(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{"slug": "test-project"}`
	req, err := http.NewRequest("POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should be 401 Unauthorized (no valid auth) or 400 Bad Request (validation)
	assert.True(t, rr.Code == http.StatusUnauthorized || rr.Code == http.StatusBadRequest,
		"Expected 401 or 400, got %d", rr.Code)
}

// TestCreateProject_MissingSlug tests project creation without slug
func TestCreateProject_MissingSlug(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{"name": "Test Project"}`
	req, err := http.NewRequest("POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should be 401 Unauthorized (no valid auth) or 400 Bad Request (validation)
	assert.True(t, rr.Code == http.StatusUnauthorized || rr.Code == http.StatusBadRequest,
		"Expected 401 or 400, got %d", rr.Code)
}

// TestCreateProject_RouteRegistered verifies the route is registered
func TestCreateProject_RouteRegistered(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{"name": "Test Project", "slug": "test-project"}`
	req, err := http.NewRequest("POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should not return 404 (route not found)
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "Route should be registered")
}

// TestGetProject_Unauthorized tests get project without auth
func TestGetProject_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects/test-project", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestGetProject_RouteRegistered verifies the route is registered
func TestGetProject_RouteRegistered(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects/test-project", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should not return 404 (route not found)
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "Route should be registered")
}

// TestUpdateProject_Unauthorized tests update project without auth
func TestUpdateProject_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{"name": "Updated Project"}`
	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects/test-project"
	req, err := http.NewRequest("PUT", url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestUpdateProject_RouteRegistered verifies the route is registered
func TestUpdateProject_RouteRegistered(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{"name": "Updated Project"}`
	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects/test-project"
	req, err := http.NewRequest("PUT", url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should not return 404 (route not found)
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "Route should be registered")
}

// TestDeleteProject_Unauthorized tests delete project without auth
func TestDeleteProject_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("DELETE", "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects/test-project", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestDeleteProject_RouteRegistered verifies the route is registered
func TestDeleteProject_RouteRegistered(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("DELETE", "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects/test-project", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should not return 404 (route not found)
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "Route should be registered")
}

// TestProjectEndpoints_MethodNotAllowed tests wrong HTTP methods
// Note: Auth middleware runs first, so unauthorized requests return 401
func TestProjectEndpoints_MethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{
			name:     "POST on get project - unauthorized",
			method:   "POST",
			path:     "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects/test-project",
			expected: http.StatusUnauthorized, // Auth runs before method check
		},
		{
			name:     "PATCH on projects list - unauthorized",
			method:   "PATCH",
			path:     "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects",
			expected: http.StatusUnauthorized, // Auth runs before method check
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			assert.Equal(t, tt.expected, rr.Code)
		})
	}
}

// TestBuildProjectFilters_Defaults tests filter building with defaults
func TestBuildProjectFilters_Defaults(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/projects", nil)
	require.NoError(t, err)

	filters := srv.buildProjectFilters(req, "test-team")

	assert.Equal(t, "test-team", filters.TeamID)
	assert.Equal(t, 1, filters.Page)
	assert.Equal(t, 20, filters.Limit)
	assert.Empty(t, filters.Search)
	assert.Empty(t, filters.SortBy)
	assert.Empty(t, filters.SortOrder)
}

// TestBuildProjectFilters_WithParams tests filter building with query params
func TestBuildProjectFilters_WithParams(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/projects?page=2&limit=50&search=test&sort_by=name&sort_order=desc", nil)
	require.NoError(t, err)

	filters := srv.buildProjectFilters(req, "test-team")

	assert.Equal(t, "test-team", filters.TeamID)
	assert.Equal(t, 2, filters.Page)
	assert.Equal(t, 50, filters.Limit)
	assert.Equal(t, "test", filters.Search)
	assert.Equal(t, "name", filters.SortBy)
	assert.Equal(t, "desc", filters.SortOrder)
}

// TestBuildProjectFilters_InvalidPage tests filter building with invalid page
func TestBuildProjectFilters_InvalidPage(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name         string
		pageStr      string
		expectedPage int
	}{
		{"Non-numeric page", "abc", 1},
		{"Negative page", "-1", 1},
		{"Zero page", "0", 1},
		{"Valid page", "5", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET",
				"/api/v1/550e8400-e29b-41d4-a716-446655440000/projects?page="+tt.pageStr, nil)
			require.NoError(t, err)

			filters := srv.buildProjectFilters(req, "test")
			assert.Equal(t, tt.expectedPage, filters.Page)
		})
	}
}

// TestBuildProjectFilters_InvalidLimit tests filter building with invalid limit
func TestBuildProjectFilters_InvalidLimit(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name          string
		limitStr      string
		expectedLimit int
	}{
		{"Non-numeric limit", "abc", 20},
		{"Negative limit", "-1", 20},
		{"Zero limit", "0", 20},
		{"Exceeds max limit", "200", 100},
		{"Far exceeds max", "1000", 100},
		{"Valid limit", "50", 50},
		{"Max valid limit", "100", 100},
		{"Min valid limit", "1", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET",
				"/api/v1/550e8400-e29b-41d4-a716-446655440000/projects?limit="+tt.limitStr, nil)
			require.NoError(t, err)

			filters := srv.buildProjectFilters(req, "test")
			assert.Equal(t, tt.expectedLimit, filters.Limit)
		})
	}
}

// TestValidateCreateProjectRequest_Success tests successful validation
func TestValidateCreateProjectRequest_Success(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	srv := New("8080", nil, "test-api-key", cfg, logger)

	rr := httptest.NewRecorder()

	req := &models.CreateProjectRequest{
		Name: "Test Project",
		Slug: "test-project",
	}

	result := srv.validateCreateProjectRequest(rr, req)
	assert.True(t, result)
}

// TestValidateCreateProjectRequest_EmptyName tests validation with empty name
func TestValidateCreateProjectRequest_EmptyName(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	srv := New("8080", nil, "test-api-key", cfg, logger)

	rr := httptest.NewRecorder()

	req := &models.CreateProjectRequest{
		Name: "",
		Slug: "test-project",
	}

	result := srv.validateCreateProjectRequest(rr, req)
	assert.False(t, result)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestValidateCreateProjectRequest_EmptySlug tests validation with empty slug
func TestValidateCreateProjectRequest_EmptySlug(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	srv := New("8080", nil, "test-api-key", cfg, logger)

	rr := httptest.NewRecorder()

	req := &models.CreateProjectRequest{
		Name: "Test Project",
		Slug: "",
	}

	result := srv.validateCreateProjectRequest(rr, req)
	assert.False(t, result)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestValidateProjectFieldLengths_TooLong tests validation with too long fields
func TestValidateProjectFieldLengths_TooLong(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name        string
		fieldName   string
		value       string
		maxLen      int
		expectValid bool
	}{
		{"Name within limit", "name", strings.Repeat("a", 255), 255, true},
		{"Name exceeds limit", "name", strings.Repeat("a", 256), 255, false},
		{"Slug within limit", "slug", strings.Repeat("a", 100), 100, true},
		{"Slug exceeds limit", "slug", strings.Repeat("a", 101), 100, false},
		{"Description within limit", "desc", strings.Repeat("a", 1000), 1000, true},
		{"Description exceeds limit", "desc", strings.Repeat("a", 1001), 1000, false},
		{"Empty value", "name", "", 255, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			result := srv.validateProjectStringLength(rr, tt.value, tt.maxLen, tt.fieldName)
			assert.Equal(t, tt.expectValid, result)
		})
	}
}

// TestDecodeProjectSlug_ValidSlug tests successful slug decoding
func TestDecodeProjectSlug_ValidSlug(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	srv := New("8080", nil, "test-api-key", cfg, logger)

	rr := httptest.NewRecorder()

	decodedSlug, ok := srv.decodeProjectSlug(rr, "user-123", "test", "test-project")
	assert.True(t, ok)
	assert.Equal(t, "test-project", decodedSlug)
}

// TestDecodeProjectSlug_EncodedSlug tests URL-encoded slug decoding
func TestDecodeProjectSlug_EncodedSlug(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	srv := New("8080", nil, "test-api-key", cfg, logger)

	rr := httptest.NewRecorder()

	// URL-encoded slug with space
	decodedSlug, ok := srv.decodeProjectSlug(rr, "user-123", "test", "test%20project")
	assert.True(t, ok)
	assert.Equal(t, "test project", decodedSlug)
}

// TestProjectFilters_Structure tests ProjectFilters struct fields
func TestProjectFilters_Structure(t *testing.T) {
	filters := services.ProjectFilters{
		TeamID:    "team-123",
		Search:    "search-term",
		SortBy:    "name",
		SortOrder: "asc",
		Page:      2,
		Limit:     50,
	}

	assert.Equal(t, "team-123", filters.TeamID)
	assert.Equal(t, "search-term", filters.Search)
	assert.Equal(t, "name", filters.SortBy)
	assert.Equal(t, "asc", filters.SortOrder)
	assert.Equal(t, 2, filters.Page)
	assert.Equal(t, 50, filters.Limit)
}

// MockProjectStatsContainer is a minimal container for project stats handler tests.
type MockProjectStatsContainer struct {
	BaseMockContainer
	mock.Mock
	projectService *svcmocks.MockProjectServiceInterface
}

func (m *MockProjectStatsContainer) ProjectService() services.ProjectServiceInterface {
	return m.projectService
}

// createTestProjectStatsServer builds a minimal chi router with only the stats route wired up.
func createTestProjectStatsServer(c *MockProjectStatsContainer) *Server {
	cfg := &config.Config{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: c,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	r.Route("/api/v1/{team_id}/projects", func(r chi.Router) {
		r.Get("/{slug}/stats", srv.handleGetProjectStats)
	})

	return srv
}

// TestHandleGetProjectStats_Unauthorized verifies that unauthenticated requests return 401
// via the full server routing (auth middleware runs before the handler).
func TestHandleGetProjectStats_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/projects/test-project/stats", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestHandleGetProjectStats_RouteRegistered ensures the route is not 404.
func TestHandleGetProjectStats_RouteRegistered(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/projects/test-project/stats", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.NotEqual(t, http.StatusNotFound, rr.Code, "stats route should be registered")
}

// TestHandleGetProjectStats_Success tests the happy path: service returns stats, handler encodes JSON.
func TestHandleGetProjectStats_Success(t *testing.T) {
	mockSvc := svcmocks.NewMockProjectServiceInterface(t)
	c := &MockProjectStatsContainer{projectService: mockSvc}

	expected := &models.ProjectStatsResponse{
		TotalPrompts:    5,
		TotalArtifacts:  3,
		TotalBlueprints: 2,
		TotalMemories:   7,
		TotalFeedItems:  1,
	}
	mockSvc.EXPECT().GetProjectStats("team-abc", "user-xyz", "my-project").Return(expected, nil).Once()

	srv := createTestProjectStatsServer(c)
	req := httptest.NewRequest("GET", "/api/v1/team-abc/projects/my-project/stats", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-xyz"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var got models.ProjectStatsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, expected.TotalPrompts, got.TotalPrompts)
	assert.Equal(t, expected.TotalArtifacts, got.TotalArtifacts)
	assert.Equal(t, expected.TotalBlueprints, got.TotalBlueprints)
	assert.Equal(t, expected.TotalMemories, got.TotalMemories)
	assert.Equal(t, expected.TotalFeedItems, got.TotalFeedItems)

	mockSvc.AssertExpectations(t)
}

// TestHandleGetProjectStats_NotFound verifies that ErrProjectNotFoundForRepo maps to 404.
func TestHandleGetProjectStats_NotFound(t *testing.T) {
	mockSvc := svcmocks.NewMockProjectServiceInterface(t)
	c := &MockProjectStatsContainer{projectService: mockSvc}

	mockSvc.EXPECT().GetProjectStats("team-abc", "user-xyz", "missing-project").
		Return(nil, fmt.Errorf("%w: slug=missing-project team=team-abc", repositories.ErrProjectNotFoundForRepo)).Once()

	srv := createTestProjectStatsServer(c)
	req := httptest.NewRequest("GET", "/api/v1/team-abc/projects/missing-project/stats", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-xyz"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	mockSvc.AssertExpectations(t)
}

// TestHandleGetProjectStats_InternalError verifies that unexpected service errors map to 500.
func TestHandleGetProjectStats_InternalError(t *testing.T) {
	mockSvc := svcmocks.NewMockProjectServiceInterface(t)
	c := &MockProjectStatsContainer{projectService: mockSvc}

	mockSvc.EXPECT().GetProjectStats(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("database timeout")).Once()

	srv := createTestProjectStatsServer(c)
	req := httptest.NewRequest("GET", "/api/v1/team-abc/projects/any-project/stats", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-xyz"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}

// TestHandleGetProjectStats_AllZeroCounts verifies that zero counts are correctly serialized.
func TestHandleGetProjectStats_AllZeroCounts(t *testing.T) {
	mockSvc := svcmocks.NewMockProjectServiceInterface(t)
	c := &MockProjectStatsContainer{projectService: mockSvc}

	mockSvc.EXPECT().GetProjectStats("team-abc", "user-xyz", "empty-project").
		Return(&models.ProjectStatsResponse{}, nil).Once()

	srv := createTestProjectStatsServer(c)
	req := httptest.NewRequest("GET", "/api/v1/team-abc/projects/empty-project/stats", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-xyz"))
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var got models.ProjectStatsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, 0, got.TotalPrompts)
	assert.Equal(t, 0, got.TotalArtifacts)
	assert.Equal(t, 0, got.TotalBlueprints)
	assert.Equal(t, 0, got.TotalMemories)
	assert.Equal(t, 0, got.TotalFeedItems)

	mockSvc.AssertExpectations(t)
}
