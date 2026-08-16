package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/pkg/events"
)

// --- dispatcher test doubles ---

// countingProvider records the peak number of concurrent GenerateEmbeddings calls
// so a test can assert the per-provider concurrency bound is honored.
type countingProvider struct {
	tracker inFlightTracker
	calls   atomic.Int32
	delay   time.Duration
	err     error
}

func (p *countingProvider) GenerateEmbeddings(_ context.Context, texts []string) ([][]float32, error) {
	p.tracker.enter()
	defer p.tracker.leave()
	p.calls.Add(1)
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	if p.err != nil {
		return nil, p.err
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{1, 2, 3}
	}
	return out, nil
}
func (p *countingProvider) Model() string   { return "counting-model" }
func (p *countingProvider) Dimensions() int { return 3 }
func (p *countingProvider) Type() string    { return ProviderTypeOpenAICompatible }

// perTeamResolver resolves a per-team provider + concurrency, so a test can drive
// several providers with independent limits through one dispatcher.
type perTeamResolver struct {
	providers   map[string]EmbeddingProvider
	concurrency map[string]int
}

func (r *perTeamResolver) ResolveActiveProvider(
	_ context.Context, teamID string,
) (*ResolvedEmbeddingProvider, error) {
	prov, ok := r.providers[teamID]
	if !ok {
		return nil, nil
	}
	return &ResolvedEmbeddingProvider{
		Provider:     prov,
		ChunkSize:    1000,
		ChunkOverlap: 200,
		Concurrency:  r.concurrency[teamID],
	}, nil
}

// recordingEmbeddingService maps each entity to a team and counts saves.
type recordingEmbeddingService struct {
	teamOf func(entityID string) string

	mu    sync.Mutex
	saved map[string]int // entityID -> save count
}

func newRecordingEmbeddingService(teamOf func(string) string) *recordingEmbeddingService {
	return &recordingEmbeddingService{teamOf: teamOf, saved: map[string]int{}}
}

func (s *recordingEmbeddingService) ResolveEntityTeam(_ context.Context, _, _, entityID string) (string, error) {
	return s.teamOf(entityID), nil
}

func (s *recordingEmbeddingService) SaveEmbeddingChunks(
	_, _, entityID, _ string, _ []EmbeddingChunk,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved[entityID]++
	return nil
}

func (s *recordingEmbeddingService) savedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.saved)
}

func (s *recordingEmbeddingService) SaveEmbedding(_, _, _, _ string, _ [][]float32) error { return nil }
func (s *recordingEmbeddingService) GetEmbeddingsByEntity(_, _, _ string) ([]models.Embedding, error) {
	return nil, nil
}
func (s *recordingEmbeddingService) FindSimilar(_, _ string, _ []float32, _ int) ([]models.EmbeddingSimilarity, error) {
	return nil, nil
}
func (s *recordingEmbeddingService) DeleteEmbeddingsByEntity(_, _ string) error { return nil }

// syncBuffer is a concurrency-safe io.Writer for capturing log output.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newDispatcher(
	resolver ActiveEmbeddingProviderResolver,
	svc EmbeddingServiceInterface,
	logger *slog.Logger,
	retry EmbeddingRetryConfig,
) *EmbeddingDispatcher {
	engine := NewEmbeddingGenerationProcessor(resolver, svc, logger)
	return NewEmbeddingDispatcher(engine, 4, retry, logger)
}

func promptEvent(id string) events.Event {
	return events.NewPromptCreatedEvent(events.PromptCreatedPayload{
		PromptID:    id,
		UserID:      "u1",
		Email:       "e",
		ProjectName: "proj",
		Slug:        "slug",
		Title:       "Title",
		Description: "",
		Body:        "Body of " + id,
		CreatedAt:   time.Now(),
	})
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	require.Eventually(t, cond, 5*time.Second, 2*time.Millisecond, msg)
}

// A slow provider with concurrency=1 must never run two requests at once, and a
// burst far larger than the limit must still fully embed — no silent drops.
func TestEmbeddingDispatcher_BoundsToProviderConcurrency(t *testing.T) {
	t.Parallel()

	provider := &countingProvider{delay: 2 * time.Millisecond}
	resolver := &perTeamResolver{
		providers:   map[string]EmbeddingProvider{"team-1": provider},
		concurrency: map[string]int{"team-1": 1},
	}
	svc := newRecordingEmbeddingService(func(string) string { return "team-1" })

	d := newDispatcher(resolver, svc, slog.New(slog.DiscardHandler),
		EmbeddingRetryConfig{MaxRetries: 3, BaseBackoff: time.Millisecond})
	defer d.Stop()

	const k = 40
	for i := 0; i < k; i++ {
		require.NoError(t, d.ProcessEvent(context.Background(), promptEvent(fmt.Sprintf("p%d", i))))
	}

	waitFor(t, func() bool { return svc.savedCount() == k }, "all entities must be embedded")
	assert.Equal(t, int32(1), provider.tracker.max.Load(),
		"at most one request in flight to a concurrency=1 provider")
	assert.Equal(t, int32(k), provider.calls.Load())
}

