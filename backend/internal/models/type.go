package models

import "time"

// Type is a resource-type-agnostic, team-customizable category (for example an
// artifact's "Work reports" / "Bug report"). System defaults are global rows
// (TeamID empty, IsSystem true) visible to every team; custom types belong to a
// single team. Uniqueness is on (team_id, resource_type, slug).
type Type struct {
	ID           string    `json:"id" db:"id"`
	TeamID       string    `json:"team_id,omitempty" db:"team_id"`
	ResourceType string    `json:"resource_type" db:"resource_type"`
	Slug         string    `json:"slug" db:"slug"`
	Name         string    `json:"name" db:"name"`
	IsSystem     bool      `json:"is_system" db:"is_system"`
	CreatedBy    string    `json:"created_by,omitempty" db:"created_by"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}
