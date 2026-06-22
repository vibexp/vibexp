package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
)

func (s *Server) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetPreferences",
		"user_id", userID,
	).Info("User preferences get request received")

	prefs, err := s.container.UserPreferencesService().GetPreferences(r.Context(), userID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetPreferences",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get user preferences")
		errors.WriteJSONError(w, r, errors.NewDatabaseError(
			"Failed to retrieve user preferences. Please try again later.",
		))
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetPreferences",
		"user_id", userID,
	).Info("User preferences retrieved successfully")

	writeOK(w, prefs, s.logger)
}

func (s *Server) handleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdatePreferences",
		"user_id", userID,
	).Info("User preferences update request received")

	var req models.UpdatePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleUpdatePreferences",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to decode request body")
		apiErr := errors.NewBadRequestError(
			fmt.Sprintf("Invalid request body: %s. Please ensure the JSON is well-formed.", err.Error()),
		)
		errors.WriteJSONError(w, r, apiErr)
		return
	}

	prefs, err := s.container.UserPreferencesService().UpdatePreferences(r.Context(), userID, req)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleUpdatePreferences",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to update user preferences")
		errors.WriteJSONError(w, r, errors.NewPreferencesUpdateFailedError(
			"Unable to update preferences. Please check your input and try again.",
		))
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdatePreferences",
		"user_id", userID,
	).Info("User preferences updated successfully")

	writeOK(w, prefs, s.logger)
}
