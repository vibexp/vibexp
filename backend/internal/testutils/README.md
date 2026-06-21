# Test Utilities

This package provides comprehensive testing utilities for the VibeExp.io backend application. It includes JWT token generation, authentication helpers, HTTP testing utilities, and custom assertions to streamline unit and integration testing.

## Overview

The test utilities are organized into five main modules:

- **JWT Utilities** (`jwt.go`) - Generate JWT tokens for API testing
- **Authentication Helpers** (`auth.go`) - Create test users, API keys, and authenticated requests
- **HTTP Testing Helpers** (`helpers.go`) - Simplify HTTP request/response testing
- **Custom Assertions** (`assertions.go`) - Domain-specific assertions for models
- **Mock Utilities** (`mocks.go`) - Comprehensive mock management for all interfaces

## JWT Utilities

### Basic JWT Generation

```go
import "github.com/vibexp/vibexp/internal/testutils"

// Generate a standard test JWT
token, err := testutils.GenerateTestJWT("user-123", "test@example.com")
if err != nil {
    t.Fatal(err)
}

// Use the token in Authorization header
req.Header.Set("Authorization", "Bearer " + token)
```

### Advanced JWT Generation

```go
// Generate JWT with custom expiration
expiredTime := time.Now().Add(-1 * time.Hour)
options := &testutils.JWTTestOptions{
    ExpiresAt: &expiredTime,
}
expiredToken, err := testutils.GenerateTestJWTWithOptions("user-123", "test@example.com", options)

// Generate expired JWT (shorthand)
expiredToken, err := testutils.GenerateExpiredTestJWT("user-123", "test@example.com")

// Generate invalid JWT with wrong signature
invalidToken, err := testutils.GenerateInvalidTestJWT("user-123", "test@example.com")

// Generate JWT with custom claims
claims := services.JWTClaims{
    UserID: "custom-user",
    Email:  "custom@example.com",
    RegisteredClaims: jwt.RegisteredClaims{
        ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Hour)),
        IssuedAt:  jwt.NewNumericDate(time.Now()),
        NotBefore: jwt.NewNumericDate(time.Now()),
    },
}
customToken, err := testutils.GenerateTestJWTWithClaims(claims)
```

## Authentication Helpers

### Creating Test Users

```go
// Create a default test user
user := testutils.CreateTestUser()
// Returns user with ID "test-user-123" and email "test@example.com"

// Create user with specific ID
user := testutils.CreateTestUserWithID("my-user-id")

// Create user with specific email
user := testutils.CreateTestUserWithEmail("specific@example.com")
```

### Creating Test API Keys

```go
// Create a test API key
apiKeyModel, fullAPIKey, err := testutils.CreateTestAPIKey("user-123")
if err != nil {
    t.Fatal(err)
}

// Create API key with specific name
apiKeyModel, fullAPIKey, err := testutils.CreateTestAPIKeyWithName("user-123", "My Test Key")
```

### Creating Authenticated Requests

```go
// Create JWT authenticated request
req, err := testutils.CreateAuthenticatedRequest("GET", "/api/v1/prompts", "user-123", "test@example.com")

// Create JWT authenticated request with JSON body
requestBody := map[string]string{"name": "test"}
req, err := testutils.CreateAuthenticatedRequestWithBody("POST", "/api/v1/prompts", "user-123", "test@example.com", requestBody)

// Create API key authenticated request
req, err := testutils.CreateAPIKeyAuthenticatedRequest("GET", "/api/v1/prompts", "ak_1234567890abcdef...")

// Manually add auth headers
req, _ := http.NewRequest("GET", "/api/v1/test", nil)
testutils.AddAuthHeader(req, jwtToken)
testutils.AddAPIKeyHeader(req, apiKey)
```

### Context Helpers

```go
// Create context with user ID
ctx := testutils.MockContext("user-123")

// Create context with full user object
user := testutils.CreateTestUser()
ctx := testutils.MockContextWithUser(user)

// Get standard test credentials
userID, email := testutils.GetTestUserCredentials()
// Returns "test-user-123", "test@example.com"
```

## HTTP Testing Helpers

### Creating HTTP Requests

```go
// Create simple request
req, err := testutils.CreateTestRequest("GET", "/api/v1/test", nil)

// Create request with JSON body
body := map[string]string{"name": "test"}
req, err := testutils.CreateTestRequest("POST", "/api/v1/test", body)

// Create request with string body
req, err := testutils.CreateTestRequest("POST", "/api/v1/test", "raw body content")

// Create request with context
ctx := context.Background()
req, err := testutils.CreateTestRequestWithContext(ctx, "GET", "/api/v1/test", nil)
```

### Response Assertions

