package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Instance authentication settings for the admin API (#1238, epic #1230): the
// sign-in identity providers, the access allowlist and the DB-granted instance
// admins.
//
// There is no authorization inside, with one exception: who may reach these
// operations is the admin route guard's decision (an instance admin, or a setup
// session for the providers and the allowlist), while granting and revoking
// admins is root-only and enforced here through InstanceAdminResolver.

// Reasons the lockout guard refuses a change (ErrAuthLockoutRisk).
const (
	// AuthLockoutNoEnabledProvider is a provider change that would leave no
	// enabled provider while password login is not a working way in either, so
	// nobody could sign in.
	AuthLockoutNoEnabledProvider = "no_enabled_provider"
	// AuthLockoutOwnProvider is a change that takes away the provider the
	// caller's own session was issued by.
	AuthLockoutOwnProvider = "own_provider"
	// AuthLockoutNoUsableMethod is switching password login off while it is
	// the only working way in (#1438).
	AuthLockoutNoUsableMethod = "no_usable_method"
	// AuthLockoutOwnMethod is switching password login off from a session
	// that signed in with a password.
	AuthLockoutOwnMethod = "own_method"
	// AuthLockoutPasswordOnlyAccounts is switching password login off while
	// accounts exist whose only sign-in method is a password.
	AuthLockoutPasswordOnlyAccounts = "password_only_accounts"
)

// ErrAuthLockoutRisk is returned when a provider or policy change could lock
// sign-in out and the caller did not confirm the risk.
type ErrAuthLockoutRisk struct {
	// Reason is one of the AuthLockout* constants: the first that applies.
	Reason string
	// Reasons is every reason that applies, Reason first (#1473). Set by the
	// policy guard, which can find several; nil for a provider change, which
	// has one.
	Reasons []string
	// PasswordOnlyAccounts is how many accounts have a password as their only
	// sign-in method; set with AuthLockoutPasswordOnlyAccounts.
	PasswordOnlyAccounts int
}

// AllReasons is every reason that applies: Reasons, or Reason alone.
func (e *ErrAuthLockoutRisk) AllReasons() []string {
	if len(e.Reasons) > 0 {
		return e.Reasons
	}
	return []string{e.Reason}
}

func (e *ErrAuthLockoutRisk) Error() string {
	reasons := e.AllReasons()
	texts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		texts = append(texts, e.reasonText(reason))
	}
	return strings.Join(texts, "; ")
}

func (e *ErrAuthLockoutRisk) reasonText(reason string) string {
	switch reason {
	case AuthLockoutOwnProvider:
		return "this change disables the sign-in provider your own session was issued by"
	case AuthLockoutNoUsableMethod:
		return "this change switches off the only sign-in method that works"
	case AuthLockoutOwnMethod:
		return "this change switches off password sign-in, which your own session used"
	case AuthLockoutPasswordOnlyAccounts:
		if e.PasswordOnlyAccounts == 1 {
			return "this change leaves 1 account, whose only sign-in method is a password, unable to sign in"
		}
		return fmt.Sprintf("this change leaves %d accounts, whose only sign-in method is a password, unable to sign in",
			e.PasswordOnlyAccounts)
	}
	return "this change leaves no enabled sign-in provider"
}

// ErrInstanceAdminGrantTargetAmbiguous is returned when a grant names neither
// or both of a user id and an email.
var ErrInstanceAdminGrantTargetAmbiguous = errors.New("exactly one of user_id or email is required")

