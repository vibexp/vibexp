// Package authkit verifies WorkOS AuthKit-issued JWT access tokens. It is the
// shared core behind both the MCP resource server (internal/auth/mcptoken) and
// the /api/v1 bearer-token middleware: JWKS-backed signature verification, an
// RS256 algorithm pin, registered-claim validation with clock-skew leeway, a
// pluggable audience policy, and resolution of the WorkOS subject to an
// internal VibeXP user ID.
package authkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrInvalidToken marks any verification failure that is an authentication
// failure (malformed token, bad signature, expired, wrong issuer or audience,
// unknown subject). Callers map it to a 401.
var ErrInvalidToken = errors.New("authkit: invalid token")

// ErrUserResolution signals that resolving the subject to an internal user
// failed for an infrastructure reason (e.g. a transient database error).
// Callers map it to a 500, never a 401. A genuinely unknown subject is an auth
// failure and is reported as ErrInvalidToken instead.
var ErrUserResolution = errors.New("authkit: user resolution failed")

// ErrUnknownSubject is the ErrInvalidToken sub-case for a cryptographically
// valid token whose subject does not resolve to a provisioned user. It is
// distinguishable so callers can keep their client-facing message fully opaque
// (a "valid token, unknown user" detail is an account-enumeration oracle).
var ErrUnknownSubject = fmt.Errorf("%w: unknown subject", ErrInvalidToken)

// ClockSkewLeeway is the tolerance applied to the exp and nbf checks to absorb
// minor clock drift between AuthKit and this server.
const ClockSkewLeeway = 60 * time.Second

// allowedSigningAlgs is the allow-list of JWS signing algorithms accepted for
// AuthKit access tokens. AuthKit signs with RS256; pinning the algorithm is
// defense-in-depth against algorithm-substitution attacks (e.g. "none", or an
// HMAC alg verified against public key material).
var allowedSigningAlgs = map[string]bool{"RS256": true}

// UserResolver resolves a WorkOS subject to an internal VibeXP user. It is
// satisfied by an adapter over repositories.UserRepository.GetByIDPSubject.
type UserResolver interface {
	ResolveUserID(ctx context.Context, provider, subject string) (string, error)
}

// AudiencePolicy validates the token's aud claim. The two resources this
// codebase protects have different audience realities: MCP tokens are minted
// with an RFC 8707 resource indicator and must be audience-bound, while plain
// AuthKit PKCE access tokens (web/mobile login) carry no aud claim at all.
type AudiencePolicy func(aud jwt.ClaimStrings) error

// RequireAudience requires the aud claim to contain the given resource URI
// (RFC 8707 audience binding). Used by the MCP resource server.
func RequireAudience(resource string) AudiencePolicy {
	return func(aud jwt.ClaimStrings) error {
		for _, a := range aud {
			if a == resource {
				return nil
			}
		}
		return fmt.Errorf("%w: token audience does not include the expected resource", ErrInvalidToken)
	}
}

// RequireAnyAudience requires the aud claim to contain at least one entry from
// the allow-list. Empty allow-list entries are ignored.
func RequireAnyAudience(allowed []string) AudiencePolicy {
	set := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		if a != "" {
			set[a] = true
		}
	}
	return func(aud jwt.ClaimStrings) error {
		for _, a := range aud {
			if set[a] {
				return nil
			}
		}
		return fmt.Errorf("%w: token audience is not in the allow-list", ErrInvalidToken)
	}
}

// AllowAnyAudience skips audience validation. Plain AuthKit access tokens from
// the PKCE login flow carry no aud claim (no RFC 8707 resource indicator is
// requested), so the API surface cannot require one; issuer, signature,
// expiry, and subject-to-user binding are still enforced.
func AllowAnyAudience() AudiencePolicy {
	return func(jwt.ClaimStrings) error { return nil }
}

// AllowAnyAudienceExcept accepts tokens with no aud claim or any audience NOT
// in the denied list. It keeps the AllowAnyAudience posture for audience-less
// PKCE tokens while rejecting tokens explicitly minted for another resource —
// e.g. the API surface denies MCP-resource-bound tokens so an MCP client's
// narrow grant cannot be replayed as a full API credential.
func AllowAnyAudienceExcept(denied ...string) AudiencePolicy {
	set := make(map[string]bool, len(denied))
	for _, d := range denied {
		if d != "" {
			set[d] = true
		}
	}
	return func(aud jwt.ClaimStrings) error {
		for _, a := range aud {
			if set[a] {
				return fmt.Errorf("%w: token audience is bound to another resource", ErrInvalidToken)
			}
		}
		return nil
	}
}

// claims holds the registered claims the verifier inspects. Audience uses the
// jwt library's ClaimStrings type, which decodes both the single-string and
// the array forms (RFC 7519 §4.1.3 / RFC 8707) — AuthKit may emit either.
type claims struct {
	Issuer    string           `json:"iss"`
	Subject   string           `json:"sub"`
	Audience  jwt.ClaimStrings `json:"aud"`
	ExpiresAt int64            `json:"exp"`
	NotBefore int64            `json:"nbf"`
	Scope     string           `json:"scope"`
}

// TokenInfo is the result of a successful verification: the resolved internal
// VibeXP user plus the token's subject, scopes, and expiry.
type TokenInfo struct {
	UserID     string
	Subject    string
	Scopes     []string
	Expiration time.Time
}

