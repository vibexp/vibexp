package config

import (
	"crypto/rsa"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/observability"
	"github.com/vibexp/vibexp/pkg/events"
)

// Config is the fully-resolved, validated application configuration. It is
// loaded from a hierarchical config.yaml (see Load): code defaults are merged
// with the file, ${VAR} references in the file are interpolated against the
// process environment, and the result is unmarshalled into this nested struct.
// The koanf tags name each YAML section/key.
type Config struct {
	Server   ServerConfig   `koanf:"server"`
	Database DatabaseConfig `koanf:"database"`
	Security SecurityConfig `koanf:"security"`
	Auth     AuthConfig     `koanf:"auth"`
	MCP      MCPConfig      `koanf:"mcp"`
	// LegacyEmail is the deprecated `email:` section (#1190): imported once into
	// the instance_email_provider table at boot, then ignored. Removed in the
	// next minor (#1193).
	LegacyEmail LegacyEmailConfig `koanf:"email"`
	Frontend    FrontendConfig    `koanf:"frontend"`
	Search      SearchConfig      `koanf:"search"`
	AISummary   AISummaryConfig   `koanf:"ai_summary"`
	Storage     StorageConfig     `koanf:"storage"`
	GCP         GCPConfig         `koanf:"gcp"`
	RateLimit   RateLimitConfig   `koanf:"rate_limit"`
	Retention   RetentionConfig   `koanf:"retention"`
	Scheduler   SchedulerConfig   `koanf:"scheduler"`
	Embedding   EmbeddingConfig   `koanf:"embedding"`
	A2A         A2AConfig         `koanf:"a2a"`
	Deployment  DeploymentConfig  `koanf:"deployment"`

	// EventBus holds in-memory event-bus tuning (see pkg/events).
	EventBus events.Config `koanf:"event_bus"`
	// OTel holds OpenTelemetry export configuration (see internal/observability).
	OTel observability.Config `koanf:"otel"`

	// DeprecationWarnings lists the deprecated keys the loaded config.yaml
	// still sets, one operator-facing message each (see removedConfigKeys). The
	// loader has no logger, so the server logs them at boot. Never read from the
	// file.
	DeprecationWarnings []string `koanf:"-"`
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

	// TrustedProxies lists CIDRs whose requests may assert a client IP via
	// X-Forwarded-For / X-Real-IP. EMPTY BY DEFAULT (fail closed): with no entry,
	// those headers are ignored entirely and the peer address is the client IP.
	//
	// This is the switch that makes per-IP rate limiting real. VibeXP is
	// deploy-anywhere, so an instance may be directly reachable; trusting the
	// headers unconditionally let any client rotate X-Forwarded-For and bypass
	// the limiter completely (#465). Set this to your reverse proxy's CIDR(s)
	// when you run behind one, or per-client limits collapse into a single
	// bucket keyed on the proxy.
	// Declared EnvStringSlice (not []string) so the combined image can supply it
	// as a comma-separated ${TRUSTED_PROXIES} placeholder, like instance_admins.
	TrustedProxies EnvStringSlice `koanf:"trusted_proxies"`

	// Pprof configures the opt-in profiling listener (#1277).
	Pprof PprofConfig `koanf:"pprof"`
}

// DefaultPprofListenAddr is the loopback address the profiling listener binds
// when server.pprof.listen_addr is not set.
const DefaultPprofListenAddr = "127.0.0.1:6060"

// PprofConfig holds the opt-in net/http/pprof listener (#1277). Profiles expose
// heap contents and can be used to load the process, and the listener has no
// authentication: it is protected by where it binds. So it is off by default,
// served on its own port (never the API port), and bound to loopback unless an
// operator says otherwise.
type PprofConfig struct {
	// Enabled starts the profiling listener. EnvBool so the combined image can
	// expose it as ${PPROF_ENABLED}.
	Enabled EnvBool `koanf:"enabled"`
	// ListenAddr is the host:port the listener binds. Defaults to
	// 127.0.0.1:6060. Inside a container, loopback is only reachable with
	// `docker exec`; bind 0.0.0.0:6060 there and publish the port to the HOST's
	// loopback only (-p 127.0.0.1:6060:6060).
	ListenAddr string `koanf:"listen_addr"`
}

// validatePprofConfig rejects, when the listener is enabled, a
// server.pprof.listen_addr that is not host:port with a numeric port (0-65535),
// so a malformed address fails at load rather than leaving an operator with a
// profiling endpoint that silently never came up. The host is not resolved
// here; an address that is well-formed but cannot be bound is reported at boot.
func validatePprofConfig(cfg *Config) error {
	if !cfg.Server.Pprof.Enabled {
		return nil
	}
	addr := cfg.Server.Pprof.ListenAddr
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("server.pprof.listen_addr %q must be host:port: %w", addr, err)
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return fmt.Errorf("server.pprof.listen_addr %q must end in a numeric port (0-65535): %w", addr, err)
	}
	return nil
}

// ParsedTrustedProxies returns Server.TrustedProxies as parsed CIDRs. Entries
// are validated at load time, so a parse failure here cannot happen for a
// config that booted; malformed entries are skipped defensively rather than
// panicking in the request path.
func (c ServerConfig) ParsedTrustedProxies() []*net.IPNet {
	if len(c.TrustedProxies) == 0 {
		return nil
	}
	nets := make([]*net.IPNet, 0, len(c.TrustedProxies))
	for _, entry := range c.TrustedProxies {
		_, network, err := net.ParseCIDR(strings.TrimSpace(entry))
		if err != nil {
			continue
		}
		nets = append(nets, network)
	}
	return nets
}

// validateTrustedProxies fails startup on a malformed CIDR, consistent with the
// rest of the config's fail-fast validation. A typo here would otherwise be
// silently ignored and quietly leave the deployment keying every request on its
// proxy's address.
func validateTrustedProxies(entries EnvStringSlice) error {
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(trimmed); err != nil {
			return fmt.Errorf(
				"server.trusted_proxies: %q is not a valid CIDR (use e.g. 10.0.0.0/8 or 192.168.1.5/32)", trimmed)
		}
	}
	return nil
}

// DatabaseConfig holds PostgreSQL connection settings. Host may be a Unix
// socket path (Cloud SQL) when it begins with '/'.
type DatabaseConfig struct {
	Host     string `koanf:"host"`
	Port     string `koanf:"port"`
	User     string `koanf:"user"`
	Password string `koanf:"password"`
	Name     string `koanf:"name"`
	// SSLMode is the libpq TLS mode for the connection. Only "disable" (default,
	// no TLS) and "require" (encrypt without server-certificate verification) are
	// supported; validated by validateDatabaseSSLMode. Managed Postgres offerings
	// commonly require TLS, so "require" unblocks them.
	SSLMode string `koanf:"sslmode"`
}

