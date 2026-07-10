package events

import "context"

// EventListener defines the interface for event listeners
type EventListener interface {
	// Handle processes an event
	Handle(ctx context.Context, event Event) error

	// EventTypes returns the list of event types this listener handles
	EventTypes() []string
}

// ConcurrencyManagedListener is an EventListener that runs its work on its own
// bounded worker goroutines. When ManagesOwnConcurrency reports true, the bus
// invokes the listener inline on the dispatch goroutine instead of through the
// shared worker pool, so its handling never rides the pool's unbounded
// `go task()` saturation fallback (#142). Such a listener's Handle MUST be fast
// and non-blocking (enqueue only) and owns its own retry policy — the bus applies
// none. A listener that returns false is dispatched normally (via the pool).
type ConcurrencyManagedListener interface {
	EventListener

	// ManagesOwnConcurrency reports whether this listener bounds its own
	// concurrency and must bypass the shared worker pool.
	ManagesOwnConcurrency() bool
}

// NoOpListener is a listener that does nothing (for initial implementation)
type NoOpListener struct {
	eventTypes []string
}

// NewNoOpListener creates a new no-op listener
func NewNoOpListener(eventTypes ...string) *NoOpListener {
	return &NoOpListener{
		eventTypes: eventTypes,
	}
}

// Handle does nothing and returns nil
func (l *NoOpListener) Handle(ctx context.Context, event Event) error {
	// No-op: do nothing
	return nil
}

// EventTypes returns the event types this listener handles
func (l *NoOpListener) EventTypes() []string {
	return l.eventTypes
}
