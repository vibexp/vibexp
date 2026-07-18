package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

type agentExecutionRepository struct {
	db *database.DB
}

// NewAgentExecutionRepository creates a new agent execution repository
func NewAgentExecutionRepository(db *database.DB) repositories.AgentExecutionRepository {
	return &agentExecutionRepository{db: db}
}

// Create creates a new agent execution
func (r *agentExecutionRepository) Create(ctx context.Context, execution *models.AgentExecution) error {
	logger := contextkeys.GetLoggerFromContext(ctx)

	execution.ID = uuid.New().String()
	execution.StartedAt = time.Now()

	if execution.Status == "" {
		execution.Status = "running"
	}

	inputJSON, err := json.Marshal(execution.Input)
	if err != nil {
		return fmt.Errorf("failed to marshal input: %w", err)
	}

	// RETURNING version so the in-memory struct stays in sync with the DB-assigned
	// version (default 1). Without this, execution.Version stays 0, and the later
	// optimistic-locking Update (WHERE version = $n) silently matches no rows —
	// e.g. a streaming error would never persist its "error" status (issue #197).
	query := `
		INSERT INTO agent_executions (id, agent_id, user_id, status, input, started_at, conversation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING version
	`

	var conversationID interface{}
	if execution.ConversationID != nil {
		conversationID = *execution.ConversationID
	}

	err = r.db.QueryRowContext(ctx, query,
		execution.ID, execution.AgentID, execution.UserID, execution.Status,
		inputJSON, execution.StartedAt, conversationID).Scan(&execution.Version)

	if err != nil {
		logger.With(
			"method", "Create",
			"agent_id", execution.AgentID,
			"conversation_id", conversationID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create agent execution")
		return fmt.Errorf("failed to create agent execution: %w", err)
	}

	logger.With(
		"method", "Create",
		"execution_id", execution.ID,
		"agent_id", execution.AgentID,
		"conversation_id", conversationID,
	).Debug("Agent execution created")

	return nil
}

// nullTimePtr returns a pointer to the time value when valid, nil otherwise.
func nullTimePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// nullStringPtr returns a pointer to the string value when valid, nil otherwise.
func nullStringPtr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

// nullIntPtr returns a pointer to the int64 value as int when valid, nil otherwise.
func nullIntPtr(i sql.NullInt64) *int {
	if !i.Valid {
		return nil
	}
	v := int(i.Int64)
	return &v
}

// GetByID retrieves an agent execution by ID
func (r *agentExecutionRepository) GetByID(ctx context.Context, userID, executionID string) (*models.AgentExecution, error) { //nolint:lll
	query := `
		SELECT id, agent_id, user_id, status, input, error, started_at, ended_at, duration,
		       task_id, context_id, current_state, artifacts, conversation_id, version
		FROM agent_executions
		WHERE id = $1 AND user_id = $2
	`

	var execution models.AgentExecution
	var inputJSON, artifactsJSON []byte
	var endedAt sql.NullTime
	var errorMsg, taskID, contextID, currentState, conversationID sql.NullString
	var duration sql.NullInt64

	err := r.db.QueryRowContext(ctx, query, executionID, userID).Scan(
		&execution.ID, &execution.AgentID, &execution.UserID, &execution.Status,
		&inputJSON, &errorMsg, &execution.StartedAt, &endedAt, &duration,
		&taskID, &contextID, &currentState, &artifactsJSON, &conversationID, &execution.Version)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repositories.ErrAgentExecutionNotFound
		}
		slog.Error(
			"Failed to get agent execution by ID",
			"service", "vibexp-api",
			"method", "GetByID",
			"execution_id", executionID,
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		)
		return nil, fmt.Errorf("failed to get agent execution: %w", err)
	}

	execution.EndedAt = nullTimePtr(endedAt)
	execution.Error = nullStringPtr(errorMsg)
	execution.Duration = nullIntPtr(duration)

	// Handle A2A streaming fields
	execution.TaskID = nullStringPtr(taskID)
	execution.ContextID = nullStringPtr(contextID)
	execution.CurrentState = nullStringPtr(currentState)
	execution.ConversationID = nullStringPtr(conversationID)

	if err := unmarshalLoggedField(
		inputJSON, &execution.Input, "input", "Failed to unmarshal input", executionID,
	); err != nil {
		return nil, err
	}
	if err := unmarshalLoggedField(
		artifactsJSON, &execution.Artifacts, "artifacts", "Failed to unmarshal artifacts", executionID,
	); err != nil {
		return nil, err
	}

	return &execution, nil
}