// SecurityConfig holds process-wide secrets, admin keys, and the outbound
// (SSRF) network policy.
type SecurityConfig struct {
	// EncryptionKey encrypts sensitive data (API keys, OAuth-AS signing keys).
	// Required; must be exactly 32 bytes for AES-256 (see validateEncryptionKey).
	EncryptionKey string `koanf:"encryption_key"`
	// APIKeyCommon is the global API key for the common API surface.
	APIKeyCommon string `koanf:"api_key_common"`
	// BackofficeAdminAPIKey grants super-admin access to back-office endpoints.
	BackofficeAdminAPIKey string `koanf:"backoffice_admin_api_key"`

	// OutboundAllowedCIDRs lists networks the SSRF guard may dial even though
	// they are loopback / private / IPv6-unique-local. EMPTY BY DEFAULT (fail
	// closed): with no entry every reserved range is refused, which is the #464
	// posture.
	//
	// This is what makes a self-hosted embedding/model sidecar usable. A TEI or
	// Ollama container on a private Docker network resolves to an RFC1918
	// address, and the guard refuses the dial with "connection to disallowed
	// address range blocked" — killing semantic search and every background
	// embedding on the deployment (#745). Local development never hit this:
	// IsLocalDevelopment() already permits reserved ranges, so only real
	// deployments need this knob. Declaring the sidecar's subnet (e.g.
	// 172.16.0.0/12) or the loopback host reopens exactly that range and
	// nothing else.
	//
	// It can NEVER unblock link-local (169.254.0.0/16 — cloud metadata — and
	// fe80::/10) or multicast: validateOutboundAllowedCIDRs rejects an
	// overlapping entry at startup, and services.ssrfGuard blocks those ranges
	// before it ever consults this list.
	//
	// Declared EnvStringSlice (not []string) so the combined image can supply it
	// as a comma-separated ${OUTBOUND_ALLOWED_CIDRS} placeholder, like
	// server.trusted_proxies.
	OutboundAllowedCIDRs EnvStringSlice `koanf:"outbound_allowed_cidrs"`
}

// neverAllowlistableCIDRs are the ranges security.outbound_allowed_cidrs must
// never cover: cloud-metadata/link-local and multicast. An operator declaring a
// prefix that overlaps one of these is rejected at startup rather than silently
// narrowed, so a single careless entry (including the "disable the guard"
// 0.0.0.0/0, which overlaps 169.254.0.0/16) cannot reopen the SSRF hole #464
// closed.
var neverAllowlistableCIDRs = []string{
	"169.254.0.0/16", // IPv4 link-local, incl. the 169.254.169.254 metadata IP
	"224.0.0.0/4",    // IPv4 multicast
	"fe80::/10",      // IPv6 link-local
	"ff00::/8",       // IPv6 multicast
}

// ParsedOutboundAllowedCIDRs returns Security.OutboundAllowedCIDRs as parsed
// CIDRs. Entries are validated at load time, so a parse failure here cannot
// happen for a config that booted; malformed entries are skipped defensively
// rather than panicking in the request path (mirrors ParsedTrustedProxies).
func (c SecurityConfig) ParsedOutboundAllowedCIDRs() []*net.IPNet {
	if len(c.OutboundAllowedCIDRs) == 0 {
		return nil
	}
	nets := make([]*net.IPNet, 0, len(c.OutboundAllowedCIDRs))
	for _, entry := range c.OutboundAllowedCIDRs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(entry))
		if err != nil {
			continue
		}
		nets = append(nets, network)
	}
	return nets
}

// validateOutboundAllowedCIDRs fails startup on a malformed CIDR or on one that
// overlaps a never-allowlistable range. Fail-fast matters more here than for
// trusted_proxies: a typo does not merely get ignored, it is the difference
// between "my sidecar is reachable" and "an authenticated member can reach the
// cloud metadata service".
func validateOutboundAllowedCIDRs(entries EnvStringSlice) error {
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		_, network, err := net.ParseCIDR(trimmed)
		if err != nil {
			return fmt.Errorf(
				"security.outbound_allowed_cidrs: %q is not a valid CIDR (use e.g. 172.16.0.0/12 or 127.0.0.1/32)",
				trimmed)
		}
		if blocked := overlappingNeverAllowlistable(network); blocked != "" {
			return fmt.Errorf(
				"security.outbound_allowed_cidrs: %q overlaps %s, which can never be allowlisted "+
					"(link-local/cloud-metadata and multicast stay blocked); declare only the network your "+
					"own service runs on", trimmed, blocked)
		}
	}
	return nil
}

// overlappingNeverAllowlistable returns the first never-allowlistable range that
// intersects network, or "" when there is none. Two prefixes intersect exactly
// when one contains the other's base address, so checking both directions also
// catches a declared prefix wide enough to swallow a blocked one (0.0.0.0/0).
func overlappingNeverAllowlistable(network *net.IPNet) string {
	for _, raw := range neverAllowlistableCIDRs {
		_, blocked, err := net.ParseCIDR(raw)
		if err != nil {
			continue
		}
		if network.Contains(blocked.IP) || blocked.Contains(network.IP) {
			return raw
		}
	}
	return ""
}

