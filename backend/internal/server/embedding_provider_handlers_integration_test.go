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
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// MockEmbeddingProviderContainer implements Container interface for embedding provider handler tests
type MockEmbeddingProviderContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	embeddingProviderService *svcmocks.MockEmbeddingProviderServiceInterface
	embeddingRepository      *repomocks.MockEmbeddingRepository
	embeddingBackfillService *svcmocks.MockEmbeddingBackfillServiceInterface
	embeddingStatusService   *svcmocks.MockEmbeddingStatusServiceInterface
}

func (m *MockEmbeddingProviderContainer) EmbeddingProviderService() services.EmbeddingProviderServiceInterface {
	return m.embeddingProviderService
}

func (m *MockEmbeddingProviderContainer) EmbeddingStatusService() services.EmbeddingStatusServiceInterface {
	return m.embeddingStatusService
}

func (m *MockEmbeddingProviderContainer) EmbeddingRepository() repositories.EmbeddingRepository {
	return m.embeddingRepository
}

func (m *MockEmbeddingProviderContainer) EmbeddingBackfillService() services.EmbeddingBackfillServiceInterface {
	return m.embeddingBackfillService
}

func newMockEmbeddingProviderContainer(t *testing.T) *MockEmbeddingProviderContainer {
	return &MockEmbeddingProviderContainer{
		embeddingProviderService: svcmocks.NewMockEmbeddingProviderServiceInterface(t),
		embeddingRepository:      repomocks.NewMockEmbeddingRepository(t),
		embeddingBackfillService: svcmocks.NewMockEmbeddingBackfillServiceInterface(t),
		embeddingStatusService:   svcmocks.NewMockEmbeddingStatusServiceInterface(t),
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

	// Register routes via the production setup under both prefixes (bare and
	// settings) so tests exercise the same route tree the server mounts,
	// including the reprocess and coverage endpoints under each.
	r.Route("/api/v1/{team_id}/embedding-providers", srv.setupEmbeddingProvidersRoutes)
	r.Route("/api/v1/{team_id}/settings/embedding-providers", srv.setupEmbeddingProvidersRoutes)

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

// expectTeamReembed sets up the background, missing-only, team-scoped enqueue
// (Backfill with All + MissingOnly, no wipe) that both provider create and
// reprocess trigger, and registers a cleanup that waits for it to fire — so the
// async goroutine settles before the mock's own AssertExpectations cleanup runs
// (cleanups are LIFO, and this is registered after the mock's).
func expectTeamReembed(t *testing.T, c *MockEmbeddingProviderContainer) {
	t.Helper()
	done := make(chan struct{})
	c.embeddingBackfillService.
		On("Backfill", mock.Anything, mock.MatchedBy(func(r services.EmbeddingBackfillRequest) bool {
			return r.TeamID == "team-123" && r.All && r.MissingOnly
		})).
		Return(&services.EmbeddingBackfillResult{}, nil).
		Run(func(mock.Arguments) { close(done) })
	t.Cleanup(func() {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("background team re-embed was not triggered")
		}
	})
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
	expectTeamReembed(t, mockContainer)

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
	expectTeamReembed(t, mockContainer)

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
	expectTeamReembed(t, mockContainer)

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

	mockContainer.embeddingProviderService.
		On("GetEmbeddingProvider", mock.Anything, "team-123", "provider-1").
		Return(&models.EmbeddingProviderResponse{EmbeddingProvider: *updatedProvider}, nil)
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

	mockContainer.embeddingProviderService.
		On("GetEmbeddingProvider", mock.Anything, "team-123", "provider-1").
		Return(&models.EmbeddingProviderResponse{EmbeddingProvider: *updatedProvider}, nil)
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

	mockContainer.embeddingProviderService.
		On("GetEmbeddingProvider", mock.Anything, "team-123", "non-existent").
		Return((*models.EmbeddingProviderResponse)(nil), services.ErrProviderNotFound)
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

	mockContainer.embeddingProviderService.
		On("GetEmbeddingProvider", mock.Anything, "team-123", "provider-1").
		Return((*models.EmbeddingProviderResponse)(nil), errors.New("database error"))
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

// TestHandleUpdateEmbeddingProvider_ReembedsOnModelChange verifies that changing a
// provider's embedding identity (here the model) wipes the team's embeddings and
// triggers a background re-embed scoped to that team (issue #79).
func TestHandleUpdateEmbeddingProvider_ReembedsOnModelChange(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	baseURL := "https://api.openai.com/v1"
	oldProvider := &models.EmbeddingProviderResponse{
		EmbeddingProvider: models.EmbeddingProvider{
			ID: "provider-1", ProviderType: "openai_compatible",
			Model: "old-model", BaseURL: &baseURL,
		},
	}
	newModel := "new-model"
	reqBody := models.UpdateEmbeddingProviderRequest{Model: &newModel}
	updatedProvider := &models.EmbeddingProvider{
		ID: "provider-1", ProviderType: "openai_compatible",
		Model: "new-model", BaseURL: &baseURL,
	}

	mockContainer.embeddingProviderService.
		On("GetEmbeddingProvider", mock.Anything, "team-123", "provider-1").
		Return(oldProvider, nil)
	mockContainer.embeddingProviderService.
		On("UpdateEmbeddingProvider", mock.Anything, "team-123", "provider-1", reqBody).
		Return(updatedProvider, nil)
	mockContainer.embeddingRepository.
		On("DeleteByTeam", mock.Anything, "team-123").
		Return(int64(5), nil)

	backfillDone := make(chan struct{})
	mockContainer.embeddingBackfillService.
		On("Backfill", mock.Anything, mock.MatchedBy(func(r services.EmbeddingBackfillRequest) bool {
			return r.TeamID == "team-123" && r.All
		})).
		Return(&services.EmbeddingBackfillResult{}, nil).
		Run(func(mock.Arguments) { close(backfillDone) })

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest(
		"PUT", "/api/v1/team-123/embedding-providers/provider-1", reqBody, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Wait for the async re-embed to fire before the test (and its mocks) tear down.
	select {
	case <-backfillDone:
	case <-time.After(2 * time.Second):
		t.Fatal("background re-embed was not triggered on a model change")
	}

	mockContainer.embeddingRepository.AssertExpectations(t)
	mockContainer.embeddingBackfillService.AssertExpectations(t)
}

// TestHandleUpdateEmbeddingProvider_ReembedsOnDocumentPrefixChange verifies that
// changing document_prefix — which alters the text every document is embedded
// with — wipes the team's embeddings and triggers a background re-embed, exactly
// like a model change (issue #171).
func TestHandleUpdateEmbeddingProvider_ReembedsOnDocumentPrefixChange(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	baseURL := "https://api.openai.com/v1"
	oldProvider := &models.EmbeddingProviderResponse{
		EmbeddingProvider: models.EmbeddingProvider{
			ID: "provider-1", ProviderType: "openai_compatible",
			Model: "same-model", BaseURL: &baseURL,
		},
	}
	newDocPrefix := "passage: "
	reqBody := models.UpdateEmbeddingProviderRequest{DocumentPrefix: &newDocPrefix}
	updatedProvider := &models.EmbeddingProvider{
		ID: "provider-1", ProviderType: "openai_compatible",
		Model: "same-model", BaseURL: &baseURL, DocumentPrefix: &newDocPrefix,
	}

	mockContainer.embeddingProviderService.
		On("GetEmbeddingProvider", mock.Anything, "team-123", "provider-1").
		Return(oldProvider, nil)
	mockContainer.embeddingProviderService.
		On("UpdateEmbeddingProvider", mock.Anything, "team-123", "provider-1", reqBody).
		Return(updatedProvider, nil)
	mockContainer.embeddingRepository.
		On("DeleteByTeam", mock.Anything, "team-123").
		Return(int64(3), nil)

	backfillDone := make(chan struct{})
	mockContainer.embeddingBackfillService.
		On("Backfill", mock.Anything, mock.MatchedBy(func(r services.EmbeddingBackfillRequest) bool {
			return r.TeamID == "team-123" && r.All
		})).
		Return(&services.EmbeddingBackfillResult{}, nil).
		Run(func(mock.Arguments) { close(backfillDone) })

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest(
		"PUT", "/api/v1/team-123/embedding-providers/provider-1", reqBody, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	select {
	case <-backfillDone:
	case <-time.After(2 * time.Second):
		t.Fatal("background re-embed was not triggered on a document_prefix change")
	}

	mockContainer.embeddingRepository.AssertExpectations(t)
	mockContainer.embeddingBackfillService.AssertExpectations(t)
}

// TestHandleUpdateEmbeddingProvider_NoReembedOnQueryPrefixChange verifies that a
// query_prefix-only edit does NOT wipe or re-embed: the query prefix affects only
// the query side, so stored document vectors stay valid (issue #171). The absence
// of DeleteByTeam / Backfill expectations means either call would fail the mock.
func TestHandleUpdateEmbeddingProvider_NoReembedOnQueryPrefixChange(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	baseURL := "https://api.openai.com/v1"
	oldProvider := &models.EmbeddingProviderResponse{
		EmbeddingProvider: models.EmbeddingProvider{
			ID: "provider-1", ProviderType: "openai_compatible",
			Model: "same-model", BaseURL: &baseURL,
		},
	}
	newQueryPrefix := "query: "
	reqBody := models.UpdateEmbeddingProviderRequest{QueryPrefix: &newQueryPrefix}
	updatedProvider := &models.EmbeddingProvider{
		ID: "provider-1", ProviderType: "openai_compatible",
		Model: "same-model", BaseURL: &baseURL, QueryPrefix: &newQueryPrefix,
	}

	mockContainer.embeddingProviderService.
		On("GetEmbeddingProvider", mock.Anything, "team-123", "provider-1").
		Return(oldProvider, nil)
	mockContainer.embeddingProviderService.
		On("UpdateEmbeddingProvider", mock.Anything, "team-123", "provider-1", reqBody).
		Return(updatedProvider, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest(
		"PUT", "/api/v1/team-123/embedding-providers/provider-1", reqBody, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockContainer.embeddingRepository.AssertNotCalled(t, "DeleteByTeam", mock.Anything, mock.Anything)
	mockContainer.embeddingBackfillService.AssertNotCalled(t, "Backfill", mock.Anything, mock.Anything)
}

// providerForReprocess is the minimal provider the reprocess existence check
// resolves before enqueuing.
func providerForReprocess() *models.EmbeddingProviderResponse {
	baseURL := "https://api.openai.com/v1"
	return &models.EmbeddingProviderResponse{
		EmbeddingProvider: models.EmbeddingProvider{
			ID: "provider-1", ProviderType: "openai",
			Model: "text-embedding-3-small", BaseURL: &baseURL,
		},
	}
}

// TestHandleReprocessEmbeddingProvider_Success verifies reprocess returns a
// spec-conforming 202 and enqueues a background, missing-only, team-scoped
// re-embed (no wipe).
func TestHandleReprocessEmbeddingProvider_Success(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)
	mockContainer.embeddingProviderService.
		On("GetEmbeddingProvider", mock.Anything, "team-123", "provider-1").
		Return(providerForReprocess(), nil)
	expectTeamReembed(t, mockContainer)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest(
		"POST", "/api/v1/team-123/embedding-providers/provider-1/reprocess", nil, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleReprocessEmbeddingProviderSettings_SpecConformance covers the
// settings-route variant so its documented operation has a spec-validated
// response too.
func TestHandleReprocessEmbeddingProviderSettings_SpecConformance(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)
	mockContainer.embeddingProviderService.
		On("GetEmbeddingProvider", mock.Anything, "team-123", "provider-1").
		Return(providerForReprocess(), nil)
	expectTeamReembed(t, mockContainer)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest(
		"POST", "/api/v1/team-123/settings/embedding-providers/provider-1/reprocess", nil, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
}

// TestHandleReprocessEmbeddingProvider_NotFound verifies an unknown provider id
// returns a spec-conforming 404 and never enqueues a re-embed.
func TestHandleReprocessEmbeddingProvider_NotFound(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)
	mockContainer.embeddingProviderService.
		On("GetEmbeddingProvider", mock.Anything, "team-123", "missing").
		Return((*models.EmbeddingProviderResponse)(nil), services.ErrProviderNotFound)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest(
		"POST", "/api/v1/team-123/embedding-providers/missing/reprocess", nil, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)
	mockContainer.embeddingProviderService.AssertExpectations(t)
}

// TestHandleReprocessEmbeddingProvider_InFlightGuardSkipsDuplicate verifies the
// per-team guard: a second reprocess issued while the first run is still in
// flight does not fan out a second Backfill for the same team.
func TestHandleReprocessEmbeddingProvider_InFlightGuardSkipsDuplicate(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)
	mockContainer.embeddingProviderService.
		On("GetEmbeddingProvider", mock.Anything, "team-123", "provider-1").
		Return(providerForReprocess(), nil)

	started := make(chan struct{})
	release := make(chan struct{})
	// Exactly one Backfill: the guard must drop the concurrent second call.
	mockContainer.embeddingBackfillService.
		On("Backfill", mock.Anything, mock.Anything).
		Return(&services.EmbeddingBackfillResult{}, nil).
		Run(func(mock.Arguments) {
			close(started)
			<-release
		}).Once()

	srv := createTestEmbeddingProviderServer(mockContainer)
	post := func() *httptest.ResponseRecorder {
		req := makeAuthenticatedEmbeddingProviderRequest(
			"POST", "/api/v1/team-123/embedding-providers/provider-1/reprocess", nil, "user-123",
		)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		return w
	}

	// First call: its background Backfill starts and blocks (still in flight).
	assert.Equal(t, http.StatusAccepted, post().Code)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first reprocess did not start")
	}

	// Second call while the first is in flight: 202, but no second Backfill.
	assert.Equal(t, http.StatusAccepted, post().Code)

	close(release) // let the first run finish
	mockContainer.embeddingBackfillService.AssertExpectations(t)
}

// TestHandleGetEmbeddingCoverage_Success verifies the coverage handler returns the
// service payload and that the response conforms to the OpenAPI spec.
func TestHandleGetEmbeddingCoverage_Success(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	model := "text-embedding-3-small"
	expected := &models.EmbeddingCoverageResponse{
		HasActiveProvider: true,
		ActiveModel:       &model,
		Coverage: []models.EmbeddingCoverageItem{
			{EntityType: "prompt", Total: 8, Embedded: 6, Pending: 2, EmbeddedPercent: 75},
			{EntityType: "artifact", Total: 0, Embedded: 0, Pending: 0, EmbeddedPercent: 0},
			{EntityType: "memory", Total: 4, Embedded: 4, Pending: 0, EmbeddedPercent: 100},
			{EntityType: "blueprint", Total: 1, Embedded: 0, Pending: 1, EmbeddedPercent: 0},
			{EntityType: "feed_item", Total: 3, Embedded: 1, Pending: 2, EmbeddedPercent: 33},
		},
	}

	mockContainer.embeddingStatusService.On("GetCoverage", mock.Anything, "team-123").
		Return(expected, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest(
		"GET", "/api/v1/team-123/embedding-providers/coverage", nil, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response models.EmbeddingCoverageResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.True(t, response.HasActiveProvider)
	require.NotNil(t, response.ActiveModel)
	assert.Equal(t, "text-embedding-3-small", *response.ActiveModel)
	require.Len(t, response.Coverage, 5)
	assert.Equal(t, "prompt", response.Coverage[0].EntityType)
	assert.Equal(t, int64(2), response.Coverage[0].Pending)
	assert.Equal(t, 75, response.Coverage[0].EmbeddedPercent)

	mockContainer.embeddingStatusService.AssertExpectations(t)
}

// TestHandleGetEmbeddingCoverage_NoActiveProvider verifies the no-active-provider
// shape (all pending, null model) is returned as 200 and conforms to the spec.
func TestHandleGetEmbeddingCoverage_NoActiveProvider(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	expected := &models.EmbeddingCoverageResponse{
		HasActiveProvider: false,
		ActiveModel:       nil,
		Coverage: []models.EmbeddingCoverageItem{
			{EntityType: "prompt", Total: 5, Embedded: 0, Pending: 5, EmbeddedPercent: 0},
			{EntityType: "artifact", Total: 0, Embedded: 0, Pending: 0, EmbeddedPercent: 0},
			{EntityType: "memory", Total: 0, Embedded: 0, Pending: 0, EmbeddedPercent: 0},
			{EntityType: "blueprint", Total: 0, Embedded: 0, Pending: 0, EmbeddedPercent: 0},
			{EntityType: "feed_item", Total: 0, Embedded: 0, Pending: 0, EmbeddedPercent: 0},
		},
	}

	mockContainer.embeddingStatusService.On("GetCoverage", mock.Anything, "team-123").
		Return(expected, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest(
		"GET", "/api/v1/team-123/embedding-providers/coverage", nil, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	var response models.EmbeddingCoverageResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.False(t, response.HasActiveProvider)
	assert.Nil(t, response.ActiveModel)

	mockContainer.embeddingStatusService.AssertExpectations(t)
}

// TestHandleGetEmbeddingCoverageSettings_Success covers the settings-prefixed variant
// of the coverage endpoint so its distinct spec operation is conformance-validated.
func TestHandleGetEmbeddingCoverageSettings_Success(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	model := "text-embedding-3-small"
	expected := &models.EmbeddingCoverageResponse{
		HasActiveProvider: true,
		ActiveModel:       &model,
		Coverage: []models.EmbeddingCoverageItem{
			{EntityType: "prompt", Total: 2, Embedded: 1, Pending: 1, EmbeddedPercent: 50},
		},
	}

	mockContainer.embeddingStatusService.On("GetCoverage", mock.Anything, "team-123").
		Return(expected, nil)

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest(
		"GET", "/api/v1/team-123/settings/embedding-providers/coverage", nil, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	specconformance.AssertConformsToSpec(t, req, w)

	mockContainer.embeddingStatusService.AssertExpectations(t)
}

// TestHandleGetEmbeddingCoverage_ServiceError verifies a service failure maps to 500.
func TestHandleGetEmbeddingCoverage_ServiceError(t *testing.T) {
	mockContainer := newMockEmbeddingProviderContainer(t)

	mockContainer.embeddingStatusService.On("GetCoverage", mock.Anything, "team-123").
		Return((*models.EmbeddingCoverageResponse)(nil), errors.New("database error"))

	srv := createTestEmbeddingProviderServer(mockContainer)
	req := makeAuthenticatedEmbeddingProviderRequest(
		"GET", "/api/v1/team-123/embedding-providers/coverage", nil, "user-123",
	)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)

	mockContainer.embeddingStatusService.AssertExpectations(t)
}
