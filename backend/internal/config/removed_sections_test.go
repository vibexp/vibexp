package config

import (
	"encoding/json"
	"os"
	"strings"
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
	assert.Equal(t, "gh-web-login-id", cfg.Auth.LegacyGitHub.ClientID)
	assert.Equal(t, "gh-web-login-secret", cfg.Auth.LegacyGitHub.ClientSecret)
	assert.Equal(t, "https://app.example.com/cb/github", cfg.Auth.LegacyGitHub.RedirectURI)
	assert.Equal(t, []string{"github"}, cfg.Auth.LegacyProviders)
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
			warnings, err := checkRemovedSections("config.yaml", tt.parsed)

			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "config.yaml", "the error must name the file")
				return
			}
			assert.NoError(t, err)
			assert.Empty(t, warnings)
		})
	}
}

// TestCheckRemovedSections_ReportsAllDeterministically guards the two properties
// that make this check usable during a multi-section migration: every offending
// section is named in one go (no restart-per-section discovery loop), and the
// order is fixed whatever order the entries are declared in.
func TestCheckRemovedSections_ReportsAllDeterministically(t *testing.T) {
	original := removedConfigKeys
	t.Cleanup(func() { removedConfigKeys = original })
	removedConfigKeys = []removedConfigKey{
		{path: "zeta", mode: removalFatal, guidance: "zeta guidance"},
		{path: "alpha", mode: removalFatal, guidance: "alpha guidance"},
		{path: "mid", mode: removalFatal, guidance: "mid guidance"},
	}

	parsed := map[string]interface{}{
		"zeta":   map[string]interface{}{},
		"alpha":  map[string]interface{}{},
		"mid":    map[string]interface{}{},
		"server": map[string]interface{}{},
	}

	_, first := checkRemovedSections("config.yaml", parsed)
	require.Error(t, first)

	msg := first.Error()
	for _, section := range []string{"alpha", "mid", "zeta"} {
		assert.Contains(t, msg, `"`+section+`"`, "every offending section must be reported")
		assert.Contains(t, msg, section+" guidance", "each section brings its own guidance")
	}
	assert.Less(t, strings.Index(msg, "alpha"), strings.Index(msg, "mid"), "sections must be sorted")
	assert.Less(t, strings.Index(msg, "mid"), strings.Index(msg, "zeta"), "sections must be sorted")

	for i := 0; i < 20; i++ {
		_, again := checkRemovedSections("config.yaml", parsed)
		assert.Equal(t, msg, again.Error())
	}
}

// legacyAuthKeys are the nested auth keys deprecated by #1232 (warn mode).
var legacyAuthKeys = []string{
	"auth.access_allowlist", "auth.github", "auth.google", "auth.oidc", "auth.provider", "auth.providers",
}

// TestCheckRemovedSections_NestedWarnMode pins the warn-mode half of the check
// (#1232): each deprecated auth key warns only when POPULATED — the baked
// config.docker.yaml declares every one of them on every install, with empty
// values and a localhost redirect_uri — and a warning never fails the load.
func TestCheckRemovedSections_NestedWarnMode(t *testing.T) {
	auth := func(key string, value interface{}) map[string]interface{} {
		return map[string]interface{}{"auth": map[string]interface{}{key: value}}
	}
	provider := func(fields map[string]interface{}) map[string]interface{} { return fields }

	tests := []struct {
		name     string
		parsed   map[string]interface{}
		wantPath string // "" = no warning
	}{
		{"providers list", auth("providers", []interface{}{"google"}), "auth.providers"},
		{"providers comma string", auth("providers", "google,github"), "auth.providers"},
		{"providers empty list", auth("providers", []interface{}{}), ""},
		{"providers blank entries", auth("providers", []interface{}{"", "  "}), ""},
		{"provider set", auth("provider", "oidc"), "auth.provider"},
		{"provider blank", auth("provider", "  "), ""},
		{"provider null", auth("provider", nil), ""},
		{"allowlist domains", auth("access_allowlist",
			map[string]interface{}{"domains": "example.com", "emails": ""}), "auth.access_allowlist"},
		{"allowlist emails list", auth("access_allowlist",
			map[string]interface{}{"emails": []interface{}{"a@example.com"}}), "auth.access_allowlist"},
		{"allowlist empty", auth("access_allowlist",
			map[string]interface{}{"domains": "", "emails": ""}), ""},
		{"google client id", auth("google", provider(map[string]interface{}{"client_id": "id"})), "auth.google"},
		{"github secret", auth("github", provider(map[string]interface{}{"client_secret": "s"})), "auth.github"},
		{"oidc issuer", auth("oidc", provider(map[string]interface{}{"issuer_url": "https://sso"})), "auth.oidc"},
		{"redirect_uri alone is not populated", auth("google", provider(map[string]interface{}{
			"client_id": "", "client_secret": "", "redirect_uri": "http://localhost:8080/api/v1/auth/callback",
		})), ""},
		{"empty provider block", auth("oidc", map[string]interface{}{}), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings, err := checkRemovedSections("config.yaml", tt.parsed)
			require.NoError(t, err, "a deprecated auth key only warns")
			if tt.wantPath == "" {
				assert.Empty(t, warnings)
				return
			}
			require.Len(t, warnings, 1)
			assert.Contains(t, warnings[0], `"`+tt.wantPath+`"`, "the warning names the key")
			assert.Contains(t, warnings[0], "Admin → Settings → Authentication", "the warning says where it went")
			assert.Contains(t, warnings[0], "second minor release", "the warning says when it becomes fatal")
			assert.Contains(t, warnings[0], "https://vibexp.io/docs/", "the warning points at the docs")
		})
	}
}

