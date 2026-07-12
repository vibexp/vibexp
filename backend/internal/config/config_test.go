package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note: tests that use t.Setenv must not be parallelised.

// validTestEncryptionKey is a 32-byte key so Load() passes encryption-key
// validation; tests exercising the key specifically override it.
const validTestEncryptionKey = "12345678901234567890123456789012"

// baseValidYAML is the minimal config.yaml that loads successfully: it supplies
// the only field without a default and validate hook (the encryption key); the
// rest come from defaults().
const baseValidYAML = `
security:
  encryption_key: "` + validTestEncryptionKey + `"
`

// parityYAML is a representative, fully-populated config.yaml for the parity
// test. Values deliberately differ from the code defaults so the test proves the
// file overrides them, and it exercises every section, both slice spellings
// (comma string and YAML list), and duration/int/float/bool coercion. It is an
// inline constant rather than a testdata/ fixture because the repo .gitignore
// excludes all testdata/ directories.
const parityYAML = `
server:
  port: "9090"
  log_level: debug
  log_format: text
  service_version: "1.2.3"
  release_sha: "abc1234"
  release_date: "2026-06-30"
  max_body_size_bytes: 2097152
  cors_allowed_origins: "https://a.example.com,https://b.example.com"
  error_type_base_uri: "https://errors.example.com"
database:
  host: db.example.com
  port: "6543"
  user: appuser
  password: "s3cret"
  name: appdb
security:
  encryption_key: "` + validTestEncryptionKey + `"
  api_key_common: "common-key"
  backoffice_admin_api_key: "admin-key"
auth:
  providers: "google,github"
  provider: google
  session_encryption_key: "sessionkey"
  dev_login_enabled: true
  signin_allowed_emails:
    - alice@example.com
    - bob@example.com
  google:
    client_id: g-id
    client_secret: g-secret
    redirect_uri: https://app.example.com/cb/google
  github:
    client_id: gh-id
    client_secret: gh-secret
    redirect_uri: https://app.example.com/cb/github
  oidc:
    issuer_url: https://oidc.example.com
    client_id: o-id
    client_secret: o-secret
    redirect_uri: https://app.example.com/cb/oidc
  oauth_as:
    issuer_url: https://connect.example.com
    access_token_ttl: 30m
    refresh_token_ttl: 1000h
    auth_code_ttl: 5m
    key_rotation_interval: 168h
    cleanup_interval: 2h
  api_oauth:
    issuer: https://api-oauth.example.com
    audiences: "aud1,aud2"
mcp:
  oauth_issuer: https://connect.example.com
  resource_uri: https://connect.example.com/mcp/v1/common
email:
  provider: mailgun
  from_address: noreply@example.com
  contact_recipient_address: support@example.com
  privacy_policy_url: https://example.com/privacy-policy
  smtp:
    host: smtp.example.com
    port: "2525"
    username: smtpuser
    password: smtppass
  mailgun:
    base_url: https://api.mailgun.net
    domain: mg.example.com
    sending_key: mg-key
  postmark:
    server_token: pm-token
    message_stream: broadcast
  sendgrid:
    api_key: sg-key
frontend:
  base_url: https://app.example.com
  site_name: VibeXP
  site_legal_name: VibeXP Inc
  site_url: https://app.example.com
  terms_url: https://example.com/terms
  privacy_url: https://example.com/privacy
  support_email: help@example.com
  brand_logo_url: https://example.com/logo.png
  mcp_endpoint: https://connect.example.com/mcp/v1/common
  error_type_base_uri: https://errors.example.com
  gtm_id: GTM-XXXX
  gtm_enabled: "true"
  ga4_measurement_id: G-XXXX
search:
  recency_ranking_enabled: true
  rank_weight_relevance: 0.6
  rank_weight_created: 0.25
  rank_weight_updated: 0.15
  rank_half_life_days: 45
  rank_candidate_cap: 300
github:
  app_id: "123456"
  app_slug: vibexp-app
  app_private_key: "PEMDATA"
  webhook_url: https://app.example.com/webhook
  webhook_secret: wh-secret
storage:
  attachments_bucket: my-bucket
gcp:
  project_id: my-project
  pubsub_push_audience: https://app.example.com
  pubsub_push_service_account_suffix: "@my-project.iam.gserviceaccount.com"
rate_limit:
  auth_per_minute: 50
  api_per_minute: 500
retention:
  activity_days: 30
  access_event_days: 60
  content_version_limit: 10
a2a:
  default_timeout: 10s
  stream_timeout: 3h
fcm:
  enabled: true
deployment:
  otel_environment: staging
  environment: stg
  env: s
  deployment_environment: staging-env
  kubernetes_service_host: 10.0.0.1
  google_cloud_project: gcp-proj
  gcp_project: gcp-proj2
  aws_region: us-east-1
  aws_default_region: us-west-2
  k_service: vibexp-svc
  k_revision: rev-1
event_bus:
  worker_count: 10
  buffer_size: 250
  max_retries: 5
  retry_backoff: 500ms
  retry_jitter: false
otel:
  endpoint: otel.example.com:4317
  export_interval: 30s
  trace_sample_ratio: 0.5
  tracing_enabled: true
`

