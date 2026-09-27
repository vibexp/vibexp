package services_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
)

const testInstanceAdminID = "99999999-8888-7777-6666-555555555555"

func newInstanceSearchService(t *testing.T, logs *bytes.Buffer) (
	*services.InstanceSearchSettingsService, *repomocks.MockInstanceSearchSettingsRepository,
) {
	t.Helper()
	repo := repomocks.NewMockInstanceSearchSettingsRepository(t)
	return services.NewInstanceSearchSettingsService(repo, testLogger(logs)), repo
}

func storedInstanceRow() *models.InstanceSearchSettings {
	updatedBy := testInstanceAdminID
	return &models.InstanceSearchSettings{
		RecencyRankingEnabled: true,
		RankWeightRelevance:   0.6,
		RankWeightCreated:     0.25,
		RankWeightUpdated:     0.15,
		RankHalfLifeDays:      30,
		RankCandidateCap:      400,
		UpdatedAt:             time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
		UpdatedBy:             &updatedBy,
		Version:               3,
	}
}

func validInstanceValues() models.InstanceSearchSettingsValues {
	return models.InstanceSearchSettingsValues{
		RecencyRankingEnabled: true,
		RankWeightRelevance:   0.4,
		RankWeightCreated:     0.4,
		RankWeightUpdated:     0.2,
		RankHalfLifeDays:      60,
		RankCandidateCap:      500,
	}
}

// With no instance row, the built-in defaults are exactly today's values.
func TestBuiltInSearchDefaults_AreTodaysValues(t *testing.T) {
	assert.Equal(t, models.InstanceSearchSettingsValues{
		RecencyRankingEnabled: false,
		RankWeightRelevance:   0.5,
		RankWeightCreated:     0.3,
		RankWeightUpdated:     0.2,
		RankHalfLifeDays:      90,
		RankCandidateCap:      200,
	}, services.BuiltInSearchDefaults())
}

// The built-in defaults and config.yaml's `search:` defaults must not drift
// while both exist (until #1203 removes the config block): the boot-time
// import (#1201) compares against the config ones.
func TestBuiltInSearchDefaults_MatchConfigDefaults(t *testing.T) {
	// Only the one required field; everything else inherits config defaults().
	body := "security:\n  encryption_key: " + strings.Repeat("k", 32) + "\n"
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	cfg, err := config.Load(path)
	require.NoError(t, err)

	assert.Equal(t, models.InstanceSearchSettingsValues{
		RecencyRankingEnabled: cfg.Search.RecencyRankingEnabled,
		RankWeightRelevance:   cfg.Search.RankWeightRelevance,
		RankWeightCreated:     cfg.Search.RankWeightCreated,
		RankWeightUpdated:     cfg.Search.RankWeightUpdated,
		RankHalfLifeDays:      cfg.Search.RankHalfLifeDays,
		RankCandidateCap:      cfg.Search.RankCandidateCap,
	}, services.BuiltInSearchDefaults())
}

func TestInstanceSearchSettingsService_Resolve_NoRowReturnsBuiltInDefaults(t *testing.T) {
	svc, repo := newInstanceSearchService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceSearchSettingsNotFound)

	assert.Equal(t, services.BuiltInSearchDefaults(), svc.Resolve(context.Background()))
}

func TestInstanceSearchSettingsService_Resolve_StoredRowIsUsed(t *testing.T) {
	svc, repo := newInstanceSearchService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(storedInstanceRow(), nil)

	assert.Equal(t, models.InstanceSearchSettingsValues{
		RecencyRankingEnabled: true,
		RankWeightRelevance:   0.6,
		RankWeightCreated:     0.25,
		RankWeightUpdated:     0.15,
		RankHalfLifeDays:      30,
		RankCandidateCap:      400,
	}, svc.Resolve(context.Background()))
}

// No cache: every Resolve reads the row, so a change applies to the next call.
func TestInstanceSearchSettingsService_Resolve_SeesAChangeOnTheNextCall(t *testing.T) {
	svc, repo := newInstanceSearchService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceSearchSettingsNotFound).Once()
	repo.EXPECT().Get(mock.Anything).Return(storedInstanceRow(), nil).Once()

	first := svc.Resolve(context.Background())
	second := svc.Resolve(context.Background())

	assert.Equal(t, services.BuiltInSearchDefaults(), first)
	assert.Equal(t, 400, second.RankCandidateCap)
	assert.NotEqual(t, first, second)
}

func TestInstanceSearchSettingsService_Resolve_RepositoryErrorFailsOpen(t *testing.T) {
	var logs bytes.Buffer
	svc, repo := newInstanceSearchService(t, &logs)
	repo.EXPECT().Get(mock.Anything).Return(nil, errors.New("connection refused"))

	assert.Equal(t, services.BuiltInSearchDefaults(), svc.Resolve(context.Background()),
		"a settings read failure must fall back to the built-in defaults, not fail the search")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
	assert.Equal(t, "WARN", entry["level"])
	assert.Contains(t, entry["error"], "connection refused")
}

