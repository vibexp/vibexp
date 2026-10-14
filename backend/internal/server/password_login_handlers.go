package server

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/vibexp/vibexp/internal/auth/password"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	passwordauthgen "github.com/vibexp/vibexp/internal/server/gen/passwordauth"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// Email and password sign-in (#1431, epic #1426): the strict-server adapter
// over services.PasswordAuthService.
//
// The operation is public and takes a guessable secret, so it is mounted under
// the strict per-IP auth rate limit, in addition to the per-account throttle
// the service applies.
//
// Secret safety: the request body is never logged, and the handler passes the
// password to the service and nowhere else. Every account-dependent refusal is
// answered with one identical 401, whatever the service reports as the reason;
// the reason goes to logs, metrics and the account's own activity record only.

const (
	// signInInvalidDetail is the one message for every account-dependent
	// refusal. It must not vary with the reason.
	signInInvalidDetail = "Invalid email or password"
	signInFailedDetail  = "Sign-in could not be completed"
	// signInUnavailableDetail answers a sign-in that could not be
	// decided because the policy, the allowlist or the database was unreadable.
	signInUnavailableDetail = "Sign-in is temporarily unavailable"
	// maxSignInEmailBytes and maxSignInPasswordBytes are the maxLength the spec
	// gives PasswordLoginRequest's fields. The password bound is far above the
	// policy's maximum on purpose: the policy applies when a password is set,
	// and a sign-in must be refused, not rejected as malformed, for being wrong.
	maxSignInEmailBytes    = 320
	maxSignInPasswordBytes = 1024
	signInHandlerName      = "PasswordLogin"
)

// passwordAuthStrictServer implements passwordauthgen.StrictServerInterface.
type passwordAuthStrictServer struct {
	s *Server
}

var _ passwordauthgen.StrictServerInterface = (*passwordAuthStrictServer)(nil)

// passwordAuthRequestKey is the context key of the HTTP request a generated
// strict operation is serving. A strict handler receives only a context and the
// decoded body, but the sign-in activity records the client address and user
// agent, which live on the request.
type passwordAuthRequestKey struct{}

// setupPasswordAuthRoutes mounts the Password Authentication operations at
// their spec paths (under /api/v1/auth) behind the strict per-IP auth limit.
func (s *Server) setupPasswordAuthRoutes() {
	keepRequest := func(next passwordauthgen.StrictHandlerFunc, _ string) passwordauthgen.StrictHandlerFunc {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
			return next(context.WithValue(ctx, passwordAuthRequestKey{}, r), w, r, request)
		}
	}
	strict := passwordauthgen.NewStrictHandlerWithOptions(
		&passwordAuthStrictServer{s: s},
		[]passwordauthgen.StrictMiddlewareFunc{keepRequest},
		passwordauthgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  s.passwordAuthBindErrorHandler,
			ResponseErrorHandlerFunc: s.passwordAuthResponseErrorHandler,
		},
	)
	s.router.Group(func(r chi.Router) {
		s.signInRateLimit(r)
		r.Use(s.requirePasswordLoginEnabled)
		r.Use(requireJSONBody)
		passwordauthgen.HandlerWithOptions(strict, passwordauthgen.ChiServerOptions{
			BaseRouter:       r,
			ErrorHandlerFunc: s.passwordAuthBindErrorHandler,
		})
	})
}

// signInRateLimit applies the strict per-IP auth limit to the group, answering
// an RFC 9457 429 as the spec documents. The shared rateLimitByIP uses the
// limiter's default plain-text response, which is fine for routes that do not
// document a 429 and wrong for these. Disabled under the same conditions as
// rateLimitByIP.
func (s *Server) signInRateLimit(r chi.Router) {
	limit := s.config.RateLimit.AuthPerMinute
	if limit < 1 || s.config.IsLocalDevelopment() {
		return
	}
	r.Use(httprate.LimitBy(limit, time.Minute, rateLimitKey,
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			apierrors.WriteJSONError(w, r, apierrors.NewAPIError(
				apierrors.CodeRateLimitExceeded, apierrors.GetErrorTitle(apierrors.CodeRateLimitExceeded),
				"Too many sign-in requests. Wait a minute and try again.", http.StatusTooManyRequests))
		}),
	))
}

