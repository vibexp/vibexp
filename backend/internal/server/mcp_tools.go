package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/contextkeys"
)

// mcpServerInstructions is the server-level guidance returned to clients at
// initialize time. The detailed team-scoping explanation lives here once,
// rather than being repeated in every team-scoped tool's team_id parameter
// description, which keeps the per-tool schemas terse.
const mcpServerInstructions = "VibeXP MCP server. Most tools are team-scoped and take a required team_id " +
	"parameter — a team UUID or slug. If you do not have a team_id, call vibexp_io_list_teams first to " +
	"discover the teams you belong to, then pass one you are a member of (each call resolves and " +
	"membership-checks the team). The only user-scoped tools that take no team_id are vibexp_io_get_user " +
	"(your identity) and vibexp_io_list_teams (team discovery). As you work, record how resources relate " +
	"with vibexp_io_link_resources (governed-by, supersedes, built-from, explained-by); every " +
	"vibexp_io_get_resource returns the resource's typed neighborhood in its `related` array."

// MCPToolsManager manages all MCP tools and provides better organization
type MCPToolsManager struct {
	server *Server
}

// NewMCPToolsManager creates a new MCP tools manager
func NewMCPToolsManager(server *Server) *MCPToolsManager {
	return &MCPToolsManager{
		server: server,
	}
}

// AddAllTools registers every MCP tool on the given server for the authenticated
// user. Team-scoped tools take a required team_id (UUID or slug) parameter that
// each handler resolves and membership-checks per call via resolveTeam; the two
// exceptions are vibexp_io_get_user (user identity) and vibexp_io_list_teams
// (team discovery), which are user-scoped and take no team_id.
func (m *MCPToolsManager) AddAllTools(mcpServer *mcp.Server, userID string) {
	m.addUserTools(mcpServer, userID)
	m.addTeamTools(mcpServer, userID)
	m.addArtifactTools(mcpServer, userID)
	m.addMemoryTools(mcpServer, userID)
	m.addPromptTools(mcpServer, userID)
	m.addBlueprintTools(mcpServer, userID)
	m.addProjectTools(mcpServer, userID)
	m.addFeedTools(mcpServer, userID)
	m.addSearchTools(mcpServer, userID)
	m.addReadTools(mcpServer, userID)
	m.addAttachmentTools(mcpServer, userID)
	m.addLinkTools(mcpServer, userID)
	m.addDeleteTools(mcpServer, userID)

	slog.With(
		"user_id", userID,
	).Info("Added all MCP tools to server")
}

// addUserTools adds the user info tool (user-scoped; no team_id).
func (m *MCPToolsManager) addUserTools(mcpServer *mcp.Server, userID string) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_get_user",
		Description: "Get basic information about the currently authenticated user",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params *GetUserParams) (*mcp.CallToolResult, any, error) {
		return m.server.getUserWithUser(ctx, req, params, userID)
	})
}

// addTeamTools adds the team discovery tool (user-scoped; no team_id).
func (m *MCPToolsManager) addTeamTools(mcpServer *mcp.Server, userID string) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_list_teams",
		Description: listTeamsToolDescription,
	}, func(ctx context.Context, req *mcp.CallToolRequest, params *ListTeamsParams) (*mcp.CallToolResult, any, error) {
		return m.server.listTeamsForUser(ctx, req, params, userID)
	})
}

// addArtifactTools adds artifact write tools (create/update). Reads go through
// the generic get_resource / list_resources tools (addReadTools).
func (m *MCPToolsManager) addArtifactTools(mcpServer *mcp.Server, userID string) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_create_artifact",
		Description: "Create a new artifact",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *CreateArtifactParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.createArtifact(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_update_artifact",
		Description: "Update specific artifact",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *UpdateArtifactParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.updateArtifact(ctx, req, params, userID)
	})
}

// addReadTools adds the two generic, resource_type-keyed read tools that replace
// the former per-domain get/search tools. get_resource fetches one resource;
// list_resources lists a project's resources. Both cover memory, artifact, and
// blueprint (blueprints previously had no MCP read tool).
func (m *MCPToolsManager) addReadTools(mcpServer *mcp.Server, userID string) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        getResourceToolName,
		Description: getResourceToolDescription,
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *GetResourceParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.getResource(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        listResourcesToolName,
		Description: listResourcesToolDescription,
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *ListResourcesParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.listResources(ctx, req, params, userID)
	})
}

// addAttachmentTools adds the universal attachment management tools (upload,
// list, delete) keyed by owner_type/owner_id.
func (m *MCPToolsManager) addAttachmentTools(mcpServer *mcp.Server, userID string) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name: "vibexp_io_upload_attachment",
		Description: "Upload a base64-encoded file and attach it to a resource (e.g. an artifact), " +
			"keyed by owner_type and owner_id. Max 5 MB per file, 10 MB total per owner.",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *UploadAttachmentParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.uploadAttachment(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name: "vibexp_io_list_attachments",
		Description: "List the attachments (metadata plus a download URL) for a resource, " +
			"keyed by owner_type and owner_id.",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *ListAttachmentsParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.listAttachments(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_delete_attachment",
		Description: "Delete an attachment by its id. The owning resource is resolved and authorized before deletion.",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *DeleteAttachmentParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.deleteAttachment(ctx, req, params, userID)
	})
}

