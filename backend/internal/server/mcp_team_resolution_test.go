package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/internal/services/mocks"
)

// Canonical fixtures used across team-scoped MCP tool tests.
const (
	testMemberUserID  = "user-member"
	testTeamUUID      = "550e8400-e29b-41d4-a716-446655440000"
	testTeamSlug      = "acme-team"
	testOtherTeamUUID = "550e8400-e29b-41d4-a716-44665544ffff"
	testOtherTeamSlug = "other-team"
)

// newServerWithNullLogger builds a *Server with a silent logger for tests.
func newServerWithNullLogger(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{}
	logger := slog.New(slog.DiscardHandler)
	return New("8080", nil, "test-api-key", cfg, logger)
}

// stubTeamResolution builds the TeamRepository mock resolveTeam resolves team_id
// through, standing in for the single owner-OR-member query: an identifier
// matching one of the given teams by UUID or slug resolves, anything else gets
// ErrTeamNotFound exactly as the real query's no-rows path does. This is the
// only dependency resolveTeam has, so it is enough to drive membership
// resolution in tool tests.
//
// The expectation is optional (.Maybe()) because some tool tests (e.g. missing
// team_id) reject before resolveTeam reaches the repository.
func stubTeamResolution(t *testing.T, teams []models.Team) *repomocks.MockTeamRepository {
	t.Helper()
	repo := repomocks.NewMockTeamRepository(t)
	repo.On("ResolveByIdentifier", mock.Anything, testMemberUserID, mock.AnythingOfType("string")).
		Return(func(_ context.Context, _, identifier string) (*models.Team, error) {
			for i := range teams {
				if teams[i].ID == identifier || teams[i].Slug == identifier {
					return &teams[i], nil
				}
			}
			return nil, repositories.ErrTeamNotFound
		}).Maybe()
	return repo
}

// stubListTeams configures a MockTeamServiceInterface so ListTeams returns the
// given teams for testMemberUserID (single page). Team resolution no longer goes
// through ListTeams (#812) — this is for the surfaces that genuinely enumerate a
// user's teams: the list_teams tool and prompt-primitive registration.
func stubListTeams(m *mocks.MockTeamServiceInterface, teams []models.Team) {
	m.On("ListTeams", mock.Anything, testMemberUserID, mock.AnythingOfType("int"), mock.AnythingOfType("int")).
		Return(&models.TeamListResponse{
			Teams:      teams,
			TotalCount: len(teams),
			Page:       1,
			PageSize:   100,
		}, nil).Maybe()
}

// memberTeam returns the single team the member user belongs to.
func memberTeam() models.Team {
	return models.Team{ID: testTeamUUID, Name: "Acme Team", Slug: testTeamSlug}
}

func TestResolveTeam_MatchesByUUID(t *testing.T) {
	srv := newServerWithNullLogger(t)
	teamRepo := stubTeamResolution(t, []models.Team{memberTeam()})
	srv.container = &TestContainer{TeamRepositoryMock: teamRepo}

	teamID, errResult := srv.resolveTeam(context.Background(), testMemberUserID, testTeamUUID)

	assert.Nil(t, errResult)
	assert.Equal(t, testTeamUUID, teamID)
	// The whole point of #812: one lookup, never a page-through of every team.
	teamRepo.AssertNumberOfCalls(t, "ResolveByIdentifier", 1)
}

func TestResolveTeam_MatchesBySlug(t *testing.T) {
	srv := newServerWithNullLogger(t)
	teamRepo := stubTeamResolution(t, []models.Team{memberTeam()})
	srv.container = &TestContainer{TeamRepositoryMock: teamRepo}

	teamID, errResult := srv.resolveTeam(context.Background(), testMemberUserID, testTeamSlug)

	assert.Nil(t, errResult)
	assert.Equal(t, testTeamUUID, teamID, "slug must resolve to the canonical UUID")
	teamRepo.AssertNumberOfCalls(t, "ResolveByIdentifier", 1)
}

// TestResolveTeam_SingleQueryRegardlessOfTeamCount pins the acceptance criterion
// directly: membership in many teams must still cost exactly one lookup. The old
// implementation paged ListTeams, so its call count grew with the team count.
func TestResolveTeam_SingleQueryRegardlessOfTeamCount(t *testing.T) {
	srv := newServerWithNullLogger(t)
	teams := make([]models.Team, 0, 250)
	for i := range 250 {
		teams = append(teams, models.Team{
			ID:   fmt.Sprintf("550e8400-e29b-41d4-a716-4466554400%03d", i),
			Slug: fmt.Sprintf("team-%03d", i),
		})
	}
	teams = append(teams, memberTeam())
	teamRepo := stubTeamResolution(t, teams)
	srv.container = &TestContainer{TeamRepositoryMock: teamRepo}

	teamID, errResult := srv.resolveTeam(context.Background(), testMemberUserID, testTeamSlug)

	assert.Nil(t, errResult)
	assert.Equal(t, testTeamUUID, teamID)
	teamRepo.AssertNumberOfCalls(t, "ResolveByIdentifier", 1)
}

