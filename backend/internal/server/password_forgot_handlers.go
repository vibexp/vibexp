package server

import (
	"context"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	passwordauthgen "github.com/vibexp/vibexp/internal/server/gen/passwordauth"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// Forgot password (#1433, epic #1426): the strict-server adapter over
// services.AccountLinkMailer.RequestPasswordReset.
//
// The answer must not depend on the address in any way a caller can observe,
// content or timing. So the handler does nothing with the address except bound
// it and hand it on: the lookup, the decision and the send all happen in the
// background, after the same 202 has been decided for everyone. It is mounted
// in the password sign-in group, so it shares the per-IP limit, the 404 while
// password login is off and the application/json requirement.

// RequestPasswordReset handles POST /api/v1/auth/password/forgot.
func (h *passwordAuthStrictServer) RequestPasswordReset(
	ctx context.Context, request passwordauthgen.RequestPasswordResetRequestObject,
) (passwordauthgen.RequestPasswordResetResponseObject, error) {
	if request.Body == nil || request.Body.Email == "" {
		return nil, apierrors.NewBadRequestError("email is required")
	}
	if len(request.Body.Email) > maxSignInEmailBytes {
		return nil, apierrors.NewBadRequestError("email is too long")
	}

	h.s.container.AccountLinkMailer().RequestPasswordReset(string(request.Body.Email))
	// Recorded here, not in the mailer: the mailer works in the background,
	// without the request context. The attempted address is the actor; whether
	// it matched an account is never recorded, which keeps the log from
	// answering what the 202 deliberately does not.
	h.s.recordAudit(ctx, audit.Event{
		Operation: models.AuditOpAuthLocalCredentialResetRequested,
		Actor:     &audit.Actor{Email: string(request.Body.Email), Type: models.AuditActorAnonymous},
	})

	return passwordauthgen.RequestPasswordReset202JSONResponse{
		Status: passwordauthgen.AccountResetEmailStatusAccepted,
	}, nil
}
