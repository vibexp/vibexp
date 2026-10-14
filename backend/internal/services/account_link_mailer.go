package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Emailing one-time account links, and the password-changed notice (#1433,
// epic #1426).
//
// # Whether email works
//
// An instance with no email provider "sends" successfully: the fallback is a
// stub that discards the message. So nothing here infers that email works from
// a send. The one source is AuthPolicyResolver.PasswordResetChannel, which
// reads the STORED provider row. When it does not say email, no link is
// issued and no send is attempted.
//
// # The link and the mailer
//
// A link is issued with AccountTokenService.IssueByEmail, which hands it only
// to the send function given here. That function passes it to EmailService and
// nowhere else; it is not logged, returned or stored.

const (
	// passwordResetEmailCooldown is the least time between two reset emails to
	// one account. It is enforced where the link is stored, in the same
	// transaction and under the user's lock, so it holds across replicas and
	// against simultaneous requests. With the per-IP limit on the endpoint it
	// bounds how fast anyone can fill a mailbox.
	passwordResetEmailCooldown = 5 * time.Minute

	// Why a reset request sent no email. For logs and tests; a caller of the
	// endpoint never sees it.
	resetSkippedNoEmail       = "no_email_provider"
	resetSkippedUnknown       = "unknown_address"
	resetSkippedSuspended     = "suspended"
	resetSkippedDisabled      = "credential_disabled"
	resetSkippedNotAllowed    = "access_restricted"
	resetSkippedShared        = "unverified_with_identity"
	resetSkippedCooldown      = "cooldown"
	accountEmailTaskReset     = "password_reset_email"
	accountEmailTaskChanged   = "password_changed_email"
	backgroundAccountEmails   = 16
	backgroundAccountEmailTTL = 30 * time.Second
)

// ErrAccountEmailUnavailable is returned by EmailSetupLink when the instance
// has no email provider stored, so no link was issued and nothing was sent.
// The caller should offer a copied link instead.
var ErrAccountEmailUnavailable = errors.New("the instance has no email provider")

// ErrAccountEmailUnverifiedShared refuses to email a link to an account whose
// address was never verified while somebody signs in to it with a provider:
// the mailbox owner and that person may be two people (see
// unverifiedWithIdentity). An administrator can share a link instead.
var ErrAccountEmailUnverifiedShared = errors.New(
	"the address is not verified and someone signs in to this account with a provider; share a link instead")

// AccountEmailRunners are the two background runners of the account emails.
// They are separate on purpose: Requests serves a public endpoint that anyone
// can fill, and Notices carries the owner's only out-of-band signal that their
// password changed. Sharing one would let a flood of reset requests crowd the
// notices out.
type AccountEmailRunners struct {
	// Requests runs forgot-password requests.
	Requests *BackgroundRunner
	// Notices runs password-changed notices.
	Notices *BackgroundRunner
}

// NewAccountEmailRunners creates the two runners.
func NewAccountEmailRunners(logger *slog.Logger) *AccountEmailRunners {
	return &AccountEmailRunners{
		Requests: NewBackgroundRunner(backgroundAccountEmails, backgroundAccountEmailTTL, logger),
		Notices:  NewBackgroundRunner(backgroundAccountEmails, backgroundAccountEmailTTL, logger),
	}
}

// Shutdown drains both runners until ctx ends, then cancels what is left. The
// two shut down together, so the wait is one grace period and not two.
func (r *AccountEmailRunners) Shutdown(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Go(func() { r.Requests.Shutdown(ctx) })
	wg.Go(func() { r.Notices.Shutdown(ctx) })
	wg.Wait()
}

// AccountLinkMailerInterface emails one-time account links.
type AccountLinkMailerInterface interface {
	// EmailSetupLink issues a setup link for the user and emails it to the
	// account's address. It returns when the link expires, or
	// ErrAccountEmailUnavailable when the instance has no email provider
	// stored (nothing is issued). A failed send is returned and the link is
	// withdrawn. createdBy is the administrator acting, or empty.
	EmailSetupLink(ctx context.Context, userID, createdBy string) (time.Time, error)
	// EmailResetLink issues a reset link for the user and emails it: an
	// administrator's reset (#1436). It refuses as CheckResetEmail does; a
	// failed send is returned and the link is withdrawn.
	EmailResetLink(ctx context.Context, userID, createdBy string) (time.Time, error)
	// CheckResetEmail issues and sends nothing: it reports why EmailResetLink
	// would refuse the user (ErrAccountEmailUnavailable,
	// ErrAccountEmailUnverifiedShared), or nil. It lets a caller refuse before
	// it changes anything.
	CheckResetEmail(ctx context.Context, userID string) error
	// RequestPasswordReset is the forgot-password request for an address. It
	// returns at once and never reports anything about the address: whether a
	// reset email is sent is decided, and the email sent, in the background.
	RequestPasswordReset(email string)
}

// AccountLinkMailerDeps are the collaborators of AccountLinkMailer.
type AccountLinkMailerDeps struct {
	Tokens     AccountTokenServiceInterface
	Users      repositories.UserRepository
	Identities repositories.UserIdentityRepository
	Passwords  PasswordAuthServiceInterface
	Policy     AuthPolicyResolver
	Allowlist  AccessAllowlistResolver
	Email      AccountEmailSender
	// Runner runs forgot-password requests (AccountEmailRunners.Requests).
	Runner *BackgroundRunner
	Logger *slog.Logger
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// AccountLinkMailer implements AccountLinkMailerInterface.
type AccountLinkMailer struct {
	tokens     AccountTokenServiceInterface
	users      repositories.UserRepository
	identities repositories.UserIdentityRepository
	passwords  PasswordAuthServiceInterface
	policy     AuthPolicyResolver
	allowlist  AccessAllowlistResolver
	email      AccountEmailSender
	runner     *BackgroundRunner
	logger     *slog.Logger
	now        func() time.Time
}

var _ AccountLinkMailerInterface = (*AccountLinkMailer)(nil)

// NewAccountLinkMailer creates the mailer.
func NewAccountLinkMailer(deps AccountLinkMailerDeps) *AccountLinkMailer {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &AccountLinkMailer{
		tokens: deps.Tokens, users: deps.Users, identities: deps.Identities, passwords: deps.Passwords,
		policy: deps.Policy, allowlist: deps.Allowlist, email: deps.Email, runner: deps.Runner,
		logger: deps.Logger, now: now,
	}
}

// emailWorks reports whether an instance email provider is stored.
func (m *AccountLinkMailer) emailWorks(ctx context.Context) (bool, error) {
	channel, err := m.policy.PasswordResetChannel(ctx)
	if err != nil {
		return false, fmt.Errorf("read how password resets are delivered: %w", err)
	}
	return channel == PasswordResetByEmail, nil
}

// EmailSetupLink implements AccountLinkMailerInterface.
func (m *AccountLinkMailer) EmailSetupLink(ctx context.Context, userID, createdBy string) (time.Time, error) {
	works, err := m.emailWorks(ctx)
	if err != nil {
		return time.Time{}, err
	}
	if !works {
		return time.Time{}, ErrAccountEmailUnavailable
	}
	return m.tokens.IssueByEmail(ctx,
		IssueAccountLinkParams{UserID: userID, Purpose: models.AccountTokenPurposeSetup, CreatedBy: createdBy},
		func(ctx context.Context, link AccountLinkEmail) error {
			return m.email.SendAccountSetup(ctx, accountLinkMessage(link, m.now()))
		})
}

// CheckResetEmail implements AccountLinkMailerInterface.
func (m *AccountLinkMailer) CheckResetEmail(ctx context.Context, userID string) error {
	works, err := m.emailWorks(ctx)
	if err != nil {
		return err
	}
	if !works {
		return ErrAccountEmailUnavailable
	}
	user, err := m.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("load the account for a reset email: %w", err)
	}
	shared, err := m.unverifiedWithIdentity(ctx, user)
	if err != nil {
		return err
	}
	if shared {
		return ErrAccountEmailUnverifiedShared
	}
	return nil
}

// EmailResetLink implements AccountLinkMailerInterface. Unlike a
// forgot-password request it has no cooldown, and a disabled password does not
// stop it: the administrator decides.
func (m *AccountLinkMailer) EmailResetLink(ctx context.Context, userID, createdBy string) (time.Time, error) {
	if err := m.CheckResetEmail(ctx, userID); err != nil {
		return time.Time{}, err
	}
	return m.tokens.IssueByEmail(ctx,
		IssueAccountLinkParams{UserID: userID, Purpose: models.AccountTokenPurposeReset, CreatedBy: createdBy},
		func(ctx context.Context, link AccountLinkEmail) error {
			return m.email.SendPasswordReset(ctx, accountLinkMessage(link, m.now()))
		})
}

// accountLinkMessage turns an issued link into the message the email service
// sends. This is the one place a link is handed to the mailer.
func accountLinkMessage(link AccountLinkEmail, now time.Time) AccountLinkMessage {
	return AccountLinkMessage{
		To:       link.User.Email,
		Name:     link.User.Name,
		LinkURL:  link.URL,
		ValidFor: roundLinkLifetime(link.ExpiresAt.Sub(now)),
	}
}

// roundLinkLifetime rounds the time a link has left to the minute. The email
// is written a moment after the link is issued, so the time left is a hair
// under the nominal lifetime; without rounding "1 hour" would be written as
// "60 minutes".
func roundLinkLifetime(left time.Duration) time.Duration {
	return left.Round(time.Minute)
}

// RequestPasswordReset implements AccountLinkMailerInterface.
func (m *AccountLinkMailer) RequestPasswordReset(email string) {
	m.runner.Go(accountEmailTaskReset, func(ctx context.Context) {
		skipped, err := m.sendPasswordReset(ctx, email)
		switch {
		case err != nil:
			// The error names what failed; it never contains the link.
			m.logger.With("error", err.Error()).Error("A password reset email could not be sent")
		case skipped != "":
			// No address: for most of these reasons it is not an account's.
			m.logger.With("reason", skipped).Info("Password reset requested; no email sent")
		}
	})
}

// sendPasswordReset decides whether the address gets a reset email and sends
// it. It returns why none was sent, or "" when one was.
//
// An account is eligible when it exists, is not suspended, an active allowlist
// admits its address, its password was not disabled by an administrator, and
// it is not an unverified account somebody already signs in to with a provider
// (see unverifiedWithIdentity).
// An account with NO password is eligible: a person whose setup link expired
// has no other way in, and an emailed link proves the mailbox. A disabled
// password is not: storing a new password clears the disabled mark, so a reset
// would let the person undo the administrator's action.
func (m *AccountLinkMailer) sendPasswordReset(ctx context.Context, email string) (skipped string, err error) {
	works, err := m.emailWorks(ctx)
	if err != nil {
		return "", err
	}
	if !works {
		return resetSkippedNoEmail, nil
	}
	user, skipped, err := m.resetRecipient(ctx, email)
	if err != nil || skipped != "" {
		return skipped, err
	}

	// MinInterval is the cooldown: the token service refuses, without issuing
	// or sending, while the last emailed reset link is younger than it.
	_, err = m.tokens.IssueByEmail(ctx,
		IssueAccountLinkParams{
			UserID: user.ID, Purpose: models.AccountTokenPurposeReset, MinInterval: passwordResetEmailCooldown,
		},
		func(ctx context.Context, link AccountLinkEmail) error {
			return m.email.SendPasswordReset(ctx, accountLinkMessage(link, m.now()))
		})
	if errors.Is(err, ErrAccountLinkTooRecent) {
		return resetSkippedCooldown, nil
	}
	if err != nil {
		return "", fmt.Errorf("email a password reset link: %w", err)
	}
	m.logger.With("user_id", user.ID).Info("Password reset email sent")
	return "", nil
}

// resetRecipient returns the account a reset email for this address would go
// to, or why there is none. See sendPasswordReset for the rules.
func (m *AccountLinkMailer) resetRecipient(
	ctx context.Context, email string,
) (user *models.User, skipped string, err error) {
	user, err = m.users.GetByEmail(ctx, models.NormalizeEmail(email))
	if errors.Is(err, repositories.ErrUserNotFound) {
		return nil, resetSkippedUnknown, nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("look up the account for a password reset: %w", err)
	}
	if user.IsSuspended() {
		return nil, resetSkippedSuspended, nil
	}
	err = ensureEmailAllowed(ctx, m.allowlist, m.logger, user.Email, LocalSignInProvider, true)
	if errors.Is(err, ErrAccessRestricted) {
		return nil, resetSkippedNotAllowed, nil
	}
	if err != nil {
		return nil, "", err
	}
	status, err := m.passwords.PasswordStatus(ctx, user.ID)
	if err != nil {
		return nil, "", fmt.Errorf("read the password status for a password reset: %w", err)
	}
	if status.Set && !status.Usable {
		return nil, resetSkippedDisabled, nil
	}
	shared, err := m.unverifiedWithIdentity(ctx, user)
	if err != nil {
		return nil, "", err
	}
	if shared {
		return nil, resetSkippedShared, nil
	}
	return user, "", nil
}

// unverifiedWithIdentity reports whether the account's address was never
// verified AND somebody already signs in to it with a provider.
//
// Such an account must not get an emailed link. The mailbox's owner and the
// person behind the provider identity may be two different people: that is
// exactly why a provider sign-in with a verified address is refused for it
// (ErrExistingSignInRequired). Completing an emailed link would put the
// mailbox owner's password on the account and mark the address verified while
// the other person's identity, sessions' successors and API keys stay attached.
// An unverified account with no identity is fine (nobody else is in it), and
// so is a verified one.
func (m *AccountLinkMailer) unverifiedWithIdentity(ctx context.Context, user *models.User) (bool, error) {
	if user.EmailVerifiedAt != nil {
		return false, nil
	}
	if user.IDPSubject != nil && *user.IDPSubject != "" {
		return true, nil
	}
	identities, err := m.identities.ListByUserID(ctx, user.ID)
	if err != nil {
		return false, fmt.Errorf("read the account's sign-in identities for a password reset: %w", err)
	}
	return len(identities) > 0, nil
}

// AccountEmailNotifierDeps are the collaborators of the notifier.
type AccountEmailNotifierDeps struct {
	Users  repositories.UserRepository
	Policy AuthPolicyResolver
	Email  AccountEmailSender
	Runner *BackgroundRunner
	Logger *slog.Logger
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// accountEmailNotifier is the AccountNotifier that emails the account's owner.
type accountEmailNotifier struct {
	users  repositories.UserRepository
	policy AuthPolicyResolver
	email  AccountEmailSender
	runner *BackgroundRunner
	logger *slog.Logger
	now    func() time.Time
}

// NewAccountEmailNotifier creates the AccountNotifier that sends the
// password-changed notice. The notice goes out in the background: it never
// delays the change it reports and never fails it. It is sent only when an
// instance email provider is stored, and it carries no link.
func NewAccountEmailNotifier(deps AccountEmailNotifierDeps) AccountNotifier {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &accountEmailNotifier{
		users: deps.Users, policy: deps.Policy, email: deps.Email, runner: deps.Runner,
		logger: deps.Logger, now: now,
	}
}

// PasswordChanged implements AccountNotifier.
func (n *accountEmailNotifier) PasswordChanged(_ context.Context, userID string, kind AccountChangeKind) {
	changedAt := n.now()
	accepted := n.runner.Go(accountEmailTaskChanged, func(ctx context.Context) {
		if err := n.sendPasswordChanged(ctx, userID, kind, changedAt); err != nil {
			n.logger.With("user_id", userID, "error", err.Error()).
				Error("The password-changed notice could not be sent")
		}
	})
	if !accepted {
		// The runner logged the drop without knowing whose it was.
		n.logger.With("user_id", userID).Error("The password-changed notice was not sent: it was dropped")
	}
}

func (n *accountEmailNotifier) sendPasswordChanged(
	ctx context.Context, userID string, kind AccountChangeKind, changedAt time.Time,
) error {
	channel, err := n.policy.PasswordResetChannel(ctx)
	if err != nil {
		return fmt.Errorf("read whether the instance has email: %w", err)
	}
	if channel != PasswordResetByEmail {
		return nil
	}
	user, err := n.users.GetByID(ctx, userID)
	if errors.Is(err, repositories.ErrUserNotFound) {
		// Deleted since; nobody to tell.
		return nil
	}
	if err != nil {
		return fmt.Errorf("load the account to notify: %w", err)
	}
	err = n.email.SendPasswordChanged(ctx, PasswordChangedNotice{
		To: user.Email, Name: user.Name, Kind: kind, ChangedAt: changedAt, CanResetByEmail: true,
	})
	if err != nil {
		return fmt.Errorf("send the password-changed notice: %w", err)
	}
	return nil
}
