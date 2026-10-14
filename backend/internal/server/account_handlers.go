package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/vibexp/vibexp/internal/auth/password"
	sesslib "github.com/vibexp/vibexp/internal/auth/session"
	"github.com/vibexp/vibexp/internal/contextkeys"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	accountgen "github.com/vibexp/vibexp/internal/server/gen/account"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// Self-service account API (#1437): the signed-in user's own name, password
// and sign-in methods.
//
// Three rules live here rather than in services.AccountService, because they
// are about the HTTP session and not about the account:
//
//   - WHICH credential authenticated the request. Changing how an account signs
//     in is open to a browser session only. An API key or a bearer token that
//     could set a password would turn a leaked key into a permanent way in.
//   - HOW LONG AGO that session signed in (recentSignInWindow).
//   - RE-ISSUING the caller's cookie. Every change to a sign-in method ends the
//     user's other browser sessions and keeps this one (revokeOtherSessions).

// recentSignInWindow is how long after signing in a session may perform an
// operation that needs a "recent sign-in": setting a first password, removing
// the password, disconnecting an identity.
const recentSignInWindow = 10 * time.Minute

const (
	accountReauthenticationDetail = "Sign in again to continue: this needs a sign-in within the last 10 minutes"
	accountCookieSessionDetail    = "This can only be done from a signed-in browser session, not with an API key or token"
	accountLastMethodDetail       = "This is the account's only usable sign-in method, so it cannot be removed"
	accountLocalSignInOffDetail   = "Password sign-in is switched off on this instance"
	accountTooManyAttemptsDetail  = "Too many wrong passwords. Wait before trying again"
	accountInternalDetail         = "The account request could not be completed"
	// accountChangedButNotRevokedDetail is the one partial outcome: the password
	// change itself succeeded, and the optional API-access revocation did not.
	accountChangedButNotRevokedDetail = "Your password was changed, but API keys and connected tools " +
		"could not be revoked. Try revoking them again"
	// accountChangedNothingEndedDetail is the outcome when both follow-ups of a
	// password change failed.
	accountChangedNothingEndedDetail = "Your password was changed, but your other sessions could not be " +
		"signed out and API keys and connected tools could not be revoked. Sign out and in again, then revoke them"
	fieldCurrentCredential       = "current_password"
	fieldNewCredential           = "new_password"
	codeInvalidCurrentCredential = "invalid_current_password"
)

// accountStrictServer implements the generated accountgen.StrictServerInterface.
type accountStrictServer struct {
	s *Server
}

var _ accountgen.StrictServerInterface = (*accountStrictServer)(nil)

// accountHTTPKey carries the request and response writer into the strict
// handlers, which otherwise see neither: re-issuing the session cookie writes a
// header, and the activity log records the request.
type accountHTTPKey struct{}

type accountHTTP struct {
	w http.ResponseWriter
	r *http.Request
}

// setupAccountRoutes mounts the generated Account handlers on r, which is
// already behind flexibleAuthMiddleware.
func (s *Server) setupAccountRoutes(r chi.Router) {
	keepHTTP := func(next accountgen.StrictHandlerFunc, _ string) accountgen.StrictHandlerFunc {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
			return next(context.WithValue(ctx, accountHTTPKey{}, accountHTTP{w: w, r: r}), w, r, request)
		}
	}
	strict := accountgen.NewStrictHandlerWithOptions(
		&accountStrictServer{s: s},
		[]accountgen.StrictMiddlewareFunc{keepHTTP},
		accountgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  s.accountBindErrorHandler,
			ResponseErrorHandlerFunc: s.accountResponseErrorHandler,
		},
	)
	r.Group(func(gr chi.Router) {
		gr.Use(rejectUnknownAccountBodyFields)
		accountgen.HandlerWithOptions(strict, accountgen.ChiServerOptions{
			BaseRouter:       gr,
			ErrorHandlerFunc: s.accountBindErrorHandler,
		})
	})
}

// accountBodyFields lists, per method, the fields the two account request
// bodies accept. It is derived from the generated request types, so it follows
// the spec.
var accountBodyFields = map[string]map[string]struct{}{
	http.MethodPatch: allowedJSONFields(accountgen.AccountUpdateRequest{}),
	http.MethodPut:   allowedJSONFields(accountgen.AccountPasswordRequest{}),
}

