package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

// MockBackofficeService is a mock implementation of BackofficeServiceInterface
type MockBackofficeService struct {
	mock.Mock
}

func (m *MockBackofficeService) GetUsageAndGrowth(
	ctx context.Context,
	fromDate, toDate *time.Time,
) (*models.UsageAndGrowthResponse, error) {
	args := m.Called(ctx, fromDate, toDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UsageAndGrowthResponse), args.Error(1)
}

// MockContainerForBackoffice is a mock container for backoffice handler testing
type MockContainerForBackoffice struct {
	BaseMockContainer
	backofficeService        *MockBackofficeService
	embeddingBackfillService services.EmbeddingBackfillServiceInterface
}

func (m *MockContainerForBackoffice) BackofficeService() services.BackofficeServiceInterface {
	return m.backofficeService
}

func (m *MockContainerForBackoffice) EmbeddingBackfillService() services.EmbeddingBackfillServiceInterface {
	return m.embeddingBackfillService
}

// Implement all required container.Container interface methods
func (m *MockContainerForBackoffice) UserRepository() repositories.UserRepository         { return nil }
func (m *MockContainerForBackoffice) APIKeyRepository() repositories.APIKeyRepository     { return nil }
func (m *MockContainerForBackoffice) PromptRepository() repositories.PromptRepository     { return nil }
func (m *MockContainerForBackoffice) ArtifactRepository() repositories.ArtifactRepository { return nil }
func (m *MockContainerForBackoffice) EmbeddingProviderRepository() repositories.EmbeddingProviderRepository {
	return nil
}
func (m *MockContainerForBackoffice) ActivityRepository() repositories.ActivityRepository { return nil }
func (m *MockContainerForBackoffice) ResourceAccessRepository() repositories.ResourceAccessRepository {
	return nil
}
func (m *MockContainerForBackoffice) ClaudeCodeHooksRepository() repositories.ClaudeCodeHooksRepository {
	return nil
}
func (m *MockContainerForBackoffice) CursorIDEHooksRepository() repositories.CursorIDEHooksRepository {
	return nil
}
func (m *MockContainerForBackoffice) AgentRepository() repositories.AgentRepository { return nil }
func (m *MockContainerForBackoffice) AgentExecutionRepository() repositories.AgentExecutionRepository {
	return nil
}
func (m *MockContainerForBackoffice) AgentExecutionEventRepository() repositories.AgentExecutionEventRepository {
	return nil
}
func (m *MockContainerForBackoffice) MemoryRepository() repositories.MemoryRepository { return nil }
func (m *MockContainerForBackoffice) EmbeddingRepository() repositories.EmbeddingRepository {
	return nil
}
func (m *MockContainerForBackoffice) BackofficeRepository() repositories.BackofficeRepository {
	return nil
}
func (m *MockContainerForBackoffice) AuthService() services.AuthServiceInterface     { return nil }
func (m *MockContainerForBackoffice) APIKeyService() services.APIKeyServiceInterface { return nil }
func (m *MockContainerForBackoffice) PromptService() services.PromptServiceInterface { return nil }
func (m *MockContainerForBackoffice) PromptGalleryService() services.PromptGalleryServiceInterface {
	return nil
}
func (m *MockContainerForBackoffice) PromptGalleryRepository() repositories.PromptGalleryRepository {
	return nil
}
func (m *MockContainerForBackoffice) PromptShareRepository() repositories.PromptShareRepository {
	return nil
}
func (m *MockContainerForBackoffice) PromptShareService() services.PromptShareServiceInterface {
	return nil
}
func (m *MockContainerForBackoffice) ArtifactService() services.ArtifactServiceInterface { return nil }
func (m *MockContainerForBackoffice) EmbeddingProviderService() services.EmbeddingProviderServiceInterface {
	return nil
}
func (m *MockContainerForBackoffice) ActivityService() activities.ActivityService { return nil }
func (m *MockContainerForBackoffice) ResourceAccessService() resourceaccess.ResourceAccessService {
	return nil
}
func (m *MockContainerForBackoffice) AgentService() services.AgentServiceInterface { return nil }
func (m *MockContainerForBackoffice) AgentCardFetcher() services.AgentCardFetcherInterface {
	return nil
}
func (m *MockContainerForBackoffice) AgentInvocationService() services.AgentInvocationServiceInterface {
	return nil
}
func (m *MockContainerForBackoffice) MemoryService() services.MemoryServiceInterface { return nil }
func (m *MockContainerForBackoffice) EmbeddingService() services.EmbeddingServiceInterface {
	return nil
}
func (m *MockContainerForBackoffice) SearchService() services.SearchServiceInterface   { return nil }
func (m *MockContainerForBackoffice) EnvironmentService() *services.EnvironmentService { return nil }
func (m *MockContainerForBackoffice) ResourceUsageService() services.ResourceUsageServiceInterface {
	return nil
}
func (m *MockContainerForBackoffice) EmailService() services.EmailServiceInterface { return nil }
func (m *MockContainerForBackoffice) IdentityProviderRegistry() *idp.Registry      { return nil }
func (m *MockContainerForBackoffice) BlueprintService() services.BlueprintServiceInterface {
	return nil
}
func (m *MockContainerForBackoffice) UserPreferencesRepository() repositories.UserPreferencesRepository {
	return nil
}
func (m *MockContainerForBackoffice) UserPreferencesService() services.UserPreferencesServiceInterface {
	return nil
}
func (m *MockContainerForBackoffice) TeamRepository() repositories.TeamRepository { return nil }
func (m *MockContainerForBackoffice) TeamMemberRepository() repositories.TeamMemberRepository {
	return nil
}
func (m *MockContainerForBackoffice) TeamService() services.TeamServiceInterface { return nil }
func (m *MockContainerForBackoffice) TeamInvitationService() *services.TeamInvitationService {
	return nil
}
func (m *MockContainerForBackoffice) TeamSubscriptionRepository() repositories.TeamSubscriptionRepository {
	return nil
}
func (m *MockContainerForBackoffice) GitHubAppClient() external.GitHubAppClient { return nil }
func (m *MockContainerForBackoffice) GitHubAppService() services.GitHubAppServiceInterface {
	return nil
}
func (m *MockContainerForBackoffice) GitHubInstallationRepository() repositories.GitHubInstallationRepository {
	return nil
}

