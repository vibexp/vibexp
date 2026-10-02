package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Authentication setup mode (#1236, epic #1230, decision 10).
//
// Sign-in providers live in the database and are configured from the admin
// panel, which itself requires signing in. An instance with no enabled provider
// therefore has no way in. While that holds the instance is in SETUP MODE: at
// boot it mints a one-time setup token and logs a setup URL; whoever holds the
// URL exchanges the token for a short-lived, userless setup session that may
// reach the authentication settings and nothing else. The first provider
// sign-in by a root instance admin (auth.instance_admins) consumes the token and
// ends setup.

const (
	// SetupTokenLifetime is how long a minted setup token stays exchangeable.
	SetupTokenLifetime = 24 * time.Hour

	// setupTokenBytes is the token's entropy: 32 random bytes, encoded as 43
	// unpadded URL-safe base64 characters.
	setupTokenBytes = 32

	// setupTokenMaxLength bounds the candidate hashed by ExchangeToken, so an
	// oversized body costs nothing. It is far above the real token's length.
	setupTokenMaxLength = 256

	// setupPagePath is the SPA route the setup URL points at.
	setupPagePath = "/setup"
)

var (
	// ErrSetupNotActive is returned by ExchangeToken when the instance is not
	// in setup mode.
	ErrSetupNotActive = errors.New("authentication setup mode is not active")
	// ErrSetupTokenInvalid is returned by ExchangeToken for every rejected
	// token — unknown, expired or already consumed — so a caller cannot tell
	// which.
	ErrSetupTokenInvalid = errors.New("invalid setup token")
	// ErrSetupSessionInvalid is returned by ValidateSession for a setup session
	// that is expired, was issued before the current token, or whose setup has
	// been consumed.
	ErrSetupSessionInvalid = errors.New("invalid setup session")
)

// SetupModeService owns the setup-mode state machine over the
// instance_auth_setup singleton.
type SetupModeService interface {
	// IsActive reports whether the instance is in setup mode: the recovery
	// input is set, OR setup was re-armed and not yet consumed, OR no identity
	// provider is enabled. It is computed on every call, never stored.
	IsActive(ctx context.Context) (bool, error)
	// EnsureTokenAtBoot mints a setup token when setup mode is active and no
	// exchangeable token is stored, logs the setup URL at WARN, and returns it
	// with minted = true. When another boot's token is still valid it mints
	// nothing — the token cannot be re-logged, only its hash is stored — and
	// logs when it was issued instead. When setup mode is not active it does
	// nothing. With the recovery input set it always mints, replacing any
	// outstanding token, so every boot logs a usable URL.
	EnsureTokenAtBoot(ctx context.Context) (setupURL string, minted bool, err error)
	// ExchangeToken trades the setup token for a setup session. It returns
	// ErrSetupNotActive outside setup mode and ErrSetupTokenInvalid for any
	// rejected token. The token stays exchangeable until it expires or is
	// consumed, so an operator who loses the session can use the URL again.
	ExchangeToken(ctx context.Context, token string) (*session.SetupSession, error)
	// ValidateSession reports ErrSetupSessionInvalid unless sess is unexpired
	// and was issued for the stored, unconsumed setup. A session stays valid
	// after the first provider is enabled — until a root admin signs in — so a
	// mistyped provider can still be corrected.
	ValidateSession(ctx context.Context, sess *session.SetupSession) error
	// ConsumeOnRootLogin ends setup when user is a root instance admin and a
	// setup is outstanding; for anyone else, or with nothing outstanding, it
	// does nothing.
	ConsumeOnRootLogin(ctx context.Context, user *models.User) error
	// ForceRearm mints a new token unconditionally, replacing any outstanding
	// one, and keeps setup mode active until it is consumed even though a
	// provider may be enabled. It returns the setup URL's token. It is the
	// input of the break-glass CLI (#1237).
	ForceRearm(ctx context.Context) (token string, err error)
}

// SetupModeDeps are the collaborators of the setup-mode service.
type SetupModeDeps struct {
	Setup     repositories.InstanceAuthSetupRepository
	Providers repositories.InstanceAuthProviderRepository
	// IsRootAdmin reports whether an email is a root instance admin
	// (InstanceAdminResolver.IsRootAdmin).
	IsRootAdmin func(email string) bool
	// BaseURL is frontend.base_url, the origin the setup URL is built on.
	BaseURL string
	// RecoveryMode forces setup mode on regardless of the stored providers. It
	// is the input of the recovery flag (#1237).
	RecoveryMode bool
	Logger       *slog.Logger
}

type setupModeService struct {
	setup        repositories.InstanceAuthSetupRepository
	providers    repositories.InstanceAuthProviderRepository
	isRootAdmin  func(email string) bool
	baseURL      string
	recoveryMode bool
	logger       *slog.Logger

	// Injectable for tests.
	now func() time.Time
}

var _ SetupModeService = (*setupModeService)(nil)

// NewSetupModeService creates the setup-mode service.
func NewSetupModeService(deps SetupModeDeps) SetupModeService {
	return newSetupModeService(deps)
}

func newSetupModeService(deps SetupModeDeps) *setupModeService {
	return &setupModeService{
		setup:        deps.Setup,
		providers:    deps.Providers,
		isRootAdmin:  deps.IsRootAdmin,
		baseURL:      strings.TrimRight(deps.BaseURL, "/"),
		recoveryMode: deps.RecoveryMode,
		logger:       deps.Logger,
		now:          time.Now,
	}
}

// Why setup mode is active. Each is the "reason" logged beside the setup URL.
const (
	setupReasonNoProvider = "no identity provider is enabled; open this URL to configure sign-in"
	setupReasonRearmed    = "authentication setup was re-armed with `vibexp admin auth setup rearm`; " +
		"open this URL to configure sign-in"
	setupReasonRecovery = "auth.recovery_mode (AUTH_SETTINGS_RECOVERY_MODE) is set; open this URL to " +
		"repair sign-in, then remove the flag and restart"
)

// IsActive implements SetupModeService.
func (s *setupModeService) IsActive(ctx context.Context) (bool, error) {
	reason, err := s.activeReason(ctx)
	return reason != "", err
}

// activeReason returns why setup mode is active, or "" when it is not.
func (s *setupModeService) activeReason(ctx context.Context) (string, error) {
	if s.recoveryMode {
		return setupReasonRecovery, nil
	}
	row, err := s.stored(ctx)
	if err != nil {
		return "", err
	}
	if row != nil && row.Rearmed && !row.IsConsumed() {
		return setupReasonRearmed, nil
	}
	enabled, err := s.hasEnabledProvider(ctx)
	if err != nil {
		return "", err
	}
	if enabled {
		return "", nil
	}
	return setupReasonNoProvider, nil
}

// hasEnabledProvider reads the provider ROWS rather than the resolver's built
// snapshot: the snapshot leaves out an enabled provider that failed to build,
// so judging by it would open setup mode whenever an identity provider had an
// outage.
func (s *setupModeService) hasEnabledProvider(ctx context.Context) (bool, error) {
	rows, err := s.providers.List(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to list identity providers: %w", err)
	}
	for _, row := range rows {
		if row.Enabled {
			return true, nil
		}
	}
	return false, nil
}

// stored returns the setup row, or nil when no token was ever issued.
func (s *setupModeService) stored(ctx context.Context) (*models.InstanceAuthSetup, error) {
	row, err := s.setup.Get(ctx)
	if errors.Is(err, repositories.ErrInstanceAuthSetupNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read authentication setup state: %w", err)
	}
	return row, nil
}

// EnsureTokenAtBoot implements SetupModeService.
func (s *setupModeService) EnsureTokenAtBoot(ctx context.Context) (string, bool, error) {
	reason, err := s.activeReason(ctx)
	if err != nil {
		return "", false, err
	}
	if reason == "" {
		return "", false, nil
	}

	token, hash, err := newSetupToken()
	if err != nil {
		return "", false, err
	}
	now := s.now()
	expiresAt := now.Add(SetupTokenLifetime)

	// Recovery mode exists to hand a locked-out operator a setup URL, and only
	// the hash of an earlier token is stored, so every boot replaces the token
	// rather than keeping one whose URL may be lost. It does not re-arm: setup
	// mode must end when the flag is removed.
	if s.recoveryMode {
		replaced, mintErr := s.setup.MintReplacing(ctx, hash, expiresAt)
		if mintErr != nil {
			return "", false, fmt.Errorf("failed to mint the recovery setup token: %w", mintErr)
		}
		return s.logSetupURL(token, reason, replaced), true, nil
	}

	row, minted, err := s.setup.MintIfAbsentOrExpired(ctx, hash, expiresAt, now)
	if err != nil {
		return "", false, fmt.Errorf("failed to mint the setup token: %w", err)
	}
	if !minted {
		s.logger.Warn("Authentication setup mode is active and a setup token is already issued; "+
			"only its hash is stored, so the setup URL cannot be shown again. Use the URL logged when it was "+
			"issued, restart after it expires, or run `vibexp admin auth setup rearm`",
			"issued_at", row.UpdatedAt, "valid_until", row.ExpiresAt)
		return "", false, nil
	}
	return s.logSetupURL(token, reason, row), true, nil
}

// logSetupURL logs the setup URL for a freshly minted token and returns it.
func (s *setupModeService) logSetupURL(token, reason string, row *models.InstanceAuthSetup) string {
	setupURL := SetupURL(s.baseURL, token)
	s.logger.Warn("SETUP URL: "+setupURL, "reason", reason, "valid_until", row.ExpiresAt)
	return setupURL
}

// SetupURL is the SPA setup page on baseURL (frontend.base_url) carrying token.
// The token is unpadded URL-safe base64, so it needs no escaping.
func SetupURL(baseURL, token string) string {
	return strings.TrimRight(baseURL, "/") + setupPagePath + "?token=" + token
}

// ExchangeToken implements SetupModeService.
func (s *setupModeService) ExchangeToken(ctx context.Context, token string) (*session.SetupSession, error) {
	active, err := s.IsActive(ctx)
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, ErrSetupNotActive
	}
	if token == "" || len(token) > setupTokenMaxLength {
		return nil, ErrSetupTokenInvalid
	}

	row, err := s.stored(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if row == nil || !row.HasLiveToken(now) {
		return nil, ErrSetupTokenInvalid
	}
	candidate := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(candidate[:], row.TokenHash) != 1 {
		return nil, ErrSetupTokenInvalid
	}

	// The session never outlives the token it was issued for.
	expiresAt := now.Add(session.SetupSessionLifetime)
	if row.ExpiresAt.Before(expiresAt) {
		expiresAt = *row.ExpiresAt
	}
	return &session.SetupSession{Generation: row.Generation, ExpiresAt: expiresAt}, nil
}

// ValidateSession implements SetupModeService.
func (s *setupModeService) ValidateSession(ctx context.Context, sess *session.SetupSession) error {
	if sess == nil || sess.IsExpired(s.now()) {
		return ErrSetupSessionInvalid
	}
	row, err := s.stored(ctx)
	if err != nil {
		return err
	}
	if row == nil || row.IsConsumed() || row.Generation != sess.Generation {
		return ErrSetupSessionInvalid
	}
	return nil
}

// ConsumeOnRootLogin implements SetupModeService.
func (s *setupModeService) ConsumeOnRootLogin(ctx context.Context, user *models.User) error {
	if user == nil || !s.isRootAdmin(user.Email) {
		return nil
	}
	// Read first: once setup is consumed this runs on every root sign-in, and
	// the common case should cost one primary-key read, not a locking write.
	row, err := s.stored(ctx)
	if err != nil {
		return err
	}
	if row == nil || row.IsConsumed() {
		return nil
	}
	consumed, err := s.setup.Consume(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to consume the setup token: %w", err)
	}
	if consumed {
		s.logger.Info("Authentication setup completed: a root instance admin signed in, the setup token is consumed",
			"user_id", user.ID)
	}
	return nil
}

// ForceRearm implements SetupModeService.
func (s *setupModeService) ForceRearm(ctx context.Context) (string, error) {
	return forceRearmSetup(ctx, s.setup, s.now())
}

// forceRearmSetup mints a re-arming setup token at now and returns it. It is a
// function rather than only a method so the break-glass CLI (#1237), which has
// no server and so no full setup-mode service, mints through the same code.
func forceRearmSetup(
	ctx context.Context, setup repositories.InstanceAuthSetupRepository, now time.Time,
) (string, error) {
	token, hash, err := newSetupToken()
	if err != nil {
		return "", err
	}
	if _, err := setup.ForceMint(ctx, hash, now.Add(SetupTokenLifetime)); err != nil {
		return "", fmt.Errorf("failed to re-arm authentication setup: %w", err)
	}
	return token, nil
}

// newSetupToken returns a fresh setup token and the SHA-256 that is stored in
// its place. A fast hash is right here: the token is 256 bits of randomness,
// not a password, so there is nothing to brute-force.
func newSetupToken() (token string, hash []byte, err error) {
	raw := make([]byte, setupTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("failed to generate setup token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	return token, sum[:], nil
}
