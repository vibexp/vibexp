package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/pkg/events"
)

// embeddingInput is the normalized text and identifiers extracted from a domain
// event for embedding.
type embeddingInput struct {
	entityType string
	entityID   string
	userID     string
	text       string
}

// EmbeddingGenerationProcessor implements events.EmbeddingProcessor. On each entity
// created/updated event it resolves the active system-wide provider, chunks the
// entity text in Go, embeds the chunks, and persists them via EmbeddingService.
// It is the in-process replacement for the external ai-service embedding path.
//
// model and dimensions are the deployment-wide values (EMBEDDING_MODEL /
// EMBEDDING_DIMENSIONS). model is the model_id tag written on every chunk row and
// is also used by search to filter candidate rows, so document and query
// embeddings stay comparable.
type EmbeddingGenerationProcessor struct {
	resolver         ActiveEmbeddingProviderResolver
	chunker          Chunker
	embeddingService EmbeddingServiceInterface
	model            string
	dimensions       int
	logger           *logrus.Logger
}

// Ensure EmbeddingGenerationProcessor implements events.EmbeddingProcessor.
var _ events.EmbeddingProcessor = (*EmbeddingGenerationProcessor)(nil)

// NewEmbeddingGenerationProcessor creates an EmbeddingGenerationProcessor.
func NewEmbeddingGenerationProcessor(
	resolver ActiveEmbeddingProviderResolver,
	chunker Chunker,
	embeddingService EmbeddingServiceInterface,
	model string,
	dimensions int,
	logger *logrus.Logger,
) *EmbeddingGenerationProcessor {
	return &EmbeddingGenerationProcessor{
		resolver:         resolver,
		chunker:          chunker,
		embeddingService: embeddingService,
		model:            model,
		dimensions:       dimensions,
		logger:           logger,
	}
}

// ProcessEvent resolves the provider, chunks, embeds, and saves. It no-ops when
// the event is not embeddable, carries no text, or no provider is configured —
// the originating entity write must never be failed by embedding generation.
func (p *EmbeddingGenerationProcessor) ProcessEvent(ctx context.Context, event events.Event) error {
	input, ok := extractEmbeddingInput(event)
	if !ok {
		return nil // event type is not embeddable
	}
	if input.entityID == "" || input.userID == "" || strings.TrimSpace(input.text) == "" {
		return nil // nothing to embed
	}

	provider, err := p.resolver.ResolveActiveProvider(ctx, p.model, p.dimensions)
	if err != nil {
		return fmt.Errorf("failed to resolve embedding provider: %w", err)
	}
	if provider == nil {
		p.logger.WithFields(logrus.Fields{
			"service":     "embedding",
			"component":   "embedding-processor",
			"entity_type": input.entityType,
			"entity_id":   input.entityID,
		}).Debug("No active embedding provider configured; skipping embedding generation")
		return nil
	}

	chunkTexts := p.chunker.Chunk(input.text)
	if len(chunkTexts) == 0 {
		return nil
	}

	vectors, err := provider.GenerateEmbeddings(ctx, chunkTexts)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings for %s %s: %w", input.entityType, input.entityID, err)
	}
	if len(vectors) != len(chunkTexts) {
		return fmt.Errorf(
			"provider returned %d vectors for %d chunks (%s %s)",
			len(vectors), len(chunkTexts), input.entityType, input.entityID,
		)
	}

	chunks := make([]EmbeddingChunk, len(chunkTexts))
	for i := range chunkTexts {
		chunks[i] = EmbeddingChunk{Content: chunkTexts[i], Embedding: vectors[i]}
	}

	if err := p.embeddingService.SaveEmbeddingChunks(
		input.userID, input.entityType, input.entityID, p.model, chunks,
	); err != nil {
		return fmt.Errorf("failed to save embeddings for %s %s: %w", input.entityType, input.entityID, err)
	}

	p.logger.WithFields(logrus.Fields{
		"service":     "embedding",
		"component":   "embedding-processor",
		"entity_type": input.entityType,
		"entity_id":   input.entityID,
		"model":       p.model,
		"chunk_count": len(chunks),
	}).Info("Embeddings generated and saved")

	return nil
}

// extractEmbeddingInput maps a domain event to the entity text + identifiers to
// embed. The second return is false for event types that are not embeddable. The
// entity-type strings must match the embeddings registry keys in embedding.go.
func extractEmbeddingInput(event events.Event) (embeddingInput, bool) {
	in := func(entityType, entityID, userID string, parts ...string) (embeddingInput, bool) {
		return embeddingInput{
			entityType: entityType,
			entityID:   entityID,
			userID:     userID,
			text:       joinEmbeddingText(parts...),
		}, true
	}

	switch p := event.Payload().(type) {
	case *events.PromptCreatedPayload:
		return in("prompt", p.PromptID, p.UserID, p.Title, p.Body)
	case *events.PromptUpdatedPayload:
		return in("prompt", p.PromptID, p.UserID, p.Title, p.Body)
	case *events.ArtifactCreatedPayload:
		return in("artifact", p.ArtifactID, p.UserID, p.Title, p.Body)
	case *events.ArtifactUpdatedPayload:
		return in("artifact", p.ArtifactID, p.UserID, p.Title, p.Body)
	case *events.MemoryCreatedPayload:
		return in("memory", p.MemoryID, p.UserID, p.Text)
	case *events.MemoryUpdatedPayload:
		return in("memory", p.MemoryID, p.UserID, p.Text)
	case *events.BlueprintCreatedPayload:
		return in("blueprint", p.BlueprintID, p.UserID, p.Title, p.Body)
	case *events.BlueprintUpdatedPayload:
		return in("blueprint", p.BlueprintID, p.UserID, p.Title, p.Body)
	case *events.FeedItemCreatedPayload:
		return in("feed_item", p.ItemID, p.UserID, p.Title, p.Content)
	case *events.FeedItemReplyCreatedPayload:
		return in("feed_item_reply", p.ReplyID, p.UserID, p.Content)
	default:
		return embeddingInput{}, false
	}
}

// joinEmbeddingText joins non-empty parts (e.g. title + body) with a blank line so
// the embedded text carries both the entity's title and its content.
func joinEmbeddingText(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		if s := strings.TrimSpace(part); s != "" {
			nonEmpty = append(nonEmpty, s)
		}
	}
	return strings.Join(nonEmpty, "\n\n")
}
