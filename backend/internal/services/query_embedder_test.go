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
	e := NewProviderQueryEmbedder(resolver, slog.New(slog.DiscardHandler))

	vec, model, err := e.EmbedQuery(context.Background(), "team-9", "find me")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, vec)
	assert.Equal(t, "fake-model", model)
	assert.Equal(t, []string{"find me"}, provider.gotTexts)
	assert.Equal(t, "team-9", resolver.gotTeamID)
}

func TestProviderQueryEmbedder_NoProvider_ReturnsSentinel(t *testing.T) {
	resolver := &fakeResolver{provider: nil}
	e := NewProviderQueryEmbedder(resolver, slog.New(slog.DiscardHandler))

	vec, model, err := e.EmbedQuery(context.Background(), "team-1", "q")
	// No provider is a distinguishable, non-fatal condition (the search service
	// branches on it to keyword search), so it must surface as ErrNoEmbeddingProvider
	// with a nil vector — not an opaque error.
	require.ErrorIs(t, err, ErrNoEmbeddingProvider)
	assert.Nil(t, vec)
	assert.Empty(t, model)
}

func TestProviderQueryEmbedder_ResolverError(t *testing.T) {
	resolver := &fakeResolver{err: errors.New("db down")}
	e := NewProviderQueryEmbedder(resolver, slog.New(slog.DiscardHandler))

	_, _, err := e.EmbedQuery(context.Background(), "team-1", "q")
	require.Error(t, err)
}
