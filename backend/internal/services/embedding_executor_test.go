package services

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inFlightTracker records the peak number of tasks running concurrently.
type inFlightTracker struct {
	cur atomic.Int32
	max atomic.Int32
}

func (t *inFlightTracker) enter() {
	c := t.cur.Add(1)
	for {
		m := t.max.Load()
		if c <= m || t.max.CompareAndSwap(m, c) {
			break
		}
	}
}

func (t *inFlightTracker) leave() { t.cur.Add(-1) }

func TestBoundedExecutor_BoundsConcurrency(t *testing.T) {
	t.Parallel()

	for _, workers := range []int{1, 3} {
		workers := workers
		t.Run("workers="+string(rune('0'+workers)), func(t *testing.T) {
			t.Parallel()

			e := newBoundedExecutor(workers)
			defer e.close()

			var tracker inFlightTracker
			var ran atomic.Int32
			const tasks = 50

			var wg sync.WaitGroup
			wg.Add(tasks)
			for i := 0; i < tasks; i++ {
				require.True(t, e.submit(func() {
					defer wg.Done()
					tracker.enter()
					time.Sleep(time.Millisecond)
					ran.Add(1)
					tracker.leave()
				}))
			}
			wg.Wait()

			assert.Equal(t, int32(tasks), ran.Load(), "every submitted task must run")
			assert.LessOrEqual(t, int(tracker.max.Load()), workers,
				"never more than `workers` tasks in flight at once")
		})
	}
}

func TestBoundedExecutor_CloseDrainsQueuedTasks(t *testing.T) {
	t.Parallel()

	// A single worker with a slow first task leaves the rest queued; close must
	// let them all finish, not drop them.
	e := newBoundedExecutor(1)
	var ran atomic.Int32
	const tasks = 20
	for i := 0; i < tasks; i++ {
		require.True(t, e.submit(func() {
			time.Sleep(time.Millisecond)
			ran.Add(1)
		}))
	}

	e.close() // blocks until the queue is fully drained
	assert.Equal(t, int32(tasks), ran.Load(), "close must drain all queued tasks")
}

func TestBoundedExecutor_SubmitAfterCloseIsRejected(t *testing.T) {
	t.Parallel()

	e := newBoundedExecutor(2)
	e.close()

	var ran atomic.Bool
	assert.False(t, e.submit(func() { ran.Store(true) }), "submit after close returns false")
	assert.False(t, ran.Load(), "a rejected task must not run")

	assert.NotPanics(t, e.close, "close is idempotent")
}

func TestBoundedExecutor_DefaultsToOneWorker(t *testing.T) {
	t.Parallel()

	e := newBoundedExecutor(0) // non-positive falls back to a single worker
	defer e.close()

	var tracker inFlightTracker
	var wg sync.WaitGroup
	const tasks = 10
	wg.Add(tasks)
	for i := 0; i < tasks; i++ {
		require.True(t, e.submit(func() {
			defer wg.Done()
			tracker.enter()
			time.Sleep(time.Millisecond)
			tracker.leave()
		}))
	}
	wg.Wait()

	assert.Equal(t, int32(1), tracker.max.Load())
}
