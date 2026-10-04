package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
)

// connectCompactTestClient builds the compact surface for a test user and
// returns a client connected to it in memory.
func connectCompactTestClient(t *testing.T) *mcp.ClientSession {
	t.Helper()

	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
	inner, catalog, err := connectInnerCatalog(NewMCPToolsManager(srv), "test-user")
	require.NoError(t, err)
	closeOnCleanup(t, inner)

	outer := newMCPServer()
	addCompactTools(outer, inner, catalog)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	_, err = outer.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil).
		Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	closeOnCleanup(t, session)
	return session
}

// closeOnCleanup closes session when the test ends, logging a failure to close.
func closeOnCleanup(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Logf("session.Close: %v", err)
		}
	})
}

func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	return text.Text
}

func TestCompactSurfaceExposesTwoTools(t *testing.T) {
	session := connectCompactTestClient(t)

	list, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, list.Tools, 2)

	compactBytes, err := json.Marshal(list.Tools)
	require.NoError(t, err)

	full := mcp.NewServer(&mcp.Implementation{Name: "full", Version: "1.0.0"}, nil)
	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
	NewMCPToolsManager(srv).AddAllTools(full, "test-user")
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	_, err = full.Connect(context.Background(), serverTransport, nil)
	require.NoError(t, err)
	fullSession, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1.0.0"}, nil).
		Connect(context.Background(), clientTransport, nil)
	require.NoError(t, err)
	closeOnCleanup(t, fullSession)
	fullList, err := fullSession.ListTools(context.Background(), nil)
	require.NoError(t, err)
	fullBytes, err := json.Marshal(fullList.Tools)
	require.NoError(t, err)

	t.Logf("tools/list bytes: full=%d (%d tools) compact=%d (2 tools)", len(fullBytes), len(fullList.Tools), len(compactBytes))
	assert.Less(t, len(compactBytes), len(fullBytes)/4)
}

func TestCompactSearchToolsReturnsDefinitions(t *testing.T) {
	session := connectCompactTestClient(t)
	ctx := context.Background()

	byName, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      searchToolsToolName,
		Arguments: map[string]any{"names": []string{"vibexp_io_create_memory"}},
	})
	require.NoError(t, err)
	require.False(t, byName.IsError)
	text := resultText(t, byName)
	assert.Contains(t, text, `"vibexp_io_create_memory"`)
	assert.Contains(t, text, `"required"`)
	t.Logf("names lookup bytes=%d", len(text))

	byQuery, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      searchToolsToolName,
		Arguments: map[string]any{"query": "create memory"},
	})
	require.NoError(t, err)
	require.False(t, byQuery.IsError)
	assert.Contains(t, resultText(t, byQuery), `"vibexp_io_create_memory"`)
	t.Logf("query lookup bytes=%d", len(resultText(t, byQuery)))

	shortName, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      searchToolsToolName,
		Arguments: map[string]any{"names": []string{"create_memory"}},
	})
	require.NoError(t, err)
	require.False(t, shortName.IsError)
	assert.Contains(t, resultText(t, shortName), `"vibexp_io_create_memory"`)

	unknown, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      searchToolsToolName,
		Arguments: map[string]any{"names": []string{"vibexp_io_nope"}},
	})
	require.NoError(t, err)
	assert.True(t, unknown.IsError)
	assert.Contains(t, resultText(t, unknown), "vibexp_io_search")
}

// The point of forwarding through an inner server: a bad payload is rejected by
// the SDK's schema validation with the same field-level message as today.
func TestCompactCallToolKeepsFieldLevelErrors(t *testing.T) {
	session := connectCompactTestClient(t)
	ctx := context.Background()

	cases := map[string]map[string]any{
		"missing required": {"team_id": "t"},
		"wrong type":       {"team_id": "t", "project_id": "p", "text": 5},
		"unknown field":    {"team_id": "t", "project_id": "p", "text": "x", "bogus": true},
	}
	for name, arguments := range cases {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      callToolToolName,
			Arguments: map[string]any{"name": "vibexp_io_create_memory", "arguments": arguments},
		})
		require.NoError(t, err, name)
		require.True(t, result.IsError, name)
		t.Logf("%s -> %s", name, resultText(t, result))
	}

	unknownTool, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      callToolToolName,
		Arguments: map[string]any{"name": "vibexp_io_nope"},
	})
	require.NoError(t, err)
	assert.True(t, unknownTool.IsError)
	assert.Contains(t, resultText(t, unknownTool), "Valid names")
}

func TestCompactSplitExecutorsEnforceRiskClass(t *testing.T) {
	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
	inner, catalog, err := connectInnerCatalog(NewMCPToolsManager(srv), "test-user")
	require.NoError(t, err)
	closeOnCleanup(t, inner)

	outer := newCompactMCPServer()
	addCompactSplitTools(outer, inner, catalog)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	_, err = outer.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil).
		Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	closeOnCleanup(t, session)

	list, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, list.Tools, 4)
	listBytes, err := json.Marshal(list.Tools)
	require.NoError(t, err)
	t.Logf("split tools/list bytes=%d", len(listBytes))

	classes := map[compactRiskClass]int{}
	for _, tool := range catalog {
		classes[compactRiskOf(tool.Name)]++
	}
	t.Logf("catalog classes: read=%d write=%d delete=%d", classes[compactRiskRead], classes[compactRiskWrite], classes[compactRiskDelete])

	wrongClass, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      callReadToolToolName,
		Arguments: map[string]any{"name": "vibexp_io_delete_resource", "arguments": map[string]any{}},
	})
	require.NoError(t, err)
	require.True(t, wrongClass.IsError)
	assert.Contains(t, resultText(t, wrongClass), "vibexp_io_call_delete_tool")

	rightClass, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      callWriteToolToolName,
		Arguments: map[string]any{"name": "vibexp_io_create_memory", "arguments": map[string]any{"team_id": "t"}},
	})
	require.NoError(t, err)
	require.True(t, rightClass.IsError)
	assert.Contains(t, resultText(t, rightClass), "missing properties")
}

func TestCompactCoreKeepsLoopToolsTyped(t *testing.T) {
	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
	inner, catalog, err := connectInnerCatalog(NewMCPToolsManager(srv), "test-user")
	require.NoError(t, err)
	closeOnCleanup(t, inner)

	outer := newCompactMCPServer()
	addCompactCoreTools(outer, inner, catalog)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	_, err = outer.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil).
		Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	closeOnCleanup(t, session)

	list, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, list.Tools, len(compactCoreTools)+2)
	listBytes, err := json.Marshal(list.Tools)
	require.NoError(t, err)
	t.Logf("core tools/list bytes=%d (%d tools)", len(listBytes), len(list.Tools))

	direct, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "vibexp_io_create_memory",
		Arguments: map[string]any{"team_id": "t"},
	})
	require.NoError(t, err)
	require.True(t, direct.IsError)
	assert.Contains(t, resultText(t, direct), "missing properties")

	viaPair, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      searchToolsToolName,
		Arguments: map[string]any{"names": []string{"link_resources"}},
	})
	require.NoError(t, err)
	require.False(t, viaPair.IsError)
	assert.Contains(t, resultText(t, viaPair), `"vibexp_io_link_resources"`)
}
