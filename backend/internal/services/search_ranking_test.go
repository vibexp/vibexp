package services

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

const testHalfLife = 90 * 24 * time.Hour

func defaultRanking() SearchRankingConfig {
	return SearchRankingConfig{
		Enabled:         true,
		WeightRelevance: 0.5,
		WeightCreated:   0.3,
		WeightUpdated:   0.2,
		HalfLife:        testHalfLife,
		CandidateCap:    200,
	}
}

func TestRecencyScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		age  time.Duration
		want float64
	}{
		{"zero age scores 1", 0, 1},
		{"one half-life scores 0.5", testHalfLife, 0.5},
		{"two half-lives scores 0.25", 2 * testHalfLife, 0.25},
		{"negative age (future) clamps to 1", -testHalfLife, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, tt.want, recencyScore(tt.age, testHalfLife), 1e-9)
		})
	}
}

func TestRecencyScore_NonPositiveHalfLife(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0.0, recencyScore(time.Hour, 0))
	assert.Equal(t, 0.0, recencyScore(time.Hour, -time.Hour))
}

func TestRecencyScore_MonotonicallyDecreasingWithAge(t *testing.T) {
	t.Parallel()
	prev := recencyScore(0, testHalfLife)
	for _, age := range []time.Duration{time.Hour, 24 * time.Hour, 30 * 24 * time.Hour, 365 * 24 * time.Hour} {
		cur := recencyScore(age, testHalfLife)
		assert.Less(t, cur, prev, "score must decrease as age grows")
		assert.Greater(t, cur, 0.0, "score stays positive")
		prev = cur
	}
}

func TestFinalScore_RelevanceOnlyWeightsReproduceRelevance(t *testing.T) {
	t.Parallel()
	// With only the relevance weight set, the blended score must equal relevance
	// regardless of the freshness signals — this is the relevance-only mode.
	got := finalScore(0.42, 1.0, 1.0, 1.0, 0.0, 0.0)
	assert.InDelta(t, 0.42, got, 1e-9)
}

func TestFinalScore_NormalizesWeights(t *testing.T) {
	t.Parallel()
	// Un-normalized weights (sum 2) must give the same result as normalized ones.
	unnormalized := finalScore(0.8, 0.4, 0.2, 1.0, 0.6, 0.4)
	normalized := finalScore(0.8, 0.4, 0.2, 0.5, 0.3, 0.2)
	assert.InDelta(t, normalized, unnormalized, 1e-9)
}

func TestFinalScore_ZeroWeightSumFallsBackToRelevance(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, 0.7, finalScore(0.7, 0.1, 0.1, 0, 0, 0), 1e-9)
}

func TestRankCandidates_RelevanceDominates(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	// When freshness is held equal, relevance is the sole differentiator: the
	// lower-distance (more relevant) item must rank first.
	stale := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []models.SearchResultRow{
		{ChunkID: "less-relevant", Distance: 0.6, CreatedAt: stale, UpdatedAt: stale},
		{ChunkID: "more-relevant", Distance: 0.1, CreatedAt: stale, UpdatedAt: stale},
	}

	ranked := rankCandidates(rows, defaultRanking(), now)

	require.Len(t, ranked, 2)
	assert.Equal(t, "more-relevant", ranked[0].ChunkID)
	assert.Equal(t, "less-relevant", ranked[1].ChunkID)
}

func TestFinalScore_RelevanceIsTheDominantFactor(t *testing.T) {
	t.Parallel()
	// "Relevance dominant" means a given numeric change in relevance moves the
	// blended score by more than the same change applied to either freshness
	// signal alone, because w_relevance >= w_created >= w_updated. Hold two
	// signals fixed and bump each by the same delta in turn.
	const delta = 0.4
	wRel, wCreated, wUpdated := 0.5, 0.3, 0.2

	base := finalScore(0.3, 0.3, 0.3, wRel, wCreated, wUpdated)
	bumpRelevance := finalScore(0.3+delta, 0.3, 0.3, wRel, wCreated, wUpdated)
	bumpCreated := finalScore(0.3, 0.3+delta, 0.3, wRel, wCreated, wUpdated)
	bumpUpdated := finalScore(0.3, 0.3, 0.3+delta, wRel, wCreated, wUpdated)

	gainRelevance := bumpRelevance - base
	gainCreated := bumpCreated - base
	gainUpdated := bumpUpdated - base

	assert.Greater(t, gainRelevance, gainCreated, "relevance must move the score most")
	assert.GreaterOrEqual(t, gainCreated, gainUpdated, "created must move the score at least as much as updated")
}

func TestRankCandidates_CreatedOutweighsUpdatedWhenRelevanceTies(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	old := now.Add(-180 * 24 * time.Hour)
	// Two equally relevant rows. Row A is newly created but not recently updated;
	// row B was created long ago but updated just now. Because w_created (0.3) >
	// w_updated (0.2), the newly-created row A must win.
	rowA := models.SearchResultRow{ChunkID: "a-new-created", Distance: 0.2, CreatedAt: now, UpdatedAt: old}
	rowB := models.SearchResultRow{ChunkID: "b-new-updated", Distance: 0.2, CreatedAt: old, UpdatedAt: now}
	rows := []models.SearchResultRow{rowB, rowA}

	ranked := rankCandidates(rows, defaultRanking(), now)

	require.Len(t, ranked, 2)
	assert.Equal(t, "a-new-created", ranked[0].ChunkID)
	assert.Equal(t, "b-new-updated", ranked[1].ChunkID)
}