// rejectUnknownAccountBodyFields enforces the `additionalProperties: false`
// the spec declares on both account request bodies, which oapi-codegen does
// not. It matters most for a password change: a misspelt `revoke_api_access`
// would otherwise be dropped and answered 204, and the person would believe
// their API keys were revoked. Only field NAMES are reported, never values.
//
// A body that is empty or not a JSON object is left for the generated decoder.
func rejectUnknownAccountBodyFields(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, guarded := accountBodyFields[r.Method]
		if !guarded || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}
		fields, ok := peekJSONObjectBody(w, r)
		if !ok {
			return
		}
		if unknown := unknownFields(fields, allowed); len(unknown) > 0 {
			apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(fmt.Sprintf(
				"Unknown field(s): %s. Only %s may be set.",
				strings.Join(unknown, ", "), strings.Join(sortedKeys(allowed), ", "))))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// accountBindErrorHandler turns request-decoding failures from the generated
// layer into an RFC 9457 400. The decoding error can quote the body, which may
// hold a password, so it is neither logged nor returned.
func (s *Server) accountBindErrorHandler(w http.ResponseWriter, r *http.Request, _ error) {
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("Invalid request"))
}

// accountResponseErrorHandler writes errors returned by the strict handlers.
// A *tooManyAttemptsError also sets Retry-After.
func (s *Server) accountResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var locked *tooManyAttemptsError
	if errors.As(err, &locked) {
		if locked.retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(locked.retryAfter))
		}
		apierrors.WriteJSONError(w, r, locked.APIError)
		return
	}
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}
	s.logger.With("error", err).Error("Account strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError(accountInternalDetail))
}

// tooManyAttemptsError is the 429 for a locked password, with the seconds until
// the lock ends for the Retry-After header.
type tooManyAttemptsError struct {
	*apierrors.APIError
	retryAfter int
}

func (e *tooManyAttemptsError) Unwrap() error { return e.APIError }

// accountCaller is who is calling an account operation and how.
type accountCaller struct {
	userID string
	// cookieSession is the caller's browser session; nil for an API key or a
	// bearer token.
	cookieSession *sesslib.Session
}

// recentSignIn reports whether the caller's browser session signed in within
// recentSignInWindow of now. AuthenticatedAt is the sign-in itself: a refresh
// or a sliding extension does not move it, so an old session that is still
// alive is not recent. A session with no sign-in time (minted before #1428) is
// not recent either.
func (c accountCaller) recentSignIn(now time.Time) bool {
	if c.cookieSession == nil || c.cookieSession.AuthenticatedAt.IsZero() {
		return false
	}
	age := now.Sub(c.cookieSession.AuthenticatedAt)
	return age >= 0 && age <= recentSignInWindow
}

// caller resolves the authenticated user of ctx. The routes are behind
// flexibleAuthMiddleware, so a missing user is a wiring fault, answered 401.
func (h *accountStrictServer) caller(ctx context.Context) (accountCaller, error) {
	userID, _ := ctx.Value(contextkeys.UserID).(string)
	if userID == "" {
		return accountCaller{}, apierrors.NewAuthRequiredError("Authentication required")
	}
	caller := accountCaller{userID: userID}
	if authType, _ := ctx.Value(contextkeys.AuthType).(string); authType == authTypeCookie {
		caller.cookieSession = authenticatedSession(ctx)
	}
	return caller, nil
}

// browserCaller is caller for the operations that change how the account signs
// in: it refuses anything but a browser session.
func (h *accountStrictServer) browserCaller(ctx context.Context) (accountCaller, error) {
	caller, err := h.caller(ctx)
	if err != nil {
		return accountCaller{}, err
	}
	if caller.cookieSession == nil {
		return accountCaller{}, apierrors.NewAPIError(apierrors.CodeCookieSessionRequired,
			apierrors.GetErrorTitle(apierrors.CodeCookieSessionRequired), accountCookieSessionDetail, http.StatusForbidden)
	}
	return caller, nil
}

// GetAccount handles GET /api/v1/account.
func (h *accountStrictServer) GetAccount(
	ctx context.Context, _ accountgen.GetAccountRequestObject,
) (accountgen.GetAccountResponseObject, error) {
	caller, err := h.caller(ctx)
	if err != nil {
		return nil, err
	}
	view, err := h.s.container.AccountService().Get(ctx, caller.userID)
	if err != nil {
		return nil, h.accountError("GetAccount", caller.userID, err)
	}
	account, err := toGenAccount(view, caller.recentSignIn(time.Now()))
	if err != nil {
		return nil, h.accountError("GetAccount", caller.userID, err)
	}
	return accountgen.GetAccount200JSONResponse(account), nil
}

