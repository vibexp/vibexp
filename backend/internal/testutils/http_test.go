package testutils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibexp/vibexp/internal/models"
)

func TestNewTestRequest(t *testing.T) {
	tests := getNewTestRequestTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := NewTestRequest(tt.method, tt.url, tt.body)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			validateHTTPMethod(t, req, tt.expectedMethod)
			validateHTTPURL(t, req, tt.expectedURL)
			validateHTTPHeaders(t, req)
		})
	}
}

// getNewTestRequestTestCases returns test cases for NewTestRequest
func getNewTestRequestTestCases() []struct {
	name           string
	method         string
	url            string
	body           interface{}
	expectError    bool
	expectedMethod string
	expectedURL    string
} {
	return []struct {
		name           string
		method         string
		url            string
		body           interface{}
		expectError    bool
		expectedMethod string
		expectedURL    string
	}{
		{
			name:           "valid GET request",
			method:         "GET",
			url:            "/api/v1/test",
			body:           nil,
			expectError:    false,
			expectedMethod: "GET",
			expectedURL:    "/api/v1/test",
		},
		{
			name:           "valid POST request with JSON body",
			method:         "POST",
			url:            "/api/v1/test",
			body:           map[string]string{"key": "value"},
			expectError:    false,
			expectedMethod: "POST",
			expectedURL:    "/api/v1/test",
		},
		{
			name:           "valid request with string body",
			method:         "PUT",
			url:            "/api/v1/test",
			body:           "test string",
			expectError:    false,
			expectedMethod: "PUT",
			expectedURL:    "/api/v1/test",
		},
		{
			name:        "empty method",
			method:      "",
			url:         "/api/v1/test",
			body:        nil,
			expectError: true,
		},
		{
			name:        "empty URL",
			method:      "GET",
			url:         "",
			body:        nil,
			expectError: true,
		},
	}
}

// validateHTTPMethod validates the HTTP method of a request
func validateHTTPMethod(t *testing.T, req *http.Request, expectedMethod string) {
	t.Helper()
	if req.Method != expectedMethod {
		t.Errorf("Expected method %s, got %s", expectedMethod, req.Method)
	}
}

// validateHTTPURL validates the URL path of a request
func validateHTTPURL(t *testing.T, req *http.Request, expectedURL string) {
	t.Helper()
	if req.URL.Path != expectedURL {
		t.Errorf("Expected URL %s, got %s", expectedURL, req.URL.Path)
	}
}

// validateHTTPHeaders validates HTTP headers for JSON content
func validateHTTPHeaders(t *testing.T, req *http.Request) {
	t.Helper()
	contentType := req.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	accept := req.Header.Get("Accept")
	if accept != "application/json" {
		t.Errorf("Expected Accept application/json, got %s", accept)
	}
}

func TestNewAuthenticatedRequest(t *testing.T) {
	userID := "test-user-123"
	email := "test@example.com"

	req, err := NewAuthenticatedRequest("POST", "/api/v1/test", map[string]string{"key": "value"}, userID, email)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check that Authorization header is set
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		t.Error("Expected Authorization header to be set")
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		t.Errorf("Expected Authorization header to start with 'Bearer ', got %s", authHeader)
	}
}

func TestNewAPIKeyRequest(t *testing.T) {
	apiKey := "test-api-key"

	req, err := NewAPIKeyRequest("GET", "/api/v1/test", nil, apiKey)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check that X-API-Key header is set
	apiKeyHeader := req.Header.Get("X-API-Key")
	if apiKeyHeader != apiKey {
		t.Errorf("Expected X-API-Key header to be %s, got %s", apiKey, apiKeyHeader)
	}
}

func TestAssertHTTPStatusCode(t *testing.T) {
	response := httptest.NewRecorder()
	response.WriteHeader(http.StatusOK)

	// Test successful assertion
	mockT := &MockTestingT{}
	AssertHTTPStatusCode(mockT, response, http.StatusOK)
	if mockT.errorCalled {
		t.Error("Expected no error for correct status code")
	}

	// Test failed assertion
	mockT = &MockTestingT{}
	AssertHTTPStatusCode(mockT, response, http.StatusBadRequest)
	if !mockT.errorCalled {
		t.Error("Expected error for incorrect status code")
	}
}

func TestAssertHTTPErrorResponse(t *testing.T) {
	response := httptest.NewRecorder()
	response.WriteHeader(http.StatusBadRequest)

	errorResp := models.ErrorResponse{
		Error:   "validation_error",
		Message: "Invalid input",
	}

	jsonData, err := json.Marshal(errorResp)
	if err != nil {
		t.Fatalf("Failed to marshal error response: %v", err)
	}
	if _, err := response.Body.Write(jsonData); err != nil {
		t.Logf("Failed to write to response body: %v", err)
	}
	response.Header().Set("Content-Type", "application/json")

	mockT := &MockTestingT{}
	AssertHTTPErrorResponse(mockT, response, http.StatusBadRequest, "validation_error")
	if mockT.errorCalled {
		t.Error("Expected no error for correct error response")
	}
}

