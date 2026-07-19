package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/utils"
	"github.com/vibexp/vibexp/pkg/events"
)

// Mock implementations for testing

type MockGitHubInstallationRepository struct {
	mock.Mock
}

func (m *MockGitHubInstallationRepository) Create(ctx context.Context, installation *models.GitHubInstallation) error {
	return mockErrCall(&m.Mock, "Create", ctx, installation)
}

func (m *MockGitHubInstallationRepository) GetByTeamID(
	ctx context.Context,
	teamID string,
) (*models.GitHubInstallation, error) {
	args := m.Called(ctx, teamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.GitHubInstallation), args.Error(1)
}

func (m *MockGitHubInstallationRepository) GetByInstallationID(
	ctx context.Context,
	installationID int64,
) (*models.GitHubInstallation, error) {
	args := m.Called(ctx, installationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.GitHubInstallation), args.Error(1)
}

func (m *MockGitHubInstallationRepository) Update(ctx context.Context, installation *models.GitHubInstallation) error {
	return mockErrCall(&m.Mock, "Update", ctx, installation)
}

func (m *MockGitHubInstallationRepository) Delete(ctx context.Context, teamID string) error {
	args := m.Called(ctx, teamID)
	return args.Error(0)
}

type MockProjectRepository struct {
	mock.Mock
}

func (m *MockProjectRepository) Create(ctx context.Context, project *models.Project) error {
	return mockErrCall(&m.Mock, "Create", ctx, project)
}

func (m *MockProjectRepository) GetBySlug(ctx context.Context, teamID, userID, slug string) (*models.Project, error) {
	args := m.Called(ctx, teamID, userID, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Project), args.Error(1)
}

func (m *MockProjectRepository) GetByID(ctx context.Context, userID, projectID string) (*models.Project, error) {
	args := m.Called(ctx, userID, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Project), args.Error(1)
}

func (m *MockProjectRepository) GetByGitURL(
	ctx context.Context, teamID, userID, gitURL string,
) (*models.Project, error) {
	args := m.Called(ctx, teamID, userID, gitURL)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Project), args.Error(1)
}

func (m *MockProjectRepository) List(
	ctx context.Context,
	userID string,
	filters repositories.ProjectListFilters,
) ([]models.Project, int, error) {
	args := m.Called(ctx, userID, filters)
	return args.Get(0).([]models.Project), args.Int(1), args.Error(2)
}

func (m *MockProjectRepository) Update(ctx context.Context, project *models.Project) error {
	return mockErrCall(&m.Mock, "Update", ctx, project)
}

func (m *MockProjectRepository) Delete(ctx context.Context, teamID, userID, slug string) error {
	args := m.Called(ctx, teamID, userID, slug)
	return args.Error(0)
}

func (m *MockProjectRepository) CountByTeamID(ctx context.Context, teamID string) (int, error) {
	args := m.Called(ctx, teamID)
	return args.Int(0), args.Error(1)
}

func (m *MockProjectRepository) GetNamesByIDs(_ context.Context, _ string, _ []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (m *MockProjectRepository) GetProjectStats(
	ctx context.Context, teamID, userID, projectSlug string,
) (*models.ProjectStatsResponse, error) {
	args := m.Called(ctx, teamID, userID, projectSlug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ProjectStatsResponse), args.Error(1)
}

func (m *MockProjectRepository) GetProjectResourceCreationMetrics(
	ctx context.Context, teamID, userID, projectSlug string, since time.Time,
) ([]models.ProjectResourceCreationCount, error) {
	args := m.Called(ctx, teamID, userID, projectSlug, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.ProjectResourceCreationCount), args.Error(1)
}

func (m *MockProjectRepository) ListGitURLToSlugByTeam(
	ctx context.Context, teamID, userID string,
) (map[string]string, error) {
	args := m.Called(ctx, teamID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]string), args.Error(1)
}

type MockGitHubAppClient struct {
	mock.Mock
}

func (m *MockGitHubAppClient) GetInstallationToken(
	ctx context.Context,
	installationID int64,
) (string, time.Time, error) {
	args := m.Called(ctx, installationID)
	return args.String(0), args.Get(1).(time.Time), args.Error(2)
}

func (m *MockGitHubAppClient) GetInstallationRepositories(
	ctx context.Context,
	installationID int64,
	page int,
) ([]models.GitHubRepository, int, error) {
	args := m.Called(ctx, installationID, page)
	return args.Get(0).([]models.GitHubRepository), args.Int(1), args.Error(2)
}

func (m *MockGitHubAppClient) GetInstallation(
	ctx context.Context,
	installationID int64,
) (*external.GitHubInstallationInfo, error) {
	args := m.Called(ctx, installationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*external.GitHubInstallationInfo), args.Error(1)
}

func (m *MockGitHubAppClient) GetRepository(
	ctx context.Context,
	installationID int64,
	repoID int64,
) (*models.GitHubRepository, error) {
	args := m.Called(ctx, installationID, repoID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.GitHubRepository), args.Error(1)
}

func (m *MockGitHubAppClient) EvictCachedClient(_ int64) {}

type MockEventPublisher struct {
	mock.Mock
}

func (m *MockEventPublisher) Publish(ctx context.Context, event events.Event) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

type setupMocksFunc func(
	*MockGitHubInstallationRepository,
	*MockProjectRepository,
	*MockGitHubAppClient,
	*MockEventPublisher,
)

type testCase struct {
	name            string
	userID          string
	teamID          string
	repoID          int64
	setupMocks      setupMocksFunc
	wantProject     *models.Project
	wantCreated     bool
	wantErr         bool
	wantErrContains string
}

func mockSuccessfulImportSetup(
	installationRepo *MockGitHubInstallationRepository,
	projectRepo *MockProjectRepository,
	githubClient *MockGitHubAppClient,
	eventManager *MockEventPublisher,
) {
	installation := &models.GitHubInstallation{
		ID:             "install-1",
		TeamID:         "team-456",
		InstallationID: 12345,
	}
	installationRepo.On("GetByTeamID", mock.Anything, "team-456").Return(installation, nil)

	repo := &models.GitHubRepository{
		ID:          789,
		Name:        "test-repo",
		FullName:    "owner/test-repo",
		Description: stringPtr("A test repository"),
		HTMLURL:     "https://github.com/owner/test-repo",
		Owner: models.GitHubRepositoryOwner{
			Login: "owner",
			Type:  "User",
		},
	}
	githubClient.On("GetRepository", mock.Anything, int64(12345), int64(789)).Return(repo, nil)

	projectRepo.On("GetByGitURL", mock.Anything, "team-456", "user-123", "https://github.com/owner/test-repo").
		Return(nil, errors.New("project not found"))

	projectRepo.On("Create", mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
		return p.UserID == "user-123" &&
			p.TeamID == "team-456" &&
			p.Name == "owner/test-repo" &&
			p.Slug == "owner-test-repo" &&
			p.Description == "A test repository" &&
			p.GitURL == "https://github.com/owner/test-repo"
	})).Run(func(args mock.Arguments) {
		p := args.Get(1).(*models.Project)
		p.ID = "project-new-123"
		p.CreatedAt = time.Now()
		p.UpdatedAt = time.Now()
	}).Return(nil)

	eventManager.On("Publish", mock.Anything, mock.Anything).Return(nil)
}

