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

func TestProviderQueryEmbedder_AppliesQueryPrefix(t *testing.T) {
	provider := &fakeProvider{vectors: [][]float32{{0.1, 0.2, 0.3}}}
	resolver := &fakeResolver{
		provider:    provider,
		queryPrefix: "Represent this sentence for searching relevant passages: ",
	}
	e := NewProviderQueryEmbedder(resolver, slog.New(slog.DiscardHandler))

	_, _, err := e.EmbedQuery(context.Background(), "team-9", "find me")
	require.NoError(t, err)
	// The provider's query_prefix must be prepended to the exact text sent to the
	// provider — asymmetric models depend on it for ranking quality.
	assert.Equal(t,
		[]string{"Represent this sentence for searching relevant passages: find me"},
		provider.gotTexts,
	)
}

func TestProviderQueryEmbedder_EmptyPrefix_UnchangedQuery(t *testing.T) {
	provider := &fakeProvider{vectors: [][]float32{{0.1, 0.2, 0.3}}}
	resolver := &fakeResolver{provider: provider} // no prefix configured
	e := NewProviderQueryEmbedder(resolver, slog.New(slog.DiscardHandler))

	_, _, err := e.EmbedQuery(context.Background(), "team-9", "find me")
	require.NoError(t, err)
	// An empty prefix must send the query verbatim — exact prior behaviour.
	assert.Equal(t, []string{"find me"}, provider.gotTexts)
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
