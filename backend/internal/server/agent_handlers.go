package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services"
	"github.com/vibexp/vibexp/internal/services/activities"
)

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateAgent",
		"user_id", userID,
		"team_id", teamID,
	).
		Info("Create agent request received")

	var req models.CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logDecodeError(w, "handleCreateAgent", userID, "", err)
		return
	}

	if !s.validateCreateAgentRequest(w, r, &req) || !s.checkAgentResourceLimit(w, r, r.Context(), userID) {
		return
	}

	agent, err := s.container.AgentService().CreateAgent(r.Context(), userID, teamID, &req)
	if err != nil {
		s.handleAgentCreationError(w, r, userID, err)
		return
	}

	s.recordAgentActivity(
		r.Context(), userID, activities.ActivityTypeAgentCreated,
		agent.ID, "Created new agent: "+req.Name, r,
	)
	writeCreated(w, agent, s.logger)
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	agentID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetAgent",
		"user_id", userID,
		"team_id", teamID,
		"agent_id", agentID,
	).Info("Get agent request received")

	agent, err := s.container.AgentService().GetAgentByID(r.Context(), userID, teamID, agentID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetAgent",
			"user_id", userID,
			"team_id", teamID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get agent")

		if errors.Is(err, repositories.ErrAgentNotFound) {
			writeErrorResponse(w, nil, "not_found", "Agent not found", http.StatusNotFound)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to get agent", http.StatusInternalServerError)
		return
	}

	contextkeys.SetAccessedResourceID(r.Context(), agent.ID)

	writeOK(w, agent, s.logger)
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleListAgents",
		"user_id", userID,
		"team_id", teamID,
	).
		Info("List agents request received")

	filters, ok := parseAgentFilters(w, r, teamID)
	if !ok {
		return
	}

	response, err := s.container.AgentService().ListAgents(r.Context(), userID, filters)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListAgents",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list agents")
		writeErrorResponse(w, nil, "internal_error", "Failed to list agents", http.StatusInternalServerError)
		return
	}

	writeOK(w, response, s.logger)
}

// allowedAgentSortFields contains the allowlisted sort fields for agents
var allowedAgentSortFields = map[string]bool{
	"name": true, "status": true, "updated_at": true, "created_at": true,
	"last_run": true, "success_rate": true,
}

// parseAgentFilters parses query parameters into AgentFilters
// Returns (filters, ok) where ok is false if a validation error was written to w
func parseAgentFilters(w http.ResponseWriter, r *http.Request, teamID string) (services.AgentFilters, bool) {
	query := r.URL.Query()

	sortBy := query.Get("sort_by")
	if sortBy != "" && !allowedAgentSortFields[sortBy] {
		writeErrorResponse(w, nil, "validation_error", "invalid sort_by value: "+sortBy, http.StatusBadRequest)
		return services.AgentFilters{}, false
	}

	// Parse and validate pagination parameters with bounds checking
	pagination := validatePaginationParams(query.Get("page"), query.Get("limit"))

	filters := services.AgentFilters{
		Status:    query.Get("status"),
		Search:    query.Get("search"),
		TeamID:    teamID,
		SortBy:    sortBy,
		SortOrder: strings.ToLower(query.Get("sort_order")),
		Page:      pagination.Page,
		Limit:     pagination.Limit,
	}

	return filters, true
}