func testCaseSuccessfulImport() testCase {
	return testCase{
		name:       "successfully creates new project from repository",
		userID:     "user-123",
		teamID:     "team-456",
		repoID:     789,
		setupMocks: mockSuccessfulImportSetup,
		wantProject: &models.Project{
			ID:          "project-new-123",
			UserID:      "user-123",
			TeamID:      "team-456",
			Name:        "owner/test-repo",
			Slug:        "owner-test-repo",
			Description: "A test repository",
			GitURL:      "https://github.com/owner/test-repo",
		},
		wantCreated: true,
		wantErr:     false,
	}
}

func testCaseExistingProject() testCase {
	return testCase{
		name:   "returns existing project when git_url already exists",
		userID: "user-123",
		teamID: "team-456",
		repoID: 789,
		setupMocks: func(
			installationRepo *MockGitHubInstallationRepository,
			projectRepo *MockProjectRepository,
			githubClient *MockGitHubAppClient,
			eventManager *MockEventPublisher,
		) {
			// Installation exists
			installation := &models.GitHubInstallation{
				ID:             "install-1",
				TeamID:         "team-456",
				InstallationID: 12345,
			}
			installationRepo.On("GetByTeamID", mock.Anything, "team-456").Return(installation, nil)

			// GitHub API returns repository
			repo := &models.GitHubRepository{
				ID:          789,
				Name:        "test-repo",
				FullName:    "owner/test-repo",
				Description: stringPtr("A test repository"),
				HTMLURL:     "https://github.com/owner/test-repo",
				Owner: models.GitHubRepositoryOwner{
					Login: "owner",
					Type:  "User",
				},
			}
			githubClient.On("GetRepository", mock.Anything, int64(12345), int64(789)).Return(repo, nil)

			// Existing project with this git_url
			existingProject := &models.Project{
				ID:          "project-existing-456",
				UserID:      "user-123",
				TeamID:      "team-456",
				Name:        "owner/test-repo",
				Slug:        "owner-test-repo",
				Description: "A test repository",
				GitURL:      "https://github.com/owner/test-repo",
				CreatedAt:   time.Now().Add(-24 * time.Hour),
				UpdatedAt:   time.Now().Add(-24 * time.Hour),
			}
			projectRepo.On("GetByGitURL", mock.Anything, "team-456", "user-123", "https://github.com/owner/test-repo").
				Return(existingProject, nil)
		},
		wantProject: &models.Project{
			ID:          "project-existing-456",
			UserID:      "user-123",
			TeamID:      "team-456",
			Name:        "owner/test-repo",
			Slug:        "owner-test-repo",
			Description: "A test repository",
			GitURL:      "https://github.com/owner/test-repo",
		},
		wantCreated: false,
		wantErr:     false,
	}
}

func testCaseInstallationNotFound() testCase {
	return testCase{
		name:   "returns error when GitHub installation not found",
		userID: "user-123",
		teamID: "team-456",
		repoID: 789,
		setupMocks: func(
			installationRepo *MockGitHubInstallationRepository,
			projectRepo *MockProjectRepository,
			githubClient *MockGitHubAppClient,
			eventManager *MockEventPublisher,
		) {
			// No installation
			installationRepo.On("GetByTeamID", mock.Anything, "team-456").
				Return(nil, repositories.ErrGitHubInstallationNotFound)
		},
		wantErr:         true,
		wantErrContains: "GitHub installation not found",
	}
}

func testCaseRepositoryNotFound() testCase {
	return testCase{
		name:   "returns error when repository not found in GitHub",
		userID: "user-123",
		teamID: "team-456",
		repoID: 789,
		setupMocks: func(
			installationRepo *MockGitHubInstallationRepository,
			projectRepo *MockProjectRepository,
			githubClient *MockGitHubAppClient,
			eventManager *MockEventPublisher,
		) {
			// Installation exists
			installation := &models.GitHubInstallation{
				ID:             "install-1",
				TeamID:         "team-456",
				InstallationID: 12345,
			}
			installationRepo.On("GetByTeamID", mock.Anything, "team-456").Return(installation, nil)

			// GitHub API returns 404
			githubClient.On("GetRepository", mock.Anything, int64(12345), int64(789)).
				Return(nil, errors.New("failed to get repository: 404 Not Found"))
		},
		wantErr:         true,
		wantErrContains: "failed to get repository",
	}
}

func testCaseNilDescription() testCase {
	return testCase{
		name:   "handles repository with nil description",
		userID: "user-123",
		teamID: "team-456",
		repoID: 789,
		setupMocks: func(
			installationRepo *MockGitHubInstallationRepository,
			projectRepo *MockProjectRepository,
			githubClient *MockGitHubAppClient,
			eventManager *MockEventPublisher,
		) {
			// Installation exists
			installation := &models.GitHubInstallation{
				ID:             "install-1",
				TeamID:         "team-456",
				InstallationID: 12345,
			}
			installationRepo.On("GetByTeamID", mock.Anything, "team-456").Return(installation, nil)

			// GitHub API returns repository with nil description
			repo := &models.GitHubRepository{
				ID:          789,
				Name:        "test-repo",
				FullName:    "owner/test-repo",
				Description: nil, // nil description
				HTMLURL:     "https://github.com/owner/test-repo",
				Owner: models.GitHubRepositoryOwner{
					Login: "owner",
					Type:  "User",
				},
			}
			githubClient.On("GetRepository", mock.Anything, int64(12345), int64(789)).Return(repo, nil)

			// No existing project
			projectRepo.On("GetByGitURL", mock.Anything, "team-456", "user-123", "https://github.com/owner/test-repo").
				Return(nil, errors.New("project not found"))

			// Project creation succeeds
			projectRepo.On("Create", mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
				return p.Description == "" // Should be empty string, not nil
			})).Run(func(args mock.Arguments) {
				p := args.Get(1).(*models.Project)
				p.ID = "project-new-123"
				p.CreatedAt = time.Now()
				p.UpdatedAt = time.Now()
			}).Return(nil)

			// Event published
			eventManager.On("Publish", mock.Anything, mock.Anything).Return(nil)
		},
		wantProject: &models.Project{
			ID:          "project-new-123",
			UserID:      "user-123",
			TeamID:      "team-456",
			Name:        "owner/test-repo",
			Slug:        "owner-test-repo",
			Description: "",
			GitURL:      "https://github.com/owner/test-repo",
		},
		wantCreated: true,
		wantErr:     false,
	}
}

