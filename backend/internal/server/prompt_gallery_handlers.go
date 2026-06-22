package server

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Input validation constants
const (
	maxSearchQueryLength  = 255
	maxCategoryNameLength = 100
	maxTagsCount          = 10
)

// uuidRegex matches UUID format (including gallery- prefix format)
var uuidRegex = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$|^gallery-[0-9]+$`)

// validatePromptID validates that the prompt ID is in a valid format
func validatePromptID(promptID string) bool {
	if promptID == "" {
		return false
	}
	return uuidRegex.MatchString(promptID)
}

// sanitizeQueryParam sanitizes and validates query parameters
func sanitizeQueryParam(param string, maxLength int) string {
	// Trim whitespace
	param = strings.TrimSpace(param)

	// Enforce max length
	if len(param) > maxLength {
		param = param[:maxLength]
	}

	return param
}

// handleGetPromptGalleryCategories handles GET /api/v1/prompt-gallery/categories
func (s *Server) handleGetPromptGalleryCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := s.container.PromptGalleryService().GetCategories()
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetPromptGalleryCategories",
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get prompt gallery categories")
		writeErrorResponse(w, nil, "internal_error", "Failed to get categories", http.StatusInternalServerError)
		return
	}

	writeOK(w, categories, s.logger)
}

// handleListPromptGalleryPrompts handles GET /api/v1/prompt-gallery/prompts
func (s *Server) handleListPromptGalleryPrompts(w http.ResponseWriter, r *http.Request) {
	// Sanitize and validate query parameters
	category := sanitizeQueryParam(r.URL.Query().Get("category"), maxCategoryNameLength)
	search := sanitizeQueryParam(r.URL.Query().Get("search"), maxSearchQueryLength)

	// Parse tags from query parameter (comma-separated)
	var tags []string
	if tagsParam := r.URL.Query().Get("tags"); tagsParam != "" {
		tags = strings.Split(tagsParam, ",")
		// Trim whitespace from each tag
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		// Limit number of tags to prevent abuse
		if len(tags) > maxTagsCount {
			tags = tags[:maxTagsCount]
		}
	}

	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit < 1 {
		limit = 20
	}
	// Enforce maximum limit to prevent resource exhaustion
	if limit > 100 {
		limit = 100
	}

	prompts, err := s.container.PromptGalleryService().ListPrompts(category, search, tags, page, limit)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListPromptGalleryPrompts",
			"category", category,
			"search", search,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list prompt gallery prompts")
		writeErrorResponse(w, nil, "internal_error", "Failed to list prompts", http.StatusInternalServerError)
		return
	}

	writeOK(w, prompts, s.logger)
}

// handleGetPromptGalleryPrompt handles GET /api/v1/prompt-gallery/prompts/{id}
func (s *Server) handleGetPromptGalleryPrompt(w http.ResponseWriter, r *http.Request) {
	promptID := chi.URLParam(r, "id")
	if promptID == "" {
		writeErrorResponse(w, nil, "validation_error", "Prompt ID is required", http.StatusBadRequest)
		return
	}

	// Validate UUID format
	if !validatePromptID(promptID) {
		writeErrorResponse(w, nil, "validation_error", "Invalid prompt ID format", http.StatusBadRequest)
		return
	}

	prompt, err := s.container.PromptGalleryService().GetPromptByID(promptID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetPromptGalleryPrompt",
			"prompt_id", promptID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get prompt gallery prompt")

		if errors.Is(err, repositories.ErrPromptNotFound) {
			writeErrorResponse(w, nil, "not_found", "Prompt not found", http.StatusNotFound)
		} else {
			writeErrorResponse(w, nil, "internal_error", "Failed to get prompt", http.StatusInternalServerError)
		}
		return
	}

	writeOK(w, prompt, s.logger)
}

// handleTrackPromptGalleryUsage handles POST /api/v1/prompt-gallery/prompts/{id}/use
func (s *Server) handleTrackPromptGalleryUsage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(string)
	if !ok {
		writeErrorResponse(w, nil, "unauthorized", "User not authenticated", http.StatusUnauthorized)
		return
	}

	promptID := chi.URLParam(r, "id")
	if promptID == "" {
		writeErrorResponse(w, nil, "validation_error", "Prompt ID is required", http.StatusBadRequest)
		return
	}

	// Validate UUID format
	if !validatePromptID(promptID) {
		writeErrorResponse(w, nil, "validation_error", "Invalid prompt ID format", http.StatusBadRequest)
		return
	}

	req := &models.PromptGalleryUsageRequest{
		PromptID: promptID,
	}

	if err := s.container.PromptGalleryService().TrackPromptUsage(userID, req); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleTrackPromptGalleryUsage",
			"user_id", userID,
			"prompt_id", promptID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to track prompt gallery usage")

		if errors.Is(err, repositories.ErrPromptNotFound) {
			writeErrorResponse(w, nil, "not_found", "Prompt not found", http.StatusNotFound)
		} else {
			writeErrorResponse(w, nil, "internal_error", "Failed to track usage", http.StatusInternalServerError)
		}
		return
	}

	writeOK(w, map[string]string{"message": "Usage tracked successfully"}, s.logger)
}