func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	agentID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateAgent",
		"user_id", userID,
		"team_id", teamID,
		"agent_id", agentID,
	).Info("Update agent request received")

	var req models.UpdateAgentRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		s.logDecodeError(w, "handleUpdateAgent", userID, agentID, decodeErr)
		return
	}

	if !s.validateUpdateAgentRequest(w, r, &req) {
		return
	}

	agent, err := s.container.AgentService().UpdateAgent(r.Context(), userID, teamID, agentID, &req)
	if err != nil {
		// Handle team reassignment errors
		if strings.Contains(err.Error(), "cannot be moved between teams") {
			writeErrorResponse(w, nil, "bad_request",
				"Agents cannot be transferred or re-associated with other teams once created",
				http.StatusBadRequest)
			return
		}
		s.handleUpdateAgentError(w, userID, agentID, &req, err)
		return
	}

	s.recordAgentActivity(
		r.Context(), userID, activities.ActivityTypeAgentUpdated,
		agentID, "Updated agent: "+agent.Name, r,
	)

	writeOK(w, agent, s.logger)
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	agentID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleDeleteAgent",
		"user_id", userID,
		"team_id", teamID,
		"agent_id", agentID,
	).Info("Delete agent request received")

	// Get agent name for activity logging
	agent, err := s.container.AgentService().GetAgentByID(r.Context(), userID, teamID, agentID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteAgent",
			"user_id", userID,
			"team_id", teamID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get agent for deletion")

		if errors.Is(err, repositories.ErrAgentNotFound) {
			writeErrorResponse(w, nil, "not_found", "Agent not found", http.StatusNotFound)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to delete agent", http.StatusInternalServerError)
		return
	}

	err = s.container.AgentService().DeleteAgent(r.Context(), userID, teamID, agentID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleDeleteAgent",
			"user_id", userID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to delete agent")

		if errors.Is(err, repositories.ErrAgentNotFound) {
			writeErrorResponse(w, nil, "not_found", "Agent not found", http.StatusNotFound)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to delete agent", http.StatusInternalServerError)
		return
	}

	// Record activity for agent deletion
	s.recordAgentActivity(
		r.Context(), userID, activities.ActivityTypeAgentDeleted, agentID,
		"Deleted agent: "+agent.Name, r,
	)

	writeNoContent(w)
}

//nolint:funlen // structured slog attributes are marginally more verbose than the prior logrus WithFields calls
func (s *Server) handleUpdateAgentCredentials(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	agentID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateAgentCredentials",
		"user_id", userID,
		"team_id", teamID,
		"agent_id", agentID,
	).Info("Update agent credentials request received")

	var req models.UpdateAgentCredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleUpdateAgentCredentials",
			"user_id", userID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to decode request body")
		writeErrorResponse(w, nil, "invalid_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate the request
	if err := validate.Struct(&req); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleUpdateAgentCredentials",
			"user_id", userID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Validation failed for update agent credentials request")
		writeErrorResponse(w, nil, "validation_error", "Validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.container.AgentService().UpdateAgentCredentials(r.Context(), userID, teamID, agentID, &req); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleUpdateAgentCredentials",
			"user_id", userID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to update agent credentials")

		if errors.Is(err, repositories.ErrAgentNotFound) {
			writeErrorResponse(w, nil, "not_found", "Agent not found", http.StatusNotFound)
			return
		}

		writeErrorResponse(
			w,
			nil,
			"internal_error",
			"Failed to update agent credentials",
			http.StatusInternalServerError,
		)
		return
	}

	// Record activity for credentials update
	s.recordAgentActivity(
		r.Context(), userID, activities.ActivityTypeAgentUpdated, agentID,
		"Updated agent credentials", r,
	)

	writeNoContent(w)
}

func (s *Server) handleGetAgentStats(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetAgentStats",
		"user_id", userID,
		"team_id", teamID,
	).
		Info("Get agent stats request received")

	stats, err := s.container.AgentService().GetAgentStats(r.Context(), userID, teamID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetAgentStats",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get agent stats")
		writeErrorResponse(w, nil, "internal_error", "Failed to get agent stats", http.StatusInternalServerError)
		return
	}

	writeOK(w, stats, s.logger)
}

func (s *Server) handleStartAgentExecution(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	teamID := chi.URLParam(r, "team_id") // Already validated by middleware
	agentID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleStartAgentExecution",
		"user_id", userID,
		"team_id", teamID,
		"agent_id", agentID,
	).Info("Start agent execution request received")

	var req models.CreateAgentExecutionRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleStartAgentExecution",
			"user_id", userID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", decodeErr),
		).Error("Failed to decode request body")
		writeErrorResponse(w, nil, "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	req.AgentID = agentID

	execution, err := s.container.AgentService().StartExecution(r.Context(), userID, teamID, agentID, &req)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleStartAgentExecution",
			"user_id", userID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to start agent execution")

		if errors.Is(err, repositories.ErrAgentNotFound) {
			writeErrorResponse(w, nil, "not_found", "Agent not found", http.StatusNotFound)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to start execution", http.StatusInternalServerError)
		return
	}

	// Record activity for execution start
	s.recordAgentActivity(
		r.Context(), userID, activities.ActivityTypeAgentExecutionStarted, agentID,
		"Started execution for agent", r,
	)

	writeCreated(w, execution, s.logger)
}

