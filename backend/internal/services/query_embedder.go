package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// ErrNoEmbeddingProvider signals that no active embedding provider is configured,
// so a query cannot be embedded. It is non-fatal and distinguishable: SearchService
// treats it as "embedding disabled" and falls back to keyword (full-text) search
// rather than failing the request.
var ErrNoEmbeddingProvider = errors.New("no active embedding provider configured")

// QueryEmbedder converts a free-text query into an embedding vector.
type QueryEmbedder interface {
	// EmbedQuery returns the embedding vector for query using the given team's
	// active provider, plus the model id that produced it (so callers filter
	// stored embeddings by the same model). It returns ErrNoEmbeddingProvider (a
	// nil vector, empty model) when the team has no provider configured, signalling
	// callers to fall back to keyword search instead of erroring.
	EmbedQuery(ctx context.Context, teamID, query string) ([]float32, string, error)
}

// ProviderQueryEmbedder embeds search queries through a team's active embedding
// provider — the same provider, model, and dimension used to embed that team's
// documents — so query and document vectors are directly comparable.
type ProviderQueryEmbedder struct {
	resolver ActiveEmbeddingProviderResolver
	logger   *slog.Logger
}

// Ensure ProviderQueryEmbedder implements QueryEmbedder.
var _ QueryEmbedder = (*ProviderQueryEmbedder)(nil)

// NewProviderQueryEmbedder creates a ProviderQueryEmbedder.
func NewProviderQueryEmbedder(
	resolver ActiveEmbeddingProviderResolver, logger *slog.Logger,
) *ProviderQueryEmbedder {
	return &ProviderQueryEmbedder{
		resolver: resolver,
		logger:   logger,
	}
}

// EmbedQuery resolves the team's active provider and embeds query. It returns
// ErrNoEmbeddingProvider when none is configured so the caller can fall back to
// keyword search rather than failing — semantic search cannot run without one.
func (e *ProviderQueryEmbedder) EmbedQuery(
	ctx context.Context, teamID, query string,
) ([]float32, string, error) {
	resolved, err := e.resolver.ResolveActiveProvider(ctx, teamID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to resolve embedding provider: %w", err)
	}
	if resolved == nil {
		return nil, "", ErrNoEmbeddingProvider
	}

	vectors, err := resolved.Provider.GenerateEmbeddings(ctx, []string{query})
	if err != nil {
		return nil, "", fmt.Errorf("failed to embed query: %w", err)
	}
	if len(vectors) != 1 {
		return nil, "", fmt.Errorf("expected 1 query embedding, got %d", len(vectors))
	}

	// Per-vector length is validated against the configured dimensions inside the
	// provider, so vectors[0] already matches the fixed width on success.
	return vectors[0], resolved.Provider.Model(), nil
}
