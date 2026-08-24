package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/pkg/events"
)

// These tests cover the dispatcher's half of the durable queue (#820): what it
// writes on accept, and how it settles a claimed row. The queue's own
// guarantees -- SKIP LOCKED, lease expiry, the coalescing index -- are proven
// against real Postgres in internal/repositories/postgres, because a mock can
// only replay what the test told it and so cannot fail on them.

// fakeQueue is a hand-written in-memory EmbeddingJobRepository. It is preferred
// over the generated mock for the run-loop tests because the poller calls Claim
// repeatedly on its own schedule, which mockery's call-count expectations model
// badly; it records every settle so a test can assert the terminal decision.
type fakeQueue struct {
	mu        sync.Mutex
	enqueued  []*models.EmbeddingJob
	claimable []*models.EmbeddingJob
	acked     []string
	released  []string
	failed    []string
	lastError string
	settled   chan string
}

func newFakeQueue() *fakeQueue {
	return &fakeQueue{settled: make(chan string, 16)}
}

func (q *fakeQueue) Enqueue(_ context.Context, job *models.EmbeddingJob) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	job.ID = fmt.Sprintf("job-%d", len(q.enqueued)+1)
	job.State = models.EmbeddingJobStatePending
	q.enqueued = append(q.enqueued, job)
	return nil
}

// offer makes a job claimable exactly once, the way a real claim consumes a row.
func (q *fakeQueue) offer(job *models.EmbeddingJob) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.claimable = append(q.claimable, job)
}

func (q *fakeQueue) Claim(
	_ context.Context, workerID string, limit int, _ time.Duration, _ int,
) ([]*models.EmbeddingJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.claimable) == 0 {
		return nil, nil
	}
	if limit > len(q.claimable) {
		limit = len(q.claimable)
	}
	out := q.claimable[:limit]
	q.claimable = q.claimable[limit:]
	for _, job := range out {
		job.State = models.EmbeddingJobStateClaimed
		job.Attempts++
		owner := workerID
		job.ClaimedBy = &owner
	}
	return out, nil
}

func (q *fakeQueue) Ack(_ context.Context, id, _ string) (bool, error) {
	q.settle(&q.acked, id, "")
	return true, nil
}

func (q *fakeQueue) Release(_ context.Context, id, _, lastErr string, _ time.Duration) (bool, error) {
	q.settle(&q.released, id, lastErr)
	return true, nil
}

func (q *fakeQueue) Fail(_ context.Context, id, _, lastErr string) (bool, error) {
	q.settle(&q.failed, id, lastErr)
	return true, nil
}

func (q *fakeQueue) FailExhausted(context.Context, int) (int64, error) { return 0, nil }

func (q *fakeQueue) CountByState(context.Context) (map[string]int64, error) { return nil, nil }

func (q *fakeQueue) settle(bucket *[]string, id, lastErr string) {
	q.mu.Lock()
	*bucket = append(*bucket, id)
	if lastErr != "" {
		q.lastError = lastErr
	}
	q.mu.Unlock()
	select {
	case q.settled <- id:
	default:
	}
}

func (q *fakeQueue) snapshot() (acked, released, failed []string, lastErr string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]string(nil), q.acked...), append([]string(nil), q.released...),
		append([]string(nil), q.failed...), q.lastError
}

// awaitSettle waits for one job to reach a terminal decision.
func (q *fakeQueue) awaitSettle(t *testing.T) {
	t.Helper()
	select {
	case <-q.settled:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the job to be settled")
	}
}

// newQueuedDispatcher builds a dispatcher whose backlog is the given queue. The
// poll interval is long on purpose: every test here drives the poller through an
// enqueue signal or the startup sweep, so a test that passes only because a
// timer fired would be hiding a broken wake-up path.
func newQueuedDispatcher(
	resolver ActiveEmbeddingProviderResolver,
	svc EmbeddingServiceInterface,
	queue *fakeQueue,
	retry EmbeddingRetryConfig,
) *EmbeddingDispatcher {
	logger := slog.New(slog.DiscardHandler)
	engine := NewEmbeddingGenerationProcessor(resolver, svc, logger)
	d := NewEmbeddingDispatcher(engine, 4, retry, logger,
		WithDurableQueue(queue, EmbeddingQueueConfig{PollInterval: time.Hour}))
	d.Start()
	return d
}

