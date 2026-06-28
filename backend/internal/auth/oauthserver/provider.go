package oauthserver

import (
	"context"
	"log/slog"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/models"
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
	Issuer              string // public base URL; token `iss` and metadata `issuer`
	ResourceURI         string // MCP resource URI; issued-token audience (RFC 8707)
	AccessTokenTTL      time.Duration
	RefreshTokenTTL     time.Duration
	AuthCodeTTL         time.Duration
	KeyRotationInterval time.Duration
	CleanupInterval     time.Duration // how often expired rows/retired keys are pruned
	// DevLoginEnabled allows the /authorize flow to authenticate via the
	// development-login bypass (no upstream IdP) when no identity providers are
	// configured. It MUST be the same hard dev gate as the /auth/dev/login endpoint
	// (development environment AND DEV_LOGIN_ENABLED); it is never set in
	// production, so the dev-login authenticator is unreachable there.
	DevLoginEnabled bool
}

// UserProvisioner resolves and provisions a VibeXP user. ProvisionFromClaims runs
// the same create-on-first-login logic as the web login flow (federated leg);
// HandleDevLogin provisions/reuses the development user, mirroring the
// /auth/dev/login endpoint, for the dev-login authorize bypass. AuthService
// satisfies it.
type UserProvisioner interface {
	ProvisionFromClaims(ctx context.Context, providerName string, claims *idp.Claims) (*models.User, error)
	HandleDevLogin(ctx context.Context, email, name string) (*models.User, error)
}

// Service is the embedded OAuth 2.1 Authorization Server.
type Service struct {
	provider      fosite.OAuth2Provider
	keys          *KeyManager
	store         *Store
	clients       repositories.OAuthClientRepository
	loginSessions repositories.OAuthLoginSessionRepository
	registry      *idp.Registry
	provisioner   UserProvisioner
	cfg           Config
	consentMACKey []byte
	logger        *slog.Logger
}

// NewService wires the Authorization Server. encKey is the 32-byte app encryption
// key (seals signing keys at rest and derives fosite's HMAC global secret).
func NewService(
	cfg Config,
	encKey []byte,
	clients repositories.OAuthClientRepository,
	codes, access, refresh, pkce repositories.OAuthRequestRepository,
	signingKeys repositories.OAuthSigningKeyRepository,
	loginSessions repositories.OAuthLoginSessionRepository,
	registry *idp.Registry,
	provisioner UserProvisioner,
	logger *slog.Logger,
) *Service {
	store := NewStore(clients, codes, access, refresh, pkce)
	keys := NewKeyManager(signingKeys, encKey, cfg.KeyRotationInterval)
	fc := newFositeConfig(cfg, encKey)

	return &Service{
		provider:      buildProvider(fc, store, keys),
		keys:          keys,
		store:         store,
		clients:       clients,
		loginSessions: loginSessions,
		registry:      registry,
		provisioner:   provisioner,
		cfg:           cfg,
		consentMACKey: deriveSecret(encKey, consentMACTag),
		logger:        logger,
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
