package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// =============================================================================
// Project+GitHub test container
// =============================================================================

// projectGitHubTestContainer wires both ProjectService and GitHubAppService for
// testing the handleListProjects handler's GitHub enrichment logic.
type projectGitHubTestContainer struct {
	BaseMockContainer
	projectSvc   *svcmocks.MockProjectServiceInterface
	githubAppSvc *svcmocks.MockGitHubAppServiceInterface
}

func (c *projectGitHubTestContainer) ProjectService() services.ProjectServiceInterface {
	return c.projectSvc
}

func (c *projectGitHubTestContainer) GitHubAppService() services.GitHubAppServiceInterface {
	return c.githubAppSvc
}

func newProjectGitHubTestContainer(t *testing.T) *projectGitHubTestContainer {
	t.Helper()
	return &projectGitHubTestContainer{
		projectSvc:   svcmocks.NewMockProjectServiceInterface(t),
		githubAppSvc: svcmocks.NewMockGitHubAppServiceInterface(t),
	}
}

// createProjectGitHubServer creates a minimal server for direct handler invocation.
// Routes are NOT set up; tests call handlers directly to avoid middleware.
func createProjectGitHubServer(container *projectGitHubTestContainer) *Server {
	cfg := &config.Config{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	r := chi.NewRouter()
	return &Server{
		port:      "8080",
		container: container,
		logger:    logger,
		config:    cfg,
		router:    r,
	}
}

// makeListProjectsRequest creates a GET request for the test team with userID and
// team_id injected into the context so that the handler can read them without middleware.
func makeListProjectsRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/"+listProjTeamID+"/projects", nil)

	// Inject userID (normally set by auth middleware)
	ctx := context.WithValue(req.Context(), contextKeyUserID, "user-test-123")

	// Inject chi URL params (normally set by the router)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("team_id", listProjTeamID)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	return req.WithContext(ctx)
}

// =============================================================================
// Tests
// =============================================================================

const listProjTeamID = "550e8400-e29b-41d4-a716-446655440099"

// baseProjectListResponse builds a ProjectListResponse with three projects:
// - p1 has a git_url pointing to a GitHub repo
// - p2 has no git_url
// - p3 has a git_url that does NOT match any accessible repo
func baseProjectListResponse() *models.ProjectListResponse {
	return &models.ProjectListResponse{
		Projects: []models.ProjectResponse{
			{
				Project:         models.Project{ID: "p1", Name: "Repo Project", GitURL: "https://github.com/org/repo-one"},
				GitHubConnected: false,
			},
			{
				Project:         models.Project{ID: "p2", Name: "No Git Project", GitURL: ""},
				GitHubConnected: false,
			},
			{
				Project:         models.Project{ID: "p3", Name: "Other Git Project", GitURL: "https://github.com/org/other-repo"},
				GitHubConnected: false,
			},
		},
		TotalCount: 3,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}
}

// TestHandleListProjects_GitHubConnected_MatchingURL verifies that a project whose
// git_url matches an accessible GitHub repository has github_connected set to true.
func TestHandleListProjects_GitHubConnected_MatchingURL(t *testing.T) {
	container := newProjectGitHubTestContainer(t)

	container.projectSvc.On("ListProjects", "user-test-123", mock.AnythingOfType("services.ProjectFilters")).
		Return(baseProjectListResponse(), nil)

	// Only repo-one is accessible
	container.githubAppSvc.On("GetAccessibleRepoURLs", mock.Anything, listProjTeamID).
		Return(map[string]bool{"https://github.com/org/repo-one": true}, nil)

	srv := createProjectGitHubServer(container)
	rr := httptest.NewRecorder()
	srv.handleListProjects(rr, makeListProjectsRequest())

	require.Equal(t, http.StatusOK, rr.Code)

	var resp models.ProjectListResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	require.Len(t, resp.Projects, 3)

	// p1 matches an accessible repo
	assert.True(t, resp.Projects[0].GitHubConnected, "p1 should be github_connected")
	// p2 has no git_url
	assert.False(t, resp.Projects[1].GitHubConnected, "p2 should not be github_connected (no git_url)")
	// p3 has a git_url but it's not in the accessible set
	assert.False(t, resp.Projects[2].GitHubConnected, "p3 should not be github_connected (url not accessible)")
}

