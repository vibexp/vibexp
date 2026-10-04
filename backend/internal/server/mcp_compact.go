package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// PoC, not for merge: the "compact" MCP surface served at /mcp/v1/compact.
//
// Instead of registering every tool on the session's server, it registers two:
// vibexp_io_search_tools returns the definitions of the tools a task needs, and
// vibexp_io_call_tool runs one. The full catalog lives on an inner server built
// by the same AddAllTools as /mcp/v1/common and reached over the SDK's
// in-memory transport, so a forwarded call goes through the SDK's own argument
// validation and the unchanged handler: same schemas, same errors, no second
// definition of any tool.
//
// Known PoC gap: the inner session is not closed when the outer one ends.

const (
	searchToolsToolName    = "vibexp_io_search_tools"
	callToolToolName       = "vibexp_io_call_tool"
	callReadToolToolName   = "vibexp_io_call_read_tool"
	callWriteToolToolName  = "vibexp_io_call_write_tool"
	callDeleteToolToolName = "vibexp_io_call_delete_tool"

	// compactToolPrefix is the prefix every catalog tool name carries; the
	// index lists names without it.
	compactToolPrefix = "vibexp_io_"

	// searchToolsMaxResults bounds how many full definitions one query returns.
	searchToolsMaxResults = 3
)

// compactInstructionsPreamble tells the agent that the tools the shared
// instructions name are reached through the catalog, not called directly.
const compactInstructionsPreamble = "This endpoint exposes VibeXP's tools through a catalog: every " +
	"vibexp_io_* tool named below is run through the call tool(s), after fetching its definition with " +
	searchToolsToolName + ". Use them exactly as you would the tools themselves: read the team's knowledge " +
	"before a task (vibexp_io_search) and write back what you learned when it ends (vibexp_io_create_memory, " +
	"vibexp_io_create_artifact, vibexp_io_post_to_feed) without waiting to be asked. "

// compactLoopGuidance is the read-then-write-back nudge carried by the search
// tool's own description, because a client is not required to show server
// instructions to the model.
const compactLoopGuidance = " Search the team's knowledge before a task, and when it ends save what you " +
	"learned (create_memory, create_artifact, post_to_feed) without being asked."

// newCompactMCPServer builds the outer server of a compact endpoint.
func newCompactMCPServer() *mcp.Server {
	return newMCPServerWithInstructions(compactInstructionsPreamble + mcpServerInstructions)
}

// compactRiskClass is the permission class a catalog tool belongs to.
type compactRiskClass string

const (
	compactRiskRead   compactRiskClass = "read"
	compactRiskWrite  compactRiskClass = "write"
	compactRiskDelete compactRiskClass = "delete"
)

// compactRiskOf classifies a tool by its verb. A tool with an unrecognized verb
// is a write, so a new tool is never reachable through the read-only executor
// by accident.
func compactRiskOf(name string) compactRiskClass {
	verb := strings.TrimPrefix(name, compactToolPrefix)
	switch {
	case strings.HasPrefix(verb, "delete_"):
		return compactRiskDelete
	case strings.HasPrefix(verb, "get_"), strings.HasPrefix(verb, "list_"),
		strings.HasPrefix(verb, "render_"), verb == "search":
		return compactRiskRead
	default:
		return compactRiskWrite
	}
}

// compactPurposes is the order the name index lists its groups in.
var compactPurposes = []string{"discovery", "read", "write", "feed", "delete"}

// compactPurposeOf groups a tool by what an agent uses it for.
func compactPurposeOf(name string) string {
	verb := strings.TrimPrefix(name, compactToolPrefix)
	switch {
	case strings.Contains(verb, "feed"):
		return "feed"
	case verb == "get_user", strings.HasPrefix(verb, "list_teams"), verb == "list_projects":
		return "discovery"
	}
	return string(compactRiskOf(name))
}

// compactNameIndex lists every catalog tool by short name, grouped by purpose.
func compactNameIndex(catalog []*mcp.Tool) string {
	byPurpose := map[string][]string{}
	for _, tool := range catalog {
		purpose := compactPurposeOf(tool.Name)
		byPurpose[purpose] = append(byPurpose[purpose], strings.TrimPrefix(tool.Name, compactToolPrefix))
	}
	groups := make([]string, 0, len(compactPurposes))
	for _, purpose := range compactPurposes {
		if len(byPurpose[purpose]) > 0 {
			groups = append(groups, purpose+": "+strings.Join(byPurpose[purpose], ", "))
		}
	}
	return strings.Join(groups, "; ")
}

