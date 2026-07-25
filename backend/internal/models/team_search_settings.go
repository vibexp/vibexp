package models

import "time"

// TeamSearchSettings is a team's override of the instance search ranking
// defaults (the `search:` block of config.yaml).
//
// The override is whole-row, never per-field: a team either owns this complete
// profile or has no row at all and inherits every instance default. There is
// deliberately no RankCandidateCap field — the candidate cap governs how many
// rows are pulled from Postgres and re-ranked in memory, so it stays
// instance-only and a team cannot raise it.
type TeamSearchSettings struct {
	TeamID                string    `json:"team_id" db:"team_id"`
	RecencyRankingEnabled bool      `json:"recency_ranking_enabled" db:"recency_ranking_enabled"`
	RankWeightRelevance   float64   `json:"rank_weight_relevance" db:"rank_weight_relevance"`
	RankWeightCreated     float64   `json:"rank_weight_created" db:"rank_weight_created"`
	RankWeightUpdated     float64   `json:"rank_weight_updated" db:"rank_weight_updated"`
	RankHalfLifeDays      float64   `json:"rank_half_life_days" db:"rank_half_life_days"`
	CreatedAt             time.Time `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time `json:"updated_at" db:"updated_at"`
	Version               int64     `json:"version" db:"version"`
}

// Provenance of the search ranking settings in effect for a team.
const (
	// TeamSearchSettingsSourceInstance means the team has no override and
	// inherits the deployment defaults from config.yaml.
	TeamSearchSettingsSourceInstance = "instance"
	// TeamSearchSettingsSourceTeam means the team has stored its own profile.
	TeamSearchSettingsSourceTeam = "team"
)

// TeamSearchSettingsValues is a complete ranking profile, without the storage
// metadata (team id, timestamps, version) carried by TeamSearchSettings. It is
// what clients send on an update and what the API echoes back.
type TeamSearchSettingsValues struct {
	RecencyRankingEnabled bool    `json:"recency_ranking_enabled"`
	RankWeightRelevance   float64 `json:"rank_weight_relevance"`
	RankWeightCreated     float64 `json:"rank_weight_created"`
	RankWeightUpdated     float64 `json:"rank_weight_updated"`
	RankHalfLifeDays      float64 `json:"rank_half_life_days"`
}

// TeamSearchSettingsView is the read model for a team's search settings: the
// effective values plus everything a client needs to render the whole settings
// surface from one response — where the values came from, the defaults a reset
// would restore, and the instance-owned candidate cap.
type TeamSearchSettingsView struct {
	// Source is TeamSearchSettingsSourceInstance or TeamSearchSettingsSourceTeam.
	Source           string
	Values           TeamSearchSettingsValues
	InstanceDefaults TeamSearchSettingsValues
	// RankCandidateCap is instance-owned and never team-configurable; it is
	// exposed only so clients can explain the pagination clamp.
	RankCandidateCap int
}
