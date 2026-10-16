package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrInvalidAuditSettings is returned when a submitted set of audit-log
// settings is rejected. It wraps a *SettingsFieldError naming the field at
// fault.
var ErrInvalidAuditSettings = errors.New("invalid audit settings")

// InstanceAuditSettingsServiceInterface is the instance audit-log settings
// surface (#1512, epic #1510): the admin read and writes. The switch the audit
// recorder consults is AuditClientInfoCapture. Instance-admin authorization is
// the route middleware's job, so no method takes a permission check of its
// own.
type InstanceAuditSettingsServiceInterface interface {
	// Get returns the settings in effect and the stored row's metadata: the
	// built-in defaults, marked IsDefault, when nothing is stored. Unlike
	// AuditClientInfoCapture it returns a repository error, so a caller sees
	// real state.
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
}

// InstanceAuditSettingsService implements
// InstanceAuditSettingsServiceInterface.
//
// Get, Update and Reset read the row on every call. A successful Update or
// Reset drops the cached capture switch, so the change applies to the very
// next event recorded through this process. The change reaches the audit log
// as audit.settings.updated through the instance settings observer (#1515),
// once its instance_settings_audit entry commits.
type InstanceAuditSettingsService struct {
	repo    repositories.InstanceAuditSettingsRepository
	capture auditCaptureInvalidator
	logger  *slog.Logger
}

// auditCaptureInvalidator drops a cached capture switch after a write;
// *AuditClientInfoCapture implements it.
type auditCaptureInvalidator interface {
	Invalidate()
}

var _ InstanceAuditSettingsServiceInterface = (*InstanceAuditSettingsService)(nil)

// NewInstanceAuditSettingsService creates a new InstanceAuditSettingsService.
// capture is the switch the audit recorder consults.
func NewInstanceAuditSettingsService(
	repo repositories.InstanceAuditSettingsRepository,
	capture auditCaptureInvalidator,
	logger *slog.Logger,
) *InstanceAuditSettingsService {
	return &InstanceAuditSettingsService{repo: repo, capture: capture, logger: logger}
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
	s.capture.Invalidate()

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
		s.capture.Invalidate()
	}
	return nil
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
