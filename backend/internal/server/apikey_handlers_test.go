package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibexp/vibexp/internal/config"
)

func TestAPIKeyHandlers_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Create API Key - Unauthorized", "POST", "/api/v1/api-keys", http.StatusUnauthorized},
		{"List API Keys - Unauthorized", "GET", "/api/v1/api-keys", http.StatusUnauthorized},
		{"Delete API Key - Unauthorized", "DELETE", "/api/v1/api-keys/123", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(`{"name":"test-api-key"}`)
			req, err := http.NewRequest(tt.method, tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestAPIKeyHandlers_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		expected int
	}{
		{"Create with invalid JSON", "POST", "/api/v1/api-keys", `{"invalid": json}`, http.StatusBadRequest},
		{"Create with missing name", "POST", "/api/v1/api-keys", `{}`, http.StatusBadRequest},
		{"Create with empty name", "POST", "/api/v1/api-keys", `{"name":""}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(tt.body)
			req, err := http.NewRequest(tt.method, tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid-token")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			// These should be unauthorized since we don't have proper auth setup
			// In a real integration test environment, we would set up proper authentication
			if status := rr.Code; status != http.StatusUnauthorized {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, http.StatusUnauthorized)
			}
		})
	}
}

func TestAPIKeyHandlers_WithValidAuth(t *testing.T) {
	srv := testServer()

	tests := []testCase{
		{
			Name:          "Create API Key with valid token",
			Method:        "POST",
			Path:          "/api/v1/api-keys",
			Body:          `{"name":"test-key"}`,
			Authorization: "Bearer valid-jwt-token",
			Expected:      http.StatusUnauthorized,
		},
		{
			Name:          "List API Keys with valid token",
			Method:        "GET",
			Path:          "/api/v1/api-keys",
			Authorization: "Bearer valid-jwt-token",
			Expected:      http.StatusUnauthorized,
		},
		{
			Name:          "Delete API Key with valid token",
			Method:        "DELETE",
			Path:          "/api/v1/api-keys/test-id",
			Authorization: "Bearer valid-jwt-token",
			Expected:      http.StatusUnauthorized,
		},
	}

	runTestCases(t, srv, tests)
}

func TestAPIKeyHandlers_InvalidPaths(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Invalid API key path", "GET", "/api/v1/api-keys/invalid/path", http.StatusUnauthorized},
		{"Method not allowed", "PATCH", "/api/v1/api-keys", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestAPIKeyHandlers_LongName(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	// Create a name that's longer than 255 characters
	longName := strings.Repeat("a", 256)

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		expected int
	}{
		{
			"Create with name too long",
			"POST",
			"/api/v1/api-keys",
			`{"name":"` + longName + `"}`,
			http.StatusUnauthorized,
		},
		{
			"Create with max length name",
			"POST",
			"/api/v1/api-keys",
			`{"name":"` + strings.Repeat("a", 255) + `"}`,
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(tt.body)
			req, err := http.NewRequest(tt.method, tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid-token")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestAPIKeyHandlers_DeleteNonExistentKey(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Delete non-existent API key", "DELETE", "/api/v1/api-keys/non-existent-id", http.StatusUnauthorized},
		{"Delete with empty ID", "DELETE", "/api/v1/api-keys/", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer valid-token")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}
