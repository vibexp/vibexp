package services

import (
	"context"
	"errors"
	"testing"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
)

func newSanitizeTestService() (*GitHubAppService, *MockBlueprintRepository) {
	installationRepo := new(MockGitHubInstallationRepository)
	projectRepo := new(MockProjectRepository)
	blueprintRepo := new(MockBlueprintRepository)
	githubClient := new(MockGitHubAppClient)
	encryptionSvc := new(MockEncryptionService)
	eventManager := new(MockEventPublisher)

	logger := slog.New(slog.DiscardHandler)

	svc := NewGitHubAppService(
		installationRepo, projectRepo, blueprintRepo,
		githubClient, encryptionSvc, eventManager, logger,
	)
	return svc.(*GitHubAppService), blueprintRepo
}

func newSanitizeTestFixtures() (*models.GitHubRepository, *external.GitHubFile, *models.BlueprintImportReport) {
	repo := &models.GitHubRepository{
		ID: 100, Name: "my-repo", FullName: "org/my-repo",
		HTMLURL: "https://github.com/org/my-repo",
		Owner:   models.GitHubRepositoryOwner{Login: "org", Type: "Organization"},
	}
	file := &external.GitHubFile{
		Path:    ".claude/agents/agent.md",
		Content: "# My Agent\n\nThis is an agent description.",
	}
	report := &models.BlueprintImportReport{
		SuccessfulItems: []models.BlueprintImportSuccess{},
		FailedItems:     []models.BlueprintImportFailed{},
		SkippedItems:    []models.BlueprintImportSkipped{},
	}
	return repo, file, report
}

// TestImportSingleFile_ErrorSanitization verifies that when blueprintRepo.Create
// returns a raw DB error, the FailedItems entry contains only the generic sanitized
// message ("failed to import blueprint") and NOT the raw DB error string.
func TestImportSingleFile_ErrorSanitization(t *testing.T) {
	service, blueprintRepo := newSanitizeTestService()
	repo, file, report := newSanitizeTestFixtures()

	rawDBError := errors.New(`ERROR: value too long for type character varying(255) (SQLSTATE 22001)`)
	blueprintRepo.On("GetByProjectIDAndPath",
		mock.Anything, "user-1", "team-1", "project-1", mock.Anything,
	).Return(nil, errors.New("not found"))
	blueprintRepo.On("GetByProjectIDAndSlug",
		mock.Anything, "user-1", "team-1", "project-1", mock.Anything,
	).Return(nil, errors.New("not found"))
	blueprintRepo.On("Create", mock.Anything, mock.Anything).Return(rawDBError)

	job := &blueprintImportJob{
		userID: "user-1", teamID: "team-1", projectID: "project-1", repo: repo, report: report,
	}
	service.importSingleFile(context.Background(), job, file)

	assert.Equal(t, 1, report.TotalFailed, "expected exactly one failure")
	assert.Equal(t, 0, report.TotalSuccessful)
	require.Len(t, report.FailedItems, 1)

	item := report.FailedItems[0]
	assert.Equal(t, ".claude/agents/agent.md", item.FilePath)
	assert.Equal(t, "failed to import blueprint", item.Error,
		"FailedItems[].Error must be sanitized, not the raw DB error")
	assert.NotContains(t, item.Error, "SQLSTATE", "DB-internal details must not leak")
	assert.NotContains(t, item.Error, "character varying", "DB schema must not leak")
	blueprintRepo.AssertExpectations(t)
}

// TestImportSingleFile_SuccessNotSanitized verifies that on success the report
// records the correct file path (sanitization does not affect the happy path).
func TestImportSingleFile_SuccessNotSanitized(t *testing.T) {
	service, blueprintRepo := newSanitizeTestService()
	repo, file, report := newSanitizeTestFixtures()

	blueprintRepo.On("GetByProjectIDAndPath",
		mock.Anything, "user-1", "team-1", "project-1", mock.Anything,
	).Return(nil, errors.New("not found"))
	blueprintRepo.On("GetByProjectIDAndSlug",
		mock.Anything, "user-1", "team-1", "project-1", mock.Anything,
	).Return(nil, errors.New("not found"))
	blueprintRepo.On("Create", mock.Anything, mock.Anything).Return(nil)

	job := &blueprintImportJob{
		userID: "user-1", teamID: "team-1", projectID: "project-1", repo: repo, report: report,
	}
	service.importSingleFile(context.Background(), job, file)

	assert.Equal(t, 1, report.TotalSuccessful)
	assert.Equal(t, 0, report.TotalFailed)
	require.Len(t, report.SuccessfulItems, 1)
	assert.Equal(t, ".claude/agents/agent.md", report.SuccessfulItems[0].FilePath)
	blueprintRepo.AssertExpectations(t)
}
