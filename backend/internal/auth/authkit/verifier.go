// Package authkit verifies OAuth 2.1 issuer-signed JWT access tokens. It is the
// shared core behind both the MCP resource server (internal/auth/mcptoken) and
// the /api/v1 bearer-token middleware: JWKS-backed signature verification, an
// RS256 algorithm pin, registered-claim validation with clock-skew leeway, a
// pluggable audience policy, and resolution of the token subject to an
// internal VibeXP user ID.
package authkit

import (
	"context"
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
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

// ErrKeyRetrieval signals that obtaining the verification keys failed for an
// infrastructure reason (e.g. a transient database error while an in-process key
// set reads the Authorization Server's signing keys — see NewLocalKeySet). Like
// ErrUserResolution it is NOT an authentication failure: callers map it to a
// 500, never a 401. Collapsing it into ErrInvalidToken would tell the client its
// token is invalid (RFC 6750 §3.1), pushing every MCP client into a spurious
// re-auth loop on a transient key-store blip.
var ErrKeyRetrieval = errors.New("authkit: verification key retrieval failed")

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

// UserResolver resolves a token subject to an internal VibeXP user. It is
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
	keys     oidc.KeySet
	issuer   string
	audience AudiencePolicy
	resolver UserResolver
}

// Option customizes a Verifier constructed by New.
type Option func(*options)

type options struct {
	keySet oidc.KeySet
}

// WithKeySet overrides the default JWKS-over-HTTP key set with a caller-supplied
// one. VibeXP's embedded Authorization Server uses this to verify its own tokens
// with in-process public keys (see NewLocalKeySet) instead of an HTTP round-trip
// to <issuer>/oauth2/jwks.json — that public issuer URL need not be reachable
// from within the server (e.g. when the container publishes a host port that
// differs from the one it listens on). The issuer string is still used for the
// `iss` claim check.
func WithKeySet(ks oidc.KeySet) Option {
	return func(o *options) { o.keySet = ks }
}

// New constructs a Verifier. By default it creates a caching JWKS key set pointed
// at the issuer's JWKS endpoint (<issuer>/oauth2/jwks.json); pass WithKeySet to
// verify against an in-process key set instead. issuer must be non-empty;
// audience and resolver must be non-nil.
func New(
	ctx context.Context,
	issuer string,
	audience AudiencePolicy,
	resolver UserResolver,
	opts ...Option,
) (*Verifier, error) {
	if issuer == "" {
		return nil, fmt.Errorf("authkit: issuer is required")
	}
	if audience == nil {
		return nil, fmt.Errorf("authkit: audience policy is required")
	}
	if resolver == nil {
		return nil, fmt.Errorf("authkit: user resolver is required")
	}

	var cfg options
	for _, opt := range opts {
		opt(&cfg)
	}
	keys := cfg.keySet
	if keys == nil {
		keys = oidc.NewRemoteKeySet(ctx, jwksURL(issuer))
	}

	return &Verifier{
		keys:     keys,
		issuer:   issuer,
		audience: audience,
		resolver: resolver,
	}, nil
}

const (
	// localKeySetTTL bounds how long the cached verification keys are served
	// before a proactive re-fetch. It caps how long a just-pruned key stays
	// accepted and is far under any access-token TTL.
	localKeySetTTL = 60 * time.Second
	// localKeySetRefreshBackoff rate-limits the reactive re-fetch triggered when a
	// token fails to verify against the cached keys (typically a just-rotated
	// signing key not yet fetched). It bounds key-store load: a flood of invalid
	// tokens forces at most one re-fetch per backoff window.
	localKeySetRefreshBackoff = 5 * time.Second
)

// NewLocalKeySet builds an oidc.KeySet that verifies RS256 JWTs against public
// keys obtained in-process (no HTTP request), caching them the way
// oidc.RemoteKeySet does. getKeys must return the verification public keys
// currently trusted for the issuer — for the embedded Authorization Server that
// is its active key plus any retired-but-not-pruned keys, so a token signed just
// before a key rotation still verifies.
//
// The keys are cached for localKeySetTTL and re-fetched proactively after that;
// a verification miss (e.g. a brand-new key from a rotation) triggers a
// rate-limited reactive re-fetch, so a rotation is picked up within
// localKeySetRefreshBackoff — WITHOUT a key-store round-trip on every request.
// That matters because Verify reaches the key set before the issuer, audience,
// and subject checks, so an uncached fetch would let any unauthenticated caller
// sending an RS256-shaped JWT force a database read per request.
func NewLocalKeySet(getKeys func(context.Context) ([]crypto.PublicKey, error)) oidc.KeySet {
	return &localKeySet{getKeys: getKeys}
}

