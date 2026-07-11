package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pgvector/pgvector-go"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// SearchRepository implements repositories.SearchRepository for PostgreSQL using pgvector.
type SearchRepository struct {
	db *database.DB
}

// NewSearchRepository creates a new SearchRepository.
func NewSearchRepository(db *database.DB) repositories.SearchRepository {
	return &SearchRepository{db: db}
}

// entitySource describes how to join an embedding chunk to its source table and
// which expressions yield the title, body, slug and visibility filter for that
// entity type. The team filter is applied on the denormalized embeddings.team_id
// column; the source join remains for title/body/slug/status/project columns and
// naturally excludes orphan chunks whose source row is gone. Every source table
// carries a NOT NULL project_id, so the project id and name are derived uniformly
// (src.project_id + a LEFT JOIN to projects) rather than per type.
type entitySource struct {
	table     string
	titleExpr string
	bodyExpr  string
	// slugExpr yields the resource's own slug, or the literal '' when it has none.
	slugExpr string
	// statusFilter is empty when the entity has no status column.
	statusFilter string
	// hasTitle is true when titleExpr is a real, distinct title column that should
	// be weighted above the body in the keyword-mode full-text vector (weight A vs
	// D). It is false for entities whose "title" is only a prefix of the body
	// (memories: LEFT(text, 100)), which therefore use a single body-weighted
	// vector to avoid double-counting the leading text. See ftsExpr.
	hasTitle bool
}

// entitySources maps the singular embeddings entity_type to its source-table metadata.
var entitySources = map[string]entitySource{
	"prompt": {
		table:        "prompts",
		titleExpr:    "src.name",
		bodyExpr:     "src.body",
		slugExpr:     "src.slug",
		statusFilter: "src.status = 'published'",
		hasTitle:     true,
	},
	"artifact": {
		table:        "artifacts",
		titleExpr:    "src.title",
		bodyExpr:     "src.content",
		slugExpr:     "src.slug",
		statusFilter: "src.status = 'active'",
		hasTitle:     true,
	},
	"blueprint": {
		table:        "blueprints",
		titleExpr:    "src.title",
		bodyExpr:     "src.content",
		slugExpr:     "src.slug",
		statusFilter: "src.status = 'active'",
		hasTitle:     true,
	},
	"memory": {
		table:        "memories",
		titleExpr:    "LEFT(src.text, 100)",
		bodyExpr:     "src.text",
		slugExpr:     "''",
		statusFilter: "src.status = 'active'",
	},
}

