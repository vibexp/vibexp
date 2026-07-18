package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// testify's mock.Mock.Called derives the mock method name from the call
// stack, so mechanically identical mock methods cannot share one body
// directly. These helpers take the method name explicitly (MethodCalled) so
// boilerplate mock methods across this package's test files can delegate to a
// single shared implementation.

func mockErrCall(m *mock.Mock, method string, callArgs ...any) error {
	return m.MethodCalled(method, callArgs...).Error(0)
}

func mockPtrCall[T any](m *mock.Mock, method string, callArgs ...any) (*T, error) {
	args := m.MethodCalled(method, callArgs...)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*T), args.Error(1)
}

func mockListCall[T any](m *mock.Mock, method string, callArgs ...any) ([]T, int, error) {
	args := m.MethodCalled(method, callArgs...)
	return args.Get(0).([]T), args.Int(1), args.Error(2)
}

// Mock implementations
type mockUserRepo struct {
	mock.Mock
}

func (m *mockUserRepo) GetByID(ctx context.Context, userID string) (*models.User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *mockUserRepo) GetByGoogleID(ctx context.Context, googleID string) (*models.User, error) {
	args := m.Called(ctx, googleID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *mockUserRepo) GetByIDPSubject(ctx context.Context, provider, subject string) (*models.User, error) {
	args := m.Called(ctx, provider, subject)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *mockUserRepo) GetByStripeCustomerID(ctx context.Context, stripeCustomerID string) (*models.User, error) {
	args := m.Called(ctx, stripeCustomerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func (m *mockUserRepo) Create(ctx context.Context, user *models.User) error {
	return mockErrCall(&m.Mock, "Create", ctx, user)
}

func (m *mockUserRepo) Update(ctx context.Context, user *models.User) error {
	return mockErrCall(&m.Mock, "Update", ctx, user)
}

func (m *mockUserRepo) UpdateSubscriptionStatus(ctx context.Context, userID, status string, plan *string) error {
	args := m.Called(ctx, userID, status, plan)
	return args.Error(0)
}

func (m *mockUserRepo) UpdateSubscriptionStatusWithTrial(
	ctx context.Context,
	userID, status string,
	plan *string,
	trialEnd *time.Time,
) error {
	args := m.Called(ctx, userID, status, plan, trialEnd)
	return args.Error(0)
}

func (m *mockUserRepo) UpdateSubscriptionWithCancellation(
	ctx context.Context,
	userID, status string,
	plan *string,
	trialEnd *time.Time,
	canceledAt *time.Time,
) error {
	args := m.Called(ctx, userID, status, plan, trialEnd, canceledAt)
	return args.Error(0)
}

func (m *mockUserRepo) UpdateStripeCustomerID(ctx context.Context, userID, customerID string) error {
	args := m.Called(ctx, userID, customerID)
	return args.Error(0)
}

func (m *mockUserRepo) UpdateTrialEndsAt(ctx context.Context, userID string, trialEndsAt *time.Time) error {
	args := m.Called(ctx, userID, trialEndsAt)
	return args.Error(0)
}

func (m *mockUserRepo) UpdateDefaultTeamID(ctx context.Context, userID, teamID string) error {
	args := m.Called(ctx, userID, teamID)
	return args.Error(0)
}

func (m *mockUserRepo) MarkOnboardingCompleted(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *mockUserRepo) GetNamesByIDs(_ context.Context, _ []string) (map[string]string, error) {
	return map[string]string{}, nil
}

type mockPromptRepo struct {
	mock.Mock
}

func (m *mockPromptRepo) CountByStatus(ctx context.Context, userID, status string) (int, error) {
	args := m.Called(ctx, userID, status)
	return args.Int(0), args.Error(1)
}

func (m *mockPromptRepo) Create(ctx context.Context, prompt *models.Prompt) error {
	return mockErrCall(&m.Mock, "Create", ctx, prompt)
}

func (m *mockPromptRepo) GetByID(ctx context.Context, userID, teamID, promptID string) (*models.Prompt, error) {
	args := m.Called(ctx, userID, teamID, promptID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Prompt), args.Error(1)
}

func (m *mockPromptRepo) GetBySlug(ctx context.Context, userID, teamID, slug string) (*models.Prompt, error) {
	args := m.Called(ctx, userID, teamID, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Prompt), args.Error(1)
}

func (m *mockPromptRepo) List(
	ctx context.Context,
	userID string,
	filters repositories.PromptFilters,
) ([]models.Prompt, int, error) {
	args := m.Called(ctx, userID, filters)
	return args.Get(0).([]models.Prompt), args.Int(1), args.Error(2)
}

func (m *mockPromptRepo) Update(ctx context.Context, prompt *models.Prompt) error {
	return mockErrCall(&m.Mock, "Update", ctx, prompt)
}

func (m *mockPromptRepo) Delete(ctx context.Context, userID, teamID, promptID string) error {
	args := m.Called(ctx, userID, teamID, promptID)
	return args.Error(0)
}

func (m *mockPromptRepo) GetUserLabels(ctx context.Context, userID string) ([]string, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockPromptRepo) GetByIDCrossTeam(ctx context.Context, userID, promptID string) (*models.Prompt, error) {
	args := m.Called(ctx, userID, promptID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Prompt), args.Error(1)
}

func (m *mockPromptRepo) GetBySlugCrossTeam(ctx context.Context, userID, slug string) (*models.Prompt, error) {
	args := m.Called(ctx, userID, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Prompt), args.Error(1)
}

func (m *mockPromptRepo) GetNamesByIDsCrossTeam(_ context.Context, _ string, _ []string) (map[string]string, error) {
	return map[string]string{}, nil
}

type mockArtifactRepo struct {
	mock.Mock
}

func (m *mockArtifactRepo) GetStats(ctx context.Context, userID, teamID string) (*models.ArtifactStatsResponse, error) {
	args := m.Called(ctx, userID, teamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ArtifactStatsResponse), args.Error(1)
}

func (m *mockArtifactRepo) CountAll(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func (m *mockArtifactRepo) Create(ctx context.Context, artifact *models.Artifact) error {
	return mockErrCall(&m.Mock, "Create", ctx, artifact)
}

func (m *mockArtifactRepo) GetByID(ctx context.Context, userID, teamID, artifactID string) (*models.Artifact, error) {
	args := m.Called(ctx, userID, teamID, artifactID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Artifact), args.Error(1)
}

func (m *mockArtifactRepo) GetByProjectAndSlug(
	ctx context.Context,
	userID, projectName, slug string,
) (*models.Artifact, error) {
	args := m.Called(ctx, userID, projectName, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Artifact), args.Error(1)
}

func (m *mockArtifactRepo) GetByProjectIDAndSlug(
	ctx context.Context,
	userID, teamID, projectID, slug string,
) (*models.Artifact, error) {
	args := m.Called(ctx, userID, teamID, projectID, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Artifact), args.Error(1)
}

func (m *mockArtifactRepo) List(
	ctx context.Context,
	userID string,
	filters repositories.ArtifactFilters,
) ([]models.Artifact, int, error) {
	return mockListCall[models.Artifact](&m.Mock, "List", ctx, userID, filters)
}

func (m *mockArtifactRepo) ListProjects(
	ctx context.Context,
	userID string,
	filters repositories.ProjectFilters,
) ([]repositories.ProjectInfo, int, error) {
	args := m.Called(ctx, userID, filters)
	return args.Get(0).([]repositories.ProjectInfo), args.Int(1), args.Error(2)
}

func (m *mockArtifactRepo) Update(ctx context.Context, artifact *models.Artifact) error {
	return mockErrCall(&m.Mock, "Update", ctx, artifact)
}

func (m *mockArtifactRepo) Delete(ctx context.Context, userID, teamID, artifactID string) error {
	args := m.Called(ctx, userID, teamID, artifactID)
	return args.Error(0)
}

func (m *mockArtifactRepo) GetByIDCrossTeam(ctx context.Context, userID, artifactID string) (*models.Artifact, error) {
	args := m.Called(ctx, userID, artifactID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Artifact), args.Error(1)
}

func (m *mockArtifactRepo) GetByProjectIDAndSlugCrossTeam(
	ctx context.Context, userID, projectID, slug string,
) (*models.Artifact, error) {
	args := m.Called(ctx, userID, projectID, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Artifact), args.Error(1)
}

func (m *mockArtifactRepo) ListCrossTeam(
	ctx context.Context, userID string, filters repositories.ArtifactFilters,
) ([]models.Artifact, int, error) {
	return mockListCall[models.Artifact](&m.Mock, "ListCrossTeam", ctx, userID, filters)
}

func (m *mockArtifactRepo) GetNamesByIDsCrossTeam(
	_ context.Context, _ string, _ []string,
) (map[string]string, error) {
	return map[string]string{}, nil
}

type mockMemoryRepo struct {
	mock.Mock
}

func (m *mockMemoryRepo) List(
	ctx context.Context,
	userID string,
	filters repositories.MemoryFilters,
) ([]models.Memory, int, error) {
	args := m.Called(ctx, userID, filters)
	return args.Get(0).([]models.Memory), args.Int(1), args.Error(2)
}

func (m *mockMemoryRepo) Create(ctx context.Context, memory *models.Memory) error {
	return mockErrCall(&m.Mock, "Create", ctx, memory)
}

func (m *mockMemoryRepo) GetByID(ctx context.Context, userID, teamID, memoryID string) (*models.Memory, error) {
	args := m.Called(ctx, userID, teamID, memoryID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Memory), args.Error(1)
}

func (m *mockMemoryRepo) Update(ctx context.Context, memory *models.Memory) error {
	return mockErrCall(&m.Mock, "Update", ctx, memory)
}

func (m *mockMemoryRepo) Delete(ctx context.Context, userID, teamID, memoryID string) error {
	args := m.Called(ctx, userID, teamID, memoryID)
	return args.Error(0)
}

func (m *mockMemoryRepo) GetByIDCrossTeam(ctx context.Context, userID, memoryID string) (*models.Memory, error) {
	args := m.Called(ctx, userID, memoryID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Memory), args.Error(1)
}

func (m *mockMemoryRepo) SearchByMetadata(
	ctx context.Context,
	userID string,
	metadataKey, metadataValue string,
	filters repositories.MemoryFilters,
) ([]models.Memory, int, error) {
	args := m.Called(ctx, userID, metadataKey, metadataValue, filters)
	return args.Get(0).([]models.Memory), args.Int(1), args.Error(2)
}

func (m *mockMemoryRepo) CountAll(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func (m *mockMemoryRepo) GetNamesByIDsCrossTeam(
	ctx context.Context, userID string, ids []string,
) (map[string]string, error) {
	args := m.Called(ctx, userID, ids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]string), args.Error(1)
}

type mockSpecLibraryRepo struct {
	mock.Mock
}

func (m *mockSpecLibraryRepo) GetStats(ctx context.Context, userID string) (*models.BlueprintStatsResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.BlueprintStatsResponse), args.Error(1)
}

func (m *mockSpecLibraryRepo) Create(ctx context.Context, specLibrary *models.Blueprint) error {
	return mockErrCall(&m.Mock, "Create", ctx, specLibrary)
}

func (m *mockSpecLibraryRepo) GetByID(
	ctx context.Context, userID, teamID, specLibraryID string,
) (*models.Blueprint, error) {
	args := m.Called(ctx, userID, teamID, specLibraryID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Blueprint), args.Error(1)
}

func (m *mockSpecLibraryRepo) GetByIDCrossTeam(
	ctx context.Context, userID, specLibraryID string,
) (*models.Blueprint, error) {
	args := m.Called(ctx, userID, specLibraryID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Blueprint), args.Error(1)
}

func (m *mockSpecLibraryRepo) GetByProjectAndSlug(
	ctx context.Context,
	userID, projectName, slug string,
) (*models.Blueprint, error) {
	args := m.Called(ctx, userID, projectName, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Blueprint), args.Error(1)
}

func (m *mockSpecLibraryRepo) GetByProjectIDAndSlug(
	ctx context.Context,
	userID, teamID, projectID, slug string,
) (*models.Blueprint, error) {
	args := m.Called(ctx, userID, teamID, projectID, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Blueprint), args.Error(1)
}

func (m *mockSpecLibraryRepo) List(
	ctx context.Context,
	userID string,
	filters repositories.BlueprintFilters,
) ([]models.Blueprint, int, error) {
	args := m.Called(ctx, userID, filters)
	return args.Get(0).([]models.Blueprint), args.Int(1), args.Error(2)
}

func (m *mockSpecLibraryRepo) ListProjects(
	ctx context.Context,
	userID string,
	filters repositories.ProjectFilters,
) ([]repositories.ProjectInfo, int, error) {
	args := m.Called(ctx, userID, filters)
	return args.Get(0).([]repositories.ProjectInfo), args.Int(1), args.Error(2)
}

func (m *mockSpecLibraryRepo) Update(ctx context.Context, specLibrary *models.Blueprint) error {
	return mockErrCall(&m.Mock, "Update", ctx, specLibrary)
}

func (m *mockSpecLibraryRepo) Delete(ctx context.Context, userID, teamID, specLibraryID string) error {
	args := m.Called(ctx, userID, teamID, specLibraryID)
	return args.Error(0)
}

func (m *mockSpecLibraryRepo) GetByProjectIDAndSlugCrossTeam(
	ctx context.Context, userID, projectID, slug string,
) (*models.Blueprint, error) {
	args := m.Called(ctx, userID, projectID, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Blueprint), args.Error(1)
}

func (m *mockSpecLibraryRepo) GetNamesByIDsCrossTeam(
	ctx context.Context, userID string, ids []string,
) (map[string]string, error) {
	args := m.Called(ctx, userID, ids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]string), args.Error(1)
}

type mockAgentRepo struct {
	mock.Mock
}

func (m *mockAgentRepo) GetStats(ctx context.Context, userID, teamID string) (*models.AgentStatsResponse, error) {
	args := m.Called(ctx, userID, teamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AgentStatsResponse), args.Error(1)
}

func (m *mockAgentRepo) Create(ctx context.Context, agent *models.Agent) error {
	return mockErrCall(&m.Mock, "Create", ctx, agent)
}

func (m *mockAgentRepo) GetByID(ctx context.Context, userID, teamID, agentID string) (*models.Agent, error) {
	args := m.Called(ctx, userID, teamID, agentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Agent), args.Error(1)
}

func (m *mockAgentRepo) List(
	ctx context.Context,
	userID string,
	filters repositories.AgentFilters,
) ([]models.Agent, int, error) {
	args := m.Called(ctx, userID, filters)
	return args.Get(0).([]models.Agent), args.Int(1), args.Error(2)
}

func (m *mockAgentRepo) Update(ctx context.Context, agent *models.Agent) error {
	return mockErrCall(&m.Mock, "Update", ctx, agent)
}

func (m *mockAgentRepo) Delete(ctx context.Context, userID, teamID, agentID string) error {
	args := m.Called(ctx, userID, teamID, agentID)
	return args.Error(0)
}

func (m *mockAgentRepo) UpdateExecutionStats(
	ctx context.Context,
	agentID string,
	success bool,
	duration int,
) error {
	args := m.Called(ctx, agentID, success, duration)
	return args.Error(0)
}

func (m *mockAgentRepo) GetByIDCrossTeam(ctx context.Context, userID, agentID string) (*models.Agent, error) {
	args := m.Called(ctx, userID, agentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Agent), args.Error(1)
}

func (m *mockAgentRepo) GetNamesByIDsCrossTeam(
	ctx context.Context, userID string, ids []string,
) (map[string]string, error) {
	args := m.Called(ctx, userID, ids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]string), args.Error(1)
}

type mockAgentExecRepo struct {
	mock.Mock
}

type mockClaudeCodeRepo struct {
	mock.Mock
}

func (m *mockClaudeCodeRepo) CountUniqueSessions(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

// Implement other required methods as no-ops for now
func (m *mockClaudeCodeRepo) Create(
	ctx context.Context,
	payload *models.ClaudeCodeHookPayload,
) error {
	return nil
}
func (m *mockClaudeCodeRepo) GetByID(
	ctx context.Context,
	userID string,
	id int,
) (*models.ClaudeCodeHookPayload, error) {
	return nil, nil
}
func (m *mockClaudeCodeRepo) List(
	ctx context.Context,
	filters repositories.ClaudeCodeHooksFilters,
) (*models.ClaudeCodeHooksPaginatedResponse, error) {
	return nil, nil
}
func (m *mockClaudeCodeRepo) GetSessions(
	ctx context.Context,
	filters repositories.SessionFilters,
) (*models.SessionsResponse, error) {
	return nil, nil
}
func (m *mockClaudeCodeRepo) GetSessionCounts(
	ctx context.Context,
	userID string,
	days int,
) (*models.SessionCountsResponse, error) {
	return nil, nil
}
func (m *mockClaudeCodeRepo) GetOverviewStats(ctx context.Context, userID string) (*models.OverviewStats, error) {
	return nil, nil
}
func (m *mockClaudeCodeRepo) GetRecentActivities(
	ctx context.Context,
	filters repositories.RecentActivitiesFilters,
) (*models.RecentActivitiesResponse, error) {
	return nil, nil
}
func (m *mockClaudeCodeRepo) SessionExists(ctx context.Context, userID, sessionID string) (bool, error) {
	return false, nil
}

func (m *mockClaudeCodeRepo) DeleteSession(ctx context.Context, userID, sessionID string) error {
	return nil
}

type mockCursorIDERepo struct {
	mock.Mock
}

func (m *mockCursorIDERepo) CountUniqueSessions(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

// Implement other required methods as no-ops for now
func (m *mockCursorIDERepo) Create(
	ctx context.Context,
	payload *models.CursorIDEHookPayload,
) error {
	return nil
}
func (m *mockCursorIDERepo) GetByID(
	ctx context.Context,
	userID string,
	id int,
) (*models.CursorIDEHookPayload, error) {
	return nil, nil
}
func (m *mockCursorIDERepo) List(
	ctx context.Context,
	filters repositories.CursorIDEHooksFilters,
) (*models.CursorIDEHooksPaginatedResponse, error) {
	return nil, nil
}
func (m *mockCursorIDERepo) GetSessions(
	ctx context.Context,
	filters repositories.CursorSessionFilters,
) (*models.CursorSessionsResponse, error) {
	return nil, nil
}
func (m *mockCursorIDERepo) GetSessionCounts(
	ctx context.Context,
	userID string,
	days int,
) (*models.CursorSessionCountsResponse, error) {
	return nil, nil
}
func (m *mockCursorIDERepo) GetOverviewStats(ctx context.Context, userID string) (*models.CursorOverviewStats, error) {
	return nil, nil
}
func (m *mockCursorIDERepo) GetRecentActivities(
	ctx context.Context,
	filters repositories.CursorRecentActivitiesFilters,
) (*models.CursorRecentActivitiesResponse, error) {
	return nil, nil
}
func (m *mockCursorIDERepo) SessionExists(ctx context.Context, userID, sessionID string) (bool, error) {
	return false, nil
}

func (m *mockCursorIDERepo) DeleteSession(ctx context.Context, userID, sessionID string) error {
	return nil
}

// mockFeedRepo is a minimal mock for the FeedRepository interface used in resource usage tests.
type mockFeedRepo struct {
	mock.Mock
}

func (m *mockFeedRepo) Create(ctx context.Context, feed *models.Feed) error {
	return nil
}

func (m *mockFeedRepo) GetByID(ctx context.Context, userID, teamID, feedID string) (*models.Feed, error) {
	return nil, nil
}

func (m *mockFeedRepo) List(
	ctx context.Context, userID string, filters repositories.FeedFilters,
) ([]models.Feed, int, error) {
	return nil, 0, nil
}

func (m *mockFeedRepo) ListWithLastPost(
	ctx context.Context, userID string, filters repositories.FeedFilters,
) ([]models.FeedWithLastPost, error) {
	return nil, nil
}

func (m *mockFeedRepo) Update(ctx context.Context, feed *models.Feed) error {
	return nil
}

func (m *mockFeedRepo) Delete(ctx context.Context, userID, teamID, feedID string) error {
	return nil
}

func (m *mockFeedRepo) CountAll(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

// mockFeedItemRepo is a minimal mock for the FeedItemRepository interface used in resource usage tests.
type mockFeedItemRepo struct {
	mock.Mock
}

func (m *mockFeedItemRepo) Create(ctx context.Context, item *models.FeedItem) error {
	return nil
}

func (m *mockFeedItemRepo) GetByID(ctx context.Context, userID, teamID, itemID string) (*models.FeedItem, error) {
	return nil, nil
}

func (m *mockFeedItemRepo) GetByIDForPoster(
	ctx context.Context, posterUserID, itemID string,
) (*models.FeedItem, error) {
	return nil, nil
}

func (m *mockFeedItemRepo) List(
	ctx context.Context, userID string, filters repositories.FeedItemFilters,
) ([]models.FeedItem, int, error) {
	return nil, 0, nil
}

func (m *mockFeedItemRepo) Archive(ctx context.Context, userID, teamID, itemID string) error {
	return nil
}

func (m *mockFeedItemRepo) Unarchive(ctx context.Context, userID, teamID, itemID string) error {
	return nil
}

func (m *mockFeedItemRepo) Delete(ctx context.Context, userID, teamID, itemID string) error {
	return nil
}

func (m *mockFeedItemRepo) CountAll(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

// mockFeedItemReplyRepo is a minimal mock for the FeedItemReplyRepository interface used in resource usage tests.
type mockFeedItemReplyRepo struct {
	mock.Mock
}

func (m *mockFeedItemReplyRepo) CreateReply(
	ctx context.Context, reply *models.FeedItemReply,
) (*models.FeedItemReply, error) {
	return nil, nil
}

func (m *mockFeedItemReplyRepo) ListReplies(
	ctx context.Context, teamID, feedItemID string, page, limit int,
) ([]models.FeedItemReply, int, error) {
	return nil, 0, nil
}

func (m *mockFeedItemReplyRepo) CountRepliesByItemIDs(
	ctx context.Context, teamID string, itemIDs []string,
) (map[string]int, error) {
	return nil, nil
}

func (m *mockFeedItemReplyRepo) GetReply(
	ctx context.Context, userID, teamID, replyID string,
) (*models.FeedItemReply, error) {
	return nil, nil
}

func (m *mockFeedItemReplyRepo) GetReplyForPoster(
	ctx context.Context, posterUserID, replyID string,
) (*models.FeedItemReply, error) {
	return nil, nil
}

func (m *mockFeedItemReplyRepo) ListReplyPostersByItemID(
	ctx context.Context, teamID, feedItemID string,
) ([]repositories.FeedItemReplyPoster, error) {
	return nil, nil
}

func (m *mockFeedItemReplyRepo) CountAll(ctx context.Context, userID string) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func (m *mockAgentExecRepo) Create(ctx context.Context, execution *models.AgentExecution) error {
	return mockErrCall(&m.Mock, "Create", ctx, execution)
}

func (m *mockAgentExecRepo) GetByID(ctx context.Context, userID, executionID string) (*models.AgentExecution, error) {
	args := m.Called(ctx, userID, executionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AgentExecution), args.Error(1)
}

func (m *mockAgentExecRepo) List(
	ctx context.Context,
	userID string,
	filters repositories.AgentExecutionFilters,
) ([]models.AgentExecution, int, error) {
	args := m.Called(ctx, userID, filters)
	return args.Get(0).([]models.AgentExecution), args.Int(1), args.Error(2)
}

func (m *mockAgentExecRepo) Update(ctx context.Context, execution *models.AgentExecution) error {
	return mockErrCall(&m.Mock, "Update", ctx, execution)
}

func (m *mockAgentExecRepo) GetByAgentID(
	ctx context.Context,
	userID, agentID string,
	filters repositories.AgentExecutionFilters,
) ([]models.AgentExecution, int, error) {
	args := m.Called(ctx, userID, agentID, filters)
	return args.Get(0).([]models.AgentExecution), args.Int(1), args.Error(2)
}

func (m *mockAgentExecRepo) GetByTaskID(ctx context.Context, userID, taskID string) (*models.AgentExecution, error) {
	args := m.Called(ctx, userID, taskID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AgentExecution), args.Error(1)
}

func (m *mockAgentExecRepo) UpdateTaskInfo(
	ctx context.Context,
	executionID, taskID, contextID, currentState string,
) error {
	args := m.Called(ctx, executionID, taskID, contextID, currentState)
	return args.Error(0)
}

func (m *mockAgentExecRepo) UpdateArtifacts(
	ctx context.Context,
	executionID string,
	artifacts []map[string]interface{},
) error {
	args := m.Called(ctx, executionID, artifacts)
	return args.Error(0)
}

func (m *mockAgentExecRepo) UpdateStatus(ctx context.Context, executionID, status string) error {
	args := m.Called(ctx, executionID, status)
	return args.Error(0)
}

func (m *mockAgentExecRepo) GetByConversationID(
	ctx context.Context,
	userID, conversationID string,
	limit int,
	before *time.Time,
) ([]models.AgentExecution, bool, int, error) {
	args := m.Called(ctx, userID, conversationID, limit, before)
	return args.Get(0).([]models.AgentExecution), args.Bool(1), args.Int(2), args.Error(3)
}

func (m *mockAgentExecRepo) GetFirstExecutionInConversation(
	ctx context.Context,
	userID, conversationID string,
) (*models.AgentExecution, error) {
	args := m.Called(ctx, userID, conversationID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AgentExecution), args.Error(1)
}

func (m *mockAgentExecRepo) UpdateConversationID(ctx context.Context, executionID, conversationID string) error {
	args := m.Called(ctx, executionID, conversationID)
	return args.Error(0)
}

func (m *mockAgentExecRepo) ListConversations(
	ctx context.Context,
	userID, agentID string,
	page, limit int,
) ([]models.ConversationSummary, int, error) {
	args := m.Called(ctx, userID, agentID, page, limit)
	return args.Get(0).([]models.ConversationSummary), args.Int(1), args.Error(2)
}

func TestCheckResourceLimit(t *testing.T) {
	// The open-source build has no paid tiers or quotas: CheckResourceLimit
	// always permits the operation regardless of resource type or usage.
	service := NewResourceUsageService(
		new(mockUserRepo),
		new(mockPromptRepo),
		new(mockArtifactRepo),
		new(mockMemoryRepo),
		new(mockAgentRepo),
		new(mockAgentExecRepo),
		new(mockClaudeCodeRepo),
		new(mockCursorIDERepo),
		new(mockSpecLibraryRepo),
		new(mockTeamRepo),
		new(mockTeamMemberRepo),
		new(mockTeamSubscriptionRepo),
		new(mockFeedRepo),
		new(mockFeedItemRepo),
		new(mockFeedItemReplyRepo),
		slog.New(slog.DiscardHandler),
	)

	for _, resourceType := range []string{
		events.ResourceTypePrompt,
		events.ResourceTypeArtifact,
		events.ResourceTypeMemory,
		events.ResourceTypeBlueprint,
		events.ResourceTypeTeam,
	} {
		allowed, err := service.CheckResourceLimit(context.Background(), "test-user-id", resourceType)
		assert.NoError(t, err)
		assert.True(t, allowed)
	}
}

func TestNormalizePlanName_StripePlans(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"VibeXP - Power User", models.PlanPowerUser},
		{"VibeXP - Professional", models.PlanPro},
		{"VibeXP - Starter", models.PlanStarter},
		{"vibexp - Power User", models.PlanPowerUser},
		{"VIBEXP - POWER USER", models.PlanPowerUser},
	}

	for _, tt := range tests {
		result := models.NormalizePlanName(tt.input)
		assert.Equal(t, tt.expected, result)
	}
}

func TestNormalizePlanName_WithoutPrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Power User", models.PlanPowerUser},
		{"Professional", models.PlanPro},
		{"Pro", models.PlanPro},
		{"PowerUser", models.PlanPowerUser},
		{"free", models.PlanBasic},
	}

	for _, tt := range tests {
		result := models.NormalizePlanName(tt.input)
		assert.Equal(t, tt.expected, result)
	}
}

func TestNormalizePlanName_AlreadyNormalized(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"power_user", models.PlanPowerUser},
		{"professional", models.PlanPro},
		{"starter", models.PlanStarter},
		{"free", models.PlanBasic},
	}

	for _, tt := range tests {
		result := models.NormalizePlanName(tt.input)
		assert.Equal(t, tt.expected, result)
	}
}

