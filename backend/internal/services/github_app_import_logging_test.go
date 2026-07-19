package services

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
)

// newTestLoggerWithBuffer creates an slog logger that writes to a buffer so we can
// assert log output in tests. The level is set to Debug so all levels are captured.
func newTestLoggerWithBuffer() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
	return logger, buf
}

// setupImportLoggingMocks sets up the standard mocks needed for blueprint import tests:
// installation, github client (repo lookup + all directories/files returning not-found
// by default), project lookup returning a project.
// It returns the project for further customisation.
func setupImportLoggingMocks(
	installationRepo *MockGitHubInstallationRepository,
	projectRepo *MockProjectRepository,
	githubClient *MockGitHubAppClient,
) *models.Project {
	installation := &models.GitHubInstallation{
		ID:             "install-log-1",
		TeamID:         "team-log",
		InstallationID: 99999,
	}
	installationRepo.On("GetByTeamID", mock.Anything, "team-log").Return(installation, nil)

	repo := &models.GitHubRepository{
		ID:       111,
		Name:     "log-repo",
		FullName: "owner/log-repo",
		HTMLURL:  "https://github.com/owner/log-repo",
		Owner:    models.GitHubRepositoryOwner{Login: "owner", Type: "User"},
	}
	githubClient.On("GetRepository", mock.Anything, int64(99999), int64(111)).Return(repo, nil)

	project := &models.Project{
		ID:     "project-log-1",
		TeamID: "team-log",
		GitURL: "https://github.com/owner/log-repo",
	}
	projectRepo.On("GetByGitURL", mock.Anything, "team-log", "user-log", "https://github.com/owner/log-repo").
		Return(project, nil)

	// Default: all directories and root files return not-found.
	for _, dir := range []string{".claude", ".cursor", ".codex", ".agents"} {
		githubClient.On("GetDirectoryContentsRecursive", mock.Anything, int64(99999), "owner", "log-repo", dir).
			Return(nil, errors.New("directory not found"))
	}
	for _, f := range []string{"CLAUDE.md", "CURSOR.md", "AGENTS.md"} {
		githubClient.On("GetFileContent", mock.Anything, int64(99999), "owner", "log-repo", f).
			Return(nil, errors.New("file not found"))
	}

	return project
}

// newImportLoggingService creates a GitHubAppService with the provided logger.
func newImportLoggingService(
	installationRepo *MockGitHubInstallationRepository,
	projectRepo *MockProjectRepository,
	blueprintRepo *MockBlueprintRepository,
	githubClient *MockGitHubAppClient,
	logger *slog.Logger,
) *GitHubAppService {
	eventManager := new(MockEventPublisher)
	encryptionSvc := new(MockEncryptionService)
	return NewGitHubAppService(
		installationRepo,
		projectRepo,
		blueprintRepo,
		githubClient,
		encryptionSvc,
		eventManager,
		logger,
	).(*GitHubAppService)
}

// TestImportSingleFile_SuccessLog verifies that a successfully imported blueprint
// emits an Info log with the required fields.
func TestImportSingleFile_SuccessLog(t *testing.T) {
	logger, buf := newTestLoggerWithBuffer()

	installationRepo := new(MockGitHubInstallationRepository)
	projectRepo := new(MockProjectRepository)
	blueprintRepo := new(MockBlueprintRepository)
	githubClient := new(MockGitHubAppClient)

	project := setupImportLoggingMocks(installationRepo, projectRepo, githubClient)

	// Override CLAUDE.md to return valid content
	githubClient.ExpectedCalls = removeMockCall(githubClient.ExpectedCalls, "GetFileContent", "CLAUDE.md")
	githubClient.On("GetFileContent", mock.Anything, int64(99999), "owner", "log-repo", "CLAUDE.md").
		Return(&external.GitHubFile{Path: "CLAUDE.md", Content: "# Agent config"}, nil)

	blueprintRepo.On("GetByProjectIDAndPath", mock.Anything, "user-log", "team-log", project.ID, mock.Anything).
		Return(nil, errors.New("not found"))
	blueprintRepo.On("GetByProjectIDAndSlug", mock.Anything, "user-log", "team-log", project.ID, mock.Anything).
		Return(nil, errors.New("not found"))
	blueprintRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Blueprint")).Return(nil)

	svc := newImportLoggingService(installationRepo, projectRepo, blueprintRepo, githubClient, logger)

	report, err := svc.ImportBlueprintsFromRepository(context.Background(), "user-log", "team-log", 111)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, 1, report.TotalSuccessful)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "Successfully imported blueprint from GitHub")
	assert.Contains(t, logOutput, "github-app")
	assert.Contains(t, logOutput, "CLAUDE.md")
	assert.Contains(t, logOutput, "blueprint_id")
	assert.Contains(t, logOutput, "team-log")
}

