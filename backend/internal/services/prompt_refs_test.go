package services

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

func TestParsePromptReferences(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []promptRef
	}{
		{name: "no references", body: "plain text", want: nil},
		{
			name: "explicit reference",
			body: "use @prompt:base now",
			want: []promptRef{{Start: 4, End: 16, Slug: "base", Explicit: true}},
		},
		{
			name: "explicit at start of body",
			body: "@prompt:a-b_1",
			want: []promptRef{{Start: 0, End: 13, Slug: "a-b_1", Explicit: true}},
		},
		{
			name: "email is a legacy match, never explicit",
			body: "mail@shaharia.com",
			want: []promptRef{{Start: 4, End: 13, Slug: "shaharia"}},
		},
		{
			name: "git remote is a legacy match",
			body: "git@github.com:o/r",
			want: []promptRef{{Start: 3, End: 10, Slug: "github"}},
		},
		{
			name: "handle is a legacy match",
			body: "ping @hello",
			want: []promptRef{{Start: 5, End: 11, Slug: "hello"}},
		},
		{
			// A word character before @prompt: makes it literal; only the bare
			// @prompt legacy match remains, exactly as before #1097.
			name: "explicit preceded by a word character is not explicit",
			body: "mail@prompt:x",
			want: []promptRef{{Start: 4, End: 11, Slug: "prompt"}},
		},
		{
			name: "adjacent explicit references with overlapping slugs",
			body: "@prompt:ab @prompt:abc",
			want: []promptRef{
				{Start: 0, End: 10, Slug: "ab", Explicit: true},
				{Start: 11, End: 22, Slug: "abc", Explicit: true},
			},
		},
		{
			name: "explicit and legacy mixed, in body order",
			body: "@legacy then (@prompt:x)",
			want: []promptRef{
				{Start: 0, End: 7, Slug: "legacy"},
				{Start: 14, End: 23, Slug: "x", Explicit: true},
			},
		},
		{
			name: "escaped @@ never parses",
			body: escapedAtSentinel + "prompt:x",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parsePromptReferences(tt.body))
		})
	}
}

// renderRefs renders body with the given options. Prompts in existing resolve;
// slugs in missing are not found. Any other lookup fails the test, because the
// mockery mock rejects an unexpected call: that is the "never resolved" guard.
func renderRefs(
	t *testing.T, body string, existing map[string]string, missing []string,
	placeholders map[string]string, opts RenderOptions,
) (*models.RenderPromptResponse, error) {
	t.Helper()
	mockRepo := mocks.NewMockPromptRepository(t)
	service := createTestPromptService(mockRepo, nil)
	mockRepo.On("GetBySlug", mock.AnythingOfType("context.backgroundCtx"), "user-123", "team-123", "test-prompt").
		Return(&models.Prompt{ID: "prompt-123", Body: body, UserID: "user-123", TeamID: "team-123"}, nil)
	for slug, refBody := range existing {
		mockRepo.On("GetBySlugInTeam", mock.AnythingOfType("context.backgroundCtx"), "team-123", slug).
			Return(&models.Prompt{ID: slug + "-id", Slug: slug, Body: refBody, TeamID: "team-123"}, nil)
	}
	for _, slug := range missing {
		mockRepo.On("GetBySlugInTeam", mock.AnythingOfType("context.backgroundCtx"), "team-123", slug).
			Return(nil, repositories.ErrPromptNotFound)
	}
	return service.RenderPrompt("user-123", "team-123", "test-prompt", placeholders, opts)
}

