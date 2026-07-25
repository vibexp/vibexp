//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level suite for the metadata containment filter (#519) against real
// Postgres. The squirrel tests assert the SQL that gets generated; these assert
// what the database actually MATCHES — which is the part that can be subtly
// wrong, because JSONB containment is type-strict and metadata is stored in
// several shapes (scalar, array, number, boolean).

type metadataFilterFixture struct {
	repo      *BlueprintRepository
	userID    string
	teamID    string
	projectID string
}

func newMetadataFilterFixture(t *testing.T) metadataFilterFixture {
	t.Helper()
	resetMetadataCatalogTables(t)

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	return metadataFilterFixture{
		repo:      NewBlueprintRepository(integrationDB).(*BlueprintRepository),
		userID:    userID,
		teamID:    teamID,
		projectID: insertTestProject(t, userID, teamID),
	}
}

// listTitles runs the filter and returns the matched blueprints' slugs, which
// the seed helper derives from the row id — so the assertions read as "these
// rows and no others".
func (f metadataFilterFixture) matchCount(t *testing.T, filter repositories.MetadataFilter) int {
	t.Helper()
	_, total, err := f.repo.List(context.Background(), f.userID, repositories.BlueprintFilters{
		TeamID:         f.teamID,
		MetadataFilter: filter,
		Page:           1,
		Limit:          100,
	})
	require.NoError(t, err)
	return total
}

func TestIntegrationMetadataFilter_MatchesScalarAndArrayValues(t *testing.T) {
	f := newMetadataFilterFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"prod"}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":["prod","eu"]}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"staging"}`)

	// Both the scalar row and the array row must match one filter value —
	// containment is type-strict, so this only works because both probe shapes
	// are emitted.
	assert.Equal(t, 2, f.matchCount(t, repositories.MetadataFilter{"env": {"prod"}}))
}

func TestIntegrationMetadataFilter_ValuesOrKeysAnd(t *testing.T) {
	f := newMetadataFilterFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"prod","team":"core"}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"staging","team":"core"}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"prod","team":"platform"}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"dev","team":"core"}`)

	// (env=prod OR env=staging) AND team=core -> the first two rows only.
	got := f.matchCount(t, repositories.MetadataFilter{
		"env":  {"prod", "staging"},
		"team": {"core"},
	})

	assert.Equal(t, 2, got)
}

func TestIntegrationMetadataFilter_EmptyValuesMatchesKeyExists(t *testing.T) {
	f := newMetadataFilterFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"prod"}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":null}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"other":"x"}`)

	// Every row carrying the key at all, whatever its value — including null.
	assert.Equal(t, 2, f.matchCount(t, repositories.MetadataFilter{"env": {}}))
}

// TestIntegrationMetadataFilter_TypedProbes is the acceptance criterion that a
// value stored as the number 3 is matched by the filter string "3". Without the
// typed probes these would all return 0.
func TestIntegrationMetadataFilter_TypedProbes(t *testing.T) {
	f := newMetadataFilterFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"n":3}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"n":[7]}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"flag":true}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"flag":false}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"n":"3"}`)

	tests := []struct {
		name   string
		filter repositories.MetadataFilter
		want   int
	}{
		{
			name:   "numeric scalar and its string twin both match",
			filter: repositories.MetadataFilter{"n": {"3"}},
			want:   2, // {"n":3} via the typed probe, {"n":"3"} via the string probe
		},
		{name: "number inside an array", filter: repositories.MetadataFilter{"n": {"7"}}, want: 1},
		{name: "boolean true", filter: repositories.MetadataFilter{"flag": {"true"}}, want: 1},
		{name: "boolean false", filter: repositories.MetadataFilter{"flag": {"false"}}, want: 1},
		{name: "no match", filter: repositories.MetadataFilter{"n": {"99"}}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, f.matchCount(t, tt.filter))
		})
	}
}

// TestIntegrationMetadataFilter_KeysTheOldAllowlistRejected covers the reason
// the charset allowlist was dropped: these keys were always writable, and are
// now filterable too.
func TestIntegrationMetadataFilter_KeysTheOldAllowlistRejected(t *testing.T) {
	f := newMetadataFilterFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"spec.type":"openapi"}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"vibexp:source":"github"}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"plain":"x"}`)

	assert.Equal(t, 1, f.matchCount(t, repositories.MetadataFilter{"spec.type": {"openapi"}}))
	assert.Equal(t, 1, f.matchCount(t, repositories.MetadataFilter{"vibexp:source": {"github"}}))
}

// TestIntegrationMetadataFilter_QuotedKeyIsBoundNotInterpolated proves the
// dropped regex cost no safety: a key full of SQL punctuation runs as a
// parameter and simply matches nothing.
func TestIntegrationMetadataFilter_QuotedKeyIsBoundNotInterpolated(t *testing.T) {
	f := newMetadataFilterFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"prod"}`)

	got := f.matchCount(t, repositories.MetadataFilter{`x'; DROP TABLE blueprints; --`: {"v"}})
	assert.Equal(t, 0, got)

	// The table is still there.
	assert.Equal(t, 1, f.matchCount(t, repositories.MetadataFilter{"env": {"prod"}}))
}

// TestIntegrationMetadataFilter_NoFilterReturnsEverything guards the nil-guard
// in metadataContainment: an empty filter must not append a predicate, because
// squirrel renders an empty conjunction as a dangling WHERE.
func TestIntegrationMetadataFilter_NoFilterReturnsEverything(t *testing.T) {
	f := newMetadataFilterFixture(t)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{"env":"prod"}`)
	insertBlueprintWithMetadata(t, f.userID, f.teamID, f.projectID, `{}`)

	assert.Equal(t, 2, f.matchCount(t, nil))
	assert.Equal(t, 2, f.matchCount(t, repositories.MetadataFilter{}))
}
