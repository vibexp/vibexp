package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
)

func TestIsAPIKey_ValidPrefixes(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected bool
	}{
		{
			name:     "Valid API key with PrefixEverything (ak_)",
			token:    models.PrefixEverything + "testvalue",
			expected: true,
		},
		{
			name:     "Valid API key with PrefixAITools (aait-)",
			token:    models.PrefixAITools + "testvalue",
			expected: true,
		},
		{
			name:     "Valid API key with PrefixCLI (acli-)",
			token:    models.PrefixCLI + "testvalue",
			expected: true,
		},
		{
			name:     "Valid API key with PrefixMCP (amcp-)",
			token:    models.PrefixMCP + "testvalue",
			expected: true,
		},
		{
			name:     "Valid API key with PrefixVibeXPKey (vxk_)",
			token:    models.PrefixVibeXPKey + "testvalue",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAPIKey(tt.token)
			if result != tt.expected {
				t.Errorf("isAPIKey(%q) = %v, want %v", tt.token, result, tt.expected)
			}
		})
	}
}

func TestIsAPIKey_InvalidTokens(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected bool
	}{
		{ // #nosec G101 - test credential
			name:     "JWT-like token should not be recognized as API key",
			token:    "eyJ.eyJ.signature",
			expected: false,
		},
		{
			name:     "Random string should not be recognized as API key",
			token:    "some-random-token-string",
			expected: false,
		},
		{
			name:     "Empty string should not be recognized as API key",
			token:    "",
			expected: false,
		},
		{
			name:     "Similar but invalid prefix (vxk without underscore)",
			token:    "vxktestvalue",
			expected: false,
		},
		{ // #nosec G101 - test credential
			name:     "Case sensitive - uppercase VXK_ should not match",
			token:    "VXK_testvalue",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAPIKey(tt.token)
			if result != tt.expected {
				t.Errorf("isAPIKey(%q) = %v, want %v", tt.token, result, tt.expected)
			}
		})
	}
}

func TestIsAPIKey_AllDefinedPrefixes(t *testing.T) {
	// This test ensures all prefixes defined in models are recognized by isAPIKey
	// If a new prefix is added to models but not to isAPIKey, this test should fail
	prefixes := []struct {
		name   string
		prefix string
	}{
		{"PrefixEverything", models.PrefixEverything},
		{"PrefixAITools", models.PrefixAITools},
		{"PrefixCLI", models.PrefixCLI},
		{"PrefixMCP", models.PrefixMCP},
		{"PrefixVibeXPKey", models.PrefixVibeXPKey},
	}

	for _, p := range prefixes {
		t.Run(p.name, func(t *testing.T) {
			token := p.prefix + "testvalue"
			if !isAPIKey(token) {
				t.Errorf("isAPIKey should recognize tokens with %s prefix (%s)", p.name, p.prefix)
			}
		})
	}
}

// newQueryParamTestServer builds a Server with the full router initialised so
// the MCP OAuth route and the flexible-auth non-MCP routes are wired.
func newQueryParamTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	return New("8080", nil, "test-api-key", cfg, logger)
}

// TestMCPQueryParamAPIKeyRejected verifies that the insecure ?api_key query
// parameter (forbidden by the MCP authorization spec) is no longer accepted on
// the MCP endpoint. With no Authorization header the OAuth bearer-token
// middleware rejects the request with 401 regardless of the query string.
func TestMCPQueryParamAPIKeyRejected(t *testing.T) {
	// #nosec G101 - test credential, not a real key
	const fakeKey = "?api_key=amcp-valid-query-param-key-abc"
	tests := []struct {
		name  string
		query string
	}{
		{name: "valid-looking mcp key in query", query: fakeKey},
		{name: "empty api_key query", query: "?api_key="},
		{name: "no query at all", query: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newQueryParamTestServer(t)

			req, err := http.NewRequest(http.MethodGet, "/mcp/v1/common"+tt.query, nil)
			assert.NoError(t, err)

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusUnauthorized, rr.Code,
				"?api_key query param must not authenticate the MCP endpoint (got %d)", rr.Code)
		})
	}
}

// TestQueryParamRejectedOnNonMCPPaths verifies that the removed ?api_key query
// parameter fallback is also rejected on non-MCP flexible-auth endpoints.
func TestQueryParamRejectedOnNonMCPPaths(t *testing.T) {
	const mcpKey = "amcp-valid-query-param-key-abc" // #nosec G101 - test credential

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "GET /api/v1/auth/me with api_key query param",
			method: http.MethodGet,
			path:   "/api/v1/auth/me?api_key=" + mcpKey,
		},
		{
			name:   "POST /api/v1/cursor-ide/hooks with api_key query param",
			method: http.MethodPost,
			path:   "/api/v1/cursor-ide/hooks?api_key=" + mcpKey,
		},
		{
			name:   "POST /api/v1/claude-code/hooks with api_key query param",
			method: http.MethodPost,
			path:   "/api/v1/claude-code/hooks?api_key=" + mcpKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newQueryParamTestServer(t)

			req, err := http.NewRequest(tt.method, tt.path, nil)
			assert.NoError(t, err)

			rr := httptest.NewRecorder()
			srv.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusUnauthorized, rr.Code,
				"?api_key query param must not authenticate non-MCP path %s (got %d)", tt.path, rr.Code)
		})
	}
}

func TestIsTransientRefreshError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		transient bool
	}{
		{"nil error", nil, false},
		{"WorkOS 500", errors.New("workos: authenticate endpoint returned 500: oops"), true},
		{"WorkOS 503", errors.New("workos: authenticate endpoint returned 503: try later"), true},
		{"WorkOS 400", errors.New("workos: authenticate endpoint returned 400: bad request"), false},
		{"WorkOS 401 invalid_grant", errors.New("workos: authenticate endpoint returned 401: invalid_grant"), false},
		{"context deadline", context.DeadlineExceeded, true},
		{"context canceled", context.Canceled, true},
		{"DNS lookup failure", errors.New("dial tcp: lookup api.workos.com: no such host"), true},
		{"connection refused", errors.New("dial tcp: connection refused"), true},
		{"unexpected EOF", errors.New("read tcp: EOF"), true},
		{"plain unknown error", errors.New("something else"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.transient, isTransientRefreshError(tt.err))
		})
	}
}

func TestRefreshLockFor_ReturnsSameMutexPerUser(t *testing.T) {
	srv := &Server{}
	mu1 := srv.refreshLockFor("user-A")
	mu2 := srv.refreshLockFor("user-A")
	mu3 := srv.refreshLockFor("user-B")

	assert.Same(t, mu1, mu2, "same user should always get the same mutex")
	assert.NotSame(t, mu1, mu3, "different users must get distinct mutexes")
}

func TestRefreshLockFor_SerializesConcurrentRefreshes(t *testing.T) {
	srv := &Server{}

	const goroutines = 16
	mu := srv.refreshLockFor("user-X")

	var concurrent int32
	var maxConcurrent int32
	var done sync.WaitGroup
	done.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer done.Done()
			lock := srv.refreshLockFor("user-X")
			lock.Lock()
			// Simulate some work; track maximum observed concurrency.
			cur := concurrent + 1
			concurrent = cur
			if cur > maxConcurrent {
				maxConcurrent = cur
			}
			time.Sleep(2 * time.Millisecond)
			concurrent--
			lock.Unlock()
		}()
	}
	done.Wait()

	assert.Equal(t, mu, srv.refreshLockFor("user-X"))
	assert.LessOrEqual(t, maxConcurrent, int32(1),
		"per-user refresh lock must serialize concurrent goroutines")
}
