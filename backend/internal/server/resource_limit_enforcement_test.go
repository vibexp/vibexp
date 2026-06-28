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

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	"github.com/vibexp/vibexp/pkg/events"
)

// MockResourceUsageServiceForHandlers is a mock implementation for testing handlers
type MockResourceUsageServiceForHandlers struct {
	mock.Mock
}

func (m *MockResourceUsageServiceForHandlers) CheckResourceLimit(
	ctx context.Context, userID, resourceType string,
) (bool, error) {
	args := m.Called(ctx, userID, resourceType)
	return args.Bool(0), args.Error(1)
}

func (m *MockResourceUsageServiceForHandlers) TrackResourceCreation(
	ctx context.Context, userID, resourceType, resourceID string,
) error {
	args := m.Called(ctx, userID, resourceType, resourceID)
	return args.Error(0)
}

func (m *MockResourceUsageServiceForHandlers) TrackResourceDeletion(
	ctx context.Context, userID, resourceType, resourceID string,
) error {
	args := m.Called(ctx, userID, resourceType, resourceID)
	return args.Error(0)
}

func (m *MockResourceUsageServiceForHandlers) GetResourceUsage(
	ctx context.Context, userID string,
) (*models.ResourceUsageResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ResourceUsageResponse), args.Error(1)
}

// MockContainerForHandlers is a mock container for testing
type MockContainerForHandlers struct {
	BaseMockContainer
	mock.Mock
	resourceUsageService *MockResourceUsageServiceForHandlers
	authService          services.AuthServiceInterface
	teamService          services.TeamServiceInterface
}

func (m *MockContainerForHandlers) ResourceUsageService() services.ResourceUsageServiceInterface {
	return m.resourceUsageService
}

func (m *MockContainerForHandlers) BackofficeService() services.BackofficeServiceInterface {
	return nil
}
func (m *MockContainerForHandlers) EmbeddingBackfillService() services.EmbeddingBackfillServiceInterface {
	return nil
}
func (m *MockContainerForHandlers) AgentService() services.AgentServiceInterface {
	return nil
}

func (m *MockContainerForHandlers) MemoryService() services.MemoryServiceInterface {
	return nil
}

func (m *MockContainerForHandlers) PromptService() services.PromptServiceInterface {
	return nil
}
func (m *MockContainerForHandlers) PromptGalleryService() services.PromptGalleryServiceInterface {
	return nil
}
func (m *MockContainerForHandlers) PromptGalleryRepository() repositories.PromptGalleryRepository {
	return nil
}
func (m *MockContainerForHandlers) PromptShareRepository() repositories.PromptShareRepository {
	return nil
}
func (m *MockContainerForHandlers) PromptShareService() services.PromptShareServiceInterface {
	return nil
}

func (m *MockContainerForHandlers) ArtifactService() services.ArtifactServiceInterface {
	return nil
}

func (m *MockContainerForHandlers) EmbeddingService() services.EmbeddingServiceInterface {
	return nil
}

func (m *MockContainerForHandlers) SearchService() services.SearchServiceInterface {
	return nil
}

func (m *MockContainerForHandlers) ActivityService() activities.ActivityService {
	return nil
}

func (m *MockContainerForHandlers) ResourceAccessService() resourceaccess.ResourceAccessService {
	return nil
}

func (m *MockContainerForHandlers) AgentCardFetcher() services.AgentCardFetcherInterface {
	return nil
}

func (m *MockContainerForHandlers) AgentInvocationService() services.AgentInvocationServiceInterface {
	return nil
}

func (m *MockContainerForHandlers) APIKeyService() services.APIKeyServiceInterface {
	return nil
}

// Repository methods
func (m *MockContainerForHandlers) UserRepository() repositories.UserRepository {
	return nil
}

func (m *MockContainerForHandlers) APIKeyRepository() repositories.APIKeyRepository {
	return nil
}

func (m *MockContainerForHandlers) PromptRepository() repositories.PromptRepository {
	return nil
}

func (m *MockContainerForHandlers) ArtifactRepository() repositories.ArtifactRepository {
	return nil
}

func (m *MockContainerForHandlers) EmbeddingProviderRepository() repositories.EmbeddingProviderRepository {
	return nil
}

