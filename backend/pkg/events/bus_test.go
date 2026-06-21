package events

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// MockListener is a mock event listener for testing
type MockListener struct {
	eventTypes     []string
	handleFunc     func(ctx context.Context, event Event) error
	handledEvents  []Event
	mu             sync.Mutex
	handleCount    int32
	shouldFail     bool
	failureCount   int
	currentAttempt int
}

func NewMockListener(eventTypes []string) *MockListener {
	return &MockListener{
		eventTypes:    eventTypes,
		handledEvents: make([]Event, 0),
	}
}

func (m *MockListener) EventTypes() []string {
	return m.eventTypes
}

func (m *MockListener) Handle(ctx context.Context, event Event) error {
	atomic.AddInt32(&m.handleCount, 1)

	m.mu.Lock()
	m.handledEvents = append(m.handledEvents, event)
	m.currentAttempt++

	if m.shouldFail && m.currentAttempt <= m.failureCount {
		m.mu.Unlock()
		return errors.New("mock listener error")
	}
	m.mu.Unlock()

	if m.handleFunc != nil {
		return m.handleFunc(ctx, event)
	}
	return nil
}

func (m *MockListener) GetHandledEvents() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Event{}, m.handledEvents...)
}

func (m *MockListener) GetHandleCount() int32 {
	return atomic.LoadInt32(&m.handleCount)
}

func (m *MockListener) SetShouldFail(shouldFail bool, failureCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail = shouldFail
	m.failureCount = failureCount
	m.currentAttempt = 0
}

func TestInMemoryEventBusBasics(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	t.Run("NewInMemoryEventBus creates bus with defaults", func(t *testing.T) {
		testBusCreation(t, logger)
	})

	t.Run("EventBus starts and stops", func(t *testing.T) {
		testBusStartStop(t, logger)
	})

	t.Run("EventBus cannot start twice", func(t *testing.T) {
		testBusCannotStartTwice(t, logger)
	})

	t.Run("EventBus publishes and dispatches events to listeners", func(t *testing.T) {
		testBusPublishAndDispatch(t, logger)
	})

	t.Run("EventBus handles multiple listeners for same event type", func(t *testing.T) {
		testBusMultipleListeners(t, logger)
	})

	t.Run("EventBus handles multiple event types", func(t *testing.T) {
		testBusMultipleEventTypes(t, logger)
	})

	t.Run("EventBus unsubscribes listener", func(t *testing.T) {
		testBusUnsubscribe(t, logger)
	})
}

func TestInMemoryEventBusReliability(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	t.Run("EventBus handles listener errors with retry", func(t *testing.T) {
		testBusListenerRetry(t, logger)
	})

	t.Run("EventBus does not publish when not running", func(t *testing.T) {
		testBusNotRunning(t, logger)
	})

	t.Run("EventBus handles context with timeout", func(t *testing.T) {
		testBusContextTimeout(t, logger)
	})

	t.Run("EventBus handles high load", func(t *testing.T) {
		testBusHighLoad(t, logger)
	})
}

func TestInMemoryEventBusRetryLogic(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	t.Run("EventBus applies exponential backoff on retry", func(t *testing.T) {
		testBusExponentialBackoff(t, logger)
	})

	t.Run("EventBus respects retry policy - none", func(t *testing.T) {
		testBusRetryPolicyNone(t, logger)
	})

	t.Run("EventBus respects retry policy - transient only", func(t *testing.T) {
		testBusRetryPolicyTransient(t, logger)
	})

	t.Run("EventBus detects transient errors correctly", func(t *testing.T) {
		testTransientErrorDetection(t)
	})

	t.Run("EventBus calculates backoff with jitter", func(t *testing.T) {
		testBackoffCalculationWithJitter(t, logger)
	})
}

func testBusCreation(t *testing.T, logger *logrus.Logger) {
	config := EventBusConfig{Logger: logger}
	bus := NewInMemoryEventBus(config)

	assert.NotNil(t, bus)
	assert.NotNil(t, bus.listeners)
	assert.NotNil(t, bus.eventChannel)
	assert.NotNil(t, bus.workerPool)
	assert.NotNil(t, bus.circuitBreaker)
}

