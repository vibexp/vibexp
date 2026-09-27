//go:build integration

package postgres

import (
	"errors"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Up/down round-trip for migration 020_instance_email_provider (#1186), on its
// OWN scratch database: this test migrates DOWN, which would corrupt the shared
// integrationDB.
//
// Beyond "it applies", it pins the singleton at the database level (a second
// row and an id of false are both rejected by Postgres itself, whatever code
// path tries them) and that the down migration drops a POPULATED table.

const insertInstanceEmailProviderSQL = `INSERT INTO instance_email_provider
	(provider_type, from_address) VALUES ('smtp', 'noreply@example.com')`

func requirePQCode(t *testing.T, err error, code pq.ErrorCode) {
	t.Helper()
	require.Error(t, err)
	var pqErr *pq.Error
	require.True(t, errors.As(err, &pqErr), "want a *pq.Error, got %T: %v", err, err)
	assert.Equal(t, code, pqErr.Code, "unexpected SQLSTATE: %v", pqErr)
}

func TestMigration020_InstanceEmailProvider_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)

	// 1. The version immediately before this migration: nothing exists yet.
	require.NoError(t, m.Migrate(19), "migrate to 019")
	require.False(t, tableExists(t, db, "instance_email_provider"),
		"fixture: instance_email_provider must not exist before 020")

	// 2. Apply 020.
	require.NoError(t, m.Migrate(20), "migrate to 020")

	t.Run("creates the table, empty", func(t *testing.T) {
		require.True(t, tableExists(t, db, "instance_email_provider"))
		assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM instance_email_provider"))
	})

	t.Run("a first row takes the defaults", func(t *testing.T) {
		_, err := db.Exec(insertInstanceEmailProviderSQL)
		require.NoError(t, err)
		assert.Equal(t, 1, countRows(t, db,
			`SELECT count(*) FROM instance_email_provider
			  WHERE id AND settings = '{}'::jsonb AND secret_encrypted IS NULL AND version = 1`))
	})

	t.Run("a second row is a unique violation", func(t *testing.T) {
		_, err := db.Exec(insertInstanceEmailProviderSQL)
		requirePQCode(t, err, "23505")
		assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM instance_email_provider"))
	})

	t.Run("id false is a check violation", func(t *testing.T) {
		_, err := db.Exec(`INSERT INTO instance_email_provider (id, provider_type, from_address)
			VALUES (false, 'smtp', 'noreply@example.com')`)
		requirePQCode(t, err, "23514")
	})

	// 3. Roll back with the row still in place.
	require.NoError(t, m.Migrate(19), "migrate down to 019")
	assert.False(t, tableExists(t, db, "instance_email_provider"),
		"down must drop the populated table")

	// 4. Re-apply: an operator who rolls back must be able to roll forward.
	require.NoError(t, m.Migrate(20), "re-apply 020")
	require.True(t, tableExists(t, db, "instance_email_provider"))
	assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM instance_email_provider"),
		"the re-applied table starts empty")
	_, err := db.Exec(insertInstanceEmailProviderSQL)
	require.NoError(t, err, "the re-applied table is usable")
}
