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

func (s *Server) handleCreateEmbeddingProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateEmbeddingProvider",
		"user_id", userID,
	).Info("Embedding provider creation request received")

	var req models.CreateEmbeddingProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logEmbeddingProviderError("handleCreateEmbeddingProvider", userID, "", err, "Failed to decode request body")
		apiErr := errors.NewBadRequestError(
			"Invalid request body. Please ensure the JSON is well-formed.",
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	if !s.validateCreateEmbeddingProviderRequest(w, r, userID, &req) {
		return
	}

	provider, err := s.container.EmbeddingProviderService().CreateEmbeddingProvider(r.Context(), userID, req)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateEmbeddingProvider",
			"user_id", userID,
			"name", req.Name,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create embedding provider")

		// Check for duplicate provider error using sentinel error
		if stderrors.Is(err, services.ErrProviderAlreadyExists) {
			errors.WriteJSONError(w, r, errors.NewProviderAlreadyExistsError(req.Name))
			return
		}

		errors.WriteJSONError(w, r, errors.NewProviderCreateFailedError(
			"Unable to create embedding provider. Please check your configuration and try again.",
		))
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateEmbeddingProvider",
		"user_id", userID,
		"provider_id", provider.ID,
		"name", req.Name,
	).Info("Embedding provider created successfully")

	s.writeEmbeddingProviderResponse(w, provider)
}

func (s *Server) validateCreateEmbeddingProviderRequest(
	w http.ResponseWriter,
	r *http.Request,
	userID string,
	req *models.CreateEmbeddingProviderRequest,
) bool {
	var validationErrors []errors.ValidationError

	if req.Name == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateEmbeddingProvider",
			"user_id", userID,
		).Error("Embedding provider name is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("name"))
	}

	if req.ProviderType == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateEmbeddingProvider",
			"user_id", userID,
		).Error("Provider type is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("provider_type"))
	}

	if len(validationErrors) > 0 {
		apiErr := errors.NewProviderValidationError(
			"Embedding provider validation failed. Please check the required fields.",
			validationErrors,
		)
		errors.WriteJSONError(w, r, apiErr)
		return false
	}

	return true
}

func (s *Server) logEmbeddingProviderError(
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

func (s *Server) writeEmbeddingProviderResponse(w http.ResponseWriter, provider *models.EmbeddingProvider) {
	response := models.EmbeddingProviderResponse{
		EmbeddingProvider: *provider,
		HasAPIKey:         provider.APIKeyEncrypted != nil && *provider.APIKeyEncrypted != "",
	}
	response.APIKeyEncrypted = nil

	writeOK(w, response, s.logger)
}

func (s *Server) handleListEmbeddingProviders(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleListEmbeddingProviders",
		"user_id", userID,
	).Info("Embedding providers list request received")

	providers, err := s.container.EmbeddingProviderService().GetEmbeddingProvidersByUserID(r.Context(), userID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListEmbeddingProviders",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get embedding providers")
		errors.WriteJSONError(w, r, errors.NewDatabaseError(
			"Failed to retrieve embedding providers. Please try again later.",
		))
		return
	}

	writeOK(w, providers, s.logger)
}

func (s *Server) handleGetEmbeddingProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	providerID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Embedding provider get request received")

	if providerID == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetEmbeddingProvider",
			"user_id", userID,
		).Error("Provider ID is required")
		apiErr := errors.NewProviderValidationError(
			"Provider ID is required in the URL path",
			[]errors.ValidationError{errors.NewRequiredFieldError("id")},
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	provider, err := s.container.EmbeddingProviderService().GetEmbeddingProvider(r.Context(), userID, providerID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetEmbeddingProvider",
			"user_id", userID,
			"provider_id", providerID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get embedding provider")
		if stderrors.Is(err, services.ErrProviderNotFound) {
			errors.WriteJSONError(w, r, errors.NewProviderNotFoundError(providerID))
			return
		}
		errors.WriteJSONError(w, r, errors.NewDatabaseError(
			fmt.Sprintf("Failed to retrieve embedding provider '%s'. Please try again later.", providerID),
		))
		return
	}

	writeOK(w, provider, s.logger)
}

func (s *Server) handleUpdateEmbeddingProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	providerID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Embedding provider update request received")

	if providerID == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleUpdateEmbeddingProvider",
			"user_id", userID,
		).Error("Provider ID is required")
		apiErr := errors.NewProviderValidationError(
			"Provider ID is required in the URL path",
			[]errors.ValidationError{errors.NewRequiredFieldError("id")},
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	var req models.UpdateEmbeddingProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logEmbeddingProviderError(
			"handleUpdateEmbeddingProvider", userID, providerID, err,
			"Failed to decode request body",
		)
		apiErr := errors.NewBadRequestError(
			"Invalid request body. Please ensure the JSON is well-formed.",
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	provider, err := s.container.EmbeddingProviderService().UpdateEmbeddingProvider(r.Context(), userID, providerID, req)
	if err != nil {
		s.handleUpdateEmbeddingProviderError(w, r, userID, providerID, err)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Embedding provider updated successfully")

	s.writeEmbeddingProviderResponse(w, provider)
}

func (s *Server) handleUpdateEmbeddingProviderError(
	w http.ResponseWriter,
	r *http.Request,
	userID, providerID string,
	err error,
) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to update embedding provider")

	if stderrors.Is(err, services.ErrProviderNotFound) {
		errors.WriteJSONError(w, r, errors.NewProviderNotFoundError(providerID))
		return
	}
	errors.WriteJSONError(w, r, errors.NewProviderUpdateFailedError(
		"Unable to update embedding provider. Please check your configuration and try again.",
	))
}

func (s *Server) handleDeleteEmbeddingProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	providerID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleDeleteEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Embedding provider deletion request received")

	if providerID == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteEmbeddingProvider",
			"user_id", userID,
		).Error("Provider ID is required")
		apiErr := errors.NewProviderValidationError(
			"Provider ID is required in the URL path",
			[]errors.ValidationError{errors.NewRequiredFieldError("id")},
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	err := s.container.EmbeddingProviderService().DeleteEmbeddingProvider(r.Context(), userID, providerID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteEmbeddingProvider",
			"user_id", userID,
			"provider_id", providerID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to delete embedding provider")
		if stderrors.Is(err, services.ErrProviderNotFound) {
			errors.WriteJSONError(w, r, errors.NewProviderNotFoundError(providerID))
			return
		}
		if stderrors.Is(err, services.ErrLastProviderDelete) {
			errors.WriteJSONError(w, r, errors.NewProviderLastDeleteBlockedError())
			return
		}
		errors.WriteJSONError(w, r, errors.NewProviderDeleteFailedError(
			"Unable to delete embedding provider. Please try again later.",
		))
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleDeleteEmbeddingProvider",
		"user_id", userID,
		"provider_id", providerID,
	).Info("Embedding provider deleted successfully")

	writeNoContent(w)
}

func (s *Server) handleValidateEmbeddingProvider(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleValidateEmbeddingProvider",
		"user_id", userID,
	).Info("Embedding provider validation request received")

	var req models.ValidateEmbeddingProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logEmbeddingProviderError("handleValidateEmbeddingProvider", userID, "", err, "Failed to decode request body")
		apiErr := errors.NewBadRequestError(
			"Invalid request body. Please ensure the JSON is well-formed.",
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	if !s.validateEmbeddingProviderRequest(w, r, userID, &req) {
		return
	}

	response, err := s.container.EmbeddingProviderService().ValidateEmbeddingProvider(r.Context(), req)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleValidateEmbeddingProvider",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to validate embedding provider")
		// Service errors (network issues, etc.) are internal errors - don't expose raw error
		errors.WriteJSONError(w, r, errors.NewInternalError(
			"Provider validation failed due to a service error. Please try again later.",
		))
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleValidateEmbeddingProvider",
		"user_id", userID,
		"is_valid", response.IsValid,
		"message", response.Message,
	).Info("Embedding provider validation completed")

	writeOK(w, response, s.logger)
}

func (s *Server) validateEmbeddingProviderRequest(
	w http.ResponseWriter,
	r *http.Request,
	userID string,
	req *models.ValidateEmbeddingProviderRequest,
) bool {
	var validationErrors []errors.ValidationError

	if req.ProviderType == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleValidateEmbeddingProvider",
			"user_id", userID,
		).Error("Provider type is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("provider_type"))
	}

	if req.BaseURL == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleValidateEmbeddingProvider",
			"user_id", userID,
		).Error("Base URL is required")
		validationErrors = append(validationErrors, errors.NewRequiredFieldError("base_url"))
	}

	if len(validationErrors) > 0 {
		apiErr := errors.NewProviderValidationError(
			"Provider validation request is missing required fields",
			validationErrors,
		)
		errors.WriteJSONError(w, r, apiErr)
		return false
	}

	return true
}
