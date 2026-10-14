//go:build integration

package postgres

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

const (
	migrationUserIdentitiesBefore = 21
	migrationUserIdentitiesAfter  = 22
)

// The user-identities block of migration 022_consolidated (#1427; originally
// migration 023) on its OWN scratch database: it migrates down, which would
// corrupt the shared integrationDB.
//
// It pins the backfill of user_identities from the inline identity columns, the
// lower-casing of users.email with its CHECK, the refusal to run over accounts
// that differ only by case, and the down migration.
func TestMigration022_UserIdentities(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()
	m := newMigrator(t, db)

	// reset returns the database to version 021 with no users.
	reset := func(t *testing.T) {
		t.Helper()
		// A failed migration leaves the version dirty; clear it the way an
		// operator would before migrating again.
		if version, dirty, err := m.Version(); err == nil && dirty {
			require.NoError(t, m.Force(int(version)-1))
		}
		require.NoError(t, m.Migrate(migrationUserIdentitiesBefore), "migrate to 021")
		_, err := db.Exec("TRUNCATE TABLE users CASCADE")
		require.NoError(t, err)
		require.False(t, tableExists(t, db, "user_identities"), "fixture: the table must not exist before 022")
	}
	// seed inserts a pre-022 user; nil means the column is NULL.
	seed := func(t *testing.T, email string, googleID, provider, subject *string) string {
		t.Helper()
		id := uuid.New().String()
		_, err := db.Exec(
			"INSERT INTO users (id, email, name, google_id, idp_provider, idp_subject) VALUES ($1, $2, 'U', $3, $4, $5)",
			id, email, googleID, provider, subject)
		require.NoError(t, err)
		return id
	}
	identitiesOf := func(t *testing.T, userID string) map[string]string {
		t.Helper()
		rows, err := db.Query("SELECT provider, subject FROM user_identities WHERE user_id = $1", userID)
		require.NoError(t, err)
		defer rows.Close() //nolint:errcheck
		got := map[string]string{}
		for rows.Next() {
			var provider, subject string
			require.NoError(t, rows.Scan(&provider, &subject))
			got[provider] = subject
		}
		require.NoError(t, rows.Err())
		return got
	}
	str := func(s string) *string { return &s }

	t.Run("backfills every inline identity", func(t *testing.T) {
		reset(t)
		oidc := seed(t, "oidc@example.com", nil, str("corp-sso"), str("sub-1"))
		legacy := seed(t, "legacy@example.com", str("g-legacy"), nil, nil)
		google := seed(t, "google@example.com", str("g-1"), str("google"), str("g-1"))
		dev := seed(t, "dev@example.com", nil, str("dev"), str("dev_dev@example.com"))
		pending := seed(t, "pending@example.com", nil, str("corp-sso"), nil)
		bare := seed(t, "bare@example.com", nil, nil, nil)

		require.NoError(t, m.Migrate(migrationUserIdentitiesAfter), "migrate to 022")

		assert.Equal(t, map[string]string{"corp-sso": "sub-1"}, identitiesOf(t, oidc))
		assert.Equal(t, map[string]string{models.LegacyGoogleIdentityProvider: "g-legacy"}, identitiesOf(t, legacy),
			"a google_id with no slug identity is kept under the reserved provider")
		assert.Equal(t, map[string]string{"google": "g-1", models.LegacyGoogleIdentityProvider: "g-1"},
			identitiesOf(t, google), "a Google user keeps both ways of being found")
		assert.Equal(t, map[string]string{"dev": "dev_dev@example.com"}, identitiesOf(t, dev))
		assert.Empty(t, identitiesOf(t, pending), "a provider hint with no subject is not an identity")
		assert.Empty(t, identitiesOf(t, bare))

		assert.Equal(t, 0, countRows(t, db, `SELECT count(*) FROM users u
			WHERE (u.idp_provider IS NOT NULL AND u.idp_subject IS NOT NULL AND NOT EXISTS (
			          SELECT 1 FROM user_identities i WHERE i.user_id = u.id
			             AND i.provider = u.idp_provider AND i.subject = u.idp_subject))
			   OR (u.google_id IS NOT NULL AND NOT EXISTS (
			          SELECT 1 FROM user_identities i WHERE i.user_id = u.id
			             AND i.provider = 'legacy_google_id' AND i.subject = u.google_id))`),
			"no user with an idp_subject or google_id is left without a matching identity")
		assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM user_identities WHERE last_used_at IS NOT NULL"),
			"a backfilled identity has not been used yet")
		assert.Equal(t, 0, countRows(t, db,
			"SELECT count(*) FROM user_identities i JOIN users u ON u.id = i.user_id WHERE i.created_at <> u.created_at"),
			"a backfilled identity is as old as its account")
		assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM users WHERE email_verified_at IS NOT NULL"),
			"verification was never recorded, so nobody starts verified")
	})

	t.Run("lower-cases emails and pins them", func(t *testing.T) {
		reset(t)
		mixed := seed(t, "Ada.Lovelace@Example.COM", nil, nil, nil)
		lower := seed(t, "grace@example.com", nil, nil, nil)

		require.NoError(t, m.Migrate(migrationUserIdentitiesAfter), "migrate to 022")

		var email string
		require.NoError(t, db.QueryRow("SELECT email FROM users WHERE id = $1", mixed).Scan(&email))
		assert.Equal(t, "ada.lovelace@example.com", email)
		require.NoError(t, db.QueryRow("SELECT email FROM users WHERE id = $1", lower).Scan(&email))
		assert.Equal(t, "grace@example.com", email)

		_, err := db.Exec("INSERT INTO users (email, name) VALUES ('Mixed@Example.com', 'M')")
		requirePQCode(t, err, pqCheckViolation)
		_, err = db.Exec("UPDATE users SET email = 'GRACE@example.com' WHERE id = $1", lower)
		requirePQCode(t, err, pqCheckViolation)
		_, err = db.Exec("INSERT INTO users (email, name) VALUES ('grace@example.com', 'Dup')")
		requirePQCode(t, err, "23505")
	})

	t.Run("stops, changing nothing, when accounts differ only by case", func(t *testing.T) {
		reset(t)
		first := seed(t, "Sam@Example.com", nil, str("corp-sso"), str("sam-1"))
		second := seed(t, "sam@example.com", nil, str("corp-sso"), str("sam-2"))
		seed(t, "Unrelated@Example.com", nil, nil, nil)

		err := m.Migrate(migrationUserIdentitiesAfter)

		require.Error(t, err)
		for _, want := range []string{
			"differ only by letter case", "Sam@Example.com", "sam@example.com", first, second,
		} {
			assert.Contains(t, err.Error(), want, "the error names the conflicting accounts")
		}
		assert.NotContains(t, err.Error(), "Unrelated@Example.com", "only the conflicting accounts are named")
		// The operator is told the version to re-point to: the one before the
		// consolidated migration, so the retry runs all of it.
		assert.Contains(t, err.Error(), "migration 022 stopped")
		assert.Contains(t, err.Error(), "SET version = 21,")

		// The blocks run in one transaction, so the block before this one
		// (resource_write_events) is rolled back too.
		assert.False(t, tableExists(t, db, "resource_write_events"))
		assert.False(t, tableExists(t, db, "user_identities"))
		assert.False(t, columnExists(t, db, "users", "email_verified_at"))
		assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM users WHERE email = 'Unrelated@Example.com'"),
			"no email was lower-cased")

		// Once the operator resolves the conflict and clears the failed
		// migration, it applies.
		_, err = db.Exec("UPDATE users SET email = 'sam.two@example.com' WHERE id = $1", second)
		require.NoError(t, err)
		require.NoError(t, m.Force(migrationUserIdentitiesBefore))
		require.NoError(t, m.Migrate(migrationUserIdentitiesAfter))
		assert.Equal(t, 2, countRows(t, db, "SELECT count(*) FROM user_identities"))
	})

	t.Run("constraints on user_identities", func(t *testing.T) {
		reset(t)
		owner := seed(t, "owner@example.com", nil, str("corp-sso"), str("sub-1"))
		other := seed(t, "other@example.com", nil, nil, nil)
		require.NoError(t, m.Migrate(migrationUserIdentitiesAfter), "migrate to 022")

		_, err := db.Exec("INSERT INTO user_identities (user_id, provider, subject) VALUES ($1, 'corp-sso', 'sub-1')", other)
		requirePQCode(t, err, "23505")
		_, err = db.Exec("INSERT INTO user_identities (user_id, provider, subject) VALUES ($1, 'x', 'y')", uuid.New().String())
		requirePQCode(t, err, "23503")
		_, err = db.Exec("INSERT INTO user_identities (user_id, provider, subject) VALUES ($1, 'github', 'gh-1')", owner)
		require.NoError(t, err, "one user may hold several identities")

		_, err = db.Exec("DELETE FROM users WHERE id = $1", owner)
		require.NoError(t, err)
		assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM user_identities"), "identities go with their user")
	})

	t.Run("down drops a populated table and keeps the inline identity", func(t *testing.T) {
		reset(t)
		id := seed(t, "Down@Example.com", str("g-down"), str("google"), str("g-down"))
		require.NoError(t, m.Migrate(migrationUserIdentitiesAfter), "migrate to 022")
		require.NotEmpty(t, identitiesOf(t, id))

		require.NoError(t, m.Migrate(migrationUserIdentitiesBefore), "migrate back to 021")

		assert.False(t, tableExists(t, db, "user_identities"))
		assert.False(t, columnExists(t, db, "users", "email_verified_at"))
		var email string
		var googleID, provider, subject sql.NullString
		require.NoError(t, db.QueryRow(
			"SELECT email, google_id, idp_provider, idp_subject FROM users WHERE id = $1", id).
			Scan(&email, &googleID, &provider, &subject))
		assert.Equal(t, "down@example.com", email, "the original casing was not kept")
		assert.Equal(t, "g-down", googleID.String)
		assert.Equal(t, "google", provider.String)
		assert.Equal(t, "g-down", subject.String)
		_, err := db.Exec("INSERT INTO users (email, name) VALUES ('Mixed@Example.com', 'M')")
		require.NoError(t, err, "the lower-case CHECK is gone")

		// And up again over the restored state.
		_, err = db.Exec("DELETE FROM users WHERE email = 'Mixed@Example.com'")
		require.NoError(t, err)
		require.NoError(t, m.Migrate(migrationUserIdentitiesAfter), "migrate to 022 again")
		assert.Len(t, identitiesOf(t, id), 2)
	})
}
