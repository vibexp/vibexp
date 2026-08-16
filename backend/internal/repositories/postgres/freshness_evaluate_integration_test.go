//go:build integration

package postgres

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/freshness"
)

// End-to-end proof that the rule engine (#732) composes: real repositories,
// real Postgres, one seeded team taken through mark -> re-run -> clear. The
// unit suite covers the reconciliation logic against mocks; what only this can
// show is that the pieces agree -- that Upsert really preserves `since` across
// runs, that the audit log gains exactly one row per transition, and that a
// second run in the same state writes nothing at all.
//
// It lives in this package rather than beside the evaluator because the
// integration harness (TestMain, migrations, seeding helpers) lives here;
// duplicating 170 lines of harness for one suite would cost more than the
// layering nicety is worth. The import direction is still one-way: the
// evaluator depends on repository INTERFACES, never on this package.

// newIntegrationEvaluator wires the evaluator to the real repositories.
func newIntegrationEvaluator() *freshness.Evaluator {
	return freshness.NewEvaluator(
		NewFreshnessRuleRepository(integrationDB),
		NewFreshnessCandidateRepository(integrationDB),
		NewResourceFreshnessRepository(integrationDB),
		NewFreshnessAuditRepository(integrationDB),
		slog.New(slog.DiscardHandler),
	)
}

// insertEvaluationRule stores an enabled rule over the given resource types,
// watching every medium — the default shape, and the one under which reversal
// can never flap because any access moves a column the rule reads.
func insertEvaluationRule(t *testing.T, teamID string, thresholdDays int, resourceTypes ...string) string {
	t.Helper()
	return insertScopedEvaluationRule(t, teamID, thresholdDays, nil, resourceTypes...)
}

// insertScopedEvaluationRule is the same, with the rule's mediums under the
// caller's control. Passing a non-empty set is what reaches the #770 behaviour:
// the evaluator then reads only those mediums' columns, so an access through
// any other medium must not reverse the mark.
func insertScopedEvaluationRule(
	t *testing.T, teamID string, thresholdDays int, mediums []string, resourceTypes ...string,
) string {
	t.Helper()
	if mediums == nil {
		mediums = []string{}
	}
	rule := &models.FreshnessRule{
		TeamID:        teamID,
		ResourceTypes: resourceTypes,
		Mediums:       mediums,
		ThresholdDays: thresholdDays,
		Enabled:       true,
	}
	require.NoError(t, NewFreshnessRuleRepository(integrationDB).Create(context.Background(), rule))
	return rule.ID
}

func disableRule(t *testing.T, ruleID string) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"UPDATE freshness_rules SET enabled = false WHERE id = $1", ruleID)
	require.NoError(t, err)
}

// appendAuditEntry writes one log entry through the same repository the
// evaluator uses, so a fixture cannot drift from the real write path. It is
// how a test reproduces a log that disagrees with the state: writing a
// `cleared` WITHOUT deleting the row is precisely the damage a failed audit
// write leaves behind, and no production path produces it.
func appendAuditEntry(t *testing.T, teamID, resourceType, resourceID, action string, ruleID *string) {
	t.Helper()
	require.NoError(t, NewFreshnessAuditRepository(integrationDB).Create(context.Background(),
		&models.ResourceFreshnessAudit{
			TeamID:       teamID,
			ResourceType: resourceType,
			ResourceID:   resourceID,
			RuleID:       ruleID,
			Action:       action,
			Reason:       models.FreshnessReasonRuleRun,
		}))
}

func auditEntries(t *testing.T, teamID string) []*models.ResourceFreshnessAudit {
	t.Helper()
	entries, _, err := NewFreshnessAuditRepository(integrationDB).
		ListByTeam(context.Background(), teamID, 100, 0)
	require.NoError(t, err)
	return entries
}

