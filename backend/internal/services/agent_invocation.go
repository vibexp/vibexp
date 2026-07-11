package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// ErrExecutionNotCancelable indicates a cancel request targeted an execution
// that is already terminal (mapped to a 409 by the handler).
var ErrExecutionNotCancelable = errors.New("execution is already terminal and cannot be cancelled")

// AgentInvocationServiceInterface defines the interface for agent invocation
type AgentInvocationServiceInterface interface {
	InvokeAgent(
		ctx context.Context, userID, agentID string,
		input map[string]interface{}, conversationID *string,
	) (*models.AgentExecution, error)
	// CancelExecution cancels a running execution: it aborts local streaming,
	// asks the remote agent to cancel the task (when there is one), and marks the
	// execution cancelled. Returns ErrExecutionNotCancelable / a2a.ErrTaskNotCancelable
	// when the execution/task is already terminal.
	CancelExecution(
		ctx context.Context, execution *models.AgentExecution, agent *models.Agent,
	) (*models.AgentExecution, error)
}

// AgentInvocationService handles agent invocation logic
type AgentInvocationService struct {
	agentRepo       repositories.AgentRepository
	executionRepo   repositories.AgentExecutionRepository
	eventRepo       repositories.AgentExecutionEventRepository
	a2aClient       A2AHTTPClientInterface
	streamProcessor A2AStreamProcessorInterface
	logger          *slog.Logger
	// execCancels holds the cancel func for each in-flight streaming execution
	// (keyed by execution ID) so a cancel request can abort local goroutines.
	execCancels sync.Map
}

