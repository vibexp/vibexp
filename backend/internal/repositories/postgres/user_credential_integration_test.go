//go:build integration

package postgres

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Real-Postgres coverage for user_credentials (#1431). A sign-in attempt is
// admitted, counted and (from the threshold on) locked by one UPDATE whose
// WHERE clause is the lock. These tests pin the schedule that statement
// computes against models.PasswordLockout.LockFor, and pin the property the
// statement exists for: a burst of simultaneous attempts is bounded by the
// threshold.

// A hash-shaped fixture. The repository never interprets it.
const credentialTestHash = "$argon2id$v=19$m=64,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2U"

func TestIntegrationUserCredential_Lifecycle(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()
	userID := insertTestUser(t)

	_, err := repo.Get(ctx, userID)
	assert.ErrorIs(t, err, repositories.ErrUserCredentialNotFound, "a user starts with no password")

	require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash))
	got, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, userID, got.UserID)
	assert.Equal(t, credentialTestHash, got.PasswordHash)
	assert.Zero(t, got.FailedAttempts)
	assert.Nil(t, got.LockedUntil)
	assert.Nil(t, got.DisabledAt)

	deleted, err := repo.Delete(ctx, userID)
	require.NoError(t, err)
	assert.True(t, deleted)
	deleted, err = repo.Delete(ctx, userID)
	require.NoError(t, err)
	assert.False(t, deleted, "deleting nothing is not an error")
	_, err = repo.Get(ctx, userID)
	assert.ErrorIs(t, err, repositories.ErrUserCredentialNotFound)
}

// UpdateHash is a rehash: a compare-and-swap on the hash that was verified,
// which leaves the throttle exactly as it was.
func TestIntegrationUserCredential_UpdateHashIsACompareAndSwap(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()
	userID := insertTestUser(t)
	require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash))
	_, err := repo.BeginAttempt(ctx, userID, time.Now(), models.DefaultPasswordLockout)
	require.NoError(t, err)

	updated, err := repo.UpdateHash(ctx, userID, credentialTestHash, credentialTestHash+"-rehashed")
	require.NoError(t, err)
	assert.True(t, updated)
	got, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, credentialTestHash+"-rehashed", got.PasswordHash)
	assert.Equal(t, 1, got.FailedAttempts, "a rehash does not forgive failed attempts")

	// The password was changed meanwhile: a sign-in that verified the OLD hash
	// must not write its rehash over the new one.
	require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash+"-new-password"))
	updated, err = repo.UpdateHash(ctx, userID, credentialTestHash+"-rehashed", credentialTestHash+"-stale")
	require.NoError(t, err)
	assert.False(t, updated)
	got, err = repo.Get(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, credentialTestHash+"-new-password", got.PasswordHash, "the new password survives")

	updated, err = repo.UpdateHash(ctx, insertTestUser(t), credentialTestHash, credentialTestHash)
	require.NoError(t, err)
	assert.False(t, updated, "a user with no credential has nothing to swap")
}

func TestIntegrationUserCredential_MissingRows(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()
	noPassword := insertTestUser(t)
	ghost := uuid.New().String()

	assert.ErrorIs(t, repo.Upsert(ctx, ghost, credentialTestHash), repositories.ErrUserNotFound)
	_, err := repo.BeginAttempt(ctx, noPassword, time.Now(), models.DefaultPasswordLockout)
	assert.ErrorIs(t, err, repositories.ErrUserCredentialNotFound,
		"no credential is told apart from a locked or disabled one")
	assert.NoError(t, repo.RecordSuccess(ctx, noPassword), "nothing to reset is not an error")
}

// TestIntegrationUserCredential_LockoutLadderMatchesTheSchedule walks the
// throttle past its cap. Each attempt is made at the instant the previous lock
// ends, and every lock the SQL computes is checked against the Go schedule.
func TestIntegrationUserCredential_LockoutLadderMatchesTheSchedule(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()
	userID := insertTestUser(t)
	require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash))
	lockout := models.DefaultPasswordLockout
	now := time.Now().UTC().Truncate(time.Microsecond)

	for attempts := 1; attempts <= 12; attempts++ {
		got, err := repo.BeginAttempt(ctx, userID, now, lockout)
		require.NoError(t, err, "attempt %d is admitted", attempts)
		require.Equal(t, attempts, got.FailedAttempts)
		assert.Equal(t, credentialTestHash, got.PasswordHash, "the admitted attempt carries the hash to check")

		want := lockout.LockFor(attempts)
		if want == 0 {
			assert.Nil(t, got.LockedUntil, "no lock after %d attempts", attempts)
			continue
		}
		require.NotNil(t, got.LockedUntil, "a lock after %d attempts", attempts)
		assert.True(t, now.Add(want).Equal(*got.LockedUntil),
			"after %d attempts: want a %s lock, got %s", attempts, want, got.LockedUntil.Sub(now))

		// While the lock runs, an attempt is refused and nothing is counted.
		_, err = repo.BeginAttempt(ctx, userID, now.Add(want-time.Microsecond), lockout)
		require.ErrorIs(t, err, repositories.ErrUserCredentialUnavailable)
		require.ErrorIs(t, err, repositories.ErrUserCredentialLocked, "and says it was the lock")
		stored, err := repo.Get(ctx, userID)
		require.NoError(t, err)
		require.Equal(t, attempts, stored.FailedAttempts, "a refused attempt is not counted")
		require.True(t, got.LockedUntil.Equal(*stored.LockedUntil), "and does not extend the lock")

		// The lock ends by itself: the next attempt is made at that instant.
		now = *got.LockedUntil
	}

	// The 9th attempt and every one after it hit the cap: no permanent lock.
	capped, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, lockout.Max, lockout.LockFor(capped.FailedAttempts))

	require.NoError(t, repo.RecordSuccess(ctx, userID))
	reset, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	assert.Zero(t, reset.FailedAttempts)
	assert.Nil(t, reset.LockedUntil, "a success clears the lock and the count")
}

