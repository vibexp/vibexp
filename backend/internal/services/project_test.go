package services

import (
	"fmt"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

func createTestProjectService(repo repositories.ProjectRepository) *ProjectService {
	return NewProjectService(
		repo, nil, allowAllAuthz{}, nil,
		func() *slog.Logger { l, _ := logtest.New(); return l }(),
	)
}

func createTestProject() *models.Project {
	now := time.Now()
	return &models.Project{
		ID:          "project-123",
		UserID:      "user-123",
		TeamID:      "team-123",
		Name:        "Test Project",
		Slug:        "test-project",
		Description: "This is a test project",
		GitURL:      "https://github.com/test/project",
		Homepage:    "https://testproject.com",
		CreatedAt:   now,
		UpdatedAt:   now,
		Version:     1,
	}
}

func createTestCreateProjectRequest() *models.CreateProjectRequest {
	return &models.CreateProjectRequest{
		Name:        "New Project",
		Slug:        "new-project",
		Description: "A new test project",
		GitURL:      "https://github.com/test/new-project",
		Homepage:    "https://newproject.com",
	}
}

func createTestUpdateProjectRequest() *models.UpdateProjectRequest {
	name := "Updated Project"
	description := "Updated description"
	return &models.UpdateProjectRequest{
		Name:        &name,
		Description: &description,
	}
}

//nolint:funlen,gocyclo // Test function requires comprehensive setup and assertions
func TestProjectService_CreateProject(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		teamID     string
		request    *models.CreateProjectRequest
		setupMock  func(*mocks.MockProjectRepository)
		expectErr  bool
		errMessage string
	}{
		{
			name:    "successful creation",
			userID:  "user-123",
			teamID:  "team-123",
			request: createTestCreateProjectRequest(),
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				mockRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(project *models.Project) bool {
					return project.UserID == "user-123" &&
						project.TeamID == "team-123" &&
						project.Name == "New Project" &&
						project.Slug == "new-project" &&
						project.Description == "A new test project" &&
						project.GitURL == "https://github.com/test/new-project" &&
						project.Homepage == "https://newproject.com" &&
						!project.CreatedAt.IsZero() &&
						!project.UpdatedAt.IsZero()
				})).Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:   "creation with minimal fields",
			userID: "user-123",
			teamID: "team-123",
			request: &models.CreateProjectRequest{
				Name: "Minimal Project",
				Slug: "minimal-project",
			},
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				mockRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(project *models.Project) bool {
					return project.UserID == "user-123" &&
						project.TeamID == "team-123" &&
						project.Name == "Minimal Project" &&
						project.Slug == "minimal-project" &&
						project.Description == "" &&
						project.GitURL == "" &&
						project.Homepage == ""
				})).Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:    "repository error",
			userID:  "user-123",
			teamID:  "team-123",
			request: createTestCreateProjectRequest(),
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				mockRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(fmt.Errorf("database error")).Once()
			},
			expectErr:  true,
			errMessage: "database error",
		},
		{
			name:    "duplicate slug error",
			userID:  "user-123",
			teamID:  "team-123",
			request: createTestCreateProjectRequest(),
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				mockRepo.EXPECT().Create(mock.Anything, mock.Anything).
					Return(fmt.Errorf("project with slug 'new-project' already exists")).Once()
			},
			expectErr:  true,
			errMessage: "already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockProjectRepository(t)
			tt.setupMock(mockRepo)

			service := createTestProjectService(mockRepo)
			result, err := service.CreateProject(tt.userID, tt.teamID, tt.request)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.userID, result.UserID)
				assert.Equal(t, tt.teamID, result.TeamID)
				assert.Equal(t, tt.request.Name, result.Name)
				assert.Equal(t, tt.request.Slug, result.Slug)
				assert.NotZero(t, result.CreatedAt)
				assert.NotZero(t, result.UpdatedAt)
			}
		})
	}
}

