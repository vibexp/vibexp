package services

import (
	"context"
	"log/slog"

	"github.com/vibexp/vibexp/internal/models"
)

// FreshnessClearer reverses a resource's stale state when the resource is
// edited (issue #733). It is declared here, in the consuming package, so the
// four resource services depend on the one method they call rather than on the
// whole freshness package; *freshness.Clearer satisfies it.
type FreshnessClearer interface {
	// ClearIfStale removes the resource's freshness state and records the
	// reversal, when the resource is stale and the team has reversibility
	// enabled. Clearing a resource that is not stale is a no-op.
	//
	// medium scopes an ACCESS-triggered clear to the mediums of the rules that
	// marked the resource (#770); the edit path below reverses unconditionally
	// and passes models.FreshnessMediumNone.
	ClearIfStale(ctx context.Context, teamID, resourceType, resourceID, reason, medium string) error
}

// clearFreshnessAfterEdit reverses stale state after a successful update.
//
// One helper rather than the same block in four services: the resource
// services differ in almost everything else, and a reversal that behaved
// slightly differently per resource type would be a bug nobody would look for.
//
// It is best-effort by design. The edit itself has already been persisted and
// is what the caller asked for; failing the request because freshness
// bookkeeping did not land would turn a saved change into an error the user
// cannot act on. The next scheduled evaluation (#732) reconciles the row
// anyway, since the edit moved `updated_at`.
//
// Call it only AFTER the repository write succeeded — a validation failure or
// a no-op update must not look like an edit.
func clearFreshnessAfterEdit(
	ctx context.Context,
	clearer FreshnessClearer,
	logger *slog.Logger,
	teamID, resourceType, resourceID string,
) {
	if clearer == nil {
		return
	}

	// No medium: an edit moves `updated_at`, which is in every rule's staleness
	// expression whatever its mediums, so this clear can never be undone by the
	// next evaluation run and must not be scoped (#770).
	err := clearer.ClearIfStale(
		ctx, teamID, resourceType, resourceID, models.FreshnessReasonEdited, models.FreshnessMediumNone,
	)
	if err == nil {
		return
	}

	if logger == nil {
		return
	}
	logger.With("error", err).With(
		"team_id", teamID,
		"resource_type", resourceType,
		"resource_id", resourceID,
	).Warn("failed to clear freshness state after edit")
}
