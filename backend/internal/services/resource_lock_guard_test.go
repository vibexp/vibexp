package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

func guardTestLock() *models.ResourceLock {
	return &models.ResourceLock{LockedBy: "user-locker", LockedByName: "Ada Lovelace", Comment: "Used by the release agent"}
}

func TestResourceLockedError_Message(t *testing.T) {
	tests := []struct {
		name string
		err  ResourceLockedError
		want string
	}{
		{
			name: "update names the field, the locker and the comment",
			err: ResourceLockedError{
				ResourceType: "prompt", Field: "status", LockedByName: "Ada Lovelace", Comment: "Used by the release agent",
			},
			want: `cannot change status: this prompt is locked by Ada Lovelace: "Used by the release agent"`,
		},
		{
			name: "delete has no field",
			err:  ResourceLockedError{ResourceType: "artifact", LockedByName: "Ada Lovelace", Comment: "keep"},
			want: `cannot delete: this artifact is locked by Ada Lovelace: "keep"`,
		},
		{
			name: "a locker without a display name and a lock without a comment still read as a sentence",
			err:  ResourceLockedError{ResourceType: "blueprint"},
			want: "cannot delete: this blueprint is locked by another member",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Error())
		})
	}
}

func TestGuardLockedDelete(t *testing.T) {
	require.NoError(t, guardLockedDelete(models.RelationResourceTypePrompt, nil))

	err := guardLockedDelete(models.RelationResourceTypePrompt, guardTestLock())
	var locked *ResourceLockedError
	require.ErrorAs(t, err, &locked)
	assert.Equal(t, ResourceLockedError{
		ResourceType: "prompt", LockedBy: "user-locker", LockedByName: "Ada Lovelace",
		Comment: "Used by the release agent",
	}, *locked)
}

func TestStatusChanged(t *testing.T) {
	empty, same, other := "", "active", "archived"
	assert.False(t, statusChanged(nil, "active"), "not sent")
	assert.False(t, statusChanged(&empty, "active"), "an empty status is ignored by the update paths")
	assert.False(t, statusChanged(&same, "active"))
	assert.True(t, statusChanged(&other, "active"))
}

func TestLabelsChanged(t *testing.T) {
	tests := []struct {
		name    string
		sent    []string
		current []string
		want    bool
	}{
		{"not sent", nil, []string{"a"}, false},
		{"same order", []string{"a", "b"}, []string{"a", "b"}, false},
		{"other order", []string{"b", "a"}, []string{"a", "b"}, false},
		{"empty list on a resource without labels", []string{}, nil, false},
		{"label removed", []string{"a"}, []string{"a", "b"}, true},
		{"label added", []string{"a", "b", "c"}, []string{"a", "b"}, true},
		{"emptied", []string{}, []string{"a"}, true},
		{"same size, other label", []string{"a", "c"}, []string{"a", "b"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, labelsChanged(tt.sent, tt.current))
		})
	}
}

func TestLabelsChanged_LeavesTheCallersOrderAlone(t *testing.T) {
	sent, current := []string{"b", "a"}, []string{"z", "a"}
	labelsChanged(sent, current)
	assert.Equal(t, []string{"b", "a"}, sent)
	assert.Equal(t, []string{"z", "a"}, current)
}

func TestNormalizedLabelsOrNil(t *testing.T) {
	assert.Nil(t, normalizedLabelsOrNil(nil), "nil stays nil so it still reads as not sent")
	assert.Equal(t, []string{"a", "b"}, normalizedLabelsOrNil([]string{" a ", "b", "a", ""}))
	assert.NotNil(t, normalizedLabelsOrNil([]string{}), "an empty list is a request to clear")
}

func TestMetadataChanged(t *testing.T) {
	stored := map[string]interface{}{
		"model": "x", "retries": 3, "nested": map[string]interface{}{"on": true, "ids": []interface{}{1, 2}},
	}
	tests := []struct {
		name    string
		sent    map[string]interface{}
		current map[string]interface{}
		want    bool
	}{
		{"not sent", nil, stored, false},
		{"empty map on a resource without metadata", map[string]interface{}{}, nil, false},
		{
			name: "same document decoded from JSON: numbers are float64",
			sent: map[string]interface{}{
				"nested":  map[string]interface{}{"ids": []interface{}{float64(1), float64(2)}, "on": true},
				"retries": float64(3), "model": "x",
			},
			current: stored,
			want:    false,
		},
		{"value changed", map[string]interface{}{"model": "y", "retries": 3}, stored, true},
		{"key removed", map[string]interface{}{"model": "x"}, map[string]interface{}{"model": "x", "retries": 3}, true},
		{"key added", map[string]interface{}{"model": "x"}, nil, true},
		{"emptied", map[string]interface{}{}, stored, true},
		{"unencodable metadata fails closed", map[string]interface{}{"f": func() {}}, stored, true},
		{
			name:    "unencodable stored metadata fails closed",
			sent:    map[string]interface{}{"model": "x"},
			current: map[string]interface{}{"f": func() {}},
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, metadataChanged(tt.sent, tt.current))
		})
	}
}
