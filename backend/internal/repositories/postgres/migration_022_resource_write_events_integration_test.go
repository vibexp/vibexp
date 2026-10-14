//go:build integration

package postgres

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Up/down round-trip for the resource-write-events block of migration
// 022_consolidated (#1392; originally migration 022_resource_write_events), on
// its OWN scratch database: this test migrates DOWN, which would corrupt the
// shared integrationDB.
//
// Beyond "it applies", it pins what the rest of the feature leans on: a version
// written before the migration reads NULL for its source (unknown, never a
// guessed channel), every CHECK, the one-create-event-per-resource index, what
// happens to events when a team or a user is deleted, and that the down
// migration drops a POPULATED table.

const (
	migrationWriteEventsTable     = "resource_write_events"
	migrationWriteEventsCreateIdx = "idx_resource_write_events_create"
	migrationWriteEventsTeamIdx   = "idx_resource_write_events_team_created"
)

// insertWriteEvent inserts one event with the given overrides and returns the
// insert error, so a test can assert which constraint refused it.
func insertWriteEvent(db *sql.DB, teamID string, overrides map[string]any) error {
	row := map[string]any{
		"team_id":       teamID,
		"resource_type": "memory",
		"resource_id":   uuid.New().String(),
		"action":        "create",
		"source":        "mcp",
	}
	for column, value := range overrides {
		row[column] = value
	}

	columns := make([]string, 0, len(row))
	placeholders := make([]string, 0, len(row))
	args := make([]any, 0, len(row))
	for column, value := range row {
		columns = append(columns, column)
		args = append(args, value)
		placeholders = append(placeholders, "$"+string(rune('0'+len(args))))
	}
	_, err := db.Exec(
		"INSERT INTO "+migrationWriteEventsTable+" ("+strings.Join(columns, ", ")+") VALUES ("+
			strings.Join(placeholders, ", ")+")", args...)
	return err
}

