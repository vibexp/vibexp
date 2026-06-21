package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

func setupEmbeddingTest(t *testing.T) (*EmbeddingRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewEmbeddingRepository(db)

	return repo.(*EmbeddingRepository), mock, mockDB
}

//nolint:funlen // Test function with comprehensive scenarios
func TestEmbeddingRepository_Create(t *testing.T) {
	repo, mock, mockDB := setupEmbeddingTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	vector := pgvector.NewVector([]float32{0.1, 0.2, 0.3})

	embedding := &models.Embedding{
		UserID:           "user-123",
		TeamID:           "team-789",
		EntityType:       "prompt",
		EntityID:         "prompt-456",
		VectorEmbeddings: vector,
		Content:          "test content",
		ModelID:          "test-model",
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	// noTeamEmbedding has no resolved team, so Create must bind SQL NULL (an empty
	// string is not a valid UUID for the nullable team_id column).
	noTeamEmbedding := &models.Embedding{
		UserID:           "user-123",
		TeamID:           "",
		EntityType:       "prompt",
		EntityID:         "prompt-456",
		VectorEmbeddings: vector,
		Content:          "test content",
		ModelID:          "test-model",
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	tests := []struct {
		name      string
		embedding *models.Embedding
		setupMock func()
		expectErr bool
	}{
		{
			name:      "successful creation persists team_id",
			embedding: embedding,
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
					AddRow("emb-123", now, now)

				mock.ExpectQuery(`INSERT INTO embeddings`).
					WithArgs(
						embedding.UserID,
						embedding.TeamID,
						embedding.EntityType,
						embedding.EntityID,
						embedding.VectorEmbeddings,
						embedding.Content,
						embedding.ModelID,
						embedding.CreatedAt,
						embedding.UpdatedAt,
					).
					WillReturnRows(rows)
			},
			expectErr: false,
		},
		{
			name:      "empty team_id binds SQL NULL",
			embedding: noTeamEmbedding,
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
					AddRow("emb-124", now, now)

				mock.ExpectQuery(`INSERT INTO embeddings`).
					WithArgs(
						noTeamEmbedding.UserID,
						nil, // team_id bound as NULL
						noTeamEmbedding.EntityType,
						noTeamEmbedding.EntityID,
						noTeamEmbedding.VectorEmbeddings,
						noTeamEmbedding.Content,
						noTeamEmbedding.ModelID,
						noTeamEmbedding.CreatedAt,
						noTeamEmbedding.UpdatedAt,
					).
					WillReturnRows(rows)
			},
			expectErr: false,
		},
		{
			name:      "database error",
			embedding: embedding,
			setupMock: func() {
				mock.ExpectQuery(`INSERT INTO embeddings`).
					WithArgs(
						embedding.UserID,
						embedding.TeamID,
						embedding.EntityType,
						embedding.EntityID,
						embedding.VectorEmbeddings,
						embedding.Content,
						embedding.ModelID,
						embedding.CreatedAt,
						embedding.UpdatedAt,
					).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.Create(ctx, tt.embedding)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, tt.embedding.ID)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // Test function with comprehensive scenarios
func TestEmbeddingRepository_GetByEntity(t *testing.T) {
	repo, mock, mockDB := setupEmbeddingTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	vector := pgvector.NewVector([]float32{0.1, 0.2, 0.3})

	tests := []struct {
		name       string
		userID     string
		entityType string
		entityID   string
		setupMock  func()
		expectErr  bool
		expectLen  int
	}{
		{
			name:       "successful retrieval with multiple embeddings",
			userID:     "user-123",
			entityType: "prompt",
			entityID:   "prompt-456",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "user_id", "team_id", "entity_type", "entity_id",
					"vector_embeddings", "content", "model_id", "created_at", "updated_at",
				}).
					AddRow("emb-1", "user-123", "team-789", "prompt", "prompt-456", vector, "content 1", "model-1", now, now).
					AddRow("emb-2", "user-123", "team-789", "prompt", "prompt-456", vector, "content 2", "model-2", now, now)

				mock.ExpectQuery(`SELECT (.+) FROM embeddings WHERE`).
					WithArgs("user-123", "prompt", "prompt-456").
					WillReturnRows(rows)
			},
			expectErr: false,
			expectLen: 2,
		},
		{
			name:       "no embeddings found",
			userID:     "user-123",
			entityType: "artifact",
			entityID:   "artifact-789",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "user_id", "team_id", "entity_type", "entity_id",
					"vector_embeddings", "content", "model_id", "created_at", "updated_at",
				})

				mock.ExpectQuery(`SELECT (.+) FROM embeddings WHERE`).
					WithArgs("user-123", "artifact", "artifact-789").
					WillReturnRows(rows)
			},
			expectErr: false,
			expectLen: 0,
		},
		{
			name:       "database error",
			userID:     "user-123",
			entityType: "memory",
			entityID:   "memory-999",
			setupMock: func() {
				mock.ExpectQuery(`SELECT (.+) FROM embeddings WHERE`).
					WithArgs("user-123", "memory", "memory-999").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
			expectLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			embeddings, err := repo.GetByEntity(ctx, tt.userID, tt.entityType, tt.entityID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, embeddings)
			} else {
				assert.NoError(t, err)
				assert.Len(t, embeddings, tt.expectLen)
				for _, emb := range embeddings {
					assert.Equal(t, "team-789", emb.TeamID, "team_id must round-trip from the row")
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // Test function with comprehensive scenarios
func TestEmbeddingRepository_FindSimilar(t *testing.T) {
	repo, mock, mockDB := setupEmbeddingTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()
	now := time.Now()
	searchVector := []float32{0.1, 0.2, 0.3}
	storedVector := pgvector.NewVector([]float32{0.11, 0.21, 0.31})

	tests := []struct {
		name       string
		userID     string
		entityType string
		vector     []float32
		limit      int
		setupMock  func()
		expectErr  bool
		expectLen  int
	}{
		{
			name:       "successful similarity search",
			userID:     "user-123",
			entityType: "prompt",
			vector:     searchVector,
			limit:      5,
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "user_id", "team_id", "entity_type", "entity_id",
					"vector_embeddings", "content", "model_id", "created_at", "updated_at", "distance",
				}).
					AddRow("emb-1", "user-123", "team-789", "prompt", "prompt-1",
						storedVector, "content 1", "model-1", now, now, 0.05).
					AddRow("emb-2", "user-123", "team-789", "prompt", "prompt-2",
						storedVector, "content 2", "model-1", now, now, 0.10)

				mock.ExpectQuery(`SELECT (.+) FROM embeddings WHERE`).
					WithArgs(sqlmock.AnyArg(), "user-123", "prompt", 5).
					WillReturnRows(rows)
			},
			expectErr: false,
			expectLen: 2,
		},
		{
			name:       "no similar embeddings found",
			userID:     "user-123",
			entityType: "artifact",
			vector:     searchVector,
			limit:      10,
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "user_id", "team_id", "entity_type", "entity_id",
					"vector_embeddings", "content", "model_id", "created_at", "updated_at", "distance",
				})

				mock.ExpectQuery(`SELECT (.+) FROM embeddings WHERE`).
					WithArgs(sqlmock.AnyArg(), "user-123", "artifact", 10).
					WillReturnRows(rows)
			},
			expectErr: false,
			expectLen: 0,
		},
		{
			name:       "database error",
			userID:     "user-123",
			entityType: "memory",
			vector:     searchVector,
			limit:      5,
			setupMock: func() {
				mock.ExpectQuery(`SELECT (.+) FROM embeddings WHERE`).
					WithArgs(sqlmock.AnyArg(), "user-123", "memory", 5).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
			expectLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			results, err := repo.FindSimilar(ctx, tt.userID, tt.entityType, tt.vector, tt.limit)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, results)
			} else {
				assert.NoError(t, err)
				assert.Len(t, results, tt.expectLen)
				if tt.expectLen > 0 {
					// Verify distance values are included
					for _, result := range results {
						assert.Greater(t, result.Distance, float64(-1))
						assert.Equal(t, "team-789", result.TeamID, "team_id must round-trip from the row")
					}
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestEmbeddingRepository_DeleteByEntity asserts the entity-scoped delete keys
// solely on (entity_type, entity_id) and removes the row regardless of the
// creator's user_id (the orphan-fix contract). The deleter's identity is never
// part of the query.
func TestEmbeddingRepository_DeleteByEntity(t *testing.T) {
	repo, mock, mockDB := setupEmbeddingTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := context.Background()

	t.Run("deletes by entity regardless of creator", func(t *testing.T) {
		// The DELETE binds only entity_type and entity_id — no user_id. This is what
		// lets a team admin remove another member's embedding rows.
		mock.ExpectExec(`DELETE FROM embeddings\s+WHERE entity_type = \$1 AND entity_id = \$2`).
			WithArgs("prompt", "prompt-456").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.DeleteByEntity(ctx, "prompt", "prompt-456")

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no rows deleted is not an error", func(t *testing.T) {
		mock.ExpectExec(`DELETE FROM embeddings`).
			WithArgs("memory", "memory-999").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.DeleteByEntity(ctx, "memory", "memory-999")

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("database error is wrapped", func(t *testing.T) {
		mock.ExpectExec(`DELETE FROM embeddings`).
			WithArgs("artifact", "artifact-1").
			WillReturnError(sql.ErrConnDone)

		err := repo.DeleteByEntity(ctx, "artifact", "artifact-1")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete embeddings for entity")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
