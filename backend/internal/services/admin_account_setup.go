package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Instance-admin management of how an account signs in (#1436, epic #1426):
// the setup link a new account's owner gets, and a setup link issued later.
//
// # Fewer bearer links
//
// A copied link is a bearer credential that can leak from wherever it is
// pasted. So with an instance email provider stored, a new account's setup
// link is EMAILED and no copied link is made; a link to share exists only when
// an administrator asks for one (IssueSetupLink with AdminLinkByCopy). With no
// email provider, the copied link is the only way, and creation returns it once.
//
// # Root instance admins
//
// An address in auth.instance_admins holds instance-admin power by address
// alone (#1467). A copied setup link for such an account would hand that power
// to whoever holds the link, so none is ever issued: the account gets an
// emailed link (which proves the mailbox) or uses the setup page or the
// recovery CLI.

// AdminLinkDelivery is how an administrator wants a link delivered.
type AdminLinkDelivery string

const (
	// AdminLinkByEmail sends the link to the account's address.
	AdminLinkByEmail AdminLinkDelivery = "email"
	// AdminLinkByCopy returns the link to the administrator, once.
	AdminLinkByCopy AdminLinkDelivery = "link"
	// adminLinkNone records that no link was made.
	adminLinkNone AdminLinkDelivery = "none"
)

// Why a new account got no setup link (AdminUserSetup.Reason).
const (
	AdminSetupReasonLocalSignInOff = "password_login_disabled"
	AdminSetupReasonRootAdmin      = "root_admin_link_refused"
	AdminSetupReasonEmailFailed    = "email_failed"
	AdminSetupReasonSetupFailed    = "setup_failed"
)

var (
	// ErrAdminOwnAccount refuses an action an administrator takes on their own
	// account through the admin API; they use the account page instead.
	ErrAdminOwnAccount = errors.New("this action cannot be taken on your own account here")
	// ErrAdminAccountHasSignInMethod refuses a setup link for an account that
	// can already sign in.
	ErrAdminAccountHasSignInMethod = errors.New("the account can already sign in")
	// ErrAdminAccountSuspended refuses a setup link for a suspended account:
	// completing it would be refused, so the email would be a dead end.
	ErrAdminAccountSuspended = errors.New("the account is suspended; reactivate it first")
	// ErrAdminRootAdminLinkRefused refuses a link to share for a root instance
	// admin's address.
	ErrAdminRootAdminLinkRefused = errors.New(
		"no shareable link is issued for a root instance admin's address; " +
			"send it by email, or use the setup page or the recovery command")
	// ErrAdminLocalSignInOff refuses a link while password sign-in is off.
	ErrAdminLocalSignInOff = errors.New("password sign-in is switched off on this instance")
	// ErrAdminUserNotFound is returned for an unknown user.
	ErrAdminUserNotFound = errors.New("no such user")
)

// AdminUserSetup is how a new account's owner will get in.
type AdminUserSetup struct {
	// Delivery is email, link or none.
	Delivery AdminLinkDelivery
	// LinkURL is the copied link, only for AdminLinkByCopy. Show it once.
	LinkURL string
	// ExpiresAt is when the link stops working; zero for none.
	ExpiresAt time.Time
	// Reason says why Delivery is none.
	Reason string
}

// AdminSetupLink is a setup link issued on request.
type AdminSetupLink struct {
	// Email is the account's address, for the audit record.
	Email     string
	Delivery  AdminLinkDelivery
	LinkURL   string
	ExpiresAt time.Time
}

// AdminAccountServiceInterface manages how accounts sign in, for instance
// admins.
type AdminAccountServiceInterface interface {
	// SetUpNewUser gives a just-created account its way in and reports it. It
	// never fails the creation: a link that could not be made or sent is
	// reported with Delivery none and a Reason. actorID is the administrator.
	SetUpNewUser(ctx context.Context, actorID, userID string) AdminUserSetup
	// IssueSetupLink issues a setup link for an account that cannot sign in
	// yet, by email or to copy. It returns ErrAdminOwnAccount,
	// ErrAdminLocalSignInOff, ErrAdminUserNotFound,
	// ErrAdminAccountHasSignInMethod, ErrAccountEmailUnavailable or
	// ErrAdminRootAdminLinkRefused.
	IssueSetupLink(ctx context.Context, actorID, userID string, delivery AdminLinkDelivery) (*AdminSetupLink, error)
}

// AdminAccountDeps are the collaborators of AdminAccountService.
type AdminAccountDeps struct {
	Admin  repositories.AdminRepository
	Users  repositories.UserRepository
	Policy AuthPolicyResolver
	Tokens AccountTokenServiceInterface
	Mailer AccountLinkMailerInterface
	// RootAdmins says whether an address is a root instance admin's.
	RootAdmins RootAdminChecker
	Logger     *slog.Logger
}

// RootAdminChecker is the one question this service asks of the instance
// admin resolver. InstanceAdminResolver satisfies it.
type RootAdminChecker interface {
	IsRootAdmin(email string) bool
}

// AdminAccountService implements AdminAccountServiceInterface.
type AdminAccountService struct {
	admin      repositories.AdminRepository
	users      repositories.UserRepository
	policy     AuthPolicyResolver
	tokens     AccountTokenServiceInterface
	mailer     AccountLinkMailerInterface
	rootAdmins RootAdminChecker
	logger     *slog.Logger
}

var _ AdminAccountServiceInterface = (*AdminAccountService)(nil)

// NewAdminAccountService creates the service.
func NewAdminAccountService(deps AdminAccountDeps) *AdminAccountService {
	return &AdminAccountService{
		admin: deps.Admin, users: deps.Users, policy: deps.Policy, tokens: deps.Tokens,
		mailer: deps.Mailer, rootAdmins: deps.RootAdmins, logger: deps.Logger,
	}
}

// localSignInOn reports whether password sign-in is on.
func (s *AdminAccountService) localSignInOn(ctx context.Context) (bool, error) {
	policy, err := s.policy.Policy(ctx)
	if err != nil {
		return false, fmt.Errorf("read the authentication policy: %w", err)
	}
	return policy.PasswordLoginEnabled, nil
}

// emailWorks reports whether an instance email provider is stored.
func (s *AdminAccountService) emailWorks(ctx context.Context) (bool, error) {
	channel, err := s.policy.PasswordResetChannel(ctx)
	if err != nil {
		return false, fmt.Errorf("read whether the instance has email: %w", err)
	}
	return channel == PasswordResetByEmail, nil
}

// SetUpNewUser implements AdminAccountServiceInterface.
func (s *AdminAccountService) SetUpNewUser(ctx context.Context, actorID, userID string) AdminUserSetup {
	log := s.logger.With("user_id", userID, "actor_id", actorID)
	none := func(reason string) AdminUserSetup { return AdminUserSetup{Delivery: adminLinkNone, Reason: reason} }

	on, err := s.localSignInOn(ctx)
	if err != nil {
		log.With("error", err.Error()).Error("A new account got no setup link: the policy could not be read")
		return none(AdminSetupReasonSetupFailed)
	}
	if !on {
		return none(AdminSetupReasonLocalSignInOff)
	}
	works, err := s.emailWorks(ctx)
	if err != nil {
		log.With("error", err.Error()).Error("A new account got no setup link: email could not be checked")
		return none(AdminSetupReasonSetupFailed)
	}

	if works {
		expiresAt, sendErr := s.mailer.EmailSetupLink(ctx, userID, actorID)
		if sendErr != nil {
			log.With("error", sendErr.Error()).Error("A new account's setup email could not be sent")
			return none(AdminSetupReasonEmailFailed)
		}
		log.Info("Emailed a new account its setup link")
		return AdminUserSetup{Delivery: AdminLinkByEmail, ExpiresAt: expiresAt}
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		log.With("error", err.Error()).Error("A new account got no setup link: it could not be read")
		return none(AdminSetupReasonSetupFailed)
	}
	if s.rootAdmins != nil && s.rootAdmins.IsRootAdmin(user.Email) {
		return none(AdminSetupReasonRootAdmin)
	}
	link, err := s.tokens.IssueLink(ctx, IssueAccountLinkParams{
		UserID: userID, Purpose: models.AccountTokenPurposeSetup, CreatedBy: actorID,
	})
	if err != nil {
		log.With("error", err.Error()).Error("A new account's setup link could not be issued")
		return none(AdminSetupReasonSetupFailed)
	}
	return AdminUserSetup{Delivery: AdminLinkByCopy, LinkURL: link.URL, ExpiresAt: link.ExpiresAt}
}

// setupLinkTarget returns the account a setup link may be issued for, or why
// not: it is the administrator's own, password sign-in is off, it does not
// exist, or it can already sign in.
func (s *AdminAccountService) setupLinkTarget(
	ctx context.Context, actorID, userID string,
) (*models.AdminUserDetail, error) {
	if actorID == userID {
		return nil, ErrAdminOwnAccount
	}
	on, err := s.localSignInOn(ctx)
	if err != nil {
		return nil, err
	}
	if !on {
		return nil, ErrAdminLocalSignInOff
	}
	detail, err := s.admin.GetUserDetail(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("read the account: %w", err)
	}
	if detail == nil {
		return nil, ErrAdminUserNotFound
	}
	if !detail.SignIn.PendingSetup {
		return nil, ErrAdminAccountHasSignInMethod
	}
	if detail.Status == models.UserStatusSuspended {
		return nil, ErrAdminAccountSuspended
	}
	return detail, nil
}

// IssueSetupLink implements AdminAccountServiceInterface.
func (s *AdminAccountService) IssueSetupLink(
	ctx context.Context, actorID, userID string, delivery AdminLinkDelivery,
) (*AdminSetupLink, error) {
	if delivery != AdminLinkByEmail && delivery != AdminLinkByCopy {
		return nil, fmt.Errorf("%w: unknown delivery %q", ErrInvalidAccountTokenRequest, delivery)
	}
	detail, err := s.setupLinkTarget(ctx, actorID, userID)
	if err != nil {
		return nil, err
	}

	if delivery == AdminLinkByEmail {
		expiresAt, sendErr := s.mailer.EmailSetupLink(ctx, userID, actorID)
		if sendErr != nil {
			return nil, sendErr
		}
		return &AdminSetupLink{Email: detail.Email, Delivery: AdminLinkByEmail, ExpiresAt: expiresAt}, nil
	}

	if s.rootAdmins != nil && s.rootAdmins.IsRootAdmin(detail.Email) {
		return nil, ErrAdminRootAdminLinkRefused
	}
	link, err := s.tokens.IssueLink(ctx, IssueAccountLinkParams{
		UserID: userID, Purpose: models.AccountTokenPurposeSetup, CreatedBy: actorID,
	})
	if err != nil {
		return nil, fmt.Errorf("issue the setup link: %w", err)
	}
	return &AdminSetupLink{
		Email: detail.Email, Delivery: AdminLinkByCopy, LinkURL: link.URL, ExpiresAt: link.ExpiresAt,
	}, nil
}