// SearchToolsParams defines the parameters for vibexp_io_search_tools.
type SearchToolsParams struct {
	Query string   `json:"query,omitempty" jsonschema:"What you want to do, e.g. \"create memory\"."`
	Names []string `json:"names,omitempty" jsonschema:"Exact tool names. Takes precedence over query."`
}

// CallToolParams defines the parameters for vibexp_io_call_tool.
type CallToolParams struct {
	Name      string         `json:"name" jsonschema:"REQUIRED. A tool name from the catalog."`
	Arguments map[string]any `json:"arguments,omitempty" jsonschema:"Arguments matching the tool's input_schema."`
}

// compactToolDefinition is one catalog entry as vibexp_io_search_tools returns it.
type compactToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

type searchToolsResponse struct {
	Tools []compactToolDefinition `json:"tools"`
}

// setupMCPServerCompact registers the two compact tools on mcpServer, backed by
// an inner server holding the full catalog for the authenticated user.
func (s *Server) setupMCPServerCompact(mcpServer *mcp.Server, toolsManager *MCPToolsManager, req *http.Request) {
	userID, ok := getUserFromContext(req)
	if !ok {
		slog.Warn("Missing user ID in MCP handler despite auth middleware")
		return
	}

	inner, catalog, err := connectInnerCatalog(toolsManager, userID)
	if err != nil {
		slog.With("user_id", userID, "error", err).Error("Failed to build the compact MCP catalog")
		return
	}

	addCompactTools(mcpServer, inner, catalog)
	s.addUserPromptsToMCP(req.Context(), mcpServer, userID)
}

// setupMCPServerCompactSplit is setupMCPServerCompact with one executor per
// risk class instead of a single vibexp_io_call_tool.
func (s *Server) setupMCPServerCompactSplit(mcpServer *mcp.Server, toolsManager *MCPToolsManager, req *http.Request) {
	userID, ok := getUserFromContext(req)
	if !ok {
		slog.Warn("Missing user ID in MCP handler despite auth middleware")
		return
	}

	inner, catalog, err := connectInnerCatalog(toolsManager, userID)
	if err != nil {
		slog.With("user_id", userID, "error", err).Error("Failed to build the compact MCP catalog")
		return
	}

	addCompactSplitTools(mcpServer, inner, catalog)
	s.addUserPromptsToMCP(req.Context(), mcpServer, userID)
}

// addCompactSplitTools registers vibexp_io_search_tools and the three
// class-scoped executors, each annotated so a client can gate them separately.
func addCompactSplitTools(mcpServer *mcp.Server, inner *mcp.ClientSession, catalog []*mcp.Tool) {
	byClass := map[compactRiskClass][]string{}
	for _, tool := range catalog {
		class := compactRiskOf(tool.Name)
		byClass[class] = append(byClass[class], strings.TrimPrefix(tool.Name, compactToolPrefix))
	}
	executors := "vibexp_io_call_read_tool (" + strings.Join(byClass[compactRiskRead], ", ") + "), " +
		"vibexp_io_call_write_tool (" + strings.Join(byClass[compactRiskWrite], ", ") + ") or " +
		"vibexp_io_call_delete_tool (" + strings.Join(byClass[compactRiskDelete], ", ") + ")"

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name: searchToolsToolName,
		Description: "Get the definitions (description and input_schema) of VibeXP tools before calling them. " +
			"Pass `names` for exact tools or `query` to find tools by what they do. Tool names are vibexp_io_<name>; " +
			"run each with " + executors + "." + compactLoopGuidance,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, params *SearchToolsParams) (*mcp.CallToolResult, any, error) {
		return searchCompactCatalog(catalog, params)
	})

	destructive, additive := true, false
	split := []struct {
		name        string
		class       compactRiskClass
		summary     string
		annotations *mcp.ToolAnnotations
	}{
		{callReadToolToolName, compactRiskRead, "Run a read-only VibeXP tool (get, list, search, render).",
			&mcp.ToolAnnotations{ReadOnlyHint: true}},
		{callWriteToolToolName, compactRiskWrite, "Run a VibeXP tool that creates or updates data.",
			&mcp.ToolAnnotations{DestructiveHint: &additive}},
		{callDeleteToolToolName, compactRiskDelete, "Run a VibeXP tool that deletes data.",
			&mcp.ToolAnnotations{DestructiveHint: &destructive}},
	}
	for _, executor := range split {
		mcp.AddTool(mcpServer, &mcp.Tool{
			Name: executor.name,
			Description: executor.summary + " Fetch its input_schema with " + searchToolsToolName +
				" first; `arguments` is validated against it and errors name the offending field.",
			Annotations: executor.annotations,
		}, func(ctx context.Context, _ *mcp.CallToolRequest, params *CallToolParams) (*mcp.CallToolResult, any, error) {
			if tool := findCompactTool(catalog, params.Name); tool != nil && compactRiskOf(tool.Name) != executor.class {
				return mcpTextError(fmt.Sprintf("%s is a %s tool; call it with vibexp_io_call_%s_tool",
					params.Name, compactRiskOf(tool.Name), compactRiskOf(tool.Name))), nil, nil
			}
			return callCompactTool(ctx, inner, catalog, params)
		})
	}
}

