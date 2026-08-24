package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// Durable embedding queue defaults. They are the values used when a knob is left
// unset, and they mirror config's defaults() so a dispatcher built in a test
// behaves like the wired one.
const (
	// defaultEmbeddingQueueLease is how long a claim is held before the job
	// becomes claimable again. It has to comfortably exceed the worst-case time
	// one claimed job spends in this process -- the wait for a free provider slot
	// plus embeddingJobTimeout -- because a lease that expires while the job is
	// still running is re-run by the next poll. That costs a duplicate embed,
	// which is wasted work rather than wrong data (generation is
	// delete-then-insert per entity), but it is worth not doing routinely.
	defaultEmbeddingQueueLease = 30 * time.Minute
	// defaultEmbeddingQueueMaxAttempts bounds how many times one job may be
	// claimed before it is retired as a poison pill.
	defaultEmbeddingQueueMaxAttempts = 5
	// defaultEmbeddingQueueBatchSize caps how many jobs one claim leases.
	defaultEmbeddingQueueBatchSize = 20
	// defaultEmbeddingQueuePollInterval is the floor on how often the queue is
	// swept. The common path does not wait for it -- an enqueue wakes the poller
	// immediately -- so this interval exists for the work no enqueue announces:
	// jobs left behind by a dead process, and jobs released to back off.
	defaultEmbeddingQueuePollInterval = 30 * time.Second
	// defaultEmbeddingQueueRetryBackoff holds a released job back before it may be
	// claimed again, so a job that fails fast does not consume a claim slot on
	// every single poll.
	defaultEmbeddingQueueRetryBackoff = 2 * time.Minute
)

// EmbeddingQueueConfig tunes the durable embedding job queue (#820). A zero
// value is valid: withDefaults fills every knob.
type EmbeddingQueueConfig struct {
	// LeaseDuration is how long a claimed job is held before it is reclaimable.
	LeaseDuration time.Duration
	// MaxAttempts bounds claims per job. Attempts are counted at CLAIM time, so a
	// job that kills its worker still converges on this bound.
	MaxAttempts int
	// BatchSize caps how many jobs one claim leases.
	BatchSize int
	// PollInterval is how often the queue is swept for work no enqueue announced.
	PollInterval time.Duration
	// RetryBackoff holds a released job back before it may be claimed again.
	RetryBackoff time.Duration
}

// withDefaults returns c with every unset knob replaced by its default.
func (c EmbeddingQueueConfig) withDefaults() EmbeddingQueueConfig {
	if c.LeaseDuration <= 0 {
		c.LeaseDuration = defaultEmbeddingQueueLease
	}
	if c.MaxAttempts < 1 {
		c.MaxAttempts = defaultEmbeddingQueueMaxAttempts
	}
	if c.BatchSize < 1 {
		c.BatchSize = defaultEmbeddingQueueBatchSize
	}
	if c.PollInterval <= 0 {
		c.PollInterval = defaultEmbeddingQueuePollInterval
	}
	if c.RetryBackoff < 0 {
		c.RetryBackoff = defaultEmbeddingQueueRetryBackoff
	}
	return c
}

// EmbeddingDispatcherOption configures an EmbeddingDispatcher at construction.
type EmbeddingDispatcherOption func(*EmbeddingDispatcher)

// WithDurableQueue makes the dispatcher's backlog durable (#820): accepted
// events are written to embedding_jobs before ProcessEvent returns, and a poller
// leases them back out. Without it the dispatcher keeps its pre-#820 in-memory
// backlog, which a restart discards.
func WithDurableQueue(
	queue repositories.EmbeddingJobRepository, cfg EmbeddingQueueConfig,
) EmbeddingDispatcherOption {
	return func(d *EmbeddingDispatcher) {
		d.queue = queue
		d.queueCfg = cfg
	}
}

// enqueueDurably persists one accepted event as an embedding job and nudges the
// poller. An event with nothing to embed is dropped here, before any database
// call: the extraction is pure, so the cheap check happens first.
func (d *EmbeddingDispatcher) enqueueDurably(ctx context.Context, event events.Event) error {
	input, ok := embeddableInput(event)
	if !ok {
		return nil
	}

	job := &models.EmbeddingJob{
		EntityType: input.entityType,
		EntityID:   input.entityID,
		UserID:     input.userID,
		Payload: models.EmbeddingJobPayload{
			Title:       input.title,
			Description: input.description,
			Body:        input.body,
		},
	}
	if err := d.queue.Enqueue(ctx, job); err != nil {
		// Returning the error is what makes the loss visible: the event bus logs
		// and retries a failed handler, and the caller sees a publish that did not
		// take. Swallowing it here would be the silent drop this issue exists to
		// remove, just one layer up.
		return fmt.Errorf("failed to durably enqueue embedding job: %w", err)
	}

	d.entityLogger(input, "").With("job_id", job.ID).Debug("Embedding job enqueued durably")
	d.signalPoller()
	return nil
}

