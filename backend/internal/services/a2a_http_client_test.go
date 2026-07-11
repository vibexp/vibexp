package services

import (
	"context"
	"errors"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
)

// passthroughEncryption is an identity EncryptionService so that a stored
// credential's decrypted value equals the value under test.
type passthroughEncryption struct{}

func (passthroughEncryption) Encrypt(plaintext string) (string, error)  { return plaintext, nil }
func (passthroughEncryption) Decrypt(ciphertext string) (string, error) { return ciphertext, nil }

// testExecutor is a configurable a2asrv.AgentExecutor used to stand up an
// in-process A2A agent for client round-trip tests.
type testExecutor struct {
	events func(execCtx *a2asrv.ExecutorContext) []a2a.Event
	err    error
}

func (e *testExecutor) Execute(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if e.err != nil {
			yield(nil, e.err)
			return
		}
		for _, ev := range e.events(execCtx) {
			if !yield(ev, nil) {
				return
			}
		}
	}
}

func (e *testExecutor) Cancel(_ context.Context, _ *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(func(a2a.Event, error) bool) {}
}

// newTestA2AServer starts an in-process JSON-RPC A2A server backed by exec and
// returns its /invoke URL.
func newTestA2AServer(t *testing.T, exec a2asrv.AgentExecutor) string {
	t.Helper()
	handler := a2asrv.NewHandler(exec)
	mux := http.NewServeMux()
	mux.Handle("/invoke", a2asrv.NewJSONRPCHandler(handler))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/invoke"
}

// testAgent builds an agent whose card advertises the given JSON-RPC endpoint.
func testAgent(endpoint string, streaming bool) *models.Agent {
	return &models.Agent{
		ID:     "agent-1",
		UserID: "user-1",
		Status: "active",
		AgentCard: &models.AgentCard{
			Name:               "Test Agent",
			Description:        "test",
			Version:            "1.0.0",
			Capabilities:       a2a.AgentCapabilities{Streaming: streaming},
			DefaultInputModes:  []string{"text/plain"},
			DefaultOutputModes: []string{"text/plain"},
			SupportedInterfaces: []*a2a.AgentInterface{
				a2a.NewAgentInterface(endpoint, a2a.TransportProtocolJSONRPC),
			},
			Skills: []a2a.AgentSkill{{ID: "s", Name: "s", Description: "s", Tags: []string{"t"}}},
		},
	}
}

// newTestA2AHTTPClient builds a client whose SSRF guard permits loopback so
// tests can reach httptest servers.
func newTestA2AHTTPClient() *A2AHTTPClient {
	auth := NewAgentAuthenticator(passthroughEncryption{})
	return newA2AHTTPClient(auth, &config.Config{}, &ssrfGuard{allowPrivate: true})
}

func TestA2AHTTPClient_InvokeAgent_Message(t *testing.T) {
	endpoint := newTestA2AServer(t, &testExecutor{
		events: func(*a2asrv.ExecutorContext) []a2a.Event {
			return []a2a.Event{a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("hi"))}
		},
	})
	client := newTestA2AHTTPClient()

	execution, err := client.InvokeAgent(
		context.Background(), testAgent(endpoint, false), map[string]interface{}{"text": "hello"}, nil,
	)

	require.NoError(t, err)
	require.NotNil(t, execution)
	assert.Equal(t, "completed", execution.Status)
	assert.Nil(t, execution.Error)
	assert.NotNil(t, execution.Duration)
	// #163: the reply is persisted as artifacts.
	require.Len(t, execution.Artifacts, 1)
	parts, ok := execution.Artifacts[0]["parts"].([]interface{})
	require.True(t, ok)
	require.Len(t, parts, 1)
	assert.Equal(t, "hi", parts[0].(map[string]interface{})["text"])
}

func TestArtifactsFromMessage(t *testing.T) {
	arts := artifactsFromMessage(a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("the answer")))
	require.Len(t, arts, 1)
	parts, ok := arts[0]["parts"].([]interface{})
	require.True(t, ok)
	require.Len(t, parts, 1)
	assert.Equal(t, "the answer", parts[0].(map[string]interface{})["text"])

	assert.Nil(t, artifactsFromMessage(a2a.NewMessage(a2a.MessageRoleAgent)))
	assert.Nil(t, artifactsFromMessage(nil))
}

