package services

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
)

func TestBackgroundRunner_RunsDetachedFromTheCaller(t *testing.T) {
	logger, _ := logtest.New()
	r := NewBackgroundRunner(2, time.Second, logger)
	var alive atomic.Bool

	require.True(t, r.Go("task", func(ctx context.Context) { alive.Store(ctx.Err() == nil) }))
	r.Shutdown(context.Background())

	assert.True(t, alive.Load(), "the task's context is its own, and alive")
}

// A task offered while every slot is busy is dropped at once, never queued.
func TestBackgroundRunner_DropsBeyondItsLimit(t *testing.T) {
	logger, logs := logtest.New()
	r := NewBackgroundRunner(2, 5*time.Second, logger)
	release := make(chan struct{})
	var ran atomic.Int32
	block := func(context.Context) { <-release; ran.Add(1) }

	require.True(t, r.Go("task", block))
	require.True(t, r.Go("task", block))
	started := time.Now()
	accepted := r.Go("task", func(context.Context) { ran.Add(1) })

	assert.False(t, accepted)
	assert.Less(t, time.Since(started), time.Second, "dropping does not wait")
	require.NotNil(t, logs.LastEntry())
	assert.Contains(t, logs.LastEntry().Message, "Dropped a background task")
	close(release)
	r.Shutdown(context.Background())
	assert.EqualValues(t, 2, ran.Load())

	// A slot freed by a finished task is usable again... on a runner that is
	// still open.
	open := NewBackgroundRunner(1, time.Second, logger)
	require.True(t, open.Go("task", func(context.Context) {}))
	open.Shutdown(context.Background())
}

func TestBackgroundRunner_TimesATaskOut(t *testing.T) {
	logger, _ := logtest.New()
	r := NewBackgroundRunner(1, 20*time.Millisecond, logger)
	ended := make(chan error, 1)

	require.True(t, r.Go("task", func(ctx context.Context) {
		<-ctx.Done()
		ended <- ctx.Err()
	}))

	select {
	case err := <-ended:
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(5 * time.Second):
		t.Fatal("the task was not timed out")
	}
	r.Shutdown(context.Background())
}

// Shutdown waits for running tasks until its context ends, then cancels them,
// and refuses new ones from the moment it is called.
func TestBackgroundRunner_Shutdown(t *testing.T) {
	logger, logs := logtest.New()

	t.Run("waits for a task that finishes in time", func(t *testing.T) {
		r := NewBackgroundRunner(1, 5*time.Second, logger)
		var finished atomic.Bool
		require.True(t, r.Go("task", func(context.Context) {
			time.Sleep(30 * time.Millisecond)
			finished.Store(true)
		}))

		r.Shutdown(context.Background())

		assert.True(t, finished.Load())
		assert.False(t, r.Go("late", func(context.Context) { t.Error("ran after shutdown") }))
		r.Shutdown(context.Background()) // safe to call again
	})

	t.Run("cancels a task that outlives the grace period, and returns", func(t *testing.T) {
		logs.Reset()
		r := NewBackgroundRunner(1, time.Minute, logger)
		cancelled := make(chan struct{})
		require.True(t, r.Go("task", func(ctx context.Context) {
			<-ctx.Done()
			close(cancelled)
		}))
		grace, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		done := make(chan struct{})
		go func() { r.Shutdown(grace); close(done) }()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Shutdown did not return")
		}
		<-cancelled
		require.NotNil(t, logs.LastEntry())
		assert.Contains(t, logs.LastEntry().Message, "have been cancelled")
	})
}

// A task that panics is logged and frees its slot; the process and the runner
// carry on.
func TestBackgroundRunner_SurvivesAPanic(t *testing.T) {
	logger, logs := logtest.New()
	r := NewBackgroundRunner(1, time.Second, logger)

	require.True(t, r.Go("bad", func(context.Context) { panic("provider exploded") }))
	var ran atomic.Bool
	require.Eventually(t, func() bool {
		return r.Go("good", func(context.Context) { ran.Store(true) })
	}, 5*time.Second, time.Millisecond, "the slot of the panicked task is free again")
	r.Shutdown(context.Background())

	assert.True(t, ran.Load())
	found := false
	for _, entry := range logs.AllEntries() {
		if entry.Message == "A background task panicked" {
			found = true
			assert.Equal(t, "bad", entry.Data["task"])
		}
	}
	assert.True(t, found)
}

// A task that ignores its context (an SMTP send does) is abandoned shortly
// after its deadline: the slot is free again and Shutdown does not wait for
// it. Too many abandoned tasks stop new ones being accepted, so a dependency
// that hangs cannot grow memory without bound.
func TestBackgroundRunner_AbandonsATaskThatIgnoresItsDeadline(t *testing.T) {
	logger, logs := logtest.New()
	r := NewBackgroundRunner(1, 10*time.Millisecond, logger)
	r.abandonAfter = 10 * time.Millisecond
	stuck := make(chan struct{})
	deaf := func(context.Context) { <-stuck }

	// One slot, so each accepted task proves the previous one was abandoned.
	for i := range 4 {
		require.Eventually(t, func() bool { return r.Go("deaf", deaf) }, 5*time.Second, time.Millisecond,
			"task %d: the slot of the stuck task before it was freed", i)
	}
	require.Eventually(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.abandoned == 4
	}, 5*time.Second, time.Millisecond)

	assert.False(t, r.Go("one too many", func(context.Context) {}), "four stuck tasks is the cap for one slot")
	assert.Contains(t, logs.LastEntry().Message, "too many earlier tasks are still stuck")

	done := make(chan struct{})
	go func() { r.Shutdown(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown waited for abandoned tasks")
	}

	// When the stuck tasks finally return they stop counting.
	close(stuck)
	require.Eventually(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.abandoned == 0
	}, 5*time.Second, time.Millisecond)
}
