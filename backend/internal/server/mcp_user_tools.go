package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/models"
)

// GetUserParams defines the parameters for the get user tool. This tool
// takes no input parameters; the user is identified from the MCP session.
type GetUserParams struct{}

// getUserWithUser implements the vibexp_io_get_user MCP tool. It returns a
// safe subset of the authenticated user's profile (see models.UserBasicInfo).
// Sensitive fields (GoogleID, IDPProvider, IDPSubject, StripeCustomerID,
// SubscriptionCanceledAt, Version) are never exposed.
func (s *Server) getUserWithUser(
	ctx context.Context, _ *mcp.CallToolRequest, _ *GetUserParams, userID string,
) (*mcp.CallToolResult, any, error) {
	user, err := s.container.AuthService().GetUserByID(ctx, userID)
	if err != nil {
		slog.With(
			"tool", "vibexp_io_get_user",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get user via MCP")
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to get user: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	info := models.NewUserBasicInfo(user)
	jsonData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal response to JSON: %w", err)
	}

	if s.metrics != nil {
		s.metrics.RecordMCPGetUser(context.Background())
	}

	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(jsonData)}},
		StructuredContent: info,
	}, info, nil
}
