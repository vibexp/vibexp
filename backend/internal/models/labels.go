package models

import (
	"database/sql/driver"
	"encoding/json"

	"github.com/lib/pq"
)

// MaxLabels is the number of labels a single resource may carry.
const MaxLabels = 10

// MaxLabelLength is the maximum length of one label, in characters.
const MaxLabelLength = 50

// LabelList is the shared `labels` taxonomy carried by artifacts, blueprints
// and memories (issue #910). It has to satisfy two invariants at once, which is
// why it is its own type rather than a plain []string or a pq.StringArray:
//
//   - On the wire it is a REQUIRED array in the OpenAPI response schemas, so it
//     must never serialize as `null` — the JSONArray[T] rule (issue #125).
//     pq.StringArray has no MarshalJSON, so a nil one emits `null`.
//   - In Postgres it is a `text[]` column, so it must implement sql.Scanner and
//     driver.Valuer — which models.JSONArray[string] does not.
//
// Prompt.Labels is deliberately NOT this type: its spec field is `nullable`, so
// `null` is the documented wire value for a prompt with no labels and coercing
// it to `[]` would be a breaking change for existing clients.
//
// A LabelList is assignable to and from a plain []string, so services and
// handlers keep working with ordinary slices.
type LabelList []string

// MarshalJSON encodes a nil LabelList as `[]`, exactly like JSONArray[string].
func (l LabelList) MarshalJSON() ([]byte, error) {
	if l == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]string(l))
}

// Scan reads a Postgres text[] column, delegating to pq.StringArray.
func (l *LabelList) Scan(src any) error {
	var arr pq.StringArray
	if err := arr.Scan(src); err != nil {
		return err
	}
	*l = LabelList(arr)
	return nil
}

// Value writes a Postgres text[] column, delegating to pq.StringArray. A nil
// LabelList is written as an empty array rather than NULL, matching the
// column's NOT NULL DEFAULT '{}' and the required-array wire contract.
func (l LabelList) Value() (driver.Value, error) {
	if l == nil {
		return pq.StringArray{}.Value()
	}
	return pq.StringArray(l).Value()
}