func (s *Server) handleCompleteAgentExecution(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	executionID := chi.URLParam(r, "execution_id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCompleteAgentExecution",
		"user_id", userID,
		"execution_id", executionID,
	).Info("Complete agent execution request received")

	var req models.UpdateAgentExecutionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCompleteAgentExecution",
			"user_id", userID,
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to decode request body")
		writeErrorResponse(w, nil, "bad_request", "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Status != "running" && req.Status != "success" && req.Status != "error" {
		writeErrorResponse(w, nil, "validation_error",
			"Status must be 'running', 'success', or 'error'", http.StatusBadRequest)
		return
	}

	execution, err := s.container.AgentService().CompleteExecution(r.Context(), userID, executionID, &req)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleCompleteAgentExecution",
			"user_id", userID,
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to complete agent execution")
		if errors.Is(err, repositories.ErrAgentExecutionNotFound) {
			writeErrorResponse(w, nil, "not_found", "Execution not found", http.StatusNotFound)
			return
		}
		writeErrorResponse(w, nil, "internal_error", "Failed to complete execution", http.StatusInternalServerError)
		return
	}

	activityType := activities.ActivityTypeAgentExecutionCompleted
	if req.Status == "error" {
		activityType = activities.ActivityTypeAgentExecutionFailed
	}
	s.recordAgentActivity(r.Context(), userID, activityType, execution.AgentID, "Execution "+req.Status, r)

	writeOK(w, execution, s.logger)
}

func (s *Server) handleListAgentExecutions(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	agentID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleListAgentExecutions",
		"user_id", userID,
		"agent_id", agentID,
	).Info("List agent executions request received")

	filters := s.buildAgentExecutionFilters(r, agentID)

	executions, totalCount, err := s.container.AgentService().ListExecutions(r.Context(), userID, filters)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleListAgentExecutions",
			"user_id", userID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list agent executions")
		writeErrorResponse(w, nil, "internal_error", "Failed to list executions", http.StatusInternalServerError)
		return
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(filters.Limit)))

	response := map[string]interface{}{
		"executions":  executions,
		"total_count": totalCount,
		"page":        filters.Page,
		"per_page":    filters.Limit,
		"total_pages": totalPages,
	}

	writeOK(w, response, s.logger)
}

func (s *Server) handleGetAgentExecution(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	executionID := chi.URLParam(r, "execution_id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleGetAgentExecution",
		"user_id", userID,
		"execution_id", executionID,
	).Info("Get agent execution request received")

	execution, err := s.container.AgentService().GetExecution(r.Context(), userID, executionID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "handleGetAgentExecution",
			"user_id", userID,
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get agent execution")

		if errors.Is(err, repositories.ErrAgentExecutionNotFound) {
			writeErrorResponse(w, nil, "not_found", "Execution not found", http.StatusNotFound)
			return
		}

		writeErrorResponse(w, nil, "internal_error", "Failed to get execution", http.StatusInternalServerError)
		return
	}

	writeOK(w, execution, s.logger)
}

// handlePreviewAgentCard fetches and returns agent card details without creating an agent
func (s *Server) handlePreviewAgentCard(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handlePreviewAgentCard",
		"user_id", userID,
	).
		Info("Preview agent card request received")

	var req struct {
		CardURL string `json:"card_url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logDecodeError(w, "handlePreviewAgentCard", userID, "", err)
		return
	}

	if req.CardURL == "" {
		writeErrorResponse(w, nil, "validation_error", "Agent card URL is required", http.StatusBadRequest)
		return
	}

	// Preview happens before any agent (and its credentials) exists, so the card is
	// fetched without authentication.
	agentCard, err := s.container.AgentCardFetcher().FetchAgentCard(r.Context(), req.CardURL, nil)
	if err != nil {
		s.handleAgentCardFetchError(w, userID, req.CardURL, err)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handlePreviewAgentCard",
		"user_id", userID,
		"card_url", req.CardURL,
		"name", agentCard.Name,
	).Info("Successfully previewed agent card")

	writeOK(w, agentCard, s.logger)
}

// handleExecuteAgent handles agent execution requests
// For streaming-capable agents:
//   - Returns immediately with status='pending' or 'submitted'
//   - Frontend should poll GET /agents/executions/{id}/status for updates
//   - Frontend can fetch GET /agents/executions/{id}/events for streaming events
//
// For non-streaming agents:
//   - Waits for execution to complete
//   - Returns final result with status='success' or 'error'
func (s *Server) handleExecuteAgent(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(contextKeyUserID).(string)
	agentID := chi.URLParam(r, "id")

	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleExecuteAgent",
		"user_id", userID,
		"agent_id", agentID,
	).
		Info("Execute agent request received")

	var req struct {
		Input          map[string]interface{} `json:"input"`
		ConversationID *string                `json:"conversation_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logDecodeError(w, "handleExecuteAgent", userID, agentID, err)
		return
	}

	execution, err := s.container.AgentInvocationService().InvokeAgent(
		r.Context(), userID, agentID, req.Input, req.ConversationID,
	)
	if err != nil {
		s.handleExecuteAgentError(w, userID, agentID, err)
		return
	}

	activityType := activities.ActivityTypeAgentExecutionCompleted
	if execution.Status == "error" {
		activityType = activities.ActivityTypeAgentExecutionFailed
	}
	s.recordAgentActivity(r.Context(), userID, activityType, agentID, "Executed agent", r)

	writeOK(w, execution, s.logger)
}

