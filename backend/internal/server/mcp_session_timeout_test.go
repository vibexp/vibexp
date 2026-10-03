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
// an MCP client does, minus the part under test: it never sends DELETE.
type mcpSessionTestClient struct {
	t      *testing.T
	url    string
	client *http.Client
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
		transport.CloseIdleConnections()
		ts.Close()
	})
	return c
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

	status, _ = c.post(sessionID, mcpInitializedBody)
	require.Equal(c.t, http.StatusAccepted, status, "notifications/initialized must be accepted")
	return sessionID
}

// TestMCPSessionTimeout_EvictsAbandonedSessions is the regression guard for
// #1275: sessions a client opens and never deletes must be closed once they
// have been idle for mcp.session_timeout, giving back both the session (404 on
// its old ID) and the goroutine each one parks.
func TestMCPSessionTimeout_EvictsAbandonedSessions(t *testing.T) {
	const (
		sessions = 20
		// Long enough that every session is still open when the goroutine
		// count is sampled below: building a session's MCP server is slow
		// under -race, and a shorter timeout evicts the first sessions while
		// the last are still being opened.
		sessionTimeout = 3 * time.Second
	)

	c := newMCPSessionTestClient(t, sessionTimeout)
	// Sampled after the server is up, so its own background goroutines and the
	// httptest accept loop are in the baseline and only sessions move it.
	baseline := runtime.NumGoroutine()

	ids := make([]string, 0, sessions)
	for range sessions {
		ids = append(ids, c.openSession())
	}
	require.GreaterOrEqual(t, runtime.NumGoroutine(), baseline+sessions,
		"each open session should hold at least one goroutine, or the baseline check below proves nothing")

	// Probing a live session would count as activity and re-arm its timer, so
	// sleep past the timeout first and only then look.
	time.Sleep(sessionTimeout + 500*time.Millisecond)
	for _, id := range ids {
		require.Eventually(t, func() bool {
			status, _ := c.post(id, mcpToolsListBody)
			return status == http.StatusNotFound
		}, 3*sessionTimeout, sessionTimeout+500*time.Millisecond, "abandoned session %s must be evicted and answer 404", id)
	}

	// Drop the client's keep-alive connections so only leaked session
	// goroutines could keep the count above the baseline.
	c.client.CloseIdleConnections()
	// Polled inline rather than with assert.Eventually, which runs its
	// condition on a goroutine of its own and so moves the number it reads.
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > baseline && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	assert.LessOrEqual(t, runtime.NumGoroutine(), baseline,
		"goroutines must return to the pre-test baseline once the sessions are evicted")
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
