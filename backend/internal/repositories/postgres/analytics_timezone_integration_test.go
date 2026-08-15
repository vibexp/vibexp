//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
)

// The analytics daily-series queries bucket a calendar day out of a timestamp,
// and every one of them must produce the SAME key no matter what the Postgres
// session timezone happens to be (#773).
//
// That property is invisible to every other test in this repository: the unit
// suite asserts query TEXT and the rest of the integration suite runs under
// whatever timezone the container defaults to, which is UTC — so a bucket that
// silently follows the session would look perfectly correct everywhere. This
// file is the only thing that varies the session timezone, which is why the
// divergence between these queries and their already-fixed neighbours was able
// to appear in the first place.
//
// The failure it guards is silent and lossy, not merely cosmetic: the handler
// zero-fills a continuous series of UTC day keys and looks each bucket up in a
// map, so a bucket key the series never generated is DROPPED — no error, no log,
// just a short bar. On the cumulative freshness series it is worse, because a
// dropped flow shifts every earlier day's level.

// tzTestZone is UTC+13 (or +12 outside DST), chosen so that a row anywhere in
// the last 13 hours of a UTC day falls on the NEXT calendar day locally. A
// zone west of UTC would work equally well in the other direction; what matters
// is that the offset is large enough that the boundary rows land on a different
// date, and Auckland is the standard "far from UTC" fixture.
const tzTestZone = "Pacific/Auckland"

// openSessionTZDB opens a SECOND pool to the same database pinned to one
// connection, so `SET TimeZone` sticks for every query the returned handle
// makes. The shared integrationDB cannot be used: it is a pool, so a session
// setting would apply to whichever connection happened to serve the statement
// and leak into unrelated tests.
func openSessionTZDB(t *testing.T, zone string) *database.DB {
	t.Helper()

	dsn, ok := os.LookupEnv("POSTGRES_TEST_DSN")
	if !ok || dsn == "" {
		dsn = defaultTestDSN
	}

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("failed to close session-tz database: %v", closeErr)
		}
	})

	// Not SET LOCAL: that would need a transaction wrapping every call, and the
	// repositories issue their own statements on the handle they are given.
	_, err = db.ExecContext(context.Background(), "SET TimeZone = '"+zone+"'")
	require.NoError(t, err)

	var got string
	require.NoError(t, db.QueryRowContext(context.Background(), "SHOW TimeZone").Scan(&got))
	require.Equal(t, zone, got, "fixture: the session timezone must actually be set, or this test proves nothing")

	return &database.DB{DB: db}
}

// seedBoundaryRows creates a team, a project and four prompts placed either
// side of a UTC midnight, returning the team and project ids.
//
// Two rows sit in the last half-hour of 2026-03-01 UTC and two in the first
// half-hour of 2026-03-02 UTC. Under Auckland (UTC+13) all four read as local
// 2026-03-02, so a session-dependent bucket collapses them into one day while a
// UTC bucket keeps them two-and-two. That difference is the whole assertion.
func seedBoundaryRows(t *testing.T) (teamID, projectID string, beforeUTC, afterUTC time.Time) {
	t.Helper()

	userID := insertTestUser(t)
	teamID = insertTestTeam(t, userID)
	projectID = insertTestProject(t, userID, teamID)

	beforeUTC = time.Date(2026, 3, 1, 23, 30, 0, 0, time.UTC)
	afterUTC = time.Date(2026, 3, 2, 0, 30, 0, 0, time.UTC)

	for _, at := range []time.Time{beforeUTC, beforeUTC.Add(5 * time.Minute), afterUTC, afterUTC.Add(5 * time.Minute)} {
		promptID := insertTestPrompt(t, userID, teamID, projectID, "p-"+uuid.New().String()[:8], "body", "published")
		_, err := integrationDB.ExecContext(context.Background(),
			"UPDATE prompts SET created_at = $1 WHERE id = $2", at, promptID)
		require.NoError(t, err)
	}

	// A memories row either side too. This is not decoration: memories is the
	// NAIVE branch of the same UNION, and its window edge behaves differently
	// from its bucket -- a fixture of prompts alone makes both branches of that
	// distinction invisible, which is exactly how the shared-placeholder defect
	// survived the first version of this test.
	for _, at := range []time.Time{beforeUTC, afterUTC} {
		memoryID := insertTestMemory(t, userID, teamID, projectID, "m-"+uuid.New().String()[:8])
		// memories carries a BEFORE UPDATE trigger that rewrites updated_at
		// (migration 014); it does not touch created_at, so a plain UPDATE is
		// fine here. The value is written naive, as the column is.
		_, err := integrationDB.ExecContext(context.Background(),
			"UPDATE memories SET created_at = $1 WHERE id = $2", at.Format("2006-01-02 15:04:05"), memoryID)
		require.NoError(t, err)
	}
	return teamID, projectID, beforeUTC, afterUTC
}

