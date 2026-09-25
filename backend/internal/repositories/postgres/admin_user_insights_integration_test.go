//go:build integration

package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// Integration coverage for the #1135 user insights queries. Everything is
// scoped by the seeded user's id, so no table truncation is needed.

// insightsDay is the fixed day most fixture rows are created on.
var insightsDay = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

// adminInsightsFixture is the seeded population.
type adminInsightsFixture struct {
	user, teammate           string
	teamA, teamB             string
	projectA1, projectA2     string
	projectB                 string
	phantomPrompt, edited    string
	expectedEvents           int
	expectedUpdatedResources []string
}

// seedAdminInsightsFixture seeds, for one user:
//
//	team A (member): 2 prompts + 1 artifact + 1 blueprint in project A1,
//	  1 memory in project A2 (naive timestamp at a day edge), 1 agent, 1 feed,
//	  1 feed item (posted_at, attached to project A1), 1 comment, 1 attachment
//	team B (NOT a member): 1 prompt in project B, 1 memory in project B
//
// plus a teammate's prompt in project A1 that must be excluded everywhere.
// One of the user's prompts is "phantom-updated" (updated_at 500ms after
// created_at, as split time.Now() calls produce) and one is genuinely edited.
func seedAdminInsightsFixture(t *testing.T) adminInsightsFixture {
	t.Helper()
	ctx := context.Background()
	f := adminInsightsFixture{
		user:     insertAdminListUser(t, "ins"+uuid.New().String()[:8], "user"),
		teammate: insertAdminListUser(t, "ins"+uuid.New().String()[:8], "teammate"),
	}
	f.teamA = insertTestTeam(t, f.user)
	f.teamB = insertTestTeam(t, f.teammate)
	membership := "INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)"
	adminListExec(t, membership, f.teamA, f.user, "owner")
	adminListExec(t, membership, f.teamA, f.teammate, "member")
	adminListExec(t, membership, f.teamB, f.teammate, "owner")

	f.projectA1 = insertTestProject(t, f.user, f.teamA)
	f.projectA2 = insertTestProject(t, f.user, f.teamA)
	f.projectB = insertTestProject(t, f.teammate, f.teamB)

	f.phantomPrompt = insertTestPrompt(t, f.user, f.teamA, f.projectA1, "p", "body", "published")
	f.edited = insertTestPrompt(t, f.user, f.teamA, f.projectA1, "p", "body", "published")
	insertTestPrompt(t, f.user, f.teamB, f.projectB, "p", "body", "published")
	insertTestPrompt(t, f.teammate, f.teamA, f.projectA1, "teammate", "body", "published")
	artifactID := insertTestArtifact(t, f.user, f.teamA, f.projectA1, "a", "content", "active")
	adminListExec(t, "INSERT INTO blueprints (id, user_id, team_id, project_id, slug, title, content, path) "+
		"VALUES ($1, $2, $3, $4, $5, 'b', 'content', 'CLAUDE.md')",
		uuid.New().String(), f.user, f.teamA, f.projectA1, "bp-"+uuid.New().String()[:8])
	insertTestAgent(t, f.user, f.teamA)
	feedID := uuid.New().String()
	adminListExec(t, "INSERT INTO feeds (id, team_id, name, created_by_user_id) VALUES ($1, $2, 'feed', $3)",
		feedID, f.teamA, f.user)
	adminListExec(t, "INSERT INTO feed_items "+
		"(id, team_id, feed_id, project_id, title, content, excerpt, ai_assistant_name, posted_by_user_id, posted_at) "+
		"VALUES ($1, $2, $3, $4, 'item', 'content', 'excerpt', 'Claude Code', $5, $6)",
		uuid.New().String(), f.teamA, feedID, f.projectA1, f.user,
		time.Date(2026, 3, 11, 23, 59, 59, 0, time.UTC))
	adminListExec(t, "INSERT INTO comments (team_id, resource_type, resource_id, user_id, content) "+
		"VALUES ($1, 'artifact', $2, $3, 'nice')", f.teamA, artifactID, f.user)
	adminListExec(t, "INSERT INTO attachments "+
		"(team_id, user_id, owner_type, owner_id, file_name, content_type, size_bytes, gcs_object_key) "+
		"VALUES ($1, $2, 'artifact', $3, $4, 'text/plain', 1, $4)",
		f.teamA, f.user, artifactID, "a-"+uuid.New().String())

	// Memories are inserted with explicit NAIVE timestamps (the table's family),
	// so the BEFORE UPDATE trigger never gets a chance to bump updated_at. The
	// team-A one sits exactly on a day boundary; the team-B one one second
	// before the end of the series window below.
	memory := "INSERT INTO memories (id, user_id, team_id, project_id, text, created_at, updated_at) " +
		"VALUES ($1, $2, $3, $4, 'remember', $5::timestamp, $5::timestamp)"
	adminListExec(t, memory, uuid.New().String(), f.user, f.teamA, f.projectA2, "2026-03-11 00:00:00")
	adminListExec(t, memory, uuid.New().String(), f.user, f.teamB, f.projectB, "2026-03-12 23:59:59")

	// Pin every aware creation time to insightsDay (updated_at == created_at, so
	// no update events), then shape the two prompts under test.
	for _, stmt := range []string{
		"UPDATE prompts SET created_at = $2, updated_at = $2 WHERE user_id = $1",
		"UPDATE artifacts SET created_at = $2, updated_at = $2 WHERE user_id = $1",
		"UPDATE blueprints SET created_at = $2, updated_at = $2 WHERE user_id = $1",
		"UPDATE agents SET created_at = $2, updated_at = $2 WHERE user_id = $1",
		"UPDATE feeds SET created_at = $2, updated_at = $2 WHERE created_by_user_id = $1",
		"UPDATE comments SET created_at = $2, updated_at = $2 WHERE user_id = $1",
		"UPDATE attachments SET created_at = $2 WHERE user_id = $1",
	} {
		_, err := integrationDB.ExecContext(ctx, stmt, f.user, insightsDay)
		require.NoError(t, err)
	}
	adminListExec(t, "UPDATE prompts SET updated_at = created_at + interval '500 milliseconds' WHERE id = $1",
		f.phantomPrompt)
	adminListExec(t, "UPDATE prompts SET updated_at = created_at + interval '2 days' WHERE id = $1", f.edited)

	// 12 authored resources → 12 created events, plus the one real edit.
	f.expectedEvents = 13
	f.expectedUpdatedResources = []string{f.edited}
	return f
}

