package specconformance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComponentEnumReadsSpecEnum proves ComponentEnum reads a standalone string
// enum out of the spec, in spec order.
func TestComponentEnumReadsSpecEnum(t *testing.T) {
	got, err := ComponentEnum("FreshnessMetricsRange")
	require.NoError(t, err)
	assert.Equal(t, []string{"7d", "14d", "30d", "60d", "90d", "180d"}, got)
}

// TestComponentEnumFailsLoudly pins the failure modes. Each of these must
// return an error rather than an empty slice: a caller comparing a Go allowlist
// against an empty `want` would pass while proving nothing, which is the exact
// failure this helper exists to prevent (#774).
func TestComponentEnumFailsLoudly(t *testing.T) {
	t.Run("unknown schema", func(t *testing.T) {
		got, err := ComponentEnum("NoSuchSchemaAnywhere")
		require.Error(t, err)
		assert.Empty(t, got)
		assert.Contains(t, err.Error(), "not found in spec")
	})

	t.Run("schema is not an enum", func(t *testing.T) {
		// Team is an object schema, so it declares no enum of its own.
		got, err := ComponentEnum("Team")
		require.Error(t, err)
		assert.Empty(t, got)
		assert.Contains(t, err.Error(), "declares no enum")
	})
}

// TestArrayItemEnumStillReadsItemEnums guards the refactor that generalized
// scalarEnum: ArrayItemEnum must keep resolving one level deeper than
// ComponentEnum, and must still error on a schema whose property is not an
// array of enums.
func TestArrayItemEnumStillReadsItemEnums(t *testing.T) {
	got, err := ArrayItemEnum("Team", "permissions")
	require.NoError(t, err)
	assert.NotEmpty(t, got)
	assert.Contains(t, got, "team.update")

	_, err = ArrayItemEnum("Team", "name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not an array")
}
