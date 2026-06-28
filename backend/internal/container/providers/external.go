package providers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/darkrockmountain/gomail"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/auth/idp/github"
	"github.com/vibexp/vibexp/internal/auth/idp/google"
	"github.com/vibexp/vibexp/internal/auth/idp/oidc"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/external/implementations"
)

// ProvideIdentityProviderRegistry builds the set of web-login identity
// providers enabled for this deployment. A deployment may enable one or
// several providers at once (e.g. Google + GitHub) via AUTH_PROVIDERS.
//
// Provider selection (see resolveEnabledProviderNames):
//   - AUTH_PROVIDERS (comma list) when set — the multi-provider path.
//   - else AUTH_PROVIDER (single value) — the backward-compatible shim.
//   - else no providers (dev login only).
//
// Construction is resilient: an enabled provider whose credentials are absent
// or whose OIDC discovery fails is logged and skipped rather than crashing
// startup, so the server always boots (web login is simply limited to the
// providers that built successfully; dev login is unaffected). An empty
// registry means web login is disabled.
func ProvideIdentityProviderRegistry(cfg *config.Config, logger *slog.Logger) (*idp.Registry, error) {
	names := resolveEnabledProviderNames(cfg)

	built := make([]idp.IdentityProvider, 0, len(names))
	for _, name := range names {
		if provider, ok := buildIdentityProvider(name, cfg, logger); ok {
			built = append(built, provider)
		}
	}

	registry := idp.NewRegistry(built...)
	enabled := registry.Enabled()
	enabledStrs := make([]string, len(enabled))
	for i, n := range enabled {
		enabledStrs[i] = string(n)
	}
	logger.With("providers", enabledStrs).Info("Identity provider registry initialized")
	return registry, nil
}

