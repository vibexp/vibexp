package models

// EmbeddingCoverageCount is the repository-level, per-entity-type count of how many
// embeddable entities a team has and how many already have an embedding under the
// team's active model. It is the raw input the status service turns into an
// EmbeddingCoverageItem by deriving pending + percent.
type EmbeddingCoverageCount struct {
	EntityType string
	Total      int64
	Embedded   int64
}

// EmbeddingCoverageItem reports embedding coverage for a single entity type. The
// counts are derived from existing rows (no per-entity state): pending is
// total − embedded, and embedded_percent is round(embedded / total * 100), 0 when
// total is 0.
type EmbeddingCoverageItem struct {
	EntityType      string `json:"entity_type"`
	Total           int64  `json:"total"`
	Embedded        int64  `json:"embedded"`
	Pending         int64  `json:"pending"`
	EmbeddedPercent int    `json:"embedded_percent"`
}

// EmbeddingCoverageResponse reports, per entity type, how much of a team's embeddable
// content has an embedding under the team's active provider model. It is derived by
// diffing the source tables against the embeddings table (no new state), so a
// non-decreasing pending count is the signal that embedding is stuck. When the team
// has no active provider, HasActiveProvider is false, ActiveModel is null, and every
// type reports all entities as pending (0% embedded) rather than an error.
type EmbeddingCoverageResponse struct {
	HasActiveProvider bool                             `json:"has_active_provider"`
	ActiveModel       *string                          `json:"active_model"`
	Coverage          JSONArray[EmbeddingCoverageItem] `json:"coverage"`
}
