//go:build integration

package postgres

import (
	"context"
	"strings"
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

func TestIntegrationSchedule_ListDueOrderingAndLimit(t *testing.T) {
	resetIntegrationTables(t)
	teamID := seedScheduleTeam(t)
	repo := NewScheduleRepository(integrationDB)
	ctx := context.Background()

	// Read the database clock: ListDue compares against now() in the database,
	// so the test fixtures are anchored to it, not the app clock.
	var dbNow time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT now()").Scan(&dbNow))

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

	overdue := insert("due_overdue", dbNow.Add(-time.Hour))
	justNow := insert("due_just_now", dbNow.Add(-time.Second)) // due (<= now)
	insert("not_due", dbNow.Add(time.Hour))                    // future, not due

	due, err := repo.ListDue(ctx, 10)
	require.NoError(t, err)
	require.Len(t, due, 2)
	// Most overdue first.
	assert.Equal(t, overdue.ID, due[0].ID)
	assert.Equal(t, justNow.ID, due[1].ID)

	// Limit caps the result.
	capped, err := repo.ListDue(ctx, 1)
	require.NoError(t, err)
	require.Len(t, capped, 1)
	assert.Equal(t, overdue.ID, capped[0].ID)
}

func TestIntegrationSchedule_ListDueNotDueWhenFuture(t *testing.T) {
	resetIntegrationTables(t)
	teamID := seedScheduleTeam(t)
	repo := NewScheduleRepository(integrationDB)
	ctx := context.Background()

	var dbNow time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT now()").Scan(&dbNow))

	// Every row is in the future, so nothing is due.
	require.NoError(t, repo.Upsert(ctx, &models.Schedule{
		TeamID: teamID, JobType: "future", IntervalSeconds: 3600,
		NextRunAt: dbNow.Add(time.Hour),
	}))

	due, err := repo.ListDue(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, due)
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

	// A small/seeded table makes the planner choose between an index scan, a
	// bitmap scan (index used in a child node), or — with no usable index — a
	// seq scan. SET LOCAL enable_seqscan = off removes the seq-scan fallback so
	// the index preference is visible; EXPLAIN (without ANALYZE) is one plan
	// line per node, joined here so both "Index Scan using idx_…" and a bitmap
	// "Bitmap Index Scan on idx_…" child are caught. A dropped index still
	// yields a seq scan and fails the assertion.
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off")
	require.NoError(t, err)

	// The predicate must mirror ListDue's verbatim, interval floor included: an
	// EXPLAIN of a hand-retyped query proves nothing about the query that runs.
	rows, err := tx.QueryContext(ctx, `
		EXPLAIN SELECT id FROM schedules
		WHERE next_run_at <= now()
		  AND (
		        last_run_at IS NULL
		     OR last_run_at <= now() - make_interval(secs => interval_seconds)
		      )`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var plan strings.Builder
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	require.NoError(t, rows.Err())
	assert.Contains(t, plan.String(), "idx_schedules_next_run_at",
		"due-selection must use the next_run_at index, got plan:\n%s", plan.String())
}

func TestIntegrationSchedule_MarkRunAdvancesFromDBClock(t *testing.T) {
	resetIntegrationTables(t)
	teamID := seedScheduleTeam(t)
	repo := NewScheduleRepository(integrationDB)
	ctx := context.Background()

	s := &models.Schedule{
		TeamID:          teamID,
		JobType:         "freshness_evaluate",
		IntervalSeconds: 3600,
		NextRunAt:       time.Now().Add(-time.Minute), // already overdue
	}
	require.NoError(t, repo.Upsert(ctx, s))

	// Read the DB clock just before marking the run; MarkRun uses now() in the
	// database, so last/next are anchored to it (within a small delta).
	var before time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT now()").Scan(&before))
	require.NoError(t, repo.MarkRun(ctx, s.ID))
	var after time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT now()").Scan(&after))

	var lastRunAt time.Time
	var nextRunAt time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT last_run_at, next_run_at FROM schedules WHERE id = $1", s.ID,
	).Scan(&lastRunAt, &nextRunAt))

	// last_run_at is the DB clock at mark time.
	assert.False(t, lastRunAt.Before(before), "last_run_at %v before %v", lastRunAt, before)
	assert.False(t, lastRunAt.After(after), "last_run_at %v after %v", lastRunAt, after)
	// next_run_at advances one interval from the run time (DB clock).
	wantNext := lastRunAt.Add(time.Hour)
	assert.True(t, nextRunAt.Equal(wantNext), "next_run_at %v != %v", nextRunAt, wantNext)

	// Unknown id is an error.
	err := repo.MarkRun(ctx, uuid.New().String())
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

// setLastRunAt backdates a schedule's last_run_at to an offset from the
// DATABASE clock. The floor is evaluated in SQL against now(), so anchoring
// fixtures to the app clock would make these tests sensitive to skew between
// the test process and Postgres.
func setLastRunAt(t *testing.T, scheduleID string, offset time.Duration) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"UPDATE schedules SET last_run_at = now() + make_interval(secs => $2) WHERE id = $1",
		scheduleID, offset.Seconds())
	require.NoError(t, err)
}

