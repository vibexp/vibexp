package postgres

import (
	"github.com/Masterminds/squirrel"
)

// FreshnessFilterStale is the only value the `freshness` list filter accepts.
// Freshness state exists only while a resource IS stale (the row is deleted
// when it clears), so "fresh" would have to be expressed as an absence — a
// NOT EXISTS that cannot use the same index and that no caller has asked for.
const FreshnessFilterStale = "stale"

// applyStaleFilter narrows a resource list to the stale ones.
//
// It is a correlated EXISTS rather than a join, and that is deliberate. Every
// resource repository builds its COUNT query and its PAGE query with separately
// hard-coded FROM clauses, sharing only the WHERE: a join added for the page
// would leave the count counting unfiltered rows, so `total_count` and
// `total_pages` would describe a different result set than the one returned.
// A predicate in the shared WHERE is correct in both by construction.
//
// It is index-backed: `idx_resource_freshness_resource` is unique on
// (resource_type, resource_id), and both are equality predicates here.
//
// idColumn is the qualified primary key of the resource table being filtered
// (e.g. "a.id"); it is a caller-supplied SQL identifier, never user input.
func applyStaleFilter(where squirrel.And, resourceType, idColumn string) squirrel.And {
	return append(where, squirrel.Expr(
		`EXISTS (SELECT 1 FROM resource_freshness rf
		          WHERE rf.resource_type = ? AND rf.resource_id = `+idColumn+`)`,
		resourceType,
	))
}
