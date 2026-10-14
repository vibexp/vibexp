package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Self-service account (#1437): what a signed-in user reads and changes about
// their own account. The rules that depend on the HTTP session (which
// credential authenticated the request, how long ago it signed in, re-issuing
// its cookie) stay in the handler and reach this service as plain values.

// AccountNameMaxLength is the longest display name, in characters
// (users.name is varchar(255)).
const AccountNameMaxLength = 255

var (
	// ErrReauthenticationRequired is returned when an operation needs a recent
	// sign-in and the session is older: setting a first password, removing the
	// password, disconnecting an identity. The person signs in again and
	// repeats it.
	ErrReauthenticationRequired = errors.New("a recent sign-in is required")
	// ErrLastSignInMethod is returned when removing a password or an identity
	// would leave the account with no usable way to sign in.
	ErrLastSignInMethod = errors.New("the account's last usable sign-in method cannot be removed")
	// ErrAccountNameInvalid is returned for a display name that is blank or
	// longer than AccountNameMaxLength.
	ErrAccountNameInvalid = errors.New("display name must be 1 to 255 characters")
	// ErrAccountPasswordNotSet is returned when removing a password the
	// account does not have.
	ErrAccountPasswordNotSet = errors.New("the account has no password")
	// ErrAccountIdentityNotFound is returned when disconnecting an identity
	// the account does not have.
	ErrAccountIdentityNotFound = errors.New("the account has no such sign-in identity")
	// ErrDisconnectedIdentityNotFound is returned when allowing again a
	// disconnected identity the account does not have.
	ErrDisconnectedIdentityNotFound = errors.New("the account has no such disconnected identity")
)

// AccountNotifier is told about changes to an account that its owner should
// hear about out of band. The default does nothing; the password-changed email
// (#1433) implements it.
type AccountNotifier interface {
	// PasswordChanged is called after the user's password was set, changed,
	// reset, removed or disabled; kind says which, and the notice says so. It
	// must not fail the operation: an error is logged by the implementation,
	// not returned.
	PasswordChanged(ctx context.Context, userID string, kind AccountChangeKind)
}

// AccountChangeKind is what happened to an account's password, as its owner
// is told.
type AccountChangeKind string

const (
	// AccountChangeSet: a password was set where there was none (the account
	// page, or a setup link).
	AccountChangeSet AccountChangeKind = "set"
	// AccountChangeChanged: the owner changed the password.
	AccountChangeChanged AccountChangeKind = "changed"
	// AccountChangeReset: a new password was set through a reset link.
	AccountChangeReset AccountChangeKind = "reset"
	// AccountChangeRemoved: the owner removed the password.
	AccountChangeRemoved AccountChangeKind = "removed"
	// AccountChangeRemovedByAdmin: an administrator removed the password.
	AccountChangeRemovedByAdmin AccountChangeKind = "removed_by_admin"
	// AccountChangeDisabledByAdmin: an administrator disabled the password.
	AccountChangeDisabledByAdmin AccountChangeKind = "disabled_by_admin"
)

type noopAccountNotifier struct{}

func (noopAccountNotifier) PasswordChanged(context.Context, string, AccountChangeKind) {}

// AccountView is an account as its owner sees it.
type AccountView struct {
	User       *models.User
	Password   AccountPasswordView
	Identities []AccountIdentityView
	// Disconnected are the provider accounts the owner disconnected and has
	// not allowed again (#1460), newest first.
	Disconnected []AccountDisconnectedView
}

// AccountDisconnectedView is a provider account the owner disconnected.
type AccountDisconnectedView struct {
	ID       string
	Provider string
	// DisplayName is the provider's name on the sign-in page, or its slug when
	// the provider is not enabled.
	DisplayName    string
	DisconnectedAt time.Time
}

// AccountPasswordView is the password sign-in method of an account.
type AccountPasswordView struct {
	// Set reports whether the account has a password.
	Set bool
	// ChangedAt is when it was last set or changed; nil when there is none.
	ChangedAt *time.Time
	// LoginEnabled reports whether password sign-in is on for the instance.
	LoginEnabled bool
	// Disabled reports whether an administrator disabled the password (#1461).
	Disabled bool
	// usable reports whether the password can actually sign in right now.
	usable bool
}

// AccountIdentityView is one identity provider an account can sign in with.
type AccountIdentityView struct {
	ID       string
	Provider string
	// DisplayName is the provider's name on the sign-in page, or its slug when
	// the provider is not enabled.
	DisplayName string
	// Available reports whether the provider is currently enabled.
	Available   bool
	ConnectedAt time.Time
	LastUsedAt  *time.Time
}

// AccountPasswordChange is a request to set or change the account's password.
type AccountPasswordChange struct {
	// Current is the current password; required when the account has one.
	Current string
	// New is the new password.
	New string
	// RecentSignIn reports whether the session signed in recently; required
	// when the account has no password yet.
	RecentSignIn bool
}

// AccountServiceInterface is the self-service account API.
type AccountServiceInterface interface {
	// Get returns the user's account.
	Get(ctx context.Context, userID string) (*AccountView, error)
	// UpdateName changes the display name and returns the account.
	// ErrAccountNameInvalid for a blank or over-long name.
	UpdateName(ctx context.Context, userID, name string) (*AccountView, error)
	// SetPassword sets a first password or changes the existing one, and
	// reports which it was. Changing requires the current password
	// (ErrCurrentPasswordInvalid, ErrPasswordAttemptsExceeded); a first
	// password requires a recent sign-in (ErrReauthenticationRequired).
	// ErrPasswordLoginDisabled while the instance has password sign-in off;
	// ErrPasswordDisabled while an administrator has the password disabled; a
	// *password.PolicyError when the new password breaks the policy.
	SetPassword(ctx context.Context, userID string, change AccountPasswordChange) (first bool, err error)
	// RemovePassword removes the password. It requires a recent sign-in and
	// another usable sign-in method, and is refused (ErrPasswordDisabled) for a
	// password an administrator disabled.
	RemovePassword(ctx context.Context, userID string, recentSignIn bool) error
	// AdminRemovePassword removes the password for an administrator (#1436):
	// a disabled one too, and the last sign-in method when allowLast. The
	// owner is told. ErrAccountPasswordNotSet when there is none,
	// ErrLastSignInMethod when it is the last way in and not allowLast.
	AdminRemovePassword(ctx context.Context, userID string, allowLast bool) error
	// DisconnectIdentity removes one identity and returns its provider slug.
	// It requires a recent sign-in and another usable sign-in method.
	DisconnectIdentity(ctx context.Context, userID, identityID string, recentSignIn bool) (provider string, err error)
	// AllowIdentityAgain lets a disconnected provider account link to the
	// account by email again (#1460) and returns its provider slug. It
	// requires a recent sign-in; ErrDisconnectedIdentityNotFound when the
	// account has no such record.
	AllowIdentityAgain(ctx context.Context, userID, id string, recentSignIn bool) (provider string, err error)
	// RevokeAPIAccess deletes the user's API keys and MCP/OAuth grants (the
	// "also revoke API keys and connected tools" option of a password change).
	RevokeAPIAccess(ctx context.Context, userID string) error
}

// AccountServiceDeps are the collaborators of AccountService.
type AccountServiceDeps struct {
	Users      repositories.UserRepository
	Identities repositories.UserIdentityRepository
	// Methods removes sign-in methods, never the last usable one.
	Methods   repositories.SignInMethodRepository
	Passwords PasswordAuthServiceInterface
	Policy    AuthPolicyResolver
	Providers IdentityProviderResolver
	APIAccess APIAccessServiceInterface
	// Notifier is told about password changes; nil means nobody is.
	Notifier AccountNotifier
	Logger   *slog.Logger
}

// AccountService implements AccountServiceInterface.
type AccountService struct {
	users      repositories.UserRepository
	identities repositories.UserIdentityRepository
	methods    repositories.SignInMethodRepository
	passwords  PasswordAuthServiceInterface
	policy     AuthPolicyResolver
	providers  IdentityProviderResolver
	apiAccess  APIAccessServiceInterface
	notifier   AccountNotifier
	logger     *slog.Logger
}

var _ AccountServiceInterface = (*AccountService)(nil)

// NewAccountService creates the self-service account service.
func NewAccountService(deps AccountServiceDeps) *AccountService {
	notifier := deps.Notifier
	if notifier == nil {
		notifier = noopAccountNotifier{}
	}
	return &AccountService{
		users:      deps.Users,
		identities: deps.Identities,
		methods:    deps.Methods,
		passwords:  deps.Passwords,
		policy:     deps.Policy,
		providers:  deps.Providers,
		apiAccess:  deps.APIAccess,
		notifier:   notifier,
		logger:     deps.Logger,
	}
}

// Get implements AccountServiceInterface.
func (s *AccountService) Get(ctx context.Context, userID string) (*AccountView, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load the account: %w", err)
	}
	passwordView, err := s.passwordView(ctx, userID)
	if err != nil {
		return nil, err
	}
	identities, err := s.identityViews(ctx, userID)
	if err != nil {
		return nil, err
	}
	disconnected, err := s.disconnectedViews(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &AccountView{
		User: user, Password: passwordView, Identities: identities, Disconnected: disconnected,
	}, nil
}

// passwordView reads the password's state and whether the instance allows
// password sign-in.
func (s *AccountService) passwordView(ctx context.Context, userID string) (AccountPasswordView, error) {
	policy, err := s.policy.Policy(ctx)
	if err != nil {
		return AccountPasswordView{}, fmt.Errorf("read the authentication policy: %w", err)
	}
	status, err := s.passwords.PasswordStatus(ctx, userID)
	if err != nil {
		return AccountPasswordView{}, err
	}
	view := AccountPasswordView{
		Set:          status.Set,
		LoginEnabled: policy.PasswordLoginEnabled,
		Disabled:     status.Set && !status.Usable,
		usable:       status.Usable && policy.PasswordLoginEnabled,
	}
	if status.Set {
		changedAt := status.ChangedAt
		view.ChangedAt = &changedAt
	}
	return view, nil
}

// hiddenIdentityProvider reports whether an identity row is internal
// bookkeeping rather than a sign-in method its owner manages: the legacy Google
// alias of another identity, and the localhost-only dev login.
func hiddenIdentityProvider(provider string) bool {
	return provider == models.LegacyGoogleIdentityProvider || provider == devIDPProvider
}

// identityViews lists the account's identities, oldest first, each with the
// name and availability of its provider.
func (s *AccountService) identityViews(ctx context.Context, userID string) ([]AccountIdentityView, error) {
	identities, err := s.identities.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list the account's sign-in identities: %w", err)
	}
	registry, err := s.providers.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIdentityProvidersUnresolvable, err)
	}
	enabled := map[string]string{}
	for _, entry := range registry.Entries() {
		enabled[string(entry.Provider.Name())] = entry.DisplayName
	}

	views := make([]AccountIdentityView, 0, len(identities))
	for i := range identities {
		identity := identities[i]
		if hiddenIdentityProvider(identity.Provider) {
			continue
		}
		displayName, available := enabled[identity.Provider]
		if !available {
			displayName = identity.Provider
		}
		views = append(views, AccountIdentityView{
			ID:          identity.ID,
			Provider:    identity.Provider,
			DisplayName: displayName,
			Available:   available,
			ConnectedAt: identity.CreatedAt,
			LastUsedAt:  identity.LastUsedAt,
		})
	}
	return views, nil
}

