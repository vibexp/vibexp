//go:build integration

package postgres

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// The durable embedding job queue (issue #820) is tested against real Postgres
// rather than sqlmock throughout, because every guarantee it makes is a
// guarantee of the DATABASE, not of the Go code: FOR UPDATE SKIP LOCKED under
// genuine concurrency, a partial unique index inferred by ON CONFLICT, and
// claimability computed on the server clock. sqlmock returns whatever a test
// declares, so it can only pin the query text -- it cannot fail when the
// semantics are wrong, which is the only failure mode worth catching here.

// resetEmbeddingJobs empties the queue table. embedding_jobs has no foreign
// keys (entity_id is polymorphic), so the shared resetIntegrationTables CASCADE
// does not reach it.
func resetEmbeddingJobs(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), "TRUNCATE TABLE embedding_jobs")
	require.NoError(t, err)
}

// newEmbeddingJob builds an enqueue-ready job for the given entity.
func newEmbeddingJob(entityType, entityID, body string) *models.EmbeddingJob {
	return &models.EmbeddingJob{
		EntityType: entityType,
		EntityID:   entityID,
		UserID:     "user-1",
		Payload:    models.EmbeddingJobPayload{Title: "t", Description: "d", Body: body},
	}
}

// enqueueEmbeddingJob enqueues and returns the stored row.
func enqueueEmbeddingJob(t *testing.T, repo repositories.EmbeddingJobRepository, job *models.EmbeddingJob) *models.EmbeddingJob {
	t.Helper()
	require.NoError(t, repo.Enqueue(context.Background(), job))
	return job
}

// expireLease backdates a claimed job's lease so the next claim treats it as
// orphaned -- the state a process that died mid-flight leaves behind. Doing it
// in SQL keeps the whole comparison on the database clock.
func expireLease(t *testing.T, id string) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"UPDATE embedding_jobs SET lease_expires_at = now() - interval '1 second' WHERE id = $1", id)
	require.NoError(t, err)
}

// readEmbeddingJobState returns one job's state and attempt count.
func readEmbeddingJobState(t *testing.T, id string) (string, int) {
	t.Helper()
	var state string
	var attempts int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT state, attempts FROM embedding_jobs WHERE id = $1", id).Scan(&state, &attempts))
	return state, attempts
}

func TestIntegrationEmbeddingJob_EnqueueCoalescesOutstandingEntity(t *testing.T) {
	resetEmbeddingJobs(t)
	repo := NewEmbeddingJobRepository(integrationDB)
	ctx := context.Background()

	first := enqueueEmbeddingJob(t, repo, newEmbeddingJob("memory", "m1", "original"))
	require.Equal(t, models.EmbeddingJobStatePending, first.State)

	// A second enqueue of the same entity while the first is still outstanding
	// must land on the SAME row with the NEWER text. This is what proves the
	// ON CONFLICT target infers the partial unique index: without inference
	// Postgres rejects the statement outright.
	second := enqueueEmbeddingJob(t, repo, newEmbeddingJob("memory", "m1", "updated"))
	assert.Equal(t, first.ID, second.ID, "a re-enqueue must coalesce, not queue the entity twice")
	assert.Equal(t, "updated", second.Payload.Body, "the newest content must win")

	counts, err := repo.CountByState(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]int64{models.EmbeddingJobStatePending: 1}, counts)

	// A DIFFERENT entity is a different job even with identical text, and the
	// same entity queues again once its previous job is terminal.
	other := enqueueEmbeddingJob(t, repo, newEmbeddingJob("memory", "m2", "updated"))
	assert.NotEqual(t, first.ID, other.ID)
}

func TestIntegrationEmbeddingJob_EnqueueAfterTerminalCreatesNewRow(t *testing.T) {
	resetEmbeddingJobs(t)
	repo := NewEmbeddingJobRepository(integrationDB)
	ctx := context.Background()

	job := enqueueEmbeddingJob(t, repo, newEmbeddingJob("prompt", "p1", "body"))
	claimed, err := repo.Claim(ctx, "worker-a", 10, time.Minute, 5)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	acked, err := repo.Ack(ctx, claimed[0].ID, "worker-a")
	require.NoError(t, err)
	require.True(t, acked)

	// The coalescing index covers only pending/claimed rows, so an entity that
	// was embedded once must be embeddable again when it is next updated.
	again := enqueueEmbeddingJob(t, repo, newEmbeddingJob("prompt", "p1", "new body"))
	assert.NotEqual(t, job.ID, again.ID, "a terminal job must not block the entity from queueing again")

	counts, err := repo.CountByState(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), counts[models.EmbeddingJobStateDone])
	assert.Equal(t, int64(1), counts[models.EmbeddingJobStatePending])
}