func TestMigration022_ResourceWriteEvents_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)

	// 1. The version immediately before this migration, with a content version
	//    already written: the row every existing instance has.
	require.NoError(t, m.Migrate(21), "migrate to 021")
	require.False(t, tableExists(t, db, migrationWriteEventsTable), "fixture: the table must not exist before 022")
	require.False(t, columnExists(t, db, "content_versions", "source"))

	teamID := seedMigrationTeam(t, db)
	legacyResource := uuid.New().String()
	_, err := db.Exec(
		`INSERT INTO content_versions (team_id, resource_type, resource_id, version_number, content, actor_type)
		 VALUES ($1, 'artifact', $2, 1, 'written before 022', 'human')`, teamID, legacyResource)
	require.NoError(t, err)

	// 2. Apply 022.
	require.NoError(t, m.Migrate(22), "migrate to 022")
	require.True(t, tableExists(t, db, migrationWriteEventsTable))
	require.True(t, indexExists(t, db, migrationWriteEventsCreateIdx))
	require.True(t, indexExists(t, db, migrationWriteEventsTeamIdx))

	t.Run("a version written before the migration has no source", func(t *testing.T) {
		assert.Equal(t, 1, countRows(t, db,
			`SELECT count(*) FROM content_versions
			  WHERE resource_id = $1 AND source IS NULL AND assistant_name IS NULL`, legacyResource),
			"an old version must read as unknown, never as a backfilled channel")
	})

	t.Run("content_versions.source accepts the four channels and nothing else", func(t *testing.T) {
		insert := func(source any) error {
			_, insertErr := db.Exec(
				`INSERT INTO content_versions
				   (team_id, resource_type, resource_id, version_number, content, actor_type, source)
				 VALUES ($1, 'artifact', $2, 1, 'c', 'human', $3)`, teamID, uuid.New().String(), source)
			return insertErr
		}
		for _, source := range []string{"web", "cli", "mcp", "api"} {
			require.NoError(t, insert(source), source)
		}
		require.NoError(t, insert(nil), "NULL stays allowed")
		requirePQCode(t, insert("unknown"), pqCheckViolation)
		requirePQCode(t, insert("browser"), pqCheckViolation)
	})

	t.Run("event CHECKs", func(t *testing.T) {
		require.NoError(t, insertWriteEvent(db, teamID, nil))
		for _, source := range []string{"web", "cli", "api"} {
			require.NoError(t, insertWriteEvent(db, teamID, map[string]any{"source": source}), source)
		}
		for _, resourceType := range []string{"prompt", "artifact", "blueprint"} {
			require.NoError(t, insertWriteEvent(db, teamID, map[string]any{"resource_type": resourceType}), resourceType)
		}
		require.NoError(t, insertWriteEvent(db, teamID, map[string]any{"action": "update"}))

		// "unknown" is how the metrics report a MISSING event. It must never be
		// storable as an event's source.
		requirePQCode(t, insertWriteEvent(db, teamID, map[string]any{"source": "unknown"}), pqCheckViolation)
		requirePQCode(t, insertWriteEvent(db, teamID, map[string]any{"source": "browser"}), pqCheckViolation)
		requirePQCode(t, insertWriteEvent(db, teamID, map[string]any{"action": "delete"}), pqCheckViolation)
		requirePQCode(t, insertWriteEvent(db, teamID, map[string]any{"resource_type": "project"}), pqCheckViolation)
	})

	t.Run("assistant_name is capped where the Go constant says", func(t *testing.T) {
		atCap := strings.Repeat("é", models.MaxAssistantNameLength)
		require.NoError(t, insertWriteEvent(db, teamID, map[string]any{"assistant_name": atCap}),
			"the cap counts characters, so 30 multi-byte runes fit")
		requirePQCode(t, insertWriteEvent(db, teamID, map[string]any{"assistant_name": atCap + "x"}), "22001")
	})

	t.Run("one create event per resource, any number of updates", func(t *testing.T) {
		resourceID := uuid.New().String()
		require.NoError(t, insertWriteEvent(db, teamID, map[string]any{"resource_id": resourceID}))
		requirePQCode(t, insertWriteEvent(db, teamID, map[string]any{"resource_id": resourceID}), "23505")
		for range 2 {
			require.NoError(t, insertWriteEvent(db, teamID, map[string]any{
				"resource_id": resourceID, "action": "update",
			}))
		}
		// The same id under another resource type is a different resource.
		require.NoError(t, insertWriteEvent(db, teamID, map[string]any{
			"resource_id": resourceID, "resource_type": "prompt",
		}))
	})

	t.Run("deleting the user keeps the event, deleting the team removes it", func(t *testing.T) {
		userID := uuid.New().String()
		_, err := db.Exec("INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
			userID, "writer-"+userID[:8]+"@example.com", "Writer")
		require.NoError(t, err)
		ownTeam := uuid.New().String()
		_, err = db.Exec("INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, $3, $4)",
			ownTeam, userID, "Team "+ownTeam[:8], "team-"+ownTeam[:8])
		require.NoError(t, err)

		writer := uuid.New().String()
		_, err = db.Exec("INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
			writer, "member-"+writer[:8]+"@example.com", "Member")
		require.NoError(t, err)
		require.NoError(t, insertWriteEvent(db, ownTeam, map[string]any{"user_id": writer}))

		_, err = db.Exec("DELETE FROM users WHERE id = $1", writer)
		require.NoError(t, err)
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM "+migrationWriteEventsTable+" WHERE team_id = $1 AND user_id IS NULL", ownTeam),
			"the event outlives its author, with the author blanked")

		_, err = db.Exec("DELETE FROM teams WHERE id = $1", ownTeam)
		require.NoError(t, err)
		assert.Equal(t, 0, countRows(t, db,
			"SELECT count(*) FROM "+migrationWriteEventsTable+" WHERE team_id = $1", ownTeam))
	})

	// 3. Roll back with the table populated and sourced versions present.
	require.Positive(t, countRows(t, db, "SELECT count(*) FROM "+migrationWriteEventsTable),
		"fixture: the table must hold rows before rolling back")
	require.NoError(t, m.Migrate(21), "migrate down to 021")

	assert.False(t, tableExists(t, db, migrationWriteEventsTable))
	assert.False(t, columnExists(t, db, "content_versions", "source"))
	assert.False(t, columnExists(t, db, "content_versions", "assistant_name"))
	assert.Equal(t, 0, constraintCount(t, db, "content_versions_source_check"))
	assert.Equal(t, 1, countRows(t, db,
		"SELECT count(*) FROM content_versions WHERE resource_id = $1", legacyResource),
		"the versions themselves survive the rollback")

	// 4. And forward again, so a re-applied migration is proven too.
	require.NoError(t, m.Migrate(22), "re-apply 022")
	assert.True(t, tableExists(t, db, migrationWriteEventsTable))
	assert.True(t, columnExists(t, db, "content_versions", "source"))
}
