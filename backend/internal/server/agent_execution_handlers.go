package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
)

// handleGetExecutionStatus handles GET /api/v1/{team_id}/agents/executions/{id}/status
// Returns the current status and state of an agent execution
//
//nolint:funlen // Security validation requires additional lines
func (s *Server) handleGetExecutionStatus(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	executionID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetExecutionStatus",
		"user_id", userID,
		"team_id", teamID,
		"execution_id", executionID,
	).Info("Get execution status request received")

	// Get execution from repository
	execution, err := s.container.AgentExecutionRepository().GetByID(r.Context(), userID, executionID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetExecutionStatus",
			"user_id", userID,
			"team_id", teamID,
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get execution")

		// Check if execution not found or belongs to different user
		if errors.Is(err, repositories.ErrAgentExecutionNotFound) {
			writeErrorResponse(w, nil, "not_found", "Execution not found", http.StatusNotFound)
			return
		}

		writeErrorResponse(
			w,
			nil,
			"internal_error",
			"Failed to retrieve execution status",
			http.StatusInternalServerError,
		)
		return
	}

	// Verify that the execution's agent belongs to the specified team
	agent, err := s.container.AgentRepository().GetByID(r.Context(), userID, teamID, execution.AgentID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetExecutionStatus",
			"execution_id", executionID,
			"agent_id", execution.AgentID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Execution's agent not found in specified team")
		writeErrorResponse(w, nil, "not_found", "Execution not found", http.StatusNotFound)
		return
	}

	// Log what we're returning
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetExecutionStatus",
		"execution_id", executionID,
		"agent_id", agent.ID,
		"team_id", teamID,
		"status", execution.Status,
		"current_state", execution.CurrentState,
		"artifact_count", len(execution.Artifacts),
	).Info("Returning execution status")

	// Return full execution with all A2A fields
	writeOK(w, execution, s.logger)
}

// handleCancelExecution handles POST /api/v1/{team_id}/agents/executions/{execution_id}/cancel
// Cancels a running execution: aborts local streaming, cancels the remote task,
// and marks the execution cancelled.
func (s *Server) handleCancelExecution(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	executionID := chi.URLParam(r, "execution_id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCancelExecution",
		"user_id", userID,
		"team_id", teamID,
		"execution_id", executionID,
	).Info("Cancel execution request received")

	execution, err := s.container.AgentExecutionRepository().GetByID(r.Context(), userID, executionID)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentExecutionNotFound) {
			writeErrorResponse(w, nil, "not_found", "Execution not found", http.StatusNotFound)
			return
		}
		writeErrorResponse(w, nil, "internal_error", "Failed to retrieve execution", http.StatusInternalServerError)
		return
	}

	// Verify the execution's agent belongs to the specified team (cross-team 404).
	agent, err := s.container.AgentRepository().GetByID(r.Context(), userID, teamID, execution.AgentID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCancelExecution",
			"execution_id", executionID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Execution's agent not found in specified team")
		writeErrorResponse(w, nil, "not_found", "Execution not found", http.StatusNotFound)
		return
	}

	updated, err := s.container.AgentInvocationService().CancelExecution(r.Context(), execution, agent)
	if err != nil {
		if errors.Is(err, services.ErrExecutionNotCancelable) || errors.Is(err, a2a.ErrTaskNotCancelable) {
			writeErrorResponse(
				w, nil, "conflict",
				"Execution cannot be cancelled: it is already terminal or the task is not cancelable",
				http.StatusConflict,
			)
			return
		}
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCancelExecution",
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to cancel execution")
		writeErrorResponse(w, nil, "internal_error", "Failed to cancel execution", http.StatusInternalServerError)
		return
	}

	writeOK(w, updated, s.logger)
}

