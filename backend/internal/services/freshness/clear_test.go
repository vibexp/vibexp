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

// testAccessMedium is the medium the pre-#770 cases implicitly assumed: any
// one at all, against the default "any medium" rule.
const testAccessMedium = "web"

type clearerDeps struct {
	settings *repomocks.MockTeamFreshnessSettingsRepository
	state    *repomocks.MockResourceFreshnessRepository
	audit    *repomocks.MockFreshnessAuditRepository
	rules    *repomocks.MockFreshnessRuleRepository
}

func newClearer(t *testing.T) (*freshness.Clearer, clearerDeps) {
	t.Helper()

	deps := clearerDeps{
		settings: repomocks.NewMockTeamFreshnessSettingsRepository(t),
		state:    repomocks.NewMockResourceFreshnessRepository(t),
		audit:    repomocks.NewMockFreshnessAuditRepository(t),
		rules:    repomocks.NewMockFreshnessRuleRepository(t),
	}
	clearer := freshness.NewClearer(
		deps.settings, deps.state, deps.audit, deps.rules, slog.New(slog.DiscardHandler))
	return clearer, deps
}

// expectStale answers the freshness lookup with a stale row for the team.
func (d clearerDeps) expectStale() {
	d.state.EXPECT().GetByResource(mock.Anything, "prompt", testPromptID).
		Return(&models.ResourceFreshness{
			TeamID:         testTeamID,
			ProjectID:      testProjectID,
			ResourceType:   "prompt",
			ResourceID:     testPromptID,
			Status:         models.FreshnessStatusStale,
			MatchedRuleIDs: []string{testRuleID},
		}, nil).Once()
}

// expectRules answers the rule lookup the access path makes to decide whether
// the access medium is one the matching rules watch (#770).
func (d clearerDeps) expectRules(rules ...*models.FreshnessRule) {
	d.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).Return(rules, nil).Once()
}

// expectAnyMediumRule answers it with the default rule — no mediums, i.e. "any
// medium" — which is what every case predating #770 assumed and what the
// overwhelming majority of teams run.
func (d clearerDeps) expectAnyMediumRule() {
	d.expectRules(&models.FreshnessRule{ID: testRuleID, TeamID: testTeamID})
}

// expectReversibility answers the settings lookup with the given toggle.
func (d clearerDeps) expectReversibility(enabled bool) {
	d.settings.EXPECT().Get(mock.Anything, testTeamID).
		Return(&models.TeamFreshnessSettings{
			TeamID:               testTeamID,
			IntervalSeconds:      models.DefaultFreshnessIntervalSeconds,
			ReversibilityEnabled: enabled,
		}, nil).Once()
}

func clear(t *testing.T, c *freshness.Clearer, reason string) error {
	t.Helper()
	return clearVia(t, c, reason, testAccessMedium)
}

func clearVia(t *testing.T, c *freshness.Clearer, reason, medium string) error {
	t.Helper()
	return c.ClearIfStale(context.Background(), testTeamID, "prompt", testPromptID, reason, medium)
}

// The full matrix the acceptance criteria name: {on, off} x {accessed, edited}
// x {stale, not stale}. Reversal is the whole point of the feature, so the
// combinations are asserted rather than sampled.
func TestClearIfStale_Matrix(t *testing.T) {
	tests := []struct {
		name        string
		stale       bool
		enabled     bool
		reason      string
		wantCleared bool
	}{
		{name: "stale + on + accessed clears", stale: true, enabled: true,
			reason: models.FreshnessReasonAccessed, wantCleared: true},
		{name: "stale + on + edited clears", stale: true, enabled: true,
			reason: models.FreshnessReasonEdited, wantCleared: true},
		{name: "stale + off + accessed keeps state", stale: true, enabled: false,
			reason: models.FreshnessReasonAccessed},
		{name: "stale + off + edited keeps state", stale: true, enabled: false,
			reason: models.FreshnessReasonEdited},
		{name: "not stale + on + accessed is a no-op", reason: models.FreshnessReasonAccessed, enabled: true},
		{name: "not stale + on + edited is a no-op", reason: models.FreshnessReasonEdited, enabled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearer, deps := newClearer(t)

			if !tt.stale {
				// The cheap short-circuit: a fresh resource costs one indexed
				// miss and must not read settings OR rules at all. The t-bound
				// mocks have no expectation, so either read fails the test.
				deps.state.EXPECT().GetByResource(mock.Anything, "prompt", testPromptID).
					Return(nil, nil).Once()
			} else {
				deps.expectStale()
				if tt.reason == models.FreshnessReasonAccessed {
					deps.expectAnyMediumRule()
				}
				deps.expectReversibility(tt.enabled)
			}

			var logged *models.ResourceFreshnessAudit
			if tt.wantCleared {
				deps.state.EXPECT().DeleteByResource(mock.Anything, "prompt", testPromptID).
					Return(true, nil).Once()
				deps.audit.EXPECT().Create(mock.Anything, mock.Anything).
					Run(func(_ context.Context, e *models.ResourceFreshnessAudit) { logged = e }).
					Return(nil).Once()
			}

			require.NoError(t, clear(t, clearer, tt.reason))

			if !tt.wantCleared {
				// No DeleteByResource and no audit Create were expected; the
				// t-bound mocks fail the test if either happened.
				return
			}
			require.NotNil(t, logged)
			assert.Equal(t, models.FreshnessActionCleared, logged.Action)
			assert.Equal(t, tt.reason, logged.Reason)
			assert.Equal(t, testTeamID, logged.TeamID)
			assert.Equal(t, testPromptID, logged.ResourceID)
			assert.Nil(t, logged.RuleID,
				"a reversal is caused by the reader or editor, not by any rule that had matched")
		})
	}
}