// Verifier validates AuthKit-issued access tokens for a single issuer and
// audience policy.
type Verifier struct {
	keys     *oidc.RemoteKeySet
	issuer   string
	audience AudiencePolicy
	resolver UserResolver
}

// New constructs a Verifier. It creates a caching JWKS key set pointed at the
// AuthKit JWKS endpoint (<issuer>/oauth2/jwks). issuer must be non-empty;
// audience and resolver must be non-nil.
func New(ctx context.Context, issuer string, audience AudiencePolicy, resolver UserResolver) (*Verifier, error) {
	if issuer == "" {
		return nil, fmt.Errorf("authkit: issuer is required")
	}
	if audience == nil {
		return nil, fmt.Errorf("authkit: audience policy is required")
	}
	if resolver == nil {
		return nil, fmt.Errorf("authkit: user resolver is required")
	}

	return &Verifier{
		keys:     oidc.NewRemoteKeySet(ctx, jwksURL(issuer)),
		issuer:   issuer,
		audience: audience,
		resolver: resolver,
	}, nil
}

// workosUserManagementPrefix is the base of WorkOS's environment-scoped issuer
// form (https://api.workos.com/user_management/<client_id>), carried by access
// tokens minted via the user_management authorize/authenticate endpoints — the
// flow native PKCE clients (mobile) use.
const workosUserManagementPrefix = "https://api.workos.com/user_management/"

// jwksURL returns the JWKS endpoint for a WorkOS issuer. AuthKit domains serve
// keys at <issuer>/oauth2/jwks; the api.workos.com user_management issuer form
// serves them at https://api.workos.com/sso/jwks/<client_id> instead (per its
// OIDC discovery document). Deriving the URL here lets both issuer forms verify
// without extra configuration.
func jwksURL(issuer string) string {
	clientID, ok := strings.CutPrefix(issuer, workosUserManagementPrefix)
	if ok && clientID != "" && !strings.Contains(clientID, "/") {
		return "https://api.workos.com/sso/jwks/" + clientID
	}
	return issuer + "/oauth2/jwks"
}

// Verify validates the token signature and claims and resolves the subject to
// an internal user. Authentication failures unwrap to ErrInvalidToken;
// infrastructure failures during subject resolution unwrap to
// ErrUserResolution. Error messages stay opaque — no claim or infrastructure
// detail is included beyond the failure category.
func (v *Verifier) Verify(ctx context.Context, token string) (*TokenInfo, error) {
	if err := assertSigningAlg(token); err != nil {
		return nil, err
	}

	payload, err := v.keys.VerifySignature(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("%w: signature verification failed", ErrInvalidToken)
	}

	var c claims
	if err = json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("%w: malformed claims", ErrInvalidToken)
	}

	if err = v.validateClaims(&c); err != nil {
		return nil, err
	}

	userID, err := v.resolveUserID(ctx, c.Subject)
	if err != nil {
		return nil, err
	}

	return &TokenInfo{
		UserID:     userID,
		Subject:    c.Subject,
		Scopes:     strings.Fields(c.Scope),
		Expiration: time.Unix(c.ExpiresAt, 0),
	}, nil
}

func (v *Verifier) validateClaims(c *claims) error {
	now := time.Now()
	if c.Issuer != v.issuer {
		return fmt.Errorf("%w: unexpected issuer", ErrInvalidToken)
	}
	if c.ExpiresAt == 0 {
		return fmt.Errorf("%w: token missing expiration", ErrInvalidToken)
	}
	if now.Add(-ClockSkewLeeway).After(time.Unix(c.ExpiresAt, 0)) {
		return fmt.Errorf("%w: token expired", ErrInvalidToken)
	}
	if c.NotBefore != 0 && now.Add(ClockSkewLeeway).Before(time.Unix(c.NotBefore, 0)) {
		return fmt.Errorf("%w: token not yet valid", ErrInvalidToken)
	}
	if c.Subject == "" {
		return fmt.Errorf("%w: token missing subject", ErrInvalidToken)
	}
	return v.audience(c.Audience)
}

// resolveUserID maps the token subject to an internal user ID. It distinguishes
// a genuinely unknown subject (an auth failure → ErrInvalidToken) from an
// infrastructure error during the lookup (→ ErrUserResolution). Client-facing
// errors stay opaque; the subject and any underlying detail are kept out of the
// returned message and surface only in the server logs.
func (v *Verifier) resolveUserID(ctx context.Context, subject string) (string, error) {
	userID, err := v.resolver.ResolveUserID(ctx, string(idp.ProviderWorkOS), subject)
	if err != nil {
		if errors.Is(err, repositories.ErrUserNotFound) {
			return "", ErrUnknownSubject
		}
		contextkeys.GetLoggerFromContext(ctx).WithError(err).
			Error("AuthKit token subject resolution failed (infrastructure error)")
		return "", ErrUserResolution
	}
	if userID == "" {
		return "", ErrUnknownSubject
	}
	return userID, nil
}

// assertSigningAlg rejects a token whose JWS header "alg" is not in the
// allow-list. go-oidc's VerifySignature does not enforce alg, so this guards
// against algorithm-substitution ("none", HMAC) at the application layer.
func assertSigningAlg(token string) error {
	parser := jwt.NewParser()
	parsed, _, err := parser.ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return fmt.Errorf("%w: malformed token", ErrInvalidToken)
	}
	alg, _ := parsed.Header["alg"].(string)
	if !allowedSigningAlgs[alg] {
		return fmt.Errorf("%w: unsupported signing algorithm", ErrInvalidToken)
	}
	return nil
}
