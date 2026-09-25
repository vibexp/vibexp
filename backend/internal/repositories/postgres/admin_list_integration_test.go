//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// The admin user listing is instance-wide over a shared database, so every
// assertion here is scoped by a per-test search token embedded in the seeded
// emails rather than by truncating tables other tests rely on.

// adminListFixture is the seeded population for the #1133 aggregate tests.
type adminListFixture struct {
	token string
	heavy string // authored at least one of every resource type
	light string // one prompt only
	empty string // nothing at all
}

// insertAdminListUser seeds a user whose email carries token (for scoping) and
// label (for identification), returning its id.
func insertAdminListUser(t *testing.T, token, label string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO users (id, email, name) VALUES ($1, $2, $3)",
		id, token+"-"+label+"@admin-list.test", "Admin List "+label)
	require.NoError(t, err)
	return id
}

// adminListExec runs one fixture statement.
func adminListExec(t *testing.T, query string, args ...any) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), query, args...)
	require.NoError(t, err)
}

// seedAdminListFixture seeds three users with known per-type counts:
//
//	heavy: 2 teams, 2 projects, 3 prompts, 1 of every other type but 2 feed items
//	light: 1 team (its own), 1 prompt
//	empty: nothing
//
// plus one attachment with a NULL author, which must count for nobody.
func seedAdminListFixture(t *testing.T) adminListFixture {
	t.Helper()
	f := adminListFixture{token: "al" + uuid.New().String()[:8]}
	f.heavy = insertAdminListUser(t, f.token, "heavy")
	f.light = insertAdminListUser(t, f.token, "light")
	f.empty = insertAdminListUser(t, f.token, "empty")

	teamA := insertTestTeam(t, f.heavy)
	teamB := insertTestTeam(t, f.light)
	membership := "INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)"
	adminListExec(t, membership, teamA, f.heavy, "owner")
	adminListExec(t, membership, teamB, f.heavy, "member")
	adminListExec(t, membership, teamB, f.light, "owner")

	projectA := insertTestProject(t, f.heavy, teamA)
	insertTestProject(t, f.heavy, teamA)

	for range 3 {
		insertTestPrompt(t, f.heavy, teamA, projectA, "p", "body", "published")
	}
	// light authors a prompt in heavy's project: projects count by creator, so
	// this adds a prompt to light without adding a project.
	insertTestPrompt(t, f.light, teamA, projectA, "p", "body", "published")
	insertTestMemory(t, f.heavy, teamA, projectA, "remember")
	artifactID := insertTestArtifact(t, f.heavy, teamA, projectA, "a", "content", "active")
	adminListExec(t, "INSERT INTO blueprints (id, user_id, team_id, project_id, slug, title, content, path) "+
		"VALUES ($1, $2, $3, $4, $5, 'b', 'content', 'CLAUDE.md')",
		uuid.New().String(), f.heavy, teamA, projectA, "bp-"+uuid.New().String()[:8])
	insertTestAgent(t, f.heavy, teamA)

	feedID := uuid.New().String()
	adminListExec(t, "INSERT INTO feeds (id, team_id, name, created_by_user_id) VALUES ($1, $2, 'feed', $3)",
		feedID, teamA, f.heavy)
	for range 2 {
		adminListExec(t, "INSERT INTO feed_items "+
			"(id, team_id, feed_id, title, content, excerpt, ai_assistant_name, posted_by_user_id) "+
			"VALUES ($1, $2, $3, 'item', 'content', 'excerpt', 'Claude Code', $4)",
			uuid.New().String(), teamA, feedID, f.heavy)
	}
	adminListExec(t, "INSERT INTO comments (team_id, resource_type, resource_id, user_id, content) "+
		"VALUES ($1, 'artifact', $2, $3, 'nice')", teamA, artifactID, f.heavy)
	attachment := "INSERT INTO attachments " +
		"(team_id, user_id, owner_type, owner_id, file_name, content_type, size_bytes, gcs_object_key) " +
		"VALUES ($1, $2, 'artifact', $3, $4, 'text/plain', 1, $4)"
	adminListExec(t, attachment, teamA, f.heavy, artifactID, "a-"+uuid.New().String())
	adminListExec(t, attachment, teamA, nil, artifactID, "orphan-"+uuid.New().String())

	return f
}

