//go:build integration

package postgres

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigration011_RemoveAIToolHooks verifies the remove-ai-tool-hooks block of
// migration 011_consolidated (epic #610, issue #614; originally migration 015)
// against a real Postgres, on its OWN scratch database.
//
// It deliberately does not use the shared `integrationDB`: TestMain migrates that one
// straight to head, whereas this test must observe the pre-migration (v0.8.0,
// version 010) state, seed the legacy shapes the migration has to cope with, and only
// then apply 011. Migrating the shared database up and down mid-run would corrupt
// every other suite in the binary (and, because that database is shared across
// worktrees, other checkouts too).
//
// The critical property under test is the one that cannot be walked back: **no API key
// is deleted and no key loses the ability to authenticate.**
func TestMigration011_RemoveAIToolHooks(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)

	// 1. Bring the schema to the version immediately BEFORE this migration.
	require.NoError(t, m.Migrate(10), "migrate to 010")

	// 2. Seed the pre-migration state.
	fx := seedAIToolHooksFixtures(t, db)

	// Sanity-check the fixtures really are in the pre-migration shape, so a silently
	// failed seed cannot make the post-migration assertions pass vacuously.
	require.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM api_keys WHERE usage_type = 'ai_tools'`),
		"fixture: a legacy ai_tools key must exist before migrating")
	require.Equal(t, 3, countRows(t, db,
		`SELECT count(*) FROM api_key_integration_permissions WHERE integration_code = 'ai_tools'`),
		"fixture: ai_tools grants must exist before migrating")
	require.Equal(t, 2, countRows(t, db, `SELECT count(*) FROM claude_code_hooks_payload`))
	require.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM cursor_ide_hooks_payload`))
	require.Equal(t, 4, countRows(t, db, `SELECT count(*) FROM activities`))

	// 3. Apply migration 011.
	require.NoError(t, m.Migrate(11), "migrate to 011")

	t.Run("no API key is deleted", func(t *testing.T) {
		assert.Equal(t, 3, countRows(t, db, `SELECT count(*) FROM api_keys`),
			"all three seeded keys must survive")
	})

	t.Run("no key loses the ability to authenticate", func(t *testing.T) {
		// The legacy key was re-pointed rather than left on a value the new CHECK rejects.
		assert.Equal(t, 0, countRows(t, db, `SELECT count(*) FROM api_keys WHERE usage_type = 'ai_tools'`))
		assert.Equal(t, 1, countRows(t, db,
			`SELECT count(*) FROM api_keys WHERE id = $1 AND usage_type = 'everything'`, fx.legacyAIKey),
			"the legacy ai_tools key must be re-pointed to everything, not deleted")

		// The key whose ONLY grant was ai_tools still exists; it now has zero grants,
		// which is inert but authenticates.
		assert.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM api_keys WHERE id = $1`, fx.onlyAIKey))
		assert.Equal(t, 0, countRows(t, db,
			`SELECT count(*) FROM api_key_integration_permissions WHERE api_key_id = $1`, fx.onlyAIKey))

		// The mixed key keeps its surviving grant.
		codes := integrationCodesFor(t, db, fx.mixedKey)
		assert.Equal(t, []string{"cli"}, codes, "the cli grant must survive, only ai_tools is dropped")
	})

	t.Run("ai_tools scope is fully retired", func(t *testing.T) {
		assert.Equal(t, 0, countRows(t, db,
			`SELECT count(*) FROM api_key_integration_permissions WHERE integration_code = 'ai_tools'`))
		assert.Equal(t, 0, countRows(t, db,
			`SELECT count(*) FROM api_key_integrations_catalog WHERE integration_code = 'ai_tools'`))
		// The other catalog rows are untouched.
		assert.Equal(t, 1, countRows(t, db,
			`SELECT count(*) FROM api_key_integrations_catalog WHERE integration_code = 'cli'`))
	})

	t.Run("chk_usage_type allows exactly cli, mcp, everything", func(t *testing.T) {
		for _, allowed := range []string{"cli", "mcp", "everything"} {
			_, err := db.Exec(`UPDATE api_keys SET usage_type = $1 WHERE id = $2`, allowed, fx.legacyAIKey)
			assert.NoErrorf(t, err, "usage_type %q must still be permitted", allowed)
		}
		_, err := db.Exec(`UPDATE api_keys SET usage_type = 'ai_tools' WHERE id = $1`, fx.legacyAIKey)
		assert.Error(t, err, "usage_type ai_tools must now violate chk_usage_type")
	})

	t.Run("hook payload tables are dropped", func(t *testing.T) {
		assert.False(t, tableExists(t, db, "claude_code_hooks_payload"))
		assert.False(t, tableExists(t, db, "cursor_ide_hooks_payload"))
	})

	t.Run("claude_code activity rows are purged, others kept", func(t *testing.T) {
		assert.Equal(t, 0, countRows(t, db,
			`SELECT count(*) FROM activities
			  WHERE activity_type IN ('claude_code_session','claude_code_tool','claude_code_prompt')`))
		assert.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM activities`),
			"the unrelated activity row must be preserved")
	})

	t.Run("down restores structure and re-runs clean", func(t *testing.T) {
		require.NoError(t, m.Migrate(10), "roll back to 010")
		assert.True(t, tableExists(t, db, "claude_code_hooks_payload"), "down must recreate the table")
		assert.True(t, tableExists(t, db, "cursor_ide_hooks_payload"), "down must recreate the table")
		assert.Equal(t, 1, countRows(t, db,
			`SELECT count(*) FROM api_key_integrations_catalog WHERE integration_code = 'ai_tools'`),
			"down must restore the catalog row")

		// Structure only: the deleted rows are gone for good, and the down file says so.
		assert.Equal(t, 0, countRows(t, db, `SELECT count(*) FROM claude_code_hooks_payload`),
			"down restores structure, not data")

		// Re-applying up over a rolled-back database must still succeed (idempotent cycle).
		require.NoError(t, m.Migrate(11), "re-apply 011 after rollback")
		assert.False(t, tableExists(t, db, "claude_code_hooks_payload"))
	})

}

