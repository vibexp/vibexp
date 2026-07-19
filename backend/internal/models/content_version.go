package models

import (
	"time"
)

// ActorType distinguishes a human-authored version from a system-generated one
// (e.g. a restore, or a future auto-save). It is part of the generic versioning
// core so every resource_type renders the actor consistently.
const (
	ActorTypeHuman  = "human"
	ActorTypeSystem = "system"
)

// VersionAuthor is the resolved, render-ready attribution for a version's author.
// It is populated server-side from CreatedBy so clients do not have to issue a
// per-version user lookup. It is nil when the version has no author (CreatedBy is
// null) or the user can no longer be resolved.
type VersionAuthor struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	Initials    string  `json:"initials"`
}

// ContentVersion is a single immutable snapshot of a versioned resource's content.
// The core is polymorphic: a snapshot is keyed by (ResourceType, ResourceID), so any
// resource that registers an adapter can be versioned without a schema change.
type ContentVersion struct {
	ID            string `json:"id" db:"id"`
	TeamID        string `json:"team_id" db:"team_id"`
	ResourceType  string `json:"resource_type" db:"resource_type"`
	ResourceID    string `json:"resource_id" db:"resource_id"`
	VersionNumber int    `json:"version_number" db:"version_number"`
	Content       string `json:"content" db:"content"`
	// RawContent is the resource's original raw bytes at this version, threaded
	// through by resources that keep a raw representation (blueprints, epic #334).
	// Empty for resources that do not (artifacts/memories/prompts).
	RawContent string `json:"raw_content,omitempty" db:"raw_content"`
	// ChangeSummary is an optional human-readable description of the change captured
	// at this version. It is the "commit message" rendered in the version history.
	ChangeSummary *string `json:"change_summary" db:"change_summary"`
	// ActorType is ActorTypeHuman or ActorTypeSystem.
	ActorType string  `json:"actor_type" db:"actor_type"`
	CreatedBy *string `json:"created_by" db:"created_by"`
	// Author is the resolved attribution for CreatedBy, populated server-side on read.
	// It is not persisted; nil when there is no resolvable author.
	Author    *VersionAuthor `json:"author"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
}

// ArtifactVersionListResponse is the wire shape returned by the artifact version
// listing endpoint: a single object with a versions array (newest-first).
type ArtifactVersionListResponse struct {
	Versions JSONArray[*ContentVersion] `json:"versions"`
}
