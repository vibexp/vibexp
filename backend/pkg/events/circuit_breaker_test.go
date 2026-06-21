package events

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCircuitBreaker_StartsClosed(t *testing.T) {
	cb := NewCircuitBreaker(3, 10)
	assert.True(t, cb.CanExecute("test-key"))
}

func TestCircuitBreaker_TripsAfterMaxFailures(t *testing.T) {
	tests := []struct {
		name        string
		maxFailures int
	}{
		{name: "trips at 2", maxFailures: 2},
		{name: "trips at 3", maxFailures: 3},
		{name: "trips at 5", maxFailures: 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cb := NewCircuitBreaker(tc.maxFailures, 10)
			key := "test-key"

			for i := 0; i < tc.maxFailures-1; i++ {
				cb.RecordFailure(key)
				assert.True(t, cb.CanExecute(key), "should stay closed before reaching the threshold")
			}

			cb.RecordFailure(key)
			assert.False(t, cb.CanExecute(key), "should be open after %d failures", tc.maxFailures)
		})
	}
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(3, 10)
	key := "test-key"

	// Two failures (one short of tripping), then a success resets the count.
	cb.RecordFailure(key)
	cb.RecordFailure(key)
	cb.RecordSuccess(key)

	// Two more failures should still not trip the breaker.
	cb.RecordFailure(key)
	cb.RecordFailure(key)
	assert.True(t, cb.CanExecute(key), "success should have reset the consecutive-failure count")
}

func TestCircuitBreaker_ResetsAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(2, 1) // 1 second reset timeout
	key := "test-key"

	cb.RecordFailure(key)
	cb.RecordFailure(key)
	assert.False(t, cb.CanExecute(key), "breaker should be open after reaching the threshold")

	time.Sleep(1100 * time.Millisecond)

	assert.True(t, cb.CanExecute(key), "breaker should admit a trial request after the reset timeout")
}

func TestCircuitBreaker_HalfOpenSuccessCloses(t *testing.T) {
	cb := NewCircuitBreaker(2, 1)
	key := "test-key"

	cb.RecordFailure(key)
	cb.RecordFailure(key)
	assert.False(t, cb.CanExecute(key))

	time.Sleep(1100 * time.Millisecond)

	// Trial request succeeds -> breaker closes and tolerates failures again.
	assert.True(t, cb.CanExecute(key))
	cb.RecordSuccess(key)
	cb.RecordFailure(key)
	assert.True(t, cb.CanExecute(key), "a single failure after closing should not re-open the breaker")
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker(2, 1)
	key := "test-key"

	cb.RecordFailure(key)
	cb.RecordFailure(key)
	assert.False(t, cb.CanExecute(key))

	time.Sleep(1100 * time.Millisecond)

	// Trial request fails -> breaker re-opens immediately.
	assert.True(t, cb.CanExecute(key))
	cb.RecordFailure(key)
	assert.False(t, cb.CanExecute(key), "a failed trial request should re-open the breaker")
}

func TestCircuitBreaker_FullCycleCanReTrip(t *testing.T) {
	cb := NewCircuitBreaker(2, 1)
	key := "test-key"

	// Trip the breaker.
	cb.RecordFailure(key)
	cb.RecordFailure(key)
	assert.False(t, cb.CanExecute(key))

	time.Sleep(1100 * time.Millisecond)

	// Trial succeeds -> breaker closes again.
	assert.True(t, cb.CanExecute(key))
	cb.RecordSuccess(key)

	// The closed breaker must re-trip after maxFailures consecutive failures.
	cb.RecordFailure(key)
	assert.True(t, cb.CanExecute(key), "should stay closed before reaching the threshold again")
	cb.RecordFailure(key)
	assert.False(t, cb.CanExecute(key), "breaker should re-trip after a full open/trial/close cycle")
}

func TestCircuitBreaker_DifferentKeysAreIndependent(t *testing.T) {
	cb := NewCircuitBreaker(2, 10)
	key1 := "test-key-1"
	key2 := "test-key-2"

	cb.RecordFailure(key1)
	cb.RecordFailure(key1)
	assert.False(t, cb.CanExecute(key1), "key1 should be open")

	assert.True(t, cb.CanExecute(key2), "key2 should be unaffected by key1 tripping")
}

func TestCircuitBreaker_ConcurrentAccessIsSafe(t *testing.T) {
	cb := NewCircuitBreaker(5, 10)
	keys := []string{"a", "b", "c", "d"}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		for _, key := range keys {
			wg.Add(1)
			go func(k string) {
				defer wg.Done()
				cb.CanExecute(k)
				cb.RecordFailure(k)
				cb.RecordSuccess(k)
			}(key)
		}
	}
	wg.Wait()

	// Primary purpose is race-detector coverage; as a post-condition, the
	// breaker must still answer queries for every key without panicking.
	for _, key := range keys {
		assert.NotPanics(t, func() { cb.CanExecute(key) })
	}
}
