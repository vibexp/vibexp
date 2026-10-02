package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	"github.com/vibexp/vibexp/internal/contextkeys"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	setupgen "github.com/vibexp/vibexp/internal/server/gen/setup"
	"github.com/vibexp/vibexp/internal/services"
)

// First-run authentication setup (#1236, epic #1230): the public setup status,
// the setup-token exchange, and the guard that lets a setup session reach the
// authentication settings.

const (
	// authTypeSetup is the contextkeys.AuthType of a request admitted on a setup
	// session. It names no user.
	authTypeSetup = "setup"

	// setupMsgRejected is the ONE message every rejected setup token gets,
	// so a caller cannot tell an unknown token from an expired or consumed one.
	setupMsgRejected      = "Invalid or expired setup token"
	setupMsgStatusFailed  = "Failed to determine the setup status"
	setupMsgSessionFailed = "Failed to start the setup session"
)

// setupStrictServer implements setupgen.StrictServerInterface. Like the other
// spec-first domains, handlers return *apierrors.APIError, rendered as RFC 9457
// application/problem+json by setupResponseErrorHandler.
type setupStrictServer struct {
	s *Server
}

var _ setupgen.StrictServerInterface = (*setupStrictServer)(nil)

// setupSetupRoutes mounts the generated Setup strict-server handler. Both
// operations are unauthenticated: the status is public, and the session
// exchange is authenticated by the setup token in its body.
func (s *Server) setupSetupRoutes() {
	strict := setupgen.NewStrictHandlerWithOptions(
		&setupStrictServer{s: s},
		nil,
		setupgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  s.setupBindErrorHandler,
			ResponseErrorHandlerFunc: s.setupResponseErrorHandler,
		},
	)
	s.router.Group(func(r chi.Router) {
		rateLimitByIP(r, s.config.RateLimit.APIPerMinute, s.config.IsLocalDevelopment())
		r.Use(s.setupTokenExchangeRateLimit())
		setupgen.HandlerWithOptions(strict, setupgen.ChiServerOptions{
			BaseRouter:       r,
			ErrorHandlerFunc: s.setupBindErrorHandler,
		})
	})
}

// setupTokenExchangeRateLimit applies the strict per-IP auth limit to the
// token exchange (the POST) only: it is the one operation here that takes a
// guessable secret. The status is fetched by the web app on every load, so it
// stays on the group's general API limit. Disabled under the same conditions as
// rateLimitByIP.
func (s *Server) setupTokenExchangeRateLimit() func(http.Handler) http.Handler {
	limit := s.config.RateLimit.AuthPerMinute
	if limit < 1 || s.config.IsLocalDevelopment() {
		return func(next http.Handler) http.Handler { return next }
	}
	limiter := httprate.LimitBy(limit, time.Minute, rateLimitKey)
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				limited.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetSetupStatus handles GET /api/v1/setup/status. The body is the one boolean
// and nothing else: the endpoint is public.
func (h *setupStrictServer) GetSetupStatus(
	ctx context.Context, _ setupgen.GetSetupStatusRequestObject,
) (setupgen.GetSetupStatusResponseObject, error) {
	active, err := h.s.container.SetupModeService().IsActive(ctx)
	if err != nil {
		h.s.logger.With("handler", "GetSetupStatus", "error", err.Error()).Error(setupMsgStatusFailed)
		return nil, apierrors.NewInternalError(setupMsgStatusFailed)
	}
	return setupgen.GetSetupStatus200JSONResponse{SetupRequired: active}, nil
}

// CreateSetupSession handles POST /api/v1/setup/session: it exchanges the setup
// token for the setup cookie.
func (h *setupStrictServer) CreateSetupSession(
	ctx context.Context, request setupgen.CreateSetupSessionRequestObject,
) (setupgen.CreateSetupSessionResponseObject, error) {
	if request.Body == nil || request.Body.Token == "" {
		return nil, apierrors.NewBadRequestError("token is required")
	}
	if h.s.sessionManager == nil {
		h.s.logger.With("handler", "CreateSetupSession").Error("Session manager is not configured")
		return nil, apierrors.NewInternalError(setupMsgSessionFailed)
	}

	sess, err := h.s.container.SetupModeService().ExchangeToken(ctx, request.Body.Token)
	switch {
	case errors.Is(err, services.ErrSetupNotActive):
		return nil, apierrors.NewResourceNotFoundError(endpointResource, endpointNotFoundMsg)
	case errors.Is(err, services.ErrSetupTokenInvalid):
		h.s.logger.With("handler", "CreateSetupSession").Warn("Rejected a setup token")
		return nil, apierrors.NewAuthInvalidError(setupMsgRejected)
	case err != nil:
		h.s.logger.With("handler", "CreateSetupSession", "error", err.Error()).Error(setupMsgSessionFailed)
		return nil, apierrors.NewInternalError(setupMsgSessionFailed)
	}

	cookie, err := h.s.sessionManager.SetupCookie(sess)
	if err != nil {
		h.s.logger.With("handler", "CreateSetupSession", "error", err.Error()).Error(setupMsgSessionFailed)
		return nil, apierrors.NewInternalError(setupMsgSessionFailed)
	}
	h.s.logger.With("handler", "CreateSetupSession").Warn("Setup session started",
		"expires_at", sess.ExpiresAt)

	setCookie := cookie.String()
	return setupgen.CreateSetupSession200JSONResponse{
		Body:    setupgen.SetupSessionResponse{ExpiresAt: sess.ExpiresAt},
		Headers: setupgen.CreateSetupSession200ResponseHeaders{SetCookie: &setCookie},
	}, nil
}

// setupBindErrorHandler translates request-decoding failures from the generated
// layer into an RFC 9457 400 (the generated default writes a plain-text 400).
func (s *Server) setupBindErrorHandler(w http.ResponseWriter, r *http.Request, _ error) {
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("Invalid request body"))
}

