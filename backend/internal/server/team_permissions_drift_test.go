package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// TestTeamPermissionsEnumMatchesAuthzConstants pins the documented
// `Team.permissions` enum to the authz constants that produce it (#224).
//
// The permission strings are published API surface: clients gate their UI on
// them, so the OpenAPI enum IS the contract and `internal/authz` is the only
// thing allowed to define it. Without this gate the two could drift silently —
// a new permission would reach clients undocumented (and untyped in the
// generated client), and a renamed one would break them with a green build.
// Either way the fix is the same: change the constant and the enum together.
//
// It lives here, with the other spec gates, because it needs specconformance
// to read the spec; the matrix's own behavior is asserted in internal/authz.
//
// Order is not asserted: an enum is a set, and the spec listing it in matrix
// declaration order is documentation hygiene rather than contract.
func TestTeamPermissionsEnumMatchesAuthzConstants(t *testing.T) {
	documented, err := specconformance.ArrayItemEnum("Team", "permissions")
	require.NoError(t, err, "read the documented Team.permissions enum from the spec")

	all := authz.All()
	constants := make([]string, 0, len(all))
	for _, perm := range all {
		constants = append(constants, perm.String())
	}

	assert.ElementsMatch(t, constants, documented,
		"the Team.permissions enum in schemas/teams.yaml has drifted from the authz "+
			"constants in internal/authz/permission.go. These strings are published API "+
			"surface (#224): update both together, and treat a rename as a breaking change.")
}
