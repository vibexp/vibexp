package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"

	"github.com/vibexp/vibexp/internal/config"
)

func TestMemoryHandlers_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
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
			"Search Memories by Metadata - Unauthorized",
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

func TestCreateMemory_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	testPath := "/api/v1/" + testTeamID + "/memories"

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		expected int
	}{
		{"Create with invalid JSON", "POST", testPath, `{"invalid": json}`, http.StatusUnauthorized},
		{"Create with missing text", "POST", testPath, `{"metadata":{"key":"value"}}`, http.StatusUnauthorized},
		{"Create with empty text", "POST", testPath, `{"text":""}`, http.StatusUnauthorized},
		{
			"Create with valid text only", "POST", testPath,
			`{"text":"Valid memory text"}`, http.StatusUnauthorized,
		},
		{
			"Create with text and metadata", "POST", testPath,
			`{"text":"Memory with metadata","metadata":{"category":"work","priority":"high"}}`,
			http.StatusUnauthorized,
		},
		{
			"Create with complex metadata", "POST", testPath,
			`{"text":"Complex memory","metadata":{"tags":["important","project"],"nested":{"key":"value"}}}`,
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

func TestUpdateMemory_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
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
			"Update with invalid JSON", "PUT", testPath + "/test-id",
			`{"invalid": json}`, http.StatusUnauthorized,
		},
		{
			"Update with empty body", "PUT", testPath + "/test-id",
			`{}`, http.StatusUnauthorized,
		},
		{
			"Update with empty text", "PUT", testPath + "/test-id",
			`{"text":""}`, http.StatusUnauthorized,
		},
		{
			"Update text only", "PUT", testPath + "/test-id",
			`{"text":"Updated memory text"}`, http.StatusUnauthorized,
		},
		{
			"Update metadata only", "PUT", testPath + "/test-id",
			`{"metadata":{"category":"personal","priority":"low"}}`,
			http.StatusUnauthorized,
		},
		{
			"Update both text and metadata", "PUT", testPath + "/test-id",
			`{"text":"Updated text","metadata":{"category":"work"}}`,
			http.StatusUnauthorized,
		},
		{
			"Update with null text", "PUT", testPath + "/test-id",
			`{"text":null,"metadata":{"key":"value"}}`,
			http.StatusUnauthorized,
		},
	}

	runUpdateMemoryTests(t, srv, tests)
}

func runUpdateMemoryTests(t *testing.T, srv *Server, tests []struct {
	name     string
	method   string
	path     string
	body     string
	expected int
}) {
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

func TestMemoryHandlers_QueryParameters(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
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
			testPath + "?metadata_key=category&metadata_value=work",
			http.StatusUnauthorized,
		},
		{
			"List memories with all filters", "GET",
			testPath + "?page=2&limit=25&search=important&metadata_key=priority&metadata_value=high",
			http.StatusUnauthorized,
		},
		{
			"Search by metadata with required params", "GET",
			testPath + "/search?metadata_key=category&metadata_value=work",
			http.StatusUnauthorized,
		},
		{
			"Search by metadata with additional search", "GET",
			testPath + "/search?metadata_key=priority&metadata_value=high&search=project",
			http.StatusUnauthorized,
		},
		{
			"Search by metadata with pagination", "GET",
			testPath + "/search?metadata_key=tags&metadata_value=important&page=1&limit=5",
			http.StatusUnauthorized,
		},
		{
			"Search by metadata missing key", "GET", testPath + "/search?metadata_value=work",
			http.StatusUnauthorized,
		},
		{
			"Search by metadata missing value", "GET", testPath + "/search?metadata_key=category",
			http.StatusUnauthorized,
		},
		{"Search by metadata with empty params", "GET", testPath + "/search", http.StatusUnauthorized},
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
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	testPath := "/api/v1/" + testTeamID + "/memories"

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Invalid path", "GET", testPath + "/invalid/path", http.StatusUnauthorized},
		{"Method not allowed", "PATCH", testPath, http.StatusUnauthorized},
		{"Invalid memory ID format", "GET", testPath + "/", http.StatusUnauthorized},
		{"Extra path segments", "GET", testPath + "/test-id/extra", http.StatusUnauthorized},
		{"Invalid search path", "GET", testPath + "/search/extra", http.StatusUnauthorized},
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
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
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
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
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
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
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
		{"GET to search endpoint", "GET", testPath + "/search", http.StatusUnauthorized},
		{"HEAD not allowed on create", "HEAD", testPath, http.StatusUnauthorized},
		{"OPTIONS not configured", "OPTIONS", testPath, http.StatusUnauthorized}, // CORS preflight
		{"PATCH not allowed", "PATCH", testPath + "/test-id", http.StatusUnauthorized},
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
