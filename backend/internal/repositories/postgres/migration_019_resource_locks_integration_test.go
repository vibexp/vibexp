//go:build integration

package postgres

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Up/down/up round-trip for the resource-locks block of migration
// 019_consolidated (#1286; originally migration 027), on its OWN
// scratch database: this test migrates DOWN, which would corrupt the shared
// integrationDB.
//
// It pins that existing rows come through the up and the down unchanged
// (updated_at AND version: locking is storage, not an edit), that the schema
// itself makes "locked with no locker" impossible, and that deleting the
// locker's account unlocks the resource with no application code involved.

// resourceLockTables maps each lockable table to a status its rows can carry,
// its lock CHECK and its partial index.
var resourceLockTables = map[string]struct{ status, check, index string }{
	"prompts":    {models.PromptStatusPublished, "prompts_lock_state_check", "idx_prompts_locked"},
	"blueprints": {models.StatusActive, "blueprints_lock_state_check", "idx_blueprints_locked"},
	"artifacts":  {models.StatusActive, "artifacts_lock_state_check", "idx_artifacts_locked"},
}

var resourceLockColumnNames = []string{"locked_by", "locked_at", "lock_comment"}

// resourceLockSnapshot is what a lock migration must not disturb on a row.
type resourceLockSnapshot struct {
	updatedAt time.Time
	version   int64
}

func resourceLockSnap(t *testing.T, db *sql.DB, table, id string) resourceLockSnapshot {
	t.Helper()
	var snap resourceLockSnapshot
	require.NoError(t, db.QueryRow(
		"SELECT updated_at, version FROM "+table+" WHERE id = $1", id,
	).Scan(&snap.updatedAt, &snap.version))
	return snap
}

// resourceLockRows is the one seeded row per lockable table, with what each
// looked like before 019 was applied.
type resourceLockRows struct {
	ids    map[string]string
	before map[string]resourceLockSnapshot
}

// seedResourceLockRows inserts one row per lockable table on a pre-019 schema.
func seedResourceLockRows(t *testing.T, db *sql.DB, f teamStatusesFixture) resourceLockRows {
	t.Helper()
	rows := resourceLockRows{
		ids:    make(map[string]string, len(resourceLockTables)),
		before: make(map[string]resourceLockSnapshot, len(resourceLockTables)),
	}
	for table, spec := range resourceLockTables {
		require.False(t, columnExists(t, db, table, "locked_by"), "fixture: %s has no lock columns before 019", table)
		rows.ids[table] = f.insert(t, table, spec.status)
		rows.before[table] = resourceLockSnap(t, db, table, rows.ids[table])
	}
	return rows
}

func assertResourceLockRowsUnchangedAndUnlocked(t *testing.T, db *sql.DB, rows resourceLockRows) {
	t.Helper()
	for table, id := range rows.ids {
		assert.Equal(t, rows.before[table], resourceLockSnap(t, db, table, id), table)
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM "+table+
				" WHERE id = $1 AND locked_by IS NULL AND locked_at IS NULL AND lock_comment IS NULL", id), table)
	}
}

func assertResourceLockSchemaPresent(t *testing.T, db *sql.DB) {
	t.Helper()
	for table, spec := range resourceLockTables {
		for _, column := range resourceLockColumnNames {
			assert.True(t, columnExists(t, db, table, column), "%s.%s", table, column)
		}
		assert.Equal(t, 1, constraintCount(t, db, spec.check), spec.check)
		assert.True(t, indexExists(t, db, spec.index), spec.index)
		assert.Equal(t, 1, countRows(t, db,
			`SELECT count(*) FROM pg_indexes
			  WHERE indexname = $1 AND indexdef LIKE '%(team_id) WHERE (locked_by IS NOT NULL)'`, spec.index),
			"%s must be partial on locked rows", spec.index)
	}
}