// aiToolHooksFixtures holds the ids of the seeded API keys, so assertions can
// address each ai_tools shape the migration has to handle.
type aiToolHooksFixtures struct {
	legacyAIKey string // legacy key pinned to usage_type = 'ai_tools'
	onlyAIKey   string // key whose ONLY grant is ai_tools
	mixedKey    string // key granted ai_tools AND cli
}

// seedAIToolHooksFixtures writes the pre-migration state: three API keys spanning
// every ai_tools shape the migration must handle, hook payload rows in both tables,
// and activity rows of which only some are Claude Code.
func seedAIToolHooksFixtures(t *testing.T, db *sql.DB) aiToolHooksFixtures {
	t.Helper()

	userID := uuid.New().String()
	_, err := db.Exec(
		`INSERT INTO users (id, email, name, created_at, updated_at)
		 VALUES ($1, $2, 'Migration Fixture', NOW(), NOW())`,
		userID, fmt.Sprintf("migration-015-%s@example.com", userID[:8]))
	require.NoError(t, err, "seed user")

	teamID := uuid.New().String()
	_, err = db.Exec(
		`INSERT INTO teams (id, name, slug, owner_id, is_personal, created_at, updated_at)
		 VALUES ($1, 'Migration Fixture Team', $2, $3, false, NOW(), NOW())`,
		teamID, "migration-015-"+teamID[:8], userID)
	require.NoError(t, err, "seed team")

	fx := aiToolHooksFixtures{
		legacyAIKey: uuid.New().String(),
		onlyAIKey:   uuid.New().String(),
		mixedKey:    uuid.New().String(),
	}

	// The legacy key is the one that makes a CHECK-first ordering fail: its
	// usage_type is a value the rewritten constraint no longer permits.
	keys := []struct {
		id, label, usageType string
		grants               []string
	}{
		{fx.legacyAIKey, "legacy-ai", "ai_tools", []string{"ai_tools"}},
		{fx.onlyAIKey, "only-ai", "everything", []string{"ai_tools"}},
		{fx.mixedKey, "ai-and-cli", "everything", []string{"ai_tools", "cli"}},
	}
	for _, k := range keys {
		_, err = db.Exec(
			`INSERT INTO api_keys (id, name, user_id, key_hash, key_prefix, usage_type, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, 'aait-x', $5, NOW(), NOW())`,
			k.id, k.label, userID, "hash-"+k.label, k.usageType)
		require.NoErrorf(t, err, "seed api key %s", k.label)

		for _, code := range k.grants {
			_, err = db.Exec(
				`INSERT INTO api_key_integration_permissions (id, api_key_id, integration_code, granted_at)
				 VALUES ($1, $2, $3, NOW())`,
				uuid.New().String(), k.id, code)
			require.NoErrorf(t, err, "seed grant %s for %s", code, k.label)
		}
	}

	// Hook payload rows in both tables.
	_, err = db.Exec(
		`INSERT INTO claude_code_hooks_payload (session_id, hook_event_name, payload, team_id, user_id)
		 VALUES ('sess-a', 'PostToolUse', '{}', $1, $2), ('sess-b', 'PostToolUse', '{}', $1, $2)`,
		teamID, userID)
	require.NoError(t, err, "seed claude hook rows")
	_, err = db.Exec(
		`INSERT INTO cursor_ide_hooks_payload (session_id, hook_event_name, payload, team_id, user_id)
		 VALUES ('sess-c', 'afterShellExecution', '{}', $1, $2)`,
		teamID, userID)
	require.NoError(t, err, "seed cursor hook row")

	// Three Claude Code activity rows plus one unrelated row that must survive.
	for _, at := range []string{"claude_code_session", "claude_code_tool", "claude_code_prompt", "api_key_created"} {
		_, err = db.Exec(
			`INSERT INTO activities (id, user_id, activity_type, entity_type, description, created_at)
			 VALUES ($1, $2, $3, 'session', 'migration fixture', NOW())`,
			uuid.New().String(), userID, at)
		require.NoErrorf(t, err, "seed activity %s", at)
	}

	return fx
}

