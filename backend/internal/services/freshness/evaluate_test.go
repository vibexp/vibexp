package freshness_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/freshness"
)

const (
	testTeamID    = "team-1"
	testProjectID = "project-1"
	testRuleID    = "rule-1"
	testOtherRule = "rule-2"
	testPromptID  = "prompt-1"
)

type evaluatorDeps struct {
	rules      *repomocks.MockFreshnessRuleRepository
	candidates *repomocks.MockFreshnessCandidateRepository
	state      *repomocks.MockResourceFreshnessRepository
	audit      *repomocks.MockFreshnessAuditRepository
}

func newEvaluator(t *testing.T) (*freshness.Evaluator, evaluatorDeps) {
	t.Helper()
	evaluator, deps, _ := newEvaluatorWithCounters(t)
	return evaluator, deps
}

// countingHandler captures the per-run tallies from the summary log record.
//
// marked/refreshed/cleared are unexported and returned only to Evaluate, which
// logs them — so the log line is the sole place they are observable, and
// asserting them means reading it. They are not decoration: "marked" claims a
// transition was audited and "refreshed" claims one was deliberately not, so a
// run that audits while reporting `refreshed` is lying about what it did.
type countingHandler struct {
	slog.Handler
	counts map[string]int64
}

// Enabled must be overridden: the embedded DiscardHandler reports false for
// every level, and slog checks that before it ever calls Handle -- so without
// this the counters silently stay empty and every assertion on them passes
// vacuously against zero.
func (h *countingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *countingHandler) Handle(_ context.Context, record slog.Record) error {
	record.Attrs(func(a slog.Attr) bool {
		if v, ok := a.Value.Any().(int64); ok {
			h.counts[a.Key] = v
		}
		return true
	})
	return nil
}

func newEvaluatorWithCounters(t *testing.T) (*freshness.Evaluator, evaluatorDeps, map[string]int64) {
	t.Helper()

	deps := evaluatorDeps{
		rules:      repomocks.NewMockFreshnessRuleRepository(t),
		candidates: repomocks.NewMockFreshnessCandidateRepository(t),
		state:      repomocks.NewMockResourceFreshnessRepository(t),
		audit:      repomocks.NewMockFreshnessAuditRepository(t),
	}
	counts := map[string]int64{}
	evaluator := freshness.NewEvaluator(
		deps.rules, deps.candidates, deps.state, deps.audit,
		slog.New(&countingHandler{Handler: slog.DiscardHandler, counts: counts}))
	return evaluator, deps, counts
}

// rule builds an enabled rule over one resource type.
func rule(id string, resourceTypes ...string) *models.FreshnessRule {
	return &models.FreshnessRule{
		ID:            id,
		TeamID:        testTeamID,
		ResourceTypes: resourceTypes,
		ThresholdDays: 30,
		Enabled:       true,
	}
}

// promptCandidate is the stale prompt every scenario below revolves around.
func promptCandidate() models.FreshnessCandidate {
	return models.FreshnessCandidate{
		ResourceType: "prompt",
		ResourceID:   testPromptID,
		ProjectID:    testProjectID,
	}
}

// storedStale is the persisted freshness row for that prompt.
func storedStale(ruleIDs ...string) *models.ResourceFreshness {
	return &models.ResourceFreshness{
		TeamID:         testTeamID,
		ProjectID:      testProjectID,
		ResourceType:   "prompt",
		ResourceID:     testPromptID,
		Status:         models.FreshnessStatusStale,
		MatchedRuleIDs: ruleIDs,
	}
}

// expectCandidates answers one (rule, resource type) query with the given
// candidates. The evaluator pages until a short page comes back, and these
// pages are always short, so one expectation per query is right.
func (d evaluatorDeps) expectCandidates(
	resourceType string, candidates ...models.FreshnessCandidate,
) {
	d.candidates.EXPECT().
		ListStaleCandidates(mock.Anything, mock.MatchedBy(func(q models.FreshnessCandidateQuery) bool {
			return q.TeamID == testTeamID && q.ResourceType == resourceType && q.AfterID == ""
		})).
		Return(candidates, nil).Once()
}

