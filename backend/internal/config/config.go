package config

import (
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/observability"
	"github.com/vibexp/vibexp/pkg/events"
)

// Config is the fully-resolved, validated application configuration. It is
// loaded from a hierarchical config.yaml (see Load): code defaults are merged
// with the file, ${VAR} references in the file are interpolated against the
// process environment, and the result is unmarshalled into this nested struct.
// The koanf tags name each YAML section/key.
type Config struct {
	Server     ServerConfig     `koanf:"server"`
	Database   DatabaseConfig   `koanf:"database"`
	Security   SecurityConfig   `koanf:"security"`
	Auth       AuthConfig       `koanf:"auth"`
	MCP        MCPConfig        `koanf:"mcp"`
	Email      EmailConfig      `koanf:"email"`
	Frontend   FrontendConfig   `koanf:"frontend"`
	Search     SearchConfig     `koanf:"search"`
	Embedding  EmbeddingConfig  `koanf:"embedding"`
	GitHub     GitHubConfig     `koanf:"github"`
	Storage    StorageConfig    `koanf:"storage"`
	GCP        GCPConfig        `koanf:"gcp"`
	RateLimit  RateLimitConfig  `koanf:"rate_limit"`
	Retention  RetentionConfig  `koanf:"retention"`
	A2A        A2AConfig        `koanf:"a2a"`
	FCM        FCMConfig        `koanf:"fcm"`
	Deployment DeploymentConfig `koanf:"deployment"`

	// EventBus holds in-memory event-bus tuning (see pkg/events).
	EventBus events.Config `koanf:"event_bus"`
	// OTel holds OpenTelemetry export configuration (see internal/observability).
	OTel observability.Config `koanf:"otel"`
}

// ServerConfig holds HTTP server, logging, and build-metadata settings.
type ServerConfig struct {
	Port           string `koanf:"port"`
	LogLevel       string `koanf:"log_level"`
	LogFormat      string `koanf:"log_format"`
	ServiceVersion string `koanf:"service_version"`
	ReleaseSHA     string `koanf:"release_sha"`
	ReleaseDate    string `koanf:"release_date"`

	// MaxBodySizeBytes caps the size of request bodies the server will read for
	// general API routes (memory-exhaustion backstop). Defaults to 10MiB.
	MaxBodySizeBytes int64 `koanf:"max_body_size_bytes"`

	// CORSAllowedOrigins lists permitted CORS origins. When empty, only the
	// localhost dev origins are allowed (defaulted in Load); production frontend
	// origins must be supplied so no tenant-specific domains are hardcoded.
	CORSAllowedOrigins []string `koanf:"cors_allowed_origins"`

	// ErrorTypeBaseURI is the base URI used to build the RFC 9457 "type" member
	// of error responses (joined as <base>/<error-code>). Defaults to the
	// neutral "about:blank".
	ErrorTypeBaseURI string `koanf:"error_type_base_uri"`
}

// DatabaseConfig holds PostgreSQL connection settings. Host may be a Unix
// socket path (Cloud SQL) when it begins with '/'.
type DatabaseConfig struct {
	Host     string `koanf:"host"`
	Port     string `koanf:"port"`
	User     string `koanf:"user"`
	Password string `koanf:"password"`
	Name     string `koanf:"name"`
}

// SecurityConfig holds process-wide secrets and admin keys.
type SecurityConfig struct {
	// EncryptionKey encrypts sensitive data (API keys, OAuth-AS signing keys).
	// Required; must be exactly 32 bytes for AES-256 (see validateEncryptionKey).
	EncryptionKey string `koanf:"encryption_key"`
	// APIKeyCommon is the global API key for the common API surface.
	APIKeyCommon string `koanf:"api_key_common"`
	// BackofficeAdminAPIKey grants super-admin access to back-office endpoints.
	BackofficeAdminAPIKey string `koanf:"backoffice_admin_api_key"`
}

// AuthConfig holds web-login identity-provider settings and the embedded
// OAuth 2.1 Authorization Server configuration.
type AuthConfig struct {
	// Providers is the comma-separated (or YAML list) set of web-login identity
	// providers to enable simultaneously (e.g. "google,github,oidc"). When set it
	// takes precedence over Provider. Unknown names are ignored with a warning;
	// providers with missing credentials are skipped at startup. Matched
	// case-insensitively against "google", "github", and "oidc".
	Providers []string `koanf:"providers"`
	// Provider selects a single web-login provider; the backward-compatible shim
	// used only when Providers is empty.
	Provider string `koanf:"provider"`

	// SessionEncryptionKey is the hex-encoded secret backing the AES-256-GCM
	// session cookie (and, via domain separation, the OAuth state HMAC). It must
	// decode to exactly 32 bytes (64 hex chars). When empty, cookie session auth
	// is disabled (stub/test mode).
	SessionEncryptionKey string `koanf:"session_encryption_key"`

	// DevLoginEnabled gates the /api/v1/auth/dev/login endpoint. It must be
	// explicitly true AND the environment detected as development
	// (frontend.base_url points at localhost) for the endpoint to respond.
	DevLoginEnabled bool `koanf:"dev_login_enabled"`

	// SignInAllowedEmails restricts which email addresses may sign in. Empty
	// (default) means open registration; non-empty is an allow-list.
	SignInAllowedEmails []string `koanf:"signin_allowed_emails"`

	Google  GoogleAuthConfig `koanf:"google"`
	GitHub  GitHubAuthConfig `koanf:"github"`
	OIDC    OIDCAuthConfig   `koanf:"oidc"`
	OAuthAS OAuthASConfig    `koanf:"oauth_as"`
	APIAuth APIOAuthConfig   `koanf:"api_oauth"`
}

