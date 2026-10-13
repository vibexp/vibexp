package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/vibexp/vibexp/internal/auth/password"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	passwordauthgen "github.com/vibexp/vibexp/internal/server/gen/passwordauth"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// One-time account links (#1432, epic #1426): the strict-server adapter over
// services.AccountTokenService. The three operations are mounted by
// setupPasswordAuthRoutes in the same group as password sign-in, so they share
// its per-IP rate limit, its 404 while password login is off, and its
// application/json requirement (completing a link sets a session cookie, which
// makes it a sign-in a cross-site form must not be able to trigger).
//
// Secret safety: a request body holds the link's token and a new password, and
// is never logged. The token is passed to the service and nowhere else. Every
// link that cannot be used is answered with one identical 401, whatever the
// reason.

const (
	// accountLinkInvalidDetail is the one message for every link that cannot be
	// used. It must not vary with the reason.
	accountLinkInvalidDetail = "This link is invalid or has expired"
	// accountLinkSpentDetail answers a completion that failed after the link
	// was used up. By then the new password is stored and the earlier sessions
	// are ended; what failed is the requested revocation of API access, or
	// issuing the session. Trying again with the same link cannot work, and the
	// person must be told the password did change.
	accountLinkSpentDetail = "Your password was changed, but the request could not be finished, " +
		"and this link can no longer be used. Sign in with your new password."
	// accountLinkUnavailableDetail answers a request that could not be decided
	// because the allowlist or the database was unreadable.
	accountLinkUnavailableDetail = "This is temporarily unavailable. Try again shortly."
	// maxAccountLinkTokenBytes and maxAccountNameBytes are the maxLength the
	// spec gives the token and the name. The name's limit is in characters, so
	// its byte bound allows four bytes for each.
	maxAccountLinkTokenBytes = 256
	maxAccountNameBytes      = 4 * 255

	// accountLinkFieldsRequiredDetail answers a completion with no token or no
	// new secret in its body.
	accountLinkFieldsRequiredDetail = "token and password are required"

	accountLinkPreviewHandler = "PreviewAccountLink"

	accountLinkViaSetup = "setup"
	accountLinkViaReset = "reset"
)

// PreviewAccountLink handles POST /api/v1/auth/password/token/preview.
func (h *passwordAuthStrictServer) PreviewAccountLink(
	ctx context.Context, request passwordauthgen.PreviewAccountLinkRequestObject,
) (passwordauthgen.PreviewAccountLinkResponseObject, error) {
	if request.Body == nil || request.Body.Token == "" {
		return nil, apierrors.NewBadRequestError("token is required")
	}
	if len(request.Body.Token) > maxAccountLinkTokenBytes {
		return nil, apierrors.NewBadRequestError("token is too long")
	}

	preview, err := h.s.container.AccountTokenService().Preview(ctx, request.Body.Token)
	if err != nil {
		return nil, h.accountLinkFailure(ctx, accountLinkPreviewHandler, err)
	}
	return passwordauthgen.PreviewAccountLink200JSONResponse{
		Purpose:   passwordauthgen.AccountLinkPreviewPurpose(preview.Purpose),
		Email:     openapi_types.Email(preview.Email),
		Name:      preview.Name,
		ExpiresAt: preview.ExpiresAt,
	}, nil
}

// CompleteAccountSetup handles POST /api/v1/auth/password/setup.
func (h *passwordAuthStrictServer) CompleteAccountSetup(
	ctx context.Context, request passwordauthgen.CompleteAccountSetupRequestObject,
) (passwordauthgen.CompleteAccountSetupResponseObject, error) {
	const handler = "CompleteAccountSetup"
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(accountLinkFieldsRequiredDetail)
	}
	name := ""
	if request.Body.Name != nil {
		name = *request.Body.Name
	}
	if len(name) > maxAccountNameBytes {
		return nil, apierrors.NewBadRequestError("name is too long")
	}
	if err := h.checkAccountLinkCompletion(handler, request.Body.Token, request.Body.Password); err != nil {
		return nil, err
	}

	user, err := h.s.container.AccountTokenService().
		CompleteSetup(ctx, request.Body.Token, request.Body.Password, name)
	if err != nil {
		return nil, h.accountLinkFailure(ctx, handler, err)
	}
	setCookie, err := h.signInWithPassword(ctx, handler, user, accountLinkViaSetup)
	if err != nil {
		return nil, err
	}
	return passwordauthgen.CompleteAccountSetup200JSONResponse{
		Body:    toGenPasswordAuthUser(user),
		Headers: passwordauthgen.CompleteAccountSetup200ResponseHeaders{SetCookie: &setCookie},
	}, nil
}

// CompleteAccountReset handles POST /api/v1/auth/password/reset.
func (h *passwordAuthStrictServer) CompleteAccountReset(
	ctx context.Context, request passwordauthgen.CompleteAccountResetRequestObject,
) (passwordauthgen.CompleteAccountResetResponseObject, error) {
	const handler = "CompleteAccountReset"
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError(accountLinkFieldsRequiredDetail)
	}
	if err := h.checkAccountLinkCompletion(handler, request.Body.Token, request.Body.Password); err != nil {
		return nil, err
	}
	revoke := request.Body.RevokeApiAccess != nil && *request.Body.RevokeApiAccess

	user, err := h.s.container.AccountTokenService().
		CompleteReset(ctx, request.Body.Token, request.Body.Password, revoke)
	if err != nil {
		return nil, h.accountLinkFailure(ctx, handler, err)
	}
	setCookie, err := h.signInWithPassword(ctx, handler, user, accountLinkViaReset)
	if err != nil {
		return nil, err
	}
	return passwordauthgen.CompleteAccountReset200JSONResponse{
		Body:    toGenPasswordAuthUser(user),
		Headers: passwordauthgen.CompleteAccountReset200ResponseHeaders{SetCookie: &setCookie},
	}, nil
}

