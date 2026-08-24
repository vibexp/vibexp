package services

import "sync"

// boundedExecutor runs submitted tasks on a fixed number of worker goroutines
// draining an unbounded FIFO queue. It is the concurrency primitive that caps how
// many embedding requests run against a single provider at once (#142): sized to a
// provider's configured concurrency, no more than that many tasks execute
// concurrently, no matter how many are submitted at once.
//
// submit never blocks (the queue is unbounded), so a burst is absorbed in memory
// rather than dropped or fanned out onto unbounded goroutines.
//
// This type is still NOT durable, and deliberately so: since #820 it is no longer
// where a backlog accumulates. The dispatcher's system of record is the
// embedding_jobs table, and it claims work only up to the number of workers that
// can run it, so what this queue holds at any moment is a handful of in-flight
// jobs whose durable rows are still leased. A process that dies with jobs in here
// loses nothing -- their leases expire and the next poller reclaims them.
type boundedExecutor struct {
	mu     sync.Mutex
	cond   *sync.Cond
	queue  []func()
	closed bool
	wg     sync.WaitGroup
}

// newBoundedExecutor starts `workers` goroutines (at least one) draining the
// queue until close is called.
func newBoundedExecutor(workers int) *boundedExecutor {
	if workers < 1 {
		workers = 1
	}
	e := &boundedExecutor{}
	e.cond = sync.NewCond(&e.mu)
	e.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go e.worker()
	}
	return e
}

// submit enqueues a task and wakes a worker. It never blocks and returns false
// (without running the task) once the executor has been closed.
func (e *boundedExecutor) submit(task func()) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return false
	}
	e.queue = append(e.queue, task)
	e.cond.Signal()
	return true
}

func (e *boundedExecutor) worker() {
	defer e.wg.Done()
	for {
		e.mu.Lock()
		for len(e.queue) == 0 && !e.closed {
			e.cond.Wait()
		}
		if len(e.queue) == 0 { // closed and fully drained
			e.mu.Unlock()
			return
		}
		task := e.queue[0]
		e.queue[0] = nil // release the closure for GC before reslicing
		e.queue = e.queue[1:]
		e.mu.Unlock()

		task()
	}
}

// close stops accepting new tasks, lets the workers finish everything already
// queued, then waits for them to exit. It is idempotent.
func (e *boundedExecutor) close() {
	e.mu.Lock()
	if !e.closed {
		e.closed = true
		e.cond.Broadcast()
	}
	e.mu.Unlock()
	e.wg.Wait()
}