func requireResourceLockNeedsTimestampAndComment(t *testing.T, db *sql.DB, rows resourceLockRows, userID string) {
	t.Helper()
	for table, id := range rows.ids {
		_, err := db.Exec("UPDATE "+table+" SET locked_by = $1 WHERE id = $2", userID, id)
		requirePQCode(t, err, pqCheckViolation)
		_, err = db.Exec("UPDATE "+table+" SET locked_by = $1, locked_at = now() WHERE id = $2", userID, id)
		requirePQCode(t, err, pqCheckViolation)
		_, err = db.Exec("UPDATE "+table+" SET locked_by = $1, lock_comment = 'c' WHERE id = $2", userID, id)
		requirePQCode(t, err, pqCheckViolation)
	}
}

func requireResourceLockRejectsUnknownLocker(t *testing.T, db *sql.DB, rows resourceLockRows) {
	t.Helper()
	for table, id := range rows.ids {
		_, err := db.Exec(
			"UPDATE "+table+" SET locked_by = $1, locked_at = now(), lock_comment = 'c' WHERE id = $2",
			uuid.New().String(), id)
		requirePQCode(t, err, "23503")
	}
}

// lockResourceLockRows locks every seeded row as lockerID.
func lockResourceLockRows(t *testing.T, db *sql.DB, rows resourceLockRows, lockerID string) {
	t.Helper()
	for table, id := range rows.ids {
		_, err := db.Exec(
			"UPDATE "+table+" SET locked_by = $1, locked_at = now(), lock_comment = 'freeze' WHERE id = $2",
			lockerID, id)
		require.NoError(t, err, "lock %s", table)
	}
}

func assertResourceLockRowsUnlockedByLockerDeletion(t *testing.T, db *sql.DB, rows resourceLockRows) {
	t.Helper()
	for table, id := range rows.ids {
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM "+table+" WHERE id = $1 AND locked_by IS NULL", id),
			"%s: locked_by must be NULL after the locker is deleted", table)
		// The ghost a reader must ignore: this is why "locked" is defined
		// as locked_by IS NOT NULL and nothing else.
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM "+table+
				" WHERE id = $1 AND locked_at IS NOT NULL AND lock_comment = 'freeze'", id), table)
		assert.Equal(t, rows.before[table], resourceLockSnap(t, db, table, id),
			"%s: the unlock-by-deletion must not look like an edit", table)
	}
}

func assertResourceLockSchemaDropped(t *testing.T, db *sql.DB) {
	t.Helper()
	assert.False(t, tableExists(t, db, "resource_lock_events"))
	for table, spec := range resourceLockTables {
		for _, column := range resourceLockColumnNames {
			assert.False(t, columnExists(t, db, table, column), "%s.%s", table, column)
		}
		assert.Equal(t, 0, constraintCount(t, db, spec.check), spec.check)
		assert.False(t, indexExists(t, db, spec.index), spec.index)
	}
}

func assertResourceLockRowsUnchanged(t *testing.T, db *sql.DB, rows resourceLockRows) {
	t.Helper()
	for table, id := range rows.ids {
		assert.Equal(t, rows.before[table], resourceLockSnap(t, db, table, id), table)
	}
}

func assertResourceLockRowsUnlockedAfterReapply(t *testing.T, db *sql.DB, rows resourceLockRows) {
	t.Helper()
	assert.True(t, tableExists(t, db, "resource_lock_events"))
	for table, id := range rows.ids {
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM "+table+" WHERE id = $1 AND locked_by IS NULL AND locked_at IS NULL", id), table)
		assert.Equal(t, rows.before[table], resourceLockSnap(t, db, table, id), table)
	}
}

