package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

func TestListTeamsForUser_ReturnsUUIDNameSlug(t *testing.T) {
	srv := newServerWithNullLogger(t)
	mockTeam := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{TeamServiceMock: mockTeam}

	teams := []models.Team{
		{ID: testTeamUUID, Name: "Acme Team", Slug: testTeamSlug},
		{ID: testOtherTeamUUID, Name: "Other Team", Slug: testOtherTeamSlug},
	}
	stubUserTeams(mockTeam, teams)

	result, structured, err := srv.listTeamsForUser(context.Background(), nil, &ListTeamsParams{}, testMemberUserID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	resp, ok := structured.(*listTeamsResponse)
	require.True(t, ok, "expected *listTeamsResponse")
	require.Len(t, resp.Teams, 2)
	assert.Equal(t, testTeamUUID, resp.Teams[0].UUID)
	assert.Equal(t, "Acme Team", resp.Teams[0].Name)
	assert.Equal(t, testTeamSlug, resp.Teams[0].Slug)
	assert.Equal(t, testOtherTeamUUID, resp.Teams[1].UUID)

	// JSON payload must carry uuid, name, slug keys.
	text := extractText(t, result)
	var parsed listTeamsResponse
	require.NoError(t, json.Unmarshal([]byte(text), &parsed))
	require.Len(t, parsed.Teams, 2)
	assert.Equal(t, testTeamSlug, parsed.Teams[0].Slug)
	assert.Contains(t, text, `"uuid"`)
	assert.Contains(t, text, `"name"`)
	assert.Contains(t, text, `"slug"`)
}

func TestListTeamsForUser_Empty(t *testing.T) {
	srv := newServerWithNullLogger(t)
	mockTeam := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{TeamServiceMock: mockTeam}
	stubUserTeams(mockTeam, []models.Team{})

	result, structured, err := srv.listTeamsForUser(context.Background(), nil, &ListTeamsParams{}, testMemberUserID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	resp, ok := structured.(*listTeamsResponse)
	require.True(t, ok)
	assert.Empty(t, resp.Teams)
}

func TestListTeamsForUser_ServiceError(t *testing.T) {
	srv := newServerWithNullLogger(t)
	mockTeam := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{TeamServiceMock: mockTeam}
	mockTeam.On("ListTeams", mock.Anything, testMemberUserID, mock.AnythingOfType("int"), mock.AnythingOfType("int")).
		Return(nil, errors.New("db error"))

	result, structured, err := srv.listTeamsForUser(context.Background(), nil, &ListTeamsParams{}, testMemberUserID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Nil(t, structured)
}

// TestAddAllTools_RegistersDiscoveryAndTeamScopedTools verifies the single common
// path registers list_teams, get_user, and the team-scoped tools.
func TestAddAllTools_RegistersDiscoveryAndTeamScopedTools(t *testing.T) {
	srv := newServerWithNullLogger(t)

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	NewMCPToolsManager(srv).AddAllTools(mcpServer, testMemberUserID)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpServer.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := serverSession.Close(); closeErr != nil {
			t.Logf("serverSession.Close: %v", closeErr)
		}
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := clientSession.Close(); closeErr != nil {
			t.Logf("clientSession.Close: %v", closeErr)
		}
	})

	listResult, err := clientSession.ListTools(ctx, nil)
	require.NoError(t, err)

	toolNames := make(map[string]struct{}, len(listResult.Tools))
	for _, tool := range listResult.Tools {
		toolNames[tool.Name] = struct{}{}
	}

	for _, want := range []string{
		"vibexp_io_list_teams",
		"vibexp_io_get_user",
		"vibexp_io_create_artifact",
		"vibexp_io_search_artifacts",
		"vibexp_io_create_memory",
		"vibexp_io_create_prompt",
		"vibexp_io_update_prompt",
		"vibexp_io_create_blueprint",
		"vibexp_io_update_blueprint",
		"vibexp_io_list_projects",
		"vibexp_io_list_feeds",
		"vibexp_io_search",
	} {
		_, ok := toolNames[want]
		assert.True(t, ok, "AddAllTools should register %s", want)
	}
}

// TestSearchToolsOmitFullDetails guards the #260 "description diet": the
// search_artifacts and search_memories tools no longer expose the dead
// full_details parameter (it was documented unsupported and only ever
// rejected). The functional feed full_details param is out of scope here.
func TestSearchToolsOmitFullDetails(t *testing.T) {
	srv := newServerWithNullLogger(t)

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	NewMCPToolsManager(srv).AddAllTools(mcpServer, testMemberUserID)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpServer.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := serverSession.Close(); closeErr != nil {
			t.Logf("serverSession.Close: %v", closeErr)
		}
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := clientSession.Close(); closeErr != nil {
			t.Logf("clientSession.Close: %v", closeErr)
		}
	})

	listResult, err := clientSession.ListTools(ctx, nil)
	require.NoError(t, err)

	schemas := make(map[string]string, len(listResult.Tools))
	for _, tool := range listResult.Tools {
		raw, marshalErr := json.Marshal(tool.InputSchema)
		require.NoError(t, marshalErr)
		schemas[tool.Name] = string(raw)
	}

	for _, name := range []string{"vibexp_io_search_artifacts", "vibexp_io_search_memories"} {
		schema, ok := schemas[name]
		require.True(t, ok, "%s should be registered", name)
		assert.NotContains(t, schema, "full_details", "%s must not expose the dead full_details param", name)
		assert.Contains(t, schema, "team_id", "%s should still declare team_id", name)
	}
}

// TestMCPServerAdvertisesInstructions guards the #260 relocation of the
// team-scoping guidance: it must reach clients via the server Instructions
// (returned in the initialize result). Removing the ServerOptions.Instructions
// wiring would silently drop the guidance from every client, so pin it here.
func TestMCPServerAdvertisesInstructions(t *testing.T) {
	mcpServer := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "1.0.0"},
		&mcp.ServerOptions{Instructions: mcpServerInstructions},
	)

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpServer.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := serverSession.Close(); closeErr != nil {
			t.Logf("serverSession.Close: %v", closeErr)
		}
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if closeErr := clientSession.Close(); closeErr != nil {
			t.Logf("clientSession.Close: %v", closeErr)
		}
	})

	initResult := clientSession.InitializeResult()
	require.NotNil(t, initResult)
	assert.NotEmpty(t, initResult.Instructions, "server must advertise instructions to clients")
	assert.Contains(t, initResult.Instructions, "vibexp_io_list_teams",
		"instructions should point clients to team discovery")
}
