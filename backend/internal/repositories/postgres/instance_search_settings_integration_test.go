//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level suite for InstanceSearchSettingsRepository against real
// Postgres (#1197). The table is a singleton that is global to the shared test
// database, so every test starts from and leaves behind an empty table, and none
// runs in parallel.

// resetInstanceSettingsTable empties a singleton settings table now and again
// when the test ends.
func resetInstanceSettingsTable(t *testing.T, table string) {
	t.Helper()
	clear := func() {
		_, err := integrationDB.ExecContext(context.Background(), "DELETE FROM "+table) // #nosec G202 -- test-only, fixed identifier
		require.NoError(t, err)
	}
	clear()
	t.Cleanup(clear)
}

func TestIntegrationInstanceSearchSettings_Get_NoRow(t *testing.T) {
	resetInstanceSettingsTable(t, "instance_search_settings")

	got, err := NewInstanceSearchSettingsRepository(integrationDB).Get(context.Background())

	assert.Nil(t, got)
	assert.ErrorIs(t, err, repositories.ErrInstanceSearchSettingsNotFound)
}

func TestIntegrationInstanceSearchSettings_UpsertCreateThenUpdate(t *testing.T) {
	resetInstanceSettingsTable(t, "instance_search_settings")
	repo := NewInstanceSearchSettingsRepository(integrationDB)
	ctx := context.Background()

	first := instanceSearchSettingsFixture()
	require.NoError(t, repo.Upsert(ctx, first))
	assert.Equal(t, int64(1), first.Version)
	assert.False(t, first.CreatedAt.IsZero(), "created_at must come back on the struct")

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.True(t, got.RecencyRankingEnabled)
	assert.InDelta(t, 0.7, got.RankWeightRelevance, 1e-9)
	assert.InDelta(t, 0.1, got.RankWeightCreated, 1e-9)
	assert.InDelta(t, 0.2, got.RankWeightUpdated, 1e-9)
	assert.InDelta(t, 30.0, got.RankHalfLifeDays, 1e-9)
	assert.Equal(t, 200, got.RankCandidateCap)
	assert.Nil(t, got.UpdatedBy)

	editor := insertTestUser(t)
	second := &models.InstanceSearchSettings{
		RecencyRankingEnabled: false,
		RankWeightRelevance:   1,
		RankHalfLifeDays:      90,
		RankCandidateCap:      5000,
		UpdatedBy:             &editor,
	}
	require.NoError(t, repo.Upsert(ctx, second))
	assert.Equal(t, int64(2), second.Version, "an update bumps the version")
	assert.True(t, second.CreatedAt.Equal(first.CreatedAt), "the update keeps the original created_at")

	got, err = repo.Get(ctx)
	require.NoError(t, err)
	assert.False(t, got.RecencyRankingEnabled)
	assert.InDelta(t, 1.0, got.RankWeightRelevance, 1e-9)
	assert.InDelta(t, 0.0, got.RankWeightCreated, 1e-9)
	assert.InDelta(t, 90.0, got.RankHalfLifeDays, 1e-9)
	assert.Equal(t, 5000, got.RankCandidateCap)
	require.NotNil(t, got.UpdatedBy)
	assert.Equal(t, editor, *got.UpdatedBy)
	assert.Equal(t, int64(2), got.Version)
	assert.True(t, got.CreatedAt.Equal(first.CreatedAt))
	assert.Equal(t, 1, countRows(t, integrationDB.DB, "SELECT count(*) FROM instance_search_settings"))
}

func TestIntegrationInstanceSearchSettings_InsertIfAbsent(t *testing.T) {
	resetInstanceSettingsTable(t, "instance_search_settings")
	repo := NewInstanceSearchSettingsRepository(integrationDB)
	ctx := context.Background()

	s := instanceSearchSettingsFixture()
	inserted, err := repo.InsertIfAbsent(ctx, s)
	require.NoError(t, err)
	assert.True(t, inserted, "an empty table takes the row")
	assert.Equal(t, int64(1), s.Version)

	other := &models.InstanceSearchSettings{RankWeightRelevance: 1, RankHalfLifeDays: 7, RankCandidateCap: 50}
	inserted, err = repo.InsertIfAbsent(ctx, other)
	require.NoError(t, err)
	assert.False(t, inserted, "an existing row is not replaced")

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, 200, got.RankCandidateCap, "the existing row is left untouched")
	assert.InDelta(t, 30.0, got.RankHalfLifeDays, 1e-9)
	assert.Equal(t, int64(1), got.Version)
}

func TestIntegrationInstanceSearchSettings_Delete(t *testing.T) {
	resetInstanceSettingsTable(t, "instance_search_settings")
	repo := NewInstanceSearchSettingsRepository(integrationDB)
	ctx := context.Background()

	require.NoError(t, repo.Delete(ctx), "deleting nothing is not an error")

	require.NoError(t, repo.Upsert(ctx, instanceSearchSettingsFixture()))
	require.NoError(t, repo.Delete(ctx))

	_, err := repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceSearchSettingsNotFound, "the row is gone")
}
