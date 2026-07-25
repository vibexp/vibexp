package server

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
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

// validateMCPMetadataFilter checks a metadata filter that arrived already
// decoded as a native JSON-RPC object. It delegates to the same limits the REST
// layer enforces (repositories.ValidateMetadataFilter) — the only difference
// between the two transports is the decoding, so the rules must not fork.
//
// The violated limit is reported verbatim: an agent can correct "at most 10
// keys are allowed, got 12", but not "invalid request".
func validateMCPMetadataFilter(filter map[string][]string) *mcp.CallToolResult {
	if len(filter) == 0 {
		return nil
	}
	if err := repositories.ValidateMetadataFilter(repositories.MetadataFilter(filter)); err != nil {
		return mcpTextError(err.Error())
	}
	return nil
}

// isInvalidMetadataCatalogQuery reports whether an error is a rejected catalog
// query (the agent's fault, and actionable) rather than a backend failure.
func isInvalidMetadataCatalogQuery(err error) bool {
	return errors.Is(err, services.ErrInvalidMetadataCatalogQuery)
}
