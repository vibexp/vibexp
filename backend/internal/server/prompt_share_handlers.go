package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
)

// handleCreatePromptShare creates or updates a share for a prompt
func (s *Server) handleCreatePromptShare(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	_ = chi.URLParam(r, "team_id") // Already validated by middleware, not needed for share service
	promptSlug := chi.URLParam(r, "slug")

	var req models.CreateShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, r, "validation_error", "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.ShareType == "" {
		writeErrorResponse(w, r, "validation_error", "share_type is required", http.StatusBadRequest)
		return
	}

	if req.ShareType != "public" && req.ShareType != "restricted" {
		writeErrorResponse(w, r, "validation_error", "share_type must be 'public' or 'restricted'", http.StatusBadRequest)
		return
	}

	if req.ShareType == "restricted" && len(req.Emails) == 0 {
		writeErrorResponse(w, r, "validation_error", "emails are required for restricted shares", http.StatusBadRequest)
		return
	}

	// Create share
	shareResp, err := s.container.PromptShareService().CreateShare(userID, promptSlug, &req)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service":     "vibexp-api",
			"handler":     "handleCreatePromptShare",
			"user_id":     userID,
			"prompt_slug": promptSlug,
			"error":       fmt.Sprintf("%+v", err),
		}).Error("Failed to create prompt share")

		if err.Error() == "prompt not found" {
			writeErrorResponse(w, r, "not_found", "Prompt not found", http.StatusNotFound)
			return
		}

		writeErrorResponse(w, r, "internal_error", "Failed to create share", http.StatusInternalServerError)
		return
	}

	writeOK(w, shareResp, s.logger)
}

// handleGetPromptShare retrieves share details for a prompt
func (s *Server) handleGetPromptShare(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	_ = chi.URLParam(r, "team_id") // Already validated by middleware, not needed for share service
	promptSlug := chi.URLParam(r, "slug")

	shareResp, err := s.container.PromptShareService().GetShare(userID, promptSlug)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service":     "vibexp-api",
			"handler":     "handleGetPromptShare",
			"user_id":     userID,
			"prompt_slug": promptSlug,
			"error":       fmt.Sprintf("%+v", err),
		}).Error("Failed to get prompt share")

		if err.Error() == "prompt not found" {
			writeErrorResponse(w, r, "not_found", "Prompt not found", http.StatusNotFound)
			return
		}

		if err.Error() == "share not found" {
			writeErrorResponse(w, r, "not_found", "Share not found", http.StatusNotFound)
			return
		}

		writeErrorResponse(w, r, "internal_error", "Failed to get share", http.StatusInternalServerError)
		return
	}

	writeOK(w, shareResp, s.logger)
}

// handleDeletePromptShare deletes a share for a prompt
func (s *Server) handleDeletePromptShare(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	_ = chi.URLParam(r, "team_id") // Already validated by middleware, not needed for share service
	promptSlug := chi.URLParam(r, "slug")

	err := s.container.PromptShareService().DeleteShare(userID, promptSlug)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"service":     "vibexp-api",
			"handler":     "handleDeletePromptShare",
			"user_id":     userID,
			"prompt_slug": promptSlug,
			"error":       fmt.Sprintf("%+v", err),
		}).Error("Failed to delete prompt share")

		if err.Error() == "prompt not found" {
			writeErrorResponse(w, r, "not_found", "Prompt not found", http.StatusNotFound)
			return
		}

		if err.Error() == "share not found" {
			writeErrorResponse(w, r, "not_found", "Share not found", http.StatusNotFound)
			return
		}

		writeErrorResponse(w, r, "internal_error", "Failed to delete share", http.StatusInternalServerError)
		return
	}

	writeNoContent(w)
}

// handleSharedPromptError handles error responses for shared prompt retrieval
func (s *Server) handleSharedPromptError(w http.ResponseWriter, r *http.Request, err error, shareToken string) {
	s.logger.WithFields(logrus.Fields{
		"service":     "vibexp-api",
		"handler":     "handleGetSharedPrompt",
		"share_token": shareToken,
		"error":       fmt.Sprintf("%+v", err),
	}).Warn("Failed to get shared prompt")

	errMsg := err.Error()
	switch errMsg {
	case "authentication required":
		apiErr := errors.NewAuthRequiredError("This shared prompt requires authentication")
		errors.WriteJSONError(w, r, apiErr)
	case "access denied":
		writeErrorResponse(w, r, "forbidden", "You do not have access to this shared prompt", http.StatusForbidden)
	case "shared prompt not found", "prompt not found":
		writeErrorResponse(w, r, "not_found", "Shared prompt not found", http.StatusNotFound)
	case "share has been disabled":
		writeErrorResponse(w, r, "forbidden", "This share has been disabled", http.StatusForbidden)
	case "share has expired":
		writeErrorResponse(w, r, "forbidden", "This share has expired", http.StatusForbidden)
	default:
		writeErrorResponse(w, r, "internal_error", "Failed to get shared prompt", http.StatusInternalServerError)
	}
}

// handleGetSharedPrompt retrieves a shared prompt by token (public endpoint)
func (s *Server) handleGetSharedPrompt(w http.ResponseWriter, r *http.Request) {
	shareToken := chi.URLParam(r, "token")

	// Extract user email from context if authenticated
	var userEmail *string
	if userIDVal := r.Context().Value(contextKeyUserID); userIDVal != nil {
		userID := userIDVal.(string)
		// Get user from repository to get email
		user, err := s.container.UserRepository().GetByID(r.Context(), userID)
		if err == nil && user != nil {
			userEmail = &user.Email
		}
	}

	sharedPrompt, err := s.container.PromptShareService().GetSharedPrompt(shareToken, userEmail)
	if err != nil {
		s.handleSharedPromptError(w, r, err, shareToken)
		return
	}

	// Check if request is from a crawler (for SEO/social media previews)
	userAgent := r.Header.Get("User-Agent")
	if isCrawlerUserAgent(userAgent) {
		// Construct URLs dynamically from request
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		host := r.Host
		shareURL := fmt.Sprintf("%s://%s/shared/prompts/%s", scheme, host, shareToken)

		// Use frontend base URL for static assets like logo
		imageURL := fmt.Sprintf("%s/logo_rounded.png", s.config.FrontendBaseURL)

		description := sharedPrompt.Prompt.Description
		if description == "" {
			description = "Shared prompt"
		}

		html := generateSharedPromptHTML(
			sharedPrompt.Prompt.Name,
			description,
			shareURL,
			imageURL,
		)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		// #nosec G705 - HTML content is properly sanitized via html.EscapeString in generateSharedPromptHTML
		if _, err := w.Write([]byte(html)); err != nil {
			s.logger.WithError(err).Error("Failed to write HTML response")
		}
		return
	}

	// For regular API clients, return JSON
	writeOK(w, sharedPrompt, s.logger)
}
