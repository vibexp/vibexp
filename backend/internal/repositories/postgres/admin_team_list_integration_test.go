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

// The admin team listing (#1138) is instance-wide over a shared database, so
// every assertion is scoped by a per-test token embedded in the seeded team
// names (search matches t.name), exactly like the #1133 user tests.

// adminTeamListFixture is the seeded population for the #1138 team tests.
type adminTeamListFixture struct {
	token      string
	heavy      string // 4 members, every resource type, every setting configured
	light      string // 2 members, 1 project + 1 prompt, only fallback-shaped rows
	empty      string // no members (integrity drift), nothing at all
	heavyOwner string // email of heavy's owner, as seeded (lower case)
}

// insertAdminTeamListTeam seeds a team named after token and label, owned by
// ownerID, returning its id. No team_members row is added.
func insertAdminTeamListTeam(t *testing.T, token, label, ownerID string, createdAt time.Time) string {
	t.Helper()
	id := uuid.New().String()
	adminListExec(t, "INSERT INTO teams (id, owner_id, name, slug, created_at) VALUES ($1, $2, $3, $4, $5)",
		id, ownerID, token+" "+label, token+"-"+label, createdAt)
	return id
}

// seedAdminTeamListFixture seeds three teams with known counts and configuration:
//
//	heavy: members owner+admin+admin+member; 2 projects; 3 prompts and 1 of every
//	       other resource type but 2 feed items; 2 embedding providers, 2 model
//	       providers, AI summary enabled, an email provider, its own GitHub App
//	       with 2 installations, search settings, 2 enabled freshness rules.
//	light: members owner+member; 1 project, 1 prompt; AI summary row DISABLED,
//	       freshness settings with only a DISABLED rule, and a GitHub
//	       installation but no App of its own (exercises the OR branch).
//	empty: no members, no resources, nothing configured.
//
// plus a legacy embedding_providers row with team_id NULL, which must count for
// no team.
func seedAdminTeamListFixture(t *testing.T) adminTeamListFixture {
	t.Helper()
	f := adminTeamListFixture{token: "atl" + uuid.New().String()[:8]}
	hOwner := insertAdminListUser(t, f.token, "hown")
	lOwner := insertAdminListUser(t, f.token, "lown")
	eOwner := insertAdminListUser(t, f.token, "eown")
	admin1 := insertAdminListUser(t, f.token, "adm1")
	admin2 := insertAdminListUser(t, f.token, "adm2")
	member := insertAdminListUser(t, f.token, "mem")
	f.heavyOwner = f.token + "-hown@admin-list.test"

	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	f.heavy = insertAdminTeamListTeam(t, f.token, "heavy", hOwner, base)
	f.light = insertAdminTeamListTeam(t, f.token, "light", lOwner, base.Add(time.Hour))
	f.empty = insertAdminTeamListTeam(t, f.token, "empty", eOwner, base.Add(2*time.Hour))

	membership := "INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)"
	adminListExec(t, membership, f.heavy, hOwner, "owner")
	adminListExec(t, membership, f.heavy, admin1, "admin")
	adminListExec(t, membership, f.heavy, admin2, "admin")
	adminListExec(t, membership, f.heavy, member, "member")
	adminListExec(t, membership, f.light, lOwner, "owner")
	adminListExec(t, membership, f.light, member, "member")

	seedAdminTeamListResources(t, f.heavy, f.light, hOwner, lOwner)
	seedAdminTeamListConfiguration(t, f.heavy, f.light, hOwner)
	return f
}

