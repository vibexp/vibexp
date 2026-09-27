//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level suite for instance_settings_audit (#1187) against real
// Postgres: jsonb and NULL round-tripping, SET NULL on the actor, the setting
// filter, and that walking the keyset cursor visits every entry exactly once --
// including entries that share one created_at. Query text is the sqlmock
// suite's job.

// otherInstanceSetting is a second setting value, so the filter has something
// to exclude. The column accepts any non-empty setting.
const otherInstanceSetting = "other_setting"

func resetInstanceSettingsAuditTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, instance_settings_audit CASCADE")
	require.NoError(t, err)
}

func newInstanceSettingsAuditRepo() repositories.InstanceSettingsAuditRepository {
	return NewInstanceSettingsAuditRepository(integrationDB)
}

// walkInstanceSettingsAudit pages through a setting's whole log and returns
// every entry id in the order it was visited.
func walkInstanceSettingsAudit(
	t *testing.T, repo repositories.InstanceSettingsAuditRepository, setting string, limit int,
) (ids []string, pages int) {
	t.Helper()
	var cursor *models.InstanceSettingsAuditCursor
	for {
		page, next, err := repo.List(context.Background(), setting, limit, cursor)
		require.NoError(t, err)
		pages++
		for _, entry := range page {
			ids = append(ids, entry.ID)
		}
		if next == nil {
			return ids, pages
		}
		require.Less(t, pages, 100, "the walk must terminate")
		cursor = next
	}
}

func TestIntegrationInstanceSettingsAudit_AppendRoundTrip(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	ctx := context.Background()
	repo := newInstanceSettingsAuditRepo()
	actorID := insertTestUser(t)

	entry := &models.InstanceSettingsAuditEntry{
		Setting:     models.InstanceSettingEmailProvider,
		Action:      models.InstanceSettingsAuditActionUpsert,
		ActorUserID: &actorID,
		Before:      json.RawMessage(`{"from_address":"old@example.com","secret":"unchanged"}`),
		After:       json.RawMessage(`{"from_address":"new@example.com","secret":"changed"}`),
	}
	require.NoError(t, repo.Append(ctx, entry))
	assert.NotEmpty(t, entry.ID, "the database assigns the id")
	assert.False(t, entry.CreatedAt.IsZero(), "the database assigns created_at")

	entries, next, err := repo.List(ctx, models.InstanceSettingEmailProvider, 10, nil)
	require.NoError(t, err)
	assert.Nil(t, next)
	require.Len(t, entries, 1)
	got := entries[0]
	assert.Equal(t, entry.ID, got.ID)
	assert.Equal(t, models.InstanceSettingsAuditActionUpsert, got.Action)
	require.NotNil(t, got.ActorUserID)
	assert.Equal(t, actorID, *got.ActorUserID)
	assert.JSONEq(t, `{"from_address":"old@example.com","secret":"unchanged"}`, string(got.Before))
	assert.JSONEq(t, `{"from_address":"new@example.com","secret":"changed"}`, string(got.After))
}

// The boot-time import has no actor and no prior state; a delete has no after.
// Both NULLs must survive the round trip as nil, not as an empty document.
func TestIntegrationInstanceSettingsAudit_NullActorAndSnapshots(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	ctx := context.Background()
	repo := newInstanceSettingsAuditRepo()

	imported := &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingEmailProvider,
		Action:  models.InstanceSettingsAuditActionImport,
		After:   json.RawMessage(`{"provider_type":"smtp"}`),
	}
	require.NoError(t, repo.Append(ctx, imported))
	assert.Nil(t, imported.ActorUserID)
	assert.Nil(t, imported.Before)

	deleted := &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingEmailProvider,
		Action:  models.InstanceSettingsAuditActionDelete,
		Before:  json.RawMessage(`{"provider_type":"smtp"}`),
	}
	require.NoError(t, repo.Append(ctx, deleted))
	assert.Nil(t, deleted.After)

	entries, _, err := repo.List(ctx, models.InstanceSettingEmailProvider, 10, nil)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	for _, entry := range entries {
		switch entry.ID {
		case imported.ID:
			assert.Nil(t, entry.ActorUserID)
			assert.Nil(t, entry.Before)
		case deleted.ID:
			assert.Nil(t, entry.After)
		default:
			t.Fatalf("unexpected entry %s", entry.ID)
		}
	}
}