```go
// Test HTTP response status
testutils.AssertStatusCode(t, recorder, http.StatusOK)

// Test JSON response structure
expected := map[string]interface{}{
    "id":   "123",
    "name": "test",
}
testutils.AssertJSONResponse(t, recorder, expected)

// Test JSON response contains specific fields
expectedFields := map[string]interface{}{
    "status": "success",
    "count":  5,
}
testutils.AssertJSONResponseContains(t, recorder, expectedFields)

// Test error responses
testutils.AssertErrorResponse(t, recorder, http.StatusBadRequest, "invalid input")

// Test response contains text
testutils.AssertResponseContains(t, recorder, "success")

// Test response headers
testutils.AssertHeader(t, recorder, "Content-Type", "application/json")

// Test empty response
testutils.AssertEmptyResponse(t, recorder)
```

### JSON Utilities

```go
// Parse JSON response into struct
var response MyResponseStruct
testutils.ParseJSONResponse(t, recorder, &response)

// Compare JSON strings (ignoring formatting)
expectedJSON := `{"id": "123", "name": "test"}`
actualJSON := recorder.Body.String()
testutils.CompareJSON(t, expectedJSON, actualJSON)
```

### File Upload Testing

```go
// Create multipart form request
fields := map[string]string{"name": "test"}
files := map[string][]byte{"file.txt": []byte("file content")}
req, err := testutils.CreateMultipartFormRequest("POST", "/upload", fields, files)
```

## Custom Assertions

### Model Assertions

```go
// Assert user models are equal
expectedUser := testutils.CreateTestUser()
testutils.AssertUserEqual(t, expectedUser, actualUser)

// Assert API key models are equal (excludes sensitive fields)
testutils.AssertAPIKeyEqual(t, expectedAPIKey, actualAPIKey)

// Assert prompt models are equal
testutils.AssertPromptEqual(t, expectedPrompt, actualPrompt)

// Assert static context models are equal
testutils.AssertStaticContextEqual(t, expectedContext, actualContext)

// Assert work report models are equal
testutils.AssertWorkReportEqual(t, expectedReport, actualReport)

// Assert subscription models are equal
testutils.AssertSubscriptionEqual(t, expectedSub, actualSub)
```

### General Assertions

```go
// Assert time values are close (within tolerance)
testutils.AssertTimeAlmostEqual(t, expected, actual, time.Second)

// Assert slices are equal
testutils.AssertSliceEqual(t, expectedSlice, actualSlice)

// Assert slice contains element
testutils.AssertSliceContains(t, mySlice, expectedElement)

// Assert not nil
testutils.AssertNotNil(t, value, "value should not be nil")

// Assert nil
testutils.AssertNil(t, value, "value should be nil")
```

## Complete Example

Here's a complete example testing a protected endpoint:

```go
func TestGetPrompts(t *testing.T) {
    // Setup
    cfg := &config.Config{}
    srv := server.New("8080", nil, "test-api-key", cfg)

    // Create authenticated request
    req, err := testutils.CreateAuthenticatedRequest(
        "GET",
        "/api/v1/prompts",
        "test-user-123",
        "test@example.com",
    )
    if err != nil {
        t.Fatal(err)
    }

    // Execute request
    recorder := httptest.NewRecorder()
    srv.Router().ServeHTTP(recorder, req)

    // Assert response
    testutils.AssertStatusCode(t, recorder, http.StatusOK)
    testutils.AssertJSONResponseContains(t, recorder, map[string]interface{}{
        "prompts": []interface{}{},
        "total_count": 0,
    })
}

func TestCreatePromptUnauthorized(t *testing.T) {
    // Setup
    cfg := &config.Config{}
    srv := server.New("8080", nil, "test-api-key", cfg)

    // Create unauthenticated request
    req, err := testutils.CreateTestRequest("POST", "/api/v1/prompts", map[string]string{
        "name": "test prompt",
        "body": "test content",
    })
    if err != nil {
        t.Fatal(err)
    }

    // Execute request
    recorder := httptest.NewRecorder()
    srv.Router().ServeHTTP(recorder, req)

    // Assert unauthorized
    testutils.AssertErrorResponse(t, recorder, http.StatusUnauthorized, "unauthorized")
}
```

## Integration with Existing Tests

These utilities are designed to work alongside existing test patterns. The JWT tokens generated will be valid with the application's auth service, and the utilities follow the same patterns as existing HTTP tests in `internal/server/server_test.go`.

## Mock Utilities

