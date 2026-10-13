//go:build integration

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Up/down round-trip for migration 026_resource_write_events_latest (#1418), on
// its OWN scratch database: this test migrates DOWN, which would corrupt the
// shared integrationDB.
//
// The migration adds two lookup indexes and no data. What the reads lean on is
// their exact shape: the latest-write lookup is index-only because source and
// assistant_name are INCLUDEd, and the unsourced-version index stays small
// because it is partial. That the planner uses the first one is asserted
// against the production join in TestIntegrationLastWrite_LookupIsServedByItsIndex.

const (
	migration026LatestIdx    = "idx_resource_write_events_resource_latest"
	migration026UnsourcedIdx = "idx_content_versions_team_created_unsourced"
)

func TestMigration026_ResourceWriteEventsLatest_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)
	indexDef := func(name string) string {
		var def string
		require.NoError(t, db.QueryRow(
			"SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1", name).Scan(&def))
		return def
	}

	require.NoError(t, m.Migrate(25), "migrate to 025")
	require.False(t, indexExists(t, db, migration026LatestIdx), "fixture: the index must not exist before 026")
	require.False(t, indexExists(t, db, migration026UnsourcedIdx))

	// Rows written before the migration: it must apply over a populated log.
	teamID := seedMigrationTeam(t, db)
	require.NoError(t, insertWriteEvent(db, teamID, nil))
	require.NoError(t, insertWriteEvent(db, teamID, map[string]any{"action": "update", "source": "web"}))

	require.NoError(t, m.Migrate(26), "migrate to 026")
	require.True(t, indexExists(t, db, migration026LatestIdx))
	require.True(t, indexExists(t, db, migration026UnsourcedIdx))

	latest := indexDef(migration026LatestIdx)
	assert.Contains(t, latest, "(resource_type, resource_id, created_at DESC)")
	assert.Contains(t, latest, "INCLUDE (source, assistant_name)")

	unsourced := indexDef(migration026UnsourcedIdx)
	assert.Contains(t, unsourced, "(team_id, created_at DESC)")
	assert.Contains(t, unsourced, "WHERE (source IS NULL)")

	require.NoError(t, m.Migrate(25), "migrate down to 025")
	assert.False(t, indexExists(t, db, migration026LatestIdx))
	assert.False(t, indexExists(t, db, migration026UnsourcedIdx))
	assert.Equal(t, 2, countRows(t, db, "SELECT count(*) FROM resource_write_events WHERE team_id = $1", teamID),
		"dropping the indexes loses no events")

	require.NoError(t, m.Migrate(26), "re-apply 026")
	assert.True(t, indexExists(t, db, migration026LatestIdx))
	assert.True(t, indexExists(t, db, migration026UnsourcedIdx))
}
