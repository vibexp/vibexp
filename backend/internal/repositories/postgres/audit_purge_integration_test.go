//go:build integration

package postgres

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// End-to-end retention purge (#1512): the REAL AuditPurgeService over the
// REAL settings and audit event repositories. audit_events and the settings
// singleton are global to the shared test database, so each test empties both
// and none runs in parallel.

func resetAuditPurgeTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), `DELETE FROM audit_events`)
	require.NoError(t, err)
	resetInstanceSettingsTable(t, instanceAuditSettingsTable)
}

// insertAuditEventsAt inserts n minimal events created at at, in one
// statement.
func insertAuditEventsAt(t *testing.T, at time.Time, n int) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), `
		INSERT INTO audit_events (created_at, actor_type, operation, source, outcome)
		SELECT $1, 'system', 'project.migrated', 'system', 'success' FROM generate_series(1, $2)`,
		at, n)
	require.NoError(t, err)
}

func countAuditEvents(t *testing.T) int {
	t.Helper()
	return countRows(t, integrationDB.DB, `SELECT count(*) FROM audit_events`)
}

func newIntegrationAuditPurge() *services.AuditPurgeService {
	return services.NewAuditPurgeService(
		NewInstanceAuditSettingsRepository(integrationDB),
		NewAuditEventRepository(integrationDB),
		slog.New(slog.DiscardHandler),
	)
}

// With nothing stored the default retention (2 weeks) applies; the purge
// deletes only the events older than it, across more than one batch.
func TestIntegrationAuditPurge_DeletesOnlyOlderThanTheDefaultRetention(t *testing.T) {
	resetAuditPurgeTables(t)
	now := time.Now().UTC()
	window := models.DefaultInstanceAuditSettingsValues().RetentionWindow()
	insertAuditEventsAt(t, now.Add(-window-time.Hour), services.AuditPurgeBatchSize+5)
	insertAuditEventsAt(t, now.Add(-window+time.Hour), 3)
	insertAuditEventsAt(t, now, 2)

	deleted, skipped, err := newIntegrationAuditPurge().Purge(context.Background())

	require.NoError(t, err)
	assert.False(t, skipped)
	assert.Equal(t, int64(services.AuditPurgeBatchSize+5), deleted, "every expired event, over two batches")
	assert.Equal(t, 5, countAuditEvents(t), "the events inside the window are kept")
}

// A stored retention is the one applied.
func TestIntegrationAuditPurge_UsesTheStoredRetention(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetAuditPurgeTables(t)
	require.NoError(t, NewInstanceAuditSettingsRepository(integrationDB).UpsertAudited(context.Background(),
		&models.InstanceAuditSettings{RetentionWeeks: 1, CaptureIPUA: true}, nil, upsertAuditSettingsAudit()))
	now := time.Now().UTC()
	insertAuditEventsAt(t, now.Add(-8*24*time.Hour), 4) // past one week
	insertAuditEventsAt(t, now.Add(-6*24*time.Hour), 2) // inside it

	deleted, _, err := newIntegrationAuditPurge().Purge(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(4), deleted)
	assert.Equal(t, 2, countAuditEvents(t))
}

// While another session holds the purge lock (another replica mid-purge), a
// purge skips and deletes nothing; once it is released the next purge runs.
func TestIntegrationAuditPurge_SecondConcurrentPurgeSkips(t *testing.T) {
	resetAuditPurgeTables(t)
	insertAuditEventsAt(t, time.Now().UTC().Add(-60*24*time.Hour), 3)

	acquired, release, err := NewInstanceAuditSettingsRepository(integrationDB).TryPurgeLock(context.Background())
	require.NoError(t, err)
	require.True(t, acquired, "fixture: this test plays the replica holding the lock")

	deleted, skipped, err := newIntegrationAuditPurge().Purge(context.Background())
	require.NoError(t, err)
	assert.True(t, skipped, "the lock is held elsewhere")
	assert.Zero(t, deleted)
	assert.Equal(t, 3, countAuditEvents(t), "a skipped purge deletes nothing")

	require.NoError(t, release())
	deleted, skipped, err = newIntegrationAuditPurge().Purge(context.Background())
	require.NoError(t, err)
	assert.False(t, skipped)
	assert.Equal(t, int64(3), deleted)
}

// Replicas starting together: of several purges racing, the ones that lose
// the lock skip, every expired event is deleted exactly once, and nothing
// newer is touched.
func TestIntegrationAuditPurge_RacingReplicas(t *testing.T) {
	resetAuditPurgeTables(t)
	now := time.Now().UTC()
	insertAuditEventsAt(t, now.Add(-30*24*time.Hour), 2500)
	insertAuditEventsAt(t, now, 7)

	const replicas = 4
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		total   int64
		skipped int
	)
	start := make(chan struct{})
	for range replicas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			n, skip, err := newIntegrationAuditPurge().Purge(context.Background())
			assert.NoError(t, err)
			mu.Lock()
			defer mu.Unlock()
			total += n
			if skip {
				skipped++
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, int64(2500), total, "every expired event deleted exactly once")
	assert.Less(t, skipped, replicas, "at least one replica purged")
	assert.Equal(t, 7, countAuditEvents(t))
}
