//go:build integration

package postgres

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 015 seeds `freshness_evaluate` schedules for teams whose freshness
// rules predate #732 (nothing else can: only the service write path creates the
// row, and those teams may never write again). It runs on its OWN scratch
// database because it migrates DOWN, which would corrupt the shared
// integrationDB — and, since that database is shared across worktrees, other
// checkouts too.
//
// A data migration's whole content is a SELECT, so "it applies" proves nothing;
// what has to hold is WHICH teams get a row, which interval they get, and that
// re-running cannot disturb a row that already exists.

// seedMigrationTeam inserts a user, team and project on a scratch database and
// returns the team id. It bypasses the repositories deliberately: this test is
// about SQL that runs before any Go code exists to call.
func seedMigrationTeam(t *testing.T, db *sql.DB) string {
	t.Helper()
	userID, teamID := uuid.New().String(), uuid.New().String()

	_, err := db.Exec(
		"INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
		userID, "u-"+userID[:8]+"@example.com", "User "+userID[:8])
	require.NoError(t, err)

	_, err = db.Exec(
		"INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, $3, $4)",
		teamID, userID, "Team "+teamID[:8], "team-"+teamID[:8])
	require.NoError(t, err)

	return teamID
}

func insertMigrationRule(t *testing.T, db *sql.DB, teamID string, enabled bool) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO freshness_rules (team_id, resource_types, threshold_days, enabled)
		 VALUES ($1, ARRAY['prompt'], 30, $2)`, teamID, enabled)
	require.NoError(t, err)
}

// scheduleInterval returns the seeded interval for a team, or -1 when it has no
// freshness_evaluate row.
func scheduleInterval(t *testing.T, db *sql.DB, teamID string) int {
	t.Helper()
	var interval int
	err := db.QueryRow(
		`SELECT interval_seconds FROM schedules
		  WHERE team_id = $1 AND job_type = 'freshness_evaluate'`, teamID).Scan(&interval)
	if err == sql.ErrNoRows {
		return -1
	}
	require.NoError(t, err)
	return interval
}

func TestMigration015_SeedsFreshnessSchedulesForExistingRules(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(14), "migrate to 014")

	// Four teams covering every branch of the seeding SELECT.
	withDefaults := seedMigrationTeam(t, db)
	withSettings := seedMigrationTeam(t, db)
	disabledOnly := seedMigrationTeam(t, db)
	noRules := seedMigrationTeam(t, db)

	insertMigrationRule(t, db, withDefaults, true)
	// Two rules, to prove DISTINCT is what stops the unique index rejecting the
	// insert rather than luck.
	insertMigrationRule(t, db, withDefaults, true)
	insertMigrationRule(t, db, withSettings, true)
	insertMigrationRule(t, db, disabledOnly, false)

	_, err := db.Exec(
		`INSERT INTO team_freshness_settings (team_id, interval_seconds, reversibility_enabled)
		 VALUES ($1, 7200, true)`, withSettings)
	require.NoError(t, err)

	require.Equal(t, 0, countRows(t, db,
		"SELECT count(*) FROM schedules WHERE job_type = 'freshness_evaluate'"),
		"fixture: no freshness schedules before 015")

	require.NoError(t, m.Migrate(15), "migrate to 015")

	assert.Equal(t, 86400, scheduleInterval(t, db, withDefaults),
		"a team with no stored settings inherits the documented daily default")
	assert.Equal(t, 7200, scheduleInterval(t, db, withSettings),
		"a team with stored settings gets its own interval")
	assert.Equal(t, 86400, scheduleInterval(t, db, disabledOnly),
		"a disabled rule set must still be evaluated, so its stale state gets cleared")
	assert.Equal(t, -1, scheduleInterval(t, db, noRules),
		"a team with no rules has nothing to evaluate")
}

// The seed must never disturb a row the service already wrote — otherwise
// re-running migrations (or a restore) would reset a team's cadence.
func TestMigration015_LeavesAnExistingScheduleUntouched(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(14), "migrate to 014")

	teamID := seedMigrationTeam(t, db)
	insertMigrationRule(t, db, teamID, true)

	// A row as the service would have written it, deliberately not due yet.
	_, err := db.Exec(
		`INSERT INTO schedules (team_id, job_type, interval_seconds, next_run_at)
		 VALUES ($1, 'freshness_evaluate', 3600, now() + interval '30 minutes')`, teamID)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(15), "migrate to 015")

	assert.Equal(t, 3600, scheduleInterval(t, db, teamID), "the existing interval must survive")
	assert.Equal(t, 1, countRows(t, db,
		`SELECT count(*) FROM schedules
		  WHERE team_id = $1 AND job_type = 'freshness_evaluate'
		    AND next_run_at > now()`, teamID),
		"next_run_at must not be reset to now()")
}

// Rolling back removes the schedules, and going back up re-seeds them: an
// operator who reverts a release must not be stranded with rows for a job whose
// handler no longer exists.
func TestMigration015_DownUpRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(14), "migrate to 014")

	teamID := seedMigrationTeam(t, db)
	insertMigrationRule(t, db, teamID, true)

	require.NoError(t, m.Migrate(15), "migrate to 015")
	require.Equal(t, 86400, scheduleInterval(t, db, teamID))

	require.NoError(t, m.Migrate(14), "migrate back down to 014")
	assert.Equal(t, -1, scheduleInterval(t, db, teamID),
		"rolling back removes the job the handler no longer serves")
	assert.True(t, tableExists(t, db, "schedules"), "the table itself belongs to 012 and must survive")

	require.NoError(t, m.Migrate(15), "migrate back up to 015")
	assert.Equal(t, 86400, scheduleInterval(t, db, teamID))
}
