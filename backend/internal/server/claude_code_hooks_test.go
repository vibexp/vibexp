package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"

	"github.com/vibexp/vibexp/internal/config"
)

func TestClaudeCodeHooksHandlers_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Create Hook - Unauthorized", "POST", "/api/v1/claude-code/hooks", http.StatusUnauthorized},
		{"List Hooks - Unauthorized", "GET", "/api/v1/ai-tools/claude-code/hooks", http.StatusUnauthorized},
		{"List Sessions - Unauthorized", "GET", "/api/v1/ai-tools/claude-code/sessions", http.StatusUnauthorized},
		{"Get Session Counts - Unauthorized", "GET", "/api/v1/ai-tools/claude-code/session-counts", http.StatusUnauthorized},
		{"Get Overview Stats - Unauthorized", "GET",
			"/api/v1/ai-tools/claude-code/overview-stats", http.StatusUnauthorized},
		{"Get Recent Activities - Unauthorized", "GET",
			"/api/v1/ai-tools/claude-code/recent-activities", http.StatusUnauthorized},
		{"Delete Session - Unauthorized", "DELETE",
			"/api/v1/ai-tools/claude-code/sessions/test-session-123", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *strings.Reader
			if tt.method == "POST" {
				body = strings.NewReader(`{"session_id":"test-session","hook_event_name":"UserPromptSubmit"}`)
			} else {
				body = strings.NewReader("")
			}

			req, err := http.NewRequest(tt.method, tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v, body: %s",
					status, tt.expected, rr.Body.String())
			}
		})
	}
}

// TestClaudeCodeHooksUserIsolation_AuthenticationRequired tests that all endpoints require authentication
func TestClaudeCodeHooksUserIsolation_AuthenticationRequired(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	// Test endpoints that must require authentication (cannot access without user context)
	testCases := []struct {
		name   string
		method string
		path   string
	}{
		{"POST hook endpoint", "POST", "/api/v1/claude-code/hooks"},
		{"GET hooks endpoint", "GET", "/api/v1/ai-tools/claude-code/hooks"},
		{"GET sessions endpoint", "GET", "/api/v1/ai-tools/claude-code/sessions"},
		{"GET session counts endpoint", "GET", "/api/v1/ai-tools/claude-code/session-counts"},
		{"GET overview stats endpoint", "GET", "/api/v1/ai-tools/claude-code/overview-stats"},
		{"GET recent activities endpoint", "GET", "/api/v1/ai-tools/claude-code/recent-activities"},
		{"DELETE session endpoint", "DELETE",
			"/api/v1/ai-tools/claude-code/sessions/test-session-123"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var body *strings.Reader
			if tc.method == "POST" {
				body = strings.NewReader(`{"session_id":"test-session","hook_event_name":"UserPromptSubmit"}`)
			} else {
				body = strings.NewReader("")
			}

			req, err := http.NewRequest(tc.method, tc.path, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			// All endpoints should return 401 Unauthorized when no authentication is provided
			if rr.Code != http.StatusUnauthorized {
				t.Errorf("Expected 401 Unauthorized for %s %s, got %d", tc.method, tc.path, rr.Code)
			}
		})
	}
}
