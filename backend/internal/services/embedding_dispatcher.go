package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

const (
	// embeddingJobTimeout bounds one entity's full embed attempt sequence
	// (all retries included). It is decoupled from the event's own context,
	// which may already be cancelled by the time an enqueued job runs.
	embeddingJobTimeout = 2 * time.Minute

	// maxEmbeddingRetryBackoff caps the base retry interval before jitter.
	maxEmbeddingRetryBackoff = 30 * time.Second

	// logComponentEmbeddingDispatcher is the "component" structured-log field
	// value for the dispatcher's log lines.
	logComponentEmbeddingDispatcher = "embedding-dispatcher"
)

// EmbeddingRetryConfig is the bounded-retry policy applied to a single entity's
// embedding generation. It mirrors the event bus's retry knobs so operators tune
// one set of values (event_bus.*) for both paths.
type EmbeddingRetryConfig struct {
	MaxRetries  int
	BaseBackoff time.Duration
	Jitter      bool
}

// EmbeddingDispatcher is the concurrency-bounded, event-driven embedding path
// (#142). It implements events.EmbeddingProcessor but, unlike the synchronous
// EmbeddingGenerationProcessor it wraps, it does not generate on the caller's
// goroutine: ProcessEvent only enqueues (no I/O, never blocks), so the event bus
// never rides its unbounded `go task()` saturation fallback for embedding work.
//
// It runs two stages, each a boundedExecutor:
//   - resolve: a fixed pool drains the intake queue and, for each event, resolves
//     the entity's team + active provider — the DB calls that must stay off the
//     bus dispatch goroutine — then routes the job to that provider's executor.
//   - generate: one executor per team's active provider, sized by
//     ResolvedEmbeddingProvider.Concurrency, so no more than that many requests
//     are ever in flight to a single provider regardless of how many entities are
//     enqueued. Each job runs GenerateEmbeddings + save with bounded retry and, on
//     terminal failure, logs at ERROR with the entity id (the silent WARN-only
//     drop was the core defect this issue fixes).
//
// Every generate job is bracketed by a submitted line and exactly one terminal line
// (#755). The pair is what makes a loss between the two diffable from the logs
// alone: an entity with a submitted line and no terminal line is work that was
// accepted and never finished.
//
// With a durable queue wired (#820) the two executors stop being the BACKLOG and
// become only the concurrency limiters. Every accepted event is written to
// embedding_jobs before ProcessEvent returns; a poller then leases rows out of
// that table and runs them through the same two stages. A restart no longer
// discards queued work: the rows are still there, and a lease left behind by a
// dead process expires and is reclaimed.
type EmbeddingDispatcher struct {
	engine *EmbeddingGenerationProcessor
	retry  EmbeddingRetryConfig
	logger *slog.Logger

	intake *boundedExecutor
	// intakeWorkers is how many jobs may be in flight at once, which is also the
	// cap the queue poller claims against: an intake worker stays occupied for its
	// job's whole lifetime (it blocks on the generate stage), so claiming beyond
	// this would only lease rows nothing can run.
	intakeWorkers int

	stats dispatcherStats

	// queue, when non-nil, is the durable system of record for outstanding work
	// (#820). Nil keeps the pre-#820 behaviour: ProcessEvent routes straight onto
	// the in-memory executors, which is what the executor-level unit tests drive.
	queue    repositories.EmbeddingJobRepository
	queueCfg EmbeddingQueueConfig
	// workerID identifies this process's claims. It is regenerated per dispatcher,
	// so a restarted process never matches its predecessor's claimed_by and cannot
	// ack a job it did not run.
	workerID  string
	wake      chan struct{}
	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	pollerWG  sync.WaitGroup
	inFlight  atomic.Int64

	mu    sync.Mutex
	execs map[string]*providerExecutor
}

// dispatcherStats is the dispatcher's in-process submitted/terminal ledger. It
// mirrors the log lines rather than replacing them: the logs are the operator-facing
// surface (they survive the process and carry entity ids), while these counters give
// tests — and any future metrics export — a cheap, race-free total.
//
// The invariant is Submitted == Succeeded + Failed once every in-flight job has
// finished; a persistent shortfall is work the process lost.
type dispatcherStats struct {
	submitted atomic.Int64
	succeeded atomic.Int64
	failed    atomic.Int64
}

// EmbeddingDispatcherStats is a point-in-time snapshot of the dispatcher's ledger.
type EmbeddingDispatcherStats struct {
	// Submitted counts generate jobs accepted onto a provider executor.
	Submitted int64
	// Succeeded counts jobs whose embeddings were generated and saved.
	Succeeded int64
	// Failed counts jobs that exhausted their retries (or were cut short by the
	// job timeout) and left the entity unembedded.
	Failed int64
}

