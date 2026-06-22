package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services/feature_flags"
	"github.com/vibexp/vibexp/pkg/events"
)

// AuthService handles authentication operations using WorkOS as the identity
// provider. Tokens are delivered via AES-GCM encrypted httpOnly cookies managed
// by the session package — HS256 JWT signing is removed in this release.
type AuthService struct {
	userRepo       repositories.UserRepository
	idp            idp.IdentityProvider
	redirectURL    string
	featureFlagSvc feature_flags.FeatureFlagServiceInterface
	eventManager   events.EventPublisher
	logger         *slog.Logger
}

// Ensure AuthService implements AuthServiceInterface
var _ AuthServiceInterface = (*AuthService)(nil)

func NewAuthService(
	userRepo repositories.UserRepository, cfg *config.Config, identityProvider idp.IdentityProvider,
	eventManager events.EventPublisher, logger *slog.Logger,
	featureFlagSvc feature_flags.FeatureFlagServiceInterface,
) *AuthService {
	return &AuthService{
		userRepo:       userRepo,
		idp:            identityProvider,
		redirectURL:    cfg.WorkOSRedirectURI,
		featureFlagSvc: featureFlagSvc,
		eventManager:   eventManager,
		logger:         logger,
	}
}

// GetLoginURL returns the WorkOS AuthKit authorization URL for the given state
// and optional OAuth provider hint (e.g. "GitHubOAuth"). An empty provider
// string causes the IDP to use its default (GoogleOAuth for WorkOS).
func (as *AuthService) GetLoginURL(state, provider string) string {
	return as.idp.AuthorizeURL(state, as.redirectURL, provider)
}

// HandleCallback exchanges the authorization code for tokens, looks up or
// creates the user, and returns the user, IDP tokens, and whether they are new.
func (as *AuthService) HandleCallback(
	ctx context.Context, code string,
) (*models.User, *idp.Tokens, bool, error) {
	as.logger.Info("Processing OAuth callback")

	tokens, claims, err := as.idp.ExchangeCode(ctx, code, as.redirectURL)
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

	// L2: We do not currently reject !EmailVerified. WorkOS's Google
	// connection enforces email verification at the provider level, so
	// every claim arriving here has already been verified. If we add
	// connections where this is not true (e.g., magic link without
	// verification), gate sign-in here:
	//   if !claims.EmailVerified { return ErrUnverifiedEmail }

	user, isNewUser, err := as.createOrUpdateUserFromClaims(ctx, claims)
	if err != nil {
		as.logger.With(
			"email", claims.Email,
			"error", fmt.Sprintf("%+v", err),
			"idp", as.idp.Name(),
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

// RefreshTokens refreshes the access token using the given refresh token.
func (as *AuthService) RefreshTokens(ctx context.Context, refreshToken string) (*idp.Tokens, error) {
	return as.idp.Refresh(ctx, refreshToken)
}

// createOrUpdateUserFromClaims looks up an existing user via the
// (idp_provider, idp_subject) tuple, falling back to legacy lookup by
// google_id, then creates a new user if none is found.
func (as *AuthService) createOrUpdateUserFromClaims(
	ctx context.Context, claims *idp.Claims,
) (*models.User, bool, error) {
	providerName := string(as.idp.Name())

	user, err := as.findUserForClaims(ctx, providerName, claims.Subject)
	if err != nil {
		return nil, false, err
	}

	// Email fallback: when a WorkOS sign-in does not match by IDP tuple,
	// look up the legacy row by email and adopt it. This is the migration
	// path for users created under the old Google OAuth flow whose
	// idp_provider is currently "google" — first WorkOS login claims the
	// row and updates idp_provider/idp_subject below.
	if user == nil && providerName == string(idp.ProviderWorkOS) && claims.Email != "" {
		legacyByEmail, lerr := as.userRepo.GetByEmail(ctx, claims.Email)
		if lerr != nil && !isUserNotFoundErr(lerr) {
			return nil, false, fmt.Errorf("failed to query user by email: %w", lerr)
		}
		if legacyByEmail != nil {
			as.logger.With(
				"email", claims.Email,
				"user_id", legacyByEmail.ID,
			).
				Info("Adopting legacy user row for WorkOS sign-in via email match")
			user = legacyByEmail
		}
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
	// WorkOS users do not receive a google_id.
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

func (as *AuthService) createDevUser(ctx context.Context, email, name string) (*models.User, error) {
	devProvider := string(idp.ProviderWorkOS)
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
