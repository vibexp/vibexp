package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/auth/idp"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// AuthService handles authentication operations. It dispatches web login to
// one of the identity providers resolved from the database at runtime (#1234),
// selected per-request by the provider slug carried through the login/callback
// flow. Tokens are
// delivered via AES-GCM encrypted httpOnly cookies managed by the session
// package — HS256 JWT signing is removed in this release.
type AuthService struct {
	userRepo     repositories.UserRepository
	identityRepo repositories.UserIdentityRepository
	claimHook    UnverifiedAccountClaimHook
	// registrationGate decides whether a sign-in may create a new account.
	registrationGate RegistrationGate
	// disconnected says whether an owner disconnected a provider account
	// (#1460); nil means none is recorded.
	disconnected DisconnectedIdentityChecker
	resolver     IdentityProviderResolver
	allowlist    AccessAllowlistResolver
	eventManager events.EventPublisher
	logger       *slog.Logger
}

// Ensure AuthService implements AuthServiceInterface
var _ AuthServiceInterface = (*AuthService)(nil)

// ErrAccessRestricted is returned by the login entry points when the
// authenticated email is not permitted by the configured access allowlist. The
// HTTP layer branches on it with errors.Is to surface a policy denial (redirect
// for the OAuth callback, 403 for dev login) distinct from other failures.
var ErrAccessRestricted = errors.New("access restricted by allowlist")

// ErrEmailNotVerified is returned by the provider sign-in entry points when the
// sign-in has no identity yet, an account with the same email exists, and the
// provider did not verify that email. Attaching the sign-in to the account
// would hand it to anyone able to register that address unverified at a
// provider, so it is refused; nothing is written.
var ErrEmailNotVerified = errors.New("identity provider did not verify the email of an existing account")

// ErrExistingSignInRequired is returned by the provider sign-in entry points
// when the sign-in has no identity yet and the account with the same email (a)
// has never had that email verified and (b) already has a sign-in identity.
//
// Such an account may have been created by someone else: an open instance
// creates an account from an UNVERIFIED claim. Linking the proven owner into it
// would leave both people in one account, and nothing can revoke everything the
// first one holds (API keys, connected tools). So the link is refused and
// nothing is written. Signing in once with the account's own identity, with a
// verified email, verifies the account and lifts the refusal.
var ErrExistingSignInRequired = errors.New("existing account must first be signed in to with its own method")

// ErrIdentityDisconnected refuses to link, by email, a provider account the
// account's owner disconnected (#1460). The owner allows it again from Account
// settings, signed in another way.
var ErrIdentityDisconnected = errors.New("this sign-in identity was disconnected from the account")

// ErrIdentityProviderUnavailable is returned by HandleCallback and
// RefreshTokens when the provider slug names no enabled provider, e.g. one
// disabled between login and callback.
var ErrIdentityProviderUnavailable = errors.New("identity provider is not enabled")

// ErrIdentityProviderTemporarilyUnavailable is returned by RefreshTokens when
// the session's provider is enabled but currently failed to build (unhealthy,
// see IdentityProviderResolver.Health): the session is still valid and a later
// refresh may succeed, so it must not be treated as a revoked session.
var ErrIdentityProviderTemporarilyUnavailable = errors.New("identity provider is temporarily unavailable")

// ErrIdentityProvidersUnresolvable is returned when the enabled providers could
// not be read (the database is unreachable).
var ErrIdentityProvidersUnresolvable = errors.New("identity providers could not be resolved")

// ProviderInfo describes one enabled login provider.
type ProviderInfo struct {
	// Slug is the provider's identity: passed back as ?provider=, carried in
	// the state cookie and the session, and persisted as users.idp_provider.
	Slug string
	// DisplayName is the label the login UI shows.
	DisplayName string
	// Type is the provider kind: google, github or oidc.
	Type string
}

