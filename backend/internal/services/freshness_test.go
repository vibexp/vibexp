package services_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	servicemocks "github.com/vibexp/vibexp/internal/services/mocks"
)

const (
	freshnessTestTeamID    = "team-1"
	freshnessTestUserID    = "user-1"
	freshnessTestRuleID    = "rule-1"
	freshnessTestProjectID = "project-1"
)

type freshnessDeps struct {
	rules     *repomocks.MockFreshnessRuleRepository
	freshness *repomocks.MockResourceFreshnessRepository
	settings  *repomocks.MockTeamFreshnessSettingsRepository
	projects  *repomocks.MockProjectRepository
	authz     *servicemocks.MockAuthorizationServiceInterface
}

func newFreshnessService(t *testing.T) (*services.FreshnessService, freshnessDeps) {
	t.Helper()

	deps := freshnessDeps{
		rules:     repomocks.NewMockFreshnessRuleRepository(t),
		freshness: repomocks.NewMockResourceFreshnessRepository(t),
		settings:  repomocks.NewMockTeamFreshnessSettingsRepository(t),
		projects:  repomocks.NewMockProjectRepository(t),
		authz:     servicemocks.NewMockAuthorizationServiceInterface(t),
	}
	svc := services.NewFreshnessService(
		deps.rules, deps.freshness, deps.settings, deps.projects, deps.authz,
		slog.New(slog.DiscardHandler),
	)
	return svc, deps
}

// allowWrites lets the write permission through.
func (d freshnessDeps) allowWrites() {
	d.authz.EXPECT().
		Can(mock.Anything, freshnessTestUserID, freshnessTestTeamID, authz.TeamSettingsUpdate).
		Return(nil)
}

func validRuleInput() services.FreshnessRuleInput {
	return services.FreshnessRuleInput{
		ResourceTypes: []string{"artifact", "prompt"},
		Mediums:       []string{"web"},
		ThresholdDays: 90,
		Enabled:       true,
	}
}

// Every write must go through team.settings.update, and a denial must surface
// unwrapped so the handler can turn it into a 403.
func TestFreshnessService_WritesRequireTeamSettingsUpdate(t *testing.T) {
	tests := []struct {
		name string
		call func(svc *services.FreshnessService) error
	}{
		{
			name: "create rule",
			call: func(svc *services.FreshnessService) error {
				_, err := svc.CreateRule(context.Background(), freshnessTestUserID, freshnessTestTeamID, validRuleInput())
				return err
			},
		},
		{
			name: "update rule",
			call: func(svc *services.FreshnessService) error {
				_, err := svc.UpdateRule(
					context.Background(), freshnessTestUserID, freshnessTestTeamID, freshnessTestRuleID, validRuleInput())
				return err
			},
		},
		{
			name: "delete rule",
			call: func(svc *services.FreshnessService) error {
				return svc.DeleteRule(context.Background(), freshnessTestUserID, freshnessTestTeamID, freshnessTestRuleID)
			},
		},
		{
			name: "update settings",
			call: func(svc *services.FreshnessService) error {
				_, err := svc.UpdateSettings(
					context.Background(), freshnessTestUserID, freshnessTestTeamID,
					models.FreshnessSettingsValues{IntervalSeconds: 3600})
				return err
			},
		},
		{
			name: "reset settings",
			call: func(svc *services.FreshnessService) error {
				return svc.ResetSettings(context.Background(), freshnessTestUserID, freshnessTestTeamID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, deps := newFreshnessService(t)
			deps.authz.EXPECT().
				Can(mock.Anything, freshnessTestUserID, freshnessTestTeamID, authz.TeamSettingsUpdate).
				Return(services.ErrPermissionDenied).
				Once()

			err := tt.call(svc)

			require.Error(t, err)
			assert.ErrorIs(t, err, services.ErrPermissionDenied,
				"the authz error must reach the handler unwrapped so it maps to 403")
		})
	}
}

// Reads are deliberately NOT permission-checked: the tenancy middleware has
// already established membership and every member may see the team's policy.
func TestFreshnessService_ReadsDoNotCheckPermissions(t *testing.T) {
	t.Run("list rules", func(t *testing.T) {
		svc, deps := newFreshnessService(t)
		deps.rules.EXPECT().ListByTeam(mock.Anything, freshnessTestTeamID, false).
			Return([]*models.FreshnessRule{}, nil).Once()

		rules, err := svc.ListRules(context.Background(), freshnessTestTeamID)

		require.NoError(t, err)
		assert.Empty(t, rules)
	})

	t.Run("get settings", func(t *testing.T) {
		svc, deps := newFreshnessService(t)
		deps.settings.EXPECT().Get(mock.Anything, freshnessTestTeamID).Return(nil, nil).Once()

		view, err := svc.GetSettings(context.Background(), freshnessTestTeamID)

		require.NoError(t, err)
		require.NotNil(t, view)
	})
	// The authz mock has no expectations in either subtest; mockery fails the
	// test if Can were called at all.
}

func TestFreshnessService_CreateRule_Persists(t *testing.T) {
	svc, deps := newFreshnessService(t)
	deps.allowWrites()
	deps.rules.EXPECT().
		Create(mock.Anything, mock.MatchedBy(func(rule *models.FreshnessRule) bool {
			return rule.TeamID == freshnessTestTeamID && rule.ThresholdDays == 90 &&
				rule.ProjectID == nil && rule.Enabled
		})).
		Return(nil).Once()

	rule, err := svc.CreateRule(context.Background(), freshnessTestUserID, freshnessTestTeamID, validRuleInput())

	require.NoError(t, err)
	require.NotNil(t, rule)
	assert.Equal(t, []string{"artifact", "prompt"}, rule.ResourceTypes)
}

// A rule may only scope itself to a project in its own team; the schema's FK
// has no team predicate, so this is the only thing preventing cross-tenant
// scoping.
func TestFreshnessService_CreateRule_RejectsProjectOutsideTeam(t *testing.T) {
	tests := []struct {
		name    string
		project *models.Project
		err     error
	}{
		{name: "project in another team", project: &models.Project{TeamID: "other-team"}},
		{name: "project does not exist", project: nil, err: repositories.ErrProjectNotFoundForRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, deps := newFreshnessService(t)
			deps.allowWrites()
			deps.projects.EXPECT().
				GetByID(mock.Anything, freshnessTestUserID, freshnessTestProjectID).
				Return(tt.project, tt.err).Once()

			input := validRuleInput()
			projectID := freshnessTestProjectID
			input.ProjectID = &projectID

			_, err := svc.CreateRule(context.Background(), freshnessTestUserID, freshnessTestTeamID, input)

			require.Error(t, err)
			assert.ErrorIs(t, err, services.ErrInvalidFreshnessRule)
			assert.Contains(t, err.Error(), "does not belong to this team")
		})
	}
}

