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

// upsertCols is the projection Upsert returns: the shared columns plus the
// synthetic `inserted` flag it appends (#771).
func upsertCols() []string {
	return append(resourceFreshnessCols(), "inserted")
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

// upsertRow is the same row plus the `inserted` flag, as Upsert's RETURNING
// clause projects it.
func upsertRow(ruleIDs string, since time.Time, inserted bool) *sqlmock.Rows {
	return sqlmock.NewRows(upsertCols()).AddRow(
		"fresh-1", "team-1", "proj-1", "artifact", "res-1",
		models.FreshnessStatusStale, []byte(ruleIDs), since,
		models.FreshnessReasonRuleRun, since, since, inserted,
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
		WillReturnRows(upsertRow("{rule-a}", since, true))

	f := &models.ResourceFreshness{
		TeamID: "team-1", ProjectID: "proj-1",
		ResourceType: "artifact", ResourceID: "res-1",
		Status: models.FreshnessStatusStale, MatchedRuleIDs: []string{"rule-a"},
		Since: since, Reason: models.FreshnessReasonRuleRun,
	}
	inserted, err := repo.Upsert(context.Background(), f)
	require.NoError(t, err)
	assert.True(t, inserted)

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
		WillReturnRows(upsertRow("{}", dbSince, true))

	f := &models.ResourceFreshness{
		TeamID: "team-1", ProjectID: "proj-1",
		ResourceType: "prompt", ResourceID: "res-2",
		Status: models.FreshnessStatusStale, MatchedRuleIDs: []string{},
		Reason: models.FreshnessReasonRuleRun,
	}
	_, err := repo.Upsert(context.Background(), f)
	require.NoError(t, err)

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
	).WillReturnRows(upsertRow("{}", since, false))

	_, err := repo.Upsert(context.Background(), &models.ResourceFreshness{Since: since})
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The flag is what tells a genuine stale-again transition from bookkeeping on a
// surviving row (#771), so the projection must actually ask for it — a caller
// cannot recover it from anything else in the result.
func TestResourceFreshnessRepository_Upsert_SelectsAndScansTheInsertedFlag(t *testing.T) {
	for _, inserted := range []bool{true, false} {
		name := "conflict update reports false"
		if inserted {
			name = "insert reports true"
		}
		t.Run(name, func(t *testing.T) {
			repo, mock := setupResourceFreshnessTest(t)
			since := time.Now().UTC()

			mock.ExpectQuery(`RETURNING .*, \(xmax = 0\) AS inserted`).
				WillReturnRows(upsertRow("{}", since, inserted))

			got, err := repo.Upsert(context.Background(), &models.ResourceFreshness{Since: since})

			require.NoError(t, err)
			assert.Equal(t, inserted, got)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestResourceFreshnessRepository_Upsert_Error(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectQuery(`INSERT INTO resource_freshness`).WillReturnError(errors.New("boom"))

	inserted, err := repo.Upsert(context.Background(), &models.ResourceFreshness{})
	require.Error(t, err)
	assert.False(t, inserted, "a failed upsert must not claim it inserted")
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

// Rule evaluation reconciles a team's WHOLE stale set, so this listing must be
// unpaginated and scoped by team alone.
func TestResourceFreshnessRepository_ListAllByTeam_ReturnsEveryRow(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)
	since := time.Now().UTC().Truncate(time.Second)

	mock.ExpectQuery(`SELECT .+ FROM resource_freshness WHERE team_id = \$1$`).
		WithArgs("team-1").
		WillReturnRows(resourceFreshnessRow("{rule-a,rule-b}", since))

	got, err := repo.ListAllByTeam(context.Background(), "team-1")

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, []string{"rule-a", "rule-b"}, got[0].MatchedRuleIDs)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// A team with nothing stale returns an empty slice, not nil: the caller ranges
// over it to decide what to clear.
func TestResourceFreshnessRepository_ListAllByTeam_EmptyIsNotNil(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	mock.ExpectQuery(`FROM resource_freshness`).
		WillReturnRows(sqlmock.NewRows(resourceFreshnessCols()))

	got, err := repo.ListAllByTeam(context.Background(), "team-1")

	require.NoError(t, err)
	assert.Equal(t, []*models.ResourceFreshness{}, got)
}

func TestResourceFreshnessRepository_ListAllByTeam_Errors(t *testing.T) {
	tests := []struct {
		name    string
		arrange func(mock sqlmock.Sqlmock)
		wantIn  string
	}{
		{
			name: "query fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM resource_freshness`).WillReturnError(errors.New("boom"))
			},
			wantIn: "failed to list team resource freshness",
		},
		{
			name: "scan fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM resource_freshness`).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("fresh-1"))
			},
			wantIn: "failed to scan team resource freshness",
		},
		{
			name: "iteration fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM resource_freshness`).
					WillReturnRows(resourceFreshnessRow("{rule-a}", time.Now().UTC()).
						RowError(0, errors.New("stream broken")))
			},
			wantIn: "failed to iterate team resource freshness",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := setupResourceFreshnessTest(t)
			tt.arrange(mock)

			_, err := repo.ListAllByTeam(context.Background(), "team-1")

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantIn)
		})
	}
}

// The grouped counts back the analytics charts (#734). sqlmock can only pin
// the query shape; the semantics (what unnest does to a multi-rule resource,
// whether the totals agree) are asserted against real Postgres.
func TestResourceFreshnessRepository_GroupedCounts(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		call    func(repo *ResourceFreshnessRepository) ([]models.FreshnessBucketCount, error)
	}{
		{
			name:    "by type",
			pattern: `SELECT resource_type, COUNT\(\*\).+FROM resource_freshness.+GROUP BY resource_type`,
			call: func(repo *ResourceFreshnessRepository) ([]models.FreshnessBucketCount, error) {
				return repo.CountStaleByType(context.Background(), "team-1")
			},
		},
		{
			name:    "by project",
			pattern: `SELECT project_id::text, COUNT\(\*\).+FROM resource_freshness.+GROUP BY project_id`,
			call: func(repo *ResourceFreshnessRepository) ([]models.FreshnessBucketCount, error) {
				return repo.CountStaleByProject(context.Background(), "team-1")
			},
		},
		{
			// unnest is what makes the union semantics visible: one resource
			// matched by two rules contributes a row to each.
			name:    "by rule",
			pattern: `FROM resource_freshness, unnest\(matched_rule_ids\) AS rule_id.+GROUP BY rule_id`,
			call: func(repo *ResourceFreshnessRepository) ([]models.FreshnessBucketCount, error) {
				return repo.CountStaleByRule(context.Background(), "team-1")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := setupResourceFreshnessTest(t)
			mock.ExpectQuery(tt.pattern).
				WithArgs("team-1").
				WillReturnRows(sqlmock.NewRows([]string{"key", "count"}).
					AddRow("alpha", 3).
					AddRow("beta", 1))

			got, err := tt.call(repo)

			require.NoError(t, err)
			assert.Equal(t, []models.FreshnessBucketCount{{Key: "alpha", Count: 3}, {Key: "beta", Count: 1}}, got)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// A team with nothing stale gets an empty slice, not nil: the service ranges
// over it to zero-fill.
func TestResourceFreshnessRepository_GroupedCounts_EmptyIsNotNil(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)
	mock.ExpectQuery(`GROUP BY resource_type`).
		WillReturnRows(sqlmock.NewRows([]string{"key", "count"}))

	got, err := repo.CountStaleByType(context.Background(), "team-1")

	require.NoError(t, err)
	assert.Equal(t, []models.FreshnessBucketCount{}, got)
}

func TestResourceFreshnessRepository_GroupedCounts_Errors(t *testing.T) {
	tests := []struct {
		name    string
		arrange func(mock sqlmock.Sqlmock)
		wantIn  string
	}{
		{
			name: "query fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`GROUP BY resource_type`).WillReturnError(errors.New("boom"))
			},
			wantIn: "failed to count stale resources by type",
		},
		{
			name: "scan fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`GROUP BY resource_type`).
					WillReturnRows(sqlmock.NewRows([]string{"key"}).AddRow("artifact"))
			},
			wantIn: "failed to scan stale count by type",
		},
		{
			name: "iteration fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`GROUP BY resource_type`).
					WillReturnRows(sqlmock.NewRows([]string{"key", "count"}).
						AddRow("artifact", 1).RowError(0, errors.New("stream broken")))
			},
			wantIn: "failed to iterate stale counts by type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := setupResourceFreshnessTest(t)
			tt.arrange(mock)

			_, err := repo.CountStaleByType(context.Background(), "team-1")

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantIn)
		})
	}
}