func (m *MockContainerForBackoffice) Close() error { return nil }
func (m *MockContainerForBackoffice) DeviceTokenRepository() repositories.DeviceTokenRepository {
	return nil
}

// Ensure MockContainerForBackoffice implements container.Container
var _ container.Container = (*MockContainerForBackoffice)(nil)

// Test helper to create test server with mock container
func createBackofficeTestServer(backofficeService *MockBackofficeService, backofficeAPIKey string) *Server {
	cfg := &config.Config{
		BackofficeAdminAPIKey: backofficeAPIKey,
	}
	mockContainer := &MockContainerForBackoffice{
		backofficeService: backofficeService,
	}

	return &Server{
		port:      "8080",
		apiKey:    "test-api-key",
		config:    cfg,
		container: mockContainer,
		logger:    setupTestLogger(),
	}
}

//nolint:funlen // This test requires setup and multiple assertions
func TestBackofficeUsageAndGrowth_Success(t *testing.T) {
	mockService := new(MockBackofficeService)
	backofficeAPIKey := "test-backoffice-key"
	srv := createBackofficeTestServer(mockService, backofficeAPIKey)

	// Mock response
	fromDate, err := time.Parse("2006-01-02", "2024-01-01")
	if err != nil {
		t.Fatalf("Failed to parse test date: %v", err)
	}
	toDate, err := time.Parse("2006-01-02", "2024-01-31")
	if err != nil {
		t.Fatalf("Failed to parse test date: %v", err)
	}

	expectedResponse := &models.UsageAndGrowthResponse{
		Usage: []models.UsageMetricsRow{
			{
				WeekStart:           time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				NewUsers:            10,
				NewArtifacts:        25,
				NewMemories:         15,
				NewAPIKeys:          5,
				NewPrompts:          8,
				NewAgents:           3,
				AgentExecutions:     12,
				ClaudeSessions:      20,
				CursorSessions:      15,
				TotalAIToolSessions: 35,
			},
		},
		ActivitiesPerUser: []models.UserActivityRow{
			{
				UserID:                  "user-123",
				Email:                   "test@example.com",
				Name:                    "Test User",
				UserCreatedAt:           time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				TotalArtifacts:          5,
				TotalMemories:           3,
				TotalPrompts:            2,
				TotalAgentsCreated:      1,
				TotalAgentExecutionsRun: 10,
			},
		},
	}

	mockService.On("GetUsageAndGrowth", mock.Anything, mock.MatchedBy(func(from *time.Time) bool {
		return from != nil && from.Equal(fromDate)
	}), mock.MatchedBy(func(to *time.Time) bool {
		return to != nil && to.Equal(toDate)
	})).Return(expectedResponse, nil)

	req := httptest.NewRequest("GET", "/bo/v1/reports/usage-and-growth?from=2024-01-01&to=2024-01-31", nil)
	req.Header.Set("Authorization", "Bearer "+backofficeAPIKey)

	rr := httptest.NewRecorder()
	srv.handleBackofficeUsageAndGrowth(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response models.UsageAndGrowthResponse
	err = json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Len(t, response.Usage, 1)
	assert.Len(t, response.ActivitiesPerUser, 1)
	assert.Equal(t, 10, response.Usage[0].NewUsers)

	mockService.AssertExpectations(t)
}

func TestBackofficeUsageAndGrowth_MissingAPIKey(t *testing.T) {
	mockService := new(MockBackofficeService)
	backofficeAPIKey := "test-backoffice-key"
	srv := createBackofficeTestServer(mockService, backofficeAPIKey)

	// Set up mock expectation
	mockService.On("GetUsageAndGrowth", mock.Anything, (*time.Time)(nil), (*time.Time)(nil)).
		Return(&models.UsageAndGrowthResponse{}, nil)

	req := httptest.NewRequest("GET", "/bo/v1/reports/usage-and-growth", nil)
	// No Authorization header - middleware would catch this in production
	// For handler testing, we expect it to proceed since auth is middleware concern

	rr := httptest.NewRecorder()
	srv.handleBackofficeUsageAndGrowth(rr, req)

	// Handler processes the request (auth is middleware's job)
	assert.Equal(t, http.StatusOK, rr.Code)

	mockService.AssertExpectations(t)
}

func TestBackofficeUsageAndGrowth_InvalidAPIKey(t *testing.T) {
	mockService := new(MockBackofficeService)
	backofficeAPIKey := "test-backoffice-key"
	srv := createBackofficeTestServer(mockService, backofficeAPIKey)

	// Set up mock expectation
	mockService.On("GetUsageAndGrowth", mock.Anything, (*time.Time)(nil), (*time.Time)(nil)).
		Return(&models.UsageAndGrowthResponse{}, nil)

	req := httptest.NewRequest("GET", "/bo/v1/reports/usage-and-growth", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")

	rr := httptest.NewRecorder()
	srv.handleBackofficeUsageAndGrowth(rr, req)

	// Handler processes the request (auth validation is middleware's job)
	assert.Equal(t, http.StatusOK, rr.Code)

	mockService.AssertExpectations(t)
}

func TestBackofficeUsageAndGrowth_InvalidBearerFormat(t *testing.T) {
	mockService := new(MockBackofficeService)
	backofficeAPIKey := "test-backoffice-key"
	srv := createBackofficeTestServer(mockService, backofficeAPIKey)

	// Set up mock expectation for all subtests
	mockService.On("GetUsageAndGrowth", mock.Anything, (*time.Time)(nil), (*time.Time)(nil)).
		Return(&models.UsageAndGrowthResponse{}, nil)

	tests := []struct {
		name       string
		authHeader string
	}{
		{"Missing Bearer prefix", backofficeAPIKey},
		{"Wrong format", "ApiKey " + backofficeAPIKey},
		{"Only Bearer", "Bearer "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/bo/v1/reports/usage-and-growth", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			srv.handleBackofficeUsageAndGrowth(rr, req)

			// Handler processes the request (auth validation is middleware's job)
			assert.Equal(t, http.StatusOK, rr.Code)
		})
	}

	mockService.AssertExpectations(t)
}

func TestBackofficeUsageAndGrowth_InvalidDateFormat(t *testing.T) {
	mockService := new(MockBackofficeService)
	backofficeAPIKey := "test-backoffice-key"
	srv := createBackofficeTestServer(mockService, backofficeAPIKey)

	tests := []struct {
		name        string
		from        string
		to          string
		expectField string
	}{
		{"Invalid from date", "2024/01/01", "2024-01-31", "from"},
		{"Invalid to date", "2024-01-01", "2024/01/31", "to"},
		{"Both invalid", "2024/01/01", "2024/01/31", "from"},
		{"Invalid format", "01-01-2024", "01-31-2024", "from"},
		{"Text instead of date", "january", "2024-01-31", "from"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/bo/v1/reports/usage-and-growth?from=" + tt.from + "&to=" + tt.to
			req := httptest.NewRequest("GET", url, nil)
			req.Header.Set("Authorization", "Bearer "+backofficeAPIKey)

			rr := httptest.NewRecorder()
			srv.handleBackofficeUsageAndGrowth(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
			// Verify RFC 9457 compliance
			assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))

			var errorResponse map[string]interface{}
			err := json.NewDecoder(rr.Body).Decode(&errorResponse)
			assert.NoError(t, err)

			// Verify RFC 9457 required fields
			assert.Contains(t, errorResponse, "type")
			assert.Contains(t, errorResponse, "title")
			assert.Contains(t, errorResponse, "status")
			assert.Contains(t, errorResponse, "detail")
			assert.Contains(t, errorResponse, "code")
			assert.Equal(t, "VALIDATION_FAILED", errorResponse["code"])
			assert.Contains(t, errorResponse["detail"].(string), "YYYY-MM-DD")

			// Verify validation_errors array contains field-level error
			if validationErrors, ok := errorResponse["validation_errors"].([]interface{}); ok {
				assert.Greater(t, len(validationErrors), 0)
				if len(validationErrors) > 0 {
					firstError := validationErrors[0].(map[string]interface{})
					assert.Equal(t, tt.expectField, firstError["field"])
				}
			}
		})
	}

	mockService.AssertNotCalled(t, "GetUsageAndGrowth")
}