func TestProjectService_GetProjectBySlug(t *testing.T) {
	tests := []struct {
		name       string
		teamID     string
		userID     string
		slug       string
		setupMock  func(*mocks.MockProjectRepository)
		expected   *models.Project
		expectErr  bool
		errMessage string
	}{
		{
			name:   "successful get",
			teamID: "team-123",
			userID: "user-123",
			slug:   "test-project",
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				project := createTestProject()
				mockRepo.EXPECT().
					GetBySlug(mock.Anything, "team-123", "user-123", "test-project").
					Return(project, nil).
					Once()
			},
			expected:  createTestProject(),
			expectErr: false,
		},
		{
			name:   "project not found",
			teamID: "team-123",
			userID: "user-123",
			slug:   "nonexistent",
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				mockRepo.EXPECT().
					GetBySlug(mock.Anything, "team-123", "user-123", "nonexistent").
					Return(nil, fmt.Errorf("project not found")).Once()
			},
			expected:   nil,
			expectErr:  true,
			errMessage: "project not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockProjectRepository(t)
			tt.setupMock(mockRepo)

			service := createTestProjectService(mockRepo)
			result, err := service.GetProjectBySlug(tt.teamID, tt.userID, tt.slug)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expected.ID, result.ID)
				assert.Equal(t, tt.expected.Name, result.Name)
				assert.Equal(t, tt.expected.Slug, result.Slug)
			}
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestProjectService_ListProjects(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		filters    ProjectFilters
		setupMock  func(*mocks.MockProjectRepository)
		expectErr  bool
		expectLen  int
		errMessage string
	}{
		{
			name:   "successful list",
			userID: "user-123",
			filters: ProjectFilters{
				TeamID: "team-123",
				Page:   1,
				Limit:  10,
			},
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				projects := []models.Project{*createTestProject()}
				mockRepo.EXPECT().
					List(mock.Anything, "user-123", mock.MatchedBy(func(f repositories.ProjectListFilters) bool {
						return f.TeamID == "team-123" && f.Page == 1 && f.Limit == 10
					})).
					Return(projects, 1, nil).
					Once()
			},
			expectErr: false,
			expectLen: 1,
		},
		{
			name:   "list with search filter",
			userID: "user-123",
			filters: ProjectFilters{
				Search: "test",
				TeamID: "team-123",
				Page:   1,
				Limit:  10,
			},
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				projects := []models.Project{*createTestProject()}
				mockRepo.EXPECT().
					List(mock.Anything, "user-123", mock.MatchedBy(func(f repositories.ProjectListFilters) bool {
						return f.Search == "test" && f.TeamID == "team-123" && f.Page == 1 && f.Limit == 10
					})).
					Return(projects, 1, nil).
					Once()
			},
			expectErr: false,
			expectLen: 1,
		},
		{
			name:   "empty list",
			userID: "user-123",
			filters: ProjectFilters{
				TeamID: "team-123",
				Page:   1,
				Limit:  10,
			},
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				mockRepo.EXPECT().
					List(mock.Anything, "user-123", mock.Anything).
					Return([]models.Project{}, 0, nil).
					Once()
			},
			expectErr: false,
			expectLen: 0,
		},
		{
			name:   "repository error",
			userID: "user-123",
			filters: ProjectFilters{
				TeamID: "team-123",
				Page:   1,
				Limit:  10,
			},
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				mockRepo.EXPECT().List(mock.Anything, "user-123", mock.Anything).
					Return(nil, 0, fmt.Errorf("database error")).Once()
			},
			expectErr:  true,
			errMessage: "database error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockProjectRepository(t)
			tt.setupMock(mockRepo)

			service := createTestProjectService(mockRepo)
			result, err := service.ListProjects(tt.userID, tt.filters)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Len(t, result.Projects, tt.expectLen)
			}
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestProjectService_UpdateProject(t *testing.T) {
	tests := []struct {
		name       string
		teamID     string
		userID     string
		slug       string
		request    *models.UpdateProjectRequest
		setupMock  func(*mocks.MockProjectRepository)
		expectErr  bool
		errMessage string
	}{
		{
			name:    "successful update",
			teamID:  "team-123",
			userID:  "user-123",
			slug:    "test-project",
			request: createTestUpdateProjectRequest(),
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				project := createTestProject()
				mockRepo.EXPECT().
					GetBySlug(mock.Anything, "team-123", "user-123", "test-project").
					Return(project, nil).
					Once()
				mockRepo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
					return p.Name == "Updated Project" &&
						p.Description == "Updated description" &&
						p.Slug == "test-project"
				})).Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:   "update slug only",
			teamID: "team-123",
			userID: "user-123",
			slug:   "test-project",
			request: func() *models.UpdateProjectRequest {
				slug := "new-slug"
				return &models.UpdateProjectRequest{Slug: &slug}
			}(),
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				project := createTestProject()
				mockRepo.EXPECT().
					GetBySlug(mock.Anything, "team-123", "user-123", "test-project").
					Return(project, nil).
					Once()
				mockRepo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(p *models.Project) bool {
					return p.Slug == "new-slug"
				})).Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:    "project not found",
			teamID:  "team-123",
			userID:  "user-123",
			slug:    "nonexistent",
			request: createTestUpdateProjectRequest(),
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				mockRepo.EXPECT().GetBySlug(mock.Anything, "team-123", "user-123", "nonexistent").
					Return(nil, fmt.Errorf("project not found")).Once()
			},
			expectErr:  true,
			errMessage: "project not found",
		},
		{
			name:    "repository update error",
			teamID:  "team-123",
			userID:  "user-123",
			slug:    "test-project",
			request: createTestUpdateProjectRequest(),
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				project := createTestProject()
				mockRepo.EXPECT().
					GetBySlug(mock.Anything, "team-123", "user-123", "test-project").
					Return(project, nil).
					Once()
				mockRepo.EXPECT().Update(mock.Anything, mock.Anything).
					Return(fmt.Errorf("database error")).Once()
			},
			expectErr:  true,
			errMessage: "database error",
		},
		{
			name:   "slug conflict error",
			teamID: "team-123",
			userID: "user-123",
			slug:   "test-project",
			request: func() *models.UpdateProjectRequest {
				slug := "existing-slug"
				return &models.UpdateProjectRequest{Slug: &slug}
			}(),
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				project := createTestProject()
				mockRepo.EXPECT().
					GetBySlug(mock.Anything, "team-123", "user-123", "test-project").
					Return(project, nil).
					Once()
				mockRepo.EXPECT().Update(mock.Anything, mock.Anything).
					Return(fmt.Errorf("project with slug 'existing-slug' already exists")).Once()
			},
			expectErr:  true,
			errMessage: "already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockProjectRepository(t)
			tt.setupMock(mockRepo)

			service := createTestProjectService(mockRepo)
			result, err := service.UpdateProject(tt.teamID, tt.userID, tt.slug, tt.request)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestProjectService_DeleteProject(t *testing.T) {
	tests := []struct {
		name       string
		teamID     string
		userID     string
		slug       string
		setupMock  func(*mocks.MockProjectRepository)
		expectErr  bool
		errMessage string
	}{
		{
			name:   "successful delete with multiple projects",
			teamID: "team-123",
			userID: "user-123",
			slug:   "test-project",
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				project := createTestProject()
				mockRepo.EXPECT().
					GetBySlug(mock.Anything, "team-123", "user-123", "test-project").
					Return(project, nil).
					Once()
				mockRepo.EXPECT().CountByTeamID(mock.Anything, "team-123").Return(2, nil).Once()
				mockRepo.EXPECT().Delete(mock.Anything, "team-123", "user-123", "test-project").Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:   "cannot delete last project in team",
			teamID: "team-123",
			userID: "user-123",
			slug:   "last-project",
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				project := createTestProject()
				project.Slug = "last-project"
				mockRepo.EXPECT().
					GetBySlug(mock.Anything, "team-123", "user-123", "last-project").
					Return(project, nil).
					Once()
				mockRepo.EXPECT().CountByTeamID(mock.Anything, "team-123").Return(1, nil).Once()
			},
			expectErr:  true,
			errMessage: "teams must have at least one project",
		},
		{
			name:   "project not found on get",
			teamID: "team-123",
			userID: "user-123",
			slug:   "nonexistent",
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				mockRepo.EXPECT().GetBySlug(mock.Anything, "team-123", "user-123", "nonexistent").
					Return(nil, fmt.Errorf("project not found")).Once()
			},
			expectErr:  true,
			errMessage: "project not found",
		},
		{
			name:   "count error",
			teamID: "team-123",
			userID: "user-123",
			slug:   "test-project",
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				project := createTestProject()
				mockRepo.EXPECT().
					GetBySlug(mock.Anything, "team-123", "user-123", "test-project").
					Return(project, nil).
					Once()
				mockRepo.EXPECT().CountByTeamID(mock.Anything, "team-123").
					Return(0, fmt.Errorf("database error")).Once()
			},
			expectErr:  true,
			errMessage: "failed to verify project count",
		},
		{
			name:   "repository delete error",
			teamID: "team-123",
			userID: "user-123",
			slug:   "test-project",
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				project := createTestProject()
				mockRepo.EXPECT().
					GetBySlug(mock.Anything, "team-123", "user-123", "test-project").
					Return(project, nil).
					Once()
				mockRepo.EXPECT().CountByTeamID(mock.Anything, "team-123").Return(2, nil).Once()
				mockRepo.EXPECT().Delete(mock.Anything, "team-123", "user-123", "test-project").
					Return(fmt.Errorf("database error")).Once()
			},
			expectErr:  true,
			errMessage: "database error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockProjectRepository(t)
			tt.setupMock(mockRepo)

			service := createTestProjectService(mockRepo)
			err := service.DeleteProject(tt.teamID, tt.userID, tt.slug)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestProjectService_DeleteProject_LastProjectError verifies the custom error type
