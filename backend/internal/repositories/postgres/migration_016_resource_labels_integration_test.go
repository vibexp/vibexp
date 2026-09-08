//go:build integration

package postgres

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 016_resource_labels (#910, epic #899), on its OWN scratch database:
// this test migrates DOWN, which would corrupt the shared integrationDB (and,
// since that database is shared across worktrees, other checkouts too).
//
// The property that matters is the memory backfill. `metadata.tags` was a pure
// frontend convention, so the values in the wild are unvalidated: the migration
// has to survive a scalar or an object parked at that key (a bare
// jsonb_array_elements_text would abort the WHOLE migration on one such row) and
// must not land a value the API's own limits would reject on the next edit.

type migration016Fixtures struct {
	teamID       string
	projectID    string
	withTags     string
	emptyTags    string
	noTags       string
	scalarTags   string
	objectTags   string
	overflowTags string
	messyTags    string
	backdated    string
}

func seedMigration016Fixtures(t *testing.T, db *sql.DB) migration016Fixtures {
	t.Helper()

	userID := uuid.New().String()
	_, err := db.Exec("INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
		userID, "labels-"+userID[:8]+"@example.com", "Labels Fixture")
	require.NoError(t, err)

	fx := migration016Fixtures{teamID: uuid.New().String(), projectID: uuid.New().String()}
	_, err = db.Exec("INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, $3, $4)",
		fx.teamID, userID, "Team "+fx.teamID[:8], "team-"+fx.teamID[:8])
	require.NoError(t, err)
	_, err = db.Exec(
		"INSERT INTO projects (id, user_id, team_id, name, slug) VALUES ($1, $2, $3, $4, $5)",
		fx.projectID, userID, fx.teamID, "Project", "project-"+fx.projectID[:8])
	require.NoError(t, err)

	insert := func(metadata string) string {
		id := uuid.New().String()
		_, insErr := db.Exec(
			`INSERT INTO memories (id, user_id, team_id, project_id, text, status, metadata)
			 VALUES ($1, $2, $3, $4, $5, 'active', $6::jsonb)`,
			id, userID, fx.teamID, fx.projectID, "memory "+id[:8], metadata)
		require.NoError(t, insErr)
		return id
	}

	fx.withTags = insert(`{"tags": ["onboarding", "api"], "priority": "high"}`)
	fx.emptyTags = insert(`{"tags": []}`)
	fx.noTags = insert(`{"priority": "low"}`)
	fx.scalarTags = insert(`{"tags": "not-an-array"}`)
	fx.objectTags = insert(`{"tags": {"nested": true}}`)
	// 12 tags, the THIRD of them 60 characters: neither shape can be produced
	// through the API, but `metadata.tags` was never validated, so both exist in
	// the wild. The long one is placed inside the first ten deliberately -- at
	// the tail it would be dropped by the count cap and the length cap would
	// never be exercised.
	fx.overflowTags = insert(
		`{"tags": ["t1","t2",` +
			`"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",` +
			`"t4","t5","t6","t7","t8","t9","t10","t11","t12"]}`)
	// Untrimmed and duplicated: the write path trims and de-duplicates, so a
	// backfill that did not would land values it could never produce -- and an
	// untrimmed label matches no `?labels=` filter, because the query side IS
	// trimmed.
	fx.messyTags = insert(`{"tags": ["  api ", "api", "onboarding ", "  "]}`)
	fx.backdated = insert(`{"tags": ["stale-check"]}`)

	// updated_at is an EDIT signal: search recency ranking and resource freshness
	// both read it. Backdate one tagged row so the assertion below can prove the
	// backfill did not rewrite it. The unconditional update_memories_updated_at
	// trigger would overwrite this, so suspend it for the one statement --
	// exactly what the migration itself has to do.
	_, err = db.Exec("ALTER TABLE memories DISABLE TRIGGER update_memories_updated_at")
	require.NoError(t, err)
	_, err = db.Exec(
		"UPDATE memories SET updated_at = TIMESTAMP '2020-01-01 00:00:00' WHERE id = $1", fx.backdated)
	require.NoError(t, err)
	_, err = db.Exec("ALTER TABLE memories ENABLE TRIGGER update_memories_updated_at")
	require.NoError(t, err)

	return fx
}

func memoryLabels(t *testing.T, db *sql.DB, id string) []string {
	t.Helper()
	var labels pq.StringArray
	require.NoError(t, db.QueryRow("SELECT labels FROM memories WHERE id = $1", id).Scan(&labels))
	return labels
}

func TestMigration016_ResourceLabels(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)

	// 1. The version immediately before this migration.
	require.NoError(t, m.Migrate(15), "migrate to 015")
	for _, table := range []string{"artifacts", "blueprints", "memories"} {
		require.False(t, columnExists(t, db, table, "labels"),
			"fixture: %s.labels must not exist before 016", table)
	}

	// 2. Seed the pre-migration state.
	fx := seedMigration016Fixtures(t, db)

	// 3. Apply 016.
	require.NoError(t, m.Migrate(16), "migrate to 016")

	t.Run("adds the column and its GIN index to all three tables", func(t *testing.T) {
		for table, index := range map[string]string{
			"artifacts":  "idx_artifacts_labels",
			"blueprints": "idx_blueprints_labels",
			"memories":   "idx_memories_labels",
		} {
			assert.True(t, columnExists(t, db, table, "labels"), table)
			assert.True(t, indexExists(t, db, index), index)
		}
	})

	t.Run("existing rows default to an empty array, never NULL", func(t *testing.T) {
		assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM memories WHERE labels IS NULL"))
		assert.Empty(t, memoryLabels(t, db, fx.noTags))
	})

	t.Run("metadata.tags becomes labels and the key is gone", func(t *testing.T) {
		assert.Equal(t, []string{"onboarding", "api"}, memoryLabels(t, db, fx.withTags))
		assert.Equal(t, 0, countRows(t, db,
			"SELECT count(*) FROM memories WHERE id = $1 AND metadata ? 'tags'", fx.withTags))
		assert.Equal(t, 1, countRows(t, db,
			`SELECT count(*) FROM memories WHERE id = $1 AND metadata->>'priority' = 'high'`, fx.withTags),
			"other metadata keys survive")
	})

	t.Run("an empty tags array yields no labels and still drops the key", func(t *testing.T) {
		assert.Empty(t, memoryLabels(t, db, fx.emptyTags))
		assert.Equal(t, 0, countRows(t, db,
			"SELECT count(*) FROM memories WHERE id = $1 AND metadata ? 'tags'", fx.emptyTags))
	})

	// The jsonb_typeof guard: without it, jsonb_array_elements_text over a scalar
	// or an object raises and aborts the ENTIRE migration, not just this row.
	t.Run("a non-array tags value is left exactly where it is", func(t *testing.T) {
		for _, id := range []string{fx.scalarTags, fx.objectTags} {
			assert.Empty(t, memoryLabels(t, db, id))
			assert.Equal(t, 1, countRows(t, db,
				"SELECT count(*) FROM memories WHERE id = $1 AND metadata ? 'tags'", id),
				"an ordinary metadata entry that happens to be called tags is not taxonomy")
		}
	})

	t.Run("the backfill respects the API's own 10 x 50 limits", func(t *testing.T) {
		labels := memoryLabels(t, db, fx.overflowTags)
		assert.Len(t, labels, 10, "a longer legacy list must not land a row the API would reject")
		for _, label := range labels {
			assert.LessOrEqual(t, len([]rune(label)), 50)
		}
		assert.Equal(t, []string{
			"t1", "t2", "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			"t4", "t5", "t6", "t7", "t8", "t9", "t10",
		}, []string(labels), "the first ten in order, each truncated to 50 characters")
	})

	// The backfill is an UPDATE on the one table carrying an unconditional
	// updated_at trigger. Left enabled, it would mark every migrated memory as
	// edited just now, corrupting search recency ranking and resource freshness.
	t.Run("the backfill normalises exactly as the write path does", func(t *testing.T) {
		assert.Equal(t, []string{"api", "onboarding"}, memoryLabels(t, db, fx.messyTags),
			"trimmed, de-duplicated on the first occurrence, empties dropped")
	})

	t.Run("the backfill does not look like an edit", func(t *testing.T) {
		var updatedAt string
		require.NoError(t, db.QueryRow(
			"SELECT to_char(updated_at, 'YYYY-MM-DD') FROM memories WHERE id = $1", fx.backdated).Scan(&updatedAt))
		assert.Equal(t, "2020-01-01", updatedAt,
			"update_memories_updated_at must be suspended for the backfill statement")
	})

	t.Run("down restores metadata.tags and re-runs clean", func(t *testing.T) {
		require.NoError(t, m.Migrate(15), "migrate down to 015")

		for _, table := range []string{"artifacts", "blueprints", "memories"} {
			assert.False(t, columnExists(t, db, table, "labels"), table)
		}
		assert.Equal(t, 1, countRows(t, db,
			`SELECT count(*) FROM memories
			  WHERE id = $1 AND metadata->'tags' = '["onboarding","api"]'::jsonb`, fx.withTags),
			"rolling back must not lose the taxonomy the up migration moved")

		require.NoError(t, m.Migrate(16), "migrate back up to 016")
		assert.Equal(t, []string{"onboarding", "api"}, memoryLabels(t, db, fx.withTags))
	})
}