func TestTaskToExecution(t *testing.T) {
	// Terminal task → completed status + persisted artifacts.
	task := &a2a.Task{
		ID: "t1", ContextID: "c1",
		Status:    a2a.TaskStatus{State: a2a.TaskStateCompleted},
		Artifacts: []*a2a.Artifact{{ID: "a1", Parts: a2a.ContentParts{a2a.NewTextPart("done")}}},
	}
	exec := taskToExecution(task, 0)
	assert.Equal(t, "completed", exec.Status)
	require.NotNil(t, exec.TaskID)
	assert.Equal(t, "t1", *exec.TaskID)
	require.Len(t, exec.Artifacts, 1)

	// Non-terminal task → working, no artifacts yet.
	working := &a2a.Task{ID: "t2", Status: a2a.TaskStatus{State: a2a.TaskStateWorking}}
	exec2 := taskToExecution(working, 0)
	assert.Equal(t, "working", exec2.Status)
	assert.Nil(t, exec2.Artifacts)
}

func TestA2AHTTPClient_InvokeAgent_Error(t *testing.T) {
	endpoint := newTestA2AServer(t, &testExecutor{err: errors.New("boom")})
	client := newTestA2AHTTPClient()

	execution, err := client.InvokeAgent(
		context.Background(), testAgent(endpoint, false), map[string]interface{}{"text": "hello"}, nil,
	)

	require.Error(t, err)
	assert.Nil(t, execution)
	assert.Contains(t, err.Error(), "agent message send failed")
}

func TestA2AHTTPClient_InvokeAgentStreaming(t *testing.T) {
	endpoint := newTestA2AServer(t, &testExecutor{
		events: func(*a2asrv.ExecutorContext) []a2a.Event {
			return []a2a.Event{a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("streamed"))}
		},
	})
	client := newTestA2AHTTPClient()

	eventChan := make(chan a2a.Event, 16)
	err := client.InvokeAgentStreaming(
		context.Background(), testAgent(endpoint, true), map[string]interface{}{"text": "hello"}, nil, eventChan,
	)
	close(eventChan)

	require.NoError(t, err)
	received := 0
	for range eventChan {
		received++
	}
	assert.Positive(t, received, "expected at least one streamed event")
}

