package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/auth/password"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// Setting a root admin's password without signing in (#1438, epic #1426).
//
// A password-only instance has nobody who can sign in until its first admin
// has a password, and an operator locked out of one needs a way back. Two
// callers may set a password for an address in auth.instance_admins with no
// session of that account: the setup session (the setup page) and the recovery
// CLI. Both are held by the operator, who is therefore the one vouching for
// the address, and both go through RootAdminPasswordService so they cannot
// differ in what they check or leave behind.

var (
	// ErrNotRootAdmin is returned when the email is not listed in
	// auth.instance_admins.
	ErrNotRootAdmin = errors.New("the email is not a root instance admin (auth.instance_admins)")
	// ErrRootAdminSuspended is returned when the account exists and is
	// suspended: a password would not let it sign in.
	ErrRootAdminSuspended = errors.New("the account is suspended")
	// ErrRootAdminAccountMissing is returned when no account has the email and
	// this caller cannot create one (the CLI: see RootAdminPasswordDeps.Events).
	ErrRootAdminAccountMissing = errors.New("no account exists for the email")
)

// RootAdminPasswordSetter stores a password; PasswordAuthServiceInterface
// satisfies it. Going through it keeps the policy and the hashing the ones
// sign-in uses.
type RootAdminPasswordSetter interface {
	SetPassword(ctx context.Context, userID, plaintext string) error
}

// RootAdminPasswordResult says what Set did.
type RootAdminPasswordResult struct {
	UserID string
	Email  string
	// AccountCreated: no account had this email.
	AccountCreated bool
	// PasswordReplaced: the account already had a password.
	PasswordReplaced bool
	// LoginSwitchedOn: password login was off and this call switched it on.
	LoginSwitchedOn bool
}

// RootAdminPasswordDeps are the collaborators of RootAdminPasswordService.
type RootAdminPasswordDeps struct {
	Users       repositories.UserRepository
	Credentials repositories.UserCredentialRepository
	Policies    repositories.InstanceAuthPolicyRepository
	Passwords   RootAdminPasswordSetter
	// IsRootAdmin reports whether an email is in auth.instance_admins
	// (InstanceAdminResolver.IsRootAdmin).
	IsRootAdmin func(email string) bool
	// Events publishes user.created for an account Set creates; the server's
	// listeners provision the account's workspace from it. Nil means this
	// caller cannot create accounts: the CLI runs with no server and so no
	// listeners, and a row created there would be an account with no
	// workspace. Set then returns ErrRootAdminAccountMissing instead.
	Events events.EventPublisher
	Logger *slog.Logger
}

// RootAdminPasswordService sets a root admin's password on the operator's
// word. Who may call it is the caller's decision (a setup session, the CLI).
type RootAdminPasswordService struct {
	deps RootAdminPasswordDeps
	now  func() time.Time
}

// NewRootAdminPasswordService creates the service.
func NewRootAdminPasswordService(deps RootAdminPasswordDeps) *RootAdminPasswordService {
	return &RootAdminPasswordService{deps: deps, now: time.Now}
}

// Set gives the root admin with email the password plaintext, creating the
// account when there is none (and Events is set), and with enableLogin also
// switches password login on. It returns ErrNotRootAdmin,
// ErrRootAdminSuspended, ErrRootAdminAccountMissing or a *password.PolicyError
// without changing anything.
//
// On an existing account it ends every session first (the password of an
// account someone else may be signed in to is about to change). The email is marked verified
// when it was not: the operator vouches for an address they put in the
// configuration, and without it a later provider sign-in would treat the
// account as unclaimed and remove the password again. Password login is
// switched on last, so a failure before it leaves sign-in as it was.
func (s *RootAdminPasswordService) Set(
	ctx context.Context, email, plaintext string, enableLogin bool,
) (*RootAdminPasswordResult, error) {
	email = models.NormalizeEmail(email)
	if email == "" || !s.deps.IsRootAdmin(email) {
		return nil, ErrNotRootAdmin
	}
	// Before the account is created: a refused password must leave nothing.
	if err := password.Validate(plaintext, email); err != nil {
		return nil, err
	}

	user, created, err := s.account(ctx, email)
	if err != nil {
		return nil, err
	}
	result := &RootAdminPasswordResult{UserID: user.ID, Email: email, AccountCreated: created}
	if !created {
		if result.PasswordReplaced, err = s.hasPassword(ctx, user.ID); err != nil {
			return nil, err
		}
	}

	// Sessions end BEFORE the password changes: if a step fails part-way, the
	// account is never left with a new password and its old sessions alive.
	if err = s.prepareExisting(ctx, user, created); err != nil {
		return nil, err
	}
	if err = s.deps.Passwords.SetPassword(ctx, user.ID, plaintext); err != nil {
		return nil, err
	}
	if enableLogin {
		if result.LoginSwitchedOn, err = s.switchLoginOn(ctx); err != nil {
			return nil, err
		}
	}

	log := s.deps.Logger.With("user_id", user.ID, "email", email,
		"account_created", created, "login_switched_on", result.LoginSwitchedOn)
	if result.PasswordReplaced {
		log.Warn("A root admin's existing password was replaced without a sign-in")
	} else {
		log.Info("A root admin's password was set without a sign-in")
	}
	return result, nil
}