// TestImportSingleFile_SkipInvalidExtensionLog verifies that non-markdown files
// emit an Info log with reason=invalid_extension.
func TestImportSingleFile_SkipInvalidExtensionLog(t *testing.T) {
	logger, buf := newTestLoggerWithBuffer()

	installationRepo := new(MockGitHubInstallationRepository)
	projectRepo := new(MockProjectRepository)
	blueprintRepo := new(MockBlueprintRepository)
	githubClient := new(MockGitHubAppClient)

	setupImportLoggingMocks(installationRepo, projectRepo, githubClient)

	// Override .claude directory to return a Python file (non-markdown)
	githubClient.ExpectedCalls = removeMockCall(githubClient.ExpectedCalls, "GetDirectoryContentsRecursive", ".claude")
	githubClient.On("GetDirectoryContentsRecursive", mock.Anything, int64(99999), "owner", "log-repo", ".claude").
		Return([]*external.GitHubFile{
			{Path: ".claude/agents/script.py", Content: "print('hello')"},
		}, nil)

	svc := newImportLoggingService(installationRepo, projectRepo, blueprintRepo, githubClient, logger)

	report, err := svc.ImportBlueprintsFromRepository(context.Background(), "user-log", "team-log", 111)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, 1, report.TotalSkipped)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "Skipped file during blueprint import")
	assert.Contains(t, logOutput, "invalid_extension")
	assert.Contains(t, logOutput, "script.py")
}

// TestImportSingleFile_SkipEmptyContentLog verifies that empty markdown files
// emit a Debug log with reason=empty_content.
func TestImportSingleFile_SkipEmptyContentLog(t *testing.T) {
	logger, buf := newTestLoggerWithBuffer()

	installationRepo := new(MockGitHubInstallationRepository)
	projectRepo := new(MockProjectRepository)
	blueprintRepo := new(MockBlueprintRepository)
	githubClient := new(MockGitHubAppClient)

	setupImportLoggingMocks(installationRepo, projectRepo, githubClient)

	// Override CLAUDE.md to return empty content
	githubClient.ExpectedCalls = removeMockCall(githubClient.ExpectedCalls, "GetFileContent", "CLAUDE.md")
	githubClient.On("GetFileContent", mock.Anything, int64(99999), "owner", "log-repo", "CLAUDE.md").
		Return(&external.GitHubFile{Path: "CLAUDE.md", Content: ""}, nil)

	svc := newImportLoggingService(installationRepo, projectRepo, blueprintRepo, githubClient, logger)

	report, err := svc.ImportBlueprintsFromRepository(context.Background(), "user-log", "team-log", 111)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, 1, report.TotalSkipped)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "Skipped empty file during blueprint import")
	assert.Contains(t, logOutput, "empty_content")
	assert.Contains(t, logOutput, "CLAUDE.md")
}

