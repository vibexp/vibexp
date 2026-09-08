package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// OptionalString exists to carry the three states of a nullable field in a
// partial update body (issue #911). These pin all three, in both directions.

func TestOptionalString_DecodesThreeStates(t *testing.T) {
	type body struct {
		Title OptionalString `json:"title,omitzero"`
	}

	t.Run("absent key leaves it unset", func(t *testing.T) {
		var b body
		require.NoError(t, json.Unmarshal([]byte(`{}`), &b))
		assert.False(t, b.Title.Set)
		assert.Nil(t, b.Title.Value)
		assert.True(t, b.Title.IsZero())
	})

	t.Run("explicit null is set with no value", func(t *testing.T) {
		var b body
		require.NoError(t, json.Unmarshal([]byte(`{"title":null}`), &b))
		assert.True(t, b.Title.Set, "null must be distinguishable from an absent key")
		assert.Nil(t, b.Title.Value)
		assert.False(t, b.Title.IsZero())
	})

	t.Run("a value is set with that value", func(t *testing.T) {
		var b body
		require.NoError(t, json.Unmarshal([]byte(`{"title":"Deploy checklist"}`), &b))
		require.NotNil(t, b.Title.Value)
		assert.Equal(t, "Deploy checklist", *b.Title.Value)
	})

	t.Run("a non-string value is rejected", func(t *testing.T) {
		var b body
		assert.Error(t, json.Unmarshal([]byte(`{"title":42}`), &b))
	})
}

// omitzero is what keeps a caller that builds the struct in Go and marshals it
// producing byte-identical JSON to before the field existed.
func TestOptionalString_MarshalsBack(t *testing.T) {
	type body struct {
		Title OptionalString `json:"title,omitzero"`
	}

	unset, err := json.Marshal(body{})
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(unset))

	cleared, err := json.Marshal(body{Title: ClearedString()})
	require.NoError(t, err)
	assert.JSONEq(t, `{"title":null}`, string(cleared))

	set, err := json.Marshal(body{Title: NewOptionalString("Deploy checklist")})
	require.NoError(t, err)
	assert.JSONEq(t, `{"title":"Deploy checklist"}`, string(set))
}

// The memory response must carry `title` as an always-present nullable key:
// `title` is REQUIRED in the schema so the SPA can rely on it, and null (not
// "") is what says "untitled, derive one from the first heading".
func TestMemory_TitleAlwaysSerializes(t *testing.T) {
	raw, err := json.Marshal(Memory{})
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"title":null`)

	title := "Deploy checklist"
	raw, err = json.Marshal(Memory{Title: &title})
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"title":"Deploy checklist"`)
}