// A resource the rules match and the state does not know about is marked, with
// exactly one audit row recording the transition.
func TestEvaluate_MarksNewlyStaleResource(t *testing.T) {
	evaluator, deps := newEvaluator(t)

	deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
		Return([]*models.FreshnessRule{rule(testRuleID, "prompt")}, nil).Once()
	deps.expectCandidates("prompt", promptCandidate())
	deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
		Return([]*models.ResourceFreshness{}, nil).Once()

	var written *models.ResourceFreshness
	deps.state.EXPECT().Upsert(mock.Anything, mock.Anything).
		Run(func(_ context.Context, f *models.ResourceFreshness) { written = f }).
		Return(true, nil).Once()

	var logged *models.ResourceFreshnessAudit
	deps.audit.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, e *models.ResourceFreshnessAudit) { logged = e }).
		Return(nil).Once()

	require.NoError(t, evaluator.Evaluate(context.Background(), testTeamID))

	require.NotNil(t, written)
	assert.Equal(t, models.FreshnessStatusStale, written.Status)
	assert.Equal(t, models.FreshnessReasonRuleRun, written.Reason)
	assert.Equal(t, testProjectID, written.ProjectID)
	assert.Equal(t, []string{testRuleID}, written.MatchedRuleIDs)
	// Zero Since is what makes the repository preserve "first marked at" for a
	// resource that was already stale.
	assert.True(t, written.Since.IsZero())

	require.NotNil(t, logged)
	assert.Equal(t, models.FreshnessActionMarked, logged.Action)
	assert.Equal(t, models.FreshnessReasonRuleRun, logged.Reason)
	require.NotNil(t, logged.RuleID)
	assert.Equal(t, testRuleID, *logged.RuleID)
}

// Decision #5: a resource stale under several rules carries all of them, and
// the audit row attributes none of them rather than picking one arbitrarily.
func TestEvaluate_UnionsMatchingRules(t *testing.T) {
	evaluator, deps := newEvaluator(t)

	// Returned in reverse id order to prove the union is sorted, not insertion
	// ordered -- the stored/desired comparison depends on it.
	deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
		Return([]*models.FreshnessRule{rule(testOtherRule, "prompt"), rule(testRuleID, "prompt")}, nil).Once()
	deps.candidates.EXPECT().ListStaleCandidates(mock.Anything, mock.Anything).
		Return([]models.FreshnessCandidate{promptCandidate()}, nil).Twice()
	deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
		Return([]*models.ResourceFreshness{}, nil).Once()

	var written *models.ResourceFreshness
	deps.state.EXPECT().Upsert(mock.Anything, mock.Anything).
		Run(func(_ context.Context, f *models.ResourceFreshness) { written = f }).
		Return(true, nil).Once()

	var logged *models.ResourceFreshnessAudit
	deps.audit.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, e *models.ResourceFreshnessAudit) { logged = e }).
		Return(nil).Once()

	require.NoError(t, evaluator.Evaluate(context.Background(), testTeamID))

	require.NotNil(t, written)
	assert.Equal(t, []string{testRuleID, testOtherRule}, written.MatchedRuleIDs)
	require.NotNil(t, logged)
	assert.Nil(t, logged.RuleID, "several rules matched, so no single rule caused it")
}

