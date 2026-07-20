package models

import "time"

// Resource types that can be either endpoint of a relation. These mirror the
// commentable resource types (issue #273) — relations connect the same four
// polymorphic resource kinds.
const (
	RelationResourceTypeArtifact  = "artifact"
	RelationResourceTypeMemory    = "memory"
	RelationResourceTypePrompt    = "prompt"
	RelationResourceTypeBlueprint = "blueprint"
)

// Relation types. Each names the intent an edge carries; the object (to) type a
// type permits is fixed by RelationTypeMatrix.
const (
	// RelationTypeGovernedBy: subject is governed by a blueprint (rules apply).
	RelationTypeGovernedBy = "governed-by"
	// RelationTypeSupersedes: subject replaces a same-typed object (lifecycle).
	RelationTypeSupersedes = "supersedes"
	// RelationTypeBuiltFrom: subject was built from a prompt (provenance).
	RelationTypeBuiltFrom = "built-from"
	// RelationTypeExplainedBy: subject is explained by a memory (context).
	RelationTypeExplainedBy = "explained-by"
)

// Relation origins: who proposed the edge.
const (
	RelationOriginAI    = "ai"
	RelationOriginHuman = "human"
)

// Relation statuses: the tiered-trust lifecycle. A suggested edge is a proposal
// awaiting a human confirm; a confirmed edge is trusted.
const (
	RelationStatusSuggested = "suggested"
	RelationStatusConfirmed = "confirmed"
)

// relationSameTypeObject is the sentinel used in RelationTypeMatrix to mean
// "the object type must equal the subject type" (supersedes), rather than a
// fixed object type.
const relationSameTypeObject = "*same*"

// RelationTypeMatrix maps each relation type to the object (to) type it
// requires. The subject (from) type is unconstrained except for supersedes,
// whose object must match the subject (relationSameTypeObject). This is the
// single source of truth for endpoint-type validation; see ValidateRelationEndpoints.
var RelationTypeMatrix = map[string]string{
	RelationTypeGovernedBy:  RelationResourceTypeBlueprint,
	RelationTypeBuiltFrom:   RelationResourceTypePrompt,
	RelationTypeExplainedBy: RelationResourceTypeMemory,
	RelationTypeSupersedes:  relationSameTypeObject,
}

// IsValidRelationResourceType reports whether t is one of the four resource
// types a relation may connect.
func IsValidRelationResourceType(t string) bool {
	switch t {
	case RelationResourceTypeArtifact, RelationResourceTypeMemory,
		RelationResourceTypePrompt, RelationResourceTypeBlueprint:
		return true
	default:
		return false
	}
}

// IsValidRelationType reports whether t is a known relation type.
func IsValidRelationType(t string) bool {
	_, ok := RelationTypeMatrix[t]
	return ok
}

// IsValidRelationOrigin reports whether o is a known origin.
func IsValidRelationOrigin(o string) bool {
	return o == RelationOriginAI || o == RelationOriginHuman
}

// RequiredObjectType returns the object (to) type that relationType requires
// for a given subject (from) type, and whether relationType is known. For
// supersedes the required object type is the subject type itself.
func RequiredObjectType(relationType, fromType string) (string, bool) {
	obj, ok := RelationTypeMatrix[relationType]
	if !ok {
		return "", false
	}
	if obj == relationSameTypeObject {
		return fromType, true
	}
	return obj, true
}

// Relation is a directed, typed edge between two resources within one project
// of a team. It is keyed by the polymorphic (FromType, FromID) -> (ToType,
// ToID) endpoints; neither id has a foreign key (each spans four resource
// tables), so a resource's edges are cleaned up in app code when it is deleted.
// CreatedBy/ConfirmedBy are nullable: the edge outlives the users who authored
// or confirmed it (ON DELETE SET NULL).
type Relation struct {
	ID           string    `json:"id"            db:"id"`
	TeamID       string    `json:"team_id"       db:"team_id"`
	ProjectID    string    `json:"project_id"    db:"project_id"`
	FromType     string    `json:"from_type"     db:"from_type"`
	FromID       string    `json:"from_id"       db:"from_id"`
	ToType       string    `json:"to_type"       db:"to_type"`
	ToID         string    `json:"to_id"         db:"to_id"`
	RelationType string    `json:"relation_type" db:"relation_type"`
	Origin       string    `json:"origin"        db:"origin"`
	Status       string    `json:"status"        db:"status"`
	CreatedBy    *string   `json:"created_by,omitempty"   db:"created_by"`
	ConfirmedBy  *string   `json:"confirmed_by,omitempty" db:"confirmed_by"`
	CreatedAt    time.Time `json:"created_at"    db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"    db:"updated_at"`
}

// InitialRelationStatus applies the tiered-trust rule for a new edge's status.
// A human-proposed edge is trusted immediately (confirmed). An AI-proposed edge
// is confirmed only for the low-risk provenance/context types (built-from,
// explained-by); the higher-stakes governed-by / supersedes edges start as
// suggested and await a human confirm. Callers must pass a valid origin and
// relation type (validated upstream); an unknown combination defaults to the
// safe suggested state.
func InitialRelationStatus(origin, relationType string) string {
	if origin == RelationOriginHuman {
		return RelationStatusConfirmed
	}
	if relationType == RelationTypeBuiltFrom || relationType == RelationTypeExplainedBy {
		return RelationStatusConfirmed
	}
	return RelationStatusSuggested
}

// Relation directions, from the perspective of the resource a list was
// requested for: an outgoing edge has that resource as its subject (from),
// an incoming edge has it as its object (to).
const (
	RelationDirectionOutgoing = "outgoing"
	RelationDirectionIncoming = "incoming"
)

// RelatedResource is one endpoint of a relation as seen from the other
// endpoint, enriched at query time with the related resource's resolved title
// and link fields (the same title expressions the recent-comments query uses).
// It is the read-payload shape surfaced on the four resource GET responses
// (#424) and the MCP get_resource (#425). Slug is nil for memories (no slug);
// ProjectID is present for every type.
type RelatedResource struct {
	RelationID   string    `json:"relation_id"`
	RelationType string    `json:"relation_type"`
	Direction    string    `json:"direction"`
	Origin       string    `json:"origin"`
	Status       string    `json:"status"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	Title        string    `json:"title"`
	ProjectID    *string   `json:"project_id,omitempty"`
	Slug         *string   `json:"slug,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// CreateRelationRequest is the request body for creating a relation. Origin is
// set by the surface (human for the REST UI, ai for the MCP tool), not chosen
// by an end user.
type CreateRelationRequest struct {
	FromType     string `json:"from_type"     validate:"required,oneof=artifact memory prompt blueprint"`
	FromID       string `json:"from_id"       validate:"required,uuid"`
	ToType       string `json:"to_type"       validate:"required,oneof=artifact memory prompt blueprint"`
	ToID         string `json:"to_id"         validate:"required,uuid"`
	RelationType string `json:"relation_type" validate:"required,oneof=governed-by supersedes built-from explained-by"`
	Origin       string `json:"origin"        validate:"required,oneof=ai human"`
}

// RelationListResponse is the paginated response for listing a resource's
// relations. Related is declared JSONArray so it always marshals as [] (never
// null) for this required array (issue #125, "Layer C").
type RelationListResponse struct {
	Related    JSONArray[RelatedResource] `json:"related"`
	TotalCount int                        `json:"total_count"`
	Page       int                        `json:"page"`
	PerPage    int                        `json:"per_page"`
	TotalPages int                        `json:"total_pages"`
}