// Helper function to record agent-related activities
func (s *Server) recordAgentActivity(
	ctx context.Context, userID, activityType, agentID, description string, r *http.Request,
) {
	ar := NewActivityRecorder(s.activityService)
	ar.RecordResourceActivity(ctx, userID, activityType, activities.EntityTypeAgent, &agentID, description, nil, r)
}

// validateCreateAgentRequest validates the create agent request
func (s *Server) validateCreateAgentRequest(
	w http.ResponseWriter, _ *http.Request, req *models.CreateAgentRequest,
) bool {
	if req.CardURL == "" {
		writeErrorResponse(w, nil, "validation_error",
			"Agent card URL is required. Please provide a valid agent card URL "+
				"(e.g., http://localhost:8000/.well-known/agent-card.json)",
			http.StatusBadRequest,
		)
		return false
	}

	if req.Status != "" && req.Status != "active" && req.Status != "paused" {
		writeErrorResponse(
			w,
			nil,
			"validation_error",
			"Status must be either 'active' or 'paused'",
			http.StatusBadRequest,
		)
		return false
	}

	return true
}

// checkAgentResourceLimit checks if the user has reached their agent resource limit
func (s *Server) checkAgentResourceLimit(
	w http.ResponseWriter, _ *http.Request, ctx context.Context, userID string,
) bool {
	allowed, err := s.container.ResourceUsageService().CheckResourceLimit(ctx, userID, "agent")
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "checkAgentResourceLimit",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to check resource limit")
		writeErrorResponse(w, nil, "internal_error", "Failed to check resource limit", http.StatusInternalServerError)
		return false
	}

	if !allowed {
		s.logger.With(
			"service", "vibexp-api",
			"handler", "checkAgentResourceLimit",
			"user_id", userID,
			"resource_type", "agent",
		).Warn("User has reached their agent limit")
		writeErrorResponse(w, nil, "resource_limit_exceeded",
			"You have reached the maximum number of agents allowed for your subscription plan",
			http.StatusForbidden,
		)
		return false
	}

	return true
}

// handleAgentCreationError handles errors from agent creation
func (s *Server) handleAgentCreationError(w http.ResponseWriter, _ *http.Request, userID string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleCreateAgent",
		"user_id", userID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to create agent")

	errMsg := err.Error()

	if s.isAgentCardNotFoundError(errMsg) {
		writeErrorResponse(w, nil, "agent_card_not_found", errMsg, http.StatusBadRequest)
		return
	}
	if s.isAgentCardUnauthorizedError(errMsg) {
		writeErrorResponse(w, nil, "agent_card_unauthorized", errMsg, http.StatusBadRequest)
		return
	}
	if s.isAgentCardForbiddenError(errMsg) {
		writeErrorResponse(w, nil, "agent_card_forbidden", errMsg, http.StatusBadRequest)
		return
	}
	if s.isInvalidAgentCardURLError(errMsg) {
		writeErrorResponse(w, nil, "invalid_agent_card_url", errMsg, http.StatusBadRequest)
		return
	}
	if s.isInvalidAgentCardFormatError(errMsg) {
		writeErrorResponse(w, nil, "invalid_agent_card", errMsg, http.StatusBadRequest)
		return
	}
	if s.isAgentCardNetworkError(errMsg) {
		writeErrorResponse(w, nil, "agent_card_network_error", errMsg, http.StatusBadGateway)
		return
	}
	if s.isAgentCardServerError(errMsg) {
		writeErrorResponse(w, nil, "agent_card_server_error", errMsg, http.StatusBadGateway)
		return
	}
	if isAgentNameConflict(err) {
		writeErrorResponse(w, nil, "conflict", errMsg, http.StatusConflict)
		return
	}

	writeErrorResponse(w, nil, "internal_error", errMsg, http.StatusInternalServerError)
}

