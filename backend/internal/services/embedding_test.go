package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// testEmbeddingDimensions matches the dimensionality of createTestVector384 so the
// service's vector-length checks pass for the existing fixtures.
const testEmbeddingDimensions = 384

func createTestEmbeddingService(
	repo *mocks.MockEmbeddingRepository,
	promptRepo *mocks.MockPromptRepository,
	artifactRepo *mocks.MockArtifactRepository,
	memoryRepo *mocks.MockMemoryRepository,
	blueprintRepo *mocks.MockBlueprintRepository,
) *EmbeddingService {
	return NewEmbeddingService(
		repo,
		promptRepo,
		artifactRepo,
		memoryRepo,
		blueprintRepo,
		nil, // feedItemRepo — not exercised by non-feed tests
		nil, // feedItemReplyRepo — not exercised by non-feed tests
		testEmbeddingDimensions,
		func() *slog.Logger { l, _ := logtest.New(); return l }(),
	)
}

// createTestEmbeddingServiceWithFeed builds an EmbeddingService wired with the feed
// repositories so feed_item / feed_item_reply validation can be exercised. Non-feed
// repos are left nil since feed tests don't touch them.
func createTestEmbeddingServiceWithFeed(
	repo *mocks.MockEmbeddingRepository,
	feedItemRepo *mocks.MockFeedItemRepository,
	feedItemReplyRepo *mocks.MockFeedItemReplyRepository,
) *EmbeddingService {
	return NewEmbeddingService(
		repo,
		nil,
		nil,
		nil,
		nil,
		feedItemRepo,
		feedItemReplyRepo,
		testEmbeddingDimensions,
		func() *slog.Logger { l, _ := logtest.New(); return l }(),
	)
}

