package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/models"
)

// Status discovery (#1268). A team can add its own statuses to the system ones
// (epic #1263), and the `status` descriptions on the write tools are struct
// tags, which cannot carry a per-team value. This tool is how an agent learns
// which statuses a team actually has before it sets or filters by one.
const listStatusesToolName = "vibexp_io_list_statuses"

const listStatusesToolDescription = "List the statuses a team can use on its prompts, artifacts, blueprints " +
	"and memories: the system statuses every team has, then the team's own custom ones, each with `is_system` " +
	"and `is_live`. Call it before setting or filtering by `status`; omit `resource_type` to get all four types."

// statusListHint points an agent that sent an unknown status at the discovery
// tool. It is appended to the service's rejection, which already lists the
// allowed values.
const statusListHint = " (call " + listStatusesToolName + " to see the team's statuses)"

// ListStatusesParams defines the parameters for the status discovery tool.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type ListStatusesParams struct {
	TeamID       string `json:"team_id" jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	ResourceType string `json:"resource_type,omitempty" jsonschema:"Optional. Limit the result to one resource type: one of \"prompt\", \"artifact\", \"blueprint\", or \"memory\". Omit to list the statuses of all four."`
}

// mcpStatusItem is one status of a team's catalog as the discovery tool
// returns it. Slug is the value to send as `status`.
type mcpStatusItem struct {
	ResourceType string `json:"resource_type"`
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	IsSystem     bool   `json:"is_system"`
	IsLive       bool   `json:"is_live"`
}

// listStatusesResponse is the shape returned by the discovery tool.
type listStatusesResponse struct {
	Statuses []mcpStatusItem `json:"statuses"`
}

// mcpStatusResourceTypes resolves the optional resource_type parameter to the
// resource types to list: all of models.StatusResourceTypes when it is blank,
// the one named otherwise. ok is false for a value that has no status catalog.
func mcpStatusResourceTypes(raw string) (resourceTypes []string, ok bool) {
	resourceType := strings.ToLower(strings.TrimSpace(raw))
	if resourceType == "" {
		return models.StatusResourceTypes, true
	}
	if !models.IsStatusResourceType(resourceType) {
		return nil, false
	}
	return []string{resourceType}, true
}

// listStatuses implements the status discovery tool.
func (s *Server) listStatuses(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *ListStatusesParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	resourceTypes, ok := mcpStatusResourceTypes(params.ResourceType)
	if !ok {
		return mcpTextError(
			"resource_type must be one of: " + strings.Join(models.StatusResourceTypes, ", "),
		), nil, nil
	}

	// Non-nil, so a result with no rows still serializes as [] rather than null.
	statuses := make([]mcpStatusItem, 0)
	for _, resourceType := range resourceTypes {
		catalog, err := s.container.StatusCatalogService().List(ctx, teamID, resourceType)
		if err != nil {
			slog.With(
				"tool", listStatusesToolName,
				"resource_type", resourceType,
				"user_id", userID,
				"team_id", teamID,
				"error", fmt.Sprintf("%+v", err),
			).Error("MCP status catalog lookup failed")
			return mcpTextError("Failed to list statuses"), nil, nil
		}
		for i := range catalog {
			statuses = append(statuses, mcpStatusItem{
				ResourceType: resourceType,
				Slug:         catalog[i].Slug,
				Name:         catalog[i].Name,
				IsSystem:     catalog[i].IsSystem,
				IsLive:       catalog[i].IsLive,
			})
		}
	}

	return mcpJSONResult(&listStatusesResponse{Statuses: statuses})
}