// GoogleAuthConfig is the Google OIDC web-login client (used when "google" is
// enabled). Google is reached directly via accounts.google.com discovery.
type GoogleAuthConfig struct {
	ClientID     string `koanf:"client_id"`
	ClientSecret string `koanf:"client_secret"`
	RedirectURI  string `koanf:"redirect_uri"`
}

// GitHubAuthConfig is the GitHub OAuth2 web-login client (used when "github" is
// enabled). GitHub is OAuth2, not OIDC; claims come from the GitHub REST API.
type GitHubAuthConfig struct {
	ClientID     string `koanf:"client_id"`
	ClientSecret string `koanf:"client_secret"`
	RedirectURI  string `koanf:"redirect_uri"`
}

// OIDCAuthConfig is the generic OIDC web-login client (used when "oidc" is
// enabled). Works with any OIDC-compliant issuer; IssuerURL drives discovery.
type OIDCAuthConfig struct {
	IssuerURL    string `koanf:"issuer_url"`
	ClientID     string `koanf:"client_id"`
	ClientSecret string `koanf:"client_secret"`
	RedirectURI  string `koanf:"redirect_uri"`
}

// OAuthASConfig holds the embedded OAuth 2.1 Authorization Server (issue #31).
// When IssuerURL is set the AS is mounted; empty disables it. IssuerURL is the
// public base URL and becomes the token `iss` and the metadata `issuer`; it
// must be HTTPS in production. Token lifespans must be positive and ordered.
type OAuthASConfig struct {
	IssuerURL           string        `koanf:"issuer_url"`
	AccessTokenTTL      time.Duration `koanf:"access_token_ttl"`
	RefreshTokenTTL     time.Duration `koanf:"refresh_token_ttl"`
	AuthCodeTTL         time.Duration `koanf:"auth_code_ttl"`
	KeyRotationInterval time.Duration `koanf:"key_rotation_interval"`
	// CleanupInterval is how often the AS prunes expired authorization codes,
	// tokens, PKCE and login sessions, and retired signing keys.
	CleanupInterval time.Duration `koanf:"cleanup_interval"`
}

// APIOAuthConfig configures the /api/v1 bearer-JWT path. When Issuer is set,
// /api/v1/* accepts AuthKit bearer JWTs (native OAuth clients) alongside
// session cookies and API keys; empty disables the JWT branch. Audiences
// optionally pins the JWT aud claim to an allow-list.
type APIOAuthConfig struct {
	Issuer    string   `koanf:"issuer"`
	Audiences []string `koanf:"audiences"`
}

// MCPConfig configures the MCP OAuth 2.1 resource server. The MCP endpoint
// delegates authorization to the configured issuer and validates bearer JWTs
// minted for ResourceURI (the audience, RFC 8707).
type MCPConfig struct {
	OAuthIssuer string `koanf:"oauth_issuer"`
	ResourceURI string `koanf:"resource_uri"`
}

// EmailConfig holds email delivery settings: the selected provider, shared
// sender/recipient addresses, and per-provider sub-structs.
type EmailConfig struct {
	// Provider selects the delivery backend: smtp (default), mailgun, postmark,
	// or sendgrid.
	Provider string `koanf:"provider"`
	// FromAddress is the sender address used by all providers; when empty it
	// falls back to SMTP.Username.
	FromAddress string `koanf:"from_address"`
	// ContactRecipientAddress is the destination for contact/support notification
	// emails; when empty it falls back to FromAddress, then SMTP.Username.
	ContactRecipientAddress string `koanf:"contact_recipient_address"`
	// PrivacyPolicyURL is the privacy-policy link in transactional email footers.
	PrivacyPolicyURL string `koanf:"privacy_policy_url"`

	SMTP     SMTPConfig     `koanf:"smtp"`
	Mailgun  MailgunConfig  `koanf:"mailgun"`
	Postmark PostmarkConfig `koanf:"postmark"`
	SendGrid SendGridConfig `koanf:"sendgrid"`
}

// SMTPConfig holds SMTP delivery settings (the default provider). When the host
// or port is absent the SMTP provider falls back to a no-op stub.
type SMTPConfig struct {
	Host     string `koanf:"host"`
	Port     string `koanf:"port"`
	Username string `koanf:"username"`
	Password string `koanf:"password"`
}