// TestHandleListProjects_GitHubConnected_NoInstallation verifies that all projects have
// github_connected=false when no GitHub installation exists (empty map returned without error).
func TestHandleListProjects_GitHubConnected_NoInstallation(t *testing.T) {
	container := newProjectGitHubTestContainer(t)

	container.projectSvc.On("ListProjects", "user-test-123", mock.AnythingOfType("services.ProjectFilters")).
		Return(baseProjectListResponse(), nil)

	// Empty map — no installation
	container.githubAppSvc.On("GetAccessibleRepoURLs", mock.Anything, listProjTeamID).
		Return(map[string]bool{}, nil)

	srv := createProjectGitHubServer(container)
	rr := httptest.NewRecorder()
	srv.handleListProjects(rr, makeListProjectsRequest())

	require.Equal(t, http.StatusOK, rr.Code)

	var resp models.ProjectListResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	require.Len(t, resp.Projects, 3)

	for _, p := range resp.Projects {
		assert.False(t, p.GitHubConnected, "all projects should have github_connected=false when no installation")
	}
}

// TestHandleListProjects_GitHubConnected_GitHubServiceError verifies that a failure in
// GetAccessibleRepoURLs causes the endpoint to still return 200 with all github_connected=false.
func TestHandleListProjects_GitHubConnected_GitHubServiceError(t *testing.T) {
	container := newProjectGitHubTestContainer(t)

	container.projectSvc.On("ListProjects", "user-test-123", mock.AnythingOfType("services.ProjectFilters")).
		Return(baseProjectListResponse(), nil)

	// GitHub service returns an error
	container.githubAppSvc.On("GetAccessibleRepoURLs", mock.Anything, listProjTeamID).
		Return(nil, errors.New("github api unavailable"))

	srv := createProjectGitHubServer(container)
	rr := httptest.NewRecorder()
	srv.handleListProjects(rr, makeListProjectsRequest())

	// Must not fail the entire request
	require.Equal(t, http.StatusOK, rr.Code)

	var resp models.ProjectListResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	require.Len(t, resp.Projects, 3)

	for _, p := range resp.Projects {
		assert.False(t, p.GitHubConnected, "all projects should default to github_connected=false on error")
	}
}

// TestHandleListProjects_GitHubConnected_URLNormalization verifies that git_url values
// with .git suffix or trailing slashes are matched correctly against the repo URL set.
func TestHandleListProjects_GitHubConnected_URLNormalization(t *testing.T) {
	container := newProjectGitHubTestContainer(t)

	resp := &models.ProjectListResponse{
		Projects: []models.ProjectResponse{
			// git_url has .git suffix — should match normalized accessible URL
			{Project: models.Project{ID: "p1", GitURL: "https://github.com/org/repo-one.git"}, GitHubConnected: false},
			// git_url has trailing slash — should also match
			{Project: models.Project{ID: "p2", GitURL: "https://github.com/org/repo-two/"}, GitHubConnected: false},
			// git_url with .git and slash — should also match
			{Project: models.Project{ID: "p3", GitURL: "https://github.com/org/repo-three.git/"}, GitHubConnected: false},
		},
		TotalCount: 3,
		Page:       1,
		PerPage:    20,
		TotalPages: 1,
	}
	container.projectSvc.On("ListProjects", "user-test-123", mock.AnythingOfType("services.ProjectFilters")).
		Return(resp, nil)

	// Accessible set uses clean URLs (already normalized by GetAccessibleRepoURLs)
	container.githubAppSvc.On("GetAccessibleRepoURLs", mock.Anything, listProjTeamID).
		Return(map[string]bool{
			"https://github.com/org/repo-one":   true,
			"https://github.com/org/repo-two":   true,
			"https://github.com/org/repo-three": true,
		}, nil)

	srv := createProjectGitHubServer(container)
	rr := httptest.NewRecorder()
	srv.handleListProjects(rr, makeListProjectsRequest())

	require.Equal(t, http.StatusOK, rr.Code)

	var result models.ProjectListResponse
	err := json.NewDecoder(rr.Body).Decode(&result)
	require.NoError(t, err)
	require.Len(t, result.Projects, 3)

	assert.True(t, result.Projects[0].GitHubConnected, "p1 (.git suffix) should be github_connected")
	assert.True(t, result.Projects[1].GitHubConnected, "p2 (trailing slash) should be github_connected")
	assert.True(t, result.Projects[2].GitHubConnected, "p3 (.git + slash) should be github_connected")
}

