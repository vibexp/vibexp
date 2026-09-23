package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const summaryUserMessage = "<documents>\n<document index=\"1\">\ntitle: a\n---\nbody\n</document>\n" +
	"<document index=\"2\">\ntitle: b\n---\nbody\n</document>\n</documents>\n\nQuestion: q"

func completionBody(t *testing.T, system, user string) string {
	t.Helper()
	body, err := json.Marshal(chatRequest{
		Model: "configured-model",
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	})
	require.NoError(t, err)
	return string(body)
}

func TestHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		key        string
		body       string
		wantStatus int
		wantBody   []string
	}{
		{
			name: "health", method: http.MethodGet, path: "/healthz",
			wantStatus: http.StatusOK,
		},
		{
			name: "models", method: http.MethodGet, path: "/v1/models", key: "e2e-good-key",
			wantStatus: http.StatusOK, wantBody: []string{`"id":"e2e-stub-model"`},
		},
		{
			name: "models with the bad key", method: http.MethodGet, path: "/v1/models", key: badKey,
			wantStatus: http.StatusUnauthorized, wantBody: []string{"invalid api key"},
		},
		{
			name:   "completion cites the first document and echoes style and count",
			method: http.MethodPost, path: "/v1/chat/completions", key: "e2e-good-key",
			body:       completionBody(t, "rules\n\nStyle: be thorough — cover every point", summaryUserMessage),
			wantStatus: http.StatusOK,
			wantBody: []string{
				`"model":"configured-model"`,
				"matching documents [1].",
				"Style: detailed · Documents: 2",
				`"finish_reason":"stop"`,
			},
		},
		{
			name:   "completion with no documents cites nothing",
			method: http.MethodPost, path: "/v1/chat/completions",
			body:       completionBody(t, "Style: be concise — at most three sentences.", "Question: q"),
			wantStatus: http.StatusOK,
			wantBody:   []string{"matching documents.", "Style: concise · Documents: 0"},
		},
		{
			name: "completion with the bad key", method: http.MethodPost, path: "/v1/chat/completions", key: badKey,
			body:       completionBody(t, "", summaryUserMessage),
			wantStatus: http.StatusUnauthorized, wantBody: []string{"invalid api key"},
		},
		{
			name: "completion with a malformed body", method: http.MethodPost, path: "/v1/chat/completions",
			body: "{", wantStatus: http.StatusBadRequest, wantBody: []string{"invalid request body"},
		},
		{
			name: "completion with no messages", method: http.MethodPost, path: "/v1/chat/completions",
			body: `{"model":"m","messages":[]}`, wantStatus: http.StatusBadRequest,
			wantBody: []string{"messages are required"},
		},
	}

	handler := newHandler(0)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.key != "" {
				req.Header.Set("Authorization", "Bearer "+tc.key)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			for _, want := range tc.wantBody {
				assert.Contains(t, rec.Body.String(), want)
			}
		})
	}
}

func TestSummarizeStyles(t *testing.T) {
	for _, tc := range []struct{ system, want string }{
		{"Style: be concise — answer in at most three sentences.", "Style: concise"},
		{"Style: answer in a short paragraph or a few bullet points.", "Style: balanced"},
		{"Style: be thorough — cover every relevant point", "Style: detailed"},
		{"no style at all", "Style: unknown"},
	} {
		got := summarize([]chatMessage{{Role: "system", Content: tc.system}})
		assert.Contains(t, got, tc.want)
	}
}

func TestCompletionDelayHonoursCancellation(t *testing.T) {
	handler := newHandler(time.Hour)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(completionBody(t, "", summaryUserMessage)))
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req.WithContext(ctx))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled request kept waiting out the completion delay")
	}
	assert.NotContains(t, rec.Body.String(), "E2E stub summary")
}
