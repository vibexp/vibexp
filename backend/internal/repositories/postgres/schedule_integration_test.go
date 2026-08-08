//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// seedScheduleTeam inserts a minimal team row (FK target for schedules) and
// returns its ID.
func seedScheduleTeam(t *testing.T) string {
	t.Helper()
	ownerID := insertTestUser(t)
	teamID := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, $3, $4)",
		teamID, ownerID, "Schedule Team", "schedule-team-"+teamID[:8])
	require.NoError(t, err)
	return teamID
}

func TestIntegrationSchedule_UpsertInsertThenUpdate(t *testing.T) {
	resetIntegrationTables(t)
	teamID := seedScheduleTeam(t)
	repo := NewScheduleRepository(integrationDB)
	ctx := context.Background()

	next := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	s := &models.Schedule{
		TeamID:          teamID,
		JobType:         "freshness_evaluate",
		IntervalSeconds: 86400,
		NextRunAt:       next,
	}
	require.NoError(t, repo.Upsert(ctx, s))
	require.NotEmpty(t, s.ID)
	assert.Equal(t, teamID, s.TeamID)
	assert.Equal(t, 86400, s.IntervalSeconds)
	assert.True(t, s.NextRunAt.Equal(next), "NextRunAt %v != %v", s.NextRunAt, next)
	assert.Nil(t, s.LastRunAt)

	// Second upsert on the same (team, job) updates in place: same ID, new
	// interval and next run time.
	later := next.Add(2 * time.Hour)
	upd := &models.Schedule{
		TeamID:          teamID,
		JobType:         "freshness_evaluate",
		IntervalSeconds: 3600,
		NextRunAt:       later,
	}
	require.NoError(t, repo.Upsert(ctx, upd))
	assert.Equal(t, s.ID, upd.ID, "upsert must update the existing row, not insert a second")
	assert.Equal(t, 3600, upd.IntervalSeconds)
	assert.True(t, upd.NextRunAt.Equal(later))

	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT count(*) FROM schedules WHERE team_id = $1", teamID).Scan(&count))
	assert.Equal(t, 1, count)

	// A zero NextRunAt defaults to the database clock (due immediately).
	other := &models.Schedule{TeamID: teamID, JobType: "digest", IntervalSeconds: 3600}
	require.NoError(t, repo.Upsert(ctx, other))
	assert.False(t, other.NextRunAt.IsZero(), "zero NextRunAt must default to now()")
}

func TestIntegrationSchedule_ListDueBoundaryOrderingAndLimit(t *testing.T) {
	resetIntegrationTables(t)
	teamID := seedScheduleTeam(t)
	repo := NewScheduleRepository(integrationDB)
	ctx := context.Background()

	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	insert := func(jobType string, nextRunAt time.Time) *models.Schedule {
		t.Helper()
		s := &models.Schedule{
			TeamID:          teamID,
			JobType:         jobType,
			IntervalSeconds: 3600,
			NextRunAt:       nextRunAt,
		}
		require.NoError(t, repo.Upsert(ctx, s))
		return s
	}

	exactlyNow := insert("due_now", now) // boundary: <= now is due
	overdue := insert("due_overdue", now.Add(-time.Hour))
	insert("not_due", now.Add(time.Minute)) // not due

	due, err := repo.ListDue(ctx, now, 10)
	require.NoError(t, err)
	require.Len(t, due, 2)
	// Most overdue first.
	assert.Equal(t, overdue.ID, due[0].ID)
	assert.Equal(t, exactlyNow.ID, due[1].ID)

	// Limit caps the result.
	capped, err := repo.ListDue(ctx, now, 1)
	require.NoError(t, err)
	require.Len(t, capped, 1)
	assert.Equal(t, overdue.ID, capped[0].ID)

	// A moment before the boundary only the overdue row is due; timestamptz is
	// microsecond-precision, so "exactly now" stays due at now - 1s.
	beforeBoundary, err := repo.ListDue(ctx, now.Add(-time.Second), 10)
	require.NoError(t, err)
	require.Len(t, beforeBoundary, 1)
	assert.Equal(t, overdue.ID, beforeBoundary[0].ID)

	// Well before every row, nothing is due.
	none, err := repo.ListDue(ctx, now.Add(-2*time.Hour), 10)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestIntegrationSchedule_ListDueUsesIndex(t *testing.T) {
	resetIntegrationTables(t)
	teamID := seedScheduleTeam(t)
	repo := NewScheduleRepository(integrationDB)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, &models.Schedule{
		TeamID: teamID, JobType: "idx_probe", IntervalSeconds: 3600,
		NextRunAt: time.Now().Add(-time.Minute),
	}))

	// A near-empty table makes the planner prefer a seq scan regardless of the
	// index; SET LOCAL enable_seqscan = off makes the index preference visible
	// (the planner still falls back to a seq scan if no usable index exists,
	// so a dropped index fails this test).
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off")
	require.NoError(t, err)

	var plan string
	require.NoError(t, tx.QueryRowContext(ctx,
		"EXPLAIN SELECT id FROM schedules WHERE next_run_at <= now()").Scan(&plan))
	assert.Contains(t, plan, "idx_schedules_next_run_at",
		"due-selection must use the next_run_at index, got plan: %s", plan)
}

