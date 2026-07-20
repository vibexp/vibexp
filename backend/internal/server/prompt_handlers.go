package server

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/errors"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

const (
	// serverLogServiceName tags prompt handler log entries.

	promptMsgGetFailed = "Failed to get prompt"
	promptMsgNotFound  = "Prompt not found"
)

// writeErrorResponse is a helper function for backward compatibility
// It maps old error types to new RFC 9457 compliant errors
func writeErrorResponse(w http.ResponseWriter, r *http.Request, errorType, message string, statusCode int) {
	var apiErr *errors.APIError

	// Map old error types to new error codes
	switch errorType {
	case "validation_error":
		apiErr = errors.NewBadRequestError(message)
		apiErr.Code = errors.CodeValidationFailed
	case "bad_request":
		apiErr = errors.NewBadRequestError(message)
	case "not_found":
		apiErr = errors.NewResourceNotFoundError("resource", message)
	case "conflict":
		apiErr = errors.NewResourceExistsError("resource", message)
	case "internal_error":
		apiErr = errors.NewInternalError(message)
	case "resource_limit_exceeded":
		apiErr = errors.NewResourceLimitExceededError(message)
	case "forbidden":
		apiErr = errors.NewForbiddenError(message)
	case "unauthorized":
		apiErr = errors.NewAuthInvalidError(message)
	default:
		apiErr = errors.NewAPIError(errorType, errorType, message, statusCode)
	}

	// If request is nil, create a dummy request for the error response
	if r == nil {
		r = &http.Request{URL: &url.URL{Path: "/unknown"}}
	}

	errors.WriteJSONError(w, r, apiErr)
}

func validateCreatePromptRequest(req *models.CreatePromptRequest, w http.ResponseWriter) bool {
	if req.Name == "" {
		writeErrorResponse(w, nil, "validation_error", "Name is required", http.StatusBadRequest)
		return false
	}
	if req.Slug == "" {
		writeErrorResponse(w, nil, "validation_error", "Slug is required", http.StatusBadRequest)
		return false
	}
	if req.Body == "" {
		writeErrorResponse(w, nil, "validation_error", "Body is required", http.StatusBadRequest)
		return false
	}
	if req.ProjectID == "" {
		writeErrorResponse(w, nil, "validation_error", "project_id is required", http.StatusBadRequest)
		return false
	}
	if len(req.Description) > 200 {
		writeErrorResponse(
			w, nil, "validation_error",
			"Description cannot be longer than 200 characters",
			http.StatusBadRequest,
		)
		return false
	}
	if len(req.Name) > 50 {
		writeErrorResponse(w, nil, "validation_error", "Name cannot be longer than 50 characters", http.StatusBadRequest)
		return false
	}
	if len(req.Slug) > 255 {
		writeErrorResponse(w, nil, "validation_error", "Slug cannot be longer than 255 characters", http.StatusBadRequest)
		return false
	}
	if req.Status != "" && req.Status != "draft" && req.Status != "published" {
		writeErrorResponse(w, nil, "validation_error", "Status must be either 'draft' or 'published'", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) checkPromptResourceLimit(w http.ResponseWriter, r *http.Request, userID string) bool {
	allowed, err := s.container.ResourceUsageService().CheckResourceLimit(r.Context(), userID, "prompt")
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleCreatePrompt",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to check resource limit")
		writeErrorResponse(w, nil, "internal_error", "Failed to check resource limit", http.StatusInternalServerError)
		return false
	}

	if !allowed {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleCreatePrompt",
			"user_id", userID,
			"resource_type", "prompt",
		).Warn("User has reached their prompt limit")
		writeErrorResponse(
			w, nil, "resource_limit_exceeded",
			"You have reached the maximum number of prompts allowed for your subscription plan",
			http.StatusForbidden,
		)
		return false
	}
	return true
}

