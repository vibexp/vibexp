package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/pgvector/pgvector-go"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// EmbeddingRepository implements the repositories.EmbeddingRepository interface for PostgreSQL
type EmbeddingRepository struct {
	db *database.DB
}

// NewEmbeddingRepository creates a new EmbeddingRepository
func NewEmbeddingRepository(db *database.DB) repositories.EmbeddingRepository {
	return &EmbeddingRepository{
		db: db,
	}
}

// Create creates a new embedding
func (r *EmbeddingRepository) Create(ctx context.Context, embedding *models.Embedding) error {
	query := `
		INSERT INTO embeddings (
			user_id, team_id, entity_type, entity_id, vector_embeddings, content, model_id, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`

	// team_id is a nullable UUID column. An empty string is not a valid UUID, so
	// bind SQL NULL when the embedding has no resolved team.
	var teamID interface{}
	if embedding.TeamID != "" {
		teamID = embedding.TeamID
	}

	err := r.db.QueryRowContext(ctx, query,
		embedding.UserID,
		teamID,
		embedding.EntityType,
		embedding.EntityID,
		embedding.VectorEmbeddings,
		embedding.Content,
		embedding.ModelID,
		embedding.CreatedAt,
		embedding.UpdatedAt,
	).Scan(&embedding.ID, &embedding.CreatedAt, &embedding.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create embedding: %w", err)
	}

	return nil
}

// GetByEntity retrieves all embeddings for a specific entity
func (r *EmbeddingRepository) GetByEntity(
	ctx context.Context, userID, entityType, entityID string,
) ([]models.Embedding, error) {
	query := `
		SELECT id, user_id, team_id, entity_type, entity_id, vector_embeddings, content, model_id, created_at, updated_at
		FROM embeddings
		WHERE user_id = $1 AND entity_type = $2 AND entity_id = $3
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("failed to get embeddings by entity: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	var embeddings []models.Embedding
	for rows.Next() {
		var embedding models.Embedding
		var teamID sql.NullString
		scanErr := rows.Scan(
			&embedding.ID,
			&embedding.UserID,
			&teamID,
			&embedding.EntityType,
			&embedding.EntityID,
			&embedding.VectorEmbeddings,
			&embedding.Content,
			&embedding.ModelID,
			&embedding.CreatedAt,
			&embedding.UpdatedAt,
		)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan embedding: %w", scanErr)
		}
		embedding.TeamID = teamID.String
		embeddings = append(embeddings, embedding)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate embeddings: %w", err)
	}

	return embeddings, nil
}

// FindSimilar finds embeddings similar to the given vector using cosine similarity
func (r *EmbeddingRepository) FindSimilar(
	ctx context.Context, userID, entityType string, vector []float32, limit int,
) ([]models.EmbeddingSimilarity, error) {
	query := `
		SELECT
			id, user_id, team_id, entity_type, entity_id, vector_embeddings, content,
			model_id, created_at, updated_at, vector_embeddings <=> $1 AS distance
		FROM embeddings
		WHERE user_id = $2 AND entity_type = $3
		ORDER BY vector_embeddings <=> $1
		LIMIT $4
	`

	// Convert float32 slice to pgvector.Vector
	searchVector := pgvector.NewVector(vector)

	rows, err := r.db.QueryContext(ctx, query, searchVector, userID, entityType, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find similar embeddings: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	var results []models.EmbeddingSimilarity
	for rows.Next() {
		var result models.EmbeddingSimilarity
		var teamID sql.NullString
		scanErr := rows.Scan(
			&result.ID,
			&result.UserID,
			&teamID,
			&result.EntityType,
			&result.EntityID,
			&result.VectorEmbeddings,
			&result.Content,
			&result.ModelID,
			&result.CreatedAt,
			&result.UpdatedAt,
			&result.Distance,
		)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan similarity result: %w", scanErr)
		}
		result.TeamID = teamID.String
		results = append(results, result)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate similarity results: %w", err)
	}

	return results, nil
}

// DeleteByEntity deletes all embeddings for a specific entity. It keys solely on
// (entity_type, entity_id) — entity_id is a globally-unique UUID, so every chunk
// for the entity is removed regardless of which user triggers the delete. This
// fixes the orphan bug where deleting another team member's content (deleter
// user_id != creator user_id) left the embedding rows behind.
func (r *EmbeddingRepository) DeleteByEntity(ctx context.Context, entityType, entityID string) error {
	query := `
		DELETE FROM embeddings
		WHERE entity_type = $1 AND entity_id = $2
	`

	if _, err := r.db.ExecContext(ctx, query, entityType, entityID); err != nil {
		return fmt.Errorf("failed to delete embeddings for entity: %w", err)
	}

	return nil
}

// DeleteByTeam removes every embedding owned by a team and returns the rows
// deleted.
func (r *EmbeddingRepository) DeleteByTeam(ctx context.Context, teamID string) (int64, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM embeddings WHERE team_id = $1`, teamID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete embeddings for team: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read rows affected: %w", err)
	}
	return rows, nil
}

