package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// SupportedCredentialTypes are the credential types the backend authenticator can
// actually apply (matching the a2a-go SDK's APIKeySecurityScheme and
// HTTPAuthSecurityScheme). Anything else (oauth2, openIdConnect, mutualTLS) is
// rejected at save time so a user can never store a credential that then fails at
// chat time with "unsupported security scheme type".
var SupportedCredentialTypes = []string{"apiKey", "http"}

// IsSupportedCredentialType reports whether t is a credential type the backend can apply.
func IsSupportedCredentialType(t string) bool {
	for _, s := range SupportedCredentialTypes {
		if t == s {
			return true
		}
	}
	return false
}

// ValidateAgentCredentials rejects credentials whose type the backend cannot
// apply, with an actionable error naming the offending scheme and the supported
// set. It is called on every save path (create, update, update-credentials) so
// the enum is enforced consistently, not only on the dedicated endpoint.
func ValidateAgentCredentials(credentials map[string]CredentialRequest) error {
	for name, cred := range credentials {
		if !IsSupportedCredentialType(cred.Type) {
			return fmt.Errorf(
				"unsupported credential type %q for scheme %q: supported types are %s",
				cred.Type, name, strings.Join(SupportedCredentialTypes, ", "),
			)
		}
	}
	return nil
}

// AgentCard is the A2A agent card. VibeXP tracks the official A2A specification
// via the a2a-go SDK, so the SDK's typed card (A2A protocol v1.0) is the
// canonical representation used for discovery, storage, and the API surface.
type AgentCard = a2a.AgentCard

