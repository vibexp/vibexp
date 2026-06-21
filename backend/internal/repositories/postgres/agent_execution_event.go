package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

type agentExecutionEventRepository struct {
	db *database.DB
}

// NewAgentExecutionEventRepository creates a new agent execution event repository
func NewAgentExecutionEventRepository(db *database.DB) repositories.AgentExecutionEventRepository {
	return &agentExecutionEventRepository{db: db}
}

// Create stores a new streaming event
func (r *agentExecutionEventRepository) Create(ctx context.Context, event *models.AgentExecutionEvent) error {
	logger := contextkeys.GetLoggerFromContext(ctx)

	if event.ID == "" {
		event.ID = uuid.New().String()
	}

	eventDataJSON, err := json.Marshal(event.EventData)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"method":       "Create",
			"execution_id": event.ExecutionID,
			"error":        fmt.Sprintf("%+v", err),
		}).Error("Failed to marshal event_data")
		return fmt.Errorf("failed to marshal event_data: %w", err)
	}

	query := `
		INSERT INTO agent_execution_events (id, execution_id, event_type, event_data, sequence_number, received_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err = r.db.ExecContext(ctx, query,
		event.ID, event.ExecutionID, event.EventType, eventDataJSON, event.SequenceNumber, event.ReceivedAt)

	if err != nil {
		if uniqueViolation(err) != nil {
			logger.WithFields(logrus.Fields{
				"method":          "Create",
				"execution_id":    event.ExecutionID,
				"sequence_number": event.SequenceNumber,
				"error":           fmt.Sprintf("%+v", err),
			}).Warn("Duplicate event detected")
			return fmt.Errorf(
				"event with execution_id %s and sequence_number %d already exists",
				event.ExecutionID, event.SequenceNumber,
			)
		}

		logger.WithFields(logrus.Fields{
			"method":       "Create",
			"execution_id": event.ExecutionID,
			"error":        fmt.Sprintf("%+v", err),
		}).Error("Failed to create agent execution event")
		return fmt.Errorf("failed to create agent execution event: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"method":          "Create",
		"event_id":        event.ID,
		"execution_id":    event.ExecutionID,
		"event_type":      event.EventType,
		"sequence_number": event.SequenceNumber,
	}).Debug("Agent execution event created")

	return nil
}

// GetByID retrieves a single event by ID
func (r *agentExecutionEventRepository) GetByID(
	ctx context.Context, eventID string,
) (*models.AgentExecutionEvent, error) {
	logger := contextkeys.GetLoggerFromContext(ctx)

	query := `
		SELECT id, execution_id, event_type, event_data, sequence_number, received_at
		FROM agent_execution_events
		WHERE id = $1
	`

	var event models.AgentExecutionEvent
	var eventDataJSON []byte

	err := r.db.QueryRowContext(ctx, query, eventID).Scan(
		&event.ID, &event.ExecutionID, &event.EventType, &eventDataJSON, &event.SequenceNumber, &event.ReceivedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repositories.ErrAgentExecutionEventNotFound
		}
		logger.WithFields(logrus.Fields{
			"method":   "GetByID",
			"event_id": eventID,
			"error":    fmt.Sprintf("%+v", err),
		}).Error("Failed to get agent execution event by ID")
		return nil, fmt.Errorf("failed to get agent execution event: %w", err)
	}

	if len(eventDataJSON) > 0 {
		if err := json.Unmarshal(eventDataJSON, &event.EventData); err != nil {
			logger.WithFields(logrus.Fields{
				"method":   "GetByID",
				"event_id": eventID,
				"error":    fmt.Sprintf("%+v", err),
			}).Error("Failed to unmarshal event_data")
			return nil, fmt.Errorf("failed to unmarshal event_data: %w", err)
		}
	}

	return &event, nil
}

// ListByExecutionID gets paginated list of events for an execution
//
//nolint:funlen,lll // Repository code with necessary complexity
func (r *agentExecutionEventRepository) ListByExecutionID(ctx context.Context, executionID string, limit, offset int) ([]models.AgentExecutionEvent, int, error) {
	logger := contextkeys.GetLoggerFromContext(ctx)

	// Get total count first
	countQuery := `SELECT COUNT(*) FROM agent_execution_events WHERE execution_id = $1`
	var totalCount int
	err := r.db.QueryRowContext(ctx, countQuery, executionID).Scan(&totalCount)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"method":       "ListByExecutionID",
			"execution_id": executionID,
			"error":        fmt.Sprintf("%+v", err),
		}).Error("Failed to count agent execution events")
		return nil, 0, fmt.Errorf("failed to count agent execution events: %w", err)
	}

	// Set default pagination values
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	// Query events with pagination
	query := `
		SELECT id, execution_id, event_type, event_data, sequence_number, received_at
		FROM agent_execution_events
		WHERE execution_id = $1
		ORDER BY sequence_number ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, executionID, limit, offset)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"method":       "ListByExecutionID",
			"execution_id": executionID,
			"error":        fmt.Sprintf("%+v", err),
		}).Error("Failed to list agent execution events")
		return nil, 0, fmt.Errorf("failed to list agent execution events: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logger.WithError(closeErr).Error("Failed to close rows")
		}
	}()

	events := make([]models.AgentExecutionEvent, 0)
	for rows.Next() {
		var event models.AgentExecutionEvent
		var eventDataJSON []byte

		err := rows.Scan(
			&event.ID, &event.ExecutionID, &event.EventType, &eventDataJSON,
			&event.SequenceNumber, &event.ReceivedAt,
		)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"method":       "ListByExecutionID",
				"execution_id": executionID,
				"error":        fmt.Sprintf("%+v", err),
			}).Error("Failed to scan agent execution event row")
			return nil, 0, fmt.Errorf("failed to scan agent execution event: %w", err)
		}

		if len(eventDataJSON) > 0 {
			if err := json.Unmarshal(eventDataJSON, &event.EventData); err != nil {
				logger.WithFields(logrus.Fields{
					"method":       "ListByExecutionID",
					"execution_id": executionID,
					"event_id":     event.ID,
					"error":        fmt.Sprintf("%+v", err),
				}).Warn("Failed to unmarshal event_data, continuing with empty event_data")
				event.EventData = make(map[string]interface{})
			}
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		logger.WithFields(logrus.Fields{
			"method":       "ListByExecutionID",
			"execution_id": executionID,
			"error":        fmt.Sprintf("%+v", err),
		}).Error("Error iterating agent execution event rows")
		return nil, 0, fmt.Errorf("failed to iterate agent execution events: %w", err)
	}

	return events, totalCount, nil
}

