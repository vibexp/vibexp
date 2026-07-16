package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// CommentRepository is the Postgres implementation of
// repositories.CommentRepository. Every query is keyed by team_id (tenancy
// only, no role predicates — decision D3); the polymorphic resource_id has no
// foreign key, so a resource's comments are cleaned up in app code.
type CommentRepository struct {
	db *database.DB
}

// NewCommentRepository creates a new CommentRepository.
func NewCommentRepository(db *database.DB) repositories.CommentRepository {
	return &CommentRepository{db: db}
}

// commentColumns is the shared SELECT column list, ordered to match scanComment.
const commentColumns = "id, team_id, resource_type, resource_id, user_id, content, created_at, updated_at"

func scanComment(row interface{ Scan(...interface{}) error }, c *models.Comment) error {
	return row.Scan(
		&c.ID, &c.TeamID, &c.ResourceType, &c.ResourceID,
		&c.UserID, &c.Content, &c.CreatedAt, &c.UpdatedAt,
	)
}

func (r *CommentRepository) Create(ctx context.Context, comment *models.Comment) error {
	query := `
		INSERT INTO comments (team_id, resource_type, resource_id, user_id, content)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`
	err := r.db.QueryRowContext(ctx, query,
		comment.TeamID, comment.ResourceType, comment.ResourceID, comment.UserID, comment.Content,
	).Scan(&comment.ID, &comment.CreatedAt, &comment.UpdatedAt)
	if err != nil {
		if isFKViolation(err) {
			return fmt.Errorf("team or user not found for comment: %w", err)
		}
		return fmt.Errorf("failed to create comment: %w", err)
	}
	return nil
}

func (r *CommentRepository) GetByID(ctx context.Context, teamID, id string) (*models.Comment, error) {
	query := "SELECT " + commentColumns + " FROM comments WHERE id = $1 AND team_id = $2"

	var c models.Comment
	err := scanComment(r.db.QueryRowContext(ctx, query, id, teamID), &c)
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get comment by ID: %w", err),
			repositories.ErrCommentNotFound,
		)
	}
	return &c, nil
}

func (r *CommentRepository) ListByResource(
	ctx context.Context, teamID, resourceType, resourceID string, page, limit int,
) ([]models.Comment, int, error) {
	countQuery := `
		SELECT COUNT(*) FROM comments
		WHERE team_id = $1 AND resource_type = $2 AND resource_id = $3
	`
	var totalCount int
	if err := r.db.QueryRowContext(ctx, countQuery, teamID, resourceType, resourceID).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("failed to count comments: %w", err)
	}

	offset := (page - 1) * limit
	query := "SELECT " + commentColumns + " FROM comments " +
		"WHERE team_id = $1 AND resource_type = $2 AND resource_id = $3 " +
		"ORDER BY created_at DESC LIMIT $4 OFFSET $5"

	rows, err := r.db.QueryContext(ctx, query, teamID, resourceType, resourceID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list comments: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close comment rows", "error", closeErr)
		}
	}()

	comments := make([]models.Comment, 0)
	for rows.Next() {
		var c models.Comment
		if scanErr := scanComment(rows, &c); scanErr != nil {
			return nil, 0, fmt.Errorf("failed to scan comment: %w", scanErr)
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate comments: %w", err)
	}
	return comments, totalCount, nil
}

// recentCommentsQuery resolves each comment's resource title and link fields at
// query time via a per-type LEFT JOIN (the same title expressions search.go
// uses: artifact/blueprint -> title, prompt -> name, memory -> LEFT(text,100)).
// A comment whose resource has vanished has all four joins NULL and is excluded
// by the WHERE clause, so no dangling row is returned. Ordering is by latest
// activity so an edited comment resurfaces.
const recentCommentsQuery = `
	SELECT c.id, c.team_id, c.resource_type, c.resource_id, c.user_id, c.content, c.created_at, c.updated_at,
		COALESCE(a.title, b.title, p.name, LEFT(m.text, 100)) AS resource_title,
		COALESCE(a.project_id, b.project_id, p.project_id, m.project_id) AS project_id,
		COALESCE(a.slug, b.slug, p.slug) AS slug
	FROM comments c
	LEFT JOIN artifacts  a ON c.resource_type = 'artifact'  AND a.id = c.resource_id AND a.team_id = c.team_id
	LEFT JOIN blueprints b ON c.resource_type = 'blueprint' AND b.id = c.resource_id AND b.team_id = c.team_id
	LEFT JOIN prompts    p ON c.resource_type = 'prompt'    AND p.id = c.resource_id AND p.team_id = c.team_id
	LEFT JOIN memories   m ON c.resource_type = 'memory'    AND m.id = c.resource_id AND m.team_id = c.team_id
	WHERE c.team_id = $1
	  AND (a.id IS NOT NULL OR b.id IS NOT NULL OR p.id IS NOT NULL OR m.id IS NOT NULL)
	ORDER BY GREATEST(c.created_at, c.updated_at) DESC
	LIMIT $2
`