// TestCheckRemovedSections_NestedAllKeys asserts every deprecated auth key is
// registered, reported once, and in sorted order.
func TestCheckRemovedSections_NestedAllKeys(t *testing.T) {
	parsed := map[string]interface{}{"auth": map[string]interface{}{
		"providers":        []interface{}{"google", "oidc"},
		"provider":         "github",
		"access_allowlist": map[string]interface{}{"domains": []interface{}{"example.com"}},
		"google":           map[string]interface{}{"client_id": "g"},
		"github":           map[string]interface{}{"client_id": "gh"},
		"oidc":             map[string]interface{}{"client_secret": "o"},
	}}

	warnings, err := checkRemovedSections("config.yaml", parsed)
	require.NoError(t, err)
	require.Len(t, warnings, len(legacyAuthKeys))
	for i, key := range legacyAuthKeys {
		assert.Contains(t, warnings[i], `"`+key+`"`, "warnings are sorted by key")
	}
}

// TestCheckRemovedSections_SiblingAuthKeysUnaffected guards the exact-path
// match: the auth keys that stay in config.yaml never warn, however populated.
func TestCheckRemovedSections_SiblingAuthKeysUnaffected(t *testing.T) {
	parsed := map[string]interface{}{"auth": map[string]interface{}{
		"instance_admins":        "admin@example.com",
		"session_encryption_key": "0123",
		"dev_login_enabled":      true,
		"oauth_as":               map[string]interface{}{"issuer_url": "https://as.example.com"},
		"api_oauth":              map[string]interface{}{"issuer": "https://idp.example.com"},
	}}

	warnings, err := checkRemovedSections("config.yaml", parsed)
	require.NoError(t, err)
	assert.Empty(t, warnings)
}

// TestCheckRemovedSections_TopLevelGitHubStaysFatal pins that the top-level
// `github` entry keeps failing boot next to warn-mode auth keys, and that the
// warn-mode `auth.github` entry does not make it fatal.
func TestCheckRemovedSections_TopLevelGitHubStaysFatal(t *testing.T) {
	_, err := checkRemovedSections("config.yaml", map[string]interface{}{
		"github": map[string]interface{}{"app_id": "1"},
		"auth":   map[string]interface{}{"github": map[string]interface{}{"client_id": "gh"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"github"`)
	assert.NotContains(t, err.Error(), `"auth.github"`, "auth.github only warns")

	warnings, err := checkRemovedSections("config.yaml", map[string]interface{}{
		"auth": map[string]interface{}{"github": map[string]interface{}{"client_id": "gh"}},
	})
	require.NoError(t, err, "auth.github alone must not fail boot")
	require.Len(t, warnings, 1)
}

// TestLoad_LegacyAuthKeys_DeprecationWarnings is the end-to-end half: Load
// carries the warnings on the Config (the server logs them) and still loads the
// legacy values for the bridge.
func TestLoad_LegacyAuthKeys_DeprecationWarnings(t *testing.T) {
	cfg, err := loadYAML(t, baseValidYAML+`
auth:
  provider: google
  google:
    client_id: gid
    client_secret: gsecret
`)
	require.NoError(t, err)
	require.Len(t, cfg.DeprecationWarnings, 2)
	assert.Contains(t, cfg.DeprecationWarnings[0], `"auth.google"`)
	assert.Contains(t, cfg.DeprecationWarnings[1], `"auth.provider"`)
	assert.Equal(t, "google", cfg.Auth.LegacyProvider)
	assert.Equal(t, "gid", cfg.Auth.LegacyGoogle.ClientID)

	plain, err := loadYAML(t, baseValidYAML)
	require.NoError(t, err)
	assert.Empty(t, plain.DeprecationWarnings)
}

// TestConfigSchema_LegacyAuthKeysDeprecated pins that the generated schema
// flags each deprecated auth key (and only those) so editors strike it
// through (#1232). The schema is generated and drift-gated; this asserts its
// content, not its freshness.
func TestConfigSchema_LegacyAuthKeysDeprecated(t *testing.T) {
	raw, err := os.ReadFile("../../config.schema.json")
	require.NoError(t, err)
	var schema struct {
		Defs map[string]struct {
			Properties map[string]struct {
				Deprecated  bool   `json:"deprecated"`
				Description string `json:"description"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(raw, &schema))

	auth := schema.Defs["AuthConfig"].Properties
	for _, key := range legacyAuthKeys {
		name := strings.TrimPrefix(key, "auth.")
		prop, ok := auth[name]
		require.True(t, ok, "AuthConfig has %s", name)
		assert.True(t, prop.Deprecated, "%s is deprecated", key)
		assert.Contains(t, prop.Description, "Admin → Settings → Authentication")
	}
	for _, name := range []string{"instance_admins", "session_encryption_key", "dev_login_enabled", "oauth_as", "api_oauth"} {
		assert.False(t, auth[name].Deprecated, "auth.%s stays in config.yaml", name)
	}
}
