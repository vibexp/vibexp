package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// These sqlmock tests cover only the paths the integration suite
// (backoffice_integration_test.go) cannot reach against a real database:
// driver failures in getWeekStarts / GetUserActivities and the
// swallow-error-to-0 semantics of the per-week sub-counts.

func newBackofficeMockRepo(t *testing.T) (repositories.BackofficeRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	return NewBackofficeRepository(&database.DB{DB: mockDB}), mock, mockDB
}

// userActivityColumns mirrors the 12-column projection of the user-activity
// aggregation query.
var userActivityColumns = []string{
	"user_id", "email", "name", "user_created_at",
	"total_artifacts", "first_artifact_created_at",
	"total_memories", "first_memory_created_at",
	"total_prompts", "first_prompt_created_at",
	"total_agents_created", "total_agent_executions_run",
}

const timelinePattern = `SELECT DISTINCT date_trunc`

func TestBackofficeRepository_GetUsageMetrics_TimelineErrors(t *testing.T) {
	cases := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantSub   string
	}{
		{
			name: "driver error querying the timeline",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(timelinePattern).WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to query timeline",
		},
		{
			name: "unscannable week start",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(timelinePattern).
					WillReturnRows(sqlmock.NewRows([]string{"week_start"}).AddRow("not-a-time"))
			},
			wantSub: "failed to scan week start",
		},
		{
			name: "row iteration error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(timelinePattern).
					WillReturnRows(sqlmock.NewRows([]string{"week_start"}).
						AddRow(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)).
						RowError(0, sql.ErrConnDone))
			},
			wantSub: "error iterating timeline rows",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, mockDB := newBackofficeMockRepo(t)
			defer closeMockDB(t, mockDB)

			tc.setupMock(mock)

			got, err := repo.GetUsageMetrics(context.Background(), nil, nil)
			require.Error(t, err)
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), tc.wantSub)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestBackofficeRepository_GetUsageMetrics_SubqueryErrorsSwallowedToZero pins
// the countByTable / countDistinctSessions contract: a failing per-week
// sub-count yields 0 for that metric while the call as a whole still succeeds
// and the healthy counts survive.
func TestBackofficeRepository_GetUsageMetrics_SubqueryErrorsSwallowedToZero(t *testing.T) {
	repo, mock, mockDB := newBackofficeMockRepo(t)
	defer closeMockDB(t, mockDB)

	weekStart := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	weekEnd := weekStart.AddDate(0, 0, 6).Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	mock.ExpectQuery(timelinePattern).
		WillReturnRows(sqlmock.NewRows([]string{"week_start"}).AddRow(weekStart))

	// buildUsageMetricsRow issues the nine sub-counts in this fixed order.
	mock.ExpectQuery(`FROM users WHERE created_at`).
		WithArgs(weekStart, weekEnd).
		WillReturnRows(countResult(5))
	mock.ExpectQuery(`FROM artifacts WHERE created_at`).WillReturnError(sql.ErrConnDone)
	mock.ExpectQuery(`FROM memories WHERE created_at`).WillReturnRows(countResult(2))
	mock.ExpectQuery(`FROM api_keys WHERE created_at`).WillReturnError(sql.ErrConnDone)
	mock.ExpectQuery(`FROM prompts WHERE created_at`).WillReturnRows(countResult(1))
	mock.ExpectQuery(`FROM agents WHERE created_at`).WillReturnRows(countResult(3))
	mock.ExpectQuery(`FROM agent_executions WHERE started_at`).
		WithArgs(weekStart, weekEnd).
		WillReturnRows(countResult(4))
	mock.ExpectQuery(`COUNT\(DISTINCT session_id\), 0\) FROM claude_code_hooks_payload`).
		WillReturnError(sql.ErrConnDone)
	mock.ExpectQuery(`COUNT\(DISTINCT session_id\), 0\) FROM cursor_ide_hooks_payload`).
		WillReturnRows(countResult(2))

	got, err := repo.GetUsageMetrics(context.Background(), nil, nil)
	require.NoError(t, err, "a failing sub-count must not fail the whole call")
	require.Len(t, got, 1)
	assert.Equal(t, models.UsageMetricsRow{
		WeekStart:           weekStart,
		NewUsers:            5,
		NewArtifacts:        0, // swallowed driver error
		NewMemories:         2,
		NewAPIKeys:          0, // swallowed driver error
		NewPrompts:          1,
		NewAgents:           3,
		AgentExecutions:     4,
		ClaudeSessions:      0, // swallowed driver error
		CursorSessions:      2,
		TotalAIToolSessions: 2,
	}, got[0])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBackofficeRepository_GetUserActivities_Errors(t *testing.T) {
	now := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		setupMock func(mock sqlmock.Sqlmock)
		wantSub   string
	}{
		{
			name: "driver error querying user activities",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM users u`).WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to query user activities",
		},
		{
			name: "unscannable row",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM users u`).
					WillReturnRows(sqlmock.NewRows(userActivityColumns).
						AddRow("user-1", "user@example.com", nil, now,
							"not-an-int", nil, 0, nil, 0, nil, 0, 0))
			},
			wantSub: "failed to scan user activity row",
		},
		{
			name: "row iteration error",
			setupMock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM users u`).
					WillReturnRows(sqlmock.NewRows(userActivityColumns).
						AddRow("user-1", "user@example.com", nil, now, 0, nil, 0, nil, 0, nil, 0, 0).
						RowError(0, sql.ErrConnDone))
			},
			wantSub: "error iterating user activity rows",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, mockDB := newBackofficeMockRepo(t)
			defer closeMockDB(t, mockDB)

			tc.setupMock(mock)

			got, err := repo.GetUserActivities(context.Background())
			require.Error(t, err)
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), tc.wantSub)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
