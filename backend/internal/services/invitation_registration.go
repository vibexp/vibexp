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

// Joining through an invitation link without an account (#1435, epic #1426).
//
// Password accounts are invite-only: nobody can sign up for one. The invitation
// link is therefore the only way a person with no account, on an instance with
// no identity provider they can use, gets in. Two things are public here, for
// whoever holds a link: a preview of what the invitation is, and registration
// with a password, which creates the account, joins the team and signs in.
//
// The link is a bearer link, and a team admin can copy it ("Copy invite link"),
// so completing it does NOT prove the mailbox: the account's email is left
// unverified. A later sign-in through a provider that verifies the address
// claims the account and removes this password (the account-claim rule).

var (
	// ErrInvitationUnusable is the ONE answer for a link that cannot be used:
	// unknown, expired, revoked, accepted or rejected. A caller must not be
	// able to tell which.
	ErrInvitationUnusable = errors.New("the invitation link is not valid")
	// ErrInvitationAccountExists is returned by Register when an account
	// already has the invited email: the person signs in and accepts instead.
	ErrInvitationAccountExists = errors.New("an account already exists for the invited email")
	// ErrInvitationEmailNotAllowed is returned by Register when an active
	// access allowlist does not admit the invited email.
	ErrInvitationEmailNotAllowed = errors.New("the invited email is not permitted by the access allowlist")
)

// InvitationNameMaxLength bounds the display name given at registration.
const InvitationNameMaxLength = 255

// InvitationPreview is what a link holder may see before signing in. It holds
// no identifier and no inviter email.
type InvitationPreview struct {
	TeamName string
	// InviterName is the inviter's display name; empty when the inviter no
	// longer exists or has none.
	InviterName string
	Email       string
	Role        models.TeamMemberRole
	ExpiresAt   time.Time
	// AccountExists: an account already has the invited email.
	AccountExists bool
	// PasswordRegistration: this link can create an account with a password
	// right now (password login is on and no account has the email).
	PasswordRegistration bool
}

// InvitationRegistration is the outcome of Register.
type InvitationRegistration struct {
	User   *models.User
	TeamID string
	// InvitationAccepted is false when the account was created but joining the
	// team failed. The invitation is then still pending and the person, now
	// signed in, accepts it in the app.
	InvitationAccepted bool
}

// InvitationRegistrationServiceInterface is the public side of invitations.
type InvitationRegistrationServiceInterface interface {
	// Preview describes the invitation behind token. It returns
	// ErrInvitationUnusable for every unusable link.
	Preview(ctx context.Context, token string) (*InvitationPreview, error)
	// Register creates an account for the invited email with the given
	// password, joins the team and returns the user to sign in. name may be
	// empty. Refusals, each changing nothing: ErrInvitationUnusable,
	// ErrInvitationAccountExists, ErrInvitationEmailNotAllowed, a
	// *password.PolicyError. Whether password login is on is the caller's gate.
	Register(ctx context.Context, token, plaintext, name string) (*InvitationRegistration, error)
}

// InvitationPasswordSetter stores a password; PasswordAuthServiceInterface
// satisfies it.
type InvitationPasswordSetter interface {
	SetPassword(ctx context.Context, userID, plaintext string) error
}

// InvitationRegistrationDeps are the collaborators of the service.
type InvitationRegistrationDeps struct {
	Invitations repositories.TeamInvitationRepository
	Teams       repositories.TeamRepository
	Users       repositories.UserRepository
	Passwords   InvitationPasswordSetter
	Policy      AuthPolicyResolver
	Allowlist   AccessAllowlistResolver
	// Events publishes user.created, from which the account's private
	// workspace and default project are provisioned.
	Events events.EventPublisher
	Logger *slog.Logger
}

// InvitationRegistrationService implements InvitationRegistrationServiceInterface.
type InvitationRegistrationService struct {
	deps InvitationRegistrationDeps
	now  func() time.Time
}

var _ InvitationRegistrationServiceInterface = (*InvitationRegistrationService)(nil)

// NewInvitationRegistrationService creates the service.
func NewInvitationRegistrationService(deps InvitationRegistrationDeps) *InvitationRegistrationService {
	return &InvitationRegistrationService{deps: deps, now: time.Now}
}

// usable loads the invitation behind token when it is pending and unexpired.
// Every other state, and an unknown token, is ErrInvitationUnusable.
func (s *InvitationRegistrationService) usable(ctx context.Context, token string) (*models.TeamInvitation, error) {
	if token == "" {
		return nil, ErrInvitationUnusable
	}
	invitation, err := s.deps.Invitations.GetByToken(ctx, token)
	if errors.Is(err, repositories.ErrTeamInvitationNotFound) {
		return nil, ErrInvitationUnusable
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load the invitation: %w", err)
	}
	if invitation.Status != models.InvitationStatusPending || !s.now().Before(invitation.ExpiresAt) {
		return nil, ErrInvitationUnusable
	}
	return invitation, nil
}

// accountExists reports whether an account has email.
func (s *InvitationRegistrationService) accountExists(ctx context.Context, email string) (bool, error) {
	_, err := s.deps.Users.GetByEmail(ctx, email)
	if errors.Is(err, repositories.ErrUserNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to look up the invited account: %w", err)
	}
	return true, nil
}