// A rule covering several resource types is evaluated once per type, and the
// results merge into one desired set.
func TestEvaluate_EvaluatesEveryResourceTypeOfARule(t *testing.T) {
	evaluator, deps := newEvaluator(t)

	deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
		Return([]*models.FreshnessRule{rule(testRuleID, "prompt", "memory")}, nil).Once()
	deps.expectCandidates("prompt", promptCandidate())
	deps.expectCandidates("memory", models.FreshnessCandidate{
		ResourceType: "memory", ResourceID: "memory-1", ProjectID: testProjectID,
	})
	deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
		Return([]*models.ResourceFreshness{}, nil).Once()

	marked := make(map[string]string)
	deps.state.EXPECT().Upsert(mock.Anything, mock.Anything).
		Run(func(_ context.Context, f *models.ResourceFreshness) { marked[f.ResourceType] = f.ResourceID }).
		Return(true, nil).Twice()
	deps.audit.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Twice()

	require.NoError(t, evaluator.Evaluate(context.Background(), testTeamID))
	assert.Equal(t, map[string]string{"prompt": testPromptID, "memory": "memory-1"}, marked)
}

// Re-running immediately must write nothing at all: no state write and, above
// all, no audit row. This is the property that keeps the audit log readable.
func TestEvaluate_IsIdempotent(t *testing.T) {
	evaluator, deps := newEvaluator(t)

	deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
		Return([]*models.FreshnessRule{rule(testRuleID, "prompt")}, nil).Once()
	deps.expectCandidates("prompt", promptCandidate())
	deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
		Return([]*models.ResourceFreshness{storedStale(testRuleID)}, nil).Once()

	// No Upsert, DeleteByResource or audit Create is expected; the t-bound
	// mocks fail the test if any of them is called.
	require.NoError(t, evaluator.Evaluate(context.Background(), testTeamID))
}

// A stored rule set in a different order than the computed one is still the
// same set, so it must not provoke a write on every single run.
func TestEvaluate_StoredRuleOrderDoesNotForceAWrite(t *testing.T) {
	evaluator, deps := newEvaluator(t)

	deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
		Return([]*models.FreshnessRule{rule(testRuleID, "prompt"), rule(testOtherRule, "prompt")}, nil).Once()
	deps.candidates.EXPECT().ListStaleCandidates(mock.Anything, mock.Anything).
		Return([]models.FreshnessCandidate{promptCandidate()}, nil).Twice()
	deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
		Return([]*models.ResourceFreshness{storedStale(testOtherRule, testRuleID)}, nil).Once()

	require.NoError(t, evaluator.Evaluate(context.Background(), testTeamID))
}

// When one of two matching rules stops matching, the row is rewritten with the
// narrowed set -- but the resource never stopped being stale, so no audit row
// is written. Auditing here is what would flood the log.
//
// The upsert reporting `inserted = false` is what makes this the bookkeeping
// path: the row the snapshot saw was still there for it to update. This is the
// #732 guard, and the half of #771's change that must NOT widen -- the t-bound
// audit mock has no expectation, so a stray audit row fails the test.
func TestEvaluate_NarrowedRuleSetRefreshesWithoutAudit(t *testing.T) {
	evaluator, deps, counts := newEvaluatorWithCounters(t)

	deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
		Return([]*models.FreshnessRule{rule(testRuleID, "prompt")}, nil).Once()
	deps.expectCandidates("prompt", promptCandidate())
	deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
		Return([]*models.ResourceFreshness{storedStale(testRuleID, testOtherRule)}, nil).Once()

	var written *models.ResourceFreshness
	deps.state.EXPECT().Upsert(mock.Anything, mock.Anything).
		Run(func(_ context.Context, f *models.ResourceFreshness) { written = f }).
		Return(false, nil).Once()

	require.NoError(t, evaluator.Evaluate(context.Background(), testTeamID))
	require.NotNil(t, written)
	assert.Equal(t, []string{testRuleID}, written.MatchedRuleIDs)

	// Reported as bookkeeping, and nothing was audited — the counter and the
	// absent audit row have to agree, or the log line misdescribes the run.
	assert.Equal(t, int64(1), counts["refreshed"])
	assert.Equal(t, int64(0), counts["marked"])
}

