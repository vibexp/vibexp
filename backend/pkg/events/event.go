package events

import (
	"time"
)

// RetryPolicy defines the retry behavior for event handling
type RetryPolicy string

const (
	// RetryPolicyDefault enables retries with exponential backoff for all errors
	RetryPolicyDefault RetryPolicy = "default"

	// RetryPolicyTransient enables retries only for transient errors (network, timeout, 5xx)
	RetryPolicyTransient RetryPolicy = "transient"

	// RetryPolicyNone disables all retries
	RetryPolicyNone RetryPolicy = "none"
)

// Event represents a domain event in the system
type Event interface {
	// Type returns the event type (e.g., "user.created", "prompt.updated")
	Type() string

	// Payload returns the event payload
	Payload() interface{}

	// Timestamp returns when the event was created
	Timestamp() time.Time

	// UserID returns the ID of the user associated with this event
	UserID() string

	// RetryPolicy returns the retry policy for this event
	RetryPolicy() RetryPolicy
}

// BaseEvent implements common Event interface methods
type BaseEvent struct {
	EventType        string
	EventPayload     interface{}
	EventTimestamp   time.Time
	EventUserID      string
	EventRetryPolicy RetryPolicy

	// backfillOrigin marks events republished by the embeddings backfill tool so
	// side-effect listeners (CRM, notifications) can skip them while the embedding
	// pipeline still processes them. It is intentionally unexported: it is an
	// in-process routing hint, not part of the wire payload, so it never leaks into
	// the Pub/Sub message the forwarder serializes. Read/write it through the
	// package helpers IsBackfillOrigin / MarkBackfillOrigin.
	backfillOrigin bool
}

// Type returns the event type
func (e *BaseEvent) Type() string {
	return e.EventType
}

// Payload returns the event payload
func (e *BaseEvent) Payload() interface{} {
	return e.EventPayload
}

// Timestamp returns the event timestamp
func (e *BaseEvent) Timestamp() time.Time {
	return e.EventTimestamp
}

// UserID returns the user ID
func (e *BaseEvent) UserID() string {
	return e.EventUserID
}

// RetryPolicy returns the retry policy
func (e *BaseEvent) RetryPolicy() RetryPolicy {
	if e.EventRetryPolicy == "" {
		return RetryPolicyDefault // default to retry
	}
	return e.EventRetryPolicy
}

// NewBaseEvent creates a new base event with default retry policy
func NewBaseEvent(eventType string, payload interface{}, userID string) *BaseEvent {
	return &BaseEvent{
		EventType:        eventType,
		EventPayload:     payload,
		EventTimestamp:   time.Now(),
		EventUserID:      userID,
		EventRetryPolicy: RetryPolicyDefault,
	}
}

// NewBaseEventWithRetryPolicy creates a new base event with custom retry policy
func NewBaseEventWithRetryPolicy(
	eventType string, payload interface{}, userID string, retryPolicy RetryPolicy,
) *BaseEvent {
	return &BaseEvent{
		EventType:        eventType,
		EventPayload:     payload,
		EventTimestamp:   time.Now(),
		EventUserID:      userID,
		EventRetryPolicy: retryPolicy,
	}
}