// signalPoller wakes the poller without blocking. The channel is size 1 and a
// full buffer already means "there is work to look at", so a dropped signal
// loses nothing.
func (d *EmbeddingDispatcher) signalPoller() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// pollQueue is the durable queue's run loop: sweep on every enqueue signal and,
// failing that, on a timer. The FIRST sweep happens at startup, before the loop,
// which is the whole of restart recovery -- there is no separate boot-time
// orphan sweep because an expired lease is claimable by the ordinary claim query.
func (d *EmbeddingDispatcher) pollQueue() {
	defer d.pollerWG.Done()

	ticker := time.NewTicker(d.queueCfg.PollInterval)
	defer ticker.Stop()

	d.retireExhausted()
	d.drainQueue()
	for {
		select {
		case <-d.stopCh:
			return
		case <-d.wake:
		case <-ticker.C:
			// The poison-pill sweep rides the TIMER only, never the enqueue
			// signal. Its UPDATE has to consider the whole outstanding set, and a
			// backfill wakes this loop once per enqueue and once per completed
			// job -- running it there would make a scan of the backlog scale with
			// the backlog. Nothing is lost by the delay: an exhausted job is
			// already excluded from Claim, so it is retired late, not missed.
			d.retireExhausted()
		}
		d.drainQueue()
	}
}

// retireExhausted runs the poison-pill sweep, reporting what it retired. Such a
// job is one no retry can rescue -- typically an entity whose worker kept dying
// -- and its entity stays unembedded, so the line is an ERROR, not a statistic.
func (d *EmbeddingDispatcher) retireExhausted() {
	n, err := d.queue.FailExhausted(context.Background(), d.queueCfg.MaxAttempts)
	switch {
	case err != nil:
		d.queueLogger().With("error", fmt.Sprintf("%+v", err)).
			Error("Failed to retire exhausted embedding jobs")
	case n > 0:
		d.queueLogger().With("retired", n).
			Error("Embedding jobs exhausted their attempt bound; entities left unembedded")
	}
}

// drainQueue claims and dispatches work until the queue is empty or this
// process has no free intake worker. It never blocks on generation: each claimed
// job occupies one intake worker for its whole lifetime, and the loop stops
// claiming once they are all busy, so the durable table -- not a Go slice -- is
// where a backlog waits.
func (d *EmbeddingDispatcher) drainQueue() {
	ctx := context.Background()

	for {
		select {
		case <-d.stopCh:
			return
		default:
		}

		capacity := d.intakeWorkers - int(d.inFlight.Load())
		if capacity < 1 {
			return
		}
		limit := min(capacity, d.queueCfg.BatchSize)

		jobs, err := d.queue.Claim(
			ctx, d.workerID, limit, d.queueCfg.LeaseDuration, d.queueCfg.MaxAttempts,
		)
		if err != nil {
			d.queueLogger().With("error", fmt.Sprintf("%+v", err)).
				Error("Failed to claim embedding jobs")
			return
		}
		if len(jobs) == 0 {
			return
		}

		for _, job := range jobs {
			d.dispatchClaimed(ctx, job)
		}
	}
}

// dispatchClaimed hands one claimed job to an intake worker, or gives the claim
// straight back if the dispatcher is already stopping. Releasing rather than
// holding is what keeps a shutdown from parking rows under a lease nobody is
// working for its full duration.
func (d *EmbeddingDispatcher) dispatchClaimed(ctx context.Context, job *models.EmbeddingJob) {
	d.inFlight.Add(1)
	submitted := d.intake.submit(func() {
		defer func() {
			d.inFlight.Add(-1)
			// Freeing a worker is the other thing that makes more work claimable,
			// alongside an enqueue. Without this nudge a backlog larger than the
			// worker count stalls: the drain loop stops at zero capacity and
			// nothing but the poll timer would ever start it again.
			d.signalPoller()
		}()
		d.runQueuedJob(job)
	})
	if submitted {
		return
	}

	d.inFlight.Add(-1)
	if _, err := d.queue.Release(
		ctx, job.ID, d.workerID, "dispatcher stopped before the job ran", 0,
	); err != nil {
		d.jobLogger(job).With("error", fmt.Sprintf("%+v", err)).
			Error("Failed to release embedding job after dispatcher stop")
	}
}

