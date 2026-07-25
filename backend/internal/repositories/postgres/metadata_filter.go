package postgres

import (
	"encoding/json"
	"strings"

	"github.com/Masterminds/squirrel"

	"github.com/vibexp/vibexp/internal/repositories"
)

// maxMetadataValueProbes caps how many containment probes a single filter value
// expands into. Two are always emitted (the scalar and single-element-array
// forms) plus at most one typed pair, so the cap holds by construction; it is
// enforced anyway so a future probe form cannot quietly blow up the query.
const maxMetadataValueProbes = 4

// metadataContainment renders a parsed metadata filter as a squirrel predicate
// using JSONB containment (`@>`) and `jsonb_exists`, both of which the default
// jsonb_ops GIN indexes on artifacts.metadata, blueprints.metadata and
// memories.metadata can serve. Keys are ANDed, values within a key ORed.
//
// It returns nil for an empty filter — callers must not append a nil predicate,
// because squirrel writes a dangling " WHERE " for a conjunction that renders
// to nothing.
//
// column is a column reference interpolated into the SQL text (for example
// "a.metadata"); it must be a compile-time constant, never user input. Keys and
// values are always bound as parameters.
func metadataContainment(column string, filter repositories.MetadataFilter) squirrel.Sqlizer {
	if len(filter) == 0 {
		return nil
	}

	conditions := make(squirrel.And, 0, len(filter))
	for _, key := range filter.SortedKeys() {
		values := filter[key]

		// No values means "the row has this key at all". The JSONB `?` operator
		// would express that, but squirrel's Dollar placeholder format rewrites
		// every `?` in the SQL text into $n, so a literal operator would be
		// mangled. jsonb_exists is the function form, index-backed just the same
		// and fully parameter-bound.
		if len(values) == 0 {
			conditions = append(conditions, squirrel.Expr("jsonb_exists("+column+", ?)", key))
			continue
		}

		alternatives := make(squirrel.Or, 0, len(values)*maxMetadataValueProbes)
		for _, value := range values {
			for _, probe := range metadataProbes(key, value) {
				alternatives = append(alternatives, squirrel.Expr(column+" @> ?::jsonb", probe))
			}
		}
		if len(alternatives) == 0 {
			continue
		}
		conditions = append(conditions, alternatives)
	}

	if len(conditions) == 0 {
		return nil
	}

	return conditions
}

// metadataProbes builds the containment probe documents for one key/value pair.
//
// Containment is type-strict and metadata values may be stored either as a
// scalar or inside an array, so a single filter value expands into several
// probes: the string scalar, the single-element string array, and — when the
// value unambiguously denotes a number or a boolean — the same pair with that
// type. Without the typed probes a value stored as the number 3 would never
// match the filter value "3".
func metadataProbes(key, value string) []string {
	probes := make([]string, 0, maxMetadataValueProbes)

	probes = appendMetadataProbe(probes, key, value)
	probes = appendMetadataProbe(probes, key, []string{value})

	if number, ok := metadataNumberToken(value); ok {
		probes = appendMetadataProbe(probes, key, number)
		probes = appendMetadataProbe(probes, key, []json.RawMessage{number})
	} else if boolean, ok := metadataBoolToken(value); ok {
		probes = appendMetadataProbe(probes, key, boolean)
		probes = appendMetadataProbe(probes, key, []bool{boolean})
	}

	return probes
}

// appendMetadataProbe marshals {key: value} and appends it, respecting the
// per-value probe cap. A value that cannot be marshaled is skipped rather than
// failing the query — every value reaching here is a string, number, bool or a
// slice of those, so this is defensive only.
func appendMetadataProbe(probes []string, key string, value any) []string {
	if len(probes) >= maxMetadataValueProbes {
		return probes
	}
	encoded, err := json.Marshal(map[string]any{key: value})
	if err != nil {
		return probes
	}
	return append(probes, string(encoded))
}

// metadataNumberToken reports whether the value is a canonical JSON number and,
// if so, returns it verbatim so the probe preserves the caller's exact
// representation (3.10 stays 3.10). Forms JSON rejects — leading zeros, a
// trailing plus, hexadecimal — are not numbers here.
func metadataNumberToken(value string) (json.RawMessage, bool) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, false
	}
	if decoder.More() {
		return nil, false
	}

	number, ok := decoded.(json.Number)
	if !ok {
		return nil, false
	}
	return json.RawMessage(number.String()), true
}

// metadataBoolToken recognises only the two JSON boolean literals. strconv's
// ParseBool would also accept "1", "t" and friends, which would make a filter
// for the string "1" probe for `true` — surprising, and it would compete with
// the numeric probe for the per-value cap.
func metadataBoolToken(value string) (bool, bool) {
	switch value {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}