// InstanceAuthSettingsServiceInterface defines the operations behind the
// instance authentication settings admin API. actorUserID is the acting
// admin's id, or empty on a setup session, which has no user: the change is
// then audited with no actor.
type InstanceAuthSettingsServiceInterface interface {
	// ListProviders returns every stored provider with its health and the
	// shared auth settings version. It resolves the enabled providers first, so
	// the health describes the rows it returns.
	ListProviders(ctx context.Context) (*models.InstanceAuthProviderList, error)
	// CreateProvider validates, encrypts and stores a new provider.
	CreateProvider(ctx context.Context, actorUserID string,
		req models.CreateInstanceAuthProviderRequest) (*models.InstanceAuthProviderSaved, error)
	// UpdateProvider replaces a provider's editable fields. A nil secret keeps
	// the stored one, but only while the issuer URL is unchanged. A change that
	// disables a provider is subject to the lockout guard (ErrAuthLockoutRisk).
	UpdateProvider(ctx context.Context, actorUserID, id string,
		req models.UpdateInstanceAuthProviderRequest) (*models.InstanceAuthProviderSaved, error)
	// DeleteProvider removes a provider, subject to the lockout guard.
	DeleteProvider(ctx context.Context, actorUserID, id string, req models.DeleteInstanceAuthProviderRequest) error
	// TestProvider checks a stored provider or a candidate without storing or
	// auditing anything. A failed check is reported in the result.
	TestProvider(ctx context.Context,
		req models.TestInstanceAuthProviderRequest) (*models.InstanceAuthProviderTestResult, error)

	// GetAllowlist returns the stored allowlist, or nil when none is stored
	// (open access).
	GetAllowlist(ctx context.Context) (*models.InstanceAuthAllowlist, error)
	// UpdateAllowlist validates, normalizes and stores the allowlist. A non-nil
	// expectedVersion is compared with the stored allowlist's own version;
	// repositories.InstanceSettingsNoStoredVersion expects none stored.
	UpdateAllowlist(ctx context.Context, actorUserID string, domains, emails []string,
		expectedVersion *int64) (*models.InstanceAuthAllowlist, error)
	// ResetAllowlist removes the stored allowlist, reverting to open access.
	// With none stored it is a no-op.
	ResetAllowlist(ctx context.Context, actorUserID string) error
	// PreviewAllowlist reports who a candidate allowlist would shut out. It
	// stores nothing.
	PreviewAllowlist(ctx context.Context, domains, emails []string) (*AllowlistImpact, error)

	// GetPolicy returns the authentication policy:
	// models.DefaultInstanceAuthPolicy (not stored, version 0) when none is
	// stored. It reads the table, not the resolver's cache, so an admin always
	// sees the version their next save must name.
	GetPolicy(ctx context.Context) (*models.InstanceAuthPolicy, error)
	// UpdatePolicy validates and stores a whole replacement of the policy. A
	// non-nil expectedVersion is compared with the stored policy's own version;
	// repositories.InstanceSettingsNoStoredVersion expects none stored.
	//
	// Switching password login off is subject to the lockout guard
	// (ErrAuthLockoutRisk): it is refused, unless lockout.Confirmed, while
	// password login is the only working way in, when the caller's own session
	// signed in with a password, and while accounts exist whose only sign-in
	// method is a password. Stored passwords are kept either way.
	UpdatePolicy(ctx context.Context, actorUserID string, policy models.InstanceAuthPolicy,
		expectedVersion *int64, lockout models.InstanceAuthLockoutContext) (*models.InstanceAuthPolicy, error)
	// BootstrapAdminPassword gives the root admin with email a password and
	// switches password login on (RootAdminPasswordService.Set). It is the
	// setup session's way to make a password-only instance usable; the caller
	// decides who may reach it.
	BootstrapAdminPassword(ctx context.Context, email, plaintext string) (*RootAdminPasswordResult, error)

	// ListAdmins returns the root admins' emails and the DB-granted admins.
	ListAdmins(ctx context.Context) (*models.InstanceAdminList, error)
	// GrantAdmin makes an existing user an instance admin. actingUserID must be
	// a root admin (ErrInstanceAdminNotRoot), checked before the target is
	// looked up, so a non-root caller learns nothing about which users exist.
	GrantAdmin(ctx context.Context, actingUserID string,
		target models.InstanceAdminGrantTarget) (*models.InstanceAdminView, error)
	// RevokeAdmin removes a DB grant. actingUserID must be a root admin.
	RevokeAdmin(ctx context.Context, actingUserID, targetUserID string) error
}

