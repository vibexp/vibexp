package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// decodeCursorHookPayload decodes and validates the cursor hook payload
func (s *Server) decodeCursorHookPayload(r *http.Request) (*models.IncomingCursorHookPayload, error) {
	var rawPayload map[string]interface{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&rawPayload); err != nil {
		return nil, err
	}

	userID := r.Context().Value(contextKeyUserID).(string)
	s.logger.WithFields(logrus.Fields{
		"raw_payload": rawPayload,
		"user_id":     userID,
	}).Info("Received Cursor IDE hook payload")

	payloadBytes, err := json.Marshal(rawPayload)
	if err != nil {
		return nil, err
	}

	var payload models.IncomingCursorHookPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, err
	}

	// Use conversation_id as session_id if session_id is not provided
	if payload.SessionID == "" && payload.ConversationID != "" {
		payload.SessionID = payload.ConversationID
	}

	return &payload, nil
}

// validateCursorHookPayload validates required fields in the payload
func validateCursorHookPayload(payload *models.IncomingCursorHookPayload) error {
	if payload.SessionID == "" {
		return &ValidationError{Field: "session_id or conversation_id", Message: "Missing required field"}
	}
	if payload.HookEventName == "" {
		return &ValidationError{Field: "hook_event_name", Message: "Missing required field"}
	}
	return nil
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message + ": " + e.Field
}

// checkSessionLimitAndRespond checks if user has reached session limit and responds accordingly
func (s *Server) checkSessionLimitAndRespond(
	w http.ResponseWriter,
	r *http.Request,
	userID string,
) bool {
	resourceUsageService := s.container.ResourceUsageService()
	canCreateSession, err := resourceUsageService.CheckResourceLimit(r.Context(), userID, "ai_session")
	if err != nil {
		s.logger.WithError(err).Error("Failed to check AI session limit")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to check resource limits", s.logger)
		return false
	}

	if !canCreateSession {
		var currentUsage, limit int
		if usage, usageErr := resourceUsageService.GetResourceUsage(r.Context(), userID); usageErr == nil {
			for _, resource := range usage.Resources {
				if resource.ResourceType == "ai_session" {
					currentUsage = resource.Count
					limit = resource.Limit
					break
				}
			}
		}

		s.logger.WithFields(logrus.Fields{
			"user_id":       userID,
			"current_usage": currentUsage,
			"limit":         limit,
		}).Warn("AI session limit reached")

		writeJSON(w, http.StatusForbidden, map[string]any{
			"status":  "error",
			"message": "AI session limit reached for your subscription plan",
			"details": map[string]interface{}{
				"resource_type": "ai_session",
				"current_usage": currentUsage,
				"limit":         limit,
			},
		}, s.logger)
		return false
	}
	return true
}

// checkToolLimitAndRespond checks if user has reached tool limit and responds accordingly
func (s *Server) checkToolLimitAndRespond(
	w http.ResponseWriter,
	r *http.Request,
	userID string,
) bool {
	resourceUsageService := s.container.ResourceUsageService()
	canCreateTool, err := resourceUsageService.CheckResourceLimit(r.Context(), userID, "ai_tool")
	if err != nil {
		s.logger.WithError(err).Error("Failed to check AI tool limit")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to check resource limits", s.logger)
		return false
	}

	if !canCreateTool {
		var currentUsage, limit int
		if usage, usageErr := resourceUsageService.GetResourceUsage(r.Context(), userID); usageErr == nil {
			for _, resource := range usage.Resources {
				if resource.ResourceType == "ai_tool" {
					currentUsage = resource.Count
					limit = resource.Limit
					break
				}
			}
		}

		s.logger.WithFields(logrus.Fields{
			"user_id":       userID,
			"current_usage": currentUsage,
			"limit":         limit,
		}).Warn("AI tool limit reached")

		writeJSON(w, http.StatusForbidden, map[string]any{
			"status":  "error",
			"message": "AI tools limit reached for your subscription plan",
			"details": map[string]interface{}{
				"resource_type": "ai_tool",
				"current_usage": currentUsage,
				"limit":         limit,
			},
		}, s.logger)
		return false
	}
	return true
}

