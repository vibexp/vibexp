package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/repositories"
)

// HalfLifeFromDays converts a half-life expressed in days into a Duration. It
// is the single definition of that conversion, shared by the instance-default
// ranking config built at wire time and by per-team overrides resolved per
// request, so the two can never interpret the same number differently.
//
// Callers are expected to have validated the input against
// config.MaxSearchRankHalfLifeDays, which keeps the result clear of int64
// nanosecond overflow.
func HalfLifeFromDays(days float64) time.Duration {
	return time.Duration(days * float64(24*time.Hour))
}

// SearchSettingsResolver resolves the ranking configuration that applies to a
// team's searches.
//
// Resolve deliberately returns no error: resolution failures fall back to the
// instance defaults rather than failing the search (see
// TeamSearchSettingsResolver.Resolve).
type SearchSettingsResolver interface {
	Resolve(ctx context.Context, teamID string) SearchRankingConfig
}

// TeamSearchSettingsResolver resolves a team's stored ranking profile, falling
// back to the instance defaults when the team has not overridden them.
//
// It performs one primary-key lookup per search. That is deliberate: there is
// NO caching here. A TTL cache would mean "I changed the setting and search
// didn't change" in a multi-replica deployment — a worse failure than the cost
// it saves, given the lookup rides alongside a pgvector similarity scan that
// dominates the request. The interface leaves room to add caching if profiling
// ever justifies it; please do not add it speculatively.
type TeamSearchSettingsResolver struct {
	repo     repositories.TeamSearchSettingsRepository
	defaults SearchRankingConfig
	logger   *slog.Logger
}

var _ SearchSettingsResolver = (*TeamSearchSettingsResolver)(nil)

// NewTeamSearchSettingsResolver creates a resolver over the team settings
// repository. defaults is the instance-wide ranking config from config.yaml,
// returned whenever a team has no override of its own.
func NewTeamSearchSettingsResolver(
	repo repositories.TeamSearchSettingsRepository,
	defaults SearchRankingConfig,
	logger *slog.Logger,
) *TeamSearchSettingsResolver {
	return &TeamSearchSettingsResolver{repo: repo, defaults: defaults, logger: logger}
}

// Resolve returns the ranking config for teamID.
//
// It FAILS OPEN: a repository error logs at warn and yields the instance
// defaults instead of an error. The search query hits the same database moments
// later, so a genuine outage still surfaces as a failed search; a transient blip
// reading a settings row must not turn a working search into a 500.
func (r *TeamSearchSettingsResolver) Resolve(ctx context.Context, teamID string) SearchRankingConfig {
	settings, err := r.repo.Get(ctx, teamID)
	if err != nil {
		r.logger.With("team_id", teamID, "error", err).
			Warn("failed to read team search settings; falling back to instance defaults")
		return r.defaults
	}
	if settings == nil {
		// No override row: the team inherits the instance defaults entirely.
		return r.defaults
	}

	return SearchRankingConfig{
		Enabled:         settings.RecencyRankingEnabled,
		WeightRelevance: settings.RankWeightRelevance,
		WeightCreated:   settings.RankWeightCreated,
		WeightUpdated:   settings.RankWeightUpdated,
		HalfLife:        HalfLifeFromDays(settings.RankHalfLifeDays),
		// CandidateCap is ALWAYS the instance value, never the team's. It bounds
		// how many rows are pulled from Postgres and sorted in memory on every
		// ranked query, so letting one team raise it would let that team degrade
		// the whole instance. This is a cost/isolation boundary, not a default —
		// team_search_settings deliberately has no column for it.
		CandidateCap: r.defaults.CandidateCap,
	}
}