func (m *MockContainerForHandlers) ActivityRepository() repositories.ActivityRepository {
	return nil
}

func (m *MockContainerForHandlers) ResourceAccessRepository() repositories.ResourceAccessRepository {
	return nil
}

func (m *MockContainerForHandlers) ClaudeCodeHooksRepository() repositories.ClaudeCodeHooksRepository {
	return nil
}

func (m *MockContainerForHandlers) CursorIDEHooksRepository() repositories.CursorIDEHooksRepository {
	return nil
}

func (m *MockContainerForHandlers) AgentRepository() repositories.AgentRepository {
	return nil
}

func (m *MockContainerForHandlers) AgentExecutionRepository() repositories.AgentExecutionRepository {
	return nil
}

func (m *MockContainerForHandlers) AgentExecutionEventRepository() repositories.AgentExecutionEventRepository {
	return nil
}

func (m *MockContainerForHandlers) MemoryRepository() repositories.MemoryRepository {
	return nil
}

func (m *MockContainerForHandlers) EmbeddingRepository() repositories.EmbeddingRepository {
	return nil
}
func (m *MockContainerForHandlers) BackofficeRepository() repositories.BackofficeRepository {
	return nil
}

// Other service methods
func (m *MockContainerForHandlers) AuthService() services.AuthServiceInterface {
	return m.authService
}

func (m *MockContainerForHandlers) EmbeddingProviderService() services.EmbeddingProviderServiceInterface {
	return nil
}

func (m *MockContainerForHandlers) EmailService() services.EmailServiceInterface {
	return nil
}

func (m *MockContainerForHandlers) EnvironmentService() *services.EnvironmentService {
	return nil
}

// External dependencies
func (m *MockContainerForHandlers) IdentityProviderRegistry() *idp.Registry {
	return nil
}

func (m *MockContainerForHandlers) SMTPClient() external.SMTPClient {
	return nil
}

// Database access
func (m *MockContainerForHandlers) Database() *database.DB {
	return nil
}

// EventManager returns the event manager
func (m *MockContainerForHandlers) EventManager() events.EventPublisher {
	return nil
}

// Cleanup
func (m *MockContainerForHandlers) BlueprintRepository() repositories.BlueprintRepository {
	return nil
}
func (m *MockContainerForHandlers) BlueprintService() services.BlueprintServiceInterface {
	return nil
}
func (m *MockContainerForHandlers) TeamRepository() repositories.TeamRepository { return nil }
func (m *MockContainerForHandlers) TeamMemberRepository() repositories.TeamMemberRepository {
	return nil
}
func (m *MockContainerForHandlers) TeamService() services.TeamServiceInterface {
	return m.teamService
}
func (m *MockContainerForHandlers) TeamInvitationService() *services.TeamInvitationService {
	return nil
}
func (m *MockContainerForHandlers) TeamSubscriptionRepository() repositories.TeamSubscriptionRepository {
	return nil
}
func (m *MockContainerForHandlers) GitHubAppClient() external.GitHubAppClient            { return nil }
func (m *MockContainerForHandlers) GitHubAppService() services.GitHubAppServiceInterface { return nil }
func (m *MockContainerForHandlers) GitHubInstallationRepository() repositories.GitHubInstallationRepository {
	return nil
}

func (m *MockContainerForHandlers) Close() error {
	return nil
}

// TestAgentCreation_ResourceLimitEnforcement tests agent creation with resource limits
type agentLimitTestCase struct {
	name               string
	userID             string
	subscriptionPlan   string
	currentCount       int
	limit              int
	checkLimitReturns  bool
	checkLimitError    error
	expectedStatusCode int
	expectedErrorCode  string
	description        string
}

