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

const testSettingsUserID = "11111111-2222-3333-4444-555555555555"

func settingsInstanceValues() models.InstanceSearchSettingsValues {
	return models.InstanceSearchSettingsValues{
		RecencyRankingEnabled: true,
		RankWeightRelevance:   0.5,
		RankWeightCreated:     0.3,
		RankWeightUpdated:     0.2,
		RankHalfLifeDays:      90,
		RankCandidateCap:      200,
	}
}

func teamProfile() models.TeamSearchSettingsValues {
	return models.TeamSearchSettingsValues{
		RecencyRankingEnabled: false,
		RankWeightRelevance:   0.9,
		RankWeightCreated:     0.05,
		RankWeightUpdated:     0.05,
		RankHalfLifeDays:      7,
	}
}

// allowAllAuthz permits every check; permission behaviour has its own tests.
type allowAllAuthz struct {
	services.AuthorizationServiceInterface
}

func (allowAllAuthz) Can(context.Context, string, string, authz.Permission) error { return nil }

// denyAuthz refuses every check, standing in for a member role.
type denyAuthz struct {
	services.AuthorizationServiceInterface
}

func (denyAuthz) Can(context.Context, string, string, authz.Permission) error {
	return services.ErrPermissionDenied
}

func newSettingsService(t *testing.T, authzSvc services.AuthorizationServiceInterface) (
	*services.TeamSearchSettingsService, *repomocks.MockTeamSearchSettingsRepository,
) {
	t.Helper()
	repo := repomocks.NewMockTeamSearchSettingsRepository(t)
	// The service-interface mock satisfies InstanceSearchSettingsReader. Only
	// Get is expected: the settings API must never take the fail-open Resolve.
	instance := servicemocks.NewMockInstanceSearchSettingsServiceInterface(t)
	// Maybe: denied and rejected writes never reach the instance defaults.
	instance.EXPECT().Get(mock.Anything).Return(&models.InstanceSearchSettingsView{
		Source: models.InstanceSearchSettingsSourceInstance,
		Values: settingsInstanceValues(),
	}, nil).Maybe()
	return services.NewTeamSearchSettingsService(
		repo, authzSvc, instance, slog.New(slog.DiscardHandler)), repo
}

// newSettingsServiceWithUnreadableInstance builds the service over a REAL
// InstanceSearchSettingsService whose repository read fails, so the tests
// exercise the same error path production takes.
func newSettingsServiceWithUnreadableInstance(t *testing.T) (
	*services.TeamSearchSettingsService, *repomocks.MockTeamSearchSettingsRepository,
) {
	t.Helper()
	repo := repomocks.NewMockTeamSearchSettingsRepository(t)
	instanceRepo := repomocks.NewMockInstanceSearchSettingsRepository(t)
	instanceRepo.EXPECT().Get(mock.Anything).Return(nil, errors.New("instance row unreadable"))
	return services.NewTeamSearchSettingsService(repo, allowAllAuthz{},
		services.NewInstanceSearchSettingsService(instanceRepo, slog.New(slog.DiscardHandler)),
		slog.New(slog.DiscardHandler)), repo
}

// Team settings reads report instance_defaults and rank_candidate_cap from the
// instance_search_settings row, resolved on each read, not from config.yaml.
func TestTeamSearchSettingsService_Get_InstanceDefaultsComeFromTheInstanceRow(t *testing.T) {
	teamRepo := repomocks.NewMockTeamSearchSettingsRepository(t)
	instanceRepo := repomocks.NewMockInstanceSearchSettingsRepository(t)
	svc := services.NewTeamSearchSettingsService(teamRepo, allowAllAuthz{},
		services.NewInstanceSearchSettingsService(instanceRepo, slog.New(slog.DiscardHandler)),
		slog.New(slog.DiscardHandler))
	teamRepo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil).Times(2)
	instanceRepo.EXPECT().Get(mock.Anything).
		Return(nil, repositories.ErrInstanceSearchSettingsNotFound).Once()
	instanceRepo.EXPECT().Get(mock.Anything).Return(&models.InstanceSearchSettings{
		RecencyRankingEnabled: true,
		RankWeightRelevance:   0.6,
		RankWeightCreated:     0.3,
		RankWeightUpdated:     0.1,
		RankHalfLifeDays:      45,
		RankCandidateCap:      750,
	}, nil).Once()

	before, err := svc.Get(context.Background(), testTeamID)
	require.NoError(t, err)
	after, err := svc.Get(context.Background(), testTeamID)
	require.NoError(t, err)

	assert.Equal(t, services.BuiltInSearchDefaults().TeamValues(), before.InstanceDefaults)
	assert.Equal(t, 200, before.RankCandidateCap)
	want := models.TeamSearchSettingsValues{
		RecencyRankingEnabled: true,
		RankWeightRelevance:   0.6,
		RankWeightCreated:     0.3,
		RankWeightUpdated:     0.1,
		RankHalfLifeDays:      45,
	}
	assert.Equal(t, want, after.InstanceDefaults, "instance_defaults must follow the stored row")
	assert.Equal(t, want, after.Values, "a team with no profile inherits the stored row")
	assert.Equal(t, 750, after.RankCandidateCap)
}

