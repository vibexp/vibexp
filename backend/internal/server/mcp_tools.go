package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/contextkeys"
)

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
	m.addAttachmentTools(mcpServer, userID)
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

// addArtifactTools adds artifact management tools.
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
		Name:        "vibexp_io_search_artifacts",
		Description: "Search and list artifacts with filtering and pagination",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *ListArtifactsByProjectParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.listArtifactsByProject(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_get_artifact",
		Description: "Get specific artifact content by project and slug",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params *GetArtifactParams) (*mcp.CallToolResult, any, error) {
		return m.server.getArtifact(ctx, req, params, userID)
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

// addMemoryTools adds memory management tools.
func (m *MCPToolsManager) addMemoryTools(mcpServer *mcp.Server, userID string) {
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_create_memory",
		Description: "Create new memory with text content and metadata",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params *StoreMemoryParams) (*mcp.CallToolResult, any, error) {
		return m.server.storeMemory(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_search_memories",
		Description: "Search and list memories with filtering and pagination",
	}, func(
		ctx context.Context, req *mcp.CallToolRequest, params *ListMemoriesByProjectParams,
	) (*mcp.CallToolResult, any, error) {
		return m.server.listMemoriesByProject(ctx, req, params, userID)
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "vibexp_io_get_memory",
		Description: "Get specific memory by ID",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params *GetMemoryParams) (*mcp.CallToolResult, any, error) {
		return m.server.getMemory(ctx, req, params, userID)
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
			HasPrompts: true,
			PageSize:   100,
		})

		s.setupMCPServerCommon(mcpServer, toolsManager, req)
		return mcpServer
	}, nil)

	return handler
}