func (s *Server) createPromptWithErrorHandling(
	w http.ResponseWriter,
	userID, teamID string,
	req *models.CreatePromptRequest,
) (*models.Prompt, bool) {
	prompt, err := s.container.PromptService().CreatePrompt(userID, teamID, req)
	if err != nil {
		// Denials are benign client errors: handled before the ERROR log and
		// before the string-matching below, which ErrPermissionDenied's text
		// matches nowhere — it would otherwise fall through to a 500.
		if stderrors.Is(err, services.ErrPermissionDenied) {
			s.logger.With("service", serverLogServiceName, "handler", "handleCreatePrompt", "user_id", userID).
				Warn("Forbidden prompt write attempt")
			writeErrorResponse(
				w, nil, "forbidden",
				"You do not have permission to create prompts in this team", http.StatusForbidden,
			)
			return nil, false
		}

		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleCreatePrompt",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create prompt")

		if err.Error() == "prompt with slug '"+req.Slug+"' already exists for this user" {
			writeErrorResponse(w, nil, "conflict", "A prompt with this slug already exists", http.StatusConflict)
			return nil, false
		}

		// Handle project validation errors
		if strings.Contains(err.Error(), "project not found") ||
			strings.Contains(err.Error(), "project does not belong to user") ||
			strings.Contains(err.Error(), "project_id is required") {
			writeErrorResponse(w, nil, "validation_error", "Invalid or inaccessible project", http.StatusBadRequest)
			return nil, false
		}

		// Handle team validation errors
		if strings.Contains(err.Error(), "user is not a member of the specified team") {
			writeErrorResponse(w, nil, "forbidden", "Access denied", http.StatusForbidden)
			return nil, false
		}
		if strings.Contains(err.Error(), "resources cannot be moved between teams") {
			writeErrorResponse(w, nil, "bad_request", "Resources cannot be moved between teams", http.StatusBadRequest)
			return nil, false
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to create prompt", http.StatusInternalServerError)
		return nil, false
	}
	return prompt, true
}

func (s *Server) handleCreatePrompt(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleCreatePrompt",
		"user_id", userID,
		"team_id", teamID,
	).Info("Create prompt request received")

	var req models.CreatePromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleCreatePrompt",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error(msgDecodeRequestBodyFailed)
		writeErrorResponse(w, nil, "bad_request", msgInvalidRequestBody, http.StatusBadRequest)
		return
	}

	if !validateCreatePromptRequest(&req, w) || !s.checkPromptResourceLimit(w, r, userID) {
		return
	}

	prompt, ok := s.createPromptWithErrorHandling(w, userID, teamID, &req)
	if !ok {
		return
	}

	s.recordPromptActivity(
		r.Context(), userID, activities.ActivityTypePromptCreated,
		prompt.ID, req.Slug, "Created new prompt: "+req.Name, r,
	)
	if s.metrics != nil {
		s.metrics.RecordPromptCreated(r.Context())
	}
	writeCreated(w, prompt, s.logger)
}

func (s *Server) handleGetPrompt(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	promptSlug := chi.URLParam(r, "slug")

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleGetPrompt",
		"user_id", userID,
		"team_id", teamID,
		"prompt_slug", promptSlug,
	).Info("Get prompt request received")

	prompt, err := s.container.PromptService().GetPromptBySlug(userID, teamID, promptSlug)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleGetPrompt",
			"user_id", userID,
			"prompt_slug", promptSlug,
			"error", fmt.Sprintf("%+v", err),
		).Error(promptMsgGetFailed)

		if stderrors.Is(err, repositories.ErrPromptNotFound) {
			writeErrorResponse(w, nil, "not_found", promptMsgNotFound, http.StatusNotFound)
			return
		}

		writeErrorResponse(w, nil, "internal_error", promptMsgGetFailed, http.StatusInternalServerError)
		return
	}

	contextkeys.SetAccessedResourceID(r.Context(), prompt.ID)

	prompt.Related = s.relatedForResource(
		r.Context(), userID, teamID, models.RelationResourceTypePrompt, prompt.ID,
	)

	writeOK(w, prompt, s.logger)
}

// parseBoolParam parses a boolean query parameter
func parseBoolParam(query string) *bool {
	if query != "" {
		if parsed, err := strconv.ParseBool(query); err == nil {
			return &parsed
		}
	}
	return nil
}

// parseIntParam parses an integer query parameter with validation
//
//nolint:unused // Kept for potential future use with other query parameters
func parseIntParam(query string, min, max int) int {
	if query == "" {
		return 0
	}
	val, err := strconv.Atoi(query)
	if err != nil || val < min {
		return 0
	}
	if max > 0 && val > max {
		return 0
	}
	return val
}

