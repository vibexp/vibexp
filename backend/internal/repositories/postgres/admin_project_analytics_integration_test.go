//go:build integration

package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Integration coverage for the #1145 per-project admin queries. Everything is
// scoped by freshly seeded project ids, so no table truncation is needed.

// projectDay is the day most fixture activity happens on.
var projectDay = time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)

// adminProjectFixture is the seeded population: project P in team T, a sibling
// project Q in the same team, and an agent of team T.
type adminProjectFixture struct {
	user, team, project, sibling       string
	prompt, artifact, blueprint, agent string
	memory, siblingPrompt              string
}

// seedAdminProjectFixture seeds one resource of each project-scoped type in P
// (plus one in Q), and access events:
//
//	P's prompt    3 web + 1 mcp (the next day)
//	P's memory    2 cli
//	P's artifact  2 web (ties with the memory)
//	P itself      1 web (counts in the series, never in top resources)
//	Q's prompt    4 web, and T's agent 3 api (both must be excluded)
//
// plus one access to P's prompt just before and one just after the window
// [projectDay's midnight, +2 days).
func seedAdminProjectFixture(t *testing.T) adminProjectFixture {
	t.Helper()
	f := adminProjectFixture{user: insertAdminListUser(t, "prj"+uuid.New().String()[:8], "user")}
	f.team = insertTestTeam(t, f.user)
	f.project = insertTestProject(t, f.user, f.team)
	f.sibling = insertTestProject(t, f.user, f.team)

	f.prompt = insertTestPrompt(t, f.user, f.team, f.project, "secret title", "body", "published")
	f.siblingPrompt = insertTestPrompt(t, f.user, f.team, f.sibling, "other", "body", "published")
	f.artifact = insertTestArtifact(t, f.user, f.team, f.project, "a", "content", "active")
	f.blueprint = uuid.New().String()
	adminListExec(t, "INSERT INTO blueprints (id, user_id, team_id, project_id, slug, title, content, path) "+
		"VALUES ($1, $2, $3, $4, $5, 'b', 'content', 'CLAUDE.md')",
		f.blueprint, f.user, f.team, f.project, "bp-"+uuid.New().String()[:8])
	f.agent = insertTestAgent(t, f.user, f.team)

	// Pin the aware creation times to projectDay (UPDATE is safe on these; the
	// memories trigger is why memories are INSERTed with explicit naive values).
	for _, stmt := range []string{
		"UPDATE prompts SET created_at = $2, updated_at = $2 WHERE team_id = $1",
		"UPDATE artifacts SET created_at = $2, updated_at = $2 WHERE team_id = $1",
		"UPDATE blueprints SET created_at = $2, updated_at = $2 WHERE team_id = $1",
	} {
		adminListExec(t, stmt, f.team, projectDay)
	}

	feedID := uuid.New().String()
	adminListExec(t, "INSERT INTO feeds (id, team_id, name, created_by_user_id) VALUES ($1, $2, 'feed', $3)",
		feedID, f.team, f.user)
	feedItem := "INSERT INTO feed_items " +
		"(id, team_id, feed_id, project_id, title, content, excerpt, ai_assistant_name, posted_by_user_id, posted_at) " +
		"VALUES ($1, $2, $3, $4, 'item', 'content', 'excerpt', 'Claude Code', $5, $6)"
	// One just inside the window's end, one exactly at it (excluded).
	windowEnd := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	adminListExec(t, feedItem, uuid.New().String(), f.team, feedID, f.project, f.user, windowEnd.Add(-time.Second))
	adminListExec(t, feedItem, uuid.New().String(), f.team, feedID, f.project, f.user, windowEnd)

	// Memories: naive timestamps, one exactly at the window start (included),
	// one a second before it (excluded), one in the sibling project.
	memory := "INSERT INTO memories (id, user_id, team_id, project_id, text, created_at, updated_at) " +
		"VALUES ($1, $2, $3, $4, 'remember', $5::timestamp, $5::timestamp)"
	f.memory = uuid.New().String()
	adminListExec(t, memory, f.memory, f.user, f.team, f.project, "2026-05-12 00:00:00")
	adminListExec(t, memory, uuid.New().String(), f.user, f.team, f.project, "2026-05-11 23:59:59")
	adminListExec(t, memory, uuid.New().String(), f.user, f.team, f.sibling, "2026-05-12 10:00:00")

	for i := 0; i < 3; i++ {
		insertAccessEvent(t, f.team, f.user, "prompt", f.prompt, "web", projectDay)
	}
	insertAccessEvent(t, f.team, f.user, "prompt", f.prompt, "mcp", projectDay.AddDate(0, 0, 1))
	insertAccessEvent(t, f.team, f.user, "memory", f.memory, "cli", projectDay)
	insertAccessEvent(t, f.team, f.user, "memory", f.memory, "cli", projectDay)
	insertAccessEvent(t, f.team, f.user, "artifact", f.artifact, "web", projectDay)
	insertAccessEvent(t, f.team, f.user, "artifact", f.artifact, "web", projectDay)
	insertAccessEvent(t, f.team, f.user, "project", f.project, "web", projectDay)
	for i := 0; i < 4; i++ {
		insertAccessEvent(t, f.team, f.user, "prompt", f.siblingPrompt, "web", projectDay)
	}
	for i := 0; i < 3; i++ {
		insertAccessEvent(t, f.team, f.user, "agent", f.agent, "api", projectDay)
	}
	insertAccessEvent(t, f.team, f.user, "project", f.sibling, "web", projectDay)
	insertAccessEvent(t, f.team, f.user, "prompt", f.prompt, "web", time.Date(2026, 5, 11, 23, 59, 59, 0, time.UTC))
	insertAccessEvent(t, f.team, f.user, "prompt", f.prompt, "web", windowEnd)
	return f
}

