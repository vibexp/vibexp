package postgres

import (
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// agentExecutionEventColumns mirrors the 6 columns every event query scans.
var agentExecutionEventColumns = []string{
	"id", "execution_id", "event_type", "event_data", "sequence_number", "received_at",
}

func setupAgentExecutionEventTest(t *testing.T) (repositories.AgentExecutionEventRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewAgentExecutionEventRepository(&database.DB{DB: mockDB})
	return repo, mock, mockDB
}

func TestAgentExecutionEventRepository_Create(t *testing.T) {
	ctx := contextWithLogger()
	receivedAt := time.Now()

	tests := []struct {
		name       string
		event      *models.AgentExecutionEvent
		setupMock  func(mock sqlmock.Sqlmock)
		wantErr    string
		validateFn func(*testing.T, *models.AgentExecutionEvent)
	}{
		{
			name: "successful create generates a UUID and JSON-marshals event_data",
			event: &models.AgentExecutionEvent{
				ExecutionID:    "exec-123",
				EventType:      "status-update",
				EventData:      map[string]interface{}{"text": "hi"},
				SequenceNumber: 1,
				ReceivedAt:     receivedAt,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`INSERT INTO agent_execution_events`).
					WithArgs(
						sqlmock.AnyArg(), // id (generated)
						"exec-123",
						"status-update",
						[]byte(`{"text":"hi"}`),
						int64(1),
						receivedAt,
					).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			validateFn: func(t *testing.T, event *models.AgentExecutionEvent) {
				require.NotEmpty(t, event.ID, "Create must assign an ID when none is set")
				_, parseErr := uuid.Parse(event.ID)
				assert.NoError(t, parseErr, "generated ID must be a UUID")
			},
		},
		{
			name: "caller-supplied id is preserved",
			event: &models.AgentExecutionEvent{
				ID:             "evt-preset",
				ExecutionID:    "exec-123",
				EventType:      "task",
				EventData:      map[string]interface{}{"k": "v"},
				SequenceNumber: 2,
				ReceivedAt:     receivedAt,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`INSERT INTO agent_execution_events`).
					WithArgs("evt-preset", "exec-123", "task", []byte(`{"k":"v"}`), int64(2), receivedAt).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			validateFn: func(t *testing.T, event *models.AgentExecutionEvent) {
				assert.Equal(t, "evt-preset", event.ID)
			},
		},
		{
			name: "unique violation on (execution_id, sequence_number) maps to duplicate-event error",
			event: &models.AgentExecutionEvent{
				ExecutionID:    "exec-123",
				EventType:      "status-update",
				EventData:      map[string]interface{}{},
				SequenceNumber: 7,
				ReceivedAt:     receivedAt,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`INSERT INTO agent_execution_events`).
					WithArgs(
						sqlmock.AnyArg(), "exec-123", "status-update",
						[]byte(`{}`), int64(7), receivedAt,
					).
					WillReturnError(&pq.Error{
						Code:       "23505",
						Constraint: "unique_execution_sequence",
					})
			},
			wantErr: "event with execution_id exec-123 and sequence_number 7 already exists",
		},
		{
			name: "generic database error is wrapped, not reported as duplicate",
			event: &models.AgentExecutionEvent{
				ExecutionID:    "exec-err",
				EventType:      "task",
				EventData:      map[string]interface{}{},
				SequenceNumber: 1,
				ReceivedAt:     receivedAt,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`INSERT INTO agent_execution_events`).
					WithArgs(
						sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
						sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to create agent execution event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentExecutionEventTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			err := repo.Create(ctx, tt.event)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				if tt.validateFn != nil {
					tt.validateFn(t, tt.event)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAgentExecutionEventRepository_Create_UnmarshalableEventData(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionEventTest(t)
	defer closeMockDB(t, mockDB)

	// A channel cannot be JSON-marshaled: Create must fail before touching the DB.
	event := &models.AgentExecutionEvent{
		ExecutionID:    "exec-123",
		EventType:      "task",
		EventData:      map[string]interface{}{"bad": make(chan int)},
		SequenceNumber: 1,
	}

	err := repo.Create(contextWithLogger(), event)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal event_data")
	assert.NoError(t, mock.ExpectationsWereMet(), "no query must be issued when marshaling fails")
}

func TestAgentExecutionEventRepository_GetByID(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()

	tests := []struct {
		name       string
		eventID    string
		setupMock  func(mock sqlmock.Sqlmock)
		wantErr    string
		wantErrIs  error
		validateFn func(*testing.T, *models.AgentExecutionEvent)
	}{
		{
			name:    "successful retrieval unmarshals event_data",
			eventID: "evt-1",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT .+ FROM agent_execution_events WHERE id`).
					WithArgs("evt-1").
					WillReturnRows(sqlmock.NewRows(agentExecutionEventColumns).AddRow(
						"evt-1", "exec-123", "status-update", []byte(`{"state":"working"}`), 3, now,
					))
			},
			validateFn: func(t *testing.T, event *models.AgentExecutionEvent) {
				assert.Equal(t, "evt-1", event.ID)
				assert.Equal(t, "exec-123", event.ExecutionID)
				assert.Equal(t, "status-update", event.EventType)
				assert.Equal(t, map[string]interface{}{"state": "working"}, event.EventData)
				assert.Equal(t, 3, event.SequenceNumber)
			},
		},
		{
			name:    "empty event_data column leaves EventData nil",
			eventID: "evt-2",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT .+ FROM agent_execution_events WHERE id`).
					WithArgs("evt-2").
					WillReturnRows(sqlmock.NewRows(agentExecutionEventColumns).AddRow(
						"evt-2", "exec-123", "task", []byte(nil), 1, now,
					))
			},
			validateFn: func(t *testing.T, event *models.AgentExecutionEvent) {
				assert.Nil(t, event.EventData)
			},
		},
		{
			name:    "not found maps to ErrAgentExecutionEventNotFound",
			eventID: "evt-missing",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT .+ FROM agent_execution_events WHERE id`).
					WithArgs("evt-missing").
					WillReturnError(sql.ErrNoRows)
			},
			wantErrIs: repositories.ErrAgentExecutionEventNotFound,
		},
		{
			name:    "database error is wrapped",
			eventID: "evt-err",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT .+ FROM agent_execution_events WHERE id`).
					WithArgs("evt-err").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to get agent execution event",
		},
		{
			name:    "invalid event_data JSON surfaces an unmarshal error",
			eventID: "evt-bad",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT .+ FROM agent_execution_events WHERE id`).
					WithArgs("evt-bad").
					WillReturnRows(sqlmock.NewRows(agentExecutionEventColumns).AddRow(
						"evt-bad", "exec-123", "task", []byte(`{not json`), 1, now,
					))
			},
			wantErr: "failed to unmarshal event_data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentExecutionEventTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			event, err := repo.GetByID(ctx, tt.eventID)

			switch {
			case tt.wantErrIs != nil:
				require.ErrorIs(t, err, tt.wantErrIs)
				assert.Nil(t, event)
			case tt.wantErr != "":
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, event)
			default:
				require.NoError(t, err)
				require.NotNil(t, event)
				if tt.validateFn != nil {
					tt.validateFn(t, event)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAgentExecutionEventRepository_ListByExecutionID(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()

	twoRows := func() *sqlmock.Rows {
		return sqlmock.NewRows(agentExecutionEventColumns).
			AddRow("evt-1", "exec-123", "task", []byte(`{"n":1}`), 1, now).
			AddRow("evt-2", "exec-123", "status-update", []byte(`{"n":2}`), 2, now)
	}

	tests := []struct {
		name        string
		limit       int
		offset      int
		setupMock   func(mock sqlmock.Sqlmock)
		wantErr     string
		expectCount int
		expectTotal int
		validateFn  func(*testing.T, []models.AgentExecutionEvent)
	}{
		{
			name:   "explicit limit and offset are bound as given",
			limit:  10,
			offset: 5,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_execution_events`).
					WithArgs("exec-123").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(12))
				mock.ExpectQuery(`FROM agent_execution_events\s+WHERE execution_id = \$1\s+ORDER BY sequence_number ASC`).
					WithArgs("exec-123", int64(10), int64(5)).
					WillReturnRows(twoRows())
			},
			expectCount: 2,
			expectTotal: 12,
			validateFn: func(t *testing.T, events []models.AgentExecutionEvent) {
				assert.Equal(t, 1, events[0].SequenceNumber)
				assert.Equal(t, map[string]interface{}{"n": float64(1)}, events[0].EventData)
				assert.Equal(t, 2, events[1].SequenceNumber)
			},
		},
		{
			name:   "non-positive limit defaults to 50 and negative offset to 0",
			limit:  0,
			offset: -3,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_execution_events`).
					WithArgs("exec-123").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
				mock.ExpectQuery(`FROM agent_execution_events`).
					WithArgs("exec-123", int64(50), int64(0)).
					WillReturnRows(twoRows())
			},
			expectCount: 2,
			expectTotal: 2,
		},
		{
			name:  "empty result returns a non-nil empty slice",
			limit: 10,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_execution_events`).
					WithArgs("exec-123").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
				mock.ExpectQuery(`FROM agent_execution_events`).
					WithArgs("exec-123", int64(10), int64(0)).
					WillReturnRows(sqlmock.NewRows(agentExecutionEventColumns))
			},
			expectCount: 0,
			expectTotal: 0,
		},
		{
			name:  "malformed event_data row is kept with empty EventData (warn-and-continue)",
			limit: 10,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_execution_events`).
					WithArgs("exec-123").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM agent_execution_events`).
					WithArgs("exec-123", int64(10), int64(0)).
					WillReturnRows(sqlmock.NewRows(agentExecutionEventColumns).AddRow(
						"evt-bad", "exec-123", "task", []byte(`{corrupt`), 1, now,
					))
			},
			expectCount: 1,
			expectTotal: 1,
			validateFn: func(t *testing.T, events []models.AgentExecutionEvent) {
				assert.NotNil(t, events[0].EventData)
				assert.Empty(t, events[0].EventData, "EventData must be reset to an empty map")
			},
		},
		{
			name:  "count query error propagates wrapped",
			limit: 10,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_execution_events`).
					WithArgs("exec-123").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count agent execution events",
		},
		{
			name:  "list query error propagates wrapped",
			limit: 10,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_execution_events`).
					WithArgs("exec-123").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM agent_execution_events`).
					WithArgs("exec-123", int64(10), int64(0)).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to list agent execution events",
		},
		{
			name:  "scan error propagates wrapped",
			limit: 10,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_execution_events`).
					WithArgs("exec-123").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				// One column instead of six forces a scan error.
				mock.ExpectQuery(`FROM agent_execution_events`).
					WithArgs("exec-123", int64(10), int64(0)).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("evt-1"))
			},
			wantErr: "failed to scan agent execution event",
		},
		{
			name:  "row iteration error propagates wrapped",
			limit: 10,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_execution_events`).
					WithArgs("exec-123").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM agent_execution_events`).
					WithArgs("exec-123", int64(10), int64(0)).
					WillReturnRows(sqlmock.NewRows(agentExecutionEventColumns).AddRow(
						"evt-1", "exec-123", "task", []byte(`{}`), 1, now,
					).RowError(0, sql.ErrConnDone))
			},
			wantErr: "failed to iterate agent execution events",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentExecutionEventTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			events, total, err := repo.ListByExecutionID(ctx, "exec-123", tt.limit, tt.offset)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, events)
				assert.Zero(t, total)
			} else {
				require.NoError(t, err)
				require.NotNil(t, events, "must return a non-nil slice, never nil")
				assert.Len(t, events, tt.expectCount)
				assert.Equal(t, tt.expectTotal, total)
				if tt.validateFn != nil {
					tt.validateFn(t, events)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAgentExecutionEventRepository_ListAfterSequence(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()

	tests := []struct {
		name          string
		afterSequence int
		setupMock     func(mock sqlmock.Sqlmock)
		wantErr       string
		expectSeqs    []int
		validateFn    func(*testing.T, []models.AgentExecutionEvent)
	}{
		{
			name:          "cursor binds sequence_number > afterSequence",
			afterSequence: 42,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM agent_execution_events\s+WHERE execution_id = \$1 AND sequence_number > \$2`).
					WithArgs("exec-123", int64(42)).
					WillReturnRows(sqlmock.NewRows(agentExecutionEventColumns).
						AddRow("evt-43", "exec-123", "status-update", []byte(`{"n":43}`), 43, now).
						AddRow("evt-44", "exec-123", "artifact-update", []byte(`{"n":44}`), 44, now))
			},
			expectSeqs: []int{43, 44},
		},
		{
			name:          "no newer events returns a non-nil empty slice",
			afterSequence: 99,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM agent_execution_events`).
					WithArgs("exec-123", int64(99)).
					WillReturnRows(sqlmock.NewRows(agentExecutionEventColumns))
			},
			expectSeqs: []int{},
		},
		{
			name:          "malformed event_data row is kept with empty EventData",
			afterSequence: 0,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM agent_execution_events`).
					WithArgs("exec-123", int64(0)).
					WillReturnRows(sqlmock.NewRows(agentExecutionEventColumns).AddRow(
						"evt-1", "exec-123", "task", []byte(`{corrupt`), 1, now,
					))
			},
			expectSeqs: []int{1},
			validateFn: func(t *testing.T, events []models.AgentExecutionEvent) {
				assert.NotNil(t, events[0].EventData)
				assert.Empty(t, events[0].EventData)
			},
		},
		{
			name:          "query error propagates wrapped",
			afterSequence: 1,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM agent_execution_events`).
					WithArgs("exec-123", int64(1)).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to list agent execution events after sequence",
		},
		{
			name:          "scan error propagates wrapped",
			afterSequence: 1,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM agent_execution_events`).
					WithArgs("exec-123", int64(1)).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("evt-1"))
			},
			wantErr: "failed to scan agent execution event",
		},
		{
			name:          "row iteration error propagates wrapped",
			afterSequence: 1,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM agent_execution_events`).
					WithArgs("exec-123", int64(1)).
					WillReturnRows(sqlmock.NewRows(agentExecutionEventColumns).AddRow(
						"evt-2", "exec-123", "task", []byte(`{}`), 2, now,
					).RowError(0, sql.ErrConnDone))
			},
			wantErr: "failed to iterate agent execution events",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentExecutionEventTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			events, err := repo.ListAfterSequence(ctx, "exec-123", tt.afterSequence)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, events)
			} else {
				require.NoError(t, err)
				require.NotNil(t, events, "must return a non-nil slice, never nil")
				seqs := make([]int, 0, len(events))
				for _, e := range events {
					seqs = append(seqs, e.SequenceNumber)
				}
				assert.Equal(t, tt.expectSeqs, seqs)
				if tt.validateFn != nil {
					tt.validateFn(t, events)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAgentExecutionEventRepository_GetLatestByExecutionID(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()

	tests := []struct {
		name       string
		setupMock  func(mock sqlmock.Sqlmock)
		wantErr    string
		wantErrIs  error
		validateFn func(*testing.T, *models.AgentExecutionEvent)
	}{
		{
			name: "returns the highest-sequence event",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM agent_execution_events\s+WHERE execution_id = \$1\s+ORDER BY sequence_number DESC\s+LIMIT 1`).
					WithArgs("exec-123").
					WillReturnRows(sqlmock.NewRows(agentExecutionEventColumns).AddRow(
						"evt-9", "exec-123", "artifact-update", []byte(`{"final":true}`), 9, now,
					))
			},
			validateFn: func(t *testing.T, event *models.AgentExecutionEvent) {
				assert.Equal(t, "evt-9", event.ID)
				assert.Equal(t, 9, event.SequenceNumber)
				assert.Equal(t, map[string]interface{}{"final": true}, event.EventData)
			},
		},
		{
			name: "no events maps to ErrAgentExecutionEventNotFound",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM agent_execution_events`).
					WithArgs("exec-123").
					WillReturnError(sql.ErrNoRows)
			},
			wantErrIs: repositories.ErrAgentExecutionEventNotFound,
		},
		{
			name: "database error is wrapped and is not the not-found sentinel",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM agent_execution_events`).
					WithArgs("exec-123").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to get latest agent execution event",
		},
		{
			name: "invalid event_data JSON surfaces an unmarshal error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM agent_execution_events`).
					WithArgs("exec-123").
					WillReturnRows(sqlmock.NewRows(agentExecutionEventColumns).AddRow(
						"evt-9", "exec-123", "task", []byte(`{bad`), 9, now,
					))
			},
			wantErr: "failed to unmarshal event_data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentExecutionEventTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			event, err := repo.GetLatestByExecutionID(ctx, "exec-123")

			switch {
			case tt.wantErrIs != nil:
				require.ErrorIs(t, err, tt.wantErrIs)
				assert.Nil(t, event)
			case tt.wantErr != "":
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.NotErrorIs(t, err, repositories.ErrAgentExecutionEventNotFound)
				assert.Nil(t, event)
			default:
				require.NoError(t, err)
				require.NotNil(t, event)
				if tt.validateFn != nil {
					tt.validateFn(t, event)
				}
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAgentExecutionEventRepository_CountByExecutionID(t *testing.T) {
	ctx := contextWithLogger()

	t.Run("returns the count for the execution", func(t *testing.T) {
		repo, mock, mockDB := setupAgentExecutionEventTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_execution_events WHERE execution_id`).
			WithArgs("exec-123").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7))

		count, err := repo.CountByExecutionID(ctx, "exec-123")

		require.NoError(t, err)
		assert.Equal(t, 7, count)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("database error is wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupAgentExecutionEventTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_execution_events WHERE execution_id`).
			WithArgs("exec-123").
			WillReturnError(sql.ErrConnDone)

		count, err := repo.CountByExecutionID(ctx, "exec-123")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to count agent execution events")
		assert.Zero(t, count)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
