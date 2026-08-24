package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// scheduleColumns is the canonical column list for schedule SELECT/RETURNING
// clauses; scanScheduleDest reads them in this order.
const scheduleColumns = "id, team_id, job_type, interval_seconds, last_run_at, next_run_at, created_at, updated_at"

// ScheduleRepository implements repositories.ScheduleRepository for PostgreSQL.
type ScheduleRepository struct {
	db *database.DB
}

// NewScheduleRepository creates a new ScheduleRepository.
func NewScheduleRepository(db *database.DB) repositories.ScheduleRepository {
	return &ScheduleRepository{db: db}
}

// Upsert inserts the schedule or, when (team_id, job_type) already exists,
// replaces its interval and next run time (and clears nothing else), relying
// on the (team_id, job_type) unique constraint. A zero NextRunAt defaults to
// the database clock (now()) so the first run is due immediately.
func (r *ScheduleRepository) Upsert(ctx context.Context, s *models.Schedule) error {
	query := `
		INSERT INTO schedules
			(team_id, job_type, interval_seconds, next_run_at)
		VALUES ($1, $2, $3, COALESCE($4, now()))
		ON CONFLICT (team_id, job_type) DO UPDATE
		SET interval_seconds = EXCLUDED.interval_seconds,
		    next_run_at = EXCLUDED.next_run_at,
		    updated_at = now()
		RETURNING ` + scheduleColumns

	var nextRunAt *time.Time
	if !s.NextRunAt.IsZero() {
		nextRunAt = &s.NextRunAt
	}

	if err := r.db.QueryRowContext(
		ctx, query, s.TeamID, s.JobType, s.IntervalSeconds, nextRunAt,
	).Scan(scanScheduleDest(s)...); err != nil {
		return fmt.Errorf("failed to upsert schedule: %w", err)
	}
	return nil
}

// ListDue returns schedules with next_run_at at or before the database clock
// (now()), most overdue first (id as deterministic tiebreaker), capped at
// limit. Due-ness uses the database clock so all replicas agree on what is due
// regardless of app-server clock skew.
//
// A schedule is also held back until at least interval_seconds have passed
// since its last run -- the RUN-SPACING floor (#767). Not to be confused with
// the storage floor on interval_seconds itself (CHECK >= 3600, migration 012):
// same column, different guarantee. That CHECK is also why this expression can
// never be zero or negative, so the spacing floor cannot silently become a
// no-op. next_run_at alone cannot be
// trusted as a rate limit: Upsert resets it to now() on every write, so a
// feature that syncs its schedule from a settings save (FreshnessService does)
// lets an admin keep one team permanently due just by saving repeatedly. tick
// runs due schedules SERIALLY, so that team would evaluate on every tick
// instead of on its own interval -- up to 1440x the intended cadence -- and
// delay every other team's jobs behind it.
//
// The floor lives here, in SQL, for three reasons: the comparison stays on the
// database clock rather than mixing in an app-server time.Now(); a held-back
// row is never fetched, so it cannot consume the caller's limit budget; and no
// writer can bypass it, including a future job that stamps next_run_at itself
// or a direct SQL write. It is derived from each schedule's own
// interval_seconds, so it needs no new configuration knob.
//
// last_run_at IS NULL means the schedule has never run, which is exempt: a
// newly provisioned schedule must still fire on the next tick. The cost is
// that a genuine configuration change made shortly after a run waits out the
// remainder of the interval, which is the cadence the interval already asks
// for.
//
// The floor keys on last_run_at, so it does not disturb the two paths that
// deliberately leave a schedule due: a failed MarkRun and a lock claimed by
// another replica (see Scheduler.runDue) both leave last_run_at untouched, so
// a row that qualified still qualifies on the next tick. Under normal
// operation the floor is redundant -- MarkRun sets last_run_at = now() and
// next_run_at = now() + interval together, so the two agree exactly. It bites
// only when something moves next_run_at backwards on its own.
func (r *ScheduleRepository) ListDue(
	ctx context.Context, limit int,
) ([]*models.Schedule, error) {
	query := `
		SELECT ` + scheduleColumns + `
		FROM schedules
		WHERE next_run_at <= now()
		  AND (
		        last_run_at IS NULL
		     OR last_run_at <= now() - make_interval(secs => interval_seconds)
		      )
		ORDER BY next_run_at ASC, id ASC
		LIMIT $1
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list due schedules: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close schedule rows", "error", closeErr)
		}
	}()

	schedules := make([]*models.Schedule, 0)
	for rows.Next() {
		var s models.Schedule
		if err := rows.Scan(scanScheduleDest(&s)...); err != nil {
			return nil, fmt.Errorf("failed to scan schedule: %w", err)
		}
		schedules = append(schedules, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate due schedules: %w", err)
	}
	return schedules, nil
}

// MarkRun records a run against the database clock: last_run_at = now() and
// next_run_at advances to now() + interval_seconds in the same statement, so
// the advancement is atomic with the write and every replica shares one clock.
// It is an error when no schedule has the given id.
func (r *ScheduleRepository) MarkRun(ctx context.Context, id string) error {
	query := `
		UPDATE schedules
		SET last_run_at = now(),
		    next_run_at = now() + make_interval(secs => interval_seconds),
		    updated_at = now()
		WHERE id = $1
	`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to mark schedule run: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read schedule mark-run result: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("failed to mark schedule run: schedule %s not found", id)
	}
	return nil
}

// Delete removes the schedule for (teamID, jobType); a missing row is not an
// error.
func (r *ScheduleRepository) Delete(ctx context.Context, teamID, jobType string) error {
	query := `DELETE FROM schedules WHERE team_id = $1 AND job_type = $2`
	if _, err := r.db.ExecContext(ctx, query, teamID, jobType); err != nil {
		return fmt.Errorf("failed to delete schedule: %w", err)
	}
	return nil
}

// scanScheduleDest returns the scan targets for scheduleColumns, in order.
func scanScheduleDest(s *models.Schedule) []interface{} {
	return []interface{}{
		&s.ID, &s.TeamID, &s.JobType, &s.IntervalSeconds,
		&s.LastRunAt, &s.NextRunAt, &s.CreatedAt, &s.UpdatedAt,
	}
}
