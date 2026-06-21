package events

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/logging"
)

// EventBus is an in-memory event bus for publishing and subscribing to events
type EventBus interface {
	// Publish publishes an event to all registered listeners
	Publish(ctx context.Context, event Event) error

	// Subscribe registers a listener for specific event types
	Subscribe(listener EventListener) error

	// Unsubscribe removes a listener
	Unsubscribe(listener EventListener) error

	// Start starts the event bus processing
	Start() error

	// Stop gracefully stops the event bus
	Stop() error
}

// MetricsRecorder is an interface for recording event bus metrics
// This allows the event bus to be decoupled from the specific metrics implementation
type MetricsRecorder interface {
	RecordEventBusRetryAttempt(ctx context.Context, listenerType, eventType string, attemptNum int)
	RecordEventBusRetrySuccess(ctx context.Context, listenerType, eventType string, attemptNum int)
	RecordEventBusRetryFailure(ctx context.Context, listenerType, eventType string)
	RecordEventBusRetryBackoff(ctx context.Context, listenerType, eventType string, backoffDuration time.Duration)
	RecordEventBusEventDuration(ctx context.Context, listenerType, eventType string, duration time.Duration, success bool)
	RecordEventBusCircuitBreakerOpen(ctx context.Context, listenerType string)
}

// InMemoryEventBus is an in-memory implementation of EventBus
type InMemoryEventBus struct {
	listeners      map[string][]EventListener // map[eventType][]listeners
	eventChannel   chan Event
	workerPool     *WorkerPool
	mu             sync.RWMutex
	running        bool
	stopChan       chan struct{}
	wg             sync.WaitGroup
	logger         *logrus.Logger
	maxRetries     int
	retryBackoff   time.Duration
	retryJitter    bool
	circuitBreaker *CircuitBreaker
	metrics        MetricsRecorder
}

// EventBusConfig holds configuration for the event bus including runtime dependencies
// It embeds Config which contains environment-scanned configuration values
type EventBusConfig struct {
	Config                        // Embedded environment configuration
	Logger         *logrus.Logger // Runtime dependency
	CircuitBreaker *CircuitBreaker
	Metrics        MetricsRecorder // Optional metrics recorder for observability
}

// NewInMemoryEventBus creates a new in-memory event bus
func NewInMemoryEventBus(config EventBusConfig) *InMemoryEventBus {
	// Validate and set WorkerCount with reasonable bounds
	if config.WorkerCount <= 0 {
		config.WorkerCount = 10 // default
	} else if config.WorkerCount > 1000 {
		// Cap at 1000 workers to prevent resource exhaustion
		config.Logger.Warn("WorkerCount exceeds maximum (1000), capping at 1000")
		config.WorkerCount = 1000
	}

	// Validate and set BufferSize with reasonable bounds
	if config.BufferSize <= 0 {
		config.BufferSize = 100 // default
	} else if config.BufferSize > 10000 {
		// Cap at 10000 events to prevent excessive memory usage
		config.Logger.Warn("BufferSize exceeds maximum (10000), capping at 10000")
		config.BufferSize = 10000
	}

	// Validate and set MaxRetries with reasonable bounds
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3 // default
	} else if config.MaxRetries > 10 {
		// Cap at 10 retries to prevent excessive delays
		config.Logger.Warn("MaxRetries exceeds maximum (10), capping at 10")
		config.MaxRetries = 10
	}

	// Validate and set RetryBackoff with reasonable bounds
	if config.RetryBackoff <= 0 {
		config.RetryBackoff = 100 * time.Millisecond // default: 100ms base backoff
	} else if config.RetryBackoff > 5*time.Second {
		// Cap at 5 seconds to prevent excessive initial backoff
		config.Logger.Warn("RetryBackoff exceeds maximum (5s), capping at 5s")
		config.RetryBackoff = 5 * time.Second
	}

	// RetryJitter defaults to false
	if config.Logger == nil {
		config.Logger = logging.NewCloudLogger(logging.CloudLoggerConfig{})
	}
	if config.CircuitBreaker == nil {
		config.CircuitBreaker = NewCircuitBreaker(5, 10) // default: 5 failures, 10 second timeout
	}

	eb := &InMemoryEventBus{
		listeners:      make(map[string][]EventListener),
		eventChannel:   make(chan Event, config.BufferSize),
		workerPool:     NewWorkerPool(config.WorkerCount),
		stopChan:       make(chan struct{}),
		logger:         config.Logger,
		maxRetries:     config.MaxRetries,
		retryBackoff:   config.RetryBackoff,
		retryJitter:    config.RetryJitter,
		circuitBreaker: config.CircuitBreaker,
		metrics:        config.Metrics,
	}

	return eb
}

