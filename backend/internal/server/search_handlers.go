package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/models"
)

// handleSearch performs a team-scoped semantic search across the team's prompts,
// artifacts, blueprints and memories.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	var req models.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, r, "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := validate.Struct(&req); err != nil {
		writeErrorResponse(w, r, "validation_error", "Validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	pagination := validatePaginationParams(strconv.Itoa(req.Page), strconv.Itoa(req.PerPage))
	req.Page = pagination.Page
	req.PerPage = pagination.Limit

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleSearch",
		"user_id", userID,
		"team_id", teamID,
		"types", req.Types,
		"project_id", req.ProjectID,
		"page", req.Page,
		"per_page", req.PerPage,
	).Info("Semantic search request received")

	response, err := s.container.SearchService().Search(r.Context(), teamID, &req)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleSearch",
			"user_id", userID,
			"team_id", teamID,
			"error", err.Error(),
		).Error("Failed to perform semantic search")
		writeErrorResponse(w, r, "internal_error", "Failed to perform search", http.StatusInternalServerError)
		return
	}

	writeOK(w, response, s.logger)
}