func (r *CommentRepository) ListRecentByTeam(
	ctx context.Context, teamID string, limit int,
) ([]models.CommentActivity, error) {
	rows, err := r.db.QueryContext(ctx, recentCommentsQuery, teamID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list recent comments: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close recent comment rows", "error", closeErr)
		}
	}()

	activities := make([]models.CommentActivity, 0)
	for rows.Next() {
		var (
			act       models.CommentActivity
			projectID sql.NullString
			slug      sql.NullString
		)
		if scanErr := rows.Scan(
			&act.ID, &act.TeamID, &act.ResourceType, &act.ResourceID,
			&act.UserID, &act.Content, &act.CreatedAt, &act.UpdatedAt,
			&act.ResourceTitle, &projectID, &slug,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan recent comment: %w", scanErr)
		}
		if projectID.Valid {
			act.ProjectID = &projectID.String
		}
		if slug.Valid {
			act.Slug = &slug.String
		}
		activities = append(activities, act)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate recent comments: %w", err)
	}
	return activities, nil
}

func (r *CommentRepository) UpdateContent(
	ctx context.Context, teamID, id, content string,
) (*models.Comment, error) {
	query := "UPDATE comments SET content = $1, updated_at = now() " +
		"WHERE id = $2 AND team_id = $3 RETURNING " + commentColumns

	var c models.Comment
	err := scanComment(r.db.QueryRowContext(ctx, query, content, id, teamID), &c)
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to update comment: %w", err),
			repositories.ErrCommentNotFound,
		)
	}
	return &c, nil
}

func (r *CommentRepository) Delete(ctx context.Context, teamID, id string) error {
	query := "DELETE FROM comments WHERE id = $1 AND team_id = $2"

	result, err := r.db.ExecContext(ctx, query, id, teamID)
	if err != nil {
		return fmt.Errorf("failed to delete comment: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read delete result: %w", err)
	}
	if affected == 0 {
		return repositories.ErrCommentNotFound
	}
	return nil
}

func (r *CommentRepository) DeleteByResource(
	ctx context.Context, teamID, resourceType, resourceID string,
) (int64, error) {
	query := "DELETE FROM comments WHERE team_id = $1 AND resource_type = $2 AND resource_id = $3"

	result, err := r.db.ExecContext(ctx, query, teamID, resourceType, resourceID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete comments for resource: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read delete result: %w", err)
	}
	return affected, nil
}

func (r *CommentRepository) DeleteByUser(ctx context.Context, teamID, userID string) (int64, error) {
	query := "DELETE FROM comments WHERE team_id = $1 AND user_id = $2"

	result, err := r.db.ExecContext(ctx, query, teamID, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete comments for user: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read delete result: %w", err)
	}
	return affected, nil
}

// resourceExistsQueries maps each resource type to a tenancy-scoped existence
// check. Using per-type literal queries (rather than interpolating a table name)
// keeps the SQL free of string formatting.
var resourceExistsQueries = map[string]string{
	models.CommentResourceTypeArtifact:  "SELECT EXISTS(SELECT 1 FROM artifacts WHERE id = $1 AND team_id = $2)",
	models.CommentResourceTypeBlueprint: "SELECT EXISTS(SELECT 1 FROM blueprints WHERE id = $1 AND team_id = $2)",
	models.CommentResourceTypePrompt:    "SELECT EXISTS(SELECT 1 FROM prompts WHERE id = $1 AND team_id = $2)",
	models.CommentResourceTypeMemory:    "SELECT EXISTS(SELECT 1 FROM memories WHERE id = $1 AND team_id = $2)",
}

func (r *CommentRepository) ResourceExists(
	ctx context.Context, teamID, resourceType, resourceID string,
) (bool, error) {
	query, ok := resourceExistsQueries[resourceType]
	if !ok {
		return false, fmt.Errorf("unknown resource type: %q", resourceType)
	}
	var exists bool
	if err := r.db.QueryRowContext(ctx, query, resourceID, teamID).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check resource existence: %w", err)
	}
	return exists, nil
}
