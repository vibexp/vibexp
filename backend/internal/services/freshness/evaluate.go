// Package freshness evaluates a team's freshness rules and reconciles the
// system-owned stale state they imply (issue #732, epic #726).
//
// It is the write half of the epic's read -> work -> write-back loop: the
// access path (#730) records when a resource was last read, and this package
// turns "not read for long enough" into a durable, auditable flag without
// anyone maintaining it by hand.
//
// The evaluator is a pure reconciliation pass. It computes the set of stale
// resources a team's enabled rules currently imply, diffs that against what is
// stored, and applies only the differences -- so running it twice in a row is
// a no-op, which is what makes it safe under the scheduler's at-least-once
// delivery.
//
// # The mark and its audit entry are two writes, and that is deliberate
//
// Marking a resource writes the freshness row and the audit row as separate
// statements with no transaction around them, for the reason Evaluate's own
// doc gives: one transaction spanning a large team would hold the scheduler's
// advisory lock while blocking writers on resource_freshness, for no
// correctness gain over reconciling next run.
//
// The residue is that a failure BETWEEN the two -- audit.Create erroring, the
// job's timeout firing, the process dying -- leaves a live stale row with no
// `marked` entry, and no later run repairs it: the row now exists, so every
// subsequent upsert UPDATEs rather than INSERTs, takes the bookkeeping branch,
// and writes no audit row. The log would contradict the live state forever
// (#796).
//
// repairMissingMarks is what closes that, and it is why the non-transactional
// design is safe rather than merely cheap. It asks the database which live
// stale rows have no `marked` as their newest entry and writes the missing
// entry, so the log converges on the state within one tick. Repairing after
// the fact was chosen over a transactional seam because a transaction only
// prevents FUTURE gaps: rows already broken in a running deployment stay
// broken, and finding those needs this query regardless.
package freshness

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// candidateBatchSize bounds one page of the per-rule candidate query, so a
// rule matching an entire team's resources never materializes one enormous
// result set (or holds one long-lived cursor) in the database driver.
//
// It does NOT bound the evaluator's own memory: the desired-state map
// accumulates every match across every batch, type and rule, and the stored
// state is read unpaginated. The ceiling on both is the team's resource count;
// maxCandidateBatches below is what keeps that finite.
const candidateBatchSize = 500

// maxCandidateBatches caps how many batches a single (rule, resource type)
// pair may read. Without it a bug that fails to advance the keyset cursor
// would loop forever inside a job the scheduler cannot interrupt cleanly. At
// the default batch size this is half a million resources per rule and type,
// far beyond any real team.
//
// Reaching it FAILS the run. A truncated match set is not "some resources go
// unmarked": reconciliation treats everything outside the computed set as no
// longer stale, so continuing would actively clear -- and audit as cleared --
// every already-stale resource in the unread tail, then re-mark it next run.
// Aborting leaves the team's state untouched, exactly like any other error.
const maxCandidateBatches = 1000

// ErrCandidateBatchCapExceeded reports that one rule's match set did not fit
// within maxCandidateBatches. It is a distinct sentinel because it means the
// cap needs revisiting, not that the database failed.
var ErrCandidateBatchCapExceeded = errors.New("freshness rule matched more resources than one run may read")

// Evaluator runs the freshness rule engine for one team at a time. It matches
// the scheduler's Handler shape through Evaluate, so registering it is a
// one-liner and the scheduler keeps owning timing, locking and retries.
type Evaluator struct {
	rules      repositories.FreshnessRuleRepository
	candidates repositories.FreshnessCandidateRepository
	state      repositories.ResourceFreshnessRepository
	audit      repositories.FreshnessAuditRepository
	logger     *slog.Logger
}

// NewEvaluator creates an Evaluator.
func NewEvaluator(
	rules repositories.FreshnessRuleRepository,
	candidates repositories.FreshnessCandidateRepository,
	state repositories.ResourceFreshnessRepository,
	audit repositories.FreshnessAuditRepository,
	logger *slog.Logger,
) *Evaluator {
	return &Evaluator{
		rules:      rules,
		candidates: candidates,
		state:      state,
		audit:      audit,
		logger:     logger,
	}
}

// resourceKey identifies a resource across the four resource tables. It is the
// same pair the unique index on resource_freshness uses, so a map keyed on it
// and the stored rows correspond one to one.
type resourceKey struct {
	resourceType string
	resourceID   string
}

// desiredState is what the rules say about one resource: the project it lives
// in (denormalized onto the freshness row) and every rule currently matching
// it. The rule ids are the union across rules -- decision #5 of the epic --
// so "why is this stale" is answerable as a set with no rule ordering to
// configure.
type desiredState struct {
	projectID string
	ruleIDs   []string
}

