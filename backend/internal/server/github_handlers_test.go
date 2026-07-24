package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	"github.com/vibexp/vibexp/internal/specconformance"
)

const githubTestUserID = "user-github-123"
const githubTestTeamID = "550e8400-e29b-41d4-a716-446655440001"

// githubTestInstallCode is the OAuth authorization code the install-callback
// tests submit. The handler only requires it to be non-empty; proving the
// caller's authority with it is the service's job (#463).
const githubTestInstallCode = "gh-install-code"

// =============================================================================
// Mock Activity Service for tracking calls
// =============================================================================

// trackingActivityService is a test implementation that records RecordResourceActivity calls
type trackingActivityService struct {
	recordedActivities []recordedActivity
}

type recordedActivity struct {
	userID       string
	activityType string
	entityType   string
	entityID     *string
	description  string
	metadata     map[string]interface{}
}

func (t *trackingActivityService) RecordActivity(
	ctx context.Context, userID string, req activities.CreateActivityRequest,
) (*activities.Activity, error) {
	return &activities.Activity{}, nil
}

func (t *trackingActivityService) RecordAuthActivity(
	ctx context.Context, userID, activityType string, sessionID *string,
	metadata map[string]interface{}, sourceIP, userAgent *string,
) error {
	return nil
}

func (t *trackingActivityService) RecordResourceActivity(
	ctx context.Context, userID, activityType, entityType string,
	entityID *string, description string, metadata map[string]interface{},
) error {
	t.recordedActivities = append(t.recordedActivities, recordedActivity{
		userID:       userID,
		activityType: activityType,
		entityType:   entityType,
		entityID:     entityID,
		description:  description,
		metadata:     metadata,
	})
	return nil
}

func (t *trackingActivityService) RecordClaudeCodeActivity(
	ctx context.Context, userID, sessionID string, toolName *string,
	hookEventName string, metadata map[string]interface{},
) error {
	return nil
}

func (t *trackingActivityService) GetActivities(
	ctx context.Context, filters activities.ActivityFilters,
) (*activities.ActivityListResponse, error) {
	return &activities.ActivityListResponse{}, nil
}

func (t *trackingActivityService) GetActivityByID(
	ctx context.Context, userID, activityID string,
) (*activities.Activity, error) {
	return nil, nil
}

func (t *trackingActivityService) GetActivityStats(
	ctx context.Context, userID string,
) (*activities.ActivityStatsResponse, error) {
	return &activities.ActivityStatsResponse{}, nil
}

func (t *trackingActivityService) GetAllTypes() *activities.ActivityTypesResponse {
	return &activities.ActivityTypesResponse{}
}

func (t *trackingActivityService) DeleteActivity(ctx context.Context, activityID string) error {
	return nil
}

func (t *trackingActivityService) GetActivityTypes() []string              { return []string{} }
func (t *trackingActivityService) GetEntityTypes() []string                { return []string{} }
func (t *trackingActivityService) RunRetentionJob(_ context.Context) error { return nil }

// =============================================================================
// GitHub Test Container
// =============================================================================

// GitHubTestContainer is a test container with GitHub and Activity services
type GitHubTestContainer struct {
	BaseMockContainer
	mock.Mock
	gitHubAppService *svcmocks.MockGitHubAppServiceInterface
	activitySvc      activities.ActivityService
	// authzService gates handler-level permission checks (install-url, #463).
	// BaseMockContainer returns nil, which would panic the moment a handler
	// consults it, so tests that reach one must set this.
	authzService services.AuthorizationServiceInterface
}

func (c *GitHubTestContainer) GitHubAppService() services.GitHubAppServiceInterface {
	return c.gitHubAppService
}

func (c *GitHubTestContainer) AuthorizationService() services.AuthorizationServiceInterface {
	return c.authzService
}
func (c *GitHubTestContainer) ActivityService() activities.ActivityService { return c.activitySvc }
func (c *GitHubTestContainer) ResourceAccessService() resourceaccess.ResourceAccessService {
	return nil
}

