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

// Up/down round-trip for migration 015_team_settings_audit (#828), on its OWN
// scratch database: this test migrates DOWN, which would corrupt the shared
// integrationDB (and, since that database is shared across worktrees, other
// checkouts too).
//
// Beyond "it applies", the properties that matter are that the down migration
// drops a POPULATED table cleanly — the acceptance criterion — and that going
// down and back up leaves the schema usable, so an operator who rolls back a
// release is not stranded.

// seedMigration015Fixtures writes an actor, two teams and one audit entry, so
// the rollback below runs against a populated table rather than an empty one.
func seedMigration015Fixtures(t *testing.T, db *sql.DB) (destTeamID string) {
	t.Helper()

	userID := uuid.New().String()
	_, err := db.Exec(
		"INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
		userID, "audit-"+userID[:8]+"@example.com", "Audit Fixture")
	require.NoError(t, err)

	destTeamID = uuid.New().String()
	sourceTeamID := uuid.New().String()
	for _, teamID := range []string{destTeamID, sourceTeamID} {
		_, err = db.Exec(
			"INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, $3, $4)",
			teamID, userID, "Team "+teamID[:8], "team-"+teamID[:8])
		require.NoError(t, err)
	}

	_, err = db.Exec(
		`INSERT INTO team_settings_audit
		     (team_id, actor_user_id, surface, source_team_id, source_resource_id,
		      created_resource_id, detail)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		destTeamID, userID, models.SettingsAuditSurfaceModelProvider, sourceTeamID,
		uuid.New().String(), uuid.New().String(), `{"name":"OpenAI (copy)"}`)
	require.NoError(t, err)

	return destTeamID
}

func TestMigration015_TeamSettingsAudit_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)

	// 1. The version immediately before this migration: nothing exists yet.
	require.NoError(t, m.Migrate(14), "migrate to 014")
	require.False(t, tableExists(t, db, "team_settings_audit"),
		"fixture: team_settings_audit must not exist before 015")

	// 2. Apply 015.
	require.NoError(t, m.Migrate(15), "migrate to 015")

	t.Run("creates the table, empty", func(t *testing.T) {
		require.True(t, tableExists(t, db, "team_settings_audit"))
		assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM team_settings_audit"),
			"015 backfills nothing")
	})

	t.Run("indexes the team's newest-first page", func(t *testing.T) {
		assert.Equal(t, 1, countRows(t, db,
			`SELECT count(*) FROM pg_indexes
			  WHERE schemaname = 'public' AND tablename = 'team_settings_audit'
			    AND indexname = 'idx_team_settings_audit_team_created'`),
			"the read endpoint (#832) pages on this index")
	})

	t.Run("detail defaults to an empty object", func(t *testing.T) {
		// Proves the column default rather than the repository's substitution:
		// an INSERT that omits detail entirely must still store an object.
		userID := uuid.New().String()
		_, err := db.Exec("INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
			userID, "default-"+userID[:8]+"@example.com", "Default Fixture")
		require.NoError(t, err)
		teamID := uuid.New().String()
		_, err = db.Exec("INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, $3, $4)",
			teamID, userID, "Team "+teamID[:8], "team-"+teamID[:8])
		require.NoError(t, err)

		_, err = db.Exec(
			"INSERT INTO team_settings_audit (team_id, surface) VALUES ($1, $2)",
			teamID, models.SettingsAuditSurfaceCustomTypes)
		require.NoError(t, err)

		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM team_settings_audit WHERE team_id = $1 AND detail = '{}'::jsonb",
			teamID))
	})

	// 3. Populate before rolling back, so `down` is exercised against a table
	//    that holds rows.
	destTeamID := seedMigration015Fixtures(t, db)
	require.Equal(t, 1, countRows(t, db,
		"SELECT count(*) FROM team_settings_audit WHERE team_id = $1", destTeamID),
		"fixture: the audit entry must exist before rolling back")

	// 4. Roll back.
	require.NoError(t, m.Migrate(14), "migrate down to 014")

	t.Run("down drops the populated table and leaves its FK targets intact", func(t *testing.T) {
		assert.False(t, tableExists(t, db, "team_settings_audit"),
			"team_settings_audit must be gone after rollback")
		assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM teams WHERE id = $1", destTeamID),
			"rolling back must not delete the teams the log referenced")
	})

	// 5. Re-apply: an operator who rolls back a release must be able to roll
	//    forward again.
	require.NoError(t, m.Migrate(15), "re-apply 015")
	assert.True(t, tableExists(t, db, "team_settings_audit"))
	assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM team_settings_audit"),
		"the re-applied table starts empty -- the dropped rows are gone for good")
}
