package server

import (
	"encoding/json"
	stderrors "errors"
	"log/slog"
	"net/http"

	"github.com/go-playground/validator/v10"

	"github.com/vibexp/vibexp/internal/models"
)

var validate = validator.New()

// contactMaxBodyBytes caps the contact-form request body at 64KiB. The form carries
// only a name, email, phone, and short message, so a tighter cap than the global one
// rejects oversized payloads before they are decoded.
const contactMaxBodyBytes int64 = 64 * 1024

// decodeContactRequest caps the body at contactMaxBodyBytes and decodes it, writing the
// appropriate error response (413 for oversized, 400 for malformed) and returning ok=false
// on failure.
func (s *Server) decodeContactRequest(
	w http.ResponseWriter, r *http.Request,
) (models.ContactFormRequest, bool) {
	var req models.ContactFormRequest
	r.Body = http.MaxBytesReader(w, r.Body, contactMaxBodyBytes)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.With("error", err).Error("Failed to decode contact form request")
		var maxBytesErr *http.MaxBytesError
		if stderrors.As(err, &maxBytesErr) {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return req, false
		}
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return req, false
	}
	return req, true
}

func (s *Server) handleContactSendMessage(w http.ResponseWriter, r *http.Request) {
	req, ok := s.decodeContactRequest(w, r)
	if !ok {
		return
	}

	// Validate the request
	if err := validate.Struct(&req); err != nil {
		s.logger.With("error", err).Error("Contact form validation failed")

		// Return validation errors
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		response := models.ContactFormResponse{
			Message: "Validation failed. Please check your input.",
			Success: false,
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
		return
	}

	// Get email service and send emails
	emailService := s.container.EmailService()
	if err := emailService.SendContactMessage(&req); err != nil {
		s.logger.With("error", err).Error("Failed to send contact form emails")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		response := models.ContactFormResponse{
			Message: "Failed to send message. Please try again later.",
			Success: false,
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
		return
	}

	// Log successful submission
	s.logger.With(
		"name", req.Name,
		"email", req.Email,
	).Info("Contact form submitted successfully")

	// Return success response
	response := models.ContactFormResponse{
		Message: "Thank you for your message! We'll get back to you soon.",
		Success: true,
	}
	writeOK(w, response, s.logger)
}