// seedAdminTeamListResources seeds heavy's and light's projects and resources.
func seedAdminTeamListResources(t *testing.T, heavy, light, hOwner, lOwner string) {
	t.Helper()
	projectH := insertTestProject(t, hOwner, heavy)
	insertTestProject(t, hOwner, heavy)
	projectL := insertTestProject(t, lOwner, light)

	for range 3 {
		insertTestPrompt(t, hOwner, heavy, projectH, "p", "body", "published")
	}
	insertTestPrompt(t, lOwner, light, projectL, "p", "body", "published")
	insertTestMemory(t, hOwner, heavy, projectH, "remember")
	artifactID := insertTestArtifact(t, hOwner, heavy, projectH, "a", "content", "active")
	adminListExec(t, "INSERT INTO blueprints (id, user_id, team_id, project_id, slug, title, content, path) "+
		"VALUES ($1, $2, $3, $4, $5, 'b', 'content', 'CLAUDE.md')",
		uuid.New().String(), hOwner, heavy, projectH, "bp-"+uuid.New().String()[:8])
	insertTestAgent(t, hOwner, heavy)

	feedID := uuid.New().String()
	adminListExec(t, "INSERT INTO feeds (id, team_id, name, created_by_user_id) VALUES ($1, $2, 'feed', $3)",
		feedID, heavy, hOwner)
	for range 2 {
		adminListExec(t, "INSERT INTO feed_items "+
			"(id, team_id, feed_id, title, content, excerpt, ai_assistant_name, posted_by_user_id) "+
			"VALUES ($1, $2, $3, 'item', 'content', 'excerpt', 'Claude Code', $4)",
			uuid.New().String(), heavy, feedID, hOwner)
	}
	adminListExec(t, "INSERT INTO comments (team_id, resource_type, resource_id, user_id, content) "+
		"VALUES ($1, 'artifact', $2, $3, 'nice')", heavy, artifactID, hOwner)
	adminListExec(t, "INSERT INTO attachments "+
		"(team_id, user_id, owner_type, owner_id, file_name, content_type, size_bytes, gcs_object_key) "+
		"VALUES ($1, NULL, 'artifact', $2, $3, 'text/plain', 1, $3)", heavy, artifactID, "a-"+uuid.New().String())
}

// seedAdminTeamListConfiguration seeds the own-row settings described on
// seedAdminTeamListFixture. Multi-row sources get two rows on heavy so a join
// that fanned out would inflate counts and total_count.
func seedAdminTeamListConfiguration(t *testing.T, heavy, light, hOwner string) {
	t.Helper()
	embedding := "INSERT INTO embedding_providers (user_id, team_id, name, provider_type, model) " +
		"VALUES ($1, $2, $3, 'openai', 'text-embedding-3-small')"
	adminListExec(t, embedding, hOwner, heavy, "e1")
	adminListExec(t, embedding, hOwner, heavy, "e2")
	adminListExec(t, embedding, hOwner, nil, "legacy-"+uuid.New().String()[:8])

	model := "INSERT INTO model_providers (team_id, name, provider_type, model) VALUES ($1, $2, 'openai', 'gpt')"
	adminListExec(t, model, heavy, "m1")
	adminListExec(t, model, heavy, "m2")

	aiSummary := "INSERT INTO team_ai_summary_settings (team_id, enabled, top_n, style, max_output_tokens) " +
		"VALUES ($1, $2, 3, 'balanced', 500)"
	adminListExec(t, aiSummary, heavy, true)
	adminListExec(t, aiSummary, light, false)

	adminListExec(t, "INSERT INTO team_email_providers (team_id, provider_type, secret_encrypted, from_address) "+
		"VALUES ($1, 'smtp', 'enc', 'noreply@example.com')", heavy)

	appConfigID := insertTestGitHubAppConfig(t, heavy, uuid.New().String()[:12])
	installation := "INSERT INTO github_installations (team_id, app_config_id, installation_id, account_login, " +
		"account_type, target_type, encrypted_access_token, token_expires_at) " +
		"VALUES ($1, $2, $3, 'octo', 'Organization', 'Organization', 'enc', now())"
	adminListExec(t, installation, heavy, appConfigID, time.Now().UnixNano())
	adminListExec(t, installation, heavy, appConfigID, time.Now().UnixNano()+1)
	// light has an installation but no App of its own (the FK does not tie an
	// installation's App to its team), so only the gi half of the OR is true.
	adminListExec(t, installation, light, appConfigID, time.Now().UnixNano()+2)

	adminListExec(t, "INSERT INTO team_search_settings (team_id, recency_ranking_enabled, rank_weight_relevance, "+
		"rank_weight_created, rank_weight_updated, rank_half_life_days) VALUES ($1, true, 1, 0, 0, 30)", heavy)

	rule := "INSERT INTO freshness_rules (team_id, resource_types, threshold_days, enabled) " +
		"VALUES ($1, '{prompt}', 30, $2)"
	adminListExec(t, rule, heavy, true)
	adminListExec(t, rule, heavy, true)
	adminListExec(t, rule, light, false)
	adminListExec(t, "INSERT INTO team_freshness_settings (team_id, interval_seconds, reversibility_enabled) "+
		"VALUES ($1, 3600, false)", light)
}

// listAdminTeamsByToken lists every team of the fixture (one page of 100).
func listAdminTeamsByToken(
	t *testing.T, repo repositories.AdminRepository, token string, filters repositories.AdminTeamFilters,
) ([]models.AdminTeamListItem, int) {
	t.Helper()
	filters.Search = &token
	if filters.Limit == 0 {
		filters.Page, filters.Limit = 1, 100
	}
	teams, total, err := repo.ListTeams(context.Background(), filters)
	require.NoError(t, err)
	return teams, total
}

