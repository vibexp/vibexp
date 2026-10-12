package models

import (
	"strings"
	"time"
)

// Write actions recorded on a ResourceWriteEvent.
const (
	WriteActionCreate = "create"
	WriteActionUpdate = "update"
)

// MaxAssistantNameLength caps the assistant name stored for a write. It matches
// the varchar(30) columns on resource_write_events and content_versions.
const MaxAssistantNameLength = 30

// WriteOrigin says how a resource write arrived: the channel it came through
// (web, cli, mcp or api, the resourceaccess.Source* vocabulary) and, for an MCP
// write, the name the client reported for itself.
//
// The server resolves it from how the request was authenticated and routed. It
// is never read from a request body, which is why the request models carry it
// under `json:"-"`.
type WriteOrigin struct {
	Source string
	// AssistantName is nil for every write that is not MCP and for an MCP
	// session that reported no client name.
	AssistantName *string
}

// NormalizeAssistantName trims a client-reported assistant name and caps it at
// MaxAssistantNameLength runes. It returns nil when nothing is left, so an
// empty or whitespace-only name is stored as "not reported" rather than as "".
func NormalizeAssistantName(name string) *string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil
	}
	if runes := []rune(trimmed); len(runes) > MaxAssistantNameLength {
		// Trim again: the cut can land right after a space.
		trimmed = strings.TrimSpace(string(runes[:MaxAssistantNameLength]))
	}
	return &trimmed
}

// ResourceWriteEvent is one successful create or update of a team resource. It
// is the write-side twin of ResourceAccessEvent.
type ResourceWriteEvent struct {
	ID            string    `json:"id" db:"id"`
	TeamID        string    `json:"team_id" db:"team_id"`
	ProjectID     *string   `json:"project_id,omitempty" db:"project_id"`
	UserID        *string   `json:"user_id,omitempty" db:"user_id"`
	ResourceType  string    `json:"resource_type" db:"resource_type"`
	ResourceID    string    `json:"resource_id" db:"resource_id"`
	Action        string    `json:"action" db:"action"`
	Source        string    `json:"source" db:"source"`
	AssistantName *string   `json:"assistant_name,omitempty" db:"assistant_name"`
	APIKeyID      *string   `json:"api_key_id,omitempty" db:"api_key_id"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// ResourceCreationSourceCount is the number of resources of one type created
// through one channel. ResourceType uses the plural keys of the creation
// metrics ("prompts", "artifacts", "blueprints", "memories"). Source is a
// write channel, or "unknown" for resources that have no create event because
// they were created before write events were recorded.
type ResourceCreationSourceCount struct {
	ResourceType string `json:"resource_type"`
	Source       string `json:"source"`
	Count        int    `json:"count"`
}

// ResourceWriteSourceCount is the number of write events (creates and updates)
// recorded for one channel.
type ResourceWriteSourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}
