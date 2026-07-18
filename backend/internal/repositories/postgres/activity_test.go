package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

func setupActivityRepoTest(t *testing.T) (*activityRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	db := &database.DB{DB: mockDB}
	repo := &activityRepository{db: db}

	return repo, mock, mockDB
}

func TestActivityRepository_DeleteOlderThan_DeletesRows(t *testing.T) {
	t.Parallel()

	cutoff := time.Now().UTC().AddDate(0, 0, -90)
	repo, dbMock, mockDB := setupActivityRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	// Expect advisory lock acquisition
	dbMock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).
		WithArgs(int64(activityRetentionAdvisoryLockID)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	// First batch: 5 rows deleted (< batchSize → last batch)
	dbMock.ExpectExec(`DELETE FROM activities WHERE id IN`).
		WithArgs(cutoff, activityRetentionBatchSize).
		WillReturnResult(sqlmock.NewResult(0, 5))
	// Advisory unlock
	dbMock.ExpectExec(`SELECT pg_advisory_unlock\(\$1\)`).
		WithArgs(int64(activityRetentionAdvisoryLockID)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := repo.DeleteOlderThan(context.Background(), cutoff)

	assert.NoError(t, err)
	assert.Equal(t, int64(5), count)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestActivityRepository_DeleteOlderThan_NoRowsToDelete(t *testing.T) {
	t.Parallel()

	cutoff := time.Now().UTC().AddDate(0, 0, -90)
	repo, dbMock, mockDB := setupActivityRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	dbMock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).
		WithArgs(int64(activityRetentionAdvisoryLockID)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	dbMock.ExpectExec(`DELETE FROM activities WHERE id IN`).
		WithArgs(cutoff, activityRetentionBatchSize).
		WillReturnResult(sqlmock.NewResult(0, 0))
	dbMock.ExpectExec(`SELECT pg_advisory_unlock\(\$1\)`).
		WithArgs(int64(activityRetentionAdvisoryLockID)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := repo.DeleteOlderThan(context.Background(), cutoff)

	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestActivityRepository_DeleteOlderThan_AdvisoryLockHeld(t *testing.T) {
	t.Parallel()

	repo, dbMock, mockDB := setupActivityRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	// Lock is already held by another instance — pg_try_advisory_lock returns false.
	dbMock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).
		WithArgs(int64(activityRetentionAdvisoryLockID)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))

	count, err := repo.DeleteOlderThan(context.Background(), time.Now())

	assert.NoError(t, err) // Skipped cleanly — not an error
	assert.Equal(t, int64(0), count)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestActivityRepository_DeleteOlderThan_DatabaseError(t *testing.T) {
	t.Parallel()

	cutoff := time.Now().UTC().AddDate(0, 0, -90)
	repo, dbMock, mockDB := setupActivityRepoTest(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("Failed to close mock DB: %v", closeErr)
		}
	}()

	dbMock.ExpectQuery(`SELECT pg_try_advisory_lock\(\$1\)`).
		WithArgs(int64(activityRetentionAdvisoryLockID)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	dbMock.ExpectExec(`DELETE FROM activities WHERE id IN`).
		WithArgs(cutoff, activityRetentionBatchSize).
		WillReturnError(sql.ErrConnDone)
	dbMock.ExpectExec(`SELECT pg_advisory_unlock\(\$1\)`).
		WithArgs(int64(activityRetentionAdvisoryLockID)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	count, err := repo.DeleteOlderThan(context.Background(), cutoff)

	assert.Error(t, err)
	assert.Equal(t, int64(0), count)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestActivityRepository_DeleteOlderThan_NilDB(t *testing.T) {
	t.Parallel()

	repo := &activityRepository{db: nil}

	count, err := repo.DeleteOlderThan(context.Background(), time.Now())

	assert.Error(t, err)
	assert.Equal(t, fmt.Errorf("database connection is nil"), err)
	assert.Equal(t, int64(0), count)
}

// activityTestTime is the fixed timestamp used by the activity CRUD tests.
var activityTestTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// activityColumnsTest mirrors the 11-column activity projection.
var activityColumnsTest = []string{
	"id", "user_id", "activity_type", "entity_type", "entity_id", "session_id",
	"description", "metadata", "source_ip", "user_agent", "created_at",
}

// activityRowWithMetadata returns one activity row carrying the given raw
// metadata JSON string.
func activityRowWithMetadata(metadataJSON string) *sqlmock.Rows {
	return sqlmock.NewRows(activityColumnsTest).
		AddRow("act-1", "user-1", "prompt.created", "prompt", nil, nil, "created a prompt",
			metadataJSON, nil, nil, activityTestTime)
}

// activityCountRows returns a single-column COUNT(*) result.
func activityCountRows(n int) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"count"}).AddRow(n)
}

// activityCreateScenario drives Create with an explicit metadata input.
type activityCreateScenario struct {
	name     string
	metadata map[string]interface{}
	setup    func(mock sqlmock.Sqlmock)
	wantSub  string
}

func runActivityCreateScenario(t *testing.T, sc activityCreateScenario) {
	repo, mock, mockDB := setupActivityRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	activity := &models.Activity{
		ID: "act-1", UserID: "user-1", ActivityType: "prompt.created",
		EntityType: "prompt", Description: "created a prompt", Metadata: sc.metadata,
	}
	err := repo.Create(context.Background(), activity)

	assertWantRepoErr(t, err, nil, sc.wantSub)
	if err == nil {
		assert.Equal(t, activityTestTime, activity.CreatedAt)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestActivityRepository_Create(t *testing.T) {
	expectInsert := func(mock sqlmock.Sqlmock, metadataJSON string) *sqlmock.ExpectedQuery {
		return mock.ExpectQuery(`INSERT INTO activities`).
			WithArgs("act-1", "user-1", "prompt.created", "prompt", nil, nil,
				"created a prompt", metadataJSON, nil, nil)
	}

	scenarios := []activityCreateScenario{
		{
			name: "nil metadata defaults to the empty JSON object",
			setup: func(mock sqlmock.Sqlmock) {
				expectInsert(mock, "{}").WillReturnRows(
					sqlmock.NewRows([]string{"created_at"}).AddRow(activityTestTime))
			},
		},
		{
			name:     "metadata is marshaled to JSON",
			metadata: map[string]interface{}{"source": "api"},
			setup: func(mock sqlmock.Sqlmock) {
				expectInsert(mock, `{"source":"api"}`).WillReturnRows(
					sqlmock.NewRows([]string{"created_at"}).AddRow(activityTestTime))
			},
		},
		{
			name: "driver error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				expectInsert(mock, "{}").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to create activity",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runActivityCreateScenario(t, sc) })
	}
}

// activityGetScenario drives GetByID and pins the metadata parse contract.
type activityGetScenario struct {
	name         string
	setup        func(mock sqlmock.Sqlmock)
	wantIs       error
	wantSub      string
	wantMetadata map[string]interface{}
}

func runActivityGetByIDScenario(t *testing.T, sc activityGetScenario) {
	repo, mock, mockDB := setupActivityRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	got, err := repo.GetByID(context.Background(), "user-1", "act-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	if err == nil {
		assert.Equal(t, "act-1", got.ID)
		assert.Equal(t, "user-1", got.UserID)
		assert.Equal(t, sc.wantMetadata, got.Metadata)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestActivityRepository_GetByID(t *testing.T) {
	const getRE = `SELECT .+ FROM activities WHERE id = \$1 AND user_id = \$2`

	scenarios := []activityGetScenario{
		{
			name: "found parses the metadata JSON",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getRE).WithArgs("act-1", "user-1").
					WillReturnRows(activityRowWithMetadata(`{"source":"api"}`))
			},
			wantMetadata: map[string]interface{}{"source": "api"},
		},
		{
			name: "corrupt metadata degrades to nil, best-effort",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getRE).WithArgs("act-1", "user-1").
					WillReturnRows(activityRowWithMetadata(`{not-json`))
			},
		},
		{
			name: "empty metadata string stays nil",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getRE).WithArgs("act-1", "user-1").
					WillReturnRows(activityRowWithMetadata(""))
			},
		},
		{
			name: "no rows maps to the activity not-found sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getRE).WithArgs("act-1", "user-1").WillReturnError(sql.ErrNoRows)
			},
			wantIs: repositories.ErrActivityNotFound,
		},
		{
			name: "driver error is wrapped, not the sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(getRE).WithArgs("act-1", "user-1").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to get activity",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runActivityGetByIDScenario(t, sc) })
	}
}