// writeConfig writes body to a temp config.yaml and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// loadYAML writes body to a temp file and loads it.
func loadYAML(t *testing.T, body string) (*Config, error) {
	t.Helper()
	return Load(writeConfig(t, body))
}

func TestLoad_MissingFile_ReturnsError(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "does-not-exist.yaml", "error must name the expected path")
	assert.Contains(t, err.Error(), "config.example.yaml", "error must point at config.example.yaml")
}

// TestLoad_EmptyPath_UsesEnvFile verifies the path-resolution precedence: an empty
// path (no --config flag) falls back to VIBEXP_CONFIG_FILE.
func TestLoad_EmptyPath_UsesEnvFile(t *testing.T) {
	path := writeConfig(t, baseValidYAML+"server:\n  port: \"7777\"\n")
	t.Setenv("VIBEXP_CONFIG_FILE", path)

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, "7777", cfg.Server.Port, "Load(\"\") must read the file named by VIBEXP_CONFIG_FILE")
}

// TestLoad_EmptyPath_DefaultsToConfigYAML verifies that with no path and no
// VIBEXP_CONFIG_FILE, Load falls back to ./config.yaml (absent here → the
// required-file error naming that default path).
func TestLoad_EmptyPath_DefaultsToConfigYAML(t *testing.T) {
	require.NoError(t, os.Unsetenv("VIBEXP_CONFIG_FILE"))

	cfg, err := Load("")
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "config.yaml", "the default path must be ./config.yaml")
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := loadYAML(t, baseValidYAML)
	require.NoError(t, err)

	// Server / logging / build metadata defaults.
	assert.Equal(t, "8080", cfg.Server.Port)
	assert.Equal(t, "info", cfg.Server.LogLevel)
	assert.Equal(t, "json", cfg.Server.LogFormat)
	assert.Equal(t, int64(10<<20), cfg.Server.MaxBodySizeBytes)
	assert.Equal(t, "about:blank", cfg.Server.ErrorTypeBaseURI)

	// Database defaults.
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, "5432", cfg.Database.Port)
	assert.Equal(t, "postgres", cfg.Database.User)
	assert.Equal(t, "vibexp_io", cfg.Database.Name)

	// Email defaults.
	assert.Equal(t, "smtp", cfg.Email.Provider)
	assert.Equal(t, "smtp.gmail.com", cfg.Email.SMTP.Host)
	assert.Equal(t, "587", cfg.Email.SMTP.Port)
	assert.Equal(t, "outbound", cfg.Email.Postmark.MessageStream)

	// Search defaults.
	assert.False(t, cfg.Search.RecencyRankingEnabled)
	assert.InDelta(t, 0.5, cfg.Search.RankWeightRelevance, 1e-9)
	assert.InDelta(t, 0.3, cfg.Search.RankWeightCreated, 1e-9)
	assert.InDelta(t, 0.2, cfg.Search.RankWeightUpdated, 1e-9)
	assert.InDelta(t, 90, cfg.Search.RankHalfLifeDays, 1e-9)
	assert.Equal(t, 200, cfg.Search.RankCandidateCap)

	// Rate limits / retention defaults.
	assert.Equal(t, 100, cfg.RateLimit.AuthPerMinute)
	assert.Equal(t, 1000, cfg.RateLimit.APIPerMinute)
	assert.Equal(t, 90, cfg.Retention.ActivityDays)
	assert.Equal(t, 90, cfg.Retention.AccessEventDays)
	assert.Equal(t, 20, cfg.Retention.ContentVersionLimit)

	// Embedded sub-config defaults.
	assert.Equal(t, 20, cfg.EventBus.WorkerCount)
	assert.Equal(t, 200*time.Millisecond, cfg.EventBus.RetryBackoff)
	assert.True(t, cfg.EventBus.RetryJitter)
	assert.Equal(t, "localhost:4317", cfg.OTel.Endpoint)
	assert.Equal(t, 60*time.Second, cfg.OTel.ExportInterval)

	// A2A duration defaults.
	assert.Equal(t, 5*time.Minute, cfg.A2A.DefaultTimeout)
	assert.Equal(t, 2*time.Hour, cfg.A2A.StreamTimeout)

	// CORS is defaulted to the localhost dev origins when unset.
	assert.Equal(t, []string{"http://localhost:5173", "http://localhost:5174"}, cfg.Server.CORSAllowedOrigins)
}