func TestNormalizePlanName_UnknownPlan(t *testing.T) {
	result := models.NormalizePlanName("Unknown Plan")
	assert.Equal(t, models.PlanStarter, result)
}

func TestGetResourceLimit_PowerUserPlan(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	service := &ResourceUsageService{logger: logger}
	planName := "VibeXP - Power User"

	tests := []struct {
		resourceType  string
		expectedLimit int
	}{
		{events.ResourceTypeAITool, -1},
		{events.ResourceTypeAISession, 2000},
		{events.ResourceTypePrompt, 1000},
		{events.ResourceTypeArtifact, 1000},
		{events.ResourceTypeMemory, 1000},
		{events.ResourceTypeAgent, -1},
		{events.ResourceTypeAgentConv, 1500},
	}

	for _, tt := range tests {
		limit := service.getResourceLimit(tt.resourceType, &planName)
		assert.Equal(t, tt.expectedLimit, limit)
	}
}

func TestGetResourceLimit_ProfessionalPlan(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	service := &ResourceUsageService{logger: logger}
	planName := "VibeXP - Professional"

	limit := service.getResourceLimit(events.ResourceTypePrompt, &planName)
	assert.Equal(t, 500, limit)
}

func TestGetResourceLimit_StarterPlan(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	service := &ResourceUsageService{logger: logger}
	planName := "VibeXP - Starter"

	limit := service.getResourceLimit(events.ResourceTypePrompt, &planName)
	assert.Equal(t, 200, limit)
}

