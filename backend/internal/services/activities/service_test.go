package activities

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

func setupTestService(_t *testing.T) (*Service, *mocks.ActivityRepositoryMock) {
	mockRepo := &mocks.ActivityRepositoryMock{}
	service := NewService(ServiceDeps{
		Repo:          mockRepo,
		ProjectRepo:   nil,
		PromptRepo:    nil,
		ArtifactRepo:  nil,
		UserRepo:      nil,
		AgentRepo:     nil,
		BlueprintRepo: nil,
		APIKeyRepo:    nil,
		MemoryRepo:    nil,
		RetentionDays: 90,
	})
	return service, mockRepo
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestService_RecordActivity(t *testing.T) {
	service, mockRepo := setupTestService(t)
	ctx := context.Background()
	userID := uuid.New().String()

	tests := []struct {
		name    string
		req     CreateActivityRequest
		setup   func()
		wantErr bool
	}{
		{
			name: "successful activity creation",
			req: CreateActivityRequest{
				ActivityType: ActivityTypeAuthLogin,
				EntityType:   EntityTypeUser,
				Description:  "User logged in successfully",
				Metadata: map[string]interface{}{
					"method": "oauth",
				},
			},
			setup: func() {
				mockRepo.On("Create", ctx, mock.MatchedBy(func(activity *models.Activity) bool {
					return activity.UserID == userID &&
						activity.ActivityType == ActivityTypeAuthLogin &&
						activity.EntityType == EntityTypeUser &&
						activity.Description == "User logged in successfully"
				})).Return(nil).Once()
			},
			wantErr: false,
		},
		{
			name: "repository error",
			req: CreateActivityRequest{
				ActivityType: ActivityTypeAuthLogin,
				EntityType:   EntityTypeUser,
				Description:  "User logged in successfully",
			},
			setup: func() {
				mockRepo.On("Create", ctx, mock.Anything).Return(assert.AnError).Once()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			activity, err := service.RecordActivity(ctx, userID, tt.req)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, activity)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, activity)
				assert.Equal(t, userID, activity.UserID)
				assert.Equal(t, tt.req.ActivityType, activity.ActivityType)
				assert.Equal(t, tt.req.EntityType, activity.EntityType)
				assert.Equal(t, tt.req.Description, activity.Description)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestService_RecordAuthActivity(t *testing.T) {
	service, mockRepo := setupTestService(t)
	ctx := context.Background()
	userID := uuid.New().String()

	tests := []struct {
		name         string
		activityType string
		sessionID    *string
		metadata     map[string]interface{}
		sourceIP     *string
		userAgent    *string
		setup        func()
		wantErr      bool
	}{
		{
			name:         "successful auth login activity",
			activityType: ActivityTypeAuthLogin,
			sessionID:    stringPtr("session-123"),
			metadata: map[string]interface{}{
				"provider": "google",
			},
			sourceIP:  stringPtr("192.168.1.1"),
			userAgent: stringPtr("Mozilla/5.0"),
			setup: func() {
				mockRepo.On("Create", ctx, mock.MatchedBy(func(activity *models.Activity) bool {
					return activity.UserID == userID &&
						activity.ActivityType == ActivityTypeAuthLogin &&
						activity.EntityType == EntityTypeUser
				})).Return(nil).Once()
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			err := service.RecordAuthActivity(ctx, userID, tt.activityType, tt.sessionID, tt.metadata, tt.sourceIP, tt.userAgent)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestService_GetActivityByID(t *testing.T) {
	service, mockRepo := setupTestService(t)
	ctx := context.Background()
	userID := uuid.New().String()
	activityID := uuid.New().String()

	tests := []struct {
		name    string
		setup   func()
		wantErr bool
	}{
		{
			name: "successful get activity",
			setup: func() {
				expectedActivity := &models.Activity{
					ID:           activityID,
					UserID:       userID,
					ActivityType: ActivityTypeAuthLogin,
					EntityType:   EntityTypeUser,
					Description:  "User logged in",
				}
				mockRepo.On("GetByID", ctx, userID, activityID).Return(expectedActivity, nil).Once()
			},
			wantErr: false,
		},
		{
			name: "repository error",
			setup: func() {
				mockRepo.On("GetByID", ctx, userID, activityID).Return(nil, assert.AnError).Once()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			activity, err := service.GetActivityByID(ctx, userID, activityID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, activity)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, activity)
				assert.Equal(t, activityID, activity.ID)
				assert.Equal(t, userID, activity.UserID)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestService_GetActivityTypes(t *testing.T) {
	service, _ := setupTestService(t)

	types := service.GetActivityTypes()

	assert.NotNil(t, types)
	assert.Greater(t, len(types), 0)
	assert.Contains(t, types, ActivityTypeAuthLogin)
	assert.Contains(t, types, ActivityTypePromptCreated)
}

func TestService_GetEntityTypes(t *testing.T) {
	service, _ := setupTestService(t)

	types := service.GetEntityTypes()

	assert.NotNil(t, types)
	assert.Greater(t, len(types), 0)
	assert.Contains(t, types, EntityTypeUser)
	assert.Contains(t, types, EntityTypePrompt)
}

func TestService_GetAllTypes(t *testing.T) {
	service, _ := setupTestService(t)

	response := service.GetAllTypes()

	assert.NotNil(t, response)
	assert.NotNil(t, response.ActivityTypes)
	assert.NotNil(t, response.EntityTypes)

	// Verify activity types are populated
	assert.Greater(t, len(response.ActivityTypes), 0)
	assert.Contains(t, response.ActivityTypes, ActivityTypeAuthLogin)
	assert.Contains(t, response.ActivityTypes, ActivityTypePromptCreated)

	// Verify entity types are populated
	assert.Greater(t, len(response.EntityTypes), 0)
	assert.Contains(t, response.EntityTypes, EntityTypeUser)
	assert.Contains(t, response.EntityTypes, EntityTypePrompt)
}

func stringPtr(s string) *string {
	return &s
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestService_GetActivities(t *testing.T) {
	service, mockRepo := setupTestService(t)
	ctx := context.Background()
	userID := uuid.New().String()

	tests := []struct {
		name    string
		filters ActivityFilters
		setup   func()
		wantErr bool
	}{
		{
			name: "successful activities retrieval",
			filters: ActivityFilters{
				UserID: &userID,
				Limit:  10,
				Offset: 0,
			},
			setup: func() {
				mockRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
					Activities: []models.Activity{
						{
							ID:           uuid.New().String(),
							UserID:       userID,
							ActivityType: ActivityTypeAuthLogin,
							EntityType:   EntityTypeUser,
							Description:  "User logged in",
						},
					},
					TotalCount: 1,
					Page:       1,
					PerPage:    10,
					TotalPages: 1,
				}, nil).Once()
			},
			wantErr: false,
		},
		{
			name: "empty activities result",
			filters: ActivityFilters{
				UserID: &userID,
				Limit:  10,
				Offset: 0,
			},
			setup: func() {
				mockRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
					Activities: []models.Activity{},
					TotalCount: 0,
					Page:       1,
					PerPage:    10,
					TotalPages: 0,
				}, nil).Once()
			},
			wantErr: false,
		},
		{
			name: "repository error",
			filters: ActivityFilters{
				UserID: &userID,
				Limit:  10,
				Offset: 0,
			},
			setup: func() {
				mockRepo.On("List", ctx, mock.Anything).Return(nil, assert.AnError).Once()
			},
			wantErr: true,
		},
		{
			name: "activities with all filters",
			filters: ActivityFilters{
				UserID:       &userID,
				ActivityType: stringPtr(ActivityTypePromptCreated),
				EntityType:   stringPtr(EntityTypePrompt),
				Limit:        5,
				Offset:       0,
			},
			setup: func() {
				mockRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
					Activities: []models.Activity{
						{
							ID:           uuid.New().String(),
							UserID:       userID,
							ActivityType: ActivityTypePromptCreated,
							EntityType:   EntityTypePrompt,
							Description:  "Created prompt",
						},
					},
					TotalCount: 1,
					Page:       1,
					PerPage:    5,
					TotalPages: 1,
				}, nil).Once()
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			response, err := service.GetActivities(ctx, tt.filters)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestService_GetActivityStats(t *testing.T) {
	service, mockRepo := setupTestService(t)
	ctx := context.Background()
	userID := uuid.New().String()

	tests := []struct {
		name    string
		setup   func()
		wantErr bool
	}{
		{
			name: "successful stats retrieval",
			setup: func() {
				mockRepo.On("GetStats", ctx, userID).Return(&models.ActivityStatsResponse{
					TotalActivities:    100,
					ActivitiesToday:    10,
					ActivitiesThisWeek: 50,
					TopActivityTypes: []models.ActivityTypeCount{
						{ActivityType: ActivityTypeAuthLogin, Count: 30},
						{ActivityType: ActivityTypePromptCreated, Count: 20},
					},
					TopEntityTypes: []models.EntityTypeCount{
						{EntityType: EntityTypeUser, Count: 40},
						{EntityType: EntityTypePrompt, Count: 25},
					},
					RecentActivities: []models.Activity{
						{
							ID:           uuid.New().String(),
							UserID:       userID,
							ActivityType: ActivityTypeAuthLogin,
							EntityType:   EntityTypeUser,
							Description:  "User logged in",
						},
					},
					ActivitiesByDateWeek: []models.ActivityCountByDate{
						{Date: "2024-01-01", Count: 15},
						{Date: "2024-01-02", Count: 20},
					},
				}, nil).Once()
			},
			wantErr: false,
		},
		{
			name: "repository error",
			setup: func() {
				mockRepo.On("GetStats", ctx, userID).Return(nil, assert.AnError).Once()
			},
			wantErr: true,
		},
		{
			name: "empty stats",
			setup: func() {
				mockRepo.On("GetStats", ctx, userID).Return(&models.ActivityStatsResponse{
					TotalActivities:      0,
					ActivitiesToday:      0,
					ActivitiesThisWeek:   0,
					TopActivityTypes:     []models.ActivityTypeCount{},
					TopEntityTypes:       []models.EntityTypeCount{},
					RecentActivities:     []models.Activity{},
					ActivitiesByDateWeek: []models.ActivityCountByDate{},
				}, nil).Once()
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			stats, err := service.GetActivityStats(ctx, userID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, stats)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, stats)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestService_DeleteActivity(t *testing.T) {
	service, mockRepo := setupTestService(t)
	ctx := context.Background()
	activityID := uuid.New().String()

	tests := []struct {
		name    string
		setup   func()
		wantErr bool
	}{
		{
			name: "successful deletion",
			setup: func() {
				mockRepo.On("Delete", ctx, activityID).Return(nil).Once()
			},
			wantErr: false,
		},
		{
			name: "repository error",
			setup: func() {
				mockRepo.On("Delete", ctx, activityID).Return(assert.AnError).Once()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			err := service.DeleteActivity(ctx, activityID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestService_RecordResourceActivity(t *testing.T) {
	service, mockRepo := setupTestService(t)
	ctx := context.Background()
	userID := uuid.New().String()
	entityID := uuid.New().String()

	tests := []struct {
		name         string
		activityType string
		entityType   string
		entityID     *string
		description  string
		metadata     map[string]interface{}
		setup        func()
		wantErr      bool
	}{
		{
			name:         "successful prompt creation activity",
			activityType: ActivityTypePromptCreated,
			entityType:   EntityTypePrompt,
			entityID:     &entityID,
			description:  "Created a new prompt",
			metadata:     map[string]interface{}{"prompt_name": "Test Prompt"},
			setup: func() {
				mockRepo.On("Create", ctx, mock.MatchedBy(func(activity *models.Activity) bool {
					return activity.UserID == userID &&
						activity.ActivityType == ActivityTypePromptCreated &&
						activity.EntityType == EntityTypePrompt
				})).Return(nil).Once()
			},
			wantErr: false,
		},
		{
			name:         "repository error",
			activityType: ActivityTypeArtifactDeleted,
			entityType:   EntityTypeArtifact,
			entityID:     &entityID,
			description:  "Deleted an artifact",
			metadata:     nil,
			setup: func() {
				mockRepo.On("Create", ctx, mock.Anything).Return(assert.AnError).Once()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			err := service.RecordResourceActivity(
				ctx, userID, tt.activityType, tt.entityType,
				tt.entityID, tt.description, tt.metadata,
			)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestService_RecordClaudeCodeActivity(t *testing.T) {
	service, mockRepo := setupTestService(t)
	ctx := context.Background()
	userID := uuid.New().String()
	sessionID := uuid.New().String()

	tests := []struct {
		name          string
		sessionID     string
		toolName      *string
		hookEventName string
		metadata      map[string]interface{}
		setup         func()
		wantErr       bool
	}{
		{
			name:          "user prompt submit",
			sessionID:     sessionID,
			toolName:      nil,
			hookEventName: "UserPromptSubmit",
			metadata:      map[string]interface{}{"prompt_length": 100},
			setup: func() {
				mockRepo.On("Create", ctx, mock.MatchedBy(func(activity *models.Activity) bool {
					return activity.UserID == userID &&
						activity.ActivityType == ActivityTypeClaudeCodePrompt &&
						activity.EntityType == EntityTypeSession
				})).Return(nil).Once()
			},
			wantErr: false,
		},
		{
			name:          "tool use activity",
			sessionID:     sessionID,
			toolName:      stringPtr("Read"),
			hookEventName: "ToolUse",
			metadata:      map[string]interface{}{"tool_result": "success"},
			setup: func() {
				mockRepo.On("Create", ctx, mock.MatchedBy(func(activity *models.Activity) bool {
					return activity.UserID == userID &&
						activity.ActivityType == ActivityTypeClaudeCodeTool
				})).Return(nil).Once()
			},
			wantErr: false,
		},
		{
			name:          "session start",
			sessionID:     sessionID,
			toolName:      nil,
			hookEventName: "SessionStart",
			metadata:      nil,
			setup: func() {
				mockRepo.On("Create", ctx, mock.MatchedBy(func(activity *models.Activity) bool {
					return activity.ActivityType == ActivityTypeClaudeCodeSession
				})).Return(nil).Once()
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			err := service.RecordClaudeCodeActivity(ctx, userID, tt.sessionID, tt.toolName, tt.hookEventName, tt.metadata)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestService_RunRetentionJob_Success(t *testing.T) {
	t.Parallel()

	const testRetentionDays = 30
	mockRepo := &mocks.ActivityRepositoryMock{}
	service := NewService(ServiceDeps{
		Repo:          mockRepo,
		ProjectRepo:   nil,
		PromptRepo:    nil,
		ArtifactRepo:  nil,
		UserRepo:      nil,
		AgentRepo:     nil,
		BlueprintRepo: nil,
		APIKeyRepo:    nil,
		MemoryRepo:    nil,
		RetentionDays: testRetentionDays,
	})

	ctx := context.Background()
	before := time.Now()

	mockRepo.On("DeleteOlderThan", ctx, mock.MatchedBy(func(cutoff time.Time) bool {
		// The cutoff must be approximately (now - retentionDays).
		// Allow a small delta to account for execution time.
		expectedCutoff := before.UTC().AddDate(0, 0, -testRetentionDays)
		diff := expectedCutoff.Sub(cutoff)
		if diff < 0 {
			diff = -diff
		}
		return diff < time.Second
	})).Return(int64(42), nil).Once()

	err := service.RunRetentionJob(ctx)

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

func TestService_RunRetentionJob_RepositoryError(t *testing.T) {
	t.Parallel()

	mockRepo := &mocks.ActivityRepositoryMock{}
	service := NewService(ServiceDeps{
		Repo:          mockRepo,
		ProjectRepo:   nil,
		PromptRepo:    nil,
		ArtifactRepo:  nil,
		UserRepo:      nil,
		AgentRepo:     nil,
		BlueprintRepo: nil,
		APIKeyRepo:    nil,
		MemoryRepo:    nil,
		RetentionDays: 90,
	})

	ctx := context.Background()

	mockRepo.On("DeleteOlderThan", ctx, mock.Anything).Return(int64(0), assert.AnError).Once()

	err := service.RunRetentionJob(ctx)

	assert.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
	mockRepo.AssertExpectations(t)
}