// addDeleteTools adds the single generic resource-deletion tool. One tool covers
// memory, artifact, blueprint, and prompt (keyed by resource_type) so the tool
// list stays compact instead of adding a delete tool per resource type.
func (m *MCPToolsManager) addDeleteTools(mcpServer *mcp.Server, userID string) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        deleteResourceToolName,
		Description: deleteResourceToolDescription,
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *DeleteResourceParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.deleteResource(ctx, req, params, userID)
	})
}

// addMemoryTools adds memory write tools (create/update). Reads go through the
// generic get_resource / list_resources tools (addReadTools).
func (m *MCPToolsManager) addMemoryTools(mcpServer *mcp.Server, userID string) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_create_memory",
		Description: "Create new memory with text content and metadata",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params *StoreMemoryParams) (*mcp.CallToolResult, any, error) {
		return m.server.storeMemory(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_update_memory",
		Description: "Update specific memory",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params *UpdateMemoryParams) (*mcp.CallToolResult, any, error) {
		return m.server.updateMemory(ctx, req, params, userID)
	})
}

// addPromptTools adds prompt write tools (create/update). Read access to prompts
// is provided separately via MCP prompt primitives (addUserPromptsToMCP).
func (m *MCPToolsManager) addPromptTools(mcpServer *mcp.Server, userID string) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_create_prompt",
		Description: "Create a new prompt",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *CreatePromptParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.createPrompt(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_update_prompt",
		Description: "Update specific prompt",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *UpdatePromptParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.updatePrompt(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name: "vibexp_io_render_prompt",
		Description: "Render a published, MCP-exposed prompt by slug, substituting values for its " +
			"{{placeholders}}. Use this to run any of your team's prompts as a tool — including prompts " +
			"not exposed as a slash-command primitive (only the most recent are). Returns the rendered body.",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *RenderPromptParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.renderPrompt(ctx, req, params, userID)
	})
}

// addBlueprintTools adds blueprint write tools (create/update). Blueprints are
// located by project_id + slug, unlike prompts which use the team-scoped slug.
func (m *MCPToolsManager) addBlueprintTools(mcpServer *mcp.Server, userID string) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_create_blueprint",
		Description: "Create a new blueprint",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *CreateBlueprintParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.createBlueprint(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_update_blueprint",
		Description: "Update specific blueprint",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *UpdateBlueprintParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.updateBlueprint(ctx, req, params, userID)
	})
}

// addProjectTools adds project listing tools.
func (m *MCPToolsManager) addProjectTools(mcpServer *mcp.Server, userID string) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_list_projects",
		Description: "List projects with optional search and pagination",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *ListProjectsParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.listProjects(ctx, req, params, userID)
	})
}

// getUserFromContext extracts the authenticated user ID from the request context.
func getUserFromContext(req *http.Request) (userID string, ok bool) {
	userIDValue := req.Context().Value(contextkeys.UserID)
	if userIDValue == nil {
		slog.Warn("No user ID found in context for MCP handler")
		return "", false
	}

	userID, ok = userIDValue.(string)
	if !ok {
		slog.Warn("User ID in context is not a string")
		return "", false
	}

	return userID, true
}

// setupMCPServerCommon configures the MCP server with all tools and the user's
// prompts. Identity comes from the flexible-auth middleware; team scoping is per
// call via the required team_id parameter on team-scoped tools.
func (s *Server) setupMCPServerCommon(mcpServer *mcp.Server, toolsManager *MCPToolsManager, req *http.Request) {
	userID, ok := getUserFromContext(req)
	if !ok {
		// This should never happen since flexibleAuthMiddleware validates identity.
		slog.Warn("Missing user ID in MCP handler despite auth middleware")
		return
	}

	toolsManager.AddAllTools(mcpServer, userID)
	s.addUserPromptsToMCP(req.Context(), mcpServer, userID)
}

// createMCPHandlerCommon creates the user-scoped MCP handler served at
// /mcp/v1/common. There is no team in the URL; team-scoped tools take a
// required team_id parameter validated per call.
func (s *Server) createMCPHandlerCommon() http.Handler {
	toolsManager := NewMCPToolsManager(s)

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		mcpServer := mcp.NewServer(&mcp.Implementation{
			Name:    "vibexp-mcp-server",
			Version: "1.0.0",
		}, &mcp.ServerOptions{
			HasPrompts:   true,
			PageSize:     100,
			Instructions: mcpServerInstructions,
		})

		s.setupMCPServerCommon(mcpServer, toolsManager, req)
		return mcpServer
	}, nil)

	return handler
}
