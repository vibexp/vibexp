package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/services"
)

// ListProjectsParams defines the parameters for listing projects
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type ListProjectsParams struct {
	TeamID    string `json:"team_id"              jsonschema:"REQUIRED. The team UUID or slug to operate within. Call vibexp_io_list_teams first if you don't have one."`
	Search    string `json:"search,omitempty"     jsonschema:"Search in project name/description"`
	SortBy    string `json:"sort_by,omitempty"    jsonschema:"Field to sort by"`
	SortOrder string `json:"sort_order,omitempty" jsonschema:"Sort direction: asc or desc"`
	Page      int    `json:"page,omitempty"       jsonschema:"Page number (default: 1)"`
	Limit     int    `json:"limit,omitempty"      jsonschema:"Items per page (default: 10, max: 10)"`
}

// projectFiltersFromParams builds a ProjectFilters from MCP params applying defaults and caps
func projectFiltersFromParams(params *ListProjectsParams) services.ProjectFilters {
	page := params.Page
	if page <= 0 {
		page = 1
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 10
	}

	if limit > 10 {
		limit = 10
	}

	return services.ProjectFilters{
		Search:    params.Search,
		SortBy:    params.SortBy,
		SortOrder: params.SortOrder,
		Page:      page,
		Limit:     limit,
	}
}

// listProjects implements the tool that lists projects for the resolved team.
func (s *Server) listProjects(
	ctx context.Context, _ *mcp.CallToolRequest, params *ListProjectsParams, userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	filters := projectFiltersFromParams(params)
	filters.TeamID = teamID

	response, err := s.container.ProjectService().ListProjects(userID, filters)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"tool":    "vibexp_io_list_projects",
			"user_id": userID,
			"team_id": teamID,
			"error":   fmt.Sprintf("%+v", err),
		}).Error("Failed to list projects via MCP")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to list projects: %v", err)},
			},
			IsError: true,
		}, nil, nil
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
