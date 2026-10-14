//go:build integration

package postgres

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/password"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/pkg/events"
)

// Registration through an invitation (#1435) with the real service over the
// real repositories: what the rows look like afterwards is the point.

// invitedOnlyPasswordPolicy is an instance with password login on and
// registration by invitation only.
type invitedOnlyPasswordPolicy struct{ services.AuthPolicyResolver }

func (invitedOnlyPasswordPolicy) Policy(context.Context) (models.InstanceAuthPolicy, error) {
	return models.InstanceAuthPolicy{
		PasswordLoginEnabled: true, RegistrationMode: models.RegistrationModeInviteOnly,
	}, nil
}

// denyingAllowlist admits nobody but the listed addresses.
type denyingAllowlist struct {
	services.AccessAllowlistResolver
	admitted map[string]bool
}

func (a denyingAllowlist) IsEmailAllowed(_ context.Context, email string) (allowed, active bool, err error) {
	return a.admitted[email], true, nil
}

// countingEvents records the user.created events and can be made to fail.
type countingEvents struct {
	mu        sync.Mutex
	published int
	err       error
}

func (e *countingEvents) Publish(context.Context, events.Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err != nil {
		return e.err
	}
	e.published++
	return nil
}

type invitationRegistrationStack struct {
	t       *testing.T
	svc     *services.InvitationRegistrationService
	events  *countingEvents
	teamID  string
	ownerID string
}

