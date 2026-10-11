package services

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

const (
	lockTestTeamID     = "team-lock"
	lockTestUserID     = "user-caller"
	lockTestOtherID    = "user-other"
	lockTestResourceID = "resource-1"
)

// lockHarness runs the REAL authorization service over a team-member mock, so
// a test states the caller's role and the who-may-unlock table is asserted on
// the production permission matrix rather than on a double of it.
type lockHarness struct {
	svc        *ResourceLockService
	members    *repomocks.MockTeamMemberRepository
	prompts    *repomocks.MockPromptRepository
	blueprints *repomocks.MockBlueprintRepository
	artifacts  *repomocks.MockArtifactRepository
	events     *repomocks.MockResourceLockEventRepository
	// recorded collects every history event that reached the repository.
	recorded []*models.ResourceLockEvent
}

func newLockHarness(t *testing.T) *lockHarness {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	h := &lockHarness{
		members:    repomocks.NewMockTeamMemberRepository(t),
		prompts:    repomocks.NewMockPromptRepository(t),
		blueprints: repomocks.NewMockBlueprintRepository(t),
		artifacts:  repomocks.NewMockArtifactRepository(t),
		events:     repomocks.NewMockResourceLockEventRepository(t),
	}
	h.svc = NewResourceLockService(ResourceLockServiceDeps{
		Prompts:    h.prompts,
		Blueprints: h.blueprints,
		Artifacts:  h.artifacts,
		Events:     h.events,
		Authorizer: NewAuthorizationService(h.members, logger),
		Logger:     logger,
	})
	return h
}

// as makes the caller a team member with the given role.
func (h *lockHarness) as(role models.TeamMemberRole) *lockHarness {
	h.members.EXPECT().GetByTeamAndUser(mock.Anything, lockTestTeamID, lockTestUserID).
		Return(&models.TeamMember{Role: role}, nil)
	return h
}

// asNonMember makes the caller a stranger to the team.
func (h *lockHarness) asNonMember() *lockHarness {
	h.members.EXPECT().GetByTeamAndUser(mock.Anything, lockTestTeamID, lockTestUserID).
		Return(nil, repositories.ErrTeamMemberNotFound)
	return h
}

// expectEvent lets exactly one history event be stored and captures it.
func (h *lockHarness) expectEvent() {
	h.events.EXPECT().Append(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, event *models.ResourceLockEvent) error {
			h.recorded = append(h.recorded, event)
			return nil
		}).Once()
}

// lockTypeCase adapts one resource type's repository mock to the
// type-agnostic tests below.
type lockTypeCase struct {
	resourceType string
	notFound     error
	expectSet    func(h *lockHarness, comment string, err error)
	expectClear  func(h *lockHarness, err error)
	// expectCurrent makes the repository report the resource with the given
	// lock (nil for unlocked), or fail with err.
	expectCurrent func(h *lockHarness, lock *models.ResourceLock, err error)
}

func lockTypeCases() []lockTypeCase {
	return []lockTypeCase{
		{
			resourceType: models.RelationResourceTypePrompt,
			notFound:     repositories.ErrPromptNotFound,
			expectSet: func(h *lockHarness, comment string, err error) {
				h.prompts.EXPECT().
					SetLock(mock.Anything, lockTestTeamID, lockTestResourceID, lockTestUserID, comment).
					Return(err).Once()
			},
			expectClear: func(h *lockHarness, err error) {
				h.prompts.EXPECT().ClearLock(mock.Anything, lockTestTeamID, lockTestResourceID).Return(err).Once()
			},
			expectCurrent: func(h *lockHarness, lock *models.ResourceLock, err error) {
				call := h.prompts.EXPECT().
					GetByID(mock.Anything, lockTestUserID, lockTestTeamID, lockTestResourceID).Once()
				if err != nil {
					call.Return(nil, err)
					return
				}
				call.Return(&models.Prompt{ID: lockTestResourceID, Lock: lock}, nil)
			},
		},
		{
			resourceType: models.RelationResourceTypeBlueprint,
			notFound:     repositories.ErrBlueprintNotFound,
			expectSet: func(h *lockHarness, comment string, err error) {
				h.blueprints.EXPECT().
					SetLock(mock.Anything, lockTestTeamID, lockTestResourceID, lockTestUserID, comment).
					Return(err).Once()
			},
			expectClear: func(h *lockHarness, err error) {
				h.blueprints.EXPECT().ClearLock(mock.Anything, lockTestTeamID, lockTestResourceID).Return(err).Once()
			},
			expectCurrent: func(h *lockHarness, lock *models.ResourceLock, err error) {
				call := h.blueprints.EXPECT().
					GetByID(mock.Anything, lockTestUserID, lockTestTeamID, lockTestResourceID).Once()
				if err != nil {
					call.Return(nil, err)
					return
				}
				call.Return(&models.Blueprint{ID: lockTestResourceID, Lock: lock}, nil)
			},
		},
		{
			resourceType: models.RelationResourceTypeArtifact,
			notFound:     repositories.ErrArtifactNotFound,
			expectSet: func(h *lockHarness, comment string, err error) {
				h.artifacts.EXPECT().
					SetLock(mock.Anything, lockTestTeamID, lockTestResourceID, lockTestUserID, comment).
					Return(err).Once()
			},
			expectClear: func(h *lockHarness, err error) {
				h.artifacts.EXPECT().ClearLock(mock.Anything, lockTestTeamID, lockTestResourceID).Return(err).Once()
			},
			expectCurrent: func(h *lockHarness, lock *models.ResourceLock, err error) {
				call := h.artifacts.EXPECT().
					GetByID(mock.Anything, lockTestUserID, lockTestTeamID, lockTestResourceID).Once()
				if err != nil {
					call.Return(nil, err)
					return
				}
				call.Return(&models.Artifact{ID: lockTestResourceID, Lock: lock}, nil)
			},
		},
	}
}