// TestLoad_ParityFixture is the parity criterion: a fully-populated YAML file
// produces an identically-populated nested *Config (covering every section, both
// slice spellings, and duration/int/float/bool coercion).
func TestLoad_ParityFixture(t *testing.T) {
	cfg, err := loadYAML(t, parityYAML)
	require.NoError(t, err)

	// Server.
	assert.Equal(t, "9090", cfg.Server.Port)
	assert.Equal(t, "debug", cfg.Server.LogLevel)
	assert.Equal(t, "text", cfg.Server.LogFormat)
	assert.Equal(t, "1.2.3", cfg.Server.ServiceVersion)
	assert.Equal(t, int64(2097152), cfg.Server.MaxBodySizeBytes)
	assert.Equal(t, []string{"https://a.example.com", "https://b.example.com"}, cfg.Server.CORSAllowedOrigins)
	assert.Equal(t, "https://errors.example.com", cfg.Server.ErrorTypeBaseURI)

	// Database.
	assert.Equal(t, "db.example.com", cfg.Database.Host)
	assert.Equal(t, "6543", cfg.Database.Port)
	assert.Equal(t, "appuser", cfg.Database.User)
	assert.Equal(t, "s3cret", cfg.Database.Password)
	assert.Equal(t, "appdb", cfg.Database.Name)

	// Security.
	assert.Equal(t, validTestEncryptionKey, cfg.Security.EncryptionKey)
	assert.Equal(t, "common-key", cfg.Security.APIKeyCommon)
	assert.Equal(t, "admin-key", cfg.Security.BackofficeAdminAPIKey)

	// Auth + sub-structs. providers is a comma string; signin emails a YAML list.
	assert.Equal(t, []string{"google", "github"}, cfg.Auth.Providers)
	assert.Equal(t, "google", cfg.Auth.Provider)
	assert.Equal(t, "sessionkey", cfg.Auth.SessionEncryptionKey)
	assert.True(t, cfg.Auth.DevLoginEnabled)
	assert.Equal(t, []string{"alice@example.com", "bob@example.com"}, cfg.Auth.SignInAllowedEmails)
	assert.Equal(t, "g-id", cfg.Auth.Google.ClientID)
	assert.Equal(t, "g-secret", cfg.Auth.Google.ClientSecret)
	assert.Equal(t, "https://app.example.com/cb/google", cfg.Auth.Google.RedirectURI)
	assert.Equal(t, "gh-id", cfg.Auth.GitHub.ClientID)
	assert.Equal(t, "https://app.example.com/cb/github", cfg.Auth.GitHub.RedirectURI)
	assert.Equal(t, "https://oidc.example.com", cfg.Auth.OIDC.IssuerURL)
	assert.Equal(t, "o-secret", cfg.Auth.OIDC.ClientSecret)
	assert.Equal(t, "https://connect.example.com", cfg.Auth.OAuthAS.IssuerURL)
	assert.Equal(t, 30*time.Minute, cfg.Auth.OAuthAS.AccessTokenTTL)
	assert.Equal(t, 1000*time.Hour, cfg.Auth.OAuthAS.RefreshTokenTTL)
	assert.Equal(t, 5*time.Minute, cfg.Auth.OAuthAS.AuthCodeTTL)
	assert.Equal(t, 168*time.Hour, cfg.Auth.OAuthAS.KeyRotationInterval)
	assert.Equal(t, 2*time.Hour, cfg.Auth.OAuthAS.CleanupInterval)
	assert.Equal(t, "https://api-oauth.example.com", cfg.Auth.APIAuth.Issuer)
	assert.Equal(t, []string{"aud1", "aud2"}, cfg.Auth.APIAuth.Audiences)

	// MCP.
	assert.Equal(t, "https://connect.example.com", cfg.MCP.OAuthIssuer)
	assert.Equal(t, "https://connect.example.com/mcp/v1/common", cfg.MCP.ResourceURI)

	// Email + sub-structs.
	assert.Equal(t, "mailgun", cfg.Email.Provider)
	assert.Equal(t, "noreply@example.com", cfg.Email.FromAddress)
	assert.Equal(t, "support@example.com", cfg.Email.ContactRecipientAddress)
	assert.Equal(t, "https://example.com/privacy-policy", cfg.Email.PrivacyPolicyURL)
	assert.Equal(t, "smtp.example.com", cfg.Email.SMTP.Host)
	assert.Equal(t, "2525", cfg.Email.SMTP.Port)
	assert.Equal(t, "smtppass", cfg.Email.SMTP.Password)
	assert.Equal(t, "mg.example.com", cfg.Email.Mailgun.Domain)
	assert.Equal(t, "mg-key", cfg.Email.Mailgun.SendingKey)
	assert.Equal(t, "pm-token", cfg.Email.Postmark.ServerToken)
	assert.Equal(t, "broadcast", cfg.Email.Postmark.MessageStream)
	assert.Equal(t, "sg-key", cfg.Email.SendGrid.APIKey)

	// Frontend.
	assert.Equal(t, "https://app.example.com", cfg.Frontend.BaseURL)
	assert.Equal(t, "VibeXP", cfg.Frontend.SiteName)
	assert.Equal(t, "VibeXP Inc", cfg.Frontend.SiteLegalName)
	assert.Equal(t, "true", cfg.Frontend.GTMEnabled)
	assert.Equal(t, "G-XXXX", cfg.Frontend.GA4MeasurementID)

	// Search.
	assert.True(t, cfg.Search.RecencyRankingEnabled)
	assert.InDelta(t, 0.6, cfg.Search.RankWeightRelevance, 1e-9)
	assert.InDelta(t, 45, cfg.Search.RankHalfLifeDays, 1e-9)
	assert.Equal(t, 300, cfg.Search.RankCandidateCap)

	// GitHub app, storage, gcp.
	assert.Equal(t, "123456", cfg.GitHub.AppID)
	assert.Equal(t, "vibexp-app", cfg.GitHub.AppSlug)
	assert.Equal(t, "wh-secret", cfg.GitHub.WebhookSecret)
	assert.Equal(t, "my-bucket", cfg.Storage.AttachmentsBucket)
	assert.Equal(t, "my-project", cfg.GCP.ProjectID)
	assert.Equal(t, "@my-project.iam.gserviceaccount.com", cfg.GCP.PubSubPushServiceAccountSuffix)

	// Rate limit / retention / a2a / fcm.
	assert.Equal(t, 50, cfg.RateLimit.AuthPerMinute)
	assert.Equal(t, 500, cfg.RateLimit.APIPerMinute)
	assert.Equal(t, 30, cfg.Retention.ActivityDays)
	assert.Equal(t, 60, cfg.Retention.AccessEventDays)
	assert.Equal(t, 10, cfg.Retention.ContentVersionLimit)
	assert.Equal(t, 10*time.Second, cfg.A2A.DefaultTimeout)
	assert.Equal(t, 3*time.Hour, cfg.A2A.StreamTimeout)
	assert.True(t, cfg.FCM.Enabled)

	// Deployment.
	assert.Equal(t, "staging", cfg.Deployment.OTelEnvironment)
	assert.Equal(t, "us-east-1", cfg.Deployment.AWSRegion)
	assert.Equal(t, "vibexp-svc", cfg.Deployment.KService)

	// Embedded sub-configs.
	assert.Equal(t, 10, cfg.EventBus.WorkerCount)
	assert.Equal(t, 250, cfg.EventBus.BufferSize)
	assert.Equal(t, 5, cfg.EventBus.MaxRetries)
	assert.Equal(t, 500*time.Millisecond, cfg.EventBus.RetryBackoff)
	assert.False(t, cfg.EventBus.RetryJitter)
	assert.Equal(t, "otel.example.com:4317", cfg.OTel.Endpoint)
	assert.Equal(t, 30*time.Second, cfg.OTel.ExportInterval)
	assert.InDelta(t, 0.5, cfg.OTel.TraceSampleRatio, 1e-9)
	assert.True(t, cfg.OTel.TracingEnabled)

	// GetDeploymentEnvironment derives from the otel_environment field.
	assert.Equal(t, "staging", cfg.GetDeploymentEnvironment())
}

