package config

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

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

	// The required secret resolved from the environment.
	require.Equal(t, "change_me_to_a_32_byte_secret_ok", cfg.Security.EncryptionKey)

	// A2A ${VAR:-default} knobs fall back to the baked defaults.
	require.Equal(t, 5*time.Minute, cfg.A2A.DefaultTimeout)
	require.Equal(t, 2*time.Hour, cfg.A2A.StreamTimeout)

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

// TestConfigDockerYAML_AccessAllowlistEnvSplitsToSlices proves the acceptance
// criterion that comma-separated AUTH_ALLOWED_DOMAINS / AUTH_ALLOWED_EMAILS env
// values unmarshal into the []string allowlist fields (koanf's
// StringToSliceHookFunc(",")), while unset vars leave both lists empty (open
// access, the fail-open default).
func TestConfigDockerYAML_AccessAllowlistEnvSplitsToSlices(t *testing.T) {
	setDockerRequiredEnv(t)
	t.Setenv("AUTH_ALLOWED_DOMAINS", "example.com,corp.io")
	t.Setenv("AUTH_ALLOWED_EMAILS", "alice@example.com,bob@other.com")

	cfg, err := Load(dockerConfigPath)
	require.NoError(t, err)

	require.Equal(t, []string{"example.com", "corp.io"}, []string(cfg.Auth.AccessAllowlist.Domains))
	require.Equal(t, []string{"alice@example.com", "bob@other.com"}, []string(cfg.Auth.AccessAllowlist.Emails))
}

// TestConfigDockerYAML_AccessAllowlistDefaultsOpen verifies the fail-open
// default: with the allowlist env vars unset, both lists are empty so access is
// open (every user may sign in).
func TestConfigDockerYAML_AccessAllowlistDefaultsOpen(t *testing.T) {
	setDockerRequiredEnv(t)

	cfg, err := Load(dockerConfigPath)
	require.NoError(t, err)

	require.Empty(t, cfg.Auth.AccessAllowlist.Domains)
	require.Empty(t, cfg.Auth.AccessAllowlist.Emails)
}

// TestConfigDockerYAML_OutboundAllowedCIDRsEnv proves the #745 acceptance
// criterion for the combined image: a self-hoster reaches an embedding sidecar
// on a private subnet with `docker run -e OUTBOUND_ALLOWED_CIDRS=...` alone —
// no config file to author — and gets the strict default when it is unset.
func TestConfigDockerYAML_OutboundAllowedCIDRsEnv(t *testing.T) {
	t.Run("declared ranges reach the guard", func(t *testing.T) {
		setDockerRequiredEnv(t)
		t.Setenv("OUTBOUND_ALLOWED_CIDRS", "172.16.0.0/12,127.0.0.1/32")

		cfg, err := Load(dockerConfigPath)
		require.NoError(t, err)

		require.Equal(t, []string{"172.16.0.0/12", "127.0.0.1/32"},
			[]string(cfg.Security.OutboundAllowedCIDRs))
		nets := cfg.Security.ParsedOutboundAllowedCIDRs()
		require.Len(t, nets, 2)
		require.True(t, nets[0].Contains(net.ParseIP("172.18.0.5")),
			"a container on a Docker bridge network must be reachable")
	})

	t.Run("unset keeps the strict default", func(t *testing.T) {
		setDockerRequiredEnv(t)

		cfg, err := Load(dockerConfigPath)
		require.NoError(t, err)

		require.Empty(t, cfg.Security.OutboundAllowedCIDRs)
		require.Empty(t, cfg.Security.ParsedOutboundAllowedCIDRs())
	})

	t.Run("a metadata-reaching range aborts startup", func(t *testing.T) {
		setDockerRequiredEnv(t)
		t.Setenv("OUTBOUND_ALLOWED_CIDRS", "169.254.0.0/16")

		cfg, err := Load(dockerConfigPath)
		require.Error(t, err, "the image must not boot with the SSRF hole reopened")
		require.Nil(t, cfg)
	})
}

// TestConfigDockerYAML_InstanceAdminsEnvSplitsToSlice proves the comma-separated
// INSTANCE_ADMIN_EMAILS env value unmarshals into the []string instance-admins
// field (koanf's StringToSliceHookFunc(",")), matching the AccessAllowlist
// pattern.
func TestConfigDockerYAML_InstanceAdminsEnvSplitsToSlice(t *testing.T) {
	setDockerRequiredEnv(t)
	t.Setenv("INSTANCE_ADMIN_EMAILS", "alice@example.com,bob@other.com")

	cfg, err := Load(dockerConfigPath)
	require.NoError(t, err)

	require.Equal(t, []string{"alice@example.com", "bob@other.com"}, []string(cfg.Auth.InstanceAdmins))
	require.True(t, cfg.IsInstanceAdmin("BOB@other.com"))
}

