package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pgvector/pgvector-go"
	"github.com/sirupsen/logrus"

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
			logrus.WithError(closeErr).Error("Failed to close rows")
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
			logrus.WithError(closeErr).Error("Failed to close rows")
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