func getImportProjectTestCases() []testCase {
	return []testCase{
		testCaseSuccessfulImport(),
		testCaseExistingProject(),
		testCaseInstallationNotFound(),
		testCaseRepositoryNotFound(),
		testCaseNilDescription(),
	}
}

func setupTestService(
	installationRepo *MockGitHubInstallationRepository,
	projectRepo *MockProjectRepository,
	githubClient *MockGitHubAppClient,
	eventManager *MockEventPublisher,
) GitHubAppServiceInterface {
	blueprintRepo := new(MockBlueprintRepository)
	encryptionSvc := new(MockEncryptionService)
	logger := slog.New(slog.DiscardHandler)

	return NewGitHubAppService(
		installationRepo,
		projectRepo,
		blueprintRepo,
		githubClient,
		encryptionSvc,
		eventManager,
		logger,
	)
}

func assertImportProjectResult(
	t *testing.T,
	tt testCase,
	project *models.Project,
	created bool,
	err error,
) {
	if tt.wantErr {
		assert.Error(t, err)
		if tt.wantErrContains != "" {
			assert.Contains(t, err.Error(), tt.wantErrContains)
		}
		assert.Nil(t, project)
		assert.False(t, created)
		return
	}

	assert.NoError(t, err)
	assert.NotNil(t, project)
	assert.Equal(t, tt.wantCreated, created)

	if tt.wantProject != nil {
		assert.Equal(t, tt.wantProject.ID, project.ID)
		assert.Equal(t, tt.wantProject.UserID, project.UserID)
		assert.Equal(t, tt.wantProject.TeamID, project.TeamID)
		assert.Equal(t, tt.wantProject.Name, project.Name)
		assert.Equal(t, tt.wantProject.Slug, project.Slug)
		assert.Equal(t, tt.wantProject.Description, project.Description)
		assert.Equal(t, tt.wantProject.GitURL, project.GitURL)
	}
}

func TestGitHubAppService_ImportProjectFromRepository(t *testing.T) {
	tests := getImportProjectTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installationRepo := new(MockGitHubInstallationRepository)
			projectRepo := new(MockProjectRepository)
			githubClient := new(MockGitHubAppClient)
			eventManager := new(MockEventPublisher)

			tt.setupMocks(installationRepo, projectRepo, githubClient, eventManager)

			service := setupTestService(installationRepo, projectRepo, githubClient, eventManager)

			project, created, err := service.ImportProjectFromRepository(
				context.Background(),
				tt.userID,
				tt.teamID,
				tt.repoID,
			)

			assertImportProjectResult(t, tt, project, created, err)

			installationRepo.AssertExpectations(t)
			projectRepo.AssertExpectations(t)
			githubClient.AssertExpectations(t)
		})
	}
}

type slugTestCase struct {
	name     string
	input    string
	expected string
}

func generateSlugFromNameTestCases() []slugTestCase {
	return []slugTestCase{
		{name: "simple name", input: "test-repo", expected: "test-repo"},
		{name: "name with spaces", input: "my test repo", expected: "my-test-repo"},
		{name: "name with special characters", input: "test@repo#123", expected: "testrepo123"},
		{name: "name with uppercase", input: "TestRepo", expected: "testrepo"},
		{name: "name with consecutive hyphens", input: "test--repo", expected: "test-repo"},
		{name: "name with leading/trailing hyphens", input: "-test-repo-", expected: "test-repo"},
		{name: "empty name", input: "", expected: "project"},
		{name: "name with only special characters", input: "@#$%", expected: "project"},
		{name: "owner-qualified repo name", input: "owner/repo", expected: "owner-repo"},
		{name: "org-qualified dotted repo name", input: "shaharia-lab/vibexp.io", expected: "shaharia-lab-vibexp-io"},
		{name: "dotted name", input: "my.cool.repo", expected: "my-cool-repo"},
	}
}

func TestGenerateSlugFromName(t *testing.T) {
	for _, tt := range generateSlugFromNameTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			result := generateSlugFromName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGenerateSlugFromName_Truncation verifies that a full name long enough to
// exceed the projects.slug VARCHAR(100) column is truncated to fit, never ends
// on a hyphen, and never collapses to empty.
func TestGenerateSlugFromName_Truncation(t *testing.T) {
	// owner/<200 'a'> sanitizes to "owner-aaa...", well over 100 chars.
	longName := "owner/" + strings.Repeat("a", 200)

	slug := generateSlugFromName(longName)

	assert.LessOrEqual(t, len(slug), maxSlugLength, "slug must fit the VARCHAR(100) column")
	assert.NotEmpty(t, slug)
	assert.False(t, strings.HasSuffix(slug, "-"), "truncated slug must not end on a hyphen")
}

func TestBuildSuffixedSlug(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		attempt  int
		expected string
	}{
		{
			name:     "short base keeps full slug",
			base:     "owner-repo",
			attempt:  2,
			expected: "owner-repo-2",
		},
		{
			name:     "base at column limit is truncated to fit suffix",
			base:     strings.Repeat("a", maxSlugLength),
			attempt:  2,
			expected: strings.Repeat("a", maxSlugLength-2) + "-2",
		},
		{
			// base[:maxSlugLength-2] cuts right after the hyphen, which must be trimmed.
			name:     "truncation that lands on a hyphen is trimmed",
			base:     strings.Repeat("a", maxSlugLength-3) + "-bbb",
			attempt:  2,
			expected: strings.Repeat("a", maxSlugLength-3) + "-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildSuffixedSlug(tt.base, tt.attempt)
			assert.Equal(t, tt.expected, result)
			assert.LessOrEqual(t, len(result), maxSlugLength)
		})
	}
}

// slugExistsErr builds the exact error shape ProjectRepository.Create returns on a
// Postgres 23505 unique violation, so the retry path is exercised against the real error.
func slugExistsErr(slug string) error {
	return fmt.Errorf("project with slug '%s' already exists: %w", slug, repositories.ErrProjectSlugExists)
}

// gitURLExistsErr builds the exact error shape ProjectRepository.Create returns on a
// Postgres 23505 unique violation against the git_url constraint, so the race-condition
// backstop path is exercised against the real error.
func gitURLExistsErr(gitURL string) error {
	return fmt.Errorf("project with git_url '%s' already exists: %w", gitURL, repositories.ErrProjectGitURLExists)
}