// AuthConfig holds web-login identity-provider settings and the embedded
// OAuth 2.1 Authorization Server configuration.
type AuthConfig struct {
	// LegacyProviders is the comma-separated (or YAML list) set of web-login
	// identity providers to enable simultaneously (e.g. "google,github,oidc").
	// When set it takes precedence over LegacyProvider. Unknown names are ignored
	// with a warning; providers with missing credentials are skipped at startup.
	// Matched case-insensitively against "google", "github", and "oidc".
	//
	// DEPRECATED, like every Legacy* field of AuthConfig, and deliberately not in
	// the Go "Deprecated:" form (staticcheck SA1019 would then flag the bridge's
	// own reads): sign-in providers and the access allowlist are stored in the
	// database and configured under Admin → Settings → Authentication (epic
	// #1230). At boot these keys are imported once into the database when it
	// holds none (services.ImportLegacyAuthConfig, #1232) and a deprecation
	// warning is logged on every boot while they are set
	// (Config.DeprecationWarnings). They become a boot failure no earlier than the
	// second minor release after the one that deprecated them (#1240). Until the
	// runtime reads the database (#1234, #1235) they are still what login uses.
	LegacyProviders []string `koanf:"providers"`
	// LegacyProvider selects a single web-login provider; the
	// backward-compatible shim used only when LegacyProviders is empty.
	// DEPRECATED, see LegacyProviders.
	LegacyProvider string `koanf:"provider"`

	// SessionEncryptionKey is the hex-encoded secret backing the AES-256-GCM
	// session cookie (and, via domain separation, the OAuth state HMAC). It must
	// decode to exactly 32 bytes (64 hex chars). When empty, cookie session auth
	// is disabled (stub/test mode).
	SessionEncryptionKey string `koanf:"session_encryption_key"`

	// DevLoginEnabled gates the /api/v1/auth/dev/login endpoint. It must be
	// explicitly true AND the environment detected as development
	// (frontend.base_url points at localhost) for the endpoint to respond.
	DevLoginEnabled bool `koanf:"dev_login_enabled"`

	// RecoveryMode forces authentication setup mode on at boot regardless of the
	// stored identity providers (#1237): every boot logs a fresh setup URL and a
	// WARN naming the flag. Providers that are configured keep signing users in;
	// the flag only guarantees a way back into the sign-in settings after a
	// lockout. Remove it once sign-in works again. It is EnvBool so the combined
	// image can set it with AUTH_SETTINGS_RECOVERY_MODE alone.
	RecoveryMode EnvBool `koanf:"recovery_mode"`

	// LegacyAccessAllowlist is the config.yaml access allowlist: email domains
	// and/or exact addresses. It is no longer enforced from here. It is imported
	// once at boot into the instance_auth_allowlist table (#1232), and the
	// database-stored allowlist is what sign-in and every authenticated request
	// are checked against (#1235, services.AccessAllowlistResolver). See
	// AccessAllowlistConfig. DEPRECATED, see LegacyProviders.
	LegacyAccessAllowlist AccessAllowlistConfig `koanf:"access_allowlist"`

	// InstanceAdmins is the set of ROOT instance-admin email addresses (authored
	// as a comma-separated ${VAR} in the combined image, or a YAML list). Root
	// admins may access instance-level admin surfaces, are the only ones who may
	// grant or revoke DB-granted instance admins (#1233), and cannot be
	// suspended, deleted or revoked. They are also exempt from the access
	// allowlist (#1235), so an allowlist mistake cannot lock out the trust
	// root. Resolved at request time by
	// services.InstanceAdminResolver (case-insensitive, whitespace-trimmed,
	// blank entries ignored). Empty (the zero value) means no root admin, so no
	// one can grant a DB admin either. Follows the same EnvStringSlice pattern
	// as AccessAllowlist.
	InstanceAdmins EnvStringSlice `koanf:"instance_admins"`

	// LegacyGoogle, LegacyGitHub and LegacyOIDC are the web-login clients of
	// the three provider kinds. DEPRECATED, see LegacyProviders.
	LegacyGoogle GoogleAuthConfig `koanf:"google"`
	LegacyGitHub GitHubAuthConfig `koanf:"github"`
	LegacyOIDC   OIDCAuthConfig   `koanf:"oidc"`

	OAuthAS OAuthASConfig  `koanf:"oauth_as"`
	APIAuth APIOAuthConfig `koanf:"api_oauth"`
}

// LegacyEnabledProviderNames is the ordered, de-duplicated list of web-login
// providers the deprecated keys enable: auth.providers when it has any entry,
// else auth.provider. Names are trimmed and lower-cased; blanks and "none" are
// dropped. Unknown names are kept for the caller to report. The identity
// provider registry and the boot-time import (#1232) both read it, so they can
// never disagree about which providers are enabled.
func (a AuthConfig) LegacyEnabledProviderNames() []string {
	var raw []string
	switch {
	case len(a.LegacyProviders) > 0:
		raw = a.LegacyProviders
	case strings.TrimSpace(a.LegacyProvider) != "":
		raw = []string{a.LegacyProvider}
	}

	seen := make(map[string]struct{}, len(raw))
	names := make([]string, 0, len(raw))
	for _, r := range raw {
		name := strings.ToLower(strings.TrimSpace(r))
		if name == "" || name == "none" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

// EnvStringSlice is a []string that, in the combined-image config.docker.yaml,
// may be authored as a single comma-separated ${VAR:-default} placeholder rather
// than a YAML list — koanf's StringToSliceHookFunc(",") splits it at load. The
// generated schema therefore accepts either a string array or such a placeholder
// (see cmd/gen-config-schema, mirroring the time.Duration mapper). It is
// assignable to and from a plain []string.
type EnvStringSlice []string

// EnvBool is a bool that, in the combined-image config.docker.yaml, may be
// authored as a ${VAR:-true} placeholder. Decoding already works for a plain
// bool — the loader runs with WeaklyTypedInput, so the interpolated "true" /
// "false" string coerces — so this type exists purely so gen-config-schema can
// emit a placeholder-tolerant schema for it (TestConfigDockerYAML_MatchesSchema
// validates the RAW, pre-interpolation YAML, where a placeholder is a string).
// Its underlying kind is bool, so it is assignable to and from a plain bool and
// usable directly in a boolean expression.
//
// Declare a field EnvBool only when it is an operator knob exposed in
// config.docker.yaml; every other bool keeps the strict boolean schema, which is
// what makes editor validation catch a typo'd value.
type EnvBool bool

// EnvInt is the integer counterpart of EnvBool: an int an operator may supply as
// a ${VAR:-100} placeholder in config.docker.yaml. Same rationale, same rule for
// when to use it — the schema, not the decoder, is what needs the named type.
type EnvInt int

// stringToEnvStringSliceHookFunc splits a comma-separated string into an
// EnvStringSlice. mapstructure's built-in StringToSliceHookFunc only fires for
// the exact type []string (it checks `t == reflect.SliceOf(f)`), so the defined
// EnvStringSlice type needs its own hook to get the same comma-split behavior as
// a plain []string. A YAML list is already a slice and passes through untouched.
func stringToEnvStringSliceHookFunc(sep string) mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if f.Kind() != reflect.String || t != reflect.TypeFor[EnvStringSlice]() {
			return data, nil
		}
		raw, ok := data.(string)
		if !ok || raw == "" {
			return EnvStringSlice{}, nil
		}
		return EnvStringSlice(strings.Split(raw, sep)), nil
	}
}

// AccessAllowlistConfig is the shape of the legacy config.yaml access allowlist
// (see AuthConfig.LegacyAccessAllowlist; it is imported into the database, not
// enforced from config). Domains matches the part after the
// last "@" of a user's email exactly (case-insensitively); Emails matches the
// full address exactly. A user is allowed if either list matches. When BOTH
// lists are empty (the zero value) access is open — every user may sign in.
type AccessAllowlistConfig struct {
	Domains EnvStringSlice `koanf:"domains"`
	Emails  EnvStringSlice `koanf:"emails"`
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
//
// When the embedded Authorization Server is enabled and Issuer is left empty,
// applyAPIOAuthDefaults auto-wires this to the AS out-of-the-box (Issuer = the
// AS issuer, Audiences = the resource the AS binds tokens to) so a native-CLI
// browser-login token is honored on /api/v1 with no manual config. An explicit
// Issuer/Audiences (external IdP) always wins.
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
	// SessionTimeout is how long an MCP session may sit idle before the server
	// closes it and frees its memory. A client that exits without sending DELETE
	// never ends its session itself, so without this every abandoned session is
	// held for the life of the process (#1275).
	SessionTimeout time.Duration `koanf:"session_timeout"`
}

