package config

import (
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kelseyhightower/envconfig"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/observability"
	"github.com/vibexp/vibexp/pkg/events"
)

type Config struct {
	Port     string `envconfig:"PORT" default:"8080"`
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
	// LogFormat selects the log output format: "json" (default) or "text".
	LogFormat string `envconfig:"LOG_FORMAT" default:"json"`

	// Service version for metrics and observability
	ServiceVersion string `envconfig:"SERVICE_VERSION" default:"dev"`

	// Release metadata for production observability
	ReleaseSHA  string `envconfig:"RELEASE_SHA" default:"dev"`
	ReleaseDate string `envconfig:"RELEASE_DATE" default:"unknown"`

	// Database configuration
	DBHost     string `envconfig:"DB_HOST" default:"localhost"`
	DBPort     string `envconfig:"DB_PORT" default:"5432"`
	DBUser     string `envconfig:"DB_USER" default:"postgres"`
	DBPassword string `envconfig:"DB_PASSWORD" default:""`
	DBName     string `envconfig:"DB_NAME" default:"vibexp_io"`

	// API Key configuration
	APIKeyCommon string `envconfig:"API_KEY_COMMON" default:""`

	// Encryption key for sensitive data (API keys, etc.). Required; must be exactly
	// 32 bytes for AES-256. No default — the service fails to start if it is unset
	// or the wrong length (see validateEncryptionKey).
	EncryptionKey string `envconfig:"ENCRYPTION_KEY"`

	// Back-office admin API key for super admin access
	BackofficeAdminAPIKey string `envconfig:"BACKOFFICE_ADMIN_API_KEY" default:""`

	// AuthProvider selects the web-login identity provider. Valid values:
	// "workos", "oidc", or "" (none). When empty, the provider is auto-detected
	// from WorkOS credentials for backward compatibility, falling back to a
	// no-op stub (dev login only). The value is matched case-insensitively.
	AuthProvider string `envconfig:"AUTH_PROVIDER" default:""`

	// WorkOS AuthKit configuration (active identity provider)
	WorkOSAPIKey         string `envconfig:"WORKOS_API_KEY" default:""`
	WorkOSClientID       string `envconfig:"WORKOS_CLIENT_ID" default:""`
	WorkOSCookiePassword string `envconfig:"WORKOS_COOKIE_PASSWORD" default:""`
	WorkOSRedirectURI    string `envconfig:"WORKOS_REDIRECT_URI" default:"http://localhost:8080/api/v1/auth/callback"`

	// Generic OIDC provider configuration (used when AUTH_PROVIDER=oidc). Works
	// with any OIDC-compliant issuer (Keycloak, Authentik, Zitadel, Auth0,
	// Google). OIDCIssuerURL is used for OIDC discovery at startup.
	OIDCIssuerURL    string `envconfig:"OIDC_ISSUER_URL" default:""`
	OIDCClientID     string `envconfig:"OIDC_CLIENT_ID" default:""`
	OIDCClientSecret string `envconfig:"OIDC_CLIENT_SECRET" default:""`
	OIDCRedirectURI  string `envconfig:"OIDC_REDIRECT_URI" default:"http://localhost:8080/api/v1/auth/callback"`

	// MCP OAuth 2.1 resource-server configuration. The MCP endpoint delegates
	// authorization to WorkOS AuthKit (the authorization server) and validates
	// bearer JWTs minted for the MCPResourceURI audience. MCPOAuthIssuer differs
	// per environment (prod vs staging AuthKit domain) and must never be hardcoded.
	MCPOAuthIssuer string `envconfig:"MCP_OAUTH_ISSUER" default:""`
	MCPResourceURI string `envconfig:"MCP_RESOURCE_URI" default:""`

	// API-surface OAuth configuration. When APIOAuthIssuer is set, /api/v1/*
	// accepts AuthKit bearer JWTs (mobile and other native OAuth clients)
	// alongside session cookies and API keys; empty disables the JWT branch
	// and non-API-key bearer tokens keep getting 401. APIOAuthAudiences
	// optionally pins the JWT aud claim to an allow-list. Plain AuthKit PKCE
	// access tokens carry no aud claim (no RFC 8707 resource indicator), so
	// the default accepts any audience EXCEPT the MCP resource URI (an MCP
	// client's narrow grant must not double as an API credential); issuer,
	// signature, expiry, and subject-to-user binding are always enforced.
	// Note the accepted-token surface this implies: ANY AuthKit access token
	// from this issuer for a provisioned user is an API credential — including
	// the access token inside every web session — so such tokens must never be
	// logged or forwarded. Distinct from the MCP resource-server config above,
	// which requires RFC 8707 audience binding.
	APIOAuthIssuer    string   `envconfig:"API_OAUTH_ISSUER" default:""`
	APIOAuthAudiences []string `envconfig:"API_OAUTH_AUDIENCES"`

	// DevLoginEnabled gates the /api/v1/auth/dev/login endpoint.
	// Defaults to false (off) so misconfigured environments cannot
	// accidentally expose unauthenticated user impersonation. Must be
	// explicitly set AND the environment must be detected as development
	// (FRONTEND_BASE_URL points at localhost) for the endpoint to respond.
	DevLoginEnabled bool `envconfig:"DEV_LOGIN_ENABLED" default:"false"`

	// SignInAllowedEmails restricts which email addresses may sign in. When
	// empty (the default), registration is open and anyone may sign in. When
	// non-empty, only the listed addresses are permitted. Provide a
	// comma-separated list, e.g. "alice@example.com,bob@example.com".
	SignInAllowedEmails []string `envconfig:"SIGNIN_ALLOWED_EMAILS"`

	// SMTP configuration
	SMTPHost     string `envconfig:"SMTP_HOST" default:"smtp.gmail.com"`
	SMTPPort     string `envconfig:"SMTP_PORT" default:"587"`
	SMTPUsername string `envconfig:"SMTP_USERNAME" default:""`
	SMTPPassword string `envconfig:"SMTP_PASSWORD" default:""`

	// Email provider selection
	// EmailProvider selects the email delivery backend. Valid values: smtp, mailgun.
	// Defaults to "smtp" for backwards compatibility.
	EmailProvider string `envconfig:"EMAIL_PROVIDER" default:"smtp"`
	// EmailFromAddress is the sender address used by all email providers.
	// When empty, falls back to SMTPUsername for backwards compatibility.
	EmailFromAddress string `envconfig:"EMAIL_FROM_ADDRESS" default:""`

	// ContactRecipientAddress is the destination for contact-form and support
	// notification emails (the "admin" inbox). When empty it falls back to
	// EmailFromAddress, then SMTPUsername. No tenant-specific address is baked in.
	ContactRecipientAddress string `envconfig:"CONTACT_RECIPIENT_ADDRESS" default:""`

	// Mailgun configuration
	MailgunBaseURL string `envconfig:"MAILGUN_BASE_URL" default:""`
	// MailgunDomain is the Mailgun sending domain (e.g. mg.example.com).
	// Required when EMAIL_PROVIDER=mailgun.
	MailgunDomain     string `envconfig:"MAILGUN_DOMAIN" default:""`
	MailgunSendingKey string `envconfig:"MAILGUN_SENDING_KEY" default:""`

	// Postmark configuration
	// PostmarkServerToken is the Postmark Server API token used for sending.
	// Required when EMAIL_PROVIDER=postmark.
	PostmarkServerToken string `envconfig:"POSTMARK_SERVER_TOKEN" default:""`
	// PostmarkMessageStream selects the Postmark message stream to send on.
	// Defaults to "outbound" (the default transactional stream).
	PostmarkMessageStream string `envconfig:"POSTMARK_MESSAGE_STREAM" default:"outbound"`

	// SendGrid configuration
	// SendGridAPIKey is the SendGrid API key (needs "Mail Send" permission).
	// Required when EMAIL_PROVIDER=sendgrid.
	SendGridAPIKey string `envconfig:"SENDGRID_API_KEY" default:""`

	// Frontend URL configuration for redirects
	FrontendBaseURL string `envconfig:"FRONTEND_BASE_URL" default:"http://localhost:5173"`

	// PrivacyPolicyURL is the public URL of the privacy policy, linked from the
	// footer of transactional emails. Defaults to a neutral placeholder; set
	// PRIVACY_POLICY_URL to your deployment's privacy policy page. No
	// tenant-specific domain is baked in.
	PrivacyPolicyURL string `envconfig:"PRIVACY_POLICY_URL" default:"https://example.com/privacy-policy"`

	// CORS configuration. When empty, only the localhost dev origins are
	// allowed; set CORS_ALLOWED_ORIGINS (comma-separated) to permit your
	// production frontend origins. No production origins are baked in.
	CORSAllowedOrigins []string `envconfig:"CORS_ALLOWED_ORIGINS"`

	// ErrorTypeBaseURI is the base URI used to build the RFC 9457 "type" member
	// of error responses (joined as <base>/<error-code>). Defaults to the
	// neutral "about:blank"; set it to a public URL that documents your error
	// codes (e.g. https://example.com/errors) to point clients at human-readable
	// docs.
	ErrorTypeBaseURI string `envconfig:"ERROR_TYPE_BASE_URI" default:"about:blank"`

	// Resource attachments (GCS) configuration. AttachmentsBucket is the GCS
	// bucket backing artifact (and future resource) file attachments, accessed
	// via Workload Identity / Application Default Credentials — no service
	// account JSON key. When empty (the default), or when the GCS client cannot
	// be initialized (e.g. a credential-less local/CI environment), the
	// attachments subsystem is disabled and upload/download/delete return 503
	// rather than crashing startup. Set it to your GCS bucket name to enable
	// attachments.
	AttachmentsBucket string `envconfig:"GCS_RESOURCE_ATTACHMENTS_BUCKET" default:""`

	// GCPProjectID is the Google Cloud project id used for observability
	// (trace/log correlation in the tracing exporter and request logger). It is
	// optional and empty by default; it is NOT required for embedding generation,
	// which is broker-free and runs in-process.
	GCPProjectID string `envconfig:"GCP_PROJECT_ID" default:""`

	// PubSubPushAudience is the OIDC token audience Cloud Scheduler (and any other
	// Google OIDC push caller) mints its bearer token for; the internal job
	// endpoints under /internal/jobs/* validate the incoming ID token against this
	// value. It must equal the public base URL the caller targets. No default.
	PubSubPushAudience string `envconfig:"PUBSUB_PUSH_AUDIENCE" default:""`

	// PubSubPushServiceAccountSuffix restricts which service-account identities the
	// OIDC middleware accepts for the internal job endpoints: the token's email
	// claim must end with this suffix (e.g. "@my-project.iam.gserviceaccount.com").
	// When empty, the service-account-domain check is skipped (issuer, signature
	// and audience are still enforced).
	PubSubPushServiceAccountSuffix string `envconfig:"PUBSUB_PUSH_SERVICE_ACCOUNT_SUFFIX" default:""`

	// EmbeddingModel is the model identifier used to embed both documents and search
	// queries; it is the model_id tag written on every embedding row and the filter
	// search uses to keep query and document vectors comparable. Operators set it to
	// match the model their configured provider serves.
	//
	// The embedding vector width is NOT configured here — it is the fixed constant
	// services.EmbeddingVectorDimensions (locked to the vector(N) migration). The
	// active provider (base URL, API key, type) lives in the embedding_providers table.
	EmbeddingModel string `envconfig:"EMBEDDING_MODEL" default:"gemini-embedding-001"`

	// EmbeddingChunkSize and EmbeddingChunkOverlap configure the in-Go document
	// chunker (rune-based sliding window). Overlap preserves context across chunk
	// boundaries and must be smaller than the chunk size.
	EmbeddingChunkSize    int `envconfig:"EMBEDDING_CHUNK_SIZE" default:"1000"`
	EmbeddingChunkOverlap int `envconfig:"EMBEDDING_CHUNK_OVERLAP" default:"200"`

	// GitHub App configuration
	GitHubAppID         string `envconfig:"GITHUB_APP_ID" default:""`
	GitHubAppSlug       string `envconfig:"GITHUB_APP_SLUG" default:""`
	GitHubAppPrivateKey string `envconfig:"GITHUB_APP_PRIVATE_KEY" default:""`
	GitHubClientID      string `envconfig:"GITHUB_CLIENT_ID" default:""`
	GitHubClientSecret  string `envconfig:"GITHUB_CLIENT_SECRET" default:""`
	GitHubWebhookURL    string `envconfig:"GITHUB_WEBHOOK_URL" default:""`
	GitHubWebhookSecret string `envconfig:"GITHUB_WEBHOOK_SECRET" default:""`

	// Event Bus configuration
	EventBus events.Config

	// OpenTelemetry configuration
	OTel observability.Config

	// FCMEnabled gates the Firebase Cloud Messaging web push channel.
	// When true, the Firebase Admin SDK uses Application Default Credentials
	// (Workload Identity on Cloud Run, gcloud user creds for local dev) — no
	// service account JSON key is ever required. When false, the WebPushChannel
	// is omitted from the channel list and notifications fall back to other channels.
	FCMEnabled bool `envconfig:"FCM_ENABLED" default:"false"`

	// MaxBodySizeBytes caps the size of request bodies the server will read for
	// general API routes. The maxBodySize middleware wraps r.Body with
	// http.MaxBytesReader using this value so a single oversized request cannot
	// exhaust memory. Defaults to 10MiB: this is a memory-exhaustion backstop, not
	// a functional limit, so it is set generously above the size of legitimate
	// payloads (artifact/memory/prompt content is unbounded TEXT and can be large).
	// Abuse-prone endpoints apply their own much tighter caps (contact form and
	// webhooks cap at 64KiB), so the global value only guards against absurd bodies.
	MaxBodySizeBytes int64 `envconfig:"MAX_BODY_SIZE_BYTES" default:"10485760"`

	// Per-IP rate limits (requests per minute), applied per route group by the
	// httprate middleware. middleware.RealIP runs first so the limiter keys on the
	// client IP. Each must be >= 1.
	//   - AuthRateLimitPerMinute: unauthenticated auth endpoints (login/callback/logout).
	//   - ContactRateLimitPerMinute: contact form (fans out to the email provider).
	//   - APIRateLimitPerMinute: the authenticated API surface (web app + CLI).
	//
	// Known limitations (intentional for this defense-in-depth pass; tracked for
	// follow-up): (1) the limiter keys on middleware.RealIP, which trusts the
	// leftmost X-Forwarded-For entry — an attacker who can forge that header gets a
	// fresh bucket per request, so this is a best-effort backstop against naive
	// abuse, not a spoof-proof control. (2) httprate uses an in-memory per-instance
	// counter, so on a multi-instance Cloud Run deployment the effective limit is
	// roughly N x the configured value and resets on cold start. A trusted-proxy
	// client-IP derivation and a shared (e.g. Redis-backed) counter are the
	// hardening follow-ups if real abuse is observed.
	AuthRateLimitPerMinute    int `envconfig:"AUTH_RATE_LIMIT_PER_MINUTE" default:"10"`
	ContactRateLimitPerMinute int `envconfig:"CONTACT_RATE_LIMIT_PER_MINUTE" default:"5"`
	APIRateLimitPerMinute     int `envconfig:"API_RATE_LIMIT_PER_MINUTE" default:"100"`

	// ActivityRetentionDays is the number of days to retain activity records.
	// Activities older than this value are deleted by the daily retention job.
	// Must be >= 1.
	ActivityRetentionDays int `envconfig:"ACTIVITY_RETENTION_DAYS" default:"90"`

	// AccessEventRetentionDays is the number of days to retain resource detail-access events.
	// Events older than this value are deleted by the daily retention job.
	// Must be >= 1.
	AccessEventRetentionDays int `envconfig:"ACCESS_EVENT_RETENTION_DAYS" default:"90"`

	// Search ranking configuration. When SearchRecencyRankingEnabled is false
	// (the default), search results keep the historical relevance-only ordering.
	// When true, the service re-ranks a candidate pool by a weighted blend of
	// semantic relevance and two freshness signals (created_at, updated_at), with
	// relevance dominant. The three weights are expected to satisfy
	// relevance >= created >= updated; they are normalized by their sum at
	// ranking time so they need not pre-sum to 1.
	SearchRecencyRankingEnabled bool    `envconfig:"SEARCH_RECENCY_RANKING_ENABLED" default:"false"`
	SearchRankWeightRelevance   float64 `envconfig:"SEARCH_RANK_WEIGHT_RELEVANCE" default:"0.5"`
	SearchRankWeightCreated     float64 `envconfig:"SEARCH_RANK_WEIGHT_CREATED" default:"0.3"`
	SearchRankWeightUpdated     float64 `envconfig:"SEARCH_RANK_WEIGHT_UPDATED" default:"0.2"`
	// SearchRankHalfLifeDays is the single shared half-life (in days) for the
	// exponential recency decay applied to both created_at and updated_at.
	SearchRankHalfLifeDays float64 `envconfig:"SEARCH_RANK_HALF_LIFE_DAYS" default:"90"`
	// SearchRankCandidateCap bounds how many top-by-distance candidates are pulled
	// for in-memory re-ranking before pagination. Re-ranking cannot use the HNSW
	// index, so this caps the work per query.
	SearchRankCandidateCap int `envconfig:"SEARCH_RANK_CANDIDATE_CAP" default:"200"`

	// A2A (Agent-to-Agent) configuration
	A2ADefaultTimeout time.Duration `envconfig:"A2A_DEFAULT_TIMEOUT" default:"5m"`

	// Deployment environment configuration
	OTelEnvironment       string `envconfig:"OTEL_ENVIRONMENT" default:""`
	Environment           string `envconfig:"ENVIRONMENT" default:""`
	Env                   string `envconfig:"ENV" default:""`
	DeploymentEnvironment string `envconfig:"DEPLOYMENT_ENVIRONMENT" default:""`
	KubernetesServiceHost string `envconfig:"KUBERNETES_SERVICE_HOST" default:""`
	GoogleCloudProject    string `envconfig:"GOOGLE_CLOUD_PROJECT" default:""`
	GCPProject            string `envconfig:"GCP_PROJECT" default:""`
	AWSRegion             string `envconfig:"AWS_REGION" default:""`
	AWSDefaultRegion      string `envconfig:"AWS_DEFAULT_REGION" default:""`

	// Cloud Run specific configuration (automatically set by Cloud Run)
	KService  string `envconfig:"K_SERVICE" default:""`
	KRevision string `envconfig:"K_REVISION" default:""`
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
	if c.GitHubAppID == "" || c.GitHubAppPrivateKey == "" {
		return nil, nil // No GitHub App configured
	}

	// Try to decode from base64 first (for easier .env management)
	// If decoding fails, treat it as raw PEM.
	// This automatic fallback allows keys to be stored in either format.
	privateKeyBytes := []byte(c.GitHubAppPrivateKey)
	isBase64Encoded := false
	if decoded, err := base64.StdEncoding.DecodeString(c.GitHubAppPrivateKey); err == nil {
		// Successfully decoded from base64
		privateKeyBytes = decoded
		isBase64Encoded = true
	}

	// Log which format was detected (visible during startup)
	if isBase64Encoded {
		fmt.Println("GitHub App private key loaded from base64-encoded format")
	} else {
		fmt.Println("GitHub App private key loaded from raw PEM format")
	}

	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GitHub App private key: %w", err)
	}

	return &GitHubAppConfig{
		AppID:         c.GitHubAppID,
		PrivateKey:    privateKey,
		PrivateKeyPEM: privateKeyBytes, // Store PEM bytes for ghinstallation
		WebhookSecret: c.GitHubWebhookSecret,
	}, nil
}

