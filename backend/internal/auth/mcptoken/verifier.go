// Package mcptoken adapts the shared AuthKit JWT verifier
// (internal/auth/authkit) to the MCP SDK's TokenVerifier contract for the
// VibeXP MCP endpoint. It enforces RFC 8707 audience binding for the MCP
// resource and translates verification failures to mcpauth.ErrInvalidToken so
// the SDK's bearer-token middleware emits a 401. The SDK writes err.Error()
// into the 401 response body, so the translation reconstructs the exact
// pre-extraction messages ("invalid token: <reason>", and a bare
// "invalid token" for an unknown subject — never an enumeration oracle).
//
// The JWKS URL is derived from the configured issuer as <issuer>/oauth2/jwks
// (see authkit.New).
package mcptoken

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/vibexp/vibexp/internal/auth/authkit"
)

// errUserResolution signals that resolving the subject to an internal user
// failed for an infrastructure reason (e.g. a transient database error). It does
// NOT unwrap to mcpauth.ErrInvalidToken, so the bearer-token middleware maps it
// to a 500 rather than a 401. A genuinely unknown subject is an auth failure and
// is reported as ErrInvalidToken instead.
var errUserResolution = errors.New("mcptoken: user resolution failed")

// clockSkewLeeway is the tolerance applied to the exp and nbf checks to absorb
// minor clock drift between AuthKit and this server.
const clockSkewLeeway = authkit.ClockSkewLeeway

// UserResolver resolves a token subject to an internal VibeXP user. It is
// satisfied by an adapter over repositories.UserRepository.GetByIDPSubject.
type UserResolver = authkit.UserResolver

// Verifier validates AuthKit-issued access tokens for a single MCP resource.
type Verifier struct {
	inner *authkit.Verifier
}

// New constructs a Verifier. issuer and resourceURI must be non-empty;
// resourceURI is the RFC 8707 audience this server accepts.
func New(ctx context.Context, issuer, resourceURI string, resolver UserResolver) (*Verifier, error) {
	if resourceURI == "" {
		return nil, fmt.Errorf("mcptoken: resource URI is required")
	}
	inner, err := authkit.New(ctx, issuer, authkit.RequireAudience(resourceURI), resolver)
	if err != nil {
		return nil, fmt.Errorf("mcptoken: %w", err)
	}
	return &Verifier{inner: inner}, nil
}

// Verify implements auth.TokenVerifier. On any validation failure it returns an
// error that unwraps to auth.ErrInvalidToken so the bearer-token middleware
// emits a 401; infrastructure failures during subject resolution stay distinct
// so they surface as a 500.
func (v *Verifier) Verify(ctx context.Context, token string, _ *http.Request) (*mcpauth.TokenInfo, error) {
	info, err := v.inner.Verify(ctx, token)
	if err != nil {
		return nil, translateError(err)
	}

	return &mcpauth.TokenInfo{
		UserID:     info.UserID,
		Scopes:     info.Scopes,
		Expiration: info.Expiration,
		Extra:      map[string]any{"sub": info.Subject},
	}, nil
}

// translateError maps authkit errors to the MCP SDK contract while preserving
// the exact pre-extraction 401 body strings (the SDK writes err.Error() to the
// response): infrastructure failures stay non-ErrInvalidToken (→ 500), an
// unknown subject yields a bare "invalid token" (no enumeration oracle), and
// every other rejection keeps its reason with the authkit prefix stripped.
func translateError(err error) error {
	if errors.Is(err, authkit.ErrUserResolution) {
		return errUserResolution
	}
	if errors.Is(err, authkit.ErrUnknownSubject) {
		return fmt.Errorf("%w", mcpauth.ErrInvalidToken)
	}
	reason := strings.TrimPrefix(err.Error(), authkit.ErrInvalidToken.Error()+": ")
	return fmt.Errorf("%w: %s", mcpauth.ErrInvalidToken, reason)
}
