package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// RelationRepository is the Postgres implementation of
// repositories.RelationRepository. Every query is keyed by team_id (tenancy
// only, no role predicates — decision D3); the polymorphic from_id/to_id have
// no foreign key, so a resource's edges are cleaned up in app code.
type RelationRepository struct {
	db *database.DB
}

// NewRelationRepository creates a new RelationRepository.
func NewRelationRepository(db *database.DB) repositories.RelationRepository {
	return &RelationRepository{db: db}
}

// relationColumns is the shared SELECT column list, ordered to match scanRelation.
const relationColumns = "id, team_id, project_id, from_type, from_id, to_type, to_id, " +
	"relation_type, origin, status, created_by, confirmed_by, created_at, updated_at"

func scanRelation(row interface{ Scan(...interface{}) error }, r *models.Relation) error {
	var createdBy, confirmedBy sql.NullString
	if err := row.Scan(
		&r.ID, &r.TeamID, &r.ProjectID, &r.FromType, &r.FromID, &r.ToType, &r.ToID,
		&r.RelationType, &r.Origin, &r.Status, &createdBy, &confirmedBy,
		&r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return err
	}
	if createdBy.Valid {
		r.CreatedBy = &createdBy.String
	}
	if confirmedBy.Valid {
		r.ConfirmedBy = &confirmedBy.String
	}
	return nil
}

// Create inserts an edge idempotently. ON CONFLICT DO NOTHING on the unique
// endpoint tuple makes a duplicate a no-op; when the insert is suppressed
// (RETURNING yields no row) the pre-existing edge is fetched and returned, so
// concurrent creators both observe the same row without a unique-violation
// error surfacing.
func (r *RelationRepository) Create(ctx context.Context, relation *models.Relation) (*models.Relation, bool, error) {
	query := `
		INSERT INTO resource_relations
			(team_id, project_id, from_type, from_id, to_type, to_id, relation_type, origin, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (team_id, from_type, from_id, relation_type, to_type, to_id) DO NOTHING
		RETURNING ` + relationColumns

	var out models.Relation
	err := scanRelation(r.db.QueryRowContext(ctx, query,
		relation.TeamID, relation.ProjectID, relation.FromType, relation.FromID,
		relation.ToType, relation.ToID, relation.RelationType, relation.Origin,
		relation.Status, relation.CreatedBy,
	), &out)
	if errors.Is(err, sql.ErrNoRows) {
		// The edge already exists (conflict suppressed the insert): return it.
		existing, fetchErr := r.getByEndpoints(ctx, relation)
		return existing, false, fetchErr
	}
	if err != nil {
		if isFKViolation(err) {
			return nil, false, fmt.Errorf("team or project not found for relation: %w", err)
		}
		return nil, false, fmt.Errorf("failed to create relation: %w", err)
	}
	return &out, true, nil
}

// getByEndpoints fetches the edge matching the unique endpoint tuple of rel.
func (r *RelationRepository) getByEndpoints(ctx context.Context, rel *models.Relation) (*models.Relation, error) {
	query := "SELECT " + relationColumns + " FROM resource_relations " +
		"WHERE team_id = $1 AND from_type = $2 AND from_id = $3 AND relation_type = $4 " +
		"AND to_type = $5 AND to_id = $6"

	var out models.Relation
	err := scanRelation(r.db.QueryRowContext(ctx, query,
		rel.TeamID, rel.FromType, rel.FromID, rel.RelationType, rel.ToType, rel.ToID,
	), &out)
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to fetch existing relation: %w", err),
			repositories.ErrRelationNotFound,
		)
	}
	return &out, nil
}

func (r *RelationRepository) GetByID(ctx context.Context, teamID, id string) (*models.Relation, error) {
	query := "SELECT " + relationColumns + " FROM resource_relations WHERE id = $1 AND team_id = $2"

	var out models.Relation
	err := scanRelation(r.db.QueryRowContext(ctx, query, id, teamID), &out)
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get relation by ID: %w", err),
			repositories.ErrRelationNotFound,
		)
	}
	return &out, nil
}