// The #770 fix: an ACCESS clears only when its medium is one the rules that
// marked the resource actually watch. Anything else would be undone by the very
// next evaluation run — a badge flapping once per interval, and a
// cleared/marked audit pair per interval per resource.
//
// The reason axis is in the table on purpose: an EDIT must clear regardless of
// the rule's mediums, because `updated_at` is in every rule's staleness
// expression whatever mediums it names, so an edit-triggered clear can never
// flap.
func TestClearIfStale_ScopesAccessReversalToTheMatchedRulesMediums(t *testing.T) {
	tests := []struct {
		name        string
		rules       []*models.FreshnessRule
		reason      string
		medium      string
		wantCleared bool
	}{
		{
			name:   "mcp-scoped rule does not clear on a web access",
			rules:  []*models.FreshnessRule{{ID: testRuleID, TeamID: testTeamID, Mediums: []string{"mcp"}}},
			reason: models.FreshnessReasonAccessed, medium: "web",
		},
		{
			name:        "mcp-scoped rule clears on an mcp access",
			rules:       []*models.FreshnessRule{{ID: testRuleID, TeamID: testTeamID, Mediums: []string{"mcp"}}},
			reason:      models.FreshnessReasonAccessed,
			medium:      "mcp",
			wantCleared: true,
		},
		{
			name:        "any-medium rule clears on any access",
			rules:       []*models.FreshnessRule{{ID: testRuleID, TeamID: testTeamID}},
			reason:      models.FreshnessReasonAccessed,
			medium:      "cli",
			wantCleared: true,
		},
		{
			// `api` is deliberately not valid rule input, but it IS in the
			// any-medium column set, so it matches an unscoped rule only.
			name:   "api access does not clear a medium-scoped rule",
			rules:  []*models.FreshnessRule{{ID: testRuleID, TeamID: testTeamID, Mediums: []string{"mcp", "web"}}},
			reason: models.FreshnessReasonAccessed, medium: "api",
		},
		{
			name:        "api access clears an any-medium rule",
			rules:       []*models.FreshnessRule{{ID: testRuleID, TeamID: testTeamID}},
			reason:      models.FreshnessReasonAccessed,
			medium:      "api",
			wantCleared: true,
		},
		{
			// The evaluator marks on the UNION of its rules, so the reversal
			// answers on the union too: one matching rule is enough.
			name: "union of several matched rules clears",
			rules: []*models.FreshnessRule{
				{ID: testRuleID, TeamID: testTeamID, Mediums: []string{"mcp"}},
				{ID: "rule-2", TeamID: testTeamID, Mediums: []string{"web"}},
			},
			reason:      models.FreshnessReasonAccessed,
			medium:      "web",
			wantCleared: true,
		},
		{
			name: "no matched rule names the medium, even across several",
			rules: []*models.FreshnessRule{
				{ID: testRuleID, TeamID: testTeamID, Mediums: []string{"mcp"}},
				{ID: "rule-2", TeamID: testTeamID, Mediums: []string{"cli"}},
			},
			reason: models.FreshnessReasonAccessed, medium: "web",
		},
		{
			// Every rule that marked this resource has since been deleted,
			// disabled or narrowed, so none of them resolves. Nothing survives
			// to re-mark it, which makes this the same case as "no rule claims
			// it": clear now rather than leave the badge up for a whole interval
			// waiting for the evaluator to reach the same conclusion. Disabling
			// a rule is an ordinary admin action — UpdateRule does not strip the
			// state it produced — so this path is reached without any race.
			name:        "clears when no matched rule resolves any more",
			rules:       []*models.FreshnessRule{{ID: "some-other-rule", TeamID: testTeamID}},
			reason:      models.FreshnessReasonAccessed,
			medium:      "web",
			wantCleared: true,
		},
		{
			name: "a resolvable sibling still clears when one id is dangling",
			rules: []*models.FreshnessRule{
				{ID: "rule-2", TeamID: testTeamID, Mediums: []string{"web"}},
			},
			reason:      models.FreshnessReasonAccessed,
			medium:      "web",
			wantCleared: true,
		},
		{
			// The distinction the previous two cases turn on: one live rule that
			// does NOT watch this medium is enough to refuse, even though the
			// other matched id is dangling. Only a surviving rule can re-mark.
			name: "a dangling id does not rescue a live rule that ignores the medium",
			rules: []*models.FreshnessRule{
				{ID: "rule-2", TeamID: testTeamID, Mediums: []string{"mcp"}},
			},
			reason: models.FreshnessReasonAccessed, medium: "web",
		},
		{
			name:        "an edit clears regardless of the rule's mediums",
			rules:       []*models.FreshnessRule{{ID: testRuleID, TeamID: testTeamID, Mediums: []string{"mcp"}}},
			reason:      models.FreshnessReasonEdited,
			medium:      models.FreshnessMediumNone,
			wantCleared: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearer, deps := newClearer(t)

			deps.state.EXPECT().GetByResource(mock.Anything, "prompt", testPromptID).
				Return(&models.ResourceFreshness{
					TeamID:         testTeamID,
					ResourceType:   "prompt",
					ResourceID:     testPromptID,
					Status:         models.FreshnessStatusStale,
					MatchedRuleIDs: []string{testRuleID, "rule-2"},
				}, nil).Once()

			if tt.reason == models.FreshnessReasonAccessed {
				deps.expectRules(tt.rules...)
			}
			// An edit never reads rules at all: the t-bound rule mock has no
			// expectation in that case, so a lookup fails the test.
			if tt.wantCleared {
				deps.expectReversibility(true)
				deps.state.EXPECT().DeleteByResource(mock.Anything, "prompt", testPromptID).
					Return(true, nil).Once()
				deps.audit.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
			}
			// When it must NOT clear, the settings read never happens either —
			// the check sits before it — and the t-bound mocks enforce that
			// along with the absent delete and audit rows.

			require.NoError(t, clearVia(t, clearer, tt.reason, tt.medium))
		})
	}
}

