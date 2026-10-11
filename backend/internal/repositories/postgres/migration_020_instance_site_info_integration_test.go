//go:build integration

package postgres

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Up/down round-trip for migration 020_instance_site_info (#1353), on its OWN
// scratch database: this test migrates DOWN, which would corrupt the shared
// integrationDB.
//
// Beyond "it applies", it pins the singleton at the database level, that every
// value is optional, each length CHECK at and just past the Go constant it
// mirrors, SET NULL on the editor, and that the down migration drops a
// POPULATED table.

// instanceSiteInfoCheckCases builds the length boundary cases from the Go
// constants, so a cap changed on one side only fails here.
func instanceSiteInfoCheckCases() []instanceSettingsCheckCase {
	cases := []instanceSettingsCheckCase{
		{"every value set", nil, true},
		{"company_name at the cap", map[string]any{
			"company_name": strings.Repeat("a", models.MaxSiteInfoCompanyNameLength),
		}, true},
		{"company_name at the cap in multi-byte characters", map[string]any{
			"company_name": strings.Repeat("é", models.MaxSiteInfoCompanyNameLength),
		}, true},
		{"company_name over the cap", map[string]any{
			"company_name": strings.Repeat("a", models.MaxSiteInfoCompanyNameLength+1),
		}, false},
		{"company_email at the cap", map[string]any{
			"company_email": strings.Repeat("a", models.MaxSiteInfoEmailLength),
		}, true},
		{"company_email over the cap", map[string]any{
			"company_email": strings.Repeat("a", models.MaxSiteInfoEmailLength+1),
		}, false},
	}
	for _, col := range []string{
		"website", "support_url", "terms_url", "privacy_url", "logo_url", "logo_dark_url",
	} {
		cases = append(cases,
			instanceSettingsCheckCase{col + " at the cap", map[string]any{
				col: strings.Repeat("a", models.MaxSiteInfoURLLength),
			}, true},
			instanceSettingsCheckCase{col + " over the cap", map[string]any{
				col: strings.Repeat("a", models.MaxSiteInfoURLLength+1),
			}, false},
		)
	}
	return cases
}

func validInstanceSiteInfoRow() map[string]any {
	return map[string]any{
		"company_name":  "Acme Inc.",
		"company_email": "hello@acme.test",
		"website":       "https://acme.test",
		"support_url":   "https://acme.test/help",
		"terms_url":     "https://acme.test/terms",
		"privacy_url":   "https://acme.test/privacy",
		"logo_url":      "https://cdn.acme.test/logo.svg",
		"logo_dark_url": "https://cdn.acme.test/logo-dark.svg",
	}
}

func TestMigration020_InstanceSiteInfo_UpDownRoundTrip(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()

	m := newMigrator(t, db)
	const table = "instance_site_info"
	wipe := func() {
		_, err := db.Exec("DELETE FROM " + table)
		require.NoError(t, err)
	}

	// 1. The version immediately before this migration: nothing exists yet.
	require.NoError(t, m.Migrate(19), "migrate to 019")
	require.False(t, tableExists(t, db, table), "fixture: the table must not exist before 020")

	// 2. Apply 020.
	require.NoError(t, m.Migrate(20), "migrate to 020")

	t.Run("it is a singleton", func(t *testing.T) {
		assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM "+table))

		require.NoError(t, insertInstanceSettingsRow(db, table, validInstanceSiteInfoRow(), nil))
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM "+table+" WHERE id AND version = 1 AND updated_by IS NULL"),
			"a first row takes the defaults")

		requirePQCode(t, insertInstanceSettingsRow(db, table, validInstanceSiteInfoRow(), nil), "23505")
		assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM "+table))

		wipe()
		requirePQCode(t, insertInstanceSettingsRow(db, table, validInstanceSiteInfoRow(), map[string]any{"id": false}),
			pqCheckViolation)
	})

	t.Run("every value is optional", func(t *testing.T) {
		wipe()
		_, err := db.Exec("INSERT INTO " + table + " DEFAULT VALUES")
		require.NoError(t, err)
		assert.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM instance_site_info
			WHERE company_name IS NULL AND company_email IS NULL AND website IS NULL
			  AND support_url IS NULL AND terms_url IS NULL AND privacy_url IS NULL
			  AND logo_url IS NULL AND logo_dark_url IS NULL`))
		wipe()
	})

	t.Run("length CHECKs", func(t *testing.T) {
		runInstanceSettingsChecks(t, db, table, validInstanceSiteInfoRow(), instanceSiteInfoCheckCases())
	})

	t.Run("deleting the editor keeps the row and blanks updated_by", func(t *testing.T) {
		userID := uuid.New().String()
		_, err := db.Exec("INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
			userID, "site-info-"+userID[:8]+"@example.com", "Site Info Fixture")
		require.NoError(t, err)
		require.NoError(t, insertInstanceSettingsRow(db, table, validInstanceSiteInfoRow(),
			map[string]any{"updated_by": userID}))

		_, err = db.Exec("DELETE FROM users WHERE id = $1", userID)
		require.NoError(t, err)

		assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM "+table+" WHERE updated_by IS NULL"))
	})

	// 3. Roll back with the table populated.
	require.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM "+table),
		"fixture: the table must hold its row before rolling back")
	require.NoError(t, m.Migrate(19), "migrate down to 019")
	assert.False(t, tableExists(t, db, table), "down must drop the populated table")
	assert.True(t, tableExists(t, db, "instance_settings_audit"), "rolling back 020 must leave earlier migrations in place")

	// 4. Re-apply: an operator who rolls back must be able to roll forward.
	require.NoError(t, m.Migrate(20), "re-apply 020")
	assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM "+table), "the re-applied table starts empty")
	require.NoError(t, insertInstanceSettingsRow(db, table, validInstanceSiteInfoRow(), nil),
		"the re-applied table is usable")
}
