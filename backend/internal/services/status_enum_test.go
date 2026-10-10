package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// TestResourceSchemasDeclareNoInlineStatusEnum keeps the four status schemas in
// common.yaml the only description of what a resource's status may be.
//
// A resource status is an open string since #1266: a team may define its own.
// An inline `enum:` on a `status` property would close it again for that one
// schema, and every other check in the suite would wave it through -- the spec
// would still be valid and the bundle would still build -- while both
// generated clients lost the ability to represent a custom status there. (It is
// also how eleven copies of three vocabularies came to drift before #912.)
//
// It reads the authored YAML as text rather than the parsed model on purpose:
// libopenapi resolves a $ref transparently, so by the time a schema is a
// *base.Schema an inline enum and a $ref'd one are indistinguishable, which is
// exactly the difference being asserted.
func TestResourceSchemasDeclareNoInlineStatusEnum(t *testing.T) {
	root := repoBackendDir(t)

	for _, file := range []string{"prompts.yaml", "artifacts.yaml", "blueprints.yaml", "memories.yaml"} {
		t.Run(file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, "schemas", file))
			require.NoError(t, err)

			offenders := inlineStatusEnumOffenders(file, raw)

			assert.Empty(t, offenders,
				"a `status` property declares an inline enum. Every one of them must be "+
					"a $ref to its open schema in common.yaml, or that one site rejects "+
					"the team's custom statuses in both generated clients (#1266)")
		})
	}

	// Prove the scan can actually see an enum, so an empty result means "none"
	// rather than "the walk never matched anything".
	closed := []byte("Thing:\n  properties:\n    status:\n      type: string\n      enum: [active, draft]\n")
	require.Equal(t, []string{"closed.yaml:5"}, inlineStatusEnumOffenders("closed.yaml", closed))
}

// inlineStatusEnumOffenders returns the "file:line" of every `enum:` declared
// inside the body of a `status:` property in the authored YAML source.
func inlineStatusEnumOffenders(file string, raw []byte) []string {
	lines := strings.Split(string(raw), "\n")
	var offenders []string
	for i, line := range lines {
		if strings.TrimSpace(line) != "status:" {
			continue
		}
		for _, j := range enumLinesInPropertyBody(lines, i) {
			offenders = append(offenders, fmt.Sprintf("%s:%d", file, j+1))
		}
	}
	return offenders
}

// enumLinesInPropertyBody scans the body of the property declared at
// lines[start] -- everything indented deeper than it -- and returns the
// indexes of the lines that open an `enum:`.
func enumLinesInPropertyBody(lines []string, start int) []int {
	indent := indentWidth(lines[start])
	var found []int
	for j := start + 1; j < len(lines); j++ {
		body := lines[j]
		if strings.TrimSpace(body) == "" {
			continue
		}
		if indentWidth(body) <= indent {
			break
		}
		if strings.HasPrefix(strings.TrimSpace(body), "enum:") {
			found = append(found, j)
		}
	}
	return found
}

// indentWidth is the number of leading spaces on a line.
func indentWidth(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// repoBackendDir walks up to the directory holding openapi.yaml, the same way
// specconformance locates the authored spec.
func repoBackendDir(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for range 6 {
		if _, statErr := os.Stat(filepath.Join(dir, "openapi.yaml")); statErr == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate backend/openapi.yaml from the test working directory")
	return ""
}

// TestSystemStatusSubsetsArePinned pins the SYSTEM statuses of each resource
// type to their exact values, in order.
//
// The spec no longer can: the four status schemas are open strings since
// #1266, so nothing generated carries these values any more. They are still
// contract -- they are what every team has without defining anything, what the
// MCP tool descriptions name, and what the 400 lists first -- and each has
// behaviour attached (a prompt is exposed over MCP only once `published`, an
// `archived` memory drops out of the default list). Adding or removing one is
// a product decision; this test makes it a deliberate edit in two places.
func TestSystemStatusSubsetsArePinned(t *testing.T) {
	cases := []struct {
		resourceType string
		subset       []string
		want         []string
	}{
		{models.StatusResourceTypePrompt, models.PromptStatuses, []string{"draft", "published"}},
		{models.StatusResourceTypeArtifact, models.ArtifactStatuses, []string{"active", "draft", "archived"}},
		{models.StatusResourceTypeBlueprint, models.BlueprintStatuses, []string{"active", "expired"}},
		{models.StatusResourceTypeMemory, models.MemoryStatuses, []string{"active", "draft", "archived"}},
	}

	for _, tc := range cases {
		t.Run(tc.resourceType, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.subset)
			assert.Equal(t, tc.want, models.SystemStatusesFor(tc.resourceType),
				"SystemStatusesFor is what the validator and the catalog read")
		})
	}
}

