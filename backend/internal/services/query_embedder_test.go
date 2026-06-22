package services

import (
	"context"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderQueryEmbedder_HappyPath(t *testing.T) {
	provider := &fakeProvider{vectors: [][]float32{{0.1, 0.2, 0.3}}}
	resolver := &fakeResolver{provider: provider}
	e := NewProviderQueryEmbedder(resolver, "fake-model", 3, logrus.New())

	vec, err := e.EmbedQuery(context.Background(), "find me")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, vec)
	assert.Equal(t, []string{"find me"}, provider.gotTexts)
	assert.Equal(t, "fake-model", resolver.gotModel)
	assert.Equal(t, 3, resolver.gotDims)
}

func TestProviderQueryEmbedder_NoProvider_Errors(t *testing.T) {
	resolver := &fakeResolver{provider: nil}
	e := NewProviderQueryEmbedder(resolver, "fake-model", 3, logrus.New())

	_, err := e.EmbedQuery(context.Background(), "q")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no active embedding provider")
}

func TestProviderQueryEmbedder_ResolverError(t *testing.T) {
	resolver := &fakeResolver{err: errors.New("db down")}
	e := NewProviderQueryEmbedder(resolver, "fake-model", 3, logrus.New())

	_, err := e.EmbedQuery(context.Background(), "q")
	require.Error(t, err)
}
