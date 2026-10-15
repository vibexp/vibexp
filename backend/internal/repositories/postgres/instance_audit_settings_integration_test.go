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

// Behavior-level suite for InstanceAuditSettingsRepository against real
// Postgres (#1512). The table is a singleton that is global to the shared test
// database, so every test starts from and leaves behind an empty table, and
// none runs in parallel.

const instanceAuditSettingsTable = "instance_audit_settings"

// auditSettingsAuditEntry snapshots only the retention, which is enough to tell
// the transitions apart.
func auditSettingsAuditEntry(action string) repositories.InstanceAuditSettingsAuditFunc {
	weeks := func(row *models.InstanceAuditSettings) []byte {
		if row == nil {
			return nil
		}
		return []byte(fmt.Sprintf(`{"retention_weeks":%d}`, row.RetentionWeeks))
	}
	return func(before, after *models.InstanceAuditSettings) (*models.InstanceSettingsAuditEntry, error) {
		return &models.InstanceSettingsAuditEntry{
			Setting: models.InstanceSettingAuditLog, Action: action,
			Before: weeks(before), After: weeks(after),
		}, nil
	}
}

func upsertAuditSettingsAudit() repositories.InstanceAuditSettingsAuditFunc {
	return auditSettingsAuditEntry(models.InstanceSettingsAuditActionUpsert)
}

func countInstanceAuditSettingsAudit(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT count(*) FROM instance_settings_audit WHERE setting = $1", models.InstanceSettingAuditLog).Scan(&n))
	return n
}

func TestIntegrationInstanceAuditSettings_Get_NoRow(t *testing.T) {
	resetInstanceSettingsTable(t, instanceAuditSettingsTable)

	got, err := NewInstanceAuditSettingsRepository(integrationDB).Get(context.Background())

	assert.Nil(t, got)
	assert.ErrorIs(t, err, repositories.ErrInstanceAuditSettingsNotFound)
}

func TestIntegrationInstanceAuditSettings_UpsertAuditedCreateThenUpdate(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, instanceAuditSettingsTable)
	repo := NewInstanceAuditSettingsRepository(integrationDB)
	ctx := context.Background()

	first := &models.InstanceAuditSettings{RetentionWeeks: 4, CaptureIPUA: true}
	require.NoError(t, repo.UpsertAudited(ctx, first, nil, upsertAuditSettingsAudit()))
	assert.Equal(t, int64(1), first.Version)
	assert.False(t, first.CreatedAt.IsZero(), "created_at must come back on the struct")

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, 4, got.RetentionWeeks)
	assert.True(t, got.CaptureIPUA)
	assert.Nil(t, got.UpdatedBy)

	editor := insertTestUser(t)
	second := &models.InstanceAuditSettings{RetentionWeeks: 52, CaptureIPUA: false, UpdatedBy: &editor}
	require.NoError(t, repo.UpsertAudited(ctx, second, nil, upsertAuditSettingsAudit()))
	assert.Equal(t, int64(2), second.Version, "an update bumps the version")
	assert.True(t, second.CreatedAt.Equal(first.CreatedAt), "the update keeps the original created_at")

	got, err = repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, 52, got.RetentionWeeks)
	assert.False(t, got.CaptureIPUA)
	require.NotNil(t, got.UpdatedBy)
	assert.Equal(t, editor, *got.UpdatedBy)
	assert.Equal(t, int64(2), got.Version)
	assert.Equal(t, 1, countRows(t, integrationDB.DB, "SELECT count(*) FROM instance_audit_settings"))
}

