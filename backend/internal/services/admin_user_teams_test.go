package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

// Team assignment at user creation (#1436).

const (
	teamAlpha    = "11111111-1111-4111-8111-111111111111"
	teamBeta     = "22222222-2222-4222-8222-222222222222"
	teamPersonal = "33333333-3333-4333-8333-333333333333"
)

type teamFixture struct {
	svc       AdminServiceInterface
	adminRepo *repomocks.MockAdminRepository
	users     *repomocks.MockUserRepository
	teams     *repomocks.MockTeamRepository
	members   *repomocks.MockTeamMemberRepository
	pub       *stubPublisher
}

func newTeamFixture(t *testing.T) *teamFixture {
	t.Helper()
	f := &teamFixture{
		adminRepo: repomocks.NewMockAdminRepository(t),
		users:     repomocks.NewMockUserRepository(t),
		teams:     repomocks.NewMockTeamRepository(t),
		members:   repomocks.NewMockTeamMemberRepository(t),
		pub:       &stubPublisher{},
	}
	f.svc = NewAdminService(f.adminRepo, f.users, f.pub, nil, WithAdminTeamAssignment(f.teams, f.members))
	f.teams.On("GetByID", mock.Anything, teamAlpha).Return(&models.Team{ID: teamAlpha, Name: "Alpha"}, nil).Maybe()
	f.teams.On("GetByID", mock.Anything, teamBeta).Return(&models.Team{ID: teamBeta, Name: "Beta"}, nil).Maybe()
	f.teams.On("GetByID", mock.Anything, teamPersonal).
		Return(&models.Team{ID: teamPersonal, Name: "Private Workspace", IsPersonal: true}, nil).Maybe()
	f.teams.On("GetByID", mock.Anything, mock.Anything).Return(nil, repositories.ErrTeamNotFound).Maybe()
	return f
}

// expectCreate sets up a creation that reaches the user insert.
func (f *teamFixture) expectCreate() {
	f.users.On("GetByEmail", mock.Anything, "new.user@example.com").Return(nil, repositories.ErrUserNotFound)
	f.users.On("Create", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { args.Get(1).(*models.User).ID = createdUserID }).Return(nil)
}

func TestCreateUser_AddsTheUserToItsTeams(t *testing.T) {
	f := newTeamFixture(t)
	f.expectCreate()
	var joined []models.TeamMember
	f.members.On("Create", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { joined = append(joined, *args.Get(1).(*models.TeamMember)) }).Return(nil)
	f.adminRepo.On("GetUserDetail", mock.Anything, createdUserID).Return(createdDetail(), nil)

	_, err := f.svc.CreateUser(context.Background(), AdminUserCreateRequest{
		Email: "new.user@example.com",
		Teams: []AdminTeamAssignment{
			{TeamID: teamAlpha, Role: models.TeamMemberRoleMember},
			{TeamID: teamBeta, Role: models.TeamMemberRoleAdmin},
		},
	})

	require.NoError(t, err)
	require.Len(t, joined, 2)
	assert.Equal(t, models.TeamMember{TeamID: teamAlpha, UserID: createdUserID, Role: models.TeamMemberRoleMember,
		CreatedAt: joined[0].CreatedAt, UpdatedAt: joined[0].UpdatedAt}, joined[0])
	assert.Equal(t, models.TeamMemberRoleAdmin, joined[1].Role)
	require.Len(t, f.pub.published, 1, "the user still gets its own workspace")
}

// A bad team is refused before anything is written.
func TestCreateUser_RefusesABadTeamBeforeWritingAnything(t *testing.T) {
	for name, tc := range map[string]struct {
		teams []AdminTeamAssignment
		index int
	}{
		"unknown team": {[]AdminTeamAssignment{{TeamID: "44444444-4444-4444-8444-444444444444",
			Role: models.TeamMemberRoleMember}}, 0},
		"personal workspace": {[]AdminTeamAssignment{{TeamID: teamAlpha, Role: models.TeamMemberRoleMember},
			{TeamID: teamPersonal, Role: models.TeamMemberRoleMember}}, 1},
		"team named twice": {[]AdminTeamAssignment{{TeamID: teamAlpha, Role: models.TeamMemberRoleMember},
			{TeamID: teamAlpha, Role: models.TeamMemberRoleAdmin}}, 1},
		"owner role": {[]AdminTeamAssignment{{TeamID: teamAlpha, Role: models.TeamMemberRoleOwner}}, 0},
	} {
		t.Run(name, func(t *testing.T) {
			f := newTeamFixture(t)
			f.users.On("GetByEmail", mock.Anything, "new.user@example.com").Return(nil, repositories.ErrUserNotFound).Maybe()

			_, err := f.svc.CreateUser(context.Background(),
				AdminUserCreateRequest{Email: "new.user@example.com", Teams: tc.teams})

			var invalid *ErrAdminUserTeamInvalid
			require.ErrorAs(t, err, &invalid)
			assert.Equal(t, tc.index, invalid.Index)
			f.users.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
			f.members.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
			assert.Empty(t, f.pub.published)
		})
	}
}

// A membership that cannot be added rolls the whole creation back: the user
// row is deleted (its other memberships cascade) and nothing is published.
func TestCreateUser_RollsBackWhenAMembershipFails(t *testing.T) {
	f := newTeamFixture(t)
	f.expectCreate()
	boom := errors.New("connection refused")
	f.members.On("Create", mock.Anything, mock.MatchedBy(func(m *models.TeamMember) bool { return m.TeamID == teamAlpha })).
		Return(nil).Once()
	f.members.On("Create", mock.Anything, mock.MatchedBy(func(m *models.TeamMember) bool { return m.TeamID == teamBeta })).
		Return(boom).Once()
	f.users.On("DeleteByID", mock.Anything, createdUserID).Return(nil).Once()

	_, err := f.svc.CreateUser(context.Background(), AdminUserCreateRequest{
		Email: "new.user@example.com",
		Teams: []AdminTeamAssignment{
			{TeamID: teamAlpha, Role: models.TeamMemberRoleMember},
			{TeamID: teamBeta, Role: models.TeamMemberRoleMember},
		},
	})

	require.ErrorIs(t, err, boom)
	f.users.AssertCalled(t, "DeleteByID", mock.Anything, createdUserID)
	assert.Empty(t, f.pub.published, "no workspace is provisioned for a creation that failed")
}

// Without team repositories wired, a request that names teams is refused, not
// silently half-done.
func TestCreateUser_TeamsNeedTheRepositories(t *testing.T) {
	svc, _, users, _ := newCreateService(t)
	users.On("GetByEmail", mock.Anything, mock.Anything).Return(nil, repositories.ErrUserNotFound).Maybe()

	_, err := svc.CreateUser(context.Background(), AdminUserCreateRequest{
		Email: "new.user@example.com", Teams: []AdminTeamAssignment{{TeamID: teamAlpha, Role: models.TeamMemberRoleMember}},
	})

	require.Error(t, err)
	users.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}
