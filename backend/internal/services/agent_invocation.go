package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// AgentInvocationServiceInterface defines the interface for agent invocation
type AgentInvocationServiceInterface interface {
	InvokeAgent(
		ctx context.Context, userID, agentID string,
		input map[string]interface{}, conversationID *string,
	) (*models.AgentExecution, error)
}

// AgentInvocationService handles agent invocation logic
type AgentInvocationService struct {
	agentRepo       repositories.AgentRepository
	executionRepo   repositories.AgentExecutionRepository
	a2aClient       A2AHTTPClientInterface
	streamProcessor A2AStreamProcessorInterface
	logger          *slog.Logger
}

// NewAgentInvocationService creates a new agent invocation service
func NewAgentInvocationService(
	agentRepo repositories.AgentRepository,
	executionRepo repositories.AgentExecutionRepository,
	a2aClient A2AHTTPClientInterface,
	streamProcessor A2AStreamProcessorInterface,
	logger *slog.Logger,
) *AgentInvocationService {
	return &AgentInvocationService{
		agentRepo:       agentRepo,
		executionRepo:   executionRepo,
		a2aClient:       a2aClient,
		streamProcessor: streamProcessor,
		logger:          logger,
	}
}

// InvokeAgent invokes an agent and returns the execution result
// If conversationID is provided, it will continue an existing conversation by:
// 1. Looking up the first execution in the conversation to get the context_id
// 2. Including the context_id in the A2A message to maintain conversation continuity
func (s *AgentInvocationService) InvokeAgent(
	ctx context.Context,
	userID, agentID string,
	input map[string]interface{},
	conversationID *string,
) (*models.AgentExecution, error) {
	// 1. Get and validate agent
	agent, err := s.getAndValidateAgent(ctx, userID, agentID)
	if err != nil {
		return nil, err
	}

	// 2. If conversationID provided, get context_id from first execution
	contextID, err := s.getContextIDForConversation(ctx, userID, agentID, conversationID)
	if err != nil {
		return nil, err
	}

	// 3. Check if agent supports streaming
	supportsStreaming := s.a2aClient.SupportsStreaming(agent)

	// 4. Create execution record with appropriate initial status
	execution, err := s.createExecutionRecord(ctx, userID, agentID, input, conversationID, supportsStreaming)
	if err != nil {
		return nil, err
	}

	// 5. Set conversation ID for new conversations
	if err := s.setupConversationID(ctx, execution, conversationID); err != nil {
		// Log warning but continue
		s.logger.With(
			"service", "vibexp-api",
			"method", "InvokeAgent",
			"execution_id", execution.ID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to set conversation_id (non-fatal)")
	}

	s.logger.With(
		"service", "vibexp-api",
		"method", "InvokeAgent",
		"user_id", userID,
		"agent_id", agentID,
		"execution_id", execution.ID,
		"conversation_id", execution.ConversationID,
		"context_id", contextID,
		"supports_streaming", supportsStreaming,
	).Info("Created execution record, invoking agent")

	// 6. Route to appropriate method based on streaming support
	if supportsStreaming {
		return s.invokeAgentStreaming(ctx, agent, execution, input, contextID)
	}

	return s.invokeAgentSync(ctx, agent, execution, input, contextID)
}

// getAndValidateAgent retrieves and validates the agent
// Note: This method needs to determine the teamID. Since agents are team-scoped,
// we need to get the user's default team or handle this differently in the future.
// For now, we pass empty teamID which will require the repository to search across teams.
func (s *AgentInvocationService) getAndValidateAgent(
	ctx context.Context,
	userID, agentID string,
) (*models.Agent, error) {
	// Search across all user's teams
	agent, err := s.agentRepo.GetByIDCrossTeam(ctx, userID, agentID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"method", "InvokeAgent",
			"user_id", userID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get agent")
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	if agent.Status != "active" {
		s.logger.With(
			"service", "vibexp-api",
			"method", "InvokeAgent",
			"user_id", userID,
			"agent_id", agentID,
			"status", agent.Status,
		).Warn("Agent is not active")
		return nil, fmt.Errorf("agent is not active: %s", agent.Status)
	}

	return agent, nil
}

// getContextIDForConversation retrieves the context ID for an existing conversation
func (s *AgentInvocationService) getContextIDForConversation(
	ctx context.Context,
	userID, agentID string,
	conversationID *string,
) (*string, error) {
	if conversationID == nil || *conversationID == "" {
		return nil, nil
	}

	firstExecution, err := s.executionRepo.GetFirstExecutionInConversation(ctx, userID, *conversationID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"method", "InvokeAgent",
			"user_id", userID,
			"agent_id", agentID,
			"conversation_id", *conversationID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get first execution in conversation")
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}

	s.logger.With(
		"service", "vibexp-api",
		"method", "InvokeAgent",
		"user_id", userID,
		"agent_id", agentID,
		"conversation_id", *conversationID,
		"context_id", firstExecution.ContextID,
	).Info("Continuing existing conversation")

	return firstExecution.ContextID, nil
}

// createExecutionRecord creates a new execution record
func (s *AgentInvocationService) createExecutionRecord(
	ctx context.Context,
	userID, agentID string,
	input map[string]interface{},
	conversationID *string,
	supportsStreaming bool,
) (*models.AgentExecution, error) {
	initialStatus := "running"
	if supportsStreaming {
		initialStatus = "pending"
	}

	execution := &models.AgentExecution{
		AgentID:        agentID,
		UserID:         userID,
		Status:         initialStatus,
		Input:          input,
		StartedAt:      time.Now(),
		ConversationID: conversationID,
	}

	err := s.executionRepo.Create(ctx, execution)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"method", "InvokeAgent",
			"user_id", userID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create execution record")
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	return execution, nil
}

// setupConversationID sets the conversation ID for new conversations
func (s *AgentInvocationService) setupConversationID(
	ctx context.Context,
	execution *models.AgentExecution,
	conversationID *string,
) error {
	if conversationID == nil || *conversationID == "" {
		execution.ConversationID = &execution.ID
		return s.executionRepo.UpdateConversationID(ctx, execution.ID, execution.ID)
	}
	return nil
}

// invokeAgentSync handles synchronous agent invocation (non-streaming)
func (s *AgentInvocationService) invokeAgentSync(
	ctx context.Context,
	agent *models.Agent,
	execution *models.AgentExecution,
	input map[string]interface{},
	contextID *string,
) (*models.AgentExecution, error) {
	// Invoke agent via A2A protocol (contextID will be used by A2A client)
	result, err := s.a2aClient.InvokeAgent(ctx, agent, input, contextID)
	if err != nil {
		return s.handleSyncInvocationError(ctx, agent, execution, err)
	}

	return s.handleSyncInvocationSuccess(ctx, agent, execution, result)
}

// handleSyncInvocationError handles errors during synchronous invocation
func (s *AgentInvocationService) handleSyncInvocationError(
	ctx context.Context,
	agent *models.Agent,
	execution *models.AgentExecution,
	invocationErr error,
) (*models.AgentExecution, error) {
	endedAt := time.Now()
	execution.Status = "error"
	errorMsg := invocationErr.Error()
	execution.Error = &errorMsg
	execution.EndedAt = &endedAt
	durationMs := int(endedAt.Sub(execution.StartedAt).Milliseconds())
	execution.Duration = &durationMs

	if updateErr := s.executionRepo.Update(ctx, execution); updateErr != nil {
		s.logger.With(
			"service", "vibexp-api",
			"method", "invokeAgentSync",
			"execution_id", execution.ID,
			"error", fmt.Sprintf("%+v", updateErr),
		).Error("Failed to update execution with error")
		return nil, fmt.Errorf("invocation failed: %w, update failed: %v", invocationErr, updateErr)
	}

	// Update agent statistics
	if statsErr := s.agentRepo.UpdateExecutionStats(ctx, agent.ID, false, durationMs); statsErr != nil {
		s.logger.With(
			"service", "vibexp-api",
			"method", "invokeAgentSync",
			"agent_id", agent.ID,
			"error", fmt.Sprintf("%+v", statsErr),
		).Warn("Failed to update agent statistics after error")
	}

	s.logger.With(
		"service", "vibexp-api",
		"method", "invokeAgentSync",
		"execution_id", execution.ID,
		"error", fmt.Sprintf("%+v", invocationErr),
	).Error("Agent invocation failed")

	return execution, nil
}

// handleSyncInvocationSuccess handles successful synchronous invocation
func (s *AgentInvocationService) handleSyncInvocationSuccess(
	ctx context.Context,
	agent *models.Agent,
	execution *models.AgentExecution,
	result *models.AgentExecution,
) (*models.AgentExecution, error) {
	endedAt := time.Now()

	// Map A2A status to database-allowed values
	execution.Status = s.mapA2AStatusToDBStatus(result.Status)
	execution.Error = result.Error
	execution.EndedAt = &endedAt

	if result.Duration != nil {
		execution.Duration = result.Duration
	} else {
		durationMs := int(endedAt.Sub(execution.StartedAt).Milliseconds())
		execution.Duration = &durationMs
	}

	err := s.executionRepo.Update(ctx, execution)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"method", "invokeAgentSync",
			"execution_id", execution.ID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to update execution with result")
		return nil, fmt.Errorf("failed to update execution: %w", err)
	}

	// Update agent statistics
	success := result.Status == "completed"
	if err := s.agentRepo.UpdateExecutionStats(ctx, agent.ID, success, *execution.Duration); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"method", "invokeAgentSync",
			"agent_id", agent.ID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to update agent statistics")
	}

	s.logger.With(
		"service", "vibexp-api",
		"method", "invokeAgentSync",
		"execution_id", execution.ID,
		"status", result.Status,
		"duration_ms", *execution.Duration,
	).Info("Agent invocation completed successfully")

	return execution, nil
}

