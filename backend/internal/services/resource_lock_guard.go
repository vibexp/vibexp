package services

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/vibexp/vibexp/internal/models"
)

// A lock (epic #1285) freezes a resource's identity and classification while
// leaving its content editable: a locked prompt, blueprint or artifact cannot
// be deleted, and an update may not change one of its frozen fields. The guards
// below are called by the helper each resource's writes funnel through, right
// after the authorization check, so a caller without the permission still gets
// the denial and learns nothing about the lock.
//
// The guard reads the lock off the row the caller already loaded and the write
// follows in a separate statement, so a lock taken in between is missed. That
// window is accepted, as it is for ResourceLockService.Unlock.

// frozenFieldCheck pairs a frozen field's JSON name with whether the update
// request changes it.
type frozenFieldCheck struct {
	field   string
	changed bool
}

// guardLockedDelete refuses to delete a locked resource, whatever the caller's
// role.
func guardLockedDelete(resourceType string, lock *models.ResourceLock) error {
	if lock == nil {
		return nil
	}
	return newResourceLockedError(resourceType, "", lock)
}

// guardLockedPromptUpdate refuses an update that changes a frozen field of a
// locked prompt. Name, description and body stay editable.
func guardLockedPromptUpdate(prompt *models.Prompt, req *models.UpdatePromptRequest) error {
	if prompt.Lock == nil {
		return nil
	}
	return firstFrozenChange(models.RelationResourceTypePrompt, prompt.Lock, []frozenFieldCheck{
		{"slug", stringChanged(req.Slug, prompt.Slug)},
		{"status", statusChanged(req.Status, prompt.Status)},
		{"project_id", stringChanged(req.ProjectID, prompt.ProjectID)},
		{"mcp_expose", req.MCPExpose != nil && *req.MCPExpose != prompt.MCPExpose},
		// The prompt update stores labels exactly as sent, so they are compared
		// as sent: normalizing first would let a differently spelt list through.
		{"labels", labelsChanged(req.Labels, prompt.Labels)},
	})
}

// guardLockedBlueprintUpdate refuses an update that changes a frozen field of a
// locked blueprint. It compares the REQUEST with the loaded row, before the
// update is applied, so what a content edit derives afterwards (a recomputed
// path, a skill's synced title, raw_content and its hash) is never mistaken for
// a requested change.
func guardLockedBlueprintUpdate(blueprint *models.Blueprint, req *models.UpdateBlueprintRequest) error {
	if blueprint.Lock == nil {
		return nil
	}
	return firstFrozenChange(models.RelationResourceTypeBlueprint, blueprint.Lock, []frozenFieldCheck{
		{"slug", stringChanged(req.Slug, blueprint.Slug)},
		{"status", statusChanged(req.Status, blueprint.Status)},
		{"type", stringChanged(req.Type, blueprint.Type)},
		{"subtype", stringChanged(req.Subtype, blueprintSubtype(blueprint))},
		{"project_id", stringChanged(req.ProjectID, blueprint.ProjectID)},
		{"labels", labelsChanged(normalizedLabelsOrNil(req.Labels), blueprint.Labels)},
		{"metadata", metadataChanged(req.Metadata, blueprint.Metadata)},
		// An empty path is "not sent", as applyUpdatePath reads it.
		{"path", req.Path != nil && *req.Path != "" && *req.Path != blueprint.Path},
	})
}

// guardLockedArtifactUpdate refuses an update that changes a frozen field of a
// locked artifact. Title, description and content stay editable.
func guardLockedArtifactUpdate(artifact *models.Artifact, req *models.UpdateArtifactRequest) error {
	if artifact.Lock == nil {
		return nil
	}
	return firstFrozenChange(models.RelationResourceTypeArtifact, artifact.Lock, []frozenFieldCheck{
		{"slug", stringChanged(req.Slug, artifact.Slug)},
		{"status", statusChanged(req.Status, artifact.Status)},
		{"type", stringChanged(req.Type, artifact.Type)},
		{"project_id", stringChanged(req.ProjectID, artifact.ProjectID)},
		{"labels", labelsChanged(normalizedLabelsOrNil(req.Labels), artifact.Labels)},
		{"metadata", metadataChanged(req.Metadata, artifact.Metadata)},
	})
}

// firstFrozenChange returns a ResourceLockedError naming the first check that
// reports a change, or nil when the request leaves every frozen field alone.
func firstFrozenChange(resourceType string, lock *models.ResourceLock, checks []frozenFieldCheck) error {
	for _, check := range checks {
		if check.changed {
			return newResourceLockedError(resourceType, check.field, lock)
		}
	}
	return nil
}

func newResourceLockedError(resourceType, field string, lock *models.ResourceLock) *ResourceLockedError {
	return &ResourceLockedError{
		ResourceType: resourceType,
		Field:        field,
		LockedBy:     lock.LockedBy,
		LockedByName: lock.LockedByName,
		Comment:      lock.Comment,
	}
}

// stringChanged reports whether sent carries a value other than current. A nil
// pointer means the field was not sent.
func stringChanged(sent *string, current string) bool {
	return sent != nil && *sent != current
}

// statusChanged is stringChanged for a status, where an empty value is
// "unchanged" too: the update paths ignore it (#912).
func statusChanged(sent *string, current string) bool {
	return sent != nil && *sent != "" && *sent != current
}

// normalizedLabelsOrNil normalizes a sent label list the way the blueprint and
// artifact updates store it, keeping nil ("not sent") apart from an empty list.
func normalizedLabelsOrNil(sent []string) []string {
	if sent == nil {
		return nil
	}
	return normalizeLabels(sent)
}

// labelsChanged reports whether sent holds a different set of labels than
// current. Order is not a change; a nil slice means the field was not sent.
func labelsChanged(sent, current []string) bool {
	if sent == nil {
		return false
	}
	a, b := slices.Clone(sent), slices.Clone(current)
	slices.Sort(a)
	slices.Sort(b)
	return !slices.Equal(a, b)
}

// metadataChanged reports whether sent differs from current by JSON value, not
// by Go type: a number decoded from a request body is a float64 and must equal
// the integer the stored document holds. A nil map means the field was not
// sent, and an empty map equals a missing one. Metadata that cannot be encoded
// counts as changed, so the guard fails closed.
func metadataChanged(sent, current map[string]interface{}) bool {
	if sent == nil || (len(sent) == 0 && len(current) == 0) {
		return false
	}
	sentJSON, err := json.Marshal(sent)
	if err != nil {
		return true
	}
	currentJSON, err := json.Marshal(current)
	if err != nil {
		return true
	}
	return !bytes.Equal(sentJSON, currentJSON)
}
