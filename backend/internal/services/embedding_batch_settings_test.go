package services

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// --- per-provider batch size and request timeout (#1665) ---

// distinctTexts returns n inputs of distinct lengths, so the batchingServer's
// vec[0] identifies which input produced each vector.
func distinctTexts(n int) []string {
	texts := make([]string, n)
	for i := range texts {
		texts[i] = strings.Repeat("z", i+1)
	}
	return texts
}

// A configured batch size splits the request into batches of exactly that size,
// sent sequentially, with the vectors reassembled in input order.
func TestOpenAICompatibleProvider_ConfiguredBatchSize(t *testing.T) {
	cases := []struct {
		name      string
		batchSize int
		inputs    int
		want      []int
	}{
		{name: "batch size 1 sends one chunk per request", batchSize: 1, inputs: 3, want: []int{1, 1, 1}},
		{name: "batch size 8 splits 20 chunks 8/8/4", batchSize: 8, inputs: 20, want: []int{8, 8, 4}},
		{name: "batch size 32 is today's default", batchSize: 32, inputs: 40, want: []int{32, 8}},
		{name: "zero means the default", batchSize: 0, inputs: 40, want: []int{32, 8}},
		{name: "negative means the default", batchSize: -3, inputs: 33, want: []int{32, 1}},
		{name: "above the cap is clamped to it", batchSize: 64, inputs: 40, want: []int{32, 8}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, batches := batchingServer(t, 2)
			defer server.Close()

			p, err := NewOpenAICompatibleProvider(
				server.URL, "k", "m", 2, 5*time.Second, tc.batchSize, loopbackProviderGuard(),
			)
			require.NoError(t, err)

			vectors, err := p.GenerateEmbeddings(context.Background(), distinctTexts(tc.inputs))
			require.NoError(t, err)
			require.Len(t, vectors, tc.inputs)
			for i := range vectors {
				assert.Equal(t, float32(i+1), vectors[i][0], "vector %d must correspond to input %d", i, i)
			}

			sizes := make([]int, 0, len(*batches))
			for _, b := range *batches {
				sizes = append(sizes, len(b))
			}
			assert.Equal(t, tc.want, sizes)
		})
	}
}

// stallingServer answers every request after the given delay, or as soon as the
// client gives up, so a timed-out test does not hold the server open.
func stallingServer(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		writeJSON(t, w, map[string]interface{}{
			"data": []map[string]interface{}{{"index": 0, "embedding": []float32{0.1, 0.2}}},
		})
	}))
}

// The request timeout bounds each request: a server slower than it fails the
// call, and the same server succeeds once the timeout is longer than its stall.
func TestOpenAICompatibleProvider_ConfiguredRequestTimeout(t *testing.T) {
	server := stallingServer(t, 300*time.Millisecond)
	defer server.Close()

	short, err := NewOpenAICompatibleProvider(
		server.URL, "k", "m", 2, 100*time.Millisecond, 0, loopbackProviderGuard(),
	)
	require.NoError(t, err)
	_, err = short.GenerateEmbeddings(context.Background(), []string{"x"})
	require.Error(t, err, "a request slower than the timeout must fail")
	assert.Contains(t, err.Error(), "Client.Timeout exceeded")

	long, err := NewOpenAICompatibleProvider(
		server.URL, "k", "m", 2, 2*time.Second, 0, loopbackProviderGuard(),
	)
	require.NoError(t, err)
	vectors, err := long.GenerateEmbeddings(context.Background(), []string{"x"})
	require.NoError(t, err, "the same request succeeds within a longer timeout")
	assert.Len(t, vectors, 1)
}

// NewGenerationProvider sizes the provider from the stored row, and a row
// without the values (zero) gets today's defaults.
func TestNewGenerationProvider_UsesRowBatchSizeAndTimeout(t *testing.T) {
	build := func(row *models.EmbeddingProvider) *OpenAICompatibleProvider {
		t.Helper()
		row.ProviderType = ProviderTypeOpenAICompatible
		row.BaseURL = strptr("http://localhost:1234/v1")
		p, err := NewGenerationProvider(row, "", "m", 2, loopbackProviderGuard())
		require.NoError(t, err)
		concrete, ok := p.(*OpenAICompatibleProvider)
		require.True(t, ok)
		return concrete
	}

	configured := build(&models.EmbeddingProvider{BatchSize: 8, RequestTimeoutSeconds: 5})
	assert.Equal(t, 8, configured.batchSize)
	assert.Equal(t, 5*time.Second, configured.httpClient.Timeout)

	unset := build(&models.EmbeddingProvider{})
	assert.Equal(t, maxEmbeddingBatchSize, unset.batchSize)
	assert.Equal(t, generateEmbeddingsTimeout, unset.httpClient.Timeout)
}

