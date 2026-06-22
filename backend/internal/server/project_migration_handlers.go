package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/services/activities"
	"github.com/vibexp/vibexp/internal/services/projectmigration"
)

// handleGetMigrationInventory returns a count+list of all resources in the given project.
//
// GET /api/v1/{team_id}/projects/{project_id}/migration/inventory
func (s *Server) handleGetMigrationInventory(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	projectID := chi.URLParam(r, "project_id")

	if _, err := uuid.Parse(projectID); err != nil {
		writeErrorResponse(w, r, "bad_request", "project_id must be a valid UUID", http.StatusBadRequest)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetMigrationInventory",
		"user_id", userID,
		"team_id", teamID,
		"project_id", projectID,
	).Info("Get migration inventory request received")

	inventory, err := s.container.ProjectMigrationService().GetInventory(r.Context(), userID, teamID, projectID)
	if err != nil {
		s.handleMigrationError(w, "handleGetMigrationInventory", userID, projectID, err)
		return
	}

	writeOK(w, inventory, s.logger)
}

// handleMigrateProject moves selected resources from the source project to the destination project.
//
// POST /api/v1/{team_id}/projects/{project_id}/migration
func (s *Server) handleMigrateProject(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	projectID := chi.URLParam(r, "project_id")

	if _, err := uuid.Parse(projectID); err != nil {
		writeErrorResponse(w, r, "bad_request", "project_id must be a valid UUID", http.StatusBadRequest)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleMigrateProject",
		"user_id", userID,
		"team_id", teamID,
		"project_id", projectID,
	).Info("Migrate project request received")

	req, ok := decodeMigrationRequest(w, r)
	if !ok {
		return
	}

	result, err := s.container.ProjectMigrationService().Migrate(r.Context(), userID, teamID, projectID, req)
	if err != nil {
		s.handleMigrationError(w, "handleMigrateProject", userID, projectID, err)
		return
	}

	ar := NewActivityRecorder(s.activityService)
	ar.RecordResourceActivity(
		r.Context(), userID,
		activities.ActivityTypeProjectMigrated,
		activities.EntityTypeProject,
		&projectID,
		fmt.Sprintf("Migrated resources from project %s to project %s",
			result.SourceProjectName, result.DestinationProjectName),
		map[string]interface{}{
			"source_project_id":      projectID,
			"destination_project_id": req.DestinationProjectID,
			"migrated_prompts":       result.Migrated.Prompts,
			"migrated_artifacts":     result.Migrated.Artifacts,
			"migrated_blueprints":    result.Migrated.Blueprints,
			"migrated_feed_items":    result.Migrated.FeedItems,
			"conflict_policy":        string(req.ConflictPolicy),
		},
		r,
	)

	writeOK(w, result, s.logger)
}

// decodeMigrationRequest decodes, validates, and checks UUIDs in a migration request body.
// It writes an error response and returns (nil, false) on any failure.
func decodeMigrationRequest(w http.ResponseWriter, r *http.Request) (*projectmigration.MigrationRequest, bool) {
	var req projectmigration.MigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, r, "bad_request", "Invalid request body", http.StatusBadRequest)
		return nil, false
	}
	if err := validateMigrationRequest(&req); err != nil {
		writeErrorResponse(w, r, "validation_error", err.Error(), http.StatusBadRequest)
		return nil, false
	}
	if _, err := uuid.Parse(req.DestinationProjectID); err != nil {
		writeErrorResponse(w, r, "bad_request", "destination_project_id must be a valid UUID", http.StatusBadRequest)
		return nil, false
	}
	return &req, true
}

// validateMigrationRequest performs basic structural validation of the request body.
func validateMigrationRequest(req *projectmigration.MigrationRequest) error {
	if req.DestinationProjectID == "" {
		return fmt.Errorf("destination_project_id is required")
	}

	validPolicies := map[projectmigration.ConflictPolicy]bool{
		projectmigration.ConflictPolicySkip:      true,
		projectmigration.ConflictPolicyRename:    true,
		projectmigration.ConflictPolicyOverwrite: true,
	}
	if req.ConflictPolicy != "" && !validPolicies[req.ConflictPolicy] {
		return fmt.Errorf("conflict_policy must be one of: skip, rename, overwrite")
	}
	// Default to skip when not specified.
	if req.ConflictPolicy == "" {
		req.ConflictPolicy = projectmigration.ConflictPolicySkip
	}

	return nil
}

// handleMigrationError maps service errors to HTTP responses for migration endpoints.
func (s *Server) handleMigrationError(
	w http.ResponseWriter, handler, userID, projectID string, err error,
) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", handler,
		"user_id", userID,
		"project_id", projectID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Migration operation failed")

	errStr := err.Error()
	switch {
	case errors.Is(err, projectmigration.ErrTeamMismatch):
		writeErrorResponse(w, nil, "forbidden", "Project does not belong to the specified team", http.StatusForbidden)
	case strings.Contains(errStr, "not accessible") || strings.Contains(errStr, "not found"):
		writeErrorResponse(w, nil, "not_found", "Project not found", http.StatusNotFound)
	case strings.Contains(errStr, "cross-team migration not supported"):
		writeErrorResponse(w, nil, "bad_request", err.Error(), http.StatusBadRequest)
	default:
		writeErrorResponse(w, nil, "internal_error", "Migration operation failed", http.StatusInternalServerError)
	}
}
