package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/vibexp/vibexp/internal/auth/password"
	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	invitationgen "github.com/vibexp/vibexp/internal/server/gen/invitationregistration"
	"github.com/vibexp/vibexp/internal/services"
)

// Public invitation operations (#1435, epic #1426): what the holder of an
// invitation link can do with no account and no session. Thin adapters over
// InvitationRegistrationService.
//
// Both are authenticated by the token in the path and nothing else, so both
// sit behind the strict per-IP auth limit. Registration signs the person in,
// which makes it a login: it requires a JSON body (an HTML form on another
// site cannot then sign a visitor's browser in to an account its author
// controls) and exists only while password login is switched on.

const (
	invitationRegistrationHandler = "RegisterWithInvitation"
	invitationPreviewHandler      = "PreviewInvitation"

	invitationInvalidDetail       = "This invitation link is not valid. It may have expired or been used or withdrawn."
	invitationAccountExistsDetail = "An account already exists for the invited email. Sign in to accept the invitation."
	invitationNotAllowedDetail    = "Access to this instance is restricted. " +
		"Contact your administrator if you believe this is a mistake."
	invitationFailedDetail = "The invitation request could not be completed"
	// invitationRegisteredNotSignedInDetail answers the one case where the
	// account exists and the session could not be minted.
	invitationRegisteredNotSignedInDetail = "Your account was created, but you could not be signed in. " +
		"Sign in with your email and password."

	// maxInvitationTokenBytes bounds the token before it is looked up. Stored
	// tokens are far shorter.
	maxInvitationTokenBytes = 512
	// signInViaInvitation is the `via` recorded on the sign-in activity.
	signInViaInvitation = "invitation"
)

// setupInvitationRegistrationRoutes mounts the public invitation operations at
// their spec paths. They live beside the authenticated invitation routes
// (/api/v1/invitations/{token}, …/accept, …/reject), which are a separate,
// authenticated subtree.
func (s *Server) setupInvitationRegistrationRoutes() {
	keepRequest := func(next invitationgen.StrictHandlerFunc, _ string) invitationgen.StrictHandlerFunc {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
			// The same key the password sign-in uses: signInWithPassword
			// reads the request from it to record the sign-in activity.
			return next(context.WithValue(ctx, passwordAuthRequestKey{}, r), w, r, request)
		}
	}
	strict := invitationgen.NewStrictHandlerWithOptions(
		&invitationRegistrationStrictServer{s: s},
		[]invitationgen.StrictMiddlewareFunc{keepRequest},
		invitationgen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  s.invitationRegistrationBindErrorHandler,
			ResponseErrorHandlerFunc: s.passwordAuthResponseErrorHandler,
		},
	)
	wrapper := invitationgen.ServerInterfaceWrapper{
		Handler:          strict,
		ErrorHandlerFunc: s.invitationRegistrationBindErrorHandler,
	}
	s.router.Group(func(r chi.Router) {
		s.signInRateLimit(r)
		r.Get("/api/v1/invitations/{token}/preview", wrapper.PreviewInvitation)
		// Registration is a sign-in: it does not exist while password login
		// is off, and it takes JSON only.
		r.With(s.requirePasswordLoginEnabled, requireJSONBody, rejectUnknownInvitationRegisterFields).
			Post("/api/v1/invitations/{token}/register", wrapper.RegisterWithInvitation)
	})
}

// invitationRegistrationBindErrorHandler turns a request-decoding failure from
// the generated layer (an undecodable path token, a malformed body) into an
// RFC 9457 400. The error can quote the body or the token, so it is neither
// logged nor returned.
func (s *Server) invitationRegistrationBindErrorHandler(w http.ResponseWriter, r *http.Request, _ error) {
	apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("Invalid request"))
}

// invitationRegisterBodyFields are the fields the register body accepts,
// derived from the generated request type so they follow the spec.
var invitationRegisterBodyFields = allowedJSONFields(invitationgen.InvitationRegisterRequest{})

// rejectUnknownInvitationRegisterFields enforces the `additionalProperties:
// false` the spec declares, which oapi-codegen does not. Only field NAMES are
// reported, never values. A body that is empty or not a JSON object is left for
// the generated decoder.
func rejectUnknownInvitationRegisterFields(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}
		fields, ok := peekJSONObjectBody(w, r)
		if !ok {
			return
		}
		if unknown := unknownFields(fields, invitationRegisterBodyFields); len(unknown) > 0 {
			apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError(fmt.Sprintf(
				"Unknown field(s): %s. Only %s may be set.",
				strings.Join(unknown, ", "), strings.Join(sortedKeys(invitationRegisterBodyFields), ", "))))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// invitationRegistrationStrictServer implements invitationgen.StrictServerInterface.
type invitationRegistrationStrictServer struct {
	s *Server
}

var _ invitationgen.StrictServerInterface = (*invitationRegistrationStrictServer)(nil)

