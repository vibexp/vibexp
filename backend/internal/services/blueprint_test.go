package services

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
	event_mocks "github.com/vibexp/vibexp/pkg/events/mocks"
)

// MockBlueprintRepository is a mock implementation of repositories.BlueprintRepository
type MockBlueprintRepository struct {
	mock.Mock
}

func (m *MockBlueprintRepository) Create(ctx context.Context, blueprint *models.Blueprint) error {
	args := m.Called(ctx, blueprint)
	// Set ID to simulate repository behavior
	if args.Error(0) == nil {
		blueprint.ID = "spec-library-123"
	}
	return args.Error(0)
}

func (m *MockBlueprintRepository) GetByID(
	ctx context.Context, userID, teamID, blueprintID string,
) (*models.Blueprint, error) {
	args := m.Called(ctx, userID, teamID, blueprintID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Blueprint), args.Error(1)
}

func (m *MockBlueprintRepository) GetByProjectIDAndSlug(
	ctx context.Context, userID, teamID, projectID, slug string,
) (*models.Blueprint, error) {
	args := m.Called(ctx, userID, teamID, projectID, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Blueprint), args.Error(1)
}

func (m *MockBlueprintRepository) List(
	ctx context.Context, userID string, filters repositories.BlueprintFilters,
) ([]models.Blueprint, int, error) {
	args := m.Called(ctx, userID, filters)
	return args.Get(0).([]models.Blueprint), args.Int(1), args.Error(2)
}

func (m *MockBlueprintRepository) ListProjects(
	ctx context.Context, userID string, filters repositories.ProjectFilters,
) ([]repositories.ProjectInfo, int, error) {
	args := m.Called(ctx, userID, filters)
	return args.Get(0).([]repositories.ProjectInfo), args.Int(1), args.Error(2)
}

func (m *MockBlueprintRepository) Update(ctx context.Context, blueprint *models.Blueprint) error {
	args := m.Called(ctx, blueprint)
	return args.Error(0)
}

func (m *MockBlueprintRepository) Delete(ctx context.Context, userID, teamID, blueprintID string) error {
	args := m.Called(ctx, userID, teamID, blueprintID)
	return args.Error(0)
}

func (m *MockBlueprintRepository) GetStats(
	ctx context.Context, userID string,
) (*models.BlueprintStatsResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.BlueprintStatsResponse), args.Error(1)
}

func (m *MockBlueprintRepository) GetByProjectIDAndSlugCrossTeam(
	ctx context.Context, userID, projectID, slug string,
) (*models.Blueprint, error) {
	args := m.Called(ctx, userID, projectID, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Blueprint), args.Error(1)
}

func (m *MockBlueprintRepository) GetByIDCrossTeam(
	ctx context.Context, userID, blueprintID string,
) (*models.Blueprint, error) {
	args := m.Called(ctx, userID, blueprintID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Blueprint), args.Error(1)
}

func (m *MockBlueprintRepository) GetNamesByIDsCrossTeam(
	ctx context.Context, userID string, ids []string,
) (map[string]string, error) {
	args := m.Called(ctx, userID, ids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]string), args.Error(1)
}