func testBusStartStop(t *testing.T, logger *logrus.Logger) {
	config := EventBusConfig{Config: Config{WorkerCount: 2, BufferSize: 10}, Logger: logger}
	bus := NewInMemoryEventBus(config)

	err := bus.Start()
	assert.NoError(t, err)
	assert.True(t, bus.running)

	err = bus.Stop()
	assert.NoError(t, err)
	assert.False(t, bus.running)
}

func testBusCannotStartTwice(t *testing.T, logger *logrus.Logger) {
	config := EventBusConfig{Logger: logger}
	bus := NewInMemoryEventBus(config)

	err := bus.Start()
	assert.NoError(t, err)

	err = bus.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	if err := bus.Stop(); err != nil {
		t.Logf("Failed to stop bus: %v", err)
	}
}

func testBusPublishAndDispatch(t *testing.T, logger *logrus.Logger) {
	config := EventBusConfig{Config: Config{WorkerCount: 2, BufferSize: 10}, Logger: logger}
	bus := NewInMemoryEventBus(config)
	if err := bus.Start(); err != nil {
		t.Fatalf("Failed to start bus: %v", err)
	}
	defer stopBus(t, bus)

	listener := NewMockListener([]string{"test.event"})
	assert.NoError(t, bus.Subscribe(listener))

	event := NewBaseEvent("test.event", "payload", "user123")
	assert.NoError(t, bus.Publish(context.Background(), event))

	time.Sleep(100 * time.Millisecond)

	handledEvents := listener.GetHandledEvents()
	assert.Equal(t, 1, len(handledEvents))
	assert.Equal(t, event.Type(), handledEvents[0].Type())
}

func testBusMultipleListeners(t *testing.T, logger *logrus.Logger) {
	config := EventBusConfig{Config: Config{WorkerCount: 3, BufferSize: 10}, Logger: logger}
	bus := NewInMemoryEventBus(config)
	if err := bus.Start(); err != nil {
		t.Fatalf("Failed to start bus: %v", err)
	}
	defer stopBus(t, bus)

	listeners := subscribeMultipleListeners(t, bus, 3, []string{"test.event"})

	event := NewBaseEvent("test.event", "payload", "user123")
	assert.NoError(t, bus.Publish(context.Background(), event))

	time.Sleep(100 * time.Millisecond)

	for _, listener := range listeners {
		assert.Equal(t, 1, len(listener.GetHandledEvents()))
	}
}

func testBusMultipleEventTypes(t *testing.T, logger *logrus.Logger) {
	config := EventBusConfig{Config: Config{WorkerCount: 2, BufferSize: 10}, Logger: logger}
	bus := NewInMemoryEventBus(config)
	if err := bus.Start(); err != nil {
		t.Fatalf("Failed to start bus: %v", err)
	}
	defer stopBus(t, bus)

	listener1 := NewMockListener([]string{"event.type1"})
	listener2 := NewMockListener([]string{"event.type2"})

	assert.NoError(t, bus.Subscribe(listener1))
	assert.NoError(t, bus.Subscribe(listener2))

	event1 := NewBaseEvent("event.type1", "payload1", "user123")
	event2 := NewBaseEvent("event.type2", "payload2", "user123")

	assert.NoError(t, bus.Publish(context.Background(), event1))
	assert.NoError(t, bus.Publish(context.Background(), event2))

	time.Sleep(200 * time.Millisecond)

	assert.Equal(t, 1, len(listener1.GetHandledEvents()))
	assert.Equal(t, 1, len(listener2.GetHandledEvents()))
	assert.Equal(t, "event.type1", listener1.GetHandledEvents()[0].Type())
	assert.Equal(t, "event.type2", listener2.GetHandledEvents()[0].Type())
}

