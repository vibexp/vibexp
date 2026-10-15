package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/resourceaccess"
)

// Memory Tool Parameters

// memoryWriteResponse is the slim response returned by create/update memory tools.
type memoryWriteResponse struct {
	ID      string `json:"id"`
	FullURL string `json:"full_url"`
}

// memorySearchItem is the per-item shape returned by search_memories (text truncated to 300 chars).
type memorySearchItem struct {
	ID        string                 `json:"id"`
	UserID    string                 `json:"user_id"`
	TeamID    string                 `json:"team_id"`
	ProjectID string                 `json:"project_id"`
	Text      string                 `json:"text"`
	Truncated bool                   `json:"truncated,omitempty"`
	Metadata  map[string]interface{} `json:"metadata"`
	CreatedAt string                 `json:"created_at"`
	UpdatedAt string                 `json:"updated_at"`
}

// memorySearchResponse is the list shape returned by search_memories.
type memorySearchResponse struct {
	Memories   []memorySearchItem `json:"memories"`
	TotalCount int                `json:"total_count"`
	Page       int                `json:"page"`
	PerPage    int                `json:"per_page"`
	TotalPages int                `json:"total_pages"`
}

// StoreMemoryParams defines the parameters for storing a new memory
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type StoreMemoryParams struct {
	TeamID    string                 `json:"team_id" jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	ProjectID string                 `json:"project_id" jsonschema:"Project UUID — required; the project this memory belongs to"`
	Title     string                 `json:"title,omitempty" jsonschema:"Optional short title for the memory (max 255 characters)"`
	Text      string                 `json:"text" jsonschema:"Memory content/text"`
	Status    string                 `json:"status,omitempty" jsonschema:"Lifecycle status: one of \"active\" (default), \"draft\", \"archived\", or a custom status of the team (call vibexp_io_list_statuses)"`
	Metadata  map[string]interface{} `json:"metadata,omitempty" jsonschema:"Additional key-value metadata pairs"`
	Labels    []string               `json:"labels,omitempty" jsonschema:"Up to 10 labels (max 50 chars each)"`
}

// UpdateMemoryParams defines the parameters for updating a specific memory
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type UpdateMemoryParams struct {
	TeamID   string `json:"team_id" jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	MemoryID string `json:"memory_id" jsonschema:"Memory identifier"`
	// Clearing a title is REST-only; see buildMemoryUpdateRequest.
	Title    string                 `json:"title,omitempty" jsonschema:"New short title (max 255); omit to keep it"`
	Text     string                 `json:"text,omitempty" jsonschema:"New memory text"`
	Status   string                 `json:"status,omitempty" jsonschema:"New lifecycle status: one of \"active\", \"draft\", \"archived\", or a custom status of the team (call vibexp_io_list_statuses)"`
	Metadata map[string]interface{} `json:"metadata,omitempty" jsonschema:"New metadata"`
	Labels   []string               `json:"labels,omitempty" jsonschema:"Up to 10 labels (max 50 chars each)"`
}

// Memory Tool Implementations

