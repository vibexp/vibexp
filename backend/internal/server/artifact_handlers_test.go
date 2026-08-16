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
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
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
		// No trailing slash: the domain no longer sits behind an
		// `r.Route("/api/v1/{team_id}/artifacts")` prefix subrouter, whose
		// Get("/") used to answer the trailing-slash form too. Every route is
		// registered at full length now, because the generated handler uses
		// absolute paths (#776). Neither the SPA nor the CLI ever sent the
		// trailing slash — both build "/artifacts" exactly — and whether to
		// accept it API-wide is tracked in #800.
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
		{"List artifacts with project filter", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?project_name=test-project", http.StatusUnauthorized},
		{"List artifacts with status filter", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?status=active", http.StatusUnauthorized},
		{"List artifacts with type filter", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?type=general", http.StatusUnauthorized},
		{"List artifacts with search", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?search=test", http.StatusUnauthorized},
		{
			"List artifacts with sort by created_at",
			"GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?sort_by=created_at&sort_order=asc",
			http.StatusUnauthorized,
		},
		{
			"List artifacts with sort by updated_at",
			"GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?sort_by=updated_at&sort_order=desc",
			http.StatusUnauthorized,
		},
		{
			"List artifacts with metadata filter",
			"GET",
			`/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?metadata=%7B%22key%22%3A%5B%22value%22%5D%7D`,
			http.StatusUnauthorized,
		},
		{"List artifacts with pagination", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?page=2&limit=10", http.StatusUnauthorized},
		{"List artifacts with max limit", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?limit=100", http.StatusUnauthorized},
		{
			"List artifacts with multiple filters",
			"GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?project_name=test&status=active&type=general&search=test" +
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
		// These paths and methods match no route, so chi answers from its NotFound/
		// MethodNotAllowed handlers without running the protected group's auth
		// middleware. They reported 401 only because the domain used to sit behind an
		// `r.Route("/api/v1/{team_id}/artifacts")` prefix subrouter, which matched the
		// whole subtree; #776 registers every route at full length because the
		// generated handler uses absolute paths. The routes that DO exist still answer
		// 401 unauthenticated — see TestArtifactHandlers_Unauthorized.
	}{
		{
			"Invalid path", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/invalid/path/too/long",
			http.StatusNotFound,
		},
		{
			"Method not allowed", "PATCH",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts", http.StatusMethodNotAllowed,
		},
		{
			// 401, not 404, since #800: stripAPITrailingSlash normalises this to
			// `/artifacts/project`, which matches a real route and therefore
			// reaches the auth middleware. Making the trailing-slash form
			// equivalent to the bare path is the point of that change; the
			// consequence for a path whose LAST segment was empty is that it now
			// resolves one segment shorter instead of matching nothing.
			"Trailing slash resolves one segment shorter", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/project/",
			http.StatusUnauthorized,
		},
		{
			"Invalid stats path", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/stats", http.StatusMethodNotAllowed,
		},
		{
			// "projects" binds as {project_id}; PUT is only registered on
			// /{project_id}/{slug}, so chi answers MethodNotAllowed.
			"Invalid projects path", "PUT",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/projects",
			http.StatusMethodNotAllowed,
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
		{"Single metadata filter", `/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?metadata=%7B%22env%22%3A%5B%22production%22%5D%7D`, http.StatusUnauthorized},
		{
			"Multiple metadata filters",
			`/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?metadata=%7B%22env%22%3A%5B%22production%22%5D%2C%22team%22%3A%5B%22backend%22%5D%7D`,
			http.StatusUnauthorized,
		},
		{"Metadata with special chars", `/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?metadata=%7B%22version%22%3A%5B%221.0.0%22%5D%7D`, http.StatusUnauthorized},
		{
			"Metadata with spaces (encoded)",
			`/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?metadata=%7B%22description%22%3A%5B%22test%20value%22%5D%7D`,
			http.StatusUnauthorized,
		},
		{
			"Complex metadata filtering",
			`/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?metadata=%7B%22env%22%3A%5B%22prod%22%5D%7D&project_name=test&status=active`,
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
	BaseMockContainer       // Embed base container for default nil implementations
	ArtifactServiceMock     services.ArtifactServiceInterface
	EmbeddingServiceMock    services.EmbeddingServiceInterface
	ActivityServiceMock     activities.ActivityService
	AuthServiceMock         services.AuthServiceInterface
	TeamServiceMock         services.TeamServiceInterface
	TypeServiceMock         services.TypeServiceInterface
	RelationServiceMock     services.RelationServiceInterface
	FreshnessServiceMock    services.FreshnessServiceInterface
	EmbeddingRepositoryMock repositories.EmbeddingRepository
	// ResourceAccessServiceMock lets a test observe the access event the detail
	// read records (#776); the embedded base returns nil, which the middleware
	// treats as "do not record".
	ResourceAccessServiceMock resourceaccess.ResourceAccessService
}

func (m *MockArtifactContainer) ResourceAccessService() resourceaccess.ResourceAccessService {
	return m.ResourceAccessServiceMock
}

// FreshnessService returns the freshness mock when a test installs one; the
// embedded base returns nil otherwise, which the surfacing helpers treat as
// "no freshness" rather than as an error (#735).
func (m *MockArtifactContainer) FreshnessService() services.FreshnessServiceInterface {
	return m.FreshnessServiceMock
}

func (m *MockArtifactContainer) ArtifactService() services.ArtifactServiceInterface {
	return m.ArtifactServiceMock
}

// RelationService returns the configured relation-service mock (nil by default,
// which the detail-GET `related` population treats as an empty neighborhood).
func (m *MockArtifactContainer) RelationService() services.RelationServiceInterface {
	return m.RelationServiceMock
}

func (m *MockArtifactContainer) EmbeddingRepository() repositories.EmbeddingRepository {
	return m.EmbeddingRepositoryMock
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
			ProjectID: "550e8400-e29b-41d4-a716-446655440001",
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
			ProjectID: "550e8400-e29b-41d4-a716-446655440001",
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
		return filters.ProjectID == "550e8400-e29b-41d4-a716-446655440001" && filters.Status == "active" &&
			filters.TeamID == teamID && filters.Page == 1 && filters.Limit == 10
	})).Return(expectedResponse, nil)

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock: mockArtifactService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := mountArtifactReadRoutes(New("8080", nil, "test-api-key", cfg, logger))
	srv.container = mockContainer

	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?project_id=550e8400-e29b-41d4-a716-446655440001&status=active"
	req := createAuthenticatedRequest("GET", url, "", "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

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
		Artifacts:  []models.Artifact{{ID: "art-1", Slug: "test-1", ProjectID: "550e8400-e29b-41d4-a716-446655440001"}},
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
	srv := mountArtifactReadRoutes(New("8080", nil, "test-api-key", cfg, logger))
	srv.container = mockContainer

	req := createAuthenticatedRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?team_id="+teamID+"&page=2&limit=10", "", "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

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

// TestHandleListArtifacts_LegacyMetadataParamsAreInert asserts the removed
// metadata_<key> convention (#526) is IGNORED rather than rejected: the request
// still succeeds and reaches the service with an empty MetadataFilter, exactly
// as if the params had not been supplied.
//
// Inert, not erroring, is the deliberate contract — an old client or bookmarked
// URL degrades to an unfiltered list instead of a 400.
func TestHandleListArtifacts_LegacyMetadataParamsAreInert(t *testing.T) {
	teamID := "550e8400-e29b-41d4-a716-446655440000"
	mockArtifactService := servicesmocks.NewMockArtifactServiceInterface(t)

	expectedResponse := &models.ArtifactListResponse{
		Artifacts:  []models.Artifact{{ID: "art-1", ProjectID: "550e8400-e29b-41d4-a716-446655440001"}},
		TotalCount: 1,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}

	mockArtifactService.On("ListArtifacts", "user-123", mock.MatchedBy(func(filters services.ArtifactFilters) bool {
		return filters.TeamID == teamID && len(filters.MetadataFilter) == 0
	})).Return(expectedResponse, nil)

	mockContainer := &MockArtifactContainer{
		ArtifactServiceMock: mockArtifactService,
	}

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := mountArtifactReadRoutes(New("8080", nil, "test-api-key", cfg, logger))
	srv.container = mockContainer

	req := createAuthenticatedRequest(
		"GET",
		"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?team_id="+teamID+"&metadata_env=production&metadata_team=backend",
		"",
		"user-123",
	)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)
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
	srv := mountArtifactReadRoutes(New("8080", nil, "test-api-key", cfg, logger))
	srv.container = mockContainer

	req := createAuthenticatedRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts?team_id="+teamID, "", "user-123")
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

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

// artifactValidationServer builds an authenticated artifact server whose type
// service accepts every type, so a case reaches the validator it is aimed at.
func artifactValidationServer(t *testing.T) *Server {
	t.Helper()

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	srv.container = &MockArtifactContainer{
		ArtifactServiceMock: servicesmocks.NewMockArtifactServiceInterface(t),
	}

	return srv
}

// TestHandleCreateArtifact_Validation and its update twin replace
// TestCreateArtifact_BadRequest / TestUpdateArtifact_BadRequest (and the shared
// artifactBadRequestCases helper), which built a nil-container server with a
// bogus bearer token: all 33 cases stopped at the auth middleware and asserted
// 401, so no body was ever parsed (#665). TestArtifactHandlers_Unauthorized
// still covers the unauthenticated path for these routes.
func TestHandleCreateArtifact_Validation(t *testing.T) {
	const teamID = "550e8400-e29b-41d4-a716-446655440000"
	const projectID = "550e8400-e29b-41d4-a716-446655440111"

	tests := []struct {
		name          string
		body          string
		expectedError string
	}{
		{
			name:          "Invalid JSON",
			body:          `{"invalid": json}`,
			expectedError: "Invalid request body",
		},
		{
			name:          "Missing project_id",
			body:          `{"slug":"s","title":"t","content":"c"}`,
			expectedError: "project_id is required",
		},
		{
			name:          "project_id is not a UUID",
			body:          `{"project_id":"not-a-uuid","slug":"s","title":"t","content":"c"}`,
			expectedError: "project_id must be a valid UUID",
		},
		{
			name:          "Missing slug",
			body:          `{"project_id":"` + projectID + `","title":"t","content":"c"}`,
			expectedError: "Slug is required",
		},
		{
			name:          "Empty slug",
			body:          `{"project_id":"` + projectID + `","slug":"","title":"t","content":"c"}`,
			expectedError: "Slug is required",
		},
		{
			name:          "Missing title",
			body:          `{"project_id":"` + projectID + `","slug":"s","content":"c"}`,
			expectedError: "Title is required",
		},
		{
			name:          "Empty title",
			body:          `{"project_id":"` + projectID + `","slug":"s","title":"","content":"c"}`,
			expectedError: "Title is required",
		},
		{
			name:          "Missing content",
			body:          `{"project_id":"` + projectID + `","slug":"s","title":"t"}`,
			expectedError: "Content is required",
		},
		{
			name:          "Empty content",
			body:          `{"project_id":"` + projectID + `","slug":"s","title":"t","content":""}`,
			expectedError: "Content is required",
		},
		{
			name: "Slug too long",
			body: `{"project_id":"` + projectID + `","slug":"` + strings.Repeat("a", 256) +
				`","title":"t","content":"c"}`,
			expectedError: "Slug cannot be longer than 255 characters",
		},
		{
			name: "Title too long",
			body: `{"project_id":"` + projectID + `","slug":"s","title":"` + strings.Repeat("a", 256) +
				`","content":"c"}`,
			expectedError: "Title cannot be longer than 255 characters",
		},
		{
			name: "Description too long",
			body: `{"project_id":"` + projectID + `","slug":"s","title":"t","content":"c","description":"` +
				strings.Repeat("a", 501) + `"}`,
			expectedError: "Description cannot be longer than 500 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := artifactValidationServer(t)

			req := createAuthenticatedRequest(
				"POST", "/api/v1/"+teamID+"/artifacts", tt.body, "user-123")
			req = addURLParams(req, map[string]string{"team_id": teamID})
			rr := httptest.NewRecorder()

			srv.handleCreateArtifact(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)

			var response map[string]interface{}
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
			assert.Contains(t, response["detail"], tt.expectedError)
		})
	}
}

func TestHandleUpdateArtifact_Validation(t *testing.T) {
	const teamID = "550e8400-e29b-41d4-a716-446655440000"
	const routeProjectID = "550e8400-e29b-41d4-a716-446655440111"

	tests := []struct {
		name          string
		body          string
		expectedError string
	}{
		{
			name:          "Invalid JSON",
			body:          `{"invalid": json}`,
			expectedError: "Invalid request body",
		},
		{
			name:          "project_id is not a UUID",
			body:          `{"project_id":"not-a-uuid"}`,
			expectedError: "project_id must be a valid UUID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := artifactValidationServer(t)

			// The update route is keyed on {project_id}/{slug}, and the handler
			// validates those BEFORE decoding the body — supply real ones or
			// every case fails on "Invalid project_id format" instead.
			req := createAuthenticatedRequest(
				"PUT", "/api/v1/"+teamID+"/artifacts/"+routeProjectID+"/some-slug", tt.body, "user-123")
			req = addURLParams(req, map[string]string{
				"team_id": teamID, "project_id": routeProjectID, "slug": "some-slug",
			})
			rr := httptest.NewRecorder()

			srv.handleUpdateArtifact(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)

			var response map[string]interface{}
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
			assert.Contains(t, response["detail"], tt.expectedError)
		})
	}
}
