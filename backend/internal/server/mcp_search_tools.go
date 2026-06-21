package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/models"
)

// Pagination bounds for semantic search, mirroring validatePaginationParams in
// handlers_helpers.go so the MCP tool behaves identically to the HTTP endpoint.
const (
	searchDefaultPage  = 1
	searchMaxPage      = 10000
	searchDefaultLimit = 10
	searchMaxLimit     = 100
)

// SemanticSearchParams defines the arguments for the vibexp_io_search MCP tool.
// They mirror the HTTP search payload (POST /api/v1/{team_id}/search), with
// team_id supplied per call (UUID or slug) and membership-validated.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type SemanticSearchParams struct {
	TeamID    string   `json:"team_id" jsonschema:"REQUIRED. The team UUID or slug to operate within. Call vibexp_io_list_teams first if you don't have one."`
	Query     string   `json:"query" jsonschema:"The natural-language search query. Required, max 1000 chars."`
	Types     []string `json:"types,omitempty" jsonschema:"Subset of prompts,artifacts,blueprints,memories; omit for all."`
	ProjectID string   `json:"project_id,omitempty" jsonschema:"Optional project UUID; restrict results to one project."`
	Page      int      `json:"page,omitempty" jsonschema:"1-based page number (default 1, max 10000)."`
	Limit     int      `json:"limit,omitempty" jsonschema:"Results per page (default 10, max 100)."`
}

// semanticSearchToolDescription is the agent-facing description of vibexp_io_search.
const semanticSearchToolDescription = "Semantic (RAG) retrieval across the current team's prompts, artifacts, " +
	"blueprints, and memories. Use this when you need to *find* relevant team knowledge by meaning — e.g. " +
	"\"how did we configure the staging database?\" or \"prior decisions about pricing\" — rather than by an exact " +
	"id, slug, or filter. Prefer vibexp_io_search_artifacts when you already know the project and want an exact, " +
	"filterable artifact listing; prefer this tool for open-ended, cross-entity discovery. " +
	"Pass a single natural-language query; optionally narrow with types (plural: prompts, artifacts, blueprints, " +
	"memories) and/or project_id (a project UUID), and paginate with page/limit. Omitting types searches all four " +
	"entity types; omitting project_id searches across all projects. " +
	"Returns relevance-ranked results: each has type (singular: prompt, artifact, blueprint, memory), id, title, " +
	"slug, project_id, project_name, a short excerpt, a score in [0,1] (higher is more relevant), chunk_id, and " +
	"updated_at, plus pagination metadata (total_count, page, per_page, total_pages). Results are always scoped " +
	"to the authenticated team."

// normalizeSearchPagination applies default and max bounds to page/limit, matching
// validatePaginationParams so page/limit of 0 never yield a negative offset.
func normalizeSearchPagination(page, limit int) (int, int) {
	if page < searchDefaultPage || page > searchMaxPage {
		page = searchDefaultPage
	}
	if limit < 1 || limit > searchMaxLimit {
		limit = searchDefaultLimit
	}
	return page, limit
}

// addSearchTools registers the semantic search MCP tool.
func (m *MCPToolsManager) addSearchTools(mcpServer *mcp.Server, userID string) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_search",
		Description: semanticSearchToolDescription,
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *SemanticSearchParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.search(ctx, req, params, userID)
	})
}

// searchToolTextError builds an error CallToolResult carrying a plain-text message.
func searchToolTextError(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}
}

// search implements the vibexp_io_search tool: it resolves the per-call team,
// then wraps the team-scoped SearchService.Search, reusing the exact response
// shape of the HTTP search endpoint.
func (s *Server) search(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *SemanticSearchParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	query := strings.TrimSpace(params.Query)
	if query == "" {
		return searchToolTextError("query is required and must not be empty"), nil, nil
	}

	page, limit := normalizeSearchPagination(params.Page, params.Limit)
	searchReq := &models.SearchRequest{
		Query:     query,
		Types:     params.Types,
		ProjectID: strings.TrimSpace(params.ProjectID),
		Page:      page,
		PerPage:   limit,
	}

	// Mirror handleSearch's validation before reaching the billed embeddings service.
	if err := validate.Struct(searchReq); err != nil {
		return searchToolTextError(fmt.Sprintf("invalid search parameters: %v", err)), nil, nil
	}

	response, err := s.container.SearchService().Search(ctx, teamID, searchReq)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"tool":    "vibexp_io_search",
			"user_id": userID,
			"team_id": teamID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to perform semantic search via MCP")

		return searchToolTextError("Failed to perform search. Please try again later."), nil, nil
	}

	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal response to JSON: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
		StructuredContent: response,
	}, response, nil
}
