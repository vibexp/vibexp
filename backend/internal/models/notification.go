package models

import (
	"encoding/json"
	"time"
)

// Notification represents a persisted notification record
type Notification struct {
	ID              string          `json:"id" db:"id"`
	RecipientUserID string          `json:"recipient_user_id" db:"recipient_user_id"`
	TeamID          string          `json:"team_id,omitempty" db:"team_id"`
	Type            string          `json:"type" db:"type"`
	Category        string          `json:"category" db:"category"`
	Title           string          `json:"title" db:"title"`
	Body            string          `json:"body,omitempty" db:"body"`
	ActionURL       string          `json:"action_url,omitempty" db:"action_url"`
	EntityRef       json.RawMessage `json:"entity_ref,omitempty" db:"entity_ref"`
	DedupeKey       string          `json:"dedupe_key,omitempty" db:"dedupe_key"`
	ReadAt          *time.Time      `json:"read_at,omitempty" db:"read_at"`
	DismissedAt     *time.Time      `json:"dismissed_at,omitempty" db:"dismissed_at"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
}

// NotificationDelivery represents the delivery record for a notification channel
type NotificationDelivery struct {
	ID             string     `json:"id" db:"id"`
	NotificationID string     `json:"notification_id" db:"notification_id"`
	Channel        string     `json:"channel" db:"channel"`
	Status         string     `json:"status" db:"status"`
	Reason         string     `json:"reason,omitempty" db:"reason"`
	Attempts       int        `json:"attempts" db:"attempts"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty" db:"delivered_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
}
