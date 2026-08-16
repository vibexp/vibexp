package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

// Tests for vibexp_io_list_teams_and_projects (#814).

const testOtherProjectID = "550e8400-e29b-41d4-a716-446655440099"

// workspaceMocks bundles the doubles the merged tool needs.
type workspaceMocks struct {
	teams    *repomocks.MockTeamRepository
	projects *repomocks.MockProjectRepository
	service  *mocks.MockProjectServiceInterface
}

// newWorkspaceTestServer wires a server whose team repository resolves
// memberTeam() (so resolveTeam succeeds for the member user) and whose project
// repository/service are mockable.
func newWorkspaceTestServer(t *testing.T) (*Server, workspaceMocks) {
	t.Helper()
	srv := newServerWithNullLogger(t)
	m := workspaceMocks{
		teams:    stubTeamResolution(t, []models.Team{memberTeam()}),
		projects: repomocks.NewMockProjectRepository(t),
		service:  mocks.NewMockProjectServiceInterface(t),
	}
	srv.container = &TestContainer{
		TeamRepositoryMock:    m.teams,
		ProjectRepositoryMock: m.projects,
		ProjectServiceMock:    m.service,
	}
	return srv, m
}

func callWorkspace(
	t *testing.T, srv *Server, params *ListTeamsAndProjectsParams,
) (*listTeamsAndProjectsResponse, string) {
	t.Helper()
	result, structured, err := srv.listTeamsAndProjects(context.Background(), nil, params, testMemberUserID)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "unexpected tool error: %s", extractText(t, result))

	resp, ok := structured.(*listTeamsAndProjectsResponse)
	require.True(t, ok, "expected *listTeamsAndProjectsResponse")
	return resp, extractText(t, result)
}

// TestWorkspace_NoArgsReturnsTeamsOnly is the orientation call: teams with their
// counts, and NO projects array — the response that has to be strictly smaller
// than the list_teams it replaces.
func TestWorkspace_NoArgsReturnsTeamsOnly(t *testing.T) {
	srv, m := newWorkspaceTestServer(t)
	m.teams.On("ListByUserID", mock.Anything, testMemberUserID, 10, 0).
		Return([]models.Team{memberTeam()}, 1, nil)
	m.projects.On("CountsByTeamIDs", mock.Anything, []string{testTeamUUID}).
		Return(map[string]int{testTeamUUID: 31}, nil)

	resp, text := callWorkspace(t, srv, &ListTeamsAndProjectsParams{})

	require.Len(t, resp.Teams, 1)
	assert.Equal(t, testTeamUUID, resp.Teams[0].UUID)
	assert.Equal(t, 31, resp.Teams[0].ProjectCount)
	assert.Nil(t, resp.Teams[0].Projects)
	assert.NotContains(t, text, `"projects"`,
		"the orientation call must omit the projects array entirely, not send an empty one")
}

// TestWorkspace_QueryFindsProjectAcrossTeams is the capability the merge exists
// for: one call, no team_id, and a project in a team the caller merely belongs to
// comes back nested under a named team.
func TestWorkspace_QueryFindsProjectAcrossTeams(t *testing.T) {
	srv, m := newWorkspaceTestServer(t)
	otherTeam := models.Team{ID: testOtherTeamUUID, Name: "Other Team", Slug: testOtherTeamSlug}

	m.projects.On("SearchProjects", mock.Anything, testMemberUserID, mock.MatchedBy(
		func(f repositories.ProjectSearchFilters) bool {
			return f.Query == "games for agents" && f.TeamID == ""
		},
	)).Return([]models.ProjectSearchResult{{
		Project: models.Project{
			ID: testOtherProjectID, Slug: "games-for-agents",
			Name: "shaharia-lab/games-for-agents", Description: "Chess",
			TeamID: testOtherTeamUUID,
		},
		Score: 0.82,
	}}, nil)
	// The team itself does not match the query...
	m.teams.On("SearchTeams", mock.Anything, testMemberUserID, "games for agents", 10).
		Return([]models.TeamSearchResult{}, nil)
	// ...so its name has to come from the caller's own teams.
	m.teams.On("ListByUserID", mock.Anything, testMemberUserID, workspaceTeamLookupLimit, 0).
		Return([]models.Team{memberTeam(), otherTeam}, 2, nil)
	m.projects.On("CountsByTeamIDs", mock.Anything, []string{testOtherTeamUUID}).
		Return(map[string]int{testOtherTeamUUID: 3}, nil)

	resp, _ := callWorkspace(t, srv, &ListTeamsAndProjectsParams{Query: "games for agents"})

	require.Len(t, resp.Teams, 1, "only the team holding the match is returned")
	assert.Equal(t, testOtherTeamUUID, resp.Teams[0].UUID)
	assert.Equal(t, "Other Team", resp.Teams[0].Name, "a project hit must never be orphaned under an unnamed team")
	require.Len(t, resp.Teams[0].Projects, 1)
	assert.Equal(t, "shaharia-lab/games-for-agents", resp.Teams[0].Projects[0].Name)
	assert.InDelta(t, 0.82, resp.Teams[0].Projects[0].Score, 0.001)
}