// seedFeedBoundaryRows creates a feed and a feed item either side of a UTC
// midnight. GetTeamFeedCreationMetrics buckets two different aware columns
// (feeds.created_at and feed_items.posted_at), so it needs its own fixture.
func seedFeedBoundaryRows(t *testing.T, userID, teamID string, beforeUTC, afterUTC time.Time) {
	t.Helper()

	feedID := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		`INSERT INTO feeds (id, team_id, name, created_by_user_id, created_at)
		 VALUES ($1, $2, 'boundary feed', $3, $4)`,
		feedID, teamID, userID, beforeUTC)
	require.NoError(t, err)

	_, err = integrationDB.ExecContext(context.Background(),
		`INSERT INTO feed_items
		   (id, team_id, feed_id, title, content, excerpt, ai_assistant_name, posted_by_user_id, posted_at)
		 VALUES ($1, $2, $3, 'boundary item', 'content', 'excerpt', 'Claude Code', $4, $5)`,
		uuid.New().String(), teamID, feedID, userID, afterUTC)
	require.NoError(t, err)
}

// countsByDate flattens a creation-metrics result into date -> total.
func countsByDate[T any](rows []T, date func(T) string, count func(T) int) map[string]int {
	out := map[string]int{}
	for _, row := range rows {
		out[date(row)] += count(row)
	}
	return out
}

// The property in one line: same data, two session timezones, identical buckets.
func TestIntegrationAnalyticsTimezone_TeamResourceCreationBucketsAreUTC(t *testing.T) {
	resetIntegrationTables(t)
	teamID, _, beforeUTC, _ := seedBoundaryRows(t)
	// The window starts EXACTLY at the earliest seeded row, not a day before:
	// the naive-branch defect is a window-EDGE failure, and a 24h-wide margin
	// swallows the largest session offset there is, so the row stays inside the
	// range under both timezones and the test sees nothing. A fixture that
	// cannot distinguish the branch under test is exactly what let the shared
	// placeholder ship in the first place.
	since := beforeUTC
	ctx := context.Background()

	utc, err := NewTeamRepository(integrationDB).
		GetTeamResourceCreationMetrics(ctx, teamID, since)
	require.NoError(t, err)

	shifted, err := NewTeamRepository(openSessionTZDB(t, tzTestZone)).
		GetTeamResourceCreationMetrics(ctx, teamID, since)
	require.NoError(t, err)

	assert.Equal(t, utc, shifted, "the bucket must not follow the session timezone")

	// Only the prompts series: this query also counts the team's own project,
	// which the fixture creates at the current time and which would otherwise
	// add a today-bucket that says nothing about the boundary.
	prompts := make([]models.TeamResourceCreationCount, 0, len(utc))
	for _, row := range utc {
		if row.ResourceType == "prompts" {
			prompts = append(prompts, row)
		}
	}
	byDate := countsByDate(prompts,
		func(r models.TeamResourceCreationCount) string { return r.Date },
		func(r models.TeamResourceCreationCount) int { return r.Count })
	assert.Equal(t, map[string]int{"2026-03-01": 2, "2026-03-02": 2}, byDate,
		"rows must land on their UTC day, two either side of the boundary")

	// The naive branch, asserted separately because its failure mode is the
	// window edge rather than the bucket: with the memories predicate sharing a
	// placeholder the other branches make timestamptz, the earlier row is
	// filtered out entirely under a non-UTC session.
	//
	// Built from `shifted`, NOT `utc` -- the defect only manifests in the
	// non-UTC session, so a map derived from the UTC result could never observe
	// it and the assertion would be decoration.
	memories := make([]models.TeamResourceCreationCount, 0, len(shifted))
	for _, row := range shifted {
		if row.ResourceType == "memories" {
			memories = append(memories, row)
		}
	}
	byMemoryDate := countsByDate(memories,
		func(r models.TeamResourceCreationCount) string { return r.Date },
		func(r models.TeamResourceCreationCount) int { return r.Count })
	assert.Equal(t, map[string]int{"2026-03-01": 1, "2026-03-02": 1}, byMemoryDate,
		"the naive memories branch must survive the range predicate too")
}

