package config

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// dockerConfigPath points at the production-neutral config baked into the
// combined Docker image (Phase 3, issue #71), relative to this package dir.
const dockerConfigPath = "../../config.docker.yaml"

// setDockerRequiredEnv sets the two secrets config.docker.yaml references without
// a ${VAR:-default} fallback (ENCRYPTION_KEY must be a valid 32-byte key); every
// other reference defaults when its variable is unset.
func setDockerRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", "change_me_to_a_32_byte_secret_ok")
	t.Setenv("DB_PASSWORD", "local_password")
}

// TestConfigDockerYAML_LoadsWithSecretEnvOnly is the acceptance criterion
// "`docker run` with the documented secret env vars boots using the baked
// config.yaml": with only the required secrets set, the baked config loads,
// interpolates every ${VAR:-default}, and yields the documented defaults.
func TestConfigDockerYAML_LoadsWithSecretEnvOnly(t *testing.T) {
	setDockerRequiredEnv(t)

	cfg, err := Load(dockerConfigPath)
	require.NoError(t, err, "config.docker.yaml must load using secret env vars alone")

	// ${VAR:-default} fell back to the baked defaults.
	require.Equal(t, "8080", cfg.Server.Port)
	require.Equal(t, "localhost", cfg.Database.Host)
	require.Equal(t, "vibexp", cfg.Database.User)
	require.Equal(t, "vibexp", cfg.Database.Name)
	require.Equal(t, "gemini-embedding-001", cfg.Embedding.Model)

	// The required secret resolved from the environment.
	require.Equal(t, "change_me_to_a_32_byte_secret_ok", cfg.Security.EncryptionKey)

	// FRONTEND_BASE_URL defaults to EMPTY (fail-closed): a bare `docker run` that
	// forgets to set it is NOT treated as local development, so the dev-login
	// bypass is gated off and the embedded OAuth AS does not auto-enable on a
	// possibly-public surface. (docker-compose.yml sets FRONTEND_BASE_URL=localhost
	// explicitly for local evaluation — see the next test.)
	require.False(t, cfg.IsLocalDevelopment())
	require.Empty(t, cfg.Auth.OAuthAS.IssuerURL)
}

// TestConfigDockerYAML_LocalEvalEnablesDevLoginAndAS mirrors how docker-compose
// runs the published image for local evaluation: FRONTEND_BASE_URL points at
// localhost, which flips the deployment into local mode so the dev-login bypass
// is effective and the embedded OAuth AS auto-enables (zero-config local MCP).
func TestConfigDockerYAML_LocalEvalEnablesDevLoginAndAS(t *testing.T) {
	setDockerRequiredEnv(t)
	t.Setenv("FRONTEND_BASE_URL", "http://localhost:8080")

	cfg, err := Load(dockerConfigPath)
	require.NoError(t, err)

	require.True(t, cfg.IsLocalDevelopment())
	require.True(t, cfg.Auth.DevLoginEnabled)
	require.Equal(t, "http://localhost:8080", cfg.Auth.OAuthAS.IssuerURL)
	require.Equal(t, "http://localhost:8080/mcp/v1/common", cfg.MCP.ResourceURI)
}

// TestConfigDockerYAML_EnvOverridesAndProductionGate proves env injection alone
// reconfigures the container, and that a real (non-localhost) FRONTEND_BASE_URL
// flips the deployment out of local mode — the switch that turns dev login off.
func TestConfigDockerYAML_EnvOverridesAndProductionGate(t *testing.T) {
	setDockerRequiredEnv(t)
	t.Setenv("PORT", "9090")
	t.Setenv("DB_HOST", "postgres")
	t.Setenv("DB_NAME", "vibexp_prod")
	t.Setenv("FRONTEND_BASE_URL", "https://vibexp.example.com")
	// Production MCP auth: enabling the AS requires an explicit resource_uri
	// (RFC 8707 audience) — the dev auto-derivation only runs in local mode.
	t.Setenv("OAUTH_AS_ISSUER_URL", "https://vibexp.example.com")
	t.Setenv("MCP_RESOURCE_URI", "https://vibexp.example.com/mcp/v1/common")

	cfg, err := Load(dockerConfigPath)
	require.NoError(t, err)

	require.Equal(t, "9090", cfg.Server.Port)
	require.Equal(t, "postgres", cfg.Database.Host)
	require.Equal(t, "vibexp_prod", cfg.Database.Name)

	// Non-localhost base URL → not local: dev login is gated off regardless of the
	// baked dev_login_enabled:true, and the AS uses the explicit production issuer.
	require.False(t, cfg.IsLocalDevelopment())
	require.Equal(t, "https://vibexp.example.com", cfg.Auth.OAuthAS.IssuerURL)
	require.Equal(t, "https://vibexp.example.com/mcp/v1/common", cfg.MCP.ResourceURI)
}

// TestConfigDockerYAML_MatchesSchema validates the baked config against the
// committed config.schema.json (additionalProperties:false), so a stray or
// misspelled key in config.docker.yaml fails CI just as it would for the example.
func TestConfigDockerYAML_MatchesSchema(t *testing.T) {
	schemaBytes, err := os.ReadFile(configSchemaPath)
	require.NoError(t, err)
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
	require.NoError(t, err)

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("config.schema.json", schemaDoc))
	schema, err := compiler.Compile("config.schema.json")
	require.NoError(t, err)

	dockerBytes, err := os.ReadFile(dockerConfigPath)
	require.NoError(t, err)
	var parsed any
	require.NoError(t, yaml.Unmarshal(dockerBytes, &parsed))
	normalized, err := json.Marshal(parsed)
	require.NoError(t, err)
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(normalized))
	require.NoError(t, err)

	require.NoError(t, schema.Validate(instance),
		"config.docker.yaml must validate against config.schema.json")
}