func TestAssertPaginatedResponse(t *testing.T) {
	response := httptest.NewRecorder()
	response.WriteHeader(http.StatusOK)

	paginatedResp := struct {
		TotalCount int `json:"total_count"`
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		TotalPages int `json:"total_pages"`
	}{
		TotalCount: 25,
		Page:       1,
		PerPage:    10,
		TotalPages: 3,
	}

	jsonData, err := json.Marshal(paginatedResp)
	if err != nil {
		t.Fatalf("Failed to marshal paginated response: %v", err)
	}
	if _, err := response.Body.Write(jsonData); err != nil {
		t.Logf("Failed to write to response body: %v", err)
	}
	response.Header().Set("Content-Type", "application/json")

	mockT := &MockTestingT{}
	AssertPaginatedResponse(mockT, response, 25)
	if mockT.errorCalled {
		t.Error("Expected no error for correct paginated response")
	}
}

func TestAssertContentType(t *testing.T) {
	response := httptest.NewRecorder()
	response.Header().Set("Content-Type", "application/json; charset=utf-8")

	mockT := &MockTestingT{}
	AssertContentType(mockT, response, "application/json")
	if mockT.errorCalled {
		t.Error("Expected no error for correct content type")
	}

	// Test with wrong content type
	mockT = &MockTestingT{}
	AssertContentType(mockT, response, "text/html")
	if !mockT.errorCalled {
		t.Error("Expected error for incorrect content type")
	}
}

func TestAssertHasHeader(t *testing.T) {
	response := httptest.NewRecorder()
	response.Header().Set("X-Custom-Header", "test-value")

	mockT := &MockTestingT{}
	AssertHasHeader(mockT, response, "X-Custom-Header")
	if mockT.errorCalled {
		t.Error("Expected no error when header is present")
	}

	// Test with missing header
	mockT = &MockTestingT{}
	AssertHasHeader(mockT, response, "X-Missing-Header")
	if !mockT.errorCalled {
		t.Error("Expected error when header is missing")
	}
}

func TestAssertHeaderValue(t *testing.T) {
	response := httptest.NewRecorder()
	response.Header().Set("X-Custom-Header", "expected-value")

	mockT := &MockTestingT{}
	AssertHeaderValue(mockT, response, "X-Custom-Header", "expected-value")
	if mockT.errorCalled {
		t.Error("Expected no error for correct header value")
	}

	// Test with wrong header value
	mockT = &MockTestingT{}
	AssertHeaderValue(mockT, response, "X-Custom-Header", "wrong-value")
	if !mockT.errorCalled {
		t.Error("Expected error for incorrect header value")
	}
}

func TestAssertValidJSON(t *testing.T) {
	// Test with valid JSON
	response := httptest.NewRecorder()
	response.Body.WriteString(`{"key": "value"}`)

	mockT := &MockTestingT{}
	AssertValidJSON(mockT, response)
	if mockT.errorCalled {
		t.Error("Expected no error for valid JSON")
	}

	// Test with invalid JSON
	response = httptest.NewRecorder()
	response.Body.WriteString(`{"key": "value"`)

	mockT = &MockTestingT{}
	AssertValidJSON(mockT, response)
	if !mockT.errorCalled {
		t.Error("Expected error for invalid JSON")
	}
}

func TestAssertResponseNotEmpty(t *testing.T) {
	// Test with non-empty response
	response := httptest.NewRecorder()
	response.Body.WriteString("test content")

	mockT := &MockTestingT{}
	AssertResponseNotEmpty(mockT, response)
	if mockT.errorCalled {
		t.Error("Expected no error for non-empty response")
	}

	// Test with empty response
	response = httptest.NewRecorder()

	mockT = &MockTestingT{}
	AssertResponseNotEmpty(mockT, response)
	if !mockT.errorCalled {
		t.Error("Expected error for empty response")
	}
}

// MockTestingT is a mock implementation of TestingT for testing
type MockTestingT struct {
	errorCalled  bool
	fatalCalled  bool
	helperCalled bool
}

func (m *MockTestingT) Errorf(format string, args ...interface{}) {
	m.errorCalled = true
}

func (m *MockTestingT) Error(args ...interface{}) {
	m.errorCalled = true
}

func (m *MockTestingT) Fatal(args ...interface{}) {
	m.fatalCalled = true
}

func (m *MockTestingT) Fatalf(format string, args ...interface{}) {
	m.fatalCalled = true
}

func (m *MockTestingT) Helper() {
	m.helperCalled = true
}

func (m *MockTestingT) Run(name string, f func(TestingT)) bool {
	f(m)
	return !m.errorCalled && !m.fatalCalled
}
