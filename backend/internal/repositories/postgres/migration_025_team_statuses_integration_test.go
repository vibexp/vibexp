//go:build integration

package postgres

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Up/down round-trip for migration 025_team_statuses (#1264), on its OWN
// scratch database: this test migrates DOWN, which would corrupt the shared
// integrationDB.
//
// It pins that existing rows come through the up unchanged (status AND
// updated_at), that a custom status becomes storable on all four resource
// tables, and that the documented, lossy down reassigns exactly the rows the
// pre-025 schema cannot hold and nothing else.

// migration025Tables maps each resource table to its status resource type.
var migration025Tables = map[string]string{
	"prompts":    models.StatusResourceTypePrompt,
	"artifacts":  models.StatusResourceTypeArtifact,
	"blueprints": models.StatusResourceTypeBlueprint,
	"memories":   models.StatusResourceTypeMemory,
}

type migration025Fixture struct {
	db                        *sql.DB
	userID, teamID, projectID string
}

func newMigration025Fixture(t *testing.T, db *sql.DB) migration025Fixture {
	t.Helper()
	userID := refScopeInsertUser(t, db)
	teamID := refScopeInsertTeam(t, db, userID)
	return migration025Fixture{
		db: db, userID: userID, teamID: teamID,
		projectID: refScopeInsertProject(t, db, userID, teamID),
	}
}

// insert adds one resource row carrying status to table and returns its id.
func (f migration025Fixture) insert(t *testing.T, table, status string) string {
	t.Helper()
	id := uuid.New().String()
	var err error
	switch table {
	case "prompts":
		_, err = f.db.Exec(
			`INSERT INTO prompts (id, user_id, team_id, project_id, name, slug, body, status)
			 VALUES ($1, $2, $3, $4, $5, $6, 'body', $7)`,
			id, f.userID, f.teamID, f.projectID, "P "+id[:8], "p-"+id[:8], status)
	case "artifacts":
		_, err = f.db.Exec(
			`INSERT INTO artifacts (id, user_id, team_id, project_id, slug, title, content, status)
			 VALUES ($1, $2, $3, $4, $5, $6, 'content', $7)`,
			id, f.userID, f.teamID, f.projectID, "a-"+id[:8], "A "+id[:8], status)
	case "blueprints":
		_, err = f.db.Exec(
			`INSERT INTO blueprints (id, user_id, team_id, project_id, slug, title, content, path, status)
			 VALUES ($1, $2, $3, $4, $5, $6, 'content', $7, $8)`,
			id, f.userID, f.teamID, f.projectID, "b-"+id[:8], "B "+id[:8], "rules/"+id[:8]+".md", status)
	case "memories":
		_, err = f.db.Exec(
			`INSERT INTO memories (id, user_id, team_id, project_id, text, status)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			id, f.userID, f.teamID, f.projectID, "memory "+id[:8], status)
	default:
		t.Fatalf("unknown table %q", table)
	}
	require.NoError(t, err, "insert into %s with status %q", table, status)
	return id
}

// snapshot is a row's status and updated_at, the two things 025 must not
// disturb on a row it has no reason to touch.
type migration025Snapshot struct {
	status    string
	updatedAt time.Time
}

func (f migration025Fixture) snapshot(t *testing.T, table, id string) migration025Snapshot {
	t.Helper()
	var snap migration025Snapshot
	require.NoError(t, f.db.QueryRow(
		"SELECT status, updated_at FROM "+table+" WHERE id = $1", id,
	).Scan(&snap.status, &snap.updatedAt))
	return snap
}

func statusColumnLength(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	return countRows(t, db,
		`SELECT character_maximum_length FROM information_schema.columns
		  WHERE table_schema = 'public' AND table_name = $1 AND column_name = 'status'`, table)
}

func constraintCount(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	return countRows(t, db, "SELECT count(*) FROM pg_constraint WHERE conname = $1", name)
}

func TestMigration025_TeamStatuses_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)

	// 1. The version immediately before this migration, populated with one row
	//    per system status of every resource type.
	require.NoError(t, m.Migrate(24), "migrate to 024")
	require.False(t, tableExists(t, db, "team_statuses"), "fixture: team_statuses must not exist before 025")

	f := newMigration025Fixture(t, db)
	type seeded struct{ table, id string }
	var existing []seeded
	for table, resourceType := range migration025Tables {
		require.Equal(t, 20, statusColumnLength(t, db, table), "fixture: %s.status is varchar(20) before 025", table)
		for _, status := range models.SystemStatusesFor(resourceType) {
			existing = append(existing, seeded{table, f.insert(t, table, status)})
		}
	}
	before := make(map[string]migration025Snapshot, len(existing))
	for _, row := range existing {
		before[row.id] = f.snapshot(t, row.table, row.id)
	}
	require.Equal(t, 1, constraintCount(t, db, "prompts_status_check"), "fixture")
	require.Equal(t, 1, constraintCount(t, db, "memories_status_check"), "fixture")

	// 2. Apply 025.
	require.NoError(t, m.Migrate(25), "migrate to 025")

	t.Run("leaves existing rows unchanged", func(t *testing.T) {
		for _, row := range existing {
			assert.Equal(t, before[row.id], f.snapshot(t, row.table, row.id), "%s %s", row.table, row.id)
		}
	})

	t.Run("creates team_statuses, empty", func(t *testing.T) {
		require.True(t, tableExists(t, db, "team_statuses"))
		assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM team_statuses"))
	})

	t.Run("drops both status CHECKs and widens all four columns", func(t *testing.T) {
		assert.Equal(t, 0, constraintCount(t, db, "prompts_status_check"))
		assert.Equal(t, 0, constraintCount(t, db, "memories_status_check"))
		for table := range migration025Tables {
			assert.Equal(t, 50, statusColumnLength(t, db, table), table)
		}
	})

	t.Run("keeps the status indexes", func(t *testing.T) {
		assert.Equal(t, 3, countRows(t, db,
			`SELECT count(*) FROM pg_indexes
			  WHERE schemaname = 'public'
			    AND indexname IN ('idx_artifacts_status', 'idx_blueprints_status', 'idx_prompts_status')`))
	})

	t.Run("team_statuses is unique per team, resource type and slug", func(t *testing.T) {
		insert := "INSERT INTO team_statuses (team_id, resource_type, slug, name) VALUES ($1, $2, $3, $4)"
		_, err := db.Exec(insert, f.teamID, models.StatusResourceTypeArtifact, "in-review", "In review")
		require.NoError(t, err)
		_, err = db.Exec(insert, f.teamID, models.StatusResourceTypeArtifact, "in-review", "Again")
		requirePQCode(t, err, "23505")
		_, err = db.Exec(insert, f.teamID, models.StatusResourceTypeMemory, "in-review", "In review")
		require.NoError(t, err, "the same slug is free on another resource type")
	})

	t.Run("team_statuses rejects an unknown team and an unknown creator", func(t *testing.T) {
		_, err := db.Exec(
			"INSERT INTO team_statuses (team_id, resource_type, slug, name) VALUES ($1, 'artifact', 'x', 'X')",
			uuid.New().String())
		requirePQCode(t, err, "23503")
		_, err = db.Exec(
			`INSERT INTO team_statuses (team_id, resource_type, slug, name, created_by)
			 VALUES ($1, 'artifact', 'y', 'Y', $2)`, f.teamID, uuid.New().String())
		requirePQCode(t, err, "23503")
	})

	// 3. What 025 exists for: a custom status, up to 50 characters, on every
	//    resource table -- including the two that had a CHECK.
	longSlug := strings.Repeat("s", 50)
	custom := make(map[string]string, len(migration025Tables))
	for table := range migration025Tables {
		custom[table] = f.insert(t, table, longSlug)
	}
	for table := range migration025Tables {
		_, err := db.Exec("UPDATE "+table+" SET status = $1 WHERE id = $2",
			strings.Repeat("s", 51), custom[table])
		require.Error(t, err, "%s.status must still be capped at 50", table)
	}
	// Backdate the custom memory so the down can be shown not to fake an edit.
	// The memories trigger rewrites updated_at on any UPDATE, so suspend it
	// for the one statement, exactly as the migration has to.
	backdated := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	_, err := db.Exec("ALTER TABLE memories DISABLE TRIGGER update_memories_updated_at")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE memories SET updated_at = $1 WHERE id = $2", backdated, custom["memories"])
	require.NoError(t, err)
	_, err = db.Exec("ALTER TABLE memories ENABLE TRIGGER update_memories_updated_at")
	require.NoError(t, err)
	customMemoryBefore := f.snapshot(t, "memories", custom["memories"])

	// 4. Roll back a populated schema.
	require.NoError(t, m.Migrate(24), "migrate down to 024")

	t.Run("down drops the table and restores the CHECKs and the width", func(t *testing.T) {
		assert.False(t, tableExists(t, db, "team_statuses"))
		assert.Equal(t, 1, constraintCount(t, db, "prompts_status_check"))
		assert.Equal(t, 1, constraintCount(t, db, "memories_status_check"))
		for table := range migration025Tables {
			assert.Equal(t, 20, statusColumnLength(t, db, table), table)
		}
	})

	t.Run("down leaves rows on a system status unchanged", func(t *testing.T) {
		for _, row := range existing {
			assert.Equal(t, before[row.id], f.snapshot(t, row.table, row.id), "%s %s", row.table, row.id)
		}
	})

	t.Run("down reassigns a custom status to the type's default", func(t *testing.T) {
		for table, resourceType := range migration025Tables {
			assert.Equal(t, models.DefaultStatusFor(resourceType),
				f.snapshot(t, table, custom[table]).status, table)
		}
	})

	t.Run("down does not fake an edit on the reassigned memory", func(t *testing.T) {
		assert.Equal(t, customMemoryBefore.updatedAt,
			f.snapshot(t, "memories", custom["memories"]).updatedAt)
		assert.Equal(t, 1, countRows(t, db,
			`SELECT count(*) FROM pg_trigger
			  WHERE tgname = 'update_memories_updated_at' AND tgenabled = 'O'`),
			"the trigger must be enabled again after the down")
	})

	t.Run("the restored CHECKs enforce the system sets again", func(t *testing.T) {
		_, insErr := db.Exec(
			`INSERT INTO prompts (id, user_id, team_id, project_id, name, slug, body, status)
			 VALUES ($1, $2, $3, $4, 'P', $5, 'body', 'active')`,
			uuid.New().String(), f.userID, f.teamID, f.projectID, "p-"+uuid.New().String()[:8])
		requirePQCode(t, insErr, pqCheckViolation)
		_, insErr = db.Exec(
			`INSERT INTO memories (id, user_id, team_id, project_id, text, status)
			 VALUES ($1, $2, $3, $4, 'm', 'published')`,
			uuid.New().String(), f.userID, f.teamID, f.projectID)
		requirePQCode(t, insErr, pqCheckViolation)
	})

	// 5. Re-apply: an operator who rolls back a release must be able to roll
	//    forward again.
	require.NoError(t, m.Migrate(25), "re-apply 025")
	assert.True(t, tableExists(t, db, "team_statuses"))
	assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM team_statuses"),
		"the re-applied table starts empty -- the dropped rows are gone for good")
}
