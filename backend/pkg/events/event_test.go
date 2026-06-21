package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBaseEvent(t *testing.T) {
	t.Run("NewBaseEvent creates event with correct fields", func(t *testing.T) {
		eventType := "test.event"
		payload := map[string]string{"key": "value"}
		userID := "user123"

		event := NewBaseEvent(eventType, payload, userID)

		assert.NotNil(t, event)
		assert.Equal(t, eventType, event.Type())
		assert.Equal(t, payload, event.Payload())
		assert.Equal(t, userID, event.UserID())
		assert.False(t, event.Timestamp().IsZero())
		assert.True(t, time.Since(event.Timestamp()) < time.Second)
	})

	t.Run("BaseEvent implements Event interface", func(t *testing.T) {
		event := NewBaseEvent("test.event", nil, "user123")
		var _ Event = event // Verify it implements Event interface
	})
}
