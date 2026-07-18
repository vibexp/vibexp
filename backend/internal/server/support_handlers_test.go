package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/container"
	vibexperrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	"github.com/vibexp/vibexp/pkg/events"
)

// MockEmailServiceForSupport is a mock implementation of EmailServiceInterface for support handler testing
type MockEmailServiceForSupport struct {
	mock.Mock
}

func (m *MockEmailServiceForSupport) SendSupportRequest(userName, userEmail string, req *models.SupportRequest) error {
	args := m.Called(userName, userEmail, req)
	return args.Error(0)
}

func (m *MockEmailServiceForSupport) SendTeamInvitation(
	invitation *models.TeamInvitation, teamName, inviterName string,
) error {
	args := m.Called(invitation, teamName, inviterName)
	return args.Error(0)
}

func (m *MockEmailServiceForSupport) SendNotificationEmail(to, subject, htmlBody string) error {
	args := m.Called(to, subject, htmlBody)
	return args.Error(0)
}

// MockContainerForSupport is a mock container specifically for support handler testing
type MockContainerForSupport struct {
	BaseMockContainer
	emailService *MockEmailServiceForSupport
	authService  *svcmocks.MockAuthServiceInterface
}

func (m *MockContainerForSupport) EmailService() services.EmailServiceInterface {
	return m.emailService
}

func (m *MockContainerForSupport) AuthService() services.AuthServiceInterface {
	return m.authService
}

// Implement all required container.Container interface methods
func (m *MockContainerForSupport) UserRepository() repositories.UserRepository         { return nil }
func (m *MockContainerForSupport) APIKeyRepository() repositories.APIKeyRepository     { return nil }
func (m *MockContainerForSupport) PromptRepository() repositories.PromptRepository     { return nil }
func (m *MockContainerForSupport) ArtifactRepository() repositories.ArtifactRepository { return nil }
func (m *MockContainerForSupport) EmbeddingProviderRepository() repositories.EmbeddingProviderRepository {
	return nil
}
func (m *MockContainerForSupport) ActivityRepository() repositories.ActivityRepository { return nil }
func (m *MockContainerForSupport) ResourceAccessRepository() repositories.ResourceAccessRepository {
	return nil
}
func (m *MockContainerForSupport) ClaudeCodeHooksRepository() repositories.ClaudeCodeHooksRepository {
	return nil
}
func (m *MockContainerForSupport) CursorIDEHooksRepository() repositories.CursorIDEHooksRepository {
	return nil
}
func (m *MockContainerForSupport) AgentRepository() repositories.AgentRepository { return nil }
func (m *MockContainerForSupport) AgentExecutionRepository() repositories.AgentExecutionRepository {
	return nil
}
func (m *MockContainerForSupport) AgentExecutionEventRepository() repositories.AgentExecutionEventRepository {
	return nil
}
func (m *MockContainerForSupport) MemoryRepository() repositories.MemoryRepository       { return nil }
func (m *MockContainerForSupport) EmbeddingRepository() repositories.EmbeddingRepository { return nil }
func (m *MockContainerForSupport) BackofficeRepository() repositories.BackofficeRepository {
	return nil
}
func (m *MockContainerForSupport) APIKeyService() services.APIKeyServiceInterface { return nil }
func (m *MockContainerForSupport) PromptService() services.PromptServiceInterface { return nil }
func (m *MockContainerForSupport) PromptGalleryService() services.PromptGalleryServiceInterface {
	return nil
}
func (m *MockContainerForSupport) PromptGalleryRepository() repositories.PromptGalleryRepository {
	return nil
}
func (m *MockContainerForSupport) PromptShareRepository() repositories.PromptShareRepository {
	return nil
}
func (m *MockContainerForSupport) PromptShareService() services.PromptShareServiceInterface {
	return nil
}
func (m *MockContainerForSupport) ArtifactService() services.ArtifactServiceInterface { return nil }
func (m *MockContainerForSupport) EmbeddingProviderService() services.EmbeddingProviderServiceInterface {
	return nil
}
func (m *MockContainerForSupport) ActivityService() activities.ActivityService { return nil }
func (m *MockContainerForSupport) ResourceAccessService() resourceaccess.ResourceAccessService {
	return nil
}
func (m *MockContainerForSupport) AgentService() services.AgentServiceInterface { return nil }
func (m *MockContainerForSupport) AgentCardFetcher() services.CardFetcher       { return nil }
func (m *MockContainerForSupport) AgentInvocationService() services.AgentInvocationServiceInterface {
	return nil
}
func (m *MockContainerForSupport) MemoryService() services.MemoryServiceInterface       { return nil }
func (m *MockContainerForSupport) EmbeddingService() services.EmbeddingServiceInterface { return nil }
func (m *MockContainerForSupport) SearchService() services.Searcher                     { return nil }
func (m *MockContainerForSupport) EnvironmentService() *services.EnvironmentService     { return nil }
func (m *MockContainerForSupport) ResourceUsageService() services.ResourceUsageServiceInterface {
	return nil
}
func (m *MockContainerForSupport) BackofficeService() services.UsageAndGrowthGetter { return nil }
func (m *MockContainerForSupport) AdminService() services.AdminServiceInterface     { return nil }
func (m *MockContainerForSupport) EmbeddingBackfillService() services.EmbeddingBackfiller {
	return nil
}
func (m *MockContainerForSupport) BlueprintService() services.BlueprintServiceInterface {
	return nil
}
func (m *MockContainerForSupport) UserPreferencesRepository() repositories.UserPreferencesRepository {
	return nil
}
func (m *MockContainerForSupport) UserPreferencesService() services.UserPreferencesServiceInterface {
	return nil
}
func (m *MockContainerForSupport) TeamRepository() repositories.TeamRepository { return nil }
func (m *MockContainerForSupport) TeamMemberRepository() repositories.TeamMemberRepository {
	return nil
}
func (m *MockContainerForSupport) TeamService() services.TeamServiceInterface             { return nil }
func (m *MockContainerForSupport) TeamInvitationService() *services.TeamInvitationService { return nil }
func (m *MockContainerForSupport) TeamSubscriptionRepository() repositories.TeamSubscriptionRepository {
	return nil
}
func (m *MockContainerForSupport) GitHubAppClient() external.GitHubAppClient { return nil }
func (m *MockContainerForSupport) GitHubAppService() services.GitHubAppServiceInterface {
	return nil
}
func (m *MockContainerForSupport) GitHubInstallationRepository() repositories.GitHubInstallationRepository {
	return nil
}