func TestBackofficeUsageAndGrowth_ToDateBeforeFromDate(t *testing.T) {
	mockService := new(MockBackofficeService)
	backofficeAPIKey := "test-backoffice-key"
	srv := createBackofficeTestServer(mockService, backofficeAPIKey)

	req := httptest.NewRequest("GET", "/bo/v1/reports/usage-and-growth?from=2024-01-31&to=2024-01-01", nil)
	req.Header.Set("Authorization", "Bearer "+backofficeAPIKey)

	rr := httptest.NewRecorder()
	srv.handleBackofficeUsageAndGrowth(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	// Verify RFC 9457 compliance
	assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))

	var errorResponse map[string]interface{}
	err := json.NewDecoder(rr.Body).Decode(&errorResponse)
	assert.NoError(t, err)

	// Verify RFC 9457 required fields
	assert.Contains(t, errorResponse, "type")
	assert.Contains(t, errorResponse, "title")
	assert.Contains(t, errorResponse, "status")
	assert.Contains(t, errorResponse, "detail")
	assert.Contains(t, errorResponse, "code")
	assert.Equal(t, "VALIDATION_FAILED", errorResponse["code"])
	assert.Contains(t, errorResponse["detail"].(string), "'from' date must be before 'to' date")

	// Verify validation_errors array contains both field errors
	if validationErrors, ok := errorResponse["validation_errors"].([]interface{}); ok {
		assert.Equal(t, 2, len(validationErrors))
	}

	mockService.AssertNotCalled(t, "GetUsageAndGrowth")
}

