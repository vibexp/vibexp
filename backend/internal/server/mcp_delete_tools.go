package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// deleteResourceToolDescription documents the single generic delete tool. One
// tool covers four resource types so the MCP tool list (and its per-session
// schema token cost) stays small instead of growing one delete tool per type.
const deleteResourceToolDescription = "Delete a single resource. Supported resource_type values and their " +
	"required identifiers: \"memory\" needs id; \"prompt\" needs slug; " +
	"\"artifact\" and \"blueprint\" need project_id and slug. The team is resolved " +
	"and membership-checked per call; the resource's embeddings (and an artifact's " +
	"attachments) are removed alongside it."

// DeleteResourceParams defines the parameters for the generic delete_resource tool.
// The required identifier fields vary by resource_type and are validated per type.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type DeleteResourceParams struct {
	TeamID       string `json:"team_id" jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	ResourceType string `json:"resource_type" jsonschema:"REQUIRED. The resource type to delete: one of \"memory\", \"artifact\", \"blueprint\", or \"prompt\"."`
	ID           string `json:"id,omitempty" jsonschema:"Resource UUID. REQUIRED when resource_type is \"memory\"; ignored otherwise."`
	ProjectID    string `json:"project_id,omitempty" jsonschema:"Project UUID. REQUIRED when resource_type is \"artifact\" or \"blueprint\"; ignored otherwise."`
	Slug         string `json:"slug,omitempty" jsonschema:"Resource slug. REQUIRED when resource_type is \"prompt\", \"artifact\", or \"blueprint\"; ignored otherwise."`
}

// deleteResourceResponse is the slim, structured result returned on a successful delete.
type deleteResourceResponse struct {
	Deleted      bool   `json:"deleted"`
	ResourceType string `json:"resource_type"`
	ID           string `json:"id,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	Slug         string `json:"slug,omitempty"`
}

const deleteResourceToolName = "vibexp_io_delete_resource"

// deleteResource implements the generic delete tool. It resolves and
// membership-checks the team, then dispatches to a per-type handler that
// validates that type's required identifiers before deleting. This mirrors the
// REST DELETE handlers, including their best-effort embeddings/attachment
// cleanup, so the end state is identical regardless of entry point.
func (s *Server) deleteResource(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *DeleteResourceParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	switch strings.ToLower(strings.TrimSpace(params.ResourceType)) {
	case resourceTypeMemory:
		return s.deleteMemoryResource(ctx, params, userID, teamID)
	case resourceTypePrompt:
		return s.deletePromptResource(ctx, params, userID, teamID)
	case resourceTypeArtifact:
		return s.deleteArtifactResource(ctx, params, userID, teamID)
	case resourceTypeBlueprint:
		return s.deleteBlueprintResource(ctx, params, userID, teamID)
	default:
		return mcpTextError(fmt.Sprintf(
			"resource_type must be one of: %s, %s, %s, %s",
			resourceTypeMemory, resourceTypeArtifact, resourceTypeBlueprint, resourceTypePrompt,
		)), nil, nil
	}
}

// deleteMemoryResource deletes a memory by id (team-scoped at the repo layer).
func (s *Server) deleteMemoryResource(
	ctx context.Context, params *DeleteResourceParams, userID, teamID string,
) (*mcp.CallToolResult, any, error) {
	id := strings.TrimSpace(params.ID)
	if id == "" {
		return mcpTextError("id is required to delete a memory"), nil, nil
	}

	if err := s.container.MemoryService().DeleteMemory(userID, teamID, id); err != nil {
		return s.mcpDeleteError(resourceTypeMemory, userID, teamID, err), nil, nil
	}

	// Best-effort embeddings cleanup mirrors handleDeleteMemory; keyed on entity id only.
	s.deleteMCPResourceEmbeddings(resourceTypeMemory, userID, teamID, id)

	return s.deleteResourceSuccess(ctx, &deleteResourceResponse{
		Deleted: true, ResourceType: resourceTypeMemory, ID: id,
	})
}

// deletePromptResource deletes a prompt by slug. The prompt is loaded first to
// capture its id for embeddings cleanup, matching handleDeletePrompt.
func (s *Server) deletePromptResource(
	ctx context.Context, params *DeleteResourceParams, userID, teamID string,
) (*mcp.CallToolResult, any, error) {
	slug := strings.TrimSpace(params.Slug)
	if slug == "" {
		return mcpTextError("slug is required to delete a prompt"), nil, nil
	}

	prompt, err := s.container.PromptService().GetPromptBySlug(userID, teamID, slug)
	if err != nil {
		return s.mcpDeleteError(resourceTypePrompt, userID, teamID, err), nil, nil
	}

	if err := s.container.PromptService().DeletePromptBySlug(userID, teamID, slug); err != nil {
		return s.mcpDeleteError(resourceTypePrompt, userID, teamID, err), nil, nil
	}

	s.deleteMCPResourceEmbeddings(resourceTypePrompt, userID, teamID, prompt.ID)

	return s.deleteResourceSuccess(ctx, &deleteResourceResponse{
		Deleted: true, ResourceType: resourceTypePrompt, ID: prompt.ID, Slug: slug,
	})
}

