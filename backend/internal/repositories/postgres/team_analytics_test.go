package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
)

const testAnalyticsTeamID = "team-analytics-1"

// The bucket expressions are the point of these assertions, not incidental
// query text (#773). Each branch is pinned SEPARATELY and anchored from its own
// FROM through its own GROUP BY: an unanchored `FROM prompts .*GROUP BY <utc>`
// would be satisfied by any later branch's converted GROUP BY, so a regression
// of the prompts branch alone would still match and the assertion would be a
// decoration.
//
// Both halves of the aware/naive rule are asserted, because the two failure
// modes are opposite:
//   - a TIMESTAMPTZ branch must convert with `AT TIME ZONE 'UTC'`, or a bare
//     DATE() casts through the session timezone and produces keys the handler's
//     zero-fill never generated, which are then dropped silently;
//   - the memories branch is a plain TIMESTAMP and must NOT convert, since on a
//     naive value the same operator offsets in the other direction -- and its
//     predicate needs its own $3::timestamp, or the shared timestamptz $2
//     converts the column instead.
//
// sqlmock collapses all whitespace to single spaces before matching (its
// stripQuery), so these patterns are written against a one-line query and need
// no (?s).
const (
	awareBucketGroupBy = `GROUP BY \(created_at AT TIME ZONE 'UTC'\)::date`

	teamResourceCreationBucketRE = `FROM prompts WHERE team_id = \$1 AND created_at >= \$2 ` + awareBucketGroupBy +
		`.*FROM artifacts WHERE team_id = \$1 AND created_at >= \$2 ` + awareBucketGroupBy +
		`.*FROM blueprints WHERE team_id = \$1 AND created_at >= \$2 ` + awareBucketGroupBy +
		`.*FROM memories WHERE team_id = \$1 AND created_at >= \$3::timestamp GROUP BY DATE\(created_at\)` +
		`.*FROM projects WHERE team_id = \$1 AND created_at >= \$2 ` + awareBucketGroupBy

	teamFeedCreationBucketRE = `FROM feeds WHERE team_id = \$1 AND created_at >= \$2 ` + awareBucketGroupBy +
		`.*FROM feed_items WHERE team_id = \$1 AND posted_at >= \$2 ` +
		`GROUP BY \(posted_at AT TIME ZONE 'UTC'\)::date`

	// The two access-metric queries changed identically to their siblings, so
	// they are pinned the same way rather than by a bare table name.
	teamAccessMetricsRE = `SELECT TO_CHAR\(\(created_at AT TIME ZONE 'UTC'\)::date, 'YYYY-MM-DD'\) AS date, ` +
		`source, COUNT\(\*\) AS count FROM resource_access_events`
)

