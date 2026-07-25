package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/repositories"
)

// Metadata discovery closes the loop opened by the metadata filter on
// list_resources: without it an agent is in the same guessing position the UI
// used to be in — it must already know a key exists to filter on it.
//
// One tool rather than two, with `key` as the mode discriminator, follows the
// same collapse-behind-a-discriminator principle as get_resource /
// list_resources: the two reads are homogeneous, so a second tool would grow
// the tool list without adding expressiveness.
const listResourceMetadataToolName = "vibexp_io_list_resource_metadata"

const listResourceMetadataToolDescription = "Discover the metadata a team actually uses, so a metadata filter " +
	"can be built from real keys and values instead of guesses. Omit `key` to list the distinct metadata keys " +
	"present on a resource type; supply `key` to list that key's distinct values, optionally narrowed by `q`. " +
	"Values held in an array are flattened and non-string scalars are rendered in their text form, so every " +
	"value returned can be used directly in the `metadata` filter of vibexp_io_list_resources. " +
	"`truncated` reports that more exist than were returned. The team is resolved and membership-checked per " +
	"call, and results never include another team's metadata."

// ListResourceMetadataParams defines the parameters for the metadata discovery
// tool. `key` selects the mode: absent lists keys, present lists that key's values.
//
//nolint:lll // struct tag values contain verbatim tool descriptions; cannot be shortened
type ListResourceMetadataParams struct {
	TeamID       string `json:"team_id" jsonschema:"REQUIRED. Team UUID or slug to operate within."`
	ResourceType string `json:"resource_type" jsonschema:"REQUIRED. The resource type whose metadata to enumerate: one of \"memory\", \"artifact\", or \"blueprint\"."`
	Key          string `json:"key,omitempty" jsonschema:"Omit to list the distinct metadata KEYS. Supply a key to list that key's distinct VALUES."`
	ProjectID    string `json:"project_id,omitempty" jsonschema:"Optional project UUID; narrows the catalog to a single project."`
	Q            string `json:"q,omitempty" jsonschema:"Case-insensitive substring filter, applied when listing values."`
	Limit        int    `json:"limit,omitempty" jsonschema:"Maximum entries to return (default: 100, max: 500)."`
}

// listResourceMetadataResponse is the shape returned by the discovery tool.
// `keys` and `values` are mutually exclusive — whichever mode ran is populated,
// the other is omitted — so an agent can tell which question it asked.
type listResourceMetadataResponse struct {
	ResourceType string `json:"resource_type"`
	Key          string `json:"key,omitempty"`
	// Pointers, not plain slices: `omitempty` on a slice would drop an EMPTY
	// result entirely, which would both hide which mode ran and force the agent
	// to distinguish "no keys" from "not a keys call". A nil pointer omits the
	// field; a pointer to an empty slice serializes as [].
	Keys      *[]string `json:"keys,omitempty"`
	Values    *[]string `json:"values,omitempty"`
	Truncated bool      `json:"truncated"`
}

// mcpMetadataResourceType maps the MCP tools' singular resource_type vocabulary
// onto the repository's plural one. The singular form is what every other
// resource_type-keyed tool uses, so the discovery tool matches its siblings
// rather than the REST enum.
func mcpMetadataResourceType(raw string) (repositories.MetadataResourceType, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case resourceTypeMemory:
		return repositories.MetadataResourceMemories, true
	case resourceTypeArtifact:
		return repositories.MetadataResourceArtifacts, true
	case resourceTypeBlueprint:
		return repositories.MetadataResourceBlueprints, true
	default:
		return "", false
	}
}

// listResourceMetadata implements the metadata discovery tool.
func (s *Server) listResourceMetadata(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	params *ListResourceMetadataParams,
	userID string,
) (*mcp.CallToolResult, any, error) {
	teamID, errResult := s.resolveTeam(ctx, userID, params.TeamID)
	if errResult != nil {
		return errResult, nil, nil
	}

	resourceType, ok := mcpMetadataResourceType(params.ResourceType)
	if !ok {
		return mcpTextError(fmt.Sprintf(
			"resource_type must be one of: %s, %s, %s",
			resourceTypeMemory, resourceTypeArtifact, resourceTypeBlueprint,
		)), nil, nil
	}

	query := repositories.MetadataCatalogQuery{
		UserID:       userID,
		TeamID:       teamID,
		ResourceType: resourceType,
		Key:          strings.TrimSpace(params.Key),
		// Deliberately NOT normalizeMCPListPagination: its max of 10 would make
		// key discovery useless. The repository applies the REST catalog cap.
		Limit: params.Limit,
	}
	if projectID := strings.TrimSpace(params.ProjectID); projectID != "" {
		query.ProjectID = &projectID
	}
	if q := strings.TrimSpace(params.Q); q != "" {
		query.Search = &q
	}

	if query.Key == "" {
		return s.listResourceMetadataKeys(ctx, query, userID, teamID)
	}
	return s.listResourceMetadataValues(ctx, query, userID, teamID)
}

func (s *Server) listResourceMetadataKeys(
	ctx context.Context, query repositories.MetadataCatalogQuery, userID, teamID string,
) (*mcp.CallToolResult, any, error) {
	result, err := s.container.MetadataCatalogService().Keys(ctx, query)
	if err != nil {
		return s.mcpMetadataCatalogError("keys", userID, teamID, err), nil, nil
	}

	keys := nonNilStrings(result.Entries)
	return mcpJSONResult(&listResourceMetadataResponse{
		ResourceType: string(query.ResourceType),
		Keys:         &keys,
		Truncated:    result.Truncated,
	})
}

func (s *Server) listResourceMetadataValues(
	ctx context.Context, query repositories.MetadataCatalogQuery, userID, teamID string,
) (*mcp.CallToolResult, any, error) {
	result, err := s.container.MetadataCatalogService().Values(ctx, query)
	if err != nil {
		return s.mcpMetadataCatalogError("values", userID, teamID, err), nil, nil
	}

	values := nonNilStrings(result.Entries)
	return mcpJSONResult(&listResourceMetadataResponse{
		ResourceType: string(query.ResourceType),
		Key:          query.Key,
		Values:       &values,
		Truncated:    result.Truncated,
	})
}

// mcpMetadataCatalogError reports a rejected query back to the agent in its own
// words — an agent cannot act on "internal error" — while anything else is
// logged and returned generically.
func (s *Server) mcpMetadataCatalogError(
	mode, userID, teamID string, err error,
) *mcp.CallToolResult {
	if isInvalidMetadataCatalogQuery(err) {
		return mcpTextError(err.Error())
	}
	slog.With(
		"tool", listResourceMetadataToolName,
		"mode", mode,
		"user_id", userID,
		"team_id", teamID,
		"error", err,
	).Error("MCP metadata catalog lookup failed")
	return mcpTextError("Failed to list resource metadata")
}

// nonNilStrings keeps an empty catalog serializing as [] rather than null, so
// an agent parsing the result never has to special-case a missing array.
func nonNilStrings(entries []string) []string {
	if entries == nil {
		return []string{}
	}
	return entries
}