The mock utilities provide a comprehensive system for creating and managing mocks for all interfaces in the application. All mocks are automatically generated using [Mockery](https://vektra.github.io/mockery/) and provide full expectation support.

### Mock Containers

Mock containers organize related mocks for easy setup and teardown:

```go
// Create all mocks in one container
mocks := testutils.NewMockContainerForTest(t)

// Access individual services
mocks.Services.Auth.EXPECT().GetUserByID(mock.Anything, "user-123").Return(user, nil)
mocks.Repositories.User.EXPECT().GetByID(mock.Anything, "user-123").Return(user, nil)
mocks.External.OAuth.EXPECT().GetAuthCodeURL("state").Return("https://oauth.url")
```

### Individual Mock Containers

Create specific mock containers for focused testing:

```go
// Service mocks only
serviceMocks := testutils.NewMockServiceContainer(t)
serviceMocks.Auth.EXPECT().ValidateJWT("token").Return(&claims, nil)

// Repository mocks only
repoMocks := testutils.NewMockRepositoryContainer(t)
repoMocks.User.EXPECT().Create(mock.Anything, mock.AnythingOfType("*models.User")).Return(nil)

// External service mocks only
externalMocks := testutils.NewMockExternalContainer(t)
externalMocks.Stripe.EXPECT().CreateCustomer(mock.Anything, "email", "name").Return(customer, nil)
```

### Service Interface Mocks

Available service mocks:
- `AuthServiceInterface` - Authentication operations
- `APIKeyServiceInterface` - API key management
- `PromptServiceInterface` - Prompt operations
- `StaticContextServiceInterface` - Static context management
- `WorkReportServiceInterface` - Work report operations
- `EmbeddingProviderServiceInterface` - Embedding provider management
- `SubscriptionServiceInterface` - Subscription operations
- `EmailServiceInterface` - Email operations

### Repository Interface Mocks

Available repository mocks:
- `UserRepository` - User data access
- `APIKeyRepository` - API key data access
- `PromptRepository` - Prompt data access
- `StaticContextRepository` - Static context data access
- `WorkReportRepository` - Work report data access
- `EmbeddingProviderRepository` - Embedding provider data access
- `SubscriptionRepository` - Subscription data access

### External Interface Mocks

Available external service mocks:
- `OAuthProvider` - OAuth operations
- `StripeClient` - Stripe payment operations
- `SMTPClient` - Email sending operations
- `HTTPClient` - HTTP client operations

### Mock Regeneration

To regenerate mocks when interfaces change:

```bash
# Install or upgrade Mockery
go install github.com/vektra/mockery/v2@latest

# Regenerate all mocks
mockery

# Verify mocks compile
go build ./...

# Run mock validation tests
go test ./internal/testutils -v -run TestMock
```

### Mock Configuration

The mock generation is configured in `.mockery.yaml`:

```yaml
with-expecter: true
dir: "{{.InterfaceDir}}/mocks"
outpkg: "mocks"
filename: "mock_{{.InterfaceName}}.go"
```

Mocks are generated in separate `mocks` packages to avoid import cycles.

### Complete Example with Mocks

```go
func TestCreatePrompt(t *testing.T) {
    // Setup mocks
    mocks := testutils.NewMockContainerForTest(t)

    // Configure expectations
    user := testutils.CreateTestUser()
    prompt := &models.Prompt{
        ID:      "prompt-123",
        UserID:  user.ID,
        Name:    "Test Prompt",
        Body:    "Test content",
        Status:  "active",
    }

    mocks.Services.Auth.EXPECT().GetUserByID(mock.Anything, user.ID).Return(user, nil)
    mocks.Services.Prompt.EXPECT().CreatePrompt(user.ID, mock.AnythingOfType("*models.CreatePromptRequest")).Return(prompt, nil)

    // Create service with mocks
    service := NewPromptService(mocks.Repositories.Prompt, mocks.Services.Auth)

    // Execute test
    req := &models.CreatePromptRequest{
        Name: "Test Prompt",
        Body: "Test content",
    }
    result, err := service.CreatePrompt(user.ID, req)

    // Assert results
    assert.NoError(t, err)
    assert.Equal(t, prompt.ID, result.ID)
    assert.Equal(t, prompt.Name, result.Name)

    // Verify mock expectations
    mocks.Services.Auth.AssertExpectations(t)
    mocks.Services.Prompt.AssertExpectations(t)
}
```

## Best Practices

1. **Use Default Credentials**: For most tests, use the default test user credentials unless you need specific values
2. **JWT vs API Key**: Use JWT for user-based endpoints, API keys for service-to-service endpoints
3. **Response Assertions**: Use specific assertions (`AssertJSONResponse`) over generic ones when possible
4. **Error Testing**: Always test both success and error cases using `AssertErrorResponse`
5. **Model Assertions**: Use custom model assertions for comparing complex domain objects
6. **Time Tolerance**: Use `AssertTimeAlmostEqual` for time comparisons to account for processing delays
7. **Mock Expectations**: Always call `AssertExpectations(t)` on mocks to verify all expected calls were made
8. **Mock Containers**: Use `NewMockContainerForTest(t)` for comprehensive testing with all dependencies mocked
9. **Interface Testing**: Use the interface validation tests to ensure mocks implement interfaces correctly
10. **Mock Regeneration**: Regenerate mocks whenever interfaces change to keep tests up to date

## Dependencies

- `github.com/golang-jwt/jwt/v5` - JWT token handling
- `github.com/stretchr/testify` - Testing assertions and mock framework
- `github.com/vektra/mockery/v2` - Mock generation tool
- Standard library packages for HTTP testing

## Mock Generation Commands

Quick reference for mock generation:

```bash
# Install Mockery
go install github.com/vektra/mockery/v2@latest

# Generate all mocks
mockery

# Verify generation
go build ./...
go test ./internal/testutils -v -run TestMock
```