func createTestVector384() []float32 {
	vector := make([]float32, 384)
	for i := range vector {
		vector[i] = float32(i) / 384.0
	}
	return vector
}

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmbeddingService_SaveEmbedding(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		entityType string
		entityID   string
		modelID    string
		vectors    [][]float32
		setupMocks func(
			*mocks.MockEmbeddingRepository,
			*mocks.MockPromptRepository,
			*mocks.MockArtifactRepository,
			*mocks.MockMemoryRepository,
			*mocks.MockBlueprintRepository,
		)
		expectErr  bool
		errMessage string
	}{
		{
			name:       "successful save for prompt",
			userID:     "user-123",
			entityType: "prompt",
			entityID:   "prompt-456",
			modelID:    "test-model",
			vectors:    [][]float32{createTestVector384()},
			setupMocks: func(
				embRepo *mocks.MockEmbeddingRepository,
				promptRepo *mocks.MockPromptRepository,
				_ *mocks.MockArtifactRepository,
				_ *mocks.MockMemoryRepository,
				_ *mocks.MockBlueprintRepository,
			) {
				promptRepo.EXPECT().GetByIDCrossTeam(
					mock.Anything, "user-123", "prompt-456",
				).Return(&models.Prompt{}, nil).Once()
				embRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(emb *models.Embedding) bool {
					return emb.UserID == "user-123" &&
						emb.EntityType == "prompt" &&
						emb.EntityID == "prompt-456" &&
						emb.ModelID == "test-model"
				})).Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:       "successful save for artifact with multiple vectors",
			userID:     "user-123",
			entityType: "artifact",
			entityID:   "artifact-789",
			modelID:    "test-model",
			vectors:    [][]float32{createTestVector384(), createTestVector384()},
			setupMocks: func(
				embRepo *mocks.MockEmbeddingRepository,
				_ *mocks.MockPromptRepository,
				artifactRepo *mocks.MockArtifactRepository,
				_ *mocks.MockMemoryRepository,
				_ *mocks.MockBlueprintRepository,
			) {
				artifactRepo.EXPECT().GetByIDCrossTeam(
					mock.Anything, "user-123", "artifact-789",
				).Return(&models.Artifact{}, nil).Once()
				embRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Times(2)
			},
			expectErr: false,
		},
		{
			name:       "successful save for memory",
			userID:     "user-123",
			entityType: "memory",
			entityID:   "memory-999",
			modelID:    "test-model",
			vectors:    [][]float32{createTestVector384()},
			setupMocks: func(
				embRepo *mocks.MockEmbeddingRepository,
				_ *mocks.MockPromptRepository,
				_ *mocks.MockArtifactRepository,
				memoryRepo *mocks.MockMemoryRepository,
				_ *mocks.MockBlueprintRepository,
			) {
				memoryRepo.EXPECT().GetByIDCrossTeam(
					mock.Anything, "user-123", "memory-999",
				).Return(&models.Memory{}, nil).Once()
				embRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:       "successful save for blueprint",
			userID:     "user-123",
			entityType: "blueprint",
			entityID:   "blueprint-321",
			modelID:    "test-model",
			vectors:    [][]float32{createTestVector384()},
			setupMocks: func(
				embRepo *mocks.MockEmbeddingRepository,
				_ *mocks.MockPromptRepository,
				_ *mocks.MockArtifactRepository,
				_ *mocks.MockMemoryRepository,
				blueprintRepo *mocks.MockBlueprintRepository,
			) {
				blueprintRepo.EXPECT().GetByIDCrossTeam(
					mock.Anything, "user-123", "blueprint-321",
				).Return(&models.Blueprint{}, nil).Once()
				embRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(emb *models.Embedding) bool {
					return emb.UserID == "user-123" &&
						emb.EntityType == "blueprint" &&
						emb.EntityID == "blueprint-321" &&
						emb.ModelID == "test-model"
				})).Return(nil).Once()
			},
			expectErr: false,
		},
		{
			name:       "entity not found",
			userID:     "user-123",
			entityType: "prompt",
			entityID:   "prompt-nonexistent",
			modelID:    "test-model",
			vectors:    [][]float32{createTestVector384()},
			setupMocks: func(
				_ *mocks.MockEmbeddingRepository,
				promptRepo *mocks.MockPromptRepository,
				_ *mocks.MockArtifactRepository,
				_ *mocks.MockMemoryRepository,
				_ *mocks.MockBlueprintRepository,
			) {
				promptRepo.EXPECT().GetByIDCrossTeam(
					mock.Anything,
					"user-123",
					"prompt-nonexistent",
				).Return(nil, fmt.Errorf("not found")).Once()
			},
			expectErr:  true,
			errMessage: "entity validation failed",
		},
		{
			name:       "invalid vector dimension",
			userID:     "user-123",
			entityType: "prompt",
			entityID:   "prompt-456",
			modelID:    "test-model",
			vectors:    [][]float32{{0.1, 0.2}}, // Only 2 dimensions
			setupMocks: func(
				_ *mocks.MockEmbeddingRepository,
				promptRepo *mocks.MockPromptRepository,
				_ *mocks.MockArtifactRepository,
				_ *mocks.MockMemoryRepository,
				_ *mocks.MockBlueprintRepository,
			) {
				promptRepo.EXPECT().GetByIDCrossTeam(
					mock.Anything, "user-123", "prompt-456",
				).Return(&models.Prompt{}, nil).Once()
			},
			expectErr:  true,
			errMessage: "invalid vector dimension",
		},
		{
			name:       "unsupported entity type",
			userID:     "user-123",
			entityType: "unknown",
			entityID:   "entity-123",
			modelID:    "test-model",
			vectors:    [][]float32{createTestVector384()},
			setupMocks: func(
				_ *mocks.MockEmbeddingRepository,
				_ *mocks.MockPromptRepository,
				_ *mocks.MockArtifactRepository,
				_ *mocks.MockMemoryRepository,
				_ *mocks.MockBlueprintRepository,
			) {
				// No mocks needed as validation will fail immediately
			},
			expectErr:  true,
			errMessage: "unsupported entity type: unknown",
		},
		{
			name:       "database error during save",
			userID:     "user-123",
			entityType: "prompt",
			entityID:   "prompt-456",
			modelID:    "test-model",
			vectors:    [][]float32{createTestVector384()},
			setupMocks: func(
				embRepo *mocks.MockEmbeddingRepository,
				promptRepo *mocks.MockPromptRepository,
				_ *mocks.MockArtifactRepository,
				_ *mocks.MockMemoryRepository,
				_ *mocks.MockBlueprintRepository,
			) {
				promptRepo.EXPECT().GetByIDCrossTeam(
					mock.Anything, "user-123", "prompt-456",
				).Return(&models.Prompt{}, nil).Once()
				embRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(fmt.Errorf("database error")).Once()
			},
			expectErr:  true,
			errMessage: "failed to save embedding",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embRepo := mocks.NewMockEmbeddingRepository(t)
			promptRepo := mocks.NewMockPromptRepository(t)
			artifactRepo := mocks.NewMockArtifactRepository(t)
			memoryRepo := mocks.NewMockMemoryRepository(t)
			blueprintRepo := mocks.NewMockBlueprintRepository(t)

			tt.setupMocks(embRepo, promptRepo, artifactRepo, memoryRepo, blueprintRepo)

			service := createTestEmbeddingService(embRepo, promptRepo, artifactRepo, memoryRepo, blueprintRepo)
			err := service.SaveEmbedding(tt.userID, tt.entityType, tt.entityID, tt.modelID, tt.vectors)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmbeddingService_GetEmbeddingsByEntity(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		entityType string
		entityID   string
		setupMocks func(*mocks.MockEmbeddingRepository)
		expectErr  bool
		expectLen  int
	}{
		{
			name:       "successful retrieval",
			userID:     "user-123",
			entityType: "prompt",
			entityID:   "prompt-456",
			setupMocks: func(embRepo *mocks.MockEmbeddingRepository) {
				embeddings := []models.Embedding{
					{
						ID:               "emb-1",
						UserID:           "user-123",
						EntityType:       "prompt",
						EntityID:         "prompt-456",
						VectorEmbeddings: pgvector.NewVector(createTestVector384()),
						ModelID:          "model-1",
					},
					{
						ID:               "emb-2",
						UserID:           "user-123",
						EntityType:       "prompt",
						EntityID:         "prompt-456",
						VectorEmbeddings: pgvector.NewVector(createTestVector384()),
						ModelID:          "model-1",
					},
				}
				embRepo.EXPECT().GetByEntity(mock.Anything, "user-123", "prompt", "prompt-456").Return(embeddings, nil).Once()
			},
			expectErr: false,
			expectLen: 2,
		},
		{
			name:       "no embeddings found",
			userID:     "user-123",
			entityType: "artifact",
			entityID:   "artifact-999",
			setupMocks: func(embRepo *mocks.MockEmbeddingRepository) {
				embRepo.EXPECT().GetByEntity(
					mock.Anything,
					"user-123",
					"artifact",
					"artifact-999",
				).Return([]models.Embedding{}, nil).Once()
			},
			expectErr: false,
			expectLen: 0,
		},
		{
			name:       "database error",
			userID:     "user-123",
			entityType: "memory",
			entityID:   "memory-789",
			setupMocks: func(embRepo *mocks.MockEmbeddingRepository) {
				embRepo.EXPECT().GetByEntity(
					mock.Anything,
					"user-123",
					"memory",
					"memory-789",
				).Return(nil, fmt.Errorf("database error")).Once()
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embRepo := mocks.NewMockEmbeddingRepository(t)
			tt.setupMocks(embRepo)

			service := createTestEmbeddingService(embRepo, nil, nil, nil, nil)
			embeddings, err := service.GetEmbeddingsByEntity(tt.userID, tt.entityType, tt.entityID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, embeddings)
			} else {
				assert.NoError(t, err)
				assert.Len(t, embeddings, tt.expectLen)
			}
		})
	}
	//nolint:funlen // Test function requires comprehensive setup and assertions
}

//nolint:funlen // Test function requires comprehensive setup and assertions