// TestResourceStatusesIsTheUnionOfTheSubsets pins the Go side of the system
// vocabulary. models.ResourceStatuses mirrors the ResourceStatus schema (that
// pairing is asserted by TestSpecEnumsMatchServiceAllowlists) and is the list
// of slugs a team may not take for a custom status, so a value that belongs to
// no subset would reserve a slug no resource type gives a meaning to.
func TestResourceStatusesIsTheUnionOfTheSubsets(t *testing.T) {
	union := map[string]struct{}{}
	for _, subset := range [][]string{
		models.PromptStatuses,
		models.ArtifactStatuses,
		models.BlueprintStatuses,
		models.MemoryStatuses,
	} {
		require.NotEmpty(t, subset, "an empty subset would make this assertion vacuous")
		for _, status := range subset {
			union[status] = struct{}{}
		}
	}

	members := make([]string, 0, len(union))
	for status := range union {
		members = append(members, status)
	}

	assert.ElementsMatch(t, members, models.ResourceStatuses,
		"models.ResourceStatuses must stay exactly the union of the four per-type "+
			"system subsets: a value in no subset reserves a slug nothing uses, and a "+
			"subset value missing from it could be taken as a custom status (#912)")
}

// fakeStatusCatalog is a StatusCatalogServiceInterface over a fixed set of
// custom statuses, keyed by team and resource type the way `team_statuses` is.
// It answers AllowedSlugs the way the real catalog does -- the system statuses,
// then the team's own -- and counts the lookups, so a test can tell the
// query-free path from the catalog-backed one.
type fakeStatusCatalog struct {
	custom  map[string][]string // "<teamID>/<resourceType>" -> custom slugs
	err     error
	lookups int
}

var _ StatusCatalogServiceInterface = (*fakeStatusCatalog)(nil)

func (f *fakeStatusCatalog) AllowedSlugs(_ context.Context, teamID, resourceType string) ([]string, error) {
	f.lookups++
	if f.err != nil {
		return nil, f.err
	}
	return append(models.SystemStatusesFor(resourceType), f.custom[teamID+"/"+resourceType]...), nil
}

func (f *fakeStatusCatalog) IsAllowed(ctx context.Context, teamID, resourceType, status string) (bool, error) {
	slugs, err := f.AllowedSlugs(ctx, teamID, resourceType)
	return slices.Contains(slugs, status), err
}

func (f *fakeStatusCatalog) List(ctx context.Context, teamID, resourceType string) ([]CatalogStatus, error) {
	slugs, err := f.AllowedSlugs(ctx, teamID, resourceType)
	catalog := make([]CatalogStatus, 0, len(slugs))
	for _, slug := range slugs {
		catalog = append(catalog, CatalogStatus{Slug: slug})
	}
	return catalog, err
}

const (
	statusTestTeam      = "team-1"
	statusTestOtherTeam = "team-2"
)

// newFakeStatusCatalog gives statusTestTeam one custom status per resource
// type, each named after its type so a leak between types is visible, and
// gives statusTestOtherTeam a custom artifact status of its own.
func newFakeStatusCatalog() *fakeStatusCatalog {
	return &fakeStatusCatalog{custom: map[string][]string{
		statusTestTeam + "/" + models.StatusResourceTypePrompt:        {"prompt-review"},
		statusTestTeam + "/" + models.StatusResourceTypeArtifact:      {"in-review", "approved"},
		statusTestTeam + "/" + models.StatusResourceTypeBlueprint:     {"blueprint-review"},
		statusTestTeam + "/" + models.StatusResourceTypeMemory:        {"memory-review"},
		statusTestOtherTeam + "/" + models.StatusResourceTypeArtifact: {"other-team-only"},
	}}
}

