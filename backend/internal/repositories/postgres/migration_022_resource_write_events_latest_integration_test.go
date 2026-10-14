//go:build integration

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Up/down round-trip for the resource-write-events-latest block of migration
// 022_consolidated (#1418; originally migration
// 026_resource_write_events_latest), on its OWN scratch database: this test
// migrates DOWN, which would corrupt the shared integrationDB.
//
// The block adds two lookup indexes and no data. What the reads lean on is
// their exact shape: the latest-write lookup is index-only because source and
// assistant_name are INCLUDEd, and the unsourced-version index stays small
// because it is partial. That the planner uses the first one is asserted
// against the production join in TestIntegrationLastWrite_LookupIsServedByItsIndex.

const (
	migrationWriteEventsLatestIdx    = "idx_resource_write_events_resource_latest"
	migrationWriteEventsUnsourcedIdx = "idx_content_versions_team_created_unsourced"
)

func TestMigration022_ResourceWriteEventsLatest_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)
	indexDef := func(name string) string {
		var def string
		require.NoError(t, db.QueryRow(
			"SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1", name).Scan(&def))
		return def
	}

	require.NoError(t, m.Migrate(21), "migrate to 021")
	require.False(t, indexExists(t, db, migrationWriteEventsLatestIdx), "fixture: the index must not exist before 022")
	require.False(t, indexExists(t, db, migrationWriteEventsUnsourcedIdx))

	require.NoError(t, m.Migrate(22), "migrate to 022")
	require.True(t, indexExists(t, db, migrationWriteEventsLatestIdx))
	require.True(t, indexExists(t, db, migrationWriteEventsUnsourcedIdx))

	latest := indexDef(migrationWriteEventsLatestIdx)
	assert.Contains(t, latest, "(resource_type, resource_id, created_at DESC)")
	assert.Contains(t, latest, "INCLUDE (source, assistant_name)")

	unsourced := indexDef(migrationWriteEventsUnsourcedIdx)
	assert.Contains(t, unsourced, "(team_id, created_at DESC)")
	assert.Contains(t, unsourced, "WHERE (source IS NULL)")

	// Roll back over a populated log.
	teamID := seedMigrationTeam(t, db)
	require.NoError(t, insertWriteEvent(db, teamID, nil))
	require.NoError(t, insertWriteEvent(db, teamID, map[string]any{"action": "update", "source": "web"}))

	require.NoError(t, m.Migrate(21), "migrate down to 021")
	assert.False(t, indexExists(t, db, migrationWriteEventsLatestIdx))
	assert.False(t, indexExists(t, db, migrationWriteEventsUnsourcedIdx))

	require.NoError(t, m.Migrate(22), "re-apply 022")
	assert.True(t, indexExists(t, db, migrationWriteEventsLatestIdx))
	assert.True(t, indexExists(t, db, migrationWriteEventsUnsourcedIdx))
}
