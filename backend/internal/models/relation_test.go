package models

import "testing"

// TestInitialRelationStatus pins the tiered-trust rule for every
// origin × relation_type combination: a human proposal is always confirmed; an
// AI proposal is confirmed only for built-from / explained-by and stays
// suggested for the higher-stakes governed-by / supersedes.
func TestInitialRelationStatus(t *testing.T) {
	cases := []struct {
		origin       string
		relationType string
		want         string
	}{
		{RelationOriginHuman, RelationTypeGovernedBy, RelationStatusConfirmed},
		{RelationOriginHuman, RelationTypeSupersedes, RelationStatusConfirmed},
		{RelationOriginHuman, RelationTypeBuiltFrom, RelationStatusConfirmed},
		{RelationOriginHuman, RelationTypeExplainedBy, RelationStatusConfirmed},
		{RelationOriginAI, RelationTypeGovernedBy, RelationStatusSuggested},
		{RelationOriginAI, RelationTypeSupersedes, RelationStatusSuggested},
		{RelationOriginAI, RelationTypeBuiltFrom, RelationStatusConfirmed},
		{RelationOriginAI, RelationTypeExplainedBy, RelationStatusConfirmed},
	}
	for _, tc := range cases {
		t.Run(tc.origin+"/"+tc.relationType, func(t *testing.T) {
			if got := InitialRelationStatus(tc.origin, tc.relationType); got != tc.want {
				t.Fatalf("InitialRelationStatus(%q, %q) = %q, want %q", tc.origin, tc.relationType, got, tc.want)
			}
		})
	}
}

// TestRequiredObjectType pins the object-type matrix: governed-by -> blueprint,
// built-from -> prompt, explained-by -> memory, and supersedes -> the subject's
// own type.
func TestRequiredObjectType(t *testing.T) {
	cases := []struct {
		relationType string
		fromType     string
		wantObj      string
		wantOK       bool
	}{
		{RelationTypeGovernedBy, RelationResourceTypeArtifact, RelationResourceTypeBlueprint, true},
		{RelationTypeGovernedBy, RelationResourceTypeMemory, RelationResourceTypeBlueprint, true},
		{RelationTypeBuiltFrom, RelationResourceTypeArtifact, RelationResourceTypePrompt, true},
		{RelationTypeExplainedBy, RelationResourceTypeArtifact, RelationResourceTypeMemory, true},
		{RelationTypeSupersedes, RelationResourceTypePrompt, RelationResourceTypePrompt, true},
		{RelationTypeSupersedes, RelationResourceTypeArtifact, RelationResourceTypeArtifact, true},
		{"unknown", RelationResourceTypeArtifact, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.relationType+"/"+tc.fromType, func(t *testing.T) {
			obj, ok := RequiredObjectType(tc.relationType, tc.fromType)
			if ok != tc.wantOK || obj != tc.wantObj {
				t.Fatalf("RequiredObjectType(%q, %q) = (%q, %v), want (%q, %v)",
					tc.relationType, tc.fromType, obj, ok, tc.wantObj, tc.wantOK)
			}
		})
	}
}

func TestRelationValidators(t *testing.T) {
	if !IsValidRelationResourceType(RelationResourceTypePrompt) {
		t.Error("prompt should be a valid resource type")
	}
	if IsValidRelationResourceType("project") {
		t.Error("project should not be a valid resource type")
	}
	if !IsValidRelationType(RelationTypeSupersedes) {
		t.Error("supersedes should be a valid relation type")
	}
	if IsValidRelationType("relates-to") {
		t.Error("relates-to should not be a valid relation type")
	}
	if !IsValidRelationOrigin(RelationOriginAI) || !IsValidRelationOrigin(RelationOriginHuman) {
		t.Error("ai and human should be valid origins")
	}
	if IsValidRelationOrigin("system") {
		t.Error("system should not be a valid origin")
	}
}
