package events

import (
	"errors"
	"sync"
	"time"

	"github.com/sony/gobreaker/v2"
)

// errListenerFailed is a sentinel error handed to gobreaker's done callback to mark a
// trial/closed-state request as failed. gobreaker treats any non-nil error as a failure.
var errListenerFailed = errors.New("event listener failed")

// CircuitBreaker prevents cascading failures by tripping per listener type.
//
// State is backed by sony/gobreaker. One gobreaker breaker is kept per key
// (the listener type) in a lazily-populated map, preserving the original
// per-key independence: tripping one listener type never affects another.
//
// Semantics differ from the previous hand-rolled breaker in two deliberate ways:
//
//  1. Half-open trial: the old implementation reset fully to closed the moment
//     the reset timeout elapsed, whereas gobreaker moves Open -> HalfOpen and
//     admits one trial request (MaxRequests=1) whose outcome decides whether
//     the breaker closes or re-opens.
//  2. Records while Open are dropped: RecordSuccess/RecordFailure are no-ops
//     while the breaker is open (gobreaker's Allow returns ErrOpenState). The
//     old implementation counted failures recorded while open and extended the
//     open window on each one; with this implementation, in-flight events that
//     fail after the breaker has opened no longer prolong the outage window.
type CircuitBreaker struct {
	maxFailures  int
	resetTimeout time.Duration
	mu           sync.Mutex
	breakers     map[string]*gobreaker.TwoStepCircuitBreaker[any]
}

// NewCircuitBreaker creates a circuit breaker that trips after maxFailures
// consecutive failures for a given key and admits a trial request after
// resetTimeoutSeconds have elapsed.
//
// Both arguments must be positive: maxFailures <= 0 trips the breaker on the
// first recorded failure, and resetTimeoutSeconds <= 0 falls back to
// gobreaker's default open duration of 60 seconds rather than disabling the
// timeout. The event bus always constructs the breaker with positive values.
func NewCircuitBreaker(maxFailures, resetTimeoutSeconds int) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: time.Duration(resetTimeoutSeconds) * time.Second,
		breakers:     make(map[string]*gobreaker.TwoStepCircuitBreaker[any]),
	}
}

// breakerFor returns the breaker for key, creating it on first use.
func (cb *CircuitBreaker) breakerFor(key string) *gobreaker.TwoStepCircuitBreaker[any] {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if b, ok := cb.breakers[key]; ok {
		return b
	}

	maxFailures := cb.maxFailures
	b := gobreaker.NewTwoStepCircuitBreaker[any](gobreaker.Settings{
		Name:        key,
		MaxRequests: 1,
		Interval:    0,
		Timeout:     cb.resetTimeout,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return int(c.ConsecutiveFailures) >= maxFailures
		},
	})
	cb.breakers[key] = b
	return b
}

// CanExecute reports whether execution is allowed for key. It returns true when
// the breaker is closed or half-open (gobreaker's State lazily transitions an
// expired open breaker to half-open), and false only while the breaker is open.
func (cb *CircuitBreaker) CanExecute(key string) bool {
	return cb.breakerFor(key).State() != gobreaker.StateOpen
}

// RecordSuccess records a successful execution for key, resetting the
// consecutive-failure count (and closing the breaker if it was half-open).
func (cb *CircuitBreaker) RecordSuccess(key string) {
	if done, err := cb.breakerFor(key).Allow(); err == nil {
		done(nil)
	}
}

// RecordFailure records a failed execution for key, advancing the
// consecutive-failure count toward the trip threshold.
func (cb *CircuitBreaker) RecordFailure(key string) {
	if done, err := cb.breakerFor(key).Allow(); err == nil {
		done(errListenerFailed)
	}
}