// The #771 race. Same setup as the narrowed-rule-set case above -- the snapshot
// says the resource is already stale -- but a Clearer (an access or an edit,
// #733) deleted the row between the snapshot and the upsert, so the upsert
// INSERTED. That is a real stale-again transition: without the audit row, the
// newest entry for this resource would be the clear, and the audit tab would
// contradict the live state.
//
// It is asserted through the mocked `inserted` flag rather than by a real
// concurrent delete, because that flag IS the fix -- it is how the evaluator
// learns what the database did instead of trusting its snapshot.
func TestEvaluate_ResourceClearedMidRunIsAuditedAsMarkedAgain(t *testing.T) {
	evaluator, deps, counts := newEvaluatorWithCounters(t)

	deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
		Return([]*models.FreshnessRule{rule(testRuleID, "prompt")}, nil).Once()
	deps.expectCandidates("prompt", promptCandidate())
	deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
		Return([]*models.ResourceFreshness{storedStale(testRuleID, testOtherRule)}, nil).Once()

	// The clear happened here, so the upsert inserts rather than updates.
	deps.state.EXPECT().Upsert(mock.Anything, mock.Anything).Return(true, nil).Once()

	var logged *models.ResourceFreshnessAudit
	deps.audit.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, e *models.ResourceFreshnessAudit) { logged = e }).
		Return(nil).Once()

	require.NoError(t, evaluator.Evaluate(context.Background(), testTeamID))

	require.NotNil(t, logged, "an insert is a transition and must be audited, whatever the snapshot said")
	assert.Equal(t, models.FreshnessActionMarked, logged.Action)
	assert.Equal(t, models.FreshnessReasonRuleRun, logged.Reason)
	assert.Equal(t, testPromptID, logged.ResourceID)
	require.NotNil(t, logged.RuleID, "one matched rule is attributable")
	assert.Equal(t, testRuleID, *logged.RuleID)

	// The run must also REPORT it as a mark, not as bookkeeping: an operator
	// reading "refreshed: 1" would conclude nothing was audited.
	assert.Equal(t, int64(1), counts["marked"])
	assert.Equal(t, int64(0), counts["refreshed"])
}

// Clearing is the other half of reconciliation: a stored row no rule matches
// any more is deleted and audited as cleared.
func TestEvaluate_ClearsResourceNoRuleMatches(t *testing.T) {
	evaluator, deps := newEvaluator(t)

	deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
		Return([]*models.FreshnessRule{rule(testRuleID, "prompt")}, nil).Once()
	deps.expectCandidates("prompt")
	deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
		Return([]*models.ResourceFreshness{storedStale(testRuleID)}, nil).Once()
	deps.state.EXPECT().DeleteByResource(mock.Anything, "prompt", testPromptID).
		Return(true, nil).Once()

	var logged *models.ResourceFreshnessAudit
	deps.audit.EXPECT().Create(mock.Anything, mock.Anything).
		Run(func(_ context.Context, e *models.ResourceFreshnessAudit) { logged = e }).
		Return(nil).Once()

	require.NoError(t, evaluator.Evaluate(context.Background(), testTeamID))

	require.NotNil(t, logged)
	assert.Equal(t, models.FreshnessActionCleared, logged.Action)
	assert.Equal(t, models.FreshnessReasonRuleRun, logged.Reason)
	require.NotNil(t, logged.RuleID)
	assert.Equal(t, testRuleID, *logged.RuleID)
}

// Disabling the last rule is how the feature is turned off, so the run must
// still clear everything rather than exit early on "no rules".
func TestEvaluate_NoEnabledRulesClearsEverything(t *testing.T) {
	evaluator, deps := newEvaluator(t)

	deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
		Return([]*models.FreshnessRule{}, nil).Once()
	deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
		Return([]*models.ResourceFreshness{storedStale(testRuleID)}, nil).Once()
	deps.state.EXPECT().DeleteByResource(mock.Anything, "prompt", testPromptID).
		Return(true, nil).Once()
	deps.audit.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()

	require.NoError(t, evaluator.Evaluate(context.Background(), testTeamID))
}