// allowedPromptSortFields contains the allowlisted sort fields for prompts
var allowedPromptSortFields = map[string]bool{
	"name": true, "status": true, "updated_at": true, "created_at": true,
}

// parsePromptFilters parses query parameters into PromptFilters
func parsePromptFilters(w http.ResponseWriter, r *http.Request, userID, teamID string) (services.PromptFilters, bool) {
	query := r.URL.Query()

	sortBy := query.Get("sort_by")
	if sortBy != "" && !allowedPromptSortFields[sortBy] {
		writeErrorResponse(w, nil, "validation_error", "invalid sort_by value: "+sortBy, http.StatusBadRequest)
		return services.PromptFilters{}, false
	}

	filters := services.PromptFilters{
		Status:    query.Get("status"),
		Search:    query.Get("search"),
		UserID:    userID,
		TeamID:    teamID,
		SortBy:    sortBy,
		SortOrder: strings.ToLower(query.Get("sort_order")),
		Page:      1,
		Limit:     20,
	}

	if labelsStr := query.Get("labels"); labelsStr != "" {
		filters.Labels = strings.Split(labelsStr, ",")
	}

	filters.MCPExpose = parseBoolParam(query.Get("mcp_expose"))
	filters.IsShared = parseBoolParam(query.Get("shared"))

	if projectID := query.Get("project_id"); projectID != "" {
		filters.ProjectID = &projectID
	}

	// Parse and validate pagination parameters with bounds checking
	pagination := validatePaginationParams(query.Get("page"), query.Get("limit"))
	filters.Page = pagination.Page
	filters.Limit = pagination.Limit

	return filters, true
}

func (s *Server) handleListPrompts(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleListPrompts",
		"user_id", userID,
		"team_id", teamID,
	).Info("List prompts request received")

	filters, ok := parsePromptFilters(w, r, userID, teamID)
	if !ok {
		return
	}

	response, err := s.container.PromptService().ListPrompts(userID, filters)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleListPrompts",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list prompts")

		writeErrorResponse(w, nil, "internal_error", "Failed to list prompts", http.StatusInternalServerError)
		return
	}

	writeOK(w, map[string]interface{}{
		"status":  "success",
		"message": "Prompts retrieved successfully",
		"data":    response,
	}, s.logger)
}

func (s *Server) handleGetPromptLabels(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleGetPromptLabels",
		"user_id", userID,
		"team_id", teamID,
	).Info("Get prompt labels request received")

	// Note: GetUserLabels doesn't require teamID - it queries across user's prompts
	labels, err := s.container.PromptService().GetUserLabels(userID)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleGetPromptLabels",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get prompt labels")

		writeErrorResponse(w, nil, "internal_error", "Failed to get prompt labels", http.StatusInternalServerError)
		return
	}

	writeOK(w, map[string]interface{}{
		"status":  "success",
		"message": "Labels retrieved successfully",
		"data": map[string]interface{}{
			"labels": labels,
		},
	}, s.logger)
}

func (s *Server) validateUpdatePromptRequest(req *models.UpdatePromptRequest, w http.ResponseWriter) bool {
	if req.Name != nil && len(*req.Name) > 50 {
		writeErrorResponse(w, nil, "validation_error", "Name cannot be longer than 50 characters", http.StatusBadRequest)
		return false
	}

	if req.Slug != nil && len(*req.Slug) > 255 {
		writeErrorResponse(w, nil, "validation_error", "Slug cannot be longer than 255 characters", http.StatusBadRequest)
		return false
	}

	if req.Description != nil && len(*req.Description) > 200 {
		writeErrorResponse(
			w, nil, "validation_error",
			"Description cannot be longer than 200 characters",
			http.StatusBadRequest,
		)
		return false
	}

	if req.Status != nil && *req.Status != "draft" && *req.Status != "published" {
		writeErrorResponse(w, nil, "validation_error", "Status must be either 'draft' or 'published'", http.StatusBadRequest)
		return false
	}

	return true
}

