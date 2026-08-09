package services

import "github.com/vibexp/vibexp/internal/repositories/postgres"

// FreshnessFilterStale is the only accepted value of the `freshness` list
// filter, re-exported so handlers validate against one constant rather than a
// literal. Freshness state exists only while a resource IS stale, so there is
// no meaningful "fresh" value to ask for.
const FreshnessFilterStale = postgres.FreshnessFilterStale

// freshnessFilter converts the service-layer filter (empty = unset) into the
// repository's optional pointer.
//
// An unrecognized value maps to nil rather than to a predicate that matches
// nothing: handlers reject unknown values with a 400 before reaching here, and
// silently returning an empty list would be the worse failure if one ever slipped
// through — an empty list looks like an answer.
func freshnessFilter(value string) *string {
	if value != FreshnessFilterStale {
		return nil
	}
	stale := FreshnessFilterStale
	return &stale
}
