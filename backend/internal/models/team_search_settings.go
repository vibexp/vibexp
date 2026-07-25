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