func lockHeldBy(userID string) *models.ResourceLock {
	return &models.ResourceLock{LockedBy: userID, Comment: "in use"}
}

func (h *lockHarness) lock(tc lockTypeCase, comment string) error {
	return h.svc.Lock(context.Background(), lockTestUserID, lockTestTeamID, tc.resourceType, lockTestResourceID, comment)
}

func (h *lockHarness) unlock(tc lockTypeCase) error {
	return h.svc.Unlock(context.Background(), lockTestUserID, lockTestTeamID, tc.resourceType, lockTestResourceID)
}

// Any member can lock, whatever their role, and a successful lock appends
// exactly one history event carrying the trimmed comment.
func TestResourceLockService_Lock_AnyMemberLocksAndAppendsOneEvent(t *testing.T) {
	roles := []models.TeamMemberRole{
		models.TeamMemberRoleMember, models.TeamMemberRoleAdmin, models.TeamMemberRoleOwner,
	}
	for _, tc := range lockTypeCases() {
		for _, role := range roles {
			t.Run(tc.resourceType+"/"+string(role), func(t *testing.T) {
				h := newLockHarness(t).as(role)
				tc.expectSet(h, "in use by the release agent", nil)
				h.expectEvent()

				require.NoError(t, h.lock(tc, "  in use by the release agent \n"))

				require.Len(t, h.recorded, 1, "exactly one history event per successful lock")
				event := h.recorded[0]
				assert.Equal(t, lockTestTeamID, event.TeamID)
				assert.Equal(t, tc.resourceType, event.ResourceType)
				assert.Equal(t, lockTestResourceID, event.ResourceID)
				assert.Equal(t, models.LockActionLock, event.Action)
				require.NotNil(t, event.ActorUserID)
				assert.Equal(t, lockTestUserID, *event.ActorUserID)
				require.NotNil(t, event.Comment)
				assert.Equal(t, "in use by the release agent", *event.Comment, "the stored comment is trimmed")
			})
		}
	}
}

// A comment that is empty once trimmed, or longer than the limit, is rejected
// before any write: the repository mocks have no SetLock expectation, so a call
// would fail the test.
func TestResourceLockService_Lock_RejectsBadComments(t *testing.T) {
	cases := map[string]struct {
		comment string
		want    error
	}{
		"empty":           {comment: "", want: ErrLockCommentRequired},
		"whitespace only": {comment: " \t\n ", want: ErrLockCommentRequired},
		"one too long":    {comment: strings.Repeat("a", MaxLockCommentLength+1), want: ErrLockCommentTooLong},
	}
	for _, tc := range lockTypeCases() {
		for name, c := range cases {
			t.Run(tc.resourceType+"/"+name, func(t *testing.T) {
				h := newLockHarness(t).as(models.TeamMemberRoleMember)

				err := h.lock(tc, c.comment)

				require.ErrorIs(t, err, c.want)
				assert.Empty(t, h.recorded, "a rejected lock appends no history event")
			})
		}
	}
}

// The limit counts characters, not bytes, and is measured after trimming: a
// comment of exactly the limit in multi-byte runes, padded with whitespace, is
// accepted.
func TestResourceLockService_Lock_CommentLimitCountsTrimmedCharacters(t *testing.T) {
	tc := lockTypeCases()[0]
	comment := strings.Repeat("é", MaxLockCommentLength)
	require.Greater(t, len(comment), MaxLockCommentLength, "the fixture must be longer than the limit in bytes")

	h := newLockHarness(t).as(models.TeamMemberRoleMember)
	tc.expectSet(h, comment, nil)
	h.expectEvent()

	require.NoError(t, h.lock(tc, "  "+comment+"  "))
}