func (s *Server) handleUpdatePromptError(err error, req *models.UpdatePromptRequest, w http.ResponseWriter) {
	// Denial before the string-matching chain (see handleCreatePrompt).
	if stderrors.Is(err, services.ErrPermissionDenied) {
		s.logger.With("service", serverLogServiceName, "handler", "handleUpdatePrompt").
			Warn("Forbidden prompt write attempt")
		writeErrorResponse(
			w, nil, "forbidden",
			"You do not have permission to update this prompt", http.StatusForbidden,
		)
		return
	}

	if stderrors.Is(err, repositories.ErrPromptNotFound) {
		writeErrorResponse(w, nil, "not_found", promptMsgNotFound, http.StatusNotFound)
		return
	}

	if strings.Contains(err.Error(), "version mismatch") {
		writeErrorResponse(
			w, nil, "conflict",
			"The prompt was modified by another process. Please refresh and try again.",
			http.StatusConflict,
		)
		return
	}

	if req.Slug != nil && err.Error() == "prompt with slug '"+*req.Slug+"' already exists for this user" {
		writeErrorResponse(w, nil, "conflict", "A prompt with this slug already exists", http.StatusConflict)
		return
	}

	// Handle project validation errors
	if strings.Contains(err.Error(), "project not found") ||
		strings.Contains(err.Error(), "project does not belong to user") {
		writeErrorResponse(w, nil, "validation_error", "Invalid or inaccessible project", http.StatusBadRequest)
		return
	}

	// Handle team validation errors
	if strings.Contains(err.Error(), "user is not a member of the specified team") {
		writeErrorResponse(w, nil, "forbidden", "Access denied", http.StatusForbidden)
		return
	}
	if strings.Contains(err.Error(), "resources cannot be moved between teams") {
		writeErrorResponse(w, nil, "bad_request", "Resources cannot be moved between teams", http.StatusBadRequest)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to update prompt", http.StatusInternalServerError)
}

//nolint:funlen // Handler function with comprehensive validation and error handling
func (s *Server) handleUpdatePrompt(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	promptSlug := chi.URLParam(r, "slug")

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleUpdatePrompt",
		"user_id", userID,
		"team_id", teamID,
		"prompt_slug", promptSlug,
	).Info("Update prompt request received")

	// Check resource limit before allowing update
	if !s.checkPromptResourceLimit(w, r, userID) {
		return
	}

	var req models.UpdatePromptRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleUpdatePrompt",
			"user_id", userID,
			"prompt_slug", promptSlug,
			"error", fmt.Sprintf("%+v", decodeErr),
		).Error(msgDecodeRequestBodyFailed)
		writeErrorResponse(w, nil, "bad_request", msgInvalidRequestBody, http.StatusBadRequest)
		return
	}

	if !s.validateUpdatePromptRequest(&req, w) {
		return
	}

	prompt, err := s.container.PromptService().UpdatePromptBySlug(userID, teamID, promptSlug, &req)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleUpdatePrompt",
			"user_id", userID,
			"prompt_slug", promptSlug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to update prompt")

		s.handleUpdatePromptError(err, &req, w)
		return
	}

	// Record activity for prompt update
	s.recordPromptActivity(
		r.Context(), userID, activities.ActivityTypePromptUpdated,
		prompt.ID, promptSlug, "Updated prompt: "+promptSlug, r,
	)

	writeOK(w, prompt, s.logger)
}

// handleListPromptVersions returns the content-version history (newest-first) for a prompt.
func (s *Server) handleListPromptVersions(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	promptSlug := chi.URLParam(r, "slug")

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleListPromptVersions",
		"user_id", userID,
		"team_id", teamID,
		"prompt_slug", promptSlug,
	).Info("List prompt versions request received")

	versions, err := s.container.PromptService().ListPromptVersions(userID, teamID, promptSlug)
	if err != nil {
		s.handlePromptVersionError(w, userID, promptSlug, err)
		return
	}

	writeOK(w, models.PromptVersionListResponse{Versions: versions}, s.logger)
}

// handleGetPromptVersion returns a single content-version snapshot of a prompt.
func (s *Server) handleGetPromptVersion(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	promptSlug := chi.URLParam(r, "slug")

	versionNumber, ok := s.parseVersionNumber(w, chi.URLParam(r, "version_number"))
	if !ok {
		return
	}

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleGetPromptVersion",
		"user_id", userID,
		"team_id", teamID,
		"prompt_slug", promptSlug,
		"version_number", versionNumber,
	).Info("Get prompt version request received")

	version, err := s.container.PromptService().GetPromptVersion(userID, teamID, promptSlug, versionNumber)
	if err != nil {
		s.handlePromptVersionError(w, userID, promptSlug, err)
		return
	}

	writeOK(w, version, s.logger)
}

