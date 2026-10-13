package models

import "time"

// UserCredential is a user's password credential (#1431): the argon2id hash and
// the state of the per-account sign-in throttle. A user has at most one; a user
// with none has no password.
//
// The hash is never serialized. Nothing outside the password sign-in service
// has a reason to read it.
type UserCredential struct {
	UserID string `json:"user_id" db:"user_id"`
	// PasswordHash is the encoded argon2id hash (see internal/auth/password).
	PasswordHash string `json:"-" db:"password_hash"`
	// FailedAttempts counts consecutive failed sign-ins since the last success.
	FailedAttempts int `json:"failed_attempts" db:"failed_attempts"`
	// LockedUntil, while in the future, refuses sign-in without checking the
	// password. Nil when the account was never locked.
	LockedUntil *time.Time `json:"locked_until,omitempty" db:"locked_until"`
	// DisabledAt is set when an administrator disabled the password without
	// removing it.
	DisabledAt *time.Time `json:"disabled_at,omitempty" db:"disabled_at"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
}

// IsDisabled reports whether an administrator disabled the password.
func (c *UserCredential) IsDisabled() bool {
	return c.DisabledAt != nil
}

// IsLockedAt reports whether sign-in is refused at now because of failed
// attempts. A lock ends by itself when its time passes.
func (c *UserCredential) IsLockedAt(now time.Time) bool {
	return c.LockedUntil != nil && now.Before(*c.LockedUntil)
}

// PasswordLockout is the schedule of the per-account sign-in throttle: the
// Threshold-th consecutive failure locks the account for Base, and every
// failure after that doubles the lock, up to Max. A successful sign-in resets
// the count. There is no permanent lock.
type PasswordLockout struct {
	// Threshold is the number of consecutive failures that starts locking.
	Threshold int
	// Base is the first lock's length.
	Base time.Duration
	// Max caps a lock's length.
	Max time.Duration
}

// DefaultPasswordLockout locks for 1 minute on the 5th consecutive failure,
// then 2, 4 and 8 minutes, and 15 minutes from the 9th on.
var DefaultPasswordLockout = PasswordLockout{Threshold: 5, Base: time.Minute, Max: 15 * time.Minute}

// LockFor returns how long the account is locked after its failures-th
// consecutive failure: zero below the threshold.
func (l PasswordLockout) LockFor(failures int) time.Duration {
	if l.Threshold < 1 || failures < l.Threshold {
		return 0
	}
	lock := l.Base
	for range failures - l.Threshold {
		lock *= 2
		if lock >= l.Max {
			return l.Max
		}
	}
	return min(lock, l.Max)
}