// MailgunConfig holds Mailgun settings; Domain and SendingKey are required when
// email.provider is "mailgun".
type MailgunConfig struct {
	BaseURL    string `koanf:"base_url"`
	Domain     string `koanf:"domain"`
	SendingKey string `koanf:"sending_key"`
}

// PostmarkConfig holds Postmark settings; ServerToken is required when
// email.provider is "postmark".
type PostmarkConfig struct {
	ServerToken   string `koanf:"server_token"`
	MessageStream string `koanf:"message_stream"`
}

// SendGridConfig holds SendGrid settings; APIKey is required when
// email.provider is "sendgrid".
type SendGridConfig struct {
	APIKey string `koanf:"api_key"`
}

// FrontendConfig holds the SPA base URL plus the deploy-time, non-secret
// frontend values served to the SPA via /config.js (window.__VIBEXP_ENV__).
// Each Site*/Brand*/GTM*/GA4 field mirrors a VITE_* the frontend otherwise
// bakes in at build time. SECURITY: /config.js is world-readable — only
// non-secret values belong here.
type FrontendConfig struct {
	// BaseURL is the frontend SPA base URL; used for redirects, email links, and
	// the IsLocalDevelopment heuristic.
	BaseURL string `koanf:"base_url"`

	SiteName         string `koanf:"site_name"`
	SiteLegalName    string `koanf:"site_legal_name"`
	SiteURL          string `koanf:"site_url"`
	TermsURL         string `koanf:"terms_url"`
	PrivacyURL       string `koanf:"privacy_url"`
	SupportEmail     string `koanf:"support_email"`
	BrandLogoURL     string `koanf:"brand_logo_url"`
	MCPEndpoint      string `koanf:"mcp_endpoint"`
	ErrorTypeBaseURI string `koanf:"error_type_base_uri"`
	GTMID            string `koanf:"gtm_id"`
	GTMEnabled       string `koanf:"gtm_enabled"`
	GA4MeasurementID string `koanf:"ga4_measurement_id"`
}

// SearchConfig holds search ranking parameters. When RecencyRankingEnabled is
// false (default) results keep relevance-only ordering; when true a weighted
// blend of relevance and freshness is used.
type SearchConfig struct {
	RecencyRankingEnabled bool    `koanf:"recency_ranking_enabled"`
	RankWeightRelevance   float64 `koanf:"rank_weight_relevance"`
	RankWeightCreated     float64 `koanf:"rank_weight_created"`
	RankWeightUpdated     float64 `koanf:"rank_weight_updated"`
	RankHalfLifeDays      float64 `koanf:"rank_half_life_days"`
	RankCandidateCap      int     `koanf:"rank_candidate_cap"`
}

// EmbeddingConfig holds the embedding model id and the in-Go chunker sizing.
type EmbeddingConfig struct {
	Model        string `koanf:"model"`
	ChunkSize    int    `koanf:"chunk_size"`
	ChunkOverlap int    `koanf:"chunk_overlap"`
}

// GitHubConfig holds GitHub App / integration settings (distinct from the
// GitHub web-login client in auth.github).
type GitHubConfig struct {
	AppID         string `koanf:"app_id"`
	AppSlug       string `koanf:"app_slug"`
	AppPrivateKey string `koanf:"app_private_key"`
	WebhookURL    string `koanf:"webhook_url"`
	WebhookSecret string `koanf:"webhook_secret"`
}

// StorageConfig holds resource-attachment storage settings.
type StorageConfig struct {
	// AttachmentsBucket is the GCS bucket backing artifact (and future resource)
	// file attachments. Empty (or an uninitialisable client) disables attachments
	// (upload/download/delete return 503).
	AttachmentsBucket string `koanf:"attachments_bucket"`
}

// GCPConfig holds Google Cloud settings used for observability and the internal
// job (Pub/Sub push) authentication.
type GCPConfig struct {
	// ProjectID is the GCP project id used for trace/log correlation. Optional.
	ProjectID string `koanf:"project_id"`
	// PubSubPushAudience is the OIDC token audience Cloud Scheduler mints for the
	// internal job endpoints; it must equal the public base URL the caller targets.
	PubSubPushAudience string `koanf:"pubsub_push_audience"`
	// PubSubPushServiceAccountSuffix restricts which service-account identities the
	// OIDC middleware accepts (the token email must end with this suffix). Empty
	// skips the service-account-domain check.
	PubSubPushServiceAccountSuffix string `koanf:"pubsub_push_service_account_suffix"`
}

// RateLimitConfig holds per-IP request rate limits (requests per minute),
// applied per route group. Each must be >= 1.
type RateLimitConfig struct {
	AuthPerMinute int `koanf:"auth_per_minute"`
	APIPerMinute  int `koanf:"api_per_minute"`
}