// UpdateAccount handles PATCH /api/v1/account.
func (h *accountStrictServer) UpdateAccount(
	ctx context.Context, request accountgen.UpdateAccountRequestObject,
) (accountgen.UpdateAccountResponseObject, error) {
	caller, err := h.caller(ctx)
	if err != nil {
		return nil, err
	}
	if request.Body == nil {
		return nil, apierrors.NewBadRequestError("Invalid request")
	}
	view, err := h.s.container.AccountService().UpdateName(ctx, caller.userID, request.Body.Name)
	if err != nil {
		return nil, h.accountError("UpdateAccount", caller.userID, err)
	}
	account, err := toGenAccount(view, caller.recentSignIn(time.Now()))
	if err != nil {
		return nil, h.accountError("UpdateAccount", caller.userID, err)
	}
	return accountgen.UpdateAccount200JSONResponse(account), nil
}

// SetAccountPassword handles PUT /api/v1/account/password.
func (h *accountStrictServer) SetAccountPassword(
	ctx context.Context, request accountgen.SetAccountPasswordRequestObject,
) (accountgen.SetAccountPasswordResponseObject, error) {
	caller, err := h.browserCaller(ctx)
	if err != nil {
		return nil, err
	}
	change, err := accountPasswordChange(request.Body)
	if err != nil {
		return nil, h.accountError("SetAccountPassword", caller.userID, err)
	}
	change.RecentSignIn = caller.recentSignIn(time.Now())

	first, err := h.s.container.AccountService().SetPassword(ctx, caller.userID, change)
	if err != nil {
		return nil, h.accountError("SetAccountPassword", caller.userID, err)
	}

	activityType := activities.ActivityTypeAuthCredentialChanged
	if first {
		activityType = activities.ActivityTypeAuthCredentialSet
	}
	revokeAPIAccess := request.Body.RevokeApiAccess != nil && *request.Body.RevokeApiAccess
	h.recordActivity(ctx, caller.userID, activityType, map[string]interface{}{"revoke_api_access": revokeAPIAccess})

	if err = h.afterPasswordChange(ctx, caller.userID, revokeAPIAccess); err != nil {
		return nil, err
	}
	return accountgen.SetAccountPassword204Response{}, nil
}

// accountPasswordChange reads a password request body, enforcing the bounds
// the spec declares. Nothing else enforces them, and without them one request
// could have megabytes of "password" hashed.
func accountPasswordChange(body *accountgen.AccountPasswordRequest) (services.AccountPasswordChange, error) {
	if body == nil || body.NewPassword == "" {
		return services.AccountPasswordChange{}, newCredentialError("A new password is required", "required")
	}
	if len(body.NewPassword) > maxSignInPasswordBytes {
		return services.AccountPasswordChange{}, newCredentialError("The new password is too long", "too_long")
	}
	change := services.AccountPasswordChange{New: body.NewPassword}
	if body.CurrentPassword != nil {
		// Longer than any password this server accepts, so it is not the
		// current one; it is refused before it is hashed or counted.
		if len(*body.CurrentPassword) > maxSignInPasswordBytes {
			return services.AccountPasswordChange{}, services.ErrCurrentPasswordInvalid
		}
		change.Current = *body.CurrentPassword
	}
	return change, nil
}

// newCredentialError is a 400 on the new_password field.
func newCredentialError(message, code string) error {
	return apierrors.NewValidationError(message, []apierrors.ValidationError{
		apierrors.NewFieldValidationError(fieldNewCredential, message, code),
	})
}

// afterPasswordChange ends the user's other sessions and, when asked, revokes
// API access. The password IS changed by now. Both are attempted whatever the
// other does: the person is relying on each of them if the old password
// leaked, so one failing must not skip the other.
func (h *accountStrictServer) afterPasswordChange(ctx context.Context, userID string, revokeAPIAccess bool) error {
	sessionsErr := h.endOtherSessions(ctx, "SetAccountPassword", userID)
	var revokeErr error
	if revokeAPIAccess {
		if revokeErr = h.s.container.AccountService().RevokeAPIAccess(ctx, userID); revokeErr != nil {
			h.s.logger.With("handler", "SetAccountPassword", "user_id", userID, "error", revokeErr.Error()).
				Error("Password changed, but revoking API access failed")
		}
	}
	switch {
	case sessionsErr != nil && revokeErr != nil:
		return apierrors.NewInternalError(accountChangedNothingEndedDetail)
	case sessionsErr != nil:
		return sessionsErr
	case revokeErr != nil:
		return apierrors.NewInternalError(accountChangedButNotRevokedDetail)
	}
	return nil
}

