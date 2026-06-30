package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

// newSearchServer wires mock search + team services with the member user
// belonging to testTeamUUID/testTeamSlug so resolveTeam succeeds.
func newSearchServer(t *testing.T) (*Server, *mocks.MockSearchServiceInterface) {
	t.Helper()
	srv := newServerWithNullLogger(t)
	srv.config.Frontend.BaseURL = "https://app.vibexp.io"
	mockSearchSvc := mocks.NewMockSearchServiceInterface(t)
	mockTeamService := mocks.NewMockTeamServiceInterface(t)
	srv.container = &TestContainer{
		SearchServiceMock: mockSearchSvc,
		TeamServiceMock:   mockTeamService,
	}
	stubUserTeams(mockTeamService, []models.Team{memberTeam()})
	return srv, mockSearchSvc
}

func TestSearch_HappyPath(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	srv, mockSearchSvc := newSearchServer(t)

	expected := &models.SearchResultsResponse{
		Results: []models.SearchResultItem{
			{
				Type: "prompt", ID: "prompt-uuid-1", Title: "Staging DB setup",
				Excerpt: "How we configured the staging database.", Score: 0.92, ChunkID: "chunk-1", UpdatedAt: now,
			},
		},
		TotalCount: 1, Page: 2, PerPage: 25, TotalPages: 1,
	}
	mockSearchSvc.On(
		"Search", mock.Anything, testTeamUUID,
		mock.MatchedBy(func(req *models.SearchRequest) bool {
			return req.Query == "staging database config" &&
				len(req.Types) == 1 && req.Types[0] == "prompts" && req.Page == 2 && req.PerPage == 25
		}),
	).Return(expected, nil)

	params := &SemanticSearchParams{
		TeamID: testTeamSlug, Query: "staging database config",
		Types: []string{"prompts"}, Page: 2, Limit: 25,
	}
	result, structured, err := srv.search(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	resp, ok := structured.(*models.SearchResultsResponse)
	require.True(t, ok)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "prompt-uuid-1", resp.Results[0].ID)

	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	var jsonResp models.SearchResultsResponse
	require.NoError(t, json.Unmarshal([]byte(textContent.Text), &jsonResp))
	assert.Equal(t, "Staging DB setup", jsonResp.Results[0].Title)
}

func TestSearch_NonMemberTeamDenied(t *testing.T) {
	srv, mockSearchSvc := newSearchServer(t)

	params := &SemanticSearchParams{TeamID: testOtherTeamUUID, Query: "q"}
	result, structured, err := srv.search(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	assert.Nil(t, structured)
	assertGenericAccessDenied(t, result)
	mockSearchSvc.AssertNotCalled(t, "Search")
}

func TestSearch_MissingTeamID(t *testing.T) {
	srv, mockSearchSvc := newSearchServer(t)

	params := &SemanticSearchParams{Query: "q"}
	result, _, err := srv.search(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	text := extractText(t, result)
	assert.Contains(t, text, "team_id is required")
	assert.Contains(t, text, "vibexp_io_list_teams")
	mockSearchSvc.AssertNotCalled(t, "Search")
}

func TestSearch_EmptyQueryReturnsIsError(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
	}{
		{name: "empty string", query: ""},
		{name: "whitespace only", query: "   "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, mockSearchSvc := newSearchServer(t)

			params := &SemanticSearchParams{TeamID: testTeamUUID, Query: tc.query}
			result, structured, err := srv.search(context.Background(), nil, params, testMemberUserID)

			assert.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsError)
			assert.Nil(t, structured)
			mockSearchSvc.AssertNotCalled(t, "Search")
		})
	}
}

