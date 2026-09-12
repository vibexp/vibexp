package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
)

// The SPA's /mcp page documents the MCP tools from a hand-written catalog under
// frontend/src/pages/mcp. Nothing in Go references those files, so the catalog
// used to drift silently: at the time this test was written it documented 18 of
// the 27 tools AddAllTools registers, and the page's own "the N tools your agent
// can call" headline was wrong by a third (#939). This test is the gate that
// turns that invisible drift into a red build on the PR that causes it.
//
// It deliberately compares NAMES only. Descriptions and parameter lists are
// curated prose written for a human reading the page, so they are neither
// derived from nor identical to the Go schemas; pinning them here would gate
// the wrong thing.

// catalogGlob matches the curated catalog modules: the main file plus the
// per-domain modules spread into it (mcp-tools-memory.ts, -blueprint.ts, ...).
// It is intentionally NOT recursive, so the pinned tool names in
// __tests__/mcp-tools.test.ts -- which include names that are deliberately
// absent from the catalog -- are not read as catalog entries.
const catalogGlob = "frontend/src/pages/mcp/mcp-tools*.ts"

// catalogNamePattern matches a catalog entry's `name:` field specifically,
// rather than every vibexp_io_* token in the file. Tool descriptions reference
// other tools by name in prose ("Call vibexp_io_list_teams_and_projects
// first..."), and a looser pattern would read those references as entries.
var catalogNamePattern = regexp.MustCompile(`name:\s*'(vibexp_io_[a-z0-9_]+)'`)

// catalogOmissions lists tools that are registered on the MCP server but are
// deliberately NOT documented on the /mcp page. It is empty on purpose: every
// registered tool is currently documented. Adding an entry here is how an
// intentional omission is made visible -- with the reason written down -- rather
// than silently absent.
var catalogOmissions = map[string]string{}

func TestFrontendMCPCatalogMatchesRegisteredTools(t *testing.T) {
	registered := registeredMCPToolNames(t)
	require.NotEmpty(t, registered, "AddAllTools registered no tools")

	documented := documentedMCPToolNames(t)
	require.NotEmpty(t, documented, "the frontend catalog parsed to zero tool names; has %s moved?", catalogGlob)

	for name := range registered {
		if reason, omitted := catalogOmissions[name]; omitted {
			assert.NotContains(t, documented, name,
				"%s is listed in catalogOmissions (%s) but IS documented -- drop the omission entry", name, reason)
			continue
		}
		assert.Contains(t, documented, name,
			"%s is registered by AddAllTools but missing from the frontend MCP catalog (%s); "+
				"add it to a per-domain mcp-tools-*.ts module, or record a reason in catalogOmissions",
			name, catalogGlob)
	}

	for name := range documented {
		assert.Contains(t, registered, name,
			"%s is documented in the frontend MCP catalog (%s) but is not registered by AddAllTools; "+
				"remove it from the catalog", name, catalogGlob)
	}
}

// registeredMCPToolNames builds the real tool set through AddAllTools and lists
// it over an in-memory client session -- the same construction the other
// registration tests use. Reading the names any other way (grepping the
// package, say) would pick up the vibexp_io_test* fixtures in _test.go files,
// which are not registrations.
func registeredMCPToolNames(t *testing.T) map[string]struct{} {
	t.Helper()

	srv := New("8080", nil, "test-api-key", &config.Config{}, slog.New(slog.DiscardHandler))
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	NewMCPToolsManager(srv).AddAllTools(mcpServer, "test-user")

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverSession, err := mcpServer.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := serverSession.Close(); closeErr != nil {
			t.Logf("serverSession.Close: %v", closeErr)
		}
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := clientSession.Close(); closeErr != nil {
			t.Logf("clientSession.Close: %v", closeErr)
		}
	})

	names := make(map[string]struct{})
	// Paginate: the server is constructed with nil options here, so the page
	// size is the SDK default. Iterating is what keeps this correct if the
	// registry outgrows one page.
	for tool, iterErr := range clientSession.Tools(ctx, nil) {
		require.NoError(t, iterErr)
		names[tool.Name] = struct{}{}
	}
	return names
}

// documentedMCPToolNames parses the curated catalog modules in the frontend
// tree. The backend module lives at backend/, so the repo root is three levels
// up from internal/server.
func documentedMCPToolNames(t *testing.T) map[string]struct{} {
	t.Helper()

	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)

	matches, err := filepath.Glob(filepath.Join(repoRoot, filepath.FromSlash(catalogGlob)))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "no catalog modules matched %s under %s", catalogGlob, repoRoot)

	sort.Strings(matches)
	names := make(map[string]struct{})
	for _, path := range matches {
		content, readErr := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- path comes from a constant glob inside the repo, not from user input.
		require.NoError(t, readErr)
		for _, match := range catalogNamePattern.FindAllStringSubmatch(string(content), -1) {
			names[match[1]] = struct{}{}
		}
	}
	return names
}
