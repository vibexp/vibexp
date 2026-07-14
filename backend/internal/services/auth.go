package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services/feature_flags"
	"github.com/vibexp/vibexp/pkg/events"
)

// AuthService handles authentication operations. It dispatches web login to
// one of the identity providers held in the registry, selected per-request by
// the provider name carried through the login/callback flow. Tokens are
// delivered via AES-GCM encrypted httpOnly cookies managed by the session
// package — HS256 JWT signing is removed in this release.
type AuthService struct {
	userRepo       repositories.UserRepository
	registry       *idp.Registry
	featureFlagSvc feature_flags.FeatureFlagServiceInterface
	eventManager   events.EventPublisher
	logger         *slog.Logger
}

// Ensure AuthService implements AuthServiceInterface
var _ AuthServiceInterface = (*AuthService)(nil)

// ErrAccessRestricted is returned by the login entry points when the
// authenticated email is not permitted by the configured access allowlist. The
// HTTP layer branches on it with errors.Is to surface a policy denial (redirect
// for the OAuth callback, 403 for dev login) distinct from other failures.
var ErrAccessRestricted = errors.New("access restricted by allowlist")

// ensureAccessAllowed denies sign-in when email is not permitted by the access
// allowlist, returning ErrAccessRestricted. An unconfigured allowlist (both
// lists empty) is open access and always allowed. The decision is evaluated
// through the user_signin_allowlist feature flag (which reads the email from
// context); provider is included in the audit log for operator traceability.
func (as *AuthService) ensureAccessAllowed(ctx context.Context, email, provider string) error {
	ctx = context.WithValue(ctx, feature_flags.EmailContextKey, email)
	if as.featureFlagSvc.IsEnabled(ctx, feature_flags.FlagUserSignInAllowlist) {
		return nil
	}
	as.logger.With(
		"email", email,
		"provider", provider,
	).Info("Sign-in denied by access allowlist")
	return ErrAccessRestricted
}

func NewAuthService(
	userRepo repositories.UserRepository, registry *idp.Registry,
	eventManager events.EventPublisher, logger *slog.Logger,
	featureFlagSvc feature_flags.FeatureFlagServiceInterface,
) *AuthService {
	return &AuthService{
		userRepo:       userRepo,
		registry:       registry,
		featureFlagSvc: featureFlagSvc,
		eventManager:   eventManager,
		logger:         logger,
	}
}

// EnabledProviders returns the canonical names of the enabled login providers,
// stable-sorted, for the HTTP layer to validate the ?provider= hint against
// and to surface the available choices.
func (as *AuthService) EnabledProviders() []string {
	enabled := as.registry.Enabled()
	names := make([]string, len(enabled))
	for i, n := range enabled {
		names[i] = string(n)
	}
	return names
}

// GetLoginURL returns the authorization URL for the named provider. It returns
// an empty string when the provider is not enabled, so the caller can surface
// a "provider unavailable" response. Each provider uses its own configured
// redirect URI (the empty override here means "use the provider's default").
func (as *AuthService) GetLoginURL(state, provider string) string {
	p, ok := as.registry.Get(idp.ProviderName(provider))
	if !ok {
		return ""
	}
	return p.AuthorizeURL(state, "", provider)
}

// HandleCallback exchanges the authorization code for tokens using the named
// provider, looks up or creates the user, and returns the user, IDP tokens,
// and whether they are new. The provider name is resolved from the signed
// state cookie by the HTTP layer.
func (as *AuthService) HandleCallback(
	ctx context.Context, code, provider string,
) (*models.User, *idp.Tokens, bool, error) {
	as.logger.With("provider", provider).Info("Processing OAuth callback")

	p, ok := as.registry.Get(idp.ProviderName(provider))
	if !ok {
		return nil, nil, false, fmt.Errorf("unknown identity provider %q", provider)
	}

	tokens, claims, err := p.ExchangeCode(ctx, code, "")
	if err != nil {
		as.logger.With("error", err).Error("Failed to exchange OAuth token")
		return nil, nil, false, fmt.Errorf("failed to exchange token: %w", err)
	}

	// Email and name are PII; INFO-level logging is consistent with the
	// rest of the auth service. See L10 in the GDPR audit if logging tier
	// changes are required (e.g., move to DEBUG or scrub).
	as.logger.With(
		"email", claims.Email,
		"name", claims.Name,
		"email_verified", claims.EmailVerified,
	).
		Info("Retrieved user info from identity provider")

	// L2: We do not currently reject !EmailVerified. The enabled providers
	// (Google/GitHub/generic OIDC) enforce email verification at the provider
	// level, so every claim arriving here has already been verified. If we add
	// connections where this is not true (e.g., magic link without
	// verification), gate sign-in here:
	//   if !claims.EmailVerified { return ErrUnverifiedEmail }

	// Enforce the access allowlist BEFORE any user row is created or updated, so
	// a denied identity leaves zero database residue and emits no user.created
	// event.
	if err = as.ensureAccessAllowed(ctx, claims.Email, string(p.Name())); err != nil {
		return nil, nil, false, err
	}

	user, isNewUser, err := as.createOrUpdateUserFromClaims(ctx, string(p.Name()), claims)
	if err != nil {
		as.logger.With(
			"email", claims.Email,
			"error", fmt.Sprintf("%+v", err),
			"idp", string(p.Name()),
			"idp_subject", claims.Subject,
		).Error("Failed to create or update user")
		return nil, nil, false, fmt.Errorf("failed to create or update user: %w", err)
	}

	as.logger.With(
		"user_id", user.ID,
		"email", user.Email,
	).Info("Authentication completed successfully")

	return user, tokens, isNewUser, nil
}