// unmarshalLoggedField unmarshals an optional JSON column into dest, logging
// and wrapping errors with the same fields GetByID always emitted.
func unmarshalLoggedField(data []byte, dest any, field, logMsg, executionID string) error {
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, dest); err != nil {
		slog.Error(
			logMsg,
			"service", "vibexp-api",
			"method", "GetByID",
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		)
		return fmt.Errorf("failed to unmarshal %s: %w", field, err)
	}
	return nil
}

// buildAgentExecutionListWhereClause builds the shared WHERE conditions for the
// agent execution list query. The count and page queries consume the same
// conditions, so they can never diverge. user_id is always constrained; the
// remaining filters are appended only when their pointer is non-nil (no
// empty-string checks — matching the prior hand-built query exactly).
func buildAgentExecutionListWhereClause(
	userID string, filters repositories.AgentExecutionFilters,
) squirrel.Sqlizer {
	where := squirrel.And{squirrel.Eq{"user_id": userID}}

	if filters.AgentID != nil {
		where = append(where, squirrel.Eq{"agent_id": *filters.AgentID})
	}
	if filters.Status != nil {
		where = append(where, squirrel.Eq{"status": *filters.Status})
	}
	if filters.DateFrom != nil {
		where = append(where, squirrel.GtOrEq{"started_at": *filters.DateFrom})
	}
	if filters.DateTo != nil {
		where = append(where, squirrel.LtOrEq{"started_at": *filters.DateTo})
	}

	return where
}

// List retrieves agent executions with filtering and pagination
func (r *agentExecutionRepository) List(
	ctx context.Context, userID string, filters repositories.AgentExecutionFilters,
) ([]models.AgentExecution, int, error) {
	// Shared WHERE conditions guarantee the count and page queries can never diverge.
	where := buildAgentExecutionListWhereClause(userID, filters)

	totalCount, err := r.countList(ctx, where)
	if err != nil {
		return nil, 0, err
	}

	executions, err := r.queryList(ctx, where, filters)
	if err != nil {
		return nil, 0, err
	}

	return executions, totalCount, nil
}

// countList counts agent executions matching the shared WHERE conditions used
// by List, so the count and page queries can never diverge.
func (r *agentExecutionRepository) countList(ctx context.Context, where squirrel.Sqlizer) (int, error) {
	query, args, err := psql.
		Select("COUNT(*)").
		From("agent_executions").
		Where(where).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build agent executions count query: %w", err)
	}

	var totalCount int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); err != nil {
		slog.Error(
			"Failed to count agent executions",
			"service", "vibexp-api",
			"method", "List",
			"error", fmt.Sprintf("%+v", err),
		)
		return 0, fmt.Errorf("failed to count agent executions: %w", err)
	}

	return totalCount, nil
}

// agentExecutionPaging resolves the LIMIT/OFFSET for the List page query.
//
// Defaulting contract (NOT clamping): this repository historically treats a
// non-positive Limit as "use the default page size 10" and a non-positive Page
// as "page 1". This differs deliberately from the LIMIT-0 clamp convention of
// the other migrated repositories; the defaulting predates the squirrel
// migration and is preserved verbatim. After defaulting both values are
// guaranteed positive, so the conversions cannot wrap; the explicit `> 0`
// guards make that provable to gosec (G115).
func agentExecutionPaging(filters repositories.AgentExecutionFilters) (limit, offset uint64) {
	pageSize := filters.Limit
	if pageSize <= 0 {
		pageSize = 10
	}
	page := filters.Page
	if page <= 0 {
		page = 1
	}

	if pageSize > 0 {
		limit = uint64(pageSize)
		if rawOffset := (page - 1) * pageSize; rawOffset > 0 {
			offset = uint64(rawOffset)
		}
	}
	return limit, offset
}