// Something else cleared the row first (an access or an edit, #733). That
// clear has its own audit entry, so this run must not invent a second one.
func TestEvaluate_ClearAlreadyGoneWritesNoAudit(t *testing.T) {
	evaluator, deps := newEvaluator(t)

	deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
		Return([]*models.FreshnessRule{}, nil).Once()
	deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
		Return([]*models.ResourceFreshness{storedStale(testRuleID)}, nil).Once()
	deps.state.EXPECT().DeleteByResource(mock.Anything, "prompt", testPromptID).
		Return(false, nil).Once()

	require.NoError(t, evaluator.Evaluate(context.Background(), testTeamID))
}

// A rule matching more resources than one batch holds must be paged through,
// with the last id of each page carried into the next query.
func TestEvaluate_PagesThroughCandidates(t *testing.T) {
	evaluator, deps := newEvaluator(t)

	first := make([]models.FreshnessCandidate, 500)
	for i := range first {
		first[i] = models.FreshnessCandidate{
			ResourceType: "prompt",
			ResourceID:   promptIDForIndex(i),
			ProjectID:    testProjectID,
		}
	}
	lastOfFirstPage := first[len(first)-1].ResourceID

	deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
		Return([]*models.FreshnessRule{rule(testRuleID, "prompt")}, nil).Once()
	deps.candidates.EXPECT().
		ListStaleCandidates(mock.Anything, mock.MatchedBy(func(q models.FreshnessCandidateQuery) bool {
			return q.AfterID == ""
		})).
		Return(first, nil).Once()
	deps.candidates.EXPECT().
		ListStaleCandidates(mock.Anything, mock.MatchedBy(func(q models.FreshnessCandidateQuery) bool {
			return q.AfterID == lastOfFirstPage
		})).
		Return([]models.FreshnessCandidate{promptCandidate()}, nil).Once()

	deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
		Return([]*models.ResourceFreshness{}, nil).Once()
	deps.state.EXPECT().Upsert(mock.Anything, mock.Anything).Return(true, nil).Times(501)
	deps.audit.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Times(501)

	require.NoError(t, evaluator.Evaluate(context.Background(), testTeamID))
}

// Every repository failure must surface, not be swallowed: a run that silently
// half-applied would leave state nobody can explain.
func TestEvaluate_PropagatesRepositoryErrors(t *testing.T) {
	failure := errors.New("boom")

	tests := []struct {
		name  string
		setup func(deps evaluatorDeps)
	}{
		{
			name: "loading rules",
			setup: func(deps evaluatorDeps) {
				deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).Return(nil, failure).Once()
			},
		},
		{
			name: "listing candidates",
			setup: func(deps evaluatorDeps) {
				deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
					Return([]*models.FreshnessRule{rule(testRuleID, "prompt")}, nil).Once()
				deps.candidates.EXPECT().ListStaleCandidates(mock.Anything, mock.Anything).
					Return(nil, failure).Once()
			},
		},
		{
			name: "loading stored state",
			setup: func(deps evaluatorDeps) {
				deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
					Return([]*models.FreshnessRule{}, nil).Once()
				deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).Return(nil, failure).Once()
			},
		},
		{
			name: "marking stale",
			setup: func(deps evaluatorDeps) {
				deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
					Return([]*models.FreshnessRule{rule(testRuleID, "prompt")}, nil).Once()
				deps.expectCandidates("prompt", promptCandidate())
				deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
					Return([]*models.ResourceFreshness{}, nil).Once()
				deps.state.EXPECT().Upsert(mock.Anything, mock.Anything).Return(false, failure).Once()
			},
		},
		{
			name: "writing the mark audit",
			setup: func(deps evaluatorDeps) {
				deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
					Return([]*models.FreshnessRule{rule(testRuleID, "prompt")}, nil).Once()
				deps.expectCandidates("prompt", promptCandidate())
				deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
					Return([]*models.ResourceFreshness{}, nil).Once()
				deps.state.EXPECT().Upsert(mock.Anything, mock.Anything).Return(true, nil).Once()
				deps.audit.EXPECT().Create(mock.Anything, mock.Anything).Return(failure).Once()
			},
		},
		{
			name: "clearing",
			setup: func(deps evaluatorDeps) {
				deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
					Return([]*models.FreshnessRule{}, nil).Once()
				deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
					Return([]*models.ResourceFreshness{storedStale(testRuleID)}, nil).Once()
				deps.state.EXPECT().DeleteByResource(mock.Anything, "prompt", testPromptID).
					Return(false, failure).Once()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator, deps := newEvaluator(t)
			tt.setup(deps)

			err := evaluator.Evaluate(context.Background(), testTeamID)

			require.Error(t, err)
			assert.ErrorIs(t, err, failure)
		})
	}
}

