package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

const (
	logAddedAllTools    = "Added all MCP tools to server"
	logAddedUserPrompts = "Added user prompts to MCP server across teams"
)

// messageCountingHandler is a slog.Handler that counts records by message.
type messageCountingHandler struct {
	mu     sync.Mutex
	counts map[string]int
}

func (h *messageCountingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *messageCountingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.counts[r.Message]++
	return nil
}

func (h *messageCountingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *messageCountingHandler) WithGroup(string) slog.Handler { return h }

func (h *messageCountingHandler) count(msg string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.counts[msg]
}

// mcpConstructionFixture serves the real /mcp/v1/common handler over HTTP for an
// authenticated user and records everything a populated server construction
// leaves behind: the team and prompt queries, and the two Info log lines.
type mcpConstructionFixture struct {
	url        string
	teams      *mocks.MockTeamServiceInterface
	prompts    *mocks.MockPromptServiceInterface
	logs       *messageCountingHandler
	httpClient *http.Client
	// requests counts every HTTP request; sessionPosts counts the ones that can
	// create a session (a POST without a session id).
	requests     *atomic.Int64
	sessionPosts *atomic.Int64
}

func newMCPConstructionFixture(t *testing.T) *mcpConstructionFixture {
	t.Helper()

	logs := &messageCountingHandler{counts: map[string]int{}}
	logger := slog.New(logs)

	// AddAllTools logs through the default logger. Top-level parallel tests only
	// resume once this (serial) test has returned, so swapping it here is safe.
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(previous) })

	srv := New("8080", nil, "test-api-key", &config.Config{}, logger)

	mockTeam := mocks.NewMockTeamServiceInterface(t)
	mockPrompt := mocks.NewMockPromptServiceInterface(t)
	srv.container = &TestContainer{TeamServiceMock: mockTeam, PromptServiceMock: mockPrompt}

	stubListTeams(mockTeam, []models.Team{memberTeam()})
	mockPrompt.On("ListPrompts", testMemberUserID, mock.Anything).Return(&models.PromptListResponse{
		Prompts: []models.Prompt{{Slug: "deploy", Description: "deploy", Body: "hello", TeamID: testTeamUUID}},
	}, nil).Maybe()
	mockPrompt.On("ExtractAllPlaceholders", testTeamUUID, "hello", mock.Anything).Return([]string{}, nil).Maybe()

	requests, sessionPosts := &atomic.Int64{}, &atomic.Int64{}
	mcpHandler := srv.createMCPHandlerCommon()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method == http.MethodPost && r.Header.Get(mcpSessionIDHeader) == "" {
			sessionPosts.Add(1)
		}
		ctx := context.WithValue(r.Context(), contextkeys.UserID, testMemberUserID)
		mcpHandler.ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(httpServer.Close)

	return &mcpConstructionFixture{
		url:          httpServer.URL,
		teams:        mockTeam,
		prompts:      mockPrompt,
		logs:         logs,
		httpClient:   httpServer.Client(),
		requests:     requests,
		sessionPosts: sessionPosts,
	}
}

// connect opens a real streamable-HTTP client session against the fixture. The
// SDK client sends two session-less POSTs to do so — a server/discover probe,
// then initialize — and each of them builds one populated server.
func (f *mcpConstructionFixture) connect(t *testing.T) *mcp.ClientSession {
	t.Helper()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   f.url,
		HTTPClient: f.httpClient,
	}, nil)
	require.NoError(t, err)
	return session
}

// assertPopulatedConstructions asserts how many populated servers were built so
// far, through each trace a construction leaves.
func (f *mcpConstructionFixture) assertPopulatedConstructions(t *testing.T, want int) {
	t.Helper()

	f.teams.AssertNumberOfCalls(t, "ListTeams", want)
	f.prompts.AssertNumberOfCalls(t, "ListPrompts", want)
	assert.Equal(t, want, f.logs.count(logAddedAllTools), "tools log line count")
	assert.Equal(t, want, f.logs.count(logAddedUserPrompts), "prompts log line count")
}

// rawRequest sends one hand-built MCP HTTP request and returns its status code.
func (f *mcpConstructionFixture) rawRequest(t *testing.T, method, body string, headers map[string]string) int {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), method, f.url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := f.httpClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode
}

