package oauthserver

import (
	"context"
	"log/slog"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"

	"github.com/vibexp/vibexp/internal/repositories"
)

const (
	// HMAC domain-separation tags (constant labels, not credentials).
	globalSecretTag = "vx-oauth-as-global-hmac-v1" // #nosec G101 -- domain-separation tag, not a credential
	consentMACTag   = "vx-oauth-as-consent-mac-v1"

	// defaultCleanupInterval is used when no retention interval is configured.
	defaultCleanupInterval = time.Hour
)

// Config holds the Authorization Server settings derived from app config.
type Config struct {
	Issuer      string // public base URL; token `iss` and metadata `issuer`
	ResourceURI string // MCP resource URI; issued-token audience (RFC 8707)
	// FrontendBaseURL is the SPA origin the post-login browser is redirected to
	// for the consent screen (`<FrontendBaseURL>/oauth/consent`). The page then
	// drives the JSON consent API; it is not the OAuth issuer origin.
	FrontendBaseURL     string
	AccessTokenTTL      time.Duration
	RefreshTokenTTL     time.Duration
	AuthCodeTTL         time.Duration
	KeyRotationInterval time.Duration
	CleanupInterval     time.Duration // how often expired rows/retired keys are pruned
}

// ConsentAccessPolicy re-checks, at consent time, whether a user may still be
// bound to a login session (issue #217).
//
// The AS authenticates nobody itself — it piggybacks on the app's vx_session, so
// allowlist enforcement at login (#215) covers it only transitively. Because that
// enforcement is login-time only, a user removed from the allowlist keeps a valid
// session until its TTL expires and could still authorize NEW MCP clients in that
// window. ConsentAttach is the single choke point where a user's identity is bound
// to an authorization request, so re-checking here closes that window for MCP
// token issuance.
//
// It is deliberately one method over a user id: the AS must not grow a dependency
// on the services layer to answer it. Implementations resolve the user's email and
// apply the configured access allowlist. A nil policy disables the re-check.
type ConsentAccessPolicy interface {
	// AllowUser reports whether userID may be bound to a consent session. A
	// non-nil error means the decision could not be made; callers fail closed.
	AllowUser(ctx context.Context, userID string) (bool, error)
}

// Service is the embedded OAuth 2.1 Authorization Server.
type Service struct {
	provider      fosite.OAuth2Provider
	keys          *KeyManager
	store         *Store
	clients       repositories.OAuthClientRepository
	loginSessions repositories.OAuthLoginSessionRepository
	access        ConsentAccessPolicy
	cfg           Config
	consentMACKey []byte
	logger        *slog.Logger
}

// Dependencies bundles the storage and policy collaborators NewService wires
// into the Authorization Server.
type Dependencies struct {
	Clients       repositories.OAuthClientRepository
	Codes         repositories.OAuthRequestRepository
	AccessTokens  repositories.OAuthRequestRepository
	RefreshTokens repositories.OAuthRequestRepository
	PKCE          repositories.OAuthRequestRepository
	SigningKeys   repositories.OAuthSigningKeyRepository
	LoginSessions repositories.OAuthLoginSessionRepository
	// AccessPolicy re-checks the access allowlist at consent-attach; leave nil to
	// disable the re-check (an unconfigured allowlist is open access anyway).
	AccessPolicy ConsentAccessPolicy
	Logger       *slog.Logger
}

// NewService wires the Authorization Server. encKey is the 32-byte app encryption
// key (seals signing keys at rest and derives fosite's HMAC global secret).
func NewService(cfg Config, encKey []byte, deps Dependencies) *Service {
	store := NewStore(deps.Clients, deps.Codes, deps.AccessTokens, deps.RefreshTokens, deps.PKCE)
	keys := NewKeyManager(deps.SigningKeys, encKey, cfg.KeyRotationInterval)
	fc := newFositeConfig(cfg, encKey)

	return &Service{
		provider:      buildProvider(fc, store, keys),
		keys:          keys,
		store:         store,
		clients:       deps.Clients,
		loginSessions: deps.LoginSessions,
		access:        deps.AccessPolicy,
		cfg:           cfg,
		consentMACKey: deriveSecret(encKey, consentMACTag),
		logger:        deps.Logger,
	}
}

