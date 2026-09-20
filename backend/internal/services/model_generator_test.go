package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

const testCompletionModel = "gpt-4o-mini"

// newTestCompletionProvider points a provider at an httptest server. It uses the
// loopback-permitting guard; a test asserting the guard's REJECTIONS must build a
// production-shaped one instead (see TestComplete_SSRFGuardRefusesReservedRange).
func newTestCompletionProvider(t *testing.T, baseURL string) *OpenAICompatibleModelProvider {
	t.Helper()
	provider, err := NewOpenAICompatibleModelProvider(
		baseURL, "test-key", testCompletionModel, 5*time.Second, loopbackProviderGuard(),
	)
	require.NoError(t, err)
	return provider
}

// completionServer serves one canned chat/completions response, and fails the test
// if anything else is requested.
func completionServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, writeErr := io.WriteString(w, body)
		require.NoError(t, writeErr)
	}))
}

func TestComplete_SuccessWithUsage(t *testing.T) {
	server := completionServer(t, http.StatusOK, `{
		"choices": [{"message": {"role": "assistant", "content": "hello there"}, "finish_reason": "stop"}],
		"usage": {"prompt_tokens": 11, "completion_tokens": 3}
	}`)
	defer server.Close()

	resp, err := newTestCompletionProvider(t, server.URL).Complete(context.Background(), models.CompletionRequest{
		Messages: []models.CompletionMessage{{Role: completionRoleUser, Content: "hi"}},
	})

	require.NoError(t, err)
	assert.Equal(t, "hello there", resp.Content)
	assert.Equal(t, "stop", resp.FinishReason)
	assert.Equal(t, 11, resp.Usage.PromptTokens)
	assert.Equal(t, 3, resp.Usage.CompletionTokens)
}

func TestComplete_SuccessWithoutUsage(t *testing.T) {
	// An OpenAI-compatible server is not obliged to report usage; a zero here means
	// "unreported", and Complete must not fail over its absence.
	server := completionServer(t, http.StatusOK, `{
		"choices": [{"message": {"role": "assistant", "content": "ok"}, "finish_reason": "length"}]
	}`)
	defer server.Close()

	resp, err := newTestCompletionProvider(t, server.URL).Complete(context.Background(), models.CompletionRequest{
		Messages: []models.CompletionMessage{{Role: completionRoleUser, Content: "hi"}},
	})

	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Content)
	assert.Equal(t, "length", resp.FinishReason)
	assert.Equal(t, models.TokenUsage{}, resp.Usage)
}

func TestComplete_SendsConfiguredModelMessagesAndTuning(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		_, writeErr := io.WriteString(w, `{"choices":[{"message":{"content":"x"},"finish_reason":"stop"}]}`)
		require.NoError(t, writeErr)
	}))
	defer server.Close()

	temperature := 0.25
	_, err := newTestCompletionProvider(t, server.URL).Complete(context.Background(), models.CompletionRequest{
		Messages: []models.CompletionMessage{
			{Role: "system", Content: "be brief"},
			{Role: completionRoleUser, Content: "hi"},
		},
		MaxTokens:   64,
		Temperature: &temperature,
	})
	require.NoError(t, err)

	// The model is the provider's own, never the caller's: a caller picks a model by
	// picking a provider.
	assert.Equal(t, testCompletionModel, captured["model"])
	assert.InDelta(t, 0.25, captured["temperature"], 0.0001)
	assert.InDelta(t, 64.0, captured["max_tokens"], 0.0001)
	messages, ok := captured["messages"].([]any)
	require.True(t, ok)
	require.Len(t, messages, 2)
	assert.Equal(t, map[string]any{"role": "system", "content": "be brief"}, messages[0])
}

func TestComplete_OmitsUnsetTuningKnobs(t *testing.T) {
	// max_tokens:0 reads as "generate nothing" on an OpenAI-compatible server, and a
	// temperature of 0 is a real value — so both must be absent when unset, not zero.
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
		_, writeErr := io.WriteString(w, `{"choices":[{"message":{"content":"x"},"finish_reason":"stop"}]}`)
		require.NoError(t, writeErr)
	}))
	defer server.Close()

	_, err := newTestCompletionProvider(t, server.URL).Complete(context.Background(), models.CompletionRequest{
		Messages: []models.CompletionMessage{{Role: completionRoleUser, Content: "hi"}},
	})
	require.NoError(t, err)

	assert.NotContains(t, captured, "max_tokens")
	assert.NotContains(t, captured, "temperature")
}

