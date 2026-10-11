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

// Up/down round-trip for the memories-preserve-updated-at block of migration
// 019_consolidated (#1265; originally migration 026), on its own scratch
// database because it migrates down.
//
// It pins the one thing that block adds -- an update made while the transaction-local
// setting is on leaves memories.updated_at alone -- and that every update
// outside such a transaction is stamped exactly as before.

const memoriesPreserveUpdatedAtSQL = `SELECT set_config('vibexp.preserve_updated_at', 'on', true)`

// memoriesUpdateStatus changes one memory's status, with or without the
// setting, inside a transaction of its own, and returns the row's updated_at
// afterwards.
func memoriesUpdateStatus(t *testing.T, db *sql.DB, id, status string, preserve bool) time.Time {
	t.Helper()
	tx, err := db.Begin()
	require.NoError(t, err)
	if preserve {
		_, err = tx.Exec(memoriesPreserveUpdatedAtSQL)
		require.NoError(t, err)
	}
	_, err = tx.Exec("UPDATE memories SET status = $1 WHERE id = $2", status, id)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var updatedAt time.Time
	require.NoError(t, db.QueryRow("SELECT updated_at FROM memories WHERE id = $1", id).Scan(&updatedAt))
	return updatedAt
}

func TestMigration019_MemoriesPreserveUpdatedAt_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(18), "migrate to 018")

	f := newTeamStatusesFixture(t, db)
	id := f.insert(t, "memories", models.MemoryStatusActive)
	inserted := f.snapshot(t, "memories", id).updatedAt

	t.Run("before 019 the setting does nothing: the update is stamped", func(t *testing.T) {
		// A system status: 018 still carries memories_status_check, which the
		// same consolidated step drops.
		got := memoriesUpdateStatus(t, db, id, models.MemoryStatusDraft, true)
		assert.True(t, got.After(inserted), "updated_at must move without 019")
		inserted = got
	})

	require.NoError(t, m.Migrate(19), "migrate to 019")

	t.Run("with the setting on, the update leaves updated_at alone", func(t *testing.T) {
		got := memoriesUpdateStatus(t, db, id, models.MemoryStatusActive, true)
		assert.Equal(t, inserted, got)
		assert.Equal(t, models.MemoryStatusActive, f.snapshot(t, "memories", id).status,
			"the update itself must still apply")
	})

	t.Run("the setting ends with its transaction", func(t *testing.T) {
		got := memoriesUpdateStatus(t, db, id, models.MemoryStatusDraft, false)
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

	require.NoError(t, m.Migrate(18), "migrate down to 018")

	t.Run("down restores the unconditional stamp", func(t *testing.T) {
		got := memoriesUpdateStatus(t, db, id, models.MemoryStatusActive, true)
		assert.True(t, got.After(inserted), "updated_at must move again once 019 is rolled back")
		inserted = got
	})

	t.Run("down keeps the last-accessed exemption", func(t *testing.T) {
		_, err := db.Exec("UPDATE memories SET last_accessed_cli_at = now() WHERE id = $1", id)
		require.NoError(t, err)
		assert.Equal(t, inserted, f.snapshot(t, "memories", id).updatedAt)
	})

	require.NoError(t, m.Migrate(19), "re-apply 019")
	assert.Equal(t, inserted, memoriesUpdateStatus(t, db, id, models.MemoryStatusDraft, true),
		"the re-applied trigger honours the setting again")
}
