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

// End-to-end proof of automatic reversal (#733) against real Postgres: mark a
// resource stale through the evaluator, then clear it the way an access or an
// edit does, and check what the two tables actually hold afterwards.
//
// The unit suite covers the decision matrix against mocks. What only this can
// show is that the clear and the mark agree about the same row — that the
// delete really removes what the evaluator wrote, and that the audit log ends
// up with the mark and the reversal as two entries rather than one or three.
//
// It lives in this package for the same reason the evaluator's suite does: the
// integration harness is here, and the clearer depends on repository
// interfaces, never on this package.

func newIntegrationClearer() *freshness.Clearer {
	return freshness.NewClearer(
		NewTeamFreshnessSettingsRepository(integrationDB),
		NewResourceFreshnessRepository(integrationDB),
		NewFreshnessAuditRepository(integrationDB),
		NewFreshnessRuleRepository(integrationDB),
		slog.New(slog.DiscardHandler),
	)
}

// clearAccess reverses the way a read does, through the given medium.
func clearAccess(t *testing.T, teamID, resourceID, medium string) error {
	t.Helper()
	return newIntegrationClearer().ClearIfStale(
		context.Background(), teamID, "prompt", resourceID, models.FreshnessReasonAccessed, medium)
}

// setReversibility stores the team's toggle. An absent row means "inherit the
// defaults", so tests that need a specific value must write one.
func setReversibility(t *testing.T, teamID string, enabled bool) {
	t.Helper()
	require.NoError(t, NewTeamFreshnessSettingsRepository(integrationDB).Upsert(context.Background(),
		&models.TeamFreshnessSettings{
			TeamID:               teamID,
			IntervalSeconds:      models.DefaultFreshnessIntervalSeconds,
			ReversibilityEnabled: enabled,
		}))
}

// markStaleForReversal drives the real evaluator so the row under test is
// exactly the one production would have written.
func markStaleForReversal(t *testing.T, scope candidateScope) string {
	t.Helper()
	return markStaleForReversalScoped(t, scope, nil)
}

// markStaleForReversalScoped is the same fixture with the rule's mediums under
// the caller's control, which is what the #770 cases need: with an all-medium
// rule the access genuinely does move the timestamp the rule reads, so the flap
// is structurally unreachable.
func markStaleForReversalScoped(t *testing.T, scope candidateScope, mediums []string) string {
	t.Helper()
	promptID := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "old", "body", "published")
	touchResource(t, "prompts", promptID, "", daysAgo(120), nil)
	insertScopedEvaluationRule(t, scope.teamID, 30, mediums, "prompt")

	require.NoError(t, newIntegrationEvaluator().Evaluate(context.Background(), scope.teamID))

	stale, err := NewResourceFreshnessRepository(integrationDB).GetByResource(context.Background(), "prompt", promptID)
	require.NoError(t, err)
	require.NotNil(t, stale, "fixture: the prompt must be stale before it can be reversed")
	return promptID
}

// With reversibility on, a read or an edit removes the row and appends exactly
// one entry naming which of the two it was.
func TestIntegrationFreshnessClear_ReversesOnAccessAndEdit(t *testing.T) {
	for _, reason := range []string{models.FreshnessReasonAccessed, models.FreshnessReasonEdited} {
		t.Run(reason, func(t *testing.T) {
			resetFreshnessTables(t)
			scope := seedCandidateScope(t)
			ctx := context.Background()
			promptID := markStaleForReversal(t, scope)
			setReversibility(t, scope.teamID, true)

			require.NoError(t, newIntegrationClearer().
				ClearIfStale(ctx, scope.teamID, "prompt", promptID, reason, "web"))

			cleared, err := NewResourceFreshnessRepository(integrationDB).GetByResource(ctx, "prompt", promptID)
			require.NoError(t, err)
			assert.Nil(t, cleared, "clearing deletes the row; the audit log preserves the history")

			entries := auditEntries(t, scope.teamID)
			require.Len(t, entries, 2, "the mark and the reversal, and nothing else")
			assert.Equal(t, models.FreshnessActionCleared, entries[0].Action, "newest first")
			assert.Equal(t, reason, entries[0].Reason)
			assert.Nil(t, entries[0].RuleID, "a reversal is not attributable to a rule")
			assert.Equal(t, models.FreshnessActionMarked, entries[1].Action)
		})
	}
}

// With reversibility off the row survives untouched, and no audit entry claims
// otherwise — the scheduled run stays the only authority, which is exactly what
// turning the toggle off asks for.
func TestIntegrationFreshnessClear_RespectsTheDisabledToggle(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	ctx := context.Background()
	promptID := markStaleForReversal(t, scope)
	setReversibility(t, scope.teamID, false)

	require.NoError(t, clearAccess(t, scope.teamID, promptID, "web"))

	stillStale, err := NewResourceFreshnessRepository(integrationDB).GetByResource(ctx, "prompt", promptID)
	require.NoError(t, err)
	assert.NotNil(t, stillStale)
	assert.Len(t, auditEntries(t, scope.teamID), 1, "only the mark")
}

