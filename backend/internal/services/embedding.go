package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/pgvector/pgvector-go"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrEntityNotFound marks an embedding validation failure as permanent: the
// target entity no longer exists, so retrying can never succeed. Pub/Sub push
// handlers detect it via errors.Is to ack-and-drop the event instead of
// returning 5xx, which would make Pub/Sub redeliver the message forever.
var ErrEntityNotFound = errors.New("entity not found")

// entityNotFoundSentinels are the repository not-found sentinels the cross-team
// lookups in the entity registry can surface, one per registered entity type.
var entityNotFoundSentinels = []error{
	repositories.ErrPromptNotFound,
	repositories.ErrArtifactNotFound,
	repositories.ErrMemoryNotFound,
	repositories.ErrBlueprintNotFound,
	repositories.ErrFeedItemNotFound,
	repositories.ErrFeedItemReplyNotFound,
}

// isEntityNotFound reports whether err is a repository not-found sentinel,
// distinguishing permanent "entity deleted" failures from transient ones
// (DB unavailable, etc.) that callers should keep retrying.
func isEntityNotFound(err error) bool {
	for _, sentinel := range entityNotFoundSentinels {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}

// EmbeddingChunk represents a single embedding chunk with its content
type EmbeddingChunk struct {
	Embedding []float32
	Content   string
}

// EntityValidatorFunc validates that the entity identified by entityID exists for
// userID AND resolves the entity's team id (denormalized onto the embedding rows
// so team-scoped search can filter directly on embeddings.team_id). Returning a
// non-nil error means the embedding payload should be rejected. A nil
// EntityValidatorFunc on a registered entity is treated as "skip validation",
// returning an empty teamID (the embeddings.team_id column is nullable).
type EntityValidatorFunc func(ctx context.Context, userID, entityID string) (teamID string, err error)

// EmbeddingEntityConfig holds the validation configuration for a registered
// entity type in the embedding pipeline. It is populated per-instance in
// NewEmbeddingService so each ValidatorFunc can close over the service's
// repositories for existence validation and team resolution.
type EmbeddingEntityConfig struct {
	// ValidatorFunc checks that the entity identified by entityID exists for userID
	// and resolves its team id. A nil ValidatorFunc means validation is skipped for
	// this entity type, returning an empty teamID.
	ValidatorFunc EntityValidatorFunc
}

// ParseEmbeddingChunks converts a raw embeddings array (as decoded from JSON)
// into typed EmbeddingChunk values. It returns an error if any entry is malformed.
func ParseEmbeddingChunks(embeddingsRaw []interface{}) ([]EmbeddingChunk, error) {
	chunks := make([]EmbeddingChunk, len(embeddingsRaw))

	for i, emb := range embeddingsRaw {
		embMap, ok := emb.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("embedding at index %d is not a map", i)
		}

		embArray, ok := embMap["embedding"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("embedding at index %d missing 'embedding' field", i)
		}

		embedding := make([]float32, len(embArray))
		for j, val := range embArray {
			floatVal, ok := val.(float64)
			if !ok {
				return nil, fmt.Errorf("embedding value at index %d,%d is not a number", i, j)
			}
			embedding[j] = float32(floatVal)
		}

		content, _ := embMap["content"].(string)
		chunks[i] = EmbeddingChunk{
			Embedding: embedding,
			Content:   content,
		}
	}

	return chunks, nil
}

// EmbeddingServiceInterface defines the interface for embedding service operations
type EmbeddingServiceInterface interface {
	SaveEmbedding(userID, entityType, entityID, modelID string, vectors [][]float32) error
	SaveEmbeddingChunks(userID, entityType, entityID, modelID string, chunks []EmbeddingChunk) error
	GetEmbeddingsByEntity(userID, entityType, entityID string) ([]models.Embedding, error)
	FindSimilar(userID, entityType string, vector []float32, limit int) ([]models.EmbeddingSimilarity, error)
	DeleteEmbeddingsByEntity(entityType, entityID string) error
	// ResolveEntityTeam validates the entity exists and returns its team id, so the
	// embedding worker can pick that team's provider before embedding.
	ResolveEntityTeam(ctx context.Context, userID, entityType, entityID string) (string, error)
}

