package models

import "time"

// Project represents a user's project entity
type Project struct {
	ID          string    `json:"id" db:"id"`
	UserID      string    `json:"user_id" db:"user_id"`
	TeamID      string    `json:"team_id" db:"team_id"`
	Name        string    `json:"name" db:"name"`
	Slug        string    `json:"slug" db:"slug"`
	Description string    `json:"description" db:"description"`
	GitURL      string    `json:"git_url" db:"git_url"`
	Homepage    string    `json:"homepage" db:"homepage"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	Version     int64     `json:"version" db:"version"`
}

// CreateProjectRequest represents the request to create a new project
type CreateProjectRequest struct {
	Name        string  `json:"name" validate:"required,min=1,max=255"`
	Slug        string  `json:"slug" validate:"required,min=1,max=100,slug"`
	TeamID      *string `json:"team_id,omitempty" validate:"omitempty,uuid"`
	Description string  `json:"description" validate:"omitempty,max=1000"`
	GitURL      string  `json:"git_url" validate:"omitempty,url,max=500"`
	Homepage    string  `json:"homepage" validate:"omitempty,url,max=500"`
}

// DefaultProjectRequest returns the request used to bootstrap the default
// "Project 1" project for a newly created workspace or team, so resources
// created immediately after team creation have a project to be scoped to.
func DefaultProjectRequest() *CreateProjectRequest {
	return &CreateProjectRequest{
		Name:        "Project 1",
		Slug:        "project-1",
		Description: "Your first project - rename or customize as needed",
		GitURL:      "",
		Homepage:    "",
	}
}

// UpdateProjectRequest represents the request to update an existing project
type UpdateProjectRequest struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Slug        *string `json:"slug,omitempty" validate:"omitempty,min=1,max=100,slug"`
	TeamID      *string `json:"team_id,omitempty" validate:"omitempty,uuid"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=1000"`
	GitURL      *string `json:"git_url,omitempty" validate:"omitempty,url,max=500"`
	Homepage    *string `json:"homepage,omitempty" validate:"omitempty,url,max=500"`
}

// ProjectResponse wraps a Project with computed fields for API responses
type ProjectResponse struct {
	Project
	GitHubConnected bool `json:"github_connected"`
}

// ProjectListResponse represents the paginated response for listing projects
type ProjectListResponse struct {
	Projects   []ProjectResponse `json:"projects"`
	TotalCount int               `json:"total_count"`
	Page       int               `json:"page"`
	PerPage    int               `json:"per_page"`
	TotalPages int               `json:"total_pages"`
}

// ProjectStatsResponse holds resource counts for a single project.
// All counts reflect items that belong to the project regardless of their status.
type ProjectStatsResponse struct {
	TotalPrompts    int `json:"total_prompts"`
	TotalArtifacts  int `json:"total_artifacts"`
	TotalBlueprints int `json:"total_blueprints"`
	TotalMemories   int `json:"total_memories"`
	TotalFeedItems  int `json:"total_feed_items"`
}

// ProjectResourceCreationCount is a sparse per-day creation count for a single
// resource type within a project, as returned by the repository before the
// handler zero-fills it into a continuous daily series. ResourceType is one of:
// "prompts", "artifacts", "blueprints", "memories". Date is a YYYY-MM-DD string.
type ProjectResourceCreationCount struct {
	Date         string
	ResourceType string
	Count        int
}