// resolveEnabledProviderNames computes the ordered, de-duplicated list of
// provider names to enable, applying the AUTH_PROVIDERS → AUTH_PROVIDER
// precedence.
func resolveEnabledProviderNames(cfg *config.Config) []idp.ProviderName {
	normalize := func(raw string) idp.ProviderName {
		return idp.ProviderName(strings.ToLower(strings.TrimSpace(raw)))
	}

	var raw []string
	switch {
	case len(cfg.AuthProviders) > 0:
		raw = cfg.AuthProviders
	case strings.TrimSpace(cfg.AuthProvider) != "":
		raw = []string{cfg.AuthProvider}
	}

	seen := make(map[idp.ProviderName]struct{}, len(raw))
	names := make([]idp.ProviderName, 0, len(raw))
	for _, r := range raw {
		name := normalize(r)
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

// buildIdentityProvider constructs a single provider by name, returning
// (provider, true) on success or (nil, false) when it is unrecognized,
// missing credentials, or fails to initialize. Failures are logged, never
// fatal.
func buildIdentityProvider(
	name idp.ProviderName, cfg *config.Config, logger *slog.Logger,
) (idp.IdentityProvider, bool) {
	switch name {
	case idp.ProviderGoogle:
		return buildGoogleProvider(cfg, logger)
	case idp.ProviderGitHub:
		return buildGitHubProvider(cfg, logger)
	case idp.ProviderOIDC:
		return buildOIDCProvider(cfg, logger)
	default:
		logger.With("provider", string(name)).
			Warn("Unrecognized identity provider in AUTH_PROVIDERS; skipping")
		return nil, false
	}
}

func buildGoogleProvider(cfg *config.Config, logger *slog.Logger) (idp.IdentityProvider, bool) {
	if cfg.GoogleClientID == "" || cfg.GoogleClientSecret == "" {
		logger.With("provider", "google").
			Warn("Google enabled but GOOGLE_CLIENT_ID/SECRET are absent; skipping")
		return nil, false
	}
	provider, err := google.New(context.Background(), google.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  cfg.GoogleRedirectURI,
	})
	if err != nil {
		logger.With("provider", "google", "error", err).
			Warn("Google provider initialization failed; skipping")
		return nil, false
	}
	logger.With("provider", "google").Info("Identity provider enabled")
	return provider, true
}

func buildGitHubProvider(cfg *config.Config, logger *slog.Logger) (idp.IdentityProvider, bool) {
	if cfg.GitHubClientID == "" || cfg.GitHubClientSecret == "" {
		logger.With("provider", "github").
			Warn("GitHub enabled but GITHUB_CLIENT_ID/SECRET are absent; skipping")
		return nil, false
	}
	provider, err := github.New(github.Config{
		ClientID:     cfg.GitHubClientID,
		ClientSecret: cfg.GitHubClientSecret,
		RedirectURL:  cfg.GitHubRedirectURI,
	})
	if err != nil {
		logger.With("provider", "github", "error", err).
			Warn("GitHub provider initialization failed; skipping")
		return nil, false
	}
	logger.With("provider", "github").Info("Identity provider enabled")
	return provider, true
}

func buildOIDCProvider(cfg *config.Config, logger *slog.Logger) (idp.IdentityProvider, bool) {
	provider, err := oidc.New(context.Background(), oidc.Config{
		Name:         idp.ProviderOIDC,
		IssuerURL:    cfg.OIDCIssuerURL,
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		RedirectURL:  cfg.OIDCRedirectURI,
	})
	if err != nil {
		logger.With("provider", "oidc", "issuer_url", cfg.OIDCIssuerURL, "error", err).
			Warn("OIDC provider initialization failed; skipping")
		return nil, false
	}
	logger.With("provider", "oidc", "issuer_url", cfg.OIDCIssuerURL).Info("Identity provider enabled")
	return provider, true
}

// ProvideEmailProvider creates an EmailProvider based on the EMAIL_PROVIDER config value.
// Supported providers: "smtp" (default), "mailgun", "postmark", "sendgrid". The value
// is normalised to lowercase and trimmed before matching, so "MAILGUN" and "smtp " work
// correctly. When EMAIL_PROVIDER is empty or "smtp" and no SMTP host/port are configured,
// a no-op stub is returned so the container can wire up without email credentials.
func ProvideEmailProvider(cfg *config.Config, logger *slog.Logger) (external.EmailProvider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.EmailProvider)) {
	case "mailgun":
		provider, err := implementations.NewMailgunEmailProvider(cfg)
		if err != nil {
			return nil, fmt.Errorf("email provider factory: %w", err)
		}
		logger.With("email_provider", "mailgun").Info("Email provider initialized")
		return provider, nil
	case "postmark":
		provider, err := implementations.NewPostmarkEmailProvider(cfg)
		if err != nil {
			return nil, fmt.Errorf("email provider factory: %w", err)
		}
		logger.With("email_provider", "postmark").Info("Email provider initialized")
		return provider, nil
	case "sendgrid":
		provider, err := implementations.NewSendGridEmailProvider(cfg)
		if err != nil {
			return nil, fmt.Errorf("email provider factory: %w", err)
		}
		logger.With("email_provider", "sendgrid").Info("Email provider initialized")
		return provider, nil
	case "smtp", "":
		if cfg.SMTPHost == "" || cfg.SMTPPort == "" {
			logger.With("email_provider", "stub").Info("Email provider initialized")
			return &stubEmailProvider{}, nil
		}
		provider, err := implementations.NewSMTPEmailProvider(cfg)
		if err != nil {
			return nil, fmt.Errorf("email provider factory: %w", err)
		}
		logger.With("email_provider", "smtp").Info("Email provider initialized")
		return provider, nil
	default:
		return nil, fmt.Errorf("email provider factory: unknown email provider %q", cfg.EmailProvider)
	}
}

// stubEmailProvider is a no-op provider for testing without SMTP config
type stubEmailProvider struct{}

func (s *stubEmailProvider) SendEmail(ctx context.Context, message *gomail.EmailMessage) error {
	// No-op for tests
	return nil
}

// ProvideSMTPClient creates a new SMTPClient (DEPRECATED: Use ProvideEmailProvider instead)
func ProvideSMTPClient(cfg *config.Config) external.SMTPClient {
	return implementations.NewSMTPClient(cfg)
}

// ProvideGitHubAppClient creates a new GitHubAppClient
func ProvideGitHubAppClient(cfg *config.Config, logger *slog.Logger) (external.GitHubAppClient, error) {
	githubCfg, err := cfg.GetGitHubAppConfig()
	if err != nil {
		return nil, err
	}
	return implementations.NewGitHubAppClient(githubCfg, logger), nil
}
