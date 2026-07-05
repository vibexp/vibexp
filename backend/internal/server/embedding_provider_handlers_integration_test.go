package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// MockEmbeddingProviderContainer implements Container interface for embedding provider handler tests
type MockEmbeddingProviderContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	embeddingProviderService *svcmocks.MockEmbeddingProviderServiceInterface
}

func (m *MockEmbeddingProviderContainer) EmbeddingProviderService() services.EmbeddingProviderServiceInterface {
	return m.embeddingProviderService
}

func newMockEmbeddingProviderContainer(t *testing.T) *MockEmbeddingProviderContainer {
	return &MockEmbeddingProviderContainer{
		embeddingProviderService: svcmocks.NewMockEmbeddingProviderServiceInterface(t),
	}
}

func createTestEmbeddingProviderServer(container *MockEmbeddingProviderContainer) *Server {
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

	// Register routes manually (simplified version for testing)
	r.Route("/api/v1/{team_id}/embedding-providers", func(r chi.Router) {
		r.Get("/", srv.handleListEmbeddingProviders)
		r.Post("/", srv.handleCreateEmbeddingProvider)
		r.Get("/{id}", srv.handleGetEmbeddingProvider)
		r.Put("/{id}", srv.handleUpdateEmbeddingProvider)
		r.Delete("/{id}", srv.handleDeleteEmbeddingProvider)
		r.Post("/validate", srv.handleValidateEmbeddingProvider)
	})

	return srv
}