// The defaults are the values that were hardcoded before they became settings,
// so a provider created without them behaves exactly as before.
func TestEmbeddingProviderService_buildEmbeddingProvider_batchDefaults(t *testing.T) {
	service := createTestEmbeddingProviderService(mocks.NewMockEmbeddingProviderRepository(t))

	omitted := service.buildEmbeddingProvider("team-1", "user-1",
		models.CreateEmbeddingProviderRequest{Name: "P", ProviderType: "openai_compatible", Model: "m"},
		nil, "{}")
	assert.Equal(t, 32, omitted.BatchSize)
	assert.Equal(t, 30, omitted.RequestTimeoutSeconds)

	batch, timeout := 8, 120
	explicit := service.buildEmbeddingProvider("team-1", "user-1",
		models.CreateEmbeddingProviderRequest{
			Name: "P", ProviderType: "openai_compatible", Model: "m",
			BatchSize: &batch, RequestTimeoutSeconds: &timeout,
		}, nil, "{}")
	assert.Equal(t, 8, explicit.BatchSize)
	assert.Equal(t, 120, explicit.RequestTimeoutSeconds)
}

// CreateEmbeddingProvider persists the defaults when the request omits both.
func TestEmbeddingProviderService_CreateEmbeddingProvider_PersistsBatchDefaults(t *testing.T) {
	repo := mocks.NewMockEmbeddingProviderRepository(t)
	service := createTestEmbeddingProviderService(repo)

	var created *models.EmbeddingProvider
	repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*models.EmbeddingProvider")).
		RunAndReturn(func(_ context.Context, p *models.EmbeddingProvider) error {
			created = p
			return nil
		})

	req := createTestCreateEmbeddingProviderRequest()
	req.IsDefault = nil
	_, err := service.CreateEmbeddingProvider(context.Background(), testProviderTeamID, testProviderUserID, req)
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, 32, created.BatchSize)
	assert.Equal(t, 30, created.RequestTimeoutSeconds)
}

// An update applies a sent value and leaves an omitted one as stored.
func TestEmbeddingProviderService_UpdateEmbeddingProvider_BatchOverrides(t *testing.T) {
	cases := []struct {
		name        string
		req         models.UpdateEmbeddingProviderRequest
		wantBatch   int
		wantTimeout int
	}{
		{name: "omitted keeps the stored values", wantBatch: 16, wantTimeout: 60},
		{
			name:      "batch size only",
			req:       models.UpdateEmbeddingProviderRequest{BatchSize: ptr(4)},
			wantBatch: 4, wantTimeout: 60,
		},
		{
			name:      "both",
			req:       models.UpdateEmbeddingProviderRequest{BatchSize: ptr(8), RequestTimeoutSeconds: ptr(120)},
			wantBatch: 8, wantTimeout: 120,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewMockEmbeddingProviderRepository(t)
			service := createTestEmbeddingProviderService(repo)
			repo.EXPECT().GetByID(mock.Anything, "team-1", "provider-1").Return(&models.EmbeddingProvider{
				ID: "provider-1", Name: "p", ProviderType: "openai_compatible", Model: "m",
				Configuration: "{}", BatchSize: 16, RequestTimeoutSeconds: 60,
			}, nil)
			var captured *models.EmbeddingProvider
			repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*models.EmbeddingProvider")).
				RunAndReturn(func(_ context.Context, p *models.EmbeddingProvider) error {
					captured = p
					return nil
				})

			_, err := service.UpdateEmbeddingProvider(
				context.Background(), "team-1", testProviderUserID, "provider-1", tc.req,
			)
			require.NoError(t, err)
			require.NotNil(t, captured)
			assert.Equal(t, tc.wantBatch, captured.BatchSize)
			assert.Equal(t, tc.wantTimeout, captured.RequestTimeoutSeconds)
		})
	}
}

// ResolveActiveProvider builds the runtime provider with the stored row's batch
// size and timeout.
func TestEmbeddingProviderService_ResolveActiveProvider_PropagatesBatchSettings(t *testing.T) {
	repo := mocks.NewMockEmbeddingProviderRepository(t)
	service := createTestEmbeddingProviderService(repo)
	baseURL := "http://localhost:1234/v1"
	repo.EXPECT().GetActiveProvider(mock.Anything, testProviderTeamID).Return(&models.EmbeddingProvider{
		ProviderType: ProviderTypeOpenAICompatible, BaseURL: &baseURL, Model: "m",
		ChunkSize: 1000, ChunkOverlap: 200, Concurrency: 1, BatchSize: 8, RequestTimeoutSeconds: 120,
	}, nil)

	resolved, err := service.ResolveActiveProvider(context.Background(), testProviderTeamID)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	concrete, ok := resolved.Provider.(*OpenAICompatibleProvider)
	require.True(t, ok)
	assert.Equal(t, 8, concrete.batchSize)
	assert.Equal(t, 120*time.Second, concrete.httpClient.Timeout)
	assert.Equal(t, 8, resolved.BatchSize, "the dispatcher sizes the generate deadline from these")
	assert.Equal(t, 120*time.Second, resolved.RequestTimeout)
}

