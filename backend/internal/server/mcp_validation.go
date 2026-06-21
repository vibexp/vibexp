package server

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// validateProjectID returns nil if id is a non-empty, well-formed UUID.
// Otherwise it returns a CallToolResult with IsError: true and a user-friendly
// message safe to surface to the MCP caller. The caller should return that
// result directly to the MCP layer (and log the rejection at Warn, not Error,
// because it is caller-input — not an operator-actionable failure).
func validateProjectID(id string) *mcp.CallToolResult {
	if id == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: "project_id is required and must be a project UUID. " +
					"Use vibexp_io_list_projects to retrieve the UUID.",
			}},
			IsError: true,
		}
	}
	if _, err := uuid.Parse(id); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: fmt.Sprintf(
					"project_id %q is not a valid UUID. "+
						"Use vibexp_io_list_projects to retrieve the UUID for the project.",
					id,
				),
			}},
			IsError: true,
		}
	}
	return nil
}
