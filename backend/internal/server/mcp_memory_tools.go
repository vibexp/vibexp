package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
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
	Text      string                 `json:"text" jsonschema:"Memory content/text"`
	Status    string                 `json:"status,omitempty" jsonschema:"Lifecycle status: active (default), draft, or archived"`
	Metadata  map[string]interface{} `json:"metadata,omitempty" jsonschema:"Additional key-value metadata pairs"`
}

// ListMemoriesByProjectParams defines the parameters for listing memories by project
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type ListMemoriesByProjectParams struct {
	TeamID    string `json:"team_id" jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	ProjectID string `json:"project_id" jsonschema:"Project UUID — required; scopes results to this project"`
	Page      int    `json:"page,omitempty" jsonschema:"Page number (default: 1)"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Items per page (default: 10, max: 10)"`
	Search    string `json:"search,omitempty" jsonschema:"Search in memory text"`
	Status    string `json:"status,omitempty" jsonschema:"Filter by lifecycle status: active, draft, or archived. Omit to hide archived"`
}

// GetMemoryParams defines the parameters for getting a specific memory
type GetMemoryParams struct {
	TeamID   string `json:"team_id" jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	MemoryID string `json:"memory_id" jsonschema:"Memory identifier"`
}

// UpdateMemoryParams defines the parameters for updating a specific memory
type UpdateMemoryParams struct {
	TeamID   string                 `json:"team_id" jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	MemoryID string                 `json:"memory_id" jsonschema:"Memory identifier"`
	Text     string                 `json:"text,omitempty" jsonschema:"New memory text"`
	Status   string                 `json:"status,omitempty" jsonschema:"New lifecycle status: active, draft, or archived"`
	Metadata map[string]interface{} `json:"metadata,omitempty" jsonschema:"New metadata"`
}

// Memory Tool Implementations

// storeMemory implements the tool that stores a new memory in the resolved team.
//
//nolint:funlen // structured slog attributes are marginally more verbose than the prior logrus WithFields calls
func (s *Server) storeMemory(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *StoreMemoryParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
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

	statusPtr, statusErr := validateMCPMemoryStatus(params.Status)
	if statusErr != nil {
		return statusErr, nil, nil
	}

	createReq := &models.CreateMemoryRequest{
		ProjectID: params.ProjectID,
		Text:      params.Text,
		Status:    statusPtr,
		Metadata:  params.Metadata,
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

// validateMCPMemoryStatus validates an optional MCP status argument. It returns
// the status as a pointer to thread into a request (nil when unset, so the
// service default applies), or a non-nil error result to hand back to the caller
// when the value falls outside the allowed enum.
func validateMCPMemoryStatus(status string) (*string, *mcp.CallToolResult) {
	if status == "" {
		return nil, nil
	}
	if !isAllowedMemoryStatus(status) {
		return nil, &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "status must be one of: active, draft, archived"},
			},
			IsError: true,
		}
	}
	s := status
	return &s, nil
}

// buildMemoryWriteResponse builds the slim write response for a memory by ID.
func buildMemoryWriteResponse(frontendBaseURL, memoryID string) *memoryWriteResponse {
	baseURL := strings.TrimRight(frontendBaseURL, "/")
	return &memoryWriteResponse{
		ID:      memoryID,
		FullURL: fmt.Sprintf("%s/memories/%s", baseURL, memoryID),
	}
}

// listMemoriesByProject implements the search_memories tool scoped to the resolved team.
//
//nolint:funlen // resolve team, normalize pagination, call service, truncate, marshal
func (s *Server) listMemoriesByProject(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *ListMemoriesByProjectParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	if r := validateProjectID(params.ProjectID); r != nil {
		slog.Warn(
			"MCP tool rejected: invalid project_id",
			"tool", "vibexp_io_search_memories",
			"user_id", userID,
			"team_id", teamID,
			"project_id", params.ProjectID,
		)
		return r, nil, nil
	}

	statusPtr, statusErr := validateMCPMemoryStatus(params.Status)
	if statusErr != nil {
		return statusErr, nil, nil
	}

	page := params.Page
	if page <= 0 {
		page = 1
	}
	limit := params.Limit
	if limit <= 0 || limit > 10 {
		limit = 10
	}

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
		slog.Error(
			"Failed to list memories via MCP",
			"tool", "vibexp_io_search_memories",
			"user_id", userID,
			"team_id", teamID,
			"project_id", params.ProjectID,
			"error", fmt.Sprintf("%+v", err),
		)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to search memories: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	if s.metrics != nil {
		s.metrics.RecordMCPListMemories(context.Background())
	}

	searchResp := &memorySearchResponse{
		Memories:   buildMemorySearchItems(response.Memories),
		TotalCount: response.TotalCount,
		Page:       response.Page,
		PerPage:    response.PerPage,
		TotalPages: response.TotalPages,
	}

	jsonData, marshalErr := json.MarshalIndent(searchResp, "", "  ")
	if marshalErr != nil {
		return nil, nil, fmt.Errorf("failed to marshal response to JSON: %w", marshalErr)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
		StructuredContent: searchResp,
	}, searchResp, nil
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

// getMemory implements the tool that gets a specific memory in the resolved team.
func (s *Server) getMemory(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *GetMemoryParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	memory, err := s.container.MemoryService().GetMemory(userID, teamID, params.MemoryID)
	if err != nil {
		slog.Error(
			"Failed to get memory via MCP",
			"tool", "vibexp_io_get_memory",
			"user_id", userID,
			"team_id", teamID,
			"memory_id", params.MemoryID,
			"error", fmt.Sprintf("%+v", err),
		)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to get memory: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	if s.metrics != nil {
		s.metrics.RecordMCPGetMemory(ctx)
	}

	jsonData, err := json.MarshalIndent(memory, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal response to JSON: %w", err)
	}

	// Record the successful detail read; MCP get-tools bypass the HTTP middleware.
	// Recorded only after a successful response, mirroring the HTTP 2xx contract.
	s.recordMCPResourceAccess(ctx, teamID, userID, resourceTypeMemory, memory.ID)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
		StructuredContent: memory,
	}, memory, nil
}

// updateMemory implements the tool that updates a specific memory in the resolved team.
func (s *Server) updateMemory(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *UpdateMemoryParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	statusPtr, statusErr := validateMCPMemoryStatus(params.Status)
	if statusErr != nil {
		return statusErr, nil, nil
	}

	updateReq := &models.UpdateMemoryRequest{Status: statusPtr}
	if params.Text != "" {
		updateReq.Text = &params.Text
	}
	if params.Metadata != nil {
		updateReq.Metadata = params.Metadata
	}

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
