package services

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
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
type EmbeddingDispatcher struct {
	engine *EmbeddingGenerationProcessor
	retry  EmbeddingRetryConfig
	logger *slog.Logger

	intake *boundedExecutor

	mu    sync.Mutex
	execs map[string]*providerExecutor
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
		// ERROR — silently swallowing it would be exactly the invisible drop #142
		// exists to eliminate.
		d.logger.With(
			"service", "embedding",
			"component", "embedding-dispatcher",
			"event_type", event.Type(),
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to resolve embedding job; entity left unembedded")
		return
	}
	if resolved == nil {
		return // not embeddable, no text, or no provider configured (engine logs)
	}

	d.executorFor(teamID, resolved.Concurrency).submit(func() {
		d.generate(input, teamID, resolved)
	})
}

// executorFor returns the generate executor for a team's active provider,
// creating it (sized to concurrency) on first use. If the provider's concurrency
// has changed since the executor was built, the old one is retired in the
// background (it drains what it already holds) and a new one is created at the new
// size — so a rare admin concurrency change is picked up without a restart.
func (d *EmbeddingDispatcher) executorFor(teamID string, concurrency int) *boundedExecutor {
	if concurrency < 1 {
		concurrency = 1
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if pe := d.execs[teamID]; pe != nil {
		if pe.concurrency == concurrency {
			return pe.exec
		}
		go pe.exec.close() // drain the old executor, then let its workers exit
	}

	exec := newBoundedExecutor(concurrency)
	d.execs[teamID] = &providerExecutor{concurrency: concurrency, exec: exec}
	return exec
}

// generate runs one entity's embedding with bounded retry. On success after a
// retry it logs at INFO; when every attempt fails it logs at ERROR with the
// entity id and reason (never a silent drop). The context is dispatcher-owned, not
// the event's, since the event context may be cancelled well before this runs.
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
			if attempt > 0 {
				d.logger.With(
					"service", "embedding",
					"component", "embedding-dispatcher",
					"entity_type", input.entityType,
					"entity_id", input.entityID,
					"team_id", teamID,
					"attempt", attempt+1,
				).Info("Embeddings generated after retry")
			}
			return
		}

		// The provider call already bounds itself with its own HTTP timeout; a
		// cancelled dispatcher context is terminal, so stop retrying.
		if ctx.Err() != nil {
			break
		}
	}

	d.logger.With(
		"service", "embedding",
		"component", "embedding-dispatcher",
		"entity_type", input.entityType,
		"entity_id", input.entityID,
		"team_id", teamID,
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
