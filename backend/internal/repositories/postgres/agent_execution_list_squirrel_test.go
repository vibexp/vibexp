package postgres

import (
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/repositories"
)

// agentExecutionListColumns mirrors the 9 columns scanned by the List page query.
var agentExecutionListColumns = []string{
	"id", "agent_id", "user_id", "status", "input",
	"error", "started_at", "ended_at", "duration",
}

// agentExecutionOneRow builds a single fully-populated result row for the list
// projection.
func agentExecutionOneRow(now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(agentExecutionListColumns).AddRow(
		"exec-1", "agent-1", "user-123", "success",
		[]byte(`{"text":"hi"}`), nil, now, nil, nil,
	)
}

//nolint:funlen // table-driven test exercising every optional filter and pagination shape
func TestAgentExecutionRepository_List_SquirrelMigration(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()
	agentID := "agent-123"
	status := "success"
	dateFrom := "2026-01-01T00:00:00Z"
	dateTo := "2026-12-31T23:59:59Z"

	tests := []struct {
		name        string
		filters     repositories.AgentExecutionFilters
		countArgs   []driver.Value
		listMatcher string
		listArgs    []driver.Value
		expectTotal int
		expectCount int
	}{
		{
			name:        "baseline only user_id, default pagination",
			filters:     repositories.AgentExecutionFilters{Page: 1, Limit: 10},
			countArgs:   []driver.Value{"user-123"},
			listMatcher: `FROM agent_executions WHERE \(user_id = \$1\) ORDER BY started_at DESC LIMIT 10 OFFSET 0`,
			listArgs:    []driver.Value{"user-123"},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:        "AgentID filter binds agent_id equality",
			filters:     repositories.AgentExecutionFilters{AgentID: &agentID, Page: 1, Limit: 10},
			countArgs:   []driver.Value{"user-123", "agent-123"},
			listMatcher: `FROM agent_executions WHERE \(user_id = \$1 AND agent_id = \$2\)`,
			listArgs:    []driver.Value{"user-123", "agent-123"},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:        "Status filter binds status equality",
			filters:     repositories.AgentExecutionFilters{Status: &status, Page: 1, Limit: 10},
			countArgs:   []driver.Value{"user-123", "success"},
			listMatcher: `FROM agent_executions WHERE \(user_id = \$1 AND status = \$2\)`,
			listArgs:    []driver.Value{"user-123", "success"},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:        "DateFrom filter binds started_at >=",
			filters:     repositories.AgentExecutionFilters{DateFrom: &dateFrom, Page: 1, Limit: 10},
			countArgs:   []driver.Value{"user-123", dateFrom},
			listMatcher: `FROM agent_executions WHERE \(user_id = \$1 AND started_at >= \$2\)`,
			listArgs:    []driver.Value{"user-123", dateFrom},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:        "DateTo filter binds started_at <=",
			filters:     repositories.AgentExecutionFilters{DateTo: &dateTo, Page: 1, Limit: 10},
			countArgs:   []driver.Value{"user-123", dateTo},
			listMatcher: `FROM agent_executions WHERE \(user_id = \$1 AND started_at <= \$2\)`,
			listArgs:    []driver.Value{"user-123", dateTo},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name: "all filters combined bind in order user, agent, status, from, to",
			filters: repositories.AgentExecutionFilters{
				AgentID: &agentID, Status: &status, DateFrom: &dateFrom, DateTo: &dateTo,
				Page: 1, Limit: 10,
			},
			countArgs: []driver.Value{"user-123", "agent-123", "success", dateFrom, dateTo},
			listMatcher: `FROM agent_executions WHERE \(user_id = \$1 AND agent_id = \$2 AND ` +
				`status = \$3 AND started_at >= \$4 AND started_at <= \$5\)`,
			listArgs:    []driver.Value{"user-123", "agent-123", "success", dateFrom, dateTo},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:        "defaulting: zero page and limit default to LIMIT 10 OFFSET 0",
			filters:     repositories.AgentExecutionFilters{Page: 0, Limit: 0},
			countArgs:   []driver.Value{"user-123"},
			listMatcher: `FROM agent_executions WHERE \(user_id = \$1\) ORDER BY started_at DESC LIMIT 10 OFFSET 0`,
			listArgs:    []driver.Value{"user-123"},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:        "defaulting: negative page and limit default to LIMIT 10 OFFSET 0",
			filters:     repositories.AgentExecutionFilters{Page: -3, Limit: -5},
			countArgs:   []driver.Value{"user-123"},
			listMatcher: `FROM agent_executions WHERE \(user_id = \$1\) ORDER BY started_at DESC LIMIT 10 OFFSET 0`,
			listArgs:    []driver.Value{"user-123"},
			expectTotal: 1,
			expectCount: 1,
		},
		{
			name:        "pagination: page 3 limit 5 computes LIMIT 5 OFFSET 10",
			filters:     repositories.AgentExecutionFilters{Page: 3, Limit: 5},
			countArgs:   []driver.Value{"user-123"},
			listMatcher: `FROM agent_executions WHERE \(user_id = \$1\) ORDER BY started_at DESC LIMIT 5 OFFSET 10`,
			listArgs:    []driver.Value{"user-123"},
			expectTotal: 1,
			expectCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentExecutionTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions`).
				WithArgs(tt.countArgs...).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(tt.expectTotal))
			mock.ExpectQuery(tt.listMatcher).
				WithArgs(tt.listArgs...).
				WillReturnRows(agentExecutionOneRow(now))

			executions, total, err := repo.List(ctx, "user-123", tt.filters)

			assert.NoError(t, err)
			assert.NotNil(t, executions, "List must return a non-nil empty slice, never nil")
			assert.Len(t, executions, tt.expectCount)
			assert.Equal(t, tt.expectTotal, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestAgentExecutionRepository_List_ExplicitProjection pins the full 9-column
// projection for the default path. A `.+` matcher would not catch column drift,
// so the projection is asserted verbatim.
func TestAgentExecutionRepository_List_ExplicitProjection(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()
	now := time.Now()
	filters := repositories.AgentExecutionFilters{Page: 1, Limit: 10}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions`).
		WithArgs("user-123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(
		`SELECT id, agent_id, user_id, status, input, ` +
			`error, started_at, ended_at, duration ` +
			`FROM agent_executions WHERE`,
	).
		WithArgs("user-123").
		WillReturnRows(agentExecutionOneRow(now))

	executions, total, err := repo.List(ctx, "user-123", filters)
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, executions, 1)
	assert.Equal(t, "exec-1", executions[0].ID)
	assert.Equal(t, "hi", executions[0].Input["text"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestAgentExecutionRepository_List_NullHandling verifies nullable columns map to
// nil pointers when NULL and to set pointers when present.
//
//nolint:funlen // two subtests covering NULL and populated nullable columns
func TestAgentExecutionRepository_List_NullHandling(t *testing.T) {
	ctx := contextWithLogger()
	now := time.Now()
	endedAt := now.Add(time.Minute)
	filters := repositories.AgentExecutionFilters{Page: 1, Limit: 10}

	t.Run("all nullable columns NULL yield nil pointers", func(t *testing.T) {
		repo, mock, mockDB := setupAgentExecutionTest(t)
		defer func() {
			if closeErr := mockDB.Close(); closeErr != nil {
				t.Logf("Failed to close mock DB: %v", closeErr)
			}
		}()

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions`).
			WithArgs("user-123").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`FROM agent_executions`).
			WithArgs("user-123").
			WillReturnRows(sqlmock.NewRows(agentExecutionListColumns).AddRow(
				"exec-1", "agent-1", "user-123", "running",
				[]byte(`{"text":"hi"}`), nil, now, nil, nil,
			))

		executions, _, err := repo.List(ctx, "user-123", filters)
		require.NoError(t, err)
		require.Len(t, executions, 1)
		assert.Nil(t, executions[0].EndedAt)
		assert.Nil(t, executions[0].Error)
		assert.Nil(t, executions[0].Duration)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("populated nullable columns yield set pointers", func(t *testing.T) {
		repo, mock, mockDB := setupAgentExecutionTest(t)
		defer func() {
			if closeErr := mockDB.Close(); closeErr != nil {
				t.Logf("Failed to close mock DB: %v", closeErr)
			}
		}()

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions`).
			WithArgs("user-123").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`FROM agent_executions`).
			WithArgs("user-123").
			WillReturnRows(sqlmock.NewRows(agentExecutionListColumns).AddRow(
				"exec-1", "agent-1", "user-123", "error",
				[]byte(`{"text":"hi"}`), "boom", now, endedAt, int64(1500),
			))

		executions, _, err := repo.List(ctx, "user-123", filters)
		require.NoError(t, err)
		require.Len(t, executions, 1)
		require.NotNil(t, executions[0].EndedAt)
		assert.WithinDuration(t, endedAt, *executions[0].EndedAt, time.Second)
		require.NotNil(t, executions[0].Error)
		assert.Equal(t, "boom", *executions[0].Error)
		require.NotNil(t, executions[0].Duration)
		assert.Equal(t, 1500, *executions[0].Duration)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestAgentExecutionRepository_List_InvalidInputJSON verifies the WARN-and-continue
// contract: a row with malformed input JSON is kept with an empty Input map and
// does NOT surface an error.
func TestAgentExecutionRepository_List_InvalidInputJSON(t *testing.T) {
	repo, mock, mockDB := setupAgentExecutionTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	ctx := contextWithLogger()
	now := time.Now()
	filters := repositories.AgentExecutionFilters{Page: 1, Limit: 10}

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions`).
		WithArgs("user-123").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM agent_executions`).
		WithArgs("user-123").
		WillReturnRows(sqlmock.NewRows(agentExecutionListColumns).AddRow(
			"exec-1", "agent-1", "user-123", "success",
			[]byte(`{not valid json`), nil, now, nil, nil,
		))

	executions, total, err := repo.List(ctx, "user-123", filters)

	require.NoError(t, err, "malformed input JSON must not surface an error")
	assert.Equal(t, 1, total)
	require.Len(t, executions, 1, "the row must still be included")
	assert.NotNil(t, executions[0].Input)
	assert.Empty(t, executions[0].Input, "Input must be reset to an empty map")
	assert.NoError(t, mock.ExpectationsWereMet())
}

//nolint:funlen // table-driven error-path test with multiple scenarios
func TestAgentExecutionRepository_List_ErrorPaths(t *testing.T) {
	now := time.Now()
	filters := repositories.AgentExecutionFilters{Page: 1, Limit: 10}

	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "count query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions`).
					WithArgs("user-123").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count agent executions",
		},
		{
			name: "list query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions`).
					WithArgs("user-123").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`FROM agent_executions`).
					WithArgs("user-123").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to list agent executions",
		},
		{
			name: "scan error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions`).
					WithArgs("user-123").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				// One column instead of the nine the scan expects forces a scan error.
				mock.ExpectQuery(`FROM agent_executions`).
					WithArgs("user-123").
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("exec-1"))
			},
			wantErr: "failed to scan agent execution",
		},
		{
			name: "row iteration error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agent_executions`).
					WithArgs("user-123").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				rows := sqlmock.NewRows(agentExecutionListColumns).AddRow(
					"exec-1", "agent-1", "user-123", "success",
					[]byte(`{"text":"hi"}`), nil, now, nil, nil,
				).RowError(0, sql.ErrConnDone)
				mock.ExpectQuery(`FROM agent_executions`).
					WithArgs("user-123").
					WillReturnRows(rows)
			},
			wantErr: "failed to iterate agent executions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupAgentExecutionTest(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("Failed to close mock DB: %v", closeErr)
				}
			}()

			tt.setupMock(mock)

			executions, total, err := repo.List(contextWithLogger(), "user-123", filters)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, executions)
			assert.Zero(t, total)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
