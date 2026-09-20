package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

const (
	testLLMProviderID = "11111111-1111-1111-1111-111111111111"
	testLLMAPIKey     = "sk-test-not-a-real-key"
)

// newTestLLMService builds the service with a real (fail-closed) EncryptionService
// and a discarding logger. cfg decides the SSRF policy: localDevProviderConfig()
// permits the loopback httptest servers, a production-shaped one refuses them.
func newTestLLMService(
	t *testing.T, repo repositories.ModelProviderRepository, cfg *config.Config,
) *LLMService {
	t.Helper()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	return NewLLMService(repo, enc, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// llmProviderRow is a persisted openai_compatible row pointing at baseURL, with
// its API key sealed exactly as the database holds it.
func llmProviderRow(t *testing.T, baseURL string) *models.ModelProvider {
	t.Helper()
	enc, err := NewEncryptionService(testEncryptionKey)
	require.NoError(t, err)
	sealed, err := enc.Encrypt(testLLMAPIKey)
	require.NoError(t, err)

	return &models.ModelProvider{
		ID:              testLLMProviderID,
		TeamID:          &[]string{testProviderTeamID}[0],
		Name:            "default",
		ProviderType:    ProviderTypeOpenAICompatible,
		Model:           testCompletionModel,
		IsDefault:       true,
		BaseURL:         &baseURL,
		APIKeyEncrypted: &sealed,
	}
}

// llmCompletionServer answers chat/completions with status and body, recording
// whether it was reached at all. The flag is atomic because a handler can still be
// running when the test reads it — the deadline and cancellation cases return from
// Complete while the server is deliberately still sleeping.
func llmCompletionServer(t *testing.T, status int, body string, reached *atomic.Bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Store(true)
		assert.Equal(t, "Bearer "+testLLMAPIKey, r.Header.Get("Authorization"))
		w.WriteHeader(status)
		_, writeErr := io.WriteString(w, body)
		require.NoError(t, writeErr)
	}))
}

func oneUserMessage() models.CompletionRequest {
	return models.CompletionRequest{
		Messages: []models.CompletionMessage{{Role: completionRoleUser, Content: "summarise this"}},
	}
}

// --- Resolve ---

func TestLLMResolve_RequiresTeamID(t *testing.T) {
	svc := newTestLLMService(t, mocks.NewMockModelProviderRepository(t), localDevProviderConfig())

	_, err := svc.Resolve(context.Background(), "   ", nil)

	require.ErrorIs(t, err, ErrNoModelProvider)
}

func TestLLMResolve_UsesTheTeamDefaultWhenNoProviderIsNamed(t *testing.T) {
	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).
		Return(llmProviderRow(t, "https://api.example.com/v1"), nil).Once()

	provider, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Resolve(context.Background(), testProviderTeamID, nil)

	require.NoError(t, err)
	assert.Equal(t, testCompletionModel, provider.Model())
	assert.Equal(t, ProviderTypeOpenAICompatible, provider.Type())
}

func TestLLMResolve_LoadsTheNamedProvider(t *testing.T) {
	repo := mocks.NewMockModelProviderRepository(t)
	row := llmProviderRow(t, "https://api.example.com/v1")
	row.Model = "llama3.1:70b"
	// Naming a provider must not also consult the default.
	repo.EXPECT().GetByID(context.Background(), testProviderTeamID, testLLMProviderID).
		Return(row, nil).Once()

	providerID := testLLMProviderID
	provider, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Resolve(context.Background(), testProviderTeamID, &providerID)

	require.NoError(t, err)
	assert.Equal(t, "llama3.1:70b", provider.Model())
}

func TestLLMResolve_BlankProviderIDFallsBackToTheDefault(t *testing.T) {
	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).
		Return(llmProviderRow(t, "https://api.example.com/v1"), nil).Once()

	blank := "   "
	_, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Resolve(context.Background(), testProviderTeamID, &blank)

	require.NoError(t, err)
}

func TestLLMResolve_NoDefaultProviderIsErrNoModelProvider(t *testing.T) {
	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).
		Return(nil, repositories.ErrDefaultModelProviderNotFound).Once()

	_, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Resolve(context.Background(), testProviderTeamID, nil)

	require.ErrorIs(t, err, ErrNoModelProvider)
}

func TestLLMResolve_NamedProviderNotFoundIsErrNoModelProvider(t *testing.T) {
	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetByID(context.Background(), testProviderTeamID, testLLMProviderID).
		Return(nil, repositories.ErrModelProviderNotFound).Once()

	providerID := testLLMProviderID
	_, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Resolve(context.Background(), testProviderTeamID, &providerID)

	require.ErrorIs(t, err, ErrNoModelProvider)
}