//nolint:funlen // Test function requires comprehensive setup and assertions
func TestEmbeddingService_FindSimilar(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		entityType string
		vector     []float32
		limit      int
		setupMocks func(*mocks.MockEmbeddingRepository)
		expectErr  bool
		expectLen  int
	}{
		{
			name:       "successful similarity search",
			userID:     "user-123",
			entityType: "prompt",
			vector:     createTestVector384(),
			limit:      5,
			setupMocks: func(embRepo *mocks.MockEmbeddingRepository) {
				results := []models.EmbeddingSimilarity{
					{
						Embedding: models.Embedding{
							ID:               "emb-1",
							UserID:           "user-123",
							EntityType:       "prompt",
							EntityID:         "prompt-1",
							VectorEmbeddings: pgvector.NewVector(createTestVector384()),
							ModelID:          "model-1",
						},
						Distance: 0.05,
					},
					{
						Embedding: models.Embedding{
							ID:               "emb-2",
							UserID:           "user-123",
							EntityType:       "prompt",
							EntityID:         "prompt-2",
							VectorEmbeddings: pgvector.NewVector(createTestVector384()),
							ModelID:          "model-1",
						},
						Distance: 0.10,
					},
				}
				embRepo.EXPECT().FindSimilar(
					mock.Anything,
					"user-123",
					"prompt",
					createTestVector384(),
					5,
				).Return(results, nil).Once()
			},
			expectErr: false,
			expectLen: 2,
		},
		{
			name:       "invalid vector dimension",
			userID:     "user-123",
			entityType: "prompt",
			vector:     []float32{0.1, 0.2},
			limit:      5,
			setupMocks: func(_ *mocks.MockEmbeddingRepository) {
				// No mock needed as validation fails early
			},
			expectErr: true,
		},
		{
			name:       "database error",
			userID:     "user-123",
			entityType: "artifact",
			vector:     createTestVector384(),
			limit:      10,
			setupMocks: func(embRepo *mocks.MockEmbeddingRepository) {
				embRepo.EXPECT().FindSimilar(
					mock.Anything,
					"user-123",
					"artifact",
					createTestVector384(),
					10,
				).Return(nil, fmt.Errorf("database error")).Once()
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embRepo := mocks.NewMockEmbeddingRepository(t)
			tt.setupMocks(embRepo)

			service := createTestEmbeddingService(embRepo, nil, nil, nil, nil)
			results, err := service.FindSimilar(tt.userID, tt.entityType, tt.vector, tt.limit)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, results)
			} else {
				assert.NoError(t, err)
				assert.Len(t, results, tt.expectLen)
			}
		})
	}
}

func makeChunk(content string) EmbeddingChunk {
	return EmbeddingChunk{Embedding: createTestVector384(), Content: content}
}