// InstanceAuthSettingsDeps are the collaborators of InstanceAuthSettingsService.
type InstanceAuthSettingsDeps struct {
	Providers  repositories.InstanceAuthProviderRepository
	Allowlists repositories.InstanceAuthAllowlistRepository
	Policies   repositories.InstanceAuthPolicyRepository
	Versions   repositories.InstanceAuthSettingsVersionRepository
	Grants     repositories.InstanceAdminRepository
	Users      repositories.UserRepository
	// Resolver reports provider health and runs provider tests.
	Resolver IdentityProviderResolver
	// AllowlistResolver previews a candidate allowlist's impact.
	AllowlistResolver AccessAllowlistResolver
	Admins            InstanceAdminResolver
	// WayIn tells the lockout guard whether password login is a working way in.
	WayIn AuthWayIn
	// Credentials counts the accounts a policy change would lock out.
	Credentials repositories.UserCredentialRepository
	// RootPasswords sets a root admin's password for the setup session.
	RootPasswords *RootAdminPasswordService
	// Caches is told after every policy or allowlist write, so this process
	// serves the new settings at once (#1485). Nil means no cache to tell.
	Caches *AuthSettingsInvalidator
	Enc    EncryptionServiceInterface
	// CallbackURL is the derived redirect URI every provider is built with
	// (config.Config.AuthCallbackURL).
	CallbackURL string
	Logger      *slog.Logger
}

// InstanceAuthSettingsService implements InstanceAuthSettingsServiceInterface.
type InstanceAuthSettingsService struct {
	deps InstanceAuthSettingsDeps
}

var _ InstanceAuthSettingsServiceInterface = (*InstanceAuthSettingsService)(nil)

// NewInstanceAuthSettingsService creates the instance authentication settings
// service.
func NewInstanceAuthSettingsService(deps InstanceAuthSettingsDeps) *InstanceAuthSettingsService {
	return &InstanceAuthSettingsService{deps: deps}
}

// --- providers ----------------------------------------------------------------

// ListProviders implements InstanceAuthSettingsServiceInterface. The version is
// read before the rows: if a write lands in between, the caller holds an older
// version than the rows it sees and its next save is a conflict, never a
// silent overwrite.
func (s *InstanceAuthSettingsService) ListProviders(ctx context.Context) (*models.InstanceAuthProviderList, error) {
	version, err := s.deps.Versions.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read the auth settings version: %w", err)
	}
	rows, err := s.deps.Providers.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the auth providers: %w", err)
	}

	health := s.resolvedHealth(ctx)
	views := make([]models.InstanceAuthProviderView, 0, len(rows))
	for _, row := range rows {
		views = append(views, s.providerView(row, health))
	}
	return &models.InstanceAuthProviderList{Providers: views, Version: version}, nil
}

// resolvedHealth resolves the enabled providers and returns their health by
// slug. When they cannot be resolved it returns nil, so every enabled provider
// reads as unknown rather than as whatever an earlier resolve remembered.
func (s *InstanceAuthSettingsService) resolvedHealth(ctx context.Context) map[string]ProviderHealth {
	if _, err := s.deps.Resolver.Snapshot(ctx); err != nil {
		s.deps.Logger.With("error", err).Warn("Failed to resolve the auth providers; reporting their health as unknown")
		return nil
	}
	return s.deps.Resolver.Health()
}

func (s *InstanceAuthSettingsService) providerView(
	row *models.InstanceAuthProvider, health map[string]ProviderHealth,
) models.InstanceAuthProviderView {
	return models.InstanceAuthProviderView{
		Provider:    *row,
		Health:      instanceAuthProviderHealth(row, health),
		RedirectURI: s.deps.CallbackURL,
	}
}

// instanceAuthProviderHealth maps the resolver's build outcome of row onto the
// published health. A disabled provider is never built; an enabled one the
// resolver has no outcome for is unknown.
func instanceAuthProviderHealth(
	row *models.InstanceAuthProvider, health map[string]ProviderHealth,
) models.InstanceAuthProviderHealth {
	if !row.Enabled {
		return models.InstanceAuthProviderHealth{Status: models.InstanceAuthProviderDisabled}
	}
	h, ok := health[row.Slug]
	if !ok {
		return models.InstanceAuthProviderHealth{Status: models.InstanceAuthProviderHealthUnknown}
	}
	checkedAt := h.CheckedAt
	if h.Healthy {
		return models.InstanceAuthProviderHealth{Status: models.InstanceAuthProviderHealthy, CheckedAt: &checkedAt}
	}
	lastError := h.LastError
	return models.InstanceAuthProviderHealth{
		Status: models.InstanceAuthProviderUnhealthy, LastError: &lastError, CheckedAt: &checkedAt,
	}
}

