package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrInvalidAuditSettings is returned when a submitted set of audit-log
// settings is rejected. It wraps a *SettingsFieldError naming the field at
// fault.
var ErrInvalidAuditSettings = errors.New("invalid audit settings")

// auditCaptureCacheTTL is how long a resolved client-info capture switch is
// served from memory. A write through this process drops the cached value at
// once; another replica picks the change up within this long.
const auditCaptureCacheTTL = 30 * time.Second

// auditCaptureReadTimeout bounds one read of the capture switch. The read runs
// detached from the caller's cancellation (see readCapture), so this is what
// keeps a stuck database from holding the recorder.
const auditCaptureReadTimeout = 5 * time.Second

// InstanceAuditSettingsServiceInterface is the instance audit-log settings
// surface (#1512, epic #1510): the admin read and writes, and the capture
// switch the audit recorder consults. Instance-admin authorization is the route
// middleware's job, so no method takes a permission check of its own.
type InstanceAuditSettingsServiceInterface interface {
	// Get returns the settings in effect and the stored row's metadata: the
	// built-in defaults, marked IsDefault, when nothing is stored. Unlike
	// CaptureClientInfo it returns a repository error, so a caller sees real
	// state.
	Get(ctx context.Context) (*models.InstanceAuditSettingsView, error)
	// Update validates and stores a complete replacement set of settings,
	// auditing the change in the same transaction. Invalid input returns an
	// ErrInvalidAuditSettings-wrapped error (carrying a *SettingsFieldError)
	// and writes nothing. A non-nil expectedVersion makes it a compare-and-set
	// (repositories.InstanceSettingsNoStoredVersion expects nothing stored): a
	// mismatch returns repositories.ErrInstanceSettingsVersionConflict and
	// writes nothing.
	Update(
		ctx context.Context, actorUserID string, values models.InstanceAuditSettingsValues, expectedVersion *int64,
	) (*models.InstanceAuditSettingsView, error)
	// Reset drops the stored settings so the built-in defaults apply again,
	// auditing the change in the same transaction. Resetting with nothing
	// stored is a no-op and writes no audit entry.
	Reset(ctx context.Context, actorUserID string) error
	// CaptureClientInfo reports whether a newly recorded audit event stores
	// the client IP address and user agent. It never fails: see
	// InstanceAuditSettingsService.CaptureClientInfo.
	CaptureClientInfo(ctx context.Context) bool
}

// InstanceAuditSettingsService implements
// InstanceAuditSettingsServiceInterface.
//
// Get, Update and Reset read the row on every call. CaptureClientInfo runs on
// every recorded audit event, so it serves a cached value for up to
// auditCaptureCacheTTL; Update and Reset drop the cache, so a change made
// through this process applies to the very next event.
type InstanceAuditSettingsService struct {
	repo   repositories.InstanceAuditSettingsRepository
	logger *slog.Logger
	now    func() time.Time

	mu sync.Mutex
	// capture is the cached capture switch; nil when nothing is cached.
	capture *cachedAuditCapture
	// generation is bumped by every write, so a read that started before a
	// write cannot cache what it read after the write dropped the cache.
	generation uint64
}

// cachedAuditCapture is one resolved capture switch and when it was read.
type cachedAuditCapture struct {
	value  bool
	readAt time.Time
}

var _ InstanceAuditSettingsServiceInterface = (*InstanceAuditSettingsService)(nil)

// NewInstanceAuditSettingsService creates a new InstanceAuditSettingsService.
func NewInstanceAuditSettingsService(
	repo repositories.InstanceAuditSettingsRepository,
	logger *slog.Logger,
) *InstanceAuditSettingsService {
	return &InstanceAuditSettingsService{repo: repo, logger: logger, now: time.Now}
}

