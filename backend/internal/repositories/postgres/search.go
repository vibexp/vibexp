package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/pgvector/pgvector-go"
	"github.com/sirupsen/logrus"

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
		statusFilter: "",
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

// buildCountBranch builds a COUNT-only SELECT for one entity type. It omits the
// cosine-distance expression (and unused projections) so counting never runs the
// <=> operator per row, and uses positional args: $1 = team_id, $2 = model_id,
// $3 = project_id (NULL = no filter).
func buildCountBranch(entityType string, src entitySource) string {
	var b strings.Builder
	b.WriteString("SELECT 1 ")
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

	query := fmt.Sprintf(
		"SELECT entity_type, entity_id, chunk_id, title, slug, project_id, project_name, "+
			"chunk_content, source_body, created_at, updated_at, distance "+
			"FROM (%s) AS results ORDER BY distance ASC LIMIT $5 OFFSET $6",
		union,
	)

	rows, err := r.db.QueryContext(ctx, query, searchVector, teamID, modelID, projectArg, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to run semantic search: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close search rows")
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

	query := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS results", union)

	var total int
	if scanErr := r.db.QueryRowContext(ctx, query, teamID, modelID, projectArg).Scan(&total); scanErr != nil {
		return 0, fmt.Errorf("failed to count semantic search results: %w", scanErr)
	}

	return total, nil
}