// SearchSimilar runs a UNION ALL semantic search across the requested entity types.
func (r *SearchRepository) SearchSimilar(
	ctx context.Context,
	teamID string,
	vec []float32,
	modelID string,
	entityTypes []string,
	projectID string,
	limit, offset int,
) ([]models.SearchResultRow, int, error) {
	searchVector := pgvector.NewVector(vec)
	projectArg := projectFilterArg(projectID)

	total, err := r.countSimilar(ctx, teamID, modelID, entityTypes, projectArg)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []models.SearchResultRow{}, 0, nil
	}

	rows, err := r.queryPage(ctx, teamID, searchVector, modelID, entityTypes, projectArg, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

// projectFilterArg converts an optional project id into a bind value: nil (SQL NULL)
// disables the project filter, a non-empty id activates it.
func projectFilterArg(projectID string) interface{} {
	if projectID == "" {
		return nil
	}
	return projectID
}

// buildBranch builds the SELECT for one entity type. It uses positional args:
// $1 = query vector, $2 = team_id, $3 = model_id, $4 = project_id (NULL = no filter),
// all shared across every branch.
func buildBranch(entityType string, src entitySource) string {
	var b strings.Builder
	fmt.Fprintf(&b, "SELECT '%s' AS entity_type, e.entity_id AS entity_id, e.id AS chunk_id, ", entityType)
	fmt.Fprintf(&b, "%s AS title, %s AS slug, ", src.titleExpr, src.slugExpr)
	b.WriteString("src.project_id::text AS project_id, COALESCE(proj.name, '') AS project_name, ")
	fmt.Fprintf(&b, "e.content AS chunk_content, %s AS source_body, ", src.bodyExpr)
	b.WriteString("src.created_at AS created_at, src.updated_at AS updated_at, ")
	b.WriteString("e.vector_embeddings <=> $1 AS distance ")
	fmt.Fprintf(&b, "FROM embeddings e JOIN %s src ON e.entity_id = src.id ", src.table)
	b.WriteString("LEFT JOIN projects proj ON src.project_id = proj.id ")
	b.WriteString("WHERE e.entity_type = '" + entityType + "' AND e.team_id = $2 AND e.model_id = $3")
	if src.statusFilter != "" {
		b.WriteString(" AND " + src.statusFilter)
	}
	b.WriteString(" AND ($4::uuid IS NULL OR src.project_id = $4::uuid)")
	return b.String()
}

// buildCountBranch builds a COUNT-only SELECT for one entity type. It projects the
// entity key (entity_type, entity_id) — not distance — so the enclosing count query
// can COUNT(*) over DISTINCT entities and match the deduplicated page, never running
// the <=> operator per row. Positional args: $1 = team_id, $2 = model_id,
// $3 = project_id (NULL = no filter).
func buildCountBranch(entityType string, src entitySource) string {
	var b strings.Builder
	fmt.Fprintf(&b, "SELECT '%s' AS entity_type, e.entity_id AS entity_id ", entityType)
	fmt.Fprintf(&b, "FROM embeddings e JOIN %s src ON e.entity_id = src.id ", src.table)
	b.WriteString("WHERE e.entity_type = '" + entityType + "' AND e.team_id = $1 AND e.model_id = $2")
	if src.statusFilter != "" {
		b.WriteString(" AND " + src.statusFilter)
	}
	b.WriteString(" AND ($3::uuid IS NULL OR src.project_id = $3::uuid)")
	return b.String()
}

// buildUnion assembles the page-query UNION ALL body (including the cosine
// distance) for the requested entity types.
func buildUnion(entityTypes []string) (string, error) {
	return buildUnionWith(entityTypes, buildBranch)
}

// buildCountUnion assembles the count-query UNION ALL body, which omits the
// distance computation, for the requested entity types.
func buildCountUnion(entityTypes []string) (string, error) {
	return buildUnionWith(entityTypes, buildCountBranch)
}

// buildDedupPageQuery wraps the relevance-ordered union with a per-entity dedup:
// it ranks each entity's chunks by ascending distance (chunk_id as a deterministic
// tie-break) and keeps only the best-scoring one, so a long document with several
// matching chunks appears exactly once, carrying its closest chunk's excerpt and
// score. entity_id is a stable secondary sort key on the outer ORDER BY so
// pagination never repeats or skips a resource across pages when distances tie.
// Positional args are unchanged: $5 = limit, $6 = offset.
func buildDedupPageQuery(union string) string {
	return fmt.Sprintf(
		"SELECT entity_type, entity_id, chunk_id, title, slug, project_id, project_name, "+
			"chunk_content, source_body, created_at, updated_at, distance "+
			"FROM (SELECT results.*, ROW_NUMBER() OVER ("+
			"PARTITION BY entity_type, entity_id ORDER BY distance ASC, chunk_id ASC) AS dedup_rank "+
			"FROM (%s) AS results) AS ranked "+
			"WHERE dedup_rank = 1 "+
			"ORDER BY distance ASC, entity_id ASC LIMIT $5 OFFSET $6",
		union,
	)
}

// buildUnionWith assembles a UNION ALL body for the requested entity types in a
// deterministic order, rendering each branch with branch. The entity type
// strings are validated against entitySources, never interpolated from raw user
// input.
func buildUnionWith(entityTypes []string, branch func(string, entitySource) string) (string, error) {
	order := []string{"prompt", "artifact", "blueprint", "memory"}
	branches := make([]string, 0, len(order))
	requested := make(map[string]bool, len(entityTypes))
	for _, t := range entityTypes {
		requested[t] = true
	}

	for _, entityType := range order {
		if !requested[entityType] {
			continue
		}
		src, ok := entitySources[entityType]
		if !ok {
			return "", fmt.Errorf("unsupported entity type: %s", entityType)
		}
		branches = append(branches, branch(entityType, src))
	}

	if len(branches) == 0 {
		return "", fmt.Errorf("no entity types requested")
	}

	return strings.Join(branches, " UNION ALL "), nil
}

func (r *SearchRepository) queryPage(
	ctx context.Context,
	teamID string,
	searchVector pgvector.Vector,
	modelID string,
	entityTypes []string,
	projectArg interface{},
	limit, offset int,
) ([]models.SearchResultRow, error) {
	union, err := buildUnion(entityTypes)
	if err != nil {
		return nil, err
	}

	query := buildDedupPageQuery(union)

	rows, err := r.db.QueryContext(ctx, query, searchVector, teamID, modelID, projectArg, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to run semantic search: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close search rows", "error", closeErr)
		}
	}()

	results := make([]models.SearchResultRow, 0, limit)
	for rows.Next() {
		var row models.SearchResultRow
		if scanErr := rows.Scan(
			&row.EntityType,
			&row.EntityID,
			&row.ChunkID,
			&row.Title,
			&row.Slug,
			&row.ProjectID,
			&row.ProjectName,
			&row.ChunkContent,
			&row.SourceBody,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.Distance,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", scanErr)
		}
		results = append(results, row)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate search results: %w", err)
	}

	return results, nil
}

