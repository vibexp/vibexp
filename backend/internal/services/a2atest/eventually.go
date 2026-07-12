package a2atest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Default polling bounds for Eventually. Generous enough for background
// streaming goroutines to finalize under CI + the race detector.
const (
	eventuallyTimeout = 5 * time.Second
	eventuallyTick    = 5 * time.Millisecond
)

// Eventually polls cond until it returns true or the deadline is reached,
// failing the test otherwise. Use it instead of sleeps for assertions that
// depend on the invocation service's background goroutines finalizing.
func Eventually(t testing.TB, cond func() bool, msgAndArgs ...any) {
	t.Helper()
	require.Eventually(t, cond, eventuallyTimeout, eventuallyTick, msgAndArgs...)
}