// TestMCPHandler_InitializeBuildsPopulatedServerOnce pins the session-creation
// half of #1276: the SDK calls getServer twice while serving an initialize
// request, and both calls must share one populated server.
func TestMCPHandler_InitializeBuildsPopulatedServerOnce(t *testing.T) {
	f := newMCPConstructionFixture(t)

	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{` +
		`"protocolVersion":"2025-06-18","capabilities":{},` +
		`"clientInfo":{"name":"raw-client","version":"1.0.0"}}}`
	status := f.rawRequest(t, http.MethodPost, initialize, nil)

	assert.Equal(t, http.StatusOK, status)
	f.assertPopulatedConstructions(t, 1)
}

// TestMCPHandler_ExistingSessionRequestsBuildNothing pins the per-request half
// of #1276: once a session exists, no request on it — tool and prompt listing,
// pings, the standalone GET stream, the closing DELETE — builds a populated
// server (tools + the user's prompts from the database).
func TestMCPHandler_ExistingSessionRequestsBuildNothing(t *testing.T) {
	f := newMCPConstructionFixture(t)
	ctx := context.Background()

	session := f.connect(t)
	built := int(f.sessionPosts.Load())
	f.assertPopulatedConstructions(t, built)

	tools, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	toolNames := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		toolNames = append(toolNames, tool.Name)
	}
	assert.Contains(t, toolNames, "vibexp_io_get_user", "the session serves the populated server's tools")

	prompts, err := session.ListPrompts(ctx, nil)
	require.NoError(t, err)
	require.Len(t, prompts.Prompts, 1)
	assert.Equal(t, "deploy", prompts.Prompts[0].Name)

	const pings = 20
	for range pings {
		require.NoError(t, session.Ping(ctx, nil))
	}

	require.NoError(t, session.Close())

	assert.Greater(t, f.requests.Load(), int64(pings), "the session made more HTTP requests than pings")
	assert.Equal(t, built, int(f.sessionPosts.Load()), "no further session-creating request was sent")
	f.assertPopulatedConstructions(t, built)
}

// TestMCPHandler_PopulatedServerBuiltPerSession verifies each new session gets
// its own populated server: the shared bare server is never handed to a session.
func TestMCPHandler_PopulatedServerBuiltPerSession(t *testing.T) {
	f := newMCPConstructionFixture(t)
	ctx := context.Background()

	const sessions = 3
	for i := range sessions {
		session := f.connect(t)
		tools, err := session.ListTools(ctx, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, tools.Tools, "session %d lists tools", i)
		require.NoError(t, session.Close())
	}

	built := int(f.sessionPosts.Load())
	assert.GreaterOrEqual(t, built, sessions)
	f.assertPopulatedConstructions(t, built)
}

// TestMCPHandler_SessionlessAndRejectedRequestsBuildNothing verifies the
// requests the SDK answers without a session's server build no populated server,
// and that the MCP-Protocol-Version check still rejects an unsupported version.
func TestMCPHandler_SessionlessAndRejectedRequestsBuildNothing(t *testing.T) {
	f := newMCPConstructionFixture(t)

	session := f.connect(t)
	t.Cleanup(func() { assert.NoError(t, session.Close()) })
	sessionID := session.ID()
	require.NotEmpty(t, sessionID)
	built := int(f.sessionPosts.Load())
	f.assertPopulatedConstructions(t, built)

	const ping = `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	tests := []struct {
		name    string
		method  string
		body    string
		headers map[string]string
		want    int
	}{
		{
			name:    "unsupported protocol version on an existing session",
			method:  http.MethodPost,
			body:    ping,
			headers: map[string]string{mcpSessionIDHeader: sessionID, "Mcp-Protocol-Version": "1999-01-01"},
			want:    http.StatusBadRequest,
		},
		{
			name:    "supported request on an existing session",
			method:  http.MethodPost,
			body:    ping,
			headers: map[string]string{mcpSessionIDHeader: sessionID},
			want:    http.StatusOK,
		},
		{
			name:    "unknown session id",
			method:  http.MethodPost,
			body:    ping,
			headers: map[string]string{mcpSessionIDHeader: "no-such-session"},
			want:    http.StatusNotFound,
		},
		{
			name:   "GET without a session id",
			method: http.MethodGet,
			want:   http.StatusBadRequest,
		},
		{
			name:   "DELETE without a session id",
			method: http.MethodDelete,
			want:   http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, f.rawRequest(t, tt.method, tt.body, tt.headers))
			f.assertPopulatedConstructions(t, built)
		})
	}
}

// TestMCPHandler_UnsupportedProtocolVersionOnNewSessionRejected verifies the
// version check also holds on the session-creating path, where the populated
// server (not the bare one) supplies the supported versions.
func TestMCPHandler_UnsupportedProtocolVersionOnNewSessionRejected(t *testing.T) {
	f := newMCPConstructionFixture(t)

	status := f.rawRequest(t, http.MethodPost, `{"jsonrpc":"2.0","id":1,"method":"ping"}`,
		map[string]string{"Mcp-Protocol-Version": "1999-01-01"})

	assert.Equal(t, http.StatusBadRequest, status)
}
