package testutils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/container"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/server"
)

// CreateTestServer creates a test server with all routes configured
// It uses a nil database and creates a container with mocked services
func CreateTestServer(t TestingT, services container.Container) *httptest.Server {
	if t == nil {
		panic("testing interface cannot be nil")
	}
	t.Helper()

	cfg := &config.Config{
		Server: config.ServerConfig{
			CORSAllowedOrigins: []string{"*"},
		},
	}

	// Create a test logger with minimal output
	logger := slog.New(slog.DiscardHandler)

	// Create a server with the provided services container
	srv := server.New("8080", &database.DB{}, "test-api-key", cfg, logger)

	// Create test server
	testServer := httptest.NewServer(srv)
	return testServer
}

// CreateTestServerWithDB creates a test server with a real database connection
func CreateTestServerWithDB(t TestingT, db *database.DB) *httptest.Server {
	if t == nil {
		panic("testing interface cannot be nil")
	}
	if db == nil {
		t.Fatal("database cannot be nil")
		return nil
	}
	t.Helper()

	cfg := &config.Config{
		Server: config.ServerConfig{
			CORSAllowedOrigins: []string{"*"},
		},
	}

	// Create a test logger with minimal output
	logger := slog.New(slog.DiscardHandler)

	srv := server.New("8080", db, "test-api-key", cfg, logger)
	testServer := httptest.NewServer(srv)
	return testServer
}

// NewTestRequest creates a new HTTP request with proper headers
func NewTestRequest(method, url string, body interface{}) (*http.Request, error) {
	if method == "" {
		return nil, fmt.Errorf("method cannot be empty")
	}
	if url == "" {
		return nil, fmt.Errorf("url cannot be empty")
	}

	var bodyReader io.Reader

	if body != nil {
		switch v := body.(type) {
		case string:
			bodyReader = strings.NewReader(v)
		case []byte:
			bodyReader = bytes.NewReader(v)
		case io.Reader:
			bodyReader = v
		default:
			// Marshal to JSON for structs/maps
			jsonData, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal body to JSON: %w", err)
			}
			bodyReader = bytes.NewReader(jsonData)
		}
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set default headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return req, nil
}

// NewAuthenticatedRequest creates a request with JWT token authentication
func NewAuthenticatedRequest(method, url string, body interface{}, userID, email string) (*http.Request, error) {
	req, err := NewTestRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create test request: %w", err)
	}

	token, err := GenerateTestJWT(userID, email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate test JWT: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	return req, nil
}

// NewAPIKeyRequest creates a request with API key authentication
func NewAPIKeyRequest(method, url string, body interface{}, apiKey string) (*http.Request, error) {
	req, err := NewTestRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create test request: %w", err)
	}

	req.Header.Set("X-API-Key", apiKey)
	return req, nil
}

// ExecuteRequest executes an HTTP request against a test server and returns the response recorder
func ExecuteRequest(server *httptest.Server, req *http.Request) *httptest.ResponseRecorder {
	// Update the request URL to use the test server's URL
	if req.URL.Scheme == "" {
		req.URL.Scheme = "http"
	}
	if req.URL.Host == "" {
		req.URL.Host = server.Listener.Addr().String()
	}

	// Create a response recorder
	rr := httptest.NewRecorder()

	// Execute the request
	server.Config.Handler.ServeHTTP(rr, req)

	return rr
}

// ExecuteRequestWithClient executes an HTTP request using the server's client
func ExecuteRequestWithClient(server *httptest.Server, req *http.Request) (*http.Response, error) {
	// Update the request URL to use the test server's URL
	req.URL.Scheme = "http"
	req.URL.Host = server.Listener.Addr().String()

	client := server.Client()
	// #nosec G704 - Test utility function using httptest.Server, not user-controllable URL
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	return resp, nil
}

