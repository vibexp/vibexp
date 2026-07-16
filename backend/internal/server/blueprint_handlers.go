package server

import (
	"context"
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
	"github.com/vibexp/vibexp/internal/services/activities"
)

func (s *Server) handleCreateBlueprint(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateBlueprint",
		"user_id", userID,
		"team_id", teamID,
	).
		Info("Create blueprint request received")

	var req models.CreateBlueprintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logBlueprintError(w, "handleCreateBlueprint", userID, "", "", err,
			"Failed to decode request body", "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if !s.validateCreateBlueprintRequest(w, &req) {
		return
	}

	if !s.checkBlueprintResourceLimit(w, r.Context(), userID) {
		return
	}

	blueprint, err := s.container.BlueprintService().CreateBlueprint(userID, teamID, &req)
	if err != nil {
		s.handleCreateBlueprintError(w, userID, err)
		return
	}

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordBlueprintCreated(r.Context())
	}

	s.recordBlueprintActivity(
		r.Context(), userID, activities.ActivityTypeBlueprintCreated, blueprint.ID,
		req.ProjectID, req.Slug, "Created new blueprint: "+req.Title, r,
	)

	writeCreated(w, blueprint, s.logger)
}

func (s *Server) handleGetBlueprint(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")
	slug := chi.URLParam(r, "slug")

	decodedProjectID, decodedSlug, ok := s.decodeBlueprintURLParams(
		w, userID, "handleGetBlueprint", projectID, slug,
	)
	if !ok {
		return
	}

	// Validate project_id is a valid UUID
	if !isValidUUID(decodedProjectID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid project_id format", http.StatusBadRequest)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetBlueprint",
		"user_id", userID,
		"project_id", decodedProjectID,
		"slug", decodedSlug,
	).Info("Get blueprint request received")

	blueprint, err := s.container.BlueprintService().GetBlueprintByProjectIDAndSlugInTeam(
		userID, teamID, decodedProjectID, decodedSlug,
	)
	if err != nil {
		s.handleGetBlueprintError(w, userID, decodedProjectID, decodedSlug, err)
		return
	}

	contextkeys.SetAccessedResourceID(r.Context(), blueprint.ID)

	writeOK(w, blueprint, s.logger)
}

func (s *Server) handleListBlueprints(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleListBlueprints",
		"user_id", userID,
		"team_id", teamID,
	).
		Info("List blueprints request received")

	filters, ok := s.buildBlueprintFilters(r, "", teamID)
	if !ok {
		return
	}

	response, listErr := s.container.BlueprintService().ListBlueprints(userID, filters)
	if listErr != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListBlueprints",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", listErr),
		).Error("Failed to list blueprints")

		writeErrorResponse(w, nil, "internal_error", "Failed to list blueprints", http.StatusInternalServerError)
		return
	}

	writeOK(w, response, s.logger)
}

func (s *Server) handleListBlueprintsByProject(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")

	decodedProjectID, err := url.QueryUnescape(projectID)
	if err != nil {
		s.logBlueprintError(w, "handleListBlueprintsByProject", userID, projectID, "", err,
			"Failed to decode project ID", "bad_request", "Invalid project ID encoding", http.StatusBadRequest)
		return
	}

	// Validate project_id is a valid UUID
	if !isValidUUID(decodedProjectID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid project_id format", http.StatusBadRequest)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleListBlueprintsByProject",
		"user_id", userID,
		"team_id", teamID,
		"project_id", decodedProjectID,
	).Info("List blueprints by project request received")

	filters, ok := s.buildBlueprintFilters(r, decodedProjectID, teamID)
	if !ok {
		return
	}

	response, listErr := s.container.BlueprintService().ListBlueprints(userID, filters)
	if listErr != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListBlueprintsByProject",
			"user_id", userID,
			"team_id", teamID,
			"project_id", decodedProjectID,
			"error", fmt.Sprintf("%+v", listErr),
		).Error("Failed to list blueprints by project")

		writeErrorResponse(w, nil, "internal_error", "Failed to list blueprints", http.StatusInternalServerError)
		return
	}

	writeOK(w, response, s.logger)
}

