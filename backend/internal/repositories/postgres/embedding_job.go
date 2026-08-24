package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// embeddingJobColumns is the canonical column list for embedding_jobs
// SELECT/RETURNING clauses; scanEmbeddingJob reads them in this order.
const embeddingJobColumns = `id, entity_type, entity_id, user_id, payload, state, attempts,
	available_at, claimed_by, claimed_at, lease_expires_at, last_error, created_at, updated_at`

// embeddingJobOutstanding is the predicate that defines an outstanding job. It
// is repeated verbatim in the partial indexes of migration 014 and in the
// ON CONFLICT target below -- Postgres infers a partial unique index only when
// the statement's predicate matches the index's, so these three spellings must
// stay identical.
const embeddingJobOutstanding = `state IN ('pending', 'claimed')`

// EmbeddingJobRepository implements repositories.EmbeddingJobRepository for
// PostgreSQL (issue #820).
type EmbeddingJobRepository struct {
	db *database.DB
}

// NewEmbeddingJobRepository creates a new EmbeddingJobRepository.
func NewEmbeddingJobRepository(db *database.DB) repositories.EmbeddingJobRepository {
	return &EmbeddingJobRepository{db: db}
}

// Enqueue durably records one unit of embedding work, coalescing onto the
// entity's existing outstanding job when there is one.
//
// The conflict target repeats the partial index's predicate so Postgres infers
// idx_embedding_jobs_outstanding_entity. On conflict the payload is refreshed
// (newest content wins) and the row is returned to pending with its claim
// cleared, which is what makes an in-flight worker's later Ack/Release/Fail --
// all of which match on claimed_by -- a no-op rather than a way to lose the
// newer text. attempts is deliberately NOT reset: a genuinely poisonous entity
// must not become immortal just because it keeps being updated.
func (r *EmbeddingJobRepository) Enqueue(ctx context.Context, job *models.EmbeddingJob) error {
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding job payload: %w", err)
	}

	query := `
		INSERT INTO embedding_jobs (entity_type, entity_id, user_id, payload)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (entity_type, entity_id) WHERE ` + embeddingJobOutstanding + ` DO UPDATE
		SET payload = EXCLUDED.payload,
		    user_id = EXCLUDED.user_id,
		    state = 'pending',
		    available_at = now(),
		    claimed_by = NULL,
		    claimed_at = NULL,
		    lease_expires_at = NULL,
		    updated_at = now()
		RETURNING ` + embeddingJobColumns

	row := r.db.QueryRowContext(ctx, query, job.EntityType, job.EntityID, job.UserID, payload)
	if err := scanEmbeddingJob(row, job); err != nil {
		return fmt.Errorf("failed to enqueue embedding job: %w", err)
	}
	return nil
}

// Claim leases up to limit claimable jobs for workerID.
//
// FOR UPDATE SKIP LOCKED on the inner SELECT is what makes concurrent claimers
// safe without serializing them: a row another transaction is already claiming
// is skipped rather than waited on. Claimability is evaluated entirely on the
// database clock (now()), so replicas agree regardless of app-server skew, and
// an expired lease is claimable -- which is the whole restart-recovery path.
//
// attempts is incremented HERE rather than on failure: a process that dies
// mid-flight never reaches a failure path, so a failure-time counter would let
// a job that crashes its worker retry forever.
func (r *EmbeddingJobRepository) Claim(
	ctx context.Context, workerID string, limit int, lease time.Duration, maxAttempts int,
) ([]*models.EmbeddingJob, error) {
	if limit < 1 {
		return nil, nil
	}

	query := `
		UPDATE embedding_jobs
		SET state = 'claimed',
		    attempts = attempts + 1,
		    claimed_by = $1,
		    claimed_at = now(),
		    lease_expires_at = now() + make_interval(secs => $2),
		    updated_at = now()
		WHERE id IN (
			SELECT id FROM embedding_jobs
			WHERE ` + embeddingJobOutstanding + `
			  AND available_at <= now()
			  AND attempts < $3
			  AND (state = 'pending' OR lease_expires_at < now())
			ORDER BY created_at, id
			LIMIT $4
			FOR UPDATE SKIP LOCKED
		)
		RETURNING ` + embeddingJobColumns

	rows, err := r.db.QueryContext(ctx, query, workerID, lease.Seconds(), maxAttempts, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to claim embedding jobs: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close embedding job rows", "error", closeErr)
		}
	}()

	var jobs []*models.EmbeddingJob
	for rows.Next() {
		job := &models.EmbeddingJob{}
		if err := scanEmbeddingJob(rows, job); err != nil {
			return nil, fmt.Errorf("failed to scan claimed embedding job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate claimed embedding jobs: %w", err)
	}
	return jobs, nil
}

// Ack marks a claimed job done.
func (r *EmbeddingJobRepository) Ack(ctx context.Context, id, workerID string) (bool, error) {
	return r.finish(ctx, id, workerID, `
		UPDATE embedding_jobs
		SET state = 'done',
		    last_error = NULL,
		    claimed_by = NULL,
		    lease_expires_at = NULL,
		    updated_at = now()
		WHERE id = $1 AND state = 'claimed' AND claimed_by = $2`, "ack embedding job")
}