func TestIntegrationInstanceAuditSettings_AuditTrailAndDelete(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, instanceAuditSettingsTable)
	repo := NewInstanceAuditSettingsRepository(integrationDB)
	ctx := context.Background()

	require.NoError(t, repo.UpsertAudited(ctx,
		&models.InstanceAuditSettings{RetentionWeeks: 2, CaptureIPUA: true}, nil, upsertAuditSettingsAudit()))
	require.NoError(t, repo.UpsertAudited(ctx,
		&models.InstanceAuditSettings{RetentionWeeks: 8, CaptureIPUA: true}, nil, upsertAuditSettingsAudit()))

	deleteAudit := auditSettingsAuditEntry(models.InstanceSettingsAuditActionDelete)
	deleted, err := repo.DeleteAudited(ctx, deleteAudit)
	require.NoError(t, err)
	assert.True(t, deleted)
	deleted, err = repo.DeleteAudited(ctx, deleteAudit)
	require.NoError(t, err)
	assert.False(t, deleted, "a reset with no row deletes nothing")

	_, err = repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceAuditSettingsNotFound)

	entries, _, err := NewInstanceSettingsAuditRepository(integrationDB).
		List(ctx, models.InstanceSettingAuditLog, 10, nil)
	require.NoError(t, err)
	require.Len(t, entries, 3, "one entry per write, none for the no-op reset")
	// Newest first: delete(8→nil), upsert(2→8), upsert(nil→2).
	assert.Equal(t, models.InstanceSettingsAuditActionDelete, entries[0].Action)
	assert.JSONEq(t, `{"retention_weeks":8}`, string(entries[0].Before))
	assert.Nil(t, entries[0].After)
	assert.JSONEq(t, `{"retention_weeks":2}`, string(entries[1].Before), "before is read inside the transaction")
	assert.JSONEq(t, `{"retention_weeks":8}`, string(entries[1].After))
	assert.Nil(t, entries[2].Before, "the first save had no previous row")
}

// The compare-and-set is applied against the row read under the lock: a stale
// or wrong expectation writes neither the row nor an audit entry.
func TestIntegrationInstanceAuditSettings_UpsertAudited_CompareAndSet(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, instanceAuditSettingsTable)
	repo := NewInstanceAuditSettingsRepository(integrationDB)
	ctx := context.Background()
	version := func(v int64) *int64 { return &v }
	row := func(weeks int) *models.InstanceAuditSettings {
		return &models.InstanceAuditSettings{RetentionWeeks: weeks, CaptureIPUA: true}
	}

	err := repo.UpsertAudited(ctx, row(3), version(1), upsertAuditSettingsAudit())
	require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict, "a version with nothing stored")
	assert.Zero(t, countInstanceAuditSettingsAudit(t))

	noRow := repositories.InstanceSettingsNoStoredVersion
	require.NoError(t, repo.UpsertAudited(ctx, row(3), &noRow, upsertAuditSettingsAudit()))

	err = repo.UpsertAudited(ctx, row(4), &noRow, upsertAuditSettingsAudit())
	require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict, "expecting no row while one is stored")

	updated := row(5)
	require.NoError(t, repo.UpsertAudited(ctx, updated, version(1), upsertAuditSettingsAudit()))
	assert.Equal(t, int64(2), updated.Version)

	err = repo.UpsertAudited(ctx, row(6), version(1), upsertAuditSettingsAudit())
	require.ErrorIs(t, err, repositories.ErrInstanceSettingsVersionConflict, "a stale version")

	got, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, got.RetentionWeeks, "the conflicting write changed nothing")
	assert.Equal(t, int64(2), got.Version)
	assert.Equal(t, 2, countInstanceAuditSettingsAudit(t), "only the two successful writes are audited")
}

// A failed audit insert rolls the write back: the row never changes without
// its audit entry.
func TestIntegrationInstanceAuditSettings_UpsertAudited_AuditFailureRollsBack(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, instanceAuditSettingsTable)
	repo := NewInstanceAuditSettingsRepository(integrationDB)
	ctx := context.Background()

	// An unknown action violates the audit table's CHECK, failing the insert
	// after the upsert already ran in the same transaction.
	err := repo.UpsertAudited(ctx, &models.InstanceAuditSettings{RetentionWeeks: 3, CaptureIPUA: true},
		nil, auditSettingsAuditEntry("not-an-action"))
	require.Error(t, err)

	_, err = repo.Get(ctx)
	assert.ErrorIs(t, err, repositories.ErrInstanceAuditSettingsNotFound, "the upsert must have rolled back")
	assert.Zero(t, countInstanceAuditSettingsAudit(t))
}

