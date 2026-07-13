package models

import (
	"time"
)

// Artifact lifecycle statuses. An artifact is active (the default, surfaced in
// lists and search), draft (a work-in-progress, visible to its owner but never
// returned by search), or archived (retired, hidden from default lists and
// search but still reachable via an explicit status filter).
const (
	ArtifactStatusActive   = "active"
	ArtifactStatusDraft    = "draft"
	ArtifactStatusArchived = "archived"
)

type Artifact struct {
	ID          string                 `json:"id" db:"id"`
	ProjectID   string                 `json:"project_id" db:"project_id"`
	Slug        string                 `json:"slug" db:"slug"`
	UserID      string                 `json:"user_id" db:"user_id"`
	TeamID      string                 `json:"team_id" db:"team_id"`
	Content     string                 `json:"content,omitempty" db:"content"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
	Status      string                 `json:"status" db:"status"`
	Title       string                 `json:"title" db:"title"`
	Description string                 `json:"description" db:"description"`
	Type        string                 `json:"type" db:"type"`
	Metadata    map[string]interface{} `json:"metadata" db:"metadata"`
	Version     int64                  `json:"version,omitempty" db:"version"`
}

type CreateArtifactRequest struct {
	ProjectID   string `json:"project_id" validate:"required,uuid"`
	Slug        string `json:"slug" validate:"required,min=1,max=255"`
	Content     string `json:"content" validate:"required"`
	Title       string `json:"title" validate:"required,min=1,max=255"`
	Description string `json:"description" validate:"omitempty,max=500"`
	// Type is validated against the team's types (system defaults + custom) by
	// the handler via TypeService, not by a static oneof here (#1846).
	Type     string                 `json:"type" validate:"omitempty"`
	Status   string                 `json:"status" validate:"omitempty,oneof=active draft archived"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type UpdateArtifactRequest struct {
	ProjectID   *string `json:"project_id,omitempty" validate:"omitempty,uuid"`
	Slug        *string `json:"slug,omitempty" validate:"omitempty,min=1,max=255"`
	Content     *string `json:"content,omitempty"`
	Title       *string `json:"title,omitempty" validate:"omitempty,min=1,max=255"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=500"`
	// Type is validated against the team's types (system defaults + custom) by
	// the handler via TypeService, not by a static oneof here (#1846).
	Type     *string                `json:"type,omitempty" validate:"omitempty"`
	Status   *string                `json:"status,omitempty" validate:"omitempty,oneof=active draft archived"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	// ChangeSummary is an optional human-readable description of this edit, recorded on
	// the content-version snapshot it produces and rendered in the version history.
	ChangeSummary *string `json:"change_summary,omitempty" validate:"omitempty,max=500"`
}

type ArtifactListResponse struct {
	Artifacts  JSONArray[Artifact] `json:"artifacts"`
	TotalCount int                 `json:"total_count"`
	Page       int                 `json:"page"`
	PerPage    int                 `json:"per_page"`
	TotalPages int                 `json:"total_pages"`
}

type ArtifactStatsResponse struct {
	TotalProjects  int            `json:"total_projects"`
	TotalArtifacts int            `json:"total_artifacts"`
	AddedThisWeek  int            `json:"added_this_week"`
	TotalByType    map[string]int `json:"total_by_type"`
	TotalByStatus  map[string]int `json:"total_by_status"`
}