// convertCursorFieldToJSONBData converts an interface{} to models.JSONBData
func convertCursorFieldToJSONBData(data interface{}) (*models.JSONBData, error) {
	if data == nil {
		return nil, nil
	}
	dataMap := make(models.JSONBData)
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(dataBytes, &dataMap); err != nil {
		return nil, err
	}
	return &dataMap, nil
}

// convertCursorPayloadFields converts all cursor payload fields to JSONB
func convertCursorPayloadFields(payload *models.IncomingCursorHookPayload) (
	configuration, reference, context, input, output, inducedFailure *models.JSONBData,
) {
	var err error
	configuration, err = convertCursorFieldToJSONBData(payload.Configuration)
	if err != nil {
		logrus.WithError(err).Error("Failed to convert configuration")
	}

	reference, err = convertCursorFieldToJSONBData(payload.Reference)
	if err != nil {
		logrus.WithError(err).Error("Failed to convert reference")
	}

	context, err = convertCursorFieldToJSONBData(payload.Context)
	if err != nil {
		logrus.WithError(err).Error("Failed to convert context")
	}

	input, err = convertCursorFieldToJSONBData(payload.Input)
	if err != nil {
		logrus.WithError(err).Error("Failed to convert input")
	}

	output, err = convertCursorFieldToJSONBData(payload.Output)
	if err != nil {
		logrus.WithError(err).Error("Failed to convert output")
	}

	inducedFailure, err = convertCursorFieldToJSONBData(payload.InducedFailure)
	if err != nil {
		logrus.WithError(err).Error("Failed to convert induced failure")
	}

	return configuration, reference, context, input, output, inducedFailure
}

// prepareCursorHookPayload prepares the hook payload model for storage

func prepareCursorHookPayload(
	userID, teamID string,
	payload *models.IncomingCursorHookPayload,
) (*models.CursorIDEHookPayload, error) {
	// Convert the entire payload to JSONB
	fullPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	var fullPayloadMap models.JSONBData
	err = json.Unmarshal(fullPayload, &fullPayloadMap)
	if err != nil {
		return nil, err
	}

	// Convert individual fields
	configuration, reference, context, input, output, inducedFailure := convertCursorPayloadFields(payload)

	// Prepare conversation_id and generation_id as pointers
	var conversationIDPtr, generationIDPtr *string
	if payload.ConversationID != "" {
		conversationIDPtr = &payload.ConversationID
	}
	if payload.GenerationID != "" {
		generationIDPtr = &payload.GenerationID
	}

	return &models.CursorIDEHookPayload{
		UserID:         &userID,
		TeamID:         teamID,
		SessionID:      payload.SessionID,
		ConversationID: conversationIDPtr,
		GenerationID:   generationIDPtr,
		HookEventName:  payload.HookEventName,
		ToolName:       payload.ToolName,
		WorkspaceRoots: payload.WorkspaceRoots,
		Configuration:  configuration,
		Reference:      reference,
		Context:        context,
		Input:          input,
		Output:         output,
		InducedFailure: inducedFailure,
		Payload:        fullPayloadMap,
	}, nil
}

// checkCursorSessionAndCount checks if session exists and gets Cursor IDE session count
func (s *Server) checkCursorSessionAndCount(
	ctx context.Context, repo repositories.CursorIDEHooksRepository, userID, sessionID string,
) (sessionExists bool, count int, err error) {
	sessionExists, err = repo.SessionExists(ctx, userID, sessionID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to check if session exists")
		return false, 0, err
	}

	if !sessionExists {
		count, err = repo.CountUniqueSessions(ctx, userID)
		if err != nil {
			s.logger.WithError(err).Error("Failed to count Cursor IDE sessions")
			// Don't fail the request, just log the error
			return sessionExists, 0, nil
		}
	}

	return sessionExists, count, nil
}