// queuedJob builds a claimed-shaped job for an entity the fixtures know.
func queuedJob(id, entityID string) *models.EmbeddingJob {
	return &models.EmbeddingJob{
		ID:         id,
		EntityType: "prompt",
		EntityID:   entityID,
		UserID:     "u1",
		Payload:    models.EmbeddingJobPayload{Title: "Title", Body: "Body of " + entityID},
	}
}

func embeddingFixtures(provider EmbeddingProvider) (*perTeamResolver, *recordingEmbeddingService) {
	resolver := &perTeamResolver{
		providers:   map[string]EmbeddingProvider{"team-1": provider},
		concurrency: map[string]int{"team-1": 2},
	}
	return resolver, newRecordingEmbeddingService(func(string) string { return "team-1" })
}

// TestEmbeddingDispatcher_ProcessEventPersistsBeforeReturning is the core of
// #820: acceptance and durability must be the same instant. If the row were
// written on a background worker instead, a crash between the two would lose the
// entity exactly as the in-memory queue did.
func TestEmbeddingDispatcher_ProcessEventPersistsBeforeReturning(t *testing.T) {
	t.Parallel()

	queue := newFakeQueue()
	resolver, svc := embeddingFixtures(&countingProvider{})
	d := newQueuedDispatcher(resolver, svc, queue, EmbeddingRetryConfig{MaxRetries: 1})
	defer d.Stop()

	require.NoError(t, d.ProcessEvent(context.Background(), promptEvent("p1")))

	queue.mu.Lock()
	defer queue.mu.Unlock()
	require.Len(t, queue.enqueued, 1, "the row must exist by the time ProcessEvent returns")
	assert.Equal(t, "prompt", queue.enqueued[0].EntityType)
	assert.Equal(t, "p1", queue.enqueued[0].EntityID)
	assert.Equal(t, "u1", queue.enqueued[0].UserID)
	assert.Equal(t, "Title", queue.enqueued[0].Payload.Title)
	assert.Contains(t, queue.enqueued[0].Payload.Body, "Body of p1",
		"the stored payload must carry the text to embed -- the event is gone by the time the job runs")
}

