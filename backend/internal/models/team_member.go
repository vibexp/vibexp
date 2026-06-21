package models

import "time"

// TeamMemberRole represents the role of a user within a team
type TeamMemberRole string

const (
	// TeamMemberRoleOwner represents the owner role with full permissions
	TeamMemberRoleOwner TeamMemberRole = "owner"
	// TeamMemberRoleAdmin represents the admin role with elevated permissions
	TeamMemberRoleAdmin TeamMemberRole = "admin"
	// TeamMemberRoleMember represents a regular member role
	TeamMemberRoleMember TeamMemberRole = "member"
)

// IsValid checks if the role is a valid TeamMemberRole
func (r TeamMemberRole) IsValid() bool {
	switch r {
	case TeamMemberRoleOwner, TeamMemberRoleAdmin, TeamMemberRoleMember:
		return true
	default:
		return false
	}
}

// String returns the string representation of the role
func (r TeamMemberRole) String() string {
	return string(r)
}

// TeamMember represents a user's membership in a team
type TeamMember struct {
	ID        string         `json:"id" db:"id"`
	TeamID    string         `json:"team_id" db:"team_id"`
	UserID    string         `json:"user_id" db:"user_id"`
	Role      TeamMemberRole `json:"role" db:"role"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at"`
}
