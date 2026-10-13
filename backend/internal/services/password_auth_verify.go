package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/auth/password"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Checking a user's CURRENT password from inside an authenticated session
// (#1437): "change my password" has to prove the person at the keyboard knows
// the old one. Login cannot be reused for that. It starts from an email and
// applies the sign-in gates (policy, allowlist, suspension), which the session
// has already passed.
//
// The check goes through the SAME per-account throttle as Login. Without it,
// "change password" would be an unthrottled password oracle for anyone holding
// a session cookie. The price is that wrong attempts here lock password
// sign-in for the account for a while, exactly as wrong sign-ins do.

var (
	// ErrCurrentPasswordInvalid is returned by VerifyPassword when the
	// password is wrong, or the account has no password to check it against.
	// The attempt was counted when there was a password.
	ErrCurrentPasswordInvalid = errors.New("current password is not valid")
	// ErrPasswordAttemptsExceeded is returned by VerifyPassword, as a
	// *PasswordLockedError carrying when the lock ends, while the account's
	// password is locked after too many wrong attempts. Nothing was checked
	// and nothing was counted.
	ErrPasswordAttemptsExceeded = errors.New("too many wrong password attempts")
)

// PasswordLockedError is ErrPasswordAttemptsExceeded with the end of the lock.
type PasswordLockedError struct {
	// Until is when the lock ends; the zero time when it could not be read.
	Until time.Time
}

func (e *PasswordLockedError) Error() string { return ErrPasswordAttemptsExceeded.Error() }

// Is makes errors.Is(err, ErrPasswordAttemptsExceeded) match.
func (e *PasswordLockedError) Is(target error) bool { return target == ErrPasswordAttemptsExceeded }

// PasswordStatus is what the owner of an account may know about its password.
type PasswordStatus struct {
	// Set reports whether the account has a password.
	Set bool
	// Usable reports whether that password can sign in as far as the
	// credential itself is concerned: it is set and an administrator has not
	// disabled it. Whether password sign-in is switched on for the instance is
	// a separate question (the authentication policy).
	Usable bool
	// ChangedAt is when the password was last set or changed; the zero time
	// when there is none.
	ChangedAt time.Time
}

// VerifyPassword implements PasswordAuthServiceInterface.
func (s *PasswordAuthService) VerifyPassword(ctx context.Context, userID, plaintext string) error {
	credential, err := s.credentials.BeginAttempt(ctx, userID, s.now(), s.lockout)
	switch {
	case errors.Is(err, repositories.ErrUserCredentialLocked):
		return s.lockedError(ctx, userID)
	case errors.Is(err, repositories.ErrUserCredentialNotFound),
		errors.Is(err, repositories.ErrUserCredentialDisabled):
		// Nothing to check against. Spend the same computation a real check
		// costs, so the answer takes as long either way.
		if dummyErr := s.hasher.DummyVerify(ctx, plaintext); dummyErr != nil {
			return fmt.Errorf("verify the current password: %w", dummyErr)
		}
		return ErrCurrentPasswordInvalid
	case err != nil:
		return fmt.Errorf("admit the current password check: %w", err)
	}

	ok, needsRehash, err := s.hasher.Verify(ctx, credential.PasswordHash, plaintext)
	switch {
	case errors.Is(err, password.ErrMalformedHash):
		// A stored hash this server cannot read. The error carries no part of it.
		s.logger.With("user_id", userID, "error", err.Error()).
			Error("Current password check refused: the stored hash is unreadable")
		return ErrCurrentPasswordInvalid
	case err != nil:
		// The request was abandoned while waiting to hash. Not a wrong password.
		return fmt.Errorf("verify the current password: %w", err)
	case !ok:
		s.logIfLocked(userID, credential)
		return ErrCurrentPasswordInvalid
	}

	s.afterSuccess(ctx, userID, credential.PasswordHash, plaintext, needsRehash)
	return nil
}

// lockedError builds the refusal for a locked credential. BeginAttempt does not
// say when the lock ends, so it is read back; if that fails the refusal stands
// without the time.
func (s *PasswordAuthService) lockedError(ctx context.Context, userID string) error {
	locked := &PasswordLockedError{}
	credential, err := s.credentials.Get(ctx, userID)
	if err != nil {
		s.logger.With("user_id", userID, "error", err.Error()).
			Warn("Could not read when the password lock ends")
		return locked
	}
	if credential.LockedUntil != nil {
		locked.Until = *credential.LockedUntil
	}
	return locked
}

// PasswordStatus implements PasswordAuthServiceInterface.
func (s *PasswordAuthService) PasswordStatus(ctx context.Context, userID string) (PasswordStatus, error) {
	credential, err := s.credentials.Get(ctx, userID)
	if errors.Is(err, repositories.ErrUserCredentialNotFound) {
		return PasswordStatus{}, nil
	}
	if err != nil {
		return PasswordStatus{}, fmt.Errorf("load the password credential: %w", err)
	}
	return PasswordStatus{
		Set:       true,
		Usable:    !credential.IsDisabled(),
		ChangedAt: credential.PasswordChangedAt,
	}, nil
}