// DeleteAccountPassword handles DELETE /api/v1/account/password.
func (h *accountStrictServer) DeleteAccountPassword(
	ctx context.Context, _ accountgen.DeleteAccountPasswordRequestObject,
) (accountgen.DeleteAccountPasswordResponseObject, error) {
	caller, err := h.browserCaller(ctx)
	if err != nil {
		return nil, err
	}
	err = h.s.container.AccountService().RemovePassword(ctx, caller.userID, caller.recentSignIn(time.Now()))
	if err != nil {
		return nil, h.accountError("DeleteAccountPassword", caller.userID, err)
	}
	h.recordActivity(ctx, caller.userID, activities.ActivityTypeAuthCredentialRemoved, nil)
	if err = h.endOtherSessions(ctx, "DeleteAccountPassword", caller.userID); err != nil {
		return nil, err
	}
	return accountgen.DeleteAccountPassword204Response{}, nil
}

// DeleteAccountIdentity handles DELETE /api/v1/account/identities/{id}.
func (h *accountStrictServer) DeleteAccountIdentity(
	ctx context.Context, request accountgen.DeleteAccountIdentityRequestObject,
) (accountgen.DeleteAccountIdentityResponseObject, error) {
	caller, err := h.browserCaller(ctx)
	if err != nil {
		return nil, err
	}
	provider, err := h.s.container.AccountService().DisconnectIdentity(
		ctx, caller.userID, request.Id.String(), caller.recentSignIn(time.Now()))
	if err != nil {
		return nil, h.accountError("DeleteAccountIdentity", caller.userID, err)
	}
	h.recordActivity(ctx, caller.userID, activities.ActivityTypeAuthIdentityDisconnected,
		map[string]interface{}{"provider": provider})
	if err = h.endOtherSessions(ctx, "DeleteAccountIdentity", caller.userID); err != nil {
		return nil, err
	}
	return accountgen.DeleteAccountIdentity204Response{}, nil
}

// endOtherSessions ends every other browser session of the user and re-issues
// the caller's cookie under the new session epoch, so the caller stays signed
// in. The change it follows has already been made, so a failure here is
// reported as a failure: the person must not believe a leaked session is gone
// when it is not.
func (h *accountStrictServer) endOtherSessions(ctx context.Context, handler, userID string) error {
	exchange, ok := ctx.Value(accountHTTPKey{}).(accountHTTP)
	if !ok {
		return fmt.Errorf("account handler %s has no HTTP exchange to re-issue the session on", handler)
	}
	if err := h.s.revokeOtherSessions(exchange.w, exchange.r, userID); err != nil {
		h.s.logger.With("handler", handler, "user_id", userID, "error", err.Error()).
			Error("The account was changed, but ending the other sessions failed")
		return apierrors.NewInternalError(
			"The change was saved, but your other sessions could not be signed out. Sign out and in again")
	}
	return nil
}

// recordActivity writes an auth activity entry for the account's owner.
func (h *accountStrictServer) recordActivity(
	ctx context.Context, userID, activityType string, metadata map[string]interface{},
) {
	exchange, ok := ctx.Value(accountHTTPKey{}).(accountHTTP)
	if !ok {
		return
	}
	NewActivityRecorder(h.s.container.ActivityService()).
		RecordAuthActivity(ctx, userID, activityType, nil, metadata, exchange.r)
}

// accountRefusals are the account service's refusals that always get the same
// answer: a status, a stable code and a fixed detail.
var accountRefusals = []struct {
	err    error
	code   string
	detail string
	status int
}{
	{services.ErrReauthenticationRequired, apierrors.CodeReauthenticationRequired,
		accountReauthenticationDetail, http.StatusForbidden},
	{services.ErrLastSignInMethod, apierrors.CodeLastSignInMethod, accountLastMethodDetail, http.StatusConflict},
	{services.ErrPasswordDisabled, apierrors.CodeDisabledByAdmin,
		services.ErrPasswordDisabled.Error(), http.StatusConflict},
	{services.ErrPasswordLoginDisabled, apierrors.CodeLocalSignInDisabled,
		accountLocalSignInOffDetail, http.StatusConflict},
}

