package testutils

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/models"
)

func TestCreateTestUser(t *testing.T) {
	user := CreateTestUser()

	if user == nil {
		t.Fatal("Expected non-nil user")
		return
	}

	if user.ID != "test-user-123" {
		t.Errorf("Expected ID 'test-user-123', got '%s'", user.ID)
	}

	if user.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%s'", user.Email)
	}

	if user.Name != "Test User" {
		t.Errorf("Expected name 'Test User', got '%s'", user.Name)
	}

	if user.SubscriptionStatus != models.SubscriptionStatusBasic {
		t.Errorf("Expected subscription status '%s', got '%s'", models.SubscriptionStatusBasic, user.SubscriptionStatus)
	}

	if user.SubscriptionPlan == nil || *user.SubscriptionPlan != models.PlanBasic {
		t.Errorf("Expected plan '%s', got %v", models.PlanBasic, user.SubscriptionPlan)
	}

	// Check timestamps are reasonable (within last minute)
	if time.Since(user.CreatedAt) > time.Minute {
		t.Error("CreatedAt should be recent")
	}

	if time.Since(user.UpdatedAt) > time.Minute {
		t.Error("UpdatedAt should be recent")
	}
}

func TestCreateTestUserWithID(t *testing.T) {
	customID := "custom-user-456"
	user := CreateTestUserWithID(customID)

	if user.ID != customID {
		t.Errorf("Expected ID '%s', got '%s'", customID, user.ID)
	}

	expectedGoogleID := "google-" + customID
	if user.GoogleID == nil || *user.GoogleID != expectedGoogleID {
		t.Errorf("Expected GoogleID 'google-%s', got %v", customID, user.GoogleID)
	}

	// Other fields should still have defaults
	if user.Email != "test@example.com" {
		t.Errorf("Expected default email, got '%s'", user.Email)
	}
}

func TestCreateTestUserWithEmail(t *testing.T) {
	customEmail := "custom@test.com"
	user := CreateTestUserWithEmail(customEmail)

	if user.Email != customEmail {
		t.Errorf("Expected email '%s', got '%s'", customEmail, user.Email)
	}

	// Other fields should still have defaults
	if user.ID != "test-user-123" {
		t.Errorf("Expected default ID, got '%s'", user.ID)
	}
}

func TestCreateTestAPIKey(t *testing.T) {
	userID := "test-user-123"
	apiKey, fullKey, err := CreateTestAPIKey(userID)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if apiKey == nil {
		t.Fatal("Expected non-nil API key")
		return
	}

	if apiKey.UserID != userID {
		t.Errorf("Expected UserID '%s', got '%s'", userID, apiKey.UserID)
	}

	if apiKey.Name != "Test API Key" {
		t.Errorf("Expected name 'Test API Key', got '%s'", apiKey.Name)
	}

	if fullKey == "" {
		t.Fatal("Expected non-empty full key")
	}

	if len(fullKey) != 67 { // "ak_" + 64 hex chars
		t.Errorf("Expected key length 67, got %d", len(fullKey))
	}

	if len(apiKey.KeyPrefix) != 10 { // "ak_" + 7 chars
		t.Errorf("Expected prefix length 10, got %d", len(apiKey.KeyPrefix))
	}

	if apiKey.KeyHash == "" {
		t.Fatal("Expected non-empty key hash")
	}

	if len(apiKey.KeyHash) != 64 { // SHA256 hex
		t.Errorf("Expected hash length 64, got %d", len(apiKey.KeyHash))
	}
}

func TestCreateTestAPIKeyWithName(t *testing.T) {
	userID := "test-user-123"
	customName := "Custom Test Key"
	apiKey, _, err := CreateTestAPIKeyWithName(userID, customName)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if apiKey.Name != customName {
		t.Errorf("Expected name '%s', got '%s'", customName, apiKey.Name)
	}
}

