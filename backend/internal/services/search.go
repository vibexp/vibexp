package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// searchExcerptMaxLen bounds the length of a result excerpt in runes.
const searchExcerptMaxLen = 500

// pluralToSingular maps the request's plural resource type to the singular
// embeddings entity_type value.
var pluralToSingular = map[string]string{
	"prompts":    "prompt",
	"artifacts":  "artifact",
	"blueprints": "blueprint",
	"memories":   "memory",
}

// allEntityTypes is the singular entity_type set used when the request omits types.
var allEntityTypes = []string{"prompt", "artifact", "blueprint", "memory"}

// SearchService implements SearchServiceInterface.
type SearchService struct {
	repo     repositories.SearchRepository
	embedder QueryEmbedder
	logger   *slog.Logger
	ranking  SearchRankingConfig
	// model is the configured embedding model id; it gates the model_id filter so
	// query embeddings only match rows written by the same model.
	model string
	// now returns the reference time for recency decay; overridable in tests.
	now func() time.Time
}

var _ SearchServiceInterface = (*SearchService)(nil)

// NewSearchService creates a new SearchService. ranking controls result ordering:
// when disabled the service preserves the historical relevance-only ordering.
// model is the embedding model id used to filter stored embeddings; it must match
// the model the query embedder uses so query and document vectors are comparable.
func NewSearchService(
	repo repositories.SearchRepository,
	embedder QueryEmbedder,
	logger *slog.Logger,
	ranking SearchRankingConfig,
	model string,
) *SearchService {
	return &SearchService{
		repo:     repo,
		embedder: embedder,
		logger:   logger,
		ranking:  ranking,
		model:    model,
		now:      time.Now,
	}
}

// Search embeds the query, fetches matches from the repository, orders them
// (relevance-only or recency-blended depending on the ranking config), and maps
// the requested page to the response DTO.
func (s *SearchService) Search(
	ctx context.Context,
	teamID string,
	req *models.SearchRequest,
) (*models.SearchResultsResponse, error) {
	entityTypes := resolveEntityTypes(req.Types)

	vector, err := s.embedder.EmbedQuery(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("SearchService.Search: failed to embed query: %w", err)
	}

	pageRows, total, err := s.fetchPage(ctx, teamID, vector, entityTypes, req)
	if err != nil {
		return nil, err
	}

	if total == 0 {
		// No matches can mean a genuinely empty result or that no embeddings exist
		// for the configured model (e.g. the AI service changed its model
		// identifier). Log enough to tell the two apart without warning-level noise.
		s.logger.With(
			"team_id", teamID,
			"model_id", s.model,
			"entity_types", entityTypes,
		).
			Debug("semantic search returned no results")
	}

	items := make([]models.SearchResultItem, 0, len(pageRows))
	for i := range pageRows {
		items = append(items, mapRowToItem(&pageRows[i]))
	}

	return &models.SearchResultsResponse{
		Results:    items,
		TotalCount: total,
		Page:       req.Page,
		PerPage:    req.PerPage,
		TotalPages: totalPages(total, req.PerPage),
	}, nil
}

// fetchPage returns the rows for the requested page plus the full match count.
// With recency ranking disabled it asks the repository for exactly the page
// (distance order). With it enabled it pulls a distance-ordered candidate pool,
// re-ranks by the blended score in memory, and slices out the page.
func (s *SearchService) fetchPage(
	ctx context.Context,
	teamID string,
	vector []float32,
	entityTypes []string,
	req *models.SearchRequest,
) ([]models.SearchResultRow, int, error) {
	offset := (req.Page - 1) * req.PerPage

	if !s.ranking.Enabled {
		rows, total, err := s.repo.SearchSimilar(
			ctx, teamID, vector, s.model, entityTypes, req.ProjectID, req.PerPage, offset,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("SearchService.Search: %w", err)
		}
		return rows, total, nil
	}

	candidates, total, err := s.repo.SearchSimilar(
		ctx, teamID, vector, s.model, entityTypes, req.ProjectID, s.ranking.CandidateCap, 0,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("SearchService.Search: %w", err)
	}

	ranked := rankCandidates(candidates, s.ranking, s.now())

	// Only the candidate pool is re-ranked and therefore paginable; report the
	// paginable count rather than the full match count so TotalCount/TotalPages
	// stay consistent with the rows actually reachable via pagination (otherwise
	// a page past the pool would be empty while TotalPages still claimed more).
	// total >= len(candidates) always, since the pool is capped at CandidateCap.
	pageableTotal := total
	if len(candidates) < pageableTotal {
		pageableTotal = len(candidates)
	}
	return paginateRows(ranked, offset, req.PerPage), pageableTotal, nil
}

// resolveEntityTypes maps the requested plural types to singular entity types,
// defaulting to all four when the request omits them. Unknown values are skipped
// (handler-level validation rejects them before reaching the service).
func resolveEntityTypes(requested []string) []string {
	if len(requested) == 0 {
		return allEntityTypes
	}
	resolved := make([]string, 0, len(requested))
	for _, t := range requested {
		if singular, ok := pluralToSingular[t]; ok {
			resolved = append(resolved, singular)
		}
	}
	if len(resolved) == 0 {
		return allEntityTypes
	}
	return resolved
}

// mapRowToItem builds a response item from a repository row, deriving the excerpt
// and clamped relevance score.
func mapRowToItem(row *models.SearchResultRow) models.SearchResultItem {
	excerptSource := row.ChunkContent
	if excerptSource == "" {
		excerptSource = row.SourceBody
	}
	excerpt, _ := truncateExcerpt(excerptSource, searchExcerptMaxLen)

	return models.SearchResultItem{
		Type:        row.EntityType,
		ID:          row.EntityID,
		Title:       row.Title,
		Slug:        row.Slug,
		ProjectID:   row.ProjectID,
		ProjectName: row.ProjectName,
		Excerpt:     excerpt,
		Score:       clampScore(1 - row.Distance),
		ChunkID:     row.ChunkID,
		UpdatedAt:   row.UpdatedAt,
	}
}

// truncateExcerpt truncates s to at most maxLen runes, appending "..." if truncated.
func truncateExcerpt(s string, maxLen int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s, false
	}
	return string(runes[:maxLen]) + "...", true
}

// clampScore constrains score to [0,1].
func clampScore(score float64) float64 {
	return math.Max(0, math.Min(1, score))
}

// totalPages computes ceil(total/perPage), returning 0 when there are no results.
func totalPages(total, perPage int) int {
	if total == 0 || perPage <= 0 {
		return 0
	}
	return (total + perPage - 1) / perPage
}
