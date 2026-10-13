//go:build integration

package postgres

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The lock is what makes "never remove the last sign-in method" hold when two
// removals run together (#1437): the second must not start its check until the
// first has finished its delete.
func TestIntegrationSignInMethodLock_SerializesPerUser(t *testing.T) {
	db := integrationDB
	lock := NewSignInMethodLock(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	userID := uuid.NewString()

	t.Run("two holders for one user never overlap", func(t *testing.T) {
		var inside, overlaps atomic.Int32
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				err := lock.WithLock(ctx, userID, func(context.Context) error {
					if inside.Add(1) > 1 {
						overlaps.Add(1)
					}
					time.Sleep(20 * time.Millisecond)
					inside.Add(-1)
					return nil
				})
				assert.NoError(t, err)
			})
		}
		wg.Wait()

		assert.Zero(t, overlaps.Load())
	})

	t.Run("a check-then-delete under the lock leaves one of two rows", func(t *testing.T) {
		// Two "removals", each allowed only while the other row is still there.
		_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS sign_in_lock_probe`)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `CREATE TABLE sign_in_lock_probe (name text PRIMARY KEY)`)
		require.NoError(t, err)
		t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DROP TABLE IF EXISTS sign_in_lock_probe`) })
		_, err = db.ExecContext(ctx, `INSERT INTO sign_in_lock_probe VALUES ('password'), ('identity')`)
		require.NoError(t, err)

		remove := func(mine, other string) error {
			return lock.WithLock(ctx, userID, func(ctx context.Context) error {
				var others int
				if err := db.QueryRowContext(ctx,
					`SELECT count(*) FROM sign_in_lock_probe WHERE name = $1`, other).Scan(&others); err != nil {
					return err
				}
				if others == 0 {
					return nil // the guard refuses
				}
				time.Sleep(30 * time.Millisecond) // widen the window a race would need
				_, err := db.ExecContext(ctx, `DELETE FROM sign_in_lock_probe WHERE name = $1`, mine)
				return err
			})
		}
		var wg sync.WaitGroup
		wg.Go(func() { assert.NoError(t, remove("password", "identity")) })
		wg.Go(func() { assert.NoError(t, remove("identity", "password")) })
		wg.Wait()

		var left int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM sign_in_lock_probe`).Scan(&left))
		assert.Equal(t, 1, left, "without the lock both guards pass and nothing is left")
	})

	t.Run("different users do not wait on each other", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- lock.WithLock(ctx, userID, func(context.Context) error {
				close(entered)
				<-release
				return nil
			})
		}()
		<-entered

		other, otherCancel := context.WithTimeout(ctx, 5*time.Second)
		defer otherCancel()
		err := lock.WithLock(other, uuid.NewString(), func(context.Context) error { return nil })

		close(release)
		require.NoError(t, <-done)
		require.NoError(t, err)
	})

	t.Run("fn's error is returned and the lock is released", func(t *testing.T) {
		err := lock.WithLock(ctx, userID, func(context.Context) error { return assert.AnError })
		require.ErrorIs(t, err, assert.AnError)

		again, againCancel := context.WithTimeout(ctx, 5*time.Second)
		defer againCancel()
		require.NoError(t, lock.WithLock(again, userID, func(context.Context) error { return nil }))
	})
}
