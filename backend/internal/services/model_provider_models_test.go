package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// Tests for the model listing op (#1070).

// modelListServer serves GET /models with the given body and status, recording
// the Authorization header it was called with.
func modelListServer(t *testing.T, status int, body string, gotAuth *string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotAuth != nil {
			*gotAuth = r.Header.Get("Authorization")
		}
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, err := w.Write([]byte(body))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	return server
}

func listRequest(baseURL string) models.ListProviderModelsRequest {
	return models.ListProviderModelsRequest{
		ProviderType: ProviderTypeOpenAICompatible,
		BaseURL:      baseURL,
		APIKey:       stringPtr("sk-inline"),
	}
}

func TestListProviderModels_ReturnsSortedUnfilteredList(t *testing.T) {
	var gotAuth string
	server := modelListServer(t, http.StatusOK, `{"object":"list","data":[
		{"id":"text-embedding-3-small","owned_by":"openai"},
		{"id":"gpt-4o","owned_by":"openai"},
		{"id":""},
		{"id":"llama3"}
	]}`, &gotAuth)

	resp, err := createTestModelProviderService(nil).ListProviderModels(
		context.Background(), testProviderTeamID, testProviderUserID, listRequest(server.URL),
	)

	require.NoError(t, err)
	assert.True(t, resp.Supported)
	assert.Empty(t, resp.Message)
	assert.Equal(t, []models.ProviderModel{
		{ID: "gpt-4o", OwnedBy: "openai"},
		{ID: "llama3"},
		{ID: "text-embedding-3-small", OwnedBy: "openai"},
	}, []models.ProviderModel(resp.Models), "sorted by id, embedding model kept, empty id dropped")
	assert.Equal(t, "Bearer sk-inline", gotAuth)
}

