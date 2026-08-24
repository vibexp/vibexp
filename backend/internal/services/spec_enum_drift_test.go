package services

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/specconformance"
)

// TestSpecEnumsMatchServiceAllowlists pins every Go allowlist in this package
// that mirrors a standalone string enum in the OpenAPI spec to that enum
// (#774).
//
// These pairings need a gate because nothing else provides one: oapi-codegen
// does not validate a query parameter's enum — it binds the raw string — so the
// Go allowlist is the sole enforcement point and the spec enum is only
// documentation. They drift in both directions: a value added to the spec alone
// is documented but always 400s; a value removed from the spec alone keeps
// working undocumented. Both API clients are generated from the spec, so the
// side that is wrong is the side clients are built against.
//
// Same shape as TestTeamPermissionsEnumMatchesAuthzConstants, deliberately:
// that test pins the authz permission constants to their spec enum for exactly
// this reason. Adding the next pairing is one line here.
//
// Order is not asserted — an enum is a set, and the spec listing it in any
// particular order is documentation hygiene rather than contract.
//
// SCOPE — what a sweep for further mirrors found (2026-08-24, #774). The
// pattern is repo-wide: roughly 40 more Go value sets mirror a spec enum
// (relation types/origins/statuses, blueprint type+subtype, resource-access
// types and ranges, embedding entity types, search types, email provider
// types, every `sort_by` allowlist, the admin granularities, …). NONE of them
// were drifting when measured. They are left unpinned here for a structural
// reason, not an arbitrary one: almost all are either a scalar *property* of a
// component schema or a path-level *query parameter*, and specconformance can
// read neither — ComponentEnum reads a schema that IS an enum and ArrayItemEnum
// reads an array property's items. Pinning them wants two more siblings
// (PropertyEnum(schema, property) and ParameterEnum(path, method, param)),
// which is a separate change; componentProperty and scalarEnum already compose
// for the first. Deliberate non-mirrors stay out regardless — e.g.
// TeamMemberDetail.invitation_status documents the 2 states a member can be in
// while models.InvitationStatus has 4, and freshness/clear.go's clearReasons is
// an intentional subset of the reason enum.
func TestSpecEnumsMatchServiceAllowlists(t *testing.T) {
	cases := []struct {
		schema  string   // component schema in backend/schemas/*.yaml
		goName  string   // the Go allowlist it must match, for the failure message
		goValue []string // that allowlist's members
	}{
		{
			schema:  "FreshnessMetricsRange",
			goName:  "freshnessMetricsRanges (internal/services/freshness_metrics.go)",
			goValue: slices.Collect(maps.Keys(freshnessMetricsRanges)),
		},
		{
			schema:  "FreshnessRuleResourceType",
			goName:  "freshnessRuleResourceTypes (internal/services/freshness.go)",
			goValue: freshnessRuleResourceTypes,
		},
		{
			schema:  "FreshnessRuleMedium",
			goName:  "freshnessRuleMediums (internal/services/freshness.go)",
			goValue: freshnessRuleMediums,
		},
	}

	for _, tc := range cases {
		t.Run(tc.schema, func(t *testing.T) {
			documented, err := specconformance.ComponentEnum(tc.schema)
			require.NoError(t, err,
				"read the %s enum from the spec; a rename in schemas/*.yaml means renaming it here too", tc.schema)
			require.NotEmpty(t, documented,
				"the %s enum must be non-empty — an empty want would make this assertion vacuous", tc.schema)

			assert.ElementsMatch(t, documented, tc.goValue,
				"the %s enum in backend/schemas has drifted from %s. "+
					"Nothing else enforces this pairing — oapi-codegen binds the raw query string "+
					"without validating its enum — so update both together (#774).", tc.schema, tc.goName)
		})
	}
}
