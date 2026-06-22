package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-playground/validator/v10"

	vibexperrors "github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
)

func (s *Server) handleSupportMessage(w http.ResponseWriter, r *http.Request) {
	userID, userEmail := s.extractUserContext(r)
	s.logSupportRequest(userID, userEmail)

	var req models.SupportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(
			w, r,
			vibexperrors.NewBadRequestError("Invalid request body"),
			"Failed to decode support request",
		)
		return
	}

	if validationErrors := s.validateSupportRequest(&req); validationErrors != nil {
		s.writeErrorResponse(
			w, r,
			vibexperrors.NewValidationError("Request validation failed", validationErrors),
			"Support request validation failed",
		)
		return
	}

	userName, err := s.getUserName(r.Context(), userID, userEmail)
	if err != nil {
		s.writeErrorResponse(
			w, r,
			vibexperrors.NewInternalError("Failed to retrieve user information"),
			"Failed to get user details",
		)
		return
	}

	if err := s.sendSupportEmail(userName, userEmail, &req); err != nil {
		s.writeErrorResponse(
			w, r,
			vibexperrors.NewInternalError("Failed to send support request. Please try again later."),
			"Failed to send support request",
		)
		return
	}

	s.writeSuccessResponse(w, userID, &req)
}

// extractUserContext extracts user ID and email from request context
func (s *Server) extractUserContext(r *http.Request) (userID, userEmail string) {
	return r.Context().Value(contextKeyUserID).(string), r.Context().Value(contextKeyUserEmail).(string)
}

// logSupportRequest logs incoming support request
func (s *Server) logSupportRequest(userID, userEmail string) {
	s.logger.With(
		"user_id", userID,
		"email", userEmail,
	).Info("Support message request received")
}

// validateSupportRequest validates the support request and returns validation errors if any
func (s *Server) validateSupportRequest(req *models.SupportRequest) []vibexperrors.ValidationError {
	if err := validate.Struct(req); err != nil {
		s.logger.With("error", err).Error("Support request validation failed")
		var validationErrors []vibexperrors.ValidationError
		if validationErrs, ok := err.(validator.ValidationErrors); ok {
			for _, fieldErr := range validationErrs {
				validationErrors = append(validationErrors, vibexperrors.ValidationError{
					Field:      fieldErr.Field(),
					Message:    getValidationErrorMessage(fieldErr),
					Code:       getValidationErrorCode(fieldErr.Tag()),
					Constraint: fieldErr.Tag(),
				})
			}
		}
		return validationErrors
	}
	return nil
}

// getUserName retrieves the user's name, falling back to email if name is empty
func (s *Server) getUserName(ctx context.Context, userID, userEmail string) (string, error) {
	user, err := s.container.AuthService().GetUserByID(ctx, userID)
	if err != nil {
		return "", err
	}
	userName := user.Name
	if userName == "" {
		userName = userEmail
	}
	return userName, nil
}

// sendSupportEmail sends the support request via email service
func (s *Server) sendSupportEmail(userName, userEmail string, req *models.SupportRequest) error {
	emailService := s.container.EmailService()
	return emailService.SendSupportRequest(userName, userEmail, req)
}

// writeErrorResponse writes an error response to the client
func (s *Server) writeErrorResponse(
	w http.ResponseWriter,
	r *http.Request,
	apiErr *vibexperrors.APIError,
	logMsg string,
) {
	s.logger.With("error", apiErr.Detail).Error(logMsg)
	vibexperrors.WriteJSONError(w, r, apiErr)
}

// writeSuccessResponse writes a successful response to the client
func (s *Server) writeSuccessResponse(w http.ResponseWriter, userID string, req *models.SupportRequest) {
	s.logger.With(
		"user_id", userID,
		"acknowledgement", req.Acknowledgement,
	).Info("Support request sent successfully")

	response := map[string]interface{}{
		"message": "Thank you for your message! We'll get back to you soon.",
		"success": true,
	}
	writeOK(w, response, s.logger)
}

// getValidationErrorMessage returns a human-readable error message for a validation error
func getValidationErrorMessage(fieldErr validator.FieldError) string {
	switch fieldErr.Tag() {
	case "required":
		return fieldErr.Field() + " is required"
	case "min":
		return fieldErr.Field() + " must be at least " + fieldErr.Param() + " characters"
	case "max":
		return fieldErr.Field() + " must be at most " + fieldErr.Param() + " characters"
	case "email":
		return fieldErr.Field() + " must be a valid email address"
	default:
		return fieldErr.Field() + " is invalid"
	}
}

// getValidationErrorCode returns an error code for a validation tag
func getValidationErrorCode(tag string) string {
	switch tag {
	case "required":
		return "REQUIRED_FIELD_MISSING"
	case "min":
		return "FIELD_TOO_SHORT"
	case "max":
		return "FIELD_TOO_LONG"
	case "email":
		return "INVALID_EMAIL_FORMAT"
	default:
		return "VALIDATION_ERROR"
	}
}
