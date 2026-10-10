package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusResourceTypes(t *testing.T) {
	assert.Equal(t, []string{"prompt", "artifact", "blueprint", "memory"}, StatusResourceTypes)
	for _, resourceType := range StatusResourceTypes {
		assert.True(t, IsStatusResourceType(resourceType), resourceType)
	}
	// The plural spellings the types taxonomy uses are NOT status resource
	// types: the catalog is keyed on the singular.
	for _, resourceType := range []string{"", "prompts", "artifacts", "feed", "Prompt"} {
		assert.False(t, IsStatusResourceType(resourceType), resourceType)
	}
}

func TestSystemStatusesFor(t *testing.T) {
	cases := []struct {
		resourceType string
		want         []string
	}{
		{StatusResourceTypePrompt, PromptStatuses},
		{StatusResourceTypeArtifact, ArtifactStatuses},
		{StatusResourceTypeBlueprint, BlueprintStatuses},
		{StatusResourceTypeMemory, MemoryStatuses},
	}
	for _, tc := range cases {
		t.Run(tc.resourceType, func(t *testing.T) {
			assert.Equal(t, tc.want, SystemStatusesFor(tc.resourceType))
		})
	}

	assert.Nil(t, SystemStatusesFor("artifacts"), "an unsupported type has no system statuses")
}

// A caller appends the team's custom statuses to the result, so it must not
// share a backing array with the package-level subset.
func TestSystemStatusesFor_ReturnsACopy(t *testing.T) {
	before := append([]string(nil), ArtifactStatuses...)

	got := SystemStatusesFor(StatusResourceTypeArtifact)
	got[0] = "mutated"
	_ = append(got[:1], "overwritten")

	assert.Equal(t, before, ArtifactStatuses)
}

func TestDefaultStatusFor(t *testing.T) {
	assert.Equal(t, StatusDraft, DefaultStatusFor(StatusResourceTypePrompt))
	assert.Equal(t, StatusActive, DefaultStatusFor(StatusResourceTypeArtifact))
	assert.Equal(t, StatusActive, DefaultStatusFor(StatusResourceTypeBlueprint))
	assert.Equal(t, StatusActive, DefaultStatusFor(StatusResourceTypeMemory))
	assert.Empty(t, DefaultStatusFor("artifacts"))

	// A default must be one of the type's own system statuses.
	for _, resourceType := range StatusResourceTypes {
		assert.Contains(t, SystemStatusesFor(resourceType), DefaultStatusFor(resourceType), resourceType)
	}
}

func TestReservedStatusSlugs_IsTheWholeVocabulary(t *testing.T) {
	assert.Equal(t, ResourceStatuses, ReservedStatusSlugs)
	for _, resourceType := range StatusResourceTypes {
		for _, slug := range SystemStatusesFor(resourceType) {
			assert.Contains(t, ReservedStatusSlugs, slug, "%s/%s", resourceType, slug)
		}
	}
}

// nonLiveStatuses is the complete list of (resource type, status) pairs that
// are not live. IsLiveStatus must be false for exactly these.
var nonLiveStatuses = map[string][]string{
	StatusResourceTypeArtifact:  {StatusDraft, StatusArchived},
	StatusResourceTypeMemory:    {StatusDraft, StatusArchived},
	StatusResourceTypePrompt:    {StatusDraft},
	StatusResourceTypeBlueprint: {StatusExpired},
}

func TestIsLiveStatus(t *testing.T) {
	// Every system word, a custom slug, an unknown value and the empty string,
	// against every resource type: false only for the pairs listed above.
	candidates := append([]string{"in-review", "Draft", "", "something-else"}, ResourceStatuses...)

	for _, resourceType := range StatusResourceTypes {
		for _, status := range candidates {
			want := !IsAllowedStatus(nonLiveStatuses[resourceType], status)
			assert.Equal(t, want, IsLiveStatus(resourceType, status), "%s/%q", resourceType, status)
		}
	}
}

func TestIsLiveStatus_UnknownResourceTypeIsLive(t *testing.T) {
	for _, status := range ResourceStatuses {
		assert.True(t, IsLiveStatus("artifacts", status), status)
	}
}

// Every non-live status is a system status of its own type: there is no way to
// mark a custom status non-live.
func TestNonLiveStatusesAreSystemStatuses(t *testing.T) {
	for resourceType, statuses := range nonLiveStatuses {
		for _, status := range statuses {
			assert.Contains(t, SystemStatusesFor(resourceType), status, resourceType)
		}
	}
}
