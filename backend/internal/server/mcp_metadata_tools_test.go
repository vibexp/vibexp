package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

// mcpTextOf returns the single text content of a tool result.
func mcpTextOf(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.Len(t, result.Content, 1)
	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "expected a text content block")
	return textContent.Text
}

// decodeMetadataResult parses the tool's JSON payload.
func decodeMetadataResult(t *testing.T, result *mcp.CallToolResult) listResourceMetadataResponse {
	t.Helper()
	var decoded listResourceMetadataResponse
	require.NoError(t, json.Unmarshal([]byte(mcpTextOf(t, result)), &decoded))
	return decoded
}

// assertAnError is a stand-in backend failure whose text must never reach the agent.
func assertAnError() error {
	return errors.New("connection refused")
}

func newMetadataToolTestServer(
	t *testing.T,
) (*Server, *servicesmocks.MockMetadataCatalogServiceInterface) {
	t.Helper()
	srv := newServerWithNullLogger(t)
	catalog := servicesmocks.NewMockMetadataCatalogServiceInterface(t)
	srv.container = &TestContainer{
		MetadataCatalogMock: catalog,
		TeamRepositoryMock:  stubTeamResolution(t, []models.Team{memberTeam()}),
	}
	return srv, catalog
}

func TestListResourceMetadata_ListsKeys(t *testing.T) {
	srv, catalog := newMetadataToolTestServer(t)
	catalog.EXPECT().Keys(mock.Anything, repositories.MetadataCatalogQuery{
		UserID:       testMemberUserID,
		TeamID:       testTeamUUID,
		ResourceType: repositories.MetadataResourceBlueprints,
	}).Return(repositories.MetadataCatalogResult{
		Entries: []string{"env", "scope"}, Truncated: false,
	}, nil)

	result, _, err := srv.listResourceMetadata(context.Background(), nil,
		&ListResourceMetadataParams{TeamID: testTeamUUID, ResourceType: "blueprint"},
		testMemberUserID)

	require.NoError(t, err)
	require.False(t, result.IsError)
	decoded := decodeMetadataResult(t, result)
	require.NotNil(t, decoded.Keys)
	assert.Equal(t, []string{"env", "scope"}, *decoded.Keys)
	// Mode is discoverable from the payload: keys present, values absent.
	assert.Nil(t, decoded.Values)
	assert.Equal(t, "blueprints", decoded.ResourceType)
}

func TestListResourceMetadata_ListsValuesForAKey(t *testing.T) {
	srv, catalog := newMetadataToolTestServer(t)
	search := "pro"
	projectID := testProjectID
	catalog.EXPECT().Values(mock.Anything, repositories.MetadataCatalogQuery{
		UserID:       testMemberUserID,
		TeamID:       testTeamUUID,
		ResourceType: repositories.MetadataResourceMemories,
		Key:          "env",
		ProjectID:    &projectID,
		Search:       &search,
		Limit:        50,
	}).Return(repositories.MetadataCatalogResult{
		Entries: []string{"prod"}, Truncated: true,
	}, nil)

	result, _, err := srv.listResourceMetadata(context.Background(), nil,
		&ListResourceMetadataParams{
			TeamID: testTeamUUID, ResourceType: "memory",
			Key: "env", ProjectID: testProjectID, Q: "pro", Limit: 50,
		}, testMemberUserID)

	require.NoError(t, err)
	require.False(t, result.IsError)
	decoded := decodeMetadataResult(t, result)
	require.NotNil(t, decoded.Values)
	assert.Equal(t, []string{"prod"}, *decoded.Values)
	assert.Equal(t, "env", decoded.Key)
	assert.True(t, decoded.Truncated)
}

// TestListResourceMetadata_LimitIsNotCappedAtTen guards the decision not to run
// this tool through normalizeMCPListPagination: a max of 10 would make key
// discovery useless for any team with a real metadata vocabulary.
func TestListResourceMetadata_LimitIsNotCappedAtTen(t *testing.T) {
	srv, catalog := newMetadataToolTestServer(t)
	catalog.EXPECT().Keys(mock.Anything, mock.MatchedBy(
		func(q repositories.MetadataCatalogQuery) bool { return q.Limit == 200 },
	)).Return(repositories.MetadataCatalogResult{Entries: []string{}}, nil)

	_, _, err := srv.listResourceMetadata(context.Background(), nil,
		&ListResourceMetadataParams{
			TeamID: testTeamUUID, ResourceType: "artifact", Limit: 200,
		}, testMemberUserID)

	require.NoError(t, err)
}

func TestListResourceMetadata_RejectsUnknownResourceType(t *testing.T) {
	srv, _ := newMetadataToolTestServer(t)

	result, _, err := srv.listResourceMetadata(context.Background(), nil,
		&ListResourceMetadataParams{TeamID: testTeamUUID, ResourceType: "prompt"},
		testMemberUserID)

	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Contains(t, mcpTextOf(t, result), "resource_type must be one of")
}

