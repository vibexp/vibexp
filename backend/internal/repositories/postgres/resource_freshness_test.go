package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

// resourceFreshnessCols mirrors the resourceFreshnessColumns projection order.
func resourceFreshnessCols() []string {
	return []string{
		"id", "team_id", "project_id", "resource_type", "resource_id",
		"status", "matched_rule_ids", "since", "reason", "created_at", "updated_at",
	}
}

func setupResourceFreshnessTest(t *testing.T) (*ResourceFreshnessRepository, sqlmock.Sqlmock) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		// Registered here rather than at setup: sqlmock matches expectations in
		// order, so an ExpectClose declared up front would be expected before
		// the test's own query.
		mock.ExpectClose()
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	})

	repo := NewResourceFreshnessRepository(&database.DB{DB: mockDB})
	return repo.(*ResourceFreshnessRepository), mock
}

// resourceFreshnessRow builds a single result row with the given rule-id array
// literal, as Postgres would return a uuid[].
func resourceFreshnessRow(ruleIDs string, since time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(resourceFreshnessCols()).AddRow(
		"fresh-1", "team-1", "proj-1", "artifact", "res-1",
		models.FreshnessStatusStale, []byte(ruleIDs), since,
		models.FreshnessReasonRuleRun, since, since,
	)
}

func TestResourceFreshnessRepository_Upsert_PopulatesModel(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)
	since := time.Now().UTC().Truncate(time.Second)

	mock.ExpectQuery(`INSERT INTO resource_freshness`).
		WithArgs(
			"team-1", "proj-1", "artifact", "res-1", models.FreshnessStatusStale,
			pq.Array([]string{"rule-a"}), since, models.FreshnessReasonRuleRun,
		).
		WillReturnRows(resourceFreshnessRow("{rule-a}", since))

	f := &models.ResourceFreshness{
		TeamID: "team-1", ProjectID: "proj-1",
		ResourceType: "artifact", ResourceID: "res-1",
		Status: models.FreshnessStatusStale, MatchedRuleIDs: []string{"rule-a"},
		Since: since, Reason: models.FreshnessReasonRuleRun,
	}
	require.NoError(t, repo.Upsert(context.Background(), f))

	assert.Equal(t, "fresh-1", f.ID)
	assert.Equal(t, []string{"rule-a"}, f.MatchedRuleIDs)
	assert.True(t, f.Since.Equal(since))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A zero Since must bind NULL so the COALESCE in the statement defers to the
// database clock; binding the zero time would persist year 1.
func TestResourceFreshnessRepository_Upsert_ZeroSinceBindsNull(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)
	dbSince := time.Now().UTC()

	mock.ExpectQuery(`INSERT INTO resource_freshness`).
		WithArgs(
			"team-1", "proj-1", "prompt", "res-2", models.FreshnessStatusStale,
			pq.Array([]string{}), nil, models.FreshnessReasonRuleRun,
		).
		WillReturnRows(resourceFreshnessRow("{}", dbSince))

	f := &models.ResourceFreshness{
		TeamID: "team-1", ProjectID: "proj-1",
		ResourceType: "prompt", ResourceID: "res-2",
		Status: models.FreshnessStatusStale, MatchedRuleIDs: []string{},
		Reason: models.FreshnessReasonRuleRun,
	}
	require.NoError(t, repo.Upsert(context.Background(), f))

	assert.False(t, f.Since.IsZero(), "Since must come back from the database")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// `since` is deliberately absent from the ON CONFLICT SET list: it means