// checkAccountLinkCompletion enforces the bounds the spec declares on a
// completion's token and password, and that a session can be issued at all
// BEFORE the link is consumed: a link must not be spent by a request that could
// never have signed its holder in.
func (h *passwordAuthStrictServer) checkAccountLinkCompletion(handler, token, plaintext string) error {
	if token == "" || plaintext == "" {
		return apierrors.NewBadRequestError(accountLinkFieldsRequiredDetail)
	}
	if len(token) > maxAccountLinkTokenBytes || len(plaintext) > maxSignInPasswordBytes {
		return apierrors.NewBadRequestError("token or password is too long")
	}
	if h.s.sessionManager == nil {
		// 503, not 500: on these operations a 500 tells the client the link is
		// used up, and here it is untouched.
		h.s.logger.With("handler", handler).Error("Session manager is not configured")
		return apierrors.NewServiceUnavailableError(accountLinkUnavailableDetail)
	}
	return nil
}

// accountLinkFailure maps an AccountTokenService error onto its response. A
// refused completion counts as a failed sign-in in the metrics; a refused
// preview is not a sign-in attempt and does not.
func (h *passwordAuthStrictServer) accountLinkFailure(ctx context.Context, handler string, err error) error {
	var policy *password.PolicyError
	completing := handler != accountLinkPreviewHandler
	switch {
	case errors.Is(err, services.ErrAccountTokenInvalid):
		// Nothing identifies the caller or the link here, on purpose: the
		// service logs the redacted token where one was found.
		h.s.logger.With("handler", handler).Info("Account link refused")
		if completing {
			h.recordLoginFailed(ctx, "account_link_invalid")
		}
		return apierrors.NewAuthInvalidError(accountLinkInvalidDetail)
	case errors.Is(err, services.ErrAccountLinkSpent):
		// Checked before the context cases: a spent link is a 500 whatever
		// made the completion fail, because nothing can be retried with it.
		// The service has logged the failure with the redacted token.
		return apierrors.NewInternalError(accountLinkSpentDetail)
	case errors.Is(err, services.ErrAccessRestricted):
		if completing {
			h.recordLoginFailed(ctx, "access_restricted")
		}
		return apierrors.NewAccessRestrictedError("Your account is not permitted to sign in")
	case errors.As(err, &policy):
		return passwordPolicyProblem(policy)
	case errors.Is(err, services.ErrInvalidAccountName):
		return apierrors.NewBadRequestError("name is too long")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		h.s.logger.With("handler", handler, "error", err.Error()).
			Info("Account link request abandoned before it finished")
		return apierrors.NewServiceUnavailableError(accountLinkUnavailableDetail)
	default:
		// The error names what failed, never the token or the password.
		h.s.logger.With("handler", handler, "error", fmt.Sprintf("%+v", err)).
			Error("Account link request could not be completed")
		return apierrors.NewServiceUnavailableError(accountLinkUnavailableDetail)
	}
}

// signInWithPassword issues the session of a person who has just proved
// themselves with a password, or with a one-time link that set one, and
// records the sign-in. It returns the Set-Cookie header value. via is empty
// for a sign-in with the password itself, and "setup" or "reset" for a link.
//
// user.SessionEpoch must be the account's current epoch: completing a link
// bumps it, and a cookie carrying the earlier value would be refused on its
// first use.
func (h *passwordAuthStrictServer) signInWithPassword(
	ctx context.Context, handler string, user *models.User, via string,
) (string, error) {
	now := time.Now()
	sess := &sesslib.Session{
		// Like dev login, the access token is a non-validating marker: nothing
		// ever presents it to an identity provider. With no refresh token the
		// session is a sliding one (30 days from its last use).
		AccessToken:     "password:" + user.ID,
		UserID:          user.ID,
		AuthMethod:      sesslib.AuthMethodPassword,
		AuthenticatedAt: now,
		Epoch:           user.SessionEpoch,
	}
	sess.StartSliding(now)
	cookie, err := h.s.sessionManager.Cookie(sess)
	if err != nil {
		h.s.logger.With("handler", handler, "user_id", user.ID, "error", err.Error()).
			Error("Failed to create the session of a password sign-in")
		if via != "" {
			// The link is used up and the password is set; signing in with
			// it works, the link does not.
			return "", apierrors.NewInternalError(accountLinkSpentDetail)
		}
		return "", apierrors.NewInternalError(signInFailedDetail)
	}

	log := h.s.logger.With("handler", handler, "user_id", user.ID)
	metadata := map[string]interface{}{
		"provider": services.LocalSignInProvider,
		"email":    user.Email,
	}
	if via != "" {
		log = log.With("via", via)
		metadata["via"] = via
	}
	log.Info("Password sign-in successful")
	if h.s.metrics != nil {
		h.s.metrics.RecordUserLoginSuccessful(ctx)
	}
	h.recordAuthActivity(ctx, user.ID, activities.ActivityTypeAuthLogin, metadata)
	// A root admin signing in with a password, or through a link that set
	// one, ends setup mode as a provider sign-in does: on a password-only
	// instance nothing else ever would (#1438).
	h.s.consumeSetupOnRootLogin(ctx, user)
	return cookie.String(), nil
}
