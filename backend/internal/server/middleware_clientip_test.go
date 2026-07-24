package server

import (
	"bytes"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/httprate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression tests for #465. middleware.RealIP overwrote RemoteAddr from
// client-supplied X-Forwarded-For / X-Real-IP with no notion of which peers may
// assert them, so a directly-reachable instance's per-IP rate limiter could be
// defeated by rotating a header.

// mustCIDRs parses trusted-proxy CIDRs for a test.
func mustCIDRs(t *testing.T, entries ...string) []*net.IPNet {
	t.Helper()
	nets := make([]*net.IPNet, 0, len(entries))
	for _, e := range entries {
		_, n, err := net.ParseCIDR(e)
		require.NoError(t, err, "bad test CIDR %q", e)
		nets = append(nets, n)
	}
	return nets
}

func TestResolveClientIP(t *testing.T) {
	tests := []struct {
		name          string
		peer          string
		trusted       []string
		xForwardedFor string
		xRealIP       string
		want          string
	}{
		{
			name:          "untrusted peer: spoofed header ignored",
			peer:          "203.0.113.9:4444",
			xForwardedFor: "1.2.3.4",
			want:          "203.0.113.9",
		},
		{
			name:          "no trusted proxies configured: header ignored even from a private peer",
			peer:          "10.0.0.5:4444",
			xForwardedFor: "1.2.3.4",
			want:          "10.0.0.5",
		},
		{
			name:          "trusted peer, single hop",
			peer:          "10.0.0.5:4444",
			trusted:       []string{"10.0.0.0/8"},
			xForwardedFor: "203.0.113.7",
			want:          "203.0.113.7",
		},
		{
			// The classic bypass: the attacker prepends whatever they like. Reading
			// left-to-right (the old getClientIP) would return the spoofed value.
			name:          "trusted peer, multi-hop: right-most untrusted hop wins",
			peer:          "10.0.0.5:4444",
			trusted:       []string{"10.0.0.0/8"},
			xForwardedFor: "1.2.3.4, 203.0.113.7, 10.0.0.9",
			want:          "203.0.113.7",
		},
		{
			name:          "trusted peer, whole chain trusted: falls back to X-Real-IP",
			peer:          "10.0.0.5:4444",
			trusted:       []string{"10.0.0.0/8"},
			xForwardedFor: "10.0.0.8, 10.0.0.9",
			xRealIP:       "203.0.113.7",
			want:          "203.0.113.7",
		},
		{
			name:    "trusted peer, no XFF: falls back to X-Real-IP",
			peer:    "10.0.0.5:4444",
			trusted: []string{"10.0.0.0/8"},
			xRealIP: "203.0.113.7",
			want:    "203.0.113.7",
		},
		{
			name:    "trusted peer, no headers at all: peer wins",
			peer:    "10.0.0.5:4444",
			trusted: []string{"10.0.0.0/8"},
			want:    "10.0.0.5",
		},
		{
			name:          "trusted peer, malformed hop: falls back rather than trusting it",
			peer:          "10.0.0.5:4444",
			trusted:       []string{"10.0.0.0/8"},
			xForwardedFor: "not-an-ip",
			want:          "10.0.0.5",
		},
		{
			name:          "trusted peer, malformed right-most hop stops the walk",
			peer:          "10.0.0.5:4444",
			trusted:       []string{"10.0.0.0/8"},
			xForwardedFor: "203.0.113.7, garbage",
			want:          "10.0.0.5",
		},
		{
			name:          "IPv6 peer, untrusted",
			peer:          "[2001:db8::1]:4444",
			xForwardedFor: "1.2.3.4",
			want:          "2001:db8::1",
		},
		{
			name:          "IPv6 hop from a trusted peer",
			peer:          "10.0.0.5:4444",
			trusted:       []string{"10.0.0.0/8"},
			xForwardedFor: "2001:db8::7",
			want:          "2001:db8::7",
		},
		{
			name:          "bracketed IPv6 hop with port",
			peer:          "10.0.0.5:4444",
			trusted:       []string{"10.0.0.0/8"},
			xForwardedFor: "[2001:db8::7]:9999",
			want:          "2001:db8::7",
		},
		{
			name:          "IPv4 hop carrying a port",
			peer:          "10.0.0.5:4444",
			trusted:       []string{"10.0.0.0/8"},
			xForwardedFor: "203.0.113.7:9999",
			want:          "203.0.113.7",
		},
		{
			name:          "trusted /32 proxy",
			peer:          "192.168.1.5:4444",
			trusted:       []string{"192.168.1.5/32"},
			xForwardedFor: "203.0.113.7",
			want:          "203.0.113.7",
		},
		{
			name:          "peer just outside the trusted CIDR",
			peer:          "192.168.1.6:4444",
			trusted:       []string{"192.168.1.5/32"},
			xForwardedFor: "203.0.113.7",
			want:          "192.168.1.6",
		},
		{
			name:          "empty XFF value from a trusted peer",
			peer:          "10.0.0.5:4444",
			trusted:       []string{"10.0.0.0/8"},
			xForwardedFor: "   ",
			want:          "10.0.0.5",
		},
		{
			name:    "peer without a port",
			peer:    "203.0.113.9",
			trusted: []string{"10.0.0.0/8"},
			want:    "203.0.113.9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.peer
			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}

			got := resolveClientIP(req, mustCIDRs(t, tt.trusted...))

			assert.Equal(t, tt.want, got)
		})
	}
}