// UpdateName implements AccountServiceInterface.
func (s *AccountService) UpdateName(ctx context.Context, userID, name string) (*AccountView, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > AccountNameMaxLength {
		return nil, ErrAccountNameInvalid
	}
	if err := s.users.UpdateName(ctx, userID, name); err != nil {
		return nil, fmt.Errorf("update the display name: %w", err)
	}
	s.logger.With("user_id", userID).Info("Account display name changed")
	return s.Get(ctx, userID)
}

// SetPassword implements AccountServiceInterface.
func (s *AccountService) SetPassword(
	ctx context.Context, userID string, change AccountPasswordChange,
) (bool, error) {
	passwordView, err := s.passwordView(ctx, userID)
	if err != nil {
		return false, err
	}
	if !passwordView.LoginEnabled {
		return false, ErrPasswordLoginDisabled
	}
	if passwordView.Disabled {
		return false, ErrPasswordDisabled
	}

	first := !passwordView.Set
	if first {
		// No password to prove knowledge of, so the session itself has to be
		// fresh: an unattended, long-open browser must not be enough to add a
		// way in.
		if !change.RecentSignIn {
			return false, ErrReauthenticationRequired
		}
	} else {
		// An empty value cannot be the password; refusing it here keeps a
		// client that forgot the field from spending the account's attempts.
		if change.Current == "" {
			return false, ErrCurrentPasswordInvalid
		}
		if err = s.passwords.VerifyPassword(ctx, userID, change.Current); err != nil {
			return false, err
		}
	}

	if err = s.passwords.ChangeOwnPassword(ctx, userID, change.New); err != nil {
		return false, err
	}
	kind := AccountChangeChanged
	if first {
		kind = AccountChangeSet
	}
	s.notifier.PasswordChanged(ctx, userID, kind)
	return first, nil
}