func TestLLMResolve_RepositoryFailureDoesNotFallBack(t *testing.T) {
	// A database failure is NOT "nothing configured": mapping it onto
	// ErrNoModelProvider would let a caller present an outage as an unconfigured
	// team, and picking another provider would spend credits on an unchosen model.
	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).
		Return(nil, errors.New("connection reset")).Once()

	_, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Resolve(context.Background(), testProviderTeamID, nil)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoModelProvider)
}

func TestLLMResolve_UndecryptableKeyDoesNotFallBack(t *testing.T) {
	repo := mocks.NewMockModelProviderRepository(t)
	row := llmProviderRow(t, "https://api.example.com/v1")
	garbage := "not-valid-ciphertext"
	row.APIKeyEncrypted = &garbage
	repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).Return(row, nil).Once()

	_, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Resolve(context.Background(), testProviderTeamID, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt")
	assert.NotErrorIs(t, err, ErrNoModelProvider)
}

func TestLLMResolve_MissingEncryptionServiceFailsClosed(t *testing.T) {
	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).
		Return(llmProviderRow(t, "https://api.example.com/v1"), nil).Once()

	// ProvideEncryptionService returns nil when Security.EncryptionKey is unset; a row
	// WITH a sealed key must then fail rather than call out unauthenticated.
	svc := NewLLMService(repo, nil, localDevProviderConfig(), nil)
	_, err := svc.Resolve(context.Background(), testProviderTeamID, nil)

	require.ErrorIs(t, err, ErrEncryptionUnavailable)
}

func TestLLMResolve_ProviderWithNoStoredKeyIsValid(t *testing.T) {
	// A self-hosted Ollama needs no credential at all.
	repo := mocks.NewMockModelProviderRepository(t)
	row := llmProviderRow(t, "http://ollama.internal:11434/v1")
	row.APIKeyEncrypted = nil
	repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).Return(row, nil).Once()

	provider, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Resolve(context.Background(), testProviderTeamID, nil)

	require.NoError(t, err)
	assert.Equal(t, testCompletionModel, provider.Model())
}

func TestLLMResolve_UnsupportedProviderTypeErrors(t *testing.T) {
	repo := mocks.NewMockModelProviderRepository(t)
	row := llmProviderRow(t, "https://api.example.com/v1")
	row.ProviderType = "anthropic_messages"
	repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).Return(row, nil).Once()

	_, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Resolve(context.Background(), testProviderTeamID, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported model provider type")
	assert.NotErrorIs(t, err, ErrNoModelProvider)
}

// --- Complete ---

func TestLLMComplete_ReturnsContentFinishReasonAndUsage(t *testing.T) {
	var reached atomic.Bool
	server := llmCompletionServer(t, http.StatusOK, `{
		"choices": [{"message": {"role": "assistant", "content": "a summary"}, "finish_reason": "stop"}],
		"usage": {"prompt_tokens": 120, "completion_tokens": 18}
	}`, &reached)
	defer server.Close()

	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).
		Return(llmProviderRow(t, server.URL), nil).Once()

	resp, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Complete(context.Background(), testProviderTeamID, nil, oneUserMessage())

	require.NoError(t, err)
	assert.True(t, reached.Load())
	assert.Equal(t, "a summary", resp.Content)
	assert.Equal(t, "stop", resp.FinishReason)
	assert.Equal(t, models.TokenUsage{PromptTokens: 120, CompletionTokens: 18}, resp.Usage)
}

