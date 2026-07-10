package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

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
// model is EMBEDDING_MODEL and dimensions is the fixed EmbeddingVectorDimensions
// constant. model is the model_id tag written on every chunk row and is also used
// by search to filter candidate rows, so document and query embeddings stay
// comparable.
type EmbeddingGenerationProcessor struct {
	resolver         ActiveEmbeddingProviderResolver
	embeddingService EmbeddingServiceInterface
	logger           *slog.Logger
}

// Ensure EmbeddingGenerationProcessor implements events.EmbeddingProcessor.
var _ events.EmbeddingProcessor = (*EmbeddingGenerationProcessor)(nil)

// NewEmbeddingGenerationProcessor creates an EmbeddingGenerationProcessor. The
// provider (and its model + chunk sizing) is resolved per event from the entity's
// team, so there is no global model or chunker.
func NewEmbeddingGenerationProcessor(
	resolver ActiveEmbeddingProviderResolver,
	embeddingService EmbeddingServiceInterface,
	logger *slog.Logger,
) *EmbeddingGenerationProcessor {
	return &EmbeddingGenerationProcessor{
		resolver:         resolver,
		embeddingService: embeddingService,
		logger:           logger,
	}
}

// ProcessEvent resolves the provider, chunks, embeds, and saves. It no-ops when
// the event is not embeddable, carries no text, or no provider is configured —
// the originating entity write must never be failed by embedding generation.
func (p *EmbeddingGenerationProcessor) ProcessEvent(ctx context.Context, event events.Event) error {
	input, teamID, resolved, err := p.resolveJob(ctx, event)
	if err != nil {
		return err
	}
	if resolved == nil {
		return nil // not embeddable, no text, or no provider configured
	}
	return p.generateAndSave(ctx, input, teamID, resolved)
}

// resolveJob extracts the embeddable input from event and resolves the entity's
// team and active provider. It returns a nil *ResolvedEmbeddingProvider (with no
// error) for the no-op cases the caller skips: an event that is not embeddable,
// carries no text, or whose team has no provider configured. A non-nil error is a
// genuine resolution failure (team lookup or provider decode) worth surfacing. It
// is the shared front half of the sync ProcessEvent path and the async
// EmbeddingDispatcher, so both resolve identically.
func (p *EmbeddingGenerationProcessor) resolveJob(
	ctx context.Context, event events.Event,
) (embeddingInput, string, *ResolvedEmbeddingProvider, error) {
	input, ok := extractEmbeddingInput(event)
	if !ok {
		return embeddingInput{}, "", nil, nil // event type is not embeddable
	}
	if input.entityID == "" || input.userID == "" || strings.TrimSpace(input.text) == "" {
		return embeddingInput{}, "", nil, nil // nothing to embed
	}

	teamID, err := p.embeddingService.ResolveEntityTeam(ctx, input.userID, input.entityType, input.entityID)
	if err != nil {
		return embeddingInput{}, "", nil, fmt.Errorf(
			"failed to resolve team for %s %s: %w", input.entityType, input.entityID, err,
		)
	}

	resolved, err := p.resolver.ResolveActiveProvider(ctx, teamID)
	if err != nil {
		return embeddingInput{}, "", nil, fmt.Errorf("failed to resolve embedding provider: %w", err)
	}
	if resolved == nil {
		p.logger.With(
			"service", "embedding",
			"component", "embedding-processor",
			"entity_type", input.entityType,
			"entity_id", input.entityID,
			"team_id", teamID,
		).Debug("No active embedding provider configured for team; skipping embedding generation")
		return embeddingInput{}, teamID, nil, nil
	}

	return input, teamID, resolved, nil
}

// generateAndSave chunks the input, embeds it with the team's resolved provider,
// and persists the chunks tagged with that provider's model.
func (p *EmbeddingGenerationProcessor) generateAndSave(
	ctx context.Context, input embeddingInput, teamID string, resolved *ResolvedEmbeddingProvider,
) error {
	chunker := NewTextChunker(resolved.ChunkSize, resolved.ChunkOverlap)
	chunkTexts := chunker.Chunk(input.text)
	if len(chunkTexts) == 0 {
		return nil
	}

	vectors, err := resolved.Provider.GenerateEmbeddings(ctx, chunkTexts)
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

	model := resolved.Provider.Model()
	if err := p.embeddingService.SaveEmbeddingChunks(
		input.userID, input.entityType, input.entityID, model, chunks,
	); err != nil {
		return fmt.Errorf("failed to save embeddings for %s %s: %w", input.entityType, input.entityID, err)
	}

	p.logger.With(
		"service", "embedding",
		"component", "embedding-processor",
		"entity_type", input.entityType,
		"entity_id", input.entityID,
		"team_id", teamID,
		"model", model,
		"chunk_count", len(chunks),
	).Info("Embeddings generated and saved")

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
