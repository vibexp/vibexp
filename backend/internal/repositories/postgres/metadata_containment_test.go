package postgres

import (
	"strings"
	"testing"

	"github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
)

// renderContainment renders the predicate the way a caller would, through psql,
// so the assertions see the same $n placeholders the real query gets.
func renderContainment(t *testing.T, filter repositories.MetadataFilter) (string, []any) {
	t.Helper()

	predicate := metadataContainment("a.metadata", filter)
	require.NotNil(t, predicate, "expected a predicate for a non-empty filter")

	sqlText, args, err := psql.Select("id").From("artifacts a").Where(predicate).ToSql()
	require.NoError(t, err)
	return sqlText, args
}

func TestMetadataContainment_EmptyFilterIsNil(t *testing.T) {
	// A nil predicate is the contract callers rely on: squirrel emits a dangling
	// " WHERE " for a conjunction that renders to nothing, so an empty filter
	// must not be appended at all.
	assert.Nil(t, metadataContainment("a.metadata", nil))
	assert.Nil(t, metadataContainment("a.metadata", repositories.MetadataFilter{}))
}

func TestMetadataContainment_SingleValueProbesScalarAndArray(t *testing.T) {
	sqlText, args := renderContainment(t, repositories.MetadataFilter{"env": {"prod"}})

	assert.Contains(t, sqlText, "a.metadata @> $1::jsonb")
	assert.Contains(t, sqlText, "a.metadata @> $2::jsonb")
	// Containment is type-strict, so the scalar and single-element-array shapes
	// are both probed — metadata may legitimately be stored either way.
	assert.Equal(t, []any{`{"env":"prod"}`, `{"env":["prod"]}`}, args)
}

func TestMetadataContainment_KeysAreANDedValuesAreORed(t *testing.T) {
	sqlText, args := renderContainment(t, repositories.MetadataFilter{
		"env":  {"prod", "staging"},
		"team": {"core"},
	})

	// Keys render in sorted order so the same filter always produces the same
	// SQL: env (4 probes) then team (2 probes).
	assert.Equal(t, []any{
		`{"env":"prod"}`, `{"env":["prod"]}`,
		`{"env":"staging"}`, `{"env":["staging"]}`,
		`{"team":"core"}`, `{"team":["core"]}`,
	}, args)

	// The env alternatives are ORed inside one group, and that group is ANDed
	// with the team group.
	assert.Equal(t,
		"SELECT id FROM artifacts a WHERE ("+
			"(a.metadata @> $1::jsonb OR a.metadata @> $2::jsonb "+
			"OR a.metadata @> $3::jsonb OR a.metadata @> $4::jsonb) "+
			"AND (a.metadata @> $5::jsonb OR a.metadata @> $6::jsonb))",
		sqlText)
}

func TestMetadataContainment_EmptyValuesRendersKeyExists(t *testing.T) {
	sqlText, args := renderContainment(t, repositories.MetadataFilter{"env": {}})

	// The JSONB `?` key-exists operator cannot be used: psql rewrites every `?`
	// in the SQL text into $n, which would mangle the operator into a
	// placeholder. jsonb_exists is the parameter-safe function form.
	assert.Contains(t, sqlText, "jsonb_exists(a.metadata, $1)")
	assert.NotContains(t, sqlText, "a.metadata ?")
	assert.Equal(t, []any{"env"}, args)
}

