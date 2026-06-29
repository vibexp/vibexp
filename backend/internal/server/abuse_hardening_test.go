package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
)

// TestMaxBodySizeMiddleware_DefaultFallback verifies that a non-positive limit falls
// back to the 1MiB default rather than rejecting every request.
func TestMaxBodySizeMiddleware_DefaultFallback(t *testing.T) {
	handlerCalled := false
	h := maxBodySize(0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("small body"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.True(t, handlerCalled, "small body must pass through the default-fallback limiter")
	assert.Equal(t, http.StatusOK, rr.Code)
}

// TestRateLimit_AuthRoute_Returns429AfterThreshold builds a server with a strict
// auth rate limit and confirms requests beyond the per-IP threshold get 429.
func TestRateLimit_AuthRoute_Returns429AfterThreshold(t *testing.T) {
	const limit = 3
	srv := testServerWithConfig(&config.Config{AuthRateLimitPerMinute: limit})

	var sawTooMany bool
	for i := 0; i < limit+2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)
		if rr.Code == http.StatusTooManyRequests {
			sawTooMany = true
		}
	}

	assert.True(t, sawTooMany, "requests beyond the auth rate limit must return 429")
}

// TestRateLimit_Disabled_WhenLimitZero confirms a zero limit disables the limiter so
// many requests from one IP are never throttled.
func TestRateLimit_Disabled_WhenLimitZero(t *testing.T) {
	srv := testServerWithConfig(&config.Config{AuthRateLimitPerMinute: 0})

	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)
		require.NotEqual(t, http.StatusTooManyRequests, rr.Code,
			"limiter must be disabled when the configured limit is 0")
	}
}

// TestServerTimeouts_Constants locks in the hardened server timeout values so a
// regression that loosens them is caught.
func TestServerTimeouts_Constants(t *testing.T) {
	assert.Equal(t, serverReadHeaderTimeoutSeconds, int(serverReadHeaderTimeout.Seconds()))
	assert.Equal(t, 30, int(serverReadTimeout.Seconds()))
	assert.Equal(t, 60, int(serverWriteTimeout.Seconds()))
	assert.Equal(t, 120, int(serverIdleTimeout.Seconds()))
}

const serverReadHeaderTimeoutSeconds = 10