// newScratchMigrationDB creates (and drops) a throwaway database so this test can
// migrate up and down without touching the shared integration database.
func newScratchMigrationDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	base := os.Getenv("POSTGRES_TEST_DSN")
	if base == "" {
		base = defaultTestDSN
	}
	u, err := url.Parse(base)
	require.NoError(t, err, "parse base DSN")

	name := "vibexp_mig_" + strings.ReplaceAll(uuid.New().String()[:8], "-", "")
	adminURL := *u
	adminURL.Path = "/postgres"

	admin, err := sql.Open("postgres", adminURL.String())
	require.NoError(t, err, "open admin connection")
	defer closeQuietly(admin)

	_, err = admin.Exec("DROP DATABASE IF EXISTS " + pq.QuoteIdentifier(name))
	require.NoError(t, err)
	_, err = admin.Exec("CREATE DATABASE " + pq.QuoteIdentifier(name))
	require.NoError(t, err, "create scratch database (is docker-compose postgres running?)")

	scratchURL := *u
	scratchURL.Path = "/" + name
	db, err := sql.Open("postgres", scratchURL.String())
	require.NoError(t, err, "open scratch database")
	require.NoError(t, db.Ping(), "ping scratch database")

	return db, func() {
		closeQuietly(db)
		cleanupAdmin, adminErr := sql.Open("postgres", adminURL.String())
		if adminErr != nil {
			t.Logf("scratch cleanup: open admin: %v", adminErr)
			return
		}
		defer closeQuietly(cleanupAdmin)
		dropStmt := "DROP DATABASE IF EXISTS " + pq.QuoteIdentifier(name) + " WITH (FORCE)"
		if _, dropErr := cleanupAdmin.Exec(dropStmt); dropErr != nil {
			t.Logf("scratch cleanup: drop %s: %v", name, dropErr)
		}
	}
}

func newMigrator(t *testing.T, db *sql.DB) *migrate.Migrate {
	t.Helper()
	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	require.NoError(t, err, "create migrate driver")
	m, err := migrate.NewWithDatabaseInstance("file://../../../migrations", "postgres", driver)
	require.NoError(t, err, "create migrate instance")
	return m
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(query, args...).Scan(&n), "count query: %s", query)
	return n
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRow(
		`SELECT EXISTS (
		    SELECT 1 FROM information_schema.tables
		     WHERE table_schema = 'public' AND table_name = $1
		 )`, name).Scan(&exists))
	return exists
}

func integrationCodesFor(t *testing.T, db *sql.DB, apiKeyID string) []string {
	t.Helper()
	rows, err := db.Query(
		`SELECT integration_code FROM api_key_integration_permissions
		  WHERE api_key_id = $1 ORDER BY integration_code`, apiKeyID)
	require.NoError(t, err)
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			t.Logf("close rows: %v", closeErr)
		}
	}()

	var codes []string
	for rows.Next() {
		var code string
		require.NoError(t, rows.Scan(&code))
		codes = append(codes, code)
	}
	require.NoError(t, rows.Err())
	if codes == nil {
		codes = []string{}
	}
	return codes
}