// TestCountAgentConversations_EmptyAgentID tests counting all conversations across all agents
func TestCountAgentConversations_EmptyAgentID(t *testing.T) {
	// Setup mocks
	userRepo := new(mockUserRepo)
	promptRepo := new(mockPromptRepo)
	artifactRepo := new(mockArtifactRepo)
	memoryRepo := new(mockMemoryRepo)
	agentRepo := new(mockAgentRepo)
	agentExecRepo := new(mockAgentExecRepo)
	logger := slog.New(slog.DiscardHandler)

	// Setup mock AI hooks repositories
	claudeCodeRepo := new(mockClaudeCodeRepo)
	cursorIDERepo := new(mockCursorIDERepo)
	specLibraryRepo := new(mockSpecLibraryRepo)
	teamRepo := new(mockTeamRepo)

	// Setup service
	service := NewResourceUsageService(
		userRepo,
		promptRepo,
		artifactRepo,
		memoryRepo,
		agentRepo,
		agentExecRepo,
		claudeCodeRepo,
		cursorIDERepo,
		specLibraryRepo,
		teamRepo,
		nil, // teamMemberRepo
		nil, // teamSubscriptionRepo
		new(mockFeedRepo),
		new(mockFeedItemRepo),
		new(mockFeedItemReplyRepo),
		logger,
	)

	// Setup test data
	userID := "test-user-id"

	// Create mock conversation summaries
	conversations := []models.ConversationSummary{
		{
			ConversationID: "conv-1",
			AgentID:        "agent-1",
			MessageCount:   3,
		},
		{
			ConversationID: "conv-2",
			AgentID:        "agent-2",
			MessageCount:   5,
		},
		{
			ConversationID: "conv-3",
			AgentID:        "agent-1",
			MessageCount:   2,
		},
	}

	// Mock ListConversations to be called with empty agentID
	// First call returns full page
	agentExecRepo.On("ListConversations", mock.Anything, userID, "", 1, 100).Return(conversations, 3, nil)

	// Execute
	count, err := service.countAgentConversations(context.Background(), userID)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 3, count)

	// Verify mocks - specifically that empty string was passed as agentID
	agentExecRepo.AssertExpectations(t)
}

