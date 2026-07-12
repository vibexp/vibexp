package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/a2atest"
)

// These tests drive the REAL AgentInvocationService (+ real A2A SDK client and
// stream processor) against an in-process a2asrv toy agent, over the canonical
// A2A v1.0 wire protocol. Only the repository boundary is faked (in-memory,
// concurrency-safe), so the suite runs under plain `make backend-test` with no
// Postgres and no network egress (loopback httptest only). The postgres SQL for
// these repos is covered separately by the repositories/postgres integration
// tests.

// e2eHarness wires the real invocation stack to a toy agent + fake repos.
type e2eHarness struct {
	svc        *AgentInvocationService
	srv        *a2atest.Server
	agent      *models.Agent
	agentStore *a2atest.AgentStore
	execStore  *a2atest.ExecutionStore
	eventStore *a2atest.EventStore
}

func newE2EHarness(t *testing.T, script a2atest.Script, customize func(*models.Agent)) *e2eHarness {
	t.Helper()
	srv := a2atest.NewServer(t, script)
	agent := srv.Agent("agent-e2e")
	if customize != nil {
		customize(agent)
	}
	agentStore := a2atest.NewAgentStore(agent)
	execStore := a2atest.NewExecutionStore()
	eventStore := a2atest.NewEventStore()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth := NewAgentAuthenticator(passthroughEncryption{})
	// Loopback-permitting client so the SSRF guard allows the httptest agent.
	client := newA2AHTTPClient(auth, &config.Config{}, &ssrfGuard{allowPrivate: true})
	sp := NewA2AStreamProcessor(eventStore, execStore, logger)
	svc := NewAgentInvocationService(agentStore, execStore, eventStore, client, sp, logger)

	return &e2eHarness{
		svc: svc, srv: srv, agent: agent,
		agentStore: agentStore, execStore: execStore, eventStore: eventStore,
	}
}

func (h *e2eHarness) invoke(
	ctx context.Context, t *testing.T, conversationID *string,
) *models.AgentExecution {
	t.Helper()
	exec, err := h.svc.InvokeAgent(ctx, h.agent.UserID, h.agent.ID, map[string]interface{}{"text": "hello"}, conversationID)
	require.NoError(t, err)
	require.NotNil(t, exec)
	return exec
}

func (h *e2eHarness) stored(t *testing.T, executionID string) *models.AgentExecution {
	t.Helper()
	exec, err := h.execStore.GetByID(context.Background(), h.agent.UserID, executionID)
	require.NoError(t, err)
	return exec
}

// artifactText concatenates the text of every part across all artifacts.
func artifactText(exec *models.AgentExecution) string {
	out := ""
	for _, art := range exec.Artifacts {
		parts, ok := art["parts"].([]interface{})
		if !ok {
			continue
		}
		for _, p := range parts {
			if m, ok := p.(map[string]interface{}); ok {
				if txt, ok := m["text"].(string); ok {
					out += txt
				}
			}
		}
	}
	return out
}

func messageEvents(text string) func(*a2asrv.ExecutorContext) []a2a.Event {
	return func(*a2asrv.ExecutorContext) []a2a.Event {
		return []a2a.Event{a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(text))}
	}
}

// --- Sync scenarios ------------------------------------------------------

func TestAgentInvocationE2E_SyncMessageReply(t *testing.T) {
	h := newE2EHarness(t, a2atest.Script{Events: messageEvents("the answer")}, nil)

	exec := h.invoke(context.Background(), t, nil)

	assert.Equal(t, "success", exec.Status)
	// The reply is persisted (#163) and visible via the execution store.
	stored := h.stored(t, exec.ID)
	assert.Equal(t, "success", stored.Status)
	assert.Equal(t, "the answer", artifactText(stored))
}

func TestAgentInvocationE2E_SyncTerminalTaskWithArtifacts(t *testing.T) {
	events := func(ec *a2asrv.ExecutorContext) []a2a.Event {
		task := a2a.NewSubmittedTask(ec, ec.Message)
		task.Status = a2a.TaskStatus{State: a2a.TaskStateCompleted}
		task.Artifacts = []*a2a.Artifact{{ID: "a1", Parts: a2a.ContentParts{a2a.NewTextPart("done")}}}
		return []a2a.Event{task}
	}
	h := newE2EHarness(t, a2atest.Script{Events: events}, nil)

	exec := h.invoke(context.Background(), t, nil)

	assert.Equal(t, "success", exec.Status)
	require.NotNil(t, exec.TaskID)
	require.NotNil(t, exec.ContextID)
	assert.Equal(t, "done", artifactText(h.stored(t, exec.ID)))
}

