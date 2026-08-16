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

// freshnessAuditCols mirrors the freshnessAuditColumns projection order — the
// STORED columns, which is what Create's INSERT ... RETURNING can produce.
func freshnessAuditCols() []string {
	return []string{
		"id", "team_id", "resource_type", "resource_id", "rule_id",
		"action", "reason", "created_at",
	}
}

// freshnessAuditListCols mirrors freshnessAuditListColumns: the stored columns
// plus the two resolved by joining the live resource row (#789). ListByTeam
// scans this shape; Create must NOT, since its RETURNING cannot produce them.
func freshnessAuditListCols() []string {
	return append(freshnessAuditCols(), "slug", "project_id")
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

// freshnessAuditListRow is the ListByTeam row shape: the stored columns plus the
// join-resolved slug/project_id. Pass nil for either to model a resource that has
// since been deleted (or a memory, which never has a slug).
func freshnessAuditListRow(
	ruleID interface{}, action, reason string, slug, projectID interface{},
) *sqlmock.Rows {
	return sqlmock.NewRows(freshnessAuditListCols()).AddRow(
		"audit-1", "team-1", "artifact", "res-1", ruleID,
		action, reason, time.Now().UTC(), slug, projectID,
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
		WillReturnRows(freshnessAuditListRow("rule-1",
			models.FreshnessActionMarked, models.FreshnessReasonRuleRun,
			"my-artifact", "project-9"))

	entries, total, err := repo.ListByTeam(context.Background(), "team-1", 25, 50)

	require.NoError(t, err)
	assert.Equal(t, 42, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "audit-1", entries[0].ID)
	require.NotNil(t, entries[0].Slug)
	assert.Equal(t, "my-artifact", *entries[0].Slug)
	require.NotNil(t, entries[0].ProjectID)
	assert.Equal(t, "project-9", *entries[0].ProjectID)
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
				WillReturnRows(sqlmock.NewRows(freshnessAuditListCols()))

			entries, total, err := repo.ListByTeam(context.Background(), "team-1", tt.limit, tt.offset)

			require.NoError(t, err)
			assert.Equal(t, 0, total)
			assert.Equal(t, []*models.ResourceFreshnessAudit{}, entries)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// Create's INSERT ... RETURNING cannot produce the joined columns, so it must
// keep scanning the narrow set. If it were widened alongside ListByTeam it would
// try to read two columns the statement never returns.
func TestFreshnessAuditRepository_Create_DoesNotScanJoinedColumns(t *testing.T) {
	repo, mock := setupFreshnessAuditTest(t)
	ruleID := "rule-1"

	// Narrow rows on purpose: a Create that scanned the list shape fails here.
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

	assert.Nil(t, entry.Slug, "slug is resolved at read time, never by Create")
	assert.Nil(t, entry.ProjectID, "project_id is resolved at read time, never by Create")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A resource deleted after its event was logged joins to nothing, so both fields
// come back NULL and must scan into nil pointers rather than erroring — that is
// what lets the client render plain text instead of a broken link.
func TestFreshnessAuditRepository_ListByTeam_DeletedResourceScansNull(t *testing.T) {
	repo, mock := setupFreshnessAuditTest(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM resource_freshness_audit`).
		WillReturnRows(freshnessAuditListRow("rule-1",
			models.FreshnessActionMarked, models.FreshnessReasonRuleRun, nil, nil))

	entries, _, err := repo.ListByTeam(context.Background(), "team-1", 10, 0)

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Nil(t, entries[0].Slug)
	assert.Nil(t, entries[0].ProjectID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The page-first-then-join shape is load-bearing: joining before paging would fan
// the four LEFT JOINs across the team's whole history and stop the
// team_id/created_at index from driving the page.
func TestFreshnessAuditRepository_ListByTeam_PagesBeforeJoining(t *testing.T) {
	repo, mock := setupFreshnessAuditTest(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// LIMIT/OFFSET must appear INSIDE the subquery, before any LEFT JOIN, and the
	// outer select must re-apply the ordering a join does not preserve.
	mock.ExpectQuery(
		`FROM \( SELECT .* FROM resource_freshness_audit WHERE team_id = \$1 `+
			`ORDER BY created_at DESC, id DESC LIMIT \$2 OFFSET \$3 \) a `+
			`LEFT JOIN prompts .* LEFT JOIN artifacts .* LEFT JOIN blueprints .* LEFT JOIN memories .* `+
			`ORDER BY a.created_at DESC, a.id DESC`,
	).WithArgs("team-1", 10, 0).
		WillReturnRows(freshnessAuditListRow("rule-1",
			models.FreshnessActionMarked, models.FreshnessReasonRuleRun, "s", "p"))

	_, _, err := repo.ListByTeam(context.Background(), "team-1", 10, 0)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
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

	// The AT TIME ZONE 'UTC' is the point of the assertion, not incidental: a
	// bare DATE() would truncate in the session timezone and mis-key the
	// series (#773).
	mock.ExpectQuery(
		`SELECT TO_CHAR\(\(created_at AT TIME ZONE 'UTC'\)::date, 'YYYY-MM-DD'\) AS date, action, COUNT\(\*\).+`+
			`FROM resource_freshness_audit.+WHERE team_id = \$1 AND created_at >= \$2.+`+
			`GROUP BY \(created_at AT TIME ZONE 'UTC'\)::date, action`).
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
				mock.ExpectQuery(`GROUP BY \(created_at AT TIME ZONE 'UTC'\)::date`).WillReturnError(errors.New("boom"))
			},
			wantIn: "failed to count freshness transitions by day",
		},
		{
			name: "scan fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`GROUP BY \(created_at AT TIME ZONE 'UTC'\)::date`).
					WillReturnRows(sqlmock.NewRows([]string{"date"}).AddRow("2026-05-01"))
			},
			wantIn: "failed to scan freshness transition count",
		},
		{
			name: "iteration fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`GROUP BY \(created_at AT TIME ZONE 'UTC'\)::date`).
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

// The repair query (#796) drives off the STATE table and asks the log only for
// each live row's newest entry, so what comes back is bounded by what is
// currently stale rather than by everything the team ever audited.
//
// The expectation pins the whole shape, because each piece is load-bearing and
// a laxer regex would match a query that answers a different question: the
// FROM must be resource_freshness (not the log), the subquery must order by
// created_at DESC with the id tiebreaker (a run writes one transaction
// timestamp to every row, so without it "newest" is undefined between a mark
// and a clear), and the predicate must accept a NULL action -- that is the
// resource with no audit history at all, which this query answers rather than
// leaving the caller to infer from an absence.
func TestFreshnessAuditRepository_ListStaleResourcesMissingMark(t *testing.T) {
	repo, mock := setupFreshnessAuditTest(t)

	mock.ExpectQuery(`FROM resource_freshness f\s+LEFT JOIN LATERAL \(\s+SELECT a\.action\s+`+
		`FROM resource_freshness_audit a\s+WHERE a\.team_id = f\.team_id\s+`+
		`AND a\.resource_type = f\.resource_type\s+AND a\.resource_id = f\.resource_id\s+`+
		`ORDER BY a\.created_at DESC, a\.id DESC\s+LIMIT 1\s+\) latest ON TRUE\s+`+
		`WHERE f\.team_id = \$1\s+AND \(latest\.action IS NULL OR latest\.action <> \$2\)`).
		WithArgs("team-1", models.FreshnessActionMarked).
		WillReturnRows(sqlmock.NewRows([]string{"resource_type", "resource_id"}).
			AddRow("artifact", "res-1").
			AddRow("prompt", "res-2"))

	refs, err := repo.ListStaleResourcesMissingMark(context.Background(), "team-1")
	require.NoError(t, err)

	assert.Equal(t, []models.FreshnessResourceRef{
		{ResourceType: "artifact", ResourceID: "res-1"},
		{ResourceType: "prompt", ResourceID: "res-2"},
	}, refs)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Nothing to repair is the normal case, and it must come back as an empty
// slice rather than nil: the caller ranges over it either way, but a nil here
// would be the only list in this repository that is not [].
func TestFreshnessAuditRepository_ListStaleResourcesMissingMark_Empty(t *testing.T) {
	repo, mock := setupFreshnessAuditTest(t)

	mock.ExpectQuery(`FROM resource_freshness f`).
		WithArgs("team-1", models.FreshnessActionMarked).
		WillReturnRows(sqlmock.NewRows([]string{"resource_type", "resource_id"}))

	refs, err := repo.ListStaleResourcesMissingMark(context.Background(), "team-1")
	require.NoError(t, err)
	assert.Equal(t, []models.FreshnessResourceRef{}, refs)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFreshnessAuditRepository_ListStaleResourcesMissingMark_Errors(t *testing.T) {
	tests := []struct {
		name    string
		arrange func(sqlmock.Sqlmock)
		wantErr string
	}{
		{
			name: "query fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM resource_freshness f`).WillReturnError(errors.New("boom"))
			},
			wantErr: "failed to list stale resources missing a mark",
		},
		{
			name: "row does not scan",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM resource_freshness f`).WillReturnRows(
					sqlmock.NewRows([]string{"resource_type"}).AddRow("only-one-column"),
				)
			},
			wantErr: "failed to scan stale resource missing a mark",
		},
		{
			name: "iteration fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM resource_freshness f`).WillReturnRows(
					sqlmock.NewRows([]string{"resource_type", "resource_id"}).
						AddRow("artifact", "res-1").
						RowError(0, errors.New("boom")),
				)
			},
			wantErr: "failed to iterate stale resources missing a mark",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := setupFreshnessAuditTest(t)
			tt.arrange(mock)

			_, err := repo.ListStaleResourcesMissingMark(context.Background(), "team-1")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