// A custom schedule is honoured, and a count far past the cap neither errors
// nor overflows.
func TestIntegrationUserCredential_LockoutHonoursTheGivenSchedule(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()
	userID := insertTestUser(t)
	require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash))
	lockout := models.PasswordLockout{Threshold: 2, Base: 10 * time.Second, Max: 25 * time.Second}
	now := time.Now().UTC().Truncate(time.Microsecond)

	for attempt, want := range []time.Duration{0, 10 * time.Second, 20 * time.Second, 25 * time.Second} {
		got, err := repo.BeginAttempt(ctx, userID, now, lockout)
		require.NoError(t, err)
		if want == 0 {
			assert.Nil(t, got.LockedUntil)
			continue
		}
		require.NotNil(t, got.LockedUntil)
		assert.Equal(t, want, got.LockedUntil.Sub(now), "attempt %d", attempt+1)
		now = *got.LockedUntil
	}

	_, err := integrationDB.ExecContext(ctx,
		"UPDATE user_credentials SET failed_attempts = 2000000 WHERE user_id = $1", userID)
	require.NoError(t, err)
	got, err := repo.BeginAttempt(ctx, userID, now, lockout)
	require.NoError(t, err)
	assert.Equal(t, lockout.Max, got.LockedUntil.Sub(now))
}

// TestIntegrationUserCredential_ConcurrentAttemptsAreBoundedByTheThreshold is
// the reason the attempt is counted before the password is checked. Many
// attempts arriving at the same instant are admitted one at a time on the row
// lock, each re-evaluating the lock the previous one committed: exactly the
// threshold's worth are admitted, and the rest find the account locked.
func TestIntegrationUserCredential_ConcurrentAttemptsAreBoundedByTheThreshold(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()
	userID := insertTestUser(t)
	require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash))
	lockout := models.DefaultPasswordLockout
	now := time.Now().UTC().Truncate(time.Microsecond)

	const attempts = 60
	var admitted, refused atomic.Int32
	var wg sync.WaitGroup
	for range attempts {
		wg.Go(func() {
			_, err := repo.BeginAttempt(ctx, userID, now, lockout)
			switch {
			case err == nil:
				admitted.Add(1)
			case errors.Is(err, repositories.ErrUserCredentialUnavailable):
				refused.Add(1)
			default:
				assert.NoError(t, err)
			}
		})
	}
	wg.Wait()

	assert.Equal(t, int32(lockout.Threshold), admitted.Load(), "a burst gets the threshold's worth of guesses, no more")
	assert.Equal(t, int32(attempts-lockout.Threshold), refused.Load())
	got, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, lockout.Threshold, got.FailedAttempts, "refused attempts are not counted")
	require.NotNil(t, got.LockedUntil)
	assert.True(t, now.Add(lockout.Base).Equal(*got.LockedUntil), "and the lock is the first one, not extended")
}

// A disabled credential admits no attempt and counts none.
func TestIntegrationUserCredential_DisabledAdmitsNoAttempt(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()
	userID := insertTestUser(t)
	require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash))
	_, err := integrationDB.ExecContext(ctx,
		"UPDATE user_credentials SET disabled_at = CURRENT_TIMESTAMP WHERE user_id = $1", userID)
	require.NoError(t, err)

	_, err = repo.BeginAttempt(ctx, userID, time.Now(), models.DefaultPasswordLockout)

	require.ErrorIs(t, err, repositories.ErrUserCredentialUnavailable)
	require.ErrorIs(t, err, repositories.ErrUserCredentialDisabled, "and says it was disabled, not locked")
	assert.NotErrorIs(t, err, repositories.ErrUserCredentialLocked)
	got, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	assert.Zero(t, got.FailedAttempts)
	assert.True(t, got.IsDisabled())
}

// A new password is a fresh start: it clears the throttle and a disabled mark.
func TestIntegrationUserCredential_UpsertIsAFreshStart(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()
	userID := insertTestUser(t)
	require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash))
	now := time.Now().UTC()
	for range models.DefaultPasswordLockout.Threshold {
		_, err := repo.BeginAttempt(ctx, userID, now, models.DefaultPasswordLockout)
		require.NoError(t, err)
	}
	_, err := integrationDB.ExecContext(ctx,
		"UPDATE user_credentials SET disabled_at = CURRENT_TIMESTAMP WHERE user_id = $1", userID)
	require.NoError(t, err)
	locked, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	require.True(t, locked.IsDisabled())
	require.NotNil(t, locked.LockedUntil)

	require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash+"new"))

	fresh, err := repo.Get(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, credentialTestHash+"new", fresh.PasswordHash)
	assert.Zero(t, fresh.FailedAttempts)
	assert.Nil(t, fresh.LockedUntil)
	assert.False(t, fresh.IsDisabled())
	assert.True(t, fresh.CreatedAt.Equal(locked.CreatedAt), "the row is updated, not replaced")
}

// A deleted account leaves no hash behind.
func TestIntegrationUserCredential_GoesWithTheUser(t *testing.T) {
	resetIntegrationTables(t)
	repo := NewUserCredentialRepository(integrationDB)
	ctx := context.Background()
	userID := insertTestUser(t)
	require.NoError(t, repo.Upsert(ctx, userID, credentialTestHash))

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
	require.NoError(t, err)

	var remaining int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM user_credentials WHERE user_id = $1", userID).Scan(&remaining))
	assert.Zero(t, remaining)
}
