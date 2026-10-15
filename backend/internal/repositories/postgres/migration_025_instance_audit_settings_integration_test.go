//go:build integration

package postgres

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Up/down round-trip for migration 025_instance_audit_settings (#1512), on its
// OWN scratch database: this test migrates DOWN, which would corrupt the shared
// integrationDB.
//
// Beyond "it applies", it pins the singleton at the database level, the
// retention CHECK at and just past the Go constants it mirrors, SET NULL on the
// editor, and that the down migration drops a POPULATED table.

// migration025Previous is the version immediately before 025.
const migration025Previous = 24

func validInstanceAuditSettingsRow() map[string]any {
	return map[string]any{"retention_weeks": models.DefaultAuditRetentionWeeks, "capture_ip_ua": true}
}

// instanceAuditSettingsCheckCases builds the retention boundary cases from the
// Go constants, so a bound changed on one side only fails here.
func instanceAuditSettingsCheckCases() []instanceSettingsCheckCase {
	return []instanceSettingsCheckCase{
		{"the defaults", nil, true},
		{"retention at the minimum", map[string]any{"retention_weeks": models.MinAuditRetentionWeeks}, true},
		{"retention below the minimum", map[string]any{"retention_weeks": models.MinAuditRetentionWeeks - 1}, false},
		{"retention at the maximum", map[string]any{"retention_weeks": models.MaxAuditRetentionWeeks}, true},
		{"retention above the maximum", map[string]any{"retention_weeks": models.MaxAuditRetentionWeeks + 1}, false},
	}
}

func TestMigration025_InstanceAuditSettings_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)
	const table = "instance_audit_settings"
	wipe := func() {
		_, err := db.Exec("DELETE FROM " + table)
		require.NoError(t, err)
	}

	// 1. The version immediately before this migration: nothing exists yet.
	require.NoError(t, m.Migrate(migration025Previous), "migrate to the previous version")
	require.False(t, tableExists(t, db, table), "fixture: the table must not exist before 025")

	// 2. Apply 025.
	require.NoError(t, m.Migrate(25), "migrate to 025")

	t.Run("it is a singleton", func(t *testing.T) {
		assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM "+table))

		require.NoError(t, insertInstanceSettingsRow(db, table, validInstanceAuditSettingsRow(), nil))
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM "+table+" WHERE id AND version = 1 AND updated_by IS NULL"),
			"a first row takes the defaults")

		requirePQCode(t, insertInstanceSettingsRow(db, table, validInstanceAuditSettingsRow(), nil), "23505")
		assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM "+table))

		wipe()
		requirePQCode(t, insertInstanceSettingsRow(db, table, validInstanceAuditSettingsRow(),
			map[string]any{"id": false}), pqCheckViolation)
	})

	t.Run("both settings are required", func(t *testing.T) {
		wipe()
		for _, col := range []string{"retention_weeks", "capture_ip_ua"} {
			requirePQCode(t, insertInstanceSettingsRow(db, table, validInstanceAuditSettingsRow(),
				map[string]any{col: nil}), "23502")
		}
	})

	t.Run("retention CHECK", func(t *testing.T) {
		runInstanceSettingsChecks(t, db, table, validInstanceAuditSettingsRow(), instanceAuditSettingsCheckCases())
	})

	t.Run("deleting the editor keeps the row and blanks updated_by", func(t *testing.T) {
		wipe()
		userID := uuid.New().String()
		_, err := db.Exec("INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
			userID, "audit-settings-"+userID[:8]+"@example.com", "Audit Settings Fixture")
		require.NoError(t, err)
		require.NoError(t, insertInstanceSettingsRow(db, table, validInstanceAuditSettingsRow(),
			map[string]any{"updated_by": userID}))

		_, err = db.Exec("DELETE FROM users WHERE id = $1", userID)
		require.NoError(t, err)

		assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM "+table+" WHERE updated_by IS NULL"))
	})

	// 3. Roll back with the table populated.
	require.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM "+table),
		"fixture: the table must hold its row before rolling back")
	require.NoError(t, m.Migrate(migration025Previous), "migrate down to the previous version")
	assert.False(t, tableExists(t, db, table), "down must drop the populated table")
	assert.True(t, tableExists(t, db, "instance_settings_audit"),
		"rolling back 025 must leave earlier migrations in place")

	// 4. Re-apply: an operator who rolls back must be able to roll forward.
	require.NoError(t, m.Migrate(25), "re-apply 025")
	assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM "+table), "the re-applied table starts empty")
	require.NoError(t, insertInstanceSettingsRow(db, table, validInstanceAuditSettingsRow(), nil),
		"the re-applied table is usable")
}
