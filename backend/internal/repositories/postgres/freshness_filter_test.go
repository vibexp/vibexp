package postgres

import (
	"testing"

	"github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The stale filter is the one predicate shared by all four resource list
// queries, and it goes into the clause their COUNT and PAGE queries share —
// so its exact shape decides both what a page contains and what total_count
// claims. These pin the shape; the integration suite proves the semantics and
// the index usage.
func TestApplyStaleFilter(t *testing.T) {
	where := applyStaleFilter(squirrel.And{squirrel.Eq{"a.team_id": "team-1"}}, "artifact", "a.id")

	sql, args, err := psql.Select("a.id").From("artifacts a").Where(where).ToSql()

	require.NoError(t, err)
	// EXISTS, not a join: a join would have to be mirrored in the count query
	// or pagination totals would describe a different result set.
	assert.Contains(t, sql, "EXISTS (SELECT 1 FROM resource_freshness rf")
	// Both index columns are equality predicates, which is what lets the
	// unique (resource_type, resource_id) index serve it.
	assert.Contains(t, sql, "rf.resource_type =")
	assert.Contains(t, sql, "rf.resource_id = a.id")
	assert.Equal(t, []interface{}{"team-1", "artifact"}, args,
		"the resource type is bound, never interpolated")
}

// The predicate must be appended, never replace what is already there — the
// tenancy predicates live in the same clause.
func TestApplyStaleFilter_PreservesExistingPredicates(t *testing.T) {
	base := squirrel.And{squirrel.Eq{"m.team_id": "team-1"}, squirrel.Eq{"m.project_id": "project-1"}}

	where := applyStaleFilter(base, "memory", "m.id")

	sql, args, err := psql.Select("m.id").From("memories m").Where(where).ToSql()

	require.NoError(t, err)
	assert.Contains(t, sql, "m.team_id")
	assert.Contains(t, sql, "m.project_id")
	assert.Equal(t, []interface{}{"team-1", "project-1", "memory"}, args)
}