func TestIntegrationAnalyticsTimezone_ProjectResourceCreationBucketsAreUTC(t *testing.T) {
	resetIntegrationTables(t)
	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	projectID := insertTestProject(t, userID, teamID)

	beforeUTC := time.Date(2026, 3, 1, 23, 30, 0, 0, time.UTC)
	afterUTC := time.Date(2026, 3, 2, 0, 30, 0, 0, time.UTC)
	for _, at := range []time.Time{beforeUTC, afterUTC} {
		promptID := insertTestPrompt(t, userID, teamID, projectID, "p-"+uuid.New().String()[:8], "body", "published")
		_, err := integrationDB.ExecContext(context.Background(),
			"UPDATE prompts SET created_at = $1 WHERE id = $2", at, promptID)
		require.NoError(t, err)

		// The naive branch, for the same reason as the team fixture.
		memoryID := insertTestMemory(t, userID, teamID, projectID, "m-"+uuid.New().String()[:8])
		_, err = integrationDB.ExecContext(context.Background(),
			"UPDATE memories SET created_at = $1 WHERE id = $2", at.Format("2006-01-02 15:04:05"), memoryID)
		require.NoError(t, err)
	}

	var slug string
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT slug FROM projects WHERE id = $1", projectID).Scan(&slug))

	ctx := context.Background()
	// The window starts EXACTLY at the earliest seeded row, not a day before:
	// the naive-branch defect is a window-EDGE failure, and a 24h-wide margin
	// swallows the largest session offset there is, so the row stays inside the
	// range under both timezones and the test sees nothing. A fixture that
	// cannot distinguish the branch under test is exactly what let the shared
	// placeholder ship in the first place.
	since := beforeUTC

	utc, err := NewProjectRepository(integrationDB).
		GetProjectResourceCreationMetrics(ctx, teamID, userID, slug, since)
	require.NoError(t, err)

	shifted, err := NewProjectRepository(openSessionTZDB(t, tzTestZone)).
		GetProjectResourceCreationMetrics(ctx, teamID, userID, slug, since)
	require.NoError(t, err)

	assert.Equal(t, utc, shifted)
	byDate := countsByDate(utc,
		func(r models.ProjectResourceCreationCount) string { return r.Date },
		func(r models.ProjectResourceCreationCount) int { return r.Count })
	// One prompt + one memory on each side of the boundary.
	assert.Equal(t, map[string]int{"2026-03-01": 2, "2026-03-02": 2}, byDate)
}

// GetTeamFeedCreationMetrics buckets two DIFFERENT aware columns
// (feeds.created_at and feed_items.posted_at), so a fix applied to one and not
// the other would be invisible to every other test.
func TestIntegrationAnalyticsTimezone_TeamFeedCreationBucketsAreUTC(t *testing.T) {
	resetIntegrationTables(t)
	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)

	beforeUTC := time.Date(2026, 3, 1, 23, 30, 0, 0, time.UTC)
	afterUTC := time.Date(2026, 3, 2, 0, 30, 0, 0, time.UTC)
	seedFeedBoundaryRows(t, userID, teamID, beforeUTC, afterUTC)

	ctx := context.Background()
	// A margin is safe here ONLY because this query has no naive branch and no
	// placeholder shared between column families -- its failure mode is the
	// bucket, which the straddling rows expose regardless of the window. Add a
	// naive branch to this query and the window must move to the earliest row,
	// as it did for the resource-creation tests above.
	since := beforeUTC.Add(-24 * time.Hour)

	utc, err := NewTeamRepository(integrationDB).GetTeamFeedCreationMetrics(ctx, teamID, since)
	require.NoError(t, err)
	shifted, err := NewTeamRepository(openSessionTZDB(t, tzTestZone)).
		GetTeamFeedCreationMetrics(ctx, teamID, since)
	require.NoError(t, err)

	assert.Equal(t, utc, shifted, "neither created_at nor posted_at may follow the session timezone")
	assert.ElementsMatch(t, []models.TeamFeedCreationCount{
		{Date: "2026-03-01", EntityType: "feeds", Count: 1},
		{Date: "2026-03-02", EntityType: "feed_items", Count: 1},
	}, utc, "the feed and the item sit on different UTC days")
}