func getAgentLimitTestCases() []agentLimitTestCase {
	return []agentLimitTestCase{
		{
			name:               "Free plan - at limit (1/1)",
			userID:             "user-free-at-limit",
			subscriptionPlan:   models.PlanBasic,
			currentCount:       1,
			limit:              1,
			checkLimitReturns:  false,
			checkLimitError:    nil,
			expectedStatusCode: http.StatusForbidden,
			expectedErrorCode:  "RESOURCE_LIMIT_EXCEEDED",
			description:        "Free plan allows 1 agent, user has 1, should be blocked",
		},
		{
			name:               "Starter plan - at limit (3/3)",
			userID:             "user-starter-at-limit",
			subscriptionPlan:   models.PlanStarter,
			currentCount:       3,
			limit:              3,
			checkLimitReturns:  false,
			checkLimitError:    nil,
			expectedStatusCode: http.StatusForbidden,
			expectedErrorCode:  "RESOURCE_LIMIT_EXCEEDED",
			description:        "Starter plan allows 3 agents, user has 3, should be blocked",
		},
		{
			name:               "Pro plan - at limit (5/5)",
			userID:             "user-pro-at-limit",
			subscriptionPlan:   models.PlanPro,
			currentCount:       5,
			limit:              5,
			checkLimitReturns:  false,
			checkLimitError:    nil,
			expectedStatusCode: http.StatusForbidden,
			expectedErrorCode:  "RESOURCE_LIMIT_EXCEEDED",
			description:        "Pro plan allows 5 agents, user has 5, should be blocked",
		},
		{
			name:               "Resource limit check error",
			userID:             "user-error",
			subscriptionPlan:   models.PlanBasic,
			currentCount:       0,
			limit:              1,
			checkLimitReturns:  false,
			checkLimitError:    errors.New("database error"),
			expectedStatusCode: http.StatusInternalServerError,
			expectedErrorCode:  "INTERNAL_ERROR",
			description:        "When CheckResourceLimit returns error, should return 500",
		},
	}
}

func runAgentLimitTest(t *testing.T, tt agentLimitTestCase) {
	// Create mock resource usage service
	mockResourceService := new(MockResourceUsageServiceForHandlers)
	mockContainer := &MockContainerForHandlers{
		resourceUsageService: mockResourceService,
	}

	// Setup expected call
	mockResourceService.On("CheckResourceLimit", mock.Anything, tt.userID, "agent").
		Return(tt.checkLimitReturns, tt.checkLimitError)

	// Create server with mock container
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := &Server{
		port:      "8080",
		container: mockContainer,
		logger:    logger,
		config:    cfg,
	}

	// Create request body
	reqBody := map[string]interface{}{
		"card_url": "http://localhost:8000/.well-known/agent-card.json",
		"status":   "active",
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatal(err)
	}

	// Create request
	req := httptest.NewRequest("POST", "/api/v1/agents", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, tt.userID))

	// Create response recorder
	w := httptest.NewRecorder()

	// Call handler directly
	srv.handleCreateAgent(w, req)

	// Assert status code
	assert.Equal(t, tt.expectedStatusCode, w.Code, tt.description)

	// If expecting an error, check the error code in response
	if tt.expectedErrorCode != "" {
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, tt.expectedErrorCode, response["code"], "Expected error code to match")
	}

	// Verify mock expectations
	mockResourceService.AssertExpectations(t)
}

func TestAgentCreation_ResourceLimitEnforcement(t *testing.T) {
	// Test cases focus on resource limit exceeded scenarios (the bug we're fixing)
	// Testing successful creation would require full service mocking which is out of scope
	for _, tt := range getAgentLimitTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			runAgentLimitTest(t, tt)
		})
	}
}

// TestArtifactCreation_ResourceLimitEnforcement tests artifact creation with resource limits
type resourceLimitTestCase struct {
	name               string
	userID             string
	subscriptionPlan   string
	currentCount       int
	limit              int
	checkLimitReturns  bool
	checkLimitError    error
	expectedStatusCode int
	expectedErrorCode  string
	description        string
}

func setupResourceLimitTestServer(
	t *testing.T,
	userID string,
	checkLimitReturns bool,
	checkLimitError error,
) (*Server, *httptest.ResponseRecorder, *MockResourceUsageServiceForHandlers) {
	t.Helper()
	mockResourceService := new(MockResourceUsageServiceForHandlers)
	mockContainer := &MockContainerForHandlers{
		resourceUsageService: mockResourceService,
	}

	mockResourceService.On("CheckResourceLimit", mock.Anything, userID, "artifact").
		Return(checkLimitReturns, checkLimitError)

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := &Server{
		port:      "8080",
		container: mockContainer,
		logger:    logger,
		config:    cfg,
	}

	return srv, httptest.NewRecorder(), mockResourceService
}