// CreateProvider implements InstanceAuthSettingsServiceInterface.
func (s *InstanceAuthSettingsService) CreateProvider(
	ctx context.Context, actorUserID string, req models.CreateInstanceAuthProviderRequest,
) (*models.InstanceAuthProviderSaved, error) {
	row := models.InstanceAuthProvider{
		Type:        req.Type,
		Slug:        req.Slug,
		DisplayName: strings.TrimSpace(req.DisplayName),
		Enabled:     req.Enabled,
		SortOrder:   req.SortOrder,
		ClientID:    strings.TrimSpace(req.ClientID),
		IssuerURL:   trimmedOrNil(req.IssuerURL),
	}
	if err := ValidateInstanceAuthProvider(row); err != nil {
		return nil, err
	}
	encrypted, err := s.encryptClientSecret(req.ClientSecret)
	if err != nil {
		return nil, err
	}
	row.ClientSecretEncrypted = &encrypted

	version, err := s.writeVersion(ctx, req.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Providers.Create(ctx, &row, optionalActor(actorUserID), &version); err != nil {
		return nil, fmt.Errorf("failed to create the auth provider: %w", err)
	}
	return s.saved(ctx, &row, version), nil
}

// UpdateProvider implements InstanceAuthSettingsServiceInterface.
func (s *InstanceAuthSettingsService) UpdateProvider(
	ctx context.Context, actorUserID, id string, req models.UpdateInstanceAuthProviderRequest,
) (*models.InstanceAuthProviderSaved, error) {
	version, err := s.writeVersion(ctx, &req.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	rows, existing, err := s.providerAmong(ctx, id)
	if err != nil {
		return nil, err
	}

	next := *existing
	next.DisplayName = strings.TrimSpace(req.DisplayName)
	next.Enabled = req.Enabled
	next.SortOrder = req.SortOrder
	next.ClientID = strings.TrimSpace(req.ClientID)
	next.IssuerURL = trimmedOrNil(req.IssuerURL)
	if err = ValidateInstanceAuthProvider(next); err != nil {
		return nil, err
	}
	next.ClientSecretEncrypted, err = s.updatedClientSecret(existing, &next, req.ClientSecret)
	if err != nil {
		return nil, err
	}
	if err = s.guardProviderChange(ctx, rows, id, next.Enabled, req.Lockout); err != nil {
		return nil, err
	}

	if err = s.deps.Providers.Update(ctx, &next, optionalActor(actorUserID), &version); err != nil {
		return nil, fmt.Errorf("failed to update the auth provider: %w", err)
	}
	return s.saved(ctx, &next, version), nil
}

// updatedClientSecret returns the ciphertext an update stores. A submitted
// secret is encrypted. An omitted one keeps the stored ciphertext, but only
// while the issuer URL is unchanged: the issuer decides which token endpoint
// the secret is sent to at sign-in, so keeping it across an issuer change would
// hand the stored secret to an endpoint its owner never entered it for.
func (s *InstanceAuthSettingsService) updatedClientSecret(
	existing, next *models.InstanceAuthProvider, secret *string,
) (*string, error) {
	if secret != nil {
		encrypted, err := s.encryptClientSecret(*secret)
		if err != nil {
			return nil, err
		}
		return &encrypted, nil
	}
	if derefString(existing.IssuerURL) != derefString(next.IssuerURL) {
		return nil, invalidInstanceAuthProvider("client_secret",
			"client_secret is required when issuer_url changes")
	}
	return existing.ClientSecretEncrypted, nil
}

// DeleteProvider implements InstanceAuthSettingsServiceInterface.
func (s *InstanceAuthSettingsService) DeleteProvider(
	ctx context.Context, actorUserID, id string, req models.DeleteInstanceAuthProviderRequest,
) error {
	version, err := s.writeVersion(ctx, req.ExpectedVersion)
	if err != nil {
		return err
	}
	rows, _, err := s.providerAmong(ctx, id)
	if err != nil {
		return err
	}
	if err := s.guardProviderChange(ctx, rows, id, false, req.Lockout); err != nil {
		return err
	}
	if err := s.deps.Providers.Delete(ctx, id, optionalActor(actorUserID), &version); err != nil {
		return fmt.Errorf("failed to delete the auth provider: %w", err)
	}
	return nil
}

// writeVersion returns the shared auth settings version a provider write is
// compared-and-set against: the current one, which a non-nil expected must
// equal (ErrInstanceSettingsVersionConflict otherwise).
//
// Every provider write passes this version to the repository, including one
// whose caller sent no expected version. That is what makes the lockout guard
// sound: the guard reads the providers after this version, and the repository
// refuses the write if anything changed in between, so the guard's verdict
// always describes the rows the write lands on.
func (s *InstanceAuthSettingsService) writeVersion(ctx context.Context, expected *int64) (int64, error) {
	current, err := s.deps.Versions.Get(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to read the auth settings version: %w", err)
	}
	if expected != nil && *expected != current {
		return 0, repositories.ErrInstanceSettingsVersionConflict
	}
	return current, nil
}

// providerAmong returns every stored provider and the one with id, or
// ErrInstanceAuthProviderNotFound.
func (s *InstanceAuthSettingsService) providerAmong(
	ctx context.Context, id string,
) ([]*models.InstanceAuthProvider, *models.InstanceAuthProvider, error) {
	rows, err := s.deps.Providers.List(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list the auth providers: %w", err)
	}
	for _, row := range rows {
		if row.ID == id {
			return rows, row, nil
		}
	}
	return nil, nil, repositories.ErrInstanceAuthProviderNotFound
}

// saved builds the response of a write made at version. The repository
// compare-and-sets every provider write on the shared version and bumps it by
// one, so the version after the write is version+1; reading it back instead
// could return a later writer's version and let the caller's next save
// overwrite a change it never saw.
func (s *InstanceAuthSettingsService) saved(
	ctx context.Context, row *models.InstanceAuthProvider, version int64,
) *models.InstanceAuthProviderSaved {
	return &models.InstanceAuthProviderSaved{
		Provider: s.providerView(row, s.resolvedHealth(ctx)),
		Version:  version + 1,
	}
}

// encryptClientSecret encrypts a submitted client secret. A blank one is
// rejected: an empty string is never "no secret".
func (s *InstanceAuthSettingsService) encryptClientSecret(secret string) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", invalidInstanceAuthProvider("client_secret", "client_secret must not be empty")
	}
	if s.deps.Enc == nil {
		return "", fmt.Errorf("failed to encrypt the client secret: %w", ErrEncryptionUnavailable)
	}
	encrypted, err := s.deps.Enc.Encrypt(secret)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt the client secret: %w", err)
	}
	return encrypted, nil
}

