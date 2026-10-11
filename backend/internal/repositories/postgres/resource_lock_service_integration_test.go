//go:build integration

package postgres

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// The lock rules end to end (#1288): the real ResourceLockService and the real
// AuthorizationService over the real repositories, against real Postgres. The
// service unit tests pin the rules on mocks; these pin that the rules hold on
// the statements the repositories actually run -- above all that two
// concurrent lock requests leave exactly one lock and exactly one history
// event, with nothing in the service to arbitrate between them.

func newIntegrationLockService() *services.ResourceLockService {
	logger := slog.New(slog.DiscardHandler)
	return services.NewResourceLockService(services.ResourceLockServiceDeps{
		Prompts:    NewPromptRepository(integrationDB),
		Blueprints: NewBlueprintRepository(integrationDB),
		Artifacts:  NewArtifactRepository(integrationDB),
		Events:     NewResourceLockEventRepository(integrationDB),
		Authorizer: services.NewAuthorizationService(NewTeamMemberRepository(integrationDB), logger),
		Logger:     logger,
	})
}

// insertLockTeamMember makes userID a member of the team with the given role.
func insertLockTeamMember(t *testing.T, teamID, userID string, role models.TeamMemberRole) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)", teamID, userID, string(role))
	require.NoError(t, err)
}

// lockIsHeld reads lock state straight off the row. storedLock cannot answer
// for an unlocked row: it scans the NULL columns into strings.
func lockIsHeld(t *testing.T, table, id string) bool {
	t.Helper()
	var held bool
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT locked_by IS NOT NULL FROM "+table+" WHERE id = $1", id).Scan(&held))
	return held
}

// lockHistoryActions returns the actions of a resource's history, newest first.
func lockHistoryActions(
	t *testing.T, svc *services.ResourceLockService, userID string, s lockScope, resourceType, id string,
) []string {
	t.Helper()
	events, err := svc.History(context.Background(), userID, s.teamID, resourceType, id)
	require.NoError(t, err)
	actions := make([]string, 0, len(events))
	for _, event := range events {
		actions = append(actions, event.Action)
	}
	return actions
}

// Two members ask to lock the same unlocked resource at the same moment.
// Exactly one request succeeds, the other is told the resource is already
// locked, and the history holds one lock event: the winner's.
func TestIntegrationResourceLockService_ConcurrentLockHasExactlyOneWinner(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationLockService()
	for _, target := range lockTargets() {
		t.Run(target.resourceType, func(t *testing.T) {
			s := newLockScope(t)
			insertLockTeamMember(t, s.teamID, s.userID, models.TeamMemberRoleOwner)
			second := insertTestUser(t)
			insertLockTeamMember(t, s.teamID, second, models.TeamMemberRoleMember)
			lockers := []string{s.userID, second}

			// Repeated on fresh rows: a single race can be won by luck.
			for round := 0; round < 20; round++ {
				row := target.insert(t, s)
				errs := make([]error, len(lockers))
				start := make(chan struct{})
				var wg sync.WaitGroup
				for i, locker := range lockers {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						errs[i] = svc.Lock(ctx, locker, s.teamID, target.resourceType, row.id, "by "+locker)
					}()
				}
				close(start)
				wg.Wait()

				winner := -1
				for i, err := range errs {
					if err == nil {
						require.Equal(t, -1, winner, "round %d: both requests succeeded", round)
						winner = i
						continue
					}
					require.ErrorIs(t, err, repositories.ErrResourceAlreadyLocked, "round %d", round)
				}
				require.NotEqual(t, -1, winner, "round %d: neither request succeeded", round)

				stored := target.storedLock(t, row.id)
				assert.Equal(t, lockers[winner], stored.LockedBy, "round %d", round)

				events, err := svc.History(ctx, s.userID, s.teamID, target.resourceType, row.id)
				require.NoError(t, err)
				require.Len(t, events, 1, "round %d: the losing request must leave no history event", round)
				require.NotNil(t, events[0].ActorUserID)
				assert.Equal(t, lockers[winner], *events[0].ActorUserID, "round %d", round)
				require.NotNil(t, events[0].Comment)
				assert.Equal(t, "by "+lockers[winner], *events[0].Comment, "round %d", round)
			}
		})
	}
}

