package events

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEventManager(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	t.Run("NewEventManager creates manager", func(t *testing.T) {
		testManagerCreation(t, logger)
	})

	t.Run("EventManager starts and stops", func(t *testing.T) {
		testManagerStartStop(t, logger)
	})

	t.Run("EventManager cannot start twice", func(t *testing.T) {
		testManagerCannotStartTwice(t, logger)
	})

	t.Run("EventManager cannot stop if not started", func(t *testing.T) {
		testManagerCannotStopIfNotStarted(t, logger)
	})

	t.Run("EventManager publishes events", func(t *testing.T) {
		testManagerPublishEvents(t, logger)
	})

	t.Run("EventManager does not publish when not started", func(t *testing.T) {
		testManagerNotStarted(t, logger)
	})

	t.Run("EventManager subscribes and unsubscribes listeners", func(t *testing.T) {
		testManagerSubscribeUnsubscribe(t, logger)
	})

	t.Run("EventManager handles multiple event types", func(t *testing.T) {
		testManagerMultipleEventTypes(t, logger)
	})
}

func testManagerCreation(t *testing.T, logger *slog.Logger) {
	config := EventBusConfig{Config: Config{WorkerCount: 5, BufferSize: 50}, Logger: logger}
	manager := NewEventManager(config)

	assert.NotNil(t, manager)
	assert.NotNil(t, manager.bus)
	assert.NotNil(t, manager.logger)
	assert.False(t, manager.started)
}

func testManagerStartStop(t *testing.T, logger *slog.Logger) {
	config := EventBusConfig{Logger: logger}
	manager := NewEventManager(config)

	err := manager.Start()
	assert.NoError(t, err)
	assert.True(t, manager.started)

	err = manager.Stop()
	assert.NoError(t, err)
	assert.False(t, manager.started)
}

func testManagerCannotStartTwice(t *testing.T, logger *slog.Logger) {
	config := EventBusConfig{Logger: logger}
	manager := NewEventManager(config)

	err := manager.Start()
	assert.NoError(t, err)

	err = manager.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")

	if err := manager.Stop(); err != nil {
		t.Logf("Failed to stop manager: %v", err)
	}
}

func testManagerCannotStopIfNotStarted(t *testing.T, logger *slog.Logger) {
	config := EventBusConfig{Logger: logger}
	manager := NewEventManager(config)

	err := manager.Stop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func testManagerPublishEvents(t *testing.T, logger *slog.Logger) {
	config := EventBusConfig{Config: Config{WorkerCount: 2, BufferSize: 10}, Logger: logger}
	manager := NewEventManager(config)
	if err := manager.Start(); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}
	defer stopManager(t, manager)

	listener := NewMockListener([]string{"test.event"})
	assert.NoError(t, manager.Subscribe(listener))

	event := NewBaseEvent("test.event", "payload", "user123")
	assert.NoError(t, manager.Publish(context.Background(), event))

	time.Sleep(100 * time.Millisecond)

	handledEvents := listener.GetHandledEvents()
	assert.Equal(t, 1, len(handledEvents))
}

func testManagerNotStarted(t *testing.T, logger *slog.Logger) {
	config := EventBusConfig{Logger: logger}
	manager := NewEventManager(config)

	event := NewBaseEvent("test.event", "payload", "user123")
	err := manager.Publish(context.Background(), event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func testManagerSubscribeUnsubscribe(t *testing.T, logger *slog.Logger) {
	config := EventBusConfig{Config: Config{WorkerCount: 2, BufferSize: 10}, Logger: logger}
	manager := NewEventManager(config)
	if err := manager.Start(); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}
	defer stopManager(t, manager)

	listener1 := NewMockListener([]string{"test.event"})
	listener2 := NewMockListener([]string{"test.event"})

	assert.NoError(t, manager.Subscribe(listener1))
	assert.NoError(t, manager.Subscribe(listener2))

	event1 := NewBaseEvent("test.event", "payload1", "user123")
	assert.NoError(t, manager.Publish(context.Background(), event1))
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, len(listener1.GetHandledEvents()))
	assert.Equal(t, 1, len(listener2.GetHandledEvents()))

	assert.NoError(t, manager.Unsubscribe(listener1))

	event2 := NewBaseEvent("test.event", "payload2", "user123")
	assert.NoError(t, manager.Publish(context.Background(), event2))
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, len(listener1.GetHandledEvents()))
	assert.Equal(t, 2, len(listener2.GetHandledEvents()))
}

func testManagerMultipleEventTypes(t *testing.T, logger *slog.Logger) {
	config := EventBusConfig{Config: Config{WorkerCount: 3, BufferSize: 20}, Logger: logger}
	manager := NewEventManager(config)
	if err := manager.Start(); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}
	defer stopManager(t, manager)

	userListener := NewMockListener([]string{EventTypeUserCreated})
	promptListener := NewMockListener([]string{EventTypePromptCreated})
	multiListener := NewMockListener([]string{EventTypeUserCreated, EventTypePromptCreated})

	assert.NoError(t, manager.Subscribe(userListener))
	assert.NoError(t, manager.Subscribe(promptListener))
	assert.NoError(t, manager.Subscribe(multiListener))

	userEvent := NewUserCreatedEvent("user123", "test@example.com", "Test User", time.Now())
	assert.NoError(t, manager.Publish(context.Background(), userEvent))

	promptEvent := NewPromptCreatedEvent(
		"prompt123", "user123", "test@example.com", "project", "slug", "title", "Test prompt body", time.Now(),
	)
	assert.NoError(t, manager.Publish(context.Background(), promptEvent))

	time.Sleep(200 * time.Millisecond)

	assert.Equal(t, 1, len(userListener.GetHandledEvents()))
	assert.Equal(t, 1, len(promptListener.GetHandledEvents()))
	assert.Equal(t, 2, len(multiListener.GetHandledEvents()))

	assert.Equal(t, EventTypeUserCreated, userListener.GetHandledEvents()[0].Type())
	assert.Equal(t, EventTypePromptCreated, promptListener.GetHandledEvents()[0].Type())
}

func stopManager(t *testing.T, manager *EventManager) {
	if err := manager.Stop(); err != nil {
		t.Logf("Failed to stop manager: %v", err)
	}
}
