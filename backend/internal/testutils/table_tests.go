package testutils

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/vibexp/vibexp/internal/container"
)

// HTTPTestCase represents a single HTTP test case for table-driven tests
type HTTPTestCase struct {
	// Test identification
	Name string

	// Request configuration
	Method string
	URL    string
	Body   interface{}

	// Authentication setup
	SetupAuth func() (string, AuthType) // Returns token/key and auth type

	// Mock setup
	SetupMocks func(container.Container)

	// Expected results
	ExpectedStatus int
	ExpectedBody   interface{}
	ExpectedError  string

	// Optional custom validations
	CustomValidations []func(TestingT, *httptest.ResponseRecorder)

	// Test configuration
	SkipContentTypeCheck bool
	Headers              map[string]string
}

// AuthType represents the type of authentication to use
type AuthType int

const (
	AuthTypeNone AuthType = iota
	AuthTypeJWT
	AuthTypeAPIKey
)

// TableTestConfig holds configuration for running table tests
type TableTestConfig struct {
	// Server setup
	CreateServer func(TestingT, container.Container) *httptest.Server

	// Default validations
	SkipDefaultValidations bool

	// Common setup for all tests
	CommonSetup func(TestingT, container.Container)

	// Common cleanup for all tests
	CommonCleanup func(TestingT, container.Container)
}

// RunHTTPTests executes a table of HTTP test cases
func RunHTTPTests(t TestingT, tests []HTTPTestCase, config TableTestConfig) {
	if t == nil {
		panic("testing interface cannot be nil")
	}
	if len(tests) == 0 {
		t.Fatal("test cases cannot be empty")
		return
	}

	t.Helper()

	for _, tt := range tests {
		// For now, just run each test directly without subtests
		// In a real implementation, you would use the testing framework's Run method
		runSingleHTTPTest(t, tt, config)
	}
}

// RunHTTPTestsWithServer executes a table of HTTP test cases with a pre-created server
func RunHTTPTestsWithServer(t TestingT, tests []HTTPTestCase, server *httptest.Server) {
	if t == nil {
		panic("testing interface cannot be nil")
	}
	if server == nil {
		t.Fatal("server cannot be nil")
		return
	}
	if len(tests) == 0 {
		t.Fatal("test cases cannot be empty")
		return
	}

	t.Helper()

	for _, tt := range tests {
		// For now, just run each test directly without subtests
		runSingleHTTPTestWithServer(t, tt, server)
	}
}

// runSingleHTTPTest executes a single HTTP test case
func runSingleHTTPTest(t TestingT, tt HTTPTestCase, config TableTestConfig) {
	t.Helper()

	// Validate test case
	if err := validateTestCase(tt); err != nil {
		t.Fatalf("Invalid test case: %v", err)
		return
	}

	// Create mock container (assuming we have one)
	mockContainer := CreateMockContainer(t)

	// Run common setup
	if config.CommonSetup != nil {
		config.CommonSetup(t, mockContainer)
	}

	// Setup mocks for this specific test
	if tt.SetupMocks != nil {
		tt.SetupMocks(mockContainer)
	}

	// Create server
	var server *httptest.Server
	if config.CreateServer != nil {
		server = config.CreateServer(t, mockContainer)
	} else {
		server = CreateTestServer(t, mockContainer)
	}
	defer server.Close()

	// Run the test
	runSingleHTTPTestWithServer(t, tt, server)

	// Run common cleanup
	if config.CommonCleanup != nil {
		config.CommonCleanup(t, mockContainer)
	}
}

// runSingleHTTPTestWithServer executes a single HTTP test case with a pre-created server
func runSingleHTTPTestWithServer(t TestingT, tt HTTPTestCase, server *httptest.Server) {
	t.Helper()

	// Create request
	req, err := createRequestForTest(tt)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
		return
	}

	// Add custom headers
	for key, value := range tt.Headers {
		req.Header.Set(key, value)
	}

	// Setup authentication
	if tt.SetupAuth != nil {
		token, authType := tt.SetupAuth()
		switch authType {
		case AuthTypeJWT:
			req.Header.Set("Authorization", "Bearer "+token)
		case AuthTypeAPIKey:
			req.Header.Set("X-API-Key", token)
		}
	}

	// Execute request
	response := ExecuteRequest(server, req)

	// Validate response
	validateHTTPTestResponse(t, tt, response)
}