// TestWorkspace_TeamIDWithoutQueryListsProjects is the behaviour
// vibexp_io_list_projects used to serve.
func TestWorkspace_TeamIDWithoutQueryListsProjects(t *testing.T) {
	srv, m := newWorkspaceTestServer(t)
	m.teams.On("GetByID", mock.Anything, testTeamUUID).Return(&models.Team{
		ID: testTeamUUID, Name: "Acme Team", Slug: testTeamSlug,
	}, nil)
	m.projects.On("CountsByTeamIDs", mock.Anything, []string{testTeamUUID}).
		Return(map[string]int{testTeamUUID: 2}, nil)
	m.service.On("ListProjects", testMemberUserID, mock.MatchedBy(func(f services.ProjectFilters) bool {
		return f.TeamID == testTeamUUID && f.Page == 1 && f.Limit == 10
	})).Return(&models.ProjectListResponse{
		Projects: models.JSONArray[models.ProjectResponse]{
			{Project: models.Project{ID: "p1", Slug: "alpha", Name: "Alpha"}},
		},
		TotalCount: 1,
	}, nil)

	// The slug form exercises resolveTeam, which must run first.
	resp, _ := callWorkspace(t, srv, &ListTeamsAndProjectsParams{TeamID: testTeamSlug})

	require.Len(t, resp.Teams, 1)
	assert.Equal(t, testTeamUUID, resp.Teams[0].UUID, "the slug resolved to the canonical UUID")
	require.Len(t, resp.Teams[0].Projects, 1)
	assert.Equal(t, "Alpha", resp.Teams[0].Projects[0].Name)
	assert.Zero(t, resp.Teams[0].Projects[0].Score, "a listing carries no relevance score")
}

// TestWorkspace_ProjectFieldSetIsPinned is the guard the issue asks for: the fat
// models.Project must not leak back into the response through a later refactor.
// It asserts the EXACT key set of the marshalled project object, so an added
// field fails here rather than quietly growing every agent's context.
func TestWorkspace_ProjectFieldSetIsPinned(t *testing.T) {
	data, err := json.Marshal(workspaceProject{
		ID: "p1", Slug: "alpha", Name: "Alpha", Description: "d", Score: 0.5,
	})
	require.NoError(t, err)

	var keyed map[string]any
	require.NoError(t, json.Unmarshal(data, &keyed))

	keys := make([]string, 0, len(keyed))
	for k := range keyed {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"id", "slug", "name", "description", "score"}, keys,
		"the lean project shape is load-bearing: %s", string(data))

	// And the fields deliberately excluded stay excluded.
	for _, banned := range []string{"user_id", "team_id", "version", "homepage", "created_at", "updated_at"} {
		assert.NotContains(t, keyed, banned, "%s must not reappear in the MCP projection", banned)
	}
}

func TestWorkspace_NormalizeParams(t *testing.T) {
	for _, tc := range []struct {
		name       string
		in         ListTeamsAndProjectsParams
		wantLimit  int
		wantPage   int
		wantScope  string
		wantsTeams bool
	}{
		{"defaults", ListTeamsAndProjectsParams{}, 10, 1, scopeBoth, true},
		{"limit capped at 25", ListTeamsAndProjectsParams{Limit: 500}, 25, 1, scopeBoth, true},
		{"non-positive limit defaults", ListTeamsAndProjectsParams{Limit: -3}, 10, 1, scopeBoth, true},
		{"page honoured", ListTeamsAndProjectsParams{Page: 4}, 10, 4, scopeBoth, true},
		{"non-positive page defaults", ListTeamsAndProjectsParams{Page: 0}, 10, 1, scopeBoth, true},
		{"unknown scope falls back to both", ListTeamsAndProjectsParams{Scope: "teamz"}, 10, 1, scopeBoth, true},
		{"scope projects", ListTeamsAndProjectsParams{Scope: scopeProjects}, 10, 1, scopeProjects, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := normalizeWorkspaceParams(&tc.in)
			assert.Equal(t, tc.wantLimit, q.limit)
			assert.Equal(t, tc.wantPage, q.page)
			assert.Equal(t, tc.wantScope, q.scope)
			assert.Equal(t, tc.wantsTeams, q.wantsTeams())
		})
	}
}

