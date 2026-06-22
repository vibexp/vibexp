package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// A2AStreamProcessorInterface defines the interface for A2A stream processing
type A2AStreamProcessorInterface interface {
	ProcessStream(ctx context.Context, executionID string, eventChan <-chan *A2AStreamEvent) error
}

// A2AStreamProcessor processes streaming events from A2A agents
type A2AStreamProcessor struct {
	eventRepo     repositories.AgentExecutionEventRepository
	executionRepo repositories.AgentExecutionRepository
	logger        *slog.Logger
}

// NewA2AStreamProcessor creates a new A2A stream processor
func NewA2AStreamProcessor(
	eventRepo repositories.AgentExecutionEventRepository,
	executionRepo repositories.AgentExecutionRepository,
	logger *slog.Logger,
) *A2AStreamProcessor {
	return &A2AStreamProcessor{
		eventRepo:     eventRepo,
		executionRepo: executionRepo,
		logger:        logger,
	}
}

// ProcessStream processes streaming events from an agent execution
// storeEventInDB stores an event in the database
func (p *A2AStreamProcessor) storeEventInDB(
	ctx context.Context,
	executionID string,
	event *A2AStreamEvent,
	sequenceNumber int,
) {
	eventModel := &models.AgentExecutionEvent{
		ID:             uuid.New().String(),
		ExecutionID:    executionID,
		EventType:      event.Type,
		EventData:      event.Data,
		SequenceNumber: sequenceNumber,
		ReceivedAt:     event.Timestamp,
	}

	if err := p.eventRepo.Create(ctx, eventModel); err != nil {
		p.logger.With(
			"service", "vibexp-api",
			"method", "ProcessStream",
			"execution_id", executionID,
			"sequence_number", sequenceNumber,
			"event_type", event.Type,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to store event")
	}
}

// processEventByType processes an event based on its type
func (p *A2AStreamProcessor) processEventByType(
	ctx context.Context,
	executionID string,
	event *A2AStreamEvent,
	artifacts map[string]map[string]interface{},
) {
	switch event.Type {
	case "task":
		if err := p.handleTaskEvent(ctx, executionID, event); err != nil {
			p.logger.With(
				"service", "vibexp-api",
				"method", "ProcessStream",
				"execution_id", executionID,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to handle task event")
		}
	case "status-update":
		if err := p.handleStatusUpdate(ctx, executionID, event); err != nil {
			p.logger.With(
				"service", "vibexp-api",
				"method", "ProcessStream",
				"execution_id", executionID,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to handle status update")
		}
	case "artifact-update":
		p.handleArtifactUpdate(executionID, event, artifacts)
		if len(artifacts) > 0 {
			p.saveIncrementalArtifacts(ctx, executionID, artifacts)
		}
	}
}

// saveIncrementalArtifacts saves artifacts incrementally during processing
func (p *A2AStreamProcessor) saveIncrementalArtifacts(
	ctx context.Context,
	executionID string,
	artifacts map[string]map[string]interface{},
) {
	if err := p.saveArtifacts(ctx, executionID, artifacts); err != nil {
		p.logger.With(
			"service", "vibexp-api",
			"method", "ProcessStream",
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to save incremental artifacts")
	}
}

// finalizeExecution saves final artifacts and ensures execution is finalized
func (p *A2AStreamProcessor) finalizeExecution(
	executionID string,
	artifacts map[string]map[string]interface{},
) {
	bgCtx := context.Background()

	// Save final artifacts if any
	if len(artifacts) > 0 {
		if err := p.saveArtifacts(bgCtx, executionID, artifacts); err != nil {
			p.logger.With(
				"service", "vibexp-api",
				"method", "ProcessStream",
				"execution_id", executionID,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to save artifacts")
		}
	}

	// Ensure execution is finalized
	if err := p.executionRepo.UpdateStatus(bgCtx, executionID, "success"); err != nil {
		p.logger.With(
			"service", "vibexp-api",
			"method", "ProcessStream",
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to auto-finalize execution (may already be finalized)")
	} else {
		p.logger.With(
			"service", "vibexp-api",
			"method", "ProcessStream",
			"execution_id", executionID,
		).Info("Ensured execution is finalized after stream completion")
	}
}

func (p *A2AStreamProcessor) ProcessStream(
	ctx context.Context,
	executionID string,
	eventChan <-chan *A2AStreamEvent,
) error {
	sequenceNumber := 0
	artifacts := make(map[string]map[string]interface{})

	p.logger.With(
		"service", "vibexp-api",
		"method", "ProcessStream",
		"execution_id", executionID,
	).
		Info("Starting stream processing")

	for event := range eventChan {
		bgCtx := context.Background()
		p.storeEventInDB(bgCtx, executionID, event, sequenceNumber)
		p.processEventByType(bgCtx, executionID, event, artifacts)
		sequenceNumber++
	}

	p.finalizeExecution(executionID, artifacts)

	p.logger.With(
		"service", "vibexp-api",
		"method", "ProcessStream",
		"execution_id", executionID,
		"total_events", sequenceNumber,
	).Info("Stream processing completed")

	return nil
}

// handleTaskEvent processes a task event
func (p *A2AStreamProcessor) handleTaskEvent(
	ctx context.Context,
	executionID string,
	event *A2AStreamEvent,
) error {
	// Extract task_id and context_id from event data
	// A2A spec uses "id" for task ID and "contextId" for context ID
	taskID := ""
	contextID := ""
	currentState := ""

	// Try "id" first (A2A spec), then "taskId" as fallback
	if tid, ok := event.Data["id"].(string); ok {
		taskID = tid
	} else if tid, ok := event.Data["taskId"].(string); ok {
		taskID = tid
	}

	if cid, ok := event.Data["contextId"].(string); ok {
		contextID = cid
	}

	// Extract initial state from status.state
	if status, ok := event.Data["status"].(map[string]interface{}); ok {
		if state, ok := status["state"].(string); ok {
			currentState = state
		}
	}

	// Update execution with task info
	if err := p.executionRepo.UpdateTaskInfo(ctx, executionID, taskID, contextID, currentState); err != nil {
		return fmt.Errorf("failed to update task info: %w", err)
	}

	p.logger.With(
		"service", "vibexp-api",
		"method", "handleTaskEvent",
		"execution_id", executionID,
		"task_id", taskID,
		"context_id", contextID,
		"current_state", currentState,
	).Info("Task event processed")

	return nil
}

func extractStatusFields(event *A2AStreamEvent) (currentState, taskID, contextID string, isFinal bool) {
	if status, ok := event.Data["status"].(map[string]interface{}); ok {
		if state, ok := status["state"].(string); ok {
			currentState = state
		}
	}

	if tid, ok := event.Data["task_id"].(string); ok {
		taskID = tid
	} else if tid, ok := event.Data["taskId"].(string); ok {
		taskID = tid
	}

	if cid, ok := event.Data["context_id"].(string); ok {
		contextID = cid
	} else if cid, ok := event.Data["contextId"].(string); ok {
		contextID = cid
	}

	if final, ok := event.Data["final"].(bool); ok {
		isFinal = final
	}

	return
}

func mapStateToFinalStatus(currentState string) string {
	switch currentState {
	case "completed":
		return "success"
	case "failed", "rejected", "error":
		return "error"
	case "cancelled":
		return "error"
	default:
		return "success"
	}
}

func (p *A2AStreamProcessor) finalizeFinalStatus(ctx context.Context, executionID, currentState string) error {
	finalStatus := mapStateToFinalStatus(currentState)

	if err := p.executionRepo.UpdateStatus(ctx, executionID, finalStatus); err != nil {
		return fmt.Errorf("failed to finalize execution status: %w", err)
	}

	p.logger.With(
		"service", "vibexp-api",
		"method", "handleStatusUpdate",
		"execution_id", executionID,
		"current_state", currentState,
		"final_status", finalStatus,
		"final", true,
	).Info("Final status update received - execution finalized")

	return nil
}

// handleStatusUpdate processes a status update event
func (p *A2AStreamProcessor) handleStatusUpdate(
	ctx context.Context,
	executionID string,
	event *A2AStreamEvent,
) error {
	currentState, taskID, contextID, isFinal := extractStatusFields(event)

	if err := p.executionRepo.UpdateTaskInfo(ctx, executionID, taskID, contextID, currentState); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	if isFinal {
		return p.finalizeFinalStatus(ctx, executionID, currentState)
	}

	p.logger.With(
		"service", "vibexp-api",
		"method", "handleStatusUpdate",
		"execution_id", executionID,
		"current_state", currentState,
	).Info("Status update processed")

	return nil
}

// extractArtifactID extracts artifact ID from event data
func extractArtifactID(eventData map[string]interface{}) string {
	if aid, ok := eventData["artifactId"].(string); ok {
		return aid
	}
	if artifact, ok := eventData["artifact"].(map[string]interface{}); ok {
		if aid, ok := artifact["artifactId"].(string); ok {
			return aid
		}
	}
	return ""
}

// extractArtifactFlags extracts append and lastChunk flags from event data
func extractArtifactFlags(eventData map[string]interface{}) (shouldAppend, lastChunk bool) {
	if appendFlag, ok := eventData["append"].(bool); ok {
		shouldAppend = appendFlag
	}
	if last, ok := eventData["lastChunk"].(bool); ok {
		lastChunk = last
	}
	return shouldAppend, lastChunk
}

// extractNewArtifact extracts the artifact object from event data
func extractNewArtifact(eventData map[string]interface{}) map[string]interface{} {
	if artifact, ok := eventData["artifact"].(map[string]interface{}); ok {
		return artifact
	}
	return eventData
}

// mergeLegacyArtifactParts merges new parts into existing artifact
func mergeArtifactParts(
	existing map[string]interface{},
	newParts []interface{},
) map[string]interface{} {
	if existingParts, ok := existing["parts"].([]interface{}); ok {
		if len(newParts) > 0 {
			existingParts = append(existingParts, newParts...)
			existing["parts"] = existingParts
		}
	} else if len(newParts) > 0 {
		existing["parts"] = newParts
	}
	return existing
}

// calculateTotalParts counts the total parts in an artifact
func calculateTotalParts(artifacts map[string]map[string]interface{}, artifactID string) int {
	if artifact, exists := artifacts[artifactID]; exists {
		if parts, ok := artifact["parts"].([]interface{}); ok {
			return len(parts)
		}
	}
	return 0
}

// handleArtifactUpdate processes an artifact update event
func (p *A2AStreamProcessor) handleArtifactUpdate(
	executionID string,
	event *A2AStreamEvent,
	artifacts map[string]map[string]interface{},
) {
	artifactID := extractArtifactID(event.Data)
	if artifactID == "" {
		p.logger.With(
			"service", "vibexp-api",
			"method", "handleArtifactUpdate",
			"execution_id", executionID,
		).
			Warn("Artifact update missing artifactId")
		return
	}

	shouldAppend, lastChunk := extractArtifactFlags(event.Data)
	newArtifact := extractNewArtifact(event.Data)

	var newParts []interface{}
	if parts, ok := newArtifact["parts"].([]interface{}); ok {
		newParts = parts
	}

	// Update artifacts based on append mode
	if !shouldAppend {
		artifacts[artifactID] = newArtifact
	} else {
		if existing, exists := artifacts[artifactID]; exists {
			artifacts[artifactID] = mergeArtifactParts(existing, newParts)
		} else {
			artifacts[artifactID] = newArtifact
		}
	}

	totalParts := calculateTotalParts(artifacts, artifactID)

	p.logger.With(
		"service", "vibexp-api",
		"method", "handleArtifactUpdate",
		"execution_id", executionID,
		"artifact_id", artifactID,
		"append", shouldAppend,
		"last_chunk", lastChunk,
		"new_parts_count", len(newParts),
		"total_parts_count", totalParts,
	).Info("Artifact update processed")
}

// saveArtifacts saves the accumulated artifacts to the database
// Ensures A2A-compliant artifact structure with proper fields (artifactId, parts, etc.)
func (p *A2AStreamProcessor) saveArtifacts(
	ctx context.Context,
	executionID string,
	artifacts map[string]map[string]interface{},
) error {
	// Convert map to array of A2A-compliant artifacts
	artifactArray := make([]map[string]interface{}, 0, len(artifacts))
	for artifactID, artifactData := range artifacts {
		// Extract the nested artifact object if present
		var a2aArtifact map[string]interface{}
		if nestedArtifact, ok := artifactData["artifact"].(map[string]interface{}); ok {
			a2aArtifact = nestedArtifact
		} else {
			a2aArtifact = artifactData
		}

		// Ensure artifactId is set (A2A spec requirement)
		if _, ok := a2aArtifact["artifactId"]; !ok {
			a2aArtifact["artifactId"] = artifactID
		}

		// Only include artifacts with non-empty parts
		if parts, ok := a2aArtifact["parts"].([]interface{}); ok && len(parts) > 0 {
			artifactArray = append(artifactArray, a2aArtifact)
		}
	}

	// Save artifacts array to database (A2A-compliant format)
	if err := p.executionRepo.UpdateArtifacts(ctx, executionID, artifactArray); err != nil {
		return fmt.Errorf("failed to update artifacts: %w", err)
	}

	// Log detailed artifact information
	totalParts := 0
	for _, artifact := range artifactArray {
		if parts, ok := artifact["parts"].([]interface{}); ok {
			totalParts += len(parts)
		}
	}

	p.logger.With(
		"service", "vibexp-api",
		"method", "saveArtifacts",
		"execution_id", executionID,
		"artifact_count", len(artifactArray),
		"total_parts", totalParts,
	).Info("Artifacts saved in A2A-compliant format")

	return nil
}
