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
// (e.g. "google", "oidc"). It is persisted in users.idp_provider so it
// must remain stable across releases.
type ProviderName string

const (
	// ProviderGoogle is the canonical name for the Google OIDC provider.
	ProviderGoogle ProviderName = "google"

	// ProviderGitHub is the canonical name for the GitHub OAuth2 provider.
	// This value is persisted in users.idp_provider and must not change.
	ProviderGitHub ProviderName = "github"

	// ProviderOIDC is the canonical name for the generic OIDC provider used
	// with any OIDC-compliant issuer (Keycloak, Authentik, Zitadel, Auth0,
	// Clerk via discovery). It is persisted in users.idp_provider and must
	// not change.
	ProviderOIDC ProviderName = "oidc"
)

// ErrTokenInvalid is returned when an access or ID token cannot be validated.
var ErrTokenInvalid = errors.New("identity provider: invalid token")

// ErrNonceMismatch is returned by ExchangeCode when the ID token's nonce claim
// is missing or differs from the nonce sent in the authorization request
// (OpenID Connect Core §3.1.3.7), i.e. the ID token is not bound to the
// browser flow that requested it.
var ErrNonceMismatch = errors.New("identity provider: id token nonce mismatch")

// ErrProviderUnavailable marks a failure to reach the identity provider (an
// unreachable host, a 5xx or a throttling answer) while it was being asked
// about a token, as opposed to a verdict on the token. A refresh that fails
// with it must keep the session.
var ErrProviderUnavailable = errors.New("identity provider: temporarily unavailable")

// ErrRevocationUnsupported is returned by TokenRevoker.RevokeRefreshToken when
// the provider offers no token revocation endpoint (RFC 7009), and by callers
// resolving a provider that does not implement TokenRevoker at all.
var ErrRevocationUnsupported = errors.New("identity provider: token revocation not supported")

// TokenRevoker is an optional capability of an IdentityProvider: revoking a
// refresh token at the issuer (RFC 7009), so a signed-out session leaves no
// live grant behind (#1649). Callers type-assert for it; a provider without
// it cannot revoke.
type TokenRevoker interface {
	// RevokeRefreshToken asks the issuer to revoke refreshToken. It returns
	// ErrRevocationUnsupported when the issuer advertises no revocation
	// endpoint, and an error for any answer other than 200 OK (RFC 7009
	// §2.2 answers 200 for an unknown or already revoked token too).
	RevokeRefreshToken(ctx context.Context, refreshToken string) error
}

// FlowParams are the per-authorization-request secrets of one sign-in flow.
// They are generated fresh for every authorization request and must survive
// the redirect to the identity provider and back, so the callback can hand the
// same values to ExchangeCode. An empty field disables that protection.
type FlowParams struct {
	// CodeVerifier is the PKCE code verifier (RFC 7636). AuthorizeURL sends its
	// S256 challenge; ExchangeCode sends the verifier itself.
	CodeVerifier string
	// Nonce is the OpenID Connect nonce. AuthorizeURL sends it; ExchangeCode
	// requires the ID token to carry the same value. Providers without an ID
	// token ignore it.
	Nonce string
}

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
	// Raw is the full verified claim set the canonical claims were read from:
	// the ID token's, or userinfo's when the token response carried no ID
	// token. A role mapping reads it (#1644). Nil for a provider that does not
	// expose one. It may carry personal data, so it is never logged.
	Raw map[string]any
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
	// provider routing should accept and ignore the parameter. flow carries
	// the PKCE verifier and nonce bound to this authorization request.
	AuthorizeURL(state, redirectURI, provider string, flow FlowParams) string

	// ExchangeCode exchanges an authorization code for tokens and the
	// validated user claims (sourced from the ID token or userinfo). flow must
	// be the FlowParams the authorization URL was built with; a nonce the ID
	// token does not echo yields ErrNonceMismatch.
	ExchangeCode(ctx context.Context, code, redirectURI string, flow FlowParams) (*Tokens, *Claims, error)

	// Refresh requests a new set of tokens using the given refresh token and
	// returns them with the user's current claims: those of the verified ID
	// token in the refresh response (no nonce is checked, since a refresh is
	// not bound to a browser flow), or of userinfo when the response carries no
	// ID token. Claims may be nil only for a provider that offers neither.
	Refresh(ctx context.Context, refreshToken string) (*Tokens, *Claims, error)
}