// Agent represents an AI agent with configuration and execution metadata
type Agent struct {
	ID          string                 `json:"id" db:"id"`
	UserID      string                 `json:"user_id" db:"user_id"`
	TeamID      string                 `json:"team_id" db:"team_id"`
	Name        string                 `json:"name" db:"name"`
	Description string                 `json:"description" db:"description"`
	Status      string                 `json:"status" db:"status"` // active, paused, error
	CardURL     *string                `json:"card_url,omitempty" db:"card_url"`
	AgentCard   *AgentCard             `json:"agent_card,omitempty" db:"agent_card"`
	Config      map[string]interface{} `json:"config" db:"config"`
	Credentials *AgentCredentials      `json:"-" db:"credentials"`
	// List of credential names that are set (no values)
	HasCredentials []string   `json:"has_credentials,omitempty" db:"-"`
	LastRun        *time.Time `json:"last_run,omitempty" db:"last_run"`
	LastSyncedAt   *time.Time `json:"last_synced_at,omitempty" db:"last_synced_at"`
	TotalRuns      int        `json:"total_runs" db:"total_runs"`
	SuccessRate    float64    `json:"success_rate" db:"success_rate"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
	Version        int64      `json:"version" db:"version"`
}

// AgentCredentials represents the authentication credentials for an agent
type AgentCredentials map[string]AgentCredential

// AgentCredential represents a single credential with type and encrypted value
type AgentCredential struct {
	Type  string `json:"type"`  // e.g., "apiKey", "http"
	Value string `json:"value"` // encrypted value
}

// CredentialRequest represents a credential in API requests
type CredentialRequest struct {
	Type  string `json:"type" validate:"required,oneof=apiKey http"`
	Value string `json:"value" validate:"required"`
}

// CreateAgentRequest represents the request to create a new agent
type CreateAgentRequest struct {
	Name        string                       `json:"name,omitempty"`
	Description string                       `json:"description,omitempty"`
	Status      string                       `json:"status,omitempty" validate:"omitempty,oneof=active paused"`
	CardURL     string                       `json:"card_url" validate:"required,url"`
	Credentials map[string]CredentialRequest `json:"credentials,omitempty"`
}

// UpdateAgentRequest represents the request to update an existing agent
type UpdateAgentRequest struct {
	Name        *string                      `json:"name,omitempty" validate:"omitempty,min=1,max=100"`
	Description *string                      `json:"description,omitempty" validate:"omitempty,min=1,max=500"`
	Status      *string                      `json:"status,omitempty" validate:"omitempty,oneof=active paused error"`
	CardURL     *string                      `json:"card_url,omitempty" validate:"omitempty,url"`
	Credentials map[string]CredentialRequest `json:"credentials,omitempty"`
}

// UpdateAgentCredentialsRequest represents a request to update agent credentials
type UpdateAgentCredentialsRequest struct {
	Credentials map[string]CredentialRequest `json:"credentials" validate:"required"`
}

// AgentListResponse represents paginated agent response
type AgentListResponse struct {
	Agents     JSONArray[Agent] `json:"agents"`
	TotalCount int              `json:"total_count"`
	Page       int              `json:"page"`
	PerPage    int              `json:"per_page"`
	TotalPages int              `json:"total_pages"`
}

// AgentStatsResponse represents agent statistics
type AgentStatsResponse struct {
	TotalAgents    int     `json:"total_agents"`
	ActiveAgents   int     `json:"active_agents"`
	PausedAgents   int     `json:"paused_agents"`
	ErrorAgents    int     `json:"error_agents"`
	TotalRuns      int     `json:"total_runs"`
	AvgSuccessRate float64 `json:"avg_success_rate"`
	RunsToday      int     `json:"runs_today"`
	RunsThisWeek   int     `json:"runs_this_week"`
	// recent_activities is nullable per the spec (serialized as null when there
	// is no recent activity), so it stays a plain slice — out of scope for the
	// required-array invariant (#125).
	RecentActivities []AgentActivity `json:"recent_activities"`
}

// AgentActivity represents recent agent activity
type AgentActivity struct {
	ID          string    `json:"id"`
	AgentID     string    `json:"agent_id"`
	AgentName   string    `json:"agent_name"`
	Action      string    `json:"action"`
	Status      string    `json:"status"` // success, warning, error
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// AgentExecution represents an agent execution record
type AgentExecution struct {
	ID      string `json:"id" db:"id"`
	AgentID string `json:"agent_id" db:"agent_id"`
	UserID  string `json:"user_id" db:"user_id"`
	// running, success, error, pending, submitted, working, completed, failed, cancelled
	Status    string                 `json:"status" db:"status"`
	Input     map[string]interface{} `json:"input,omitempty" db:"input"`
	Error     *string                `json:"error,omitempty" db:"error"`
	StartedAt time.Time              `json:"started_at" db:"started_at"`
	EndedAt   *time.Time             `json:"ended_at,omitempty" db:"ended_at"`
	Duration  *int                   `json:"duration,omitempty" db:"duration"` // in milliseconds

	// A2A streaming fields (optional, for Agent-to-Agent protocol)
	TaskID    *string `json:"task_id,omitempty" db:"task_id"`
	ContextID *string `json:"context_id,omitempty" db:"context_id"`
	// submitted, working, input-required, auth-required, completed, failed, cancelled
	CurrentState *string                  `json:"current_state,omitempty" db:"current_state"`
	Artifacts    []map[string]interface{} `json:"artifacts,omitempty" db:"artifacts"`
	// Groups related executions into conversations
	ConversationID *string `json:"conversation_id,omitempty" db:"conversation_id"`

	Version int64 `json:"version" db:"version"`
}

// ConversationSummary represents an aggregated view of a conversation
type ConversationSummary struct {
	ConversationID string    `json:"conversation_id" db:"conversation_id"`
	AgentID        string    `json:"agent_id" db:"agent_id"`
	MessageCount   int       `json:"message_count" db:"message_count"`
	FirstMessage   string    `json:"first_message" db:"first_message"`
	LastMessage    string    `json:"last_message" db:"last_message"`
	StartedAt      time.Time `json:"started_at" db:"started_at"`
	LastActivityAt time.Time `json:"last_activity_at" db:"last_activity_at"`
	LastStatus     string    `json:"last_status" db:"last_status"`
}

// ConversationListResponse represents paginated conversation summaries
type ConversationListResponse struct {
	Conversations JSONArray[ConversationSummary] `json:"conversations"`
	TotalCount    int                            `json:"total_count"`
	Page          int                            `json:"page"`
	PerPage       int                            `json:"per_page"`
	TotalPages    int                            `json:"total_pages"`
}

// CreateAgentExecutionRequest represents the request to create a new agent execution
type CreateAgentExecutionRequest struct {
	AgentID string                 `json:"agent_id" validate:"required"`
	Input   map[string]interface{} `json:"input,omitempty"`
}

// UpdateAgentExecutionRequest represents the request to update an agent execution
type UpdateAgentExecutionRequest struct {
	Status string                 `json:"status" validate:"required,oneof=running success error"`
	Output map[string]interface{} `json:"output,omitempty"`
	Error  *string                `json:"error,omitempty"`
}
