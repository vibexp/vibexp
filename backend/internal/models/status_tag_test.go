package models

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The `validate:"omitempty,oneof=..."` tag on each request model's Status field
// is documentation, not enforcement -- validate.Struct is never called on these
// four domains, which is why the check lives in the services (see
// services/status.go). Documentation that nothing checks is how the three
// vocabularies drifted apart in the first place, so the tags are pinned to the
// allowlists here rather than deleted: they are the shape a reader sees first
// (#912).
func TestStatusValidateTagsMatchTheAllowlists(t *testing.T) {
	cases := []struct {
		name    string
		typ     reflect.Type
		allowed []string
	}{
		{"CreatePromptRequest", reflect.TypeOf(CreatePromptRequest{}), PromptStatuses},
		{"UpdatePromptRequest", reflect.TypeOf(UpdatePromptRequest{}), PromptStatuses},
		{"CreateArtifactRequest", reflect.TypeOf(CreateArtifactRequest{}), ArtifactStatuses},
		{"UpdateArtifactRequest", reflect.TypeOf(UpdateArtifactRequest{}), ArtifactStatuses},
		{"CreateBlueprintRequest", reflect.TypeOf(CreateBlueprintRequest{}), BlueprintStatuses},
		{"UpdateBlueprintRequest", reflect.TypeOf(UpdateBlueprintRequest{}), BlueprintStatuses},
		{"CreateMemoryRequest", reflect.TypeOf(CreateMemoryRequest{}), MemoryStatuses},
		{"UpdateMemoryRequest", reflect.TypeOf(UpdateMemoryRequest{}), MemoryStatuses},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			field, ok := tc.typ.FieldByName("Status")
			require.True(t, ok, "%s has no Status field", tc.name)

			documented := oneofValues(field.Tag.Get("validate"))
			require.NotEmpty(t, documented,
				"%s.Status lost its validate:\"oneof=...\" tag; either restore it or "+
					"remove this case, but do not leave an unpinned copy behind", tc.name)

			assert.ElementsMatch(t, tc.allowed, documented,
				"%s.Status's validate tag has drifted from its allowlist in "+
					"models/status.go. Nothing calls validate.Struct on this type, so the "+
					"tag is read by humans only -- and a stale one is worse than none (#912).",
				tc.name)
		})
	}
}

// oneofValues extracts the values of a go-playground `oneof=a b c` rule.
func oneofValues(tag string) []string {
	for _, rule := range strings.Split(tag, ",") {
		if after, found := strings.CutPrefix(rule, "oneof="); found {
			return strings.Fields(after)
		}
	}
	return nil
}
