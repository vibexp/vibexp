package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// promptsListPageSize is the page size used when paging through a user's teams
// to register their prompts. It matches the maximum allowed by ListTeams.
const promptsListPageSize = 100

// extractPromptArguments extracts placeholder arguments from prompt body
func (s *Server) extractPromptArguments(body string, userID string) []*mcp.PromptArgument {
	// Use the same logic as the prompt service to extract placeholders
	// Note: ExtractAllPlaceholders doesn't require teamID as it operates on prompt body content
	placeholders, err := s.container.PromptService().ExtractAllPlaceholders(userID, body, make(map[string]bool))
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to extract placeholders for MCP prompt arguments")
		return nil
	}

	// Pre-allocate slice with known capacity
	arguments := make([]*mcp.PromptArgument, 0, len(placeholders))

	// Convert placeholders to MCP arguments
	caser := cases.Title(language.English)
	for _, placeholder := range placeholders {
		arg := &mcp.PromptArgument{
			Name:        placeholder,
			Title:       caser.String(strings.ReplaceAll(placeholder, "_", " ")),
			Description: fmt.Sprintf("Value for placeholder: %s", placeholder),
			Required:    true, // All placeholders are required
		}
		arguments = append(arguments, arg)
	}

	return arguments
}

// addUserPromptsToMCP registers the authenticated user's published, MCP-exposed
// prompts across all teams the user belongs to. Each prompt's render closure
// captures its own team_id, so prompt rendering operates on a membership-validated
// team without any per-call team parameter. Prompt names are disambiguated across
// teams to avoid slug collisions (a slug is unique within a team, not across teams).
func (s *Server) addUserPromptsToMCP(ctx context.Context, mcpServer *mcp.Server, userID string) {
	registered := make(map[string]struct{})
	total := 0

	page := 1
	for {
		listResp, err := s.container.TeamService().ListTeams(ctx, userID, page, promptsListPageSize)
		if err != nil {
			s.logger.With(
				"user_id", userID,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to list teams for MCP prompts")
			return
		}

		for i := range listResp.Teams {
			team := &listResp.Teams[i]
			total += s.addTeamPromptsToMCP(mcpServer, userID, team.ID, team.Slug, registered)
		}

		if page*promptsListPageSize >= listResp.TotalCount || len(listResp.Teams) == 0 {
			break
		}
		page++
	}

	s.logger.With(
		"user_id", userID,
		"prompt_count", total,
	).Info("Added user prompts to MCP server across teams")
}

// addTeamPromptsToMCP registers one team's published, MCP-exposed prompts and
// returns the number registered. The registered set tracks names already taken
// so cross-team slug collisions are disambiguated with the team slug.
func (s *Server) addTeamPromptsToMCP(
	mcpServer *mcp.Server, userID, teamID, teamSlug string, registered map[string]struct{},
) int {
	mcpExposeTrue := true
	filters := services.PromptFilters{
		Status:    "published",
		MCPExpose: &mcpExposeTrue,
		UserID:    userID,
		TeamID:    teamID,
		Page:      1,
		Limit:     10000,
	}

	promptResponse, err := s.container.PromptService().ListPrompts(userID, filters)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to load team prompts for MCP")
		return 0
	}

	count := 0
	for _, prompt := range promptResponse.Prompts {
		name := uniquePromptName(prompt.Slug, teamSlug, registered)
		registered[name] = struct{}{}

		arguments := s.extractPromptArguments(prompt.Body, userID)
		mcpPrompt := &mcp.Prompt{
			Name:        name,
			Title:       name,
			Description: prompt.Description,
			Arguments:   arguments,
		}

		// Capture per-prompt data and the team this prompt belongs to.
		promptData := prompt
		promptTeamID := teamID
		mcpServer.AddPrompt(mcpPrompt, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return s.handleMCPPromptRequestWithTeam(ctx, req, promptData, userID, promptTeamID)
		})
		count++
	}

	return count
}

// uniquePromptName returns slug if it is not already registered, otherwise a
// slug disambiguated by the team slug so prompts with the same slug in different
// teams do not collide. If the team-disambiguated name is itself already taken
// (a 3-way or adversarial collision), a numeric suffix is appended until the
// candidate is free, so every registered prompt name is guaranteed unique and
// no prompt silently overwrites another in the SDK's prompt map.
func uniquePromptName(slug, teamSlug string, registered map[string]struct{}) string {
	if _, taken := registered[slug]; !taken {
		return slug
	}

	candidate := fmt.Sprintf("%s__%s", slug, teamSlug)
	for i := 2; ; i++ {
		if _, taken := registered[candidate]; !taken {
			return candidate
		}
		candidate = fmt.Sprintf("%s__%s__%d", slug, teamSlug, i)
	}
}

// handleMCPPromptRequestWithTeam handles MCP prompt requests with explicit team ID
func (s *Server) handleMCPPromptRequestWithTeam(
	_ context.Context,
	req *mcp.GetPromptRequest,
	promptData models.Prompt,
	userID, teamID string,
) (*mcp.GetPromptResult, error) {
	// Convert MCP arguments to placeholders map
	placeholders := make(map[string]string)
	if req.Params.Arguments != nil {
		for key, value := range req.Params.Arguments {
			placeholders[key] = value
		}
	}

	// Render the prompt using the prompt service
	renderedResponse, err := s.container.PromptService().RenderPrompt(userID, teamID, promptData.Slug, placeholders)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"team_id", teamID,
			"prompt_slug", promptData.Slug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to render prompt for MCP request")
		return nil, fmt.Errorf("failed to render prompt: %w", err)
	}

	// Create the MCP response
	result := &mcp.GetPromptResult{
		Description: promptData.Description,
		Messages: []*mcp.PromptMessage{
			{
				Role: "user",
				Content: &mcp.TextContent{
					Text: renderedResponse.RenderedBody,
				},
			},
		},
	}

	return result, nil
}
