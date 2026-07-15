package server

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// MCP tools call the same PromptService as the REST handlers, so they inherit the
// RBAC gates from #236 for free — the acceptance criteria ask for proof of that
// rather than a separate MCP-side check.
//
// The MCP layer has no HTTP status codes: a denial surfaces as an IsError result
// carrying the message. What matters is that a denied delete does not silently
// report success and does not reach the embedding cleanup.

// TestDeleteResource_PromptPermissionDenied is the per-role MCP proof: a plain
// member deleting another member's prompt is denied by PromptService, and the
// tool reports it as an error result.
func TestDeleteResource_PromptPermissionDenied(t *testing.T) {
	srv, m := newDeleteResourceTestServer(t)
	m.prompt.On("GetPromptBySlug", testMemberUserID, testTeamUUID, "victim").
		Return(&models.Prompt{ID: "prompt-1", UserID: "user-other"}, nil)
	m.prompt.On("DeletePromptBySlug", testMemberUserID, testTeamUUID, "victim").
		Return(services.ErrPermissionDenied)

	params := &DeleteResourceParams{TeamID: testTeamUUID, ResourceType: "prompt", Slug: "victim"}
	result, _, err := srv.deleteResource(context.Background(), nil, params, testMemberUserID)

	if err != nil {
		t.Fatalf("the tool call itself should succeed; the denial rides in the result: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("a denied delete must surface as an MCP error result, got %+v", result)
	}

	var text string
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text += tc.Text
		}
	}
	if !strings.Contains(strings.ToLower(text), "permission denied") {
		t.Errorf("denial message should reach the caller, got %q", text)
	}

	// The prompt was never deleted, so its embeddings must not be cleaned up.
	m.embedding.AssertNotCalled(t, "DeleteEmbeddingsByEntity", resourceTypePrompt, "prompt-1")
	m.prompt.AssertExpectations(t)
}