func TestMigration019_ResourceLocks_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)

	// 1. The version immediately before this migration, with one row per table.
	require.NoError(t, m.Migrate(18), "migrate to 018")
	require.False(t, tableExists(t, db, "resource_lock_events"), "fixture: no history table before 019")

	// teamStatusesFixture is reused for its per-table INSERTs; with a system
	// status they are valid on 018 as well as 019.
	f := newTeamStatusesFixture(t, db)
	rows := seedResourceLockRows(t, db, f)

	// 2. Apply 019.
	require.NoError(t, m.Migrate(19), "migrate to 019")

	t.Run("up leaves existing rows unchanged and unlocked", func(t *testing.T) {
		assertResourceLockRowsUnchangedAndUnlocked(t, db, rows)
	})

	t.Run("up adds the lock columns, the CHECK and the partial index to all three tables", func(t *testing.T) {
		assertResourceLockSchemaPresent(t, db)
	})

	t.Run("a locker needs a timestamp and a comment", func(t *testing.T) {
		requireResourceLockNeedsTimestampAndComment(t, db, rows, f.userID)
	})

	t.Run("the locker must be a real user", func(t *testing.T) {
		requireResourceLockRejectsUnknownLocker(t, db, rows)
	})

	// 3. What the foreign key exists for: lock every row as a second user, then
	//    delete that account.
	lockerID := refScopeInsertUser(t, db)
	lockResourceLockRows(t, db, rows, lockerID)
	_, err := db.Exec("DELETE FROM users WHERE id = $1", lockerID)
	require.NoError(t, err, "delete the locker")

	t.Run("deleting the locker unlocks the resource and leaves the stale columns behind", func(t *testing.T) {
		assertResourceLockRowsUnlockedByLockerDeletion(t, db, rows)
	})

	t.Run("the history table follows the audit-log foreign keys", func(t *testing.T) {
		require.True(t, tableExists(t, db, "resource_lock_events"))
		assert.True(t, indexExists(t, db, "idx_resource_lock_events_resource"))

		insert := `INSERT INTO resource_lock_events (team_id, resource_type, resource_id, action, actor_user_id, comment)
		           VALUES ($1, 'prompt', $2, 'lock', $3, 'freeze')`

		_, insErr := db.Exec(insert, uuid.New().String(), rows.ids["prompts"], f.userID)
		requirePQCode(t, insErr, "23503")
		_, insErr = db.Exec(insert, f.teamID, rows.ids["prompts"], uuid.New().String())
		requirePQCode(t, insErr, "23503")

		// resource_id is polymorphic and carries no foreign key: an event may
		// name a resource that does not (or no longer does) exist.
		actorID := refScopeInsertUser(t, db)
		_, insErr = db.Exec(insert, f.teamID, uuid.New().String(), actorID)
		require.NoError(t, insErr)

		// The entry outlives its actor ...
		_, delErr := db.Exec("DELETE FROM users WHERE id = $1", actorID)
		require.NoError(t, delErr)
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM resource_lock_events WHERE team_id = $1 AND actor_user_id IS NULL", f.teamID))
	})

	t.Run("the history goes with its team", func(t *testing.T) {
		ownerID := refScopeInsertUser(t, db)
		teamID := refScopeInsertTeam(t, db, ownerID)
		_, insErr := db.Exec(
			`INSERT INTO resource_lock_events (team_id, resource_type, resource_id, action)
			 VALUES ($1, 'artifact', $2, 'unlock')`, teamID, uuid.New().String())
		require.NoError(t, insErr)

		_, delErr := db.Exec("DELETE FROM teams WHERE id = $1", teamID)
		require.NoError(t, delErr)
		assert.Equal(t, 0, countRows(t, db,
			"SELECT count(*) FROM resource_lock_events WHERE team_id = $1", teamID))
	})

	// 4. Roll back a populated schema: one live lock and some history.
	_, err = db.Exec(
		"UPDATE artifacts SET locked_by = $1, locked_at = now(), lock_comment = 'freeze' WHERE id = $2",
		f.userID, rows.ids["artifacts"])
	require.NoError(t, err)
	require.NoError(t, m.Migrate(18), "migrate down to 018")

	t.Run("down drops the history, the columns, the CHECKs and the indexes", func(t *testing.T) {
		assertResourceLockSchemaDropped(t, db)
	})

	t.Run("down leaves the rows themselves unchanged", func(t *testing.T) {
		assertResourceLockRowsUnchanged(t, db, rows)
	})

	// 5. Re-apply: an operator who rolls back a release must be able to roll
	//    forward again.
	require.NoError(t, m.Migrate(19), "re-apply 019")

	t.Run("re-applying after a down starts every row unlocked", func(t *testing.T) {
		assertResourceLockRowsUnlockedAfterReapply(t, db, rows)
	})
}
