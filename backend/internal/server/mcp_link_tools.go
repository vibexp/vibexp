package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/models"
)

const linkResourcesToolName = "vibexp_io_link_resources"

// linkResourcesToolDescription is the §8.1 write-path lever: agents only call a
// tool they are nudged toward, so the description tells them WHEN to link and
// embeds the type-constraint matrix so they can self-validate before calling.
const linkResourcesToolDescription = "Record a typed relation between two resources in a project, so a resource's " +
	"neighborhood compounds for the whole team. After creating or updating a resource, record how it relates to " +
	"existing resources: 'governed-by' a blueprint it must obey, 'supersedes' the version it replaces, 'built-from' " +
	"the prompt that produced it, 'explained-by' the memory that holds the why. Re-linking an existing edge is a safe " +
	"no-op (idempotent). Type rules: governed-by → the object (to) must be a blueprint; built-from → the object must " +
	"be a prompt; explained-by → the object must be a memory; supersedes → both ends must be the same type. " +
	"Self-links and cross-project links are rejected. from_type/to_type are one of artifact|memory|prompt|blueprint. " +
	"Edges you propose are recorded as AI-suggested; the higher-stakes governed-by/supersedes await a human confirm."

// LinkResourcesParams defines the parameters for the link_resources tool. The
// schema is auto-derived from these jsonschema tags (GetResourceParams precedent).
type LinkResourcesParams struct {
	TeamID       string `json:"team_id" jsonschema:"REQUIRED. Team UUID or slug."`
	ProjectID    string `json:"project_id" jsonschema:"REQUIRED. Project UUID; both resources must be in it."`
	FromType     string `json:"from_type" jsonschema:"REQUIRED. Subject type: artifact|memory|prompt|blueprint."`
	FromID       string `json:"from_id" jsonschema:"REQUIRED. Subject resource UUID."`
	RelationType string `json:"relation_type" jsonschema:"REQUIRED. governed-by|supersedes|built-from|explained-by."`
	ToType       string `json:"to_type" jsonschema:"REQUIRED. Object type; constrained by relation_type."`
	ToID         string `json:"to_id" jsonschema:"REQUIRED. Object resource UUID."`
}

// linkResources implements the vibexp_io_link_resources tool. It resolves and
// membership-checks the team (first statement — anti-enumeration), then delegates
// to RelationService.Create with origin=ai (tiered trust: governed-by/supersedes
// born suggested, built-from/explained-by born confirmed). Creation is idempotent.
func (s *Server) linkResources(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *LinkResourcesParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	relation, _, err := s.container.RelationService().Create(ctx, userID, teamID, &models.CreateRelationRequest{
		FromType:     params.FromType,
		FromID:       params.FromID,
		ToType:       params.ToType,
		ToID:         params.ToID,
		RelationType: params.RelationType,
		Origin:       models.RelationOriginAI,
	})
	if err != nil {
		s.logger.With(
			"handler", "linkResources",
			"team_id", teamID,
			"project_id", params.ProjectID,
			"relation_type", params.RelationType,
			"error", err.Error(),
		).Warn("Failed to link resources over MCP")
		return mcpTextError(err.Error()), nil, nil
	}

	s.recordMCPResourceAccess(ctx, teamID, userID, relation.FromType, relation.FromID)
	return mcpJSONResult(relation)
}

// addLinkTools registers the relation write tool (link_resources).
func (m *MCPToolsManager) addLinkTools(mcpServer *mcp.Server, userID string) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        linkResourcesToolName,
		Description: linkResourcesToolDescription,
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *LinkResourcesParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.linkResources(ctx, req, params, userID)
	})
}