func adminTeamIDs(teams []models.AdminTeamListItem) []string {
	ids := make([]string, 0, len(teams))
	for _, tm := range teams {
		ids = append(ids, tm.ID)
	}
	return ids
}

func boolp(v bool) *bool { return &v }

// TestAdminTeamList_CountsAndConfiguration pins every aggregate and flag against
// the seeded fixture: multi-row sources do not fan out, the NULL-team legacy
// embedding provider counts for nobody, a disabled AI-summary row and a
// settings-only freshness setup read as not configured, and a team with no
// member rows reports zero owners.
func TestAdminTeamList_CountsAndConfiguration(t *testing.T) {
	f := seedAdminTeamListFixture(t)
	repo := NewAdminRepository(integrationDB)

	teams, total := listAdminTeamsByToken(t, repo, f.token, repositories.AdminTeamFilters{})
	require.Equal(t, 3, total)
	require.Len(t, teams, 3)
	byID := make(map[string]models.AdminTeamListItem, len(teams))
	for _, tm := range teams {
		byID[tm.ID] = tm
	}

	heavy := byID[f.heavy]
	assert.Equal(t, [4]int64{4, 1, 2, 2}, [4]int64{heavy.MemberCount, heavy.OwnerCount, heavy.AdminCount, heavy.ProjectCount})
	assert.Equal(t, models.AdminResourceCounts{
		Prompts: 3, Memories: 1, Artifacts: 1, Blueprints: 1, Agents: 1,
		Feeds: 1, FeedItems: 2, Comments: 1, Attachments: 1, Total: 12,
	}, heavy.ResourceCounts)
	assert.Equal(t, models.AdminTeamConfiguration{
		EmbeddingConfigured: true, LLMConfigured: true, AISummaryEnabled: true, EmailConfigured: true,
		GitHubConfigured: true, SearchSettingsCustomized: true, FreshnessEnabled: true,
	}, heavy.Configuration)

	light := byID[f.light]
	assert.Equal(t, [4]int64{2, 1, 0, 1}, [4]int64{light.MemberCount, light.OwnerCount, light.AdminCount, light.ProjectCount})
	assert.Equal(t, models.AdminResourceCounts{Prompts: 1, Total: 1}, light.ResourceCounts)
	assert.Equal(t, models.AdminTeamConfiguration{GitHubConfigured: true}, light.Configuration)

	empty := byID[f.empty]
	assert.Equal(t, [4]int64{0, 0, 0, 0}, [4]int64{empty.MemberCount, empty.OwnerCount, empty.AdminCount, empty.ProjectCount})
	assert.Equal(t, models.AdminResourceCounts{}, empty.ResourceCounts)
	assert.Equal(t, models.AdminTeamConfiguration{}, empty.Configuration)
}

