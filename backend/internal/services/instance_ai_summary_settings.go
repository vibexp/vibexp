package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrInvalidInstanceAISummarySettings is returned when submitted instance AI
// summary settings are outside the hard limits. The admin endpoint (#1200) maps
// it to 422.
var ErrInvalidInstanceAISummarySettings = errors.New("invalid instance AI summary settings")

// InstanceAISummarySettingsResolver resolves the instance AI summary defaults
// and budgets that apply right now.
//
// Resolve deliberately returns no error: it fails open to the built-in defaults
// (see InstanceAISummarySettingsService.Resolve).
type InstanceAISummarySettingsResolver interface {
	Resolve(ctx context.Context) models.InstanceAISummarySettingsValues
}

// InstanceAISummarySettingsReader is the read half of the instance settings:
// the fail-open Resolve plus the fail-closed Get. The team settings service
// holds it, so its own fail-closed reads (the settings API) can report a failed
// instance read instead of a guess.
type InstanceAISummarySettingsReader interface {
	InstanceAISummarySettingsResolver
	// Get returns the settings in effect and where they come from. Unlike
	// Resolve it returns a repository error, so a caller sees real state.
	Get(ctx context.Context) (*models.InstanceAISummarySettingsView, error)
}

// InstanceAISummarySettingsServiceInterface is the instance-level AI summary
// settings surface an instance admin edits (#1200). Instance-admin
// authorization is the route middleware's job, so no method takes a permission
// check of its own.
type InstanceAISummarySettingsServiceInterface interface {
	InstanceAISummarySettingsReader
	// Update validates and stores a complete replacement set of settings,
	// auditing the change in the same transaction. Invalid input returns an
	// ErrInvalidInstanceAISummarySettings-wrapped error and writes nothing.
	Update(
		ctx context.Context, actorUserID string, values models.InstanceAISummarySettingsValues,
	) (*models.InstanceAISummarySettingsView, error)
	// Reset drops the stored settings so the built-in ones apply again,
	// auditing the change in the same transaction. Resetting with no stored
	// row is a no-op and writes no audit entry.
	Reset(ctx context.Context, actorUserID string) error
}

// InstanceAISummarySettingsService implements
// InstanceAISummarySettingsServiceInterface.
//
// It reads the row on every call and caches nothing: the lookup is one
// primary-key read of a singleton row, negligible next to the LLM call it
// tunes, and a change must apply to the very next summary on every replica.
type InstanceAISummarySettingsService struct {
	repo   repositories.InstanceAISummarySettingsRepository
	logger *slog.Logger
}

var _ InstanceAISummarySettingsServiceInterface = (*InstanceAISummarySettingsService)(nil)

// NewInstanceAISummarySettingsService creates a new InstanceAISummarySettingsService.
func NewInstanceAISummarySettingsService(
	repo repositories.InstanceAISummarySettingsRepository,
	logger *slog.Logger,
) *InstanceAISummarySettingsService {
	return &InstanceAISummarySettingsService{repo: repo, logger: logger}
}

// Resolve implements InstanceAISummarySettingsResolver.
//
// It FAILS OPEN: a repository error logs at warn and yields the built-in
// defaults, so a blip reading a settings row never fails a summary or a search
// response. Every caller hits the same database moments later, so a genuine
// outage still surfaces there.
func (s *InstanceAISummarySettingsService) Resolve(ctx context.Context) models.InstanceAISummarySettingsValues {
	stored, err := s.repo.Get(ctx)
	if errors.Is(err, repositories.ErrInstanceAISummarySettingsNotFound) {
		return models.DefaultInstanceAISummarySettings()
	}
	if err != nil {
		s.logger.With("error", err).
			Warn("failed to read instance AI summary settings; falling back to built-in defaults")
		return models.DefaultInstanceAISummarySettings()
	}
	return instanceAISummaryValuesFromStored(stored)
}