// RetentionConfig holds data-retention windows and limits.
type RetentionConfig struct {
	// ActivityDays / AccessEventDays must be in 1..3650.
	ActivityDays    int `koanf:"activity_days"`
	AccessEventDays int `koanf:"access_event_days"`
	// ContentVersionLimit bounds content-version snapshots per resource. 0 (or
	// negative) disables pruning, keeping every version.
	ContentVersionLimit int `koanf:"content_version_limit"`
}

// A2AConfig holds Agent-to-Agent client settings.
type A2AConfig struct {
	DefaultTimeout time.Duration `koanf:"default_timeout"`
}

// FCMConfig gates the Firebase Cloud Messaging web push channel.
type FCMConfig struct {
	Enabled bool `koanf:"enabled"`
}

// DeploymentConfig holds environment-detection indicators (see
// GetDeploymentEnvironment). Most are auto-populated by the hosting platform
// and surfaced into the YAML via ${VAR} interpolation.
type DeploymentConfig struct {
	OTelEnvironment       string `koanf:"otel_environment"`
	Environment           string `koanf:"environment"`
	Env                   string `koanf:"env"`
	DeploymentEnvironment string `koanf:"deployment_environment"`
	KubernetesServiceHost string `koanf:"kubernetes_service_host"`
	GoogleCloudProject    string `koanf:"google_cloud_project"`
	GCPProject            string `koanf:"gcp_project"`
	AWSRegion             string `koanf:"aws_region"`
	AWSDefaultRegion      string `koanf:"aws_default_region"`
	KService              string `koanf:"k_service"`
	KRevision             string `koanf:"k_revision"`
}

// RuntimeFrontendEnv returns the deploy-time frontend configuration served to
// the SPA via /config.js (window.__VIBEXP_ENV__). Keys are the VITE_* names the
// frontend reads through getEnv(); only non-empty values are included so the
// frontend's build-time defaults remain the fallback for anything unset. The
// result is served publicly and MUST contain only non-secret values.
func (c *Config) RuntimeFrontendEnv() map[string]string {
	pairs := []struct{ key, val string }{
		{"VITE_SITE_NAME", c.Frontend.SiteName},
		{"VITE_SITE_LEGAL_NAME", c.Frontend.SiteLegalName},
		{"VITE_SITE_URL", c.Frontend.SiteURL},
		{"VITE_TERMS_URL", c.Frontend.TermsURL},
		{"VITE_PRIVACY_URL", c.Frontend.PrivacyURL},
		{"VITE_SUPPORT_EMAIL", c.Frontend.SupportEmail},
		{"VITE_BRAND_LOGO_URL", c.Frontend.BrandLogoURL},
		{"VITE_MCP_ENDPOINT", c.Frontend.MCPEndpoint},
		{"VITE_ERROR_TYPE_BASE_URI", c.Frontend.ErrorTypeBaseURI},
		{"VITE_GTM_ID", c.Frontend.GTMID},
		{"VITE_GTM_ENABLED", c.Frontend.GTMEnabled},
		{"VITE_GA4_MEASUREMENT_ID", c.Frontend.GA4MeasurementID},
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		if p.val != "" {
			out[p.key] = p.val
		}
	}
	return out
}

// GitHubAppConfig holds parsed GitHub App configuration
type GitHubAppConfig struct {
	AppID         string
	PrivateKey    *rsa.PrivateKey
	PrivateKeyPEM []byte // PEM-encoded private key for ghinstallation
	WebhookSecret string
}

// GetGitHubAppConfig returns the GitHub App configuration with parsed private key
func (c *Config) GetGitHubAppConfig() (*GitHubAppConfig, error) {
	if c.GitHub.AppID == "" || c.GitHub.AppPrivateKey == "" {
		return nil, nil // No GitHub App configured
	}

	// Try to decode from base64 first (for easier config management).
	// If decoding fails, treat it as raw PEM.
	// This automatic fallback allows keys to be stored in either format.
	privateKeyBytes := []byte(c.GitHub.AppPrivateKey)
	isBase64Encoded := false
	if decoded, err := base64.StdEncoding.DecodeString(c.GitHub.AppPrivateKey); err == nil {
		// Successfully decoded from base64
		privateKeyBytes = decoded
		isBase64Encoded = true
	}

	// Log which format was detected (visible during startup)
	if isBase64Encoded {
		slog.Info("GitHub App private key loaded from base64-encoded format")
	} else {
		slog.Info("GitHub App private key loaded from raw PEM format")
	}

	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GitHub App private key: %w", err)
	}

	return &GitHubAppConfig{
		AppID:         c.GitHub.AppID,
		PrivateKey:    privateKey,
		PrivateKeyPEM: privateKeyBytes, // Store PEM bytes for ghinstallation
		WebhookSecret: c.GitHub.WebhookSecret,
	}, nil
}