// Start starts the event bus processing
func (b *InMemoryEventBus) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		return fmt.Errorf("event bus already running")
	}

	b.running = true
	b.workerPool.Start()

	// Start the event dispatcher
	b.wg.Add(1)
	go b.dispatchEvents()

	b.logger.Info("Event bus started")
	return nil
}

// Stop gracefully stops the event bus
func (b *InMemoryEventBus) Stop() error {
	b.mu.Lock()
	if !b.running {
		b.mu.Unlock()
		return fmt.Errorf("event bus not running")
	}
	b.running = false
	b.mu.Unlock()

	// Close the stop channel to signal dispatcher to stop
	close(b.stopChan)

	// Wait for dispatcher to finish
	b.wg.Wait()

	// Close event channel
	close(b.eventChannel)

	// Stop worker pool
	b.workerPool.Stop()

	b.logger.Info("Event bus stopped")
	return nil
}

// Publish publishes an event to all registered listeners
func (b *InMemoryEventBus) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	if !b.running {
		b.mu.RUnlock()
		return fmt.Errorf("event bus not running")
	}
	b.mu.RUnlock()

	// Non-blocking send with timeout handling
	select {
	case b.eventChannel <- event:
		b.logger.WithFields(logrus.Fields{
			"event_type": event.Type(),
			"user_id":    event.UserID(),
			"timestamp":  event.Timestamp(),
		}).Debug("Event published")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("event channel full, event dropped")
	}
}

// Subscribe registers a listener for specific event types
func (b *InMemoryEventBus) Subscribe(listener EventListener) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, eventType := range listener.EventTypes() {
		b.listeners[eventType] = append(b.listeners[eventType], listener)
		b.logger.WithFields(logrus.Fields{
			"event_type":    eventType,
			"listener_type": fmt.Sprintf("%T", listener),
		}).Debug("Listener subscribed")
	}

	return nil
}

// Unsubscribe removes a listener
func (b *InMemoryEventBus) Unsubscribe(listener EventListener) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, eventType := range listener.EventTypes() {
		listeners := b.listeners[eventType]
		for i, l := range listeners {
			if l == listener {
				// Remove listener from slice
				b.listeners[eventType] = append(listeners[:i], listeners[i+1:]...)
				b.logger.WithFields(logrus.Fields{
					"event_type":    eventType,
					"listener_type": fmt.Sprintf("%T", listener),
				}).Debug("Listener unsubscribed")
				break
			}
		}
	}

	return nil
}

// dispatchEvents dispatches events to registered listeners
func (b *InMemoryEventBus) drainRemainingEvents() {
	for {
		select {
		case event := <-b.eventChannel:
			if event != nil {
				b.processEvent(event)
			}
		default:
			return
		}
	}
}

func (b *InMemoryEventBus) dispatchEvents() {
	defer b.wg.Done()

	for {
		select {
		case event := <-b.eventChannel:
			if event == nil {
				continue
			}
			b.processEvent(event)
		case <-b.stopChan:
			b.drainRemainingEvents()
			return
		}
	}
}

