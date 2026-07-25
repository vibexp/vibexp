package providers

import (
	"context"
	"log/slog"
	"strings"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/auth/idp/github"
	"github.com/vibexp/vibexp/internal/auth/idp/google"
	"github.com/vibexp/vibexp/internal/auth/idp/oidc"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/external/implementations"
)

// Log messages shared by the per-provider construction paths below.
const (
	msgIdentityProviderEnabled  = "Identity provider enabled"
	msgEmailProviderInitialized = "Email provider initialized"
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
	case len(cfg.Auth.Providers) > 0:
		raw = cfg.Auth.Providers
	case strings.TrimSpace(cfg.Auth.Provider) != "":
		raw = []string{cfg.Auth.Provider}
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
	if cfg.Auth.Google.ClientID == "" || cfg.Auth.Google.ClientSecret == "" {
		logger.With("provider", "google").
			Warn("Google enabled but GOOGLE_CLIENT_ID/SECRET are absent; skipping")
		return nil, false
	}
	provider, err := google.New(context.Background(), google.Config{
		ClientID:     cfg.Auth.Google.ClientID,
		ClientSecret: cfg.Auth.Google.ClientSecret,
		RedirectURL:  cfg.Auth.Google.RedirectURI,
	})
	if err != nil {
		logger.With("provider", "google", "error", err).
			Warn("Google provider initialization failed; skipping")
		return nil, false
	}
	logger.With("provider", "google").Info(msgIdentityProviderEnabled)
	return provider, true
}

func buildGitHubProvider(cfg *config.Config, logger *slog.Logger) (idp.IdentityProvider, bool) {
	if cfg.Auth.GitHub.ClientID == "" || cfg.Auth.GitHub.ClientSecret == "" {
		logger.With("provider", "github").
			Warn("GitHub enabled but GITHUB_CLIENT_ID/SECRET are absent; skipping")
		return nil, false
	}
	provider, err := github.New(github.Config{
		ClientID:     cfg.Auth.GitHub.ClientID,
		ClientSecret: cfg.Auth.GitHub.ClientSecret,
		RedirectURL:  cfg.Auth.GitHub.RedirectURI,
	})
	if err != nil {
		logger.With("provider", "github", "error", err).
			Warn("GitHub provider initialization failed; skipping")
		return nil, false
	}
	logger.With("provider", "github").Info(msgIdentityProviderEnabled)
	return provider, true
}

func buildOIDCProvider(cfg *config.Config, logger *slog.Logger) (idp.IdentityProvider, bool) {
	provider, err := oidc.New(context.Background(), oidc.Config{
		Name:         idp.ProviderOIDC,
		IssuerURL:    cfg.Auth.OIDC.IssuerURL,
		ClientID:     cfg.Auth.OIDC.ClientID,
		ClientSecret: cfg.Auth.OIDC.ClientSecret,
		RedirectURL:  cfg.Auth.OIDC.RedirectURI,
	})
	if err != nil {
		logger.With("provider", "oidc", "issuer_url", cfg.Auth.OIDC.IssuerURL, "error", err).
			Warn("OIDC provider initialization failed; skipping")
		return nil, false
	}
	logger.With("provider", "oidc", "issuer_url", cfg.Auth.OIDC.IssuerURL).Info(msgIdentityProviderEnabled)
	return provider, true
}

// ProvideEmailProvider creates an EmailProvider based on the EMAIL_PROVIDER config value.
// Supported providers: "smtp" (default), "mailgun", "postmark", "sendgrid". The value
// is normalised to lowercase and trimmed before matching, so "MAILGUN" and "smtp " work
// correctly. When EMAIL_PROVIDER is empty or "smtp" and no SMTP host/port are configured,
// a no-op stub is returned so the container can wire up without email credentials.
//
// This is a mapping adapter: provider selection itself lives in
// implementations.NewEmailProvider, which is config-free so it can also be
// called per send with team-supplied values.
func ProvideEmailProvider(cfg *config.Config, logger *slog.Logger) (external.EmailProvider, error) {
	provider, err := implementations.NewEmailProvider(emailProviderSpec(cfg), logger)
	if err != nil {
		return nil, err
	}

	logger.With("email_provider", implementations.ProviderLabel(provider)).Info(msgEmailProviderInitialized)
	return provider, nil
}

// emailProviderSpec maps the process-wide email config onto the config-free
// spec the factory consumes.
func emailProviderSpec(cfg *config.Config) implementations.ProviderSpec {
	return implementations.ProviderSpec{
		Type: cfg.Email.Provider,
		SMTP: implementations.SMTPSpec{
			Host:     cfg.Email.SMTP.Host,
			Port:     cfg.Email.SMTP.Port,
			Username: cfg.Email.SMTP.Username,
			Password: cfg.Email.SMTP.Password,
		},
		Mailgun: implementations.MailgunSpec{
			BaseURL:    cfg.Email.Mailgun.BaseURL,
			Domain:     cfg.Email.Mailgun.Domain,
			SendingKey: cfg.Email.Mailgun.SendingKey,
		},
		Postmark: implementations.PostmarkSpec{
			ServerToken:   cfg.Email.Postmark.ServerToken,
			MessageStream: cfg.Email.Postmark.MessageStream,
		},
		SendGrid: implementations.SendGridSpec{
			APIKey: cfg.Email.SendGrid.APIKey,
		},
	}
}

// stubEmailProvider aliases the no-op provider that now lives beside the
// factory. Kept as an alias so this package (and its tests) can keep referring
// to the stub by its original name while there is only one such type.
type stubEmailProvider = implementations.StubEmailProvider

// ProvideEmailSender creates a new EmailSender (DEPRECATED: Use ProvideEmailProvider instead)
func ProvideEmailSender(cfg *config.Config) external.EmailSender {
	return implementations.NewEmailSender(cfg)
}

// The process-wide GitHubAppClient provider is gone (#480). Clients are now
// built per team by services.GitHubAppClientResolver from the team's
// github_app_configs row, so nothing constructs one from instance config.
