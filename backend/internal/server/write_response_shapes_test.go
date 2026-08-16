package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// keysOf marshals a value and returns its top-level object keys.
func keysOf(t *testing.T, v any) map[string]struct{} {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)

	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &obj))

	keys := make(map[string]struct{}, len(obj))
	for k := range obj {
		keys[k] = struct{}{}
	}
	return keys
}

func populatedArtifact() *models.Artifact {
	return &models.Artifact{
		ID: "a-1", TeamID: "team-1", ProjectID: "p-1", Slug: "s", UserID: "u-1",
		Title: "T", Type: "general", Status: "active", Content: "body", Version: 7,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
}

func populatedBlueprint() *models.Blueprint {
	return &models.Blueprint{
		ID: "b-1", TeamID: "team-1", ProjectID: "p-1", Slug: "s", UserID: "u-1",
		Title: "T", Type: "general", Status: "active", Content: "body", Version: 7,
		Path: ".claude/s.md", RawContent: "---\nname: s\n---\nbody",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
}

// The write bodies drop exactly `team_id` and `version` — the two keys the
// models emit that `schemas/{artifacts,blueprints}.yaml` never declared, and
// which the converted READ paths therefore already stopped emitting (#800).
//
// Both halves matter. Asserting only that the keys are gone would pass against
// a model that never had them; asserting only that the model HAS them would
// pass against a wrapper that does nothing. The first sub-assertion is what
// makes the second one meaningful — and it is not hypothetical: the obvious
// spelling for this wrapper, shadowing with `json:"-"`, silently emits both
// keys anyway, because such fields are excluded from encoding/json's conflict
// resolution and the embedded field wins.
func TestWriteResponseShapesDropUndeclaredKeys(t *testing.T) {
	t.Run("artifact", func(t *testing.T) {
		model := populatedArtifact()
		modelKeys := keysOf(t, model)
		require.Contains(t, modelKeys, "team_id", "fixture: the model must still emit it, or this proves nothing")
		require.Contains(t, modelKeys, "version", "fixture: the model must still emit it, or this proves nothing")

		bodyKeys := keysOf(t, artifactWriteBody(model))
		assert.NotContains(t, bodyKeys, "team_id")
		assert.NotContains(t, bodyKeys, "version")

		// Everything else is byte-identical: this narrows the body to the
		// documented contract, it does not reshape it.
		delete(modelKeys, "team_id")
		delete(modelKeys, "version")
		assert.Equal(t, modelKeys, bodyKeys, "no other key may change")
	})

	t.Run("blueprint", func(t *testing.T) {
		model := populatedBlueprint()
		modelKeys := keysOf(t, model)
		require.Contains(t, modelKeys, "team_id", "fixture: the model must still emit it, or this proves nothing")
		require.Contains(t, modelKeys, "version", "fixture: the model must still emit it, or this proves nothing")

		bodyKeys := keysOf(t, blueprintWriteBody(model))
		assert.NotContains(t, bodyKeys, "team_id")
		assert.NotContains(t, bodyKeys, "version")

		// raw_content in particular must SURVIVE. It is undeclared on the
		// `Blueprint` schema these write ops return, so a wrapper built by
		// converting to the generated type would drop it — but the write paths
		// populate it (services.regenerateRaw) and only `team_id`/`version` were
		// decided on #800, blast radius included. Widening the removal to a field
		// blueprint sync may depend on is a separate decision.
		assert.Contains(t, bodyKeys, "raw_content")

		delete(modelKeys, "team_id")
		delete(modelKeys, "version")
		assert.Equal(t, modelKeys, bodyKeys, "no other key may change")
	})
}

// The values that survive must survive unchanged — a wrapper that dropped the
// two keys but also blanked a field would satisfy the key-set assertions above.
func TestWriteResponseShapesPreserveValues(t *testing.T) {
	model := populatedArtifact()

	raw, err := json.Marshal(artifactWriteBody(model))
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))

	assert.Equal(t, "a-1", body["id"])
	assert.Equal(t, "p-1", body["project_id"])
	assert.Equal(t, "s", body["slug"])
	assert.Equal(t, "T", body["title"])
	assert.Equal(t, "body", body["content"])
	assert.Equal(t, "active", body["status"])
}
