package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrInvalidSearchSettings is returned when a submitted ranking profile is
// degenerate. Handlers map it to 400.
var ErrInvalidSearchSettings = errors.New("invalid search settings")

// TeamSearchSettingsServiceInterface is the team-level search settings surface.
type TeamSearchSettingsServiceInterface interface {
	// Get returns the settings in effect for the team, reporting whether they
	// come from the team's own profile or from the instance defaults. Readable
	// by any team member, so it takes no permission check of its own — team
	// membership is enforced by the tenancy middleware.
	Get(ctx context.Context, teamID string) (*models.TeamSearchSettingsView, error)
	// Update stores a complete replacement profile for the team. Requires
	// authz.TeamSettingsUpdate; returns an ErrInvalidSearchSettings-wrapped
	// error for a degenerate profile.
	Update(
		ctx context.Context, userID, teamID string, values models.TeamSearchSettingsValues,
	) (*models.TeamSearchSettingsView, error)
	// Reset drops the team's profile so it inherits the instance defaults again.
	// Requires authz.TeamSettingsUpdate. Resetting a team with no profile is a
	// no-op, not an error.
	Reset(ctx context.Context, userID, teamID string) error
}

// TeamSearchSettingsService implements TeamSearchSettingsServiceInterface.
//
// instance resolves the instance ranking defaults per call. They are both the
// fallback for a team with no stored profile and the `instance_defaults`
// reported on every read, so a client can preview a reset without a second
// request.
type TeamSearchSettingsService struct {
	repo     repositories.TeamSearchSettingsRepository
	authz    AuthorizationServiceInterface
	instance InstanceSearchSettingsResolver
	logger   *slog.Logger
}

var _ TeamSearchSettingsServiceInterface = (*TeamSearchSettingsService)(nil)

// NewTeamSearchSettingsService creates a new TeamSearchSettingsService.
func NewTeamSearchSettingsService(
	repo repositories.TeamSearchSettingsRepository,
	authzService AuthorizationServiceInterface,
	instance InstanceSearchSettingsResolver,
	logger *slog.Logger,
) *TeamSearchSettingsService {
	return &TeamSearchSettingsService{
		repo:     repo,
		authz:    authzService,
		instance: instance,
		logger:   logger,
	}
}

// teamSearchSettingsView assembles the response shape from the effective
// values, their source and the instance defaults. The caller resolves the
// instance defaults once per request, so every field of one response comes from
// the same snapshot.
func teamSearchSettingsView(
	source string, values models.TeamSearchSettingsValues, instance models.InstanceSearchSettingsValues,
) *models.TeamSearchSettingsView {
	return &models.TeamSearchSettingsView{
		Source:           source,
		Values:           values,
		InstanceDefaults: instance.TeamValues(),
		// Instance-owned and never team-configurable: it bounds per-query cost
		// for the whole deployment.
		RankCandidateCap: instance.RankCandidateCap,
	}
}

// Get implements TeamSearchSettingsServiceInterface.
func (s *TeamSearchSettingsService) Get(
	ctx context.Context, teamID string,
) (*models.TeamSearchSettingsView, error) {
	stored, err := s.repo.Get(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("TeamSearchSettingsService.Get: %w", err)
	}
	instance := s.instance.Resolve(ctx)
	if stored == nil {
		return teamSearchSettingsView(models.TeamSearchSettingsSourceInstance, instance.TeamValues(), instance), nil
	}
	return teamSearchSettingsView(models.TeamSearchSettingsSourceTeam, valuesFromStored(stored), instance), nil
}

