package server

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/services"
)

// sweepCounter is an AccountTokenServiceInterface whose Sweep counts its calls.
type sweepCounter struct {
	services.AccountTokenServiceInterface
	calls   atomic.Int32
	removed int64
	err     error
}

func (s *sweepCounter) Sweep(context.Context) (int64, error) {
	s.calls.Add(1)
	return s.removed, s.err
}

func newSweepServer(t *testing.T, links services.AccountTokenServiceInterface) (*Server, *logtest.Recorder) {
	t.Helper()
	container := newMockAuthContainer(t)
	container.accountTokens = links
	srv := createTestAuthServer(container)
	logger, logs := logtest.New()
	srv.logger = logger
	return srv, logs
}

// The sweep runs at once, then on every tick, and its goroutine ends with the
// server's context.
func TestAccountLinkSweep_RunsUntilTheServerStops(t *testing.T) {
	links := &sweepCounter{removed: 3}
	srv, logs := newSweepServer(t, links)
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})

	go func() {
		srv.runAccountLinkSweep(ctx, 5*time.Millisecond)
		close(stopped)
	}()

	require.Eventually(t, func() bool { return links.calls.Load() >= 3 }, 5*time.Second, time.Millisecond,
		"once at start-up and again on each tick")
	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the sweep did not stop with the server's context")
	}
	after := links.calls.Load()
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, after, links.calls.Load(), "nothing is swept after the stop")
	require.NotNil(t, logs.LastEntry())
	assert.EqualValues(t, 3, logs.LastEntry().Data["removed"])
}

// The server starts the sweep itself, and does so with the authorization
// server disabled: dead links have nothing to do with it.
func TestServerStartsTheAccountLinkSweepWithoutTheAuthorizationServer(t *testing.T) {
	links := &sweepCounter{}
	srv, _ := newSweepServer(t, links)
	require.Nil(t, srv.oauthAS, "the authorization server is off in this server")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, srv.startBackgroundWork(ctx))

	require.Eventually(t, func() bool { return links.calls.Load() >= 1 }, 5*time.Second, time.Millisecond,
		"the start-up sweep ran")
}

// A failed sweep is logged and the loop carries on; a server with no link
// service runs no loop.
func TestAccountLinkSweep_FailureDoesNotStopTheLoop(t *testing.T) {
	links := &sweepCounter{err: errors.New("connection refused")}
	srv, logs := newSweepServer(t, links)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.runAccountLinkSweep(ctx, 5*time.Millisecond)

	require.Eventually(t, func() bool { return links.calls.Load() >= 2 }, 5*time.Second, time.Millisecond)
	require.NotNil(t, logs.LastEntry())
	assert.Contains(t, logs.LastEntry().Message, "Failed to sweep")

	none, _ := newSweepServer(t, nil)
	done := make(chan struct{})
	go func() {
		none.runAccountLinkSweep(context.Background(), time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a server with no link service must return at once")
	}
}
