package services

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
	event_mocks "github.com/vibexp/vibexp/pkg/events/mocks"
)

// MockArtifactRepository is a mock implementation of repositories.ArtifactRepository
type MockArtifactRepository struct {
	mock.Mock
}

// MockResourceUsageService is a mock implementation of ResourceUsageServiceInterface
type MockResourceUsageService struct {
	mock.Mock
}

func (m *MockResourceUsageService) CheckResourceLimit(ctx context.Context, userID, resourceType string) (bool, error) {
	args := m.Called(ctx, userID, resourceType)
	return args.Bool(0), args.Error(1)
}

func (m *MockResourceUsageService) TrackResourceCreation(
	ctx context.Context, userID, resourceType, resourceID string,
) error {
	args := m.Called(ctx, userID, resourceType, resourceID)
	return args.Error(0)
}

func (m *MockResourceUsageService) TrackResourceDeletion(
	ctx context.Context, userID, resourceType, resourceID string,
) error {
	args := m.Called(ctx, userID, resourceType, resourceID)
	return args.Error(0)
}

func (m *MockResourceUsageService) GetResourceUsage(
	ctx context.Context, userID string,
) (*models.ResourceUsageResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ResourceUsageResponse), args.Error(1)
}

func (m *MockArtifactRepository) Create(ctx context.Context, artifact *models.Artifact) error {
	args := m.Called(ctx, artifact)
	// Set ID to simulate repository behavior
	if args.Error(0) == nil {
		artifact.ID = "artifact-123"
	}
	return args.Error(0)
}

func (m *MockArtifactRepository) GetByID(
	ctx context.Context, userID, teamID, artifactID string,
) (*models.Artifact, error) {
	args := m.Called(ctx, userID, teamID, artifactID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Artifact), args.Error(1)
}

func (m *MockArtifactRepository) GetByProjectIDAndSlug(
	ctx context.Context, userID, teamID, projectID, slug string,
) (*models.Artifact, error) {
	args := m.Called(ctx, userID, teamID, projectID, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Artifact), args.Error(1)
}

func (m *MockArtifactRepository) List(
	ctx context.Context, userID string, filters repositories.ArtifactFilters,
) ([]models.Artifact, int, error) {
	args := m.Called(ctx, userID, filters)
	return args.Get(0).([]models.Artifact), args.Int(1), args.Error(2)
}

func (m *MockArtifactRepository) Update(ctx context.Context, artifact *models.Artifact) error {
	args := m.Called(ctx, artifact)
	return args.Error(0)
}

func (m *MockArtifactRepository) Delete(ctx context.Context, userID, teamID, artifactID string) error {
	args := m.Called(ctx, userID, teamID, artifactID)
	return args.Error(0)
}

func (m *MockArtifactRepository) GetStats(
	ctx context.Context, userID, teamID string,
) (*models.ArtifactStatsResponse, error) {
	args := m.Called(ctx, userID, teamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ArtifactStatsResponse), args.Error(1)
}

