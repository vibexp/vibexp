package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/contextkeys"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

func setupAgentExecutionTest(t *testing.T) (*agentExecutionRepository, sqlmock.Sqlmock, *sql.DB) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := NewAgentExecutionRepository(db).(*agentExecutionRepository)

	return repo, mock, mockDB
}

// contextWithLogger creates a context with a logger for testing
func contextWithLogger() context.Context {
	logger := slog.New(slog.DiscardHandler) // Suppress logs during tests
	return context.WithValue(context.Background(), contextkeys.Logger, logger)
}

//nolint:funlen // table-driven test with multiple test cases
func TestAgentExecutionRepository_Create(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()

	tests := []struct {
		name      string
		execution *models.AgentExecution
		setupMock func()
		expectErr bool
	}{
		{
			name: "successful create",
			execution: &models.AgentExecution{
				AgentID: "agent-123",
				UserID:  "user-123",
				Input:   map[string]interface{}{"text": "Hello"},
			},
			setupMock: func() {
				mock.ExpectExec(`INSERT INTO agent_executions`).
					WithArgs(
						sqlmock.AnyArg(), // id (generated)
						"agent-123",
						"user-123",
						"running",
						sqlmock.AnyArg(), // input JSON
						sqlmock.AnyArg(), // started_at
						nil,              // conversation_id
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			expectErr: false,
		},
		{
			name: "successful create with conversation_id",
			execution: &models.AgentExecution{
				AgentID:        "agent-123",
				UserID:         "user-123",
				Input:          map[string]interface{}{"text": "Continue"},
				ConversationID: strPtr("conv-123"),
			},
			setupMock: func() {
				mock.ExpectExec(`INSERT INTO agent_executions`).
					WithArgs(
						sqlmock.AnyArg(), // id
						"agent-123",
						"user-123",
						"running",
						sqlmock.AnyArg(), // input JSON
						sqlmock.AnyArg(), // started_at
						"conv-123",       // conversation_id
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			expectErr: false,
		},
		{
			name: "database error",
			execution: &models.AgentExecution{
				AgentID: "agent-error",
				UserID:  "user-error",
				Input:   map[string]interface{}{},
			},
			setupMock: func() {
				mock.ExpectExec(`INSERT INTO agent_executions`).
					WithArgs(
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.Create(ctx, tt.execution)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, tt.execution.ID)
				assert.Equal(t, "running", tt.execution.Status)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestAgentExecutionRepository_GetByID(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()
	now := time.Now()
	inputJSON, err := json.Marshal(map[string]interface{}{"text": "Hello"})
	require.NoError(t, err)

	tests := []struct {
		name        string
		userID      string
		executionID string
		setupMock   func()
		expectErr   bool
		validateFn  func(*testing.T, *models.AgentExecution)
	}{
		{
			name:        "successful retrieval",
			userID:      "user-123",
			executionID: "exec-123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "agent_id", "user_id", "status", "input", "error",
					"started_at", "ended_at", "duration", "task_id", "context_id",
					"current_state", "artifacts", "conversation_id", "version",
				}).AddRow(
					"exec-123", "agent-123", "user-123", "success", inputJSON, nil,
					now, nil, nil, nil, nil, nil, nil, nil, 1,
				)

				mock.ExpectQuery(`SELECT .+ FROM agent_executions WHERE id`).
					WithArgs("exec-123", "user-123").
					WillReturnRows(rows)
			},
			expectErr: false,
			validateFn: func(t *testing.T, exec *models.AgentExecution) {
				assert.Equal(t, "exec-123", exec.ID)
				assert.Equal(t, "agent-123", exec.AgentID)
				assert.Equal(t, "user-123", exec.UserID)
				assert.Equal(t, "success", exec.Status)
			},
		},
		{
			name:        "not found",
			userID:      "user-123",
			executionID: "exec-notfound",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM agent_executions WHERE id`).
					WithArgs("exec-notfound", "user-123").
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
		},
		{
			name:        "database error",
			userID:      "user-123",
			executionID: "exec-error",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM agent_executions WHERE id`).
					WithArgs("exec-error", "user-123").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetByID(ctx, tt.userID, tt.executionID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.validateFn != nil {
					tt.validateFn(t, result)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestAgentExecutionRepository_List(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()
	now := time.Now()
	inputJSON, err := json.Marshal(map[string]interface{}{"text": "Test"})
	require.NoError(t, err)

	tests := []struct {
		name        string
		userID      string
		filters     repositories.AgentExecutionFilters
		setupMock   func()
		expectErr   bool
		expectCount int
		expectTotal int
	}{
		{
			name:   "successful list with default pagination",
			userID: "user-123",
			filters: repositories.AgentExecutionFilters{
				Limit: 10,
				Page:  1,
			},
			setupMock: func() {
				// Count query
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				// List query
				rows := sqlmock.NewRows([]string{
					"id", "agent_id", "user_id", "status", "input", "error",
					"started_at", "ended_at", "duration",
				}).AddRow(
					"exec-1", "agent-1", "user-123", "success", inputJSON, nil,
					now, nil, nil,
				).AddRow(
					"exec-2", "agent-2", "user-123", "running", inputJSON, nil,
					now, nil, nil,
				)

				mock.ExpectQuery(`SELECT .+ FROM agent_executions .* LIMIT 10 OFFSET 0`).
					WithArgs("user-123").
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 2,
			expectTotal: 2,
		},
		{
			name:   "list with agent_id filter",
			userID: "user-123",
			filters: repositories.AgentExecutionFilters{
				AgentID: strPtr("agent-123"),
				Limit:   10,
				Page:    1,
			},
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions`).
					WithArgs("user-123", "agent-123").
					WillReturnRows(countRows)

				rows := sqlmock.NewRows([]string{
					"id", "agent_id", "user_id", "status", "input", "error",
					"started_at", "ended_at", "duration",
				}).AddRow(
					"exec-1", "agent-123", "user-123", "success", inputJSON, nil,
					now, nil, nil,
				)

				mock.ExpectQuery(`SELECT .+ FROM agent_executions .* LIMIT 10 OFFSET 0`).
					WithArgs("user-123", "agent-123").
					WillReturnRows(rows)
			},
			expectErr:   false,
			expectCount: 1,
			expectTotal: 1,
		},
		{
			name:   "count query error",
			userID: "user-error",
			filters: repositories.AgentExecutionFilters{
				Limit: 10,
				Page:  1,
			},
			setupMock: func() {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions`).
					WithArgs("user-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr:   true,
			expectCount: 0,
			expectTotal: 0,
		},
		{
			name:   "list query error",
			userID: "user-123",
			filters: repositories.AgentExecutionFilters{
				Limit: 10,
				Page:  1,
			},
			setupMock: func() {
				countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions`).
					WithArgs("user-123").
					WillReturnRows(countRows)

				mock.ExpectQuery(`SELECT .+ FROM agent_executions .* LIMIT 10 OFFSET 0`).
					WithArgs("user-123").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr:   true,
			expectCount: 0,
			expectTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			executions, total, err := repo.List(ctx, tt.userID, tt.filters)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, executions, tt.expectCount)
				assert.Equal(t, tt.expectTotal, total)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestAgentExecutionRepository_Update(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()
	now := time.Now()
	endedAt := now.Add(time.Minute)

	tests := []struct {
		name      string
		execution *models.AgentExecution
		setupMock func()
		expectErr bool
	}{
		{
			name: "successful update",
			execution: &models.AgentExecution{
				ID:        "exec-123",
				UserID:    "user-123",
				Status:    "success",
				StartedAt: now,
				EndedAt:   &endedAt,
				Version:   1,
			},
			setupMock: func() {
				rows := sqlmock.NewRows([]string{"version"}).AddRow(2)
				mock.ExpectQuery(`UPDATE agent_executions SET`).
					WithArgs(
						"success",
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						"exec-123",
						"user-123",
						int64(1),
					).
					WillReturnRows(rows)
			},
			expectErr: false,
		},
		{
			name: "version mismatch (not found)",
			execution: &models.AgentExecution{
				ID:        "exec-old",
				UserID:    "user-123",
				Status:    "success",
				StartedAt: now,
				Version:   1,
			},
			setupMock: func() {
				mock.ExpectQuery(`UPDATE agent_executions SET`).
					WithArgs(
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						"exec-old",
						"user-123",
						int64(1),
					).
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
		},
		{
			name: "database error",
			execution: &models.AgentExecution{
				ID:        "exec-error",
				UserID:    "user-123",
				Status:    "error",
				StartedAt: now,
				Version:   1,
			},
			setupMock: func() {
				mock.ExpectQuery(`UPDATE agent_executions SET`).
					WithArgs(
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
						"exec-error",
						"user-123",
						int64(1),
					).
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.Update(ctx, tt.execution)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestAgentExecutionRepository_GetByTaskID(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()
	now := time.Now()
	inputJSON, err := json.Marshal(map[string]interface{}{"text": "Test"})
	require.NoError(t, err)

	tests := []struct {
		name       string
		userID     string
		taskID     string
		setupMock  func()
		expectErr  bool
		validateFn func(*testing.T, *models.AgentExecution)
	}{
		{
			name:   "successful retrieval",
			userID: "user-123",
			taskID: "task-123",
			setupMock: func() {
				rows := sqlmock.NewRows([]string{
					"id", "agent_id", "user_id", "status", "input", "error",
					"started_at", "ended_at", "duration", "task_id", "context_id",
					"current_state", "artifacts",
				}).AddRow(
					"exec-123", "agent-123", "user-123", "working", inputJSON, nil,
					now, nil, nil, "task-123", "ctx-123", "working", nil,
				)

				mock.ExpectQuery(`SELECT .+ FROM agent_executions WHERE task_id`).
					WithArgs("task-123", "user-123").
					WillReturnRows(rows)
			},
			expectErr: false,
			validateFn: func(t *testing.T, exec *models.AgentExecution) {
				assert.Equal(t, "exec-123", exec.ID)
				assert.Equal(t, "task-123", *exec.TaskID)
			},
		},
		{
			name:   "not found",
			userID: "user-123",
			taskID: "task-notfound",
			setupMock: func() {
				mock.ExpectQuery(`SELECT .+ FROM agent_executions WHERE task_id`).
					WithArgs("task-notfound", "user-123").
					WillReturnError(sql.ErrNoRows)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := repo.GetByTaskID(ctx, tt.userID, tt.taskID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				if tt.validateFn != nil {
					tt.validateFn(t, result)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestAgentExecutionRepository_UpdateTaskInfo(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()

	tests := []struct {
		name         string
		executionID  string
		taskID       string
		contextID    string
		currentState string
		setupMock    func()
		expectErr    bool
	}{
		{
			name:         "successful update",
			executionID:  "exec-123",
			taskID:       "task-123",
			contextID:    "ctx-123",
			currentState: "working",
			setupMock: func() {
				mock.ExpectExec(`UPDATE agent_executions SET task_id`).
					WithArgs("task-123", "ctx-123", "working", "exec-123").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:         "not found",
			executionID:  "exec-notfound",
			taskID:       "task-123",
			contextID:    "ctx-123",
			currentState: "working",
			setupMock: func() {
				mock.ExpectExec(`UPDATE agent_executions SET task_id`).
					WithArgs("task-123", "ctx-123", "working", "exec-notfound").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectErr: true,
		},
		{
			name:         "database error",
			executionID:  "exec-error",
			taskID:       "task-123",
			contextID:    "ctx-123",
			currentState: "working",
			setupMock: func() {
				mock.ExpectExec(`UPDATE agent_executions SET task_id`).
					WithArgs("task-123", "ctx-123", "working", "exec-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.UpdateTaskInfo(ctx, tt.executionID, tt.taskID, tt.contextID, tt.currentState)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestAgentExecutionRepository_UpdateStatus(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()

	tests := []struct {
		name        string
		executionID string
		status      string
		setupMock   func()
		expectErr   bool
	}{
		{
			name:        "successful update",
			executionID: "exec-123",
			status:      "completed",
			setupMock: func() {
				mock.ExpectExec(`UPDATE agent_executions SET status`).
					WithArgs("completed", sqlmock.AnyArg(), "exec-123").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:        "already finalized - no error",
			executionID: "exec-already-done",
			status:      "completed",
			setupMock: func() {
				mock.ExpectExec(`UPDATE agent_executions SET status`).
					WithArgs("completed", sqlmock.AnyArg(), "exec-already-done").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectErr: false, // No error when already finalized
		},
		{
			name:        "database error",
			executionID: "exec-error",
			status:      "failed",
			setupMock: func() {
				mock.ExpectExec(`UPDATE agent_executions SET status`).
					WithArgs("failed", sqlmock.AnyArg(), "exec-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.UpdateStatus(ctx, tt.executionID, tt.status)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestAgentExecutionRepository_UpdateArtifacts(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()

	tests := []struct {
		name        string
		executionID string
		artifacts   []map[string]interface{}
		setupMock   func()
		expectErr   bool
	}{
		{
			name:        "successful update",
			executionID: "exec-123",
			artifacts:   []map[string]interface{}{{"type": "text", "data": "result"}},
			setupMock: func() {
				mock.ExpectExec(`UPDATE agent_executions SET artifacts`).
					WithArgs(sqlmock.AnyArg(), "exec-123").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:        "not found",
			executionID: "exec-notfound",
			artifacts:   []map[string]interface{}{{"type": "text"}},
			setupMock: func() {
				mock.ExpectExec(`UPDATE agent_executions SET artifacts`).
					WithArgs(sqlmock.AnyArg(), "exec-notfound").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectErr: true,
		},
		{
			name:        "database error",
			executionID: "exec-error",
			artifacts:   []map[string]interface{}{},
			setupMock: func() {
				mock.ExpectExec(`UPDATE agent_executions SET artifacts`).
					WithArgs(sqlmock.AnyArg(), "exec-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.UpdateArtifacts(ctx, tt.executionID, tt.artifacts)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//nolint:funlen // table-driven test with multiple test cases
func TestAgentExecutionRepository_UpdateConversationID(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()

	tests := []struct {
		name           string
		executionID    string
		conversationID string
		setupMock      func()
		expectErr      bool
	}{
		{
			name:           "successful update",
			executionID:    "exec-123",
			conversationID: "conv-123",
			setupMock: func() {
				mock.ExpectExec(`UPDATE agent_executions SET conversation_id`).
					WithArgs("conv-123", "exec-123").
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectErr: false,
		},
		{
			name:           "not found",
			executionID:    "exec-notfound",
			conversationID: "conv-123",
			setupMock: func() {
				mock.ExpectExec(`UPDATE agent_executions SET conversation_id`).
					WithArgs("conv-123", "exec-notfound").
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectErr: true,
		},
		{
			name:           "database error",
			executionID:    "exec-error",
			conversationID: "conv-123",
			setupMock: func() {
				mock.ExpectExec(`UPDATE agent_executions SET conversation_id`).
					WithArgs("conv-123", "exec-error").
					WillReturnError(sql.ErrConnDone)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := repo.UpdateConversationID(ctx, tt.executionID, tt.conversationID)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestAgentExecutionRepository_UpdateTaskInfo_RowsAffectedError tests rows affected error
func TestAgentExecutionRepository_UpdateTaskInfo_RowsAffectedError(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()

	mock.ExpectExec(`UPDATE agent_executions SET task_id`).
		WithArgs("task-123", "ctx-123", "working", "exec-123").
		WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

	err := repo.UpdateTaskInfo(ctx, "exec-123", "task-123", "ctx-123", "working")

	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAgentExecutionRepository_UpdateStatus_RowsAffectedError tests rows affected error
func TestAgentExecutionRepository_UpdateStatus_RowsAffectedError(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()

	mock.ExpectExec(`UPDATE agent_executions SET status`).
		WithArgs("completed", sqlmock.AnyArg(), "exec-123").
		WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

	err := repo.UpdateStatus(ctx, "exec-123", "completed")

	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAgentExecutionRepository_UpdateArtifacts_RowsAffectedError tests rows affected error
func TestAgentExecutionRepository_UpdateArtifacts_RowsAffectedError(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()

	mock.ExpectExec(`UPDATE agent_executions SET artifacts`).
		WithArgs(sqlmock.AnyArg(), "exec-123").
		WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

	err := repo.UpdateArtifacts(ctx, "exec-123", []map[string]interface{}{})

	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAgentExecutionRepository_UpdateConversationID_RowsAffectedError tests rows affected error
func TestAgentExecutionRepository_UpdateConversationID_RowsAffectedError(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()

	mock.ExpectExec(`UPDATE agent_executions SET conversation_id`).
		WithArgs("conv-123", "exec-123").
		WillReturnResult(sqlmock.NewErrorResult(sql.ErrConnDone))

	err := repo.UpdateConversationID(ctx, "exec-123", "conv-123")

	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// strPtr is a helper function to create a pointer to a string
func strPtr(s string) *string {
	return &s
}
