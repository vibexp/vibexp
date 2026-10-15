package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// AuditPurgeBatchSize is how many audit events one purge DELETE removes. The
// purge loops batch by batch, so a large backlog never holds one long
// statement and its locks.
const AuditPurgeBatchSize = 1000

// AuditEventPurger deletes audit events older than a cutoff, batchSize rows
// per statement, until none remain, and returns how many it deleted.
// repositories.AuditEventRepository satisfies it.
type AuditEventPurger interface {
	PurgeOlderThan(ctx context.Context, cutoff time.Time, batchSize int) (int64, error)
}

// AuditPurgeServiceInterface enforces the audit-log retention (#1512, epic
// #1510).
type AuditPurgeServiceInterface interface {
	// Purge deletes every audit event older than the configured retention.
	// skipped is true, with nothing deleted and no error, when another
	// replica holds the purge lock. A settings read failure is an error and
	// deletes nothing: the purge never guesses a retention window.
	Purge(ctx context.Context) (deleted int64, skipped bool, err error)
}

// AuditPurgeService implements AuditPurgeServiceInterface.
type AuditPurgeService struct {
	settings repositories.InstanceAuditSettingsRepository
	events   AuditEventPurger
	logger   *slog.Logger
	now      func() time.Time
}

var _ AuditPurgeServiceInterface = (*AuditPurgeService)(nil)

// NewAuditPurgeService creates a new AuditPurgeService.
func NewAuditPurgeService(
	settings repositories.InstanceAuditSettingsRepository,
	events AuditEventPurger,
	logger *slog.Logger,
) *AuditPurgeService {
	return &AuditPurgeService{settings: settings, events: events, logger: logger, now: time.Now}
}

// Purge implements AuditPurgeServiceInterface.
//
// It runs under the purge advisory lock, so replicas started together do not
// race the same deletes. The retention is read inside the lock, on every run,
// so a changed retention applies from the next run with no restart.
func (s *AuditPurgeService) Purge(ctx context.Context) (deleted int64, skipped bool, err error) {
	acquired, release, err := s.settings.TryPurgeLock(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("AuditPurgeService.Purge: %w", err)
	}
	if !acquired {
		return 0, true, nil
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			s.logger.With("error", releaseErr.Error()).Warn("failed to release the audit purge lock")
		}
	}()

	values, err := s.retention(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("AuditPurgeService.Purge: %w", err)
	}
	cutoff := s.now().Add(-values.RetentionWindow())
	deleted, err = s.events.PurgeOlderThan(ctx, cutoff, AuditPurgeBatchSize)
	if err != nil {
		// What the completed batches deleted stays deleted; report it.
		return deleted, false, fmt.Errorf("AuditPurgeService.Purge: %w", err)
	}
	return deleted, false, nil
}

// retention reads the settings that decide the window: the built-in defaults
// when none are stored, an error when they cannot be read.
func (s *AuditPurgeService) retention(ctx context.Context) (models.InstanceAuditSettingsValues, error) {
	stored, err := s.settings.Get(ctx)
	switch {
	case errors.Is(err, repositories.ErrInstanceAuditSettingsNotFound):
		return models.DefaultInstanceAuditSettingsValues(), nil
	case err != nil:
		return models.InstanceAuditSettingsValues{}, fmt.Errorf("read audit retention: %w", err)
	default:
		return instanceAuditSettingsValues(stored), nil
	}
}
