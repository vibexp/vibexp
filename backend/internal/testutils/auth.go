package testutils

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/models"
)

// apiAccessHeader is the HTTP header used for API key authentication
const apiAccessHeader = "X-API-Key"

// CreateTestUser creates a test user with default values for testing
func CreateTestUser() *models.User {
	return CreateTestUserWithID("test-user-123")
}

// CreateTestUserWithID creates a test user with a specific ID
func CreateTestUserWithID(userID string) *models.User {
	now := time.Now()
	googleID := "google-" + userID
	return &models.User{
		ID:                 userID,
		GoogleID:           &googleID,
		Email:              "test@example.com",
		Name:               "Test User",
		AvatarURL:          nil,
		StripeCustomerID:   nil,
		SubscriptionStatus: models.SubscriptionStatusBasic,
		TrialEndsAt:        nil,
		SubscriptionPlan:   &[]string{models.PlanBasic}[0],
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// CreateTestUserWithEmail creates a test user with a specific email
func CreateTestUserWithEmail(email string) *models.User {
	user := CreateTestUser()
	user.Email = email
	return user
}

// CreateTestAPIKey creates a test API key for a given user ID
// Returns the APIKey model and the full API key string
func CreateTestAPIKey(userID string) (*models.APIKey, string, error) {
	return CreateTestAPIKeyWithName(userID, "Test API Key")
}

// CreateTestAPIKeyWithName creates a test API key with a specific name
func CreateTestAPIKeyWithName(userID, name string) (*models.APIKey, string, error) {
	if userID == "" {
		return nil, "", fmt.Errorf("userID cannot be empty")
	}
	if name == "" {
		return nil, "", fmt.Errorf("name cannot be empty")
	}

	// Generate a 32-byte random key like the real implementation
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, "", fmt.Errorf("failed to generate random key: %w", err)
	}

	// Create the full key as hex string with 'ak_' prefix
	fullKey := "ak_" + hex.EncodeToString(keyBytes)

	// Create prefix (first 8 characters after 'ak_')
	keyPrefix := fullKey[:10] // 'ak_' + first 7 chars

	// Hash the full key for storage
	hash := sha256.Sum256([]byte(fullKey))
	keyHash := hex.EncodeToString(hash[:])

	now := time.Now()

	// Generate a unique ID based on timestamp and random bytes
	var uniqueID string
	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		// Fallback to timestamp-only ID if random generation fails
		uniqueID = fmt.Sprintf("test-api-key-%d", now.UnixNano())
	} else {
		uniqueID = fmt.Sprintf("test-api-key-%d-%s", now.UnixNano(), hex.EncodeToString(idBytes)[:8])
	}

	apiKey := &models.APIKey{
		ID:         uniqueID,
		UserID:     userID,
		Name:       name,
		KeyHash:    keyHash,
		KeyPrefix:  keyPrefix,
		LastUsedAt: nil,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	return apiKey, fullKey, nil
}

// AddAuthHeader adds authentication via a session cookie to an HTTP request.
// The token parameter is treated as the vx_session cookie value.
// NOTE: In the cookie-based auth model, there is no Authorization Bearer header.
// This function is kept for API compatibility; it now sets the session cookie.
func AddAuthHeader(req *http.Request, token string) {
	req.AddCookie(&http.Cookie{
		Name:  sesslib.CookieName,
		Value: token,
	})
}

// AddAPIKeyHeader adds API key authentication header to an HTTP request
func AddAPIKeyHeader(req *http.Request, apiKey string) {
	req.Header.Set(apiAccessHeader, apiKey)
}

// CreateAuthenticatedRequest creates an HTTP request with JWT authentication
func CreateAuthenticatedRequest(method, url, userID, email string) (*http.Request, error) {
	return CreateAuthenticatedRequestWithBody(method, url, userID, email, nil)
}

// CreateAuthenticatedRequestWithBody creates an HTTP request with JWT authentication and body
func CreateAuthenticatedRequestWithBody(method, url, userID, email string, body interface{}) (*http.Request, error) {
	req, err := CreateTestRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create test request: %w", err)
	}

	token, err := GenerateTestJWT(userID, email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate test JWT: %w", err)
	}

	AddAuthHeader(req, token)
	return req, nil
}

// CreateAPIKeyAuthenticatedRequest creates an HTTP request with API key authentication
func CreateAPIKeyAuthenticatedRequest(method, url, apiKey string) (*http.Request, error) {
	return CreateAPIKeyAuthenticatedRequestWithBody(method, url, apiKey, nil)
}

// CreateAPIKeyAuthenticatedRequestWithBody creates an HTTP request with API key authentication and body
func CreateAPIKeyAuthenticatedRequestWithBody(method, url, apiKey string, body interface{}) (*http.Request, error) {
	req, err := CreateTestRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create test request: %w", err)
	}

	AddAPIKeyHeader(req, apiKey)
	return req, nil
}

