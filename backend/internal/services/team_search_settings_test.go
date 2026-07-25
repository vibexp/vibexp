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
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
)

const testSettingsUserID = "11111111-2222-3333-4444-555555555555"

func settingsInstanceConfig() config.SearchConfig {
	return config.SearchConfig{
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
	return services.NewTeamSearchSettingsService(
		repo, authzSvc, settingsInstanceConfig(), slog.New(slog.DiscardHandler)), repo
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
	assert.Equal(t, 200, view.RankCandidateCap, "the cap still comes from the instance config")
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
	hugeHalfLife.RankHalfLifeDays = config.MaxSearchRankHalfLifeDays + 1

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
	values.RankHalfLifeDays = config.MaxSearchRankHalfLifeDays

	_, err := svc.Update(context.Background(), testSettingsUserID, testTeamID, values)

	assert.NoError(t, err)
}