// Get implements InstanceAISummarySettingsServiceInterface.
func (s *InstanceAISummarySettingsService) Get(ctx context.Context) (*models.InstanceAISummarySettingsView, error) {
	stored, err := s.repo.Get(ctx)
	if errors.Is(err, repositories.ErrInstanceAISummarySettingsNotFound) {
		return &models.InstanceAISummarySettingsView{
			Source: models.InstanceAISummarySettingsSourceDefault,
			Values: models.DefaultInstanceAISummarySettings(),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("InstanceAISummarySettingsService.Get: %w", err)
	}
	return instanceAISummaryView(stored), nil
}

// Update implements InstanceAISummarySettingsServiceInterface.
func (s *InstanceAISummarySettingsService) Update(
	ctx context.Context, actorUserID string, values models.InstanceAISummarySettingsValues,
) (*models.InstanceAISummarySettingsView, error) {
	if err := ValidateInstanceAISummarySettings(values); err != nil {
		return nil, err
	}

	stored := &models.InstanceAISummarySettings{
		Enabled:           values.Enabled,
		TopN:              values.TopN,
		Style:             values.Style,
		MaxOutputTokens:   values.MaxOutputTokens,
		PerDocumentChars:  values.PerDocumentChars,
		TotalContextChars: values.TotalContextChars,
		RequestTimeout:    values.RequestTimeout,
		UpdatedBy:         optionalActor(actorUserID),
	}
	err := s.repo.UpsertAudited(ctx, stored,
		instanceAISummaryAuditFunc(models.InstanceSettingsAuditActionUpsert, actorUserID))
	if err != nil {
		return nil, fmt.Errorf("InstanceAISummarySettingsService.Update: %w", err)
	}

	return instanceAISummaryView(stored), nil
}

// Reset implements InstanceAISummarySettingsServiceInterface.
func (s *InstanceAISummarySettingsService) Reset(ctx context.Context, actorUserID string) error {
	_, err := s.repo.DeleteAudited(ctx,
		instanceAISummaryAuditFunc(models.InstanceSettingsAuditActionDelete, actorUserID))
	if err != nil {
		return fmt.Errorf("InstanceAISummarySettingsService.Reset: %w", err)
	}
	return nil
}

// instanceAISummaryAuditFunc builds the audit entry for one change. The
// snapshots hold only the settings values: the table stores no credential, so
// there is nothing to redact.
func instanceAISummaryAuditFunc(action, actorUserID string) repositories.InstanceAISummarySettingsAuditFunc {
	return func(before, after *models.InstanceAISummarySettings) (*models.InstanceSettingsAuditEntry, error) {
		beforeDoc, err := instanceAISummaryAuditSnapshot(before)
		if err != nil {
			return nil, err
		}
		afterDoc, err := instanceAISummaryAuditSnapshot(after)
		if err != nil {
			return nil, err
		}
		return &models.InstanceSettingsAuditEntry{
			Setting:     models.InstanceSettingAISummary,
			Action:      action,
			ActorUserID: optionalActor(actorUserID),
			Before:      beforeDoc,
			After:       afterDoc,
		}, nil
	}
}

// instanceAISummaryAuditValues is the audit snapshot of the instance AI summary
// settings. It records the timeout as request_timeout_ms, the unit the table
// stores, rather than time.Duration's bare nanosecond JSON form: audit rows are
// kept permanently, so their format must not change later.
type instanceAISummaryAuditValues struct {
	Enabled           bool   `json:"enabled"`
	TopN              int    `json:"top_n"`
	Style             string `json:"style"`
	MaxOutputTokens   int    `json:"max_output_tokens"`
	PerDocumentChars  int    `json:"per_document_chars"`
	TotalContextChars int    `json:"total_context_chars"`
	RequestTimeoutMS  int64  `json:"request_timeout_ms"`
}

// instanceAISummaryAuditSnapshot renders a row as its audit values, or nil for
// no row.
func instanceAISummaryAuditSnapshot(row *models.InstanceAISummarySettings) (json.RawMessage, error) {
	if row == nil {
		return nil, nil
	}
	doc, err := json.Marshal(instanceAISummaryAuditValues{
		Enabled:           row.Enabled,
		TopN:              row.TopN,
		Style:             row.Style,
		MaxOutputTokens:   row.MaxOutputTokens,
		PerDocumentChars:  row.PerDocumentChars,
		TotalContextChars: row.TotalContextChars,
		RequestTimeoutMS:  row.RequestTimeout.Milliseconds(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal instance AI summary settings snapshot: %w", err)
	}
	return doc, nil
}

// instanceAISummaryView renders a stored row as the read model.
func instanceAISummaryView(stored *models.InstanceAISummarySettings) *models.InstanceAISummarySettingsView {
	updatedAt := stored.UpdatedAt
	return &models.InstanceAISummarySettingsView{
		Source:    models.InstanceAISummarySettingsSourceInstance,
		Values:    instanceAISummaryValuesFromStored(stored),
		UpdatedAt: &updatedAt,
		UpdatedBy: stored.UpdatedBy,
	}
}

// instanceAISummaryValuesFromStored projects a persisted row onto its values.
func instanceAISummaryValuesFromStored(
	stored *models.InstanceAISummarySettings,
) models.InstanceAISummarySettingsValues {
	return models.InstanceAISummarySettingsValues{
		Enabled:           stored.Enabled,
		TopN:              stored.TopN,
		Style:             stored.Style,
		MaxOutputTokens:   stored.MaxOutputTokens,
		PerDocumentChars:  stored.PerDocumentChars,
		TotalContextChars: stored.TotalContextChars,
		RequestTimeout:    stored.RequestTimeout,
	}
}

// ValidateInstanceAISummarySettings rejects instance AI summary settings that
// could not be honoured: top_n in [1, models.MaxAISummaryTopN], max_output_tokens
// in [1, models.MaxAISummaryOutputTokens], a positive per-document budget, a
// total context budget of at least one document's, a request timeout of at
// least 1ms (it is stored in whole milliseconds) and a style from
// models.AISummaryStyles. The upper bounds on the budgets and the timeout are
// what the 32-bit storage columns can hold. The instance_ai_summary_settings
// CHECK constraints (migration 022) enforce the same bounds in the database.
// It is exported for the boot-time config import (#1201).
func ValidateInstanceAISummarySettings(v models.InstanceAISummarySettingsValues) error {
	if err := validateAISummaryProfileBounds(v.TeamValues()); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInstanceAISummarySettings, err)
	}
	if v.PerDocumentChars < 1 || v.PerDocumentChars > math.MaxInt32 {
		return fmt.Errorf("%w: per_document_chars must be between 1 and %d, got %d",
			ErrInvalidInstanceAISummarySettings, math.MaxInt32, v.PerDocumentChars)
	}
	if v.TotalContextChars < v.PerDocumentChars || v.TotalContextChars > math.MaxInt32 {
		return fmt.Errorf("%w: total_context_chars must be between per_document_chars (%d) and %d, got %d",
			ErrInvalidInstanceAISummarySettings, v.PerDocumentChars, math.MaxInt32, v.TotalContextChars)
	}
	return validateInstanceAISummaryRequestTimeout(v.RequestTimeout)
}

// maxInstanceAISummaryRequestTimeout is the longest timeout request_timeout_ms
// (a 32-bit integer column) can hold.
const maxInstanceAISummaryRequestTimeout = time.Duration(math.MaxInt32) * time.Millisecond

// validateInstanceAISummaryRequestTimeout requires a timeout the storage column
// can represent: it is stored in whole milliseconds with CHECK (>= 1), so a
// positive but sub-millisecond value would truncate to 0 and fail the write.
func validateInstanceAISummaryRequestTimeout(timeout time.Duration) error {
	if timeout < time.Millisecond || timeout > maxInstanceAISummaryRequestTimeout {
		return fmt.Errorf("%w: request_timeout must be between 1ms and %s, got %s",
			ErrInvalidInstanceAISummarySettings, maxInstanceAISummaryRequestTimeout, timeout)
	}
	return nil
}