// activityListFilterScenario pins one buildActivityListConditions predicate:
// the WHERE fragment both queries share and the args it binds.
type activityListFilterScenario struct {
	name    string
	filters repositories.ActivityFilters
	whereRE string
	args    []driver.Value
}

func runActivityListFilterScenario(t *testing.T, sc activityListFilterScenario) {
	repo, mock, mockDB := setupActivityRepoTest(t)
	registerMockDBClose(t, mockDB)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM activities` + sc.whereRE).
		WithArgs(sc.args...).
		WillReturnRows(activityCountRows(7))

	rows := activityRowWithMetadata(`{"source":"api"}`).
		AddRow("act-2", "user-1", "memory.created", "memory", nil, nil, "created a memory",
			`{corrupt`, nil, nil, activityTestTime)
	mock.ExpectQuery(`SELECT .+ FROM activities` + sc.whereRE + ` ORDER BY created_at DESC LIMIT 10 OFFSET 0`).
		WithArgs(sc.args...).
		WillReturnRows(rows)

	resp, err := repo.List(context.Background(), sc.filters)

	require.NoError(t, err)
	assert.Equal(t, 7, resp.TotalCount)
	require.Len(t, resp.Activities, 2)
	assert.Equal(t, map[string]interface{}{"source": "api"}, resp.Activities[0].Metadata)
	assert.Nil(t, resp.Activities[1].Metadata, "corrupt metadata must degrade to nil, not error")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestActivityRepository_List_Filters(t *testing.T) {
	withLimit := func(f repositories.ActivityFilters) repositories.ActivityFilters {
		f.Limit = 10
		return f
	}

	scenarios := []activityListFilterScenario{
		{
			name:    "no filters emits no WHERE clause",
			filters: withLimit(repositories.ActivityFilters{}),
			whereRE: ``,
			args:    nil,
		},
		{
			name:    "user_id filter",
			filters: withLimit(repositories.ActivityFilters{UserID: strPtr("user-1")}),
			whereRE: ` WHERE \(user_id = \$1\)`,
			args:    []driver.Value{"user-1"},
		},
		{
			name:    "activity_type filter",
			filters: withLimit(repositories.ActivityFilters{ActivityType: strPtr("prompt.created")}),
			whereRE: ` WHERE \(activity_type = \$1\)`,
			args:    []driver.Value{"prompt.created"},
		},
		{
			name:    "entity_type filter",
			filters: withLimit(repositories.ActivityFilters{EntityType: strPtr("prompt")}),
			whereRE: ` WHERE \(entity_type = \$1\)`,
			args:    []driver.Value{"prompt"},
		},
		{
			name:    "entity_id filter",
			filters: withLimit(repositories.ActivityFilters{EntityID: strPtr("ent-1")}),
			whereRE: ` WHERE \(entity_id = \$1\)`,
			args:    []driver.Value{"ent-1"},
		},
		{
			name:    "session_id filter",
			filters: withLimit(repositories.ActivityFilters{SessionID: strPtr("sess-1")}),
			whereRE: ` WHERE \(session_id = \$1\)`,
			args:    []driver.Value{"sess-1"},
		},
		{
			name:    "search matches description OR activity_type via ILIKE",
			filters: withLimit(repositories.ActivityFilters{Search: strPtr("deploy")}),
			whereRE: ` WHERE \(\(description ILIKE \$1 OR activity_type ILIKE \$2\)\)`,
			args:    []driver.Value{"%deploy%", "%deploy%"},
		},
		{
			name:    "empty search is treated as no filter",
			filters: withLimit(repositories.ActivityFilters{Search: strPtr("")}),
			whereRE: ``,
			args:    nil,
		},
		{
			name:    "date_from filter",
			filters: withLimit(repositories.ActivityFilters{DateFrom: strPtr("2026-01-01")}),
			whereRE: ` WHERE \(created_at >= \$1\)`,
			args:    []driver.Value{"2026-01-01"},
		},
		{
			name:    "date_to filter",
			filters: withLimit(repositories.ActivityFilters{DateTo: strPtr("2026-02-01")}),
			whereRE: ` WHERE \(created_at <= \$1\)`,
			args:    []driver.Value{"2026-02-01"},
		},
		{
			name: "all filters combine in declaration order",
			filters: withLimit(repositories.ActivityFilters{
				UserID:       strPtr("user-1"),
				ActivityType: strPtr("prompt.created"),
				EntityType:   strPtr("prompt"),
				EntityID:     strPtr("ent-1"),
				SessionID:    strPtr("sess-1"),
				Search:       strPtr("deploy"),
				DateFrom:     strPtr("2026-01-01"),
				DateTo:       strPtr("2026-02-01"),
			}),
			whereRE: ` WHERE \(user_id = \$1 AND .+ AND created_at <= \$9\)`,
			args: []driver.Value{
				"user-1", "prompt.created", "prompt", "ent-1", "sess-1",
				"%deploy%", "%deploy%", "2026-01-01", "2026-02-01",
			},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runActivityListFilterScenario(t, sc) })
	}
}

// activityListPagingScenario pins the List pagination math: the rendered
// LIMIT/OFFSET and the derived page/per-page/total-pages response fields.
type activityListPagingScenario struct {
	name           string
	filters        repositories.ActivityFilters
	limitRE        string
	total          int
	wantPage       int
	wantPerPage    int
	wantTotalPages int
}

func runActivityListPagingScenario(t *testing.T, sc activityListPagingScenario) {
	repo, mock, mockDB := setupActivityRepoTest(t)
	registerMockDBClose(t, mockDB)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM activities`).
		WillReturnRows(activityCountRows(sc.total))
	mock.ExpectQuery(`SELECT .+ FROM activities ORDER BY created_at DESC ` + sc.limitRE).
		WillReturnRows(sqlmock.NewRows(activityColumnsTest))

	resp, err := repo.List(context.Background(), sc.filters)

	require.NoError(t, err)
	assert.Equal(t, sc.wantPage, resp.Page)
	assert.Equal(t, sc.wantPerPage, resp.PerPage)
	assert.Equal(t, sc.wantTotalPages, resp.TotalPages)
	assert.NotNil(t, resp.Activities)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestActivityRepository_List_PaginationMath(t *testing.T) {
	scenarios := []activityListPagingScenario{
		{
			name:    "limit and offset pass through and derive page numbers",
			filters: repositories.ActivityFilters{Limit: 10, Offset: 20},
			limitRE: `LIMIT 10 OFFSET 20`,
			total:   25, wantPage: 3, wantPerPage: 10, wantTotalPages: 3,
		},
		{
			name:    "zero limit guards the division with per_page 1",
			filters: repositories.ActivityFilters{},
			limitRE: `LIMIT 0 OFFSET 0`,
			total:   5, wantPage: 1, wantPerPage: 1, wantTotalPages: 5,
		},
		{
			// Negative paging inputs clamp to LIMIT 0 OFFSET 0 in SQL; the
			// response Page still reflects the raw offset arithmetic.
			name:    "negative limit and offset clamp to zero in SQL",
			filters: repositories.ActivityFilters{Limit: -3, Offset: -7},
			limitRE: `LIMIT 0 OFFSET 0`,
			total:   4, wantPage: -6, wantPerPage: 1, wantTotalPages: 4,
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runActivityListPagingScenario(t, sc) })
	}
}