func TestProjectService_DeleteProject_LastProjectError(t *testing.T) {
	mockRepo := mocks.NewMockProjectRepository(t)

	project := createTestProject()
	project.Slug = "last-project"

	mockRepo.EXPECT().GetBySlug(mock.Anything, "team-123", "user-123", "last-project").Return(project, nil).Once()
	mockRepo.EXPECT().CountByTeamID(mock.Anything, "team-123").Return(1, nil).Once()

	service := createTestProjectService(mockRepo)
	err := service.DeleteProject("team-123", "user-123", "last-project")

	assert.Error(t, err)

	// Verify the error is of the correct type
	var lastProjectErr *CannotDeleteLastProjectError
	assert.ErrorAs(t, err, &lastProjectErr)
	assert.Equal(t, "team-123", lastProjectErr.TeamID)
	assert.Equal(t, "last-project", lastProjectErr.ProjectSlug)
}

func TestProjectService_ListProjects_Pagination(t *testing.T) {
	mockRepo := mocks.NewMockProjectRepository(t)

	// Create multiple projects for pagination test
	projects := make([]models.Project, 25)
	for i := 0; i < 25; i++ {
		projects[i] = models.Project{
			ID:        fmt.Sprintf("project-%d", i),
			UserID:    "user-123",
			TeamID:    "team-123",
			Name:      fmt.Sprintf("Project %d", i),
			Slug:      fmt.Sprintf("project-%d", i),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Version:   1,
		}
	}

	// Return first page (10 items)
	mockRepo.EXPECT().List(mock.Anything, "user-123", mock.MatchedBy(func(f repositories.ProjectListFilters) bool {
		return f.TeamID == "team-123" && f.Page == 1 && f.Limit == 10
	})).Return(projects[:10], 25, nil).Once()

	service := createTestProjectService(mockRepo)
	result, err := service.ListProjects("user-123", ProjectFilters{TeamID: "team-123", Page: 1, Limit: 10})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Projects, 10)
	assert.Equal(t, 25, result.TotalCount)
	assert.Equal(t, 3, result.TotalPages)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 10, result.PerPage)
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestProjectService_GetProjectStats(t *testing.T) {
	tests := []struct {
		name       string
		teamID     string
		userID     string
		slug       string
		setupMock  func(*mocks.MockProjectRepository)
		expected   *models.ProjectStatsResponse
		expectErr  bool
		errMessage string
	}{
		{
			name:   "successful stats",
			teamID: "team-123",
			userID: "user-123",
			slug:   "test-project",
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				stats := &models.ProjectStatsResponse{
					TotalPrompts:    5,
					TotalArtifacts:  3,
					TotalBlueprints: 2,
					TotalMemories:   7,
					TotalFeedItems:  1,
				}
				mockRepo.EXPECT().
					GetProjectStats(mock.Anything, "team-123", "user-123", "test-project").
					Return(stats, nil).
					Once()
			},
			expected: &models.ProjectStatsResponse{
				TotalPrompts:    5,
				TotalArtifacts:  3,
				TotalBlueprints: 2,
				TotalMemories:   7,
				TotalFeedItems:  1,
			},
			expectErr: false,
		},
		{
			name:   "all zero counts",
			teamID: "team-123",
			userID: "user-123",
			slug:   "empty-project",
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				stats := &models.ProjectStatsResponse{}
				mockRepo.EXPECT().
					GetProjectStats(mock.Anything, "team-123", "user-123", "empty-project").
					Return(stats, nil).
					Once()
			},
			expected:  &models.ProjectStatsResponse{},
			expectErr: false,
		},
		{
			name:   "project not found",
			teamID: "team-123",
			userID: "user-123",
			slug:   "nonexistent",
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				mockRepo.EXPECT().GetProjectStats(mock.Anything, "team-123", "user-123", "nonexistent").
					Return(
						nil,
						fmt.Errorf("%w: slug=nonexistent team=team-123", repositories.ErrProjectNotFoundForRepo),
					).
					Once()
			},
			expected:   nil,
			expectErr:  true,
			errMessage: "project not found for repository",
		},
		{
			name:   "repository error",
			teamID: "team-123",
			userID: "user-123",
			slug:   "test-project",
			setupMock: func(mockRepo *mocks.MockProjectRepository) {
				mockRepo.EXPECT().GetProjectStats(mock.Anything, "team-123", "user-123", "test-project").
					Return(nil, fmt.Errorf("database connection error")).Once()
			},
			expected:   nil,
			expectErr:  true,
			errMessage: "database connection error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mocks.NewMockProjectRepository(t)
			tt.setupMock(mockRepo)

			service := createTestProjectService(mockRepo)
			result, err := service.GetProjectStats(tt.teamID, tt.userID, tt.slug)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
