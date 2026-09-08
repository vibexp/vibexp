package models

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// OptionalString distinguishes the three states a nullable string field can be
// in inside a PARTIAL update body, which a plain *string cannot: absent
// ("leave it alone"), explicit null ("clear it") and a value ("set it").
//
// Every other field on the update requests uses the pointer-means-provided
// convention, where `null` and an absent key both decode to nil and are
// therefore indistinguishable. That is fine for a field with no meaningful
// null -- `text` cannot be cleared, it can only be replaced. It is NOT fine for
// a nullable one like `memories.title`, where clearing is a legitimate edit the
// spec documents (`nullable: true` on UpdateMemoryRequest.title, issue #911).
//
// Set is true only when the key was PRESENT in the decoded JSON, because
// encoding/json calls UnmarshalJSON only for keys it actually saw. Value is nil
// when that present key was `null`.
//
// The zero value is the absent state, which is what makes the `omitzero` struct
// tag (Go 1.24+) omit the key when marshalling a request that does not set it --
// so an existing caller that builds the struct in Go and marshals it keeps
// producing byte-identical JSON.
type OptionalString struct {
	Set   bool
	Value *string
}

// NewOptionalString returns the "set to this value" state.
func NewOptionalString(v string) OptionalString {
	return OptionalString{Set: true, Value: &v}
}

// ClearedString returns the "explicit null -- clear it" state.
func ClearedString() OptionalString {
	return OptionalString{Set: true}
}

// IsZero reports whether the field is absent. encoding/json consults it for the
// `omitzero` tag option.
func (o OptionalString) IsZero() bool { return !o.Set }

// UnmarshalJSON records that the key was present and captures its value, with
// `null` decoding to a set-but-nil state.
func (o *OptionalString) UnmarshalJSON(data []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		o.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("invalid string value: %w", err)
	}
	o.Value = &s
	return nil
}

// MarshalJSON emits the captured value, or `null` for the cleared state. An
// absent field is normally omitted by `omitzero` before this is reached; when it
// is not, `null` is the honest rendering of "no value".
func (o OptionalString) MarshalJSON() ([]byte, error) {
	if o.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*o.Value)
}
