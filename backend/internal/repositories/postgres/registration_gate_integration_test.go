//go:build integration

package postgres

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// Invite-only registration against real invitations (#1434): the real
// AuthService and registration gate over the real user, identity and invitation
// repositories. What counts as "invited" is decided by a SQL predicate (pending
// and unexpired), which mocks cannot exercise.

const gateTestRootAdmin = "root@corp.example"

// inviteOnlyPolicy is an instance switched to invite_only.
type inviteOnlyPolicy struct{ services.AuthPolicyResolver }

func (inviteOnlyPolicy) Policy(context.Context) (models.InstanceAuthPolicy, error) {
	return models.InstanceAuthPolicy{RegistrationMode: models.RegistrationModeInviteOnly}, nil
}

func newInviteOnlyAuthService() *services.AuthService {
	logger := slog.New(slog.DiscardHandler)
	gate := services.NewRegistrationGate(services.RegistrationGateDeps{
		Policy:      inviteOnlyPolicy{},
		Invitations: NewTeamInvitationRepository(integrationDB),
		RootAdmins:  services.NewInstanceAdminService([]string{gateTestRootAdmin}, nil, nil, logger),
		Logger:      logger,
	})
	return services.NewAuthService(
		NewUserRepository(integrationDB), NewUserIdentityRepository(integrationDB),
		nil, nil, logger, openAllowlist{}, services.WithRegistrationGate(gate))
}

// gateTeam returns a team to invite into, owned by a fresh user.
func gateTeam(t *testing.T) (teamID, ownerID string) {
	t.Helper()
	ownerID = insertTestUser(t)
	return insertTestTeam(t, ownerID), ownerID
}

// invite stores an invitation the way the service does (through the
// repository), then forces its status and expiry.
func invite(t *testing.T, teamID, inviterID, email string, status models.InvitationStatus, expiresIn time.Duration) {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	inv := &models.TeamInvitation{
		TeamID: teamID, InviterID: inviterID, InviteeEmail: email, Role: models.TeamMemberRoleMember,
		Token: uuid.New().String(), Status: models.InvitationStatusPending,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, NewTeamInvitationRepository(integrationDB).Create(ctx, inv))
	_, err := integrationDB.ExecContext(ctx,
		"UPDATE team_invitations SET status = $1, expires_at = $2 WHERE id = $3",
		status, now.Add(expiresIn), inv.ID)
	require.NoError(t, err)
}

func userCount(t *testing.T) int {
	t.Helper()
	return countRows(t, integrationDB.DB, "SELECT count(*) FROM users")
}

