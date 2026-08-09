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

type clearerDeps struct {
	settings *repomocks.MockTeamFreshnessSettingsRepository
	state    *repomocks.MockResourceFreshnessRepository
	audit    *repomocks.MockFreshnessAuditRepository
}

func newClearer(t *testing.T) (*freshness.Clearer, clearerDeps) {
	t.Helper()

	deps := clearerDeps{
		settings: repomocks.NewMockTeamFreshnessSettingsRepository(t),
		state:    repomocks.NewMockResourceFreshnessRepository(t),
		audit:    repomocks.NewMockFreshnessAuditRepository(t),
	}
	clearer := freshness.NewClearer(deps.settings, deps.state, deps.audit, slog.New(slog.DiscardHandler))
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
	return c.ClearIfStale(context.Background(), testTeamID, "prompt", testPromptID, reason)
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
				// miss and must not read settings at all. The t-bound settings
				// mock has no expectation, so a read fails the test.
				deps.state.EXPECT().GetByResource(mock.Anything, "prompt", testPromptID).
					Return(nil, nil).Once()
			} else {
				deps.expectStale()
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

// A team with no stored settings row inherits the defaults, and the default is
// reversibility ON — so a team that never opened the settings card still gets
// the behaviour the epic describes.
func TestClearIfStale_AbsentSettingsInheritTheDefault(t *testing.T) {
	clearer, deps := newClearer(t)

	deps.expectStale()
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
			name: "settings lookup",
			setup: func(deps clearerDeps) {
				deps.expectStale()
				deps.settings.EXPECT().Get(mock.Anything, testTeamID).Return(nil, failure).Once()
			},
		},
		{
			name: "delete",
			setup: func(deps clearerDeps) {
				deps.expectStale()
				deps.expectReversibility(true)
				deps.state.EXPECT().DeleteByResource(mock.Anything, "prompt", testPromptID).
					Return(false, failure).Once()
			},
		},
		{
			name: "audit",
			setup: func(deps clearerDeps) {
				deps.expectStale()
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
				context.Background(), testTeamID, resourceType, testPromptID, models.FreshnessReasonAccessed)

			require.NoError(t, err, "an unevaluable type is a no-op, not a failure")
			// No repository call at all — the t-bound mocks enforce it.
		})
	}
}