// TestTeamRepository_GetTeamStats verifies all six team-wide counts are scoped by
// team_id and scanned into the response in order.
func TestTeamRepository_GetTeamStats(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewTeamRepository(&database.DB{DB: db})

	mock.ExpectQuery(`SELECT`).
		WithArgs(testAnalyticsTeamID).
		WillReturnRows(sqlmock.NewRows([]string{"projects", "prompts", "artifacts", "blueprints", "memories", "feed_items"}).
			AddRow(4, 25, 13, 6, 40, 52))

	got, err := repo.GetTeamStats(context.Background(), testAnalyticsTeamID)

	require.NoError(t, err)
	assert.Equal(t, &models.TeamStatsResponse{
		TotalProjects:   4,
		TotalPrompts:    25,
		TotalArtifacts:  13,
		TotalBlueprints: 6,
		TotalMemories:   40,
		TotalFeedItems:  52,
	}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTeamRepository_GetTeamResourceCreationMetrics_Success verifies the method
// returns the sparse per-day per-type counts verbatim (zero-fill is the handler's
// job), including the team-only "projects" series.
func TestTeamRepository_GetTeamResourceCreationMetrics_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewTeamRepository(&database.DB{DB: db})
	since := time.Now().UTC().AddDate(0, 0, -7)

	mock.ExpectQuery(teamResourceCreationBucketRE).
		WithArgs(testAnalyticsTeamID, since, since.UTC()).
		WillReturnRows(sqlmock.NewRows([]string{"date", "resource_type", "count"}).
			AddRow("2026-05-28", "projects", 1).
			AddRow("2026-05-28", "prompts", 3).
			AddRow("2026-05-29", "memories", 2))

	got, err := repo.GetTeamResourceCreationMetrics(context.Background(), testAnalyticsTeamID, since)

	require.NoError(t, err)
	assert.Equal(t, []models.TeamResourceCreationCount{
		{Date: "2026-05-28", ResourceType: "projects", Count: 1},
		{Date: "2026-05-28", ResourceType: "prompts", Count: 3},
		{Date: "2026-05-29", ResourceType: "memories", Count: 2},
	}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTeamRepository_GetTeamResourceCreationMetrics_Empty verifies a team with no
// creations in the window returns an empty (non-nil) slice.
func TestTeamRepository_GetTeamResourceCreationMetrics_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewTeamRepository(&database.DB{DB: db})
	since := time.Now().UTC().AddDate(0, 0, -30)

	mock.ExpectQuery(teamResourceCreationBucketRE).
		WithArgs(testAnalyticsTeamID, since, since.UTC()).
		WillReturnRows(sqlmock.NewRows([]string{"date", "resource_type", "count"}))

	got, err := repo.GetTeamResourceCreationMetrics(context.Background(), testAnalyticsTeamID, since)

	require.NoError(t, err)
	assert.Equal(t, []models.TeamResourceCreationCount{}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestResourceAccessRepository_GetTeamMetrics verifies the team-wide access query
// groups by day+source across the whole team (no resource_type/resource_id) and
// returns the sparse rows verbatim.
func TestResourceAccessRepository_GetTeamMetrics(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewResourceAccessRepository(&database.DB{DB: db})
	since := time.Now().UTC().AddDate(0, 0, -7)

	mock.ExpectQuery(teamAccessMetricsRE).
		WithArgs(testAnalyticsTeamID, since).
		WillReturnRows(sqlmock.NewRows([]string{"date", "source", "count"}).
			AddRow("2026-05-28", "web", 3).
			AddRow("2026-05-28", "cli", 1).
			AddRow("2026-05-29", "mcp", 2))

	got, err := repo.GetTeamMetrics(context.Background(), testAnalyticsTeamID, since)

	require.NoError(t, err)
	assert.Equal(t, []models.DailyAccessCount{
		{Date: "2026-05-28", Source: "web", Count: 3},
		{Date: "2026-05-28", Source: "cli", Count: 1},
		{Date: "2026-05-29", Source: "mcp", Count: 2},
	}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTeamRepository_GetTeamFeedCreationMetrics_Success verifies the method returns
// the sparse per-day per-entity counts verbatim (zero-fill is the handler's job),
// covering both the feeds and feed_items series.
func TestTeamRepository_GetTeamFeedCreationMetrics_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewTeamRepository(&database.DB{DB: db})
	since := time.Now().UTC().AddDate(0, 0, -7)

	mock.ExpectQuery(teamFeedCreationBucketRE).
		WithArgs(testAnalyticsTeamID, since).
		WillReturnRows(sqlmock.NewRows([]string{"date", "entity_type", "count"}).
			AddRow("2026-05-28", "feed_items", 4).
			AddRow("2026-05-29", "feeds", 1).
			AddRow("2026-05-29", "feed_items", 2))

	got, err := repo.GetTeamFeedCreationMetrics(context.Background(), testAnalyticsTeamID, since)

	require.NoError(t, err)
	assert.Equal(t, []models.TeamFeedCreationCount{
		{Date: "2026-05-28", EntityType: "feed_items", Count: 4},
		{Date: "2026-05-29", EntityType: "feeds", Count: 1},
		{Date: "2026-05-29", EntityType: "feed_items", Count: 2},
	}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestTeamRepository_GetTeamFeedCreationMetrics_Empty verifies a team with no feed
// activity in the window returns an empty (non-nil) slice.
func TestTeamRepository_GetTeamFeedCreationMetrics_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewTeamRepository(&database.DB{DB: db})
	since := time.Now().UTC().AddDate(0, 0, -30)

	mock.ExpectQuery(teamFeedCreationBucketRE).
		WithArgs(testAnalyticsTeamID, since).
		WillReturnRows(sqlmock.NewRows([]string{"date", "entity_type", "count"}))

	got, err := repo.GetTeamFeedCreationMetrics(context.Background(), testAnalyticsTeamID, since)

	require.NoError(t, err)
	assert.Equal(t, []models.TeamFeedCreationCount{}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestResourceAccessRepository_GetTopAccessedResources verifies the ranking query
// passes the limit, scans resource_type/resource_id/access_count/name, and returns
// the resolved rows in order.
func TestResourceAccessRepository_GetTopAccessedResources(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewResourceAccessRepository(&database.DB{DB: db})
	since := time.Now().UTC().AddDate(0, 0, -30)

	mock.ExpectQuery(`WITH ranked AS`).
		WithArgs(testAnalyticsTeamID, since, 5).
		WillReturnRows(sqlmock.NewRows([]string{"resource_type", "resource_id", "access_count", "name"}).
			AddRow("prompt", "b3f1c2d4-5678-49ab-9cde-0123456789ab", 128, "Onboarding checklist").
			AddRow("artifact", "c4f2d3e5-6789-4abc-8def-1234567890bc", 64, "Q2 report"))

	got, err := repo.GetTopAccessedResources(context.Background(), testAnalyticsTeamID, since, "", 5)

	require.NoError(t, err)
	assert.Equal(t, []models.TopAccessedResource{
		{
			ResourceType: "prompt",
			ResourceID:   "b3f1c2d4-5678-49ab-9cde-0123456789ab",
			Name:         "Onboarding checklist",
			AccessCount:  128,
		},
		{ResourceType: "artifact", ResourceID: "c4f2d3e5-6789-4abc-8def-1234567890bc", Name: "Q2 report", AccessCount: 64},
	}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestResourceAccessRepository_GetTopAccessedResources_SourceFilter verifies a
// concrete source adds the channel predicate and binds it as an arg before the
// limit, restricting the ranking to that access channel.
func TestResourceAccessRepository_GetTopAccessedResources_SourceFilter(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewResourceAccessRepository(&database.DB{DB: db})
	since := time.Now().UTC().AddDate(0, 0, -30)

	mock.ExpectQuery(`source = \$3`).
		WithArgs(testAnalyticsTeamID, since, "cli", 5).
		WillReturnRows(sqlmock.NewRows([]string{"resource_type", "resource_id", "access_count", "name"}).
			AddRow("prompt", "b3f1c2d4-5678-49ab-9cde-0123456789ab", 12, "CLI snippet"))

	got, err := repo.GetTopAccessedResources(context.Background(), testAnalyticsTeamID, since, "cli", 5)

	require.NoError(t, err)
	assert.Equal(t, []models.TopAccessedResource{
		{ResourceType: "prompt", ResourceID: "b3f1c2d4-5678-49ab-9cde-0123456789ab", Name: "CLI snippet", AccessCount: 12},
	}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestResourceAccessRepository_GetTopAccessedResources_Empty verifies a team with
// no access events in the window returns an empty (non-nil) slice.
func TestResourceAccessRepository_GetTopAccessedResources_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Failed to close database: %v", closeErr)
		}
	}()

	repo := postgres.NewResourceAccessRepository(&database.DB{DB: db})
	since := time.Now().UTC().AddDate(0, 0, -30)

	mock.ExpectQuery(`WITH ranked AS`).
		WithArgs(testAnalyticsTeamID, since, 5).
		WillReturnRows(sqlmock.NewRows([]string{"resource_type", "resource_id", "access_count", "name"}))

	got, err := repo.GetTopAccessedResources(context.Background(), testAnalyticsTeamID, since, "all", 5)

	require.NoError(t, err)
	assert.Equal(t, []models.TopAccessedResource{}, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}
