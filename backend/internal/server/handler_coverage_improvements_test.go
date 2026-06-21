package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

const coverageTestUserID = "user-123"

// =============================================================================
// Shared Test Container and Helpers
// =============================================================================

// CoverageTestContainer implements Container interface for coverage improvement tests
type CoverageTestContainer struct {
	BaseMockContainer
	mock.Mock
	agentService             *svcmocks.MockAgentServiceInterface
	agentInvocationService   *svcmocks.MockAgentInvocationServiceInterface
	memoryService            *svcmocks.MockMemoryServiceInterface
	teamService              *svcmocks.MockTeamServiceInterface
	resourceUsageService     *svcmocks.MockResourceUsageServiceInterface
	authService              *svcmocks.MockAuthServiceInterface
	embeddingProviderService *svcmocks.MockEmbeddingProviderServiceInterface
	embeddingService         *svcmocks.MockEmbeddingServiceInterface
	promptService            *svcmocks.MockPromptServiceInterface
	artifactService          *svcmocks.MockArtifactServiceInterface
	apiKeyService            *svcmocks.MockAPIKeyServiceInterface
	activityService          activities.ActivityService
	teamMemberRepo           *repomocks.MockTeamMemberRepository
	claudeCodeHooksRepo      *repomocks.MockClaudeCodeHooksRepository
}

// Container interface implementations - Services
func (c *CoverageTestContainer) AgentService() services.AgentServiceInterface { return c.agentService }
func (c *CoverageTestContainer) AgentInvocationService() services.AgentInvocationServiceInterface {
	return c.agentInvocationService
}
func (c *CoverageTestContainer) MemoryService() services.MemoryServiceInterface {
	return c.memoryService
}
func (c *CoverageTestContainer) TeamService() services.TeamServiceInterface { return c.teamService }
func (c *CoverageTestContainer) AuthService() services.AuthServiceInterface { return c.authService }
func (c *CoverageTestContainer) ResourceUsageService() services.ResourceUsageServiceInterface {
	return c.resourceUsageService
}
func (c *CoverageTestContainer) EmbeddingProviderService() services.EmbeddingProviderServiceInterface {
	return c.embeddingProviderService
}
func (c *CoverageTestContainer) EmbeddingService() services.EmbeddingServiceInterface {
	return c.embeddingService
}
func (c *CoverageTestContainer) SearchService() services.SearchServiceInterface {
	return nil
}
func (c *CoverageTestContainer) PromptService() services.PromptServiceInterface {
	return c.promptService
}
func (c *CoverageTestContainer) ArtifactService() services.ArtifactServiceInterface {
	return c.artifactService
}
func (c *CoverageTestContainer) APIKeyService() services.APIKeyServiceInterface {
	return c.apiKeyService
}
func (c *CoverageTestContainer) ActivityService() activities.ActivityService {
	return c.activityService
}
func (c *CoverageTestContainer) ResourceAccessService() resourceaccess.ResourceAccessService {
	return nil
}
func (c *CoverageTestContainer) TeamMemberRepository() repositories.TeamMemberRepository {
	return c.teamMemberRepo
}
func (c *CoverageTestContainer) ClaudeCodeHooksRepository() repositories.ClaudeCodeHooksRepository {
	return c.claudeCodeHooksRepo
}

// Stub implementations for unused services
func (c *CoverageTestContainer) BackofficeService() services.BackofficeServiceInterface { return nil }
func (c *CoverageTestContainer) EmbeddingBackfillService() services.EmbeddingBackfillServiceInterface {
	return nil
}
func (c *CoverageTestContainer) EmailService() services.EmailServiceInterface     { return nil }
func (c *CoverageTestContainer) EnvironmentService() *services.EnvironmentService { return nil }
func (c *CoverageTestContainer) UserPreferencesService() services.UserPreferencesServiceInterface {
	return nil
}
func (c *CoverageTestContainer) TeamInvitationService() *services.TeamInvitationService { return nil }
func (c *CoverageTestContainer) PromptGalleryService() services.PromptGalleryServiceInterface {
	return nil
}
func (c *CoverageTestContainer) PromptShareService() services.PromptShareServiceInterface {
	return nil
}
func (c *CoverageTestContainer) BlueprintService() services.BlueprintServiceInterface { return nil }
func (c *CoverageTestContainer) ProjectService() services.ProjectServiceInterface     { return nil }

