package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/repositories"
)

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
// It delegates to TeamRepository.ResolveByIdentifier, which matches the
// identifier against the team's ID (UUID) or slug while enforcing owner-OR-member
// access in the SQL. Because the query only considers the user's own teams, a
// successful match implicitly proves membership — there is no separate
// IsUserMemberOfTeam call to forget. On no match it returns a generic
// access-denied result that does not reveal whether the team exists.
//
// The lookup is a single query regardless of how many teams the user belongs to:
// this runs before every team-scoped MCP tool, so the previous implementation's
// page-through-every-team cost was paid on every call (#812).
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

	team, err := s.container.TeamRepository().ResolveByIdentifier(ctx, userID, identifier)
	if err != nil {
		// Both arms return the SAME generic message: a not-found and an
		// infrastructure failure must stay indistinguishable to the caller, or
		// the anti-enumeration property is lost.
		if errors.Is(err, repositories.ErrTeamNotFound) {
			slog.With(
				"user_id", userID,
				"team_identifier", identifier,
			).Warn("MCP tool rejected: team_id did not match any team the user belongs to")
		} else {
			slog.With(
				"user_id", userID,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to resolve team identifier for MCP tool")
		}
		return "", mcpTextError(teamAccessDeniedText)
	}

	return team.ID, nil
}