// TestCountAgentConversations_Pagination tests counting conversations with pagination
func TestCountAgentConversations_Pagination(t *testing.T) {
	// Setup mocks
	userRepo := new(mockUserRepo)
	promptRepo := new(mockPromptRepo)
	artifactRepo := new(mockArtifactRepo)
	memoryRepo := new(mockMemoryRepo)
	agentRepo := new(mockAgentRepo)
	agentExecRepo := new(mockAgentExecRepo)
	logger := slog.New(slog.DiscardHandler)

	// Setup mock AI hooks repositories
	claudeCodeRepo := new(mockClaudeCodeRepo)
	cursorIDERepo := new(mockCursorIDERepo)
	specLibraryRepo := new(mockSpecLibraryRepo)
	teamRepo := new(mockTeamRepo)

	// Setup service
	service := NewResourceUsageService(
		userRepo,
		promptRepo,
		artifactRepo,
		memoryRepo,
		agentRepo,
		agentExecRepo,
		claudeCodeRepo,
		cursorIDERepo,
		specLibraryRepo,
		teamRepo,
		nil, // teamMemberRepo
		nil, // teamSubscriptionRepo
		new(mockFeedRepo),
		new(mockFeedItemRepo),
		new(mockFeedItemReplyRepo),
		logger,
	)

	// Setup test data
	userID := "test-user-id"

	// Create mock conversation summaries - simulate pagination
	firstPageConversations := make([]models.ConversationSummary, 100)
	for i := 0; i < 100; i++ {
		firstPageConversations[i] = models.ConversationSummary{
			ConversationID: fmt.Sprintf("conv-%d", i),
			AgentID:        fmt.Sprintf("agent-%d", i%3),
			MessageCount:   i + 1,
		}
	}

	secondPageConversations := []models.ConversationSummary{
		{
			ConversationID: "conv-100",
			AgentID:        "agent-1",
			MessageCount:   101,
		},
	}

	// Mock ListConversations - first page returns 100 items, second page returns 1 item
	agentExecRepo.On("ListConversations", mock.Anything, userID, "", 1, 100).Return(firstPageConversations, 101, nil)
	agentExecRepo.On("ListConversations", mock.Anything, userID, "", 2, 100).Return(secondPageConversations, 101, nil)

	// Execute
	count, err := service.countAgentConversations(context.Background(), userID)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 101, count)

	// Verify mocks
	agentExecRepo.AssertExpectations(t)
}

//nolint:funlen // Table-driven test with multiple test cases
func TestCountSpecLibraries(t *testing.T) {
	// Setup mocks
	userRepo := new(mockUserRepo)
	promptRepo := new(mockPromptRepo)
	artifactRepo := new(mockArtifactRepo)
	memoryRepo := new(mockMemoryRepo)
	agentRepo := new(mockAgentRepo)
	agentExecRepo := new(mockAgentExecRepo)
	claudeCodeRepo := new(mockClaudeCodeRepo)
	cursorIDERepo := new(mockCursorIDERepo)
	specLibraryRepo := new(mockSpecLibraryRepo)
	teamRepo := new(mockTeamRepo)
	logger := slog.New(slog.DiscardHandler)

	// Setup service
	service := NewResourceUsageService(
		userRepo,
		promptRepo,
		artifactRepo,
		memoryRepo,
		agentRepo,
		agentExecRepo,
		claudeCodeRepo,
		cursorIDERepo,
		specLibraryRepo,
		teamRepo,
		nil, // teamMemberRepo
		nil, // teamSubscriptionRepo
		new(mockFeedRepo),
		new(mockFeedItemRepo),
		new(mockFeedItemReplyRepo),
		logger,
	)

	// Setup test data
	userID := "test-user-id"
	expectedCount := 42

	// Mock GetStats
	specLibraryRepo.On("GetStats", mock.Anything, userID).Return(&models.BlueprintStatsResponse{
		TotalBlueprints: expectedCount,
	}, nil)

	// Execute
	count, err := service.countSpecLibraries(context.Background(), userID)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, expectedCount, count)

	// Verify mocks
	specLibraryRepo.AssertExpectations(t)
}

type mockTeamRepo struct {
	mock.Mock
}

func (m *mockTeamRepo) Create(ctx context.Context, team *models.Team) error {
	return mockErrCall(&m.Mock, "Create", ctx, team)
}

func (m *mockTeamRepo) GetByID(ctx context.Context, teamID string) (*models.Team, error) {
	args := m.Called(ctx, teamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Team), args.Error(1)
}

func (m *mockTeamRepo) GetByOwnerID(ctx context.Context, ownerID string) (*models.Team, error) {
	args := m.Called(ctx, ownerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Team), args.Error(1)
}

func (m *mockTeamRepo) GetByOwnerAndSlug(ctx context.Context, ownerID, slug string) (*models.Team, error) {
	args := m.Called(ctx, ownerID, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Team), args.Error(1)
}

func (m *mockTeamRepo) Update(ctx context.Context, team *models.Team) error {
	return mockErrCall(&m.Mock, "Update", ctx, team)
}

func (m *mockTeamRepo) Delete(ctx context.Context, ownerID, teamID string) error {
	args := m.Called(ctx, ownerID, teamID)
	return args.Error(0)
}

func (m *mockTeamRepo) TransferOwnership(ctx context.Context, teamID, fromUserID, toUserID string) error {
	args := m.Called(ctx, teamID, fromUserID, toUserID)
	return args.Error(0)
}

func (m *mockTeamRepo) ListByOwnerID(
	ctx context.Context, ownerID string, limit, offset int,
) ([]models.Team, int, error) {
	args := m.Called(ctx, ownerID, limit, offset)
	return args.Get(0).([]models.Team), args.Int(1), args.Error(2)
}

func (m *mockTeamRepo) ListByUserID(
	ctx context.Context, userID string, limit, offset int,
) ([]models.Team, int, error) {
	args := m.Called(ctx, userID, limit, offset)
	return args.Get(0).([]models.Team), args.Int(1), args.Error(2)
}

func (m *mockTeamRepo) CountByOwnerID(ctx context.Context, ownerID string) (int, error) {
	args := m.Called(ctx, ownerID)
	return args.Int(0), args.Error(1)
}

func (m *mockTeamRepo) GetTeamStats(ctx context.Context, teamID string) (*models.TeamStatsResponse, error) {
	args := m.Called(ctx, teamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TeamStatsResponse), args.Error(1)
}

func (m *mockTeamRepo) GetTeamResourceCreationMetrics(
	ctx context.Context, teamID string, since time.Time,
) ([]models.TeamResourceCreationCount, error) {
	args := m.Called(ctx, teamID, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.TeamResourceCreationCount), args.Error(1)
}

func (m *mockTeamRepo) GetTeamFeedCreationMetrics(
	ctx context.Context, teamID string, since time.Time,
) ([]models.TeamFeedCreationCount, error) {
	args := m.Called(ctx, teamID, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.TeamFeedCreationCount), args.Error(1)
}

//nolint:funlen // Table-driven test with multiple test cases
func TestCountTeams(t *testing.T) {
	// Setup mocks
	userRepo := new(mockUserRepo)
	promptRepo := new(mockPromptRepo)
	artifactRepo := new(mockArtifactRepo)
	memoryRepo := new(mockMemoryRepo)
	agentRepo := new(mockAgentRepo)
	agentExecRepo := new(mockAgentExecRepo)
	claudeCodeRepo := new(mockClaudeCodeRepo)
	cursorIDERepo := new(mockCursorIDERepo)
	specLibraryRepo := new(mockSpecLibraryRepo)
	teamRepo := new(mockTeamRepo)
	logger := slog.New(slog.DiscardHandler)

	// Setup service
	service := NewResourceUsageService(
		userRepo,
		promptRepo,
		artifactRepo,
		memoryRepo,
		agentRepo,
		agentExecRepo,
		claudeCodeRepo,
		cursorIDERepo,
		specLibraryRepo,
		teamRepo,
		nil, // teamMemberRepo
		nil, // teamSubscriptionRepo
		new(mockFeedRepo),
		new(mockFeedItemRepo),
		new(mockFeedItemReplyRepo),
		logger,
	)

	// Setup test data
	userID := "test-user-id"
	expectedCount := 5

	// Mock CountByOwnerID
	teamRepo.On("CountByOwnerID", mock.Anything, userID).Return(expectedCount, nil)

	// Execute
	count, err := service.countTeams(context.Background(), userID)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, expectedCount, count)

	// Verify mocks
	teamRepo.AssertExpectations(t)
}

//nolint:funlen // Table-driven test with multiple mock setups
func TestGetResourceUsage_IncludesTeams(t *testing.T) {
	// Setup mocks
	userRepo := new(mockUserRepo)
	promptRepo := new(mockPromptRepo)
	artifactRepo := new(mockArtifactRepo)
	memoryRepo := new(mockMemoryRepo)
	agentRepo := new(mockAgentRepo)
	agentExecRepo := new(mockAgentExecRepo)
	claudeCodeRepo := new(mockClaudeCodeRepo)
	cursorIDERepo := new(mockCursorIDERepo)
	specLibraryRepo := new(mockSpecLibraryRepo)
	teamRepo := new(mockTeamRepo)
	teamMemberRepo := new(mockTeamMemberRepo)
	teamSubscriptionRepo := new(mockTeamSubscriptionRepo)
	feedRepo := new(mockFeedRepo)
	feedItemRepo := new(mockFeedItemRepo)
	feedItemReplyRepo := new(mockFeedItemReplyRepo)
	logger := slog.New(slog.DiscardHandler)

	// Setup service
	service := NewResourceUsageService(
		userRepo,
		promptRepo,
		artifactRepo,
		memoryRepo,
		agentRepo,
		agentExecRepo,
		claudeCodeRepo,
		cursorIDERepo,
		specLibraryRepo,
		teamRepo,
		teamMemberRepo,
		teamSubscriptionRepo,
		feedRepo,
		feedItemRepo,
		feedItemReplyRepo,
		logger,
	)

	// Setup test data
	userID := "test-user-id"
	plan := models.PlanPro
	user := &models.User{
		ID:               userID,
		SubscriptionPlan: &plan,
	}

	// Mock all repositories
	userRepo.On("GetByID", mock.Anything, userID).Return(user, nil)
	teamMemberRepo.On("GetByUserID", mock.Anything, userID).Return([]models.TeamMember{}, nil)
	claudeCodeRepo.On("CountUniqueSessions", mock.Anything, userID).Return(1, nil)
	cursorIDERepo.On("CountUniqueSessions", mock.Anything, userID).Return(1, nil)
	promptRepo.On("CountByStatus", mock.Anything, userID, "draft").Return(10, nil)
	promptRepo.On("CountByStatus", mock.Anything, userID, "published").Return(20, nil)
	artifactRepo.On("CountAll", mock.Anything, userID).Return(15, nil)
	memoryRepo.On("CountAll", mock.Anything, userID).Return(12, nil)
	specLibraryRepo.On("GetStats", mock.Anything, userID).Return(
		&models.BlueprintStatsResponse{TotalBlueprints: 8}, nil,
	)
	agentRepo.On("GetStats", mock.Anything, userID, "").Return(&models.AgentStatsResponse{TotalAgents: 3}, nil)

	// Create mock conversation summaries
	conversations := []models.ConversationSummary{
		{ConversationID: "conv-1", AgentID: "agent-1", MessageCount: 3},
		{ConversationID: "conv-2", AgentID: "agent-2", MessageCount: 5},
	}
	agentExecRepo.On("ListConversations", mock.Anything, userID, "", 1, 100).Return(conversations, 2, nil)

	teamRepo.On("CountByOwnerID", mock.Anything, userID).Return(3, nil)
	feedRepo.On("CountAll", mock.Anything, userID).Return(1, nil)
	feedItemRepo.On("CountAll", mock.Anything, userID).Return(50, nil)
	feedItemReplyRepo.On("CountAll", mock.Anything, userID).Return(10, nil)

	// Execute
	response, err := service.GetResourceUsage(context.Background(), userID)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, response)

	// Find the team resource in the response
	var teamResource *models.ResourceUsageItem
	for i := range response.Resources {
		if response.Resources[i].ResourceType == events.ResourceTypeTeam {
			teamResource = &response.Resources[i]
			break
		}
	}

	// Verify team resource is included
	assert.NotNil(t, teamResource, "Team resource should be included in response")
	assert.Equal(t, 3, teamResource.Count)
	assert.Equal(t, 8, teamResource.Limit)
	assert.Equal(t, 37, teamResource.Percentage) // 3/8 * 100 = 37

	// Verify mocks
	userRepo.AssertExpectations(t)
	teamRepo.AssertExpectations(t)
	feedRepo.AssertExpectations(t)
	feedItemRepo.AssertExpectations(t)
	feedItemReplyRepo.AssertExpectations(t)
}