func TestAgentInvocationE2E_SyncJSONRPCError(t *testing.T) {
	h := newE2EHarness(t, a2atest.Script{Err: errors.New("boom")}, nil)

	exec := h.invoke(context.Background(), t, nil)

	assert.Equal(t, "error", exec.Status)
	require.NotNil(t, exec.Error)
	assert.Equal(t, "error", h.stored(t, exec.ID).Status)
}

func TestAgentInvocationE2E_SyncHTTP500(t *testing.T) {
	h := newE2EHarness(t, a2atest.Script{ForceStatus: 500}, nil)

	exec := h.invoke(context.Background(), t, nil)

	assert.Equal(t, "error", exec.Status)
	require.NotNil(t, exec.Error)
}

func TestAgentInvocationE2E_SyncSlowAgentDeadline(t *testing.T) {
	// A slow agent against a short caller deadline surfaces as an error, not a hang.
	h := newE2EHarness(t, a2atest.Script{Delay: time.Second, Events: messageEvents("late")}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	exec := h.invoke(ctx, t, nil)

	assert.Equal(t, "error", exec.Status)
	require.NotNil(t, exec.Error)
}

// --- Streaming scenarios -------------------------------------------------

// streamingLifecycle yields task → working → artifact(chunk1) → append(chunk2) → completed.
func streamingLifecycle(ec *a2asrv.ExecutorContext) []a2a.Event {
	art := a2a.NewArtifactEvent(ec, a2a.NewTextPart("Hello "))
	return []a2a.Event{
		a2a.NewSubmittedTask(ec, ec.Message),
		a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil),
		art,
		a2a.NewArtifactUpdateEvent(ec, art.Artifact.ID, a2a.NewTextPart("world")),
		a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil),
	}
}

func TestAgentInvocationE2E_StreamingHappyPath(t *testing.T) {
	h := newE2EHarness(t, a2atest.Script{Streaming: true, Events: streamingLifecycle}, nil)

	// Streaming returns immediately (status "pending"); the returned pointer is
	// then mutated by background goroutines, so assert only via the store.
	exec := h.invoke(context.Background(), t, nil)

	// Background goroutines finalize asynchronously — poll, never sleep.
	a2atest.Eventually(t, func() bool {
		return h.stored(t, exec.ID).Status == "success"
	}, "streaming execution should finalize as success")

	stored := h.stored(t, exec.ID)
	require.NotNil(t, stored.TaskID, "task_id persisted")
	require.NotNil(t, stored.ContextID, "context_id persisted")
	// Artifact content assembled from the appended chunks (not just statuses).
	assert.Equal(t, "Hello world", artifactText(stored))

	// Event rows: contiguous sequence numbers, only allowed types, cursor reads.
	events, err := h.eventStore.ListAfterSequence(context.Background(), exec.ID, -1)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	allowed := map[string]bool{"task": true, "status-update": true, "artifact-update": true}
	for i, e := range events {
		assert.Equal(t, i, e.SequenceNumber, "sequence numbers are contiguous from 0")
		assert.True(t, allowed[e.EventType], "unexpected event type %q", e.EventType)
	}
	// ListAfterSequence is a real cursor: reading after the first row drops it.
	after0, err := h.eventStore.ListAfterSequence(context.Background(), exec.ID, 0)
	require.NoError(t, err)
	assert.Len(t, after0, len(events)-1)
}

// TestAgentInvocationE2E_V03StreamingMessageReply guards this PR's fix: an agent
// that advertises A2A v0.3 and answers with a direct streamed *a2a.Message. It
// needs BOTH the a2av0 compat client transports (else NewFromCard fails with "no
// compatible transports found") AND the stream processor capturing a message
// reply as an artifact (else the reply text is dropped). Without either fix this
// test fails, which the v1.0-only harness tests could not catch.
func TestAgentInvocationE2E_V03StreamingMessageReply(t *testing.T) {
	h := newE2EHarness(t, a2atest.Script{
		ProtocolV03: true,
		Streaming:   true,
		Events:      messageEvents("v0.3 streamed answer"),
	}, nil)

	exec := h.invoke(context.Background(), t, nil)

	a2atest.Eventually(t, func() bool {
		return h.stored(t, exec.ID).Status == "success"
	}, "a v0.3 streaming message reply should finalize as success")
	assert.Equal(t, "v0.3 streamed answer", artifactText(h.stored(t, exec.ID)))
}

