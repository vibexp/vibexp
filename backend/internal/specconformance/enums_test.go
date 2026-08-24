package specconformance

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComponentEnumReadsSpecEnum proves ComponentEnum reads a standalone string
// enum out of the spec, in spec order.
//
// It deliberately does NOT hardcode the enum's values. Which values
// FreshnessMetricsRange holds is already pinned — to the Go allowlist that
// enforces them — by TestSpecEnumsMatchServiceAllowlists in internal/services,
// and a second copy here would turn a correct coordinated change (a window
// added to both the spec and the map) into a failure in an unrelated package,
// with a message about ComponentEnum rather than about ranges. What is left for
// this test is what only it can see: that the values resolve at all, and that
// they come back in SPEC order rather than sorted or map order. The spec
// declares the windows shortest-first, so spec order is ascending by the
// numeric prefix — lexical sorting would give 14d, 180d, 30d, …, and a map
// would give no stable order at all.
func TestComponentEnumReadsSpecEnum(t *testing.T) {
	got, err := ComponentEnum("FreshnessMetricsRange")
	require.NoError(t, err)
	require.Greater(t, len(got), 1, "the enum must have several values for order to mean anything")

	days := make([]int, 0, len(got))
	for _, v := range got {
		n, err := strconv.Atoi(strings.TrimSuffix(v, "d"))
		require.NoError(t, err, "every FreshnessMetricsRange value is <n>d; got %q", v)
		days = append(days, n)
	}
	assert.IsIncreasing(t, days, "ComponentEnum must preserve spec order, not sort or reorder: %v", got)
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
