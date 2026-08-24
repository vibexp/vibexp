package specconformance

import (
	"fmt"

	base "github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// ArrayItemEnum returns the enum values declared on the `items` of an array
// property of a component schema, in spec order — e.g.
// ArrayItemEnum("Team", "permissions").
//
// It exists so a test can prove that a documented closed set of strings still
// matches the Go constants that produce it: the spec is one half of that
// comparison and this is how the test reads it. Every failure mode (unknown
// schema, unknown property, a property that is no longer an array, missing or
// non-scalar enum) returns an error rather than an empty slice, so a rename or
// an accidental deletion fails loudly instead of silently comparing against
// nothing.
//
// Access is guarded by validateMu because resolving schema proxies mutates the
// shared *v3.Document (see RequiredArrayFields).
func ArrayItemEnum(schema, property string) ([]string, error) {
	s, err := load()
	if err != nil {
		return nil, err
	}

	validateMu.Lock()
	defer validateMu.Unlock()

	items, err := arrayItemsSchema(s.model, schema, property)
	if err != nil {
		return nil, err
	}
	return scalarEnum(items, fmt.Sprintf("schema %q property %q", schema, property))
}

// ComponentEnum returns the enum values declared by a named component schema
// itself, in spec order — e.g. ComponentEnum("FreshnessMetricsRange").
//
// It is the sibling of ArrayItemEnum for the case where the schema IS the enum:
// a standalone string enum used as a query parameter or a field type. Those are
// mirrored by hand on the Go side as a validation allowlist keyed on the same
// strings, and oapi-codegen does not validate a query parameter's enum — it
// binds the raw string — so the Go allowlist is the sole enforcement point and
// the spec is only documentation. Without a test comparing the two they drift
// in both directions: a value added to the spec alone is documented but always
// 400s, and a value removed from the spec alone keeps working undocumented
// (#774).
//
// Every failure mode (unknown schema, a schema that declares no enum, a
// non-scalar enum entry) returns an error rather than an empty slice, so a
// renamed or deleted schema fails loudly instead of making the caller's
// assertion vacuous by comparing against nothing.
//
// Access is guarded by validateMu because resolving schema proxies mutates the
// shared *v3.Document (see RequiredArrayFields).
func ComponentEnum(schema string) ([]string, error) {
	s, err := load()
	if err != nil {
		return nil, err
	}

	validateMu.Lock()
	defer validateMu.Unlock()

	proxy, err := componentSchema(s.model, schema)
	if err != nil {
		return nil, err
	}
	return scalarEnum(proxy, fmt.Sprintf("component schema %q", schema))
}

// componentSchema resolves a named component schema.
func componentSchema(model *v3.Document, schema string) (*base.SchemaProxy, error) {
	if model.Components == nil || model.Components.Schemas == nil {
		return nil, fmt.Errorf("spec has no component schemas")
	}
	proxy, ok := model.Components.Schemas.Get(schema)
	if !ok || proxy == nil {
		return nil, fmt.Errorf("component schema %q not found in spec", schema)
	}
	return proxy, nil
}

// componentProperty resolves a named property of a named component schema.
func componentProperty(model *v3.Document, schema, property string) (*base.SchemaProxy, error) {
	proxy, err := componentSchema(model, schema)
	if err != nil {
		return nil, err
	}
	sch := proxy.Schema()
	if sch == nil || sch.Properties == nil {
		return nil, fmt.Errorf("component schema %q has no properties", schema)
	}
	prop, ok := sch.Properties.Get(property)
	if !ok || prop == nil {
		return nil, fmt.Errorf("schema %q has no property %q", schema, property)
	}
	return prop, nil
}

// arrayItemsSchema resolves the items schema of an array property.
func arrayItemsSchema(model *v3.Document, schema, property string) (*base.SchemaProxy, error) {
	prop, err := componentProperty(model, schema, property)
	if err != nil {
		return nil, err
	}
	psch := prop.Schema()
	if psch == nil {
		return nil, fmt.Errorf("schema %q property %q has no schema", schema, property)
	}
	if !typeContains(psch.Type, "array") {
		return nil, fmt.Errorf("schema %q property %q is not an array (type %v)", schema, property, psch.Type)
	}
	if psch.Items == nil || !psch.Items.IsA() {
		return nil, fmt.Errorf("schema %q property %q has no items schema", schema, property)
	}
	return psch.Items.A, nil
}

// scalarEnum extracts the scalar enum values from the schema behind proxy.
// subject names what was being read (e.g. `component schema "X"`) and is used
// verbatim in every error, so a caller's failure says which spec location was
// wrong rather than just that something was empty.
func scalarEnum(proxy *base.SchemaProxy, subject string) ([]string, error) {
	if proxy == nil {
		return nil, fmt.Errorf("%s: nil schema", subject)
	}
	sch := proxy.Schema()
	if sch == nil {
		return nil, fmt.Errorf("%s: schema did not resolve", subject)
	}
	if len(sch.Enum) == 0 {
		return nil, fmt.Errorf("%s declares no enum", subject)
	}

	out := make([]string, 0, len(sch.Enum))
	for i, node := range sch.Enum {
		if node == nil || node.Value == "" {
			return nil, fmt.Errorf("%s: enum entry %d is not a scalar value", subject, i)
		}
		out = append(out, node.Value)
	}
	return out, nil
}