func TestAdminUserInsights_UserExists(t *testing.T) {
	repo := NewAdminRepository(integrationDB)
	f := seedAdminInsightsFixture(t)

	ok, err := repo.UserExists(context.Background(), f.user)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = repo.UserExists(context.Background(), uuid.New().String())
	require.NoError(t, err)
	assert.False(t, ok)
}

// TestAdminUserInsights_CountsSumToTotals pins every counted type, the
// exclusion of a teammate's rows, the is_member flag, and that per-team counts
// sum to totals and per-project counts sum to their team's count.
func TestAdminUserInsights_CountsSumToTotals(t *testing.T) {
	repo := NewAdminRepository(integrationDB)
	f := seedAdminInsightsFixture(t)

	rows, err := repo.GetUserResourceCounts(context.Background(), f.user)
	require.NoError(t, err)

	type key struct{ typ, team, project string }
	byKey := make(map[key]int64)
	totals := make(map[string]int64)
	perTeam := make(map[string]map[string]int64)
	perProjectSum := make(map[string]map[string]int64) // team → type → sum over projects
	membership := make(map[string]bool)
	for _, r := range rows {
		project := ""
		if r.ProjectID != nil {
			project = *r.ProjectID
			if perProjectSum[r.TeamID] == nil {
				perProjectSum[r.TeamID] = make(map[string]int64)
			}
			perProjectSum[r.TeamID][r.ResourceType] += r.Count
			require.NotNil(t, r.ProjectName)
		}
		byKey[key{r.ResourceType, r.TeamID, project}] += r.Count
		totals[r.ResourceType] += r.Count
		if perTeam[r.TeamID] == nil {
			perTeam[r.TeamID] = make(map[string]int64)
		}
		perTeam[r.TeamID][r.ResourceType] += r.Count
		membership[r.TeamID] = r.IsMember
	}

	assert.Equal(t, map[string]int64{
		"prompt": 3, "memory": 2, "artifact": 1, "blueprint": 1, "agent": 1,
		"feed": 1, "feed_item": 1, "comment": 1, "attachment": 1,
	}, totals, "the teammate's prompt must not be counted")

	assert.Equal(t, int64(2), byKey[key{"prompt", f.teamA, f.projectA1}])
	assert.Equal(t, int64(1), byKey[key{"prompt", f.teamB, f.projectB}])
	assert.Equal(t, int64(1), byKey[key{"memory", f.teamA, f.projectA2}])
	assert.Equal(t, int64(1), byKey[key{"feed_item", f.teamA, ""}],
		"feed items are counted per team only, even with a project_id")

	assert.True(t, membership[f.teamA])
	assert.False(t, membership[f.teamB], "authored resources in a team the user is not in still count")

	// For every type the per-team counts sum to the totals.
	for typ, total := range totals {
		var sum int64
		for _, counts := range perTeam {
			sum += counts[typ]
		}
		assert.Equal(t, total, sum, "per-team %s", typ)
	}
	// For the four project-scoped types the per-project counts sum to the team's.
	for team, counts := range perTeam {
		for _, typ := range []string{"prompt", "memory", "artifact", "blueprint"} {
			assert.Equal(t, counts[typ], perProjectSum[team][typ], "per-project %s in team %s", typ, team)
		}
	}
}

