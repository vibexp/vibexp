package server

import (
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
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/services/notifications"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
	"github.com/vibexp/vibexp/pkg/events"
)

func TestContactSendMessage_Success(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	// Valid contact form request
	validBody := `{
		"name": "John Doe",
		"email": "john@example.com",
		"message": "This is a test message for the contact form functionality."
	}`

	req, err := http.NewRequest("POST", "/api/v1/website/contact/send-message", strings.NewReader(validBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// With stub email provider, the request should succeed
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}
}

func TestContactSendMessage_InvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name string
		body string
	}{
		{"Malformed JSON", `{"name": "John", "email": "invalid json}`},
		{"Invalid JSON syntax", `{"name": "John" "email": "test@example.com"}`},
		{"Empty JSON", ``},
		{"Non-JSON body", `this is not json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/api/v1/website/contact/send-message", strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != http.StatusBadRequest {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, http.StatusBadRequest)
			}
		})
	}
}

func runContactValidationTest(t *testing.T, srv *Server, body string) {
	t.Helper()
	rr := makeRequest(t, srv, testRequest{
		Method: "POST",
		Path:   "/api/v1/website/contact/send-message",
		Body:   body,
	})
	assertStatus(t, rr.Code, http.StatusBadRequest)
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %v", ct)
	}
}

func TestContactSendMessage_ValidationErrors(t *testing.T) {
	srv := testServer()
	validMsg := "This is a test message for validation."

	t.Run("Missing name", func(t *testing.T) {
		body := `{"email": "test@example.com", "message": "` + validMsg + `"}`
		runContactValidationTest(t, srv, body)
	})
	t.Run("Missing email", func(t *testing.T) {
		body := `{"name": "John Doe", "message": "` + validMsg + `"}`
		runContactValidationTest(t, srv, body)
	})
	t.Run("Missing message", func(t *testing.T) {
		runContactValidationTest(t, srv, `{"name": "John Doe", "email": "test@example.com"}`)
	})
	t.Run("Empty name", func(t *testing.T) {
		body := `{"name": "", "email": "test@example.com", "message": "` + validMsg + `"}`
		runContactValidationTest(t, srv, body)
	})
	t.Run("Empty email", func(t *testing.T) {
		body := `{"name": "John Doe", "email": "", "message": "` + validMsg + `"}`
		runContactValidationTest(t, srv, body)
	})
	t.Run("Empty message", func(t *testing.T) {
		runContactValidationTest(t, srv, `{"name": "John Doe", "email": "test@example.com", "message": ""}`)
	})
	t.Run("Name too short", func(t *testing.T) {
		body := `{"name": "J", "email": "test@example.com", "message": "` + validMsg + `"}`
		runContactValidationTest(t, srv, body)
	})
	t.Run("Name too long", func(t *testing.T) {
		body := `{"name": "` + strings.Repeat("a", 101) +
			`", "email": "test@example.com", "message": "` + validMsg + `"}`
		runContactValidationTest(t, srv, body)
	})
	t.Run("Invalid email format", func(t *testing.T) {
		body := `{"name": "John Doe", "email": "invalid-email", "message": "` + validMsg + `"}`
		runContactValidationTest(t, srv, body)
	})
	t.Run("Message too short", func(t *testing.T) {
		body := `{"name": "John Doe", "email": "test@example.com", "message": "short"}`
		runContactValidationTest(t, srv, body)
	})
	t.Run("Message too long", func(t *testing.T) {
		body := `{"name": "John Doe", "email": "test@example.com", "message": "` +
			strings.Repeat("a", 1001) + `"}`
		runContactValidationTest(t, srv, body)
	})
	t.Run("Phone number too short", func(t *testing.T) {
		body := `{"name": "John Doe", "email": "test@example.com", "message": "` +
			validMsg + `", "phone_number": "123"}`
		runContactValidationTest(t, srv, body)
	})
	t.Run("Phone number too long", func(t *testing.T) {
		body := `{"name": "John Doe", "email": "test@example.com", "message": "` +
			validMsg + `", "phone_number": "` + strings.Repeat("1", 21) + `"}`
		runContactValidationTest(t, srv, body)
	})
}

