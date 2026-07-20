package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// buildGitHubAppServiceForReposTest constructs a GitHubAppService backed by the
// installation, project, and GitHub client mocks needed by the GetRepositories
// enrichment tests. The other dependencies are nil because GetRepositories
// never reaches them.
func buildGitHubAppServiceForReposTest(
	installationRepo *MockGitHubInstallationRepository,
	projectRepo *MockProjectRepository,
	githubClient *MockGitHubAppClient,
	logger *slog.Logger,
) GitHubAppServiceInterface {
	if logger == nil {
		logger, _ = logtest.New()
	}
	return NewGitHubAppService(
		installationRepo,
		projectRepo,
		nil, // blueprintRepo not needed
		githubClient,
		nil, // encryptionSvc not needed
		nil, // attachmentSvc not needed
		nil, // eventManager not needed
		logger,
	)
}

func sampleInstallationForRepos() *models.GitHubInstallation {
	return &models.GitHubInstallation{
		ID:             "inst-uuid-1",
		TeamID:         "team-123",
		InstallationID: 99999,
		AccountLogin:   "test-org",
	}
}

// TestGetRepositories_EnrichesMatchedRepoWithSlug verifies that a repository whose
// HTMLURL matches an existing project's git_url gets ImportedProjectSlug populated.
func TestGetRepositories_EnrichesMatchedRepoWithSlug(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	projectRepo := &MockProjectRepository{}
	githubClient := &MockGitHubAppClient{}

	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(sampleInstallationForRepos(), nil)

	repos := []models.GitHubRepository{
		{ID: 1, Name: "matched", HTMLURL: "https://github.com/owner/matched"},
	}
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 1).
		Return(repos, 1, nil)

	projectRepo.On("ListGitURLToSlugByTeam", mock.Anything, "team-123", "user-1").
		Return(map[string]string{
			"https://github.com/owner/matched": "matched-slug",
		}, nil)

	svc := buildGitHubAppServiceForReposTest(installationRepo, projectRepo, githubClient, nil)

	got, err := svc.GetRepositories(context.Background(), "team-123", "user-1", 1)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 1, got.TotalCount)
	require.Len(t, got.Repositories, 1)
	assert.Equal(t, "matched-slug", got.Repositories[0].ImportedProjectSlug)

	installationRepo.AssertExpectations(t)
	projectRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
}

// TestGetRepositories_UnmatchedRepoHasEmptySlug verifies that a repository with no
// matching project keeps an empty ImportedProjectSlug.
func TestGetRepositories_UnmatchedRepoHasEmptySlug(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	projectRepo := &MockProjectRepository{}
	githubClient := &MockGitHubAppClient{}

	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(sampleInstallationForRepos(), nil)

	repos := []models.GitHubRepository{
		{ID: 1, Name: "unmatched", HTMLURL: "https://github.com/owner/unmatched"},
	}
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 1).
		Return(repos, 1, nil)

	// Empty map: no project matches this repo.
	projectRepo.On("ListGitURLToSlugByTeam", mock.Anything, "team-123", "user-1").
		Return(map[string]string{}, nil)

	svc := buildGitHubAppServiceForReposTest(installationRepo, projectRepo, githubClient, nil)

	got, err := svc.GetRepositories(context.Background(), "team-123", "user-1", 1)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.Repositories, 1)
	assert.Empty(t, got.Repositories[0].ImportedProjectSlug)

	installationRepo.AssertExpectations(t)
	projectRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
}

// TestGetRepositories_MixedMatchedAndUnmatched verifies a typical mixed page where
// some repos correspond to imported projects and others do not.
func TestGetRepositories_MixedMatchedAndUnmatched(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	projectRepo := &MockProjectRepository{}
	githubClient := &MockGitHubAppClient{}

	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(sampleInstallationForRepos(), nil)

	repos := []models.GitHubRepository{
		{ID: 1, Name: "matched-one", HTMLURL: "https://github.com/owner/matched-one"},
		{ID: 2, Name: "unmatched", HTMLURL: "https://github.com/owner/unmatched"},
		{ID: 3, Name: "matched-two", HTMLURL: "https://github.com/owner/matched-two"},
	}
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 1).
		Return(repos, 3, nil)

	projectRepo.On("ListGitURLToSlugByTeam", mock.Anything, "team-123", "user-1").
		Return(map[string]string{
			"https://github.com/owner/matched-one": "matched-one-slug",
			"https://github.com/owner/matched-two": "matched-two-slug",
			// An extra entry that no current repo references; must not pollute.
			"https://github.com/other/extra": "extra-slug",
		}, nil)

	svc := buildGitHubAppServiceForReposTest(installationRepo, projectRepo, githubClient, nil)

	got, err := svc.GetRepositories(context.Background(), "team-123", "user-1", 1)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.Repositories, 3)
	assert.Equal(t, "matched-one-slug", got.Repositories[0].ImportedProjectSlug)
	assert.Empty(t, got.Repositories[1].ImportedProjectSlug)
	assert.Equal(t, "matched-two-slug", got.Repositories[2].ImportedProjectSlug)

	installationRepo.AssertExpectations(t)
	projectRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
}

