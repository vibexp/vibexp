package notifications

import "time"

// NotificationType identifies the semantic type of a notification
type NotificationType string

// Category classifies the urgency/priority of a notification
type Category string

// ChannelName identifies a delivery channel
type ChannelName string

// DeliveryStatus represents the outcome of a delivery attempt
type DeliveryStatus string

const (
	// ChannelInApp is the in-application notification channel
	ChannelInApp ChannelName = "in_app"
	// ChannelEmail is the email notification channel
	ChannelEmail ChannelName = "email"
	// ChannelWebPush is the browser / OS-level push notification channel via FCM
	ChannelWebPush ChannelName = "web_push"
)

const (
	// StatusQueued means delivery is deferred (e.g. digest queue)
	StatusQueued DeliveryStatus = "queued"
	// StatusSent means delivery was successful
	StatusSent DeliveryStatus = "sent"
	// StatusFailed means delivery failed after exhausting attempts
	StatusFailed DeliveryStatus = "failed"
	// StatusSkipped means delivery was intentionally skipped (e.g. preference = none)
	StatusSkipped DeliveryStatus = "skipped"
)

const (
	// CategoryHigh is for urgent/high-priority notifications
	CategoryHigh Category = "high"
	// CategoryLow is for informational/low-priority notifications
	CategoryLow Category = "low"
)

// SendRequest holds all data needed to create and dispatch a notification
type SendRequest struct {
	// RecipientUserID is the user who will receive the notification
	RecipientUserID string
	// TeamID is the team context (optional)
	TeamID string
	// Type is the semantic type (e.g. "feed.item.created")
	Type NotificationType
	// Category is the urgency level
	Category Category
	// Title is the short notification headline (in-app)
	Title string
	// Body is the optional longer description (in-app, plain text)
	Body string
	// ActionURL is the optional deep-link URL
	ActionURL string
	// EntityRef is optional structured metadata about the related entity
	EntityRef map[string]interface{}
	// DedupeKey prevents duplicate notifications when set; insert is silently skipped on conflict
	DedupeKey string
	// RenderedEmailSubject is the pre-rendered email subject line (auto-escaped via html/template)
	RenderedEmailSubject string
	// RenderedEmailHTML is the pre-rendered HTML email body (auto-escaped via html/template)
	RenderedEmailHTML string
}

// Notification is the domain representation of a stored notification
type Notification struct {
	ID              string                 `json:"id"`
	RecipientUserID string                 `json:"-"`
	TeamID          string                 `json:"team_id,omitempty"`
	Type            NotificationType       `json:"type"`
	Category        Category               `json:"category"`
	Title           string                 `json:"title"`
	Body            string                 `json:"body,omitempty"`
	ActionURL       string                 `json:"action_url,omitempty"`
	EntityRef       map[string]interface{} `json:"entity_ref,omitempty"`
	// DedupeKey is an internal field and must not be exposed in API responses
	DedupeKey   string     `json:"-"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
	DismissedAt *time.Time `json:"dismissed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	// RenderedEmailSubject and RenderedEmailHTML are transient fields set during dispatch;
	// they are rendered by html/template (auto-escaped) and not persisted.
	RenderedEmailSubject string `json:"-"`
	RenderedEmailHTML    string `json:"-"`
}

// ListFilters controls pagination and filtering for notification listing
type ListFilters struct {
	UnreadOnly bool
	Limit      int
	Offset     int
}

// DeliveryResult captures the outcome of delivering to a single channel
type DeliveryResult struct {
	Status DeliveryStatus
	Reason string
}
