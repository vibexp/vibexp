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

// ListDue returns schedules with next_run_at at or before now, most overdue
// first (id as deterministic tiebreaker), capped at limit.
func (r *ScheduleRepository) ListDue(
	ctx context.Context, now time.Time, limit int,
) ([]*models.Schedule, error) {
	query := `
		SELECT ` + scheduleColumns + `
		FROM schedules
		WHERE next_run_at <= $1
		ORDER BY next_run_at ASC, id ASC
		LIMIT $2
	`

	rows, err := r.db.QueryContext(ctx, query, now, limit)
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

// MarkRun records a run: last_run_at = ranAt and next_run_at advances to
// ranAt + interval_seconds in the same statement, so the advancement is
// atomic with the write. It is an error when no schedule has the given id.
func (r *ScheduleRepository) MarkRun(ctx context.Context, id string, ranAt time.Time) error {
	// $2 is referenced twice via an explicit text::timestamptz cast: lib/pq
	// infers one type per placeholder, and a bare $2 in both a timestamptz
	// assignment and an interval expression fails with "inconsistent types
	// deduced" (42P08). (Deriving from the just-SET last_run_at instead does
	// not work either: SET expressions evaluate against the OLD row values.)
	query := `
		UPDATE schedules
		SET last_run_at = $2::text::timestamptz,
		    next_run_at = $2::text::timestamptz + make_interval(secs => interval_seconds),
		    updated_at = now()
		WHERE id = $1
	`
	res, err := r.db.ExecContext(ctx, query, id, ranAt)
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