// TestImportSingleFile_SkipTooLargeLog verifies that files exceeding 1MB emit an
// Info log with reason=file_too_large.
func TestImportSingleFile_SkipTooLargeLog(t *testing.T) {
	logger, buf := newTestLoggerWithBuffer()

	installationRepo := new(MockGitHubInstallationRepository)
	projectRepo := new(MockProjectRepository)
	blueprintRepo := new(MockBlueprintRepository)
	githubClient := new(MockGitHubAppClient)

	setupImportLoggingMocks(installationRepo, projectRepo, githubClient)

	// Override CLAUDE.md to return oversized content
	largeContent := strings.Repeat("x", 1024*1024+1) // 1MB + 1 byte
	githubClient.ExpectedCalls = removeMockCall(githubClient.ExpectedCalls, "GetFileContent", "CLAUDE.md")
	githubClient.On("GetFileContent", mock.Anything, int64(99999), "owner", "log-repo", "CLAUDE.md").
		Return(&external.GitHubFile{Path: "CLAUDE.md", Content: largeContent}, nil)

	svc := newImportLoggingService(installationRepo, projectRepo, blueprintRepo, githubClient, logger)

	report, err := svc.ImportBlueprintsFromRepository(context.Background(), "user-log", "team-log", 111)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, 1, report.TotalSkipped)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "Skipped file during blueprint import")
	assert.Contains(t, logOutput, "file_too_large")
	assert.Contains(t, logOutput, "CLAUDE.md")
}

// TestImportSingleFile_ReimportConflict verifies that a re-import of a file
// matching an existing blueprint whose provenance cannot confirm it is unedited
// is reported as a conflict (never overwritten), replacing the old blanket
// skip-on-slug behavior (#341).
func TestImportSingleFile_ReimportConflict(t *testing.T) {
	logger, _ := newTestLoggerWithBuffer()

	installationRepo := new(MockGitHubInstallationRepository)
	projectRepo := new(MockProjectRepository)
	blueprintRepo := new(MockBlueprintRepository)
	githubClient := new(MockGitHubAppClient)

	project := setupImportLoggingMocks(installationRepo, projectRepo, githubClient)

	// Override CLAUDE.md to return valid content
	githubClient.ExpectedCalls = removeMockCall(githubClient.ExpectedCalls, "GetFileContent", "CLAUDE.md")
	githubClient.On("GetFileContent", mock.Anything, int64(99999), "owner", "log-repo", "CLAUDE.md").
		Return(&external.GitHubFile{Path: "CLAUDE.md", Content: "# existing"}, nil)

	// Simulate an existing blueprint with the same path but no provenance we can
	// confirm as unedited -> re-import must treat it as a conflict.
	existingBlueprint := &models.Blueprint{
		ID:         "existing-bp-1",
		Slug:       "claude-from-log-repo",
		Path:       "CLAUDE.md",
		RawContent: "# edited in vibexp",
	}
	blueprintRepo.On("GetByProjectIDAndPath", mock.Anything, "user-log", "team-log", project.ID, "CLAUDE.md").
		Return(existingBlueprint, nil)

	svc := newImportLoggingService(installationRepo, projectRepo, blueprintRepo, githubClient, logger)

	report, err := svc.ImportBlueprintsFromRepository(context.Background(), "user-log", "team-log", 111)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, 1, report.TotalConflicts)
	assert.Equal(t, 0, report.TotalSuccessful)
	require.Len(t, report.ConflictItems, 1)
	assert.Equal(t, "existing-bp-1", report.ConflictItems[0].BlueprintID)
	assert.Equal(t, "CLAUDE.md", report.ConflictItems[0].FilePath)
}