func TestTeamSearchSettingsService_Get_NoRowReportsInstanceSource(t *testing.T) {
	svc, repo := newSettingsService(t, allowAllAuthz{})
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil)

	view, err := svc.Get(context.Background(), testTeamID)

	require.NoError(t, err)
	assert.Equal(t, models.TeamSearchSettingsSourceInstance, view.Source)
	assert.Equal(t, view.InstanceDefaults, view.Values,
		"with no override the effective values ARE the instance defaults")
	assert.InDelta(t, 90.0, view.Values.RankHalfLifeDays, 1e-9)
	assert.Equal(t, 200, view.RankCandidateCap)
}

func TestTeamSearchSettingsService_Get_StoredRowReportsTeamSource(t *testing.T) {
	svc, repo := newSettingsService(t, allowAllAuthz{})
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(&models.TeamSearchSettings{
		TeamID:              testTeamID,
		RankWeightRelevance: 0.9,
		RankWeightCreated:   0.05,
		RankWeightUpdated:   0.05,
		RankHalfLifeDays:    7,
	}, nil)

	view, err := svc.Get(context.Background(), testTeamID)

	require.NoError(t, err)
	assert.Equal(t, models.TeamSearchSettingsSourceTeam, view.Source)
	assert.InDelta(t, 7.0, view.Values.RankHalfLifeDays, 1e-9)
	assert.InDelta(t, 90.0, view.InstanceDefaults.RankHalfLifeDays, 1e-9,
		"instance_defaults must keep reporting the deployment values, not the team's")
	assert.Equal(t, 200, view.RankCandidateCap)
}

func TestTeamSearchSettingsService_Update_StoresAndReportsTeamSource(t *testing.T) {
	svc, repo := newSettingsService(t, allowAllAuthz{})
	repo.EXPECT().Upsert(mock.Anything, mock.MatchedBy(func(s *models.TeamSearchSettings) bool {
		return s.TeamID == testTeamID && s.RankHalfLifeDays == 7
	})).Return(nil)

	view, err := svc.Update(context.Background(), testSettingsUserID, testTeamID, teamProfile())

	require.NoError(t, err)
	assert.Equal(t, models.TeamSearchSettingsSourceTeam, view.Source)
	assert.InDelta(t, 0.9, view.Values.RankWeightRelevance, 1e-9)
	assert.Equal(t, 200, view.RankCandidateCap, "the cap still comes from the instance defaults")
}

func TestTeamSearchSettingsService_Update_DeniedWithoutPermission(t *testing.T) {
	svc, _ := newSettingsService(t, denyAuthz{})

	_, err := svc.Update(context.Background(), testSettingsUserID, testTeamID, teamProfile())

	assert.ErrorIs(t, err, services.ErrPermissionDenied)
}

// Authorization must be checked BEFORE validation, so an unauthorized caller
// cannot use the error body to probe which values the endpoint accepts.
func TestTeamSearchSettingsService_Update_AuthorizesBeforeValidating(t *testing.T) {
	svc, _ := newSettingsService(t, denyAuthz{})
	invalid := teamProfile()
	invalid.RankHalfLifeDays = -1

	_, err := svc.Update(context.Background(), testSettingsUserID, testTeamID, invalid)

	assert.ErrorIs(t, err, services.ErrPermissionDenied)
	assert.NotErrorIs(t, err, services.ErrInvalidSearchSettings)
}

func TestTeamSearchSettingsService_Reset(t *testing.T) {
	svc, repo := newSettingsService(t, allowAllAuthz{})
	repo.EXPECT().Delete(mock.Anything, testTeamID).Return(nil)

	assert.NoError(t, svc.Reset(context.Background(), testSettingsUserID, testTeamID))
}

func TestTeamSearchSettingsService_Reset_DeniedWithoutPermission(t *testing.T) {
	svc, _ := newSettingsService(t, denyAuthz{})

	err := svc.Reset(context.Background(), testSettingsUserID, testTeamID)

	assert.ErrorIs(t, err, services.ErrPermissionDenied)
}

func TestTeamSearchSettingsService_Get_RepositoryErrorPropagates(t *testing.T) {
	svc, repo := newSettingsService(t, allowAllAuthz{})
	repo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, errors.New("boom"))

	_, err := svc.Get(context.Background(), testTeamID)

	assert.Error(t, err, "unlike the search resolver, a read here must NOT fail open — "+
		"the caller is asking what the settings are and deserves the truth")
}