func (m *MockContainerForSupport) Close() error                        { return nil }
func (m *MockContainerForSupport) EventManager() events.EventPublisher { return nil }
func (m *MockContainerForSupport) DeviceTokenRepository() repositories.DeviceTokenRepository {
	return nil
}

// Ensure MockContainerForSupport implements container.Container
var _ container.Container = (*MockContainerForSupport)(nil)

// Helper functions for support handler tests

// setupSupportTestServer creates a test server with mocked services
func setupSupportTestServer(
	emailSvc *MockEmailServiceForSupport,
	authSvc *svcmocks.MockAuthServiceInterface,
) *Server {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	mockContainer := &MockContainerForSupport{
		emailService: emailSvc,
		authService:  authSvc,
	}

	return &Server{
		port:      "8080",
		apiKey:    "test-api-key",
		config:    cfg,
		container: mockContainer,
		logger:    logger,
	}
}

// createSupportTestRequest creates an HTTP request with user context for testing
func createSupportTestRequest(userID, userEmail, body string) *http.Request {
	req := httptest.NewRequest("POST", "/api/v1/support/message", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), contextKeyUserID, userID)
	ctx = context.WithValue(ctx, contextKeyUserEmail, userEmail)
	return req.WithContext(ctx)
}

// assertSupportSuccessResponse asserts common success response properties
func assertSupportSuccessResponse(t *testing.T, rr *httptest.ResponseRecorder) {
	assert.Equal(t, http.StatusOK, rr.Code, "Expected status OK")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response models.SupportResponse
	err := json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err, "Response should be valid JSON")
	assert.True(t, response.Success, "Response should indicate success")
	assert.Contains(t, response.Message, "Thank you", "Response should contain thank you message")
}

// Integration Tests with Mocked Services

func TestHandleSupportMessage_IntegrationSuccess(t *testing.T) {
	mockEmailService := new(MockEmailServiceForSupport)
	mockAuthService := svcmocks.NewMockAuthServiceInterface(t)
	srv := setupSupportTestServer(mockEmailService, mockAuthService)

	userID := "test-user-123"
	userEmail := "test@example.com"
	userName := "Test User"

	validBody := `{
		"text": "This is a test support message for integration testing.",
		"acknowledgement": true
	}`

	testUser := &models.User{ID: userID, Email: userEmail, Name: userName}
	mockAuthService.On("GetUserByID", mock.Anything, userID).Return(testUser, nil)

	mockEmailService.On("SendSupportRequest", userName, userEmail, mock.MatchedBy(func(req *models.SupportRequest) bool {
		return req.Text == "This is a test support message for integration testing." &&
			req.Acknowledgement == true
	})).Return(nil)

	req := createSupportTestRequest(userID, userEmail, validBody)
	rr := httptest.NewRecorder()
	srv.handleSupportMessage(rr, req)

	assertSupportSuccessResponse(t, rr)
	mockAuthService.AssertExpectations(t)
	mockEmailService.AssertExpectations(t)
}

