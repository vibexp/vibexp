package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibexp/vibexp/internal/config"
)

func TestAgentHandlers_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Create Agent - Unauthorized", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/", http.StatusUnauthorized},
		{"List Agents - Unauthorized", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/", http.StatusUnauthorized},
		{"Get Agent Stats - Unauthorized", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/stats", http.StatusUnauthorized},
		{"Get Agent - Unauthorized", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/123", http.StatusUnauthorized},
		{"Update Agent - Unauthorized", "PUT", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/123", http.StatusUnauthorized},
		{"Delete Agent - Unauthorized", "DELETE", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/123", http.StatusUnauthorized},
		{"Start Agent Execution - Unauthorized", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/123/executions", http.StatusUnauthorized},
		{"Complete Agent Execution - Unauthorized", "PUT", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/executions/456", http.StatusUnauthorized},
		{"Get Agent Execution - Unauthorized", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/executions/456", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(`{"name":"test","card_url":"http://localhost:8000/.well-known/agent-card.json"}`)
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

func TestAgentHandlers_QueryParameters(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"List agents with status filter", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/?status=active", http.StatusUnauthorized},
		{"List agents with search", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/?search=test", http.StatusUnauthorized},
		{"List agents with pagination", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/?page=1&limit=5", http.StatusUnauthorized},
		{
			"List agents with all filters",
			"GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/?status=active&search=test&page=2&limit=10",
			http.StatusUnauthorized,
		},
		{"List agents with invalid page", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/?page=0", http.StatusUnauthorized},
		{"List agents with invalid limit", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/?limit=0", http.StatusUnauthorized},
		{"List agents with limit too high", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/?limit=101", http.StatusUnauthorized},
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

func TestAgentHandlers_InvalidPaths(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Invalid agent path", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/invalid/path", http.StatusUnauthorized},
		{"Method not allowed", "PATCH", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/", http.StatusUnauthorized},
		{"Invalid execution path", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/123/executions/invalid", http.StatusUnauthorized},
		{"Method not allowed on execution", "PATCH", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/executions/456", http.StatusUnauthorized},
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

func TestAgentHandlers_ContentTypeValidation(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		expected    int
	}{
		{"Create agent without content type", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/", "", http.StatusUnauthorized},
		{"Create agent with wrong content type", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/", "text/plain", http.StatusUnauthorized},
		{"Update agent without content type", "PUT", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/123", "", http.StatusUnauthorized},
		{"Start execution without content type", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/123/executions", "", http.StatusUnauthorized},
		{"Complete execution without content type", "PUT", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/executions/456", "", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(`{"name":"test"}`)
			req, err := http.NewRequest(tt.method, tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
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

func TestAgentHandlers_AuthorizationHeaders(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name          string
		method        string
		path          string
		authorization string
		expected      int
	}{
		{"Missing authorization header", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/", "", http.StatusUnauthorized},
		{"Invalid Bearer format", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/", "InvalidBearer token", http.StatusUnauthorized},
		{"Missing Bearer prefix", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/", "token-without-bearer-prefix", http.StatusUnauthorized},
		{"Empty Bearer token", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/", "Bearer ", http.StatusUnauthorized},
		{"Invalid token format", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/agents/", "Bearer invalid-token", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
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