// TestAdminTeamList_RangeFilters covers every count range alone and combined
// with other predicates; total always equals the returned rows.
func TestAdminTeamList_RangeFilters(t *testing.T) {
	f := seedAdminTeamListFixture(t)
	repo := NewAdminRepository(integrationDB)
	one := repositories.AdminCountRange{Min: int64p(1)}
	zero := repositories.AdminCountRange{Max: int64p(0)}

	tests := []struct {
		name    string
		filters repositories.AdminTeamFilters
		want    []string
	}{
		{"member_count 2..3", repositories.AdminTeamFilters{MemberCount: repositories.AdminCountRange{Min: int64p(2), Max: int64p(3)}}, []string{f.light}},
		{"member_count >= 3", repositories.AdminTeamFilters{MemberCount: repositories.AdminCountRange{Min: int64p(3)}}, []string{f.heavy}},
		{"owner_count = 0 surfaces drift", repositories.AdminTeamFilters{OwnerCount: zero}, []string{f.empty}},
		{"owner_count = 1", repositories.AdminTeamFilters{OwnerCount: repositories.AdminCountRange{Min: int64p(1), Max: int64p(1)}}, []string{f.heavy, f.light}},
		{"admin_count >= 2", repositories.AdminTeamFilters{AdminCount: repositories.AdminCountRange{Min: int64p(2)}}, []string{f.heavy}},
		{"project_count = 1", repositories.AdminTeamFilters{ProjectCount: repositories.AdminCountRange{Min: int64p(1), Max: int64p(1)}}, []string{f.light}},
		{"prompt_count >= 2", repositories.AdminTeamFilters{PromptCount: repositories.AdminCountRange{Min: int64p(2)}}, []string{f.heavy}},
		{"prompt_count = 0", repositories.AdminTeamFilters{PromptCount: zero}, []string{f.empty}},
		{"memory_count >= 1", repositories.AdminTeamFilters{MemoryCount: one}, []string{f.heavy}},
		{"artifact_count >= 1", repositories.AdminTeamFilters{ArtifactCount: one}, []string{f.heavy}},
		{"blueprint_count >= 1", repositories.AdminTeamFilters{BlueprintCount: one}, []string{f.heavy}},
		{"agent_count >= 1", repositories.AdminTeamFilters{AgentCount: one}, []string{f.heavy}},
		{"feed_count >= 1", repositories.AdminTeamFilters{FeedCount: one}, []string{f.heavy}},
		{"feed_item_count = 2", repositories.AdminTeamFilters{FeedItemCount: repositories.AdminCountRange{Min: int64p(2), Max: int64p(2)}}, []string{f.heavy}},
		{"comment_count >= 1", repositories.AdminTeamFilters{CommentCount: one}, []string{f.heavy}},
		{"attachment_count >= 1 (author-less still counts for the team)", repositories.AdminTeamFilters{AttachmentCount: one}, []string{f.heavy}},
		{"total_resource_count 1..12", repositories.AdminTeamFilters{TotalResourceCount: repositories.AdminCountRange{Min: int64p(1), Max: int64p(12)}}, []string{f.heavy, f.light}},
		{"total_resource_count <= 1", repositories.AdminTeamFilters{TotalResourceCount: repositories.AdminCountRange{Max: int64p(1)}}, []string{f.light, f.empty}},
		{
			name: "counts combined with a tri-state and is_personal",
			filters: repositories.AdminTeamFilters{
				MemberCount: one, TotalResourceCount: repositories.AdminCountRange{Max: int64p(5)},
				GitHubConfigured: boolp(true), IsPersonal: boolp(false),
			},
			want: []string{f.light},
		},
		{
			name:    "no team satisfies an over-tight combination",
			filters: repositories.AdminTeamFilters{AdminCount: one, ProjectCount: zero},
			want:    []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			teams, total := listAdminTeamsByToken(t, repo, f.token, tc.filters)
			assert.ElementsMatch(t, tc.want, adminTeamIDs(teams))
			assert.Equal(t, len(tc.want), total)
		})
	}
}

// TestAdminTeamList_TriStates checks each configured tri-state as true, false
// and absent: true returns only the matching teams, false exactly the rest, and
// absent narrows nothing.
func TestAdminTeamList_TriStates(t *testing.T) {
	f := seedAdminTeamListFixture(t)
	repo := NewAdminRepository(integrationDB)
	all := []string{f.heavy, f.light, f.empty}

	tests := []struct {
		name     string
		set      func(*repositories.AdminTeamFilters, *bool)
		wantTrue []string
	}{
		{"embedding_configured", func(fl *repositories.AdminTeamFilters, v *bool) { fl.EmbeddingConfigured = v }, []string{f.heavy}},
		{"llm_configured", func(fl *repositories.AdminTeamFilters, v *bool) { fl.LLMConfigured = v }, []string{f.heavy}},
		{"ai_summary_enabled (disabled row is false)", func(fl *repositories.AdminTeamFilters, v *bool) { fl.AISummaryEnabled = v }, []string{f.heavy}},
		{"email_configured", func(fl *repositories.AdminTeamFilters, v *bool) { fl.EmailConfigured = v }, []string{f.heavy}},
		{"github_configured (app or installation)", func(fl *repositories.AdminTeamFilters, v *bool) { fl.GitHubConfigured = v }, []string{f.heavy, f.light}},
		{"search_settings_customized", func(fl *repositories.AdminTeamFilters, v *bool) { fl.SearchSettingsCustomized = v }, []string{f.heavy}},
		{"freshness_enabled (settings-only is false)", func(fl *repositories.AdminTeamFilters, v *bool) { fl.FreshnessEnabled = v }, []string{f.heavy}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var trueF, falseF, absentF repositories.AdminTeamFilters
			tc.set(&trueF, boolp(true))
			tc.set(&falseF, boolp(false))
			tc.set(&absentF, nil)

			teams, total := listAdminTeamsByToken(t, repo, f.token, trueF)
			assert.ElementsMatch(t, tc.wantTrue, adminTeamIDs(teams), "true")
			assert.Equal(t, len(tc.wantTrue), total)

			wantFalse := slices.DeleteFunc(slices.Clone(all), func(id string) bool {
				return slices.Contains(tc.wantTrue, id)
			})
			teams, total = listAdminTeamsByToken(t, repo, f.token, falseF)
			assert.ElementsMatch(t, wantFalse, adminTeamIDs(teams), "false")
			assert.Equal(t, len(wantFalse), total)

			teams, total = listAdminTeamsByToken(t, repo, f.token, absentF)
			assert.ElementsMatch(t, all, adminTeamIDs(teams), "absent")
			assert.Equal(t, 3, total)
		})
	}

	t.Run("tri-states combine with AND", func(t *testing.T) {
		teams, total := listAdminTeamsByToken(t, repo, f.token, repositories.AdminTeamFilters{
			GitHubConfigured: boolp(true), EmbeddingConfigured: boolp(false), FreshnessEnabled: boolp(false),
		})
		assert.Equal(t, []string{f.light}, adminTeamIDs(teams))
		assert.Equal(t, 1, total)
	})
}

