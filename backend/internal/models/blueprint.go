package models

import (
	"time"
)

type Blueprint struct {
	ID          string    `json:"id" db:"id"`
	ProjectID   string    `json:"project_id" db:"project_id"`
	Slug        string    `json:"slug" db:"slug"`
	UserID      string    `json:"user_id" db:"user_id"`
	TeamID      string    `json:"team_id" db:"team_id"`
	Content     string    `json:"content" db:"content"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	Status      string    `json:"status" db:"status"`
	Title       string    `json:"title" db:"title"`
	Description string    `json:"description" db:"description"`
	Type        string    `json:"type" db:"type"`
	Subtype     *string   `json:"subtype,omitempty" db:"subtype"`
	// Metadata is optional per the spec (type object, not required). Use
	// omitempty so a nil/empty map serializes as absent rather than JSON null —
	// a null would violate the schema's `type: object` (drift caught by the #122
	// response-conformance assertions).
	Metadata map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
	Version  int64                  `json:"version" db:"version"`

	// Sync-ready fields (epic #334).
	//
	// Path is the canonical repo-relative path this blueprint materializes to.
	// PathDerived is true when Path was derived from (type, subtype, slug) and
	// should recompute on rename; false once imported or explicitly overridden
	// (frozen). PathDerived is internal lifecycle state, not part of the API.
	Path        string `json:"path" db:"path"`
	PathDerived bool   `json:"-" db:"path_derived"`
	// RawContent is the original raw bytes (frontmatter + body). It is returned
	// only on the detail GET (omitted from list responses for payload size) —
	// the List repository query does not select it, leaving it empty.
	RawContent string `json:"raw_content,omitempty" db:"raw_content"`
	// ContentSHA is the SHA-256 (lowercase hex) of RawContent.
	ContentSHA string `json:"content_sha,omitempty" db:"content_sha"`
	// SourceContentSHA is the SHA-256 of the raw bytes AS IMPORTED, captured once
	// at import and never regenerated. Update-aware re-import (#341) treats the
	// blueprint as unedited iff ContentSHA still equals SourceContentSHA. Internal
	// edit-detection signal, not part of the API. Empty for VibeXP-authored rows.
	SourceContentSHA string `json:"-" db:"source_content_sha"`
	// Source is import provenance, server-set only; nil for VibeXP-authored
	// blueprints. Assembled from the nullable source_* / imported_at columns.
	Source *BlueprintSource `json:"source,omitempty"`
}

// BlueprintSource is the read-only import provenance exposed as the spec's
// `source` object.
type BlueprintSource struct {
	Repo       string     `json:"repo,omitempty"`
	CommitSHA  string     `json:"commit_sha,omitempty"`
	BlobSHA    string     `json:"blob_sha,omitempty"`
	ImportedAt *time.Time `json:"imported_at,omitempty"`
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
	// Path optionally freezes the blueprint's repo-relative path; when omitted a
	// default is derived from (type, subtype, slug). Traversal-validated.
	Path *string `json:"path,omitempty" validate:"omitempty,max=1024"`
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
	// Path optionally overrides (freezes) the blueprint's repo-relative path.
	// Traversal-validated.
	Path *string `json:"path,omitempty" validate:"omitempty,max=1024"`
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