// Stats returns a snapshot of the submitted/terminal ledger. The three reads are
// not atomic with respect to each other, so a snapshot taken while jobs are in
// flight can show Submitted > Succeeded+Failed — that is the in-flight set, not a
// loss. Compare them only once the dispatcher is quiesced.
func (d *EmbeddingDispatcher) Stats() EmbeddingDispatcherStats {
	return EmbeddingDispatcherStats{
		Submitted: d.stats.submitted.Load(),
		Succeeded: d.stats.succeeded.Load(),
		Failed:    d.stats.failed.Load(),
	}
}

// providerExecutor is a team's active-provider generate executor plus the
// concurrency it was sized for, so a later provider change can be detected and
// the executor rebuilt at the new size.
type providerExecutor struct {
	concurrency int
	exec        *boundedExecutor
}

var (
	_ events.EmbeddingProcessor = (*EmbeddingDispatcher)(nil)
	// ManagesOwnConcurrency signals the bus to dispatch this processor's worker
	// inline rather than through the shared, unbounded worker pool (#142).
	_ interface{ ManagesOwnConcurrency() bool } = (*EmbeddingDispatcher)(nil)
)

// NewEmbeddingDispatcher builds a dispatcher around a generation engine.
// resolveWorkers bounds concurrent provider/team resolution (the DB stage); the
// per-provider generate concurrency comes from each resolved provider.
// Pass WithDurableQueue to make it durable (#820); without it the backlog is
// in-memory only and a restart discards it.
func NewEmbeddingDispatcher(
	engine *EmbeddingGenerationProcessor,
	resolveWorkers int,
	retry EmbeddingRetryConfig,
	logger *slog.Logger,
	opts ...EmbeddingDispatcherOption,
) *EmbeddingDispatcher {
	if resolveWorkers < 1 {
		resolveWorkers = 1
	}
	if retry.MaxRetries < 1 {
		retry.MaxRetries = 1
	}
	if retry.BaseBackoff <= 0 {
		retry.BaseBackoff = 200 * time.Millisecond
	}
	d := &EmbeddingDispatcher{
		engine:        engine,
		retry:         retry,
		logger:        logger,
		intakeWorkers: resolveWorkers,
		workerID:      uuid.NewString(),
		wake:          make(chan struct{}, 1),
		stopCh:        make(chan struct{}),
		execs:         make(map[string]*providerExecutor),
	}
	for _, opt := range opts {
		opt(d)
	}
	d.intake = newBoundedExecutor(resolveWorkers)
	if d.queue != nil {
		d.queueCfg = d.queueCfg.withDefaults()
	}
	return d
}

// Start launches the durable queue's poller (#820). It is separate from
// construction on purpose: the poller's first sweep is the restart-recovery
// path, so it queries the database immediately, and a container assembled with a
// placeholder database (as every handler test does) must not be made to survive
// that. Callers start it once the database is migrated and ready, exactly like
// the scheduler.
//
// It is a no-op without a durable queue, and idempotent.
func (d *EmbeddingDispatcher) Start() {
	if d.queue == nil {
		return
	}
	d.startOnce.Do(func() {
		d.pollerWG.Add(1)
		go d.pollQueue()
	})
}

// ManagesOwnConcurrency reports that generation runs on this dispatcher's own
// bounded, per-provider workers, so the event bus invokes the embedding worker
// inline instead of through the shared worker pool.
func (d *EmbeddingDispatcher) ManagesOwnConcurrency() bool { return true }

// ProcessEvent accepts the event for asynchronous, concurrency-bounded embedding.
//
// With a durable queue wired it writes ONE row to embedding_jobs and returns; the
// poller picks it up. That single insert is the price of durability and it is
// paid deliberately on the caller's goroutine: enqueueing in the background first
// would reopen exactly the window this issue closes, because a crash between
// "accepted" and "persisted" is indistinguishable from the in-memory loss #755
// investigated. Extraction of the event's text is pure, so an event with nothing
// to embed costs no database call at all.
//
// Without a queue it keeps the pre-#820 behaviour: no I/O, straight onto the
// in-memory intake executor.
func (d *EmbeddingDispatcher) ProcessEvent(ctx context.Context, event events.Event) error {
	if d.queue == nil {
		if !d.intake.submit(func() { d.resolveAndRoute(event) }) {
			return fmt.Errorf("embedding dispatcher is stopped")
		}
		return nil
	}
	return d.enqueueDurably(ctx, event)
}