func createArtifactTestRequest(t *testing.T, userID string) *http.Request {
	t.Helper()
	reqBody := map[string]interface{}{
		"project_id": "550e8400-e29b-41d4-a716-446655440000",
		"slug":       "test-artifact",
		"title":      "Test Artifact",
		"content":    "Test content for artifact",
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatal(err)
	}

	url := "/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts"
	req := httptest.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, userID))
	return req
}

func verifyResourceLimitResponse(t *testing.T, w *httptest.ResponseRecorder, tt resourceLimitTestCase) {
	t.Helper()
	assert.Equal(t, tt.expectedStatusCode, w.Code, tt.description)

	if tt.expectedErrorCode != "" {
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, tt.expectedErrorCode, response["code"], "Expected error code to match")

		if tt.expectedErrorCode == "RESOURCE_LIMIT_EXCEEDED" {
			message, ok := response["detail"].(string)
			assert.True(t, ok, "Expected message field to be a string")
			if tt.name == fmt.Sprintf("Free plan - at limit (%d/%d)", tt.currentCount, tt.limit) {
				assert.Contains(t, message, "maximum number")
				assert.Contains(t, message, "allowed for your subscription plan")
			}
		}
	}
}

func getPlanAtLimitTestCases() []resourceLimitTestCase {
	return []resourceLimitTestCase{
		{
			name: "Free plan - at limit (20/20)", userID: "user-free-at-limit",
			subscriptionPlan: models.PlanBasic, currentCount: 20, limit: 20,
			checkLimitReturns: false, checkLimitError: nil,
			expectedStatusCode: http.StatusForbidden, expectedErrorCode: "RESOURCE_LIMIT_EXCEEDED",
			description: "Free plan allows 20 artifacts, user has 20, should be blocked",
		},
		{
			name: "Starter plan - at limit (50/50)", userID: "user-starter-at-limit",
			subscriptionPlan: models.PlanStarter, currentCount: 50, limit: 50,
			checkLimitReturns: false, checkLimitError: nil,
			expectedStatusCode: http.StatusForbidden, expectedErrorCode: "RESOURCE_LIMIT_EXCEEDED",
			description: "Starter plan allows 50 artifacts, user has 50, should be blocked",
		},
		{
			name: "Pro plan - at limit (100/100)", userID: "user-pro-at-limit",
			subscriptionPlan: models.PlanPro, currentCount: 100, limit: 100,
			checkLimitReturns: false, checkLimitError: nil,
			expectedStatusCode: http.StatusForbidden, expectedErrorCode: "RESOURCE_LIMIT_EXCEEDED",
			description: "Pro plan allows 100 artifacts, user has 100, should be blocked",
		},
		{
			name: "PowerUser plan - at limit (500/500)", userID: "user-poweruser-at-limit",
			subscriptionPlan: models.PlanPowerUser, currentCount: 500, limit: 500,
			checkLimitReturns: false, checkLimitError: nil,
			expectedStatusCode: http.StatusForbidden, expectedErrorCode: "RESOURCE_LIMIT_EXCEEDED",
			description: "PowerUser plan allows 500 artifacts, user has 500, should be blocked",
		},
	}
}

func getArtifactResourceLimitErrorTestCases() []resourceLimitTestCase {
	return []resourceLimitTestCase{
		{
			name: "Resource limit check error", userID: "user-error",
			subscriptionPlan: models.PlanBasic, currentCount: 0, limit: 20,
			checkLimitReturns: false, checkLimitError: errors.New("database error"),
			expectedStatusCode: http.StatusInternalServerError, expectedErrorCode: "INTERNAL_ERROR",
			description: "When CheckResourceLimit returns error, should return 500",
		},
		{
			name: "Off-by-one boundary test - exactly at limit", userID: "user-boundary",
			subscriptionPlan: models.PlanBasic, currentCount: 20, limit: 20,
			checkLimitReturns: false, checkLimitError: nil,
			expectedStatusCode: http.StatusForbidden, expectedErrorCode: "RESOURCE_LIMIT_EXCEEDED",
			description: "Boundary test: exactly at limit (count >= limit) should be blocked",
		},
	}
}