// createRequestForTest creates an HTTP request for a test case
func createRequestForTest(tt HTTPTestCase) (*http.Request, error) {
	if tt.Method == "" {
		return nil, fmt.Errorf("HTTP method is required")
	}
	if tt.URL == "" {
		return nil, fmt.Errorf("URL is required")
	}

	return NewTestRequest(tt.Method, tt.URL, tt.Body)
}

// validateTestCase validates a test case configuration
func validateTestCase(tt HTTPTestCase) error {
	if tt.Name == "" {
		return fmt.Errorf("test name is required")
	}
	if tt.Method == "" {
		return fmt.Errorf("HTTP method is required")
	}
	if tt.URL == "" {
		return fmt.Errorf("URL is required")
	}
	if tt.ExpectedStatus == 0 {
		return fmt.Errorf("expected status code is required")
	}
	return nil
}

// validateHTTPTestResponse validates the HTTP response against test expectations
func validateHTTPTestResponse(t TestingT, tt HTTPTestCase, response *httptest.ResponseRecorder) {
	t.Helper()

	// Validate status code
	AssertStatusCode(t, response, tt.ExpectedStatus)

	// Validate content type if not skipped
	if !tt.SkipContentTypeCheck && response.Code < 400 {
		AssertContentType(t, response, "application/json")
	}

	// Validate expected body if provided
	if tt.ExpectedBody != nil {
		AssertJSONResponse(t, response, tt.ExpectedBody)
	}

	// Validate expected error if provided
	if tt.ExpectedError != "" {
		AssertErrorResponse(t, response, tt.ExpectedStatus, tt.ExpectedError)
	}

	// Run custom validations
	for _, validation := range tt.CustomValidations {
		validation(t, response)
	}
}

// AuthTestCase represents a test case specifically for authentication testing
type AuthTestCase struct {
	Name           string
	Method         string
	URL            string
	Body           interface{}
	AuthHeader     string
	ExpectedStatus int
	ExpectedError  string
}

// RunAuthTests executes authentication-focused test cases
func RunAuthTests(t TestingT, tests []AuthTestCase, server *httptest.Server) {
	if t == nil {
		panic("testing interface cannot be nil")
	}
	if server == nil {
		t.Fatal("server cannot be nil")
		return
	}
	if len(tests) == 0 {
		t.Fatal("test cases cannot be empty")
		return
	}

	t.Helper()

	for _, tt := range tests {
		// For now, just run each test directly without subtests
		runSingleAuthTest(t, tt, server)
	}
}

// runSingleAuthTest executes a single authentication test case
func runSingleAuthTest(t TestingT, tt AuthTestCase, server *httptest.Server) {
	t.Helper()

	// Create request
	req, err := NewTestRequest(tt.Method, tt.URL, tt.Body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
		return
	}

	// Add auth header if provided
	if tt.AuthHeader != "" {
		req.Header.Set("Authorization", tt.AuthHeader)
	}

	// Execute request
	response := ExecuteRequest(server, req)

	// Validate response
	AssertStatusCode(t, response, tt.ExpectedStatus)

	if tt.ExpectedError != "" {
		AssertErrorResponse(t, response, tt.ExpectedStatus, tt.ExpectedError)
	}
}

// CRUDTestSuite represents a complete CRUD test suite for a resource
type CRUDTestSuite struct {
	ResourceName string
	BasePath     string

	// Test data
	CreateData  interface{}
	UpdateData  interface{}
	InvalidData interface{}

	// Authentication
	// #nosec G117 - Test utility struct field for test authentication tokens
	AuthToken string
	AuthType  AuthType

	// Expected responses
	CreateResponse interface{}
	UpdateResponse interface{}

	// Custom validations
	CustomCreateValidation func(TestingT, *httptest.ResponseRecorder)
	CustomUpdateValidation func(TestingT, *httptest.ResponseRecorder)
	CustomDeleteValidation func(TestingT, *httptest.ResponseRecorder)
	CustomListValidation   func(TestingT, *httptest.ResponseRecorder)
}

// RunCRUDTests executes a complete CRUD test suite
// Note: This is a simplified version that runs all tests sequentially
// In a real implementation, you would use testing.T.Run for subtests
func RunCRUDTests(t TestingT, suite CRUDTestSuite, server *httptest.Server) {
	if t == nil {
		panic("testing interface cannot be nil")
	}
	if server == nil {
		t.Fatal("server cannot be nil")
		return
	}

	t.Helper()

	resourceID := runCRUDCreateTest(t, suite, server)
	runCRUDReadTest(t, suite, server, resourceID)
	runCRUDUpdateTest(t, suite, server, resourceID)
	runCRUDListTest(t, suite, server)
	runCRUDDeleteTest(t, suite, server, resourceID)
	runCRUDInvalidDataTest(t, suite, server)
}