func newInvitationRegistrationStack(t *testing.T, allowlist services.AccessAllowlistResolver) *invitationRegistrationStack {
	t.Helper()
	resetSearchTables(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	users := NewUserRepository(integrationDB)
	passwords := services.NewPasswordAuthService(services.PasswordAuthDeps{
		Users: users, Credentials: NewUserCredentialRepository(integrationDB),
		Hasher: password.NewHasher(password.Params{Memory: 64, Time: 1, Threads: 1, SaltLength: 16, KeyLength: 32}),
		Logger: logger,
	})
	stack := &invitationRegistrationStack{t: t, events: &countingEvents{}}
	stack.teamID, stack.ownerID = gateTeam(t)
	stack.svc = services.NewInvitationRegistrationService(services.InvitationRegistrationDeps{
		Invitations: NewTeamInvitationRepository(integrationDB), Teams: NewTeamRepository(integrationDB),
		Users: users, Passwords: passwords, Policy: invitedOnlyPasswordPolicy{}, Allowlist: allowlist,
		Events: stack.events, Logger: logger,
	})
	return stack
}

// invite stores a pending invitation for email and returns its token.
func (s *invitationRegistrationStack) invite(email string) string {
	s.t.Helper()
	now := time.Now()
	inv := &models.TeamInvitation{
		TeamID: s.teamID, InviterID: s.ownerID, InviteeEmail: email, Role: models.TeamMemberRoleAdmin,
		Token: uuid.NewString(), Status: models.InvitationStatusPending,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(s.t, NewTeamInvitationRepository(integrationDB).Create(context.Background(), inv))
	return inv.Token
}

func (s *invitationRegistrationStack) count(query string, args ...any) int {
	s.t.Helper()
	var n int
	require.NoError(s.t, integrationDB.QueryRowContext(context.Background(), query, args...).Scan(&n))
	return n
}

func (s *invitationRegistrationStack) invitationStatus(token string) models.InvitationStatus {
	s.t.Helper()
	inv, err := NewTeamInvitationRepository(integrationDB).GetByToken(context.Background(), token)
	require.NoError(s.t, err)
	return inv.Status
}

const invitedPassphrase = "an invited person's passphrase"

func TestIntegrationInvitationRegistration_CreatesTheAccountAndJoinsTheTeam(t *testing.T) {
	ctx := context.Background()
	// invite_only never blocks this: a registration through an invitation is
	// by definition invited.
	stack := newInvitationRegistrationStack(t, openAllowlist{})
	token := stack.invite("grace@integration.test")

	preview, err := stack.svc.Preview(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, "grace@integration.test", preview.Email)
	assert.False(t, preview.AccountExists)
	assert.True(t, preview.PasswordRegistration)

	result, err := stack.svc.Register(ctx, token, invitedPassphrase, "Grace Hopper")

	require.NoError(t, err)
	assert.True(t, result.InvitationAccepted)
	assert.Equal(t, stack.teamID, result.TeamID)
	user, err := NewUserRepository(integrationDB).GetByEmail(ctx, "grace@integration.test")
	require.NoError(t, err)
	assert.Equal(t, result.User.ID, user.ID)
	assert.Equal(t, "Grace Hopper", user.Name)
	assert.Nil(t, user.EmailVerifiedAt, "holding an invitation link does not prove the mailbox")
	assert.Equal(t, 1, stack.events.published, "user.created provisions the private workspace")
	assert.Equal(t, 1, stack.count("SELECT count(*) FROM user_credentials WHERE user_id = $1", user.ID))
	assert.Equal(t, 1, stack.count(
		"SELECT count(*) FROM team_members WHERE team_id = $1 AND user_id = $2 AND role = 'admin'", stack.teamID, user.ID),
		"a member with the invited role")
	assert.Equal(t, models.InvitationStatusAccepted, stack.invitationStatus(token))

	t.Run("the link is spent, and the preview says so with the uniform error", func(t *testing.T) {
		_, err := stack.svc.Register(ctx, token, invitedPassphrase, "")
		require.ErrorIs(t, err, services.ErrInvitationUnusable)
		_, err = stack.svc.Preview(ctx, token)
		require.ErrorIs(t, err, services.ErrInvitationUnusable)
	})

	t.Run("a second invitation for the same person: sign in instead", func(t *testing.T) {
		again := stack.invite("grace@integration.test")

		preview, err := stack.svc.Preview(ctx, again)
		require.NoError(t, err)
		assert.True(t, preview.AccountExists)
		assert.False(t, preview.PasswordRegistration)
		_, err = stack.svc.Register(ctx, again, invitedPassphrase, "")
		require.ErrorIs(t, err, services.ErrInvitationAccountExists)
		assert.Equal(t, models.InvitationStatusPending, stack.invitationStatus(again),
			"still acceptable from inside the app")
	})
}

// An allowlist tightened after the invitation was sent refuses the
// registration and leaves nothing behind.
func TestIntegrationInvitationRegistration_AllowlistTightenedAfterTheInvite(t *testing.T) {
	ctx := context.Background()
	stack := newInvitationRegistrationStack(t, denyingAllowlist{admitted: map[string]bool{"inside@integration.test": true}})
	outside := stack.invite("outside@integration.test")
	usersBefore := stack.count("SELECT count(*) FROM users")

	_, err := stack.svc.Register(ctx, outside, invitedPassphrase, "")

	require.ErrorIs(t, err, services.ErrInvitationEmailNotAllowed)
	assert.Equal(t, usersBefore, stack.count("SELECT count(*) FROM users"), "no account")
	assert.Zero(t, stack.count("SELECT count(*) FROM user_credentials"))
	assert.Zero(t, stack.events.published)
	assert.Equal(t, models.InvitationStatusPending, stack.invitationStatus(outside))

	inside := stack.invite("inside@integration.test")
	_, err = stack.svc.Register(ctx, inside, invitedPassphrase, "")
	require.NoError(t, err, "an admitted address still registers")
}

// A failure after the account row is written removes it, so the same link
// works on the next attempt.
func TestIntegrationInvitationRegistration_AFailedProvisioningLeavesTheLinkUsable(t *testing.T) {
	ctx := context.Background()
	stack := newInvitationRegistrationStack(t, openAllowlist{})
	token := stack.invite("retry@integration.test")
	stack.events.err = errors.New("bus down")

	_, err := stack.svc.Register(ctx, token, invitedPassphrase, "")

	require.Error(t, err)
	_, lookupErr := NewUserRepository(integrationDB).GetByEmail(ctx, "retry@integration.test")
	require.ErrorIs(t, lookupErr, repositories.ErrUserNotFound, "the half-made account is gone")
	assert.Equal(t, models.InvitationStatusPending, stack.invitationStatus(token))

	stack.events.err = nil
	result, err := stack.svc.Register(ctx, token, invitedPassphrase, "")
	require.NoError(t, err)
	assert.True(t, result.InvitationAccepted)
}

// Two people (or one double click) using one link at the same moment: one
// account, one membership.
func TestIntegrationInvitationRegistration_SimultaneousRegistrationsCreateOneAccount(t *testing.T) {
	ctx := context.Background()
	stack := newInvitationRegistrationStack(t, openAllowlist{})

	for round := range 5 {
		email := "race" + uuid.NewString()[:8] + "@integration.test"
		token := stack.invite(email)
		const callers = 6
		errs := make([]error, callers)
		var wg sync.WaitGroup
		for i := range callers {
			wg.Go(func() { _, errs[i] = stack.svc.Register(ctx, token, invitedPassphrase, "") })
		}
		wg.Wait()

		won := 0
		for _, err := range errs {
			switch {
			case err == nil:
				won++
			case errors.Is(err, services.ErrInvitationAccountExists), errors.Is(err, services.ErrInvitationUnusable):
				// The loser saw the winner's account, or the spent link.
			default:
				t.Errorf("round %d: unexpected error: %v", round, err)
			}
		}
		assert.Equal(t, 1, won, "round %d", round)
		assert.Equal(t, 1, stack.count("SELECT count(*) FROM users WHERE email = $1", email))
		assert.Equal(t, 1, stack.count(
			"SELECT count(*) FROM team_members m JOIN users u ON u.id = m.user_id WHERE u.email = $1", email))
		assert.Equal(t, models.InvitationStatusAccepted, stack.invitationStatus(token))
	}
}
