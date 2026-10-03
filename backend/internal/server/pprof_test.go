package server

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
)

// pprofIndexMarker is text only pprof.Index's HTML page carries.
const pprofIndexMarker = "Types of profiles available"

// startTestPprofServer starts the profiling listener on an ephemeral loopback
// port and returns it with a buffer holding everything it logged.
func startTestPprofServer(t *testing.T, listenAddr string) (*PprofServer, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	p, err := StartPprofServer(context.Background(),
		config.PprofConfig{Enabled: true, ListenAddr: listenAddr},
		slog.New(slog.NewTextHandler(&logs, nil)))
	require.NoError(t, err)
	require.NotNil(t, p)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		assert.NoError(t, p.Shutdown(ctx))
	})
	return p, &logs
}

func pprofGet(t *testing.T, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode, string(body)
}

// TestPprofServer_ServesProfilesOnItsOwnListener covers the enabled path: heap
// and goroutine profiles (and the rest of the pprof surface) answer 200 on the
// dedicated listener.
func TestPprofServer_ServesProfilesOnItsOwnListener(t *testing.T) {
	p, logs := startTestPprofServer(t, "127.0.0.1:0")
	base := "http://" + p.Addr()

	for _, path := range []string{
		"/debug/pprof/heap",
		"/debug/pprof/goroutine",
		"/debug/pprof/allocs",
		"/debug/pprof/cmdline",
		"/debug/pprof/symbol",
	} {
		status, body := pprofGet(t, base+path)
		assert.Equal(t, http.StatusOK, status, path)
		assert.NotEmpty(t, body, path)
	}

	status, body := pprofGet(t, base+"/debug/pprof/")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, pprofIndexMarker)

	// The mux serves pprof and nothing else.
	status, _ = pprofGet(t, base+"/api/v1/health")
	assert.Equal(t, http.StatusNotFound, status)

	assert.Contains(t, logs.String(), "pprof listener started")
	assert.NotContains(t, logs.String(), "non-loopback",
		"a loopback bind must not log the exposure warning")
}

// TestPprofServer_ShutdownClosesTheListener pins that Shutdown releases the
// port: a profile request succeeds before it and the dial is refused after.
func TestPprofServer_ShutdownClosesTheListener(t *testing.T) {
	p, err := StartPprofServer(context.Background(),
		config.PprofConfig{Enabled: true, ListenAddr: "127.0.0.1:0"}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	addr := p.Addr()

	status, _ := pprofGet(t, "http://"+addr+"/debug/pprof/heap")
	require.Equal(t, http.StatusOK, status)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, p.Shutdown(ctx))

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err == nil {
		require.NoError(t, conn.Close())
	}
	require.Error(t, err, "the pprof port must be closed after Shutdown")
}

// TestPprofServer_NilShutdownIsANoOp covers the disabled case, where the caller
// holds a nil server.
func TestPprofServer_NilShutdownIsANoOp(t *testing.T) {
	var p *PprofServer
	assert.NoError(t, p.Shutdown(context.Background()))
}

// TestPprofServer_BindFailureIsReturned pins that an address already in use is
// reported to the caller rather than swallowed.
func TestPprofServer_BindFailureIsReturned(t *testing.T) {
	first, _ := startTestPprofServer(t, "127.0.0.1:0")

	second, err := StartPprofServer(context.Background(),
		config.PprofConfig{Enabled: true, ListenAddr: first.Addr()}, slog.New(slog.DiscardHandler))
	require.Error(t, err)
	assert.Nil(t, second)
	assert.Contains(t, err.Error(), "failed to bind pprof listener")
}

// TestPprofServer_NonLoopbackBindWarns pins the exposure warning: binding every
// interface (0.0.0.0) is allowed, since a container needs it, but it is logged.
func TestPprofServer_NonLoopbackBindWarns(t *testing.T) {
	_, logs := startTestPprofServer(t, "0.0.0.0:0")

	assert.Contains(t, logs.String(), "level=WARN")
	assert.Contains(t, logs.String(), "non-loopback address")
}

func TestIsLoopbackListenAddr(t *testing.T) {
	tests := []struct {
		name string
		addr net.Addr
		want bool
	}{
		{"ipv4 loopback", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 6060}, true},
		{"ipv6 loopback", &net.TCPAddr{IP: net.ParseIP("::1"), Port: 6060}, true},
		{"ipv4 unspecified", &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 6060}, false},
		{"ipv6 unspecified", &net.TCPAddr{IP: net.ParseIP("::"), Port: 6060}, false},
		{"private address", &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 6060}, false},
		{"not a TCP address", &net.UnixAddr{Name: "/tmp/pprof.sock", Net: "unix"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isLoopbackListenAddr(tt.addr))
		})
	}
}

// TestMainRouter_DoesNotServePprof guards the security boundary of #1277: the
// API port must never answer a profile request, whatever server.pprof says. It
// checks both the route table and what a request actually gets back.
func TestMainRouter_DoesNotServePprof(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := &config.Config{}
		cfg.Server.Pprof = config.PprofConfig{Enabled: config.EnvBool(enabled), ListenAddr: "127.0.0.1:0"}
		srv := New("8080", nil, "test-api-key", cfg, slog.New(slog.DiscardHandler))

		require.NoError(t, chi.Walk(srv.router,
			func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
				assert.NotContains(t, route, "/debug", "%s %s is mounted on the API router", method, route)
				return nil
			}))

		for _, path := range []string{"/debug/pprof/", "/debug/pprof/heap", "/debug/pprof/goroutine"} {
			rec := httptest.NewRecorder()
			srv.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			assert.Equal(t, http.StatusNotFound, rec.Code, "%s (pprof enabled=%v)", path, enabled)
			assert.NotContains(t, rec.Body.String(), pprofIndexMarker, path)
			assert.False(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "application/octet-stream"),
				"%s returned a binary profile on the API port", path)
		}
	}
}