// Agent card error checking helper functions
func (s *Server) isAgentCardNotFoundError(errMsg string) bool {
	return strings.Contains(errMsg, "agent card not found") || strings.Contains(errMsg, "returned 404 Not Found")
}

func (s *Server) isAgentCardUnauthorizedError(errMsg string) bool {
	return strings.Contains(errMsg, "unauthorized access") || strings.Contains(errMsg, "returned 401")
}

func (s *Server) isAgentCardForbiddenError(errMsg string) bool {
	return strings.Contains(errMsg, "access forbidden") || strings.Contains(errMsg, "returned 403")
}

func (s *Server) isInvalidAgentCardURLError(errMsg string) bool {
	return strings.Contains(errMsg, "invalid agent card URL format") || strings.Contains(errMsg, "invalid URL scheme")
}

func (s *Server) isInvalidAgentCardFormatError(errMsg string) bool {
	return strings.Contains(errMsg, "invalid JSON format") || strings.Contains(errMsg, "invalid agent card format")
}

func (s *Server) isAgentCardNetworkError(errMsg string) bool {
	return strings.Contains(errMsg, "request timeout") || strings.Contains(errMsg, "network error")
}

func (s *Server) isAgentCardServerError(errMsg string) bool {
	return strings.Contains(errMsg, "server error") || strings.Contains(errMsg, "returned 500") ||
		strings.Contains(errMsg, "service unavailable") || strings.Contains(errMsg, "returned 503")
}

// isAgentNameConflict reports whether err is (or wraps) a duplicate agent-name
// conflict from the repository layer. Used by both create and update handlers so
// the 409 branch is reachable through the service's %w-wrapping (an exact-string
// compare against the unwrapped message never matched — see #1704).
func isAgentNameConflict(err error) bool {
	return errors.Is(err, repositories.ErrAgentNameConflict)
}

// logDecodeError logs a request body decode error and writes error response
func (s *Server) logDecodeError(
	w http.ResponseWriter,
	handler, userID, resourceID string,
	err error,
) {
	fields := []any{"service", "vibexp-api", "handler", handler, "user_id", userID, "error", fmt.Sprintf("%+v", err)}
	if resourceID != "" {
		fields = append(fields, "resource_id", resourceID)
	}
	s.logger.With(fields...).Error("Failed to decode request body")
	writeErrorResponse(w, nil, "bad_request", "Invalid request body", http.StatusBadRequest)
}

// validateUpdateAgentRequest validates the update agent request
func (s *Server) validateUpdateAgentRequest(
	w http.ResponseWriter, r *http.Request, req *models.UpdateAgentRequest,
) bool {
	if req.CardURL == nil || *req.CardURL == "" {
		if !s.validateAgentNameField(w, r, req.Name) {
			return false
		}
		if !s.validateAgentDescriptionField(w, req.Description) {
			return false
		}
	}

	if req.Status != nil && *req.Status != "active" && *req.Status != "paused" && *req.Status != "error" {
		writeErrorResponse(
			w,
			nil,
			"validation_error",
			"Status must be 'active', 'paused', or 'error'",
			http.StatusBadRequest,
		)
		return false
	}

	return true
}

// validateAgentNameField validates the agent name field
func (s *Server) validateAgentNameField(w http.ResponseWriter, _ *http.Request, name *string) bool {
	if name != nil && *name == "" {
		writeErrorResponse(w, nil, "validation_error", "Name cannot be empty", http.StatusBadRequest)
		return false
	}

	if name != nil && len(*name) > 100 {
		writeErrorResponse(
			w,
			nil,
			"validation_error",
			"Name cannot be longer than 100 characters",
			http.StatusBadRequest,
		)
		return false
	}

	return true
}

