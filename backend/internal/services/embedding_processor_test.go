package services

import (
	"context"
	"errors"
	"log/slog"
	"strings"
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

// echoCountProvider returns one vector per input text, so it works for
// multi-chunk inputs (the processor requires len(vectors) == len(chunks)). It
// records the exact texts it was asked to embed.
type echoCountProvider struct{ gotTexts []string }

func (e *echoCountProvider) GenerateEmbeddings(_ context.Context, texts []string) ([][]float32, error) {
	e.gotTexts = texts
	vectors := make([][]float32, len(texts))
	for i := range texts {
		vectors[i] = []float32{0.1, 0.2, 0.3}
	}
	return vectors, nil
}
func (e *echoCountProvider) Model() string   { return "fake-model" }
func (e *echoCountProvider) Dimensions() int { return 3 }
func (e *echoCountProvider) Type() string    { return ProviderTypeOpenAICompatible }

type fakeResolver struct {
	provider       EmbeddingProvider
	err            error
	calls          int
	gotTeamID      string
	queryPrefix    string
	documentPrefix string
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
	return &ResolvedEmbeddingProvider{
		Provider:       f.provider,
		ChunkSize:      1000,
		ChunkOverlap:   200,
		QueryPrefix:    f.queryPrefix,
		DocumentPrefix: f.documentPrefix,
	}, nil
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
		"prompt-1", "user-1", "u@example.com", "proj", "slug", "Title", "", "Body text", time.Now(),
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

func TestProcessEvent_AppliesDocumentPrefix_StoresRawContent(t *testing.T) {
	provider := &fakeProvider{vectors: [][]float32{{0.1, 0.2, 0.3}}}
	resolver := &fakeResolver{provider: provider, documentPrefix: "passage: "}
	svc := &fakeEmbeddingService{team: "team-9"}
	p := newProcessor(resolver, svc)

	event := events.NewPromptCreatedEvent(
		"prompt-1", "user-1", "u@example.com", "proj", "slug", "Title", "", "Body text", time.Now(),
	)

	require.NoError(t, p.ProcessEvent(context.Background(), event))

	// The text SENT to the provider must carry the document prefix...
	require.Len(t, provider.gotTexts, 1)
	assert.True(t,
		strings.HasPrefix(provider.gotTexts[0], "passage: "),
		"embedded text must be prefixed with document_prefix",
	)
	// ...but the STORED chunk content must stay raw (the prefix is a model
	// instruction, not part of the document — it must not pollute snippets or
	// keyword-search fallback).
	require.Len(t, svc.chunks, 1)
	assert.False(t,
		strings.HasPrefix(svc.chunks[0].Content, "passage: "),
		"stored content must not include the prefix",
	)
	assert.Contains(t, svc.chunks[0].Content, "Title")
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

// --- context header (issue #173) ---

func TestProcessEvent_ContextHeader_MultiChunk_EveryChunkPrefixed(t *testing.T) {
	provider := &echoCountProvider{}
	resolver := &fakeResolver{provider: provider}
	svc := &fakeEmbeddingService{team: "team-1"}
	p := newProcessor(resolver, svc)

	// Body well over the 1000-rune window so it spans several chunks.
	body := strings.Repeat("lorem ipsum dolor sit amet ", 200)
	event := events.NewArtifactCreatedEvent(
		"art-1", "user-1", "proj", "slug", "My Title", "A short description", "note", body, time.Now(),
	)

	require.NoError(t, p.ProcessEvent(context.Background(), event))

	require.Greater(t, len(svc.chunks), 1, "body should span multiple chunks")
	header := "My Title\nA short description"
	for i, c := range svc.chunks {
		assert.Truef(t, strings.HasPrefix(c.Content, header),
			"chunk %d must start with the title+description header", i)
	}
	// With no document prefix, the stored content is exactly the embedded text.
	require.Equal(t, len(svc.chunks), len(provider.gotTexts))
	for i := range svc.chunks {
		assert.Equal(t, svc.chunks[i].Content, provider.gotTexts[i],
			"stored chunk content must equal the embedded text")
	}
}

func TestProcessEvent_ContextHeader_TitleOnly(t *testing.T) {
	provider := &echoCountProvider{}
	resolver := &fakeResolver{provider: provider}
	svc := &fakeEmbeddingService{team: "team-1"}
	p := newProcessor(resolver, svc)

	body := strings.Repeat("lorem ipsum dolor sit amet ", 200)
	// Empty description: header is the title alone.
	event := events.NewArtifactCreatedEvent(
		"art-1", "user-1", "proj", "slug", "My Title", "", "note", body, time.Now(),
	)

	require.NoError(t, p.ProcessEvent(context.Background(), event))

	require.Greater(t, len(svc.chunks), 1)
	for i, c := range svc.chunks {
		assert.Truef(t, strings.HasPrefix(c.Content, "My Title"),
			"chunk %d must start with the title header", i)
	}
	// No description means the header line has no second line before the body.
	assert.True(t, strings.HasPrefix(svc.chunks[0].Content, "My Title\n\n"))
}

func TestProcessEvent_ContextHeader_SingleChunk_HeaderPlusBody(t *testing.T) {
	provider := &echoCountProvider{}
	resolver := &fakeResolver{provider: provider}
	svc := &fakeEmbeddingService{team: "team-1"}
	p := newProcessor(resolver, svc)

	event := events.NewArtifactCreatedEvent(
		"art-1", "user-1", "proj", "slug", "My Title", "Desc", "note", "short body", time.Now(),
	)

	require.NoError(t, p.ProcessEvent(context.Background(), event))

	require.Len(t, svc.chunks, 1)
	assert.Equal(t, "My Title\nDesc\n\nshort body", svc.chunks[0].Content)
}

func TestProcessEvent_UntitledMemory_ByteIdentical_NoHeader(t *testing.T) {
	provider := &echoCountProvider{}
	resolver := &fakeResolver{provider: provider}
	svc := &fakeEmbeddingService{team: "team-1"}
	p := newProcessor(resolver, svc)

	event := events.NewMemoryCreatedEvent("mem-1", "user-1", "proj", "just the memory text", time.Now())

	require.NoError(t, p.ProcessEvent(context.Background(), event))

	require.Len(t, svc.chunks, 1)
	// Untitled/undescribed entities carry no header — stored content is the raw body.
	assert.Equal(t, "just the memory text", svc.chunks[0].Content)
}

func TestEmbeddingInputHeader_Truncation(t *testing.T) {
	// Description is truncated to maxHeaderDescriptionRunes.
	longDesc := strings.Repeat("x", 500)
	got := embeddingInput{title: "T", description: longDesc}.header()
	assert.Equal(t, "T\n"+strings.Repeat("x", maxHeaderDescriptionRunes), got)

	// The whole header is capped at maxHeaderRunes even with a pathological title.
	longTitle := strings.Repeat("y", 600)
	capped := embeddingInput{title: longTitle}.header()
	assert.Equal(t, maxHeaderRunes, len([]rune(capped)))

	// Neither title nor description → no header.
	assert.Equal(t, "", embeddingInput{body: "b"}.header())
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