// listAdminUsersByToken lists every user of the fixture (one page of 100).
func listAdminUsersByToken(
	t *testing.T, repo repositories.AdminRepository, token string, filters repositories.AdminUserFilters,
) ([]models.AdminUserListItem, int) {
	t.Helper()
	filters.Search = &token
	if filters.Limit == 0 {
		filters.Page, filters.Limit = 1, 100
	}
	users, total, err := repo.ListUsers(context.Background(), filters)
	require.NoError(t, err)
	return users, total
}

func adminListIDs(users []models.AdminUserListItem) []string {
	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	return ids
}

func adminListByID(users []models.AdminUserListItem) map[string]models.AdminUserListItem {
	out := make(map[string]models.AdminUserListItem, len(users))
	for _, u := range users {
		out[u.ID] = u
	}
	return out
}

func int64p(v int64) *int64 { return &v }

// TestAdminUserList_Counts pins every aggregate against the seeded fixture,
// including that an author-less attachment counts for nobody and a user with no
// resources has no last_resource_created_at.
func TestAdminUserList_Counts(t *testing.T) {
	f := seedAdminListFixture(t)
	repo := NewAdminRepository(integrationDB)

	users, total := listAdminUsersByToken(t, repo, f.token, repositories.AdminUserFilters{})
	require.Equal(t, 3, total)
	byID := adminListByID(users)

	heavy := byID[f.heavy]
	assert.Equal(t, int64(2), heavy.TeamCount)
	assert.Equal(t, int64(2), heavy.ProjectCount)
	assert.Equal(t, models.AdminResourceCounts{
		Prompts: 3, Memories: 1, Artifacts: 1, Blueprints: 1, Agents: 1,
		Feeds: 1, FeedItems: 2, Comments: 1, Attachments: 1, Total: 12,
	}, heavy.ResourceCounts)
	require.NotNil(t, heavy.LastResourceCreatedAt)

	light := byID[f.light]
	assert.Equal(t, int64(1), light.TeamCount)
	assert.Equal(t, models.AdminResourceCounts{Prompts: 1, Total: 1}, light.ResourceCounts)
	require.NotNil(t, light.LastResourceCreatedAt)

	empty := byID[f.empty]
	assert.Equal(t, int64(0), empty.TeamCount)
	assert.Equal(t, models.AdminResourceCounts{}, empty.ResourceCounts)
	assert.Nil(t, empty.LastResourceCreatedAt)
}

// TestAdminUserList_CountFilters asserts each count range narrows the listing to
// exactly the users inside it, with bounds inclusive.
func TestAdminUserList_CountFilters(t *testing.T) {
	f := seedAdminListFixture(t)
	repo := NewAdminRepository(integrationDB)
	atLeastOne := repositories.AdminCountRange{Min: int64p(1)}

	tests := []struct {
		name    string
		filters repositories.AdminUserFilters
		want    []string
	}{
		{"team_count >= 2", repositories.AdminUserFilters{TeamCount: repositories.AdminCountRange{Min: int64p(2)}}, []string{f.heavy}},
		{"team_count == 1", repositories.AdminUserFilters{TeamCount: repositories.AdminCountRange{Min: int64p(1), Max: int64p(1)}}, []string{f.light}},
		{"project_count >= 1", repositories.AdminUserFilters{ProjectCount: atLeastOne}, []string{f.heavy}},
		{"prompt_count >= 1", repositories.AdminUserFilters{PromptCount: atLeastOne}, []string{f.heavy, f.light}},
		{"prompt_count in [1,2]", repositories.AdminUserFilters{PromptCount: repositories.AdminCountRange{Min: int64p(1), Max: int64p(2)}}, []string{f.light}},
		{"memory_count >= 1", repositories.AdminUserFilters{MemoryCount: atLeastOne}, []string{f.heavy}},
		{"artifact_count >= 1", repositories.AdminUserFilters{ArtifactCount: atLeastOne}, []string{f.heavy}},
		{"blueprint_count >= 1", repositories.AdminUserFilters{BlueprintCount: atLeastOne}, []string{f.heavy}},
		{"agent_count >= 1", repositories.AdminUserFilters{AgentCount: atLeastOne}, []string{f.heavy}},
		{"feed_count >= 1", repositories.AdminUserFilters{FeedCount: atLeastOne}, []string{f.heavy}},
		{"feed_item_count >= 2", repositories.AdminUserFilters{FeedItemCount: repositories.AdminCountRange{Min: int64p(2)}}, []string{f.heavy}},
		{"comment_count >= 1", repositories.AdminUserFilters{CommentCount: atLeastOne}, []string{f.heavy}},
		{"attachment_count >= 1", repositories.AdminUserFilters{AttachmentCount: atLeastOne}, []string{f.heavy}},
		{"total_resource_count <= 1", repositories.AdminUserFilters{TotalResourceCount: repositories.AdminCountRange{Max: int64p(1)}}, []string{f.light, f.empty}},
		{"total_resource_count == 12", repositories.AdminUserFilters{TotalResourceCount: repositories.AdminCountRange{Min: int64p(12), Max: int64p(12)}}, []string{f.heavy}},
		{"memory_count == 0 keeps users without memories", repositories.AdminUserFilters{MemoryCount: repositories.AdminCountRange{Max: int64p(0)}}, []string{f.light, f.empty}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			users, total := listAdminUsersByToken(t, repo, f.token, tc.filters)
			assert.ElementsMatch(t, tc.want, adminListIDs(users))
			assert.Equal(t, len(tc.want), total)
		})
	}
}

