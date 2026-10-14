//go:build integration

package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// TeamInvitationRepository.Accept (#1435): the member row and the status
// change are one transaction, so neither exists without the other.

// acceptFixture is one team with an invitation in the given state.
type acceptFixture struct {
	t        *testing.T
	repo     repositories.TeamInvitationRepository
	teamID   string
	inviteID string
}

func newAcceptFixture(t *testing.T, status models.InvitationStatus, expiresIn time.Duration) *acceptFixture {
	t.Helper()
	resetSearchTables(t)
	teamID, ownerID := gateTeam(t)
	inv := &models.TeamInvitation{
		TeamID: teamID, InviterID: ownerID, InviteeEmail: "invitee@integration.test",
		Role: models.TeamMemberRoleAdmin, Token: uuid.NewString(), Status: status,
		ExpiresAt: time.Now().Add(expiresIn), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo := NewTeamInvitationRepository(integrationDB)
	require.NoError(t, repo.Create(context.Background(), inv))
	return &acceptFixture{t: t, repo: repo, teamID: teamID, inviteID: inv.ID}
}

func (f *acceptFixture) member(userID string) *models.TeamMember {
	now := time.Now()
	return &models.TeamMember{
		TeamID: f.teamID, UserID: userID, Role: models.TeamMemberRoleAdmin, CreatedAt: now, UpdatedAt: now,
	}
}

func (f *acceptFixture) status() models.InvitationStatus {
	f.t.Helper()
	inv, err := f.repo.GetByID(context.Background(), f.inviteID)
	require.NoError(f.t, err)
	return inv.Status
}

func (f *acceptFixture) isMember(userID string) bool {
	f.t.Helper()
	var n int
	require.NoError(f.t, integrationDB.QueryRowContext(context.Background(),
		"SELECT count(*) FROM team_members WHERE team_id = $1 AND user_id = $2", f.teamID, userID).Scan(&n))
	return n == 1
}

func TestIntegrationTeamInvitation_Accept(t *testing.T) {
	ctx := context.Background()

	t.Run("a pending invitation is accepted and the member created with its role", func(t *testing.T) {
		f := newAcceptFixture(t, models.InvitationStatusPending, time.Hour)
		userID := insertTestUser(t)
		member := f.member(userID)

		require.NoError(t, f.repo.Accept(ctx, f.inviteID, member))

		assert.Equal(t, models.InvitationStatusAccepted, f.status())
		assert.True(t, f.isMember(userID))
		assert.NotEmpty(t, member.ID, "the member's id is filled")
	})

	for name, tc := range map[string]struct {
		status    models.InvitationStatus
		expiresIn time.Duration
	}{
		"expired":          {models.InvitationStatusPending, -time.Minute},
		"revoked":          {models.InvitationStatusRevoked, time.Hour},
		"rejected":         {models.InvitationStatusRejected, time.Hour},
		"already accepted": {models.InvitationStatusAccepted, time.Hour},
	} {
		t.Run("not pending, nothing is written: "+name, func(t *testing.T) {
			f := newAcceptFixture(t, tc.status, tc.expiresIn)
			userID := insertTestUser(t)

			err := f.repo.Accept(ctx, f.inviteID, f.member(userID))

			require.ErrorIs(t, err, repositories.ErrTeamInvitationNotPending)
			assert.Equal(t, tc.status, f.status())
			assert.False(t, f.isMember(userID))
		})
	}

	t.Run("an unknown invitation is not pending", func(t *testing.T) {
		f := newAcceptFixture(t, models.InvitationStatusPending, time.Hour)

		err := f.repo.Accept(ctx, uuid.NewString(), f.member(insertTestUser(t)))

		require.ErrorIs(t, err, repositories.ErrTeamInvitationNotPending)
	})

	t.Run("an existing member: the status change is rolled back", func(t *testing.T) {
		f := newAcceptFixture(t, models.InvitationStatusPending, time.Hour)
		userID := insertTestUser(t)
		insertLockTeamMember(t, f.teamID, userID, models.TeamMemberRoleMember)

		err := f.repo.Accept(ctx, f.inviteID, f.member(userID))

		require.ErrorIs(t, err, repositories.ErrTeamMemberExists)
		assert.Equal(t, models.InvitationStatusPending, f.status(), "the invitation stays pending")
	})

	t.Run("a member that cannot be inserted: the status change is rolled back", func(t *testing.T) {
		f := newAcceptFixture(t, models.InvitationStatusPending, time.Hour)

		// No such user: the insert breaks the foreign key.
		err := f.repo.Accept(ctx, f.inviteID, f.member(uuid.NewString()))

		require.Error(t, err)
		require.False(t, errors.Is(err, repositories.ErrTeamMemberExists))
		assert.Equal(t, models.InvitationStatusPending, f.status(), "before #1435 this left a member-less accepted invitation or a pending one with a member")
	})

	t.Run("of simultaneous accepts, one wins", func(t *testing.T) {
		f := newAcceptFixture(t, models.InvitationStatusPending, time.Hour)
		const callers = 12
		users := make([]string, callers)
		for i := range users {
			users[i] = insertTestUser(t)
		}
		errs := make([]error, callers)
		var wg sync.WaitGroup
		for i := range users {
			wg.Go(func() { errs[i] = f.repo.Accept(ctx, f.inviteID, f.member(users[i])) })
		}
		wg.Wait()

		won := 0
		for i, err := range errs {
			if err == nil {
				won++
				assert.True(t, f.isMember(users[i]))
				continue
			}
			require.ErrorIs(t, err, repositories.ErrTeamInvitationNotPending)
			assert.False(t, f.isMember(users[i]))
		}
		assert.Equal(t, 1, won)
		assert.Equal(t, models.InvitationStatusAccepted, f.status())
	})
}