// --- Interpolation -------------------------------------------------------

func TestInterpolateString(t *testing.T) {
	t.Setenv("VX_TEST_SET", "resolved")
	t.Setenv("VX_TEST_EMPTY", "")
	require.NoError(t, os.Unsetenv("VX_TEST_UNSET"))

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "no vars here", "no vars here"},
		{"simple var", "${VX_TEST_SET}", "resolved"},
		{"var in context", "prefix-${VX_TEST_SET}-suffix", "prefix-resolved-suffix"},
		{"default used when unset", "${VX_TEST_UNSET:-fallback}", "fallback"},
		{"default used when empty", "${VX_TEST_EMPTY:-fallback}", "fallback"},
		{"set value wins over default", "${VX_TEST_SET:-fallback}", "resolved"},
		{"unset no default is empty", "x${VX_TEST_UNSET}y", "xy"},
		{"escaped literal", "$${VX_TEST_SET}", "${VX_TEST_SET}"},
		{"empty default", "${VX_TEST_UNSET:-}", ""},
		{"lone dollar", "cost is $5", "cost is $5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, interpolateString(tc.in))
		})
	}
}

func TestLoad_Interpolation_EndToEnd(t *testing.T) {
	t.Setenv("VX_DB_HOST", "db.from.env")
	t.Setenv("VX_CHUNK", "750")

	cfg, err := loadYAML(t, baseValidYAML+`
database:
  host: ${VX_DB_HOST}
  name: ${VX_DB_NAME:-defaultdb}
search:
  rank_candidate_cap: ${VX_CHUNK}
frontend:
  base_url: $${LITERAL}
`)
	require.NoError(t, err)
	assert.Equal(t, "db.from.env", cfg.Database.Host)
	assert.Equal(t, "defaultdb", cfg.Database.Name, "unset var falls back to :- default")
	assert.Equal(t, 750, cfg.Search.RankCandidateCap, "interpolated string coerces to int")
	assert.Equal(t, "${LITERAL}", cfg.Frontend.BaseURL, "$$ escapes to a literal ${...}")
}

