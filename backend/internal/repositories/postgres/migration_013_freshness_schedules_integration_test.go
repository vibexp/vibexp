//go:build integration

package postgres

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Rolling back 013_consolidated must remove every `freshness_evaluate` row from
// `schedules` (originally migration 015's down-block, #732).
//
// This is the one part of #732's migration that survived the post-v0.10.0
// squash. Its UP half -- seeding a schedule for teams whose freshness rules
// predated the write path -- was dropped, because on the consolidated chain
// `freshness_rules` is created empty by the same migration and the seeding
// SELECT could never match a row. There is therefore no longer any way to
// reach the state those tests set up, and they went with it.
//
// The DOWN half is still load-bearing and testable: the freshness service
// writes `freshness_evaluate` rows at runtime, and `schedules` belongs to 012
// and survives this rollback. Without the DELETE an operator who reverts a
// release is left with rows for a job whose handler no longer exists, which
// the scheduler can only log as unregistered on every tick.
//
// It runs on its OWN scratch database because it migrates DOWN, which would
// corrupt the shared integrationDB -- and, since that database is shared
// across worktrees, other checkouts too.

// seedMigrationTeam inserts a user and a team on a scratch database and returns
// the team id. It bypasses the repositories deliberately: this test is about
// SQL that runs before any Go code exists to call.
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

// countFreshnessSchedules reports how many freshness_evaluate rows a team has.
func countFreshnessSchedules(t *testing.T, db *sql.DB, teamID string) int {
	t.Helper()
	return countRows(t, db,
		`SELECT count(*) FROM schedules
		  WHERE team_id = $1 AND job_type = 'freshness_evaluate'`, teamID)
}

func TestMigration013_DownRemovesFreshnessSchedules(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(13), "migrate to 013")

	teamID := seedMigrationTeam(t, db)

	// A row exactly as the freshness service writes one at runtime.
	_, err := db.Exec(
		`INSERT INTO schedules (team_id, job_type, interval_seconds, next_run_at)
		 VALUES ($1, 'freshness_evaluate', 86400, now() + interval '1 hour')`, teamID)
	require.NoError(t, err)
	require.Equal(t, 1, countFreshnessSchedules(t, db, teamID), "fixture: the row must exist before rolling back")

	// A schedule for some other job must be left alone -- the DELETE is scoped
	// by job_type, and a rollback of the freshness feature has no business
	// touching another job's cadence.
	_, err = db.Exec(
		`INSERT INTO schedules (team_id, job_type, interval_seconds, next_run_at)
		 VALUES ($1, 'some_other_job', 3600, now() + interval '1 hour')`, teamID)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(12), "migrate down to 012")

	assert.Equal(t, 0, countFreshnessSchedules(t, db, teamID),
		"rolling back removes the job whose handler no longer exists")
	assert.True(t, tableExists(t, db, "schedules"),
		"the table itself belongs to 012 and must survive")
	assert.Equal(t, 1, countRows(t, db,
		`SELECT count(*) FROM schedules WHERE team_id = $1 AND job_type = 'some_other_job'`, teamID),
		"another job's schedule must be untouched")

	// Rolling forward again must leave the schema usable, and must NOT re-seed:
	// the consolidated migration carries no seeding SELECT any more, so the
	// service write path is the only thing that creates these rows.
	require.NoError(t, m.Migrate(13), "re-apply 013")
	assert.Equal(t, 0, countFreshnessSchedules(t, db, teamID),
		"013 seeds nothing -- only the service write path creates a freshness_evaluate row")
}