// mapA2AStatusToDBStatus maps the A2A-derived status returned by the client to a
// database-allowed execution status (agent_executions_status_check).
// A2A can return: completed, failed, rejected, cancelled, working, etc.
func (s *AgentInvocationService) mapA2AStatusToDBStatus(a2aStatus string) string {
	switch a2aStatus {
	case "completed":
		return "success"
	case "cancelled":
		return "cancelled"
	case "working", "submitted", "running":
		// Non-terminal task from a sync send — still running; polled in #163.
		return "running"
	case "error", "failed", "rejected":
		return "error"
	default:
		return "error" // default to error for unknown statuses
	}
}

// invokeAgentStreaming handles asynchronous agent invocation with streaming
func (s *AgentInvocationService) invokeAgentStreaming(
	_ctx context.Context,
	agent *models.Agent,
	execution *models.AgentExecution,
	input map[string]interface{},
	contextID *string,
) (*models.AgentExecution, error) {
	// Create buffered channel for streaming events
	eventChan := make(chan a2a.Event, 100)

	// Start stream processor in a separate goroutine
	go s.startStreamProcessor(execution.ID, eventChan) // #nosec G118 -- intentional: outlives request

	// Start HTTP streaming in another goroutine
	go s.startHTTPStreaming(
		agent,
		execution,
		input,
		contextID,
		eventChan,
	) // #nosec G118 -- intentional: outlives request

	// Return execution immediately (don't wait for goroutine)
	s.logger.With(
		"service", "vibexp-api",
		"method", "invokeAgentStreaming",
		"execution_id", execution.ID,
	).
		Info("Returning execution, processing will continue in background")

	return execution, nil
}