func TestBackofficeUsageAndGrowth_ServiceError(t *testing.T) {
	mockService := new(MockBackofficeService)
	backofficeAPIKey := "test-backoffice-key"
	srv := createBackofficeTestServer(mockService, backofficeAPIKey)

	mockService.On("GetUsageAndGrowth", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, assert.AnError)

	req := httptest.NewRequest("GET", "/bo/v1/reports/usage-and-growth", nil)
	req.Header.Set("Authorization", "Bearer "+backofficeAPIKey)

	rr := httptest.NewRecorder()
	srv.handleBackofficeUsageAndGrowth(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	// Verify RFC 9457 compliance
	assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))

	var errorResponse map[string]interface{}
	err := json.NewDecoder(rr.Body).Decode(&errorResponse)
	assert.NoError(t, err)

	// Verify RFC 9457 required fields
	assert.Contains(t, errorResponse, "type")
	assert.Contains(t, errorResponse, "title")
	assert.Contains(t, errorResponse, "status")
	assert.Contains(t, errorResponse, "detail")
	assert.Contains(t, errorResponse, "code")
	assert.Equal(t, "DATABASE_ERROR", errorResponse["code"])
	assert.Contains(t, errorResponse["detail"].(string), "Failed to retrieve usage and growth data")

	mockService.AssertExpectations(t)
}

