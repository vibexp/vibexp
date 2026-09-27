//go:build integration

package postgres

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Up/down round-trip for migration 021_instance_settings_audit (#1187), on its
// OWN scratch database: this test migrates DOWN, which would corrupt the shared
// integrationDB.
//
// Beyond "it applies", it pins the constraints the database itself enforces
// (the closed action set, a non-empty setting, SET NULL on the actor), that the
// newest-first index exists, and that the down migration drops a POPULATED
// table and can be re-applied.

const pqCheckViolation = "23514"

func TestMigration021_InstanceSettingsAudit_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)

	// 1. The version immediately before this migration: nothing exists yet.
	require.NoError(t, m.Migrate(20), "migrate to 020")
	require.False(t, tableExists(t, db, "instance_settings_audit"),
		"fixture: instance_settings_audit must not exist before 021")

	// 2. Apply 021.
	require.NoError(t, m.Migrate(21), "migrate to 021")

	t.Run("creates the table, empty", func(t *testing.T) {
		require.True(t, tableExists(t, db, "instance_settings_audit"))
		assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM instance_settings_audit"))
	})

	t.Run("indexes a setting's newest-first page", func(t *testing.T) {
		assert.Equal(t, 1, countRows(t, db,
			`SELECT count(*) FROM pg_indexes
			  WHERE schemaname = 'public' AND tablename = 'instance_settings_audit'
			    AND indexname = 'idx_instance_settings_audit_setting_created'
			    AND indexdef LIKE '%(setting, created_at DESC, id DESC)%'`))
	})

	t.Run("has no column that could hold a credential", func(t *testing.T) {
		assert.Equal(t, 0, countRows(t, db,
			`SELECT count(*) FROM information_schema.columns
			  WHERE table_schema = 'public' AND table_name = 'instance_settings_audit'
			    AND (column_name ILIKE '%secret%' OR column_name ILIKE '%password%'
			         OR column_name ILIKE '%token%' OR column_name ILIKE '%credential%')`))
		assert.Equal(t, 7, countRows(t, db,
			`SELECT count(*) FROM information_schema.columns
			  WHERE table_schema = 'public' AND table_name = 'instance_settings_audit'`),
			"a new column must be checked against the no-secret rule and added here deliberately")
	})

	t.Run("rejects an action outside upsert|delete|import", func(t *testing.T) {
		_, err := db.Exec(
			"INSERT INTO instance_settings_audit (setting, action) VALUES ($1, 'update')",
			models.InstanceSettingEmailProvider)
		requirePQCode(t, err, pqCheckViolation)
	})

	t.Run("rejects an empty setting", func(t *testing.T) {
		_, err := db.Exec(
			"INSERT INTO instance_settings_audit (setting, action) VALUES ('', $1)",
			models.InstanceSettingsAuditActionImport)
		requirePQCode(t, err, pqCheckViolation)
	})

	t.Run("accepts every action with no actor and no snapshots", func(t *testing.T) {
		for _, action := range []string{
			models.InstanceSettingsAuditActionUpsert,
			models.InstanceSettingsAuditActionDelete,
			models.InstanceSettingsAuditActionImport,
		} {
			_, err := db.Exec(
				"INSERT INTO instance_settings_audit (setting, action) VALUES ($1, $2)",
				models.InstanceSettingEmailProvider, action)
			require.NoError(t, err, action)
		}
	})

	t.Run("deleting the actor keeps the entry and blanks the actor", func(t *testing.T) {
		userID := uuid.New().String()
		_, err := db.Exec("INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
			userID, "audit-"+userID[:8]+"@example.com", "Audit Fixture")
		require.NoError(t, err)

		var entryID string
		require.NoError(t, db.QueryRow(
			`INSERT INTO instance_settings_audit (setting, action, actor_user_id, after)
			 VALUES ($1, $2, $3, '{"provider_type":"smtp"}') RETURNING id`,
			models.InstanceSettingEmailProvider, models.InstanceSettingsAuditActionUpsert, userID,
		).Scan(&entryID))

		_, err = db.Exec("DELETE FROM users WHERE id = $1", userID)
		require.NoError(t, err)

		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM instance_settings_audit WHERE id = $1 AND actor_user_id IS NULL",
			entryID))
	})

	// 3. Roll back a populated table.
	require.Positive(t, countRows(t, db, "SELECT count(*) FROM instance_settings_audit"),
		"fixture: the table must hold rows before rolling back")
	require.NoError(t, m.Migrate(20), "migrate down to 020")
	assert.False(t, tableExists(t, db, "instance_settings_audit"),
		"instance_settings_audit must be gone after rollback")
	assert.True(t, tableExists(t, db, "instance_email_provider"),
		"rolling back 021 must leave 020 in place")

	// 4. Re-apply: an operator who rolls back a release must be able to roll
	//    forward again.
	require.NoError(t, m.Migrate(21), "re-apply 021")
	assert.True(t, tableExists(t, db, "instance_settings_audit"))
	assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM instance_settings_audit"),
		"the re-applied table starts empty -- the dropped rows are gone for good")
}
