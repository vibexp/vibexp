package testutils

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateTestRequest(t *testing.T) {
	tests := getCreateTestRequestTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := CreateTestRequest(tt.method, tt.url, tt.body)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			validateRequestMethod(t, req, tt.expectedMethod)
			validateRequestURL(t, req, tt.expectedURL)
			validateRequestBody(t, req, tt.expectBody)
			validateRequestJSON(t, req, tt.expectJSON)
		})
	}
}

// getCreateTestRequestTestCases returns test cases for CreateTestRequest
func getCreateTestRequestTestCases() []struct {
	name           string
	method         string
	url            string
	body           interface{}
	expectedMethod string
	expectedURL    string
	expectBody     bool
	expectJSON     bool
} {
	return []struct {
		name           string
		method         string
		url            string
		body           interface{}
		expectedMethod string
		expectedURL    string
		expectBody     bool
		expectJSON     bool
	}{
		{
			name:           "GET request with no body",
			method:         "GET",
			url:            "/api/v1/test",
			body:           nil,
			expectedMethod: "GET",
			expectedURL:    "/api/v1/test",
			expectBody:     false,
			expectJSON:     false,
		},
		{
			name:           "POST request with string body",
			method:         "POST",
			url:            "/api/v1/test",
			body:           "test content",
			expectedMethod: "POST",
			expectedURL:    "/api/v1/test",
			expectBody:     true,
			expectJSON:     false,
		},
		{
			name:           "POST request with JSON body",
			method:         "POST",
			url:            "/api/v1/test",
			body:           map[string]string{"name": "test"},
			expectedMethod: "POST",
			expectedURL:    "/api/v1/test",
			expectBody:     true,
			expectJSON:     true,
		},
		{
			name:           "PUT request with byte body",
			method:         "PUT",
			url:            "/api/v1/test",
			body:           []byte("test bytes"),
			expectedMethod: "PUT",
			expectedURL:    "/api/v1/test",
			expectBody:     true,
			expectJSON:     false,
		},
	}
}

// validateRequestMethod validates the HTTP method of a request
func validateRequestMethod(t *testing.T, req *http.Request, expectedMethod string) {
	t.Helper()
	if req.Method != expectedMethod {
		t.Errorf("Expected method '%s', got '%s'", expectedMethod, req.Method)
	}
}

// validateRequestURL validates the URL path of a request
func validateRequestURL(t *testing.T, req *http.Request, expectedURL string) {
	t.Helper()
	if req.URL.Path != expectedURL {
		t.Errorf("Expected URL '%s', got '%s'", expectedURL, req.URL.Path)
	}
}

// validateRequestBody validates whether request body is set as expected
func validateRequestBody(t *testing.T, req *http.Request, expectBody bool) {
	t.Helper()
	if expectBody {
		if req.Body == nil {
			t.Error("Expected request body to be set")
		}
	} else {
		if req.Body != nil {
			t.Error("Expected request body to be nil")
		}
	}
}

// validateRequestJSON validates JSON content type header
func validateRequestJSON(t *testing.T, req *http.Request, expectJSON bool) {
	t.Helper()
	if expectJSON {
		contentType := req.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
		}
	}
}

func TestCreateTestRequestWithContext(t *testing.T) {
	// Define a test-specific context key type
	type testContextKey string
	testKey := testContextKey("test")
	ctx := context.WithValue(context.Background(), testKey, "value")
	req, err := CreateTestRequestWithContext(ctx, "GET", "/test", nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if req.Context() != ctx {
		t.Error("Expected request to have the provided context")
	}

	// Verify context value is preserved
	if req.Context().Value(testKey) != "value" {
		t.Error("Expected context value to be preserved")
	}
}

func TestAssertStatusCode(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusOK)

	// This should pass
	AssertStatusCode(t, recorder, http.StatusOK)

	// Test failure case with a mock testing.T
	mockT := &mockTesting{}
	recorder2 := httptest.NewRecorder()
	recorder2.WriteHeader(http.StatusBadRequest)
	AssertStatusCode(mockT, recorder2, http.StatusOK)

	if !mockT.errorFCalled {
		t.Error("Expected AssertStatusCode to call Error on test failure")
	}
}

func TestAssertJSONResponse(t *testing.T) {
	tests := []struct {
		name       string
		response   interface{}
		expected   interface{}
		shouldFail bool
	}{
		{
			name:       "matching simple objects",
			response:   map[string]interface{}{"id": "123", "name": "test"},
			expected:   map[string]interface{}{"id": "123", "name": "test"},
			shouldFail: false,
		},
		{
			name:       "non-matching objects",
			response:   map[string]interface{}{"id": "123", "name": "test"},
			expected:   map[string]interface{}{"id": "456", "name": "test"},
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(recorder).Encode(tt.response); err != nil {
				t.Fatalf("Failed to encode response: %v", err)
			}

			mockT := &mockTesting{}
			AssertJSONResponse(mockT, recorder, tt.expected)

			if tt.shouldFail && !mockT.errorFCalled {
				t.Error("Expected assertion to fail but it didn't")
			}
			if !tt.shouldFail && mockT.errorFCalled {
				t.Error("Expected assertion to pass but it failed")
			}
		})
	}
}

