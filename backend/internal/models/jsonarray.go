package models

import "encoding/json"

// JSONArray is a slice type whose JSON encoding is guaranteed to never be
// `null`: a nil JSONArray marshals to `[]`, and a populated one marshals
// exactly like the underlying slice.
//
// It exists to enforce, by construction, the invariant that a *required*
// array field in a documented API response never serializes as `null`
// (issue #125, "Layer C" of the response-conformance epic #122). Go's
// encoding/json marshals a nil slice to `null`, so a plain `[]T` field emits
// `null` for an empty list regardless of test coverage, which crashes
// consumers that treat the field as an always-present array (Bug B in #121).
//
// Use JSONArray[T] for response fields the OpenAPI spec marks as a *required*
// array. Do NOT use it for arrays that are legitimately nullable/optional per
// the spec (e.g. an `x | null` field) — those must keep a plain `[]T` so a
// nil value can still serialize as `null`. The enforcement test
// TestRequiredResponseArraysNeverNull (internal/server) checks that every
// required-array response field is protected by this type or an explicitly
// documented exception.
//
// A JSONArray[T] is assignable to and from a plain []T (the underlying type is
// []T and []T is an unnamed composite type), so services, constructors,
// append, range, and indexing keep working without conversions — only the
// struct field declaration changes.
type JSONArray[T any] []T

// MarshalJSON encodes a nil JSONArray as `[]` and otherwise defers to the
// standard slice encoding.
func (a JSONArray[T]) MarshalJSON() ([]byte, error) {
	if a == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]T(a))
}
