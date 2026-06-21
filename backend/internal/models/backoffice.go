package models

import (
	"time"
)

// UsageMetricsRow represents a single row of weekly usage metrics
type UsageMetricsRow struct {
	WeekStart           time.Time `json:"week_start"`
	NewUsers            int       `json:"new_users"`
	NewArtifacts        int       `json:"new_artifacts"`
	NewMemories         int       `json:"new_memories"`
	NewAPIKeys          int       `json:"new_api_keys"`
	NewPrompts          int       `json:"new_prompts"`
	NewAgents           int       `json:"new_agents"`
	AgentExecutions     int       `json:"agent_executions"`
	TotalAIToolSessions int       `json:"total_ai_tool_sessions"`
	ClaudeSessions      int       `json:"claude_sessions"`
	CursorSessions      int       `json:"cursor_sessions"`
}

// UserActivityRow represents a single user's activity summary
type UserActivityRow struct {
	UserID                  string     `json:"user_id"`
	Email                   string     `json:"email"`
	Name                    string     `json:"name"`
	UserCreatedAt           time.Time  `json:"user_created_at"`
	TotalArtifacts          int        `json:"total_artifacts"`
	FirstArtifactCreatedAt  *time.Time `json:"first_artifact_created_at,omitempty"`
	TotalMemories           int        `json:"total_memories"`
	FirstMemoryCreatedAt    *time.Time `json:"first_memory_created_at,omitempty"`
	TotalPrompts            int        `json:"total_prompts"`
	FirstPromptCreatedAt    *time.Time `json:"first_prompt_created_at,omitempty"`
	TotalAgentsCreated      int        `json:"total_agents_created"`
	TotalAgentExecutionsRun int        `json:"total_agent_executions_run"`
}

// UsageAndGrowthResponse is the response structure for the usage-and-growth endpoint
type UsageAndGrowthResponse struct {
	Usage             []UsageMetricsRow `json:"usage"`
	ActivitiesPerUser []UserActivityRow `json:"activities_per_user"`
}