// The retention CHECK is the storage backstop: an out-of-range value that got
// past the service is still refused, and nothing is audited.
func TestIntegrationInstanceAuditSettings_UpsertAudited_RetentionOutOfRange(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, instanceAuditSettingsTable)
	repo := NewInstanceAuditSettingsRepository(integrationDB)

	err := repo.UpsertAudited(context.Background(),
		&models.InstanceAuditSettings{RetentionWeeks: models.MaxAuditRetentionWeeks + 1},
		nil, upsertAuditSettingsAudit())

	require.Error(t, err)
	assert.Zero(t, countInstanceAuditSettingsAudit(t))
}

// Concurrent first saves on an empty table are serialized by the table write
// lock: exactly one of them sees no previous row.
func TestIntegrationInstanceAuditSettings_ConcurrentFirstSavesAreSerialized(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetInstanceSettingsTable(t, instanceAuditSettingsTable)
	repo := NewInstanceAuditSettingsRepository(integrationDB)
	ctx := context.Background()

	const writers = 4
	start := make(chan struct{})
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		s := &models.InstanceAuditSettings{RetentionWeeks: i + 1, CaptureIPUA: true}
		go func() {
			<-start
			errs <- repo.UpsertAudited(ctx, s, nil, upsertAuditSettingsAudit())
		}()
	}
	close(start)
	for i := 0; i < writers; i++ {
		require.NoError(t, <-errs)
	}

	entries, _, err := NewInstanceSettingsAuditRepository(integrationDB).
		List(ctx, models.InstanceSettingAuditLog, 10, nil)
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

// The purge lock admits one holder at a time across sessions, and release
// frees it for the next one.
func TestIntegrationInstanceAuditSettings_TryPurgeLock(t *testing.T) {
	ctx := context.Background()
	first := NewInstanceAuditSettingsRepository(integrationDB)
	second := NewInstanceAuditSettingsRepository(integrationDB)

	acquired, release, err := first.TryPurgeLock(ctx)
	require.NoError(t, err)
	require.True(t, acquired)

	contended, noop, err := second.TryPurgeLock(ctx)
	require.NoError(t, err)
	assert.False(t, contended, "a second session must not get the lock while it is held")
	require.NoError(t, noop(), "release of an unacquired lock is a no-op")

	assert.Equal(t, 1, countRows(t, integrationDB.DB,
		fmt.Sprintf("SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND granted "+
			"AND ((classid::bigint << 32) | objid::bigint) = %d", auditPurgeLockID)),
		"exactly one session holds the lock")

	require.NoError(t, release())
	assert.Zero(t, countRows(t, integrationDB.DB,
		fmt.Sprintf("SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' "+
			"AND ((classid::bigint << 32) | objid::bigint) = %d", auditPurgeLockID)),
		"release must unlock on the session that locked, so nothing leaks")

	again, releaseAgain, err := second.TryPurgeLock(ctx)
	require.NoError(t, err)
	require.True(t, again, "the lock is free again after release")
	require.NoError(t, releaseAgain())
}

// A cancelled context still releases the lock: the unlock runs on a background
// context, so a purge cut short by shutdown cannot strand it.
func TestIntegrationInstanceAuditSettings_TryPurgeLock_ReleaseAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := NewInstanceAuditSettingsRepository(integrationDB)

	acquired, release, err := repo.TryPurgeLock(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	cancel()
	require.NoError(t, release())

	again, releaseAgain, err := repo.TryPurgeLock(context.Background())
	require.NoError(t, err)
	assert.True(t, again, "the lock must have been released despite the cancelled context")
	require.NoError(t, releaseAgain())
}
