package models

import "time"

// DeviceToken represents a registered push notification device token
type DeviceToken struct {
	ID         string    `db:"id"`
	UserID     string    `db:"user_id"`
	Token      string    `db:"token"`
	Platform   string    `db:"platform"`
	UserAgent  string    `db:"user_agent"`
	LastUsedAt time.Time `db:"last_used_at"`
	CreatedAt  time.Time `db:"created_at"`
}
