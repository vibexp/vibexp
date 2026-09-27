//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level suite for InstanceAISummarySettingsRepository against real
// Postgres (#1197). Like the search suite, it runs serially on a table it
// empties before and after each test.

func TestIntegrationInstanceAISummarySettings_Get_NoRow(t *testing.T) {
	resetInstanceSettingsTable(t, "instance_ai_summary_settings")

	got, err := NewInstanceAISummarySettingsRepository(integrationDB).Get(context.Background())

	assert.Nil(t, got)
	assert.ErrorIs(t, err, repositories.ErrInstanceAISummarySettingsNotFound)
}

func TestIntegrationInstanceAISummarySettings_UpsertCreateThenUpdate(t *testing.T) {
	resetInstanceSettingsTable(t, "instance_ai_summary_settings")
	repo := NewInstanceAISummarySettingsRepository(integrationDB)
	ctx := context.Background()

	first := instanceAISummarySettingsFixture()
	require.NoError(t, repo.Upsert(ctx, first))
	assert.Equal(t, int64(1), first.Version)

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.True(t, got.Enabled)
	assert.Equal(t, 5, got.TopN)
	assert.Equal(t, models.AISummaryStyleBalanced, got.Style)
	assert.Equal(t, 1024, got.MaxOutputTokens)
	assert.Equal(t, 4000, got.PerDocumentChars)
	assert.Equal(t, 20000, got.TotalContextChars)
	assert.Equal(t, 45*time.Second, got.RequestTimeout, "the timeout round-trips through milliseconds")
	assert.Nil(t, got.UpdatedBy)

	editor := insertTestUser(t)
	second := &models.InstanceAISummarySettings{
		Enabled:           false,
		TopN:              10,
		Style:             models.AISummaryStyleDetailed,
		MaxOutputTokens:   32768,
		PerDocumentChars:  1000,
		TotalContextChars: 1000,
		RequestTimeout:    1500 * time.Millisecond,
		UpdatedBy:         &editor,
	}
	require.NoError(t, repo.Upsert(ctx, second))
	assert.Equal(t, int64(2), second.Version, "an update bumps the version")
	assert.True(t, second.CreatedAt.Equal(first.CreatedAt), "the update keeps the original created_at")

	got, err = repo.Get(ctx)
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.Equal(t, 10, got.TopN)
	assert.Equal(t, models.AISummaryStyleDetailed, got.Style)
	assert.Equal(t, 32768, got.MaxOutputTokens)
	assert.Equal(t, 1000, got.TotalContextChars)
	assert.Equal(t, 1500*time.Millisecond, got.RequestTimeout)
	require.NotNil(t, got.UpdatedBy)
	assert.Equal(t, editor, *got.UpdatedBy)
	assert.Equal(t, int64(2), got.Version)
	assert.True(t, got.CreatedAt.Equal(first.CreatedAt))
	assert.Equal(t, 1, countRows(t, integrationDB.DB, "SELECT count(*) FROM instance_ai_summary_settings"))
}

func TestIntegrationInstanceAISummarySettings_InsertIfAbsent(t *testing.T) {
	resetInstanceSettingsTable(t, "instance_ai_summary_settings")
	repo := NewInstanceAISummarySettingsRepository(integrationDB)
	ctx := context.Background()

	s := instanceAISummarySettingsFixture()
	inserted, err := repo.InsertIfAbsent(ctx, s)
	require.NoError(t, err)
	assert.True(t, inserted, "an empty table takes the row")
	assert.Equal(t, int64(1), s.Version)

	other := instanceAISummarySettingsFixture()
	other.TopN = 1
	other.Style = models.AISummaryStyleConcise
	inserted, err = repo.InsertIfAbsent(ctx, other)
	require.NoError(t, err)
	assert.False(t, inserted, "an existing row is not replaced")

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, got.TopN, "the existing row is left untouched")
	assert.Equal(t, models.AISummaryStyleBalanced, got.Style)
	assert.Equal(t, int64(1), got.Version)
}

func TestIntegrationInstanceAISummarySettings_Delete(t *testing.T) {
	resetInstanceSettingsTable(t, "instance_ai_summary_settings")
	repo := NewInstanceAISummarySettingsRepository(integrationDB)
	ctx := context.Background()

	require.NoError(t, repo.Delete(ctx), "deleting nothing is not an error")

	require.NoError(t, repo.Upsert(ctx, instanceAISummarySettingsFixture()))
	require.NoError(t, repo.Delete(ctx))

	_, err := repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceAISummarySettingsNotFound, "the row is gone")
}

