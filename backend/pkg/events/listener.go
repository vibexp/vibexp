package events

import "context"

// EventListener defines the interface for event listeners
type EventListener interface {
	// Handle processes an event
	Handle(ctx context.Context, event Event) error

	// EventTypes returns the list of event types this listener handles
	EventTypes() []string
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