func TestAgentInvocationE2E_StreamingFailed(t *testing.T) {
	events := func(ec *a2asrv.ExecutorContext) []a2a.Event {
		return []a2a.Event{
			a2a.NewSubmittedTask(ec, ec.Message),
			a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil),
			a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed, nil),
		}
	}
	h := newE2EHarness(t, a2atest.Script{Streaming: true, Events: events}, nil)

	exec := h.invoke(context.Background(), t, nil)

	a2atest.Eventually(t, func() bool {
		return h.stored(t, exec.ID).Status == "error"
	}, "a failed stream should finalize as error")
}

func TestAgentInvocationE2E_StreamingConnectionDrop(t *testing.T) {
	// Drop the connection mid-stream; the execution must reach a terminal state,
	// never wedged in pending/working.
	h := newE2EHarness(t, a2atest.Script{Streaming: true, Events: streamingLifecycle, DropAfter: 2}, nil)

	exec := h.invoke(context.Background(), t, nil)

	a2atest.Eventually(t, func() bool {
		s := h.stored(t, exec.ID).Status
		return s != "pending" && s != "working"
	}, "a mid-stream drop must not leave the execution wedged")
	assert.True(t, isTerminalStatus(h.stored(t, exec.ID).Status))
}

// --- Conversation continuity --------------------------------------------

func TestAgentInvocationE2E_ConversationContinuity(t *testing.T) {
	events := func(ec *a2asrv.ExecutorContext) []a2a.Event {
		task := a2a.NewSubmittedTask(ec, ec.Message) // carries a context id
		task.Status = a2a.TaskStatus{State: a2a.TaskStateCompleted}
		return []a2a.Event{task}
	}
	h := newE2EHarness(t, a2atest.Script{Events: events}, nil)

	first := h.invoke(context.Background(), t, nil)
	require.NotNil(t, first.ContextID)
	require.NotNil(t, first.ConversationID)

	// Second message in the same conversation must reuse the stored context_id.
	second := h.invoke(context.Background(), t, first.ConversationID)
	require.NotEqual(t, first.ID, second.ID)

	msgs := h.srv.Recorder.Messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, *first.ContextID, msgs[1].ContextID,
		"the toy agent should receive the first execution's context_id on the second message")
}

// --- Auth ----------------------------------------------------------------

func TestAgentInvocationE2E_AuthBearerApplied(t *testing.T) {
	const token = "test-token"
	h := newE2EHarness(t,
		a2atest.Script{RequireAuth: "Bearer " + token, Events: messageEvents("ok")},
		func(agent *models.Agent) {
			agent.AgentCard.SecurityRequirements = a2a.SecurityRequirementsOptions{{"scheme": {}}}
			agent.AgentCard.SecuritySchemes = a2a.NamedSecuritySchemes{
				"scheme": a2a.HTTPAuthSecurityScheme{Scheme: "bearer"},
			}
			creds := models.AgentCredentials{"scheme": models.AgentCredential{Type: "apiKey", Value: token}}
			agent.Credentials = &creds
		},
	)

	exec := h.invoke(context.Background(), t, nil)

	assert.Equal(t, "success", exec.Status, "authenticated call should succeed")
	assert.Equal(t, "Bearer "+token, h.srv.Recorder.LastAuthHeader(),
		"the interceptor should apply the stored bearer credential")
}

// --- Non-streaming card → unary --------------------------------------------

func TestAgentInvocationE2E_NonStreamingCardUsesUnary(t *testing.T) {
	h := newE2EHarness(t, a2atest.Script{Streaming: false, Events: messageEvents("unary")}, nil)

	// A non-streaming card is invoked over the unary (message/send) path.
	client := newTestA2AHTTPClient()
	assert.False(t, client.SupportsStreaming(h.agent))

	exec := h.invoke(context.Background(), t, nil)
	assert.Equal(t, "success", exec.Status)
	assert.Equal(t, "unary", artifactText(h.stored(t, exec.ID)))
}