func (s *Server) handleUpdateBlueprint(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")
	slug := chi.URLParam(r, "slug")

	decodedProjectID, decodedSlug, ok := s.decodeBlueprintURLParams(
		w, userID, "handleUpdateBlueprint", projectID, slug,
	)
	if !ok {
		return
	}

	// Validate project_id is a valid UUID
	if !isValidUUID(decodedProjectID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid project_id format", http.StatusBadRequest)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateBlueprint",
		"user_id", userID,
		"project_id", decodedProjectID,
		"slug", decodedSlug,
	).Info("Update blueprint request received")

	if !s.checkBlueprintResourceLimit(w, r.Context(), userID) {
		return
	}

	var req models.UpdateBlueprintRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		s.logBlueprintError(w, "handleUpdateBlueprint", userID, decodedProjectID, decodedSlug, decodeErr,
			"Failed to decode request body", "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if !s.validateUpdateBlueprintRequest(w, &req) {
		return
	}

	blueprint, err := s.container.BlueprintService().UpdateBlueprintByProjectIDAndSlugInTeam(
		userID, teamID, decodedProjectID, decodedSlug, &req,
	)
	if err != nil {
		s.handleUpdateBlueprintError(w, userID, decodedProjectID, decodedSlug, err)
		return
	}

	s.recordBlueprintActivity(
		r.Context(), userID, activities.ActivityTypeBlueprintUpdated, blueprint.ID,
		decodedProjectID, decodedSlug, "Updated blueprint: "+blueprint.Title, r,
	)

	writeOK(w, blueprint, s.logger)
}

//nolint:funlen // structured slog attributes are marginally more verbose than the prior logrus WithFields calls
func (s *Server) handleDeleteBlueprint(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")
	slug := chi.URLParam(r, "slug")

	decodedProjectID, decodedSlug, ok := s.decodeBlueprintURLParams(
		w, userID, "handleDeleteBlueprint", projectID, slug,
	)
	if !ok {
		return
	}

	// Validate project_id is a valid UUID
	if !isValidUUID(decodedProjectID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid project_id format", http.StatusBadRequest)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleDeleteBlueprint",
		"user_id", userID,
		"project_id", decodedProjectID,
		"slug", decodedSlug,
	).Info("Delete blueprint request received")

	blueprint, err := s.container.BlueprintService().GetBlueprintByProjectIDAndSlugInTeam(
		userID, teamID, decodedProjectID, decodedSlug,
	)
	if err != nil {
		s.handleGetBlueprintError(w, userID, decodedProjectID, decodedSlug, err)
		return
	}

	err = s.container.BlueprintService().DeleteBlueprintByProjectIDAndSlug(userID, teamID, decodedProjectID, decodedSlug)
	if err != nil {
		// Without this branch a denied delete would hit logBlueprintError's
		// unconditional 500 — it has no branching at all.
		if errors.Is(err, services.ErrPermissionDenied) {
			s.logger.With(
				"service", "vibexp-api", "handler", "handleDeleteBlueprint",
				"user_id", userID, "slug", decodedSlug,
			).Warn("Forbidden blueprint delete attempt")
			writeErrorResponse(
				w, nil, "forbidden",
				"You can only delete blueprints you created; team admins can delete any", http.StatusForbidden,
			)
			return
		}

		s.logBlueprintError(
			w,
			"handleDeleteBlueprint",
			userID,
			decodedProjectID,
			decodedSlug,
			err,
			"Failed to delete blueprint",
			"internal_error",
			"Failed to delete blueprint",
			http.StatusInternalServerError,
		)
		return
	}

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordBlueprintDeleted(r.Context())
	}

	s.deleteBlueprintEmbeddings(userID, blueprint.ID, decodedProjectID, decodedSlug)

	s.recordBlueprintActivity(
		r.Context(), userID, activities.ActivityTypeBlueprintDeleted, blueprint.ID,
		decodedProjectID, decodedSlug, "Deleted blueprint: "+decodedSlug, r,
	)

	writeNoContent(w)
}