type localKeySet struct {
	getKeys func(context.Context) ([]crypto.PublicKey, error)

	mu        sync.Mutex
	cached    []crypto.PublicKey
	fetchedAt time.Time
	loaded    bool
}

// load returns the cached keys, re-fetching from getKeys only when the cache is
// empty or older than maxAge. The bool reports whether a fetch actually ran (so
// callers can avoid retrying a verification against unchanged keys).
func (l *localKeySet) load(ctx context.Context, maxAge time.Duration) ([]crypto.PublicKey, bool, error) {
	l.mu.Lock()
	if l.loaded && time.Since(l.fetchedAt) < maxAge {
		keys := l.cached
		l.mu.Unlock()
		return keys, false, nil
	}
	l.mu.Unlock()

	// Fetch outside the lock so a slow key store does not serialize verifications.
	keys, err := l.getKeys(ctx)
	if err != nil {
		return nil, false, err
	}
	l.mu.Lock()
	l.cached, l.fetchedAt, l.loaded = keys, time.Now(), true
	l.mu.Unlock()
	return keys, true, nil
}

func (l *localKeySet) VerifySignature(ctx context.Context, token string) ([]byte, error) {
	keys, _, err := l.load(ctx, localKeySetTTL)
	if err != nil {
		return nil, l.retrievalError(ctx, err)
	}
	payload, verifyErr := (&oidc.StaticKeySet{PublicKeys: keys}).VerifySignature(ctx, token)
	if verifyErr == nil {
		return payload, nil
	}
	// The signature matched no cached key. A rotation may have added a signing key
	// we have not fetched yet: re-fetch (rate-limited) and retry once. If nothing
	// was re-fetched (cache still within the backoff window) the token is simply
	// invalid — do not hit the key store again.
	refreshed, refetched, err := l.load(ctx, localKeySetRefreshBackoff)
	if err != nil {
		return nil, l.retrievalError(ctx, err)
	}
	if !refetched {
		return nil, verifyErr
	}
	return (&oidc.StaticKeySet{PublicKeys: refreshed}).VerifySignature(ctx, token)
}

// retrievalError logs the underlying key-store failure (with detail) and returns
// the opaque ErrKeyRetrieval sentinel, so Verify keeps it out of ErrInvalidToken
// and it surfaces as a 5xx rather than a 401.
func (l *localKeySet) retrievalError(ctx context.Context, err error) error {
	contextkeys.GetLoggerFromContext(ctx).
		Error("AuthKit in-process key retrieval failed (infrastructure error)", "error", err)
	return ErrKeyRetrieval
}

// jwksURL returns the JWKS endpoint for an issuer. VibeXP's embedded OAuth 2.1
// Authorization Server publishes its keys at <issuer>/oauth2/jwks.json (the
// jwks_uri advertised in its RFC 8414 metadata), so the verifier must request
// that exact path.
func jwksURL(issuer string) string {
	return issuer + "/oauth2/jwks.json"
}

// Verify validates the token signature and claims and resolves the subject to
// an internal user. Authentication failures unwrap to ErrInvalidToken;
// infrastructure failures unwrap to ErrKeyRetrieval (obtaining the verification
// keys) or ErrUserResolution (resolving the subject). Error messages stay opaque
// — no claim or infrastructure detail is included beyond the failure category.
func (v *Verifier) Verify(ctx context.Context, token string) (*TokenInfo, error) {
	if err := assertSigningAlg(token); err != nil {
		return nil, err
	}

	payload, err := v.keys.VerifySignature(ctx, token)
	if err != nil {
		// A failure to obtain the verification keys (e.g. an in-process key set
		// hitting a transient DB error) is infrastructure, not a bad token: keep
		// it as ErrKeyRetrieval (→ 500) rather than collapsing it into a 401.
		if errors.Is(err, ErrKeyRetrieval) {
			return nil, err
		}
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
	userID, err := v.resolver.ResolveUserID(ctx, string(idp.ProviderOIDC), subject)
	if err != nil {
		if errors.Is(err, repositories.ErrUserNotFound) {
			return "", ErrUnknownSubject
		}
		contextkeys.GetLoggerFromContext(ctx).
			Error("AuthKit token subject resolution failed (infrastructure error)", "error", err)
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