// TestIntegrationEmbeddingJob_ConcurrentClaimersNeverShareAJob is the
// FOR UPDATE SKIP LOCKED proof. It has to run real, simultaneous claims against
// one database: a mock cannot distinguish a correct SKIP LOCKED claim from one
// that would hand the same row to two workers, and neither can a sequential
// test, because sequential claims are safe even without the lock clause.
func TestIntegrationEmbeddingJob_ConcurrentClaimersNeverShareAJob(t *testing.T) {
	resetEmbeddingJobs(t)
	repo := NewEmbeddingJobRepository(integrationDB)
	ctx := context.Background()

	const jobCount = 40
	for i := 0; i < jobCount; i++ {
		enqueueEmbeddingJob(t, repo, newEmbeddingJob("memory", fmt.Sprintf("c%d", i), "body"))
	}

	const claimers = 8
	var (
		start sync.WaitGroup
		done  sync.WaitGroup
		mu    sync.Mutex
	)
	claimedBy := make(map[string]string, jobCount)
	start.Add(1)
	for i := 0; i < claimers; i++ {
		worker := fmt.Sprintf("worker-%d", i)
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			for {
				jobs, err := repo.Claim(ctx, worker, 5, time.Minute, 5)
				if err != nil || len(jobs) == 0 {
					assert.NoError(t, err)
					return
				}
				mu.Lock()
				for _, job := range jobs {
					if prev, seen := claimedBy[job.ID]; seen {
						assert.Failf(t, "job claimed twice",
							"job %s claimed by both %s and %s", job.ID, prev, worker)
					}
					claimedBy[job.ID] = worker
				}
				mu.Unlock()
			}
		}()
	}
	start.Done()
	done.Wait()

	assert.Len(t, claimedBy, jobCount, "every job must be claimed exactly once")

	counts, err := repo.CountByState(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(jobCount), counts[models.EmbeddingJobStateClaimed])
}

// TestIntegrationEmbeddingJob_ExpiredLeaseIsReclaimed is the headline
// crash-recovery guarantee, expressed the way a dead process expresses it: a job
// is leased and NEVER acked. No cooperation from the first worker is involved.
func TestIntegrationEmbeddingJob_ExpiredLeaseIsReclaimed(t *testing.T) {
	resetEmbeddingJobs(t)
	repo := NewEmbeddingJobRepository(integrationDB)
	ctx := context.Background()

	job := enqueueEmbeddingJob(t, repo, newEmbeddingJob("artifact", "a1", "body"))

	claimed, err := repo.Claim(ctx, "dead-worker", 10, time.Hour, 5)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, 1, claimed[0].Attempts, "attempts must increment at CLAIM time")

	// While the lease is live the job is invisible to everyone else.
	none, err := repo.Claim(ctx, "live-worker", 10, time.Hour, 5)
	require.NoError(t, err)
	assert.Empty(t, none, "a live lease must not be reclaimable")

	expireLease(t, job.ID)

	reclaimed, err := repo.Claim(ctx, "live-worker", 10, time.Hour, 5)
	require.NoError(t, err)
	require.Len(t, reclaimed, 1, "an expired lease must become claimable again")
	assert.Equal(t, job.ID, reclaimed[0].ID)
	assert.Equal(t, 2, reclaimed[0].Attempts)
	require.NotNil(t, reclaimed[0].ClaimedBy)
	assert.Equal(t, "live-worker", *reclaimed[0].ClaimedBy)

	// The dead worker's claim is void: were it ever to come back, it must not be
	// able to report the job done.
	acked, err := repo.Ack(ctx, job.ID, "dead-worker")
	require.NoError(t, err)
	assert.False(t, acked, "a stolen claim must not be ackable by its previous holder")
	state, _ := readEmbeddingJobState(t, job.ID)
	assert.Equal(t, models.EmbeddingJobStateClaimed, state)
}

