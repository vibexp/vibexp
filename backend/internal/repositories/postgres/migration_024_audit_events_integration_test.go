//go:build integration

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Up/down round-trip for migration 024_audit_events (#1511), on its OWN
// scratch database: this test migrates DOWN, which would corrupt the shared
// integrationDB.

var migration024AuditIndexes = map[string]string{
	"idx_audit_events_created":           "(created_at DESC, id DESC)",
	"idx_audit_events_actor_created":     "(actor_user_id, created_at DESC)",
	"idx_audit_events_team_created":      "(team_id, created_at DESC)",
	"idx_audit_events_operation_created": "(operation, created_at DESC)",
}

func TestMigration024_AuditEvents_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()
	m := newMigrator(t, db)

	require.NoError(t, m.Migrate(23), "migrate to 023")
	require.False(t, tableExists(t, db, "audit_events"), "fixture: the table must not exist before 024")

	require.NoError(t, m.Migrate(24), "migrate to 024")
	require.True(t, tableExists(t, db, "audit_events"))

	for name, columns := range migration024AuditIndexes {
		var def string
		require.NoError(t, db.QueryRow(
			"SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1", name).Scan(&def), name)
		assert.Contains(t, def, columns, name)
	}

	var foreignKeys int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM pg_constraint
		WHERE conrelid = 'audit_events'::regclass AND contype = 'f'`).Scan(&foreignKeys))
	assert.Zero(t, foreignKeys, "the log must outlive what it names: no foreign keys")

	for _, check := range []string{
		"audit_events_actor_type_check", "audit_events_source_check", "audit_events_outcome_check",
	} {
		assert.Equal(t, 1, constraintCount(t, db, check), check)
	}

	var metadataDefault, metadataNullable string
	require.NoError(t, db.QueryRow(`SELECT column_default, is_nullable FROM information_schema.columns
		WHERE table_name = 'audit_events' AND column_name = 'metadata'`).Scan(&metadataDefault, &metadataNullable))
	assert.Equal(t, "'{}'::jsonb", metadataDefault)
	assert.Equal(t, "NO", metadataNullable)

	// A row with only the required columns takes every default.
	_, err := db.Exec(`INSERT INTO audit_events (actor_type, operation, source, outcome)
		VALUES ('system', 'audit.export', 'system', 'success')`)
	require.NoError(t, err)
	var metadata string
	require.NoError(t, db.QueryRow("SELECT metadata::text FROM audit_events").Scan(&metadata))
	assert.Equal(t, "{}", metadata)

	// Roll back over a populated log, then re-apply.
	require.NoError(t, m.Migrate(23), "migrate down to 023")
	assert.False(t, tableExists(t, db, "audit_events"))
	for name := range migration024AuditIndexes {
		assert.False(t, indexExists(t, db, name), name)
	}

	require.NoError(t, m.Migrate(24), "re-apply 024")
	assert.True(t, tableExists(t, db, "audit_events"))
}