// asGitHubAppService returns the concrete *GitHubAppService so tests can drive its
// unexported retry helpers directly.
func asGitHubAppService(t *testing.T, svc GitHubAppServiceInterface) *GitHubAppService {
	t.Helper()
	concrete, ok := svc.(*GitHubAppService)
	require.True(t, ok, "expected *GitHubAppService")
	return concrete
}

func TestGitHubAppService_HandleSlugConstraintViolation_RetriesPastFirstCollision(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	projectRepo := new(MockProjectRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	// Attempts 2 and 3 collide; attempt 4 succeeds. The loop must continue past the
	// first retry instead of breaking, proving collisions are detected via errors.Is.
	projectRepo.On("Create", mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
		return p.Slug == "owner-repo-2"
	})).Return(slugExistsErr("owner-repo-2")).Once()
	projectRepo.On("Create", mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
		return p.Slug == "owner-repo-3"
	})).Return(slugExistsErr("owner-repo-3")).Once()
	projectRepo.On("Create", mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
		return p.Slug == "owner-repo-4"
	})).Return(nil).Once()

	service := asGitHubAppService(t, setupTestService(installationRepo, projectRepo, githubClient, eventManager))

	project := &models.Project{
		UserID: "user-123",
		TeamID: "team-456",
		Name:   "owner/repo",
		Slug:   "owner-repo",
		GitURL: "https://github.com/owner/repo",
	}

	err := service.handleSlugConstraintViolation(context.Background(), project, "team-456", "user-123", 789)

	assert.NoError(t, err)
	assert.Equal(t, "owner-repo-4", project.Slug)
	projectRepo.AssertExpectations(t)
}

func TestGitHubAppService_HandleSlugConstraintViolation_FailsFastOnNonCollisionError(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	projectRepo := new(MockProjectRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	// A non-collision error (e.g. connection failure) must break the loop immediately;
	// Create is invoked exactly once for the retry phase.
	projectRepo.On("Create", mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
		return p.Slug == "owner-repo-2"
	})).Return(errors.New("connection refused")).Once()

	service := asGitHubAppService(t, setupTestService(installationRepo, projectRepo, githubClient, eventManager))

	project := &models.Project{
		UserID: "user-123",
		TeamID: "team-456",
		Name:   "owner/repo",
		Slug:   "owner-repo",
		GitURL: "https://github.com/owner/repo",
	}

	err := service.handleSlugConstraintViolation(context.Background(), project, "team-456", "user-123", 789)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
	projectRepo.AssertExpectations(t)
	projectRepo.AssertNumberOfCalls(t, "Create", 1)
}

// mockInstallationAndRepo wires the installation lookup and GetRepository call shared by the
// end-to-end import tests below.
func mockInstallationAndRepo(
	installationRepo *MockGitHubInstallationRepository,
	githubClient *MockGitHubAppClient,
) {
	installation := &models.GitHubInstallation{
		ID:             "install-1",
		TeamID:         "team-456",
		InstallationID: 12345,
	}
	installationRepo.On("GetByTeamID", mock.Anything, "team-456").Return(installation, nil)

	repo := &models.GitHubRepository{
		ID:          789,
		Name:        "test-repo",
		FullName:    "owner/test-repo",
		Description: stringPtr("A test repository"),
		HTMLURL:     "https://github.com/owner/test-repo",
		Owner: models.GitHubRepositoryOwner{
			Login: "owner",
			Type:  "User",
		},
	}
	githubClient.On("GetRepository", mock.Anything, int64(12345), int64(789)).Return(repo, nil)
}

// TestGitHubAppService_ImportProjectFromRepository_SlugCollisionRoutesToRetry is the
// regression test for issue #1387: a real slug collision returned by Create (the production
// error shape, a wrapped sentinel without a *pq.Error) must be routed through the dispatcher
// into the slug-retry loop, not hard-failed. Without the sentinel-based dispatcher this test
// fails because handleConstraintViolation reports "not handled" and the import errors out.
func TestGitHubAppService_ImportProjectFromRepository_SlugCollisionRoutesToRetry(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	projectRepo := new(MockProjectRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	mockInstallationAndRepo(installationRepo, githubClient)

	// git_url pre-check: no existing project.
	projectRepo.On("GetByGitURL", mock.Anything, "team-456", "user-123", "https://github.com/owner/test-repo").
		Return(nil, errors.New("project not found"))

	// Base slug collides, then the first suffix collides, then the second suffix succeeds.
	projectRepo.On("Create", mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
		return p.Slug == "owner-test-repo"
	})).Return(slugExistsErr("owner-test-repo")).Once()
	projectRepo.On("Create", mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
		return p.Slug == "owner-test-repo-2"
	})).Return(slugExistsErr("owner-test-repo-2")).Once()
	projectRepo.On("Create", mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
		return p.Slug == "owner-test-repo-3"
	})).Run(func(args mock.Arguments) {
		p := args.Get(1).(*models.Project)
		p.ID = "project-new-123"
		p.CreatedAt = time.Now()
		p.UpdatedAt = time.Now()
	}).Return(nil).Once()

	eventManager.On("Publish", mock.Anything, mock.Anything).Return(nil)

	service := setupTestService(installationRepo, projectRepo, githubClient, eventManager)

	project, created, err := service.ImportProjectFromRepository(context.Background(), "user-123", "team-456", 789)

	require.NoError(t, err)
	require.NotNil(t, project)
	assert.True(t, created, "a project created after slug-retry must report wasCreated=true")
	assert.Equal(t, "owner-test-repo-3", project.Slug, "import must persist the collision-resolved slug")

	installationRepo.AssertExpectations(t)
	projectRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
	eventManager.AssertExpectations(t)
}

