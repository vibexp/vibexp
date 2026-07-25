package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/config"
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
// defaults is the deployment-wide `search:` config. It is both the fallback for
// a team with no stored profile and the `instance_defaults` reported on every
// read, so a client can preview a reset without a second request.
type TeamSearchSettingsService struct {
	repo     repositories.TeamSearchSettingsRepository
	authz    AuthorizationServiceInterface
	defaults config.SearchConfig
	logger   *slog.Logger
}

var _ TeamSearchSettingsServiceInterface = (*TeamSearchSettingsService)(nil)

// NewTeamSearchSettingsService creates a new TeamSearchSettingsService.
func NewTeamSearchSettingsService(
	repo repositories.TeamSearchSettingsRepository,
	authzService AuthorizationServiceInterface,
	defaults config.SearchConfig,
	logger *slog.Logger,
) *TeamSearchSettingsService {
	return &TeamSearchSettingsService{
		repo:     repo,
		authz:    authzService,
		defaults: defaults,
		logger:   logger,
	}
}

// instanceValues renders the deployment defaults as a profile.
func (s *TeamSearchSettingsService) instanceValues() models.TeamSearchSettingsValues {
	return models.TeamSearchSettingsValues{
		RecencyRankingEnabled: s.defaults.RecencyRankingEnabled,
		RankWeightRelevance:   s.defaults.RankWeightRelevance,
		RankWeightCreated:     s.defaults.RankWeightCreated,
		RankWeightUpdated:     s.defaults.RankWeightUpdated,
		RankHalfLifeDays:      s.defaults.RankHalfLifeDays,
	}
}

// view assembles the response shape from the effective values and their source.
func (s *TeamSearchSettingsService) view(
	source string, values models.TeamSearchSettingsValues,
) *models.TeamSearchSettingsView {
	return &models.TeamSearchSettingsView{
		Source:           source,
		Values:           values,
		InstanceDefaults: s.instanceValues(),
		// Instance-owned and never team-configurable: it bounds per-query cost
		// for the whole deployment.
		RankCandidateCap: s.defaults.RankCandidateCap,
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
	if stored == nil {
		return s.view(models.TeamSearchSettingsSourceInstance, s.instanceValues()), nil
	}
	return s.view(models.TeamSearchSettingsSourceTeam, valuesFromStored(stored)), nil
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

	return s.view(models.TeamSearchSettingsSourceTeam, valuesFromStored(stored)), nil
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

// ValidateSearchSettings rejects a degenerate ranking profile, mirroring
// config.validateSearchRankingConfig bound for bound and reusing its message
// wording so operators reading startup errors and API clients reading a 400 see
// one vocabulary. The team_search_settings CHECK constraints enforce the same
// bounds in the database, so this is the friendly-error layer over a guarantee
// the schema already makes.
func ValidateSearchSettings(v models.TeamSearchSettingsValues) error {
	weights := []float64{v.RankWeightRelevance, v.RankWeightCreated, v.RankWeightUpdated}
	var sum float64
	for _, w := range weights {
		if w < 0 {
			return fmt.Errorf("%w: rank_weight_* must be non-negative, got %v",
				ErrInvalidSearchSettings, weights)
		}
		sum += w
	}
	if sum == 0 {
		return fmt.Errorf("%w: rank_weight_* must not all be zero", ErrInvalidSearchSettings)
	}
	if v.RankHalfLifeDays <= 0 {
		return fmt.Errorf("%w: rank_half_life_days must be positive, got %v",
			ErrInvalidSearchSettings, v.RankHalfLifeDays)
	}
	if v.RankHalfLifeDays > config.MaxSearchRankHalfLifeDays {
		return fmt.Errorf("%w: rank_half_life_days must be <= %d, got %v",
			ErrInvalidSearchSettings, config.MaxSearchRankHalfLifeDays, v.RankHalfLifeDays)
	}
	return nil
}
