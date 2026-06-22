package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibexp/vibexp/internal/config"
)

// testTeamID is a valid team UUID fixture shared by routing/middleware tests.
const testTeamID = "550e8400-e29b-41d4-a716-446655440000"

// legacyTeamScopedMCPPath is the removed per-team MCP route; it must now 404.
const legacyTeamScopedMCPPath = "/mcp/v1/teams/" + testTeamID + "/common"

func newRoutingTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	return New("8080", nil, "test-api-key", cfg, logger)
}

func TestMCPCommon_Unauthorized(t *testing.T) {
	srv := newRoutingTestServer(t)

	tests := []struct {
		name     string
		method   string
		expected int
	}{
		{"GET without auth", "GET", http.StatusUnauthorized},
		{"POST without auth", "POST", http.StatusUnauthorized},
		{"PUT without auth", "PUT", http.StatusUnauthorized},
		{"DELETE without auth", "DELETE", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(`{"method":"tools/list"}`)
			req, err := http.NewRequest(tt.method, "/mcp/v1/common", body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if rr.Code != tt.expected {
				t.Errorf("got status %v want %v", rr.Code, tt.expected)
			}
		})
	}
}

// TestMCPCommon_LegacyTeamRouteRemoved verifies the old per-team MCP URL is gone.
// It must return 404 (route not registered) rather than 401, since there is no
// route to apply auth middleware to.
func TestMCPCommon_LegacyTeamRouteRemoved(t *testing.T) {
	srv := newRoutingTestServer(t)

	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			body := strings.NewReader(`{"method":"tools/list"}`)
			req, err := http.NewRequest(method, legacyTeamScopedMCPPath, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if rr.Code != http.StatusNotFound {
				t.Errorf("legacy team-scoped MCP route must 404; got %v", rr.Code)
			}
		})
	}
}

func TestMCPCommon_InvalidPaths(t *testing.T) {
	srv := newRoutingTestServer(t)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Invalid MCP version", "POST", "/mcp/v2/common", http.StatusNotFound},
		{"Legacy team-scoped route removed", "POST", legacyTeamScopedMCPPath, http.StatusNotFound},
		{"Missing MCP prefix", "POST", "/v1/common", http.StatusNotFound},
		{"Wrong MCP path", "POST", "/mcp/common", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(`{"method":"tools/list"}`)
			req, err := http.NewRequest(tt.method, tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if rr.Code != tt.expected {
				t.Errorf("got status %v want %v for %s", rr.Code, tt.expected, tt.path)
			}
		})
	}
}

func TestMCPCommon_ContentTypeHandling(t *testing.T) {
	srv := newRoutingTestServer(t)

	tests := []struct {
		name        string
		contentType string
		expected    int
	}{
		{"Valid JSON content type", "application/json", http.StatusUnauthorized},
		{"Missing content type", "", http.StatusUnauthorized},
		{"Wrong content type", "text/plain", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(`{"method":"tools/list"}`)
			req, err := http.NewRequest("POST", "/mcp/v1/common", body)
			if err != nil {
				t.Fatal(err)
			}
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if rr.Code != tt.expected {
				t.Errorf("got status %v want %v", rr.Code, tt.expected)
			}
		})
	}
}