func (s *Server) handleGetBlueprintStats(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetBlueprintStats",
		"user_id", userID,
		"team_id", teamID,
	).
		Info("Get blueprint stats request received")

	stats, err := s.container.BlueprintService().GetBlueprintStats(userID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetBlueprintStats",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get blueprint stats")

		writeErrorResponse(w, nil, "internal_error", "Failed to get blueprint stats", http.StatusInternalServerError)
		return
	}

	writeOK(w, stats, s.logger)
}

// Helper functions for blueprint handlers

// validateCreateBlueprintRequest validates the create blueprint request
func (s *Server) validateCreateBlueprintRequest(w http.ResponseWriter, req *models.CreateBlueprintRequest) bool {
	if req.ProjectID == "" {
		writeErrorResponse(w, nil, "validation_error", "project_id is required", http.StatusBadRequest)
		return false
	}
	if !isValidUUID(req.ProjectID) {
		writeErrorResponse(w, nil, "validation_error", "project_id must be a valid UUID", http.StatusBadRequest)
		return false
	}
	if req.Slug == "" {
		writeErrorResponse(w, nil, "validation_error", "Slug is required", http.StatusBadRequest)
		return false
	}
	if req.Title == "" {
		writeErrorResponse(w, nil, "validation_error", "Title is required", http.StatusBadRequest)
		return false
	}
	if req.Content == "" {
		writeErrorResponse(w, nil, "validation_error", "Content is required", http.StatusBadRequest)
		return false
	}
	// Validate type-subtype relationship (handled in validateBlueprintFieldLengths)

	return s.validateBlueprintFieldLengths(
		w, req.Slug, req.Title, req.Description, &req.Type, req.Subtype, &req.Status, req.Metadata,
	)
}

// validateUpdateBlueprintRequest validates the update blueprint request
func (s *Server) validateUpdateBlueprintRequest(w http.ResponseWriter, req *models.UpdateBlueprintRequest) bool {
	slug := ""
	title := ""
	description := ""

	if req.ProjectID != nil && !isValidUUID(*req.ProjectID) {
		writeErrorResponse(w, nil, "validation_error", "project_id must be a valid UUID", http.StatusBadRequest)
		return false
	}
	if req.Slug != nil {
		slug = *req.Slug
	}
	if req.Title != nil {
		title = *req.Title
	}
	if req.Description != nil {
		description = *req.Description
	}

	// Validate type-subtype relationship (handled in validateBlueprintFieldLengths)

	return s.validateBlueprintFieldLengths(
		w, slug, title, description, req.Type, req.Subtype, req.Status, req.Metadata,
	)
}

// validateBlueprintFieldLengths validates field lengths for blueprints
func (s *Server) validateBlueprintFieldLengths(w http.ResponseWriter, slug, title, description string,
	blueprintType, subtype, status *string, metadata map[string]interface{}) bool {
	if !s.validateBlueprintStringLength(w, slug, 255, "Slug") {
		return false
	}
	if !s.validateBlueprintStringLength(w, title, 255, "Title") {
		return false
	}
	if !s.validateBlueprintStringLength(w, description, 500, "Description") {
		return false
	}
	if !s.validateBlueprintType(w, blueprintType) {
		return false
	}
	if !s.validateBlueprintSubtype(w, blueprintType, subtype, metadata) {
		return false
	}
	if !s.validateBlueprintStatus(w, status) {
		return false
	}
	return true
}