func TestAdminUserInsights_UnknownUserHasNoRows(t *testing.T) {
	repo := NewAdminRepository(integrationDB)
	rows, err := repo.GetUserResourceCounts(context.Background(), uuid.New().String())
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// TestAdminUserInsights_CreationSeries checks the per-type buckets, including
// the naive memories rows on both window edges and feed items on posted_at.
func TestAdminUserInsights_CreationSeries(t *testing.T) {
	repo := NewAdminRepository(integrationDB)
	f := seedAdminInsightsFixture(t)

	from := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 13, 0, 0, 0, 0, time.UTC)
	rows, err := repo.GetUserCreationSeries(context.Background(), f.user, from, to, "day")
	require.NoError(t, err)

	got := make(map[string]int64)
	for _, r := range rows {
		got[fmt.Sprintf("%s@%s", r.Entity, r.Bucket.UTC().Format("2006-01-02"))] = r.Count
	}
	assert.Equal(t, map[string]int64{
		"prompt@2026-03-10":     3,
		"artifact@2026-03-10":   1,
		"blueprint@2026-03-10":  1,
		"agent@2026-03-10":      1,
		"feed@2026-03-10":       1,
		"comment@2026-03-10":    1,
		"attachment@2026-03-10": 1,
		"memory@2026-03-11":     1,
		"feed_item@2026-03-11":  1,
		"memory@2026-03-12":     1,
	}, got)

	// A window that starts exactly at the naive memory's instant includes it,
	// and one that ends there excludes it.
	edge := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	rows, err = repo.GetUserCreationSeries(context.Background(), f.user, edge, edge.Add(time.Hour), "day")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "memory", rows[0].Entity)
	rows, err = repo.GetUserCreationSeries(context.Background(), f.user, edge.Add(-time.Hour), edge, "day")
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// TestAdminUserInsights_TimelinePaging pages with limit 2 across many events
// that share one occurred_at, and checks every event appears exactly once, in
// descending order, with no phantom update.
func TestAdminUserInsights_TimelinePaging(t *testing.T) {
	repo := NewAdminRepository(integrationDB)
	f := seedAdminInsightsFixture(t)
	ctx := context.Background()

	all, err := repo.ListUserTimeline(ctx, f.user, nil, 1000)
	require.NoError(t, err)
	require.Len(t, all, f.expectedEvents)

	updated := make([]string, 0)
	for _, ev := range all {
		if ev.Action == models.AdminTimelineActionUpdated {
			updated = append(updated, ev.ResourceID)
		}
		assert.NotEqual(t, "", ev.TeamName)
	}
	assert.Equal(t, f.expectedUpdatedResources, updated, "a 500ms updated_at gap is not an edit")

	var paged []models.AdminUserTimelineEvent
	var cursor *models.AdminTimelineCursor
	for range 20 {
		page, pageErr := repo.ListUserTimeline(ctx, f.user, cursor, 2)
		require.NoError(t, pageErr)
		paged = append(paged, page...)
		if len(page) < 2 {
			break
		}
		last := page[len(page)-1]
		cursor = &models.AdminTimelineCursor{
			OccurredAt: last.OccurredAt, ResourceType: last.ResourceType,
			Action: last.Action, ResourceID: last.ResourceID,
		}
	}

	require.Len(t, paged, len(all))
	seen := make(map[string]bool)
	for i, ev := range paged {
		k := ev.ResourceType + "/" + ev.Action + "/" + ev.ResourceID
		assert.False(t, seen[k], "event %s repeated across pages", k)
		seen[k] = true
		assert.Equal(t, all[i], ev, "paging must reproduce the unpaged order")
		if i > 0 {
			assert.False(t, ev.OccurredAt.After(paged[i-1].OccurredAt), "timeline must be newest first")
		}
	}

	// The newest event is the team-B memory (naive, normalized to UTC), then
	// the genuine edit; the feed item keeps its project.
	assert.Equal(t, models.AdminResourceTypeMemory, all[0].ResourceType)
	assert.True(t, all[0].OccurredAt.Equal(time.Date(2026, 3, 12, 23, 59, 59, 0, time.UTC)), all[0].OccurredAt)
	assert.Equal(t, f.edited, all[1].ResourceID)
	assert.Equal(t, models.AdminTimelineActionUpdated, all[1].Action)
	for _, ev := range all {
		if ev.ResourceType == models.AdminResourceTypeFeedItem {
			require.NotNil(t, ev.ProjectID)
			assert.Equal(t, f.projectA1, *ev.ProjectID)
		}
		if ev.ResourceType == models.AdminResourceTypeAgent {
			assert.Nil(t, ev.ProjectID)
		}
	}
}