// TestAdminUserList_TotalMatchesPagedRows is the envelope invariant under mixed
// count and date filters: total_count equals the rows reached by paging to the
// end, one row per page.
func TestAdminUserList_TotalMatchesPagedRows(t *testing.T) {
	f := seedAdminListFixture(t)
	repo := NewAdminRepository(integrationDB)
	epoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	filters := repositories.AdminUserFilters{
		PromptCount:             repositories.AdminCountRange{Min: int64p(1)},
		TotalResourceCount:      repositories.AdminCountRange{Max: int64p(100)},
		LastResourceCreatedFrom: &epoch,
		SortBy:                  "total_resource_count",
		SortOrder:               "desc",
	}

	var paged []string
	total := -1
	for page := 1; page <= 10; page++ {
		filters.Page, filters.Limit = page, 1
		users, pageTotal := listAdminUsersByToken(t, repo, f.token, filters)
		if total == -1 {
			total = pageTotal
		}
		require.Equal(t, total, pageTotal, "total_count must not change between pages")
		if len(users) == 0 {
			break
		}
		paged = append(paged, adminListIDs(users)...)
	}

	assert.Equal(t, 2, total)
	assert.Equal(t, []string{f.heavy, f.light}, paged)
}

// TestAdminUserList_SortByEveryKey sorts by each aggregate key in both
// directions. heavy >= light >= empty on every count, with ties broken by id;
// last_resource_created_at keeps the resource-less user last either way.
func TestAdminUserList_SortByEveryKey(t *testing.T) {
	f := seedAdminListFixture(t)
	repo := NewAdminRepository(integrationDB)

	// Per key: which of (heavy, light, empty) have a non-zero value. Users sharing
	// a value are ordered by id in both directions (the ", u.id" tie-breaker).
	keys := map[string][3]int64{
		"team_count":           {2, 1, 0},
		"project_count":        {2, 0, 0},
		"prompt_count":         {3, 1, 0},
		"memory_count":         {1, 0, 0},
		"artifact_count":       {1, 0, 0},
		"blueprint_count":      {1, 0, 0},
		"agent_count":          {1, 0, 0},
		"feed_count":           {1, 0, 0},
		"feed_item_count":      {2, 0, 0},
		"comment_count":        {1, 0, 0},
		"attachment_count":     {1, 0, 0},
		"total_resource_count": {12, 1, 0},
	}

	for key, vals := range keys {
		for _, dir := range []string{"asc", "desc"} {
			t.Run(key+" "+dir, func(t *testing.T) {
				users, _ := listAdminUsersByToken(t, repo, f.token,
					repositories.AdminUserFilters{SortBy: key, SortOrder: dir})
				assert.Equal(t, expectedAdminOrder(f, vals, dir), adminListIDs(users))
			})
		}
	}

	t.Run("last_resource_created_at keeps resource-less users last", func(t *testing.T) {
		for _, dir := range []string{"asc", "desc"} {
			users, _ := listAdminUsersByToken(t, repo, f.token,
				repositories.AdminUserFilters{SortBy: "last_resource_created_at", SortOrder: dir})
			ids := adminListIDs(users)
			require.Len(t, ids, 3)
			assert.Equal(t, f.empty, ids[2], "direction %s", dir)
		}
	})
}