func TestContactSendMessage_ValidWithPhoneNumber(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	// Valid contact form request with phone number
	validBody := `{
		"name": "John Doe",
		"email": "john@example.com",
		"phone_number": "+1234567890",
		"message": "This is a test message for the contact form functionality with phone number."
	}`

	req, err := http.NewRequest("POST", "/api/v1/website/contact/send-message", strings.NewReader(validBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// With stub email provider, the request should succeed
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}
}

func TestContactSendMessage_MethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name   string
		method string
	}{
		{"GET method", "GET"},
		{"PUT method", "PUT"},
		{"DELETE method", "DELETE"},
		{"PATCH method", "PATCH"},
		{"HEAD method", "HEAD"},
		{"OPTIONS method", "OPTIONS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, "/api/v1/website/contact/send-message", nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != http.StatusMethodNotAllowed {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestContactSendMessage_ContentTypeHandling(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	validBody := `{
		"name": "John Doe",
		"email": "john@example.com",
		"message": "This is a test message for content type testing."
	}`

	tests := []struct {
		name        string
		contentType string
		expected    int
	}{
		// With stub email provider, these should all succeed
		{"Valid JSON content type", "application/json", http.StatusOK},
		{"Missing content type", "", http.StatusOK}, // Should still work
		// Should still work as JSON is parsed from body
		{"Wrong content type", "text/plain", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/api/v1/website/contact/send-message", strings.NewReader(validBody))
			if err != nil {
				t.Fatal(err)
			}
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestContactSendMessage_EdgeCases(t *testing.T) {
	srv := testServer()

	const contactPath = "/api/v1/website/contact/send-message"
	const validMsg = "This is a test message for validation."

	tests := []testCase{
		{
			Name: "Exact minimum name length", Method: "POST", Path: contactPath,
			Body:     `{"name": "Jo", "email": "test@example.com", "message": "` + validMsg + `"}`,
			Expected: http.StatusOK,
		},
		{
			Name: "Exact maximum name length", Method: "POST", Path: contactPath,
			Body: `{"name": "` + strings.Repeat("a", 100) +
				`", "email": "test@example.com", "message": "` + validMsg + `"}`,
			Expected: http.StatusOK,
		},
		{
			Name: "Exact minimum message length", Method: "POST", Path: contactPath,
			Body:     `{"name": "John Doe", "email": "test@example.com", "message": "` + strings.Repeat("a", 10) + `"}`,
			Expected: http.StatusOK,
		},
		{
			Name: "Exact maximum message length", Method: "POST", Path: contactPath,
			Body: `{"name": "John Doe", "email": "test@example.com", "message": "` +
				strings.Repeat("a", 1000) + `"}`,
			Expected: http.StatusOK,
		},
		{
			Name: "Exact minimum phone length", Method: "POST", Path: contactPath,
			Body: `{"name": "John Doe", "email": "test@example.com", "message": "` + validMsg +
				`", "phone_number": "` + strings.Repeat("1", 10) + `"}`,
			Expected: http.StatusOK,
		},
		{
			Name: "Exact maximum phone length", Method: "POST", Path: contactPath,
			Body: `{"name": "John Doe", "email": "test@example.com", "message": "` + validMsg +
				`", "phone_number": "` + strings.Repeat("1", 20) + `"}`,
			Expected: http.StatusOK,
		},
		{
			Name: "Valid email with plus sign", Method: "POST", Path: contactPath,
			Body:     `{"name": "John Doe", "email": "john+test@example.com", "message": "` + validMsg + `"}`,
			Expected: http.StatusOK,
		},
		{
			Name: "Valid email with subdomain", Method: "POST", Path: contactPath,
			Body:     `{"name": "John Doe", "email": "john@mail.example.com", "message": "` + validMsg + `"}`,
			Expected: http.StatusOK,
		},
	}

	runTestCases(t, srv, tests)
}