// Mock implementations for team-related repositories
type mockTeamMemberRepo struct {
	mock.Mock
}

func (m *mockTeamMemberRepo) Create(ctx context.Context, member *models.TeamMember) error {
	args := m.Called(ctx, member)
	return args.Error(0)
}

func (m *mockTeamMemberRepo) GetByTeamAndUser(ctx context.Context, teamID, userID string) (*models.TeamMember, error) {
	args := m.Called(ctx, teamID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TeamMember), args.Error(1)
}

func (m *mockTeamMemberRepo) GetByTeamID(ctx context.Context, teamID string) ([]models.TeamMember, error) {
	args := m.Called(ctx, teamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.TeamMember), args.Error(1)
}

func (m *mockTeamMemberRepo) GetByUserID(ctx context.Context, userID string) ([]models.TeamMember, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.TeamMember), args.Error(1)
}

func (m *mockTeamMemberRepo) UpdateRole(ctx context.Context, teamID, userID string, role models.TeamMemberRole) error {
	args := m.Called(ctx, teamID, userID, role)
	return args.Error(0)
}

func (m *mockTeamMemberRepo) Delete(ctx context.Context, teamID, userID string) error {
	args := m.Called(ctx, teamID, userID)
	return args.Error(0)
}

type mockTeamSubscriptionRepo struct {
	mock.Mock
}

func (m *mockTeamSubscriptionRepo) Create(ctx context.Context, subscription *models.TeamSubscription) error {
	return mockErrCall(&m.Mock, "Create", ctx, subscription)
}

func (m *mockTeamSubscriptionRepo) GetByID(ctx context.Context, id string) (*models.TeamSubscription, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TeamSubscription), args.Error(1)
}

func (m *mockTeamSubscriptionRepo) GetByTeamID(ctx context.Context, teamID string) (*models.TeamSubscription, error) {
	return mockPtrCall[models.TeamSubscription](&m.Mock, "GetByTeamID", ctx, teamID)
}

func (m *mockTeamSubscriptionRepo) GetByStripeSubscriptionID(
	ctx context.Context,
	stripeSubID string,
) (*models.TeamSubscription, error) {
	args := m.Called(ctx, stripeSubID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.TeamSubscription), args.Error(1)
}

func (m *mockTeamSubscriptionRepo) Update(ctx context.Context, subscription *models.TeamSubscription) error {
	return mockErrCall(&m.Mock, "Update", ctx, subscription)
}

func (m *mockTeamSubscriptionRepo) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockTeamSubscriptionRepo) UpdateStatus(ctx context.Context, stripeSubID, status string) error {
	args := m.Called(ctx, stripeSubID, status)
	return args.Error(0)
}

func (m *mockTeamSubscriptionRepo) UpdateSeatCount(ctx context.Context, stripeSubID string, seatCount int) error {
	args := m.Called(ctx, stripeSubID, seatCount)
	return args.Error(0)
}

func (m *mockTeamSubscriptionRepo) ListByStatus(
	ctx context.Context,
	status string,
	limit, offset int,
) ([]*models.TeamSubscription, int, error) {
	args := m.Called(ctx, status, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*models.TeamSubscription), args.Int(1), args.Error(2)
}

func (m *mockTeamSubscriptionRepo) ListByTier(
	ctx context.Context,
	tier string,
	limit, offset int,
) ([]*models.TeamSubscription, int, error) {
	args := m.Called(ctx, tier, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*models.TeamSubscription), args.Int(1), args.Error(2)
}

func (m *mockTeamSubscriptionRepo) GetActiveByTeamID(
	ctx context.Context,
	teamID string,
) (*models.TeamSubscription, error) {
	return mockPtrCall[models.TeamSubscription](&m.Mock, "GetActiveByTeamID", ctx, teamID)
}

func (m *mockTeamSubscriptionRepo) GetCanceledByTeamID(
	ctx context.Context,
	teamID string,
) (*models.TeamSubscription, error) {
	return mockPtrCall[models.TeamSubscription](&m.Mock, "GetCanceledByTeamID", ctx, teamID)
}

// TestGetTeamQuotaContribution_SingleTeam tests quota calculation for a single team
//
//nolint:funlen // Table-driven test with multiple test cases and mock setups
func TestGetTeamQuotaContribution_SingleTeam(t *testing.T) {
	tests := []struct {
		name               string
		tier               string
		seatCount          int
		resourceType       string
		expectedQuota      int
		subscriptionStatus string
	}{
		{
			name:               "Teams Starter - Prompts - 3 seats",
			tier:               models.TeamTierStarter,
			seatCount:          3,
			resourceType:       events.ResourceTypePrompt,
			expectedQuota:      1300, // 1000 base + (100 * 3) - PRD Section 9.2
			subscriptionStatus: models.TeamSubscriptionStatusActive,
		},
		{
			name:               "Teams Professional - Artifacts - 5 seats",
			tier:               models.TeamTierProfessional,
			seatCount:          5,
			resourceType:       events.ResourceTypeArtifact,
			expectedQuota:      1500, // 1000 base + (100 * 5)
			subscriptionStatus: models.TeamSubscriptionStatusActive,
		},
		{
			name:               "Teams Enterprise - Memory - 10 seats",
			tier:               models.TeamTierEnterprise,
			seatCount:          10,
			resourceType:       events.ResourceTypeMemory,
			expectedQuota:      -1, // Unlimited - PRD Section 9.2
			subscriptionStatus: models.TeamSubscriptionStatusActive,
		},
		{
			name:               "Teams Starter - Agent - 2 seats",
			tier:               models.TeamTierStarter,
			seatCount:          2,
			resourceType:       events.ResourceTypeAgent,
			expectedQuota:      7, // 5 base + (1 * 2)
			subscriptionStatus: models.TeamSubscriptionStatusActive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			userRepo := new(mockUserRepo)
			promptRepo := new(mockPromptRepo)
			artifactRepo := new(mockArtifactRepo)
			memoryRepo := new(mockMemoryRepo)
			agentRepo := new(mockAgentRepo)
			agentExecRepo := new(mockAgentExecRepo)
			claudeCodeRepo := new(mockClaudeCodeRepo)
			cursorIDERepo := new(mockCursorIDERepo)
			specLibraryRepo := new(mockSpecLibraryRepo)
			teamRepo := new(mockTeamRepo)
			teamMemberRepo := new(mockTeamMemberRepo)
			teamSubscriptionRepo := new(mockTeamSubscriptionRepo)
			logger := slog.New(slog.DiscardHandler)

			// Setup service
			service := NewResourceUsageService(
				userRepo,
				promptRepo,
				artifactRepo,
				memoryRepo,
				agentRepo,
				agentExecRepo,
				claudeCodeRepo,
				cursorIDERepo,
				specLibraryRepo,
				teamRepo,
				teamMemberRepo,
				teamSubscriptionRepo,
				new(mockFeedRepo),
				new(mockFeedItemRepo),
				new(mockFeedItemReplyRepo),
				logger,
			)

			// Setup test data
			userID := "test-user-id"
			teamID := "team-123"

			// Mock team membership
			teamMemberRepo.On("GetByUserID", mock.Anything, userID).Return([]models.TeamMember{
				{TeamID: teamID, UserID: userID, Role: models.TeamMemberRoleMember},
			}, nil)

			// Mock team (not personal workspace)
			teamRepo.On("GetByID", mock.Anything, teamID).Return(&models.Team{
				ID:         teamID,
				IsPersonal: false,
			}, nil)

			// Mock team subscription
			teamSubscriptionRepo.On("GetByTeamID", mock.Anything, teamID).Return(&models.TeamSubscription{
				TeamID:    teamID,
				Tier:      tt.tier,
				SeatCount: tt.seatCount,
				Status:    tt.subscriptionStatus,
			}, nil)

			// Execute
			quota, err := service.getTeamQuotaContribution(context.Background(), userID, tt.resourceType)

			// Assert
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedQuota, quota)

			// Verify mocks
			teamMemberRepo.AssertExpectations(t)
			teamRepo.AssertExpectations(t)
			teamSubscriptionRepo.AssertExpectations(t)
		})
	}
}

