package models

import "time"

// NotificationDigestQueueRow represents a row in the notification_digest_queue table.
// Rows are enqueued when a user's email preference for a notification type is "digest".
// The DigestRunner reads pending rows, groups them by user, and sends one summary email.
type NotificationDigestQueueRow struct {
	ID             string     `json:"id" db:"id"`
	UserID         string     `json:"user_id" db:"user_id"`
	NotificationID string     `json:"notification_id" db:"notification_id"`
	ScheduledFor   time.Time  `json:"scheduled_for" db:"scheduled_for"`
	SentAt         *time.Time `json:"sent_at,omitempty" db:"sent_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
}