// LegacyEmailConfig is the deprecated `email:` section: the selected provider,
// shared sender/recipient addresses, and per-provider sub-structs.
//
// DEPRECATED, deliberately not in the Go "Deprecated:" form (staticcheck
// SA1019 would then flag the bridge's own reads): the instance's mail provider
// is stored in the database and configured under Admin → Settings → Email
// (#1188). This block is still loaded
// for one release so an upgraded install keeps sending: at boot it is imported
// once into the instance_email_provider table when no row exists, and ignored
// afterwards (services.ImportLegacyEmailConfig, #1190). It is removed in the
// next minor (#1193). Nothing on the send path reads it.
type LegacyEmailConfig struct {
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
	// GTMID is the Google Tag Manager container ID. Setting it IS the opt-in:
	// the SPA loads GTM only when it is non-empty. There is no separate enable
	// flag, and VibeXP ships no cookie-consent gate of its own (#740).
	GTMID            string `koanf:"gtm_id"`
	GA4MeasurementID string `koanf:"ga4_measurement_id"`
}

// SearchConfig holds the `search:` block of config.yaml.
//
// DEPRECATED (#1196): since #1198 no service reads it at runtime; the instance
// ranking defaults come from the instance_search_settings row, edited under
// Admin → Settings → Search. At boot a block that differs from the built-in
// defaults is imported once into that row when none is stored, and ignored
// afterwards (#1201). The block is removed in the next minor release (#1203).
// It is still validated at startup (validateSearchRankingConfig) until then.
type SearchConfig struct {
	RecencyRankingEnabled bool    `koanf:"recency_ranking_enabled"`
	RankWeightRelevance   float64 `koanf:"rank_weight_relevance"`
	RankWeightCreated     float64 `koanf:"rank_weight_created"`
	RankWeightUpdated     float64 `koanf:"rank_weight_updated"`
	RankHalfLifeDays      float64 `koanf:"rank_half_life_days"`
	RankCandidateCap      int     `koanf:"rank_candidate_cap"`
}

// InstanceValues projects the block onto the instance search settings values
// the boot-time import (#1201) compares with the built-in defaults and stores.
func (s SearchConfig) InstanceValues() models.InstanceSearchSettingsValues {
	return models.InstanceSearchSettingsValues{
		RecencyRankingEnabled: s.RecencyRankingEnabled,
		RankWeightRelevance:   s.RankWeightRelevance,
		RankWeightCreated:     s.RankWeightCreated,
		RankWeightUpdated:     s.RankWeightUpdated,
		RankHalfLifeDays:      s.RankHalfLifeDays,
		RankCandidateCap:      s.RankCandidateCap,
	}
}

// AISummaryConfig holds the `ai_summary:` block of config.yaml (#1071).
//
// DEPRECATED (#1196): since #1199 no service reads it at runtime; the instance
// defaults and budgets come from the instance_ai_summary_settings row
// (InstanceAISummarySettingsService, edited under Admin → Settings → AI
// Summary), or from models.DefaultInstanceAISummarySettings when none is
// stored. At boot a block that differs from those defaults is validated and
// imported once into the row when none is stored, and ignored afterwards
// (#1201); config does not validate it. The block is removed in the next minor
// release (#1203).
type AISummaryConfig struct {
	// Enabled switches the feature on for the whole instance. EnvBool so the
	// combined image can expose it as ${AI_SUMMARY_ENABLED}.
	Enabled EnvBool `koanf:"enabled"`
	// TopN is the default number of documents fed to the summariser. EnvInt for
	// the same reason as Enabled.
	TopN EnvInt `koanf:"top_n"`
	// MaxTopN bounds nothing since #1199 (a team's top_n is bounded by the hard
	// limit models.MaxAISummaryTopN alone) and is not imported (#1201): a
	// non-default value only logs that it is ignored. It still loads so a
	// leftover key does not fail boot.
	MaxTopN int `koanf:"max_top_n"`
	// PerDocumentChars truncates each document before it enters the prompt.
	PerDocumentChars int `koanf:"per_document_chars"`
	// TotalContextChars caps the assembled context across all documents. It is
	// the binding budget when TopN documents would together exceed it.
	TotalContextChars int `koanf:"total_context_chars"`
	// MaxOutputTokens is the default answer-length budget.
	MaxOutputTokens int `koanf:"max_output_tokens"`
	// MaxOutputTokensCeiling (#1085) is, like MaxTopN, ignored since #1199 (a
	// team's max_output_tokens is bounded by the hard limit
	// models.MaxAISummaryOutputTokens alone) and not imported.
	MaxOutputTokensCeiling int `koanf:"max_output_tokens_ceiling"`
	// RequestTimeout bounds a single summarisation call to the model provider.
	RequestTimeout time.Duration `koanf:"request_timeout"`
	// Style is the default summary style, one of models.AISummaryStyles.
	Style string `koanf:"style"`
}

// DefaultLegacyAISummaryOutputTokensCeiling is the defaults() value of the
// ignored ai_summary.max_output_tokens_ceiling key.
const DefaultLegacyAISummaryOutputTokensCeiling = 4096

// InstanceValues projects the block onto the instance AI summary settings
// values the boot-time import (#1201) compares with the built-in defaults and
// stores. The ignored ceilings are not part of it; see LegacyCeilingsSet.
func (a AISummaryConfig) InstanceValues() models.InstanceAISummarySettingsValues {
	return models.InstanceAISummarySettingsValues{
		Enabled:           bool(a.Enabled),
		TopN:              int(a.TopN),
		Style:             a.Style,
		MaxOutputTokens:   a.MaxOutputTokens,
		PerDocumentChars:  a.PerDocumentChars,
		TotalContextChars: a.TotalContextChars,
		RequestTimeout:    a.RequestTimeout,
	}
}

// LegacyCeilingsSet reports whether ai_summary.max_top_n or
// ai_summary.max_output_tokens_ceiling differs from its defaults() value. Both
// are ignored since #1199, so a non-default value is worth a warning.
func (a AISummaryConfig) LegacyCeilingsSet() bool {
	return a.MaxTopN != models.MaxAISummaryTopN ||
		a.MaxOutputTokensCeiling != DefaultLegacyAISummaryOutputTokensCeiling
}