// validateAgentDescriptionField validates the agent description field
func (s *Server) validateAgentDescriptionField(w http.ResponseWriter, description *string) bool {
	if description != nil && *description == "" {
		writeErrorResponse(w, nil, "validation_error", "Description cannot be empty", http.StatusBadRequest)
		return false
	}

	if description != nil && len(*description) > 500 {
		writeErrorResponse(
			w, nil, "validation_error",
			"Description cannot be longer than 500 characters",
			http.StatusBadRequest,
		)
		return false
	}

	return true
}

// handleUpdateAgentError handles errors from agent update
func (s *Server) handleUpdateAgentError(
	w http.ResponseWriter,
	userID, agentID string,
	_ *models.UpdateAgentRequest,
	err error,
) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleUpdateAgent",
		"user_id", userID,
		"agent_id", agentID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to update agent")

	if errors.Is(err, repositories.ErrAgentNotFound) {
		writeErrorResponse(w, nil, "not_found", "Agent not found", http.StatusNotFound)
		return
	}

	if isAgentNameConflict(err) {
		writeErrorResponse(w, nil, "conflict", err.Error(), http.StatusConflict)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to update agent", http.StatusInternalServerError)
}

// handleAgentCardFetchError handles errors from agent card fetch for preview
func (s *Server) handleAgentCardFetchError(w http.ResponseWriter, userID, cardURL string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handlePreviewAgentCard",
		"user_id", userID,
		"card_url", cardURL,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to fetch agent card for preview")

	errMsg := err.Error()

	if s.isAgentCardNotFoundError(errMsg) {
		writeErrorResponse(w, nil, "agent_card_not_found", errMsg, http.StatusBadRequest)
		return
	}
	if s.isAgentCardUnauthorizedError(errMsg) {
		writeErrorResponse(w, nil, "agent_card_unauthorized", errMsg, http.StatusBadRequest)
		return
	}
	if s.isAgentCardForbiddenError(errMsg) {
		writeErrorResponse(w, nil, "agent_card_forbidden", errMsg, http.StatusBadRequest)
		return
	}
	if s.isInvalidAgentCardURLError(errMsg) || strings.Contains(errMsg, "invalid URL path") {
		writeErrorResponse(w, nil, "invalid_agent_card_url", errMsg, http.StatusBadRequest)
		return
	}
	if s.isInvalidAgentCardFormatError(errMsg) {
		writeErrorResponse(w, nil, "invalid_agent_card", errMsg, http.StatusBadRequest)
		return
	}
	if s.isAgentCardNetworkError(errMsg) {
		writeErrorResponse(w, nil, "agent_card_network_error", errMsg, http.StatusBadGateway)
		return
	}
	if s.isAgentCardServerError(errMsg) {
		writeErrorResponse(w, nil, "agent_card_server_error", errMsg, http.StatusBadGateway)
		return
	}

	writeErrorResponse(w, nil, "agent_card_fetch_error", errMsg, http.StatusBadGateway)
}

// buildAgentExecutionFilters builds agent execution filters from request
func (s *Server) buildAgentExecutionFilters(r *http.Request, agentID string) services.AgentExecutionFilters {
	status := r.URL.Query().Get("status")
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 10
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	return services.AgentExecutionFilters{
		AgentID:  s.stringPtrIfNotEmpty(agentID),
		Status:   s.stringPtrIfNotEmpty(status),
		DateFrom: s.stringPtrIfNotEmpty(dateFrom),
		DateTo:   s.stringPtrIfNotEmpty(dateTo),
		Page:     page,
		Limit:    limit,
	}
}

// stringPtrIfNotEmpty returns a pointer to the string if it's not empty, nil otherwise
func (s *Server) stringPtrIfNotEmpty(str string) *string {
	if str != "" {
		return &str
	}
	return nil
}

// handleExecuteAgentError handles errors from agent execution
func (s *Server) handleExecuteAgentError(w http.ResponseWriter, userID, agentID string, err error) {
	s.logger.With(
		"service", "vibexp-api",
		"handler", "handleExecuteAgent",
		"user_id", userID,
		"agent_id", agentID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Failed to execute agent")

	errMsg := err.Error()
	if strings.Contains(errMsg, "agent not found") {
		writeErrorResponse(w, nil, "not_found", "Agent not found", http.StatusNotFound)
		return
	}
	if strings.Contains(errMsg, "agent is not active") {
		writeErrorResponse(w, nil, "agent_not_active", errMsg, http.StatusBadRequest)
		return
	}

	writeErrorResponse(w, nil, "internal_error", "Failed to execute agent", http.StatusInternalServerError)
}