func TestRankCandidates_RelevanceOnlyWeightsReproduceDistanceOrdering(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	cfg := SearchRankingConfig{
		Enabled:         true,
		WeightRelevance: 1.0,
		WeightCreated:   0,
		WeightUpdated:   0,
		HalfLife:        testHalfLife,
		CandidateCap:    200,
	}
	// Freshness deliberately inverts distance order; with relevance-only weights
	// the final order must match ascending distance (the current behavior).
	old := now.Add(-365 * 24 * time.Hour)
	rows := []models.SearchResultRow{
		{ChunkID: "c", Distance: 0.5, CreatedAt: now, UpdatedAt: now},
		{ChunkID: "a", Distance: 0.1, CreatedAt: old, UpdatedAt: old},
		{ChunkID: "b", Distance: 0.3, CreatedAt: now, UpdatedAt: now},
	}

	ranked := rankCandidates(rows, cfg, now)

	require.Len(t, ranked, 3)
	assert.Equal(t, "a", ranked[0].ChunkID) // distance 0.1
	assert.Equal(t, "b", ranked[1].ChunkID) // distance 0.3
	assert.Equal(t, "c", ranked[2].ChunkID) // distance 0.5
}

func TestRankCandidates_TieBreakerCreatedThenChunkID(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	earlier := now.Add(-time.Hour)
	// All three are identical in relevance and updated_at. Newer created_at wins;
	// when created_at also ties, chunk_id breaks lexically ascending.
	rows := []models.SearchResultRow{
		{ChunkID: "zzz", Distance: 0.2, CreatedAt: now, UpdatedAt: now},
		{ChunkID: "aaa", Distance: 0.2, CreatedAt: now, UpdatedAt: now},
		{ChunkID: "older", Distance: 0.2, CreatedAt: earlier, UpdatedAt: now},
	}

	ranked := rankCandidates(rows, defaultRanking(), now)

	require.Len(t, ranked, 3)
	assert.Equal(t, "aaa", ranked[0].ChunkID)   // same created_at as zzz, lexically first
	assert.Equal(t, "zzz", ranked[1].ChunkID)   // same created_at as aaa
	assert.Equal(t, "older", ranked[2].ChunkID) // oldest created_at
}

func TestRankCandidates_DecayBehavesAcrossEqualRelevance(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	cfg := defaultRanking()
	// Three equally relevant rows differing only in age. Exponential decay must
	// produce a strict newest-first ordering.
	rows := []models.SearchResultRow{
		{ChunkID: "old", Distance: 0.3, CreatedAt: now.Add(-300 * 24 * time.Hour), UpdatedAt: now.Add(-300 * 24 * time.Hour)},
		{ChunkID: "mid", Distance: 0.3, CreatedAt: now.Add(-90 * 24 * time.Hour), UpdatedAt: now.Add(-90 * 24 * time.Hour)},
		{ChunkID: "new", Distance: 0.3, CreatedAt: now, UpdatedAt: now},
	}

	ranked := rankCandidates(rows, cfg, now)

	require.Len(t, ranked, 3)
	assert.Equal(t, "new", ranked[0].ChunkID)
	assert.Equal(t, "mid", ranked[1].ChunkID)
	assert.Equal(t, "old", ranked[2].ChunkID)
}

func TestRankCandidates_Empty(t *testing.T) {
	t.Parallel()
	ranked := rankCandidates(nil, defaultRanking(), time.Now())
	assert.Empty(t, ranked)
}

func TestPaginateRows(t *testing.T) {
	t.Parallel()
	mk := func(ids ...string) []models.SearchResultRow {
		out := make([]models.SearchResultRow, len(ids))
		for i, id := range ids {
			out[i] = models.SearchResultRow{ChunkID: id}
		}
		return out
	}
	all := mk("a", "b", "c", "d", "e")

	tests := []struct {
		name    string
		offset  int
		limit   int
		wantIDs []string
	}{
		{"first page", 0, 2, []string{"a", "b"}},
		{"middle page", 2, 2, []string{"c", "d"}},
		{"last partial page", 4, 2, []string{"e"}},
		{"offset beyond end returns empty", 10, 2, nil},
		{"offset at end returns empty", 5, 2, nil},
		{"zero limit returns empty", 0, 0, nil},
		{"negative offset treated as zero", -3, 2, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := paginateRows(all, tt.offset, tt.limit)
			ids := make([]string, len(got))
			for i := range got {
				ids[i] = got[i].ChunkID
			}
			if tt.wantIDs == nil {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tt.wantIDs, ids)
		})
	}
}

// TestRecencyScore_MatchesClosedForm guards the exponential decay formula against
// accidental drift (e.g. using a different base).
func TestRecencyScore_MatchesClosedForm(t *testing.T) {
	t.Parallel()
	age := 45 * 24 * time.Hour
	want := math.Exp(-float64(age) * math.Log(2) / float64(testHalfLife))
	assert.InDelta(t, want, recencyScore(age, testHalfLife), 1e-12)
}
