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
	"github.com/vibexp/vibexp/internal/services/activities"
)

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateAPIKey",
		"user_id", userID,
	).
		Info("API key creation request received")

	var req models.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logAPIKeyError("handleCreateAPIKey", userID, "Failed to decode request body", err)
		apiErr := errors.NewBadRequestError("Invalid request body")
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	if !s.validateAPIKeyName(w, r, userID, req.Name) {
		return
	}

	if !s.validateIntegrationCodes(w, r, userID, req.IntegrationCodes) {
		return
	}

	apiKey, fullKey, err := s.container.APIKeyService().
		GenerateAPIKey(r.Context(), userID, req.Name, req.IntegrationCodes)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateAPIKey",
			"user_id", userID,
			"name", req.Name,
			"integrations", req.IntegrationCodes,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to generate API key")
		apiErr := errors.NewInternalError("Failed to generate API key")
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateAPIKey",
		"user_id", userID,
		"api_key_id", apiKey.ID,
		"name", req.Name,
		"integrations", apiKey.Integrations,
	).Info("API key created successfully")

	s.recordAPIKeyActivity(
		r.Context(), userID, activities.ActivityTypeAPIKeyCreated,
		apiKey.ID, "Created new API key: "+req.Name, r,
	)

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordAPIKeyCreated(r.Context())
	}

	s.writeAPIKeyResponse(w, apiKey, fullKey)
}

func (s *Server) validateAPIKeyName(w http.ResponseWriter, r *http.Request, userID, name string) bool {
	if name == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateAPIKey",
			"user_id", userID,
		).
			Error("API key name is required")
		validationErrs := []errors.ValidationError{
			errors.NewRequiredFieldError("name"),
		}
		apiErr := errors.NewValidationError("Request validation failed", validationErrs)
		errors.WriteJSONError(w, r, apiErr)
		return false
	}

	if len(name) > 255 {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateAPIKey",
			"user_id", userID,
			"name_len", len(name),
		).Error("API key name too long")
		validationErrs := []errors.ValidationError{
			errors.NewMaxLengthError("name", 255),
		}
		apiErr := errors.NewValidationError("Request validation failed", validationErrs)
		errors.WriteJSONError(w, r, apiErr)
		return false
	}

	return true
}

func (s *Server) validateIntegrationCodes(w http.ResponseWriter, r *http.Request, userID string, codes []string) bool {
	if len(codes) == 0 {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateAPIKey",
			"user_id", userID,
		).
			Warn("No integration codes provided")
		apiErr := errors.NewValidationError(
			"At least one integration must be selected",
			[]errors.ValidationError{{Field: "integration_codes", Message: "required"}},
		)
		errors.WriteJSONError(w, r, apiErr)
		return false
	}

	// Validate each individual integration code
	validCodes := models.ValidIntegrationCodes()
	validCodesMap := make(map[string]bool)
	for _, code := range validCodes {
		validCodesMap[code] = true
	}

	for _, code := range codes {
		if !validCodesMap[code] {
			s.logger.With(
				"service", "vibexp-api",
				"handler", "handleCreateAPIKey",
				"user_id", userID,
				"invalid_code", code,
				"valid_codes", validCodes,
			).Warn("Invalid integration code provided")
			apiErr := errors.NewValidationError(
				"Invalid integration code",
				[]errors.ValidationError{{
					Field:   "integration_codes",
					Message: "invalid integration code: " + code,
				}},
			)
			errors.WriteJSONError(w, r, apiErr)
			return false
		}
	}

	return true
}

func (s *Server) logAPIKeyError(handler, userID, msg string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", handler,
		"user_id", userID,
		"error", fmt.Sprintf("%+v", err),
	).
		Error(msg)
}

func (s *Server) writeAPIKeyResponse(w http.ResponseWriter, apiKey *models.APIKey, fullKey string) {
	response := models.CreateAPIKeyResponse{
		APIKey:    *apiKey,
		FullKey:   fullKey,
		KeyPrefix: apiKey.KeyPrefix,
	}

	writeOK(w, response, s.logger)
}

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleListAPIKeys",
		"user_id", userID,
	).
		Info("API keys list request received")

	apiKeys, err := s.container.APIKeyService().GetAPIKeysByUserID(r.Context(), userID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListAPIKeys",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get API keys")
		apiErr := errors.NewInternalError("Failed to get API keys")
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	writeOK(w, apiKeys, s.logger)
}

func (s *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	apiKeyID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleDeleteAPIKey",
		"user_id", userID,
		"api_key_id", apiKeyID,
	).
		Info("API key deletion request received")

	if apiKeyID == "" {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteAPIKey",
			"user_id", userID,
		).
			Error("API key ID is required")
		validationErrs := []errors.ValidationError{
			errors.NewRequiredFieldError("id"),
		}
		apiErr := errors.NewValidationError("Request validation failed", validationErrs)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	err := s.container.APIKeyService().DeleteAPIKey(r.Context(), userID, apiKeyID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteAPIKey",
			"user_id", userID,
			"api_key_id", apiKeyID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to delete API key")
		if stderrors.Is(err, repositories.ErrAPIKeyNotFound) {
			apiErr := errors.NewResourceNotFoundError("api_key", "API key not found")
			errors.WriteJSONError(w, r, apiErr)
			return
		}
		apiErr := errors.NewInternalError("Failed to delete API key")
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleDeleteAPIKey",
		"user_id", userID,
		"api_key_id", apiKeyID,
	).
		Info("API key deleted successfully")

	// Record activity for API key deletion
	s.recordAPIKeyActivity(
		r.Context(), userID, activities.ActivityTypeAPIKeyDeleted,
		apiKeyID, "Deleted API key: "+apiKeyID, r,
	)

	writeNoContent(w)
}
