//go:build integration

package postgres

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Up/down round trip for migration 028 (#1449) on its OWN scratch database: it
// migrates down, which would corrupt the shared integrationDB.
//
// "Before" is 026 rather than 027: 027 belongs to a sibling change of the same
// epic and nothing in 028 depends on it, so the test holds whether or not 027
// is present.
func TestMigration028_APIAccessRevocation_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()
	m := newMigrator(t, db)
	subjectIndexes := []string{
		"idx_oauth_access_tokens_subject", "idx_oauth_refresh_tokens_subject",
		"idx_oauth_authorization_codes_subject", "idx_oauth_pkce_sessions_subject",
	}

	require.NoError(t, m.Migrate(26), "migrate to 026")
	require.False(t, columnExists(t, db, "users", "api_tokens_valid_after"), "fixture: no column before 028")
	userID := uuid.New().String()
	_, err := db.Exec("INSERT INTO users (id, email, name) VALUES ($1, 'existing@example.com', 'Existing')", userID)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(28), "migrate to 028")

	var cutoff sql.NullTime
	require.NoError(t, db.QueryRow("SELECT api_tokens_valid_after FROM users WHERE id = $1", userID).Scan(&cutoff))
	assert.False(t, cutoff.Valid, "an existing user starts with no cut-off, so the upgrade disconnects nobody")
	for _, index := range subjectIndexes {
		assert.True(t, indexExists(t, db, index), index)
	}
	_, err = db.Exec("UPDATE users SET api_tokens_valid_after = now() WHERE id = $1", userID)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(26), "migrate back to 026 over a populated column")
	assert.False(t, columnExists(t, db, "users", "api_tokens_valid_after"))
	for _, index := range subjectIndexes {
		assert.False(t, indexExists(t, db, index), index)
	}
	assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM users WHERE id = $1", userID), "the user is untouched")

	require.NoError(t, m.Migrate(28), "and up again")
	assert.True(t, columnExists(t, db, "users", "api_tokens_valid_after"))
}
