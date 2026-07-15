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
	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

func (s *Server) handleCreateArtifact(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateArtifact",
		"user_id", userID,
		"team_id", teamID,
	).
		Info("Create artifact request received")

	var req models.CreateArtifactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logArtifactError(w, "handleCreateArtifact", userID, "", "", err,
			"Failed to decode request body", "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if !s.validateCreateArtifactRequest(r.Context(), w, teamID, &req) {
		return
	}

	if !s.checkArtifactResourceLimit(w, r.Context(), userID) {
		return
	}

	artifact, err := s.container.ArtifactService().CreateArtifact(userID, teamID, &req)
	if err != nil {
		s.handleCreateArtifactError(w, userID, err)
		return
	}

	s.recordArtifactActivity(
		r.Context(), userID, activities.ActivityTypeArtifactCreated, artifact.ID,
		req.ProjectID, req.Slug, "Created new artifact: "+req.Title, r,
	)

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordArtifactCreated(r.Context())
	}

	writeCreated(w, artifact, s.logger)
}

func (s *Server) handleGetArtifact(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")
	slug := chi.URLParam(r, "slug")

	decodedProjectID, decodedSlug, ok := s.decodeArtifactURLParams(w, userID, "handleGetArtifact", projectID, slug)
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
		"handler", "handleGetArtifact",
		"user_id", userID,
		"team_id", teamID,
		"project_id", decodedProjectID,
		"slug", decodedSlug,
	).Info("Get artifact request received")

	artifact, err := s.container.ArtifactService().GetArtifactByProjectIDAndSlug(userID, decodedProjectID, decodedSlug)
	if err != nil {
		s.handleGetArtifactError(w, userID, decodedProjectID, decodedSlug, err)
		return
	}

	contextkeys.SetAccessedResourceID(r.Context(), artifact.ID)

	writeOK(w, artifact, s.logger)
}

func (s *Server) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleListArtifacts",
		"user_id", userID,
		"team_id", teamID,
	).
		Info("List artifacts request received")

	filters := s.buildArtifactFilters(r, "", teamID)

	response, listErr := s.container.ArtifactService().ListArtifacts(userID, filters)
	if listErr != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListArtifacts",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", listErr),
		).Error("Failed to list artifacts")

		writeErrorResponse(w, nil, "internal_error", "Failed to list artifacts", http.StatusInternalServerError)
		return
	}

	writeOK(w, response, s.logger)
}

func (s *Server) handleListArtifactsByProject(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")

	decodedProjectID, err := url.QueryUnescape(projectID)
	if err != nil {
		s.logArtifactError(w, "handleListArtifactsByProject", userID, projectID, "", err,
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
		"handler", "handleListArtifactsByProject",
		"user_id", userID,
		"team_id", teamID,
		"project_id", decodedProjectID,
	).Info("List artifacts by project request received")

	filters := s.buildArtifactFilters(r, decodedProjectID, teamID)

	response, listErr := s.container.ArtifactService().ListArtifacts(userID, filters)
	if listErr != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListArtifactsByProject",
			"user_id", userID,
			"team_id", teamID,
			"project_id", decodedProjectID,
			"error", fmt.Sprintf("%+v", listErr),
		).Error("Failed to list artifacts by project")

		writeErrorResponse(w, nil, "internal_error", "Failed to list artifacts", http.StatusInternalServerError)
		return
	}

	writeOK(w, response, s.logger)
}

func (s *Server) handleUpdateArtifact(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")
	slug := chi.URLParam(r, "slug")

	decodedProjectID, decodedSlug, ok := s.decodeArtifactURLParams(w, userID, "handleUpdateArtifact", projectID, slug)
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
		"handler", "handleUpdateArtifact",
		"user_id", userID,
		"team_id", teamID,
		"project_id", decodedProjectID,
		"slug", decodedSlug,
	).Info("Update artifact request received")

	// Check resource limit before allowing update
	if !s.checkArtifactResourceLimit(w, r.Context(), userID) {
		return
	}

	var req models.UpdateArtifactRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		s.logArtifactError(w, "handleUpdateArtifact", userID, decodedProjectID, decodedSlug, decodeErr,
			"Failed to decode request body", "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if !s.validateUpdateArtifactRequest(r.Context(), w, teamID, &req) {
		return
	}

	artifact, err := s.container.ArtifactService().UpdateArtifactByProjectIDAndSlug(
		userID, decodedProjectID, decodedSlug, &req,
	)
	if err != nil {
		s.handleUpdateArtifactError(w, userID, decodedProjectID, decodedSlug, err)
		return
	}

	s.recordArtifactActivity(
		r.Context(), userID, activities.ActivityTypeArtifactUpdated, artifact.ID,
		decodedProjectID, decodedSlug, "Updated artifact: "+artifact.Title, r,
	)

	writeOK(w, artifact, s.logger)
}

