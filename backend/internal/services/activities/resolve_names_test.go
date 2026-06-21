package activities

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// setupServiceWithRepos creates a Service wired with individual mock repos for name-resolution tests.
func setupServiceWithRepos(
	t *testing.T,
	projectRepo *mocks.MockProjectRepository,
	promptRepo *mocks.MockPromptRepository,
	artifactRepo *mocks.MockArtifactRepository,
	userRepo *mocks.MockUserRepository,
) (*Service, *mocks.ActivityRepositoryMock) {
	t.Helper()
	activityRepo := &mocks.ActivityRepositoryMock{}
	svc := NewService(activityRepo, projectRepo, promptRepo, artifactRepo, userRepo, nil, nil, nil, nil, 90)
	return svc, activityRepo
}

// setupServiceWithAllRepos creates a Service wired with all mock repos for name-resolution tests.
func setupServiceWithAllRepos(
	t *testing.T,
	projectRepo *mocks.MockProjectRepository,
	promptRepo *mocks.MockPromptRepository,
	artifactRepo *mocks.MockArtifactRepository,
	userRepo *mocks.MockUserRepository,
	agentRepo *mocks.MockAgentRepository,
	blueprintRepo *mocks.MockBlueprintRepository,
	apiKeyRepo *mocks.MockAPIKeyRepository,
	memoryRepo *mocks.MockMemoryRepository,
) (*Service, *mocks.ActivityRepositoryMock) {
	t.Helper()
	activityRepo := &mocks.ActivityRepositoryMock{}
	svc := NewService(
		activityRepo, projectRepo, promptRepo, artifactRepo, userRepo,
		agentRepo, blueprintRepo, apiKeyRepo, memoryRepo, 90,
	)
	return svc, activityRepo
}