// checkCursorResourceLimits checks session existence and resource limits for new sessions
func (s *Server) checkCursorResourceLimits(
	w http.ResponseWriter,
	r *http.Request,
	userID, sessionID string,
) bool {
	cursorIDERepo := s.container.CursorIDEHooksRepository()
	sessionExists, checkErr := cursorIDERepo.SessionExists(r.Context(), userID, sessionID)
	if checkErr != nil {
		s.logger.WithError(checkErr).Error("Failed to check if session exists")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to check session", s.logger)
		return false
	}

	// If it's a new session, check resource limits
	if !sessionExists {
		if !s.checkSessionLimitAndRespond(w, r, userID) {
			return false
		}

		cursorCount, countErr := cursorIDERepo.CountUniqueSessions(r.Context(), userID)
		if countErr != nil {
			s.logger.WithError(countErr).Error("Failed to count Cursor IDE sessions")
			respondWithHookError(w, http.StatusInternalServerError, "Failed to check resource limits", s.logger)
			return false
		}

		if cursorCount == 0 {
			if !s.checkToolLimitAndRespond(w, r, userID) {
				return false
			}
		}
	}
	return true
}

// handleCursorIDEHooksPost handles POST /api/v1/cursor-ide/hooks
func (s *Server) handleCursorIDEHooksPost(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	payload, err := s.decodeCursorHookPayload(r)
	if err != nil {
		s.logger.WithError(err).Error("Failed to decode Cursor IDE hook payload")
		respondWithHookError(w, http.StatusBadRequest, "Invalid JSON payload", s.logger)
		return
	}

	if validationErr := validateCursorHookPayload(payload); validationErr != nil {
		s.logger.WithField("payload", payload).Error("Validation failed")
		respondWithHookError(w, http.StatusBadRequest, validationErr.Error(), s.logger)
		return
	}

	repo := s.container.CursorIDEHooksRepository()
	sessionExists, cursorCount, err := s.checkCursorSessionAndCount(r.Context(), repo, userID, payload.SessionID)
	if err != nil {
		respondWithHookError(w, http.StatusInternalServerError, "Failed to check session", s.logger)
		return
	}

	if !s.checkCursorResourceLimits(w, r, userID, payload.SessionID) {
		return
	}

	teamID, err := s.getUserDefaultTeamID(r.Context(), userID)
	if err != nil {
		s.logger.WithField("user_id", userID).WithError(err).Error("Failed to get user's default team")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to get user's team", s.logger)
		return
	}

	hookPayload, err := prepareCursorHookPayload(userID, teamID, payload)
	if err != nil {
		s.logger.WithError(err).Error("Failed to prepare hook payload")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to process payload", s.logger)
		return
	}

	if err = repo.Create(r.Context(), hookPayload); err != nil {
		logrus.WithError(err).Error("Failed to store Cursor IDE hook payload")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to store hook payload", s.logger)
		return
	}

	logrus.WithFields(logrus.Fields{
		"id": hookPayload.ID, "session_id": payload.SessionID,
		"hook_event_name": payload.HookEventName, "tool_name": payload.ToolName,
	}).Info("Cursor IDE hook payload stored successfully")

	if !sessionExists && cursorCount == 0 {
		s.fireAIToolSessionEvent(r.Context(), userID, payload.SessionID, "cursor_ide", true)
	}
	respondWithHookSuccess(w, hookPayload, s.logger)
}