func TestLLMComplete_ClassifiesProviderStatuses(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		sentinel error
	}{
		{"unauthorized", http.StatusUnauthorized, `{"error":{"message":"invalid api key"}}`, ErrProviderUnauthorized},
		{"forbidden", http.StatusForbidden, `{"error":{"message":"no access"}}`, ErrProviderUnauthorized},
		{"unknown model", http.StatusNotFound, `{"error":{"message":"model not found"}}`, ErrModelRejected},
		{"generic bad request", http.StatusBadRequest, `{"error":{"message":"bad role"}}`, ErrModelRejected},
		{"rate limited", http.StatusTooManyRequests, `{"error":{"message":"slow down"}}`, ErrModelRejected},
		{
			"oversized context",
			http.StatusBadRequest,
			`{"error":{"code":"context_length_exceeded","message":"maximum context length is 8192 tokens"}}`,
			ErrContextTooLarge,
		},
		{"payload too large", http.StatusRequestEntityTooLarge, ``, ErrContextTooLarge},
		{"provider request timeout", http.StatusRequestTimeout, ``, ErrCompletionTimeout},
		{"gateway timeout", http.StatusGatewayTimeout, ``, ErrCompletionTimeout},
		{"server error", http.StatusInternalServerError, `oops`, ErrProviderUnreachable},
		{"bad gateway", http.StatusBadGateway, `<html>nginx</html>`, ErrProviderUnreachable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reached atomic.Bool
			server := llmCompletionServer(t, tt.status, tt.body, &reached)
			defer server.Close()

			repo := mocks.NewMockModelProviderRepository(t)
			repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).
				Return(llmProviderRow(t, server.URL), nil).Once()

			_, err := newTestLLMService(t, repo, localDevProviderConfig()).
				Complete(context.Background(), testProviderTeamID, nil, oneUserMessage())

			require.ErrorIs(t, err, tt.sentinel)
		})
	}
}

func TestLLMComplete_NeverReturnsTheProviderBody(t *testing.T) {
	// The body is a success/failure oracle for the operator's internal network
	// (#464): it is logged, never handed back.
	var reached atomic.Bool
	const secretish = "upstream host db-primary.internal refused the api key"
	server := llmCompletionServer(t, http.StatusUnauthorized, secretish, &reached)
	defer server.Close()

	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).
		Return(llmProviderRow(t, server.URL), nil).Once()

	_, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Complete(context.Background(), testProviderTeamID, nil, oneUserMessage())

	require.ErrorIs(t, err, ErrProviderUnauthorized)
	assert.NotContains(t, err.Error(), secretish)
	assert.NotContains(t, err.Error(), "db-primary.internal")
}

func TestLLMComplete_UnreachableProviderIsErrProviderUnreachable(t *testing.T) {
	var reached atomic.Bool
	server := llmCompletionServer(t, http.StatusOK, `{}`, &reached)
	baseURL := server.URL
	server.Close()

	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).
		Return(llmProviderRow(t, baseURL), nil).Once()

	_, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Complete(context.Background(), testProviderTeamID, nil, oneUserMessage())

	require.ErrorIs(t, err, ErrProviderUnreachable)
	// The dial wording stays server-side.
	assert.NotContains(t, err.Error(), "connection refused")
}

func TestLLMComplete_DeadlineIsErrCompletionTimeout(t *testing.T) {
	var reached atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Store(true)
		// 600ms is a deliberate compromise: httptest's Close() waits for the handler,
		// so this sleep is paid in real wall clock however early the client gives up
		// (measured — the client's abort does not cancel r.Context() here, so
		// selecting on it is dead code). 4x the 150ms budget is margin enough.
		time.Sleep(600 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetDefault(mock.Anything, testProviderTeamID).
		Return(llmProviderRow(t, server.URL), nil).Once()

	// 150ms matches the package's precedent for this shape (agent_invocation_*_test.go
	// use 100-200ms); 30ms is too tight for connect + dispatch under -race, and the
	// handler sleeps far longer, so the deadline still fires first.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Complete(ctx, testProviderTeamID, nil, oneUserMessage())

	require.ErrorIs(t, err, ErrCompletionTimeout)
	assert.True(t, reached.Load())
}

func TestLLMComplete_CancelledContextIsReturnedUnchanged(t *testing.T) {
	// The caller abandoned the request; attributing it to the provider would be a
	// lie, so context.Canceled is the one failure that is not reclassified.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// See the deadline test: Close() waits for the handler, so keep this short.
		time.Sleep(600 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetDefault(mock.Anything, testProviderTeamID).
		Return(llmProviderRow(t, server.URL), nil).Once()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	_, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Complete(ctx, testProviderTeamID, nil, oneUserMessage())

	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, ErrCompletionTimeout)
	assert.NotErrorIs(t, err, ErrProviderUnreachable)
}

func TestLLMComplete_SSRFGuardRefusesAReservedRangeBaseURL(t *testing.T) {
	// A production-shaped config gives the service a guard that refuses loopback at
	// dial time — the whole point of #464. localDevProviderConfig() here would prove
	// nothing.
	var reached atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached.Store(true)
	}))
	defer server.Close()

	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).
		Return(llmProviderRow(t, server.URL), nil).Once()

	prodCfg := &config.Config{Frontend: config.FrontendConfig{BaseURL: "https://app.example.com"}}
	_, err := newTestLLMService(t, repo, prodCfg).
		Complete(context.Background(), testProviderTeamID, nil, oneUserMessage())

	require.ErrorIs(t, err, ErrProviderUnreachable)
	assert.False(t, reached.Load(), "the SSRF guard must refuse the dial before the provider is reached")
	assert.NotContains(t, err.Error(), "disallowed address range")
}