// validateBlueprintStringLength validates string length
func (s *Server) validateBlueprintStringLength(
	w http.ResponseWriter, value string, maxLen int, fieldName string,
) bool {
	if value != "" && len(value) > maxLen {
		writeErrorResponse(w, nil, "validation_error",
			fieldName+" cannot be longer than "+strconv.Itoa(maxLen)+" characters", http.StatusBadRequest)
		return false
	}
	return true
}

// validateBlueprintType validates blueprint type
func (s *Server) validateBlueprintType(w http.ResponseWriter, blueprintType *string) bool {
	if blueprintType == nil || *blueprintType == "" {
		return true
	}
	validTypes := []string{"general", "claude-code", "claude", "cursor", "codex"}
	for _, validType := range validTypes {
		if *blueprintType == validType {
			return true
		}
	}
	writeErrorResponse(w, nil, "validation_error",
		"Type must be one of: general, claude-code, claude, cursor, codex", http.StatusBadRequest)
	return false
}

// validSubtypesByType maps blueprint types to their valid subtypes
var validSubtypesByType = map[string][]string{
	"claude-code": {"sub-agents", "skills", "slash-commands", "others"},
	"claude":      {"claude-md"},
	"cursor":      {"skills", "agents", "commands", "rules", "cursor-md"},
	"codex":       {"rules", "skills", "agents-md"},
}

// isSubtypeValidForType checks if the given subtype is valid for the blueprint type
func isSubtypeValidForType(subtype string, validSubtypes []string) bool {
	for _, validSubtype := range validSubtypes {
		if subtype == validSubtype {
			return true
		}
	}
	return false
}

// validateSubAgentsMetadata validates that sub-agents subtype has required model metadata
func validateSubAgentsMetadata(w http.ResponseWriter, metadata map[string]interface{}) bool {
	if metadata == nil {
		writeErrorResponse(w, nil, "validation_error",
			"Sub-agents subtype requires 'model' metadata field", http.StatusBadRequest)
		return false
	}
	modelVal, ok := metadata["model"].(string)
	if !ok || modelVal == "" {
		writeErrorResponse(w, nil, "validation_error",
			"Sub-agents subtype requires valid 'model' metadata value", http.StatusBadRequest)
		return false
	}
	return true
}

// validateBlueprintSubtype validates blueprint subtype
func (s *Server) validateBlueprintSubtype(w http.ResponseWriter, blueprintType *string, subtype *string,
	metadata map[string]interface{}) bool {
	// If no subtype provided, that's valid
	if subtype == nil || *subtype == "" {
		return true
	}

	// Subtype can only be set for specific types (not general)
	if blueprintType == nil || *blueprintType == "" {
		writeErrorResponse(w, nil, "validation_error",
			"Type must be specified when setting subtype", http.StatusBadRequest)
		return false
	}

	// Check if the type supports subtypes
	validSubtypes, typeSupportsSubtypes := validSubtypesByType[*blueprintType]
	if !typeSupportsSubtypes {
		writeErrorResponse(w, nil, "validation_error",
			fmt.Sprintf("Subtype cannot be set for type '%s'", *blueprintType), http.StatusBadRequest)
		return false
	}

	// Validate subtype value is valid for this type
	if !isSubtypeValidForType(*subtype, validSubtypes) {
		writeErrorResponse(w, nil, "validation_error",
			fmt.Sprintf("Invalid subtype for type '%s'", *blueprintType), http.StatusBadRequest)
		return false
	}

	// Validate that sub-agents subtype requires model metadata
	if *subtype == "sub-agents" {
		return validateSubAgentsMetadata(w, metadata)
	}

	return true
}

// validateBlueprintStatus validates blueprint status
func (s *Server) validateBlueprintStatus(w http.ResponseWriter, status *string) bool {
	if status == nil || *status == "" {
		return true
	}
	if *status != "active" && *status != "expired" {
		writeErrorResponse(w, nil, "validation_error", "Status must be one of: active, expired", http.StatusBadRequest)
		return false
	}
	return true
}

