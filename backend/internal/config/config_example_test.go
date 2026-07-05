package config

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// exampleConfigPath / configSchemaPath point at the committed operator-facing
// artifacts, relative to this package directory (backend/internal/config).
const (
	exampleConfigPath = "../../config.example.yaml"
	configSchemaPath  = "../../config.schema.json"
)

// setExampleSecretEnv sets the ${VAR} secrets that config.example.yaml
// references, so loading it interpolates cleanly (ENCRYPTION_KEY must be a valid
// 32-byte key; the rest may be empty for the test).
func setExampleSecretEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", "change_me_to_a_32_byte_secret_ok")
	for _, k := range []string{
		"DB_PASSWORD", "API_KEY_COMMON", "BACKOFFICE_ADMIN_API_KEY",
		"SESSION_ENCRYPTION_KEY", "GOOGLE_CLIENT_SECRET", "GITHUB_CLIENT_SECRET",
		"OIDC_CLIENT_SECRET", "GITHUB_APP_PRIVATE_KEY", "GITHUB_WEBHOOK_SECRET",
		"SMTP_PASSWORD", "MAILGUN_SENDING_KEY", "POSTMARK_SERVER_TOKEN",
		"SENDGRID_API_KEY",
	} {
		t.Setenv(k, "")
	}
}

// TestConfigExampleYAML_Loads is the smoke test behind the acceptance criterion
// "a fresh clone boots": the shipped config.example.yaml must load, interpolate,
// and validate through the real loader, and its values must match the code
// defaults (so the documented surface never drifts from defaults()).
func TestConfigExampleYAML_Loads(t *testing.T) {
	setExampleSecretEnv(t)

	cfg, err := Load(exampleConfigPath)
	require.NoError(t, err, "config.example.yaml must load and validate cleanly")

	// Spot-check parity with defaults() across several sections.
	require.Equal(t, "8080", cfg.Server.Port)
	require.Equal(t, int64(10<<20), cfg.Server.MaxBodySizeBytes)
	require.Equal(t, "vibexp_io", cfg.Database.Name)
	require.Equal(t, 15*time.Minute, cfg.Auth.OAuthAS.AccessTokenTTL)
	require.Equal(t, 720*time.Hour, cfg.Auth.OAuthAS.RefreshTokenTTL)
	require.Equal(t, 100, cfg.RateLimit.AuthPerMinute)
	require.Equal(t, 20, cfg.EventBus.WorkerCount)
	require.Equal(t, 0.1, cfg.OTel.TraceSampleRatio)

	// Interpolation actually resolved the required secret from the environment.
	require.Equal(t, "change_me_to_a_32_byte_secret_ok", cfg.Security.EncryptionKey)

	// The example targets localhost, so the dev safety-net auto-enables the
	// embedded OAuth AS and derives the MCP issuer/resource (zero-config local MCP).
	require.Equal(t, "http://localhost:8080", cfg.Auth.OAuthAS.IssuerURL)
	require.Equal(t, "http://localhost:8080/mcp/v1/common", cfg.MCP.ResourceURI)
}

// TestConfigExampleYAML_MatchesSchema validates the committed config.example.yaml
// against the committed config.schema.json. This is what an editor does via the
// `# yaml-language-server: $schema=` directive, so it guards the acceptance
// criterion "opening it gives schema validation" — additionalProperties:false
// makes it fail on a stray or misspelled key, and the property types must line up.
func TestConfigExampleYAML_MatchesSchema(t *testing.T) {
	schemaBytes, err := os.ReadFile(configSchemaPath)
	require.NoError(t, err)
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
	require.NoError(t, err)

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("config.schema.json", schemaDoc))
	schema, err := compiler.Compile("config.schema.json")
	require.NoError(t, err)

	// Parse the example as authored (no ${VAR} interpolation — an editor sees the
	// raw file), then normalize through JSON so numbers/maps match the types the
	// validator expects.
	exampleBytes, err := os.ReadFile(exampleConfigPath)
	require.NoError(t, err)
	var parsed any
	require.NoError(t, yaml.Unmarshal(exampleBytes, &parsed))
	normalized, err := json.Marshal(parsed)
	require.NoError(t, err)
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(normalized))
	require.NoError(t, err)

	require.NoError(t, schema.Validate(instance),
		"config.example.yaml must validate against config.schema.json")
}
