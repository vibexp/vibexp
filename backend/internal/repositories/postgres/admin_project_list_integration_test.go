//go:build integration

package postgres

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// The admin project listing (#1143) is instance-wide over a shared database, so
// every assertion is scoped by a per-test token embedded in the seeded project
// names (search matches p.name), like the #1133 user and #1138 team tests.

// adminProjectListBase anchors every seeded timestamp.
var adminProjectListBase = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

// adminProjectListFixture is the seeded population for the #1143 project tests.
type adminProjectListFixture struct {
	token        string
	heavy        string // every project-scoped type, created by hOwner
	light        string // one prompt, created by lOwner
	empty        string // nothing, created by lOwner
	lightCreator string // email of light's and empty's creator (lower case)
	heavyLastAt  time.Time
	lightLastAt  time.Time
}

// insertAdminProjectListProject seeds a project named after token and label.
func insertAdminProjectListProject(t *testing.T, token, label, userID, teamID string) string {
	t.Helper()
	id := uuid.New().String()
	adminListExec(t, "INSERT INTO projects (id, user_id, team_id, name, slug) VALUES ($1, $2, $3, $4, $5)",
		id, userID, teamID, token+" "+label, token+"-"+label)
	return id
}

// seedAdminProjectListFixture seeds three projects in one team:
//
//	heavy: 3 prompts, 1 memory, 1 artifact, 1 blueprint, 2 feed items; its most
//	       recent resource is the memory, whose created_at column is naive
//	light: 1 prompt
//	empty: nothing
//
// plus, in the same team, an agent, a feed, a comment and an attachment (none
// has a project_id, so none may count) and a feed item with a NULL project_id,
// which must count for no project.
func seedAdminProjectListFixture(t *testing.T) adminProjectListFixture {
	t.Helper()
	f := adminProjectListFixture{token: "apl" + uuid.New().String()[:8]}
	hOwner := insertAdminListUser(t, f.token, "hown")
	lOwner := insertAdminListUser(t, f.token, "lown")
	f.lightCreator = f.token + "-lown@admin-list.test"
	team := insertAdminTeamListTeam(t, f.token, "team", hOwner, adminProjectListBase)

	f.heavy = insertAdminProjectListProject(t, f.token, "heavy", hOwner, team)
	f.light = insertAdminProjectListProject(t, f.token, "light", lOwner, team)
	f.empty = insertAdminProjectListProject(t, f.token, "empty", lOwner, team)

	day := 24 * time.Hour
	f.heavyLastAt = adminProjectListBase.Add(5 * day)
	f.lightLastAt = adminProjectListBase.Add(4 * day)

	// prompts/artifacts/blueprints have no created_at trigger, so an UPDATE
	// pins their timestamps safely.
	for range 3 {
		id := insertTestPrompt(t, hOwner, team, f.heavy, "p", "body", "published")
		adminListExec(t, "UPDATE prompts SET created_at = $2, updated_at = $2 WHERE id = $1", id, adminProjectListBase.Add(day))
	}
	id := insertTestPrompt(t, lOwner, team, f.light, "p", "body", "published")
	adminListExec(t, "UPDATE prompts SET created_at = $2, updated_at = $2 WHERE id = $1", id, f.lightLastAt)

	artifactID := insertTestArtifact(t, hOwner, team, f.heavy, "a", "content", "active")
	adminListExec(t, "UPDATE artifacts SET created_at = $2, updated_at = $2 WHERE id = $1",
		artifactID, adminProjectListBase.Add(2*day))
	adminListExec(t, "INSERT INTO blueprints (id, user_id, team_id, project_id, slug, title, content, path, created_at) "+
		"VALUES ($1, $2, $3, $4, $5, 'b', 'content', 'CLAUDE.md', $6)",
		uuid.New().String(), hOwner, team, f.heavy, "bp-"+uuid.New().String()[:8], adminProjectListBase.Add(2*day))

	// memories.created_at is naive and has an updated_at trigger: INSERT the
	// UTC wall-clock value directly rather than UPDATE it.
	adminListExec(t, "INSERT INTO memories (id, user_id, team_id, project_id, text, created_at, updated_at) "+
		"VALUES ($1, $2, $3, $4, 'remember', $5::timestamp, $5::timestamp)",
		uuid.New().String(), hOwner, team, f.heavy, f.heavyLastAt.Format("2006-01-02 15:04:05"))

	feedID := uuid.New().String()
	adminListExec(t, "INSERT INTO feeds (id, team_id, name, created_by_user_id) VALUES ($1, $2, 'feed', $3)",
		feedID, team, hOwner)
	feedItem := "INSERT INTO feed_items " +
		"(id, team_id, feed_id, project_id, title, content, excerpt, ai_assistant_name, posted_by_user_id, posted_at) " +
		"VALUES ($1, $2, $3, $4, 'item', 'content', 'excerpt', 'Claude Code', $5, $6)"
	for range 2 {
		adminListExec(t, feedItem, uuid.New().String(), team, feedID, f.heavy, hOwner, adminProjectListBase.Add(3*day))
	}
	// Posted without a project, and later than everything else: it must move
	// neither a count nor a last_resource_created_at.
	adminListExec(t, feedItem, uuid.New().String(), team, feedID, nil, hOwner, adminProjectListBase.Add(9*day))

	insertTestAgent(t, hOwner, team)
	adminListExec(t, "INSERT INTO comments (team_id, resource_type, resource_id, user_id, content) "+
		"VALUES ($1, 'artifact', $2, $3, 'nice')", team, artifactID, hOwner)
	adminListExec(t, "INSERT INTO attachments "+
		"(team_id, user_id, owner_type, owner_id, file_name, content_type, size_bytes, gcs_object_key) "+
		"VALUES ($1, $2, 'artifact', $3, $4, 'text/plain', 1, $4)", team, hOwner, artifactID, "a-"+uuid.New().String())
	return f
}