// Update implements TeamSearchSettingsServiceInterface.
func (s *TeamSearchSettingsService) Update(
	ctx context.Context, userID, teamID string, values models.TeamSearchSettingsValues,
) (*models.TeamSearchSettingsView, error) {
	if err := s.authz.Can(ctx, userID, teamID, authz.TeamSettingsUpdate); err != nil {
		return nil, err
	}
	if err := ValidateSearchSettings(values); err != nil {
		return nil, err
	}

	stored := &models.TeamSearchSettings{
		TeamID:                teamID,
		RecencyRankingEnabled: values.RecencyRankingEnabled,
		RankWeightRelevance:   values.RankWeightRelevance,
		RankWeightCreated:     values.RankWeightCreated,
		RankWeightUpdated:     values.RankWeightUpdated,
		RankHalfLifeDays:      values.RankHalfLifeDays,
	}
	if err := s.repo.Upsert(ctx, stored); err != nil {
		return nil, fmt.Errorf("TeamSearchSettingsService.Update: %w", err)
	}

	return teamSearchSettingsView(
		models.TeamSearchSettingsSourceTeam, valuesFromStored(stored), s.instance.Resolve(ctx)), nil
}

// Reset implements TeamSearchSettingsServiceInterface.
func (s *TeamSearchSettingsService) Reset(ctx context.Context, userID, teamID string) error {
	if err := s.authz.Can(ctx, userID, teamID, authz.TeamSettingsUpdate); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, teamID); err != nil {
		return fmt.Errorf("TeamSearchSettingsService.Reset: %w", err)
	}
	return nil
}

// valuesFromStored projects a persisted row onto the wire profile.
func valuesFromStored(stored *models.TeamSearchSettings) models.TeamSearchSettingsValues {
	return models.TeamSearchSettingsValues{
		RecencyRankingEnabled: stored.RecencyRankingEnabled,
		RankWeightRelevance:   stored.RankWeightRelevance,
		RankWeightCreated:     stored.RankWeightCreated,
		RankWeightUpdated:     stored.RankWeightUpdated,
		RankHalfLifeDays:      stored.RankHalfLifeDays,
	}
}

// ValidateSearchSettings rejects a degenerate team ranking profile. The
// team_search_settings CHECK constraints enforce the same bounds in the
// database, so this is the friendly-error layer over a guarantee the schema
// already makes.
func ValidateSearchSettings(v models.TeamSearchSettingsValues) error {
	return validateSearchRankingWeightsAndHalfLife(v)
}

// validateSearchRankingWeightsAndHalfLife holds the rules every ranking profile
// shares, team and instance alike: non-negative weights that are not all zero,
// and a half-life in (0, models.MaxSearchRankHalfLifeDays]. It mirrors
// config.validateSearchRankingConfig bound for bound and reuses its message
// wording, so operators reading startup errors and API clients reading a 400
// see one vocabulary.
func validateSearchRankingWeightsAndHalfLife(v models.TeamSearchSettingsValues) error {
	weights := []float64{v.RankWeightRelevance, v.RankWeightCreated, v.RankWeightUpdated}
	weightFields := []string{"rank_weight_relevance", "rank_weight_created", "rank_weight_updated"}
	var sum float64
	var negative []string
	for i, w := range weights {
		if w < 0 {
			negative = append(negative, weightFields[i])
		}
		sum += w
	}
	if len(negative) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidSearchSettings, settingsFieldError(negative,
			"rank_weight_* must be non-negative, got %v", weights))
	}
	if sum == 0 {
		return fmt.Errorf("%w: %w", ErrInvalidSearchSettings, settingsFieldError(weightFields,
			"rank_weight_* must not all be zero"))
	}
	if v.RankHalfLifeDays <= 0 {
		return fmt.Errorf("%w: %w", ErrInvalidSearchSettings, settingsFieldError(
			[]string{"rank_half_life_days"}, "rank_half_life_days must be positive, got %v", v.RankHalfLifeDays))
	}
	if v.RankHalfLifeDays > models.MaxSearchRankHalfLifeDays {
		return fmt.Errorf("%w: %w", ErrInvalidSearchSettings, settingsFieldError(
			[]string{"rank_half_life_days"}, "rank_half_life_days must be <= %d, got %v",
			models.MaxSearchRankHalfLifeDays, v.RankHalfLifeDays))
	}
	return nil
}