func TestResolveTeam_NonMemberDenied(t *testing.T) {
	srv := newServerWithNullLogger(t)
	// The user only belongs to their own team; a different team's UUID must not resolve.
	teamRepo := stubTeamResolution(t, []models.Team{memberTeam()})
	srv.container = &TestContainer{TeamRepositoryMock: teamRepo}

	teamID, errResult := srv.resolveTeam(context.Background(), testMemberUserID, testOtherTeamUUID)

	require.NotNil(t, errResult)
	assert.True(t, errResult.IsError)
	assert.Empty(t, teamID)
	assertGenericAccessDenied(t, errResult)
}

func TestResolveTeam_NonMemberSlugDenied(t *testing.T) {
	srv := newServerWithNullLogger(t)
	teamRepo := stubTeamResolution(t, []models.Team{memberTeam()})
	srv.container = &TestContainer{TeamRepositoryMock: teamRepo}

	teamID, errResult := srv.resolveTeam(context.Background(), testMemberUserID, testOtherTeamSlug)

	require.NotNil(t, errResult)
	assert.True(t, errResult.IsError)
	assert.Empty(t, teamID)
	assertGenericAccessDenied(t, errResult)
}

func TestResolveTeam_EmptyIdentifier(t *testing.T) {
	srv := newServerWithNullLogger(t)
	teamRepo := stubTeamResolution(t, []models.Team{memberTeam()})
	srv.container = &TestContainer{TeamRepositoryMock: teamRepo}

	teamID, errResult := srv.resolveTeam(context.Background(), testMemberUserID, "  ")

	require.NotNil(t, errResult)
	assert.True(t, errResult.IsError)
	assert.Empty(t, teamID)
	text := extractText(t, errResult)
	assert.Contains(t, text, "team_id is required")
	// The full merged-tool name, not the bare "vibexp_io_list_teams" prefix it
	// contains — otherwise this assertion cannot tell the deprecated tool from
	// its replacement (#815).
	assert.Contains(t, text, "vibexp_io_list_teams_and_projects")
	// Membership lookup must not even be attempted for empty input.
	teamRepo.AssertNotCalled(t, "ResolveByIdentifier")
}

// TestResolveTeam_RepositoryError_GenericDenied covers the infrastructure-failure
// arm. It must be byte-identical to the not-found arm — see
// TestResolveTeam_FailureModesAreIndistinguishable.
func TestResolveTeam_RepositoryError_GenericDenied(t *testing.T) {
	srv := newServerWithNullLogger(t)
	teamRepo := repomocks.NewMockTeamRepository(t)
	teamRepo.On("ResolveByIdentifier", mock.Anything, testMemberUserID, testTeamUUID).
		Return(nil, errors.New("db error"))
	srv.container = &TestContainer{TeamRepositoryMock: teamRepo}

	teamID, errResult := srv.resolveTeam(context.Background(), testMemberUserID, testTeamUUID)

	require.NotNil(t, errResult)
	assert.True(t, errResult.IsError)
	assert.Empty(t, teamID)
	assertGenericAccessDenied(t, errResult)
}

// TestResolveTeam_FailureModesAreIndistinguishable is the anti-enumeration
// guarantee stated as one assertion: a nonexistent team, a team the user is not
// a member of, and an infrastructure failure must all produce the SAME text, so
// no caller can probe for the existence of teams it cannot see. Comparing the
// three against each other (not each against a literal) is what makes it fail if
// any single arm is later given its own message.
func TestResolveTeam_FailureModesAreIndistinguishable(t *testing.T) {
	resolveWith := func(t *testing.T, identifier string, ret func() (*models.Team, error)) string {
		t.Helper()
		srv := newServerWithNullLogger(t)
		teamRepo := repomocks.NewMockTeamRepository(t)
		teamRepo.On("ResolveByIdentifier", mock.Anything, testMemberUserID, identifier).
			Return(func(context.Context, string, string) (*models.Team, error) { return ret() })
		srv.container = &TestContainer{TeamRepositoryMock: teamRepo}

		teamID, errResult := srv.resolveTeam(context.Background(), testMemberUserID, identifier)
		require.NotNil(t, errResult)
		assert.Empty(t, teamID)
		return extractText(t, errResult)
	}

	notFound := resolveWith(t, testOtherTeamUUID, func() (*models.Team, error) {
		return nil, repositories.ErrTeamNotFound
	})
	nonMember := resolveWith(t, testOtherTeamSlug, func() (*models.Team, error) {
		return nil, repositories.ErrTeamNotFound
	})
	infraError := resolveWith(t, testTeamUUID, func() (*models.Team, error) {
		return nil, errors.New("connection refused")
	})

	assert.Equal(t, notFound, nonMember, "not-found and non-member must be indistinguishable")
	assert.Equal(t, notFound, infraError, "an infrastructure failure must not be distinguishable either")
}

// assertGenericAccessDenied verifies the result is the generic, anti-enumeration
// access-denied message that nudges the model to list_teams and does not reveal
// whether the team exists.
func assertGenericAccessDenied(t *testing.T, res *mcp.CallToolResult) {
	t.Helper()
	text := extractText(t, res)
	assert.Contains(t, text, "Access denied")
	assert.Contains(t, text, "vibexp_io_list_teams_and_projects")
}