func getArtifactResourceLimitTestCases() []resourceLimitTestCase {
	cases := getPlanAtLimitTestCases()
	cases = append(cases, getArtifactResourceLimitErrorTestCases()...)
	return cases
}

func TestArtifactCreation_ResourceLimitEnforcement(t *testing.T) {
	tests := getArtifactResourceLimitTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, w, mockService := setupResourceLimitTestServer(t, tt.userID, tt.checkLimitReturns, tt.checkLimitError)
			req := createArtifactTestRequest(t, tt.userID)

			srv.handleCreateArtifact(w, req)

			verifyResourceLimitResponse(t, w, tt)
			mockService.AssertExpectations(t)
		})
	}
}

type resourceLimitErrorTestCase struct {
	name            string
	resourceType    string
	expectedMessage string
	requestBody     map[string]interface{}
}

func getResourceLimitErrorTestCases() []resourceLimitErrorTestCase {
	return []resourceLimitErrorTestCase{
		{
			name:            "Agent limit exceeded message",
			resourceType:    "agent",
			expectedMessage: "You have reached the maximum number of agents allowed for your subscription plan",
			requestBody: map[string]interface{}{
				"card_url": "http://localhost:8000/.well-known/agent-card.json",
			},
		},
		{
			name:            "Artifact limit exceeded message",
			resourceType:    "artifact",
			expectedMessage: "You have reached the maximum number of artifacts allowed for your subscription plan",
			requestBody: map[string]interface{}{
				"project_id": "550e8400-e29b-41d4-a716-446655440000",
				"slug":       "test-slug",
				"title":      "Test Title",
				"content":    "Test content",
			},
		},
	}
}

func runResourceLimitErrorTest(t *testing.T, tt resourceLimitErrorTestCase) {
	// Create mock service that returns false (limit exceeded)
	mockResourceService := new(MockResourceUsageServiceForHandlers)
	mockContainer := &MockContainerForHandlers{
		resourceUsageService: mockResourceService,
	}

	mockResourceService.On("CheckResourceLimit", mock.Anything, mock.Anything, tt.resourceType).
		Return(false, nil)

	// Create server
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := &Server{
		port:      "8080",
		container: mockContainer,
		logger:    logger,
		config:    cfg,
	}

	// Create request
	bodyBytes, err := json.Marshal(tt.requestBody)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/v1/"+tt.resourceType+"s", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "test-user"))

	w := httptest.NewRecorder()

	// Call appropriate handler
	switch tt.resourceType {
	case "agent":
		srv.handleCreateAgent(w, req)
	case "artifact":
		srv.handleCreateArtifact(w, req)
	}

	// Verify response
	assert.Equal(t, http.StatusForbidden, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "RESOURCE_LIMIT_EXCEEDED", response["code"])
	assert.Equal(t, tt.expectedMessage, response["detail"])

	mockResourceService.AssertExpectations(t)
}

// TestResourceLimitErrorMessages tests that error messages are correct
func TestResourceLimitErrorMessages(t *testing.T) {
	for _, tt := range getResourceLimitErrorTestCases() {
		t.Run(tt.name, func(t *testing.T) {
			runResourceLimitErrorTest(t, tt)
		})
	}
}

type updateResourceLimitTestCase struct {
	name               string
	userID             string
	checkLimitReturns  bool
	checkLimitError    error
	expectedStatusCode int
	expectedErrorCode  string
	description        string
}

func setupUpdateTestServer(
	t *testing.T,
	userID, resourceType string,
	checkLimitReturns bool,
	checkLimitError error,
) (*Server, *MockResourceUsageServiceForHandlers) {
	t.Helper()
	mockResourceService := new(MockResourceUsageServiceForHandlers)
	mockAuthService := mocks.NewMockAuthServiceInterface(t)
	mockTeamService := mocks.NewMockTeamServiceInterface(t)

	// Setup default team for getUserDefaultTeamID
	defaultTeamID := "550e8400-e29b-41d4-a716-446655440000"
	mockAuthService.On("GetUserByID", mock.Anything, userID).
		Return(&models.User{ID: userID, DefaultTeamID: &defaultTeamID}, nil).Maybe()
	mockTeamService.On("IsUserMemberOfTeam", mock.Anything, userID, defaultTeamID).
		Return(true, nil).Maybe()
	mockTeamService.On("GetTeam", mock.Anything, userID, defaultTeamID).
		Return(&models.Team{ID: defaultTeamID, Name: "Test Team"}, nil).Maybe()

	mockContainer := &MockContainerForHandlers{
		resourceUsageService: mockResourceService,
		authService:          mockAuthService,
		teamService:          mockTeamService,
	}

	mockResourceService.On("CheckResourceLimit", mock.Anything, userID, resourceType).
		Return(checkLimitReturns, checkLimitError).Once()

	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	return &Server{
		port:      "8080",
		container: mockContainer,
		logger:    logger,
		config:    cfg,
	}, mockResourceService
}