// writeArtifactDeleteDenial writes a 403 for a denied delete and reports whether
// it handled the error. Without it the denial would reach logArtifactError,
// which writes an unconditional 500.
func (s *Server) writeArtifactDeleteDenial(w http.ResponseWriter, userID, slug string, err error) bool {
	if !errors.Is(err, services.ErrPermissionDenied) {
		return false
	}
	s.logger.With(
		"service", "vibexp-api", "handler", "handleDeleteArtifact",
		"user_id", userID, "slug", slug,
	).Warn("Forbidden artifact delete attempt")
	writeErrorResponse(
		w, nil, "forbidden",
		"You can only delete artifacts you created; team admins can delete any", http.StatusForbidden,
	)
	return true
}

func (s *Server) handleDeleteArtifact(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")
	slug := chi.URLParam(r, "slug")

	decodedProjectID, decodedSlug, ok := s.decodeArtifactURLParams(w, userID, "handleDeleteArtifact", projectID, slug)
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
		"handler", "handleDeleteArtifact",
		"user_id", userID,
		"team_id", teamID,
		"project_id", decodedProjectID,
		"slug", decodedSlug,
	).Info("Delete artifact request received")

	artifact, err := s.container.ArtifactService().GetArtifactByProjectIDAndSlug(userID, decodedProjectID, decodedSlug)
	if err != nil {
		s.handleGetArtifactError(w, userID, decodedProjectID, decodedSlug, err)
		return
	}

	err = s.container.ArtifactService().DeleteArtifactByProjectIDAndSlug(userID, decodedProjectID, decodedSlug)
	if err != nil {
		if s.writeArtifactDeleteDenial(w, userID, decodedSlug, err) {
			return
		}

		s.logArtifactError(w, "handleDeleteArtifact", userID, decodedProjectID, decodedSlug, err,
			"Failed to delete artifact", "internal_error", "Failed to delete artifact", http.StatusInternalServerError)
		return
	}

	s.deleteArtifactEmbeddings(userID, artifact.ID, decodedProjectID, decodedSlug)
	s.deleteArtifactAttachments(userID, artifact.ID)

	s.recordArtifactActivity(
		r.Context(), userID, activities.ActivityTypeArtifactDeleted, artifact.ID,
		decodedProjectID, decodedSlug, "Deleted artifact: "+decodedSlug, r,
	)

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordArtifactDeleted(r.Context())
	}

	writeNoContent(w)
}

func (s *Server) handleGetArtifactStats(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetArtifactStats",
		"user_id", userID,
		"team_id", teamID,
	).
		Info("Get artifact stats request received")

	stats, err := s.container.ArtifactService().GetArtifactStats(userID, teamID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetArtifactStats",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get artifact stats")

		writeErrorResponse(w, nil, "internal_error", "Failed to get artifact stats", http.StatusInternalServerError)
		return
	}

	writeOK(w, stats, s.logger)
}

func (s *Server) handleListArtifactVersions(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")
	slug := chi.URLParam(r, "slug")

	decodedProjectID, decodedSlug, ok := s.decodeArtifactURLParams(
		w, userID, "handleListArtifactVersions", projectID, slug,
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
		"handler", "handleListArtifactVersions",
		"user_id", userID,
		"team_id", teamID,
		"project_id", decodedProjectID,
		"slug", decodedSlug,
	).Info("List artifact versions request received")

	versions, err := s.container.ArtifactService().ListArtifactVersionsInTeam(
		userID, teamID, decodedProjectID, decodedSlug,
	)
	if err != nil {
		s.handleArtifactVersionError(w, userID, decodedProjectID, decodedSlug, err)
		return
	}

	writeOK(w, models.ArtifactVersionListResponse{Versions: versions}, s.logger)
}