// ListAfterSequence gets all events after a specific sequence number (for cursor-based polling)
//
//nolint:funlen,lll // Repository code with necessary complexity
func (r *agentExecutionEventRepository) ListAfterSequence(ctx context.Context, executionID string, afterSequence int) ([]models.AgentExecutionEvent, error) {
	logger := contextkeys.GetLoggerFromContext(ctx)

	query := `
		SELECT id, execution_id, event_type, event_data, sequence_number, received_at
		FROM agent_execution_events
		WHERE execution_id = $1 AND sequence_number > $2
		ORDER BY sequence_number ASC
	`

	rows, err := r.db.QueryContext(ctx, query, executionID, afterSequence)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"method":         "ListAfterSequence",
			"execution_id":   executionID,
			"after_sequence": afterSequence,
			"error":          fmt.Sprintf("%+v", err),
		}).Error("Failed to list agent execution events after sequence")
		return nil, fmt.Errorf("failed to list agent execution events after sequence: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			logger.WithError(closeErr).Error("Failed to close rows")
		}
	}()

	events := make([]models.AgentExecutionEvent, 0)
	for rows.Next() {
		var event models.AgentExecutionEvent
		var eventDataJSON []byte

		err := rows.Scan(
			&event.ID, &event.ExecutionID, &event.EventType, &eventDataJSON,
			&event.SequenceNumber, &event.ReceivedAt,
		)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"method":       "ListAfterSequence",
				"execution_id": executionID,
				"error":        fmt.Sprintf("%+v", err),
			}).Error("Failed to scan agent execution event row")
			return nil, fmt.Errorf("failed to scan agent execution event: %w", err)
		}

		if len(eventDataJSON) > 0 {
			if err := json.Unmarshal(eventDataJSON, &event.EventData); err != nil {
				logger.WithFields(logrus.Fields{
					"method":       "ListAfterSequence",
					"execution_id": executionID,
					"event_id":     event.ID,
					"error":        fmt.Sprintf("%+v", err),
				}).Warn("Failed to unmarshal event_data, continuing with empty event_data")
				event.EventData = make(map[string]interface{})
			}
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		logger.WithFields(logrus.Fields{
			"method":         "ListAfterSequence",
			"execution_id":   executionID,
			"after_sequence": afterSequence,
			"error":          fmt.Sprintf("%+v", err),
		}).Error("Error iterating agent execution event rows")
		return nil, fmt.Errorf("failed to iterate agent execution events: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"method":         "ListAfterSequence",
		"execution_id":   executionID,
		"after_sequence": afterSequence,
		"event_count":    len(events),
	}).Debug("Events retrieved after sequence")

	return events, nil
}