func TestListResourceMetadata_AcceptsSingularTypesCaseInsensitively(t *testing.T) {
	for _, raw := range []string{"Blueprint", " blueprint ", "BLUEPRINT"} {
		t.Run(raw, func(t *testing.T) {
			srv, catalog := newMetadataToolTestServer(t)
			catalog.EXPECT().Keys(mock.Anything, mock.MatchedBy(
				func(q repositories.MetadataCatalogQuery) bool {
					return q.ResourceType == repositories.MetadataResourceBlueprints
				},
			)).Return(repositories.MetadataCatalogResult{Entries: []string{}}, nil)

			result, _, err := srv.listResourceMetadata(context.Background(), nil,
				&ListResourceMetadataParams{TeamID: testTeamUUID, ResourceType: raw},
				testMemberUserID)

			require.NoError(t, err)
			assert.False(t, result.IsError)
		})
	}
}

// TestListResourceMetadata_RejectedQueryTellsTheAgentWhy: an agent cannot act on
// "internal error", so a rejected query is reported in the service's own words.
func TestListResourceMetadata_RejectedQueryTellsTheAgentWhy(t *testing.T) {
	srv, catalog := newMetadataToolTestServer(t)
	catalog.EXPECT().Values(mock.Anything, mock.Anything).Return(
		repositories.MetadataCatalogResult{},
		services.ErrInvalidMetadataCatalogQuery,
	)

	result, _, err := srv.listResourceMetadata(context.Background(), nil,
		&ListResourceMetadataParams{
			TeamID: testTeamUUID, ResourceType: "memory", Key: "env",
		}, testMemberUserID)

	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Contains(t, mcpTextOf(t, result), "invalid metadata catalog query")
}

func TestListResourceMetadata_BackendFailureIsGeneric(t *testing.T) {
	srv, catalog := newMetadataToolTestServer(t)
	catalog.EXPECT().Keys(mock.Anything, mock.Anything).Return(
		repositories.MetadataCatalogResult{}, assertAnError(),
	)

	result, _, err := srv.listResourceMetadata(context.Background(), nil,
		&ListResourceMetadataParams{TeamID: testTeamUUID, ResourceType: "memory"},
		testMemberUserID)

	require.NoError(t, err)
	require.True(t, result.IsError)
	body := mcpTextOf(t, result)
	assert.Contains(t, body, "Failed to list resource metadata")
	// The underlying failure must not leak to the agent.
	assert.NotContains(t, body, "connection refused")
}

func TestListResourceMetadata_EmptyCatalogSerializesAsArray(t *testing.T) {
	srv, catalog := newMetadataToolTestServer(t)
	catalog.EXPECT().Keys(mock.Anything, mock.Anything).
		Return(repositories.MetadataCatalogResult{Entries: nil}, nil)

	result, _, err := srv.listResourceMetadata(context.Background(), nil,
		&ListResourceMetadataParams{TeamID: testTeamUUID, ResourceType: "memory"},
		testMemberUserID)

	require.NoError(t, err)
	// [] not null, so an agent never has to special-case a missing array.
	decoded := decodeMetadataResult(t, result)
	require.NotNil(t, decoded.Keys, "an empty catalog must still report the mode")
	assert.Equal(t, []string{}, *decoded.Keys)
}

// TestValidateMCPMetadataFilter covers the transport-specific half: MCP receives
// the filter already decoded, so only validation applies — and it must be the
// same validation the REST layer runs.
func TestValidateMCPMetadataFilter(t *testing.T) {
	tests := []struct {
		name    string
		filter  map[string][]string
		wantMsg string
	}{
		{name: "nil is allowed", filter: nil},
		{name: "empty is allowed", filter: map[string][]string{}},
		{name: "ordinary filter", filter: map[string][]string{"env": {"prod"}}},
		{name: "key-exists form", filter: map[string][]string{"env": {}}},
		{
			name:    "too many keys",
			filter:  manyMetadataKeys(11),
			wantMsg: "at most 10 keys",
		},
		{
			name:    "too many values",
			filter:  map[string][]string{"env": make([]string, 26)},
			wantMsg: "at most 25 are allowed",
		},
		{
			name:    "over-long key",
			filter:  map[string][]string{strings.Repeat("k", 256): {"v"}},
			wantMsg: "key length must be at most 255",
		},
		{
			name:    "over-long value",
			filter:  map[string][]string{"env": {strings.Repeat("v", 513)}},
			wantMsg: "at most 512 are allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateMCPMetadataFilter(tt.filter)

			if tt.wantMsg == "" {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			assert.True(t, result.IsError)
			// The agent needs the specific limit to correct its own call.
			assert.Contains(t, mcpTextOf(t, result), tt.wantMsg)
		})
	}
}