// connectInnerCatalog builds the full-catalog server for userID, connects a
// client to it in memory and returns that client with the tool list.
func connectInnerCatalog(toolsManager *MCPToolsManager, userID string) (*mcp.ClientSession, []*mcp.Tool, error) {
	// The inner session outlives the request that creates it.
	ctx := context.Background()

	innerServer := newMCPServer()
	toolsManager.AddAllTools(innerServer, userID)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := innerServer.Connect(ctx, serverTransport, nil); err != nil {
		return nil, nil, fmt.Errorf("connect inner server: %w", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "vibexp-mcp-compact", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("connect inner client: %w", err)
	}

	var catalog []*mcp.Tool
	for tool, iterErr := range session.Tools(ctx, nil) {
		if iterErr != nil {
			return nil, nil, fmt.Errorf("list inner tools: %w", iterErr)
		}
		catalog = append(catalog, tool)
	}
	sort.Slice(catalog, func(i, j int) bool { return catalog[i].Name < catalog[j].Name })

	return session, catalog, nil
}

// addCompactTools registers vibexp_io_search_tools and vibexp_io_call_tool.
func addCompactTools(mcpServer *mcp.Server, inner *mcp.ClientSession, catalog []*mcp.Tool) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name: searchToolsToolName,
		Description: "Get the definitions (description and input_schema) of VibeXP tools before calling them with " +
			callToolToolName + ". Pass `names` for exact tools or `query` to find tools by what they do. " +
			"Tool names are " + compactToolPrefix + "<name>. Available, by purpose: " + compactNameIndex(catalog) + "." +
			compactLoopGuidance,
	}, func(_ context.Context, _ *mcp.CallToolRequest, params *SearchToolsParams) (*mcp.CallToolResult, any, error) {
		return searchCompactCatalog(catalog, params)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name: callToolToolName,
		Description: "Call one VibeXP tool by name. Fetch its input_schema with " + searchToolsToolName +
			" first; `arguments` is validated against that schema and errors name the offending field.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, params *CallToolParams) (*mcp.CallToolResult, any, error) {
		return callCompactTool(ctx, inner, catalog, params)
	})
}

// searchCompactCatalog returns the definitions selected by names, or else the
// best matches for query.
func searchCompactCatalog(catalog []*mcp.Tool, params *SearchToolsParams) (*mcp.CallToolResult, any, error) {
	var selected []*mcp.Tool
	switch {
	case len(params.Names) > 0:
		var unknown []string
		for _, name := range params.Names {
			if tool := findCompactTool(catalog, name); tool != nil {
				selected = append(selected, tool)
			} else {
				unknown = append(unknown, name)
			}
		}
		if len(unknown) > 0 {
			return mcpTextError(fmt.Sprintf("unknown tool name(s): %s. Valid names: %s",
				strings.Join(unknown, ", "), strings.Join(compactToolNames(catalog), ", "))), nil, nil
		}
	case strings.TrimSpace(params.Query) != "":
		selected = rankCompactTools(catalog, params.Query)
	default:
		return mcpTextError("pass `names` (exact tool names) or `query` (what you want to do)"), nil, nil
	}

	response := searchToolsResponse{Tools: make([]compactToolDefinition, 0, len(selected))}
	for _, tool := range selected {
		response.Tools = append(response.Tools, compactToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	// Compact JSON: a definition is read by the model, not a person, and it
	// stays in context for the rest of the session.
	jsonData, err := json.Marshal(response)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal tool definitions: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(jsonData)}},
	}, nil, nil
}

