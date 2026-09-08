package models

import (
	"encoding/json"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLabelListMarshalsEmptyArrayNotNull is the whole reason LabelList exists:
// `labels` is a REQUIRED array in the artifact/blueprint/memory response
// schemas, so a resource with no labels must serialize as [] rather than null.
// Reverting the field to pq.StringArray makes this fail (nil -> `null`).
func TestLabelListMarshalsEmptyArrayNotNull(t *testing.T) {
	t.Run("nil marshals as an empty array", func(t *testing.T) {
		var labels LabelList
		out, err := json.Marshal(labels)
		require.NoError(t, err)
		assert.JSONEq(t, `[]`, string(out))
		assert.NotContains(t, string(out), "null")
	})

	t.Run("empty marshals as an empty array", func(t *testing.T) {
		out, err := json.Marshal(LabelList{})
		require.NoError(t, err)
		assert.JSONEq(t, `[]`, string(out))
	})

	t.Run("values marshal like a plain slice", func(t *testing.T) {
		out, err := json.Marshal(LabelList{"onboarding", "api"})
		require.NoError(t, err)
		assert.JSONEq(t, `["onboarding","api"]`, string(out))
	})

	t.Run("a nil field on a resource still emits the key as []", func(t *testing.T) {
		for name, payload := range map[string]any{
			"artifact":  Artifact{},
			"blueprint": Blueprint{},
			"memory":    Memory{},
		} {
			out, err := json.Marshal(payload)
			require.NoError(t, err, name)

			var decoded map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(out, &decoded), name)
			require.Contains(t, decoded, "labels", name)
			assert.JSONEq(t, `[]`, string(decoded["labels"]), name)
		}
	})
}

// TestLabelListRoundTripsPostgresTextArray pins the other half of the type: it
// still has to behave like pq.StringArray against a text[] column, which
// models.JSONArray[string] cannot (it implements neither Scanner nor Valuer).
func TestLabelListRoundTripsPostgresTextArray(t *testing.T) {
	t.Run("scan reads a text[] literal", func(t *testing.T) {
		var labels LabelList
		require.NoError(t, labels.Scan([]byte(`{onboarding,api}`)))
		assert.Equal(t, LabelList{"onboarding", "api"}, labels)
	})

	t.Run("scan reads an empty text[] literal", func(t *testing.T) {
		var labels LabelList
		require.NoError(t, labels.Scan([]byte(`{}`)))
		assert.Empty(t, labels)
	})

	t.Run("value writes a text[] literal", func(t *testing.T) {
		got, err := LabelList{"onboarding", "api"}.Value()
		require.NoError(t, err)
		want, err := pq.StringArray{"onboarding", "api"}.Value()
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("a nil value writes an empty array, never NULL", func(t *testing.T) {
		var labels LabelList
		got, err := labels.Value()
		require.NoError(t, err)
		assert.NotNil(t, got, "a NULL labels column would break the NOT NULL constraint and the [] wire contract")
		assert.Equal(t, "{}", got)
	})

	t.Run("scan rejects a malformed literal", func(t *testing.T) {
		var labels LabelList
		assert.Error(t, labels.Scan(42))
	})
}
