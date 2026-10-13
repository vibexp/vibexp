package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPasswordLockout_LockFor(t *testing.T) {
	// The schedule the epic settled on: 1, 2, 4, 8 and then 15 minutes.
	want := map[int]time.Duration{
		0: 0, 1: 0, 4: 0,
		5: time.Minute, 6: 2 * time.Minute, 7: 4 * time.Minute, 8: 8 * time.Minute,
		9: 15 * time.Minute, 10: 15 * time.Minute, 500: 15 * time.Minute,
	}
	for failures, lock := range want {
		assert.Equal(t, lock, DefaultPasswordLockout.LockFor(failures), "after %d failures", failures)
	}
}

func TestPasswordLockout_LockForEdgeSchedules(t *testing.T) {
	t.Run("a base above the cap is capped", func(t *testing.T) {
		l := PasswordLockout{Threshold: 1, Base: time.Hour, Max: time.Minute}
		assert.Equal(t, time.Minute, l.LockFor(1))
	})
	t.Run("a schedule with no threshold never locks", func(t *testing.T) {
		assert.Zero(t, PasswordLockout{Base: time.Minute, Max: time.Hour}.LockFor(100))
	})
	t.Run("a huge failure count does not overflow", func(t *testing.T) {
		assert.Equal(t, 15*time.Minute, DefaultPasswordLockout.LockFor(1<<30))
	})
}

func TestUserCredential_State(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Second), now.Add(time.Second)

	assert.False(t, (&UserCredential{}).IsLockedAt(now), "never locked")
	assert.False(t, (&UserCredential{LockedUntil: &past}).IsLockedAt(now), "a lock ends by itself")
	assert.False(t, (&UserCredential{LockedUntil: &now}).IsLockedAt(now), "the lock's last instant is over")
	assert.True(t, (&UserCredential{LockedUntil: &future}).IsLockedAt(now))

	assert.False(t, (&UserCredential{}).IsDisabled())
	assert.True(t, (&UserCredential{DisabledAt: &past}).IsDisabled())
}
