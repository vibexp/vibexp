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

// BuiltInSearchDefaults returns the instance search ranking defaults in effect
// when no instance_search_settings row is stored. It is the single source of
// truth for them in the services layer.
//
// config.yaml's `search:` block still carries the same numbers in
// config.defaults() until it is removed (#1203); a test pins the two together
// so they cannot drift before then.
func BuiltInSearchDefaults() models.InstanceSearchSettingsValues {
	return models.InstanceSearchSettingsValues{
		RecencyRankingEnabled: false,
		RankWeightRelevance:   0.5,
		RankWeightCreated:     0.3,
		RankWeightUpdated:     0.2,
		RankHalfLifeDays:      90,
		RankCandidateCap:      200,
	}
}

// InstanceSearchSettingsResolver resolves the instance search ranking defaults
// that apply right now.
//
// Resolve deliberately returns no error: it fails open to the built-in defaults
// (see InstanceSearchSettingsService.Resolve).
type InstanceSearchSettingsResolver interface {
	Resolve(ctx context.Context) models.InstanceSearchSettingsValues
}

// InstanceSearchSettingsServiceInterface is the instance-level search settings
// surface an instance admin edits (#1200). Instance-admin authorization is the
// route middleware's job, so no method takes a permission check of its own.
type InstanceSearchSettingsServiceInterface interface {
	InstanceSearchSettingsResolver
	// Get returns the defaults in effect and where they come from. Unlike
	// Resolve it returns a repository error, so an admin sees real state.
	Get(ctx context.Context) (*models.InstanceSearchSettingsView, error)
	// Update validates and stores a complete replacement set of defaults,
	// auditing the change in the same transaction. Invalid input returns an
	// ErrInvalidSearchSettings-wrapped error and writes nothing.
	Update(
		ctx context.Context, actorUserID string, values models.InstanceSearchSettingsValues,
	) (*models.InstanceSearchSettingsView, error)
	// Reset drops the stored defaults so the built-in ones apply again,
	// auditing the change in the same transaction. Resetting with no stored
	// row is a no-op and writes no audit entry.
	Reset(ctx context.Context, actorUserID string) error
}

// InstanceSearchSettingsService implements InstanceSearchSettingsServiceInterface.
//
// Like TeamSearchSettingsResolver it reads the row on every call and caches
// nothing: the lookup is one primary-key read of a singleton row, and a change
// must apply to the very next search on every replica.
type InstanceSearchSettingsService struct {
	repo   repositories.InstanceSearchSettingsRepository
	logger *slog.Logger
}

var _ InstanceSearchSettingsServiceInterface = (*InstanceSearchSettingsService)(nil)

// NewInstanceSearchSettingsService creates a new InstanceSearchSettingsService.
func NewInstanceSearchSettingsService(
	repo repositories.InstanceSearchSettingsRepository,
	logger *slog.Logger,
) *InstanceSearchSettingsService {
	return &InstanceSearchSettingsService{repo: repo, logger: logger}
}

// Resolve implements InstanceSearchSettingsResolver.
//
// It FAILS OPEN: a repository error logs at warn and yields the built-in
// defaults, so a blip reading a settings row never turns a working search into
// a 500. The search query hits the same database moments later, so a genuine
// outage still surfaces there.
func (s *InstanceSearchSettingsService) Resolve(ctx context.Context) models.InstanceSearchSettingsValues {
	stored, err := s.repo.Get(ctx)
	if errors.Is(err, repositories.ErrInstanceSearchSettingsNotFound) {
		return BuiltInSearchDefaults()
	}
	if err != nil {
		s.logger.With("error", err).
			Warn("failed to read instance search settings; falling back to built-in defaults")
		return BuiltInSearchDefaults()
	}
	return instanceSearchValuesFromStored(stored)
}

