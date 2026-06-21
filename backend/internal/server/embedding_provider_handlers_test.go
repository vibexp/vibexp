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

func TestCreateEmbeddingProvider_Unauthorized(t *testing.T) {
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
		{"Create Provider - Unauthorized", "POST", "/api/v1/embedding-providers", http.StatusUnauthorized},
		{
			"Create Provider via Settings - Unauthorized",
			"POST",
			"/api/v1/settings/embedding-providers",
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

func getCreateEmbeddingProviderBadRequestTests() []testCase {
	const providerPath = "/api/v1/embedding-providers"
	const validAuth = "Bearer valid-token"
	return []testCase{
		{
			Name: "Invalid JSON", Method: "POST", Path: providerPath,
			Body: `{"invalid": json}`, Authorization: validAuth, Expected: http.StatusUnauthorized,
		},
		{
			Name: "Missing name", Method: "POST", Path: providerPath,
			Body: `{"provider_type":"openai"}`, Authorization: validAuth, Expected: http.StatusUnauthorized,
		},
		{
			Name: "Empty name", Method: "POST", Path: providerPath,
			Body:          `{"name":"","provider_type":"openai"}`,
			Authorization: validAuth, Expected: http.StatusUnauthorized,
		},
		{
			Name: "Missing provider_type", Method: "POST", Path: providerPath,
			Body: `{"name":"test-provider"}`, Authorization: validAuth, Expected: http.StatusUnauthorized,
		},
		{
			Name: "Empty provider_type", Method: "POST", Path: providerPath,
			Body:          `{"name":"test-provider","provider_type":""}`,
			Authorization: validAuth, Expected: http.StatusUnauthorized,
		},
		{
			Name: "Valid OpenAI provider", Method: "POST", Path: providerPath,
			Body: `{"name":"test-openai","provider_type":"openai",` +
				`"base_url":"https://api.openai.com/v1","api_key":"sk-test"}`,
			Authorization: validAuth, Expected: http.StatusUnauthorized,
		},
		{
			Name: "Valid Anthropic provider", Method: "POST", Path: providerPath,
			Body: `{"name":"test-anthropic","provider_type":"anthropic",` +
				`"base_url":"https://api.anthropic.com","api_key":"sk-ant-test"}`,
			Authorization: validAuth, Expected: http.StatusUnauthorized,
		},
		{
			Name: "Valid custom provider", Method: "POST", Path: providerPath,
			Body: `{"name":"test-custom","provider_type":"custom",` +
				`"base_url":"https://custom-api.com/v1","api_key":"custom-key"}`,
			Authorization: validAuth, Expected: http.StatusUnauthorized,
		},
		{
			Name: "Provider with configuration", Method: "POST", Path: providerPath,
			Body: `{"name":"test-config","provider_type":"openai",` +
				`"base_url":"https://api.openai.com/v1",` +
				`"api_key":"sk-test","configuration":{"model":"text-embedding-3-small"}}`,
			Authorization: validAuth, Expected: http.StatusUnauthorized,
		},
		{
			Name: "Provider with is_default", Method: "POST", Path: providerPath,
			Body: `{"name":"test-default","provider_type":"openai",` +
				`"base_url":"https://api.openai.com/v1",` +
				`"api_key":"sk-test","is_default":true}`,
			Authorization: validAuth, Expected: http.StatusUnauthorized,
		},
	}
}

func TestCreateEmbeddingProvider_BadRequest(t *testing.T) {
	srv := testServer()
	tests := getCreateEmbeddingProviderBadRequestTests()
	runTestCases(t, srv, tests)
}

func TestListEmbeddingProviders_Unauthorized(t *testing.T) {
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
		{"List Providers - Unauthorized", "GET", "/api/v1/embedding-providers", http.StatusUnauthorized},
		{
			"List Providers via Settings - Unauthorized",
			"GET",
			"/api/v1/settings/embedding-providers",
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
			"Get Provider - Unauthorized",
			"GET",
			"/api/v1/embedding-providers/provider-123",
			http.StatusUnauthorized,
		},
		{
			"Get Provider via Settings - Unauthorized",
			"GET",
			"/api/v1/settings/embedding-providers/provider-123",
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
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{"Empty provider ID", "/api/v1/embedding-providers/", http.StatusUnauthorized},
		{"Invalid provider ID format", "/api/v1/embedding-providers/invalid-id-format", http.StatusUnauthorized},
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
			"Update Provider - Unauthorized",
			"PUT",
			"/api/v1/embedding-providers/provider-123",
			http.StatusUnauthorized,
		},
		{
			"Update Provider via Settings - Unauthorized",
			"PUT",
			"/api/v1/settings/embedding-providers/provider-123",
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

func TestUpdateEmbeddingProvider_BadRequest(t *testing.T) {
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
		{"Empty name update", `{"name":""}`, http.StatusUnauthorized},
		{"Empty provider_type update", `{"provider_type":""}`, http.StatusUnauthorized},
		{"Valid name update", `{"name":"updated-provider"}`, http.StatusUnauthorized},
		{"Valid provider_type update", `{"provider_type":"anthropic"}`, http.StatusUnauthorized},
		{"Valid API key update", `{"api_key":"new-api-key"}`, http.StatusUnauthorized},
		{"Valid base_url update", `{"base_url":"https://new-api.com/v1"}`, http.StatusUnauthorized},
		{
			"Valid configuration update",
			`{"configuration":{"model":"text-embedding-3-large"}}`,
			http.StatusUnauthorized,
		},
		{"Valid is_default update", `{"is_default":true}`, http.StatusUnauthorized},
		{
			"Multiple field update",
			`{"name":"multi-update","provider_type":"custom","is_default":false}`,
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(tt.body)
			req, err := http.NewRequest("PUT", "/api/v1/embedding-providers/provider-123", body)
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

func TestDeleteEmbeddingProvider_Unauthorized(t *testing.T) {
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
			"Delete Provider - Unauthorized",
			"DELETE",
			"/api/v1/embedding-providers/provider-123",
			http.StatusUnauthorized,
		},
		{
			"Delete Provider via Settings - Unauthorized",
			"DELETE",
			"/api/v1/settings/embedding-providers/provider-123",
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
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{"Empty provider ID", "/api/v1/embedding-providers/", http.StatusUnauthorized},
		{"Invalid provider ID", "/api/v1/embedding-providers/invalid-id", http.StatusUnauthorized},
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
			"Validate Provider - Unauthorized",
			"POST",
			"/api/v1/embedding-providers/validate",
			http.StatusUnauthorized,
		},
		{
			"Validate Provider via Settings - Unauthorized",
			"POST",
			"/api/v1/settings/embedding-providers/validate",
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

func TestValidateEmbeddingProvider_BadRequest(t *testing.T) {
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
		{"Missing provider_type", `{"base_url":"https://api.openai.com/v1"}`, http.StatusUnauthorized},
		{
			"Empty provider_type",
			`{"provider_type":"","base_url":"https://api.openai.com/v1"}`,
			http.StatusUnauthorized,
		},
		{"Missing base_url", `{"provider_type":"openai"}`, http.StatusUnauthorized},
		{"Empty base_url", `{"provider_type":"openai","base_url":""}`, http.StatusUnauthorized},
		{
			"Invalid base_url",
			`{"provider_type":"openai","base_url":"not-a-url"}`,
			http.StatusUnauthorized,
		},
		{
			"Valid OpenAI validation",
			`{"provider_type":"openai","base_url":"https://api.openai.com/v1","api_key":"sk-test"}`,
			http.StatusUnauthorized,
		},
		{
			"Valid Anthropic validation",
			`{"provider_type":"anthropic","base_url":"https://api.anthropic.com","api_key":"sk-ant-test"}`,
			http.StatusUnauthorized,
		},
		{
			"Valid without API key",
			`{"provider_type":"openai","base_url":"https://api.openai.com/v1"}`,
			http.StatusUnauthorized,
		},
		{
			"Valid with configuration",
			`{"provider_type":"openai","base_url":"https://api.openai.com/v1",` +
				`"api_key":"sk-test","configuration":{"timeout":30}}`,
			http.StatusUnauthorized,
		},
	}

	runValidateEmbeddingProviderTests(t, srv, tests)
}

func runValidateEmbeddingProviderTests(t *testing.T, srv *Server, tests []struct {
	name     string
	body     string
	expected int
}) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(tt.body)
			req, err := http.NewRequest("POST", "/api/v1/embedding-providers/validate", body)
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

func TestEmbeddingProviderHandlers_InvalidPaths(t *testing.T) {
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
		{"Invalid provider path", "GET", "/api/v1/embedding-providers/invalid/path", http.StatusUnauthorized},
		{"Method not allowed on validate", "GET", "/api/v1/embedding-providers/validate", http.StatusUnauthorized},
		{"Method not allowed on list", "POST", "/api/v1/embedding-providers", http.StatusUnauthorized},
		{"Method not allowed on get", "POST", "/api/v1/embedding-providers/provider-123", http.StatusUnauthorized},
		{"Invalid settings path", "GET", "/api/v1/settings/embedding-providers/invalid/path", http.StatusUnauthorized},
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
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []authHeaderTestCase{
		{"Create with valid token", "POST", "/api/v1/embedding-providers",
			`{"name":"test","provider_type":"openai","base_url":"https://api.openai.com/v1"}`,
			"Bearer valid-jwt-token", http.StatusUnauthorized},
		{"List with valid token", "GET", "/api/v1/embedding-providers", "",
			"Bearer valid-jwt-token", http.StatusUnauthorized},
		{"Get with valid token", "GET", "/api/v1/embedding-providers/provider-123", "",
			"Bearer valid-jwt-token", http.StatusUnauthorized},
		{"Update with valid token", "PUT", "/api/v1/embedding-providers/provider-123",
			`{"name":"updated"}`, "Bearer valid-jwt-token", http.StatusUnauthorized},
		{"Delete with valid token", "DELETE", "/api/v1/embedding-providers/provider-123", "",
			"Bearer valid-jwt-token", http.StatusUnauthorized},
		{"Validate with valid token", "POST", "/api/v1/embedding-providers/validate",
			`{"provider_type":"openai","base_url":"https://api.openai.com/v1"}`,
			"Bearer valid-jwt-token", http.StatusUnauthorized},
		{"Create with invalid token", "POST", "/api/v1/embedding-providers",
			`{"name":"test","provider_type":"openai"}`, "Bearer invalid-token", http.StatusUnauthorized},
		{"List with invalid token", "GET", "/api/v1/embedding-providers", "",
			"Bearer invalid-token", http.StatusUnauthorized},
		{"Get with invalid token", "GET", "/api/v1/embedding-providers/provider-123", "",
			"Bearer invalid-token", http.StatusUnauthorized},
		{"Update with invalid token", "PUT", "/api/v1/embedding-providers/provider-123",
			`{"name":"updated"}`, "Bearer invalid-token", http.StatusUnauthorized},
		{"Delete with invalid token", "DELETE", "/api/v1/embedding-providers/provider-123", "",
			"Bearer invalid-token", http.StatusUnauthorized},
		{"Validate with invalid token", "POST", "/api/v1/embedding-providers/validate",
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
	logger := func() *logrus.Logger { l, _ := test.NewNullLogger(); return l }()
	logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
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
			"/api/v1/embedding-providers",
			`{"name":"` + longName + `","provider_type":"openai","base_url":"https://api.openai.com/v1"}`,
			http.StatusUnauthorized,
		},
		{
			"Create with max length name",
			"POST",
			"/api/v1/embedding-providers",
			`{"name":"` + strings.Repeat("a", 255) +
				`","provider_type":"openai","base_url":"https://api.openai.com/v1"}`,
			http.StatusUnauthorized,
		},
		{
			"Update with name too long",
			"PUT",
			"/api/v1/embedding-providers/provider-123",
			`{"name":"` + longName + `"}`,
			http.StatusUnauthorized,
		},
		{
			"Update with max length name",
			"PUT",
			"/api/v1/embedding-providers/provider-123",
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
			Path:          "/api/v1/embedding-providers",
			Body:          `{"name":"test","provider_type":"openai","base_url":"not-a-url"}`,
			Authorization: "Bearer valid-token", Expected: http.StatusUnauthorized,
		},
		{
			Name: "Create with HTTP URL", Method: "POST",
			Path:          "/api/v1/embedding-providers",
			Body:          `{"name":"test","provider_type":"openai","base_url":"http://api.openai.com/v1"}`,
			Authorization: "Bearer valid-token", Expected: http.StatusUnauthorized,
		},
		{
			Name: "Create with HTTPS URL", Method: "POST",
			Path:          "/api/v1/embedding-providers",
			Body:          `{"name":"test","provider_type":"openai","base_url":"https://api.openai.com/v1"}`,
			Authorization: "Bearer valid-token", Expected: http.StatusUnauthorized,
		},
		{
			Name: "Update with invalid URL", Method: "PUT",
			Path:          "/api/v1/embedding-providers/provider-123",
			Body:          `{"base_url":"not-a-url"}`,
			Authorization: "Bearer valid-token", Expected: http.StatusUnauthorized,
		},
		{
			Name: "Update with valid URL", Method: "PUT",
			Path:          "/api/v1/embedding-providers/provider-123",
			Body:          `{"base_url":"https://new-api.com/v1"}`,
			Authorization: "Bearer valid-token", Expected: http.StatusUnauthorized,
		},
		{
			Name: "Validate with invalid URL", Method: "POST",
			Path:          "/api/v1/embedding-providers/validate",
			Body:          `{"provider_type":"openai","base_url":"not-a-url"}`,
			Authorization: "Bearer valid-token", Expected: http.StatusUnauthorized,
		},
		{
			Name: "Validate with valid URL", Method: "POST",
			Path:          "/api/v1/embedding-providers/validate",
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