// TestGetTeamQuotaContribution_MultipleTeams tests quota aggregation from multiple teams
//
//nolint:funlen // Comprehensive test with multiple team setups
func TestGetTeamQuotaContribution_MultipleTeams(t *testing.T) {
	// Setup mocks
	userRepo := new(mockUserRepo)
	promptRepo := new(mockPromptRepo)
	artifactRepo := new(mockArtifactRepo)
	memoryRepo := new(mockMemoryRepo)
	agentRepo := new(mockAgentRepo)
	agentExecRepo := new(mockAgentExecRepo)
	claudeCodeRepo := new(mockClaudeCodeRepo)
	cursorIDERepo := new(mockCursorIDERepo)
	specLibraryRepo := new(mockSpecLibraryRepo)
	teamRepo := new(mockTeamRepo)
	teamMemberRepo := new(mockTeamMemberRepo)
	teamSubscriptionRepo := new(mockTeamSubscriptionRepo)
	logger := slog.New(slog.DiscardHandler)

	// Setup service
	service := NewResourceUsageService(
		userRepo,
		promptRepo,
		artifactRepo,
		memoryRepo,
		agentRepo,
		agentExecRepo,
		claudeCodeRepo,
		cursorIDERepo,
		specLibraryRepo,
		teamRepo,
		teamMemberRepo,
		teamSubscriptionRepo,
		new(mockFeedRepo),
		new(mockFeedItemRepo),
		new(mockFeedItemReplyRepo),
		logger,
	)

	// Setup test data
	userID := "test-user-id"
	team1ID := "team-1"
	team2ID := "team-2"

	// Mock team memberships (user is member of 2 teams)
	teamMemberRepo.On("GetByUserID", mock.Anything, userID).Return([]models.TeamMember{
		{TeamID: team1ID, UserID: userID, Role: models.TeamMemberRoleMember},
		{TeamID: team2ID, UserID: userID, Role: models.TeamMemberRoleMember},
	}, nil)

	// Mock teams (both not personal workspaces)
	teamRepo.On("GetByID", mock.Anything, team1ID).Return(&models.Team{
		ID:         team1ID,
		IsPersonal: false,
	}, nil)
	teamRepo.On("GetByID", mock.Anything, team2ID).Return(&models.Team{
		ID:         team2ID,
		IsPersonal: false,
	}, nil)

	// Mock team subscriptions
	// Team 1: Starter with 3 seats -> 1000 + (100 * 3) = 1300 prompts (PRD Section 9.2)
	teamSubscriptionRepo.On("GetByTeamID", mock.Anything, team1ID).Return(&models.TeamSubscription{
		TeamID:    team1ID,
		Tier:      models.TeamTierStarter,
		SeatCount: 3,
		Status:    models.TeamSubscriptionStatusActive,
	}, nil)

	// Team 2: Professional with 5 seats -> 5000 + (500 * 5) = 7500 prompts (PRD Section 9.2)
	teamSubscriptionRepo.On("GetByTeamID", mock.Anything, team2ID).Return(&models.TeamSubscription{
		TeamID:    team2ID,
		Tier:      models.TeamTierProfessional,
		SeatCount: 5,
		Status:    models.TeamSubscriptionStatusActive,
	}, nil)

	// Execute
	quota, err := service.getTeamQuotaContribution(context.Background(), userID, events.ResourceTypePrompt)

	// Assert - should aggregate quotas from both teams
	assert.NoError(t, err)
	assert.Equal(t, 8800, quota) // 1300 + 7500

	// Verify mocks
	teamMemberRepo.AssertExpectations(t)
	teamRepo.AssertExpectations(t)
	teamSubscriptionRepo.AssertExpectations(t)
}

// TestGetTeamQuotaContribution_PersonalWorkspaceExcluded tests that personal workspaces are excluded
//
//nolint:funlen // Detailed test with multiple mock configurations
func TestGetTeamQuotaContribution_PersonalWorkspaceExcluded(t *testing.T) {
	// Setup mocks
	userRepo := new(mockUserRepo)
	promptRepo := new(mockPromptRepo)
	artifactRepo := new(mockArtifactRepo)
	memoryRepo := new(mockMemoryRepo)
	agentRepo := new(mockAgentRepo)
	agentExecRepo := new(mockAgentExecRepo)
	claudeCodeRepo := new(mockClaudeCodeRepo)
	cursorIDERepo := new(mockCursorIDERepo)
	specLibraryRepo := new(mockSpecLibraryRepo)
	teamRepo := new(mockTeamRepo)
	teamMemberRepo := new(mockTeamMemberRepo)
	teamSubscriptionRepo := new(mockTeamSubscriptionRepo)
	logger := slog.New(slog.DiscardHandler)

	// Setup service
	service := NewResourceUsageService(
		userRepo,
		promptRepo,
		artifactRepo,
		memoryRepo,
		agentRepo,
		agentExecRepo,
		claudeCodeRepo,
		cursorIDERepo,
		specLibraryRepo,
		teamRepo,
		teamMemberRepo,
		teamSubscriptionRepo,
		new(mockFeedRepo),
		new(mockFeedItemRepo),
		new(mockFeedItemReplyRepo),
		logger,
	)

	// Setup test data
	userID := "test-user-id"
	personalTeamID := "personal-team"
	regularTeamID := "regular-team"

	// Mock team memberships
	teamMemberRepo.On("GetByUserID", mock.Anything, userID).Return([]models.TeamMember{
		{TeamID: personalTeamID, UserID: userID, Role: models.TeamMemberRoleOwner},
		{TeamID: regularTeamID, UserID: userID, Role: models.TeamMemberRoleMember},
	}, nil)

	// Mock teams - one personal, one regular
	teamRepo.On("GetByID", mock.Anything, personalTeamID).Return(&models.Team{
		ID:         personalTeamID,
		IsPersonal: true, // Personal workspace
	}, nil)
	teamRepo.On("GetByID", mock.Anything, regularTeamID).Return(&models.Team{
		ID:         regularTeamID,
		IsPersonal: false, // Regular team
	}, nil)

	// Mock team subscription only for regular team
	teamSubscriptionRepo.On("GetByTeamID", mock.Anything, regularTeamID).Return(&models.TeamSubscription{
		TeamID:    regularTeamID,
		Tier:      models.TeamTierStarter,
		SeatCount: 3,
		Status:    models.TeamSubscriptionStatusActive,
	}, nil)

	// Execute
	quota, err := service.getTeamQuotaContribution(context.Background(), userID, events.ResourceTypePrompt)

	// Assert - should only count regular team, not personal workspace
	assert.NoError(t, err)
	assert.Equal(t, 1300, quota) // Only from regular team: 1000 + (100 * 3) - PRD Section 9.2

	// Verify mocks
	teamMemberRepo.AssertExpectations(t)
	teamRepo.AssertExpectations(t)
	teamSubscriptionRepo.AssertExpectations(t)
	// Note: teamSubscriptionRepo.GetByTeamID should NOT be called for personal team
}

// TestGetTeamQuotaContribution_InactiveSubscriptionExcluded tests that inactive subscriptions are excluded
//
//nolint:funlen // Comprehensive test with active and inactive subscription scenarios
func TestGetTeamQuotaContribution_InactiveSubscriptionExcluded(t *testing.T) {
	// Setup mocks
	userRepo := new(mockUserRepo)
	promptRepo := new(mockPromptRepo)
	artifactRepo := new(mockArtifactRepo)
	memoryRepo := new(mockMemoryRepo)
	agentRepo := new(mockAgentRepo)
	agentExecRepo := new(mockAgentExecRepo)
	claudeCodeRepo := new(mockClaudeCodeRepo)
	cursorIDERepo := new(mockCursorIDERepo)
	specLibraryRepo := new(mockSpecLibraryRepo)
	teamRepo := new(mockTeamRepo)
	teamMemberRepo := new(mockTeamMemberRepo)
	teamSubscriptionRepo := new(mockTeamSubscriptionRepo)
	logger := slog.New(slog.DiscardHandler)

	// Setup service
	service := NewResourceUsageService(
		userRepo,
		promptRepo,
		artifactRepo,
		memoryRepo,
		agentRepo,
		agentExecRepo,
		claudeCodeRepo,
		cursorIDERepo,
		specLibraryRepo,
		teamRepo,
		teamMemberRepo,
		teamSubscriptionRepo,
		new(mockFeedRepo),
		new(mockFeedItemRepo),
		new(mockFeedItemReplyRepo),
		logger,
	)

	// Setup test data
	userID := "test-user-id"
	inactiveTeamID := "inactive-team"
	activeTeamID := "active-team"

	// Mock team memberships
	teamMemberRepo.On("GetByUserID", mock.Anything, userID).Return([]models.TeamMember{
		{TeamID: inactiveTeamID, UserID: userID, Role: models.TeamMemberRoleMember},
		{TeamID: activeTeamID, UserID: userID, Role: models.TeamMemberRoleMember},
	}, nil)

	// Mock teams (both not personal)
	teamRepo.On("GetByID", mock.Anything, inactiveTeamID).Return(&models.Team{
		ID:         inactiveTeamID,
		IsPersonal: false,
	}, nil)
	teamRepo.On("GetByID", mock.Anything, activeTeamID).Return(&models.Team{
		ID:         activeTeamID,
		IsPersonal: false,
	}, nil)

	// Mock team subscriptions - one canceled, one active
	teamSubscriptionRepo.On("GetByTeamID", mock.Anything, inactiveTeamID).Return(&models.TeamSubscription{
		TeamID:    inactiveTeamID,
		Tier:      models.TeamTierProfessional,
		SeatCount: 5,
		Status:    models.TeamSubscriptionStatusCanceled, // Inactive
	}, nil)
	teamSubscriptionRepo.On("GetByTeamID", mock.Anything, activeTeamID).Return(&models.TeamSubscription{
		TeamID:    activeTeamID,
		Tier:      models.TeamTierStarter,
		SeatCount: 3,
		Status:    models.TeamSubscriptionStatusActive, // Active
	}, nil)

	// Execute
	quota, err := service.getTeamQuotaContribution(context.Background(), userID, events.ResourceTypePrompt)

	// Assert - should only count active subscription
	assert.NoError(t, err)
	assert.Equal(t, 1300, quota) // Only from active team: 1000 + (100 * 3) - PRD Section 9.2

	// Verify mocks
	teamMemberRepo.AssertExpectations(t)
	teamRepo.AssertExpectations(t)
	teamSubscriptionRepo.AssertExpectations(t)
}

