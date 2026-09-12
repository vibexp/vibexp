package postgres

import (
	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"
)

// applyLabelsFilter appends the `labels` list filter to a shared WHERE clause
// (issue #910). It is deliberately the OVERLAP operator `&&` — a resource
// matches when it carries AT LEAST ONE of the requested labels — because the
// documented query parameter is "a comma-separated list of labels to filter by"
// and a multi-select label filter that narrowed on every additional selection
// would return nothing as soon as two labels are combined.
//
// All four resources share it, prompts included since #938 — its older
// containment filter (`@>`, AND-of-all-labels) was the last place one query
// parameter name meant two things. Both operators are served by the same
// default `array_ops` GIN opclass, so the per-table GIN index applies either
// way and the plan shape is unchanged.
//
// It belongs in the SHARED where-clause builder, never in the page query alone:
// each resource repository hard-codes its count query and its page query
// separately, so a predicate applied to only one of them yields a short page
// describing an unfiltered total.
func applyLabelsFilter(where squirrel.And, column string, labels []string) squirrel.And {
	if len(labels) == 0 {
		return where
	}
	return append(where, squirrel.Expr(column+" && ?", pq.Array(labels)))
}
