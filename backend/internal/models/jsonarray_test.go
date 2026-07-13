package models

import (
	"encoding/json"
	"testing"
)

func TestJSONArrayMarshalNilIsEmptyArray(t *testing.T) {
	var nilArr JSONArray[string]
	b, err := json.Marshal(nilArr)
	if err != nil {
		t.Fatalf("marshal nil JSONArray: %v", err)
	}
	if got := string(b); got != "[]" {
		t.Fatalf("nil JSONArray marshaled to %q, want \"[]\"", got)
	}
}

func TestJSONArrayMarshalEmptyNonNilIsEmptyArray(t *testing.T) {
	empty := JSONArray[int]{}
	b, err := json.Marshal(empty)
	if err != nil {
		t.Fatalf("marshal empty JSONArray: %v", err)
	}
	if got := string(b); got != "[]" {
		t.Fatalf("empty JSONArray marshaled to %q, want \"[]\"", got)
	}
}

func TestJSONArrayMarshalPopulatedMatchesSlice(t *testing.T) {
	arr := JSONArray[string]{"a", "b"}
	b, err := json.Marshal(arr)
	if err != nil {
		t.Fatalf("marshal populated JSONArray: %v", err)
	}
	if got := string(b); got != `["a","b"]` {
		t.Fatalf("populated JSONArray marshaled to %q, want [\"a\",\"b\"]", got)
	}
}

// A nil JSONArray field inside a struct must serialize as `[]`, which is the
// whole point of the type (issue #125).
func TestJSONArrayNilFieldSerializesAsEmptyArray(t *testing.T) {
	type resp struct {
		Items JSONArray[int] `json:"items"`
	}
	b, err := json.Marshal(resp{})
	if err != nil {
		t.Fatalf("marshal struct with nil JSONArray field: %v", err)
	}
	if got := string(b); got != `{"items":[]}` {
		t.Fatalf("struct marshaled to %q, want {\"items\":[]}", got)
	}
}

// Round-trips: a JSONArray unmarshals like a plain slice (no custom
// UnmarshalJSON needed), so response types stay decodable in tests.
func TestJSONArrayUnmarshal(t *testing.T) {
	var arr JSONArray[string]
	if err := json.Unmarshal([]byte(`["x","y"]`), &arr); err != nil {
		t.Fatalf("unmarshal JSONArray: %v", err)
	}
	if len(arr) != 2 || arr[0] != "x" || arr[1] != "y" {
		t.Fatalf("unmarshaled JSONArray = %v, want [x y]", arr)
	}
}

// JSONArray is assignable to/from a plain []T so existing call sites keep
// compiling unchanged.
func TestJSONArraySliceInterop(t *testing.T) {
	plain := []int{1, 2, 3}
	var arr JSONArray[int] = plain // []T -> JSONArray[T]
	arr = append(arr, 4)
	var back []int = arr // JSONArray[T] -> []T
	if len(back) != 4 {
		t.Fatalf("interop slice len = %d, want 4", len(back))
	}
}