func TestContactSendMessage_InvalidPaths(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{"Wrong contact path", "/api/v1/website/contact", http.StatusNotFound},
		{"Invalid contact subpath", "/api/v1/website/contact/send", http.StatusNotFound},
		{"Missing website prefix", "/api/v1/contact/send-message", http.StatusNotFound},
		{"Wrong API version", "/api/v2/website/contact/send-message", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"name":"test","email":"test@example.com","message":"test message"}`
			req, err := http.NewRequest("POST", tt.path, strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

// MockEmailServiceForContact is a mock implementation of EmailServiceInterface for testing
type MockEmailServiceForContact struct {
	mock.Mock
}

func (m *MockEmailServiceForContact) SendContactMessage(req *models.ContactFormRequest) error {
	args := m.Called(req)
	return args.Error(0)
}

func (m *MockEmailServiceForContact) SendSupportRequest(userName, userEmail string, req *models.SupportRequest) error {
	args := m.Called(userName, userEmail, req)
	return args.Error(0)
}

func (m *MockEmailServiceForContact) SendTeamInvitation(
	invitation *models.TeamInvitation, teamName, inviterName string,
) error {
	args := m.Called(invitation, teamName, inviterName)
	return args.Error(0)
}

func (m *MockEmailServiceForContact) SendNotificationEmail(to, subject, htmlBody string) error {
	args := m.Called(to, subject, htmlBody)
	return args.Error(0)
}

// MockContainerForContact is a mock container specifically for contact handler testing
type MockContainerForContact struct {
	BaseMockContainer
	emailService *MockEmailServiceForContact
}

func (m *MockContainerForContact) EmailService() services.EmailServiceInterface {
	return m.emailService
}

// Implement all required container.Container interface methods
func (m *MockContainerForContact) UserRepository() repositories.UserRepository         { return nil }
func (m *MockContainerForContact) APIKeyRepository() repositories.APIKeyRepository     { return nil }
func (m *MockContainerForContact) PromptRepository() repositories.PromptRepository     { return nil }
func (m *MockContainerForContact) ArtifactRepository() repositories.ArtifactRepository { return nil }
func (m *MockContainerForContact) EmbeddingProviderRepository() repositories.EmbeddingProviderRepository {
	return nil
}
func (m *MockContainerForContact) ActivityRepository() repositories.ActivityRepository { return nil }
func (m *MockContainerForContact) ResourceAccessRepository() repositories.ResourceAccessRepository {
	return nil
}
func (m *MockContainerForContact) ClaudeCodeHooksRepository() repositories.ClaudeCodeHooksRepository {
	return nil
}
func (m *MockContainerForContact) CursorIDEHooksRepository() repositories.CursorIDEHooksRepository {
	return nil
}
func (m *MockContainerForContact) AgentRepository() repositories.AgentRepository { return nil }
func (m *MockContainerForContact) AgentExecutionRepository() repositories.AgentExecutionRepository {
	return nil
}
func (m *MockContainerForContact) AgentExecutionEventRepository() repositories.AgentExecutionEventRepository {
	return nil
}
func (m *MockContainerForContact) MemoryRepository() repositories.MemoryRepository       { return nil }
func (m *MockContainerForContact) EmbeddingRepository() repositories.EmbeddingRepository { return nil }
func (m *MockContainerForContact) BackofficeRepository() repositories.BackofficeRepository {
	return nil
}
func (m *MockContainerForContact) AuthService() services.AuthServiceInterface     { return nil }
func (m *MockContainerForContact) APIKeyService() services.APIKeyServiceInterface { return nil }
func (m *MockContainerForContact) PromptService() services.PromptServiceInterface { return nil }
func (m *MockContainerForContact) PromptGalleryService() services.PromptGalleryServiceInterface {
	return nil
}
func (m *MockContainerForContact) PromptGalleryRepository() repositories.PromptGalleryRepository {
	return nil
}
func (m *MockContainerForContact) PromptShareRepository() repositories.PromptShareRepository {
	return nil
}
func (m *MockContainerForContact) PromptShareService() services.PromptShareServiceInterface {
	return nil
}
func (m *MockContainerForContact) ArtifactService() services.ArtifactServiceInterface { return nil }
func (m *MockContainerForContact) EmbeddingProviderService() services.EmbeddingProviderServiceInterface {
	return nil
}
func (m *MockContainerForContact) ActivityService() activities.ActivityService { return nil }
func (m *MockContainerForContact) ResourceAccessService() resourceaccess.ResourceAccessService {
	return nil
}
func (m *MockContainerForContact) AgentService() services.AgentServiceInterface         { return nil }
func (m *MockContainerForContact) AgentCardFetcher() services.AgentCardFetcherInterface { return nil }
func (m *MockContainerForContact) AgentInvocationService() services.AgentInvocationServiceInterface {
	return nil
}
func (m *MockContainerForContact) MemoryService() services.MemoryServiceInterface       { return nil }
func (m *MockContainerForContact) EmbeddingService() services.EmbeddingServiceInterface { return nil }
func (m *MockContainerForContact) SearchService() services.SearchServiceInterface       { return nil }
func (m *MockContainerForContact) EnvironmentService() *services.EnvironmentService     { return nil }
func (m *MockContainerForContact) ResourceUsageService() services.ResourceUsageServiceInterface {
	return nil
}
func (m *MockContainerForContact) BackofficeService() services.BackofficeServiceInterface { return nil }
func (m *MockContainerForContact) EmbeddingBackfillService() services.EmbeddingBackfillServiceInterface {
	return nil
}
func (m *MockContainerForContact) BlueprintService() services.BlueprintServiceInterface {
	return nil
}
func (m *MockContainerForContact) UserPreferencesRepository() repositories.UserPreferencesRepository {
	return nil
}
func (m *MockContainerForContact) UserPreferencesService() services.UserPreferencesServiceInterface {
	return nil
}
func (m *MockContainerForContact) TeamRepository() repositories.TeamRepository { return nil }
func (m *MockContainerForContact) TeamMemberRepository() repositories.TeamMemberRepository {
	return nil
}
func (m *MockContainerForContact) TeamService() services.TeamServiceInterface             { return nil }
func (m *MockContainerForContact) TeamInvitationService() *services.TeamInvitationService { return nil }
func (m *MockContainerForContact) TeamSubscriptionRepository() repositories.TeamSubscriptionRepository {
	return nil
}
func (m *MockContainerForContact) GitHubAppClient() external.GitHubAppClient { return nil }
func (m *MockContainerForContact) GitHubAppService() services.GitHubAppServiceInterface {
	return nil
}
func (m *MockContainerForContact) GitHubInstallationRepository() repositories.GitHubInstallationRepository {
	return nil
}

func (m *MockContainerForContact) EventManager() events.EventPublisher { return nil }
func (m *MockContainerForContact) Close() error                        { return nil }
func (m *MockContainerForContact) DeviceTokenRepository() repositories.DeviceTokenRepository {
	return nil
}

// Ensure MockContainerForContact implements container.Container
var _ container.Container = (*MockContainerForContact)(nil)

// Integration Tests with Mocked Services

func TestContactSendMessage_IntegrationSuccess(t *testing.T) {
	// Setup
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	mockEmailService := new(MockEmailServiceForContact)
	mockContainer := &MockContainerForContact{
		emailService: mockEmailService,
	}

	srv := &Server{
		port:      "8080",
		apiKey:    "test-api-key",
		config:    cfg,
		container: mockContainer,
		logger:    logger,
	}

	// Test case: successful message sending
	validBody := `{
		"name": "John Doe",
		"email": "john@example.com",
		"message": "This is a test message for integration testing with mocked email service."
	}`

	// Mock email service to return success
	mockEmailService.On("SendContactMessage", mock.MatchedBy(func(req *models.ContactFormRequest) bool {
		return req.Name == "John Doe" &&
			req.Email == "john@example.com" &&
			strings.Contains(req.Message, "integration testing")
	})).Return(nil)

	req := httptest.NewRequest("POST", "/api/v1/website/contact/send-message", strings.NewReader(validBody))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.handleContactSendMessage(rr, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rr.Code, "Expected status OK")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	// Verify response structure
	var response models.ContactFormResponse
	err := json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err, "Response should be valid JSON")
	assert.True(t, response.Success, "Response should indicate success")
	assert.Contains(t, response.Message, "Thank you", "Response should contain thank you message")

	// Verify mock was called
	mockEmailService.AssertExpectations(t)
}

func TestContactSendMessage_IntegrationWithPhoneNumber(t *testing.T) {
	// Setup
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	mockEmailService := new(MockEmailServiceForContact)
	mockContainer := &MockContainerForContact{
		emailService: mockEmailService,
	}

	srv := &Server{
		port:      "8080",
		apiKey:    "test-api-key",
		config:    cfg,
		container: mockContainer,
		logger:    logger,
	}

	phoneNumber := "+1234567890"
	validBody := `{
		"name": "Jane Smith",
		"email": "jane@example.com",
		"phone_number": "+1234567890",
		"message": "This is a test message with phone number included."
	}`

	// Mock email service with phone number verification
	mockEmailService.On("SendContactMessage", mock.MatchedBy(func(req *models.ContactFormRequest) bool {
		return req.Name == "Jane Smith" &&
			req.Email == "jane@example.com" &&
			req.PhoneNumber != nil &&
			*req.PhoneNumber == phoneNumber
	})).Return(nil)

	req := httptest.NewRequest("POST", "/api/v1/website/contact/send-message", strings.NewReader(validBody))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.handleContactSendMessage(rr, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.ContactFormResponse
	err := json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err)
	assert.True(t, response.Success)

	mockEmailService.AssertExpectations(t)
}

func TestContactSendMessage_IntegrationEmailServiceError(t *testing.T) {
	// Setup
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	mockEmailService := new(MockEmailServiceForContact)
	mockContainer := &MockContainerForContact{
		emailService: mockEmailService,
	}

	srv := &Server{
		port:      "8080",
		apiKey:    "test-api-key",
		config:    cfg,
		container: mockContainer,
		logger:    logger,
	}

	validBody := `{
		"name": "Test User",
		"email": "test@example.com",
		"message": "This message will fail to send due to email service error."
	}`

	// Mock email service to return error
	mockEmailService.On("SendContactMessage", mock.AnythingOfType("*models.ContactFormRequest")).
		Return(assert.AnError)

	req := httptest.NewRequest("POST", "/api/v1/website/contact/send-message", strings.NewReader(validBody))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.handleContactSendMessage(rr, req)

	// Assertions
	assert.Equal(t, http.StatusInternalServerError, rr.Code, "Should return internal server error")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	// Verify error response structure
	var response models.ContactFormResponse
	err := json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err, "Response should be valid JSON")
	assert.False(t, response.Success, "Response should indicate failure")
	assert.Contains(t, response.Message, "Failed to send message", "Error message should be present")

	mockEmailService.AssertExpectations(t)
}

//nolint:funlen // Table-driven test with multiple test cases
func TestContactSendMessage_IntegrationEmailValidation(t *testing.T) {
	// Setup
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	mockEmailService := new(MockEmailServiceForContact)
	mockContainer := &MockContainerForContact{
		emailService: mockEmailService,
	}

	srv := &Server{
		port:      "8080",
		apiKey:    "test-api-key",
		config:    cfg,
		container: mockContainer,
		logger:    logger,
	}

	tests := []struct {
		name          string
		email         string
		shouldSucceed bool
	}{
		{
			name:          "Valid standard email",
			email:         "user@example.com",
			shouldSucceed: true,
		},
		{
			name:          "Valid email with plus",
			email:         "user+tag@example.com",
			shouldSucceed: true,
		},
		{
			name:          "Valid email with subdomain",
			email:         "user@mail.example.com",
			shouldSucceed: true,
		},
		{
			name:          "Invalid email - no @",
			email:         "userexample.com",
			shouldSucceed: false,
		},
		{
			name:          "Invalid email - no domain",
			email:         "user@",
			shouldSucceed: false,
		},
		{
			name:          "Invalid email - no local part",
			email:         "@example.com",
			shouldSucceed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{
				"name": "Test User",
				"email": "` + tt.email + `",
				"message": "Test message for email validation."
			}`

			if tt.shouldSucceed {
				// Only set up mock for valid emails
				mockEmailService.On("SendContactMessage", mock.AnythingOfType("*models.ContactFormRequest")).
					Return(nil).Once()
			}

			req := httptest.NewRequest("POST", "/api/v1/website/contact/send-message", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.handleContactSendMessage(rr, req)

			if tt.shouldSucceed {
				assert.Equal(t, http.StatusOK, rr.Code, "Valid email should succeed")
			} else {
				assert.Equal(t, http.StatusBadRequest, rr.Code, "Invalid email should fail validation")
			}
		})
	}

	mockEmailService.AssertExpectations(t)
}