// projectWindow is the [from, to) window the fixture's edges are built around.
func projectWindow() (from, to time.Time) {
	return time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC), time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
}

func TestAdminProjectAnalytics_ProjectTeamID(t *testing.T) {
	f := seedAdminProjectFixture(t)
	repo := NewAdminRepository(integrationDB)

	teamID, found, err := repo.ProjectTeamID(context.Background(), f.project)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, f.team, teamID)

	_, found, err = repo.ProjectTeamID(context.Background(), uuid.New().String())
	require.NoError(t, err)
	assert.False(t, found)
}

func TestAdminProjectAnalytics_CreationSeries(t *testing.T) {
	f := seedAdminProjectFixture(t)
	repo := NewAdminRepository(integrationDB)
	from, to := projectWindow()

	rows, err := repo.GetProjectCreationSeries(context.Background(), f.project, from, to, "day")
	require.NoError(t, err)
	got := make(map[string]int64)
	for _, r := range rows {
		got[fmt.Sprintf("%s@%s", r.Entity, r.Bucket.UTC().Format("2006-01-02"))] = r.Count
	}
	assert.Equal(t, map[string]int64{
		"prompt@2026-05-12":    1,
		"artifact@2026-05-12":  1,
		"blueprint@2026-05-12": 1,
		"memory@2026-05-12":    1,
		"feed_item@2026-05-13": 1,
	}, got, "the sibling project's rows and both out-of-window edges are excluded")

	// Session-TZ trap: a naive memory at exactly the window start is included
	// whatever the session timezone is.
	tx, err := integrationDB.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(context.Background(), "SET LOCAL TIME ZONE 'America/New_York'")
	require.NoError(t, err)
	var memories int
	require.NoError(t, tx.QueryRowContext(context.Background(),
		fmt.Sprintf("SELECT COALESCE(SUM(count), 0) FROM (%s) s WHERE resource_type = 'memory'",
			fmt.Sprintf(adminProjectCreationQueryFmt, adminTruncUnit("day"))),
		f.project, from, to, from.UTC(), to.UTC()).Scan(&memories))
	assert.Equal(t, 1, memories)
}