func TestListProviderModels_Unsupported(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"endpoint 404s", http.StatusNotFound, `{"error":"not found"}`},
		{"body is not JSON", http.StatusOK, `<html>gateway</html>`},
		{"JSON without a data list", http.StatusOK, `{"object":"list"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := modelListServer(t, tt.status, tt.body, nil)

			resp, err := createTestModelProviderService(nil).ListProviderModels(
				context.Background(), testProviderTeamID, testProviderUserID, listRequest(server.URL),
			)

			require.NoError(t, err)
			assert.False(t, resp.Supported)
			assert.Empty(t, resp.Models)
			assert.Empty(t, resp.Message, "not implementing /models is not a failure")
		})
	}
}

func TestListProviderModels_Unauthorized(t *testing.T) {
	server := modelListServer(t, http.StatusUnauthorized, `{"error":"invalid key sk-inline at `+"internal"+`"}`, nil)

	resp, err := createTestModelProviderService(nil).ListProviderModels(
		context.Background(), testProviderTeamID, testProviderUserID, listRequest(server.URL),
	)

	require.NoError(t, err)
	assert.False(t, resp.Supported)
	assert.Empty(t, resp.Models)
	assert.Equal(t, providerErrUnauthorized, resp.Message, "a fixed category, never the provider body")
}

func TestListProviderModels_ServerErrorIsConnectionFailed(t *testing.T) {
	server := modelListServer(t, http.StatusBadGateway, `upstream down`, nil)

	resp, err := createTestModelProviderService(nil).ListProviderModels(
		context.Background(), testProviderTeamID, testProviderUserID, listRequest(server.URL),
	)

	require.NoError(t, err)
	assert.False(t, resp.Supported)
	assert.Equal(t, providerErrConnectionFailed, resp.Message)
}

func TestListProviderModels_ConnectionRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := server.URL
	server.Close()

	resp, err := createTestModelProviderService(nil).ListProviderModels(
		context.Background(), testProviderTeamID, testProviderUserID, listRequest(closedURL),
	)

	require.NoError(t, err)
	assert.False(t, resp.Supported)
	assert.Empty(t, resp.Models)
	assert.Equal(t, providerErrConnectionFailed, resp.Message)
	assert.NotContains(t, resp.Message, strings.TrimPrefix(closedURL, "http://"))
}

func TestListProviderModels_ReusesStoredKeyWhenAPIKeyBlank(t *testing.T) {
	for _, apiKey := range []*string{nil, stringPtr("")} {
		mockRepo := mocks.NewMockModelProviderRepository(t)
		svc := createTestModelProviderService(mockRepo)

		encrypted, err := svc.encrypt("sk-stored")
		require.NoError(t, err)
		mockRepo.On("GetByID", mock.Anything, testProviderTeamID, "provider-1").
			Return(&models.ModelProvider{ID: "provider-1", APIKeyEncrypted: &encrypted}, nil)

		var gotAuth string
		server := modelListServer(t, http.StatusOK, `{"data":[{"id":"m"}]}`, &gotAuth)

		req := listRequest(server.URL)
		req.APIKey = apiKey
		req.ProviderID = "provider-1"
		resp, err := svc.ListProviderModels(context.Background(), testProviderTeamID, testProviderUserID, req)

		require.NoError(t, err)
		assert.True(t, resp.Supported)
		assert.Equal(t, "Bearer sk-stored", gotAuth)
	}
}

func TestListProviderModels_InlineKeyWinsOverProviderID(t *testing.T) {
	// No repository expectation: a supplied api_key must not touch the store.
	svc := createTestModelProviderService(mocks.NewMockModelProviderRepository(t))
	var gotAuth string
	server := modelListServer(t, http.StatusOK, `{"data":[]}`, &gotAuth)

	req := listRequest(server.URL)
	req.ProviderID = "provider-1"
	resp, err := svc.ListProviderModels(context.Background(), testProviderTeamID, testProviderUserID, req)

	require.NoError(t, err)
	assert.True(t, resp.Supported)
	assert.NotNil(t, resp.Models)
	assert.Equal(t, "Bearer sk-inline", gotAuth)
}

func TestListProviderModels_UnknownProviderID(t *testing.T) {
	mockRepo := mocks.NewMockModelProviderRepository(t)
	mockRepo.On("GetByID", mock.Anything, testProviderTeamID, "missing").
		Return((*models.ModelProvider)(nil), repositories.ErrModelProviderNotFound)

	req := listRequest("https://api.openai.com/v1")
	req.APIKey = nil
	req.ProviderID = "missing"
	_, err := createTestModelProviderService(mockRepo).ListProviderModels(
		context.Background(), testProviderTeamID, testProviderUserID, req,
	)

	require.ErrorIs(t, err, ErrModelProviderNotFound)
}

func TestListProviderModels_UnsupportedProviderType(t *testing.T) {
	req := listRequest("https://api.openai.com/v1")
	req.ProviderType = "anthropic"

	resp, err := createTestModelProviderService(nil).ListProviderModels(
		context.Background(), testProviderTeamID, testProviderUserID, req,
	)

	require.NoError(t, err)
	assert.False(t, resp.Supported)
	assert.Equal(t, providerErrMisconfigured, resp.Message)
}

func TestListProviderModels_RejectsInternalDestinations(t *testing.T) {
	for _, tt := range blockedDestinations {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := prodModelService(t).ListProviderModels(
				context.Background(), testProviderTeamID, testProviderUserID, listRequest(tt.baseURL),
			)

			require.NoError(t, err)
			assert.False(t, resp.Supported)
			assert.Empty(t, resp.Models)
			assert.Equal(t, providerErrDestinationNotAllowed, resp.Message)
		})
	}
}

func TestListProviderModels_DeniedForMember(t *testing.T) {
	_, err := deniedModelService(t).ListProviderModels(
		context.Background(), testProviderTeamID, testProviderUserID, listRequest("https://api.openai.com/v1"),
	)
	assert.True(t, errors.Is(err, ErrPermissionDenied), "got: %v", err)
}

func TestNewOpenAICompatibleModelLister_NeedsNoModel(t *testing.T) {
	lister, err := NewOpenAICompatibleModelLister("https://api.openai.com/v1/", "", 0, loopbackProviderGuard())
	require.NoError(t, err)
	assert.Empty(t, lister.Model())
	assert.Equal(t, "https://api.openai.com/v1", lister.baseURL)

	_, err = NewOpenAICompatibleModelLister(" ", "", 0, loopbackProviderGuard())
	require.Error(t, err)
	_, err = NewOpenAICompatibleModelLister("ftp://example.com", "", 0, loopbackProviderGuard())
	require.Error(t, err)

	// The completion-capable constructor keeps requiring a model.
	_, err = NewOpenAICompatibleModelProvider("https://api.openai.com/v1", "", "", 0, loopbackProviderGuard())
	require.Error(t, err)
}