// A failed enqueue must be REPORTED, not swallowed. The event bus retries a
// handler that returns an error, so surfacing it is what keeps a database blip
// from becoming the very silent drop this issue removes.
func TestEmbeddingDispatcher_ProcessEventReportsAFailedEnqueue(t *testing.T) {
	t.Parallel()

	queue := repomocks.NewMockEmbeddingJobRepository(t)
	queue.EXPECT().Enqueue(mock.Anything, mock.Anything).Return(errors.New("db is down")).Once()
	queue.EXPECT().FailExhausted(mock.Anything, mock.Anything).Return(0, nil).Maybe()
	queue.EXPECT().Claim(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, nil).Maybe()

	resolver, svc := embeddingFixtures(&countingProvider{})
	logger := slog.New(slog.DiscardHandler)
	engine := NewEmbeddingGenerationProcessor(resolver, svc, logger)
	d := NewEmbeddingDispatcher(engine, 4, EmbeddingRetryConfig{MaxRetries: 1}, logger,
		WithDurableQueue(queue, EmbeddingQueueConfig{PollInterval: time.Hour}))
	d.Start()
	defer d.Stop()

	err := d.ProcessEvent(context.Background(), promptEvent("p1"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "durably enqueue")
}

// An event with nothing to embed must cost no database write at all: extraction
// is pure, so the queue should never see a row it would only ever no-op on.
func TestEmbeddingDispatcher_ProcessEventSkipsUnembeddableEvents(t *testing.T) {
	t.Parallel()

	queue := newFakeQueue()
	resolver, svc := embeddingFixtures(&countingProvider{})
	d := newQueuedDispatcher(resolver, svc, queue, EmbeddingRetryConfig{MaxRetries: 1})
	defer d.Stop()

	// A user.created event is not an embeddable type at all.
	require.NoError(t, d.ProcessEvent(context.Background(),
		events.NewUserCreatedEvent("u1", "u@example.com", "User", time.Now())))
	// An embeddable type carrying no text is likewise nothing to queue.
	require.NoError(t, d.ProcessEvent(context.Background(), blankPromptEvent()))

	queue.mu.Lock()
	defer queue.mu.Unlock()
	assert.Empty(t, queue.enqueued, "an unembeddable event must not reach the database")
}

// blankPromptEvent is an embeddable event type carrying no embeddable text.
func blankPromptEvent() events.Event {
	return events.NewPromptCreatedEvent(events.PromptCreatedPayload{
		PromptID: "p-empty",
		UserID:   "u1",
		Slug:     "slug",
	})
}

// A claimed job that embeds cleanly must be ACKED. Without the ack its lease
// eventually expires and the entity is embedded all over again, forever.
func TestEmbeddingDispatcher_ClaimedJobIsAckedOnSuccess(t *testing.T) {
	t.Parallel()

	queue := newFakeQueue()
	queue.offer(queuedJob("job-1", "p1"))
	resolver, svc := embeddingFixtures(&countingProvider{})

	d := newQueuedDispatcher(resolver, svc, queue, EmbeddingRetryConfig{MaxRetries: 1})
	defer d.Stop()

	queue.awaitSettle(t)

	acked, released, failed, _ := queue.snapshot()
	assert.Equal(t, []string{"job-1"}, acked)
	assert.Empty(t, released)
	assert.Empty(t, failed)
	assert.Equal(t, 1, svc.savedCount(), "the stored payload must actually have been embedded")
}

// TestEmbeddingDispatcher_ConstructionTouchesNoDatabase pins why the poller has
// its own Start: the first sweep queries immediately, so starting it from the
// constructor would make merely BUILDING the DI container hit the database.
// Every handler test assembles one with a placeholder *database.DB, so that
// version panicked in a background goroutine and took whole packages with it.
func TestEmbeddingDispatcher_ConstructionTouchesNoDatabase(t *testing.T) {
	t.Parallel()

	// A mock with NO expectations at all fails the test if any method is called.
	queue := repomocks.NewMockEmbeddingJobRepository(t)
	resolver, svc := embeddingFixtures(&countingProvider{})
	logger := slog.New(slog.DiscardHandler)
	engine := NewEmbeddingGenerationProcessor(resolver, svc, logger)

	d := NewEmbeddingDispatcher(engine, 4, EmbeddingRetryConfig{MaxRetries: 1}, logger,
		WithDurableQueue(queue, EmbeddingQueueConfig{PollInterval: time.Millisecond}))
	defer d.Stop()

	// Even with a poll interval that would have fired many times over.
	time.Sleep(50 * time.Millisecond)
}

// TestEmbeddingDispatcher_StartupClaimsWorkLeftBehind is restart recovery seen
// from the dispatcher's side: a fresh process must pick up rows it never
// enqueued, with no enqueue signal and before any poll timer could fire.
func TestEmbeddingDispatcher_StartupClaimsWorkLeftBehind(t *testing.T) {
	t.Parallel()

	queue := newFakeQueue()
	// Three rows a previous process left outstanding.
	for i := 1; i <= 3; i++ {
		queue.offer(queuedJob(fmt.Sprintf("job-%d", i), fmt.Sprintf("p%d", i)))
	}
	resolver, svc := embeddingFixtures(&countingProvider{})

	d := newQueuedDispatcher(resolver, svc, queue, EmbeddingRetryConfig{MaxRetries: 1})
	defer d.Stop()

	waitFor(t, func() bool { return svc.savedCount() == 3 },
		"a restarted dispatcher must resume the queue without being told to")

	waitFor(t, func() bool {
		acked, _, _, _ := queue.snapshot()
		return len(acked) == 3
	}, "every resumed job must be acked")
}

// A retryable failure must RELEASE the job so it is tried again, carrying the
// error forward for an operator.
func TestEmbeddingDispatcher_RetryableFailureReleasesTheJob(t *testing.T) {
	t.Parallel()

	queue := newFakeQueue()
	queue.offer(queuedJob("job-1", "p1"))
	provider := &countingProvider{err: errors.New("provider unreachable")}
	resolver, svc := embeddingFixtures(provider)

	d := newQueuedDispatcher(resolver, svc, queue, EmbeddingRetryConfig{
		MaxRetries: 1, BaseBackoff: time.Millisecond,
	})
	defer d.Stop()

	queue.awaitSettle(t)

	acked, released, failed, lastErr := queue.snapshot()
	assert.Empty(t, acked)
	assert.Equal(t, []string{"job-1"}, released, "an unreachable provider is worth retrying")
	assert.Empty(t, failed)
	assert.Contains(t, lastErr, "provider unreachable")
}

// A provider rejection an identical retry cannot fix (#756) must be TERMINAL, so
// a poisonous entity stops consuming claims and provider quota.
func TestEmbeddingDispatcher_PermanentProviderErrorFailsTheJob(t *testing.T) {
	t.Parallel()

	queue := newFakeQueue()
	queue.offer(queuedJob("job-1", "p1"))
	provider := &countingProvider{err: &providerHTTPError{StatusCode: 400, Body: "bad request"}}
	resolver, svc := embeddingFixtures(provider)

	d := newQueuedDispatcher(resolver, svc, queue, EmbeddingRetryConfig{
		MaxRetries: 3, BaseBackoff: time.Millisecond,
	})
	defer d.Stop()

	queue.awaitSettle(t)

	acked, released, failed, _ := queue.snapshot()
	assert.Empty(t, acked)
	assert.Empty(t, released, "a permanent rejection must not go back on the queue")
	assert.Equal(t, []string{"job-1"}, failed)
}

// A team with no active provider is not a failure and not retryable work: acking
// is what stops such a job cycling through its attempt bound and being reported
// as a poison pill it never was.
func TestEmbeddingDispatcher_JobWithNoProviderIsAcked(t *testing.T) {
	t.Parallel()

	queue := newFakeQueue()
	queue.offer(queuedJob("job-1", "p1"))
	// An empty resolver map means "this team has no active provider".
	resolver := &perTeamResolver{providers: map[string]EmbeddingProvider{}}
	svc := newRecordingEmbeddingService(func(string) string { return "team-1" })

	d := newQueuedDispatcher(resolver, svc, queue, EmbeddingRetryConfig{MaxRetries: 1})
	defer d.Stop()

	queue.awaitSettle(t)

	acked, released, failed, _ := queue.snapshot()
	assert.Equal(t, []string{"job-1"}, acked)
	assert.Empty(t, released)
	assert.Empty(t, failed)
	assert.Zero(t, svc.savedCount())
}

// The #755 submitted/terminal ledger must stay balanced on the durable path:
// that invariant is what an operator diffs to spot lost work, and it would be
// worse than useless if the new code path stopped maintaining it.
func TestEmbeddingDispatcher_QueuedJobsKeepTheLedgerBalanced(t *testing.T) {
	t.Parallel()

	queue := newFakeQueue()
	for i := 1; i <= 5; i++ {
		queue.offer(queuedJob(fmt.Sprintf("job-%d", i), fmt.Sprintf("p%d", i)))
	}
	resolver, svc := embeddingFixtures(&countingProvider{})

	d := newQueuedDispatcher(resolver, svc, queue, EmbeddingRetryConfig{MaxRetries: 1})
	defer d.Stop()

	waitFor(t, func() bool { return svc.savedCount() == 5 }, "every queued job must run")
	waitFor(t, func() bool {
		s := d.Stats()
		return s.Submitted == 5 && s.Succeeded+s.Failed == 5
	}, "submitted must equal succeeded+failed once the queue is drained")
}

// TestEmbeddingDispatcher_ClaimsNoMoreThanItCanRun pins the reason the poller
// asks for a bounded batch: an intake worker is occupied for its job's whole
// lifetime, so claiming past that would lease rows nothing is working -- leases
// ticking down on jobs sitting in a Go slice, which is the in-memory backlog
// this issue set out to remove.
func TestEmbeddingDispatcher_ClaimsNoMoreThanItCanRun(t *testing.T) {
	t.Parallel()

	limits := make(chan int, 8)
	queue := repomocks.NewMockEmbeddingJobRepository(t)
	queue.EXPECT().FailExhausted(mock.Anything, mock.Anything).Return(0, nil).Maybe()
	queue.EXPECT().
		Claim(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, limit int, _ time.Duration, _ int) ([]*models.EmbeddingJob, error) {
			select {
			case limits <- limit:
			default:
			}
			return nil, nil
		}).Maybe()

	resolver, svc := embeddingFixtures(&countingProvider{})
	logger := slog.New(slog.DiscardHandler)
	engine := NewEmbeddingGenerationProcessor(resolver, svc, logger)
	// batch_size deliberately larger than the 4 intake workers, so the cap that
	// wins has to be the worker count rather than the configured batch.
	d := NewEmbeddingDispatcher(engine, 4, EmbeddingRetryConfig{MaxRetries: 1}, logger,
		WithDurableQueue(queue, EmbeddingQueueConfig{PollInterval: time.Hour, BatchSize: 100}))
	d.Start()
	defer d.Stop()

	select {
	case limit := <-limits:
		assert.LessOrEqual(t, limit, 4, "a claim must never lease more jobs than there are workers to run them")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the startup claim")
	}
}

