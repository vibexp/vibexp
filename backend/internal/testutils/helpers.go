package testutils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
)

// Common guard-clause messages and HTTP header names/values shared by the test helpers
const (
	nilTestingInterfaceMsg = "testing interface cannot be nil"
	nilResponseRecorderMsg = "response recorder cannot be nil"
	headerContentType      = "Content-Type"
	contentTypeJSON        = "application/json"
)

// validateTestingParams performs common nil checks for testing functions
func validateTestingParams(t TestingT, response *httptest.ResponseRecorder) bool {
	if t == nil {
		panic(nilTestingInterfaceMsg)
	}
	if response == nil {
		t.Fatal(nilResponseRecorderMsg)
		return false
	}
	return true
}

// CreateTestRequest creates an HTTP request for testing
func CreateTestRequest(method, url string, body interface{}) (*http.Request, error) {
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
			// Assume it's a struct/map that should be JSON encoded
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

	// Set content type for JSON requests
	if body != nil && !isStringOrBytes(body) {
		req.Header.Set(headerContentType, contentTypeJSON)
	}

	return req, nil
}

// CreateTestRequestWithContext creates an HTTP request with context for testing
func CreateTestRequestWithContext(ctx context.Context, method, url string, body interface{}) (*http.Request, error) {
	req, err := CreateTestRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	return req.WithContext(ctx), nil
}

// AssertJSONResponse asserts that the HTTP response matches the expected JSON structure
func AssertJSONResponse(t TestingT, response *httptest.ResponseRecorder, expected interface{}) {
	if t == nil {
		panic(nilTestingInterfaceMsg)
	}
	if response == nil {
		t.Fatal(nilResponseRecorderMsg)
		return
	}
	t.Helper()

	// Check content type
	contentType := response.Header().Get(headerContentType)
	if !strings.Contains(contentType, contentTypeJSON) {
		t.Errorf("Expected JSON content type, got: %s", contentType)
		return
	}

	// Check if response body exists
	if response.Body == nil {
		t.Fatal("response body is nil")
		return
	}

	// Decode response body
	var actual interface{}
	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("Failed to decode response JSON: %v", err)
		return
	}

	// Compare structures
	if !reflect.DeepEqual(actual, expected) {
		// Only marshal JSON for error messages when there's actually a mismatch
		actualJSON, errActual := json.MarshalIndent(actual, "", "  ")
		expectedJSON, errExpected := json.MarshalIndent(expected, "", "  ")
		_ = errActual   // ignore marshal error for test diagnostics
		_ = errExpected // ignore marshal error for test diagnostics
		t.Errorf("Response JSON mismatch.\nExpected:\n%s\nActual:\n%s", expectedJSON, actualJSON)
	}
}

// AssertJSONResponseContains asserts that the HTTP response contains the expected JSON fields
func AssertJSONResponseContains(
	t TestingT, response *httptest.ResponseRecorder, expectedFields map[string]interface{},
) {
	if t == nil {
		panic(nilTestingInterfaceMsg)
	}
	if response == nil {
		t.Fatal(nilResponseRecorderMsg)
		return
	}
	if expectedFields == nil {
		t.Fatal("expected fields map cannot be nil")
		return
	}
	t.Helper()

	// Check content type
	contentType := response.Header().Get(headerContentType)
	if !strings.Contains(contentType, contentTypeJSON) {
		t.Errorf("Expected JSON content type, got: %s", contentType)
		return
	}

	// Check if response body exists
	if response.Body == nil {
		t.Fatal("response body is nil")
		return
	}

	// Decode response body
	var actual map[string]interface{}
	if err := json.NewDecoder(response.Body).Decode(&actual); err != nil {
		t.Fatalf("Failed to decode response JSON: %v", err)
		return
	}

	// Check each expected field
	for key, expectedValue := range expectedFields {
		if actualValue, exists := actual[key]; !exists {
			t.Errorf("Expected field '%s' not found in response", key)
		} else if !reflect.DeepEqual(actualValue, expectedValue) {
			t.Errorf("Field '%s' mismatch. Expected: %v, Actual: %v", key, expectedValue, actualValue)
		}
	}
}