// TestConfigDockerYAML_InstanceAdminsDefaultDormant verifies the dormant default:
// with INSTANCE_ADMIN_EMAILS unset the list is empty and no one is an admin.
func TestConfigDockerYAML_InstanceAdminsDefaultDormant(t *testing.T) {
	setDockerRequiredEnv(t)

	cfg, err := Load(dockerConfigPath)
	require.NoError(t, err)

	require.Empty(t, cfg.Auth.InstanceAdmins)
	require.False(t, cfg.IsInstanceAdmin("alice@example.com"))
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

// TestConfigDockerYAML_SchedulerDefaults is the acceptance criterion "with none
// of the SCHEDULER_* vars set, the baked defaults match defaults()": a bare
// `docker run` keeps the scheduler on with its documented cadence.
func TestConfigDockerYAML_SchedulerDefaults(t *testing.T) {
	setDockerRequiredEnv(t)

	cfg, err := Load(dockerConfigPath)
	require.NoError(t, err)

	require.Equal(t, EnvBool(true), cfg.Scheduler.Enabled)
	require.Equal(t, time.Minute, cfg.Scheduler.TickInterval)
	require.Equal(t, 10*time.Minute, cfg.Scheduler.JobTimeout)
	require.Equal(t, EnvInt(100), cfg.Scheduler.DueLimit)
}

// TestConfigDockerYAML_SchedulerEnvOverrides is the headline acceptance
// criterion: every scheduler knob is settable with `docker run -e` alone, no
// mounted config file. The bool and int cases are the ones that needed
// EnvBool/EnvInt — they are authored as ${VAR:-default} strings in the raw YAML
// and must still arrive as typed Go values.
func TestConfigDockerYAML_SchedulerEnvOverrides(t *testing.T) {
	setDockerRequiredEnv(t)
	t.Setenv("SCHEDULER_ENABLED", "false")
	t.Setenv("SCHEDULER_TICK_INTERVAL", "5m")
	t.Setenv("SCHEDULER_JOB_TIMEOUT", "30m")
	t.Setenv("SCHEDULER_DUE_LIMIT", "50")

	cfg, err := Load(dockerConfigPath)
	require.NoError(t, err)

	require.Equal(t, EnvBool(false), cfg.Scheduler.Enabled,
		"SCHEDULER_ENABLED=false must turn the loop off with no mounted config file")
	require.Equal(t, 5*time.Minute, cfg.Scheduler.TickInterval)
	require.Equal(t, 30*time.Minute, cfg.Scheduler.JobTimeout)
	require.Equal(t, EnvInt(50), cfg.Scheduler.DueLimit)
}

// TestConfigDockerYAML_SchedulerInvalidEnvFailsFast pins the failure mode of the
// weak decoding EnvBool/EnvInt rely on: a value that is not a recognised bool is
// a load error, not a silently flipped knob.
func TestConfigDockerYAML_SchedulerInvalidEnvFailsFast(t *testing.T) {
	setDockerRequiredEnv(t)
	t.Setenv("SCHEDULER_ENABLED", "yes-please")

	cfg, err := Load(dockerConfigPath)

	require.Error(t, err, "an undecodable SCHEDULER_ENABLED must fail startup")
	require.Nil(t, cfg)
	// Name the field, so this cannot pass because Load failed for some unrelated
	// reason (a broken secret in setDockerRequiredEnv would do it).
	require.ErrorContains(t, err, "scheduler.enabled")
}

// TestConfigSchema_EnvPlaceholderTypesAreOptIn guards the decision that
// EnvBool/EnvInt loosen the schema for exactly the fields that opt in. A blanket
// mapper over every bool/int would make the schema accept a typo'd "tru" on any
// boolean knob, losing the editor validation config.schema.json exists to give.
func TestConfigSchema_EnvPlaceholderTypesAreOptIn(t *testing.T) {
	raw, err := os.ReadFile(configSchemaPath)
	require.NoError(t, err)

	var doc struct {
		Defs map[string]struct {
			Properties map[string]struct {
				Type  string            `json:"type"`
				OneOf []json.RawMessage `json:"oneOf"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))

	// Opted in: authored as ${SCHEDULER_*} placeholders in config.docker.yaml.
	require.Len(t, doc.Defs["SchedulerConfig"].Properties["enabled"].OneOf, 2,
		"scheduler.enabled is EnvBool, so its schema must also accept a ${VAR} placeholder")
	require.Len(t, doc.Defs["SchedulerConfig"].Properties["due_limit"].OneOf, 2,
		"scheduler.due_limit is EnvInt, so its schema must also accept a ${VAR} placeholder")

	// Not opted in: plain bools keep the strict schema.
	for _, tc := range []struct{ def, field string }{
		{"StorageConfig", "s3_path_style"},
		{"AuthConfig", "dev_login_enabled"},
	} {
		prop := doc.Defs[tc.def].Properties[tc.field]
		require.Equal(t, "boolean", prop.Type, "%s.%s must stay a strict boolean", tc.def, tc.field)
		require.Empty(t, prop.OneOf, "%s.%s must not accept a placeholder — it is a literal knob", tc.def, tc.field)
	}
}