// handleGetExecutionEvents handles GET /api/v1/agents/executions/{id}/events
// Returns streaming events for an execution with cursor-based pagination
// Supports both ?since={sequence} (for polling) and ?page={page} (for historical viewing)
// handleCursorBasedPolling handles real-time event polling with cursor
func (s *Server) handleCursorBasedPolling(
	w http.ResponseWriter,
	r *http.Request,
	execution *models.AgentExecution,
	sinceParam string,
) {
	executionID := execution.ID
	since := 0
	if parsedSince, parseErr := strconv.Atoi(sinceParam); parseErr == nil && parsedSince >= 0 {
		since = parsedSince
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetExecutionEvents",
		"execution_id", executionID,
		"since", since,
	).Info("Polling events since sequence")

	// Get events after the specified sequence number
	events, listErr := s.container.AgentExecutionEventRepository().ListAfterSequence(r.Context(), execution.ID, since)
	if listErr != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetExecutionEvents",
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", listErr),
		).Error("Failed to get events after sequence")

		writeErrorResponse(w, nil, "internal_error", "Failed to retrieve events", http.StatusInternalServerError)
		return
	}

	// Calculate the cursor for the next poll. The repository filters
	// `sequence_number > since`, so the cursor must be the last sequence we
	// returned (NOT lastSeq+1) — otherwise re-polling with this value as `since`
	// skips event lastSeq+1 (#1704).
	nextSequence := since
	if len(events) > 0 {
		nextSequence = events[len(events)-1].SequenceNumber
	}

	// Build response for polling
	response := map[string]interface{}{
		"execution_id":  execution.ID,
		"status":        execution.Status,
		"current_state": execution.CurrentState,
		"events":        events,
		"has_more":      execution.Status == "pending" || execution.Status == "running",
		"next_sequence": nextSequence,
	}

	writeOK(w, response, s.logger)
}

// handlePageBasedPagination handles traditional page-based event pagination
func (s *Server) handlePageBasedPagination(
	w http.ResponseWriter,
	r *http.Request,
	execution *models.AgentExecution,
	userID string,
) {
	executionID := execution.ID
	page := 1
	if pageParam := r.URL.Query().Get("page"); pageParam != "" {
		if p, pageErr := strconv.Atoi(pageParam); pageErr == nil && p > 0 {
			page = p
		}
	}

	limit := 50
	if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
		if l, limitErr := strconv.Atoi(limitParam); limitErr == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	offset := (page - 1) * limit

	// Get events from repository
	events, totalCount, err := s.container.AgentExecutionEventRepository().ListByExecutionID(
		r.Context(), execution.ID, limit, offset,
	)
	if err != nil {
		s.logger.With(
			"user_id", userID,
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		).
			Error("Failed to get execution events")
		writeErrorResponse(
			w,
			nil,
			"internal_error",
			"Failed to retrieve execution events",
			http.StatusInternalServerError,
		)
		return
	}

	// Calculate total pages
	totalPages := (totalCount + limit - 1) / limit

	// Build response
	response := map[string]interface{}{
		"events":      events,
		"total_count": totalCount,
		"page":        page,
		"per_page":    limit,
		"total_pages": totalPages,
	}

	writeOK(w, response, s.logger)
}

func (s *Server) handleGetExecutionEvents(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	executionID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetExecutionEvents",
		"user_id", userID,
		"team_id", teamID,
		"execution_id", executionID,
	).Info("Get execution events request received")

	// Verify execution belongs to user
	execution, err := s.container.AgentExecutionRepository().GetByID(r.Context(), userID, executionID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetExecutionEvents",
			"user_id", userID,
			"team_id", teamID,
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get execution")

		if errors.Is(err, repositories.ErrAgentExecutionNotFound) {
			writeErrorResponse(w, nil, "not_found", "Execution not found", http.StatusNotFound)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to retrieve execution", http.StatusInternalServerError)
		return
	}

	// Verify that the execution's agent belongs to the specified team
	_, err = s.container.AgentRepository().GetByID(r.Context(), userID, teamID, execution.AgentID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetExecutionEvents",
			"execution_id", executionID,
			"agent_id", execution.AgentID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Execution's agent not found in specified team")
		writeErrorResponse(w, nil, "not_found", "Execution not found", http.StatusNotFound)
		return
	}

	// Check if using cursor-based polling (since parameter)
	sinceParam := r.URL.Query().Get("since")
	if sinceParam != "" {
		s.handleCursorBasedPolling(w, r, execution, sinceParam)
		return
	}

	// Traditional page-based pagination for historical viewing
	s.handlePageBasedPagination(w, r, execution, userID)
}