func (r *SearchRepository) countSimilar(
	ctx context.Context,
	teamID string,
	modelID string,
	entityTypes []string,
	projectArg interface{},
) (int, error) {
	union, err := buildCountUnion(entityTypes)
	if err != nil {
		return 0, err
	}

	// Count distinct matching entities, not chunks: a document with many matching
	// chunks contributes one row to the deduplicated page, so total_count/total_pages
	// must count it once too.
	query := fmt.Sprintf(
		"SELECT COUNT(*) FROM (SELECT DISTINCT entity_type, entity_id FROM (%s) AS branch_rows) AS distinct_entities",
		union,
	)

	var total int
	if scanErr := r.db.QueryRowContext(ctx, query, teamID, modelID, projectArg).Scan(&total); scanErr != nil {
		return 0, fmt.Errorf("failed to count semantic search results: %w", scanErr)
	}

	return total, nil
}

const (
	// keywordStrictTSQuery is the precise full-text match. websearch_to_tsquery
	// ANDs plain words (like the old plainto_tsquery) but additionally honours
	// quoted phrases and explicit OR / "-" operators, so exact-term and operator
	// queries stay precise. The explicit 'english' regconfig keeps it IMMUTABLE so
	// the migration-006 weighted GIN indexes still apply.
	keywordStrictTSQuery = "websearch_to_tsquery('english', $1)"
	// keywordRelaxedTSQuery loosens the strict query to OR semantics by rewriting
	// every AND (&) between lexemes into an OR (|). websearch_to_tsquery first
	// normalises the raw input into sanitised lexemes (and, unlike to_tsquery over
	// raw words, never errors on arbitrary punctuation), so this yields an
	// OR-joined tsquery of exactly those lexemes while leaving quoted phrases
	// (<->) intact. It is used only as a second pass when the strict match returns
	// nothing, keeping precise queries precise while making multi-word,
	// question-style queries return results. Still composed of IMMUTABLE functions,
	// so the GIN indexes apply here too.
	keywordRelaxedTSQuery = "replace(websearch_to_tsquery('english', $1)::text, '&', '|')::tsquery"
)

// SearchKeyword runs a UNION ALL full-text search across the requested entity
// types, reading the source tables directly. It is the fallback for when no
// embedding provider is configured, so the embeddings table is empty.
//
// It runs up to two passes. The strict pass (websearch_to_tsquery) ANDs plain
// words, so precise/exact-term queries stay precise; multi-word natural-language
// questions, though, AND to nothing. When the strict pass yields zero rows a
// relaxed pass retries with OR semantics over the same sanitised lexemes so the
// fallback still returns relevant results instead of nothing.
func (r *SearchRepository) SearchKeyword(
	ctx context.Context,
	teamID string,
	query string,
	entityTypes []string,
	projectID string,
	limit, offset int,
) ([]models.SearchResultRow, int, error) {
	projectArg := projectFilterArg(projectID)

	rows, total, err := r.runKeywordPass(
		ctx, teamID, query, entityTypes, projectArg, limit, offset, keywordStrictTSQuery)
	if err != nil {
		return nil, 0, err
	}
	if total > 0 {
		return rows, total, nil
	}

	return r.runKeywordPass(
		ctx, teamID, query, entityTypes, projectArg, limit, offset, keywordRelaxedTSQuery)
}