// A repository failure is OURS, not the caller's: reporting it as a 400 would
// tell them to fix a project_id that is in fact correct, and hide the outage.
func TestFreshnessService_CreateRule_ProjectLookupFailureIsNotAValidationError(t *testing.T) {
	svc, deps := newFreshnessService(t)
	deps.allowWrites()
	deps.projects.EXPECT().
		GetByID(mock.Anything, freshnessTestUserID, freshnessTestProjectID).
		Return(nil, errors.New("db down")).Once()

	input := validRuleInput()
	projectID := freshnessTestProjectID
	input.ProjectID = &projectID

	_, err := svc.CreateRule(context.Background(), freshnessTestUserID, freshnessTestTeamID, input)

	require.Error(t, err)
	assert.NotErrorIs(t, err, services.ErrInvalidFreshnessRule,
		"a database failure must surface as 500, not as a validation error the caller cannot fix")
	assert.Contains(t, err.Error(), "db down")
}

func TestFreshnessService_CreateRule_AcceptsProjectInTeam(t *testing.T) {
	svc, deps := newFreshnessService(t)
	deps.allowWrites()
	deps.projects.EXPECT().
		GetByID(mock.Anything, freshnessTestUserID, freshnessTestProjectID).
		Return(&models.Project{TeamID: freshnessTestTeamID}, nil).Once()
	deps.rules.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()

	input := validRuleInput()
	projectID := freshnessTestProjectID
	input.ProjectID = &projectID

	rule, err := svc.CreateRule(context.Background(), freshnessTestUserID, freshnessTestTeamID, input)

	require.NoError(t, err)
	require.NotNil(t, rule.ProjectID)
	assert.Equal(t, freshnessTestProjectID, *rule.ProjectID)
}

