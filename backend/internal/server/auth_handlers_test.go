package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/config"
)

func TestAuthLogin_Endpoints(t *testing.T) {
	cfg := &config.Config{
		Frontend: config.FrontendConfig{BaseURL: "http://localhost:5173"},
		Auth:     config.AuthConfig{SessionEncryptionKey: testCookiePassword},
	}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		// Login returns 503 (Service Unavailable) when identity provider not configured (stub/local dev)
		{"Login - GET returns 503 when IDP not configured", "GET", "/api/v1/auth/login", http.StatusServiceUnavailable},
		// POST to login should return method not allowed
		{"Login - Method Not Allowed", "POST", "/api/v1/auth/login", http.StatusMethodNotAllowed},
		// Callback without code returns 400
		{"Callback without code - Bad Request", "GET", "/api/v1/auth/callback", http.StatusBadRequest},
		// GET /auth/me without auth returns 401
		{"Get Me - Unauthorized", "GET", "/api/v1/auth/me", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v (body: %s)",
					status, tt.expected, rr.Body.String())
			}
		})
	}
}

func TestDevLogin_BadRequest(t *testing.T) {
	cfg := &config.Config{
		Frontend: config.FrontendConfig{BaseURL: "http://localhost:5173"},
		Auth:     config.AuthConfig{SessionEncryptionKey: testCookiePassword, DevLoginEnabled: true},
	}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		expected int
	}{
		{
			"Dev Login - Invalid JSON",
			"POST",
			"/api/v1/auth/dev/login",
			`{"invalid": json}`,
			http.StatusBadRequest,
		},
		{
			"Dev Login - Missing Email",
			"POST",
			"/api/v1/auth/dev/login",
			`{"name":"Test User"}`,
			http.StatusBadRequest,
		},
		{
			"Dev Login - Empty Email",
			"POST",
			"/api/v1/auth/dev/login",
			`{"email":"","name":"Test User"}`,
			http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader(tt.body)
			req, err := http.NewRequest(tt.method, tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestFlexibleAuthMiddleware_MissingAuth(t *testing.T) {
	cfg := &config.Config{
		Frontend: config.FrontendConfig{BaseURL: "http://localhost:5173"},
		Auth:     config.AuthConfig{SessionEncryptionKey: testCookiePassword},
	}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Protected endpoint without auth", "GET", "/api/v1/auth/me", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestFlexibleAuthMiddleware_InvalidHeader(t *testing.T) {
	cfg := &config.Config{
		Frontend: config.FrontendConfig{BaseURL: "http://localhost:5173"},
		Auth:     config.AuthConfig{SessionEncryptionKey: testCookiePassword},
	}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name          string
		method        string
		path          string
		authorization string
		expected      int
	}{
		{"Invalid Bearer format", "GET", "/api/v1/auth/me", "InvalidBearer token", http.StatusUnauthorized},
		{"Missing Bearer prefix", "GET", "/api/v1/auth/me", "token-without-bearer-prefix", http.StatusUnauthorized},
		// Non-API-key Bearer tokens are rejected
		{"Non-API-key Bearer token", "GET", "/api/v1/auth/me", "Bearer not-an-api-key", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", tt.authorization)

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestAuthHandlers_InvalidPaths(t *testing.T) {
	cfg := &config.Config{
		Frontend: config.FrontendConfig{BaseURL: "http://localhost:5173"},
		Auth:     config.AuthConfig{SessionEncryptionKey: testCookiePassword},
	}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{"Invalid auth path", "GET", "/api/v1/auth/invalid", http.StatusNotFound},
		{"Method not allowed on /me", "PATCH", "/api/v1/auth/me", http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expected {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expected)
			}
		})
	}
}

func TestLogout_ClearsSessionCookie(t *testing.T) {
	cfg := &config.Config{
		Frontend: config.FrontendConfig{BaseURL: "http://localhost:5173"},
		Auth:     config.AuthConfig{SessionEncryptionKey: testCookiePassword},
	}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("POST", "/api/v1/auth/logout", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify session cookie was cleared
	var sessionCookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == sesslib.CookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Error("Expected Set-Cookie header for vx_session")
		return
	}
	if sessionCookie.MaxAge > 0 {
		t.Errorf("Expected MaxAge <= 0, got %d", sessionCookie.MaxAge)
	}
}

func TestStateSigningAndValidation(t *testing.T) {
	cfg := &config.Config{
		Frontend: config.FrontendConfig{BaseURL: "http://localhost:5173"},
		Auth:     config.AuthConfig{SessionEncryptionKey: testCookiePassword},
	}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	// Extract the Server from the http.Handler (it implements ServeHTTP)
	server := srv

	state := "test-state-value"
	signed := server.signState(state, "github")

	if signed == "" {
		t.Fatal("Expected non-empty signed state")
	}

	// Create a fake request with the signed state cookie and the original state param
	req := httptest.NewRequest("GET", "/?state="+state, nil)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: signed})

	provider, err := server.validateStateCookie(req, state)
	if err != nil {
		t.Errorf("Expected valid state, got error: %v", err)
	}
	if provider != "github" {
		t.Errorf("Expected provider 'github' recovered from cookie, got %q", provider)
	}

	// Tampered state should fail
	req2 := httptest.NewRequest("GET", "/?state=tampered", nil)
	req2.AddCookie(&http.Cookie{Name: stateCookieName, Value: signed})

	if _, err := server.validateStateCookie(req2, "tampered"); err == nil {
		t.Error("Expected error for tampered state")
	}
}

// TestSessionCookieAuth_EndToEnd verifies session cookie authentication end-to-end
func TestSessionCookieAuth_EndToEnd(t *testing.T) {
	cfg := &config.Config{
		Frontend: config.FrontendConfig{BaseURL: "http://localhost:5173"},
		Auth:     config.AuthConfig{SessionEncryptionKey: testCookiePassword},
	}
	logger := slog.New(slog.DiscardHandler)
	srv := New("8080", nil, "test-api-key", cfg, logger)
	server := srv

	// Build a valid session cookie
	mgr, err := sesslib.NewManager(testCookiePassword, true)
	if err != nil {
		t.Fatal(err)
	}
	sess := &sesslib.Session{
		AccessToken: "valid-access-token",
		ExpiresAt:   time.Now().Add(time.Hour),
		IDPSubject:  "oidc-sub-123",
		UserID:      "test-user-123",
	}
	rw := httptest.NewRecorder()
	if err := mgr.Write(rw, sess); err != nil {
		t.Fatal(err)
	}

	var sessionCookie *http.Cookie
	for _, c := range rw.Result().Cookies() {
		if c.Name == sesslib.CookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("Session cookie not set")
	}

	// Request /api/v1/auth/me with the session cookie.
	// With no container, the handler panics after auth succeeds (nil UserRepository).
	// The panic recoverer returns 500. 401 means auth failed; 500 means auth passed.
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	// 401 = auth failed (cookie rejected); anything else = auth middleware passed
	if w.Code == http.StatusUnauthorized {
		t.Errorf("Expected auth to succeed (non-401), got %d: %s", w.Code, w.Body.String())
	}
	// 500 is expected here because container is nil (no DB in unit test)
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
		t.Logf("Got status %d (expected 500 or 404, auth passed)", w.Code)
	}
}
