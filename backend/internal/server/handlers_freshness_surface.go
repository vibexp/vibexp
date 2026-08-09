package server

import (
	"context"
	"net/http"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// Surfacing freshness on the resource payloads and list endpoints (issue #735).
//
// Both helpers are best-effort in exactly the way relatedForResource is: a
// freshness failure logs a Warn and leaves the field absent rather than failing
// a read of the resource itself. Freshness is an advisory badge; a resource
// list that 500s because the badge could not be loaded would be a strictly
// worse product than one without the badge.
//
// Note the field is OPTIONAL in the spec and omitted when a resource is fresh,
// so every existing client is unaffected: absence already meant "no freshness"
// before this existed.

// freshnessQueryParam is the list filter's query-string name.
const freshnessQueryParam = "freshness"

// parseFreshnessFilter reads and validates the `freshness` query parameter,
// writing a 400 and returning false when it is present but unrecognized.
//
// Rejecting rather than ignoring matters more for a filter than for a sort:
// an ignored `?freshness=stail` returns the FULL list, which looks like a
// legitimate answer to the question that was asked. (These routes are
// hand-written chi handlers, so nothing validates the enum for us — and
// oapi-codegen would not have either; it does not enforce query-param enums.)
func parseFreshnessFilter(w http.ResponseWriter, r *http.Request) (string, bool) {
	value := r.URL.Query().Get(freshnessQueryParam)
	if value == "" {
		return "", true
	}
	if value != services.FreshnessFilterStale {
		writeErrorResponse(w, r, "validation_error",
			"freshness must be "+services.FreshnessFilterStale, http.StatusBadRequest)
		return "", false
	}
	return value, true
}

// freshnessForResource loads one resource's freshness for a detail response.
func (s *Server) freshnessForResource(
	ctx context.Context, teamID, resourceType, resourceID string,
) *models.ResourceFreshnessState {
	svc := s.container.FreshnessService()
	if svc == nil {
		return nil
	}

	state, err := svc.GetResourceFreshness(ctx, teamID, resourceType, resourceID)
	if err != nil {
		s.logger.With(
			"handler", "freshnessForResource",
			"team_id", teamID,
			"resource_type", resourceType,
			"resource_id", resourceID,
			"error", err.Error(),
		).Warn("Failed to load freshness for detail GET")
		return nil
	}
	return state
}

// freshnessForPage loads freshness for a whole list page in one query and
// returns a lookup keyed by resource id. A nil map is usable: a lookup on it
// yields nil, so callers assign unconditionally.
func (s *Server) freshnessForPage(
	ctx context.Context, teamID, resourceType string, resourceIDs []string,
) map[string]*models.ResourceFreshnessState {
	svc := s.container.FreshnessService()
	if svc == nil || len(resourceIDs) == 0 {
		return nil
	}

	states, err := svc.ListResourceFreshness(ctx, teamID, resourceType, resourceIDs)
	if err != nil {
		s.logger.With(
			"handler", "freshnessForPage",
			"team_id", teamID,
			"resource_type", resourceType,
			"count", len(resourceIDs),
			"error", err.Error(),
		).Warn("Failed to load freshness for list page")
		return nil
	}
	return states
}

// attachPageFreshness fills the freshness field on every item of a list page,
// using ONE query for the page.
//
// It is generic over the four resource types because their list responses are
// structurally identical and a per-type copy of this loop would be four places
// to fix the same bug. The accessors are what keep it honest: each caller says
// which id to look up and where to put the answer, so a type cannot silently be
// keyed on the wrong field.
func attachPageFreshness[T any](
	s *Server,
	ctx context.Context,
	teamID, resourceType string,
	items []T,
	idOf func(*T) string,
	assign func(*T, *models.ResourceFreshnessState),
) {
	if len(items) == 0 {
		return
	}

	ids := make([]string, 0, len(items))
	for i := range items {
		ids = append(ids, idOf(&items[i]))
	}

	// A nil map is deliberate and usable: a failed load leaves every item
	// without freshness rather than failing the list.
	states := s.freshnessForPage(ctx, teamID, resourceType, ids)
	for i := range items {
		assign(&items[i], states[idOf(&items[i])])
	}
}

// Per-type accessors for attachPageFreshness. They exist because Go has no
// field-level generics; keeping them adjacent makes a mismatch obvious.
func artifactID(a *models.Artifact) string   { return a.ID }
func promptID(p *models.Prompt) string       { return p.ID }
func blueprintID(b *models.Blueprint) string { return b.ID }
func memoryID(m *models.Memory) string       { return m.ID }

func setArtifactFreshness(a *models.Artifact, state *models.ResourceFreshnessState) {
	a.Freshness = state
}

func setPromptFreshness(p *models.Prompt, state *models.ResourceFreshnessState) {
	p.Freshness = state
}

func setBlueprintFreshness(b *models.Blueprint, state *models.ResourceFreshnessState) {
	b.Freshness = state
}

func setMemoryFreshness(m *models.Memory, state *models.ResourceFreshnessState) {
	m.Freshness = state
}
