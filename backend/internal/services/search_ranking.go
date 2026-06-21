package services

import (
	"math"
	"sort"
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// ln2 is the natural log of 2, used to convert a half-life into the decay
// constant for exponential recency decay.
var ln2 = math.Log(2)

// SearchRankingConfig controls how search results are ordered. When Enabled is
// false the service preserves the historical relevance-only ordering. When true
// the service re-ranks a candidate pool by a weighted blend of relevance and two
// freshness signals; relevance is expected to be the dominant weight.
type SearchRankingConfig struct {
	Enabled bool
	// WeightRelevance, WeightCreated and WeightUpdated weight the three ranking
	// factors. They are normalized by their sum at ranking time, so they need not
	// pre-sum to 1.
	WeightRelevance float64
	WeightCreated   float64
	WeightUpdated   float64
	// HalfLife is the single shared half-life for the exponential recency decay
	// applied to both created_at and updated_at.
	HalfLife time.Duration
	// CandidateCap bounds how many top-by-distance candidates are pulled for
	// in-memory re-ranking.
	CandidateCap int
}

// recencyScore maps an age to a (0,1] freshness score using exponential decay
// with the given half-life: score = exp(-age * ln2 / halfLife). A zero-age item
// scores 1; an item exactly one half-life old scores 0.5. Negative ages (clock
// skew / future timestamps) are clamped to 0 so they cannot exceed 1.
func recencyScore(age, halfLife time.Duration) float64 {
	if halfLife <= 0 {
		return 0
	}
	if age < 0 {
		age = 0
	}
	return math.Exp(-float64(age) * ln2 / float64(halfLife))
}

// finalScore blends relevance with the two recency scores using the supplied
// weights. The weights are normalized by their sum, so callers may pass raw
// weights. A non-positive weight sum is treated as relevance-only.
func finalScore(relevance, createdRecency, updatedRecency, wRel, wCreated, wUpdated float64) float64 {
	sum := wRel + wCreated + wUpdated
	if sum <= 0 {
		return relevance
	}
	return (wRel*relevance + wCreated*createdRecency + wUpdated*updatedRecency) / sum
}

// rankCandidates orders the candidate rows by their blended final score
// (descending). Ties break by created_at (newer first), then by chunk_id
// (lexical, ascending) for a stable total order across pages. now is supplied by
// the caller so decay is deterministic and testable. The input slice is sorted
// in place and returned.
func rankCandidates(rows []models.SearchResultRow, cfg SearchRankingConfig, now time.Time) []models.SearchResultRow {
	scores := make([]float64, len(rows))
	for i := range rows {
		relevance := clampScore(1 - rows[i].Distance)
		createdRecency := recencyScore(now.Sub(rows[i].CreatedAt), cfg.HalfLife)
		updatedRecency := recencyScore(now.Sub(rows[i].UpdatedAt), cfg.HalfLife)
		scores[i] = finalScore(
			relevance, createdRecency, updatedRecency,
			cfg.WeightRelevance, cfg.WeightCreated, cfg.WeightUpdated,
		)
	}

	indices := make([]int, len(rows))
	for i := range indices {
		indices[i] = i
	}

	sort.SliceStable(indices, func(a, b int) bool {
		ia, ib := indices[a], indices[b]
		if scores[ia] != scores[ib] {
			return scores[ia] > scores[ib]
		}
		if !rows[ia].CreatedAt.Equal(rows[ib].CreatedAt) {
			return rows[ia].CreatedAt.After(rows[ib].CreatedAt)
		}
		return rows[ia].ChunkID < rows[ib].ChunkID
	})

	ranked := make([]models.SearchResultRow, len(rows))
	for i, idx := range indices {
		ranked[i] = rows[idx]
	}
	return ranked
}

// paginateRows returns the [offset, offset+limit) slice of rows, returning an
// empty slice when offset is beyond the end. limit <= 0 yields an empty page.
func paginateRows(rows []models.SearchResultRow, offset, limit int) []models.SearchResultRow {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(rows) || limit <= 0 {
		return []models.SearchResultRow{}
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[offset:end]
}