func TestLLMComplete_ResolutionFailurePropagates(t *testing.T) {
	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).
		Return(nil, repositories.ErrDefaultModelProviderNotFound).Once()

	_, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Complete(context.Background(), testProviderTeamID, nil, oneUserMessage())

	require.ErrorIs(t, err, ErrNoModelProvider)
}

func TestLLMComplete_NilServiceIsRefused(t *testing.T) {
	var svc *LLMService

	_, err := svc.Complete(context.Background(), testProviderTeamID, nil, oneUserMessage())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLMService is nil")
}

func TestLLMResolve_NamedProviderRepositoryFailureDoesNotFallBack(t *testing.T) {
	// Same rule as the default path: an outage is not "nothing configured", and it
	// must not quietly resolve some other provider instead.
	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetByID(context.Background(), testProviderTeamID, testLLMProviderID).
		Return(nil, errors.New("connection reset")).Once()

	providerID := testLLMProviderID
	_, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Resolve(context.Background(), testProviderTeamID, &providerID)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoModelProvider)
}

func TestLLMComplete_UnusableProviderResponseIsNotUnreachable(t *testing.T) {
	// The commonest real misconfiguration is a base_url missing its /v1 suffix, so
	// /chat/completions lands on a proxy's 200 catch-all. The provider answered —
	// reporting that as unreachability sends the operator to the network instead of
	// to their base_url.
	tests := []struct {
		name string
		body string
	}{
		{"html from a catch-all route", "<html><body>Not Found</body></html>"},
		{"valid json with no choices", `{"choices": [], "usage": {"prompt_tokens": 7}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reached atomic.Bool
			server := llmCompletionServer(t, http.StatusOK, tt.body, &reached)
			defer server.Close()

			repo := mocks.NewMockModelProviderRepository(t)
			repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).
				Return(llmProviderRow(t, server.URL), nil).Once()

			_, err := newTestLLMService(t, repo, localDevProviderConfig()).
				Complete(context.Background(), testProviderTeamID, nil, oneUserMessage())

			require.ErrorIs(t, err, ErrModelRejected)
			assert.NotErrorIs(t, err, ErrProviderUnreachable)
			assert.True(t, reached.Load())
		})
	}
}

func TestLLMComplete_RejectsEmptyMessagesBeforeTouchingTheDatabase(t *testing.T) {
	// An un-EXPECTed mock is the assertion: mockery fails the test if GetDefault or
	// GetByID is called at all, which is what proves the guard runs before resolution.
	repo := mocks.NewMockModelProviderRepository(t)

	_, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Complete(context.Background(), testProviderTeamID, nil, models.CompletionRequest{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one message")
	// A caller's own bug is not a provider fault.
	assert.NotErrorIs(t, err, ErrProviderUnreachable)
	assert.NotErrorIs(t, err, ErrModelRejected)
	assert.NotErrorIs(t, err, ErrNoModelProvider)
}

func TestLLMComplete_TruncatedBodyIsUnreachableNotRejected(t *testing.T) {
	// A body that fails to ARRIVE is a transport fault, so it must keep the transient
	// ErrProviderUnreachable classification — the marker that turns an unusable-but-
	// intact 2xx into ErrModelRejected must not also swallow this. Getting it wrong
	// inverts the retryability signal for every consumer of this seam.
	//
	// Promising more bytes than are written and then aborting the connection is what
	// makes the client's body read fail mid-stream.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusOK)
		_, writeErr := io.WriteString(w, `{"choices":[{"message":{"content":"par`)
		require.NoError(t, writeErr)
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}))
	defer server.Close()

	repo := mocks.NewMockModelProviderRepository(t)
	repo.EXPECT().GetDefault(context.Background(), testProviderTeamID).
		Return(llmProviderRow(t, server.URL), nil).Once()

	_, err := newTestLLMService(t, repo, localDevProviderConfig()).
		Complete(context.Background(), testProviderTeamID, nil, oneUserMessage())

	require.ErrorIs(t, err, ErrProviderUnreachable)
	assert.NotErrorIs(t, err, ErrModelRejected)
}
