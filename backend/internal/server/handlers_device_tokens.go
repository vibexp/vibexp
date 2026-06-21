package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"

	apierrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// deviceTokenMaxBodyBytes caps the request body size for device-token endpoints.
const deviceTokenMaxBodyBytes = int64(4096)

// registerDeviceTokenRequest is the request body for POST /api/v1/device-tokens
type registerDeviceTokenRequest struct {
	Token     string `json:"token"`
	Platform  string `json:"platform"`
	UserAgent string `json:"user_agent,omitempty"`
}

// deleteDeviceTokenRequest is the request body for DELETE /api/v1/device-tokens
type deleteDeviceTokenRequest struct {
	Token string `json:"token"`
}

// validPlatforms lists the push notification platforms that this API supports.
// Currently only "web" (Web Push / FCM browser push) is accepted.
var validPlatforms = map[string]bool{
	"web": true,
}

// validateRegisterDeviceTokenRequest validates the fields of a registerDeviceTokenRequest
// and returns a non-nil *apierrors.APIError when any field is invalid.
func validateRegisterDeviceTokenRequest(req *registerDeviceTokenRequest) *apierrors.APIError {
	if req.Token == "" {
		return apierrors.NewBadRequestError("token is required")
	}

	if len(req.Token) > 4096 {
		return apierrors.NewBadRequestError("token too long")
	}

	if req.Platform == "" {
		return apierrors.NewBadRequestError("platform is required")
	}

	if !validPlatforms[req.Platform] {
		return apierrors.NewBadRequestError("invalid platform; must be one of: web")
	}

	return nil
}

// handleRegisterDeviceToken handles POST /api/v1/device-tokens.
// Upserts a push notification device token for the authenticated user.
func (s *Server) handleRegisterDeviceToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(string)
	if !ok || userID == "" {
		apierrors.WriteJSONError(w, r, apierrors.NewAuthRequiredError("Authentication required"))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, deviceTokenMaxBodyBytes)

	var req registerDeviceTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("Invalid request body"))
		return
	}

	req.Token = strings.TrimSpace(req.Token)
	req.Platform = strings.TrimSpace(req.Platform)

	if apiErr := validateRegisterDeviceTokenRequest(&req); apiErr != nil {
		apierrors.WriteJSONError(w, r, apiErr)
		return
	}

	dt := &models.DeviceToken{
		UserID:    userID,
		Token:     req.Token,
		Platform:  req.Platform,
		UserAgent: req.UserAgent,
	}

	if err := s.container.DeviceTokenRepository().Upsert(r.Context(), dt); err != nil {
		if errors.Is(err, repositories.ErrDeviceTokenConflict) {
			conflictErr := apierrors.NewResourceExistsError(
				"device_token", "token is registered to another account",
			)
			apierrors.WriteJSONError(w, r, conflictErr)
			return
		}

		s.logger.WithFields(logrus.Fields{
			"handler":  "handleRegisterDeviceToken",
			"user_id":  userID,
			"platform": req.Platform,
			"error":    err.Error(),
		}).Error("Failed to register device token")

		apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Failed to register device token"))

		return
	}

	w.WriteHeader(http.StatusCreated)
}

// handleDeleteDeviceToken handles DELETE /api/v1/device-tokens.
// Removes a push notification device token for the authenticated user.
func (s *Server) handleDeleteDeviceToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(string)
	if !ok || userID == "" {
		apierrors.WriteJSONError(w, r, apierrors.NewAuthRequiredError("Authentication required"))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, deviceTokenMaxBodyBytes)

	var req deleteDeviceTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("Invalid request body"))
		return
	}

	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		apierrors.WriteJSONError(w, r, apierrors.NewBadRequestError("token is required"))
		return
	}

	if err := s.container.DeviceTokenRepository().Delete(r.Context(), req.Token, userID); err != nil {
		s.logger.WithFields(logrus.Fields{
			"handler": "handleDeleteDeviceToken",
			"user_id": userID,
			"error":   err.Error(),
		}).Error("Failed to delete device token")

		apierrors.WriteJSONError(w, r, apierrors.NewInternalError("Failed to delete device token"))

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