// GetDeploymentEnvironment determines the deployment environment from config.
// It checks otel_environment first, then standard env-derived vars, then cloud
// indicators, then defaults to production.
func (c *Config) GetDeploymentEnvironment() string {
	d := c.Deployment
	// Check OpenTelemetry standard env var
	if d.OTelEnvironment != "" {
		return d.OTelEnvironment
	}

	// Check common deployment environment variables
	if d.Environment != "" {
		return d.Environment
	}
	if d.Env != "" {
		return d.Env
	}
	if d.DeploymentEnvironment != "" {
		return d.DeploymentEnvironment
	}

	// Check common cloud provider environment indicators
	if d.KubernetesServiceHost != "" {
		return "kubernetes"
	}
	if d.GoogleCloudProject != "" || d.GCPProject != "" {
		return "gcp-cloud-run"
	}
	if d.AWSRegion != "" || d.AWSDefaultRegion != "" {
		return "aws"
	}

	// Default to production
	return "production"
}

// maxSearchRankHalfLifeDays caps the half-life at 100 years. This keeps the
// days→time.Duration conversion (in ProvideSearchService) well clear of int64
// nanosecond overflow, which would otherwise wrap to a negative duration and
// silently zero the recency contribution.
const maxSearchRankHalfLifeDays = 36500

// maxSearchRankCandidateCap bounds the re-rank candidate pool so a misconfigured
// cap cannot blow up per-query memory and sort cost (the cap becomes the SQL
// LIMIT and the in-memory slice that is sorted on every ranked query).
const maxSearchRankCandidateCap = 5000

// validateSearchRankingConfig rejects degenerate ranking parameters so a
// misconfigured deployment fails fast at startup rather than silently producing
// garbage ordering. Weights must be non-negative (and not all zero); the
// half-life and candidate cap must each be positive and within a sane ceiling.
func validateSearchRankingConfig(cfg *Config) error {
	s := cfg.Search
	weights := []float64{s.RankWeightRelevance, s.RankWeightCreated, s.RankWeightUpdated}
	var sum float64
	for _, w := range weights {
		if w < 0 {
			return fmt.Errorf("search.rank_weight_* must be non-negative, got %v", weights)
		}
		sum += w
	}
	if sum == 0 {
		return fmt.Errorf("search.rank_weight_* must not all be zero")
	}
	if s.RankHalfLifeDays <= 0 {
		return fmt.Errorf("search.rank_half_life_days must be positive, got %v", s.RankHalfLifeDays)
	}
	if s.RankHalfLifeDays > maxSearchRankHalfLifeDays {
		return fmt.Errorf("search.rank_half_life_days must be <= %d, got %v",
			maxSearchRankHalfLifeDays, s.RankHalfLifeDays)
	}
	if s.RankCandidateCap < 1 {
		return fmt.Errorf("search.rank_candidate_cap must be >= 1, got %d", s.RankCandidateCap)
	}
	if s.RankCandidateCap > maxSearchRankCandidateCap {
		return fmt.Errorf("search.rank_candidate_cap must be <= %d, got %d",
			maxSearchRankCandidateCap, s.RankCandidateCap)
	}
	return nil
}

// encryptionKeyLength is the required AES-256 key length in bytes.
const encryptionKeyLength = 32

// validateEncryptionKey enforces that security.encryption_key is present and
// exactly 32 bytes so the service fails closed at startup rather than running
// with a weak/default key.
func validateEncryptionKey(cfg *Config) error {
	if cfg.Security.EncryptionKey == "" {
		return fmt.Errorf("security.encryption_key is required and must be exactly %d bytes", encryptionKeyLength)
	}
	if len(cfg.Security.EncryptionKey) != encryptionKeyLength {
		return fmt.Errorf("security.encryption_key must be exactly %d bytes, got %d",
			encryptionKeyLength, len(cfg.Security.EncryptionKey))
	}
	return nil
}

// IsLocalDevelopment reports whether the process is running in local development,
// detected from frontend.base_url pointing at localhost/127.0.0.1. An empty value
// is treated as NOT development (fail-closed) so a misconfigured deployment never
// enables dev-only paths. This is the single source of truth for the dev
// heuristic, shared by services.EnvironmentService.IsDevelopment and the dev-only
// config derivation (applyDevOAuthASDefaults); production never matches.
func (c *Config) IsLocalDevelopment() bool {
	u := strings.ToLower(c.Frontend.BaseURL)
	if u == "" {
		return false
	}
	return strings.Contains(u, "localhost") || strings.Contains(u, "127.0.0.1")
}

// applyDevOAuthASDefaults auto-enables the embedded Authorization Server for local
// development by deriving sane defaults when they are left unset, so a fresh
// checkout boots a connectable MCP endpoint with zero auth configuration. It runs
// ONLY in local development (frontend.base_url points at localhost); production
// keeps the AS strictly opt-in and never guesses a public issuer. Explicit config
// always wins — a value already set is never overwritten. The derived issuer is
// the server's own local base URL (http://localhost:<PORT>) and the resource URI
// is <issuer>/mcp/v1/common.
func applyDevOAuthASDefaults(cfg *Config) {
	if !cfg.IsLocalDevelopment() {
		return
	}
	// Respect an explicit opt-out: if the developer pointed the MCP resource server
	// at their own external issuer (mcp.oauth_issuer set) without enabling the
	// embedded AS, do not auto-enable it — that would force a conflicting issuer
	// onto their setup. Only the truly-unconfigured local case is auto-enabled.
	if cfg.Auth.OAuthAS.IssuerURL == "" && cfg.MCP.OAuthIssuer != "" {
		return
	}
	if cfg.Auth.OAuthAS.IssuerURL == "" {
		cfg.Auth.OAuthAS.IssuerURL = "http://localhost:" + cfg.Server.Port
	}
	if cfg.MCP.ResourceURI == "" {
		cfg.MCP.ResourceURI = strings.TrimRight(cfg.Auth.OAuthAS.IssuerURL, "/") + "/mcp/v1/common"
	}
}