func TestAddAuthHeader(t *testing.T) {
	req, err := http.NewRequest("GET", "/test", nil)
	require.NoError(t, err)
	// #nosec G101 -- Test cookie value, not a real credential
	cookieValue := "test-session-cookie-value"

	AddAuthHeader(req, cookieValue)

	// AddAuthHeader now sets a session cookie
	found := false
	for _, c := range req.Cookies() {
		if c.Name == sesslib.CookieName && c.Value == cookieValue {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected session cookie '%s' to be set", sesslib.CookieName)
	}
}

func TestAddAPIKeyHeader(t *testing.T) {
	req, err := http.NewRequest("GET", "/test", nil)
	require.NoError(t, err)
	apiKey := "ak_1234567890abcdef"

	AddAPIKeyHeader(req, apiKey)

	apiKeyHeader := req.Header.Get("X-API-Key")

	if apiKeyHeader != apiKey {
		t.Errorf("Expected X-API-Key header '%s', got '%s'", apiKey, apiKeyHeader)
	}
}

func TestCreateAuthenticatedRequest(t *testing.T) {
	userID := "test-user-123"
	email := "test@example.com"

	req, err := CreateAuthenticatedRequest("GET", "/api/v1/test", userID, email)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if req.Method != "GET" {
		t.Errorf("Expected method 'GET', got '%s'", req.Method)
	}

	if req.URL.Path != "/api/v1/test" {
		t.Errorf("Expected path '/api/v1/test', got '%s'", req.URL.Path)
	}

	// Should have session cookie
	found := false
	for _, c := range req.Cookies() {
		if c.Name == sesslib.CookieName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected session cookie '%s' to be set", sesslib.CookieName)
	}
}

func TestCreateAuthenticatedRequestWithBody(t *testing.T) {
	userID := "test-user-123"
	email := "test@example.com"
	body := map[string]string{"name": "test"}

	req, err := CreateAuthenticatedRequestWithBody("POST", "/api/v1/test", userID, email, body)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if req.Method != "POST" {
		t.Errorf("Expected method 'POST', got '%s'", req.Method)
	}

	// Should have session cookie
	found := false
	for _, c := range req.Cookies() {
		if c.Name == sesslib.CookieName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected session cookie '%s' to be set", sesslib.CookieName)
	}

	// Should have JSON content type
	contentType := req.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}

	// Body should be set
	if req.Body == nil {
		t.Fatal("Expected request body to be set")
	}
}

func TestCreateAPIKeyAuthenticatedRequest(t *testing.T) {
	apiKey := "ak_1234567890abcdef"

	req, err := CreateAPIKeyAuthenticatedRequest("GET", "/api/v1/test", apiKey)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	apiKeyHeader := req.Header.Get("X-API-Key")
	if apiKeyHeader != apiKey {
		t.Errorf("Expected X-API-Key header '%s', got '%s'", apiKey, apiKeyHeader)
	}
}

func TestMockContext(t *testing.T) {
	userID := "test-user-123"
	ctx := MockContext(userID)

	if ctx == nil {
		t.Fatal("Expected non-nil context")
	}

	// Verify user ID is in context
	ctxUserID := ctx.Value(contextKeyUserID)
	if ctxUserID != userID {
		t.Errorf("Expected userID '%s' in context, got %v", userID, ctxUserID)
	}
}

func TestMockContextWithUser(t *testing.T) {
	user := CreateTestUser()
	ctx := MockContextWithUser(user)

	if ctx == nil {
		t.Fatal("Expected non-nil context")
	}

	// Verify user ID is in context
	ctxUserID := ctx.Value(contextKeyUserID)
	if ctxUserID != user.ID {
		t.Errorf("Expected userID '%s' in context, got %v", user.ID, ctxUserID)
	}

	// Verify user object is in context
	ctxUser := ctx.Value(contextKeyUser)
	if ctxUser != user {
		t.Error("Expected user object in context")
	}
}

func TestGetTestUserCredentials(t *testing.T) {
	userID, email := GetTestUserCredentials()

	if userID != "test-user-123" {
		t.Errorf("Expected userID 'test-user-123', got '%s'", userID)
	}

	if email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%s'", email)
	}
}

func TestCreateTestJWTForDefaultUser(t *testing.T) {
	token, err := CreateTestJWTForDefaultUser()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if token == "" {
		t.Fatal("Expected non-empty session cookie value")
	}

	// Verify the cookie value can be decrypted
	mgr, err := sesslib.NewManager(TestSessionCookiePassword, true)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	// Build a fake request with the cookie
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sesslib.CookieName, Value: token})

	sess, err := mgr.Read(req)
	if err != nil {
		t.Fatalf("Session should be valid: %v", err)
	}

	expectedUserID, _ := GetTestUserCredentials()
	if sess.UserID != expectedUserID {
		t.Errorf("Expected UserID '%s', got '%s'", expectedUserID, sess.UserID)
	}
}