// The search and AI summary sections share #1187's audit log; the setting
// column is not DB-enumerated, so the new constants need no migration.
func TestIntegrationInstanceSettingsAudit_SearchAndAISummarySettingsRoundTrip(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	ctx := context.Background()
	repo := newInstanceSettingsAuditRepo()

	for _, setting := range []string{models.InstanceSettingSearch, models.InstanceSettingAISummary} {
		entry := &models.InstanceSettingsAuditEntry{
			Setting: setting,
			Action:  models.InstanceSettingsAuditActionImport,
			After:   json.RawMessage(`{"enabled":true}`),
		}
		require.NoError(t, repo.Append(ctx, entry), setting)

		entries, next, err := repo.List(ctx, setting, 10, nil)
		require.NoError(t, err)
		assert.Nil(t, next)
		require.Len(t, entries, 1, "each setting lists only its own entries")
		assert.Equal(t, entry.ID, entries[0].ID)
		assert.Equal(t, setting, entries[0].Setting)
		assert.Nil(t, entries[0].ActorUserID)
		assert.JSONEq(t, `{"enabled":true}`, string(entries[0].After))
	}
}

func aiSummaryAuditEntry(action string) repositories.InstanceAISummarySettingsAuditFunc {
	return func(before, after *models.InstanceAISummarySettings) (*models.InstanceSettingsAuditEntry, error) {
		entry := &models.InstanceSettingsAuditEntry{Setting: models.InstanceSettingAISummary, Action: action}
		if before != nil {
			entry.Before = []byte(fmt.Sprintf(`{"top_n":%d}`, before.TopN))
		}
		if after != nil {
			entry.After = []byte(fmt.Sprintf(`{"top_n":%d}`, after.TopN))
		}
		return entry, nil
	}
}

// Audited writes (#1199): one entry per write, before read inside the
// transaction, none for a no-op reset.
func TestIntegrationInstanceAISummarySettings_UpsertAuditedAndDeleteAudited(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, "instance_ai_summary_settings")
	repo := NewInstanceAISummarySettingsRepository(integrationDB)
	ctx := context.Background()

	first := instanceAISummarySettingsFixture()
	require.NoError(t, repo.UpsertAudited(ctx, first, nil, aiSummaryAuditEntry(models.InstanceSettingsAuditActionUpsert)))
	second := instanceAISummarySettingsFixture()
	second.TopN = 9
	require.NoError(t, repo.UpsertAudited(ctx, second, nil, aiSummaryAuditEntry(models.InstanceSettingsAuditActionUpsert)))
	assert.Equal(t, int64(2), second.Version)

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, 9, got.TopN)
	assert.Equal(t, 45*time.Second, got.RequestTimeout)

	deleted, err := repo.DeleteAudited(ctx, aiSummaryAuditEntry(models.InstanceSettingsAuditActionDelete))
	require.NoError(t, err)
	assert.True(t, deleted)
	deleted, err = repo.DeleteAudited(ctx, aiSummaryAuditEntry(models.InstanceSettingsAuditActionDelete))
	require.NoError(t, err)
	assert.False(t, deleted, "a reset with no row deletes nothing")

	entries, _, err := NewInstanceSettingsAuditRepository(integrationDB).
		List(ctx, models.InstanceSettingAISummary, 10, nil)
	require.NoError(t, err)
	require.Len(t, entries, 3, "one entry per write, none for the no-op reset")
	assert.Equal(t, models.InstanceSettingsAuditActionDelete, entries[0].Action)
	assert.JSONEq(t, `{"top_n":9}`, string(entries[0].Before))
	assert.Nil(t, entries[0].After)
	assert.JSONEq(t, `{"top_n":5}`, string(entries[1].Before), "before is read inside the transaction")
	assert.Nil(t, entries[2].Before, "the first save had no previous row")
}

// A failed audit insert rolls the settings write back.
func TestIntegrationInstanceAISummarySettings_UpsertAudited_AuditFailureRollsBack(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, "instance_ai_summary_settings")
	repo := NewInstanceAISummarySettingsRepository(integrationDB)
	ctx := context.Background()

	// An unknown action violates the audit table's CHECK after the upsert ran.
	err := repo.UpsertAudited(ctx, instanceAISummarySettingsFixture(), nil, aiSummaryAuditEntry("not-an-action"))
	require.Error(t, err)

	_, err = repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceAISummarySettingsNotFound, "the upsert must have rolled back")
}