// A zero EmbeddingQueueConfig must be usable: the wired provider passes config
// values straight through, so an operator who omits the section entirely gets
// working defaults rather than a ticker panic or a queue that claims nothing.
func TestEmbeddingQueueConfig_ZeroValueGetsWorkingDefaults(t *testing.T) {
	t.Parallel()

	cfg := EmbeddingQueueConfig{}.withDefaults()

	assert.Equal(t, defaultEmbeddingQueueLease, cfg.LeaseDuration)
	assert.Equal(t, defaultEmbeddingQueueMaxAttempts, cfg.MaxAttempts)
	assert.Equal(t, defaultEmbeddingQueueBatchSize, cfg.BatchSize)
	assert.Equal(t, defaultEmbeddingQueuePollInterval, cfg.PollInterval)
	assert.Greater(t, cfg.LeaseDuration, embeddingJobTimeout,
		"a lease shorter than one job's own timeout would reclaim work that is still running")

	// Explicit values are preserved, so the config knobs are not decorative.
	explicit := EmbeddingQueueConfig{
		LeaseDuration: 5 * time.Minute,
		MaxAttempts:   2,
		BatchSize:     7,
		PollInterval:  time.Second,
		RetryBackoff:  30 * time.Second,
	}.withDefaults()
	assert.Equal(t, EmbeddingQueueConfig{
		LeaseDuration: 5 * time.Minute,
		MaxAttempts:   2,
		BatchSize:     7,
		PollInterval:  time.Second,
		RetryBackoff:  30 * time.Second,
	}, explicit)
}