// TestActivityRepository_List_SkipsUnscannableRows pins the scan-skip
// contract: a row that fails to scan is logged and skipped, never an error.
// (The count/page query error paths live in followups_list_squirrel_test.go.)
func TestActivityRepository_List_SkipsUnscannableRows(t *testing.T) {
	repo, mock, mockDB := setupActivityRepoTest(t)
	registerMockDBClose(t, mockDB)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM activities`).WillReturnRows(activityCountRows(3))
	// One fewer column than the scan expects: the row is logged and skipped.
	mock.ExpectQuery(`SELECT .+ FROM activities`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("act-1"))

	resp, err := repo.List(context.Background(), repositories.ActivityFilters{Limit: 10})

	require.NoError(t, err)
	assert.Equal(t, 3, resp.TotalCount)
	require.NotNil(t, resp.Activities)
	assert.Empty(t, resp.Activities)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// activityStatsScenario drives GetStats and verifies the assembled payload.
type activityStatsScenario struct {
	name    string
	setup   func(mock sqlmock.Sqlmock)
	wantErr bool
	verify  func(t *testing.T, stats *models.ActivityStatsResponse)
}

func runActivityStatsScenario(t *testing.T, sc activityStatsScenario) {
	repo, mock, mockDB := setupActivityRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	stats, err := repo.GetStats(context.Background(), "user-1")

	if sc.wantErr {
		require.Error(t, err)
	} else {
		require.NoError(t, err)
		sc.verify(t, stats)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

// The seven GetStats queries, in execution order, distinguished by their
// characteristic fragments.
const (
	statsTotalRE      = `SELECT COUNT\(\*\) FROM activities WHERE user_id = \$1`
	statsTodayRE      = `DATE\(created_at\) = CURRENT_DATE`
	statsWeekRE       = `DATE_TRUNC\('week', CURRENT_DATE\)`
	statsTopActRE     = `GROUP BY activity_type`
	statsTopEntityRE  = `GROUP BY entity_type`
	statsRecentRE     = `ORDER BY created_at DESC LIMIT 10`
	statsByDateWeekRE = `GROUP BY DATE\(created_at\)`
)

func TestActivityRepository_GetStats(t *testing.T) {
	scenarios := []activityStatsScenario{
		{
			name: "assembles totals and breakdowns",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(statsTotalRE).WithArgs("user-1").WillReturnRows(activityCountRows(42))
				mock.ExpectQuery(statsTodayRE).WithArgs("user-1").WillReturnRows(activityCountRows(5))
				mock.ExpectQuery(statsWeekRE).WithArgs("user-1").WillReturnRows(activityCountRows(12))
				mock.ExpectQuery(statsTopActRE).WithArgs("user-1").WillReturnRows(
					sqlmock.NewRows([]string{"activity_type", "count"}).AddRow("prompt.created", 9))
				mock.ExpectQuery(statsTopEntityRE).WithArgs("user-1").WillReturnRows(
					sqlmock.NewRows([]string{"entity_type", "count"}).AddRow("prompt", 7))
				mock.ExpectQuery(statsRecentRE).WithArgs("user-1").
					WillReturnRows(activityRowWithMetadata(`{"source":"api"}`))
				mock.ExpectQuery(statsByDateWeekRE).WithArgs("user-1").WillReturnRows(
					sqlmock.NewRows([]string{"date", "count"}).AddRow("2026-01-02", 3))
			},
			verify: func(t *testing.T, stats *models.ActivityStatsResponse) {
				assert.Equal(t, 42, stats.TotalActivities)
				assert.Equal(t, 5, stats.ActivitiesToday)
				assert.Equal(t, 12, stats.ActivitiesThisWeek)
				assert.Equal(t, []models.ActivityTypeCount{{ActivityType: "prompt.created", Count: 9}},
					stats.TopActivityTypes)
				assert.Equal(t, []models.EntityTypeCount{{EntityType: "prompt", Count: 7}},
					stats.TopEntityTypes)
				require.Len(t, stats.RecentActivities, 1)
				assert.Equal(t, map[string]interface{}{"source": "api"}, stats.RecentActivities[0].Metadata)
				assert.Equal(t, []models.ActivityCountByDate{{Date: "2026-01-02", Count: 3}},
					stats.ActivitiesByDateWeek)
			},
		},
		{
			// The degrade contract: only the total is load-bearing; every other
			// query is best-effort and failure degrades to zero / empty non-nil.
			name: "best-effort queries degrade to zero and empty, never error",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(statsTotalRE).WithArgs("user-1").WillReturnRows(activityCountRows(42))
				for _, re := range []string{
					statsTodayRE, statsWeekRE, statsTopActRE,
					statsTopEntityRE, statsRecentRE, statsByDateWeekRE,
				} {
					mock.ExpectQuery(re).WithArgs("user-1").WillReturnError(sql.ErrConnDone)
				}
			},
			verify: func(t *testing.T, stats *models.ActivityStatsResponse) {
				assert.Equal(t, 42, stats.TotalActivities)
				assert.Zero(t, stats.ActivitiesToday)
				assert.Zero(t, stats.ActivitiesThisWeek)
				require.NotNil(t, stats.TopActivityTypes)
				assert.Empty(t, stats.TopActivityTypes)
				require.NotNil(t, stats.TopEntityTypes)
				assert.Empty(t, stats.TopEntityTypes)
				require.NotNil(t, stats.RecentActivities)
				assert.Empty(t, stats.RecentActivities)
				require.NotNil(t, stats.ActivitiesByDateWeek)
				assert.Empty(t, stats.ActivitiesByDateWeek)
			},
		},
		{
			name: "total count failure is the one fatal error",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(statsTotalRE).WithArgs("user-1").WillReturnError(sql.ErrConnDone)
			},
			wantErr: true,
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runActivityStatsScenario(t, sc) })
	}
}

// activityDeleteScenario drives Delete.
type activityDeleteScenario struct {
	name    string
	setup   func(mock sqlmock.Sqlmock)
	wantIs  error
	wantSub string
}

func runActivityDeleteScenario(t *testing.T, sc activityDeleteScenario) {
	repo, mock, mockDB := setupActivityRepoTest(t)
	registerMockDBClose(t, mockDB)
	sc.setup(mock)

	err := repo.Delete(context.Background(), "act-1")

	assertWantRepoErr(t, err, sc.wantIs, sc.wantSub)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestActivityRepository_Delete(t *testing.T) {
	const deleteRE = `DELETE FROM activities WHERE id = \$1`

	scenarios := []activityDeleteScenario{
		{
			name: "deletes the activity",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deleteRE).WithArgs("act-1").WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
		{
			name: "zero rows affected maps to the not-found sentinel",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deleteRE).WithArgs("act-1").WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantIs: repositories.ErrActivityNotFound,
		},
		{
			name: "exec error is wrapped",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(deleteRE).WithArgs("act-1").WillReturnError(sql.ErrConnDone)
			},
			wantSub: "failed to delete activity",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { runActivityDeleteScenario(t, sc) })
	}
}

// TestActivityRepository_NilDBGuards pins the nil-connection guard on every
// method that carries one.
func TestActivityRepository_NilDBGuards(t *testing.T) {
	repo := NewActivityRepository(nil)
	ctx := context.Background()

	assert.ErrorContains(t, repo.Create(ctx, &models.Activity{}), "database connection is nil")

	_, err := repo.GetByID(ctx, "user-1", "act-1")
	assert.ErrorContains(t, err, "database connection is nil")

	_, err = repo.List(ctx, repositories.ActivityFilters{})
	assert.ErrorContains(t, err, "database connection is nil")

	_, err = repo.GetStats(ctx, "user-1")
	assert.ErrorContains(t, err, "database connection is nil")

	assert.ErrorContains(t, repo.Delete(ctx, "act-1"), "database connection is nil")
}