// Deleting the actor blanks actor_user_id and keeps the entry: deleting an
// account must not erase the record of what that account did.
func TestIntegrationInstanceSettingsAudit_ActorDeleteSetsNull(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	ctx := context.Background()
	repo := newInstanceSettingsAuditRepo()
	actorID := insertTestUser(t)

	require.NoError(t, repo.Append(ctx, &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingEmailProvider, Action: models.InstanceSettingsAuditActionUpsert,
		ActorUserID: &actorID, After: json.RawMessage(`{}`),
	}))

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", actorID)
	require.NoError(t, err)

	entries, _, err := repo.List(ctx, models.InstanceSettingEmailProvider, 10, nil)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Nil(t, entries[0].ActorUserID, "the entry outlives its actor")
}

// The redaction guard holds against the real database too: a rejected entry
// leaves no row behind.
func TestIntegrationInstanceSettingsAudit_UnredactedSnapshotWritesNothing(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	ctx := context.Background()
	repo := newInstanceSettingsAuditRepo()

	doc, err := json.Marshal(map[string]string{"secret_encrypted": instanceSettingsAuditLeakSentinel})
	require.NoError(t, err)

	err = repo.Append(ctx, &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingEmailProvider, Action: models.InstanceSettingsAuditActionUpsert,
		After: doc,
	})
	require.ErrorIs(t, err, repositories.ErrInstanceSettingsAuditUnredacted)
	assert.Equal(t, 0, countRows(t, integrationDB.DB, "SELECT count(*) FROM instance_settings_audit"))
}

// List returns only the requested setting's entries, newest first.
func TestIntegrationInstanceSettingsAudit_ListFiltersBySettingNewestFirst(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	ctx := context.Background()
	repo := newInstanceSettingsAuditRepo()

	var wantNewestFirst []string
	for i := 0; i < 3; i++ {
		entry := &models.InstanceSettingsAuditEntry{
			Setting: models.InstanceSettingEmailProvider, Action: models.InstanceSettingsAuditActionUpsert,
		}
		require.NoError(t, repo.Append(ctx, entry))
		wantNewestFirst = append([]string{entry.ID}, wantNewestFirst...)

		require.NoError(t, repo.Append(ctx, &models.InstanceSettingsAuditEntry{
			Setting: otherInstanceSetting, Action: models.InstanceSettingsAuditActionUpsert,
		}))
	}

	entries, next, err := repo.List(ctx, models.InstanceSettingEmailProvider, 10, nil)
	require.NoError(t, err)
	assert.Nil(t, next)
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		assert.Equal(t, models.InstanceSettingEmailProvider, entry.Setting)
		got = append(got, entry.ID)
	}
	assert.Equal(t, wantNewestFirst, got)

	empty, emptyNext, err := repo.List(ctx, "no_such_setting", 10, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
	assert.Nil(t, emptyNext, "an empty log hands back no cursor")
}

