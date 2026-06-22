package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
)

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateProject",
		"user_id", userID,
		"team_id", teamID,
	).Info("Create project request received")

	var req models.CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logProjectError(w, "handleCreateProject", userID, "", err,
			"Failed to decode request body", "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if !s.validateCreateProjectRequest(w, &req) {
		return
	}

	project, err := s.container.ProjectService().CreateProject(userID, teamID, &req)
	if err != nil {
		s.handleCreateProjectError(w, userID, err)
		return
	}

	writeCreated(w, project, s.logger)
}

func (s *Server) handleGetProjectStats(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	slug := chi.URLParam(r, "slug")

	decodedSlug, ok := s.decodeProjectSlug(w, userID, "handleGetProjectStats", slug)
	if !ok {
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetProjectStats",
		"user_id", userID,
		"team_id", teamID,
		"slug", decodedSlug,
	).Info("Get project stats request received")

	stats, err := s.container.ProjectService().GetProjectStats(teamID, userID, decodedSlug)
	if err != nil {
		s.handleGetProjectStatsError(w, userID, decodedSlug, err)
		return
	}

	writeOK(w, stats, s.logger)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	slug := chi.URLParam(r, "slug")

	decodedSlug, ok := s.decodeProjectSlug(w, userID, "handleGetProject", slug)
	if !ok {
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetProject",
		"user_id", userID,
		"team_id", teamID,
		"slug", decodedSlug,
	).Info("Get project request received")

	project, err := s.container.ProjectService().GetProjectBySlug(teamID, userID, decodedSlug)
	if err != nil {
		s.handleGetProjectError(w, userID, decodedSlug, err)
		return
	}

	contextkeys.SetAccessedResourceID(r.Context(), project.ID)

	writeOK(w, project, s.logger)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleListProjects",
		"user_id", userID,
		"team_id", teamID,
	).Info("List projects request received")

	filters := s.buildProjectFilters(r, teamID)

	response, listErr := s.container.ProjectService().ListProjects(userID, filters)
	if listErr != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListProjects",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", listErr),
		).Error("Failed to list projects")

		writeErrorResponse(w, nil, "internal_error", "Failed to list projects", http.StatusInternalServerError)
		return
	}

	// Enrich projects with GitHub connectivity status.
	// A single batch fetch keeps this O(n) per page rather than O(n) GitHub API calls.
	repoURLs, ghErr := s.container.GitHubAppService().GetAccessibleRepoURLs(r.Context(), teamID)
	if ghErr != nil {
		// Non-fatal: log and continue with github_connected defaulting to false
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListProjects",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", ghErr),
		).Warn("Failed to fetch GitHub accessible repositories; defaulting github_connected to false")
		repoURLs = map[string]bool{}
	}

	for i := range response.Projects {
		gitURL := response.Projects[i].GitURL
		if gitURL != "" {
			response.Projects[i].GitHubConnected = repoURLs[normalizeGitURL(gitURL)]
		}
	}

	writeOK(w, response, s.logger)
}

// normalizeGitURL normalizes a git URL for comparison against GitHub repository URLs.
// It lowercases, trims trailing slashes, and removes .git suffix.
func normalizeGitURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u := strings.ToLower(rawURL)
	u = strings.TrimSuffix(u, "/")
	u = strings.TrimSuffix(u, ".git")
	u = strings.TrimSuffix(u, "/")
	return u
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	slug := chi.URLParam(r, "slug")

	decodedSlug, ok := s.decodeProjectSlug(w, userID, "handleUpdateProject", slug)
	if !ok {
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateProject",
		"user_id", userID,
		"team_id", teamID,
		"slug", decodedSlug,
	).Info("Update project request received")

	var req models.UpdateProjectRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		s.logProjectError(w, "handleUpdateProject", userID, decodedSlug, decodeErr,
			"Failed to decode request body", "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if !s.validateUpdateProjectRequest(w, &req) {
		return
	}

	project, err := s.container.ProjectService().UpdateProject(teamID, userID, decodedSlug, &req)
	if err != nil {
		s.handleUpdateProjectError(w, userID, decodedSlug, err)
		return
	}

	writeOK(w, project, s.logger)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	slug := chi.URLParam(r, "slug")

	decodedSlug, ok := s.decodeProjectSlug(w, userID, "handleDeleteProject", slug)
	if !ok {
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleDeleteProject",
		"user_id", userID,
		"team_id", teamID,
		"slug", decodedSlug,
	).Info("Delete project request received")

	err := s.container.ProjectService().DeleteProject(teamID, userID, decodedSlug)
	if err != nil {
		s.handleDeleteProjectError(w, userID, decodedSlug, err)
		return
	}

	writeNoContent(w)
}

