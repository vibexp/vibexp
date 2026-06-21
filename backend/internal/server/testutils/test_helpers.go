package testutils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// HTTPTestCase represents a single HTTP test case
type HTTPTestCase struct {
	Name           string
	Method         string
	URL            string
	Body           interface{}
	Headers        map[string]string
	ExpectedStatus int
	ExpectedBody   interface{}
	Setup          func(*TestServer)
	Cleanup        func(*TestServer)
}

// MakeRequest creates and executes an HTTP request
func MakeRequest(
	ts *TestServer, method, path string, body interface{}, headers map[string]string,
) *httptest.ResponseRecorder {
	var reqBody io.Reader

	if body != nil {
		switch v := body.(type) {
		case string:
			reqBody = strings.NewReader(v)
		case []byte:
			reqBody = bytes.NewReader(v)
		default:
			jsonBody, err := json.Marshal(v)
			if err != nil {
				panic(fmt.Sprintf("failed to marshal request body: %v", err))
			}
			reqBody = bytes.NewReader(jsonBody)
		}
	}

	req := httptest.NewRequest(method, path, reqBody)

	// Set default Content-Type for POST/PUT requests with body
	if body != nil && (method == "POST" || method == "PUT") {
		req.Header.Set("Content-Type", "application/json")
	}

	// Set custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	rr := httptest.NewRecorder()
	ts.Server.ServeHTTP(rr, req)

	return rr
}

// AssertJSONResponse asserts that the response has the expected status and JSON body
func AssertJSONResponse(t *testing.T, rr *httptest.ResponseRecorder, expectedStatus int, expectedBody interface{}) {
	assert.Equal(t, expectedStatus, rr.Code, "Status code mismatch")

	if expectedBody != nil {
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"), "Content-Type should be application/json")

		var actualBody interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &actualBody)
		require.NoError(t, err, "Response body should be valid JSON")

		expectedJSON, err := json.Marshal(expectedBody)
		require.NoError(t, err, "Expected body should be marshallable to JSON")

		var expectedBodyParsed interface{}
		err = json.Unmarshal(expectedJSON, &expectedBodyParsed)
		require.NoError(t, err, "Expected body should be valid JSON")

		assert.Equal(t, expectedBodyParsed, actualBody, "Response body mismatch")
	}
}

// AssertTextResponse asserts that the response has the expected status and text body
func AssertTextResponse(t *testing.T, rr *httptest.ResponseRecorder, expectedStatus int, expectedBody string) {
	assert.Equal(t, expectedStatus, rr.Code, "Status code mismatch")
	assert.Equal(t, expectedBody, rr.Body.String(), "Response body mismatch")
}

// AssertErrorResponse asserts that the response is an error with the expected status and message
func AssertErrorResponse(t *testing.T, rr *httptest.ResponseRecorder, expectedStatus int, expectedMessage string) {
	assert.Equal(t, expectedStatus, rr.Code, "Status code mismatch")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"), "Content-Type should be application/json")

	var errorResp map[string]interface{}
	err := json.Unmarshal(rr.Body.Bytes(), &errorResp)
	require.NoError(t, err, "Error response should be valid JSON")

	if expectedMessage != "" {
		assert.Contains(t, errorResp["error"], expectedMessage, "Error message should contain expected text")
	}
}

// CreateJWTToken creates a valid JWT token for testing (mock implementation)
func CreateJWTToken(userID string) string {
	// This is a mock implementation for testing
	// In real tests, you might want to use the actual JWT signing logic
	return fmt.Sprintf("mock-jwt-token-for-user-%s", userID)
}

// WithJWTAuth adds JWT authentication header to the request headers
func WithJWTAuth(userID string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + CreateJWTToken(userID),
	}
}

// WithAPIKeyAuth adds API key authentication header to the request headers
func WithAPIKeyAuth(apiKey string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + apiKey,
	}
}

// RunHTTPTestCases runs a set of HTTP test cases
func RunHTTPTestCases(t *testing.T, ts *TestServer, testCases []HTTPTestCase) {
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			// Setup
			if tc.Setup != nil {
				tc.Setup(ts)
			}

			// Make request
			rr := MakeRequest(ts, tc.Method, tc.URL, tc.Body, tc.Headers)

			// Assert response
			if tc.ExpectedBody != nil {
				switch expectedBody := tc.ExpectedBody.(type) {
				case string:
					AssertTextResponse(t, rr, tc.ExpectedStatus, expectedBody)
				default:
					AssertJSONResponse(t, rr, tc.ExpectedStatus, expectedBody)
				}
			} else {
				assert.Equal(t, tc.ExpectedStatus, rr.Code, "Status code mismatch")
			}

			// Cleanup
			if tc.Cleanup != nil {
				tc.Cleanup(ts)
			}
		})
	}
}

// LogResponse logs the response for debugging purposes
func LogResponse(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Logf("Response Status: %d", rr.Code)
	t.Logf("Response Headers: %v", rr.Header())
	t.Logf("Response Body: %s", rr.Body.String())
}

// ParseJSONResponse parses the response body as JSON into the provided interface
func ParseJSONResponse(t *testing.T, rr *httptest.ResponseRecorder, v interface{}) {
	err := json.Unmarshal(rr.Body.Bytes(), v)
	require.NoError(t, err, "Response body should be valid JSON")
}

// AssertStatusCode is a simple helper to assert just the status code
func AssertStatusCode(t *testing.T, rr *httptest.ResponseRecorder, expectedStatus int) {
	assert.Equal(t, expectedStatus, rr.Code, "Status code mismatch")
}

// AssertHeader asserts that a specific header has the expected value
func AssertHeader(t *testing.T, rr *httptest.ResponseRecorder, headerName, expectedValue string) {
	assert.Equal(t, expectedValue, rr.Header().Get(headerName), fmt.Sprintf("Header %s mismatch", headerName))
}

// AssertContentType is a helper to assert the Content-Type header
func AssertContentType(t *testing.T, rr *httptest.ResponseRecorder, expectedContentType string) {
	AssertHeader(t, rr, "Content-Type", expectedContentType)
}