func (m *MockArtifactRepository) CountAll(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func (m *MockArtifactRepository) GetByIDCrossTeam(
	ctx context.Context, userID, artifactID string,
) (*models.Artifact, error) {
	args := m.Called(ctx, userID, artifactID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Artifact), args.Error(1)
}

func (m *MockArtifactRepository) GetByProjectIDAndSlugCrossTeam(
	ctx context.Context, userID, projectID, slug string,
) (*models.Artifact, error) {
	args := m.Called(ctx, userID, projectID, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Artifact), args.Error(1)
}

func (m *MockArtifactRepository) ListCrossTeam(
	ctx context.Context, userID string, filters repositories.ArtifactFilters,
) ([]models.Artifact, int, error) {
	args := m.Called(ctx, userID, filters)
	return args.Get(0).([]models.Artifact), args.Int(1), args.Error(2)
}

func (m *MockArtifactRepository) GetNamesByIDsCrossTeam(
	_ context.Context, _ string, _ []string,
) (map[string]string, error) {
	return map[string]string{}, nil
}

func TestNewArtifactService(t *testing.T) {
	repo := &MockArtifactRepository{}
	mockResourceUsageSvc := &MockResourceUsageService{}
	service := NewArtifactService(
		repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
		func() *slog.Logger { l, _ := logtest.New(); return l }(),
		nil,
	)

	assert.NotNil(t, service)
	assert.IsType(t, &ArtifactService{}, service)
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestArtifactService_CreateArtifact(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		request  *models.CreateArtifactRequest
		setup    func(*MockArtifactRepository)
		expected func(*testing.T, *models.Artifact, error)
	}{
		{
			name:   "Successful creation with defaults",
			userID: "user-123",
			request: &models.CreateArtifactRequest{
				ProjectID: "550e8400-e29b-41d4-a716-446655440000",
				Slug:      "test-artifact",
				Title:     "Test Artifact",
				Content:   "Test content",
			},
			setup: func(repo *MockArtifactRepository) {
				repo.On("Create", mock.Anything, mock.MatchedBy(func(artifact *models.Artifact) bool {
					return artifact.ProjectID == "550e8400-e29b-41d4-a716-446655440000" &&
						artifact.Slug == "test-artifact" &&
						artifact.Title == "Test Artifact" &&
						artifact.Content == "Test content" &&
						artifact.Status == "active" &&
						artifact.Type == "general" &&
						artifact.UserID == "user-123"
				})).Return(nil)
			},
			expected: func(t *testing.T, artifact *models.Artifact, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, artifact)
				assert.Equal(t, "artifact-123", artifact.ID)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", artifact.ProjectID)
				assert.Equal(t, "test-artifact", artifact.Slug)
				assert.Equal(t, "Test Artifact", artifact.Title)
				assert.Equal(t, "Test content", artifact.Content)
				assert.Equal(t, "active", artifact.Status)
				assert.Equal(t, "general", artifact.Type)
				assert.Equal(t, "user-123", artifact.UserID)
				assert.NotNil(t, artifact.Metadata)
			},
		},
		{
			name:   "Successful creation with custom values",
			userID: "user-123",
			request: &models.CreateArtifactRequest{
				ProjectID:   "550e8400-e29b-41d4-a716-446655440001",
				Slug:        "custom-artifact",
				Title:       "Custom Artifact",
				Content:     "Custom content",
				Description: "Custom description",
				Type:        "work_reports",
				Status:      "archived",
				Metadata:    map[string]interface{}{"custom": "value"},
			},
			setup: func(repo *MockArtifactRepository) {
				repo.On("Create", mock.Anything, mock.MatchedBy(func(artifact *models.Artifact) bool {
					return artifact.ProjectID == "550e8400-e29b-41d4-a716-446655440001" &&
						artifact.Type == "work_reports" &&
						artifact.Status == "archived"
				})).Return(nil)
			},
			expected: func(t *testing.T, artifact *models.Artifact, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, artifact)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440001", artifact.ProjectID)
				assert.Equal(t, "work_reports", artifact.Type)
				assert.Equal(t, "archived", artifact.Status)
				assert.Equal(t, "Custom description", artifact.Description)
				assert.Equal(t, "value", artifact.Metadata["custom"])
			},
		},
		{
			name:   "Repository error",
			userID: "user-123",
			request: &models.CreateArtifactRequest{
				Slug:    "test-artifact",
				Title:   "Test Artifact",
				Content: "Test content",
			},
			setup: func(repo *MockArtifactRepository) {
				repo.On("Create", mock.Anything, mock.Anything).Return(assert.AnError)
			},
			expected: func(t *testing.T, artifact *models.Artifact, err error) {
				assert.Error(t, err)
				assert.Nil(t, artifact)
				assert.Equal(t, assert.AnError, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockArtifactRepository{}
			mockResourceUsageSvc := &MockResourceUsageService{}

			// Setup repository expectations
			tt.setup(repo)

			// Note: TrackResourceCreation was removed as part of resource tracking simplification
			// Resource limits are now checked directly in handlers via CheckResourceLimit

			service := NewArtifactService(
				repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
			)
			artifact, err := service.CreateArtifact(tt.userID, "team-123", tt.request)

			tt.expected(t, artifact, err)
			repo.AssertExpectations(t)
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestArtifactService_GetArtifactByProjectIDAndSlug(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		projectName string
		slug        string
		setup       func(*MockArtifactRepository)
		expected    func(*testing.T, *models.Artifact, error)
	}{
		{
			name:        "Successful retrieval",
			userID:      "user-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "test-slug",
			setup: func(repo *MockArtifactRepository) {
				artifact := &models.Artifact{
					ID:        "artifact-123",
					ProjectID: "550e8400-e29b-41d4-a716-446655440000",
					Slug:      "test-slug",
					UserID:    "user-123",
					Title:     "Test Artifact",
					Content:   "Test content",
				}
				repo.On(
					"GetByProjectIDAndSlugCrossTeam",
					mock.Anything,
					"user-123",
					"550e8400-e29b-41d4-a716-446655440000",
					"test-slug",
				).Return(artifact, nil)
			},
			expected: func(t *testing.T, artifact *models.Artifact, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, artifact)
				assert.Equal(t, "artifact-123", artifact.ID)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", artifact.ProjectID)
				assert.Equal(t, "test-slug", artifact.Slug)
			},
		},
		{
			name:        "Artifact not found",
			userID:      "user-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "non-existent",
			setup: func(repo *MockArtifactRepository) {
				repo.On(
					"GetByProjectIDAndSlugCrossTeam",
					mock.Anything, "user-123",
					"550e8400-e29b-41d4-a716-446655440000", "non-existent",
				).Return(nil, assert.AnError)
			},
			expected: func(t *testing.T, artifact *models.Artifact, err error) {
				assert.Error(t, err)
				assert.Nil(t, artifact)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockArtifactRepository{}
			tt.setup(repo)

			mockResourceUsageSvc := &MockResourceUsageService{}
			service := NewArtifactService(
				repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
			)
			artifact, err := service.GetArtifactByProjectIDAndSlug(tt.userID, tt.projectName, tt.slug)

			tt.expected(t, artifact, err)
			repo.AssertExpectations(t)
			mockResourceUsageSvc.AssertExpectations(t)
		})
	}
}

// TestArtifactService_GetArtifactByProjectIDAndSlugInTeam verifies the team-scoped
// lookup routes through the team-scoped repository method (which constrains by team_id)
// and never the cross-team method, so a caller cannot reach an artifact in another team.
func TestArtifactService_GetArtifactByProjectIDAndSlugInTeam(t *testing.T) {
	const (
		userID    = "user-123"
		teamID    = "550e8400-e29b-41d4-a716-446655440000"
		projectID = "550e8400-e29b-41d4-a716-446655440010"
		slug      = "test-slug"
	)

	tests := []struct {
		name     string
		setup    func(*MockArtifactRepository)
		expected func(*testing.T, *models.Artifact, error)
	}{
		{
			name: "Successful team-scoped retrieval",
			setup: func(repo *MockArtifactRepository) {
				repo.On(
					"GetByProjectIDAndSlug", mock.Anything, userID, teamID, projectID, slug,
				).Return(&models.Artifact{ID: "artifact-123", ProjectID: projectID, Slug: slug, TeamID: teamID}, nil)
			},
			expected: func(t *testing.T, artifact *models.Artifact, err error) {
				require.NoError(t, err)
				require.NotNil(t, artifact)
				assert.Equal(t, "artifact-123", artifact.ID)
				assert.Equal(t, teamID, artifact.TeamID)
			},
		},
		{
			name: "Artifact in a different team is not found",
			setup: func(repo *MockArtifactRepository) {
				repo.On(
					"GetByProjectIDAndSlug", mock.Anything, userID, teamID, projectID, slug,
				).Return(nil, assert.AnError)
			},
			expected: func(t *testing.T, artifact *models.Artifact, err error) {
				assert.Error(t, err)
				assert.Nil(t, artifact)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockArtifactRepository{}
			tt.setup(repo)

			service := NewArtifactService(
				repo, nil, allowAllAuthz{}, nil, &MockResourceUsageService{},
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
			)
			artifact, err := service.GetArtifactByProjectIDAndSlugInTeam(userID, teamID, projectID, slug)

			tt.expected(t, artifact, err)
			repo.AssertExpectations(t)
			repo.AssertNotCalled(t, "GetByProjectIDAndSlugCrossTeam")
		})
	}
}

// TestArtifactService_UpdateArtifactByProjectIDAndSlugInTeam verifies the team-scoped
// update loads the artifact via the team-scoped repository method (constrained by team_id)
// before mutating, and never falls through to the cross-team lookup.
func TestArtifactService_UpdateArtifactByProjectIDAndSlugInTeam(t *testing.T) {
	const (
		userID    = "user-123"
		teamID    = "550e8400-e29b-41d4-a716-446655440000"
		projectID = "550e8400-e29b-41d4-a716-446655440010"
		slug      = "test-slug"
	)

	t.Run("Successful team-scoped update", func(t *testing.T) {
		repo := &MockArtifactRepository{}
		repo.On(
			"GetByProjectIDAndSlug", mock.Anything, userID, teamID, projectID, slug,
		).Return(&models.Artifact{ID: "artifact-123", ProjectID: projectID, Slug: slug, TeamID: teamID}, nil)
		repo.On("Update", mock.Anything, mock.MatchedBy(func(a *models.Artifact) bool {
			return a.Title == "Updated Title" && a.TeamID == teamID
		})).Return(nil)

		service := NewArtifactService(
			repo, nil, allowAllAuthz{}, nil, &MockResourceUsageService{},
			func() *slog.Logger { l, _ := logtest.New(); return l }(),
			nil,
		)
		artifact, err := service.UpdateArtifactByProjectIDAndSlugInTeam(
			userID, teamID, projectID, slug, &models.UpdateArtifactRequest{Title: stringPtr("Updated Title")},
		)

		require.NoError(t, err)
		require.NotNil(t, artifact)
		assert.Equal(t, "Updated Title", artifact.Title)
		repo.AssertExpectations(t)
		repo.AssertNotCalled(t, "GetByProjectIDAndSlugCrossTeam")
	})

	t.Run("Artifact in a different team is not updated", func(t *testing.T) {
		repo := &MockArtifactRepository{}
		repo.On(
			"GetByProjectIDAndSlug", mock.Anything, userID, teamID, projectID, slug,
		).Return(nil, assert.AnError)

		service := NewArtifactService(
			repo, nil, allowAllAuthz{}, nil, &MockResourceUsageService{},
			func() *slog.Logger { l, _ := logtest.New(); return l }(),
			nil,
		)
		artifact, err := service.UpdateArtifactByProjectIDAndSlugInTeam(
			userID, teamID, projectID, slug, &models.UpdateArtifactRequest{Title: stringPtr("Hijacked")},
		)

		assert.Error(t, err)
		assert.Nil(t, artifact)
		repo.AssertExpectations(t)
		repo.AssertNotCalled(t, "Update")
		repo.AssertNotCalled(t, "GetByProjectIDAndSlugCrossTeam")
	})
}

//nolint:gocyclo,funlen // Test function with multiple test cases requires higher complexity and length
func TestArtifactService_ListArtifacts(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		filters  ArtifactFilters
		setup    func(*MockArtifactRepository)
		expected func(*testing.T, *models.ArtifactListResponse, error)
	}{
		{
			name:   "Successful list with filters",
			userID: "user-123",
			filters: ArtifactFilters{
				ProjectID: "550e8400-e29b-41d4-a716-446655440000",
				Status:    "active",
				Type:      "general",
				Search:    "test",
				SortBy:    "created_at",
				SortOrder: "desc",
				Metadata:  map[string]string{"key": "value"},
				Page:      1,
				Limit:     20,
			},
			setup: func(repo *MockArtifactRepository) {
				artifacts := []models.Artifact{
					{
						ID:    "artifact-1",
						Title: "Test Artifact 1",
					},
					{
						ID:    "artifact-2",
						Title: "Test Artifact 2",
					},
				}
				repo.On("List", mock.Anything, "user-123", mock.MatchedBy(func(filters repositories.ArtifactFilters) bool {
					return *filters.ProjectID == "550e8400-e29b-41d4-a716-446655440000" &&
						*filters.Status == "active" &&
						*filters.Type == "general" &&
						filters.Search == "test" &&
						filters.SortBy == "created_at" &&
						filters.SortOrder == "desc" &&
						filters.Metadata["key"] == "value" &&
						filters.Page == 1 &&
						filters.Limit == 20
				})).Return(artifacts, 2, nil)
			},
			expected: func(t *testing.T, response *models.ArtifactListResponse, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Len(t, response.Artifacts, 2)
				assert.Equal(t, 2, response.TotalCount)
				assert.Equal(t, 1, response.Page)
				assert.Equal(t, 20, response.PerPage)
				assert.Equal(t, 1, response.TotalPages)
			},
		},
		{
			name:   "Empty filters with defaults",
			userID: "user-123",
			filters: ArtifactFilters{
				Page:  1,
				Limit: 10,
			},
			setup: func(repo *MockArtifactRepository) {
				repo.On("List", mock.Anything, "user-123", mock.MatchedBy(func(filters repositories.ArtifactFilters) bool {
					return filters.ProjectID == nil &&
						filters.Status == nil &&
						filters.Type == nil &&
						filters.Search == "" &&
						filters.Page == 1 &&
						filters.Limit == 10
				})).Return([]models.Artifact{}, 0, nil)
			},
			expected: func(t *testing.T, response *models.ArtifactListResponse, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Empty(t, response.Artifacts)
				assert.Zero(t, response.TotalCount)
			},
		},
		{
			name:    "Repository error",
			userID:  "user-123",
			filters: ArtifactFilters{Page: 1, Limit: 10},
			setup: func(repo *MockArtifactRepository) {
				repo.On("List", mock.Anything, "user-123", mock.Anything).Return([]models.Artifact{}, 0, assert.AnError)
			},
			expected: func(t *testing.T, response *models.ArtifactListResponse, err error) {
				assert.Error(t, err)
				assert.Nil(t, response)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockArtifactRepository{}
			tt.setup(repo)

			mockResourceUsageSvc := &MockResourceUsageService{}
			service := NewArtifactService(
				repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
			)
			response, err := service.ListArtifacts(tt.userID, tt.filters)

			tt.expected(t, response, err)
			repo.AssertExpectations(t)
		})
	}
}

func TestArtifactService_ListArtifactsByProject(t *testing.T) {
	repo := &MockArtifactRepository{}
	artifacts := []models.Artifact{
		{ID: "artifact-1", ProjectID: "550e8400-e29b-41d4-a716-446655440000"},
	}
	repo.On("List", mock.Anything, "user-123", mock.MatchedBy(func(filters repositories.ArtifactFilters) bool {
		return *filters.ProjectID == "550e8400-e29b-41d4-a716-446655440000"
	})).Return(artifacts, 1, nil)

	mockResourceUsageSvc := &MockResourceUsageService{}
	service := NewArtifactService(
		repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
		func() *slog.Logger { l, _ := logtest.New(); return l }(),
		nil,
	)
	response, err := service.ListArtifactsByProject(
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		ArtifactFilters{Page: 1, Limit: 10},
	)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Artifacts, 1)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", response.Artifacts[0].ProjectID)
	repo.AssertExpectations(t)
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestArtifactService_UpdateArtifactByProjectIDAndSlug(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		projectName string
		slug        string
		request     *models.UpdateArtifactRequest
		setup       func(*MockArtifactRepository)
		expected    func(*testing.T, *models.Artifact, error)
	}{
		{
			name:        "Successful update with multiple fields",
			userID:      "user-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "test-slug",
			request: &models.UpdateArtifactRequest{
				Title:       stringPtr("Updated Title"),
				Description: stringPtr("Updated Description"),
				Content:     stringPtr("Updated Content"),
				Status:      stringPtr("archived"),
				Type:        stringPtr("work_reports"),
				Metadata:    map[string]interface{}{"updated": "value"},
			},
			setup: func(repo *MockArtifactRepository) {
				existingArtifact := &models.Artifact{
					ID:          "artifact-123",
					ProjectID:   "550e8400-e29b-41d4-a716-446655440000",
					Slug:        "test-slug",
					UserID:      "user-123",
					Title:       "Original Title",
					Description: "Original Description",
					Content:     "Original Content",
					Status:      "active",
					Type:        "general",
					Metadata:    map[string]interface{}{"original": "value"},
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				repo.On(
					"GetByProjectIDAndSlugCrossTeam",
					mock.Anything,
					"user-123",
					"550e8400-e29b-41d4-a716-446655440000",
					"test-slug",
				).Return(existingArtifact, nil)
				repo.On("Update", mock.Anything, mock.MatchedBy(func(artifact *models.Artifact) bool {
					return artifact.Title == "Updated Title" &&
						artifact.Description == "Updated Description" &&
						artifact.Content == "Updated Content" &&
						artifact.Status == "archived" &&
						artifact.Type == "work_reports" &&
						artifact.Metadata["updated"] == "value"
				})).Return(nil)
			},
			expected: func(t *testing.T, artifact *models.Artifact, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, artifact)
				assert.Equal(t, "Updated Title", artifact.Title)
				assert.Equal(t, "Updated Description", artifact.Description)
				assert.Equal(t, "Updated Content", artifact.Content)
				assert.Equal(t, "archived", artifact.Status)
				assert.Equal(t, "work_reports", artifact.Type)
				assert.Equal(t, "value", artifact.Metadata["updated"])
			},
		},
		{
			name:        "Artifact not found",
			userID:      "user-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "non-existent",
			request:     &models.UpdateArtifactRequest{},
			setup: func(repo *MockArtifactRepository) {
				repo.On(
					"GetByProjectIDAndSlugCrossTeam",
					mock.Anything, "user-123",
					"550e8400-e29b-41d4-a716-446655440000", "non-existent",
				).Return(nil, assert.AnError)
			},
			expected: func(t *testing.T, artifact *models.Artifact, err error) {
				assert.Error(t, err)
				assert.Nil(t, artifact)
			},
		},
		{
			name:        "Update error",
			userID:      "user-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "test-slug",
			request: &models.UpdateArtifactRequest{
				Title: stringPtr("Updated Title"),
			},
			setup: func(repo *MockArtifactRepository) {
				existingArtifact := &models.Artifact{
					ID:        "artifact-123",
					ProjectID: "550e8400-e29b-41d4-a716-446655440000",
					Slug:      "test-slug",
					UserID:    "user-123",
					Title:     "Original Title",
				}
				repo.On(
					"GetByProjectIDAndSlugCrossTeam",
					mock.Anything,
					"user-123", "550e8400-e29b-41d4-a716-446655440000",
					"test-slug",
				).Return(existingArtifact, nil)
				repo.On("Update", mock.Anything, mock.Anything).Return(assert.AnError)
			},
			expected: func(t *testing.T, artifact *models.Artifact, err error) {
				assert.Error(t, err)
				assert.Nil(t, artifact)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockArtifactRepository{}
			tt.setup(repo)

			mockResourceUsageSvc := &MockResourceUsageService{}
			service := NewArtifactService(
				repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
			)
			artifact, err := service.UpdateArtifactByProjectIDAndSlug(tt.userID, tt.projectName, tt.slug, tt.request)

			tt.expected(t, artifact, err)
			repo.AssertExpectations(t)
			mockResourceUsageSvc.AssertExpectations(t)
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestArtifactService_DeleteArtifactByProjectIDAndSlug(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		teamID      string
		projectName string
		slug        string
		setup       func(*MockArtifactRepository)
		expected    func(*testing.T, error)
	}{
		{
			name:        "Successful deletion",
			userID:      "user-123",
			teamID:      "team-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "test-slug",
			setup: func(repo *MockArtifactRepository) {
				artifact := &models.Artifact{
					ID:        "artifact-123",
					ProjectID: "550e8400-e29b-41d4-a716-446655440000",
					Slug:      "test-slug",
					UserID:    "user-123",
				}
				repo.On(
					"GetByProjectIDAndSlug",
					mock.Anything,
					"user-123", "team-123", "550e8400-e29b-41d4-a716-446655440000",
					"test-slug",
				).Return(artifact, nil)
				repo.On("Delete", mock.Anything, "user-123", mock.Anything, "artifact-123").Return(nil)
			},
			expected: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:        "Artifact not found",
			userID:      "user-123",
			teamID:      "team-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "non-existent",
			setup: func(repo *MockArtifactRepository) {
				repo.On(
					"GetByProjectIDAndSlug",
					mock.Anything, "user-123", "team-123",
					"550e8400-e29b-41d4-a716-446655440000", "non-existent",
				).Return(nil, assert.AnError)
			},
			expected: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
		{
			name:        "Repository error",
			userID:      "user-123",
			teamID:      "team-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "test-slug",
			setup: func(repo *MockArtifactRepository) {
				artifact := &models.Artifact{
					ID:        "artifact-123",
					ProjectID: "550e8400-e29b-41d4-a716-446655440000",
					Slug:      "test-slug",
					UserID:    "user-123",
				}
				repo.On(
					"GetByProjectIDAndSlug",
					mock.Anything,
					"user-123", "team-123", "550e8400-e29b-41d4-a716-446655440000",
					"test-slug",
				).Return(artifact, nil)
				repo.On("Delete", mock.Anything, "user-123", mock.Anything, "artifact-123").Return(assert.AnError)
			},
			expected: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockArtifactRepository{}
			mockResourceUsageSvc := &MockResourceUsageService{}

			// Setup repository expectations
			tt.setup(repo)

			// Note: TrackResourceDeletion was removed as part of resource tracking simplification
			// Deletions are never blocked by resource limits

			service := NewArtifactService(
				repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
			)
			err := service.DeleteArtifactByProjectIDAndSlug(tt.userID, tt.teamID, tt.projectName, tt.slug)

			tt.expected(t, err)
			repo.AssertExpectations(t)
			//nolint:funlen // Test function requires comprehensive setup and assertions
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestArtifactService_GetArtifactStats(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		teamID   string
		setup    func(*MockArtifactRepository)
		expected func(*testing.T, *models.ArtifactStatsResponse, error)
	}{
		{
			name:   "Successful stats retrieval",
			userID: "user-123",
			teamID: "team-456",
			setup: func(repo *MockArtifactRepository) {
				stats := &models.ArtifactStatsResponse{
					TotalProjects:  3,
					TotalArtifacts: 10,
					AddedThisWeek:  2,
					TotalByType: map[string]int{
						"general":         5,
						"work_reports":    3,
						"static_contexts": 2,
					},
					TotalByStatus: map[string]int{
						"active":   8,
						"archived": 2,
					},
				}
				repo.On("GetStats", mock.Anything, "user-123", "team-456").Return(stats, nil)
			},
			expected: func(t *testing.T, stats *models.ArtifactStatsResponse, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, stats)
				assert.Equal(t, 3, stats.TotalProjects)
				assert.Equal(t, 10, stats.TotalArtifacts)
				assert.Equal(t, 2, stats.AddedThisWeek)
				assert.Equal(t, 5, stats.TotalByType["general"])
				assert.Equal(t, 8, stats.TotalByStatus["active"])
			},
		},
		{
			name:   "Repository error",
			userID: "user-123",
			teamID: "team-456",
			setup: func(repo *MockArtifactRepository) {
				repo.On("GetStats", mock.Anything, "user-123", "team-456").Return(nil, assert.AnError)
			},
			expected: func(t *testing.T, stats *models.ArtifactStatsResponse, err error) {
				assert.Error(t, err)
				assert.Nil(t, stats)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockArtifactRepository{}
			tt.setup(repo)

			mockResourceUsageSvc := &MockResourceUsageService{}
			service := NewArtifactService(
				repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
			)
			stats, err := service.GetArtifactStats(tt.userID, tt.teamID)

			tt.expected(t, stats, err)
			//nolint:funlen // Test function requires comprehensive setup and assertions
			repo.AssertExpectations(t)
			//nolint:funlen // Test function requires comprehensive setup and assertions
		})
		//nolint:funlen // Test function requires comprehensive setup and assertions
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen
func TestArtifactService_ListArtifactsByProjectCrossTeam(t *testing.T) {
	tests := []struct {
		name      string
		userID    string
		projectID string
		filters   ArtifactFilters
		setup     func(*MockArtifactRepository)
		expected  func(*testing.T, *models.ArtifactListResponse, error)
	}{
		{
			name:      "returns artifacts from multiple teams via user_id ownership",
			userID:    "user-alice",
			projectID: "project-multiTeam",
			filters:   ArtifactFilters{Page: 1, Limit: 20},
			setup: func(repo *MockArtifactRepository) {
				projectID := "project-multiTeam"
				artifacts := []models.Artifact{
					{ID: "a1", ProjectID: projectID, TeamID: "team-a", UserID: "user-alice"},
					{ID: "a2", ProjectID: projectID, TeamID: "team-b", UserID: "user-alice"},
				}
				repo.On("ListCrossTeam", mock.Anything, "user-alice",
					mock.MatchedBy(func(f repositories.ArtifactFilters) bool {
						return f.ProjectID != nil && *f.ProjectID == projectID &&
							f.TeamID == "" && // must NOT have TeamID
							f.Page == 1 && f.Limit == 20
					})).Return(artifacts, 2, nil)
			},
			expected: func(t *testing.T, resp *models.ArtifactListResponse, err error) {
				assert.NoError(t, err)
				require.NotNil(t, resp)
				assert.Len(t, resp.Artifacts, 2)
				assert.Equal(t, 2, resp.TotalCount)
				assert.Equal(t, 1, resp.TotalPages)
			},
		},
		{
			name:      "applies status and type filters",
			userID:    "user-alice",
			projectID: "project-filters",
			filters:   ArtifactFilters{Status: "active", Type: "work_reports", Page: 1, Limit: 10},
			setup: func(repo *MockArtifactRepository) {
				artifacts := []models.Artifact{
					{ID: "a1", Status: "active", Type: "work_reports"},
				}
				repo.On("ListCrossTeam", mock.Anything, "user-alice",
					mock.MatchedBy(func(f repositories.ArtifactFilters) bool {
						return f.Status != nil && *f.Status == "active" &&
							f.Type != nil && *f.Type == "work_reports" &&
							f.TeamID == ""
					})).Return(artifacts, 1, nil)
			},
			expected: func(t *testing.T, resp *models.ArtifactListResponse, err error) {
				assert.NoError(t, err)
				require.NotNil(t, resp)
				assert.Len(t, resp.Artifacts, 1)
			},
		},
		{
			name:      "propagates repository error",
			userID:    "user-alice",
			projectID: "project-error",
			filters:   ArtifactFilters{Page: 1, Limit: 10},
			setup: func(repo *MockArtifactRepository) {
				repo.On("ListCrossTeam", mock.Anything, "user-alice", mock.Anything).
					Return([]models.Artifact{}, 0, assert.AnError)
			},
			expected: func(t *testing.T, resp *models.ArtifactListResponse, err error) {
				assert.Error(t, err)
				assert.Nil(t, resp)
			},
		},
		{
			name:      "returns empty result when no artifacts found",
			userID:    "user-alice",
			projectID: "project-empty",
			filters:   ArtifactFilters{Page: 1, Limit: 20},
			setup: func(repo *MockArtifactRepository) {
				repo.On("ListCrossTeam", mock.Anything, "user-alice",
					mock.MatchedBy(func(f repositories.ArtifactFilters) bool {
						return f.TeamID == "" // cross-team: no TeamID
					})).Return([]models.Artifact{}, 0, nil)
			},
			expected: func(t *testing.T, resp *models.ArtifactListResponse, err error) {
				assert.NoError(t, err)
				require.NotNil(t, resp)
				assert.Empty(t, resp.Artifacts)
				assert.Zero(t, resp.TotalCount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockArtifactRepository{}
			tt.setup(repo)

			mockResourceUsageSvc := &MockResourceUsageService{}
			service := NewArtifactService(
				repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
			)
			resp, err := service.ListArtifactsByProjectCrossTeam(tt.userID, tt.projectID, tt.filters)

			tt.expected(t, resp, err)
			repo.AssertExpectations(t)
		})
	}
}

func TestArtifactService_ImplementsInterface(t *testing.T) {
	repo := &MockArtifactRepository{}
	mockResourceUsageSvc := &MockResourceUsageService{}
	service := NewArtifactService(
		repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
		func() *slog.Logger { l, _ := logtest.New(); return l }(),
		nil,
		//nolint:funlen // Test function requires comprehensive setup and assertions
	)
	//nolint:funlen // Test function requires comprehensive setup and assertions

	//nolint:funlen // Test function requires comprehensive setup and assertions
	// Verify that ArtifactService implements ArtifactServiceInterface
	//nolint:funlen // Test function requires comprehensive setup and assertions
	var _ ArtifactServiceInterface = service
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestArtifactService_PublishesArtifactEvents(t *testing.T) {
	tests := []struct {
		name             string
		setupMocks       func(*MockArtifactRepository, *event_mocks.MockEventPublisher)
		executeAction    func(*ArtifactService) error
		expectEventCalls int
		eventType        string
	}{
		{
			name: "publishes artifact.created event when creating artifact",
			setupMocks: func(mockRepo *MockArtifactRepository, mockEventManager *event_mocks.MockEventPublisher) {
				mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Artifact")).
					Return(nil).Run(func(args mock.Arguments) {
					artifact := args.Get(1).(*models.Artifact)
					artifact.ID = "artifact-new-123"
				})

				// Expect event to be published exactly once
				mockEventManager.On("Publish", mock.Anything, mock.MatchedBy(func(event events.Event) bool {
					return event.Type() == events.EventTypeArtifactCreated
				})).Return(nil).Once()
			},
			executeAction: func(service *ArtifactService) error {
				req := &models.CreateArtifactRequest{
					ProjectID:   "550e8400-e29b-41d4-a716-446655440000",
					Slug:        "test-artifact",
					Title:       "Test Artifact",
					Description: "Test Description",
					Content:     "Test Content",
					Type:        "general",
					Status:      "active",
				}
				_, err := service.CreateArtifact("user-123", "team-123", req)
				return err
			},
			expectEventCalls: 1,
			eventType:        events.EventTypeArtifactCreated,
		},
		{
			name: "publishes artifact.updated event when updating artifact",
			setupMocks: func(mockRepo *MockArtifactRepository, mockEventManager *event_mocks.MockEventPublisher) {
				existingArtifact := &models.Artifact{
					ID:        "artifact-123",
					ProjectID: "550e8400-e29b-41d4-a716-446655440000",
					Slug:      "test-artifact",
					Title:     "Original Title",
					UserID:    "user-123",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				mockRepo.On(
					"GetByProjectIDAndSlugCrossTeam",
					mock.Anything,
					"user-123", "550e8400-e29b-41d4-a716-446655440000",
					"test-artifact",
				).Return(existingArtifact, nil)

				mockRepo.On("Update", mock.Anything, mock.AnythingOfType("*models.Artifact")).
					Return(nil)

				// Expect event to be published exactly once
				mockEventManager.On("Publish", mock.Anything, mock.MatchedBy(func(event events.Event) bool {
					return event.Type() == events.EventTypeArtifactUpdated
				})).Return(nil).Once()
			},
			executeAction: func(service *ArtifactService) error {
				title := "Updated Title"
				req := &models.UpdateArtifactRequest{
					Title: &title,
				}
				_, err := service.UpdateArtifactByProjectIDAndSlug(
					"user-123",
					"550e8400-e29b-41d4-a716-446655440000",
					"test-artifact",
					req,
				)
				return err
			},
			expectEventCalls: 1,
			eventType:        events.EventTypeArtifactUpdated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockArtifactRepository{}
			mockEventManager := &event_mocks.MockEventPublisher{}
			mockResourceUsageSvc := &MockResourceUsageService{}

			// Setup mocks
			tt.setupMocks(mockRepo, mockEventManager)

			// Setup resource usage tracking expectations
			mockResourceUsageSvc.On(
				"TrackResourceCreation", mock.Anything, mock.Anything,
				events.ResourceTypeArtifact, mock.Anything,
			).Return(nil).Maybe()

			service := NewArtifactService(
				mockRepo, nil, allowAllAuthz{}, mockEventManager, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
			)

			err := tt.executeAction(service)
			assert.NoError(t, err)

			mockRepo.AssertExpectations(t)
			mockEventManager.AssertExpectations(t)
			mockResourceUsageSvc.AssertExpectations(t)

			// Verify the event was published the expected number of times
			if tt.expectEventCalls > 0 {
				mockEventManager.AssertNumberOfCalls(t, "Publish", tt.expectEventCalls)
			} else {
				mockEventManager.AssertNotCalled(t, "Publish")
			}
		})
	}
}

// TestArtifactService_UpdateArtifact_PreservesTeamID tests that team_id is preserved during update
func TestArtifactService_UpdateArtifact_PreservesTeamID(t *testing.T) {
	mockRepo := &MockArtifactRepository{}
	mockResourceUsageSvc := &MockResourceUsageService{}
	service := NewArtifactService(
		mockRepo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
		func() *slog.Logger { l, _ := logtest.New(); return l }(),
		nil,
	)

	// Create existing artifact with team_id
	existingArtifact := &models.Artifact{
		ID:        "artifact-123",
		ProjectID: "project-456",
		Slug:      "test-artifact",
		Title:     "Original Title",
		UserID:    "user-123",
		TeamID:    "team-999",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	mockRepo.On(
		"GetByProjectIDAndSlugCrossTeam",
		mock.Anything,
		"user-123",
		"project-456",
		"test-artifact",
	).Return(existingArtifact, nil)

	// Verify that Update is called with team_id preserved
	mockRepo.On("Update", mock.Anything,
		mock.MatchedBy(func(artifact *models.Artifact) bool {
			return artifact.ID == "artifact-123" &&
				artifact.TeamID == "team-999" && // TeamID must be preserved
				artifact.Title == "Updated Title"
		})).Return(nil)

	title := "Updated Title"
	request := &models.UpdateArtifactRequest{
		Title: &title,
	}

	artifact, err := service.UpdateArtifactByProjectIDAndSlug("user-123", "project-456", "test-artifact", request)

	assert.NoError(t, err)
	assert.NotNil(t, artifact)
	assert.Equal(t, "team-999", artifact.TeamID, "TeamID should be preserved during update")
	mockRepo.AssertExpectations(t)
}
