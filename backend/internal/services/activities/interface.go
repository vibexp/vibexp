package activities

import (
	"context"
)

// ActivityTracker defines the interface for recording activities
type ActivityTracker interface {
	// RecordActivity records a new activity
	RecordActivity(ctx context.Context, userID string, req CreateActivityRequest) (*Activity, error)

	// RecordAuthActivity records authentication-related activities
	RecordAuthActivity(
		ctx context.Context, userID string, activityType string, sessionID *string,
		metadata map[string]interface{}, sourceIP *string, userAgent *string,
	) error

	// RecordResourceActivity records resource management activities
	RecordResourceActivity(
		ctx context.Context, userID string, activityType string, entityType string,
		entityID *string, description string, metadata map[string]interface{},
	) error

	// RecordClaudeCodeActivity records Claude Code session activities
	RecordClaudeCodeActivity(
		ctx context.Context, userID string, sessionID string, toolName *string,
		hookEventName string, metadata map[string]interface{},
	) error
}

// ActivityService defines the interface for activity management
type ActivityService interface {
	ActivityTracker

	// GetActivities retrieves activities with filtering and pagination
	GetActivities(ctx context.Context, filters ActivityFilters) (*ActivityListResponse, error)

	// GetActivityByID retrieves a specific activity by ID
	GetActivityByID(ctx context.Context, userID string, activityID string) (*Activity, error)

	// GetActivityStats retrieves activity statistics
	GetActivityStats(ctx context.Context, userID string) (*ActivityStatsResponse, error)

	// DeleteActivity deletes an activity (admin only)
	DeleteActivity(ctx context.Context, activityID string) error

	// GetActivityTypes returns all available activity types
	GetActivityTypes() []string

	// GetEntityTypes returns all available entity types
	GetEntityTypes() []string

	// GetAllTypes returns both activity types and entity types in a single response
	GetAllTypes() *ActivityTypesResponse

	// RunRetentionJob deletes activities older than the configured retention window.
	// Called via HTTP from Cloud Scheduler.
	RunRetentionJob(ctx context.Context) error
}