// ensureAccessAllowed denies sign-in when email is not permitted by the access
// allowlist, returning ErrAccessRestricted. With no active allowlist the
// instance is open and every email is allowed. The decision comes from the
// AccessAllowlistResolver, which reads the allowlist stored in the database
// (#1235) and exempts root instance admins; provider is included in the audit
// log for operator traceability.
//
// emailVerified reports whether the identity provider VERIFIED the address. An
// active allowlist rejects an unverified one (#218), root admins included: the
// allowlist and the root-admin exemption both grant access by address, so an
// address the user merely claimed must satisfy neither. Callers with no
// verification concept (dev login) pass true, keeping their behavior.
//
// An allowlist that is active but cannot be read fails closed: the resolver's
// error is returned and nobody but a root admin signs in.
func (as *AuthService) ensureAccessAllowed(
	ctx context.Context, email, provider string, emailVerified bool,
) error {
	return ensureEmailAllowed(ctx, as.allowlist, as.logger, email, provider, emailVerified)
}

func NewAuthService(
	userRepo repositories.UserRepository, identityRepo repositories.UserIdentityRepository,
	resolver IdentityProviderResolver,
	eventManager events.EventPublisher, logger *slog.Logger,
	allowlist AccessAllowlistResolver,
	opts ...AuthServiceOption,
) *AuthService {
	as := &AuthService{
		userRepo:         userRepo,
		identityRepo:     identityRepo,
		claimHook:        noopUnverifiedAccountClaimHook{},
		registrationGate: openRegistrationGate{},
		resolver:         resolver,
		allowlist:        allowlist,
		eventManager:     eventManager,
		logger:           logger,
	}
	for _, opt := range opts {
		opt(as)
	}
	return as
}

// snapshot resolves the enabled providers, wrapping a failure in
// ErrIdentityProvidersUnresolvable.
func (as *AuthService) snapshot(ctx context.Context) (*idp.Registry, error) {
	reg, err := as.resolver.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIdentityProvidersUnresolvable, err)
	}
	return reg, nil
}

// EnabledProviders returns the enabled login providers in the admin-defined
// sort order, for the HTTP layer to validate the ?provider= hint against and to
// surface the available choices.
func (as *AuthService) EnabledProviders(ctx context.Context) ([]ProviderInfo, error) {
	reg, err := as.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	entries := reg.Entries()
	out := make([]ProviderInfo, len(entries))
	for i, e := range entries {
		out[i] = ProviderInfo{Slug: string(e.Provider.Name()), DisplayName: e.DisplayName, Type: string(e.Type)}
	}
	return out, nil
}

// GetLoginURL returns the authorization URL for the provider with slug. It
// returns an empty string when no such provider is enabled, so the caller can
// surface a "provider unavailable" response. Every provider is built with the
// derived callback URL (config.Config.AuthCallbackURL), so no override is
// passed here.
func (as *AuthService) GetLoginURL(ctx context.Context, state, slug string) (string, error) {
	reg, err := as.snapshot(ctx)
	if err != nil {
		return "", err
	}
	p, ok := reg.Get(idp.ProviderName(slug))
	if !ok {
		return "", nil
	}
	return p.AuthorizeURL(state, "", slug), nil
}