func TestInstanceSearchSettingsService_Get_NoRowReportsDefaultSource(t *testing.T) {
	svc, repo := newInstanceSearchService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(nil, repositories.ErrInstanceSearchSettingsNotFound)

	view, err := svc.Get(context.Background())

	require.NoError(t, err)
	assert.Equal(t, &models.InstanceSearchSettingsView{
		Source: models.InstanceSearchSettingsSourceDefault,
		Values: services.BuiltInSearchDefaults(),
	}, view)
}

func TestInstanceSearchSettingsService_Get_StoredRowReportsInstanceSource(t *testing.T) {
	svc, repo := newInstanceSearchService(t, nil)
	row := storedInstanceRow()
	repo.EXPECT().Get(mock.Anything).Return(row, nil)

	view, err := svc.Get(context.Background())

	require.NoError(t, err)
	assert.Equal(t, models.InstanceSearchSettingsSourceInstance, view.Source)
	assert.Equal(t, 400, view.Values.RankCandidateCap)
	require.NotNil(t, view.UpdatedAt)
	assert.Equal(t, row.UpdatedAt, *view.UpdatedAt)
	assert.Equal(t, row.UpdatedBy, view.UpdatedBy)
}

func TestInstanceSearchSettingsService_Get_RepositoryErrorPropagates(t *testing.T) {
	svc, repo := newInstanceSearchService(t, nil)
	repo.EXPECT().Get(mock.Anything).Return(nil, errors.New("boom"))

	_, err := svc.Get(context.Background())

	assert.Error(t, err, "unlike Resolve, Get must not fail open: an admin must see real state")
}

func TestInstanceSearchSettingsService_Update_StoresAndAudits(t *testing.T) {
	svc, repo := newInstanceSearchService(t, nil)
	before := storedInstanceRow()
	var entry *models.InstanceSettingsAuditEntry
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(
			_ context.Context, s *models.InstanceSearchSettings, audit repositories.InstanceSearchSettingsAuditFunc,
		) error {
			assert.Equal(t, 500, s.RankCandidateCap)
			require.NotNil(t, s.UpdatedBy)
			assert.Equal(t, testInstanceAdminID, *s.UpdatedBy)
			s.UpdatedAt = time.Date(2026, 9, 27, 13, 0, 0, 0, time.UTC)
			var err error
			entry, err = audit(before, s)
			return err
		})

	view, err := svc.Update(context.Background(), testInstanceAdminID, validInstanceValues())

	require.NoError(t, err)
	assert.Equal(t, models.InstanceSearchSettingsSourceInstance, view.Source)
	assert.Equal(t, validInstanceValues(), view.Values)
	require.NotNil(t, view.UpdatedAt)
	assert.Equal(t, time.Date(2026, 9, 27, 13, 0, 0, 0, time.UTC), *view.UpdatedAt)

	require.NotNil(t, entry)
	assert.Equal(t, models.InstanceSettingSearch, entry.Setting)
	assert.Equal(t, models.InstanceSettingsAuditActionUpsert, entry.Action)
	require.NotNil(t, entry.ActorUserID)
	assert.Equal(t, testInstanceAdminID, *entry.ActorUserID)
	assert.JSONEq(t, `{"recency_ranking_enabled":true,"rank_weight_relevance":0.6,"rank_weight_created":0.25,
		"rank_weight_updated":0.15,"rank_half_life_days":30,"rank_candidate_cap":400}`, string(entry.Before))
	assert.JSONEq(t, `{"recency_ranking_enabled":true,"rank_weight_relevance":0.4,"rank_weight_created":0.4,
		"rank_weight_updated":0.2,"rank_half_life_days":60,"rank_candidate_cap":500}`, string(entry.After))
}

// A first save has no previous row: the entry's before is empty.
func TestInstanceSearchSettingsService_Update_FirstSaveHasNoBefore(t *testing.T) {
	svc, repo := newInstanceSearchService(t, nil)
	var entry *models.InstanceSettingsAuditEntry
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(
			_ context.Context, s *models.InstanceSearchSettings, audit repositories.InstanceSearchSettingsAuditFunc,
		) error {
			var err error
			entry, err = audit(nil, s)
			return err
		})

	_, err := svc.Update(context.Background(), testInstanceAdminID, validInstanceValues())

	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Nil(t, entry.Before)
	assert.NotNil(t, entry.After)
}

// A system change (no actor) records no actor rather than an empty or blank
// user id, the same way the instance email provider service does.
func TestInstanceSearchSettingsService_Update_BlankActorIsNil(t *testing.T) {
	for _, actor := range []string{"", "   "} {
		svc, repo := newInstanceSearchService(t, nil)
		var entry *models.InstanceSettingsAuditEntry
		repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(
				_ context.Context, s *models.InstanceSearchSettings, audit repositories.InstanceSearchSettingsAuditFunc,
			) error {
				assert.Nil(t, s.UpdatedBy)
				var err error
				entry, err = audit(nil, s)
				return err
			})

		_, err := svc.Update(context.Background(), actor, validInstanceValues())

		require.NoError(t, err)
		assert.Nil(t, entry.ActorUserID, "actor %q", actor)
	}
}

