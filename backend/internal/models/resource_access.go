package models

import (
	"time"
)

// ResourceAccessEvent represents a single detail-access event for a team resource.
type ResourceAccessEvent struct {
	ID           string    `json:"id" db:"id"`
	TeamID       string    `json:"team_id" db:"team_id"`
	UserID       *string   `json:"user_id,omitempty" db:"user_id"`
	ResourceType string    `json:"resource_type" db:"resource_type"`
	ResourceID   string    `json:"resource_id" db:"resource_id"`
	Source       string    `json:"source" db:"source"`
	APIKeyID     *string   `json:"api_key_id,omitempty" db:"api_key_id"`
	UserAgent    *string   `json:"user_agent,omitempty" db:"user_agent"`
	SourceIP     *string   `json:"source_ip,omitempty" db:"source_ip"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// DailyAccessCount represents the raw count of access events for a single day, grouped by source.
// Downstream consumers pivot and zero-fill this into a per-source timeseries.
type DailyAccessCount struct {
	Date   string `json:"date"`
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// TopAccessedResource is one row of a team's most-accessed resources ranking over
// a time window. Name is the owning resource's display name (prompt/project name,
// artifact/blueprint title, or a truncated memory text), resolved by the repository
// so the frontend can render and deep-link each row without N extra calls. Name is
// empty when the resource type has no name table or the resource no longer exists.
type TopAccessedResource struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Name         string `json:"name"`
	AccessCount  int    `json:"access_count"`
}
