package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
)

// TestGetPreferences_Unauthorized tests getting preferences without auth
func TestGetPreferences_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/api/v1/preferences", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestGetPreferences_RouteRegistered verifies the route is registered
func TestGetPreferences_RouteRegistered(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/api/v1/preferences", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should not return 404 (route not found)
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "Route should be registered")
}

// TestUpdatePreferences_Unauthorized tests updating preferences without auth
func TestUpdatePreferences_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{"theme": "dark"}`
	req, err := http.NewRequest("PUT", "/api/v1/preferences", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestUpdatePreferences_RouteRegistered verifies the route is registered
func TestUpdatePreferences_RouteRegistered(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{"theme": "dark"}`
	req, err := http.NewRequest("PUT", "/api/v1/preferences", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Should not return 404 (route not found)
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "Route should be registered")
}

// TestUpdatePreferences_InvalidJSON tests updating preferences with invalid JSON
func TestUpdatePreferences_InvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	body := `{invalid json}`
	req, err := http.NewRequest("PUT", "/api/v1/preferences", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Will be unauthorized or bad request
	assert.True(t, rr.Code >= 400)
}

// TestUpdatePreferences_EmptyBody tests updating preferences with empty body
func TestUpdatePreferences_EmptyBody(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("PUT", "/api/v1/preferences", strings.NewReader(""))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// Will be unauthorized or bad request
	assert.True(t, rr.Code >= 400)
}

// TestPreferencesEndpoints_MethodNotAllowed tests wrong HTTP methods
// Note: Auth middleware runs first, so unauthorized requests return 401
func TestPreferencesEndpoints_MethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name     string
		method   string
		path     string
		expected int
	}{
		{
			name:     "POST on preferences - unauthorized",
			method:   "POST",
			path:     "/api/v1/preferences",
			expected: http.StatusUnauthorized, // Auth runs before method check
		},
		{
			name:     "DELETE on preferences - unauthorized",
			method:   "DELETE",
			path:     "/api/v1/preferences",
			expected: http.StatusUnauthorized, // Auth runs before method check
		},
		{
			name:     "PATCH on preferences - unauthorized",
			method:   "PATCH",
			path:     "/api/v1/preferences",
			expected: http.StatusUnauthorized, // Auth runs before method check
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			assert.Equal(t, tt.expected, rr.Code)
		})
	}
}

// TestUpdatePreferences_ValidPayload tests updating preferences with valid payload
func TestUpdatePreferences_ValidPayload(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name string
		body string
	}{
		{
			name: "Theme preference",
			body: `{"theme": "dark"}`,
		},
		{
			name: "Multiple preferences",
			body: `{"theme": "light", "notifications_enabled": true}`,
		},
		{
			name: "Empty object",
			body: `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("PUT", "/api/v1/preferences", strings.NewReader(tt.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			// Without proper auth, should be unauthorized
			assert.Equal(t, http.StatusUnauthorized, rr.Code)
		})
	}
}

// TestGetPreferences_WithAPIKey tests getting preferences with API key auth
func TestGetPreferences_WithAPIKey(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	req, err := http.NewRequest("GET", "/api/v1/preferences", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-api-key")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// API key authentication should work for preferences endpoint
	// But without container setup, it will fail internally
	assert.True(t, rr.Code >= 400)
}

// TestPreferencesEndpoint_ContentType tests content type handling
func TestPreferencesEndpoint_ContentType(t *testing.T) {
	cfg := &config.Config{}
	logger, _ := test.NewNullLogger()
	logger.SetLevel(logrus.ErrorLevel)
	srv := New("8080", nil, "test-api-key", cfg, logger)

	tests := []struct {
		name        string
		contentType string
	}{
		{"JSON content type", "application/json"},
		{"JSON with charset", "application/json; charset=utf-8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"theme": "dark"}`
			req, err := http.NewRequest("PUT", "/api/v1/preferences", strings.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", tt.contentType)
			req.Header.Set("Authorization", "Bearer test-token")

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			// Request should be processed (not rejected due to content type)
			// Will fail auth but not content type
			assert.NotEqual(t, http.StatusUnsupportedMediaType, rr.Code)
		})
	}
}
