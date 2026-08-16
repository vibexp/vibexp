package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

	p, err := NewOpenAICompatibleProvider(server.URL+"/v1", "secret-key", "test-model", 2, time.Second, loopbackProviderGuard())
	require.NoError(t, err)

	vectors, err := p.GenerateEmbeddings(context.Background(), []string{"alpha", "beta"})
	require.NoError(t, err)

	assert.Equal(t, "Bearer secret-key", gotAuth)
	assert.Equal(t, "/v1/embeddings", gotPath)
	assert.Equal(t, []string{"alpha", "beta"}, gotReq.Input)
	assert.Equal(t, "test-model", gotReq.Model)
	assert.Equal(t, 2, gotReq.Dimensions, "provider must request the configured dimension width")
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

	p, err := NewOpenAICompatibleProvider(server.URL, "", "m", 2, time.Second, loopbackProviderGuard())
	require.NoError(t, err)
	_, err = p.GenerateEmbeddings(context.Background(), []string{"x"})
	require.NoError(t, err)
	assert.False(t, hadAuth, "no Authorization header when API key is empty")
}

func TestOpenAICompatibleProvider_EmptyInputNoCall(t *testing.T) {
	p, err := NewOpenAICompatibleProvider("http://example.invalid", "k", "m", 2, time.Second, loopbackProviderGuard())
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

	p, err := NewOpenAICompatibleProvider(server.URL, "k", "m", 2, time.Second, loopbackProviderGuard())
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

	p, err := NewOpenAICompatibleProvider(server.URL, "k", "m", 2, time.Second, loopbackProviderGuard())
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

	p, err := NewOpenAICompatibleProvider(server.URL, "k", "m", 2, time.Second, loopbackProviderGuard())
	require.NoError(t, err)
	_, err = p.GenerateEmbeddings(context.Background(), []string{"a", "b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 2")
}

func TestNewOpenAICompatibleProvider_Validation(t *testing.T) {
	_, err := NewOpenAICompatibleProvider("", "k", "m", 2, time.Second, loopbackProviderGuard())
	assert.ErrorContains(t, err, "base_url is required")
	_, err = NewOpenAICompatibleProvider("http://x", "k", "", 2, time.Second, loopbackProviderGuard())
	assert.ErrorContains(t, err, "model is required")
	_, err = NewOpenAICompatibleProvider("http://x", "k", "m", 0, time.Second, loopbackProviderGuard())
	assert.ErrorContains(t, err, "dimensions must be")
}

func TestNewGenerationProvider_Factory(t *testing.T) {
	// openai_compatible builds an OpenAICompatibleProvider.
	p, err := NewGenerationProvider(&models.EmbeddingProvider{
		ProviderType: ProviderTypeOpenAICompatible,
		BaseURL:      strptr("http://localhost:1234/v1"),
	}, "key", "model", 1024, time.Second, loopbackProviderGuard())
	require.NoError(t, err)
	assert.Equal(t, ProviderTypeOpenAICompatible, p.Type())

	// Unknown provider type is rejected.
	_, err = NewGenerationProvider(&models.EmbeddingProvider{ProviderType: "cohere"}, "", "m", 2, time.Second, loopbackProviderGuard())
	assert.ErrorContains(t, err, "unsupported embedding provider type")

	// Missing base_url is rejected.
	_, err = NewGenerationProvider(
		&models.EmbeddingProvider{ProviderType: ProviderTypeOpenAICompatible}, "", "m", 2, time.Second,
		loopbackProviderGuard(),
	)
	assert.ErrorContains(t, err, "base_url is required")

	// Nil provider is rejected.
	_, err = NewGenerationProvider(nil, "", "m", 2, time.Second, loopbackProviderGuard())
	assert.ErrorContains(t, err, "nil")
}

// --- batching, error bodies, and permanence classification (#756) ---

// batchingServer records the Input of every request it serves and answers each
// with correctly-indexed unit vectors, so a test can assert both the split and the
// reassembled order.
func batchingServer(t *testing.T, dims int) (*httptest.Server, *[][]string) {
	t.Helper()
	var batches [][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIEmbeddingsRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		batches = append(batches, req.Input)

		data := make([]map[string]interface{}, 0, len(req.Input))
		for i, text := range req.Input {
			// Encode the input text's identity into the vector so the assembled
			// output can be checked against input ORDER, not just length.
			vec := make([]float32, dims)
			vec[0] = float32(len(text))
			data = append(data, map[string]interface{}{"index": i, "embedding": vec})
		}
		writeJSON(t, w, map[string]interface{}{"data": data})
	}))
	return server, &batches
}

