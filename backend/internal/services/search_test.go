package services_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

const testTeamID = "550e8400-e29b-41d4-a716-446655440000"

// testEmbeddingModel is the model id the test SearchService is built with; the
// service must pass it through as the SearchSimilar model_id arg.
const testEmbeddingModel = "gemini-embedding-001"

func newTestSearchService(t *testing.T) (
	*services.SearchService, *repomocks.MockSearchRepository, *svcmocks.MockQueryEmbedder,
) {
	// Ranking disabled preserves the historical relevance-only path that these
	// tests assert against.
	return newTestSearchServiceWithRanking(t, services.SearchRankingConfig{})
}

func newTestSearchServiceWithRanking(t *testing.T, ranking services.SearchRankingConfig) (
	*services.SearchService, *repomocks.MockSearchRepository, *svcmocks.MockQueryEmbedder,
) {
	repo := repomocks.NewMockSearchRepository(t)
	embedder := svcmocks.NewMockQueryEmbedder(t)
	logger := slog.New(slog.DiscardHandler)
	return services.NewSearchService(repo, embedder, logger, ranking, testEmbeddingModel), repo, embedder
}

func validVector() []float32 {
	return make([]float32, 384)
}

func TestSearchService_Search_DefaultsToAllTypes(t *testing.T) {
	svc, repo, embedder := newTestSearchService(t)
	vec := validVector()

	embedder.EXPECT().EmbedQuery(mock.Anything, "hello").Return(vec, nil)
	repo.EXPECT().
		SearchSimilar(mock.Anything, testTeamID, vec, testEmbeddingModel,
			[]string{"prompt", "artifact", "blueprint", "memory"}, "", 10, 0).
		Return([]models.SearchResultRow{}, 0, nil)

	resp, err := svc.Search(context.Background(), testTeamID, &models.SearchRequest{
		Query: "hello", Page: 1, PerPage: 10,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, resp.TotalCount)
	assert.Equal(t, 0, resp.TotalPages)
	assert.Empty(t, resp.Results)
}

func TestSearchService_Search_MapsPluralToSingular(t *testing.T) {
	svc, repo, embedder := newTestSearchService(t)
	vec := validVector()

	embedder.EXPECT().EmbedQuery(mock.Anything, "q").Return(vec, nil)
	// Service preserves the request order when mapping plural -> singular;
	// the repository enforces deterministic SQL ordering independently.
	repo.EXPECT().
		SearchSimilar(mock.Anything, testTeamID, vec, testEmbeddingModel,
			[]string{"memory", "prompt"}, "", 10, 0).
		Return([]models.SearchResultRow{}, 0, nil)

	_, err := svc.Search(context.Background(), testTeamID, &models.SearchRequest{
		Query: "q", Types: []string{"memories", "prompts"}, Page: 1, PerPage: 10,
	})

	require.NoError(t, err)
}

func TestSearchService_Search_PassesProjectFilter(t *testing.T) {
	svc, repo, embedder := newTestSearchService(t)
	vec := validVector()
	projectID := "7c9e6679-7425-40de-944b-e07fc1f90ae7"

	embedder.EXPECT().EmbedQuery(mock.Anything, "q").Return(vec, nil)
	// The request's project_id must be threaded verbatim into the repository call.
	repo.EXPECT().
		SearchSimilar(mock.Anything, testTeamID, vec, testEmbeddingModel,
			[]string{"prompt", "artifact", "blueprint", "memory"}, projectID, 10, 0).
		Return([]models.SearchResultRow{}, 0, nil)

	_, err := svc.Search(context.Background(), testTeamID, &models.SearchRequest{
		Query: "q", ProjectID: projectID, Page: 1, PerPage: 10,
	})

	require.NoError(t, err)
}

//nolint:funlen // Table-driven test with multiple mapping scenarios
func TestSearchService_Search_MapsRows(t *testing.T) {
	updatedAt := time.Now()
	longChunk := strings.Repeat("a", 600)

	tests := []struct {
		name            string
		row             models.SearchResultRow
		wantExcerpt     string
		wantScore       float64
		wantSlug        string
		wantProjectID   string
		wantProjectName string
	}{
		{
			name: "prompt carries its own slug and the parent project",
			row: models.SearchResultRow{
				EntityType: "prompt", EntityID: "p-1", ChunkID: "c-1",
				Title: "T", Slug: "my-prompt", ProjectID: "proj-1", ProjectName: "Project One",
				ChunkContent: "chunk text", SourceBody: "body", Distance: 0.2, UpdatedAt: updatedAt,
			},
			wantExcerpt:     "chunk text",
			wantScore:       0.8,
			wantSlug:        "my-prompt",
			wantProjectID:   "proj-1",
			wantProjectName: "Project One",
		},
		{
			name: "memory has empty slug but still carries the parent project",
			row: models.SearchResultRow{
				EntityType: "memory", EntityID: "m-1", ChunkID: "c-2",
				Title: "M", Slug: "", ProjectID: "proj-2", ProjectName: "Project Two",
				ChunkContent: "", SourceBody: "fallback body", Distance: 0.0, UpdatedAt: updatedAt,
			},
			wantExcerpt:     "fallback body",
			wantScore:       1.0,
			wantSlug:        "",
			wantProjectID:   "proj-2",
			wantProjectName: "Project Two",
		},
		{
			name: "artifact carries slug, project id and project name",
			row: models.SearchResultRow{
				EntityType: "artifact", EntityID: "a-1", ChunkID: "c-3",
				Title: "A", Slug: "my-artifact", ProjectID: "proj-3", ProjectName: "My Project",
				ChunkContent: "x", SourceBody: "y", Distance: 1.5, UpdatedAt: updatedAt,
			},
			wantExcerpt:     "x",
			wantScore:       0.0,
			wantSlug:        "my-artifact",
			wantProjectID:   "proj-3",
			wantProjectName: "My Project",
		},
		{
			name: "blueprint carries slug and project, truncates excerpt",
			row: models.SearchResultRow{
				EntityType: "blueprint", EntityID: "b-1", ChunkID: "c-4",
				Title: "B", Slug: "my-blueprint", ProjectID: "proj-3", ProjectName: "My Project",
				ChunkContent: longChunk, SourceBody: "", Distance: 0.5, UpdatedAt: updatedAt,
			},
			wantExcerpt:     strings.Repeat("a", 500) + "...",
			wantScore:       0.5,
			wantSlug:        "my-blueprint",
			wantProjectID:   "proj-3",
			wantProjectName: "My Project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, embedder := newTestSearchService(t)
			vec := validVector()
			embedder.EXPECT().EmbedQuery(mock.Anything, "q").Return(vec, nil)
			repo.EXPECT().
				SearchSimilar(mock.Anything, testTeamID, vec, testEmbeddingModel,
					mock.Anything, "", 10, 0).
				Return([]models.SearchResultRow{tt.row}, 1, nil)

			resp, err := svc.Search(context.Background(), testTeamID, &models.SearchRequest{
				Query: "q", Page: 1, PerPage: 10,
			})

			require.NoError(t, err)
			require.Len(t, resp.Results, 1)
			item := resp.Results[0]
			assert.Equal(t, tt.row.EntityType, item.Type)
			assert.Equal(t, tt.row.EntityID, item.ID)
			assert.Equal(t, tt.row.ChunkID, item.ChunkID)
			assert.Equal(t, tt.wantExcerpt, item.Excerpt)
			assert.InDelta(t, tt.wantScore, item.Score, 0.0001)
			assert.Equal(t, tt.wantSlug, item.Slug)
			assert.Equal(t, tt.wantProjectID, item.ProjectID)
			assert.Equal(t, tt.wantProjectName, item.ProjectName)
		})
	}
}

