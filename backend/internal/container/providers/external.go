package providers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/darkrockmountain/gomail"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/auth/idp/oidc"
	"github.com/vibexp/vibexp/internal/auth/idp/workos"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/external/implementations"
)

// ProvideIdentityProvider creates a provider-agnostic IdentityProvider based
// on the AUTH_PROVIDER config value. Supported providers: "workos", "oidc",
// and "" (none / auto-detect). The value is matched case-insensitively.
//
//   - "workos": WorkOS AuthKit. When credentials are absent a no-op stub is
//     returned so the container can still be wired up (CI / dev-login).
//   - "oidc": a generic OIDC provider (Keycloak, Authentik, Zitadel, Auth0,
//     Google). OIDC discovery runs at startup; on discovery failure (bad or
//     unreachable issuer) a WARNING is logged and a no-op stub is returned so
//     the server still boots (web login disabled; dev login unaffected).
//   - "" / "none" / unrecognized: backward-compatible auto-detect — WorkOS if
//     its credentials are present, otherwise the no-op stub.
//
// No branch returns a fatal error on misconfiguration: a misconfigured
// provider degrades to the stub rather than crashing startup.
func ProvideIdentityProvider(cfg *config.Config, logger *slog.Logger) (idp.IdentityProvider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.AuthProvider)) {
	case "workos":
		return provideWorkOSIdentityProvider(cfg, logger)
	case "oidc":
		return provideOIDCIdentityProvider(cfg, logger)
	case "", "none":
		if cfg.WorkOSClientID != "" && cfg.WorkOSAPIKey != "" {
			return provideWorkOSIdentityProvider(cfg, logger)
		}
		logger.With("auth_provider", "stub").Info("Identity provider initialized")
		return &stubIdentityProvider{}, nil
	default:
		logger.With("auth_provider", cfg.AuthProvider).
			Warn("Unrecognized AUTH_PROVIDER; web login disabled (using no-op stub)")
		return &stubIdentityProvider{}, nil
	}
}

// provideWorkOSIdentityProvider constructs the WorkOS provider, falling back to
// the no-op stub when credentials are absent.
func provideWorkOSIdentityProvider(cfg *config.Config, logger *slog.Logger) (idp.IdentityProvider, error) {
	if cfg.WorkOSClientID == "" || cfg.WorkOSAPIKey == "" {
		logger.With("auth_provider", "stub").
			Warn("AUTH_PROVIDER=workos but WorkOS credentials are absent; web login disabled (using no-op stub)")
		return &stubIdentityProvider{}, nil
	}

	provider, err := workos.New(context.Background(), workos.Config{
		ClientID:    cfg.WorkOSClientID,
		APIKey:      cfg.WorkOSAPIKey,
		RedirectURI: cfg.WorkOSRedirectURI,
	})
	if err != nil {
		return nil, err
	}
	logger.With("auth_provider", "workos").Info("Identity provider initialized")
	return provider, nil
}

// provideOIDCIdentityProvider constructs the generic OIDC provider. OIDC
// discovery failure is non-fatal: a WARNING is logged and the no-op stub is
// returned so the server still boots with web login disabled.
func provideOIDCIdentityProvider(cfg *config.Config, logger *slog.Logger) (idp.IdentityProvider, error) {
	provider, err := oidc.New(context.Background(), oidc.Config{
		Name:         idp.ProviderName("oidc"),
		IssuerURL:    cfg.OIDCIssuerURL,
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		RedirectURL:  cfg.OIDCRedirectURI,
	})
	if err != nil {
		logger.With(
			"error", err,
			"issuer_url", cfg.OIDCIssuerURL,
		).
			Warn("OIDC provider initialization failed; web login disabled (using no-op stub)")
		return &stubIdentityProvider{}, nil
	}
	logger.Info(
		"Identity provider initialized",
		"auth_provider", "oidc",
		"issuer_url", cfg.OIDCIssuerURL,
	)
	return provider, nil
}

// stubIdentityProvider is a no-op implementation used when OAuth credentials
// are absent (typically test environments). All methods return errors to make
// accidental production use loudly fail.
type stubIdentityProvider struct{}

func (s *stubIdentityProvider) Name() idp.ProviderName {
	return idp.ProviderWorkOS
}

func (s *stubIdentityProvider) AuthorizeURL(state, redirectURI, provider string) string {
	return ""
}

func (s *stubIdentityProvider) ExchangeCode(
	ctx context.Context, code, redirectURI string,
) (*idp.Tokens, *idp.Claims, error) {
	return nil, nil, fmt.Errorf("idp: identity provider not configured")
}

func (s *stubIdentityProvider) Refresh(ctx context.Context, refreshToken string) (*idp.Tokens, error) {
	return nil, fmt.Errorf("idp: identity provider not configured")
}

// ProvideEmailProvider creates an EmailProvider based on the EMAIL_PROVIDER config value.
// Supported providers: "smtp" (default), "mailgun". The value is normalised to
// lowercase and trimmed before matching, so "MAILGUN" and "smtp " work correctly.
// When EMAIL_PROVIDER is empty or "smtp" and no SMTP host/port are configured,
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
