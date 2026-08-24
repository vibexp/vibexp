package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// fakeReconciler records its calls and returns a canned error/panic. A
// hand-rolled double rather than a mockery one: Reconciler lives in this
// package, is one method wide, and is not in .mockery.yaml (adding it there
// would generate a mock nothing outside this file wants).
type fakeReconciler struct {
	calls  atomic.Int32
	err    error
	panics bool
}

func (f *fakeReconciler) ReconcileSchedules(ctx context.Context) error {
	f.calls.Add(1)
	if f.panics {
		panic("reconciler exploded")
	}
	return f.err
}

// newReconcileScheduler builds a Scheduler wired for unit tests with the given
// reconcilers, mirroring newTestScheduler's sqlmock setup.
func newReconcileScheduler(t *testing.T, reconcilers ...Reconciler) (*Scheduler, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, sqlMock, err := sqlmock.NewWithDSN(testSchedulerDSN + "?case=" + t.Name())
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlMock.ExpectClose()
		if cerr := sqlDB.Close(); cerr != nil {
			t.Errorf("close mock db: %v", cerr)
		}
	})

	cfg := Config{TickInterval: time.Hour, JobTimeout: time.Minute, DueLimit: 10}
	s := New(mocks.NewMockScheduleRepository(t), &database.DB{DB: sqlDB},
		NewRegistry(), cfg, discardLogger(), reconcilers...)
	return s, sqlMock
}

func TestReconcile_ClaimsLockAndRunsEveryReconciler(t *testing.T) {
	first, second := &fakeReconciler{}, &fakeReconciler{}
	s, sqlMock := newReconcileScheduler(t, first, second)
	expectLockClaim(sqlMock)

	s.reconcile(context.Background())

	assert.Equal(t, int32(1), first.calls.Load())
	assert.Equal(t, int32(1), second.calls.Load())
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

// The lock is instance-wide, so a replica that loses the claim does no work at
// all — not "does the work without the lock".
func TestReconcile_LockHeldSkipsSweep(t *testing.T) {
	r := &fakeReconciler{}
	s, sqlMock := newReconcileScheduler(t, r)
	sqlMock.ExpectQuery(`SELECT pg_try_advisory_lock`).WillReturnRows(
		sqlmock.NewRows([]string{"locked"}).AddRow(false))
	// No unlock: a lock that was never acquired must not be released, or the
	// replica that DID acquire it loses it.

	s.reconcile(context.Background())

	assert.Zero(t, r.calls.Load())
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

// One reconciler failing must not stop the next: they are independent
// features, and the engine's whole job here is to keep repairing.
func TestReconcile_FailingReconcilerDoesNotStopTheNext(t *testing.T) {
	failing := &fakeReconciler{err: errors.New("boom")}
	healthy := &fakeReconciler{}
	s, sqlMock := newReconcileScheduler(t, failing, healthy)
	expectLockClaim(sqlMock)

	s.reconcile(context.Background())

	assert.Equal(t, int32(1), healthy.calls.Load())
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

// A panicking reconciler is recovered — and the lock is still released,
// because the unlock is deferred above the recover in runReconciler's caller.
func TestReconcile_PanicIsRecoveredAndLockReleased(t *testing.T) {
	exploding := &fakeReconciler{panics: true}
	healthy := &fakeReconciler{}
	s, sqlMock := newReconcileScheduler(t, exploding, healthy)
	expectLockClaim(sqlMock)

	assert.NotPanics(t, func() { s.reconcile(context.Background()) })

	assert.Equal(t, int32(1), healthy.calls.Load())
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

// With no reconcilers there is no sweep and, critically, no connection
// acquired: the sqlmock has no expectations at all, so any query would fail
// the test.
func TestReconcile_NoReconcilersTouchesNothing(t *testing.T) {
	s, sqlMock := newReconcileScheduler(t)

	s.reconcile(context.Background())

	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

// Start runs the first sweep immediately rather than after ReconcileInterval,
// so restarting an instance repairs it. Close() before asserting: the unlock
// and the connection close are DEFERRED, so a reconciler-side signal is
// released one step early and would race them (the same trap the runDue loop
// tests hit).
func TestStart_RunsFirstSweepImmediately(t *testing.T) {
	swept := make(chan struct{})
	r := &signallingReconciler{done: swept}
	s, sqlMock := newReconcileScheduler(t, r)
	expectLockClaim(sqlMock)
	// The run loop's own first tick fires too; let it find nothing due.
	s.repo.(*mocks.MockScheduleRepository).EXPECT().
		ListDue(mock.Anything, 10).Return(nil, nil).Maybe()

	s.Start(context.Background())
	select {
	case <-swept:
	case <-time.After(5 * time.Second):
		t.Fatal("the first reconcile sweep did not run at start")
	}
	s.Close()

	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

// Close returns only once an in-flight sweep has finished, so shutdown never
// races a half-written repair.
func TestClose_WaitsForAnInFlightSweep(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	r := &blockingReconciler{started: started, release: release}
	s, sqlMock := newReconcileScheduler(t, r)
	expectLockClaim(sqlMock)
	s.repo.(*mocks.MockScheduleRepository).EXPECT().
		ListDue(mock.Anything, 10).Return(nil, nil).Maybe()

	s.Start(context.Background())
	<-started

	closed := make(chan struct{})
	go func() { s.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned while a sweep was still running")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after the sweep finished")
	}
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

type signallingReconciler struct {
	done chan struct{}
	once atomic.Bool
}

func (s *signallingReconciler) ReconcileSchedules(context.Context) error {
	if s.once.CompareAndSwap(false, true) {
		close(s.done)
	}
	return nil
}

type blockingReconciler struct {
	started chan struct{}
	release chan struct{}
	once    atomic.Bool
}

func (b *blockingReconciler) ReconcileSchedules(context.Context) error {
	if b.once.CompareAndSwap(false, true) {
		close(b.started)
	}
	<-b.release
	return nil
}