// Deleting strips the rule from freshness state BEFORE removing the rule, so a
// failure between the two leaves a state the next evaluation run repairs.
func TestFreshnessService_DeleteRule_StripsStateBeforeDeletingRule(t *testing.T) {
	svc, deps := newFreshnessService(t)
	deps.allowWrites()

	var order []string
	deps.rules.EXPECT().GetByID(mock.Anything, freshnessTestTeamID, freshnessTestRuleID).
		Return(&models.FreshnessRule{ID: freshnessTestRuleID}, nil).Once()
	deps.freshness.EXPECT().RemoveRule(mock.Anything, freshnessTestRuleID).
		Run(func(_ context.Context, _ string) { order = append(order, "strip") }).
		Return(int64(2), nil).Once()
	deps.rules.EXPECT().Delete(mock.Anything, freshnessTestTeamID, freshnessTestRuleID).
		Run(func(_ context.Context, _, _ string) { order = append(order, "delete") }).
		Return(true, nil).Once()

	require.NoError(t, svc.DeleteRule(context.Background(), freshnessTestUserID, freshnessTestTeamID, freshnessTestRuleID))
	assert.Equal(t, []string{"strip", "delete"}, order,
		"stripping first is what makes a partial failure self-healing")
}

// Another team's rule id must be indistinguishable from a missing one, and must
// not trigger any state mutation.
func TestFreshnessService_DeleteRule_MissingRuleIsNotFound(t *testing.T) {
	svc, deps := newFreshnessService(t)
	deps.allowWrites()
	deps.rules.EXPECT().GetByID(mock.Anything, freshnessTestTeamID, freshnessTestRuleID).
		Return(nil, nil).Once()

	err := svc.DeleteRule(context.Background(), freshnessTestUserID, freshnessTestTeamID, freshnessTestRuleID)

	assert.ErrorIs(t, err, repositories.ErrFreshnessRuleNotFound)
	deps.freshness.AssertNotCalled(t, "RemoveRule", mock.Anything, mock.Anything)
}

func TestFreshnessService_UpdateRule_NotFoundPassesThroughUnwrapped(t *testing.T) {
	svc, deps := newFreshnessService(t)
	deps.allowWrites()
	deps.rules.EXPECT().Update(mock.Anything, mock.Anything).
		Return(repositories.ErrFreshnessRuleNotFound).Once()

	_, err := svc.UpdateRule(
		context.Background(), freshnessTestUserID, freshnessTestTeamID, freshnessTestRuleID, validRuleInput())

	assert.ErrorIs(t, err, repositories.ErrFreshnessRuleNotFound,
		"the sentinel must stay recognizable so the handler returns 404, not 500")
}

func TestFreshnessService_Settings_SourceAndDefaults(t *testing.T) {
	t.Run("no stored row inherits the defaults", func(t *testing.T) {
		svc, deps := newFreshnessService(t)
		deps.settings.EXPECT().Get(mock.Anything, freshnessTestTeamID).Return(nil, nil).Once()

		view, err := svc.GetSettings(context.Background(), freshnessTestTeamID)

		require.NoError(t, err)
		assert.Equal(t, models.FreshnessSettingsSourceInstance, view.Source)
		assert.Equal(t, models.DefaultFreshnessIntervalSeconds, view.Values.IntervalSeconds)
		assert.Equal(t, models.DefaultFreshnessSettingsValues(), view.Defaults,
			"defaults are echoed on every read so a client can preview a reset")
	})

	t.Run("stored row reports team source", func(t *testing.T) {
		svc, deps := newFreshnessService(t)
		deps.settings.EXPECT().Get(mock.Anything, freshnessTestTeamID).
			Return(&models.TeamFreshnessSettings{IntervalSeconds: 7200, ReversibilityEnabled: false}, nil).Once()

		view, err := svc.GetSettings(context.Background(), freshnessTestTeamID)

		require.NoError(t, err)
		assert.Equal(t, models.FreshnessSettingsSourceTeam, view.Source)
		assert.Equal(t, 7200, view.Values.IntervalSeconds)
		assert.False(t, view.Values.ReversibilityEnabled)
		assert.Equal(t, models.DefaultFreshnessIntervalSeconds, view.Defaults.IntervalSeconds,
			"defaults stay the defaults, not a copy of the stored values")
	})
}

func TestFreshnessService_UpdateSettings_Persists(t *testing.T) {
	svc, deps := newFreshnessService(t)
	deps.allowWrites()
	deps.settings.EXPECT().
		Upsert(mock.Anything, mock.MatchedBy(func(s *models.TeamFreshnessSettings) bool {
			return s.TeamID == freshnessTestTeamID && s.IntervalSeconds == 7200 && s.ReversibilityEnabled
		})).
		Return(nil).Once()

	view, err := svc.UpdateSettings(context.Background(), freshnessTestUserID, freshnessTestTeamID,
		models.FreshnessSettingsValues{IntervalSeconds: 7200, ReversibilityEnabled: true})

	require.NoError(t, err)
	assert.Equal(t, models.FreshnessSettingsSourceTeam, view.Source)
	assert.Equal(t, 7200, view.Values.IntervalSeconds)
}