// guardProviderChange runs checkAuthLockoutRisk, asking whether password login
// is a working way in only when the answer can matter.
func (s *InstanceAuthSettingsService) guardProviderChange(
	ctx context.Context, current []*models.InstanceAuthProvider, targetID string, enabledAfter bool,
	caller models.InstanceAuthLockoutContext,
) error {
	if caller.Confirmed || enabledAfter {
		return nil
	}
	passwordWorks, err := s.deps.WayIn.PasswordWorks(ctx)
	if err != nil {
		return err
	}
	return checkAuthLockoutRisk(current, targetID, enabledAfter, caller, passwordWorks)
}

// checkAuthLockoutRisk is the save guard on a provider change (epic #1230,
// decision 13). current is every stored provider; the change leaves the
// provider with targetID enabled or not (enabledAfter; false for a delete).
//
// Only a change that takes an ENABLED provider away is judged, and it is
// judged on the enabled set that results, not on the single row: it is refused
// when no enabled provider would remain and password login is not a working
// way in either (passwordWorks, #1438), or when the provider is the one the
// caller's own session was issued by. Enabled is the stored flag, not the
// resolver's health, so an identity provider outage never reads as "none
// enabled". A confirmed change is never refused.
func checkAuthLockoutRisk(
	current []*models.InstanceAuthProvider, targetID string, enabledAfter bool,
	caller models.InstanceAuthLockoutContext, passwordWorks bool,
) error {
	if caller.Confirmed || enabledAfter {
		return nil
	}
	var target *models.InstanceAuthProvider
	remaining := 0
	for _, p := range current {
		switch {
		case p.ID == targetID:
			target = p
		case p.Enabled:
			remaining++
		}
	}
	if target == nil || !target.Enabled {
		return nil
	}
	if remaining == 0 && !passwordWorks {
		return &ErrAuthLockoutRisk{Reason: AuthLockoutNoEnabledProvider}
	}
	if caller.SessionProvider != "" && caller.SessionProvider == target.Slug {
		return &ErrAuthLockoutRisk{Reason: AuthLockoutOwnProvider}
	}
	return nil
}

