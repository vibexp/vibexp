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
// backend/schemas/common.yaml: the complete set of SYSTEM status values, the
// ones VibeXP itself defines. Each resource type has one of the named subsets
// below as its system statuses -- never the whole vocabulary -- and may carry,
// besides those, the custom statuses its team defined for that type in
// `team_statuses` (epic #1263).
//
// The per-type schemas (PromptStatus, ArtifactStatus, BlueprintStatus,
// MemoryStatus) are open strings since #1266, so these slices are the only
// definition of the system subsets. Three things keep them honest:
// TestSpecEnumsMatchServiceAllowlists pins ResourceStatuses to its spec enum,
// TestSystemStatusSubsetsArePinned pins each subset's exact values, and
// TestStatusSchemasAreOpenAndNameSystemValues fails the build if a schema's
// description stops naming exactly its type's subset.
//
// Whether a status is ALLOWED on a resource is not decided here: that is the
// status catalog's question (services.StatusCatalogService), asked by the one
// validator every create, update and list path goes through.
//
// Subsets are deliberately NOT unified. A system status is only meaningful
// where the resource type has semantics for it: `published` is the counterpart
// of a prompt's `draft` (which keeps it out of MCP) and exists only for
// prompts, `expired` retires a blueprint whose rules no longer apply. Widening
// one is a product decision -- it changes what the UI must render and what
// every team gets by default -- not a spec tidy-up.
var (
	// ResourceStatuses is the full system vocabulary, and must stay the union
	// of the four subsets below.
	ResourceStatuses = []string{
		StatusActive, StatusDraft, StatusArchived, StatusPublished, StatusExpired,
	}

	// PromptStatuses are the system statuses of a prompt.
	PromptStatuses = []string{PromptStatusDraft, PromptStatusPublished}

	// ArtifactStatuses are the system statuses of an artifact.
	ArtifactStatuses = []string{ArtifactStatusActive, ArtifactStatusDraft, ArtifactStatusArchived}

	// BlueprintStatuses are the system statuses of a blueprint.
	BlueprintStatuses = []string{BlueprintStatusActive, BlueprintStatusExpired}

	// MemoryStatuses are the system statuses of a memory.
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

// NonLiveStatusesFor returns the system statuses that take a resource of the
// given type out of the places live content is read (search, keyword lists,
// MCP prompt exposure): `draft` and `archived` for an artifact or memory,
// `draft` for a prompt, `expired` for a blueprint. It is nil for a type that
// has no status catalog.
//
// This is the DENYLIST liveness is defined by, and the only definition: SQL
// read paths build a `NOT IN` from it and IsLiveStatus tests membership in it,
// so the two cannot disagree. Every value is a system status, so a team's
// custom statuses are live by construction. The result is a copy.
func NonLiveStatusesFor(resourceType string) []string {
	switch resourceType {
	case StatusResourceTypeArtifact, StatusResourceTypeMemory:
		return []string{StatusDraft, StatusArchived}
	case StatusResourceTypePrompt:
		return []string{StatusDraft}
	case StatusResourceTypeBlueprint:
		return []string{StatusExpired}
	default:
		return nil
	}
}

// IsLiveStatus reports whether a resource carrying status counts as live:
// current, usable content, as opposed to work in progress or retired content.
//
// It is false ONLY for the non-live system statuses of the type
// (NonLiveStatusesFor) and true for everything else. In particular every
// custom status is live: a team that invents `in-review` has not asked for its
// resources to drop out of the places live content is read. The function is
// total and does no validation, so it also answers true for a status or
// resource type it has never heard of; whether a status is ALLOWED is the
// catalog's question.
func IsLiveStatus(resourceType, status string) bool {
	return !slices.Contains(NonLiveStatusesFor(resourceType), status)
}