// The /validate probe runs with the submitted batch size and timeout, or the
// defaults when they are omitted.
func TestValidationProbeRow(t *testing.T) {
	req := models.ValidateEmbeddingProviderRequest{
		ProviderType: ProviderTypeOpenAICompatible, Model: "m", BaseURL: "http://localhost:1234/v1",
	}
	omitted := validationProbeRow(req)
	assert.Equal(t, 32, omitted.BatchSize)
	assert.Equal(t, 30, omitted.RequestTimeoutSeconds)
	require.NotNil(t, omitted.BaseURL)
	assert.Equal(t, req.BaseURL, *omitted.BaseURL)

	req.BatchSize, req.RequestTimeoutSeconds = ptr(4), ptr(5)
	submitted := validationProbeRow(req)
	assert.Equal(t, 4, submitted.BatchSize)
	assert.Equal(t, 5, submitted.RequestTimeoutSeconds)

	p, err := NewGenerationProvider(submitted, "", req.Model, 2, loopbackProviderGuard())
	require.NoError(t, err)
	concrete, ok := p.(*OpenAICompatibleProvider)
	require.True(t, ok)
	assert.Equal(t, 5*time.Second, concrete.httpClient.Timeout, "the probe's client uses the submitted timeout")
}

// deadlineProvider records how long the context it was called with had left, so
// a test can see the generate-stage deadline the dispatcher chose.
type deadlineProvider struct {
	countingProvider
	remaining time.Duration
}

func (p *deadlineProvider) GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if deadline, ok := ctx.Deadline(); ok {
		p.remaining = time.Until(deadline)
	}
	return p.countingProvider.GenerateEmbeddings(ctx, texts)
}

// chunkedInput is an entity whose body chunks into exactly n chunks at chunk
// size 100 with no overlap.
func chunkedInput(n int) embeddingInput {
	return embeddingInput{
		entityType: "memory", entityID: "m-1", userID: "u-1",
		body: strings.Repeat("a", 100*n),
	}
}

func resolvedWith(batchSize int, timeout time.Duration) *ResolvedEmbeddingProvider {
	return &ResolvedEmbeddingProvider{
		ChunkSize: 100, ChunkOverlap: 0, Concurrency: 1, BatchSize: batchSize, RequestTimeout: timeout,
	}
}

// The generate deadline fits one full attempt at the provider's pace, never
// drops below the fixed job timeout, and is capped.
func TestGenerateTimeout(t *testing.T) {
	require.Len(t, NewTextChunker(100, 0).Chunk(chunkedInput(20).embedText()), 20, "fixture: 20 chunks")

	cases := []struct {
		name     string
		input    embeddingInput
		resolved *ResolvedEmbeddingProvider
		want     time.Duration
	}{
		{"no provider", chunkedInput(1), nil, embeddingJobTimeout},
		{"unset settings", chunkedInput(20), resolvedWith(0, 0), embeddingJobTimeout},
		{"a small entity at the defaults", chunkedInput(20), resolvedWith(32, 30*time.Second), embeddingJobTimeout},
		{"20 chunks, batch 8, 60 s each: 3 requests", chunkedInput(20), resolvedWith(8, time.Minute), 3 * time.Minute},
		{"20 chunks, batch 1, 300 s each: capped", chunkedInput(20), resolvedWith(1, 300*time.Second),
			maxEmbeddingGenerateTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, generateTimeout(tc.input, tc.resolved))
		})
	}
	assert.Less(t, maxEmbeddingGenerateTimeout, defaultEmbeddingQueueLease,
		"the queue lease must outlast the longest generate deadline")
}

// generate runs under that deadline, so a slow provider's multi-batch entity is
// not cut off at the fixed 2 minutes.
func TestEmbeddingDispatcher_GenerateUsesProviderPacedDeadline(t *testing.T) {
	provider := &deadlineProvider{}
	resolved := resolvedWith(8, time.Minute)
	resolved.Provider = provider
	svc := newRecordingEmbeddingService(func(string) string { return "team-1" })
	d := newDispatcher(&perTeamResolver{}, svc, slog.New(slog.NewTextHandler(io.Discard, nil)),
		EmbeddingRetryConfig{MaxRetries: 1})

	outcome := d.generate(chunkedInput(20), "team-1", resolved)
	require.NoError(t, outcome.err)
	assert.Greater(t, provider.remaining, embeddingJobTimeout,
		"20 chunks at batch 8 and 60 s per request need longer than the fixed job timeout")
	assert.LessOrEqual(t, provider.remaining, 3*time.Minute)
}