// listAdminProjectsByToken lists every project of the fixture (one page of 100
// unless the filters page explicitly).
func listAdminProjectsByToken(
	t *testing.T, repo repositories.AdminRepository, token string, filters repositories.AdminProjectFilters,
) ([]models.AdminProjectListItem, int) {
	t.Helper()
	filters.Search = &token
	if filters.Limit == 0 {
		filters.Page, filters.Limit = 1, 100
	}
	projects, total, err := repo.ListProjects(context.Background(), filters)
	require.NoError(t, err)
	return projects, total
}

func adminProjectIDs(projects []models.AdminProjectListItem) []string {
	ids := make([]string, 0, len(projects))
	for _, p := range projects {
		ids = append(ids, p.ID)
	}
	return ids
}

// TestAdminProjectList_CountsMatchDetail pins every aggregate against the
// fixture — excluded types and the project-less feed item count for nothing,
// the naive memory timestamp reads as UTC — and requires the list row's counts
// to equal GET /admin/projects/{id}'s for every project.
func TestAdminProjectList_CountsMatchDetail(t *testing.T) {
	f := seedAdminProjectListFixture(t)
	repo := NewAdminRepository(integrationDB)

	projects, total := listAdminProjectsByToken(t, repo, f.token, repositories.AdminProjectFilters{})
	require.Equal(t, 3, total)
	require.Len(t, projects, 3)
	byID := make(map[string]models.AdminProjectListItem, len(projects))
	for _, p := range projects {
		byID[p.ID] = p
	}

	heavy := byID[f.heavy]
	assert.Equal(t, models.AdminProjectResourceCounts{
		Prompts: 3, Memories: 1, Artifacts: 1, Blueprints: 1, FeedItems: 2, Total: 8,
	}, heavy.ResourceCounts)
	require.NotNil(t, heavy.LastResourceCreatedAt)
	assert.True(t, f.heavyLastAt.Equal(*heavy.LastResourceCreatedAt), "got %s", heavy.LastResourceCreatedAt)

	light := byID[f.light]
	assert.Equal(t, models.AdminProjectResourceCounts{Prompts: 1, Total: 1}, light.ResourceCounts)
	require.NotNil(t, light.LastResourceCreatedAt)
	assert.True(t, f.lightLastAt.Equal(*light.LastResourceCreatedAt))

	empty := byID[f.empty]
	assert.Equal(t, models.AdminProjectResourceCounts{}, empty.ResourceCounts)
	assert.Nil(t, empty.LastResourceCreatedAt)

	for _, p := range projects {
		detail, err := repo.GetProjectDetail(context.Background(), p.ID)
		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Equal(t, p.ResourceCounts, detail.ResourceCounts, "list and detail disagree for %s", p.Name)
	}
}

