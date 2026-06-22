package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

// handleCreateMemoryError handles errors from CreateMemory and writes appropriate responses
func (s *Server) handleCreateMemoryError(w http.ResponseWriter, userID string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateMemory",
		"user_id", userID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to create memory")

	writeErrorResponse(w, nil, "internal_error", "Failed to create memory", http.StatusInternalServerError)
}

func (s *Server) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateMemory",
		"user_id", userID,
		"team_id", teamID,
	).Info("Create memory request received")

	var req models.CreateMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logMemoryError("handleCreateMemory", userID, "", err, "Failed to decode request body")
		writeErrorResponse(w, nil, "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ProjectID == "" {
		writeErrorResponse(w, nil, "validation_error", "project_id is required", http.StatusBadRequest)
		return
	}

	if !isValidUUID(req.ProjectID) {
		writeErrorResponse(w, nil, "validation_error", "project_id must be a valid UUID", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		writeErrorResponse(w, nil, "validation_error", "Text is required", http.StatusBadRequest)
		return
	}

	if !s.validateProjectBelongsToTeam(r.Context(), w, userID, teamID, req.ProjectID) {
		return
	}

	if !s.checkMemoryResourceLimit(w, r, userID) {
		return
	}

	memory, err := s.container.MemoryService().CreateMemory(userID, teamID, &req)
	if err != nil {
		s.handleCreateMemoryError(w, userID, err)
		return
	}

	s.recordMemoryActivity(r.Context(), userID, activities.ActivityTypeMemoryCreated, memory.ID, "Created new memory", r)

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordMemoryCreated(r.Context())
	}

	writeCreated(w, memory, s.logger)
}

func (s *Server) checkMemoryResourceLimit(w http.ResponseWriter, r *http.Request, userID string) bool {
	allowed, err := s.container.ResourceUsageService().CheckResourceLimit(r.Context(), userID, "memory")
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateMemory",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to check resource limit")
		writeErrorResponse(w, nil, "internal_error", "Failed to check resource limit", http.StatusInternalServerError)
		return false
	}

	if !allowed {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCreateMemory",
			"user_id", userID,
			"resource_type", "memory",
		).Warn("User has reached their memory limit")
		writeErrorResponse(
			w, nil, "resource_limit_exceeded",
			"You have reached the maximum number of memories allowed for your subscription plan",
			http.StatusForbidden,
		)
		return false
	}

	return true
}

func (s *Server) logMemoryError(handler, userID, memoryID string, err error, msg string) {
	fields := []any{
		"service", "vibexp-api",
		"handler", handler,
		"user_id", userID,
		"error", fmt.Sprintf("%+v", err),
	}
	if memoryID != "" {
		fields = append(fields, "memory_id", memoryID)
	}
	s.logger.With(fields...).Error(msg)
}

// allowedMemorySortFields contains the allowlisted sort fields for memories
var allowedMemorySortFields = map[string]bool{
	"text": true, "updated_at": true, "created_at": true,
}

// parseMemoryFilters parses query parameters into MemoryFilters
func parseMemoryFilters(w http.ResponseWriter, r *http.Request, teamID string) (services.MemoryFilters, bool) {
	query := r.URL.Query()

	sortBy := query.Get("sort_by")
	if sortBy != "" && !allowedMemorySortFields[sortBy] {
		writeErrorResponse(w, nil, "validation_error", "invalid sort_by value: "+sortBy, http.StatusBadRequest)
		return services.MemoryFilters{}, false
	}

	// Parse and validate pagination parameters with bounds checking
	pagination := validatePaginationParams(query.Get("page"), query.Get("limit"))

	search := query.Get("search")

	var metadataKey, metadataValue *string
	if key := query.Get("metadata_key"); key != "" {
		metadataKey = &key
		if value := query.Get("metadata_value"); value != "" {
			metadataValue = &value
		}
	}

	var projectID *string
	if pid := query.Get("project_id"); pid != "" {
		if !isValidUUID(pid) {
			writeErrorResponse(w, nil, "validation_error", "project_id must be a valid UUID", http.StatusBadRequest)
			return services.MemoryFilters{}, false
		}
		projectID = &pid
	}

	filters := services.MemoryFilters{
		TeamID:        teamID,
		ProjectID:     projectID,
		Search:        search,
		MetadataKey:   metadataKey,
		MetadataValue: metadataValue,
		SortBy:        sortBy,
		SortOrder:     strings.ToLower(query.Get("sort_order")),
		Page:          pagination.Page,
		Limit:         pagination.Limit,
	}

	return filters, true
}

func (s *Server) handleGetMemory(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	memoryID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetMemory",
		"user_id", userID,
		"team_id", teamID,
		"memory_id", memoryID,
	).Info("Get memory request received")

	memory, err := s.container.MemoryService().GetMemory(userID, teamID, memoryID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetMemory",
			"user_id", userID,
			"memory_id", memoryID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get memory")

		if errors.Is(err, repositories.ErrMemoryNotFound) {
			writeErrorResponse(w, nil, "not_found", "Memory not found", http.StatusNotFound)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to get memory", http.StatusInternalServerError)
		return
	}

	contextkeys.SetAccessedResourceID(r.Context(), memory.ID)

	writeOK(w, memory, s.logger)
}