// accountError maps a service error onto the response. Policy outcomes are the
// documented 4xx with their stable codes; anything else is logged and answered
// 500 without detail.
func (h *accountStrictServer) accountError(handler, userID string, err error) error {
	var policyErr *password.PolicyError
	var locked *services.PasswordLockedError
	var requestErr *apierrors.APIError
	for _, refusal := range accountRefusals {
		if errors.Is(err, refusal.err) {
			return apierrors.NewAPIError(refusal.code, apierrors.GetErrorTitle(refusal.code), refusal.detail, refusal.status)
		}
	}
	switch {
	case errors.As(err, &requestErr):
		// Already an answer for the caller (a request validation error).
		return requestErr
	case errors.Is(err, services.ErrAccountNameInvalid):
		return apierrors.NewValidationError("The name is not valid", []apierrors.ValidationError{
			apierrors.NewFieldValidationError("name", "Enter a name of 1 to 255 characters", "invalid_length"),
		})
	case errors.Is(err, services.ErrCurrentPasswordInvalid):
		return apierrors.NewValidationError("The current password is not correct", []apierrors.ValidationError{
			apierrors.NewFieldValidationError(fieldCurrentCredential,
				"The current password is not correct", codeInvalidCurrentCredential),
		})
	case errors.As(err, &locked):
		return h.tooManyAttempts(locked)
	case errors.Is(err, services.ErrPasswordAttemptsExceeded):
		return h.tooManyAttempts(&services.PasswordLockedError{})
	case errors.As(err, &policyErr):
		return newPasswordPolicyProblem(policyErr)
	case errors.Is(err, services.ErrAccountPasswordNotSet):
		return apierrors.NewResourceNotFoundError("password", "")
	case errors.Is(err, services.ErrAccountIdentityNotFound):
		return apierrors.NewResourceNotFoundError("identity", "")
	}
	h.s.logger.With("handler", handler, "user_id", userID, "error", err.Error()).
		Error("Account request failed")
	return apierrors.NewInternalError(accountInternalDetail)
}

// tooManyAttempts builds the 429 for a locked password. The lock's end is the
// owner's to know: they are signed in, and it is their own account.
func (h *accountStrictServer) tooManyAttempts(locked *services.PasswordLockedError) error {
	out := &tooManyAttemptsError{APIError: apierrors.NewAPIError(apierrors.CodeTooManyAttempts,
		apierrors.GetErrorTitle(apierrors.CodeTooManyAttempts), accountTooManyAttemptsDetail,
		http.StatusTooManyRequests)}
	if remaining := time.Until(locked.Until); !locked.Until.IsZero() && remaining > 0 {
		out.retryAfter = int(math.Ceil(remaining.Seconds()))
	}
	return out
}

// newPasswordPolicyProblem is passwordPolicyProblem for this API, whose request
// field is `new_password`: the same codes and messages, on that field.
func newPasswordPolicyProblem(err *password.PolicyError) *apierrors.APIError {
	problem := passwordPolicyProblem(err)
	for i := range problem.ValidationErrors {
		problem.ValidationErrors[i].Field = fieldNewCredential
	}
	return problem
}

// toGenAccount maps an account onto the Account schema. Identities is always a
// non-nil slice: the schema requires the array, and null would not conform.
func toGenAccount(view *services.AccountView, recentSignIn bool) (accountgen.Account, error) {
	id, err := uuid.Parse(view.User.ID)
	if err != nil {
		return accountgen.Account{}, fmt.Errorf("account id is not a UUID: %w", err)
	}
	identities := make([]accountgen.AccountIdentity, 0, len(view.Identities))
	for i := range view.Identities {
		identity := view.Identities[i]
		identityID, parseErr := uuid.Parse(identity.ID)
		if parseErr != nil {
			return accountgen.Account{}, fmt.Errorf("identity id is not a UUID: %w", parseErr)
		}
		identities = append(identities, accountgen.AccountIdentity{
			Id:          identityID,
			Provider:    identity.Provider,
			DisplayName: identity.DisplayName,
			Available:   identity.Available,
			ConnectedAt: identity.ConnectedAt,
			LastUsedAt:  identity.LastUsedAt,
		})
	}
	return accountgen.Account{
		Id:    id,
		Email: openapi_types.Email(view.User.Email),
		Name:  view.User.Name,
		Password: accountgen.AccountPassword{
			Set:          view.Password.Set,
			ChangedAt:    view.Password.ChangedAt,
			LoginEnabled: view.Password.LoginEnabled,
			Disabled:     view.Password.Disabled,
		},
		Identities:   identities,
		RecentSignIn: recentSignIn,
	}, nil
}
