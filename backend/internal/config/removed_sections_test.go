package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #483 deleted the instance-wide `github:` section: GitHub App credentials are
// now per team, stored encrypted in the database.
//
// These tests exist because an unknown key in config.yaml is otherwise SILENT.
// config.schema.json is `additionalProperties: false`, but nothing consults it
// at runtime — koanf/mapstructure simply ignore a key with no matching field.
// Without the pre-flight check an operator upgrading past this change would keep
// a fully-populated `github:` block that does nothing and believe the
// integration is configured.

// TestLoad_WithoutGitHubSection is the happy path after the removal: the
// section's absence is not merely tolerated, it is now the only valid shape.
func TestLoad_WithoutGitHubSection(t *testing.T) {
	cfg, err := loadYAML(t, baseValidYAML)

	require.NoError(t, err)
	require.NotNil(t, cfg)
}

// TestLoad_RemovedGitHubSection_FailsFast pins the acceptance criterion that the
// failure names the offending section. An operator hitting this at boot must not
// have to guess which key to delete, so the message is asserted piece by piece
// rather than just "an error occurred".
func TestLoad_RemovedGitHubSection_FailsFast(t *testing.T) {
	cfg, err := loadYAML(t, baseValidYAML+`
github:
  app_id: "123456"
  app_slug: vibexp-app
`)

	require.Error(t, err)
	assert.Nil(t, cfg, "a config carrying a removed section must not load at all")

	msg := err.Error()
	assert.Contains(t, msg, `"github"`, "the error must name the offending section")
	assert.Contains(t, msg, "per team", "the error must say where the setting went")
	assert.Contains(t, msg, "auth.github",
		"the error must disambiguate from the web-login client an operator also has")
}

// TestLoad_RemovedSectionCheck_IgnoresAuthGitHub is the regression guard named in
// #483 as the single easiest way to break the issue: `auth.github` is the
// web-login OAuth client — a different credential set on a different code path
// (internal/auth/idp/github) — and it must keep loading untouched. The check
// matches TOP-LEVEL keys only, which is what makes that true.
func TestLoad_RemovedSectionCheck_IgnoresAuthGitHub(t *testing.T) {
	cfg, err := loadYAML(t, baseValidYAML+`
auth:
  providers: ["github"]
  github:
    client_id: gh-web-login-id
    client_secret: gh-web-login-secret
    redirect_uri: https://app.example.com/cb/github
`)

	require.NoError(t, err, "auth.github is not the removed section and must still load")
	require.NotNil(t, cfg)
	assert.Equal(t, "gh-web-login-id", cfg.Auth.GitHub.ClientID)
	assert.Equal(t, "gh-web-login-secret", cfg.Auth.GitHub.ClientSecret)
	assert.Equal(t, "https://app.example.com/cb/github", cfg.Auth.GitHub.RedirectURI)
	assert.Equal(t, []string{"github"}, cfg.Auth.Providers)
}

// TestCheckRemovedSections covers the helper directly, including that an
// unrelated top-level key is none of its business — this check is a targeted
// migration aid, not a strict-unknown-key mode.
func TestCheckRemovedSections(t *testing.T) {
	tests := []struct {
		name      string
		parsed    map[string]interface{}
		wantError bool
	}{
		{"empty config", map[string]interface{}{}, false},
		{"unrelated sections", map[string]interface{}{"server": map[string]interface{}{}}, false},
		{"removed section present", map[string]interface{}{"github": map[string]interface{}{}}, true},
		{"removed section present but empty", map[string]interface{}{"github": nil}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkRemovedSections("config.yaml", tt.parsed)

			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "config.yaml", "the error must name the file")
				return
			}
			assert.NoError(t, err)
		})
	}
}