// AssertStatusCode asserts that the HTTP response has the expected status code
func AssertStatusCode(t TestingT, response *httptest.ResponseRecorder, expectedStatus int) {
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

// AssertErrorResponse asserts that the HTTP response is an error with expected message
func AssertErrorResponse(t TestingT, response *httptest.ResponseRecorder, expectedStatus int, expectedError string) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()

	AssertStatusCode(t, response, expectedStatus)

	var errorResp map[string]interface{}
	if err := json.NewDecoder(response.Body).Decode(&errorResp); err != nil {
		t.Fatalf("Failed to decode error response JSON: %v", err)
	}

	if errorMessage, exists := errorResp["error"]; !exists {
		t.Error("Expected 'error' field in error response")
	} else if errorMessage != expectedError {
		t.Errorf("Expected error message '%s', got '%s'", expectedError, errorMessage)
	}
}

// AssertEmptyResponse asserts that the HTTP response body is empty
func AssertEmptyResponse(t TestingT, response *httptest.ResponseRecorder) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()
	if response.Body != nil && response.Body.Len() > 0 {
		t.Errorf("Expected empty response body, got: %s", response.Body.String())
	}
}

// AssertResponseContains asserts that the HTTP response body contains a specific string
func AssertResponseContains(t TestingT, response *httptest.ResponseRecorder, expectedSubstring string) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()
	if response.Body == nil {
		t.Error("Cannot check contents of nil response body")
		return
	}
	body := response.Body.String()
	if !strings.Contains(body, expectedSubstring) {
		t.Errorf("Expected response to contain '%s', but got: %s", expectedSubstring, body)
	}
}

// AssertHeader asserts that the HTTP response has a specific header value
func AssertHeader(t TestingT, response *httptest.ResponseRecorder, headerName, expectedValue string) {
	if !validateTestingParams(t, response) {
		return
	}
	t.Helper()
	actualValue := response.Header().Get(headerName)
	if actualValue != expectedValue {
		t.Errorf("Expected header '%s' to be '%s', got '%s'", headerName, expectedValue, actualValue)
	}
}

// ParseJSONResponse parses the JSON response into the provided interface
func ParseJSONResponse(t TestingT, response *httptest.ResponseRecorder, target interface{}) {
	if !validateTestingParams(t, response) {
		return
	}
	if target == nil {
		t.Fatal("target interface cannot be nil")
		return
	}
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}
}

// CreateMultipartFormRequest creates a multipart form request for testing file uploads
func CreateMultipartFormRequest(
	method, url string, fields map[string]string, files map[string][]byte,
) (*http.Request, error) {
	body := &bytes.Buffer{}
	boundary := "test-boundary-123456789"

	// Write form fields
	for key, value := range fields {
		fmt.Fprintf(body, "--%s\r\n", boundary)
		fmt.Fprintf(body, "Content-Disposition: form-data; name=\"%s\"\r\n\r\n", key)
		body.WriteString(value)
		body.WriteString("\r\n")
	}

	// Write files
	for filename, content := range files {
		fmt.Fprintf(body, "--%s\r\n", boundary)
		fmt.Fprintf(body, "Content-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\n", filename)
		body.WriteString("Content-Type: application/octet-stream\r\n\r\n")
		body.Write(content)
		body.WriteString("\r\n")
	}

	fmt.Fprintf(body, "--%s--\r\n", boundary)

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set(headerContentType, fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
	return req, nil
}

// isStringOrBytes checks if the value is a string or byte slice
func isStringOrBytes(v interface{}) bool {
	switch v.(type) {
	case string, []byte:
		return true
	default:
		return false
	}
}

// CompareJSON compares two JSON strings for equality, ignoring formatting differences
func CompareJSON(t TestingT, expected, actual string) {
	t.Helper()

	// Quick check for exact string match first - avoids unmarshaling if identical
	if expected == actual {
		return
	}

	var expectedObj, actualObj interface{}

	if err := json.Unmarshal([]byte(expected), &expectedObj); err != nil {
		t.Fatalf("Failed to unmarshal expected JSON: %v", err)
	}

	if err := json.Unmarshal([]byte(actual), &actualObj); err != nil {
		t.Fatalf("Failed to unmarshal actual JSON: %v", err)
	}

	if !reflect.DeepEqual(expectedObj, actualObj) {
		expectedFormatted, errExp := json.MarshalIndent(expectedObj, "", "  ")
		actualFormatted, errAct := json.MarshalIndent(actualObj, "", "  ")
		_ = errExp // ignore marshal error for test diagnostics
		_ = errAct // ignore marshal error for test diagnostics
		t.Errorf("JSON mismatch.\nExpected:\n%s\nActual:\n%s", expectedFormatted, actualFormatted)
	}
}
