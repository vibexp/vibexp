package models

import "slices"

// The resource status vocabulary (#912).
//
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
		ArtifactStatusActive,
		ArtifactStatusDraft,
		ArtifactStatusArchived,
		PromptStatusPublished,
		BlueprintStatusExpired,
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

// IsAllowedStatus reports whether status is a member of the given subset.
// An empty status is NOT a member: every caller treats "" as "not supplied"
// and applies its own default, so deciding that is the caller's job.
func IsAllowedStatus(allowed []string, status string) bool {
	return slices.Contains(allowed, status)
}
