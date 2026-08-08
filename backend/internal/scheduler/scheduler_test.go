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