func manyMetadataKeys(n int) map[string][]string {
	filter := make(map[string][]string, n)
	for i := range n {
		filter["k"+string(rune('a'+i))] = []string{"v"}
	}
	return filter
}

// TestListResources_MetadataFilterReachesEachService is the passthrough guard:
// the filter crosses ListResourcesParams -> services.*Filters for three
// different resource types, and nothing else covers that hop. Dropping any one
// mapping would leave the whole suite green while the filter silently did
// nothing for that type.
func TestListResources_MetadataFilterReachesEachService(t *testing.T) {
	filter := map[string][]string{"env": {"prod", "staging"}, "scope": {"backend"}}

	t.Run("memory", func(t *testing.T) {
		srv := newServerWithNullLogger(t)
		memSvc := servicesmocks.NewMockMemoryServiceInterface(t)
		srv.container = &TestContainer{
			MemoryServiceMock:  memSvc,
			TeamRepositoryMock: stubTeamResolution(t, []models.Team{memberTeam()}),
		}

		memSvc.EXPECT().ListMemories(testMemberUserID, mock.MatchedBy(
			func(f services.MemoryFilters) bool {
				return len(f.MetadataFilter["env"]) == 2 && len(f.MetadataFilter["scope"]) == 1
			},
		)).Return(&models.MemoryListResponse{}, nil)

		_, _, err := srv.listResources(context.Background(), nil, &ListResourcesParams{
			TeamID: testTeamUUID, ResourceType: "memory",
			ProjectID: testProjectID, Metadata: filter,
		}, testMemberUserID)
		require.NoError(t, err)
	})

	t.Run("artifact", func(t *testing.T) {
		srv := newServerWithNullLogger(t)
		artSvc := servicesmocks.NewMockArtifactServiceInterface(t)
		srv.container = &TestContainer{
			ArtifactServiceMock: artSvc,
			TeamRepositoryMock:  stubTeamResolution(t, []models.Team{memberTeam()}),
		}

		artSvc.EXPECT().ListArtifactsByProject(testMemberUserID, testProjectID, mock.MatchedBy(
			func(f services.ArtifactFilters) bool {
				return len(f.MetadataFilter["env"]) == 2 && len(f.MetadataFilter["scope"]) == 1
			},
		)).Return(&models.ArtifactListResponse{}, nil)

		_, _, err := srv.listResources(context.Background(), nil, &ListResourcesParams{
			TeamID: testTeamUUID, ResourceType: "artifact",
			ProjectID: testProjectID, Metadata: filter,
		}, testMemberUserID)
		require.NoError(t, err)
	})

	t.Run("blueprint", func(t *testing.T) {
		srv := newServerWithNullLogger(t)
		bpSvc := servicesmocks.NewMockBlueprintServiceInterface(t)
		srv.container = &TestContainer{
			BlueprintServiceMock: bpSvc,
			TeamRepositoryMock:   stubTeamResolution(t, []models.Team{memberTeam()}),
		}

		bpSvc.EXPECT().ListBlueprintsByProject(testMemberUserID, testProjectID, mock.MatchedBy(
			func(f services.BlueprintFilters) bool {
				return len(f.MetadataFilter["env"]) == 2 && len(f.MetadataFilter["scope"]) == 1
			},
		)).Return(&models.BlueprintListResponse{}, nil)

		_, _, err := srv.listResources(context.Background(), nil, &ListResourcesParams{
			TeamID: testTeamUUID, ResourceType: "blueprint",
			ProjectID: testProjectID, Metadata: filter,
		}, testMemberUserID)
		require.NoError(t, err)
	})
}

// TestListResources_InvalidMetadataIsRejectedBeforeTheService: the mock has no
// EXPECT calls, so reaching the service at all fails the test.
func TestListResources_InvalidMetadataIsRejectedBeforeTheService(t *testing.T) {
	srv := newServerWithNullLogger(t)
	memSvc := servicesmocks.NewMockMemoryServiceInterface(t)
	srv.container = &TestContainer{
		MemoryServiceMock:  memSvc,
		TeamRepositoryMock: stubTeamResolution(t, []models.Team{memberTeam()}),
	}

	result, _, err := srv.listResources(context.Background(), nil, &ListResourcesParams{
		TeamID: testTeamUUID, ResourceType: "memory", ProjectID: testProjectID,
		Metadata: manyMetadataKeys(11),
	}, testMemberUserID)

	require.NoError(t, err)
	require.True(t, result.IsError)
	// The agent needs the specific limit, not "invalid request".
	assert.Contains(t, mcpTextOf(t, result), "at most 10 keys")
}
