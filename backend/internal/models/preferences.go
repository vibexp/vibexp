package models

import (
	"time"
)

// EmailNotificationPreferences contains email notification settings
type EmailNotificationPreferences struct {
	PlatformAnnouncement bool `json:"platform_announcement"`
	AccountSecurity      bool `json:"account_security"`
	NewFeature           bool `json:"new_feature"`
	MarketingPromotional bool `json:"marketing_promotional"`
}

// NotificationTypePreference controls per-type delivery for each channel.
// Email values: "instant" | "digest" | "none"
type NotificationTypePreference struct {
	InApp   bool   `json:"in_app"`
	Email   string `json:"email"`
	WebPush bool   `json:"web_push"`
}

// NotificationChannelPreferences controls global channel on/off switches
type NotificationChannelPreferences struct {
	InApp   bool `json:"in_app"`
	Email   bool `json:"email"`
	WebPush bool `json:"web_push"`
}

// NotificationPreferences aggregates channel and per-type preferences
type NotificationPreferences struct {
	Channels NotificationChannelPreferences        `json:"channels"`
	Types    map[string]NotificationTypePreference `json:"types"`
}

// Preferences contains all user preference categories
type Preferences struct {
	EmailNotification EmailNotificationPreferences `json:"email_notification"`
	Notifications     NotificationPreferences      `json:"notifications"`
}

// UserPreferences represents the user preferences entity stored in the database
type UserPreferences struct {
	ID          string      `json:"id" db:"id"`
	UserID      string      `json:"user_id" db:"user_id"`
	Preferences Preferences `json:"preferences" db:"preferences"`
	CreatedAt   time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at" db:"updated_at"`
	Version     int64       `json:"version" db:"version"`
}

// UpdatePreferencesRequest represents the request to update user preferences
type UpdatePreferencesRequest struct {
	EmailNotification *EmailNotificationPreferences `json:"email_notification,omitempty"`
	Notifications     *NotificationPreferences      `json:"notifications,omitempty"`
}

// PreferencesResponse represents the API response for user preferences
type PreferencesResponse struct {
	Preferences Preferences `json:"preferences"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// DefaultNotificationPreferences returns sensible default notification preferences
func DefaultNotificationPreferences() NotificationPreferences {
	return NotificationPreferences{
		Channels: NotificationChannelPreferences{
			InApp:   true,
			Email:   true,
			WebPush: false,
		},
		Types: map[string]NotificationTypePreference{
			"feed.item.created": {
				InApp:   true,
				Email:   "digest",
				WebPush: true,
			},
			"feed.reply.created": {
				InApp:   true,
				Email:   "instant",
				WebPush: true,
			},
			"team.invitation": {
				InApp:   true,
				Email:   "instant",
				WebPush: false,
			},
		},
	}
}

// DefaultPreferences returns the default preferences for a new user
func DefaultPreferences() Preferences {
	return Preferences{
		EmailNotification: EmailNotificationPreferences{
			PlatformAnnouncement: true,
			AccountSecurity:      true,
			NewFeature:           true,
			MarketingPromotional: false,
		},
		Notifications: DefaultNotificationPreferences(),
	}
}