// RemovePassword implements AccountServiceInterface.
func (s *AccountService) RemovePassword(ctx context.Context, userID string, recentSignIn bool) error {
	if !recentSignIn {
		return ErrReauthenticationRequired
	}
	usable, err := s.usableMethods(ctx)
	if err != nil {
		return err
	}
	if err = s.methods.RemovePassword(ctx, userID, usable, repositories.RemovePasswordOptions{}); err != nil {
		return removalError("remove the password", err)
	}
	s.logger.With("user_id", userID).Info("Password removed")
	s.notifier.PasswordChanged(ctx, userID, AccountChangeRemoved)
	return nil
}

// AdminRemovePassword implements AccountServiceInterface.
func (s *AccountService) AdminRemovePassword(ctx context.Context, userID string, allowLast bool) error {
	usable, err := s.usableMethods(ctx)
	if err != nil {
		return err
	}
	err = s.methods.RemovePassword(ctx, userID, usable,
		repositories.RemovePasswordOptions{AllowLast: allowLast, AllowDisabled: true})
	if err != nil {
		return removalError("remove the password", err)
	}
	s.logger.With("user_id", userID, "allow_last", allowLast).Info("Password removed by an administrator")
	s.notifier.PasswordChanged(ctx, userID, AccountChangeRemovedByAdmin)
	return nil
}