// The full lifecycle in one test, because the interesting properties are all
// about what the SECOND and THIRD runs do.
func TestIntegrationFreshnessEvaluate_MarksThenClearsAcrossRuns(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	ctx := context.Background()
	evaluator := newIntegrationEvaluator()
	state := NewResourceFreshnessRepository(integrationDB)

	promptID := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "old", "body", "published")
	touchResource(t, "prompts", promptID, "", daysAgo(120), nil)
	ruleID := insertEvaluationRule(t, scope.teamID, 30, "prompt")

	// Run 1 -- marks.
	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))

	marked, err := state.GetByResource(ctx, "prompt", promptID)
	require.NoError(t, err)
	require.NotNil(t, marked, "the prompt is 120 days untouched under a 30-day rule")
	assert.Equal(t, models.FreshnessStatusStale, marked.Status)
	assert.Equal(t, []string{ruleID}, marked.MatchedRuleIDs)
	assert.Equal(t, scope.projectID, marked.ProjectID)
	assert.Equal(t, models.FreshnessReasonRuleRun, marked.Reason)

	entries := auditEntries(t, scope.teamID)
	require.Len(t, entries, 1)
	assert.Equal(t, models.FreshnessActionMarked, entries[0].Action)
	assert.Equal(t, models.FreshnessReasonRuleRun, entries[0].Reason)
	require.NotNil(t, entries[0].RuleID)
	assert.Equal(t, ruleID, *entries[0].RuleID)

	// Run 2 -- nothing changed, so nothing is written. `since` in particular
	// must not be reset, or the age the UI reports would restart every run.
	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))

	rerun, err := state.GetByResource(ctx, "prompt", promptID)
	require.NoError(t, err)
	require.NotNil(t, rerun)
	assert.Equal(t, marked.Since, rerun.Since, "since means FIRST marked at")
	assert.Equal(t, marked.UpdatedAt, rerun.UpdatedAt, "an unchanged rule set must not provoke a write")
	assert.Len(t, auditEntries(t, scope.teamID), 1, "a repeated run must not add audit rows")

	// Run 3 -- the rule is disabled, so the resource stops being stale.
	disableRule(t, ruleID)
	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))

	cleared, err := state.GetByResource(ctx, "prompt", promptID)
	require.NoError(t, err)
	assert.Nil(t, cleared, "clearing deletes the row; the audit log preserves the history")

	entries = auditEntries(t, scope.teamID)
	require.Len(t, entries, 2)
	assert.Equal(t, models.FreshnessActionCleared, entries[0].Action, "newest first")
	assert.Equal(t, models.FreshnessReasonRuleRun, entries[0].Reason)
}

// Two rules matching one resource must both appear in matched_rule_ids, and
// removing one must leave the resource stale under the other.
func TestIntegrationFreshnessEvaluate_UnionAcrossRules(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	ctx := context.Background()
	evaluator := newIntegrationEvaluator()
	state := NewResourceFreshnessRepository(integrationDB)

	promptID := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "old", "body", "published")
	touchResource(t, "prompts", promptID, "", daysAgo(120), nil)
	ruleA := insertEvaluationRule(t, scope.teamID, 30, "prompt")
	ruleB := insertEvaluationRule(t, scope.teamID, 60, "prompt")

	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))

	both, err := state.GetByResource(ctx, "prompt", promptID)
	require.NoError(t, err)
	require.NotNil(t, both)
	assert.ElementsMatch(t, []string{ruleA, ruleB}, both.MatchedRuleIDs)

	disableRule(t, ruleA)
	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))

	remaining, err := state.GetByResource(ctx, "prompt", promptID)
	require.NoError(t, err)
	require.NotNil(t, remaining, "the second rule still matches, so it stays stale")
	assert.Equal(t, []string{ruleB}, remaining.MatchedRuleIDs)
	assert.Equal(t, both.Since, remaining.Since)
	assert.Len(t, auditEntries(t, scope.teamID), 1,
		"narrowing the rule set is not a status change, so it must not be audited")
}

// A rule scoped to one project, and a resource type it does not cover, must
// both narrow what the run marks -- proof the scope reaches the engine, not
// just the query.
func TestIntegrationFreshnessEvaluate_HonoursRuleScope(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	otherProject := insertTestProject(t, scope.userID, scope.teamID)
	ctx := context.Background()
	evaluator := newIntegrationEvaluator()
	state := NewResourceFreshnessRepository(integrationDB)

	inScope := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "in", "body", "published")
	outOfProject := insertTestPrompt(t, scope.userID, scope.teamID, otherProject, "out", "body", "published")
	otherType := insertTestArtifact(t, scope.userID, scope.teamID, scope.projectID, "art", "content", "active")
	touchResource(t, "prompts", inScope, "", daysAgo(120), nil)
	touchResource(t, "prompts", outOfProject, "", daysAgo(120), nil)
	touchResource(t, "artifacts", otherType, "", daysAgo(120), nil)

	rule := &models.FreshnessRule{
		TeamID:        scope.teamID,
		ProjectID:     &scope.projectID,
		ResourceTypes: []string{"prompt"},
		Mediums:       []string{},
		ThresholdDays: 30,
		Enabled:       true,
	}
	require.NoError(t, NewFreshnessRuleRepository(integrationDB).Create(ctx, rule))

	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))

	skippedProject, err := state.GetByResource(ctx, "prompt", outOfProject)
	require.NoError(t, err)
	assert.Nil(t, skippedProject, "a project the rule does not scope must not be marked")

	skippedType, err := state.GetByResource(ctx, "artifact", otherType)
	require.NoError(t, err)
	assert.Nil(t, skippedType, "a type the rule does not cover must not be marked")

	got, err := state.GetByResource(ctx, "prompt", inScope)
	require.NoError(t, err)
	assert.NotNil(t, got)
}

