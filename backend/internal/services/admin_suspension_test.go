package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	repomocks "github.com/vibexp/vibexp/internal/repositories/mocks"
)

const (
	suspendActingAdmin = "admin-1"
	suspendTarget      = "user-2"
)

func suspensionTarget(id, email, status string) *models.AdminUserDetail {
	return &models.AdminUserDetail{ID: id, Email: email, Status: status}
}

// noInstanceAdmins is the predicate for "the config allowlist is empty".
func noInstanceAdmins(string) bool { return false }

// TestSuspendUser_Success covers the happy path and pins that the service
// re-reads the row after the write, so the caller sees PERSISTED state rather
// than an optimistic local edit.
func TestSuspendUser_Success(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetUserDetail", mock.Anything, suspendTarget).
		Return(suspensionTarget(suspendTarget, "target@example.com", models.UserStatusActive), nil).Once()
	repo.On("UpdateUserStatus", mock.Anything, suspendTarget, models.UserStatusSuspended).
		Return(true, nil)
	repo.On("GetUserDetail", mock.Anything, suspendTarget).
		Return(suspensionTarget(suspendTarget, "target@example.com", models.UserStatusSuspended), nil).Once()

	got, err := NewAdminService(repo).SuspendUser(
		context.Background(), suspendActingAdmin, suspendTarget, noInstanceAdmins,
	)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, models.UserStatusSuspended, got.Status)
}

// TestSuspendUser_LockoutGuards is the security core of this issue: neither
// guard may be bypassable, and NEITHER may reach the write.
func TestSuspendUser_LockoutGuards(t *testing.T) {
	tests := []struct {
		name      string
		actingID  string
		target    *models.AdminUserDetail
		predicate InstanceAdminPredicate
		assertErr func(*testing.T, error)
	}{
		{
			name:      "acting admin cannot suspend themselves",
			actingID:  suspendTarget, // same id as the target
			target:    suspensionTarget(suspendTarget, "me@example.com", models.UserStatusActive),
			predicate: noInstanceAdmins,
			assertErr: func(t *testing.T, err error) {
				var e *ErrAdminSuspendSelf
				require.ErrorAs(t, err, &e)
			},
		},
		{
			name:     "config-listed instance admin cannot be suspended",
			actingID: suspendActingAdmin,
			target:   suspensionTarget(suspendTarget, "root@example.com", models.UserStatusActive),
			predicate: func(email string) bool {
				return email == "root@example.com"
			},
			assertErr: func(t *testing.T, err error) {
				var e *ErrAdminSuspendInstanceAdmin
				require.ErrorAs(t, err, &e)
				assert.Equal(t, "root@example.com", e.Email)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := repomocks.NewMockAdminRepository(t)
			// Only the read is expected. UpdateUserStatus has NO expectation, so if
			// a guard ever stopped short-circuiting, this test fails loudly rather
			// than silently suspending the account.
			repo.On("GetUserDetail", mock.Anything, suspendTarget).Return(tc.target, nil)

			got, err := NewAdminService(repo).SuspendUser(
				context.Background(), tc.actingID, suspendTarget, tc.predicate,
			)
			require.Nil(t, got)
			tc.assertErr(t, err)
		})
	}
}

// TestSuspendUser_NilPredicateStillGuardsSelf proves a missing predicate
// degrades to "no config-admin protection" rather than panicking, and that the
// self-guard is independent of it.
func TestSuspendUser_NilPredicateStillGuardsSelf(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetUserDetail", mock.Anything, suspendTarget).
		Return(suspensionTarget(suspendTarget, "me@example.com", models.UserStatusActive), nil)

	_, err := NewAdminService(repo).SuspendUser(
		context.Background(), suspendTarget, suspendTarget, nil,
	)
	var e *ErrAdminSuspendSelf
	require.ErrorAs(t, err, &e)
}