// DisconnectIdentity implements AccountServiceInterface.
func (s *AccountService) DisconnectIdentity(
	ctx context.Context, userID, identityID string, recentSignIn bool,
) (string, error) {
	if !recentSignIn {
		return "", ErrReauthenticationRequired
	}
	usable, err := s.usableMethods(ctx)
	if err != nil {
		return "", err
	}
	provider, err := s.methods.RemoveIdentity(ctx, userID, identityID, usable)
	if err != nil {
		return "", removalError("disconnect the sign-in identity", err)
	}
	s.logger.With("user_id", userID, "provider", provider).Info("Sign-in identity disconnected")
	return provider, nil
}

// usableMethods says which sign-in methods WORK on the instance right now: a
// password only while password sign-in is switched on, an identity only while
// its provider is enabled. The repository applies it to the account's rows
// under a lock, so "another usable method remains" is checked and the method
// deleted with nothing in between. Counting anything else would let a user
// remove their way back in.
func (s *AccountService) usableMethods(ctx context.Context) (repositories.UsableSignInMethods, error) {
	policy, err := s.policy.Policy(ctx)
	if err != nil {
		return repositories.UsableSignInMethods{}, fmt.Errorf("read the authentication policy: %w", err)
	}
	registry, err := s.providers.Snapshot(ctx)
	if err != nil {
		return repositories.UsableSignInMethods{}, fmt.Errorf("%w: %w", ErrIdentityProvidersUnresolvable, err)
	}
	entries := registry.Entries()
	usable := repositories.UsableSignInMethods{
		Providers:     make([]string, 0, len(entries)),
		PasswordLogin: policy.PasswordLoginEnabled,
	}
	for _, entry := range entries {
		usable.Providers = append(usable.Providers, string(entry.Provider.Name()))
	}
	return usable, nil
}

// removalError maps the repository's refusals to the service's.
func removalError(action string, err error) error {
	switch {
	case errors.Is(err, repositories.ErrLastSignInMethod):
		return ErrLastSignInMethod
	case errors.Is(err, repositories.ErrUserCredentialNotFound):
		return ErrAccountPasswordNotSet
	case errors.Is(err, repositories.ErrUserCredentialDisabled):
		return ErrPasswordDisabled
	case errors.Is(err, repositories.ErrUserIdentityNotFound):
		return ErrAccountIdentityNotFound
	}
	return fmt.Errorf("%s: %w", action, err)
}

// disconnectedViews lists the provider accounts the owner disconnected, each
// with its provider's name.
func (s *AccountService) disconnectedViews(ctx context.Context, userID string) ([]AccountDisconnectedView, error) {
	disconnected, err := s.methods.ListDisconnected(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list the account's disconnected identities: %w", err)
	}
	if len(disconnected) == 0 {
		return []AccountDisconnectedView{}, nil
	}
	registry, err := s.providers.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIdentityProvidersUnresolvable, err)
	}
	names := map[string]string{}
	for _, entry := range registry.Entries() {
		names[string(entry.Provider.Name())] = entry.DisplayName
	}
	views := make([]AccountDisconnectedView, 0, len(disconnected))
	for _, d := range disconnected {
		name, ok := names[d.Provider]
		if !ok {
			name = d.Provider
		}
		views = append(views, AccountDisconnectedView{
			ID: d.ID, Provider: d.Provider, DisplayName: name, DisconnectedAt: d.DisconnectedAt,
		})
	}
	return views, nil
}

// AllowIdentityAgain implements AccountServiceInterface.
func (s *AccountService) AllowIdentityAgain(
	ctx context.Context, userID, id string, recentSignIn bool,
) (string, error) {
	if !recentSignIn {
		return "", ErrReauthenticationRequired
	}
	provider, found, err := s.methods.AllowIdentityAgain(ctx, userID, id)
	if err != nil {
		return "", fmt.Errorf("allow the identity again: %w", err)
	}
	if !found {
		return "", ErrDisconnectedIdentityNotFound
	}
	s.logger.With("user_id", userID, "provider", provider).Info("A disconnected sign-in identity was allowed again")
	return provider, nil
}

// RevokeAPIAccess implements AccountServiceInterface.
func (s *AccountService) RevokeAPIAccess(ctx context.Context, userID string) error {
	if _, err := s.apiAccess.RevokeAPIAccess(ctx, userID); err != nil {
		return err
	}
	return nil
}