// setupResponseErrorHandler writes errors returned by the strict handler
// implementations. *apierrors.APIError carries the intended RFC 9457 error;
// anything else is defensive and maps to a generic 500.
func (s *Server) setupResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if errors.As(err, &apiErr) {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}
	s.logger.With("error", err).Error("Setup strict handler failed")
	apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Internal server error"))
}

// setupSessionOrInstanceAdmin guards the instance authentication settings: it
// admits an instance admin (exactly as optionalAuthMiddleware followed by
// instanceAdminMiddleware does) OR a request carrying a valid setup session,
// and 404s everything else.
//
// It is the ONLY place a setup session is honoured. The setup cookie is a
// different cookie from vx_session and names no user, so authMiddleware (401)
// and instanceAdminMiddleware (404) reject a setup-only request by
// construction rather than by a deny-list. Mount it on the authentication
// settings routes alone, and INSTEAD of optionalAuthMiddleware +
// instanceAdminMiddleware, not after them: it authenticates the request itself.
//
// An instance admin takes precedence, so an admin who still holds a setup
// cookie acts — and is audited — as themselves. A request admitted on the setup
// session is USERLESS by construction: its context is the one from before any
// authentication ran, so a signed-in non-admin who happens to hold the cookie
// is never the acting user of a setup-session write.
func (s *Server) setupSessionOrInstanceAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.optionalAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, authed *http.Request) {
			if s.isInstanceAdminRequest(authed) {
				next.ServeHTTP(w, authed)
				return
			}
			// r, not authed: the unauthenticated request this guard received.
			if ctx, ok := s.setupSessionContext(r); ok {
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			apierrors.WriteJSONError(w, r, apierrors.NewResourceNotFoundError(endpointResource, endpointNotFoundMsg))
		})).ServeHTTP(w, r)
	})
}

// setupSessionContext validates the request's setup cookie and, when it is a
// live setup session, returns the request's context marked as one.
func (s *Server) setupSessionContext(r *http.Request) (context.Context, bool) {
	if s.sessionManager == nil {
		return nil, false
	}
	sess, err := s.sessionManager.ReadSetup(r)
	if err != nil {
		return nil, false
	}
	if err := s.container.SetupModeService().ValidateSession(r.Context(), sess); err != nil {
		if !errors.Is(err, services.ErrSetupSessionInvalid) {
			s.logger.With("middleware", "setupSessionOrInstanceAdmin", "error", err).
				Error("Failed to validate the setup session; denying")
		}
		return nil, false
	}

	ctx := context.WithValue(r.Context(), contextkeys.AuthType, authTypeSetup)
	return context.WithValue(ctx, contextkeys.SetupSession, true), true
}

// isSetupSessionRequest reports whether the request was admitted on a setup
// session by setupSessionOrInstanceAdmin (and so has no acting user).
func isSetupSessionRequest(ctx context.Context) bool {
	admitted, ok := ctx.Value(contextkeys.SetupSession).(bool)
	return ok && admitted
}

// consumeSetupOnRootLogin ends authentication setup when user is a root
// instance admin who just signed in through a provider. A failure is logged and
// never fails the sign-in: the worst case is that setup stays open until the
// next root sign-in or the token's expiry.
func (s *Server) consumeSetupOnRootLogin(ctx context.Context, user *models.User) {
	if err := s.container.SetupModeService().ConsumeOnRootLogin(ctx, user); err != nil {
		s.logger.With("handler", "handleCallback", "user_id", user.ID, "error", err).
			Error("Failed to consume the setup token on a root admin sign-in")
	}
}
