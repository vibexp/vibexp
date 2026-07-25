package server

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// HTTP surface for the per-team email provider (#503, epic #499).
//
// The credential never appears in a response by construction rather than by
// masking: the service returns models.TeamEmailProviderEffective, which has no
// field able to carry a secret — only the has_credential boolean derived from it.
// There is nowhere in these handlers where a secret could be forgotten.

// teamEmailProviderPermissionMessage is what a caller without the right role is
// told. Configuring the provider decides what address the team's mail comes from
// and stores a sending credential, so it is owner/admin surface.
const teamEmailProviderPermissionMessage = "You do not have permission to manage this team's email provider."

// writeTeamEmailProviderError maps the service and repository sentinels onto their
// documented status codes. Permission denial is checked FIRST by every caller, so
// an authorization failure can never be reported as a generic failure.
func writeTeamEmailProviderError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case stderrors.Is(err, repositories.ErrTeamEmailProviderNotFound):
		errors.WriteJSONError(w, r, errors.NewTeamEmailProviderNotConfiguredError())
	case stderrors.Is(err, services.ErrTeamEmailProviderValidation):
		errors.WriteJSONError(w, r, errors.NewTeamEmailProviderValidationError(
			"The email provider configuration is invalid", teamEmailProviderValidationErrors(err)))
	default:
		return false
	}
	return true
}

// teamEmailProviderValidationErrors converts the service's field errors into the
// API's validation-error shape. The services package is HTTP-agnostic and cannot
// import internal/errors, so the translation happens here.
func teamEmailProviderValidationErrors(err error) []errors.ValidationError {
	var verr *services.TeamEmailProviderValidationError
	if !stderrors.As(err, &verr) {
		return nil
	}

	out := make([]errors.ValidationError, 0, len(verr.Fields))
	for _, field := range verr.Fields {
		out = append(out, errors.ValidationError{
			Field:   field.Field,
			Message: field.Message,
		})
	}
	return out
}

// writeTeamEmailProviderAPIError is the shared tail of every handler's error path:
// 403 first, then the mapped sentinels, then a generic failure. Checking 403
// before the sentinels is what stops an authorization failure degrading into a
// 500.
func (s *Server) writeTeamEmailProviderAPIError(
	w http.ResponseWriter, r *http.Request, handler, userID, teamID string, err error, fallback *errors.APIError,
) {
	if writeIfPermissionDeniedWithMessage(w, r, err, teamEmailProviderPermissionMessage) {
		return
	}
	if writeTeamEmailProviderError(w, r, err) {
		return
	}

	s.logger.With(
		"service", serverLogServiceName,
		"handler", handler,
		"user_id", userID,
		"team_id", teamID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Team email provider request failed")

	errors.WriteJSONError(w, r, fallback)
}

// teamEmailProviderRequestContext pulls the two identifiers every handler needs.
// team_id is a UUID, so it carries no reserved characters and needs no
// percent-decoding.
func teamEmailProviderRequestContext(r *http.Request) (userID, teamID string) {
	userID, _ = r.Context().Value(contextKeyUserID).(string)
	return userID, chi.URLParam(r, "team_id")
}

// handleGetTeamEmailProvider reports the configuration in force for the team.
//
// This ALWAYS returns 200. A team with no provider of its own is inheriting the
// instance provider — a valid state the caller needs described, not a missing
// resource. Returning 404 would make the fallback invisible to the UI.
func (s *Server) handleGetTeamEmailProvider(w http.ResponseWriter, r *http.Request) {
	userID, teamID := teamEmailProviderRequestContext(r)

	effective, err := s.container.TeamEmailProviderService().GetEffective(r.Context(), userID, teamID)
	if err != nil {
		s.writeTeamEmailProviderAPIError(w, r, "handleGetTeamEmailProvider", userID, teamID, err,
			errors.NewInternalError("Unable to load the email provider configuration."))
		return
	}

	writeOK(w, effective, s.logger)
}

// handleUpsertTeamEmailProvider creates or replaces the team's provider.
func (s *Server) handleUpsertTeamEmailProvider(w http.ResponseWriter, r *http.Request) {
	userID, teamID := teamEmailProviderRequestContext(r)

	var req models.UpsertTeamEmailProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteJSONError(w, r, errors.NewBadRequestError(msgInvalidBodyWellFormedJSON))
		return
	}

	if _, err := s.container.TeamEmailProviderService().Upsert(r.Context(), userID, teamID, req); err != nil {
		s.writeTeamEmailProviderAPIError(w, r, "handleUpsertTeamEmailProvider", userID, teamID, err,
			errors.NewTeamEmailProviderUpdateFailedError("Unable to save the email provider configuration."))
		return
	}

	// Re-read rather than echoing what was stored: the response is the effective
	// view, and building it from the row keeps one construction path for GET and
	// PUT so they can never disagree.
	effective, err := s.container.TeamEmailProviderService().GetEffective(r.Context(), userID, teamID)
	if err != nil {
		s.writeTeamEmailProviderAPIError(w, r, "handleUpsertTeamEmailProvider", userID, teamID, err,
			errors.NewTeamEmailProviderUpdateFailedError("Unable to load the saved email provider configuration."))
		return
	}

	writeOK(w, effective, s.logger)
}

// handleDeleteTeamEmailProvider removes the team's provider, reverting it to the
// instance provider.
func (s *Server) handleDeleteTeamEmailProvider(w http.ResponseWriter, r *http.Request) {
	userID, teamID := teamEmailProviderRequestContext(r)

	if err := s.container.TeamEmailProviderService().Delete(r.Context(), userID, teamID); err != nil {
		s.writeTeamEmailProviderAPIError(w, r, "handleDeleteTeamEmailProvider", userID, teamID, err,
			errors.NewTeamEmailProviderDeleteFailedError("Unable to remove the email provider configuration."))
		return
	}

	writeNoContent(w)
}

// handleTestTeamEmailProvider sends a test message with the configuration in the
// request body.
//
// The recipient is resolved server-side from the acting user, so any recipient a
// caller puts in the body is ignored — this endpoint cannot send mail to third
// parties. A failed send is a 200 with is_valid: false; 500 is reserved for
// internal faults.
func (s *Server) handleTestTeamEmailProvider(w http.ResponseWriter, r *http.Request) {
	userID, teamID := teamEmailProviderRequestContext(r)

	var req models.TestTeamEmailProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req.UpsertTeamEmailProviderRequest); err != nil {
		errors.WriteJSONError(w, r, errors.NewBadRequestError(msgInvalidBodyWellFormedJSON))
		return
	}

	result, err := s.container.TeamEmailProviderService().Test(r.Context(), userID, teamID, req)
	if err != nil {
		s.writeTeamEmailProviderAPIError(w, r, "handleTestTeamEmailProvider", userID, teamID, err,
			errors.NewInternalError("Unable to send the test email."))
		return
	}

	writeOK(w, models.NewTeamEmailProviderTestResponse(result), s.logger)
}