// PreviewInvitation handles GET /api/v1/invitations/{token}/preview.
func (h *invitationRegistrationStrictServer) PreviewInvitation(
	ctx context.Context, request invitationgen.PreviewInvitationRequestObject,
) (invitationgen.PreviewInvitationResponseObject, error) {
	if len(request.Token) > maxInvitationTokenBytes {
		return nil, invitationInvalidProblem()
	}
	preview, err := h.s.container.InvitationRegistrationService().Preview(ctx, request.Token)
	if err != nil {
		return nil, h.invitationError(invitationPreviewHandler, request.Token, err)
	}

	resp := invitationgen.InvitationPreview{
		TeamName:             preview.TeamName,
		Email:                openapi_types.Email(preview.Email),
		Role:                 invitationgen.InvitationPreviewRole(preview.Role),
		ExpiresAt:            preview.ExpiresAt,
		AccountExists:        preview.AccountExists,
		PasswordRegistration: preview.PasswordRegistration,
	}
	if preview.InviterName != "" {
		resp.InviterName = &preview.InviterName
	}
	return invitationgen.PreviewInvitation200JSONResponse(resp), nil
}

// RegisterWithInvitation handles POST /api/v1/invitations/{token}/register.
func (h *invitationRegistrationStrictServer) RegisterWithInvitation(
	ctx context.Context, request invitationgen.RegisterWithInvitationRequestObject,
) (invitationgen.RegisterWithInvitationResponseObject, error) {
	if len(request.Token) > maxInvitationTokenBytes {
		return nil, invitationInvalidProblem()
	}
	if request.Body == nil || request.Body.Password == "" {
		return nil, apierrors.NewValidationError("A password is required", []apierrors.ValidationError{
			apierrors.NewFieldValidationError("password", "A password is required", "required"),
		})
	}
	name := ""
	if request.Body.Name != nil {
		name = *request.Body.Name
	}
	// The bounds the spec declares. Nothing else enforces them, and without
	// them one request could have megabytes of "password" hashed.
	if len(request.Body.Password) > maxSignInPasswordBytes ||
		utf8.RuneCountInString(name) > services.InvitationNameMaxLength {
		return nil, apierrors.NewBadRequestError("password or name is too long")
	}
	if h.s.sessionManager == nil {
		// Checked before anything is created: an account nobody can be signed
		// in to would only be confusing.
		h.s.logger.With("handler", invitationRegistrationHandler).Error("Session manager is not configured")
		return nil, apierrors.NewInternalError(invitationFailedDetail)
	}

	result, err := h.s.container.InvitationRegistrationService().
		Register(ctx, request.Token, request.Body.Password, name)
	if err != nil {
		return nil, h.invitationError(invitationRegistrationHandler, request.Token, err)
	}

	signIn := &passwordAuthStrictServer{s: h.s}
	setCookie, err := signIn.signInWithPassword(ctx, invitationRegistrationHandler, result.User, signInViaInvitation)
	if err != nil {
		// The account exists and its password works; only the session failed.
		return nil, apierrors.NewInternalError(invitationRegisteredNotSignedInDetail)
	}

	teamID, err := uuid.Parse(result.TeamID)
	if err != nil {
		h.s.logger.With("handler", invitationRegistrationHandler, "error", err.Error()).
			Error("Invitation registration produced a team id that is not a UUID")
		return nil, apierrors.NewInternalError(invitationFailedDetail)
	}
	return invitationgen.RegisterWithInvitation200JSONResponse{
		Body: invitationgen.InvitationRegistration{
			User:               toGenInvitationUser(result.User),
			TeamId:             teamID,
			InvitationAccepted: result.InvitationAccepted,
		},
		Headers: invitationgen.RegisterWithInvitation200ResponseHeaders{SetCookie: &setCookie},
	}, nil
}

// invitationInvalidProblem is the one 404 for a link that cannot be used.
func invitationInvalidProblem() *apierrors.APIError {
	return apierrors.NewAPIError(apierrors.CodeInvitationInvalid,
		apierrors.GetErrorTitle(apierrors.CodeInvitationInvalid), invitationInvalidDetail, http.StatusNotFound)
}

// invitationError maps a service error onto its HTTP shape. The token is
// logged as a digest only.
func (h *invitationRegistrationStrictServer) invitationError(handler, token string, err error) error {
	var policyErr *password.PolicyError
	switch {
	case errors.Is(err, services.ErrInvitationUnusable):
		return invitationInvalidProblem()
	case errors.Is(err, services.ErrInvitationAccountExists):
		return apierrors.NewAPIError(apierrors.CodeAccountExists,
			apierrors.GetErrorTitle(apierrors.CodeAccountExists), invitationAccountExistsDetail, http.StatusConflict)
	case errors.Is(err, services.ErrInvitationEmailNotAllowed):
		return apierrors.NewAccessRestrictedError(invitationNotAllowedDetail)
	case errors.As(err, &policyErr):
		return passwordPolicyProblem(policyErr)
	}
	h.s.logger.With("handler", handler, "token_digest", redactToken(token), "error", err.Error()).
		Error("Invitation request failed")
	return apierrors.NewInternalError(invitationFailedDetail)
}

// toGenInvitationUser maps the new account onto the User schema, the same body
// password sign-in returns.
func toGenInvitationUser(user *models.User) invitationgen.User {
	return invitationgen.User{
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