// TestAdminProjectList_RangeFilters covers every count range alone and combined
// with other predicates; total always equals the returned rows.
func TestAdminProjectList_RangeFilters(t *testing.T) {
	f := seedAdminProjectListFixture(t)
	repo := NewAdminRepository(integrationDB)
	rng := func(lo, hi *int64) repositories.AdminCountRange {
		return repositories.AdminCountRange{Min: lo, Max: hi}
	}

	tests := []struct {
		name    string
		filters repositories.AdminProjectFilters
		want    []string
	}{
		{"prompt_count >= 2", repositories.AdminProjectFilters{PromptCount: rng(int64p(2), nil)}, []string{f.heavy}},
		{"prompt_count <= 0", repositories.AdminProjectFilters{PromptCount: rng(nil, int64p(0))}, []string{f.empty}},
		{"memory_count >= 1", repositories.AdminProjectFilters{MemoryCount: rng(int64p(1), nil)}, []string{f.heavy}},
		{"artifact_count 1..1", repositories.AdminProjectFilters{ArtifactCount: rng(int64p(1), int64p(1))}, []string{f.heavy}},
		{"blueprint_count <= 0", repositories.AdminProjectFilters{BlueprintCount: rng(nil, int64p(0))}, []string{f.light, f.empty}},
		{"feed_item_count 2..2", repositories.AdminProjectFilters{FeedItemCount: rng(int64p(2), int64p(2))}, []string{f.heavy}},
		{"total_resource_count 1..1", repositories.AdminProjectFilters{TotalResourceCount: rng(int64p(1), int64p(1))}, []string{f.light}},
		{"total_resource_count >= 8", repositories.AdminProjectFilters{TotalResourceCount: rng(int64p(8), nil)}, []string{f.heavy}},
		{"total_resource_count >= 9", repositories.AdminProjectFilters{TotalResourceCount: rng(int64p(9), nil)}, nil},
		{
			"combined with owner_email and a date bound",
			repositories.AdminProjectFilters{
				OwnerEmail: &f.lightCreator, PromptCount: rng(int64p(1), nil),
				LastResourceCreatedFrom: &f.lightLastAt,
			},
			[]string{f.light},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.filters.SortBy, tc.filters.SortOrder = "name", "desc"
			projects, total := listAdminProjectsByToken(t, repo, f.token, tc.filters)
			assert.ElementsMatch(t, tc.want, adminProjectIDs(projects))
			assert.Equal(t, len(tc.want), total)
		})
	}
}

// TestAdminProjectList_OwnerEmail matches the creator case-insensitively and
// exactly (never a substring), and never the team's owner.
func TestAdminProjectList_OwnerEmail(t *testing.T) {
	f := seedAdminProjectListFixture(t)
	repo := NewAdminRepository(integrationDB)

	for _, email := range []string{f.lightCreator, strings.ToUpper(f.lightCreator)} {
		projects, total := listAdminProjectsByToken(t, repo, f.token, repositories.AdminProjectFilters{OwnerEmail: &email})
		assert.ElementsMatch(t, []string{f.light, f.empty}, adminProjectIDs(projects), email)
		assert.Equal(t, 2, total)
	}

	partial := f.token + "-lown"
	projects, total := listAdminProjectsByToken(t, repo, f.token, repositories.AdminProjectFilters{OwnerEmail: &partial})
	assert.Empty(t, projects)
	assert.Equal(t, 0, total)
}