// Get implements InstanceAuditSettingsServiceInterface.
func (s *InstanceAuditSettingsService) Get(ctx context.Context) (*models.InstanceAuditSettingsView, error) {
	stored, err := s.repo.Get(ctx)
	if errors.Is(err, repositories.ErrInstanceAuditSettingsNotFound) {
		return &models.InstanceAuditSettingsView{
			Values:    models.DefaultInstanceAuditSettingsValues(),
			IsDefault: true,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("InstanceAuditSettingsService.Get: %w", err)
	}
	return instanceAuditSettingsView(stored), nil
}

// Update implements InstanceAuditSettingsServiceInterface.
func (s *InstanceAuditSettingsService) Update(
	ctx context.Context, actorUserID string, values models.InstanceAuditSettingsValues, expectedVersion *int64,
) (*models.InstanceAuditSettingsView, error) {
	if err := ValidateInstanceAuditSettings(values); err != nil {
		return nil, err
	}

	stored := &models.InstanceAuditSettings{
		RetentionWeeks: values.RetentionWeeks,
		CaptureIPUA:    values.CaptureIPUA,
		UpdatedBy:      optionalActor(actorUserID),
	}
	err := s.repo.UpsertAudited(ctx, stored, expectedVersion,
		instanceAuditSettingsAuditFunc(models.InstanceSettingsAuditActionUpsert, actorUserID))
	if err != nil {
		return nil, fmt.Errorf("InstanceAuditSettingsService.Update: %w", err)
	}
	s.invalidateCapture()

	return instanceAuditSettingsView(stored), nil
}

// Reset implements InstanceAuditSettingsServiceInterface.
func (s *InstanceAuditSettingsService) Reset(ctx context.Context, actorUserID string) error {
	deleted, err := s.repo.DeleteAudited(ctx,
		instanceAuditSettingsAuditFunc(models.InstanceSettingsAuditActionDelete, actorUserID))
	if err != nil {
		return fmt.Errorf("InstanceAuditSettingsService.Reset: %w", err)
	}
	if deleted {
		s.invalidateCapture()
	}
	return nil
}

// CaptureClientInfo implements InstanceAuditSettingsServiceInterface.
//
// With nothing stored it returns models.DefaultAuditCaptureClientInfo. When
// the settings cannot be read it FAILS CLOSED to false (store no IP address or
// user agent): an admin who switched capture off must never have it silently
// switched back on by a database blip, while losing the client info of the
// few events recorded during an outage costs little. That fallback is cached
// like a real read, so an outage costs one failed read per TTL rather than one
// per event.
func (s *InstanceAuditSettingsService) CaptureClientInfo(ctx context.Context) bool {
	s.mu.Lock()
	if c := s.capture; c != nil && s.now().Before(c.readAt.Add(auditCaptureCacheTTL)) {
		s.mu.Unlock()
		return c.value
	}
	generation := s.generation
	s.mu.Unlock()

	resolved := s.readCapture(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation == generation {
		s.capture = &resolved
	}
	return resolved.value
}

// readCapture reads the capture switch from the database.
//
// The read is detached from the caller's cancellation, as AuthPolicyResolver's
// refresh is: its result is cached for every caller, so one request that goes
// away mid-read must not turn into a cached "database fault" that strips the
// client info from everyone's events for a whole TTL.
func (s *InstanceAuditSettingsService) readCapture(ctx context.Context) cachedAuditCapture {
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), auditCaptureReadTimeout)
	defer cancel()
	stored, err := s.repo.Get(readCtx)
	switch {
	case errors.Is(err, repositories.ErrInstanceAuditSettingsNotFound):
		return cachedAuditCapture{value: models.DefaultAuditCaptureClientInfo, readAt: s.now()}
	case err != nil:
		s.logger.With("error", err).
			Warn("failed to read instance audit settings; not capturing client info")
		return cachedAuditCapture{value: false, readAt: s.now()}
	default:
		return cachedAuditCapture{value: stored.CaptureIPUA, readAt: s.now()}
	}
}

// invalidateCapture drops the cached capture switch after a write.
func (s *InstanceAuditSettingsService) invalidateCapture() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.capture = nil
	s.generation++
}

// instanceAuditSettingsAuditFunc builds the instance_settings_audit entry for
// one change. The snapshots hold the two values as-is: the table stores no
// credential, so there is nothing to redact.
func instanceAuditSettingsAuditFunc(action, actorUserID string) repositories.InstanceAuditSettingsAuditFunc {
	return func(before, after *models.InstanceAuditSettings) (*models.InstanceSettingsAuditEntry, error) {
		beforeDoc, err := instanceAuditSettingsSnapshot(before)
		if err != nil {
			return nil, err
		}
		afterDoc, err := instanceAuditSettingsSnapshot(after)
		if err != nil {
			return nil, err
		}
		return &models.InstanceSettingsAuditEntry{
			Setting:     models.InstanceSettingAuditLog,
			Action:      action,
			ActorUserID: optionalActor(actorUserID),
			Before:      beforeDoc,
			After:       afterDoc,
		}, nil
	}
}

// instanceAuditSettingsSnapshot renders a row as its values, or nil for no row.
func instanceAuditSettingsSnapshot(row *models.InstanceAuditSettings) (json.RawMessage, error) {
	if row == nil {
		return nil, nil
	}
	doc, err := json.Marshal(instanceAuditSettingsValues(row))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal instance audit settings snapshot: %w", err)
	}
	return doc, nil
}

// instanceAuditSettingsView renders a stored row as the read model.
func instanceAuditSettingsView(stored *models.InstanceAuditSettings) *models.InstanceAuditSettingsView {
	updatedAt := stored.UpdatedAt
	version := stored.Version
	return &models.InstanceAuditSettingsView{
		Values:    instanceAuditSettingsValues(stored),
		UpdatedAt: &updatedAt,
		UpdatedBy: stored.UpdatedBy,
		Version:   &version,
	}
}

// instanceAuditSettingsValues projects a persisted row onto its values.
func instanceAuditSettingsValues(stored *models.InstanceAuditSettings) models.InstanceAuditSettingsValues {
	return models.InstanceAuditSettingsValues{
		RetentionWeeks: stored.RetentionWeeks,
		CaptureIPUA:    stored.CaptureIPUA,
	}
}

// ValidateInstanceAuditSettings checks a set of audit-log settings:
// retention_weeks must lie in models.MinAuditRetentionWeeks to
// models.MaxAuditRetentionWeeks. capture_ip_ua is a boolean and needs no check.
// The instance_audit_settings CHECK constraint enforces the same bounds in the
// database.
func ValidateInstanceAuditSettings(v models.InstanceAuditSettingsValues) error {
	if v.RetentionWeeks < models.MinAuditRetentionWeeks || v.RetentionWeeks > models.MaxAuditRetentionWeeks {
		return fmt.Errorf("%w: %w", ErrInvalidAuditSettings, settingsFieldError(
			[]string{"retention_weeks"}, "retention_weeks must be between %d and %d, got %d",
			models.MinAuditRetentionWeeks, models.MaxAuditRetentionWeeks, v.RetentionWeeks))
	}
	return nil
}