// TestGitHubAppService_ImportProjectFromRepository_GitURLRaceBackstop verifies the
// race-condition backstop: when the git_url pre-check misses but Create then loses the race
// (returns ErrProjectGitURLExists), the dispatcher's git_url branch re-fetches the existing
// project and returns it with wasCreated=false and no error.
func TestGitHubAppService_ImportProjectFromRepository_GitURLRaceBackstop(t *testing.T) {
	installationRepo := new(MockGitHubInstallationRepository)
	projectRepo := new(MockProjectRepository)
	githubClient := new(MockGitHubAppClient)
	eventManager := new(MockEventPublisher)

	mockInstallationAndRepo(installationRepo, githubClient)

	existingProject := &models.Project{
		ID:          "project-existing-456",
		UserID:      "user-123",
		TeamID:      "team-456",
		Name:        "owner/test-repo",
		Slug:        "owner-test-repo",
		Description: "A test repository",
		GitURL:      "https://github.com/owner/test-repo",
		CreatedAt:   time.Now().Add(-24 * time.Hour),
		UpdatedAt:   time.Now().Add(-24 * time.Hour),
	}

	// First GetByGitURL (pre-check) misses; the backstop GetByGitURL finds the existing project.
	projectRepo.On("GetByGitURL", mock.Anything, "team-456", "user-123", "https://github.com/owner/test-repo").
		Return(nil, errors.New("project not found")).Once()
	projectRepo.On("GetByGitURL", mock.Anything, "team-456", "user-123", "https://github.com/owner/test-repo").
		Return(existingProject, nil).Once()

	projectRepo.On("Create", mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
		return p.Slug == "owner-test-repo"
	})).Return(gitURLExistsErr("https://github.com/owner/test-repo")).Once()

	service := setupTestService(installationRepo, projectRepo, githubClient, eventManager)

	project, created, err := service.ImportProjectFromRepository(context.Background(), "user-123", "team-456", 789)

	require.NoError(t, err)
	require.NotNil(t, project)
	assert.False(t, created, "a git_url race that resolves to an existing project must report wasCreated=false")
	assert.Equal(t, "project-existing-456", project.ID, "import must return the existing project on a git_url race")

	installationRepo.AssertExpectations(t)
	projectRepo.AssertExpectations(t)
	githubClient.AssertExpectations(t)
	eventManager.AssertExpectations(t)
}

// MockEncryptionService is a mock for testing
type MockEncryptionService struct{}

func (m *MockEncryptionService) Encrypt(plaintext string) (string, error) {
	return "encrypted-" + plaintext, nil
}

func (m *MockEncryptionService) Decrypt(ciphertext string) (string, error) {
	return "decrypted-" + ciphertext, nil
}

func (m *MockGitHubAppClient) GetFileContent(
	ctx context.Context,
	installationID int64,
	owner, repoName, path string,
) (*external.GitHubFile, error) {
	args := m.Called(ctx, installationID, owner, repoName, path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*external.GitHubFile), args.Error(1)
}

func (m *MockGitHubAppClient) GetDirectoryContentsRecursive(
	ctx context.Context,
	installationID int64,
	owner, repoName, dirPath string,
) ([]*external.GitHubFile, error) {
	args := m.Called(ctx, installationID, owner, repoName, dirPath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*external.GitHubFile), args.Error(1)
}

// Test determineTypeFromPath
func TestGitHubAppService_DetermineTypeFromPath(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		expectedType    string
		expectedSubtype string
	}{
		// .claude paths
		{"claude agents", ".claude/agents/agent.md", "claude-code", "sub-agents"},
		{"claude skills", ".claude/skills/skill.md", "claude-code", "skills"},
		{"claude commands", ".claude/commands/cmd.md", "claude-code", "slash-commands"},
		{"claude others", ".claude/other/file.md", "claude-code", "others"},
		{"CLAUDE.md", "CLAUDE.md", "claude", "claude-md"},

		// .cursor paths
		{"cursor skills", ".cursor/skills/skill.md", "cursor", "skills"},
		{"cursor agents", ".cursor/agents/agent.md", "cursor", "agents"},
		{"cursor commands", ".cursor/commands/cmd.md", "cursor", "commands"},
		{"cursor rules", ".cursor/rules/rule.md", "cursor", "rules"},
		{"cursor others", ".cursor/other/file.md", "cursor", "cursor-md"},
		{"CURSOR.md", "CURSOR.md", "cursor", "cursor-md"},

		// .codex paths
		{"codex rules", ".codex/rules/rule.md", "codex", "rules"},
		{"codex skills", ".codex/skills/skill.md", "codex", "skills"},
		{"codex others", ".codex/other/file.md", "codex", "others"},
		{"AGENTS.md", "AGENTS.md", "codex", "agents-md"},

		// .agents paths
		{"agents skills", ".agents/skills/skill.md", "codex", "skills"},
		{"agents others", ".agents/other/file.md", "codex", "others"},

		// Unmapped paths
		{"unmapped", ".unknown/file.md", "general", ""},
		{"unmapped root", "README.md", "general", ""},
	}

	service := &GitHubAppService{
		logger: slog.New(slog.DiscardHandler),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotSubtype := service.determineTypeFromPath(tt.path)
			assert.Equal(t, tt.expectedType, gotType)
			assert.Equal(t, tt.expectedSubtype, gotSubtype)
		})
	}
}

// TestGitHubAppService_BuildImportedBlueprint_NestedFrontMatter verifies that an
// imported file with nested/typed frontmatter keeps its structure end to end:
// name/description are lifted to dedicated fields, and the remaining metadata
// (nested map, list, typed scalars) is stored verbatim rather than flattened.
func TestGitHubAppService_BuildImportedBlueprint_NestedFrontMatter(t *testing.T) {
	service := &GitHubAppService{
		logger: slog.New(slog.DiscardHandler),
	}
	repo := &models.GitHubRepository{
		Name:     "myrepo",
		FullName: "owner/myrepo",
	}
	content := "---\n" +
		"name: Deploy Skill\n" +
		"description: Ships the app\n" +
		"allowed-tools:\n  - Bash\n  - Read\n" +
		"config:\n  retries: 3\n  verbose: true\n" +
		"---\n" +
		"Skill body here"
	file := &external.GitHubFile{
		Path:    ".claude/skills/deploy/SKILL.md",
		Content: content,
	}

	bp := service.buildImportedBlueprint("user-1", "team-1", "proj-1", repo, file, "claude-code", "skills")

	assert.Equal(t, "Deploy Skill", bp.Title, "name should be lifted to Title")
	assert.Equal(t, "Ships the app", bp.Description, "description should be lifted")
	assert.Equal(t, "Skill body here", bp.Content, "body should exclude frontmatter")

	// name/description are lifted out; the rest keeps nested structure + types.
	assert.NotContains(t, bp.Metadata, "name")
	assert.NotContains(t, bp.Metadata, "description")
	assert.Equal(t, []any{"Bash", "Read"}, bp.Metadata["allowed-tools"])
	assert.Equal(t, map[string]any{"retries": 3, "verbose": true}, bp.Metadata["config"])
}

// TestBlueprintMetadataFromFrontMatter_PreservesNested verifies the metadata copy
// helper keeps non-string values and drops the lifted keys.
func TestBlueprintMetadataFromFrontMatter_PreservesNested(t *testing.T) {
	fm := utils.FrontMatterResult{
		HasFrontMatter: true,
		Metadata: map[string]any{
			"name":        "X",
			"title":       "Y",
			"description": "Z",
			"version":     2,
			"enabled":     true,
			"tags":        []any{"a", "b"},
			"nested":      map[string]any{"k": "v"},
		},
	}
	got := blueprintMetadataFromFrontMatter(fm)
	assert.Equal(t, map[string]any{
		"version": 2,
		"enabled": true,
		"tags":    []any{"a", "b"},
		"nested":  map[string]any{"k": "v"},
	}, got)
}

