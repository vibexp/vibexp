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

// ErrInvalidAISummarySettings is returned when a submitted summary profile is
// outside the bounds the instance allows. Handlers map it to 400.
var ErrInvalidAISummarySettings = errors.New("invalid AI summary settings")

// TeamAISummarySettingsServiceInterface is the team-level AI summary settings
// surface.
type TeamAISummarySettingsServiceInterface interface {
	// Resolve returns the settings in effect for the team, reporting whether
	// they come from the team's own profile or from the instance defaults.
	//
	// It FAILS OPEN — see TeamAISummarySettingsService.Resolve.
	Resolve(ctx context.Context, teamID string) (*models.TeamAISummarySettingsView, error)
	// Update stores a complete replacement profile for the team. Requires
	// authz.TeamSettingsUpdate; returns an ErrInvalidAISummarySettings-wrapped
	// error for a profile outside the instance bounds.
	Update(
		ctx context.Context, userID, teamID string, values models.TeamAISummarySettingsValues,
	) (*models.TeamAISummarySettingsView, error)
	// Reset drops the team's profile so it inherits the instance defaults again.
	// Requires authz.TeamSettingsUpdate. Resetting a team with no profile is a
	// no-op, not an error.
	Reset(ctx context.Context, userID, teamID string) error
}

// TeamAISummarySettingsService implements TeamAISummarySettingsServiceInterface.
//
// defaults is the deployment-wide `ai_summary:` config. It is both the fallback
// for a team with no stored profile and the instance_defaults reported on every
// read, so a client can preview a reset without a second request.
type TeamAISummarySettingsService struct {
	repo     repositories.TeamAISummarySettingsRepository
	authz    AuthorizationServiceInterface
	defaults config.AISummaryConfig
	logger   *slog.Logger
}

var _ TeamAISummarySettingsServiceInterface = (*TeamAISummarySettingsService)(nil)

// NewTeamAISummarySettingsService creates a new TeamAISummarySettingsService.
func NewTeamAISummarySettingsService(
	repo repositories.TeamAISummarySettingsRepository,
	authzService AuthorizationServiceInterface,
	defaults config.AISummaryConfig,
	logger *slog.Logger,
) *TeamAISummarySettingsService {
	return &TeamAISummarySettingsService{
		repo:     repo,
		authz:    authzService,
		defaults: defaults,
		logger:   logger,
	}
}

// instanceValues renders the deployment defaults as a profile.
//
// ModelProviderID is always nil: the instance has no opinion on which of a
// team's model providers to use, so inheriting means "the team default".
func (s *TeamAISummarySettingsService) instanceValues() models.TeamAISummarySettingsValues {
	return models.TeamAISummarySettingsValues{
		Enabled:         bool(s.defaults.Enabled),
		ModelProviderID: nil,
		TopN:            int(s.defaults.TopN),
		Style:           s.defaults.Style,
		MaxOutputTokens: s.defaults.MaxOutputTokens,
	}
}

// view assembles the response shape from the effective values and their source.
func (s *TeamAISummarySettingsService) view(
	source string, values models.TeamAISummarySettingsValues,
) *models.TeamAISummarySettingsView {
	return &models.TeamAISummarySettingsView{
		Source:           source,
		Values:           values,
		InstanceDefaults: s.instanceValues(),
		// Instance-owned and never team-configurable: it bounds how much context
		// a single summary request may assemble.
		MaxTopN: s.defaults.MaxTopN,
	}
}