// Helper functions for project handlers

// validateCreateProjectRequest validates the create project request
func (s *Server) validateCreateProjectRequest(w http.ResponseWriter, req *models.CreateProjectRequest) bool {
	if req.Name == "" {
		writeErrorResponse(w, nil, "validation_error", "Name is required", http.StatusBadRequest)
		return false
	}
	if req.Slug == "" {
		writeErrorResponse(w, nil, "validation_error", "Slug is required", http.StatusBadRequest)
		return false
	}
	return s.validateProjectFieldLengths(w, req.Name, req.Slug, req.Description, req.GitURL, req.Homepage)
}

// validateUpdateProjectRequest validates the update project request
func (s *Server) validateUpdateProjectRequest(w http.ResponseWriter, req *models.UpdateProjectRequest) bool {
	name := ""
	slug := ""
	description := ""
	gitURL := ""
	homepage := ""

	if req.Name != nil {
		name = *req.Name
	}
	if req.Slug != nil {
		slug = *req.Slug
	}
	if req.Description != nil {
		description = *req.Description
	}
	if req.GitURL != nil {
		gitURL = *req.GitURL
	}
	if req.Homepage != nil {
		homepage = *req.Homepage
	}

	return s.validateProjectFieldLengths(w, name, slug, description, gitURL, homepage)
}

// validateProjectFieldLengths validates field lengths for projects
func (s *Server) validateProjectFieldLengths(
	w http.ResponseWriter, name, slug, description, gitURL, homepage string,
) bool {
	if !s.validateProjectStringLength(w, name, 255, "Name") {
		return false
	}
	if !s.validateProjectStringLength(w, slug, 100, "Slug") {
		return false
	}
	if !s.validateProjectStringLength(w, description, 1000, "Description") {
		return false
	}
	if !s.validateProjectStringLength(w, gitURL, 500, "Git URL") {
		return false
	}
	if !s.validateProjectStringLength(w, homepage, 500, "Homepage") {
		return false
	}
	return true
}

// validateProjectStringLength validates string length
func (s *Server) validateProjectStringLength(w http.ResponseWriter, value string, maxLen int, fieldName string) bool {
	if value != "" && len(value) > maxLen {
		writeErrorResponse(w, nil, "validation_error",
			fieldName+" cannot be longer than "+strconv.Itoa(maxLen)+" characters", http.StatusBadRequest)
		return false
	}
	return true
}

// decodeProjectSlug decodes URL-encoded slug
func (s *Server) decodeProjectSlug(w http.ResponseWriter, userID, handler, slug string) (string, bool) {
	decodedSlug, err := url.QueryUnescape(slug)
	if err != nil {
		s.logProjectError(w, handler, userID, slug, err,
			"Failed to decode slug", "bad_request", "Invalid slug encoding", http.StatusBadRequest)
		return "", false
	}
	return decodedSlug, true
}