// EmbeddingService implements the EmbeddingServiceInterface.
//
// The entityRegistry is populated in NewEmbeddingService and drives
// resolveEntityTeam. To support a new entity type, the required edits are:
//  1. Add a config entry (with its ValidatorFunc) to entityRegistry in
//     NewEmbeddingService — drives existence validation + team resolution.
//  2. Wire the new repository into the constructor signature.
//  3. Add the repository's Err*NotFound sentinel to entityNotFoundSentinels —
//     without it, embedding events for deleted entities of the new type are
//     classified as transient and the event bus retries them indefinitely.
//
// No switch statements are required: resolveEntityTeam dispatches by entity-type
// name through the registry.
type EmbeddingService struct {
	repo              repositories.EmbeddingRepository
	promptRepo        repositories.PromptRepository
	artifactRepo      repositories.ArtifactRepository
	memoryRepo        repositories.MemoryRepository
	blueprintRepo     repositories.BlueprintRepository
	feedItemRepo      repositories.FeedItemRepository
	feedItemReplyRepo repositories.FeedItemReplyRepository
	entityRegistry    map[string]EmbeddingEntityConfig
	// dimensions is the expected length of every stored embedding vector; it must
	// match the vector(N) column width set by the active migration.
	dimensions int
	logger     *slog.Logger
}

// Ensure EmbeddingService implements EmbeddingServiceInterface
var _ EmbeddingServiceInterface = (*EmbeddingService)(nil)

// crossTeamLookup is the minimal cross-team accessor every embeddable entity
// repository implements. It returns the entity's team id (denormalized onto the
// embedding rows) so the validator doubles as a team resolver. Using a function
// value (rather than the full repo interface) lets buildEntityValidator stay
// generic across entity types.
type crossTeamLookup func(ctx context.Context, userID, entityID string) (teamID string, err error)

// buildEntityValidator returns a ValidatorFunc that invokes lookup when the
// underlying repository is wired in (lookup != nil), returning the entity's team
// id, and wraps any error with a stable, entity-typed prefix. Repository
// not-found sentinels are additionally marked with ErrEntityNotFound so callers
// can classify the failure as permanent via errors.Is. When lookup is nil
// (test stubs / nil repo) the validator is a no-op that returns an empty teamID
// so handlers can still process events without a backing store.
//
// The nil-vs-set decision is captured at construction time by each per-entity
// *CrossTeamLookup helper and baked into the closure stored in the registry —
// repos are not reassigned post-construction in this codebase, so late-bound
// semantics are not required.
func buildEntityValidator(entityName string, lookup crossTeamLookup) EntityValidatorFunc {
	return func(ctx context.Context, userID, entityID string) (string, error) {
		if lookup == nil {
			return "", nil
		}
		teamID, err := lookup(ctx, userID, entityID)
		if err != nil {
			if isEntityNotFound(err) {
				return "", fmt.Errorf("%s not found: %w: %w", entityName, ErrEntityNotFound, err)
			}
			return "", fmt.Errorf("%s not found: %w", entityName, err)
		}
		return teamID, nil
	}
}

// EmbeddingServiceDeps groups the dependencies injected into EmbeddingService.
type EmbeddingServiceDeps struct {
	Repo              repositories.EmbeddingRepository
	PromptRepo        repositories.PromptRepository
	ArtifactRepo      repositories.ArtifactRepository
	MemoryRepo        repositories.MemoryRepository
	BlueprintRepo     repositories.BlueprintRepository
	FeedItemRepo      repositories.FeedItemRepository
	FeedItemReplyRepo repositories.FeedItemReplyRepository
	Dimensions        int
	Logger            *slog.Logger
}