func TestResourceFreshnessRepository_CountStale(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM resource_freshness WHERE team_id = \$1`).
		WithArgs("team-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(9))

	total, err := repo.CountStale(context.Background(), "team-1")

	require.NoError(t, err)
	assert.Equal(t, 9, total)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestResourceFreshnessRepository_CountStale_Error(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)
	mock.ExpectQuery(`SELECT COUNT`).WillReturnError(errors.New("boom"))

	_, err := repo.CountStale(context.Background(), "team-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to count stale resources")
}

// ListByResources backs the per-page freshness attach on every resource list,
// so its query shape and its empty-page short-circuit both matter.
func TestResourceFreshnessRepository_ListByResources(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)
	since := time.Now().UTC().Truncate(time.Second)

	mock.ExpectQuery(`FROM resource_freshness\s+WHERE resource_type = \$1 AND resource_id = ANY\(\$2::uuid\[\]\)`).
		WithArgs("artifact", pq.Array([]string{"res-1", "res-2"})).
		WillReturnRows(resourceFreshnessRow("{rule-a}", since))

	got, err := repo.ListByResources(context.Background(), "artifact", []string{"res-1", "res-2"})

	require.NoError(t, err)
	require.Len(t, got, 1, "ids without a row are simply absent — absence is freshness")
	require.Contains(t, got, "res-1")
	assert.Equal(t, []string{"rule-a"}, got["res-1"].MatchedRuleIDs)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// An empty page must not issue a query at all, and must still return a usable
// map so the caller can look ids up unconditionally.
func TestResourceFreshnessRepository_ListByResources_EmptyIssuesNoQuery(t *testing.T) {
	repo, mock := setupResourceFreshnessTest(t)

	got, err := repo.ListByResources(context.Background(), "artifact", nil)

	require.NoError(t, err)
	assert.Equal(t, map[string]*models.ResourceFreshness{}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestResourceFreshnessRepository_ListByResources_Errors(t *testing.T) {
	tests := []struct {
		name    string
		arrange func(mock sqlmock.Sqlmock)
		wantIn  string
	}{
		{
			name: "query fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM resource_freshness`).WillReturnError(errors.New("boom"))
			},
			wantIn: "failed to list freshness for artifact resources",
		},
		{
			name: "scan fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM resource_freshness`).
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("fresh-1"))
			},
			wantIn: "failed to scan freshness for artifact resource",
		},
		{
			name: "iteration fails",
			arrange: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`FROM resource_freshness`).
					WillReturnRows(resourceFreshnessRow("{rule-a}", time.Now().UTC()).
						RowError(0, errors.New("stream broken")))
			},
			wantIn: "failed to iterate freshness for artifact resources",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := setupResourceFreshnessTest(t)
			tt.arrange(mock)

			_, err := repo.ListByResources(context.Background(), "artifact", []string{"res-1"})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantIn)
		})
	}
}