// relatedResourceQuery lists the edges touching a resource on either endpoint
// and resolves, per row, the OTHER endpoint's title and link fields via a
// per-type LEFT JOIN (the same title expressions search.go / recentCommentsQuery
// use: artifact/blueprint -> title, prompt -> name, memory -> LEFT(text,100)).
// The CTE folds each edge to its "other" side and its direction relative to the
// queried resource. $2/$3 are the queried resource's (type, id).
const relatedResourceQuery = `
	WITH edges AS (
		SELECT
			rr.id, rr.relation_type, rr.origin, rr.status, rr.created_at,
			CASE WHEN rr.from_type = $2 AND rr.from_id = $3
				THEN 'outgoing' ELSE 'incoming' END AS direction,
			CASE WHEN rr.from_type = $2 AND rr.from_id = $3
				THEN rr.to_type ELSE rr.from_type END AS other_type,
			CASE WHEN rr.from_type = $2 AND rr.from_id = $3
				THEN rr.to_id ELSE rr.from_id END AS other_id
		FROM resource_relations rr
		WHERE rr.team_id = $1
			AND ((rr.from_type = $2 AND rr.from_id = $3) OR (rr.to_type = $2 AND rr.to_id = $3))
	)
	SELECT
		e.id, e.relation_type, e.direction, e.origin, e.status, e.other_type, e.other_id, e.created_at,
		COALESCE(a.title, b.title, p.name, LEFT(m.text, 100), '') AS title,
		COALESCE(a.project_id, b.project_id, p.project_id, m.project_id) AS project_id,
		COALESCE(a.slug, b.slug, p.slug) AS slug
	FROM edges e
	LEFT JOIN artifacts  a ON e.other_type = 'artifact'  AND a.id = e.other_id AND a.team_id = $1
	LEFT JOIN blueprints b ON e.other_type = 'blueprint' AND b.id = e.other_id AND b.team_id = $1
	LEFT JOIN prompts    p ON e.other_type = 'prompt'    AND p.id = e.other_id AND p.team_id = $1
	LEFT JOIN memories   m ON e.other_type = 'memory'    AND m.id = e.other_id AND m.team_id = $1
	ORDER BY e.created_at DESC, e.id DESC
	LIMIT $4 OFFSET $5
`

func (r *RelationRepository) ListByResource(
	ctx context.Context, teamID, resourceType, resourceID string, page, limit int,
) ([]models.RelatedResource, int, error) {
	countQuery := `
		SELECT COUNT(*) FROM resource_relations
		WHERE team_id = $1
			AND ((from_type = $2 AND from_id = $3) OR (to_type = $2 AND to_id = $3))
	`
	var totalCount int
	if err := r.db.QueryRowContext(ctx, countQuery, teamID, resourceType, resourceID).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("failed to count relations: %w", err)
	}

	offset := (page - 1) * limit
	rows, err := r.db.QueryContext(ctx, relatedResourceQuery, teamID, resourceType, resourceID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list relations: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close relation rows", "error", closeErr)
		}
	}()

	related := make([]models.RelatedResource, 0)
	for rows.Next() {
		var (
			rr        models.RelatedResource
			projectID sql.NullString
			slug      sql.NullString
		)
		if scanErr := rows.Scan(
			&rr.RelationID, &rr.RelationType, &rr.Direction, &rr.Origin, &rr.Status,
			&rr.ResourceType, &rr.ResourceID, &rr.CreatedAt, &rr.Title, &projectID, &slug,
		); scanErr != nil {
			return nil, 0, fmt.Errorf("failed to scan related resource: %w", scanErr)
		}
		if projectID.Valid {
			rr.ProjectID = &projectID.String
		}
		if slug.Valid {
			rr.Slug = &slug.String
		}
		related = append(related, rr)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate relations: %w", err)
	}
	return related, totalCount, nil
}

// Confirm flips a suggested edge to confirmed. The status = 'suggested' guard
// makes the transition happen exactly once even under a concurrent confirm; a
// row already confirmed (or absent) matches nothing and yields ErrRelationNotFound.
func (r *RelationRepository) Confirm(
	ctx context.Context, teamID, id, confirmedBy string,
) (*models.Relation, error) {
	query := "UPDATE resource_relations SET status = 'confirmed', confirmed_by = $1, updated_at = now() " +
		"WHERE id = $2 AND team_id = $3 AND status = 'suggested' RETURNING " + relationColumns

	var out models.Relation
	err := scanRelation(r.db.QueryRowContext(ctx, query, confirmedBy, id, teamID), &out)
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to confirm relation: %w", err),
			repositories.ErrRelationNotFound,
		)
	}
	return &out, nil
}

