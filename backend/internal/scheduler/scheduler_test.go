package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/vibexp/vibexp/internal/models"
)

const testSchedulerDSN = "postgres://scheduler:test@localhost:5432/scheduler_unit?sslmode=disable"

// discardLogger keeps unit-test output clean; log content is not what these
// tests assert on.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestTick_UnknownJobTypeSkipped(t *testing.T) {
	s, sqlMock, repo := newTestScheduler(t)
	sched := dueSchedule("sched-unknown", "no_such_job")
	repo.EXPECT().ListDue(mock.Anything, 10).Return([]*models.Schedule{sched}, nil).Once()
	// No handler registered: the schedule is skipped — no lock attempt, no MarkRun.

	s.tick(context.Background())
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestTick_ClaimsRunsAndAdvances(t *testing.T) {
	s, sqlMock, repo := newTestScheduler(t)
	sched := dueSchedule("sched-1", "job_a")

	var ran atomic.Int32
	s.registry.Register("job_a", func(ctx context.Context, teamID string) error {
		ran.Add(1)
		assert.Equal(t, "team-1", teamID)
		return nil
	})

	repo.EXPECT().ListDue(mock.Anything, 10).Return([]*models.Schedule{sched}, nil).Once()
	expectLockClaim(sqlMock)
	repo.EXPECT().MarkRun(mock.Anything, "sched-1").Return(nil).Once()

	s.tick(context.Background())
	assert.Equal(t, int32(1), ran.Load())
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestTick_LockHeldSkipsHandler(t *testing.T) {
	s, sqlMock, repo := newTestScheduler(t)
	sched := dueSchedule("sched-1", "job_a")

	var ran atomic.Int32
	s.registry.Register("job_a", func(ctx context.Context, teamID string) error {
		ran.Add(1)
		return nil
	})

	repo.EXPECT().ListDue(mock.Anything, 10).Return([]*models.Schedule{sched}, nil).Once()
	// pg_try_advisory_lock returns false: another replica holds the schedule.
	sqlMock.ExpectQuery(`SELECT pg_try_advisory_lock`).WillReturnRows(
		sqlmock.NewRows([]string{"locked"}).AddRow(false))
	// No handler run, no MarkRun, no unlock (never locked).

	s.tick(context.Background())
	assert.Equal(t, int32(0), ran.Load())
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestTick_HandlerPanicStillAdvances(t *testing.T) {
	s, sqlMock, repo := newTestScheduler(t)
	sched := dueSchedule("sched-1", "job_a")

	s.registry.Register("job_a", func(ctx context.Context, teamID string) error {
		panic("boom")
	})

	repo.EXPECT().ListDue(mock.Anything, 10).Return([]*models.Schedule{sched}, nil).Once()
	expectLockClaim(sqlMock)
	// The panic is recovered and the schedule still advances — a panicking job
	// must not be retried every tick.
	repo.EXPECT().MarkRun(mock.Anything, "sched-1").Return(nil).Once()

	s.tick(context.Background())
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestTick_HandlerErrorStillAdvances(t *testing.T) {
	s, sqlMock, repo := newTestScheduler(t)
	sched := dueSchedule("sched-1", "job_a")

	var ran atomic.Int32
	s.registry.Register("job_a", func(ctx context.Context, teamID string) error {
		ran.Add(1)
		return errors.New("job failed")
	})

	repo.EXPECT().ListDue(mock.Anything, 10).Return([]*models.Schedule{sched}, nil).Once()
	expectLockClaim(sqlMock)
	repo.EXPECT().MarkRun(mock.Anything, "sched-1").Return(nil).Once()

	s.tick(context.Background())
	assert.Equal(t, int32(1), ran.Load())
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestTick_ListDueErrorIsNonFatal(t *testing.T) {
	s, _, repo := newTestScheduler(t)
	repo.EXPECT().ListDue(mock.Anything, 10).Return(nil, errors.New("db down")).Once()

	// Returns without panicking; the loop would tick again later.
	s.tick(context.Background())
}

func TestTick_MarkRunErrorLeavesScheduleDue(t *testing.T) {
	s, sqlMock, repo := newTestScheduler(t)
	sched := dueSchedule("sched-1", "job_a")

	var ran atomic.Int32
	s.registry.Register("job_a", func(ctx context.Context, teamID string) error {
		ran.Add(1)
		return nil
	})

	repo.EXPECT().ListDue(mock.Anything, 10).Return([]*models.Schedule{sched}, nil).Once()
	expectLockClaim(sqlMock)
	repo.EXPECT().MarkRun(mock.Anything, "sched-1").Return(errors.New("write failed")).Once()

	// The failure is logged, not propagated — the row stays due for the next tick.
	s.tick(context.Background())
	assert.Equal(t, int32(1), ran.Load())
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestTick_RunsMultipleDueSchedules(t *testing.T) {
	s, sqlMock, repo := newTestScheduler(t)
	a := dueSchedule("sched-a", "job_a")
	b := dueSchedule("sched-b", "job_b")

	var mu sync.Mutex
	ran := map[string]int{}
	register := func(jobType string) {
		s.registry.Register(jobType, func(ctx context.Context, teamID string) error {
			mu.Lock()
			ran[jobType]++
			mu.Unlock()
			return nil
		})
	}
	register("job_a")
	register("job_b")

	repo.EXPECT().ListDue(mock.Anything, 10).Return([]*models.Schedule{a, b}, nil).Once()
	expectLockClaim(sqlMock)
	repo.EXPECT().MarkRun(mock.Anything, "sched-a").Return(nil).Once()
	expectLockClaim(sqlMock)
	repo.EXPECT().MarkRun(mock.Anything, "sched-b").Return(nil).Once()

	s.tick(context.Background())
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, map[string]int{"job_a": 1, "job_b": 1}, ran)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestStartClose_LoopRunsThenStops(t *testing.T) {
	s, _, repo := newTestScheduler(t)

	done := make(chan struct{})
	repo.EXPECT().ListDue(mock.Anything, 10).
		RunAndReturn(func(ctx context.Context, limit int) ([]*models.Schedule, error) {
			select {
			case <-done:
			default:
				close(done) // signal the first (immediate) tick ran
			}
			return nil, nil
		})

	s.Start(context.Background())
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("first tick did not run")
	}
	s.Close() // must return promptly: no in-flight handler, loop exits on cancel

	// Closing again is a no-op, and Close without Start is safe too.
	s.Close()
	(&Scheduler{}).Close()
}

// TestStart_LoopSurvivesHandlerPanic covers the half of the panic-resilience AC
// that TestTick_HandlerPanicStillAdvances cannot: that one calls tick() directly,
// proving recovery and advance within a single tick. This one runs the real loop
// and asserts a SUBSEQUENT tick still claims and runs a schedule — i.e. a
// panicking job cannot take the scheduler down.
func TestStart_LoopSurvivesHandlerPanic(t *testing.T) {
	s, sqlMock, repo := newTestScheduler(t)
	// A short polling cadence so the second tick lands promptly. The loop's
	// first tick is immediate, so this only bounds the gap to the second.
	s.cfg.TickInterval = 10 * time.Millisecond

	// Every invocation panics, so the loop has to survive it more than once.
	s.registry.Register("job_a", func(ctx context.Context, teamID string) error {
		panic("boom")
	})

	var ticks atomic.Int32
	repo.EXPECT().ListDue(mock.Anything, 10).RunAndReturn(
		func(ctx context.Context, limit int) ([]*models.Schedule, error) {
			switch ticks.Add(1) {
			case 1:
				return []*models.Schedule{dueSchedule("sched-1", "job_a")}, nil
			case 2:
				return []*models.Schedule{dueSchedule("sched-2", "job_a")}, nil
			default:
				return nil, nil // the loop keeps polling; nothing left to claim
			}
		})

	secondRan := make(chan struct{})
	expectLockClaim(sqlMock)
	repo.EXPECT().MarkRun(mock.Anything, "sched-1").Return(nil).Once()
	expectLockClaim(sqlMock)
	repo.EXPECT().MarkRun(mock.Anything, "sched-2").Return(nil).
		Run(func(ctx context.Context, id string) { close(secondRan) }).Once()

	s.Start(context.Background())
	// Stop the loop before the cleanup closes the mock DB underneath it. Close
	// is idempotent, so the explicit call below is safe alongside this one,
	// which covers the t.Fatal path.
	defer s.Close()

	select {
	case <-secondRan:
	case <-time.After(5 * time.Second):
		t.Fatal("no tick ran after the first tick's handler panicked — the loop did not survive")
	}
	// MarkRun (which signalled secondRan) runs BEFORE runDue's deferred
	// pg_advisory_unlock, so asserting here directly would race the second
	// unlock. Close waits for the in-flight tick to finish first — the same
	// ordering TestClose_WaitsForInFlightHandler relies on.
	s.Close()
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

// TestStart_NonPositiveTickIntervalDoesNotPanic covers the caller-bug backstop
// in run: time.NewTicker panics on a non-positive duration, so a Scheduler
// constructed in code with a zero interval must fall back to defaultTickInterval
// rather than take the process down. (config's validateSchedulerConfig is the
// operator-facing check for the same mistake.)
func TestStart_NonPositiveTickIntervalDoesNotPanic(t *testing.T) {
	s, _, repo := newTestScheduler(t)
	s.cfg.TickInterval = 0

	polled := make(chan struct{})
	var once sync.Once
	repo.EXPECT().ListDue(mock.Anything, 10).RunAndReturn(
		func(ctx context.Context, limit int) ([]*models.Schedule, error) {
			once.Do(func() { close(polled) })
			return nil, nil
		})

	// The first tick is immediate, so this proves the loop started at all —
	// which it could not have done if NewTicker had panicked.
	s.Start(context.Background())
	defer s.Close()

	select {
	case <-polled:
	case <-time.After(5 * time.Second):
		t.Fatal("the loop never ran with a zero tick interval")
	}
}

func TestClose_WaitsForInFlightHandler(t *testing.T) {
	s, sqlMock, repo := newTestScheduler(t)
	sched := dueSchedule("sched-1", "job_a")

	started := make(chan struct{})
	release := make(chan struct{})
	s.registry.Register("job_a", func(ctx context.Context, teamID string) error {
		close(started)
		<-release // simulate a long in-flight job
		return nil
	})

	repo.EXPECT().ListDue(mock.Anything, 10).Return([]*models.Schedule{sched}, nil).Once()
	expectLockClaim(sqlMock)
	repo.EXPECT().MarkRun(mock.Anything, "sched-1").Return(nil).Once()

	s.Start(context.Background())
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not start")
	}

	closed := make(chan struct{})
	go func() {
		s.Close()
		close(closed)
	}()

	// The handler is mid-flight: Close must wait rather than interrupt it.
	select {
	case <-closed:
		t.Fatal("Close returned while the handler was still running")
	case <-time.After(200 * time.Millisecond):
	}

	close(release)
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after the handler finished")
	}
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}
