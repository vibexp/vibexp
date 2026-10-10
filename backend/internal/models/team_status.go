package models

import "time"

// TeamStatus is a status a team defined for one of its resource types (epic
// #1263), stored in `team_statuses`. Unlike Type there are no system rows: the
// system statuses stay in Go (SystemStatusesFor), so every row here belongs to
// exactly one team. Uniqueness is on (team_id, resource_type, slug).
type TeamStatus struct {
	ID     string `json:"id" db:"id"`
	TeamID string `json:"team_id" db:"team_id"`
	// ResourceType is one of StatusResourceTypes (singular: "prompt", ...).
	ResourceType string `json:"resource_type" db:"resource_type"`
	// Slug is the value stored in the resource's status column.
	Slug string `json:"slug" db:"slug"`
	Name string `json:"name" db:"name"`
	// CreatedBy is nil once the creating user has been deleted (ON DELETE SET
	// NULL), or when the row was created without an actor.
	CreatedBy *string   `json:"created_by,omitempty" db:"created_by"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
