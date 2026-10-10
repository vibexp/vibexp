package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	servicesmocks "github.com/vibexp/vibexp/internal/services/mocks"
)

func newStatusToolTestServer(
	t *testing.T,
) (*Server, *servicesmocks.MockStatusCatalogServiceInterface) {
	t.Helper()
	srv := newServerWithNullLogger(t)
	catalog := servicesmocks.NewMockStatusCatalogServiceInterface(t)
	srv.container = &TestContainer{
		StatusCatalogMock:  catalog,
		TeamRepositoryMock: stubTeamResolution(t, []models.Team{memberTeam()}),
	}
	return srv, catalog
}

// decodeStatusesResult parses the tool's JSON payload.
func decodeStatusesResult(t *testing.T, result *mcp.CallToolResult) listStatusesResponse {
	t.Helper()
	var decoded listStatusesResponse
	require.NoError(t, json.Unmarshal([]byte(mcpTextOf(t, result)), &decoded))
	return decoded
}

// systemCatalog is what StatusCatalogService.List answers for a team with no
// custom statuses of resourceType.
func systemCatalog(resourceType string) []services.CatalogStatus {
	system := models.SystemStatusesFor(resourceType)
	catalog := make([]services.CatalogStatus, 0, len(system))
	for _, slug := range system {
		catalog = append(catalog, services.CatalogStatus{
			Slug: slug, Name: slug, IsSystem: true,
			IsLive: models.IsLiveStatus(resourceType, slug),
		})
	}
	return catalog
}

// With no resource_type the tool walks every type that has a status catalog,
// in models.StatusResourceTypes order, and asks for the RESOLVED team only.
func TestListStatuses_AllResourceTypes(t *testing.T) {
	srv, catalog := newStatusToolTestServer(t)
	want := make([]mcpStatusItem, 0)
	for _, resourceType := range models.StatusResourceTypes {
		rows := systemCatalog(resourceType)
		catalog.EXPECT().List(mock.Anything, testTeamUUID, resourceType).Return(rows, nil).Once()
		for _, row := range rows {
			want = append(want, mcpStatusItem{
				ResourceType: resourceType, Slug: row.Slug, Name: row.Name,
				IsSystem: true, IsLive: row.IsLive,
			})
		}
	}

	// The slug resolves to the same team as the UUID does.
	result, _, err := srv.listStatuses(context.Background(), nil,
		&ListStatusesParams{TeamID: testTeamSlug}, testMemberUserID)

	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Equal(t, want, decodeStatusesResult(t, result).Statuses)
}