// "first marked stale at" and must survive re-evaluation of a resource that
// stays stale.
func TestResourceFreshnessRepository_Upsert_DoesNotOverwriteSince(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)
	since := time.Now().UTC()

	mock.ExpectQuery(
		`ON CONFLICT \(resource_type, resource_id\) DO UPDATE SET team_id = EXCLUDED\.team_id, ` +
			`project_id = EXCLUDED\.project_id, status = EXCLUDED\.status, ` +
			`matched_rule_ids = EXCLUDED\.matched_rule_ids, reason = EXCLUDED\.reason, ` +
			`updated_at = now\(\) RETURNING`,
	).WillReturnRows(resourceFreshnessRow("{}", since))

	require.NoError(t, repo.Upsert(context.Background(), &models.ResourceFreshness{Since: since}))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestResourceFreshnessRepository_Upsert_Error(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectQuery(`INSERT INTO resource_freshness`).WillReturnError(errors.New("boom"))

	err := repo.Upsert(context.Background(), &models.ResourceFreshness{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upsert resource freshness")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestResourceFreshnessRepository_GetByResource_Found(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)
	since := time.Now().UTC()

	mock.ExpectQuery(`SELECT .* FROM resource_freshness WHERE resource_type = \$1 AND resource_id = \$2`).
		WithArgs("artifact", "res-1").
		WillReturnRows(resourceFreshnessRow("{rule-a,rule-b}", since))

	got, err := repo.GetByResource(context.Background(), "artifact", "res-1")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "fresh-1", got.ID)
	assert.Equal(t, []string{"rule-a", "rule-b"}, got.MatchedRuleIDs)
	assert.Equal(t, models.FreshnessStatusStale, got.Status)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Not being stale is the normal case, so it must be (nil, nil) and not an
// error a caller has to special-case.
func TestResourceFreshnessRepository_GetByResource_NotStaleReturnsNilNil(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectQuery(`FROM resource_freshness`).
		WithArgs("artifact", "res-1").
		WillReturnError(sql.ErrNoRows)

	got, err := repo.GetByResource(context.Background(), "artifact", "res-1")

	require.NoError(t, err)
	assert.Nil(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestResourceFreshnessRepository_GetByResource_Error(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectQuery(`FROM resource_freshness`).WillReturnError(errors.New("boom"))

	_, err := repo.GetByResource(context.Background(), "artifact", "res-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get resource freshness")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestResourceFreshnessRepository_List_TeamOnly(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)
	since := time.Now().UTC()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM resource_freshness WHERE team_id = \$1`).
		WithArgs("team-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectQuery(
		`FROM resource_freshness WHERE team_id = \$1 ORDER BY since DESC, id DESC LIMIT 10 OFFSET 5`,
	).WithArgs("team-1").WillReturnRows(resourceFreshnessRow("{rule-a}", since))

	items, total, err := repo.List(context.Background(), models.ResourceFreshnessFilters{
		TeamID: "team-1", Limit: 10, Offset: 5,
	})

	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, items, 1)
	assert.Equal(t, "fresh-1", items[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Both optional filters must reach the WHERE clause; a dropped predicate would
// leak other projects' or types' rows into the listing.
func TestResourceFreshnessRepository_List_AppliesOptionalFilters(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectQuery(
		`SELECT COUNT\(\*\) FROM resource_freshness WHERE project_id = \$1 AND resource_type = \$2 AND team_id = \$3`,
	).WithArgs("proj-1", "artifact", "team-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(
		`FROM resource_freshness WHERE project_id = \$1 AND resource_type = \$2 AND team_id = \$3`,
	).WithArgs("proj-1", "artifact", "team-1").
		WillReturnRows(sqlmock.NewRows(resourceFreshnessCols()))

	items, total, err := repo.List(context.Background(), models.ResourceFreshnessFilters{
		TeamID: "team-1", ResourceType: "artifact", ProjectID: "proj-1",
	})

	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Equal(t, []*models.ResourceFreshness{}, items, "an empty listing must be [] and never nil")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestResourceFreshnessRepository_List_CountError(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).WillReturnError(errors.New("boom"))

	_, _, err := repo.List(context.Background(), models.ResourceFreshnessFilters{TeamID: "team-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to count resource freshness")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestResourceFreshnessRepository_List_QueryError(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM resource_freshness`).WillReturnError(errors.New("boom"))

	_, _, err := repo.List(context.Background(), models.ResourceFreshnessFilters{TeamID: "team-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list resource freshness")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestResourceFreshnessRepository_List_ScanError(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM resource_freshness`).WillReturnRows(
		sqlmock.NewRows([]string{"id"}).AddRow("only-one-column"),
	)

	_, _, err := repo.List(context.Background(), models.ResourceFreshnessFilters{TeamID: "team-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan resource freshness")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestResourceFreshnessRepository_DeleteByResource(t *testing.T) {
	tests := []struct {
		name        string
		affected    int64
		wantDeleted bool
	}{
		{name: "row removed", affected: 1, wantDeleted: true},
		{name: "resource was not stale", affected: 0, wantDeleted: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := setupResourceFreshnessTest(t)

			mock.ExpectExec(`DELETE FROM resource_freshness WHERE resource_type = \$1 AND resource_id = \$2`).
				WithArgs("artifact", "res-1").
				WillReturnResult(sqlmock.NewResult(0, tt.affected))

			deleted, err := repo.DeleteByResource(context.Background(), "artifact", "res-1")

			require.NoError(t, err)
			assert.Equal(t, tt.wantDeleted, deleted)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestResourceFreshnessRepository_DeleteByResource_Error(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectExec(`DELETE FROM resource_freshness`).WillReturnError(errors.New("boom"))

	_, err := repo.DeleteByResource(context.Background(), "artifact", "res-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete resource freshness")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// RemoveRule is the rule-deletion cleanup: strip the id everywhere, then drop
// the rows that are left stale for no reason. Both statements must run, in one
// transaction, and the delete must be keyed on the ids the update emptied --
// never on `cardinality(...) = 0`, which would also match rows this call never
// touched.
func TestResourceFreshnessRepository_RemoveRule_DeletesOnlyTheRowsItEmptied(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectBegin()
	mock.ExpectQuery(
		// `@>` rather than `= ANY (...)`: only the containment operator is
		// served by the GIN index, and rule deletion scans the whole table.
		`UPDATE resource_freshness SET matched_rule_ids = array_remove\(matched_rule_ids, \$1::uuid\), ` +
			`updated_at = now\(\) WHERE matched_rule_ids @> ARRAY\[\$1::uuid\] ` +
			`RETURNING id, cardinality\(matched_rule_ids\)`,
	).WithArgs("rule-a").WillReturnRows(
		sqlmock.NewRows([]string{"id", "cardinality"}).
			AddRow("orphaned-1", 0).
			AddRow("still-matched", 2).
			AddRow("orphaned-2", 0),
	)
	// Only the two emptied ids are deleted; the row still matching another
	// rule is not named at all.
	mock.ExpectExec(`DELETE FROM resource_freshness WHERE id = ANY\(\$1::uuid\[\]\)`).
		WithArgs(pq.Array([]string{"orphaned-1", "orphaned-2"})).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	deleted, err := repo.RemoveRule(context.Background(), "rule-a")

	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted, "only the rows left matching no rule are deleted")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Nothing emptied means nothing to delete: the DELETE must not run at all,
// since an unfiltered one would reach unrelated rows.
func TestResourceFreshnessRepository_RemoveRule_SkipsDeleteWhenNothingOrphaned(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE resource_freshness`).WithArgs("rule-a").WillReturnRows(
		sqlmock.NewRows([]string{"id", "cardinality"}).AddRow("still-matched", 1),
	)
	mock.ExpectCommit()

	deleted, err := repo.RemoveRule(context.Background(), "rule-a")

	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestResourceFreshnessRepository_RemoveRule_ScanError(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE resource_freshness`).WillReturnRows(
		sqlmock.NewRows([]string{"id"}).AddRow("only-one-column"),
	)
	mock.ExpectRollback()

	_, err := repo.RemoveRule(context.Background(), "rule-a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan stripped resource freshness")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestResourceFreshnessRepository_RemoveRule_BeginError(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectBegin().WillReturnError(errors.New("boom"))

	_, err := repo.RemoveRule(context.Background(), "rule-a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to begin resource freshness rule-removal")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A failure in either statement must roll back, leaving no half-stripped
// state behind.
func TestResourceFreshnessRepository_RemoveRule_RollsBackOnFailure(t *testing.T) {
	tests := []struct {
		name    string
		arrange func(sqlmock.Sqlmock)
		wantMsg string
	}{
		{
			name: "strip fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`UPDATE resource_freshness`).WillReturnError(errors.New("boom"))
			},
			wantMsg: "failed to strip rule from resource freshness",
		},
		{
			name: "orphan delete fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`UPDATE resource_freshness`).WillReturnRows(
					sqlmock.NewRows([]string{"id", "cardinality"}).AddRow("orphaned-1", 0),
				)
				mock.ExpectExec(`DELETE FROM resource_freshness`).WillReturnError(errors.New("boom"))
			},
			wantMsg: "failed to delete unmatched resource freshness",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := setupResourceFreshnessTest(t)

			mock.ExpectBegin()
			tt.arrange(mock)
			mock.ExpectRollback()

			_, err := repo.RemoveRule(context.Background(), "rule-a")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestResourceFreshnessRepository_RemoveRule_CommitError(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE resource_freshness`).WillReturnRows(
		sqlmock.NewRows([]string{"id", "cardinality"}).AddRow("orphaned-1", 0),
	)
	mock.ExpectExec(`DELETE FROM resource_freshness`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(errors.New("boom"))

	_, err := repo.RemoveRule(context.Background(), "rule-a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to commit resource freshness rule-removal")
	assert.NoError(t, mock.ExpectationsWereMet())
}