// resolveAndRoute resolves the event's team + provider and routes the generate
// job to that provider's bounded executor. It runs on a resolve-stage worker, off
// the bus dispatch goroutine, so the per-entity DB lookups never stall dispatch.
func (d *EmbeddingDispatcher) resolveAndRoute(event events.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), embeddingJobTimeout)
	defer cancel()

	input, teamID, resolved, err := d.engine.resolveJob(ctx, event)
	if err != nil {
		// A real resolution failure (team lookup / provider decode). Surface it at
		// ERROR with the entity id — silently swallowing it would be exactly the
		// invisible drop #142 exists to eliminate.
		d.entityLogger(input, teamID).With(
			"event_type", event.Type(),
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to resolve embedding job; entity left unembedded")
		return
	}
	if resolved == nil {
		return // not embeddable, no text, or no provider configured (engine logs)
	}

	d.submitGenerate(input, teamID, resolved)
}

// submitGenerate is the fire-and-forget form used by the pre-#820 in-memory path:
// it hands the job to the provider executor and returns without waiting.
func (d *EmbeddingDispatcher) submitGenerate(input embeddingInput, teamID string, resolved *ResolvedEmbeddingProvider) {
	d.runGenerate(input, teamID, resolved, false)
}

// runGenerate enqueues an entity's generate job onto its provider's executor,
// creating that executor (sized to concurrency) on first use. If the provider's
// concurrency has changed since the executor was built, the old one is retired in
// the background — it drains what it already holds — and a new one is created at
// the new size, so a rare admin concurrency change is picked up without a
// restart. The get-or-create and the submit happen under one lock so a concurrent
// concurrency change can never close the executor between the two and drop the
// job.
//
// When wait is set the caller blocks until the job reaches its terminal state and
// gets that outcome back. That is what the durable path needs: the row's lease is
// only meaningful while somebody is actually working it, and the ack has to
// happen after the outcome is known. It also keeps the provider executor's own
// in-memory queue shallow (at most one job per intake worker), so the durable
// table — not a Go slice — is where a backlog accumulates.
func (d *EmbeddingDispatcher) runGenerate(
	input embeddingInput, teamID string, resolved *ResolvedEmbeddingProvider, wait bool,
) generateOutcome {
	concurrency := resolved.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}

	// The submitted half of the submitted/terminal pair (#755), emitted BEFORE the
	// enqueue: submit() signals a worker that can run the job to completion while
	// this goroutine is still between the unlock and its own log call, which would
	// order a terminal line ahead of the submitted line it closes. It stays outside
	// the mutex on purpose — logging under d.mu would serialize every team's
	// submissions behind one lock on the hot path.
	d.stats.submitted.Add(1)
	d.entityLogger(input, teamID).Info("Embedding job submitted")

	var outcome generateOutcome
	done := make(chan struct{})

	d.mu.Lock()
	pe := d.execs[teamID]
	if pe == nil || pe.concurrency != concurrency {
		if pe != nil {
			go pe.exec.close() // drain the old executor, then let its workers exit
		}
		pe = &providerExecutor{concurrency: concurrency, exec: newBoundedExecutor(concurrency)}
		d.execs[teamID] = pe
	}
	ok := pe.exec.submit(func() {
		defer close(done)
		outcome = d.generate(input, teamID, resolved)
	})
	d.mu.Unlock()

	if !ok {
		// Unreachable in practice: the executor was just fetched or created live
		// under the lock, and an executor is only closed after being replaced in
		// the map. Log rather than drop, so a regression never goes silent. This
		// counts as the job's terminal — the entity was announced as submitted and
		// will never be generated — so the ledger stays balanced.
		d.stats.failed.Add(1)
		d.entityLogger(input, teamID).Error("Embedding job not enqueued; entity left unembedded")
		return generateOutcome{
			err:       fmt.Errorf("provider executor rejected the job"),
			retryable: true,
		}
	}

	if !wait {
		return generateOutcome{}
	}
	<-done
	return outcome
}

// entityLogger builds the dispatcher's standard structured logger for one entity,
// so every line in the submitted/terminal ledger carries an identical field set and
// can be diffed on entity_id alone.
func (d *EmbeddingDispatcher) entityLogger(input embeddingInput, teamID string) *slog.Logger {
	return d.logger.With(
		"service", "embedding",
		"component", logComponentEmbeddingDispatcher,
		"entity_type", input.entityType,
		"entity_id", input.entityID,
		"team_id", teamID,
	)
}