// TestCheckResourceLimit_WithTeamQuota tests quota aggregation (individual + team)
//
// TestGetResourceUsage_WithTeamQuotaBreakdown tests that GetResourceUsage returns quota breakdown
//
//nolint:funlen // Table-driven test with multiple quota aggregation scenarios
//nolint:funlen // Multiple resource types require extensive mock setup
func TestGetResourceUsage_WithTeamQuotaBreakdown(t *testing.T) {
	// Setup mocks
	userRepo := new(mockUserRepo)
	promptRepo := new(mockPromptRepo)
	artifactRepo := new(mockArtifactRepo)
	memoryRepo := new(mockMemoryRepo)
	agentRepo := new(mockAgentRepo)
	agentExecRepo := new(mockAgentExecRepo)
	claudeCodeRepo := new(mockClaudeCodeRepo)
	cursorIDERepo := new(mockCursorIDERepo)
	specLibraryRepo := new(mockSpecLibraryRepo)
	teamRepo := new(mockTeamRepo)
	teamMemberRepo := new(mockTeamMemberRepo)
	teamSubscriptionRepo := new(mockTeamSubscriptionRepo)
	logger := slog.New(slog.DiscardHandler)

	feedRepo2 := new(mockFeedRepo)
	feedItemRepo2 := new(mockFeedItemRepo)
	feedItemReplyRepo2 := new(mockFeedItemReplyRepo)

	// Setup service
	service := NewResourceUsageService(
		userRepo,
		promptRepo,
		artifactRepo,
		memoryRepo,
		agentRepo,
		agentExecRepo,
		claudeCodeRepo,
		cursorIDERepo,
		specLibraryRepo,
		teamRepo,
		teamMemberRepo,
		teamSubscriptionRepo,
		feedRepo2,
		feedItemRepo2,
		feedItemReplyRepo2,
		logger,
	)

	// Setup test data
	userID := "test-user-id"
	teamID := "team-123"
	individualPlan := models.PlanPro
	user := &models.User{
		ID:               userID,
		SubscriptionPlan: &individualPlan,
	}

	// Mock user repository
	userRepo.On("GetByID", mock.Anything, userID).Return(user, nil)

	// Mock all resource counts
	promptRepo.On("CountByStatus", mock.Anything, userID, "draft").Return(50, nil)
	promptRepo.On("CountByStatus", mock.Anything, userID, "published").Return(50, nil)
	artifactRepo.On("CountAll", mock.Anything, userID).Return(100, nil)
	memoryRepo.On("CountAll", mock.Anything, userID).Return(50, nil)
	specLibraryRepo.On("GetStats", mock.Anything, userID).Return(
		&models.BlueprintStatsResponse{TotalBlueprints: 20}, nil,
	)
	agentRepo.On("GetStats", mock.Anything, userID, "").Return(&models.AgentStatsResponse{TotalAgents: 3}, nil)
	conversations := []models.ConversationSummary{
		{ConversationID: "conv-1", AgentID: "agent-1", MessageCount: 5},
	}
	agentExecRepo.On("ListConversations", mock.Anything, userID, "", 1, 100).Return(conversations, 1, nil)
	claudeCodeRepo.On("CountUniqueSessions", mock.Anything, userID).Return(1, nil)
	cursorIDERepo.On("CountUniqueSessions", mock.Anything, userID).Return(1, nil)
	teamRepo.On("CountByOwnerID", mock.Anything, userID).Return(1, nil)

	// Mock team membership (called multiple times, once per resource type)
	teamMemberRepo.On("GetByUserID", mock.Anything, userID).Return([]models.TeamMember{
		{TeamID: teamID, UserID: userID, Role: models.TeamMemberRoleMember},
	}, nil)

	// Mock team (not personal) (called multiple times)
	teamRepo.On("GetByID", mock.Anything, teamID).Return(&models.Team{
		ID:         teamID,
		IsPersonal: false,
	}, nil)

	// Mock team subscription (called multiple times)
	teamSubscriptionRepo.On("GetByTeamID", mock.Anything, teamID).Return(&models.TeamSubscription{
		TeamID:    teamID,
		Tier:      models.TeamTierStarter,
		SeatCount: 3,
		Status:    models.TeamSubscriptionStatusActive,
	}, nil)

	// Mock feed repos (new resource types now included in GetResourceUsage)
	feedRepo2.On("CountAll", mock.Anything, userID).Return(0, nil)
	feedItemRepo2.On("CountAll", mock.Anything, userID).Return(0, nil)
	feedItemReplyRepo2.On("CountAll", mock.Anything, userID).Return(0, nil)

	// Execute
	response, err := service.GetResourceUsage(context.Background(), userID)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, response)

	// Find prompt resource in response
	var promptResource *models.ResourceUsageItem
	for i := range response.Resources {
		if response.Resources[i].ResourceType == events.ResourceTypePrompt {
			promptResource = &response.Resources[i]
			break
		}
	}

	// Verify quota breakdown for prompts
	assert.NotNil(t, promptResource)
	assert.Equal(t, 100, promptResource.Count)           // 50 draft + 50 published
	assert.Equal(t, 500, promptResource.IndividualLimit) // Pro plan
	assert.Equal(t, 1300, promptResource.TeamQuota)      // 1000 base + (100 * 3) - PRD Section 9.2
	assert.Equal(t, 1800, promptResource.Limit)          // Total: 500 + 1300

	// Verify mocks
	userRepo.AssertExpectations(t)
	promptRepo.AssertExpectations(t)
	teamMemberRepo.AssertExpectations(t)
	teamRepo.AssertExpectations(t)
	teamSubscriptionRepo.AssertExpectations(t)
	feedRepo2.AssertExpectations(t)
	feedItemRepo2.AssertExpectations(t)
	feedItemReplyRepo2.AssertExpectations(t)
}

// TestGetTeamQuotaContribution_OverflowProtection tests overflow protection with realistic and extreme values
//
//nolint:funlen // Table-driven test with multiple overflow scenarios
func TestGetTeamQuotaContribution_OverflowProtection(t *testing.T) {
	tests := []struct {
		name          string
		tier          string
		seatCount     int
		resourceType  string
		expectError   bool
		expectedQuota int
	}{
		{
			name:          "Maximum realistic values - 1000 seats × 500 per-seat bonus (Professional)",
			tier:          models.TeamTierProfessional,
			seatCount:     1000,
			resourceType:  events.ResourceTypePrompt,
			expectError:   false,
			expectedQuota: 505000, // 5000 base + (500 * 1000) - PRD Section 9.2
		},
		{
			name:          "Very large team - 10000 seats (Professional)",
			tier:          models.TeamTierProfessional,
			seatCount:     10000,
			resourceType:  events.ResourceTypePrompt,
			expectError:   false,
			expectedQuota: 5005000, // 5000 base + (500 * 10000) - PRD Section 9.2
		},
		{
			name:          "Zero seats should work",
			tier:          models.TeamTierStarter,
			seatCount:     0,
			resourceType:  events.ResourceTypePrompt,
			expectError:   false,
			expectedQuota: 1000, // Just base quota - PRD Section 9.2
		},
		{
			name:          "Enterprise tier returns unlimited",
			tier:          models.TeamTierEnterprise,
			seatCount:     1000,
			resourceType:  events.ResourceTypePrompt,
			expectError:   false,
			expectedQuota: -1, // Unlimited - PRD Section 9.2
		},
		{
			name:         "Overflow scenario - seats approaching INT_MAX",
			tier:         models.TeamTierProfessional,
			seatCount:    (int(^uint(0)>>1) / 500) + 1, // Would overflow when multiplied by 500
			resourceType: events.ResourceTypePrompt,
			expectError:  true, // Should detect overflow and return error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			userRepo := new(mockUserRepo)
			promptRepo := new(mockPromptRepo)
			artifactRepo := new(mockArtifactRepo)
			memoryRepo := new(mockMemoryRepo)
			agentRepo := new(mockAgentRepo)
			agentExecRepo := new(mockAgentExecRepo)
			claudeCodeRepo := new(mockClaudeCodeRepo)
			cursorIDERepo := new(mockCursorIDERepo)
			specLibraryRepo := new(mockSpecLibraryRepo)
			teamRepo := new(mockTeamRepo)
			teamMemberRepo := new(mockTeamMemberRepo)
			teamSubscriptionRepo := new(mockTeamSubscriptionRepo)
			logger := slog.New(slog.DiscardHandler)

			// Setup service
			service := NewResourceUsageService(
				userRepo,
				promptRepo,
				artifactRepo,
				memoryRepo,
				agentRepo,
				agentExecRepo,
				claudeCodeRepo,
				cursorIDERepo,
				specLibraryRepo,
				teamRepo,
				teamMemberRepo,
				teamSubscriptionRepo,
				new(mockFeedRepo),
				new(mockFeedItemRepo),
				new(mockFeedItemReplyRepo),
				logger,
			)

			// Setup test data
			userID := "test-user-id"
			teamID := "team-123"

			// Mock team membership
			teamMemberRepo.On("GetByUserID", mock.Anything, userID).Return([]models.TeamMember{
				{TeamID: teamID, UserID: userID, Role: models.TeamMemberRoleMember},
			}, nil)

			// Mock team (not personal workspace)
			teamRepo.On("GetByID", mock.Anything, teamID).Return(&models.Team{
				ID:         teamID,
				IsPersonal: false,
			}, nil)

			// Mock team subscription
			teamSubscriptionRepo.On("GetByTeamID", mock.Anything, teamID).Return(&models.TeamSubscription{
				TeamID:    teamID,
				Tier:      tt.tier,
				SeatCount: tt.seatCount,
				Status:    models.TeamSubscriptionStatusActive,
			}, nil)

			// Execute
			quota, err := service.getTeamQuotaContribution(context.Background(), userID, tt.resourceType)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "overflow")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedQuota, quota)
			}

			// Verify mocks
			teamMemberRepo.AssertExpectations(t)
			teamRepo.AssertExpectations(t)
			teamSubscriptionRepo.AssertExpectations(t)
		})
	}
}

// TestGetTeamQuotaContribution_AccumulationOverflow tests multiple teams accumulating quotas without overflow
//
//nolint:funlen // Comprehensive test with multiple team setups
func TestGetTeamQuotaContribution_AccumulationOverflow(t *testing.T) {
	// Setup mocks
	userRepo := new(mockUserRepo)
	promptRepo := new(mockPromptRepo)
	artifactRepo := new(mockArtifactRepo)
	memoryRepo := new(mockMemoryRepo)
	agentRepo := new(mockAgentRepo)
	agentExecRepo := new(mockAgentExecRepo)
	claudeCodeRepo := new(mockClaudeCodeRepo)
	cursorIDERepo := new(mockCursorIDERepo)
	specLibraryRepo := new(mockSpecLibraryRepo)
	teamRepo := new(mockTeamRepo)
	teamMemberRepo := new(mockTeamMemberRepo)
	teamSubscriptionRepo := new(mockTeamSubscriptionRepo)
	logger := slog.New(slog.DiscardHandler)

	// Setup service
	service := NewResourceUsageService(
		userRepo,
		promptRepo,
		artifactRepo,
		memoryRepo,
		agentRepo,
		agentExecRepo,
		claudeCodeRepo,
		cursorIDERepo,
		specLibraryRepo,
		teamRepo,
		teamMemberRepo,
		teamSubscriptionRepo,
		new(mockFeedRepo),
		new(mockFeedItemRepo),
		new(mockFeedItemReplyRepo),
		logger,
	)

	// Setup test data - user in 3 large teams
	userID := "test-user-id"
	team1ID := "team-1"
	team2ID := "team-2"
	team3ID := "team-3"

	// Mock team memberships
	teamMemberRepo.On("GetByUserID", mock.Anything, userID).Return([]models.TeamMember{
		{TeamID: team1ID, UserID: userID, Role: models.TeamMemberRoleMember},
		{TeamID: team2ID, UserID: userID, Role: models.TeamMemberRoleMember},
		{TeamID: team3ID, UserID: userID, Role: models.TeamMemberRoleMember},
	}, nil)

	// Mock teams (all not personal workspaces)
	teamRepo.On("GetByID", mock.Anything, team1ID).Return(&models.Team{
		ID:         team1ID,
		IsPersonal: false,
	}, nil)
	teamRepo.On("GetByID", mock.Anything, team2ID).Return(&models.Team{
		ID:         team2ID,
		IsPersonal: false,
	}, nil)
	teamRepo.On("GetByID", mock.Anything, team3ID).Return(&models.Team{
		ID:         team3ID,
		IsPersonal: false,
	}, nil)

	// Mock team subscriptions - all large teams
	// Team 1: Professional with 1000 seats -> 5000 + (500 * 1000) = 505000
	teamSubscriptionRepo.On("GetByTeamID", mock.Anything, team1ID).Return(&models.TeamSubscription{
		TeamID:    team1ID,
		Tier:      models.TeamTierProfessional,
		SeatCount: 1000,
		Status:    models.TeamSubscriptionStatusActive,
	}, nil)

	// Team 2: Professional with 500 seats -> 5000 + (500 * 500) = 255000
	teamSubscriptionRepo.On("GetByTeamID", mock.Anything, team2ID).Return(&models.TeamSubscription{
		TeamID:    team2ID,
		Tier:      models.TeamTierProfessional,
		SeatCount: 500,
		Status:    models.TeamSubscriptionStatusActive,
	}, nil)

	// Team 3: Starter with 300 seats -> 1000 + (100 * 300) = 31000
	teamSubscriptionRepo.On("GetByTeamID", mock.Anything, team3ID).Return(&models.TeamSubscription{
		TeamID:    team3ID,
		Tier:      models.TeamTierStarter,
		SeatCount: 300,
		Status:    models.TeamSubscriptionStatusActive,
	}, nil)

	// Execute
	quota, err := service.getTeamQuotaContribution(context.Background(), userID, events.ResourceTypePrompt)

	// Assert - should successfully accumulate without overflow
	assert.NoError(t, err)
	assert.Equal(t, 791000, quota) // 505000 + 255000 + 31000 - PRD Section 9.2

	// Verify mocks
	teamMemberRepo.AssertExpectations(t)
	teamRepo.AssertExpectations(t)
	teamSubscriptionRepo.AssertExpectations(t)
}