// queryList runs the paginated page query for List using the shared WHERE
// conditions, so it can never diverge from the count query.
func (r *agentExecutionRepository) queryList(
	ctx context.Context, where squirrel.Sqlizer, filters repositories.AgentExecutionFilters,
) ([]models.AgentExecution, error) {
	limit, offset := agentExecutionPaging(filters)

	query, args, err := psql.
		Select(
			"id", "agent_id", "user_id", "status", "input",
			"error", "started_at", "ended_at", "duration",
		).
		From("agent_executions").
		Where(where).
		OrderBy("started_at DESC").
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build agent executions list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		slog.Error(
			"Failed to list agent executions",
			"service", "vibexp-api",
			"method", "List",
			"error", fmt.Sprintf("%+v", err),
		)
		return nil, fmt.Errorf("failed to list agent executions: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	executions, err := scanAgentExecutionListRows(rows)
	if err != nil {
		return nil, err
	}

	if err := rows.Err(); err != nil {
		slog.Error(
			"Error iterating agent execution rows",
			"service", "vibexp-api",
			"method", "List",
			"error", fmt.Sprintf("%+v", err),
		)
		return nil, fmt.Errorf("failed to iterate agent executions: %w", err)
	}

	return executions, nil
}

// scanAgentExecutionListRows scans the 9-column projection used by the List page
// query, mapping nullable columns to pointers. A malformed input JSON column is
// logged at WARN and the row is kept with an empty Input map (never an error) —
// this matches the prior hand-built behaviour exactly.
func scanAgentExecutionListRows(rows *sql.Rows) ([]models.AgentExecution, error) {
	executions := make([]models.AgentExecution, 0)
	for rows.Next() {
		var execution models.AgentExecution
		var inputJSON []byte
		var endedAt sql.NullTime
		var errorMsg sql.NullString
		var duration sql.NullInt64

		if scanErr := rows.Scan(&execution.ID, &execution.AgentID, &execution.UserID,
			&execution.Status, &inputJSON, &errorMsg,
			&execution.StartedAt, &endedAt, &duration); scanErr != nil {
			slog.Error(
				"Failed to scan agent execution row",
				"service", "vibexp-api",
				"method", "List",
				"error", fmt.Sprintf("%+v", scanErr),
			)
			return nil, fmt.Errorf("failed to scan agent execution: %w", scanErr)
		}

		if endedAt.Valid {
			execution.EndedAt = &endedAt.Time
		}
		if errorMsg.Valid {
			execution.Error = &errorMsg.String
		}
		if duration.Valid {
			durationInt := int(duration.Int64)
			execution.Duration = &durationInt
		}

		if len(inputJSON) > 0 {
			if jsonErr := json.Unmarshal(inputJSON, &execution.Input); jsonErr != nil {
				slog.Warn(
					"Failed to unmarshal input, continuing with empty input",
					"service", "vibexp-api",
					"method", "List",
					"execution_id", execution.ID,
					"error", fmt.Sprintf("%+v", jsonErr),
				)
				execution.Input = make(map[string]interface{})
			}
		}

		executions = append(executions, execution)
	}

	return executions, nil
}

// Update updates an existing agent execution
func (r *agentExecutionRepository) Update(ctx context.Context, execution *models.AgentExecution) error {
	// Calculate duration if ended
	var duration *int
	if execution.EndedAt != nil {
		d := int(execution.EndedAt.Sub(execution.StartedAt).Milliseconds())
		duration = &d
	}

	query := `
		UPDATE agent_executions
		SET status = $1, error = $2, ended_at = $3, duration = $4, version = version + 1
		WHERE id = $5 AND user_id = $6 AND version = $7
		RETURNING version
	`

	err := r.db.QueryRowContext(ctx, query,
		execution.Status, execution.Error, execution.EndedAt, duration,
		execution.ID, execution.UserID, execution.Version,
	).Scan(&execution.Version)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("agent execution not found or version mismatch")
		}
		slog.Error(
			"Failed to update agent execution",
			"service", "vibexp-api",
			"method", "Update",
			"execution_id", execution.ID,
			"error", fmt.Sprintf("%+v", err),
		)
		return fmt.Errorf("failed to update agent execution: %w", err)
	}

	return nil
}

// GetByAgentID retrieves agent executions by agent ID
func (r *agentExecutionRepository) GetByAgentID(
	ctx context.Context, userID, agentID string, filters repositories.AgentExecutionFilters,
) ([]models.AgentExecution, int, error) {
	filters.AgentID = &agentID
	return r.List(ctx, userID, filters)
}

