package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/password"
	"github.com/vibexp/vibexp/internal/logging/logtest"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repo_mocks "github.com/vibexp/vibexp/internal/repositories/mocks"
	"github.com/vibexp/vibexp/pkg/events"
)

const (
	invRegToken   = "tok-invitation"
	invRegEmail   = "new.person@example.com"
	invRegTeamID  = "team-1"
	invRegInvID   = "inv-1"
	invRegInviter = "user-inviter"
	invRegPhrase  = "a long invited passphrase"
)

// invRegAllowlist admits everything but the addresses in denied.
type invRegAllowlist struct {
	AccessAllowlistResolver
	denied map[string]bool
	err    error
}

func (a invRegAllowlist) IsEmailAllowed(_ context.Context, email string) (allowed, active bool, err error) {
	return !a.denied[email], len(a.denied) > 0, a.err
}

// invRegSetter records stored passwords.
type invRegSetter struct {
	stored []string
	err    error
}

func (s *invRegSetter) SetPassword(_ context.Context, userID, _ string) error {
	if s.err != nil {
		return s.err
	}
	s.stored = append(s.stored, userID)
	return nil
}

// invRegEvents records published events.
type invRegEvents struct {
	published []events.Event
	err       error
}

func (e *invRegEvents) Publish(_ context.Context, event events.Event) error {
	if e.err != nil {
		return e.err
	}
	e.published = append(e.published, event)
	return nil
}

type invRegFixture struct {
	svc         *InvitationRegistrationService
	invitations *repo_mocks.MockTeamInvitationRepository
	teams       *repo_mocks.MockTeamRepository
	users       *repo_mocks.MockUserRepository
	setter      *invRegSetter
	events      *invRegEvents
	allowlist   *invRegAllowlist
	now         time.Time
}

func newInvRegFixture(t *testing.T, policy AuthPolicyResolver) *invRegFixture {
	t.Helper()
	logger, _ := logtest.New()
	f := &invRegFixture{
		invitations: repo_mocks.NewMockTeamInvitationRepository(t),
		teams:       repo_mocks.NewMockTeamRepository(t),
		users:       repo_mocks.NewMockUserRepository(t),
		setter:      &invRegSetter{},
		events:      &invRegEvents{},
		allowlist:   &invRegAllowlist{denied: map[string]bool{}},
		now:         time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC),
	}
	f.svc = NewInvitationRegistrationService(InvitationRegistrationDeps{
		Invitations: f.invitations, Teams: f.teams, Users: f.users, Passwords: f.setter,
		Policy: policy, Allowlist: f.allowlist, Events: f.events, Logger: logger,
	})
	f.svc.now = func() time.Time { return f.now }
	return f
}

// invitation is a pending invitation that expires tomorrow, changed by mutate.
func (f *invRegFixture) invitation(mutate func(*models.TeamInvitation)) *models.TeamInvitation {
	inv := &models.TeamInvitation{
		ID: invRegInvID, TeamID: invRegTeamID, InviterID: invRegInviter, InviteeEmail: invRegEmail,
		Role: models.TeamMemberRoleAdmin, Token: invRegToken, Status: models.InvitationStatusPending,
		ExpiresAt: f.now.Add(24 * time.Hour),
	}
	if mutate != nil {
		mutate(inv)
	}
	return inv
}

func (f *invRegFixture) pending() {
	f.invitations.On("GetByToken", mock.Anything, invRegToken).Return(f.invitation(nil), nil)
}

func (f *invRegFixture) noAccount() {
	f.users.On("GetByEmail", mock.Anything, invRegEmail).Return(nil, repositories.ErrUserNotFound)
}

// unusableInvitations are the states one uniform error covers.
func unusableInvitations() map[string]func(*models.TeamInvitation) {
	return map[string]func(*models.TeamInvitation){
		"accepted": func(i *models.TeamInvitation) { i.Status = models.InvitationStatusAccepted },
		"rejected": func(i *models.TeamInvitation) { i.Status = models.InvitationStatusRejected },
		"revoked":  func(i *models.TeamInvitation) { i.Status = models.InvitationStatusRevoked },
		"expired":  func(i *models.TeamInvitation) { i.ExpiresAt = i.ExpiresAt.Add(-48 * time.Hour) },
		"expiring this instant": func(i *models.TeamInvitation) {
			i.ExpiresAt = time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
		},
	}
}

