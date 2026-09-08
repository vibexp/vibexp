package services

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vibexp/vibexp/internal/models"
)

// ErrInvalidLabels is returned by a create/update when the caller's `labels`
// exceed the limits the OpenAPI schema documents (maxItems 10, item maxLength
// 50). Handlers map it to 400.
//
// Enforcement lives in the SERVICE, not in the handlers: the `validate:` struct
// tags on these request models are inert -- nothing calls validate.Struct on
// them (the artifact/blueprint/memory handlers hand-validate field by field) --
// and the generated request binder validates neither maxItems nor maxLength.
// Putting the check in the one place both the REST handlers and the MCP tools
// funnel through is what makes the documented limits actually true on every
// transport.
var ErrInvalidLabels = errors.New("invalid labels")

// validateLabels rejects a label list the spec's own limits would reject.
// It deliberately does NOT run over labels folded out of a legacy
// `metadata.tags` payload: those never passed any validation and are normalised
// (capped, truncated) instead, exactly as migration 016 does for the rows that
// already exist -- 400-ing an old client's edit because a pre-existing tag list
// is too long would break writes that work today.
func validateLabels(labels []string) error {
	if len(labels) > models.MaxLabels {
		return fmt.Errorf("%w: at most %d labels are allowed, got %d",
			ErrInvalidLabels, models.MaxLabels, len(labels))
	}
	for _, label := range labels {
		if len([]rune(label)) > models.MaxLabelLength {
			return fmt.Errorf("%w: each label may be at most %d characters, got %d",
				ErrInvalidLabels, models.MaxLabelLength, len([]rune(label)))
		}
	}
	return nil
}

// legacyMemoryTagsKey is the metadata key the SPA and older MCP clients used as
// a memory taxonomy before `labels` existed (issue #910).
const legacyMemoryTagsKey = "tags"

// normalizeLabels canonicalises a label list before it is persisted: whitespace
// is trimmed, empties are dropped, duplicates collapse to the first occurrence,
// each label is capped at models.MaxLabelLength and the list at models.MaxLabels.
//
// The request validator (`max=10,dive,max=50`) already rejects an over-long list
// coming straight off the wire, so for those inputs this is a no-op. It is
// load-bearing for values that never passed the validator: the labels folded out
// of a legacy `metadata.tags` payload (see foldLegacyMemoryTags), which could
// otherwise persist a row the API's own validation would reject on the next edit.
//
// It always returns a non-nil LabelList, so the `labels text[] NOT NULL` column
// and the required response array are both satisfied by construction.
func normalizeLabels(labels []string) models.LabelList {
	out := make(models.LabelList, 0, len(labels))
	seen := make(map[string]struct{}, len(labels))
	for _, raw := range labels {
		label := strings.TrimSpace(raw)
		if label == "" {
			continue
		}
		if runes := []rune(label); len(runes) > models.MaxLabelLength {
			label = string(runes[:models.MaxLabelLength])
		}
		if _, dup := seen[label]; dup {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
		if len(out) == models.MaxLabels {
			break
		}
	}
	return out
}

// foldLegacyMemoryTags moves a legacy `metadata.tags` array into the label list
// and removes the key, returning the merged labels, the metadata to persist, and
// whether the request carried a taxonomy at all (so a partial update can tell
// "no labels supplied" from "labels explicitly cleared").
//
// This is a WRITE-PATH shim, not a migration leftover. Migration 016 backfills
// the rows that exist today, but the SPA and any MCP client written before this
// change keep sending `metadata: {"tags": [...]}` on every edit, which would
// silently re-create the key the migration just removed — so the invariant
// "no `tags` key in memory metadata" only holds if the write path enforces it
// for old and new clients alike. Explicit `labels` take precedence in ordering;
// tags are appended and de-duplicated against them by normalizeLabels.
//
// A `tags` value that is not an array of strings is left exactly where it is:
// it is an ordinary metadata entry that happens to share the name, and eating it
// would be data loss.
func foldLegacyMemoryTags(
	labels []string, metadata map[string]interface{},
) (models.LabelList, map[string]interface{}, bool) {
	tags, ok := legacyTagValues(metadata[legacyMemoryTagsKey])
	if !ok {
		return normalizeLabels(labels), metadata, labels != nil
	}

	// Copy rather than mutate: the caller's request struct is shared with its
	// own tests and, on the MCP path, with the decoded tool arguments.
	trimmed := make(map[string]interface{}, len(metadata))
	for k, v := range metadata {
		if k == legacyMemoryTagsKey {
			continue
		}
		trimmed[k] = v
	}

	return normalizeLabels(append(append([]string{}, labels...), tags...)), trimmed, true
}

// legacyTagValues reports whether v is an array whose entries are all strings,
// returning them. JSON decoding yields []interface{}, so both shapes are handled.
func legacyTagValues(v interface{}) ([]string, bool) {
	switch tags := v.(type) {
	case []string:
		return tags, true
	case []interface{}:
		out := make([]string, 0, len(tags))
		for _, entry := range tags {
			s, ok := entry.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}