// Get implements InstanceSearchSettingsServiceInterface.
func (s *InstanceSearchSettingsService) Get(ctx context.Context) (*models.InstanceSearchSettingsView, error) {
	stored, err := s.repo.Get(ctx)
	if errors.Is(err, repositories.ErrInstanceSearchSettingsNotFound) {
		return &models.InstanceSearchSettingsView{
			Source: models.InstanceSearchSettingsSourceDefault,
			Values: BuiltInSearchDefaults(),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("InstanceSearchSettingsService.Get: %w", err)
	}
	return instanceSearchView(stored), nil
}

// Update implements InstanceSearchSettingsServiceInterface.
func (s *InstanceSearchSettingsService) Update(
	ctx context.Context, actorUserID string, values models.InstanceSearchSettingsValues,
) (*models.InstanceSearchSettingsView, error) {
	if err := ValidateInstanceSearchSettings(values); err != nil {
		return nil, err
	}

	stored := &models.InstanceSearchSettings{
		RecencyRankingEnabled: values.RecencyRankingEnabled,
		RankWeightRelevance:   values.RankWeightRelevance,
		RankWeightCreated:     values.RankWeightCreated,
		RankWeightUpdated:     values.RankWeightUpdated,
		RankHalfLifeDays:      values.RankHalfLifeDays,
		RankCandidateCap:      values.RankCandidateCap,
		UpdatedBy:             optionalActor(actorUserID),
	}
	err := s.repo.UpsertAudited(ctx, stored,
		instanceSearchAuditFunc(models.InstanceSettingsAuditActionUpsert, actorUserID))
	if err != nil {
		return nil, fmt.Errorf("InstanceSearchSettingsService.Update: %w", err)
	}

	return instanceSearchView(stored), nil
}

// Reset implements InstanceSearchSettingsServiceInterface.
func (s *InstanceSearchSettingsService) Reset(ctx context.Context, actorUserID string) error {
	_, err := s.repo.DeleteAudited(ctx,
		instanceSearchAuditFunc(models.InstanceSettingsAuditActionDelete, actorUserID))
	if err != nil {
		return fmt.Errorf("InstanceSearchSettingsService.Reset: %w", err)
	}
	return nil
}

// instanceSearchAuditFunc builds the audit entry for one change. The snapshots
// hold only the ranking values: the table stores no credential, so there is
// nothing to redact.
func instanceSearchAuditFunc(action, actorUserID string) repositories.InstanceSearchSettingsAuditFunc {
	return func(before, after *models.InstanceSearchSettings) (*models.InstanceSettingsAuditEntry, error) {
		beforeDoc, err := instanceSearchAuditSnapshot(before)
		if err != nil {
			return nil, err
		}
		afterDoc, err := instanceSearchAuditSnapshot(after)
		if err != nil {
			return nil, err
		}
		return &models.InstanceSettingsAuditEntry{
			Setting:     models.InstanceSettingSearch,
			Action:      action,
			ActorUserID: optionalActor(actorUserID),
			Before:      beforeDoc,
			After:       afterDoc,
		}, nil
	}
}

// instanceSearchAuditSnapshot renders a row as its values, or nil for no row.
func instanceSearchAuditSnapshot(row *models.InstanceSearchSettings) (json.RawMessage, error) {
	if row == nil {
		return nil, nil
	}
	doc, err := json.Marshal(instanceSearchValuesFromStored(row))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal instance search settings snapshot: %w", err)
	}
	return doc, nil
}

// instanceSearchView renders a stored row as the read model.
func instanceSearchView(stored *models.InstanceSearchSettings) *models.InstanceSearchSettingsView {
	updatedAt := stored.UpdatedAt
	return &models.InstanceSearchSettingsView{
		Source:    models.InstanceSearchSettingsSourceInstance,
		Values:    instanceSearchValuesFromStored(stored),
		UpdatedAt: &updatedAt,
		UpdatedBy: stored.UpdatedBy,
	}
}

// instanceSearchValuesFromStored projects a persisted row onto its values.
func instanceSearchValuesFromStored(stored *models.InstanceSearchSettings) models.InstanceSearchSettingsValues {
	return models.InstanceSearchSettingsValues{
		RecencyRankingEnabled: stored.RecencyRankingEnabled,
		RankWeightRelevance:   stored.RankWeightRelevance,
		RankWeightCreated:     stored.RankWeightCreated,
		RankWeightUpdated:     stored.RankWeightUpdated,
		RankHalfLifeDays:      stored.RankHalfLifeDays,
		RankCandidateCap:      stored.RankCandidateCap,
	}
}

// ValidateInstanceSearchSettings rejects degenerate instance ranking defaults:
// the weight and half-life rules every ranking profile shares (see
// validateSearchRankingWeightsAndHalfLife) plus the instance-only candidate cap
// in [1, models.MaxSearchRankCandidateCap]. It mirrors
// config.validateSearchRankingConfig bound for bound, and is exported for the
// boot-time config import (#1201). The instance_search_settings CHECK
// constraints enforce the same bounds in the database.
func ValidateInstanceSearchSettings(v models.InstanceSearchSettingsValues) error {
	if err := validateSearchRankingWeightsAndHalfLife(v.TeamValues()); err != nil {
		return err
	}
	if v.RankCandidateCap < 1 {
		return fmt.Errorf("%w: rank_candidate_cap must be >= 1, got %d",
			ErrInvalidSearchSettings, v.RankCandidateCap)
	}
	if v.RankCandidateCap > models.MaxSearchRankCandidateCap {
		return fmt.Errorf("%w: rank_candidate_cap must be <= %d, got %d",
			ErrInvalidSearchSettings, models.MaxSearchRankCandidateCap, v.RankCandidateCap)
	}
	return nil
}
