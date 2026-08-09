//go:build integration

package postgres

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Up/down round-trip for migration 013_resource_freshness (#729), on its OWN
// scratch database: this test migrates DOWN, which would corrupt the shared
// integrationDB (and, since that database is shared across worktrees, other
// checkouts too).
//
// Two properties matter beyond "it applies":
//  1. the down migration drops the resource columns even when they hold data
//     (DROP COLUMN is catalog-only, but the migration must not be written in a
//     way that trips over populated columns);
//  2. going down and back up leaves the schema usable, so an operator who
//     rolls back a release is not stranded.

// freshnessTables are the four tables 013 creates.
var freshnessTables = []string{
	"resource_freshness",
	"freshness_rules",
	"team_freshness_settings",
	"resource_freshness_audit",
}

// freshnessResourceTables are the tables that gain last_accessed_* columns.
var freshnessResourceTables = []string{"prompts", "artifacts", "blueprints", "memories"}

// countLastAccessedColumns reports how many last_accessed_* columns a table has.
func countLastAccessedColumns(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	return countRows(t, db,
		`SELECT count(*) FROM information_schema.columns
		  WHERE table_schema = 'public' AND table_name = $1
		    AND column_name LIKE 'last_accessed%'`, table)
}

func TestMigration013_ResourceFreshness_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)

	// 1. The version immediately before this migration: nothing exists yet.
	require.NoError(t, m.Migrate(12), "migrate to 012")
	for _, table := range freshnessTables {
		require.False(t, tableExists(t, db, table),
			"fixture: %s must not exist before 013", table)
	}
	for _, table := range freshnessResourceTables {
		require.Equal(t, 0, countLastAccessedColumns(t, db, table),
			"fixture: %s must have no last_accessed columns before 013", table)
	}

	// 2. Apply 013.
	require.NoError(t, m.Migrate(13), "migrate to 013")

	t.Run("creates all four tables", func(t *testing.T) {
		for _, table := range freshnessTables {
			assert.True(t, tableExists(t, db, table), "%s must exist after 013", table)
		}
	})

	t.Run("adds four last_accessed columns to every resource table", func(t *testing.T) {
		for _, table := range freshnessResourceTables {
			assert.Equal(t, 4, countLastAccessedColumns(t, db, table),
				"%s must gain one column per medium", table)
		}
	})

	t.Run("nothing is backfilled", func(t *testing.T) {
		for _, table := range freshnessTables {
			assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM "+table),
				"%s must start empty -- 013 seeds nothing (decision #7)", table)
		}
	})

	// 3. Seed data into BOTH the new tables and the new columns, so the down
	//    migration is exercised against populated columns rather than empty ones.
	teamID, promptID := seedMigration013Fixtures(t, db)
	require.Equal(t, 1, countRows(t, db,
		"SELECT count(*) FROM prompts WHERE id = $1 AND last_accessed_web_at IS NOT NULL", promptID),
		"fixture: the prompt must carry a last-accessed value before rolling back")
	require.Equal(t, 1, countRows(t, db,
		"SELECT count(*) FROM resource_freshness WHERE team_id = $1", teamID))

	// 4. Roll back.
	require.NoError(t, m.Migrate(12), "migrate down to 012")

	t.Run("down drops the tables", func(t *testing.T) {
		for _, table := range freshnessTables {
			assert.False(t, tableExists(t, db, table), "%s must be gone after rollback", table)
		}
	})

	t.Run("down drops populated columns and leaves the table usable", func(t *testing.T) {
		for _, table := range freshnessResourceTables {
			assert.Equal(t, 0, countLastAccessedColumns(t, db, table),
				"%s must lose its last_accessed columns", table)
		}
		// The surviving row still reads: the rollback dropped columns, not data.
		assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM prompts WHERE id = $1", promptID),
			"rolling back must not delete resource rows")
	})

	// 5. Re-apply: an operator who rolls back a release must be able to roll
	//    forward again.
	require.NoError(t, m.Migrate(13), "re-apply 013")
	for _, table := range freshnessTables {
		assert.True(t, tableExists(t, db, table), "%s must exist again after re-applying", table)
	}
	for _, table := range freshnessResourceTables {
		assert.Equal(t, 4, countLastAccessedColumns(t, db, table))
	}
	assert.Equal(t, 1, countRows(t, db,
		"SELECT count(*) FROM prompts WHERE id = $1 AND last_accessed_web_at IS NULL", promptID),
		"a re-added column starts NULL again -- the rollback discarded the value, as documented")
}

// seedMigration013Fixtures writes one row into every table 013 touches,
// returning the team and prompt ids.
func seedMigration013Fixtures(t *testing.T, db *sql.DB) (teamID, promptID string) {
	t.Helper()

	userID := uuid.New().String()
	teamID = uuid.New().String()
	projectID := uuid.New().String()
	promptID = uuid.New().String()
	ruleID := uuid.New().String()

	_, err := db.Exec(
		"INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
		userID, "freshness-"+userID[:8]+"@example.com", "Freshness User")
	require.NoError(t, err)

	_, err = db.Exec(
		"INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, $3, $4)",
		teamID, userID, "Freshness Team", "freshness-team-"+teamID[:8])
	require.NoError(t, err)

	_, err = db.Exec(
		"INSERT INTO projects (id, user_id, team_id, name, slug) VALUES ($1, $2, $3, $4, $5)",
		projectID, userID, teamID, "Freshness Project", "freshness-project-"+projectID[:8])
	require.NoError(t, err)

	_, err = db.Exec(
		"INSERT INTO prompts (id, user_id, team_id, project_id, name, slug, body, last_accessed_web_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, now())",
		promptID, userID, teamID, projectID, "Prompt", "prompt-"+promptID[:8], "body")
	require.NoError(t, err)

	_, err = db.Exec(
		"INSERT INTO freshness_rules (id, team_id, project_id, resource_types, mediums, threshold_days) "+
			"VALUES ($1, $2, $3, ARRAY['prompt'], ARRAY['web'], 90)",
		ruleID, teamID, projectID)
	require.NoError(t, err)

	_, err = db.Exec(
		"INSERT INTO team_freshness_settings (team_id, interval_seconds, reversibility_enabled) "+
			"VALUES ($1, 86400, true)", teamID)
	require.NoError(t, err)

	_, err = db.Exec(
		"INSERT INTO resource_freshness "+
			"(team_id, project_id, resource_type, resource_id, status, matched_rule_ids, since, reason) "+
			"VALUES ($1, $2, 'prompt', $3, 'stale', ARRAY[$4]::uuid[], now(), 'rule_run')",
		teamID, projectID, promptID, ruleID)
	require.NoError(t, err)

	_, err = db.Exec(
		"INSERT INTO resource_freshness_audit "+
			"(team_id, resource_type, resource_id, rule_id, action, reason) "+
			"VALUES ($1, 'prompt', $2, $3, 'marked', 'rule_run')",
		teamID, promptID, ruleID)
	require.NoError(t, err)

	return teamID, promptID
}
