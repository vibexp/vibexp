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

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// MockAPIKeyContainer implements Container interface for API key handler tests
type MockAPIKeyContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	apiKeyService *svcmocks.MockAPIKeyServiceInterface
}

func (m *MockAPIKeyContainer) APIKeyService() services.APIKeyServiceInterface {
	return m.apiKeyService
}

func (m *MockAPIKeyContainer) ActivityService() activities.ActivityService {
	return nil // Return nil for activity service as it's handled gracefully in the handler
}

func newMockAPIKeyContainer(t *testing.T) *MockAPIKeyContainer {
	return &MockAPIKeyContainer{
		apiKeyService: svcmocks.NewMockAPIKeyServiceInterface(t),
	}
}

func createTestAPIKeyServer(container *MockAPIKeyContainer) *Server {
	cfg := &config.Config{}
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during test

	// Initialize router manually for testing
	r := chi.NewRouter()

	srv := &Server{
		port:      "8080",
		container: container,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	// Register routes manually (simplified version for testing)
	r.Route("/api/v1/api-keys", func(r chi.Router) {
		r.Get("/", srv.handleListAPIKeys)
		r.Post("/", srv.handleCreateAPIKey)
		r.Delete("/{id}", srv.handleDeleteAPIKey)
	})

	return srv
}

//nolint:unparam // userID parameter is used with different values in different test scenarios
func makeAuthenticatedAPIKeyRequest(method, path string, body interface{}, userID string) *http.Request {
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
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, userID))

	return req
}