func TestAdminProjectAnalytics_AccessBySourceSeries(t *testing.T) {
	f := seedAdminProjectFixture(t)
	repo := NewAdminRepository(integrationDB)
	from, to := projectWindow()
	next := from.AddDate(0, 0, 1)

	got, err := repo.GetProjectAccessBySourceSeries(context.Background(), f.project, f.team, from, to, "day")
	require.NoError(t, err)
	assert.Equal(t, []models.AdminSourcePoint{
		{Bucket: from, Source: "cli", Count: 2},
		{Bucket: from, Source: "web", Count: 6},
		{Bucket: next, Source: "mcp", Count: 1},
	}, normalizeSourcePoints(got),
		"the project's own page counts; the sibling project, the agent and both window edges do not")

	// A wrong team yields nothing: events are filtered on the project's team.
	got, err = repo.GetProjectAccessBySourceSeries(context.Background(), f.project, uuid.New().String(),
		from, to, "day")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestAdminProjectAnalytics_TopAccessedResources(t *testing.T) {
	f := seedAdminProjectFixture(t)
	repo := NewAdminRepository(integrationDB)
	from, to := projectWindow()

	got, err := repo.GetProjectTopAccessedResources(context.Background(), f.project, f.team, from, to, 10)
	require.NoError(t, err)
	require.Len(t, got, 3, "the project row, the sibling's prompt and the agent are not listed")

	assert.Equal(t, f.prompt, got[0].ResourceID)
	assert.Equal(t, "prompt", got[0].ResourceType)
	assert.Equal(t, int64(4), got[0].AccessCount)
	for _, r := range got {
		assert.Equal(t, f.team, r.TeamID)
		assert.NotEmpty(t, r.TeamName)
		require.NotNil(t, r.ProjectID)
		assert.Equal(t, f.project, *r.ProjectID)
		assert.NotNil(t, r.ProjectName)
		assert.False(t, r.ResourceDeleted)
		assert.NotEqual(t, "project", r.ResourceType)
	}
	// The memory and the artifact tie at 2; the tie is broken by resource id.
	assert.Equal(t, got[1].AccessCount, got[2].AccessCount)
	assert.Less(t, got[1].ResourceID, got[2].ResourceID)
	assert.ElementsMatch(t, []string{f.memory, f.artifact}, []string{got[1].ResourceID, got[2].ResourceID})

	got, err = repo.GetProjectTopAccessedResources(context.Background(), f.project, f.team, from, to, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, f.prompt, got[0].ResourceID)

	// A deleted resource drops out of the project's ranking.
	adminListExec(t, "DELETE FROM artifacts WHERE id = $1", f.artifact)
	got, err = repo.GetProjectTopAccessedResources(context.Background(), f.project, f.team, from, to, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, r := range got {
		assert.NotEqual(t, f.artifact, r.ResourceID)
	}
}

// TestAdminProjectAnalytics_AccessQueriesUseAnRAEIndex EXPLAINs the production
// access statements under SET LOCAL enable_seqscan = off (inside a transaction,
// not against the pool) and reads EVERY plan line: a bitmap scan names its
// index only in a child node.
func TestAdminProjectAnalytics_AccessQueriesUseAnRAEIndex(t *testing.T) {
	ctx := context.Background()
	f := seedAdminProjectFixture(t)
	from, to := projectWindow()

	for name, tc := range map[string]struct {
		query string
		args  []any
	}{
		"access by source": {fmt.Sprintf(adminProjectAccessBySourceQueryFmt, adminTruncUnit("day")),
			[]any{f.project, f.team, from, to}},
		"top accessed": {adminProjectTopAccessedQuery, []any{f.project, f.team, from, to, 10}},
	} {
		t.Run(name, func(t *testing.T) {
			tx, err := integrationDB.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			_, err = tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off")
			require.NoError(t, err)

			rows, err := tx.QueryContext(ctx, "EXPLAIN "+tc.query, tc.args...)
			require.NoError(t, err)
			defer func() { _ = rows.Close() }()
			var plan strings.Builder
			for rows.Next() {
				var line string
				require.NoError(t, rows.Scan(&line))
				plan.WriteString(line + "\n")
			}
			require.NoError(t, rows.Err())
			assert.Contains(t, plan.String(), "idx_rae_",
				"the project %s query must use an idx_rae_* index; plan was:\n%s", name, plan.String())
			assert.NotContains(t, plan.String(), "Seq Scan on resource_access_events",
				"plan was:\n%s", plan.String())
		})
	}
}