// Two providers with different concurrency are each bounded independently.
func TestEmbeddingDispatcher_PerProviderIndependentLimits(t *testing.T) {
	t.Parallel()

	provA := &countingProvider{delay: 2 * time.Millisecond}
	provB := &countingProvider{delay: 2 * time.Millisecond}
	resolver := &perTeamResolver{
		providers:   map[string]EmbeddingProvider{"team-a": provA, "team-b": provB},
		concurrency: map[string]int{"team-a": 1, "team-b": 3},
	}
	svc := newRecordingEmbeddingService(func(id string) string {
		if strings.HasPrefix(id, "a") {
			return "team-a"
		}
		return "team-b"
	})

	d := newDispatcher(resolver, svc, slog.New(slog.DiscardHandler),
		EmbeddingRetryConfig{MaxRetries: 3, BaseBackoff: time.Millisecond})
	defer d.Stop()

	const per = 30
	for i := 0; i < per; i++ {
		require.NoError(t, d.ProcessEvent(context.Background(), promptEvent(fmt.Sprintf("a%d", i))))
		require.NoError(t, d.ProcessEvent(context.Background(), promptEvent(fmt.Sprintf("b%d", i))))
	}

	waitFor(t, func() bool { return svc.savedCount() == 2*per }, "all entities across both teams embed")
	assert.LessOrEqual(t, provA.tracker.max.Load(), int32(1), "team-a bounded at 1")
	assert.LessOrEqual(t, provB.tracker.max.Load(), int32(3), "team-b bounded at 3")
	assert.Greater(t, provB.tracker.max.Load(), int32(1), "team-b should exploit its higher limit")
}

// A permanently failing provider must exhaust bounded retries and log a terminal
// ERROR carrying the entity id — never a silent drop, and never an infinite loop.
func TestEmbeddingDispatcher_TerminalFailureLogsEntityID(t *testing.T) {
	t.Parallel()

	provider := &countingProvider{err: errors.New("provider exploded")}
	resolver := &perTeamResolver{
		providers:   map[string]EmbeddingProvider{"team-1": provider},
		concurrency: map[string]int{"team-1": 1},
	}
	svc := newRecordingEmbeddingService(func(string) string { return "team-1" })

	var logs syncBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	const maxRetries = 3
	d := newDispatcher(resolver, svc, logger,
		EmbeddingRetryConfig{MaxRetries: maxRetries, BaseBackoff: time.Millisecond})
	defer d.Stop()

	require.NoError(t, d.ProcessEvent(context.Background(), promptEvent(nonEmbeddableFailID)))

	waitFor(t, func() bool { return provider.calls.Load() == int32(maxRetries) },
		"provider is retried exactly MaxRetries times")
	// Give the terminal log a beat to flush after the last attempt.
	waitFor(t, func() bool { return strings.Contains(logs.String(), "failed after all retries") },
		"a terminal ERROR must be logged")

	out := logs.String()
	assert.Contains(t, out, nonEmbeddableFailID, "terminal log names the entity id")
	assert.Contains(t, out, `"level":"ERROR"`, "terminal failure logs at ERROR")
	assert.Equal(t, 0, svc.savedCount(), "nothing is saved on permanent failure")
}

const nonEmbeddableFailID = "prompt-doomed-42"

// erroringResolver always fails to resolve the active provider.
type erroringResolver struct{}

func (erroringResolver) ResolveActiveProvider(_ context.Context, _ string) (*ResolvedEmbeddingProvider, error) {
	return nil, errors.New("provider resolution boom")
}