// handleGetConversationExecutions handles GET /api/v1/{team_id}/agents/conversations/{conversation_id}/executions
// Returns executions in a conversation with pagination support
//
//nolint:funlen // Security validation and pagination logic requires additional lines
func (s *Server) handleGetConversationExecutions(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	conversationID := chi.URLParam(r, "conversation_id")

	// Parse query parameters
	limit := 50 // default
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 200 {
			limit = parsedLimit
		}
	}

	var before *time.Time
	if beforeStr := r.URL.Query().Get("before"); beforeStr != "" {
		if parsedTime, err := time.Parse(time.RFC3339, beforeStr); err == nil {
			before = &parsedTime
		}
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetConversationExecutions",
		"user_id", userID,
		"team_id", teamID,
		"conversation_id", conversationID,
		"limit", limit,
		"before", before,
	).Info("Get conversation executions request received")

	// Get executions by conversation with pagination
	executions, hasMore, totalCount, err := s.container.AgentExecutionRepository().GetByConversationID(
		r.Context(),
		userID,
		conversationID,
		limit,
		before,
	)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetConversationExecutions",
			"user_id", userID,
			"team_id", teamID,
			"conversation_id", conversationID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get conversation executions")

		writeErrorResponse(w, nil, "internal_error", "Failed to retrieve conversation", http.StatusInternalServerError)
		return
	}

	// Verify team ownership: get the first execution and validate its agent belongs to the team
	if len(executions) > 0 {
		_, err = s.container.AgentRepository().GetByID(r.Context(), userID, teamID, executions[0].AgentID)
		if err != nil {
			s.logger.With(
				"service", "vibexp-api",
				"handler", "handleGetConversationExecutions",
				"conversation_id", conversationID,
				"agent_id", executions[0].AgentID,
				"team_id", teamID,
				"error", fmt.Sprintf("%+v", err),
			).Warn("Conversation's agent not found in specified team")
			writeErrorResponse(w, nil, "not_found", "Conversation not found", http.StatusNotFound)
			return
		}
	}

	// Return executions with pagination info
	response := map[string]interface{}{
		"executions":      executions,
		"conversation_id": conversationID,
		"has_more":        hasMore,
		"total_count":     totalCount,
		"count":           len(executions),
	}

	writeOK(w, response, s.logger)
}

// handleListAgentConversations handles GET /api/v1/{team_id}/agents/{id}/conversations
// Returns paginated conversation summaries for an agent
//
//nolint:funlen // Security validation and pagination logic requires additional lines
func (s *Server) handleListAgentConversations(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id")
	agentID := chi.URLParam(r, "id")

	// Parse pagination parameters
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if parsedPage, err := strconv.Atoi(pageStr); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}

	limit := 20 // default
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleListAgentConversations",
		"user_id", userID,
		"team_id", teamID,
		"agent_id", agentID,
		"page", page,
		"limit", limit,
	).Info("List agent conversations request received")

	// Verify agent belongs to the specified team
	_, err := s.container.AgentRepository().GetByID(r.Context(), userID, teamID, agentID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListAgentConversations",
			"user_id", userID,
			"team_id", teamID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Agent not found in specified team")
		writeErrorResponse(w, nil, "not_found", "Agent not found", http.StatusNotFound)
		return
	}

	// Get conversation summaries
	conversations, totalCount, err := s.container.AgentExecutionRepository().ListConversations(
		r.Context(),
		userID,
		agentID,
		page,
		limit,
	)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListAgentConversations",
			"user_id", userID,
			"team_id", teamID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list conversations")

		writeErrorResponse(w, nil, "internal_error", "Failed to retrieve conversations", http.StatusInternalServerError)
		return
	}

	// Build response
	totalPages := (totalCount + limit - 1) / limit
	response := models.ConversationListResponse{
		Conversations: conversations,
		TotalCount:    totalCount,
		Page:          page,
		PerPage:       limit,
		TotalPages:    totalPages,
	}

	writeOK(w, response, s.logger)
}