// TestGetRepositories_ProjectRepoErrorReturnsReposWithEmptySlugs verifies the
// conservative degradation path: when the project repo lookup fails, the GitHub
// page must still render — we return the GitHub repos with empty slugs and log
// a warning instead of bubbling the error to the user.
func TestGetRepositories_ProjectRepoErrorReturnsReposWithEmptySlugs(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	projectRepo := &MockProjectRepository{}
	githubClient := &MockGitHubAppClient{}

	logger, hook := logtest.New()

	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(sampleInstallationForRepos(), nil)

	repos := []models.GitHubRepository{
		{ID: 1, Name: "repo-one", HTMLURL: "https://github.com/owner/repo-one"},
		{ID: 2, Name: "repo-two", HTMLURL: "https://github.com/owner/repo-two"},
	}
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 1).
		Return(repos, 2, nil)

	projectRepo.On("ListGitURLToSlugByTeam", mock.Anything, "team-123", "user-1").
		Return(nil, errors.New("database connection refused"))

	svc := buildGitHubAppServiceForReposTest(installationRepo, projectRepo, githubClient, logger)

	got, err := svc.GetRepositories(context.Background(), "team-123", "user-1", 1)

	require.NoError(t, err, "project repo error must NOT surface to caller")
	require.NotNil(t, got)
	require.Len(t, got.Repositories, 2)
	assert.Empty(t, got.Repositories[0].ImportedProjectSlug)
	assert.Empty(t, got.Repositories[1].ImportedProjectSlug)

	// Verify a warning was logged with the expected fields.
	require.NotEmpty(t, hook.AllEntries(), "expected a warning log entry")
	var found bool
	for _, e := range hook.AllEntries() {
		if e.Level == slog.LevelWarn && e.Data["service"] == "github-app-service" {
			found = true
			assert.Equal(t, "team-123", e.Data["team_id"])
			break
		}
	}
	assert.True(t, found, "expected a warn-level entry with service=github-app-service")

	installationRepo.AssertExpectations(t)
	projectRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
}

// TestGetRepositories_InstallationLookupError verifies the existing failure mode
// is preserved: an installation lookup error still surfaces, untouched by the
// new enrichment logic.
func TestGetRepositories_InstallationLookupError(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	projectRepo := &MockProjectRepository{}
	githubClient := &MockGitHubAppClient{}

	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(nil, errors.New("db unavailable"))

	svc := buildGitHubAppServiceForReposTest(installationRepo, projectRepo, githubClient, nil)

	got, err := svc.GetRepositories(context.Background(), "team-123", "user-1", 1)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to get installation")

	installationRepo.AssertExpectations(t)
	// The project repo must not be consulted when the installation lookup fails.
	projectRepo.AssertNotCalled(t, "ListGitURLToSlugByTeam")
	githubClient.AssertNotCalled(t, "GetInstallationRepositories")
}

// TestGetRepositories_GitHubAPIError verifies that a GitHub API error short-
// circuits before enrichment is attempted.
func TestGetRepositories_GitHubAPIError(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	projectRepo := &MockProjectRepository{}
	githubClient := &MockGitHubAppClient{}

	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(sampleInstallationForRepos(), nil)

	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 1).
		Return([]models.GitHubRepository{}, 0, errors.New("github api timeout"))

	svc := buildGitHubAppServiceForReposTest(installationRepo, projectRepo, githubClient, nil)

	got, err := svc.GetRepositories(context.Background(), "team-123", "user-1", 1)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to get repositories")

	installationRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
	projectRepo.AssertNotCalled(t, "ListGitURLToSlugByTeam")
}

// TestGetRepositories_EmptyRepoListSkipsProjectLookup verifies the small
// optimisation that an empty page does not even hit the project repo.
func TestGetRepositories_EmptyRepoListSkipsProjectLookup(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	projectRepo := &MockProjectRepository{}
	githubClient := &MockGitHubAppClient{}

	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(sampleInstallationForRepos(), nil)

	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 5).
		Return([]models.GitHubRepository{}, 0, nil)

	svc := buildGitHubAppServiceForReposTest(installationRepo, projectRepo, githubClient, nil)

	got, err := svc.GetRepositories(context.Background(), "team-123", "user-1", 5)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got.Repositories)
	assert.Equal(t, 0, got.TotalCount)

	installationRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
	projectRepo.AssertNotCalled(t, "ListGitURLToSlugByTeam")
}