// applyMCPIssuerDefault points the MCP resource server at the embedded
// Authorization Server when the AS is enabled and no explicit mcp.oauth_issuer is
// set, so the protected-resource metadata advertises VibeXP itself. An explicit
// mcp.oauth_issuer still wins but must agree with the AS issuer (enforced by
// validateOAuthASConfig).
func applyMCPIssuerDefault(cfg *Config) {
	if cfg.Auth.OAuthAS.IssuerURL != "" && cfg.MCP.OAuthIssuer == "" {
		cfg.MCP.OAuthIssuer = cfg.Auth.OAuthAS.IssuerURL
	}
}

// validateOAuthASConfig validates the embedded Authorization Server settings.
// The AS is opt-in: when auth.oauth_as.issuer_url is empty it is disabled and no
// other field is checked. When enabled, a usable MCP resource URI (the token
// audience) and sane, ordered token lifespans are required so the server fails
// closed at startup rather than minting unbound or never-expiring tokens.
func validateOAuthASConfig(cfg *Config) error {
	as := cfg.Auth.OAuthAS
	if as.IssuerURL == "" {
		return nil
	}
	if cfg.MCP.ResourceURI == "" {
		return fmt.Errorf(
			"mcp.resource_uri is required when auth.oauth_as.issuer_url is set (it is the issued token audience)")
	}
	// The MCP resource server must trust the embedded AS as its issuer; an
	// explicit, divergent mcp.oauth_issuer would make the AS mint tokens the
	// resource server rejects. Leaving mcp.oauth_issuer unset is the norm — Load
	// defaults it to auth.oauth_as.issuer_url.
	if cfg.MCP.OAuthIssuer != "" && cfg.MCP.OAuthIssuer != as.IssuerURL {
		return fmt.Errorf(
			"mcp.oauth_issuer (%q) must equal auth.oauth_as.issuer_url (%q), or be left unset to default to it",
			cfg.MCP.OAuthIssuer, as.IssuerURL)
	}
	if as.AccessTokenTTL <= 0 {
		return fmt.Errorf("auth.oauth_as.access_token_ttl must be positive, got %v", as.AccessTokenTTL)
	}
	if as.AuthCodeTTL <= 0 {
		return fmt.Errorf("auth.oauth_as.auth_code_ttl must be positive, got %v", as.AuthCodeTTL)
	}
	if as.RefreshTokenTTL <= as.AccessTokenTTL {
		return fmt.Errorf("auth.oauth_as.refresh_token_ttl (%v) must exceed auth.oauth_as.access_token_ttl (%v)",
			as.RefreshTokenTTL, as.AccessTokenTTL)
	}
	if as.KeyRotationInterval <= 0 {
		return fmt.Errorf("auth.oauth_as.key_rotation_interval must be positive, got %v", as.KeyRotationInterval)
	}
	if as.CleanupInterval <= 0 {
		return fmt.Errorf("auth.oauth_as.cleanup_interval must be positive, got %v", as.CleanupInterval)
	}
	return nil
}

// validateRetentionDays enforces the shared 1..3650-day window (1 day to 10 years)
// for a retention setting. Rejecting 0/negatives prevents deleting all rows; the
// upper bound prevents silently violating the retention intent.
func validateRetentionDays(yamlPath string, value int) error {
	if value < 1 || value > 3650 {
		return fmt.Errorf("%s must be between 1 and 3650, got %d", yamlPath, value)
	}
	return nil
}

// validateEmbeddingConfig enforces that the embedding model is named and the
// chunker sizing is sane, so the service fails closed at startup rather than
// embedding/searching with an empty model_id or a stalled chunker.
func validateEmbeddingConfig(cfg *Config) error {
	e := cfg.Embedding
	if e.Model == "" {
		return fmt.Errorf("embedding.model is required and must be non-empty")
	}
	if e.ChunkSize < 1 {
		return fmt.Errorf("embedding.chunk_size must be >= 1, got %d", e.ChunkSize)
	}
	if e.ChunkOverlap < 0 || e.ChunkOverlap >= e.ChunkSize {
		return fmt.Errorf(
			"embedding.chunk_overlap must be in [0, embedding.chunk_size), got %d (chunk size %d)",
			e.ChunkOverlap, e.ChunkSize,
		)
	}
	return nil
}

