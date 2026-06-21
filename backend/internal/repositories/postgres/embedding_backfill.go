package postgres

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// EmbeddingBackfillRepository reads the source tables of every embeddable entity so
// the embedding pipeline can be re-run globally after a model change.
type EmbeddingBackfillRepository struct {
	db *database.DB
}

// NewEmbeddingBackfillRepository creates a new EmbeddingBackfillRepository.
func NewEmbeddingBackfillRepository(db *database.DB) repositories.EmbeddingBackfillRepository {
	return &EmbeddingBackfillRepository{db: db}
}

// backfillQuery describes how to read one entity type's source table into a
// BackfillEntity. selectFrom is the SELECT … FROM (… JOIN) body, projecting
// exactly the twelve columns scanned in ListEntities, in this order: entity_id,
// user_id, team_id, project_name, feed_id, slug, title, body, type, email, excerpt,
// created_at. Columns the entity lacks are emitted as an empty-string literal (or a
// join, for prompt email) so every branch scans uniformly. orderBy gives the stable
// total order. idExpr is the source row's id column, correlated against
// embeddings.entity_id when filtering to only entities missing an embedding.
type backfillQuery struct {
	selectFrom string
	orderBy    string
	idExpr     string
}

// backfillQueries maps each singular embeddable entity type to the SQL that
// reconstructs its `.created` event payload. Source columns mirror what the live
// services pass into the event constructors: project_id is carried as the
// "project name" argument, and feed item uses posted_by_user_id as the
// embedding-keying user id and posted_at as the created timestamp.
var backfillQueries = map[string]backfillQuery{
	"prompt": {
		selectFrom: `
		SELECT p.id, p.user_id, COALESCE(p.team_id::text, ''), COALESCE(p.project_id::text, ''),
		       '' AS feed_id, p.slug, p.name AS title, p.body, '' AS type,
		       COALESCE(u.email, '') AS email, '' AS excerpt, p.created_at
		FROM prompts p
		LEFT JOIN users u ON u.id = p.user_id`,
		orderBy: "p.created_at, p.id",
		idExpr:  "p.id",
	},
	"artifact": {
		selectFrom: `
		SELECT a.id, a.user_id, COALESCE(a.team_id::text, ''), COALESCE(a.project_id::text, ''),
		       '' AS feed_id, a.slug, a.title, a.content AS body, a.type, '' AS email, '' AS excerpt, a.created_at
		FROM artifacts a`,
		orderBy: "a.created_at, a.id",
		idExpr:  "a.id",
	},
	"memory": {
		selectFrom: `
		SELECT m.id, m.user_id, COALESCE(m.team_id::text, ''), COALESCE(m.project_id::text, ''),
		       '' AS feed_id, '' AS slug, '' AS title, m.text AS body, '' AS type, '' AS email,
		       '' AS excerpt, m.created_at
		FROM memories m`,
		orderBy: "m.created_at, m.id",
		idExpr:  "m.id",
	},
	"blueprint": {
		selectFrom: `
		SELECT b.id, b.user_id, COALESCE(b.team_id::text, ''), COALESCE(b.project_id::text, ''),
		       '' AS feed_id, b.slug, b.title, b.content AS body, b.type, '' AS email, '' AS excerpt, b.created_at
		FROM blueprints b`,
		orderBy: "b.created_at, b.id",
		idExpr:  "b.id",
	},
	"feed_item": {
		selectFrom: `
		SELECT f.id, f.posted_by_user_id AS user_id, COALESCE(f.team_id::text, ''),
		       '' AS project_name, COALESCE(f.feed_id::text, '') AS feed_id, '' AS slug,
		       f.title, f.content AS body, '' AS type, '' AS email, f.excerpt, f.posted_at AS created_at
		FROM feed_items f`,
		orderBy: "f.posted_at, f.id",
		idExpr:  "f.id",
	},
}

// buildBackfillSQL assembles the full paged query for entityType. When missingOnly
// is true it adds a NOT EXISTS subquery so only source rows without an embedding for
// the configured model are returned; the model id is bound as $3. entityType is a
// fixed map key (never user input), so embedding it as a literal is injection-safe.
func buildBackfillSQL(entityType string, q backfillQuery, missingOnly bool) string {
	where := ""
	if missingOnly {
		where = fmt.Sprintf(`
		WHERE NOT EXISTS (
			SELECT 1 FROM embeddings emb
			WHERE emb.entity_type = '%s' AND emb.entity_id = %s AND emb.model_id = $3
		)`, entityType, q.idExpr)
	}
	return fmt.Sprintf("%s%s\n\t\tORDER BY %s\n\t\tLIMIT $1 OFFSET $2", q.selectFrom, where, q.orderBy)
}

// ListEntities returns one page of entities of entityType, ordered by their
// creation timestamp then id for a stable total order across pages. When
// missingOnly is true, only entities without an embedding row for modelID are
// returned, so a backfill can target just the gaps left by a model swap.
func (r *EmbeddingBackfillRepository) ListEntities(
	ctx context.Context, entityType, modelID string, missingOnly bool, limit, offset int,
) ([]models.BackfillEntity, error) {
	q, ok := backfillQueries[entityType]
	if !ok {
		return nil, fmt.Errorf("unsupported backfill entity type: %s", entityType)
	}

	sql := buildBackfillSQL(entityType, q, missingOnly)

	args := []any{limit, offset}
	if missingOnly {
		args = append(args, modelID)
	}

	rows, err := r.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list %s entities for backfill: %w", entityType, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logrus.WithError(closeErr).Error("Failed to close backfill rows")
		}
	}()

	entities := make([]models.BackfillEntity, 0, limit)
	for rows.Next() {
		entity := models.BackfillEntity{EntityType: entityType}
		if scanErr := rows.Scan(
			&entity.EntityID,
			&entity.UserID,
			&entity.TeamID,
			&entity.ProjectName,
			&entity.FeedID,
			&entity.Slug,
			&entity.Title,
			&entity.Body,
			&entity.Type,
			&entity.Email,
			&entity.Excerpt,
			&entity.CreatedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan %s backfill row: %w", entityType, scanErr)
		}
		entities = append(entities, entity)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate %s backfill rows: %w", entityType, err)
	}

	return entities, nil
}
