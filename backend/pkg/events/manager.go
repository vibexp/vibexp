package events

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// EventPublisher defines the interface for publishing events
type EventPublisher interface {
	Publish(ctx context.Context, event Event) error
}

// EventManager manages the event bus lifecycle and provides a central interface
type EventManager struct {
	bus     EventBus
	logger  *slog.Logger
	mu      sync.RWMutex
	started bool
}

// NewEventManager creates a new event manager
func NewEventManager(config EventBusConfig) *EventManager {
	return &EventManager{
		bus:    NewInMemoryEventBus(config),
		logger: config.Logger,
	}
}

// Start starts the event manager
func (m *EventManager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("event manager already started")
	}

	if err := m.bus.Start(); err != nil {
		return fmt.Errorf("failed to start event bus: %w", err)
	}

	m.started = true
	m.logger.Info("Event manager started")
	return nil
}

// Stop stops the event manager
func (m *EventManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return fmt.Errorf("event manager not started")
	}

	if err := m.bus.Stop(); err != nil {
		return fmt.Errorf("failed to stop event bus: %w", err)
	}

	m.started = false
	m.logger.Info("Event manager stopped")
	return nil
}

// Publish publishes an event
func (m *EventManager) Publish(ctx context.Context, event Event) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.started {
		return fmt.Errorf("event manager not started")
	}

	return m.bus.Publish(ctx, event)
}

// Subscribe subscribes a listener to the event bus
func (m *EventManager) Subscribe(listener EventListener) error {
	return m.bus.Subscribe(listener)
}

// Unsubscribe unsubscribes a listener from the event bus
func (m *EventManager) Unsubscribe(listener EventListener) error {
	return m.bus.Unsubscribe(listener)
}
