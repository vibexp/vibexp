package models

import (
	"time"

	"github.com/lib/pq"
)

type Prompt struct {
	ID          string         `json:"id" db:"id"`
	Name        string         `json:"name" db:"name"`
	Slug        string         `json:"slug" db:"slug"`
	Description string         `json:"description" db:"description"`
	Body        string         `json:"body" db:"body"`
	UserID      string         `json:"user_id" db:"user_id"`
	TeamID      string         `json:"team_id" db:"team_id"`
	ProjectID   string         `json:"project_id" db:"project_id"`
	Status      string         `json:"status" db:"status"`
	MCPExpose   bool           `json:"mcp_expose" db:"mcp_expose"`
	IsShared    bool           `json:"is_shared" db:"-"`
	Labels      pq.StringArray `json:"labels" db:"labels"`
	CreatedAt   time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at" db:"updated_at"`
	Version     int64          `json:"version" db:"version"`
	// Related is the depth-1 typed neighborhood, populated on the detail GET
	// (issue #424). JSONArray so it always serializes as [] (never null); not a
	// DB column (db:"-").
	Related JSONArray[RelatedResource] `json:"related" db:"-"`
}

type CreatePromptRequest struct {
	Name        string   `json:"name" validate:"required,min=1,max=50"`
	Slug        string   `json:"slug" validate:"required,min=1,max=255"`
	Description string   `json:"description" validate:"omitempty,max=200"`
	Body        string   `json:"body" validate:"required"`
	ProjectID   string   `json:"project_id" validate:"required,uuid"`
	Status      string   `json:"status" validate:"omitempty,oneof=draft published"`
	MCPExpose   *bool    `json:"mcp_expose,omitempty"`
	Labels      []string `json:"labels,omitempty" validate:"omitempty,max=10,dive,max=50"`
}

type UpdatePromptRequest struct {
	Name        *string  `json:"name,omitempty" validate:"omitempty,min=1,max=50"`
	Slug        *string  `json:"slug,omitempty" validate:"omitempty,min=1,max=255"`
	Description *string  `json:"description,omitempty" validate:"omitempty,max=200"`
	Body        *string  `json:"body,omitempty" validate:"omitempty"`
	ProjectID   *string  `json:"project_id,omitempty" validate:"omitempty,uuid"`
	Status      *string  `json:"status,omitempty" validate:"omitempty,oneof=draft published"`
	MCPExpose   *bool    `json:"mcp_expose,omitempty"`
	Labels      []string `json:"labels,omitempty" validate:"omitempty,max=10,dive,max=50"`
}

type PromptListResponse struct {
	Prompts    JSONArray[Prompt] `json:"prompts"`
	TotalCount int               `json:"total_count"`
	Page       int               `json:"page"`
	PerPage    int               `json:"per_page"`
	TotalPages int               `json:"total_pages"`
}

// PromptVersionListResponse is the wire shape returned by the prompt version
// listing endpoint: a single object with a versions array (newest-first). It mirrors
// ArtifactVersionListResponse / BlueprintVersionListResponse / MemoryVersionListResponse
// and reuses the generic ContentVersion snapshot type, so the shared versioning core is
// left untouched. The versioned content is the raw prompt Body template (placeholders and
// @slug references), not any rendered output.
type PromptVersionListResponse struct {
	Versions JSONArray[*ContentVersion] `json:"versions"`
}

type RenderPromptRequest struct {
	Placeholders map[string]string `json:"placeholders"`
}

type RenderPromptResponse struct {
	RenderedBody        string   `json:"rendered_body"`
	PlaceholdersMissing []string `json:"placeholders_missing,omitempty"`
	ReferencesUsed      []string `json:"references_used,omitempty"`
	Warnings            []string `json:"warnings,omitempty"`
}

type PromptReference struct {
	ID                 string    `json:"id" db:"id"`
	PromptID           string    `json:"prompt_id" db:"prompt_id"`
	ReferencedPromptID string    `json:"referenced_prompt_id" db:"referenced_prompt_id"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
}

type PromptDependencyInfo struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	ID   string `json:"id"`
}

type PromptDependenciesResponse struct {
	UsedBy JSONArray[PromptDependencyInfo] `json:"used_by"` // Prompts that reference this prompt
	Uses   JSONArray[PromptDependencyInfo] `json:"uses"`    // Prompts that this prompt references
}