func runCRUDCreateTest(t TestingT, suite CRUDTestSuite, server *httptest.Server) string {
	t.Helper()
	req, err := createAuthenticatedRequest("POST", suite.BasePath, suite.CreateData, suite.AuthToken, suite.AuthType)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
		return ""
	}

	response := ExecuteRequest(server, req)
	AssertStatusCode(t, response, http.StatusCreated)

	if suite.CreateResponse != nil {
		AssertJSONResponse(t, response, suite.CreateResponse)
	}

	if suite.CustomCreateValidation != nil {
		suite.CustomCreateValidation(t, response)
	}

	return "test-resource-id"
}

func runCRUDReadTest(t TestingT, suite CRUDTestSuite, server *httptest.Server, resourceID string) {
	t.Helper()
	url := fmt.Sprintf("%s/%s", suite.BasePath, resourceID)
	req, err := createAuthenticatedRequest("GET", url, nil, suite.AuthToken, suite.AuthType)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
		return
	}

	response := ExecuteRequest(server, req)
	AssertStatusCode(t, response, http.StatusOK)
	AssertValidJSON(t, response)
}

func runCRUDUpdateTest(t TestingT, suite CRUDTestSuite, server *httptest.Server, resourceID string) {
	t.Helper()
	url := fmt.Sprintf("%s/%s", suite.BasePath, resourceID)
	req, err := createAuthenticatedRequest("PUT", url, suite.UpdateData, suite.AuthToken, suite.AuthType)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
		return
	}

	response := ExecuteRequest(server, req)
	AssertStatusCode(t, response, http.StatusOK)

	if suite.UpdateResponse != nil {
		AssertJSONResponse(t, response, suite.UpdateResponse)
	}

	if suite.CustomUpdateValidation != nil {
		suite.CustomUpdateValidation(t, response)
	}
}

func runCRUDListTest(t TestingT, suite CRUDTestSuite, server *httptest.Server) {
	t.Helper()
	req, err := createAuthenticatedRequest("GET", suite.BasePath, nil, suite.AuthToken, suite.AuthType)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
		return
	}

	response := ExecuteRequest(server, req)
	AssertStatusCode(t, response, http.StatusOK)
	AssertValidJSON(t, response)

	if suite.CustomListValidation != nil {
		suite.CustomListValidation(t, response)
	}
}

func runCRUDDeleteTest(t TestingT, suite CRUDTestSuite, server *httptest.Server, resourceID string) {
	t.Helper()
	url := fmt.Sprintf("%s/%s", suite.BasePath, resourceID)
	req, err := createAuthenticatedRequest("DELETE", url, nil, suite.AuthToken, suite.AuthType)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
		return
	}

	response := ExecuteRequest(server, req)
	AssertStatusCode(t, response, http.StatusNoContent)

	if suite.CustomDeleteValidation != nil {
		suite.CustomDeleteValidation(t, response)
	}
}

func runCRUDInvalidDataTest(t TestingT, suite CRUDTestSuite, server *httptest.Server) {
	t.Helper()
	if suite.InvalidData == nil {
		return
	}

	req, err := createAuthenticatedRequest("POST", suite.BasePath, suite.InvalidData, suite.AuthToken, suite.AuthType)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
		return
	}

	response := ExecuteRequest(server, req)
	AssertStatusCode(t, response, http.StatusBadRequest)
}

// createAuthenticatedRequest creates an HTTP request with authentication
func createAuthenticatedRequest(
	method, url string, body interface{}, token string, authType AuthType,
) (*http.Request, error) {
	req, err := NewTestRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	switch authType {
	case AuthTypeJWT:
		req.Header.Set("Authorization", "Bearer "+token)
	case AuthTypeAPIKey:
		req.Header.Set("X-API-Key", token)
	}

	return req, nil
}

// CreateMockContainer creates a mock container for testing
func CreateMockContainer(t TestingT) container.Container {
	// This would be implemented based on your actual container interface
	// For now, return nil as a placeholder
	return nil
}

// Helper method to run test cases with a custom test function
