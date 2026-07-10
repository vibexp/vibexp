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

func (s *Server) handleCreateModelProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateModelProvider",
		"user_id", userID,
	).Info("Model provider creation request received")

	var req models.CreateModelProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logModelProviderError("handleCreateModelProvider", userID, "", err, "Failed to decode request body")
		apiErr := errors.NewBadRequestError(
			"Invalid request body. Please ensure the JSON is well-formed.",
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	if !s.validateCreateModelProviderRequest(w, r, userID, &req) {
		return
	}

	teamID := chi.URLParam(r, "team_id")
	provider, err := s.container.ModelProviderService().CreateModelProvider(r.Context(), teamID, userID, req)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateModelProvider",
			"user_id", userID,
			"name", req.Name,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create model provider")

		// Check for duplicate provider error using sentinel error
		if stderrors.Is(err, services.ErrModelProviderAlreadyExists) {
			errors.WriteJSONError(w, r, errors.NewModelProviderAlreadyExistsError(req.Name))
			return
		}

		errors.WriteJSONError(w, r, errors.NewModelProviderCreateFailedError(
			"Unable to create model provider. Please check your configuration and try again.",
		))
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateModelProvider",
		"user_id", userID,
		"provider_id", provider.ID,
		"name", req.Name,
	).Info("Model provider created successfully")

	s.writeModelProviderResponse(w, provider)
}

func (s *Server) validateCreateModelProviderRequest(
	w http.ResponseWriter,
	r *http.Request,
	userID string,
	req *models.CreateModelProviderRequest,
) bool {
	var validationErrors []errors.ValidationError

	if req.Name == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateModelProvider",
			"user_id", userID,
		).Error("Model provider name is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("name"))
	}

	if req.ProviderType == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateModelProvider",
			"user_id", userID,
		).Error("Provider type is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("provider_type"))
	}

	if req.Model == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateModelProvider",
			"user_id", userID,
		).Error("Model is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("model"))
	}

	if len(validationErrors) > 0 {
		apiErr := errors.NewModelProviderValidationError(
			"Model provider validation failed. Please check the required fields.",
			validationErrors,
		)
		errors.WriteJSONError(w, r, apiErr)
		return false
	}

	return true
}

func (s *Server) logModelProviderError(
	handler, userID, providerID string,
	err error,
	msg string,
) {
	fields := []any{
		"service", "vibexp-api",
		"handler", handler,
		"user_id", userID,
		"error", fmt.Sprintf("%+v", err),
	}
	if providerID != "" {
		fields = append(fields, "provider_id", providerID)
	}
	s.logger.With(fields...).Error(msg)
}

func (s *Server) writeModelProviderResponse(w http.ResponseWriter, provider *models.ModelProvider) {
	response := models.ModelProviderResponse{
		ModelProvider: *provider,
		HasAPIKey:     provider.APIKeyEncrypted != nil && *provider.APIKeyEncrypted != "",
	}
	response.APIKeyEncrypted = nil

	writeOK(w, response, s.logger)
}

func (s *Server) handleListModelProviders(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleListModelProviders",
		"user_id", userID,
	).Info("Model providers list request received")

	teamID := chi.URLParam(r, "team_id")
	providers, err := s.container.ModelProviderService().GetModelProvidersByTeamID(r.Context(), teamID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListModelProviders",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get model providers")
		errors.WriteJSONError(w, r, errors.NewDatabaseError(
			"Failed to retrieve model providers. Please try again later.",
		))
		return
	}

	writeOK(w, providers, s.logger)
}

func (s *Server) handleGetModelProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	providerID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetModelProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Model provider get request received")

	if providerID == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetModelProvider",
			"user_id", userID,
		).Error("Provider ID is required")
		apiErr := errors.NewModelProviderValidationError(
			"Provider ID is required in the URL path",
			[]errors.ValidationError{errors.NewRequiredFieldError("id")},
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	provider, err := s.container.ModelProviderService().GetModelProvider(r.Context(), teamID, providerID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetModelProvider",
			"user_id", userID,
			"provider_id", providerID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get model provider")
		if stderrors.Is(err, services.ErrModelProviderNotFound) {
			errors.WriteJSONError(w, r, errors.NewModelProviderNotFoundError(providerID))
			return
		}
		errors.WriteJSONError(w, r, errors.NewDatabaseError(
			fmt.Sprintf("Failed to retrieve model provider '%s'. Please try again later.", providerID),
		))
		return
	}

	writeOK(w, provider, s.logger)
}