// The interval floor (#767): a schedule that ran less than interval_seconds
// ago is not due, even though Upsert has reset next_run_at to now().
func TestIntegrationSchedule_ListDueFloorsRecentlyRunSchedule(t *testing.T) {
	resetIntegrationTables(t)
	teamID := seedScheduleTeam(t)
	repo := NewScheduleRepository(integrationDB)
	ctx := context.Background()

	s := &models.Schedule{TeamID: teamID, JobType: "freshness_evaluate", IntervalSeconds: 3600}
	require.NoError(t, repo.Upsert(ctx, s)) // zero NextRunAt => due now
	setLastRunAt(t, s.ID, -10*time.Minute)  // ran 10 minutes into a 1-hour interval

	due, err := repo.ListDue(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, due, "a schedule inside its interval must not be due despite next_run_at <= now()")
}

// The complement, and the regression this fix must not cause: once the
// interval really has elapsed, the schedule is due again on the next tick.
func TestIntegrationSchedule_ListDueRunsOnceIntervalElapsed(t *testing.T) {
	resetIntegrationTables(t)
	teamID := seedScheduleTeam(t)
	repo := NewScheduleRepository(integrationDB)
	ctx := context.Background()

	s := &models.Schedule{TeamID: teamID, JobType: "freshness_evaluate", IntervalSeconds: 3600}
	require.NoError(t, repo.Upsert(ctx, s))
	setLastRunAt(t, s.ID, -2*time.Hour) // well past the 1-hour interval

	due, err := repo.ListDue(ctx, 10)
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, s.ID, due[0].ID)
}

// A schedule that has never run is exempt, so provisioning one still fires it
// on the next tick rather than after a full interval of silence.
func TestIntegrationSchedule_ListDueExemptsNeverRunSchedule(t *testing.T) {
	resetIntegrationTables(t)
	teamID := seedScheduleTeam(t)
	repo := NewScheduleRepository(integrationDB)
	ctx := context.Background()

	s := &models.Schedule{TeamID: teamID, JobType: "freshness_evaluate", IntervalSeconds: 3600}
	require.NoError(t, repo.Upsert(ctx, s))

	var lastRunAt *time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT last_run_at FROM schedules WHERE id = $1", s.ID).Scan(&lastRunAt))
	require.Nil(t, lastRunAt, "fixture guard: a freshly upserted schedule has never run")

	due, err := repo.ListDue(ctx, 10)
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, s.ID, due[0].ID)
}

// The issue #767 scenario end to end: an admin saving settings in a loop makes
// syncSchedule re-Upsert, which resets next_run_at to now() every time. The
// floor must keep the job at one run per interval regardless.
func TestIntegrationSchedule_RepeatedUpsertCannotBeatTheInterval(t *testing.T) {
	resetIntegrationTables(t)
	teamID := seedScheduleTeam(t)
	repo := NewScheduleRepository(integrationDB)
	ctx := context.Background()

	s := &models.Schedule{TeamID: teamID, JobType: "freshness_evaluate", IntervalSeconds: 3600}
	require.NoError(t, repo.Upsert(ctx, s))

	// First tick: never run, so it is due and runs.
	due, err := repo.ListDue(ctx, 10)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.NoError(t, repo.MarkRun(ctx, s.ID))

	// Twenty settings saves inside the interval, each resetting next_run_at.
	for i := 0; i < 20; i++ {
		require.NoError(t, repo.Upsert(ctx, &models.Schedule{
			TeamID: teamID, JobType: "freshness_evaluate", IntervalSeconds: 3600,
		}))

		// Guard that the save really did re-arm the row -- otherwise this test
		// could pass because Upsert stopped resetting next_run_at, not because
		// the floor works.
		var rearmed bool
		require.NoError(t, integrationDB.QueryRowContext(ctx,
			"SELECT next_run_at <= now() FROM schedules WHERE id = $1", s.ID).Scan(&rearmed))
		require.True(t, rearmed, "fixture guard: Upsert must reset next_run_at to now()")

		due, err = repo.ListDue(ctx, 10)
		require.NoError(t, err)
		require.Empty(t, due, "save %d re-armed the schedule inside its interval", i)
	}
}
