package postgres

import (
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// conversationExecutionColumns mirrors the 14-column projection scanned by
// GetByConversationID and GetFirstExecutionInConversation.
var conversationExecutionColumns = []string{
	"id", "agent_id", "user_id", "status", "input", "error",
	"started_at", "ended_at", "duration", "task_id", "context_id",
	"current_state", "artifacts", "conversation_id",
}

// conversationSummaryColumns mirrors the 8 columns scanned by ListConversations.
var conversationSummaryColumns = []string{
	"conversation_id", "agent_id", "message_count", "first_message",
	"last_message", "started_at", "last_activity_at", "last_status",
}

func conversationExecutionRow(rows *sqlmock.Rows, id, text string, startedAt time.Time) {
	rows.AddRow(
		id, "agent-1", "user-123", "success", []byte(`{"text":"`+text+`"}`), nil,
		startedAt, nil, nil, nil, nil, nil, nil, "conv-1",
	)
}

// TestAgentExecutionRepository_GetByAgentID verifies GetByAgentID forces the
// agent_id filter into the shared List path (count and page bind it).
func TestAgentExecutionRepository_GetByAgentID(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer closeMockDB(t, mockDB)

	ctx := contextWithLogger()
	now := time.Now()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions`).
		WithArgs("user-123", "agent-77").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM agent_executions WHERE \(user_id = \$1 AND agent_id = \$2\)`).
		WithArgs("user-123", "agent-77").
		WillReturnRows(agentExecutionOneRow(now))

	executions, total, err := repo.GetByAgentID(
		ctx, "user-123", "agent-77", repositories.AgentExecutionFilters{Page: 1, Limit: 10},
	)

	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, executions, 1)
	assert.Equal(t, "exec-1", executions[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentExecutionRepository_GetByConversationID(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()
	before := now.Add(-time.Hour)

	tests := []struct {
		name        string
		limit       int
		before      *time.Time
		setupMock   func(mock sqlmock.Sqlmock)
		wantErr     string
		expectIDs   []string
		expectMore  bool
		expectTotal int
	}{
		{
			name:  "rows come back DESC and are reversed to chronological order",
			limit: 2,
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows(conversationExecutionColumns)
				conversationExecutionRow(rows, "exec-2", "second", now)
				conversationExecutionRow(rows, "exec-1", "first", now.Add(-time.Minute))
				mock.ExpectQuery(`FROM agent_executions\s+WHERE user_id = \$1 AND conversation_id = \$2 ORDER BY started_at DESC LIMIT \$3`).
					WithArgs("user-123", "conv-1", int64(2)).
					WillReturnRows(rows)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions WHERE user_id = \$1 AND conversation_id = \$2`).
					WithArgs("user-123", "conv-1").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
			},
			expectIDs:   []string{"exec-1", "exec-2"},
			expectMore:  true, // len(executions) == limit
			expectTotal: 5,
		},
		{
			name:   "before cursor binds started_at < \\$3",
			limit:  10,
			before: &before,
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows(conversationExecutionColumns)
				conversationExecutionRow(rows, "exec-old", "old", before.Add(-time.Minute))
				mock.ExpectQuery(`AND started_at < \$3 ORDER BY started_at DESC LIMIT \$4`).
					WithArgs("user-123", "conv-1", before, int64(10)).
					WillReturnRows(rows)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions WHERE user_id = \$1 AND conversation_id = \$2`).
					WithArgs("user-123", "conv-1").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
			},
			expectIDs:   []string{"exec-old"},
			expectMore:  false, // fewer rows than limit
			expectTotal: 3,
		},
		{
			name:  "count failure falls back to page length without erroring",
			limit: 10,
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows(conversationExecutionColumns)
				conversationExecutionRow(rows, "exec-1", "only", now)
				mock.ExpectQuery(`FROM agent_executions\s+WHERE user_id = \$1 AND conversation_id = \$2`).
					WithArgs("user-123", "conv-1", int64(10)).
					WillReturnRows(rows)
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions WHERE user_id = \$1 AND conversation_id = \$2`).
					WithArgs("user-123", "conv-1").
					WillReturnError(sql.ErrConnDone)
			},
			expectIDs:   []string{"exec-1"},
			expectMore:  false,
			expectTotal: 1,
		},
		{
			name:  "query error propagates wrapped",
			limit: 10,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM agent_executions\s+WHERE user_id = \$1 AND conversation_id = \$2`).
					WithArgs("user-123", "conv-1", int64(10)).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to query executions",
		},
		{
			name:  "scan error propagates wrapped",
			limit: 10,
			setupMock: func(mock sqlmock.Sqlmock) {
				// One column instead of fourteen forces a scan error.
				mock.ExpectQuery(`FROM agent_executions\s+WHERE user_id = \$1 AND conversation_id = \$2`).
					WithArgs("user-123", "conv-1", int64(10)).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("exec-1"))
			},
			wantErr: "failed to scan execution",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentExecutionTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			executions, hasMore, total, err := repo.GetByConversationID(
				ctx, "user-123", "conv-1", tt.limit, tt.before,
			)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, executions)
			} else {
				require.NoError(t, err)
				ids := make([]string, 0, len(executions))
				for _, e := range executions {
					ids = append(ids, e.ID)
				}
				assert.Equal(t, tt.expectIDs, ids)
				assert.Equal(t, tt.expectMore, hasMore)
				assert.Equal(t, tt.expectTotal, total)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAgentExecutionRepository_GetFirstExecutionInConversation(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()

	tests := []struct {
		name       string
		setupMock  func(mock sqlmock.Sqlmock)
		wantErr    string
		wantErrIs  error
		validateFn func(*testing.T, *models.AgentExecution)
	}{
		{
			name: "returns the earliest execution",
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows(conversationExecutionColumns).AddRow(
					"exec-first", "agent-1", "user-123", "success",
					[]byte(`{"text":"opening"}`), nil, now, nil, nil,
					"task-1", "ctx-1", "completed", []byte(`[{"kind":"text"}]`), "conv-1",
				)
				mock.ExpectQuery(`FROM agent_executions\s+WHERE user_id = \$1 AND conversation_id = \$2\s+ORDER BY started_at ASC\s+LIMIT 1`).
					WithArgs("user-123", "conv-1").
					WillReturnRows(rows)
			},
			validateFn: func(t *testing.T, exec *models.AgentExecution) {
				assert.Equal(t, "exec-first", exec.ID)
				assert.Equal(t, "opening", exec.Input["text"])
				require.NotNil(t, exec.ContextID)
				assert.Equal(t, "ctx-1", *exec.ContextID)
				require.NotNil(t, exec.ConversationID)
				assert.Equal(t, "conv-1", *exec.ConversationID)
				require.Len(t, exec.Artifacts, 1)
			},
		},
		{
			name: "no rows maps to ErrConversationNotFound",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`ORDER BY started_at ASC`).
					WithArgs("user-123", "conv-1").
					WillReturnError(sql.ErrNoRows)
			},
			wantErrIs: repositories.ErrConversationNotFound,
		},
		{
			name: "database error is wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`ORDER BY started_at ASC`).
					WithArgs("user-123", "conv-1").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to get first execution",
		},
		{
			name: "invalid input JSON surfaces an unmarshal error",
			setupMock: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows(conversationExecutionColumns).AddRow(
					"exec-first", "agent-1", "user-123", "success",
					[]byte(`{bad`), nil, now, nil, nil,
					nil, nil, nil, nil, "conv-1",
				)
				mock.ExpectQuery(`ORDER BY started_at ASC`).
					WithArgs("user-123", "conv-1").
					WillReturnRows(rows)
			},
			wantErr: "failed to unmarshal input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentExecutionTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			exec, err := repo.GetFirstExecutionInConversation(ctx, "user-123", "conv-1")

			switch {
			case tt.wantErrIs != nil:
				require.ErrorIs(t, err, tt.wantErrIs)
				assert.Nil(t, exec)
			case tt.wantErr != "":
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, exec)
			default:
				require.NoError(t, err)
				require.NotNil(t, exec)
				if tt.validateFn != nil {
					tt.validateFn(t, exec)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAgentExecutionRepository_ListConversations(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()

	summaryRows := func() *sqlmock.Rows {
		return sqlmock.NewRows(conversationSummaryColumns).
			AddRow("conv-2", "agent-2", 1, "solo", "solo", now, now, "success").
			AddRow("conv-1", "agent-1", 3, "first", "third", now.Add(-time.Hour), now.Add(-time.Minute), "running")
	}

	tests := []struct {
		name        string
		agentID     string
		page        int
		limit       int
		setupMock   func(mock sqlmock.Sqlmock)
		wantErr     string
		expectIDs   []string
		expectTotal int
		validateFn  func(*testing.T, []models.ConversationSummary)
	}{
		{
			name:    "empty agentID lists across all agents and binds (user, limit, offset)",
			agentID: "",
			page:    1,
			limit:   10,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`WITH conversation_data AS`).
					WithArgs("user-123", int64(10), int64(0)).
					WillReturnRows(summaryRows())
				mock.ExpectQuery(`SELECT COUNT\(DISTINCT conversation_id\)\s+FROM agent_executions\s+WHERE user_id = \$1 AND conversation_id IS NOT NULL`).
					WithArgs("user-123").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
			},
			expectIDs:   []string{"conv-2", "conv-1"},
			expectTotal: 2,
			validateFn: func(t *testing.T, convs []models.ConversationSummary) {
				assert.Equal(t, 3, convs[1].MessageCount)
				assert.Equal(t, "first", convs[1].FirstMessage)
				assert.Equal(t, "third", convs[1].LastMessage)
				assert.Equal(t, "running", convs[1].LastStatus)
			},
		},
		{
			name:    "agentID filter binds (user, agent, limit, offset) with computed page offset",
			agentID: "agent-1",
			page:    3,
			limit:   5,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`WITH conversation_data AS`).
					WithArgs("user-123", "agent-1", int64(5), int64(10)).
					WillReturnRows(sqlmock.NewRows(conversationSummaryColumns).
						AddRow("conv-1", "agent-1", 3, "first", "third", now, now, "running"))
				mock.ExpectQuery(`SELECT COUNT\(DISTINCT conversation_id\)\s+FROM agent_executions\s+WHERE user_id = \$1 AND agent_id = \$2 AND conversation_id IS NOT NULL`).
					WithArgs("user-123", "agent-1").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(11))
			},
			expectIDs:   []string{"conv-1"},
			expectTotal: 11,
		},
		{
			name:    "count failure falls back to page length without erroring",
			agentID: "",
			page:    1,
			limit:   10,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`WITH conversation_data AS`).
					WithArgs("user-123", int64(10), int64(0)).
					WillReturnRows(summaryRows())
				mock.ExpectQuery(`SELECT COUNT\(DISTINCT conversation_id\)`).
					WithArgs("user-123").
					WillReturnError(sql.ErrConnDone)
			},
			expectIDs:   []string{"conv-2", "conv-1"},
			expectTotal: 2,
		},
		{
			name:    "empty result returns a non-nil empty slice",
			agentID: "",
			page:    1,
			limit:   10,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`WITH conversation_data AS`).
					WithArgs("user-123", int64(10), int64(0)).
					WillReturnRows(sqlmock.NewRows(conversationSummaryColumns))
				mock.ExpectQuery(`SELECT COUNT\(DISTINCT conversation_id\)`).
					WithArgs("user-123").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			},
			expectIDs:   []string{},
			expectTotal: 0,
		},
		{
			name:    "query error propagates wrapped",
			agentID: "",
			page:    1,
			limit:   10,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`WITH conversation_data AS`).
					WithArgs("user-123", int64(10), int64(0)).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to query conversations",
		},
		{
			name:    "scan error propagates wrapped",
			agentID: "",
			page:    1,
			limit:   10,
			setupMock: func(mock sqlmock.Sqlmock) {
				// One column instead of eight forces a scan error.
				mock.ExpectQuery(`WITH conversation_data AS`).
					WithArgs("user-123", int64(10), int64(0)).
					WillReturnRows(sqlmock.NewRows([]string{"conversation_id"}).AddRow("conv-1"))
			},
			wantErr: "failed to scan conversation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentExecutionTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			conversations, total, err := repo.ListConversations(ctx, "user-123", tt.agentID, tt.page, tt.limit)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, conversations)
				assert.Zero(t, total)
			} else {
				require.NoError(t, err)
				require.NotNil(t, conversations, "must return a non-nil slice, never nil")
				ids := make([]string, 0, len(conversations))
				for _, c := range conversations {
					ids = append(ids, c.ConversationID)
				}
				assert.Equal(t, tt.expectIDs, ids)
				assert.Equal(t, tt.expectTotal, total)
				if tt.validateFn != nil {
					tt.validateFn(t, conversations)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