// GetByTaskID finds an execution by A2A task identifier
//
//nolint:gocyclo,funlen,lll // Repository code with necessary complexity
func (r *agentExecutionRepository) GetByTaskID(ctx context.Context, userID, taskID string) (*models.AgentExecution, error) {
	query := `
		SELECT id, agent_id, user_id, status, input, error, started_at, ended_at, duration,
		       task_id, context_id, current_state, artifacts
		FROM agent_executions
		WHERE task_id = $1 AND user_id = $2
	`

	var execution models.AgentExecution
	var inputJSON, artifactsJSON []byte
	var endedAt sql.NullTime
	var errorMsg sql.NullString
	var duration sql.NullInt64
	var taskIDVal, contextIDVal, currentStateVal sql.NullString

	err := r.db.QueryRowContext(ctx, query, taskID, userID).Scan(
		&execution.ID, &execution.AgentID, &execution.UserID, &execution.Status,
		&inputJSON, &errorMsg, &execution.StartedAt, &endedAt, &duration,
		&taskIDVal, &contextIDVal, &currentStateVal, &artifactsJSON)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repositories.ErrAgentExecutionNotFound
		}
		slog.Error(
			"Failed to get agent execution by task ID",
			"service", "vibexp-api",
			"method", "GetByTaskID",
			"task_id", taskID,
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		)
		return nil, fmt.Errorf("failed to get agent execution by task ID: %w", err)
	}

	// Handle nullable fields
	if endedAt.Valid {
		execution.EndedAt = &endedAt.Time
	}

	if errorMsg.Valid {
		execution.Error = &errorMsg.String
	}

	if duration.Valid {
		durationInt := int(duration.Int64)
		execution.Duration = &durationInt
	}

	if taskIDVal.Valid {
		execution.TaskID = &taskIDVal.String
	}

	if contextIDVal.Valid {
		execution.ContextID = &contextIDVal.String
	}

	if currentStateVal.Valid {
		execution.CurrentState = &currentStateVal.String
	}

	// Unmarshal JSON fields
	if len(inputJSON) > 0 {
		if err := json.Unmarshal(inputJSON, &execution.Input); err != nil {
			slog.Error(
				"Failed to unmarshal input",
				"service", "vibexp-api",
				"method", "GetByTaskID",
				"task_id", taskID,
				"error", fmt.Sprintf("%+v", err),
			)
			return nil, fmt.Errorf("failed to unmarshal input: %w", err)
		}
	}

	if len(artifactsJSON) > 0 {
		if err := json.Unmarshal(artifactsJSON, &execution.Artifacts); err != nil {
			slog.Error(
				"Failed to unmarshal artifacts",
				"service", "vibexp-api",
				"method", "GetByTaskID",
				"task_id", taskID,
				"error", fmt.Sprintf("%+v", err),
			)
			return nil, fmt.Errorf("failed to unmarshal artifacts: %w", err)
		}
	}

	return &execution, nil
}