// A resolve-stage failure must also be logged at ERROR with the entity id — the
// entity is left unembedded, never dropped silently.
func TestEmbeddingDispatcher_ResolveFailureLogsEntityID(t *testing.T) {
	t.Parallel()

	svc := newRecordingEmbeddingService(func(string) string { return "team-1" })
	var logs syncBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	d := newDispatcher(erroringResolver{}, svc, logger,
		EmbeddingRetryConfig{MaxRetries: 2, BaseBackoff: time.Millisecond})
	defer d.Stop()

	require.NoError(t, d.ProcessEvent(context.Background(), promptEvent("prompt-resolve-99")))

	waitFor(t, func() bool { return strings.Contains(logs.String(), "Failed to resolve embedding job") },
		"resolve failure must be logged")
	out := logs.String()
	assert.Contains(t, out, "prompt-resolve-99", "resolve-failure log names the entity id")
	assert.Contains(t, out, `"level":"ERROR"`)
	assert.Equal(t, 0, svc.savedCount())
}

// dynamicConcurrencyResolver reports a concurrency that can change between
// resolutions, exercising the executor-rebuild path.
type dynamicConcurrencyResolver struct {
	provider    EmbeddingProvider
	concurrency atomic.Int32
}

func (r *dynamicConcurrencyResolver) ResolveActiveProvider(
	_ context.Context, _ string,
) (*ResolvedEmbeddingProvider, error) {
	return &ResolvedEmbeddingProvider{
		Provider:     r.provider,
		ChunkSize:    1000,
		ChunkOverlap: 200,
		Concurrency:  int(r.concurrency.Load()),
	}, nil
}

// A provider concurrency change mid-stream must rebuild the per-provider executor
// without dropping any entity that was already enqueued at the old size.
func TestEmbeddingDispatcher_ConcurrencyChangeDropsNothing(t *testing.T) {
	t.Parallel()

	provider := &countingProvider{delay: time.Millisecond}
	resolver := &dynamicConcurrencyResolver{provider: provider}
	resolver.concurrency.Store(1)
	svc := newRecordingEmbeddingService(func(string) string { return "team-1" })

	d := newDispatcher(resolver, svc, slog.New(slog.DiscardHandler),
		EmbeddingRetryConfig{MaxRetries: 3, BaseBackoff: time.Millisecond})
	defer d.Stop()

	const k = 60
	for i := 0; i < k; i++ {
		if i == k/2 {
			resolver.concurrency.Store(3) // flip mid-burst; executor rebuilds
		}
		require.NoError(t, d.ProcessEvent(context.Background(), promptEvent(fmt.Sprintf("p%d", i))))
	}

	waitFor(t, func() bool { return svc.savedCount() == k },
		"every entity embeds across a concurrency change — none dropped")
	assert.Equal(t, int32(k), provider.calls.Load())
}

// --- submitted/terminal ledger (#755) ---

// countJSONLines counts how many captured log lines contain every given substring.
// The handler writes one JSON object per line, so this is an exact per-entity count
// rather than the substring-anywhere check the older assertions use.
func countJSONLines(out string, contains ...string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		matched := true
		for _, c := range contains {
			if !strings.Contains(line, c) {
				matched = false
				break
			}
		}
		if matched {
			n++
		}
	}
	return n
}

// lineIndexOf returns the index of the first captured log line containing every
// given substring, or -1. Used to pin the ordering of the submitted/terminal pair.
func lineIndexOf(out string, contains ...string) int {
	for i, line := range strings.Split(out, "\n") {
		matched := true
		for _, c := range contains {
			if !strings.Contains(line, c) {
				matched = false
				break
			}
		}
		if matched && strings.TrimSpace(line) != "" {
			return i
		}
	}
	return -1
}