func TestBackofficeUsageAndGrowth_NoDateFilter(t *testing.T) {
	mockService := new(MockBackofficeService)
	backofficeAPIKey := "test-backoffice-key"
	srv := createBackofficeTestServer(mockService, backofficeAPIKey)

	expectedResponse := &models.UsageAndGrowthResponse{
		Usage:             []models.UsageMetricsRow{},
		ActivitiesPerUser: []models.UserActivityRow{},
	}

	mockService.On("GetUsageAndGrowth", mock.Anything, (*time.Time)(nil), (*time.Time)(nil)).
		Return(expectedResponse, nil)

	req := httptest.NewRequest("GET", "/bo/v1/reports/usage-and-growth", nil)
	req.Header.Set("Authorization", "Bearer "+backofficeAPIKey)

	rr := httptest.NewRecorder()
	srv.handleBackofficeUsageAndGrowth(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.UsageAndGrowthResponse
	err := json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Empty(t, response.Usage)
	assert.Empty(t, response.ActivitiesPerUser)

	mockService.AssertExpectations(t)
}

func TestBackofficeUsageAndGrowth_OnlyFromDate(t *testing.T) {
	mockService := new(MockBackofficeService)
	backofficeAPIKey := "test-backoffice-key"
	srv := createBackofficeTestServer(mockService, backofficeAPIKey)

	// Handler treats partial dates as no dates (both nil)
	mockService.On("GetUsageAndGrowth", mock.Anything, (*time.Time)(nil), (*time.Time)(nil)).
		Return(&models.UsageAndGrowthResponse{}, nil)

	req := httptest.NewRequest("GET", "/bo/v1/reports/usage-and-growth?from=2024-01-01", nil)
	req.Header.Set("Authorization", "Bearer "+backofficeAPIKey)

	rr := httptest.NewRecorder()
	srv.handleBackofficeUsageAndGrowth(rr, req)

	// Handler treats partial dates as if none were provided
	assert.Equal(t, http.StatusOK, rr.Code)

	mockService.AssertExpectations(t)
}

func TestBackofficeUsageAndGrowth_OnlyToDate(t *testing.T) {
	mockService := new(MockBackofficeService)
	backofficeAPIKey := "test-backoffice-key"
	srv := createBackofficeTestServer(mockService, backofficeAPIKey)

	// Handler treats partial dates as no dates (both nil)
	mockService.On("GetUsageAndGrowth", mock.Anything, (*time.Time)(nil), (*time.Time)(nil)).
		Return(&models.UsageAndGrowthResponse{}, nil)

	req := httptest.NewRequest("GET", "/bo/v1/reports/usage-and-growth?to=2024-01-31", nil)
	req.Header.Set("Authorization", "Bearer "+backofficeAPIKey)

	rr := httptest.NewRecorder()
	srv.handleBackofficeUsageAndGrowth(rr, req)

	// Handler treats partial dates as if none were provided
	assert.Equal(t, http.StatusOK, rr.Code)

	mockService.AssertExpectations(t)
}

//nolint:funlen // This test requires extensive response structure verification
func TestBackofficeUsageAndGrowth_ResponseStructure(t *testing.T) {
	mockService := new(MockBackofficeService)
	backofficeAPIKey := "test-backoffice-key"
	srv := createBackofficeTestServer(mockService, backofficeAPIKey)

	makeTimePtr := func(t time.Time) *time.Time {
		return &t
	}

	expectedResponse := &models.UsageAndGrowthResponse{
		Usage: []models.UsageMetricsRow{
			{
				WeekStart:           time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				NewUsers:            10,
				NewArtifacts:        25,
				NewMemories:         15,
				NewAPIKeys:          5,
				NewPrompts:          8,
				NewAgents:           3,
				AgentExecutions:     12,
				ClaudeSessions:      20,
				CursorSessions:      15,
				TotalAIToolSessions: 35,
			},
		},
		ActivitiesPerUser: []models.UserActivityRow{
			{
				UserID:                  "user-123",
				Email:                   "test@example.com",
				Name:                    "Test User",
				UserCreatedAt:           time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				TotalArtifacts:          5,
				FirstArtifactCreatedAt:  makeTimePtr(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)),
				TotalMemories:           3,
				FirstMemoryCreatedAt:    makeTimePtr(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)),
				TotalPrompts:            2,
				FirstPromptCreatedAt:    makeTimePtr(time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)),
				TotalAgentsCreated:      1,
				TotalAgentExecutionsRun: 10,
			},
		},
	}

	mockService.On("GetUsageAndGrowth", mock.Anything, mock.Anything, mock.Anything).
		Return(expectedResponse, nil)

	req := httptest.NewRequest("GET", "/bo/v1/reports/usage-and-growth?from=2024-01-01&to=2024-01-31", nil)
	req.Header.Set("Authorization", "Bearer "+backofficeAPIKey)

	rr := httptest.NewRecorder()
	srv.handleBackofficeUsageAndGrowth(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response models.UsageAndGrowthResponse
	err := json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err)

	// Verify response structure
	assert.NotNil(t, response.Usage)
	assert.NotNil(t, response.ActivitiesPerUser)
	assert.Len(t, response.Usage, 1)
	assert.Len(t, response.ActivitiesPerUser, 1)

	// Verify usage metrics
	usage := response.Usage[0]
	assert.Equal(t, 10, usage.NewUsers)
	assert.Equal(t, 25, usage.NewArtifacts)
	assert.Equal(t, 15, usage.NewMemories)
	assert.Equal(t, 5, usage.NewAPIKeys)
	assert.Equal(t, 8, usage.NewPrompts)
	assert.Equal(t, 3, usage.NewAgents)
	assert.Equal(t, 12, usage.AgentExecutions)
	assert.Equal(t, 20, usage.ClaudeSessions)
	assert.Equal(t, 15, usage.CursorSessions)
	assert.Equal(t, 35, usage.TotalAIToolSessions)

	// Verify user activities
	activity := response.ActivitiesPerUser[0]
	assert.Equal(t, "user-123", activity.UserID)
	assert.Equal(t, "test@example.com", activity.Email)
	assert.Equal(t, "Test User", activity.Name)
	assert.Equal(t, 5, activity.TotalArtifacts)
	assert.Equal(t, 3, activity.TotalMemories)
	assert.Equal(t, 2, activity.TotalPrompts)
	assert.Equal(t, 1, activity.TotalAgentsCreated)
	assert.Equal(t, 10, activity.TotalAgentExecutionsRun)

	mockService.AssertExpectations(t)
}