// TestProvider implements InstanceAuthSettingsServiceInterface.
func (s *InstanceAuthSettingsService) TestProvider(
	ctx context.Context, req models.TestInstanceAuthProviderRequest,
) (*models.InstanceAuthProviderTestResult, error) {
	candidate, err := s.testCandidate(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Resolver.TestProvider(ctx, *candidate, req.ClientSecret); err != nil {
		return &models.InstanceAuthProviderTestResult{Message: err.Error()}, nil
	}
	return &models.InstanceAuthProviderTestResult{Valid: true}, nil
}

// testCandidate builds the provider a test request describes. With an id it
// starts from the stored row, so an omitted secret means the stored one; the
// resolver never sends a stored secret to a candidate-chosen endpoint (see
// IdentityProviderResolver.TestProvider). Without an id nothing is stored to
// fall back on, so the type, client id and secret are all required.
func (s *InstanceAuthSettingsService) testCandidate(
	ctx context.Context, req models.TestInstanceAuthProviderRequest,
) (*models.InstanceAuthProvider, error) {
	candidate := &models.InstanceAuthProvider{}
	if req.ID != nil {
		stored, err := s.deps.Providers.Get(ctx, *req.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get the auth provider: %w", err)
		}
		if req.Type != nil && *req.Type != stored.Type {
			return nil, invalidInstanceAuthProvider("type", "type cannot differ from the stored provider's (%s)", stored.Type)
		}
		*candidate = *stored
	} else {
		if req.Type == nil {
			return nil, invalidInstanceAuthProvider("type", "type is required to test an unsaved provider")
		}
		if req.ClientSecret == nil {
			return nil, invalidInstanceAuthProvider("client_secret", "client_secret is required to test an unsaved provider")
		}
		candidate.Type = *req.Type
	}
	if req.ClientID != nil {
		candidate.ClientID = strings.TrimSpace(*req.ClientID)
	}
	if req.IssuerURL != nil {
		candidate.IssuerURL = trimmedOrNil(req.IssuerURL)
	}
	return candidate, validateAuthTestCandidate(candidate, req.ClientSecret)
}

// validateAuthTestCandidate applies the stored-provider rules a test needs: a
// known type, a client id, a non-blank submitted secret and a well-formed
// issuer on exactly the OIDC providers. A candidate has no slug or display name
// to check.
func validateAuthTestCandidate(candidate *models.InstanceAuthProvider, secret *string) error {
	if !candidate.Type.Valid() {
		return invalidInstanceAuthProvider("type", "type must be one of google, github, oidc, got %q", candidate.Type)
	}
	if candidate.ClientID == "" {
		return invalidInstanceAuthProvider("client_id", "client_id is required")
	}
	if secret != nil && strings.TrimSpace(*secret) == "" {
		return invalidInstanceAuthProvider("client_secret", "client_secret must not be empty")
	}
	return validateInstanceAuthIssuerURL(candidate.Type, candidate.IssuerURL)
}

// --- allowlist ----------------------------------------------------------------

// GetAllowlist implements InstanceAuthSettingsServiceInterface.
func (s *InstanceAuthSettingsService) GetAllowlist(ctx context.Context) (*models.InstanceAuthAllowlist, error) {
	allowlist, err := s.deps.Allowlists.Get(ctx)
	if errors.Is(err, repositories.ErrInstanceAuthAllowlistNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the access allowlist: %w", err)
	}
	return allowlist, nil
}

// UpdateAllowlist implements InstanceAuthSettingsServiceInterface.
func (s *InstanceAuthSettingsService) UpdateAllowlist(
	ctx context.Context, actorUserID string, domains, emails []string, expectedVersion *int64,
) (*models.InstanceAuthAllowlist, error) {
	normDomains, normEmails, err := ValidateInstanceAuthAllowlist(domains, emails)
	if err != nil {
		return nil, err
	}
	allowlist := &models.InstanceAuthAllowlist{Domains: normDomains, Emails: normEmails}
	if err := s.deps.Allowlists.UpsertAudited(ctx, allowlist, optionalActor(actorUserID), expectedVersion); err != nil {
		return nil, fmt.Errorf("failed to store the access allowlist: %w", err)
	}
	s.deps.Caches.Invalidate()
	return allowlist, nil
}

// ResetAllowlist implements InstanceAuthSettingsServiceInterface.
func (s *InstanceAuthSettingsService) ResetAllowlist(ctx context.Context, actorUserID string) error {
	if _, err := s.deps.Allowlists.DeleteAudited(ctx, optionalActor(actorUserID)); err != nil {
		return fmt.Errorf("failed to reset the access allowlist: %w", err)
	}
	s.deps.Caches.Invalidate()
	return nil
}

// PreviewAllowlist implements InstanceAuthSettingsServiceInterface.
func (s *InstanceAuthSettingsService) PreviewAllowlist(
	ctx context.Context, domains, emails []string,
) (*AllowlistImpact, error) {
	return s.deps.AllowlistResolver.PreviewAllowlistImpact(ctx,
		models.InstanceAuthAllowlist{Domains: domains, Emails: emails})
}

// --- policy -------------------------------------------------------------------

// GetPolicy implements InstanceAuthSettingsServiceInterface.
func (s *InstanceAuthSettingsService) GetPolicy(ctx context.Context) (*models.InstanceAuthPolicy, error) {
	policy, err := s.deps.Policies.Get(ctx)
	if errors.Is(err, repositories.ErrInstanceAuthPolicyNotFound) {
		defaults := models.DefaultInstanceAuthPolicy()
		return &defaults, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the authentication policy: %w", err)
	}
	return policy, nil
}

// UpdatePolicy implements InstanceAuthSettingsServiceInterface. Only the two
// settings of the passed policy are stored; its other fields are replaced by
// what the write returns.
func (s *InstanceAuthSettingsService) UpdatePolicy(
	ctx context.Context, actorUserID string, policy models.InstanceAuthPolicy, expectedVersion *int64,
	lockout models.InstanceAuthLockoutContext,
) (*models.InstanceAuthPolicy, error) {
	if err := ValidateInstanceAuthPolicy(policy); err != nil {
		return nil, err
	}
	if !policy.PasswordLoginEnabled && !lockout.Confirmed {
		if err := s.guardPasswordLoginOff(ctx, lockout); err != nil {
			return nil, err
		}
	}
	stored := &models.InstanceAuthPolicy{
		PasswordLoginEnabled: policy.PasswordLoginEnabled,
		RegistrationMode:     policy.RegistrationMode,
	}
	if err := s.deps.Policies.UpsertAudited(ctx, stored, optionalActor(actorUserID), expectedVersion); err != nil {
		return nil, fmt.Errorf("failed to store the authentication policy: %w", err)
	}
	s.deps.Caches.Invalidate()
	return stored, nil
}

// guardPasswordLoginOff is the save guard on a policy that leaves password
// login off (#1438). It judges only a change that takes password login AWAY,
// and refuses it while password login is the only working way in, when the
// caller's own session signed in with a password, or while accounts exist
// whose only sign-in method is a password. Every reason that applies is
// reported, in that order (#1473), so the admin sees the whole risk before
// confirming. The stored passwords are not touched by the change, so
// switching it back on restores every one of them.
func (s *InstanceAuthSettingsService) guardPasswordLoginOff(
	ctx context.Context, caller models.InstanceAuthLockoutContext,
) error {
	current, err := s.GetPolicy(ctx)
	if err != nil {
		return err
	}
	if !current.PasswordLoginEnabled {
		return nil // already off: nothing is taken away
	}

	var reasons []string
	providerEnabled, err := hasEnabledAuthProvider(ctx, s.deps.Providers)
	if err != nil {
		return err
	}
	if !providerEnabled {
		passwordWorks, worksErr := s.deps.WayIn.PasswordWorks(ctx)
		if worksErr != nil {
			return worksErr
		}
		if passwordWorks {
			reasons = append(reasons, AuthLockoutNoUsableMethod)
		}
	}
	if caller.AuthMethod == session.AuthMethodPassword {
		reasons = append(reasons, AuthLockoutOwnMethod)
	}
	passwordOnly, err := s.deps.Credentials.CountPasswordOnly(ctx)
	if err != nil {
		return err
	}
	if passwordOnly > 0 {
		reasons = append(reasons, AuthLockoutPasswordOnlyAccounts)
	}
	if len(reasons) == 0 {
		return nil
	}
	return &ErrAuthLockoutRisk{Reason: reasons[0], Reasons: reasons, PasswordOnlyAccounts: passwordOnly}
}

// BootstrapAdminPassword implements InstanceAuthSettingsServiceInterface.
func (s *InstanceAuthSettingsService) BootstrapAdminPassword(
	ctx context.Context, email, plaintext string,
) (*RootAdminPasswordResult, error) {
	return s.deps.RootPasswords.Set(ctx, email, plaintext, true)
}

// --- instance admins ----------------------------------------------------------

// ListAdmins implements InstanceAuthSettingsServiceInterface. A grant whose
// user was deleted between the two reads is left out.
func (s *InstanceAuthSettingsService) ListAdmins(ctx context.Context) (*models.InstanceAdminList, error) {
	grants, err := s.deps.Grants.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the instance admin grants: %w", err)
	}
	admins := make([]models.InstanceAdminView, 0, len(grants))
	for _, grant := range grants {
		user, err := s.deps.Users.GetByID(ctx, grant.UserID)
		if errors.Is(err, repositories.ErrUserNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to look up instance admin %s: %w", grant.UserID, err)
		}
		admins = append(admins, instanceAdminView(grant, user))
	}
	return &models.InstanceAdminList{RootAdmins: s.deps.Admins.RootAdminEmails(), Admins: admins}, nil
}

