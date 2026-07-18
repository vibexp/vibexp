package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// claudeCodeHookListColumns mirrors the 15-column projection scanned by GetByID
// and the List page query.
var claudeCodeHookListColumns = []string{
	"id", "user_id", "team_id", "session_id", "transcript_path", "cwd", "hook_event_name", "tool_name",
	"tool_input", "tool_response", "prompt", "message", "payload", "created_at", "updated_at",
}

// claudeCodeHookRows builds scannable rows for the List/GetByID projection, one
// per supplied id.
func claudeCodeHookRows(now time.Time, ids ...int) *sqlmock.Rows {
	rows := sqlmock.NewRows(claudeCodeHookListColumns)
	for _, id := range ids {
		rows.AddRow(
			id, "user-123", "team-1", "session-456", nil, "/repo", "PostToolUse", "Bash",
			[]byte(`{"command":"ls"}`), nil, nil, nil, []byte(`{"source":"test"}`), now, now,
		)
	}
	return rows
}

// countResult builds a single-row COUNT(...) result.
func countResult(n int) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"count"}).AddRow(n)
}

func TestClaudeCodeHooksRepository_Create(t *testing.T) {
	repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
	defer closeMockDB(t, mockDB)

	now := time.Now()
	payload := &models.ClaudeCodeHookPayload{
		UserID:        strPtr("user-123"),
		TeamID:        "team-1",
		SessionID:     "session-456",
		CWD:           strPtr("/repo"),
		HookEventName: "PostToolUse",
		ToolName:      strPtr("Bash"),
		ToolInput:     &models.JSONBData{"command": "ls"},
		Payload:       models.JSONBData{"source": "test"},
	}

	mock.ExpectQuery(
		`INSERT INTO claude_code_hooks_payload \(user_id, team_id, session_id, transcript_path, cwd, `+
			`hook_event_name, tool_name, tool_input, tool_response, prompt, message, payload\) `+
			`VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, \$11, \$12\) `+
			`RETURNING id, created_at, updated_at`,
	).
		WithArgs(
			"user-123", "team-1", "session-456", nil, "/repo", "PostToolUse", "Bash",
			models.JSONBData{"command": "ls"}, nil, nil, nil, models.JSONBData{"source": "test"},
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(7, now, now))

	require.NoError(t, repo.Create(context.Background(), payload))
	assert.Equal(t, 7, payload.ID, "Create must write the DB-assigned id back onto the struct")
	assert.Equal(t, now, payload.CreatedAt)
	assert.Equal(t, now, payload.UpdatedAt)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestClaudeCodeHooksRepository_Create_Error(t *testing.T) {
	repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
	defer closeMockDB(t, mockDB)

	mock.ExpectQuery(`INSERT INTO claude_code_hooks_payload`).WillReturnError(sql.ErrConnDone)

	err := repo.Create(context.Background(), &models.ClaudeCodeHookPayload{
		TeamID: "team-1", SessionID: "session-456", HookEventName: "PostToolUse",
		Payload: models.JSONBData{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create Claude Code hook payload")
	assert.ErrorIs(t, err, sql.ErrConnDone)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestClaudeCodeHooksRepository_GetByID(t *testing.T) {
	repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
	defer closeMockDB(t, mockDB)

	now := time.Now()
	mock.ExpectQuery(
		`SELECT id, user_id, team_id, session_id, transcript_path, cwd, hook_event_name, tool_name, `+
			`tool_input, tool_response, prompt, message, payload, created_at, updated_at `+
			`FROM claude_code_hooks_payload WHERE id = \$1 AND user_id = \$2`,
	).
		WithArgs(42, "user-123").
		WillReturnRows(claudeCodeHookRows(now, 42))

	got, err := repo.GetByID(context.Background(), "user-123", 42)
	require.NoError(t, err)
	assert.Equal(t, 42, got.ID)
	require.NotNil(t, got.UserID)
	assert.Equal(t, "user-123", *got.UserID)
	assert.Equal(t, "team-1", got.TeamID)
	assert.Equal(t, "session-456", got.SessionID)
	require.NotNil(t, got.CWD)
	assert.Equal(t, "/repo", *got.CWD)
	assert.Equal(t, "PostToolUse", got.HookEventName)
	require.NotNil(t, got.ToolName)
	assert.Equal(t, "Bash", *got.ToolName)
	require.NotNil(t, got.ToolInput)
	assert.Equal(t, models.JSONBData{"command": "ls"}, *got.ToolInput)
	assert.Nil(t, got.ToolResponse)
	assert.Equal(t, models.JSONBData{"source": "test"}, got.Payload)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestClaudeCodeHooksRepository_GetByID_Errors(t *testing.T) {
	tests := []struct {
		name      string
		driverErr error
		wantIs    error
	}{
		{name: "no rows maps to wrapped ErrNoRows", driverErr: sql.ErrNoRows, wantIs: sql.ErrNoRows},
		{name: "driver error propagates wrapped", driverErr: sql.ErrConnDone, wantIs: sql.ErrConnDone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
			defer closeMockDB(t, mockDB)

			mock.ExpectQuery(`FROM claude_code_hooks_payload WHERE id = \$1 AND user_id = \$2`).
				WithArgs(42, "user-123").
				WillReturnError(tt.driverErr)

			got, err := repo.GetByID(context.Background(), "user-123", 42)
			require.Error(t, err)
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), "failed to get Claude Code hook payload")
			assert.ErrorIs(t, err, tt.wantIs)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestClaudeCodeHooksRepository_List(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		filters     repositories.ClaudeCodeHooksFilters
		setupMock   func(mock sqlmock.Sqlmock)
		expectTotal int
		expectPages int
		expectCount int
	}{
		{
			name:    "no filters emits no WHERE clause",
			filters: repositories.ClaudeCodeHooksFilters{Page: 1, Limit: 10},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`^SELECT COUNT\(\*\) FROM claude_code_hooks_payload$`).
					WillReturnRows(countResult(2))
				mock.ExpectQuery(`FROM claude_code_hooks_payload ORDER BY created_at DESC LIMIT 10 OFFSET 0$`).
					WillReturnRows(claudeCodeHookRows(now, 1, 2))
			},
			expectTotal: 2, expectPages: 1, expectCount: 2,
		},
		{
			name:    "user filter binds user_id in both count and page queries (tenancy)",
			filters: repositories.ClaudeCodeHooksFilters{UserID: strPtr("user-123"), Page: 1, Limit: 10},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`^SELECT COUNT\(\*\) FROM claude_code_hooks_payload WHERE \(user_id = \$1\)$`).
					WithArgs("user-123").
					WillReturnRows(countResult(1))
				mock.ExpectQuery(`FROM claude_code_hooks_payload WHERE \(user_id = \$1\) ORDER BY created_at DESC LIMIT 10 OFFSET 0$`).
					WithArgs("user-123").
					WillReturnRows(claudeCodeHookRows(now, 1))
			},
			expectTotal: 1, expectPages: 1, expectCount: 1,
		},
		{
			name: "all filters bind user, session, event and tool in declaration order",
			filters: repositories.ClaudeCodeHooksFilters{
				UserID: strPtr("user-123"), SessionID: strPtr("session-456"),
				HookEventName: strPtr("PostToolUse"), ToolName: strPtr("Bash"), Page: 1, Limit: 10,
			},
			setupMock: func(mock sqlmock.Sqlmock) {
				where := `WHERE \(user_id = \$1 AND session_id = \$2 AND hook_event_name = \$3 AND tool_name = \$4\)`
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM claude_code_hooks_payload `+where).
					WithArgs("user-123", "session-456", "PostToolUse", "Bash").
					WillReturnRows(countResult(1))
				mock.ExpectQuery(`FROM claude_code_hooks_payload `+where).
					WithArgs("user-123", "session-456", "PostToolUse", "Bash").
					WillReturnRows(claudeCodeHookRows(now, 1))
			},
			expectTotal: 1, expectPages: 1, expectCount: 1,
		},
		{
			name:    "pagination computes LIMIT and OFFSET from page and limit",
			filters: repositories.ClaudeCodeHooksFilters{UserID: strPtr("user-123"), Page: 3, Limit: 5},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnRows(countResult(12))
				// offset = (3-1)*5 = 10; total pages = ceil(12/5) = 3
				mock.ExpectQuery(`FROM claude_code_hooks_payload WHERE \(user_id = \$1\) ORDER BY created_at DESC LIMIT 5 OFFSET 10$`).
					WithArgs("user-123").
					WillReturnRows(claudeCodeHookRows(now, 11))
			},
			expectTotal: 12, expectPages: 3, expectCount: 1,
		},
		{
			name:    "empty result returns non-nil empty data",
			filters: repositories.ClaudeCodeHooksFilters{UserID: strPtr("user-123"), Page: 1, Limit: 10},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnRows(countResult(0))
				mock.ExpectQuery(`FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnRows(sqlmock.NewRows(claudeCodeHookListColumns))
			},
			expectTotal: 0, expectPages: 0, expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			resp, err := repo.List(context.Background(), tt.filters)

			require.NoError(t, err)
			assert.NotNil(t, resp.Data, "List must return a non-nil data slice, never nil")
			assert.Len(t, resp.Data, tt.expectCount)
			assert.Equal(t, tt.expectTotal, resp.Total)
			assert.Equal(t, tt.expectPages, resp.TotalPages)
			assert.Equal(t, tt.filters.Page, resp.Page)
			assert.Equal(t, tt.filters.Limit, resp.Limit)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestClaudeCodeHooksRepository_List_ScansRowsAndSkipsBad verifies rows are
// scanned into the model and an unscannable row is skipped, not fatal.
func TestClaudeCodeHooksRepository_List_ScansRowsAndSkipsBad(t *testing.T) {
	repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
	defer closeMockDB(t, mockDB)

	now := time.Now()
	rows := sqlmock.NewRows(claudeCodeHookListColumns).
		AddRow(
			"not-an-int", "user-123", "team-1", "session-456", nil, nil, "PostToolUse", nil,
			nil, nil, nil, nil, []byte(`{}`), now, now,
		).
		AddRow(
			2, "user-123", "team-1", "session-456", nil, "/repo", "PostToolUse", "Bash",
			[]byte(`{"command":"ls"}`), nil, nil, nil, []byte(`{"source":"test"}`), now, now,
		)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM claude_code_hooks_payload`).
		WithArgs("user-123").
		WillReturnRows(countResult(2))
	mock.ExpectQuery(`FROM claude_code_hooks_payload`).
		WithArgs("user-123").
		WillReturnRows(rows)

	resp, err := repo.List(context.Background(),
		repositories.ClaudeCodeHooksFilters{UserID: strPtr("user-123"), Page: 1, Limit: 10})

	require.NoError(t, err)
	require.Len(t, resp.Data, 1, "the unscannable row must be skipped")
	got := resp.Data[0]
	assert.Equal(t, 2, got.ID)
	assert.Equal(t, "session-456", got.SessionID)
	require.NotNil(t, got.ToolInput)
	assert.Equal(t, models.JSONBData{"command": "ls"}, *got.ToolInput)
	assert.Equal(t, models.JSONBData{"source": "test"}, got.Payload)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestClaudeCodeHooksRepository_List_Errors(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "count query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count Claude Code hook payloads",
		},
		{
			name: "page query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnRows(countResult(1))
				mock.ExpectQuery(`FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to query Claude Code hook payloads",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			resp, err := repo.List(context.Background(),
				repositories.ClaudeCodeHooksFilters{UserID: strPtr("user-123"), Page: 1, Limit: 10})

			require.Error(t, err)
			assert.Nil(t, resp)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.ErrorIs(t, err, sql.ErrConnDone)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestClaudeCodeHooksRepository_GetSessions(t *testing.T) {
	repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
	defer closeMockDB(t, mockDB)

	now := time.Now()
	sessionColumns := []string{"session_id", "first_seen", "last_seen", "hook_count", "latest_cwd", "unique_tools"}

	mock.ExpectQuery(`^SELECT COUNT\(DISTINCT session_id\) FROM claude_code_hooks_payload WHERE user_id = \$1$`).
		WithArgs("user-123").
		WillReturnRows(countResult(3))
	// page 2 with limit 2 must bind offset (2-1)*2 = 2
	mock.ExpectQuery(`GROUP BY p1\.session_id ORDER BY MAX\(p1\.created_at\) DESC LIMIT \$2 OFFSET \$3`).
		WithArgs("user-123", 2, 2).
		WillReturnRows(sqlmock.NewRows(sessionColumns).
			AddRow("session-b", now.Add(-2*time.Hour), now, 5, "/repo", 2).
			AddRow("session-a", now.Add(-3*time.Hour), now.Add(-time.Hour), 3, nil, 1))

	resp, err := repo.GetSessions(context.Background(),
		repositories.SessionFilters{UserID: strPtr("user-123"), Page: 2, Limit: 2})

	require.NoError(t, err)
	assert.Equal(t, 3, resp.Total)
	assert.Equal(t, 2, resp.TotalPages)
	require.Len(t, resp.Data, 2)
	assert.Equal(t, "session-b", resp.Data[0].SessionID)
	assert.Equal(t, 5, resp.Data[0].HookCount)
	require.NotNil(t, resp.Data[0].LatestCWD)
	assert.Equal(t, "/repo", *resp.Data[0].LatestCWD)
	assert.Equal(t, 2, resp.Data[0].UniqueTools)
	assert.Equal(t, "session-a", resp.Data[1].SessionID)
	assert.Nil(t, resp.Data[1].LatestCWD)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestClaudeCodeHooksRepository_GetSessions_EmptyAndErrors(t *testing.T) {
	tests := []struct {
		name      string
		filters   repositories.SessionFilters
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name:      "missing user id fails before any query",
			filters:   repositories.SessionFilters{Page: 1, Limit: 10},
			setupMock: func(sqlmock.Sqlmock) {},
			wantErr:   "user_id is required for session queries",
		},
		{
			name:    "count query error propagates wrapped",
			filters: repositories.SessionFilters{UserID: strPtr("user-123"), Page: 1, Limit: 10},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(DISTINCT session_id\) FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count Claude Code sessions",
		},
		{
			name:    "data query error propagates wrapped",
			filters: repositories.SessionFilters{UserID: strPtr("user-123"), Page: 1, Limit: 10},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(DISTINCT session_id\) FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnRows(countResult(1))
				mock.ExpectQuery(`GROUP BY p1\.session_id`).
					WithArgs("user-123", 10, 0).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to query Claude Code sessions",
		},
		{
			name:    "no sessions returns non-nil empty data",
			filters: repositories.SessionFilters{UserID: strPtr("user-123"), Page: 1, Limit: 10},
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(DISTINCT session_id\) FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnRows(countResult(0))
				mock.ExpectQuery(`GROUP BY p1\.session_id`).
					WithArgs("user-123", 10, 0).
					WillReturnRows(sqlmock.NewRows(
						[]string{"session_id", "first_seen", "last_seen", "hook_count", "latest_cwd", "unique_tools"}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			resp, err := repo.GetSessions(context.Background(), tt.filters)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Nil(t, resp)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resp.Data)
				assert.Empty(t, resp.Data)
				assert.Zero(t, resp.Total)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestClaudeCodeHooksRepository_GetSessionCounts(t *testing.T) {
	repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
	defer closeMockDB(t, mockDB)

	// The days argument is interpolated into the INTERVAL, not bound.
	mock.ExpectQuery(
		`SELECT COUNT\(DISTINCT session_id\) FROM claude_code_hooks_payload ` +
			`WHERE user_id = \$1 AND created_at >= CURRENT_DATE - INTERVAL '30 day'`,
	).
		WithArgs("user-123").
		WillReturnRows(countResult(5))
	mock.ExpectQuery(`GROUP BY DATE\(created_at\) ORDER BY date DESC`).
		WithArgs("user-123").
		WillReturnRows(sqlmock.NewRows([]string{"date", "count"}).
			AddRow("2026-07-18", 3).
			AddRow("2026-07-17", 2))

	resp, err := repo.GetSessionCounts(context.Background(), "user-123", 30)

	require.NoError(t, err)
	assert.Equal(t, 5, resp.TotalSessions)
	require.Len(t, resp.Counts, 2)
	assert.Equal(t, models.SessionCountByDate{Date: "2026-07-18", Count: 3}, resp.Counts[0])
	assert.Equal(t, models.SessionCountByDate{Date: "2026-07-17", Count: 2}, resp.Counts[1])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestClaudeCodeHooksRepository_GetSessionCounts_EmptyAndErrors(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name: "count query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(DISTINCT session_id\) FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count total Claude Code sessions",
		},
		{
			name: "per-date query error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(DISTINCT session_id\) FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnRows(countResult(1))
				mock.ExpectQuery(`GROUP BY DATE\(created_at\)`).
					WithArgs("user-123").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to query Claude Code session counts by date",
		},
		{
			name: "no counts returns non-nil empty slice",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(DISTINCT session_id\) FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnRows(countResult(0))
				mock.ExpectQuery(`GROUP BY DATE\(created_at\)`).
					WithArgs("user-123").
					WillReturnRows(sqlmock.NewRows([]string{"date", "count"}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			resp, err := repo.GetSessionCounts(context.Background(), "user-123", 30)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Nil(t, resp)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resp.Counts)
				assert.Empty(t, resp.Counts)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestClaudeCodeHooksRepository_GetOverviewStats(t *testing.T) {
	repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
	defer closeMockDB(t, mockDB)

	mock.ExpectQuery(`^SELECT COUNT\(DISTINCT session_id\) FROM claude_code_hooks_payload WHERE user_id = \$1$`).
		WithArgs("user-123").WillReturnRows(countResult(10))
	mock.ExpectQuery(`INTERVAL '7 day'`).
		WithArgs("user-123").WillReturnRows(countResult(6))
	mock.ExpectQuery(`INTERVAL '14 day'`).
		WithArgs("user-123").WillReturnRows(countResult(4))
	mock.ExpectQuery(`AVG\(prompt_count\)`).
		WithArgs("user-123").
		WillReturnRows(sqlmock.NewRows([]string{"avg_prompts"}).AddRow(3.5))
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT tool_name\) FROM claude_code_hooks_payload`).
		WithArgs("user-123").WillReturnRows(countResult(7))
	mock.ExpectQuery(`ORDER BY usage_count DESC LIMIT 3`).
		WithArgs("user-123").
		WillReturnRows(sqlmock.NewRows([]string{"tool_name", "usage_count"}).
			AddRow("Bash", 12).
			AddRow("Read", 9))
	mock.ExpectQuery(`EXTRACT\(EPOCH`).
		WithArgs("user-123").
		WillReturnRows(sqlmock.NewRows([]string{"avg_duration_minutes"}).AddRow(12.5))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories WHERE user_id = \$1`).
		WithArgs("user-123").WillReturnRows(countResult(9))

	stats, err := repo.GetOverviewStats(context.Background(), "user-123")

	require.NoError(t, err)
	assert.Equal(t, 10, stats.TotalSessions)
	assert.Equal(t, 6, stats.SessionsThisWeek)
	assert.Equal(t, 4, stats.SessionsLastWeek)
	assert.InDelta(t, 50.0, stats.WeeklyTrendPercent, 0.0001)
	assert.InDelta(t, 3.5, stats.AvgUserPromptsPerSession, 0.0001)
	assert.Equal(t, 7, stats.TotalUniqueTools)
	require.Len(t, stats.TopTools, 2)
	assert.Equal(t, models.ToolUsageCount{ToolName: "Bash", Count: 12}, stats.TopTools[0])
	assert.InDelta(t, 12.5, stats.AvgSessionDurationMinutes, 0.0001)
	assert.Equal(t, 9, stats.TotalMemories)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestClaudeCodeHooksRepository_GetOverviewStats_BestEffort verifies sub-stat
// query failures degrade to zero values instead of failing the whole call.
func TestClaudeCodeHooksRepository_GetOverviewStats_BestEffort(t *testing.T) {
	repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
	defer closeMockDB(t, mockDB)

	mock.ExpectQuery(`^SELECT COUNT\(DISTINCT session_id\) FROM claude_code_hooks_payload WHERE user_id = \$1$`).
		WithArgs("user-123").WillReturnRows(countResult(10))
	mock.ExpectQuery(`INTERVAL '7 day'`).
		WithArgs("user-123").WillReturnError(sql.ErrConnDone)
	mock.ExpectQuery(`INTERVAL '14 day'`).
		WithArgs("user-123").WillReturnRows(countResult(4))
	mock.ExpectQuery(`AVG\(prompt_count\)`).
		WithArgs("user-123").WillReturnError(sql.ErrConnDone)
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT tool_name\) FROM claude_code_hooks_payload`).
		WithArgs("user-123").WillReturnRows(countResult(7))
	mock.ExpectQuery(`ORDER BY usage_count DESC LIMIT 3`).
		WithArgs("user-123").WillReturnError(sql.ErrConnDone)
	mock.ExpectQuery(`EXTRACT\(EPOCH`).
		WithArgs("user-123").WillReturnError(sql.ErrConnDone)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM memories WHERE user_id = \$1`).
		WithArgs("user-123").WillReturnRows(countResult(9))

	stats, err := repo.GetOverviewStats(context.Background(), "user-123")

	require.NoError(t, err, "sub-stat failures must not fail the overview call")
	assert.Equal(t, 10, stats.TotalSessions)
	assert.Zero(t, stats.SessionsThisWeek)
	assert.Equal(t, 4, stats.SessionsLastWeek)
	assert.InDelta(t, -100.0, stats.WeeklyTrendPercent, 0.0001)
	assert.Zero(t, stats.AvgUserPromptsPerSession)
	assert.NotNil(t, stats.TopTools)
	assert.Empty(t, stats.TopTools)
	assert.Zero(t, stats.AvgSessionDurationMinutes)
	assert.Equal(t, 9, stats.TotalMemories)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestClaudeCodeHooksRepository_GetOverviewStats_TotalError verifies the one
// non-best-effort stat (total sessions) fails the call.
func TestClaudeCodeHooksRepository_GetOverviewStats_TotalError(t *testing.T) {
	repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
	defer closeMockDB(t, mockDB)

	mock.ExpectQuery(`SELECT COUNT\(DISTINCT session_id\) FROM claude_code_hooks_payload`).
		WithArgs("user-123").WillReturnError(sql.ErrConnDone)

	stats, err := repo.GetOverviewStats(context.Background(), "user-123")

	require.Error(t, err)
	assert.Nil(t, stats)
	assert.Contains(t, err.Error(), "failed to count total sessions")
	assert.ErrorIs(t, err, sql.ErrConnDone)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWeeklyTrendPercent(t *testing.T) {
	assert.InDelta(t, 50.0, weeklyTrendPercent(6, 4), 0.0001)
	assert.InDelta(t, -100.0, weeklyTrendPercent(0, 4), 0.0001)
	assert.InDelta(t, 100.0, weeklyTrendPercent(3, 0), 0.0001, "a rise from zero reports as a 100% increase")
	assert.Zero(t, weeklyTrendPercent(0, 0))
}

func TestClaudeCodeHooksRepository_GetRecentActivities(t *testing.T) {
	repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
	defer closeMockDB(t, mockDB)

	now := time.Now()
	activityColumns := []string{"session_id", "cwd", "tool_name", "tool_input", "hook_event_name", "created_at"}

	// user_id and tool_name IS NOT NULL are always present, even with no optional filters.
	where := `WHERE \(user_id = \$1 AND tool_name IS NOT NULL\)`
	mock.ExpectQuery(`^SELECT COUNT\(\*\) FROM claude_code_hooks_payload ` + where + `$`).
		WithArgs("user-123").
		WillReturnRows(countResult(1))
	mock.ExpectQuery(
		`^SELECT session_id, cwd, tool_name, tool_input, hook_event_name, created_at ` +
			`FROM claude_code_hooks_payload ` + where +
			` ORDER BY created_at DESC LIMIT 20 OFFSET 0$`,
	).
		WithArgs("user-123").
		WillReturnRows(sqlmock.NewRows(activityColumns).
			AddRow("session-456", "/repo", "Bash", []byte(`{"command":"ls"}`), "PostToolUse", now))

	resp, err := repo.GetRecentActivities(context.Background(),
		repositories.RecentActivitiesFilters{UserID: strPtr("user-123"), Page: 1, Limit: 20})

	require.NoError(t, err)
	assert.Equal(t, 1, resp.Total)
	assert.Equal(t, 1, resp.TotalPages)
	require.Len(t, resp.Activities, 1)
	got := resp.Activities[0]
	assert.Equal(t, "session-456", got.SessionID)
	require.NotNil(t, got.ToolName)
	assert.Equal(t, "Bash", *got.ToolName)
	require.NotNil(t, got.ToolInput)
	assert.Equal(t, models.JSONBData{"command": "ls"}, *got.ToolInput)
	assert.Equal(t, "PostToolUse", got.HookEventName)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestClaudeCodeHooksRepository_GetRecentActivities_FiltersAndPagination(t *testing.T) {
	repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
	defer closeMockDB(t, mockDB)

	where := `WHERE \(user_id = \$1 AND tool_name IS NOT NULL AND session_id = \$2 AND tool_name = \$3 ` +
		`AND hook_event_name = \$4 AND created_at >= \$5 AND created_at <= \$6\)`
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM claude_code_hooks_payload `+where).
		WithArgs("user-123", "session-456", "Bash", "PostToolUse", "2026-07-01", "2026-07-18").
		WillReturnRows(countResult(11))
	// offset = (3-1)*5 = 10; total pages = ceil(11/5) = 3
	mock.ExpectQuery(`FROM claude_code_hooks_payload `+where+` ORDER BY created_at DESC LIMIT 5 OFFSET 10$`).
		WithArgs("user-123", "session-456", "Bash", "PostToolUse", "2026-07-01", "2026-07-18").
		WillReturnRows(sqlmock.NewRows(
			[]string{"session_id", "cwd", "tool_name", "tool_input", "hook_event_name", "created_at"}).
			AddRow("session-456", nil, "Bash", nil, "PostToolUse", time.Now()))

	resp, err := repo.GetRecentActivities(context.Background(), repositories.RecentActivitiesFilters{
		UserID:        strPtr("user-123"),
		SessionID:     strPtr("session-456"),
		ToolName:      strPtr("Bash"),
		HookEventName: strPtr("PostToolUse"),
		DateFrom:      strPtr("2026-07-01"),
		DateTo:        strPtr("2026-07-18"),
		Page:          3,
		Limit:         5,
	})

	require.NoError(t, err)
	assert.Equal(t, 11, resp.Total)
	assert.Equal(t, 3, resp.TotalPages)
	assert.Len(t, resp.Activities, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestClaudeCodeHooksRepository_GetRecentActivities_EmptyAndErrors(t *testing.T) {
	baseFilters := repositories.RecentActivitiesFilters{UserID: strPtr("user-123"), Page: 1, Limit: 20}
	activityColumns := []string{"session_id", "cwd", "tool_name", "tool_input", "hook_event_name", "created_at"}

	tests := []struct {
		name      string
		filters   repositories.RecentActivitiesFilters
		setupMock func(mock sqlmock.Sqlmock)
		wantErr   string
	}{
		{
			name:      "missing user id fails before any query",
			filters:   repositories.RecentActivitiesFilters{Page: 1, Limit: 20},
			setupMock: func(sqlmock.Sqlmock) {},
			wantErr:   "user_id is required for activities queries",
		},
		{
			name:    "count query error propagates wrapped",
			filters: baseFilters,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to count Claude Code activities",
		},
		{
			name:    "page query error propagates wrapped",
			filters: baseFilters,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnRows(countResult(1))
				mock.ExpectQuery(`FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to query recent Claude Code activities",
		},
		{
			name:    "no activities returns non-nil empty slice",
			filters: baseFilters,
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnRows(countResult(0))
				mock.ExpectQuery(`FROM claude_code_hooks_payload`).
					WithArgs("user-123").
					WillReturnRows(sqlmock.NewRows(activityColumns))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			resp, err := repo.GetRecentActivities(context.Background(), tt.filters)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Nil(t, resp)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resp.Activities)
				assert.Empty(t, resp.Activities)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestClaudeCodeHooksRepository_SessionExists(t *testing.T) {
	existsQuery := `SELECT EXISTS\(SELECT 1 FROM claude_code_hooks_payload WHERE user_id = \$1 AND session_id = \$2\)`

	tests := []struct {
		name       string
		setupMock  func(mock sqlmock.Sqlmock)
		wantExists bool
		wantErr    string
	}{
		{
			name: "existing session returns true",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(existsQuery).
					WithArgs("user-123", "session-456").
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			},
			wantExists: true,
		},
		{
			name: "missing session returns false",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(existsQuery).
					WithArgs("user-123", "session-456").
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			},
		},
		{
			name: "driver error propagates wrapped",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(existsQuery).
					WithArgs("user-123", "session-456").
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: "failed to check if session exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
			defer closeMockDB(t, mockDB)

			tt.setupMock(mock)

			exists, err := repo.SessionExists(context.Background(), "user-123", "session-456")

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.False(t, exists)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantExists, exists)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestClaudeCodeHooksRepository_CountUniqueSessions(t *testing.T) {
	t.Run("returns the distinct session count for the user", func(t *testing.T) {
		repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`^SELECT COUNT\(DISTINCT session_id\) FROM claude_code_hooks_payload WHERE user_id = \$1$`).
			WithArgs("user-123").
			WillReturnRows(countResult(4))

		count, err := repo.CountUniqueSessions(context.Background(), "user-123")
		require.NoError(t, err)
		assert.Equal(t, 4, count)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("driver error propagates wrapped", func(t *testing.T) {
		repo, mock, mockDB := setupClaudeCodeDeleteTest(t)
		defer closeMockDB(t, mockDB)

		mock.ExpectQuery(`SELECT COUNT\(DISTINCT session_id\) FROM claude_code_hooks_payload`).
			WithArgs("user-123").
			WillReturnError(sql.ErrConnDone)

		count, err := repo.CountUniqueSessions(context.Background(), "user-123")
		require.Error(t, err)
		assert.Zero(t, count)
		assert.Contains(t, err.Error(), "failed to count unique sessions")
		assert.ErrorIs(t, err, sql.ErrConnDone)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestClaudeCodeHooksRepository_NilDB(t *testing.T) {
	repo := &claudeCodeHooksRepository{db: nil}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"Create", func() error { return repo.Create(ctx, &models.ClaudeCodeHookPayload{}) }},
		{"GetByID", func() error { _, err := repo.GetByID(ctx, "user-123", 1); return err }},
		{"List", func() error {
			_, err := repo.List(ctx, repositories.ClaudeCodeHooksFilters{Page: 1, Limit: 10})
			return err
		}},
		{"GetSessions", func() error {
			_, err := repo.GetSessions(ctx, repositories.SessionFilters{UserID: strPtr("user-123"), Page: 1, Limit: 10})
			return err
		}},
		{"GetSessionCounts", func() error { _, err := repo.GetSessionCounts(ctx, "user-123", 30); return err }},
		{"GetOverviewStats", func() error { _, err := repo.GetOverviewStats(ctx, "user-123"); return err }},
		{"GetRecentActivities", func() error {
			_, err := repo.GetRecentActivities(ctx,
				repositories.RecentActivitiesFilters{UserID: strPtr("user-123"), Page: 1, Limit: 10})
			return err
		}},
		{"SessionExists", func() error { _, err := repo.SessionExists(ctx, "user-123", "session-456"); return err }},
		{"CountUniqueSessions", func() error { _, err := repo.CountUniqueSessions(ctx, "user-123"); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualError(t, tt.call(), "database connection is nil")
		})
	}
}
