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
	deleteActingAdmin = "admin-1"
	deleteTarget      = "user-2"
)

// deleteTargetDetail builds the target fixture. Only the email varies across
// cases — the id is always the shared deleteTarget constant.
func deleteTargetDetail(email string) *models.AdminUserDetail {
	return &models.AdminUserDetail{ID: deleteTarget, Email: email, Status: models.UserStatusActive}
}

func TestUpdateUserName_Success(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("UpdateUserName", mock.Anything, deleteTarget, "Renamed").Return(true, nil)
	repo.On("GetUserDetail", mock.Anything, deleteTarget).
		Return(&models.AdminUserDetail{ID: deleteTarget, Name: "Renamed"}, nil)

	got, err := NewAdminService(repo).UpdateUserName(context.Background(), deleteTarget, "Renamed")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Renamed", got.Name)
}

func TestUpdateUserName_UnknownTarget(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("UpdateUserName", mock.Anything, deleteTarget, "Renamed").Return(false, nil)

	got, err := NewAdminService(repo).UpdateUserName(context.Background(), deleteTarget, "Renamed")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestUpdateUserName_RepositoryError(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("UpdateUserName", mock.Anything, deleteTarget, "Renamed").
		Return(false, errors.New("boom"))

	_, err := NewAdminService(repo).UpdateUserName(context.Background(), deleteTarget, "Renamed")
	require.Error(t, err)
}

// TestDeleteUser_Success is the allowed path.
func TestDeleteUser_Success(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetUserDetail", mock.Anything, deleteTarget).
		Return(deleteTargetDetail("target@example.com"), nil)
	repo.On("DeleteUserIfUnblocked", mock.Anything, deleteTarget).Return(nil, true, nil)

	email, deleted, err := NewAdminService(repo).DeleteUser(
		context.Background(), deleteActingAdmin, deleteTarget, noInstanceAdmins,
	)
	require.NoError(t, err)
	assert.True(t, deleted)
	// The email must come back so the caller can name the account in its audit
	// row — after the delete there is no row left to read it from.
	assert.Equal(t, "target@example.com", email)
}

// TestDeleteUser_LockoutGuardsNeverReachTheRepository is the load-bearing test
// for the epic's only destructive endpoint.
//
// DeleteUserIfUnblocked has NO expectation here. Because the repository is a
// strict mock, a guard that stopped short-circuiting would call it and fail the
// test — rather than quietly deleting the account and, via
// teams_owner_id_fkey ON DELETE CASCADE, every team the user owns.
func TestDeleteUser_LockoutGuardsNeverReachTheRepository(t *testing.T) {
	tests := []struct {
		name      string
		actingID  string
		target    *models.AdminUserDetail
		predicate InstanceAdminPredicate
		assertErr func(*testing.T, error)
	}{
		{
			name:      "acting admin cannot delete themselves",
			actingID:  deleteTarget,
			target:    deleteTargetDetail("me@example.com"),
			predicate: noInstanceAdmins,
			assertErr: func(t *testing.T, err error) {
				var e *ErrAdminDeleteSelf
				require.ErrorAs(t, err, &e)
			},
		},
		{
			name:     "config-listed instance admin cannot be deleted",
			actingID: deleteActingAdmin,
			target:   deleteTargetDetail("root@example.com"),
			predicate: func(email string) bool {
				return email == "root@example.com"
			},
			assertErr: func(t *testing.T, err error) {
				var e *ErrAdminDeleteInstanceAdmin
				require.ErrorAs(t, err, &e)
				assert.Equal(t, "root@example.com", e.Email)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := repomocks.NewMockAdminRepository(t)
			repo.On("GetUserDetail", mock.Anything, deleteTarget).Return(tc.target, nil)

			_, deleted, err := NewAdminService(repo).DeleteUser(
				context.Background(), tc.actingID, deleteTarget, tc.predicate,
			)
			assert.False(t, deleted)
			tc.assertErr(t, err)
		})
	}
}

// TestDeleteUser_BlockedByOwnedSharedTeams: the repository refuses inside its
// transaction, and the service surfaces the blocker list unchanged so the client
// can name the teams needing an ownership transfer.
func TestDeleteUser_BlockedByOwnedSharedTeams(t *testing.T) {
	blockers := []models.AdminDeleteBlocker{
		{TeamID: "t1", TeamName: "Acme Engineering", MemberCount: 4},
		{TeamID: "t2", TeamName: "Beta Squad", MemberCount: 2},
	}

	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetUserDetail", mock.Anything, deleteTarget).
		Return(deleteTargetDetail("target@example.com"), nil)
	repo.On("DeleteUserIfUnblocked", mock.Anything, deleteTarget).Return(blockers, true, nil)

	email, deleted, err := NewAdminService(repo).DeleteUser(
		context.Background(), deleteActingAdmin, deleteTarget, noInstanceAdmins,
	)
	assert.False(t, deleted, "a blocked delete must report that nothing was deleted")
	assert.Empty(t, email, "a refusal returns no email — there is nothing to audit as deleted")

	var blockedErr *ErrAdminDeleteBlocked
	require.ErrorAs(t, err, &blockedErr)
	assert.Equal(t, blockers, blockedErr.Blockers)
	// The message names the teams, so an operator reading a log knows what to fix.
	assert.Contains(t, blockedErr.Error(), "Acme Engineering")
	assert.Contains(t, blockedErr.Error(), "Beta Squad")
	assert.Contains(t, blockedErr.Error(), "transfer ownership")
}

// TestDeleteUser_UnknownTarget returns (false, nil) so the handler 404s, both
// when the pre-read finds nothing and when the row vanishes before the delete.
func TestDeleteUser_UnknownTarget(t *testing.T) {
	t.Run("absent at the pre-read", func(t *testing.T) {
		repo := repomocks.NewMockAdminRepository(t)
		repo.On("GetUserDetail", mock.Anything, deleteTarget).Return(nil, nil)

		_, deleted, err := NewAdminService(repo).DeleteUser(
			context.Background(), deleteActingAdmin, deleteTarget, noInstanceAdmins,
		)
		require.NoError(t, err)
		assert.False(t, deleted)
	})

	t.Run("vanished before the delete", func(t *testing.T) {
		repo := repomocks.NewMockAdminRepository(t)
		repo.On("GetUserDetail", mock.Anything, deleteTarget).
			Return(deleteTargetDetail("t@example.com"), nil)
		repo.On("DeleteUserIfUnblocked", mock.Anything, deleteTarget).Return(nil, false, nil)

		_, deleted, err := NewAdminService(repo).DeleteUser(
			context.Background(), deleteActingAdmin, deleteTarget, noInstanceAdmins,
		)
		require.NoError(t, err)
		assert.False(t, deleted)
	})
}

// TestDeleteUser_NilPredicateStillGuardsSelf: a missing predicate degrades to
// "no config-admin protection" rather than panicking, and the self-guard is
// independent of it.
func TestDeleteUser_NilPredicateStillGuardsSelf(t *testing.T) {
	repo := repomocks.NewMockAdminRepository(t)
	repo.On("GetUserDetail", mock.Anything, deleteTarget).
		Return(deleteTargetDetail("me@example.com"), nil)

	_, _, err := NewAdminService(repo).DeleteUser(
		context.Background(), deleteTarget, deleteTarget, nil,
	)
	var e *ErrAdminDeleteSelf
	require.ErrorAs(t, err, &e)
}

// TestDeleteUser_RepositoryErrors propagate rather than being reported as a
// successful delete.
func TestDeleteUser_RepositoryErrors(t *testing.T) {
	t.Run("pre-read fails", func(t *testing.T) {
		repo := repomocks.NewMockAdminRepository(t)
		repo.On("GetUserDetail", mock.Anything, deleteTarget).Return(nil, errors.New("boom"))

		_, deleted, err := NewAdminService(repo).DeleteUser(
			context.Background(), deleteActingAdmin, deleteTarget, noInstanceAdmins,
		)
		require.Error(t, err)
		assert.False(t, deleted)
	})

	t.Run("delete fails", func(t *testing.T) {
		repo := repomocks.NewMockAdminRepository(t)
		repo.On("GetUserDetail", mock.Anything, deleteTarget).
			Return(deleteTargetDetail("t@example.com"), nil)
		repo.On("DeleteUserIfUnblocked", mock.Anything, deleteTarget).
			Return(nil, false, errors.New("serialization failure"))

		_, deleted, err := NewAdminService(repo).DeleteUser(
			context.Background(), deleteActingAdmin, deleteTarget, noInstanceAdmins,
		)
		require.Error(t, err)
		assert.False(t, deleted)
	})
}

// TestDeleteGuardErrorMessages pins the refusal messages: each becomes the 409
// detail an operator reads, so they must say WHY the delete was refused and —
// for the blocked case — WHICH teams need transferring.
func TestDeleteGuardErrorMessages(t *testing.T) {
	selfErr := &ErrAdminDeleteSelf{}
	assert.Contains(t, selfErr.Error(), "cannot delete their own account")

	adminErr := &ErrAdminDeleteInstanceAdmin{Email: "root@example.com"}
	assert.Contains(t, adminErr.Error(), "root@example.com")
	assert.Contains(t, adminErr.Error(), "configured instance admin")

	blockedErr := &ErrAdminDeleteBlocked{Blockers: []models.AdminDeleteBlocker{
		{TeamID: "t1", TeamName: "Acme Engineering", MemberCount: 4},
	}}
	assert.Contains(t, blockedErr.Error(), "Acme Engineering")
	assert.Contains(t, blockedErr.Error(), "transfer ownership")

	// With no blockers the message must still be intelligible rather than
	// trailing an empty list.
	assert.NotEmpty(t, (&ErrAdminDeleteBlocked{}).Error())
}
