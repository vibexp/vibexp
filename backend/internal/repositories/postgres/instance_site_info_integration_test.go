//go:build integration

package postgres

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level suite for InstanceSiteInfoRepository against real Postgres
// (#1353). The table is a singleton that is global to the shared test database,
// so every test starts from and leaves behind an empty table, and none runs in
// parallel.

const instanceSiteInfoTable = "instance_site_info"

// siteInfoAuditEntry snapshots only the company name, which is enough to tell
// the transitions apart.
func siteInfoAuditEntry(action string) repositories.InstanceSiteInfoAuditFunc {
	name := func(row *models.InstanceSiteInfo) []byte {
		if row == nil {
			return nil
		}
		if row.CompanyName == nil {
			return []byte(`{"company_name":""}`)
		}
		return []byte(fmt.Sprintf(`{"company_name":%q}`, *row.CompanyName))
	}
	return func(before, after *models.InstanceSiteInfo) (*models.InstanceSettingsAuditEntry, error) {
		return &models.InstanceSettingsAuditEntry{
			Setting: models.InstanceSettingSiteInfo, Action: action,
			Before: name(before), After: name(after),
		}, nil
	}
}

func upsertSiteInfoAudit() repositories.InstanceSiteInfoAuditFunc {
	return siteInfoAuditEntry(models.InstanceSettingsAuditActionUpsert)
}

func countInstanceSiteInfoAudit(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT count(*) FROM instance_settings_audit WHERE setting = $1", models.InstanceSettingSiteInfo).Scan(&n))
	return n
}

func TestIntegrationInstanceSiteInfo_Get_NoRow(t *testing.T) {
	resetInstanceSettingsTable(t, instanceSiteInfoTable)

	got, err := NewInstanceSiteInfoRepository(integrationDB).Get(context.Background())

	assert.Nil(t, got)
	assert.ErrorIs(t, err, repositories.ErrInstanceSiteInfoNotFound)
}

func TestIntegrationInstanceSiteInfo_UpsertAuditedCreateThenUpdate(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, instanceSiteInfoTable)
	repo := NewInstanceSiteInfoRepository(integrationDB)
	ctx := context.Background()

	first := instanceSiteInfoFixture()
	require.NoError(t, repo.UpsertAudited(ctx, first, nil, upsertSiteInfoAudit()))
	assert.Equal(t, int64(1), first.Version)
	assert.False(t, first.CreatedAt.IsZero(), "created_at must come back on the struct")

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, got.CompanyName)
	assert.Equal(t, "Acme Inc.", *got.CompanyName)
	require.NotNil(t, got.CompanyEmail)
	assert.Equal(t, "hello@acme.test", *got.CompanyEmail)
	require.NotNil(t, got.Website)
	assert.Equal(t, "https://acme.test", *got.Website)
	assert.Nil(t, got.SupportURL, "an unset value is stored as NULL")
	assert.Nil(t, got.TermsURL)
	assert.Nil(t, got.PrivacyURL)
	assert.Nil(t, got.LogoURL)
	assert.Nil(t, got.LogoDarkURL)
	assert.Nil(t, got.UpdatedBy)

	editor := insertTestUser(t)
	second := &models.InstanceSiteInfo{
		SupportURL:  siteInfoPtr("https://acme.test/help"),
		TermsURL:    siteInfoPtr("https://acme.test/terms"),
		PrivacyURL:  siteInfoPtr("https://acme.test/privacy"),
		LogoURL:     siteInfoPtr("https://cdn.acme.test/logo.svg"),
		LogoDarkURL: siteInfoPtr("https://cdn.acme.test/logo-dark.svg"),
		UpdatedBy:   &editor,
	}
	require.NoError(t, repo.UpsertAudited(ctx, second, nil, upsertSiteInfoAudit()))
	assert.Equal(t, int64(2), second.Version, "an update bumps the version")
	assert.True(t, second.CreatedAt.Equal(first.CreatedAt), "the update keeps the original created_at")

	got, err = repo.Get(ctx)
	require.NoError(t, err)
	assert.Nil(t, got.CompanyName, "the update replaces the whole row: a value it omits is cleared")
	assert.Nil(t, got.CompanyEmail)
	assert.Nil(t, got.Website)
	require.NotNil(t, got.SupportURL)
	assert.Equal(t, "https://acme.test/help", *got.SupportURL)
	require.NotNil(t, got.TermsURL)
	assert.Equal(t, "https://acme.test/terms", *got.TermsURL)
	require.NotNil(t, got.PrivacyURL)
	assert.Equal(t, "https://acme.test/privacy", *got.PrivacyURL)
	require.NotNil(t, got.LogoURL)
	assert.Equal(t, "https://cdn.acme.test/logo.svg", *got.LogoURL)
	require.NotNil(t, got.LogoDarkURL)
	assert.Equal(t, "https://cdn.acme.test/logo-dark.svg", *got.LogoDarkURL)
	require.NotNil(t, got.UpdatedBy)
	assert.Equal(t, editor, *got.UpdatedBy)
	assert.Equal(t, int64(2), got.Version)
	assert.Equal(t, 1, countRows(t, integrationDB.DB, "SELECT count(*) FROM instance_site_info"))
}

