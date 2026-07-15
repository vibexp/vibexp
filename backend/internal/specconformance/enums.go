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
	return scalarEnum(items, schema, property)
}

// componentProperty resolves a named property of a named component schema.
func componentProperty(model *v3.Document, schema, property string) (*base.SchemaProxy, error) {
	if model.Components == nil || model.Components.Schemas == nil {
		return nil, fmt.Errorf("spec has no component schemas")
	}
	proxy, ok := model.Components.Schemas.Get(schema)
	if !ok || proxy == nil {
		return nil, fmt.Errorf("component schema %q not found in spec", schema)
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
func scalarEnum(proxy *base.SchemaProxy, schema, property string) ([]string, error) {
	if proxy == nil {
		return nil, fmt.Errorf("schema %q property %q: nil items schema", schema, property)
	}
	items := proxy.Schema()
	if items == nil {
		return nil, fmt.Errorf("schema %q property %q: items schema did not resolve", schema, property)
	}
	if len(items.Enum) == 0 {
		return nil, fmt.Errorf("schema %q property %q declares no items enum", schema, property)
	}

	out := make([]string, 0, len(items.Enum))
	for i, node := range items.Enum {
		if node == nil || node.Value == "" {
			return nil, fmt.Errorf("schema %q property %q: enum entry %d is not a scalar value", schema, property, i)
		}
		out = append(out, node.Value)
	}
	return out, nil
}
