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
	"github.com/vibexp/vibexp/internal/services"
)

// Artifact Tool Parameters

// CreateArtifactParams defines the parameters for creating a new artifact
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type CreateArtifactParams struct {
	TeamID      string                 `json:"team_id" jsonschema:"REQUIRED. The team UUID or slug to operate within. Call vibexp_io_list_teams first if you don't have one."`
	ProjectID   string                 `json:"project_id" jsonschema:"Project UUID — required"`
	Slug        string                 `json:"slug" jsonschema:"Unique identifier for the artifact (max 255 chars)"`
	Title       string                 `json:"title" jsonschema:"Human-readable artifact title (max 255 chars)"`
	Content     string                 `json:"content" jsonschema:"Full artifact content"`
	Description string                 `json:"description,omitempty" jsonschema:"Brief description (max 500 chars)"`
	Type        string                 `json:"type,omitempty" jsonschema:"Team type slug. Defaults: general, work-reports, static-contexts; teams may add custom types. Defaults to general when omitted."`
	Status      string                 `json:"status,omitempty" jsonschema:"One of \"active\", \"draft\", \"archived\""`
	Metadata    map[string]interface{} `json:"metadata,omitempty" jsonschema:"Key-value metadata pairs"`
}

// ListArtifactsByProjectParams defines the parameters for listing artifacts by project
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type ListArtifactsByProjectParams struct {
	TeamID      string `json:"team_id" jsonschema:"REQUIRED. The team UUID or slug to operate within. Call vibexp_io_list_teams first if you don't have one."`
	ProjectID   string `json:"project_id" jsonschema:"Project UUID — required"`
	Page        int    `json:"page,omitempty" jsonschema:"Page number (default: 1)"`
	Limit       int    `json:"limit,omitempty" jsonschema:"Items per page (default: 10, max: 10)"`
	Status      string `json:"status,omitempty" jsonschema:"Filter by status"`
	Type        string `json:"type,omitempty" jsonschema:"Filter by type"`
	Search      string `json:"search,omitempty" jsonschema:"Search in title/description"`
	FullDetails bool   `json:"full_details,omitempty" jsonschema:"Not supported — search_artifacts never returns full content; use get_artifact for full content"`
}

// GetArtifactParams defines the parameters for getting a specific artifact
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type GetArtifactParams struct {
	TeamID    string `json:"team_id" jsonschema:"REQUIRED. The team UUID or slug to operate within. Call vibexp_io_list_teams first if you don't have one."`
	ProjectID string `json:"project_id" jsonschema:"Project UUID — required"`
	Slug      string `json:"slug" jsonschema:"Artifact slug identifier"`
}

