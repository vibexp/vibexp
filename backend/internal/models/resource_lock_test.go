package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestIsLockableResourceType(t *testing.T) {
	cases := map[string]bool{
		RelationResourceTypePrompt:    true,
		RelationResourceTypeBlueprint: true,
		RelationResourceTypeArtifact:  true,
		RelationResourceTypeMemory:    false,
		"prompts":                     false,
		"":                            false,
	}
	for resourceType, want := range cases {
		if got := IsLockableResourceType(resourceType); got != want {
			t.Errorf("IsLockableResourceType(%q) = %v, want %v", resourceType, got, want)
		}
	}
}

func TestIsValidLockAction(t *testing.T) {
	cases := map[string]bool{
		LockActionLock:   true,
		LockActionUnlock: true,
		"relock":         false,
		"":               false,
	}
	for action, want := range cases {
		if got := IsValidLockAction(action); got != want {
			t.Errorf("IsValidLockAction(%q) = %v, want %v", action, got, want)
		}
	}
}

// A lock marshals in the shape the spec's ResourceLock schema publishes: the
// locker is a nested object, and the flat model fields never reach the wire.
func TestResourceLock_MarshalsInTheSpecShape(t *testing.T) {
	lock := &ResourceLock{
		LockedBy:     "user-1",
		LockedByName: "Ada Lovelace",
		Comment:      "in use",
		LockedAt:     time.Date(2026, 10, 11, 10, 15, 30, 0, time.UTC),
	}

	raw, err := json.Marshal(lock)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	want := `{"locked_by":{"user_id":"user-1","display_name":"Ada Lovelace"},` +
		`"comment":"in use","locked_at":"2026-10-11T10:15:30Z"}`
	if string(raw) != want {
		t.Errorf("lock marshaled as\n %s\nwant\n %s", raw, want)
	}
}

// `lock` is required+nullable in the spec, so the three resources emit the key
// even when unlocked -- as null -- and the lock object when locked. The write
// handlers still marshal these models directly, so this is what they answer.
func TestLockableResources_AlwaysEmitTheLockKey(t *testing.T) {
	held := &ResourceLock{LockedBy: "user-1", LockedByName: "Ada Lovelace", Comment: "in use"}
	cases := map[string]struct{ unlocked, locked any }{
		"prompt":    {unlocked: Prompt{}, locked: Prompt{Lock: held}},
		"blueprint": {unlocked: Blueprint{}, locked: Blueprint{Lock: held}},
		"artifact":  {unlocked: Artifact{}, locked: Artifact{Lock: held}},
	}
	for name, c := range cases {
		var unlocked, locked map[string]json.RawMessage
		mustRoundTrip(t, c.unlocked, &unlocked)
		mustRoundTrip(t, c.locked, &locked)

		if got, ok := unlocked["lock"]; !ok || string(got) != "null" {
			t.Errorf("%s: unlocked lock = %s (present %v), want null", name, got, ok)
		}
		var lock struct {
			LockedBy struct {
				UserID string `json:"user_id"`
			} `json:"locked_by"`
		}
		if err := json.Unmarshal(locked["lock"], &lock); err != nil || lock.LockedBy.UserID != "user-1" {
			t.Errorf("%s: locked lock = %s (err %v), want the locker nested under locked_by", name, locked["lock"], err)
		}
	}
}

func mustRoundTrip(t *testing.T, value any, into *map[string]json.RawMessage) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}