// One resource through its whole life: a member locks it, another member may
// not release it, an admin may, and the history tells the story newest first
// with each actor named.
func TestIntegrationResourceLockService_LockUnlockLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := newIntegrationLockService()
	for _, target := range lockTargets() {
		t.Run(target.resourceType, func(t *testing.T) {
			s := newLockScope(t)
			insertLockTeamMember(t, s.teamID, s.userID, models.TeamMemberRoleOwner)
			locker, otherMember, admin := insertTestUser(t), insertTestUser(t), insertTestUser(t)
			insertLockTeamMember(t, s.teamID, locker, models.TeamMemberRoleMember)
			insertLockTeamMember(t, s.teamID, otherMember, models.TeamMemberRoleMember)
			insertLockTeamMember(t, s.teamID, admin, models.TeamMemberRoleAdmin)
			outsider := insertTestUser(t)
			row := target.insert(t, s)

			require.ErrorIs(t,
				svc.Lock(ctx, outsider, s.teamID, target.resourceType, row.id, "not my team"),
				services.ErrPermissionDenied)
			require.ErrorIs(t,
				svc.Unlock(ctx, locker, s.teamID, target.resourceType, row.id),
				repositories.ErrResourceNotLocked, "nothing to unlock yet")

			require.NoError(t, svc.Lock(ctx, locker, s.teamID, target.resourceType, row.id, "  in use  "))
			stored := target.storedLock(t, row.id)
			assert.Equal(t, locker, stored.LockedBy)
			assert.Equal(t, "in use", stored.Comment, "the stored comment is trimmed")

			require.ErrorIs(t,
				svc.Lock(ctx, admin, s.teamID, target.resourceType, row.id, "mine now"),
				repositories.ErrResourceAlreadyLocked, "even an admin cannot lock over a lock")
			require.ErrorIs(t,
				svc.Unlock(ctx, otherMember, s.teamID, target.resourceType, row.id),
				services.ErrPermissionDenied, "a member cannot release another member's lock")
			assert.Equal(t, locker, target.storedLock(t, row.id).LockedBy, "the refused unlock changed nothing")

			require.NoError(t, svc.Unlock(ctx, admin, s.teamID, target.resourceType, row.id))
			assert.False(t, lockIsHeld(t, target.table, row.id), "the admin's unlock released the lock")

			require.NoError(t, svc.Lock(ctx, otherMember, s.teamID, target.resourceType, row.id, "second lock"))
			require.NoError(t, svc.Unlock(ctx, otherMember, s.teamID, target.resourceType, row.id),
				"the locker releases their own lock")

			// Refused and conflicting requests left no trace: four events for
			// four successful changes, newest first.
			assert.Equal(t,
				[]string{models.LockActionUnlock, models.LockActionLock, models.LockActionUnlock, models.LockActionLock},
				lockHistoryActions(t, svc, s.userID, s, target.resourceType, row.id))

			events, err := svc.History(ctx, otherMember, s.teamID, target.resourceType, row.id)
			require.NoError(t, err)
			oldest := events[len(events)-1]
			require.NotNil(t, oldest.ActorUserID)
			assert.Equal(t, locker, *oldest.ActorUserID)
			assert.NotEmpty(t, oldest.ActorName, "the actor's display name is resolved")
			require.NotNil(t, oldest.Comment)
			assert.Equal(t, "in use", *oldest.Comment)
			assert.Nil(t, events[len(events)-2].Comment, "an unlock event carries no comment")

			_, err = svc.History(ctx, outsider, s.teamID, target.resourceType, row.id)
			require.ErrorIs(t, err, services.ErrPermissionDenied)
		})
	}
}