// The keyset walk is the case the (created_at, id) cursor exists for. Seven
// entries are written: four in ONE transaction (so they share a created_at,
// since now() is transaction-start time) and three on their own. Walking two
// per page must take four pages and visit every entry exactly once, in
// created_at DESC, id DESC order -- a cursor on created_at alone would skip or
// repeat the identically-stamped ones.
func TestIntegrationInstanceSettingsAudit_KeysetWalkVisitsEachEntryOnce(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	ctx := context.Background()
	repo := newInstanceSettingsAuditRepo()

	require.NoError(t, repo.Append(ctx, &models.InstanceSettingsAuditEntry{
		Setting: models.InstanceSettingEmailProvider, Action: models.InstanceSettingsAuditActionImport,
	}))

	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	for i := 0; i < 4; i++ {
		_, err = tx.ExecContext(ctx,
			"INSERT INTO instance_settings_audit (setting, action) VALUES ($1, $2)",
			models.InstanceSettingEmailProvider, models.InstanceSettingsAuditActionUpsert)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())

	for i := 0; i < 2; i++ {
		require.NoError(t, repo.Append(ctx, &models.InstanceSettingsAuditEntry{
			Setting: models.InstanceSettingEmailProvider, Action: models.InstanceSettingsAuditActionDelete,
		}))
	}
	require.NoError(t, repo.Append(ctx, &models.InstanceSettingsAuditEntry{
		Setting: otherInstanceSetting, Action: models.InstanceSettingsAuditActionUpsert,
	}))

	require.Equal(t, 4, countRows(t, integrationDB.DB,
		`SELECT count(*) FROM instance_settings_audit
		  WHERE setting = $1 AND action = 'upsert'
		    AND created_at = (SELECT created_at FROM instance_settings_audit
		                       WHERE setting = $1 AND action = 'upsert' LIMIT 1)`,
		models.InstanceSettingEmailProvider),
		"fixture: one transaction must stamp its four rows identically")

	rows, err := integrationDB.QueryContext(ctx,
		`SELECT id FROM instance_settings_audit WHERE setting = $1 ORDER BY created_at DESC, id DESC`,
		models.InstanceSettingEmailProvider)
	require.NoError(t, err)
	var want []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		want = append(want, id)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Len(t, want, 7)

	got, pages := walkInstanceSettingsAudit(t, repo, models.InstanceSettingEmailProvider, 2)
	assert.Equal(t, 4, pages, "7 entries at 2 per page is 4 pages, the last one short")
	assert.Equal(t, want, got, "every entry exactly once, newest first, ties broken by id")
}

// An exact multiple of the page size must not hand back a cursor that leads to
// an empty page: the limit+1 probe is what knows the log is exhausted.
func TestIntegrationInstanceSettingsAudit_ExactMultipleEndsWithoutEmptyPage(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	ctx := context.Background()
	repo := newInstanceSettingsAuditRepo()

	for i := 0; i < 4; i++ {
		require.NoError(t, repo.Append(ctx, &models.InstanceSettingsAuditEntry{
			Setting: models.InstanceSettingEmailProvider, Action: models.InstanceSettingsAuditActionUpsert,
		}))
	}

	got, pages := walkInstanceSettingsAudit(t, repo, models.InstanceSettingEmailProvider, 2)
	assert.Len(t, got, 4)
	assert.Equal(t, 2, pages)
}

// The page size is clamped, so a caller cannot ask for an unbounded read.
func TestIntegrationInstanceSettingsAudit_LimitIsClamped(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	ctx := context.Background()
	repo := newInstanceSettingsAuditRepo()

	_, err := integrationDB.ExecContext(ctx,
		`INSERT INTO instance_settings_audit (setting, action)
		 SELECT $1, 'upsert' FROM generate_series(1, $2::int)`,
		models.InstanceSettingEmailProvider, instanceSettingsAuditMaxLimit+5)
	require.NoError(t, err)

	page, next, err := repo.List(ctx, models.InstanceSettingEmailProvider, 10_000, nil)
	require.NoError(t, err)
	assert.Len(t, page, instanceSettingsAuditMaxLimit)
	assert.NotNil(t, next)

	page, _, err = repo.List(ctx, models.InstanceSettingEmailProvider, 0, nil)
	require.NoError(t, err)
	assert.Len(t, page, instanceSettingsAuditDefaultLimit)
}
