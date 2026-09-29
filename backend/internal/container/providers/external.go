package providers

import (
	"context"
	"log/slog"

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
	msgIdentityProviderEnabled = "Identity provider enabled"
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
// precedence (config.AuthConfig.LegacyEnabledProviderNames).
func resolveEnabledProviderNames(cfg *config.Config) []idp.ProviderName {
	enabled := cfg.Auth.LegacyEnabledProviderNames()
	names := make([]idp.ProviderName, len(enabled))
	for i, name := range enabled {
		names[i] = idp.ProviderName(name)
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
	if cfg.Auth.LegacyGoogle.ClientID == "" || cfg.Auth.LegacyGoogle.ClientSecret == "" {
		logger.With("provider", "google").
			Warn("Google enabled but GOOGLE_CLIENT_ID/SECRET are absent; skipping")
		return nil, false
	}
	provider, err := google.New(context.Background(), google.Config{
		ClientID:     cfg.Auth.LegacyGoogle.ClientID,
		ClientSecret: cfg.Auth.LegacyGoogle.ClientSecret,
		RedirectURL:  cfg.Auth.LegacyGoogle.RedirectURI,
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
	if cfg.Auth.LegacyGitHub.ClientID == "" || cfg.Auth.LegacyGitHub.ClientSecret == "" {
		logger.With("provider", "github").
			Warn("GitHub enabled but GITHUB_CLIENT_ID/SECRET are absent; skipping")
		return nil, false
	}
	provider, err := github.New(github.Config{
		ClientID:     cfg.Auth.LegacyGitHub.ClientID,
		ClientSecret: cfg.Auth.LegacyGitHub.ClientSecret,
		RedirectURL:  cfg.Auth.LegacyGitHub.RedirectURI,
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
		IssuerURL:    cfg.Auth.LegacyOIDC.IssuerURL,
		ClientID:     cfg.Auth.LegacyOIDC.ClientID,
		ClientSecret: cfg.Auth.LegacyOIDC.ClientSecret,
		RedirectURL:  cfg.Auth.LegacyOIDC.RedirectURI,
	})
	if err != nil {
		logger.With("provider", "oidc", "issuer_url", cfg.Auth.LegacyOIDC.IssuerURL, "error", err).
			Warn("OIDC provider initialization failed; skipping")
		return nil, false
	}
	logger.With("provider", "oidc", "issuer_url", cfg.Auth.LegacyOIDC.IssuerURL).Info(msgIdentityProviderEnabled)
	return provider, true
}

// ProvideEmailSender creates a new EmailSender. DEPRECATED: every send now goes
// through services.EmailSenderResolver, which builds the provider per send from
// the database; this legacy path is removed with the `email:` config (#1193).
func ProvideEmailSender(cfg *config.Config) external.EmailSender {
	return implementations.NewEmailSender(cfg)
}

// The process-wide GitHubAppClient provider is gone (#480). Clients are now
// built per team by services.GitHubAppClientResolver from the team's
// github_app_configs row, so nothing constructs one from instance config.
