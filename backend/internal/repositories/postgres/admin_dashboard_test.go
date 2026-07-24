package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dashFrom = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	dashTo   = time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
)

func TestAdminRepository_GetExtendedCounts(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM api_keys`).WillReturnRows(
		sqlmock.NewRows([]string{
			"users", "teams", "projects", "prompts", "artifacts",
			"memories", "blueprints", "agents", "feeds", "api_keys",
		}).AddRow(1, 2, 3, 4, 5, 6, 7, 8, 9, 10),
	)

	got, err := repo.GetExtendedCounts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), got.Users)
	assert.Equal(t, int64(6), got.Memories)
	assert.Equal(t, int64(10), got.APIKeys)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetExtendedCounts_Error(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM api_keys`).WillReturnError(errors.New("boom"))
	_, err := repo.GetExtendedCounts(context.Background())
	require.Error(t, err)
}

// TestAdminRepository_GetEntityBreakdowns checks the run-length assembly: rows
// arrive flat and grouped by (entity, field) and must fold into one breakdown
// per pair without merging two different pairs.
func TestAdminRepository_GetEntityBreakdowns(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM prompts GROUP BY status`).WillReturnRows(
		sqlmock.NewRows([]string{"entity", "field", "value", "count"}).
			AddRow("artifacts", "status", "active", 10).
			AddRow("artifacts", "status", "archived", 2).
			AddRow("artifacts", "type", "general", 12).
			AddRow("prompts", "status", "draft", 5),
	)

	got, err := repo.GetEntityBreakdowns(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 3, "artifacts/status, artifacts/type and prompts/status are three breakdowns")

	assert.Equal(t, "artifacts", got[0].Entity)
	assert.Equal(t, "status", got[0].Field)
	require.Len(t, got[0].Buckets, 2)
	assert.Equal(t, "active", got[0].Buckets[0].Value)
	assert.Equal(t, int64(10), got[0].Buckets[0].Count)

	assert.Equal(t, "type", got[1].Field)
	require.Len(t, got[1].Buckets, 1)

	assert.Equal(t, "prompts", got[2].Entity)
	require.Len(t, got[2].Buckets, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetEntityBreakdowns_Error(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`FROM prompts GROUP BY status`).WillReturnError(errors.New("boom"))
	_, err := repo.GetEntityBreakdowns(context.Background())
	require.Error(t, err)
}

func TestAdminRepository_GetSystemHealth(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	mock.ExpectQuery(`pg_database_size`).
		WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(4096))
	mock.ExpectQuery(`FROM pg_stat_user_tables`).
		WillReturnRows(sqlmock.NewRows([]string{"relname", "n_live_tup"}).
			AddRow("prompts", 340).
			AddRow("users", 42))

	got, err := repo.GetSystemHealth(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(4096), got.DatabaseSizeBytes)
	require.Len(t, got.Tables, 2)
	assert.Equal(t, "prompts", got.Tables[0].Table)
	assert.Equal(t, int64(340), got.Tables[0].EstimatedRows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetSystemHealth_Errors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(sqlmock.Sqlmock)
	}{
		{
			name: "database size fails",
			setup: func(m sqlmock.Sqlmock) {
				m.ExpectQuery(`pg_database_size`).WillReturnError(errors.New("boom"))
			},
		},
		{
			name: "table stats fail",
			setup: func(m sqlmock.Sqlmock) {
				m.ExpectQuery(`pg_database_size`).
					WillReturnRows(sqlmock.NewRows([]string{"size"}).AddRow(1))
				m.ExpectQuery(`FROM pg_stat_user_tables`).WillReturnError(errors.New("boom"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, mockDB := newAdminRepoMock(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("failed to close mock DB: %v", closeErr)
				}
			}()

			tc.setup(mock)
			_, err := repo.GetSystemHealth(context.Background())
			require.Error(t, err)
		})
	}
}

// TestAdminGrowthQuery_NormalizesBothTimestampFamilies is the guard for this
// issue's subtlest bug class. `memories` stores `timestamp without time zone`
// while every other growth table stores `timestamp with time zone`, and
// `AT TIME ZONE 'UTC'` does OPPOSITE things to the two. The generated SQL must
// therefore convert the aware branches and leave the naive one alone, or the
// two families silently disagree by the server's UTC offset.
func TestAdminGrowthQuery_NormalizesBothTimestampFamilies(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	// Aware branch: converted for bucketing, raw column in the predicate.
	mock.ExpectQuery(
		`date_trunc\('day', created_at AT TIME ZONE 'UTC'\) AS bucket FROM users `+
			`WHERE created_at >= \$1 AND created_at < \$2`,
	).
		WithArgs(dashFrom, dashTo).
		WillReturnRows(sqlmock.NewRows([]string{"entity", "bucket", "count"}).
			AddRow("users", dashFrom, 3))

	_, err := repo.GetGrowthSeries(context.Background(), dashFrom, dashTo, "day")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	// Naive branch: NOT converted, and bounded with an explicit ::timestamp cast
	// so the comparison is independent of the session timezone.
	repo2, mock2, mockDB2 := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB2.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()
	mock2.ExpectQuery(
		`SELECT 'memories', date_trunc\('day', created_at\) FROM memories `+
			`WHERE created_at >= \$1::timestamp AND created_at < \$2::timestamp`,
	).
		WithArgs(dashFrom, dashTo).
		WillReturnRows(sqlmock.NewRows([]string{"entity", "bucket", "count"}))

	_, err = repo2.GetGrowthSeries(context.Background(), dashFrom, dashTo, "day")
	require.NoError(t, err)
	require.NoError(t, mock2.ExpectationsWereMet())
}

// TestAdminSeriesQueries_UseAllowlistedTruncUnit proves the granularity reaches
// date_trunc only as one of three constants — an injection-shaped value falls
// back to 'day' rather than reaching the SQL text.
func TestAdminSeriesQueries_UseAllowlistedTruncUnit(t *testing.T) {
	tests := []struct {
		name        string
		granularity string
		wantUnit    string
	}{
		{"day", "day", "day"},
		{"week", "week", "week"},
		{"month", "month", "month"},
		{"unknown falls back", "fortnight", "day"},
		{"injection-shaped falls back", "day') OR 1=1--", "day"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantUnit, adminTruncUnit(tc.granularity))
			assert.NotContains(t, adminTruncUnit(tc.granularity), "OR 1=1")
		})
	}
}

func TestAdminRepository_GetSignInSeries(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	// activities.created_at is the naive family: bucketed as-written, bounded
	// with ::timestamp, and filtered by the auth_login activity type.
	mock.ExpectQuery(
		`date_trunc\('week', created_at\) AS bucket, COUNT\(\*\) AS count `+
			`FROM activities WHERE activity_type = \$1 `+
			`AND created_at >= \$2::timestamp AND created_at < \$3::timestamp`,
	).
		WithArgs("auth_login", dashFrom, dashTo).
		WillReturnRows(sqlmock.NewRows([]string{"bucket", "count"}).AddRow(dashFrom, 7))

	got, err := repo.GetSignInSeries(context.Background(), dashFrom, dashTo, "week")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, int64(7), got[0].Count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_GetAccessBySourceSeries(t *testing.T) {
	repo, mock, mockDB := newAdminRepoMock(t)
	defer func() {
		if closeErr := mockDB.Close(); closeErr != nil {
			t.Logf("failed to close mock DB: %v", closeErr)
		}
	}()

	// resource_access_events.created_at is the aware family: converted for
	// bucketing, raw column in the predicate so the index stays usable.
	mock.ExpectQuery(
		`date_trunc\('day', created_at AT TIME ZONE 'UTC'\) AS bucket, source, COUNT\(\*\) AS count `+
			`FROM resource_access_events WHERE created_at >= \$1 AND created_at < \$2`,
	).
		WithArgs(dashFrom, dashTo).
		WillReturnRows(sqlmock.NewRows([]string{"bucket", "source", "count"}).
			AddRow(dashFrom, "mcp", 4).
			AddRow(dashFrom, "web", 11))

	got, err := repo.GetAccessBySourceSeries(context.Background(), dashFrom, dashTo, "day")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "mcp", got[0].Source)
	assert.Equal(t, int64(11), got[1].Count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminRepository_SeriesQueryErrors(t *testing.T) {
	tests := []struct {
		name  string
		match string
		call  func(*AdminRepository) error
	}{
		{
			name:  "growth",
			match: `FROM users WHERE`,
			call: func(r *AdminRepository) error {
				_, err := r.GetGrowthSeries(context.Background(), dashFrom, dashTo, "day")
				return err
			},
		},
		{
			name:  "sign-ins",
			match: `FROM activities`,
			call: func(r *AdminRepository) error {
				_, err := r.GetSignInSeries(context.Background(), dashFrom, dashTo, "day")
				return err
			},
		},
		{
			name:  "access by source",
			match: `FROM resource_access_events`,
			call: func(r *AdminRepository) error {
				_, err := r.GetAccessBySourceSeries(context.Background(), dashFrom, dashTo, "day")
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, mockDB := newAdminRepoMock(t)
			defer func() {
				if closeErr := mockDB.Close(); closeErr != nil {
					t.Logf("failed to close mock DB: %v", closeErr)
				}
			}()

			mock.ExpectQuery(tc.match).WillReturnError(errors.New("boom"))
			require.Error(t, tc.call(repo))
		})
	}
}