// storeMemory implements the tool that stores a new memory in the resolved team.
//
//nolint:funlen // structured slog attributes are marginally more verbose than the prior logrus WithFields calls
func (s *Server) storeMemory(
	ctx context.Context,
	req *mcp.CallToolRequest,
	params *StoreMemoryParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	ctx = withMCPWriteOrigin(ctx, req)
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	if r := validateProjectID(params.ProjectID); r != nil {
		slog.Warn(
			"MCP tool rejected: invalid project_id",
			"tool", "vibexp_io_create_memory",
			"user_id", userID,
			"team_id", teamID,
			"project_id", params.ProjectID,
		)
		return r, nil, nil
	}

	// "" is "not supplied"; whether a value is one the team has is the
	// service's question (#1266).
	statusPtr := optionalString(params.Status)

	createReq := &models.CreateMemoryRequest{
		ProjectID: params.ProjectID,
		// "" is how the MCP wire format spells "argument omitted": there is no
		// null for a scalar, so an absent title must not become an empty one.
		Title:    optionalString(params.Title),
		Text:     params.Text,
		Status:   statusPtr,
		Metadata: params.Metadata,
		Labels:   params.Labels,
	}

	memory, err := s.container.MemoryService().CreateMemory(userID, teamID, createReq)
	if err != nil {
		slog.Error(
			"Failed to create memory via MCP",
			"tool", "vibexp_io_create_memory",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to create memory: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	s.recordResourceWrite(ctx, userID, models.WriteActionCreate, writtenMemory(teamID, memory))
	s.recordAuditWrite(ctx, auditCreate(teamID, resourceTypeMemory, memory.ID, memoryAuditState(memory)))

	if s.metrics != nil {
		s.metrics.RecordMCPCreateMemory(ctx)
	}

	result := buildMemoryWriteResponse(s.config.Frontend.BaseURL, memory.ID)
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

// buildMemoryWriteResponse builds the slim write response for a memory by ID.
func buildMemoryWriteResponse(frontendBaseURL, memoryID string) *memoryWriteResponse {
	baseURL := strings.TrimRight(frontendBaseURL, "/")
	return &memoryWriteResponse{
		ID:      memoryID,
		FullURL: fmt.Sprintf("%s/memories/%s", baseURL, memoryID),
	}
}

// buildMemorySearchItems converts service memories to the slim search-item shape with truncated text.
func buildMemorySearchItems(memories []models.Memory) []memorySearchItem {
	items := make([]memorySearchItem, 0, len(memories))
	for _, m := range memories {
		truncated, wasTruncated := truncateString(m.Text, excerptMaxLen)
		items = append(items, memorySearchItem{
			ID:        m.ID,
			UserID:    m.UserID,
			TeamID:    m.TeamID,
			ProjectID: m.ProjectID,
			Text:      truncated,
			Truncated: wasTruncated,
			Metadata:  m.Metadata,
			CreatedAt: m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			UpdatedAt: m.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return items
}

// buildMemoryUpdateRequest builds an UpdateMemoryRequest from the non-empty
// params fields. Empty string fields are left unset, so a field cannot be
// cleared to "" via update (consistent with the other MCP write tools); labels
// and metadata ARE clearable, because an explicitly supplied empty list is a
// meaningful edit. `title` follows the string rule rather than the list one:
// the MCP argument is a plain string, so an omitted title is indistinguishable
// from "" and both mean "leave it alone". Clearing a title is a REST-only
// operation (`{"title": null}`).
func buildMemoryUpdateRequest(params *UpdateMemoryParams, statusPtr *string) *models.UpdateMemoryRequest {
	updateReq := &models.UpdateMemoryRequest{Status: statusPtr}
	// TrimSpace, not just != "": normalizeMemoryTitle collapses a whitespace-only
	// title to nil downstream, so passing "   " through here would CLEAR the
	// title -- the one thing this transport is documented not to be able to do.
	if strings.TrimSpace(params.Title) != "" {
		updateReq.Title = models.NewOptionalString(params.Title)
	}
	if params.Text != "" {
		updateReq.Text = &params.Text
	}
	if params.Metadata != nil {
		updateReq.Metadata = params.Metadata
	}
	if params.Labels != nil {
		updateReq.Labels = params.Labels
	}
	return updateReq
}

// updateMemory implements the tool that updates a specific memory in the resolved team.
func (s *Server) updateMemory(
	ctx context.Context,
	req *mcp.CallToolRequest,
	params *UpdateMemoryParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	ctx = withMCPWriteOrigin(ctx, req)
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	// "" is "not supplied"; whether a value is one the team has is the
	// service's question (#1266).
	statusPtr := optionalString(params.Status)

	updateReq := buildMemoryUpdateRequest(params, statusPtr)
	before := s.memoryAuditBefore(userID, teamID, params.MemoryID)
	updateReq.WriteOrigin = resourceaccess.WriteOriginFromContext(ctx)

	memory, err := s.container.MemoryService().UpdateMemory(userID, teamID, params.MemoryID, updateReq)
	if err != nil {
		slog.Error(
			"Failed to update memory via MCP",
			"tool", "vibexp_io_update_memory",
			"user_id", userID,
			"team_id", teamID,
			"memory_id", params.MemoryID,
			"error", fmt.Sprintf("%+v", err),
		)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to update memory: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	s.recordResourceWrite(ctx, userID, models.WriteActionUpdate, writtenMemory(teamID, memory))
	s.recordAuditWrite(ctx, auditUpdated(teamID, resourceTypeMemory, memory.ID, before, memoryAuditState(memory)))

	if s.metrics != nil {
		s.metrics.RecordMCPUpdateMemory(ctx)
	}

	result := buildMemoryWriteResponse(s.config.Frontend.BaseURL, memory.ID)
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
