//go:build integration

package postgres

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Up/down round-trip for migration 021_move_privacy_url_to_site_info (#1363),
// on its OWN scratch database: each case migrates down and up again.
//
// It pins when the privacy URL stored with the email settings is copied into
// site info (and when it must not be), that a copy is audited, that the column
// is dropped, and that the down migration restores it.

const (
	migration021Before = 20
	migration021After  = 21
)

// seedEmailProviderPrivacyURL stores an email provider row at version 020,
// where the column still exists. A nil url stores NULL.
func seedEmailProviderPrivacyURL(t *testing.T, db *sql.DB, url *string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO instance_email_provider (provider_type, from_address, privacy_policy_url)
		VALUES ('smtp', 'noreply@acme.test', $1)`, url)
	require.NoError(t, err)
}

func emailProviderHasPrivacyColumn(t *testing.T, db *sql.DB) bool {
	t.Helper()
	return countRows(t, db, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'instance_email_provider'
		  AND column_name = 'privacy_policy_url'`) == 1
}

// siteInfoPrivacyURL returns site info's privacy URL and whether a row exists.
func siteInfoPrivacyURL(t *testing.T, db *sql.DB) (url *string, hasRow bool) {
	t.Helper()
	err := db.QueryRow("SELECT privacy_url FROM instance_site_info").Scan(&url)
	if err == sql.ErrNoRows {
		return nil, false
	}
	require.NoError(t, err)
	return url, true
}

func siteInfoAuditCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	return countRows(t, db, "SELECT count(*) FROM instance_settings_audit WHERE setting = 'site_info'")
}

