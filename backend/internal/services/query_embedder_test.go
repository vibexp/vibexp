package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderQueryEmbedder_HappyPath(t *testing.T) {
	provider := &fakeProvider{vectors: [][]float32{{0.1, 0.2, 0.3}}}
	resolver := &fakeResolver{provider: provider}
	e := NewProviderQueryEmbedder(resolver, "fake-model", 3, slog.New(slog.DiscardHandler))

	vec, err := e.EmbedQuery(context.Background(), "find me")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, vec)
	assert.Equal(t, []string{"find me"}, provider.gotTexts)
	assert.Equal(t, "fake-model", resolver.gotModel)
	assert.Equal(t, 3, resolver.gotDims)
}

func TestProviderQueryEmbedder_NoProvider_ReturnsSentinel(t *testing.T) {
	resolver := &fakeResolver{provider: nil}
	e := NewProviderQueryEmbedder(resolver, "fake-model", 3, slog.New(slog.DiscardHandler))

	vec, err := e.EmbedQuery(context.Background(), "q")
	// No provider is a distinguishable, non-fatal condition (the search service
	// branches on it to keyword search), so it must surface as ErrNoEmbeddingProvider
	// with a nil vector — not an opaque error.
	require.ErrorIs(t, err, ErrNoEmbeddingProvider)
	assert.Nil(t, vec)
}

func TestProviderQueryEmbedder_ResolverError(t *testing.T) {
	resolver := &fakeResolver{err: errors.New("db down")}
	e := NewProviderQueryEmbedder(resolver, "fake-model", 3, slog.New(slog.DiscardHandler))

	_, err := e.EmbedQuery(context.Background(), "q")
	require.Error(t, err)
}