// TestFrontMatterString covers the typed-value lifting guard: only string
// scalars are lifted; a non-string value for name/title/description is ignored.
func TestFrontMatterString(t *testing.T) {
	fm := utils.FrontMatterResult{
		Metadata: map[string]any{"name": "Agent", "count": 5, "flag": true},
	}
	assert.Equal(t, "Agent", frontMatterString(fm, "name"))
	assert.Equal(t, "", frontMatterString(fm, "count"), "non-string value must not be lifted")
	assert.Equal(t, "", frontMatterString(fm, "missing"))
}

// Test generateBlueprintSlug
func TestGitHubAppService_GenerateBlueprintSlug(t *testing.T) {
	tests := []struct {
		name         string
		filePath     string
		repoName     string
		expectedSlug string
	}{
		{
			name:         "simple filename",
			filePath:     "CLAUDE.md",
			repoName:     "vibexp",
			expectedSlug: "claude-from-vibexp",
		},
		{
			name:         "path with directory",
			filePath:     ".claude/agents/agent.md",
			repoName:     "my-repo",
			expectedSlug: "claude-agents-agent-from-my-repo",
		},
		{
			name:         "special characters",
			filePath:     "AGENTS.md",
			repoName:     "test_repo",
			expectedSlug: "agents-from-testrepo",
		},
		{
			name:         "uppercase and spaces",
			filePath:     "TEST FILE.md",
			repoName:     "Test Repo",
			expectedSlug: "test-file-from-test-repo",
		},
	}

	service := &GitHubAppService{
		logger: slog.New(slog.DiscardHandler),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSlug := service.generateBlueprintSlug(tt.filePath, tt.repoName)
			assert.Equal(t, tt.expectedSlug, gotSlug)
		})
	}
}

