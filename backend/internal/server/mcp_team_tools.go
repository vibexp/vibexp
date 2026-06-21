package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
)

// listTeamsPageSize is the page size used when listing the user's teams for the
// vibexp_io_list_teams tool. It matches the maximum allowed by ListTeams.
const listTeamsPageSize = 100

// ListTeamsParams defines the parameters for the list teams tool. This tool
// takes no input parameters; teams are scoped to the authenticated user.
type ListTeamsParams struct{}

// listTeamsToolDescription is the agent-facing description of vibexp_io_list_teams.
const listTeamsToolDescription = "List the teams the authenticated user belongs to. " +
	"Call this FIRST to discover a team_id (UUID or slug) before using any other " +
	"team-scoped tool (artifacts, memories, projects, feeds, search). " +
	"Returns one entry per team with its uuid, name, and slug; pass either the uuid " +
	"or the slug as the team_id parameter of other tools."

// teamSummary is the per-team shape returned by vibexp_io_list_teams.
type teamSummary struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// listTeamsResponse is the response shape returned by vibexp_io_list_teams.
type listTeamsResponse struct {
	Teams []teamSummary `json:"teams"`
}

// listTeamsForUser implements the vibexp_io_list_teams MCP tool. It returns the
// teams the authenticated user belongs to so the model can obtain a team_id for
// the team-scoped tools. It is user-scoped and intentionally takes no team_id.
func (s *Server) listTeamsForUser(
	ctx context.Context, _ *mcp.CallToolRequest, _ *ListTeamsParams, userID string,
) (*mcp.CallToolResult, any, error) {
	summaries := make([]teamSummary, 0)

	page := 1
	for {
		listResp, err := s.container.TeamService().ListTeams(ctx, userID, page, listTeamsPageSize)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"tool":    "vibexp_io_list_teams",
				"user_id": userID,
				"error":   fmt.Sprintf("%+v", err),
			}).Error("Failed to list teams via MCP")
			return mcpTextError("Failed to list teams. Please try again later."), nil, nil
		}

		for i := range listResp.Teams {
			team := &listResp.Teams[i]
			summaries = append(summaries, teamSummary{
				UUID: team.ID,
				Name: team.Name,
				Slug: team.Slug,
			})
		}

		if page*listTeamsPageSize >= listResp.TotalCount || len(listResp.Teams) == 0 {
			break
		}
		page++
	}

	result := &listTeamsResponse{Teams: summaries}
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal response to JSON: %w", err)
	}

	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(jsonData)}},
		StructuredContent: result,
	}, result, nil
}