// promptIDForIndex builds a distinct, ordered resource id per index.
func promptIDForIndex(i int) string {
	const digits = "0123456789"
	return "prompt-" + string([]byte{
		digits[(i/100)%10], digits[(i/10)%10], digits[i%10],
	})
}

// Exhausting the batch cap must ABORT, not return a truncated match set.
// Continuing would treat every unread resource as no longer stale and clear
// it -- turning "I could not read everything" into an active mass un-flagging
// with an audit row per resource, re-marked on the next run.
func TestEvaluate_BatchCapAbortsInsteadOfTruncating(t *testing.T) {
	evaluator, deps := newEvaluator(t)

	full := make([]models.FreshnessCandidate, 500)
	for i := range full {
		full[i] = models.FreshnessCandidate{
			ResourceType: "prompt",
			ResourceID:   promptIDForIndex(i),
			ProjectID:    testProjectID,
		}
	}

	deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
		Return([]*models.FreshnessRule{rule(testRuleID, "prompt")}, nil).Once()
	// Every page comes back full, so the cursor never reaches the end.
	deps.candidates.EXPECT().ListStaleCandidates(mock.Anything, mock.Anything).
		Return(full, nil).Times(1000)

	err := evaluator.Evaluate(context.Background(), testTeamID)

	require.Error(t, err)
	assert.ErrorIs(t, err, freshness.ErrCandidateBatchCapExceeded)
	// No ListAllByTeam, no Upsert, no DeleteByResource, no audit Create is
	// expected: the t-bound mocks fail the test if the run touched state.
}

// Clearing runs before marking. The scheduler bounds the handler with a
// timeout and marking is the unbounded half, so a team large enough to run out
// of time must still have had its clears applied.
func TestEvaluate_ClearsBeforeMarks(t *testing.T) {
	evaluator, deps := newEvaluator(t)

	goneStale := models.FreshnessCandidate{
		ResourceType: "prompt", ResourceID: "prompt-2", ProjectID: testProjectID,
	}

	deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).
		Return([]*models.FreshnessRule{rule(testRuleID, "prompt")}, nil).Once()
	deps.expectCandidates("prompt", goneStale)
	deps.state.EXPECT().ListAllByTeam(mock.Anything, testTeamID).
		Return([]*models.ResourceFreshness{storedStale(testRuleID)}, nil).Once()

	var order []string
	deps.state.EXPECT().DeleteByResource(mock.Anything, "prompt", testPromptID).
		Run(func(_ context.Context, _, _ string) { order = append(order, "clear") }).
		Return(true, nil).Once()
	deps.state.EXPECT().Upsert(mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ *models.ResourceFreshness) { order = append(order, "mark") }).
		Return(true, nil).Once()
	deps.audit.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Twice()

	require.NoError(t, evaluator.Evaluate(context.Background(), testTeamID))
	assert.Equal(t, []string{"clear", "mark"}, order)
}