// resource_access_events is the highest-traffic of these series and the one
// whose keys the zero-fill in services/resourceaccess must match exactly.
func TestIntegrationAnalyticsTimezone_ResourceAccessBucketsAreUTC(t *testing.T) {
	resetIntegrationTables(t)
	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	projectID := insertTestProject(t, userID, teamID)
	promptID := insertTestPrompt(t, userID, teamID, projectID, "accessed", "body", "published")

	beforeUTC := time.Date(2026, 3, 1, 23, 30, 0, 0, time.UTC)
	afterUTC := time.Date(2026, 3, 2, 0, 30, 0, 0, time.UTC)
	for _, at := range []time.Time{beforeUTC, afterUTC} {
		_, err := integrationDB.ExecContext(context.Background(),
			`INSERT INTO resource_access_events (id, team_id, user_id, resource_type, resource_id, source, created_at)
			 VALUES ($1, $2, $3, 'prompt', $4, 'web', $5)`,
			uuid.New().String(), teamID, userID, promptID, at)
		require.NoError(t, err)
	}

	ctx := context.Background()
	// A margin is safe here ONLY because this query has no naive branch and no
	// placeholder shared between column families -- its failure mode is the
	// bucket, which the straddling rows expose regardless of the window. Add a
	// naive branch to this query and the window must move to the earliest row,
	// as it did for the resource-creation tests above.
	since := beforeUTC.Add(-24 * time.Hour)

	utcTeam, err := NewResourceAccessRepository(integrationDB).GetTeamMetrics(ctx, teamID, since)
	require.NoError(t, err)
	shiftedTeam, err := NewResourceAccessRepository(openSessionTZDB(t, tzTestZone)).
		GetTeamMetrics(ctx, teamID, since)
	require.NoError(t, err)
	assert.Equal(t, utcTeam, shiftedTeam)

	byDate := countsByDate(utcTeam,
		func(r models.DailyAccessCount) string { return r.Date },
		func(r models.DailyAccessCount) int { return r.Count })
	assert.Equal(t, map[string]int{"2026-03-01": 1, "2026-03-02": 1}, byDate)

	utcRes, err := NewResourceAccessRepository(integrationDB).
		GetMetricsByResource(ctx, teamID, "prompt", promptID, since)
	require.NoError(t, err)
	shiftedRes, err := NewResourceAccessRepository(openSessionTZDB(t, tzTestZone)).
		GetMetricsByResource(ctx, teamID, "prompt", promptID, since)
	require.NoError(t, err)
	assert.Equal(t, utcRes, shiftedRes)
}

// The naive half of the rule. activities.created_at is `timestamp without time
// zone`, so its bucket is session-independent already and must stay a bare
// DATE() -- converting it would introduce the very bug the aware branches were
// just fixed for, in the opposite direction. This pins that the two families
// are treated differently on purpose.
func TestIntegrationAnalyticsTimezone_NaiveActivityBucketsAreSessionIndependent(t *testing.T) {
	resetIntegrationTables(t)
	userID := insertTestUser(t)

	for _, at := range []time.Time{
		time.Date(2026, 3, 1, 23, 30, 0, 0, time.UTC),
		time.Date(2026, 3, 2, 0, 30, 0, 0, time.UTC),
	} {
		_, err := integrationDB.ExecContext(context.Background(),
			`INSERT INTO activities (id, user_id, activity_type, entity_type, description, created_at)
			 VALUES ($1, $2, 'test', 'prompt', 'boundary row', $3)`,
			uuid.New().String(), userID, at)
		require.NoError(t, err)
	}

	var utcDates, shiftedDates []string
	collect := func(db *database.DB) []string {
		rows, err := db.QueryContext(context.Background(),
			"SELECT DATE(created_at)::text FROM activities WHERE user_id = $1 ORDER BY 1", userID)
		require.NoError(t, err)
		defer func() { require.NoError(t, rows.Close()) }()

		var out []string
		for rows.Next() {
			var d string
			require.NoError(t, rows.Scan(&d))
			out = append(out, d)
		}
		require.NoError(t, rows.Err())
		return out
	}

	shiftedDB := openSessionTZDB(t, tzTestZone)
	utcDates = collect(integrationDB)
	shiftedDates = collect(shiftedDB)

	assert.Equal(t, []string{"2026-03-01", "2026-03-02"}, utcDates)
	assert.Equal(t, utcDates, shiftedDates,
		"a naive column's DATE() is already session-independent; it must not be converted")

	// And the converse, which is the part that actually guards the decision:
	// applying the aware fix to this naive column WOULD break it. Without this,
	// the test above passes just as well after someone "fixes" the activities
	// branch to match its TIMESTAMPTZ neighbours.
	rows, err := shiftedDB.QueryContext(context.Background(),
		"SELECT (created_at AT TIME ZONE 'UTC')::date::text FROM activities WHERE user_id = $1 ORDER BY 1", userID)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()

	var converted []string
	for rows.Next() {
		var d string
		require.NoError(t, rows.Scan(&d))
		converted = append(converted, d)
	}
	require.NoError(t, rows.Err())
	assert.NotEqual(t, utcDates, converted,
		"AT TIME ZONE 'UTC' on a NAIVE column re-labels rather than converts, shifting the day the wrong way")
}