// Evaluate reconciles one team's freshness state against its enabled rules.
// It is the scheduler Handler for models.JobTypeFreshnessEvaluate.
//
// The pass is deliberately not transactional. Each resource's state is
// independent, so a partial run leaves a mixture of old and new state that the
// next run reconciles anyway -- whereas one long transaction over a large team
// would hold the scheduler's advisory lock while blocking writers on
// resource_freshness for no correctness gain.
func (e *Evaluator) Evaluate(ctx context.Context, teamID string) error {
	rules, err := e.rules.ListByTeam(ctx, teamID, true)
	if err != nil {
		return fmt.Errorf("freshness evaluate: failed to load rules for team %s: %w", teamID, err)
	}

	// No enabled rules is a meaningful state, not an early exit: every stored
	// row must then be cleared. Disabling a team's last rule is exactly how an
	// admin turns the feature off, and it has to take effect.
	desired, err := e.desiredStale(ctx, teamID, rules)
	if err != nil {
		return err
	}

	stored, err := e.state.ListAllByTeam(ctx, teamID)
	if err != nil {
		return fmt.Errorf("freshness evaluate: failed to load freshness state for team %s: %w", teamID, err)
	}

	counts, err := e.reconcile(ctx, teamID, desired, stored)
	if err != nil {
		return err
	}

	e.logger.Info("freshness evaluate: team reconciled",
		"team_id", teamID,
		"enabled_rules", len(rules),
		"stale_total", len(desired),
		"marked", counts.marked,
		"refreshed", counts.refreshed,
		"cleared", counts.cleared,
		"repaired", counts.repaired,
	)
	return nil
}

// reconcileCounts tallies one pass, one field per phase. It exists because the
// phases now outnumber what a readable multi-return can carry, and because
// they are reported together in a single log line.
type reconcileCounts struct {
	marked    int
	refreshed int
	cleared   int
	repaired  int
}

// desiredStale computes the union of every enabled rule's matches.
//
// Rules are evaluated independently and merged in memory rather than combined
// into one query: the rules of a single team differ in resource type, project
// scope, medium set AND threshold, so a combined query would be a union of
// per-rule branches anyway -- with none of the readability.
func (e *Evaluator) desiredStale(
	ctx context.Context, teamID string, rules []*models.FreshnessRule,
) (map[resourceKey]*desiredState, error) {
	desired := make(map[resourceKey]*desiredState)

	for _, rule := range rules {
		for _, resourceType := range rule.ResourceTypes {
			if err := e.collectRuleMatches(ctx, teamID, rule, resourceType, desired); err != nil {
				return nil, err
			}
		}
	}
	return desired, nil
}

// collectRuleMatches pages through one (rule, resource type) pair, adding the
// rule's id to every resource it matches.
func (e *Evaluator) collectRuleMatches(
	ctx context.Context,
	teamID string,
	rule *models.FreshnessRule,
	resourceType string,
	desired map[resourceKey]*desiredState,
) error {
	query := models.FreshnessCandidateQuery{
		TeamID:        teamID,
		ResourceType:  resourceType,
		ProjectID:     rule.ProjectID,
		Mediums:       rule.Mediums,
		ThresholdDays: rule.ThresholdDays,
		Limit:         candidateBatchSize,
	}

	for batch := 0; batch < maxCandidateBatches; batch++ {
		candidates, err := e.candidates.ListStaleCandidates(ctx, query)
		if err != nil {
			return fmt.Errorf(
				"freshness evaluate: failed to list stale %s for rule %s: %w", resourceType, rule.ID, err)
		}
		for _, candidate := range candidates {
			addMatch(desired, candidate, rule.ID)
		}

		// A short page is the last page: the query returns up to Limit rows
		// ordered by id, so fewer than Limit means the scan reached the end.
		if len(candidates) < candidateBatchSize {
			return nil
		}
		query.AfterID = candidates[len(candidates)-1].ResourceID
	}

	return fmt.Errorf("freshness evaluate: %w: rule %s over %s exceeded %d batches of %d",
		ErrCandidateBatchCapExceeded, rule.ID, resourceType, maxCandidateBatches, candidateBatchSize)
}

// addMatch records that ruleID matches the candidate, keeping the rule id set
// sorted and duplicate-free so comparing it with the stored set is a plain
// slices.Equal.
func addMatch(desired map[resourceKey]*desiredState, candidate models.FreshnessCandidate, ruleID string) {
	key := resourceKey{resourceType: candidate.ResourceType, resourceID: candidate.ResourceID}

	entry, ok := desired[key]
	if !ok {
		desired[key] = &desiredState{projectID: candidate.ProjectID, ruleIDs: []string{ruleID}}
		return
	}
	if index, found := slices.BinarySearch(entry.ruleIDs, ruleID); !found {
		entry.ruleIDs = slices.Insert(entry.ruleIDs, index, ruleID)
	}
}