// TestImportProgress_LogEvery10Files verifies that a progress log message is
// emitted every 10 files scanned.
func TestImportProgress_LogEvery10Files(t *testing.T) {
	logger, buf := newTestLoggerWithBuffer()

	installationRepo := new(MockGitHubInstallationRepository)
	projectRepo := new(MockProjectRepository)
	blueprintRepo := new(MockBlueprintRepository)
	githubClient := new(MockGitHubAppClient)

	project := setupImportLoggingMocks(installationRepo, projectRepo, githubClient)

	// Build 12 markdown files to trigger two progress logs (at 10 and not at 12
	// because 12 % 10 != 0, so only one log expected at file 10).
	files := make([]*external.GitHubFile, 12)
	for i := range files {
		files[i] = &external.GitHubFile{
			Path:    ".claude/agents/agent" + string(rune('A'+i)) + ".md",
			Content: "# Agent",
		}
	}

	// Override .claude directory
	githubClient.ExpectedCalls = removeMockCall(githubClient.ExpectedCalls, "GetDirectoryContentsRecursive", ".claude")
	githubClient.On("GetDirectoryContentsRecursive", mock.Anything, int64(99999), "owner", "log-repo", ".claude").
		Return(files, nil)

	blueprintRepo.On("GetByProjectIDAndPath", mock.Anything, "user-log", "team-log", project.ID, mock.Anything).
		Return(nil, errors.New("not found"))
	blueprintRepo.On("GetByProjectIDAndSlug", mock.Anything, "user-log", "team-log", project.ID, mock.Anything).
		Return(nil, errors.New("not found"))
	blueprintRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Blueprint")).Return(nil)

	svc := newImportLoggingService(installationRepo, projectRepo, blueprintRepo, githubClient, logger)

	report, err := svc.ImportBlueprintsFromRepository(context.Background(), "user-log", "team-log", 111)

	assert.NoError(t, err)
	assert.NotNil(t, report)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "GitHub blueprint import progress",
		"expected progress log when 10 files are scanned")
	assert.Contains(t, logOutput, "scanned=10")
}

// TestImportProgress_NoLogBelow10Files verifies that no progress log is emitted
// when fewer than 10 files are scanned.
func TestImportProgress_NoLogBelow10Files(t *testing.T) {
	logger, buf := newTestLoggerWithBuffer()

	installationRepo := new(MockGitHubInstallationRepository)
	projectRepo := new(MockProjectRepository)
	blueprintRepo := new(MockBlueprintRepository)
	githubClient := new(MockGitHubAppClient)

	project := setupImportLoggingMocks(installationRepo, projectRepo, githubClient)

	// Only 5 files - no progress log should appear
	files := make([]*external.GitHubFile, 5)
	for i := range files {
		files[i] = &external.GitHubFile{
			Path:    ".claude/agents/agent" + string(rune('A'+i)) + ".md",
			Content: "# Agent",
		}
	}

	githubClient.ExpectedCalls = removeMockCall(githubClient.ExpectedCalls, "GetDirectoryContentsRecursive", ".claude")
	githubClient.On("GetDirectoryContentsRecursive", mock.Anything, int64(99999), "owner", "log-repo", ".claude").
		Return(files, nil)

	blueprintRepo.On("GetByProjectIDAndPath", mock.Anything, "user-log", "team-log", project.ID, mock.Anything).
		Return(nil, errors.New("not found"))
	blueprintRepo.On("GetByProjectIDAndSlug", mock.Anything, "user-log", "team-log", project.ID, mock.Anything).
		Return(nil, errors.New("not found"))
	blueprintRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Blueprint")).Return(nil)

	svc := newImportLoggingService(installationRepo, projectRepo, blueprintRepo, githubClient, logger)

	report, err := svc.ImportBlueprintsFromRepository(context.Background(), "user-log", "team-log", 111)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, 5, report.TotalSuccessful)

	logOutput := buf.String()
	assert.NotContains(t, logOutput, "GitHub blueprint import progress",
		"should not log progress for fewer than 10 files")
}

// removeMockCall removes all expected calls from a mock that match the given method
// and one argument hint (matched by string containment). This is a test helper to
// allow overriding a broadly-configured mock expectation.
func removeMockCall(calls []*mock.Call, method string, argHint string) []*mock.Call {
	result := make([]*mock.Call, 0, len(calls))
	for _, c := range calls {
		if c.Method != method {
			result = append(result, c)
			continue
		}
		// Check if any argument matches the hint
		matched := false
		for _, arg := range c.Arguments {
			if s, ok := arg.(string); ok && strings.Contains(s, argHint) {
				matched = true
				break
			}
		}
		if !matched {
			result = append(result, c)
		}
	}
	return result
}