// similarInTeamQuery ranks a team's embeddings by cosine distance to a bound
// query vector ($1), hydrating each neighbor's title per type (the same title
// expressions search.go uses) and dropping orphan chunks whose source row is
// gone. The query vector is a bound parameter, so the HNSW index accelerates it.
// $2 team_id, $3 model_id, $4/$5 the self (type,id) to exclude, $6 limit.
const similarInTeamQuery = `
	SELECT e.entity_type, e.entity_id,
		COALESCE(a.title, b.title, p.name, LEFT(m.text, 100), '') AS title,
		(1 - (e.vector_embeddings <=> $1)) AS score
	FROM embeddings e
	LEFT JOIN artifacts  a ON e.entity_type = 'artifact'  AND a.id = e.entity_id AND a.team_id = $2
	LEFT JOIN blueprints b ON e.entity_type = 'blueprint' AND b.id = e.entity_id AND b.team_id = $2
	LEFT JOIN prompts    p ON e.entity_type = 'prompt'    AND p.id = e.entity_id AND p.team_id = $2
	LEFT JOIN memories   m ON e.entity_type = 'memory'    AND m.id = e.entity_id AND m.team_id = $2
	WHERE e.team_id = $2
		AND e.model_id = $3
		AND NOT (e.entity_type = $4 AND e.entity_id = $5)
		AND e.entity_type IN ('artifact', 'memory', 'prompt', 'blueprint')
		AND (a.id IS NOT NULL OR b.id IS NOT NULL OR p.id IS NOT NULL OR m.id IS NOT NULL)
	ORDER BY e.vector_embeddings <=> $1 ASC
	LIMIT $6
`

func (r *EmbeddingRepository) FindSimilarInTeam(
	ctx context.Context, teamID, entityType, entityID string, limit int,
) ([]models.SimilarResource, error) {
	// Look up the resource's own stored vector; missing (embedding-worker lag)
	// degrades to an empty result, never an error.
	var vec pgvector.Vector
	var modelID string
	err := r.db.QueryRowContext(ctx,
		`SELECT vector_embeddings, model_id FROM embeddings
		 WHERE team_id = $1 AND entity_type = $2 AND entity_id = $3 LIMIT 1`,
		teamID, entityType, entityID,
	).Scan(&vec, &modelID)
	if errors.Is(err, sql.ErrNoRows) {
		return []models.SimilarResource{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load resource embedding: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, similarInTeamQuery, vec, teamID, modelID, entityType, entityID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find similar resources: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close similar-resource rows", "error", closeErr)
		}
	}()

	similar := make([]models.SimilarResource, 0, limit)
	for rows.Next() {
		var s models.SimilarResource
		if scanErr := rows.Scan(&s.Type, &s.ID, &s.Title, &s.Score); scanErr != nil {
			return nil, fmt.Errorf("failed to scan similar resource: %w", scanErr)
		}
		similar = append(similar, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate similar resources: %w", err)
	}
	return similar, nil
}
