package services

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/a2atest"
)

// newTimeoutHarness wires the real invocation stack to a toy agent + fake repos
// with a caller-supplied config, so the A2A client's sync/stream timeouts come
// from cfg.A2A. Reuses streamingLifecycle / passthroughEncryption from the e2e
// suite (same package).
func newTimeoutHarness(
	t *testing.T, script a2atest.Script, cfg *config.Config,
) (*AgentInvocationService, *models.Agent, *a2atest.ExecutionStore) {
	t.Helper()
	srv := a2atest.NewServer(t, script)
	agent := srv.Agent("agent-timeout")
	execStore := a2atest.NewExecutionStore()
	eventStore := a2atest.NewEventStore()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth := NewAgentAuthenticator(passthroughEncryption{})
	client := newA2AHTTPClient(auth, cfg, &ssrfGuard{allowPrivate: true})
	sp := NewA2AStreamProcessor(eventStore, execStore, logger)
	svc := NewAgentInvocationService(a2atest.NewAgentStore(agent), execStore, eventStore, client, sp, logger)
	return svc, agent, execStore
}

// A streaming agent whose events span longer than the sync default_timeout must
// NOT be force-closed by it — the stream is bounded only by stream_timeout.
func TestAgentInvocationTimeout_StreamingSurvivesPastSyncTimeout(t *testing.T) {
	cfg := &config.Config{}
	cfg.A2A.DefaultTimeout = 150 * time.Millisecond // short sync cap
	cfg.A2A.StreamTimeout = 10 * time.Second        // generous stream cap

	// streamingLifecycle yields 5 events; 80ms each ≈ 400ms total > 150ms.
	svc, agent, execStore := newTimeoutHarness(t,
		a2atest.Script{Streaming: true, Delay: 80 * time.Millisecond, Events: streamingLifecycle}, cfg)

	exec, err := svc.InvokeAgent(
		context.Background(), agent.UserID, agent.ID, map[string]interface{}{"text": "hi"}, nil)
	require.NoError(t, err)

	// Assert the stream ran to COMPLETION (finalized success AND full artifact
	// assembled), not just a terminal status: the finalize-on-error path (#197)
	// can land on "success" even when a stream is cut short, so a status check
	// alone would pass even if the sync timeout force-closed the stream. The full
	// "Hello world" artifact is only assembled if every event through the terminal
	// completed was received. Status is finalized last, so waiting on it also
	// guarantees the artifact is already persisted (no read race).
	a2atest.Eventually(t, func() bool {
		got, gerr := execStore.GetByID(context.Background(), agent.UserID, exec.ID)
		return gerr == nil && got.Status == "success" && artifactText(got) == "Hello world"
	}, "a stream longer than default_timeout must complete, not be force-closed")
}

// A stream that exceeds stream_timeout must terminate in a terminal state — not
// stay wedged in pending/working.
func TestAgentInvocationTimeout_StreamingBoundedByStreamTimeout(t *testing.T) {
	cfg := &config.Config{}
	cfg.A2A.DefaultTimeout = 5 * time.Second
	cfg.A2A.StreamTimeout = 200 * time.Millisecond // short stream cap

	// 5 events * 300ms ≈ 1500ms of natural work, far beyond the 200ms cap, so the
	// deadline must fire mid-stream. The wide gap lets us prove the deadline (not
	// natural completion) terminated the stream by timing, without depending on
	// the exact final status (the #197 finalize race makes error-vs-success
	// nondeterministic — both are terminal).
	svc, agent, execStore := newTimeoutHarness(t,
		a2atest.Script{Streaming: true, Delay: 300 * time.Millisecond, Events: streamingLifecycle}, cfg)

	start := time.Now()
	exec, err := svc.InvokeAgent(
		context.Background(), agent.UserID, agent.ID, map[string]interface{}{"text": "hi"}, nil)
	require.NoError(t, err)

	a2atest.Eventually(t, func() bool {
		got, gerr := execStore.GetByID(context.Background(), agent.UserID, exec.ID)
		return gerr == nil && got.Status != "pending" && got.Status != "working"
	}, "a stream exceeding stream_timeout must reach a terminal state, never wedged")
	elapsed := time.Since(start)

	got, gerr := execStore.GetByID(context.Background(), agent.UserID, exec.ID)
	require.NoError(t, gerr)
	require.True(t, isTerminalStatus(got.Status), "final status %q must be terminal", got.Status)
	// If stream_timeout did not fire, the stream would only finish near its ~1500ms
	// natural completion — reaching terminal well before that proves the deadline cut it.
	require.Less(t, elapsed, time.Second,
		"stream_timeout should terminate the stream well before its ~1500ms natural completion")
}