// Stub implementations for repositories
func (c *CoverageTestContainer) UserRepository() repositories.UserRepository     { return nil }
func (c *CoverageTestContainer) APIKeyRepository() repositories.APIKeyRepository { return nil }
func (c *CoverageTestContainer) PromptRepository() repositories.PromptRepository { return nil }
func (c *CoverageTestContainer) PromptGalleryRepository() repositories.PromptGalleryRepository {
	return nil
}
func (c *CoverageTestContainer) PromptShareRepository() repositories.PromptShareRepository {
	return nil
}
func (c *CoverageTestContainer) ArtifactRepository() repositories.ArtifactRepository { return nil }
func (c *CoverageTestContainer) BlueprintRepository() repositories.BlueprintRepository {
	return nil
}
func (c *CoverageTestContainer) EmbeddingProviderRepository() repositories.EmbeddingProviderRepository {
	return nil
}
func (c *CoverageTestContainer) ActivityRepository() repositories.ActivityRepository { return nil }
func (c *CoverageTestContainer) ResourceAccessRepository() repositories.ResourceAccessRepository {
	return nil
}
func (c *CoverageTestContainer) CursorIDEHooksRepository() repositories.CursorIDEHooksRepository {
	return nil
}
func (c *CoverageTestContainer) AgentRepository() repositories.AgentRepository { return nil }
func (c *CoverageTestContainer) AgentExecutionRepository() repositories.AgentExecutionRepository {
	return nil
}
func (c *CoverageTestContainer) AgentExecutionEventRepository() repositories.AgentExecutionEventRepository {
	return nil
}
func (c *CoverageTestContainer) MemoryRepository() repositories.MemoryRepository         { return nil }
func (c *CoverageTestContainer) EmbeddingRepository() repositories.EmbeddingRepository   { return nil }
func (c *CoverageTestContainer) BackofficeRepository() repositories.BackofficeRepository { return nil }
func (c *CoverageTestContainer) UserPreferencesRepository() repositories.UserPreferencesRepository {
	return nil
}
func (c *CoverageTestContainer) TeamRepository() repositories.TeamRepository { return nil }
func (c *CoverageTestContainer) TeamSubscriptionRepository() repositories.TeamSubscriptionRepository {
	return nil
}
func (c *CoverageTestContainer) ProjectRepository() repositories.ProjectRepository { return nil }
func (c *CoverageTestContainer) WebhookEventRepository() repositories.WebhookEventRepository {
	return nil
}

func (c *CoverageTestContainer) FeedRepository() repositories.FeedRepository         { return nil }
func (c *CoverageTestContainer) FeedItemRepository() repositories.FeedItemRepository { return nil }
func (c *CoverageTestContainer) FeedItemReplyRepository() repositories.FeedItemReplyRepository {
	return nil
}
func (c *CoverageTestContainer) FeedService() services.FeedServiceInterface         { return nil }
func (c *CoverageTestContainer) FeedItemService() services.FeedItemServiceInterface { return nil }
func (c *CoverageTestContainer) FeedItemReplyService() services.FeedItemReplyServiceInterface {
	return nil
}

// External and infrastructure stubs
func (c *CoverageTestContainer) IdentityProvider() idp.IdentityProvider { return nil }
func (c *CoverageTestContainer) Close() error                           { return nil }

