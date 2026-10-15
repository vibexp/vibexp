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

// purgeCounter is an AuditPurgeServiceInterface whose Purge counts its calls.
type purgeCounter struct {
	calls    atomic.Int32
	deleted  int64
	skipped  bool
	err      error
	deadline atomic.Bool
}

func (p *purgeCounter) Purge(ctx context.Context) (int64, bool, error) {
	p.calls.Add(1)
	if _, ok := ctx.Deadline(); ok {
		p.deadline.Store(true)
	}
	return p.deleted, p.skipped, p.err
}

func newPurgeServer(t *testing.T, purger services.AuditPurgeServiceInterface) (*Server, *logtest.Recorder) {
	t.Helper()
	container := newMockAuthContainer(t)
	container.auditPurge = purger
	srv := createTestAuthServer(container)
	logger, logs := logtest.New()
	srv.logger = logger
	return srv, logs
}

// The purge runs at once, then on every tick, each run bounded by a timeout,
// and its goroutine ends with the server's context.
func TestAuditPurge_RunsUntilTheServerStops(t *testing.T) {
	purger := &purgeCounter{deleted: 7}
	srv, logs := newPurgeServer(t, purger)
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})

	go func() {
		srv.runAuditPurge(ctx, 5*time.Millisecond)
		close(stopped)
	}()

	require.Eventually(t, func() bool { return purger.calls.Load() >= 3 }, 5*time.Second, time.Millisecond,
		"once at start-up and again on each tick")
	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the purge did not stop with the server's context")
	}
	after := purger.calls.Load()
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, after, purger.calls.Load(), "nothing is purged after the stop")
	assert.True(t, purger.deadline.Load(), "each run is bounded by auditPurgeTimeout")
	require.NotNil(t, logs.LastEntry())
	assert.Contains(t, logs.LastEntry().Message, "Purged expired audit events")
	assert.EqualValues(t, 7, logs.LastEntry().Data["deleted"])
}

// The server starts the purge itself.
func TestServerStartsTheAuditPurge(t *testing.T) {
	purger := &purgeCounter{}
	srv, _ := newPurgeServer(t, purger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, srv.startBackgroundWork(ctx))

	require.Eventually(t, func() bool { return purger.calls.Load() >= 1 }, 5*time.Second, time.Millisecond,
		"the start-up purge ran")
}

// A failed purge is logged and the loop carries on.
func TestAuditPurge_FailureDoesNotStopTheLoop(t *testing.T) {
	purger := &purgeCounter{err: errors.New("connection refused")}
	srv, logs := newPurgeServer(t, purger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.runAuditPurge(ctx, 5*time.Millisecond)

	require.Eventually(t, func() bool { return purger.calls.Load() >= 2 }, 5*time.Second, time.Millisecond)
	require.NotNil(t, logs.LastEntry())
	assert.Contains(t, logs.LastEntry().Message, "Failed to purge")
}

// A run skipped because another replica holds the lock is not reported as a
// purge.
func TestAuditPurge_SkippedRunIsNotReportedAsAPurge(t *testing.T) {
	purger := &purgeCounter{skipped: true}
	srv, logs := newPurgeServer(t, purger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.runAuditPurge(ctx, 5*time.Millisecond)

	require.Eventually(t, func() bool { return purger.calls.Load() >= 2 }, 5*time.Second, time.Millisecond)
	for _, entry := range logs.AllEntries() {
		assert.NotContains(t, entry.Message, "Purged expired audit events")
	}
}

// A server with no purge service runs no loop.
func TestAuditPurge_NoServiceReturnsAtOnce(t *testing.T) {
	none, _ := newPurgeServer(t, nil)
	done := make(chan struct{})
	go func() {
		none.runAuditPurge(context.Background(), time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a server with no purge service must return at once")
	}
}