// RefreshTokens refreshes the access token using the named provider. The
// provider name is carried in the session so the right provider rotates the
// token (different providers use different refresh endpoints; some, like
// GitHub, do not support refresh at all).
func (as *AuthService) RefreshTokens(
	ctx context.Context, provider, refreshToken string,
) (*idp.Tokens, error) {
	name := idp.ProviderName(provider)
	if provider == "" {
		// Back-compat: sessions issued before multi-provider support carry no
		// provider name. When the deployment runs a single provider, route to
		// it so those sessions keep refreshing across the upgrade.
		if enabled := as.registry.Enabled(); len(enabled) == 1 {
			name = enabled[0]
		}
	}
	p, ok := as.registry.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown identity provider %q", provider)
	}
	return p.Refresh(ctx, refreshToken)
}

// ProvisionFromClaims resolves or creates the VibeXP user for the given upstream
// IdP claims, reusing the same create-on-first-login logic as the web callback.
// The embedded OAuth Authorization Server (issue #31) uses it after exchanging
// the upstream code in its own /authorize flow.
func (as *AuthService) ProvisionFromClaims(
	ctx context.Context, providerName string, claims *idp.Claims,
) (*models.User, error) {
	user, _, err := as.createOrUpdateUserFromClaims(ctx, providerName, claims)
	return user, err
}

// createOrUpdateUserFromClaims looks up an existing user via the
// (idp_provider, idp_subject) tuple, falling back to legacy lookup by
// google_id, then creates a new user if none is found. providerName is the
// canonical name of the provider that produced the claims.
func (as *AuthService) createOrUpdateUserFromClaims(
	ctx context.Context, providerName string, claims *idp.Claims,
) (*models.User, bool, error) {
	user, err := as.findUserForClaims(ctx, providerName, claims.Subject)
	if err != nil {
		return nil, false, err
	}

	var avatarURL *string
	if claims.Picture != "" {
		avatarURL = &claims.Picture
	}

	if user == nil {
		return as.createUserFromClaims(ctx, providerName, claims, avatarURL)
	}

	user.Email = claims.Email
	user.Name = claims.Name
	user.AvatarURL = avatarURL
	user.IDPProvider = &providerName
	user.IDPSubject = &claims.Subject
	user.UpdatedAt = time.Now()

	if err := as.userRepo.Update(ctx, user); err != nil {
		return nil, false, fmt.Errorf("failed to update user: %w", err)
	}

	return user, false, nil
}

// isUserNotFoundErr reports whether err is or wraps
// repositories.ErrUserNotFound. Kept as a named helper because multiple
// call sites in this service need the same check.
func isUserNotFoundErr(err error) bool {
	return errors.Is(err, repositories.ErrUserNotFound)
}

// findUserForClaims locates the user matching the IDP tuple, falling back
// to a google_id lookup so legacy users sign in cleanly on first try.
//
// Defense-in-depth: when a row is matched by GetByIDPSubject we also verify
// the row's stored idp_provider equals the current provider name. This is
// already enforced by the SQL WHERE clause, but the check guards against a
// future repository implementation that loosens the lookup, and against
// any code path that aliases a non-Google IdP under the "google" name.
func (as *AuthService) findUserForClaims(
	ctx context.Context, providerName, subject string,
) (*models.User, error) {
	user, err := as.userRepo.GetByIDPSubject(ctx, providerName, subject)
	if err != nil && !isUserNotFoundErr(err) {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}
	if user != nil {
		if user.IDPProvider != nil && *user.IDPProvider != providerName {
			return nil, fmt.Errorf("idp provider mismatch on lookup: stored=%q current=%q",
				*user.IDPProvider, providerName)
		}
		return user, nil
	}
	// Legacy fallback only applies when the current provider IS Google;
	// non-Google providers must never claim an existing google_id row.
	if providerName != string(idp.ProviderGoogle) {
		return nil, nil
	}
	legacy, lerr := as.userRepo.GetByGoogleID(ctx, subject)
	if lerr != nil && !isUserNotFoundErr(lerr) {
		return nil, fmt.Errorf("failed to query legacy user: %w", lerr)
	}
	return legacy, nil
}

