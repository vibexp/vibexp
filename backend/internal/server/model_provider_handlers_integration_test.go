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
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// MockModelProviderContainer implements the Container interface for model
// provider handler tests.
type MockModelProviderContainer struct {
	BaseMockContainer // Embed base container for default nil implementations
	mock.Mock
	modelProviderService *svcmocks.MockModelProviderServiceInterface
}

func (m *MockModelProviderContainer) ModelProviderService() services.ModelProviderServiceInterface {
	return m.modelProviderService
}

func newMockModelProviderContainer(t *testing.T) *MockModelProviderContainer {
	return &MockModelProviderContainer{
		modelProviderService: svcmocks.NewMockModelProviderServiceInterface(t),
	}
}

func createTestModelProviderServer(container *MockModelProviderContainer) *Server {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)

	r := chi.NewRouter()

	srv := &Server{
		port:      "8080",
		container: container,
		logger:    logger,
		config:    cfg,
		router:    r,
	}

	// Register routes via the production setup under both prefixes (bare and
	// settings) so tests exercise the same route tree the server mounts.
	r.Route("/api/v1/{team_id}/model-providers", srv.setupModelProvidersRoutes)
	r.Route("/api/v1/{team_id}/settings/model-providers", srv.setupModelProvidersRoutes)

	return srv
}

//nolint:unparam // userID parameter is used with same value in tests, but kept for consistency
func makeAuthenticatedModelProviderRequest(method, path string, body interface{}, userID string) *http.Request {
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

func sampleModelProvider() *models.ModelProvider {
	baseURL := "https://api.openai.com/v1"
	apiKeyEncrypted := "encrypted-key"
	return &models.ModelProvider{
		ID:              "provider-1",
		UserID:          "user-123",
		Name:            "OpenAI GPT-4o",
		ProviderType:    "openai_compatible",
		Model:           "gpt-4o-mini",
		IsDefault:       true,
		BaseURL:         &baseURL,
		APIKeyEncrypted: &apiKeyEncrypted,
		Configuration:   "{}",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Version:         1,
	}
}

func sampleModelProviderResponse() *models.ModelProviderResponse {
	return &models.ModelProviderResponse{
		ModelProvider: *sampleModelProvider(),
		HasAPIKey:     true,
	}
}

// --- Spec-conformance coverage: one conforming response per documented operation
// (both the bare and settings prefixes), so each maps to a covered payload. ---

// TestHandleCreateModelProvider_SpecConformance covers POST create on both prefixes.
func TestHandleCreateModelProvider_SpecConformance(t *testing.T) {
	for _, prefix := range []string{"model-providers", "settings/model-providers"} {
		t.Run(prefix, func(t *testing.T) {
			mockContainer := newMockModelProviderContainer(t)

			baseURL := "https://api.openai.com/v1"
			apiKey := "sk-test-key"
			isDefault := false
			reqBody := models.CreateModelProviderRequest{
				Name:         "New Provider",
				ProviderType: "openai_compatible",
				Model:        "gpt-4o-mini",
				BaseURL:      &baseURL,
				APIKey:       &apiKey,
				IsDefault:    &isDefault,
			}

			mockContainer.modelProviderService.
				On("CreateModelProvider", mock.Anything, "team-123", "user-123", reqBody).
				Return(sampleModelProvider(), nil)

			srv := createTestModelProviderServer(mockContainer)
			req := makeAuthenticatedModelProviderRequest(
				"POST", "/api/v1/team-123/"+prefix, reqBody, "user-123",
			)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			var response models.ModelProviderResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, "provider-1", response.ID)
			// The encrypted key must never leak; only has_api_key is exposed.
			assert.Nil(t, response.APIKeyEncrypted)
			assert.True(t, response.HasAPIKey)

			mockContainer.modelProviderService.AssertExpectations(t)
		})
	}
}