func (c *CoverageTestContainer) NotificationRepository() repositories.NotificationRepository {
	return nil
}
func (c *CoverageTestContainer) NotificationDeliveryRepository() repositories.NotificationDeliveryRepository {
	return nil
}
func (c *CoverageTestContainer) NotificationDigestQueueRepository() repositories.NotificationDigestQueueRepository {
	return nil
}
func (c *CoverageTestContainer) NotificationService() notifications.NotificationServiceInterface {
	return nil
}
func (c *CoverageTestContainer) DigestRunner() *notifications.DigestRunner { return nil }
func (c *CoverageTestContainer) DeviceTokenRepository() repositories.DeviceTokenRepository {
	return nil
}
func (c *CoverageTestContainer) ProjectMigrationService() services.ProjectMigrationServiceInterface {
	return nil
}

// noopActivityService is a no-op implementation for tests
type noopActivityService struct{}

func (n *noopActivityService) RecordActivity(
	ctx context.Context, userID string, req activities.CreateActivityRequest,
) (*activities.Activity, error) {
	return &activities.Activity{}, nil
}
func (n *noopActivityService) RecordAuthActivity(
	ctx context.Context, userID, activityType string, sessionID *string,
	metadata map[string]interface{}, sourceIP, userAgent *string,
) error {
	return nil
}
func (n *noopActivityService) RecordResourceActivity(
	ctx context.Context, userID, activityType, entityType string,
	entityID *string, description string, metadata map[string]interface{},
) error {
	return nil
}
func (n *noopActivityService) RecordClaudeCodeActivity(
	ctx context.Context, userID, sessionID string, toolName *string,
	hookEventName string, metadata map[string]interface{},
) error {
	return nil
}
func (n *noopActivityService) GetActivities(
	ctx context.Context, filters activities.ActivityFilters,
) (*activities.ActivityListResponse, error) {
	return &activities.ActivityListResponse{}, nil
}
func (n *noopActivityService) GetActivityByID(
	ctx context.Context, userID, activityID string,
) (*activities.Activity, error) {
	return nil, nil
}
func (n *noopActivityService) GetActivityStats(
	ctx context.Context, userID string,
) (*activities.ActivityStatsResponse, error) {
	return &activities.ActivityStatsResponse{}, nil
}
func (n *noopActivityService) GetAllTypes() *activities.ActivityTypesResponse {
	return &activities.ActivityTypesResponse{}
}
func (n *noopActivityService) DeleteActivity(ctx context.Context, activityID string) error {
	return nil
}
func (n *noopActivityService) GetActivityTypes() []string              { return []string{} }
func (n *noopActivityService) GetEntityTypes() []string                { return []string{} }
func (n *noopActivityService) RunRetentionJob(_ context.Context) error { return nil }

func newCoverageTestContainer(t *testing.T) *CoverageTestContainer {
	return &CoverageTestContainer{
		agentService:             svcmocks.NewMockAgentServiceInterface(t),
		agentInvocationService:   svcmocks.NewMockAgentInvocationServiceInterface(t),
		memoryService:            svcmocks.NewMockMemoryServiceInterface(t),
		teamService:              svcmocks.NewMockTeamServiceInterface(t),
		resourceUsageService:     svcmocks.NewMockResourceUsageServiceInterface(t),
		authService:              svcmocks.NewMockAuthServiceInterface(t),
		embeddingProviderService: svcmocks.NewMockEmbeddingProviderServiceInterface(t),
		embeddingService:         svcmocks.NewMockEmbeddingServiceInterface(t),
		promptService:            svcmocks.NewMockPromptServiceInterface(t),
		artifactService:          svcmocks.NewMockArtifactServiceInterface(t),
		apiKeyService:            svcmocks.NewMockAPIKeyServiceInterface(t),
		activityService:          &noopActivityService{},
		teamMemberRepo:           repomocks.NewMockTeamMemberRepository(t),
		claudeCodeHooksRepo:      repomocks.NewMockClaudeCodeHooksRepository(t),
	}
}

