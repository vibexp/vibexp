// Package google provides a first-class Google identity provider. Google is
// fully OIDC-compliant, so this is a thin wrapper around the generic
// internal/auth/idp/oidc client pinned to the Google issuer — it reports the
// configured name (idp.ProviderGoogle by default) and supplies Google's standard scopes.
package google

import (
	"context"
	"fmt"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/auth/idp/oidc"
)

// issuerURL is Google's OIDC issuer used for discovery. It is a var (not a
// const) so unit tests can point discovery at an httptest issuer.
var issuerURL = "https://accounts.google.com"

// Config holds the credentials required to construct the Google provider.
type Config struct {
	// Name is the provider's identity (its slug); empty defaults to
	// idp.ProviderGoogle.
	Name         idp.ProviderName
	ClientID     string
	ClientSecret string
	RedirectURL  string
	// IssuerURL overrides Google's issuer for discovery; empty uses Google's.
	// It exists so a caller's tests can point discovery at a fake issuer.
	IssuerURL string
}

// New builds a Google identity provider. It performs OIDC discovery against
// the Google issuer, so it requires network access at construction time.
func New(ctx context.Context, cfg Config) (idp.IdentityProvider, error) {
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("google: client ID is required")
	}
	if cfg.ClientSecret == "" {
		return nil, fmt.Errorf("google: client secret is required")
	}

	name := cfg.Name
	if name == "" {
		name = idp.ProviderGoogle
	}
	issuer := cfg.IssuerURL
	if issuer == "" {
		issuer = issuerURL
	}
	return oidc.New(ctx, oidc.Config{
		Name:         name,
		IssuerURL:    issuer,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       []string{"openid", "email", "profile"},
	})
}
