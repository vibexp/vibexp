package services

import (
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/pkg/events"
)

// EmbeddingHandlerAdapter implements events.EmbeddingHandlers for both the
// HTTPSyncListener (local-dev) and any other path that needs to process
// embedding payloads from the AI service. Entity-type routing is driven by
// GetEmbeddingEntityConfig so adding a new entity requires only one additional
// registry entry.
type EmbeddingHandlerAdapter struct {
	embeddingService EmbeddingServiceInterface
	logger           *slog.Logger
}

// Ensure EmbeddingHandlerAdapter satisfies the events.EmbeddingHandlers interface.
var _ events.EmbeddingHandlers = (*EmbeddingHandlerAdapter)(nil)

// NewEmbeddingHandlerAdapter creates a new EmbeddingHandlerAdapter.
func NewEmbeddingHandlerAdapter(
	embeddingService EmbeddingServiceInterface,
	logger *slog.Logger,
) *EmbeddingHandlerAdapter {
	return &EmbeddingHandlerAdapter{
		embeddingService: embeddingService,
		logger:           logger,
	}
}

// HandleEmbedding processes an embedding payload for any registered entity type.
// It uses GetEmbeddingEntityConfig to locate the entity ID field in the payload,
// parses embedding chunks, and delegates to EmbeddingService.SaveEmbeddingChunks.
func (a *EmbeddingHandlerAdapter) HandleEmbedding(entityType string, payload map[string]interface{}) error {
	cfg, ok := GetEmbeddingEntityConfig(entityType)
	if !ok {
		return fmt.Errorf("unsupported entity type: %s", entityType)
	}

	userID, _ := payload["userID"].(string)
	model, _ := payload["model"].(string)
	entityID, _ := payload[cfg.EntityIDField].(string)

	if userID == "" || entityID == "" || model == "" {
		return fmt.Errorf("missing required fields: userID=%s, %s=%s, model=%s",
			userID, cfg.EntityIDField, entityID, model)
	}

	embeddingsRaw, ok := payload["embeddings"].([]interface{})
	if !ok {
		return fmt.Errorf("embeddings field is missing or not an array")
	}

	chunks, err := ParseEmbeddingChunks(embeddingsRaw)
	if err != nil {
		return fmt.Errorf("failed to parse embeddings: %w", err)
	}

	if len(chunks) == 0 {
		return fmt.Errorf("no embeddings provided")
	}

	a.logger.With(
		"service", "vibexp-api",
		"component", "embedding-handler-adapter",
		"entity_type", entityType,
		"entity_id", entityID,
		"user_id", userID,
		"model", model,
		"embeddings_count", len(chunks),
	).Info("Processing entity embeddings")

	err = a.embeddingService.SaveEmbeddingChunks(userID, entityType, entityID, model, chunks)
	if err != nil {
		a.logger.With("error", err).Error("Failed to save entity embeddings")
		return fmt.Errorf("failed to save embeddings: %w", err)
	}

	return nil
}
