package testutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFrameworkBasicFunctionality tests that the testing framework works correctly
func TestFrameworkBasicFunctionality(t *testing.T) {
	// Create a test server
	ts := NewTestServer(t)
	defer ts.Close()

	// Test that the server is created successfully
	assert.NotNil(t, ts.Server)
	assert.NotNil(t, ts.HTTPServer)
	assert.NotNil(t, ts.Config)
	assert.NotEmpty(t, ts.URL())

	// Test ping endpoint using our test helpers
	rr := MakeRequest(ts, "GET", "/ping", nil, nil)
	AssertTextResponse(t, rr, 200, "pong")

	// Test health endpoint using our test helpers
	rr = MakeRequest(ts, "GET", "/health", nil, nil)
	AssertJSONResponse(t, rr, 200, map[string]string{"status": "healthy", "sha": ""})
}

// TestHTTPTestCasesFramework tests the HTTP test cases framework
func TestHTTPTestCasesFramework(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	testCases := []HTTPTestCase{
		{
			Name:           "Ping endpoint",
			Method:         "GET",
			URL:            "/ping",
			ExpectedStatus: 200,
			ExpectedBody:   "pong",
		},
		{
			Name:           "Health endpoint",
			Method:         "GET",
			URL:            "/health",
			ExpectedStatus: 200,
			ExpectedBody:   map[string]string{"status": "healthy", "sha": ""},
		},
		{
			Name:           "Non-existent endpoint",
			Method:         "GET",
			URL:            "/non-existent",
			ExpectedStatus: 404,
		},
	}

	RunHTTPTestCases(t, ts, testCases)
}

// TestFixtures tests that the fixtures work correctly
func TestFixtures(t *testing.T) {
	// Test user fixture
	user := CreateTestUser()
	assert.NotNil(t, user)
	assert.Equal(t, "test@example.com", user.Email)
	assert.Equal(t, "Test User", user.Name)
	assert.Equal(t, "basic", user.SubscriptionStatus)

	// Test constants
	assert.Equal(t, "test-api-key-12345", TestAPIKeyValue)
	assert.NotEmpty(t, TestJWTToken)
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", TestUserID)
}

// TestTestHelpers tests various helper functions
func TestTestHelpers(t *testing.T) {
	// Test JWT token creation
	token := CreateJWTToken("user123")
	assert.Equal(t, "mock-jwt-token-for-user-user123", token)

	// Test auth headers
	jwtHeaders := WithJWTAuth("user123")
	assert.Contains(t, jwtHeaders["Authorization"], "Bearer mock-jwt-token-for-user-user123")

	apiKeyHeaders := WithAPIKeyAuth("test-key")
	assert.Equal(t, "Bearer test-key", apiKeyHeaders["Authorization"])

	// Test constants
	assert.NotEmpty(t, JSONHeaders["Content-Type"])
	assert.NotEmpty(t, AuthHeaders["Authorization"])
	assert.NotEmpty(t, APIKeyHeaders["Authorization"])
}
