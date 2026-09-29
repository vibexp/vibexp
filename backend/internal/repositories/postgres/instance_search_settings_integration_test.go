//go:build integration

package postgres

import (
	"context"
	"fmt"
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
	wipe := func() {
		_, err := integrationDB.ExecContext(context.Background(), "DELETE FROM "+table) // #nosec G202 -- test-only, fixed identifier
		require.NoError(t, err)
	}
	wipe()
	t.Cleanup(wipe)
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

// countInstanceSearchAudit counts the search section's audit entries.
func countInstanceSearchAudit(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT count(*) FROM instance_settings_audit WHERE setting = $1", models.InstanceSettingSearch).Scan(&n))
	return n
}

func searchAuditEntry(action string) repositories.InstanceSearchSettingsAuditFunc {
	return func(before, after *models.InstanceSearchSettings) (*models.InstanceSettingsAuditEntry, error) {
		entry := &models.InstanceSettingsAuditEntry{Setting: models.InstanceSettingSearch, Action: action}
		if before != nil {
			entry.Before = []byte(fmt.Sprintf(`{"rank_candidate_cap":%d}`, before.RankCandidateCap))
		}
		if after != nil {
			entry.After = []byte(fmt.Sprintf(`{"rank_candidate_cap":%d}`, after.RankCandidateCap))
		}
		return entry, nil
	}
}

func TestIntegrationInstanceSearchSettings_UpsertAuditedAndDeleteAudited(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, "instance_search_settings")
	repo := NewInstanceSearchSettingsRepository(integrationDB)
	ctx := context.Background()

	first := instanceSearchSettingsFixture()
	require.NoError(t, repo.UpsertAudited(ctx, first, nil, searchAuditEntry(models.InstanceSettingsAuditActionUpsert)))
	second := instanceSearchSettingsFixture()
	second.RankCandidateCap = 900
	require.NoError(t, repo.UpsertAudited(ctx, second, nil, searchAuditEntry(models.InstanceSettingsAuditActionUpsert)))
	assert.Equal(t, int64(2), second.Version)

	deleted, err := repo.DeleteAudited(ctx, searchAuditEntry(models.InstanceSettingsAuditActionDelete))
	require.NoError(t, err)
	assert.True(t, deleted)
	deleted, err = repo.DeleteAudited(ctx, searchAuditEntry(models.InstanceSettingsAuditActionDelete))
	require.NoError(t, err)
	assert.False(t, deleted, "a reset with no row deletes nothing")

	_, err = repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceSearchSettingsNotFound)

	entries, _, err := NewInstanceSettingsAuditRepository(integrationDB).
		List(ctx, models.InstanceSettingSearch, 10, nil)
	require.NoError(t, err)
	require.Len(t, entries, 3, "one entry per write, none for the no-op reset")
	// Newest first: delete(900→nil), upsert(200→900), upsert(nil→200).
	assert.Equal(t, models.InstanceSettingsAuditActionDelete, entries[0].Action)
	assert.JSONEq(t, `{"rank_candidate_cap":900}`, string(entries[0].Before))
	assert.Nil(t, entries[0].After)
	assert.JSONEq(t, `{"rank_candidate_cap":200}`, string(entries[1].Before), "before is read inside the transaction")
	assert.JSONEq(t, `{"rank_candidate_cap":900}`, string(entries[1].After))
	assert.Nil(t, entries[2].Before, "the first save had no previous row")
}

// A failed audit insert rolls the settings write back: the row never changes
// without its audit entry.
func TestIntegrationInstanceSearchSettings_UpsertAudited_AuditFailureRollsBack(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, "instance_search_settings")
	repo := NewInstanceSearchSettingsRepository(integrationDB)
	ctx := context.Background()

	// An unknown action violates the audit table's CHECK, failing the insert
	// after the settings upsert already ran in the same transaction.
	err := repo.UpsertAudited(ctx, instanceSearchSettingsFixture(), nil, searchAuditEntry("not-an-action"))
	require.Error(t, err)

	_, err = repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceSearchSettingsNotFound, "the upsert must have rolled back")
	assert.Zero(t, countInstanceSearchAudit(t))
}

// Concurrent first saves on an empty table are serialized by the table write
// lock: exactly one of them sees no previous row, and the other audits the
// transition from it. A row lock alone would let both read before = nil.
func TestIntegrationInstanceSearchSettings_ConcurrentFirstSavesAreSerialized(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, "instance_search_settings")
	repo := NewInstanceSearchSettingsRepository(integrationDB)
	ctx := context.Background()

	const writers = 4
	start := make(chan struct{})
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		s := instanceSearchSettingsFixture()
		s.RankCandidateCap = 100 + i
		go func() {
			<-start
			errs <- repo.UpsertAudited(ctx, s, nil, searchAuditEntry(models.InstanceSettingsAuditActionUpsert))
		}()
	}
	close(start)
	for i := 0; i < writers; i++ {
		require.NoError(t, <-errs)
	}

	entries, _, err := NewInstanceSettingsAuditRepository(integrationDB).
		List(ctx, models.InstanceSettingSearch, 10, nil)
	require.NoError(t, err)
	require.Len(t, entries, writers)
	var firstSaves int
	for _, e := range entries {
		if e.Before == nil {
			firstSaves++
		}
	}
	assert.Equal(t, 1, firstSaves, "only the first serialized writer may see an empty table")
}
