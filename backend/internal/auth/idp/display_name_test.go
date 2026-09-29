package idp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultDisplayName(t *testing.T) {
	tests := []struct {
		name ProviderName
		want string
	}{
		{ProviderGoogle, "Google"},
		{ProviderGitHub, "GitHub"},
		{ProviderOIDC, "Single Sign-On"},
		{"keycloak", "Keycloak"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.name), func(t *testing.T) {
			assert.Equal(t, tt.want, DefaultDisplayName(tt.name))
		})
	}
}