// validateRateLimits enforces that every per-IP rate limit is positive; a
// non-positive value would reject every request to the guarded route group.
func validateRateLimits(cfg *Config) error {
	if cfg.RateLimit.AuthPerMinute < 1 {
		return fmt.Errorf("rate_limit.auth_per_minute must be >= 1, got %d", cfg.RateLimit.AuthPerMinute)
	}
	if cfg.RateLimit.APIPerMinute < 1 {
		return fmt.Errorf("rate_limit.api_per_minute must be >= 1, got %d", cfg.RateLimit.APIPerMinute)
	}
	return nil
}

// validateBodyAndRetention runs the simple positivity/range checks that do not
// warrant their own function. MaxBodySizeBytes must be positive (a zero/negative
// cap would reject every request body), and the retention windows must be in range.
func validateBodyAndRetention(cfg *Config) error {
	if cfg.Server.MaxBodySizeBytes < 1 {
		return fmt.Errorf("server.max_body_size_bytes must be >= 1, got %d", cfg.Server.MaxBodySizeBytes)
	}
	if err := validateRetentionDays("retention.activity_days", cfg.Retention.ActivityDays); err != nil {
		return err
	}
	return validateRetentionDays("retention.access_event_days", cfg.Retention.AccessEventDays)
}

// validateAll runs every config invariant in order, returning the first failure
// so the service fails closed at startup. applyDevOAuthASDefaults /
// applyMCPIssuerDefault must already have run (validateOAuthASConfig depends on
// the derived MCP issuer).
func validateAll(cfg *Config) error {
	checks := []func(*Config) error{
		validateBodyAndRetention,
		validateRateLimits,
		validateSearchRankingConfig,
		validateEmbeddingConfig,
		validateEncryptionKey,
		validateOAuthASConfig,
	}
	for _, check := range checks {
		if err := check(cfg); err != nil {
			return err
		}
	}
	return nil
}

// configFileDefaultPath is the config file path used when neither --config nor
// VIBEXP_CONFIG_FILE is provided.
const configFileDefaultPath = "./config.yaml"

// defaults returns the code-level configuration defaults as flat, dot-delimited
// keys. They are merged first (lowest precedence); the config.yaml file overrides
// any of them. Duration defaults are expressed as strings ("15m") and decoded by
// the time.Duration hook, matching how the YAML file expresses them.
func defaults() map[string]any {
	return map[string]any{
		"server.port":                         "8080",
		"server.log_level":                    "info",
		"server.log_format":                   "json",
		"server.service_version":              "dev",
		"server.release_sha":                  "dev",
		"server.release_date":                 "unknown",
		"server.max_body_size_bytes":          int64(10 << 20),
		"server.error_type_base_uri":          "about:blank",
		"database.host":                       "localhost",
		"database.port":                       "5432",
		"database.user":                       "postgres",
		"database.name":                       "vibexp_io",
		"auth.google.redirect_uri":            "http://localhost:8080/api/v1/auth/callback",
		"auth.github.redirect_uri":            "http://localhost:8080/api/v1/auth/callback",
		"auth.oidc.redirect_uri":              "http://localhost:8080/api/v1/auth/callback",
		"auth.oauth_as.access_token_ttl":      "15m",
		"auth.oauth_as.refresh_token_ttl":     "720h",
		"auth.oauth_as.auth_code_ttl":         "10m",
		"auth.oauth_as.key_rotation_interval": "720h",
		"auth.oauth_as.cleanup_interval":      "1h",
		"email.provider":                      "smtp",
		"email.privacy_policy_url":            "https://example.com/privacy-policy",
		"email.smtp.host":                     "smtp.gmail.com",
		"email.smtp.port":                     "587",
		"email.postmark.message_stream":       "outbound",
		"frontend.base_url":                   "http://localhost:5173",
		"embedding.model":                     "gemini-embedding-001",
		"embedding.chunk_size":                1000,
		"embedding.chunk_overlap":             200,
		"search.rank_weight_relevance":        0.5,
		"search.rank_weight_created":          0.3,
		"search.rank_weight_updated":          0.2,
		"search.rank_half_life_days":          90.0,
		"search.rank_candidate_cap":           200,
		"rate_limit.auth_per_minute":          100,
		"rate_limit.api_per_minute":           1000,
		"retention.activity_days":             90,
		"retention.access_event_days":         90,
		"retention.content_version_limit":     20,
		"a2a.default_timeout":                 "5m",
		"event_bus.worker_count":              20,
		"event_bus.buffer_size":               500,
		"event_bus.max_retries":               3,
		"event_bus.retry_backoff":             "200ms",
		"event_bus.retry_jitter":              true,
		"otel.endpoint":                       "localhost:4317",
		"otel.export_interval":                "60s",
		"otel.trace_sample_ratio":             0.1,
	}
}