func TestComplete_ExplicitZeroTemperatureIsSent(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
		_, writeErr := io.WriteString(w, `{"choices":[{"message":{"content":"x"},"finish_reason":"stop"}]}`)
		require.NoError(t, writeErr)
	}))
	defer server.Close()

	zero := 0.0
	_, err := newTestCompletionProvider(t, server.URL).Complete(context.Background(), models.CompletionRequest{
		Messages:    []models.CompletionMessage{{Role: completionRoleUser, Content: "hi"}},
		Temperature: &zero,
	})
	require.NoError(t, err)

	require.Contains(t, captured, "temperature")
	assert.InDelta(t, 0.0, captured["temperature"], 0.0001)
}

func TestComplete_NonSuccessReturnsTypedErrorCarryingCappedBody(t *testing.T) {
	server := completionServer(t, http.StatusUnauthorized, `{"error":{"message":"invalid api key"}}`)
	defer server.Close()

	_, err := newTestCompletionProvider(t, server.URL).Complete(context.Background(), models.CompletionRequest{
		Messages: []models.CompletionMessage{{Role: completionRoleUser, Content: "hi"}},
	})

	var httpErr *completionHTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnauthorized, httpErr.StatusCode)
	assert.Contains(t, httpErr.Body, "invalid api key")
	// The message names the endpoint it really came from, not the embeddings one.
	assert.Contains(t, httpErr.Error(), "chat completions endpoint returned status 401")
}

func TestComplete_ErrorBodyIsCappedAt512Bytes(t *testing.T) {
	server := completionServer(t, http.StatusBadGateway, string(make([]byte, 4096)))
	defer server.Close()

	_, err := newTestCompletionProvider(t, server.URL).Complete(context.Background(), models.CompletionRequest{
		Messages: []models.CompletionMessage{{Role: completionRoleUser, Content: "hi"}},
	})

	var httpErr *completionHTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.LessOrEqual(t, len(httpErr.Body), maxCompletionErrorBodyBytes)
}

func TestComplete_TransportFailureIsNotATypedHTTPError(t *testing.T) {
	server := completionServer(t, http.StatusOK, `{}`)
	baseURL := server.URL
	server.Close()

	_, err := newTestCompletionProvider(t, baseURL).Complete(context.Background(), models.CompletionRequest{
		Messages: []models.CompletionMessage{{Role: completionRoleUser, Content: "hi"}},
	})

	require.Error(t, err)
	var httpErr *completionHTTPError
	assert.False(t, errors.As(err, &httpErr))
}

func TestComplete_RejectsEmptyMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("Complete must not call the provider for an empty message list")
	}))
	defer server.Close()

	_, err := newTestCompletionProvider(t, server.URL).Complete(
		context.Background(), models.CompletionRequest{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one message")
}

func TestComplete_RejectsResponseWithNoChoices(t *testing.T) {
	// "" is not an answer: rendering it would present a provider fault as a result.
	server := completionServer(t, http.StatusOK, `{"choices": [], "usage": {"prompt_tokens": 4}}`)
	defer server.Close()

	_, err := newTestCompletionProvider(t, server.URL).Complete(context.Background(), models.CompletionRequest{
		Messages: []models.CompletionMessage{{Role: completionRoleUser, Content: "hi"}},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

func TestComplete_SSRFGuardRefusesReservedRange(t *testing.T) {
	// A production-shaped guard (allowPrivate false) must refuse the loopback
	// httptest server at DIAL time, so the handler is never reached.
	var called atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called.Store(true)
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleModelProvider(
		server.URL, "test-key", testCompletionModel, 5*time.Second, &ssrfGuard{},
	)
	require.NoError(t, err)

	_, err = provider.Complete(context.Background(), models.CompletionRequest{
		Messages: []models.CompletionMessage{{Role: completionRoleUser, Content: "hi"}},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disallowed address range")
	assert.False(t, called.Load(), "the SSRF guard must refuse the dial before the provider is reached")
}

func TestValidateProbe_StillSendsMaxTokensOne(t *testing.T) {
	// Complete's omitempty on max_tokens must not silently drop the validation
	// probe's max_tokens:1, which is what keeps the probe cheap.
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := newTestCompletionProvider(t, server.URL)
	status, err := provider.probeChatCompletions(context.Background())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)

	assert.InDelta(t, 1.0, captured["max_tokens"], 0.0001)
}

func TestCompletionHTTPError_MessageWithoutABody(t *testing.T) {
	err := &completionHTTPError{StatusCode: http.StatusRequestEntityTooLarge}

	assert.Equal(t, "chat completions endpoint returned status 413", err.Error())
}
