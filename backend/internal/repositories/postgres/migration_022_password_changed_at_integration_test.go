//go:build integration

package postgres

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The migration just before this one, and this one.
const (
	migrationPasswordChangedAtBefore = 21
	migrationPasswordChangedAtAfter  = 22
)

// Up/down round trip for the password_changed_at block of migration
// 022_consolidated (#1437; originally migration 030) on its OWN scratch
// database: it migrates down, which would corrupt the shared integrationDB.
//
// user_credentials is created by an earlier block of the same migration, so
// there is no credential to carry across the up: the test pins the column's
// default and NOT NULL on a credential written after it.
func TestMigration022_PasswordChangedAt(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()
	m := newMigrator(t, db)

	require.NoError(t, m.Migrate(migrationPasswordChangedAtBefore), "migrate to the version before")
	require.False(t, tableExists(t, db, "user_credentials"), "fixture: no credentials table yet")

	require.NoError(t, m.Migrate(migrationPasswordChangedAtAfter), "migrate up")

	userID := uuid.New().String()
	_, err := db.Exec("INSERT INTO users (id, email, name) VALUES ($1, 'existing@example.com', 'Existing')", userID)
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO user_credentials (user_id, password_hash) VALUES ($1, 'hash')", userID)
	require.NoError(t, err)

	var changedAt time.Time
	require.NoError(t, db.QueryRow(
		"SELECT password_changed_at FROM user_credentials WHERE user_id = $1", userID).Scan(&changedAt))
	assert.WithinDuration(t, time.Now(), changedAt, time.Minute, "a new password gets the current time")
	_, err = db.Exec("UPDATE user_credentials SET password_changed_at = NULL WHERE user_id = $1", userID)
	requirePQCode(t, err, "23502") // NOT NULL

	require.NoError(t, m.Migrate(migrationPasswordChangedAtBefore), "migrate down over a populated column")
	assert.False(t, tableExists(t, db, "user_credentials"))

	require.NoError(t, m.Migrate(migrationPasswordChangedAtAfter), "and up again")
	assert.True(t, columnExists(t, db, "user_credentials", "password_changed_at"))
}
