package models

import (
	"time"
)

// CursorIDEHookPayload represents the structure of Cursor IDE hook payload
type CursorIDEHookPayload struct {
	ID             int        `json:"id" db:"id"`
	UserID         *string    `json:"user_id,omitempty" db:"user_id"`
	TeamID         string     `json:"team_id" db:"team_id"`
	SessionID      string     `json:"session_id" db:"session_id"` // Stores conversation_id from Cursor
	ConversationID *string    `json:"conversation_id,omitempty" db:"conversation_id"`
	GenerationID   *string    `json:"generation_id,omitempty" db:"generation_id"`
	HookEventName  string     `json:"hook_event_name" db:"hook_event_name"`
	ToolName       *string    `json:"tool_name,omitempty" db:"tool_name"`
	WorkspaceRoots []string   `json:"workspace_roots,omitempty" db:"workspace_roots"`
	Configuration  *JSONBData `json:"configuration,omitempty" db:"configuration"`
	Reference      *JSONBData `json:"reference,omitempty" db:"reference"`
	Context        *JSONBData `json:"context,omitempty" db:"context"`
	Input          *JSONBData `json:"input,omitempty" db:"input"`
	Output         *JSONBData `json:"output,omitempty" db:"output"`
	InducedFailure *JSONBData `json:"induced_failure,omitempty" db:"induced_failure"`
	Payload        JSONBData  `json:"payload" db:"payload"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

// IncomingCursorHookPayload represents the incoming payload from Cursor IDE hooks
type IncomingCursorHookPayload struct {
	// Cursor IDE sends conversation_id and generation_id
	ConversationID string   `json:"conversation_id,omitempty"`
	GenerationID   string   `json:"generation_id,omitempty"`
	WorkspaceRoots []string `json:"workspace_roots,omitempty"`

	// Legacy/optional fields
	SessionID      string      `json:"session_id,omitempty"`
	HookEventName  string      `json:"hook_event_name"`
	ToolName       *string     `json:"tool_name,omitempty"`
	Configuration  interface{} `json:"configuration,omitempty"`
	Reference      interface{} `json:"reference,omitempty"`
	Context        interface{} `json:"context,omitempty"`
	Input          interface{} `json:"input,omitempty"`
	Output         interface{} `json:"output,omitempty"`
	InducedFailure interface{} `json:"inducedFailure,omitempty"`

	// Hook-specific fields from Cursor documentation
	Command      *string                  `json:"command,omitempty"`      // beforeShellExecution
	CWD          *string                  `json:"cwd,omitempty"`          // beforeShellExecution
	ToolInput    interface{}              `json:"tool_input,omitempty"`   // beforeMCPExecution
	URL          *string                  `json:"url,omitempty"`          // beforeMCPExecution
	FilePath     *string                  `json:"file_path,omitempty"`    // afterFileEdit, beforeReadFile
	Edits        []map[string]interface{} `json:"edits,omitempty"`        // afterFileEdit
	Content      *string                  `json:"content,omitempty"`      // beforeReadFile
	Attachments  []map[string]interface{} `json:"attachments,omitempty"`  // beforeReadFile, beforeSubmitPrompt
	Prompt       *string                  `json:"prompt,omitempty"`       // beforeSubmitPrompt
	Status       *string                  `json:"status,omitempty"`       // stop hook
	Permission   *string                  `json:"permission,omitempty"`   // Response field
	UserMessage  *string                  `json:"userMessage,omitempty"`  // Response field
	AgentMessage *string                  `json:"agentMessage,omitempty"` // Response field
	Continue     *bool                    `json:"continue,omitempty"`     // Response field
}

// CursorIDEHooksPaginatedResponse represents a paginated API response
type CursorIDEHooksPaginatedResponse struct {
	Data       JSONArray[CursorIDEHookPayload] `json:"data"`
	Page       int                             `json:"page"`
	Limit      int                             `json:"limit"`
	Total      int                             `json:"total"`
	TotalPages int                             `json:"total_pages"`
}

// CursorSessionSummary represents a summary of a Cursor IDE session
type CursorSessionSummary struct {
	SessionID   string    `json:"session_id" db:"session_id"`
	FirstSeen   time.Time `json:"first_seen" db:"first_seen"`
	LastSeen    time.Time `json:"last_seen" db:"last_seen"`
	HookCount   int       `json:"hook_count" db:"hook_count"`
	UniqueTools int       `json:"unique_tools" db:"unique_tools"`
}

// CursorSessionsResponse represents the sessions list API response
type CursorSessionsResponse struct {
	Data       JSONArray[CursorSessionSummary] `json:"data"`
	Page       int                             `json:"page"`
	Limit      int                             `json:"limit"`
	Total      int                             `json:"total"`
	TotalPages int                             `json:"total_pages"`
}

// CursorSessionCountsResponse represents the session counts API response
type CursorSessionCountsResponse struct {
	TotalSessions int                           `json:"total_sessions"`
	Counts        JSONArray[SessionCountByDate] `json:"counts"`
}

// CursorOverviewStats represents comprehensive statistics for the overview page
type CursorOverviewStats struct {
	TotalSessions             int                       `json:"total_sessions"`
	SessionsThisWeek          int                       `json:"sessions_this_week"`
	SessionsLastWeek          int                       `json:"sessions_last_week"`
	WeeklyTrendPercent        float64                   `json:"weekly_trend_percent"`
	AvgUserPromptsPerSession  float64                   `json:"avg_user_prompts_per_session"`
	TotalUniqueTools          int                       `json:"total_unique_tools"`
	TopTools                  JSONArray[ToolUsageCount] `json:"top_tools"`
	AvgSessionDurationMinutes float64                   `json:"avg_session_duration_minutes"`
}

// CursorRecentActivity represents a recent Cursor IDE session activity
type CursorRecentActivity struct {
	SessionID     string     `json:"session_id" db:"session_id"`
	ToolName      *string    `json:"tool_name,omitempty" db:"tool_name"`
	Input         *JSONBData `json:"input,omitempty" db:"input"`
	HookEventName string     `json:"hook_event_name" db:"hook_event_name"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
}

// CursorRecentActivitiesResponse represents the recent activities API response
type CursorRecentActivitiesResponse struct {
	Activities JSONArray[CursorRecentActivity] `json:"activities"`
	Page       int                             `json:"page"`
	Limit      int                             `json:"limit"`
	Total      int                             `json:"total"`
	TotalPages int                             `json:"total_pages"`
}
