package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

func strptr(s string) *string { return &s }

func writeJSON(t *testing.T, w http.ResponseWriter, body map[string]interface{}) {
	t.Helper()
	require.NoError(t, json.NewEncoder(w).Encode(body))
}

func TestOpenAICompatibleProvider_HappyPathAndAuthHeader(t *testing.T) {
	var gotAuth, gotPath string
	var gotReq openAIEmbeddingsRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotReq))
		// Return vectors out of order to exercise index-based reordering.
		writeJSON(t, w, map[string]interface{}{
			"data": []map[string]interface{}{
				{"index": 1, "embedding": []float32{0.3, 0.4}},
				{"index": 0, "embedding": []float32{0.1, 0.2}},
			},
		})
	}))
	defer server.Close()

	p, err := NewOpenAICompatibleProvider(server.URL+"/v1", "secret-key", "test-model", 2, time.Second)
	require.NoError(t, err)

	vectors, err := p.GenerateEmbeddings(context.Background(), []string{"alpha", "beta"})
	require.NoError(t, err)

	assert.Equal(t, "Bearer secret-key", gotAuth)
	assert.Equal(t, "/v1/embeddings", gotPath)
	assert.Equal(t, []string{"alpha", "beta"}, gotReq.Input)
	assert.Equal(t, "test-model", gotReq.Model)
	assert.Equal(t, [][]float32{{0.1, 0.2}, {0.3, 0.4}}, vectors)
	assert.Equal(t, "test-model", p.Model())
	assert.Equal(t, 2, p.Dimensions())
	assert.Equal(t, ProviderTypeOpenAICompatible, p.Type())
}

func TestOpenAICompatibleProvider_NoAuthHeaderWhenKeyEmpty(t *testing.T) {
	var hadAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		writeJSON(t, w, map[string]interface{}{
			"data": []map[string]interface{}{{"index": 0, "embedding": []float32{1, 2}}},
		})
	}))
	defer server.Close()

	p, err := NewOpenAICompatibleProvider(server.URL, "", "m", 2, time.Second)
	require.NoError(t, err)
	_, err = p.GenerateEmbeddings(context.Background(), []string{"x"})
	require.NoError(t, err)
	assert.False(t, hadAuth, "no Authorization header when API key is empty")
}

func TestOpenAICompatibleProvider_EmptyInputNoCall(t *testing.T) {
	p, err := NewOpenAICompatibleProvider("http://example.invalid", "k", "m", 2, time.Second)
	require.NoError(t, err)
	vectors, err := p.GenerateEmbeddings(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, vectors)
}

func TestOpenAICompatibleProvider_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	p, err := NewOpenAICompatibleProvider(server.URL, "k", "m", 2, time.Second)
	require.NoError(t, err)
	_, err = p.GenerateEmbeddings(context.Background(), []string{"x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 401")
}

func TestOpenAICompatibleProvider_DimensionMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"data": []map[string]interface{}{{"index": 0, "embedding": []float32{1, 2, 3}}},
		})
	}))
	defer server.Close()

	p, err := NewOpenAICompatibleProvider(server.URL, "k", "m", 2, time.Second)
	require.NoError(t, err)
	_, err = p.GenerateEmbeddings(context.Background(), []string{"x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 2")
}

func TestOpenAICompatibleProvider_CountMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"data": []map[string]interface{}{{"index": 0, "embedding": []float32{1, 2}}},
		})
	}))
	defer server.Close()

	p, err := NewOpenAICompatibleProvider(server.URL, "k", "m", 2, time.Second)
	require.NoError(t, err)
	_, err = p.GenerateEmbeddings(context.Background(), []string{"a", "b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 2")
}

func TestNewOpenAICompatibleProvider_Validation(t *testing.T) {
	_, err := NewOpenAICompatibleProvider("", "k", "m", 2, time.Second)
	assert.ErrorContains(t, err, "base_url is required")
	_, err = NewOpenAICompatibleProvider("http://x", "k", "", 2, time.Second)
	assert.ErrorContains(t, err, "model is required")
	_, err = NewOpenAICompatibleProvider("http://x", "k", "m", 0, time.Second)
	assert.ErrorContains(t, err, "dimensions must be")
}

func TestNewGenerationProvider_Factory(t *testing.T) {
	// openai_compatible builds an OpenAICompatibleProvider.
	p, err := NewGenerationProvider(&models.EmbeddingProvider{
		ProviderType: ProviderTypeOpenAICompatible,
		BaseURL:      strptr("http://localhost:1234/v1"),
	}, "key", "model", 768, time.Second)
	require.NoError(t, err)
	assert.Equal(t, ProviderTypeOpenAICompatible, p.Type())

	// Unknown provider type is rejected.
	_, err = NewGenerationProvider(&models.EmbeddingProvider{ProviderType: "cohere"}, "", "m", 2, time.Second)
	assert.ErrorContains(t, err, "unsupported embedding provider type")

	// Missing base_url is rejected.
	_, err = NewGenerationProvider(
		&models.EmbeddingProvider{ProviderType: ProviderTypeOpenAICompatible}, "", "m", 2, time.Second,
	)
	assert.ErrorContains(t, err, "base_url is required")

	// Nil provider is rejected.
	_, err = NewGenerationProvider(nil, "", "m", 2, time.Second)
	assert.ErrorContains(t, err, "nil")
}