// Get and the Update response are the settings API's reads and fail CLOSED on
// the instance defaults too: reporting the built-in defaults as instance_defaults
// (or as a no-profile team's values) while the instance row is unreadable would
// present a guess as fact.
func TestTeamSearchSettingsService_Get_InstanceReadErrorPropagates(t *testing.T) {
	stored := &models.TeamSearchSettings{TeamID: testTeamID, RankWeightRelevance: 1, RankHalfLifeDays: 7}
	for name, row := range map[string]*models.TeamSearchSettings{"no team profile": nil, "team profile": stored} {
		t.Run(name, func(t *testing.T) {
			svc, repo := newSettingsServiceWithUnreadableInstance(t)
			repo.EXPECT().Get(mock.Anything, testTeamID).Return(row, nil)

			view, err := svc.Get(context.Background(), testTeamID)

			assert.ErrorContains(t, err, "instance row unreadable")
			assert.Nil(t, view)
		})
	}
}

func TestTeamSearchSettingsService_Update_InstanceReadErrorPropagates(t *testing.T) {
	svc, repo := newSettingsServiceWithUnreadableInstance(t)
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)

	view, err := svc.Update(context.Background(), testSettingsUserID, testTeamID, teamProfile())

	assert.ErrorContains(t, err, "instance row unreadable")
	assert.Nil(t, view)
}

// A missing instance row is real state, not a failed read: the built-in
// defaults are what is in effect, so they are reported with no error.
func TestTeamSearchSettingsService_Get_MissingInstanceRowReportsBuiltInDefaults(t *testing.T) {
	teamRepo := repomocks.NewMockTeamSearchSettingsRepository(t)
	instanceRepo := repomocks.NewMockInstanceSearchSettingsRepository(t)
	svc := services.NewTeamSearchSettingsService(teamRepo, allowAllAuthz{},
		services.NewInstanceSearchSettingsService(instanceRepo, slog.New(slog.DiscardHandler)),
		slog.New(slog.DiscardHandler))
	teamRepo.EXPECT().Get(mock.Anything, testTeamID).Return(nil, nil)
	instanceRepo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceSearchSettingsNotFound)

	view, err := svc.Get(context.Background(), testTeamID)

	require.NoError(t, err)
	assert.Equal(t, models.TeamSearchSettingsSourceInstance, view.Source)
	assert.Equal(t, services.BuiltInSearchDefaults().TeamValues(), view.InstanceDefaults)
	assert.Equal(t, services.BuiltInSearchDefaults().TeamValues(), view.Values)
	assert.Equal(t, services.BuiltInSearchDefaults().RankCandidateCap, view.RankCandidateCap)
}

// degenerateValues covers one violation per validation bound; each mirrors a
// CHECK constraint on team_search_settings.
func degenerateValues() map[string]models.TeamSearchSettingsValues {
	negativeWeight := teamProfile()
	negativeWeight.RankWeightCreated = -0.1

	allZero := teamProfile()
	allZero.RankWeightRelevance, allZero.RankWeightCreated, allZero.RankWeightUpdated = 0, 0, 0

	zeroHalfLife := teamProfile()
	zeroHalfLife.RankHalfLifeDays = 0

	negativeHalfLife := teamProfile()
	negativeHalfLife.RankHalfLifeDays = -1

	hugeHalfLife := teamProfile()
	hugeHalfLife.RankHalfLifeDays = models.MaxSearchRankHalfLifeDays + 1

	return map[string]models.TeamSearchSettingsValues{
		"negative weight":    negativeWeight,
		"all weights zero":   allZero,
		"zero half-life":     zeroHalfLife,
		"negative half-life": negativeHalfLife,
		"half-life over cap": hugeHalfLife,
	}
}

func TestTeamSearchSettingsService_Update_RejectsDegenerateProfiles(t *testing.T) {
	for name, values := range degenerateValues() {
		t.Run(name, func(t *testing.T) {
			// No repo expectations: a rejected profile must never reach storage.
			svc, _ := newSettingsService(t, allowAllAuthz{})

			_, err := svc.Update(context.Background(), testSettingsUserID, testTeamID, values)

			assert.ErrorIs(t, err, services.ErrInvalidSearchSettings)
		})
	}
}

// The boundary itself is valid — the bound is inclusive, matching
// validateSearchRankingConfig and the CHECK constraint.
func TestTeamSearchSettingsService_Update_AcceptsHalfLifeAtTheCeiling(t *testing.T) {
	svc, repo := newSettingsService(t, allowAllAuthz{})
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).Return(nil)

	values := teamProfile()
	values.RankHalfLifeDays = models.MaxSearchRankHalfLifeDays

	_, err := svc.Update(context.Background(), testSettingsUserID, testTeamID, values)

	assert.NoError(t, err)
}