// handleRestorePromptVersion restores a prompt's raw Body template to the given version. The
// pre-restore body is snapshotted as a new (system-authored) version.
func (s *Server) handleRestorePromptVersion(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	promptSlug := chi.URLParam(r, "slug")

	versionNumber, ok := s.parseVersionNumber(w, chi.URLParam(r, "version_number"))
	if !ok {
		return
	}

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleRestorePromptVersion",
		"user_id", userID,
		"team_id", teamID,
		"prompt_slug", promptSlug,
		"version_number", versionNumber,
	).Info("Restore prompt version request received")

	prompt, err := s.container.PromptService().RestorePromptVersion(userID, teamID, promptSlug, versionNumber)
	if err != nil {
		s.handlePromptVersionError(w, userID, promptSlug, err)
		return
	}

	s.recordPromptActivity(
		r.Context(), userID, activities.ActivityTypePromptUpdated,
		prompt.ID, promptSlug, "Restored prompt: "+promptSlug, r,
	)

	writeOK(w, prompt, s.logger)
}

// handlePromptVersionError maps content-version lookup errors to HTTP responses,
// distinguishing a missing prompt/version (404) from other failures (500).
func (s *Server) handlePromptVersionError(w http.ResponseWriter, userID, promptSlug string, err error) {
	s.logger.With(
		"service", serverLogServiceName,
		"handler", "promptVersion",
		"user_id", userID,
		"prompt_slug", promptSlug,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to process prompt version request")

	if stderrors.Is(err, repositories.ErrPromptNotFound) || strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Not found", http.StatusNotFound)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to process prompt version request",
		http.StatusInternalServerError)
}

func (s *Server) handleDeletePrompt(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	promptSlug := chi.URLParam(r, "slug")

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleDeletePrompt",
		"user_id", userID,
		"team_id", teamID,
		"prompt_slug", promptSlug,
	).Info("Delete prompt request received")

	prompt, err := s.getPromptForDeletion(w, userID, teamID, promptSlug)
	if err != nil {
		return
	}

	if err := s.deletePromptAndEmbeddings(w, userID, teamID, promptSlug, prompt); err != nil {
		return
	}

	s.recordPromptActivity(
		r.Context(), userID, activities.ActivityTypePromptDeleted,
		prompt.ID, promptSlug, "Deleted prompt: "+promptSlug, r,
	)

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordPromptDeleted(r.Context())
	}

	writeNoContent(w)
}

func (s *Server) handleRenderPrompt(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	promptSlug := chi.URLParam(r, "slug")

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleRenderPrompt",
		"user_id", userID,
		"team_id", teamID,
		"prompt_slug", promptSlug,
	).Info("Render prompt request received")

	var req models.RenderPromptRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleRenderPrompt",
			"user_id", userID,
			"prompt_slug", promptSlug,
			"error", fmt.Sprintf("%+v", decodeErr),
		).Error(msgDecodeRequestBodyFailed)
		writeErrorResponse(w, nil, "bad_request", msgInvalidRequestBody, http.StatusBadRequest)
		return
	}

	renderedPrompt, err := s.container.PromptService().RenderPrompt(userID, teamID, promptSlug, req.Placeholders)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleRenderPrompt",
			"user_id", userID,
			"prompt_slug", promptSlug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to render prompt")

		if stderrors.Is(err, repositories.ErrPromptNotFound) {
			writeErrorResponse(w, nil, "not_found", promptMsgNotFound, http.StatusNotFound)
			return
		}

		writeErrorResponse(
			w, nil, "render_error",
			"Failed to render prompt. Please check the provided placeholders.",
			http.StatusBadRequest,
		)
		return
	}

	writeOK(w, renderedPrompt, s.logger)
}

