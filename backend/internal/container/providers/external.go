package providers

import (
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/external"
	"github.com/vibexp/vibexp/internal/external/implementations"
)

// The boot-time identity provider registry is gone (#1234). Sign-in providers
// are resolved from the database at runtime by services.IdentityProviderResolver
// (see ProvideIdentityProviderResolver); the legacy config.yaml providers are
// imported into it once at boot (#1232).

// ProvideEmailSender creates a new EmailSender. DEPRECATED: every send now goes
// through services.EmailSenderResolver, which builds the provider per send from
// the database; this legacy path is removed with the `email:` config (#1193).
func ProvideEmailSender(cfg *config.Config) external.EmailSender {
	return implementations.NewEmailSender(cfg)
}

// The process-wide GitHubAppClient provider is gone (#480). Clients are now
// built per team by services.GitHubAppClientResolver from the team's
// github_app_configs row, so nothing constructs one from instance config.
