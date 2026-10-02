//go:build integration

package postgres

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Up/down round-trip for migration 024_instance_auth_setup (#1236), on its OWN
// scratch database: this test migrates DOWN, which would corrupt the shared
// integrationDB.
//
// Beyond "it applies", it pins the singleton, the token-hash shape and
// token-needs-expiry CHECKs, the consumer FK behaviour, and that the down
// migration drops a POPULATED table.
func TestMigration024_InstanceAuthSetup_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)

	require.NoError(t, m.Migrate(23), "migrate to 023")
	require.False(t, tableExists(t, db, "instance_auth_setup"), "fixture: the table must not exist before 024")

	require.NoError(t, m.Migrate(24), "migrate to 024")
	assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM instance_auth_setup"),
		"the table starts empty: no token was ever issued")

	hash := make([]byte, 32)

	t.Run("CHECKs", func(t *testing.T) {
		_, err := db.Exec("INSERT INTO instance_auth_setup (token_hash, expires_at) VALUES ($1, now())", hash[:16])
		requirePQCode(t, err, pqCheckViolation)
		_, err = db.Exec("INSERT INTO instance_auth_setup (token_hash) VALUES ($1)", hash)
		requirePQCode(t, err, pqCheckViolation)
		_, err = db.Exec("INSERT INTO instance_auth_setup (id) VALUES (false)")
		requirePQCode(t, err, pqCheckViolation)
		assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM instance_auth_setup"))
	})

	t.Run("a singleton with generation 1 and not re-armed by default", func(t *testing.T) {
		_, err := db.Exec("INSERT INTO instance_auth_setup (token_hash, expires_at) VALUES ($1, now())", hash)
		require.NoError(t, err)
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM instance_auth_setup WHERE generation = 1 AND NOT rearmed AND consumed_at IS NULL"))
		_, err = db.Exec("INSERT INTO instance_auth_setup (token_hash, expires_at) VALUES ($1, now())", hash)
		requirePQCode(t, err, "23505")
	})

	t.Run("the consumer must be a user, and deleting them blanks it", func(t *testing.T) {
		_, err := db.Exec("UPDATE instance_auth_setup SET consumed_by = $1", uuid.New().String())
		requirePQCode(t, err, "23503")

		userID := uuid.New().String()
		_, err = db.Exec("INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
			userID, "setup-"+userID[:8]+"@example.com", "Setup Fixture")
		require.NoError(t, err)
		_, err = db.Exec("UPDATE instance_auth_setup SET token_hash = NULL, consumed_at = now(), consumed_by = $1", userID)
		require.NoError(t, err, "a consumed row holds no token hash")

		_, err = db.Exec("DELETE FROM users WHERE id = $1", userID)
		require.NoError(t, err)
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM instance_auth_setup WHERE consumed_by IS NULL AND consumed_at IS NOT NULL"))
	})

	require.NoError(t, m.Migrate(23), "migrate back down to 023")
	assert.False(t, tableExists(t, db, "instance_auth_setup"), "the down migration drops the populated table")
	assert.True(t, tableExists(t, db, "instance_auth_allowlist"), "023's tables are untouched")

	require.NoError(t, m.Migrate(24), "re-apply 024")
	assert.True(t, tableExists(t, db, "instance_auth_setup"))
}