func TestInvitationRegistration_Preview(t *testing.T) {
	ctx := context.Background()

	t.Run("a pending invitation for an email with no account", func(t *testing.T) {
		f := newInvRegFixture(t, passwordLoginOn)
		f.pending()
		f.noAccount()
		f.teams.On("GetByID", mock.Anything, invRegTeamID).Return(&models.Team{ID: invRegTeamID, Name: "Acme"}, nil)
		f.users.On("GetByID", mock.Anything, invRegInviter).
			Return(&models.User{ID: invRegInviter, Name: "Ada Lovelace", Email: "ada@example.com"}, nil)

		preview, err := f.svc.Preview(ctx, invRegToken)

		require.NoError(t, err)
		assert.Equal(t, &InvitationPreview{
			TeamName: "Acme", InviterName: "Ada Lovelace", Email: invRegEmail, Role: models.TeamMemberRoleAdmin,
			ExpiresAt: f.now.Add(24 * time.Hour), PasswordRegistration: true,
		}, preview)
	})

	t.Run("an existing account, or password login off, cannot register", func(t *testing.T) {
		for name, tc := range map[string]struct {
			policy AuthPolicyResolver
			exists bool
		}{
			"account exists":     {passwordLoginOn, true},
			"password login off": {passwordLoginOff, false},
		} {
			f := newInvRegFixture(t, tc.policy)
			f.pending()
			if tc.exists {
				f.users.On("GetByEmail", mock.Anything, invRegEmail).Return(&models.User{ID: "u-existing"}, nil)
			} else {
				f.noAccount()
			}
			f.teams.On("GetByID", mock.Anything, invRegTeamID).Return(&models.Team{Name: "Acme"}, nil)
			// A deleted inviter does not make the invitation unreadable.
			f.users.On("GetByID", mock.Anything, invRegInviter).Return(nil, repositories.ErrUserNotFound)

			preview, err := f.svc.Preview(ctx, invRegToken)

			require.NoError(t, err, name)
			assert.Equal(t, tc.exists, preview.AccountExists, name)
			assert.False(t, preview.PasswordRegistration, name)
			assert.Empty(t, preview.InviterName, name)
		}
	})

	t.Run("every unusable link is the same error", func(t *testing.T) {
		for name, mutate := range unusableInvitations() {
			f := newInvRegFixture(t, passwordLoginOn)
			f.invitations.On("GetByToken", mock.Anything, invRegToken).Return(f.invitation(mutate), nil)

			_, err := f.svc.Preview(ctx, invRegToken)

			require.ErrorIs(t, err, ErrInvitationUnusable, name)
		}
		f := newInvRegFixture(t, passwordLoginOn)
		f.invitations.On("GetByToken", mock.Anything, "nope").Return(nil, repositories.ErrTeamInvitationNotFound)
		_, err := f.svc.Preview(ctx, "nope")
		require.ErrorIs(t, err, ErrInvitationUnusable, "unknown")
		_, err = f.svc.Preview(ctx, "")
		require.ErrorIs(t, err, ErrInvitationUnusable, "empty")
	})

	t.Run("a storage failure is not reported as an unusable link", func(t *testing.T) {
		f := newInvRegFixture(t, passwordLoginOn)
		f.invitations.On("GetByToken", mock.Anything, invRegToken).Return(nil, errors.New("pq: down"))

		_, err := f.svc.Preview(ctx, invRegToken)

		require.Error(t, err)
		require.NotErrorIs(t, err, ErrInvitationUnusable)
	})
}