// One team's rules must never reach another team's resources, and evaluating a
// team with no rules at all must clear only its own state.
func TestIntegrationFreshnessEvaluate_IsTeamScoped(t *testing.T) {
	resetFreshnessTables(t)
	mine := seedCandidateScope(t)
	theirs := seedCandidateScope(t)
	ctx := context.Background()
	evaluator := newIntegrationEvaluator()
	state := NewResourceFreshnessRepository(integrationDB)

	myPrompt := insertTestPrompt(t, mine.userID, mine.teamID, mine.projectID, "mine", "body", "published")
	theirPrompt := insertTestPrompt(t, theirs.userID, theirs.teamID, theirs.projectID, "theirs", "body", "published")
	touchResource(t, "prompts", myPrompt, "", daysAgo(120), nil)
	touchResource(t, "prompts", theirPrompt, "", daysAgo(120), nil)
	insertEvaluationRule(t, mine.teamID, 30, "prompt")

	require.NoError(t, evaluator.Evaluate(ctx, mine.teamID))

	got, err := state.GetByResource(ctx, "prompt", theirPrompt)
	require.NoError(t, err)
	assert.Nil(t, got, "another team's resources are outside the run entirely")

	// The other team has no rules; its run must be a clean no-op rather than
	// clearing what the first team just marked.
	require.NoError(t, evaluator.Evaluate(ctx, theirs.teamID))

	stillMarked, err := state.GetByResource(ctx, "prompt", myPrompt)
	require.NoError(t, err)
	assert.NotNil(t, stillMarked)
	assert.Empty(t, auditEntries(t, theirs.teamID))
}

// A rule referencing a project that no longer exists cannot be created, so the
// evaluator never sees one -- but a rule whose id was stripped from freshness
// state must still reconcile cleanly on the next run.
func TestIntegrationFreshnessEvaluate_RecoversAfterRuleDeletion(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	ctx := context.Background()
	evaluator := newIntegrationEvaluator()
	state := NewResourceFreshnessRepository(integrationDB)
	rules := NewFreshnessRuleRepository(integrationDB)

	promptID := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "old", "body", "published")
	touchResource(t, "prompts", promptID, "", daysAgo(120), nil)
	ruleID := insertEvaluationRule(t, scope.teamID, 30, "prompt")
	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))

	// Delete the rule the way the service does: strip the state first, then
	// remove the rule.
	_, err := state.RemoveRule(ctx, ruleID)
	require.NoError(t, err)
	deleted, err := rules.Delete(ctx, scope.teamID, ruleID)
	require.NoError(t, err)
	require.True(t, deleted)

	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))

	got, err := state.GetByResource(ctx, "prompt", promptID)
	require.NoError(t, err)
	assert.Nil(t, got, "with the rule gone the resource is no longer stale")

	// The audit log outlives the rule: rule_id carries no foreign key precisely
	// so the history survives a deletion.
	entries := auditEntries(t, scope.teamID)
	require.Len(t, entries, 1, "RemoveRule already cleared the row, so this run had nothing left to clear")
	require.NotNil(t, entries[0].RuleID)
	assert.Equal(t, ruleID, *entries[0].RuleID)
}