// TestStatusValidator covers the one check every create, update and list path
// funnels through (#1266).
func TestStatusValidator(t *testing.T) {
	cases := []struct {
		name         string
		teamID       string
		resourceType string
		status       string
		wantErr      bool
		// wantLookups is how many catalog reads the answer may cost.
		wantLookups int
	}{
		// System statuses: answered from the Go subsets, no query.
		{name: "prompt accepts draft", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypePrompt, status: "draft"},
		{name: "prompt accepts published", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypePrompt, status: "published"},
		{name: "artifact accepts archived", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypeArtifact, status: "archived"},
		{name: "blueprint accepts expired", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypeBlueprint, status: "expired"},
		{name: "memory accepts draft", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypeMemory, status: "draft"},
		// "" means "not supplied" on every request model and filter.
		{name: "empty is not supplied", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypePrompt, status: ""},

		// The team's own custom statuses, for the type they were defined on.
		{name: "own custom artifact status", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypeArtifact, status: "in-review", wantLookups: 1},
		{name: "own custom prompt status", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypePrompt, status: "prompt-review", wantLookups: 1},
		{name: "own custom blueprint status", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypeBlueprint, status: "blueprint-review", wantLookups: 1},
		{name: "own custom memory status", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypeMemory, status: "memory-review", wantLookups: 1},

		// A system status of ANOTHER type is not allowed on this one.
		{name: "prompt rejects active", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypePrompt, status: "active", wantErr: true, wantLookups: 1},
		{name: "artifact rejects published", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypeArtifact, status: "published", wantErr: true, wantLookups: 1},
		{name: "blueprint rejects draft", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypeBlueprint, status: "draft", wantErr: true, wantLookups: 1},
		{name: "memory rejects expired", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypeMemory, status: "expired", wantErr: true, wantLookups: 1},

		// Custom statuses do not leak across teams or across resource types.
		{name: "another team's custom status", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypeArtifact, status: "other-team-only",
			wantErr: true, wantLookups: 1},
		{name: "own custom status of another resource type", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypeMemory, status: "in-review",
			wantErr: true, wantLookups: 1},
		{name: "unknown value", teamID: statusTestTeam,
			resourceType: models.StatusResourceTypeMemory, status: "retired", wantErr: true, wantLookups: 1},
		// No team to read a catalog for: the system statuses are all there is.
		{name: "custom status without a team", teamID: "",
			resourceType: models.StatusResourceTypeArtifact, status: "in-review", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			catalog := newFakeStatusCatalog()
			validator := statusValidator{catalog: catalog}

			err := validator.Validate(context.Background(), tc.teamID, tc.resourceType, tc.status)

			assert.Equal(t, tc.wantLookups, catalog.lookups,
				"a system status must cost no catalog read; anything else exactly one")
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidStatus,
				"the handlers map ErrInvalidStatus to 400; any other error falls through to a 500")
		})
	}
}

// The rejection is how a caller learns what it may send, so its wording is
// contract: the system statuses first, then the team's custom ones for that
// resource type, and nobody else's.
func TestStatusValidator_RejectionListsTheAllowedValues(t *testing.T) {
	validator := statusValidator{catalog: newFakeStatusCatalog()}

	err := validator.Validate(
		context.Background(), statusTestTeam, models.StatusResourceTypeArtifact, "retired")

	require.ErrorIs(t, err, ErrInvalidStatus)
	assert.EqualError(t, err,
		"invalid status: must be one of: active, draft, archived, in-review, approved")
	assert.NotContains(t, err.Error(), "other-team-only")
	assert.NotContains(t, err.Error(), "memory-review")
}

func TestStatusValidator_ValidateOptional(t *testing.T) {
	validator := statusValidator{catalog: newFakeStatusCatalog()}
	ctx := context.Background()
	custom, unknown := "in-review", "retired"

	require.NoError(t, validator.ValidateOptional(
		ctx, statusTestTeam, models.StatusResourceTypeArtifact, nil),
		"a nil status is not a status change")
	require.NoError(t, validator.ValidateOptional(
		ctx, statusTestTeam, models.StatusResourceTypeArtifact, &custom))
	require.ErrorIs(t, validator.ValidateOptional(
		ctx, statusTestTeam, models.StatusResourceTypeArtifact, &unknown), ErrInvalidStatus)
}

// A catalog that cannot be read is a server fault, not a bad request: the
// caller's status may well be valid, so answering 400 would be a lie.
func TestStatusValidator_CatalogFailureIsNotAnInvalidStatus(t *testing.T) {
	boom := errors.New("connection refused")
	validator := statusValidator{catalog: &fakeStatusCatalog{err: boom}}

	err := validator.Validate(
		context.Background(), statusTestTeam, models.StatusResourceTypeArtifact, "in-review")

	require.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, ErrInvalidStatus)
}

// A service built without a catalog -- most unit tests, and nothing in the
// wired container -- must fail CLOSED: system statuses only.
func TestStatusValidator_WithoutACatalogKnowsSystemStatusesOnly(t *testing.T) {
	var validator statusValidator
	ctx := context.Background()

	require.NoError(t, validator.Validate(
		ctx, statusTestTeam, models.StatusResourceTypeArtifact, "archived"))

	err := validator.Validate(ctx, statusTestTeam, models.StatusResourceTypeArtifact, "in-review")
	require.ErrorIs(t, err, ErrInvalidStatus)
	assert.EqualError(t, err, "invalid status: must be one of: active, draft, archived")
}