// handleCursorIDEHooksGet handles GET /api/v1/cursor-ide/hooks with pagination
func (s *Server) handleCursorIDEHooksGet(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	page, limit := parsePaginationParams(r)

	// Build filters with REQUIRED user ID for security
	filters := repositories.CursorIDEHooksFilters{
		UserID: &userID,
		Page:   page,
		Limit:  limit,
	}

	// Add optional filters
	if sessionID := r.URL.Query().Get("session_id"); sessionID != "" {
		filters.SessionID = &sessionID
	}
	if hookEventName := r.URL.Query().Get("hook_event_name"); hookEventName != "" {
		filters.HookEventName = &hookEventName
	}
	if toolName := r.URL.Query().Get("tool_name"); toolName != "" {
		filters.ToolName = &toolName
	}

	// Use repository to get data
	repo := s.container.CursorIDEHooksRepository()
	response, err := repo.List(r.Context(), filters)
	if err != nil {
		logrus.WithError(err).Error("Failed to retrieve Cursor IDE hook payloads")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to retrieve hook payloads", s.logger)
		return
	}

	logrus.WithFields(logrus.Fields{
		"page":        response.Page,
		"limit":       response.Limit,
		"total":       response.Total,
		"total_pages": response.TotalPages,
		"count":       len(response.Data),
	}).Info("Cursor IDE hook payloads retrieved successfully")

	respondWithJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "Hook payloads retrieved successfully",
		"data":    response,
	}, s.logger)
}

// handleCursorIDESessionsGet handles GET /api/v1/cursor-ide/sessions with pagination
func (s *Server) handleCursorIDESessionsGet(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	page, limit := parsePaginationParams(r)

	filters := repositories.CursorSessionFilters{
		UserID: &userID,
		Page:   page,
		Limit:  limit,
	}

	repo := s.container.CursorIDEHooksRepository()
	response, err := repo.GetSessions(r.Context(), filters)
	if err != nil {
		logrus.WithError(err).Error("Failed to retrieve Cursor IDE sessions")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to retrieve sessions", s.logger)
		return
	}

	logrus.WithFields(logrus.Fields{
		"page":        response.Page,
		"limit":       response.Limit,
		"total":       response.Total,
		"total_pages": response.TotalPages,
		"count":       len(response.Data),
	}).Info("Cursor IDE sessions retrieved successfully")

	respondWithJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "Sessions retrieved successfully",
		"data":    response,
	}, s.logger)
}

// handleCursorIDESessionCountsGet handles GET /api/v1/ai-tools/cursor-ide/session-counts
func (s *Server) handleCursorIDESessionCountsGet(w http.ResponseWriter, r *http.Request) {

	// Get authenticated user ID from context
	userID := r.Context().Value(contextKeyUserID).(string)

	// Parse range parameter (default to last 7 days)
	rangeParam := r.URL.Query().Get("range")
	days := 7 // default

	switch rangeParam {
	case "7d", "7":
		days = 7
	case "14d", "14":
		days = 14
	case "30d", "30":
		days = 30
	case "90d", "90", "3m":
		days = 90
	}

	// Use repository to get session counts with user filtering
	repo := s.container.CursorIDEHooksRepository()
	response, err := repo.GetSessionCounts(r.Context(), userID, days)
	if err != nil {
		logrus.WithError(err).Error("Failed to retrieve Cursor IDE session counts")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to retrieve session counts", s.logger)
		return
	}

	logrus.WithFields(logrus.Fields{
		"total_sessions": response.TotalSessions,
		"range_days":     days,
		"count_entries":  len(response.Counts),
	}).Info("Cursor IDE session counts retrieved successfully")

	writeOK(w, map[string]any{
		"status":  "success",
		"message": "Session counts retrieved successfully",
		"data":    response,
	}, s.logger)
}

