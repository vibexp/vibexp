package services_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/services"
)

// testEmbedderModel and testEmbedderDimensions configure the embedder under test;
// 768 matches the gemini-embedding-001 default.
const (
	testEmbedderModel      = "gemini-embedding-001"
	testEmbedderDimensions = 768
)

func newTestEmbedder(url string) *services.AIQueryEmbedder {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)
	return services.NewAIQueryEmbedder(services.AIQueryEmbedderConfig{
		AIServiceURL: url,
		Model:        testEmbedderModel,
		Dimensions:   testEmbedderDimensions,
		Logger:       logger,
	})
}

func embeddingHandler(t *testing.T, vector []float32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/embeddings", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			"data": []map[string]interface{}{
				{"object": "embedding", "embedding": vector, "index": 0},
			},
			"model": testEmbedderModel,
		})
		require.NoError(t, err)
	}
}

func TestAIQueryEmbedder_EmbedQuery_Success(t *testing.T) {
	want := make([]float32, testEmbedderDimensions)
	want[0] = 0.5

	srv := httptest.NewServer(embeddingHandler(t, want))
	defer srv.Close()

	got, err := newTestEmbedder(srv.URL).EmbedQuery(context.Background(), "hello")

	require.NoError(t, err)
	require.Len(t, got, testEmbedderDimensions)
	assert.InDelta(t, 0.5, got[0], 0.0001)
}

// TestAIQueryEmbedder_EmbedQuery_SendsQueryTaskType asserts the request carries the
// configured model and the RETRIEVAL_QUERY task type so Gemini applies its
// asymmetric query↔document objective.
func TestAIQueryEmbedder_EmbedQuery_SendsQueryTaskType(t *testing.T) {
	var captured struct {
		Model    string `json:"model"`
		TaskType string `json:"task_type"`
		Input    string `json:"input"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			"data": []map[string]interface{}{
				{"object": "embedding", "embedding": make([]float32, testEmbedderDimensions), "index": 0},
			},
		}))
	}))
	defer srv.Close()

	_, err := newTestEmbedder(srv.URL).EmbedQuery(context.Background(), "find me")

	require.NoError(t, err)
	assert.Equal(t, testEmbedderModel, captured.Model)
	assert.Equal(t, "RETRIEVAL_QUERY", captured.TaskType)
	assert.Equal(t, "find me", captured.Input)
}

func TestAIQueryEmbedder_EmbedQuery_WrongDimensions(t *testing.T) {
	srv := httptest.NewServer(embeddingHandler(t, make([]float32, 10)))
	defer srv.Close()

	_, err := newTestEmbedder(srv.URL).EmbedQuery(context.Background(), "hello")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 768")
}

func TestAIQueryEmbedder_EmbedQuery_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := newTestEmbedder(srv.URL).EmbedQuery(context.Background(), "hello")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestAIQueryEmbedder_EmbedQuery_NoData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, writeErr := w.Write([]byte(`{"object":"list","data":[]}`))
		require.NoError(t, writeErr)
	}))
	defer srv.Close()

	_, err := newTestEmbedder(srv.URL).EmbedQuery(context.Background(), "hello")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no embedding data")
}

func TestAIQueryEmbedder_EmbedQuery_MissingURL(t *testing.T) {
	_, err := newTestEmbedder("").EmbedQuery(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}
