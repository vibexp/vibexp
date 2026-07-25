package repositories

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// MetadataFilter is a parsed `metadata` list filter: each key maps to the set
// of values that satisfy it. Keys are combined with AND, the values within a
// key with OR, and an empty value slice means "the row has this key at all".
//
// The zero value (nil) is a no-op filter.
type MetadataFilter map[string][]string

// Limits on a single metadata filter. They bound both the size of the query
// the filter expands into and the work Postgres does evaluating it.
const (
	// MaxMetadataFilterKeys is the maximum number of keys in one filter.
	MaxMetadataFilterKeys = 10
	// MaxMetadataFilterValues is the maximum number of values per key.
	MaxMetadataFilterValues = 25
	// MaxMetadataFilterKeyLength is the maximum length of a metadata key. It is
	// deliberately wider than the UI editor's own limit so that keys which are
	// already stored (for example from a GitHub import) stay filterable.
	MaxMetadataFilterKeyLength = 255
	// MaxMetadataFilterValueLength is the maximum length of a metadata value.
	MaxMetadataFilterValueLength = 512
)

// ErrInvalidMetadataFilter wraps every rejection of a metadata filter, so the
// transport layers can map the whole class to one 400 without matching on
// message text.
var ErrInvalidMetadataFilter = errors.New("invalid metadata filter")

// ParseMetadataFilter decodes the REST `metadata` query parameter — a JSON
// object of key to array of string values — and validates it. An empty string
// is not an error: it yields a nil (no-op) filter.
//
// Decoding is deliberately strict about shape so a caller who passes, say, a
// bare string or a scalar value gets told exactly what is wrong rather than
// silently filtering on nothing.
func ParseMetadataFilter(raw string) (MetadataFilter, error) {
	if raw == "" {
		return nil, nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, fmt.Errorf("%w: not valid JSON", ErrInvalidMetadataFilter)
	}

	object, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: must be a JSON object", ErrInvalidMetadataFilter)
	}

	filter := make(MetadataFilter, len(object))
	// Sorted so that a filter with several bad keys always reports the same one.
	for _, key := range sortedAnyMapKeys(object) {
		values, err := metadataFilterValues(key, object[key])
		if err != nil {
			return nil, err
		}
		filter[key] = values
	}

	if err := ValidateMetadataFilter(filter); err != nil {
		return nil, err
	}

	return filter, nil
}

// ValidateMetadataFilter enforces the filter limits on an already-decoded map.
// It is the validation half of ParseMetadataFilter, split out for callers that
// receive the filter as native structured data rather than as a query string
// (the MCP tools do).
func ValidateMetadataFilter(f MetadataFilter) error {
	if len(f) == 0 {
		return nil
	}

	if len(f) > MaxMetadataFilterKeys {
		return fmt.Errorf(
			"%w: at most %d keys are allowed, got %d",
			ErrInvalidMetadataFilter, MaxMetadataFilterKeys, len(f),
		)
	}

	for _, key := range sortedMetadataFilterKeys(f) {
		if key == "" {
			return fmt.Errorf("%w: keys must not be empty", ErrInvalidMetadataFilter)
		}
		if len(key) > MaxMetadataFilterKeyLength {
			return fmt.Errorf(
				"%w: key length must be at most %d characters, got %d",
				ErrInvalidMetadataFilter, MaxMetadataFilterKeyLength, len(key),
			)
		}

		values := f[key]
		if len(values) > MaxMetadataFilterValues {
			return fmt.Errorf(
				"%w: key %q has %d values, at most %d are allowed",
				ErrInvalidMetadataFilter, key, len(values), MaxMetadataFilterValues,
			)
		}

		for _, value := range values {
			if len(value) > MaxMetadataFilterValueLength {
				return fmt.Errorf(
					"%w: a value of key %q is %d characters, at most %d are allowed",
					ErrInvalidMetadataFilter, key, len(value), MaxMetadataFilterValueLength,
				)
			}
		}
	}

	return nil
}

// SortedKeys returns the filter's keys in ascending order, so that a filter
// always renders to the same SQL regardless of map iteration order.
func (f MetadataFilter) SortedKeys() []string {
	return sortedMetadataFilterKeys(f)
}

// metadataFilterValues converts one decoded JSON value into the filter's value
// slice, rejecting anything that is not an array of strings.
func metadataFilterValues(key string, raw any) ([]string, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf(
			"%w: value of key %q must be an array of strings", ErrInvalidMetadataFilter, key,
		)
	}

	values := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf(
				"%w: value of key %q must be an array of strings", ErrInvalidMetadataFilter, key,
			)
		}
		values = append(values, value)
	}

	return values, nil
}

func sortedMetadataFilterKeys(f MetadataFilter) []string {
	keys := make([]string, 0, len(f))
	for key := range f {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedAnyMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
