package models

import "slices"

// The resource status vocabulary (#912): the five values it is made of. The
// per-type constants (ArtifactStatusActive, PromptStatusDraft, ...) alias these
// rather than respelling them, so a status means the same thing whichever type
// carries it.
const (
	StatusActive    = "active"
	StatusDraft     = "draft"
	StatusArchived  = "archived"
	StatusPublished = "published"
	StatusExpired   = "expired"
)

// ResourceStatuses mirrors the `ResourceStatus` schema in
// backend/schemas/common.yaml: the complete set of status values any of the
// four resource types may use. Each type accepts one of the named subsets
// below -- never the whole vocabulary -- and each subset mirrors its own
// component schema (PromptStatus, ArtifactStatus, BlueprintStatus,
// MemoryStatus).
//
// These slices are the ENFORCEMENT side of that contract. The spec enum is
// documentation: oapi-codegen binds a status straight off the wire without
// validating its enum, in a request body or a query parameter, and no
// spec-driven request validator runs in front of the handlers. So the two sides
// drift in both directions unless something pins them --
// TestSpecEnumsMatchServiceAllowlists does, one table row per subset, and
// TestResourceStatusSubsetsAreDrawnFromVocabulary additionally fails the build
// if a subset ever contains a value ResourceStatus does not declare.
//
// Subsets are deliberately NOT unified. A status is only meaningful where the
// resource type has semantics for it: `published` gates MCP exposure and exists
// only for prompts, `expired` retires a blueprint whose rules no longer apply.
// Widening one is a product decision -- it changes what the list filters accept
// and what the UI must render -- not a spec tidy-up.
var (
	// ResourceStatuses is the full vocabulary, and must stay the union of the
	// four subsets below.
	ResourceStatuses = []string{
		StatusActive, StatusDraft, StatusArchived, StatusPublished, StatusExpired,
	}

	// PromptStatuses is the subset a prompt may carry.
	PromptStatuses = []string{PromptStatusDraft, PromptStatusPublished}

	// ArtifactStatuses is the subset an artifact may carry.
	ArtifactStatuses = []string{ArtifactStatusActive, ArtifactStatusDraft, ArtifactStatusArchived}

	// BlueprintStatuses is the subset a blueprint may carry.
	BlueprintStatuses = []string{BlueprintStatusActive, BlueprintStatusExpired}

	// MemoryStatuses is the subset a memory may carry.
	MemoryStatuses = []string{MemoryStatusActive, MemoryStatusDraft, MemoryStatusArchived}
)

// The resource types that carry a status, spelled as the singular names the
// comment and relation subsystems already use (comment.go), so a status
// catalog request names a resource the same way every other cross-resource API
// does.
const (
	StatusResourceTypePrompt    = CommentResourceTypePrompt
	StatusResourceTypeArtifact  = CommentResourceTypeArtifact
	StatusResourceTypeBlueprint = CommentResourceTypeBlueprint
	StatusResourceTypeMemory    = CommentResourceTypeMemory
)

var (
	// StatusResourceTypes lists every resource type that has a status catalog
	// (epic #1263): the system subset below plus the team's own rows in
	// `team_statuses`.
	StatusResourceTypes = []string{
		StatusResourceTypePrompt, StatusResourceTypeArtifact,
		StatusResourceTypeBlueprint, StatusResourceTypeMemory,
	}

	// ReservedStatusSlugs are the slugs a team may not take for a custom
	// status, on ANY resource type: the whole system vocabulary, not just the
	// subset of the type being extended. A custom artifact status called
	// `published` would read as the prompt lifecycle state it is not.
	ReservedStatusSlugs = ResourceStatuses
)

// IsStatusResourceType reports whether resourceType is one of
// StatusResourceTypes.
func IsStatusResourceType(resourceType string) bool {
	return slices.Contains(StatusResourceTypes, resourceType)
}

// SystemStatusesFor returns the system statuses of a resource type, in the
// order its subset declares them, or nil for a type that has no status
// catalog. The result is a copy: callers append a team's custom statuses to
// it, which must never reach the package-level subset.
func SystemStatusesFor(resourceType string) []string {
	switch resourceType {
	case StatusResourceTypePrompt:
		return slices.Clone(PromptStatuses)
	case StatusResourceTypeArtifact:
		return slices.Clone(ArtifactStatuses)
	case StatusResourceTypeBlueprint:
		return slices.Clone(BlueprintStatuses)
	case StatusResourceTypeMemory:
		return slices.Clone(MemoryStatuses)
	default:
		return nil
	}
}

// DefaultStatusFor returns the status a resource of the given type gets when
// none is supplied -- `draft` for a prompt, `active` for the other three -- or
// "" for a type that has no status catalog. It mirrors the column defaults in
// the schema, and is what the 025 down migration reassigns a custom status to.
func DefaultStatusFor(resourceType string) string {
	switch resourceType {
	case StatusResourceTypePrompt:
		return StatusDraft
	case StatusResourceTypeArtifact, StatusResourceTypeBlueprint, StatusResourceTypeMemory:
		return StatusActive
	default:
		return ""
	}
}

// IsLiveStatus reports whether a resource carrying status counts as live:
// current, usable content, as opposed to work in progress or retired content.
//
// It is false ONLY for the non-live system statuses -- `draft` and `archived`
// on an artifact or memory, `draft` on a prompt, `expired` on a blueprint --
// and true for everything else. In particular every custom status is live:
// a team that invents `in-review` has not asked for its resources to drop out
// of the places live content is read. The function is total and does no
// validation, so it also answers true for a status or resource type it has
// never heard of; whether a status is ALLOWED is the catalog's question.
func IsLiveStatus(resourceType, status string) bool {
	switch resourceType {
	case StatusResourceTypeArtifact, StatusResourceTypeMemory:
		return status != StatusDraft && status != StatusArchived
	case StatusResourceTypePrompt:
		return status != StatusDraft
	case StatusResourceTypeBlueprint:
		return status != StatusExpired
	default:
		return true
	}
}

// IsAllowedStatus reports whether status is a member of the given subset.
// An empty status is NOT a member: every caller treats "" as "not supplied"
// and applies its own default, so deciding that is the caller's job.
func IsAllowedStatus(allowed []string, status string) bool {
	return slices.Contains(allowed, status)
}