func TestService_GetActivities_ResolvesProjectEntityName(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}

	svc, activityRepo := setupServiceWithRepos(t, projectRepo, promptRepo, artifactRepo, userRepo)
	ctx := context.Background()

	userID := uuid.New().String()
	projectID := uuid.New().String()
	projectID2 := uuid.New().String()

	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, ActivityType: ActivityTypePromptCreated,
				EntityType: EntityTypeProject, EntityID: &projectID, Description: "created"},
			{ID: uuid.New().String(), UserID: userID, ActivityType: ActivityTypePromptCreated,
				EntityType: EntityTypeProject, EntityID: &projectID2, Description: "created"},
		},
		TotalCount: 2, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	projectRepo.On("GetNamesByIDs", ctx, userID, mock.MatchedBy(func(ids []string) bool {
		return len(ids) == 2
	})).Return(map[string]string{projectID: "My Project", projectID2: "Other Project"}, nil).Once()

	userRepo.On("GetNamesByIDs", ctx, mock.MatchedBy(func(ids []string) bool {
		return len(ids) == 1 && ids[0] == userID
	})).Return(map[string]string{userID: "Alice"}, nil).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err)
	assert.Len(t, resp.Activities, 2)
	assert.NotNil(t, resp.Activities[0].EntityName)
	assert.Equal(t, "My Project", *resp.Activities[0].EntityName)
	assert.NotNil(t, resp.Activities[1].EntityName)
	assert.Equal(t, "Other Project", *resp.Activities[1].EntityName)
	assert.NotNil(t, resp.Activities[0].ActorName)
	assert.Equal(t, "Alice", *resp.Activities[0].ActorName)

	activityRepo.AssertExpectations(t)
	projectRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivities_ResolvesPromptEntityName(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}

	svc, activityRepo := setupServiceWithRepos(t, projectRepo, promptRepo, artifactRepo, userRepo)
	ctx := context.Background()

	userID := uuid.New().String()
	promptID := uuid.New().String()

	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, ActivityType: ActivityTypePromptCreated,
				EntityType: EntityTypePrompt, EntityID: &promptID, Description: "created"},
		},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	promptRepo.On("GetNamesByIDsCrossTeam", ctx, userID, mock.MatchedBy(func(ids []string) bool {
		return len(ids) == 1 && ids[0] == promptID
	})).Return(map[string]string{promptID: "Test Prompt"}, nil).Once()

	userRepo.On("GetNamesByIDs", ctx, mock.Anything).Return(map[string]string{userID: "Bob"}, nil).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err)
	assert.Len(t, resp.Activities, 1)
	assert.NotNil(t, resp.Activities[0].EntityName)
	assert.Equal(t, "Test Prompt", *resp.Activities[0].EntityName)

	activityRepo.AssertExpectations(t)
	promptRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivities_ResolvesArtifactEntityName(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}

	svc, activityRepo := setupServiceWithRepos(t, projectRepo, promptRepo, artifactRepo, userRepo)
	ctx := context.Background()

	userID := uuid.New().String()
	artifactID := uuid.New().String()

	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, ActivityType: ActivityTypeArtifactCreated,
				EntityType: EntityTypeArtifact, EntityID: &artifactID, Description: "created"},
		},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	artifactRepo.On("GetNamesByIDsCrossTeam", ctx, userID, mock.Anything).
		Return(map[string]string{artifactID: "Report Q1"}, nil).Once()

	userRepo.On("GetNamesByIDs", ctx, mock.Anything).Return(map[string]string{userID: "Carol"}, nil).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err)
	assert.Len(t, resp.Activities, 1)
	assert.NotNil(t, resp.Activities[0].EntityName)
	assert.Equal(t, "Report Q1", *resp.Activities[0].EntityName)

	activityRepo.AssertExpectations(t)
	artifactRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivities_FallbackForDeletedEntity(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}

	svc, activityRepo := setupServiceWithRepos(t, projectRepo, promptRepo, artifactRepo, userRepo)
	ctx := context.Background()

	userID := uuid.New().String()
	deletedID := "abcdef12-0000-0000-0000-000000000000"

	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, ActivityType: ActivityTypePromptDeleted,
				EntityType: EntityTypePrompt, EntityID: &deletedID, Description: "deleted"},
		},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	// Prompt not found — repo returns empty map (entity was deleted)
	promptRepo.On("GetNamesByIDsCrossTeam", ctx, userID, mock.Anything).
		Return(map[string]string{}, nil).Once()

	userRepo.On("GetNamesByIDs", ctx, mock.Anything).Return(map[string]string{userID: "Dave"}, nil).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err)
	assert.Len(t, resp.Activities, 1)
	assert.NotNil(t, resp.Activities[0].EntityName)
	assert.Contains(t, *resp.Activities[0].EntityName, "… (deleted)")

	activityRepo.AssertExpectations(t)
	promptRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivities_NameResolutionFailureDoesNotFailRequest(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}

	svc, activityRepo := setupServiceWithRepos(t, projectRepo, promptRepo, artifactRepo, userRepo)
	ctx := context.Background()

	userID := uuid.New().String()
	promptID := uuid.New().String()

	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, ActivityType: ActivityTypePromptCreated,
				EntityType: EntityTypePrompt, EntityID: &promptID, Description: "created"},
		},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	// Name lookup fails — must not propagate
	promptRepo.On("GetNamesByIDsCrossTeam", ctx, userID, mock.Anything).
		Return(nil, assert.AnError).Once()

	userRepo.On("GetNamesByIDs", ctx, mock.Anything).
		Return(nil, assert.AnError).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err, "name resolution failure must not propagate to the caller")
	assert.Len(t, resp.Activities, 1)
	// When the lookup errors out the name map is nil/empty, so the deleted-sentinel
	// fallback kicks in for resolvable entity types.
	assert.NotNil(t, resp.Activities[0].EntityName)
	assert.Contains(t, *resp.Activities[0].EntityName, "… (deleted)")
	// ActorName should be nil because the user lookup also errored
	assert.Nil(t, resp.Activities[0].ActorName)

	activityRepo.AssertExpectations(t)
	promptRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivities_SkipsNameResolutionForNonResolvableEntityTypes(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}

	svc, activityRepo := setupServiceWithRepos(t, projectRepo, promptRepo, artifactRepo, userRepo)
	ctx := context.Background()

	userID := uuid.New().String()
	sessionID := uuid.New().String()

	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, ActivityType: ActivityTypeClaudeCodeSession,
				EntityType: EntityTypeSession, EntityID: &sessionID, Description: "session"},
		},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	// No calls to project/prompt/artifact repos since entity type is "session"
	userRepo.On("GetNamesByIDs", ctx, mock.Anything).Return(map[string]string{userID: "Eve"}, nil).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err)
	assert.Len(t, resp.Activities, 1)
	// EntityName must be nil for session type
	assert.Nil(t, resp.Activities[0].EntityName)

	activityRepo.AssertExpectations(t)
	projectRepo.AssertExpectations(t)
	promptRepo.AssertExpectations(t)
	artifactRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivities_EmptyStringEntityIDDoesNotTriggerLookup(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}

	svc, activityRepo := setupServiceWithRepos(t, projectRepo, promptRepo, artifactRepo, userRepo)
	ctx := context.Background()

	userID := uuid.New().String()
	emptyID := ""

	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, EntityType: EntityTypePrompt,
				EntityID: &emptyID, Description: "logged in"},
		},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	// Only actor lookup should fire — no entity lookup because EntityID is empty string
	userRepo.On("GetNamesByIDs", ctx, mock.Anything).Return(map[string]string{userID: "Ivan"}, nil).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err)
	assert.Len(t, resp.Activities, 1)
	// EntityName must be nil because EntityID is an empty string
	assert.Nil(t, resp.Activities[0].EntityName)

	activityRepo.AssertExpectations(t)
	projectRepo.AssertExpectations(t)
	promptRepo.AssertExpectations(t)
	artifactRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivities_DeduplicatesEntityIDs(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}

	svc, activityRepo := setupServiceWithRepos(t, projectRepo, promptRepo, artifactRepo, userRepo)
	ctx := context.Background()

	userID := uuid.New().String()
	sharedProjectID := uuid.New().String()

	// Two activities referencing the same project — should only trigger one batch call
	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, EntityType: EntityTypeProject,
				EntityID: &sharedProjectID, Description: "a"},
			{ID: uuid.New().String(), UserID: userID, EntityType: EntityTypeProject,
				EntityID: &sharedProjectID, Description: "b"},
		},
		TotalCount: 2, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	projectRepo.On("GetNamesByIDs", ctx, userID, mock.MatchedBy(func(ids []string) bool {
		// Must be deduplicated to exactly 1 unique ID
		return len(ids) == 1 && ids[0] == sharedProjectID
	})).Return(map[string]string{sharedProjectID: "Shared Project"}, nil).Once()

	userRepo.On("GetNamesByIDs", ctx, mock.Anything).Return(map[string]string{userID: "Frank"}, nil).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err)
	for _, a := range resp.Activities {
		assert.NotNil(t, a.EntityName)
		assert.Equal(t, "Shared Project", *a.EntityName)
	}

	activityRepo.AssertExpectations(t)
	projectRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivityByID_ResolvesNames(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}

	svc, activityRepo := setupServiceWithRepos(t, projectRepo, promptRepo, artifactRepo, userRepo)
	ctx := context.Background()

	userID := uuid.New().String()
	activityID := uuid.New().String()
	promptID := uuid.New().String()

	activityRepo.On("GetByID", ctx, userID, activityID).Return(&models.Activity{
		ID: activityID, UserID: userID, EntityType: EntityTypePrompt, EntityID: &promptID,
		Description: "edited",
	}, nil).Once()

	promptRepo.On("GetNamesByIDsCrossTeam", ctx, userID, mock.Anything).
		Return(map[string]string{promptID: "My Best Prompt"}, nil).Once()

	userRepo.On("GetNamesByIDs", ctx, mock.Anything).Return(map[string]string{userID: "Grace"}, nil).Once()

	activity, err := svc.GetActivityByID(ctx, userID, activityID)

	assert.NoError(t, err)
	assert.NotNil(t, activity.EntityName)
	assert.Equal(t, "My Best Prompt", *activity.EntityName)
	assert.NotNil(t, activity.ActorName)
	assert.Equal(t, "Grace", *activity.ActorName)

	activityRepo.AssertExpectations(t)
	promptRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivityStats_ResolvesNamesForRecentActivities(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}

	svc, activityRepo := setupServiceWithRepos(t, projectRepo, promptRepo, artifactRepo, userRepo)
	ctx := context.Background()

	userID := uuid.New().String()
	artifactID := uuid.New().String()

	activityRepo.On("GetStats", ctx, userID).Return(&models.ActivityStatsResponse{
		TotalActivities:    5,
		ActivitiesToday:    1,
		ActivitiesThisWeek: 3,
		TopActivityTypes:   []models.ActivityTypeCount{},
		TopEntityTypes:     []models.EntityTypeCount{},
		RecentActivities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, EntityType: EntityTypeArtifact,
				EntityID: &artifactID, Description: "created artifact"},
		},
		ActivitiesByDateWeek: []models.ActivityCountByDate{},
	}, nil).Once()

	artifactRepo.On("GetNamesByIDsCrossTeam", ctx, userID, mock.Anything).
		Return(map[string]string{artifactID: "My Document"}, nil).Once()

	userRepo.On("GetNamesByIDs", ctx, mock.Anything).Return(map[string]string{userID: "Heidi"}, nil).Once()

	stats, err := svc.GetActivityStats(ctx, userID)

	assert.NoError(t, err)
	assert.Len(t, stats.RecentActivities, 1)
	assert.NotNil(t, stats.RecentActivities[0].EntityName)
	assert.Equal(t, "My Document", *stats.RecentActivities[0].EntityName)
	assert.NotNil(t, stats.RecentActivities[0].ActorName)
	assert.Equal(t, "Heidi", *stats.RecentActivities[0].ActorName)

	activityRepo.AssertExpectations(t)
	artifactRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivities_NilEntityIDDoesNotTriggerLookup(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}

	svc, activityRepo := setupServiceWithRepos(t, projectRepo, promptRepo, artifactRepo, userRepo)
	ctx := context.Background()

	userID := uuid.New().String()

	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, EntityType: EntityTypeUser,
				EntityID: nil, Description: "logged in"},
		},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	// Only actor lookup should fire — no entity lookup because EntityID is nil
	userRepo.On("GetNamesByIDs", ctx, mock.Anything).Return(map[string]string{userID: "Ivan"}, nil).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err)
	assert.Len(t, resp.Activities, 1)
	assert.Nil(t, resp.Activities[0].EntityName)

	activityRepo.AssertExpectations(t)
	projectRepo.AssertExpectations(t)
	promptRepo.AssertExpectations(t)
	artifactRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivities_ResolvesUserEntityName(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}
	agentRepo := &mocks.MockAgentRepository{}
	blueprintRepo := &mocks.MockBlueprintRepository{}
	apiKeyRepo := &mocks.MockAPIKeyRepository{}
	memoryRepo := &mocks.MockMemoryRepository{}

	svc, activityRepo := setupServiceWithAllRepos(
		t, projectRepo, promptRepo, artifactRepo, userRepo,
		agentRepo, blueprintRepo, apiKeyRepo, memoryRepo,
	)
	ctx := context.Background()

	userID := uuid.New().String()
	entityUserID := uuid.New().String()

	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, ActivityType: ActivityTypeAuthLogin,
				EntityType: EntityTypeUser, EntityID: &entityUserID, Description: "logged in"},
		},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	// Entity user lookup fires for EntityTypeUser entity IDs (entityUserID)
	userRepo.On("GetNamesByIDs", ctx, mock.MatchedBy(func(ids []string) bool {
		return len(ids) == 1 && ids[0] == entityUserID
	})).Return(map[string]string{entityUserID: "Jane Doe"}, nil).Once()

	// Actor name lookup fires for the activity's UserID (userID)
	userRepo.On("GetNamesByIDs", ctx, mock.MatchedBy(func(ids []string) bool {
		return len(ids) == 1 && ids[0] == userID
	})).Return(map[string]string{userID: "John Actor"}, nil).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err)
	assert.Len(t, resp.Activities, 1)
	assert.NotNil(t, resp.Activities[0].EntityName)
	assert.Equal(t, "Jane Doe", *resp.Activities[0].EntityName)
	assert.NotNil(t, resp.Activities[0].ActorName)
	assert.Equal(t, "John Actor", *resp.Activities[0].ActorName)

	activityRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivities_ResolvesAPIKeyEntityName(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}
	agentRepo := &mocks.MockAgentRepository{}
	blueprintRepo := &mocks.MockBlueprintRepository{}
	apiKeyRepo := &mocks.MockAPIKeyRepository{}
	memoryRepo := &mocks.MockMemoryRepository{}

	svc, activityRepo := setupServiceWithAllRepos(
		t, projectRepo, promptRepo, artifactRepo, userRepo,
		agentRepo, blueprintRepo, apiKeyRepo, memoryRepo,
	)
	ctx := context.Background()

	userID := uuid.New().String()
	apiKeyID := uuid.New().String()

	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, ActivityType: ActivityTypeAPIKeyCreated,
				EntityType: EntityTypeAPIKey, EntityID: &apiKeyID, Description: "created key"},
		},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	apiKeyRepo.On("GetNamesByIDs", ctx, userID, mock.MatchedBy(func(ids []string) bool {
		return len(ids) == 1 && ids[0] == apiKeyID
	})).Return(map[string]string{apiKeyID: "My CLI Key"}, nil).Once()

	userRepo.On("GetNamesByIDs", ctx, mock.Anything).Return(map[string]string{userID: "Alice"}, nil).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err)
	assert.Len(t, resp.Activities, 1)
	assert.NotNil(t, resp.Activities[0].EntityName)
	assert.Equal(t, "My CLI Key", *resp.Activities[0].EntityName)

	activityRepo.AssertExpectations(t)
	apiKeyRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivities_ResolvesAgentEntityName(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}
	agentRepo := &mocks.MockAgentRepository{}
	blueprintRepo := &mocks.MockBlueprintRepository{}
	apiKeyRepo := &mocks.MockAPIKeyRepository{}
	memoryRepo := &mocks.MockMemoryRepository{}

	svc, activityRepo := setupServiceWithAllRepos(
		t, projectRepo, promptRepo, artifactRepo, userRepo,
		agentRepo, blueprintRepo, apiKeyRepo, memoryRepo,
	)
	ctx := context.Background()

	userID := uuid.New().String()
	agentID := uuid.New().String()

	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, ActivityType: ActivityTypeAgentCreated,
				EntityType: EntityTypeAgent, EntityID: &agentID, Description: "created agent"},
		},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	agentRepo.On("GetNamesByIDsCrossTeam", ctx, userID, mock.MatchedBy(func(ids []string) bool {
		return len(ids) == 1 && ids[0] == agentID
	})).Return(map[string]string{agentID: "My Coding Agent"}, nil).Once()

	userRepo.On("GetNamesByIDs", ctx, mock.Anything).Return(map[string]string{userID: "Bob"}, nil).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err)
	assert.Len(t, resp.Activities, 1)
	assert.NotNil(t, resp.Activities[0].EntityName)
	assert.Equal(t, "My Coding Agent", *resp.Activities[0].EntityName)

	activityRepo.AssertExpectations(t)
	agentRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivities_ResolvesMemoryEntityName(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}
	agentRepo := &mocks.MockAgentRepository{}
	blueprintRepo := &mocks.MockBlueprintRepository{}
	apiKeyRepo := &mocks.MockAPIKeyRepository{}
	memoryRepo := &mocks.MockMemoryRepository{}

	svc, activityRepo := setupServiceWithAllRepos(
		t, projectRepo, promptRepo, artifactRepo, userRepo,
		agentRepo, blueprintRepo, apiKeyRepo, memoryRepo,
	)
	ctx := context.Background()

	userID := uuid.New().String()
	memoryID := uuid.New().String()

	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, ActivityType: ActivityTypeMemoryCreated,
				EntityType: EntityTypeMemory, EntityID: &memoryID, Description: "created memory"},
		},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	memoryRepo.On("GetNamesByIDsCrossTeam", ctx, userID, mock.MatchedBy(func(ids []string) bool {
		return len(ids) == 1 && ids[0] == memoryID
	})).Return(map[string]string{memoryID: "Always use Go 1.22 for new services"}, nil).Once()

	userRepo.On("GetNamesByIDs", ctx, mock.Anything).Return(map[string]string{userID: "Carol"}, nil).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err)
	assert.Len(t, resp.Activities, 1)
	assert.NotNil(t, resp.Activities[0].EntityName)
	assert.Equal(t, "Always use Go 1.22 for new services", *resp.Activities[0].EntityName)

	activityRepo.AssertExpectations(t)
	memoryRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivities_ResolvesBlueprintEntityName(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}
	agentRepo := &mocks.MockAgentRepository{}
	blueprintRepo := &mocks.MockBlueprintRepository{}
	apiKeyRepo := &mocks.MockAPIKeyRepository{}
	memoryRepo := &mocks.MockMemoryRepository{}

	svc, activityRepo := setupServiceWithAllRepos(
		t, projectRepo, promptRepo, artifactRepo, userRepo,
		agentRepo, blueprintRepo, apiKeyRepo, memoryRepo,
	)
	ctx := context.Background()

	userID := uuid.New().String()
	blueprintID := uuid.New().String()

	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, ActivityType: ActivityTypeBlueprintCreated,
				EntityType: EntityTypeBlueprint, EntityID: &blueprintID, Description: "created blueprint"},
		},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	blueprintRepo.On("GetNamesByIDsCrossTeam", ctx, userID, mock.MatchedBy(func(ids []string) bool {
		return len(ids) == 1 && ids[0] == blueprintID
	})).Return(map[string]string{blueprintID: "CI Pipeline Blueprint"}, nil).Once()

	userRepo.On("GetNamesByIDs", ctx, mock.Anything).Return(map[string]string{userID: "Dave"}, nil).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err)
	assert.Len(t, resp.Activities, 1)
	assert.NotNil(t, resp.Activities[0].EntityName)
	assert.Equal(t, "CI Pipeline Blueprint", *resp.Activities[0].EntityName)

	activityRepo.AssertExpectations(t)
	blueprintRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}

