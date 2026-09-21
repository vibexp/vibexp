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
// surface: the settings API's read and its two writes.
//
// It deliberately does NOT carry Resolve. The fail-open read lives on
// AISummarySettingsResolver instead, so a handler typed against this interface
// physically cannot serve the settings API from it — the same structural split
// team_search_settings uses (SearchSettingsResolver is its own interface, and
// TeamSearchSettingsServiceInterface has no Resolve). A doc comment would not
// have stopped the mistake; the type system does.
type TeamAISummarySettingsServiceInterface interface {
	// Get returns the settings in effect for the team and REPORTS A FAILED
	// READ. Readable by any team member, so it takes no permission check of its
	// own — team membership is enforced by the tenancy middleware.
	Get(ctx context.Context, teamID string) (*models.TeamAISummarySettingsView, error)
	// Update stores a complete replacement profile for the team. Requires
	// authz.TeamSettingsUpdate; returns an ErrInvalidAISummarySettings-wrapped
	// error for a profile outside the instance bounds, or for a
	// model_provider_id that is not one of the team's own providers.
	Update(
		ctx context.Context, userID, teamID string, values models.TeamAISummarySettingsValues,
	) (*models.TeamAISummarySettingsView, error)
	// Reset drops the team's profile so it inherits the instance defaults again.
	// Requires authz.TeamSettingsUpdate. Resetting a team with no profile is a
	// no-op, not an error.
	Reset(ctx context.Context, userID, teamID string) error
}

// AISummarySettingsResolver resolves the AI summary settings that apply to a
// team, for the summary generator (#1073).
//
// Resolve FAILS OPEN — see TeamAISummarySettingsService.Resolve. That is the
// whole reason this is a separate interface from
// TeamAISummarySettingsServiceInterface: the settings API must report a failed
// read, and the only reliable way to keep it from reading through the
// degrading path is to keep that path off the interface it holds.
type AISummarySettingsResolver interface {
	Resolve(ctx context.Context, teamID string) (*models.TeamAISummarySettingsView, error)
}

// AISummaryAvailabilityResolver reports whether an AI Summary can be generated
// for a team, for the REST search response (#1074).
//
// Availability FAILS OPEN towards "not available" — see
// TeamAISummarySettingsService.Availability. It has no error return on purpose:
// its caller is the search handler, and a search must never fail because this
// lookup did.
type AISummaryAvailabilityResolver interface {
	Availability(ctx context.Context, teamID string) models.AISummaryAvailability
}

// TeamAISummarySettingsService implements TeamAISummarySettingsServiceInterface,
// AISummarySettingsResolver and AISummaryAvailabilityResolver.
//
// defaults is the deployment-wide `ai_summary:` config. It is both the fallback
// for a team with no stored profile and the instance_defaults reported on every
// read, so a client can preview a reset without a second request.
type TeamAISummarySettingsService struct {
	repo repositories.TeamAISummarySettingsRepository
	// providers resolves a submitted model_provider_id WITHIN the team, which
	// is the only tenancy check on that column: the FK references
	// model_providers(id) alone and therefore proves existence, not ownership.
	providers repositories.ModelProviderRepository
	authz     AuthorizationServiceInterface
	defaults  config.AISummaryConfig
	logger    *slog.Logger
}

var (
	_ TeamAISummarySettingsServiceInterface = (*TeamAISummarySettingsService)(nil)
	_ AISummarySettingsResolver             = (*TeamAISummarySettingsService)(nil)
	_ AISummaryAvailabilityResolver         = (*TeamAISummarySettingsService)(nil)
)

