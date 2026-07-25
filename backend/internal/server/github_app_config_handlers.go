package server

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

// HTTP surface for per-team GitHub App configuration (#479, epic #476).
//
// Secrets never appear in a response by construction rather than by masking:
// the service returns models.GitHubAppConfigResponse, which has no field able
// to carry a private key or client secret, and models.GitHubAppConfigCreated,
// whose extra webhook_secret field is returned only by create. There is no
// place in these handlers where a secret could be forgotten.

// githubAppPermissionMessage is what a caller without the right role is told.
// An App registration is team-level configuration holding GitHub credentials,
// so it is owner/admin surface.
const githubAppPermissionMessage = "You do not have permission to manage this team's GitHub App."

// writeGitHubAppConfigError maps the service sentinels onto their documented
// status codes. Permission denial is checked FIRST in every caller, so an
// authorization failure can never be reported as a generic failure — the caller
// needs to know it is a role problem, and the operator needs the distinction in
// logs.
//
// All three conflict sentinels are 409 rather than 404: the endpoint exists and
// the team is addressable, it is simply not in a state that can serve the
// request. A 404 would wrongly suggest a bad URL.
func writeGitHubAppConfigError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case stderrors.Is(err, services.ErrGitHubAppNotConfigured):
		errors.WriteJSONError(w, r, errors.NewGitHubAppNotConfiguredError())
	case stderrors.Is(err, services.ErrGitHubAppAlreadyRegistered):
		errors.WriteJSONError(w, r, errors.NewGitHubAppAlreadyRegisteredError())
	case stderrors.Is(err, services.ErrGitHubAppConfigExists):
		errors.WriteJSONError(w, r, errors.NewGitHubAppConfigExistsError())
	case stderrors.Is(err, services.ErrGitHubAppConfigConflict):
		errors.WriteJSONError(w, r, errors.NewGitHubAppConfigConflictError())
	default:
		return false
	}
	return true
}

// writeGitHubAppAPIError is the shared tail of every handler's error path:
// 403 first, then the mapped sentinels, then whatever the service reported if
// it is already an APIError (a validation error from an unparseable key), and
// finally a generic 500.
func (s *Server) writeGitHubAppAPIError(
	w http.ResponseWriter, r *http.Request, handler, userID, teamID string, err error, fallback string,
) {
	if writeIfPermissionDeniedWithMessage(w, r, err, githubAppPermissionMessage) {
		return
	}
	if writeGitHubAppConfigError(w, r, err) {
		return
	}

	// A field-level validation error (for example an unparseable private key)
	// arrives as an *errors.APIError already carrying its 400 and field list;
	// passing it through beats flattening user input errors into a 500.
	var apiErr *errors.APIError
	if stderrors.As(err, &apiErr) {
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With(
		"service", serverLogServiceName,
		"handler", handler,
		"user_id", userID,
		"team_id", teamID,
		"error", fmt.Sprintf("%+v", err),
	).Error("GitHub App configuration request failed")

	errors.WriteJSONError(w, r, errors.NewInternalError(fallback))
}

// githubAppRequestContext pulls the two identifiers every handler needs.
func githubAppRequestContext(r *http.Request) (userID, teamID string) {
	userID, _ = r.Context().Value(contextKeyUserID).(string)
	return userID, chi.URLParam(r, "team_id")
}

func (s *Server) handleGetGitHubAppConfig(w http.ResponseWriter, r *http.Request) {
	userID, teamID := githubAppRequestContext(r)

	config, err := s.container.GitHubAppConfigService().GetAppConfig(r.Context(), teamID)
	if err != nil {
		s.writeGitHubAppAPIError(w, r, "handleGetGitHubAppConfig", userID, teamID, err,
			"Unable to load the GitHub App configuration.")
		return
	}

	writeOK(w, config, s.logger)
}

func (s *Server) handleCreateGitHubAppConfig(w http.ResponseWriter, r *http.Request) {
	userID, teamID := githubAppRequestContext(r)

	var req models.CreateGitHubAppConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteJSONError(w, r, errors.NewBadRequestError(msgInvalidBodyWellFormedJSON))
		return
	}

	if validationErrs := validateCreateGitHubAppConfigRequest(req); len(validationErrs) > 0 {
		errors.WriteJSONError(w, r, errors.NewValidationError(
			"The GitHub App configuration is incomplete", validationErrs))
		return
	}

	created, err := s.container.GitHubAppConfigService().CreateAppConfig(r.Context(), teamID, userID, req)
	if err != nil {
		s.writeGitHubAppAPIError(w, r, "handleCreateGitHubAppConfig", userID, teamID, err,
			"Unable to register the GitHub App. Please check the configuration and try again.")
		return
	}

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleCreateGitHubAppConfig",
		"user_id", userID,
		"team_id", teamID,
		"app_id", req.AppID,
	).Info("GitHub App registered")

	writeOK(w, created, s.logger)
}

