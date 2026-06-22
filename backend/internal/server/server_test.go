package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibexp/vibexp/internal/config"
)

func TestHandlePing(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/ping", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	expected := "pong"
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

type healthResponse struct {
	Status string `json:"status"`
	SHA    string `json:"sha"`
}

func callHealthEndpoint(t *testing.T, releaseSHA string) (int, healthResponse) {
	t.Helper()
	cfg := &config.Config{ReleaseSHA: releaseSHA}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	var body healthResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	return rr.Code, body
}

func TestHandleHealth(t *testing.T) {
	tests := []struct {
		name       string
		releaseSHA string
		wantSHA    string
	}{
		{name: "long SHA truncated to 8 chars", releaseSHA: "abc1234ef0123456", wantSHA: "abc1234e"},
		{name: "short SHA not truncated", releaseSHA: "abc123", wantSHA: "abc123"},
		{name: "exactly 8 chars SHA unchanged", releaseSHA: "abc1234e", wantSHA: "abc1234e"},
		{name: "empty SHA passes through", releaseSHA: "", wantSHA: ""},
		{name: "dev default SHA passes through", releaseSHA: "dev", wantSHA: "dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := callHealthEndpoint(t, tt.releaseSHA)

			if code != http.StatusOK {
				t.Errorf("status code: got %v want %v", code, http.StatusOK)
			}
			if body.Status != "healthy" {
				t.Errorf("status field: got %q want %q", body.Status, "healthy")
			}
			if body.SHA != tt.wantSHA {
				t.Errorf("sha field: got %q want %q", body.SHA, tt.wantSHA)
			}
		})
	}
}

func TestProtectedEndpointUnauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/api/v1/ai-tools/claude-code/hooks", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusUnauthorized {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusUnauthorized)
	}
}

func TestLegacyGitHubWebhookPathRedirect(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("POST", "/api/v1/integrations/github/webhook", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusPermanentRedirect {
		t.Errorf("legacy webhook path should redirect: got %v want %v",
			status, http.StatusPermanentRedirect)
	}

	location := rr.Header().Get("Location")
	expected := "/api/v1/webhooks/github"
	if location != expected {
		t.Errorf("redirect location wrong: got %v want %v", location, expected)
	}
}
