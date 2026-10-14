package server

import (
	"context"
	"errors"
	"net/http"

	apierrors "github.com/vibexp/vibexp/internal/errors"
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
	a.s.logger.With("service", serverLogServiceName, "handler", handler, "error", err.Error()).
		Error("Admin account action failed")
	return apierrors.NewInternalError(adminMsgInternalError)
}