// newFositeConfig builds the fosite configuration enforcing OAuth 2.1 norms:
// mandatory PKCE with S256 only, JWT access tokens issued under our issuer, and
// no debug leakage to clients.
func newFositeConfig(cfg Config, encKey []byte) *fosite.Config {
	return &fosite.Config{
		AccessTokenLifespan:            cfg.AccessTokenTTL,
		RefreshTokenLifespan:           cfg.RefreshTokenTTL,
		AuthorizeCodeLifespan:          cfg.AuthCodeTTL,
		GlobalSecret:                   deriveSecret(encKey, globalSecretTag),
		EnforcePKCE:                    true,
		EnforcePKCEForPublicClients:    true,
		EnablePKCEPlainChallengeMethod: false,
		AccessTokenIssuer:              cfg.Issuer,
		// Empty (non-nil) means every authorization-code exchange yields a refresh
		// token; MCP clients need durable sessions and do not request offline_access.
		RefreshTokenScopes:         []string{},
		ScopeStrategy:              fosite.ExactScopeStrategy,
		AudienceMatchingStrategy:   fosite.DefaultAudienceMatchingStrategy,
		SendDebugMessagesToClients: false,
	}
}

// buildProvider composes the minimal OAuth 2.1 handler set: authorization-code
// grant, refresh-token grant (rotation + reuse detection), and PKCE. Implicit,
// ROPC, client-credentials, and OIDC id_token flows are intentionally excluded —
// MCP needs only audience-bound access tokens.
func buildProvider(fc *fosite.Config, store *Store, keys *KeyManager) fosite.OAuth2Provider {
	hmacStrategy := compose.NewOAuth2HMACStrategy(fc)
	jwtStrategy := compose.NewOAuth2JWTStrategy(keys.PrivateKeyGetter(), hmacStrategy, fc)
	return compose.Compose(fc, store, jwtStrategy,
		compose.OAuth2AuthorizeExplicitFactory,
		compose.OAuth2RefreshTokenGrantFactory,
		compose.OAuth2PKCEFactory,
	)
}

// Start ensures an active signing key exists and runs periodic key rotation until
// the context is cancelled. Intended to be launched in a goroutine at startup.
func (s *Service) Start(ctx context.Context) error {
	if err := s.keys.EnsureActiveKey(ctx); err != nil {
		return err
	}
	go s.rotateLoop(ctx)
	go s.cleanupLoop(ctx)
	return nil
}

// cleanupLoop periodically prunes expired authorization codes, tokens, PKCE and
// login sessions plus retired signing keys, until the context is cancelled. It
// runs one sweep immediately so a fresh instance does not wait a full interval.
// Individual failures are logged and never stop the loop.
func (s *Service) cleanupLoop(ctx context.Context) {
	interval := s.cfg.CleanupInterval
	if interval <= 0 {
		interval = defaultCleanupInterval
	}
	s.cleanupOnce(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupOnce(ctx)
		}
	}
}

// cleanupOnce runs a single retention sweep across all OAuth AS storage.
func (s *Service) cleanupOnce(ctx context.Context) {
	if n, err := s.store.DeleteExpired(ctx); err != nil {
		s.logger.With("error", err).Error("oauth AS expired token cleanup failed")
	} else if n > 0 {
		s.logger.With("removed", n).Info("oauth AS pruned expired token rows")
	}
	if n, err := s.loginSessions.DeleteExpired(ctx); err != nil {
		s.logger.With("error", err).Error("oauth AS expired login-session cleanup failed")
	} else if n > 0 {
		s.logger.With("removed", n).Info("oauth AS pruned expired login sessions")
	}
	if n, err := s.keys.PruneRetired(ctx, s.cfg.RefreshTokenTTL); err != nil {
		s.logger.With("error", err).Error("oauth AS retired signing-key pruning failed")
	} else if n > 0 {
		s.logger.With("removed", n).Info("oauth AS pruned retired signing keys")
	}
}

func (s *Service) rotateLoop(ctx context.Context) {
	ticker := time.NewTicker(rotateCheckInterval(s.cfg.KeyRotationInterval))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.keys.MaybeRotate(ctx); err != nil {
				s.logger.With("error", err).Error("oauth AS signing-key rotation failed")
			}
		}
	}
}

// rotateCheckInterval checks at roughly a tenth of the rotation interval, bounded
// to [1h, 24h], so rotation fires promptly without busy-looping.
func rotateCheckInterval(interval time.Duration) time.Duration {
	check := interval / 10
	if check < time.Hour {
		return time.Hour
	}
	if check > 24*time.Hour {
		return 24 * time.Hour
	}
	return check
}
