package server

import (
	"encoding/json"
	"net/http"

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

	page, perPage, err := normalizeSearchPagination(req.Page, req.PerPage, "per_page")
	if err != nil {
		writeErrorResponse(w, r, "validation_error", errorMessage(err), http.StatusBadRequest)
		return
	}
	req.Page = page
	req.PerPage = perPage

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

	writeOK(w, searchResultsRESTResponse{
		SearchResultsResponse: response,
		AISummary:             s.container.AISummaryAvailability().Availability(r.Context(), teamID),
	}, s.logger)
}

// searchResultsRESTResponse is the REST search payload: the shared search
// response plus the ai_summary availability flag (#1074).
//
// The flag is added HERE, not on models.SearchResultsResponse, because
// SearchService.Search is the shared choke point for REST, MCP and CLI, and a
// UI affordance flag must not leak into those other surfaces. Availability
// fails open to available=false, so it can never fail the search itself.
type searchResultsRESTResponse struct {
	*models.SearchResultsResponse
	AISummary models.AISummaryAvailability `json:"ai_summary"`
}
