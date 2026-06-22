package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"cloud.google.com/go/pubsub" //nolint:staticcheck // v2 has breaking changes, will upgrade when stable

	"github.com/vibexp/vibexp/internal/logging"
)

// PubSubForwarderListener forwards events to Google Cloud Pub/Sub
type PubSubForwarderListener struct {
	client       *pubsub.Client
	topic        *pubsub.Topic
	eventTypes   []string
	logger       *slog.Logger
	publishAsync bool
}

// PubSubForwarderConfig holds configuration for the forwarder
type PubSubForwarderConfig struct {
	Client       *pubsub.Client
	TopicName    string
	EventTypes   []string
	Logger       *slog.Logger
	PublishAsync bool // If true, don't wait for publish result
}

// NewPubSubForwarderListener creates a new Pub/Sub forwarder listener
func NewPubSubForwarderListener(config PubSubForwarderConfig) (*PubSubForwarderListener, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("pubsub client is required")
	}
	if config.TopicName == "" {
		return nil, fmt.Errorf("topic name is required")
	}
	if config.Logger == nil {
		config.Logger = logging.New(logging.Config{})
	}

	topic := config.Client.Topic(config.TopicName)

	// Configure topic settings for better reliability
	topic.PublishSettings = pubsub.PublishSettings{
		ByteThreshold:  5000, // 5KB
		CountThreshold: 100,  // 100 messages
		DelayThreshold: 100 * time.Millisecond,
		Timeout:        60 * time.Second,
	}

	return &PubSubForwarderListener{
		client:       config.Client,
		topic:        topic,
		eventTypes:   config.EventTypes,
		logger:       config.Logger,
		publishAsync: config.PublishAsync,
	}, nil
}

// Handle processes an event by forwarding it to Pub/Sub
func (l *PubSubForwarderListener) Handle(ctx context.Context, event Event) error {
	startTime := time.Now()

	data, err := l.serializeEvent(event)
	if err != nil {
		return err
	}

	msg := l.createPubSubMessage(event, data)
	result := l.topic.Publish(ctx, msg)

	if l.publishAsync {
		l.logAsyncPublish(event)
		return nil
	}

	return l.waitForPublishResult(ctx, event, result, startTime)
}

// serializeEvent serializes an event to JSON
func (l *PubSubForwarderListener) serializeEvent(event Event) ([]byte, error) {
	data, err := json.Marshal(map[string]interface{}{
		"type":      event.Type(),
		"payload":   event.Payload(),
		"timestamp": event.Timestamp(),
		"user_id":   event.UserID(),
	})
	if err != nil {
		l.logger.With(
			"service", "vibexp-api",
			"component", "pubsub-forwarder",
			"event_type", event.Type(),
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to marshal event for Pub/Sub")
		return nil, fmt.Errorf("failed to marshal event: %w", err)
	}
	return data, nil
}

// createPubSubMessage creates a Pub/Sub message from event data
func (l *PubSubForwarderListener) createPubSubMessage(event Event, data []byte) *pubsub.Message {
	return &pubsub.Message{
		Data: data,
		Attributes: map[string]string{
			"eventType":  event.Type(),
			"event_type": event.Type(),
			"user_id":    event.UserID(),
			"timestamp":  event.Timestamp().Format(time.RFC3339),
		},
	}
}

// logAsyncPublish logs when an event is queued for async publishing
func (l *PubSubForwarderListener) logAsyncPublish(event Event) {
	l.logger.With(
		"service", "vibexp-api",
		"component", "pubsub-forwarder",
		"event_type", event.Type(),
		"user_id", event.UserID(),
	).Debug("Event queued for Pub/Sub (async)")
}

// waitForPublishResult waits for the publish result and logs the outcome
func (l *PubSubForwarderListener) waitForPublishResult(
	ctx context.Context, event Event, result *pubsub.PublishResult, startTime time.Time,
) error {
	serverID, err := result.Get(ctx)
	duration := time.Since(startTime)

	if err != nil {
		l.logPublishFailure(event, err, duration)
		return nil
	}

	l.logPublishSuccess(event, serverID, duration)
	return nil
}

// logPublishFailure logs when publish fails
func (l *PubSubForwarderListener) logPublishFailure(event Event, err error, duration time.Duration) {
	l.logger.With(
		"service", "vibexp-api",
		"component", "pubsub-forwarder",
		"event_type", event.Type(),
		"user_id", event.UserID(),
		"error", fmt.Sprintf("%+v", err),
		"duration_ms", duration.Milliseconds(),
	).Error("Failed to publish event to Pub/Sub")
}

// logPublishSuccess logs when publish succeeds
func (l *PubSubForwarderListener) logPublishSuccess(event Event, serverID string, duration time.Duration) {
	l.logger.With(
		"service", "vibexp-api",
		"component", "pubsub-forwarder",
		"event_type", event.Type(),
		"user_id", event.UserID(),
		"server_id", serverID,
		"duration_ms", duration.Milliseconds(),
	).Info("Event forwarded to Pub/Sub successfully")
}

// EventTypes returns the event types this listener handles
func (l *PubSubForwarderListener) EventTypes() []string {
	return l.eventTypes
}

// Close stops the forwarder and flushes pending messages
func (l *PubSubForwarderListener) Close() error {
	l.logger.With(
		"service", "vibexp-api",
		"component", "pubsub-forwarder",
	).Info("Stopping Pub/Sub forwarder, flushing pending messages")

	// Stop the topic to flush pending messages
	l.topic.Stop()

	l.logger.With(
		"service", "vibexp-api",
		"component", "pubsub-forwarder",
	).Info("Pub/Sub forwarder stopped")

	return nil
}
