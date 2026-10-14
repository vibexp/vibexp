package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// AdminResetLinkRequest is an administrator's request for a password reset
// link (#1436).
type AdminResetLinkRequest struct {
	Delivery AdminLinkDelivery
	// InvalidateNow stops the current password, every session and every live
	// link of the user before the new link is issued.
	InvalidateNow bool
}

// AdminResetLink is a reset link issued on request.
type AdminResetLink struct {
	AdminSetupLink
	// Invalidated reports whether the password, sessions and links were
	// stopped first.
	Invalidated bool
}

// IssueResetLink implements AdminAccountServiceInterface.
//
// Every refusal is decided before anything changes: with InvalidateNow an
// administrator who is refused must not find the password stopped anyway.
func (s *AdminAccountService) IssueResetLink(
	ctx context.Context, actorID, userID string, req AdminResetLinkRequest,
) (*AdminResetLink, error) {
	if req.Delivery != AdminLinkByEmail && req.Delivery != AdminLinkByCopy {
		return nil, fmt.Errorf("%w: unknown delivery %q", ErrInvalidAccountTokenRequest, req.Delivery)
	}
	detail, err := s.resetLinkTarget(ctx, actorID, userID)
	if err != nil {
		return nil, err
	}
	if err = s.checkResetDelivery(ctx, detail, req.Delivery); err != nil {
		return nil, err
	}

	out := &AdminResetLink{AdminSetupLink: AdminSetupLink{Email: detail.Email, Delivery: req.Delivery}}
	if req.InvalidateNow {
		if err = s.invalidate(ctx, actorID, userID); err != nil {
			return nil, err
		}
		out.Invalidated = true
	}

	if req.Delivery == AdminLinkByEmail {
		out.ExpiresAt, err = s.mailer.EmailResetLink(ctx, userID, actorID)
		if err != nil {
			return failedAfterInvalidate(out), fmt.Errorf("email the reset link: %w", err)
		}
		return out, nil
	}
	link, err := s.tokens.IssueLink(ctx, IssueAccountLinkParams{
		UserID: userID, Purpose: models.AccountTokenPurposeReset, CreatedBy: actorID,
	})
	if err != nil {
		return failedAfterInvalidate(out), fmt.Errorf("issue the reset link: %w", err)
	}
	out.LinkURL, out.ExpiresAt = link.URL, link.ExpiresAt
	return out, nil
}

// failedAfterInvalidate is what IssueResetLink returns with its error when no
// link could be made: nil, unless the password was already stopped, in which
// case the caller gets the result (Invalidated, no link) so the change that
// did happen can be audited.
func failedAfterInvalidate(out *AdminResetLink) *AdminResetLink {
	if !out.Invalidated {
		return nil
	}
	out.ExpiresAt = time.Time{}
	return out
}

// resetLinkTarget returns the account a reset link may be issued for, or why
// not: it is the administrator's own, password sign-in is off, it does not
// exist, or it is suspended. Unlike a setup link, an account that can already
// sign in is the usual case.
func (s *AdminAccountService) resetLinkTarget(
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
	if detail.Status == models.UserStatusSuspended {
		return nil, ErrAdminAccountSuspended
	}
	return detail, nil
}

// checkResetDelivery refuses a delivery that cannot reach this account: an
// email the instance cannot send or must not send, or a link to share for a
// root admin's address.
func (s *AdminAccountService) checkResetDelivery(
	ctx context.Context, detail *models.AdminUserDetail, delivery AdminLinkDelivery,
) error {
	if delivery == AdminLinkByEmail {
		return s.mailer.CheckResetEmail(ctx, detail.ID)
	}
	if s.rootAdmins != nil && s.rootAdmins.IsRootAdmin(detail.Email) {
		return ErrAdminRootAdminLinkRefused
	}
	return nil
}

// invalidate stops the user's password, sessions and live links, and tells the
// owner when there was a password to stop.
func (s *AdminAccountService) invalidate(ctx context.Context, actorID, userID string) error {
	disabled, err := s.store.DisablePassword(ctx, userID)
	if err != nil {
		return fmt.Errorf("stop the current password: %w", err)
	}
	s.logger.With("user_id", userID, "actor_id", actorID, "had_password", disabled.HadPassword,
		"links_deleted", disabled.Links).Info("An administrator stopped the account's password and sessions")
	if disabled.HadPassword {
		s.notifier.PasswordChanged(ctx, userID, AccountChangeDisabledByAdmin)
	}
	return nil
}

// RemovePassword implements AdminAccountServiceInterface.
func (s *AdminAccountService) RemovePassword(
	ctx context.Context, actorID, userID string, allowLast bool,
) (string, error) {
	if actorID == userID {
		return "", ErrAdminOwnAccount
	}
	detail, err := s.admin.GetUserDetail(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("read the account: %w", err)
	}
	if detail == nil {
		return "", ErrAdminUserNotFound
	}
	err = s.accounts.AdminRemovePassword(ctx, userID, allowLast)
	if errors.Is(err, ErrAccountPasswordNotSet) || errors.Is(err, ErrLastSignInMethod) {
		return "", err
	}
	if err != nil {
		return "", fmt.Errorf("remove the password: %w", err)
	}
	return detail.Email, nil
}