// deleteArtifactResource deletes an artifact by project_id + slug. It loads the
// artifact through the team-enforcing lookup (consistent with the get/update
// artifact MCP tools) both to authorize against the resolved team and to capture
// the id for embeddings/attachment cleanup.
func (s *Server) deleteArtifactResource(
	ctx context.Context, params *DeleteResourceParams, userID, teamID string,
) (*mcp.CallToolResult, any, error) {
	if r := validateProjectID(params.ProjectID); r != nil {
		return r, nil, nil
	}
	slug := strings.TrimSpace(params.Slug)
	if slug == "" {
		return mcpTextError("slug is required to delete an artifact"), nil, nil
	}

	artifact, err := s.container.ArtifactService().GetArtifactByProjectIDAndSlugInTeam(
		userID, teamID, params.ProjectID, slug,
	)
	if err != nil {
		return s.mcpDeleteError(resourceTypeArtifact, userID, teamID, err), nil, nil
	}

	if err := s.container.ArtifactService().DeleteArtifactByProjectIDAndSlug(userID, params.ProjectID, slug); err != nil {
		return s.mcpDeleteError(resourceTypeArtifact, userID, teamID, err), nil, nil
	}

	s.deleteArtifactEmbeddings(userID, artifact.ID, params.ProjectID, slug)
	s.deleteArtifactAttachments(userID, artifact.ID)

	return s.deleteResourceSuccess(ctx, &deleteResourceResponse{
		Deleted: true, ResourceType: resourceTypeArtifact, ID: artifact.ID, ProjectID: params.ProjectID, Slug: slug,
	})
}

// deleteBlueprintResource deletes a blueprint by project_id + slug. There is no
// team-enforcing blueprint lookup, so it uses the same ownership-scoped lookup
// the REST handler uses, then captures the id for embeddings cleanup.
func (s *Server) deleteBlueprintResource(
	ctx context.Context, params *DeleteResourceParams, userID, teamID string,
) (*mcp.CallToolResult, any, error) {
	if r := validateProjectID(params.ProjectID); r != nil {
		return r, nil, nil
	}
	slug := strings.TrimSpace(params.Slug)
	if slug == "" {
		return mcpTextError("slug is required to delete a blueprint"), nil, nil
	}

	blueprint, err := s.container.BlueprintService().GetBlueprintByProjectIDAndSlug(userID, params.ProjectID, slug)
	if err != nil {
		return s.mcpDeleteError(resourceTypeBlueprint, userID, teamID, err), nil, nil
	}

	if err := s.container.BlueprintService().DeleteBlueprintByProjectIDAndSlug(
		userID, params.ProjectID, slug,
	); err != nil {
		return s.mcpDeleteError(resourceTypeBlueprint, userID, teamID, err), nil, nil
	}

	s.deleteBlueprintEmbeddings(userID, blueprint.ID, params.ProjectID, slug)

	return s.deleteResourceSuccess(ctx, &deleteResourceResponse{
		Deleted: true, ResourceType: resourceTypeBlueprint, ID: blueprint.ID, ProjectID: params.ProjectID, Slug: slug,
	})
}

// deleteMCPResourceEmbeddings removes a resource's embeddings best-effort. A
// failure is logged and swallowed (the row is already gone) — the same contract
// the REST delete handlers use for memory and prompt embeddings.
func (s *Server) deleteMCPResourceEmbeddings(resourceType, userID, teamID, entityID string) {
	if err := s.container.EmbeddingService().DeleteEmbeddingsByEntity(resourceType, entityID); err != nil {
		slog.Warn(
			"Failed to delete resource embeddings via MCP (non-fatal)",
			"tool", deleteResourceToolName,
			"resource_type", resourceType,
			"user_id", userID,
			"team_id", teamID,
			"entity_id", entityID,
			"error", fmt.Sprintf("%+v", err),
		)
	}
}

// mcpDeleteError logs a delete failure and returns the IsError result to hand back.
func (s *Server) mcpDeleteError(resourceType, userID, teamID string, err error) *mcp.CallToolResult {
	slog.Error(
		"Failed to delete resource via MCP",
		"tool", deleteResourceToolName,
		"resource_type", resourceType,
		"user_id", userID,
		"team_id", teamID,
		"error", fmt.Sprintf("%+v", err),
	)
	return mcpTextError(fmt.Sprintf("Failed to delete %s: %v", resourceType, err))
}

// deleteResourceSuccess records the metric and marshals the structured result.
func (s *Server) deleteResourceSuccess(
	ctx context.Context, result *deleteResourceResponse,
) (*mcp.CallToolResult, any, error) {
	if s.metrics != nil {
		s.metrics.RecordMCPDeleteResource(ctx, result.ResourceType)
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
