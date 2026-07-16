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

// Artifact Tool Parameters

// CreateArtifactParams defines the parameters for creating a new artifact
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type CreateArtifactParams struct {
	TeamID      string                 `json:"team_id" jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	ProjectID   string                 `json:"project_id" jsonschema:"Project UUID — required"`
	Slug        string                 `json:"slug" jsonschema:"Unique identifier for the artifact (max 255 chars)"`
	Title       string                 `json:"title" jsonschema:"Human-readable artifact title (max 255 chars)"`
	Content     string                 `json:"content" jsonschema:"Full artifact content"`
	Description string                 `json:"description,omitempty" jsonschema:"Brief description (max 500 chars)"`
	Type        string                 `json:"type,omitempty" jsonschema:"Team type slug. Defaults: general, work-reports, static-contexts; teams may add custom types. Defaults to general when omitted."`
	Status      string                 `json:"status,omitempty" jsonschema:"One of \"active\", \"draft\", \"archived\""`
	Metadata    map[string]interface{} `json:"metadata,omitempty" jsonschema:"Key-value metadata pairs"`
}

// UpdateArtifactParams defines the parameters for updating a specific artifact
type UpdateArtifactParams struct {
	TeamID      string                 `json:"team_id" jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	ProjectID   string                 `json:"project_id" jsonschema:"Project UUID — required"`
	Slug        string                 `json:"slug" jsonschema:"Artifact slug identifier"`
	Title       string                 `json:"title,omitempty" jsonschema:"New title"`
	Content     string                 `json:"content,omitempty" jsonschema:"New content"`
	Description string                 `json:"description,omitempty" jsonschema:"New description"`
	Type        string                 `json:"type,omitempty" jsonschema:"New type"`
	Status      string                 `json:"status,omitempty" jsonschema:"New status"`
	Metadata    map[string]interface{} `json:"metadata,omitempty" jsonschema:"New metadata"`
}

// artifactWriteResponse is the slim response returned by create/update artifact tools.
type artifactWriteResponse struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	FullURL string `json:"full_url"`
}

// artifactSearchItem is the per-item shape returned by search_artifacts (no content field).
type artifactSearchItem struct {
	ID          string                 `json:"id"`
	ProjectID   string                 `json:"project_id"`
	Slug        string                 `json:"slug"`
	UserID      string                 `json:"user_id"`
	TeamID      string                 `json:"team_id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`
	Status      string                 `json:"status"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

// artifactSearchResponse is the list shape returned by search_artifacts.
type artifactSearchResponse struct {
	Artifacts  []artifactSearchItem `json:"artifacts"`
	TotalCount int                  `json:"total_count"`
	Page       int                  `json:"page"`
	PerPage    int                  `json:"per_page"`
	TotalPages int                  `json:"total_pages"`
}

// buildArtifactURL constructs the canonical web URL for an artifact.
func buildArtifactURL(baseURL, projectID, slug string) string {
	return fmt.Sprintf("%s/artifacts/%s/%s",
		strings.TrimRight(baseURL, "/"),
		url.PathEscape(projectID),
		url.PathEscape(slug),
	)
}

// Artifact Tool Implementations

// createArtifact implements the tool that creates a new artifact in the resolved team.
//
//nolint:funlen // structured slog attributes are marginally more verbose than the prior logrus WithFields calls
func (s *Server) createArtifact(
	ctx context.Context, _ *mcp.CallToolRequest, params *CreateArtifactParams, userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	if r := validateProjectID(params.ProjectID); r != nil {
		slog.Warn(
			"MCP tool rejected: invalid project_id",
			"tool", "vibexp_io_create_artifact",
			"user_id", userID,
			"team_id", teamID,
			"project_id", params.ProjectID,
		)
		return r, nil, nil
	}

	createReq := &models.CreateArtifactRequest{
		ProjectID:   params.ProjectID,
		Slug:        params.Slug,
		Title:       params.Title,
		Content:     params.Content,
		Description: params.Description,
		Type:        params.Type,
		Status:      params.Status,
		Metadata:    params.Metadata,
	}

	artifact, err := s.container.ArtifactService().CreateArtifact(userID, teamID, createReq)
	if err != nil {
		slog.Error(
			"Failed to create artifact via MCP",
			"user_id", userID,
			"team_id", teamID,
			"error", err,
		)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to create artifact: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	if s.metrics != nil {
		s.metrics.RecordMCPCreateArtifact(context.Background())
	}

	result := &artifactWriteResponse{
		ID:      artifact.ID,
		Slug:    artifact.Slug,
		FullURL: buildArtifactURL(s.config.Frontend.BaseURL, artifact.ProjectID, artifact.Slug),
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

// buildArtifactSearchItems converts service artifacts to the slim search-item shape.
func buildArtifactSearchItems(artifacts []models.Artifact) []artifactSearchItem {
	items := make([]artifactSearchItem, 0, len(artifacts))
	for _, a := range artifacts {
		items = append(items, artifactSearchItem{
			ID:          a.ID,
			ProjectID:   a.ProjectID,
			Slug:        a.Slug,
			UserID:      a.UserID,
			TeamID:      a.TeamID,
			Title:       a.Title,
			Description: a.Description,
			Type:        a.Type,
			Status:      a.Status,
			Metadata:    a.Metadata,
			CreatedAt:   a.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			UpdatedAt:   a.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return items
}

// buildArtifactUpdateRequest builds an UpdateArtifactRequest from non-empty params fields.
func buildArtifactUpdateRequest(params *UpdateArtifactParams) *models.UpdateArtifactRequest {
	updateReq := &models.UpdateArtifactRequest{}
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
	if params.Metadata != nil {
		updateReq.Metadata = params.Metadata
	}
	return updateReq
}

// updateArtifact implements the tool that updates a specific artifact in the resolved team.
//
//nolint:funlen // Must resolve team, validate project, build update request, call service, and marshal
func (s *Server) updateArtifact(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *UpdateArtifactParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	if r := validateProjectID(params.ProjectID); r != nil {
		slog.Warn(
			"MCP tool rejected: invalid project_id",
			"tool", "vibexp_io_update_artifact",
			"user_id", userID,
			"team_id", teamID,
			"project_id", params.ProjectID,
		)
		return r, nil, nil
	}

	updateReq := buildArtifactUpdateRequest(params)

	artifact, err := s.container.ArtifactService().UpdateArtifactByProjectIDAndSlugInTeam(
		userID, teamID, params.ProjectID, params.Slug, updateReq,
	)
	if err != nil {
		slog.Error(
			"Failed to update artifact via MCP",
			"tool", "vibexp_io_update_artifact",
			"user_id", userID,
			"team_id", teamID,
			"project_id", params.ProjectID,
			"slug", params.Slug,
			"error", fmt.Sprintf("%+v", err),
		)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to update artifact: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	if s.metrics != nil {
		s.metrics.RecordMCPUpdateArtifact(context.Background())
	}

	result := &artifactWriteResponse{
		ID:      artifact.ID,
		Slug:    artifact.Slug,
		FullURL: buildArtifactURL(s.config.Frontend.BaseURL, artifact.ProjectID, artifact.Slug),
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal response to JSON: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
		StructuredContent: result,
	}, result, nil
}