// An entity longer than one batch must still embed: the request is split, and the
// vectors come back in input order. This is the ~73-chunk memory from the report.
func TestOpenAICompatibleProvider_SplitsIntoBatches(t *testing.T) {
	server, batches := batchingServer(t, 2)
	defer server.Close()

	p, err := NewOpenAICompatibleProvider(server.URL, "k", "m", 2, 5*time.Second, loopbackProviderGuard())
	require.NoError(t, err)

	const n = 73
	texts := make([]string, n)
	for i := range texts {
		// Distinct lengths so vec[0] identifies which input produced each vector.
		texts[i] = strings.Repeat("x", i+1)
	}

	vectors, err := p.GenerateEmbeddings(context.Background(), texts)
	require.NoError(t, err)
	require.Len(t, vectors, n, "one vector per input across all batches")

	for i := range vectors {
		assert.Equal(t, float32(i+1), vectors[i][0],
			"vector %d must correspond to input %d — order preserved across batches", i, i)
	}

	require.Len(t, *batches, 3, "73 inputs at batch size 32 is 32+32+9")
	for i, b := range *batches {
		assert.LessOrEqual(t, len(b), maxEmbeddingBatchSize,
			"batch %d must not exceed the provider's client batch limit", i)
	}
	assert.Len(t, (*batches)[2], 9, "the trailing partial batch carries the remainder")
}

// Exactly one batch worth of input must still be a single request — batching must
// not add a round trip at the boundary.
func TestOpenAICompatibleProvider_ExactBatchSizeIsOneRequest(t *testing.T) {
	server, batches := batchingServer(t, 2)
	defer server.Close()

	p, err := NewOpenAICompatibleProvider(server.URL, "k", "m", 2, 5*time.Second, loopbackProviderGuard())
	require.NoError(t, err)

	texts := make([]string, maxEmbeddingBatchSize)
	for i := range texts {
		texts[i] = strings.Repeat("y", i+1)
	}

	vectors, err := p.GenerateEmbeddings(context.Background(), texts)
	require.NoError(t, err)
	assert.Len(t, vectors, maxEmbeddingBatchSize)
	assert.Len(t, *batches, 1, "a full-but-not-over batch is one request")
}

// A provider rejection must carry its own explanation: the status alone is what
// made the reported 422 undiagnosable.
func TestOpenAICompatibleProvider_ErrorCarriesTruncatedBody(t *testing.T) {
	const reason = `{"error":"batch size 73 > maximum allowed batch size 32"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, writeErr := w.Write([]byte(reason))
		require.NoError(t, writeErr)
	}))
	defer server.Close()

	p, err := NewOpenAICompatibleProvider(server.URL, "k", "m", 2, time.Second, loopbackProviderGuard())
	require.NoError(t, err)

	_, err = p.GenerateEmbeddings(context.Background(), []string{"x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 422", "the existing message prefix is kept for operator greps")
	assert.Contains(t, err.Error(), "maximum allowed batch size",
		"the provider's own reason must reach the error")

	var providerErr *providerHTTPError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, http.StatusUnprocessableEntity, providerErr.StatusCode)
	assert.True(t, providerErr.Permanent(), "422 is a rejection of the request itself")
}

// A huge error body (an HTML error page from a proxy) must not flood the log.
func TestOpenAICompatibleProvider_ErrorBodyIsCapped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, writeErr := w.Write([]byte(strings.Repeat("A", 100_000)))
		require.NoError(t, writeErr)
	}))
	defer server.Close()

	p, err := NewOpenAICompatibleProvider(server.URL, "k", "m", 2, time.Second, loopbackProviderGuard())
	require.NoError(t, err)

	_, err = p.GenerateEmbeddings(context.Background(), []string{"x"})
	require.Error(t, err)

	var providerErr *providerHTTPError
	require.ErrorAs(t, err, &providerErr)
	assert.Len(t, providerErr.Body, maxProviderErrorBodyBytes, "body is truncated to the cap")
	assert.False(t, providerErr.Permanent(), "502 is a server-side fault and stays retryable")
}

// The classification table. 408 and 429 are the two 4xx that describe a transient
// condition; everything else in 4xx is the provider refusing this exact request.
func TestProviderHTTPError_Permanent(t *testing.T) {
	cases := []struct {
		status    int
		permanent bool
	}{
		{http.StatusBadRequest, true},
		{http.StatusUnauthorized, true},
		{http.StatusForbidden, true},
		{http.StatusNotFound, true},
		{http.StatusUnprocessableEntity, true},
		{http.StatusRequestTimeout, false},
		{http.StatusTooManyRequests, false},
		{http.StatusInternalServerError, false},
		{http.StatusServiceUnavailable, false},
	}
	for _, tc := range cases {
		err := &providerHTTPError{StatusCode: tc.status}
		assert.Equal(t, tc.permanent, err.Permanent(), "status %d", tc.status)
	}
}

// With no body the message must be byte-identical to what it was before #756, so
// existing operator greps and alerts keep matching.
func TestProviderHTTPError_MessageWithoutBody(t *testing.T) {
	err := &providerHTTPError{StatusCode: 422}
	assert.Equal(t, "embeddings endpoint returned status 422", err.Error())
}