// UpdateTaskInfo updates A2A-specific fields (task_id, context_id, current_state)
func (r *agentExecutionRepository) UpdateTaskInfo(
	ctx context.Context, executionID, taskID, contextID, currentState string,
) error {
	query := `
		UPDATE agent_executions
		SET task_id = $1, context_id = $2, current_state = $3
		WHERE id = $4
	`

	result, err := r.db.ExecContext(ctx, query, taskID, contextID, currentState, executionID)
	if err != nil {
		slog.Error(
			"Failed to update agent execution task info",
			"service", "vibexp-api",
			"method", "UpdateTaskInfo",
			"execution_id", executionID,
			"task_id", taskID,
			"context_id", contextID,
			"current_state", currentState,
			"error", fmt.Sprintf("%+v", err),
		)
		return fmt.Errorf("failed to update agent execution task info: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrAgentExecutionNotFound
	}

	slog.Info(
		"Agent execution task info updated",
		"service", "vibexp-api",
		"method", "UpdateTaskInfo",
		"execution_id", executionID,
		"task_id", taskID,
		"context_id", contextID,
		"current_state", currentState,
	)

	return nil
}

// UpdateStatus updates the execution status to a terminal state with ended_at and duration
// This method doesn't require user_id as it's used for system operations like stream finalization
func (r *agentExecutionRepository) UpdateStatus(ctx context.Context, executionID, status string) error {
	query := `
		UPDATE agent_executions
		SET status = $1, ended_at = $2, duration = EXTRACT(EPOCH FROM ($2 - started_at)) * 1000
		WHERE id = $3 AND status = 'pending'
	`

	endedAt := time.Now()
	result, err := r.db.ExecContext(ctx, query, status, endedAt, executionID)
	if err != nil {
		slog.Error(
			"Failed to update agent execution status",
			"service", "vibexp-api",
			"method", "UpdateStatus",
			"execution_id", executionID,
			"status", status,
			"error", fmt.Sprintf("%+v", err),
		)
		return fmt.Errorf("failed to update agent execution status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		// This is not an error - execution may already be finalized
		slog.Debug(
			"Execution not updated - may already be finalized",
			"service", "vibexp-api",
			"method", "UpdateStatus",
			"execution_id", executionID,
			"status", status,
		)
		return nil
	}

	slog.Info(
		"Agent execution status updated",
		"service", "vibexp-api",
		"method", "UpdateStatus",
		"execution_id", executionID,
		"status", status,
	)

	return nil
}

// UpdateArtifacts updates the assembled artifacts array
func (r *agentExecutionRepository) UpdateArtifacts(
	ctx context.Context, executionID string, artifacts []map[string]interface{},
) error {
	artifactsJSON, err := json.Marshal(artifacts)
	if err != nil {
		slog.Error(
			"Failed to marshal artifacts",
			"service", "vibexp-api",
			"method", "UpdateArtifacts",
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		)
		return fmt.Errorf("failed to marshal artifacts: %w", err)
	}

	query := `
		UPDATE agent_executions
		SET artifacts = $1
		WHERE id = $2
	`

	result, err := r.db.ExecContext(ctx, query, artifactsJSON, executionID)
	if err != nil {
		slog.Error(
			"Failed to update agent execution artifacts",
			"service", "vibexp-api",
			"method", "UpdateArtifacts",
			"execution_id", executionID,
			"error", fmt.Sprintf("%+v", err),
		)
		return fmt.Errorf("failed to update agent execution artifacts: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrAgentExecutionNotFound
	}

	slog.Info(
		"Agent execution artifacts updated",
		"service", "vibexp-api",
		"method", "UpdateArtifacts",
		"execution_id", executionID,
		"artifact_count", len(artifacts),
	)

	return nil
}

// unmarshalExecutionPayloads decodes the input and artifacts JSON columns into
// the execution, leaving absent columns untouched.
func unmarshalExecutionPayloads(execution *models.AgentExecution, inputJSON, artifactsJSON []byte) error {
	if len(inputJSON) > 0 {
		if jsonErr := json.Unmarshal(inputJSON, &execution.Input); jsonErr != nil {
			return fmt.Errorf("failed to unmarshal input: %w", jsonErr)
		}
	}
	if len(artifactsJSON) > 0 {
		if jsonErr := json.Unmarshal(artifactsJSON, &execution.Artifacts); jsonErr != nil {
			return fmt.Errorf("failed to unmarshal artifacts: %w", jsonErr)
		}
	}
	return nil
}

// scanConversationExecutionRows scans the GetByConversationID projection into
// a non-nil slice, preserving that method's exact error wrapping and log
// fields.
func scanConversationExecutionRows(rows *sql.Rows, userID, conversationID string) ([]models.AgentExecution, error) {
	executions := make([]models.AgentExecution, 0)
	for rows.Next() {
		var execution models.AgentExecution
		var inputJSON, artifactsJSON []byte

		scanErr := rows.Scan(
			&execution.ID,
			&execution.AgentID,
			&execution.UserID,
			&execution.Status,
			&inputJSON,
			&execution.Error,
			&execution.StartedAt,
			&execution.EndedAt,
			&execution.Duration,
			&execution.TaskID,
			&execution.ContextID,
			&execution.CurrentState,
			&artifactsJSON,
			&execution.ConversationID,
		)
		if scanErr != nil {
			slog.Error(
				"Failed to scan execution row",
				"service", "vibexp-api",
				"method", "GetByConversationID",
				"user_id", userID,
				"conversation_id", conversationID,
				"error", fmt.Sprintf("%+v", scanErr),
			)
			return nil, fmt.Errorf("failed to scan execution: %w", scanErr)
		}

		if jsonErr := unmarshalExecutionPayloads(&execution, inputJSON, artifactsJSON); jsonErr != nil {
			return nil, jsonErr
		}

		executions = append(executions, execution)
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("error iterating rows: %w", rowsErr)
	}

	return executions, nil
}

// GetByConversationID retrieves executions in a conversation with pagination
// Returns: executions, hasMore, totalCount, error
// buildConversationExecutionsQuery assembles the paginated per-conversation
// query: an optional before filter, then DESC ordering (callers reverse in
// code for chronological display).
// countConversationExecutions returns the conversation's total execution
// count, falling back to the already-loaded count on query failure (warned,
// as before).
func (r *agentExecutionRepository) countConversationExecutions(
	ctx context.Context, userID, conversationID string, fallback int,
) int {
	countQuery := `SELECT COUNT(*) FROM agent_executions WHERE user_id = $1 AND conversation_id = $2`
	var totalCount int
	if err := r.db.QueryRowContext(ctx, countQuery, userID, conversationID).Scan(&totalCount); err != nil {
		slog.Warn(
			"Failed to get total count",
			"service", "vibexp-api",
			"method", "GetByConversationID",
			"user_id", userID,
			"conversation_id", conversationID,
			"error", fmt.Sprintf("%+v", err),
		)
		return fallback
	}
	return totalCount
}

func buildConversationExecutionsQuery(
	userID, conversationID string, limit int, before *time.Time,
) (string, []interface{}) {
	query := `
		SELECT id, agent_id, user_id, status, input, error, started_at, ended_at, duration,
		       task_id, context_id, current_state, artifacts, conversation_id
		FROM agent_executions
		WHERE user_id = $1 AND conversation_id = $2
	`
	args := []interface{}{userID, conversationID}
	if before != nil {
		query += ` AND started_at < $3`
		args = append(args, before)
	}
	query += ` ORDER BY started_at DESC LIMIT $` + fmt.Sprintf("%d", len(args)+1)
	args = append(args, limit)
	return query, args
}

func (r *agentExecutionRepository) GetByConversationID(
	ctx context.Context,
	userID, conversationID string,
	limit int,
	before *time.Time,
) ([]models.AgentExecution, bool, int, error) {
	query, args := buildConversationExecutionsQuery(userID, conversationID, limit, before)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		slog.Error(
			"Failed to query executions by conversation",
			"service", "vibexp-api",
			"method", "GetByConversationID",
			"user_id", userID,
			"conversation_id", conversationID,
			"error", fmt.Sprintf("%+v", err),
		)
		return nil, false, 0, fmt.Errorf("failed to query executions: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	executions, err := scanConversationExecutionRows(rows, userID, conversationID)
	if err != nil {
		return nil, false, 0, err
	}

	// Reverse to get chronological order (oldest first)
	for i, j := 0, len(executions)-1; i < j; i, j = i+1, j-1 {
		executions[i], executions[j] = executions[j], executions[i]
	}

	// Check if there are more messages
	hasMore := len(executions) == limit

	totalCount := r.countConversationExecutions(ctx, userID, conversationID, len(executions))

	slog.Info(
		"Retrieved executions by conversation",
		"service", "vibexp-api",
		"method", "GetByConversationID",
		"user_id", userID,
		"conversation_id", conversationID,
		"count", len(executions),
		"total_count", totalCount,
		"has_more", hasMore,
	)

	return executions, hasMore, totalCount, nil
}

// GetFirstExecutionInConversation retrieves the first execution in a conversation
// This is useful for getting the context_id to use for continuing the conversation
//
//nolint:funlen,lll // Repository code with necessary complexity
func (r *agentExecutionRepository) GetFirstExecutionInConversation(ctx context.Context, userID, conversationID string) (*models.AgentExecution, error) {
	query := `
		SELECT id, agent_id, user_id, status, input, error, started_at, ended_at, duration,
		       task_id, context_id, current_state, artifacts, conversation_id
		FROM agent_executions
		WHERE user_id = $1 AND conversation_id = $2
		ORDER BY started_at ASC
		LIMIT 1
	`

	var execution models.AgentExecution
	var inputJSON, artifactsJSON []byte

	err := r.db.QueryRowContext(ctx, query, userID, conversationID).Scan(
		&execution.ID,
		&execution.AgentID,
		&execution.UserID,
		&execution.Status,
		&inputJSON,
		&execution.Error,
		&execution.StartedAt,
		&execution.EndedAt,
		&execution.Duration,
		&execution.TaskID,
		&execution.ContextID,
		&execution.CurrentState,
		&artifactsJSON,
		&execution.ConversationID,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, repositories.ErrConversationNotFound
	}

	if err != nil {
		slog.Error(
			"Failed to get first execution in conversation",
			"service", "vibexp-api",
			"method", "GetFirstExecutionInConversation",
			"user_id", userID,
			"conversation_id", conversationID,
			"error", fmt.Sprintf("%+v", err),
		)
		return nil, fmt.Errorf("failed to get first execution: %w", err)
	}

	if len(inputJSON) > 0 {
		if err := json.Unmarshal(inputJSON, &execution.Input); err != nil {
			return nil, fmt.Errorf("failed to unmarshal input: %w", err)
		}
	}

	if len(artifactsJSON) > 0 {
		if err := json.Unmarshal(artifactsJSON, &execution.Artifacts); err != nil {
			return nil, fmt.Errorf("failed to unmarshal artifacts: %w", err)
		}
	}

	slog.Info(
		"Retrieved first execution in conversation",
		"service", "vibexp-api",
		"method", "GetFirstExecutionInConversation",
		"user_id", userID,
		"conversation_id", conversationID,
		"execution_id", execution.ID,
	)

	return &execution, nil
}

// UpdateConversationID updates the conversation_id for an execution
func (r *agentExecutionRepository) UpdateConversationID(ctx context.Context, executionID, conversationID string) error {
	query := `
		UPDATE agent_executions
		SET conversation_id = $1
		WHERE id = $2
	`

	result, err := r.db.ExecContext(ctx, query, conversationID, executionID)
	if err != nil {
		slog.Error(
			"Failed to update conversation_id",
			"service", "vibexp-api",
			"method", "UpdateConversationID",
			"execution_id", executionID,
			"conversation_id", conversationID,
			"error", fmt.Sprintf("%+v", err),
		)
		return fmt.Errorf("failed to update conversation_id: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrAgentExecutionNotFound
	}

	slog.Info(
		"Updated conversation_id",
		"service", "vibexp-api",
		"method", "UpdateConversationID",
		"execution_id", executionID,
		"conversation_id", conversationID,
	)
	//nolint:funlen // Repository code with necessary complexity

	return nil
}

// ListConversations retrieves conversation summaries for an agent with pagination
//
//nolint:funlen // Repository code with necessary complexity
func (r *agentExecutionRepository) ListConversations(
	ctx context.Context,
	userID, agentID string,
	page, limit int,
) ([]models.ConversationSummary, int, error) {
	offset := (page - 1) * limit

	// Build query conditionally based on whether agentID is provided
	// When agentID is empty, we want to list all conversations across all agents
	var query string
	var rows *sql.Rows
	var err error

	if agentID == "" {
		// Query for all conversations across all agents
		query = `
			WITH conversation_data AS (
				SELECT
					conversation_id,
					agent_id,
					COUNT(*) as message_count,
					MIN(started_at) as started_at,
					MAX(started_at) as last_activity_at,
					(SELECT input->>'text'
					 FROM agent_executions
					 WHERE conversation_id = ae.conversation_id
					   AND user_id = ae.user_id
					 ORDER BY started_at ASC
					 LIMIT 1) as first_message,
					(SELECT input->>'text'
					 FROM agent_executions
					 WHERE conversation_id = ae.conversation_id
					   AND user_id = ae.user_id
					 ORDER BY started_at DESC
					 LIMIT 1) as last_message,
					(SELECT status
					 FROM agent_executions
					 WHERE conversation_id = ae.conversation_id
					   AND user_id = ae.user_id
					 ORDER BY started_at DESC
					 LIMIT 1) as last_status
				FROM agent_executions ae
				WHERE user_id = $1
				  AND conversation_id IS NOT NULL
				GROUP BY conversation_id, agent_id, user_id
			)
			SELECT
				conversation_id,
				agent_id,
				message_count,
				COALESCE(first_message, '') as first_message,
				COALESCE(last_message, '') as last_message,
				started_at,
				last_activity_at,
				last_status
			FROM conversation_data
			ORDER BY last_activity_at DESC
			LIMIT $2 OFFSET $3
		`
		rows, err = r.db.QueryContext(ctx, query, userID, limit, offset)
	} else {
		// Query for conversations filtered by specific agent
		query = `
			WITH conversation_data AS (
				SELECT
					conversation_id,
					agent_id,
					COUNT(*) as message_count,
					MIN(started_at) as started_at,
					MAX(started_at) as last_activity_at,
					(SELECT input->>'text'
					 FROM agent_executions
					 WHERE conversation_id = ae.conversation_id
					   AND user_id = ae.user_id
					 ORDER BY started_at ASC
					 LIMIT 1) as first_message,
					(SELECT input->>'text'
					 FROM agent_executions
					 WHERE conversation_id = ae.conversation_id
					   AND user_id = ae.user_id
					 ORDER BY started_at DESC
					 LIMIT 1) as last_message,
					(SELECT status
					 FROM agent_executions
					 WHERE conversation_id = ae.conversation_id
					   AND user_id = ae.user_id
					 ORDER BY started_at DESC
					 LIMIT 1) as last_status
				FROM agent_executions ae
				WHERE user_id = $1
				  AND agent_id = $2
				  AND conversation_id IS NOT NULL
				GROUP BY conversation_id, agent_id, user_id
			)
			SELECT
				conversation_id,
				agent_id,
				message_count,
				COALESCE(first_message, '') as first_message,
				COALESCE(last_message, '') as last_message,
				started_at,
				last_activity_at,
				last_status
			FROM conversation_data
			ORDER BY last_activity_at DESC
			LIMIT $3 OFFSET $4
		`
		rows, err = r.db.QueryContext(ctx, query, userID, agentID, limit, offset)
	}
	if err != nil {
		slog.Error(
			"Failed to query conversations",
			"service", "vibexp-api",
			"method", "ListConversations",
			"user_id", userID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		)
		return nil, 0, fmt.Errorf("failed to query conversations: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	conversations := make([]models.ConversationSummary, 0)
	for rows.Next() {
		var conv models.ConversationSummary
		scanErr := rows.Scan(
			&conv.ConversationID,
			&conv.AgentID,
			&conv.MessageCount,
			&conv.FirstMessage,
			&conv.LastMessage,
			&conv.StartedAt,
			&conv.LastActivityAt,
			&conv.LastStatus,
		)
		if scanErr != nil {
			slog.Error(
				"Failed to scan conversation row",
				"service", "vibexp-api",
				"method", "ListConversations",
				"user_id", userID,
				"agent_id", agentID,
				"error", fmt.Sprintf("%+v", scanErr),
			)
			return nil, 0, fmt.Errorf("failed to scan conversation: %w", scanErr)
		}
		conversations = append(conversations, conv)
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, 0, fmt.Errorf("error iterating rows: %w", rowsErr)
	}

	// Get total count - conditionally filter by agent_id
	var totalCount int
	if agentID == "" {
		countQuery := `
			SELECT COUNT(DISTINCT conversation_id)
			FROM agent_executions
			WHERE user_id = $1 AND conversation_id IS NOT NULL
		`
		err = r.db.QueryRowContext(ctx, countQuery, userID).Scan(&totalCount)
	} else {
		countQuery := `
			SELECT COUNT(DISTINCT conversation_id)
			FROM agent_executions
			WHERE user_id = $1 AND agent_id = $2 AND conversation_id IS NOT NULL
		`
		err = r.db.QueryRowContext(ctx, countQuery, userID, agentID).Scan(&totalCount)
	}
	if err != nil {
		slog.Warn(
			"Failed to get total conversation count",
			"service", "vibexp-api",
			"method", "ListConversations",
			"user_id", userID,
			"agent_id", agentID,
			"error", fmt.Sprintf("%+v", err),
		)
		totalCount = len(conversations) // Fallback
	}

	slog.Info(
		"Retrieved conversation summaries",
		"service", "vibexp-api",
		"method", "ListConversations",
		"user_id", userID,
		"agent_id", agentID,
		"count", len(conversations),
		"total_count", totalCount,
		"page", page,
		"limit", limit,
	)

	return conversations, totalCount, nil
}