// reconcile applies the difference between the desired stale set and the
// stored one, returning how many resources were newly marked, refreshed
// (already stale, matching rules changed), cleared and repaired.
func (e *Evaluator) reconcile(
	ctx context.Context,
	teamID string,
	desired map[resourceKey]*desiredState,
	stored []*models.ResourceFreshness,
) (counts reconcileCounts, err error) {
	storedByKey := make(map[resourceKey]*models.ResourceFreshness, len(stored))
	for _, row := range stored {
		// The desired sets are built sorted, so sorting the stored one makes
		// the comparison below order-insensitive. Rows this evaluator wrote are
		// already sorted; normalizing here means a row written by anything else
		// cannot make an unchanged rule set look changed and spin a pointless
		// write on every run.
		slices.Sort(row.MatchedRuleIDs)
		storedByKey[resourceKey{resourceType: row.ResourceType, resourceID: row.ResourceID}] = row
	}

	// Clears run FIRST. The scheduler bounds a handler with a per-job timeout,
	// and marking is the unbounded half (every matching resource) while
	// clearing is bounded by what is already stale. Marking first would mean a
	// team big enough to exhaust the timeout never reaches its clears, so its
	// flags would become permanently sticky while new marks kept landing. The
	// two phases touch disjoint keys, so the order does not change the result.
	counts.cleared, err = e.applyClears(ctx, teamID, desired, storedByKey)
	if err != nil {
		return counts, err
	}

	counts.marked, counts.refreshed, err = e.applyMarks(ctx, teamID, desired, storedByKey)
	if err != nil {
		return counts, err
	}

	// Repairs run LAST, and that ordering is load-bearing in both directions.
	// It must follow applyMarks so a resource this run has just marked and
	// audited is not audited a second time -- the repair query reads the log
	// after those writes land, so it already sees them. And it must follow
	// applyClears so a resource this run cleared is already gone from the
	// state table when the query asks, which is what keeps the repair from
	// writing a `marked` for a resource that is no longer stale -- the
	// corruption being fixed here, inverted.
	counts.repaired, err = e.repairMissingMarks(ctx, teamID, desired)
	return counts, err
}

// repairMissingMarks writes the `marked` audit entry for every live stale row
// whose newest entry is not one, healing a mark whose audit write did not land
// (#796; see the package doc for why that gap exists by design).
//
// WHICH rows need repairing comes from the repository, never from the
// snapshot: only the database knows which rows survived this run's clears, and
// only it knows what the log's newest entry is after this run's marks.
//
// WHAT to attribute them to comes from `desired` -- this run's computed rule
// set, which is exactly what applyMarks wrote to the row. The snapshot is the
// wrong source: a row can be refreshed (its rule set rewritten, no audit) and
// repaired in the same run, and the snapshot still holds the rule ids the row
// carried BEFORE that rewrite, so the repaired entry would attribute rules the
// live row no longer matches.
//
// A live row absent from `desired` is skipped rather than guessed at. After
// applyClears every surviving row is in `desired` by construction, so this
// means the row appeared mid-pass: it is not this run's to describe, and the
// next run repairs it if it still needs it.
func (e *Evaluator) repairMissingMarks(
	ctx context.Context, teamID string, desired map[resourceKey]*desiredState,
) (int, error) {
	refs, err := e.audit.ListStaleResourcesMissingMark(ctx, teamID)
	if err != nil {
		return 0, fmt.Errorf(
			"freshness evaluate: failed to list stale resources missing a mark for team %s: %w", teamID, err)
	}

	repaired := 0
	for _, ref := range refs {
		key := resourceKey{resourceType: ref.ResourceType, resourceID: ref.ResourceID}
		want, stale := desired[key]
		if !stale {
			continue
		}
		if err := e.recordAudit(ctx, teamID, key, models.FreshnessActionMarked, want.ruleIDs); err != nil {
			return repaired, err
		}
		repaired++
	}
	return repaired, nil
}

