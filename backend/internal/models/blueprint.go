package models

import (
	"time"
)

type Blueprint struct {
	ID          string                 `json:"id" db:"id"`
	ProjectID   string                 `json:"project_id" db:"project_id"`
	Slug        string                 `json:"slug" db:"slug"`
	UserID      string                 `json:"user_id" db:"user_id"`
	TeamID      string                 `json:"team_id" db:"team_id"`
	Content     string                 `json:"content" db:"content"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
	Status      string                 `json:"status" db:"status"`
	Title       string                 `json:"title" db:"title"`
	Description string                 `json:"description" db:"description"`
	Type        string                 `json:"type" db:"type"`
	Subtype     *string                `json:"subtype,omitempty" db:"subtype"`
	Metadata    map[string]interface{} `json:"metadata" db:"metadata"`
	Version     int64                  `json:"version" db:"version"`
}

type CreateBlueprintRequest struct {
	ProjectID   string                 `json:"project_id" validate:"required,uuid"`
	Slug        string                 `json:"slug" validate:"required,min=1,max=255"`
	Content     string                 `json:"content" validate:"required"`
	Title       string                 `json:"title" validate:"required,min=1,max=255"`
	Description string                 `json:"description" validate:"omitempty,max=500"`
	Type        string                 `json:"type" validate:"omitempty"` // validated by handler + DB CHECK
	Subtype     *string                `json:"subtype,omitempty"`
	Status      string                 `json:"status" validate:"omitempty,oneof=active expired"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type UpdateBlueprintRequest struct {
	ProjectID   *string                `json:"project_id,omitempty" validate:"omitempty,uuid"`
	Slug        *string                `json:"slug,omitempty" validate:"omitempty,min=1,max=255"`
	Content     *string                `json:"content,omitempty"`
	Title       *string                `json:"title,omitempty" validate:"omitempty,min=1,max=255"`
	Description *string                `json:"description,omitempty" validate:"omitempty,max=500"`
	Type        *string                `json:"type,omitempty" validate:"omitempty"` // validated by handler + DB CHECK
	Subtype     *string                `json:"subtype,omitempty"`
	Status      *string                `json:"status,omitempty" validate:"omitempty,oneof=active expired"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type BlueprintListResponse struct {
	Blueprints JSONArray[Blueprint] `json:"blueprints"`
	TotalCount int                  `json:"total_count"`
	Page       int                  `json:"page"`
	PerPage    int                  `json:"per_page"`
	TotalPages int                  `json:"total_pages"`
}

// BlueprintVersionListResponse is the wire shape returned by the blueprint version
// listing endpoint: a single object with a versions array (newest-first). It mirrors
// ArtifactVersionListResponse and reuses the generic ContentVersion snapshot type, so
// the shared versioning core is left untouched.
type BlueprintVersionListResponse struct {
	Versions JSONArray[*ContentVersion] `json:"versions"`
}

type BlueprintStatsResponse struct {
	TotalProjects   int            `json:"total_projects"`
	TotalBlueprints int            `json:"total_blueprints"`
	AddedThisWeek   int            `json:"added_this_week"`
	TotalByType     map[string]int `json:"total_by_type"`
	TotalByStatus   map[string]int `json:"total_by_status"`
}
