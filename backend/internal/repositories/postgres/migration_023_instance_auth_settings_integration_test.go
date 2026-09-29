//go:build integration

package postgres

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Up/down round-trip for migration 023_instance_auth_settings (#1231), on its
// OWN scratch database: this test migrates DOWN, which would corrupt the shared
// integrationDB.
//
// Beyond "it applies", it pins the provider invariants at the database level
// (one google, one github, unique slug, issuer iff oidc), the seeded version
// row, both singletons, the user FK behaviour, and that the down migration
// drops POPULATED tables.

var instanceAuthSettingsTables = []string{
	"instance_auth_providers", "instance_auth_allowlist", "instance_auth_settings_version", "instance_admins",
}

func validInstanceAuthProviderRow() map[string]any {
	return map[string]any{
		"type":         "oidc",
		"slug":         "corp",
		"display_name": "Corp",
		"client_id":    "client",
		"issuer_url":   "https://sso.example.com",
	}
}

var instanceAuthProviderCheckCases = []instanceSettingsCheckCase{
	{"a valid oidc row", nil, true},
	{"a google row", map[string]any{"type": "google", "issuer_url": nil}, true},
	{"a github row", map[string]any{"type": "github", "issuer_url": nil}, true},
	{"an unknown type", map[string]any{"type": "saml", "issuer_url": nil}, false},
	{"oidc without an issuer", map[string]any{"issuer_url": nil}, false},
	{"google with an issuer", map[string]any{"type": "google"}, false},
	{"github with an issuer", map[string]any{"type": "github"}, false},
	{"an upper-case slug", map[string]any{"slug": "Corp"}, false},
	{"a slug starting with a hyphen", map[string]any{"slug": "-corp"}, false},
	{"a slug with an underscore", map[string]any{"slug": "corp_sso"}, false},
	{"an empty slug", map[string]any{"slug": ""}, false},
}

func TestMigration023_InstanceAuthSettings_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)

	// 1. The version immediately before this migration: nothing exists yet.
	require.NoError(t, m.Migrate(22), "migrate to 022")
	for _, table := range instanceAuthSettingsTables {
		require.False(t, tableExists(t, db, table), "fixture: %s must not exist before 023", table)
	}

	// 2. Apply 023.
	require.NoError(t, m.Migrate(23), "migrate to 023")

	t.Run("the settings version is seeded with one row at 1", func(t *testing.T) {
		assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM instance_auth_settings_version WHERE id AND version = 1"))
		assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM instance_auth_settings_version"))
		_, err := db.Exec("INSERT INTO instance_auth_settings_version (version) VALUES (5)")
		requirePQCode(t, err, "23505")
		_, err = db.Exec("INSERT INTO instance_auth_settings_version (id, version) VALUES (false, 5)")
		requirePQCode(t, err, pqCheckViolation)
	})

	t.Run("the allowlist is a singleton with empty-array defaults", func(t *testing.T) {
		_, err := db.Exec("INSERT INTO instance_auth_allowlist DEFAULT VALUES")
		require.NoError(t, err)
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM instance_auth_allowlist WHERE domains = '{}' AND emails = '{}' AND version = 1"))
		_, err = db.Exec("INSERT INTO instance_auth_allowlist DEFAULT VALUES")
		requirePQCode(t, err, "23505")
		_, err = db.Exec("INSERT INTO instance_auth_allowlist (id) VALUES (false)")
		requirePQCode(t, err, pqCheckViolation)
	})

	t.Run("instance_auth_providers CHECKs", func(t *testing.T) {
		runInstanceSettingsChecks(t, db, "instance_auth_providers",
			validInstanceAuthProviderRow(), instanceAuthProviderCheckCases)
	})

	t.Run("at most one google, one github, and a unique slug", func(t *testing.T) {
		row := validInstanceAuthProviderRow()
		insert := func(overrides map[string]any) error {
			return insertInstanceSettingsRow(db, "instance_auth_providers", row, overrides)
		}
		require.NoError(t, insert(map[string]any{"type": "google", "slug": "google", "issuer_url": nil}))
		requirePQCode(t, insert(map[string]any{"type": "google", "slug": "google-2", "issuer_url": nil}), "23505")

		require.NoError(t, insert(map[string]any{"type": "github", "slug": "github", "issuer_url": nil}))
		requirePQCode(t, insert(map[string]any{"type": "github", "slug": "github-2", "issuer_url": nil}), "23505")

		require.NoError(t, insert(map[string]any{"slug": "sso-a"}))
		require.NoError(t, insert(map[string]any{"slug": "sso-b"}), "two oidc rows with distinct slugs")
		requirePQCode(t, insert(map[string]any{"slug": "sso-a"}), "23505")
		requirePQCode(t, insert(map[string]any{"type": "github", "slug": "sso-a", "issuer_url": nil}), "23505")

		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM instance_auth_providers WHERE slug = 'google' AND enabled = false AND sort_order = 0"),
			"a new provider starts disabled at sort order 0")
	})

	t.Run("deleting a user removes their grant and blanks editor/grantor", func(t *testing.T) {
		grantor, grantee := uuid.New().String(), uuid.New().String()
		for _, id := range []string{grantor, grantee} {
			_, err := db.Exec("INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
				id, "auth-"+id[:8]+"@example.com", "Auth Fixture")
			require.NoError(t, err)
		}
		_, err := db.Exec("INSERT INTO instance_admins (user_id, granted_by) VALUES ($1, $2), ($2, NULL)",
			grantee, grantor)
		require.NoError(t, err)
		_, err = db.Exec("UPDATE instance_auth_providers SET updated_by = $1", grantor)
		require.NoError(t, err)
		_, err = db.Exec("UPDATE instance_auth_allowlist SET updated_by = $1", grantor)
		require.NoError(t, err)

		_, err = db.Exec("INSERT INTO instance_admins (user_id) VALUES ($1)", grantee)
		requirePQCode(t, err, "23505")
		_, err = db.Exec("INSERT INTO instance_admins (user_id) VALUES ($1)", uuid.New().String())
		requirePQCode(t, err, "23503")

		_, err = db.Exec("DELETE FROM users WHERE id = $1", grantor)
		require.NoError(t, err)
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM instance_admins WHERE user_id = $1 AND granted_by IS NULL", grantee))
		assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM instance_admins WHERE user_id = $1", grantor))
		assert.Equal(t, 0, countRows(t, db,
			"SELECT count(*) FROM instance_auth_providers WHERE updated_by IS NOT NULL"))
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM instance_auth_allowlist WHERE updated_by IS NULL"))

		_, err = db.Exec("DELETE FROM users WHERE id = $1", grantee)
		require.NoError(t, err)
		assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM instance_admins"), "the grant goes with its user")

		_, err = db.Exec("INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
			grantee, "auth-again@example.com", "Auth Fixture")
		require.NoError(t, err)
		_, err = db.Exec("INSERT INTO instance_admins (user_id) VALUES ($1)", grantee)
		require.NoError(t, err, "a populated instance_admins for the rollback below")
	})

	// 3. Roll back with every table populated.
	for _, table := range instanceAuthSettingsTables {
		require.Positive(t, countRows(t, db, "SELECT count(*) FROM "+table),
			"fixture: %s must hold rows before rolling back", table)
	}
	require.NoError(t, m.Migrate(22), "migrate down to 022")
	for _, table := range instanceAuthSettingsTables {
		assert.False(t, tableExists(t, db, table), "down must drop the populated %s", table)
	}
	assert.True(t, tableExists(t, db, "instance_search_settings"), "rolling back 023 must leave 022 in place")

	// 4. Re-apply: an operator who rolls back must be able to roll forward.
	require.NoError(t, m.Migrate(23), "re-apply 023")
	assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM instance_auth_settings_version WHERE version = 1"),
		"the re-applied version row is seeded again")
	assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM instance_auth_providers"))
	require.NoError(t, insertInstanceSettingsRow(db, "instance_auth_providers", validInstanceAuthProviderRow(), nil))
}