func TestMetadataContainment_TypedProbes(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		wantArgs []any
	}{
		{
			name:  "integer also probes the number form",
			value: "3",
			// Without the numeric probe a value stored as the number 3 would
			// never match the filter value "3".
			wantArgs: []any{`{"n":"3"}`, `{"n":["3"]}`, `{"n":3}`, `{"n":[3]}`},
		},
		{
			name:     "decimal keeps the caller's exact representation",
			value:    "3.10",
			wantArgs: []any{`{"n":"3.10"}`, `{"n":["3.10"]}`, `{"n":3.10}`, `{"n":[3.10]}`},
		},
		{
			name:     "negative number",
			value:    "-2",
			wantArgs: []any{`{"n":"-2"}`, `{"n":["-2"]}`, `{"n":-2}`, `{"n":[-2]}`},
		},
		{
			name:     "true also probes the boolean form",
			value:    "true",
			wantArgs: []any{`{"n":"true"}`, `{"n":["true"]}`, `{"n":true}`, `{"n":[true]}`},
		},
		{
			name:     "false also probes the boolean form",
			value:    "false",
			wantArgs: []any{`{"n":"false"}`, `{"n":["false"]}`, `{"n":false}`, `{"n":[false]}`},
		},
		{
			name: "leading zeros are not a JSON number",
			// strconv.ParseFloat would accept "0003"; JSON does not, and emitting
			// it raw would be invalid JSON in the probe.
			value:    "0003",
			wantArgs: []any{`{"n":"0003"}`, `{"n":["0003"]}`},
		},
		{
			name: "1 is a number, not a boolean",
			// strconv.ParseBool accepts "1"; treating it as true would be
			// surprising and would compete for the per-value probe cap.
			value:    "1",
			wantArgs: []any{`{"n":"1"}`, `{"n":["1"]}`, `{"n":1}`, `{"n":[1]}`},
		},
		{
			name:     "TRUE is not the JSON boolean literal",
			value:    "TRUE",
			wantArgs: []any{`{"n":"TRUE"}`, `{"n":["TRUE"]}`},
		},
		{
			name:     "trailing content is not a number",
			value:    "3 apples",
			wantArgs: []any{`{"n":"3 apples"}`, `{"n":["3 apples"]}`},
		},
		{
			name:     "plain string",
			value:    "prod",
			wantArgs: []any{`{"n":"prod"}`, `{"n":["prod"]}`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, args := renderContainment(t, repositories.MetadataFilter{"n": {tt.value}})

			assert.Equal(t, tt.wantArgs, args)
			assert.LessOrEqual(t, len(args), maxMetadataValueProbes, "per-value probe cap")
		})
	}
}

func TestMetadataContainment_QuotesAreEscapedNotInterpolated(t *testing.T) {
	// The key allowlist regex was dropped in #519. That is safe only because
	// every key and value is bound as a parameter, never formatted into SQL.
	filter := repositories.MetadataFilter{`weird"key'; DROP TABLE artifacts; --`: {`va"lue`}}

	sqlText, args := renderContainment(t, filter)

	assert.NotContains(t, sqlText, "DROP TABLE")
	assert.Equal(t, []any{
		`{"weird\"key'; DROP TABLE artifacts; --":"va\"lue"}`,
		`{"weird\"key'; DROP TABLE artifacts; --":["va\"lue"]}`,
	}, args)
}

func TestMetadataContainment_UsesContainmentNotTextExtraction(t *testing.T) {
	// The GIN indexes on the three metadata columns are default jsonb_ops: they
	// serve @> and jsonb_exists but NOT the ->> equality the legacy filter uses.
	// Emitting ->> here would silently make every filter a sequential scan.
	sqlText, _ := renderContainment(t, repositories.MetadataFilter{"env": {"prod"}, "n": {}})

	assert.NotContains(t, sqlText, "->>")
	assert.Contains(t, sqlText, "@>")
	assert.Contains(t, sqlText, "jsonb_exists(")
}

func TestMetadataContainment_HonoursColumnArgument(t *testing.T) {
	for _, column := range []string{"a.metadata", "s.metadata", "m.metadata"} {
		t.Run(column, func(t *testing.T) {
			predicate := metadataContainment(column, repositories.MetadataFilter{"env": {"prod"}})
			require.NotNil(t, predicate)

			sqlText, _, err := predicate.ToSql()
			require.NoError(t, err)
			assert.True(t, strings.Contains(sqlText, column+" @> "), sqlText)
		})
	}
}

// TestMetadataContainment_ComposesWithSquirrelAnd guards the interaction that
// makes the predicate safe to append to the existing where-clause builders.
func TestMetadataContainment_ComposesWithSquirrelAnd(t *testing.T) {
	where := squirrel.And{squirrel.Eq{"a.team_id": "team-123"}}
	where = append(where, metadataContainment("a.metadata", repositories.MetadataFilter{"env": {"prod"}}))

	sqlText, args, err := psql.Select("id").From("artifacts a").Where(where).ToSql()
	require.NoError(t, err)

	assert.Contains(t, sqlText, "a.team_id = $1")
	assert.Equal(t, []any{"team-123", `{"env":"prod"}`, `{"env":["prod"]}`}, args)
}
