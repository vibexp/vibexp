package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/specconformance"
)

// statusSubsetExtension is the specification extension a per-resource-type
// status enum uses to declare which vocabulary it is drawn from (#912).
const statusSubsetExtension = "x-subset-of"

// TestResourceStatusSubsetsAreDrawnFromVocabulary proves the "one vocabulary,
// per-type subsets" claim in schemas/common.yaml is true of the spec itself.
//
// Every schema carrying `x-subset-of: <Vocabulary>` must be a non-empty enum
// whose every value the vocabulary also declares. Without this, "subset" is a
// comment: nothing stops someone adding `retired` to BlueprintStatus alone, and
// the generated clients would then expose a value the shared ResourceStatus
// union has no member for, on a green build.
//
// The subsets are DISCOVERED from the extension rather than listed here on
// purpose. A hand-written list has exactly one failure mode and it is silent:
// the next subset added to common.yaml would not be checked, and nothing would
// say so.
func TestResourceStatusSubsetsAreDrawnFromVocabulary(t *testing.T) {
	subsets, err := specconformance.ComponentsWithExtension(statusSubsetExtension)
	require.NoError(t, err, "read the %s declarations from the spec", statusSubsetExtension)
	require.NotEmpty(t, subsets,
		"no component schema declares %s -- either the status subsets were removed "+
			"from schemas/common.yaml or they were no longer registered under "+
			"openapi.yaml's components.schemas, and this gate is now vacuous",
		statusSubsetExtension)

	vocabularies := map[string][]string{}

	for subset, vocabulary := range subsets {
		t.Run(subset, func(t *testing.T) {
			if _, seen := vocabularies[vocabulary]; !seen {
				values, vocabErr := specconformance.ComponentEnum(vocabulary)
				require.NoError(t, vocabErr,
					"%s declares %s: %s, but %s is not an enum component schema",
					subset, statusSubsetExtension, vocabulary, vocabulary)
				vocabularies[vocabulary] = values
			}

			values, subsetErr := specconformance.ComponentEnum(subset)
			require.NoError(t, subsetErr, "read the %s enum from the spec", subset)

			assert.Subset(t, vocabularies[vocabulary], values,
				"%s declares values %s does not. A subset schema exists so every "+
					"resource type draws its status from ONE vocabulary; adding a "+
					"value to a subset alone breaks that and ships a value the "+
					"shared %s union in both generated clients has no member for (#912).",
				subset, vocabulary, vocabulary)
		})
	}
}

// TestResourceStatusesIsTheUnionOfTheSubsets pins the Go side of the same
// claim. models.ResourceStatuses mirrors the ResourceStatus schema (that
// pairing is asserted by TestSpecEnumsMatchServiceAllowlists), so a value that
// belongs to no subset would document a status no resource type can ever hold.
func TestResourceStatusesIsTheUnionOfTheSubsets(t *testing.T) {
	union := map[string]struct{}{}
	for _, subset := range [][]string{
		models.PromptStatuses,
		models.ArtifactStatuses,
		models.BlueprintStatuses,
		models.MemoryStatuses,
	} {
		require.NotEmpty(t, subset, "an empty subset would make this assertion vacuous")
		for _, status := range subset {
			union[status] = struct{}{}
		}
	}

	members := make([]string, 0, len(union))
	for status := range union {
		members = append(members, status)
	}

	assert.ElementsMatch(t, members, models.ResourceStatuses,
		"models.ResourceStatuses must stay exactly the union of the four per-type "+
			"subsets: a value in no subset documents a status nothing can hold, and a "+
			"subset value missing from it makes the vocabulary incomplete (#912)")
}

// TestValidateStatusRejectsOutOfSubsetValues covers the shared service-layer
// check every create/update path funnels through.
func TestValidateStatusRejectsOutOfSubsetValues(t *testing.T) {
	cases := []struct {
		name    string
		allowed []string
		status  string
		wantErr bool
	}{
		{name: "prompt accepts draft", allowed: models.PromptStatuses, status: "draft"},
		{name: "prompt accepts published", allowed: models.PromptStatuses, status: "published"},
		{name: "prompt rejects active", allowed: models.PromptStatuses, status: "active", wantErr: true},
		{name: "artifact accepts archived", allowed: models.ArtifactStatuses, status: "archived"},
		{name: "artifact rejects published", allowed: models.ArtifactStatuses, status: "published", wantErr: true},
		{name: "blueprint accepts expired", allowed: models.BlueprintStatuses, status: "expired"},
		{name: "blueprint rejects draft", allowed: models.BlueprintStatuses, status: "draft", wantErr: true},
		{name: "memory accepts draft", allowed: models.MemoryStatuses, status: "draft"},
		{name: "memory rejects expired", allowed: models.MemoryStatuses, status: "expired", wantErr: true},
		// "" means "not supplied" on every request model; each caller defaults it.
		{name: "empty is not supplied", allowed: models.PromptStatuses, status: ""},
		{name: "unknown value", allowed: models.MemoryStatuses, status: "retired", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateStatus(tc.allowed, tc.status)
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidStatus,
				"the handlers map ErrInvalidStatus to 400; any other error falls through to a 500")
			assert.Contains(t, err.Error(), "status must be one of:",
				"the message must name the accepted values, built from the allowlist")
		})
	}
}
