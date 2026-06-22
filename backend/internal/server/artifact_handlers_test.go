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
	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

//nolint:funlen // Test function requires comprehensive setup for multiple scenarios
func TestArtifactHandlers_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{
			"Create Artifact - Unauthorized", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts", http.StatusUnauthorized,
		},
		{
			"List Artifacts - Unauthorized", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts", http.StatusUnauthorized,
		},
		{
			"Get Artifact Stats - Unauthorized", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/stats", http.StatusUnauthorized,
		},
		{
			"Get Artifact Projects - Unauthorized", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/projects", http.StatusUnauthorized,
		},
		{
			"List Artifacts by Project - Unauthorized", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/test-project",
			http.StatusUnauthorized,
		},
		{
			"Get Artifact - Unauthorized", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/test-project/test-slug",
			http.StatusUnauthorized,
		},
		{
			"Update Artifact - Unauthorized", "PUT",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/test-project/test-slug",
			http.StatusUnauthorized,
		},
		{
			"Delete Artifact - Unauthorized", "DELETE",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/test-project/test-slug",
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(`{"slug":"test-slug","title":"Test Artifact","content":"Test content"}`)
			req, err := http.NewRequest(tt.method, tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestCreateArtifact_BadRequest(t *testing.T) {
	srv := testServer()
	runTestCases(t, srv, artifactBadRequestCases("Bearer valid-token"))
}

func TestUpdateArtifact_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		body     string
		expected int
	}{
		{"Invalid JSON", `{"invalid": json}`, http.StatusUnauthorized},
		{"Project name too long", `{"project_id":"` + strings.Repeat("a", 81) + `"}`, http.StatusUnauthorized},
		{"Slug too long", `{"slug":"` + strings.Repeat("a", 256) + `"}`, http.StatusUnauthorized},
		{"Title too long", `{"title":"` + strings.Repeat("a", 256) + `"}`, http.StatusUnauthorized},
		{"Description too long", `{"description":"` + strings.Repeat("a", 501) + `"}`, http.StatusUnauthorized},
		{"Invalid type", `{"type":"invalid"}`, http.StatusUnauthorized},
		{"Valid type work_reports", `{"type":"work_reports"}`, http.StatusUnauthorized},
		{"Valid type static_contexts", `{"type":"static_contexts"}`, http.StatusUnauthorized},
		{"Valid type general", `{"type":"general"}`, http.StatusUnauthorized},
		{"Invalid status", `{"status":"invalid"}`, http.StatusUnauthorized},
		{"Valid status active", `{"status":"active"}`, http.StatusUnauthorized},
		{"Valid status draft", `{"status":"draft"}`, http.StatusUnauthorized},
		{"Valid status archived", `{"status":"archived"}`, http.StatusUnauthorized},
		{"Valid partial update", `{"title":"Updated Title","description":"Updated Description"}`, http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(tt.body)
			url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/test-project/test-slug"
			req, err := http.NewRequest("PUT", url, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid-token")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			// These should be unauthorized since we don't have proper auth setup
			// In a real integration test environment, we would set up proper authentication
			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestArtifactHandlers_QueryParameters(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"List artifacts with project filter", "GET", "/api/v1/artifacts?project_name=test-project", http.StatusUnauthorized},
		{"List artifacts with status filter", "GET", "/api/v1/artifacts?status=active", http.StatusUnauthorized},
		{"List artifacts with type filter", "GET", "/api/v1/artifacts?type=general", http.StatusUnauthorized},
		{"List artifacts with search", "GET", "/api/v1/artifacts?search=test", http.StatusUnauthorized},
		{
			"List artifacts with sort by created_at",
			"GET",
			"/api/v1/artifacts?sort_by=created_at&sort_order=asc",
			http.StatusUnauthorized,
		},
		{
			"List artifacts with sort by updated_at",
			"GET",
			"/api/v1/artifacts?sort_by=updated_at&sort_order=desc",
			http.StatusUnauthorized,
		},
		{"List artifacts with metadata filter", "GET", "/api/v1/artifacts?metadata_key=value", http.StatusUnauthorized},
		{"List artifacts with pagination", "GET", "/api/v1/artifacts?page=2&limit=10", http.StatusUnauthorized},
		{"List artifacts with max limit", "GET", "/api/v1/artifacts?limit=100", http.StatusUnauthorized},
		{
			"List artifacts with multiple filters",
			"GET",
			"/api/v1/artifacts?project_name=test&status=active&type=general&search=test" +
				"&sort_by=created_at&sort_order=desc&page=1&limit=20",
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer valid-token")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			// These should be unauthorized since we don't have proper auth setup
			// In a real integration test environment, we would set up proper authentication
			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestArtifactHandlers_InvalidPaths(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{
			"Invalid path", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/invalid/path/too/long",
			http.StatusUnauthorized,
		},
		{
			"Method not allowed", "PATCH",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts", http.StatusUnauthorized,
		},
		{
			"Invalid artifact path", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/project/",
			http.StatusUnauthorized,
		},
		{
			"Invalid stats path", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/stats", http.StatusUnauthorized,
		},
		{
			"Invalid projects path", "PUT",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/projects",
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(`{"test":"data"}`)
			req, err := http.NewRequest(tt.method, tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestArtifactHandlers_ContentTypeValidation(t *testing.T) {
	srv := testServer()

	testCasesWithCT := []struct {
		name        string
		method      string
		path        string
		contentType string
		body        string
	}{
		{
			"Create with application/json", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts", "application/json",
			`{"slug":"test","title":"Test","content":"Test"}`,
		},
		{
			"Create with text/plain", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts", "text/plain",
			`{"slug":"test","title":"Test","content":"Test"}`,
		},
		{
			"Create without content-type", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts", "",
			`{"slug":"test","title":"Test","content":"Test"}`,
		},
		{
			"Update with application/json", "PUT", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/project/slug",
			"application/json", `{"title":"Updated"}`,
		},
	}

	for _, tt := range testCasesWithCT {
		t.Run(tt.name, func(t *testing.T) {
			rr := makeRequest(t, srv, testRequest{
				Method:        tt.method,
				Path:          tt.path,
				Body:          tt.body,
				ContentType:   tt.contentType,
				Authorization: "Bearer valid-token",
				SkipCT:        tt.contentType == "",
			})
			assertStatus(t, rr.Code, http.StatusUnauthorized)
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup for multiple route scenarios
func TestArtifactHandlers_RouteMatching(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		// Basic CRUD operations
		{
			"POST to root", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts", http.StatusUnauthorized,
		},
		{
			"GET list all", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts", http.StatusUnauthorized,
		},
		{
			"GET stats", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/stats", http.StatusUnauthorized,
		},
		{
			"GET projects", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/projects", http.StatusUnauthorized,
		},
		{
			"GET by project", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/my-project",
			http.StatusUnauthorized,
		},
		{
			"GET specific artifact", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/my-project/my-slug",
			http.StatusUnauthorized,
		},
		{
			"PUT update artifact", "PUT",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/my-project/my-slug",
			http.StatusUnauthorized,
		},
		{
			"DELETE artifact", "DELETE",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/my-project/my-slug",
			http.StatusUnauthorized,
		},

		// Special cases for project names and slugs
		{
			"Project with hyphens", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/my-project-name",
			http.StatusUnauthorized,
		},
		{
			"Slug with hyphens", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/project/my-slug-name",
			http.StatusUnauthorized,
		},
		{
			"Project with underscores", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/my_project",
			http.StatusUnauthorized,
		},
		{
			"Slug with underscores", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/project/my_slug",
			http.StatusUnauthorized,
		},
		{
			"Numeric project", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/123", http.StatusUnauthorized,
		},
		{
			"Numeric slug", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/project/456",
			http.StatusUnauthorized,
		},

		// Edge cases that should not match reserved paths
		{
			"Stats as project name", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/stats/some-slug",
			http.StatusUnauthorized,
		},
		{
			"Projects as project name", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/projects/some-slug",
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(`{"test": "data"}`)
			req, err := http.NewRequest(tt.method, tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code for %s %s: got %v want %v",
					tt.method, tt.path, status, tt.expected)
			}
		})
	}
}

func TestArtifactHandlers_MetadataFiltering(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{"Single metadata filter", "/api/v1/artifacts?metadata_env=production", http.StatusUnauthorized},
		{
			"Multiple metadata filters",
			"/api/v1/artifacts?metadata_env=production&metadata_team=backend",
			http.StatusUnauthorized,
		},
		{"Metadata with special chars", "/api/v1/artifacts?metadata_version=1.0.0", http.StatusUnauthorized},
		{"Metadata with spaces (encoded)", "/api/v1/artifacts?metadata_description=test%20value", http.StatusUnauthorized},
		{
			"Complex metadata filtering",
			"/api/v1/artifacts?metadata_env=prod&metadata_region=us-east&project_name=test&status=active",
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer valid-token")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestArtifactHandlers_LargeBodies(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		bodySize int
		expected int
	}{
		{
			"Normal sized body", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts", 1024, http.StatusUnauthorized,
		},
		{
			"Large content", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts", 10240, http.StatusUnauthorized,
		},
		{
			"Very large content", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts", 102400, http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := strings.Repeat("a", tt.bodySize)
			bodyJSON := `{"slug":"test-slug","title":"Test Artifact","content":"` + content + `"}`
			body := strings.NewReader(bodyJSON)

			req, err := http.NewRequest(tt.method, tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid-token")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func getURLEncodingTestCases() []testCase {
	return []testCase{
		{
			Name:          "GET artifact with encoded forward slash in project name",
			Method:        "GET",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/shaharialab%2Fvibexp.io/test-slug",
			Authorization: "Bearer valid-token",
			Expected:      http.StatusUnauthorized,
		},
		{
			Name:          "GET artifact with encoded forward slash in both",
			Method:        "GET",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/org%2Frepo/path%2Fto%2Ffile",
			Authorization: "Bearer valid-token",
			Expected:      http.StatusUnauthorized,
		},
		{
			Name:          "PUT artifact with encoded forward slash",
			Method:        "PUT",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/shaharialab%2Fvibexp.io/test-slug",
			Body:          `{"title":"Updated Test Artifact","content":"Updated content"}`,
			Authorization: "Bearer valid-token",
			Expected:      http.StatusUnauthorized,
		},
		{
			Name:          "DELETE artifact with encoded forward slash",
			Method:        "DELETE",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/shaharialab%2Fvibexp.io/test-slug",
			Authorization: "Bearer valid-token",
			Expected:      http.StatusUnauthorized,
		},
		{
			Name:          "GET artifacts by project with encoded slash",
			Method:        "GET",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/shaharialab%2Fvibexp.io",
			Authorization: "Bearer valid-token",
			Expected:      http.StatusUnauthorized,
		},
		{
			Name:          "GET artifact with double encoded forward slash",
			Method:        "GET",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/shaharialab%252Fvibexp.io/test-slug",
			Authorization: "Bearer valid-token",
			Expected:      http.StatusUnauthorized,
		},
		{
			Name:          "GET artifact with encoded special characters",
			Method:        "GET",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/org%2Frepo/file%20with%20spaces%26symbols",
			Authorization: "Bearer valid-token",
			Expected:      http.StatusUnauthorized,
		},
		{
			Name:          "PUT artifact with encoded spaces and symbols",
			Method:        "PUT",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/my%20org%2Fproject/my%20slug%26version",
			Body:          `{"title":"Updated Test Artifact","content":"Updated content"}`,
			Authorization: "Bearer valid-token",
			Expected:      http.StatusUnauthorized,
		},
	}
}

func TestArtifactHandlers_URLEncoding(t *testing.T) {
	srv := testServer()
	runTestCases(t, srv, getURLEncodingTestCases())
}

// Integration tests with mocked services
// Following the pattern from DL-303 (prompt handlers integration tests)

// MockArtifactContainer is a mock container for artifact handler integration tests
type MockArtifactContainer struct {
	BaseMockContainer        // Embed base container for default nil implementations
	ArtifactServiceMock      services.ArtifactServiceInterface
	ResourceUsageServiceMock services.ResourceUsageServiceInterface
	EmbeddingServiceMock     services.EmbeddingServiceInterface
	ActivityServiceMock      activities.ActivityService
	AuthServiceMock          services.AuthServiceInterface
	TeamServiceMock          services.TeamServiceInterface
	TypeServiceMock          services.TypeServiceInterface
}

func (m *MockArtifactContainer) ArtifactService() services.ArtifactServiceInterface {
	return m.ArtifactServiceMock
}

// TypeService returns the configured type-service mock, or a permissive stub
// that accepts every type. Most artifact handler tests do not exercise type
// validation (covered by handlers_types_test.go / TypeService tests); the stub
// keeps the validateArtifactType lookup from nil-panicking. Set TypeServiceMock
// to assert specific validation behavior.
func (m *MockArtifactContainer) TypeService() services.TypeServiceInterface {
	if m.TypeServiceMock != nil {
		return m.TypeServiceMock
	}
	return permissiveTypeService{}
}

// permissiveTypeService is a test stub that treats every type as valid.
type permissiveTypeService struct{}

func (permissiveTypeService) List(context.Context, string, string) ([]models.Type, error) {
	return nil, nil
}

func (permissiveTypeService) CreateCustom(context.Context, services.CreateTypeParams) (*models.Type, error) {
	return nil, nil
}

func (permissiveTypeService) Delete(context.Context, string, string) error { return nil }

func (permissiveTypeService) ValidateType(context.Context, string, string, string) (bool, error) {
	return true, nil
}

func (m *MockArtifactContainer) AuthService() services.AuthServiceInterface {
	return m.AuthServiceMock
}

func (m *MockArtifactContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return m.ResourceUsageServiceMock
}

func (m *MockArtifactContainer) EmbeddingService() services.EmbeddingServiceInterface {
	return m.EmbeddingServiceMock
}

func (m *MockArtifactContainer) ActivityService() activities.ActivityService {
	return m.ActivityServiceMock
}

// Implement all other container methods
func (m *MockArtifactContainer) TeamService() services.TeamServiceInterface {
	return m.TeamServiceMock
}

// Ensure MockArtifactContainer implements container.Container
var _ container.Container = (*MockArtifactContainer)(nil)

// Helper function to create authenticated request with user context
//
//nolint:unparam // userID is kept as parameter for consistency
func createAuthenticatedRequest(method, path, body, userID string) *http.Request {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	// Set the context directly for the handler - this bypasses middleware
	ctx := context.WithValue(req.Context(), contextKeyUserID, userID)
	return req.WithContext(ctx)
}

// TestHandleListArtifacts_Success_WithMockedService tests successful artifact listing with mocked service
//
//nolint:funlen // Comprehensive test with mocked service
func TestHandleListArtifacts_Success_WithMockedService(t *testing.T) {
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)

	teamID := "550e8400-e29b-41d4-a716-446655440000"
	expectedArtifacts := []models.Artifact{
		{
			ID:        "art-1",
			ProjectID: "test-project",
			Slug:      "test-slug-1",
			Title:     "Test Artifact 1",
			Content:   "Content 1",
			UserID:    "user-123",
			Type:      "general",
			Status:    "active",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:        "art-2",
			ProjectID: "test-project",
			Slug:      "test-slug-2",
			Title:     "Test Artifact 2",
			Content:   "Content 2",
			UserID:    "user-123",
			Type:      "work_reports",
			Status:    "active",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	expectedResponse := &models.ArtifactListResponse{
		Artifacts:  expectedArtifacts,
		TotalCount: 2,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}

	mockArtifactService.On("ListArtifacts", "user-123", mock.MatchedBy(func(filters services.ArtifactFilters) bool {
		return filters.ProjectID == "test-project" && filters.Status == "active" &&
			filters.TeamID == teamID && filters.Page == 1 && filters.Limit == 10
	})).Return(expectedResponse, nil)

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock: mockArtifactService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	url := "/api/v1/artifacts?project_id=test-project&status=active"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	rr := httptest.NewRecorder()

	req = addURLParams(req, map[string]string{"team_id": "550e8400-e29b-41d4-a716-446655440000"})
	srv.handleListArtifacts(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response models.ArtifactListResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, 2, response.TotalCount)
	assert.Equal(t, 1, response.Page)
	assert.Equal(t, 20, response.PerPage)
	assert.Equal(t, 1, response.TotalPages)
	assert.Len(t, response.Artifacts, 2)
	assert.Equal(t, "test-slug-1", response.Artifacts[0].Slug)
	assert.Equal(t, "test-slug-2", response.Artifacts[1].Slug)

	mockArtifactService.AssertExpectations(t)
}

// TestHandleListArtifacts_WithPagination tests pagination in artifact listing
func TestHandleListArtifacts_WithPagination(t *testing.T) {
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)

	expectedResponse := &models.ArtifactListResponse{
		Artifacts:  []models.Artifact{{ID: "art-1", Slug: "test-1"}},
		TotalCount: 50,
		Page:       2,
		PerPage:    10,
		TotalPages: 5,
	}

	mockArtifactService.On("ListArtifacts", "user-123", mock.MatchedBy(func(filters services.ArtifactFilters) bool {
		return filters.Page == 2 && filters.Limit == 10 && filters.TeamID == teamID
	})).Return(expectedResponse, nil)

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock: mockArtifactService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	req := createAuthenticatedRequest("GET", "/api/v1/artifacts?team_id="+teamID+"&page=2&limit=10", "", "user-123")
	req = addURLParams(req, map[string]string{"team_id": "550e8400-e29b-41d4-a716-446655440000"})
	rr := httptest.NewRecorder()

	srv.handleListArtifacts(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.ArtifactListResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, 50, response.TotalCount)
	assert.Equal(t, 2, response.Page)
	assert.Equal(t, 10, response.PerPage)
	assert.Equal(t, 5, response.TotalPages)

	mockArtifactService.AssertExpectations(t)
}

// TestHandleListArtifacts_WithMetadataFilters tests metadata filtering
func TestHandleListArtifacts_WithMetadataFilters(t *testing.T) {
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)

	expectedResponse := &models.ArtifactListResponse{
		Artifacts:  []models.Artifact{{ID: "art-1"}},
		TotalCount: 1,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}

	mockArtifactService.On("ListArtifacts", "user-123", mock.MatchedBy(func(filters services.ArtifactFilters) bool {
		return filters.Metadata["env"] == "production" && filters.Metadata["team"] == "backend" && filters.TeamID == teamID
	})).Return(expectedResponse, nil)

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock: mockArtifactService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	req := createAuthenticatedRequest(
		"GET",
		"/api/v1/artifacts?team_id="+teamID+"&metadata_env=production&metadata_team=backend",
		"",
		"user-123",
	)
	req = addURLParams(req, map[string]string{"team_id": "550e8400-e29b-41d4-a716-446655440000"})
	rr := httptest.NewRecorder()

	srv.handleListArtifacts(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	mockArtifactService.AssertExpectations(t)
}

// TestHandleListArtifacts_ServiceError tests error handling
func TestHandleListArtifacts_ServiceError(t *testing.T) {
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)

	mockArtifactService.On("ListArtifacts", "user-123", mock.Anything).
		Return((*models.ArtifactListResponse)(nil), errors.New("database error"))

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock: mockArtifactService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = mockContainer

	req := createAuthenticatedRequest("GET", "/api/v1/artifacts?team_id="+teamID, "", "user-123")
	req = addURLParams(req, map[string]string{"team_id": "550e8400-e29b-41d4-a716-446655440000"})
	rr := httptest.NewRecorder()

	srv.handleListArtifacts(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	mockArtifactService.AssertExpectations(t)
}

// TestValidateArtifactStatus pins the artifact lifecycle enum: active, draft and
// archived are accepted (and an empty/omitted status is allowed), while the
// retired "expired" value and any unknown value are rejected with a 400.
func TestValidateArtifactStatus(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name      string
		status    *string
		wantValid bool
	}{
		{name: "nil status allowed", status: nil, wantValid: true},
		{name: "empty status allowed", status: stringPtr(""), wantValid: true},
		{name: "active accepted", status: stringPtr("active"), wantValid: true},
		{name: "draft accepted", status: stringPtr("draft"), wantValid: true},
		{name: "archived accepted", status: stringPtr("archived"), wantValid: true},
		{name: "retired expired rejected", status: stringPtr("expired"), wantValid: false},
		{name: "unknown rejected", status: stringPtr("bogus"), wantValid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			ok := srv.validateArtifactStatus(rr, tt.status)
			assert.Equal(t, tt.wantValid, ok)
			if !tt.wantValid {
				assert.Equal(t, http.StatusBadRequest, rr.Code)
			}
		})
	}
}