// NewEmbeddingService creates a new EmbeddingService and builds the per-instance
// entity registry. Each ValidatorFunc closes over the service's repositories so
// resolveEntityTeam can dispatch by entity-type name without a switch.
func NewEmbeddingService(deps EmbeddingServiceDeps) *EmbeddingService {
	svc := &EmbeddingService{
		repo:              deps.Repo,
		promptRepo:        deps.PromptRepo,
		artifactRepo:      deps.ArtifactRepo,
		memoryRepo:        deps.MemoryRepo,
		blueprintRepo:     deps.BlueprintRepo,
		feedItemRepo:      deps.FeedItemRepo,
		feedItemReplyRepo: deps.FeedItemReplyRepo,
		dimensions:        deps.Dimensions,
		logger:            deps.Logger,
	}
	svc.entityRegistry = map[string]EmbeddingEntityConfig{
		"prompt":          {ValidatorFunc: buildEntityValidator("prompt", promptCrossTeamLookup(svc))},
		"artifact":        {ValidatorFunc: buildEntityValidator("artifact", artifactCrossTeamLookup(svc))},
		"memory":          {ValidatorFunc: buildEntityValidator("memory", memoryCrossTeamLookup(svc))},
		"blueprint":       {ValidatorFunc: buildEntityValidator("blueprint", blueprintCrossTeamLookup(svc))},
		"feed_item":       {ValidatorFunc: buildEntityValidator("feed_item", feedItemLookup(svc))},
		"feed_item_reply": {ValidatorFunc: buildEntityValidator("feed_item_reply", feedItemReplyLookup(svc))},
	}
	return svc
}

// promptCrossTeamLookup returns the cross-team prompt lookup or nil when the
// repository is not wired in. Returning nil signals "no validation possible"
// to buildEntityValidator. The lookup yields the prompt's team id so it can be
// denormalized onto the embedding rows.
func promptCrossTeamLookup(svc *EmbeddingService) crossTeamLookup {
	if svc.promptRepo == nil {
		return nil
	}
	return func(ctx context.Context, userID, entityID string) (string, error) {
		ent, err := svc.promptRepo.GetByIDCrossTeam(ctx, userID, entityID)
		if err != nil {
			return "", err
		}
		return ent.TeamID, nil
	}
}

// artifactCrossTeamLookup mirrors promptCrossTeamLookup for artifacts.
func artifactCrossTeamLookup(svc *EmbeddingService) crossTeamLookup {
	if svc.artifactRepo == nil {
		return nil
	}
	return func(ctx context.Context, userID, entityID string) (string, error) {
		ent, err := svc.artifactRepo.GetByIDCrossTeam(ctx, userID, entityID)
		if err != nil {
			return "", err
		}
		return ent.TeamID, nil
	}
}

// memoryCrossTeamLookup mirrors promptCrossTeamLookup for memories.
func memoryCrossTeamLookup(svc *EmbeddingService) crossTeamLookup {
	if svc.memoryRepo == nil {
		return nil
	}
	return func(ctx context.Context, userID, entityID string) (string, error) {
		ent, err := svc.memoryRepo.GetByIDCrossTeam(ctx, userID, entityID)
		if err != nil {
			return "", err
		}
		return ent.TeamID, nil
	}
}

// blueprintCrossTeamLookup mirrors promptCrossTeamLookup for blueprints.
func blueprintCrossTeamLookup(svc *EmbeddingService) crossTeamLookup {
	if svc.blueprintRepo == nil {
		return nil
	}
	return func(ctx context.Context, userID, entityID string) (string, error) {
		ent, err := svc.blueprintRepo.GetByIDCrossTeam(ctx, userID, entityID)
		if err != nil {
			return "", err
		}
		return ent.TeamID, nil
	}
}

// feedItemLookup mirrors blueprintCrossTeamLookup for feed items, but validates
// against the posting user (posted_by_user_id) rather than cross-team ownership —
// feed embeddings are keyed by the poster, so validation must be poster-scoped.
func feedItemLookup(svc *EmbeddingService) crossTeamLookup {
	if svc.feedItemRepo == nil {
		return nil
	}
	return func(ctx context.Context, userID, entityID string) (string, error) {
		fi, err := svc.feedItemRepo.GetByIDForPoster(ctx, userID, entityID)
		if err != nil {
			return "", err
		}
		return fi.TeamID, nil
	}
}