func (s *Server) handleGetArtifactVersion(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")
	slug := chi.URLParam(r, "slug")

	decodedProjectID, decodedSlug, ok := s.decodeArtifactURLParams(
		w, userID, "handleGetArtifactVersion", projectID, slug,
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
		"handler", "handleGetArtifactVersion",
		"user_id", userID,
		"team_id", teamID,
		"project_id", decodedProjectID,
		"slug", decodedSlug,
		"version_number", versionNumber,
	).Info("Get artifact version request received")

	version, err := s.container.ArtifactService().GetArtifactVersionInTeam(
		userID, teamID, decodedProjectID, decodedSlug, versionNumber,
	)
	if err != nil {
		s.handleArtifactVersionError(w, userID, decodedProjectID, decodedSlug, err)
		return
	}

	writeOK(w, version, s.logger)
}

func (s *Server) handleRestoreArtifactVersion(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	projectID := chi.URLParam(r, "project_id")
	slug := chi.URLParam(r, "slug")

	decodedProjectID, decodedSlug, ok := s.decodeArtifactURLParams(
		w, userID, "handleRestoreArtifactVersion", projectID, slug,
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
		"handler", "handleRestoreArtifactVersion",
		"user_id", userID,
		"team_id", teamID,
		"project_id", decodedProjectID,
		"slug", decodedSlug,
		"version_number", versionNumber,
	).Info("Restore artifact version request received")

	artifact, err := s.container.ArtifactService().RestoreArtifactVersionInTeam(
		userID, teamID, decodedProjectID, decodedSlug, versionNumber,
	)
	if err != nil {
		s.handleArtifactVersionError(w, userID, decodedProjectID, decodedSlug, err)
		return
	}

	s.recordArtifactActivity(
		r.Context(), userID, activities.ActivityTypeArtifactUpdated, artifact.ID,
		decodedProjectID, decodedSlug, "Restored artifact: "+artifact.Title, r,
	)

	writeOK(w, artifact, s.logger)
}

// parseVersionNumber parses the version_number URL param as a positive integer.
func (s *Server) parseVersionNumber(w http.ResponseWriter, raw string) (int, bool) {
	versionNumber, err := strconv.Atoi(raw)
	if err != nil || versionNumber < 1 {
		writeErrorResponse(w, nil, "bad_request", "Invalid version_number", http.StatusBadRequest)
		return 0, false
	}
	return versionNumber, true
}

// handleArtifactVersionError maps content-version lookup errors to HTTP responses,
// distinguishing a missing artifact/version (404) from other failures (500).
func (s *Server) handleArtifactVersionError(w http.ResponseWriter, userID, projectID, slug string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "artifactVersion",
		"user_id", userID,
		"project_id", projectID,
		"slug", slug,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to process artifact version request")

	if strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Not found", http.StatusNotFound)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to process artifact version request",
		http.StatusInternalServerError)
}

// Helper functions for artifact handlers

// isValidUUID checks if a string is a valid UUID
func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// validateCreateArtifactRequest validates the create artifact request
func (s *Server) validateCreateArtifactRequest(
	ctx context.Context, w http.ResponseWriter, teamID string, req *models.CreateArtifactRequest,
) bool {
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
	return s.validateArtifactFieldLengths(ctx, w, teamID, req.Slug, req.Title, req.Description, &req.Type, &req.Status)
}

// validateUpdateArtifactRequest validates the update artifact request
func (s *Server) validateUpdateArtifactRequest(
	ctx context.Context, w http.ResponseWriter, teamID string, req *models.UpdateArtifactRequest,
) bool {
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

	return s.validateArtifactFieldLengths(ctx, w, teamID, slug, title, description, req.Type, req.Status)
}

// validateArtifactFieldLengths validates field lengths for artifacts
func (s *Server) validateArtifactFieldLengths(ctx context.Context, w http.ResponseWriter, teamID string,
	slug, title, description string, artifactType, status *string) bool {
	if !s.validateArtifactStringLength(w, slug, 255, "Slug") {
		return false
	}
	if !s.validateArtifactStringLength(w, title, 255, "Title") {
		return false
	}
	if !s.validateArtifactStringLength(w, description, 500, "Description") {
		return false
	}
	if !s.validateArtifactType(ctx, w, teamID, artifactType) {
		return false
	}
	if !s.validateArtifactStatus(w, status) {
		return false
	}
	return true
}