// Test ImportBlueprintsFromRepository
//
//nolint:funlen // Table-driven test requires comprehensive test cases
func TestGitHubAppService_ImportBlueprintsFromRepository(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		teamID     string
		repoID     int64
		setupMocks func(
			*MockGitHubInstallationRepository,
			*MockProjectRepository,
			*MockBlueprintRepository,
			*MockGitHubAppClient,
			*MockEventPublisher,
		)
		expectedTotalScanned int
		expectedTotalSuccess int
		expectedTotalFailed  int
		expectedTotalSkipped int
		wantErr              bool
		wantErrContains      string
	}{
		{
			name:   "successful import with markdown files",
			userID: "user-123",
			teamID: "team-456",
			repoID: 789,
			setupMocks: func(
				installationRepo *MockGitHubInstallationRepository,
				projectRepo *MockProjectRepository,
				blueprintRepo *MockBlueprintRepository,
				githubClient *MockGitHubAppClient,
				eventManager *MockEventPublisher,
			) {
				installation := &models.GitHubInstallation{
					ID:             "install-1",
					TeamID:         "team-456",
					InstallationID: 12345,
				}
				installationRepo.On("GetByTeamID", mock.Anything, "team-456").Return(installation, nil)

				repo := &models.GitHubRepository{
					ID:       789,
					Name:     "test-repo",
					FullName: "owner/test-repo",
					HTMLURL:  "https://github.com/owner/test-repo",
					Owner: models.GitHubRepositoryOwner{
						Login: "owner",
						Type:  "User",
					},
				}
				githubClient.On("GetRepository", mock.Anything, int64(12345), int64(789)).Return(repo, nil)

				// Mock project lookup by repository URL
				project := &models.Project{
					ID:     "project-123",
					TeamID: "team-456",
					GitURL: "https://github.com/owner/test-repo",
				}
				projectRepo.On(
					"GetByGitURL",
					mock.Anything,
					"team-456",
					"user-123",
					"https://github.com/owner/test-repo",
				).
					Return(
						project,
						nil,
					)

				// Simulate directory scanning (using owner and repoName instead of repoID)
				githubClient.On(
					"GetDirectoryContentsRecursive",
					mock.Anything,
					int64(12345),
					"owner",
					"test-repo",
					".claude",
				).
					Return(
						[]*external.GitHubFile{
							{Path: ".claude/agents/agent.md", Content: "# Agent"},
							{Path: ".claude/agents/script.py", Content: "# Python"},
						},
						nil,
					)

				githubClient.On(
					"GetDirectoryContentsRecursive",
					mock.Anything,
					int64(12345),
					"owner",
					"test-repo",
					".cursor",
				).
					Return(
						nil,
						errors.New("directory not found"),
					)

				githubClient.On(
					"GetDirectoryContentsRecursive",
					mock.Anything,
					int64(12345),
					"owner",
					"test-repo",
					".codex",
				).
					Return(
						nil,
						errors.New("directory not found"),
					)

				githubClient.On(
					"GetDirectoryContentsRecursive",
					mock.Anything,
					int64(12345),
					"owner",
					"test-repo",
					".agents",
				).
					Return(
						nil,
						errors.New("directory not found"),
					)

				// Root files (using owner and repoName instead of repoID)
				githubClient.On("GetFileContent", mock.Anything, int64(12345), "owner", "test-repo", "CLAUDE.md").
					Return(&external.GitHubFile{Path: "CLAUDE.md", Content: "# Claude config"}, nil)

				githubClient.On("GetFileContent", mock.Anything, int64(12345), "owner", "test-repo", "CURSOR.md").
					Return(nil, errors.New("file not found"))

				githubClient.On("GetFileContent", mock.Anything, int64(12345), "owner", "test-repo", "AGENTS.md").
					Return(nil, errors.New("file not found"))

				// Mock GetByProjectIDAndSlug to return nil (no existing blueprint)
				blueprintRepo.On(
					"GetByProjectIDAndSlug",
					mock.Anything,
					"user-123",
					"team-456",
					"project-123",
					mock.Anything,
				).
					Return(
						nil,
						errors.New("not found"),
					)

				// Expect blueprint creation for markdown files only
				blueprintRepo.On("Create", mock.Anything, mock.MatchedBy(func(b *models.Blueprint) bool {
					return b.ProjectID == "project-123" && b.Type == "claude-code" && *b.Subtype == "sub-agents"
				})).Return(nil)

				blueprintRepo.On("Create", mock.Anything, mock.MatchedBy(func(b *models.Blueprint) bool {
					return b.ProjectID == "project-123" && b.Type == "claude" && *b.Subtype == "claude-md"
				})).Return(nil)
			},
			expectedTotalScanned: 3, // 2 from .claude, 1 CLAUDE.md
			expectedTotalSuccess: 2, // 1 .md from .claude, 1 CLAUDE.md
			expectedTotalFailed:  0,
			expectedTotalSkipped: 1, // 1 .py file
			wantErr:              false,
		},
		{
			name:   "installation not found",
			userID: "user-123",
			teamID: "team-456",
			repoID: 789,
			setupMocks: func(
				installationRepo *MockGitHubInstallationRepository,
				projectRepo *MockProjectRepository,
				blueprintRepo *MockBlueprintRepository,
				githubClient *MockGitHubAppClient,
				eventManager *MockEventPublisher,
			) {
				installationRepo.On("GetByTeamID", mock.Anything, "team-456").
					Return(nil, repositories.ErrGitHubInstallationNotFound)
			},
			wantErr:         true,
			wantErrContains: "GitHub installation not found",
		},
		{
			name:   "empty file skipped",
			userID: "user-123",
			teamID: "team-456",
			repoID: 789,
			setupMocks: func(
				installationRepo *MockGitHubInstallationRepository,
				projectRepo *MockProjectRepository,
				blueprintRepo *MockBlueprintRepository,
				githubClient *MockGitHubAppClient,
				eventManager *MockEventPublisher,
			) {
				installation := &models.GitHubInstallation{
					ID:             "install-1",
					TeamID:         "team-456",
					InstallationID: 12345,
				}
				installationRepo.On("GetByTeamID", mock.Anything, "team-456").Return(installation, nil)

				repo := &models.GitHubRepository{
					ID:       789,
					Name:     "test-repo",
					FullName: "owner/test-repo",
					HTMLURL:  "https://github.com/owner/test-repo",
					Owner: models.GitHubRepositoryOwner{
						Login: "owner",
						Type:  "User",
					},
				}
				githubClient.On("GetRepository", mock.Anything, int64(12345), int64(789)).Return(repo, nil)

				// Mock project lookup by repository URL
				project := &models.Project{
					ID:     "project-123",
					TeamID: "team-456",
					GitURL: "https://github.com/owner/test-repo",
				}
				projectRepo.On(
					"GetByGitURL",
					mock.Anything,
					"team-456",
					"user-123",
					"https://github.com/owner/test-repo",
				).
					Return(
						project,
						nil,
					)

				// Empty file
				githubClient.On("GetFileContent", mock.Anything, int64(12345), "owner", "test-repo", "CLAUDE.md").
					Return(&external.GitHubFile{Path: "CLAUDE.md", Content: ""}, nil)

				// No other directories/files - using specific parameters
				githubClient.On(
					"GetDirectoryContentsRecursive",
					mock.Anything,
					int64(12345),
					"owner",
					"test-repo",
					".claude",
				).
					Return(
						nil,
						errors.New("not found"),
					)
				githubClient.On(
					"GetDirectoryContentsRecursive",
					mock.Anything,
					int64(12345),
					"owner",
					"test-repo",
					".cursor",
				).
					Return(
						nil,
						errors.New("not found"),
					)
				githubClient.On(
					"GetDirectoryContentsRecursive",
					mock.Anything,
					int64(12345),
					"owner",
					"test-repo",
					".codex",
				).
					Return(
						nil,
						errors.New("not found"),
					)
				githubClient.On(
					"GetDirectoryContentsRecursive",
					mock.Anything,
					int64(12345),
					"owner",
					"test-repo",
					".agents",
				).
					Return(
						nil,
						errors.New("not found"),
					)
				githubClient.On("GetFileContent", mock.Anything, int64(12345), "owner", "test-repo", "CURSOR.md").
					Return(nil, errors.New("not found"))
				githubClient.On("GetFileContent", mock.Anything, int64(12345), "owner", "test-repo", "AGENTS.md").
					Return(nil, errors.New("not found"))
			},
			expectedTotalScanned: 1,
			expectedTotalSuccess: 0,
			expectedTotalFailed:  0,
			expectedTotalSkipped: 1, // Empty file
			wantErr:              false,
		},
		{
			name:   "project not found for repository",
			userID: "user-123",
			teamID: "team-456",
			repoID: 789,
			setupMocks: func(
				installationRepo *MockGitHubInstallationRepository,
				projectRepo *MockProjectRepository,
				blueprintRepo *MockBlueprintRepository,
				githubClient *MockGitHubAppClient,
				eventManager *MockEventPublisher,
			) {
				installation := &models.GitHubInstallation{
					ID:             "install-1",
					TeamID:         "team-456",
					InstallationID: 12345,
				}
				installationRepo.On("GetByTeamID", mock.Anything, "team-456").Return(installation, nil)

				repo := &models.GitHubRepository{
					ID:       789,
					Name:     "test-repo",
					FullName: "owner/test-repo",
					HTMLURL:  "https://github.com/owner/test-repo",
				}
				githubClient.On("GetRepository", mock.Anything, int64(12345), int64(789)).Return(repo, nil)

				// Project not found for this repository
				projectRepo.On(
					"GetByGitURL",
					mock.Anything,
					"team-456",
					"user-123",
					"https://github.com/owner/test-repo",
				).
					Return(
						nil,
						errors.New("project not found"),
					)
			},
			wantErr:         true,
			wantErrContains: "project not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installationRepo := new(MockGitHubInstallationRepository)
			projectRepo := new(MockProjectRepository)
			blueprintRepo := new(MockBlueprintRepository)
			githubClient := new(MockGitHubAppClient)
			eventManager := new(MockEventPublisher)

			tt.setupMocks(installationRepo, projectRepo, blueprintRepo, githubClient, eventManager)

			encryptionSvc := new(MockEncryptionService)
			logger := slog.New(slog.DiscardHandler)

			service := NewGitHubAppService(
				installationRepo,
				projectRepo,
				blueprintRepo,
				githubClient,
				encryptionSvc,
				eventManager,
				logger,
			)

			report, err := service.ImportBlueprintsFromRepository(
				context.Background(),
				tt.userID,
				tt.teamID,
				tt.repoID,
			)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrContains != "" {
					assert.Contains(t, err.Error(), tt.wantErrContains)
				}
				assert.Nil(t, report)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, report)
				assert.Equal(t, tt.expectedTotalScanned, report.TotalScanned)
				assert.Equal(t, tt.expectedTotalSuccess, report.TotalSuccessful)
				assert.Equal(t, tt.expectedTotalFailed, report.TotalFailed)
				assert.Equal(t, tt.expectedTotalSkipped, report.TotalSkipped)
			}

			installationRepo.AssertExpectations(t)
			blueprintRepo.AssertExpectations(t)
			githubClient.AssertExpectations(t)
		})
	}
}