// UpdateArtifactParams defines the parameters for updating a specific artifact
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type UpdateArtifactParams struct {
	TeamID      string                 `json:"team_id" jsonschema:"REQUIRED. The team UUID or slug to operate within. Call vibexp_io_list_teams first if you don't have one."`
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

// normalizeArtifactListPagination applies default and max bounds to page/limit.
func normalizeArtifactListPagination(page, limit int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 10 {
		limit = 10
	}
	return page, limit
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
func (s *Server) createArtifact(
	ctx context.Context, _ *mcp.CallToolRequest, params *CreateArtifactParams, userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	if r := validateProjectID(params.ProjectID); r != nil {
		logrus.WithFields(logrus.Fields{
			"tool":       "vibexp_io_create_artifact",
			"user_id":    userID,
			"team_id":    teamID,
			"project_id": params.ProjectID,
		}).Warn("MCP tool rejected: invalid project_id")
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
		logrus.WithFields(logrus.Fields{
			"user_id": userID,
			"team_id": teamID,
		}).WithError(err).Error("Failed to create artifact via MCP")
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
		FullURL: buildArtifactURL(s.config.FrontendBaseURL, artifact.ProjectID, artifact.Slug),
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

// listArtifactsByProject implements the search_artifacts tool scoped to the resolved team.
//
//nolint:funlen // Must resolve team, reject full_details, validate project, call service, and marshal
func (s *Server) listArtifactsByProject(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *ListArtifactsByProjectParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	// search_artifacts never returns full content — reject full_details=true
	if params.FullDetails {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "full_details is not supported on search_artifacts; call get_artifact for full content"},
			},
			IsError: true,
		}, nil, nil
	}

	if r := validateProjectID(params.ProjectID); r != nil {
		logrus.WithFields(logrus.Fields{
			"tool":       "vibexp_io_search_artifacts",
			"user_id":    userID,
			"team_id":    teamID,
			"project_id": params.ProjectID,
		}).Warn("MCP tool rejected: invalid project_id")
		return r, nil, nil
	}

	page, limit := normalizeArtifactListPagination(params.Page, params.Limit)
	filters := services.ArtifactFilters{
		ProjectID: params.ProjectID,
		Status:    params.Status,
		Type:      params.Type,
		Search:    params.Search,
		TeamID:    teamID,
		Page:      page,
		Limit:     limit,
	}

	response, err := s.container.ArtifactService().ListArtifactsByProject(userID, params.ProjectID, filters)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"tool":       "vibexp_io_search_artifacts",
			"user_id":    userID,
			"team_id":    teamID,
			"project_id": params.ProjectID,
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to search artifacts via MCP")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to search artifacts: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	if s.metrics != nil {
		s.metrics.RecordMCPListArtifacts(context.Background())
	}

	searchResp := &artifactSearchResponse{
		Artifacts:  buildArtifactSearchItems(response.Artifacts),
		TotalCount: response.TotalCount,
		Page:       response.Page,
		PerPage:    response.PerPage,
		TotalPages: response.TotalPages,
	}

	jsonData, err := json.MarshalIndent(searchResp, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal response to JSON: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
		StructuredContent: searchResp,
	}, searchResp, nil
}

// getArtifact implements the tool that gets a specific artifact in the resolved team.
func (s *Server) getArtifact(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *GetArtifactParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	if r := validateProjectID(params.ProjectID); r != nil {
		logrus.WithFields(logrus.Fields{
			"tool":       "vibexp_io_get_artifact",
			"user_id":    userID,
			"team_id":    teamID,
			"project_id": params.ProjectID,
		}).Warn("MCP tool rejected: invalid project_id")
		return r, nil, nil
	}

	artifact, err := s.container.ArtifactService().GetArtifactByProjectIDAndSlugInTeam(
		userID, teamID, params.ProjectID, params.Slug,
	)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"tool":       "vibexp_io_get_artifact",
			"user_id":    userID,
			"team_id":    teamID,
			"project_id": params.ProjectID,
			"slug":       params.Slug,
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to get artifact via MCP")

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to get artifact: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	if s.metrics != nil {
		s.metrics.RecordMCPGetArtifact(context.Background())
	}

	jsonData, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal response to JSON: %w", err)
	}

	// Record the successful detail read; MCP get-tools bypass the HTTP middleware.
	// Recorded only after a successful response, mirroring the HTTP 2xx contract.
	s.recordMCPResourceAccess(ctx, teamID, userID, resourceTypeArtifact, artifact.ID)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
		StructuredContent: artifact,
	}, artifact, nil
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
		logrus.WithFields(logrus.Fields{
			"tool":       "vibexp_io_update_artifact",
			"user_id":    userID,
			"team_id":    teamID,
			"project_id": params.ProjectID,
		}).Warn("MCP tool rejected: invalid project_id")
		return r, nil, nil
	}

	updateReq := buildArtifactUpdateRequest(params)

	artifact, err := s.container.ArtifactService().UpdateArtifactByProjectIDAndSlugInTeam(
		userID, teamID, params.ProjectID, params.Slug, updateReq,
	)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"tool":       "vibexp_io_update_artifact",
			"user_id":    userID,
			"team_id":    teamID,
			"project_id": params.ProjectID,
			"slug":       params.Slug,
			"error":      fmt.Sprintf("%+v", err),
		}).Error("Failed to update artifact via MCP")

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
		FullURL: buildArtifactURL(s.config.FrontendBaseURL, artifact.ProjectID, artifact.Slug),
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