func TestLoad_Interpolation_UnsetNoDefault_WarnsAndEmpties(t *testing.T) {
	require.NoError(t, os.Unsetenv("VX_DEFINITELY_UNSET"))

	// Capture the bootstrap logger so we can assert the warning fires.
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	cfg, err := loadYAML(t, baseValidYAML+`
email:
  from_address: ${VX_DEFINITELY_UNSET}
`)
	require.NoError(t, err)
	assert.Empty(t, cfg.Email.FromAddress, "an unset var with no default resolves to empty")
	assert.Contains(t, buf.String(), "VX_DEFINITELY_UNSET", "a warning must name the unresolved variable")
}

// TestLoad_TypeCoercion locks in decoding of every scalar kind plus both slice
// spellings, including values that arrive as strings via interpolation.
func TestLoad_TypeCoercion(t *testing.T) {
	t.Setenv("VX_JITTER", "false")

	cfg, err := loadYAML(t, baseValidYAML+`
server:
  max_body_size_bytes: 1048576
auth:
  oauth_as:
    issuer_url: https://connect.example.com
    access_token_ttl: 720h
    refresh_token_ttl: 2000h
  providers: "google,oidc"
mcp:
  resource_uri: https://connect.example.com/mcp/v1/common
search:
  rank_weight_relevance: 0.75
  recency_ranking_enabled: true
event_bus:
  retry_jitter: ${VX_JITTER}
  retry_backoff: 1s
`)
	require.NoError(t, err)
	assert.Equal(t, int64(1048576), cfg.Server.MaxBodySizeBytes)    // int64
	assert.Equal(t, 720*time.Hour, cfg.Auth.OAuthAS.AccessTokenTTL) // duration
	assert.InDelta(t, 0.75, cfg.Search.RankWeightRelevance, 1e-9)   // float
	assert.True(t, cfg.Search.RecencyRankingEnabled)                // bool (native)
	assert.False(t, cfg.EventBus.RetryJitter)                       // bool via interpolated string
	assert.Equal(t, time.Second, cfg.EventBus.RetryBackoff)         // duration
	assert.Equal(t, []string{"google", "oidc"}, cfg.Auth.Providers) // comma slice
}

