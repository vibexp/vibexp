//go:build integration

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 014 creates the durable embedding job queue (issue #820).
//
// The queue's behaviour is covered by embedding_job_integration_test.go against
// the shared database. What is left for this file is the migration itself: the
// two partial indexes and the state CHECK exist (the queue's correctness rests
// on all three -- coalescing, claim ordering, and the state vocabulary), and the
// round trip down and back up is clean.
//
// It runs on its OWN scratch database because it migrates DOWN, which would
// corrupt the shared integrationDB and, since that database is per-machine
// rather than per-checkout, other worktrees with it.

func TestMigration014_CreatesTheLeasedQueueAndRoundTrips(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)
	require.NoError(t, m.Migrate(14), "migrate to 014")

	require.True(t, tableExists(t, db, "embedding_jobs"))
	assert.True(t, indexExists(t, db, "idx_embedding_jobs_outstanding_entity"),
		"without the partial unique index the enqueue's ON CONFLICT has nothing to infer and every re-enqueue fails")
	assert.True(t, indexExists(t, db, "idx_embedding_jobs_claimable"),
		"without the claimable index every poll sorts the whole outstanding set")

	// The state vocabulary is enforced in the schema, not only in Go: a queue
	// row in an unknown state would be neither claimable nor terminal, i.e.
	// invisible work.
	_, err := db.Exec(
		`INSERT INTO embedding_jobs (entity_type, entity_id, user_id, payload, state)
		 VALUES ('memory', 'm1', 'u1', '{}'::jsonb, 'somewhere_else')`)
	require.Error(t, err, "an unknown state must be rejected by the CHECK constraint")

	// A row in a legitimate state, so the rollback has something to destroy and
	// the re-apply has to produce a usable table rather than an empty shell.
	_, err = db.Exec(
		`INSERT INTO embedding_jobs (entity_type, entity_id, user_id, payload)
		 VALUES ('memory', 'm1', 'u1', '{"body":"text"}'::jsonb)`)
	require.NoError(t, err)
	require.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM embedding_jobs`))

	require.NoError(t, m.Migrate(13), "migrate down to 013")
	assert.False(t, tableExists(t, db, "embedding_jobs"),
		"the down migration must drop the queue table")

	require.NoError(t, m.Migrate(14), "re-apply 014")
	require.True(t, tableExists(t, db, "embedding_jobs"))
	assert.Equal(t, 0, countRows(t, db, `SELECT count(*) FROM embedding_jobs`),
		"the re-applied table starts empty -- 014 seeds nothing")

	// Usable, not merely present: a fresh enqueue-shaped insert must still work
	// after the round trip.
	_, err = db.Exec(
		`INSERT INTO embedding_jobs (entity_type, entity_id, user_id, payload)
		 VALUES ('prompt', 'p1', 'u1', '{"body":"text"}'::jsonb)`)
	require.NoError(t, err)
	assert.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM embedding_jobs`))
}