// resolveExpr resolves a single ${...} expression body (without the braces).
// Grammar: "VAR" resolves to the environment value of VAR (empty + a warning when
// unset); "VAR:-default" resolves to the environment value of VAR when set and
// non-empty, otherwise to default.
func resolveExpr(expr string) string {
	if idx := strings.Index(expr, ":-"); idx >= 0 {
		name := expr[:idx]
		def := expr[idx+2:]
		if v, ok := os.LookupEnv(name); ok && v != "" {
			return v
		}
		return def
	}
	if v, ok := os.LookupEnv(expr); ok {
		return v
	}
	slog.Warn("config: environment variable referenced in config file is not set; using empty value",
		"variable", expr)
	return ""
}

// interpolateString expands ${VAR} and ${VAR:-default} references in s against
// the process environment. A literal "${...}" is written as "$${...}": the "$$"
// escape collapses to a single "$" and the following "{...}" is left untouched.
func interpolateString(s string) string {
	if !strings.Contains(s, "$") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' {
			b.WriteByte(s[i])
			i++
			continue
		}
		// "$$" → literal "$" (escapes a following "${...}" to "${...}").
		if i+1 < len(s) && s[i+1] == '$' {
			b.WriteByte('$')
			i += 2
			continue
		}
		// "${...}" → resolved value.
		if i+1 < len(s) && s[i+1] == '{' {
			if end := strings.IndexByte(s[i+2:], '}'); end >= 0 {
				b.WriteString(resolveExpr(s[i+2 : i+2+end]))
				i += 2 + end + 1
				continue
			}
		}
		// A lone "$" not starting a valid token.
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// interpolateNode recursively interpolates ${VAR} references in every string
// scalar of a parsed config tree (maps and slices are walked; non-string scalars
// are returned unchanged). Operating on the parsed structure — not the raw bytes —
// keeps interpolation from ever corrupting YAML syntax.
func interpolateNode(node any) any {
	switch v := node.(type) {
	case string:
		return interpolateString(v)
	case map[string]any:
		for key, val := range v {
			v[key] = interpolateNode(val)
		}
		return v
	case []any:
		for i, val := range v {
			v[i] = interpolateNode(val)
		}
		return v
	default:
		return node
	}
}

// decode builds the nested Config from defaults + the config.yaml at path, with
// ${VAR} interpolation applied to the file's string scalars. The required file is
// read first so a missing config produces a clear, actionable error.
func decode(path string) (*Config, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- path is an operator-provided config file, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"config file %q not found: a config.yaml is required (copy config.example.yaml and edit it, "+
					"or pass --config / set VIBEXP_CONFIG_FILE)", path)
		}
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	parsed, err := yaml.Parser().Unmarshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file %q: %w", path, err)
	}
	interpolateNode(parsed)

	k := koanf.New(".")
	if err := k.Load(confmap.Provider(defaults(), "."), nil); err != nil {
		return nil, fmt.Errorf("failed to load config defaults: %w", err)
	}
	if err := k.Load(confmap.Provider(parsed, "."), nil); err != nil {
		return nil, fmt.Errorf("failed to load config file %q: %w", path, err)
	}

	var cfg Config
	unmarshalConf := koanf.UnmarshalConf{
		Tag: "koanf",
		DecoderConfig: &mapstructure.DecoderConfig{
			WeaklyTypedInput: true,
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				mapstructure.StringToSliceHookFunc(","),
			),
		},
	}
	if err := k.UnmarshalWithConf("", &cfg, unmarshalConf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return &cfg, nil
}

// Load reads, interpolates, validates, and returns the application configuration
// from the config.yaml at path. An empty path falls back to VIBEXP_CONFIG_FILE,
// then ./config.yaml. The file is required: a missing file fails fast with a
// message naming the expected path and config.example.yaml.
func Load(path string) (*Config, error) {
	if path == "" {
		if env, ok := os.LookupEnv("VIBEXP_CONFIG_FILE"); ok && env != "" {
			path = env
		} else {
			path = configFileDefaultPath
		}
	}

	cfg, err := decode(path)
	if err != nil {
		return nil, err
	}

	// Derive local-dev defaults that auto-enable the embedded AS BEFORE pointing
	// the MCP issuer at it, so mcp.oauth_issuer picks up the derived issuer. No-op
	// in production and whenever the values are set explicitly.
	applyDevOAuthASDefaults(cfg)
	applyMCPIssuerDefault(cfg)

	if err := validateAll(cfg); err != nil {
		return nil, err
	}

	// Default CORS allowed origins when not provided. Only localhost dev origins
	// are defaulted; production frontend origins must be supplied via
	// server.cors_allowed_origins so no tenant-specific domains are hardcoded.
	if len(cfg.Server.CORSAllowedOrigins) == 0 {
		cfg.Server.CORSAllowedOrigins = []string{
			"http://localhost:5173",
			"http://localhost:5174",
		}
	}

	// Propagate the configured error-type base URI to the errors package so
	// RFC 9457 "type" URIs are built from it.
	apierrors.SetTypeBaseURI(cfg.Server.ErrorTypeBaseURI)

	return cfg, nil
}