// goneInstallationErr mimics the error the GitHub client returns when the
// installation no longer exists on GitHub (token refresh 404).
func goneInstallationErr() error {
	return fmt.Errorf(
		"failed to list repositories: %w: %w",
		external.ErrGitHubInstallationGone,
		errors.New(`received non 2xx response status "404 Not Found"`),
	)
}

// TestGetRepositories_InstallationGoneRemovesRecord verifies that when GitHub
// reports the installation no longer exists, the stored record is deleted so
// the integration stops polling a dead installation, and the sentinel still
// surfaces to the handler for the 404 mapping.
func TestGetRepositories_InstallationGoneRemovesRecord(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	projectRepo := &MockProjectRepository{}
	githubClient := &MockGitHubAppClient{}

	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(sampleInstallationForRepos(), nil)
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 1).
		Return([]models.GitHubRepository{}, 0, goneInstallationErr())
	installationRepo.On("Delete", mock.Anything, "team-123").Return(nil)

	svc := buildGitHubAppServiceForReposTest(installationRepo, projectRepo, githubClient, nil)

	got, err := svc.GetRepositories(context.Background(), "team-123", "user-1", 1)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, external.ErrGitHubInstallationGone))

	installationRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
	projectRepo.AssertNotCalled(t, "ListGitURLToSlugByTeam")
}

// TestGetRepositories_InstallationGoneDeleteFailureStillSurfacesError verifies
// that a failed cleanup does not mask the original error or panic — the next
// poll retries the deletion.
func TestGetRepositories_InstallationGoneDeleteFailureStillSurfacesError(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	projectRepo := &MockProjectRepository{}
	githubClient := &MockGitHubAppClient{}

	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(sampleInstallationForRepos(), nil)
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 1).
		Return([]models.GitHubRepository{}, 0, goneInstallationErr())
	installationRepo.On("Delete", mock.Anything, "team-123").
		Return(errors.New("db unavailable"))

	svc := buildGitHubAppServiceForReposTest(installationRepo, projectRepo, githubClient, nil)

	got, err := svc.GetRepositories(context.Background(), "team-123", "user-1", 1)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, external.ErrGitHubInstallationGone))

	installationRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
}

// TestGetAccessibleRepoURLs_InstallationGoneReturnsEmpty verifies the gone
// installation degrades to "no accessible repositories" (matching the
// no-installation contract) while the dead record is removed.
func TestGetAccessibleRepoURLs_InstallationGoneReturnsEmpty(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	projectRepo := &MockProjectRepository{}
	githubClient := &MockGitHubAppClient{}

	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(sampleInstallationForRepos(), nil)
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 1).
		Return([]models.GitHubRepository{}, 0, goneInstallationErr())
	installationRepo.On("Delete", mock.Anything, "team-123").Return(nil)

	svc := buildGitHubAppServiceForReposTest(installationRepo, projectRepo, githubClient, nil)

	got, err := svc.GetAccessibleRepoURLs(context.Background(), "team-123")

	require.NoError(t, err)
	assert.Empty(t, got)

	installationRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
}

// TestGetRepositories_InstallationGoneAlreadyDeletedIsBenign verifies the
// concurrent-detection race: a second caller whose Delete affects no rows must
// not log an operator-facing error — the record is gone, which is the goal.
func TestGetRepositories_InstallationGoneAlreadyDeletedIsBenign(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	projectRepo := &MockProjectRepository{}
	githubClient := &MockGitHubAppClient{}

	logger, hook := logtest.New()

	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(sampleInstallationForRepos(), nil)
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 1).
		Return([]models.GitHubRepository{}, 0, goneInstallationErr())
	installationRepo.On("Delete", mock.Anything, "team-123").
		Return(repositories.ErrGitHubInstallationNotFound)

	svc := buildGitHubAppServiceForReposTest(installationRepo, projectRepo, githubClient, logger)

	_, err := svc.GetRepositories(context.Background(), "team-123", "user-1", 1)

	require.Error(t, err)
	assert.True(t, errors.Is(err, external.ErrGitHubInstallationGone))
	for _, e := range hook.AllEntries() {
		assert.NotEqual(t, slog.LevelError, e.Level,
			"already-deleted must not produce an error-level log: %s", e.Message)
	}

	installationRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
}