func (s *Server) handleUpdateGitHubAppConfig(w http.ResponseWriter, r *http.Request) {
	userID, teamID := githubAppRequestContext(r)

	var req models.UpdateGitHubAppConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteJSONError(w, r, errors.NewBadRequestError(msgInvalidBodyWellFormedJSON))
		return
	}

	config, err := s.container.GitHubAppConfigService().UpdateAppConfig(r.Context(), teamID, userID, req)
	if err != nil {
		s.writeGitHubAppAPIError(w, r, "handleUpdateGitHubAppConfig", userID, teamID, err,
			"Unable to update the GitHub App configuration.")
		return
	}

	writeOK(w, config, s.logger)
}

func (s *Server) handleDeleteGitHubAppConfig(w http.ResponseWriter, r *http.Request) {
	userID, teamID := githubAppRequestContext(r)

	if err := s.container.GitHubAppConfigService().DeleteAppConfig(r.Context(), teamID, userID); err != nil {
		s.writeGitHubAppAPIError(w, r, "handleDeleteGitHubAppConfig", userID, teamID, err,
			"Unable to delete the GitHub App configuration.")
		return
	}

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleDeleteGitHubAppConfig",
		"user_id", userID,
		"team_id", teamID,
	).Info("GitHub App configuration deleted")

	writeNoContent(w)
}

func (s *Server) handleValidateGitHubAppConfig(w http.ResponseWriter, r *http.Request) {
	userID, teamID := githubAppRequestContext(r)

	// A failed probe comes back as a 200 body with is_valid=false; only an
	// authorization or missing-config failure reaches the error path.
	result, err := s.container.GitHubAppConfigService().ValidateAppConfig(r.Context(), teamID, userID)
	if err != nil {
		s.writeGitHubAppAPIError(w, r, "handleValidateGitHubAppConfig", userID, teamID, err,
			"Unable to validate the GitHub App configuration.")
		return
	}

	writeOK(w, result, s.logger)
}

func (s *Server) handleRotateGitHubAppWebhookToken(w http.ResponseWriter, r *http.Request) {
	userID, teamID := githubAppRequestContext(r)

	config, err := s.container.GitHubAppConfigService().RotateWebhookToken(r.Context(), teamID, userID)
	if err != nil {
		s.writeGitHubAppAPIError(w, r, "handleRotateGitHubAppWebhookToken", userID, teamID, err,
			"Unable to rotate the GitHub App webhook token.")
		return
	}

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleRotateGitHubAppWebhookToken",
		"user_id", userID,
		"team_id", teamID,
	).Info("GitHub App webhook token rotated")

	writeOK(w, config, s.logger)
}

// validateCreateGitHubAppConfigRequest checks the fields the spec marks
// required. The private key's parseability is checked by the service, which
// owns the PEM handling; this only catches what is structurally absent.
func validateCreateGitHubAppConfigRequest(
	req models.CreateGitHubAppConfigRequest,
) []errors.ValidationError {
	var validationErrs []errors.ValidationError

	required := []struct {
		field, value, message string
	}{
		{"app_id", req.AppID, "app_id is required"},
		{"app_slug", req.AppSlug, "app_slug is required"},
		{"client_id", req.ClientID, "client_id is required"},
		{"private_key", req.PrivateKey, "private_key is required"},
		{"client_secret", req.ClientSecret, "client_secret is required"},
	}
	for _, f := range required {
		if f.value == "" {
			validationErrs = append(validationErrs, errors.ValidationError{
				Field:   f.field,
				Message: f.message,
			})
		}
	}

	return validationErrs
}