func verifyUpdateResponse(t *testing.T, w *httptest.ResponseRecorder, tt updateResourceLimitTestCase) {
	t.Helper()
	assert.Equal(t, tt.expectedStatusCode, w.Code, tt.description)

	if tt.expectedErrorCode != "" {
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, tt.expectedErrorCode, response["code"])
	}
}

// TestPromptUpdate_ResourceLimitEnforcement tests that UPDATE operations check resource limits
func TestPromptUpdate_ResourceLimitEnforcement(t *testing.T) {
	tests := []updateResourceLimitTestCase{
		{
			name: "Update blocked when over quota", userID: "user-over-quota",
			checkLimitReturns: false, checkLimitError: nil,
			expectedStatusCode: http.StatusForbidden, expectedErrorCode: "RESOURCE_LIMIT_EXCEEDED",
			description: "UPDATE should be blocked when user exceeds quota",
		},
		{
			name: "Update blocked on quota check error", userID: "user-error",
			checkLimitReturns: false, checkLimitError: errors.New("database error"),
			expectedStatusCode: http.StatusInternalServerError, expectedErrorCode: "INTERNAL_ERROR",
			description: "UPDATE should fail gracefully on quota check error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, mockService := setupUpdateTestServer(t, tt.userID, "prompt", tt.checkLimitReturns, tt.checkLimitError)

			reqBody := map[string]interface{}{"title": "Updated Title"}
			bodyBytes, err := json.Marshal(reqBody)
			assert.NoError(t, err)
			req := httptest.NewRequest(
				"PUT", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-prompt",
				bytes.NewReader(bodyBytes),
			)
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, tt.userID))

			// Add route context with team_id and slug parameters
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("team_id", "550e8400-e29b-41d4-a716-446655440000")
			rctx.URLParams.Add("slug", "test-prompt")
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()
			srv.handleUpdatePrompt(w, req)

			verifyUpdateResponse(t, w, tt)
			mockService.AssertExpectations(t)
			mockService.AssertNumberOfCalls(t, "CheckResourceLimit", 1)
		})
	}
}

// TestArtifactUpdate_ResourceLimitEnforcement tests that UPDATE operations check resource limits
func TestArtifactUpdate_ResourceLimitEnforcement(t *testing.T) {
	tests := []updateResourceLimitTestCase{
		{
			name: "Update blocked when over quota", userID: "user-over-quota",
			checkLimitReturns: false, checkLimitError: nil,
			expectedStatusCode: http.StatusForbidden, expectedErrorCode: "RESOURCE_LIMIT_EXCEEDED",
			description: "UPDATE should be blocked when user exceeds quota",
		},
		{
			name: "Update blocked on quota check error", userID: "user-error",
			checkLimitReturns: false, checkLimitError: errors.New("database error"),
			expectedStatusCode: http.StatusInternalServerError, expectedErrorCode: "INTERNAL_ERROR",
			description: "UPDATE should fail gracefully on quota check error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, mockService := setupUpdateTestServer(t, tt.userID, "artifact", tt.checkLimitReturns, tt.checkLimitError)

			reqBody := map[string]interface{}{"title": "Updated Title"}
			bodyBytes, err := json.Marshal(reqBody)
			assert.NoError(t, err)
			req := httptest.NewRequest(
				"PUT",
				"/api/v1/550e8400-e29b-41d4-a716-446655440000/artifacts/550e8400-e29b-41d4-a716-446655440000/test-artifact",
				bytes.NewReader(bodyBytes),
			)
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, tt.userID))

			// Add chi URL parameters to context
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("project_id", "550e8400-e29b-41d4-a716-446655440000")
			rctx.URLParams.Add("slug", "test-artifact")
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()
			srv.handleUpdateArtifact(w, req)

			verifyUpdateResponse(t, w, tt)
			mockService.AssertExpectations(t)
			mockService.AssertNumberOfCalls(t, "CheckResourceLimit", 1)
		})
	}
}