// handleCreateProjectError handles errors from project creation
func (s *Server) handleCreateProjectError(w http.ResponseWriter, userID string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateProject",
		"user_id", userID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to create project")

	if strings.Contains(err.Error(), "already exists") {
		writeErrorResponse(w, nil, "conflict", err.Error(), http.StatusConflict)
		return
	}

	// Handle team validation errors
	if strings.Contains(err.Error(), "user is not a member of the specified team") {
		writeErrorResponse(w, nil, "forbidden", err.Error(), http.StatusForbidden)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to create project", http.StatusInternalServerError)
}

// handleGetProjectError handles errors from getting a project
func (s *Server) handleGetProjectError(w http.ResponseWriter, userID, slug string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetProject",
		"user_id", userID,
		"slug", slug,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to get project")

	if strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Project not found", http.StatusNotFound)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to get project", http.StatusInternalServerError)
}

// handleUpdateProjectError handles errors from project update
func (s *Server) handleUpdateProjectError(w http.ResponseWriter, userID, slug string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateProject",
		"user_id", userID,
		"slug", slug,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to update project")

	if strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Project not found", http.StatusNotFound)
		return
	}

	if strings.Contains(err.Error(), "already exists") {
		writeErrorResponse(w, nil, "conflict", err.Error(), http.StatusConflict)
		return
	}

	if strings.Contains(err.Error(), "version mismatch") {
		writeErrorResponse(w, nil, "conflict", "Project was modified by another request", http.StatusConflict)
		return
	}

	// Handle team validation errors
	if strings.Contains(err.Error(), "user is not a member of the specified team") {
		writeErrorResponse(w, nil, "forbidden", err.Error(), http.StatusForbidden)
		return
	}

	if strings.Contains(err.Error(), "resources cannot be moved between teams") {
		writeErrorResponse(w, nil, "bad_request", err.Error(), http.StatusBadRequest)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to update project", http.StatusInternalServerError)
}

// handleDeleteProjectError handles errors from project deletion
func (s *Server) handleDeleteProjectError(w http.ResponseWriter, userID, slug string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleDeleteProject",
		"user_id", userID,
		"slug", slug,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to delete project")

	if strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Project not found", http.StatusNotFound)
		return
	}

	// Handle last project deletion error
	var lastProjectErr *services.CannotDeleteLastProjectError
	if errors.As(err, &lastProjectErr) {
		writeErrorResponse(w, nil, "cannot_delete_last_project", err.Error(), http.StatusBadRequest)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to delete project", http.StatusInternalServerError)
}

// handleGetProjectStatsError handles errors from getting project stats
func (s *Server) handleGetProjectStatsError(w http.ResponseWriter, userID, slug string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetProjectStats",
		"user_id", userID,
		"slug", slug,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to get project stats")

	if strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Project not found", http.StatusNotFound)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to get project stats", http.StatusInternalServerError)
}

// Project list pagination bounds. An out-of-range limit clamps to
// maxProjectLimit rather than silently reverting to defaultProjectLimit.
const (
	defaultProjectLimit = 20
	maxProjectLimit     = 100
)

// buildProjectFilters builds project filters from request query parameters
func (s *Server) buildProjectFilters(r *http.Request, teamID string) services.ProjectFilters {
	filters := services.ProjectFilters{
		Search:    r.URL.Query().Get("search"),
		SortBy:    r.URL.Query().Get("sort_by"),
		SortOrder: r.URL.Query().Get("sort_order"),
		TeamID:    teamID,
		Page:      1,
		Limit:     defaultProjectLimit,
	}

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if page, parseErr := strconv.Atoi(pageStr); parseErr == nil && page > 0 {
			filters.Page = page
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, parseErr := strconv.Atoi(limitStr); parseErr == nil && limit > 0 {
			filters.Limit = min(limit, maxProjectLimit)
		}
	}

	return filters
}

// logProjectError logs a project error and writes error response
func (s *Server) logProjectError(w http.ResponseWriter, handler, userID, slug string,
	err error, logMsg, errCode, errMsg string, statusCode int) {
	fields := []any{
		"service", "vibexp-api",
		"handler", handler,
		"user_id", userID,
		"error", fmt.Sprintf("%+v", err),
	}
	if slug != "" {
		fields = append(fields, "slug", slug)
	}
	s.logger.With(fields...).Error(logMsg)
	writeErrorResponse(w, nil, errCode, errMsg, statusCode)
}
