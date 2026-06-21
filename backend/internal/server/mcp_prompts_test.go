package server

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

func TestUniquePromptName(t *testing.T) {
	registered := map[string]struct{}{}

	// First occurrence keeps the bare slug.
	assert.Equal(t, "deploy", uniquePromptName("deploy", "team-a", registered))
	registered["deploy"] = struct{}{}

	// Collision across teams is disambiguated by team slug.
	assert.Equal(t, "deploy__team-b", uniquePromptName("deploy", "team-b", registered))
}

// TestUniquePromptName_ThreeWayCollision verifies that when both the bare slug AND
// its team-disambiguated form are already registered (a 3-way or adversarial
// collision, e.g. another team's slug literally equals "deploy__team-b"), a numeric
// suffix is appended so the result is still unique and no prompt is silently
// overwritten in the SDK's prompt map.
func TestUniquePromptName_ThreeWayCollision(t *testing.T) {
	registered := map[string]struct{}{
		"deploy":         {}, // team-a already took the bare slug
		"deploy__team-b": {}, // a third team's slug literally collides with the disambiguated form
	}

	// team-b cannot use the bare slug or the plain disambiguated form; it must get a suffix.
	got := uniquePromptName("deploy", "team-b", registered)
	assert.Equal(t, "deploy__team-b__2", got)

	// Registering that name and asking again pushes to the next free suffix.
	registered[got] = struct{}{}
	assert.Equal(t, "deploy__team-b__3", uniquePromptName("deploy", "team-b", registered))
}

// TestAddUserPromptsToMCP_AcrossTeamsDedupesSlugCollision verifies that prompts
// from every team the user belongs to are registered, and that a slug shared by
// two teams is disambiguated so both prompts are registered (no overwrite, no leak).
func TestAddUserPromptsToMCP_AcrossTeamsDedupesSlugCollision(t *testing.T) {
	srv := newServerWithNullLogger(t)

	mockTeam := mocks.NewMockTeamServiceInterface(t)
	mockPrompt := mocks.NewMockPromptServiceInterface(t)
	srv.container = &TestContainer{TeamServiceMock: mockTeam, PromptServiceMock: mockPrompt}

	teams := []models.Team{
		{ID: testTeamUUID, Name: "Acme", Slug: testTeamSlug},
		{ID: testOtherTeamUUID, Name: "Other", Slug: testOtherTeamSlug},
	}
	stubUserTeams(mockTeam, teams)

	// Both teams expose a prompt with the same slug "deploy".
	mockPrompt.On("ListPrompts", testMemberUserID,
		mock.MatchedBy(func(f services.PromptFilters) bool { return f.TeamID == testTeamUUID }),
	).Return(&models.PromptListResponse{
		Prompts: []models.Prompt{{Slug: "deploy", Description: "team A deploy", Body: "hello"}},
	}, nil)
	mockPrompt.On("ListPrompts", testMemberUserID,
		mock.MatchedBy(func(f services.PromptFilters) bool { return f.TeamID == testOtherTeamUUID }),
	).Return(&models.PromptListResponse{
		Prompts: []models.Prompt{{Slug: "deploy", Description: "team B deploy", Body: "world"}},
	}, nil)

	// Body has no placeholders, so ExtractAllPlaceholders returns an empty set.
	mockPrompt.On("ExtractAllPlaceholders", testMemberUserID, mock.Anything, mock.Anything).
		Return([]string{}, nil)

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, &mcp.ServerOptions{HasPrompts: true})
	srv.addUserPromptsToMCP(context.Background(), mcpServer, testMemberUserID)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpServer.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := serverSession.Close(); closeErr != nil {
			t.Logf("serverSession.Close: %v", closeErr)
		}
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := clientSession.Close(); closeErr != nil {
			t.Logf("clientSession.Close: %v", closeErr)
		}
	})

	listResult, err := clientSession.ListPrompts(ctx, nil)
	require.NoError(t, err)

	names := make(map[string]struct{}, len(listResult.Prompts))
	for _, p := range listResult.Prompts {
		names[p.Name] = struct{}{}
	}
	// First team's prompt keeps the bare slug; second team's colliding prompt is
	// disambiguated by its slug — both are present, neither overwrites the other.
	_, hasBare := names["deploy"]
	_, hasDisambiguated := names["deploy__"+testOtherTeamSlug]
	assert.True(t, hasBare, "expected the first team's prompt under its bare slug; got %v", names)
	assert.True(t, hasDisambiguated, "expected the second team's colliding prompt disambiguated; got %v", names)
}

// TestHandleMCPPromptRequestWithTeam_RendersWithCapturedTeam verifies the prompt
// render closure renders against the team_id captured at registration time.
func TestHandleMCPPromptRequestWithTeam_RendersWithCapturedTeam(t *testing.T) {
	srv := newServerWithNullLogger(t)
	mockPrompt := mocks.NewMockPromptServiceInterface(t)
	srv.container = &TestContainer{PromptServiceMock: mockPrompt}

	promptData := models.Prompt{Slug: "deploy", Description: "deploy prompt"}
	mockPrompt.On("RenderPrompt", testMemberUserID, testTeamUUID, "deploy", map[string]string{"env": "prod"}).
		Return(&models.RenderPromptResponse{RenderedBody: "deploy to prod"}, nil)

	req := &mcp.GetPromptRequest{Params: &mcp.GetPromptParams{Arguments: map[string]string{"env": "prod"}}}
	result, err := srv.handleMCPPromptRequestWithTeam(
		context.Background(), req, promptData, testMemberUserID, testTeamUUID,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Messages, 1)
	textContent, ok := result.Messages[0].Content.(*mcp.TextContent)
	require.True(t, ok)
	assert.Equal(t, "deploy to prod", textContent.Text)
}

// TestAddUserPromptsToMCP_TeamListErrorIsNonFatal verifies that a failure to list
// the user's teams does not panic and simply registers no prompts.
func TestAddUserPromptsToMCP_TeamListErrorIsNonFatal(t *testing.T) {
	srv := newServerWithNullLogger(t)
	mockTeam := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{TeamServiceMock: mockTeam}
	mockTeam.On("ListTeams", mock.Anything, testMemberUserID, mock.AnythingOfType("int"), mock.AnythingOfType("int")).
		Return(nil, assert.AnError)

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, &mcp.ServerOptions{HasPrompts: true})
	assert.NotPanics(t, func() {
		srv.addUserPromptsToMCP(context.Background(), mcpServer, testMemberUserID)
	})
}
