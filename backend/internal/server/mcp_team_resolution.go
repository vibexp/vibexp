package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// resolveTeamPageSize is the page size used when paging through a user's teams
// to resolve a team identifier. It matches the maximum allowed by ListTeams.
const resolveTeamPageSize = 100

// teamAccessDeniedText is the generic, anti-enumeration message returned when a
// supplied team identifier does not match any team the user belongs to. It does
// NOT distinguish "team does not exist" from "you are not a member" so callers
// cannot probe for the existence of other teams.
const teamAccessDeniedText = "Access denied: the supplied team_id does not match any team you belong to. " +
	"Call vibexp_io_list_teams to list the teams you can use."

// teamRequiredText is returned when team_id is missing or blank.
const teamRequiredText = "team_id is required. It accepts a team UUID or slug. " +
	"Call vibexp_io_list_teams to discover the teams you belong to."

// mcpTextError builds an IsError CallToolResult carrying a single plain-text message.
func mcpTextError(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}
}

// resolveTeam resolves an untrusted per-call team identifier (a UUID or slug)
// to the canonical team UUID, validating membership in the same pass.
//
// It lists the teams the authenticated user belongs to and matches teamIdentifier
// against each team's ID (UUID) or Slug. Because only the user's own teams are
// considered, a successful match implicitly proves membership — there is no
// separate IsUserMemberOfTeam call to forget. On no match it returns a generic
// access-denied result that does not reveal whether the team exists.
//
// resolveTeam MUST be the first statement in every team-scoped MCP tool handler.
// On success it returns the canonical UUID and a nil errResult; on any failure
// it returns an empty teamID and a non-nil *mcp.CallToolResult the handler must
// return directly to the MCP layer.
func (s *Server) resolveTeam(
	ctx context.Context, userID, teamIdentifier string,
) (teamID string, errResult *mcp.CallToolResult) {
	identifier := strings.TrimSpace(teamIdentifier)
	if identifier == "" {
		return "", mcpTextError(teamRequiredText)
	}

	page := 1
	for {
		listResp, err := s.container.TeamService().ListTeams(ctx, userID, page, resolveTeamPageSize)
		if err != nil {
			slog.With(
				"user_id", userID,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to list teams while resolving team identifier for MCP tool")
			// Generic message: never leak whether the failure was lookup or membership.
			return "", mcpTextError(teamAccessDeniedText)
		}

		for i := range listResp.Teams {
			team := &listResp.Teams[i]
			if team.ID == identifier || team.Slug == identifier {
				return team.ID, nil
			}
		}

		if page*resolveTeamPageSize >= listResp.TotalCount || len(listResp.Teams) == 0 {
			break
		}
		page++
	}

	slog.With(
		"user_id", userID,
		"team_identifier", identifier,
	).Warn("MCP tool rejected: team_id did not match any team the user belongs to")
	return "", mcpTextError(teamAccessDeniedText)
}