// HandleCallback exchanges the authorization code for tokens using the provider
// with slug, looks up or creates the user, and returns the user, IDP tokens,
// and whether they are new. The slug is resolved from the signed state cookie
// by the HTTP layer.
func (as *AuthService) HandleCallback(
	ctx context.Context, code, slug string,
) (*models.User, *idp.Tokens, bool, error) {
	as.logger.With("provider", slug).Info("Processing OAuth callback")

	reg, err := as.snapshot(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	entry, ok := reg.Entry(idp.ProviderName(slug))
	if !ok {
		return nil, nil, false, fmt.Errorf("%w: %q", ErrIdentityProviderUnavailable, slug)
	}

	tokens, claims, err := entry.Provider.ExchangeCode(ctx, code, "")
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

	// The access allowlist and the registration mode are enforced inside
	// createOrUpdateUserFromClaims, once it knows which account the sign-in
	// resolves to and BEFORE any user row is created or updated, so a denied
	// sign-in leaves zero database residue and emits no user.created event.
	user, isNewUser, err := as.createOrUpdateUserFromClaims(ctx, slug, entry.Type, claims)
	if isSignInRefusal(err) {
		// A refusal, not a failure: already logged at INFO where it was decided,
		// and returned as is so the handler can answer with its own error code.
		return nil, nil, false, err
	}
	if err != nil {
		as.logger.With(
			"email", claims.Email,
			"error", fmt.Sprintf("%+v", err),
			"idp", slug,
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

// RefreshTokens refreshes the access token using the provider with slug. The
// slug is carried in the session so the right provider rotates the token
// (different providers use different refresh endpoints; some, like GitHub, do
// not support refresh at all). A failure to read the providers returns
// ErrIdentityProvidersUnresolvable and an enabled-but-unhealthy provider
// ErrIdentityProviderTemporarilyUnavailable, both transient; only a provider
// that is not enabled returns ErrIdentityProviderUnavailable.
func (as *AuthService) RefreshTokens(
	ctx context.Context, slug, refreshToken string,
) (*idp.Tokens, error) {
	reg, err := as.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	name := idp.ProviderName(slug)
	if slug == "" {
		// Back-compat: sessions issued before multi-provider support carry no
		// provider name. When the deployment runs a single provider, route to
		// it so those sessions keep refreshing across the upgrade.
		if enabled := reg.Enabled(); len(enabled) == 1 {
			name = enabled[0]
		}
	}
	p, ok := reg.Get(name)
	if !ok {
		// Health lists every ENABLED provider, so an entry here means the
		// provider is enabled but failed to build: a transient condition.
		if _, enabled := as.resolver.Health()[string(name)]; enabled {
			return nil, fmt.Errorf("%w: %q", ErrIdentityProviderTemporarilyUnavailable, slug)
		}
		return nil, fmt.Errorf("%w: %q", ErrIdentityProviderUnavailable, slug)
	}
	return p.Refresh(ctx, refreshToken)
}

// RevokeUserSessions ends every cookie session of the user by bumping their
// session epoch, and returns the new epoch (#1428). A session cookie carries the
// epoch it was minted under and the authentication chokepoint rejects one that
// no longer matches, so the bump takes effect on each session's next request.
func (as *AuthService) RevokeUserSessions(ctx context.Context, userID string) (int64, error) {
	epoch, err := as.userRepo.BumpSessionEpoch(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to revoke sessions of user %q: %w", userID, err)
	}
	as.logger.With("user_id", userID, "session_epoch", epoch).Info("Revoked the user's cookie sessions")
	return epoch, nil
}

// ProvisionFromClaims resolves or creates the VibeXP user for the given upstream
// IdP claims, reusing the same create-on-first-login logic as the web callback.
// The embedded OAuth Authorization Server (issue #31) uses it after exchanging
// the upstream code in its own /authorize flow.
func (as *AuthService) ProvisionFromClaims(
	ctx context.Context, providerName string, claims *idp.Claims,
) (*models.User, error) {
	// The AS login leg names providers by their built-in type names.
	user, _, err := as.createOrUpdateUserFromClaims(ctx, providerName, idp.ProviderName(providerName), claims)
	return user, err
}

// createOrUpdateUserFromClaims resolves the account a provider sign-in belongs
// to, in this order:
//
//  1. the account that owns the sign-in identity (provider slug, subject) in
//     user_identities, falling back to the legacy Google identity;
//  2. the account with the same email, which the sign-in is LINKED to when the
//     provider verified that email, and refused when it did not
//     (ErrEmailNotVerified) or when the account was never verified and already
//     has a sign-in identity of its own (ErrExistingSignInRequired);
//  3. a new account, when the registration mode allows one
//     (ErrRegistrationClosed otherwise).
//
// The access allowlist is enforced here as well (ErrAccessRestricted), on the
// account the sign-in resolved to: an existing account by its stored email, a
// new one by the claim email. Every check runs before the first write.
//
// providerName is the slug of the provider that produced the claims and
// providerType its kind, which decides the Google-only legacy handling.
//
// The account email is stable: it is set when the account is created and never
// overwritten from claims afterwards.
func (as *AuthService) createOrUpdateUserFromClaims(
	ctx context.Context, providerName string, providerType idp.ProviderName, claims *idp.Claims,
) (*models.User, bool, error) {
	user, err := as.findUserForClaims(ctx, providerName, providerType, claims.Subject)
	if err != nil {
		return nil, false, err
	}
	if user != nil {
		// An existing account is judged by its STORED email, the one the
		// per-request check uses too: the account email no longer follows the
		// provider (#1430), so the claim email may be a different address.
		verified := existingAccountEmailVerified(user, claims)
		if err = as.ensureAccessAllowed(ctx, user.Email, providerName, verified); err != nil {
			return nil, false, err
		}
		return as.refreshReturningUser(ctx, user, providerName, claims)
	}

	existing, err := as.findUserByClaimEmail(ctx, claims)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		// Found BY the claim email, so stored and claimed address are the same.
		if err = as.ensureAccessAllowed(ctx, existing.Email, providerName, claims.EmailVerified); err != nil {
			return nil, false, err
		}
		return as.linkIdentityByEmail(ctx, existing, providerName, claims)
	}

	// A new account: the claim email is all there is to judge. The allowlist
	// decides who may be here at all, then the registration mode decides who
	// may create an account; an existing account never reaches this point.
	if err = as.ensureAccessAllowed(ctx, claims.Email, providerName, claims.EmailVerified); err != nil {
		return nil, false, err
	}
	if err = as.registrationGate.AllowNewAccount(ctx, claims.Email, claims.EmailVerified); err != nil {
		return nil, false, err
	}
	return as.createUserFromClaims(ctx, providerName, providerType, claims, claimAvatarURL(claims))
}

// isSignInRefusal reports whether err is a deliberate policy outcome of a
// sign-in rather than a failure: the allowlist, the registration mode, or one
// of the account-linking rules said no.
func isSignInRefusal(err error) bool {
	return errors.Is(err, ErrAccessRestricted) ||
		errors.Is(err, ErrRegistrationClosed) ||
		errors.Is(err, ErrEmailNotVerified) ||
		errors.Is(err, ErrExistingSignInRequired) ||
		errors.Is(err, ErrIdentityDisconnected)
}

// existingAccountEmailVerified reports whether the STORED email of an existing
// account counts as verified for this sign-in: a provider verified it earlier
// (email_verified_at), or this sign-in's provider verifies that same address
// now. A verified claim for some other address says nothing about the stored
// one.
func existingAccountEmailVerified(user *models.User, claims *idp.Claims) bool {
	if user.EmailVerifiedAt != nil {
		return true
	}
	return claims.EmailVerified && models.NormalizeEmail(claims.Email) == models.NormalizeEmail(user.Email)
}

// claimAvatarURL returns the claims' picture, or nil when there is none.
func claimAvatarURL(claims *idp.Claims) *string {
	if claims.Picture == "" {
		return nil
	}
	picture := claims.Picture
	return &picture
}

// findUserByClaimEmail returns the account whose email is the claims' email, or
// nil when there is none or the claims carry no email.
func (as *AuthService) findUserByClaimEmail(ctx context.Context, claims *idp.Claims) (*models.User, error) {
	if models.NormalizeEmail(claims.Email) == "" {
		return nil, nil
	}
	user, err := as.userRepo.GetByEmail(ctx, claims.Email)
	if err != nil {
		if isUserNotFoundErr(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query user by email: %w", err)
	}
	return user, nil
}

// refreshReturningUser records a sign-in by an account's own identity. The
// account's email is left alone, and its name and avatar are taken from the
// claims only while it has none: both are the user's to edit from then on.
//
// users.idp_provider and users.idp_subject are still written (the identity
// last used; the admin list and export read them) but are not what a sign-in
// is resolved by.
func (as *AuthService) refreshReturningUser(
	ctx context.Context, user *models.User, providerName string, claims *idp.Claims,
) (*models.User, bool, error) {
	now := time.Now()
	user.EmailVerifiedAt = emailVerifiedAtAfterSignIn(user, claims, now)
	fillProfileFromClaims(user, claims)
	user.IDPProvider = &providerName
	user.IDPSubject = &claims.Subject
	user.UpdatedAt = now

	if err := as.userRepo.Update(ctx, user); err != nil {
		return nil, false, fmt.Errorf("failed to update user: %w", err)
	}

	// Record the identity this sign-in used. For a user matched through the
	// legacy Google identity this is also what gives them their slug identity.
	if err := as.identityRepo.Upsert(ctx, user.ID, providerName, claims.Subject); err != nil {
		return nil, false, fmt.Errorf("failed to record sign-in identity: %w", err)
	}

	return user, false, nil
}

// linkIdentityByEmail attaches a sign-in that has no identity yet to the
// existing account with the same email (#1430). This is what lets an
// admin-created account complete its first provider sign-in, and one person
// use two providers that report the same address.
//
// It refuses, writing nothing, when:
//   - the provider did not verify the email (ErrEmailNotVerified);
//   - the account's email was never verified and the account already has a
//     sign-in identity (ErrExistingSignInRequired, see its doc).
//
// A suspended account is returned untouched, so the caller's suspension gate
// refuses the sign-in and no new way in is attached to it.
//
// The first verified link to an account whose email was never verified is a
// claim by the mailbox's proven owner; claimUnverifiedAccount closes what
// predates the proof before the account is marked verified and the identity is
// attached. Each step is safe to repeat, so a failure part-way is repaired by
// the next attempt and never leaves a link without the claim having run.
func (as *AuthService) linkIdentityByEmail(
	ctx context.Context, user *models.User, providerName string, claims *idp.Claims,
) (*models.User, bool, error) {
	if user.IsSuspended() {
		return user, false, nil
	}
	if !claims.EmailVerified {
		as.logLinkRefused(user, providerName, "unverified_email")
		return nil, false, ErrEmailNotVerified
	}
	if err := as.refuseDisconnected(ctx, user, providerName, claims.Subject); err != nil {
		return nil, false, err
	}

	now := time.Now()
	if user.EmailVerifiedAt == nil {
		fresh, err := as.claimOrReload(ctx, user, providerName, claims.Email)
		if err != nil {
			return nil, false, err
		}
		if fresh.IsSuspended() {
			// Suspended while the sign-in ran: returned untouched, as at the
			// top, so the caller's suspension gate refuses it and no way in
			// is attached.
			return fresh, false, nil
		}
		user = fresh
		if user.EmailVerifiedAt == nil {
			user.EmailVerifiedAt = &now
		}
	}
	fillProfileFromClaims(user, claims)
	user.IDPProvider = &providerName
	user.IDPSubject = &claims.Subject
	user.UpdatedAt = now

	if err := as.userRepo.Update(ctx, user); err != nil {
		return nil, false, fmt.Errorf("failed to update user: %w", err)
	}
	if err := as.identityRepo.Upsert(ctx, user.ID, providerName, claims.Subject); err != nil {
		return nil, false, fmt.Errorf("failed to link sign-in identity: %w", err)
	}

	as.logger.With(
		"user_id", user.ID,
		"provider", providerName,
	).Info("Linked a new sign-in identity to an existing account by verified email")

	return user, false, nil
}

// refuseDisconnected refuses a link to a provider account the owner disconnected
// from this account (#1460). It runs before anything is written.
func (as *AuthService) refuseDisconnected(
	ctx context.Context, user *models.User, providerName, subject string,
) error {
	if as.disconnected == nil {
		return nil
	}
	disconnected, err := as.disconnected.IsDisconnected(ctx, user.ID, providerName, subject)
	if err != nil {
		return fmt.Errorf("failed to read the account's disconnected identities: %w", err)
	}
	if disconnected {
		as.logLinkRefused(user, providerName, "identity_disconnected")
		return ErrIdentityDisconnected
	}
	return nil
}

// claimOrReload claims the unverified account, or, when the claim finds under
// its lock that the account changed since it was read (#1466), re-reads it:
// an account verified meanwhile (an emailed link was completed) is linked as
// a verified account, keeping what its owner just set; one that gained a
// sign-in identity is refused like any unverified account with one. It
// returns the user to link, which carries the current version.
func (as *AuthService) claimOrReload(
	ctx context.Context, user *models.User, providerName, claimEmail string,
) (*models.User, error) {
	err := as.claimUnverifiedAccount(ctx, user, providerName)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, repositories.ErrAccountNoLongerClaimable) {
		return nil, err
	}
	fresh, err := as.userRepo.GetByID(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to re-read an account that changed during sign-in: %w", err)
	}
	// Nothing un-verifies an account, so one still unverified became
	// unclaimable by gaining a sign-in identity.
	if fresh.EmailVerifiedAt == nil {
		as.logLinkRefused(fresh, providerName, "unverified_account_has_identity")
		return nil, ErrExistingSignInRequired
	}
	// The match was by email: an address changed meanwhile is not the one the
	// provider verified.
	if models.NormalizeEmail(fresh.Email) != models.NormalizeEmail(claimEmail) {
		as.logLinkRefused(fresh, providerName, "email_changed_during_sign_in")
		return nil, ErrExistingSignInRequired
	}
	as.logger.With("user_id", fresh.ID, "provider", providerName).
		Info("The account was verified during sign-in; linking it as a verified account")
	return fresh, nil
}

// claimUnverifiedAccount prepares an account whose email was never verified for
// its first verified link. It changes nothing on the user row except the
// session epoch, which it also sets on the model so the session about to be
// minted for this sign-in is issued under the new epoch.
//
// An account that already has a sign-in identity is refused: see
// ErrExistingSignInRequired. One without (admin-created, or set up from an
// invitation or setup link) is claimed: the hook removes what its setup left
// behind, and every cookie session ends.
func (as *AuthService) claimUnverifiedAccount(ctx context.Context, user *models.User, providerName string) error {
	identities, err := as.identityRepo.ListByUserID(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to read the account's sign-in identities: %w", err)
	}
	if len(identities) > 0 {
		as.logLinkRefused(user, providerName, "unverified_account_has_identity")
		return ErrExistingSignInRequired
	}

	if err = as.claimHook.OnUnverifiedAccountClaimed(ctx, user.ID, as.providerDisplayName(ctx, providerName)); err != nil {
		return fmt.Errorf("failed to secure the account before linking: %w", err)
	}
	epoch, err := as.userRepo.BumpSessionEpoch(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to end the account's sessions before linking: %w", err)
	}
	user.SessionEpoch = epoch
	return nil
}

// providerDisplayName is the label the sign-in page shows for the provider
// with slug, or the slug itself when the provider cannot be resolved: the
// name is only for telling a person what happened, never for a decision.
func (as *AuthService) providerDisplayName(ctx context.Context, slug string) string {
	if as.resolver == nil {
		return slug
	}
	reg, err := as.snapshot(ctx)
	if err != nil {
		return slug
	}
	entry, ok := reg.Entry(idp.ProviderName(slug))
	if !ok || strings.TrimSpace(entry.DisplayName) == "" {
		return slug
	}
	return entry.DisplayName
}

// logLinkRefused records why a sign-in was not linked to the account with the
// same email. Both refusals look alike to the person signing in only by their
// error code, so this is the operator's trace.
func (as *AuthService) logLinkRefused(user *models.User, providerName, reason string) {
	as.logger.With(
		"user_id", user.ID,
		"email", user.Email,
		"provider", providerName,
		"reason", reason,
	).Info("Sign-in refused: not linked to the existing account with the same email")
}

// fillProfileFromClaims copies the claims' name and avatar into an account that
// has none. Values the account already holds are never overwritten.
func fillProfileFromClaims(user *models.User, claims *idp.Claims) {
	if strings.TrimSpace(user.Name) == "" {
		user.Name = claims.Name
	}
	if user.AvatarURL == nil || *user.AvatarURL == "" {
		user.AvatarURL = claimAvatarURL(claims)
	}
}

// emailVerifiedAtAfterSignIn returns the email_verified_at a returning user
// should carry after this sign-in. The timestamp describes the STORED address,
// which no longer follows the claims: it is set the first time a provider
// verifies that same address and kept from then on. A provider verifying a
// different address says nothing about the stored one.
func emailVerifiedAtAfterSignIn(user *models.User, claims *idp.Claims, now time.Time) *time.Time {
	sameEmail := models.NormalizeEmail(claims.Email) == models.NormalizeEmail(user.Email)
	if user.EmailVerifiedAt == nil && claims.EmailVerified && sameEmail {
		return &now
	}
	return user.EmailVerifiedAt
}

// isUserNotFoundErr reports whether err is or wraps
// repositories.ErrUserNotFound. Kept as a named helper because multiple
// call sites in this service need the same check.
func isUserNotFoundErr(err error) bool {
	return errors.Is(err, repositories.ErrUserNotFound)
}

// findUserForClaims locates the user who owns the (provider slug, subject)
// sign-in identity, falling back to the legacy Google identity so users from
// before provider slugs sign in cleanly on first try.
func (as *AuthService) findUserForClaims(
	ctx context.Context, providerName string, providerType idp.ProviderName, subject string,
) (*models.User, error) {
	user, err := as.userRepo.GetByIDPSubject(ctx, providerName, subject)
	if err != nil && !isUserNotFoundErr(err) {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}
	if user != nil {
		return user, nil
	}
	// Legacy fallback only applies when the current provider IS Google (by
	// type, since the slug is admin-chosen); non-Google providers must never
	// claim a legacy Google identity.
	if providerType != idp.ProviderGoogle {
		return nil, nil
	}
	legacy, lerr := as.userRepo.GetByIDPSubject(ctx, models.LegacyGoogleIdentityProvider, subject)
	if lerr != nil && !isUserNotFoundErr(lerr) {
		return nil, fmt.Errorf("failed to query legacy user: %w", lerr)
	}
	return legacy, nil
}

// createUserFromClaims persists a new user populated from the IDP claims
// and emits a user.created event.
func (as *AuthService) createUserFromClaims(
	ctx context.Context, providerName string, providerType idp.ProviderName, claims *idp.Claims, avatarURL *string,
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
	if claims.EmailVerified {
		newUser.EmailVerifiedAt = &now
	}
	// Maintain google_id for Google users (legacy compatibility): the
	// repository stores it as the legacy Google identity as well, so the user
	// is still found if the Google provider is later recreated under another
	// slug. Non-Google providers do not receive a google_id.
	if providerType == idp.ProviderGoogle {
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
// the development-only login bypass, and the provider of their sign-in
// identity. Dev users are resolved by email (never by identity or the
// bearer-token verifier), so this value is purely a stable, self-describing
// marker.
const devIDPProvider = models.DevIdentityProvider

func (as *AuthService) createDevUser(ctx context.Context, email, name string) (*models.User, error) {
	devProvider := devIDPProvider
	email = models.NormalizeEmail(email)
	devSubject := fmt.Sprintf("dev_%s", email)
	now := time.Now()
	newUser := &models.User{
		IDPProvider: &devProvider,
		IDPSubject:  &devSubject,
		Email:       email,
		// Dev login treats the typed address as verified (see HandleDevLogin),
		// so a dev root admin holds root admin power (#1467).
		EmailVerifiedAt: &now,
		Name:            name,
		AvatarURL:       nil,
		CreatedAt:       now,
		UpdatedAt:       now,
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

// markDevUserVerified sets email_verified_at on an existing account dev login
// signs into, when it is not set yet. Dev login verifies the address by
// construction, and root admin power requires a verified email (#1467), so
// without this a dev account from before email verification was recorded
// would lose its admin access.
func (as *AuthService) markDevUserVerified(ctx context.Context, user *models.User) error {
	if user.EmailVerifiedAt != nil {
		return nil
	}
	now := time.Now()
	user.EmailVerifiedAt = &now
	user.UpdatedAt = now
	if err := as.userRepo.Update(ctx, user); err != nil {
		as.logger.With("error", err).With("user_id", user.ID).Error("Failed to mark dev user verified")
		return fmt.Errorf("failed to mark dev user verified: %w", err)
	}
	return nil
}

// HandleDevLogin handles demo login for the development environment.
// It creates or retrieves a user by email. The caller is responsible for
// creating the session cookie.
func (as *AuthService) HandleDevLogin(ctx context.Context, email, name string) (*models.User, error) {
	as.logger.With("email", email).Info("Processing dev login request")

	// Enforce the access allowlist before looking up or creating any user. Dev
	// login has no identity provider and so no verification concept; the address
	// is verified by construction (it is whatever the local developer typed), so
	// the #218 unverified-email rule does not apply here.
	if err := as.ensureAccessAllowed(ctx, email, "dev", true); err != nil {
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
		if err := as.markDevUserVerified(ctx, user); err != nil {
			return nil, err
		}
	}

	as.logger.With(
		"user_id", user.ID,
		"email", user.Email,
	).Info("Dev login completed successfully")

	return user, nil
}
