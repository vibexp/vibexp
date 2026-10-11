package models

import "testing"

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