func TestRenderPrompt_ExplicitReferences(t *testing.T) {
	const literal = "Mail mail@shaharia.com, ping @hello, push to git@github.com:o/r"
	strict := RenderOptions{Strict: true}

	t.Run("strict leaves literal @ text alone and never looks it up", func(t *testing.T) {
		response, err := renderRefs(t, literal, nil, nil, nil, strict)

		require.NoError(t, err)
		assert.Equal(t, literal, response.RenderedBody)
		assert.Empty(t, response.Warnings)
		assert.Empty(t, response.ReferencesUsed)
	})

	t.Run("default mode: unresolved bare @words produce no warnings", func(t *testing.T) {
		response, err := renderRefs(t, literal, nil, []string{"shaharia", "hello", "github"}, nil, RenderOptions{})

		require.NoError(t, err)
		assert.Equal(t, literal, response.RenderedBody)
		assert.Empty(t, response.Warnings)
	})

	t.Run("explicit reference expands in both modes", func(t *testing.T) {
		for _, opts := range []RenderOptions{{}, strict} {
			response, err := renderRefs(t, "A @prompt:x B", map[string]string{"x": "X"}, nil, nil, opts)

			require.NoError(t, err)
			assert.Equal(t, "A X B", response.RenderedBody)
			assert.Equal(t, []string{"x"}, response.ReferencesUsed)
			assert.Empty(t, response.Warnings)
		}
	})

	t.Run("default mode warns on an unresolved explicit reference", func(t *testing.T) {
		response, err := renderRefs(t, "A @prompt:nope B", nil, []string{"nope"}, nil, RenderOptions{})

		require.NoError(t, err)
		assert.Equal(t, "A @prompt:nope B", response.RenderedBody)
		assert.Equal(t, []string{"Reference not found: @prompt:nope"}, response.Warnings)
	})

	t.Run("strict fails on unresolved explicit references, naming each once", func(t *testing.T) {
		response, err := renderRefs(t, "@prompt:nope @prompt:gone @prompt:nope",
			nil, []string{"nope", "gone"}, nil, strict)

		require.Error(t, err)
		assert.Nil(t, response)
		var unresolved *ErrUnresolvedReferences
		require.ErrorAs(t, err, &unresolved)
		assert.Equal(t, []string{"nope", "gone"}, unresolved.Slugs)
		assert.Equal(t, "unresolved prompt references: nope, gone", err.Error())
	})

	t.Run("strict fails on an unresolved explicit reference in a nested body", func(t *testing.T) {
		_, err := renderRefs(t, "@prompt:outer", map[string]string{"outer": "hi @prompt:inner"},
			[]string{"inner"}, nil, strict)

		var unresolved *ErrUnresolvedReferences
		require.ErrorAs(t, err, &unresolved)
		assert.Equal(t, []string{"inner"}, unresolved.Slugs)
	})

	t.Run("legacy bare reference still expands in default mode", func(t *testing.T) {
		response, err := renderRefs(t, "A @x B", map[string]string{"x": "X"}, nil, nil, RenderOptions{})

		require.NoError(t, err)
		assert.Equal(t, "A X B", response.RenderedBody)
		assert.Equal(t, []string{"x"}, response.ReferencesUsed)
	})

	t.Run("legacy bare reference is literal text in strict mode, at any depth", func(t *testing.T) {
		response, err := renderRefs(t, "A @x B @prompt:outer",
			map[string]string{"outer": "nested @x"}, nil, nil, strict)

		require.NoError(t, err)
		assert.Equal(t, "A @x B nested @x", response.RenderedBody)
		assert.Equal(t, []string{"outer"}, response.ReferencesUsed)
	})

	t.Run("placeholder values are inserted verbatim, never resolved", func(t *testing.T) {
		values := map[string]string{"a": "@prompt:x", "b": "git@github.com:o/r"}
		for _, opts := range []RenderOptions{{}, strict} {
			response, err := renderRefs(t, "{{a}} and {{b}}", nil, nil, values, opts)

			require.NoError(t, err)
			assert.Equal(t, "@prompt:x and git@github.com:o/r", response.RenderedBody)
			assert.Empty(t, response.Warnings)
			assert.Empty(t, response.ReferencesUsed)
		}
	})

	t.Run("overlapping slugs each expand to their own prompt", func(t *testing.T) {
		response, err := renderRefs(t, "@prompt:ab and @prompt:abc",
			map[string]string{"ab": "AB", "abc": "ABC"}, nil, nil, strict)

		require.NoError(t, err)
		assert.Equal(t, "AB and ABC", response.RenderedBody)
		assert.ElementsMatch(t, []string{"ab", "abc"}, response.ReferencesUsed)
	})

	t.Run("legacy overlapping slugs no longer clobber each other", func(t *testing.T) {
		response, err := renderRefs(t, "@ab and @abc",
			map[string]string{"ab": "AB", "abc": "ABC"}, nil, nil, RenderOptions{})

		require.NoError(t, err)
		assert.Equal(t, "AB and ABC", response.RenderedBody)
	})

	t.Run("explicit after a word character is literal", func(t *testing.T) {
		response, err := renderRefs(t, "mail@prompt:x", nil, nil, nil, strict)

		require.NoError(t, err)
		assert.Equal(t, "mail@prompt:x", response.RenderedBody)
		assert.Empty(t, response.Warnings)
	})

	t.Run("escaped @@prompt:x renders as literal @prompt:x", func(t *testing.T) {
		response, err := renderRefs(t, "write @@prompt:x to reference", nil, nil, nil, RenderOptions{})

		require.NoError(t, err)
		assert.Equal(t, "write @prompt:x to reference", response.RenderedBody)
		assert.Empty(t, response.Warnings)
	})
}

