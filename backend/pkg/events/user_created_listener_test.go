package events

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUserCreatedListener(t *testing.T) {
	logger := logrus.New()
	listener := NewUserCreatedListener(logger)

	assert.NotNil(t, listener)
	assert.NotNil(t, listener.logger)
}

func TestNewUserCreatedListener_WithNilLogger(t *testing.T) {
	listener := NewUserCreatedListener(nil)

	assert.NotNil(t, listener)
	assert.NotNil(t, listener.logger)
}

func TestUserCreatedListener_Handle(t *testing.T) {
	logger := logrus.New()
	listener := NewUserCreatedListener(logger)

	// Create a test event
	event := NewUserCreatedEvent("user-123", "test@example.com", "Test User", time.Now())

	// Handle the event
	err := listener.Handle(context.Background(), event)

	require.NoError(t, err)
}

func TestUserCreatedListener_EventTypes(t *testing.T) {
	logger := logrus.New()
	listener := NewUserCreatedListener(logger)

	eventTypes := listener.EventTypes()

	assert.Len(t, eventTypes, 1)
	assert.Contains(t, eventTypes, EventTypeUserCreated)
}