// TestEmbeddingService_SaveEmbeddingChunks_IsIdempotent verifies that
// SaveEmbeddingChunks deletes any pre-existing embeddings for the entity before
// inserting the new chunks. The order MUST be: validate → delete → insert.
// Without this, repeated calls (e.g. on entity update) accumulate stale vectors
// and silently corrupt similarity search.
//
//nolint:funlen // Table-driven test with mock-ordering setup; matches existing pattern in this file.
func TestEmbeddingService_SaveEmbeddingChunks_IsIdempotent(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		entityType string
		entityID   string
		modelID    string
		chunks     []EmbeddingChunk
		setupMocks func(
			*mocks.MockEmbeddingRepository,
			*mocks.MockPromptRepository,
			*mocks.MockArtifactRepository,
			*mocks.MockMemoryRepository,
			*mocks.MockBlueprintRepository,
		)
		expectErr  bool
		errMessage string
	}{
		{
			name:       "single chunk: validate then delete then insert",
			userID:     "user-123",
			entityType: "prompt",
			entityID:   "prompt-1",
			modelID:    "test-model",
			chunks:     []EmbeddingChunk{makeChunk("chunk-1")},
			setupMocks: func(
				embRepo *mocks.MockEmbeddingRepository,
				promptRepo *mocks.MockPromptRepository,
				_ *mocks.MockArtifactRepository,
				_ *mocks.MockMemoryRepository,
				_ *mocks.MockBlueprintRepository,
			) {
				validate := promptRepo.EXPECT().GetByIDCrossTeam(
					mock.Anything, "user-123", "prompt-1",
				).Return(&models.Prompt{TeamID: "team-prompt"}, nil).Once()
				del := embRepo.EXPECT().DeleteByEntity(
					mock.Anything, "prompt", "prompt-1",
				).Return(nil).Once().NotBefore(validate)
				embRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(emb *models.Embedding) bool {
					return emb.UserID == "user-123" &&
						emb.TeamID == "team-prompt" &&
						emb.EntityType == "prompt" &&
						emb.EntityID == "prompt-1" &&
						emb.Content == "chunk-1"
				})).Return(nil).Once().NotBefore(del)
			},
			expectErr: false,
		},
		{
			name:       "multiple chunks: delete called once, create called per chunk",
			userID:     "user-123",
			entityType: "artifact",
			entityID:   "artifact-1",
			modelID:    "test-model",
			chunks: []EmbeddingChunk{
				makeChunk("chunk-a"),
				makeChunk("chunk-b"),
				makeChunk("chunk-c"),
			},
			setupMocks: func(
				embRepo *mocks.MockEmbeddingRepository,
				_ *mocks.MockPromptRepository,
				artifactRepo *mocks.MockArtifactRepository,
				_ *mocks.MockMemoryRepository,
				_ *mocks.MockBlueprintRepository,
			) {
				validate := artifactRepo.EXPECT().GetByIDCrossTeam(
					mock.Anything, "user-123", "artifact-1",
				).Return(&models.Artifact{}, nil).Once()
				del := embRepo.EXPECT().DeleteByEntity(
					mock.Anything, "artifact", "artifact-1",
				).Return(nil).Once().NotBefore(validate)
				embRepo.EXPECT().Create(mock.Anything, mock.Anything).
					Return(nil).Times(3).NotBefore(del)
			},
			expectErr: false,
		},
		{
			name:       "memory entity: full pipeline works",
			userID:     "user-123",
			entityType: "memory",
			entityID:   "memory-1",
			modelID:    "test-model",
			chunks:     []EmbeddingChunk{makeChunk("memory-content")},
			setupMocks: func(
				embRepo *mocks.MockEmbeddingRepository,
				_ *mocks.MockPromptRepository,
				_ *mocks.MockArtifactRepository,
				memoryRepo *mocks.MockMemoryRepository,
				_ *mocks.MockBlueprintRepository,
			) {
				validate := memoryRepo.EXPECT().GetByIDCrossTeam(
					mock.Anything, "user-123", "memory-1",
				).Return(&models.Memory{}, nil).Once()
				del := embRepo.EXPECT().DeleteByEntity(
					mock.Anything, "memory", "memory-1",
				).Return(nil).Once().NotBefore(validate)
				embRepo.EXPECT().Create(mock.Anything, mock.Anything).
					Return(nil).Once().NotBefore(del)
			},
			expectErr: false,
		},
		{
			name:       "blueprint entity: full pipeline works",
			userID:     "user-123",
			entityType: "blueprint",
			entityID:   "blueprint-1",
			modelID:    "test-model",
			chunks:     []EmbeddingChunk{makeChunk("blueprint-content")},
			setupMocks: func(
				embRepo *mocks.MockEmbeddingRepository,
				_ *mocks.MockPromptRepository,
				_ *mocks.MockArtifactRepository,
				_ *mocks.MockMemoryRepository,
				blueprintRepo *mocks.MockBlueprintRepository,
			) {
				validate := blueprintRepo.EXPECT().GetByIDCrossTeam(
					mock.Anything, "user-123", "blueprint-1",
				).Return(&models.Blueprint{TeamID: "team-bp"}, nil).Once()
				del := embRepo.EXPECT().DeleteByEntity(
					mock.Anything, "blueprint", "blueprint-1",
				).Return(nil).Once().NotBefore(validate)
				embRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(emb *models.Embedding) bool {
					return emb.UserID == "user-123" &&
						emb.TeamID == "team-bp" &&
						emb.EntityType == "blueprint" &&
						emb.EntityID == "blueprint-1" &&
						emb.Content == "blueprint-content"
				})).Return(nil).Once().NotBefore(del)
			},
			expectErr: false,
		},
		{
			name:       "validation failure: delete is NOT called",
			userID:     "user-123",
			entityType: "prompt",
			entityID:   "missing-prompt",
			modelID:    "test-model",
			chunks:     []EmbeddingChunk{makeChunk("chunk-1")},
			setupMocks: func(
				_ *mocks.MockEmbeddingRepository,
				promptRepo *mocks.MockPromptRepository,
				_ *mocks.MockArtifactRepository,
				_ *mocks.MockMemoryRepository,
				_ *mocks.MockBlueprintRepository,
			) {
				promptRepo.EXPECT().GetByIDCrossTeam(
					mock.Anything, "user-123", "missing-prompt",
				).Return(nil, fmt.Errorf("not found")).Once()
				// Crucially, no DeleteByEntity expectation — mockery would fail the test
				// if it were called when not expected.
			},
			expectErr:  true,
			errMessage: "entity validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embRepo := mocks.NewMockEmbeddingRepository(t)
			promptRepo := mocks.NewMockPromptRepository(t)
			artifactRepo := mocks.NewMockArtifactRepository(t)
			memoryRepo := mocks.NewMockMemoryRepository(t)
			blueprintRepo := mocks.NewMockBlueprintRepository(t)

			tt.setupMocks(embRepo, promptRepo, artifactRepo, memoryRepo, blueprintRepo)

			service := createTestEmbeddingService(embRepo, promptRepo, artifactRepo, memoryRepo, blueprintRepo)
			err := service.SaveEmbeddingChunks(tt.userID, tt.entityType, tt.entityID, tt.modelID, tt.chunks)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMessage != "" {
					assert.Contains(t, err.Error(), tt.errMessage)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestEmbeddingService_SaveEmbeddingChunks_DeleteFailure verifies that when the
// delete step fails, the function returns a wrapped error and does NOT proceed
// to insert any new embeddings (which would create duplicates).
func TestEmbeddingService_SaveEmbeddingChunks_DeleteFailure(t *testing.T) {
	embRepo := mocks.NewMockEmbeddingRepository(t)
	promptRepo := mocks.NewMockPromptRepository(t)
	artifactRepo := mocks.NewMockArtifactRepository(t)
	memoryRepo := mocks.NewMockMemoryRepository(t)
	blueprintRepo := mocks.NewMockBlueprintRepository(t)

	promptRepo.EXPECT().GetByIDCrossTeam(
		mock.Anything, "user-123", "prompt-1",
	).Return(&models.Prompt{}, nil).Once()
	embRepo.EXPECT().DeleteByEntity(
		mock.Anything, "prompt", "prompt-1",
	).Return(fmt.Errorf("db unavailable")).Once()
	// No Create expectation — Create must not be invoked after a delete failure.

	service := createTestEmbeddingService(embRepo, promptRepo, artifactRepo, memoryRepo, blueprintRepo)
	err := service.SaveEmbeddingChunks(
		"user-123", "prompt", "prompt-1", "test-model",
		[]EmbeddingChunk{makeChunk("chunk-1")},
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete existing embeddings before save")
	assert.Contains(t, err.Error(), "db unavailable")
}

// TestEmbeddingService_SaveEmbeddingChunks_InvalidVectorRejectedBeforeDelete verifies
// that a chunk with the wrong vector dimension is rejected BEFORE any delete or insert
// happens, so a poison message can never wipe an entity's existing embeddings.
// This is the safety contract that guards against the worst-case PubSub retry loop:
// a non-retryable validation failure that would otherwise erase the corpus on every retry.
func TestEmbeddingService_SaveEmbeddingChunks_InvalidVectorRejectedBeforeDelete(t *testing.T) {
	embRepo := mocks.NewMockEmbeddingRepository(t)
	promptRepo := mocks.NewMockPromptRepository(t)
	artifactRepo := mocks.NewMockArtifactRepository(t)
	memoryRepo := mocks.NewMockMemoryRepository(t)
	blueprintRepo := mocks.NewMockBlueprintRepository(t)

	promptRepo.EXPECT().GetByIDCrossTeam(
		mock.Anything, "user-123", "prompt-1",
	).Return(&models.Prompt{}, nil).Once()
	// No DeleteByEntity expectation — validation must fail BEFORE any state mutation.
	// No Create expectation — likewise.

	service := createTestEmbeddingService(embRepo, promptRepo, artifactRepo, memoryRepo, blueprintRepo)
	err := service.SaveEmbeddingChunks(
		"user-123", "prompt", "prompt-1", "test-model",
		[]EmbeddingChunk{
			makeChunk("good"), // first chunk valid
			{Embedding: []float32{0.1, 0.2}, Content: "bad"}, // second chunk invalid
		},
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid vector dimension at chunk 1")
}

// TestEmbeddingService_SaveEmbeddingChunks_PartialInsertFailure documents the
// known partial-failure contract: validation passes for every chunk, delete succeeds,
// but a Create call mid-batch fails. The function returns the wrapped error; the entity
// is left with fewer chunks than supplied. Caller (PubSub or HTTP-sync handler) must retry.
func TestEmbeddingService_SaveEmbeddingChunks_PartialInsertFailure(t *testing.T) {
	embRepo := mocks.NewMockEmbeddingRepository(t)
	promptRepo := mocks.NewMockPromptRepository(t)
	artifactRepo := mocks.NewMockArtifactRepository(t)
	memoryRepo := mocks.NewMockMemoryRepository(t)
	blueprintRepo := mocks.NewMockBlueprintRepository(t)

	promptRepo.EXPECT().GetByIDCrossTeam(
		mock.Anything, "user-123", "prompt-1",
	).Return(&models.Prompt{}, nil).Once()
	embRepo.EXPECT().DeleteByEntity(
		mock.Anything, "prompt", "prompt-1",
	).Return(nil).Once()
	// First Create succeeds, second fails. The function exits on the first error,
	// so only one Create is observed; chunk-3 is never reached.
	embRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
	embRepo.EXPECT().Create(mock.Anything, mock.Anything).
		Return(fmt.Errorf("connection reset")).Once()

	service := createTestEmbeddingService(embRepo, promptRepo, artifactRepo, memoryRepo, blueprintRepo)
	err := service.SaveEmbeddingChunks(
		"user-123", "prompt", "prompt-1", "test-model",
		[]EmbeddingChunk{makeChunk("a"), makeChunk("b"), makeChunk("c")},
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save embedding")
	assert.Contains(t, err.Error(), "connection reset")
}

// TestEmbeddingService_DeleteEmbeddingsByEntity verifies the entity-scoped delete
// path: the service forwards only (entityType, entityID) to the repository (no
// user_id), so the delete succeeds regardless of which member triggers it, and a
// repository error is wrapped for the caller.
func TestEmbeddingService_DeleteEmbeddingsByEntity(t *testing.T) {
	t.Run("success forwards entity-only key", func(t *testing.T) {
		embRepo := mocks.NewMockEmbeddingRepository(t)
		embRepo.EXPECT().DeleteByEntity(mock.Anything, "prompt", "prompt-1").Return(nil).Once()

		service := createTestEmbeddingService(embRepo, nil, nil, nil, nil)
		assert.NoError(t, service.DeleteEmbeddingsByEntity("prompt", "prompt-1"))
	})

	t.Run("wraps repository error", func(t *testing.T) {
		embRepo := mocks.NewMockEmbeddingRepository(t)
		embRepo.EXPECT().DeleteByEntity(mock.Anything, "memory", "memory-1").
			Return(fmt.Errorf("db unavailable")).Once()

		service := createTestEmbeddingService(embRepo, nil, nil, nil, nil)
		err := service.DeleteEmbeddingsByEntity("memory", "memory-1")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete embeddings")
		assert.Contains(t, err.Error(), "db unavailable")
	})
}

// runBuiltinValidatorErrorWrapTest exercises one built-in entity type's
// ValidatorFunc error path and asserts the per-type "<entity> not found:" wrap.
// Extracted from a parent test to stay below the 60-line funlen budget without
// adding nolint suppressions.
func runBuiltinValidatorErrorWrapTest(
	t *testing.T,
	entityType string,
	wantPrefix string,
	setup func(
		*mocks.MockPromptRepository,
		*mocks.MockArtifactRepository,
		*mocks.MockMemoryRepository,
		*mocks.MockBlueprintRepository,
	),
) {
	t.Helper()
	embRepo := mocks.NewMockEmbeddingRepository(t)
	promptRepo := mocks.NewMockPromptRepository(t)
	artifactRepo := mocks.NewMockArtifactRepository(t)
	memoryRepo := mocks.NewMockMemoryRepository(t)
	blueprintRepo := mocks.NewMockBlueprintRepository(t)
	setup(promptRepo, artifactRepo, memoryRepo, blueprintRepo)

	service := createTestEmbeddingService(embRepo, promptRepo, artifactRepo, memoryRepo, blueprintRepo)
	_, err := service.resolveEntityTeam(
		context.Background(), "u", entityType, "id",
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), wantPrefix)
	assert.Contains(t, err.Error(), "repo: not found")
}

// TestEmbeddingService_resolveEntityTeam_PromptWrapsError asserts the prompt
// ValidatorFunc preserves the "prompt not found:" wrap that callers grep on.
func TestEmbeddingService_resolveEntityTeam_PromptWrapsError(t *testing.T) {
	runBuiltinValidatorErrorWrapTest(t, "prompt", "prompt not found:",
		func(p *mocks.MockPromptRepository, _ *mocks.MockArtifactRepository,
			_ *mocks.MockMemoryRepository, _ *mocks.MockBlueprintRepository) {
			p.EXPECT().GetByIDCrossTeam(mock.Anything, "u", "id").
				Return(nil, fmt.Errorf("repo: not found")).Once()
		},
	)
}

// TestEmbeddingService_resolveEntityTeam_ArtifactWrapsError asserts the
// artifact ValidatorFunc preserves the "artifact not found:" wrap.
func TestEmbeddingService_resolveEntityTeam_ArtifactWrapsError(t *testing.T) {
	runBuiltinValidatorErrorWrapTest(t, "artifact", "artifact not found:",
		func(_ *mocks.MockPromptRepository, a *mocks.MockArtifactRepository,
			_ *mocks.MockMemoryRepository, _ *mocks.MockBlueprintRepository) {
			a.EXPECT().GetByIDCrossTeam(mock.Anything, "u", "id").
				Return(nil, fmt.Errorf("repo: not found")).Once()
		},
	)
}

// TestEmbeddingService_resolveEntityTeam_MemoryWrapsError asserts the
// memory ValidatorFunc preserves the "memory not found:" wrap.
func TestEmbeddingService_resolveEntityTeam_MemoryWrapsError(t *testing.T) {
	runBuiltinValidatorErrorWrapTest(t, "memory", "memory not found:",
		func(_ *mocks.MockPromptRepository, _ *mocks.MockArtifactRepository,
			m *mocks.MockMemoryRepository, _ *mocks.MockBlueprintRepository) {
			m.EXPECT().GetByIDCrossTeam(mock.Anything, "u", "id").
				Return(nil, fmt.Errorf("repo: not found")).Once()
		},
	)
}

// TestEmbeddingService_resolveEntityTeam_BlueprintWrapsError asserts the
// blueprint ValidatorFunc preserves the "blueprint not found:" wrap.
func TestEmbeddingService_resolveEntityTeam_BlueprintWrapsError(t *testing.T) {
	runBuiltinValidatorErrorWrapTest(t, "blueprint", "blueprint not found:",
		func(_ *mocks.MockPromptRepository, _ *mocks.MockArtifactRepository,
			_ *mocks.MockMemoryRepository, b *mocks.MockBlueprintRepository) {
			b.EXPECT().GetByIDCrossTeam(mock.Anything, "u", "id").
				Return(nil, fmt.Errorf("repo: not found")).Once()
		},
	)
}

// TestEmbeddingService_resolveEntityTeam_NotFoundMarkedPermanent verifies that
// each built-in entity type's validator marks its repository not-found sentinel
// with ErrEntityNotFound (errors.Is) while preserving the "<entity> not found:"
// wrap — the classification Pub/Sub handlers use to ack-and-drop events for
// deleted entities instead of retrying them forever.
func TestEmbeddingService_resolveEntityTeam_NotFoundMarkedPermanent(t *testing.T) {
	tests := []struct {
		entityType string
		setup      func(*mocks.MockPromptRepository, *mocks.MockArtifactRepository,
			*mocks.MockMemoryRepository, *mocks.MockBlueprintRepository)
	}{
		{"prompt", func(p *mocks.MockPromptRepository, _ *mocks.MockArtifactRepository,
			_ *mocks.MockMemoryRepository, _ *mocks.MockBlueprintRepository) {
			p.EXPECT().GetByIDCrossTeam(mock.Anything, "u", "id").
				Return(nil, repositories.ErrPromptNotFound).Once()
		}},
		{"artifact", func(_ *mocks.MockPromptRepository, a *mocks.MockArtifactRepository,
			_ *mocks.MockMemoryRepository, _ *mocks.MockBlueprintRepository) {
			a.EXPECT().GetByIDCrossTeam(mock.Anything, "u", "id").
				Return(nil, repositories.ErrArtifactNotFound).Once()
		}},
		{"memory", func(_ *mocks.MockPromptRepository, _ *mocks.MockArtifactRepository,
			m *mocks.MockMemoryRepository, _ *mocks.MockBlueprintRepository) {
			m.EXPECT().GetByIDCrossTeam(mock.Anything, "u", "id").
				Return(nil, repositories.ErrMemoryNotFound).Once()
		}},
		{"blueprint", func(_ *mocks.MockPromptRepository, _ *mocks.MockArtifactRepository,
			_ *mocks.MockMemoryRepository, b *mocks.MockBlueprintRepository) {
			b.EXPECT().GetByIDCrossTeam(mock.Anything, "u", "id").
				Return(nil, repositories.ErrBlueprintNotFound).Once()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.entityType, func(t *testing.T) {
			promptRepo := mocks.NewMockPromptRepository(t)
			artifactRepo := mocks.NewMockArtifactRepository(t)
			memoryRepo := mocks.NewMockMemoryRepository(t)
			blueprintRepo := mocks.NewMockBlueprintRepository(t)
			tt.setup(promptRepo, artifactRepo, memoryRepo, blueprintRepo)
			service := createTestEmbeddingService(mocks.NewMockEmbeddingRepository(t),
				promptRepo, artifactRepo, memoryRepo, blueprintRepo)

			_, err := service.resolveEntityTeam(context.Background(), "u", tt.entityType, "id")

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrEntityNotFound)
			assert.Contains(t, err.Error(), tt.entityType+" not found:")
		})
	}
}

// TestEmbeddingService_resolveEntityTeam_FeedNotFoundMarkedPermanent mirrors the
// permanent-classification test for the poster-scoped feed entity types.
func TestEmbeddingService_resolveEntityTeam_FeedNotFoundMarkedPermanent(t *testing.T) {
	t.Run("feed_item", func(t *testing.T) {
		feedItemRepo := mocks.NewMockFeedItemRepository(t)
		feedItemRepo.EXPECT().GetByIDForPoster(mock.Anything, "u", "id").
			Return(nil, repositories.ErrFeedItemNotFound).Once()
		svc := createTestEmbeddingServiceWithFeed(mocks.NewMockEmbeddingRepository(t),
			feedItemRepo, mocks.NewMockFeedItemReplyRepository(t))

		_, err := svc.resolveEntityTeam(context.Background(), "u", "feed_item", "id")

		assert.ErrorIs(t, err, ErrEntityNotFound)
	})
	t.Run("feed_item_reply", func(t *testing.T) {
		feedItemReplyRepo := mocks.NewMockFeedItemReplyRepository(t)
		feedItemReplyRepo.EXPECT().GetReplyForPoster(mock.Anything, "u", "id").
			Return(nil, repositories.ErrFeedItemReplyNotFound).Once()
		svc := createTestEmbeddingServiceWithFeed(mocks.NewMockEmbeddingRepository(t),
			mocks.NewMockFeedItemRepository(t), feedItemReplyRepo)

		_, err := svc.resolveEntityTeam(context.Background(), "u", "feed_item_reply", "id")

		assert.ErrorIs(t, err, ErrEntityNotFound)
	})
}

// TestEmbeddingService_resolveEntityTeam_TransientErrorNotPermanent guards the
// inverse classification: a non-sentinel lookup failure (e.g. the database is
// unreachable) must NOT be marked ErrEntityNotFound, so Pub/Sub keeps retrying.
func TestEmbeddingService_resolveEntityTeam_TransientErrorNotPermanent(t *testing.T) {
	promptRepo := mocks.NewMockPromptRepository(t)
	promptRepo.EXPECT().GetByIDCrossTeam(mock.Anything, "u", "id").
		Return(nil, errors.New("connection refused")).Once()
	service := createTestEmbeddingService(mocks.NewMockEmbeddingRepository(t), promptRepo,
		mocks.NewMockArtifactRepository(t), mocks.NewMockMemoryRepository(t),
		mocks.NewMockBlueprintRepository(t))

	_, err := service.resolveEntityTeam(context.Background(), "u", "prompt", "id")

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrEntityNotFound)
}

// TestEmbeddingService_SaveEmbeddingChunks_DeletedEntityMarkedPermanent covers
// the full save path for an entity deleted before its embedding event arrived:
// the error is classified permanent via ErrEntityNotFound and no embedding rows
// are deleted or written (the embedding-repo mock has no expectations).
func TestEmbeddingService_SaveEmbeddingChunks_DeletedEntityMarkedPermanent(t *testing.T) {
	promptRepo := mocks.NewMockPromptRepository(t)
	promptRepo.EXPECT().GetByIDCrossTeam(mock.Anything, "user-123", "prompt-deleted").
		Return(nil, repositories.ErrPromptNotFound).Once()
	service := createTestEmbeddingService(mocks.NewMockEmbeddingRepository(t), promptRepo,
		mocks.NewMockArtifactRepository(t), mocks.NewMockMemoryRepository(t),
		mocks.NewMockBlueprintRepository(t))

	err := saveOneChunk(service, "user-123", "prompt", "prompt-deleted")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEntityNotFound)
	assert.Contains(t, err.Error(), "entity validation failed")
}

// TestEmbeddingService_resolveEntityTeam_UnregisteredType_ViaSaveEmbedding
// asserts that dispatching with an entity type that is not present in the
// per-instance registry returns the documented "unsupported entity type" error
// without any switch-statement modification. This guards the contract that
// adding a new entity type requires only a registry edit (and a constructor
// wiring change). It exercises the full SaveEmbedding code path so the outer
// "entity validation failed" wrap is also covered end-to-end.
func TestEmbeddingService_resolveEntityTeam_UnregisteredType_ViaSaveEmbedding(t *testing.T) {
	embRepo := mocks.NewMockEmbeddingRepository(t)
	promptRepo := mocks.NewMockPromptRepository(t)
	artifactRepo := mocks.NewMockArtifactRepository(t)
	memoryRepo := mocks.NewMockMemoryRepository(t)
	blueprintRepo := mocks.NewMockBlueprintRepository(t)

	// No mock expectations: validation must short-circuit before any repo call.

	service := createTestEmbeddingService(embRepo, promptRepo, artifactRepo, memoryRepo, blueprintRepo)

	// "unknown" is intentionally not registered; the dispatch must short-circuit.
	err := service.SaveEmbedding(
		"user-123", "unknown", "entity-1", "test-model",
		[][]float32{createTestVector384()},
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported entity type: unknown")
}

// TestEmbeddingService_resolveEntityTeam_RegistryDispatch asserts that the
// dispatch in resolveEntityTeam actually goes through the entityRegistry's
// ValidatorFunc — by registering a fake entity type whose ValidatorFunc returns
// a sentinel error and asserting the wrapped error reaches the caller.
//
// This proves the refactor's central claim: validation routing is data-driven,
// not switch-driven. A future blueprint registration cannot regress this.
func TestEmbeddingService_resolveEntityTeam_RegistryDispatch(t *testing.T) {
	embRepo := mocks.NewMockEmbeddingRepository(t)
	promptRepo := mocks.NewMockPromptRepository(t)
	artifactRepo := mocks.NewMockArtifactRepository(t)
	memoryRepo := mocks.NewMockMemoryRepository(t)
	blueprintRepo := mocks.NewMockBlueprintRepository(t)

	service := createTestEmbeddingService(embRepo, promptRepo, artifactRepo, memoryRepo, blueprintRepo)

	sentinel := errors.New("sentinel-validator-error")
	var (
		gotUserID   string
		gotEntityID string
	)
	// Inject a fake entity type into the per-instance registry. This is the
	// minimum-viable test seam: the test lives in the same package, so it can
	// mutate svc.entityRegistry directly to register a stub validator.
	service.entityRegistry["fake-entity"] = EmbeddingEntityConfig{
		EntityIDField: "fakeID",
		ValidatorFunc: func(_ context.Context, userID, entityID string) (string, error) {
			gotUserID = userID
			gotEntityID = entityID
			return "", sentinel
		},
	}

	_, err := service.resolveEntityTeam(
		context.Background(), "user-xyz", "fake-entity", "fake-1",
	)

	require := assert.New(t)
	require.Error(err)
	require.True(errors.Is(err, sentinel),
		"resolveEntityTeam must propagate the registry validator's error verbatim")
	require.Equal("user-xyz", gotUserID)
	require.Equal("fake-1", gotEntityID)
}

// TestEmbeddingService_resolveEntityTeam_NilValidatorSkips asserts the
// "nil ValidatorFunc means skip validation" contract. This preserves the
// previous behaviour (when a per-type repo was nil, validation was skipped) and
// gives test authors a way to register an entity type without forcing a repo.
func TestEmbeddingService_resolveEntityTeam_NilValidatorSkips(t *testing.T) {
	embRepo := mocks.NewMockEmbeddingRepository(t)
	promptRepo := mocks.NewMockPromptRepository(t)
	artifactRepo := mocks.NewMockArtifactRepository(t)
	memoryRepo := mocks.NewMockMemoryRepository(t)
	blueprintRepo := mocks.NewMockBlueprintRepository(t)

	service := createTestEmbeddingService(embRepo, promptRepo, artifactRepo, memoryRepo, blueprintRepo)

	service.entityRegistry["validator-less"] = EmbeddingEntityConfig{
		EntityIDField: "vlID",
		ValidatorFunc: nil,
	}

	teamID, err := service.resolveEntityTeam(
		context.Background(), "user-xyz", "validator-less", "any-id",
	)

	assert.NoError(t, err)
	assert.Empty(t, teamID)
}

// TestEmbeddingService_resolveEntityTeam_NilRepoSkipsForBuiltinTypes asserts
// the existing behaviour for the four built-in entity types: when their repo
// dependency is nil (the test injection pattern used by GetEmbeddingsByEntity
// and FindSimilar tests in this file), validation is skipped silently. This
// behaviour was previously implemented inside the switch and is now inside
// each ValidatorFunc — the test guards that we did not regress it.
func TestEmbeddingService_resolveEntityTeam_NilRepoSkipsForBuiltinTypes(t *testing.T) {
	embRepo := mocks.NewMockEmbeddingRepository(t)
	// All typed repos are intentionally nil.
	service := NewEmbeddingService(
		embRepo, nil, nil, nil, nil, nil, nil, testEmbeddingDimensions,
		func() *slog.Logger { l, _ := logtest.New(); return l }(),
	)

	for _, et := range []string{"prompt", "artifact", "memory", "blueprint", "feed_item", "feed_item_reply"} {
		teamID, err := service.resolveEntityTeam(context.Background(), "user-x", et, "id-x")
		assert.NoErrorf(t, err, "nil repo for entity type %q must skip validation", et)
		assert.Emptyf(t, teamID, "nil repo for entity type %q must yield empty teamID", et)
	}
}

// TestGetEmbeddingEntityConfig asserts the package-level routing registry still
// exposes EntityIDField for the four built-in entity types — guards the
// embedding_handler_adapter and server.embedding_handlers callers that were
// added in PR #1318 and read cfg.EntityIDField.
func TestGetEmbeddingEntityConfig(t *testing.T) {
	tests := []struct {
		entityType     string
		wantField      string
		wantRegistered bool
	}{
		{"prompt", "promptID", true},
		{"artifact", "artifactID", true},
		{"memory", "memoryID", true},
		{"blueprint", "blueprintID", true},
		{"feed_item", "feedItemID", true},
		{"feed_item_reply", "feedItemReplyID", true},
		{"unknown", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.entityType, func(t *testing.T) {
			cfg, ok := GetEmbeddingEntityConfig(tt.entityType)
			assert.Equal(t, tt.wantRegistered, ok)
			if tt.wantRegistered {
				assert.Equal(t, tt.wantField, cfg.EntityIDField)
				// The package-level registry intentionally leaves ValidatorFunc unset;
				// existence validation is per-instance, not package-level.
				assert.Nil(t, cfg.ValidatorFunc)
			}
		})
	}
}

// TestEntityRegistries_AreConsistent guards the dual-registry split. The
// package-level embeddingEntityRegistry (consumed by handler-layer payload
// routing in embedding_handler_adapter.go and server/embedding_handlers.go)
// and the per-instance EmbeddingService.entityRegistry (consumed by
// resolveEntityTeam) must agree on the set of registered entity types AND
// on the EntityIDField value for each type. If a future contributor adds an
// entity to one but not the other, or uses a different EntityIDField in each,
// the handler layer and the service layer will silently diverge — this test
// fails CI before that ships.
func TestEntityRegistries_AreConsistent(t *testing.T) {
	embRepo := mocks.NewMockEmbeddingRepository(t)
	promptRepo := mocks.NewMockPromptRepository(t)
	artifactRepo := mocks.NewMockArtifactRepository(t)
	memoryRepo := mocks.NewMockMemoryRepository(t)
	blueprintRepo := mocks.NewMockBlueprintRepository(t)
	service := createTestEmbeddingService(embRepo, promptRepo, artifactRepo, memoryRepo, blueprintRepo)

	// Same set of keys.
	assert.Equal(t, len(embeddingEntityRegistry), len(service.entityRegistry),
		"package-level and per-instance registries must have the same number of entries")

	for entityType, pkgCfg := range embeddingEntityRegistry {
		instCfg, ok := service.entityRegistry[entityType]
		assert.Truef(t, ok,
			"entity %q present in package-level registry must also be in per-instance registry",
			entityType)
		assert.Equalf(t, pkgCfg.EntityIDField, instCfg.EntityIDField,
			"entity %q EntityIDField must match between registries (pkg=%q, inst=%q)",
			entityType, pkgCfg.EntityIDField, instCfg.EntityIDField)
	}
	for entityType := range service.entityRegistry {
		_, ok := embeddingEntityRegistry[entityType]
		assert.Truef(t, ok,
			"entity %q present in per-instance registry must also be in package-level registry "+
				"(handler-layer routing depends on the package registry)",
			entityType)
	}
}

// saveOneChunk is a convenience wrapper that saves a single-chunk embedding for the
// given keying triple, used by the feed validation tests below.
func saveOneChunk(svc *EmbeddingService, userID, entityType, entityID string) error {
	return svc.SaveEmbeddingChunks(userID, entityType, entityID, "test-model",
		[]EmbeddingChunk{{Embedding: createTestVector384(), Content: "body"}})
}

// TestEmbeddingService_FeedItem_PosterScopedValidation asserts feed_item existence
// validation is poster-scoped: it dispatches to GetByIDForPoster and the save succeeds
// for the real poster but fails for a forged/foreign id. Guards the keying invariant (#1361).
func TestEmbeddingService_FeedItem_PosterScopedValidation(t *testing.T) {
	const posterID = "poster-1"

	t.Run("passes for poster", func(t *testing.T) {
		embRepo := mocks.NewMockEmbeddingRepository(t)
		feedItemRepo := mocks.NewMockFeedItemRepository(t)
		svc := createTestEmbeddingServiceWithFeed(embRepo, feedItemRepo, mocks.NewMockFeedItemReplyRepository(t))

		feedItemRepo.EXPECT().GetByIDForPoster(mock.Anything, posterID, "item-1").
			Return(&models.FeedItem{ID: "item-1", TeamID: "team-feed", PostedByUserID: posterID}, nil).Once()
		embRepo.EXPECT().DeleteByEntity(mock.Anything, "feed_item", "item-1").Return(nil).Once()
		embRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(emb *models.Embedding) bool {
			return emb.UserID == posterID && emb.TeamID == "team-feed" &&
				emb.EntityType == "feed_item" && emb.EntityID == "item-1"
		})).Return(nil).Once()

		assert.NoError(t, saveOneChunk(svc, posterID, "feed_item", "item-1"))
	})

	t.Run("fails for forged id", func(t *testing.T) {
		embRepo := mocks.NewMockEmbeddingRepository(t)
		feedItemRepo := mocks.NewMockFeedItemRepository(t)
		svc := createTestEmbeddingServiceWithFeed(embRepo, feedItemRepo, mocks.NewMockFeedItemReplyRepository(t))

		feedItemRepo.EXPECT().GetByIDForPoster(mock.Anything, "forged-user", "item-1").
			Return(nil, errors.New("feed item not found")).Once()

		err := saveOneChunk(svc, "forged-user", "feed_item", "item-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "feed_item not found")
	})
}

// TestEmbeddingService_FeedItemReply_PosterScopedValidation mirrors the feed_item test
// for replies, dispatching to GetReplyForPoster.
func TestEmbeddingService_FeedItemReply_PosterScopedValidation(t *testing.T) {
	const posterID = "poster-1"

	t.Run("passes for poster", func(t *testing.T) {
		embRepo := mocks.NewMockEmbeddingRepository(t)
		feedItemReplyRepo := mocks.NewMockFeedItemReplyRepository(t)
		svc := createTestEmbeddingServiceWithFeed(embRepo, mocks.NewMockFeedItemRepository(t), feedItemReplyRepo)

		feedItemReplyRepo.EXPECT().GetReplyForPoster(mock.Anything, posterID, "reply-1").
			Return(&models.FeedItemReply{ID: "reply-1", TeamID: "team-feed", PostedByUserID: posterID}, nil).Once()
		embRepo.EXPECT().DeleteByEntity(mock.Anything, "feed_item_reply", "reply-1").Return(nil).Once()
		embRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(emb *models.Embedding) bool {
			return emb.UserID == posterID && emb.TeamID == "team-feed" &&
				emb.EntityType == "feed_item_reply" && emb.EntityID == "reply-1"
		})).Return(nil).Once()

		assert.NoError(t, saveOneChunk(svc, posterID, "feed_item_reply", "reply-1"))
	})

	t.Run("fails for forged id", func(t *testing.T) {
		embRepo := mocks.NewMockEmbeddingRepository(t)
		feedItemReplyRepo := mocks.NewMockFeedItemReplyRepository(t)
		svc := createTestEmbeddingServiceWithFeed(embRepo, mocks.NewMockFeedItemRepository(t), feedItemReplyRepo)

		feedItemReplyRepo.EXPECT().GetReplyForPoster(mock.Anything, "forged-user", "reply-1").
			Return(nil, errors.New("feed item reply not found")).Once()

		err := saveOneChunk(svc, "forged-user", "feed_item_reply", "reply-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "feed_item_reply not found")
	})
}
