package events

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingListener is a test EventListener that always fails, optionally
// declaring itself concurrency-managed.
type countingListener struct {
	calls      atomic.Int32
	managed    bool
	eventTypes []string
}

func (l *countingListener) Handle(_ context.Context, _ Event) error {
	l.calls.Add(1)
	return errors.New("always fails")
}
func (l *countingListener) EventTypes() []string        { return l.eventTypes }
func (l *countingListener) ManagesOwnConcurrency() bool { return l.managed }

func newTestBus(t *testing.T) *InMemoryEventBus {
	t.Helper()
	bus := NewInMemoryEventBus(EventBusConfig{
		Config: Config{WorkerCount: 2, BufferSize: 10, MaxRetries: 3, RetryBackoff: time.Millisecond},
		Logger: slog.New(slog.DiscardHandler),
	})
	require.NoError(t, bus.Start())
	t.Cleanup(func() { require.NoError(t, bus.Stop()) })
	return bus
}

// A concurrency-managed listener is invoked inline and the bus applies no retry:
// even a persistently failing Handle is called exactly once (it owns its own
// retry), proving it never rides the shared pool's retry/saturation path.
func TestBus_ConcurrencyManagedListener_DispatchedInlineWithoutRetry(t *testing.T) {
	bus := newTestBus(t)
	listener := &countingListener{managed: true, eventTypes: []string{"test.event"}}
	require.NoError(t, bus.Subscribe(listener))

	event := NewBaseEventWithRetryPolicy("test.event", "payload", "user123", RetryPolicyDefault)
	require.NoError(t, bus.Publish(context.Background(), event))

	require.Eventually(t, func() bool { return listener.calls.Load() == 1 }, 2*time.Second, 2*time.Millisecond)
	time.Sleep(50 * time.Millisecond) // settle: ensure no further (retry) calls arrive
	assert.Equal(t, int32(1), listener.calls.Load(), "managed listener is not retried by the bus")
}

// A normal listener still rides the worker pool with bus-level retry, confirming
// the inline branch is scoped to concurrency-managed listeners only (no behavior
// change for everything else).
func TestBus_NormalListener_StillRetriesViaPool(t *testing.T) {
	bus := newTestBus(t)
	listener := &countingListener{managed: false, eventTypes: []string{"test.event"}}
	require.NoError(t, bus.Subscribe(listener))

	event := NewBaseEventWithRetryPolicy("test.event", "payload", "user123", RetryPolicyDefault)
	require.NoError(t, bus.Publish(context.Background(), event))

	// MaxRetries == 3, so a persistently failing normal listener is called 3 times.
	require.Eventually(t, func() bool { return listener.calls.Load() == 3 }, 2*time.Second, 2*time.Millisecond)
}

var _ ConcurrencyManagedListener = (*countingListener)(nil)