// validateArtifactStringLength validates string length
func (s *Server) validateArtifactStringLength(w http.ResponseWriter, value string, maxLen int, fieldName string) bool {
	if value != "" && len(value) > maxLen {
		writeErrorResponse(w, nil, "validation_error",
			fieldName+" cannot be longer than "+strconv.Itoa(maxLen)+" characters", http.StatusBadRequest)
		return false
	}
	return true
}

// validateArtifactType validates that the artifact type is one of the team's
// types (a global system default or a team custom type) via the TypeService. An
// empty type is valid — the service defaults it to "general".
func (s *Server) validateArtifactType(
	ctx context.Context, w http.ResponseWriter, teamID string, artifactType *string,
) bool {
	if artifactType == nil || *artifactType == "" {
		return true
	}
	valid, err := s.container.TypeService().ValidateType(ctx, teamID, "artifacts", *artifactType)
	if err != nil {
		s.logger.With(
			"handler", "validateArtifactType",
			"team_id", teamID,
			"type", *artifactType,
			"error", err.Error(),
		).Error("Failed to validate artifact type")
		writeErrorResponse(w, nil, "internal_error", "Failed to validate artifact type", http.StatusInternalServerError)
		return false
	}
	if !valid {
		writeErrorResponse(w, nil, "validation_error",
			"Type does not exist for this team", http.StatusBadRequest)
		return false
	}
	return true
}

// validateArtifactStatus validates artifact status
func (s *Server) validateArtifactStatus(w http.ResponseWriter, status *string) bool {
	if status == nil || *status == "" {
		return true
	}
	switch *status {
	case models.ArtifactStatusActive, models.ArtifactStatusDraft, models.ArtifactStatusArchived:
		return true
	default:
		writeErrorResponse(w, nil, "validation_error",
			"Status must be one of: active, draft, archived", http.StatusBadRequest)
		return false
	}
}

// checkArtifactResourceLimit checks if user has reached their artifact resource limit
func (s *Server) checkArtifactResourceLimit(w http.ResponseWriter, ctx context.Context, userID string) bool {
	allowed, err := s.container.ResourceUsageService().CheckResourceLimit(ctx, userID, "artifact")
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "checkArtifactResourceLimit",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to check resource limit")
		writeErrorResponse(w, nil, "internal_error", "Failed to check resource limit", http.StatusInternalServerError)
		return false
	}

	if !allowed {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "checkArtifactResourceLimit",
			"user_id", userID,
			"resource_type", "artifact",
		).Warn("User has reached their artifact limit")
		writeErrorResponse(
			w, nil, "resource_limit_exceeded",
			"You have reached the maximum number of artifacts allowed for your subscription plan",
			http.StatusForbidden,
		)
		return false
	}

	return true
}

