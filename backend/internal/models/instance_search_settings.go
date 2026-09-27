package models

import "time"

// InstanceSearchSettings is the instance's own search ranking defaults (#1197,
// epic #1196): the database-stored counterpart of config.yaml's `search:`
// block, editable by an instance admin without file access or a restart.
//
// It carries the value columns of TeamSearchSettings plus the instance-only
// RankCandidateCap. The table is a singleton (primary key `id boolean CHECK
// (id)`), so there is no ID field: the row is only present or absent, and
// absent means the built-in defaults are in effect.
type InstanceSearchSettings struct {
	RecencyRankingEnabled bool    `json:"recency_ranking_enabled" db:"recency_ranking_enabled"`
	RankWeightRelevance   float64 `json:"rank_weight_relevance" db:"rank_weight_relevance"`
	RankWeightCreated     float64 `json:"rank_weight_created" db:"rank_weight_created"`
	RankWeightUpdated     float64 `json:"rank_weight_updated" db:"rank_weight_updated"`
	RankHalfLifeDays      float64 `json:"rank_half_life_days" db:"rank_half_life_days"`
	// RankCandidateCap bounds how many rows are pulled and re-ranked in memory.
	// It is instance-only: a team cannot raise it.
	RankCandidateCap int       `json:"rank_candidate_cap" db:"rank_candidate_cap"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
	// UpdatedBy is the user who last saved the defaults; nil for the boot-time
	// config.yaml import, or once that user has been deleted.
	UpdatedBy *string `json:"updated_by,omitempty" db:"updated_by"`
	Version   int64   `json:"version" db:"version"`
}

// MaxSearchRankHalfLifeDays caps the ranking half-life at 100 years. This keeps
// the days→time.Duration conversion (services.HalfLifeFromDays) well clear of
// int64 nanosecond overflow, which would otherwise wrap to a negative duration
// and silently zero the recency contribution.
//
// It lives here, in the leaf models package, because it is the single
// definition shared by every enforcement point: config.validateSearchRankingConfig,
// the team and instance settings validators in services, the
// team_search_settings CHECK constraints (migration 011_consolidated) and the
// instance_search_settings CHECK constraint (migration 022). Change them
// together.
const MaxSearchRankHalfLifeDays = 36500

// MaxSearchRankCandidateCap bounds the re-rank candidate pool so a misconfigured
// cap cannot blow up per-query memory and sort cost (the cap becomes the SQL
// LIMIT and the in-memory slice that is sorted on every ranked query). The
// instance_search_settings.rank_candidate_cap CHECK constraint (migration 022)
// mirrors it; change both together.
const MaxSearchRankCandidateCap = 5000

// Provenance of the instance search ranking defaults.
const (
	// InstanceSearchSettingsSourceInstance means an instance admin (or the
	// boot-time config import) stored the defaults in instance_search_settings.
	InstanceSearchSettingsSourceInstance = "instance"
	// InstanceSearchSettingsSourceDefault means no row is stored and the
	// built-in defaults are in effect.
	InstanceSearchSettingsSourceDefault = "default"
)

// InstanceSearchSettingsValues is the complete set of instance ranking
// defaults, without the storage metadata carried by InstanceSearchSettings.
type InstanceSearchSettingsValues struct {
	RecencyRankingEnabled bool    `json:"recency_ranking_enabled"`
	RankWeightRelevance   float64 `json:"rank_weight_relevance"`
	RankWeightCreated     float64 `json:"rank_weight_created"`
	RankWeightUpdated     float64 `json:"rank_weight_updated"`
	RankHalfLifeDays      float64 `json:"rank_half_life_days"`
	RankCandidateCap      int     `json:"rank_candidate_cap"`
}

// TeamValues projects the instance defaults onto the team-overridable profile,
// which is everything except the instance-only candidate cap.
func (v InstanceSearchSettingsValues) TeamValues() TeamSearchSettingsValues {
	return TeamSearchSettingsValues{
		RecencyRankingEnabled: v.RecencyRankingEnabled,
		RankWeightRelevance:   v.RankWeightRelevance,
		RankWeightCreated:     v.RankWeightCreated,
		RankWeightUpdated:     v.RankWeightUpdated,
		RankHalfLifeDays:      v.RankHalfLifeDays,
	}
}

// InstanceSearchSettingsView is the read model for the instance ranking
// defaults: the values in effect and where they came from.
type InstanceSearchSettingsView struct {
	// Source is InstanceSearchSettingsSourceInstance or
	// InstanceSearchSettingsSourceDefault.
	Source string
	Values InstanceSearchSettingsValues
	// UpdatedAt and UpdatedBy describe the stored row; both are nil when
	// Source is InstanceSearchSettingsSourceDefault, and UpdatedBy is also nil
	// for the boot-time import or once the saving user has been deleted.
	UpdatedAt *time.Time
	UpdatedBy *string
	// Version is the stored row's optimistic-lock counter, nil when Source is
	// InstanceSearchSettingsSourceDefault. Update's expectedVersion is compared with it.
	Version *int64
}