func TestIntegrationRegistrationGate_InviteOnly(t *testing.T) {
	resetSearchTables(t)
	ctx := context.Background()
	svc := newInviteOnlyAuthService()
	teamID, ownerID := gateTeam(t)
	before := userCount(t)
	claimsFor := func(email string, verified bool) *idp.Claims {
		return &idp.Claims{Subject: "sub-" + uuid.New().String(), Email: email, Name: "N", EmailVerified: verified}
	}

	t.Run("an uninvited newcomer is refused and no row is created", func(t *testing.T) {
		user, err := svc.ProvisionFromClaims(ctx, "corp-sso", claimsFor("stranger@example.com", true))

		require.ErrorIs(t, err, services.ErrRegistrationClosed)
		assert.Nil(t, user)
		assert.Equal(t, before, userCount(t))
		assert.Equal(t, 0, countRows(t, integrationDB.DB, "SELECT count(*) FROM user_identities"))
	})

	t.Run("a revoked, declined, accepted or expired invitation admits nobody", func(t *testing.T) {
		invite(t, teamID, ownerID, "revoked@example.com", models.InvitationStatusRevoked, time.Hour)
		invite(t, teamID, ownerID, "declined@example.com", models.InvitationStatusRejected, time.Hour)
		invite(t, teamID, ownerID, "accepted@example.com", models.InvitationStatusAccepted, time.Hour)
		invite(t, teamID, ownerID, "expired@example.com", models.InvitationStatusPending, -time.Minute)

		for _, email := range []string{
			"revoked@example.com", "declined@example.com", "accepted@example.com", "expired@example.com",
		} {
			_, err := svc.ProvisionFromClaims(ctx, "corp-sso", claimsFor(email, true))
			require.ErrorIs(t, err, services.ErrRegistrationClosed, email)
		}
		assert.Equal(t, before, userCount(t))
	})

	t.Run("a pending invitation admits its address, whatever casing was typed", func(t *testing.T) {
		invite(t, teamID, ownerID, "Invited.Person@Example.com", models.InvitationStatusPending, time.Hour)

		_, err := svc.ProvisionFromClaims(ctx, "corp-sso", claimsFor("invited.person@example.com", false))
		require.ErrorIs(t, err, services.ErrRegistrationClosed, "an unverified claim of an invited address is refused")

		user, err := svc.ProvisionFromClaims(ctx, "corp-sso", claimsFor("INVITED.Person@example.com", true))

		require.NoError(t, err)
		assert.Equal(t, "invited.person@example.com", user.Email)
		assert.Equal(t, before+1, userCount(t))
		assert.Equal(t, 1, countRows(t, integrationDB.DB,
			"SELECT count(*) FROM team_invitations WHERE invitee_email = 'invited.person@example.com' AND status = $1",
			models.InvitationStatusPending), "the invitation is not consumed by registering")
	})

	t.Run("an admin-created account signs in: the mode does not apply to it", func(t *testing.T) {
		precreated := &models.User{Email: "precreated@example.com", Name: "Pre"}
		require.NoError(t, NewUserRepository(integrationDB).Create(ctx, precreated))

		user, err := svc.ProvisionFromClaims(ctx, "corp-sso", claimsFor("precreated@example.com", true))

		require.NoError(t, err)
		assert.Equal(t, precreated.ID, user.ID)
	})

	t.Run("a root instance admin can always create their account", func(t *testing.T) {
		_, err := svc.ProvisionFromClaims(ctx, "corp-sso", claimsFor(gateTestRootAdmin, false))
		require.ErrorIs(t, err, services.ErrRegistrationClosed, "but not on an unverified claim of the address")

		user, err := svc.ProvisionFromClaims(ctx, "corp-sso", claimsFor("Root@Corp.example", true))

		require.NoError(t, err)
		assert.Equal(t, gateTestRootAdmin, user.Email)
	})

	t.Run("a returning user is unaffected", func(t *testing.T) {
		claims := claimsFor("invited.person@example.com", true)
		claims.Subject = "sub-returning"
		// Linked while invited; from now on resolved by identity.
		first, err := svc.ProvisionFromClaims(ctx, "second-sso", claims)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM team_invitations")
		require.NoError(t, err)

		again, err := svc.ProvisionFromClaims(ctx, "second-sso", claims)

		require.NoError(t, err, "no invitation is needed once the account exists")
		assert.Equal(t, first.ID, again.ID)
	})
}

// Invitation addresses are stored normalized and found whatever casing is asked
// for (#1434).
func TestIntegrationTeamInvitation_InviteeEmailIsNormalized(t *testing.T) {
	resetSearchTables(t)
	ctx := context.Background()
	repo := NewTeamInvitationRepository(integrationDB)
	teamID, ownerID := gateTeam(t)
	now := time.Now()
	inv := &models.TeamInvitation{
		TeamID: teamID, InviterID: ownerID, InviteeEmail: "  Bob@Example.COM ", Role: models.TeamMemberRoleMember,
		Token: uuid.New().String(), Status: models.InvitationStatusPending,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	}

	require.NoError(t, repo.Create(ctx, inv))

	assert.Equal(t, "bob@example.com", inv.InviteeEmail, "the model carries what was stored")
	for _, spelling := range []string{"bob@example.com", "BOB@EXAMPLE.COM", " Bob@Example.com "} {
		got, err := repo.GetPendingByEmail(ctx, spelling)
		require.NoError(t, err, spelling)
		require.Len(t, got, 1, spelling)
		assert.Equal(t, "bob@example.com", got[0].InviteeEmail)
	}

	_, err := integrationDB.ExecContext(ctx,
		`INSERT INTO team_invitations (team_id, inviter_id, invitee_email, role, token, status, expires_at)
		 VALUES ($1, $2, 'Raw@Example.com', 'member', $3, 'pending', now() + interval '1 day')`,
		teamID, ownerID, uuid.New().String())
	requirePQCode(t, err, pqCheckViolation)
}