// applyMarks writes every resource the rules currently match whose stored rule
// set differs from the computed one.
func (e *Evaluator) applyMarks(
	ctx context.Context,
	teamID string,
	desired map[resourceKey]*desiredState,
	storedByKey map[resourceKey]*models.ResourceFreshness,
) (marked, refreshed int, err error) {
	for key, want := range desired {
		existing, wasStale := storedByKey[key]

		// Identical rule sets mean nothing about this resource changed. Writing
		// anyway would move updated_at on every run and, worse, tempt an audit
		// row per run -- the single most likely source of log spam here.
		if wasStale && slices.Equal(existing.MatchedRuleIDs, want.ruleIDs) {
			continue
		}

		var inserted bool
		if inserted, err = e.markStale(ctx, teamID, key, want); err != nil {
			return marked, refreshed, err
		}
		if !inserted {
			// The upsert UPDATED a row that was already there, so the resource
			// did not change status: the state is refreshed and no audit row is
			// written. The audit log records transitions, not the bookkeeping
			// between them.
			//
			// The decision is the database's `inserted` alone -- the snapshot's
			// wasStale is used only for the equal-rule-set short-circuit above,
			// exactly as clearStale decides on `deleted` and not on what it
			// read (#796). The combination that made the difference,
			// !wasStale && !inserted, is unreachable today (Upsert has one
			// production caller and the scheduler's advisory lock keeps
			// same-team runs from overlapping), but it is the same class of bug
			// as #771 and is settled deliberately rather than left resting on
			// who holds the lock.
			refreshed++
			continue
		}
		// The upsert INSERTED. Usually that is a resource becoming stale for
		// the first time; when the snapshot also said wasStale it is the race
		// (#771) -- a clear deleted the row between the read and the upsert.
		// Either way it is a real transition into stale, and without an audit
		// row the newest entry for this resource would be the clear, an audit
		// log that contradicts the live state.
		if err = e.recordAudit(ctx, teamID, key, models.FreshnessActionMarked, want.ruleIDs); err != nil {
			return marked, refreshed, err
		}
		marked++
	}
	return marked, refreshed, nil
}

// applyClears removes every stored row the rules no longer match.
func (e *Evaluator) applyClears(
	ctx context.Context,
	teamID string,
	desired map[resourceKey]*desiredState,
	storedByKey map[resourceKey]*models.ResourceFreshness,
) (cleared int, err error) {
	for key, row := range storedByKey {
		if _, stillStale := desired[key]; stillStale {
			continue
		}
		if err = e.clearStale(ctx, teamID, key, row); err != nil {
			return cleared, err
		}
		cleared++
	}
	return cleared, nil
}

// markStale writes the freshness row for a resource the rules currently match.
// Since is left zero so the repository keeps the existing "first marked at"
// for a resource that was already stale and stamps the database clock for one
// that was not.
//
// It passes through the repository's report of whether the row was INSERTED,
// which is what lets the caller tell a genuine stale-again transition from
// bookkeeping on a row that survived (#771).
func (e *Evaluator) markStale(
	ctx context.Context, teamID string, key resourceKey, want *desiredState,
) (bool, error) {
	row := &models.ResourceFreshness{
		TeamID:         teamID,
		ProjectID:      want.projectID,
		ResourceType:   key.resourceType,
		ResourceID:     key.resourceID,
		Status:         models.FreshnessStatusStale,
		MatchedRuleIDs: want.ruleIDs,
		Reason:         models.FreshnessReasonRuleRun,
	}
	inserted, err := e.state.Upsert(ctx, row)
	if err != nil {
		return false, fmt.Errorf(
			"freshness evaluate: failed to mark %s %s stale: %w", key.resourceType, key.resourceID, err)
	}
	return inserted, nil
}

// clearStale removes the freshness row of a resource no rule matches any more
// and records the transition.
//
// The audit row is written only when a row was actually removed. A no-op
// delete means something else already cleared it -- an access or an edit
// (#733), or a concurrent run -- and that clear has its own audit entry;
// adding a second one would invent a transition that never happened.
func (e *Evaluator) clearStale(
	ctx context.Context, teamID string, key resourceKey, row *models.ResourceFreshness,
) error {
	deleted, err := e.state.DeleteByResource(ctx, key.resourceType, key.resourceID)
	if err != nil {
		return fmt.Errorf(
			"freshness evaluate: failed to clear %s %s: %w", key.resourceType, key.resourceID, err)
	}
	if !deleted {
		return nil
	}
	return e.recordAudit(ctx, teamID, key, models.FreshnessActionCleared, row.MatchedRuleIDs)
}

// recordAudit appends one mark/clear entry.
//
// rule_id is attributed only when exactly one rule is involved. With several,
// naming one of them would be a fiction -- the union that made the resource
// stale is on the freshness row itself (matched_rule_ids), which is what "why
// is this stale" reads.
func (e *Evaluator) recordAudit(
	ctx context.Context, teamID string, key resourceKey, action string, ruleIDs []string,
) error {
	entry := &models.ResourceFreshnessAudit{
		TeamID:       teamID,
		ResourceType: key.resourceType,
		ResourceID:   key.resourceID,
		Action:       action,
		Reason:       models.FreshnessReasonRuleRun,
	}
	if len(ruleIDs) == 1 {
		ruleID := ruleIDs[0]
		entry.RuleID = &ruleID
	}

	if err := e.audit.Create(ctx, entry); err != nil {
		return fmt.Errorf("freshness evaluate: failed to record %s audit for %s %s: %w",
			action, key.resourceType, key.resourceID, err)
	}
	return nil
}