func instanceAdminView(grant *models.InstanceAdminGrant, user *models.User) models.InstanceAdminView {
	return models.InstanceAdminView{
		UserID:    grant.UserID,
		Email:     user.Email,
		Name:      user.Name,
		GrantedBy: grant.GrantedBy,
		GrantedAt: grant.CreatedAt,
	}
}

// GrantAdmin implements InstanceAuthSettingsServiceInterface.
func (s *InstanceAuthSettingsService) GrantAdmin(
	ctx context.Context, actingUserID string, target models.InstanceAdminGrantTarget,
) (*models.InstanceAdminView, error) {
	if (target.UserID == "") == (strings.TrimSpace(target.Email) == "") {
		return nil, ErrInstanceAdminGrantTargetAmbiguous
	}
	if err := s.deps.Admins.RequireRootAdmin(ctx, actingUserID); err != nil {
		return nil, err
	}
	user, err := s.grantTargetUser(ctx, target)
	if err != nil {
		return nil, err
	}
	if _, err = s.deps.Admins.GrantInstanceAdmin(ctx, actingUserID, user.ID); err != nil {
		return nil, err
	}

	grants, err := s.deps.Grants.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the instance admin grants: %w", err)
	}
	for _, grant := range grants {
		if grant.UserID == user.ID {
			view := instanceAdminView(grant, user)
			return &view, nil
		}
	}
	// Revoked again between the grant and the read.
	return nil, &ErrInstanceAdminNotGranted{UserID: user.ID}
}

