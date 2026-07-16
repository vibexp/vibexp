package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// The unified read tools collapse the former per-domain read tools
// (get_artifact / get_memory / search_artifacts / search_memories) into two
// resource_type-keyed tools, mirroring delete_resource. Their params are
// homogeneous (identifiers for get; project_id + filters for list), so a single
// discriminator keeps the tool list small without the prose-gated field unions
// that would hurt a field-rich write. Blueprint reads — which had no MCP tool
// before — are covered here for free.
const getResourceToolName = "vibexp_io_get_resource"

const getResourceToolDescription = "Fetch a single resource by identifier and return its full content. " +
	"Supported resource_type values and their required identifiers: \"memory\" needs id; " +
	"\"artifact\" and \"blueprint\" need project_id and slug. The team is resolved and membership-checked per call."

const listResourcesToolName = "vibexp_io_list_resources"

const listResourcesToolDescription = "List resources in a project with filtering and pagination. Supported " +
	"resource_type values: \"memory\", \"artifact\", \"blueprint\". Requires project_id. Returns slim items " +
	"without full content — call get_resource for a single resource's content. The team is resolved per call."

// GetResourceParams defines the parameters for the generic get_resource tool.
// The required identifier fields vary by resource_type and are validated per type.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type GetResourceParams struct {
	TeamID       string `json:"team_id" jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	ResourceType string `json:"resource_type" jsonschema:"REQUIRED. The resource type to fetch: one of \"memory\", \"artifact\", or \"blueprint\"."`
	ID           string `json:"id,omitempty" jsonschema:"Resource UUID. REQUIRED when resource_type is \"memory\"; ignored otherwise."`
	ProjectID    string `json:"project_id,omitempty" jsonschema:"Project UUID. REQUIRED when resource_type is \"artifact\" or \"blueprint\"; ignored otherwise."`
	Slug         string `json:"slug,omitempty" jsonschema:"Resource slug. REQUIRED when resource_type is \"artifact\" or \"blueprint\"; ignored otherwise."`
}

// ListResourcesParams defines the parameters for the generic list_resources tool.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type ListResourcesParams struct {
	TeamID       string `json:"team_id" jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	ResourceType string `json:"resource_type" jsonschema:"REQUIRED. The resource type to list: one of \"memory\", \"artifact\", or \"blueprint\"."`
	ProjectID    string `json:"project_id" jsonschema:"REQUIRED. Project UUID to list resources within."`
	Page         int    `json:"page,omitempty" jsonschema:"Page number (default: 1)"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Items per page (default: 10, max: 10)"`
	Search       string `json:"search,omitempty" jsonschema:"Search filter (memory text, or artifact/blueprint title/description)"`
	Status       string `json:"status,omitempty" jsonschema:"Filter by status"`
	Type         string `json:"type,omitempty" jsonschema:"Filter by type (artifact and blueprint only)"`
}

