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

// TestMaxBodySize_GlobalCap verifies the global maxBodySize middleware rejects an
// oversized request body with 413 on the contact route, which surfaces the
// MaxBytesError as Request Entity Too Large.
func TestMaxBodySize_ContactRoute_RejectsOversizedBody(t *testing.T) {
	srv := testServer()

	// contactMaxBodyBytes is 64KiB; build a body comfortably over it.
	oversized := `{"name":"John Doe","email":"john@example.com","message":"` +
		strings.Repeat("a", 70*1024) + `"}`

	rr := makeRequest(t, srv, testRequest{
		Method: "POST",
		Path:   "/api/v1/website/contact/send-message",
		Body:   oversized,
	})

	assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code,
		"oversized contact body must be rejected with 413")
}

// TestMaxBodySize_WithinLimit confirms a normal-sized body is accepted (not throttled
// by the body cap).
func TestMaxBodySize_ContactRoute_WithinLimit(t *testing.T) {
	srv := testServer()

	body := `{"name":"John Doe","email":"john@example.com",` +
		`"message":"This is a normal contact message well within the size cap."}`

	rr := makeRequest(t, srv, testRequest{
		Method: "POST",
		Path:   "/api/v1/website/contact/send-message",
		Body:   body,
	})

	assert.Equal(t, http.StatusOK, rr.Code, "a within-limit body must be accepted")
}

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

// TestRateLimit_ContactRoute_Returns429AfterThreshold builds a server with a strict
// contact rate limit and confirms requests beyond the per-IP threshold get 429.
func TestRateLimit_ContactRoute_Returns429AfterThreshold(t *testing.T) {
	const limit = 3
	srv := testServerWithConfig(&config.Config{ContactRateLimitPerMinute: limit})

	body := `{"name":"John Doe","email":"john@example.com",` +
		`"message":"This is a normal contact message within the size cap."}`

	var sawTooMany bool
	// Fire more than the limit from the same client IP (httptest uses a fixed
	// RemoteAddr, so RealIP keys them together).
	for i := 0; i < limit+2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/website/contact/send-message",
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)
		if rr.Code == http.StatusTooManyRequests {
			sawTooMany = true
		}
	}

	assert.True(t, sawTooMany, "requests beyond the contact rate limit must return 429")
}

// TestRateLimit_Disabled_WhenLimitZero confirms a zero limit disables the limiter so
// many requests from one IP are never throttled.
func TestRateLimit_Disabled_WhenLimitZero(t *testing.T) {
	srv := testServerWithConfig(&config.Config{ContactRateLimitPerMinute: 0})

	body := `{"name":"John Doe","email":"john@example.com",` +
		`"message":"This is a normal contact message within the size cap."}`

	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/website/contact/send-message",
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
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