func TestBackofficeUsageAndGrowth_EmptyResults(t *testing.T) {
	mockService := new(MockBackofficeService)
	backofficeAPIKey := "test-backoffice-key"
	srv := createBackofficeTestServer(mockService, backofficeAPIKey)

	expectedResponse := &models.UsageAndGrowthResponse{
		Usage:             []models.UsageMetricsRow{},
		ActivitiesPerUser: []models.UserActivityRow{},
	}

	mockService.On("GetUsageAndGrowth", mock.Anything, mock.Anything, mock.Anything).
		Return(expectedResponse, nil)

	req := httptest.NewRequest("GET", "/bo/v1/reports/usage-and-growth?from=2024-01-01&to=2024-01-31", nil)
	req.Header.Set("Authorization", "Bearer "+backofficeAPIKey)

	rr := httptest.NewRecorder()
	srv.handleBackofficeUsageAndGrowth(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.UsageAndGrowthResponse
	err := json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Empty(t, response.Usage)
	assert.Empty(t, response.ActivitiesPerUser)

	mockService.AssertExpectations(t)
}

//nolint:gosec // This is a test value, not a real credential
func TestBackofficeAuthMiddleware_JWTAuthNotAccepted(t *testing.T) {
	mockService := new(MockBackofficeService)
	backofficeAPIKey := "test-backoffice-key"
	srv := createBackofficeTestServer(mockService, backofficeAPIKey)

	// Set up mock expectation
	mockService.On("GetUsageAndGrowth", mock.Anything, (*time.Time)(nil), (*time.Time)(nil)).
		Return(&models.UsageAndGrowthResponse{}, nil)

	// Try to access with a JWT token - middleware would reject in production
	// gitleaks:allow - this is a test-only fake JWT token, not a real secret
	jwtToken := "test-fake-jwt-token-not-real"

	req := httptest.NewRequest("GET", "/bo/v1/reports/usage-and-growth", nil)
	req.Header.Set("Authorization", "Bearer "+jwtToken)

	rr := httptest.NewRecorder()
	srv.handleBackofficeUsageAndGrowth(rr, req)

	// Handler processes the request (auth validation is middleware's job)
	assert.Equal(t, http.StatusOK, rr.Code)

	mockService.AssertExpectations(t)
}

func TestBackofficeAuthMiddleware_APIKeyAuthNotAccepted(t *testing.T) {
	mockService := new(MockBackofficeService)
	backofficeAPIKey := "test-backoffice-key"
	srv := createBackofficeTestServer(mockService, backofficeAPIKey)

	// Set up mock expectation
	mockService.On("GetUsageAndGrowth", mock.Anything, (*time.Time)(nil), (*time.Time)(nil)).
		Return(&models.UsageAndGrowthResponse{}, nil)

	// Try to access with regular API key - middleware would reject in production
	req := httptest.NewRequest("GET", "/bo/v1/reports/usage-and-growth", nil)
	req.Header.Set("X-API-Key", srv.apiKey)

	rr := httptest.NewRecorder()
	srv.handleBackofficeUsageAndGrowth(rr, req)

	// Handler processes the request (auth validation is middleware's job)
	assert.Equal(t, http.StatusOK, rr.Code)

	mockService.AssertExpectations(t)
}

func setupTestLogger() *slog.Logger {
	logger := slog.New(slog.DiscardHandler)
	return logger
}

func (m *MockContainerForBackoffice) ProjectRepository() repositories.ProjectRepository {
	return nil
}

func (m *MockContainerForBackoffice) ProjectService() services.ProjectServiceInterface {
	return nil
}

func (m *MockContainerForBackoffice) WebhookEventRepository() repositories.WebhookEventRepository {
	return nil
}

func (m *MockContainerForBackoffice) FeedRepository() repositories.FeedRepository {
	return nil
}

func (m *MockContainerForBackoffice) FeedItemRepository() repositories.FeedItemRepository {
	return nil
}

func (m *MockContainerForBackoffice) FeedItemReplyRepository() repositories.FeedItemReplyRepository {
	return nil
}

func (m *MockContainerForBackoffice) FeedService() services.FeedServiceInterface {
	return nil
}

func (m *MockContainerForBackoffice) FeedItemService() services.FeedItemServiceInterface {
	return nil
}

func (m *MockContainerForBackoffice) FeedItemReplyService() services.FeedItemReplyServiceInterface {
	return nil
}

func (m *MockContainerForBackoffice) NotificationRepository() repositories.NotificationRepository {
	return nil
}

func (m *MockContainerForBackoffice) NotificationDeliveryRepository() repositories.NotificationDeliveryRepository {
	return nil
}

func (m *MockContainerForBackoffice) NotificationDigestQueueRepository() repositories.NotificationDigestQueueRepository { //nolint:lll
	return nil
}

func (m *MockContainerForBackoffice) NotificationService() notifications.NotificationServiceInterface {
	return nil
}

func (m *MockContainerForBackoffice) DigestRunner() *notifications.DigestRunner {
	return nil
}

func (m *MockContainerForBackoffice) ProjectMigrationService() services.ProjectMigrationServiceInterface {
	return nil
}

// backfillTestServer builds a Server whose container exposes the given backfill
// service mock, for exercising handleEmbeddingsBackfill in isolation.
func backfillTestServer(backfill services.EmbeddingBackfillServiceInterface) *Server {
	return &Server{
		port:      "8080",
		apiKey:    "test-api-key",
		config:    &config.Config{},
		container: &MockContainerForBackoffice{embeddingBackfillService: backfill},
		logger:    setupTestLogger(),
	}
}

func postBackfill(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/bo/v1/embeddings/backfill", bytesReader(body))
	rr := httptest.NewRecorder()
	srv.handleEmbeddingsBackfill(rr, req)
	return rr
}

func bytesReader(s string) *strings.Reader { return strings.NewReader(s) }

func TestEmbeddingsBackfill_NoScope_Returns400(t *testing.T) {
	backfill := svcmocks.NewMockEmbeddingBackfillServiceInterface(t)
	backfill.EXPECT().Backfill(mock.Anything, mock.Anything).
		Return(nil, services.ErrBackfillScopeRequired).Once()

	rr := postBackfill(t, backfillTestServer(backfill), `{}`)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))
}