func TestInvitationRegistration_Register(t *testing.T) {
	ctx := context.Background()

	// arrange expects the account to be created (given id "user-new") and returns the fixture.
	arrange := func(t *testing.T) *invRegFixture {
		t.Helper()
		f := newInvRegFixture(t, passwordLoginOn)
		f.pending()
		f.noAccount()
		f.users.On("Create", mock.Anything, mock.MatchedBy(func(u *models.User) bool {
			return u.Email == invRegEmail && u.EmailVerifiedAt == nil
		})).Run(func(args mock.Arguments) { args.Get(1).(*models.User).ID = "user-new" }).Return(nil).Once()
		return f
	}

	t.Run("creates the account, provisions it, stores the password and joins the team", func(t *testing.T) {
		f := arrange(t)
		f.invitations.On("Accept", mock.Anything, invRegInvID, mock.MatchedBy(func(m *models.TeamMember) bool {
			return m.TeamID == invRegTeamID && m.UserID == "user-new" && m.Role == models.TeamMemberRoleAdmin
		})).Return(nil).Once()

		result, err := f.svc.Register(ctx, invRegToken, invRegPhrase, "  Grace Hopper ")

		require.NoError(t, err)
		assert.Equal(t, "user-new", result.User.ID)
		assert.Equal(t, "Grace Hopper", result.User.Name)
		assert.Nil(t, result.User.EmailVerifiedAt, "holding the link does not prove the mailbox")
		assert.Equal(t, invRegTeamID, result.TeamID)
		assert.True(t, result.InvitationAccepted)
		require.Len(t, f.events.published, 1, "user.created provisions the workspace")
		assert.Equal(t, []string{"user-new"}, f.setter.stored)
	})

	t.Run("with no name, the part of the email before the @ is used", func(t *testing.T) {
		f := arrange(t)
		f.invitations.On("Accept", mock.Anything, invRegInvID, mock.Anything).Return(nil).Once()

		result, err := f.svc.Register(ctx, invRegToken, invRegPhrase, "   ")

		require.NoError(t, err)
		assert.Equal(t, "new.person", result.User.Name)
	})

	t.Run("a failure to join the team leaves a complete account and says so", func(t *testing.T) {
		for name, acceptErr := range map[string]error{
			"revoked at that moment": repositories.ErrTeamInvitationNotPending,
			"storage failure":        errors.New("pq: down"),
		} {
			f := arrange(t)
			f.invitations.On("Accept", mock.Anything, invRegInvID, mock.Anything).Return(acceptErr).Once()

			result, err := f.svc.Register(ctx, invRegToken, invRegPhrase, "")

			require.NoError(t, err, name)
			assert.False(t, result.InvitationAccepted, name)
			assert.Equal(t, "user-new", result.User.ID, name)
			f.users.AssertNotCalled(t, "DeleteByID", mock.Anything, mock.Anything)
		}
	})

	t.Run("a failure after the account row exists removes it again", func(t *testing.T) {
		for name, breakIt := range map[string]func(*invRegFixture){
			"provisioning event": func(f *invRegFixture) { f.events.err = errors.New("bus down") },
			"password store":     func(f *invRegFixture) { f.setter.err = errors.New("pq: down") },
		} {
			f := arrange(t)
			breakIt(f)
			f.users.On("DeleteByID", mock.Anything, "user-new").Return(nil).Once()

			_, err := f.svc.Register(ctx, invRegToken, invRegPhrase, "")

			require.Error(t, err, name)
			f.invitations.AssertNotCalled(t, "Accept", mock.Anything, mock.Anything, mock.Anything)
		}
	})

	t.Run("the removal runs even when the request is already cancelled", func(t *testing.T) {
		f := arrange(t)
		f.setter.err = errors.New("pq: down")
		cancelled, cancel := context.WithCancel(ctx)
		f.users.On("DeleteByID", mock.MatchedBy(func(c context.Context) bool { return c.Err() == nil }), "user-new").
			Return(nil).Once()
		cancel()

		_, err := f.svc.Register(cancelled, invRegToken, invRegPhrase, "")

		require.Error(t, err)
	})
}

func TestInvitationRegistration_RefusalsWriteNothing(t *testing.T) {
	ctx := context.Background()
	// Every fixture uses strict mocks: Create, Accept and DeleteByID are never
	// expected, so any write fails the test.

	t.Run("an unusable link", func(t *testing.T) {
		for name, mutate := range unusableInvitations() {
			f := newInvRegFixture(t, passwordLoginOn)
			f.invitations.On("GetByToken", mock.Anything, invRegToken).Return(f.invitation(mutate), nil)

			_, err := f.svc.Register(ctx, invRegToken, invRegPhrase, "")

			require.ErrorIs(t, err, ErrInvitationUnusable, name)
		}
	})

	t.Run("an account already has the email", func(t *testing.T) {
		f := newInvRegFixture(t, passwordLoginOn)
		f.pending()
		f.users.On("GetByEmail", mock.Anything, invRegEmail).Return(&models.User{ID: "u-existing"}, nil)

		_, err := f.svc.Register(ctx, invRegToken, invRegPhrase, "")

		require.ErrorIs(t, err, ErrInvitationAccountExists)
	})

	t.Run("the allowlist no longer admits the email, or cannot be read", func(t *testing.T) {
		denied := newInvRegFixture(t, passwordLoginOn)
		denied.pending()
		denied.noAccount()
		denied.allowlist.denied[invRegEmail] = true
		_, err := denied.svc.Register(ctx, invRegToken, invRegPhrase, "")
		require.ErrorIs(t, err, ErrInvitationEmailNotAllowed)

		unreadable := newInvRegFixture(t, passwordLoginOn)
		unreadable.pending()
		unreadable.noAccount()
		unreadable.allowlist.err = errors.New("pq: down")
		_, err = unreadable.svc.Register(ctx, invRegToken, invRegPhrase, "")
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrInvitationEmailNotAllowed, "a failure is not a verdict")
	})

	t.Run("a password that breaks the policy", func(t *testing.T) {
		for name, plaintext := range map[string]string{"too short": "short", "the email itself": invRegEmail} {
			f := newInvRegFixture(t, passwordLoginOn)
			f.pending()
			f.noAccount()

			_, err := f.svc.Register(ctx, invRegToken, plaintext, "")

			var policyErr *password.PolicyError
			require.ErrorAs(t, err, &policyErr, name)
		}
	})

	t.Run("a simultaneous registration won the email", func(t *testing.T) {
		f := newInvRegFixture(t, passwordLoginOn)
		f.pending()
		f.noAccount()
		f.users.On("Create", mock.Anything, mock.Anything).Return(repositories.ErrUserEmailTaken).Once()

		_, err := f.svc.Register(ctx, invRegToken, invRegPhrase, "")

		require.ErrorIs(t, err, ErrInvitationAccountExists)
		assert.Empty(t, f.events.published)
		assert.Empty(t, f.setter.stored)
	})
}
