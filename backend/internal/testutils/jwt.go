// Package testutils provides helper utilities for tests.
// JWT-based authentication has been replaced by AES-GCM encrypted session
// cookies. This file retains helper function names for backwards
// compatibility but now generates dev session cookies instead of HS256 JWTs.
package testutils

import (
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"time"

	sesslib "github.com/vibexp/vibexp/internal/auth/session"
)

// TestSessionCookiePassword is a 32-byte AES-256-GCM key used in tests.
// It must be a hex string encoding exactly 32 bytes (64 hex chars).
// #nosec G101 - Test-only credential, not a real secret
const TestSessionCookiePassword = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

// JWTTestOptions retains the same struct for test API compatibility.
// ExpiresAt controls the session expiry.
type JWTTestOptions struct {
	ExpiresAt *time.Time
	IssuedAt  *time.Time
	NotBefore *time.Time
}

// GenerateTestJWT creates a session cookie value (not a JWT) for testing.
// The returned string is the encrypted cookie value for the vx_session cookie.
func GenerateTestJWT(userID, email string) (string, error) {
	return GenerateTestJWTWithOptions(userID, email, nil)
}

// GenerateTestJWTWithOptions creates a session cookie value with custom options.
func GenerateTestJWTWithOptions(userID, email string, options *JWTTestOptions) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("userID cannot be empty")
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	if options != nil && options.ExpiresAt != nil {
		expiresAt = *options.ExpiresAt
	}

	mgr, err := sesslib.NewManager(TestSessionCookiePassword, true /* local/test */)
	if err != nil {
		return "", fmt.Errorf("failed to create session manager: %w", err)
	}

	sess := &sesslib.Session{
		AccessToken:  "test-access-token",
		RefreshToken: "",
		ExpiresAt:    expiresAt,
		IDPSubject:   "test-subject-" + userID,
		UserID:       userID,
	}

	rw := httptest.NewRecorder()
	if err := mgr.Write(rw, sess); err != nil {
		return "", fmt.Errorf("failed to write session: %w", err)
	}

	for _, c := range rw.Result().Cookies() {
		if c.Name == sesslib.CookieName {
			return c.Value, nil
		}
	}

	return "", fmt.Errorf("session cookie not set")
}

// GenerateTestJWTWithClaims creates a session cookie for the given userID and email.
// The JWTClaimsCompat struct is a compatibility shim.
func GenerateTestJWTWithClaims(claims JWTClaimsCompat) (string, error) {
	return GenerateTestJWTWithOptions(claims.UserID, claims.Email, nil)
}

// JWTClaimsCompat is a compatibility shim so test code that used services.JWTClaims
// still compiles after JWTClaims was removed from the services package.
type JWTClaimsCompat struct {
	UserID string
	Email  string
}

// GenerateExpiredTestJWT creates an expired session cookie for testing.
func GenerateExpiredTestJWT(userID, email string) (string, error) {
	expiredTime := time.Now().Add(-1 * time.Hour)
	options := &JWTTestOptions{
		ExpiresAt: &expiredTime,
	}
	return GenerateTestJWTWithOptions(userID, email, options)
}

// GenerateInvalidTestJWT creates an invalid (garbled) session cookie value for testing.
func GenerateInvalidTestJWT(userID, email string) (string, error) {
	return hex.EncodeToString([]byte("invalid.garbage")) + ".deadbeef", nil
}
