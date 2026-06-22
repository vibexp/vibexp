package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	vibexperrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// MockEmailServiceForSupportIntegration is a mock for EmailServiceInterface
type MockEmailServiceForSupportIntegration struct {
	mock.Mock
}

func (m *MockEmailServiceForSupportIntegration) SendContactMessage(req *models.ContactFormRequest) error {
	args := m.Called(req)
	return args.Error(0)
}

func (m *MockEmailServiceForSupportIntegration) SendSupportRequest(
	userName, userEmail string, req *models.SupportRequest,
) error {
	args := m.Called(userName, userEmail, req)
	return args.Error(0)
}

func (m *MockEmailServiceForSupportIntegration) SendTeamInvitation(
	invitation *models.TeamInvitation, teamName, inviterName string,
) error {
	args := m.Called(invitation, teamName, inviterName)
	return args.Error(0)
}

func (m *MockEmailServiceForSupportIntegration) SendNotificationEmail(to, subject, htmlBody string) error {
	args := m.Called(to, subject, htmlBody)
	return args.Error(0)
}

// MockSupportContainer implements Container interface for support handler tests
type MockSupportContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	authService  *svcmocks.MockAuthServiceInterface
	emailService *MockEmailServiceForSupportIntegration
}

func (m *MockSupportContainer) AuthService() services.AuthServiceInterface {
	return m.authService
}

func (m *MockSupportContainer) EmailService() services.EmailServiceInterface {
	return m.emailService
}

func newMockSupportContainer(t *testing.T) *MockSupportContainer {
	return &MockSupportContainer{
		authService:  svcmocks.NewMockAuthServiceInterface(t),
		emailService: &MockEmailServiceForSupportIntegration{},
	}
}

func createTestSupportServer(container *MockSupportContainer) *Server {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	// Initialize router manually for testing
	r := chi.NewRouter()

	srv := &Server{
		port:      "8080",
		container: container,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	// Register routes manually for testing
	r.Route("/api/v1/support", func(r chi.Router) {
		r.Post("/message", srv.handleSupportMessage)
	})

	return srv
}

//nolint:unparam // method parameter is kept for consistency with other test helpers
func makeAuthenticatedSupportRequest(method, path string, body interface{}, userID, userEmail string) *http.Request {
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
	ctx := context.WithValue(req.Context(), contextKeyUserID, userID)
	ctx = context.WithValue(ctx, contextKeyUserEmail, userEmail)
	req = req.WithContext(ctx)

	return req
}

// TestHandleSupportMessage_Success tests successful support message submission
func TestHandleSupportMessage_Success(t *testing.T) {
	mockContainer := newMockSupportContainer(t)

	testUser := &models.User{
		ID:    "user-123",
		Email: "test@example.com",
		Name:  "Test User",
	}

	mockContainer.authService.On("GetUserByID", mock.Anything, "user-123").
		Return(testUser, nil)
	mockContainer.emailService.On("SendSupportRequest",
		"Test User", "test@example.com",
		mock.AnythingOfType("*models.SupportRequest"),
	).Return(nil)

	srv := createTestSupportServer(mockContainer)

	reqBody := map[string]interface{}{
		"text":            "This is a test support message that is at least 10 characters long",
		"acknowledgement": true,
	}
	req := makeAuthenticatedSupportRequest("POST", "/api/v1/support/message", reqBody, "user-123", "test@example.com")
	w := httptest.NewRecorder()

	srv.handleSupportMessage(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))
	assert.Contains(t, response["message"], "Thank you")

	mockContainer.authService.AssertExpectations(t)
	mockContainer.emailService.AssertExpectations(t)
}