// processEvent processes a single event by dispatching it to all registered listeners
func (b *InMemoryEventBus) processEvent(event Event) {
	b.mu.RLock()
	listeners := b.listeners[event.Type()]
	b.mu.RUnlock()

	if len(listeners) == 0 {
		b.logger.WithFields(logrus.Fields{
			"event_type": event.Type(),
		}).Debug("No listeners registered for event type")
		return
	}

	// Dispatch to each listener via worker pool
	for _, listener := range listeners {
		listener := listener // capture loop variable
		b.workerPool.Submit(func() {
			b.handleEventWithRetry(listener, event)
		})
	}
}

// isTransientError checks if an error is transient (network, timeout, 5xx) and retryable
// It uses type-safe error checking with errors.Is/errors.As and falls back to string matching
// only for HTTP status codes that don't have structured error types.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}

	// Check structured error types first (most reliable)
	if isContextError(err) || isNetworkTimeoutError(err) || isSyscallTransientError(err) {
		return true
	}

	// Fallback to string matching for errors without structured types
	return isTransientErrorByMessage(err)
}

// isContextError checks if the error is a context deadline or cancellation error
func isContextError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

// isNetworkTimeoutError checks if the error is a network timeout using net.Error interface
func isNetworkTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// isSyscallTransientError checks for specific syscall errors that are transient
func isSyscallTransientError(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.ENETRESET) ||
		errors.Is(err, syscall.ENETUNREACH)
}

