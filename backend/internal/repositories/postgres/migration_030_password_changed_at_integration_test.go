//go:build integration

package postgres

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// "Before" is 028, the migration that creates user_credentials: 029 belongs to
// a sibling change of the same epic and nothing here depends on it, so the test
// holds whether or not 029 is present.
const (
	migrationPasswordChangedAtBefore = 28
	migrationPasswordChangedAtAfter  = 30
)

// Up/down round trip for the password_changed_at migration (#1437) on its OWN
// scratch database: it migrates down, which would corrupt the shared
// integrationDB.
func TestMigration030_PasswordChangedAt(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()
	m := newMigrator(t, db)

	require.NoError(t, m.Migrate(migrationPasswordChangedAtBefore), "migrate to the version before")
	require.False(t, columnExists(t, db, "user_credentials", "password_changed_at"), "fixture: no column yet")
	userID := uuid.New().String()
	_, err := db.Exec("INSERT INTO users (id, email, name) VALUES ($1, 'existing@example.com', 'Existing')", userID)
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO user_credentials (user_id, password_hash) VALUES ($1, 'hash')", userID)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(migrationPasswordChangedAtAfter), "migrate up")

	var changedAt time.Time
	require.NoError(t, db.QueryRow(
		"SELECT password_changed_at FROM user_credentials WHERE user_id = $1", userID).Scan(&changedAt))
	assert.WithinDuration(t, time.Now(), changedAt, time.Minute, "an existing password gets the migration time")
	_, err = db.Exec("UPDATE user_credentials SET password_changed_at = NULL WHERE user_id = $1", userID)
	requirePQCode(t, err, "23502") // NOT NULL

	require.NoError(t, m.Migrate(migrationPasswordChangedAtBefore), "migrate down over a populated column")
	assert.False(t, columnExists(t, db, "user_credentials", "password_changed_at"))
	assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM user_credentials WHERE password_hash = 'hash'"),
		"the password itself is untouched")

	require.NoError(t, m.Migrate(migrationPasswordChangedAtAfter), "and up again")
	assert.True(t, columnExists(t, db, "user_credentials", "password_changed_at"))
}
