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

// resourceActivityParams groups the descriptive fields of a recorded resource
// activity so recording helpers stay within the parameter budget (go:S107).
type resourceActivityParams struct {
	userID       string
	activityType string
	entityType   string
	entityID     *string
	description  string
	metadata     map[string]interface{}
}

// RecordResourceActivity records resource management activities
func (ar *ActivityRecorder) RecordResourceActivity(
	ctx context.Context, p resourceActivityParams, r *http.Request,
) {
	// Skip activity recording if service is not available (e.g., during tests)
	if ar.activityService == nil {
		slog.Debug("Activity service not available, skipping activity recording")
		return
	}

	metadata := p.metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["endpoint"] = r.URL.Path
	metadata["method"] = r.Method
	metadata["user_agent"] = r.UserAgent()
	metadata["source_ip"] = getClientIP(r)

	err := ar.activityService.RecordResourceActivity(
		ctx, p.userID, p.activityType, p.entityType, p.entityID, p.description, metadata,
	)
	if err != nil {
		slog.With("error", err).
			With(
				"user_id", p.userID,
				"activity_type", p.activityType,
				"entity_type", p.entityType,
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

	ar.RecordResourceActivity(ctx, resourceActivityParams{
		userID:       userID,
		activityType: activities.ActivityTypeAPIKeyUsed,
		entityType:   activities.EntityTypeAPIKey,
		entityID:     &apiKeyID,
		description:  description,
		metadata:     metadata,
	}, r)
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
	ar.RecordResourceActivity(ctx, resourceActivityParams{
		userID:       userID,
		activityType: activityType,
		entityType:   activities.EntityTypePrompt,
		entityID:     &promptID,
		description:  description,
		metadata: map[string]interface{}{
			"prompt_slug": promptSlug,
		},
	}, r)
}

// artifactActivityParams identifies the artifact an activity is recorded for.
type artifactActivityParams struct {
	userID       string
	activityType string
	artifactID   string
	projectName  string
	slug         string
	description  string
}

// recordArtifactActivity records artifact-related activities
func (s *Server) recordArtifactActivity(ctx context.Context, p artifactActivityParams, r *http.Request) {
	ar := NewActivityRecorder(s.activityService)
	ar.RecordResourceActivity(ctx, resourceActivityParams{
		userID:       p.userID,
		activityType: p.activityType,
		entityType:   activities.EntityTypeArtifact,
		entityID:     &p.artifactID,
		description:  p.description,
		metadata: map[string]interface{}{
			"project_name":  p.projectName,
			"artifact_slug": p.slug,
		},
	}, r)
}

// recordAPIKeyActivity records API key-related activities
func (s *Server) recordAPIKeyActivity(
	ctx context.Context, userID string, activityType string,
	apiKeyID string, description string, r *http.Request,
) {
	ar := NewActivityRecorder(s.activityService)
	ar.RecordResourceActivity(ctx, resourceActivityParams{
		userID:       userID,
		activityType: activityType,
		entityType:   activities.EntityTypeAPIKey,
		entityID:     &apiKeyID,
		description:  description,
	}, r)
}

// recordGitHubImportActivity records GitHub import-related activities. An empty
// entityID in p is normalized to a nil entity id on the recorded activity.
func (s *Server) recordGitHubImportActivity(
	ctx context.Context, p resourceActivityParams, r *http.Request,
) {
	ar := NewActivityRecorder(s.activityService)
	if p.entityID != nil && *p.entityID == "" {
		p.entityID = nil
	}
	ar.RecordResourceActivity(ctx, p, r)
}
