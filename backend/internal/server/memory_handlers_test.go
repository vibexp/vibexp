package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibexp/vibexp/internal/config"
)

func TestMemoryHandlers_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	testPath := "/api/v1/" + testTeamID + "/memories"
	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Create Memory - Unauthorized", "POST", testPath, http.StatusUnauthorized},
		{"List Memories - Unauthorized", "GET", testPath, http.StatusUnauthorized},
		{"Get Memory - Unauthorized", "GET", testPath + "/test-id", http.StatusUnauthorized},
		{"Update Memory - Unauthorized", "PUT", testPath + "/test-id", http.StatusUnauthorized},
		{"Delete Memory - Unauthorized", "DELETE", testPath + "/test-id", http.StatusUnauthorized},
		{
			// Auth is enforced before routing, so even a path with no route
			// (metadata search was removed in #524) must 401, not 404.
			"Removed search path - Unauthorized",
			"GET",
			testPath + "/search",
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(`{"text":"test memory"}`)
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

func TestMemoryHandlers_QueryParameters(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	testPath := "/api/v1/" + testTeamID + "/memories"

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{
			"List memories with pagination", "GET", testPath + "?page=1&limit=10",
			http.StatusUnauthorized,
		},
		{
			"List memories with search", "GET", testPath + "?search=test",
			http.StatusUnauthorized,
		},
		{
			"List memories with metadata filter", "GET",
			testPath + `?metadata={"env":["prod"]}`,
			http.StatusUnauthorized,
		},
		{
			"List memories with all filters", "GET",
			testPath + "?page=2&limit=25&search=important",
			http.StatusUnauthorized,
		},
	}

	runQueryParameterTests(t, srv, tests)
}

func runQueryParameterTests(t *testing.T, srv *Server, tests []struct {
	name     string
	method   string
	path     string
	expected int
}) {
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

func TestMemoryHandlers_InvalidPaths(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	testPath := "/api/v1/" + testTeamID + "/memories"

	// These paths match no route, so chi answers from its NotFound/
	// MethodNotAllowed handlers without running the protected group's auth
	// middleware. They used to report 401 only because the domain was mounted as
	// an `r.Route("/api/v1/{team_id}/memories")` prefix subrouter, which matched
	// the whole subtree and therefore ran auth even for paths under it that no
	// route served. #779 registers every memories route at full length (the
	// generated handler uses absolute paths), so the prefix match is gone and the
	// statuses are now the accurate ones. The routes that DO exist still answer
	// 401 unauthenticated — see TestMemoryHandlers_HTTPMethods.
	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Invalid path", "GET", testPath + "/invalid/path", http.StatusNotFound},
		{"Method not allowed", "PATCH", testPath, http.StatusMethodNotAllowed},
		// 401, not 404, since #800: the trailing-slash form of a collection is
		// the collection again, so this reaches the auth middleware exactly as
		// the bare path does. That reachability is the regression #800 fixes.
		{"Collection with a trailing slash", "GET", testPath + "/", http.StatusUnauthorized},
		{"Extra path segments", "GET", testPath + "/test-id/extra", http.StatusNotFound},
		{"Invalid search path", "GET", testPath + "/search/extra", http.StatusNotFound},
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

func TestMemoryHandlers_ContentType(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	testPath := "/api/v1/" + testTeamID + "/memories"

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		expected    int
	}{
		{
			"Create with JSON content type", "POST", testPath,
			`{"text":"test"}`, "application/json", http.StatusUnauthorized,
		},
		{
			"Create without content type", "POST", testPath,
			`{"text":"test"}`, "", http.StatusUnauthorized,
		},
		{
			"Update with JSON content type", "PUT", testPath + "/test-id",
			`{"text":"updated"}`, "application/json", http.StatusUnauthorized,
		},
		{
			"Update without content type", "PUT", testPath + "/test-id",
			`{"text":"updated"}`, "", http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(tt.body)
			req, err := http.NewRequest(tt.method, tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
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

func TestMemoryHandlers_EdgeCases(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	testPath := "/api/v1/" + testTeamID + "/memories"

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		expected int
	}{
		{
			"Create with very long text", "POST", testPath,
			`{"text":"` + strings.Repeat("a", 10000) + `"}`, http.StatusUnauthorized,
		},
		{
			"Create with complex nested metadata", "POST", testPath,
			`{"text":"test","metadata":{"level1":{"level2":{"level3":"deep"}}}}`,
			http.StatusUnauthorized,
		},
		{
			"Create with array metadata", "POST", testPath,
			`{"text":"test","metadata":{"tags":["tag1","tag2","tag3"],"numbers":[1,2,3]}}`,
			http.StatusUnauthorized,
		},
		{
			"Create with null metadata", "POST", testPath,
			`{"text":"test","metadata":null}`, http.StatusUnauthorized,
		},
		{
			"Update with null values", "PUT", testPath + "/test-id",
			`{"text":null,"metadata":null}`, http.StatusUnauthorized,
		},
		{
			"Large payload", "POST", testPath,
			`{"text":"test","metadata":{"large":"` + strings.Repeat("x", 1000) + `"}}`,
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

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestMemoryHandlers_HTTPMethods(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	testPath := "/api/v1/" + testTeamID + "/memories"

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"POST to create endpoint", "POST", testPath, http.StatusUnauthorized},
		{"GET to list endpoint", "GET", testPath, http.StatusUnauthorized},
		{"GET to specific memory", "GET", testPath + "/test-id", http.StatusUnauthorized},
		{"PUT to update endpoint", "PUT", testPath + "/test-id", http.StatusUnauthorized},
		{"DELETE to delete endpoint", "DELETE", testPath + "/test-id", http.StatusUnauthorized},
		{"GET to a removed path", "GET", testPath + "/search", http.StatusUnauthorized},
		// A method chi does not serve on an existing path is answered by its
		// MethodNotAllowed handler, ahead of the group's auth middleware — see
		// the note in TestMemoryHandlers_InvalidPaths. The five real routes above
		// still prove auth is required to reach a handler.
		{"HEAD not allowed on create", "HEAD", testPath, http.StatusMethodNotAllowed},
		{"OPTIONS not configured", "OPTIONS", testPath, http.StatusMethodNotAllowed}, // CORS preflight
		{"PATCH not allowed", "PATCH", testPath + "/test-id", http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader
			if tt.method == "POST" || tt.method == "PUT" {
				body = strings.NewReader(`{"text":"test memory"}`)
			}

			req, err := http.NewRequest(tt.method, tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			if body != nil {
				req.Header.Set("Content-Type", "application/json")
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
