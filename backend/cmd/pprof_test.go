package cmd

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
)

// pprofTestConfig returns a config whose only meaningful section is
// server.pprof.
func pprofTestConfig(enabled bool, listenAddr string) *config.Config {
	cfg := &config.Config{}
	cfg.Server.Pprof = config.PprofConfig{Enabled: config.EnvBool(enabled), ListenAddr: listenAddr}
	return cfg
}

// assertNothingListens fails the test if a TCP dial to addr succeeds.
func assertNothingListens(t *testing.T, addr string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err == nil {
		require.NoError(t, conn.Close())
	}
	require.Error(t, err, "nothing may listen on %s", addr)
}

// reserveLoopbackListener binds an ephemeral loopback port and keeps it bound
// until the test ends.
func reserveLoopbackListener(t *testing.T) net.Listener {
	t.Helper()
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		// The listener may already be closed by the test itself.
		_ = l.Close() //nolint:errcheck // best-effort cleanup of a test listener
	})
	return l
}

// TestStartPprof_DisabledStartsNoListener is the #1277 acceptance criterion
// "with default config, nothing listens on the pprof address".
func TestStartPprof_DisabledStartsNoListener(t *testing.T) {
	// Learn a free port, then release it, so the dial below can only succeed if
	// startPprof bound it.
	l := reserveLoopbackListener(t)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	pprofSrv := startPprof(context.Background(), pprofTestConfig(false, addr), slog.New(slog.DiscardHandler))

	assert.Nil(t, pprofSrv)
	assertNothingListens(t, addr)
	// Stopping a listener that never started is a no-op, not a panic.
	stopPprof(pprofSrv, slog.New(slog.DiscardHandler))
}

// TestStartPprof_EnabledServesUntilStopped is the positive control and the
// shutdown criterion: the listener answers a heap profile, and stopPprof — what
// runServer defers past the main server's return — closes the port.
func TestStartPprof_EnabledServesUntilStopped(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	pprofSrv := startPprof(context.Background(), pprofTestConfig(true, "127.0.0.1:0"), logger)
	require.NotNil(t, pprofSrv)
	addr := pprofSrv.Addr()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"http://"+addr+"/debug/pprof/heap", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)

	stopPprof(pprofSrv, logger)

	assertNothingListens(t, addr)
	assert.Contains(t, logs.String(), "pprof listener stopped")
}

// TestStartPprof_BindFailureDoesNotFailBoot pins the decision that profiling is
// a diagnostic aid: an address already in use is logged and boot continues
// without it.
func TestStartPprof_BindFailureDoesNotFailBoot(t *testing.T) {
	occupied := reserveLoopbackListener(t)
	var logs bytes.Buffer

	pprofSrv := startPprof(context.Background(),
		pprofTestConfig(true, occupied.Addr().String()), slog.New(slog.NewTextHandler(&logs, nil)))

	assert.Nil(t, pprofSrv)
	assert.Contains(t, logs.String(), "level=ERROR")
	assert.Contains(t, logs.String(), "pprof listener did not start")
}
