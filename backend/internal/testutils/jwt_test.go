package testutils

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sesslib "github.com/vibexp/vibexp/internal/auth/session"
)

func TestGenerateTestJWT(t *testing.T) {
	userID := "test-user-123"
	email := "test@example.com"

	cookieValue, err := GenerateTestJWT(userID, email)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if cookieValue == "" {
		t.Fatal("Expected non-empty cookie value")
	}

	// Verify session cookie can be decrypted and contains correct user ID
	mgr, err := sesslib.NewManager(TestSessionCookiePassword, true)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sesslib.CookieName, Value: cookieValue})

	sess, err := mgr.Read(req)
	if err != nil {
		t.Fatalf("Failed to read session: %v", err)
	}

	if sess.UserID != userID {
		t.Errorf("Expected UserID %s, got %s", userID, sess.UserID)
	}
}

func TestGenerateTestJWTWithOptions(t *testing.T) {
	userID := "test-user-123"
	email := "test@example.com"

	// Test with custom expiration (future)
	customExpiry := time.Now().Add(2 * time.Hour)
	options := &JWTTestOptions{
		ExpiresAt: &customExpiry,
	}

	cookieValue, err := GenerateTestJWTWithOptions(userID, email, options)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Parse session and verify expiry
	mgr, err := sesslib.NewManager(TestSessionCookiePassword, true)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sesslib.CookieName, Value: cookieValue})

	sess, err := mgr.Read(req)
	if err != nil {
		t.Fatalf("Failed to read session: %v", err)
	}

	// Session should not be expired
	if sess.IsExpired() {
		t.Error("Session should not be expired")
	}

	// Expiry should be close to our custom time (within 1 second tolerance)
	diff := sess.ExpiresAt.Sub(customExpiry)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Second {
		t.Errorf("Expected expiry close to %v, got %v (diff: %v)", customExpiry, sess.ExpiresAt, diff)
	}
}

func TestGenerateTestJWTWithClaims(t *testing.T) {
	customClaims := JWTClaimsCompat{
		UserID: "custom-user",
		Email:  "custom@example.com",
	}

	cookieValue, err := GenerateTestJWTWithClaims(customClaims)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Parse session and verify custom claims
	mgr, err := sesslib.NewManager(TestSessionCookiePassword, true)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sesslib.CookieName, Value: cookieValue})

	sess, err := mgr.Read(req)
	if err != nil {
		t.Fatalf("Failed to read session: %v", err)
	}

	if sess.UserID != customClaims.UserID {
		t.Errorf("Expected UserID %s, got %s", customClaims.UserID, sess.UserID)
	}
}

func TestGenerateExpiredTestJWT(t *testing.T) {
	userID := "test-user-123"
	email := "test@example.com"

	cookieValue, err := GenerateExpiredTestJWT(userID, email)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Decode session and check it's expired
	mgr, err := sesslib.NewManager(TestSessionCookiePassword, true)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sesslib.CookieName, Value: cookieValue})

	sess, err := mgr.Read(req)
	if err != nil {
		t.Fatalf("Failed to read session: %v", err)
	}

	// Session should be expired
	if !sess.IsExpired() {
		t.Error("Expected session to be expired")
	}
}

func TestGenerateInvalidTestJWT(t *testing.T) {
	userID := "test-user-123"
	email := "test@example.com"

	token, err := GenerateInvalidTestJWT(userID, email)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Try to decrypt with real session manager - should fail
	mgr, err := sesslib.NewManager(TestSessionCookiePassword, true)
	if err != nil {
		t.Fatalf("Failed to create session manager: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sesslib.CookieName, Value: token})

	_, readErr := mgr.Read(req)
	if readErr == nil {
		t.Fatal("Expected invalid session cookie to fail decryption")
	}
}
