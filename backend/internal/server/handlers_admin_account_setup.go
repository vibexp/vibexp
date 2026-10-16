package server

import (
	"context"
	"errors"
	"net/http"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	admingen "github.com/vibexp/vibexp/internal/server/gen/admin"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// Setup links on request (#1436): POST /api/v1/admin/users/{id}/setup-link.

// IssueAdminUserSetupLink emails or returns a setup link for an account that
// cannot sign in yet.
func (a *adminStrictServer) IssueAdminUserSetupLink(
	ctx context.Context, request admingen.IssueAdminUserSetupLinkRequestObject,
) (admingen.IssueAdminUserSetupLinkResponseObject, error) {
	if request.Body == nil || !request.Body.Delivery.Valid() {
		return nil, apierrors.NewBadRequestError("delivery must be email or link")
	}
	userID := request.Id.String()
	link, err := a.s.container.AdminAccountService().IssueSetupLink(
		ctx, a.actingAdminID(ctx), userID, services.AdminLinkDelivery(request.Body.Delivery))
	if err != nil {
		return nil, a.mapAdminAccountError(err, "IssueAdminUserSetupLink")
	}

	a.recordUserMutationActivity(ctx, activities.ActivityTypeAdminUserSetupLinkIssued, userID, link.Email,
		map[string]interface{}{"delivery": string(link.Delivery)})
	a.s.auditLinkIssued(ctx, auditLink{
		userID: userID, email: link.Email, purpose: auditLinkPurposeSetup, delivery: string(link.Delivery), issued: true,
	})

	out := admingen.IssueAdminUserSetupLink200JSONResponse{
		Delivery:  admingen.AdminUserSetupLinkDelivery(link.Delivery),
		ExpiresAt: link.ExpiresAt.UTC(),
	}
	if link.LinkURL != "" {
		url := link.LinkURL
		out.LinkUrl = &url
	}
	return out, nil
}

// adminAccountRefusals maps each refusal of the admin account service onto its
// stable code. All are 409s.
var adminAccountRefusals = []struct {
	err  error
	code string
}{
	{services.ErrAdminOwnAccount, apierrors.CodeOwnAccount},
	{services.ErrAdminLocalSignInOff, apierrors.CodeLocalSignInDisabled},
	{services.ErrAdminAccountHasSignInMethod, apierrors.CodeHasSignInMethod},
	{services.ErrAdminAccountSuspended, apierrors.CodeAccountSuspended},
	{services.ErrAccountEmailUnavailable, apierrors.CodeEmailUnavailable},
	{services.ErrAccountEmailUnverifiedShared, apierrors.CodeEmailUnverifiedShared},
	{services.ErrLastSignInMethod, apierrors.CodeLastSignInMethod},
	{services.ErrAdminRootAdminLinkRefused, apierrors.CodeRootAdminLinkRefused},
}

// mapAdminAccountError turns the admin account service's errors into their
// HTTP shapes: a refusal is a 409 whose code says which, an unknown user is a
// 404, anything else is a logged 500 that carries no detail.
func (a *adminStrictServer) mapAdminAccountError(err error, handler string) error {
	for _, refusal := range adminAccountRefusals {
		if errors.Is(err, refusal.err) {
			return apierrors.NewAPIError(refusal.code, apierrors.GetErrorTitle(refusal.code),
				refusal.err.Error(), http.StatusConflict)
		}
	}
	if errors.Is(err, services.ErrAdminUserNotFound) {
		return apierrors.NewResourceNotFoundError("user", "User not found")
	}
	if errors.Is(err, services.ErrAccountPasswordNotSet) {
		return apierrors.NewResourceNotFoundError("password", "The user has no password")
	}
	a.s.logger.With("service", serverLogServiceName, "handler", handler, "error", err.Error()).
		Error("Admin account action failed")
	return apierrors.NewInternalError(adminMsgInternalError)
}

// Reset links and password removal (#1436):
// POST /api/v1/admin/users/{id}/reset-link and DELETE /api/v1/admin/users/{id}/password.

// IssueAdminUserResetLink emails or returns a password reset link, optionally
// stopping the current password, sessions and links first.
func (a *adminStrictServer) IssueAdminUserResetLink(
	ctx context.Context, request admingen.IssueAdminUserResetLinkRequestObject,
) (admingen.IssueAdminUserResetLinkResponseObject, error) {
	if request.Body == nil || !request.Body.Delivery.Valid() {
		return nil, apierrors.NewBadRequestError("delivery must be email or link")
	}
	invalidate := request.Body.InvalidateNow != nil && *request.Body.InvalidateNow
	userID := request.Id.String()
	link, err := a.s.container.AdminAccountService().IssueResetLink(ctx, a.actingAdminID(ctx), userID,
		services.AdminResetLinkRequest{
			Delivery: services.AdminLinkDelivery(request.Body.Delivery), InvalidateNow: invalidate,
		})
	if err != nil {
		if link != nil && link.Invalidated {
			// The password, sessions and links were stopped before the link
			// failed: that change is audited even though nothing was issued.
			a.recordUserMutationActivity(ctx, activities.ActivityTypeAdminUserResetLinkIssued, userID, link.Email,
				map[string]interface{}{"delivery": string(link.Delivery), "invalidated": true, "issued": false})
			a.s.auditLinkIssued(ctx, auditLink{
				userID: userID, email: link.Email, purpose: auditLinkPurposeReset, delivery: string(link.Delivery),
				invalidated: true,
			})
		}
		return nil, a.mapAdminAccountError(err, "IssueAdminUserResetLink")
	}

	a.recordUserMutationActivity(ctx, activities.ActivityTypeAdminUserResetLinkIssued, userID, link.Email,
		map[string]interface{}{"delivery": string(link.Delivery), "invalidated": link.Invalidated, "issued": true})
	a.s.auditLinkIssued(ctx, auditLink{
		userID: userID, email: link.Email, purpose: auditLinkPurposeReset, delivery: string(link.Delivery),
		invalidated: link.Invalidated, issued: true,
	})

	out := admingen.IssueAdminUserResetLink200JSONResponse{
		Delivery:    admingen.AdminUserResetLinkDelivery(link.Delivery),
		ExpiresAt:   link.ExpiresAt.UTC(),
		Invalidated: link.Invalidated,
	}
	if link.LinkURL != "" {
		url := link.LinkURL
		out.LinkUrl = &url
	}
	return out, nil
}

// DeleteAdminUserPassword removes a user's password.
func (a *adminStrictServer) DeleteAdminUserPassword(
	ctx context.Context, request admingen.DeleteAdminUserPasswordRequestObject,
) (admingen.DeleteAdminUserPasswordResponseObject, error) {
	allowLast := request.Params.ConfirmLastMethod != nil && *request.Params.ConfirmLastMethod
	userID := request.Id.String()
	email, err := a.s.container.AdminAccountService().RemovePassword(ctx, a.actingAdminID(ctx), userID, allowLast)
	if err != nil {
		return nil, a.mapAdminAccountError(err, "DeleteAdminUserPassword")
	}
	a.recordUserMutationActivity(ctx, activities.ActivityTypeAdminUserLocalSignInRemoved, userID, email,
		map[string]interface{}{"confirmed_last_method": allowLast})
	a.s.auditUserEvent(ctx, models.AuditOpAuthLocalCredentialRemoved, auditTarget{
		userID: userID, email: email,
		metadata: map[string]any{auditMetaBy: auditByAdmin, auditMetaTarget: userID, "confirmed_last_method": allowLast},
	})
	return admingen.DeleteAdminUserPassword204Response{}, nil
}