// GetLatestByExecutionID gets the most recent event for an execution
func (r *agentExecutionEventRepository) GetLatestByExecutionID(
	ctx context.Context, executionID string,
) (*models.AgentExecutionEvent, error) {
	logger := contextkeys.GetLoggerFromContext(ctx)

	query := `
		SELECT id, execution_id, event_type, event_data, sequence_number, received_at
		FROM agent_execution_events
		WHERE execution_id = $1
		ORDER BY sequence_number DESC
		LIMIT 1
	`

	var event models.AgentExecutionEvent
	var eventDataJSON []byte

	err := r.db.QueryRowContext(ctx, query, executionID).Scan(
		&event.ID, &event.ExecutionID, &event.EventType, &eventDataJSON, &event.SequenceNumber, &event.ReceivedAt)

	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			logger.WithFields(logrus.Fields{
				"method":       "GetLatestByExecutionID",
				"execution_id": executionID,
				"error":        fmt.Sprintf("%+v", err),
			}).Error("Failed to get latest agent execution event")
		}
		return nil, mapNoRows(
			fmt.Errorf("failed to get latest agent execution event: %w", err),
			repositories.ErrAgentExecutionEventNotFound,
		)
	}

	if len(eventDataJSON) > 0 {
		if err := json.Unmarshal(eventDataJSON, &event.EventData); err != nil {
			logger.WithFields(logrus.Fields{
				"method":       "GetLatestByExecutionID",
				"execution_id": executionID,
				"event_id":     event.ID,
				"error":        fmt.Sprintf("%+v", err),
			}).Error("Failed to unmarshal event_data")
			return nil, fmt.Errorf("failed to unmarshal event_data: %w", err)
		}
	}

	return &event, nil
}

// CountByExecutionID returns the total number of events for an execution
func (r *agentExecutionEventRepository) CountByExecutionID(ctx context.Context, executionID string) (int, error) {
	logger := contextkeys.GetLoggerFromContext(ctx)

	query := `SELECT COUNT(*) FROM agent_execution_events WHERE execution_id = $1`

	var count int
	err := r.db.QueryRowContext(ctx, query, executionID).Scan(&count)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"method":       "CountByExecutionID",
			"execution_id": executionID,
			"error":        fmt.Sprintf("%+v", err),
		}).Error("Failed to count agent execution events")
		return 0, fmt.Errorf("failed to count agent execution events: %w", err)
	}

	return count, nil
}