// The #796 repair, end to end. Only real Postgres can show this: the sqlmock
// test proves the query ASKS for the newest entry per live row, but what
// "newest" resolves to -- and that the LATERAL join really is bounded by the
// state table -- is the database's answer, not the mock's.
//
// The starting position is what a failed audit write leaves behind: a live
// stale row whose newest entry says `cleared`. No re-run repairs it on its
// own, because the row's presence makes every later upsert an UPDATE, which
// takes the bookkeeping branch and writes nothing.
func TestIntegrationFreshnessEvaluate_RepairsLiveRowWhoseNewestEntryIsCleared(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	ctx := context.Background()
	evaluator := newIntegrationEvaluator()
	state := NewResourceFreshnessRepository(integrationDB)

	promptID := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "old", "body", "published")
	touchResource(t, "prompts", promptID, "", daysAgo(120), nil)
	ruleID := insertEvaluationRule(t, scope.teamID, 30, "prompt")

	// Run 1 marks it and audits the transition.
	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))
	marked, err := state.GetByResource(ctx, "prompt", promptID)
	require.NoError(t, err)
	require.NotNil(t, marked)
	require.Len(t, auditEntries(t, scope.teamID), 1)

	// Now reproduce the damage a failed audit write leaves: the row stays, but
	// the log's newest entry is a clear. Written directly rather than through
	// the clear path, because the clear path would also delete the row -- the
	// whole defect is the two halves disagreeing.
	appendAuditEntry(t, scope.teamID, "prompt", promptID, models.FreshnessActionCleared, &ruleID)
	require.Len(t, auditEntries(t, scope.teamID), 2)
	require.Equal(t, models.FreshnessActionCleared, auditEntries(t, scope.teamID)[0].Action,
		"the log now contradicts the live row")

	// Run 2 repairs it.
	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))

	entries := auditEntries(t, scope.teamID)
	require.Len(t, entries, 3, "exactly one entry is added -- the missing mark")
	assert.Equal(t, models.FreshnessActionMarked, entries[0].Action, "newest first")
	assert.Equal(t, models.FreshnessReasonRuleRun, entries[0].Reason)
	assert.Equal(t, promptID, entries[0].ResourceID)
	require.NotNil(t, entries[0].RuleID, "one rule matched, so it is attributable")
	assert.Equal(t, ruleID, *entries[0].RuleID)

	// The repair describes the row; it must not rewrite it.
	repaired, err := state.GetByResource(ctx, "prompt", promptID)
	require.NoError(t, err)
	require.NotNil(t, repaired)
	assert.Equal(t, marked.Since, repaired.Since)
	assert.Equal(t, marked.UpdatedAt, repaired.UpdatedAt,
		"a repair writes to the log, never to the state row")

	// Run 3 is the invariant the package doc rests on: the pass stays a no-op
	// once the log agrees with the state, so a repair cannot become a source
	// of one audit row per tick.
	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))
	assert.Len(t, auditEntries(t, scope.teamID), 3, "the repair is idempotent")
}

// A live stale row with NO audit history at all is the same defect, and the
// LATERAL join's NULL arm is what sees it -- a query over the log alone could
// only omit such a resource.
func TestIntegrationFreshnessEvaluate_RepairsLiveRowWithNoAuditHistory(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	ctx := context.Background()
	evaluator := newIntegrationEvaluator()

	promptID := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "old", "body", "published")
	touchResource(t, "prompts", promptID, "", daysAgo(120), nil)
	ruleID := insertEvaluationRule(t, scope.teamID, 30, "prompt")

	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))
	require.Len(t, auditEntries(t, scope.teamID), 1)

	// Wipe the log, leaving the state row: exactly what a mark whose audit
	// write never landed looks like on a resource that had no history before.
	_, err := integrationDB.ExecContext(ctx,
		"DELETE FROM resource_freshness_audit WHERE team_id = $1", scope.teamID)
	require.NoError(t, err)

	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))

	entries := auditEntries(t, scope.teamID)
	require.Len(t, entries, 1)
	assert.Equal(t, models.FreshnessActionMarked, entries[0].Action)
	require.NotNil(t, entries[0].RuleID)
	assert.Equal(t, ruleID, *entries[0].RuleID)
}

// A resource the run CLEARS must not then be repaired. Its newest entry is the
// clear this run just wrote, so a repair driven off the run's snapshot -- which
// still holds the cleared key -- would write a `marked` for a resource that is
// no longer stale: the defect being fixed here, inverted. Driving off the state
// table is what rules it out.
func TestIntegrationFreshnessEvaluate_ClearedResourceIsNotRepaired(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	ctx := context.Background()
	evaluator := newIntegrationEvaluator()
	state := NewResourceFreshnessRepository(integrationDB)

	promptID := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "old", "body", "published")
	touchResource(t, "prompts", promptID, "", daysAgo(120), nil)
	ruleID := insertEvaluationRule(t, scope.teamID, 30, "prompt")

	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))
	require.Len(t, auditEntries(t, scope.teamID), 1)

	// One run that both clears the resource and reaches the repair phase.
	disableRule(t, ruleID)
	require.NoError(t, evaluator.Evaluate(ctx, scope.teamID))

	gone, err := state.GetByResource(ctx, "prompt", promptID)
	require.NoError(t, err)
	require.Nil(t, gone)

	entries := auditEntries(t, scope.teamID)
	require.Len(t, entries, 2, "the clear, and nothing after it")
	assert.Equal(t, models.FreshnessActionCleared, entries[0].Action,
		"a cleared resource has no live row, so there is no mark to repair")
}
