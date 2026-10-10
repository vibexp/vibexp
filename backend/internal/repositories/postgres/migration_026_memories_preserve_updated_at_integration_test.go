//go:build integration

package postgres

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Up/down round-trip for migration 026_memories_preserve_updated_at (#1265),
// on its own scratch database because it migrates down.
//
// It pins the one thing 026 adds -- an update made while the transaction-local
// setting is on leaves memories.updated_at alone -- and that every update
// outside such a transaction is stamped exactly as before.

const migration026PreserveSQL = `SELECT set_config('vibexp.preserve_updated_at', 'on', true)`

// migration026UpdateStatus changes one memory's status, with or without the
// setting, inside a transaction of its own, and returns the row's updated_at
// afterwards.
func migration026UpdateStatus(t *testing.T, db *sql.DB, id, status string, preserve bool) time.Time {
	t.Helper()
	tx, err := db.Begin()
	require.NoError(t, err)
	if preserve {
		_, err = tx.Exec(migration026PreserveSQL)
		require.NoError(t, err)
	}
	_, err = tx.Exec("UPDATE memories SET status = $1 WHERE id = $2", status, id)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var updatedAt time.Time
	require.NoError(t, db.QueryRow("SELECT updated_at FROM memories WHERE id = $1", id).Scan(&updatedAt))
	return updatedAt
}

func TestMigration026_MemoriesPreserveUpdatedAt_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(25), "migrate to 025")

	f := newMigration025Fixture(t, db)
	id := f.insert(t, "memories", models.MemoryStatusActive)
	inserted := f.snapshot(t, "memories", id).updatedAt

	t.Run("before 026 the setting does nothing: the update is stamped", func(t *testing.T) {
		got := migration026UpdateStatus(t, db, id, "in-review", true)
		assert.True(t, got.After(inserted), "updated_at must move without 026")
		inserted = got
	})

	require.NoError(t, m.Migrate(26), "migrate to 026")

	t.Run("with the setting on, the update leaves updated_at alone", func(t *testing.T) {
		got := migration026UpdateStatus(t, db, id, models.MemoryStatusActive, true)
		assert.Equal(t, inserted, got)
		assert.Equal(t, models.MemoryStatusActive, f.snapshot(t, "memories", id).status,
			"the update itself must still apply")
	})

	t.Run("the setting ends with its transaction", func(t *testing.T) {
		got := migration026UpdateStatus(t, db, id, models.MemoryStatusDraft, false)
		assert.True(t, got.After(inserted), "an ordinary edit is stamped as before")
		inserted = got
	})

	t.Run("a last-accessed write is still not an edit", func(t *testing.T) {
		_, err := db.Exec("UPDATE memories SET last_accessed_web_at = now() WHERE id = $1", id)
		require.NoError(t, err)
		assert.Equal(t, inserted, f.snapshot(t, "memories", id).updatedAt)
	})

	t.Run("the trigger is enabled", func(t *testing.T) {
		assert.Equal(t, 1, countRows(t, db,
			`SELECT count(*) FROM pg_trigger
			  WHERE tgname = 'update_memories_updated_at' AND tgenabled = 'O'`))
	})

	require.NoError(t, m.Migrate(25), "migrate down to 025")

	t.Run("down restores the unconditional stamp", func(t *testing.T) {
		got := migration026UpdateStatus(t, db, id, models.MemoryStatusActive, true)
		assert.True(t, got.After(inserted), "updated_at must move again once 026 is rolled back")
		inserted = got
	})

	t.Run("down keeps the last-accessed exemption", func(t *testing.T) {
		_, err := db.Exec("UPDATE memories SET last_accessed_cli_at = now() WHERE id = $1", id)
		require.NoError(t, err)
		assert.Equal(t, inserted, f.snapshot(t, "memories", id).updatedAt)
	})

	require.NoError(t, m.Migrate(26), "re-apply 026")
	assert.Equal(t, inserted, migration026UpdateStatus(t, db, id, models.MemoryStatusDraft, true),
		"the re-applied trigger honours the setting again")
}
