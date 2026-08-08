//go:build integration

package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

// newIntegrationScheduler wires a Scheduler against the real integration
// database and postgres schedule repository — the way the DI container does.
func newIntegrationScheduler(reg *Registry) *Scheduler {
	repo := postgres.NewScheduleRepository(integrationDB)
	cfg := Config{TickInterval: time.Hour, JobTimeout: time.Minute, DueLimit: 10}
	return New(repo, integrationDB, reg, cfg, discardLogger())
}

// insertDueSchedule seeds a schedule whose next_run_at is in the past (due).
func insertDueSchedule(t *testing.T, teamID, jobType string) *models.Schedule {
	t.Helper()
	repo := postgres.NewScheduleRepository(integrationDB)
	s := &models.Schedule{
		TeamID:          teamID,
		JobType:         jobType,
		IntervalSeconds: 3600,
		NextRunAt:       time.Now().Add(-time.Minute),
	}
	require.NoError(t, repo.Upsert(context.Background(), s))
	require.NotEmpty(t, s.ID)
	return s
}

// Two schedulers racing the same due schedule must run the handler exactly
// once: the second finds the advisory lock held and skips. This holds the lock
// on a separate connection (simulating another replica mid-run) and asserts
// the scheduler does not run the handler.
func TestIntegration_LockContentionSkipsSecondReplica(t *testing.T) {
	resetIntegrationTables(t)
	teamID := insertTestTeam(t, insertTestUser(t))
	sched := insertDueSchedule(t, teamID, "contended_job")

	// Hold the schedule's advisory lock on a dedicated connection, as a busy
	// peer replica would.
	holder, err := integrationDB.Conn(context.Background())
	require.NoError(t, err)
	defer func() { require.NoError(t, holder.Close()) }()
	var held bool
	require.NoError(t, holder.QueryRowContext(context.Background(),
		`SELECT pg_try_advisory_lock($1)`, lockKey(sched.ID)).Scan(&held))
	require.True(t, held, "test setup must hold the lock")
	defer func() {
		_, _ = holder.ExecContext(context.Background(),
			`SELECT pg_advisory_unlock($1)`, lockKey(sched.ID))
	}()

	var ran atomic.Int32
	reg := NewRegistry()
	reg.Register("contended_job", func(ctx context.Context, teamID string) error {
		ran.Add(1)
		return nil
	})

	newIntegrationScheduler(reg).tick(context.Background())

	assert.Equal(t, int32(0), ran.Load(),
		"a replica that cannot take the advisory lock must not run the handler")

	// And the schedule was NOT advanced (still due) since nobody ran it.
	var nextRun time.Time
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		`SELECT next_run_at FROM schedules WHERE id = $1`, sched.ID).Scan(&nextRun))
	assert.True(t, nextRun.Before(time.Now()), "unclaimed schedule stays due")
}

// After a run completes and releases the lock, the schedule advances and a
// later tick re-claims and runs it — the lock is not leaked.
func TestIntegration_LockReleasedAfterRun(t *testing.T) {
	resetIntegrationTables(t)
	teamID := insertTestTeam(t, insertTestUser(t))
	sched := insertDueSchedule(t, teamID, "releasable_job")

	var ran atomic.Int32
	reg := NewRegistry()
	reg.Register("releasable_job", func(ctx context.Context, teamID string) error {
		ran.Add(1)
		return nil
	})
	s := newIntegrationScheduler(reg)

	s.tick(context.Background())
	assert.Equal(t, int32(1), ran.Load())

	// The lock was released: the same key is immediately claimable again.
	var free bool
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		`SELECT pg_try_advisory_lock($1)`, lockKey(sched.ID)).Scan(&free))
	assert.True(t, free, "lock must be released after the run")
	_, _ = integrationDB.ExecContext(context.Background(),
		`SELECT pg_advisory_unlock($1)`, lockKey(sched.ID))

	// The schedule advanced into the future, so a second tick runs nothing.
	s.tick(context.Background())
	assert.Equal(t, int32(1), ran.Load(), "advanced schedule is no longer due")
}

// End to end on the real database: a due schedule is claimed, its handler runs
// with the schedule's team, and next_run_at advances by one interval.
func TestIntegration_TickRunsDueAndAdvances(t *testing.T) {
	resetIntegrationTables(t)
	teamID := insertTestTeam(t, insertTestUser(t))
	sched := insertDueSchedule(t, teamID, "advance_job")

	var gotTeam string
	reg := NewRegistry()
	reg.Register("advance_job", func(ctx context.Context, teamID string) error {
		gotTeam = teamID
		return nil
	})

	newIntegrationScheduler(reg).tick(context.Background())
	assert.Equal(t, teamID, gotTeam)

	var lastRun time.Time
	var nextRun time.Time
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		`SELECT last_run_at, next_run_at FROM schedules WHERE id = $1`, sched.ID).
		Scan(&lastRun, &nextRun))
	// next advanced ~ one interval past the run time (DB clock).
	assert.InDelta(t, time.Hour.Seconds(), nextRun.Sub(lastRun).Seconds(), 2.0)
}

// Concurrent ticks from two scheduler instances over the same due set run each
// handler at most once overall (the lock serializes them), never twice.
func TestIntegration_ConcurrentSchedulersRunOnce(t *testing.T) {
	resetIntegrationTables(t)
	teamID := insertTestTeam(t, insertTestUser(t))
	insertDueSchedule(t, teamID, "race_job")

	var ran atomic.Int32
	reg := NewRegistry()
	reg.Register("race_job", func(ctx context.Context, teamID string) error {
		ran.Add(1)
		// Hold the lock briefly so the racing tick actually contends.
		time.Sleep(50 * time.Millisecond)
		return nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			newIntegrationScheduler(reg).tick(context.Background())
		}()
	}
	wg.Wait()

	assert.LessOrEqual(t, ran.Load(), int32(1),
		"two racing schedulers must not double-run one due schedule")
}
