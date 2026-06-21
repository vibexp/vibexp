// Package idp defines the provider-agnostic Identity Provider abstraction
// used by the auth service. Concrete implementations (OIDC, etc.) live in
// sub-packages under internal/auth/idp/.
package idp

import (
	"context"
	"errors"
	"time"
)

// ProviderName uniquely identifies an identity provider implementation
// (e.g. "google", "workos"). It is persisted in users.idp_provider so it
// must remain stable across releases.
type ProviderName string

const (
	// ProviderGoogle is the canonical name for the Google OIDC provider.
	ProviderGoogle ProviderName = "google"

	// ProviderWorkOS is the canonical name for the WorkOS AuthKit provider.
	// This value is persisted in users.idp_provider and must not change.
	ProviderWorkOS ProviderName = "workos"
)

// ErrTokenInvalid is returned when an access or ID token cannot be validated.
var ErrTokenInvalid = errors.New("identity provider: invalid token")

// Tokens represents the tokens returned by an identity provider after a
// successful authorization code exchange or refresh.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresAt    time.Time
}

// Claims represents the canonical user claims sourced from an identity
// provider. Subject is the stable, unique provider-specific user identifier
// (the OIDC `sub` claim). EmailVerified reflects the provider's
// `email_verified` claim; future code paths may gate sign-in on it.
type Claims struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
}

// IdentityProvider abstracts an authorization-code-flow identity provider.
// Implementations must be safe for concurrent use.
type IdentityProvider interface {
	// Name returns the canonical provider identifier persisted alongside
	// the user (see users.idp_provider).
	Name() ProviderName

	// AuthorizeURL builds the URL used to start the authorization-code
	// flow with the given state, redirect URI, and optional provider hint.
	// The provider hint is forwarded to the identity provider so it can
	// route the user directly to the correct OAuth connection (e.g.
	// "GoogleOAuth", "GitHubOAuth"). Implementations that do not support
	// provider routing should accept and ignore the parameter.
	AuthorizeURL(state, redirectURI, provider string) string

	// ExchangeCode exchanges an authorization code for tokens and the
	// validated user claims (sourced from the ID token or userinfo).
	ExchangeCode(ctx context.Context, code, redirectURI string) (*Tokens, *Claims, error)

	// Refresh requests a new set of tokens using the given refresh token.
	Refresh(ctx context.Context, refreshToken string) (*Tokens, error)
}
