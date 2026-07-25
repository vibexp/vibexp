package activities

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
	// EntityName is the human-readable name of the referenced entity (resolved at the service layer).
	// It is nil when the entity type has no resolvable name (e.g. session, system) or when the
	// entity has been deleted. Consumers should fall back to EntityID when nil.
	EntityName *string `json:"entity_name,omitempty"`
	// ActorName is the display name of the user who performed the activity, resolved from UserID.
	// It is nil when the user record cannot be found.
	ActorName *string `json:"actor_name,omitempty"`
}

// ActivityFilters represents filtering options for activities
type ActivityFilters struct {
	UserID       *string    `json:"user_id,omitempty"`
	ActivityType *string    `json:"activity_type,omitempty"`
	EntityType   *string    `json:"entity_type,omitempty"`
	EntityID     *string    `json:"entity_id,omitempty"`
	SessionID    *string    `json:"session_id,omitempty"`
	Search       *string    `json:"search,omitempty"`
	DateFrom     *time.Time `json:"date_from,omitempty"`
	DateTo       *time.Time `json:"date_to,omitempty"`
	Limit        int        `json:"limit"`
	Offset       int        `json:"offset"`
}

// CreateActivityRequest represents the request to create a new activity
type CreateActivityRequest struct {
	ActivityType string                 `json:"activity_type" validate:"required,min=1,max=50"`
	EntityType   string                 `json:"entity_type" validate:"required,min=1,max=50"`
	EntityID     *string                `json:"entity_id,omitempty" validate:"omitempty,max=255"`
	SessionID    *string                `json:"session_id,omitempty" validate:"omitempty,max=255"`
	Description  string                 `json:"description" validate:"required,min=1"`
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

// ActivityTypesResponse represents the aggregated response for activity and entity types
type ActivityTypesResponse struct {
	ActivityTypes []string `json:"activity_types"`
	EntityTypes   []string `json:"entity_types"`
}

// Activity Type Constants
const (
	// Authentication activities
	ActivityTypeAuthLogin    = "auth_login"
	ActivityTypeAuthLogout   = "auth_logout"
	ActivityTypeAuthFailure  = "auth_failure"
	ActivityTypeTokenRefresh = "token_refresh"

	// API Key activities
	ActivityTypeAPIKeyCreated = "api_key_created"
	ActivityTypeAPIKeyDeleted = "api_key_deleted"
	ActivityTypeAPIKeyUsed    = "api_key_used"

	// Prompt activities
	ActivityTypePromptCreated = "prompt_created"
	ActivityTypePromptUpdated = "prompt_updated"
	ActivityTypePromptDeleted = "prompt_deleted"

	// Artifact activities
	ActivityTypeArtifactCreated = "artifact_created"
	ActivityTypeArtifactUpdated = "artifact_updated"
	ActivityTypeArtifactDeleted = "artifact_deleted"

	// Blueprint activities
	ActivityTypeBlueprintCreated = "blueprint_created"
	ActivityTypeBlueprintUpdated = "blueprint_updated"
	ActivityTypeBlueprintDeleted = "blueprint_deleted"

	// Claude Code activities
	ActivityTypeClaudeCodeSession = "claude_code_session"
	ActivityTypeClaudeCodeTool    = "claude_code_tool"
	ActivityTypeClaudeCodePrompt  = "claude_code_prompt"

	// Agent activities
	ActivityTypeAgentCreated            = "agent_created"
	ActivityTypeAgentUpdated            = "agent_updated"
	ActivityTypeAgentDeleted            = "agent_deleted"
	ActivityTypeAgentActivated          = "agent_activated"
	ActivityTypeAgentPaused             = "agent_paused"
	ActivityTypeAgentExecutionStarted   = "agent_execution_started"
	ActivityTypeAgentExecutionCompleted = "agent_execution_completed"
	ActivityTypeAgentExecutionFailed    = "agent_execution_failed"

	// Memory activities
	ActivityTypeMemoryCreated = "memory_created"
	ActivityTypeMemoryUpdated = "memory_updated"
	ActivityTypeMemoryDeleted = "memory_deleted"

	// System activities
	ActivityTypeSystemError   = "system_error"
	ActivityTypeSystemWarning = "system_warning"
	ActivityTypeSystemInfo    = "system_info"

	// GitHub import activities
	ActivityTypeGitHubProjectImported    = "github.project_imported"
	ActivityTypeGitHubBlueprintsImported = "github.blueprints_imported"

	// Project migration activities
	ActivityTypeProjectMigrated = "project_migrated"

	// Instance-admin activities (#454). The acting admin is the activity's
	// user_id; the affected account is the entity_id.
	ActivityTypeAdminUserSuspended   = "admin_user_suspended"
	ActivityTypeAdminUserReactivated = "admin_user_reactivated"
	ActivityTypeAdminUserUpdated     = "admin_user_updated"
	// ActivityTypeAdminUserDeleted is recorded against the ACTING ADMIN, never
	// the deleted user: an activities row owned by the target would cascade away
	// with them (activities.user_id is ON DELETE CASCADE), erasing the audit
	// trail of the deletion at the moment it happens.
	ActivityTypeAdminUserDeleted = "admin_user_deleted"
)

// Entity Type Constants
const (
	EntityTypeUser      = "user"
	EntityTypeAPIKey    = "api_key"
	EntityTypePrompt    = "prompt"
	EntityTypeArtifact  = "artifact"
	EntityTypeBlueprint = "blueprint"
	EntityTypeProject   = "project"
	EntityTypeSession   = "session"
	EntityTypeAgent     = "agent"
	EntityTypeMemory    = "memory"
	EntityTypeSystem    = "system"
)
