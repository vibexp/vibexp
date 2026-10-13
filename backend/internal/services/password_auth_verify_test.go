package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// VerifyPassword: checking the CURRENT password from an authenticated session
// (#1437). It must be throttled exactly like a sign-in, or "change password"
// is a free password oracle for whoever holds a session cookie.

func TestPasswordAuthService_VerifyPassword_CorrectPasswordResetsTheThrottle(t *testing.T) {
	f := newPwFixture(t)
	f.credentials.On("BeginAttempt", mock.Anything, pwTestUserID, f.now, models.DefaultPasswordLockout).
		Return(f.credential(), nil).Once()
	f.credentials.On("RecordSuccess", mock.Anything, pwTestUserID).Return(nil).Once()

	require.NoError(t, f.svc.VerifyPassword(context.Background(), pwTestUserID, pwTestPassword))

	verifies, dummies := f.computations()
	assert.EqualValues(t, 1, verifies)
	assert.Zero(t, dummies)
	// users is a strict mock with no expectations: the sign-in gates (email
	// lookup, allowlist, suspension) are not applied to an authenticated session.
}

func TestPasswordAuthService_VerifyPassword_WrongPasswordIsACountedAttempt(t *testing.T) {
	f := newPwFixture(t)
	// BeginAttempt is what counts the attempt, before the hash is checked.
	f.credentials.On("BeginAttempt", mock.Anything, pwTestUserID, f.now, models.DefaultPasswordLockout).
		Return(f.credential(), nil).Once()

	err := f.svc.VerifyPassword(context.Background(), pwTestUserID, "not the password at all")

	require.ErrorIs(t, err, ErrCurrentPasswordInvalid)
	assert.NotErrorIs(t, err, ErrPasswordAttemptsExceeded)
	f.credentials.AssertNumberOfCalls(t, "BeginAttempt", 1)
	f.credentials.AssertNotCalled(t, "RecordSuccess", mock.Anything, mock.Anything)
}

// While the password is locked nothing is checked and nothing is counted, so
// repeating the request cannot extend the lock; the refusal says when it ends.
func TestPasswordAuthService_VerifyPassword_LockedIsRefusedWithoutChecking(t *testing.T) {
	f := newPwFixture(t)
	until := f.now.Add(4 * time.Minute)
	locked := f.credential()
	locked.LockedUntil = &until
	f.credentials.On("BeginAttempt", mock.Anything, pwTestUserID, f.now, models.DefaultPasswordLockout).
		Return(nil, repositories.ErrUserCredentialLocked)
	f.credentials.On("Get", mock.Anything, pwTestUserID).Return(locked, nil)

	err := f.svc.VerifyPassword(context.Background(), pwTestUserID, pwTestPassword)

	require.ErrorIs(t, err, ErrPasswordAttemptsExceeded)
	var lockedErr *PasswordLockedError
	require.ErrorAs(t, err, &lockedErr)
	assert.True(t, until.Equal(lockedErr.Until))
	verifies, dummies := f.computations()
	assert.Zero(t, verifies, "the right password must not be checkable while locked")
	assert.Zero(t, dummies)
	f.credentials.AssertNotCalled(t, "RecordSuccess", mock.Anything, mock.Anything)
}

// The lock's end is a convenience. If it cannot be read the refusal stands.
func TestPasswordAuthService_VerifyPassword_LockedWithoutAReadableLockEnd(t *testing.T) {
	f := newPwFixture(t)
	f.credentials.On("BeginAttempt", mock.Anything, pwTestUserID, f.now, models.DefaultPasswordLockout).
		Return(nil, repositories.ErrUserCredentialLocked)
	f.credentials.On("Get", mock.Anything, pwTestUserID).Return(nil, errors.New("db down"))

	err := f.svc.VerifyPassword(context.Background(), pwTestUserID, pwTestPassword)

	require.ErrorIs(t, err, ErrPasswordAttemptsExceeded)
	var lockedErr *PasswordLockedError
	require.ErrorAs(t, err, &lockedErr)
	assert.True(t, lockedErr.Until.IsZero())
}

