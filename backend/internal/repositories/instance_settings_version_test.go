package repositories

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestInstanceSettingsVersionConflicts pins the compare-and-set contract every
// instance settings upsert shares: nil is last-write-wins, 0 expects no row
// (#1220), and any other value must equal the stored version.
func TestInstanceSettingsVersionConflicts(t *testing.T) {
	v := func(n int64) *int64 { return &n }
	cases := []struct {
		name     string
		expected *int64
		stored   *int64
		want     bool
	}{
		{name: "nil with no row is last-write-wins", expected: nil, stored: nil},
		{name: "nil with a row is last-write-wins", expected: nil, stored: v(3)},
		{name: "0 with no row", expected: v(InstanceSettingsNoStoredVersion), stored: nil},
		{name: "0 with a row", expected: v(0), stored: v(1), want: true},
		{name: "matching version", expected: v(3), stored: v(3)},
		{name: "stale version", expected: v(2), stored: v(3), want: true},
		{name: "positive with no row", expected: v(1), stored: nil, want: true},
		{name: "negative with no row", expected: v(-1), stored: nil, want: true},
		{name: "negative with a row", expected: v(-1), stored: v(1), want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, InstanceSettingsVersionConflicts(tc.expected, tc.stored))
		})
	}
}
