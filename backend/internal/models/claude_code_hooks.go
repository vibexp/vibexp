package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// ClaudeCodeHookPayload represents the structure of Claude Code hook payload
type ClaudeCodeHookPayload struct {
	ID             int        `json:"id" db:"id"`
	UserID         *string    `json:"user_id,omitempty" db:"user_id"`
	TeamID         string     `json:"team_id" db:"team_id"`
	SessionID      string     `json:"session_id" db:"session_id"`
	TranscriptPath *string    `json:"transcript_path,omitempty" db:"transcript_path"`
	CWD            *string    `json:"cwd,omitempty" db:"cwd"`
	HookEventName  string     `json:"hook_event_name" db:"hook_event_name"`
	ToolName       *string    `json:"tool_name,omitempty" db:"tool_name"`
	ToolInput      *JSONBData `json:"tool_input,omitempty" db:"tool_input"`
	ToolResponse   *JSONBData `json:"tool_response,omitempty" db:"tool_response"`
	Prompt         *string    `json:"prompt,omitempty" db:"prompt"`
	Message        *string    `json:"message,omitempty" db:"message"`
	Payload        JSONBData  `json:"payload" db:"payload"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

// JSONBData represents JSONB data type for PostgreSQL
type JSONBData map[string]interface{}

// Value implements driver.Valuer interface for JSONBData
func (j JSONBData) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements sql.Scanner interface for JSONBData
func (j *JSONBData) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into JSONBData", value)
	}

	return json.Unmarshal(bytes, j)
}

// IncomingHookPayload represents the incoming payload from Claude Code hooks
type IncomingHookPayload struct {
	SessionID      string      `json:"session_id"`
	TranscriptPath *string     `json:"transcript_path,omitempty"`
	CWD            *string     `json:"cwd,omitempty"`
	HookEventName  string      `json:"hook_event_name"`
	ToolName       *string     `json:"tool_name,omitempty"`
	ToolInput      interface{} `json:"tool_input,omitempty"`
	ToolResponse   interface{} `json:"tool_response,omitempty"`
	Prompt         *string     `json:"prompt,omitempty"`
	Message        *string     `json:"message,omitempty"`
}

// ClaudeCodeHooksPaginatedResponse represents a paginated API response
type ClaudeCodeHooksPaginatedResponse struct {
	Data       []ClaudeCodeHookPayload `json:"data"`
	Page       int                     `json:"page"`
	Limit      int                     `json:"limit"`
	Total      int                     `json:"total"`
	TotalPages int                     `json:"total_pages"`
}

// SessionSummary represents a summary of a Claude Code session
type SessionSummary struct {
	SessionID   string    `json:"session_id" db:"session_id"`
	FirstSeen   time.Time `json:"first_seen" db:"first_seen"`
	LastSeen    time.Time `json:"last_seen" db:"last_seen"`
	HookCount   int       `json:"hook_count" db:"hook_count"`
	LatestCWD   *string   `json:"latest_cwd,omitempty" db:"latest_cwd"`
	UniqueTools int       `json:"unique_tools" db:"unique_tools"`
}

// SessionsResponse represents the sessions list API response
type SessionsResponse struct {
	Data       []SessionSummary `json:"data"`
	Page       int              `json:"page"`
	Limit      int              `json:"limit"`
	Total      int              `json:"total"`
	TotalPages int              `json:"total_pages"`
}

// SessionCountsResponse represents the session counts API response
type SessionCountsResponse struct {
	TotalSessions int                  `json:"total_sessions"`
	Counts        []SessionCountByDate `json:"counts"`
}

// SessionCountByDate represents session count for a specific date
type SessionCountByDate struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// OverviewStats represents comprehensive statistics for the overview page
type OverviewStats struct {
	TotalSessions             int              `json:"total_sessions"`
	SessionsThisWeek          int              `json:"sessions_this_week"`
	SessionsLastWeek          int              `json:"sessions_last_week"`
	WeeklyTrendPercent        float64          `json:"weekly_trend_percent"`
	AvgUserPromptsPerSession  float64          `json:"avg_user_prompts_per_session"`
	TotalUniqueTools          int              `json:"total_unique_tools"`
	TopTools                  []ToolUsageCount `json:"top_tools"`
	AvgSessionDurationMinutes float64          `json:"avg_session_duration_minutes"`
	TotalMemories             int              `json:"total_memories"`
}

// ToolUsageCount represents tool usage statistics
type ToolUsageCount struct {
	ToolName string `json:"tool_name"`
	Count    int    `json:"count"`
}

// RecentActivity represents a recent Claude Code session activity
type RecentActivity struct {
	SessionID     string     `json:"session_id" db:"session_id"`
	CWD           *string    `json:"cwd,omitempty" db:"cwd"`
	ToolName      *string    `json:"tool_name,omitempty" db:"tool_name"`
	ToolInput     *JSONBData `json:"tool_input,omitempty" db:"tool_input"`
	HookEventName string     `json:"hook_event_name" db:"hook_event_name"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
}

// RecentActivitiesResponse represents the recent activities API response
type RecentActivitiesResponse struct {
	Activities []RecentActivity `json:"activities"`
	Page       int              `json:"page"`
	Limit      int              `json:"limit"`
	Total      int              `json:"total"`
	TotalPages int              `json:"total_pages"`
}