// A non-member cannot lock, unlock or read the history, and is turned away
// before any repository is touched.
func TestResourceLockService_NonMemberIsDenied(t *testing.T) {
	for _, tc := range lockTypeCases() {
		t.Run(tc.resourceType+"/lock", func(t *testing.T) {
			h := newLockHarness(t).asNonMember()
			require.ErrorIs(t, h.lock(tc, "in use"), ErrPermissionDenied)
		})
		t.Run(tc.resourceType+"/unlock", func(t *testing.T) {
			h := newLockHarness(t).asNonMember()
			require.ErrorIs(t, h.unlock(tc), ErrPermissionDenied)
		})
		t.Run(tc.resourceType+"/history", func(t *testing.T) {
			h := newLockHarness(t).asNonMember()
			_, err := h.svc.History(
				context.Background(), lockTestUserID, lockTestTeamID, tc.resourceType, lockTestResourceID,
			)
			require.ErrorIs(t, err, ErrPermissionDenied)
		})
	}
}

// The repository's verdict on a lock write reaches the caller unchanged, and a
// failed write appends no history event.
func TestResourceLockService_Lock_PassesRepositoryErrorsThrough(t *testing.T) {
	for _, tc := range lockTypeCases() {
		for name, repoErr := range map[string]error{
			"already locked": repositories.ErrResourceAlreadyLocked,
			"not found":      tc.notFound,
		} {
			t.Run(tc.resourceType+"/"+name, func(t *testing.T) {
				h := newLockHarness(t).as(models.TeamMemberRoleMember)
				tc.expectSet(h, "in use", repoErr)

				require.ErrorIs(t, h.lock(tc, "in use"), repoErr)
				assert.Empty(t, h.recorded)
			})
		}
	}
}

// The who-may-unlock table: the locker in any role, and an owner or admin for
// a lock somebody else placed. A plain member cannot release another member's
// lock.
func TestResourceLockService_Unlock_WhoMayUnlock(t *testing.T) {
	cases := []struct {
		name     string
		role     models.TeamMemberRole
		lockedBy string
		allowed  bool
	}{
		{name: "the locker, a member", role: models.TeamMemberRoleMember, lockedBy: lockTestUserID, allowed: true},
		{name: "the locker, an admin", role: models.TeamMemberRoleAdmin, lockedBy: lockTestUserID, allowed: true},
		{name: "another member", role: models.TeamMemberRoleMember, lockedBy: lockTestOtherID, allowed: false},
		{name: "an admin, not the locker", role: models.TeamMemberRoleAdmin, lockedBy: lockTestOtherID, allowed: true},
		{name: "the owner, not the locker", role: models.TeamMemberRoleOwner, lockedBy: lockTestOtherID, allowed: true},
	}
	for _, tc := range lockTypeCases() {
		for _, c := range cases {
			t.Run(tc.resourceType+"/"+c.name, func(t *testing.T) {
				h := newLockHarness(t).as(c.role)
				tc.expectCurrent(h, lockHeldBy(c.lockedBy), nil)
				if c.allowed {
					tc.expectClear(h, nil)
					h.expectEvent()
				}

				err := h.unlock(tc)

				if !c.allowed {
					require.ErrorIs(t, err, ErrPermissionDenied)
					assert.Empty(t, h.recorded, "a refused unlock appends no history event")
					return
				}
				require.NoError(t, err)
				require.Len(t, h.recorded, 1, "exactly one history event per successful unlock")
				event := h.recorded[0]
				assert.Equal(t, models.LockActionUnlock, event.Action)
				assert.Equal(t, tc.resourceType, event.ResourceType)
				require.NotNil(t, event.ActorUserID)
				assert.Equal(t, lockTestUserID, *event.ActorUserID)
				assert.Nil(t, event.Comment, "an unlock event carries no comment")
			})
		}
	}
}

// Unlocking a resource that carries no lock is a conflict, decided from the
// read alone: no ClearLock is attempted and nothing is appended.
func TestResourceLockService_Unlock_NotLockedIsAConflict(t *testing.T) {
	for _, tc := range lockTypeCases() {
		t.Run(tc.resourceType, func(t *testing.T) {
			h := newLockHarness(t).as(models.TeamMemberRoleOwner)
			tc.expectCurrent(h, nil, nil)

			require.ErrorIs(t, h.unlock(tc), repositories.ErrResourceNotLocked)
			assert.Empty(t, h.recorded)
		})
	}
}