// AssertHTTPStatusCode validates HTTP status code (renamed to avoid conflicts)
func AssertHTTPStatusCode(t TestingT, response *httptest.ResponseRecorder, expectedStatus int) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()

	if response.Code != expectedStatus {
		bodyStr := ""
		if response.Body != nil {
			bodyStr = response.Body.String()
		}
		t.Errorf("Expected status code %d, got %d. Response body: %s",
			expectedStatus, response.Code, bodyStr)
	}
}

// AssertHTTPJSONResponse validates JSON response structure (renamed to avoid conflicts)
func AssertHTTPJSONResponse(t TestingT, response *httptest.ResponseRecorder, expected interface{}) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()

	// Use the existing AssertJSONResponse from helpers.go
	AssertJSONResponse(t, response, expected)
}

// AssertHTTPErrorResponse validates error response format (renamed to avoid conflicts)
func AssertHTTPErrorResponse(
	t TestingT, response *httptest.ResponseRecorder, expectedStatus int, expectedError string,
) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()

	AssertStatusCode(t, response, expectedStatus)

	var errorResp models.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&errorResp); err != nil {
		t.Fatalf("Failed to decode error response JSON: %v", err)
		return
	}

	if errorResp.Error != expectedError {
		t.Errorf("Expected error message '%s', got '%s'", expectedError, errorResp.Error)
	}
}

// AssertPaginatedResponse validates paginated response structure
func AssertPaginatedResponse(t TestingT, response *httptest.ResponseRecorder, expectedCount int) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()

	AssertHTTPStatusCode(t, response, http.StatusOK)

	var paginatedResp struct {
		TotalCount int `json:"total_count"`
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		TotalPages int `json:"total_pages"`
	}

	if err := json.NewDecoder(response.Body).Decode(&paginatedResp); err != nil {
		t.Fatalf("Failed to decode paginated response JSON: %v", err)
		return
	}

	if paginatedResp.TotalCount != expectedCount {
		t.Errorf("Expected total count %d, got %d", expectedCount, paginatedResp.TotalCount)
	}

	if paginatedResp.Page <= 0 {
		t.Errorf("Expected page to be positive, got %d", paginatedResp.Page)
	}

	if paginatedResp.PerPage <= 0 {
		t.Errorf("Expected per page to be positive, got %d", paginatedResp.PerPage)
	}

	if paginatedResp.TotalPages <= 0 && expectedCount > 0 {
		t.Errorf("Expected total pages to be positive when count > 0, got %d", paginatedResp.TotalPages)
	}
}

// AssertContentType validates the response content type
func AssertContentType(t TestingT, response *httptest.ResponseRecorder, expectedContentType string) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()

	contentType := response.Header().Get("Content-Type")
	if !strings.Contains(contentType, expectedContentType) {
		t.Errorf("Expected content type to contain '%s', got '%s'", expectedContentType, contentType)
	}
}

// AssertHasHeader validates that a response has a specific header
func AssertHasHeader(t TestingT, response *httptest.ResponseRecorder, headerName string) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()

	if response.Header().Get(headerName) == "" {
		t.Errorf("Expected response to have header '%s'", headerName)
	}
}

// AssertHeaderValue validates a specific header value
func AssertHeaderValue(t TestingT, response *httptest.ResponseRecorder, headerName, expectedValue string) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()

	actualValue := response.Header().Get(headerName)
	if actualValue != expectedValue {
		t.Errorf("Expected header '%s' to be '%s', got '%s'", headerName, expectedValue, actualValue)
	}
}

// AssertResponseNotEmpty validates that the response body is not empty
func AssertResponseNotEmpty(t TestingT, response *httptest.ResponseRecorder) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()

	if response.Body == nil || response.Body.Len() == 0 {
		t.Error("Expected response body to not be empty")
	}
}

// AssertValidJSON validates that the response contains valid JSON
func AssertValidJSON(t TestingT, response *httptest.ResponseRecorder) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()

	if response.Body == nil {
		t.Error("Response body is nil")
		return
	}

	var jsonData interface{}
	if err := json.NewDecoder(response.Body).Decode(&jsonData); err != nil {
		t.Errorf("Response does not contain valid JSON: %v", err)
	}
}
