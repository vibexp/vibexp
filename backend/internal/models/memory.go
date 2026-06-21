package models

import (
	"time"
)

type Memory struct {
	ID        string                 `json:"id" db:"id"`
	UserID    string                 `json:"user_id" db:"user_id"`
	TeamID    string                 `json:"team_id" db:"team_id"`
	ProjectID string                 `json:"project_id" db:"project_id"`
	Text      string                 `json:"text" db:"text"`
	Metadata  map[string]interface{} `json:"metadata" db:"metadata"`
	CreatedAt time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt time.Time              `json:"updated_at" db:"updated_at"`
	Version   int64                  `json:"version" db:"version"`
}

type CreateMemoryRequest struct {
	ProjectID string                 `json:"project_id" validate:"required,uuid"`
	Text      string                 `json:"text" validate:"required,min=1"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type UpdateMemoryRequest struct {
	ProjectID *string                `json:"project_id,omitempty" validate:"omitempty,uuid"`
	Text      *string                `json:"text,omitempty" validate:"omitempty,min=1"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type MemoryListResponse struct {
	Memories   []Memory `json:"memories"`
	TotalCount int      `json:"total_count"`
	Page       int      `json:"page"`
	PerPage    int      `json:"per_page"`
	TotalPages int      `json:"total_pages"`
}

// MemoryVersionListResponse is the wire shape returned by the memory version
// listing endpoint: a single object with a versions array (newest-first). It mirrors
// ArtifactVersionListResponse / BlueprintVersionListResponse and reuses the generic
// ContentVersion snapshot type, so the shared versioning core is left untouched.
type MemoryVersionListResponse struct {
	Versions []*ContentVersion `json:"versions"`
}
