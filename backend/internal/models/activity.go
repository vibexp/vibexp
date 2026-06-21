package models

import (
	"time"
)

// Activity represents a comprehensive activity record
type Activity struct {
	ID           string                 `json:"id" db:"id"`
	UserID       string                 `json:"user_id" db:"user_id"`
	ActivityType string                 `json:"activity_type" db:"activity_type"`
	EntityType   string                 `json:"entity_type" db:"entity_type"`
	EntityID     *string                `json:"entity_id,omitempty" db:"entity_id"`
	SessionID    *string                `json:"session_id,omitempty" db:"session_id"`
	Description  string                 `json:"description" db:"description"`
	Metadata     map[string]interface{} `json:"metadata" db:"metadata"`
	SourceIP     *string                `json:"source_ip,omitempty" db:"source_ip"`
	UserAgent    *string                `json:"user_agent,omitempty" db:"user_agent"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
}

// CreateActivityRequest represents the request to create a new activity
type CreateActivityRequest struct {
	ActivityType string                 `json:"activity_type" validate:"required,min=1,max=50"`
	EntityType   string                 `json:"entity_type" validate:"required,min=1,max=50"`
	EntityID     *string                `json:"entity_id,omitempty" validate:"omitempty,max=255"`
	SessionID    *string                `json:"session_id,omitempty" validate:"omitempty,max=255"`
	Description  string                 `json:"description" validate:"required,min=10,max=1000"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	SourceIP     *string                `json:"source_ip,omitempty"`
	UserAgent    *string                `json:"user_agent,omitempty"`
}

// ActivityListResponse represents paginated activity response
type ActivityListResponse struct {
	Activities []Activity `json:"activities"`
	TotalCount int        `json:"total_count"`
	Page       int        `json:"page"`
	PerPage    int        `json:"per_page"`
	TotalPages int        `json:"total_pages"`
}

// ActivityStatsResponse represents activity statistics
type ActivityStatsResponse struct {
	TotalActivities      int                   `json:"total_activities"`
	ActivitiesToday      int                   `json:"activities_today"`
	ActivitiesThisWeek   int                   `json:"activities_this_week"`
	TopActivityTypes     []ActivityTypeCount   `json:"top_activity_types"`
	TopEntityTypes       []EntityTypeCount     `json:"top_entity_types"`
	RecentActivities     []Activity            `json:"recent_activities"`
	ActivitiesByDateWeek []ActivityCountByDate `json:"activities_by_date_week"`
}

// ActivityTypeCount represents activity type usage statistics
type ActivityTypeCount struct {
	ActivityType string `json:"activity_type"`
	Count        int    `json:"count"`
}

// EntityTypeCount represents entity type usage statistics
type EntityTypeCount struct {
	EntityType string `json:"entity_type"`
	Count      int    `json:"count"`
}

// ActivityCountByDate represents activity count for a specific date
type ActivityCountByDate struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}