// A missing resource, and a lock released by someone else between the read and
// the clear, both reach the caller as the repository reported them.
func TestResourceLockService_Unlock_PassesRepositoryErrorsThrough(t *testing.T) {
	for _, tc := range lockTypeCases() {
		t.Run(tc.resourceType+"/resource not found", func(t *testing.T) {
			h := newLockHarness(t).as(models.TeamMemberRoleMember)
			tc.expectCurrent(h, nil, tc.notFound)

			require.ErrorIs(t, h.unlock(tc), tc.notFound)
		})
		t.Run(tc.resourceType+"/released in between", func(t *testing.T) {
			h := newLockHarness(t).as(models.TeamMemberRoleMember)
			tc.expectCurrent(h, lockHeldBy(lockTestUserID), nil)
			tc.expectClear(h, repositories.ErrResourceNotLocked)

			require.ErrorIs(t, h.unlock(tc), repositories.ErrResourceNotLocked)
			assert.Empty(t, h.recorded)
		})
	}
}

// The lock state has already changed by the time the history insert can fail,
// so a failed insert is logged and the operation still reports success.
func TestResourceLockService_HistoryAppendFailureDoesNotFailTheChange(t *testing.T) {
	tc := lockTypeCases()[0]
	appendFails := func(h *lockHarness) {
		h.events.EXPECT().Append(mock.Anything, mock.Anything).Return(errors.New("insert failed")).Once()
	}

	t.Run("lock", func(t *testing.T) {
		h := newLockHarness(t).as(models.TeamMemberRoleMember)
		tc.expectSet(h, "in use", nil)
		appendFails(h)

		require.NoError(t, h.lock(tc, "in use"))
	})
	t.Run("unlock", func(t *testing.T) {
		h := newLockHarness(t).as(models.TeamMemberRoleMember)
		tc.expectCurrent(h, lockHeldBy(lockTestUserID), nil)
		tc.expectClear(h, nil)
		appendFails(h)

		require.NoError(t, h.unlock(tc))
	})
}

// History reads the newest events of the one resource, capped, for any member.
func TestResourceLockService_History(t *testing.T) {
	for _, tc := range lockTypeCases() {
		t.Run(tc.resourceType, func(t *testing.T) {
			h := newLockHarness(t).as(models.TeamMemberRoleMember)
			want := []*models.ResourceLockEvent{{ID: "event-2"}, {ID: "event-1"}}
			h.events.EXPECT().ListByResource(
				mock.Anything, lockTestTeamID, tc.resourceType, lockTestResourceID, ResourceLockHistoryLimit,
			).Return(want, nil).Once()

			got, err := h.svc.History(
				context.Background(), lockTestUserID, lockTestTeamID, tc.resourceType, lockTestResourceID,
			)

			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}

	t.Run("a repository failure is returned", func(t *testing.T) {
		h := newLockHarness(t).as(models.TeamMemberRoleMember)
		h.events.EXPECT().ListByResource(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errors.New("query failed")).Once()

		_, err := h.svc.History(
			context.Background(), lockTestUserID, lockTestTeamID,
			models.RelationResourceTypePrompt, lockTestResourceID,
		)

		require.ErrorContains(t, err, "failed to list lock history")
	})
}

// Memories are not lockable, and neither is anything outside the relation
// vocabulary. The check precedes the membership lookup: the member mock has no
// expectation here.
func TestResourceLockService_RejectsUnlockableResourceTypes(t *testing.T) {
	for _, resourceType := range []string{models.RelationResourceTypeMemory, "prompts", ""} {
		t.Run(resourceType, func(t *testing.T) {
			h := newLockHarness(t)
			ctx := context.Background()

			require.ErrorIs(t,
				h.svc.Lock(ctx, lockTestUserID, lockTestTeamID, resourceType, lockTestResourceID, "in use"),
				ErrResourceTypeNotLockable)
			require.ErrorIs(t,
				h.svc.Unlock(ctx, lockTestUserID, lockTestTeamID, resourceType, lockTestResourceID),
				ErrResourceTypeNotLockable)
			_, err := h.svc.History(ctx, lockTestUserID, lockTestTeamID, resourceType, lockTestResourceID)
			require.ErrorIs(t, err, ErrResourceTypeNotLockable)
		})
	}
}

// The lockable types of the service and of the model are one set.
func TestResourceLockService_StoresMatchTheLockableTypes(t *testing.T) {
	h := newLockHarness(t)
	for resourceType := range h.svc.stores {
		assert.True(t, models.IsLockableResourceType(resourceType), resourceType)
	}
	assert.Len(t, h.svc.stores, 3)
}

func TestNewResourceLockService_DefaultsTheLogger(t *testing.T) {
	svc := NewResourceLockService(ResourceLockServiceDeps{})
	assert.NotNil(t, svc.logger)
}