func TestIntegrationEmbeddingJob_ReleaseHoldsTheJobBackThenRetries(t *testing.T) {
	resetEmbeddingJobs(t)
	repo := NewEmbeddingJobRepository(integrationDB)
	ctx := context.Background()

	job := enqueueEmbeddingJob(t, repo, newEmbeddingJob("blueprint", "b1", "body"))
	claimed, err := repo.Claim(ctx, "worker-a", 10, time.Minute, 5)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	released, err := repo.Release(ctx, job.ID, "worker-a", "provider timed out", time.Hour)
	require.NoError(t, err)
	require.True(t, released)

	state, attempts := readEmbeddingJobState(t, job.ID)
	assert.Equal(t, models.EmbeddingJobStatePending, state)
	assert.Equal(t, 1, attempts, "releasing must not undo the attempt the claim counted")

	// The backoff is what stops a fast-failing job consuming a claim slot on
	// every poll, so a released job must be invisible until it elapses.
	none, err := repo.Claim(ctx, "worker-b", 10, time.Minute, 5)
	require.NoError(t, err)
	assert.Empty(t, none, "a released job must be held back for its backoff")

	_, err = integrationDB.ExecContext(ctx,
		"UPDATE embedding_jobs SET available_at = now() - interval '1 second' WHERE id = $1", job.ID)
	require.NoError(t, err)

	retried, err := repo.Claim(ctx, "worker-b", 10, time.Minute, 5)
	require.NoError(t, err)
	require.Len(t, retried, 1)
	assert.Equal(t, 2, retried[0].Attempts)
}

func TestIntegrationEmbeddingJob_FailIsTerminalAndNotReclaimed(t *testing.T) {
	resetEmbeddingJobs(t)
	repo := NewEmbeddingJobRepository(integrationDB)
	ctx := context.Background()

	job := enqueueEmbeddingJob(t, repo, newEmbeddingJob("memory", "f1", "body"))
	claimed, err := repo.Claim(ctx, "worker-a", 10, time.Minute, 5)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	failed, err := repo.Fail(ctx, job.ID, "worker-a", "provider rejected the request")
	require.NoError(t, err)
	require.True(t, failed)

	none, err := repo.Claim(ctx, "worker-b", 10, time.Minute, 5)
	require.NoError(t, err)
	assert.Empty(t, none, "a terminally failed job must never be reclaimed")

	var lastErr *string
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT last_error FROM embedding_jobs WHERE id = $1", job.ID).Scan(&lastErr))
	require.NotNil(t, lastErr)
	assert.Contains(t, *lastErr, "provider rejected")
}

// TestIntegrationEmbeddingJob_PoisonPillIsRetiredAtTheAttemptBound is the
// poison-pill guard: a job whose worker keeps dying (so it never reaches a
// failure path) must still stop being reclaimed. Attempts are counted at claim
// time precisely so that this converges.
func TestIntegrationEmbeddingJob_PoisonPillIsRetiredAtTheAttemptBound(t *testing.T) {
	resetEmbeddingJobs(t)
	repo := NewEmbeddingJobRepository(integrationDB)
	ctx := context.Background()

	const maxAttempts = 3
	job := enqueueEmbeddingJob(t, repo, newEmbeddingJob("memory", "poison", "body"))

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		claimed, err := repo.Claim(ctx, "crashy-worker", 10, time.Hour, maxAttempts)
		require.NoError(t, err, "attempt %d", attempt)
		require.Len(t, claimed, 1, "attempt %d must still be claimable", attempt)
		assert.Equal(t, attempt, claimed[0].Attempts)
		expireLease(t, job.ID) // the worker died; nobody acked or failed it
	}

	none, err := repo.Claim(ctx, "crashy-worker", 10, time.Hour, maxAttempts)
	require.NoError(t, err)
	assert.Empty(t, none, "a job at its attempt bound must not be claimed again")

	retired, err := repo.FailExhausted(ctx, maxAttempts)
	require.NoError(t, err)
	assert.Equal(t, int64(1), retired)

	state, attempts := readEmbeddingJobState(t, job.ID)
	assert.Equal(t, models.EmbeddingJobStateFailed, state)
	assert.Equal(t, maxAttempts, attempts)

	// Retiring it also frees the entity: the coalescing index covers outstanding
	// rows only, so a later update of the same entity queues normally.
	fresh := enqueueEmbeddingJob(t, repo, newEmbeddingJob("memory", "poison", "fixed"))
	assert.NotEqual(t, job.ID, fresh.ID)
}