func TestHandleSupportMessage_IntegrationWithoutAcknowledgement(t *testing.T) {
	mockEmailService := new(MockEmailServiceForSupport)
	mockAuthService := svcmocks.NewMockAuthServiceInterface(t)
	srv := setupSupportTestServer(mockEmailService, mockAuthService)

	userID := "test-user-456"
	userEmail := "user@example.com"
	userName := "Test User 2"

	validBody := `{
		"text": "Test message without acknowledgement request.",
		"acknowledgement": false,
		"additional_info": {
			"page": "/dashboard",
			"browser": "Chrome"
		}
	}`

	testUser := &models.User{ID: userID, Email: userEmail, Name: userName}
	mockAuthService.On("GetUserByID", mock.Anything, userID).Return(testUser, nil)

	mockEmailService.On("SendSupportRequest", userName, userEmail, mock.MatchedBy(func(req *models.SupportRequest) bool {
		return req.Text == "Test message without acknowledgement request." &&
			req.Acknowledgement == false &&
			len(req.AdditionalInfo) == 2
	})).Return(nil)

	req := createSupportTestRequest(userID, userEmail, validBody)
	rr := httptest.NewRecorder()
	srv.handleSupportMessage(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.SupportResponse
	err := json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err)
	assert.True(t, response.Success)

	mockAuthService.AssertExpectations(t)
	mockEmailService.AssertExpectations(t)
}

func TestHandleSupportMessage_IntegrationEmailServiceError(t *testing.T) {
	// Setup
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	mockEmailService := new(MockEmailServiceForSupport)
	mockAuthService := svcmocks.NewMockAuthServiceInterface(t)
	mockContainer := &MockContainerForSupport{
		emailService: mockEmailService,
		authService:  mockAuthService,
	}

	srv := &Server{
		port:      "8080",
		apiKey:    "test-api-key",
		config:    cfg,
		container: mockContainer,
		logger:    logger,
	}

	userID := "test-user-789"
	userEmail := "error@example.com"
	userName := "Error User"

	validBody := `{
		"text": "This message will fail to send due to email service error.",
		"acknowledgement": false
	}`

	// Mock auth service to return a user
	testUser := &models.User{
		ID:    userID,
		Email: userEmail,
		Name:  userName,
	}
	mockAuthService.On("GetUserByID", mock.Anything, userID).Return(testUser, nil)

	// Mock email service to return error
	mockEmailService.On("SendSupportRequest", userName, userEmail, mock.AnythingOfType("*models.SupportRequest")).
		Return(assert.AnError)

	req := httptest.NewRequest("POST", "/api/v1/support/message", strings.NewReader(validBody))
	req.Header.Set("Content-Type", "application/json")

	// Add user context
	ctx := context.WithValue(req.Context(), contextKeyUserID, userID)
	ctx = context.WithValue(ctx, contextKeyUserEmail, userEmail)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	srv.handleSupportMessage(rr, req)

	// Assertions
	assert.Equal(t, http.StatusInternalServerError, rr.Code, "Should return internal server error")
	assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))

	// Verify error response structure (RFC 9457 format)
	var response vibexperrors.APIError
	err := json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err, "Response should be valid JSON")
	assert.Equal(t, "INTERNAL_ERROR", response.Code, "Error code should be INTERNAL_ERROR")
	assert.Contains(t, response.Detail, "Failed to send", "Error message should be present")

	mockAuthService.AssertExpectations(t)
	mockEmailService.AssertExpectations(t)
}

//nolint:funlen
func TestHandleSupportMessage_ValidationErrors(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	mockEmailService := new(MockEmailServiceForSupport)
	mockAuthService := svcmocks.NewMockAuthServiceInterface(t)
	mockContainer := &MockContainerForSupport{
		emailService: mockEmailService,
		authService:  mockAuthService,
	}

	srv := &Server{
		port:      "8080",
		apiKey:    "test-api-key",
		config:    cfg,
		container: mockContainer,
		logger:    logger,
	}

	userID := "test-user-validation"
	userEmail := "validation@example.com"
	userName := "Validation User"

	// Mock auth service to return a user
	testUser := &models.User{
		ID:    userID,
		Email: userEmail,
		Name:  userName,
	}
	mockAuthService.On("GetUserByID", mock.Anything, userID).Return(testUser, nil).Maybe()

	tests := []struct {
		name          string
		body          string
		shouldSucceed bool
	}{
		{
			name:          "Missing text",
			body:          `{"acknowledgement": true}`,
			shouldSucceed: false,
		},
		{
			name:          "Text too short",
			body:          `{"text": "short", "acknowledgement": true}`,
			shouldSucceed: false,
		},
		{
			name:          "Invalid JSON",
			body:          `{"text": "test" invalid json}`,
			shouldSucceed: false,
		},
		{
			name:          "Valid request - minimum text length",
			body:          `{"text": "` + strings.Repeat("a", 10) + `", "acknowledgement": true}`,
			shouldSucceed: true,
		},
		{
			name:          "Valid request - maximum text length",
			body:          `{"text": "` + strings.Repeat("a", 2000) + `", "acknowledgement": false}`,
			shouldSucceed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldSucceed {
				mockEmailService.On("SendSupportRequest", userName, userEmail, mock.AnythingOfType("*models.SupportRequest")).
					Return(nil).Once()
			}

			req := httptest.NewRequest("POST", "/api/v1/support/message", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			ctx := context.WithValue(req.Context(), contextKeyUserID, userID)
			ctx = context.WithValue(ctx, contextKeyUserEmail, userEmail)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			srv.handleSupportMessage(rr, req)

			if tt.shouldSucceed {
				assert.Equal(t, http.StatusOK, rr.Code, "Valid request should succeed")
			} else {
				assert.Equal(t, http.StatusBadRequest, rr.Code, "Invalid request should fail validation")
			}
		})
	}

	mockEmailService.AssertExpectations(t)
}

