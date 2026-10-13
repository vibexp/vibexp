package services

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/mocks"
)

// Inviting against the access allowlist, and address normalization (#1434).

const (
	inviteTeamID  = "team-456"
	inviteOwnerID = "user-123"
)

// inviteFixture is a TeamInvitationService whose inviter owns a non-personal
// team, with an allowlist of exact emails (nil for an open instance).
type inviteFixture struct {
	svc         *TeamInvitationService
	invitations *mocks.MockTeamInvitationRepository
	users       *mocks.MockUserRepository
	email       *MockEmailService
}

func newInviteFixture(t *testing.T, allowedEmails []string) *inviteFixture {
	t.Helper()
	logger, _ := logtest.New()
	f := &inviteFixture{
		invitations: &mocks.MockTeamInvitationRepository{},
		users:       &mocks.MockUserRepository{},
		email:       &MockEmailService{},
	}
	teams := &mocks.MockTeamRepository{}
	members := &mocks.MockTeamMemberRepository{}
	members.On("GetByTeamAndUser", mock.Anything, inviteTeamID, inviteOwnerID).
		Return(&models.TeamMember{TeamID: inviteTeamID, UserID: inviteOwnerID, Role: models.TeamMemberRoleOwner}, nil)
	teams.On("GetByID", mock.Anything, inviteTeamID).
		Return(&models.Team{ID: inviteTeamID, Name: "Test Team"}, nil)
	f.users.On("GetByID", mock.Anything, inviteOwnerID).
		Return(&models.User{ID: inviteOwnerID, Name: "Inviter", Email: "inviter@corp.example"}, nil).Maybe()

	f.svc = NewTeamInvitationService(TeamInvitationServiceDeps{
		InvitationRepo: f.invitations,
		TeamRepo:       teams,
		TeamMemberRepo: members,
		UserRepo:       f.users,
		EmailService:   f.email,
		Authz:          NewAuthorizationService(members, nil),
		Allowlist:      newStaticAllowlistResolver(logger, allowedEmails),
		Cfg:            &config.Config{},
		Logger:         logger,
	})
	return f
}

// expectInvitable lets email pass the "not a member, not already invited"
// checks and records the invitation that gets created for it.
func (f *inviteFixture) expectInvitable(email string, created *[]string) {
	f.users.On("GetByEmail", mock.Anything, email).Return(nil, repositories.ErrUserNotFound)
	f.invitations.On("GetPendingByEmail", mock.Anything, email).Return([]models.TeamInvitation{}, nil)
	f.invitations.On("Create", mock.Anything, mock.MatchedBy(func(inv *models.TeamInvitation) bool {
		return inv.InviteeEmail == email
	})).Return(nil).Run(func(args mock.Arguments) {
		*created = append(*created, args.Get(1).(*models.TeamInvitation).InviteeEmail)
	})
	f.email.On("SendTeamInvitation", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Maybe()
}

func (f *inviteFixture) invite(emails ...string) ([]*models.TeamInvitation, error) {
	return f.svc.InviteMembers(context.Background(), inviteOwnerID, inviteTeamID, emails, models.TeamMemberRoleMember)
}

// An address the allowlist does not admit can never sign in, so its invitation
// could never be accepted: the whole request is refused, naming each one, and
// nothing is created or sent.
func TestInviteMembers_RejectsAddressesOutsideTheAllowlist(t *testing.T) {
	t.Run("one of several is outside: nothing is created", func(t *testing.T) {
		f := newInviteFixture(t, []string{"ada@corp.example", "bob@corp.example"})

		invitations, err := f.invite("ada@corp.example", "Mallory@Elsewhere.test", "bob@corp.example", "eve@elsewhere.test")

		var notAllowed *InviteesNotAllowedError
		require.True(t, stderrors.As(err, &notAllowed), "got %v", err)
		assert.Equal(t, []string{"mallory@elsewhere.test", "eve@elsewhere.test"}, notAllowed.Emails,
			"every rejected address is named, normalized, in the order given")
		assert.Contains(t, err.Error(), "mallory@elsewhere.test")
		assert.Empty(t, invitations)
		f.invitations.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		f.email.AssertNotCalled(t, "SendTeamInvitation",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		f.users.AssertNotCalled(t, "GetByEmail", mock.Anything, mock.Anything)
	})

	t.Run("all inside: invited", func(t *testing.T) {
		f := newInviteFixture(t, []string{"ada@corp.example"})
		var created []string
		f.expectInvitable("ada@corp.example", &created)

		invitations, err := f.invite("ada@corp.example")

		require.NoError(t, err)
		assert.Len(t, invitations, 1)
		assert.Equal(t, []string{"ada@corp.example"}, created)
	})

	t.Run("no allowlist: anyone can be invited", func(t *testing.T) {
		f := newInviteFixture(t, nil)
		var created []string
		f.expectInvitable("anyone@elsewhere.test", &created)

		_, err := f.invite("anyone@elsewhere.test")

		require.NoError(t, err)
		assert.Equal(t, []string{"anyone@elsewhere.test"}, created)
	})
}

// An allowlist that cannot be evaluated fails the request rather than creating
// an invitation on a guess.
func TestInviteMembers_FailsWhenTheAllowlistCannotBeEvaluated(t *testing.T) {
	f := newInviteFixture(t, nil)
	boom := stderrors.New("allowlist unreadable")
	f.svc.allowlist = failingAllowlist{err: boom}

	_, err := f.invite("ada@corp.example")

	require.ErrorIs(t, err, boom)
	var notAllowed *InviteesNotAllowedError
	assert.False(t, stderrors.As(err, &notAllowed), "an unreadable allowlist is a failure, not a rejection")
	f.invitations.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// failingAllowlist cannot be evaluated.
type failingAllowlist struct {
	AccessAllowlistResolver
	err error
}

func (a failingAllowlist) IsEmailAllowed(context.Context, string) (allowed, active bool, err error) {
	return false, true, a.err
}

// Addresses are stored and compared normalized: mixed case, padding and a
// repeat of the same person collapse to one lower-case invitation.
func TestInviteMembers_NormalizesAndDeduplicatesAddresses(t *testing.T) {
	f := newInviteFixture(t, nil)
	var created []string
	f.expectInvitable("bob@example.com", &created)

	invitations, err := f.invite("  Bob@Example.COM ", "bob@example.com", "", "BOB@EXAMPLE.COM")

	require.NoError(t, err)
	require.Len(t, invitations, 1)
	assert.Equal(t, []string{"bob@example.com"}, created, "one invitation, in the stored form")
	f.invitations.AssertNumberOfCalls(t, "Create", 1)
}

func TestNormalizeInvitees(t *testing.T) {
	assert.Equal(t, []string{"a@x.test", "b@x.test"},
		normalizeInvitees([]string{" A@x.test", "b@X.test", "a@x.test ", "  ", ""}))
	assert.Empty(t, normalizeInvitees(nil))
}
