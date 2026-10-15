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

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/audit"
	"github.com/vibexp/vibexp/pkg/events"
)

// countingDrops is an audit.DropCounter that counts drops.
type countingDrops struct {
	mu    sync.Mutex
	drops []string
}

func (d *countingDrops) RecordAuditEventDropped(_ context.Context, reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.drops = append(d.drops, reason)
}

// End to end (#1512): the REAL recorder, bound to the REAL capture switch over
// the stored settings, persists the client IP and user agent while capture is
// on and NULLs them once an admin switches it off through the settings service.
func TestIntegrationAuditCapture_SettingDecidesClientInfo(t *testing.T) {
	resetInstanceSettingsAuditTables(t)
	resetAuditPurgeTables(t)
	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	settingsRepo := NewInstanceAuditSettingsRepository(integrationDB)
	capture := services.NewAuditClientInfoCapture(settingsRepo, logger)
	pool := events.NewWorkerPool(1)
	pool.Start()
	t.Cleanup(pool.Stop)
	drops := &countingDrops{}
	recorder := audit.NewRecorder(audit.RecorderDeps{
		Repo:     NewAuditEventRepository(integrationDB),
		Users:    NewUserRepository(integrationDB),
		APIKeys:  NewAPIKeyRepository(integrationDB),
		Teams:    NewTeamRepository(integrationDB),
		Projects: NewProjectRepository(integrationDB),
		Capture:  capture,
		Pool:     pool,
		Drops:    drops,
		Logger:   logger,
	})
	settings := services.NewInstanceAuditSettingsService(settingsRepo, capture, recorder, logger)

	// A request as the HTTP middleware leaves it: request id, client IP, UA.
	reqCtx := context.WithValue(ctx, contextkeys.RequestID, "req-capture")
	reqCtx = context.WithValue(reqCtx, contextkeys.ClientIP, "203.0.113.9")
	reqCtx = context.WithValue(reqCtx, contextkeys.UserAgent, "capture-test/1.0")
	clientInfo := func(op models.AuditOperation) (ip, ua *string) {
		var row struct{ ip, ua *string }
		require.Eventually(t, func() bool {
			err := integrationDB.QueryRowContext(ctx,
				`SELECT ip, user_agent FROM audit_events WHERE operation = $1 ORDER BY created_at DESC LIMIT 1`,
				string(op)).Scan(&row.ip, &row.ua)
			return err == nil
		}, 10*time.Second, 20*time.Millisecond, "the %s event is persisted", op)
		return row.ip, row.ua
	}

	recorder.Record(reqCtx, audit.Event{Operation: models.AuditOpProjectMigrated})
	ip, ua := clientInfo(models.AuditOpProjectMigrated)
	require.NotNil(t, ip, "capture is on by default")
	assert.Equal(t, "203.0.113.9", *ip)
	require.NotNil(t, ua)
	assert.Equal(t, "capture-test/1.0", *ua)

	_, err := settings.Update(reqCtx, "",
		models.InstanceAuditSettingsValues{RetentionWeeks: 2, CaptureIPUA: false}, nil)
	require.NoError(t, err)

	// The settings change is itself an event, recorded after the write with
	// capture already off.
	ip, ua = clientInfo(models.AuditOpAuditSettingsUpdated)
	assert.Nil(t, ip, "capture is off from the very next event")
	assert.Nil(t, ua)

	recorder.Record(reqCtx, audit.Event{Operation: models.AuditOpAuthzDenied, Outcome: models.AuditOutcomeDenied})
	ip, ua = clientInfo(models.AuditOpAuthzDenied)
	assert.Nil(t, ip)
	assert.Nil(t, ua)
	assert.Empty(t, drops.drops)
}