// TestAdminTeamList_OwnerEmail matches the owner's email exactly and
// case-insensitively; a substring is not a match.
func TestAdminTeamList_OwnerEmail(t *testing.T) {
	f := seedAdminTeamListFixture(t)
	repo := NewAdminRepository(integrationDB)

	for _, email := range []string{f.heavyOwner, strings.ToUpper(f.heavyOwner)} {
		teams, total := listAdminTeamsByToken(t, repo, f.token, repositories.AdminTeamFilters{OwnerEmail: &email})
		assert.Equal(t, []string{f.heavy}, adminTeamIDs(teams), email)
		assert.Equal(t, 1, total)
	}

	partial := f.token + "-hown"
	teams, total := listAdminTeamsByToken(t, repo, f.token, repositories.AdminTeamFilters{OwnerEmail: &partial})
	assert.Empty(t, teams)
	assert.Equal(t, 0, total)

	teams, total = listAdminTeamsByToken(t, repo, f.token, repositories.AdminTeamFilters{
		OwnerEmail: &f.heavyOwner, EmbeddingConfigured: boolp(false),
	})
	assert.Empty(t, teams)
	assert.Equal(t, 0, total)
}

// TestAdminTeamList_TotalMatchesPagedRows pages a filtered set one row at a time
// to the end: total_count must equal the number of rows actually reachable.
func TestAdminTeamList_TotalMatchesPagedRows(t *testing.T) {
	f := seedAdminTeamListFixture(t)
	repo := NewAdminRepository(integrationDB)

	for _, filters := range []repositories.AdminTeamFilters{
		{},
		{GitHubConfigured: boolp(true)},
		{MemberCount: repositories.AdminCountRange{Min: int64p(1)}, SortBy: "admin_count", SortOrder: "asc"},
	} {
		var seen []string
		var reportedTotal int
		for page := 1; page <= 5; page++ {
			filters.Page, filters.Limit = page, 1
			teams, total := listAdminTeamsByToken(t, repo, f.token, filters)
			reportedTotal = total
			if len(teams) == 0 {
				break
			}
			seen = append(seen, adminTeamIDs(teams)...)
		}
		assert.Len(t, seen, reportedTotal)
		assert.Len(t, slices.Compact(slices.Sorted(slices.Values(seen))), reportedTotal, "no row repeats across pages")
	}
}

// TestAdminTeamList_SortByEveryKey sorts by every count key in both directions,
// asserting the fixture's order (ties broken by t.id).
func TestAdminTeamList_SortByEveryKey(t *testing.T) {
	f := seedAdminTeamListFixture(t)
	repo := NewAdminRepository(integrationDB)

	// Values per key for [heavy, light, empty].
	keys := map[string][3]int64{
		"member_count":         {4, 2, 0},
		"owner_count":          {1, 1, 0},
		"admin_count":          {2, 0, 0},
		"project_count":        {2, 1, 0},
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
	fx := adminListFixture{heavy: f.heavy, light: f.light, empty: f.empty}
	for key, vals := range keys {
		for _, dir := range []string{"asc", "desc"} {
			t.Run(key+" "+dir, func(t *testing.T) {
				teams, _ := listAdminTeamsByToken(t, repo, f.token, repositories.AdminTeamFilters{SortBy: key, SortOrder: dir})
				assert.Equal(t, expectedAdminOrder(fx, vals, dir), adminTeamIDs(teams))
			})
		}
	}
}
