package server

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// mockTeamNameContainer exposes a mock TeamRepository; everything else falls back to
// BaseMockContainer's nil defaults — notably TeamService(), which stays nil so that
// regressing to TeamService.GetTeam panics loudly here instead of silently emptying
// team_name again.
type mockTeamNameContainer struct {
	BaseMockContainer
	teamRepo *mocks.MockTeamRepository
}

func (m *mockTeamNameContainer) TeamRepository() repositories.TeamRepository {
	return m.teamRepo
}

func newTeamNameServer(teamRepo *mocks.MockTeamRepository) *Server {
	return &Server{
		port:      "8080",
		container: &mockTeamNameContainer{teamRepo: teamRepo},
		logger:    slog.New(slog.DiscardHandler),
	}
}

// TestFetchTeamsForInvitations_ResolvesTeamForNonMember is the regression test for
// the team_name half of #251.
//
// fetchTeamsForInvitations used to resolve teams through TeamService.GetTeam, which
// verifies the caller is a MEMBER of the team. A pending invitee is by definition
// not a member yet, so every lookup returned "team not found", the error was
// swallowed, and team_name serialized as "" — the dashboard banner rendered
// "Test invited you to ." with the team name missing.
//
// The invitee's membership is deliberately never consulted here: the caller has
// already scoped the invitation list to the authenticated user's email, so the only
// teams reachable are ones that user was actually invited to.
func TestFetchTeamsForInvitations_ResolvesTeamForNonMember(t *testing.T) {
	teamRepo := mocks.NewMockTeamRepository(t)
	teamRepo.On("GetByID", mock.Anything, "team-1").
		Return(&models.Team{ID: "team-1", Name: "Test ACL"}, nil).Once()

	srv := newTeamNameServer(teamRepo)

	teams := srv.fetchTeamsForInvitations(context.Background(), map[string]bool{"team-1": true})

	require.Contains(t, teams, "team-1", "a pending invitee must still resolve the team (#251)")
	assert.Equal(t, "Test ACL", teams["team-1"].Name)

	// Pins the tenancy-only repository read: if this regressed to
	// TeamService.GetTeam, GetByID would go uncalled and the nil TeamService would
	// panic.
	teamRepo.AssertExpectations(t)
}

// TestBuildInvitationResponses_PopulatesTeamName proves the resolved team actually
// reaches the wire — the field the banner reads.
func TestBuildInvitationResponses_PopulatesTeamName(t *testing.T) {
	srv := newTeamNameServer(mocks.NewMockTeamRepository(t))

	invitations := []*models.TeamInvitation{
		{ID: "inv-1", TeamID: "team-1", InviterID: "user-1", InviteeEmail: "test2@test.com"},
	}
	teams := map[string]*models.Team{"team-1": {ID: "team-1", Name: "Test ACL"}}

	responses := srv.buildInvitationResponses(invitations, teams, map[string]*models.User{})

	require.Len(t, responses, 1)
	assert.Equal(t, "Test ACL", responses[0].TeamName,
		"team_name must be populated, or the banner renders 'X invited you to .' (#251)")
}

// TestBuildInvitationResponses_ToleratesUnresolvableTeam documents the fallback:
// an unresolvable team degrades to an empty name rather than dropping the
// invitation or failing the whole list.
func TestBuildInvitationResponses_ToleratesUnresolvableTeam(t *testing.T) {
	srv := newTeamNameServer(mocks.NewMockTeamRepository(t))

	invitations := []*models.TeamInvitation{
		{ID: "inv-1", TeamID: "missing-team", InviterID: "user-1"},
	}

	responses := srv.buildInvitationResponses(invitations, map[string]*models.Team{}, map[string]*models.User{})

	require.Len(t, responses, 1)
	assert.Empty(t, responses[0].TeamName)
	assert.Equal(t, "inv-1", responses[0].ID, "the invitation itself must survive")
}