// TestHandleListProjects_GitHubConnected_EmptyProjectList verifies an empty project list
// returns a valid empty response without panicking or erroring.
func TestHandleListProjects_GitHubConnected_EmptyProjectList(t *testing.T) {
	container := newProjectGitHubTestContainer(t)

	emptyResp := &models.ProjectListResponse{
		Projects:   []models.ProjectResponse{},
		TotalCount: 0,
		Page:       1,
		PerPage:    20,
		TotalPages: 0,
	}
	container.projectSvc.On("ListProjects", "user-test-123", mock.AnythingOfType("services.ProjectFilters")).
		Return(emptyResp, nil)

	container.githubAppSvc.On("GetAccessibleRepoURLs", mock.Anything, listProjTeamID).
		Return(map[string]bool{}, nil)

	srv := createProjectGitHubServer(container)
	rr := httptest.NewRecorder()
	srv.handleListProjects(rr, makeListProjectsRequest())

	require.Equal(t, http.StatusOK, rr.Code)

	var result models.ProjectListResponse
	err := json.NewDecoder(rr.Body).Decode(&result)
	require.NoError(t, err)
	assert.Empty(t, result.Projects)
}

// TestHandleListProjects_GitHubConnected_AllProjectsConnected verifies that when all
// project git_urls are in the accessible set, all are marked github_connected=true.
func TestHandleListProjects_GitHubConnected_AllProjectsConnected(t *testing.T) {
	container := newProjectGitHubTestContainer(t)

	container.projectSvc.On("ListProjects", "user-test-123", mock.AnythingOfType("services.ProjectFilters")).
		Return(&models.ProjectListResponse{
			Projects: []models.ProjectResponse{
				{Project: models.Project{ID: "p1", GitURL: "https://github.com/org/repo-one"}, GitHubConnected: false},
				{Project: models.Project{ID: "p2", GitURL: "https://github.com/org/repo-two"}, GitHubConnected: false},
			},
			TotalCount: 2,
			Page:       1,
			PerPage:    20,
			TotalPages: 1,
		}, nil)

	container.githubAppSvc.On("GetAccessibleRepoURLs", mock.Anything, listProjTeamID).
		Return(map[string]bool{
			"https://github.com/org/repo-one": true,
			"https://github.com/org/repo-two": true,
		}, nil)

	srv := createProjectGitHubServer(container)
	rr := httptest.NewRecorder()
	srv.handleListProjects(rr, makeListProjectsRequest())

	require.Equal(t, http.StatusOK, rr.Code)

	var result models.ProjectListResponse
	err := json.NewDecoder(rr.Body).Decode(&result)
	require.NoError(t, err)
	require.Len(t, result.Projects, 2)

	assert.True(t, result.Projects[0].GitHubConnected)
	assert.True(t, result.Projects[1].GitHubConnected)
}

// TestNormalizeGitURL tests the normalizeGitURL helper in the handler package.
func TestNormalizeGitURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"plain URL", "https://github.com/org/repo", "https://github.com/org/repo"},
		{"trailing slash", "https://github.com/org/repo/", "https://github.com/org/repo"},
		{".git suffix", "https://github.com/org/repo.git", "https://github.com/org/repo"},
		{".git and trailing slash", "https://github.com/org/repo.git/", "https://github.com/org/repo"},
		{"uppercase", "HTTPS://GitHub.com/Org/Repo", "https://github.com/org/repo"},
		{"https no .git", "https://github.com/shaharia/my-project", "https://github.com/shaharia/my-project"},
		{"double .git", "https://github.com/org/repo.git.git", "https://github.com/org/repo.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeGitURL(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