func TestNewBlueprintService(t *testing.T) {
	repo := &MockBlueprintRepository{}
	mockResourceUsageSvc := &MockResourceUsageService{}
	service := NewBlueprintService(
		repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
		func() *slog.Logger { l, _ := logtest.New(); return l }(),
		nil,
		nil)

	assert.NotNil(t, service)
	assert.IsType(t, &BlueprintService{}, service)
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestBlueprintService_CreateBlueprint(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		request  *models.CreateBlueprintRequest
		setup    func(*MockBlueprintRepository)
		expected func(*testing.T, *models.Blueprint, error)
	}{
		{
			name:   "Successful creation with defaults",
			userID: "user-123",
			request: &models.CreateBlueprintRequest{
				ProjectID: "550e8400-e29b-41d4-a716-446655440000",
				Slug:      "test-spec-library",
				Title:     "Test Blueprint",
				Content:   "Test content",
			},
			setup: func(repo *MockBlueprintRepository) {
				repo.On("Create", mock.Anything, mock.MatchedBy(func(blueprint *models.Blueprint) bool {
					return blueprint.ProjectID == "550e8400-e29b-41d4-a716-446655440000" &&
						blueprint.Slug == "test-spec-library" &&
						blueprint.Title == "Test Blueprint" &&
						blueprint.Content == "Test content" &&
						blueprint.Status == "active" &&
						blueprint.Type == "general" &&
						blueprint.UserID == "user-123"
				})).Return(nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "spec-library-123", blueprint.ID)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", blueprint.ProjectID)
				assert.Equal(t, "test-spec-library", blueprint.Slug)
				assert.Equal(t, "Test Blueprint", blueprint.Title)
				assert.Equal(t, "Test content", blueprint.Content)
				assert.Equal(t, "active", blueprint.Status)
				assert.Equal(t, "general", blueprint.Type)
				assert.Equal(t, "user-123", blueprint.UserID)
				assert.NotNil(t, blueprint.Metadata)
			},
		},
		{
			name:   "Successful creation with custom values",
			userID: "user-123",
			request: &models.CreateBlueprintRequest{
				ProjectID:   "custom-project",
				Slug:        "custom-spec-library",
				Title:       "Custom Blueprint",
				Content:     "Custom content",
				Description: "Custom description",
				Type:        "general",
				Status:      "expired",
				Metadata:    map[string]interface{}{"custom": "value"},
			},
			setup: func(repo *MockBlueprintRepository) {
				repo.On("Create", mock.Anything, mock.MatchedBy(func(blueprint *models.Blueprint) bool {
					return blueprint.ProjectID == "custom-project" &&
						blueprint.Type == "general" &&
						blueprint.Status == "expired"
				})).Return(nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "custom-project", blueprint.ProjectID)
				assert.Equal(t, "general", blueprint.Type)
				assert.Equal(t, "expired", blueprint.Status)
				assert.Equal(t, "Custom description", blueprint.Description)
				assert.Equal(t, "value", blueprint.Metadata["custom"])
			},
		},
		{
			name:   "Repository error",
			userID: "user-123",
			request: &models.CreateBlueprintRequest{
				Slug:    "test-spec-library",
				Title:   "Test Blueprint",
				Content: "Test content",
			},
			setup: func(repo *MockBlueprintRepository) {
				repo.On("Create", mock.Anything, mock.Anything).Return(assert.AnError)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.Error(t, err)
				assert.Nil(t, blueprint)
				assert.Equal(t, assert.AnError, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockBlueprintRepository{}
			mockResourceUsageSvc := &MockResourceUsageService{}

			// Setup repository expectations
			tt.setup(repo)

			// Note: TrackResourceCreation was removed as part of resource tracking simplification
			// Resource limits are now checked directly in handlers via CheckResourceLimit

			service := NewBlueprintService(
				repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
				nil)
			blueprint, err := service.CreateBlueprint(tt.userID, "team-123", tt.request)

			tt.expected(t, blueprint, err)
			repo.AssertExpectations(t)
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestBlueprintService_GetBlueprintByProjectIDAndSlug(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		projectName string
		slug        string
		setup       func(*MockBlueprintRepository)
		expected    func(*testing.T, *models.Blueprint, error)
	}{
		{
			name:        "Successful retrieval",
			userID:      "user-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "test-slug",
			setup: func(repo *MockBlueprintRepository) {
				blueprint := &models.Blueprint{
					ID:        "spec-library-123",
					ProjectID: "550e8400-e29b-41d4-a716-446655440000",
					Slug:      "test-slug",
					UserID:    "user-123",
					Title:     "Test Blueprint",
					Content:   "Test content",
				}
				repo.On(
					"GetByProjectIDAndSlugCrossTeam",
					mock.Anything,
					"user-123",
					"550e8400-e29b-41d4-a716-446655440000",
					"test-slug",
				).Return(blueprint, nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "spec-library-123", blueprint.ID)
				assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", blueprint.ProjectID)
				assert.Equal(t, "test-slug", blueprint.Slug)
			},
		},
		{
			name:        "Blueprint not found",
			userID:      "user-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "non-existent",
			setup: func(repo *MockBlueprintRepository) {
				repo.On(
					"GetByProjectIDAndSlugCrossTeam",
					mock.Anything, "user-123",
					"550e8400-e29b-41d4-a716-446655440000", "non-existent",
				).Return(nil, assert.AnError)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.Error(t, err)
				assert.Nil(t, blueprint)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockBlueprintRepository{}
			tt.setup(repo)

			mockResourceUsageSvc := &MockResourceUsageService{}
			service := NewBlueprintService(
				repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
				nil)
			blueprint, err := service.GetBlueprintByProjectIDAndSlug(tt.userID, tt.projectName, tt.slug)

			tt.expected(t, blueprint, err)
			repo.AssertExpectations(t)
			mockResourceUsageSvc.AssertExpectations(t)
		})
	}
}

func TestBlueprintService_GetBlueprintByIDInTeam(t *testing.T) {
	const (
		userID      = "user-123"
		teamID      = "550e8400-e29b-41d4-a716-446655440000"
		blueprintID = "770e8400-e29b-41d4-a716-446655440000"
	)
	found := &models.Blueprint{ID: blueprintID, TeamID: teamID}

	tests := []struct {
		name    string
		repoRet *models.Blueprint
		repoErr error
		wantErr error // nil means success
	}{
		{"Successful retrieval", found, nil, nil},
		{"Not found propagates sentinel", nil, repositories.ErrBlueprintNotFound, repositories.ErrBlueprintNotFound},
		{"Real error propagates", nil, assert.AnError, assert.AnError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockBlueprintRepository{}
			repo.On("GetByID", mock.Anything, userID, teamID, blueprintID).
				Return(tt.repoRet, tt.repoErr)

			service := NewBlueprintService(
				repo, nil, allowAllAuthz{}, nil, &MockResourceUsageService{},
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
				nil)
			got, err := service.GetBlueprintByIDInTeam(userID, teamID, blueprintID)

			if tt.wantErr != nil {
				assert.Nil(t, got)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, blueprintID, got.ID)
			}
			repo.AssertExpectations(t)
		})
	}
}

//nolint:gocyclo,funlen // Test function with multiple test cases requires higher complexity and length
func TestBlueprintService_ListSpecLibraries(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		filters  BlueprintFilters
		setup    func(*MockBlueprintRepository)
		expected func(*testing.T, *models.BlueprintListResponse, error)
	}{
		{
			name:   "Successful list with filters",
			userID: "user-123",
			filters: BlueprintFilters{
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
			setup: func(repo *MockBlueprintRepository) {
				specLibraries := []models.Blueprint{
					{
						ID:    "spec-library-1",
						Title: "Test Spec Library 1",
					},
					{
						ID:    "spec-library-2",
						Title: "Test Spec Library 2",
					},
				}
				repo.On("List", mock.Anything, "user-123", mock.MatchedBy(func(filters repositories.BlueprintFilters) bool {
					return filters.ProjectID != nil && *filters.ProjectID == "550e8400-e29b-41d4-a716-446655440000" &&
						*filters.Status == "active" &&
						*filters.Type == "general" &&
						filters.Search == "test" &&
						filters.SortBy == "created_at" &&
						filters.SortOrder == "desc" &&
						filters.Metadata["key"] == "value" &&
						filters.Page == 1 &&
						filters.Limit == 20
				})).Return(specLibraries, 2, nil)
			},
			expected: func(t *testing.T, response *models.BlueprintListResponse, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Len(t, response.Blueprints, 2)
				assert.Equal(t, 2, response.TotalCount)
				assert.Equal(t, 1, response.Page)
				assert.Equal(t, 20, response.PerPage)
				assert.Equal(t, 1, response.TotalPages)
			},
		},
		{
			name:   "Empty filters with defaults",
			userID: "user-123",
			filters: BlueprintFilters{
				Page:  1,
				Limit: 10,
			},
			setup: func(repo *MockBlueprintRepository) {
				repo.On("List", mock.Anything, "user-123", mock.MatchedBy(func(filters repositories.BlueprintFilters) bool {
					return filters.ProjectID == nil &&
						filters.Status == nil &&
						filters.Type == nil &&
						filters.Search == "" &&
						filters.Page == 1 &&
						filters.Limit == 10
				})).Return([]models.Blueprint{}, 0, nil)
			},
			expected: func(t *testing.T, response *models.BlueprintListResponse, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Empty(t, response.Blueprints)
				assert.Zero(t, response.TotalCount)
			},
		},
		{
			name:    "Repository error",
			userID:  "user-123",
			filters: BlueprintFilters{Page: 1, Limit: 10},
			setup: func(repo *MockBlueprintRepository) {
				repo.On("List", mock.Anything, "user-123", mock.Anything).Return([]models.Blueprint{}, 0, assert.AnError)
			},
			expected: func(t *testing.T, response *models.BlueprintListResponse, err error) {
				assert.Error(t, err)
				assert.Nil(t, response)
			},
		},
		{
			name:   "List with pagination calculation",
			userID: "user-123",
			filters: BlueprintFilters{
				Page:  2,
				Limit: 10,
			},
			setup: func(repo *MockBlueprintRepository) {
				specLibraries := []models.Blueprint{
					{ID: "spec-library-11", Title: "Blueprint 11"},
					{ID: "spec-library-12", Title: "Blueprint 12"},
				}
				repo.On("List", mock.Anything, "user-123", mock.Anything).Return(specLibraries, 25, nil)
			},
			expected: func(t *testing.T, response *models.BlueprintListResponse, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Equal(t, 25, response.TotalCount)
				assert.Equal(t, 2, response.Page)
				assert.Equal(t, 10, response.PerPage)
				assert.Equal(t, 3, response.TotalPages)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockBlueprintRepository{}
			tt.setup(repo)

			mockResourceUsageSvc := &MockResourceUsageService{}
			service := NewBlueprintService(
				repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
				nil)
			response, err := service.ListBlueprints(tt.userID, tt.filters)

			tt.expected(t, response, err)
			repo.AssertExpectations(t)
		})
	}
}

func TestBlueprintService_ListSpecLibrariesByProject(t *testing.T) {
	repo := &MockBlueprintRepository{}
	specLibraries := []models.Blueprint{
		{ID: "spec-library-1", ProjectID: "550e8400-e29b-41d4-a716-446655440000"},
	}
	repo.On("List", mock.Anything, "user-123", mock.MatchedBy(func(filters repositories.BlueprintFilters) bool {
		return filters.ProjectID != nil && *filters.ProjectID == "550e8400-e29b-41d4-a716-446655440000"
	})).Return(specLibraries, 1, nil)

	mockResourceUsageSvc := &MockResourceUsageService{}
	service := NewBlueprintService(
		repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
		func() *slog.Logger { l, _ := logtest.New(); return l }(),
		nil,
		nil)
	response, err := service.ListBlueprintsByProject(
		"user-123",
		"550e8400-e29b-41d4-a716-446655440000",
		BlueprintFilters{Page: 1, Limit: 10},
	)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Blueprints, 1)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", response.Blueprints[0].ProjectID)
	repo.AssertExpectations(t)
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestBlueprintService_UpdateBlueprintByProjectIDAndSlug(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		projectName string
		slug        string
		request     *models.UpdateBlueprintRequest
		setup       func(*MockBlueprintRepository)
		expected    func(*testing.T, *models.Blueprint, error)
	}{
		{
			name:        "Successful update with multiple fields",
			userID:      "user-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "test-slug",
			request: &models.UpdateBlueprintRequest{
				Title:       stringPtr("Updated Title"),
				Description: stringPtr("Updated Description"),
				Content:     stringPtr("Updated Content"),
				Status:      stringPtr("expired"),
				Type:        stringPtr("general"),
				Metadata:    map[string]interface{}{"updated": "value"},
			},
			setup: func(repo *MockBlueprintRepository) {
				existingBlueprint := &models.Blueprint{
					ID:          "spec-library-123",
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
					mock.Anything, "user-123",
					"550e8400-e29b-41d4-a716-446655440000", "test-slug",
				).Return(existingBlueprint, nil)
				repo.On("Update", mock.Anything, mock.MatchedBy(func(blueprint *models.Blueprint) bool {
					return blueprint.Title == "Updated Title" &&
						blueprint.Description == "Updated Description" &&
						blueprint.Content == "Updated Content" &&
						blueprint.Status == "expired" &&
						blueprint.Type == "general" &&
						blueprint.Metadata["updated"] == "value"
				})).Return(nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "Updated Title", blueprint.Title)
				assert.Equal(t, "Updated Description", blueprint.Description)
				assert.Equal(t, "Updated Content", blueprint.Content)
				assert.Equal(t, "expired", blueprint.Status)
				assert.Equal(t, "general", blueprint.Type)
				assert.Equal(t, "value", blueprint.Metadata["updated"])
			},
		},
		{
			name:        "Update only title",
			userID:      "user-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "test-slug",
			request: &models.UpdateBlueprintRequest{
				Title: stringPtr("New Title Only"),
			},
			setup: func(repo *MockBlueprintRepository) {
				existingBlueprint := &models.Blueprint{
					ID:          "spec-library-123",
					ProjectID:   "550e8400-e29b-41d4-a716-446655440000",
					Slug:        "test-slug",
					UserID:      "user-123",
					Title:       "Original Title",
					Description: "Original Description",
					Content:     "Original Content",
					Status:      "active",
					Type:        "general",
				}
				repo.On(
					"GetByProjectIDAndSlugCrossTeam",
					mock.Anything, "user-123",
					"550e8400-e29b-41d4-a716-446655440000", "test-slug",
				).Return(existingBlueprint, nil)
				repo.On("Update", mock.Anything, mock.MatchedBy(func(blueprint *models.Blueprint) bool {
					return blueprint.Title == "New Title Only" &&
						blueprint.Description == "Original Description" &&
						blueprint.Content == "Original Content"
				})).Return(nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "New Title Only", blueprint.Title)
				assert.Equal(t, "Original Description", blueprint.Description)
			},
		},
		{
			name:        "Update project name and slug",
			userID:      "user-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "test-slug",
			request: &models.UpdateBlueprintRequest{
				ProjectID: stringPtr("new-project"),
				Slug:      stringPtr("new-slug"),
			},
			setup: func(repo *MockBlueprintRepository) {
				existingBlueprint := &models.Blueprint{
					ID:        "spec-library-123",
					ProjectID: "550e8400-e29b-41d4-a716-446655440000",
					Slug:      "test-slug",
					UserID:    "user-123",
					Title:     "Test Title",
				}
				repo.On(
					"GetByProjectIDAndSlugCrossTeam",
					mock.Anything, "user-123",
					"550e8400-e29b-41d4-a716-446655440000", "test-slug",
				).Return(existingBlueprint, nil)
				repo.On("Update", mock.Anything, mock.MatchedBy(func(blueprint *models.Blueprint) bool {
					return blueprint.ProjectID == "new-project" && blueprint.Slug == "new-slug"
				})).Return(nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "new-project", blueprint.ProjectID)
				assert.Equal(t, "new-slug", blueprint.Slug)
			},
		},
		{
			name:        "Blueprint not found",
			userID:      "user-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "non-existent",
			request:     &models.UpdateBlueprintRequest{},
			setup: func(repo *MockBlueprintRepository) {
				repo.On(
					"GetByProjectIDAndSlugCrossTeam",
					mock.Anything, "user-123",
					"550e8400-e29b-41d4-a716-446655440000", "non-existent",
				).Return(nil, assert.AnError)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.Error(t, err)
				assert.Nil(t, blueprint)
			},
		},
		{
			name:        "Update error",
			userID:      "user-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "test-slug",
			request: &models.UpdateBlueprintRequest{
				Title: stringPtr("Updated Title"),
			},
			setup: func(repo *MockBlueprintRepository) {
				existingBlueprint := &models.Blueprint{
					ID:        "spec-library-123",
					ProjectID: "550e8400-e29b-41d4-a716-446655440000",
					Slug:      "test-slug",
					UserID:    "user-123",
					Title:     "Original Title",
				}
				repo.On(
					"GetByProjectIDAndSlugCrossTeam",
					mock.Anything, "user-123",
					"550e8400-e29b-41d4-a716-446655440000", "test-slug",
				).Return(existingBlueprint, nil)
				repo.On("Update", mock.Anything, mock.Anything).Return(assert.AnError)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.Error(t, err)
				assert.Nil(t, blueprint)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockBlueprintRepository{}
			tt.setup(repo)

			mockResourceUsageSvc := &MockResourceUsageService{}
			service := NewBlueprintService(
				repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
				nil)
			blueprint, err := service.UpdateBlueprintByProjectIDAndSlug(tt.userID, tt.projectName, tt.slug, tt.request)

			tt.expected(t, blueprint, err)
			repo.AssertExpectations(t)
			mockResourceUsageSvc.AssertExpectations(t)
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestBlueprintService_DeleteBlueprintByProjectIDAndSlug(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		teamID      string
		projectName string
		slug        string
		setup       func(*MockBlueprintRepository)
		expected    func(*testing.T, error)
	}{
		{
			name:        "Successful deletion",
			userID:      "user-123",
			teamID:      "team-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "test-slug",
			setup: func(repo *MockBlueprintRepository) {
				blueprint := &models.Blueprint{
					ID:        "spec-library-123",
					ProjectID: "550e8400-e29b-41d4-a716-446655440000",
					Slug:      "test-slug",
					UserID:    "user-123",
					TeamID:    "team-123",
				}
				repo.On(
					"GetByProjectIDAndSlug",
					mock.Anything,
					"user-123",
					"team-123",
					"550e8400-e29b-41d4-a716-446655440000",
					"test-slug",
				).Return(blueprint, nil)
				repo.On("Delete", mock.Anything, "user-123", "team-123", "spec-library-123").Return(nil)
			},
			expected: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:        "Blueprint not found",
			userID:      "user-123",
			teamID:      "team-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "non-existent",
			setup: func(repo *MockBlueprintRepository) {
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
			name:        "Repository error during deletion",
			userID:      "user-123",
			teamID:      "team-123",
			projectName: "550e8400-e29b-41d4-a716-446655440000",
			slug:        "test-slug",
			setup: func(repo *MockBlueprintRepository) {
				blueprint := &models.Blueprint{
					ID:        "spec-library-123",
					ProjectID: "550e8400-e29b-41d4-a716-446655440000",
					Slug:      "test-slug",
					UserID:    "user-123",
					TeamID:    "team-123",
				}
				repo.On(
					"GetByProjectIDAndSlug",
					mock.Anything,
					"user-123",
					"team-123",
					"550e8400-e29b-41d4-a716-446655440000",
					"test-slug",
				).Return(blueprint, nil)
				repo.On("Delete", mock.Anything, "user-123", "team-123", "spec-library-123").Return(assert.AnError)
			},
			expected: func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockBlueprintRepository{}
			mockResourceUsageSvc := &MockResourceUsageService{}

			// Setup repository expectations
			tt.setup(repo)

			// Note: TrackResourceDeletion was removed as part of resource tracking simplification
			// Deletions are never blocked by resource limits

			service := NewBlueprintService(
				repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
				nil)
			err := service.DeleteBlueprintByProjectIDAndSlug(tt.userID, tt.teamID, tt.projectName, tt.slug)

			tt.expected(t, err)
			repo.AssertExpectations(t)
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestBlueprintService_GetBlueprintStats(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		setup    func(*MockBlueprintRepository)
		expected func(*testing.T, *models.BlueprintStatsResponse, error)
	}{
		{
			name:   "Successful stats retrieval",
			userID: "user-123",
			setup: func(repo *MockBlueprintRepository) {
				stats := &models.BlueprintStatsResponse{
					TotalProjects:   3,
					TotalBlueprints: 10,
					AddedThisWeek:   2,
					TotalByType: map[string]int{
						"general": 10,
					},
					TotalByStatus: map[string]int{
						"active":  8,
						"expired": 2,
					},
				}
				repo.On("GetStats", mock.Anything, "user-123").Return(stats, nil)
			},
			expected: func(t *testing.T, stats *models.BlueprintStatsResponse, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, stats)
				assert.Equal(t, 3, stats.TotalProjects)
				assert.Equal(t, 10, stats.TotalBlueprints)
				assert.Equal(t, 2, stats.AddedThisWeek)
				assert.Equal(t, 10, stats.TotalByType["general"])
				assert.Equal(t, 8, stats.TotalByStatus["active"])
			},
		},
		{
			name:   "Empty stats",
			userID: "user-123",
			setup: func(repo *MockBlueprintRepository) {
				stats := &models.BlueprintStatsResponse{
					TotalProjects:   0,
					TotalBlueprints: 0,
					AddedThisWeek:   0,
					TotalByType:     map[string]int{},
					TotalByStatus:   map[string]int{},
				}
				repo.On("GetStats", mock.Anything, "user-123").Return(stats, nil)
			},
			expected: func(t *testing.T, stats *models.BlueprintStatsResponse, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, stats)
				assert.Zero(t, stats.TotalProjects)
				assert.Zero(t, stats.TotalBlueprints)
				assert.Zero(t, stats.AddedThisWeek)
			},
		},
		{
			name:   "Repository error",
			userID: "user-123",
			setup: func(repo *MockBlueprintRepository) {
				repo.On("GetStats", mock.Anything, "user-123").Return(nil, assert.AnError)
			},
			expected: func(t *testing.T, stats *models.BlueprintStatsResponse, err error) {
				assert.Error(t, err)
				assert.Nil(t, stats)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockBlueprintRepository{}
			tt.setup(repo)

			mockResourceUsageSvc := &MockResourceUsageService{}
			service := NewBlueprintService(
				repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
				nil)
			stats, err := service.GetBlueprintStats(tt.userID)

			tt.expected(t, stats, err)
			repo.AssertExpectations(t)
		})
	}
}

func TestBlueprintService_ImplementsInterface(t *testing.T) {
	repo := &MockBlueprintRepository{}
	mockResourceUsageSvc := &MockResourceUsageService{}
	service := NewBlueprintService(
		repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
		func() *slog.Logger { l, _ := logtest.New(); return l }(),
		nil,
		nil)

	// Verify that BlueprintService implements BlueprintServiceInterface
	var _ BlueprintServiceInterface = service
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestBlueprintService_PublishesBlueprintEvents(t *testing.T) {
	tests := []struct {
		name             string
		setupMocks       func(*MockBlueprintRepository, *event_mocks.MockEventPublisher)
		executeAction    func(*BlueprintService) error
		expectEventCalls int
		eventType        string
	}{
		{
			name: "publishes spec_library.created event when creating blueprint",
			setupMocks: func(mockRepo *MockBlueprintRepository, mockEventManager *event_mocks.MockEventPublisher) {
				mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Blueprint")).
					Return(nil).Run(func(args mock.Arguments) {
					blueprint := args.Get(1).(*models.Blueprint)
					blueprint.ID = "spec-library-new-123"
				})

				// Expect event to be published exactly once
				mockEventManager.On("Publish", mock.Anything, mock.MatchedBy(func(event events.Event) bool {
					return event.Type() == events.EventTypeBlueprintCreated
				})).Return(nil).Once()
			},
			executeAction: func(service *BlueprintService) error {
				req := &models.CreateBlueprintRequest{
					ProjectID:   "550e8400-e29b-41d4-a716-446655440000",
					Slug:        "test-spec-library",
					Title:       "Test Blueprint",
					Description: "Test Description",
					Content:     "Test Content",
					Type:        "general",
					Status:      "active",
				}
				_, err := service.CreateBlueprint("user-123", "team-123", req)
				return err
			},
			expectEventCalls: 1,
			eventType:        events.EventTypeBlueprintCreated,
		},
		{
			name: "publishes spec_library.updated event when updating blueprint",
			setupMocks: func(mockRepo *MockBlueprintRepository, mockEventManager *event_mocks.MockEventPublisher) {
				existingBlueprint := &models.Blueprint{
					ID:        "spec-library-123",
					ProjectID: "550e8400-e29b-41d4-a716-446655440000",
					Slug:      "test-spec-library",
					Title:     "Original Title",
					UserID:    "user-123",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				mockRepo.On(
					"GetByProjectIDAndSlugCrossTeam",
					mock.Anything,
					"user-123",
					"550e8400-e29b-41d4-a716-446655440000",
					"test-spec-library",
				).Return(existingBlueprint, nil)

				mockRepo.On("Update", mock.Anything, mock.AnythingOfType("*models.Blueprint")).
					Return(nil)

				// Expect event to be published exactly once
				mockEventManager.On("Publish", mock.Anything, mock.MatchedBy(func(event events.Event) bool {
					return event.Type() == events.EventTypeBlueprintUpdated
				})).Return(nil).Once()
			},
			executeAction: func(service *BlueprintService) error {
				title := "Updated Title"
				req := &models.UpdateBlueprintRequest{
					Title: &title,
				}
				_, err := service.UpdateBlueprintByProjectIDAndSlug(
					"user-123",
					"550e8400-e29b-41d4-a716-446655440000",
					"test-spec-library",
					req,
				)
				return err
			},
			expectEventCalls: 1,
			eventType:        events.EventTypeBlueprintUpdated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockBlueprintRepository{}
			mockEventManager := &event_mocks.MockEventPublisher{}
			mockResourceUsageSvc := &MockResourceUsageService{}

			// Setup mocks
			tt.setupMocks(mockRepo, mockEventManager)

			// Setup resource usage tracking expectations
			mockResourceUsageSvc.On(
				"TrackResourceCreation", mock.Anything, mock.Anything,
				events.ResourceTypeBlueprint, mock.Anything,
			).Return(nil).Maybe()

			service := NewBlueprintService(
				mockRepo, nil, allowAllAuthz{}, mockEventManager, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
				nil)

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

// TestBlueprintService_UpdateBlueprint_PreservesTeamID tests that team_id is preserved during update
func TestBlueprintService_UpdateBlueprint_PreservesTeamID(t *testing.T) {
	mockRepo := &MockBlueprintRepository{}
	mockResourceUsageSvc := &MockResourceUsageService{}
	service := NewBlueprintService(
		mockRepo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
		func() *slog.Logger { l, _ := logtest.New(); return l }(),
		nil,
		nil)

	// Create existing blueprint with team_id
	existingBlueprint := &models.Blueprint{
		ID:        "spec-library-123",
		ProjectID: "project-789",
		Slug:      "test-spec",
		Title:     "Original Title",
		UserID:    "user-123",
		TeamID:    "team-888",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	mockRepo.On(
		"GetByProjectIDAndSlugCrossTeam",
		mock.Anything,
		"user-123",
		"project-789",
		"test-spec",
	).Return(existingBlueprint, nil)

	// Verify that Update is called with team_id preserved
	mockRepo.On("Update", mock.Anything,
		mock.MatchedBy(func(blueprint *models.Blueprint) bool {
			return blueprint.ID == "spec-library-123" &&
				blueprint.TeamID == "team-888" && // TeamID must be preserved
				blueprint.Title == "Updated Title"
		})).Return(nil)

	title := "Updated Title"
	request := &models.UpdateBlueprintRequest{
		Title: &title,
	}

	blueprint, err := service.UpdateBlueprintByProjectIDAndSlug("user-123", "project-789", "test-spec", request)

	assert.NoError(t, err)
	assert.NotNil(t, blueprint)
	assert.Equal(t, "team-888", blueprint.TeamID, "TeamID should be preserved during update")
	mockRepo.AssertExpectations(t)
}

// TestBlueprintService_CreateBlueprintWithNewTypes tests creating blueprints with new types
//
//nolint:funlen,gocyclo // Test function requires comprehensive setup and assertions
func TestBlueprintService_CreateBlueprintWithNewTypes(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		request  *models.CreateBlueprintRequest
		setup    func(*MockBlueprintRepository)
		expected func(*testing.T, *models.Blueprint, error)
	}{
		{
			name:   "Create claude type blueprint with claude-md subtype",
			userID: "user-123",
			request: &models.CreateBlueprintRequest{
				ProjectID: "550e8400-e29b-41d4-a716-446655440000",
				Slug:      "claude-md-spec",
				Title:     "Claude.md Configuration",
				Content:   "# Claude.md content here",
				Type:      "claude",
				Subtype:   func() *string { s := "claude-md"; return &s }(),
			},
			setup: func(repo *MockBlueprintRepository) {
				repo.On("Create", mock.Anything, mock.MatchedBy(func(blueprint *models.Blueprint) bool {
					return blueprint.Type == "claude" &&
						blueprint.Subtype != nil &&
						*blueprint.Subtype == "claude-md"
				})).Return(nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "claude", blueprint.Type)
				assert.NotNil(t, blueprint.Subtype)
				assert.Equal(t, "claude-md", *blueprint.Subtype)
			},
		},
		{
			name:   "Create cursor type blueprint with skills subtype",
			userID: "user-123",
			request: &models.CreateBlueprintRequest{
				ProjectID: "550e8400-e29b-41d4-a716-446655440000",
				Slug:      "cursor-skills-spec",
				Title:     "Cursor Skills",
				Content:   "Cursor skills content",
				Type:      "cursor",
				Subtype:   func() *string { s := "skills"; return &s }(),
			},
			setup: func(repo *MockBlueprintRepository) {
				repo.On("Create", mock.Anything, mock.MatchedBy(func(blueprint *models.Blueprint) bool {
					return blueprint.Type == "cursor" &&
						blueprint.Subtype != nil &&
						*blueprint.Subtype == "skills"
				})).Return(nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "cursor", blueprint.Type)
				assert.NotNil(t, blueprint.Subtype)
				assert.Equal(t, "skills", *blueprint.Subtype)
			},
		},
		{
			name:   "Create cursor type blueprint with agents subtype",
			userID: "user-123",
			request: &models.CreateBlueprintRequest{
				ProjectID: "550e8400-e29b-41d4-a716-446655440000",
				Slug:      "cursor-agents-spec",
				Title:     "Cursor Agents",
				Content:   "Cursor agents content",
				Type:      "cursor",
				Subtype:   func() *string { s := "agents"; return &s }(),
			},
			setup: func(repo *MockBlueprintRepository) {
				repo.On("Create", mock.Anything, mock.MatchedBy(func(blueprint *models.Blueprint) bool {
					return blueprint.Type == "cursor" &&
						blueprint.Subtype != nil &&
						*blueprint.Subtype == "agents"
				})).Return(nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "cursor", blueprint.Type)
				assert.NotNil(t, blueprint.Subtype)
				assert.Equal(t, "agents", *blueprint.Subtype)
			},
		},
		{
			name:   "Create cursor type blueprint with commands subtype",
			userID: "user-123",
			request: &models.CreateBlueprintRequest{
				ProjectID: "550e8400-e29b-41d4-a716-446655440000",
				Slug:      "cursor-commands-spec",
				Title:     "Cursor Commands",
				Content:   "Cursor commands content",
				Type:      "cursor",
				Subtype:   func() *string { s := "commands"; return &s }(),
			},
			setup: func(repo *MockBlueprintRepository) {
				repo.On("Create", mock.Anything, mock.MatchedBy(func(blueprint *models.Blueprint) bool {
					return blueprint.Type == "cursor" &&
						blueprint.Subtype != nil &&
						*blueprint.Subtype == "commands"
				})).Return(nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "cursor", blueprint.Type)
				assert.NotNil(t, blueprint.Subtype)
				assert.Equal(t, "commands", *blueprint.Subtype)
			},
		},
		{
			name:   "Create cursor type blueprint with rules subtype",
			userID: "user-123",
			request: &models.CreateBlueprintRequest{
				ProjectID: "550e8400-e29b-41d4-a716-446655440000",
				Slug:      "cursor-rules-spec",
				Title:     "Cursor Rules",
				Content:   "Cursor rules content",
				Type:      "cursor",
				Subtype:   func() *string { s := "rules"; return &s }(),
			},
			setup: func(repo *MockBlueprintRepository) {
				repo.On("Create", mock.Anything, mock.MatchedBy(func(blueprint *models.Blueprint) bool {
					return blueprint.Type == "cursor" &&
						blueprint.Subtype != nil &&
						*blueprint.Subtype == "rules"
				})).Return(nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "cursor", blueprint.Type)
				assert.NotNil(t, blueprint.Subtype)
				assert.Equal(t, "rules", *blueprint.Subtype)
			},
		},
		{
			name:   "Create cursor type blueprint with cursor-md subtype",
			userID: "user-123",
			request: &models.CreateBlueprintRequest{
				ProjectID: "550e8400-e29b-41d4-a716-446655440000",
				Slug:      "cursor-md-spec",
				Title:     "Cursor.md Configuration",
				Content:   "# Cursor.md content here",
				Type:      "cursor",
				Subtype:   func() *string { s := "cursor-md"; return &s }(),
			},
			setup: func(repo *MockBlueprintRepository) {
				repo.On("Create", mock.Anything, mock.MatchedBy(func(blueprint *models.Blueprint) bool {
					return blueprint.Type == "cursor" &&
						blueprint.Subtype != nil &&
						*blueprint.Subtype == "cursor-md"
				})).Return(nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "cursor", blueprint.Type)
				assert.NotNil(t, blueprint.Subtype)
				assert.Equal(t, "cursor-md", *blueprint.Subtype)
			},
		},
		{
			name:   "Create codex type blueprint with rules subtype",
			userID: "user-123",
			request: &models.CreateBlueprintRequest{
				ProjectID: "550e8400-e29b-41d4-a716-446655440000",
				Slug:      "codex-rules-spec",
				Title:     "Codex Rules",
				Content:   "Codex rules content",
				Type:      "codex",
				Subtype:   func() *string { s := "rules"; return &s }(),
			},
			setup: func(repo *MockBlueprintRepository) {
				repo.On("Create", mock.Anything, mock.MatchedBy(func(blueprint *models.Blueprint) bool {
					return blueprint.Type == "codex" &&
						blueprint.Subtype != nil &&
						*blueprint.Subtype == "rules"
				})).Return(nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "codex", blueprint.Type)
				assert.NotNil(t, blueprint.Subtype)
				assert.Equal(t, "rules", *blueprint.Subtype)
			},
		},
		{
			name:   "Create codex type blueprint with skills subtype",
			userID: "user-123",
			request: &models.CreateBlueprintRequest{
				ProjectID: "550e8400-e29b-41d4-a716-446655440000",
				Slug:      "codex-skills-spec",
				Title:     "Codex Skills",
				Content:   "Codex skills content",
				Type:      "codex",
				Subtype:   func() *string { s := "skills"; return &s }(),
			},
			setup: func(repo *MockBlueprintRepository) {
				repo.On("Create", mock.Anything, mock.MatchedBy(func(blueprint *models.Blueprint) bool {
					return blueprint.Type == "codex" &&
						blueprint.Subtype != nil &&
						*blueprint.Subtype == "skills"
				})).Return(nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "codex", blueprint.Type)
				assert.NotNil(t, blueprint.Subtype)
				assert.Equal(t, "skills", *blueprint.Subtype)
			},
		},
		{
			name:   "Create codex type blueprint with agents-md subtype",
			userID: "user-123",
			request: &models.CreateBlueprintRequest{
				ProjectID: "550e8400-e29b-41d4-a716-446655440000",
				Slug:      "codex-agents-md-spec",
				Title:     "Codex AGENTS.md",
				Content:   "# Codex AGENTS.md content here",
				Type:      "codex",
				Subtype:   func() *string { s := "agents-md"; return &s }(),
			},
			setup: func(repo *MockBlueprintRepository) {
				repo.On("Create", mock.Anything, mock.MatchedBy(func(blueprint *models.Blueprint) bool {
					return blueprint.Type == "codex" &&
						blueprint.Subtype != nil &&
						*blueprint.Subtype == "agents-md"
				})).Return(nil)
			},
			expected: func(t *testing.T, blueprint *models.Blueprint, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, blueprint)
				assert.Equal(t, "codex", blueprint.Type)
				assert.NotNil(t, blueprint.Subtype)
				assert.Equal(t, "agents-md", *blueprint.Subtype)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockBlueprintRepository{}
			mockResourceUsageSvc := &MockResourceUsageService{}

			tt.setup(repo)

			service := NewBlueprintService(
				repo, nil, allowAllAuthz{}, nil, mockResourceUsageSvc,
				func() *slog.Logger { l, _ := logtest.New(); return l }(),
				nil,
				nil)
			blueprint, err := service.CreateBlueprint(tt.userID, "team-123", tt.request)

			tt.expected(t, blueprint, err)
			repo.AssertExpectations(t)
		})
	}
}