// runKeywordPass executes one full-text pass with the given tsquery expression:
// it counts matches, short-circuits to an empty page when there are none, and
// otherwise fetches the requested page. Both passes share this logic; only the
// tsquery expression (strict vs relaxed) differs.
func (r *SearchRepository) runKeywordPass(
	ctx context.Context,
	teamID string,
	query string,
	entityTypes []string,
	projectArg interface{},
	limit, offset int,
	tsquery string,
) ([]models.SearchResultRow, int, error) {
	total, err := r.countKeyword(ctx, teamID, query, entityTypes, projectArg, tsquery)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []models.SearchResultRow{}, 0, nil
	}

	rows, err := r.queryKeywordPage(ctx, teamID, query, entityTypes, projectArg, limit, offset, tsquery)
	if err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

// ftsExpr renders the weighted title+body tsvector used for both the WHERE match
// and the ts_rank_cd score, in the 'english' text-search configuration. The title
// is tagged weight A and the body weight D via setweight(), so a term appearing in
// a resource's title outranks the same term buried in its body (see ts_rank_cd's
// default {A,B,C,D} weights of {1.0, 0.4, 0.2, 0.1}). Entities without a real
// title (memories, whose "title" is just LEFT(text, 100)) use a single
// D-weighted body vector to avoid double-weighting the leading text. Using the
// two-argument to_tsvector (explicit regconfig) keeps the expression IMMUTABLE, and
// it must stay byte-for-byte in sync with the weighted GIN indexes in migration 006
// (with src.* qualifiers resolving to the same columns) so the index still applies.
func ftsExpr(src entitySource) string {
	if !src.hasTitle {
		return fmt.Sprintf(
			"setweight(to_tsvector('english', coalesce(%s, '')), 'D')",
			src.bodyExpr,
		)
	}
	return fmt.Sprintf(
		"setweight(to_tsvector('english', coalesce(%s, '')), 'A') || "+
			"setweight(to_tsvector('english', coalesce(%s, '')), 'D')",
		src.titleExpr, src.bodyExpr,
	)
}

// buildKeywordBranch builds the page SELECT for one entity type, matching the
// source table directly via full-text search with the given tsquery expression
// (strict or relaxed). It uses positional args: $1 = query, $2 = team_id,
// $3 = project_id (NULL = no filter), shared across every branch. chunk_content is
// the empty literal (there are no embedding chunks here); distance is
// 1 - ts_rank_cd so the outer ORDER BY distance ASC yields the most relevant rows
// first and the service derives Score = 1 - distance = ts_rank_cd.
//
// Ranking uses ts_rank_cd (cover density) over the weighted title/body vector with
// normalization flags 1|32: flag 1 divides the rank by 1 + log(document length) so
// long documents no longer dominate purely by length, and flag 32 (rank/(rank+1))
// keeps the score in [0, 1) so 1 - rank stays a valid, non-negative distance.
// (The issue's HOW named 32 alone, but 32 does not length-normalize; flag 1 is
// added to satisfy the "long documents no longer dominate by length" criterion.)
func buildKeywordBranch(entityType string, src entitySource, tsquery string) string {
	tsv := ftsExpr(src)
	var b strings.Builder
	fmt.Fprintf(&b, "SELECT '%s' AS entity_type, src.id AS entity_id, src.id AS chunk_id, ", entityType)
	fmt.Fprintf(&b, "%s AS title, %s AS slug, ", src.titleExpr, src.slugExpr)
	b.WriteString("src.project_id::text AS project_id, COALESCE(proj.name, '') AS project_name, ")
	fmt.Fprintf(&b, "'' AS chunk_content, %s AS source_body, ", src.bodyExpr)
	b.WriteString("src.created_at AS created_at, src.updated_at AS updated_at, ")
	fmt.Fprintf(&b, "1 - ts_rank_cd(%s, %s, 1|32) AS distance ", tsv, tsquery)
	fmt.Fprintf(&b, "FROM %s src ", src.table)
	b.WriteString("LEFT JOIN projects proj ON src.project_id = proj.id ")
	b.WriteString("WHERE src.team_id = $2")
	if src.statusFilter != "" {
		b.WriteString(" AND " + src.statusFilter)
	}
	b.WriteString(" AND ($3::uuid IS NULL OR src.project_id = $3::uuid)")
	fmt.Fprintf(&b, " AND %s @@ %s", tsv, tsquery)
	return b.String()
}

