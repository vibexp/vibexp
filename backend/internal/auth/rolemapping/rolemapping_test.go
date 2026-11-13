package rolemapping

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// claimsJSON decodes claims the way the OIDC client does, so the evaluator
// sees the same Go types (string, []any, map[string]any, float64).
func claimsJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var claims map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &claims))
	return claims
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name        string
		claims      string
		path        string
		adminValues []string
		wantMatched string
		wantOK      bool
	}{
		{"string claim", `{"role":"admin"}`, "role", []string{"admin"}, "admin", true},
		{"array claim", `{"groups":["dev","admin"]}`, "groups", []string{"admin"}, "admin", true},
		{"nested path", `{"realm_access":{"roles":["offline","vibexp-admin"]}}`, "realm_access.roles",
			[]string{"vibexp-admin"}, "vibexp-admin", true},
		{"hyphenated value", `{"groups":["platform-admins"]}`, "groups", []string{"platform-admins"},
			"platform-admins", true},
		{"value with a space", `{"groups":["VibeXP Admins"]}`, "groups", []string{"VibeXP Admins"},
			"VibeXP Admins", true},
		{"first admin value in order wins", `{"groups":["b","a"]}`, "groups", []string{"a", "b"}, "a", true},
		{"case differs", `{"groups":["Admin"]}`, "groups", []string{"admin"}, "", false},
		{"hyphen prefix only", `{"groups":["platform"]}`, "groups", []string{"platform-admins"}, "", false},
		{"no matching value", `{"groups":["dev"]}`, "groups", []string{"admin"}, "", false},
		{"missing claim", `{"other":"admin"}`, "groups", []string{"admin"}, "", false},
		{"number", `{"groups":1}`, "groups", []string{"1"}, "", false},
		{"object", `{"groups":{"admin":true}}`, "groups", []string{"admin"}, "", false},
		{"mixed array", `{"groups":["admin",1]}`, "groups", []string{"admin"}, "", false},
		{"missing intermediate segment", `{"realm_access":{}}`, "realm_access.roles", []string{"admin"}, "", false},
		{"intermediate segment not an object", `{"realm_access":"x"}`, "realm_access.roles",
			[]string{"admin"}, "", false},
		{"empty path", `{"":"admin"}`, "", []string{"admin"}, "", false},
		{"no admin values", `{"groups":["admin"]}`, "groups", nil, "", false},
		{"empty array", `{"groups":[]}`, "groups", []string{"admin"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, ok := Evaluate(claimsJSON(t, tt.claims), tt.path, tt.adminValues)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantMatched, matched)
		})
	}
}

func TestEvaluate_NilClaims(t *testing.T) {
	matched, ok := Evaluate(nil, "groups", []string{"admin"})
	assert.False(t, ok)
	assert.Empty(t, matched)
}