func (r *RelationRepository) Delete(ctx context.Context, teamID, id string) error {
	query := "DELETE FROM resource_relations WHERE id = $1 AND team_id = $2"

	result, err := r.db.ExecContext(ctx, query, id, teamID)
	if err != nil {
		return fmt.Errorf("failed to delete relation: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read delete result: %w", err)
	}
	if affected == 0 {
		return repositories.ErrRelationNotFound
	}
	return nil
}

// DeleteByResource removes every edge where the resource appears on EITHER
// endpoint (from or to), so a deleted resource leaves no dangling edge.
func (r *RelationRepository) DeleteByResource(
	ctx context.Context, teamID, resourceType, resourceID string,
) (int64, error) {
	query := "DELETE FROM resource_relations WHERE team_id = $1 " +
		"AND ((from_type = $2 AND from_id = $3) OR (to_type = $2 AND to_id = $3))"

	result, err := r.db.ExecContext(ctx, query, teamID, resourceType, resourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete relations for resource: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read delete result: %w", err)
	}
	return affected, nil
}

// resourceProjectQueries maps each resource type to a tenancy-scoped query that
// returns the resource's project_id. Row-found doubles as the existence check.
// Per-type literal queries (rather than interpolating a table name) keep the
// SQL free of string formatting.
var resourceProjectQueries = map[string]string{
	models.RelationResourceTypeArtifact:  "SELECT project_id FROM artifacts WHERE id = $1 AND team_id = $2",
	models.RelationResourceTypeBlueprint: "SELECT project_id FROM blueprints WHERE id = $1 AND team_id = $2",
	models.RelationResourceTypePrompt:    "SELECT project_id FROM prompts WHERE id = $1 AND team_id = $2",
	models.RelationResourceTypeMemory:    "SELECT project_id FROM memories WHERE id = $1 AND team_id = $2",
}

func (r *RelationRepository) ResourceProjectID(
	ctx context.Context, teamID, resourceType, resourceID string,
) (string, bool, error) {
	query, ok := resourceProjectQueries[resourceType]
	if !ok {
		return "", false, fmt.Errorf("unknown resource type: %q", resourceType)
	}
	var projectID string
	err := r.db.QueryRowContext(ctx, query, resourceID, teamID).Scan(&projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to resolve resource project: %w", err)
	}
	return projectID, true, nil
}

// seedCandidateQuery finds similar (entity, entity) pairs within a team for the
// relation seed backfill (#426): a cosine self-join over the embeddings table,
// restricted to the same embedding model, distinct entities, both of a
// relatable resource type, with distance below the caller's threshold, nearest
// first. Distance is column-vs-column (`<=>`), so it is an O(n^2) scan per team
// (the HNSW index only accelerates query-vs-column) — acceptable for the
// one-shot, guarded, per-team backfill with a conservative threshold + LIMIT.
const seedCandidateQuery = `
	SELECT e1.entity_type, e1.entity_id, e2.entity_type, e2.entity_id,
		(e1.vector_embeddings <=> e2.vector_embeddings) AS distance,
		e1.updated_at, e2.updated_at
	FROM embeddings e1
	JOIN embeddings e2
		ON e2.team_id = e1.team_id
		AND e2.model_id = e1.model_id
		AND e1.entity_id <> e2.entity_id
	WHERE e1.team_id = $1
		AND e1.entity_type IN ('artifact', 'memory', 'prompt', 'blueprint')
		AND e2.entity_type IN ('artifact', 'memory', 'prompt', 'blueprint')
		AND (e1.vector_embeddings <=> e2.vector_embeddings) < $2
	ORDER BY distance ASC, e1.entity_id, e2.entity_id
	LIMIT $3
`

func (r *RelationRepository) FindSeedCandidates(
	ctx context.Context, teamID string, maxDistance float64, limit int,
) ([]models.RelationSeedCandidate, error) {
	rows, err := r.db.QueryContext(ctx, seedCandidateQuery, teamID, maxDistance, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find seed candidates: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close seed candidate rows", "error", closeErr)
		}
	}()

	candidates := make([]models.RelationSeedCandidate, 0)
	for rows.Next() {
		var c models.RelationSeedCandidate
		if scanErr := rows.Scan(
			&c.FromType, &c.FromID, &c.ToType, &c.ToID,
			&c.Distance, &c.FromUpdatedAt, &c.ToUpdatedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan seed candidate: %w", scanErr)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate seed candidates: %w", err)
	}
	return candidates, nil
}