// Preview implements InvitationRegistrationServiceInterface.
func (s *InvitationRegistrationService) Preview(ctx context.Context, token string) (*InvitationPreview, error) {
	invitation, err := s.usable(ctx, token)
	if err != nil {
		return nil, err
	}
	team, err := s.deps.Teams.GetByID(ctx, invitation.TeamID)
	if err != nil {
		return nil, fmt.Errorf("failed to load the invitation's team: %w", err)
	}
	exists, err := s.accountExists(ctx, invitation.InviteeEmail)
	if err != nil {
		return nil, err
	}
	policy, err := s.deps.Policy.Policy(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read the authentication policy: %w", err)
	}

	preview := &InvitationPreview{
		TeamName:             team.Name,
		Email:                invitation.InviteeEmail,
		Role:                 invitation.Role,
		ExpiresAt:            invitation.ExpiresAt,
		AccountExists:        exists,
		PasswordRegistration: policy.PasswordLoginEnabled && !exists,
	}
	// Best effort, as on the authenticated invitation page: a deleted inviter
	// does not make the invitation unreadable.
	if inviter, inviterErr := s.deps.Users.GetByID(ctx, invitation.InviterID); inviterErr == nil {
		preview.InviterName = inviter.Name
	}
	return preview, nil
}

// Register implements InvitationRegistrationServiceInterface.
//
// Every check runs before anything is written. After the account row exists, a
// failure to provision it or to store its password removes the row again (as
// admin user creation does), so the link works once more. Only a failure to
// join the team is left standing: the account is complete by then, and the
// invitation is still pending for an ordinary accept.
func (s *InvitationRegistrationService) Register(
	ctx context.Context, token, plaintext, name string,
) (*InvitationRegistration, error) {
	invitation, err := s.usable(ctx, token)
	if err != nil {
		return nil, err
	}
	email := models.NormalizeEmail(invitation.InviteeEmail)
	if err = s.admit(ctx, email, plaintext); err != nil {
		return nil, err
	}

	user, err := s.createAccount(ctx, email, displayName(name, email), plaintext)
	if err != nil {
		return nil, err
	}
	log := s.deps.Logger.With("user_id", user.ID, "invitation_id", invitation.ID, "team_id", invitation.TeamID)

	result := &InvitationRegistration{User: user, TeamID: invitation.TeamID}
	now := s.now()
	member := &models.TeamMember{
		TeamID: invitation.TeamID, UserID: user.ID, Role: invitation.Role, CreatedAt: now, UpdatedAt: now,
	}
	if err = s.deps.Invitations.Accept(ctx, invitation.ID, member); err != nil {
		// The account stands. Someone revoking the invitation at this moment,
		// or a storage failure, must not undo a registration that succeeded.
		log.With("error", err.Error()).
			Error("Account registered through an invitation, but joining the team failed; the invitation is still pending")
		return result, nil
	}
	result.InvitationAccepted = true
	log.Info("Account registered through an invitation and joined the team")
	return result, nil
}

// admit runs the refusals that depend on the invited email and the password:
// the allowlist, an existing account, the password policy.
func (s *InvitationRegistrationService) admit(ctx context.Context, email, plaintext string) error {
	exists, err := s.accountExists(ctx, email)
	if err != nil {
		return err
	}
	if exists {
		return ErrInvitationAccountExists
	}
	// The allowlist is the hard boundary. Inviting an address it rejects is
	// refused up front (#1434), but the allowlist may have changed since.
	allowed, _, err := s.deps.Allowlist.IsEmailAllowed(ctx, email)
	if err != nil {
		return fmt.Errorf("failed to evaluate the access allowlist: %w", err)
	}
	if !allowed {
		return ErrInvitationEmailNotAllowed
	}
	// Before the account is created: a refused password must leave nothing.
	return password.Validate(plaintext, email)
}

// createAccount creates the user, provisions it through user.created and
// stores the password, removing the row again when either step after the
// insert fails. A concurrent registration that won the email is
// ErrInvitationAccountExists.
func (s *InvitationRegistrationService) createAccount(
	ctx context.Context, email, name, plaintext string,
) (*models.User, error) {
	now := s.now()
	// EmailVerifiedAt stays nil: holding the link does not prove the mailbox.
	user := &models.User{Email: email, Name: name, CreatedAt: now, UpdatedAt: now}
	if err := s.deps.Users.Create(ctx, user); err != nil {
		if errors.Is(err, repositories.ErrUserEmailTaken) {
			return nil, ErrInvitationAccountExists
		}
		return nil, fmt.Errorf("failed to create the account: %w", err)
	}

	event := events.NewUserCreatedEvent(user.ID, user.Email, user.Name, user.CreatedAt)
	if err := s.deps.Events.Publish(ctx, event); err != nil {
		return nil, s.undo(ctx, user, fmt.Errorf("failed to provision the account: %w", err))
	}
	if err := s.deps.Passwords.SetPassword(ctx, user.ID, plaintext); err != nil {
		var policyErr *password.PolicyError
		if errors.As(err, &policyErr) {
			// Unreachable after admit, but never report a policy refusal as a fault.
			return nil, s.undo(ctx, user, err)
		}
		return nil, s.undo(ctx, user, fmt.Errorf("failed to store the password: %w", err))
	}
	return user, nil
}

// undo removes the half-made account and returns cause, joined with the
// removal's own failure when there is one. Without the removal the invited
// email would be taken by an account nobody can sign in to, and the link dead.
func (s *InvitationRegistrationService) undo(ctx context.Context, user *models.User, cause error) error {
	// The request may be gone by now; the cleanup must still run.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.deps.Users.DeleteByID(cleanupCtx, user.ID); err != nil {
		s.deps.Logger.With("user_id", user.ID, "error", err.Error()).
			Error("Failed to remove a half-registered account; its email is taken until it is deleted")
		return fmt.Errorf("%w (and the account could not be removed again: %w)", cause, err)
	}
	return cause
}

// displayName is the trimmed name given at registration, or the part of the
// email before the @ when none was given.
func displayName(name, email string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	local, _, _ := strings.Cut(email, "@")
	return local
}