func createCoverageTestServer(container *CoverageTestContainer) *Server {
	cfg := &config.Config{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	r := chi.NewRouter()

	srv := &Server{
		port:      "8080",
		container: container,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	return srv
}

func makeCoverageAuthRequest(method, path string, body interface{}) *http.Request {
	var reqBody []byte
	var err error
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			panic(err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	ctx := context.WithValue(req.Context(), contextKeyUserID, coverageTestUserID)
	return req.WithContext(ctx)
}

func addCoverageChiParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for key, value := range params {
		rctx.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// =============================================================================
// Memory Handler Coverage Improvement Tests
// =============================================================================

// TestMemoryListCoverage_WithTeamFilter tests memory list with team filter
func TestMemoryListCoverage_WithTeamFilter(t *testing.T) {
	container := newCoverageTestContainer(t)
	srv := createCoverageTestServer(container)

	teamID := "550e8400-e29b-41d4-a716-446655440000"

	// No longer need GetUserByID mock - team_id is in route params
	container.memoryService.On("ListMemories", "user-123", mock.MatchedBy(func(f services.MemoryFilters) bool {
		// TeamID filter is no longer set from query params
		return f.Page == 1 && f.Limit == 10
	})).Return(&models.MemoryListResponse{
		Memories:   []models.Memory{},
		TotalCount: 0,
		Page:       1,
		PerPage:    20,
		TotalPages: 0,
	}, nil).Maybe()

	req := makeCoverageAuthRequest("GET", "/api/v1/"+teamID+"/memories", nil)
	req = addCoverageChiParams(req, map[string]string{"team_id": teamID})
	rr := httptest.NewRecorder()

	srv.handleListMemories(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

// TestMemoryDeleteCoverage_InvalidTeamUUID tests delete with invalid team UUID
func TestMemoryDeleteCoverage_SuccessWithNonStandardTeamID(t *testing.T) {
	container := newCoverageTestContainer(t)
	srv := createCoverageTestServer(container)

	memoryID := "mem-123"
	nonStandardTeamID := "invalid-uuid"

	// Mock the service call - handler will pass whatever team_id it gets
	container.memoryService.On("DeleteMemory", "user-123", nonStandardTeamID, memoryID).
		Return(nil)
	container.embeddingService.On("DeleteEmbeddingsByEntity", "memory", memoryID).
		Return(nil)

	req := makeCoverageAuthRequest("DELETE", "/api/v1/"+nonStandardTeamID+"/memories/"+memoryID, nil)
	req = addCoverageChiParams(req, map[string]string{
		"team_id": nonStandardTeamID,
		"id":      memoryID,
	})
	rr := httptest.NewRecorder()

	srv.handleDeleteMemory(rr, req)

	// Handler doesn't validate UUID format - that's done by middleware
	// So it will succeed with the mock
	assert.Equal(t, http.StatusNoContent, rr.Code)
}

// TestMemoryDeleteCoverage_MemoryNotFound tests delete with memory not found
func TestMemoryDeleteCoverage_MemoryNotFound(t *testing.T) {
	container := newCoverageTestContainer(t)
	srv := createCoverageTestServer(container)

	memoryID := "mem-123"
	teamID := "550e8400-e29b-41d4-a716-446655440000"

	// Mock service to return memory not found error
	container.memoryService.On("DeleteMemory", "user-123", teamID, memoryID).
		Return(repositories.ErrMemoryNotFound)

	req := makeCoverageAuthRequest("DELETE", "/api/v1/"+teamID+"/memories/"+memoryID, nil)
	req = addCoverageChiParams(req, map[string]string{
		"team_id": teamID,
		"id":      memoryID,
	})
	rr := httptest.NewRecorder()

	srv.handleDeleteMemory(rr, req)

	// Handler returns 404 for "memory not found" error
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// =============================================================================
// Team Handler Coverage Improvement Tests
// =============================================================================

// TestTeamUpdateCoverage_ValidationNoFieldsProvided tests update with no fields
func TestTeamUpdateCoverage_ValidationNoFieldsProvided(t *testing.T) {
	container := newCoverageTestContainer(t)
	srv := createCoverageTestServer(container)

	teamID := "550e8400-e29b-41d4-a716-446655440000"

	body := map[string]interface{}{}

	req := makeCoverageAuthRequest("PUT", "/api/v1/teams/"+teamID, body)
	req = addCoverageChiParams(req, map[string]string{"id": teamID})
	rr := httptest.NewRecorder()

	srv.handleUpdateTeam(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "At least one field")
}

// TestTeamUpdateCoverage_ValidationEmptyName tests update with empty name
func TestTeamUpdateCoverage_ValidationEmptyName(t *testing.T) {
	container := newCoverageTestContainer(t)
	srv := createCoverageTestServer(container)

	teamID := "550e8400-e29b-41d4-a716-446655440000"

	body := map[string]interface{}{
		"name": "",
	}

	req := makeCoverageAuthRequest("PUT", "/api/v1/teams/"+teamID, body)
	req = addCoverageChiParams(req, map[string]string{"id": teamID})
	rr := httptest.NewRecorder()

	srv.handleUpdateTeam(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Name cannot be empty")
}

// TestTeamUpdateCoverage_ValidationNameTooLong tests update with name too long
func TestTeamUpdateCoverage_ValidationNameTooLong(t *testing.T) {
	container := newCoverageTestContainer(t)
	srv := createCoverageTestServer(container)

	teamID := "550e8400-e29b-41d4-a716-446655440000"

	// Create a name longer than 100 characters
	longName := ""
	for i := 0; i < 101; i++ {
		longName += "a"
	}

	body := map[string]interface{}{
		"name": longName,
	}

	req := makeCoverageAuthRequest("PUT", "/api/v1/teams/"+teamID, body)
	req = addCoverageChiParams(req, map[string]string{"id": teamID})
	rr := httptest.NewRecorder()

	srv.handleUpdateTeam(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Name cannot be longer than 100 characters")
}

// TestTeamUpdateCoverage_ValidationDescriptionTooLong tests update with description too long
func TestTeamUpdateCoverage_ValidationDescriptionTooLong(t *testing.T) {
	container := newCoverageTestContainer(t)
	srv := createCoverageTestServer(container)

	teamID := "550e8400-e29b-41d4-a716-446655440000"

	// Create a description longer than 500 characters
	longDesc := ""
	for i := 0; i < 501; i++ {
		longDesc += "a"
	}

	body := map[string]interface{}{
		"description": longDesc,
	}

	req := makeCoverageAuthRequest("PUT", "/api/v1/teams/"+teamID, body)
	req = addCoverageChiParams(req, map[string]string{"id": teamID})
	rr := httptest.NewRecorder()

	srv.handleUpdateTeam(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Description cannot be longer than 500 characters")
}

// TestTeamListCoverage_WithPagination tests team list with pagination
func TestTeamListCoverage_WithPagination(t *testing.T) {
	container := newCoverageTestContainer(t)
	srv := createCoverageTestServer(container)

	container.teamService.On("ListTeams", mock.Anything, "user-123", 2, 10).
		Return(&models.TeamListResponse{
			Teams:      []models.Team{},
			TotalCount: 0,
			Page:       2,
			PageSize:   10,
		}, nil).Maybe()

	req := makeCoverageAuthRequest("GET", "/api/v1/teams?page=2&page_size=10", nil)
	rr := httptest.NewRecorder()

	srv.handleListTeams(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

// TestTeamGetMembersCoverage_ServiceError tests getting members with service error
func TestTeamGetMembersCoverage_ServiceError(t *testing.T) {
	container := newCoverageTestContainer(t)
	srv := createCoverageTestServer(container)

	teamID := "550e8400-e29b-41d4-a716-446655440000"

	container.teamService.On("GetTeamMembers", mock.Anything, "user-123", teamID, 1, 100).
		Return(nil, errors.New("database error")).Maybe()

	req := makeCoverageAuthRequest("GET", "/api/v1/teams/"+teamID+"/members", nil)
	req = addCoverageChiParams(req, map[string]string{"id": teamID})
	rr := httptest.NewRecorder()

	srv.handleGetTeamMembers(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestTeamCreateCoverage_DescriptionTooLong tests creating team with description too long
func TestTeamCreateCoverage_DescriptionTooLong(t *testing.T) {
	container := newCoverageTestContainer(t)
	srv := createCoverageTestServer(container)

	// Create a description longer than 500 characters
	longDesc := ""
	for i := 0; i < 501; i++ {
		longDesc += "a"
	}

	body := map[string]interface{}{
		"name":        "Valid Name",
		"description": longDesc,
	}

	req := makeCoverageAuthRequest("POST", "/api/v1/teams", body)
	rr := httptest.NewRecorder()

	srv.handleCreateTeam(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Description cannot be longer than 500 characters")
}

// =============================================================================
// Embedding Provider Handler Coverage Improvement Tests
// =============================================================================

// TestEmbeddingProviderGetCoverage_ServiceError tests get with service error
func TestEmbeddingProviderGetCoverage_ServiceError(t *testing.T) {
	container := newCoverageTestContainer(t)
	cfg := &config.Config{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: container,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	providerID := "550e8400-e29b-41d4-a716-446655440000"

	container.embeddingProviderService.On("GetEmbeddingProvider", mock.Anything, "user-123", providerID).
		Return(nil, errors.New("database connection failed")).Maybe()

	req := makeCoverageAuthRequest("GET", "/api/v1/embedding-providers/"+providerID, nil)
	req = addCoverageChiParams(req, map[string]string{"id": providerID})
	rr := httptest.NewRecorder()

	srv.handleGetEmbeddingProvider(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestEmbeddingProviderUpdateCoverage_ServiceError tests update with service error
func TestEmbeddingProviderUpdateCoverage_ServiceError(t *testing.T) {
	container := newCoverageTestContainer(t)
	cfg := &config.Config{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: container,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	providerID := "550e8400-e29b-41d4-a716-446655440000"

	container.embeddingProviderService.On("UpdateEmbeddingProvider", mock.Anything, "user-123", providerID, mock.Anything).
		Return(nil, errors.New("database connection failed")).Maybe()

	name := "Updated Provider"
	body := map[string]interface{}{
		"name": name,
	}

	req := makeCoverageAuthRequest("PUT", "/api/v1/embedding-providers/"+providerID, body)
	req = addCoverageChiParams(req, map[string]string{"id": providerID})
	rr := httptest.NewRecorder()

	srv.handleUpdateEmbeddingProvider(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestEmbeddingProviderUpdateCoverage_SuccessWithValidData tests successful update
func TestEmbeddingProviderUpdateCoverage_SuccessWithValidData(t *testing.T) {
	container := newCoverageTestContainer(t)
	cfg := &config.Config{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	r := chi.NewRouter()
	srv := &Server{
		port:      "8080",
		container: container,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	providerID := "550e8400-e29b-41d4-a716-446655440000"

	expectedProvider := &models.EmbeddingProvider{
		ID:           providerID,
		UserID:       "user-123",
		Name:         "Updated Provider",
		ProviderType: "openai",
		IsDefault:    false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	container.embeddingProviderService.On("UpdateEmbeddingProvider", mock.Anything, "user-123", providerID, mock.Anything).
		Return(expectedProvider, nil).Maybe()

	name := "Updated Provider"
	body := map[string]interface{}{
		"name": name,
	}

	req := makeCoverageAuthRequest("PUT", "/api/v1/embedding-providers/"+providerID, body)
	req = addCoverageChiParams(req, map[string]string{"id": providerID})
	rr := httptest.NewRecorder()

	srv.handleUpdateEmbeddingProvider(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func (c *CoverageTestContainer) TypeService() services.TypeServiceInterface { return nil }

func (c *CoverageTestContainer) AttachmentService() services.AttachmentServiceInterface { return nil }
