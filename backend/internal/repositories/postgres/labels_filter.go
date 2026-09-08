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
// Note this differs from prompts, whose older `labels` filter uses containment
// (`@>`, i.e. AND-of-all-labels). Both are served by the same default
// `array_ops` GIN opclass, so the index applies either way; the prompt
// behaviour is left alone because changing it is a wire-visible change to a
// shipped endpoint.
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
