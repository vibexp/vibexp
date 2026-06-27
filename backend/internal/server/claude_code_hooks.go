package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// decodeClaudeHookPayload decodes and validates the Claude Code hook payload
func (s *Server) decodeClaudeHookPayload(r *http.Request) (*models.IncomingHookPayload, error) {
	var payload models.IncomingHookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

// validateClaudeHookPayload validates required fields in the Claude Code hook payload
func validateClaudeHookPayload(payload *models.IncomingHookPayload) error {
	if payload.SessionID == "" {
		return &ValidationError{Field: "session_id", Message: "Missing required field"}
	}
	if payload.HookEventName == "" {
		return &ValidationError{Field: "hook_event_name", Message: "Missing required field"}
	}
	return nil
}

// checkClaudeSessionLimit checks if user has reached session limit and responds accordingly
func (s *Server) checkClaudeSessionLimit(w http.ResponseWriter, r *http.Request, userID string) bool {
	resourceUsageService := s.container.ResourceUsageService()
	canCreateSession, err := resourceUsageService.CheckResourceLimit(r.Context(), userID, "ai_session")
	if err != nil {
		s.logger.With("error", err).Error("Failed to check AI session limit")
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

		s.logger.With(
			"user_id", userID,
			"current_usage", currentUsage,
			"limit", limit,
		).Warn("AI session limit reached")

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

// checkClaudeToolLimit checks if user has reached tool limit and responds accordingly
func (s *Server) checkClaudeToolLimit(w http.ResponseWriter, r *http.Request, userID string) bool {
	resourceUsageService := s.container.ResourceUsageService()
	canCreateTool, err := resourceUsageService.CheckResourceLimit(r.Context(), userID, "ai_tool")
	if err != nil {
		s.logger.With("error", err).Error("Failed to check AI tool limit")
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

		s.logger.With(
			"user_id", userID,
			"current_usage", currentUsage,
			"limit", limit,
		).Warn("AI tool limit reached")

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

// convertToJSONBData converts an interface{} to models.JSONBData
func convertToJSONBData(data interface{}) (*models.JSONBData, error) {
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

// prepareClaudeHookPayload prepares the hook payload model for storage
func prepareClaudeHookPayload(
	userID, teamID string,
	payload *models.IncomingHookPayload,
) (*models.ClaudeCodeHookPayload, error) {
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

	// Convert tool_input and tool_response if they exist
	toolInput, err := convertToJSONBData(payload.ToolInput)
	if err != nil {
		slog.With("error", err).Error("Failed to convert tool input")
	}

	toolResponse, err := convertToJSONBData(payload.ToolResponse)
	if err != nil {
		slog.With("error", err).Error("Failed to convert tool response")
	}

	return &models.ClaudeCodeHookPayload{
		UserID:         &userID,
		TeamID:         teamID,
		SessionID:      payload.SessionID,
		TranscriptPath: payload.TranscriptPath,
		CWD:            payload.CWD,
		HookEventName:  payload.HookEventName,
		ToolName:       payload.ToolName,
		ToolInput:      toolInput,
		ToolResponse:   toolResponse,
		Prompt:         payload.Prompt,
		Message:        payload.Message,
		Payload:        fullPayloadMap,
	}, nil
}

// checkClaudeResourceLimits checks session existence and resource limits for new sessions
func (s *Server) checkClaudeResourceLimits(
	w http.ResponseWriter,
	r *http.Request,
	userID, sessionID string,
) bool {
	claudeCodeRepo := s.container.ClaudeCodeHooksRepository()
	sessionExists, checkErr := claudeCodeRepo.SessionExists(r.Context(), userID, sessionID)
	if checkErr != nil {
		s.logger.With("error", checkErr).Error("Failed to check if session exists")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to check session", s.logger)
		return false
	}

	// If it's a new session, check resource limits
	if !sessionExists {
		if !s.checkClaudeSessionLimit(w, r, userID) {
			return false
		}

		claudeCount, countErr := claudeCodeRepo.CountUniqueSessions(r.Context(), userID)
		if countErr != nil {
			s.logger.With("error", countErr).Error("Failed to count Claude Code sessions")
			respondWithHookError(w, http.StatusInternalServerError, "Failed to check resource limits", s.logger)
			return false
		}

		if claudeCount == 0 {
			if !s.checkClaudeToolLimit(w, r, userID) {
				return false
			}
		}
	}
	return true
}

// parsePaginationParams parses page and limit query parameters
func parsePaginationParams(r *http.Request) (page, limit int) {
	page, limit = 1, 10 // defaults

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > 100 {
				limit = 100
			} else {
				limit = l
			}
		}
	}
	return page, limit
}

// respondWithJSON sends a JSON success response with HTTP 200 OK
func respondWithJSON(w http.ResponseWriter, data interface{}, logger *slog.Logger) {
	writeOK(w, data, logger)
}

// checkClaudeSessionAndCount checks if session exists and gets Claude Code session count
func (s *Server) checkClaudeSessionAndCount(
	ctx context.Context, repo repositories.ClaudeCodeHooksRepository, userID, sessionID string,
) (sessionExists bool, count int, err error) {
	sessionExists, err = repo.SessionExists(ctx, userID, sessionID)
	if err != nil {
		s.logger.With("error", err).Error("Failed to check if session exists")
		return false, 0, err
	}

	if !sessionExists {
		count, err = repo.CountUniqueSessions(ctx, userID)
		if err != nil {
			s.logger.With("error", err).Error("Failed to count Claude Code sessions")
			// Don't fail the request, just log the error
			return sessionExists, 0, nil
		}
	}

	return sessionExists, count, nil
}

// respondWithHookSuccess sends a successful hook creation response
func respondWithHookSuccess(w http.ResponseWriter, hookPayload interface{}, logger *slog.Logger) {
	type hookResponse struct {
		ID        int    `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	// Type assert to get fields
	var resp hookResponse
	switch v := hookPayload.(type) {
	case *models.ClaudeCodeHookPayload:
		resp.ID = v.ID
		resp.CreatedAt = v.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.UpdatedAt = v.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	case *models.CursorIDEHookPayload:
		resp.ID = v.ID
		resp.CreatedAt = v.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.UpdatedAt = v.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	writeCreated(w, map[string]any{
		"status":  "success",
		"message": "Hook payload stored successfully",
		"data":    resp,
	}, logger)
}

// fireAIToolSessionEvent fires an AI tool session created event
func (s *Server) fireAIToolSessionEvent(ctx context.Context, userID, sessionID, toolType string, isNewTool bool) {
	// Fetch user email for the event payload
	userRepo := s.container.UserRepository()
	user, userErr := userRepo.GetByID(ctx, userID)
	userEmail := ""
	if userErr != nil {
		s.logger.With("error", userErr).Warn("Failed to fetch user email for AI tool session event")
	} else {
		userEmail = user.Email
	}

	// Fire event
	eventManager := s.container.EventManager()
	if eventManager != nil {
		event := events.NewAIToolSessionCreatedEvent(userID, userEmail, sessionID, toolType, isNewTool)
		if publishErr := eventManager.Publish(ctx, event); publishErr != nil {
			s.logger.With("error", publishErr).Warn("Failed to publish AI tool session created event")
		} else {
			s.logger.With(
				"user_id", userID,
				"session_id", sessionID,
				"tool_type", toolType,
				"is_new_tool", isNewTool,
			).Info("AI tool session created event published successfully")
		}
	}
}

// processClaudeHookPayload handles the storage and post-processing of a hook payload
func (s *Server) processClaudeHookPayload(
	w http.ResponseWriter,
	r *http.Request,
	repo repositories.ClaudeCodeHooksRepository,
	userID, teamID string,
	payload *models.IncomingHookPayload,
	sessionExists bool,
	claudeCount int,
) {
	// Prepare hook payload for storage
	hookPayload, prepareErr := prepareClaudeHookPayload(userID, teamID, payload)
	if prepareErr != nil {
		s.logger.With("error", prepareErr).Error("Failed to prepare hook payload")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to process payload", s.logger)
		return
	}

	// Store the payload in repository
	if err := repo.Create(r.Context(), hookPayload); err != nil {
		s.logger.With("error", err).Error("Failed to store Claude Code hook payload")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to store hook payload", s.logger)
		return
	}

	slog.With(
		"id", hookPayload.ID,
		"session_id", payload.SessionID,
		"hook_event_name", payload.HookEventName,
		"tool_name", payload.ToolName,
	).Info("Claude Code hook payload stored successfully")

	// Record metrics
	if s.metrics != nil {
		toolName := ""
		if payload.ToolName != nil {
			toolName = *payload.ToolName
		}
		s.metrics.RecordAIToolsHooksCall(r.Context(), toolName)
	}

	// Fire AI tool session created event only if this is the first time using this tool
	if !sessionExists && claudeCount == 0 {
		isNewTool := true
		s.fireAIToolSessionEvent(r.Context(), userID, payload.SessionID, "claude_code_cli", isNewTool)
	}

	respondWithHookSuccess(w, hookPayload, s.logger)
}

// handleClaudeCodeHooksPost handles POST /api/v1/claude-code/hooks
func (s *Server) handleClaudeCodeHooksPost(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	// Decode and validate payload
	payload, err := s.decodeClaudeHookPayload(r)
	if err != nil {
		s.logger.With("error", err).Error("Failed to decode Claude Code hook payload")
		respondWithHookError(w, http.StatusBadRequest, "Invalid JSON payload", s.logger)
		return
	}

	if validationErr := validateClaudeHookPayload(payload); validationErr != nil {
		s.logger.Error(validationErr.Error())
		respondWithHookError(w, http.StatusBadRequest, validationErr.Error(), s.logger)
		return
	}

	// Check session and resource limits
	repo := s.container.ClaudeCodeHooksRepository()
	sessionExists, claudeCount, sessionCheckErr := s.checkClaudeSessionAndCount(
		r.Context(), repo, userID, payload.SessionID,
	)
	if sessionCheckErr != nil {
		respondWithHookError(w, http.StatusInternalServerError, "Failed to check session", s.logger)
		return
	}

	if !s.checkClaudeResourceLimits(w, r, userID, payload.SessionID) {
		return
	}

	// Get user's default team ID for resource creation
	teamID, teamErr := s.getUserDefaultTeamID(r.Context(), userID)
	if teamErr != nil {
		s.logger.With(
			"handler", "handleClaudeCodeHooksPost",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", teamErr),
		).
			Error("Failed to get user's default team")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to get user's team", s.logger)
		return
	}

	// Process and store the payload
	s.processClaudeHookPayload(w, r, repo, userID, teamID, payload, sessionExists, claudeCount)
}

// handleClaudeCodeHooksGet handles GET /api/v1/claude-code/hooks with pagination
func (s *Server) handleClaudeCodeHooksGet(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	page, limit := parsePaginationParams(r)

	// Build filters with REQUIRED user ID for security
	filters := repositories.ClaudeCodeHooksFilters{
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
	repo := s.container.ClaudeCodeHooksRepository()
	response, listErr := repo.List(r.Context(), filters)
	if listErr != nil {
		slog.With("error", listErr).Error("Failed to retrieve Claude Code hook payloads")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to retrieve hook payloads", s.logger)
		return
	}

	slog.With(
		"page", response.Page,
		"limit", response.Limit,
		"total", response.Total,
		"total_pages", response.TotalPages,
		"count", len(response.Data),
	).Info("Claude Code hook payloads retrieved successfully")

	respondWithJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "Hook payloads retrieved successfully",
		"data":    response,
	}, s.logger)
}

// handleClaudeCodeSessionsGet handles GET /api/v1/claude-code/sessions with pagination
func (s *Server) handleClaudeCodeSessionsGet(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	page, limit := parsePaginationParams(r)

	filters := repositories.SessionFilters{
		UserID: &userID,
		Page:   page,
		Limit:  limit,
	}

	repo := s.container.ClaudeCodeHooksRepository()
	response, getErr := repo.GetSessions(r.Context(), filters)
	if getErr != nil {
		slog.With("error", getErr).Error("Failed to retrieve Claude Code sessions")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to retrieve sessions", s.logger)
		return
	}

	slog.With(
		"page", response.Page,
		"limit", response.Limit,
		"total", response.Total,
		"total_pages", response.TotalPages,
		"count", len(response.Data),
	).Info("Claude Code sessions retrieved successfully")

	respondWithJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "Sessions retrieved successfully",
		"data":    response,
	}, s.logger)
}

// handleClaudeCodeSessionCountsGet handles GET /api/v1/ai-tools/claude-code/session-counts
func (s *Server) handleClaudeCodeSessionCountsGet(w http.ResponseWriter, r *http.Request) {

	// Get authenticated user ID from context - CRITICAL SECURITY FIX
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
	repo := s.container.ClaudeCodeHooksRepository()
	response, err := repo.GetSessionCounts(r.Context(), userID, days)
	if err != nil {
		slog.With("error", err).Error("Failed to retrieve Claude Code session counts")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to retrieve session counts", s.logger)
		return
	}

	slog.With(
		"total_sessions", response.TotalSessions,
		"range_days", days,
		"count_entries", len(response.Counts),
	).
		Info("Claude Code session counts retrieved successfully")

	writeOK(w, map[string]any{
		"status":  "success",
		"message": "Session counts retrieved successfully",
		"data":    response,
	}, s.logger)
}

// handleClaudeCodeOverviewStatsGet handles GET /api/v1/ai-tools/claude-code/overview-stats
func (s *Server) handleClaudeCodeOverviewStatsGet(w http.ResponseWriter, r *http.Request) {

	// Get authenticated user ID from context - CRITICAL SECURITY FIX
	userID := r.Context().Value(contextKeyUserID).(string)

	// Use repository to get overview stats with user filtering
	repo := s.container.ClaudeCodeHooksRepository()
	stats, err := repo.GetOverviewStats(r.Context(), userID)
	if err != nil {
		slog.With("error", err).Error("Failed to retrieve overview stats")
		http.Error(w, "Failed to retrieve stats", http.StatusInternalServerError)
		return
	}

	slog.With(
		"total_sessions", stats.TotalSessions,
		"sessions_this_week", stats.SessionsThisWeek,
		"weekly_trend_percent", stats.WeeklyTrendPercent,
		"avg_user_prompts_per_session", stats.AvgUserPromptsPerSession,
		"total_unique_tools", stats.TotalUniqueTools,
		"top_tools_count", len(stats.TopTools),
	).Info("Claude Code overview stats retrieved successfully")

	writeOK(w, map[string]any{
		"status":  "success",
		"message": "Overview stats retrieved successfully",
		"data":    stats,
	}, s.logger)
}

// handleClaudeCodeRecentActivitiesGet handles GET /api/v1/ai-tools/claude-code/recent-activities
func (s *Server) handleClaudeCodeRecentActivitiesGet(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	page, limit := parsePaginationParams(r)

	// Override default limit to 20 for recent activities
	if r.URL.Query().Get("limit") == "" {
		limit = 20
	}

	// Build filters
	filters := repositories.RecentActivitiesFilters{
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
	repo := s.container.ClaudeCodeHooksRepository()
	response, err := repo.GetRecentActivities(r.Context(), filters)
	if err != nil {
		slog.With("error", err).Error("Failed to retrieve recent Claude Code activities")
		respondWithHookError(w, http.StatusInternalServerError, "Failed to retrieve recent activities", s.logger)
		return
	}

	slog.With(
		"activities_count", len(response.Activities),
		"page", response.Page,
		"limit", response.Limit,
		"total", response.Total,
		"total_pages", response.TotalPages,
	).Info("Claude Code recent activities retrieved successfully")

	respondWithJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "Recent activities retrieved successfully",
		"data":    response,
	}, s.logger)
}

// handleClaudeCodeSessionDelete handles DELETE /api/v1/ai-tools/claude-code/sessions/{session_id}
func (s *Server) handleClaudeCodeSessionDelete(w http.ResponseWriter, r *http.Request) {

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
	repo := s.container.ClaudeCodeHooksRepository()
	err := repo.DeleteSession(r.Context(), userID, sessionID)
	if err != nil {
		s.logger.With("error", err).
			With(
				"user_id", userID,
				"session_id", sessionID,
			).
			Error("Failed to delete Claude Code session")

		// Check if it's a "not found" error
		if errors.Is(err, repositories.ErrHookSessionNotFound) {
			respondWithHookError(w, http.StatusNotFound, "Session not found or access denied", s.logger)
			return
		}

		respondWithHookError(w, http.StatusInternalServerError, "Failed to delete session", s.logger)
		return
	}

	s.logger.With(
		"user_id", userID,
		"session_id", sessionID,
	).Info("Claude Code session deleted successfully")

	writeNoContent(w)
}
