package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/models"
)

// Prompt Tool Parameters

// CreatePromptParams defines the parameters for creating a new prompt.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type CreatePromptParams struct {
	TeamID      string   `json:"team_id" jsonschema:"REQUIRED. The team UUID or slug to operate within. Call vibexp_io_list_teams first if you don't have one."`
	ProjectID   string   `json:"project_id" jsonschema:"Project UUID — required"`
	Name        string   `json:"name" jsonschema:"Human-readable prompt name (max 50 chars)"`
	Slug        string   `json:"slug" jsonschema:"Unique identifier for the prompt (max 255 chars)"`
	Body        string   `json:"body" jsonschema:"Full prompt body"`
	Description string   `json:"description,omitempty" jsonschema:"Brief description (max 200 chars)"`
	Status      string   `json:"status,omitempty" jsonschema:"One of \"draft\", \"published\""`
	MCPExpose   *bool    `json:"mcp_expose,omitempty" jsonschema:"Whether to expose this prompt as an MCP prompt primitive"`
	Labels      []string `json:"labels,omitempty" jsonschema:"Up to 10 labels (max 50 chars each)"`
}

// UpdatePromptParams defines the parameters for updating an existing prompt by slug.
// All editable fields are optional; only provided fields are changed.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type UpdatePromptParams struct {
	TeamID      string   `json:"team_id" jsonschema:"REQUIRED. The team UUID or slug to operate within. Call vibexp_io_list_teams first if you don't have one."`
	Slug        string   `json:"slug" jsonschema:"Slug identifier of the prompt to update"`
	Name        string   `json:"name,omitempty" jsonschema:"New name (max 50 chars)"`
	Body        string   `json:"body,omitempty" jsonschema:"New body"`
	Description string   `json:"description,omitempty" jsonschema:"New description (max 200 chars)"`
	Status      string   `json:"status,omitempty" jsonschema:"New status: one of \"draft\", \"published\""`
	ProjectID   string   `json:"project_id,omitempty" jsonschema:"New project UUID (move the prompt to another project)"`
	MCPExpose   *bool    `json:"mcp_expose,omitempty" jsonschema:"Whether to expose this prompt as an MCP prompt primitive"`
	Labels      []string `json:"labels,omitempty" jsonschema:"Up to 10 labels (max 50 chars each)"`
}

// promptWriteResponse is the slim response returned by create/update prompt tools.
type promptWriteResponse struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	FullURL string `json:"full_url"`
}

// buildPromptURL constructs the canonical web URL for a prompt. Prompts are
// routed by slug alone (frontend route /prompts/:slug), unlike artifacts which
// embed the project id in the path.
func buildPromptURL(baseURL, slug string) string {
	return fmt.Sprintf("%s/prompts/%s",
		strings.TrimRight(baseURL, "/"),
		url.PathEscape(slug),
	)
}

// Prompt Tool Implementations

// createPrompt implements the tool that creates a new prompt in the resolved team.
func (s *Server) createPrompt(
	ctx context.Context, _ *mcp.CallToolRequest, params *CreatePromptParams, userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	if r := validateProjectID(params.ProjectID); r != nil {
		slog.With(
			"tool", "vibexp_io_create_prompt",
			"user_id", userID,
			"team_id", teamID,
			"project_id", params.ProjectID,
		).Warn("MCP tool rejected: invalid project_id")
		return r, nil, nil
	}

	createReq := &models.CreatePromptRequest{
		ProjectID:   params.ProjectID,
		Name:        params.Name,
		Slug:        params.Slug,
		Body:        params.Body,
		Description: params.Description,
		Status:      params.Status,
		MCPExpose:   params.MCPExpose,
		Labels:      params.Labels,
	}

	prompt, err := s.container.PromptService().CreatePrompt(userID, teamID, createReq)
	if err != nil {
		slog.With(
			"user_id", userID,
			"team_id", teamID,
		).With("error", err).Error("Failed to create prompt via MCP")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to create prompt: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	if s.metrics != nil {
		s.metrics.RecordMCPCreatePrompt(context.Background())
	}

	return marshalPromptWriteResult(s.config.FrontendBaseURL, prompt)
}

// buildPromptUpdateRequest builds an UpdatePromptRequest from non-empty params fields.
func buildPromptUpdateRequest(params *UpdatePromptParams) *models.UpdatePromptRequest {
	updateReq := &models.UpdatePromptRequest{}
	if params.Name != "" {
		updateReq.Name = &params.Name
	}
	if params.Body != "" {
		updateReq.Body = &params.Body
	}
	if params.Description != "" {
		updateReq.Description = &params.Description
	}
	if params.Status != "" {
		updateReq.Status = &params.Status
	}
	if params.ProjectID != "" {
		updateReq.ProjectID = &params.ProjectID
	}
	if params.MCPExpose != nil {
		updateReq.MCPExpose = params.MCPExpose
	}
	if params.Labels != nil {
		updateReq.Labels = params.Labels
	}
	return updateReq
}

// updatePrompt implements the tool that updates a specific prompt (by slug) in the resolved team.
func (s *Server) updatePrompt(
	ctx context.Context, _ *mcp.CallToolRequest, params *UpdatePromptParams, userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	updateReq := buildPromptUpdateRequest(params)

	prompt, err := s.container.PromptService().UpdatePromptBySlug(userID, teamID, params.Slug, updateReq)
	if err != nil {
		slog.With(
			"tool", "vibexp_io_update_prompt",
			"user_id", userID,
			"team_id", teamID,
			"slug", params.Slug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to update prompt via MCP")

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to update prompt: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	if s.metrics != nil {
		s.metrics.RecordMCPUpdatePrompt(context.Background())
	}

	return marshalPromptWriteResult(s.config.FrontendBaseURL, prompt)
}

// marshalPromptWriteResult builds the slim create/update response and marshals it
// to JSON text plus StructuredContent, mirroring the artifact write tools.
func marshalPromptWriteResult(baseURL string, prompt *models.Prompt) (*mcp.CallToolResult, any, error) {
	result := &promptWriteResponse{
		ID:      prompt.ID,
		Slug:    prompt.Slug,
		FullURL: buildPromptURL(baseURL, prompt.Slug),
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal response to JSON: %w", err)
	}

	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(jsonData)}},
		StructuredContent: result,
	}, result, nil
}
