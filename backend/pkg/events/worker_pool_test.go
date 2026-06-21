package events

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWorkerPool_NewWorkerPoolWorkerCount(t *testing.T) {
	t.Run("creates pool with correct worker count", func(t *testing.T) {
		pool := NewWorkerPool(5)
		assert.NotNil(t, pool)
		assert.Equal(t, 5, pool.workerCount)
	})

	t.Run("uses default worker count when invalid", func(t *testing.T) {
		pool := NewWorkerPool(0)
		assert.Equal(t, 10, pool.workerCount)

		pool = NewWorkerPool(-1)
		assert.Equal(t, 10, pool.workerCount)
	})
}

func TestWorkerPool_ExecutesSubmittedTasks(t *testing.T) {
	pool := NewWorkerPool(3)
	pool.Start()
	defer pool.Stop()

	var counter int32
	var wg sync.WaitGroup

	taskCount := 10
	wg.Add(taskCount)

	for i := 0; i < taskCount; i++ {
		pool.Submit(func() {
			atomic.AddInt32(&counter, 1)
			wg.Done()
		})
	}

	wg.Wait()

	assert.Equal(t, int32(taskCount), atomic.LoadInt32(&counter))
}

func TestWorkerPool_HandlesVaryingExecutionTimes(t *testing.T) {
	pool := NewWorkerPool(3)
	pool.Start()
	defer pool.Stop()

	var completed int32
	var wg sync.WaitGroup

	taskCount := 5
	wg.Add(taskCount)

	for i := 0; i < taskCount; i++ {
		duration := time.Duration(i*10) * time.Millisecond
		pool.Submit(func() {
			time.Sleep(duration)
			atomic.AddInt32(&completed, 1)
			wg.Done()
		})
	}

	wg.Wait()
	assert.Equal(t, int32(taskCount), atomic.LoadInt32(&completed))
}

func TestWorkerPool_HandlesPanicsInTasks(t *testing.T) {
	pool := NewWorkerPool(2)
	pool.Start()
	defer pool.Stop()

	var successCount int32
	var wg sync.WaitGroup

	wg.Add(3)

	// Submit task that panics; its deferred wg.Done still runs because the
	// ants panic handler fires after the task's own defers.
	pool.Submit(func() {
		defer wg.Done()
		panic("test panic")
	})

	pool.Submit(func() {
		defer wg.Done()
		atomic.AddInt32(&successCount, 1)
	})

	pool.Submit(func() {
		defer wg.Done()
		atomic.AddInt32(&successCount, 1)
	})

	wg.Wait()

	assert.Equal(t, int32(2), atomic.LoadInt32(&successCount))
}

func TestWorkerPool_StopsGracefully(t *testing.T) {
	pool := NewWorkerPool(3)
	pool.Start()

	var counter int32
	var wg sync.WaitGroup

	taskCount := 5
	wg.Add(taskCount)

	for i := 0; i < taskCount; i++ {
		pool.Submit(func() {
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&counter, 1)
			wg.Done()
		})
	}

	// Stop pool (should wait for in-flight tasks to complete).
	pool.Stop()

	wg.Wait()

	assert.Equal(t, int32(taskCount), atomic.LoadInt32(&counter))
}

func TestWorkerPool_StopIsIdempotent(t *testing.T) {
	pool := NewWorkerPool(2)
	pool.Start()

	assert.NotPanics(t, func() {
		pool.Stop()
		pool.Stop()
	}, "calling Stop twice must not panic")
}

func TestWorkerPool_HandlesFullTaskQueue(t *testing.T) {
	pool := NewWorkerPool(1)
	pool.Start()
	defer pool.Stop()

	var completed int32
	var wg sync.WaitGroup

	// Submit far more tasks than there are workers to force the overflow path.
	taskCount := 150
	wg.Add(taskCount)

	for i := 0; i < taskCount; i++ {
		pool.Submit(func() {
			atomic.AddInt32(&completed, 1)
			wg.Done()
		})
	}

	wg.Wait()

	// All tasks must complete, whether in a worker or the fallback goroutine.
	assert.Equal(t, int32(taskCount), atomic.LoadInt32(&completed))
}

func TestWorkerPool_SubmitAfterStopIsNoOp(t *testing.T) {
	pool := NewWorkerPool(2)
	pool.Start()
	pool.Stop()

	var executed int32
	assert.NotPanics(t, func() {
		pool.Submit(func() {
			atomic.AddInt32(&executed, 1)
		})
	}, "Submit after Stop must not panic")

	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&executed),
		"task submitted after Stop must not run")
}
