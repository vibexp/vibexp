package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// A2AStreamProcessorInterface defines the interface for A2A stream processing.
// It consumes the SDK's typed a2a.Event union.
type A2AStreamProcessorInterface interface {
	ProcessStream(ctx context.Context, executionID string, eventChan <-chan a2a.Event) error
}

// A2AStreamProcessor processes streaming events from A2A agents.
type A2AStreamProcessor struct {
	eventRepo     repositories.AgentExecutionEventRepository
	executionRepo repositories.AgentExecutionRepository
	logger        *slog.Logger
}

// NewA2AStreamProcessor creates a new A2A stream processor.
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

// eventTypeFor maps a typed SDK event to a persisted event_type. The DB CHECK
// only allows task / status-update / artifact-update, so other events (e.g. a
// direct *a2a.Message reply — persisted by #163) are skipped.
func eventTypeFor(event a2a.Event) (string, bool) {
	switch event.(type) {
	case *a2a.Task:
		return "task", true
	case *a2a.TaskStatusUpdateEvent:
		return "status-update", true
	case *a2a.TaskArtifactUpdateEvent:
		return "artifact-update", true
	default:
		return "", false
	}
}

// eventToMap serializes a typed event to the JSON map stored in event_data,
// preserving the A2A v1.0 wire shape for the frontend polling contract.
func eventToMap(event a2a.Event) (map[string]interface{}, error) {
	raw, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event: %w", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("failed to decode event: %w", err)
	}
	return data, nil
}

// artifactToMap serializes an artifact for the agent_executions.artifacts column.
func artifactToMap(artifact *a2a.Artifact) (map[string]interface{}, error) {
	raw, err := json.Marshal(artifact)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal artifact: %w", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("failed to decode artifact: %w", err)
	}
	return data, nil
}

// storeEventInDB persists a single streaming event row.
func (p *A2AStreamProcessor) storeEventInDB(
	ctx context.Context,
	executionID, eventType string,
	event a2a.Event,
	sequenceNumber int,
) {
	data, err := eventToMap(event)
	if err != nil {
		p.logger.With(
			"service", "vibexp-api", "method", "ProcessStream",
			"execution_id", executionID, "sequence_number", sequenceNumber,
			"event_type", eventType, "error", fmt.Sprintf("%+v", err),
		).Error("Failed to serialize event")
		return
	}

	eventModel := &models.AgentExecutionEvent{
		ID:             uuid.New().String(),
		ExecutionID:    executionID,
		EventType:      eventType,
		EventData:      data,
		SequenceNumber: sequenceNumber,
		ReceivedAt:     time.Now(),
	}

	if err := p.eventRepo.Create(ctx, eventModel); err != nil {
		p.logger.With(
			"service", "vibexp-api", "method", "ProcessStream",
			"execution_id", executionID, "sequence_number", sequenceNumber,
			"event_type", eventType, "error", fmt.Sprintf("%+v", err),
		).Error("Failed to store event")
	}
}

// ProcessStream consumes typed A2A events, persisting each and updating the
// execution's task/status/artifact state, then finalizes on stream completion.
func (p *A2AStreamProcessor) ProcessStream(
	ctx context.Context,
	executionID string,
	eventChan <-chan a2a.Event,
) error {
	sequenceNumber := 0
	artifacts := make(map[string]*a2a.Artifact)
	var finalState a2a.TaskState
	haveFinal := false

	p.logger.With(
		"service", "vibexp-api", "method", "ProcessStream", "execution_id", executionID,
	).Info("Starting stream processing")

	for event := range eventChan {
		eventType, ok := eventTypeFor(event)
		if !ok {
			// Not a persisted execution event (e.g. a direct message reply — #163).
			continue
		}

		bgCtx := context.Background()
		p.storeEventInDB(bgCtx, executionID, eventType, event, sequenceNumber)
		sequenceNumber++

		switch e := event.(type) {
		case *a2a.Task:
			p.handleTaskEvent(bgCtx, executionID, e)
			collectArtifacts(artifacts, e.Artifacts)
			if e.Status.State.Terminal() {
				finalState, haveFinal = e.Status.State, true
			}
		case *a2a.TaskStatusUpdateEvent:
			p.handleStatusUpdate(bgCtx, executionID, e)
			if e.Status.State.Terminal() {
				finalState, haveFinal = e.Status.State, true
			}
		case *a2a.TaskArtifactUpdateEvent:
			mergeArtifact(artifacts, e)
			p.saveArtifacts(bgCtx, executionID, artifacts)
		}
	}

	p.finalizeExecution(executionID, artifacts, finalState, haveFinal)

	p.logger.With(
		"service", "vibexp-api", "method", "ProcessStream",
		"execution_id", executionID, "total_events", sequenceNumber,
	).Info("Stream processing completed")

	return nil
}

