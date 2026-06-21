// Email notification preferences
export interface EmailNotificationPreferences {
  platform_announcement: boolean
  account_security: boolean
  new_feature: boolean
  marketing_promotional: boolean
}

// Per-type delivery settings for each channel.
// `email` is optional because data persisted before the digest feature may omit it.
export interface NotificationTypePreference {
  in_app: boolean
  email?: string
  web_push: boolean
}

// Global channel on/off switches
export interface NotificationChannelPreferences {
  in_app: boolean
  email: boolean
  web_push: boolean
}

// Aggregated notification preferences: global channel switches + per-type delivery.
// `types` is optional because older backend data may omit it when only `channels` was stored.
export interface NotificationPreferences {
  channels: NotificationChannelPreferences
  types?: Record<string, NotificationTypePreference>
}

// All preferences categories
export interface Preferences {
  email_notification: EmailNotificationPreferences
  notifications?: NotificationPreferences
}

// API response for preferences
export interface PreferencesResponse {
  preferences: Preferences
  updated_at?: string
}

// Request to update preferences
export interface UpdatePreferencesRequest {
  email_notification?: EmailNotificationPreferences
  notifications?: NotificationPreferences
}