// TestHandleListModelProviders_SpecConformance covers GET list on both prefixes.
func TestHandleListModelProviders_SpecConformance(t *testing.T) {
	for _, prefix := range []string{"model-providers", "settings/model-providers"} {
		t.Run(prefix, func(t *testing.T) {
			mockContainer := newMockModelProviderContainer(t)

			mockContainer.modelProviderService.
				On("GetModelProvidersByTeamID", mock.Anything, "team-123").
				Return([]models.ModelProviderResponse{*sampleModelProviderResponse()}, nil)

			srv := createTestModelProviderServer(mockContainer)
			req := makeAuthenticatedModelProviderRequest(
				"GET", "/api/v1/team-123/"+prefix, nil, "user-123",
			)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			var response []models.ModelProviderResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			require.Len(t, response, 1)
			assert.Equal(t, "provider-1", response[0].ID)

			mockContainer.modelProviderService.AssertExpectations(t)
		})
	}
}

// TestHandleGetModelProvider_SpecConformance covers GET by id on both prefixes.
func TestHandleGetModelProvider_SpecConformance(t *testing.T) {
	for _, prefix := range []string{"model-providers", "settings/model-providers"} {
		t.Run(prefix, func(t *testing.T) {
			mockContainer := newMockModelProviderContainer(t)

			mockContainer.modelProviderService.
				On("GetModelProvider", mock.Anything, "team-123", "provider-1").
				Return(sampleModelProviderResponse(), nil)

			srv := createTestModelProviderServer(mockContainer)
			req := makeAuthenticatedModelProviderRequest(
				"GET", "/api/v1/team-123/"+prefix+"/provider-1", nil, "user-123",
			)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			mockContainer.modelProviderService.AssertExpectations(t)
		})
	}
}

// TestHandleUpdateModelProvider_SpecConformance covers PUT update on both prefixes.
func TestHandleUpdateModelProvider_SpecConformance(t *testing.T) {
	for _, prefix := range []string{"model-providers", "settings/model-providers"} {
		t.Run(prefix, func(t *testing.T) {
			mockContainer := newMockModelProviderContainer(t)

			newName := "Renamed Provider"
			reqBody := models.UpdateModelProviderRequest{Name: &newName}
			updated := sampleModelProvider()
			updated.Name = newName

			mockContainer.modelProviderService.
				On("UpdateModelProvider", mock.Anything, "team-123", mock.Anything, "provider-1", reqBody).
				Return(updated, nil)

			srv := createTestModelProviderServer(mockContainer)
			req := makeAuthenticatedModelProviderRequest(
				"PUT", "/api/v1/team-123/"+prefix+"/provider-1", reqBody, "user-123",
			)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			var response models.ModelProviderResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, newName, response.Name)
			assert.Nil(t, response.APIKeyEncrypted)

			mockContainer.modelProviderService.AssertExpectations(t)
		})
	}
}

