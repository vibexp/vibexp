package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/vibexp/vibexp/internal/services/activities"
)

// ActivityRecorder provides helper methods for recording activities
type ActivityRecorder struct {
	activityService activities.ActivityService
}

// NewActivityRecorder creates a new activity recorder
func NewActivityRecorder(activityService activities.ActivityService) *ActivityRecorder {
	return &ActivityRecorder{
		activityService: activityService,
	}
}

// RecordAuthActivity records authentication-related activities
func (ar *ActivityRecorder) RecordAuthActivity(
	ctx context.Context, userID string, activityType string, sessionID *string,
	metadata map[string]interface{}, r *http.Request,
) {
	// Skip activity recording if service is not available (e.g., during tests)
	if ar.activityService == nil {
		slog.Debug("Activity service not available, skipping activity recording")
		return
	}

	clientIP := getClientIP(r)
	userAgent := r.UserAgent()

	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["endpoint"] = r.URL.Path
	metadata["method"] = r.Method

	err := ar.activityService.RecordAuthActivity(
		ctx, userID, activityType, sessionID, metadata, &clientIP, &userAgent,
	)
	if err != nil {
		slog.With("error", err).
			With(
				"user_id", userID,
				"activity_type", activityType,
			).
			Error("Failed to record auth activity")
	}
}

// RecordResourceActivity records resource management activities
func (ar *ActivityRecorder) RecordResourceActivity(
	ctx context.Context, userID string, activityType string, entityType string,
	entityID *string, description string, metadata map[string]interface{}, r *http.Request,
) {
	// Skip activity recording if service is not available (e.g., during tests)
	if ar.activityService == nil {
		slog.Debug("Activity service not available, skipping activity recording")
		return
	}

	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["endpoint"] = r.URL.Path
	metadata["method"] = r.Method
	metadata["user_agent"] = r.UserAgent()
	metadata["source_ip"] = getClientIP(r)

	err := ar.activityService.RecordResourceActivity(
		ctx, userID, activityType, entityType, entityID, description, metadata,
	)
	if err != nil {
		slog.With("error", err).
			With(
				"user_id", userID,
				"activity_type", activityType,
				"entity_type", entityType,
			).
			Error("Failed to record resource activity")
	}
}

// RecordAPIKeyUsage records API key usage activities
func (ar *ActivityRecorder) RecordAPIKeyUsage(
	ctx context.Context, userID string, apiKeyID string, endpoint string, r *http.Request,
) {
	metadata := map[string]interface{}{
		"endpoint":   endpoint,
		"method":     r.Method,
		"user_agent": r.UserAgent(),
		"source_ip":  getClientIP(r),
	}

	description := "API key used for " + endpoint

	ar.RecordResourceActivity(
		ctx, userID, activities.ActivityTypeAPIKeyUsed, activities.EntityTypeAPIKey,
		&apiKeyID, description, metadata, r,
	)
}

// RecordClaudeCodeActivity records Claude Code session activities
func (ar *ActivityRecorder) RecordClaudeCodeActivity(
	ctx context.Context, userID string, sessionID string, toolName *string,
	hookEventName string, metadata map[string]interface{},
) {
	// Skip activity recording if service is not available (e.g., during tests)
	if ar.activityService == nil {
		slog.Debug("Activity service not available, skipping activity recording")
		return
	}

	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	err := ar.activityService.RecordClaudeCodeActivity(ctx, userID, sessionID, toolName, hookEventName, metadata)
	if err != nil {
		slog.With("error", err).
			With(
				"user_id", userID,
				"session_id", sessionID,
				"hook_event_name", hookEventName,
			).
			Error("Failed to record Claude Code activity")
	}
}

// Note: Removed automatic activity tracking middleware as we have explicit
// activity recording in individual handlers which provides more meaningful
// and specific activity tracking without noise or redundancy.

// Helper methods for activity recording in handlers

// recordPromptActivity records prompt-related activities.
// promptID is the UUID stored as entity_id; promptSlug is kept in metadata for debugging.
func (s *Server) recordPromptActivity(
	ctx context.Context, userID string, activityType string,
	promptID string, promptSlug string, description string, r *http.Request,
) {
	ar := NewActivityRecorder(s.activityService)
	metadata := map[string]interface{}{
		"prompt_slug": promptSlug,
	}
	ar.RecordResourceActivity(
		ctx, userID, activityType, activities.EntityTypePrompt,
		&promptID, description, metadata, r,
	)
}

// recordArtifactActivity records artifact-related activities
func (s *Server) recordArtifactActivity(
	ctx context.Context, userID string, activityType string, artifactID string,
	projectName string, slug string, description string, r *http.Request,
) {
	ar := NewActivityRecorder(s.activityService)
	metadata := map[string]interface{}{
		"project_name":  projectName,
		"artifact_slug": slug,
	}
	ar.RecordResourceActivity(
		ctx, userID, activityType, activities.EntityTypeArtifact,
		&artifactID, description, metadata, r,
	)
}

// recordAPIKeyActivity records API key-related activities
func (s *Server) recordAPIKeyActivity(
	ctx context.Context, userID string, activityType string,
	apiKeyID string, description string, r *http.Request,
) {
	ar := NewActivityRecorder(s.activityService)
	ar.RecordResourceActivity(
		ctx, userID, activityType, activities.EntityTypeAPIKey,
		&apiKeyID, description, nil, r,
	)
}

// recordGitHubImportActivity records GitHub import-related activities
func (s *Server) recordGitHubImportActivity(
	ctx context.Context, userID string, activityType string, entityType string,
	entityID string, description string, metadata map[string]interface{}, r *http.Request,
) {
	ar := NewActivityRecorder(s.activityService)
	var eid *string
	if entityID != "" {
		eid = &entityID
	}
	ar.RecordResourceActivity(ctx, userID, activityType, entityType, eid, description, metadata, r)
}