// StorageConfig holds resource-attachment storage settings.
type StorageConfig struct {
	// Backend selects the object-store implementation: "gcs", "s3" (covers
	// MinIO via s3_endpoint + s3_path_style), or "filesystem". Empty preserves
	// the pre-selector behavior: GCS is used when AttachmentsBucket is set,
	// otherwise attachments are disabled (upload/download/delete return 503).
	Backend string `koanf:"backend"`
	// AttachmentsBucket is the bucket backing artifact (and future resource)
	// file attachments for the gcs and s3 backends. Empty with an empty Backend
	// disables attachments.
	AttachmentsBucket string `koanf:"attachments_bucket"`
	// S3Endpoint overrides the S3 API endpoint (e.g. a MinIO server URL). Empty
	// targets AWS S3 in S3Region.
	S3Endpoint string `koanf:"s3_endpoint"`
	// S3Region is the S3 region; required by the SDK even for MinIO (which
	// ignores its value).
	S3Region string `koanf:"s3_region"`
	// S3AccessKey / S3SecretKey are static S3 credentials. Reference them via
	// ${VAR} interpolation; never commit values. Empty falls back to the AWS
	// SDK default credential chain (env vars, shared config, IAM).
	S3AccessKey string `koanf:"s3_access_key"`
	S3SecretKey string `koanf:"s3_secret_key"`
	// S3PathStyle forces path-style addressing (endpoint/bucket/key), required
	// by MinIO and most self-hosted S3-compatible stores. EnvBool so the
	// combined image can expose it as ${S3_PATH_STYLE} (#760).
	S3PathStyle EnvBool `koanf:"s3_path_style"`
	// FSRootDir is the root directory the filesystem backend stores objects
	// under. Required when Backend is "filesystem".
	FSRootDir string `koanf:"fs_root_dir"`
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

// SchedulerConfig holds the in-process scheduler engine's knobs (epic #725).
type SchedulerConfig struct {
	// Enabled turns the run loop on. When false nothing is claimed or run.
	// EnvBool so the combined image can expose it as ${SCHEDULER_ENABLED}.
	Enabled EnvBool `koanf:"enabled"`
	// TickInterval is how often the loop looks for due schedules. The 1-hour
	// job floor keeps work sparse, so this is a polling cadence, not a job
	// cadence — minutes, not seconds.
	TickInterval time.Duration `koanf:"tick_interval"`
	// JobTimeout bounds a single handler invocation.
	JobTimeout time.Duration `koanf:"job_timeout"`
	// DueLimit caps how many due schedules one tick claims.
	// EnvInt so the combined image can expose it as ${SCHEDULER_DUE_LIMIT}.
	DueLimit EnvInt `koanf:"due_limit"`
}

// EmbeddingConfig holds the embedding pipeline's knobs.
type EmbeddingConfig struct {
	// Queue tunes the durable embedding job queue (issue #820).
	Queue EmbeddingQueueConfig `koanf:"queue"`
}

// EmbeddingQueueConfig tunes the durable, leased embedding job queue that backs
// the dispatcher (issue #820). Durability itself is NOT a knob: the queue is
// always the system of record, because a queue whose enqueue is optional is not
// a durable queue. These knobs only tune how it is drained.
type EmbeddingQueueConfig struct {
	// LeaseDuration is how long a claimed job is held before another worker may
	// reclaim it. It must comfortably exceed the worst-case time one job spends
	// in a worker (the wait for a free provider slot plus the 2-minute per-job
	// timeout): a lease that expires under a job still running costs a duplicate
	// embed, which is wasted work rather than wrong data.
	LeaseDuration time.Duration `koanf:"lease_duration"`
	// MaxAttempts bounds how many times one job may be CLAIMED before it is
	// retired as a poison pill. Counting claims rather than failures is what
	// bounds a job whose worker dies before it can record a failure.
	// EnvInt so the combined image can expose it as ${EMBEDDING_QUEUE_MAX_ATTEMPTS}.
	MaxAttempts EnvInt `koanf:"max_attempts"`
	// BatchSize caps how many jobs one claim leases.
	// EnvInt so the combined image can expose it as ${EMBEDDING_QUEUE_BATCH_SIZE}.
	BatchSize EnvInt `koanf:"batch_size"`
	// PollInterval is how often the queue is swept for work no enqueue
	// announced: jobs orphaned by a dead process, and jobs backing off after a
	// retryable failure. An enqueue wakes the poller directly, so this is not
	// the latency of the common path.
	PollInterval time.Duration `koanf:"poll_interval"`
	// RetryBackoff holds a job back after a retryable failure, so a job that
	// fails fast does not consume a claim slot on every poll.
	RetryBackoff time.Duration `koanf:"retry_backoff"`
}

// A2AConfig holds Agent-to-Agent client settings.
type A2AConfig struct {
	// DefaultTimeout bounds a single synchronous (message/send) request.
	DefaultTimeout time.Duration `koanf:"default_timeout"`
	// StreamTimeout bounds the total lifetime of a streaming (message/stream)
	// SSE connection. It is deliberately decoupled from DefaultTimeout so a
	// long-running streaming agent is not force-closed by the sync timeout.
	StreamTimeout time.Duration `koanf:"stream_timeout"`
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

// GitHubAppConfig holds one GitHub App's parsed credentials.
//
// Despite living in this package it is no longer read from config.yaml: the
// instance-wide `github:` section was removed in #483, and every value here now
// comes from a team's own github_app_configs row, decrypted by
// services.GitHubAppClientResolver. It stays here only because that is the type
// external/implementations.NewGitHubAppClient already takes; relocating it to a
// credentials-shaped home is a separate, purely mechanical move.
type GitHubAppConfig struct {
	AppID         string
	PrivateKey    *rsa.PrivateKey
	PrivateKeyPEM []byte // PEM-encoded private key for ghinstallation
	WebhookSecret string
	// ClientID / ClientSecret back the user-authorization leg of the install
	// flow (#463). Empty means user authorization is not configured, and the
	// client rejects installation callbacks rather than trusting them.
	ClientID     string
	ClientSecret string
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

// validateSearchRankingConfig rejects degenerate ranking parameters so a
// misconfigured deployment fails fast at startup rather than silently producing
// garbage ordering. Weights must be non-negative (and not all zero); the
// half-life and candidate cap must each be positive and within a sane ceiling
// (models.MaxSearchRankHalfLifeDays, models.MaxSearchRankCandidateCap).
//
// services.ValidateInstanceSearchSettings enforces the same bounds on the
// database-stored instance defaults; this copy stays until the `search:` block
// is removed from config (#1203).
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
	if s.RankHalfLifeDays > models.MaxSearchRankHalfLifeDays {
		return fmt.Errorf("search.rank_half_life_days must be <= %d, got %v",
			models.MaxSearchRankHalfLifeDays, s.RankHalfLifeDays)
	}
	if s.RankCandidateCap < 1 {
		return fmt.Errorf("search.rank_candidate_cap must be >= 1, got %d", s.RankCandidateCap)
	}
	if s.RankCandidateCap > models.MaxSearchRankCandidateCap {
		return fmt.Errorf("search.rank_candidate_cap must be <= %d, got %d",
			models.MaxSearchRankCandidateCap, s.RankCandidateCap)
	}
	return nil
}

// supportedStorageBackends is the set of storage.backend selector values
// VibeXP accepts. The empty string is valid and means "infer": GCS when
// attachments_bucket is set, disabled otherwise (the pre-selector behavior).
var supportedStorageBackends = map[string]bool{
	"":           true,
	"gcs":        true,
	"s3":         true,
	"filesystem": true,
}

// validateStorageConfig fails closed on an unknown storage.backend selector (a
// typo would otherwise silently disable attachments) and on a selected backend
// missing its required knob, so a misconfigured deployment is caught at startup
// rather than surfacing as 503s at upload time.
func validateStorageConfig(cfg *Config) error {
	s := cfg.Storage
	if !supportedStorageBackends[s.Backend] {
		return fmt.Errorf(
			"storage.backend must be one of \"gcs\", \"s3\", or \"filesystem\", got %q",
			s.Backend,
		)
	}
	switch s.Backend {
	case "gcs", "s3":
		if s.AttachmentsBucket == "" {
			return fmt.Errorf("storage.attachments_bucket is required when storage.backend is %q", s.Backend)
		}
		if s.Backend == "s3" {
			// Region is required by the SDK even for MinIO; without it NewS3Store
			// errors and the provider degrades to 503s at upload time.
			if s.S3Region == "" {
				return fmt.Errorf("storage.s3_region is required when storage.backend is \"s3\"")
			}
			// Static credentials are both-or-neither: installing a provider with
			// only one fails every request at upload time with an opaque
			// SDK error that never names the config knob.
			if (s.S3AccessKey == "") != (s.S3SecretKey == "") {
				return fmt.Errorf("storage.s3_access_key and storage.s3_secret_key must be set together " +
					"(or both empty to use the AWS default credential chain)")
			}
		}
	case "filesystem":
		if s.FSRootDir == "" {
			return fmt.Errorf("storage.fs_root_dir is required when storage.backend is \"filesystem\"")
		}
	}
	return nil
}

// encryptionKeyLength is the required AES-256 key length in bytes.
const encryptionKeyLength = 32

// supportedDatabaseSSLModes is the set of libpq sslmode values VibeXP accepts.
// Scope is deliberately limited to "disable" (no TLS) and "require" (encrypt
// without server-cert verification); verify-ca/verify-full (which also need a
// root-cert path) are a possible future extension (#293).
var supportedDatabaseSSLModes = map[string]bool{
	"disable": true,
	"require": true,
}

// validateDatabaseSSLMode fails closed on an unsupported database.sslmode so a
// typo (or an unsupported libpq mode) is caught at startup rather than surfacing
// as an opaque connection error.
func validateDatabaseSSLMode(cfg *Config) error {
	if !supportedDatabaseSSLModes[cfg.Database.SSLMode] {
		return fmt.Errorf(
			"database.sslmode must be one of \"disable\" or \"require\", got %q",
			cfg.Database.SSLMode,
		)
	}
	return nil
}

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

// looksLikeEmail is a deliberately permissive shape check (NOT full RFC 5322
// validation) used to catch obviously malformed auth.instance_admins entries at
// startup while accepting any real-world address. It requires a single non-empty
// local part, an "@", and a dotted domain, with no internal whitespace.
func looksLikeEmail(s string) bool {
	at := strings.LastIndex(s, "@")
	if at <= 0 || at == len(s)-1 {
		return false
	}
	local, domain := s[:at], s[at+1:]
	if strings.ContainsAny(local, " \t") || strings.ContainsAny(domain, " \t") {
		return false
	}
	// The domain must contain a dot that is neither the first nor the last char.
	dot := strings.IndexByte(domain, '.')
	return dot > 0 && dot < len(domain)-1
}

// validateInstanceAdmins fails startup when auth.instance_admins contains a
// malformed email, so a self-hoster learns of a typo immediately rather than
// silently granting no one admin. An empty list is valid (the feature is
// dormant), and blank entries (e.g. from a trailing comma in the env value) are
// tolerated and ignored — mirroring how the sign-in allowlist drops blanks.
func validateInstanceAdmins(cfg *Config) error {
	for _, entry := range cfg.Auth.InstanceAdmins {
		e := strings.TrimSpace(entry)
		if e == "" {
			continue
		}
		if !looksLikeEmail(e) {
			return fmt.Errorf("auth.instance_admins entry %q is not a valid email address", entry)
		}
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

// authCallbackPath is the fixed API path every identity provider redirects back
// to after sign-in. The provider is recovered from the signed state cookie, not
// from the path.
const authCallbackPath = "/api/v1/auth/callback"

// AuthCallbackURL returns the OAuth redirect URI every sign-in identity provider
// is registered with (#1234): it is derived, not stored per provider.
//
// The combined image serves the SPA and the API from one origin, so in
// production it is <frontend.base_url>/api/v1/auth/callback. In local
// development (IsLocalDevelopment) frontend.base_url is the Vite dev server,
// which does not proxy /api, so the callback goes to the backend itself on
// http://localhost:<server.port>, as applyDevOAuthASDefaults does for the AS.
func (c *Config) AuthCallbackURL() string {
	if c.IsLocalDevelopment() {
		return "http://localhost:" + c.Server.Port + authCallbackPath
	}
	return strings.TrimRight(c.Frontend.BaseURL, "/") + authCallbackPath
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

// applyAPIOAuthDefaults makes /api/v1 accept the embedded Authorization Server's
// bearer JWTs out-of-the-box. When the AS is enabled (auth.oauth_as.issuer_url
// set) and no explicit auth.api_oauth.issuer is configured, it trusts the AS as
// the API-surface issuer and pins the audience to mcp.resource_uri — the RFC
// 8707 resource the AS binds every token to. Without this the default API
// audience policy (AllowAnyAudienceExcept(mcp.resource_uri)) rejects exactly the
// token `vibexp auth login` obtains, so a self-hosted native CLI login dead-ends
// on REST unless the operator manually mirrors this wiring.
//
// An explicit api_oauth.issuer (external IdP) always wins and is left untouched;
// likewise a caller-set api_oauth.audiences is never overwritten. Must run after
// applyMCPIssuerDefault so cfg.MCP.ResourceURI is already resolved.
func applyAPIOAuthDefaults(cfg *Config) {
	if cfg.Auth.OAuthAS.IssuerURL == "" || cfg.Auth.APIAuth.Issuer != "" {
		return
	}
	cfg.Auth.APIAuth.Issuer = cfg.Auth.OAuthAS.IssuerURL
	if len(cfg.Auth.APIAuth.Audiences) == 0 && cfg.MCP.ResourceURI != "" {
		cfg.Auth.APIAuth.Audiences = []string{cfg.MCP.ResourceURI}
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

// validateSchedulerConfig enforces that the scheduler's loop knobs are usable.
// tick_interval is the load-bearing one: time.NewTicker panics on a non-positive
// duration, so a mounted config with `tick_interval: "0s"` would crash the
// process at startup instead of reporting a configuration error. job_timeout and
// due_limit are checked in the same place so the whole section fails closed —
// a due_limit of 0 would silently claim nothing every tick.
func validateSchedulerConfig(cfg *Config) error {
	if cfg.Scheduler.TickInterval <= 0 {
		return fmt.Errorf("scheduler.tick_interval must be positive, got %v", cfg.Scheduler.TickInterval)
	}
	if cfg.Scheduler.JobTimeout <= 0 {
		return fmt.Errorf("scheduler.job_timeout must be positive, got %v", cfg.Scheduler.JobTimeout)
	}
	if cfg.Scheduler.DueLimit < 1 {
		return fmt.Errorf("scheduler.due_limit must be >= 1, got %d", cfg.Scheduler.DueLimit)
	}
	return nil
}

// validateMCPConfig enforces that mcp.session_timeout is positive. The MCP SDK
// reads a zero timeout as "never close an idle session", which is the unbounded
// memory growth of #1275, so a mounted config with `session_timeout: "0s"` must
// fail at startup rather than silently turn eviction off.
func validateMCPConfig(cfg *Config) error {
	if cfg.MCP.SessionTimeout <= 0 {
		return fmt.Errorf("mcp.session_timeout must be positive, got %v", cfg.MCP.SessionTimeout)
	}
	return nil
}

// validateEmbeddingQueueConfig enforces that the durable embedding queue's
// drain knobs are usable. poll_interval is the load-bearing one for the same
// reason scheduler.tick_interval is: it feeds time.NewTicker, which panics on a
// non-positive duration, so a mounted config with `poll_interval: "0s"` would
// crash the process at startup rather than report a configuration error. The
// rest fail closed together -- a lease_duration of 0 makes every claim instantly
// reclaimable (so every job runs repeatedly), a batch_size of 0 claims nothing,
// and a max_attempts of 0 retires every job as a poison pill on its first claim.
func validateEmbeddingQueueConfig(cfg *Config) error {
	q := cfg.Embedding.Queue
	if q.LeaseDuration <= 0 {
		return fmt.Errorf("embedding.queue.lease_duration must be positive, got %v", q.LeaseDuration)
	}
	if q.PollInterval <= 0 {
		return fmt.Errorf("embedding.queue.poll_interval must be positive, got %v", q.PollInterval)
	}
	if q.RetryBackoff < 0 {
		return fmt.Errorf("embedding.queue.retry_backoff must not be negative, got %v", q.RetryBackoff)
	}
	if q.MaxAttempts < 1 {
		return fmt.Errorf("embedding.queue.max_attempts must be >= 1, got %d", q.MaxAttempts)
	}
	if q.BatchSize < 1 {
		return fmt.Errorf("embedding.queue.batch_size must be >= 1, got %d", q.BatchSize)
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
		validateStorageConfig,
		validateDatabaseSSLMode,
		validateEncryptionKey,
		validateInstanceAdmins,
		validateOAuthASConfig,
		validateSchedulerConfig,
		validateMCPConfig,
		validateEmbeddingQueueConfig,
		func(c *Config) error { return validateTrustedProxies(c.Server.TrustedProxies) },
		validatePprofConfig,
		func(c *Config) error { return validateOutboundAllowedCIDRs(c.Security.OutboundAllowedCIDRs) },
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

// defaultAuthRedirectURI is the local-development OAuth callback used as the
// default redirect_uri for every identity provider.
const defaultAuthRedirectURI = "http://localhost:8080/api/v1/auth/callback"

// IsDefaultAuthRedirectURI reports whether uri is the built-in default
// redirect_uri (also baked into config.docker.yaml), i.e. one the operator most
// likely never set.
func IsDefaultAuthRedirectURI(uri string) bool {
	return strings.TrimSpace(uri) == defaultAuthRedirectURI
}

// Code defaults of the deprecated `email:` section (#1190). Exported so the
// boot-time import can tell an inherited default from a value an operator
// chose: koanf merges defaults() into the loaded struct, so an unset key and a
// default are indistinguishable after Load.
const (
	DefaultLegacySMTPHost         = "smtp.gmail.com"
	DefaultLegacySMTPPort         = "587"
	DefaultLegacyPrivacyPolicyURL = "https://example.com/privacy-policy"
)

// defaults returns the code-level configuration defaults as flat, dot-delimited
// keys. They are merged first (lowest precedence); the config.yaml file overrides
// any of them. Duration defaults are expressed as strings ("15m") and decoded by
// the time.Duration hook, matching how the YAML file expresses them.
func defaults() map[string]any {
	d := map[string]any{
		"database.host":                       "localhost",
		"database.port":                       "5432",
		"database.user":                       "postgres",
		"database.name":                       "vibexp_io",
		"database.sslmode":                    "disable",
		"auth.google.redirect_uri":            defaultAuthRedirectURI,
		"auth.github.redirect_uri":            defaultAuthRedirectURI,
		"auth.oidc.redirect_uri":              defaultAuthRedirectURI,
		"auth.oauth_as.access_token_ttl":      "15m",
		"auth.oauth_as.refresh_token_ttl":     "720h",
		"auth.oauth_as.auth_code_ttl":         "10m",
		"auth.oauth_as.key_rotation_interval": "720h",
		"auth.oauth_as.cleanup_interval":      "1h",
		"email.provider":                      "smtp",
		"email.privacy_policy_url":            DefaultLegacyPrivacyPolicyURL,
		"email.smtp.host":                     DefaultLegacySMTPHost,
		"email.smtp.port":                     DefaultLegacySMTPPort,
		"email.postmark.message_stream":       "outbound",
		"frontend.base_url":                   "http://localhost:5173",
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
		"mcp.session_timeout":                 "30m",
		"scheduler.enabled":                   true,
		"scheduler.tick_interval":             "1m",
		"scheduler.job_timeout":               "10m",
		"scheduler.due_limit":                 100,
		"embedding.queue.lease_duration":      "30m",
		"embedding.queue.max_attempts":        5,
		"embedding.queue.batch_size":          20,
		"embedding.queue.poll_interval":       "30s",
		"embedding.queue.retry_backoff":       "2m",
		"a2a.default_timeout":                 "5m",
		"a2a.stream_timeout":                  "2h",
		"event_bus.worker_count":              20,
		"event_bus.buffer_size":               500,
		"event_bus.max_retries":               3,
		"event_bus.retry_backoff":             "200ms",
		"event_bus.retry_jitter":              true,
		"otel.endpoint":                       "localhost:4317",
		"otel.export_interval":                "60s",
		"otel.trace_sample_ratio":             0.1,
	}
	maps.Copy(d, serverDefaults())
	maps.Copy(d, aiSummaryDefaults())
	return d
}

// serverDefaults holds the `server:` defaults. Like aiSummaryDefaults they live
// in their own map so adding a knob (server.pprof.listen_addr, #1277) does not
// push defaults() over golangci's function-length ceiling.
func serverDefaults() map[string]any {
	return map[string]any{
		"server.port":                "8080",
		"server.log_level":           "info",
		"server.log_format":          "json",
		"server.service_version":     "dev",
		"server.release_sha":         "dev",
		"server.release_date":        "unknown",
		"server.max_body_size_bytes": int64(10 << 20),
		"server.error_type_base_uri": "about:blank",
		"server.pprof.listen_addr":   DefaultPprofListenAddr,
	}
}

// aiSummaryDefaults holds the `ai_summary:` defaults (#1071). They live in their
// own map, merged above, because defaults() sits at golangci's function-length
// ceiling — folding a section in keeps adding one knob from forcing an unrelated
// refactor of every other default.
//
// The values themselves come from models.DefaultInstanceAISummarySettings, the
// same built-in defaults the instance settings service falls back to, so the
// numbers exist once.
func aiSummaryDefaults() map[string]any {
	d := models.DefaultInstanceAISummarySettings()
	return map[string]any{
		"ai_summary.enabled":                   d.Enabled,
		"ai_summary.top_n":                     d.TopN,
		"ai_summary.max_top_n":                 models.MaxAISummaryTopN,
		"ai_summary.per_document_chars":        d.PerDocumentChars,
		"ai_summary.total_context_chars":       d.TotalContextChars,
		"ai_summary.max_output_tokens":         d.MaxOutputTokens,
		"ai_summary.max_output_tokens_ceiling": DefaultLegacyAISummaryOutputTokensCeiling,
		"ai_summary.request_timeout":           d.RequestTimeout.String(),
		"ai_summary.style":                     d.Style,
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

	deprecationWarnings, err := checkRemovedSections(path, parsed)
	if err != nil {
		return nil, err
	}

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
				stringToEnvStringSliceHookFunc(","),
			),
		},
	}
	if err := k.UnmarshalWithConf("", &cfg, unmarshalConf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	cfg.DeprecationWarnings = deprecationWarnings
	return &cfg, nil
}

// removalMode is what the loader does when config.yaml still sets a removed
// key: fail boot, or only warn on every boot.
type removalMode int

const (
	// removalFatal fails startup while the key is present at all.
	removalFatal removalMode = iota
	// removalWarn adds a deprecation warning while the key is populated (see
	// populatedConfigValue), and loads normally.
	removalWarn
)

// removedConfigKey is one config.yaml key an operator must stop setting.
type removedConfigKey struct {
	// path is the dotted key, e.g. "github" or "auth.google".
	path string
	mode removalMode
	// guidance tells the operator where the setting went.
	guidance string
}

// legacyAuthKeyGuidance is the warning for every deprecated auth provider and
// allowlist key (#1232). #1240 flips those entries to removalFatal.
const legacyAuthKeyGuidance = "sign-in providers and the access allowlist are now stored in the database " +
	"and managed under Admin → Settings → Authentication. At boot this key is imported once when the " +
	"database holds none, and ignored afterwards. Delete it from your config.yaml (and its env vars): it " +
	"becomes a boot failure no earlier than the second minor release after this one. " +
	"See https://vibexp.io/docs/user-guide/self-hosting/upgrading"

// removedConfigKeys lists the config.yaml keys that no longer do what they
// used to, with the guidance an operator needs when their config.yaml still
// carries one.
//
// This exists because unknown keys are otherwise SILENT. config.schema.json is
// `additionalProperties: false`, but that schema is a drift gate and an
// editor aid — it is not consulted at runtime, and koanf/mapstructure ignore
// keys with no matching field. Without this check an operator upgrading past
// #483 would keep a `github:` block full of credentials that does nothing, and
// conclude the integration is configured when it is not. Failing at boot is the
// whole point: loud beats silently wrong.
//
// Paths are matched exactly, so the top-level `github` entry never matches
// `auth.github`, and an `auth.*` entry never touches a sibling auth key
// (instance_admins, oauth_as, session_encryption_key, …).
var removedConfigKeys = []removedConfigKey{
	{path: "github", mode: removalFatal, guidance: "GitHub App credentials are now configured per team in the UI " +
		"(open the team, then Settings → GitHub Integration), not instance-wide. " +
		"Delete the `github:` section from your config.yaml and re-register the App on each team. " +
		"Note this is NOT `auth.github` (the web-login OAuth client), which is unaffected."},
	{path: "auth.providers", mode: removalWarn, guidance: legacyAuthKeyGuidance},
	{path: "auth.provider", mode: removalWarn, guidance: legacyAuthKeyGuidance},
	{path: "auth.access_allowlist", mode: removalWarn, guidance: legacyAuthKeyGuidance},
	{path: "auth.google", mode: removalWarn, guidance: legacyAuthKeyGuidance},
	{path: "auth.github", mode: removalWarn, guidance: legacyAuthKeyGuidance},
	{path: "auth.oidc", mode: removalWarn, guidance: legacyAuthKeyGuidance},
}

// checkRemovedSections inspects the interpolated config.yaml for the keys in
// removedConfigKeys. It fails startup when one or more fatal keys are present,
// and otherwise returns one warning per populated warn-mode key.
//
// Every offending key is reported at once, in a fixed order: an operator
// mid-migration should not have to restart once per removed key to discover
// them one at a time.
func checkRemovedSections(path string, parsed map[string]interface{}) (warnings []string, err error) {
	var fatal []removedConfigKey
	for _, key := range removedConfigKeys {
		value, present := lookupConfigPath(parsed, key.path)
		switch {
		case !present:
		case key.mode == removalFatal:
			fatal = append(fatal, key)
		case populatedConfigValue(value):
			warnings = append(warnings, fmt.Sprintf("config key %q is deprecated: %s", key.path, key.guidance))
		}
	}
	sort.Strings(warnings)
	if len(fatal) == 0 {
		return warnings, nil
	}
	sort.Slice(fatal, func(i, j int) bool { return fatal[i].path < fatal[j].path })

	details := make([]string, 0, len(fatal))
	for _, key := range fatal {
		details = append(details, fmt.Sprintf("%q: %s", key.path, key.guidance))
	}
	return nil, fmt.Errorf(
		"config file %q declares removed section(s) — %s",
		path, strings.Join(details, " | "))
}

// lookupConfigPath walks a dotted path through the parsed config maps and
// reports the value found there, and whether the key is present at all.
func lookupConfigPath(parsed map[string]interface{}, dotted string) (interface{}, bool) {
	var node interface{} = parsed
	for _, part := range strings.Split(dotted, ".") {
		m, ok := node.(map[string]interface{})
		if !ok {
			return nil, false
		}
		if node, ok = m[part]; !ok {
			return nil, false
		}
	}
	return node, true
}

// populatedConfigValue reports whether a config value carries something an
// operator set: a non-blank string, a list or map with a populated element,
// or any other scalar. A `redirect_uri` is not counted: the baked
// config.docker.yaml defaults it to a localhost URL on every install, so it
// alone never makes a provider block populated.
func populatedConfigValue(value interface{}) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case []interface{}:
		for _, item := range v {
			if populatedConfigValue(item) {
				return true
			}
		}
		return false
	case map[string]interface{}:
		for key, item := range v {
			if key != "redirect_uri" && populatedConfigValue(item) {
				return true
			}
		}
		return false
	default:
		return true
	}
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
	// Auto-wire /api/v1 JWT acceptance to the embedded AS. Runs after the MCP
	// issuer default so mcp.resource_uri (the pinned audience) is resolved.
	applyAPIOAuthDefaults(cfg)

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
