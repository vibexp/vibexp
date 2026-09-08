package models

import (
	"time"
)

// Memory lifecycle statuses, mirroring Artifact (see models/artifact.go). A memory
// is active (the default, surfaced in lists and search), draft (a work-in-progress,
// visible in default lists but never returned by search), or archived (retired,
// hidden from default lists and search but still reachable via an explicit status
// filter).
const (
	MemoryStatusActive   = StatusActive
	MemoryStatusDraft    = StatusDraft
	MemoryStatusArchived = StatusArchived
)

type Memory struct {
	ID        string `json:"id" db:"id"`
	UserID    string `json:"user_id" db:"user_id"`
	TeamID    string `json:"team_id" db:"team_id"`
	ProjectID string `json:"project_id" db:"project_id"`
	// Title is the memory's optional short title (issue #911). A pointer, and
	// REQUIRED-but-nullable in the response schema, so an untitled memory
	// serializes as `"title": null` rather than `""` or an absent key -- the
	// SPA distinguishes "no title" (derive one from the first heading) from
	// "titled the empty string", which is not a thing.
	Title     *string                `json:"title" db:"title"`
	Text      string                 `json:"text" db:"text"`
	Status    string                 `json:"status" db:"status"`
	Metadata  map[string]interface{} `json:"metadata" db:"metadata"`
	CreatedAt time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt time.Time              `json:"updated_at" db:"updated_at"`
	Version   int64                  `json:"version" db:"version"`
	// Labels is the shared taxonomy field (issue #910). It is a REQUIRED array in
	// the response schema, so LabelList guarantees `[]` rather than `null`; it is
	// also the `labels text[]` column, which is why it is not a plain []string.
	Labels LabelList `json:"labels" db:"labels"`
	// Related is the depth-1 typed neighborhood, populated on the detail GET
	// (issue #424). JSONArray so it always serializes as [] (never null); not a
	// DB column (db:"-").
	Related JSONArray[RelatedResource] `json:"related" db:"-"`
	// Similar is the computed embedding-similarity neighborhood, populated on the
	// detail read (issue #427). Distinct from Related (stored typed edges); never
	// persisted. JSONArray so it always serializes as [] (never null); db:"-".
	Similar JSONArray[SimilarResource] `json:"similar" db:"-"`
	// Freshness is the resource's staleness state, or nil when it is fresh
	// (issue #735). Like Related/Similar it is not a stored column: it is
	// attached by the handler from resource_freshness, and it is OPTIONAL in
	// the spec so adding it cannot break an existing client.
	Freshness *ResourceFreshnessState `json:"freshness,omitempty" db:"-"`
}

type CreateMemoryRequest struct {
	ProjectID string `json:"project_id" validate:"required,uuid"`
	// Title is optional; nil and "" both create an untitled memory. The
	// validate tag is documentation only -- nothing calls validate.Struct on
	// this domain, so the 255-rune limit is enforced in the service.
	Title    *string                `json:"title,omitempty" validate:"omitempty,max=255"`
	Text     string                 `json:"text" validate:"required,min=1"`
	Status   *string                `json:"status,omitempty" validate:"omitempty,oneof=active draft archived"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Labels   []string               `json:"labels,omitempty" validate:"omitempty,max=10,dive,max=50"`
}

type UpdateMemoryRequest struct {
	ProjectID *string `json:"project_id,omitempty" validate:"omitempty,uuid"`
	// Title uses OptionalString rather than *string because it is the one
	// nullable field here: omitting the key leaves the title unchanged, while
	// an explicit `null` clears it. A *string cannot express that difference.
	Title    OptionalString         `json:"title,omitzero"`
	Text     *string                `json:"text,omitempty" validate:"omitempty,min=1"`
	Status   *string                `json:"status,omitempty" validate:"omitempty,oneof=active draft archived"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Labels   []string               `json:"labels,omitempty" validate:"omitempty,max=10,dive,max=50"`
}

type MemoryListResponse struct {
	Memories   JSONArray[Memory] `json:"memories"`
	TotalCount int               `json:"total_count"`
	Page       int               `json:"page"`
	PerPage    int               `json:"per_page"`
	TotalPages int               `json:"total_pages"`
}

// MemoryVersionListResponse is the wire shape returned by the memory version
// listing endpoint: a single object with a versions array (newest-first). It mirrors
// ArtifactVersionListResponse / BlueprintVersionListResponse and reuses the generic
// ContentVersion snapshot type, so the shared versioning core is left untouched.
type MemoryVersionListResponse struct {
	Versions JSONArray[*ContentVersion] `json:"versions"`
}