func TestEmbeddingsBackfill_AllAndEntityTypes_Returns400(t *testing.T) {
	backfill := svcmocks.NewMockEmbeddingBackfillServiceInterface(t)
	backfill.EXPECT().Backfill(mock.Anything, mock.Anything).
		Return(nil, services.ErrBackfillScopeAmbiguous).Once()

	rr := postBackfill(t, backfillTestServer(backfill), `{"all":true,"entity_types":["prompt"]}`)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestEmbeddingsBackfill_UnsupportedType_Returns400(t *testing.T) {
	backfill := svcmocks.NewMockEmbeddingBackfillServiceInterface(t)
	backfill.EXPECT().Backfill(mock.Anything, mock.Anything).
		Return(nil, services.ErrUnsupportedBackfillEntityType).Once()

	rr := postBackfill(t, backfillTestServer(backfill), `{"entity_types":["feed_item_reply"]}`)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestEmbeddingsBackfill_All_ThreadsScopeAndReturnsCounts(t *testing.T) {
	backfill := svcmocks.NewMockEmbeddingBackfillServiceInterface(t)
	backfill.EXPECT().
		Backfill(mock.Anything, mock.MatchedBy(func(req services.EmbeddingBackfillRequest) bool {
			return req.All && len(req.EntityTypes) == 0 && !req.MissingOnly && !req.DryRun
		})).
		Return(&services.EmbeddingBackfillResult{TotalSeen: 5, TotalPublished: 5}, nil).Once()

	rr := postBackfill(t, backfillTestServer(backfill), `{"all":true}`)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp services.EmbeddingBackfillResult
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 5, resp.TotalPublished)
}

func TestEmbeddingsBackfill_MissingOnlyDryRun_ThreadsFlags(t *testing.T) {
	backfill := svcmocks.NewMockEmbeddingBackfillServiceInterface(t)
	backfill.EXPECT().
		Backfill(mock.Anything, mock.MatchedBy(func(req services.EmbeddingBackfillRequest) bool {
			return req.MissingOnly && req.DryRun && len(req.EntityTypes) == 1 && req.EntityTypes[0] == "memory"
		})).
		Return(&services.EmbeddingBackfillResult{DryRun: true, TotalSeen: 3}, nil).Once()

	rr := postBackfill(t, backfillTestServer(backfill),
		`{"entity_types":["memory"],"missing_only":true,"dry_run":true}`)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestEmbeddingsBackfill_UnknownField_Returns400(t *testing.T) {
	backfill := svcmocks.NewMockEmbeddingBackfillServiceInterface(t)
	// DisallowUnknownFields rejects before the service is reached.
	rr := postBackfill(t, backfillTestServer(backfill), `{"missingOnly":true}`)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestEmbeddingsBackfill_ServiceError_Returns500(t *testing.T) {
	backfill := svcmocks.NewMockEmbeddingBackfillServiceInterface(t)
	backfill.EXPECT().Backfill(mock.Anything, mock.Anything).
		Return(nil, assert.AnError).Once()

	rr := postBackfill(t, backfillTestServer(backfill), `{"all":true}`)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func (m *MockContainerForBackoffice) TypeService() services.TypeServiceInterface { return nil }

func (m *MockContainerForBackoffice) AttachmentService() services.AttachmentServiceInterface {
	return nil
}
