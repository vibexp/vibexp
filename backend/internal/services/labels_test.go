package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

func TestNormalizeLabels(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  models.LabelList
	}{
		{name: "nil yields a non-nil empty list", input: nil, want: models.LabelList{}},
		{name: "trims surrounding whitespace", input: []string{"  api "}, want: models.LabelList{"api"}},
		{name: "drops empty and whitespace-only entries", input: []string{"api", "", "   "}, want: models.LabelList{"api"}},
		{
			name:  "collapses duplicates to the first occurrence",
			input: []string{"api", "onboarding", "api"},
			want:  models.LabelList{"api", "onboarding"},
		},
		{
			name:  "caps the list at MaxLabels",
			input: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"},
			want:  models.LabelList{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
		},
		{
			name:  "truncates an over-long label to MaxLabelLength",
			input: []string{strings.Repeat("x", 60)},
			want:  models.LabelList{strings.Repeat("x", models.MaxLabelLength)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeLabels(tc.input)
			assert.Equal(t, tc.want, got)
			assert.NotNil(t, got, "a nil result would serialize as null and violate the required-array contract")
		})
	}

	t.Run("counts runes, not bytes, when truncating", func(t *testing.T) {
		got := normalizeLabels([]string{strings.Repeat("é", 60)})
		require.Len(t, got, 1)
		assert.Equal(t, models.MaxLabelLength, len([]rune(got[0])))
	})
}

func TestValidateLabels(t *testing.T) {
	t.Run("accepts a list at the limits", func(t *testing.T) {
		labels := make([]string, models.MaxLabels)
		for i := range labels {
			labels[i] = strings.Repeat("x", models.MaxLabelLength)
		}
		assert.NoError(t, validateLabels(labels))
	})

	t.Run("accepts nil", func(t *testing.T) {
		assert.NoError(t, validateLabels(nil))
	})

	t.Run("rejects one label too many", func(t *testing.T) {
		labels := make([]string, models.MaxLabels+1)
		for i := range labels {
			labels[i] = "x"
		}
		err := validateLabels(labels)
		require.ErrorIs(t, err, ErrInvalidLabels)
		assert.Contains(t, err.Error(), "at most 10 labels")
	})

	t.Run("rejects one character too many", func(t *testing.T) {
		err := validateLabels([]string{strings.Repeat("x", models.MaxLabelLength+1)})
		require.ErrorIs(t, err, ErrInvalidLabels)
		assert.Contains(t, err.Error(), "at most 50 characters")
	})
}

func TestFoldLegacyMemoryTags(t *testing.T) {
	t.Run("moves a JSON tags array into labels and drops the key", func(t *testing.T) {
		metadata := map[string]interface{}{
			"tags":     []interface{}{"onboarding", "api"},
			"priority": "high",
		}

		labels, out, hasTaxonomy := foldLegacyMemoryTags(nil, metadata)

		assert.Equal(t, models.LabelList{"onboarding", "api"}, labels)
		assert.True(t, hasTaxonomy)
		assert.NotContains(t, out, "tags", "the write path must not re-create the key migration 016 removed")
		assert.Equal(t, "high", out["priority"], "other metadata keys survive untouched")
		assert.Contains(t, metadata, "tags", "the caller's map must not be mutated in place")
	})

	t.Run("merges explicit labels ahead of legacy tags and de-duplicates", func(t *testing.T) {
		labels, _, hasTaxonomy := foldLegacyMemoryTags(
			[]string{"api"},
			map[string]interface{}{"tags": []interface{}{"api", "onboarding"}},
		)
		assert.Equal(t, models.LabelList{"api", "onboarding"}, labels)
		assert.True(t, hasTaxonomy)
	})

	t.Run("caps a legacy list that never passed validation", func(t *testing.T) {
		tags := make([]interface{}, 0, 15)
		for i := range 15 {
			tags = append(tags, string(rune('a'+i)))
		}
		labels, _, _ := foldLegacyMemoryTags(nil, map[string]interface{}{"tags": tags})
		assert.Len(t, labels, models.MaxLabels)
	})

	t.Run("accepts a []string tags value too", func(t *testing.T) {
		labels, out, hasTaxonomy := foldLegacyMemoryTags(nil, map[string]interface{}{"tags": []string{"api"}})
		assert.Equal(t, models.LabelList{"api"}, labels)
		assert.NotContains(t, out, "tags")
		assert.True(t, hasTaxonomy)
	})

	t.Run("leaves a non-array tags value exactly where it is", func(t *testing.T) {
		metadata := map[string]interface{}{"tags": "not-an-array"}
		labels, out, hasTaxonomy := foldLegacyMemoryTags(nil, metadata)

		assert.Empty(t, labels)
		assert.False(t, hasTaxonomy)
		assert.Equal(t, "not-an-array", out["tags"], "an ordinary metadata entry that shares the name is not taxonomy")
	})

	t.Run("leaves a mixed-type array where it is", func(t *testing.T) {
		metadata := map[string]interface{}{"tags": []interface{}{"api", 42}}
		_, out, hasTaxonomy := foldLegacyMemoryTags(nil, metadata)

		assert.False(t, hasTaxonomy)
		assert.Contains(t, out, "tags")
	})

	t.Run("reports no taxonomy when neither labels nor tags are supplied", func(t *testing.T) {
		labels, out, hasTaxonomy := foldLegacyMemoryTags(nil, map[string]interface{}{"priority": "high"})
		assert.Empty(t, labels)
		assert.False(t, hasTaxonomy, "a partial update with no taxonomy must not clear existing labels")
		assert.Contains(t, out, "priority")
	})

	t.Run("an explicit empty label list is still a taxonomy edit", func(t *testing.T) {
		_, _, hasTaxonomy := foldLegacyMemoryTags([]string{}, nil)
		assert.True(t, hasTaxonomy, "clearing all labels must be distinguishable from not touching them")
	})

	t.Run("an empty tags array clears the labels", func(t *testing.T) {
		labels, out, hasTaxonomy := foldLegacyMemoryTags(nil, map[string]interface{}{"tags": []interface{}{}})
		assert.Empty(t, labels)
		assert.True(t, hasTaxonomy)
		assert.NotContains(t, out, "tags")
	})
}
