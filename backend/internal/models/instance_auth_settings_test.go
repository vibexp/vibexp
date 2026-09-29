package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// An allowlist with nothing in it admits everyone, as config.yaml's always
// has: an empty stored row must never read as "nobody may sign in".
func TestInstanceAuthAllowlist_IsOpenAccess(t *testing.T) {
	tests := []struct {
		name      string
		allowlist *InstanceAuthAllowlist
		open      bool
	}{
		{"none stored", nil, true},
		{"a row with both lists nil", &InstanceAuthAllowlist{}, true},
		{"a row with both lists empty", &InstanceAuthAllowlist{Domains: []string{}, Emails: []string{}}, true},
		{"a domain", &InstanceAuthAllowlist{Domains: []string{"example.com"}}, false},
		{"an email", &InstanceAuthAllowlist{Emails: []string{"a@example.com"}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.open, tc.allowlist.IsOpenAccess())
		})
	}
}

func TestInstanceAuthProviderType_Valid(t *testing.T) {
	for _, typ := range []InstanceAuthProviderType{
		InstanceAuthProviderGoogle, InstanceAuthProviderGitHub, InstanceAuthProviderOIDC,
	} {
		assert.True(t, typ.Valid(), typ)
	}
	for _, typ := range []InstanceAuthProviderType{"", "saml", "Google"} {
		assert.False(t, typ.Valid(), typ)
	}
}

func TestInstanceAuthProvider_HasClientSecret(t *testing.T) {
	empty, set := "", "ciphertext"
	assert.False(t, (&InstanceAuthProvider{}).HasClientSecret())
	assert.False(t, (&InstanceAuthProvider{ClientSecretEncrypted: &empty}).HasClientSecret())
	assert.True(t, (&InstanceAuthProvider{ClientSecretEncrypted: &set}).HasClientSecret())
}