func (s *Server) handleGetPromptPlaceholders(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	promptSlug := chi.URLParam(r, "slug")

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleGetPromptPlaceholders",
		"user_id", userID,
		"team_id", teamID,
		"prompt_slug", promptSlug,
	).Info("Get prompt placeholders request received")

	placeholders, err := s.container.PromptService().GetPromptPlaceholders(userID, teamID, promptSlug)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleGetPromptPlaceholders",
			"user_id", userID,
			"prompt_slug", promptSlug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get prompt placeholders")

		if stderrors.Is(err, repositories.ErrPromptNotFound) {
			writeErrorResponse(w, nil, "not_found", promptMsgNotFound, http.StatusNotFound)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to get prompt placeholders", http.StatusInternalServerError)
		return
	}

	// The OpenAPI contract declares placeholders as a required, non-nullable
	// array. ExtractAllPlaceholders returns a nil slice when a prompt has no
	// placeholders, which marshals to JSON null and crashes clients that trust
	// the generated types (issue #121). Coerce to an empty array so the wire
	// shape always honors the spec.
	if placeholders == nil {
		placeholders = []string{}
	}

	response := map[string]interface{}{
		"placeholders": placeholders,
	}

	writeOK(w, response, s.logger)
}

func (s *Server) getPromptForDeletion(
	w http.ResponseWriter, userID, teamID, promptSlug string,
) (*models.Prompt, error) {
	prompt, err := s.container.PromptService().GetPromptBySlug(userID, teamID, promptSlug)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleDeletePrompt",
			"user_id", userID,
			"prompt_slug", promptSlug,
			"error", fmt.Sprintf("%+v", err),
		).Error(promptMsgGetFailed)

		if stderrors.Is(err, repositories.ErrPromptNotFound) {
			writeErrorResponse(w, nil, "not_found", promptMsgNotFound, http.StatusNotFound)
			return nil, err
		}

		writeErrorResponse(w, nil, "internal_error", promptMsgGetFailed, http.StatusInternalServerError)
		return nil, err
	}

	return prompt, nil
}

func (s *Server) deletePromptAndEmbeddings(
	w http.ResponseWriter,
	userID, teamID, promptSlug string,
	prompt *models.Prompt,
) error {
	err := s.container.PromptService().DeletePromptBySlug(userID, teamID, promptSlug)
	if err != nil {
		// Denial before the string-matching chain (see handleCreatePrompt).
		if stderrors.Is(err, services.ErrPermissionDenied) {
			s.logger.With(
				"service", serverLogServiceName, "handler", "handleDeletePrompt",
				"user_id", userID, "prompt_slug", promptSlug,
			).Warn("Forbidden prompt delete attempt")
			writeErrorResponse(
				w, nil, "forbidden",
				"You can only delete prompts you created; team admins can delete any", http.StatusForbidden,
			)
			return err
		}

		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleDeletePrompt",
			"user_id", userID,
			"prompt_slug", promptSlug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to delete prompt")

		// Check if it's a dependency error
		if strings.Contains(err.Error(), "cannot delete prompt: it is being used by") {
			writeErrorResponse(
				w, nil, "dependency_error",
				"Cannot delete prompt: it is being used by other resources.",
				http.StatusConflict,
			)
			return err
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to delete prompt", http.StatusInternalServerError)
		return err
	}

	// DeleteEmbeddingsByEntity is keyed solely on the entity, so it removes the
	// embedding regardless of which team member triggers the delete.
	if err := s.container.EmbeddingService().DeleteEmbeddingsByEntity("prompt", prompt.ID); err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleDeletePrompt",
			"user_id", userID,
			"prompt_id", prompt.ID,
			"prompt_slug", promptSlug,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to delete prompt embeddings (non-fatal)")
	}

	return nil
}

func (s *Server) handleGetPromptDependencies(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	promptSlug := chi.URLParam(r, "slug")

	s.logger.With(
		"service", serverLogServiceName,
		"handler", "handleGetPromptDependencies",
		"user_id", userID,
		"team_id", teamID,
		"prompt_slug", promptSlug,
	).Info("Get prompt dependencies request received")

	dependencies, err := s.container.PromptService().GetPromptDependenciesBySlug(userID, teamID, promptSlug)
	if err != nil {
		s.logger.With(
			"service", serverLogServiceName,
			"handler", "handleGetPromptDependencies",
			"user_id", userID,
			"prompt_slug", promptSlug,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get prompt dependencies")

		if stderrors.Is(err, repositories.ErrPromptNotFound) {
			writeErrorResponse(w, nil, "not_found", promptMsgNotFound, http.StatusNotFound)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to get prompt dependencies", http.StatusInternalServerError)
		return
	}

	writeOK(w, dependencies, s.logger)
}
