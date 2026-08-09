package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

// freshnessAuditCols mirrors the freshnessAuditColumns projection order.
func freshnessAuditCols() []string {
	return []string{
		"id", "team_id", "resource_type", "resource_id", "rule_id",
		"action", "reason", "created_at",
	}
}

func setupFreshnessAuditTest(t *testing.T) (*FreshnessAuditRepository, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	repo := NewFreshnessAuditRepository(&database.DB{DB: mockDB})
	return repo.(*FreshnessAuditRepository), mock
}

func freshnessAuditRow(ruleID interface{}, action, reason string) *sqlmock.Rows {
	return sqlmock.NewRows(freshnessAuditCols()).AddRow(
		"audit-1", "team-1", "artifact", "res-1", ruleID,
		action, reason, time.Now().UTC(),
	)
}

func TestFreshnessAuditRepository_Create_Marked(t *testing.T) {
	repo, mock := setupFreshnessAuditTest(t)
	ruleID := "rule-1"

	mock.ExpectQuery(`INSERT INTO resource_freshness_audit`).
		WithArgs("team-1", "artifact", "res-1", &ruleID,
			models.FreshnessActionMarked, models.FreshnessReasonRuleRun).
		WillReturnRows(freshnessAuditRow("rule-1",
			models.FreshnessActionMarked, models.FreshnessReasonRuleRun))

	entry := &models.ResourceFreshnessAudit{
		TeamID: "team-1", ResourceType: "artifact", ResourceID: "res-1",
		RuleID: &ruleID, Action: models.FreshnessActionMarked,
		Reason: models.FreshnessReasonRuleRun,
	}
	require.NoError(t, repo.Create(context.Background(), entry))

	assert.Equal(t, "audit-1", entry.ID)
	assert.False(t, entry.CreatedAt.IsZero())
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A clear caused by an access or an edit is attributable to no rule, so
// rule_id must accept NULL rather than forcing a placeholder id.
func TestFreshnessAuditRepository_Create_ClearedWithoutRule(t *testing.T) {
	repo, mock := setupFreshnessAuditTest(t)

	mock.ExpectQuery(`INSERT INTO resource_freshness_audit`).
		WithArgs("team-1", "prompt", "res-2", (*string)(nil),
			models.FreshnessActionCleared, models.FreshnessReasonAccessed).
		WillReturnRows(freshnessAuditRow(nil,
			models.FreshnessActionCleared, models.FreshnessReasonAccessed))

	entry := &models.ResourceFreshnessAudit{
		TeamID: "team-1", ResourceType: "prompt", ResourceID: "res-2",
		Action: models.FreshnessActionCleared, Reason: models.FreshnessReasonAccessed,
	}
	require.NoError(t, repo.Create(context.Background(), entry))

	assert.Nil(t, entry.RuleID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFreshnessAuditRepository_Create_Error(t *testing.T) {
	repo, mock := setupFreshnessAuditTest(t)

	mock.ExpectQuery(`INSERT INTO resource_freshness_audit`).WillReturnError(errors.New("boom"))

	err := repo.Create(context.Background(), &models.ResourceFreshnessAudit{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create freshness audit entry")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The `id DESC` tiebreaker is load-bearing: one rule run writes an identical
// created_at (transaction-start time) to every row it inserts, so without it
// pagination can repeat or skip entries.
func TestFreshnessAuditRepository_ListByTeam_OrdersWithTiebreaker(t *testing.T) {
	repo, mock := setupFreshnessAuditTest(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM resource_freshness_audit WHERE team_id = \$1`).
		WithArgs("team-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))
	mock.ExpectQuery(
		`FROM resource_freshness_audit WHERE team_id = \$1 `+
			`ORDER BY created_at DESC, id DESC LIMIT \$2 OFFSET \$3`,
	).WithArgs("team-1", 25, 50).
		WillReturnRows(freshnessAuditRow("rule-1",
			models.FreshnessActionMarked, models.FreshnessReasonRuleRun))

	entries, total, err := repo.ListByTeam(context.Background(), "team-1", 25, 50)

	require.NoError(t, err)
	assert.Equal(t, 42, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "audit-1", entries[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A zero limit binds NULL, which Postgres reads as "no limit" — that keeps one
// static query serving both the paged and unpaged cases. A negative offset is
// clamped rather than passed through, since Postgres would reject it.
func TestFreshnessAuditRepository_ListByTeam_PagingEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		limit      int
		offset     int
		wantLimit  interface{}
		wantOffset int
	}{
		{name: "zero limit means no limit", limit: 0, offset: 0, wantLimit: nil, wantOffset: 0},
		{name: "negative limit means no limit", limit: -5, offset: 0, wantLimit: nil, wantOffset: 0},
		{name: "negative offset clamps to zero", limit: 10, offset: -3, wantLimit: 10, wantOffset: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := setupFreshnessAuditTest(t)

			mock.ExpectQuery(`SELECT COUNT\(\*\)`).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			mock.ExpectQuery(`FROM resource_freshness_audit`).
				WithArgs("team-1", tt.wantLimit, tt.wantOffset).
				WillReturnRows(sqlmock.NewRows(freshnessAuditCols()))

			entries, total, err := repo.ListByTeam(context.Background(), "team-1", tt.limit, tt.offset)

			require.NoError(t, err)
			assert.Equal(t, 0, total)
			assert.Equal(t, []*models.ResourceFreshnessAudit{}, entries)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestFreshnessAuditRepository_ListByTeam_CountError(t *testing.T) {
	repo, mock := setupFreshnessAuditTest(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).WillReturnError(errors.New("boom"))

	_, _, err := repo.ListByTeam(context.Background(), "team-1", 10, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to count freshness audit entries")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFreshnessAuditRepository_ListByTeam_QueryError(t *testing.T) {
	repo, mock := setupFreshnessAuditTest(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM resource_freshness_audit`).WillReturnError(errors.New("boom"))

	_, _, err := repo.ListByTeam(context.Background(), "team-1", 10, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list freshness audit entries")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFreshnessAuditRepository_ListByTeam_ScanError(t *testing.T) {
	repo, mock := setupFreshnessAuditTest(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM resource_freshness_audit`).WillReturnRows(
		sqlmock.NewRows([]string{"id"}).AddRow("only-one-column"),
	)

	_, _, err := repo.ListByTeam(context.Background(), "team-1", 10, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan freshness audit entry")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The date is rendered in SQL as text in the exact series layout, so the
// zero-fill keys align without the caller parsing anything back.
func TestFreshnessAuditRepository_CountTransitionsByDay(t *testing.T) {
	repo, mock := setupFreshnessAuditTest(t)
	since := time.Now().UTC().AddDate(0, 0, -7)

	mock.ExpectQuery(`SELECT TO_CHAR\(DATE\(created_at\), 'YYYY-MM-DD'\) AS date, action, COUNT\(\*\).+`+
		`FROM resource_freshness_audit.+WHERE team_id = \$1 AND created_at >= \$2.+GROUP BY DATE\(created_at\), action`).
		WithArgs("team-1", since).
		WillReturnRows(sqlmock.NewRows([]string{"date", "action", "count"}).
			AddRow("2026-05-01", models.FreshnessActionMarked, 3).
			AddRow("2026-05-01", models.FreshnessActionCleared, 1))

	got, err := repo.CountTransitionsByDay(context.Background(), "team-1", since)

	require.NoError(t, err)
	assert.Equal(t, []models.FreshnessTransitionCount{
		{Date: "2026-05-01", Action: models.FreshnessActionMarked, Count: 3},
		{Date: "2026-05-01", Action: models.FreshnessActionCleared, Count: 1},
	}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFreshnessAuditRepository_CountTransitionsByDay_Errors(t *testing.T) {
	tests := []struct {
		name    string
		arrange func(mock sqlmock.Sqlmock)
		wantIn  string
	}{
		{
			name: "query fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`GROUP BY DATE`).WillReturnError(errors.New("boom"))
			},
			wantIn: "failed to count freshness transitions by day",
		},
		{
			name: "scan fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`GROUP BY DATE`).
					WillReturnRows(sqlmock.NewRows([]string{"date"}).AddRow("2026-05-01"))
			},
			wantIn: "failed to scan freshness transition count",
		},
		{
			name: "iteration fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`GROUP BY DATE`).
					WillReturnRows(sqlmock.NewRows([]string{"date", "action", "count"}).
						AddRow("2026-05-01", "marked", 1).RowError(0, errors.New("stream broken")))
			},
			wantIn: "failed to iterate freshness transition counts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := setupFreshnessAuditTest(t)
			tt.arrange(mock)

			_, err := repo.CountTransitionsByDay(context.Background(), "team-1", time.Now())

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantIn)
		})
	}
}