// feedItemReplyLookup mirrors feedItemLookup for feed item replies.
func feedItemReplyLookup(svc *EmbeddingService) crossTeamLookup {
	if svc.feedItemReplyRepo == nil {
		return nil
	}
	return func(ctx context.Context, userID, entityID string) (string, error) {
		r, err := svc.feedItemReplyRepo.GetReplyForPoster(ctx, userID, entityID)
		if err != nil {
			return "", err
		}
		return r.TeamID, nil
	}
}

// SaveEmbedding saves embeddings for an entity (legacy method for backward compatibility)
func (s *EmbeddingService) SaveEmbedding(userID, entityType, entityID, modelID string, vectors [][]float32) error {
	ctx := context.Background()

	// Validate entity exists and resolve its team id for denormalization.
	teamID, err := s.resolveEntityTeam(ctx, userID, entityType, entityID)
	if err != nil {
		return fmt.Errorf("entity validation failed: %w", err)
	}

	// Save each embedding vector
	for _, vector := range vectors {
		if len(vector) != s.dimensions {
			return fmt.Errorf("invalid vector dimension: expected %d, got %d", s.dimensions, len(vector))
		}

		now := time.Now()
		embedding := &models.Embedding{
			UserID:           userID,
			TeamID:           teamID,
			EntityType:       entityType,
			EntityID:         entityID,
			VectorEmbeddings: pgvector.NewVector(vector),
			Content:          "", // Empty content for legacy calls
			ModelID:          modelID,
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		if err := s.repo.Create(ctx, embedding); err != nil {
			return fmt.Errorf("failed to save embedding: %w", err)
		}

		s.logger.With(
			"user_id", userID,
			"entity_type", entityType,
			"entity_id", entityID,
			"model_id", modelID,
			"embedding_id", embedding.ID,
		).Info("Embedding saved successfully")
	}

	return nil
}

// SaveEmbeddingChunks saves embedding chunks with their associated content using
// delete-then-insert semantics: any embeddings already stored for the
// (userID, entityType, entityID) triple are deleted before the new chunks are inserted.
// This prevents stale vectors from accumulating when an entity is re-embedded after an update.
//
// Order: validate entity → validate every chunk → delete existing → insert all chunks.
// Chunk validation runs before the delete so a malformed payload never wipes good data
// (e.g. a poison event that would otherwise erase the entity's embedding corpus).
//
// Note: delete and inserts are NOT wrapped in a single DB transaction today, so a Create
// failure mid-batch can leave the entity with fewer chunks than supplied. Callers must retry.
// Successful repeated calls with the same input converge to the same state (idempotent).
func (s *EmbeddingService) SaveEmbeddingChunks(
	userID, entityType, entityID, modelID string, chunks []EmbeddingChunk,
) error {
	ctx := context.Background()

	// Validate entity exists before mutating any state, and resolve its team id so
	// it can be denormalized onto every chunk row for team-scoped search.
	teamID, err := s.resolveEntityTeam(ctx, userID, entityType, entityID)
	if err != nil {
		return fmt.Errorf("entity validation failed: %w", err)
	}

	// Validate every chunk BEFORE deleting existing rows. A poison message with
	// an invalid vector must not wipe an entity's existing embeddings.
	for i, chunk := range chunks {
		if len(chunk.Embedding) != s.dimensions {
			return fmt.Errorf(
				"invalid vector dimension at chunk %d: expected %d, got %d",
				i, s.dimensions, len(chunk.Embedding),
			)
		}
	}

	// Delete existing embeddings for this entity to keep the operation idempotent.
	// Without this, repeated calls (e.g. on entity update) append duplicate chunks
	// and slowly poison similarity search with stale vectors.
	if err := s.repo.DeleteByEntity(ctx, entityType, entityID); err != nil {
		return fmt.Errorf("failed to delete existing embeddings before save: %w", err)
	}

	// Insert each pre-validated chunk.
	for _, chunk := range chunks {
		now := time.Now()
		embedding := &models.Embedding{
			UserID:           userID,
			TeamID:           teamID,
			EntityType:       entityType,
			EntityID:         entityID,
			VectorEmbeddings: pgvector.NewVector(chunk.Embedding),
			Content:          chunk.Content,
			ModelID:          modelID,
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		if err := s.repo.Create(ctx, embedding); err != nil {
			return fmt.Errorf("failed to save embedding: %w", err)
		}
	}

	s.logger.With(
		"service", "embedding",
		"user_id", userID,
		"entity_type", entityType,
		"entity_id", entityID,
		"model_id", modelID,
		"chunk_count", len(chunks),
	).Info("Embedding chunks saved successfully (delete-then-insert)")

	return nil
}

// GetEmbeddingsByEntity retrieves all embeddings for a specific entity
func (s *EmbeddingService) GetEmbeddingsByEntity(
	userID, entityType, entityID string,
) ([]models.Embedding, error) {
	ctx := context.Background()
	return s.repo.GetByEntity(ctx, userID, entityType, entityID)
}

// FindSimilar finds embeddings similar to the given vector
func (s *EmbeddingService) FindSimilar(
	userID, entityType string, vector []float32, limit int,
) ([]models.EmbeddingSimilarity, error) {
	ctx := context.Background()

	if len(vector) != s.dimensions {
		return nil, fmt.Errorf("invalid vector dimension: expected %d, got %d", s.dimensions, len(vector))
	}

	return s.repo.FindSimilar(ctx, userID, entityType, vector, limit)
}

// DeleteEmbeddingsByEntity deletes all embeddings for a specific entity. It is
// keyed solely on (entityType, entityID) so the delete works regardless of which
// team member triggers it (orphan fix).
func (s *EmbeddingService) DeleteEmbeddingsByEntity(entityType, entityID string) error {
	ctx := context.Background()

	err := s.repo.DeleteByEntity(ctx, entityType, entityID)
	if err != nil {
		s.logger.With(
			"entity_type", entityType,
			"entity_id", entityID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to delete embeddings for entity")
		return fmt.Errorf("failed to delete embeddings: %w", err)
	}

	s.logger.With(
		"entity_type", entityType,
		"entity_id", entityID,
	).Info("Successfully deleted embeddings for entity")

	return nil
}

// resolveEntityTeam validates that the entity exists and resolves its team id by
// dispatching through the per-instance entityRegistry. It has no team context of
// its own; the resolution searches across all of the user's teams via the
// *CrossTeam repository methods invoked inside each ValidatorFunc. The returned
// team id is denormalized onto the embedding rows so team-scoped search can
// filter directly on embeddings.team_id.
//
// This denormalization is correct only while a source entity's team_id is
// immutable (entity updates pin team_id and team transfers are rejected). If a
// resource ever becomes movable between teams, the embedding rows must be
// re-derived (re-embed) for the new team or search will mis-scope the stale rows.
//
// A nil ValidatorFunc on a registered entity is treated as "skip validation",
// returning an empty teamID (the embeddings.team_id column is nullable). This
// preserves the previous behaviour of allowing repos to be left out in tests.
// ResolveEntityTeam is the exported entry point for resolveEntityTeam, letting the
// embedding worker resolve an entity's team before picking that team's provider.
func (s *EmbeddingService) ResolveEntityTeam(
	ctx context.Context, userID, entityType, entityID string,
) (string, error) {
	return s.resolveEntityTeam(ctx, userID, entityType, entityID)
}

func (s *EmbeddingService) resolveEntityTeam(
	ctx context.Context, userID, entityType, entityID string,
) (string, error) {
	cfg, ok := s.entityRegistry[entityType]
	if !ok {
		return "", fmt.Errorf("unsupported entity type: %s", entityType)
	}
	if cfg.ValidatorFunc == nil {
		return "", nil
	}
	return cfg.ValidatorFunc(ctx, userID, entityID)
}