// NewTeamAISummarySettingsService creates a new TeamAISummarySettingsService.
func NewTeamAISummarySettingsService(
	repo repositories.TeamAISummarySettingsRepository,
	providers repositories.ModelProviderRepository,
	authzService AuthorizationServiceInterface,
	defaults config.AISummaryConfig,
	logger *slog.Logger,
) *TeamAISummarySettingsService {
	return &TeamAISummarySettingsService{
		repo:      repo,
		providers: providers,
		authz:     authzService,
		defaults:  defaults,
		logger:    logger,
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

// Get implements TeamAISummarySettingsServiceInterface.
//
// Unlike Resolve it does NOT fail open: the caller is asking what the settings
// ARE, and answering "the instance defaults" during a database outage would
// report a guess as fact — and, through the settings UI, invite an admin to
// save those defaults over a profile they cannot currently see.
func (s *TeamAISummarySettingsService) Get(
	ctx context.Context, teamID string,
) (*models.TeamAISummarySettingsView, error) {
	stored, err := s.repo.Get(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("TeamAISummarySettingsService.Get: %w", err)
	}
	if stored == nil {
		return s.view(models.TeamAISummarySettingsSourceInstance, s.instanceValues()), nil
	}
	return s.view(models.TeamAISummarySettingsSourceTeam, aiSummaryValuesFromStored(stored)), nil
}

// Resolve implements AISummarySettingsResolver.
//
// It FAILS OPEN: a repository error logs at warn and yields the instance
// defaults instead of an error. Summarisation is a tuning surface over work the
// caller is doing anyway, so a blip reading one settings row must degrade the
// tuning rather than fail the request that asked for the summary.
//
// The error in the signature is therefore ALWAYS nil today. It is part of the
// read shape issue #1071 specifies, shared with Get so a caller can move
// between the two without reshaping its call site — and it leaves room for a
// future non-repository failure here. Callers that must not mask an outage
// (the settings API, #1072) read through Get; they cannot reach this method at
// all, because it is not on the interface they hold.
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

// Availability implements AISummaryAvailabilityResolver.
//
// Available means the team has at least one model provider row; Enabled means
// the team's effective settings (via the fail-open Resolve) have the feature
// on. Either lookup failing degrades to available=false — logged at warn —
// rather than an error: this is a UI affordance flag on a search response, and
// hiding the affordance for one request is the right cost of a blip.
func (s *TeamAISummarySettingsService) Availability(
	ctx context.Context, teamID string,
) models.AISummaryAvailability {
	view, err := s.Resolve(ctx, teamID)
	if err != nil {
		s.logger.With("team_id", teamID, "error", err).
			Warn("failed to resolve AI summary settings; reporting AI summary as unavailable")
		return models.AISummaryAvailability{}
	}

	count, err := s.providers.Count(ctx, teamID)
	if err != nil {
		s.logger.With("team_id", teamID, "error", err).
			Warn("failed to count team model providers; reporting AI summary as unavailable")
		return models.AISummaryAvailability{Enabled: view.Values.Enabled}
	}

	return models.AISummaryAvailability{
		Available: count > 0,
		Enabled:   view.Values.Enabled,
	}
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
	if err := s.requireOwnedProvider(ctx, teamID, values.ModelProviderID); err != nil {
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

// requireOwnedProvider rejects a model_provider_id that is not one of teamID's
// own providers.
//
// The column's foreign key is REFERENCES model_providers(id) — it proves the
// provider EXISTS, not that it belongs to this team — so without this check a
// team admin could point their summaries at another team's provider row and,
// through the generator (#1073), at that team's base_url and encrypted API key.
// Every other query in ModelProviderRepository carries `AND team_id = $2` for
// the same reason; this is that predicate applied to the one place a provider
// id arrives from a client. A nil id is the "use the team default" value and
// needs no lookup.
func (s *TeamAISummarySettingsService) requireOwnedProvider(
	ctx context.Context, teamID string, providerID *string,
) error {
	if providerID == nil {
		return nil
	}
	if _, err := s.providers.GetByID(ctx, teamID, *providerID); err != nil {
		if errors.Is(err, repositories.ErrModelProviderNotFound) {
			return fmt.Errorf("%w: model_provider_id %q is not a model provider of this team",
				ErrInvalidAISummarySettings, *providerID)
		}
		return fmt.Errorf("TeamAISummarySettingsService.Update: resolving model provider: %w", err)
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