//nolint:unparam // userID parameter is used with same value in tests, but kept for consistency
func makeAuthenticatedEmbeddingProviderRequest(method, path string, body interface{}, userID string) *http.Request {
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

// TestHandleListEmbeddingProviders_Success tests successful list embedding providers
func TestHandleListEmbeddingProviders_Success(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	baseURL := "https://api.openai.com/v1"
	expectedProviders := []models.EmbeddingProviderResponse{
		{
			EmbeddingProvider: models.EmbeddingProvider{
				ID:            "provider-1",
				UserID:        "user-123",
				Name:          "OpenAI Provider",
				ProviderType:  "openai",
				IsDefault:     true,
				BaseURL:       &baseURL,
				Configuration: "{}",
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			},
			HasAPIKey: true,
		},
		{
			EmbeddingProvider: models.EmbeddingProvider{
				ID:            "provider-2",
				UserID:        "user-123",
				Name:          "Anthropic Provider",
				ProviderType:  "anthropic",
				IsDefault:     false,
				BaseURL:       &baseURL,
				Configuration: "{}",
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			},
			HasAPIKey: false,
		},
	}

	mockContainer.embeddingProviderService.On("GetEmbeddingProvidersByTeamID", mock.Anything, "team-123").
		Return(expectedProviders, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("GET", "/api/v1/team-123/embedding-providers", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []models.EmbeddingProviderResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 2)
	assert.Equal(t, "provider-1", response[0].ID)
	assert.Equal(t, "OpenAI Provider", response[0].Name)
	assert.True(t, response[0].HasAPIKey)
	assert.Equal(t, "provider-2", response[1].ID)
	assert.False(t, response[1].HasAPIKey)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleListEmbeddingProviders_Empty tests list with no providers
func TestHandleListEmbeddingProviders_Empty(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	mockContainer.embeddingProviderService.On("GetEmbeddingProvidersByTeamID", mock.Anything, "team-123").
		Return([]models.EmbeddingProviderResponse{}, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("GET", "/api/v1/team-123/embedding-providers", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []models.EmbeddingProviderResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 0)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleListEmbeddingProviders_ServiceError tests list when service returns error
func TestHandleListEmbeddingProviders_ServiceError(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	mockContainer.embeddingProviderService.On("GetEmbeddingProvidersByTeamID", mock.Anything, "team-123").
		Return(([]models.EmbeddingProviderResponse)(nil), errors.New("database error"))

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("GET", "/api/v1/team-123/embedding-providers", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleGetEmbeddingProvider_Success tests successful get embedding provider
func TestHandleGetEmbeddingProvider_Success(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	baseURL := "https://api.openai.com/v1"
	expectedProvider := &models.EmbeddingProviderResponse{
		EmbeddingProvider: models.EmbeddingProvider{
			ID:            "provider-1",
			UserID:        "user-123",
			Name:          "OpenAI Provider",
			ProviderType:  "openai",
			IsDefault:     true,
			BaseURL:       &baseURL,
			Configuration: `{"model":"text-embedding-3-small"}`,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		},
		HasAPIKey: true,
	}

	mockContainer.embeddingProviderService.On("GetEmbeddingProvider", mock.Anything, "team-123", "provider-1").
		Return(expectedProvider, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("GET", "/api/v1/team-123/embedding-providers/provider-1", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.EmbeddingProviderResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "provider-1", response.ID)
	assert.Equal(t, "OpenAI Provider", response.Name)
	assert.Equal(t, "openai", response.ProviderType)
	assert.True(t, response.HasAPIKey)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleGetEmbeddingProvider_NotFound tests get non-existent provider
func TestHandleGetEmbeddingProvider_NotFound(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	mockContainer.embeddingProviderService.On("GetEmbeddingProvider", mock.Anything, "team-123", "non-existent").
		Return((*models.EmbeddingProviderResponse)(nil), services.ErrProviderNotFound)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("GET", "/api/v1/team-123/embedding-providers/non-existent", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleCreateEmbeddingProvider_Success tests successful provider creation
func TestHandleCreateEmbeddingProvider_Success(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	baseURL := "https://api.openai.com/v1"
	apiKey := "sk-test-key"
	isDefault := false

	reqBody := models.CreateEmbeddingProviderRequest{
		Name:         "New OpenAI Provider",
		ProviderType: "openai",
		Model:        "text-embedding-3-small",
		BaseURL:      &baseURL,
		APIKey:       &apiKey,
		IsDefault:    &isDefault,
		Configuration: map[string]interface{}{
			"model": "text-embedding-3-small",
		},
	}

	apiKeyEncrypted := "encrypted-key"
	expectedProvider := &models.EmbeddingProvider{
		ID:              "provider-new",
		UserID:          "user-123",
		Name:            "New OpenAI Provider",
		ProviderType:    "openai",
		IsDefault:       false,
		BaseURL:         &baseURL,
		APIKeyEncrypted: &apiKeyEncrypted,
		Configuration:   `{"model":"text-embedding-3-small"}`,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	mockContainer.embeddingProviderService.On("CreateEmbeddingProvider", mock.Anything, "team-123", "user-123", reqBody).
		Return(expectedProvider, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("POST", "/api/v1/team-123/embedding-providers", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.EmbeddingProviderResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "provider-new", response.ID)
	assert.Equal(t, "New OpenAI Provider", response.Name)
	assert.Equal(t, "openai", response.ProviderType)
	// API key should be masked in response
	assert.Nil(t, response.APIKeyEncrypted)
	assert.True(t, response.HasAPIKey)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleCreateEmbeddingProvider_Anthropic tests creating an Anthropic provider
func TestHandleCreateEmbeddingProvider_Anthropic(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	baseURL := "https://api.anthropic.com"
	apiKey := "sk-ant-test" //nolint:gosec // Test API key

	reqBody := models.CreateEmbeddingProviderRequest{
		Name:         "Anthropic Provider",
		ProviderType: "anthropic",
		Model:        "text-embedding-3-small",
		BaseURL:      &baseURL,
		APIKey:       &apiKey,
	}

	apiKeyEncrypted := "encrypted-anthropic-key"
	expectedProvider := &models.EmbeddingProvider{
		ID:              "provider-anthropic",
		UserID:          "user-123",
		Name:            "Anthropic Provider",
		ProviderType:    "anthropic",
		IsDefault:       false,
		BaseURL:         &baseURL,
		APIKeyEncrypted: &apiKeyEncrypted,
		Configuration:   "{}",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	mockContainer.embeddingProviderService.On("CreateEmbeddingProvider", mock.Anything, "team-123", "user-123", reqBody).
		Return(expectedProvider, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("POST", "/api/v1/team-123/embedding-providers", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.EmbeddingProviderResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "anthropic", response.ProviderType)
	assert.Nil(t, response.APIKeyEncrypted)
	assert.True(t, response.HasAPIKey)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleCreateEmbeddingProvider_Custom tests creating a custom provider
func TestHandleCreateEmbeddingProvider_Custom(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	baseURL := "https://custom-api.example.com/v1"
	apiKey := "custom-key"

	reqBody := models.CreateEmbeddingProviderRequest{
		Name:         "Custom Provider",
		ProviderType: "custom",
		Model:        "text-embedding-3-small",
		BaseURL:      &baseURL,
		APIKey:       &apiKey,
	}

	apiKeyEncrypted := "encrypted-custom-key"
	expectedProvider := &models.EmbeddingProvider{
		ID:              "provider-custom",
		UserID:          "user-123",
		Name:            "Custom Provider",
		ProviderType:    "custom",
		IsDefault:       false,
		BaseURL:         &baseURL,
		APIKeyEncrypted: &apiKeyEncrypted,
		Configuration:   "{}",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	mockContainer.embeddingProviderService.On("CreateEmbeddingProvider", mock.Anything, "team-123", "user-123", reqBody).
		Return(expectedProvider, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("POST", "/api/v1/team-123/embedding-providers", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.EmbeddingProviderResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "custom", response.ProviderType)
	assert.Nil(t, response.APIKeyEncrypted)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleCreateEmbeddingProvider_ValidationError tests validation errors
func TestHandleCreateEmbeddingProvider_ValidationError(t *testing.T) {
	tests := []struct {
		name          string
		reqBody       models.CreateEmbeddingProviderRequest
		expectedError string
	}{
		{
			name: "Missing name",
			reqBody: models.CreateEmbeddingProviderRequest{
				ProviderType: "openai",
			},
			expectedError: "Embedding provider name is required",
		},
		{
			name: "Missing provider type",
			reqBody: models.CreateEmbeddingProviderRequest{
				Name: "Test Provider",
			},
			expectedError: "Provider type is required",
		},
		{
			name: "Missing model",
			reqBody: models.CreateEmbeddingProviderRequest{
				Name:         "Test Provider",
				ProviderType: "openai",
			},
			expectedError: "Model is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockEmbeddingProviderContainer(t)
			srv := createTestEmbeddingProviderServer(mockContainer)

			req := makeAuthenticatedEmbeddingProviderRequest("POST", "/api/v1/team-123/embedding-providers", tt.reqBody, "user-123")
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestHandleCreateEmbeddingProvider_ServiceError tests service error
func TestHandleCreateEmbeddingProvider_ServiceError(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	baseURL := "https://api.openai.com/v1"
	reqBody := models.CreateEmbeddingProviderRequest{
		Name:         "Test Provider",
		ProviderType: "openai",
		Model:        "text-embedding-3-small",
		BaseURL:      &baseURL,
	}

	mockContainer.embeddingProviderService.On("CreateEmbeddingProvider", mock.Anything, "team-123", "user-123", reqBody).
		Return((*models.EmbeddingProvider)(nil), errors.New("failed to create provider"))

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("POST", "/api/v1/team-123/embedding-providers", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleUpdateEmbeddingProvider_Success tests successful provider update
func TestHandleUpdateEmbeddingProvider_Success(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	newName := "Updated Provider Name"
	reqBody := models.UpdateEmbeddingProviderRequest{
		Name: &newName,
	}

	baseURL := "https://api.openai.com/v1"
	apiKeyEncrypted := "encrypted-key"
	updatedProvider := &models.EmbeddingProvider{
		ID:              "provider-1",
		UserID:          "user-123",
		Name:            newName,
		ProviderType:    "openai",
		IsDefault:       true,
		BaseURL:         &baseURL,
		APIKeyEncrypted: &apiKeyEncrypted,
		Configuration:   "{}",
		CreatedAt:       time.Now().Add(-24 * time.Hour),
		UpdatedAt:       time.Now(),
	}

	mockContainer.embeddingProviderService.On("UpdateEmbeddingProvider", mock.Anything, "team-123", "provider-1", reqBody).
		Return(updatedProvider, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("PUT", "/api/v1/team-123/embedding-providers/provider-1", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.EmbeddingProviderResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, newName, response.Name)
	assert.Nil(t, response.APIKeyEncrypted)
	assert.True(t, response.HasAPIKey)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleUpdateEmbeddingProvider_PartialUpdate tests partial field update
func TestHandleUpdateEmbeddingProvider_PartialUpdate(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	newProviderType := "anthropic"
	isDefault := true
	reqBody := models.UpdateEmbeddingProviderRequest{
		ProviderType: &newProviderType,
		IsDefault:    &isDefault,
	}

	baseURL := "https://api.anthropic.com"
	apiKeyEncrypted := "encrypted-key"
	updatedProvider := &models.EmbeddingProvider{
		ID:              "provider-1",
		UserID:          "user-123",
		Name:            "Original Name",
		ProviderType:    newProviderType,
		IsDefault:       true,
		BaseURL:         &baseURL,
		APIKeyEncrypted: &apiKeyEncrypted,
		Configuration:   "{}",
		CreatedAt:       time.Now().Add(-24 * time.Hour),
		UpdatedAt:       time.Now(),
	}

	mockContainer.embeddingProviderService.On("UpdateEmbeddingProvider", mock.Anything, "team-123", "provider-1", reqBody).
		Return(updatedProvider, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("PUT", "/api/v1/team-123/embedding-providers/provider-1", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.EmbeddingProviderResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, newProviderType, response.ProviderType)
	assert.True(t, response.IsDefault)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleUpdateEmbeddingProvider_NotFound tests update non-existent provider
func TestHandleUpdateEmbeddingProvider_NotFound(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	newName := "Updated Name"
	reqBody := models.UpdateEmbeddingProviderRequest{
		Name: &newName,
	}

	mockContainer.embeddingProviderService.On(
		"UpdateEmbeddingProvider", mock.Anything, "team-123", "non-existent", reqBody,
	).Return((*models.EmbeddingProvider)(nil), services.ErrProviderNotFound)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest(
		"PUT", "/api/v1/team-123/embedding-providers/non-existent", reqBody, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleUpdateEmbeddingProvider_ServiceError tests update service error
func TestHandleUpdateEmbeddingProvider_ServiceError(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	newName := "Updated Name"
	reqBody := models.UpdateEmbeddingProviderRequest{
		Name: &newName,
	}

	mockContainer.embeddingProviderService.On("UpdateEmbeddingProvider", mock.Anything, "team-123", "provider-1", reqBody).
		Return((*models.EmbeddingProvider)(nil), errors.New("database error"))

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("PUT", "/api/v1/team-123/embedding-providers/provider-1", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleDeleteEmbeddingProvider_Success tests successful provider deletion
func TestHandleDeleteEmbeddingProvider_Success(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	mockContainer.embeddingProviderService.On("DeleteEmbeddingProvider", mock.Anything, "team-123", "provider-1").
		Return(nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("DELETE", "/api/v1/team-123/embedding-providers/provider-1", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleDeleteEmbeddingProvider_NotFound tests delete non-existent provider
func TestHandleDeleteEmbeddingProvider_NotFound(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	mockContainer.embeddingProviderService.On("DeleteEmbeddingProvider", mock.Anything, "team-123", "non-existent").
		Return(services.ErrProviderNotFound)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("DELETE", "/api/v1/team-123/embedding-providers/non-existent", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleDeleteEmbeddingProvider_LastProvider tests delete last provider error
func TestHandleDeleteEmbeddingProvider_LastProvider(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	mockContainer.embeddingProviderService.On("DeleteEmbeddingProvider", mock.Anything, "team-123", "provider-1").
		Return(services.ErrLastProviderDelete)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("DELETE", "/api/v1/team-123/embedding-providers/provider-1", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleValidateEmbeddingProvider_Success tests successful provider validation
func TestHandleValidateEmbeddingProvider_Success(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	reqBody := models.ValidateEmbeddingProviderRequest{
		ProviderType: "openai",
		Model:        "text-embedding-3-small",
		BaseURL:      "https://api.openai.com/v1",
	}

	expectedResponse := &models.ValidateEmbeddingProviderResponse{
		IsValid: true,
		Message: "Provider configuration is valid",
		Details: models.ValidateEmbeddingProviderDetails{
			ResponseTime: 100,
			StatusCode:   200,
		},
	}

	mockContainer.embeddingProviderService.On("ValidateEmbeddingProvider", mock.Anything, reqBody).
		Return(expectedResponse, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("POST", "/api/v1/team-123/embedding-providers/validate", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ValidateEmbeddingProviderResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response.IsValid)
	assert.Equal(t, "Provider configuration is valid", response.Message)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleValidateEmbeddingProvider_Invalid tests validation failure
func TestHandleValidateEmbeddingProvider_Invalid(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	apiKey := "invalid-key"
	reqBody := models.ValidateEmbeddingProviderRequest{
		ProviderType: "openai",
		Model:        "text-embedding-3-small",
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       &apiKey,
	}

	expectedResponse := &models.ValidateEmbeddingProviderResponse{
		IsValid: false,
		Message: "Invalid API key",
		Details: models.ValidateEmbeddingProviderDetails{
			StatusCode:   401,
			ErrorDetails: "Authentication failed",
		},
	}

	mockContainer.embeddingProviderService.On("ValidateEmbeddingProvider", mock.Anything, reqBody).
		Return(expectedResponse, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("POST", "/api/v1/team-123/embedding-providers/validate", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ValidateEmbeddingProviderResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.False(t, response.IsValid)
	assert.Equal(t, "Invalid API key", response.Message)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleValidateEmbeddingProvider_ValidationError tests request validation errors
func TestHandleValidateEmbeddingProvider_ValidationError(t *testing.T) {
	tests := []struct {
		name          string
		reqBody       models.ValidateEmbeddingProviderRequest
		expectedError string
	}{
		{
			name: "Missing provider type",
			reqBody: models.ValidateEmbeddingProviderRequest{
				BaseURL: "https://api.openai.com/v1",
			},
			expectedError: "Provider type is required",
		},
		{
			name: "Missing base URL",
			reqBody: models.ValidateEmbeddingProviderRequest{
				ProviderType: "openai",
				Model:        "m",
			},
			expectedError: "Base URL is required",
		},
		{
			name: "Missing model",
			reqBody: models.ValidateEmbeddingProviderRequest{
				ProviderType: "openai",
				BaseURL:      "https://api.openai.com/v1",
			},
			expectedError: "Model is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockEmbeddingProviderContainer(t)
			srv := createTestEmbeddingProviderServer(mockContainer)

			req := makeAuthenticatedEmbeddingProviderRequest(
				"POST", "/api/v1/team-123/embedding-providers/validate", tt.reqBody, "user-123",
			)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestHandleValidateEmbeddingProvider_ServiceError tests validation service error
func TestHandleValidateEmbeddingProvider_ServiceError(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	reqBody := models.ValidateEmbeddingProviderRequest{
		ProviderType: "openai",
		Model:        "text-embedding-3-small",
		BaseURL:      "https://api.openai.com/v1",
	}

	mockContainer.embeddingProviderService.On("ValidateEmbeddingProvider", mock.Anything, reqBody).
		Return((*models.ValidateEmbeddingProviderResponse)(nil), errors.New("network error"))

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("POST", "/api/v1/team-123/embedding-providers/validate", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleValidateEmbeddingProvider_WithConfiguration tests validation with configuration
func TestHandleValidateEmbeddingProvider_WithConfiguration(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	reqBody := models.ValidateEmbeddingProviderRequest{
		ProviderType: "openai",
		Model:        "text-embedding-3-small",
		BaseURL:      "https://api.openai.com/v1",
		Configuration: map[string]interface{}{
			"model":   "text-embedding-3-small",
			"timeout": 30,
		},
	}

	expectedResponse := &models.ValidateEmbeddingProviderResponse{
		IsValid: true,
		Message: "Provider configuration is valid",
		Details: models.ValidateEmbeddingProviderDetails{
			ResponseTime: 150,
			StatusCode:   200,
		},
	}

	mockContainer.embeddingProviderService.On(
		"ValidateEmbeddingProvider",
		mock.Anything,
		mock.MatchedBy(func(req models.ValidateEmbeddingProviderRequest) bool {
			return req.ProviderType == "openai" &&
				req.BaseURL == "https://api.openai.com/v1" &&
				req.Configuration != nil
		}),
	).Return(expectedResponse, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest("POST", "/api/v1/team-123/embedding-providers/validate", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.ValidateEmbeddingProviderResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response.IsValid)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}