// Reversing a resource that is not stale must be a cheap no-op — no row to
// delete, and above all no audit entry inventing a transition.
func TestIntegrationFreshnessClear_FreshResourceIsANoOp(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	promptID := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "fresh", "body", "published")
	setReversibility(t, scope.teamID, true)

	require.NoError(t, clearAccess(t, scope.teamID, promptID, "web"))

	assert.Empty(t, auditEntries(t, scope.teamID))
}

// The evaluator must be able to re-mark a resource a reversal cleared: the two
// halves of the loop have to compose, or a resource read once would never be
// flagged again.
func TestIntegrationFreshnessClear_EvaluatorRemarksAfterReversal(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	ctx := context.Background()
	promptID := markStaleForReversal(t, scope)
	setReversibility(t, scope.teamID, true)

	require.NoError(t, clearAccess(t, scope.teamID, promptID, "web"))

	// The reversal cleared the flag but did NOT touch the timestamps the rule
	// reads, so the resource is still untouched for 120 days and the next run
	// marks it again. That is by design: reversal reflects "someone looked at
	// it", and only a real access or edit moves the underlying dates.
	require.NoError(t, newIntegrationEvaluator().Evaluate(ctx, scope.teamID))

	remarked, err := NewResourceFreshnessRepository(integrationDB).GetByResource(ctx, "prompt", promptID)
	require.NoError(t, err)
	require.NotNil(t, remarked)

	entries := auditEntries(t, scope.teamID)
	require.Len(t, entries, 3)
	assert.Equal(t, models.FreshnessActionMarked, entries[0].Action)
	assert.Equal(t, models.FreshnessReasonRuleRun, entries[0].Reason)
}

// The #770 regression case, and the one the existing suites structurally cannot
// reach: with an mcp-scoped rule, a WEB read moves only last_accessed_web_at —
// a column the rule does not read — so reversing on it would clear a badge the
// very next evaluation run re-applies, once per interval, forever.
//
// The read is spelled as a direct Clearer call rather than through the access
// service, so what this pins is the guard plus the evaluator's real reaction to
// it. Deliberately: driving the whole access path would also move
// last_accessed_web_at, and then a second run would leave the mark standing for
// reasons that have nothing to do with the guard — the assertion would pass with
// the fix reverted.
//
// Asserted across two runs with the mismatched access in between, because a
// single run cannot tell "correctly refused" from "cleared and not yet
// re-marked". On main this test fails with three audit entries (marked,
// cleared, marked) and a resource that flapped.
func TestIntegrationFreshnessClear_MediumScopedRuleDoesNotFlap(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	ctx := context.Background()
	promptID := markStaleForReversalScoped(t, scope, []string{"mcp"})
	setReversibility(t, scope.teamID, true)

	require.NoError(t, clearAccess(t, scope.teamID, promptID, "web"))

	stillStale, err := NewResourceFreshnessRepository(integrationDB).GetByResource(ctx, "prompt", promptID)
	require.NoError(t, err)
	require.NotNil(t, stillStale, "a web read must not reverse a mark an mcp-scoped rule made")

	require.NoError(t, newIntegrationEvaluator().Evaluate(ctx, scope.teamID))

	entries := auditEntries(t, scope.teamID)
	require.Len(t, entries, 1, "one mark across two runs — no cleared/marked pair per interval")
	assert.Equal(t, models.FreshnessActionMarked, entries[0].Action)
	assert.Equal(t, models.FreshnessReasonRuleRun, entries[0].Reason)
}

// The other half of the same rule: an access through the medium it DOES name is
// a genuine touch of the column it reads, so it clears exactly as before —
// scoping the reversal must not amount to switching it off.
func TestIntegrationFreshnessClear_MediumScopedRuleClearsOnItsOwnMedium(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	ctx := context.Background()
	promptID := markStaleForReversalScoped(t, scope, []string{"mcp"})
	setReversibility(t, scope.teamID, true)

	require.NoError(t, clearAccess(t, scope.teamID, promptID, "mcp"))

	cleared, err := NewResourceFreshnessRepository(integrationDB).GetByResource(ctx, "prompt", promptID)
	require.NoError(t, err)
	assert.Nil(t, cleared)

	entries := auditEntries(t, scope.teamID)
	require.Len(t, entries, 2, "the mark and the reversal")
	assert.Equal(t, models.FreshnessActionCleared, entries[0].Action)
	assert.Equal(t, models.FreshnessReasonAccessed, entries[0].Reason)
}

// Reversal sits on the detail-read path, so its cost has to be an index seek
// rather than a scan of every stale row in the instance. Issue #733's HOW named
// this as the mitigation for "extra query on every read", and only an execution
// plan can show it: a correct-but-scanning query passes every behavioural test
// in this file.
//
// The predicate is copied from ResourceFreshnessRepository.GetByResource
// verbatim — asserting a plan for a query the repository does not issue would
// prove nothing.
func TestIntegrationFreshnessClear_LookupUsesTheUniqueIndex(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	promptID := markStaleForReversal(t, scope)

	plan := explainFreshness(t,
		"SELECT "+resourceFreshnessColumns+
			" FROM resource_freshness WHERE resource_type = $1 AND resource_id = $2",
		"prompt", promptID)

	assert.Contains(t, plan, "idx_resource_freshness_resource",
		"the reversal lookup must be served by the unique (resource_type, resource_id) index; plan was:\n"+plan)
}
