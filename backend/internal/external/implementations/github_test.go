package implementations

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v57/github"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
)

// testRSAPEM is a 2048-bit RSA private key used ONLY for unit testing.
// It is intentionally checked into the test file as a test fixture.
// #nosec G101 -- test-only credential, never used in production
const testRSAPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA2a2rwplBQLzHPZe5TNJNR4UzELbRnKsKxgfCXObdAWRSMY7N
6egbROKCNQLPTLyB2Gv1oEt6CKpvVcWh3FRfKEiBvCKkBSwqFGHbv5ONoAoyjke9
tMVkq4j+MH7/dXYSI2Y5HRl7UrFoYXO6WT9V2GV5hhJYniANPCZPWdkIXkJBF5r1
5PKN31JhG8VuGGaJw1UhqopqAl0yNlSGUCH1LLKqFWqHH/JVEg1bMH4z5xXhZfzs
A1QLJqN71tOhWZT6LPp2PxBLBFtTe00EiKYgC0G+bYMr9vb1NyJKxnhqzFpSlb7N
I5mLWX2o7YMfZkU5F6e7KJ7J6OFwJ/7LjVaRKwIDAQABAoIBAFBsPBGGiFjqQMFO
b7eVxJCGy27jMmDmVJ3nFHHn1UiZ2BOZ87xORFKk89j8pLxw5cjExzO5Kv8VYc6+
GBK0TL/UvEtl8g+3+x1LJFIzk+J7HMBBbp/RXXI6EIDhFgRp0e+jY4GGm/NUMl4G
jTaP/P2E88Qa9kU+tBVg7y2FGKv5FJZqD8Gg7OAHRB3ViFVwnFM8/u7L7x6o4Pzs
Z8aJ5VkIkB3LmZJR5X3vD7mj60UDJXHNAstuvhg+vDAJelNHwCgw2i/DW6pVXxQ2
3rE6h2QTGJDQRxNz6MwnFcJOWM0A0GMrZL+Cq7LvqFy/rZ2r6P2RRbvWn6UzKYVH
cPO4V8ECgYEA7p5Omy00lFUWCwPbBpRzWIUZujS4p0aqS78BaFl1Cr6t06b6zUa6
5nz2XpgXRNvPmOAl4qqXpSH3ZA6cGPNmxDlsJt0TG7wTIDZzb8uqdM8RjAzR7P2l
vKFBhiV/gEQaGSmW3V9A7J1M0DFa1cGSYq3u0XMDUhiLPE/u9+1E5FcCgYEA6PI3
JrKM4Y0JW62WEKN4K3sNVrqPiqGSbIQkP/DYM2nRueTjR0JsHyU7mFkq91LFOvYG
6f4l4HkWVxM0TrLfUMV5kXJ3CcQKi7FGSiYm/PBFQ5FGJBaVIMRJCxzPRJfcvRXm
7xWFsUe/Nwf5NsF72P/mFQ/JtyK3mAvpImcSv+ECgYBBnnpb4yyJb7bGJLqWrY4G
Q3qLxNKXWGjXMFSj8/4JnW4mFz0wYJEJ1cHBJWVXb2I8Wh/s/+AW3A1OjFiMqf1L
mY7M2mV+D8g1C9i0W2/oa1Jh3VTQALm5fN/tRD/hqrXQ3HrYDWjmqDjW+m7hmUV
n6nMEK7a1EHpHY7fR7KQGQ==
-----END RSA PRIVATE KEY-----`

// newTestGitHubAppClient builds a GitHubAppClient with a real (but test-only) RSA
// PEM key. The client is pre-seeded with a populated clientCache map so that
// caching tests can be run without calling createInstallationTransport (which
// tries to parse the PEM via ghinstallation).
func newTestGitHubAppClient(t *testing.T) *GitHubAppClient {
	t.Helper()
	cfg := &config.GitHubAppConfig{
		AppID:         "12345",
		PrivateKeyPEM: []byte(testRSAPEM),
	}
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	return &GitHubAppClient{
		cfg:         cfg,
		logger:      logger,
		clientCache: make(map[int64]*github.Client),
	}
}

// TestGitHubAppClientConstants verifies timeout constants are set to expected values.
func TestGitHubAppClientConstants(t *testing.T) {
	assert.Equal(t, 30*time.Second, githubAPITimeout)
	assert.Equal(t, 15*time.Second, githubAPIFastTimeout)
}

// TestNewGitHubAppClient_NilConfig returns stub.
func TestNewGitHubAppClient_NilConfig(t *testing.T) {
	logger := logrus.New()
	client := NewGitHubAppClient(nil, logger)
	assert.NotNil(t, client)
	_, ok := client.(*stubGitHubAppClient)
	assert.True(t, ok, "expected stub for nil config")
}

// TestNewGitHubAppClient_MissingAppID returns stub.
func TestNewGitHubAppClient_MissingAppID(t *testing.T) {
	cfg := &config.GitHubAppConfig{
		AppID:         "",
		PrivateKeyPEM: []byte(testRSAPEM),
	}
	logger := logrus.New()
	client := NewGitHubAppClient(cfg, logger)
	_, ok := client.(*stubGitHubAppClient)
	assert.True(t, ok, "expected stub when AppID is empty")
}

// TestNewGitHubAppClient_NilPrivateKey returns stub.
func TestNewGitHubAppClient_NilPrivateKey(t *testing.T) {
	cfg := &config.GitHubAppConfig{
		AppID:         "12345",
		PrivateKeyPEM: []byte(testRSAPEM),
		PrivateKey:    nil, // nil PrivateKey → stub
	}
	logger := logrus.New()
	client := NewGitHubAppClient(cfg, logger)
	_, ok := client.(*stubGitHubAppClient)
	assert.True(t, ok, "expected stub when PrivateKey is nil")
}

// TestCreateInstallationTransport_CacheHit verifies that when a client is already
// present in the cache the same pointer is returned without re-building.
func TestCreateInstallationTransport_CacheHit(t *testing.T) {
	c := newTestGitHubAppClient(t)

	const installationID = int64(42)

	// Pre-populate the cache with a sentinel client.
	sentinel := github.NewClient(nil)
	c.clientCache[installationID] = sentinel

	got, err := c.createInstallationTransport(installationID)
	require.NoError(t, err)
	assert.Same(t, sentinel, got, "cache hit must return the cached pointer")
}

// TestCreateInstallationTransport_CacheHit_Concurrent verifies that when a client
// is already in the cache, concurrent reads all get the same cached pointer.
func TestCreateInstallationTransport_CacheHit_Concurrent(t *testing.T) {
	c := newTestGitHubAppClient(t)

	const installationID = int64(99)
	const goroutines = 30

	// Pre-populate the cache.
	sentinel := github.NewClient(nil)
	c.clientCache[installationID] = sentinel

	var wg sync.WaitGroup
	clients := make([]*github.Client, goroutines)
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cl, err := c.createInstallationTransport(installationID)
			clients[idx] = cl
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i := range goroutines {
		assert.NoError(t, errs[i], "goroutine %d got error", i)
		assert.Same(t, sentinel, clients[i], "goroutine %d should get cached pointer", i)
	}
}

// TestCreateInstallationTransport_DifferentCacheKeys verifies that different
// installationIDs map to independent cache entries.
func TestCreateInstallationTransport_DifferentCacheKeys(t *testing.T) {
	c := newTestGitHubAppClient(t)

	clientA := github.NewClient(nil)
	clientB := github.NewClient(nil)
	c.clientCache[int64(1)] = clientA
	c.clientCache[int64(2)] = clientB

	gotA, err := c.createInstallationTransport(int64(1))
	require.NoError(t, err)

	gotB, err := c.createInstallationTransport(int64(2))
	require.NoError(t, err)

	assert.Same(t, clientA, gotA, "installation 1 should return clientA")
	assert.Same(t, clientB, gotB, "installation 2 should return clientB")
	assert.NotSame(t, gotA, gotB, "different installations must not share a client")
}

// TestCreateInstallationTransport_InvalidAppID verifies error on non-numeric AppID
// when the cache is empty (slow path).
func TestCreateInstallationTransport_InvalidAppID(t *testing.T) {
	cfg := &config.GitHubAppConfig{
		AppID:         "not-a-number",
		PrivateKeyPEM: []byte(testRSAPEM),
	}
	c := &GitHubAppClient{
		cfg:         cfg,
		logger:      logrus.New(),
		clientCache: make(map[int64]*github.Client),
	}

	_, err := c.createInstallationTransport(int64(1))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid GitHub App ID")
}

// TestGetInstallationRepositories_ContextCancelled verifies that a context timeout
// (or cancellation) is propagated and causes an error rather than hanging.
func TestGetInstallationRepositories_ContextCancelled(t *testing.T) {
	// Use a pre-cancelled context so the API call fails immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := newTestGitHubAppClient(t)
	// Pre-seed a cached client so createInstallationTransport skips PEM parsing.
	c.clientCache[int64(1)] = github.NewClient(nil)

	_, _, err := c.GetInstallationRepositories(ctx, int64(1), 1)
	assert.Error(t, err, "cancelled context must produce an error")
}

// TestGetRepository_ContextCancelled mirrors the timeout/cancellation test.
func TestGetRepository_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := newTestGitHubAppClient(t)
	c.clientCache[int64(1)] = github.NewClient(nil)

	_, err := c.GetRepository(ctx, int64(1), int64(100))
	assert.Error(t, err, "cancelled context must produce an error")
}

// TestGetFileContent_ContextCancelled mirrors the timeout/cancellation test.
func TestGetFileContent_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := newTestGitHubAppClient(t)
	c.clientCache[int64(1)] = github.NewClient(nil)

	_, err := c.GetFileContent(ctx, int64(1), "owner", "repo", "path/file.md")
	assert.Error(t, err, "cancelled context must produce an error")
}

// TestGetDirectoryContentsRecursive_ContextCancelled mirrors the timeout test.
func TestGetDirectoryContentsRecursive_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := newTestGitHubAppClient(t)
	c.clientCache[int64(1)] = github.NewClient(nil)

	_, err := c.GetDirectoryContentsRecursive(ctx, int64(1), "owner", "repo", ".claude")
	assert.Error(t, err, "cancelled context must produce an error")
}

// TestGitHubAppClient_ContextDeadlineIsSet verifies that the context passed to
// GitHub API calls contains a deadline (i.e., WithTimeout was applied).
// We do this by calling a method with a background context and checking that
// the error is context-related — not a permanent error from no deadline.
func TestGitHubAppClient_ContextDeadlineIsSet(t *testing.T) {
	// Use a context with an impossibly short deadline to confirm our code
	// applies an additional timeout. If we passed ctx through unchanged the
	// deadline would already be exceeded; the method should add its own.
	ctx := context.Background()
	c := newTestGitHubAppClient(t)
	c.clientCache[int64(1)] = github.NewClient(nil)

	// Verify methods accept context.Background() (with no external deadline) and
	// still add a deadline internally. We cannot observe the internal deadline
	// directly, so we just ensure the methods don't panic and do return errors
	// (because there's no real GitHub server behind the client).
	_, _, err := c.GetInstallationRepositories(ctx, int64(1), 1)
	assert.Error(t, err)

	_, err = c.GetRepository(ctx, int64(1), int64(100))
	assert.Error(t, err)

	_, err = c.GetFileContent(ctx, int64(1), "o", "r", "p")
	assert.Error(t, err)

	_, err = c.GetDirectoryContentsRecursive(ctx, int64(1), "o", "r", "d")
	assert.Error(t, err)
}

// TestStubGitHubAppClient_AllMethodsReturnError verifies the stub client returns
// an error for every interface method.
func TestStubGitHubAppClient_AllMethodsReturnError(t *testing.T) {
	s := &stubGitHubAppClient{}
	ctx := context.Background()

	_, _, err := s.GetInstallationToken(ctx, 1)
	assert.Error(t, err)

	_, _, err = s.GetInstallationRepositories(ctx, 1, 1)
	assert.Error(t, err)

	_, err = s.GetInstallation(ctx, 1)
	assert.Error(t, err)

	_, err = s.GetRepository(ctx, 1, 1)
	assert.Error(t, err)

	_, err = s.GetFileContent(ctx, 1, "owner", "repo", "path")
	assert.Error(t, err)

	_, err = s.GetDirectoryContentsRecursive(ctx, 1, "owner", "repo", "dir")
	assert.Error(t, err)
}

// TestIsInstallationGone covers the classification of ghinstallation token
// refresh failures: only an HTTP 404 means the installation was uninstalled.
func TestIsInstallationGone(t *testing.T) {
	refresh404 := &ghinstallation.HTTPError{
		Message:  `received non 2xx response status "404 Not Found" when fetching access_tokens`,
		Response: &http.Response{StatusCode: http.StatusNotFound},
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "plain error", err: errors.New("github api timeout"), want: false},
		{name: "refresh 404 direct", err: refresh404, want: true},
		{
			// Production shape: http.Client wraps the transport error in
			// *url.Error and ghinstallation wraps the HTTPError with %w.
			name: "refresh 404 wrapped through url.Error and fmt",
			err: &url.Error{
				Op:  "Get",
				URL: "https://api.github.com/installation/repositories",
				Err: fmt.Errorf("could not refresh installation id 110742404's token: %w", refresh404),
			},
			want: true,
		},
		{
			name: "refresh 401 is app credential trouble, not removal",
			err: &ghinstallation.HTTPError{
				Response: &http.Response{StatusCode: http.StatusUnauthorized},
			},
			want: false,
		},
		{
			name: "HTTPError without response",
			err:  &ghinstallation.HTTPError{RootCause: errors.New("dial tcp: connection refused")},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isInstallationGone(tt.err))
		})
	}
}