// TestMemoryUpdate_ResourceLimitEnforcement tests that UPDATE operations check resource limits
func TestMemoryUpdate_ResourceLimitEnforcement(t *testing.T) {
	tests := []updateResourceLimitTestCase{
		{
			name: "Update blocked when over quota", userID: "user-over-quota",
			checkLimitReturns: false, checkLimitError: nil,
			expectedStatusCode: http.StatusForbidden, expectedErrorCode: "RESOURCE_LIMIT_EXCEEDED",
			description: "UPDATE should be blocked when user exceeds quota",
		},
		{
			name: "Update blocked on quota check error", userID: "user-error",
			checkLimitReturns: false, checkLimitError: errors.New("database error"),
			expectedStatusCode: http.StatusInternalServerError, expectedErrorCode: "INTERNAL_ERROR",
			description: "UPDATE should fail gracefully on quota check error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, mockService := setupUpdateTestServer(t, tt.userID, "memory", tt.checkLimitReturns, tt.checkLimitError)

			reqBody := map[string]interface{}{"text": "Updated memory text"}
			bodyBytes, err := json.Marshal(reqBody)
			assert.NoError(t, err)
			req := httptest.NewRequest("PUT", "/api/v1/memories/memory-123", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, tt.userID))

			w := httptest.NewRecorder()
			srv.handleUpdateMemory(w, req)

			verifyUpdateResponse(t, w, tt)
			mockService.AssertExpectations(t)
			mockService.AssertNumberOfCalls(t, "CheckResourceLimit", 1)
		})
	}
}

func (m *MockContainerForHandlers) UserPreferencesRepository() repositories.UserPreferencesRepository {
	return nil
}

func (m *MockContainerForHandlers) UserPreferencesService() services.UserPreferencesServiceInterface {
	return nil
}

func (m *MockContainerForHandlers) ProjectRepository() repositories.ProjectRepository {
	return nil
}

func (m *MockContainerForHandlers) ProjectService() services.ProjectServiceInterface {
	return nil
}

func (m *MockContainerForHandlers) WebhookEventRepository() repositories.WebhookEventRepository {
	return nil
}

func (m *MockContainerForHandlers) FeedRepository() repositories.FeedRepository         { return nil }
func (m *MockContainerForHandlers) FeedItemRepository() repositories.FeedItemRepository { return nil }
func (m *MockContainerForHandlers) FeedItemReplyRepository() repositories.FeedItemReplyRepository {
	return nil
}
func (m *MockContainerForHandlers) FeedService() services.FeedServiceInterface         { return nil }
func (m *MockContainerForHandlers) FeedItemService() services.FeedItemServiceInterface { return nil }
func (m *MockContainerForHandlers) FeedItemReplyService() services.FeedItemReplyServiceInterface {
	return nil
}

func (m *MockContainerForHandlers) NotificationRepository() repositories.NotificationRepository {
	return nil
}
func (m *MockContainerForHandlers) NotificationDeliveryRepository() repositories.NotificationDeliveryRepository {
	return nil
}
func (m *MockContainerForHandlers) NotificationDigestQueueRepository() repositories.NotificationDigestQueueRepository {
	return nil
}
func (m *MockContainerForHandlers) NotificationService() notifications.NotificationServiceInterface {
	return nil
}
func (m *MockContainerForHandlers) DigestRunner() *notifications.DigestRunner { return nil }
func (m *MockContainerForHandlers) DeviceTokenRepository() repositories.DeviceTokenRepository {
	return nil
}
func (m *MockContainerForHandlers) ProjectMigrationService() services.ProjectMigrationServiceInterface {
	return nil
}

func (m *MockContainerForHandlers) TypeService() services.TypeServiceInterface { return nil }

func (m *MockContainerForHandlers) AttachmentService() services.AttachmentServiceInterface {
	return nil
}
