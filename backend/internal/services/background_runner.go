package services

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// BackgroundRunner runs short pieces of work after a request has been
// answered, with limits a bare `go` statement does not have:
//
//   - at most `limit` run at once; a task offered beyond that is dropped, not
//     queued;
//   - each has `timeout` to finish, and its context ends then;
//   - a task that ignores its context is abandoned shortly after the timeout:
//     its slot is freed and Shutdown stops waiting for it. At most 4*limit
//     abandoned tasks may be outstanding; beyond that new tasks are dropped
//     until some return, so a dependency that hangs cannot grow memory without
//     bound;
//   - a task that panics is logged and does not take the process down;
//   - Shutdown waits for the tasks in flight, then cancels what is left, and
//     always returns.
//
// It exists for the account emails (#1433). The forgot-password endpoint must
// answer in the same time whether or not the address has an account, so the
// lookup and the send happen here, after the answer; and a notice that a
// password changed must never delay or fail the change. Dropping under load is
// the right failure for both: an unbounded queue of outgoing mail is what an
// attacker flooding the endpoint wants.
//
// Abandoning exists because not every mail provider honours its context: the
// SMTP provider sends through net/smtp, which takes none, so a stalled server
// would otherwise hold a slot for as long as the connection hangs.
type BackgroundRunner struct {
	slots   chan struct{}
	timeout time.Duration
	// abandonAfter is how long past its timeout a task may run before it is
	// abandoned.
	abandonAfter time.Duration
	logger       *slog.Logger

	// base is cancelled by Shutdown once its grace period is over.
	base   context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	closed    bool
	abandoned int
	wg        sync.WaitGroup
}

const (
	// backgroundAbandonAfter is how long a task gets to return once its
	// context has ended.
	backgroundAbandonAfter = 2 * time.Second
	// backgroundPanicStackBytes caps the stack trace logged for a panic.
	backgroundPanicStackBytes = 4096
)

// NewBackgroundRunner creates a runner that runs at most limit tasks at once,
// each for at most timeout.
func NewBackgroundRunner(limit int, timeout time.Duration, logger *slog.Logger) *BackgroundRunner {
	base, cancel := context.WithCancel(context.Background())
	return &BackgroundRunner{
		slots:        make(chan struct{}, limit),
		timeout:      timeout,
		abandonAfter: backgroundAbandonAfter,
		logger:       logger,
		base:         base,
		cancel:       cancel,
	}
}

// Go runs fn in the background and reports whether it was accepted. It never
// blocks. fn's context is NOT the caller's: it ends at the task's timeout or
// when the runner shuts down, so a request that has already been answered
// does not cancel the work. A task offered while the runner is full, shut
// down, or carrying too many abandoned tasks is dropped and logged under name,
// with nothing about its content.
func (r *BackgroundRunner) Go(name string, fn func(ctx context.Context)) bool {
	if reason := r.admit(); reason != "" {
		r.logger.With("task", name, "limit", cap(r.slots)).Warn("Dropped a background task: " + reason)
		return false
	}

	go func() {
		defer r.wg.Done()
		ctx, cancel := context.WithTimeout(r.base, r.timeout)
		defer cancel()

		finished := make(chan struct{})
		go r.run(ctx, name, fn, finished)

		select {
		case <-finished:
			<-r.slots
			return
		case <-ctx.Done():
		}
		// The context has ended. A task that honours it returns promptly.
		grace := time.NewTimer(r.abandonAfter)
		defer grace.Stop()
		select {
		case <-finished:
			<-r.slots
		case <-grace.C:
			r.abandon(name, finished)
		}
	}()
	return true
}

// admit takes a slot for a new task, or says why it cannot.
func (r *BackgroundRunner) admit() (refusal string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return "the server is shutting down"
	}
	if r.abandoned >= 4*cap(r.slots) {
		return "too many earlier tasks are still stuck"
	}
	select {
	case r.slots <- struct{}{}:
	default:
		return "too many are already running"
	}
	r.wg.Add(1)
	return ""
}

// run executes fn and reports when it returns. A panic is logged, never
// propagated: one bad send must not end the process.
func (r *BackgroundRunner) run(
	ctx context.Context, name string, fn func(ctx context.Context), finished chan<- struct{},
) {
	defer close(finished)
	defer func() {
		if recovered := recover(); recovered != nil {
			stack := debug.Stack()
			if len(stack) > backgroundPanicStackBytes {
				stack = stack[:backgroundPanicStackBytes]
			}
			r.logger.With("task", name, "panic", fmt.Sprintf("%v", recovered), "stack", string(stack)).
				Error("A background task panicked")
		}
	}()
	fn(ctx)
}

// abandon frees the slot of a task that did not return after its context
// ended, and keeps count of it until it does.
func (r *BackgroundRunner) abandon(name string, finished <-chan struct{}) {
	r.mu.Lock()
	r.abandoned++
	outstanding := r.abandoned
	r.mu.Unlock()
	<-r.slots
	r.logger.With("task", name, "abandoned", outstanding).
		Warn("A background task ignored its deadline and was abandoned")

	go func() {
		<-finished
		r.mu.Lock()
		r.abandoned--
		r.mu.Unlock()
	}()
}

// Shutdown stops accepting tasks and waits for the running ones until ctx
// ends; then it cancels them and waits for them to return or be abandoned. It
// always returns, and is safe to call more than once.
func (r *BackgroundRunner) Shutdown(ctx context.Context) {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		r.logger.Warn("Background tasks were still running at shutdown and have been cancelled")
	}
	r.cancel()
	<-done
}