// TestGetTeamQuotaContribution_NegativeValueValidation tests that negative values are handled gracefully
//
//nolint:funlen // Table-driven test with multiple edge cases
func TestGetTeamQuotaContribution_NegativeValueValidation(t *testing.T) {
	tests := []struct {
		name          string
		tier          string
		seatCount     int
		resourceType  string
		expectError   bool
		expectedQuota int
	}{
		{
			name:          "Negative seat count results in reduced quota",
			tier:          models.TeamTierStarter,
			seatCount:     -5,
			resourceType:  events.ResourceTypePrompt,
			expectError:   false,
			expectedQuota: 500, // 1000 base + (100 * -5) = 500 - PRD Section 9.2
		},
		{
			name:          "Zero seat count is valid",
			tier:          models.TeamTierStarter,
			seatCount:     0,
			resourceType:  events.ResourceTypePrompt,
			expectError:   false,
			expectedQuota: 1000, // Just base quota - PRD Section 9.2
		},
		{
			name:          "Positive seat count is normal",
			tier:          models.TeamTierProfessional,
			seatCount:     10,
			resourceType:  events.ResourceTypePrompt,
			expectError:   false,
			expectedQuota: 10000, // 5000 base + (500 * 10) - PRD Section 9.2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			userRepo := new(mockUserRepo)
			promptRepo := new(mockPromptRepo)
			artifactRepo := new(mockArtifactRepo)
			memoryRepo := new(mockMemoryRepo)
			agentRepo := new(mockAgentRepo)
			agentExecRepo := new(mockAgentExecRepo)
			claudeCodeRepo := new(mockClaudeCodeRepo)
			cursorIDERepo := new(mockCursorIDERepo)
			specLibraryRepo := new(mockSpecLibraryRepo)
			teamRepo := new(mockTeamRepo)
			teamMemberRepo := new(mockTeamMemberRepo)
			teamSubscriptionRepo := new(mockTeamSubscriptionRepo)
			logger := slog.New(slog.DiscardHandler)

			// Setup service
			service := NewResourceUsageService(
				userRepo,
				promptRepo,
				artifactRepo,
				memoryRepo,
				agentRepo,
				agentExecRepo,
				claudeCodeRepo,
				cursorIDERepo,
				specLibraryRepo,
				teamRepo,
				teamMemberRepo,
				teamSubscriptionRepo,
				new(mockFeedRepo),
				new(mockFeedItemRepo),
				new(mockFeedItemReplyRepo),
				logger,
			)

			// Setup test data
			userID := "test-user-id"
			teamID := "team-123"

			// Mock team membership
			teamMemberRepo.On("GetByUserID", mock.Anything, userID).Return([]models.TeamMember{
				{TeamID: teamID, UserID: userID, Role: models.TeamMemberRoleMember},
			}, nil)

			// Mock team (not personal workspace)
			teamRepo.On("GetByID", mock.Anything, teamID).Return(&models.Team{
				ID:         teamID,
				IsPersonal: false,
			}, nil)

			// Mock team subscription
			teamSubscriptionRepo.On("GetByTeamID", mock.Anything, teamID).Return(&models.TeamSubscription{
				TeamID:    teamID,
				Tier:      tt.tier,
				SeatCount: tt.seatCount,
				Status:    models.TeamSubscriptionStatusActive,
			}, nil)

			// Execute
			quota, err := service.getTeamQuotaContribution(context.Background(), userID, tt.resourceType)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedQuota, quota)
			}

			// Verify mocks
			teamMemberRepo.AssertExpectations(t)
			teamRepo.AssertExpectations(t)
			teamSubscriptionRepo.AssertExpectations(t)
		})
	}
}

// TestCountFeeds verifies that countFeeds delegates to FeedRepository.CountAll correctly.
func TestCountFeeds(t *testing.T) {
	feedRepo := new(mockFeedRepo)
	logger := slog.New(slog.DiscardHandler)

	service := NewResourceUsageService(
		new(mockUserRepo),
		new(mockPromptRepo),
		new(mockArtifactRepo),
		new(mockMemoryRepo),
		new(mockAgentRepo),
		new(mockAgentExecRepo),
		new(mockClaudeCodeRepo),
		new(mockCursorIDERepo),
		new(mockSpecLibraryRepo),
		new(mockTeamRepo),
		nil, // teamMemberRepo
		nil, // teamSubscriptionRepo
		feedRepo,
		new(mockFeedItemRepo),
		new(mockFeedItemReplyRepo),
		logger,
	)

	userID := "user-test"
	expectedCount := 7

	feedRepo.On("CountAll", mock.Anything, userID).Return(expectedCount, nil)

	count, err := service.countFeeds(context.Background(), userID)

	assert.NoError(t, err)
	assert.Equal(t, expectedCount, count)
	feedRepo.AssertExpectations(t)
}

// TestCountFeedItems verifies that countFeedItems sums feed items and replies correctly.
func TestCountFeedItems(t *testing.T) {
	feedItemRepo := new(mockFeedItemRepo)
	feedItemReplyRepo := new(mockFeedItemReplyRepo)
	logger := slog.New(slog.DiscardHandler)

	service := NewResourceUsageService(
		new(mockUserRepo),
		new(mockPromptRepo),
		new(mockArtifactRepo),
		new(mockMemoryRepo),
		new(mockAgentRepo),
		new(mockAgentExecRepo),
		new(mockClaudeCodeRepo),
		new(mockCursorIDERepo),
		new(mockSpecLibraryRepo),
		new(mockTeamRepo),
		nil, // teamMemberRepo
		nil, // teamSubscriptionRepo
		new(mockFeedRepo),
		feedItemRepo,
		feedItemReplyRepo,
		logger,
	)

	userID := "user-test"
	itemCount := 60
	replyCount := 40

	feedItemRepo.On("CountAll", mock.Anything, userID).Return(itemCount, nil)
	feedItemReplyRepo.On("CountAll", mock.Anything, userID).Return(replyCount, nil)

	count, err := service.countFeedItems(context.Background(), userID)

	assert.NoError(t, err)
	assert.Equal(t, 100, count, "expected items (%d) + replies (%d) = 100", itemCount, replyCount)
	feedItemRepo.AssertExpectations(t)
	feedItemReplyRepo.AssertExpectations(t)
}

// TestCountFeedItems_ItemRepoError verifies that an error in the feed item repo is returned.
func TestCountFeedItems_ItemRepoError(t *testing.T) {
	feedItemRepo := new(mockFeedItemRepo)
	feedItemReplyRepo := new(mockFeedItemReplyRepo)
	logger := slog.New(slog.DiscardHandler)

	service := NewResourceUsageService(
		new(mockUserRepo),
		new(mockPromptRepo),
		new(mockArtifactRepo),
		new(mockMemoryRepo),
		new(mockAgentRepo),
		new(mockAgentExecRepo),
		new(mockClaudeCodeRepo),
		new(mockCursorIDERepo),
		new(mockSpecLibraryRepo),
		new(mockTeamRepo),
		nil, // teamMemberRepo
		nil, // teamSubscriptionRepo
		new(mockFeedRepo),
		feedItemRepo,
		feedItemReplyRepo,
		logger,
	)

	userID := "user-test"

	feedItemRepo.On("CountAll", mock.Anything, userID).Return(0, fmt.Errorf("db error"))

	_, err := service.countFeedItems(context.Background(), userID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to count feed items")
	feedItemRepo.AssertExpectations(t)
}

// TestCountFeedItems_ReplyRepoError verifies that an error in the reply repo is returned.
func TestCountFeedItems_ReplyRepoError(t *testing.T) {
	feedItemRepo := new(mockFeedItemRepo)
	feedItemReplyRepo := new(mockFeedItemReplyRepo)
	logger := slog.New(slog.DiscardHandler)

	service := NewResourceUsageService(
		new(mockUserRepo),
		new(mockPromptRepo),
		new(mockArtifactRepo),
		new(mockMemoryRepo),
		new(mockAgentRepo),
		new(mockAgentExecRepo),
		new(mockClaudeCodeRepo),
		new(mockCursorIDERepo),
		new(mockSpecLibraryRepo),
		new(mockTeamRepo),
		nil, // teamMemberRepo
		nil, // teamSubscriptionRepo
		new(mockFeedRepo),
		feedItemRepo,
		feedItemReplyRepo,
		logger,
	)

	userID := "user-test"

	feedItemRepo.On("CountAll", mock.Anything, userID).Return(10, nil)
	feedItemReplyRepo.On("CountAll", mock.Anything, userID).Return(0, fmt.Errorf("db error"))

	_, err := service.countFeedItems(context.Background(), userID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to count feed item replies")
	feedItemRepo.AssertExpectations(t)
	feedItemReplyRepo.AssertExpectations(t)
}