// TestHandleDeleteModelProvider_SpecConformance covers DELETE on both prefixes.
func TestHandleDeleteModelProvider_SpecConformance(t *testing.T) {
	for _, prefix := range []string{"model-providers", "settings/model-providers"} {
		t.Run(prefix, func(t *testing.T) {
			mockContainer := newMockModelProviderContainer(t)

			mockContainer.modelProviderService.
				On("DeleteModelProvider", mock.Anything, "team-123", mock.Anything, "provider-1").
				Return(nil)

			srv := createTestModelProviderServer(mockContainer)
			req := makeAuthenticatedModelProviderRequest(
				"DELETE", "/api/v1/team-123/"+prefix+"/provider-1", nil, "user-123",
			)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			require.Equal(t, http.StatusNoContent, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			mockContainer.modelProviderService.AssertExpectations(t)
		})
	}
}

// TestHandleValidateModelProvider_SpecConformance covers POST validate on both prefixes.
func TestHandleValidateModelProvider_SpecConformance(t *testing.T) {
	for _, prefix := range []string{"model-providers", "settings/model-providers"} {
		t.Run(prefix, func(t *testing.T) {
			mockContainer := newMockModelProviderContainer(t)

			reqBody := models.ValidateModelProviderRequest{
				ProviderType: "openai_compatible",
				Model:        "gpt-4o-mini",
				BaseURL:      "https://api.openai.com/v1",
			}
			expected := &models.ValidateModelProviderResponse{
				IsValid: true,
				Message: "Model provider validation successful",
				Details: models.ValidateModelProviderDetails{
					ResponseTime: 120,
					StatusCode:   200,
				},
			}

			mockContainer.modelProviderService.
				On("ValidateModelProvider", mock.Anything, mock.Anything, mock.Anything, reqBody).
				Return(expected, nil)

			srv := createTestModelProviderServer(mockContainer)
			req := makeAuthenticatedModelProviderRequest(
				"POST", "/api/v1/team-123/"+prefix+"/validate", reqBody, "user-123",
			)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			specconformance.AssertConformsToSpec(t, req, w)

			var response models.ValidateModelProviderResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.True(t, response.IsValid)

			mockContainer.modelProviderService.AssertExpectations(t)
		})
	}
}

// --- Behavioral tests (error paths, guards) ---

// TestHandleListModelProviders_ServiceError maps a service failure to 500.
func TestHandleListModelProviders_ServiceError(t *testing.T) {
	mockContainer := newMockModelProviderContainer(t)
	mockContainer.modelProviderService.
		On("GetModelProvidersByTeamID", mock.Anything, "team-123").
		Return(([]models.ModelProviderResponse)(nil), errors.New("database error"))

	srv := createTestModelProviderServer(mockContainer)
	req := makeAuthenticatedModelProviderRequest("GET", "/api/v1/team-123/model-providers", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockContainer.modelProviderService.AssertExpectations(t)
}

// TestHandleGetModelProvider_NotFound maps the not-found sentinel to 404.
func TestHandleGetModelProvider_NotFound(t *testing.T) {
	mockContainer := newMockModelProviderContainer(t)
	mockContainer.modelProviderService.
		On("GetModelProvider", mock.Anything, "team-123", "missing").
		Return((*models.ModelProviderResponse)(nil), services.ErrModelProviderNotFound)

	srv := createTestModelProviderServer(mockContainer)
	req := makeAuthenticatedModelProviderRequest("GET", "/api/v1/team-123/model-providers/missing", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	mockContainer.modelProviderService.AssertExpectations(t)
}

// TestHandleCreateModelProvider_AlreadyExists maps the duplicate sentinel to 409.
func TestHandleCreateModelProvider_AlreadyExists(t *testing.T) {
	mockContainer := newMockModelProviderContainer(t)

	baseURL := "https://api.openai.com/v1"
	reqBody := models.CreateModelProviderRequest{
		Name:         "Dup Provider",
		ProviderType: "openai_compatible",
		Model:        "gpt-4o-mini",
		BaseURL:      &baseURL,
	}
	mockContainer.modelProviderService.
		On("CreateModelProvider", mock.Anything, "team-123", "user-123", reqBody).
		Return((*models.ModelProvider)(nil), services.ErrModelProviderAlreadyExists)

	srv := createTestModelProviderServer(mockContainer)
	req := makeAuthenticatedModelProviderRequest("POST", "/api/v1/team-123/model-providers", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	mockContainer.modelProviderService.AssertExpectations(t)
}

// TestHandleCreateModelProvider_ValidationError rejects requests missing required fields.
func TestHandleCreateModelProvider_ValidationError(t *testing.T) {
	tests := []struct {
		name    string
		reqBody models.CreateModelProviderRequest
	}{
		{"Missing name", models.CreateModelProviderRequest{ProviderType: "openai_compatible", Model: "m"}},
		{"Missing provider type", models.CreateModelProviderRequest{Name: "P", Model: "m"}},
		{"Missing model", models.CreateModelProviderRequest{Name: "P", ProviderType: "openai_compatible"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockModelProviderContainer(t)
			srv := createTestModelProviderServer(mockContainer)
			req := makeAuthenticatedModelProviderRequest(
				"POST", "/api/v1/team-123/model-providers", tt.reqBody, "user-123",
			)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestHandleUpdateModelProvider_NotFound maps the not-found sentinel to 404.
func TestHandleUpdateModelProvider_NotFound(t *testing.T) {
	mockContainer := newMockModelProviderContainer(t)

	newName := "Renamed"
	reqBody := models.UpdateModelProviderRequest{Name: &newName}
	mockContainer.modelProviderService.
		On("UpdateModelProvider", mock.Anything, "team-123", mock.Anything, "missing", reqBody).
		Return((*models.ModelProvider)(nil), services.ErrModelProviderNotFound)

	srv := createTestModelProviderServer(mockContainer)
	req := makeAuthenticatedModelProviderRequest("PUT", "/api/v1/team-123/model-providers/missing", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	mockContainer.modelProviderService.AssertExpectations(t)
}

// TestHandleDeleteModelProvider_LastProvider maps the last-delete guard to 400.
func TestHandleDeleteModelProvider_LastProvider(t *testing.T) {
	mockContainer := newMockModelProviderContainer(t)
	mockContainer.modelProviderService.
		On("DeleteModelProvider", mock.Anything, "team-123", mock.Anything, "provider-1").
		Return(services.ErrLastModelProviderDelete)

	srv := createTestModelProviderServer(mockContainer)
	req := makeAuthenticatedModelProviderRequest("DELETE", "/api/v1/team-123/model-providers/provider-1", nil, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	mockContainer.modelProviderService.AssertExpectations(t)
}

// TestHandleValidateModelProvider_Invalid returns a 200 with is_valid=false for a
// reachable-but-unauthorized provider.
func TestHandleValidateModelProvider_Invalid(t *testing.T) {
	mockContainer := newMockModelProviderContainer(t)

	apiKey := "bad-key"
	reqBody := models.ValidateModelProviderRequest{
		ProviderType: "openai_compatible",
		Model:        "gpt-4o-mini",
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       &apiKey,
	}
	expected := &models.ValidateModelProviderResponse{
		IsValid: false,
		Message: "Authentication failed - please check your API key",
		Details: models.ValidateModelProviderDetails{StatusCode: 401},
	}
	mockContainer.modelProviderService.
		On("ValidateModelProvider", mock.Anything, mock.Anything, mock.Anything, reqBody).
		Return(expected, nil)

	srv := createTestModelProviderServer(mockContainer)
	req := makeAuthenticatedModelProviderRequest("POST", "/api/v1/team-123/model-providers/validate", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response models.ValidateModelProviderResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.False(t, response.IsValid)
	mockContainer.modelProviderService.AssertExpectations(t)
}

// TestHandleValidateModelProvider_ValidationError rejects requests missing required fields.
func TestHandleValidateModelProvider_ValidationError(t *testing.T) {
	tests := []struct {
		name    string
		reqBody models.ValidateModelProviderRequest
	}{
		{"Missing provider type", models.ValidateModelProviderRequest{BaseURL: "https://api.openai.com/v1", Model: "m"}},
		{"Missing base URL", models.ValidateModelProviderRequest{ProviderType: "openai_compatible", Model: "m"}},
		{"Missing model", models.ValidateModelProviderRequest{ProviderType: "openai_compatible", BaseURL: "https://api.openai.com/v1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockContainer := newMockModelProviderContainer(t)
			srv := createTestModelProviderServer(mockContainer)
			req := makeAuthenticatedModelProviderRequest(
				"POST", "/api/v1/team-123/model-providers/validate", tt.reqBody, "user-123",
			)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestHandleValidateModelProvider_ServiceError maps an internal service failure to 500.
func TestHandleValidateModelProvider_ServiceError(t *testing.T) {
	mockContainer := newMockModelProviderContainer(t)

	reqBody := models.ValidateModelProviderRequest{
		ProviderType: "openai_compatible",
		Model:        "gpt-4o-mini",
		BaseURL:      "https://api.openai.com/v1",
	}
	mockContainer.modelProviderService.
		On("ValidateModelProvider", mock.Anything, mock.Anything, mock.Anything, reqBody).
		Return((*models.ValidateModelProviderResponse)(nil), errors.New("internal error"))

	srv := createTestModelProviderServer(mockContainer)
	req := makeAuthenticatedModelProviderRequest("POST", "/api/v1/team-123/model-providers/validate", reqBody, "user-123")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockContainer.modelProviderService.AssertExpectations(t)
}