// TestWorkspace_LimitIsCappedAtTheRepository proves the cap is not merely
// computed but actually reaches the query.
func TestWorkspace_LimitIsCappedAtTheRepository(t *testing.T) {
	srv, m := newWorkspaceTestServer(t)
	m.teams.On("ListByUserID", mock.Anything, testMemberUserID, workspaceMaxLimit, 0).
		Return([]models.Team{memberTeam()}, 1, nil)
	m.projects.On("CountsByTeamIDs", mock.Anything, mock.Anything).
		Return(map[string]int{}, nil)

	_, _ = callWorkspace(t, srv, &ListTeamsAndProjectsParams{Limit: 1000})

	m.teams.AssertCalled(t, "ListByUserID", mock.Anything, testMemberUserID, workspaceMaxLimit, 0)
}

// TestWorkspace_PageReachesTheRepositoryAsAnOffset pins that `page` is honoured
// on the listing path rather than silently ignored.
func TestWorkspace_PageReachesTheRepositoryAsAnOffset(t *testing.T) {
	srv, m := newWorkspaceTestServer(t)
	m.teams.On("ListByUserID", mock.Anything, testMemberUserID, 10, 20).
		Return([]models.Team{}, 0, nil)
	m.projects.On("CountsByTeamIDs", mock.Anything, mock.Anything).
		Return(map[string]int{}, nil)

	_, _ = callWorkspace(t, srv, &ListTeamsAndProjectsParams{Page: 3, Limit: 10})

	m.teams.AssertCalled(t, "ListByUserID", mock.Anything, testMemberUserID, 10, 20)
}

// TestWorkspace_UnknownTeamIsGenericallyDenied keeps the anti-enumeration
// property at the tool layer: an unknown team and a team the caller is not in
// must be indistinguishable.
func TestWorkspace_UnknownTeamIsGenericallyDenied(t *testing.T) {
	srv, _ := newWorkspaceTestServer(t)

	result, structured, err := srv.listTeamsAndProjects(context.Background(), nil,
		&ListTeamsAndProjectsParams{TeamID: testOtherTeamUUID}, testMemberUserID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Nil(t, structured)
	assertGenericAccessDenied(t, result)
}

// TestWorkspace_ScopeNarrowsTheResult covers scope=teams skipping the project
// search entirely — asserted by the project mock having no expectation for it,
// so a call would fail the test.
func TestWorkspace_ScopeNarrowsTheResult(t *testing.T) {
	srv, m := newWorkspaceTestServer(t)
	m.teams.On("SearchTeams", mock.Anything, testMemberUserID, "acme", 10).
		Return([]models.TeamSearchResult{{Team: memberTeam(), Score: 0.9}}, nil)
	m.projects.On("CountsByTeamIDs", mock.Anything, []string{testTeamUUID}).
		Return(map[string]int{testTeamUUID: 1}, nil)

	resp, _ := callWorkspace(t, srv,
		&ListTeamsAndProjectsParams{Query: "acme", Scope: scopeTeams})

	require.Len(t, resp.Teams, 1)
	assert.Empty(t, resp.Teams[0].Projects)
	m.projects.AssertNotCalled(t, "SearchProjects")
}

// TestWorkspace_RepositoryErrorIsGeneric checks the failure path reports a
// generic message rather than leaking database detail to the model.
func TestWorkspace_RepositoryErrorIsGeneric(t *testing.T) {
	srv, m := newWorkspaceTestServer(t)
	m.teams.On("ListByUserID", mock.Anything, testMemberUserID, 10, 0).
		Return(nil, 0, errors.New("connection refused"))

	result, structured, err := srv.listTeamsAndProjects(context.Background(), nil,
		&ListTeamsAndProjectsParams{}, testMemberUserID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	assert.Nil(t, structured)
	text := extractText(t, result)
	assert.NotContains(t, text, "connection refused", "database detail must not reach the model")
	assert.Contains(t, text, "Failed to list teams and projects")
}

func TestTruncateRunes(t *testing.T) {
	assert.Equal(t, "short", truncateRunes("short", 10))
	assert.Equal(t, "abcde…", truncateRunes("abcdefghij", 5))
	// The trap this exists for: a byte-based cut would split these characters and
	// produce invalid UTF-8.
	multibyte := strings.Repeat("é", 10)
	got := truncateRunes(multibyte, 5)
	assert.Equal(t, strings.Repeat("é", 5)+"…", got)
	assert.True(t, len([]rune(got)) == 6, "counted in runes, not bytes")
}

func TestPageOf(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	assert.Equal(t, []int{1, 2}, pageOf(items, 0, 2))
	assert.Equal(t, []int{3, 4}, pageOf(items, 2, 2))
	assert.Equal(t, []int{5}, pageOf(items, 4, 2), "a partial final page")
	assert.Nil(t, pageOf(items, 10, 2), "past the end is empty, not a panic")
}