//nolint:funlen // Table-driven test with multiple test cases
func TestContactSendMessage_IntegrationMessageLengthValidation(t *testing.T) {
	// Setup
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	mockEmailService := new(MockEmailServiceForContact)
	mockContainer := &MockContainerForContact{
		emailService: mockEmailService,
	}

	srv := &Server{
		port:      "8080",
		apiKey:    "test-api-key",
		config:    cfg,
		container: mockContainer,
		logger:    logger,
	}

	tests := []struct {
		name          string
		message       string
		shouldSucceed bool
	}{
		{
			name:          "Valid message - minimum length",
			message:       strings.Repeat("a", 10),
			shouldSucceed: true,
		},
		{
			name:          "Valid message - maximum length",
			message:       strings.Repeat("a", 1000),
			shouldSucceed: true,
		},
		{
			name:          "Valid message - normal length",
			message:       "This is a normal test message for the contact form.",
			shouldSucceed: true,
		},
		{
			name:          "Invalid message - too short",
			message:       "short",
			shouldSucceed: false,
		},
		{
			name:          "Invalid message - too long",
			message:       strings.Repeat("a", 1001),
			shouldSucceed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{
				"name": "Test User",
				"email": "test@example.com",
				"message": "` + tt.message + `"
			}`

			if tt.shouldSucceed {
				mockEmailService.On("SendContactMessage", mock.AnythingOfType("*models.ContactFormRequest")).
					Return(nil).Once()
			}

			req := httptest.NewRequest("POST", "/api/v1/website/contact/send-message", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.handleContactSendMessage(rr, req)

			if tt.shouldSucceed {
				assert.Equal(t, http.StatusOK, rr.Code, "Valid message length should succeed")

				var response models.ContactFormResponse
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.True(t, response.Success)
			} else {
				assert.Equal(t, http.StatusBadRequest, rr.Code, "Invalid message length should fail")

				var response models.ContactFormResponse
				err := json.NewDecoder(rr.Body).Decode(&response)
				assert.NoError(t, err)
				assert.False(t, response.Success)
			}
		})
	}

	mockEmailService.AssertExpectations(t)
}

func TestContactSendMessage_IntegrationRequestPayloadVerification(t *testing.T) {
	// Setup
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	mockEmailService := new(MockEmailServiceForContact)
	mockContainer := &MockContainerForContact{
		emailService: mockEmailService,
	}

	srv := &Server{
		port:      "8080",
		apiKey:    "test-api-key",
		config:    cfg,
		container: mockContainer,
		logger:    logger,
	}

	expectedName := "Alice Johnson"
	expectedEmail := "alice.johnson@example.com"
	expectedMessage := "This is a detailed test message to verify that " +
		"the payload is correctly passed to the email service."

	body := `{
		"name": "` + expectedName + `",
		"email": "` + expectedEmail + `",
		"message": "` + expectedMessage + `"
	}`

	// Mock with strict payload verification
	mockEmailService.On("SendContactMessage", mock.MatchedBy(func(req *models.ContactFormRequest) bool {
		// Verify exact payload values
		assert.Equal(t, expectedName, req.Name, "Name should match exactly")
		assert.Equal(t, expectedEmail, req.Email, "Email should match exactly")
		assert.Equal(t, expectedMessage, req.Message, "Message should match exactly")
		assert.Nil(t, req.PhoneNumber, "PhoneNumber should be nil when not provided")
		return true
	})).Return(nil)

	req := httptest.NewRequest("POST", "/api/v1/website/contact/send-message", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.handleContactSendMessage(rr, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rr.Code)

	var response models.ContactFormResponse
	err := json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err)
	assert.True(t, response.Success)
	assert.NotEmpty(t, response.Message)

	mockEmailService.AssertExpectations(t)
}

func (m *MockContainerForContact) ProjectRepository() repositories.ProjectRepository {
	return nil
}

func (m *MockContainerForContact) ProjectService() services.ProjectServiceInterface {
	return nil
}

func (m *MockContainerForContact) WebhookEventRepository() repositories.WebhookEventRepository {
	return nil
}

func (m *MockContainerForContact) FeedRepository() repositories.FeedRepository {
	return nil
}

func (m *MockContainerForContact) FeedItemRepository() repositories.FeedItemRepository {
	return nil
}

func (m *MockContainerForContact) FeedItemReplyRepository() repositories.FeedItemReplyRepository {
	return nil
}

func (m *MockContainerForContact) FeedService() services.FeedServiceInterface {
	return nil
}

func (m *MockContainerForContact) FeedItemService() services.FeedItemServiceInterface {
	return nil
}

func (m *MockContainerForContact) FeedItemReplyService() services.FeedItemReplyServiceInterface {
	return nil
}

func (m *MockContainerForContact) NotificationRepository() repositories.NotificationRepository {
	return nil
}

func (m *MockContainerForContact) NotificationDeliveryRepository() repositories.NotificationDeliveryRepository {
	return nil
}

func (m *MockContainerForContact) NotificationDigestQueueRepository() repositories.NotificationDigestQueueRepository {
	return nil
}

func (m *MockContainerForContact) NotificationService() notifications.NotificationServiceInterface {
	return nil
}

func (m *MockContainerForContact) DigestRunner() *notifications.DigestRunner {
	return nil
}

func (m *MockContainerForContact) ProjectMigrationService() services.ProjectMigrationServiceInterface {
	return nil
}

func (m *MockContainerForContact) TypeService() services.TypeServiceInterface { return nil }

func (m *MockContainerForContact) AttachmentService() services.AttachmentServiceInterface { return nil }