func TestInstanceSearchSettingsService_Update_RepositoryErrorPropagates(t *testing.T) {
	svc, repo := newInstanceSearchService(t, nil)
	repo.EXPECT().UpsertAudited(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("boom"))

	_, err := svc.Update(context.Background(), testInstanceAdminID, validInstanceValues())

	assert.ErrorContains(t, err, "boom")
}

func TestInstanceSearchSettingsService_Update_RejectsInvalidValuesWithoutWriting(t *testing.T) {
	// No repo expectations: an invalid set must write no row and no audit entry.
	svc, _ := newInstanceSearchService(t, nil)
	invalid := validInstanceValues()
	invalid.RankCandidateCap = 0

	_, err := svc.Update(context.Background(), testInstanceAdminID, invalid)

	assert.ErrorIs(t, err, services.ErrInvalidSearchSettings)
}

func TestInstanceSearchSettingsService_Reset_DeletesAndAudits(t *testing.T) {
	svc, repo := newInstanceSearchService(t, nil)
	var entry *models.InstanceSettingsAuditEntry
	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, audit repositories.InstanceSearchSettingsAuditFunc) (bool, error) {
			var err error
			entry, err = audit(storedInstanceRow(), nil)
			return true, err
		})

	require.NoError(t, svc.Reset(context.Background(), testInstanceAdminID))

	require.NotNil(t, entry)
	assert.Equal(t, models.InstanceSettingSearch, entry.Setting)
	assert.Equal(t, models.InstanceSettingsAuditActionDelete, entry.Action)
	assert.NotNil(t, entry.Before)
	assert.Nil(t, entry.After)
}

// With no stored row the repository deletes nothing and never builds an
// entry; the service treats that as success.
func TestInstanceSearchSettingsService_Reset_NoRowIsANoOp(t *testing.T) {
	svc, repo := newInstanceSearchService(t, nil)
	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).Return(false, nil)

	assert.NoError(t, svc.Reset(context.Background(), testInstanceAdminID))
}

func TestInstanceSearchSettingsService_Reset_RepositoryErrorPropagates(t *testing.T) {
	svc, repo := newInstanceSearchService(t, nil)
	repo.EXPECT().DeleteAudited(mock.Anything, mock.Anything).Return(false, errors.New("boom"))

	assert.ErrorContains(t, svc.Reset(context.Background(), testInstanceAdminID), "boom")
}

// Each bound of the instance validator, on both sides where it has two.
func TestValidateInstanceSearchSettings_Bounds(t *testing.T) {
	with := func(mutate func(*models.InstanceSearchSettingsValues)) models.InstanceSearchSettingsValues {
		v := validInstanceValues()
		mutate(&v)
		return v
	}
	cases := []struct {
		name    string
		values  models.InstanceSearchSettingsValues
		wantErr bool
	}{
		{"valid", validInstanceValues(), false},
		{"negative weight", with(func(v *models.InstanceSearchSettingsValues) { v.RankWeightUpdated = -0.1 }), true},
		{"all weights zero", with(func(v *models.InstanceSearchSettingsValues) {
			v.RankWeightRelevance, v.RankWeightCreated, v.RankWeightUpdated = 0, 0, 0
		}), true},
		{"half-life 0", with(func(v *models.InstanceSearchSettingsValues) { v.RankHalfLifeDays = 0 }), true},
		{"half-life at ceiling 36500", with(func(v *models.InstanceSearchSettingsValues) {
			v.RankHalfLifeDays = models.MaxSearchRankHalfLifeDays
		}), false},
		{"half-life 36500.01", with(func(v *models.InstanceSearchSettingsValues) { v.RankHalfLifeDays = 36500.01 }), true},
		{"cap 0", with(func(v *models.InstanceSearchSettingsValues) { v.RankCandidateCap = 0 }), true},
		{"cap 1", with(func(v *models.InstanceSearchSettingsValues) { v.RankCandidateCap = 1 }), false},
		{"cap 5000", with(func(v *models.InstanceSearchSettingsValues) {
			v.RankCandidateCap = models.MaxSearchRankCandidateCap
		}), false},
		{"cap 5001", with(func(v *models.InstanceSearchSettingsValues) { v.RankCandidateCap = 5001 }), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := services.ValidateInstanceSearchSettings(tc.values)
			if tc.wantErr {
				assert.ErrorIs(t, err, services.ErrInvalidSearchSettings)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// The built-in defaults must themselves pass the validator.
func TestValidateInstanceSearchSettings_AcceptsBuiltInDefaults(t *testing.T) {
	assert.NoError(t, services.ValidateInstanceSearchSettings(services.BuiltInSearchDefaults()))
}