func TestAssertJSONResponseContains(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/json")

	response := map[string]interface{}{
		"id":     "123",
		"name":   "test",
		"status": "active",
		"count":  42,
	}
	if err := json.NewEncoder(recorder).Encode(response); err != nil {
		t.Fatalf("Failed to encode response: %v", err)
	}

	// Test successful assertion
	expectedFields := map[string]interface{}{
		"id":   "123",
		"name": "test",
	}
	AssertJSONResponseContains(t, recorder, expectedFields)

	// Test failure case
	mockT := &mockTesting{}
	recorder2 := httptest.NewRecorder()
	recorder2.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(recorder2).Encode(response); err != nil {
		t.Fatalf("Failed to encode response: %v", err)
	}

	wrongFields := map[string]interface{}{
		"id":   "456", // Wrong value
		"name": "test",
	}
	AssertJSONResponseContains(mockT, recorder2, wrongFields)

	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for wrong field value")
	}
}

func TestAssertErrorResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusBadRequest)
	recorder.Header().Set("Content-Type", "application/json")

	errorResponse := map[string]interface{}{
		"error":   "invalid_input",
		"message": "The provided input is invalid",
	}
	if err := json.NewEncoder(recorder).Encode(errorResponse); err != nil {
		t.Fatalf("Failed to encode error response: %v", err)
	}

	// This should pass
	AssertErrorResponse(t, recorder, http.StatusBadRequest, "invalid_input")

	// Test failure case
	mockT := &mockTesting{}
	recorder2 := httptest.NewRecorder()
	recorder2.WriteHeader(http.StatusBadRequest)
	recorder2.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(recorder2).Encode(errorResponse); err != nil {
		t.Fatalf("Failed to encode error response: %v", err)
	}

	AssertErrorResponse(mockT, recorder2, http.StatusBadRequest, "wrong_error")

	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for wrong error message")
	}
}

func TestAssertEmptyResponse(t *testing.T) {
	// Test empty response
	recorder := httptest.NewRecorder()
	AssertEmptyResponse(t, recorder)

	// Test non-empty response
	mockT := &mockTesting{}
	recorder2 := httptest.NewRecorder()
	if _, err := recorder2.WriteString("not empty"); err != nil {
		t.Logf("Failed to write string: %v", err)
	}
	AssertEmptyResponse(mockT, recorder2)

	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for non-empty response")
	}
}

func TestAssertResponseContains(t *testing.T) {
	recorder := httptest.NewRecorder()
	if _, err := recorder.WriteString("This is a test response with some content"); err != nil {
		t.Fatalf("Failed to write string: %v", err)
	}

	// This should pass
	AssertResponseContains(t, recorder, "test response")

	// Test failure case
	mockT := &mockTesting{}
	recorder2 := httptest.NewRecorder()
	if _, err := recorder2.WriteString("This is a test response"); err != nil {
		t.Logf("Failed to write string: %v", err)
	}
	AssertResponseContains(mockT, recorder2, "not found")

	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for missing substring")
	}
}

func TestAssertHeader(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.Header().Set("X-Test-Header", "test-value")

	// This should pass
	AssertHeader(t, recorder, "X-Test-Header", "test-value")

	// Test failure case
	mockT := &mockTesting{}
	AssertHeader(mockT, recorder, "X-Test-Header", "wrong-value")

	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for wrong header value")
	}
}

func TestParseJSONResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/json")

	originalData := map[string]interface{}{
		"id":   "123",
		"name": "test",
	}
	if err := json.NewEncoder(recorder).Encode(originalData); err != nil {
		t.Fatalf("Failed to encode original data: %v", err)
	}

	var parsedData map[string]interface{}
	ParseJSONResponse(t, recorder, &parsedData)

	if parsedData["id"] != "123" {
		t.Errorf("Expected id '123', got %v", parsedData["id"])
	}

	if parsedData["name"] != "test" {
		t.Errorf("Expected name 'test', got %v", parsedData["name"])
	}
}

func TestCreateMultipartFormRequest(t *testing.T) {
	fields := map[string]string{
		"name":        "test",
		"description": "test description",
	}
	files := map[string][]byte{
		"test.txt": []byte("test file content"),
	}

	req, err := CreateMultipartFormRequest("POST", "/upload", fields, files)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if req.Method != "POST" {
		t.Errorf("Expected method 'POST', got '%s'", req.Method)
	}

	if req.URL.Path != "/upload" {
		t.Errorf("Expected path '/upload', got '%s'", req.URL.Path)
	}

	contentType := req.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		t.Errorf("Expected multipart content type, got '%s'", contentType)
	}

	if req.Body == nil {
		t.Fatal("Expected request body to be set")
	}
}

func TestCompareJSON(t *testing.T) {
	json1 := `{"id": "123", "name": "test"}`
	json2 := `{
		"name": "test",
		"id": "123"
	}`

	// These should be equal despite different formatting
	CompareJSON(t, json1, json2)

	// Test failure case
	mockT := &mockTesting{}
	json3 := `{"id": "456", "name": "test"}`
	CompareJSON(mockT, json1, json3)

	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for different JSON content")
	}
}

// Mock testing.T for testing test helpers
type mockTesting struct {
	errorFCalled bool
	fatalCalled  bool
}

func (m *mockTesting) Errorf(format string, args ...interface{}) {
	m.errorFCalled = true
}

func (m *mockTesting) Fatalf(format string, args ...interface{}) {
	m.fatalCalled = true
}

func (m *mockTesting) Helper() {}

func (m *mockTesting) Error(args ...interface{}) {
	m.errorFCalled = true
}

func (m *mockTesting) Fatal(args ...interface{}) {
	m.fatalCalled = true
}

// Ensure mockTesting implements TestingT
var _ TestingT = (*mockTesting)(nil)
