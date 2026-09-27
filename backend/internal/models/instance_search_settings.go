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