// NewAgentInvocationService creates a new agent invocation service
func NewAgentInvocationService(
	agentRepo repositories.AgentRepository,
	executionRepo repositories.AgentExecutionRepository,
	eventRepo repositories.AgentExecutionEventRepository,
	a2aClient A2AHTTPClientInterface,
	streamProcessor A2AStreamProcessorInterface,
	logger *slog.Logger,
) *AgentInvocationService {
	return &AgentInvocationService{
		agentRepo:       agentRepo,
		executionRepo:   executionRepo,
		eventRepo:       eventRepo,
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

// Sync task polling bounds: a message/send that returns a non-terminal Task is
// polled with capped exponential backoff until terminal or the deadline.
// Vars (not consts) so tests can shorten them.
var (
	syncTaskPollInitialBackoff = 1 * time.Second
	syncTaskPollMaxBackoff     = 5 * time.Second
	syncTaskPollDeadline       = 5 * time.Minute
)

// isNonTerminalSyncStatus reports whether a client-reported status means the
// remote task is still running and must be polled.
func isNonTerminalSyncStatus(status string) bool {
	switch status {
	case "working", "submitted", "running", "pending":
		return true
	default:
		return false
	}
}

// invokeAgentSync handles synchronous agent invocation (non-streaming). The
// reply is persisted as artifacts; a non-terminal Task result is polled until
// it reaches a terminal state.
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

	// Per the A2A spec, message/send may return a Task in submitted/working
	// state; poll it to a terminal state rather than reporting it done.
	if result.TaskID != nil && isNonTerminalSyncStatus(result.Status) {
		polled, pollErr := s.pollSyncTask(ctx, agent, execution, result)
		if pollErr != nil {
			return s.handleSyncInvocationError(ctx, agent, execution, pollErr)
		}
		result = polled
	}

	return s.handleSyncInvocationSuccess(ctx, agent, execution, result)
}

// pollSyncTask polls the remote task until it is terminal or the deadline is
// exceeded, persisting current_state live so the status endpoint reflects
// progress. It returns the terminal snapshot.
func (s *AgentInvocationService) pollSyncTask(
	ctx context.Context,
	agent *models.Agent,
	execution *models.AgentExecution,
	initial *models.AgentExecution,
) (*models.AgentExecution, error) {
	taskID := *initial.TaskID
	s.persistTaskProgress(ctx, execution, initial)

	pollCtx, cancel := context.WithTimeout(ctx, syncTaskPollDeadline)
	defer cancel()

	backoff := syncTaskPollInitialBackoff
	for {
		select {
		case <-pollCtx.Done():
			return nil, fmt.Errorf(
				"agent task %s did not reach a terminal state within %s", taskID, syncTaskPollDeadline,
			)
		case <-time.After(backoff):
		}

		snapshot, err := s.a2aClient.GetTask(pollCtx, agent, taskID)
		if err != nil {
			return nil, fmt.Errorf("failed to poll agent task %s: %w", taskID, err)
		}
		s.persistTaskProgress(ctx, execution, snapshot)

		if !isNonTerminalSyncStatus(snapshot.Status) {
			return snapshot, nil
		}

		if backoff *= 2; backoff > syncTaskPollMaxBackoff {
			backoff = syncTaskPollMaxBackoff
		}
	}
}

// persistSyncReply persists a sync execution's reply. Sync replies reuse the
// agent_executions.artifacts jsonb in the same A2A v1.0 shape the streaming path
// writes, so there is no schema or frontend-contract change.
func (s *AgentInvocationService) persistSyncReply(
	ctx context.Context, execution, result *models.AgentExecution,
) {
	if len(result.Artifacts) > 0 {
		if artErr := s.executionRepo.UpdateArtifacts(ctx, execution.ID, result.Artifacts); artErr != nil {
			s.logger.With(
				"service", "vibexp-api",
				"method", "invokeAgentSync",
				"execution_id", execution.ID,
				"error", fmt.Sprintf("%+v", artErr),
			).Warn("Failed to persist reply artifacts")
		}
	}
	if result.TaskID != nil || result.CurrentState != nil {
		s.persistTaskProgress(ctx, execution, result)
	}
}

// persistTaskProgress records live task id/context/state on the execution.
func (s *AgentInvocationService) persistTaskProgress(
	ctx context.Context, execution, snapshot *models.AgentExecution,
) {
	taskID, contextID, state := "", "", ""
	if snapshot.TaskID != nil {
		taskID = *snapshot.TaskID
	}
	if snapshot.ContextID != nil {
		contextID = *snapshot.ContextID
	}
	if snapshot.CurrentState != nil {
		state = *snapshot.CurrentState
	}
	if err := s.executionRepo.UpdateTaskInfo(ctx, execution.ID, taskID, contextID, state); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"method", "pollSyncTask",
			"execution_id", execution.ID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to update task progress")
	}
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
	execution.TaskID = result.TaskID
	execution.ContextID = result.ContextID
	execution.CurrentState = result.CurrentState
	execution.Artifacts = result.Artifacts

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

	s.persistSyncReply(ctx, execution, result)

	// Update agent statistics
	success := execution.Status == "success"
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

// isTerminalStatus reports whether an execution has already finished.
func isTerminalStatus(status string) bool {
	switch status {
	case "success", "error", "cancelled", "completed", "failed":
		return true
	default:
		return false
	}
}

// CancelExecution cancels a running execution: it aborts local streaming
// goroutines, asks the remote agent to cancel the task (when there is one), and
// marks the execution cancelled with a final status-update event.
func (s *AgentInvocationService) CancelExecution(
	ctx context.Context,
	execution *models.AgentExecution,
	agent *models.Agent,
) (*models.AgentExecution, error) {
	if isTerminalStatus(execution.Status) {
		return nil, ErrExecutionNotCancelable
	}

	// Abort any local streaming goroutines for this execution.
	if c, ok := s.execCancels.LoadAndDelete(execution.ID); ok {
		c.(context.CancelFunc)()
	}

	// Ask the remote agent to cancel its task, when we have one.
	if execution.TaskID != nil && *execution.TaskID != "" {
		if _, err := s.a2aClient.CancelTask(ctx, agent, *execution.TaskID); err != nil {
			if errors.Is(err, a2a.ErrTaskNotCancelable) {
				return nil, a2a.ErrTaskNotCancelable
			}
			// The user asked to stop; log the remote failure but still cancel locally.
			s.logger.With(
				"service", "vibexp-api",
				"method", "CancelExecution",
				"execution_id", execution.ID,
				"task_id", *execution.TaskID,
				"error", fmt.Sprintf("%+v", err),
			).Warn("Remote task cancel failed; cancelling locally")
		}
	}

	return s.finalizeCancelled(ctx, execution)
}

// finalizeCancelled marks the execution cancelled and emits a terminal
// status-update event so the polling frontend converges.
func (s *AgentInvocationService) finalizeCancelled(
	ctx context.Context, execution *models.AgentExecution,
) (*models.AgentExecution, error) {
	endedAt := time.Now()
	execution.Status = "cancelled"
	execution.Error = nil
	execution.EndedAt = &endedAt
	durationMs := int(endedAt.Sub(execution.StartedAt).Milliseconds())
	execution.Duration = &durationMs
	state := string(a2a.TaskStateCanceled)
	execution.CurrentState = &state

	if err := s.executionRepo.Update(ctx, execution); err != nil {
		return nil, fmt.Errorf("failed to mark execution cancelled: %w", err)
	}
	s.persistTaskProgress(ctx, execution, execution)
	s.emitCancelledEvent(ctx, execution)

	s.logger.With(
		"service", "vibexp-api",
		"method", "CancelExecution",
		"execution_id", execution.ID,
	).Info("Execution cancelled")

	return execution, nil
}

// emitCancelledEvent appends a terminal status-update event with the cancelled state.
func (s *AgentInvocationService) emitCancelledEvent(ctx context.Context, execution *models.AgentExecution) {
	seq, err := s.eventRepo.CountByExecutionID(ctx, execution.ID)
	if err != nil {
		seq = 0
	}
	taskID, contextID := "", ""
	if execution.TaskID != nil {
		taskID = *execution.TaskID
	}
	if execution.ContextID != nil {
		contextID = *execution.ContextID
	}
	event := &models.AgentExecutionEvent{
		ID:          uuid.New().String(),
		ExecutionID: execution.ID,
		EventType:   "status-update",
		EventData: map[string]interface{}{
			"taskId":    taskID,
			"contextId": contextID,
			"status":    map[string]interface{}{"state": string(a2a.TaskStateCanceled)},
		},
		SequenceNumber: seq,
		ReceivedAt:     time.Now(),
	}
	if createErr := s.eventRepo.Create(ctx, event); createErr != nil {
		s.logger.With(
			"service", "vibexp-api",
			"method", "CancelExecution",
			"execution_id", execution.ID,
			"error", fmt.Sprintf("%+v", createErr),
		).Warn("Failed to emit cancelled event")
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

	// A cancellable background context (won't cancel when the request ends) so a
	// cancel request can abort the streaming goroutines.
	streamCtx, cancel := context.WithCancel(context.Background())
	s.execCancels.Store(execution.ID, cancel)

	// Start stream processor in a separate goroutine
	go s.startStreamProcessor(streamCtx, execution.ID, eventChan) // #nosec G118 -- intentional: outlives request

	// Start HTTP streaming in another goroutine
	go s.startHTTPStreaming(
		streamCtx,
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

// startStreamProcessor starts the stream processor goroutine. It is the last
// goroutine to finish (the producer closes the channel), so it releases the
// execution's cancel func when done.
func (s *AgentInvocationService) startStreamProcessor(
	ctx context.Context, executionID string, eventChan <-chan a2a.Event,
) {
	defer func() {
		if c, ok := s.execCancels.LoadAndDelete(executionID); ok {
			c.(context.CancelFunc)()
		}
	}()

	s.logger.With(
		"service", "vibexp-api",
		"method", "invokeAgentStreaming",
		"execution_id", executionID,
	).
		Info("Starting stream processor")

	// Process stream events as they arrive
	if err := s.streamProcessor.ProcessStream(ctx, executionID, eventChan); err != nil {
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
	ctx context.Context,
	agent *models.Agent,
	execution *models.AgentExecution,
	input map[string]interface{},
	contextID *string,
	eventChan chan<- a2a.Event,
) {
	// Ensure channel is closed when this goroutine exits (with panic recovery)
	defer s.closeEventChannelSafely(execution.ID, eventChan)

	s.logger.With(
		"service", "vibexp-api",
		"method", "invokeAgentStreaming",
		"execution_id", execution.ID,
		"context_id", contextID,
	).Info("Starting HTTP streaming invocation")

	// Invoke agent with streaming (contextID will be used by A2A client)
	err := s.a2aClient.InvokeAgentStreaming(ctx, agent, input, contextID, eventChan)

	if err != nil {
		s.handleStreamingError(ctx, execution, err)
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
	// A cancelled stream context means CancelExecution aborted us; it owns the
	// final (cancelled) status, so do not overwrite it with "error".
	if errors.Is(err, context.Canceled) {
		s.logger.With(
			"service", "vibexp-api",
			"method", "invokeAgentStreaming",
			"execution_id", execution.ID,
		).Info("Streaming aborted by cancellation")
		return
	}

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
