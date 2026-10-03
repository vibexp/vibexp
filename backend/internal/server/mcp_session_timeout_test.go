package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

const (
	mcpInitializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{` +
		`"protocolVersion":"2025-03-26","capabilities":{},` +
		`"clientInfo":{"name":"session-timeout-test","version":"1.0.0"}}}`
	mcpInitializedBody = `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	mcpToolsListBody   = `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
)

// mcpSessionTestClient talks to the real /mcp/v1/common handler over HTTP the way
// an MCP client does, minus the part under test: it never sends DELETE while
// the test runs.
type mcpSessionTestClient struct {
	t          *testing.T
	url        string
	client     *http.Client
	sessionIDs []string
}

// newMCPSessionTestClient serves createMCPHandlerCommon (the production
// constructor, so the test fails if it stops passing the timeout) behind
// httptest with the given mcp.session_timeout. The user ID the flexible-auth
// middleware would set is injected directly.
func newMCPSessionTestClient(t *testing.T, sessionTimeout time.Duration) *mcpSessionTestClient {
	t.Helper()

	srv := newServerWithNullLogger(t)
	srv.config.MCP.SessionTimeout = sessionTimeout
	mockTeam := mocks.NewMockTeamServiceInterface(t)
	mockTeam.On("ListTeams", mock.Anything, testMemberUserID, mock.AnythingOfType("int"), mock.AnythingOfType("int")).
		Return(&models.TeamListResponse{}, nil).Maybe()
	srv.container = &TestContainer{TeamServiceMock: mockTeam}

	mcpHandler := srv.createMCPHandlerCommon()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), contextkeys.UserID, testMemberUserID)
		mcpHandler.ServeHTTP(w, r.WithContext(ctx))
	}))

	transport := &http.Transport{}
	c := &mcpSessionTestClient{t: t, url: ts.URL, client: &http.Client{Transport: transport}}
	t.Cleanup(func() {
		c.deleteSessions()
		transport.CloseIdleConnections()
		ts.Close()
	})
	return c
}

// deleteSessions ends every session the test opened, so none outlives it and
// shows up in the next test's goroutine baseline. A session the timeout already
// evicted answers 404, which is fine here.
func (c *mcpSessionTestClient) deleteSessions() {
	for _, id := range c.sessionIDs {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, c.url, http.NoBody)
		if err != nil {
			c.t.Logf("build DELETE for session %s: %v", id, err)
			continue
		}
		req.Header.Set("Mcp-Session-Id", id)
		resp, err := c.client.Do(req)
		if err != nil {
			c.t.Logf("DELETE session %s: %v", id, err)
			continue
		}
		if err := resp.Body.Close(); err != nil {
			c.t.Logf("close DELETE response for session %s: %v", id, err)
		}
	}
}

// post sends one JSON-RPC message and returns the status code and the session
// ID the server answered with. The body is drained so the connection is reused.
func (c *mcpSessionTestClient) post(sessionID, body string) (status int, gotSessionID string) {
	c.t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.url, strings.NewReader(body))
	require.NoError(c.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := c.client.Do(req)
	require.NoError(c.t, err)
	_, err = io.Copy(io.Discard, resp.Body)
	require.NoError(c.t, err)
	require.NoError(c.t, resp.Body.Close())
	return resp.StatusCode, resp.Header.Get("Mcp-Session-Id")
}

// openSession initializes a session and returns its ID. It is never deleted.
func (c *mcpSessionTestClient) openSession() string {
	c.t.Helper()

	status, sessionID := c.post("", mcpInitializeBody)
	require.Equal(c.t, http.StatusOK, status, "initialize must succeed")
	require.NotEmpty(c.t, sessionID, "initialize must hand back an Mcp-Session-Id")
	c.sessionIDs = append(c.sessionIDs, sessionID)

	status, _ = c.post(sessionID, mcpInitializedBody)
	require.Equal(c.t, http.StatusAccepted, status, "notifications/initialized must be accepted")
	return sessionID
}

// mcpSDKGoroutines counts the goroutines currently running MCP SDK code, which
// is what an open session parks. The process-wide runtime.NumGoroutine() cannot
// be compared against a baseline in this package: every server.New leaves
// telemetry exporters running, and their goroutines come and go on their own
// schedule.
func mcpSDKGoroutines() int {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}

	count := 0
	for stack := range strings.SplitSeq(string(buf), "\n\n") {
		if strings.Contains(stack, "github.com/modelcontextprotocol/go-sdk") {
			count++
		}
	}
	return count
}

// waitFor polls cond on the calling goroutine until it holds or the deadline
// passes, and reports whether it held. It stands in for require.Eventually,
// which runs its condition on a goroutine of its own, where a failing require
// inside post could not stop the test.
func waitFor(deadline, interval time.Duration, cond func() bool) bool {
	end := time.Now().Add(deadline)
	for {
		if cond() {
			return true
		}
		if time.Now().After(end) {
			return false
		}
		time.Sleep(interval)
	}
}

// TestMCPSessionTimeout_EvictsAbandonedSessions is the regression guard for
// #1275: sessions a client opens and never deletes must be closed once they
// have been idle for mcp.session_timeout, giving back both the session (404 on
// its old ID) and the goroutine each one parks.
func TestMCPSessionTimeout_EvictsAbandonedSessions(t *testing.T) {
	const (
		sessions = 5
		// Several times what opening the sessions takes (about 170ms each
		// under -race), so all of them are still open when the goroutines are
		// counted below.
		sessionTimeout = 3 * time.Second
	)

	c := newMCPSessionTestClient(t, sessionTimeout)
	baseline := mcpSDKGoroutines()

	ids := make([]string, 0, sessions)
	for range sessions {
		ids = append(ids, c.openSession())
	}
	require.GreaterOrEqual(t, mcpSDKGoroutines(), baseline+sessions,
		"each open session should park at least one SDK goroutine, or the baseline check below proves nothing")

	// Probing a live session would count as activity and re-arm its timer, so
	// sleep past the timeout first and only then look.
	time.Sleep(sessionTimeout + time.Second)
	for _, id := range ids {
		status, _ := c.post(id, mcpToolsListBody)
		require.Equal(t, http.StatusNotFound, status, "abandoned session %s must be evicted and answer 404", id)
	}

	// Closing a session is what ends its goroutine, and that finishes just
	// after the session leaves the handler's map, so give it a moment.
	evicted := waitFor(5*time.Second, 20*time.Millisecond, func() bool {
		return mcpSDKGoroutines() <= baseline
	})
	assert.True(t, evicted, "SDK goroutines must return to the pre-test baseline (%d) once the sessions are evicted, got %d",
		baseline, mcpSDKGoroutines())
}

// TestMCPSessionTimeout_KeepsActiveSession proves the timeout is an idle
// timeout: a session that keeps sending requests outlives it several times
// over, while a session opened alongside it and left alone is evicted.
func TestMCPSessionTimeout_KeepsActiveSession(t *testing.T) {
	const (
		sessionTimeout = time.Second
		pingInterval   = 100 * time.Millisecond
	)

	c := newMCPSessionTestClient(t, sessionTimeout)
	active := c.openSession()
	abandoned := c.openSession()

	deadline := time.Now().Add(3 * sessionTimeout)
	for time.Now().Before(deadline) {
		status, _ := c.post(active, mcpToolsListBody)
		require.Equal(t, http.StatusOK, status, "a session active within the timeout must not be closed")
		time.Sleep(pingInterval)
	}

	status, _ := c.post(abandoned, mcpToolsListBody)
	assert.Equal(t, http.StatusNotFound, status,
		"the idle session must be gone by now, or the timeout was never armed and the loop above proved nothing")
}