// handleCreateArtifactError handles errors from artifact creation
func (s *Server) handleCreateArtifactError(w http.ResponseWriter, userID string, err error) {
	// Denials are benign client errors: handled before the ERROR log and before
	// the strings.Contains chain below, which ErrPermissionDenied's text matches
	// nowhere — it would otherwise fall through to a 500.
	if errors.Is(err, services.ErrPermissionDenied) {
		s.logger.With("service", "vibexp-api", "handler", "handleCreateArtifact", "user_id", userID).
			Warn("Forbidden artifact write attempt")
		writeErrorResponse(
			w, nil, "forbidden",
			"You do not have permission to create artifacts in this team", http.StatusForbidden,
		)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateArtifact",
		"user_id", userID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to create artifact")

	if strings.Contains(err.Error(), "already exists") {
		writeErrorResponse(w, nil, "conflict", "An artifact with this slug already exists", http.StatusConflict)
		return
	}

	if strings.Contains(err.Error(), "project not found") {
		writeErrorResponse(w, nil, "bad_request", "Project not found or does not belong to user", http.StatusBadRequest)
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

	writeErrorResponse(w, nil, "internal_error", "Failed to create artifact", http.StatusInternalServerError)
}

// decodeArtifactURLParams decodes URL-encoded project ID and slug
func (s *Server) decodeArtifactURLParams(w http.ResponseWriter, userID, handler, projectID, slug string) (
	string, string, bool) {
	decodedProjectID, err := url.QueryUnescape(projectID)
	if err != nil {
		s.logArtifactError(w, handler, userID, projectID, "", err,
			"Failed to decode project ID", "bad_request", "Invalid project ID encoding", http.StatusBadRequest)
		return "", "", false
	}

	decodedSlug, err := url.QueryUnescape(slug)
	if err != nil {
		s.logArtifactError(w, handler, userID, "", slug, err,
			"Failed to decode slug", "bad_request", "Invalid slug encoding", http.StatusBadRequest)
		return "", "", false
	}

	return decodedProjectID, decodedSlug, true
}

// handleGetArtifactError handles errors from getting an artifact
func (s *Server) handleGetArtifactError(w http.ResponseWriter, userID, projectID, slug string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetArtifact",
		"user_id", userID,
		"project_id", projectID,
		"slug", slug,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to get artifact")

	if strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Artifact not found", http.StatusNotFound)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to get artifact", http.StatusInternalServerError)
}

// handleUpdateArtifactError handles errors from artifact update
func (s *Server) handleUpdateArtifactError(w http.ResponseWriter, userID, projectID, slug string, err error) {
	// Denials are benign client errors: handled before the ERROR log and before
	// the strings.Contains chain below, which ErrPermissionDenied's text matches
	// nowhere — it would otherwise fall through to a 500.
	if errors.Is(err, services.ErrPermissionDenied) {
		s.logger.With("service", "vibexp-api", "handler", "handleUpdateArtifact", "user_id", userID).
			Warn("Forbidden artifact write attempt")
		writeErrorResponse(w, nil, "forbidden", "You do not have permission to update this artifact", http.StatusForbidden)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateArtifact",
		"user_id", userID,
		"project_id", projectID,
		"slug", slug,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to update artifact")

	if strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Artifact not found", http.StatusNotFound)
		return
	}

	if strings.Contains(err.Error(), "already exists") {
		writeErrorResponse(w, nil, "conflict", "An artifact with this slug already exists", http.StatusConflict)
		return
	}

	if strings.Contains(err.Error(), "project not found") {
		writeErrorResponse(w, nil, "bad_request", "Project not found or does not belong to user", http.StatusBadRequest)
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

	writeErrorResponse(w, nil, "internal_error", "Failed to update artifact", http.StatusInternalServerError)
}

// buildArtifactFilters builds artifact filters from request query parameters
func (s *Server) buildArtifactFilters(
	r *http.Request, projectID, teamID string,
) services.ArtifactFilters {
	filters := services.ArtifactFilters{
		ProjectID: projectID,
		TeamID:    teamID,
		Status:    r.URL.Query().Get("status"),
		Type:      r.URL.Query().Get("type"),
		Search:    r.URL.Query().Get("search"),
		SortBy:    r.URL.Query().Get("sort_by"),
		SortOrder: r.URL.Query().Get("sort_order"),
		Page:      1,
		Limit:     20,
	}

	if projectID == "" {
		filters.ProjectID = r.URL.Query().Get("project_id")
	}

	filters.Metadata = extractMetadataFromQuery(r.URL.Query())

	// Parse and validate pagination parameters with bounds checking
	pagination := validatePaginationParams(r.URL.Query().Get("page"), r.URL.Query().Get("limit"))
	filters.Page = pagination.Page
	filters.Limit = pagination.Limit

	return filters
}

// extractMetadataFromQuery extracts metadata_* query parameters
func extractMetadataFromQuery(query url.Values) map[string]string {
	metadata := make(map[string]string)
	for key, values := range query {
		if strings.HasPrefix(key, "metadata_") {
			metaKey := strings.TrimPrefix(key, "metadata_")
			if len(values) > 0 {
				metadata[metaKey] = values[0]
			}
		}
	}
	return metadata
}

// deleteArtifactEmbeddings deletes embeddings for an artifact
func (s *Server) deleteArtifactEmbeddings(userID, artifactID, projectID, slug string) {
	err := s.container.EmbeddingService().DeleteEmbeddingsByEntity("artifact", artifactID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteArtifact",
			"user_id", userID,
			"artifact_id", artifactID,
			"project_id", projectID,
			"slug", slug,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to delete artifact embeddings (non-fatal)")
	}
}

// logArtifactError logs an artifact error and writes error response
func (s *Server) logArtifactError(w http.ResponseWriter, handler, userID, projectID, slug string,
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