// createUserFromClaims persists a new user populated from the IDP claims
// and emits a user.created event.
func (as *AuthService) createUserFromClaims(
	ctx context.Context, providerName string, claims *idp.Claims, avatarURL *string,
) (*models.User, bool, error) {
	now := time.Now()
	newUser := &models.User{
		Email:       claims.Email,
		Name:        claims.Name,
		AvatarURL:   avatarURL,
		IDPProvider: &providerName,
		IDPSubject:  &claims.Subject,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	// Maintain google_id for Google users (legacy compatibility).
	// Non-Google providers do not receive a google_id.
	if providerName == string(idp.ProviderGoogle) {
		subject := claims.Subject
		newUser.GoogleID = &subject
	}

	if err := as.userRepo.Create(ctx, newUser); err != nil {
		return nil, false, fmt.Errorf("failed to create user: %w", err)
	}

	as.publishUserCreated(ctx, newUser)
	return newUser, true, nil
}

func (as *AuthService) publishUserCreated(ctx context.Context, user *models.User) {
	if as.eventManager == nil {
		return
	}
	event := events.NewUserCreatedEvent(user.ID, user.Email, user.Name, user.CreatedAt)
	if err := as.eventManager.Publish(ctx, event); err != nil {
		as.logger.With(
			"service", "vibexp-api",
			"method", "publishUserCreated",
			"user_id", user.ID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to publish user.created event")
	}
}

func (as *AuthService) GetUserByID(ctx context.Context, userID string) (*models.User, error) {
	return as.userRepo.GetByID(ctx, userID)
}

// devIDPProvider is the idp_provider tag stored on users provisioned through
// the development-only login bypass. Dev users are resolved by email (never by
// the (idp_provider, idp_subject) tuple or the bearer-token verifier), so this
// value is purely a stable, self-describing marker.
const devIDPProvider = "dev"

func (as *AuthService) createDevUser(ctx context.Context, email, name string) (*models.User, error) {
	devProvider := devIDPProvider
	devSubject := fmt.Sprintf("dev_%s", email)
	now := time.Now()
	newUser := &models.User{
		IDPProvider: &devProvider,
		IDPSubject:  &devSubject,
		Email:       email,
		Name:        name,
		AvatarURL:   nil,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := as.userRepo.Create(ctx, newUser); err != nil {
		as.logger.With("error", err).With("email", email).Error("Failed to create dev user")
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	as.logger.With(
		"user_id", newUser.ID,
		"email", newUser.Email,
	).Info("Created new dev user")

	if as.eventManager != nil {
		event := events.NewUserCreatedEvent(newUser.ID, newUser.Email, newUser.Name, newUser.CreatedAt)
		if publishErr := as.eventManager.Publish(ctx, event); publishErr != nil {
			as.logger.With("error", publishErr).
				With("user_id", newUser.ID).
				Error("Failed to publish user.created event")
		}
	}

	return newUser, nil
}

// HandleDevLogin handles demo login for the development environment.
// It creates or retrieves a user by email. The caller is responsible for
// creating the session cookie.
func (as *AuthService) HandleDevLogin(ctx context.Context, email, name string) (*models.User, error) {
	as.logger.With("email", email).Info("Processing dev login request")

	// Enforce the access allowlist before looking up or creating any user.
	if err := as.ensureAccessAllowed(ctx, email, "dev"); err != nil {
		return nil, err
	}

	user, err := as.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if !isUserNotFoundErr(err) {
			as.logger.With("error", err).With("email", email).Error("Failed to query user by email")
			return nil, fmt.Errorf("failed to query user: %w", err)
		}
		user = nil
	}

	if user == nil {
		user, err = as.createDevUser(ctx, email, name)
		if err != nil {
			return nil, err
		}
	} else {
		as.logger.With(
			"user_id", user.ID,
			"email", user.Email,
		).Info("Retrieved existing user for dev login")
	}

	as.logger.With(
		"user_id", user.ID,
		"email", user.Email,
	).Info("Dev login completed successfully")

	return user, nil
}
