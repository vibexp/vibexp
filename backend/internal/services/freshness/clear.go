package freshness

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// clearReasons are the only reasons a Clearer may record. `rule_run` is
// deliberately absent: a scheduled reconciliation clears through the Evaluator,
// which decides staleness from the rules rather than from a single event, and
// letting it in here would let an access masquerade as a rule decision in the
// audit log.
var clearReasons = []string{models.FreshnessReasonAccessed, models.FreshnessReasonEdited}

// EvaluableResourceTypes are the resource types freshness applies to. Accesses
// to projects and agents are recorded too, but they carry no per-medium
// last-accessed columns and no rule can name them, so they can never be stale.
//
// Exported because the access path records every type and needs to know which
// ones are worth asking about -- mirroring how the last-accessed repository
// reports an unsupported type rather than querying for one.
var EvaluableResourceTypes = []string{"prompt", "artifact", "blueprint", "memory"}

// Clearer reverses a resource's stale state the moment the resource is read or
// edited (issue #733, epic #726).
//
// It is the counterpart to Evaluator: the evaluator reconciles on a schedule,
// which with a daily interval would leave a resource someone just opened
// showing "stale" for up to a day. Reversal is what keeps the flag honest
// enough to act on -- and it is team-toggleable, so a team that wants state to
// persist until an explicit review cycle can leave the scheduled run as the
// only authority.
type Clearer struct {
	settings repositories.TeamFreshnessSettingsRepository
	state    repositories.ResourceFreshnessRepository
	audit    repositories.FreshnessAuditRepository
	logger   *slog.Logger
}

// NewClearer creates a Clearer.
func NewClearer(
	settings repositories.TeamFreshnessSettingsRepository,
	state repositories.ResourceFreshnessRepository,
	audit repositories.FreshnessAuditRepository,
	logger *slog.Logger,
) *Clearer {
	return &Clearer{settings: settings, state: state, audit: audit, logger: logger}
}

// ClearIfStale removes a resource's freshness state and records the reversal,
// when the resource is actually stale and the team has reversibility enabled.
// reason must be models.FreshnessReasonAccessed or models.FreshnessReasonEdited.
//
// A resource type freshness cannot evaluate returns immediately, without
// touching the database at all.
//
// Clearing a resource that is not stale is a cheap no-op and writes no audit
// row: the freshness lookup is an indexed hit on the unique
// (resource_type, resource_id) index and it runs FIRST, so the overwhelmingly
// common fresh-resource case costs one indexed miss and never reads settings.
// That ordering is what lets this sit on a read path at all.
//
// The team setting is read per call and never cached across calls. A cache
// would make toggling reversibility appear not to work, which is a far worse
// failure than one small indexed read on the rare stale-resource path.
func (c *Clearer) ClearIfStale(ctx context.Context, teamID, resourceType, resourceID, reason string) error {
	if !slices.Contains(clearReasons, reason) {
		return fmt.Errorf("freshness clear: reason %q is not a reversal reason", reason)
	}

	// A type no rule can name has no freshness row by construction, so the
	// lookup below would be a guaranteed miss on every project and agent read.
	if !slices.Contains(EvaluableResourceTypes, resourceType) {
		return nil
	}

	existing, err := c.state.GetByResource(ctx, resourceType, resourceID)
	if err != nil {
		return fmt.Errorf("freshness clear: failed to read freshness of %s %s: %w", resourceType, resourceID, err)
	}
	if existing == nil {
		return nil
	}

	// The stored row is the authority on tenancy. A caller passing a team that
	// does not own the resource is a bug somewhere upstream, and acting on it
	// would let one team clear another's state through a guessed id.
	if existing.TeamID != teamID {
		c.logger.Warn("freshness clear: refusing to clear a resource owned by another team",
			"team_id", teamID, "owner_team_id", existing.TeamID,
			"resource_type", resourceType, "resource_id", resourceID,
		)
		return nil
	}

	enabled, err := c.reversibilityEnabled(ctx, teamID)
	if err != nil {
		return fmt.Errorf("freshness clear: failed to read settings for team %s: %w", teamID, err)
	}
	if !enabled {
		// The row survives until the next scheduled run reconciles it, which is
		// exactly what a team turning reversibility off is asking for.
		return nil
	}

	deleted, err := c.state.DeleteByResource(ctx, resourceType, resourceID)
	if err != nil {
		return fmt.Errorf("freshness clear: failed to clear %s %s: %w", resourceType, resourceID, err)
	}
	if !deleted {
		// Something else cleared it between the read and the delete -- a
		// concurrent access, or an evaluation run. That clear wrote its own
		// audit entry; a second one would invent a transition that never
		// happened.
		return nil
	}

	// RuleID stays nil. The column means "the rule that caused this", and a
	// reversal is caused by the person who read or edited the resource, not by
	// any of the rules that had matched it -- which the row carried and which
	// the marking entry already records.
	entry := &models.ResourceFreshnessAudit{
		TeamID:       teamID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       models.FreshnessActionCleared,
		Reason:       reason,
	}
	if err := c.audit.Create(ctx, entry); err != nil {
		return fmt.Errorf("freshness clear: failed to record %s audit for %s %s: %w",
			reason, resourceType, resourceID, err)
	}
	return nil
}

// reversibilityEnabled resolves the team's toggle, falling back to the default
// when the team stores no settings row -- the same "absent row means inherit"
// rule the settings API applies.
func (c *Clearer) reversibilityEnabled(ctx context.Context, teamID string) (bool, error) {
	stored, err := c.settings.Get(ctx, teamID)
	if err != nil {
		return false, err
	}
	if stored == nil {
		return models.DefaultFreshnessReversibilityEnabled, nil
	}
	return stored.ReversibilityEnabled, nil
}