// MockContext creates a context with user ID for testing
func MockContext(userID string) context.Context {
	return context.WithValue(context.Background(), contextKeyUserID, userID)
}

// MockContextWithUser creates a context with user object for testing
func MockContextWithUser(user *models.User) context.Context {
	ctx := context.WithValue(context.Background(), contextKeyUserID, user.ID)
	return context.WithValue(ctx, contextKeyUser, user)
}

// GetTestUserCredentials returns standard test user credentials
func GetTestUserCredentials() (userID, email string) {
	return "test-user-123", "test@example.com"
}

// CreateTestJWTForDefaultUser creates a JWT token for the default test user
func CreateTestJWTForDefaultUser() (string, error) {
	userID, email := GetTestUserCredentials()
	return GenerateTestJWT(userID, email)
}

// CreateUnauthorizedRequest creates an HTTP request without any authentication
func CreateUnauthorizedRequest(method, url string, body interface{}) (*http.Request, error) {
	return CreateTestRequest(method, url, body)
}

// CreateInvalidJWTRequest creates an HTTP request with an invalid JWT token
func CreateInvalidJWTRequest(method, url string, body interface{}) (*http.Request, error) {
	req, err := CreateTestRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create test request: %w", err)
	}

	userID, email := GetTestUserCredentials()
	invalidToken, err := GenerateInvalidTestJWT(userID, email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate invalid JWT: %w", err)
	}

	AddAuthHeader(req, invalidToken)
	return req, nil
}

// CreateExpiredJWTRequest creates an HTTP request with an expired JWT token
func CreateExpiredJWTRequest(method, url string, body interface{}) (*http.Request, error) {
	req, err := CreateTestRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create test request: %w", err)
	}

	userID, email := GetTestUserCredentials()
	expiredToken, err := GenerateExpiredTestJWT(userID, email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate expired JWT: %w", err)
	}

	AddAuthHeader(req, expiredToken)
	return req, nil
}

// CreateMalformedAuthRequest creates an HTTP request with malformed authorization header
func CreateMalformedAuthRequest(method, url string, body interface{}) (*http.Request, error) {
	req, err := CreateTestRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create test request: %w", err)
	}

	// Use malformed auth header (missing "Bearer" prefix)
	req.Header.Set("Authorization", "malformed-token")
	return req, nil
}

// CreateInvalidAPIKeyRequest creates an HTTP request with an invalid API key
func CreateInvalidAPIKeyRequest(method, url string, body interface{}) (*http.Request, error) {
	req, err := CreateTestRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create test request: %w", err)
	}

	AddAPIKeyHeader(req, "invalid-api-key")
	return req, nil
}

// CreateRequestWithBothAuth creates an HTTP request with both JWT and API key (to test conflicts)
func CreateRequestWithBothAuth(method, url, userID, email, apiKey string, body interface{}) (*http.Request, error) {
	req, err := CreateTestRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create test request: %w", err)
	}

	token, err := GenerateTestJWT(userID, email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate test JWT: %w", err)
	}

	AddAuthHeader(req, token)
	AddAPIKeyHeader(req, apiKey)
	return req, nil
}

// CreateRequestWithCustomClaims creates an HTTP request with a session cookie
// derived from the "user_id" key in the claims map. The JWT-based implementation
// is removed; this is a compatibility shim.
func CreateRequestWithCustomClaims(
	method, url string, claims map[string]interface{}, body interface{},
) (*http.Request, error) {
	req, err := CreateTestRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create test request: %w", err)
	}

	userID, _ := claims["user_id"].(string)
	if userID == "" {
		userID = "test-user-claims"
	}
	cookieValue, err := GenerateTestJWT(userID, "")
	if err != nil {
		return nil, fmt.Errorf("failed to generate test session cookie: %w", err)
	}

	AddAuthHeader(req, cookieValue)
	return req, nil
}

// GetAuthToken extracts the auth token from a request header
func GetAuthToken(req *http.Request) string {
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	// Remove "Bearer " prefix
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		return authHeader[7:]
	}

	return authHeader
}

// GetAPIKey extracts the API key from a request header
func GetAPIKey(req *http.Request) string {
	return req.Header.Get(apiAccessHeader)
}

// ValidateAuthHeaders checks if the request has proper authentication headers
func ValidateAuthHeaders(t TestingT, req *http.Request, expectJWT, expectAPIKey bool) {
	t.Helper()

	hasJWT := req.Header.Get("Authorization") != ""
	hasAPIKey := req.Header.Get(apiAccessHeader) != ""

	if expectJWT && !hasJWT {
		t.Error("Expected JWT authentication header to be present")
	}
	if !expectJWT && hasJWT {
		t.Error("Did not expect JWT authentication header to be present")
	}
	if expectAPIKey && !hasAPIKey {
		t.Error("Expected API key header to be present")
	}
	if !expectAPIKey && hasAPIKey {
		t.Error("Did not expect API key header to be present")
	}
}