func TestSearch_RejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params *SemanticSearchParams
	}{
		{
			name:   "query exceeding max length",
			params: &SemanticSearchParams{TeamID: testTeamUUID, Query: strings.Repeat("a", 1001)},
		},
		{
			name:   "invalid types value",
			params: &SemanticSearchParams{TeamID: testTeamUUID, Query: "valid query", Types: []string{"foobar"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, mockSearchSvc := newSearchServer(t)

			result, structured, err := srv.search(context.Background(), nil, tc.params, testMemberUserID)

			assert.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsError)
			assert.Nil(t, structured)
			mockSearchSvc.AssertNotCalled(t, "Search")
		})
	}
}

func TestSearch_PaginationClamping(t *testing.T) {
	for _, tc := range []struct {
		name        string
		inPage      int
		inLimit     int
		wantPage    int
		wantPerPage int
	}{
		{name: "zero defaults", inPage: 0, inLimit: 0, wantPage: 1, wantPerPage: 10},
		{name: "negative defaults", inPage: -5, inLimit: -3, wantPage: 1, wantPerPage: 10},
		{name: "limit above max defaults", inPage: 1, inLimit: 9999, wantPage: 1, wantPerPage: 10},
		{name: "page above max defaults to 1", inPage: 100000, inLimit: 50, wantPage: 1, wantPerPage: 50},
		{name: "within bounds preserved", inPage: 3, inLimit: 50, wantPage: 3, wantPerPage: 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, mockSearchSvc := newSearchServer(t)

			wantPage, wantPerPage := tc.wantPage, tc.wantPerPage
			mockSearchSvc.On(
				"Search", mock.Anything, testTeamUUID,
				mock.MatchedBy(func(req *models.SearchRequest) bool {
					return req.Page == wantPage && req.PerPage == wantPerPage
				}),
			).Return(&models.SearchResultsResponse{Results: []models.SearchResultItem{}}, nil)

			params := &SemanticSearchParams{TeamID: testTeamUUID, Query: "q", Page: tc.inPage, Limit: tc.inLimit}
			result, _, err := srv.search(context.Background(), nil, params, testMemberUserID)
			assert.NoError(t, err)
			assert.False(t, result.IsError)
		})
	}
}

func TestSearch_ServiceErrorReturnsIsError(t *testing.T) {
	srv, mockSearchSvc := newSearchServer(t)

	mockSearchSvc.On("Search", mock.Anything, testTeamUUID, mock.Anything).
		Return(nil, errors.New("embedding service unavailable"))

	params := &SemanticSearchParams{TeamID: testTeamUUID, Query: "q"}
	result, structured, err := srv.search(context.Background(), nil, params, testMemberUserID)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Nil(t, structured)
}

func TestSearch_TrimsQueryWhitespace(t *testing.T) {
	srv, mockSearchSvc := newSearchServer(t)

	mockSearchSvc.On(
		"Search", mock.Anything, testTeamUUID,
		mock.MatchedBy(func(req *models.SearchRequest) bool { return req.Query == "trimmed query" }),
	).Return(&models.SearchResultsResponse{Results: []models.SearchResultItem{}}, nil)

	params := &SemanticSearchParams{TeamID: testTeamUUID, Query: "  trimmed query  "}
	result, _, err := srv.search(context.Background(), nil, params, testMemberUserID)
	assert.NoError(t, err)
	assert.False(t, result.IsError)
}

func TestAddSearchTools_RegistersSearchToolWithSchema(t *testing.T) {
	srv := newServerWithNullLogger(t)

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	NewMCPToolsManager(srv).addSearchTools(mcpServer, testMemberUserID)

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

	var searchTool *mcp.Tool
	for _, tool := range listResult.Tools {
		if tool.Name == "vibexp_io_search" {
			searchTool = tool
			break
		}
	}
	require.NotNil(t, searchTool, "addSearchTools should register vibexp_io_search")
	assert.Contains(t, searchTool.Description, "Semantic")
	require.NotNil(t, searchTool.InputSchema)

	schemaJSON, err := json.Marshal(searchTool.InputSchema)
	require.NoError(t, err)
	assert.Contains(t, string(schemaJSON), "query")
	assert.Contains(t, string(schemaJSON), "team_id")
}