// TestHandleListAPIKeys_Success tests successful list API keys with mocked service
func TestHandleListAPIKeys_Success(t *testing.T) {
	mockContainer := newMockAPIKeyContainer(t)

	expectedAPIKeys := []models.APIKey{
		{
			ID:        "key-1",
			Name:      "Test Key 1",
			KeyPrefix: "vbx_test1",
			UserID:    "user-123",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:        "key-2",
			Name:      "Test Key 2",
			KeyPrefix: "vbx_test2",
			UserID:    "user-123",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	mockContainer.apiKeyService.On("GetAPIKeysByUserID", mock.Anything, "user-123").
		Return(expectedAPIKeys, nil)

	srv := createTestAPIKeyServer(mockContainer)
	req := makeAuthenticatedAPIKeyRequest("GET", "/api/v1/api-keys", nil, "user-123")
	w := httptest.NewRecorder()

	srv.handleListAPIKeys(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response []models.APIKey
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 2)
	assert.Equal(t, "key-1", response[0].ID)
	assert.Equal(t, "Test Key 1", response[0].Name)
	assert.Equal(t, "vbx_test1", response[0].KeyPrefix)
	assert.Equal(t, "key-2", response[1].ID)
	assert.Equal(t, "Test Key 2", response[1].Name)

	mockContainer.apiKeyService.AssertExpectations(t)
}

// TestHandleListAPIKeys_EmptyResult tests list API keys with no results
func TestHandleListAPIKeys_EmptyResult(t *testing.T) {
	mockContainer := newMockAPIKeyContainer(t)

	mockContainer.apiKeyService.On("GetAPIKeysByUserID", mock.Anything, "user-123").
		Return([]models.APIKey{}, nil)

	srv := createTestAPIKeyServer(mockContainer)
	req := makeAuthenticatedAPIKeyRequest("GET", "/api/v1/api-keys", nil, "user-123")
	w := httptest.NewRecorder()

	srv.handleListAPIKeys(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []models.APIKey
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 0)

	mockContainer.apiKeyService.AssertExpectations(t)
}

// TestHandleListAPIKeys_ServiceError tests list API keys when service returns error
func TestHandleListAPIKeys_ServiceError(t *testing.T) {
	mockContainer := newMockAPIKeyContainer(t)

	mockContainer.apiKeyService.On("GetAPIKeysByUserID", mock.Anything, "user-123").
		Return(([]models.APIKey)(nil), errors.New("database error"))

	srv := createTestAPIKeyServer(mockContainer)
	req := makeAuthenticatedAPIKeyRequest("GET", "/api/v1/api-keys", nil, "user-123")
	w := httptest.NewRecorder()

	srv.handleListAPIKeys(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockContainer.apiKeyService.AssertExpectations(t)
}

// TestHandleCreateAPIKey_Success tests successful API key creation
func TestHandleCreateAPIKey_Success(t *testing.T) {
	mockContainer := newMockAPIKeyContainer(t)

	reqBody := &models.CreateAPIKeyRequest{
		Name:             "New API Key",
		IntegrationCodes: []string{"ai_tools", "cli", "mcp_server"},
	}

	expectedAPIKey := &models.APIKey{
		ID:           "key-new",
		Name:         "New API Key",
		KeyPrefix:    "ak_newkey",
		Integrations: []string{"ai_tools", "cli", "mcp_server"},
		UserID:       "user-123",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	fullKey := "ak_newkey_secretpart123456789" // gitleaks:allow - test fixture data

	mockContainer.apiKeyService.On(
		"GenerateAPIKey", mock.Anything, "user-123", "New API Key",
		[]string{"ai_tools", "cli", "mcp_server"},
	).Return(expectedAPIKey, fullKey, nil)

	srv := createTestAPIKeyServer(mockContainer)
	req := makeAuthenticatedAPIKeyRequest("POST", "/api/v1/api-keys", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.handleCreateAPIKey(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response models.CreateAPIKeyResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "key-new", response.APIKey.ID)
	assert.Equal(t, "New API Key", response.APIKey.Name)
	assert.Equal(t, []string{"ai_tools", "cli", "mcp_server"}, response.APIKey.Integrations)
	assert.Equal(t, "ak_newkey", response.KeyPrefix)
	assert.Equal(t, fullKey, response.FullKey)

	mockContainer.apiKeyService.AssertExpectations(t)
}

// TestHandleCreateAPIKey_ValidationError tests create API key with validation errors
//
//nolint:funlen // Test function requires comprehensive validation test cases
func TestHandleCreateAPIKey_ValidationError(t *testing.T) {
	tests := []struct {
		name          string
		reqBody       *models.CreateAPIKeyRequest
		expectedError string
	}{
		{
			name: "Missing name",
			reqBody: &models.CreateAPIKeyRequest{
				Name:             "",
				IntegrationCodes: []string{"ai_tools", "cli", "mcp_server"},
			},
			expectedError: "API key name is required",
		},
		{
			name: "Name too long",
			reqBody: &models.CreateAPIKeyRequest{
				Name:             string(make([]byte, 256)), // 256 characters
				IntegrationCodes: []string{"ai_tools", "cli", "mcp_server"},
			},
			expectedError: "API key name too long",
		},
		{
			name: "Missing integration_codes",
			reqBody: &models.CreateAPIKeyRequest{
				Name:             "Test Key",
				IntegrationCodes: []string{},
			},
			expectedError: "At least one integration must be selected",
		},
		{
			name: "Invalid integration_code",
			reqBody: &models.CreateAPIKeyRequest{
				Name:             "Test Key",
				IntegrationCodes: []string{"invalid_type"},
			},
			expectedError: "invalid integration code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockAPIKeyContainer(t)
			srv := createTestAPIKeyServer(mockContainer)

			// Fill name with 'a' characters if empty but need long string
			if tt.reqBody.Name == string(make([]byte, 256)) {
				tt.reqBody.Name = string(make([]rune, 256))
				for i := range tt.reqBody.Name {
					tt.reqBody.Name = tt.reqBody.Name[:i] + "a" + tt.reqBody.Name[i+1:]
				}
			}

			req := makeAuthenticatedAPIKeyRequest("POST", "/api/v1/api-keys", tt.reqBody, "user-123")
			w := httptest.NewRecorder()

			srv.handleCreateAPIKey(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			// RFC 9457 validation error format
			assert.Equal(t, "VALIDATION_FAILED", response["code"])
			assert.Equal(t, 400.0, response["status"])
			assert.NotEmpty(t, response["timestamp"])
			// request_id is present (may be empty if middleware not invoked in test)
			assert.Contains(t, response, "request_id")
			// Check that validation_errors array exists and contains field errors
			validationErrors, ok := response["validation_errors"].([]interface{})
			assert.True(t, ok)
			assert.NotEmpty(t, validationErrors)
		})
	}
}

// TestHandleCreateAPIKey_InvalidJSON tests create API key with invalid JSON
func TestHandleCreateAPIKey_InvalidJSON(t *testing.T) {
	mockContainer := newMockAPIKeyContainer(t)
	srv := createTestAPIKeyServer(mockContainer)

	req := httptest.NewRequest("POST", "/api/v1/api-keys", bytes.NewReader([]byte(`{"invalid": json}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "user-123"))
	w := httptest.NewRecorder()

	srv.handleCreateAPIKey(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid request body")
}

// TestHandleCreateAPIKey_ServiceError tests create API key when service returns error
func TestHandleCreateAPIKey_ServiceError(t *testing.T) {
	mockContainer := newMockAPIKeyContainer(t)

	reqBody := &models.CreateAPIKeyRequest{
		Name:             "New API Key",
		IntegrationCodes: []string{"ai_tools", "cli", "mcp_server"},
	}

	mockContainer.apiKeyService.On(
		"GenerateAPIKey", mock.Anything, "user-123", "New API Key",
		[]string{"ai_tools", "cli", "mcp_server"},
	).Return((*models.APIKey)(nil), "", errors.New("database error"))

	srv := createTestAPIKeyServer(mockContainer)
	req := makeAuthenticatedAPIKeyRequest("POST", "/api/v1/api-keys", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.handleCreateAPIKey(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to generate API key")

	mockContainer.apiKeyService.AssertExpectations(t)
}

// TestHandleDeleteAPIKey_Success tests successful API key deletion
func TestHandleDeleteAPIKey_Success(t *testing.T) {
	mockContainer := newMockAPIKeyContainer(t)

	mockContainer.apiKeyService.On("DeleteAPIKey", mock.Anything, "user-123", "key-123").
		Return(nil)

	srv := createTestAPIKeyServer(mockContainer)
	req := makeAuthenticatedAPIKeyRequest("DELETE", "/api/v1/api-keys/key-123", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	mockContainer.apiKeyService.AssertExpectations(t)
}

// TestHandleDeleteAPIKey_NotFound tests delete non-existent API key
func TestHandleDeleteAPIKey_NotFound(t *testing.T) {
	mockContainer := newMockAPIKeyContainer(t)

	// Production shape: APIKeyService.DeleteAPIKey passes the repository
	// sentinel through unwrapped. The old handler matched the literal string
	// "API key not found or not owned by user", which this error does not
	// equal — so this test fails against the string-equality code and pins
	// the errors.Is(repositories.ErrAPIKeyNotFound) 404 branch.
	mockContainer.apiKeyService.On("DeleteAPIKey", mock.Anything, "user-123", "non-existent").
		Return(repositories.ErrAPIKeyNotFound)

	srv := createTestAPIKeyServer(mockContainer)
	req := makeAuthenticatedAPIKeyRequest("DELETE", "/api/v1/api-keys/non-existent", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "API key not found")

	mockContainer.apiKeyService.AssertExpectations(t)
}

// TestHandleDeleteAPIKey_ServiceError tests delete API key when service returns error
func TestHandleDeleteAPIKey_ServiceError(t *testing.T) {
	mockContainer := newMockAPIKeyContainer(t)

	mockContainer.apiKeyService.On("DeleteAPIKey", mock.Anything, "user-123", "key-123").
		Return(errors.New("database error"))

	srv := createTestAPIKeyServer(mockContainer)
	req := makeAuthenticatedAPIKeyRequest("DELETE", "/api/v1/api-keys/key-123", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to delete API key")

	mockContainer.apiKeyService.AssertExpectations(t)
}

// TestHandleDeleteAPIKey_EmptyID tests delete API key with empty ID
func TestHandleDeleteAPIKey_EmptyID(t *testing.T) {
	mockContainer := newMockAPIKeyContainer(t)
	srv := createTestAPIKeyServer(mockContainer)

	req := makeAuthenticatedAPIKeyRequest("DELETE", "/api/v1/api-keys/", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	// Chi router will return 405 (Method Not Allowed) for trailing slash without path parameter
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