// TestIntegrationEmbeddingJob_FailExhaustedSparesALiveLease guards the obvious
// way to get the sweep wrong: retiring a job that is at its bound but STILL
// RUNNING would abandon work in flight and, worse, let the entity be queued
// again underneath the worker.
func TestIntegrationEmbeddingJob_FailExhaustedSparesALiveLease(t *testing.T) {
	resetEmbeddingJobs(t)
	repo := NewEmbeddingJobRepository(integrationDB)
	ctx := context.Background()

	job := enqueueEmbeddingJob(t, repo, newEmbeddingJob("memory", "live", "body"))
	claimed, err := repo.Claim(ctx, "worker-a", 10, time.Hour, 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, 1, claimed[0].Attempts) // already at the bound of 1

	retired, err := repo.FailExhausted(ctx, 1)
	require.NoError(t, err)
	assert.Zero(t, retired, "a job still under a live lease must be left alone")

	state, _ := readEmbeddingJobState(t, job.ID)
	assert.Equal(t, models.EmbeddingJobStateClaimed, state)

	acked, err := repo.Ack(ctx, job.ID, "worker-a")
	require.NoError(t, err)
	assert.True(t, acked, "the running worker must still be able to finish")
}

// TestIntegrationEmbeddingJob_ReEnqueueDuringFlightWinsOverTheRunningWorker
// pins the coalescing rule that makes an update-while-embedding safe: the
// in-flight worker is embedding text that is now stale, so its terminal write
// must lose and the refreshed job must run again.
func TestIntegrationEmbeddingJob_ReEnqueueDuringFlightWinsOverTheRunningWorker(t *testing.T) {
	resetEmbeddingJobs(t)
	repo := NewEmbeddingJobRepository(integrationDB)
	ctx := context.Background()

	job := enqueueEmbeddingJob(t, repo, newEmbeddingJob("memory", "hot", "v1"))
	claimed, err := repo.Claim(ctx, "worker-a", 10, time.Hour, 5)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	updated := enqueueEmbeddingJob(t, repo, newEmbeddingJob("memory", "hot", "v2"))
	require.Equal(t, job.ID, updated.ID)

	acked, err := repo.Ack(ctx, job.ID, "worker-a")
	require.NoError(t, err)
	assert.False(t, acked, "the worker that embedded the stale text must not mark the job done")

	next, err := repo.Claim(ctx, "worker-b", 10, time.Hour, 5)
	require.NoError(t, err)
	require.Len(t, next, 1)
	assert.Equal(t, "v2", next[0].Payload.Body, "the refreshed payload must be what runs next")
}

// TestIntegrationEmbeddingJob_ClaimUsesTheClaimableIndex proves the claim query
// is index-driven. No behavioural test can catch a correct-but-seq-scanning
// claim, and this one runs on every poll of a table that accumulates a row per
// embedded entity.
func TestIntegrationEmbeddingJob_ClaimUsesTheClaimableIndex(t *testing.T) {
	resetEmbeddingJobs(t)
	repo := NewEmbeddingJobRepository(integrationDB)
	ctx := context.Background()

	for i := 0; i < 50; i++ {
		enqueueEmbeddingJob(t, repo, newEmbeddingJob("memory", fmt.Sprintf("idx%d", i), "body"))
	}
	_, err := integrationDB.ExecContext(ctx, "ANALYZE embedding_jobs")
	require.NoError(t, err)

	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off")
	require.NoError(t, err)

	rows, err := tx.QueryContext(ctx, `
		EXPLAIN SELECT id FROM embedding_jobs
		WHERE state IN ('pending', 'claimed')
		  AND available_at <= now()
		  AND attempts < 5
		  AND (state = 'pending' OR lease_expires_at < now())
		ORDER BY created_at, id
		LIMIT 20
		FOR UPDATE SKIP LOCKED`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	// Read EVERY plan line: an index scan can sit as a child of a bitmap or
	// sort node, so asserting on the first line alone would miss it.
	var plan strings.Builder
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan.WriteString(line)
		plan.WriteString("\n")
	}
	require.NoError(t, rows.Err())

	assert.Contains(t, plan.String(), "idx_embedding_jobs_claimable",
		"the claim query must be served by the claimable index, not a scan of the whole queue")
	// The index is keyed on the ORDER BY for a reason: a plan that sorts has read
	// the WHOLE claimable set before honouring the LIMIT, which is precisely the
	// cost a queue poll must not pay as the backlog grows.
	assert.NotContains(t, plan.String(), "Sort  (",
		"the claim query must walk the index in order, not sort the claimable set")
}