// A row nothing claims cannot be re-marked, so an access clears it rather than
// waiting a whole interval for the evaluator to reach the same conclusion.
func TestClearIfStale_ClearsWhenNoRuleClaimsTheResource(t *testing.T) {
	clearer, deps := newClearer(t)

	deps.state.EXPECT().GetByResource(mock.Anything, "prompt", testPromptID).
		Return(&models.ResourceFreshness{
			TeamID:       testTeamID,
			ResourceType: "prompt",
			ResourceID:   testPromptID,
			Status:       models.FreshnessStatusStale,
		}, nil).Once()
	deps.expectReversibility(true)
	deps.state.EXPECT().DeleteByResource(mock.Anything, "prompt", testPromptID).Return(true, nil).Once()
	deps.audit.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()

	require.NoError(t, clear(t, clearer, models.FreshnessReasonAccessed))
	// No rule lookup either: with no ids to resolve there is nothing to ask
	// about, and the t-bound rule mock fails the test if one happened.
}

// The cheap-path guarantee, stated on its own rather than left implicit in the
// matrix: this sits on every read, so a fresh resource must cost exactly one
// indexed miss and touch neither settings nor rules.
func TestClearIfStale_FreshResourceReadsNeitherRulesNorSettings(t *testing.T) {
	clearer, deps := newClearer(t)

	deps.state.EXPECT().GetByResource(mock.Anything, "prompt", testPromptID).Return(nil, nil).Once()

	require.NoError(t, clear(t, clearer, models.FreshnessReasonAccessed))

	deps.rules.AssertNotCalled(t, "ListByTeam", mock.Anything, mock.Anything, mock.Anything)
	deps.settings.AssertNotCalled(t, "Get", mock.Anything, mock.Anything)
}

// A team with no stored settings row inherits the defaults, and the default is
// reversibility ON — so a team that never opened the settings card still gets
// the behaviour the epic describes.
func TestClearIfStale_AbsentSettingsInheritTheDefault(t *testing.T) {
	clearer, deps := newClearer(t)

	deps.expectStale()
	deps.expectAnyMediumRule()
	deps.settings.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil).Once()
	deps.state.EXPECT().DeleteByResource(mock.Anything, "prompt", testPromptID).Return(true, nil).Once()
	deps.audit.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()

	require.NoError(t, clear(t, clearer, models.FreshnessReasonAccessed))
	require.True(t, models.DefaultFreshnessReversibilityEnabled,
		"this test only means anything while the default is ON")
}

