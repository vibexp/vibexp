//go:build integration

package postgres

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigration018_RemoveWebPushPreferencesKeys verifies migration 018 (issue
// #690) against a real Postgres, on its OWN scratch database.
//
// Like the 015–017 migration tests it avoids the shared `integrationDB`: this
// test must observe the pre-018 state, seed rows carrying the stale JSONB keys
// the migration has to strip, and only then apply 018.
//
// The load-bearing property is not "web_push is gone" — that is trivially true
// if the seed silently failed. It is that the SURVIVING keys (in_app, email,
// per-type values, sibling preference categories) are byte-identical
// afterwards, so the strip cannot have taken live preferences with it.
func TestMigration018_RemoveWebPushPreferencesKeys(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)

	// 1. Bring the schema to the version immediately BEFORE this migration.
	require.NoError(t, m.Migrate(17), "migrate to 017")

	// 2. Seed the pre-migration state: one row with web_push at both shapes
	// (channels + per-type), one row without any web_push key.
	withKeysID, withoutKeysID := seedMigration018Fixtures(t, db)

	// Sanity-check the fixtures really are in the pre-migration shape, so a
	// silently failed seed cannot make the post-migration assertions pass
	// vacuously.
	require.True(t, jsonbKeyExists(t, db, withKeysID, "{notifications,channels}", "web_push"),
		"fixture: channels web_push must exist before migrating")
	require.True(t, jsonbPathKeyExists(t, db, withKeysID,
		`SELECT t.value ? 'web_push'
		   FROM user_preferences up, jsonb_each(up.preferences #> '{notifications,types}') t
		  WHERE up.user_id = $1 AND t.key = 'feed.item.created'`),
		"fixture: per-type web_push must exist before migrating")
	require.False(t, jsonbKeyExists(t, db, withoutKeysID, "{notifications,channels}", "web_push"),
		"fixture: the clean row must NOT carry web_push before migrating")

	// 3. Apply migration 018.
	require.NoError(t, m.Migrate(18), "migrate to 018")

	t.Run("web_push keys are stripped at both shapes", func(t *testing.T) {
		assert.False(t, jsonbKeyExists(t, db, withKeysID, "{notifications,channels}", "web_push"),
			"channels web_push must be stripped")
		assert.False(t, jsonbPathKeyExists(t, db, withKeysID,
			`SELECT t.value ? 'web_push'
			   FROM user_preferences up, jsonb_each(up.preferences #> '{notifications,types}') t
			  WHERE up.user_id = $1 AND t.key = 'feed.item.created'`),
			"per-type web_push must be stripped")
	})

	t.Run("surviving keys are untouched", func(t *testing.T) {
		var channels, feedType, emailNotification string
		require.NoError(t, db.QueryRow(
			`SELECT preferences #>> '{notifications,channels}',
			        preferences #>> '{notifications,types,feed.item.created}',
			        preferences #>> '{email_notification}'
			   FROM user_preferences WHERE user_id = $1`, withKeysID).
			Scan(&channels, &feedType, &emailNotification))
		assert.JSONEq(t, `{"in_app":true,"email":false}`, channels,
			"channels must keep exactly its surviving keys")
		assert.JSONEq(t, `{"in_app":true,"email":"digest"}`, feedType,
			"per-type object must keep exactly its surviving keys")
		assert.JSONEq(t, `{"platform_announcement":true,"marketing_promotional":false}`, emailNotification,
			"sibling preference categories must be untouched")
	})

	t.Run("rows without web_push are byte-identical", func(t *testing.T) {
		var doc string
		require.NoError(t, db.QueryRow(
			`SELECT preferences::text FROM user_preferences WHERE user_id = $1`, withoutKeysID).
			Scan(&doc))
		assert.JSONEq(t,
			`{"notifications":{"channels":{"in_app":false,"email":true},"types":{}}}`, doc,
			"a row that never carried web_push must not be rewritten")
	})

	// 4. Down is a documented no-op; the cycle must land on the post-018 state.
	t.Run("down is a no-op and up re-applies cleanly", func(t *testing.T) {
		require.NoError(t, m.Migrate(17), "migrate down to 017")
		assert.False(t, jsonbKeyExists(t, db, withKeysID, "{notifications,channels}", "web_push"),
			"down must not resurrect web_push keys")

		require.NoError(t, m.Migrate(18), "re-apply 018 after rolling back")
		assert.False(t, jsonbKeyExists(t, db, withKeysID, "{notifications,channels}", "web_push"))
	})
}

// seedMigration018Fixtures inserts two users with preferences rows: one
// carrying web_push at both shapes (channels and per-type) plus sibling
// categories, one with no web_push key at all. Returns their user ids.
func seedMigration018Fixtures(t *testing.T, db *sql.DB) (withKeys, withoutKeys string) {
	t.Helper()

	withKeys = uuid.New().String()
	withoutKeys = uuid.New().String()

	_, err := db.Exec(
		`INSERT INTO users (id, email, name) VALUES ($1, $2, $3), ($4, $5, $6)`,
		withKeys, "with-keys@migration018.test", "Migration 018 With Keys",
		withoutKeys, "without-keys@migration018.test", "Migration 018 Without Keys")
	require.NoError(t, err, "seed users")

	_, err = db.Exec(
		`INSERT INTO user_preferences (user_id, preferences) VALUES ($1, $2::jsonb)`,
		withKeys,
		`{"email_notification":{"platform_announcement":true,"marketing_promotional":false},
		  "notifications":{"channels":{"in_app":true,"email":false,"web_push":true},
		                   "types":{"feed.item.created":{"in_app":true,"email":"digest","web_push":false}}}}`)
	require.NoError(t, err, "seed preferences with web_push")

	_, err = db.Exec(
		`INSERT INTO user_preferences (user_id, preferences) VALUES ($1, $2::jsonb)`,
		withoutKeys,
		`{"notifications":{"channels":{"in_app":false,"email":true},"types":{}}}`)
	require.NoError(t, err, "seed preferences without web_push")

	return withKeys, withoutKeys
}

// jsonbKeyExists reports whether the preferences row for userID carries key at
// the given jsonb path (a literal like "{notifications,channels}").
func jsonbKeyExists(t *testing.T, db *sql.DB, userID, path, key string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRow(
		`SELECT (preferences #> '`+path+`') ? '`+key+`'
		   FROM user_preferences WHERE user_id = $1`, userID).Scan(&exists))
	return exists
}

// jsonbPathKeyExists runs a boolean existence query parameterised by user id.
func jsonbPathKeyExists(t *testing.T, db *sql.DB, userID, query string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRow(query, userID).Scan(&exists))
	return exists
}