// GetDeploymentEnvironment determines the deployment environment from config
// It checks OTEL_ENVIRONMENT first, then standard env vars, then cloud indicators, then defaults
func (c *Config) GetDeploymentEnvironment() string {
	// Check OpenTelemetry standard env var
	if c.OTelEnvironment != "" {
		return c.OTelEnvironment
	}

	// Check common deployment environment variables
	if c.Environment != "" {
		return c.Environment
	}
	if c.Env != "" {
		return c.Env
	}
	if c.DeploymentEnvironment != "" {
		return c.DeploymentEnvironment
	}

	// Check common cloud provider environment indicators
	if c.KubernetesServiceHost != "" {
		return "kubernetes"
	}
	if c.GoogleCloudProject != "" || c.GCPProject != "" {
		return "gcp-cloud-run"
	}
	if c.AWSRegion != "" || c.AWSDefaultRegion != "" {
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
	weights := []float64{cfg.SearchRankWeightRelevance, cfg.SearchRankWeightCreated, cfg.SearchRankWeightUpdated}
	var sum float64
	for _, w := range weights {
		if w < 0 {
			return fmt.Errorf("search rank weights must be non-negative, got %v", weights)
		}
		sum += w
	}
	if sum == 0 {
		return fmt.Errorf("search rank weights must not all be zero")
	}
	if cfg.SearchRankHalfLifeDays <= 0 {
		return fmt.Errorf("SEARCH_RANK_HALF_LIFE_DAYS must be positive, got %v", cfg.SearchRankHalfLifeDays)
	}
	if cfg.SearchRankHalfLifeDays > maxSearchRankHalfLifeDays {
		return fmt.Errorf("SEARCH_RANK_HALF_LIFE_DAYS must be <= %d, got %v",
			maxSearchRankHalfLifeDays, cfg.SearchRankHalfLifeDays)
	}
	if cfg.SearchRankCandidateCap < 1 {
		return fmt.Errorf("SEARCH_RANK_CANDIDATE_CAP must be >= 1, got %d", cfg.SearchRankCandidateCap)
	}
	if cfg.SearchRankCandidateCap > maxSearchRankCandidateCap {
		return fmt.Errorf("SEARCH_RANK_CANDIDATE_CAP must be <= %d, got %d",
			maxSearchRankCandidateCap, cfg.SearchRankCandidateCap)
	}
	return nil
}

// encryptionKeyLength is the required AES-256 key length in bytes.
const encryptionKeyLength = 32

// validateEncryptionKey enforces that ENCRYPTION_KEY is present and exactly 32 bytes
// so the service fails closed at startup rather than running with a weak/default key.
func validateEncryptionKey(cfg *Config) error {
	if cfg.EncryptionKey == "" {
		return fmt.Errorf("ENCRYPTION_KEY is required and must be exactly %d bytes", encryptionKeyLength)
	}
	if len(cfg.EncryptionKey) != encryptionKeyLength {
		return fmt.Errorf("ENCRYPTION_KEY must be exactly %d bytes, got %d", encryptionKeyLength, len(cfg.EncryptionKey))
	}
	return nil
}

// validateRetentionDays enforces the shared 1..3650-day window (1 day to 10 years)
// for a retention setting. Rejecting 0/negatives prevents deleting all rows; the
// upper bound prevents silently violating the retention intent.
func validateRetentionDays(envName string, value int) error {
	if value < 1 || value > 3650 {
		return fmt.Errorf("%s must be between 1 and 3650, got %d", envName, value)
	}
	return nil
}

// validateEmbeddingConfig enforces that the embedding model is named and the
// chunker sizing is sane, so the service fails closed at startup rather than
// embedding/searching with an empty model_id or a stalled chunker.
func validateEmbeddingConfig(cfg *Config) error {
	if cfg.EmbeddingModel == "" {
		return fmt.Errorf("EMBEDDING_MODEL is required and must be non-empty")
	}
	if cfg.EmbeddingChunkSize < 1 {
		return fmt.Errorf("EMBEDDING_CHUNK_SIZE must be >= 1, got %d", cfg.EmbeddingChunkSize)
	}
	if cfg.EmbeddingChunkOverlap < 0 || cfg.EmbeddingChunkOverlap >= cfg.EmbeddingChunkSize {
		return fmt.Errorf(
			"EMBEDDING_CHUNK_OVERLAP must be in [0, EMBEDDING_CHUNK_SIZE), got %d (chunk size %d)",
			cfg.EmbeddingChunkOverlap, cfg.EmbeddingChunkSize,
		)
	}
	return nil
}

// validateRateLimits enforces that every per-IP rate limit is positive; a
// non-positive value would reject every request to the guarded route group.
func validateRateLimits(cfg *Config) error {
	if cfg.AuthRateLimitPerMinute < 1 {
		return fmt.Errorf("AUTH_RATE_LIMIT_PER_MINUTE must be >= 1, got %d", cfg.AuthRateLimitPerMinute)
	}
	if cfg.ContactRateLimitPerMinute < 1 {
		return fmt.Errorf("CONTACT_RATE_LIMIT_PER_MINUTE must be >= 1, got %d", cfg.ContactRateLimitPerMinute)
	}
	if cfg.APIRateLimitPerMinute < 1 {
		return fmt.Errorf("API_RATE_LIMIT_PER_MINUTE must be >= 1, got %d", cfg.APIRateLimitPerMinute)
	}
	return nil
}

func Load() (*Config, error) {
	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		return nil, err
	}

	if err = validateRetentionDays("ACTIVITY_RETENTION_DAYS", cfg.ActivityRetentionDays); err != nil {
		return nil, err
	}

	if err = validateRetentionDays("ACCESS_EVENT_RETENTION_DAYS", cfg.AccessEventRetentionDays); err != nil {
		return nil, err
	}

	// MaxBodySizeBytes must be positive; a zero/negative cap would reject every
	// request body once the maxBodySize middleware is applied.
	if cfg.MaxBodySizeBytes < 1 {
		return nil, fmt.Errorf("MAX_BODY_SIZE_BYTES must be >= 1, got %d", cfg.MaxBodySizeBytes)
	}

	if rateErr := validateRateLimits(&cfg); rateErr != nil {
		return nil, rateErr
	}

	if rankErr := validateSearchRankingConfig(&cfg); rankErr != nil {
		return nil, rankErr
	}

	if embErr := validateEmbeddingConfig(&cfg); embErr != nil {
		return nil, embErr
	}

	if keyErr := validateEncryptionKey(&cfg); keyErr != nil {
		return nil, keyErr
	}

	// Set default CORS allowed origins if not provided. Only localhost dev
	// origins are defaulted; production frontend origins must be supplied via
	// CORS_ALLOWED_ORIGINS so no tenant-specific domains are hardcoded.
	if len(cfg.CORSAllowedOrigins) == 0 {
		cfg.CORSAllowedOrigins = []string{
			"http://localhost:5173",
			"http://localhost:5174",
		}
	}

	// Propagate the configured error-type base URI to the errors package so
	// RFC 9457 "type" URIs are built from it. Done here (rather than at each
	// call site) to keep the package-level error constructors dependency-free.
	apierrors.SetTypeBaseURI(cfg.ErrorTypeBaseURI)

	// The application logger (level + format) is constructed from cfg.LogLevel /
	// cfg.LogFormat by configureLogger at startup; no global logger state is set
	// here.

	return &cfg, nil
}