func (s *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleListMemories",
		"user_id", userID,
		"team_id", teamID,
	).Info("List memories request received")

	filters, ok := parseMemoryFilters(w, r, teamID)
	if !ok {
		return
	}

	response, err := s.container.MemoryService().ListMemories(userID, filters)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListMemories",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list memories")
		writeErrorResponse(w, nil, "internal_error", "Failed to list memories", http.StatusInternalServerError)
		return
	}

	writeOK(w, response, s.logger)
}

func (s *Server) handleUpdateMemory(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	memoryID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateMemory",
		"user_id", userID,
		"team_id", teamID,
		"memory_id", memoryID,
	).Info("Update memory request received")

	// Check resource limit before allowing update
	if !s.checkMemoryResourceLimit(w, r, userID) {
		return
	}

	var req models.UpdateMemoryRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		s.logMemoryError("handleUpdateMemory", userID, memoryID, decodeErr, "Failed to decode request body")
		writeErrorResponse(w, nil, "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if !s.validateUpdateMemoryRequest(w, &req) {
		return
	}

	if req.ProjectID != nil {
		if !s.validateProjectBelongsToTeam(r.Context(), w, userID, teamID, *req.ProjectID) {
			return
		}
	}

	memory, err := s.container.MemoryService().UpdateMemory(userID, teamID, memoryID, &req)
	if err != nil {
		s.handleUpdateMemoryError(w, userID, memoryID, err)
		return
	}

	s.recordMemoryActivity(r.Context(), userID, activities.ActivityTypeMemoryUpdated, memoryID, "Updated memory", r)

	writeOK(w, memory, s.logger)
}

func (s *Server) validateUpdateMemoryRequest(w http.ResponseWriter, req *models.UpdateMemoryRequest) bool {
	if req.Text == nil && req.Metadata == nil && req.ProjectID == nil {
		writeErrorResponse(
			w, nil, "validation_error",
			"At least one field (text, metadata, or project_id) must be provided",
			http.StatusBadRequest,
		)
		return false
	}

	if req.Text != nil && *req.Text == "" {
		writeErrorResponse(w, nil, "validation_error", "Text cannot be empty", http.StatusBadRequest)
		return false
	}

	if req.ProjectID != nil && !isValidUUID(*req.ProjectID) {
		writeErrorResponse(w, nil, "validation_error", "project_id must be a valid UUID", http.StatusBadRequest)
		return false
	}

	return true
}

func (s *Server) handleUpdateMemoryError(w http.ResponseWriter, userID, memoryID string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateMemory",
		"user_id", userID,
		"memory_id", memoryID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to update memory")

	if errors.Is(err, repositories.ErrMemoryNotFound) {
		writeErrorResponse(w, nil, "not_found", "Memory not found", http.StatusNotFound)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to update memory", http.StatusInternalServerError)
}

func (s *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	memoryID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleDeleteMemory",
		"user_id", userID,
		"team_id", teamID,
		"memory_id", memoryID,
	).Info("Delete memory request received")

	// Delete the memory
	err := s.container.MemoryService().DeleteMemory(userID, teamID, memoryID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteMemory",
			"user_id", userID,
			"memory_id", memoryID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to delete memory")

		if errors.Is(err, repositories.ErrMemoryNotFound) {
			writeErrorResponse(w, nil, "not_found", "Memory not found", http.StatusNotFound)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to delete memory", http.StatusInternalServerError)
		return
	}

	// Delete associated embeddings (if any exist)
	err = s.container.EmbeddingService().DeleteEmbeddingsByEntity("memory", memoryID)
	if err != nil {
		// Log the error but don't fail the deletion - embeddings might not exist
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteMemory",
			"user_id", userID,
			"memory_id", memoryID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to delete memory embeddings (non-fatal)")
	}

	// Record activity for memory deletion
	s.recordMemoryActivity(r.Context(), userID, activities.ActivityTypeMemoryDeleted, memoryID, "Deleted memory", r)

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordMemoryDeleted(r.Context())
	}

	writeNoContent(w)
}

func (s *Server) handleSearchMemoriesByMetadata(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleSearchMemoriesByMetadata",
		"user_id", userID,
		"team_id", teamID,
	).Info("Search memories by metadata request received")

	metadataKey := r.URL.Query().Get("metadata_key")
	metadataValue := r.URL.Query().Get("metadata_value")

	if metadataKey == "" || metadataValue == "" {
		writeErrorResponse(
			w, nil, "validation_error",
			"Both metadata_key and metadata_value are required",
			http.StatusBadRequest,
		)
		return
	}

	filters, ok := parseMemoryFilters(w, r, teamID)
	if !ok {
		return
	}

	response, err := s.container.MemoryService().SearchMemoriesByMetadata(userID, metadataKey, metadataValue, filters)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleSearchMemoriesByMetadata",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to search memories by metadata")
		writeErrorResponse(w, nil, "internal_error", "Failed to search memories", http.StatusInternalServerError)
		return
	}

	writeOK(w, response, s.logger)
}