func TestSearchService_Search_TotalPagesAndOffset(t *testing.T) {
	svc, repo, embedder := newTestSearchService(t)
	vec := validVector()

	embedder.EXPECT().EmbedQuery(mock.Anything, "q").Return(vec, nil)
	// Page 2, per_page 10 -> offset (2-1)*10 = 10.
	repo.EXPECT().
		SearchSimilar(mock.Anything, testTeamID, vec, testEmbeddingModel, mock.Anything, "", 10, 10).
		Return([]models.SearchResultRow{}, 42, nil)

	resp, err := svc.Search(context.Background(), testTeamID, &models.SearchRequest{
		Query: "q", Page: 2, PerPage: 10,
	})

	require.NoError(t, err)
	assert.Equal(t, 42, resp.TotalCount)
	assert.Equal(t, 5, resp.TotalPages) // ceil(42/10)
	assert.Equal(t, 2, resp.Page)
	assert.Equal(t, 10, resp.PerPage)
}

func TestSearchService_Search_EmbedderError(t *testing.T) {
	svc, _, embedder := newTestSearchService(t)
	embedder.EXPECT().EmbedQuery(mock.Anything, "q").Return(nil, errors.New("ai down"))

	_, err := svc.Search(context.Background(), testTeamID, &models.SearchRequest{
		Query: "q", Page: 1, PerPage: 10,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to embed query")
}

func TestSearchService_Search_RepoError(t *testing.T) {
	svc, repo, embedder := newTestSearchService(t)
	vec := validVector()
	embedder.EXPECT().EmbedQuery(mock.Anything, "q").Return(vec, nil)
	repo.EXPECT().
		SearchSimilar(mock.Anything, testTeamID, vec, testEmbeddingModel, mock.Anything, "", 10, 0).
		Return(nil, 0, errors.New("db error"))

	_, err := svc.Search(context.Background(), testTeamID, &models.SearchRequest{
		Query: "q", Page: 1, PerPage: 10,
	})

	assert.Error(t, err)
}

func enabledRanking() services.SearchRankingConfig {
	return services.SearchRankingConfig{
		Enabled:         true,
		WeightRelevance: 0.5,
		WeightCreated:   0.3,
		WeightUpdated:   0.2,
		HalfLife:        90 * 24 * time.Hour,
		CandidateCap:    200,
	}
}

func TestSearchService_Search_RankingDisabled_FetchesExactPage(t *testing.T) {
	// With ranking off, the service must request exactly the page (per_page/offset)
	// and preserve the repository's distance order — no candidate-cap fetch.
	svc, repo, embedder := newTestSearchService(t)
	vec := validVector()

	embedder.EXPECT().EmbedQuery(mock.Anything, "q").Return(vec, nil)
	repo.EXPECT().
		SearchSimilar(mock.Anything, testTeamID, vec, testEmbeddingModel, mock.Anything, "", 10, 10).
		Return([]models.SearchResultRow{
			{EntityType: "prompt", EntityID: "p-1", ChunkID: "c-1", Distance: 0.1},
			{EntityType: "prompt", EntityID: "p-2", ChunkID: "c-2", Distance: 0.2},
		}, 25, nil)

	resp, err := svc.Search(context.Background(), testTeamID, &models.SearchRequest{
		Query: "q", Page: 2, PerPage: 10,
	})

	require.NoError(t, err)
	assert.Equal(t, 25, resp.TotalCount)
	require.Len(t, resp.Results, 2)
	assert.Equal(t, "c-1", resp.Results[0].ChunkID)
	assert.Equal(t, "c-2", resp.Results[1].ChunkID)
}

func TestSearchService_Search_RankingEnabled_PullsCandidateCapAndReRanks(t *testing.T) {
	svc, repo, embedder := newTestSearchServiceWithRanking(t, enabledRanking())
	now := time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	svc.SetClockForTest(func() time.Time { return now })
	vec := validVector()

	stale := now.Add(-360 * 24 * time.Hour) // ~4 half-lives => recency ~0.0625
	// Repo returns rows in ascending-distance order. After re-ranking, the fresher
	// row overtakes the slightly-closer-but-stale row, so the final order differs
	// from the distance order. This proves re-ranking actually runs.
	// c-stale: 0.5*0.70 + 0.5*~0.0625 ~= 0.381
	// c-fresh: 0.5*0.60 + 0.5*1.0      = 0.800
	candidates := []models.SearchResultRow{
		{
			EntityType: "prompt",
			EntityID:   "p-stale",
			ChunkID:    "c-stale",
			Distance:   0.30,
			CreatedAt:  stale,
			UpdatedAt:  stale,
		},
		{EntityType: "prompt", EntityID: "p-fresh", ChunkID: "c-fresh", Distance: 0.40, CreatedAt: now, UpdatedAt: now},
	}

	embedder.EXPECT().EmbedQuery(mock.Anything, "q").Return(vec, nil)
	// Ranking on: candidate cap (200) with offset 0, regardless of page size.
	repo.EXPECT().
		SearchSimilar(mock.Anything, testTeamID, vec, testEmbeddingModel, mock.Anything, "", 200, 0).
		Return(candidates, 2, nil)

	resp, err := svc.Search(context.Background(), testTeamID, &models.SearchRequest{
		Query: "q", Page: 1, PerPage: 10,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, resp.TotalCount)
	require.Len(t, resp.Results, 2)
	// Re-ranked: fresher row first despite its larger distance.
	assert.Equal(t, "c-fresh", resp.Results[0].ChunkID)
	assert.Equal(t, "c-stale", resp.Results[1].ChunkID)
	// Score stays pure relevance (clampScore(1-distance)), unchanged by ranking.
	assert.InDelta(t, 0.60, resp.Results[0].Score, 0.0001)
	assert.InDelta(t, 0.70, resp.Results[1].Score, 0.0001)
}

func TestSearchService_Search_RankingEnabled_PaginatesReRankedPool(t *testing.T) {
	svc, repo, embedder := newTestSearchServiceWithRanking(t, enabledRanking())
	now := time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	svc.SetClockForTest(func() time.Time { return now })
	vec := validVector()

	day := 24 * time.Hour
	mkRow := func(id string, ageDays int) models.SearchResultRow {
		created := now.Add(-time.Duration(ageDays) * day)
		return models.SearchResultRow{
			EntityType: "prompt", EntityID: id, ChunkID: id,
			Distance: 0.3, CreatedAt: created, UpdatedAt: now,
		}
	}
	// Four equally relevant candidates differing only by created_at; newest first
	// after re-rank. Page 2 (per_page 2) must return the two oldest.
	candidates := []models.SearchResultRow{
		mkRow("c1", 3), mkRow("c2", 1), mkRow("c3", 2), mkRow("c4", 0),
	}
	// Re-ranked newest-first: c4 (0d), c2 (-1d), c3 (-2d), c1 (-3d).

	embedder.EXPECT().EmbedQuery(mock.Anything, "q").Return(vec, nil)
	repo.EXPECT().
		SearchSimilar(mock.Anything, testTeamID, vec, testEmbeddingModel, mock.Anything, "", 200, 0).
		Return(candidates, 4, nil)

	resp, err := svc.Search(context.Background(), testTeamID, &models.SearchRequest{
		Query: "q", Page: 2, PerPage: 2,
	})

	require.NoError(t, err)
	assert.Equal(t, 4, resp.TotalCount)
	assert.Equal(t, 2, resp.TotalPages)
	require.Len(t, resp.Results, 2)
	assert.Equal(t, "c3", resp.Results[0].ChunkID)
	assert.Equal(t, "c1", resp.Results[1].ChunkID)
}

func TestSearchService_Search_RankingEnabled_OffsetBeyondPoolReturnsEmpty(t *testing.T) {
	svc, repo, embedder := newTestSearchServiceWithRanking(t, enabledRanking())
	svc.SetClockForTest(time.Now)
	vec := validVector()

	embedder.EXPECT().EmbedQuery(mock.Anything, "q").Return(vec, nil)
	repo.EXPECT().
		SearchSimilar(mock.Anything, testTeamID, vec, testEmbeddingModel, mock.Anything, "", 200, 0).
		Return([]models.SearchResultRow{
			{EntityType: "prompt", EntityID: "p-1", ChunkID: "c-1", Distance: 0.1},
		}, 1, nil)

	// Page 5 with per_page 10 -> offset 40, beyond the single-row pool.
	resp, err := svc.Search(context.Background(), testTeamID, &models.SearchRequest{
		Query: "q", Page: 5, PerPage: 10,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, resp.TotalCount) // pool size (equals the full match count here)
	assert.Empty(t, resp.Results)
}

func TestSearchService_Search_RankingEnabled_TotalCountCappedToPool(t *testing.T) {
	// When more rows match than the candidate cap, only the re-ranked pool is
	// paginable, so TotalCount/TotalPages report the pool size rather than the full
	// match count — keeping pagination metadata consistent with the reachable rows.
	cfg := enabledRanking()
	cfg.CandidateCap = 3
	svc, repo, embedder := newTestSearchServiceWithRanking(t, cfg)
	svc.SetClockForTest(time.Now)
	vec := validVector()

	embedder.EXPECT().EmbedQuery(mock.Anything, "q").Return(vec, nil)
	// Pool capped at 3, but 100 rows match overall.
	repo.EXPECT().
		SearchSimilar(mock.Anything, testTeamID, vec, testEmbeddingModel, mock.Anything, "", 3, 0).
		Return([]models.SearchResultRow{
			{EntityType: "prompt", EntityID: "p-1", ChunkID: "c-1", Distance: 0.1},
			{EntityType: "prompt", EntityID: "p-2", ChunkID: "c-2", Distance: 0.2},
			{EntityType: "prompt", EntityID: "p-3", ChunkID: "c-3", Distance: 0.3},
		}, 100, nil)

	resp, err := svc.Search(context.Background(), testTeamID, &models.SearchRequest{
		Query: "q", Page: 1, PerPage: 2,
	})

	require.NoError(t, err)
	assert.Equal(t, 3, resp.TotalCount) // capped to the candidate pool, not 100
	assert.Equal(t, 2, resp.TotalPages) // ceil(3/2)
	require.Len(t, resp.Results, 2)
}

func TestSearchService_Search_RankingEnabled_RepoError(t *testing.T) {
	svc, repo, embedder := newTestSearchServiceWithRanking(t, enabledRanking())
	vec := validVector()
	embedder.EXPECT().EmbedQuery(mock.Anything, "q").Return(vec, nil)
	repo.EXPECT().
		SearchSimilar(mock.Anything, testTeamID, vec, testEmbeddingModel, mock.Anything, "", 200, 0).
		Return(nil, 0, errors.New("db error"))

	_, err := svc.Search(context.Background(), testTeamID, &models.SearchRequest{
		Query: "q", Page: 1, PerPage: 10,
	})

	assert.Error(t, err)
}