// runQueuedJob resolves and generates one claimed job, then settles its row.
// It runs the same two stages a live event does, from the payload stored at
// enqueue time -- the event itself is long gone, and the entity may have changed
// since, so the stored text is what the pipeline promised to embed.
func (d *EmbeddingDispatcher) runQueuedJob(job *models.EmbeddingJob) {
	ctx, cancel := context.WithTimeout(context.Background(), embeddingJobTimeout)
	defer cancel()

	input := embeddingInput{
		entityType:  job.EntityType,
		entityID:    job.EntityID,
		userID:      job.UserID,
		title:       job.Payload.Title,
		description: job.Payload.Description,
		body:        job.Payload.Body,
	}

	teamID, resolved, err := d.engine.resolveInput(ctx, input)
	if err != nil {
		d.entityLogger(input, teamID).With(
			"job_id", job.ID,
			"attempts", job.Attempts,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to resolve embedding job; entity left unembedded")
		d.releaseJob(job, err.Error())
		return
	}
	if resolved == nil {
		// Not embeddable, or the team has no provider configured. There is nothing
		// to retry: acking rather than releasing is what stops such a job cycling
		// through its attempt bound and ending as a spurious poison pill.
		d.ackJob(job)
		return
	}

	// runGenerate owns the #755 submitted/terminal pair for both paths, so the
	// ledger is incremented exactly once per job however the job arrived.
	outcome := d.runGenerate(input, teamID, resolved, true)
	switch {
	case outcome.err == nil:
		d.ackJob(job)
	case outcome.retryable:
		d.releaseJob(job, outcome.err.Error())
	default:
		d.failJob(job, outcome.err.Error())
	}
}

// ackJob marks a job done. A claim that no longer matches is not an error: it
// means the entity was re-enqueued with newer text while this job ran, and that
// newer row is the one that must win.
func (d *EmbeddingDispatcher) ackJob(job *models.EmbeddingJob) {
	matched, err := d.queue.Ack(context.Background(), job.ID, d.workerID)
	d.logSettle(job, matched, err, "ack")
}

// releaseJob returns a job to pending after a retryable failure.
func (d *EmbeddingDispatcher) releaseJob(job *models.EmbeddingJob, lastErr string) {
	matched, err := d.queue.Release(
		context.Background(), job.ID, d.workerID, lastErr, d.queueCfg.RetryBackoff,
	)
	d.logSettle(job, matched, err, "release")
}

// failJob retires a job an identical retry cannot fix (#756).
func (d *EmbeddingDispatcher) failJob(job *models.EmbeddingJob, lastErr string) {
	matched, err := d.queue.Fail(context.Background(), job.ID, d.workerID, lastErr)
	d.logSettle(job, matched, err, "fail")
}

// logSettle reports the outcome of settling a job's row. A settle that errors is
// not silently swallowed: the job stays leased and will be reclaimed when the
// lease expires, which is correct but worth an operator seeing.
func (d *EmbeddingDispatcher) logSettle(job *models.EmbeddingJob, matched bool, err error, action string) {
	switch {
	case err != nil:
		d.jobLogger(job).With("error", fmt.Sprintf("%+v", err)).
			Error("Failed to " + action + " embedding job; it stays leased until the lease expires")
	case !matched:
		d.jobLogger(job).Info(
			"Embedding job claim no longer valid on " + action + "; the entity was re-enqueued while it ran",
		)
	}
}

// jobLogger builds the standard structured logger for one durable job.
func (d *EmbeddingDispatcher) jobLogger(job *models.EmbeddingJob) *slog.Logger {
	return d.logger.With(
		"service", "embedding",
		"component", logComponentEmbeddingDispatcher,
		"job_id", job.ID,
		"entity_type", job.EntityType,
		"entity_id", job.EntityID,
		"attempts", job.Attempts,
	)
}

// queueLogger builds the standard structured logger for queue-wide lines that
// name no single job.
func (d *EmbeddingDispatcher) queueLogger() *slog.Logger {
	return d.logger.With(
		"service", "embedding",
		"component", logComponentEmbeddingDispatcher,
		"worker_id", d.workerID,
	)
}