// checkBlueprintResourceLimit checks if user has reached their blueprint resource limit
func (s *Server) checkBlueprintResourceLimit(w http.ResponseWriter, ctx context.Context, userID string) bool {
	allowed, err := s.container.ResourceUsageService().CheckResourceLimit(ctx, userID, "blueprint")
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "checkBlueprintResourceLimit",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to check resource limit")
		writeErrorResponse(w, nil, "internal_error", "Failed to check resource limit", http.StatusInternalServerError)
		return false
	}

	if !allowed {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "checkBlueprintResourceLimit",
			"user_id", userID,
			"resource_type", "blueprint",
		).Warn("User has reached their blueprint limit")
		writeErrorResponse(
			w, nil, "resource_limit_exceeded",
			"You have reached the maximum number of blueprints allowed for your subscription plan",
			http.StatusForbidden,
		)
		return false
	}

	return true
}

// handleCreateBlueprintError handles errors from blueprint creation
func (s *Server) handleCreateBlueprintError(w http.ResponseWriter, userID string, err error) {
	// Denials are benign client errors: handled before the ERROR log and before
	// the strings.Contains chain below, which ErrPermissionDenied's text matches
	// nowhere — it would otherwise fall through to a 500.
	if errors.Is(err, services.ErrPermissionDenied) {
		s.logger.With("service", "vibexp-api", "handler", "handleCreateBlueprint", "user_id", userID).
			Warn("Forbidden blueprint write attempt")
		writeErrorResponse(
			w, nil, "forbidden",
			"You do not have permission to create blueprints in this team", http.StatusForbidden,
		)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateBlueprint",
		"user_id", userID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to create blueprint")

	if strings.Contains(err.Error(), "already exists") {
		writeErrorResponse(w, nil, "conflict", err.Error(), http.StatusConflict)
		return
	}

	if strings.Contains(err.Error(), "project not found") {
		writeErrorResponse(w, nil, "bad_request", "Project not found or does not belong to user", http.StatusBadRequest)
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

	writeErrorResponse(w, nil, "internal_error", "Failed to create blueprint", http.StatusInternalServerError)
}

// decodeBlueprintURLParams decodes URL-encoded project ID and slug
func (s *Server) decodeBlueprintURLParams(w http.ResponseWriter, userID, handler, projectID, slug string) (
	string, string, bool) {
	decodedProjectID, err := url.QueryUnescape(projectID)
	if err != nil {
		s.logBlueprintError(w, handler, userID, projectID, "", err,
			"Failed to decode project ID", "bad_request", "Invalid project ID encoding", http.StatusBadRequest)
		return "", "", false
	}

	decodedSlug, err := url.QueryUnescape(slug)
	if err != nil {
		s.logBlueprintError(w, handler, userID, "", slug, err,
			"Failed to decode slug", "bad_request", "Invalid slug encoding", http.StatusBadRequest)
		return "", "", false
	}

	return decodedProjectID, decodedSlug, true
}

// handleGetBlueprintError handles errors from getting a blueprint
func (s *Server) handleGetBlueprintError(w http.ResponseWriter, userID, projectID, slug string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetBlueprint",
		"user_id", userID,
		"project_id", projectID,
		"slug", slug,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to get blueprint")

	if strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Blueprint not found", http.StatusNotFound)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to get blueprint", http.StatusInternalServerError)
}

// handleUpdateBlueprintError handles errors from blueprint update
func (s *Server) handleUpdateBlueprintError(w http.ResponseWriter, userID, projectID, slug string, err error) {
	// Denials are benign client errors: handled before the ERROR log and before
	// the strings.Contains chain below, which ErrPermissionDenied's text matches
	// nowhere — it would otherwise fall through to a 500.
	if errors.Is(err, services.ErrPermissionDenied) {
		s.logger.With("service", "vibexp-api", "handler", "handleUpdateBlueprint", "user_id", userID).
			Warn("Forbidden blueprint write attempt")
		writeErrorResponse(w, nil, "forbidden", "You do not have permission to update this blueprint", http.StatusForbidden)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateBlueprint",
		"user_id", userID,
		"project_id", projectID,
		"slug", slug,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to update blueprint")

	if strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Blueprint not found", http.StatusNotFound)
		return
	}

	if strings.Contains(err.Error(), "already exists") {
		writeErrorResponse(w, nil, "conflict", err.Error(), http.StatusConflict)
		return
	}

	if strings.Contains(err.Error(), "project not found") {
		writeErrorResponse(w, nil, "bad_request", "Project not found or does not belong to user", http.StatusBadRequest)
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

	writeErrorResponse(w, nil, "internal_error", "Failed to update blueprint", http.StatusInternalServerError)
}

// buildBlueprintFilters builds blueprint filters from request query parameters
func (s *Server) buildBlueprintFilters(
	r *http.Request, projectID string, teamID string,
) (services.BlueprintFilters, bool) {
	query := r.URL.Query()

	filters := services.BlueprintFilters{
		ProjectID: projectID,
		TeamID:    teamID,
		Status:    query.Get("status"),
		Type:      query.Get("type"),
		Subtype:   query.Get("subtype"),
		Search:    query.Get("search"),
		SortBy:    query.Get("sort_by"),
		SortOrder: query.Get("sort_order"),
		Page:      1,
		Limit:     20,
	}

	if projectID == "" {
		filters.ProjectID = query.Get("project_id")
	}

	filters.Metadata = extractMetadataFromQuery(query)

	// Parse and validate pagination parameters with bounds checking
	pagination := validatePaginationParams(query.Get("page"), query.Get("limit"))
	filters.Page = pagination.Page
	filters.Limit = pagination.Limit

	return filters, true
}

// deleteBlueprintEmbeddings deletes embeddings for a blueprint
func (s *Server) deleteBlueprintEmbeddings(userID, blueprintID, projectID, slug string) {
	err := s.container.EmbeddingService().DeleteEmbeddingsByEntity("blueprint", blueprintID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteBlueprint",
			"user_id", userID,
			"spec_library_id", blueprintID,
			"project_id", projectID,
			"slug", slug,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to delete blueprint embeddings (non-fatal)")
	}
}

// logBlueprintError logs a blueprint error and writes error response
func (s *Server) logBlueprintError(w http.ResponseWriter, handler, userID, projectID, slug string,
	err error, logMsg, errCode, errMsg string, statusCode int) {
	fields := []any{"service", "vibexp-api", "handler", handler, "user_id", userID, "error", fmt.Sprintf("%+v", err)}
	if projectID != "" {
		fields = append(fields, "project_id", projectID)
	}
	if slug != "" {
		fields = append(fields, "slug", slug)
	}
	s.logger.With(fields...).Error(logMsg)
	writeErrorResponse(w, nil, errCode, errMsg, statusCode)
}

// recordBlueprintActivity records blueprint-related activities
func (s *Server) recordBlueprintActivity(
	ctx context.Context, userID string, activityType string, blueprintID string,
	projectID string, slug string, description string, r *http.Request,
) {
	ar := NewActivityRecorder(s.activityService)
	metadata := map[string]interface{}{
		"project_id":        projectID,
		"spec_library_slug": slug,
	}
	ar.RecordResourceActivity(
		ctx, userID, activityType, activities.EntityTypeBlueprint,
		&blueprintID, description, metadata, r,
	)
}

// handleListBlueprintVersions returns the content-version history (newest-first) for a blueprint.
func (s *Server) handleListBlueprintVersions(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")
	slug := chi.URLParam(r, "slug")

	decodedProjectID, decodedSlug, ok := s.decodeBlueprintURLParams(
		w, userID, "handleListBlueprintVersions", projectID, slug,
	)
	if !ok {
		return
	}

	if !isValidUUID(decodedProjectID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid project_id format", http.StatusBadRequest)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleListBlueprintVersions",
		"user_id", userID,
		"project_id", decodedProjectID,
		"slug", decodedSlug,
	).Info("List blueprint versions request received")

	versions, err := s.container.BlueprintService().ListBlueprintVersionsInTeam(
		userID, teamID, decodedProjectID, decodedSlug,
	)
	if err != nil {
		s.handleBlueprintVersionError(w, userID, decodedProjectID, decodedSlug, err)
		return
	}

	writeOK(w, models.BlueprintVersionListResponse{Versions: versions}, s.logger)
}

// handleGetBlueprintVersion returns a single content-version snapshot of a blueprint.
func (s *Server) handleGetBlueprintVersion(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")
	slug := chi.URLParam(r, "slug")

	decodedProjectID, decodedSlug, ok := s.decodeBlueprintURLParams(
		w, userID, "handleGetBlueprintVersion", projectID, slug,
	)
	if !ok {
		return
	}

	if !isValidUUID(decodedProjectID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid project_id format", http.StatusBadRequest)
		return
	}

	versionNumber, ok := s.parseVersionNumber(w, chi.URLParam(r, "version_number"))
	if !ok {
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetBlueprintVersion",
		"user_id", userID,
		"project_id", decodedProjectID,
		"slug", decodedSlug,
		"version_number", versionNumber,
	).Info("Get blueprint version request received")

	version, err := s.container.BlueprintService().GetBlueprintVersionInTeam(
		userID, teamID, decodedProjectID, decodedSlug, versionNumber,
	)
	if err != nil {
		s.handleBlueprintVersionError(w, userID, decodedProjectID, decodedSlug, err)
		return
	}

	writeOK(w, version, s.logger)
}

// handleRestoreBlueprintVersion restores a blueprint's content to the given version. The
// pre-restore content is snapshotted as a new (system-authored) version.
func (s *Server) handleRestoreBlueprintVersion(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")
	slug := chi.URLParam(r, "slug")

	decodedProjectID, decodedSlug, ok := s.decodeBlueprintURLParams(
		w, userID, "handleRestoreBlueprintVersion", projectID, slug,
	)
	if !ok {
		return
	}

	if !isValidUUID(decodedProjectID) {
		writeErrorResponse(w, nil, "bad_request", "Invalid project_id format", http.StatusBadRequest)
		return
	}

	versionNumber, ok := s.parseVersionNumber(w, chi.URLParam(r, "version_number"))
	if !ok {
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleRestoreBlueprintVersion",
		"user_id", userID,
		"project_id", decodedProjectID,
		"slug", decodedSlug,
		"version_number", versionNumber,
	).Info("Restore blueprint version request received")

	blueprint, err := s.container.BlueprintService().RestoreBlueprintVersionInTeam(
		userID, teamID, decodedProjectID, decodedSlug, versionNumber,
	)
	if err != nil {
		s.handleBlueprintVersionError(w, userID, decodedProjectID, decodedSlug, err)
		return
	}

	s.recordBlueprintActivity(
		r.Context(), userID, activities.ActivityTypeBlueprintUpdated, blueprint.ID,
		decodedProjectID, decodedSlug, "Restored blueprint: "+blueprint.Title, r,
	)

	writeOK(w, blueprint, s.logger)
}

// handleBlueprintVersionError maps content-version lookup errors to HTTP responses.
func (s *Server) handleBlueprintVersionError(w http.ResponseWriter, userID, projectID, slug string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "blueprintVersion",
		"user_id", userID,
		"project_id", projectID,
		"slug", slug,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to process blueprint version request")

	if strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Not found", http.StatusNotFound)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to process blueprint version request",
		http.StatusInternalServerError)
}
