package events

import (
	"context"

	"github.com/sirupsen/logrus"
)

// EmbeddingProcessor generates and persists embeddings for a domain event. It is
// the port the EmbeddingWorker depends on; the implementation lives in
// internal/services (which may import pkg/events), keeping this package free of a
// dependency on the services layer. This mirrors the import-cycle-breaking seam
// the removed HTTP sync listener used.
type EmbeddingProcessor interface {
	// ProcessEvent resolves the active embedding provider, chunks the event's
	// entity text, embeds it, and persists the chunks. It must no-op (return nil)
	// when no provider is configured or the event carries no embeddable text, so a
	// missing provider or an unrelated event never fails the originating write.
	ProcessEvent(ctx context.Context, event Event) error
}

// EmbeddingWorker is the in-process async embedding listener. It subscribes to
// entity created/updated events on the in-memory event bus and delegates the
// actual generation to an EmbeddingProcessor. The bus's existing non-blocking
// worker pool (with retry/backoff) provides the "async goroutine worker", so this
// listener stays thin and adds no new concurrency primitives.
type EmbeddingWorker struct {
	processor  EmbeddingProcessor
	eventTypes []string
	logger     *logrus.Logger
}

// Ensure EmbeddingWorker implements EventListener.
var _ EventListener = (*EmbeddingWorker)(nil)

// NewEmbeddingWorker creates an EmbeddingWorker subscribed to the entity
// created/updated events that drive embedding generation.
func NewEmbeddingWorker(processor EmbeddingProcessor, logger *logrus.Logger) *EmbeddingWorker {
	return &EmbeddingWorker{
		processor:  processor,
		eventTypes: EmbeddingEventTypes(),
		logger:     logger,
	}
}

// Handle delegates to the processor. Any error is returned so the event bus can
// apply its retry/backoff policy; generation stays fully off the request path.
func (w *EmbeddingWorker) Handle(ctx context.Context, event Event) error {
	return w.processor.ProcessEvent(ctx, event)
}

// EventTypes returns the entity created/updated events this worker handles.
func (w *EmbeddingWorker) EventTypes() []string {
	return w.eventTypes
}

// EmbeddingEventTypes is the set of entity events that trigger embedding
// generation. Feed items and replies are immutable, so only their created events
// appear here.
func EmbeddingEventTypes() []string {
	return []string{
		EventTypePromptCreated, EventTypePromptUpdated,
		EventTypeArtifactCreated, EventTypeArtifactUpdated,
		EventTypeMemoryCreated, EventTypeMemoryUpdated,
		EventTypeBlueprintCreated, EventTypeBlueprintUpdated,
		EventTypeFeedItemCreated,
		EventTypeFeedItemReplyCreated,
	}
}