// expectedAdminOrder orders the fixture's users by vals (heavy, light, empty) in
// dir, breaking ties by ascending id exactly as the ", u.id" tie-breaker does.
func expectedAdminOrder(f adminListFixture, vals [3]int64, dir string) []string {
	type row struct {
		id string
		v  int64
	}
	rows := []row{{f.heavy, vals[0]}, {f.light, vals[1]}, {f.empty, vals[2]}}
	less := func(a, b row) bool {
		if a.v != b.v {
			if dir == "asc" {
				return a.v < b.v
			}
			return a.v > b.v
		}
		return a.id < b.id
	}
	for i := range rows {
		for j := i + 1; j < len(rows); j++ {
			if less(rows[j], rows[i]) {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
	return []string{rows[0].id, rows[1].id, rows[2].id}
}

// TestAdminUserList_LastResourceCreatedRange_NaiveMemoryUnderNonUTCSession pins
// the memories.created_at (timestamp WITHOUT time zone) handling: a user whose
// only resource is a memory written exactly at the window edge must match an
// inclusive bound at that instant — and not one a second later — even when the
// session timezone is far from UTC. A naive column compared against an aware
// placeholder would be shifted by the session offset (13h in Auckland) and fail.
func TestAdminUserList_LastResourceCreatedRange_NaiveMemoryUnderNonUTCSession(t *testing.T) {
	token := "am" + uuid.New().String()[:8]
	userID := insertAdminListUser(t, token, "memory-only")
	teamID := insertTestTeam(t, userID)
	projectID := insertTestProject(t, userID, teamID)
	edge := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	// Written as the UTC wall-clock the naive column stores on a UTC server.
	adminListExec(t, "INSERT INTO memories (id, user_id, team_id, project_id, text, created_at) "+
		"VALUES ($1, $2, $3, $4, 'edge', $5::timestamp)",
		uuid.New().String(), userID, teamID, projectID, edge.Format("2006-01-02 15:04:05"))

	repo := NewAdminRepository(openSessionTZDB(t, "Pacific/Auckland"))

	users, _ := listAdminUsersByToken(t, repo, token, repositories.AdminUserFilters{})
	require.Len(t, users, 1)
	require.NotNil(t, users[0].LastResourceCreatedAt)
	assert.True(t, edge.Equal(*users[0].LastResourceCreatedAt),
		"got %s, want %s", users[0].LastResourceCreatedAt.UTC(), edge)

	after := edge.Add(time.Second)
	before := edge.Add(-time.Second)
	tests := []struct {
		name     string
		from, to *time.Time
		want     int
	}{
		{"from at the edge is inclusive", &edge, nil, 1},
		{"to at the edge is inclusive", nil, &edge, 1},
		{"from one second later excludes it", &after, nil, 0},
		{"to one second earlier excludes it", nil, &before, 0},
		{"a window around it matches", &before, &after, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, total := listAdminUsersByToken(t, repo, token, repositories.AdminUserFilters{
				LastResourceCreatedFrom: tc.from, LastResourceCreatedTo: tc.to,
			})
			assert.Len(t, got, tc.want)
			assert.Equal(t, tc.want, total)
		})
	}
}

// TestAdminUserList_LastResourceCreatedRange_ExcludesResourceLessUsers asserts
// a user with no resources (NULL GREATEST) never matches a date bound.
func TestAdminUserList_LastResourceCreatedRange_ExcludesResourceLessUsers(t *testing.T) {
	f := seedAdminListFixture(t)
	repo := NewAdminRepository(integrationDB)
	epoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	future := time.Now().Add(24 * time.Hour)

	users, total := listAdminUsersByToken(t, repo, f.token, repositories.AdminUserFilters{
		LastResourceCreatedFrom: &epoch, LastResourceCreatedTo: &future,
	})
	assert.ElementsMatch(t, []string{f.heavy, f.light}, adminListIDs(users))
	assert.Equal(t, 2, total)
}
