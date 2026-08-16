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

func TestCreateEmbeddingProvider_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Create Provider - Unauthorized", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers", http.StatusUnauthorized},
		{
			"Create Provider via Settings - Unauthorized",
			"POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/settings/embedding-providers",
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyJSON := `{"name":"test-provider","provider_type":"openai",` +
				`"base_url":"https://api.openai.com/v1","api_key":"sk-test"}`
			body := strings.NewReader(bodyJSON)
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

func TestListEmbeddingProviders_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"List Providers - Unauthorized", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers", http.StatusUnauthorized},
		{
			"List Providers via Settings - Unauthorized",
			"GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/settings/embedding-providers",
			http.StatusUnauthorized,
		},
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

func TestGetEmbeddingProvider_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{
			"Get Provider - Unauthorized",
			"GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/provider-123",
			http.StatusUnauthorized,
		},
		{
			"Get Provider via Settings - Unauthorized",
			"GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/settings/embedding-providers/provider-123",
			http.StatusUnauthorized,
		},
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

func TestGetEmbeddingProvider_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		path     string
		expected int
	}{
		// Since #800 a trailing slash is normalised away, so this is the
		// COLLECTION route and answers 401 like any other authenticated path.
		// Before that it matched neither `/embedding-providers` nor
		// `/embedding-providers/{id}` and 404d before auth ran.
		{"Collection with a trailing slash", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/", http.StatusUnauthorized},
		{"Invalid provider ID format", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/invalid-id-format", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.path, nil)
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

func TestUpdateEmbeddingProvider_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{
			"Update Provider - Unauthorized",
			"PUT",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/provider-123",
			http.StatusUnauthorized,
		},
		{
			"Update Provider via Settings - Unauthorized",
			"PUT",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/settings/embedding-providers/provider-123",
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(`{"name":"updated-provider","provider_type":"openai"}`)
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

func TestDeleteEmbeddingProvider_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{
			"Delete Provider - Unauthorized",
			"DELETE",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/provider-123",
			http.StatusUnauthorized,
		},
		{
			"Delete Provider via Settings - Unauthorized",
			"DELETE",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/settings/embedding-providers/provider-123",
			http.StatusUnauthorized,
		},
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

func TestDeleteEmbeddingProvider_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		path     string
		expected int
	}{
		// See TestGetEmbeddingProvider_BadRequest. Since #800 the trailing slash
		// is normalised away, so this is the COLLECTION route -- which has no
		// DELETE, hence 405 rather than the previous pre-auth 404.
		{"Collection with a trailing slash", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/", http.StatusMethodNotAllowed},
		{"Invalid provider ID", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/invalid-id", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("DELETE", tt.path, nil)
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

func TestValidateEmbeddingProvider_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{
			"Validate Provider - Unauthorized",
			"POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/validate",
			http.StatusUnauthorized,
		},
		{
			"Validate Provider via Settings - Unauthorized",
			"POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/settings/embedding-providers/validate",
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyJSON := `{"provider_type":"openai","base_url":"https://api.openai.com/v1","api_key":"sk-test"}`
			body := strings.NewReader(bodyJSON)
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

// TestEmbeddingProviderHandlers_InvalidPaths asserts how the router resolves
// malformed embedding-provider requests. chi settles path and method matching
// BEFORE running a route group's middleware, so an unrouted path is a 404 and a
// wrong method on a real path is a 405 — neither reaches the auth middleware. Only
// a request that matches a registered method+pattern gets as far as returning 401.
//
// Before #649 every case here returned 401, because the resource-usage route group
// mounted `/api/v1` with auth middleware and matched the prefix of anything under
// it. Removing that group let the real statuses surface.
func TestEmbeddingProviderHandlers_InvalidPaths(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		// No registered pattern matches these two extra segments — router 404s before auth.
		{"Invalid provider path", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/invalid/path", http.StatusNotFound},
		{"Invalid settings path", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/settings/embedding-providers/invalid/path", http.StatusNotFound},
		// Path matches but the method does not: /{id} has GET/PUT/DELETE, no POST → 405.
		{"Method not allowed on get", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/provider-123", http.StatusMethodNotAllowed},
		// These reach the auth middleware and are rejected there. Note "validate" is
		// only registered for POST, but a GET still matches the sibling /{id} pattern
		// with id="validate", so it authenticates rather than 405s.
		{"Unauthenticated GET on validate (matches /{id})", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/validate", http.StatusUnauthorized},
		{"Unauthenticated create", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers", http.StatusUnauthorized},
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

type authHeaderTestCase struct {
	name          string
	method        string
	path          string
	body          string
	authorization string
	expected      int
}

func runAuthHeaderTest(t *testing.T, srv *Server, tt authHeaderTestCase) {
	var body io.Reader
	if tt.body != "" {
		body = strings.NewReader(tt.body)
	}

	req, err := http.NewRequest(tt.method, tt.path, body)
	if err != nil {
		t.Fatal(err)
	}
	if tt.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", tt.authorization)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if status := rr.Code; status != tt.expected {
		t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expected)
	}
}

func TestEmbeddingProviderHandlers_WithAuthHeaders(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []authHeaderTestCase{
		{"Create with valid token", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers",
			`{"name":"test","provider_type":"openai","base_url":"https://api.openai.com/v1"}`,
			"Bearer valid-jwt-token", http.StatusUnauthorized},
		{"List with valid token", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers", "",
			"Bearer valid-jwt-token", http.StatusUnauthorized},
		{"Get with valid token", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/provider-123", "",
			"Bearer valid-jwt-token", http.StatusUnauthorized},
		{"Update with valid token", "PUT", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/provider-123",
			`{"name":"updated"}`, "Bearer valid-jwt-token", http.StatusUnauthorized},
		{"Delete with valid token", "DELETE", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/provider-123", "",
			"Bearer valid-jwt-token", http.StatusUnauthorized},
		{"Validate with valid token", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/validate",
			`{"provider_type":"openai","base_url":"https://api.openai.com/v1"}`,
			"Bearer valid-jwt-token", http.StatusUnauthorized},
		{"Create with invalid token", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers",
			`{"name":"test","provider_type":"openai"}`, "Bearer invalid-token", http.StatusUnauthorized},
		{"List with invalid token", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers", "",
			"Bearer invalid-token", http.StatusUnauthorized},
		{"Get with invalid token", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/provider-123", "",
			"Bearer invalid-token", http.StatusUnauthorized},
		{"Update with invalid token", "PUT", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/provider-123",
			`{"name":"updated"}`, "Bearer invalid-token", http.StatusUnauthorized},
		{"Delete with invalid token", "DELETE", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/provider-123", "",
			"Bearer invalid-token", http.StatusUnauthorized},
		{"Validate with invalid token", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/validate",
			`{"provider_type":"openai","base_url":"https://api.openai.com/v1"}`,
			"Bearer invalid-token", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runAuthHeaderTest(t, srv, tt)
		})
	}
}

func TestEmbeddingProviderHandlers_LongName(t *testing.T) {
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
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers",
			`{"name":"` + longName + `","provider_type":"openai","base_url":"https://api.openai.com/v1"}`,
			http.StatusUnauthorized,
		},
		{
			"Create with max length name",
			"POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers",
			`{"name":"` + strings.Repeat("a", 255) +
				`","provider_type":"openai","base_url":"https://api.openai.com/v1"}`,
			http.StatusUnauthorized,
		},
		{
			"Update with name too long",
			"PUT",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/provider-123",
			`{"name":"` + longName + `"}`,
			http.StatusUnauthorized,
		},
		{
			"Update with max length name",
			"PUT",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/provider-123",
			`{"name":"` + strings.Repeat("a", 255) + `"}`,
			http.StatusUnauthorized,
		},
	}

	runLongNameTests(t, srv, tests)
}

func runLongNameTests(t *testing.T, srv *Server, tests []struct {
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

func getURLValidationTestCases() []testCase {
	return []testCase{
		{
			Name: "Create with invalid URL", Method: "POST",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers",
			Body:          `{"name":"test","provider_type":"openai","base_url":"not-a-url"}`,
			Authorization: "Bearer valid-token", Expected: http.StatusUnauthorized,
		},
		{
			Name: "Create with HTTP URL", Method: "POST",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers",
			Body:          `{"name":"test","provider_type":"openai","base_url":"http://api.openai.com/v1"}`,
			Authorization: "Bearer valid-token", Expected: http.StatusUnauthorized,
		},
		{
			Name: "Create with HTTPS URL", Method: "POST",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers",
			Body:          `{"name":"test","provider_type":"openai","base_url":"https://api.openai.com/v1"}`,
			Authorization: "Bearer valid-token", Expected: http.StatusUnauthorized,
		},
		{
			Name: "Update with invalid URL", Method: "PUT",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/provider-123",
			Body:          `{"base_url":"not-a-url"}`,
			Authorization: "Bearer valid-token", Expected: http.StatusUnauthorized,
		},
		{
			Name: "Update with valid URL", Method: "PUT",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/provider-123",
			Body:          `{"base_url":"https://new-api.com/v1"}`,
			Authorization: "Bearer valid-token", Expected: http.StatusUnauthorized,
		},
		{
			Name: "Validate with invalid URL", Method: "POST",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/validate",
			Body:          `{"provider_type":"openai","base_url":"not-a-url"}`,
			Authorization: "Bearer valid-token", Expected: http.StatusUnauthorized,
		},
		{
			Name: "Validate with valid URL", Method: "POST",
			Path:          "/api/v1/550e8400-e29b-41d4-a716-446655440000/embedding-providers/validate",
			Body:          `{"provider_type":"openai","base_url":"https://api.openai.com/v1"}`,
			Authorization: "Bearer valid-token", Expected: http.StatusUnauthorized,
		},
	}
}

func TestEmbeddingProviderHandlers_URLValidation(t *testing.T) {
	srv := testServer()
	tests := getURLValidationTestCases()
	runTestCases(t, srv, tests)
}
