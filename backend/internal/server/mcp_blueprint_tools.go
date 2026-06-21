package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/models"
)

// Blueprint Tool Parameters

// CreateBlueprintParams defines the parameters for creating a new blueprint.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type CreateBlueprintParams struct {
	TeamID      string                 `json:"team_id" jsonschema:"REQUIRED. The team UUID or slug to operate within. Call vibexp_io_list_teams first if you don't have one."`
	ProjectID   string                 `json:"project_id" jsonschema:"Project UUID — required"`
	Slug        string                 `json:"slug" jsonschema:"Unique identifier for the blueprint within the project (max 255 chars)"`
	Title       string                 `json:"title" jsonschema:"Human-readable blueprint title (max 255 chars)"`
	Content     string                 `json:"content" jsonschema:"Full blueprint content"`
	Description string                 `json:"description,omitempty" jsonschema:"Brief description (max 500 chars)"`
	Type        string                 `json:"type,omitempty" jsonschema:"Blueprint type (defaults to \"general\")"`
	Subtype     *string                `json:"subtype,omitempty" jsonschema:"Optional subtype (e.g. \"sub-agents\"); when \"sub-agents\", metadata.model is required"`
	Status      string                 `json:"status,omitempty" jsonschema:"One of \"active\", \"expired\""`
	Metadata    map[string]interface{} `json:"metadata,omitempty" jsonschema:"Optional custom metadata (e.g. {\"model\": \"...\"} for sub-agents)"`
}

// UpdateBlueprintParams defines the parameters for updating an existing blueprint.
// The blueprint is located by project_id + slug; all editable fields are optional
// and only provided fields are changed.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type UpdateBlueprintParams struct {
	TeamID      string                 `json:"team_id" jsonschema:"REQUIRED. The team UUID or slug to operate within. Call vibexp_io_list_teams first if you don't have one."`
	ProjectID   string                 `json:"project_id" jsonschema:"Project UUID the blueprint belongs to — required to locate the blueprint"`
	Slug        string                 `json:"slug" jsonschema:"Slug identifier of the blueprint to update"`
	Title       string                 `json:"title,omitempty" jsonschema:"New title (max 255 chars)"`
	Content     string                 `json:"content,omitempty" jsonschema:"New content"`
	Description string                 `json:"description,omitempty" jsonschema:"New description (max 500 chars)"`
	Type        string                 `json:"type,omitempty" jsonschema:"New blueprint type"`
	Subtype     *string                `json:"subtype,omitempty" jsonschema:"New subtype (e.g. \"sub-agents\")"`
	Status      string                 `json:"status,omitempty" jsonschema:"New status: one of \"active\", \"expired\""`
	Metadata    map[string]interface{} `json:"metadata,omitempty" jsonschema:"New custom metadata (replaces the existing metadata when provided)"`
}

// blueprintWriteResponse is the slim response returned by create/update blueprint tools.
type blueprintWriteResponse struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	FullURL string `json:"full_url"`
}

// buildBlueprintURL constructs the canonical web URL for a blueprint. Blueprints
// embed the project id in the path (frontend route /blueprints/:project/:slug),
// mirroring the artifact URL shape.
func buildBlueprintURL(baseURL, projectID, slug string) string {
	return fmt.Sprintf("%s/blueprints/%s/%s",
		strings.TrimRight(baseURL, "/"),
		url.PathEscape(projectID),
		url.PathEscape(slug),
	)
}

// Blueprint Tool Implementations

// createBlueprint implements the tool that creates a new blueprint in the resolved team.
func (s *Server) createBlueprint(
	ctx context.Context, _ *mcp.CallToolRequest, params *CreateBlueprintParams, userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	if r := validateProjectID(params.ProjectID); r != nil {
		logrus.WithFields(logrus.Fields{
			"tool":       "vibexp_io_create_blueprint",
			"user_id":    userID,
			"team_id":    teamID,
			"project_id": params.ProjectID,
		}).Warn("MCP tool rejected: invalid project_id")
		return r, nil, nil
	}

	createReq := &models.CreateBlueprintRequest{
		ProjectID:   params.ProjectID,
		Slug:        params.Slug,
		Content:     params.Content,
		Title:       params.Title,
		Description: params.Description,
		Type:        params.Type,
		Subtype:     params.Subtype,
		Status:      params.Status,
		Metadata:    params.Metadata,
	}

	blueprint, err := s.container.BlueprintService().CreateBlueprint(userID, teamID, createReq)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"user_id": userID,
			"team_id": teamID,
		}).WithError(err).Error("Failed to create blueprint via MCP")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to create blueprint: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	if s.metrics != nil {
		s.metrics.RecordMCPCreateBlueprint(context.Background())
	}

	return marshalBlueprintWriteResult(s.config.FrontendBaseURL, blueprint)
}

// buildBlueprintUpdateRequest builds an UpdateBlueprintRequest from non-empty params fields.
// Empty string fields are left as nil pointers, so a field cannot be cleared to "" via update
// (consistent with the other MCP write tools). project_id/slug are locators, not editable here.
func buildBlueprintUpdateRequest(params *UpdateBlueprintParams) *models.UpdateBlueprintRequest {
	updateReq := &models.UpdateBlueprintRequest{}
	if params.Title != "" {
		updateReq.Title = &params.Title
	}
	if params.Content != "" {
		updateReq.Content = &params.Content
	}
	if params.Description != "" {
		updateReq.Description = &params.Description
	}
	if params.Type != "" {
		updateReq.Type = &params.Type
	}
	if params.Status != "" {
		updateReq.Status = &params.Status
	}
	if params.Subtype != nil {
		updateReq.Subtype = params.Subtype
	}
	if params.Metadata != nil {
		updateReq.Metadata = params.Metadata
	}
	return updateReq
}

// updateBlueprint implements the tool that updates a specific blueprint
// (located by project_id + slug) in the resolved team.
func (s *Server) updateBlueprint(
	ctx context.Context, _ *mcp.CallToolRequest, params *UpdateBlueprintParams, userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	if r := validateProjectID(params.ProjectID); r != nil {
		logrus.WithFields(logrus.Fields{
			"tool":       "vibexp_io_update_blueprint",
			"user_id":    userID,
			"team_id":    teamID,
			"project_id": params.ProjectID,
		}).Warn("MCP tool rejected: invalid project_id")
		return r, nil, nil
	}

	updateReq := buildBlueprintUpdateRequest(params)

	blueprint, err := s.container.BlueprintService().UpdateBlueprintByProjectIDAndSlug(
		userID, params.ProjectID, params.Slug, updateReq,
	)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"tool":       "vibexp_io_update_blueprint",
			"user_id":    userID,
			"team_id":    teamID,
			"project_id": params.ProjectID,
			"slug":       params.Slug,
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to update blueprint via MCP")

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to update blueprint: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	if s.metrics != nil {
		s.metrics.RecordMCPUpdateBlueprint(context.Background())
	}

	return marshalBlueprintWriteResult(s.config.FrontendBaseURL, blueprint)
}

// marshalBlueprintWriteResult builds the slim create/update response and marshals it
// to JSON text plus StructuredContent, mirroring the artifact/prompt write tools.
func marshalBlueprintWriteResult(baseURL string, blueprint *models.Blueprint) (*mcp.CallToolResult, any, error) {
	result := &blueprintWriteResponse{
		ID:      blueprint.ID,
		Slug:    blueprint.Slug,
		FullURL: buildBlueprintURL(baseURL, blueprint.ProjectID, blueprint.Slug),
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
