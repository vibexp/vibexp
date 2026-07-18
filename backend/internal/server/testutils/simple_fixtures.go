package testutils

import (
	"time"

	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/models"
)

// Test constants
var TestUserID = "00000000-0000-0000-0000-000000000001"

const TestAPIKeyValue = "test-api-key-12345"

// TestJWTToken is a test-only JWT token, not a real credential
// #nosec G101 -- This is a test fixture, not a real hardcoded credential
const TestJWTToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test.token"

// Common HTTP header names and values used across the test helpers
const (
	headerContentType = "Content-Type"
	contentTypeJSON   = "application/json"
)

// Common HTTP Headers for testing
var (
	JSONHeaders = map[string]string{
		headerContentType: contentTypeJSON,
	}

	AuthHeaders = map[string]string{
		"Authorization":   "Bearer " + TestJWTToken,
		headerContentType: contentTypeJSON,
	}

	APIKeyHeaders = map[string]string{
		"Authorization":   "Bearer " + TestAPIKeyValue,
		headerContentType: contentTypeJSON,
	}
)

// Simple test user
func CreateTestUser() *models.User {
	avatarURL := "https://example.com/profile.jpg"
	googleID := "google-123456"
	return &models.User{
		ID:                 uuid.New().String(),
		GoogleID:           &googleID,
		Email:              "test@example.com",
		Name:               "Test User",
		AvatarURL:          &avatarURL,
		SubscriptionStatus: models.SubscriptionStatusBasic,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
}

// Test error response
type TestErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// Test success response
type TestSuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