// setupCommonImportMocks sets up installation, repository, project, and directory mocks
// used by frontmatter-specific test cases.
func setupCommonImportMocks(
	installationRepo *MockGitHubInstallationRepository,
	projectRepo *MockProjectRepository,
	githubClient *MockGitHubAppClient,
) (*models.GitHubRepository, *models.Project) {
	installation := &models.GitHubInstallation{
		ID:             "install-1",
		TeamID:         "team-456",
		InstallationID: 12345,
	}
	installationRepo.On("GetByTeamID", mock.Anything, "team-456").Return(installation, nil)

	repo := &models.GitHubRepository{
		ID:       789,
		Name:     "test-repo",
		FullName: "owner/test-repo",
		HTMLURL:  "https://github.com/owner/test-repo",
		Owner: models.GitHubRepositoryOwner{
			Login: "owner",
			Type:  "User",
		},
	}
	githubClient.On("GetRepository", mock.Anything, int64(12345), int64(789)).Return(repo, nil)

	project := &models.Project{
		ID:     "project-fm-123",
		TeamID: "team-456",
		GitURL: "https://github.com/owner/test-repo",
	}
	projectRepo.On("GetByGitURL", mock.Anything, "team-456", "user-123", "https://github.com/owner/test-repo").
		Return(project, nil)

	// All directories not found
	for _, dir := range []string{".claude", ".cursor", ".codex", ".agents"} {
		githubClient.On("GetDirectoryContentsRecursive", mock.Anything, int64(12345), "owner", "test-repo", dir).
			Return(nil, errors.New("directory not found"))
	}

	// Other root files not found
	for _, f := range []string{"CURSOR.md", "AGENTS.md"} {
		githubClient.On("GetFileContent", mock.Anything, int64(12345), "owner", "test-repo", f).
			Return(nil, errors.New("file not found"))
	}

	return repo, project
}

// TestGitHubAppService_ImportSingleFile_FrontMatter verifies that importSingleFile
// correctly parses YAML frontmatter from markdown files and populates blueprint
// title, description, metadata, and content accordingly.
//
//nolint:funlen // Table-driven test requires comprehensive test cases
func TestGitHubAppService_ImportSingleFile_FrontMatter(t *testing.T) {
	tests := []struct {
		name                string
		fileContent         string
		wantTitle           string
		wantDescription     string
		wantContent         string
		wantMetadataKeys    []string // keys expected in Metadata
		wantNotMetadataKeys []string // keys that must NOT be in Metadata
	}{
		{
			name:                "file with frontmatter sets title description and metadata and strips content",
			fileContent:         "---\nname: My Agent\ndescription: Does X\nmodel: sonnet\n---\nBody here",
			wantTitle:           "My Agent",
			wantDescription:     "Does X",
			wantContent:         "Body here",
			wantMetadataKeys:    []string{"model"},
			wantNotMetadataKeys: []string{"name", "description"},
		},
		{
			name:                "file without frontmatter uses default title and description and full content",
			fileContent:         "# Just a plain agent\n\nDoes things",
			wantTitle:           "CLAUDE.md from test-repo",
			wantDescription:     "Imported from owner/test-repo",
			wantContent:         "# Just a plain agent\n\nDoes things",
			wantMetadataKeys:    []string{},
			wantNotMetadataKeys: []string{"name", "title", "description"},
		},
		{
			name:                "frontmatter with title key (not name) sets title",
			fileContent:         "---\ntitle: Title Agent\ndescription: Desc here\n---\nContent",
			wantTitle:           "Title Agent",
			wantDescription:     "Desc here",
			wantContent:         "Content",
			wantMetadataKeys:    []string{},
			wantNotMetadataKeys: []string{"name", "title", "description"},
		},
		{
			name:                "frontmatter with name takes priority over title",
			fileContent:         "---\nname: Name Value\ntitle: Title Value\ndescription: Desc\n---\nContent",
			wantTitle:           "Name Value",
			wantDescription:     "Desc",
			wantContent:         "Content",
			wantMetadataKeys:    []string{},
			wantNotMetadataKeys: []string{"name", "title", "description"},
		},
		{
			name:                "malformed frontmatter falls back to defaults",
			fileContent:         "---\nname: [unclosed\n---\nContent",
			wantTitle:           "CLAUDE.md from test-repo",
			wantDescription:     "Imported from owner/test-repo",
			wantContent:         "---\nname: [unclosed\n---\nContent",
			wantMetadataKeys:    []string{},
			wantNotMetadataKeys: []string{"name", "title", "description"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installationRepo := new(MockGitHubInstallationRepository)
			projectRepo := new(MockProjectRepository)
			blueprintRepo := new(MockBlueprintRepository)
			githubClient := new(MockGitHubAppClient)
			eventManager := new(MockEventPublisher)

			_, project := setupCommonImportMocks(installationRepo, projectRepo, githubClient)

			// CLAUDE.md returns the file content under test
			githubClient.On("GetFileContent", mock.Anything, int64(12345), "owner", "test-repo", "CLAUDE.md").
				Return(&external.GitHubFile{Path: "CLAUDE.md", Content: tt.fileContent}, nil)

			// No existing blueprint
			blueprintRepo.On("GetByProjectIDAndSlug", mock.Anything, "user-123", "team-456", project.ID, mock.Anything).
				Return(nil, errors.New("not found"))

			// Capture blueprint on Create for assertions
			var createdBlueprint *models.Blueprint
			blueprintRepo.On("Create", mock.Anything, mock.MatchedBy(func(b *models.Blueprint) bool {
				createdBlueprint = b
				return true
			})).Return(nil)

			encryptionSvc := new(MockEncryptionService)
			logger := slog.New(slog.DiscardHandler)

			service := NewGitHubAppService(
				installationRepo,
				projectRepo,
				blueprintRepo,
				githubClient,
				encryptionSvc,
				eventManager,
				logger,
			)

			report, err := service.ImportBlueprintsFromRepository(
				context.Background(),
				"user-123",
				"team-456",
				789,
			)

			if !assert.NoError(t, err) {
				return
			}
			assert.NotNil(t, report)
			assert.Equal(t, 1, report.TotalSuccessful)
			assert.NotNil(t, createdBlueprint, "blueprint should have been created")

			assert.Equal(t, tt.wantTitle, createdBlueprint.Title, "Title mismatch")
			assert.Equal(t, tt.wantDescription, createdBlueprint.Description, "Description mismatch")
			assert.Equal(t, tt.wantContent, createdBlueprint.Content, "Content mismatch")

			for _, key := range tt.wantMetadataKeys {
				assert.Contains(t, createdBlueprint.Metadata, key, "Expected metadata key %q not found", key)
			}
			for _, key := range tt.wantNotMetadataKeys {
				assert.NotContains(t, createdBlueprint.Metadata, key, "Unexpected metadata key %q found", key)
			}

			installationRepo.AssertExpectations(t)
			blueprintRepo.AssertExpectations(t)
			githubClient.AssertExpectations(t)
		})
	}
}
