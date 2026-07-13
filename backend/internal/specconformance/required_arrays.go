package specconformance

import (
	"sort"
	"strings"

	base "github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// RequiredArrayFields returns, per OpenAPI schema, the JSON names of its
// *required* array properties that are NOT legitimately nullable. It is the
// source of truth for the "required arrays never serialize as null" invariant
// (issue #125): the enforcement test in internal/server cross-checks this set
// against the Go response types so a new required-array response field cannot
// ship unprotected.
//
// A property qualifies when: the (top-level) schema lists it in `required`,
// its type is `array`, and it is not marked nullable (OpenAPI 3.0
// `nullable: true` or a 3.1 `["array","null"]` type union) — a nullable array
// may legitimately serialize as `null` and is out of scope.
//
// Both surfaces are walked and merged so a schema referenced only from an
// operation (never registered under components/schemas) is still covered:
//   - every component schema, keyed by its component name; and
//   - every operation response body, keyed by the $ref name of its schema.
//
// Only TOP-LEVEL required arrays of a response schema are reported; an array
// nested inside another object property (e.g. a `data` envelope) is not, and
// is protected at its own Go type instead. Schemas with no qualifying property
// are omitted. Access is guarded by validateMu because resolving schema
// proxies mutates the shared *v3.Document.
func RequiredArrayFields() (map[string][]string, error) {
	s, err := load()
	if err != nil {
		return nil, err
	}

	validateMu.Lock()
	defer validateMu.Unlock()

	acc := newArrayFieldAcc()
	collectComponentArrays(s.model, acc)
	collectResponseArrays(s.model, acc)
	return acc.result(), nil
}

// arrayFieldAcc accumulates required-array field names per schema, deduping.
type arrayFieldAcc struct {
	fields map[string]map[string]struct{}
}

func newArrayFieldAcc() *arrayFieldAcc {
	return &arrayFieldAcc{fields: make(map[string]map[string]struct{})}
}

func (a *arrayFieldAcc) add(name string, fields []string) {
	if name == "" || len(fields) == 0 {
		return
	}
	set := a.fields[name]
	if set == nil {
		set = make(map[string]struct{})
		a.fields[name] = set
	}
	for _, f := range fields {
		set[f] = struct{}{}
	}
}

func (a *arrayFieldAcc) result() map[string][]string {
	out := make(map[string][]string, len(a.fields))
	for name, set := range a.fields {
		fields := make([]string, 0, len(set))
		for f := range set {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		out[name] = fields
	}
	return out
}

// collectComponentArrays records required top-level arrays of every schema
// registered under components/schemas, keyed by component name.
func collectComponentArrays(model *v3.Document, acc *arrayFieldAcc) {
	if model.Components == nil || model.Components.Schemas == nil {
		return
	}
	for pair := model.Components.Schemas.First(); pair != nil; pair = pair.Next() {
		acc.add(pair.Key(), topLevelRequiredArrays(pair.Value()))
	}
}

// collectResponseArrays records required top-level arrays of every operation
// response body, keyed by the $ref name of its schema — catching schemas
// referenced only from a path, not registered under components/schemas.
func collectResponseArrays(model *v3.Document, acc *arrayFieldAcc) {
	if model.Paths == nil || model.Paths.PathItems == nil {
		return
	}
	for pair := model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		for opPair := pair.Value().GetOperations().First(); opPair != nil; opPair = opPair.Next() {
			collectOperationResponseArrays(opPair.Value(), acc)
		}
	}
}

func collectOperationResponseArrays(op *v3.Operation, acc *arrayFieldAcc) {
	if op == nil || op.Responses == nil || op.Responses.Codes == nil {
		return
	}
	for rPair := op.Responses.Codes.First(); rPair != nil; rPair = rPair.Next() {
		resp := rPair.Value()
		if resp == nil || resp.Content == nil {
			continue
		}
		for cPair := resp.Content.First(); cPair != nil; cPair = cPair.Next() {
			if mt := cPair.Value(); mt != nil && mt.Schema != nil {
				acc.add(refName(mt.Schema), topLevelRequiredArrays(mt.Schema))
			}
		}
	}
}

// topLevelRequiredArrays returns the required, non-nullable array property
// names declared directly on the schema behind proxy.
func topLevelRequiredArrays(proxy *base.SchemaProxy) []string {
	if proxy == nil {
		return nil
	}
	sch := proxy.Schema()
	if sch == nil {
		return nil
	}
	if len(sch.Required) == 0 || sch.Properties == nil {
		return nil
	}
	required := make(map[string]struct{}, len(sch.Required))
	for _, r := range sch.Required {
		required[r] = struct{}{}
	}

	var arrs []string
	for pp := sch.Properties.First(); pp != nil; pp = pp.Next() {
		pname := pp.Key()
		if _, ok := required[pname]; !ok {
			continue
		}
		psch := pp.Value().Schema()
		if psch == nil {
			continue
		}
		if !typeContains(psch.Type, "array") {
			continue
		}
		if isNullableSchema(psch.Type, psch.Nullable) {
			continue
		}
		arrs = append(arrs, pname)
	}
	return arrs
}

// refName extracts the schema name from a $ref (the segment after the last
// "/"), e.g. "./schemas/x.yaml#/FooResponse" -> "FooResponse". An inline
// (non-$ref) schema has no stable name and returns "".
func refName(proxy *base.SchemaProxy) string {
	if proxy == nil || !proxy.IsReference() {
		return ""
	}
	ref := proxy.GetReference()
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

func typeContains(types []string, want string) bool {
	for _, t := range types {
		if t == want {
			return true
		}
	}
	return false
}

// isNullableSchema reports whether an array property is legitimately nullable —
// either an OpenAPI 3.0 `nullable: true` flag or a 3.1 `["array","null"]` type
// union — in which case a nil value may serialize as `null` per the spec.
func isNullableSchema(types []string, nullable *bool) bool {
	if nullable != nil && *nullable {
		return true
	}
	return typeContains(types, "null")
}