// startStreamProcessor starts the stream processor goroutine
func (s *AgentInvocationService) startStreamProcessor(executionID string, eventChan <-chan a2a.Event) {
	bgCtx := context.Background()

	s.logger.With(
		"service", "vibexp-api",
		"method", "invokeAgentStreaming",
		"execution_id", executionID,
	).
		Info("Starting stream processor")

	// Process stream events as they arrive
	if err := s.streamProcessor.ProcessStream(bgCtx, executionID, eventChan); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"method", "invokeAgentStreaming",
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Stream processing failed")
	} else {
		s.logger.With(
			"service", "vibexp-api",
			"method", "invokeAgentStreaming",
			"execution_id", executionID,
		).Info("Stream processing completed")
	}
}

// startHTTPStreaming starts the HTTP streaming goroutine
func (s *AgentInvocationService) startHTTPStreaming(
	agent *models.Agent,
	execution *models.AgentExecution,
	input map[string]interface{},
	contextID *string,
	eventChan chan<- a2a.Event,
) {
	// Create background context (won't cancel when request ends)
	bgCtx := context.Background()

	// Ensure channel is closed when this goroutine exits (with panic recovery)
	defer s.closeEventChannelSafely(execution.ID, eventChan)

	s.logger.With(
		"service", "vibexp-api",
		"method", "invokeAgentStreaming",
		"execution_id", execution.ID,
		"context_id", contextID,
	).Info("Starting HTTP streaming invocation")

	// Invoke agent with streaming (contextID will be used by A2A client)
	err := s.a2aClient.InvokeAgentStreaming(bgCtx, agent, input, contextID, eventChan)

	if err != nil {
		s.handleStreamingError(bgCtx, execution, err)
		return
	}

	s.logger.With(
		"service", "vibexp-api",
		"method", "invokeAgentStreaming",
		"execution_id", execution.ID,
	).
		Info("HTTP streaming completed")
}

// closeEventChannelSafely closes the event channel with panic recovery
func (s *AgentInvocationService) closeEventChannelSafely(executionID string, eventChan chan<- a2a.Event) {
	if r := recover(); r != nil {
		s.logger.With(
			"service", "vibexp-api",
			"method", "invokeAgentStreaming",
			"execution_id", executionID,
			"panic", r,
		).Error("Panic in streaming goroutine")
	}

	// Safe channel close - only panic is if channel is already closed
	defer func() {
		if r := recover(); r != nil {
			// Channel was already closed, which is expected in some cases
			s.logger.With(
				"service", "vibexp-api",
				"method", "invokeAgentStreaming",
				"execution_id", executionID,
			).
				Debug("Channel close panic recovered (expected)")
		}
	}()
	close(eventChan)
}

// handleStreamingError handles errors during streaming invocation
func (s *AgentInvocationService) handleStreamingError(
	ctx context.Context,
	execution *models.AgentExecution,
	err error,
) {
	s.logger.With(
		"service", "vibexp-api",
		"method", "invokeAgentStreaming",
		"execution_id", execution.ID,
		"error", fmt.Sprintf("%+v", err),
	).Error("Streaming invocation failed")

	// Update execution status to failed
	endedAt := time.Now()
	execution.Status = "error"
	errorMsg := err.Error()
	execution.Error = &errorMsg
	execution.EndedAt = &endedAt
	durationMs := int(endedAt.Sub(execution.StartedAt).Milliseconds())
	execution.Duration = &durationMs

	if updateErr := s.executionRepo.Update(ctx, execution); updateErr != nil {
		s.logger.With(
			"service", "vibexp-api",
			"method", "invokeAgentStreaming",
			"execution_id", execution.ID,
			"error", fmt.Sprintf("%+v", updateErr),
		).Error("Failed to update execution with error")
	}
}