// With resource_type set only that type's catalog is read, and the catalog's
// order -- system statuses first, then the team's custom ones -- is kept.
func TestListStatuses_SingleResourceTypeKeepsCatalogOrder(t *testing.T) {
	srv, catalog := newStatusToolTestServer(t)
	customID := "status-row-1"
	catalog.EXPECT().List(mock.Anything, testTeamUUID, models.StatusResourceTypeArtifact).
		Return([]services.CatalogStatus{
			{Slug: "active", Name: "Active", IsSystem: true, IsLive: true},
			{Slug: "draft", Name: "Draft", IsSystem: true, IsLive: false},
			{Slug: "archived", Name: "Archived", IsSystem: true, IsLive: false},
			{ID: &customID, Slug: "in-review", Name: "In review", IsLive: true},
		}, nil).Once()

	// The discriminator is trimmed and lowercased, like its sibling tools'.
	result, _, err := srv.listStatuses(context.Background(), nil,
		&ListStatusesParams{TeamID: testTeamUUID, ResourceType: " Artifact "}, testMemberUserID)

	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Equal(t, []mcpStatusItem{
		{ResourceType: "artifact", Slug: "active", Name: "Active", IsSystem: true, IsLive: true},
		{ResourceType: "artifact", Slug: "draft", Name: "Draft", IsSystem: true},
		{ResourceType: "artifact", Slug: "archived", Name: "Archived", IsSystem: true},
		{ResourceType: "artifact", Slug: "in-review", Name: "In review", IsLive: true},
	}, decodeStatusesResult(t, result).Statuses)

	// The payload carries exactly the documented fields: the custom row's id
	// and timestamps are management data, not something to send as `status`.
	var raw struct {
		Statuses []map[string]any `json:"statuses"`
	}
	require.NoError(t, json.Unmarshal([]byte(mcpTextOf(t, result)), &raw))
	require.Len(t, raw.Statuses, 4)
	for _, item := range raw.Statuses {
		assert.ElementsMatch(t,
			[]string{"resource_type", "slug", "name", "is_system", "is_live"}, mapKeys(item))
	}
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

// A catalog with no rows must serialize as [], never null.
func TestListStatuses_EmptyCatalogIsAnEmptyArray(t *testing.T) {
	srv, catalog := newStatusToolTestServer(t)
	catalog.EXPECT().List(mock.Anything, testTeamUUID, models.StatusResourceTypeMemory).
		Return(nil, nil).Once()

	result, _, err := srv.listStatuses(context.Background(), nil,
		&ListStatusesParams{TeamID: testTeamUUID, ResourceType: "memory"}, testMemberUserID)

	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.JSONEq(t, `{"statuses":[]}`, mcpTextOf(t, result))
}

func TestListStatuses_RejectsUnsupportedResourceType(t *testing.T) {
	srv, catalog := newStatusToolTestServer(t)

	result, _, err := srv.listStatuses(context.Background(), nil,
		&ListStatusesParams{TeamID: testTeamUUID, ResourceType: "feed"}, testMemberUserID)

	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Equal(t, "resource_type must be one of: prompt, artifact, blueprint, memory",
		mcpTextOf(t, result))
	catalog.AssertNotCalled(t, "List", mock.Anything, mock.Anything, mock.Anything)
}

// A team the caller does not belong to gets the standard team-resolution
// error, and the catalog is never read for it.
func TestListStatuses_NonMemberTeamReadsNothing(t *testing.T) {
	srv, catalog := newStatusToolTestServer(t)

	result, _, err := srv.listStatuses(context.Background(), nil,
		&ListStatusesParams{TeamID: "someone-elses-team"}, testMemberUserID)

	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Equal(t, teamAccessDeniedText, mcpTextOf(t, result))
	catalog.AssertNotCalled(t, "List", mock.Anything, mock.Anything, mock.Anything)
}

func TestListStatuses_RequiresTeamID(t *testing.T) {
	srv, _ := newStatusToolTestServer(t)

	result, _, err := srv.listStatuses(context.Background(), nil,
		&ListStatusesParams{}, testMemberUserID)

	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Equal(t, teamRequiredText, mcpTextOf(t, result))
}

// A backend failure is reported generically: its text must not reach the
// agent, and no partial list is returned for the types read before it.
func TestListStatuses_CatalogFailureIsGeneric(t *testing.T) {
	srv, catalog := newStatusToolTestServer(t)
	catalog.EXPECT().List(mock.Anything, testTeamUUID, models.StatusResourceTypePrompt).
		Return(systemCatalog(models.StatusResourceTypePrompt), nil).Once()
	catalog.EXPECT().List(mock.Anything, testTeamUUID, models.StatusResourceTypeArtifact).
		Return(nil, assertAnError()).Once()

	result, _, err := srv.listStatuses(context.Background(), nil,
		&ListStatusesParams{TeamID: testTeamUUID}, testMemberUserID)

	require.NoError(t, err)
	require.True(t, result.IsError)
	assert.Equal(t, "Failed to list statuses", mcpTextOf(t, result))
}

// The tool is registered under its name with the description that tells an
// agent when to call it.
func TestListStatuses_IsRegistered(t *testing.T) {
	assert.Contains(t, registeredMCPToolNames(t), listStatusesToolName)
	assert.Contains(t, listStatusesToolDescription, "before setting or filtering by `status`")
	assert.Contains(t, mcpServerInstructions, listStatusesToolName)
}