func TestMigration021_MovePrivacyURLToSiteInfo(t *testing.T) {
	db, cleanup := newScratchMigrationDB(t)
	defer cleanup()
	m := newMigrator(t, db)

	// reset returns the database to version 020 with all three tables empty.
	reset := func(t *testing.T) {
		t.Helper()
		require.NoError(t, m.Migrate(migration021Before), "migrate to 020")
		for _, table := range []string{"instance_email_provider", "instance_site_info", "instance_settings_audit"} {
			_, err := db.Exec("DELETE FROM " + table) // #nosec G202 -- test-only, fixed identifiers
			require.NoError(t, err)
		}
		require.True(t, emailProviderHasPrivacyColumn(t, db), "fixture: the column exists before 021")
	}
	up := func(t *testing.T) {
		t.Helper()
		require.NoError(t, m.Migrate(migration021After), "migrate to 021")
		assert.False(t, emailProviderHasPrivacyColumn(t, db), "021 drops the column")
	}
	str := func(s string) *string { return &s }

	t.Run("copied into a new site info row, and audited", func(t *testing.T) {
		reset(t)
		seedEmailProviderPrivacyURL(t, db, str("  https://acme.test/privacy  "))

		up(t)

		got, hasRow := siteInfoPrivacyURL(t, db)
		require.True(t, hasRow)
		require.NotNil(t, got)
		assert.Equal(t, "https://acme.test/privacy", *got, "the value is trimmed")
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM instance_site_info WHERE version = 1 AND updated_by IS NULL AND company_name IS NULL"))

		var action string
		var actor, before sql.NullString
		var after string
		require.NoError(t, db.QueryRow(`SELECT action, actor_user_id, before, after
			FROM instance_settings_audit WHERE setting = $1`, models.InstanceSettingSiteInfo).
			Scan(&action, &actor, &before, &after))
		assert.Equal(t, models.InstanceSettingsAuditActionImport, action)
		assert.False(t, actor.Valid, "a migration has no actor")
		assert.False(t, before.Valid, "there was no row before")
		assert.JSONEq(t, `{"company_name":"","company_email":"","website":"","support_url":"",
			"terms_url":"","privacy_url":"https://acme.test/privacy","logo_url":"","logo_dark_url":""}`, after)
		assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM instance_email_provider"),
			"the email provider row itself is kept")
	})

	t.Run("copied into an existing row that has no privacy URL", func(t *testing.T) {
		reset(t)
		seedEmailProviderPrivacyURL(t, db, str("https://acme.test/privacy"))
		_, err := db.Exec(`INSERT INTO instance_site_info (company_name, terms_url, version)
			VALUES ('Acme Inc.', 'https://acme.test/terms', 4)`)
		require.NoError(t, err)

		up(t)

		assert.Equal(t, 1, countRows(t, db, `SELECT count(*) FROM instance_site_info
			WHERE privacy_url = 'https://acme.test/privacy' AND company_name = 'Acme Inc.'
			  AND terms_url = 'https://acme.test/terms' AND version = 5`),
			"the other values are kept and the version is bumped")

		var before, after string
		require.NoError(t, db.QueryRow(
			"SELECT before, after FROM instance_settings_audit WHERE setting = 'site_info'").Scan(&before, &after))
		assert.JSONEq(t, `{"company_name":"Acme Inc.","company_email":"","website":"","support_url":"",
			"terms_url":"https://acme.test/terms","privacy_url":"","logo_url":"","logo_dark_url":""}`, before)
		assert.JSONEq(t, `{"company_name":"Acme Inc.","company_email":"","website":"","support_url":"",
			"terms_url":"https://acme.test/terms","privacy_url":"https://acme.test/privacy",
			"logo_url":"","logo_dark_url":""}`, after)
	})

	t.Run("a privacy URL already in site info is never overwritten", func(t *testing.T) {
		reset(t)
		seedEmailProviderPrivacyURL(t, db, str("https://email-settings.test/privacy"))
		_, err := db.Exec("INSERT INTO instance_site_info (privacy_url) VALUES ('https://site-info.test/privacy')")
		require.NoError(t, err)

		up(t)

		got, _ := siteInfoPrivacyURL(t, db)
		require.NotNil(t, got)
		assert.Equal(t, "https://site-info.test/privacy", *got)
		assert.Equal(t, 1, countRows(t, db, "SELECT count(*) FROM instance_site_info WHERE version = 1"))
		assert.Zero(t, siteInfoAuditCount(t, db), "nothing changed, so nothing is audited")
	})

	skipped := map[string]*string{
		"NULL":                          nil,
		"empty":                         str(""),
		"whitespace only":               str("   "),
		"the legacy default":            str("https://example.com/privacy-policy"),
		"the legacy default, padded":    str(" https://example.com/privacy-policy "),
		"longer than the site info cap": str("https://acme.test/" + strings.Repeat("a", models.MaxSiteInfoURLLength)),
	}
	for name, value := range skipped {
		t.Run("not copied: "+name, func(t *testing.T) {
			reset(t)
			seedEmailProviderPrivacyURL(t, db, value)

			up(t)

			_, hasRow := siteInfoPrivacyURL(t, db)
			assert.False(t, hasRow, "no site info row is created")
			assert.Zero(t, siteInfoAuditCount(t, db))
		})
	}

	t.Run("a URL exactly at the cap is copied", func(t *testing.T) {
		reset(t)
		atCap := "https://acme.test/" + strings.Repeat("a", models.MaxSiteInfoURLLength-len("https://acme.test/"))
		seedEmailProviderPrivacyURL(t, db, &atCap)

		up(t)

		got, hasRow := siteInfoPrivacyURL(t, db)
		require.True(t, hasRow)
		require.NotNil(t, got)
		assert.Equal(t, atCap, *got)
	})

	t.Run("no email provider row at all", func(t *testing.T) {
		reset(t)

		up(t)

		_, hasRow := siteInfoPrivacyURL(t, db)
		assert.False(t, hasRow)
		assert.Zero(t, siteInfoAuditCount(t, db))
	})

	t.Run("down restores the column and refills it from site info", func(t *testing.T) {
		reset(t)
		seedEmailProviderPrivacyURL(t, db, str("https://acme.test/privacy"))
		up(t)

		require.NoError(t, m.Migrate(migration021Before), "migrate down to 020")

		require.True(t, emailProviderHasPrivacyColumn(t, db))
		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM instance_email_provider WHERE privacy_policy_url = 'https://acme.test/privacy'"))
		got, hasRow := siteInfoPrivacyURL(t, db)
		require.True(t, hasRow, "site info is left as it is")
		require.NotNil(t, got)
		assert.Equal(t, "https://acme.test/privacy", *got)

		// An operator who rolls back must be able to roll forward again: site
		// info already holds the URL, so the second pass changes nothing.
		up(t)
		assert.Equal(t, 1, siteInfoAuditCount(t, db), "the re-applied migration does not audit a no-op")
	})

	t.Run("down with no site info leaves the column NULL", func(t *testing.T) {
		reset(t)
		seedEmailProviderPrivacyURL(t, db, nil)
		up(t)

		require.NoError(t, m.Migrate(migration021Before), "migrate down to 020")

		assert.Equal(t, 1, countRows(t, db,
			"SELECT count(*) FROM instance_email_provider WHERE privacy_policy_url IS NULL"))
	})
}