func TestIntegrationInstanceSiteInfo_AuditTrailAndDelete(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, instanceSiteInfoTable)
	repo := NewInstanceSiteInfoRepository(integrationDB)
	ctx := context.Background()

	require.NoError(t, repo.UpsertAudited(ctx, instanceSiteInfoFixture(), nil, upsertSiteInfoAudit()))
	second := instanceSiteInfoFixture()
	second.CompanyName = siteInfoPtr("Acme GmbH")
	require.NoError(t, repo.UpsertAudited(ctx, second, nil, upsertSiteInfoAudit()))

	deleteAudit := siteInfoAuditEntry(models.InstanceSettingsAuditActionDelete)
	deleted, err := repo.DeleteAudited(ctx, deleteAudit)
	require.NoError(t, err)
	assert.True(t, deleted)
	deleted, err = repo.DeleteAudited(ctx, deleteAudit)
	require.NoError(t, err)
	assert.False(t, deleted, "a reset with no row deletes nothing")

	_, err = repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceSiteInfoNotFound)

	entries, _, err := NewInstanceSettingsAuditRepository(integrationDB).
		List(ctx, models.InstanceSettingSiteInfo, 10, nil)
	require.NoError(t, err)
	require.Len(t, entries, 3, "one entry per write, none for the no-op reset")
	// Newest first: delete(GmbH→nil), upsert(Inc.→GmbH), upsert(nil→Inc.).
	assert.Equal(t, models.InstanceSettingsAuditActionDelete, entries[0].Action)
	assert.JSONEq(t, `{"company_name":"Acme GmbH"}`, string(entries[0].Before))
	assert.Nil(t, entries[0].After)
	assert.JSONEq(t, `{"company_name":"Acme Inc."}`, string(entries[1].Before), "before is read inside the transaction")
	assert.JSONEq(t, `{"company_name":"Acme GmbH"}`, string(entries[1].After))
	assert.Nil(t, entries[2].Before, "the first save had no previous row")
}

// The compare-and-set is applied against the row read under the lock: a stale
// or wrong expectation writes neither the row nor an audit entry.
func TestIntegrationInstanceSiteInfo_UpsertAudited_CompareAndSet(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, instanceSiteInfoTable)
	repo := NewInstanceSiteInfoRepository(integrationDB)
	ctx := context.Background()
	version := func(v int64) *int64 { return &v }

	err := repo.UpsertAudited(ctx, instanceSiteInfoFixture(), version(1), upsertSiteInfoAudit())
	require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict, "a version with nothing stored")
	assert.Zero(t, countInstanceSiteInfoAudit(t))

	noRow := repositories.InstanceSettingsNoStoredVersion
	require.NoError(t, repo.UpsertAudited(ctx, instanceSiteInfoFixture(), &noRow, upsertSiteInfoAudit()))

	err = repo.UpsertAudited(ctx, instanceSiteInfoFixture(), &noRow, upsertSiteInfoAudit())
	require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict, "expecting no row while one is stored")

	renamed := instanceSiteInfoFixture()
	renamed.CompanyName = siteInfoPtr("Acme GmbH")
	require.NoError(t, repo.UpsertAudited(ctx, renamed, version(1), upsertSiteInfoAudit()))
	assert.Equal(t, int64(2), renamed.Version)

	stale := instanceSiteInfoFixture()
	stale.CompanyName = siteInfoPtr("Stale Co")
	err = repo.UpsertAudited(ctx, stale, version(1), upsertSiteInfoAudit())
	require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict, "a stale version")

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, got.CompanyName)
	assert.Equal(t, "Acme GmbH", *got.CompanyName, "the conflicting write changed nothing")
	assert.Equal(t, int64(2), got.Version)
	assert.Equal(t, 2, countInstanceSiteInfoAudit(t), "only the two successful writes are audited")
}

// A failed audit insert rolls the write back: the row never changes without
// its audit entry.
func TestIntegrationInstanceSiteInfo_UpsertAudited_AuditFailureRollsBack(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, instanceSiteInfoTable)
	repo := NewInstanceSiteInfoRepository(integrationDB)
	ctx := context.Background()

	// An unknown action violates the audit table's CHECK, failing the insert
	// after the upsert already ran in the same transaction.
	err := repo.UpsertAudited(ctx, instanceSiteInfoFixture(), nil, siteInfoAuditEntry("not-an-action"))
	require.Error(t, err)

	_, err = repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceSiteInfoNotFound, "the upsert must have rolled back")
	assert.Zero(t, countInstanceSiteInfoAudit(t))
}

// Concurrent first saves on an empty table are serialized by the table write
// lock: exactly one of them sees no previous row.
func TestIntegrationInstanceSiteInfo_ConcurrentFirstSavesAreSerialized(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, instanceSiteInfoTable)
	repo := NewInstanceSiteInfoRepository(integrationDB)
	ctx := context.Background()

	const writers = 4
	start := make(chan struct{})
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		s := instanceSiteInfoFixture()
		s.CompanyName = siteInfoPtr(fmt.Sprintf("Acme %d", i))
		go func() {
			<-start
			errs <- repo.UpsertAudited(ctx, s, nil, upsertSiteInfoAudit())
		}()
	}
	close(start)
	for i := 0; i < writers; i++ {
		require.NoError(t, <-errs)
	}

	entries, _, err := NewInstanceSettingsAuditRepository(integrationDB).
		List(ctx, models.InstanceSettingSiteInfo, 10, nil)
	require.NoError(t, err)
	require.Len(t, entries, writers)
	var firstSaves int
	for _, e := range entries {
		if e.Before == nil {
			firstSaves++
		}
	}
	assert.Equal(t, 1, firstSaves, "only the first serialized writer may see an empty table")
}