// No password, or one an administrator disabled: nothing to check against. The
// answer is the same "not valid", after the same amount of work.
func TestPasswordAuthService_VerifyPassword_NoUsablePasswordIsInvalid(t *testing.T) {
	for name, reason := range map[string]error{
		"no password":       repositories.ErrUserCredentialNotFound,
		"disabled password": repositories.ErrUserCredentialDisabled,
	} {
		t.Run(name, func(t *testing.T) {
			f := newPwFixture(t)
			f.credentials.On("BeginAttempt", mock.Anything, pwTestUserID, f.now, models.DefaultPasswordLockout).
				Return(nil, reason)

			err := f.svc.VerifyPassword(context.Background(), pwTestUserID, pwTestPassword)

			require.ErrorIs(t, err, ErrCurrentPasswordInvalid)
			verifies, dummies := f.computations()
			assert.Zero(t, verifies)
			assert.EqualValues(t, 1, dummies, "one computation, like a real check")
		})
	}
}

// A database fault or an abandoned request is a failure, never "wrong
// password": the caller must not tell the user their password is wrong.
func TestPasswordAuthService_VerifyPassword_InfrastructureFailuresAreNotRefusals(t *testing.T) {
	t.Run("the attempt could not be admitted", func(t *testing.T) {
		f := newPwFixture(t)
		boom := errors.New("connection reset")
		f.credentials.On("BeginAttempt", mock.Anything, pwTestUserID, f.now, models.DefaultPasswordLockout).
			Return(nil, boom)

		err := f.svc.VerifyPassword(context.Background(), pwTestUserID, pwTestPassword)

		require.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, ErrCurrentPasswordInvalid)
	})

	t.Run("the request was abandoned while waiting to hash", func(t *testing.T) {
		f := newPwFixture(t, func(d *PasswordAuthDeps) { d.Hasher = blockedHasher{} })
		f.credentials.On("BeginAttempt", mock.Anything, pwTestUserID, f.now, models.DefaultPasswordLockout).
			Return(f.credential(), nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := f.svc.VerifyPassword(ctx, pwTestUserID, pwTestPassword)

		require.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, ErrCurrentPasswordInvalid)
	})
}

// A stored hash this server cannot read is a refusal, logged for the operator.
func TestPasswordAuthService_VerifyPassword_UnreadableHashIsInvalid(t *testing.T) {
	f := newPwFixture(t)
	broken := &models.UserCredential{UserID: pwTestUserID, PasswordHash: "not-a-hash"}
	f.credentials.On("BeginAttempt", mock.Anything, pwTestUserID, f.now, models.DefaultPasswordLockout).
		Return(broken, nil)

	err := f.svc.VerifyPassword(context.Background(), pwTestUserID, pwTestPassword)

	require.ErrorIs(t, err, ErrCurrentPasswordInvalid)
	f.credentials.AssertNotCalled(t, "RecordSuccess", mock.Anything, mock.Anything)
}

func TestPasswordAuthService_PasswordStatus(t *testing.T) {
	ctx := context.Background()
	changedAt := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	disabledAt := changedAt.Add(time.Hour)

	t.Run("no password", func(t *testing.T) {
		f := newPwFixture(t)
		f.credentials.On("Get", mock.Anything, pwTestUserID).Return(nil, repositories.ErrUserCredentialNotFound)

		status, err := f.svc.PasswordStatus(ctx, pwTestUserID)

		require.NoError(t, err)
		assert.Equal(t, PasswordStatus{}, status)
	})

	t.Run("a usable password reports when it last changed", func(t *testing.T) {
		f := newPwFixture(t)
		credential := f.credential()
		credential.PasswordChangedAt = changedAt
		// A later attempt moved updated_at; that is not a password change.
		credential.UpdatedAt = changedAt.Add(72 * time.Hour)
		f.credentials.On("Get", mock.Anything, pwTestUserID).Return(credential, nil)

		status, err := f.svc.PasswordStatus(ctx, pwTestUserID)

		require.NoError(t, err)
		assert.True(t, status.Set)
		assert.True(t, status.Usable)
		assert.True(t, changedAt.Equal(status.ChangedAt))
	})

	t.Run("a disabled password is set but not usable", func(t *testing.T) {
		f := newPwFixture(t)
		credential := f.credential()
		credential.DisabledAt = &disabledAt
		f.credentials.On("Get", mock.Anything, pwTestUserID).Return(credential, nil)

		status, err := f.svc.PasswordStatus(ctx, pwTestUserID)

		require.NoError(t, err)
		assert.True(t, status.Set)
		assert.False(t, status.Usable)
	})

	t.Run("a read failure is returned", func(t *testing.T) {
		f := newPwFixture(t)
		boom := errors.New("db down")
		f.credentials.On("Get", mock.Anything, pwTestUserID).Return(nil, boom)

		_, err := f.svc.PasswordStatus(ctx, pwTestUserID)

		require.ErrorIs(t, err, boom)
	})
}