// buildKeywordCountBranch builds a COUNT-only SELECT for one entity type. It omits
// the ts_rank score (and unused projections) and uses the same positional args as
// buildKeywordBranch: $1 = query, $2 = team_id, $3 = project_id (NULL = no filter).
// The tsquery expression (strict or relaxed) matches the page branch's. Unlike the
// page branch it reads no entity_type literal, so it needs only the source metadata.
func buildKeywordCountBranch(src entitySource, tsquery string) string {
	var b strings.Builder
	b.WriteString("SELECT 1 ")
	fmt.Fprintf(&b, "FROM %s src ", src.table)
	b.WriteString("WHERE src.team_id = $2")
	if src.statusFilter != "" {
		b.WriteString(" AND " + src.statusFilter)
	}
	b.WriteString(" AND ($3::uuid IS NULL OR src.project_id = $3::uuid)")
	fmt.Fprintf(&b, " AND %s @@ %s", ftsExpr(src), tsquery)
	return b.String()
}

// buildKeywordUnion assembles the page-query UNION ALL body (including the ts_rank
// score) for the requested entity types, using the given tsquery expression.
func buildKeywordUnion(entityTypes []string, tsquery string) (string, error) {
	return buildUnionWith(entityTypes, func(entityType string, src entitySource) string {
		return buildKeywordBranch(entityType, src, tsquery)
	})
}

// buildKeywordCountUnion assembles the count-query UNION ALL body for the requested
// entity types, using the given tsquery expression.
func buildKeywordCountUnion(entityTypes []string, tsquery string) (string, error) {
	return buildUnionWith(entityTypes, func(_ string, src entitySource) string {
		return buildKeywordCountBranch(src, tsquery)
	})
}

func (r *SearchRepository) queryKeywordPage(
	ctx context.Context,
	teamID string,
	query string,
	entityTypes []string,
	projectArg interface{},
	limit, offset int,
	tsquery string,
) ([]models.SearchResultRow, error) {
	union, err := buildKeywordUnion(entityTypes, tsquery)
	if err != nil {
		return nil, err
	}

	// entity_id is a deterministic secondary sort key: ts_rank produces identical
	// scores for documents with the same matched-term frequency (common in keyword
	// mode), and an unstable sort under LIMIT/OFFSET could repeat or skip rows across
	// pages. Tie-break by entity_id so pagination is stable.
	sqlQuery := fmt.Sprintf(
		"SELECT entity_type, entity_id, chunk_id, title, slug, project_id, project_name, "+
			"chunk_content, source_body, created_at, updated_at, distance "+
			"FROM (%s) AS results ORDER BY distance ASC, entity_id ASC LIMIT $4 OFFSET $5",
		union,
	)

	rows, err := r.db.QueryContext(ctx, sqlQuery, query, teamID, projectArg, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to run keyword search: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close search rows", "error", closeErr)
		}
	}()

	results := make([]models.SearchResultRow, 0, limit)
	for rows.Next() {
		var row models.SearchResultRow
		if scanErr := rows.Scan(
			&row.EntityType,
			&row.EntityID,
			&row.ChunkID,
			&row.Title,
			&row.Slug,
			&row.ProjectID,
			&row.ProjectName,
			&row.ChunkContent,
			&row.SourceBody,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.Distance,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", scanErr)
		}
		results = append(results, row)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate search results: %w", err)
	}

	return results, nil
}

func (r *SearchRepository) countKeyword(
	ctx context.Context,
	teamID string,
	query string,
	entityTypes []string,
	projectArg interface{},
	tsquery string,
) (int, error) {
	union, err := buildKeywordCountUnion(entityTypes, tsquery)
	if err != nil {
		return 0, err
	}

	sqlQuery := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS results", union)

	var total int
	if scanErr := r.db.QueryRowContext(ctx, sqlQuery, query, teamID, projectArg).Scan(&total); scanErr != nil {
		return 0, fmt.Errorf("failed to count keyword search results: %w", scanErr)
	}

	return total, nil
}