// --- Validators ----------------------------------------------------------

func TestLoad_MissingEncryptionKey_ReturnsError(t *testing.T) {
	cfg, err := loadYAML(t, "server:\n  port: \"8080\"\n")

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "security.encryption_key")
}

func TestLoad_ShortEncryptionKey_ReturnsError(t *testing.T) {
	cfg, err := loadYAML(t, "security:\n  encryption_key: \"tooshort\"\n")

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "security.encryption_key")
}

func TestLoad_ActivityRetentionDays(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		wantErr bool
		want    int
	}{
		{"valid", "30", false, 30},
		{"at max boundary", "3650", false, 3650},
		{"zero", "0", true, 0},
		{"negative", "-1", true, 0},
		{"exceeds max", "3651", true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadYAML(t, baseValidYAML+"retention:\n  activity_days: "+tc.value+"\n")
			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, cfg)
				assert.Contains(t, err.Error(), "retention.activity_days")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.Retention.ActivityDays)
		})
	}
}

func TestLoad_AccessEventRetentionDays_Invalid_ReturnsError(t *testing.T) {
	cfg, err := loadYAML(t, baseValidYAML+"retention:\n  access_event_days: 0\n")
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "retention.access_event_days")
}

func TestLoad_ContentVersionRetentionLimit_ZeroKeepsAll(t *testing.T) {
	// 0 means "disable pruning / keep all versions"; it must NOT error.
	cfg, err := loadYAML(t, baseValidYAML+"retention:\n  content_version_limit: 0\n")
	require.NoError(t, err)
	assert.Equal(t, 0, cfg.Retention.ContentVersionLimit)
}

func TestLoad_MaxBodySize_NonPositive_ReturnsError(t *testing.T) {
	for _, v := range []string{"0", "-1"} {
		t.Run(v, func(t *testing.T) {
			cfg, err := loadYAML(t, baseValidYAML+"server:\n  max_body_size_bytes: "+v+"\n")
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), "server.max_body_size_bytes")
		})
	}
}

func TestLoad_RateLimits_NonPositive_ReturnsError(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"auth", "rate_limit:\n  auth_per_minute: 0\n", "rate_limit.auth_per_minute"},
		{"api", "rate_limit:\n  api_per_minute: 0\n", "rate_limit.api_per_minute"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadYAML(t, baseValidYAML+tc.body)
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestLoad_SearchRankWeightNegative_ReturnsError(t *testing.T) {
	cfg, err := loadYAML(t, baseValidYAML+"search:\n  rank_weight_relevance: -0.5\n")
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "search.rank_weight")
}

