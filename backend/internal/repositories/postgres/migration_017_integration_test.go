//go:build integration

package postgres

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigration017_RemoveFirebaseWebPush verifies migration 017 (issue #688)
// against a real Postgres, on its OWN scratch database.
//
// It deliberately does not use the shared `integrationDB`: TestMain migrates that one
// straight to head, whereas this test must observe the pre-017 state, seed rows the
// migration has to drop, and only then apply 017. Migrating the shared database up and
// down mid-run would corrupt every other suite in the binary (and, because that database
// is shared across worktrees, other checkouts too).
//
// The load-bearing property is not "device_tokens is gone" — that is trivially true if
// the seed silently failed. It is that `users`, whose rows the dropped foreign key
// referenced, is still USABLE afterwards.
func TestMigration017_RemoveFirebaseWebPush(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)

	// 1. Bring the schema to the version immediately BEFORE this migration.
	require.NoError(t, m.Migrate(16), "migrate to 016")

	// 2. Seed the pre-migration state.
	userID := seedMigration017Fixtures(t, db)

	// Sanity-check the fixtures really are in the pre-migration shape, so a silently
	// failed seed cannot make the post-migration assertions pass vacuously.
	require.True(t, tableExists(t, db, "device_tokens"), "fixture: device_tokens must exist before migrating")
	require.Equal(t, 2, countRows(t, db, `SELECT count(*) FROM device_tokens`),
		"fixture: seeded device tokens must exist before migrating")
	require.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM users WHERE id = $1`, userID))

	// 3. Apply migration 017.
	require.NoError(t, m.Migrate(17), "migrate to 017")

	t.Run("device_tokens and all its objects are gone", func(t *testing.T) {
		assert.False(t, tableExists(t, db, "device_tokens"))
		// CASCADE carries the indexes with the table; assert explicitly so a future
		// rewrite that drops only the table body still fails here.
		assert.False(t, indexExists(t, db, "device_tokens_token_idx"))
		assert.False(t, indexExists(t, db, "device_tokens_user_id_idx"))
	})

	t.Run("users survives and is still usable", func(t *testing.T) {
		// The dropped FK referenced users(id). Assert the referenced table still
		// selects, updates and accepts inserts — "the table is absent" says nothing
		// about whether the CASCADE took a neighbour with it.
		assert.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM users WHERE id = $1`, userID))

		var email string
		require.NoError(t, db.QueryRow(`SELECT email FROM users WHERE id = $1`, userID).Scan(&email))
		assert.Equal(t, "fixture@migration017.test", email)

		_, err := db.Exec(`UPDATE users SET name = $1 WHERE id = $2`, "Renamed Fixture", userID)
		require.NoError(t, err, "users must still accept writes")

		newID := uuid.New().String()
		_, err = db.Exec(`INSERT INTO users (id, email, name) VALUES ($1, $2, $3)`,
			newID, "post017@migration017.test", "Post Migration User")
		require.NoError(t, err, "users must still accept inserts")

		// Deleting a user used to cascade into device_tokens; with the FK gone it must
		// still succeed rather than error on a dangling constraint.
		_, err = db.Exec(`DELETE FROM users WHERE id = $1`, newID)
		require.NoError(t, err, "users must still accept deletes")
	})

	// 4. Cycle down → up. Down restores structure only, which is what it claims.
	t.Run("down restores the structure and up re-applies cleanly", func(t *testing.T) {
		require.NoError(t, m.Migrate(16), "migrate down to 016")

		assert.True(t, tableExists(t, db, "device_tokens"), "down must recreate the table")
		assert.True(t, indexExists(t, db, "device_tokens_token_idx"))
		assert.True(t, indexExists(t, db, "device_tokens_user_id_idx"))

		// Structure only: the rows are not recoverable, as the file states.
		assert.Equal(t, 0, countRows(t, db, `SELECT count(*) FROM device_tokens`),
			"down restores structure only — the dropped rows must not reappear")

		// The recreated FK must still enforce, i.e. down rebuilt a real constraint
		// rather than a bare table.
		_, err := db.Exec(
			`INSERT INTO device_tokens (user_id, token, platform) VALUES ($1, 'orphan', 'web')`,
			uuid.New().String())
		assert.Error(t, err, "down must restore the users foreign key, not just the table")

		require.NoError(t, m.Migrate(17), "re-apply 017 after rolling back")
		assert.False(t, tableExists(t, db, "device_tokens"))
	})
}

// seedMigration017Fixtures inserts a user with two registered device tokens and
// returns the user id.
func seedMigration017Fixtures(t *testing.T, db *sql.DB) string {
	t.Helper()

	userID := uuid.New().String()
	_, err := db.Exec(
		`INSERT INTO users (id, email, name) VALUES ($1, $2, $3)`,
		userID, "fixture@migration017.test", "Migration Fixture User")
	require.NoError(t, err, "seed user")

	for _, tok := range []string{"fixture-fcm-token-a", "fixture-fcm-token-b"} {
		_, err = db.Exec(
			`INSERT INTO device_tokens (user_id, token, platform, user_agent)
			 VALUES ($1, $2, 'web', 'Mozilla/5.0 (migration 017 fixture)')`,
			userID, tok)
		require.NoError(t, err, "seed device token %s", tok)
	}

	return userID
}
