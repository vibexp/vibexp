package models

import "time"

// Team represents a team in the system
type Team struct {
	ID          string    `json:"id" db:"id"`
	OwnerID     string    `json:"owner_id" db:"owner_id"`
	Name        string    `json:"name" db:"name"`
	Slug        string    `json:"slug" db:"slug"`
	Description string    `json:"description" db:"description"`
	IsPersonal  bool      `json:"is_personal" db:"is_personal"`
	Role        string    `json:"role"`         // User's role in this team (not stored in DB, populated at runtime)
	MemberCount int       `json:"member_count"` // Number of members in this team (not stored in DB, populated at runtime)
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// TeamStatsResponse holds team-wide resource counts for the team analytics page.
// All counts reflect items that belong to the team regardless of their status.
type TeamStatsResponse struct {
	TotalProjects   int `json:"total_projects"`
	TotalPrompts    int `json:"total_prompts"`
	TotalArtifacts  int `json:"total_artifacts"`
	TotalBlueprints int `json:"total_blueprints"`
	TotalMemories   int `json:"total_memories"`
	TotalFeedItems  int `json:"total_feed_items"`
}

// TeamResourceCreationCount is a sparse per-day creation count for a single
// resource type within a team, as returned by the repository before the handler
// zero-fills it into a continuous daily series. ResourceType is one of:
// "prompts", "artifacts", "blueprints", "memories", "projects". Date is a
// YYYY-MM-DD string.
type TeamResourceCreationCount struct {
	Date         string
	ResourceType string
	Count        int
}

// TeamFeedCreationCount is a sparse per-day creation count for a single feed
// entity kind within a team, as returned by the repository before the handler
// zero-fills it into a continuous daily series. EntityType is one of "feeds"
// (channels) or "feed_items" (AI updates posted). Date is a YYYY-MM-DD string.
type TeamFeedCreationCount struct {
	Date       string
	EntityType string
	Count      int
}

// CreateTeamRequest represents the request body for creating a team
type CreateTeamRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateTeamRequest represents the request body for updating a team
type UpdateTeamRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// TeamListResponse represents the response for listing teams
type TeamListResponse struct {
	Teams      []Team `json:"teams"`
	TotalCount int    `json:"total_count"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
}

// TeamMemberDetail represents detailed information about a team member
// including user information and invitation status
type TeamMemberDetail struct {
	UserID           string  `json:"user_id"`
	Email            string  `json:"email"`
	Name             string  `json:"name"`
	Role             string  `json:"role"`
	JoinedAt         string  `json:"joined_at"`
	InvitationStatus *string `json:"invitation_status,omitempty"` // "pending" or "accepted"
}

// TeamMembersListResponse represents the response for listing team members
type TeamMembersListResponse struct {
	Members    []TeamMemberDetail `json:"members"`
	TotalCount int                `json:"total_count"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
}
