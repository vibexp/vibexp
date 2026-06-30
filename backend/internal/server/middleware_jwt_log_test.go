package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/contextkeys"
	svcmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// newServerWithMockAuthSvc builds a minimal Server wired to the given mock auth service.
func newServerWithMockAuthSvc(
	t *testing.T,
	authSvc *svcmocks.MockAuthServiceInterface,
) (*Server, *slog.Logger) {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)

	// MockAuthContainer is defined in auth_handlers_integration_test.go
	// and already embeds BaseMockContainer + exposes AuthService().
	ctr := &MockAuthContainer{
		authService: authSvc,
	}

	// Build a real session manager for the test
	// #nosec G101 - test credential
	sessMgr, err := sesslib.NewManager(testCookiePassword, true)
	require.NoError(t, err)

	srv := &Server{
		port:           "8080",
		container:      ctr,
		logger:         logger,
		config:         &config.Config{Auth: config.AuthConfig{SessionEncryptionKey: testCookiePassword}},
		sessionManager: sessMgr,
		router:         chi.NewRouter(),
	}

	return srv, logger
}

// requestWithLogEntry returns a request that carries a logger in its context
// so that contextkeys.GetLoggerFromContext picks it up.
func requestWithLogEntry(logger *slog.Logger) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), contextkeys.Logger, logger)
	return req.WithContext(ctx)
}

// buildTestSession creates a valid encrypted session cookie value for a test user.
func buildTestSession(t *testing.T, userID string, expiresIn time.Duration) string {
	t.Helper()
	// #nosec G101 - test credential
	mgr, err := sesslib.NewManager(testCookiePassword, true)
	require.NoError(t, err)

	sess := &sesslib.Session{
		AccessToken: "test-access-token",
		ExpiresAt:   time.Now().Add(expiresIn),
		IDPSubject:  "oidc-sub-" + userID,
		UserID:      userID,
	}

	rw := httptest.NewRecorder()
	require.NoError(t, mgr.Write(rw, sess))

	for _, c := range rw.Result().Cookies() {
		if c.Name == sesslib.CookieName {
			return c.Value
		}
	}
	t.Fatal("session cookie not set")
	return ""
}

// TestAuthenticateWithSession_ValidSession verifies that a valid session cookie
// sets the user context and calls next.
func TestAuthenticateWithSession_ValidSession(t *testing.T) {
	mockAuthSvc := svcmocks.NewMockAuthServiceInterface(t)
	srv, logger := newServerWithMockAuthSvc(t, mockAuthSvc)

	validCookieValue := buildTestSession(t, "user-test", time.Hour)

	req := requestWithLogEntry(logger)
	req.AddCookie(&http.Cookie{Name: sesslib.CookieName, Value: validCookieValue})
	w := httptest.NewRecorder()

	var capturedUserID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = r.Context().Value(contextkeys.UserID).(string)
		w.WriteHeader(http.StatusOK)
	})

	srv.authenticateWithSession(w, req, next)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "user-test", capturedUserID)

	mockAuthSvc.AssertExpectations(t)
}

// TestAuthenticateWithSession_NoCookie verifies that a missing session cookie
// results in 401.
func TestAuthenticateWithSession_NoCookie(t *testing.T) {
	mockAuthSvc := svcmocks.NewMockAuthServiceInterface(t)
	srv, logger := newServerWithMockAuthSvc(t, mockAuthSvc)

	req := requestWithLogEntry(logger)
	w := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called without session cookie")
	})

	srv.authenticateWithSession(w, req, next)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	mockAuthSvc.AssertExpectations(t)
}

// TestAuthenticateWithSession_ExpiredToken verifies that an expired session
// (with no refresh token) results in 401.
func TestAuthenticateWithSession_ExpiredToken(t *testing.T) {
	mockAuthSvc := svcmocks.NewMockAuthServiceInterface(t)
	srv, logger := newServerWithMockAuthSvc(t, mockAuthSvc)

	// Build a session with an access token that expired 1 hour ago and no refresh token
	expiredCookieValue := buildTestSession(t, "user-expired", -time.Hour)

	req := requestWithLogEntry(logger)
	req.AddCookie(&http.Cookie{Name: sesslib.CookieName, Value: expiredCookieValue})
	w := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with expired session and no refresh token")
	})

	srv.authenticateWithSession(w, req, next)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	mockAuthSvc.AssertExpectations(t)
}

// TestAuthenticateWithSession_TamperedCookie verifies that a cookie with an
// invalid ciphertext is rejected with 401.
func TestAuthenticateWithSession_TamperedCookie(t *testing.T) {
	mockAuthSvc := svcmocks.NewMockAuthServiceInterface(t)
	srv, logger := newServerWithMockAuthSvc(t, mockAuthSvc)

	req := requestWithLogEntry(logger)
	// #nosec G101 - test invalid cookie value
	req.AddCookie(&http.Cookie{Name: sesslib.CookieName, Value: "deadbeef.invalidciphertext"})
	w := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with tampered cookie")
	})

	srv.authenticateWithSession(w, req, next)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	mockAuthSvc.AssertExpectations(t)
}