func (s *Server) handleUpdateModelProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	providerID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateModelProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Model provider update request received")

	if providerID == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleUpdateModelProvider",
			"user_id", userID,
		).Error("Provider ID is required")
		apiErr := errors.NewModelProviderValidationError(
			"Provider ID is required in the URL path",
			[]errors.ValidationError{errors.NewRequiredFieldError("id")},
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	var req models.UpdateModelProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logModelProviderError(
			"handleUpdateModelProvider", userID, providerID, err,
			"Failed to decode request body",
		)
		apiErr := errors.NewBadRequestError(
			"Invalid request body. Please ensure the JSON is well-formed.",
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	provider, err := s.container.ModelProviderService().UpdateModelProvider(r.Context(), teamID, providerID, req)
	if err != nil {
		s.handleUpdateModelProviderError(w, r, userID, providerID, err)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateModelProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Model provider updated successfully")

	s.writeModelProviderResponse(w, provider)
}

func (s *Server) handleUpdateModelProviderError(
	w http.ResponseWriter,
	r *http.Request,
	userID, providerID string,
	err error,
) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateModelProvider",
		"user_id", userID,
		"provider_id", providerID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to update model provider")

	if stderrors.Is(err, services.ErrModelProviderNotFound) {
		errors.WriteJSONError(w, r, errors.NewModelProviderNotFoundError(providerID))
		return
	}
	errors.WriteJSONError(w, r, errors.NewModelProviderUpdateFailedError(
		"Unable to update model provider. Please check your configuration and try again.",
	))
}

func (s *Server) handleDeleteModelProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	providerID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleDeleteModelProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Model provider deletion request received")

	if providerID == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteModelProvider",
			"user_id", userID,
		).Error("Provider ID is required")
		apiErr := errors.NewModelProviderValidationError(
			"Provider ID is required in the URL path",
			[]errors.ValidationError{errors.NewRequiredFieldError("id")},
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	err := s.container.ModelProviderService().DeleteModelProvider(r.Context(), teamID, providerID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteModelProvider",
			"user_id", userID,
			"provider_id", providerID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to delete model provider")
		if stderrors.Is(err, services.ErrModelProviderNotFound) {
			errors.WriteJSONError(w, r, errors.NewModelProviderNotFoundError(providerID))
			return
		}
		if stderrors.Is(err, services.ErrLastModelProviderDelete) {
			errors.WriteJSONError(w, r, errors.NewModelProviderLastDeleteBlockedError())
			return
		}
		errors.WriteJSONError(w, r, errors.NewModelProviderDeleteFailedError(
			"Unable to delete model provider. Please try again later.",
		))
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleDeleteModelProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Model provider deleted successfully")

	writeNoContent(w)
}

func (s *Server) handleValidateModelProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleValidateModelProvider",
		"user_id", userID,
	).Info("Model provider validation request received")

	var req models.ValidateModelProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logModelProviderError("handleValidateModelProvider", userID, "", err, "Failed to decode request body")
		apiErr := errors.NewBadRequestError(
			"Invalid request body. Please ensure the JSON is well-formed.",
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	if !s.validateModelProviderRequest(w, r, userID, &req) {
		return
	}

	response, err := s.container.ModelProviderService().ValidateModelProvider(r.Context(), req)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleValidateModelProvider",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to validate model provider")
		// Service errors (network issues, etc.) are internal errors - don't expose raw error
		errors.WriteJSONError(w, r, errors.NewInternalError(
			"Provider validation failed due to a service error. Please try again later.",
		))
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleValidateModelProvider",
		"user_id", userID,
		"is_valid", response.IsValid,
		"message", response.Message,
	).Info("Model provider validation completed")

	writeOK(w, response, s.logger)
}

func (s *Server) validateModelProviderRequest(
	w http.ResponseWriter,
	r *http.Request,
	userID string,
	req *models.ValidateModelProviderRequest,
) bool {
	var validationErrors []errors.ValidationError

	if req.ProviderType == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleValidateModelProvider",
			"user_id", userID,
		).Error("Provider type is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("provider_type"))
	}

	if req.BaseURL == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleValidateModelProvider",
			"user_id", userID,
		).Error("Base URL is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("base_url"))
	}

	if req.Model == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleValidateModelProvider",
			"user_id", userID,
		).Error("Model is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("model"))
	}

	if len(validationErrors) > 0 {
		apiErr := errors.NewModelProviderValidationError(
			"Provider validation request is missing required fields",
			validationErrors,
		)
		errors.WriteJSONError(w, r, apiErr)
		return false
	}

	return true
}