func TestA2AHTTPClient_InvokeAgent_SSRFBlocked(t *testing.T) {
	// Production client (strict guard) must reject a loopback endpoint.
	client := NewA2AHTTPClient(NewAgentAuthenticator(passthroughEncryption{}), &config.Config{})
	agent := testAgent("http://127.0.0.1:9/invoke", false)

	_, err := client.InvokeAgent(context.Background(), agent, map[string]interface{}{"text": "x"}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent endpoint is not allowed")
}

func TestA2AHTTPClient_buildClient_Errors(t *testing.T) {
	client := newTestA2AHTTPClient()

	_, err := client.InvokeAgent(context.Background(), &models.Agent{}, map[string]interface{}{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent card is missing")

	noIface := &models.Agent{AgentCard: &models.AgentCard{Name: "x"}}
	_, err = client.InvokeAgent(context.Background(), noIface, map[string]interface{}{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no supported interfaces")
}

type capturingRoundTripper struct{ req *http.Request }

func (c *capturingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.req = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

// TestAgentAuthRoundTripper covers the auth matrix at the transport seam; the
// full scheme matrix is exercised in agent_authenticator_test.go.
//
//nolint:funlen // table-driven auth matrix
func TestAgentAuthRoundTripper(t *testing.T) {
	cred := func(v string) *models.AgentCredentials {
		c := models.AgentCredentials{"scheme": models.AgentCredential{Type: "apiKey", Value: v}}
		return &c
	}
	cardWith := func(scheme a2a.SecurityScheme) *models.AgentCard {
		return &models.AgentCard{
			Name:                 "a",
			SecurityRequirements: a2a.SecurityRequirementsOptions{{"scheme": {}}},
			SecuritySchemes:      a2a.NamedSecuritySchemes{"scheme": scheme},
		}
	}

	tests := []struct {
		name   string
		agent  *models.Agent
		assert func(t *testing.T, req *http.Request)
	}{
		{
			name: "apiKey header",
			agent: &models.Agent{
				AgentCard: cardWith(
					a2a.APIKeySecurityScheme{Name: "X-API-Key", Location: a2a.APIKeySecuritySchemeLocationHeader},
				),
				Credentials: cred("secret-123"),
			},
			assert: func(t *testing.T, req *http.Request) {
				assert.Equal(t, "secret-123", req.Header.Get("X-API-Key"))
			},
		},
		{
			name: "apiKey query",
			agent: &models.Agent{
				AgentCard: cardWith(
					a2a.APIKeySecurityScheme{Name: "api_key", Location: a2a.APIKeySecuritySchemeLocationQuery},
				),
				Credentials: cred("q-456"),
			},
			assert: func(t *testing.T, req *http.Request) {
				assert.Equal(t, "q-456", req.URL.Query().Get("api_key"))
			},
		},
		{
			name: "http bearer",
			agent: &models.Agent{
				AgentCard:   cardWith(a2a.HTTPAuthSecurityScheme{Scheme: "bearer"}),
				Credentials: cred("tok-789"),
			},
			assert: func(t *testing.T, req *http.Request) {
				assert.Equal(t, "Bearer tok-789", req.Header.Get("Authorization"))
			},
		},
		{
			name:  "no credentials",
			agent: &models.Agent{AgentCard: &models.AgentCard{Name: "a"}},
			assert: func(t *testing.T, req *http.Request) {
				assert.Empty(t, req.Header.Get("Authorization"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturer := &capturingRoundTripper{}
			rt := &agentAuthRoundTripper{
				base:          capturer,
				authenticator: NewAgentAuthenticator(passthroughEncryption{}),
				agent:         tt.agent,
			}
			req, err := http.NewRequest("POST", "http://agent.example/invoke", nil)
			require.NoError(t, err)

			resp, err := rt.RoundTrip(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.NotNil(t, capturer.req)
			tt.assert(t, capturer.req)
		})
	}
}

func TestBuildA2AMessage(t *testing.T) {
	ctxID := "ctx-1"
	msg := buildA2AMessage(map[string]interface{}{"text": "hello"}, &ctxID)
	assert.Equal(t, a2a.MessageRoleUser, msg.Role)
	assert.Equal(t, "ctx-1", msg.ContextID)
	require.Len(t, msg.Parts, 1)
	assert.Equal(t, "hello", msg.Parts[0].Text())

	// Non-string input is stringified.
	msg2 := buildA2AMessage(map[string]interface{}{"n": 42}, nil)
	assert.Empty(t, msg2.ContextID)
	assert.Contains(t, msg2.Parts[0].Text(), "42")
}

func TestMapTaskStateToStatus(t *testing.T) {
	cases := map[a2a.TaskState]string{
		a2a.TaskStateCompleted:     "completed",
		a2a.TaskStateFailed:        "failed",
		a2a.TaskStateRejected:      "failed",
		a2a.TaskStateCanceled:      "cancelled",
		a2a.TaskStateWorking:       "working",
		a2a.TaskStateSubmitted:     "working",
		a2a.TaskStateInputRequired: "working",
	}
	for state, want := range cases {
		assert.Equal(t, want, mapTaskStateToStatus(state), "state %s", state)
	}
}

func TestMapSendResultToExecution(t *testing.T) {
	// Message result -> completed.
	exec := mapSendResultToExecution(a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("x")), 5*time.Millisecond)
	assert.Equal(t, "completed", exec.Status)
	assert.Nil(t, exec.TaskID)

	// Task result -> mapped state + captured ids.
	task := &a2a.Task{ID: "task-9", ContextID: "ctx-9", Status: a2a.TaskStatus{State: a2a.TaskStateCompleted}}
	exec = mapSendResultToExecution(task, time.Millisecond)
	assert.Equal(t, "completed", exec.Status)
	require.NotNil(t, exec.TaskID)
	assert.Equal(t, "task-9", *exec.TaskID)
	require.NotNil(t, exec.ContextID)
	assert.Equal(t, "ctx-9", *exec.ContextID)
	require.NotNil(t, exec.CurrentState)
}

func TestA2AHTTPClient_SupportsStreaming(t *testing.T) {
	client := newTestA2AHTTPClient()
	assert.True(t, client.SupportsStreaming(testAgent("http://x/invoke", true)))
	assert.False(t, client.SupportsStreaming(testAgent("http://x/invoke", false)))
	assert.False(t, client.SupportsStreaming(&models.Agent{}))
}
