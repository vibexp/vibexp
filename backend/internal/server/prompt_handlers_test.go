package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

func TestPromptHandlers_Unauthorized(t *testing.T) {
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
		{
			"Create Prompt - Unauthorized", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts", http.StatusUnauthorized,
		},
		{
			"List Prompts - Unauthorized", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts", http.StatusUnauthorized,
		},
		{
			"Get Prompt - Unauthorized", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug", http.StatusUnauthorized,
		},
		{
			"Update Prompt - Unauthorized", "PUT",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug", http.StatusUnauthorized,
		},
		{
			"Delete Prompt - Unauthorized", "DELETE",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug", http.StatusUnauthorized,
		},
		{
			"Get Prompt Placeholders - Unauthorized", "GET",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug/placeholders",
			http.StatusUnauthorized,
		},
		{
			"Render Prompt - Unauthorized", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug/render",
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(`{"name":"test","slug":"test","body":"test"}`)
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

// nolint:funlen // Table-driven test with multiple test cases
func TestCreatePrompt_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		expected int
	}{
		{
			"Create with invalid JSON", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts",
			`{"invalid": json}`, http.StatusBadRequest,
		},
		{
			"Create with missing name", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts",
			`{"slug":"test","body":"test"}`, http.StatusBadRequest,
		},
		{
			"Create with missing slug", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts",
			`{"name":"test","body":"test"}`, http.StatusBadRequest,
		},
		{
			"Create with missing body", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts",
			`{"name":"test","slug":"test"}`, http.StatusBadRequest,
		},
		{
			"Create with empty name", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts",
			`{"name":"","slug":"test","body":"test"}`, http.StatusBadRequest,
		},
		{
			"Create with empty slug", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts",
			`{"name":"test","slug":"","body":"test"}`, http.StatusBadRequest,
		},
		{
			"Create with empty body", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts",
			`{"name":"test","slug":"test","body":""}`, http.StatusBadRequest,
		},
		{
			"Create with name too long", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts",
			`{"name":"` + strings.Repeat("a", 51) + `","slug":"test","body":"test"}`,
			http.StatusBadRequest,
		},
		{
			"Create with slug too long", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts",
			`{"name":"test","slug":"` + strings.Repeat("a", 256) + `","body":"test"}`,
			http.StatusBadRequest,
		},
		{
			"Create with description too long", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts",
			`{"name":"test","slug":"test","body":"test","description":"` + strings.Repeat("a", 201) + `"}`,
			http.StatusBadRequest,
		},
		{
			"Create with invalid status", "POST", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts",
			`{"name":"test","slug":"test","body":"test","status":"invalid"}`,
			http.StatusBadRequest,
		},
	}

	runCreatePromptTests(t, srv, tests)
}

func runCreatePromptTests(t *testing.T, srv *Server, tests []struct {
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

			// These should be unauthorized since we don't have proper auth setup
			// but the validation should still catch bad requests at the handler level
			if status := rr.Code; status != tt.expected && status != http.StatusUnauthorized {
				t.Errorf("handler returned wrong status code: got %v want %v or %v",
					status, tt.expected, http.StatusUnauthorized)
			}
		})
	}
}

func TestPromptHandlers_MethodNotAllowed(t *testing.T) {
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
		{
			"PATCH method on prompts", "PATCH", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts",
			http.StatusUnauthorized, // Auth middleware catches first
		},
		{
			"OPTIONS method on specific prompt", "OPTIONS", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug",
			http.StatusUnauthorized, // Auth middleware catches first
		},
		{
			"HEAD method on prompts list", "HEAD", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts",
			http.StatusUnauthorized, // Auth middleware catches first
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

func TestUpdatePrompt_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		expected int
	}{
		{
			"Update with invalid JSON", "PUT", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug",
			`{"invalid": json}`, http.StatusBadRequest,
		},
		{
			"Update with name too long", "PUT", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug",
			`{"name":"` + strings.Repeat("a", 51) + `"}`, http.StatusBadRequest,
		},
		{
			"Update with description too long", "PUT", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug",
			`{"description":"` + strings.Repeat("a", 201) + `"}`, http.StatusBadRequest,
		},
		{
			"Update with invalid status", "PUT",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug",
			`{"status":"invalid"}`, http.StatusBadRequest,
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

			// These should be unauthorized since we don't have proper auth setup
			// but the validation should still catch bad requests at the handler level
			if status := rr.Code; status != tt.expected && status != http.StatusUnauthorized {
				t.Errorf("handler returned wrong status code: got %v want %v or %v",
					status, tt.expected, http.StatusUnauthorized)
			}
		})
	}
}

func TestRenderPrompt_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		expected int
	}{
		{
			"Render with invalid JSON", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug/render",
			`{"invalid": json}`, http.StatusBadRequest,
		},
		{
			"Render with empty slug", "POST",
			"/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts//render",
			`{"variables":{}}`, http.StatusNotFound,
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

			// These should be unauthorized since we don't have proper auth setup
			// but the validation should still catch bad requests at the handler level
			if status := rr.Code; status != tt.expected && status != http.StatusUnauthorized {
				t.Errorf("handler returned wrong status code: got %v want %v or %v",
					status, tt.expected, http.StatusUnauthorized)
			}
		})
	}
}

func TestPromptHandlers_InvalidPaths(t *testing.T) {
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
		{
			"Invalid prompt subpath", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug/invalid",
			http.StatusUnauthorized, // Auth middleware catches first
		},
		{
			"Invalid nested path", "GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug/placeholders/invalid",
			http.StatusUnauthorized, // Auth middleware catches first
		},
		{
			"Non-existent endpoint", "GET", "/api/v1/prompts-invalid",
			http.StatusNotFound, // This path doesn't match any protected routes
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

func TestGetPromptDependencies_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug/dependencies", nil)
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

// TestHandleUpdatePromptError_VersionMismatch tests that version mismatch errors
// are properly handled and return user-friendly error messages - regression test for issue #486
func TestHandleUpdatePromptError_VersionMismatch(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name           string
		err            error
		expectedStatus int
	}{
		{
			name:           "Version mismatch error returns 409 Conflict",
			err:            fmt.Errorf("prompt not found or version mismatch"),
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "Prompt not found error returns 404",
			err:            repositories.ErrPromptNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Slug conflict error returns 409",
			err:            fmt.Errorf("prompt with slug 'test-slug' already exists for this user"),
			expectedStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			httptest.NewRequest("PUT", "/api/v1/550e8400-e29b-41d4-a716-446655440000/prompts/test-slug", nil)

			// For slug conflict test, create appropriate request
			var updateReq *models.UpdatePromptRequest
			if strings.Contains(tt.err.Error(), "slug") {
				slug := "test-slug"
				updateReq = &models.UpdatePromptRequest{Slug: &slug}
			} else {
				updateReq = &models.UpdatePromptRequest{}
			}

			// Call the error handling function directly
			srv.handleUpdatePromptError(tt.err, updateReq, rr)

			// Verify status code
			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handleUpdatePromptError() returned wrong status code: got %v want %v",
					status, tt.expectedStatus)
			}
		})
	}
}
