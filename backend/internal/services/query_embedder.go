package services

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
)

// QueryEmbedder converts a free-text query into an embedding vector.
type QueryEmbedder interface {
	// EmbedQuery returns the embedding vector for query. The returned slice always
	// has the configured embedding dimensionality on success.
	EmbedQuery(ctx context.Context, query string) ([]float32, error)
}

// ProviderQueryEmbedder embeds search queries through the active system-wide
// embedding provider — the same provider, model, and dimension used to embed
// documents — so query and document vectors are directly comparable.
type ProviderQueryEmbedder struct {
	resolver   ActiveEmbeddingProviderResolver
	model      string
	dimensions int
	logger     *logrus.Logger
}

// Ensure ProviderQueryEmbedder implements QueryEmbedder.
var _ QueryEmbedder = (*ProviderQueryEmbedder)(nil)

// NewProviderQueryEmbedder creates a ProviderQueryEmbedder. model is
// EMBEDDING_MODEL and dimensions is the fixed EmbeddingVectorDimensions constant.
func NewProviderQueryEmbedder(
	resolver ActiveEmbeddingProviderResolver, model string, dimensions int, logger *logrus.Logger,
) *ProviderQueryEmbedder {
	return &ProviderQueryEmbedder{
		resolver:   resolver,
		model:      model,
		dimensions: dimensions,
		logger:     logger,
	}
}

// EmbedQuery resolves the active provider and embeds query. It returns an error
// when no provider is configured, since semantic search cannot run without one.
func (e *ProviderQueryEmbedder) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	provider, err := e.resolver.ResolveActiveProvider(ctx, e.model, e.dimensions)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve embedding provider: %w", err)
	}
	if provider == nil {
		return nil, fmt.Errorf("no active embedding provider configured")
	}

	vectors, err := provider.GenerateEmbeddings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("expected 1 query embedding, got %d", len(vectors))
	}

	// Per-vector length is validated against the configured dimensions inside the
	// provider, so vectors[0] already matches e.dimensions on success.
	return vectors[0], nil
}