func TestIntegrationSchedule_MarkRunAdvancesFromRanAt(t *testing.T) {
	resetIntegrationTables(t)
	teamID := seedScheduleTeam(t)
	repo := NewScheduleRepository(integrationDB)
	ctx := context.Background()

	s := &models.Schedule{
		TeamID:          teamID,
		JobType:         "freshness_evaluate",
		IntervalSeconds: 3600,
		NextRunAt:       time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC),
	}
	require.NoError(t, repo.Upsert(ctx, s))

	ranAt := time.Date(2026, 8, 8, 12, 30, 0, 0, time.UTC)
	require.NoError(t, repo.MarkRun(ctx, s.ID, ranAt))

	var lastRunAt time.Time
	var nextRunAt time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT last_run_at, next_run_at FROM schedules WHERE id = $1", s.ID,
	).Scan(&lastRunAt, &nextRunAt))
	assert.True(t, lastRunAt.Equal(ranAt), "last_run_at %v != %v", lastRunAt, ranAt)
	// next_run_at advances from ranAt, not from the old next_run_at.
	wantNext := ranAt.Add(time.Hour)
	assert.True(t, nextRunAt.Equal(wantNext), "next_run_at %v != %v", nextRunAt, wantNext)

	// Unknown id is an error.
	err := repo.MarkRun(ctx, uuid.New().String(), ranAt)
	assert.Error(t, err)
}

func TestIntegrationSchedule_IntervalFloorCheck(t *testing.T) {
	resetIntegrationTables(t)
	teamID := seedScheduleTeam(t)
	repo := NewScheduleRepository(integrationDB)
	ctx := context.Background()

	s := &models.Schedule{
		TeamID:          teamID,
		JobType:         "too_frequent",
		IntervalSeconds: 3599, // below the 1-hour floor
		NextRunAt:       time.Now(),
	}
	err := repo.Upsert(ctx, s)
	require.Error(t, err, "CHECK must reject interval_seconds < 3600")
	assert.Contains(t, err.Error(), "interval")

	// Exactly 3600 is allowed.
	s.IntervalSeconds = 3600
	require.NoError(t, repo.Upsert(ctx, s))
}

func TestIntegrationSchedule_Delete(t *testing.T) {
	resetIntegrationTables(t)
	teamID := seedScheduleTeam(t)
	repo := NewScheduleRepository(integrationDB)
	ctx := context.Background()

	s := &models.Schedule{
		TeamID:          teamID,
		JobType:         "freshness_evaluate",
		IntervalSeconds: 3600,
		NextRunAt:       time.Now(),
	}
	require.NoError(t, repo.Upsert(ctx, s))

	require.NoError(t, repo.Delete(ctx, teamID, "freshness_evaluate"))
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT count(*) FROM schedules WHERE team_id = $1", teamID).Scan(&count))
	assert.Equal(t, 0, count)

	// Deleting a missing schedule is not an error.
	require.NoError(t, repo.Delete(ctx, teamID, "freshness_evaluate"))
}
