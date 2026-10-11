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
// vocabulary -- the last one #912 could not replace with a $ref, because a
// struct tag cannot be computed.
//
// So it is pinned instead, on the half a tag can carry: the SYSTEM values. A
// tag must name every system status of its resource type and none of another
// type's. That catches both drift directions -- a value dropped from a
// description (agents stop using a status that works) and a value added to one
// (agents send a status the service rejects).
//
// A team's custom statuses (epic #1263) cannot be in a tag at all, so each tag
// must also name vibexp_io_list_statuses, the tool that lists them (#1268).
// Without that pointer an agent has no way to learn a custom status exists.
func TestMCPStatusDescriptionsNameSystemStatusesAndTheDiscoveryTool(t *testing.T) {
	cases := []struct {
		name   string
		typ    reflect.Type
		system []string
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

			assert.ElementsMatch(t, tc.system, mentioned,
				"%s.Status's description names the system statuses %v, but the "+
					"resource type's are %v. A struct tag cannot be built from the "+
					"allowlist -- so it is pinned here instead (#912). Description was: %q",
				tc.name, mentioned, tc.system, description)

			assert.Contains(t, description, listStatusesToolName,
				"%s.Status's description must point at %s: a team's custom statuses "+
					"cannot be written into a struct tag, and that tool is the only way "+
					"an agent learns they exist (#1268). Description was: %q",
				tc.name, listStatusesToolName, description)
		})
	}
}