// requireJSONBody refuses a request whose body is not declared as JSON.
//
// It is the defence against login CSRF. The generated handler decodes the body
// as JSON whatever its Content-Type, and a browser lets any site submit an HTML
// form with enctype text/plain whose bytes happen to be valid JSON. Such a post
// could sign the visitor's browser in to an account the attacker controls,
// where everything the visitor then saves is readable by the attacker. A
// cross-site page cannot send Content-Type: application/json without a CORS
// preflight this server refuses for foreign origins, so requiring it closes
// the hole for every operation in this group.
func requireJSONBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("Content-Type must be application/json"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requirePasswordLoginEnabled answers 404 for every Password Authentication
// operation while the instance has password login switched off, before the
// request body is read or decoded: on such an instance these endpoints do not
// exist, exactly as dev login does not exist outside development. An unreadable
// policy is a 503, never a 404, so a database fault cannot pass for the switch
// being off. The service checks the policy again; this is the early answer.
func (s *Server) requirePasswordLoginEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		policy, err := s.container.AuthPolicyResolver().Policy(r.Context())
		if err != nil {
			s.logger.With("handler", signInHandlerName, "error", err.Error()).
				Error("Password sign-in refused: the authentication policy could not be read")
			apierrors.WriteJSONError(w, r, apierrors.NewServiceUnavailableError(signInUnavailableDetail))
			return
		}
		if !policy.PasswordLoginEnabled {
			apierrors.WriteJSONError(w, r, apierrors.NewResourceNotFoundError(endpointResource, endpointNotFoundMsg))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// PasswordLogin handles POST /api/v1/auth/password/login.
func (h *passwordAuthStrictServer) PasswordLogin(
	ctx context.Context, request passwordauthgen.PasswordLoginRequestObject,
) (passwordauthgen.PasswordLoginResponseObject, error) {
	if request.Body == nil || request.Body.Email == "" || request.Body.Password == "" {
		return nil, apierrors.NewBadRequestError("email and password are required")
	}
	// The bounds the spec declares. Nothing else enforces them, and without
	// them one request could hold megabytes of "password" in memory while it
	// waits for a hashing slot and then have it hashed.
	if len(request.Body.Email) > maxSignInEmailBytes || len(request.Body.Password) > maxSignInPasswordBytes {
		return nil, apierrors.NewBadRequestError("email or password is too long")
	}
	if h.s.sessionManager == nil {
		h.s.logger.With("handler", signInHandlerName).Error("Session manager is not configured")
		return nil, apierrors.NewInternalError(signInFailedDetail)
	}

	user, err := h.s.container.PasswordAuthService().
		Login(ctx, string(request.Body.Email), request.Body.Password)
	if err != nil {
		return nil, h.loginFailure(ctx, err)
	}

	setCookie, err := h.signInWithPassword(ctx, signInHandlerName, user, "")
	if err != nil {
		return nil, err
	}
	return passwordauthgen.PasswordLogin200JSONResponse{
		Body:    toGenPasswordAuthUser(user),
		Headers: passwordauthgen.PasswordLogin200ResponseHeaders{SetCookie: &setCookie},
	}, nil
}

// loginFailure maps a Login error onto its response and records it. Every
// *services.PasswordLoginError becomes the same 401; only what is logged and
// recorded differs.
func (h *passwordAuthStrictServer) loginFailure(ctx context.Context, err error) error {
	var invalid *services.PasswordLoginError
	var suspended *services.AccountSuspendedError
	switch {
	case errors.Is(err, services.ErrPasswordLoginDisabled):
		// The same 404 dev login gives outside development: the endpoint does
		// not exist on an instance that has not switched password login on.
		return apierrors.NewResourceNotFoundError(endpointResource, endpointNotFoundMsg)
	case errors.Is(err, services.ErrAccessRestricted):
		h.recordLoginFailed(ctx, "access_restricted")
		return apierrors.NewAccessRestrictedError("Your account is not permitted to sign in")
	case errors.As(err, &invalid):
		h.s.logger.With("handler", signInHandlerName, "reason", invalid.Reason, "user_id", invalid.UserID).
			Info("Password sign-in refused")
		h.recordLoginFailed(ctx, invalid.Reason)
		if invalid.UserID != "" {
			h.recordAuthActivity(ctx, invalid.UserID, activities.ActivityTypeAuthFailure, map[string]interface{}{
				"provider": services.LocalSignInProvider,
				"reason":   invalid.Reason,
			})
		}
		return apierrors.NewAuthInvalidError(signInInvalidDetail)
	case errors.As(err, &suspended):
		// The caller proved they hold the password, so this refusal is recorded
		// against the account.
		h.recordLoginFailed(ctx, services.SignInRefusalSuspended)
		h.recordAuthActivity(ctx, suspended.UserID, activities.ActivityTypeAuthFailure, map[string]interface{}{
			"provider": services.LocalSignInProvider,
			"reason":   services.SignInRefusalSuspended,
		})
		return apierrors.NewAuthInvalidError(suspendedAuthDetail)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// The client went away, or the request timed out, while it waited its
		// turn to be checked. Expected under load, and not a fault of the
		// policy or the database, so it is not logged as one.
		h.s.logger.With("handler", signInHandlerName, "error", err.Error()).
			Info("Password sign-in abandoned before it was checked")
		return apierrors.NewServiceUnavailableError(signInUnavailableDetail)
	default:
		// The policy, the allowlist or the database could not be read. The
		// error names what failed, never the credentials.
		h.s.logger.With("handler", signInHandlerName, "error", fmt.Sprintf("%+v", err)).
			Error("Password sign-in could not be decided")
		return apierrors.NewServiceUnavailableError(signInUnavailableDetail)
	}
}

func (h *passwordAuthStrictServer) recordLoginFailed(ctx context.Context, reason string) {
	if h.s.metrics != nil {
		h.s.metrics.RecordUserLoginFailed(ctx, reason)
	}
}

// recordAuthActivity records a sign-in activity with the client address and
// user agent of the request being served.
func (h *passwordAuthStrictServer) recordAuthActivity(
	ctx context.Context, userID, activityType string, metadata map[string]interface{},
) {
	r, ok := ctx.Value(passwordAuthRequestKey{}).(*http.Request)
	if !ok {
		return
	}
	NewActivityRecorder(h.s.container.ActivityService()).
		RecordAuthActivity(ctx, userID, activityType, nil, metadata, r)
}

// toGenPasswordAuthUser maps the signed-in user onto the User schema, the same
// body dev login returns.
func toGenPasswordAuthUser(user *models.User) passwordauthgen.User {
	return passwordauthgen.User{
		Id:                    user.ID,
		Email:                 openapi_types.Email(user.Email),
		Name:                  user.Name,
		AvatarUrl:             user.AvatarURL,
		DefaultTeamId:         user.DefaultTeamID,
		GoogleId:              user.GoogleID,
		IdpProvider:           user.IDPProvider,
		IdpSubject:            user.IDPSubject,
		OnboardingCompleted:   user.OnboardingCompleted,
		OnboardingCompletedAt: user.OnboardingCompletedAt,
		CreatedAt:             user.CreatedAt,
		UpdatedAt:             user.UpdatedAt,
		Version:               user.Version,
	}
}

// passwordPolicyProblem maps a password that failed the policy onto the 400
// every flow that sets a password answers with: one field error on `password`
// per rule broken, each with a stable code a client can map to its own wording
// and a sentence it can show as it is. The password itself is never included.
func passwordPolicyProblem(err *password.PolicyError) *apierrors.APIError {
	fieldErrors := make([]apierrors.ValidationError, 0, len(err.Violations))
	for _, violation := range err.Violations {
		fieldErrors = append(fieldErrors,
			apierrors.NewFieldValidationError("password", violation.Message(), string(violation)))
	}
	return apierrors.NewValidationError("The password does not meet the password policy", fieldErrors)
}

// passwordAuthBindErrorHandler translates request-decoding failures from the
// generated layer into an RFC 9457 400 (the generated default writes a
// plain-text 400). The decoding error can quote the body, so it is not logged
// or returned.
func (s *Server) passwordAuthBindErrorHandler(w http.ResponseWriter, r *http.Request, _ error) {
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("Invalid request body"))
}

// passwordAuthResponseErrorHandler writes errors returned by the strict handler
// implementations. *apierrors.APIError carries the intended RFC 9457 error;
// anything else is defensive and maps to a generic 500.
func (s *Server) passwordAuthResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}
	s.logger.With("error", err).Error("Password authentication strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Internal server error"))
}