// Release returns a claimed job to pending after a retryable failure, held back
// by backoff so a fast-failing job does not get re-claimed on every poll.
func (r *EmbeddingJobRepository) Release(
	ctx context.Context, id, workerID, lastErr string, backoff time.Duration,
) (bool, error) {
	if backoff < 0 {
		backoff = 0
	}

	query := `
		UPDATE embedding_jobs
		SET state = 'pending',
		    last_error = $3,
		    available_at = now() + make_interval(secs => $4),
		    claimed_by = NULL,
		    claimed_at = NULL,
		    lease_expires_at = NULL,
		    updated_at = now()
		WHERE id = $1 AND state = 'claimed' AND claimed_by = $2`

	res, err := r.db.ExecContext(ctx, query, id, workerID, lastErr, backoff.Seconds())
	if err != nil {
		return false, fmt.Errorf("failed to release embedding job: %w", err)
	}
	return rowsAffected(res, "release embedding job")
}

// Fail marks a claimed job terminally failed.
func (r *EmbeddingJobRepository) Fail(ctx context.Context, id, workerID, lastErr string) (bool, error) {
	query := `
		UPDATE embedding_jobs
		SET state = 'failed',
		    last_error = $3,
		    claimed_by = NULL,
		    lease_expires_at = NULL,
		    updated_at = now()
		WHERE id = $1 AND state = 'claimed' AND claimed_by = $2`

	res, err := r.db.ExecContext(ctx, query, id, workerID, lastErr)
	if err != nil {
		return false, fmt.Errorf("failed to fail embedding job: %w", err)
	}
	return rowsAffected(res, "fail embedding job")
}

// FailExhausted retires every outstanding job that has used up its attempts and
// is not under a live lease. Claim filters on the same bound, so without this
// sweep such a row would sit outstanding forever and, through the coalescing
// unique index, keep the entity from ever being queued again.
func (r *EmbeddingJobRepository) FailExhausted(ctx context.Context, maxAttempts int) (int64, error) {
	query := `
		UPDATE embedding_jobs
		SET state = 'failed',
		    last_error = COALESCE(last_error, 'embedding job exhausted its attempt bound'),
		    claimed_by = NULL,
		    lease_expires_at = NULL,
		    updated_at = now()
		WHERE ` + embeddingJobOutstanding + `
		  AND attempts >= $1
		  AND (state = 'pending' OR lease_expires_at < now())`

	res, err := r.db.ExecContext(ctx, query, maxAttempts)
	if err != nil {
		return 0, fmt.Errorf("failed to retire exhausted embedding jobs: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read retired embedding job count: %w", err)
	}
	return n, nil
}

// CountByState returns the per-state job counts, sparse.
func (r *EmbeddingJobRepository) CountByState(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT state, COUNT(*) FROM embedding_jobs GROUP BY state`)
	if err != nil {
		return nil, fmt.Errorf("failed to count embedding jobs by state: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close embedding job rows", "error", closeErr)
		}
	}()

	counts := make(map[string]int64)
	for rows.Next() {
		var state string
		var n int64
		if err := rows.Scan(&state, &n); err != nil {
			return nil, fmt.Errorf("failed to scan embedding job state count: %w", err)
		}
		counts[state] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate embedding job state counts: %w", err)
	}
	return counts, nil
}

// finish runs one claimed-job terminal UPDATE and reports whether it matched.
func (r *EmbeddingJobRepository) finish(ctx context.Context, id, workerID, query, what string) (bool, error) {
	res, err := r.db.ExecContext(ctx, query, id, workerID)
	if err != nil {
		return false, fmt.Errorf("failed to %s: %w", what, err)
	}
	return rowsAffected(res, what)
}

// rowsAffected reports whether an UPDATE matched a row. A zero count is not an
// error here: every claimed-job write matches on claimed_by, and "no row
// matched" is the meaningful answer that the worker's claim is no longer valid.
func rowsAffected(res sql.Result, what string) (bool, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read %s result: %w", what, err)
	}
	return n > 0, nil
}

// scanEmbeddingJob reads embeddingJobColumns, in that order, into job. The
// payload column is decoded from its JSON representation here rather than being
// carried as raw bytes on the model, so callers never handle the encoding.
func scanEmbeddingJob(row rowScanner, job *models.EmbeddingJob) error {
	var payload []byte
	if err := row.Scan(
		&job.ID,
		&job.EntityType,
		&job.EntityID,
		&job.UserID,
		&payload,
		&job.State,
		&job.Attempts,
		&job.AvailableAt,
		&job.ClaimedBy,
		&job.ClaimedAt,
		&job.LeaseExpiresAt,
		&job.LastError,
		&job.CreatedAt,
		&job.UpdatedAt,
	); err != nil {
		return err
	}
	if len(payload) == 0 {
		job.Payload = models.EmbeddingJobPayload{}
		return nil
	}
	if err := json.Unmarshal(payload, &job.Payload); err != nil {
		return fmt.Errorf("failed to decode embedding job payload: %w", err)
	}
	return nil
}
