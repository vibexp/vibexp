package services

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// TestSpecEnumsMatchServiceAllowlists pins every Go allowlist in this package
// that mirrors a standalone string enum in the OpenAPI spec to that enum
// (#774).
//
// These pairings need a gate because nothing else provides one: oapi-codegen
// validates neither a query parameter's enum nor a request-body one at bind
// time — it binds the raw string — and there is no spec-driven request
// validator in front of the handlers. (FreshnessMetricsRange arrives as a query
// parameter; the two rule enums arrive as request-body arrays.) So the Go
// allowlist is the sole enforcement point in every case and the spec enum is
// only documentation. They drift in both directions: a value added to the spec alone
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
		{
			schema:  "TeamSettingsAuditSurface",
			goName:  "models.SettingsAuditSurfaces (internal/models/team_settings_audit.go)",
			goValue: models.SettingsAuditSurfaces,
		},
		// A request enum (query parameter and body field) on the statuses
		// API (#1265): the service allowlist is its only enforcement.
		{
			schema:  "StatusResourceType",
			goName:  "models.StatusResourceTypes (internal/models/status.go)",
			goValue: models.StatusResourceTypes,
		},
		// The SYSTEM status vocabulary (#912). It is the one status schema that
		// is still an enum: the per-type schemas are open strings since #1266
		// and are pinned by TestStatusSchemasAreOpenAndNameSystemValues below.
		{
			schema:  "ResourceStatus",
			goName:  "models.ResourceStatuses (internal/models/status.go)",
			goValue: models.ResourceStatuses,
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
					"Nothing else enforces this pairing — oapi-codegen binds the raw value "+
					"without validating its enum, and no spec-driven request validator runs "+
					"in front of the handlers — so update both together (#774).", tc.schema, tc.goName)
		})
	}
}

// TestStatusSchemasAreOpenAndNameSystemValues pins what the four per-type
// status schemas became in #1266: open strings of at most 50 characters whose
// description names every system status of the type.
//
// Each half guards a different regression. An `enum:` creeping back makes a
// team's custom status a value neither generated client can represent, on a
// green build. A missing or wrong maxLength disagrees with the `varchar(50)`
// status columns and the slug limit on `team_statuses`. And the description is
// the only place the spec still says what the system statuses are, so it has
// to keep naming exactly the ones models/status.go enforces -- including none
// from another type's subset, which would document a value that always 400s.
func TestStatusSchemasAreOpenAndNameSystemValues(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoBackendDir(t), "schemas", "common.yaml"))
	require.NoError(t, err)

	var schemas map[string]struct {
		Type        string   `yaml:"type"`
		MaxLength   int      `yaml:"maxLength"`
		Description string   `yaml:"description"`
		Enum        []string `yaml:"enum"`
		Example     string   `yaml:"example"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &schemas))

	cases := []struct {
		schema       string
		resourceType string
	}{
		{"PromptStatus", models.StatusResourceTypePrompt},
		{"ArtifactStatus", models.StatusResourceTypeArtifact},
		{"BlueprintStatus", models.StatusResourceTypeBlueprint},
		{"MemoryStatus", models.StatusResourceTypeMemory},
	}

	for _, tc := range cases {
		t.Run(tc.schema, func(t *testing.T) {
			schema, ok := schemas[tc.schema]
			require.True(t, ok, "%s is missing from schemas/common.yaml", tc.schema)

			assert.Equal(t, "string", schema.Type)
			assert.Empty(t, schema.Enum,
				"%s must stay an open string: a team's custom status is a legal value (#1266)", tc.schema)
			assert.Equal(t, 50, schema.MaxLength,
				"%s.maxLength must match the status column and the custom-status slug limit", tc.schema)

			_, enumErr := specconformance.ComponentEnum(tc.schema)
			require.Error(t, enumErr,
				"%s resolves to an enum in the parsed spec; it must be open there too", tc.schema)

			system := models.SystemStatusesFor(tc.resourceType)
			require.NotEmpty(t, system)
			assert.Contains(t, system, schema.Example,
				"the example must be a system status, the only kind every team has")
			for _, status := range models.ResourceStatuses {
				named := strings.Contains(schema.Description, "`"+status+"`")
				assert.Equal(t, slices.Contains(system, status), named,
					"%s's description must name `%s` if and only if it is a system "+
						"status of a %s", tc.schema, status, tc.resourceType)
			}
		})
	}
}