// TestHandleSupportMessage_WithoutAcknowledgement tests support message without acknowledgement
func TestHandleSupportMessage_WithoutAcknowledgement(t *testing.T) {
	mockContainer := newMockSupportContainer(t)

	testUser := &models.User{
		ID:    "user-123",
		Email: "test@example.com",
		Name:  "Test User",
	}

	mockContainer.authService.On("GetUserByID", mock.Anything, "user-123").
		Return(testUser, nil)
	mockContainer.emailService.On("SendSupportRequest",
		"Test User", "test@example.com",
		mock.MatchedBy(func(req *models.SupportRequest) bool {
			return !req.Acknowledgement
		}),
	).Return(nil)

	srv := createTestSupportServer(mockContainer)

	reqBody := map[string]interface{}{
		"text":            "This is a test support message that is at least 10 characters long",
		"acknowledgement": false,
	}
	req := makeAuthenticatedSupportRequest("POST", "/api/v1/support/message", reqBody, "user-123", "test@example.com")
	w := httptest.NewRecorder()

	srv.handleSupportMessage(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockContainer.authService.AssertExpectations(t)
	mockContainer.emailService.AssertExpectations(t)
}

// TestHandleSupportMessage_WithAdditionalInfo tests support message with additional info
func TestHandleSupportMessage_WithAdditionalInfo(t *testing.T) {
	mockContainer := newMockSupportContainer(t)

	testUser := &models.User{
		ID:    "user-123",
		Email: "test@example.com",
		Name:  "Test User",
	}

	mockContainer.authService.On("GetUserByID", mock.Anything, "user-123").
		Return(testUser, nil)
	mockContainer.emailService.On("SendSupportRequest",
		"Test User", "test@example.com",
		mock.MatchedBy(func(req *models.SupportRequest) bool {
			return len(req.AdditionalInfo) > 0
		}),
	).Return(nil)

	srv := createTestSupportServer(mockContainer)

	reqBody := map[string]interface{}{
		"text":            "This is a test support message that is at least 10 characters long",
		"acknowledgement": true,
		"additional_info": map[string]string{
			"browser":  "Chrome",
			"version":  "120.0",
			"platform": "Linux",
		},
	}
	req := makeAuthenticatedSupportRequest("POST", "/api/v1/support/message", reqBody, "user-123", "test@example.com")
	w := httptest.NewRecorder()

	srv.handleSupportMessage(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockContainer.authService.AssertExpectations(t)
	mockContainer.emailService.AssertExpectations(t)
}

// TestHandleSupportMessage_ValidationFailed_TextTooShort tests validation error for short text
func TestHandleSupportMessage_ValidationFailed_TextTooShort(t *testing.T) {
	mockContainer := newMockSupportContainer(t)

	srv := createTestSupportServer(mockContainer)

	reqBody := map[string]interface{}{
		"text":            "Short",
		"acknowledgement": true,
	}
	req := makeAuthenticatedSupportRequest("POST", "/api/v1/support/message", reqBody, "user-123", "test@example.com")
	w := httptest.NewRecorder()

	srv.handleSupportMessage(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var response vibexperrors.APIError
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "VALIDATION_FAILED", response.Code)
	assert.Contains(t, response.Detail, "validation failed")
	assert.NotEmpty(t, response.ValidationErrors)

	mockContainer.authService.AssertNotCalled(t, "GetUserByID")
	mockContainer.emailService.AssertNotCalled(t, "SendSupportRequest")
}

// TestHandleSupportMessage_ValidationFailed_TextTooLong tests validation error for long text
func TestHandleSupportMessage_ValidationFailed_TextTooLong(t *testing.T) {
	mockContainer := newMockSupportContainer(t)

	srv := createTestSupportServer(mockContainer)

	// Create a message that exceeds 2000 characters
	longText := make([]byte, 2001)
	for i := range longText {
		longText[i] = 'a'
	}

	reqBody := map[string]interface{}{
		"text":            string(longText),
		"acknowledgement": true,
	}
	req := makeAuthenticatedSupportRequest("POST", "/api/v1/support/message", reqBody, "user-123", "test@example.com")
	w := httptest.NewRecorder()

	srv.handleSupportMessage(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var response vibexperrors.APIError
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "VALIDATION_FAILED", response.Code)
}

// TestHandleSupportMessage_ValidationFailed_MissingText tests validation error for missing text
func TestHandleSupportMessage_ValidationFailed_MissingText(t *testing.T) {
	mockContainer := newMockSupportContainer(t)

	srv := createTestSupportServer(mockContainer)

	reqBody := map[string]interface{}{
		"acknowledgement": true,
	}
	req := makeAuthenticatedSupportRequest("POST", "/api/v1/support/message", reqBody, "user-123", "test@example.com")
	w := httptest.NewRecorder()

	srv.handleSupportMessage(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var response vibexperrors.APIError
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "VALIDATION_FAILED", response.Code)
}

// TestHandleSupportMessage_InvalidJSON tests error for invalid JSON
func TestHandleSupportMessage_InvalidJSON(t *testing.T) {
	mockContainer := newMockSupportContainer(t)

	srv := createTestSupportServer(mockContainer)

	req := httptest.NewRequest("POST", "/api/v1/support/message", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), contextKeyUserID, "user-123")
	ctx = context.WithValue(ctx, contextKeyUserEmail, "test@example.com")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	srv.handleSupportMessage(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var response vibexperrors.APIError
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "BAD_REQUEST", response.Code)
}

// TestHandleSupportMessage_UserNotFound tests error when user is not found
func TestHandleSupportMessage_UserNotFound(t *testing.T) {
	mockContainer := newMockSupportContainer(t)

	mockContainer.authService.On("GetUserByID", mock.Anything, "user-123").
		Return(&models.User{}, assert.AnError)

	srv := createTestSupportServer(mockContainer)

	reqBody := map[string]interface{}{
		"text":            "This is a test support message that is at least 10 characters long",
		"acknowledgement": true,
	}
	req := makeAuthenticatedSupportRequest("POST", "/api/v1/support/message", reqBody, "user-123", "test@example.com")
	w := httptest.NewRecorder()

	srv.handleSupportMessage(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var response vibexperrors.APIError
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "INTERNAL_ERROR", response.Code)

	mockContainer.authService.AssertExpectations(t)
	mockContainer.emailService.AssertNotCalled(t, "SendSupportRequest")
}

// TestHandleSupportMessage_EmailServiceError tests error when email service fails
func TestHandleSupportMessage_EmailServiceError(t *testing.T) {
	mockContainer := newMockSupportContainer(t)

	testUser := &models.User{
		ID:    "user-123",
		Email: "test@example.com",
		Name:  "Test User",
	}

	mockContainer.authService.On("GetUserByID", mock.Anything, "user-123").
		Return(testUser, nil)
	mockContainer.emailService.On("SendSupportRequest",
		"Test User", "test@example.com",
		mock.AnythingOfType("*models.SupportRequest"),
	).Return(assert.AnError)

	srv := createTestSupportServer(mockContainer)

	reqBody := map[string]interface{}{
		"text":            "This is a test support message that is at least 10 characters long",
		"acknowledgement": true,
	}
	req := makeAuthenticatedSupportRequest("POST", "/api/v1/support/message", reqBody, "user-123", "test@example.com")
	w := httptest.NewRecorder()

	srv.handleSupportMessage(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var response vibexperrors.APIError
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "INTERNAL_ERROR", response.Code)

	mockContainer.authService.AssertExpectations(t)
	mockContainer.emailService.AssertExpectations(t)
}

// TestHandleSupportMessage_UserWithoutName tests fallback to email when user has no name
func TestHandleSupportMessage_UserWithoutName(t *testing.T) {
	mockContainer := newMockSupportContainer(t)

	testUser := &models.User{
		ID:    "user-123",
		Email: "test@example.com",
		Name:  "", // Empty name
	}

	mockContainer.authService.On("GetUserByID", mock.Anything, "user-123").
		Return(testUser, nil)
	mockContainer.emailService.On("SendSupportRequest",
		"test@example.com", "test@example.com",
		mock.AnythingOfType("*models.SupportRequest"),
	).Return(nil)

	srv := createTestSupportServer(mockContainer)

	reqBody := map[string]interface{}{
		"text":            "This is a test support message that is at least 10 characters long",
		"acknowledgement": true,
	}
	req := makeAuthenticatedSupportRequest("POST", "/api/v1/support/message", reqBody, "user-123", "test@example.com")
	w := httptest.NewRecorder()

	srv.handleSupportMessage(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockContainer.authService.AssertExpectations(t)
	mockContainer.emailService.AssertExpectations(t)
}
