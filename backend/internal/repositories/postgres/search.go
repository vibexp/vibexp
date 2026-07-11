package postgres

import (
	"context"
	"database/sql"
	"errors"
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
}

// entitySources maps the singular embeddings entity_type to its source-table metadata.
var entitySources = map[string]entitySource{
	"prompt": {
		table:        "prompts",
		titleExpr:    "src.name",
		bodyExpr:     "src.body",
		slugExpr:     "src.slug",
		statusFilter: "src.status = 'published'",
	},
	"artifact": {
		table:        "artifacts",
		titleExpr:    "src.title",
		bodyExpr:     "src.content",
		slugExpr:     "src.slug",
		statusFilter: "src.status = 'active'",
	},
	"blueprint": {
		table:        "blueprints",
		titleExpr:    "src.title",
		bodyExpr:     "src.content",
		slugExpr:     "src.slug",
		statusFilter: "src.status = 'active'",
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

	return scanSearchRows(rows, limit)
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
	// the FTS GIN indexes still apply.
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
	// keywordTrgmThreshold is the pg_trgm word_similarity cutoff for the third
	// (typo-tolerance) fallback pass, set transaction-locally via set_config so a
	// single transposition still matches. word_similarity('widgte','widget') is
	// ~0.4, which the operator's default word_similarity_threshold of 0.6 would
	// reject; 0.3 admits a one-token typo without flooding results (only genuinely
	// near titles clear it, and this pass runs only when both FTS passes find
	// nothing). It is a fixed constant, never user input.
	keywordTrgmThreshold = "0.3"
)

// SearchKeyword runs a UNION ALL full-text search across the requested entity
// types, reading the source tables directly. It is the fallback for when no
// embedding provider is configured, so the embeddings table is empty.
//
// It runs up to three passes. The strict pass (websearch_to_tsquery) ANDs plain
// words, so precise/exact-term queries stay precise; multi-word natural-language
// questions, though, AND to nothing. When the strict pass yields zero rows a
// relaxed pass retries with OR semantics over the same sanitised lexemes. Both
// are exact-lexeme matches, so a single mistyped token still matches nothing;
// when the relaxed pass is also empty a third pg_trgm pass (#188) matches the
// query against resource TITLES/NAMES by trigram word-similarity, so a one-token
// typo (e.g. "widgte" -> "widget") still finds the target resource.
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

	rows, total, err = r.runKeywordPass(
		ctx, teamID, query, entityTypes, projectArg, limit, offset, keywordRelaxedTSQuery)
	if err != nil {
		return nil, 0, err
	}
	if total > 0 {
		return rows, total, nil
	}

	return r.runKeywordTrgmPass(ctx, teamID, query, entityTypes, projectArg, limit, offset)
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

// ftsExpr renders the combined title+body tsvector used for both the WHERE match
// and the ts_rank score, in the 'english' text-search configuration. Using the
// two-argument to_tsvector (explicit regconfig) keeps the expression IMMUTABLE so
// the matching combined-title+body FTS GIN index can satisfy it.
//
// Title/body weighting (setweight A/D + ts_rank_cd) was tried in #183 but reverted
// after a keyword-mode gold-set benchmark measured it as a net ranking regression
// (Recall@5 0.68 -> 0.61, nDCG@10 0.75 -> 0.71), because most real-world queries
// resolve via the relaxed OR pass, where a generic query word matching a title
// outranks a document that carries the relevant matches in its body.
func ftsExpr(src entitySource) string {
	return fmt.Sprintf(
		"to_tsvector('english', coalesce(%s, '') || ' ' || coalesce(%s, ''))",
		src.titleExpr, src.bodyExpr,
	)
}

// buildKeywordBranch builds the page SELECT for one entity type, matching the
// source table directly via full-text search with the given tsquery expression
// (strict or relaxed). It uses positional args: $1 = query, $2 = team_id,
// $3 = project_id (NULL = no filter), shared across every branch. chunk_content is
// the empty literal (there are no embedding chunks here); distance is 1 - ts_rank
// so the outer ORDER BY distance ASC yields the most relevant rows first and the
// service derives Score = 1 - distance = ts_rank.
func buildKeywordBranch(entityType string, src entitySource, tsquery string) string {
	tsv := ftsExpr(src)
	var b strings.Builder
	fmt.Fprintf(&b, "SELECT '%s' AS entity_type, src.id AS entity_id, src.id AS chunk_id, ", entityType)
	fmt.Fprintf(&b, "%s AS title, %s AS slug, ", src.titleExpr, src.slugExpr)
	b.WriteString("src.project_id::text AS project_id, COALESCE(proj.name, '') AS project_name, ")
	fmt.Fprintf(&b, "'' AS chunk_content, %s AS source_body, ", src.bodyExpr)
	b.WriteString("src.created_at AS created_at, src.updated_at AS updated_at, ")
	fmt.Fprintf(&b, "1 - ts_rank(%s, %s) AS distance ", tsv, tsquery)
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

// trgmTitleExpr renders the coalesced title/name expression the pg_trgm pass
// matches against — the resource's own title only (never the body), so a typo is
// tolerated against the short, high-signal title. It is kept byte-for-byte in
// sync with the migration-007 gin_trgm_ops index expressions (src.* qualifiers
// resolve to the same columns), so the `%>` operator stays index-accelerated.
func trgmTitleExpr(src entitySource) string {
	return fmt.Sprintf("coalesce(%s, '')", src.titleExpr)
}

// buildKeywordTrgmBranch builds the page SELECT for one entity type for the third
// (pg_trgm typo-tolerance) keyword pass. It matches the query against the title
// only via the `%>` word-similarity operator (index-backed) and scores with
// 1 - word_similarity so the outer ORDER BY distance ASC yields the closest
// titles first and the service derives Score = word_similarity. Positional args
// match the FTS branches: $1 = query, $2 = team_id, $3 = project_id (NULL = none).
// chunk_content is empty (no embedding chunks in keyword mode).
func buildKeywordTrgmBranch(entityType string, src entitySource) string {
	title := trgmTitleExpr(src)
	var b strings.Builder
	fmt.Fprintf(&b, "SELECT '%s' AS entity_type, src.id AS entity_id, src.id AS chunk_id, ", entityType)
	fmt.Fprintf(&b, "%s AS title, %s AS slug, ", src.titleExpr, src.slugExpr)
	b.WriteString("src.project_id::text AS project_id, COALESCE(proj.name, '') AS project_name, ")
	fmt.Fprintf(&b, "'' AS chunk_content, %s AS source_body, ", src.bodyExpr)
	b.WriteString("src.created_at AS created_at, src.updated_at AS updated_at, ")
	fmt.Fprintf(&b, "1 - word_similarity($1, %s) AS distance ", title)
	fmt.Fprintf(&b, "FROM %s src ", src.table)
	b.WriteString("LEFT JOIN projects proj ON src.project_id = proj.id ")
	b.WriteString("WHERE src.team_id = $2")
	if src.statusFilter != "" {
		b.WriteString(" AND " + src.statusFilter)
	}
	b.WriteString(" AND ($3::uuid IS NULL OR src.project_id = $3::uuid)")
	b.WriteString(" AND " + title + " %> $1")
	return b.String()
}

// buildKeywordTrgmCountBranch builds a COUNT-only SELECT for one entity type for
// the pg_trgm pass: same title `%>` predicate and positional args as
// buildKeywordTrgmBranch, without the word_similarity score or unused projections.
func buildKeywordTrgmCountBranch(src entitySource) string {
	title := trgmTitleExpr(src)
	var b strings.Builder
	b.WriteString("SELECT 1 ")
	fmt.Fprintf(&b, "FROM %s src ", src.table)
	b.WriteString("WHERE src.team_id = $2")
	if src.statusFilter != "" {
		b.WriteString(" AND " + src.statusFilter)
	}
	b.WriteString(" AND ($3::uuid IS NULL OR src.project_id = $3::uuid)")
	b.WriteString(" AND " + title + " %> $1")
	return b.String()
}

// buildKeywordTrgmUnion assembles the page-query UNION ALL body (including the
// word_similarity score) for the pg_trgm pass over the requested entity types.
func buildKeywordTrgmUnion(entityTypes []string) (string, error) {
	return buildUnionWith(entityTypes, buildKeywordTrgmBranch)
}

// buildKeywordTrgmCountUnion assembles the count-query UNION ALL body for the
// pg_trgm pass over the requested entity types.
func buildKeywordTrgmCountUnion(entityTypes []string) (string, error) {
	return buildUnionWith(entityTypes, func(_ string, src entitySource) string {
		return buildKeywordTrgmCountBranch(src)
	})
}

// runKeywordTrgmPass is the third keyword fallback (#188): a pg_trgm
// word-similarity match against resource titles/names only, reached only when
// both full-text passes returned zero rows. It lowers
// pg_trgm.word_similarity_threshold transaction-locally (set_config with
// is_local = true) so a single-token typo clears the `%>` operator, then counts
// and — if non-empty — fetches the page. Count and page share one transaction so
// the local threshold applies to both and never leaks onto the pooled connection.
func (r *SearchRepository) runKeywordTrgmPass(
	ctx context.Context,
	teamID string,
	query string,
	entityTypes []string,
	projectArg interface{},
	limit, offset int,
) ([]models.SearchResultRow, int, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to begin trgm search transaction: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			slog.Error("Failed to roll back trgm search transaction", "error", rbErr)
		}
	}()

	if _, err = tx.ExecContext(ctx,
		"SELECT set_config('pg_trgm.word_similarity_threshold', $1, true)", keywordTrgmThreshold); err != nil {
		return nil, 0, fmt.Errorf("failed to set trgm word_similarity threshold: %w", err)
	}

	total, err := countKeywordTrgm(ctx, tx, teamID, query, entityTypes, projectArg)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []models.SearchResultRow{}, 0, nil
	}

	rows, err := queryKeywordTrgmPage(ctx, tx, teamID, query, entityTypes, projectArg, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// countKeywordTrgm counts the pg_trgm title matches within the pass transaction
// tx (so the transaction-local word_similarity threshold applies). Args:
// $1 = query, $2 = team_id, $3 = project_id (NULL = no filter).
func countKeywordTrgm(
	ctx context.Context,
	tx *sql.Tx,
	teamID, query string,
	entityTypes []string,
	projectArg interface{},
) (int, error) {
	union, err := buildKeywordTrgmCountUnion(entityTypes)
	if err != nil {
		return 0, err
	}

	// The union is assembled from a fixed, validated entity-type allowlist
	// (buildUnionWith); the query, team and project are bound parameters.
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS results", union) // #nosec G201

	var total int
	if scanErr := tx.QueryRowContext(ctx, countSQL, query, teamID, projectArg).Scan(&total); scanErr != nil {
		return 0, fmt.Errorf("failed to count trgm keyword search results: %w", scanErr)
	}
	return total, nil
}

// queryKeywordTrgmPage fetches one page of pg_trgm title matches within the pass
// transaction tx. Args: $1 = query, $2 = team_id, $3 = project_id (NULL = none),
// $4 = limit, $5 = offset.
func queryKeywordTrgmPage(
	ctx context.Context,
	tx *sql.Tx,
	teamID, query string,
	entityTypes []string,
	projectArg interface{},
	limit, offset int,
) ([]models.SearchResultRow, error) {
	union, err := buildKeywordTrgmUnion(entityTypes)
	if err != nil {
		return nil, err
	}

	// entity_id is a deterministic secondary sort key so pagination stays stable
	// when several titles share a word_similarity score (mirrors the FTS passes).
	// The union comes from the validated entity-type allowlist; values are bound.
	pageSQL := fmt.Sprintf( // #nosec G201
		"SELECT entity_type, entity_id, chunk_id, title, slug, project_id, project_name, "+
			"chunk_content, source_body, created_at, updated_at, distance "+
			"FROM (%s) AS results ORDER BY distance ASC, entity_id ASC LIMIT $4 OFFSET $5",
		union,
	)

	rows, err := tx.QueryContext(ctx, pageSQL, query, teamID, projectArg, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to run trgm keyword search: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close search rows", "error", closeErr)
		}
	}()

	return scanSearchRows(rows, limit)
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

	return scanSearchRows(rows, limit)
}

// scanSearchRows scans the 12-column SearchResultRow projection shared by every
// search query (semantic and all keyword passes) into a slice, capacity hinted by
// limit. Callers own closing rows.
func scanSearchRows(rows *sql.Rows, limit int) ([]models.SearchResultRow, error) {
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

	if err := rows.Err(); err != nil {
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