func TestExtractAllPlaceholders_ExplicitAndLegacyReferences(t *testing.T) {
	mockRepo := mocks.NewMockPromptRepository(t)
	service := createTestPromptService(mockRepo, nil)
	mockRepo.On("GetBySlugInTeam", mock.Anything, "team-123", "a").
		Return(&models.Prompt{ID: "a-id", Slug: "a", Body: "{{y}}"}, nil)
	mockRepo.On("GetBySlugInTeam", mock.Anything, "team-123", "b").
		Return(&models.Prompt{ID: "b-id", Slug: "b", Body: "{{z}}"}, nil)
	mockRepo.On("GetBySlugInTeam", mock.Anything, "team-123", "example").
		Return(nil, repositories.ErrPromptNotFound)

	keys, err := service.ExtractAllPlaceholders("team-123", "{{x}} @prompt:a @b me@example.com", map[string]bool{})

	require.NoError(t, err)
	assert.Equal(t, []string{"x", "y", "z"}, keys)
}

func TestUpdatePromptReferences_RecordsExplicitAndResolvedLegacy(t *testing.T) {
	mockRepo := mocks.NewMockPromptRepository(t)
	refRepo := mocks.NewMockPromptReferenceRepository(t)
	service := NewPromptService(PromptServiceDeps{
		Repo:    mockRepo,
		RefRepo: refRepo,
		Authz:   allowAllAuthz{},
		Logger:  func() *slog.Logger { l, _ := logtest.New(); return l }(),
	})

	mockRepo.On("GetBySlugInTeam", mock.Anything, "team-123", "ab").
		Return(&models.Prompt{ID: "ab-id", Slug: "ab"}, nil)
	mockRepo.On("GetBySlugInTeam", mock.Anything, "team-123", "abc").
		Return(&models.Prompt{ID: "abc-id", Slug: "abc"}, nil)
	mockRepo.On("GetBySlugInTeam", mock.Anything, "team-123", "legacy").
		Return(&models.Prompt{ID: "legacy-id", Slug: "legacy"}, nil)
	mockRepo.On("GetBySlugInTeam", mock.Anything, "team-123", "example").
		Return(nil, repositories.ErrPromptNotFound)
	refRepo.On("DeleteByPromptID", mock.Anything, "prompt-1").Return(nil)

	var stored []string
	refRepo.On("CreateBatch", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			for _, ref := range args.Get(1).([]models.PromptReference) {
				assert.Equal(t, "prompt-1", ref.PromptID)
				stored = append(stored, ref.ReferencedPromptID)
			}
		}).Return(nil)

	err := service.updatePromptReferences(context.Background(), "team-123", "prompt-1",
		"@prompt:ab @prompt:abc @legacy me@example.com @@prompt:escaped")

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"ab-id", "abc-id", "legacy-id"}, stored)
}