// account returns the account with email, creating it when there is none.
func (s *RootAdminPasswordService) account(ctx context.Context, email string) (*models.User, bool, error) {
	user, err := s.deps.Users.GetByEmail(ctx, email)
	if err == nil {
		if user.IsSuspended() {
			return nil, false, ErrRootAdminSuspended
		}
		return user, false, nil
	}
	if !errors.Is(err, repositories.ErrUserNotFound) {
		return nil, false, fmt.Errorf("failed to look up the root admin account: %w", err)
	}

	if s.deps.Events == nil {
		return nil, false, ErrRootAdminAccountMissing
	}

	now := s.now()
	name, _, _ := strings.Cut(email, "@")
	user = &models.User{Email: email, Name: name, EmailVerifiedAt: &now, CreatedAt: now, UpdatedAt: now}
	if err = s.deps.Users.Create(ctx, user); err != nil {
		return nil, false, fmt.Errorf("failed to create the root admin account: %w", err)
	}
	// Without the event nothing provisions the account's workspace, so a row
	// left behind would be a broken account: remove it, as admin user creation
	// does. Nothing has cascaded yet.
	event := events.NewUserCreatedEvent(user.ID, user.Email, user.Name, user.CreatedAt)
	if err = s.deps.Events.Publish(ctx, event); err != nil {
		if delErr := s.deps.Users.DeleteByID(ctx, user.ID); delErr != nil {
			return nil, false, fmt.Errorf(
				"failed to provision the root admin account (%w) and to remove it again: %w", err, delErr)
		}
		return nil, false, fmt.Errorf("failed to provision the root admin account: %w", err)
	}
	return user, true, nil
}

func (s *RootAdminPasswordService) hasPassword(ctx context.Context, userID string) (bool, error) {
	_, err := s.deps.Credentials.Get(ctx, userID)
	if errors.Is(err, repositories.ErrUserCredentialNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read the root admin's credential: %w", err)
	}
	return true, nil
}

// prepareExisting verifies the email of an account that existed and ends its
// sessions. A new account is created verified and has no session.
func (s *RootAdminPasswordService) prepareExisting(ctx context.Context, user *models.User, created bool) error {
	if created {
		return nil
	}
	if user.EmailVerifiedAt == nil {
		now := s.now()
		user.EmailVerifiedAt = &now
		if err := s.deps.Users.Update(ctx, user); err != nil {
			return fmt.Errorf("failed to mark the root admin's email verified: %w", err)
		}
	}
	if _, err := s.deps.Users.BumpSessionEpoch(ctx, user.ID); err != nil {
		return fmt.Errorf("failed to end the root admin's sessions: %w", err)
	}
	return nil
}

// switchLoginOn turns password login on, keeping the stored registration mode,
// and reports whether it changed anything. The write is audited with no actor;
// its source is the context's (repositories.WithAuditSource).
//
// It is a compare-and-set on the version it read: an admin saving the policy
// between the read and the write (a registration mode, say) is re-read rather
// than overwritten with the stale value. One retry; a second conflict is an
// error.
func (s *RootAdminPasswordService) switchLoginOn(ctx context.Context) (bool, error) {
	for attempt := 0; ; attempt++ {
		changed, err := s.trySwitchLoginOn(ctx)
		if errors.Is(err, repositories.ErrInstanceSettingsVersionConflict) && attempt == 0 {
			continue
		}
		return changed, err
	}
}

func (s *RootAdminPasswordService) trySwitchLoginOn(ctx context.Context) (bool, error) {
	policy, err := s.deps.Policies.Get(ctx)
	expected := repositories.InstanceSettingsNoStoredVersion
	switch {
	case errors.Is(err, repositories.ErrInstanceAuthPolicyNotFound):
		defaults := models.DefaultInstanceAuthPolicy()
		policy = &defaults
	case err != nil:
		return false, fmt.Errorf("failed to read the authentication policy: %w", err)
	default:
		expected = policy.Version
	}
	if policy.PasswordLoginEnabled {
		return false, nil
	}
	next := &models.InstanceAuthPolicy{PasswordLoginEnabled: true, RegistrationMode: policy.RegistrationMode}
	if err = s.deps.Policies.UpsertAudited(ctx, next, nil, &expected); err != nil {
		return false, fmt.Errorf("failed to switch password login on: %w", err)
	}
	return true, nil
}