// Resolve implements TeamAISummarySettingsServiceInterface.
//
// It FAILS OPEN: a repository error logs at warn and yields the instance
// defaults instead of an error. Summarisation is a tuning surface over work the
// caller is doing anyway, so a blip reading one settings row must degrade the
// tuning rather than fail the request that asked for the summary. The error is
// kept in the signature because the settings API (#1072) is the opposite — it
// must report a failed read — and both read through this one method.
func (s *TeamAISummarySettingsService) Resolve(
	ctx context.Context, teamID string,
) (*models.TeamAISummarySettingsView, error) {
	stored, err := s.repo.Get(ctx, teamID)
	if err != nil {
		s.logger.With("team_id", teamID, "error", err).
			Warn("failed to read team AI summary settings; falling back to instance defaults")
		return s.view(models.TeamAISummarySettingsSourceInstance, s.instanceValues()), nil
	}
	if stored == nil {
		// No override row: the team inherits the instance defaults entirely.
		return s.view(models.TeamAISummarySettingsSourceInstance, s.instanceValues()), nil
	}
	return s.view(models.TeamAISummarySettingsSourceTeam, aiSummaryValuesFromStored(stored)), nil
}

// Update implements TeamAISummarySettingsServiceInterface.
func (s *TeamAISummarySettingsService) Update(
	ctx context.Context, userID, teamID string, values models.TeamAISummarySettingsValues,
) (*models.TeamAISummarySettingsView, error) {
	if err := s.authz.Can(ctx, userID, teamID, authz.TeamSettingsUpdate); err != nil {
		return nil, err
	}
	if err := ValidateAISummarySettings(values, s.defaults.MaxTopN); err != nil {
		return nil, err
	}

	stored := &models.TeamAISummarySettings{
		TeamID:          teamID,
		Enabled:         values.Enabled,
		ModelProviderID: values.ModelProviderID,
		TopN:            values.TopN,
		Style:           values.Style,
		MaxOutputTokens: values.MaxOutputTokens,
	}
	if err := s.repo.Upsert(ctx, stored); err != nil {
		return nil, fmt.Errorf("TeamAISummarySettingsService.Update: %w", err)
	}

	return s.view(models.TeamAISummarySettingsSourceTeam, aiSummaryValuesFromStored(stored)), nil
}

// Reset implements TeamAISummarySettingsServiceInterface.
func (s *TeamAISummarySettingsService) Reset(ctx context.Context, userID, teamID string) error {
	if err := s.authz.Can(ctx, userID, teamID, authz.TeamSettingsUpdate); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, teamID); err != nil {
		return fmt.Errorf("TeamAISummarySettingsService.Reset: %w", err)
	}
	return nil
}

// aiSummaryValuesFromStored projects a persisted row onto the wire profile.
func aiSummaryValuesFromStored(stored *models.TeamAISummarySettings) models.TeamAISummarySettingsValues {
	return models.TeamAISummarySettingsValues{
		Enabled:         stored.Enabled,
		ModelProviderID: stored.ModelProviderID,
		TopN:            stored.TopN,
		Style:           stored.Style,
		MaxOutputTokens: stored.MaxOutputTokens,
	}
}

// ValidateAISummarySettings rejects a summary profile the instance would not
// honour, mirroring the team_ai_summary_settings CHECK constraints so an
// operator reading a 400 and a developer reading a constraint violation see one
// vocabulary.
//
// maxTopN is the instance's ai_summary.max_top_n: it is NOT settable per team,
// which is exactly what makes it the bound checked here rather than a value
// carried in values.
func ValidateAISummarySettings(v models.TeamAISummarySettingsValues, maxTopN int) error {
	if v.TopN < 1 || v.TopN > maxTopN {
		return fmt.Errorf("%w: top_n must be between 1 and %d, got %d",
			ErrInvalidAISummarySettings, maxTopN, v.TopN)
	}
	if v.MaxOutputTokens < 1 {
		return fmt.Errorf("%w: max_output_tokens must be >= 1, got %d",
			ErrInvalidAISummarySettings, v.MaxOutputTokens)
	}
	if !models.IsValidAISummaryStyle(v.Style) {
		return fmt.Errorf("%w: style must be one of %v, got %q",
			ErrInvalidAISummarySettings, models.AISummaryStyles, v.Style)
	}
	return nil
}