// Stub implementations for all other container methods
func (c *GitHubTestContainer) AuthService() services.AuthServiceInterface     { return nil }
func (c *GitHubTestContainer) APIKeyService() services.APIKeyServiceInterface { return nil }
func (c *GitHubTestContainer) PromptService() services.PromptServiceInterface { return nil }
func (c *GitHubTestContainer) PromptGalleryService() services.PromptGalleryServiceInterface {
	return nil
}
func (c *GitHubTestContainer) PromptShareService() services.PromptShareServiceInterface { return nil }
func (c *GitHubTestContainer) ArtifactService() services.ArtifactServiceInterface       { return nil }
func (c *GitHubTestContainer) BlueprintService() services.BlueprintServiceInterface     { return nil }
func (c *GitHubTestContainer) EmbeddingProviderService() services.EmbeddingProviderServiceInterface {
	return nil
}
func (c *GitHubTestContainer) EmailService() services.EmailServiceInterface { return nil }
func (c *GitHubTestContainer) AgentService() services.AgentServiceInterface { return nil }
func (c *GitHubTestContainer) AgentCardFetcher() services.CardFetcher       { return nil }
func (c *GitHubTestContainer) AgentInvocationService() services.AgentInvocationServiceInterface {
	return nil
}
func (c *GitHubTestContainer) MemoryService() services.MemoryServiceInterface       { return nil }
func (c *GitHubTestContainer) EmbeddingService() services.EmbeddingServiceInterface { return nil }
func (c *GitHubTestContainer) SearchService() services.Searcher                     { return nil }
func (c *GitHubTestContainer) EnvironmentService() *services.EnvironmentService     { return nil }
func (c *GitHubTestContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return nil
}
func (c *GitHubTestContainer) BackofficeService() services.UsageAndGrowthGetter { return nil }
func (c *GitHubTestContainer) AdminService() services.AdminServiceInterface     { return nil }
func (c *GitHubTestContainer) EmbeddingBackfillService() services.EmbeddingBackfiller {
	return nil
}
func (c *GitHubTestContainer) TeamService() services.TeamServiceInterface             { return nil }
func (c *GitHubTestContainer) TeamInvitationService() *services.TeamInvitationService { return nil }
func (c *GitHubTestContainer) ProjectService() services.ProjectServiceInterface       { return nil }

// Repository stubs
func (c *GitHubTestContainer) UserRepository() repositories.UserRepository     { return nil }
func (c *GitHubTestContainer) APIKeyRepository() repositories.APIKeyRepository { return nil }
func (c *GitHubTestContainer) PromptRepository() repositories.PromptRepository { return nil }
func (c *GitHubTestContainer) PromptGalleryRepository() repositories.PromptGalleryRepository {
	return nil
}
func (c *GitHubTestContainer) PromptShareRepository() repositories.PromptShareRepository {
	return nil
}
func (c *GitHubTestContainer) ArtifactRepository() repositories.ArtifactRepository   { return nil }
func (c *GitHubTestContainer) BlueprintRepository() repositories.BlueprintRepository { return nil }
func (c *GitHubTestContainer) EmbeddingProviderRepository() repositories.EmbeddingProviderRepository {
	return nil
}
func (c *GitHubTestContainer) ActivityRepository() repositories.ActivityRepository { return nil }
func (c *GitHubTestContainer) ResourceAccessRepository() repositories.ResourceAccessRepository {
	return nil
}
func (c *GitHubTestContainer) ClaudeCodeHooksRepository() repositories.ClaudeCodeHooksRepository {
	return nil
}
func (c *GitHubTestContainer) CursorIDEHooksRepository() repositories.CursorIDEHooksRepository {
	return nil
}
func (c *GitHubTestContainer) AgentRepository() repositories.AgentRepository { return nil }
func (c *GitHubTestContainer) AgentExecutionRepository() repositories.AgentExecutionRepository {
	return nil
}
func (c *GitHubTestContainer) AgentExecutionEventRepository() repositories.AgentExecutionEventRepository {
	return nil
}
func (c *GitHubTestContainer) MemoryRepository() repositories.MemoryRepository         { return nil }
func (c *GitHubTestContainer) EmbeddingRepository() repositories.EmbeddingRepository   { return nil }
func (c *GitHubTestContainer) BackofficeRepository() repositories.BackofficeRepository { return nil }
func (c *GitHubTestContainer) UserPreferencesRepository() repositories.UserPreferencesRepository {
	return nil
}
func (c *GitHubTestContainer) TeamRepository() repositories.TeamRepository             { return nil }
func (c *GitHubTestContainer) TeamMemberRepository() repositories.TeamMemberRepository { return nil }
func (c *GitHubTestContainer) TeamSubscriptionRepository() repositories.TeamSubscriptionRepository {
	return nil
}
func (c *GitHubTestContainer) ProjectRepository() repositories.ProjectRepository { return nil }
func (c *GitHubTestContainer) WebhookEventRepository() repositories.WebhookEventRepository {
	return nil
}
func (c *GitHubTestContainer) GitHubInstallationRepository() repositories.GitHubInstallationRepository {
	return nil
}

