package services

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v5"

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
// (#755). The pair is what makes a loss between the two — the in-memory queue is not
// durable, so a restart discards whatever it still holds (#820) — diffable from the
// logs alone: an entity with a submitted line and no terminal line is work that was
// accepted and never finished.
type EmbeddingDispatcher struct {
	engine *EmbeddingGenerationProcessor
	retry  EmbeddingRetryConfig
	logger *slog.Logger

	intake *boundedExecutor

	stats dispatcherStats

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
func NewEmbeddingDispatcher(
	engine *EmbeddingGenerationProcessor,
	resolveWorkers int,
	retry EmbeddingRetryConfig,
	logger *slog.Logger,
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
		engine: engine,
		retry:  retry,
		logger: logger,
		execs:  make(map[string]*providerExecutor),
	}
	d.intake = newBoundedExecutor(resolveWorkers)
	return d
}

// ManagesOwnConcurrency reports that generation runs on this dispatcher's own
// bounded, per-provider workers, so the event bus invokes the embedding worker
// inline instead of through the shared worker pool.
func (d *EmbeddingDispatcher) ManagesOwnConcurrency() bool { return true }

// ProcessEvent enqueues the event for asynchronous, concurrency-bounded
// embedding. It performs no I/O and never blocks, so it is safe to call on the
// bus dispatch goroutine. It returns an error only if the dispatcher is stopped.
func (d *EmbeddingDispatcher) ProcessEvent(_ context.Context, event events.Event) error {
	if !d.intake.submit(func() { d.resolveAndRoute(event) }) {
		return fmt.Errorf("embedding dispatcher is stopped")
	}
	return nil
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

// submitGenerate enqueues an entity's generate job onto its provider's executor,
// creating that executor (sized to concurrency) on first use. If the provider's
// concurrency has changed since the executor was built, the old one is retired in
// the background — it drains what it already holds — and a new one is created at
// the new size, so a rare admin concurrency change is picked up without a
// restart. The get-or-create and the submit happen under one lock so a concurrent
// concurrency change can never close the executor between the two and drop the
// job.
func (d *EmbeddingDispatcher) submitGenerate(input embeddingInput, teamID string, resolved *ResolvedEmbeddingProvider) {
	concurrency := resolved.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}

	d.mu.Lock()
	pe := d.execs[teamID]
	if pe == nil || pe.concurrency != concurrency {
		if pe != nil {
			go pe.exec.close() // drain the old executor, then let its workers exit
		}
		pe = &providerExecutor{concurrency: concurrency, exec: newBoundedExecutor(concurrency)}
		d.execs[teamID] = pe
	}
	ok := pe.exec.submit(func() { d.generate(input, teamID, resolved) })
	d.mu.Unlock()

	if !ok {
		// Unreachable in practice: the executor was just fetched or created live
		// under the lock, and an executor is only closed after being replaced in
		// the map. Log rather than drop, so a regression never goes silent. This is
		// a non-submit, so it is deliberately outside the submitted/terminal
		// ledger — it is already its own terminal, entity-named ERROR.
		d.entityLogger(input, teamID).Error("Embedding job not enqueued; entity left unembedded")
		return
	}

	// The submitted half of the submitted/terminal pair (#755). It is emitted after
	// the enqueue succeeded and before generation is attempted, so an entity that
	// reaches a provider executor and never produces a terminal line is visible as a
	// gap in the logs — the shape the in-memory queue's non-durability (#820) takes
	// when a restart discards queued work.
	d.stats.submitted.Add(1)
	d.entityLogger(input, teamID).Info("Embedding job submitted")
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
func (d *EmbeddingDispatcher) generate(input embeddingInput, teamID string, resolved *ResolvedEmbeddingProvider) {
	ctx, cancel := context.WithTimeout(context.Background(), embeddingJobTimeout)
	defer cancel()

	bo := d.newBackoff()

	var lastErr error
	for attempt := 0; attempt < d.retry.MaxRetries; attempt++ {
		if attempt > 0 && !sleepCtx(ctx, bo.NextBackOff()) {
			lastErr = ctx.Err()
			break
		}

		lastErr = d.engine.generateAndSave(ctx, input, teamID, resolved)
		if lastErr == nil {
			d.stats.succeeded.Add(1)
			d.entityLogger(input, teamID).
				With("attempts", attempt+1).
				Info("Embedding job completed")
			return
		}

		// The provider call already bounds itself with its own HTTP timeout; a
		// cancelled dispatcher context is terminal, so stop retrying.
		if ctx.Err() != nil {
			break
		}
	}

	d.stats.failed.Add(1)
	d.entityLogger(input, teamID).With(
		"attempts", d.retry.MaxRetries,
		"error", fmt.Sprintf("%+v", lastErr),
	).Error("Embedding generation failed after all retries; entity left unembedded")
}

// Stop drains and stops all workers: the resolve stage first (so no new generate
// jobs are routed), then every per-provider generate executor. It is best-effort
// graceful shutdown and is idempotent.
func (d *EmbeddingDispatcher) Stop() {
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