// Every submitted entity must produce exactly one submitted line carrying its
// identifiers and exactly one terminal line — that pair is what turns a job lost
// between the two (the in-memory queue is not durable, #820) into a visible gap.
func TestEmbeddingDispatcher_SubmittedAndTerminalLinesPairUpOnSuccess(t *testing.T) {
	t.Parallel()

	provider := &countingProvider{}
	resolver := &perTeamResolver{
		providers:   map[string]EmbeddingProvider{"team-1": provider},
		concurrency: map[string]int{"team-1": 2},
	}
	svc := newRecordingEmbeddingService(func(string) string { return "team-1" })

	var logs syncBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	d := newDispatcher(resolver, svc, logger,
		EmbeddingRetryConfig{MaxRetries: 3, BaseBackoff: time.Millisecond})
	// Stop is idempotent, so the explicit quiescing Stop below still works; this
	// defer only guarantees the workers are reaped when an assertion fails first.
	defer d.Stop()

	const k = 5
	for i := 0; i < k; i++ {
		require.NoError(t, d.ProcessEvent(context.Background(), promptEvent(fmt.Sprintf("ledger-%d", i))))
	}

	waitFor(t, func() bool { return svc.savedCount() == k }, "all entities must embed")
	// Stop drains every executor, so the ledger is quiesced and the totals are
	// comparable — a snapshot taken mid-flight would legitimately show a shortfall.
	d.Stop()

	out := logs.String()
	for i := 0; i < k; i++ {
		id := fmt.Sprintf("ledger-%d", i)
		assert.Equal(t, 1, countJSONLines(out, "Embedding job submitted", id),
			"exactly one submitted line for %s", id)
		assert.Equal(t, 1, countJSONLines(out, "Embedding job completed", id),
			"exactly one terminal line for %s", id)
	}
	// The submitted line must carry the fields an operator diffs on.
	assert.Contains(t, out, `"entity_type":"prompt"`)
	assert.Contains(t, out, `"team_id":"team-1"`)
	// Submitted is logged before the job is enqueued, so it can never be ordered
	// after the terminal line it opens — a worker can otherwise finish the job
	// while the submitting goroutine is still on its way to the log call.
	for i := 0; i < k; i++ {
		id := fmt.Sprintf("ledger-%d", i)
		sub := lineIndexOf(out, "Embedding job submitted", id)
		term := lineIndexOf(out, "Embedding job completed", id)
		require.NotEqual(t, -1, sub)
		require.NotEqual(t, -1, term)
		assert.Less(t, sub, term, "submitted must precede its terminal line for %s", id)
	}

	stats := d.Stats()
	assert.Equal(t, int64(k), stats.Submitted)
	assert.Equal(t, int64(k), stats.Succeeded)
	assert.Zero(t, stats.Failed)
	assert.Equal(t, stats.Submitted, stats.Succeeded+stats.Failed,
		"a quiesced dispatcher must account for every submitted job")
}

// The exhausted-retries path is the other terminal: one submitted line, one ERROR,
// and no success line — so the ledger still balances when nothing gets embedded.
func TestEmbeddingDispatcher_SubmittedAndTerminalLinesPairUpOnFailure(t *testing.T) {
	t.Parallel()

	provider := &countingProvider{err: errors.New("provider exploded")}
	resolver := &perTeamResolver{
		providers:   map[string]EmbeddingProvider{"team-1": provider},
		concurrency: map[string]int{"team-1": 1},
	}
	svc := newRecordingEmbeddingService(func(string) string { return "team-1" })

	var logs syncBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	d := newDispatcher(resolver, svc, logger,
		EmbeddingRetryConfig{MaxRetries: 2, BaseBackoff: time.Millisecond})
	defer d.Stop()

	const id = "ledger-doomed"
	require.NoError(t, d.ProcessEvent(context.Background(), promptEvent(id)))

	waitFor(t, func() bool { return strings.Contains(logs.String(), "failed after all retries") },
		"a terminal ERROR must be logged")
	d.Stop()

	out := logs.String()
	assert.Equal(t, 1, countJSONLines(out, "Embedding job submitted", id))
	assert.Equal(t, 1, countJSONLines(out, "failed after all retries", id))
	assert.Zero(t, countJSONLines(out, "Embedding job completed", id),
		"a job that never embedded must not log a success terminal")

	stats := d.Stats()
	assert.Equal(t, int64(1), stats.Submitted)
	assert.Equal(t, int64(1), stats.Failed)
	assert.Zero(t, stats.Succeeded)
	assert.Equal(t, stats.Submitted, stats.Succeeded+stats.Failed)
}

// A resolve-stage failure happens BEFORE the job reaches a provider executor, so it
// must not enter the ledger at all — otherwise every unembeddable event would read
// as a lost job.
func TestEmbeddingDispatcher_ResolveFailureIsNotCountedAsSubmitted(t *testing.T) {
	t.Parallel()

	svc := newRecordingEmbeddingService(func(string) string { return "team-1" })
	var logs syncBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	d := newDispatcher(erroringResolver{}, svc, logger,
		EmbeddingRetryConfig{MaxRetries: 2, BaseBackoff: time.Millisecond})
	defer d.Stop()

	require.NoError(t, d.ProcessEvent(context.Background(), promptEvent("never-submitted")))

	waitFor(t, func() bool { return strings.Contains(logs.String(), "Failed to resolve embedding job") },
		"resolve failure must be logged")
	d.Stop()

	assert.Zero(t, countJSONLines(logs.String(), "Embedding job submitted"),
		"nothing was submitted, so nothing may be logged as submitted")
	assert.Equal(t, EmbeddingDispatcherStats{}, d.Stats())
}