// handleTaskEvent records the task's id/context/state on the execution.
func (p *A2AStreamProcessor) handleTaskEvent(ctx context.Context, executionID string, task *a2a.Task) {
	if err := p.executionRepo.UpdateTaskInfo(
		ctx, executionID, string(task.ID), task.ContextID, string(task.Status.State),
	); err != nil {
		p.logger.With(
			"service", "vibexp-api", "method", "handleTaskEvent",
			"execution_id", executionID, "error", fmt.Sprintf("%+v", err),
		).Error("Failed to update task info")
	}
}

// handleStatusUpdate records a status-update's task/context/state on the execution.
func (p *A2AStreamProcessor) handleStatusUpdate(
	ctx context.Context, executionID string, event *a2a.TaskStatusUpdateEvent,
) {
	if err := p.executionRepo.UpdateTaskInfo(
		ctx, executionID, string(event.TaskID), event.ContextID, string(event.Status.State),
	); err != nil {
		p.logger.With(
			"service", "vibexp-api", "method", "handleStatusUpdate",
			"execution_id", executionID, "error", fmt.Sprintf("%+v", err),
		).Error("Failed to update status")
	}
}

// collectArtifacts records artifacts carried on a Task snapshot.
func collectArtifacts(artifacts map[string]*a2a.Artifact, incoming []*a2a.Artifact) {
	for _, artifact := range incoming {
		if artifact == nil || artifact.ID == "" {
			continue
		}
		artifacts[string(artifact.ID)] = artifact
	}
}

// mergeArtifact applies an artifact-update event, appending parts when the event
// is incremental (Append) or replacing the artifact otherwise.
func mergeArtifact(artifacts map[string]*a2a.Artifact, event *a2a.TaskArtifactUpdateEvent) {
	if event.Artifact == nil || event.Artifact.ID == "" {
		return
	}
	id := string(event.Artifact.ID)
	existing, ok := artifacts[id]
	if event.Append && ok {
		existing.Parts = append(existing.Parts, event.Artifact.Parts...)
		return
	}
	artifacts[id] = event.Artifact
}

// saveArtifacts persists the accumulated artifacts (those with parts) as the
// execution's artifact array.
func (p *A2AStreamProcessor) saveArtifacts(
	ctx context.Context, executionID string, artifacts map[string]*a2a.Artifact,
) {
	arr := make([]map[string]interface{}, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact == nil || len(artifact.Parts) == 0 {
			continue
		}
		data, err := artifactToMap(artifact)
		if err != nil {
			continue
		}
		arr = append(arr, data)
	}

	if err := p.executionRepo.UpdateArtifacts(ctx, executionID, arr); err != nil {
		p.logger.With(
			"service", "vibexp-api", "method", "saveArtifacts",
			"execution_id", executionID, "error", fmt.Sprintf("%+v", err),
		).Warn("Failed to save artifacts")
	}
}

// mapTerminalStateToStatus maps a terminal A2A task state to the DB execution status.
func mapTerminalStateToStatus(state a2a.TaskState) string {
	switch state {
	case a2a.TaskStateCompleted:
		return "success"
	case a2a.TaskStateCanceled:
		return "cancelled"
	case a2a.TaskStateFailed, a2a.TaskStateRejected:
		return "error"
	default:
		return "success"
	}
}

// finalizeExecution saves final artifacts and finalizes the execution status.
func (p *A2AStreamProcessor) finalizeExecution(
	executionID string,
	artifacts map[string]*a2a.Artifact,
	finalState a2a.TaskState,
	haveFinal bool,
) {
	bgCtx := context.Background()

	if len(artifacts) > 0 {
		p.saveArtifacts(bgCtx, executionID, artifacts)
	}

	status := "success"
	if haveFinal {
		status = mapTerminalStateToStatus(finalState)
	}

	if err := p.executionRepo.UpdateStatus(bgCtx, executionID, status); err != nil {
		p.logger.With(
			"service", "vibexp-api", "method", "finalizeExecution",
			"execution_id", executionID, "error", fmt.Sprintf("%+v", err),
		).Warn("Failed to finalize execution (may already be finalized)")
		return
	}

	p.logger.With(
		"service", "vibexp-api", "method", "finalizeExecution",
		"execution_id", executionID, "final_status", status,
	).Info("Execution finalized after stream completion")
}