func TestValidateSearchRankingConfig(t *testing.T) {
	base := func() *Config {
		return &Config{Search: SearchConfig{
			RankWeightRelevance: 0.5,
			RankWeightCreated:   0.3,
			RankWeightUpdated:   0.2,
			RankHalfLifeDays:    90,
			RankCandidateCap:    200,
		}}
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"valid defaults", func(*Config) {}, false},
		{"negative relevance weight", func(c *Config) { c.Search.RankWeightRelevance = -0.1 }, true},
		{"all weights zero", func(c *Config) {
			c.Search.RankWeightRelevance = 0
			c.Search.RankWeightCreated = 0
			c.Search.RankWeightUpdated = 0
		}, true},
		{"zero half-life", func(c *Config) { c.Search.RankHalfLifeDays = 0 }, true},
		{"half-life above ceiling", func(c *Config) { c.Search.RankHalfLifeDays = maxSearchRankHalfLifeDays + 1 }, true},
		{"half-life at ceiling", func(c *Config) { c.Search.RankHalfLifeDays = maxSearchRankHalfLifeDays }, false},
		{"zero candidate cap", func(c *Config) { c.Search.RankCandidateCap = 0 }, true},
		{"candidate cap above ceiling", func(c *Config) { c.Search.RankCandidateCap = maxSearchRankCandidateCap + 1 }, true},
		{"candidate cap at ceiling", func(c *Config) { c.Search.RankCandidateCap = maxSearchRankCandidateCap }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			tt.mutate(cfg)
			err := validateSearchRankingConfig(cfg)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestValidateEncryptionKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid 32 bytes", validTestEncryptionKey, false},
		{"empty", "", true},
		{"too short", "shortkey", true},
		{"too long", "123456789012345678901234567890123", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateEncryptionKey(&Config{Security: SecurityConfig{EncryptionKey: tc.key}})
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- MCP issuer defaulting / OAuth AS agreement --------------------------

// asYAML returns a config.yaml enabling the embedded AS with the given explicit
// mcp.oauth_issuer (omitted when empty), in a production (non-localhost) frontend
// so the dev auto-derivation does not interfere.
func asYAML(issuer string) string {
	body := baseValidYAML + `
frontend:
  base_url: https://app.example.com
auth:
  oauth_as:
    issuer_url: https://connect.vibexp.io
mcp:
  resource_uri: https://connect.vibexp.io/mcp/v1/common
`
	if issuer != "" {
		body += "  oauth_issuer: " + issuer + "\n"
	}
	return body
}

func TestLoad_MCPOAuthIssuer_DefaultsToAS(t *testing.T) {
	cfg, err := loadYAML(t, asYAML(""))
	require.NoError(t, err)
	assert.Equal(t, "https://connect.vibexp.io", cfg.MCP.OAuthIssuer,
		"mcp.oauth_issuer must default to auth.oauth_as.issuer_url when the embedded AS is enabled")
}

func TestLoad_MCPOAuthIssuer_ExplicitMatchAllowed(t *testing.T) {
	cfg, err := loadYAML(t, asYAML("https://connect.vibexp.io"))
	require.NoError(t, err)
	assert.Equal(t, "https://connect.vibexp.io", cfg.MCP.OAuthIssuer)
}

func TestLoad_MCPOAuthIssuer_DivergentRejected(t *testing.T) {
	cfg, err := loadYAML(t, asYAML("https://evil.example"))
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "mcp.oauth_issuer")
}

// TestLoad_MCPOAuthIssuer_PreservedWhenASDisabled: with the AS disabled, an
// explicit external mcp.oauth_issuer is left untouched (production frontend so
// the dev derivation never fires).
func TestLoad_MCPOAuthIssuer_PreservedWhenASDisabled(t *testing.T) {
	cfg, err := loadYAML(t, baseValidYAML+`
frontend:
  base_url: https://app.example.com
mcp:
  oauth_issuer: https://external-idp.example
`)
	require.NoError(t, err)
	assert.Equal(t, "https://external-idp.example", cfg.MCP.OAuthIssuer)
}

func TestLoad_OAuthAS_RefreshTTLNotAboveAccessTTL_ReturnsError(t *testing.T) {
	cfg, err := loadYAML(t, baseValidYAML+`
frontend:
  base_url: https://app.example.com
auth:
  oauth_as:
    issuer_url: https://connect.vibexp.io
    access_token_ttl: 1h
    refresh_token_ttl: 30m
mcp:
  resource_uri: https://connect.vibexp.io/mcp/v1/common
`)
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "auth.oauth_as.refresh_token_ttl")
}

func TestLoad_OAuthAS_MissingResourceURI_ReturnsError(t *testing.T) {
	cfg, err := loadYAML(t, baseValidYAML+`
frontend:
  base_url: https://app.example.com
auth:
  oauth_as:
    issuer_url: https://connect.vibexp.io
`)
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "mcp.resource_uri")
}
