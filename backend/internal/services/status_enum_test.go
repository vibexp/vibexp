package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// TestResourceSchemasDeclareNoInlineStatusEnum is the other half of the gate:
// the subsets are only a single source of truth for as long as nothing restates
// one. Eleven inline `enum:` blocks are what this issue removed, and re-adding
// one is a two-line edit that every other check in the suite would wave through
// -- the spec would still be valid, the bundle would still build, and
// ComponentEnum would still read the (now unused) component happily.
//
// It reads the authored YAML as text rather than the parsed model on purpose:
// libopenapi resolves a $ref transparently, so by the time a schema is a
// *base.Schema an inline enum and a $ref'd one are indistinguishable, which is
// exactly the difference being asserted.
func TestResourceSchemasDeclareNoInlineStatusEnum(t *testing.T) {
	root := repoBackendDir(t)

	for _, file := range []string{"prompts.yaml", "artifacts.yaml", "blueprints.yaml", "memories.yaml"} {
		t.Run(file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, "schemas", file))
			require.NoError(t, err)

			lines := strings.Split(string(raw), "\n")
			var offenders []string
			for i, line := range lines {
				if strings.TrimSpace(line) != "status:" {
					continue
				}
				indent := len(line) - len(strings.TrimLeft(line, " "))
				// Scan the property body: everything indented deeper than `status:`.
				for j := i + 1; j < len(lines); j++ {
					body := lines[j]
					if strings.TrimSpace(body) == "" {
						continue
					}
					if len(body)-len(strings.TrimLeft(body, " ")) <= indent {
						break
					}
					if strings.HasPrefix(strings.TrimSpace(body), "enum:") {
						offenders = append(offenders, fmt.Sprintf("%s:%d", file, j+1))
					}
				}
			}

			assert.Empty(t, offenders,
				"a `status` property declares an inline enum. Every one of them must be "+
					"a $ref to its subset in common.yaml, or the vocabulary has a second "+
					"source again and nothing compares the two (#912)")
		})
	}

	// Prove the scan can actually see an enum, so an empty result means "none"
	// rather than "the walk never matched anything".
	raw, err := os.ReadFile(filepath.Join(root, "schemas", "common.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "enum: [active, draft, archived]",
		"the subsets themselves must still declare their enums in common.yaml")
}

// repoBackendDir walks up to the directory holding openapi.yaml, the same way
// specconformance locates the authored spec.
func repoBackendDir(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for range 6 {
		if _, statErr := os.Stat(filepath.Join(dir, "openapi.yaml")); statErr == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate backend/openapi.yaml from the test working directory")
	return ""
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
			assert.Contains(t, err.Error(), "invalid status: must be one of:",
				"the message must name the accepted values, built from the allowlist")
		})
	}
}