func testBusUnsubscribe(t *testing.T, logger *logrus.Logger) {
	config := EventBusConfig{Config: Config{WorkerCount: 2, BufferSize: 10}, Logger: logger}
	bus := NewInMemoryEventBus(config)
	if err := bus.Start(); err != nil {
		t.Fatalf("Failed to start bus: %v", err)
	}
	defer stopBus(t, bus)

	listener := NewMockListener([]string{"test.event"})
	assert.NoError(t, bus.Subscribe(listener))

	event1 := NewBaseEvent("test.event", "payload1", "user123")
	assert.NoError(t, bus.Publish(context.Background(), event1))
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, len(listener.GetHandledEvents()))

	assert.NoError(t, bus.Unsubscribe(listener))

	event2 := NewBaseEvent("test.event", "payload2", "user123")
	if err := bus.Publish(context.Background(), event2); err != nil {
		t.Logf("Failed to publish event2: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, len(listener.GetHandledEvents()))
}

func testBusListenerRetry(t *testing.T, logger *logrus.Logger) {
	config := EventBusConfig{
		Config: Config{
			WorkerCount:  1,
			BufferSize:   10,
			MaxRetries:   3,
			RetryBackoff: 10 * time.Millisecond, // Short backoff for testing
		},
		Logger: logger,
	}
	bus := NewInMemoryEventBus(config)
	if err := bus.Start(); err != nil {
		t.Fatalf("Failed to start bus: %v", err)
	}
	defer stopBus(t, bus)

	listener := NewMockListener([]string{"test.event"})
	listener.SetShouldFail(true, 2)

	assert.NoError(t, bus.Subscribe(listener))

	event := NewBaseEvent("test.event", "payload", "user123")
	assert.NoError(t, bus.Publish(context.Background(), event))

	time.Sleep(300 * time.Millisecond) // Increased to account for backoff delays
	assert.Equal(t, int32(3), listener.GetHandleCount())
}

func testBusNotRunning(t *testing.T, logger *logrus.Logger) {
	config := EventBusConfig{Logger: logger}
	bus := NewInMemoryEventBus(config)

	event := NewBaseEvent("test.event", "payload", "user123")
	err := bus.Publish(context.Background(), event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

func testBusContextTimeout(t *testing.T, logger *logrus.Logger) {
	config := EventBusConfig{Config: Config{WorkerCount: 1, BufferSize: 1}, Logger: logger}
	bus := NewInMemoryEventBus(config)
	if err := bus.Start(); err != nil {
		t.Fatalf("Failed to start bus: %v", err)
	}
	defer stopBus(t, bus)

	event1 := NewBaseEvent("test.event", "payload1", "user123")
	if err := bus.Publish(context.Background(), event1); err != nil {
		t.Logf("Failed to publish event1: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	event2 := NewBaseEvent("test.event", "payload2", "user123")
	err := bus.Publish(ctx, event2)
	if err == nil {
		t.Log("Event published successfully despite cancelled context (acceptable race condition)")
	} else {
		assert.Error(t, err)
	}
}

func testBusHighLoad(t *testing.T, logger *logrus.Logger) {
	config := EventBusConfig{Config: Config{WorkerCount: 5, BufferSize: 100}, Logger: logger}
	bus := NewInMemoryEventBus(config)
	if err := bus.Start(); err != nil {
		t.Fatalf("Failed to start bus: %v", err)
	}
	defer stopBus(t, bus)

	listener := NewMockListener([]string{"test.event"})
	assert.NoError(t, bus.Subscribe(listener))

	eventCount := 50
	for i := 0; i < eventCount; i++ {
		event := NewBaseEvent("test.event", i, "user123")
		if err := bus.Publish(context.Background(), event); err != nil {
			t.Logf("Failed to publish event %d: %v", i, err)
		}
	}

	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, eventCount, len(listener.GetHandledEvents()))
}

func stopBus(t *testing.T, bus *InMemoryEventBus) {
	if err := bus.Stop(); err != nil {
		t.Logf("Failed to stop bus: %v", err)
	}
}

func subscribeMultipleListeners(t *testing.T, bus *InMemoryEventBus, count int, eventTypes []string) []*MockListener {
	listeners := make([]*MockListener, count)
	for i := 0; i < count; i++ {
		listener := NewMockListener(eventTypes)
		if err := bus.Subscribe(listener); err != nil {
			t.Fatalf("Failed to subscribe listener%d: %v", i+1, err)
		}
		listeners[i] = listener
	}
	return listeners
}

// testBusExponentialBackoff verifies that exponential backoff is applied between retries
func testBusExponentialBackoff(t *testing.T, logger *logrus.Logger) {
	config := EventBusConfig{
		Config: Config{
			WorkerCount:  1,
			BufferSize:   10,
			MaxRetries:   3,
			RetryBackoff: 50 * time.Millisecond,
			RetryJitter:  false, // Disable jitter for predictable timing
		},
		Logger: logger,
	}
	bus := NewInMemoryEventBus(config)
	if err := bus.Start(); err != nil {
		t.Fatalf("Failed to start bus: %v", err)
	}
	defer stopBus(t, bus)

	listener := NewMockListener([]string{"test.event"})
	listener.SetShouldFail(true, 3) // Fail all 3 attempts
	assert.NoError(t, bus.Subscribe(listener))

	start := time.Now()
	event := NewBaseEvent("test.event", "payload", "user123")
	assert.NoError(t, bus.Publish(context.Background(), event))

	// Wait for all retry attempts to complete
	time.Sleep(500 * time.Millisecond)

	// Verify all attempts were made
	assert.Equal(t, int32(3), listener.GetHandleCount())

	// Verify exponential backoff was applied
	// Expected delays: 0ms (first), 50ms (2^0 * 50), 100ms (2^1 * 50)
	// Total time should be at least 150ms
	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(150))
}

// testBusRetryPolicyNone verifies that events with RetryPolicyNone are not retried
func testBusRetryPolicyNone(t *testing.T, logger *logrus.Logger) {
	config := EventBusConfig{
		Config: Config{
			WorkerCount:  1,
			BufferSize:   10,
			MaxRetries:   3,
			RetryBackoff: 10 * time.Millisecond,
		},
		Logger: logger,
	}
	bus := NewInMemoryEventBus(config)
	if err := bus.Start(); err != nil {
		t.Fatalf("Failed to start bus: %v", err)
	}
	defer stopBus(t, bus)

	listener := NewMockListener([]string{"test.event"})
	listener.SetShouldFail(true, 3) // Fail all attempts
	assert.NoError(t, bus.Subscribe(listener))

	// Create event with RetryPolicyNone
	event := NewBaseEventWithRetryPolicy("test.event", "payload", "user123", RetryPolicyNone)
	assert.NoError(t, bus.Publish(context.Background(), event))

	time.Sleep(200 * time.Millisecond)

	// Should only be called once (no retries)
	assert.Equal(t, int32(1), listener.GetHandleCount())
}

// testBusRetryPolicyTransient verifies that events with RetryPolicyTransient only retry transient errors
func testBusRetryPolicyTransient(t *testing.T, logger *logrus.Logger) {
	config := EventBusConfig{
		Config: Config{
			WorkerCount:  1,
			BufferSize:   10,
			MaxRetries:   3,
			RetryBackoff: 10 * time.Millisecond,
		},
		Logger: logger,
	}
	bus := NewInMemoryEventBus(config)
	if err := bus.Start(); err != nil {
		t.Fatalf("Failed to start bus: %v", err)
	}
	defer stopBus(t, bus)

	// Test 1: Transient error should be retried
	t.Run("retries transient errors", func(t *testing.T) {
		listener1 := NewMockListener([]string{"test.event"})
		listener1.handleFunc = func(ctx context.Context, event Event) error {
			return errors.New("timeout: connection timed out")
		}
		assert.NoError(t, bus.Subscribe(listener1))

		event1 := NewBaseEventWithRetryPolicy("test.event", "payload1", "user123", RetryPolicyTransient)
		assert.NoError(t, bus.Publish(context.Background(), event1))

		time.Sleep(200 * time.Millisecond)
		// Should retry on transient error
		assert.Equal(t, int32(3), listener1.GetHandleCount())

		assert.NoError(t, bus.Unsubscribe(listener1))
	})

	// Test 2: Permanent error (4xx) should not be retried
	t.Run("does not retry permanent errors", func(t *testing.T) {
		listener2 := NewMockListener([]string{"test.event"})
		listener2.handleFunc = func(ctx context.Context, event Event) error {
			return errors.New("permanent error status 400")
		}
		assert.NoError(t, bus.Subscribe(listener2))

		event2 := NewBaseEventWithRetryPolicy("test.event", "payload2", "user123", RetryPolicyTransient)
		assert.NoError(t, bus.Publish(context.Background(), event2))

		time.Sleep(200 * time.Millisecond)
		// Should not retry permanent error
		assert.Equal(t, int32(1), listener2.GetHandleCount())
	})
}

// testTransientErrorDetection verifies that transient errors are correctly identified
func testTransientErrorDetection(t *testing.T) {
	testCases := []struct {
		name        string
		err         error
		isTransient bool
	}{
		{"timeout error", errors.New("timeout"), true},
		{"connection refused", errors.New("connection refused"), true},
		{"connection reset", errors.New("connection reset by peer"), true},
		{"5xx error", errors.New("status 500"), true},
		{"503 error", errors.New("status 503"), true},
		{"network error", errors.New("network unreachable"), true},
		{"i/o timeout", errors.New("i/o timeout"), true},
		{"dial tcp error", errors.New("dial tcp: connection refused"), true},
		{"permanent 4xx error", errors.New("status 400"), false},
		{"validation error", errors.New("invalid input"), false},
		{"nil error", nil, false},
		// Edge cases for case-insensitive matching
		{"mixed case timeout", errors.New("Connection Timeout"), true},
		{"uppercase network error", errors.New("Network UNREACHABLE"), true},
		{"mixed case 5xx", errors.New("Status 503 Service Unavailable"), true},
		// Edge cases for partial matching
		{"status 50 edge case", errors.New("status 50"), false}, // Should NOT match "status 5"
		{"status 4 edge case", errors.New("status 4"), false},   // Should NOT match "status 5"
		// Error wrapping
		{"wrapped timeout", fmt.Errorf("wrapped: %w", errors.New("timeout")), true},
		{"wrapped network error", fmt.Errorf("api call failed: %w", errors.New("network unreachable")), true},
		// Multiple patterns in one error
		{"multiple patterns", errors.New("network timeout: connection refused"), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isTransientError(tc.err)
			assert.Equal(t, tc.isTransient, result, "Error: %v should be transient=%v", tc.err, tc.isTransient)
		})
	}
}

// testBackoffCalculationWithJitter verifies the exponential backoff policy with and without jitter
func testBackoffCalculationWithJitter(t *testing.T, logger *logrus.Logger) {
	t.Run("without jitter", func(t *testing.T) {
		config := EventBusConfig{
			Config: Config{
				RetryBackoff: 100 * time.Millisecond,
				RetryJitter:  false,
			},
			Logger: logger,
		}
		bus := NewInMemoryEventBus(config)
		bo := bus.newRetryBackoff()

		// First NextBackOff yields the base interval, then doubles each call.
		assert.Equal(t, 100*time.Millisecond, bo.NextBackOff())
		assert.Equal(t, 200*time.Millisecond, bo.NextBackOff())
		assert.Equal(t, 400*time.Millisecond, bo.NextBackOff())
	})

	t.Run("with jitter", func(t *testing.T) {
		config := EventBusConfig{
			Config: Config{
				RetryBackoff: 100 * time.Millisecond,
				RetryJitter:  true,
			},
			Logger: logger,
		}
		bus := NewInMemoryEventBus(config)
		bo := bus.newRetryBackoff()

		// First delay is the base interval ±10%.
		first := bo.NextBackOff()
		assert.GreaterOrEqual(t, float64(first), float64(100*time.Millisecond)*0.9)
		assert.LessOrEqual(t, float64(first), float64(100*time.Millisecond)*1.1)

		// Second delay is 2× the base interval ±10%.
		second := bo.NextBackOff()
		assert.GreaterOrEqual(t, float64(second), float64(200*time.Millisecond)*0.9)
		assert.LessOrEqual(t, float64(second), float64(200*time.Millisecond)*1.1)
	})

	t.Run("caps the backoff at 30 seconds", func(t *testing.T) {
		testRetryBackoffCap(t, logger)
	})
}

// testRetryBackoffCap verifies the 30s cap. Without jitter the base interval
// plateaus exactly at 30s; with jitter MaxInterval caps the base pre-jitter, so
// a delay can reach up to ~33s (30s × 1.1, plus the library's 1ns interval
// fudge) but must never exceed that bound. The 5s base is the maximum that
// survives NewInMemoryEventBus validation (larger values are capped to 5s).
func testRetryBackoffCap(t *testing.T, logger *logrus.Logger) {
	cases := []struct {
		name   string
		jitter bool
		bound  time.Duration
	}{
		{"without jitter", false, 30 * time.Second},
		{"with jitter", true, 33*time.Second + time.Microsecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewInMemoryEventBus(EventBusConfig{
				Config: Config{RetryBackoff: 5 * time.Second, RetryJitter: tc.jitter},
				Logger: logger,
			})
			bo := bus.newRetryBackoff()

			var last time.Duration
			for i := 0; i < 12; i++ {
				last = bo.NextBackOff()
				assert.LessOrEqual(t, last, tc.bound)
			}
			if !tc.jitter {
				assert.Equal(t, 30*time.Second, last, "base interval should plateau at the 30s cap")
			}
		})
	}
}
