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

func TestAgentHandlers_Unauthorized(t *testing.T) {
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
		{"Create Agent - Unauthorized", "POST", "/api/v1/agents", http.StatusUnauthorized},
		{"List Agents - Unauthorized", "GET", "/api/v1/agents", http.StatusUnauthorized},
		{"Get Agent Stats - Unauthorized", "GET", "/api/v1/agents/stats", http.StatusUnauthorized},
		{"Get Agent - Unauthorized", "GET", "/api/v1/agents/123", http.StatusUnauthorized},
		{"Update Agent - Unauthorized", "PUT", "/api/v1/agents/123", http.StatusUnauthorized},
		{"Delete Agent - Unauthorized", "DELETE", "/api/v1/agents/123", http.StatusUnauthorized},
		{"Start Agent Execution - Unauthorized", "POST", "/api/v1/agents/123/executions", http.StatusUnauthorized},
		{"Complete Agent Execution - Unauthorized", "PUT", "/api/v1/agents/executions/456", http.StatusUnauthorized},
		{"Get Agent Execution - Unauthorized", "GET", "/api/v1/agents/executions/456", http.StatusUnauthorized},
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

func TestCreateAgent_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		body     string
		expected int
	}{
		{"Invalid JSON", `{"invalid": json}`, http.StatusUnauthorized},
		{"Missing card_url", `{"name":"test"}`, http.StatusUnauthorized},
		{"Empty card_url", `{"name":"test","card_url":""}`, http.StatusUnauthorized},
		{
			"Invalid status",
			`{"name":"test","card_url":"http://localhost:8000/.well-known/agent-card.json","status":"invalid"}`,
			http.StatusUnauthorized,
		},
		{
			"Valid status active",
			`{"name":"test","card_url":"http://localhost:8000/.well-known/agent-card.json","status":"active"}`,
			http.StatusUnauthorized,
		},
		{
			"Valid status paused",
			`{"name":"test","card_url":"http://localhost:8000/.well-known/agent-card.json","status":"paused"}`,
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(tt.body)
			req, err := http.NewRequest("POST", "/api/v1/agents", body)
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

func TestUpdateAgent_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		body     string
		expected int
	}{
		{"Invalid JSON", `{"invalid": json}`, http.StatusUnauthorized},
		{"Empty name", `{"name":""}`, http.StatusUnauthorized},
		{"Name too long", `{"name":"` + strings.Repeat("a", 101) + `"}`, http.StatusUnauthorized},
		{"Empty description", `{"description":""}`, http.StatusUnauthorized},
		{"Description too long", `{"description":"` + strings.Repeat("a", 501) + `"}`, http.StatusUnauthorized},
		{"Invalid status", `{"status":"invalid"}`, http.StatusUnauthorized},
		{"Valid status active", `{"status":"active"}`, http.StatusUnauthorized},
		{"Valid status paused", `{"status":"paused"}`, http.StatusUnauthorized},
		{"Valid status error", `{"status":"error"}`, http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(tt.body)
			req, err := http.NewRequest("PUT", "/api/v1/agents/123", body)
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

func TestStartAgentExecution_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		body     string
		expected int
	}{
		{"Invalid JSON", `{"invalid": json}`, http.StatusUnauthorized},
		{"Valid empty body", `{}`, http.StatusUnauthorized},
		{"Valid with input", `{"input":{"key":"value"}}`, http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(tt.body)
			req, err := http.NewRequest("POST", "/api/v1/agents/123/executions", body)
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

func TestCompleteAgentExecution_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		body     string
		expected int
	}{
		{"Invalid JSON", `{"invalid": json}`, http.StatusUnauthorized},
		{"Missing status", `{"output":{"result":"success"}}`, http.StatusUnauthorized},
		{"Invalid status", `{"status":"invalid"}`, http.StatusUnauthorized},
		{"Valid status running", `{"status":"running"}`, http.StatusUnauthorized},
		{"Valid status success", `{"status":"success"}`, http.StatusUnauthorized},
		{"Valid status error", `{"status":"error"}`, http.StatusUnauthorized},
		{"Valid with output", `{"status":"success","output":{"result":"done"}}`, http.StatusUnauthorized},
		{"Valid with error", `{"status":"error","error":"Something went wrong"}`, http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(tt.body)
			req, err := http.NewRequest("PUT", "/api/v1/agents/executions/456", body)
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

func TestAgentHandlers_QueryParameters(t *testing.T) {
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
		{"List agents with status filter", "GET", "/api/v1/agents?status=active", http.StatusUnauthorized},
		{"List agents with search", "GET", "/api/v1/agents?search=test", http.StatusUnauthorized},
		{"List agents with pagination", "GET", "/api/v1/agents?page=1&limit=5", http.StatusUnauthorized},
		{
			"List agents with all filters",
			"GET",
			"/api/v1/agents?status=active&search=test&page=2&limit=10",
			http.StatusUnauthorized,
		},
		{"List agents with invalid page", "GET", "/api/v1/agents?page=0", http.StatusUnauthorized},
		{"List agents with invalid limit", "GET", "/api/v1/agents?limit=0", http.StatusUnauthorized},
		{"List agents with limit too high", "GET", "/api/v1/agents?limit=101", http.StatusUnauthorized},
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
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Invalid agent path", "GET", "/api/v1/agents/invalid/path", http.StatusUnauthorized},
		{"Method not allowed", "PATCH", "/api/v1/agents", http.StatusUnauthorized},
		{"Invalid execution path", "GET", "/api/v1/agents/123/executions/invalid", http.StatusUnauthorized},
		{"Method not allowed on execution", "PATCH", "/api/v1/agents/executions/456", http.StatusUnauthorized},
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
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		expected    int
	}{
		{"Create agent without content type", "POST", "/api/v1/agents", "", http.StatusUnauthorized},
		{"Create agent with wrong content type", "POST", "/api/v1/agents", "text/plain", http.StatusUnauthorized},
		{"Update agent without content type", "PUT", "/api/v1/agents/123", "", http.StatusUnauthorized},
		{"Start execution without content type", "POST", "/api/v1/agents/123/executions", "", http.StatusUnauthorized},
		{"Complete execution without content type", "PUT", "/api/v1/agents/executions/456", "", http.StatusUnauthorized},
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
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name          string
		method        string
		path          string
		authorization string
		expected      int
	}{
		{"Missing authorization header", "GET", "/api/v1/agents", "", http.StatusUnauthorized},
		{"Invalid Bearer format", "GET", "/api/v1/agents", "InvalidBearer token", http.StatusUnauthorized},
		{"Missing Bearer prefix", "GET", "/api/v1/agents", "token-without-bearer-prefix", http.StatusUnauthorized},
		{"Empty Bearer token", "GET", "/api/v1/agents", "Bearer ", http.StatusUnauthorized},
		{"Invalid token format", "GET", "/api/v1/agents", "Bearer invalid-token", http.StatusUnauthorized},
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
