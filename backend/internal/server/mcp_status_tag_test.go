package server

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// The `jsonschema:` description on each MCP tool's Status parameter is what an
// AI agent reads before choosing a value, and it is a hand-written copy of the
// vocabulary -- the last one this issue could not replace with a $ref, because
// a struct tag cannot be computed (#912).
//
// So it is pinned instead: a tag must name every status its resource type
// accepts and none that it does not. That catches both drift directions -- a
// value dropped from a description (agents stop using a status that works) and
// a value added to one (agents send a status the service now 400s).
func TestMCPStatusDescriptionsMatchTheAllowlists(t *testing.T) {
	cases := []struct {
		name    string
		typ     reflect.Type
		allowed []string
	}{
		{"CreatePromptParams", reflect.TypeOf(CreatePromptParams{}), models.PromptStatuses},
		{"UpdatePromptParams", reflect.TypeOf(UpdatePromptParams{}), models.PromptStatuses},
		{"CreateArtifactParams", reflect.TypeOf(CreateArtifactParams{}), models.ArtifactStatuses},
		{"UpdateArtifactParams", reflect.TypeOf(UpdateArtifactParams{}), models.ArtifactStatuses},
		{"CreateBlueprintParams", reflect.TypeOf(CreateBlueprintParams{}), models.BlueprintStatuses},
		{"UpdateBlueprintParams", reflect.TypeOf(UpdateBlueprintParams{}), models.BlueprintStatuses},
		{"StoreMemoryParams", reflect.TypeOf(StoreMemoryParams{}), models.MemoryStatuses},
		{"UpdateMemoryParams", reflect.TypeOf(UpdateMemoryParams{}), models.MemoryStatuses},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			field, ok := tc.typ.FieldByName("Status")
			require.True(t, ok, "%s has no Status field", tc.name)

			description := field.Tag.Get("jsonschema")
			require.NotEmpty(t, description, "%s.Status has no jsonschema description", tc.name)

			var mentioned []string
			for _, status := range models.ResourceStatuses {
				if strings.Contains(description, status) {
					mentioned = append(mentioned, status)
				}
			}

			assert.ElementsMatch(t, tc.allowed, mentioned,
				"%s.Status's description names %v, but the tool accepts %v. This tag is "+
					"the only description of the vocabulary an MCP client ever sees, and "+
					"a struct tag cannot be built from the allowlist -- so it is pinned "+
					"here instead (#912). Description was: %q",
				tc.name, mentioned, tc.allowed, description)
		})
	}
}
