package events

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoOpListener(t *testing.T) {
	t.Run("NewNoOpListener creates listener with event types", func(t *testing.T) {
		eventTypes := []string{"test.event1", "test.event2"}
		listener := NewNoOpListener(eventTypes...)

		assert.NotNil(t, listener)
		assert.Equal(t, eventTypes, listener.EventTypes())
	})

	t.Run("NoOpListener Handle returns nil", func(t *testing.T) {
		listener := NewNoOpListener("test.event")
		event := NewBaseEvent("test.event", nil, "user123")

		err := listener.Handle(context.Background(), event)
		assert.NoError(t, err)
	})

	t.Run("NoOpListener implements EventListener interface", func(t *testing.T) {
		listener := NewNoOpListener("test.event")
		var _ EventListener = listener // Verify it implements EventListener interface
	})
}