// TestEmbeddingDispatcher_QueuedJobsRespectProviderConcurrency is #820's
// "provider concurrency caps still hold" criterion, asserted on the DURABLE
// path specifically. The equivalent existing test drives the pre-#820 in-memory
// route, so it cannot see a regression in the new one -- and the new one is
// where it would happen, because the poller decides how many jobs to lease.
func TestEmbeddingDispatcher_QueuedJobsRespectProviderConcurrency(t *testing.T) {
	t.Parallel()

	provider := &countingProvider{delay: 2 * time.Millisecond}
	queue := newFakeQueue()
	const jobs = 20
	for i := 1; i <= jobs; i++ {
		queue.offer(queuedJob(fmt.Sprintf("job-%d", i), fmt.Sprintf("p%d", i)))
	}

	// concurrency 1, deliberately below the 4 intake workers: the cap that has to
	// win is the provider's, not the poller's claim budget.
	resolver := &perTeamResolver{
		providers:   map[string]EmbeddingProvider{"team-1": provider},
		concurrency: map[string]int{"team-1": 1},
	}
	svc := newRecordingEmbeddingService(func(string) string { return "team-1" })

	d := newQueuedDispatcher(resolver, svc, queue, EmbeddingRetryConfig{MaxRetries: 1})
	defer d.Stop()

	waitFor(t, func() bool { return svc.savedCount() == jobs },
		"every queued job must embed -- a bounded queue must not drop the overflow")
	assert.LessOrEqual(t, provider.tracker.max.Load(), int32(1),
		"no more than the provider's configured concurrency may run at once")
}