func TestHandleSupportMessage_IntegrationWithAdditionalInfo(t *testing.T) {
	mockEmailService := new(MockEmailServiceForSupport)
	mockAuthService := svcmocks.NewMockAuthServiceInterface(t)
	srv := setupSupportTestServer(mockEmailService, mockAuthService)

	userID := "test-user-additional"
	userEmail := "additional@example.com"
	userName := "Additional User"

	validBody := `{
		"text": "I need help with the subscription feature.",
		"acknowledgement": true,
		"additional_info": {
			"source_url": "/subscription",
			"user_agent": "Mozilla/5.0",
			"subscription_tier": "basic"
		}
	}`

	testUser := &models.User{ID: userID, Email: userEmail, Name: userName}
	mockAuthService.On("GetUserByID", mock.Anything, userID).Return(testUser, nil)

	mockEmailService.On("SendSupportRequest", userName, userEmail, mock.MatchedBy(func(req *models.SupportRequest) bool {
		return req.Text == "I need help with the subscription feature." &&
			req.Acknowledgement == true &&
			req.AdditionalInfo != nil &&
			req.AdditionalInfo["source_url"] == "/subscription" &&
			req.AdditionalInfo["user_agent"] == "Mozilla/5.0" &&
			req.AdditionalInfo["subscription_tier"] == "basic"
	})).Return(nil)

	req := createSupportTestRequest(userID, userEmail, validBody)
	rr := httptest.NewRecorder()
	srv.handleSupportMessage(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.SupportResponse
	err := json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err)
	assert.True(t, response.Success)

	mockAuthService.AssertExpectations(t)
	mockEmailService.AssertExpectations(t)
}

func (m *MockContainerForSupport) ProjectRepository() repositories.ProjectRepository {
	return nil
}

func (m *MockContainerForSupport) ProjectService() services.ProjectServiceInterface {
	return nil
}

func (m *MockContainerForSupport) WebhookEventRepository() repositories.WebhookEventRepository {
	return nil
}

func (m *MockContainerForSupport) FeedRepository() repositories.FeedRepository {
	return nil
}

func (m *MockContainerForSupport) FeedItemRepository() repositories.FeedItemRepository {
	return nil
}

func (m *MockContainerForSupport) FeedItemReplyRepository() repositories.FeedItemReplyRepository {
	return nil
}

func (m *MockContainerForSupport) FeedService() services.FeedServiceInterface {
	return nil
}

func (m *MockContainerForSupport) FeedItemService() services.FeedItemServiceInterface {
	return nil
}

func (m *MockContainerForSupport) FeedItemReplyService() services.FeedItemReplyServiceInterface {
	return nil
}

func (m *MockContainerForSupport) NotificationRepository() repositories.NotificationRepository {
	return nil
}

func (m *MockContainerForSupport) NotificationDeliveryRepository() repositories.NotificationDeliveryRepository {
	return nil
}

func (m *MockContainerForSupport) NotificationDigestQueueRepository() repositories.NotificationDigestQueueRepository {
	return nil
}

func (m *MockContainerForSupport) NotificationService() notifications.NotificationServiceInterface {
	return nil
}

func (m *MockContainerForSupport) DigestRunner() *notifications.DigestRunner {
	return nil
}

func (m *MockContainerForSupport) ProjectMigrationService() services.ProjectMigrationServiceInterface {
	return nil
}

func (m *MockContainerForSupport) TypeService() services.TypeServiceInterface { return nil }

func (m *MockContainerForSupport) AttachmentService() services.AttachmentServiceInterface { return nil }
