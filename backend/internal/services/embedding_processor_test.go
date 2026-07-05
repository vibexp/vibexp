package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/pkg/events"
)

// --- test doubles shared by processor + query embedder tests ---

type fakeProvider struct {
	vectors  [][]float32
	err      error
	gotTexts []string
}

func (f *fakeProvider) GenerateEmbeddings(_ context.Context, texts []string) ([][]float32, error) {
	f.gotTexts = texts
	if f.err != nil {
		return nil, f.err
	}
	return f.vectors, nil
}
func (f *fakeProvider) Model() string   { return "fake-model" }
func (f *fakeProvider) Dimensions() int { return 3 }
func (f *fakeProvider) Type() string    { return ProviderTypeOpenAICompatible }

type fakeResolver struct {
	provider  EmbeddingProvider
	err       error
	calls     int
	gotTeamID string
}

func (f *fakeResolver) ResolveActiveProvider(
	_ context.Context, teamID string,
) (*ResolvedEmbeddingProvider, error) {
	f.calls++
	f.gotTeamID = teamID
	if f.err != nil {
		return nil, f.err
	}
	if f.provider == nil {
		return nil, nil
	}
	return &ResolvedEmbeddingProvider{Provider: f.provider, ChunkSize: 1000, ChunkOverlap: 200}, nil
}

type fakeEmbeddingService struct {
	saveCalls  int
	userID     string
	entityType string
	entityID   string
	modelID    string
	chunks     []EmbeddingChunk
	saveErr    error
	team       string
	teamErr    error
}

func (f *fakeEmbeddingService) ResolveEntityTeam(_ context.Context, _, _, _ string) (string, error) {
	return f.team, f.teamErr
}

func (f *fakeEmbeddingService) SaveEmbedding(_, _, _, _ string, _ [][]float32) error { return nil }
func (f *fakeEmbeddingService) SaveEmbeddingChunks(
	userID, entityType, entityID, modelID string, chunks []EmbeddingChunk,
) error {
	f.saveCalls++
	f.userID, f.entityType, f.entityID, f.modelID, f.chunks = userID, entityType, entityID, modelID, chunks
	return f.saveErr
}
func (f *fakeEmbeddingService) GetEmbeddingsByEntity(_, _, _ string) ([]models.Embedding, error) {
	return nil, nil
}
func (f *fakeEmbeddingService) FindSimilar(_, _ string, _ []float32, _ int) ([]models.EmbeddingSimilarity, error) {
	return nil, nil
}
func (f *fakeEmbeddingService) DeleteEmbeddingsByEntity(_, _ string) error { return nil }

func newProcessor(resolver ActiveEmbeddingProviderResolver, svc EmbeddingServiceInterface) *EmbeddingGenerationProcessor {
	return NewEmbeddingGenerationProcessor(resolver, svc, slog.New(slog.DiscardHandler))
}

func TestProcessEvent_HappyPath_SavesChunks(t *testing.T) {
	provider := &fakeProvider{vectors: [][]float32{{0.1, 0.2, 0.3}}}
	resolver := &fakeResolver{provider: provider}
	svc := &fakeEmbeddingService{team: "team-9"}
	p := newProcessor(resolver, svc)

	event := events.NewPromptCreatedEvent(
		"prompt-1", "user-1", "u@example.com", "proj", "slug", "Title", "Body text", time.Now(),
	)

	require.NoError(t, p.ProcessEvent(context.Background(), event))

	assert.Equal(t, "team-9", resolver.gotTeamID)
	require.Equal(t, 1, svc.saveCalls)
	assert.Equal(t, "user-1", svc.userID)
	assert.Equal(t, "prompt", svc.entityType)
	assert.Equal(t, "prompt-1", svc.entityID)
	assert.Equal(t, "fake-model", svc.modelID)
	require.Len(t, svc.chunks, 1)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, svc.chunks[0].Embedding)
	assert.Contains(t, svc.chunks[0].Content, "Title")
	assert.Contains(t, svc.chunks[0].Content, "Body text")
	assert.Equal(t, []string{provider.gotTexts[0]}, provider.gotTexts)
}

func TestProcessEvent_NoProvider_NoOp(t *testing.T) {
	resolver := &fakeResolver{provider: nil} // no provider configured
	svc := &fakeEmbeddingService{}
	p := newProcessor(resolver, svc)

	event := events.NewMemoryCreatedEvent("mem-1", "user-1", "proj", "some memory", time.Now())

	require.NoError(t, p.ProcessEvent(context.Background(), event))
	assert.Equal(t, 1, resolver.calls)
	assert.Equal(t, 0, svc.saveCalls, "no embedding saved when no provider is configured")
}

func TestProcessEvent_NonEmbeddableEvent_NoOp(t *testing.T) {
	resolver := &fakeResolver{provider: &fakeProvider{}}
	svc := &fakeEmbeddingService{}
	p := newProcessor(resolver, svc)

	event := events.NewUserCreatedEvent("user-1", "u@example.com", "Name", time.Now())

	require.NoError(t, p.ProcessEvent(context.Background(), event))
	assert.Equal(t, 0, resolver.calls, "resolver not consulted for non-embeddable events")
	assert.Equal(t, 0, svc.saveCalls)
}

func TestProcessEvent_EmptyText_NoOp(t *testing.T) {
	resolver := &fakeResolver{provider: &fakeProvider{}}
	svc := &fakeEmbeddingService{}
	p := newProcessor(resolver, svc)

	event := events.NewMemoryCreatedEvent("mem-1", "user-1", "proj", "   ", time.Now())

	require.NoError(t, p.ProcessEvent(context.Background(), event))
	assert.Equal(t, 0, resolver.calls)
	assert.Equal(t, 0, svc.saveCalls)
}

func TestProcessEvent_ResolverError_Propagates(t *testing.T) {
	resolver := &fakeResolver{err: errors.New("boom")}
	svc := &fakeEmbeddingService{}
	p := newProcessor(resolver, svc)

	event := events.NewMemoryCreatedEvent("mem-1", "user-1", "proj", "text", time.Now())

	err := p.ProcessEvent(context.Background(), event)
	require.Error(t, err)
	assert.Equal(t, 0, svc.saveCalls)
}

func TestProcessEvent_ProviderError_Propagates(t *testing.T) {
	resolver := &fakeResolver{provider: &fakeProvider{err: errors.New("upstream down")}}
	svc := &fakeEmbeddingService{}
	p := newProcessor(resolver, svc)

	event := events.NewMemoryCreatedEvent("mem-1", "user-1", "proj", "text", time.Now())

	err := p.ProcessEvent(context.Background(), event)
	require.Error(t, err)
	assert.Equal(t, 0, svc.saveCalls)
}
