package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// buildGitHubAppServiceForURLTest constructs a GitHubAppService backed by the provided mocks.
func buildGitHubAppServiceForURLTest(
	installationRepo *MockGitHubInstallationRepository,
	githubClient *MockGitHubAppClient,
) GitHubAppServiceInterface {
	logger, _ := logtest.New()
	return NewGitHubAppService(
		installationRepo,
		nil, // projectRepo not needed
		nil, // blueprintRepo not needed
		githubClient,
		nil, // encryptionSvc not needed
		nil, // eventManager not needed
		logger,
	)
}

// sampleInstallation builds a non-suspended installation for testing.
func sampleInstallation() *models.GitHubInstallation {
	return &models.GitHubInstallation{
		ID:             "inst-uuid-1",
		TeamID:         "team-123",
		InstallationID: 99999,
		AccountLogin:   "test-org",
		SuspendedAt:    nil,
	}
}

func TestGetAccessibleRepoURLs_NoInstallation(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	githubClient := &MockGitHubAppClient{}

	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(nil, repositories.ErrGitHubInstallationNotFound)

	svc := buildGitHubAppServiceForURLTest(installationRepo, githubClient)

	result, err := svc.GetAccessibleRepoURLs(context.Background(), "team-123")
	require.NoError(t, err)
	assert.Empty(t, result)

	installationRepo.AssertExpectations(t)
	githubClient.AssertNotCalled(t, "GetInstallationRepositories")
}

func TestGetAccessibleRepoURLs_SuspendedInstallation(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	githubClient := &MockGitHubAppClient{}

	suspended := sampleInstallation()
	now := time.Now()
	suspended.SuspendedAt = &now

	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(suspended, nil)

	svc := buildGitHubAppServiceForURLTest(installationRepo, githubClient)

	result, err := svc.GetAccessibleRepoURLs(context.Background(), "team-123")
	require.NoError(t, err)
	assert.Empty(t, result)

	installationRepo.AssertExpectations(t)
	// No GitHub API calls should be made for a suspended installation
	githubClient.AssertNotCalled(t, "GetInstallationRepositories")
}

func TestGetAccessibleRepoURLs_ActiveInstallation_SinglePage(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	githubClient := &MockGitHubAppClient{}

	installation := sampleInstallation()
	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(installation, nil)

	repos := []models.GitHubRepository{
		{ID: 1, Name: "repo-one", HTMLURL: "https://github.com/org/repo-one"},
		{ID: 2, Name: "repo-two", HTMLURL: "https://github.com/org/repo-two"},
	}
	// Page 1 returns repos; page 2 returns empty to signal end
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 1).
		Return(repos, 2, nil)
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 2).
		Return([]models.GitHubRepository{}, 0, nil)

	svc := buildGitHubAppServiceForURLTest(installationRepo, githubClient)

	result, err := svc.GetAccessibleRepoURLs(context.Background(), "team-123")
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.True(t, result["https://github.com/org/repo-one"])
	assert.True(t, result["https://github.com/org/repo-two"])

	installationRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
}

func TestGetAccessibleRepoURLs_MultiplePages(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	githubClient := &MockGitHubAppClient{}

	installation := sampleInstallation()
	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(installation, nil)

	page1 := []models.GitHubRepository{
		{ID: 1, Name: "repo-one", HTMLURL: "https://github.com/org/repo-one"},
	}
	page2 := []models.GitHubRepository{
		{ID: 2, Name: "repo-two", HTMLURL: "https://github.com/org/repo-two"},
	}
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 1).
		Return(page1, 2, nil)
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 2).
		Return(page2, 2, nil)
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 3).
		Return([]models.GitHubRepository{}, 0, nil)

	svc := buildGitHubAppServiceForURLTest(installationRepo, githubClient)

	result, err := svc.GetAccessibleRepoURLs(context.Background(), "team-123")
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.True(t, result["https://github.com/org/repo-one"])
	assert.True(t, result["https://github.com/org/repo-two"])

	installationRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
}

func TestGetAccessibleRepoURLs_URLNormalization(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	githubClient := &MockGitHubAppClient{}

	installation := sampleInstallation()
	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(installation, nil)

	repos := []models.GitHubRepository{
		// URL with trailing slash - should be normalized
		{ID: 1, Name: "repo-slash", HTMLURL: "https://github.com/org/repo-slash/"},
		// URL with .git suffix - should be normalized
		{ID: 2, Name: "repo-git", HTMLURL: "https://github.com/org/repo-git.git"},
		// URL with .git and trailing slash - both should be removed
		{ID: 3, Name: "repo-both", HTMLURL: "https://github.com/org/repo-both.git/"},
		// Uppercase URL - should be lowercased
		{ID: 4, Name: "repo-upper", HTMLURL: "https://GitHub.com/Org/Repo-Upper"},
	}
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 1).
		Return(repos, len(repos), nil)
	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 2).
		Return([]models.GitHubRepository{}, 0, nil)

	svc := buildGitHubAppServiceForURLTest(installationRepo, githubClient)

	result, err := svc.GetAccessibleRepoURLs(context.Background(), "team-123")
	require.NoError(t, err)

	// All should be found under their normalized forms
	assert.True(t, result["https://github.com/org/repo-slash"])
	assert.True(t, result["https://github.com/org/repo-git"])
	assert.True(t, result["https://github.com/org/repo-both"])
	assert.True(t, result["https://github.com/org/repo-upper"])

	installationRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
}

func TestGetAccessibleRepoURLs_GitHubAPIError(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	githubClient := &MockGitHubAppClient{}

	installation := sampleInstallation()
	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(installation, nil)

	githubClient.On("GetInstallationRepositories", mock.Anything, int64(99999), 1).
		Return([]models.GitHubRepository{}, 0, errors.New("github api timeout"))

	svc := buildGitHubAppServiceForURLTest(installationRepo, githubClient)

	result, err := svc.GetAccessibleRepoURLs(context.Background(), "team-123")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get installation repositories")

	installationRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
}

func TestGetAccessibleRepoURLs_InstallationRepoError(t *testing.T) {
	installationRepo := &MockGitHubInstallationRepository{}
	githubClient := &MockGitHubAppClient{}

	installationRepo.On("GetByTeamID", mock.Anything, "team-123").
		Return(nil, errors.New("database connection error"))

	svc := buildGitHubAppServiceForURLTest(installationRepo, githubClient)

	result, err := svc.GetAccessibleRepoURLs(context.Background(), "team-123")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get installation")

	installationRepo.AssertExpectations(t)
	githubClient.AssertNotCalled(t, "GetInstallationRepositories")
}

// TestNormalizeRepoURL tests URL normalization edge cases.
func TestNormalizeRepoURL(t *testing.T) {
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
		{"uppercase", "https://GitHub.com/Org/Repo", "https://github.com/org/repo"},
		{"uppercase with .git", "https://GitHub.com/Org/Repo.git", "https://github.com/org/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRepoURL(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
