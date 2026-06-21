package models

import "time"

// WebhookEvent represents a processed webhook event for idempotency tracking
type WebhookEvent struct {
	ID          string    `json:"id" db:"id"`
	EventID     string    `json:"event_id" db:"event_id"`         // Stripe event ID (evt_xxx)
	EventType   string    `json:"event_type" db:"event_type"`     // Stripe event type
	ProcessedAt time.Time `json:"processed_at" db:"processed_at"` // When processed
	TeamID      *string   `json:"team_id,omitempty" db:"team_id"` // Associated team (nullable)
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}