// TestClientIPMiddleware_SetsContextAndRemoteAddr pins the two things
// downstream code depends on: the context value (read by clientIP) and the
// rewritten RemoteAddr (read by httprate).
func TestClientIPMiddleware_SetsContextAndRemoteAddr(t *testing.T) {
	var seenClientIP, seenRemoteAddr string

	handler := clientIPMiddleware(mustCIDRs(t, "10.0.0.0/8"))(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			seenClientIP = clientIP(r)
			seenRemoteAddr = r.RemoteAddr
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:4444"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")

	handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, "203.0.113.7", seenClientIP)
	host, _, err := net.SplitHostPort(seenRemoteAddr)
	require.NoError(t, err, "RemoteAddr must stay host:port for httprate")
	assert.Equal(t, "203.0.113.7", host)
}

// TestRateLimiterNotBypassableByRotatingXFF reproduces the audit's PoC. It
// drives the production chain (client-IP resolution -> httprate.LimitByIP) from
// a single peer and asserts that rotating X-Forwarded-For gives no advantage
// over sending no header at all.
//
// The audit measured 15/20 throttled without the header and 0/200 with it. The
// two must now converge.
func TestRateLimiterNotBypassableByRotatingXFF(t *testing.T) {
	const limit = 5
	const requests = 20

	// countThrottled drives `requests` requests from one peer through a fresh
	// limiter, optionally rotating X-Forwarded-For, and returns the 429 count.
	countThrottled := func(rotateHeader bool) int {
		r := chiRouterWithLimiter(limit)
		throttled := 0
		for i := range requests {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "203.0.113.9:4444" // one TCP peer throughout
			if rotateHeader {
				req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i%250))
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code == http.StatusTooManyRequests {
				throttled++
			}
		}
		return throttled
	}

	baseline := countThrottled(false)
	withRotation := countThrottled(true)

	require.Positive(t, baseline, "the limiter must throttle a single peer at all")
	assert.Equal(t, baseline, withRotation,
		"rotating X-Forwarded-For must give no advantage over sending none")
}

// chiRouterWithLimiter builds the production middleware order: client-IP
// resolution with NO trusted proxies (the default), then the per-IP limiter.
func chiRouterWithLimiter(limit int) http.Handler {
	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h = httprate.LimitByIP(limit, time.Minute)(h)
	h = clientIPMiddleware(nil)(h)
	return h
}

// TestWarnIfProxyHeadersIgnored covers the startup hint that tells an operator
// behind a proxy why every request looks like it comes from one address.
func TestWarnIfProxyHeadersIgnored(t *testing.T) {
	tests := []struct {
		name         string
		trustedCount int
		localDev     bool
		wantWarn     bool
	}{
		{name: "production, none configured", trustedCount: 0, localDev: false, wantWarn: true},
		{name: "production, configured", trustedCount: 1, localDev: false, wantWarn: false},
		{name: "local development", trustedCount: 0, localDev: true, wantWarn: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, logs := newCapturingLogger()

			warnIfProxyHeadersIgnored(logger, tt.trustedCount, tt.localDev)

			if tt.wantWarn {
				assert.Contains(t, logs.String(), "trusted_proxies")
				return
			}
			assert.Empty(t, logs.String())
		})
	}
}

// newCapturingLogger returns a logger writing into the returned buffer, so a
// test can assert on what was logged.
func newCapturingLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn})), buf
}

// TestClientIPMiddleware_VisibleToOuterMiddleware pins that the RemoteAddr
// rewrite reaches middleware registered OUTSIDE this one — the request logger
// runs there, and an audit trail split between the raw peer and the resolved
// client would defeat the point of resolving once.
func TestClientIPMiddleware_VisibleToOuterMiddleware(t *testing.T) {
	var outerSawAfter string

	outer := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			// The completion log reads the request after the chain has run.
			outerSawAfter = clientIP(r)
		})
	}

	handler := outer(clientIPMiddleware(mustCIDRs(t, "10.0.0.0/8"))(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:4444"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")

	handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, "203.0.113.7", outerSawAfter,
		"outer middleware must observe the resolved client IP, not the proxy peer")
}
