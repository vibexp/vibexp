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
// Exported so the drift check can live where BOTH lists are visible: the rule
// validator's accepted types are unexported in package services, so the
// assertion that the two agree has to run from there. Drift matters because
// this gate fails SILENTLY -- a fifth rule type would simply never reverse --
// unlike the candidate repository, which rejects an unknown type loudly.
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
	rules    repositories.FreshnessRuleRepository
	logger   *slog.Logger
}

// NewClearer creates a Clearer.
func NewClearer(
	settings repositories.TeamFreshnessSettingsRepository,
	state repositories.ResourceFreshnessRepository,
	audit repositories.FreshnessAuditRepository,
	rules repositories.FreshnessRuleRepository,
	logger *slog.Logger,
) *Clearer {
	return &Clearer{settings: settings, state: state, audit: audit, rules: rules, logger: logger}
}

// ClearIfStale removes a resource's freshness state and records the reversal,
// when the resource is actually stale and the team has reversibility enabled.
// reason must be models.FreshnessReasonAccessed or models.FreshnessReasonEdited.
//
// medium is the access medium (models.ResourceAccessEvent.Source: web, cli,
// mcp or api) and is consulted only on the ACCESS path, where an access through
// a medium none of the matching rules names must not clear -- see
// mediumMatchesRules. The EDIT path reverses unconditionally and passes
// models.FreshnessMediumNone.
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
func (c *Clearer) ClearIfStale(ctx context.Context, teamID, resourceType, resourceID, reason, medium string) error {
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

	// Reversal must not undo a mark the rule is about to re-apply. Every rule's
	// staleness expression is GREATEST(updated_at, <that rule's medium
	// columns>), so an access through a medium none of the matching rules names
	// moves a column none of them read -- clearing here would flap the badge and
	// append a cleared/marked audit pair once per interval (#770).
	//
	// The EDIT path skips the check deliberately: updated_at is in EVERY rule's
	// expression regardless of its mediums, so an edit always moves a timestamp
	// the rule reads and an edit-triggered clear can never flap. Scoping it
	// would be wrong, not merely unnecessary.
	//
	// This runs after the row is found, so a fresh resource never pays for it.
	blocked, err := c.mediumBlocksReversal(ctx, teamID, reason, existing.MatchedRuleIDs, medium)
	if err != nil {
		return fmt.Errorf("freshness clear: failed to resolve rules for team %s: %w", teamID, err)
	}
	if blocked {
		// Logged because the alternative is unanswerable: "I opened it and the
		// badge stayed" leaves no audit row by design, so without this line
		// there is nothing anywhere that says why.
		c.logger.Debug("freshness clear: access medium is not watched by the rules that marked this resource",
			"team_id", teamID, "resource_type", resourceType, "resource_id", resourceID,
			"medium", medium, "matched_rule_ids", existing.MatchedRuleIDs,
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

	return c.deleteAndRecord(ctx, teamID, resourceType, resourceID, reason)
}

// deleteAndRecord removes the freshness row and, only if it really removed one,
// appends the reversal to the audit log.
func (c *Clearer) deleteAndRecord(ctx context.Context, teamID, resourceType, resourceID, reason string) error {
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

// mediumBlocksReversal reports whether this reversal must be refused because the
// access came through a medium none of the rules that marked the resource
// watches (#770).
//
// Only an ACCESS can be blocked. An edit moves `updated_at`, which is in every
// rule's staleness expression whatever mediums it names, so an edit-triggered
// clear can never be undone by the next evaluation run -- and the rules are not
// even read for one.
func (c *Clearer) mediumBlocksReversal(
	ctx context.Context, teamID, reason string, ruleIDs []string, medium string,
) (bool, error) {
	if reason != models.FreshnessReasonAccessed {
		return false, nil
	}
	matches, err := c.mediumMatchesRules(ctx, teamID, ruleIDs, medium)
	if err != nil {
		return false, err
	}
	return !matches, nil
}

// mediumMatchesRules reports whether an access through medium is one the rules
// that marked this resource actually watch -- i.e. whether clearing now would
// stick rather than be undone by the next evaluation run.
//
// A rule with no mediums means "any medium", so it always matches; that is the
// default rule and the reason this change is a no-op for most teams. One
// matching rule out of several is enough: the evaluator marks on the union of
// its rules, so the reversal answers on the union too.
//
// One ListByTeam beats N GetByID calls regardless of how many rules matched, and
// teams have few rules. It asks for ENABLED rules only, because a disabled rule
// cannot mark anything either.
//
// A matched rule id that no longer resolves -- deleted, disabled, or narrowed so
// it no longer covers this resource -- is skipped rather than treated as an
// error. If NONE of them resolve, the answer is "clear it": nothing survives to
// re-mark the resource, so the reversal cannot flap, and refusing it would leave
// the badge up for a whole interval for no reason. That is the same reasoning as
// the empty-ruleIDs case below, and a resource whose rules were all disabled is
// an ordinary admin action, not a race -- UpdateRule does not strip freshness
// state when Enabled flips to false, only DeleteRule does.
func (c *Clearer) mediumMatchesRules(
	ctx context.Context, teamID string, ruleIDs []string, medium string,
) (bool, error) {
	// Nothing claims the resource, so nothing can re-mark it and the clear
	// cannot flap. Answered without a query, not just faster than one.
	if len(ruleIDs) == 0 {
		return true, nil
	}

	rules, err := c.rules.ListByTeam(ctx, teamID, true)
	if err != nil {
		return false, err
	}

	byID := make(map[string]*models.FreshnessRule, len(rules))
	for _, rule := range rules {
		byID[rule.ID] = rule
	}

	resolved := 0
	for _, ruleID := range ruleIDs {
		rule, ok := byID[ruleID]
		if !ok {
			continue
		}
		resolved++
		if len(rule.Mediums) == 0 || slices.Contains(rule.Mediums, medium) {
			return true, nil
		}
	}
	// Only refuse when a rule that is still live watches this resource and does
	// not watch this medium -- the one case where the next run would re-mark it.
	return resolved == 0, nil
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