// callCompactTool forwards one call to the inner catalog server and returns its
// result unchanged, including IsError results from validation or the handler.
func callCompactTool(
	ctx context.Context, inner *mcp.ClientSession, catalog []*mcp.Tool, params *CallToolParams,
) (*mcp.CallToolResult, any, error) {
	if findCompactTool(catalog, params.Name) == nil {
		return mcpTextError(fmt.Sprintf("unknown tool %q. Valid names: %s",
			params.Name, strings.Join(compactToolNames(catalog), ", "))), nil, nil
	}

	arguments := params.Arguments
	if arguments == nil {
		arguments = map[string]any{}
	}

	result, err := inner.CallTool(ctx, &mcp.CallToolParams{Name: params.Name, Arguments: arguments})
	if err != nil {
		return mcpTextError(fmt.Sprintf("%s: %v", params.Name, err)), nil, nil
	}
	return result, nil, nil
}

// findCompactTool finds a catalog tool by its full name or by the short name
// the index lists (without the vibexp_io_ prefix).
func findCompactTool(catalog []*mcp.Tool, name string) *mcp.Tool {
	if !strings.HasPrefix(name, compactToolPrefix) {
		name = compactToolPrefix + name
	}
	for _, tool := range catalog {
		if tool.Name == name {
			return tool
		}
	}
	return nil
}

func compactToolNames(catalog []*mcp.Tool) []string {
	names := make([]string, 0, len(catalog))
	for _, tool := range catalog {
		names = append(names, tool.Name)
	}
	return names
}

// rankCompactTools scores each tool by how many query words appear in its name
// (weighted) and description, and returns the best searchToolsMaxResults.
func rankCompactTools(catalog []*mcp.Tool, query string) []*mcp.Tool {
	words := strings.Fields(strings.ToLower(query))

	type scored struct {
		tool  *mcp.Tool
		score int
	}
	var matches []scored
	for _, tool := range catalog {
		name := strings.ToLower(tool.Name)
		description := strings.ToLower(tool.Description)
		score := 0
		for _, word := range words {
			if strings.Contains(name, word) {
				score += 3
			}
			if strings.Contains(description, word) {
				score++
			}
		}
		if score > 0 {
			matches = append(matches, scored{tool, score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].score > matches[j].score })

	if len(matches) > searchToolsMaxResults {
		matches = matches[:searchToolsMaxResults]
	}
	selected := make([]*mcp.Tool, 0, len(matches))
	for _, match := range matches {
		selected = append(selected, match.tool)
	}
	return selected
}

// compactCoreTools are the read/write-back loop tools the core variant keeps
// typed and always on; every other tool is reached through the catalog pair.
var compactCoreTools = map[string]bool{
	"vibexp_io_list_teams_and_projects": true,
	"vibexp_io_search":                  true,
	"vibexp_io_get_resource":            true,
	"vibexp_io_create_memory":           true,
	"vibexp_io_update_memory":           true,
	"vibexp_io_create_artifact":         true,
	"vibexp_io_post_to_feed":            true,
}

// setupMCPServerCompactCore registers the loop tools directly, with their
// unchanged definitions, and the catalog pair for the rest.
func (s *Server) setupMCPServerCompactCore(mcpServer *mcp.Server, toolsManager *MCPToolsManager, req *http.Request) {
	userID, ok := getUserFromContext(req)
	if !ok {
		slog.Warn("Missing user ID in MCP handler despite auth middleware")
		return
	}

	inner, catalog, err := connectInnerCatalog(toolsManager, userID)
	if err != nil {
		slog.With("user_id", userID, "error", err).Error("Failed to build the compact MCP catalog")
		return
	}

	addCompactCoreTools(mcpServer, inner, catalog)
	s.addUserPromptsToMCP(req.Context(), mcpServer, userID)
}

// addCompactCoreTools registers each core tool as a pass-through to the inner
// server (which validates the arguments), then the pair over the remainder.
func addCompactCoreTools(mcpServer *mcp.Server, inner *mcp.ClientSession, catalog []*mcp.Tool) {
	var rest []*mcp.Tool
	for _, tool := range catalog {
		if !compactCoreTools[tool.Name] {
			rest = append(rest, tool)
			continue
		}
		mcpServer.AddTool(&mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
			Annotations: tool.Annotations,
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			arguments := map[string]any{}
			if len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, &arguments); err != nil {
					return mcpTextError(fmt.Sprintf("%s: arguments must be a JSON object: %v", tool.Name, err)), nil
				}
			}
			result, err := inner.CallTool(ctx, &mcp.CallToolParams{Name: tool.Name, Arguments: arguments})
			if err != nil {
				return mcpTextError(fmt.Sprintf("%s: %v", tool.Name, err)), nil
			}
			return result, nil
		})
	}
	addCompactTools(mcpServer, inner, rest)
}