// Something else cleared the row between the read and the delete — a
// concurrent access, or an evaluation run. That clear wrote its own audit
// entry, so writing a second would invent a transition that never happened.
func TestClearIfStale_LostRaceWritesNoAudit(t *testing.T) {
	clearer, deps := newClearer(t)

	deps.expectStale()
	deps.expectAnyMediumRule()
	deps.expectReversibility(true)
	deps.state.EXPECT().DeleteByResource(mock.Anything, "prompt", testPromptID).Return(false, nil).Once()

	require.NoError(t, clear(t, clearer, models.FreshnessReasonAccessed))
}

// The stored row is the authority on tenancy: a caller passing a team that does
// not own the resource must not be able to clear it by guessing an id.
func TestClearIfStale_RefusesAnotherTeamsResource(t *testing.T) {
	clearer, deps := newClearer(t)

	deps.state.EXPECT().GetByResource(mock.Anything, "prompt", testPromptID).
		Return(&models.ResourceFreshness{TeamID: "someone-else", ResourceID: testPromptID}, nil).Once()

	require.NoError(t, clear(t, clearer, models.FreshnessReasonAccessed))
	// No settings read, no delete, no audit — the t-bound mocks enforce it.
}

// `rule_run` belongs to the evaluator, which decides staleness from the rules.
// Accepting it here would let an access masquerade as a rule decision in the
// audit log, and the audit tab is how a team answers "why did this change".
func TestClearIfStale_RejectsANonReversalReason(t *testing.T) {
	for _, reason := range []string{models.FreshnessReasonRuleRun, "", "deleted"} {
		t.Run("reason "+reason, func(t *testing.T) {
			clearer, _ := newClearer(t)

			err := clear(t, clearer, reason)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "is not a reversal reason")
			// Rejected before any repository call.
		})
	}
}

func TestClearIfStale_PropagatesRepositoryErrors(t *testing.T) {
	failure := errors.New("boom")

	tests := []struct {
		name  string
		setup func(deps clearerDeps)
	}{
		{
			name: "freshness lookup",
			setup: func(deps clearerDeps) {
				deps.state.EXPECT().GetByResource(mock.Anything, "prompt", testPromptID).
					Return(nil, failure).Once()
			},
		},
		{
			name: "rule lookup",
			setup: func(deps clearerDeps) {
				deps.expectStale()
				deps.rules.EXPECT().ListByTeam(mock.Anything, testTeamID, true).Return(nil, failure).Once()
			},
		},
		{
			name: "settings lookup",
			setup: func(deps clearerDeps) {
				deps.expectStale()
				deps.expectAnyMediumRule()
				deps.settings.EXPECT().Get(mock.Anything, testTeamID).Return(nil, failure).Once()
			},
		},
		{
			name: "delete",
			setup: func(deps clearerDeps) {
				deps.expectStale()
				deps.expectAnyMediumRule()
				deps.expectReversibility(true)
				deps.state.EXPECT().DeleteByResource(mock.Anything, "prompt", testPromptID).
					Return(false, failure).Once()
			},
		},
		{
			name: "audit",
			setup: func(deps clearerDeps) {
				deps.expectStale()
				deps.expectAnyMediumRule()
				deps.expectReversibility(true)
				deps.state.EXPECT().DeleteByResource(mock.Anything, "prompt", testPromptID).
					Return(true, nil).Once()
				deps.audit.EXPECT().Create(mock.Anything, mock.Anything).Return(failure).Once()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearer, deps := newClearer(t)
			tt.setup(deps)

			err := clear(t, clearer, models.FreshnessReasonAccessed)

			require.Error(t, err)
			assert.ErrorIs(t, err, failure)
		})
	}
}

// Projects and agents are recorded as accesses but can never be stale — no
// rule can name them and they carry no last-accessed columns. Asking the
// database about them would be a guaranteed miss on every such read.
func TestClearIfStale_SkipsTypesFreshnessCannotEvaluate(t *testing.T) {
	for _, resourceType := range []string{"project", "agent", "", "team"} {
		t.Run("type "+resourceType, func(t *testing.T) {
			clearer, _ := newClearer(t)

			err := clearer.ClearIfStale(
				context.Background(), testTeamID, resourceType, testPromptID,
				models.FreshnessReasonAccessed, testAccessMedium)

			require.NoError(t, err, "an unevaluable type is a no-op, not a failure")
			// No repository call at all — the t-bound mocks enforce it.
		})
	}
}