// TestLabelsRoundTripAndFilter proves the column is actually usable end to end:
// a label written through the repository comes back, and the `labels` filter
// narrows the PAGE and the TOTAL together. The count and page queries are built
// from separately hard-coded FROM clauses sharing only the where-clause builder,
// so a predicate that reached only one of them would return a short page
// describing an unfiltered total -- which an assertion about the page alone
// never notices.
func TestMigration016_LabelsFilterNarrowsPageAndTotal(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)
	require.NoError(t, m.Up(), "migrate up")

	fx := seedMigration016Fixtures(t, db)
	_, err := db.Exec(
		"UPDATE memories SET labels = $1 WHERE id = $2", pq.StringArray{"alpha", "beta"}, fx.noTags)
	require.NoError(t, err)

	assert.Equal(t, 1, countRows(t, db,
		"SELECT count(*) FROM memories WHERE team_id = $1 AND labels && $2",
		fx.teamID, pq.StringArray{"alpha"}))
	assert.Equal(t, 1, countRows(t, db,
		"SELECT count(*) FROM memories WHERE team_id = $1 AND labels && $2",
		fx.teamID, pq.StringArray{"alpha", "gamma"}),
		"overlap matches ANY of the requested labels")
	assert.Equal(t, 0, countRows(t, db,
		"SELECT count(*) FROM memories WHERE team_id = $1 AND labels && $2",
		fx.teamID, pq.StringArray{"gamma"}))
}