// validateProjectBelongsToTeam checks that the given projectID is accessible by userID
// and that its team_id matches the expected teamID. Returns false and writes an error
// response if the check fails.
func (s *Server) validateProjectBelongsToTeam(
	ctx context.Context, w http.ResponseWriter, userID, teamID, projectID string,
) bool {
	project, err := s.container.ProjectRepository().GetByID(ctx, userID, projectID)
	if err != nil {
		if errors.Is(err, repositories.ErrProjectNotFoundForRepo) ||
			strings.Contains(err.Error(), "not found") {
			writeErrorResponse(w, nil, "not_found", "Project not found", http.StatusNotFound)
			return false
		}
		s.logger.With(
			"service", "vibexp-api",
			"user_id", userID,
			"project_id", projectID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to validate project ownership")
		writeErrorResponse(w, nil, "internal_error", "Failed to validate project", http.StatusInternalServerError)
		return false
	}
	if project.TeamID != teamID {
		writeErrorResponse(w, nil, "forbidden", "project does not belong to this team", http.StatusForbidden)
		return false
	}
	return true
}

// handleListMemoryVersions returns the content-version history (newest-first) for a memory.
func (s *Server) handleListMemoryVersions(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	memoryID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleListMemoryVersions",
		"user_id", userID,
		"team_id", teamID,
		"memory_id", memoryID,
	).Info("List memory versions request received")

	versions, err := s.container.MemoryService().ListMemoryVersions(userID, teamID, memoryID)
	if err != nil {
		s.handleMemoryVersionError(w, userID, memoryID, err)
		return
	}

	writeOK(w, models.MemoryVersionListResponse{Versions: versions}, s.logger)
}

// handleGetMemoryVersion returns a single content-version snapshot of a memory.
func (s *Server) handleGetMemoryVersion(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	memoryID := chi.URLParam(r, "id")

	versionNumber, ok := s.parseVersionNumber(w, chi.URLParam(r, "version_number"))
	if !ok {
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetMemoryVersion",
		"user_id", userID,
		"team_id", teamID,
		"memory_id", memoryID,
		"version_number", versionNumber,
	).Info("Get memory version request received")

	version, err := s.container.MemoryService().GetMemoryVersion(userID, teamID, memoryID, versionNumber)
	if err != nil {
		s.handleMemoryVersionError(w, userID, memoryID, err)
		return
	}

	writeOK(w, version, s.logger)
}

// handleRestoreMemoryVersion restores a memory's text to the given version. The pre-restore
// text is snapshotted as a new (system-authored) version.
func (s *Server) handleRestoreMemoryVersion(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	memoryID := chi.URLParam(r, "id")

	versionNumber, ok := s.parseVersionNumber(w, chi.URLParam(r, "version_number"))
	if !ok {
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleRestoreMemoryVersion",
		"user_id", userID,
		"team_id", teamID,
		"memory_id", memoryID,
		"version_number", versionNumber,
	).Info("Restore memory version request received")

	memory, err := s.container.MemoryService().RestoreMemoryVersion(userID, teamID, memoryID, versionNumber)
	if err != nil {
		s.handleMemoryVersionError(w, userID, memoryID, err)
		return
	}

	s.recordMemoryActivity(r.Context(), userID, activities.ActivityTypeMemoryUpdated, memoryID, "Restored memory", r)

	writeOK(w, memory, s.logger)
}

// handleMemoryVersionError maps content-version lookup errors to HTTP responses,
// distinguishing a missing memory/version (404) from other failures (500).
func (s *Server) handleMemoryVersionError(w http.ResponseWriter, userID, memoryID string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "memoryVersion",
		"user_id", userID,
		"memory_id", memoryID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to process memory version request")

	if errors.Is(err, repositories.ErrMemoryNotFound) || strings.Contains(err.Error(), "not found") {
		writeErrorResponse(w, nil, "not_found", "Not found", http.StatusNotFound)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to process memory version request",
		http.StatusInternalServerError)
}

// recordMemoryActivity records an activity related to memory operations
func (s *Server) recordMemoryActivity(
	ctx context.Context, userID, activityType, entityID, description string, r *http.Request,
) {
	userAgent := r.Header.Get("User-Agent")
	sourceIP := getClientIP(r)

	req := activities.CreateActivityRequest{
		ActivityType: activityType,
		EntityType:   activities.EntityTypeMemory,
		EntityID:     &entityID,
		Description:  description,
		UserAgent:    &userAgent,
		SourceIP:     &sourceIP,
		SessionID:    nil, // We'll implement session tracking later
	}

	_, err := s.container.ActivityService().RecordActivity(ctx, userID, req)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"user_id", userID,
			"activity_type", activityType,
			"entity_id", entityID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to record memory activity")
	}
}