func (c *GitHubTestContainer) FeedRepository() repositories.FeedRepository         { return nil }
func (c *GitHubTestContainer) FeedItemRepository() repositories.FeedItemRepository { return nil }
func (c *GitHubTestContainer) FeedItemReplyRepository() repositories.FeedItemReplyRepository {
	return nil
}
func (c *GitHubTestContainer) FeedService() services.FeedServiceInterface         { return nil }
func (c *GitHubTestContainer) FeedItemService() services.FeedItemServiceInterface { return nil }
func (c *GitHubTestContainer) FeedItemReplyService() services.FeedItemReplyServiceInterface {
	return nil
}

// External and infrastructure stubs
func (c *GitHubTestContainer) IdentityProviderRegistry() *idp.Registry { return nil }
func (c *GitHubTestContainer) NotificationDeliveryRepository() repositories.NotificationDeliveryRepository {
	return nil
}
func (c *GitHubTestContainer) NotificationDigestQueueRepository() repositories.NotificationDigestQueueRepository {
	return nil
}
func (c *GitHubTestContainer) NotificationService() notifications.NotificationServiceInterface {
	return nil
}
func (c *GitHubTestContainer) DigestRunner() *notifications.DigestRunner                 { return nil }
func (c *GitHubTestContainer) DeviceTokenRepository() repositories.DeviceTokenRepository { return nil }
func (c *GitHubTestContainer) ProjectMigrationService() services.ProjectMigrationServiceInterface {
	return nil
}

func newGitHubTestContainer(t *testing.T) (*GitHubTestContainer, *trackingActivityService) {
	trackingSvc := &trackingActivityService{}
	c := &GitHubTestContainer{
		gitHubAppService: svcmocks.NewMockGitHubAppServiceInterface(t),
		activitySvc:      trackingSvc,
	}
	return c, trackingSvc
}

func createGitHubTestServer(container *GitHubTestContainer) *Server {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	r := chi.NewRouter()

	srv := &Server{
		port:            "8080",
		container:       container,
		logger:          logger,
		config:          cfg,
		router:          r,
		activityService: container.activitySvc,
	}

	return srv
}

func makeGitHubPOSTAuthRequest(path string, body interface{}) *http.Request {
	var reqBody []byte
	var err error
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			panic(err)
		}
	}

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	ctx := context.WithValue(req.Context(), contextKeyUserID, githubTestUserID)
	return req.WithContext(ctx)
}

func addGitHubChiParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for key, value := range params {
		rctx.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// =============================================================================
// Tests for handleGitHubStatus
// =============================================================================

func TestHandleGitHubStatus_Success(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	installedAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	container.gitHubAppService.On(
		"GetInstallationStatus", mock.Anything, githubTestTeamID,
	).Return(&models.GitHubInstallationStatus{
		Installed:      true,
		AccountLogin:   "my-org",
		InstallationID: 12345678,
		Suspended:      false,
		InstalledAt:    installedAt,
	}, nil)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/"+githubTestTeamID+"/integrations/github/status", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, githubTestUserID))
	req = addGitHubChiParams(req, map[string]string{"team_id": githubTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleGitHubStatus(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	var resp models.GitHubInstallationStatus
	err := json.NewDecoder(rr.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.True(t, resp.Installed)
	assert.Equal(t, "my-org", resp.AccountLogin)

	container.gitHubAppService.AssertExpectations(t)
}

// =============================================================================
// Tests for handleGitHubImportProject
// =============================================================================

func TestHandleGitHubImportProject_SuccessNewProject_RecordsActivity(t *testing.T) {
	container, trackingSvc := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	project := &models.Project{
		ID:        "proj-123",
		UserID:    githubTestUserID,
		TeamID:    githubTestTeamID,
		Name:      "my-repo",
		Slug:      "my-repo",
		GitURL:    "https://github.com/testowner/my-repo",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	container.gitHubAppService.On(
		"ImportProjectFromRepository",
		mock.Anything, githubTestUserID, githubTestTeamID, int64(42),
	).Return(project, true, nil)

	req := makeGitHubPOSTAuthRequest(
		"/api/v1/"+githubTestTeamID+"/integrations/github/repositories/42/import-project", nil)
	req = addGitHubChiParams(req, map[string]string{
		"team_id": githubTestTeamID,
		"repo_id": "42",
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubImportProject(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	// Verify activity was recorded
	assert.Len(t, trackingSvc.recordedActivities, 1)
	recorded := trackingSvc.recordedActivities[0]
	assert.Equal(t, githubTestUserID, recorded.userID)
	assert.Equal(t, activities.ActivityTypeGitHubProjectImported, recorded.activityType)
	assert.Equal(t, activities.EntityTypeProject, recorded.entityType)
	assert.NotNil(t, recorded.entityID)
	assert.Equal(t, "proj-123", *recorded.entityID)
	assert.Contains(t, recorded.description, "my-repo")
	assert.Equal(t, "my-repo", recorded.metadata["repo_name"])
	assert.Equal(t, githubTestTeamID, recorded.metadata["team_id"])
}

func TestHandleGitHubImportProject_ExistingProject_NoActivityRecorded(t *testing.T) {
	container, trackingSvc := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	project := &models.Project{
		ID:        "proj-existing",
		UserID:    githubTestUserID,
		TeamID:    githubTestTeamID,
		Name:      "existing-repo",
		Slug:      "existing-repo",
		GitURL:    "https://github.com/testowner/existing-repo",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// created = false means project already existed
	container.gitHubAppService.On(
		"ImportProjectFromRepository",
		mock.Anything, githubTestUserID, githubTestTeamID, int64(99),
	).Return(project, false, nil)

	req := makeGitHubPOSTAuthRequest(
		"/api/v1/"+githubTestTeamID+"/integrations/github/repositories/99/import-project", nil)
	req = addGitHubChiParams(req, map[string]string{
		"team_id": githubTestTeamID,
		"repo_id": "99",
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubImportProject(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	// No activity should be recorded since project already existed
	assert.Len(t, trackingSvc.recordedActivities, 0)
}

func TestHandleGitHubImportProject_ServiceError_NoActivityRecorded(t *testing.T) {
	container, trackingSvc := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	container.gitHubAppService.On(
		"ImportProjectFromRepository",
		mock.Anything, githubTestUserID, githubTestTeamID, int64(42),
	).Return(nil, false, errors.New("service error"))

	req := makeGitHubPOSTAuthRequest("/api/v1/"+githubTestTeamID+"/github/repos/42/import-project", nil)
	req = addGitHubChiParams(req, map[string]string{
		"team_id": githubTestTeamID,
		"repo_id": "42",
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubImportProject(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	// No activity should be recorded on failure
	assert.Len(t, trackingSvc.recordedActivities, 0)
}

func TestHandleGitHubImportProject_InvalidRepoID_NoActivityRecorded(t *testing.T) {
	container, trackingSvc := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	req := makeGitHubPOSTAuthRequest("/api/v1/"+githubTestTeamID+"/github/repos/invalid/import-project", nil)
	req = addGitHubChiParams(req, map[string]string{
		"team_id": githubTestTeamID,
		"repo_id": "invalid",
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubImportProject(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)

	// No activity should be recorded on bad request
	assert.Len(t, trackingSvc.recordedActivities, 0)
}

func TestHandleGitHubImportProject_InstallationNotFound_NoActivityRecorded(t *testing.T) {
	container, trackingSvc := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	container.gitHubAppService.On(
		"ImportProjectFromRepository",
		mock.Anything, githubTestUserID, githubTestTeamID, int64(42),
	).Return(nil, false, repositories.ErrGitHubInstallationNotFound)

	req := makeGitHubPOSTAuthRequest("/api/v1/"+githubTestTeamID+"/github/repos/42/import-project", nil)
	req = addGitHubChiParams(req, map[string]string{
		"team_id": githubTestTeamID,
		"repo_id": "42",
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubImportProject(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)

	// No activity should be recorded on installation not found
	assert.Len(t, trackingSvc.recordedActivities, 0)
}

// =============================================================================
// Tests for handleGitHubImportBlueprints
// =============================================================================

func TestHandleGitHubImportBlueprints_Success_RecordsActivity(t *testing.T) {
	container, trackingSvc := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	report := &models.BlueprintImportReport{
		TotalScanned:    5,
		TotalSuccessful: 3,
		TotalFailed:     1,
		TotalSkipped:    1,
		SuccessfulItems: []models.BlueprintImportSuccess{
			{FilePath: ".claude/skills/skill1.md", BlueprintID: "bp-1", Title: "Skill 1", Type: "claude-code"},
			{FilePath: ".claude/skills/skill2.md", BlueprintID: "bp-2", Title: "Skill 2", Type: "claude-code"},
			{FilePath: ".claude/skills/skill3.md", BlueprintID: "bp-3", Title: "Skill 3", Type: "claude-code"},
		},
		FailedItems:  []models.BlueprintImportFailed{},
		SkippedItems: []models.BlueprintImportSkipped{},
	}

	container.gitHubAppService.On(
		"ImportBlueprintsFromRepository",
		mock.Anything, githubTestUserID, githubTestTeamID, int64(42),
	).Return(report, nil)

	reqBody := map[string]interface{}{"repository_id": 42}
	req := makeGitHubPOSTAuthRequest(
		"/api/v1/"+githubTestTeamID+"/integrations/github/import-blueprints", reqBody)
	req = addGitHubChiParams(req, map[string]string{
		"team_id": githubTestTeamID,
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubImportBlueprints(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	specconformance.AssertConformsToSpec(t, req, rr)

	// Verify activity was recorded
	assert.Len(t, trackingSvc.recordedActivities, 1)
	recorded := trackingSvc.recordedActivities[0]
	assert.Equal(t, githubTestUserID, recorded.userID)
	assert.Equal(t, activities.ActivityTypeGitHubBlueprintsImported, recorded.activityType)
	assert.Equal(t, activities.EntityTypeBlueprint, recorded.entityType)
	assert.Nil(t, recorded.entityID) // bulk import has no single entity ID
	assert.Contains(t, recorded.description, "3")
	assert.Equal(t, int64(42), recorded.metadata["repo_id"])
	assert.Equal(t, githubTestTeamID, recorded.metadata["team_id"])
	assert.Equal(t, 3, recorded.metadata["total_imported"])
	assert.Equal(t, 1, recorded.metadata["total_skipped"])
	assert.Equal(t, 1, recorded.metadata["total_failed"])
}

// TestHandleGitHubImportBlueprints_ReimportOutcomes conformance-checks the
// extended report shape carrying updated/conflict/up-to-date outcomes (#341).
func TestHandleGitHubImportBlueprints_ReimportOutcomes(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	report := &models.BlueprintImportReport{
		TotalScanned:    3,
		TotalUpdated:    1,
		TotalConflicts:  1,
		TotalUpToDate:   1,
		SuccessfulItems: []models.BlueprintImportSuccess{},
		FailedItems:     []models.BlueprintImportFailed{},
		SkippedItems:    []models.BlueprintImportSkipped{},
		UpdatedItems: []models.BlueprintImportUpdated{
			{FilePath: ".claude/agents/a.md", BlueprintID: "bp-1", Title: "A", Type: "claude-code", Subtype: "sub-agents"},
		},
		ConflictItems: []models.BlueprintImportConflict{
			{FilePath: ".claude/agents/b.md", BlueprintID: "bp-2", Reason: "edited in VibeXP"},
		},
		UpToDateItems: []models.BlueprintImportUpToDate{
			{FilePath: ".claude/agents/c.md", BlueprintID: "bp-3"},
		},
	}
	container.gitHubAppService.On(
		"ImportBlueprintsFromRepository",
		mock.Anything, githubTestUserID, githubTestTeamID, int64(42),
	).Return(report, nil)

	req := makeGitHubPOSTAuthRequest(
		"/api/v1/"+githubTestTeamID+"/integrations/github/import-blueprints",
		map[string]interface{}{"repository_id": 42})
	req = addGitHubChiParams(req, map[string]string{"team_id": githubTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleGitHubImportBlueprints(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"total_updated":1`)
	assert.Contains(t, rr.Body.String(), `"conflict_items"`)
	specconformance.AssertConformsToSpec(t, req, rr)
}

// TestHandleGitHubImportBlueprints_CompanionOutcomes conformance-checks the
// report shape carrying multi-file skill companion outcomes (#342).
func TestHandleGitHubImportBlueprints_CompanionOutcomes(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	report := &models.BlueprintImportReport{
		TotalScanned:            3,
		TotalSuccessful:         1,
		TotalCompanionsImported: 1,
		TotalCompanionsSkipped:  1,
		SuccessfulItems: []models.BlueprintImportSuccess{
			{FilePath: ".claude/skills/deploy/SKILL.md", BlueprintID: "bp-1", Title: "Deploy", Type: "claude-code", Subtype: "skills"},
		},
		FailedItems:  []models.BlueprintImportFailed{},
		SkippedItems: []models.BlueprintImportSkipped{},
		CompanionItems: []models.BlueprintImportCompanion{
			{BlueprintID: "bp-1", RelativePath: "reference/data.json", Outcome: "imported"},
			{BlueprintID: "bp-1", RelativePath: "scripts/helper.py", Outcome: "skipped", Reason: "File type is not allowed"},
		},
	}
	container.gitHubAppService.On(
		"ImportBlueprintsFromRepository",
		mock.Anything, githubTestUserID, githubTestTeamID, int64(42),
	).Return(report, nil)

	req := makeGitHubPOSTAuthRequest(
		"/api/v1/"+githubTestTeamID+"/integrations/github/import-blueprints",
		map[string]interface{}{"repository_id": 42})
	req = addGitHubChiParams(req, map[string]string{"team_id": githubTestTeamID})
	rr := httptest.NewRecorder()

	srv.handleGitHubImportBlueprints(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"total_companions_imported":1`)
	assert.Contains(t, rr.Body.String(), `"companion_items"`)
	specconformance.AssertConformsToSpec(t, req, rr)
}

func TestHandleGitHubImportBlueprints_ServiceError_NoActivityRecorded(t *testing.T) {
	container, trackingSvc := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	container.gitHubAppService.On(
		"ImportBlueprintsFromRepository",
		mock.Anything, githubTestUserID, githubTestTeamID, int64(42),
	).Return(nil, errors.New("service error"))

	reqBody := map[string]interface{}{"repository_id": 42}
	req := makeGitHubPOSTAuthRequest("/api/v1/"+githubTestTeamID+"/github/import-blueprints", reqBody)
	req = addGitHubChiParams(req, map[string]string{
		"team_id": githubTestTeamID,
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubImportBlueprints(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	// No activity should be recorded on failure
	assert.Len(t, trackingSvc.recordedActivities, 0)
}

func TestHandleGitHubImportBlueprints_MissingRepositoryID_NoActivityRecorded(t *testing.T) {
	container, trackingSvc := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	// repository_id is 0 (missing)
	reqBody := map[string]interface{}{}
	req := makeGitHubPOSTAuthRequest("/api/v1/"+githubTestTeamID+"/github/import-blueprints", reqBody)
	req = addGitHubChiParams(req, map[string]string{
		"team_id": githubTestTeamID,
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubImportBlueprints(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)

	// No activity should be recorded on bad request
	assert.Len(t, trackingSvc.recordedActivities, 0)
}

func TestHandleGitHubImportBlueprints_InstallationNotFound_NoActivityRecorded(t *testing.T) {
	container, trackingSvc := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	container.gitHubAppService.On(
		"ImportBlueprintsFromRepository",
		mock.Anything, githubTestUserID, githubTestTeamID, int64(42),
	).Return(nil, repositories.ErrGitHubInstallationNotFound)

	reqBody := map[string]interface{}{"repository_id": 42}
	req := makeGitHubPOSTAuthRequest("/api/v1/"+githubTestTeamID+"/github/import-blueprints", reqBody)
	req = addGitHubChiParams(req, map[string]string{
		"team_id": githubTestTeamID,
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubImportBlueprints(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)

	// No activity on failure
	assert.Len(t, trackingSvc.recordedActivities, 0)
}

func TestHandleGitHubImportBlueprints_ProjectNotFoundForRepo_NoActivityRecorded(t *testing.T) {
	container, trackingSvc := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	container.gitHubAppService.On(
		"ImportBlueprintsFromRepository",
		mock.Anything, githubTestUserID, githubTestTeamID, int64(42),
	).Return(nil, repositories.ErrProjectNotFoundForRepo)

	reqBody := map[string]interface{}{"repository_id": 42}
	req := makeGitHubPOSTAuthRequest("/api/v1/"+githubTestTeamID+"/github/import-blueprints", reqBody)
	req = addGitHubChiParams(req, map[string]string{
		"team_id": githubTestTeamID,
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubImportBlueprints(rr, req)

	assert.Equal(t, http.StatusPreconditionFailed, rr.Code)

	// No activity on failure
	assert.Len(t, trackingSvc.recordedActivities, 0)
}

func TestHandleGitHubImportBlueprints_InvalidJSON_NoActivityRecorded(t *testing.T) {
	container, trackingSvc := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	req := httptest.NewRequest("POST", "/api/v1/"+githubTestTeamID+"/github/import-blueprints",
		bytes.NewBufferString("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")

	ctx := context.WithValue(req.Context(), contextKeyUserID, githubTestUserID)
	req = req.WithContext(ctx)
	req = addGitHubChiParams(req, map[string]string{
		"team_id": githubTestTeamID,
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubImportBlueprints(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)

	// No activity on bad request
	assert.Len(t, trackingSvc.recordedActivities, 0)
}

func TestHandleGitHubImportBlueprints_ZeroSuccessful_NoActivityRecorded(t *testing.T) {
	container, trackingSvc := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	// All blueprints were skipped or failed, none imported successfully
	report := &models.BlueprintImportReport{
		TotalScanned:    3,
		TotalSuccessful: 0,
		TotalFailed:     1,
		TotalSkipped:    2,
		SuccessfulItems: []models.BlueprintImportSuccess{},
		FailedItems:     []models.BlueprintImportFailed{},
		SkippedItems:    []models.BlueprintImportSkipped{},
	}

	container.gitHubAppService.On(
		"ImportBlueprintsFromRepository",
		mock.Anything, githubTestUserID, githubTestTeamID, int64(42),
	).Return(report, nil)

	reqBody := map[string]interface{}{"repository_id": 42}
	req := makeGitHubPOSTAuthRequest("/api/v1/"+githubTestTeamID+"/github/import-blueprints", reqBody)
	req = addGitHubChiParams(req, map[string]string{
		"team_id": githubTestTeamID,
	})
	rr := httptest.NewRecorder()

	srv.handleGitHubImportBlueprints(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	// No activity when zero blueprints were successfully imported
	assert.Len(t, trackingSvc.recordedActivities, 0)
}

// =============================================================================
// Tests for activity constants
// =============================================================================

func TestGitHubActivityTypeConstants(t *testing.T) {
	assert.Equal(t, "github.project_imported", activities.ActivityTypeGitHubProjectImported)
	assert.Equal(t, "github.blueprints_imported", activities.ActivityTypeGitHubBlueprintsImported)
	assert.Equal(t, "project", activities.EntityTypeProject)
}

// =============================================================================
// Tests for handleGitHubRepositories
// =============================================================================

func makeGitHubRepositoriesRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/"+githubTestTeamID+"/github/repositories", nil)
	ctx := context.WithValue(req.Context(), contextKeyUserID, githubTestUserID)
	req = req.WithContext(ctx)
	return addGitHubChiParams(req, map[string]string{"team_id": githubTestTeamID})
}

func TestHandleGitHubRepositories_InstallationGone_Returns404(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	goneErr := fmt.Errorf("failed to get repositories: %w", external.ErrGitHubInstallationGone)
	container.gitHubAppService.On(
		"GetRepositories", mock.Anything, githubTestTeamID, githubTestUserID, 1,
	).Return(nil, goneErr)

	rr := httptest.NewRecorder()
	srv.handleGitHubRepositories(rr, makeGitHubRepositoriesRequest())

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), "github_not_installed")
}

func TestHandleGitHubRepositories_InstallationNotFound_Returns404(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	notFoundErr := fmt.Errorf("failed to get installation: %w", repositories.ErrGitHubInstallationNotFound)
	container.gitHubAppService.On(
		"GetRepositories", mock.Anything, githubTestTeamID, githubTestUserID, 1,
	).Return(nil, notFoundErr)

	rr := httptest.NewRecorder()
	srv.handleGitHubRepositories(rr, makeGitHubRepositoriesRequest())

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), "github_not_installed")
}

func TestHandleGitHubRepositories_OtherError_Returns500(t *testing.T) {
	container, _ := newGitHubTestContainer(t)
	srv := createGitHubTestServer(container)

	container.gitHubAppService.On(
		"GetRepositories", mock.Anything, githubTestTeamID, githubTestUserID, 1,
	).Return(nil, errors.New("github api timeout"))

	rr := httptest.NewRecorder()
	srv.handleGitHubRepositories(rr, makeGitHubRepositoriesRequest())

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "INTERNAL_ERROR")
}

func (c *GitHubTestContainer) TypeService() services.TypeServiceInterface { return nil }

func (c *GitHubTestContainer) AttachmentService() services.AttachmentServiceInterface { return nil }