func TestService_GetActivities_FallbackForDeletedEntityNewTypes(t *testing.T) {
	projectRepo := &mocks.MockProjectRepository{}
	promptRepo := &mocks.MockPromptRepository{}
	artifactRepo := &mocks.MockArtifactRepository{}
	userRepo := &mocks.MockUserRepository{}
	agentRepo := &mocks.MockAgentRepository{}
	blueprintRepo := &mocks.MockBlueprintRepository{}
	apiKeyRepo := &mocks.MockAPIKeyRepository{}
	memoryRepo := &mocks.MockMemoryRepository{}

	svc, activityRepo := setupServiceWithAllRepos(
		t, projectRepo, promptRepo, artifactRepo, userRepo,
		agentRepo, blueprintRepo, apiKeyRepo, memoryRepo,
	)
	ctx := context.Background()

	userID := uuid.New().String()
	deletedAgentID := "abcdef12-0000-0000-0000-111111111111"

	activityRepo.On("List", ctx, mock.Anything).Return(&models.ActivityListResponse{
		Activities: []models.Activity{
			{ID: uuid.New().String(), UserID: userID, ActivityType: ActivityTypeAgentDeleted,
				EntityType: EntityTypeAgent, EntityID: &deletedAgentID, Description: "deleted"},
		},
		TotalCount: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil).Once()

	// Agent not found — repo returns empty map (entity was deleted)
	agentRepo.On("GetNamesByIDsCrossTeam", ctx, userID, mock.Anything).
		Return(map[string]string{}, nil).Once()

	userRepo.On("GetNamesByIDs", ctx, mock.Anything).Return(map[string]string{userID: "Eve"}, nil).Once()

	resp, err := svc.GetActivities(ctx, ActivityFilters{UserID: &userID, Limit: 10})

	assert.NoError(t, err)
	assert.Len(t, resp.Activities, 1)
	assert.NotNil(t, resp.Activities[0].EntityName)
	assert.Contains(t, *resp.Activities[0].EntityName, "… (deleted)")

	activityRepo.AssertExpectations(t)
	agentRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}