// generate runs one entity's embedding with bounded retry and emits exactly one
// terminal line — INFO on success, ERROR when every attempt fails (never a silent
// drop) — closing the pair opened by submitGenerate's submitted line. The success
// line is unconditional rather than retry-only: the dispatcher's own ledger has to
// account for every submitted job, and a clean first-attempt success is otherwise
// logged only by the processor, one layer down. The context is dispatcher-owned,
// not the event's, since the event context may be cancelled well before this runs.
// It returns the terminal outcome so the durable path (#820) can ack, release or
// fail the queue row accordingly; the in-memory path ignores it.
func (d *EmbeddingDispatcher) generate(
	input embeddingInput, teamID string, resolved *ResolvedEmbeddingProvider,
) generateOutcome {
	ctx, cancel := context.WithTimeout(context.Background(), embeddingJobTimeout)
	defer cancel()

	bo := d.newBackoff()

	var lastErr error
	attempts := 0
	retryable := true
	for attempt := 0; attempt < d.retry.MaxRetries; attempt++ {
		if attempt > 0 && !sleepCtx(ctx, bo.NextBackOff()) {
			lastErr = ctx.Err()
			break
		}

		attempts = attempt + 1
		lastErr = d.engine.generateAndSave(ctx, input, teamID, resolved)
		if lastErr == nil {
			d.stats.succeeded.Add(1)
			d.entityLogger(input, teamID).
				With("attempts", attempts).
				Info("Embedding job completed")
			return generateOutcome{}
		}

		// A provider that rejected the request itself (4xx other than 408/429) will
		// reject an identical retry, so retrying only burns two more attempts plus
		// backoff per entity on every reprocess — and buries the real reason under a
		// misleading attempts=MaxRetries (#756).
		if isPermanentProviderError(lastErr) {
			retryable = false
			break
		}

		// The provider call already bounds itself with its own HTTP timeout; a
		// cancelled dispatcher context is terminal, so stop retrying.
		if ctx.Err() != nil {
			break
		}
	}

	d.stats.failed.Add(1)
	d.entityLogger(input, teamID).With(
		"attempts", attempts,
		"retryable", retryable,
		"error", fmt.Sprintf("%+v", lastErr),
	).Error(terminalFailureMessage(retryable))

	return generateOutcome{err: lastErr, retryable: retryable}
}

// generateOutcome is one generate job's terminal result. A zero value means
// success. retryable distinguishes "the provider rejected this request and will
// reject an identical retry" (#756) from an exhausted-retries blip, which is what
// decides whether a durable job is failed terminally or released to be retried.
type generateOutcome struct {
	err       error
	retryable bool
}

// isPermanentProviderError reports whether err is a provider rejection that an
// identical retry cannot fix. generateAndSave wraps with %w, so errors.As reaches
// the provider error through the wrapping.
func isPermanentProviderError(err error) bool {
	var providerErr *providerHTTPError
	return errors.As(err, &providerErr) && providerErr.Permanent()
}

// terminalFailureMessage names the two failure outcomes distinctly, so a permanent
// rejection is not read as an exhausted-retries blip. Both keep the
// "entity left unembedded" suffix that existing operator greps match on.
func terminalFailureMessage(retryable bool) string {
	if retryable {
		return "Embedding generation failed after all retries; entity left unembedded"
	}
	return "Embedding generation rejected by provider (not retryable); entity left unembedded"
}

// Stop drains and stops all workers: the queue poller first (so nothing new is
// claimed), then the resolve stage (so no new generate jobs are routed), then
// every per-provider generate executor. It is best-effort graceful shutdown and
// is idempotent.
//
// Anything still leased when the process goes away is NOT lost with a durable
// queue wired: its lease simply expires and the next poller — this process's or
// its successor's — reclaims it.
func (d *EmbeddingDispatcher) Stop() {
	d.stopOnce.Do(func() { close(d.stopCh) })
	d.pollerWG.Wait()

	d.intake.close()

	d.mu.Lock()
	execs := make([]*boundedExecutor, 0, len(d.execs))
	for _, pe := range d.execs {
		execs = append(execs, pe.exec)
	}
	d.execs = make(map[string]*providerExecutor)
	d.mu.Unlock()

	for _, e := range execs {
		e.close()
	}
}

// newBackoff builds the per-job exponential backoff, mirroring the event bus's
// policy (base interval doubled each attempt, capped, optional ±10% jitter). The
// instance is not thread-safe; each generate call gets its own.
func (d *EmbeddingDispatcher) newBackoff() *backoff.ExponentialBackOff {
	randomizationFactor := 0.0
	if d.retry.Jitter {
		randomizationFactor = 0.1
	}
	return &backoff.ExponentialBackOff{
		InitialInterval:     d.retry.BaseBackoff,
		RandomizationFactor: randomizationFactor,
		Multiplier:          2.0,
		MaxInterval:         maxEmbeddingRetryBackoff,
	}
}

// sleepCtx waits for d, returning false if the context is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