// isTransientErrorByMessage checks error message for transient patterns as last resort
// This is necessary because HTTP client errors often don't expose structured error types
func isTransientErrorByMessage(err error) bool {
	errStr := strings.ToLower(err.Error())

	// Check for HTTP 5xx status codes
	if isHTTP5xxError(errStr) {
		return true
	}

	// Check common transient error patterns from third-party libraries
	transientPatterns := []string{
		"timeout",
		"temporary failure",
		"dial tcp",
		"connection refused",
		"network unreachable",
		"connection reset",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// isHTTP5xxError checks if error message contains HTTP 5xx status code
func isHTTP5xxError(errStr string) bool {
	if !strings.Contains(errStr, "status 5") {
		return false
	}

	idx := strings.Index(errStr, "status 5")
	if idx < 0 || idx+9 >= len(errStr) {
		return false
	}

	// Verify we have two digits after "status 5"
	firstDigit := errStr[idx+8]
	secondDigit := errStr[idx+9]
	return firstDigit >= '0' && firstDigit <= '9' && secondDigit >= '0' && secondDigit <= '9'
}

// newRetryBackoff builds the exponential backoff policy for a single retry
// sequence. The base interval is b.retryBackoff, doubled each attempt and capped
// at 30s; when b.retryJitter is set, each delay is randomized by ±10%.
//
// Note: MaxInterval caps the base interval before jitter is applied, so a
// jittered delay can briefly exceed the cap (up to ~33s = 30s * 1.1).
//
// The returned instance is not thread-safe; each retry sequence runs in its own
// goroutine, so a fresh instance per sequence keeps it safe.
func (b *InMemoryEventBus) newRetryBackoff() *backoff.ExponentialBackOff {
	randomizationFactor := 0.0
	if b.retryJitter {
		randomizationFactor = 0.1
	}
	return &backoff.ExponentialBackOff{
		InitialInterval:     b.retryBackoff,
		RandomizationFactor: randomizationFactor,
		Multiplier:          2.0,
		MaxInterval:         30 * time.Second,
	}
}

// shouldRetry determines if an error should be retried based on retry policy
func (b *InMemoryEventBus) shouldRetry(event Event, err error) bool {
	if err == nil {
		return false
	}

	policy := event.RetryPolicy()

	switch policy {
	case RetryPolicyNone:
		return false
	case RetryPolicyTransient:
		return isTransientError(err)
	case RetryPolicyDefault:
		return true
	default:
		return true // default to retry
	}
}

// applyRetryBackoffWithDuration applies a specific backoff duration before retry
func (b *InMemoryEventBus) applyRetryBackoffWithDuration(
	listenerType, eventType string,
	attempt int,
	backoffDuration time.Duration,
) {
	b.logger.WithFields(logrus.Fields{
		"listener_type": listenerType,
		"event_type":    eventType,
		"attempt":       attempt + 1,
		"backoff_ms":    backoffDuration.Milliseconds(),
	}).Debug("Applying exponential backoff before retry")
	time.Sleep(backoffDuration)
}

// logSuccessfulAttempt logs a successful event handling attempt
func (b *InMemoryEventBus) logSuccessfulAttempt(listenerType, eventType string, attempt int) {
	if attempt > 0 {
		b.logger.WithFields(logrus.Fields{
			"listener_type": listenerType,
			"event_type":    eventType,
			"attempt":       attempt + 1,
			"retry_count":   attempt,
		}).Info("Event handled successfully after retry")
	} else {
		b.logger.WithFields(logrus.Fields{
			"listener_type": listenerType,
			"event_type":    eventType,
		}).Debug("Event handled successfully")
	}
}

// logRetryAttempt logs a retry attempt with appropriate level and details
func (b *InMemoryEventBus) logRetryAttempt(listenerType, eventType string, attempt int, err error) {
	isLastAttempt := attempt == b.maxRetries-1
	if isLastAttempt {
		b.logger.WithFields(logrus.Fields{
			"listener_type": listenerType,
			"event_type":    eventType,
			"attempt":       attempt + 1,
			"error":         fmt.Sprintf("%+v", err),
		}).Warn("Failed to handle event on final attempt")
	} else {
		b.logger.WithFields(logrus.Fields{
			"listener_type": listenerType,
			"event_type":    eventType,
			"attempt":       attempt + 1,
			"error":         fmt.Sprintf("%+v", err),
			"is_transient":  isTransientError(err),
		}).Warn("Failed to handle event, will retry")
	}
}

// handleEventWithRetry handles an event with exponential backoff retry logic and circuit breaker
func (b *InMemoryEventBus) handleEventWithRetry(listener EventListener, event Event) {
	listenerType := fmt.Sprintf("%T", listener)
	eventType := event.Type()

	// Check circuit breaker before attempting
	if b.shouldSkipDueToCircuitBreaker(listenerType, eventType) {
		return
	}

	// Execute retry loop with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	lastErr := b.executeRetryLoop(ctx, listener, event, listenerType, eventType)

	// Handle final failure if all retries exhausted
	if lastErr != nil {
		b.handleFinalFailure(ctx, listenerType, eventType, lastErr, event.RetryPolicy())
	}
}

// shouldSkipDueToCircuitBreaker checks if event should be skipped due to circuit breaker
func (b *InMemoryEventBus) shouldSkipDueToCircuitBreaker(listenerType, eventType string) bool {
	if !b.circuitBreaker.CanExecute(listenerType) {
		b.logger.WithFields(logrus.Fields{
			"listener_type": listenerType,
			"event_type":    eventType,
		}).Warn("Circuit breaker open, skipping event")
		if b.metrics != nil {
			b.metrics.RecordEventBusCircuitBreakerOpen(context.Background(), listenerType)
		}
		return true
	}
	return false
}

// executeRetryLoop executes the retry loop and returns the last error (nil if successful)
func (b *InMemoryEventBus) executeRetryLoop(
	ctx context.Context,
	listener EventListener,
	event Event,
	listenerType, eventType string,
) error {
	var lastErr error

	// One backoff instance per retry sequence (the policy is not thread-safe,
	// but each sequence runs in its own goroutine).
	bo := b.newRetryBackoff()

	for attempt := 0; attempt < b.maxRetries; attempt++ {
		// Check context cancellation
		if b.isContextDone(ctx, listenerType, eventType, attempt) {
			return lastErr
		}

		// Apply backoff before retry (skip first attempt)
		b.applyRetryBackoffIfNeeded(ctx, bo, listenerType, eventType, attempt)

		// Execute listener and measure duration
		startTime := time.Now()
		err := listener.Handle(ctx, event)
		duration := time.Since(startTime)

		// Handle success
		if err == nil {
			b.handleRetrySuccess(ctx, listenerType, eventType, attempt, duration)
			return nil
		}

		// Handle failure
		lastErr = err
		b.recordFailureMetrics(ctx, listenerType, eventType, duration)

		// Check if we should continue retrying
		if !b.shouldContinueRetrying(event, err, listenerType, eventType, attempt) {
			break
		}
	}

	return lastErr
}

// isContextDone checks if context is canceled or timed out
func (b *InMemoryEventBus) isContextDone(
	ctx context.Context,
	listenerType, eventType string,
	attempt int,
) bool {
	select {
	case <-ctx.Done():
		b.logger.WithFields(logrus.Fields{
			"listener_type": listenerType,
			"event_type":    eventType,
			"error":         ctx.Err(),
			"attempt":       attempt,
		}).Warn("Context canceled or timed out, stopping retries")
		return true
	default:
		return false
	}
}

// applyRetryBackoffIfNeeded applies backoff delay and records metrics for retry attempts
func (b *InMemoryEventBus) applyRetryBackoffIfNeeded(
	ctx context.Context,
	bo *backoff.ExponentialBackOff,
	listenerType, eventType string,
	attempt int,
) {
	if attempt == 0 {
		return
	}

	// Record retry attempt metric
	if b.metrics != nil {
		b.metrics.RecordEventBusRetryAttempt(ctx, listenerType, eventType, attempt+1)
	}

	// Calculate and apply backoff. The first NextBackOff (loop attempt 1) yields
	// the base interval, matching the previous calculateBackoff(attempt-1) indexing.
	backoffDuration := bo.NextBackOff()
	if b.metrics != nil {
		b.metrics.RecordEventBusRetryBackoff(ctx, listenerType, eventType, backoffDuration)
	}
	b.applyRetryBackoffWithDuration(listenerType, eventType, attempt, backoffDuration)
}

// handleRetrySuccess handles successful event processing with metrics and logging
func (b *InMemoryEventBus) handleRetrySuccess(
	ctx context.Context,
	listenerType, eventType string,
	attempt int,
	duration time.Duration,
) {
	b.circuitBreaker.RecordSuccess(listenerType)
	b.logSuccessfulAttempt(listenerType, eventType, attempt)

	if b.metrics != nil {
		b.metrics.RecordEventBusEventDuration(ctx, listenerType, eventType, duration, true)
		if attempt > 0 {
			b.metrics.RecordEventBusRetrySuccess(ctx, listenerType, eventType, attempt)
		}
	}
}

// recordFailureMetrics records metrics for failed event processing
func (b *InMemoryEventBus) recordFailureMetrics(
	ctx context.Context,
	listenerType, eventType string,
	duration time.Duration,
) {
	if b.metrics != nil {
		b.metrics.RecordEventBusEventDuration(ctx, listenerType, eventType, duration, false)
	}
}

// shouldContinueRetrying determines if retry loop should continue
func (b *InMemoryEventBus) shouldContinueRetrying(
	event Event,
	err error,
	listenerType, eventType string,
	attempt int,
) bool {
	if !b.shouldRetry(event, err) {
		b.logger.WithFields(logrus.Fields{
			"listener_type": listenerType,
			"event_type":    eventType,
			"retry_policy":  event.RetryPolicy(),
			"error":         fmt.Sprintf("%+v", err),
		}).Warn("Non-retryable error encountered, stopping retries")
		return false
	}

	b.logRetryAttempt(listenerType, eventType, attempt, err)
	return true
}

// handleFinalFailure handles the case when all retries are exhausted
func (b *InMemoryEventBus) handleFinalFailure(
	ctx context.Context,
	listenerType, eventType string,
	err error,
	retryPolicy RetryPolicy,
) {
	b.circuitBreaker.RecordFailure(listenerType)

	if b.metrics != nil {
		b.metrics.RecordEventBusRetryFailure(ctx, listenerType, eventType)
	}

	b.logger.WithFields(logrus.Fields{
		"listener_type": listenerType,
		"event_type":    eventType,
		"error":         fmt.Sprintf("%+v", err),
		"max_retries":   b.maxRetries,
		"retry_policy":  retryPolicy,
	}).Error("Failed to handle event after all retries")
}