// Validation must reject before the repository is reached, so an invalid value
// never becomes a database constraint violation.
func TestValidateFreshnessRuleInput(t *testing.T) {
	tests := []struct {
		name   string
		input  services.FreshnessRuleInput
		wantIn string
	}{
		{
			name:   "zero threshold",
			input:  services.FreshnessRuleInput{ResourceTypes: []string{"artifact"}, ThresholdDays: 0},
			wantIn: "threshold_days must be greater than 0",
		},
		{
			name:   "negative threshold",
			input:  services.FreshnessRuleInput{ResourceTypes: []string{"artifact"}, ThresholdDays: -1},
			wantIn: "threshold_days must be greater than 0",
		},
		{
			name: "threshold above the cap",
			input: services.FreshnessRuleInput{
				ResourceTypes: []string{"artifact"}, ThresholdDays: services.MaxFreshnessThresholdDays + 1,
			},
			wantIn: "threshold_days must be at most 36500",
		},
		{
			name:   "no resource types",
			input:  services.FreshnessRuleInput{ResourceTypes: []string{}, ThresholdDays: 30},
			wantIn: "resource_types must not be empty",
		},
		{
			name:   "unsupported resource type",
			input:  services.FreshnessRuleInput{ResourceTypes: []string{"agent"}, ThresholdDays: 30},
			wantIn: `resource_types contains unsupported value "agent"`,
		},
		{
			name: "unsupported medium",
			input: services.FreshnessRuleInput{
				ResourceTypes: []string{"artifact"}, Mediums: []string{"api"}, ThresholdDays: 30,
			},
			// `api` accesses are recorded but deliberately not rule criteria.
			wantIn: `mediums contains unsupported value "api"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := services.ValidateFreshnessRuleInput(tt.input)

			require.Error(t, err)
			assert.ErrorIs(t, err, services.ErrInvalidFreshnessRule)
			assert.Contains(t, err.Error(), tt.wantIn)
		})
	}

	t.Run("empty mediums means any medium and is valid", func(t *testing.T) {
		assert.NoError(t, services.ValidateFreshnessRuleInput(services.FreshnessRuleInput{
			ResourceTypes: []string{"memory"}, Mediums: []string{}, ThresholdDays: 1,
		}))
	})

	t.Run("every documented resource type and medium is accepted", func(t *testing.T) {
		assert.NoError(t, services.ValidateFreshnessRuleInput(services.FreshnessRuleInput{
			ResourceTypes: []string{"artifact", "prompt", "blueprint", "memory"},
			Mediums:       []string{"web", "cli", "mcp"},
			ThresholdDays: 1,
		}))
	})
}

// The floor mirrors the CHECK on team_freshness_settings, so validation and
// storage cannot disagree about what is acceptable.
func TestValidateFreshnessSettings(t *testing.T) {
	err := services.ValidateFreshnessSettings(models.FreshnessSettingsValues{
		IntervalSeconds: models.MinFreshnessIntervalSeconds - 1,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, services.ErrInvalidFreshnessSettings)
	assert.Contains(t, err.Error(), "at least 3600")

	assert.NoError(t, services.ValidateFreshnessSettings(models.FreshnessSettingsValues{
		IntervalSeconds: models.MinFreshnessIntervalSeconds,
	}), "exactly the floor is accepted")
	assert.NoError(t, services.ValidateFreshnessSettings(models.FreshnessSettingsValues{
		IntervalSeconds: models.DefaultFreshnessIntervalSeconds,
	}))

	// The ceiling keeps the value inside the int32 the column and wire format
	// use, so an over-large interval is a 400 rather than a column overflow.
	tooLong := services.ValidateFreshnessSettings(models.FreshnessSettingsValues{
		IntervalSeconds: services.MaxFreshnessIntervalSeconds + 1,
	})
	require.Error(t, tooLong)
	assert.ErrorIs(t, tooLong, services.ErrInvalidFreshnessSettings)
	assert.Contains(t, tooLong.Error(), "at most 31536000")

	assert.NoError(t, services.ValidateFreshnessSettings(models.FreshnessSettingsValues{
		IntervalSeconds: services.MaxFreshnessIntervalSeconds,
	}), "exactly the ceiling is accepted")

	assert.NoError(t, services.ValidateFreshnessRuleInput(services.FreshnessRuleInput{
		ResourceTypes: []string{"artifact"}, ThresholdDays: services.MaxFreshnessThresholdDays,
	}), "exactly the threshold cap is accepted")
}