// blueprintListItem is the per-item shape returned by list_resources for
// blueprints: the full model minus its (potentially large) content, mirroring
// artifactSearchItem so the list stays a slim, browseable index.
type blueprintListItem struct {
	ID          string                 `json:"id"`
	ProjectID   string                 `json:"project_id"`
	Slug        string                 `json:"slug"`
	UserID      string                 `json:"user_id"`
	TeamID      string                 `json:"team_id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"`
	Subtype     *string                `json:"subtype,omitempty"`
	Status      string                 `json:"status"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

// blueprintListResponse is the list shape returned by list_resources for blueprints.
type blueprintListResponse struct {
	Blueprints []blueprintListItem `json:"blueprints"`
	TotalCount int                 `json:"total_count"`
	Page       int                 `json:"page"`
	PerPage    int                 `json:"per_page"`
	TotalPages int                 `json:"total_pages"`
}

// normalizeMCPListPagination applies default and max bounds to page/limit for
// the MCP list tools (default page 1, default+max limit 10).
func normalizeMCPListPagination(page, limit int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	return page, limit
}

// getResource implements the generic get_resource tool. It resolves and
// membership-checks the team, then dispatches to a per-type handler that
// validates that type's required identifiers before fetching.
func (s *Server) getResource(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *GetResourceParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	switch strings.ToLower(strings.TrimSpace(params.ResourceType)) {
	case resourceTypeMemory:
		return s.getMemoryResource(ctx, params, userID, teamID)
	case resourceTypeArtifact:
		return s.getArtifactResource(ctx, params, userID, teamID)
	case resourceTypeBlueprint:
		return s.getBlueprintResource(ctx, params, userID, teamID)
	default:
		return mcpTextError(fmt.Sprintf(
			"resource_type must be one of: %s, %s, %s",
			resourceTypeMemory, resourceTypeArtifact, resourceTypeBlueprint,
		)), nil, nil
	}
}

// getMemoryResource fetches a memory by id (team-scoped at the service layer).
func (s *Server) getMemoryResource(
	ctx context.Context, params *GetResourceParams, userID, teamID string,
) (*mcp.CallToolResult, any, error) {
	id := strings.TrimSpace(params.ID)
	if id == "" {
		return mcpTextError("id is required to get a memory"), nil, nil
	}

	memory, err := s.container.MemoryService().GetMemory(userID, teamID, id)
	if err != nil {
		return s.mcpReadError(resourceTypeMemory, "get", userID, teamID, err), nil, nil
	}

	if s.metrics != nil {
		s.metrics.RecordMCPGetMemory(ctx)
	}

	s.recordMCPResourceAccess(ctx, teamID, userID, resourceTypeMemory, memory.ID)
	return mcpJSONResult(memory)
}

// getArtifactResource fetches an artifact by project_id + slug through the
// team-enforcing lookup.
func (s *Server) getArtifactResource(
	ctx context.Context, params *GetResourceParams, userID, teamID string,
) (*mcp.CallToolResult, any, error) {
	if r := validateProjectID(params.ProjectID); r != nil {
		return r, nil, nil
	}
	slug := strings.TrimSpace(params.Slug)
	if slug == "" {
		return mcpTextError("slug is required to get an artifact"), nil, nil
	}

	artifact, err := s.container.ArtifactService().GetArtifactByProjectIDAndSlugInTeam(
		userID, teamID, params.ProjectID, slug,
	)
	if err != nil {
		return s.mcpReadError(resourceTypeArtifact, "get", userID, teamID, err), nil, nil
	}

	if s.metrics != nil {
		s.metrics.RecordMCPGetArtifact(context.Background())
	}

	s.recordMCPResourceAccess(ctx, teamID, userID, resourceTypeArtifact, artifact.ID)
	return mcpJSONResult(artifact)
}

// getBlueprintResource fetches a blueprint by project_id + slug. Blueprint reads
// are ownership-scoped (cross-team by userID), the same lookup the delete and
// REST blueprint handlers use.
func (s *Server) getBlueprintResource(
	ctx context.Context, params *GetResourceParams, userID, teamID string,
) (*mcp.CallToolResult, any, error) {
	if r := validateProjectID(params.ProjectID); r != nil {
		return r, nil, nil
	}
	slug := strings.TrimSpace(params.Slug)
	if slug == "" {
		return mcpTextError("slug is required to get a blueprint"), nil, nil
	}

	blueprint, err := s.container.BlueprintService().GetBlueprintByProjectIDAndSlug(userID, params.ProjectID, slug)
	if err != nil {
		return s.mcpReadError(resourceTypeBlueprint, "get", userID, teamID, err), nil, nil
	}

	s.recordMCPResourceAccess(ctx, teamID, userID, resourceTypeBlueprint, blueprint.ID)
	return mcpJSONResult(blueprint)
}

// listResources implements the generic list_resources tool.
func (s *Server) listResources(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *ListResourcesParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	if r := validateProjectID(params.ProjectID); r != nil {
		slog.Warn(
			"MCP tool rejected: invalid project_id",
			"tool", listResourcesToolName,
			"user_id", userID,
			"team_id", teamID,
			"project_id", params.ProjectID,
		)
		return r, nil, nil
	}

	switch strings.ToLower(strings.TrimSpace(params.ResourceType)) {
	case resourceTypeMemory:
		return s.listMemoryResources(params, userID, teamID)
	case resourceTypeArtifact:
		return s.listArtifactResources(params, userID, teamID)
	case resourceTypeBlueprint:
		return s.listBlueprintResources(params, userID, teamID)
	default:
		return mcpTextError(fmt.Sprintf(
			"resource_type must be one of: %s, %s, %s",
			resourceTypeMemory, resourceTypeArtifact, resourceTypeBlueprint,
		)), nil, nil
	}
}

// listMemoryResources lists memories in a project (slim items, text truncated).
func (s *Server) listMemoryResources(
	params *ListResourcesParams, userID, teamID string,
) (*mcp.CallToolResult, any, error) {
	statusPtr, statusErr := validateMCPMemoryStatus(params.Status)
	if statusErr != nil {
		return statusErr, nil, nil
	}

	page, limit := normalizeMCPListPagination(params.Page, params.Limit)
	projectID := params.ProjectID
	filters := services.MemoryFilters{
		TeamID:    teamID,
		ProjectID: &projectID,
		Search:    params.Search,
		Status:    statusPtr,
		Page:      page,
		Limit:     limit,
	}

	response, err := s.container.MemoryService().ListMemories(userID, filters)
	if err != nil {
		return s.mcpReadError(resourceTypeMemory, "list", userID, teamID, err), nil, nil
	}

	if s.metrics != nil {
		s.metrics.RecordMCPListMemories(context.Background())
	}

	return mcpJSONResult(&memorySearchResponse{
		Memories:   buildMemorySearchItems(response.Memories),
		TotalCount: response.TotalCount,
		Page:       response.Page,
		PerPage:    response.PerPage,
		TotalPages: response.TotalPages,
	})
}

// listArtifactResources lists artifacts in a project (slim items, no content).
func (s *Server) listArtifactResources(
	params *ListResourcesParams, userID, teamID string,
) (*mcp.CallToolResult, any, error) {
	page, limit := normalizeMCPListPagination(params.Page, params.Limit)
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
		return s.mcpReadError(resourceTypeArtifact, "list", userID, teamID, err), nil, nil
	}

	if s.metrics != nil {
		s.metrics.RecordMCPListArtifacts(context.Background())
	}

	return mcpJSONResult(&artifactSearchResponse{
		Artifacts:  buildArtifactSearchItems(response.Artifacts),
		TotalCount: response.TotalCount,
		Page:       response.Page,
		PerPage:    response.PerPage,
		TotalPages: response.TotalPages,
	})
}

// listBlueprintResources lists blueprints in a project (slim items, no content).
// Blueprint listing is ownership-scoped (cross-team by userID) with the resolved
// team applied as a filter, mirroring the REST blueprint list handler.
func (s *Server) listBlueprintResources(
	params *ListResourcesParams, userID, teamID string,
) (*mcp.CallToolResult, any, error) {
	page, limit := normalizeMCPListPagination(params.Page, params.Limit)
	filters := services.BlueprintFilters{
		Status: params.Status,
		Type:   params.Type,
		TeamID: teamID,
		Search: params.Search,
		Page:   page,
		Limit:  limit,
	}

	response, err := s.container.BlueprintService().ListBlueprintsByProject(userID, params.ProjectID, filters)
	if err != nil {
		return s.mcpReadError(resourceTypeBlueprint, "list", userID, teamID, err), nil, nil
	}

	return mcpJSONResult(&blueprintListResponse{
		Blueprints: buildBlueprintListItems(response.Blueprints),
		TotalCount: response.TotalCount,
		Page:       response.Page,
		PerPage:    response.PerPage,
		TotalPages: response.TotalPages,
	})
}

// buildBlueprintListItems converts service blueprints to the slim list-item
// shape (dropping content), mirroring buildArtifactSearchItems.
func buildBlueprintListItems(blueprints []models.Blueprint) []blueprintListItem {
	items := make([]blueprintListItem, 0, len(blueprints))
	for _, b := range blueprints {
		items = append(items, blueprintListItem{
			ID:          b.ID,
			ProjectID:   b.ProjectID,
			Slug:        b.Slug,
			UserID:      b.UserID,
			TeamID:      b.TeamID,
			Title:       b.Title,
			Description: b.Description,
			Type:        b.Type,
			Subtype:     b.Subtype,
			Status:      b.Status,
			Metadata:    b.Metadata,
			CreatedAt:   b.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			UpdatedAt:   b.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return items
}

// mcpReadError logs a read failure and returns the IsError result to hand back.
func (s *Server) mcpReadError(resourceType, op, userID, teamID string, err error) *mcp.CallToolResult {
	slog.Error(
		"Failed to read resource via MCP",
		"tool", getResourceToolName,
		"op", op,
		"resource_type", resourceType,
		"user_id", userID,
		"team_id", teamID,
		"error", fmt.Sprintf("%+v", err),
	)
	return mcpTextError(fmt.Sprintf("Failed to %s %s: %v", op, resourceType, err))
}