// handleCursorIDEOverviewStatsGet handles GET /api/v1/ai-tools/cursor-ide/overview-stats
func (s *Server) handleCursorIDEOverviewStatsGet(w http.ResponseWriter, r *http.Request) {

	// Get authenticated user ID from context
	userID := r.Context().Value(contextKeyUserID).(string)

	// Use repository to get overview stats with user filtering
	repo := s.container.CursorIDEHooksRepository()
	stats, err := repo.GetOverviewStats(r.Context(), userID)
	if err != nil {
		logrus.WithError(err).Error("Failed to retrieve overview stats")
		http.Error(w, "Failed to retrieve stats", http.StatusInternalServerError)
		return
	}

	logrus.WithFields(logrus.Fields{
		"total_sessions":               stats.TotalSessions,
		"sessions_this_week":           stats.SessionsThisWeek,
		"weekly_trend_percent":         stats.WeeklyTrendPercent,
		"avg_user_prompts_per_session": stats.AvgUserPromptsPerSession,
		"total_unique_tools":           stats.TotalUniqueTools,
		"top_tools_count":              len(stats.TopTools),
	}).Info("Cursor IDE overview stats retrieved successfully")

	writeOK(w, map[string]any{
		"status":  "success",
		"message": "Overview stats retrieved successfully",
		"data":    stats,
	}, s.logger)
}

// handleCursorIDERecentActivitiesGet handles GET /api/v1/ai-tools/cursor-ide/recent-activities
func (s *Server) handleCursorIDERecentActivitiesGet(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	page, limit := parsePaginationParams(r)

	// Override default limit to 20 for recent activities
	if r.URL.Query().Get("limit") == "" {
		limit = 20
	}

	// Build filters
	filters := repositories.CursorRecentActivitiesFilters{
		UserID: &userID,
		Page:   page,
		Limit:  limit,
	}

	// Add optional filters using inline declarations
	if sessionID := r.URL.Query().Get("session_id"); sessionID != "" {
		filters.SessionID = &sessionID
	}
	if toolName := r.URL.Query().Get("tool_name"); toolName != "" {
		filters.ToolName = &toolName
	}
	if hookEventName := r.URL.Query().Get("hook_event_name"); hookEventName != "" {
		filters.HookEventName = &hookEventName
	}
	if dateFrom := r.URL.Query().Get("date_from"); dateFrom != "" {
		filters.DateFrom = &dateFrom
	}
	if dateTo := r.URL.Query().Get("date_to"); dateTo != "" {
		filters.DateTo = &dateTo
	}

	// Use repository to get recent activities
	repo := s.container.CursorIDEHooksRepository()
	response, err := repo.GetRecentActivities(r.Context(), filters)
	if err != nil {
		logrus.WithError(err).Error("Failed to retrieve recent Cursor IDE activities")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to retrieve recent activities", s.logger)
		return
	}

	logrus.WithFields(logrus.Fields{
		"activities_count": len(response.Activities),
		"page":             response.Page,
		"limit":            response.Limit,
		"total":            response.Total,
		"total_pages":      response.TotalPages,
	}).Info("Cursor IDE recent activities retrieved successfully")

	respondWithJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "Recent activities retrieved successfully",
		"data":    response,
	}, s.logger)
}

// handleCursorIDESessionDelete handles DELETE /api/v1/ai-tools/cursor-ide/sessions/{session_id}
func (s *Server) handleCursorIDESessionDelete(w http.ResponseWriter, r *http.Request) {

	// Get authenticated user ID from context
	userID := r.Context().Value(contextKeyUserID).(string)

	// Extract session_id from URL path parameter
	sessionID := r.PathValue("session_id")
	if sessionID == "" {
		s.logger.Error("Missing session_id in URL path")
		respondWithHookError(w, http.StatusBadRequest, "Missing session_id parameter", s.logger)
		return
	}

	// Use repository to delete the session
	repo := s.container.CursorIDEHooksRepository()
	err := repo.DeleteSession(r.Context(), userID, sessionID)
	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"user_id":    userID,
			"session_id": sessionID,
		}).Error("Failed to delete Cursor IDE session")

		// Check if it's a "not found" error
		if errors.Is(err, repositories.ErrHookSessionNotFound) {
			respondWithHookError(w, http.StatusNotFound, "Session not found or access denied", s.logger)
			return
		}

		respondWithHookError(w, http.StatusInternalServerError, "Failed to delete session", s.logger)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"user_id":    userID,
		"session_id": sessionID,
	}).Info("Cursor IDE session deleted successfully")

	writeNoContent(w)
}