// TestSuspendUser_UnknownTarget returns (nil, nil) so the handler can 404.
func TestSuspendUser_UnknownTarget(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetUserDetail", mock.Anything, suspendTarget).Return(nil, nil)

	got, err := NewAdminService(repo).SuspendUser(
		context.Background(), suspendActingAdmin, suspendTarget, noInstanceAdmins,
	)
	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestSuspendUser_VanishedBetweenReadAndWrite covers the race: the row existed
// at the read and not at the write. That is a 404, not a 500.
func TestSuspendUser_VanishedBetweenReadAndWrite(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetUserDetail", mock.Anything, suspendTarget).
		Return(suspensionTarget(suspendTarget, "target@example.com", models.UserStatusActive), nil)
	repo.On("UpdateUserStatus", mock.Anything, suspendTarget, models.UserStatusSuspended).
		Return(false, nil)

	got, err := NewAdminService(repo).SuspendUser(
		context.Background(), suspendActingAdmin, suspendTarget, noInstanceAdmins,
	)
	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestSuspendUser_RepositoryErrors propagate rather than being swallowed.
func TestSuspendUser_RepositoryErrors(t *testing.T) {
	t.Run("read fails", func(t *testing.T) {
		repo := repomocks.NewMockAdminRepository(t)
		repo.On("GetUserDetail", mock.Anything, suspendTarget).Return(nil, errors.New("boom"))

		_, err := NewAdminService(repo).SuspendUser(
			context.Background(), suspendActingAdmin, suspendTarget, noInstanceAdmins,
		)
		require.Error(t, err)
	})

	t.Run("write fails", func(t *testing.T) {
		repo := repomocks.NewMockAdminRepository(t)
		repo.On("GetUserDetail", mock.Anything, suspendTarget).
			Return(suspensionTarget(suspendTarget, "t@example.com", models.UserStatusActive), nil)
		repo.On("UpdateUserStatus", mock.Anything, suspendTarget, models.UserStatusSuspended).
			Return(false, errors.New("boom"))

		_, err := NewAdminService(repo).SuspendUser(
			context.Background(), suspendActingAdmin, suspendTarget, noInstanceAdmins,
		)
		require.Error(t, err)
	})
}

// TestReactivateUser_Success restores the account. Deliberately NO self/admin
// guard: granting access back can never lock an instance out, and self-recovery
// is a feature.
func TestReactivateUser_Success(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetUserDetail", mock.Anything, suspendTarget).
		Return(suspensionTarget(suspendTarget, "t@example.com", models.UserStatusSuspended), nil).Once()
	repo.On("UpdateUserStatus", mock.Anything, suspendTarget, models.UserStatusActive).Return(true, nil)
	repo.On("GetUserDetail", mock.Anything, suspendTarget).
		Return(suspensionTarget(suspendTarget, "t@example.com", models.UserStatusActive), nil).Once()

	got, err := NewAdminService(repo).ReactivateUser(context.Background(), suspendTarget)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, models.UserStatusActive, got.Status)
}

// TestReactivateUser_SelfIsAllowed is the explicit counterpart to the suspend
// guard — an admin reactivating their own account is the recovery path.
func TestReactivateUser_SelfIsAllowed(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetUserDetail", mock.Anything, suspendActingAdmin).
		Return(suspensionTarget(suspendActingAdmin, "me@example.com", models.UserStatusSuspended), nil).Once()
	repo.On("UpdateUserStatus", mock.Anything, suspendActingAdmin, models.UserStatusActive).Return(true, nil)
	repo.On("GetUserDetail", mock.Anything, suspendActingAdmin).
		Return(suspensionTarget(suspendActingAdmin, "me@example.com", models.UserStatusActive), nil).Once()

	got, err := NewAdminService(repo).ReactivateUser(context.Background(), suspendActingAdmin)
	require.NoError(t, err)
	assert.Equal(t, models.UserStatusActive, got.Status)
}

// TestReactivateUser_UnknownTarget → (nil, nil) for a 404.
func TestReactivateUser_UnknownTarget(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetUserDetail", mock.Anything, suspendTarget).Return(nil, nil)

	got, err := NewAdminService(repo).ReactivateUser(context.Background(), suspendTarget)
	require.NoError(t, err)
	assert.Nil(t, got)
}
