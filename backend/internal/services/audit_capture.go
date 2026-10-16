package services

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// auditCaptureCacheTTL is how long a resolved client-info capture switch is
// served from memory. A write through this process drops the cached value at
// once; another replica picks the change up within this long.
const auditCaptureCacheTTL = 30 * time.Second

// auditCaptureReadTimeout bounds one read of the capture switch. The read runs
// detached from the caller's cancellation (see readCapture), so this is what
// keeps a stuck database from holding the recorder.
const auditCaptureReadTimeout = 5 * time.Second

// AuditClientInfoCapture is the instance's capture_ip_ua setting as the audit
// recorder consults it (#1512): whether a newly recorded event stores the
// client IP address and user agent.
//
// The recorder asks once per event, so the value is served from memory for up
// to auditCaptureCacheTTL. InstanceAuditSettingsService calls Invalidate after
// every committed write, so a change made through this process applies to the
// very next event.
//
// It is a type of its own, not a method of the settings service, because the
// settings service records its changes through the recorder: the recorder
// depends on this, and the settings service on both.
type AuditClientInfoCapture struct {
	repo   repositories.InstanceAuditSettingsRepository
	logger *slog.Logger
	now    func() time.Time

	mu sync.Mutex
	// cached is the cached capture switch; nil when nothing is cached.
	cached *cachedAuditCapture
	// generation is bumped by every Invalidate, so a read that started before
	// a write cannot cache what it read after the write dropped the cache.
	generation uint64
}

// cachedAuditCapture is one resolved capture switch and when it was read.
type cachedAuditCapture struct {
	value  bool
	readAt time.Time
}

var _ audit.ClientInfoCapturer = (*AuditClientInfoCapture)(nil)

// NewAuditClientInfoCapture creates the capture switch over the settings
// repository.
func NewAuditClientInfoCapture(
	repo repositories.InstanceAuditSettingsRepository, logger *slog.Logger,
) *AuditClientInfoCapture {
	return &AuditClientInfoCapture{repo: repo, logger: logger, now: time.Now}
}

// CaptureClientInfo implements audit.ClientInfoCapturer.
//
// With nothing stored it returns models.DefaultAuditCaptureClientInfo. When
// the settings cannot be read it FAILS CLOSED to false (store no IP address or
// user agent): an admin who switched capture off must never have it silently
// switched back on by a database blip, while losing the client info of the
// few events recorded during an outage costs little. That fallback is cached
// like a real read, so an outage costs one failed read per TTL rather than one
// per event.
func (c *AuditClientInfoCapture) CaptureClientInfo(ctx context.Context) bool {
	c.mu.Lock()
	if cached := c.cached; cached != nil && c.now().Before(cached.readAt.Add(auditCaptureCacheTTL)) {
		c.mu.Unlock()
		return cached.value
	}
	generation := c.generation
	c.mu.Unlock()

	resolved := c.readCapture(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation == generation {
		c.cached = &resolved
	}
	return resolved.value
}

// Invalidate drops the cached switch, so the next event reads it again.
func (c *AuditClientInfoCapture) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cached = nil
	c.generation++
}

// OnInstanceSettingsAudited drops the cached switch when the audit-log
// settings change (#1515). It is registered ahead of the audit recorder's
// observer, so the audit.settings.updated event of a change that switches
// capture off is itself recorded with capture off, like every later event.
func (c *AuditClientInfoCapture) OnInstanceSettingsAudited(_ context.Context, entry models.InstanceSettingsAuditEntry) {
	if entry.Setting == models.InstanceSettingAuditLog {
		c.Invalidate()
	}
}

// readCapture reads the capture switch from the database.
//
// The read is detached from the caller's cancellation, as AuthPolicyResolver's
// refresh is: its result is cached for every caller, so one request that goes
// away mid-read must not turn into a cached "database fault" that strips the
// client info from everyone's events for a whole TTL.
func (c *AuditClientInfoCapture) readCapture(ctx context.Context) cachedAuditCapture {
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), auditCaptureReadTimeout)
	defer cancel()
	stored, err := c.repo.Get(readCtx)
	switch {
	case errors.Is(err, repositories.ErrInstanceAuditSettingsNotFound):
		return cachedAuditCapture{value: models.DefaultAuditCaptureClientInfo, readAt: c.now()}
	case err != nil:
		c.logger.With("error", err).
			Warn("failed to read instance audit settings; not capturing client info")
		return cachedAuditCapture{value: false, readAt: c.now()}
	default:
		return cachedAuditCapture{value: stored.CaptureIPUA, readAt: c.now()}
	}
}