// grantTargetUser resolves the user a grant names, by id or by email. The email
// lookup is case-insensitive (UserRepository.GetByEmail normalizes it). An
// unknown user is ErrInstanceAdminTargetInvalid.
func (s *InstanceAuthSettingsService) grantTargetUser(
	ctx context.Context, target models.InstanceAdminGrantTarget,
) (*models.User, error) {
	if target.UserID != "" {
		return s.lookupGrantTarget(target.UserID, func() (*models.User, error) {
			return s.deps.Users.GetByID(ctx, target.UserID)
		})
	}
	email := strings.TrimSpace(target.Email)
	return s.lookupGrantTarget(email, func() (*models.User, error) {
		return s.deps.Users.GetByEmail(ctx, email)
	})
}

func (s *InstanceAuthSettingsService) lookupGrantTarget(
	named string, lookup func() (*models.User, error),
) (*models.User, error) {
	user, err := lookup()
	if errors.Is(err, repositories.ErrUserNotFound) || (err == nil && user == nil) {
		return nil, &ErrInstanceAdminTargetInvalid{UserID: named, Reason: instanceAdminTargetUnknown}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to look up user %s: %w", named, err)
	}
	return user, nil
}

// RevokeAdmin implements InstanceAuthSettingsServiceInterface.
func (s *InstanceAuthSettingsService) RevokeAdmin(ctx context.Context, actingUserID, targetUserID string) error {
	return s.deps.Admins.RevokeInstanceAdmin(ctx, actingUserID, targetUserID)
}

// trimmedOrNil trims an optional string; a nil or blank one is nil.
func trimmedOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
