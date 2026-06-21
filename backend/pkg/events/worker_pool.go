package events

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"
)

const (
	defaultWorkerCount = 10
	releaseTimeout     = 30 * time.Second
)

// WorkerPool manages a pool of worker goroutines for processing tasks.
// It is backed by an ants pool in nonblocking mode: Submit never blocks, and on
// pool saturation the task is run in a dedicated goroutine so it is never dropped.
type WorkerPool struct {
	workerCount int
	pool        *ants.Pool
	stopOnce    sync.Once
	stopped     atomic.Bool
}

// NewWorkerPool creates a new worker pool with the given worker count.
// A non-positive count falls back to the default of 10.
func NewWorkerPool(workerCount int) *WorkerPool {
	if workerCount <= 0 {
		workerCount = defaultWorkerCount
	}

	pool, err := ants.NewPool(
		workerCount,
		ants.WithNonblocking(true),
		ants.WithPanicHandler(func(any) {}),
	)
	if err != nil {
		panic(err)
	}

	return &WorkerPool{
		workerCount: workerCount,
		pool:        pool,
	}
}

// Start is retained for API compatibility; the ants pool runs immediately.
func (p *WorkerPool) Start() {}

// Stop gracefully stops the pool, waiting for in-flight tasks to finish. It is
// idempotent: subsequent calls are no-ops, so it is safe to invoke from a
// container Close() that may run more than once.
func (p *WorkerPool) Stop() {
	p.stopOnce.Do(func() {
		p.stopped.Store(true)
		if err := p.pool.ReleaseTimeout(releaseTimeout); err != nil {
			p.pool.Release()
		}
	})
}

// Submit submits a task to the worker pool. After Stop it is a silent no-op.
// If no worker is free, the task runs in a dedicated goroutine so it is never
// dropped and Submit never blocks.
func (p *WorkerPool) Submit(task func()) {
	// The explicit stopped flag (not just ants' ErrPoolClosed) is what makes
	// submit-after-stop a drop: relying on ErrPoolClosed alone would route the
	// task through the go task() fallback and run it after Stop.
	if p.stopped.Load() {
		return
	}

	if err := p.pool.Submit(task); err != nil {
		go task()
	}
}