// TestAdminProjectList_LastResourceCreated covers the inclusive bounds; a
// project with no resources never matches, and the project-less feed item
// (posted latest of all) moves no project's value.
func TestAdminProjectList_LastResourceCreated(t *testing.T) {
	f := seedAdminProjectListFixture(t)
	repo := NewAdminRepository(integrationDB)
	farFuture := adminProjectListBase.Add(365 * 24 * time.Hour)
	farPast := adminProjectListBase.Add(-365 * 24 * time.Hour)
	afterLight := f.lightLastAt.Add(time.Second)
	afterHeavy := f.heavyLastAt.Add(time.Second)

	tests := []struct {
		name     string
		from, to *time.Time
		want     []string
	}{
		{"from is inclusive", &f.heavyLastAt, nil, []string{f.heavy}},
		{"from excludes older", &afterLight, nil, []string{f.heavy}},
		{"from past the newest resource matches nothing", &afterHeavy, nil, nil},
		{"to is inclusive", nil, &f.lightLastAt, []string{f.light}},
		{"exact single instant", &f.lightLastAt, &f.lightLastAt, []string{f.light}},
		{"wide range excludes the resourceless project", &farPast, &farFuture, []string{f.heavy, f.light}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projects, total := listAdminProjectsByToken(t, repo, f.token, repositories.AdminProjectFilters{
				LastResourceCreatedFrom: tc.from, LastResourceCreatedTo: tc.to,
			})
			assert.ElementsMatch(t, tc.want, adminProjectIDs(projects))
			assert.Equal(t, len(tc.want), total)
		})
	}
}

// TestAdminProjectList_TotalMatchesPagedRows pages a filtered set one row at a
// time to the end: total_count must equal the number of rows actually reachable.
func TestAdminProjectList_TotalMatchesPagedRows(t *testing.T) {
	f := seedAdminProjectListFixture(t)
	repo := NewAdminRepository(integrationDB)

	for _, filters := range []repositories.AdminProjectFilters{
		{},
		{OwnerEmail: &f.lightCreator},
		{TotalResourceCount: repositories.AdminCountRange{Min: int64p(1)}, SortBy: "feed_item_count", SortOrder: "asc"},
		{LastResourceCreatedTo: &f.heavyLastAt, SortBy: "last_resource_created_at", SortOrder: "desc"},
	} {
		var seen []string
		var reportedTotal int
		for page := 1; page <= 5; page++ {
			filters.Page, filters.Limit = page, 1
			projects, total := listAdminProjectsByToken(t, repo, f.token, filters)
			reportedTotal = total
			if len(projects) == 0 {
				break
			}
			seen = append(seen, adminProjectIDs(projects)...)
		}
		assert.Len(t, seen, reportedTotal)
		assert.Len(t, slices.Compact(slices.Sorted(slices.Values(seen))), reportedTotal, "no row repeats across pages")
	}
}

// TestAdminProjectList_SortByEveryKey sorts by every count key in both
// directions (ties broken by p.id), and by last_resource_created_at with the
// resourceless project last in both directions.
func TestAdminProjectList_SortByEveryKey(t *testing.T) {
	f := seedAdminProjectListFixture(t)
	repo := NewAdminRepository(integrationDB)

	// Values per key for [heavy, light, empty].
	keys := map[string][3]int64{
		"prompt_count":         {3, 1, 0},
		"memory_count":         {1, 0, 0},
		"artifact_count":       {1, 0, 0},
		"blueprint_count":      {1, 0, 0},
		"feed_item_count":      {2, 0, 0},
		"total_resource_count": {8, 1, 0},
	}
	fx := adminListFixture{heavy: f.heavy, light: f.light, empty: f.empty}
	for key, vals := range keys {
		for _, dir := range []string{"asc", "desc"} {
			t.Run(key+" "+dir, func(t *testing.T) {
				projects, _ := listAdminProjectsByToken(t, repo, f.token,
					repositories.AdminProjectFilters{SortBy: key, SortOrder: dir})
				assert.Equal(t, expectedAdminOrder(fx, vals, dir), adminProjectIDs(projects))
			})
		}
	}

	for dir, want := range map[string][]string{
		"desc": {f.heavy, f.light, f.empty},
		"asc":  {f.light, f.heavy, f.empty},
	} {
		t.Run("last_resource_created_at "+dir, func(t *testing.T) {
			projects, _ := listAdminProjectsByToken(t, repo, f.token,
				repositories.AdminProjectFilters{SortBy: "last_resource_created_at", SortOrder: dir})
			assert.Equal(t, want, adminProjectIDs(projects))
		})
	}
}
