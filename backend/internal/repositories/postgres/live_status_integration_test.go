//go:build integration

package postgres

import (
	"context"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behaviour-level proof, against real Postgres, of the live-status denylist
// (#1267): the unit tests pin the SQL fragments, these pin what the database
// actually returns. Every status-dependent read path is exercised with one row
// per status of each resource type -- the system statuses plus a custom one --
// so a path that still spelled an allowlist would lose the custom row, and a
// path that dropped its filter would leak a draft, archived or expired one.

// liveStatusCustom is a status no system subset contains: the stand-in for a
// status a team defined itself in `team_statuses`.
const liveStatusCustom = "in-review"

// liveStatusModel is the embedding model the semantic pass is queried with.
const liveStatusModel = "test-embed-1024"

// liveStatusTypes are the four searchable resource types, in the singular form
// both the search repository and models.StatusResourceTypes use.
var liveStatusTypes = []string{
	models.StatusResourceTypePrompt, models.StatusResourceTypeArtifact,
	models.StatusResourceTypeBlueprint, models.StatusResourceTypeMemory,
}

// preDenylistSearchable is what search returned BEFORE #1267, spelled as the
// literal allowlist the old filters hardcoded (`src.status = 'published'` for
// prompts, `src.status = 'active'` for the rest). It is deliberately not
// derived from the models package: it is the fixed expectation the "nothing
// changes for system-only data" test compares the new behaviour against.
var preDenylistSearchable = map[string]string{
	models.StatusResourceTypePrompt:    "published",
	models.StatusResourceTypeArtifact:  "active",
	models.StatusResourceTypeBlueprint: "active",
	models.StatusResourceTypeMemory:    "active",
}

// liveStatusFixture is one team with rows keyed by resource type, then status.
type liveStatusFixture struct {
	userID    string
	teamID    string
	projectID string
	// ids[resourceType][status] is the row seeded with that status.
	ids map[string]map[string]string
}

// newLiveStatusFixture seeds, for every resource type, one row per system
// status plus (when withCustom) one row in liveStatusCustom. Every row shares
// the title "widget configuration guide" and a body mentioning "widget", and
// has one embedding on the query axis, so all three search passes match every
// row and only the status filter decides what comes back.
func newLiveStatusFixture(t *testing.T, withCustom bool) liveStatusFixture {
	t.Helper()
	resetSearchTables(t)

	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	f := liveStatusFixture{
		userID:    userID,
		teamID:    teamID,
		projectID: insertTestProject(t, userID, teamID),
		ids:       map[string]map[string]string{},
	}

	for _, resourceType := range liveStatusTypes {
		statuses := models.SystemStatusesFor(resourceType)
		if withCustom {
			statuses = append(statuses, liveStatusCustom)
		}
		f.ids[resourceType] = map[string]string{}
		for _, status := range statuses {
			id := f.insert(t, resourceType, status)
			f.ids[resourceType][status] = id
			insertTestEmbedding(t, testEmbedding{
				userID: userID, teamID: teamID, entityType: resourceType, entityID: id,
				modelID: liveStatusModel, content: "widget chunk", vec: testVector(map[int]float32{0: 1}),
			})
		}
	}
	return f
}

// insert adds one row of resourceType carrying status and returns its id.
func (f liveStatusFixture) insert(t *testing.T, resourceType, status string) string {
	t.Helper()
	const title, body = "widget configuration guide", "widget setup notes"

	switch resourceType {
	case models.StatusResourceTypePrompt:
		return insertTestPrompt(t, f.userID, f.teamID, f.projectID, title, body, status)
	case models.StatusResourceTypeArtifact:
		return insertTestArtifact(t, f.userID, f.teamID, f.projectID, title, body, status)
	case models.StatusResourceTypeMemory:
		id := uuid.New().String()
		_, err := integrationDB.ExecContext(context.Background(),
			"INSERT INTO memories (id, user_id, team_id, project_id, text, status) VALUES ($1, $2, $3, $4, $5, $6)",
			id, f.userID, f.teamID, f.projectID, title, status)
		require.NoError(t, err)
		return id
	case models.StatusResourceTypeBlueprint:
		id := uuid.New().String()
		_, err := integrationDB.ExecContext(context.Background(),
			"INSERT INTO blueprints (id, user_id, team_id, project_id, slug, title, content, path, status) "+
				"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)",
			id, f.userID, f.teamID, f.projectID, "blueprint-"+id[:8], title, body, "rules/"+id[:8]+".md", status)
		require.NoError(t, err)
		return id
	default:
		require.FailNow(t, "unknown resource type", resourceType)
		return ""
	}
}

// idsWhere returns "<type>/<id>" for every seeded row whose (type, status)
// pair keep accepts, sorted.
func (f liveStatusFixture) idsWhere(keep func(resourceType, status string) bool) []string {
	var out []string
	for resourceType, byStatus := range f.ids {
		for status, id := range byStatus {
			if keep(resourceType, status) {
				out = append(out, resourceType+"/"+id)
			}
		}
	}
	sort.Strings(out)
	return out
}

// liveStatusSearchPass runs one search pass over all four types and returns the sorted
// "<type>/<id>" of every row on the page, plus the pass's total_count.
type liveStatusSearchPass func(t *testing.T, repo *SearchRepository, f liveStatusFixture) ([]string, int)

func searchRowKeys(rows []models.SearchResultRow) []string {
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, row.EntityType+"/"+row.EntityID)
	}
	sort.Strings(keys)
	return keys
}

// liveStatusSearchPasses are the three search passes. "widget" is a lexeme of
// every row, so the strict full-text pass answers it; "widgte" is a lexeme of
// none, so both full-text passes miss and the pg_trgm title pass answers it.
func liveStatusSearchPasses() map[string]liveStatusSearchPass {
	page := repositories.Page{Limit: 50, Offset: 0}
	keyword := func(query string) liveStatusSearchPass {
		return func(t *testing.T, repo *SearchRepository, f liveStatusFixture) ([]string, int) {
			t.Helper()
			rows, total, err := repo.SearchKeyword(context.Background(), f.teamID, query, liveStatusTypes, "", page)
			require.NoError(t, err)
			return searchRowKeys(rows), total
		}
	}
	return map[string]liveStatusSearchPass{
		"semantic": func(t *testing.T, repo *SearchRepository, f liveStatusFixture) ([]string, int) {
			t.Helper()
			rows, total, err := repo.SearchSimilar(context.Background(), f.teamID,
				testVector(map[int]float32{0: 1}), liveStatusModel, liveStatusTypes, "", page)
			require.NoError(t, err)
			return searchRowKeys(rows), total
		},
		"full-text": keyword("widget"),
		"trigram":   keyword("widgte"),
	}
}

// A custom status is searchable in every pass, and the non-live system
// statuses are not; the page and total_count agree.
func TestIntegrationLiveStatus_SearchIncludesCustomAndExcludesNonLive(t *testing.T) {
	repo := NewSearchRepository(integrationDB).(*SearchRepository)
	f := newLiveStatusFixture(t, true)

	want := f.idsWhere(models.IsLiveStatus)
	// Sanity-check the expectation itself: 2 live prompts (published, custom),
	// 2 artifacts, 2 blueprints, 2 memories -- and 6 non-live rows left out.
	require.Len(t, want, 8)
	nonLive := f.idsWhere(func(resourceType, status string) bool {
		return !models.IsLiveStatus(resourceType, status)
	})
	require.Len(t, nonLive, 6)

	for name, pass := range liveStatusSearchPasses() {
		t.Run(name, func(t *testing.T) {
			got, total := pass(t, repo, f)

			assert.Equal(t, want, got)
			assert.Equal(t, len(want), total, "total_count must agree with the page")
			for _, resourceType := range liveStatusTypes {
				assert.Contains(t, got, resourceType+"/"+f.ids[resourceType][liveStatusCustom],
					"the custom-status %s must be searchable", resourceType)
			}
			for _, key := range nonLive {
				assert.NotContains(t, got, key)
			}
		})
	}
}

// For data that uses only system statuses, search returns exactly what the
// pre-#1267 allowlist returned.
func TestIntegrationLiveStatus_SearchUnchangedForSystemOnlyData(t *testing.T) {
	repo := NewSearchRepository(integrationDB).(*SearchRepository)
	f := newLiveStatusFixture(t, false)

	want := f.idsWhere(func(resourceType, status string) bool {
		return status == preDenylistSearchable[resourceType]
	})
	require.Len(t, want, len(liveStatusTypes), "one searchable row per type under the old allowlist")

	for name, pass := range liveStatusSearchPasses() {
		t.Run(name, func(t *testing.T) {
			got, total := pass(t, repo, f)

			assert.Equal(t, want, got)
			assert.Equal(t, len(want), total)
		})
	}
}

// statusesOf returns the sorted statuses of the listed rows.
func statusesOf[T any](rows []T, status func(T) string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, status(row))
	}
	sort.Strings(out)
	return out
}

// The artifact list: a search term returns live statuses (custom included),
// the default hides only archived, and an explicit status selects exactly it.
func TestIntegrationLiveStatus_ArtifactList(t *testing.T) {
	f := newLiveStatusFixture(t, true)
	repo := NewArtifactRepository(integrationDB)

	list := func(t *testing.T, filters repositories.ArtifactFilters) ([]string, int) {
		t.Helper()
		filters.TeamID, filters.Page, filters.Limit = f.teamID, 1, 50
		rows, total, err := repo.List(context.Background(), f.userID, filters)
		require.NoError(t, err)
		return statusesOf(rows, func(a models.Artifact) string { return a.Status }), total
	}
	custom, draft := liveStatusCustom, models.ArtifactStatusDraft

	t.Run("a search term returns live statuses, custom included", func(t *testing.T) {
		got, total := list(t, repositories.ArtifactFilters{Search: "widget"})
		assert.Equal(t, []string{"active", liveStatusCustom}, got)
		assert.Equal(t, 2, total)
	})
	t.Run("a search term overrides an explicit non-live status", func(t *testing.T) {
		got, _ := list(t, repositories.ArtifactFilters{Search: "widget", Status: &draft})
		assert.Equal(t, []string{"active", liveStatusCustom}, got)
	})
	t.Run("the default list hides only archived", func(t *testing.T) {
		got, total := list(t, repositories.ArtifactFilters{})
		assert.Equal(t, []string{"active", "draft", liveStatusCustom}, got)
		assert.Equal(t, 3, total)
	})
	t.Run("an explicit status returns exactly that status", func(t *testing.T) {
		got, _ := list(t, repositories.ArtifactFilters{Status: &draft})
		assert.Equal(t, []string{"draft"}, got)
		got, _ = list(t, repositories.ArtifactFilters{Status: &custom})
		assert.Equal(t, []string{liveStatusCustom}, got)
	})
}

// The memory list follows the same three rules as the artifact list.
func TestIntegrationLiveStatus_MemoryList(t *testing.T) {
	f := newLiveStatusFixture(t, true)
	repo := NewMemoryRepository(integrationDB)

	list := func(t *testing.T, filters repositories.MemoryFilters) ([]string, int) {
		t.Helper()
		filters.TeamID, filters.Page, filters.Limit = f.teamID, 1, 50
		rows, total, err := repo.List(context.Background(), f.userID, filters)
		require.NoError(t, err)
		return statusesOf(rows, func(m models.Memory) string { return m.Status }), total
	}
	custom, archived := liveStatusCustom, models.MemoryStatusArchived

	t.Run("a search term returns live statuses, custom included", func(t *testing.T) {
		got, total := list(t, repositories.MemoryFilters{Search: "widget"})
		assert.Equal(t, []string{"active", liveStatusCustom}, got)
		assert.Equal(t, 2, total)
	})
	t.Run("the default list hides only archived", func(t *testing.T) {
		got, total := list(t, repositories.MemoryFilters{})
		assert.Equal(t, []string{"active", "draft", liveStatusCustom}, got)
		assert.Equal(t, 3, total)
	})
	t.Run("an explicit status returns exactly that status", func(t *testing.T) {
		got, _ := list(t, repositories.MemoryFilters{Status: &archived})
		assert.Equal(t, []string{"archived"}, got)
		got, _ = list(t, repositories.MemoryFilters{Status: &custom})
		assert.Equal(t, []string{liveStatusCustom}, got)
	})
}

// The MCP prompt-exposure query (#1267): "not a draft AND mcp_expose" returns
// the published and the custom-status prompt, never the draft, and never a
// prompt with mcp_expose off -- the exact filter addTeamPromptsToMCP sends.
func TestIntegrationLiveStatus_PromptExposureList(t *testing.T) {
	f := newLiveStatusFixture(t, true)
	repo := NewPromptRepository(integrationDB)
	ctx := context.Background()

	// Expose every seeded prompt, then add one live prompt that is NOT exposed.
	_, err := integrationDB.ExecContext(ctx, "UPDATE prompts SET mcp_expose = true WHERE team_id = $1", f.teamID)
	require.NoError(t, err)
	hidden := insertTestPrompt(t, f.userID, f.teamID, f.projectID, "hidden prompt", "body", liveStatusCustom)
	_, err = integrationDB.ExecContext(ctx, "UPDATE prompts SET mcp_expose = false WHERE id = $1", hidden)
	require.NoError(t, err)

	exposed := true
	list := func(t *testing.T, exclude []string) ([]string, int) {
		t.Helper()
		rows, total, listErr := repo.List(ctx, f.userID, repositories.PromptFilters{
			TeamID: f.teamID, MCPExpose: &exposed, ExcludeStatuses: exclude, Page: 1, Limit: 50,
		})
		require.NoError(t, listErr)
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		sort.Strings(ids)
		return ids, total
	}

	prompts := f.ids[models.StatusResourceTypePrompt]
	want := []string{prompts[models.PromptStatusPublished], prompts[liveStatusCustom]}
	sort.Strings(want)

	got, total := list(t, models.NonLiveStatusesFor(models.StatusResourceTypePrompt))
	assert.Equal(t, want, got)
	assert.Equal(t, 2, total, "total_count must agree with the page")
	assert.NotContains(t, got, prompts[models.PromptStatusDraft])
	assert.NotContains(t, got, hidden)

	// Without the exclusion the exposed draft comes back: the filter is what
	// keeps it out, not the fixture.
	all, _ := list(t, nil)
	assert.Contains(t, all, prompts[models.PromptStatusDraft])
	assert.Len(t, all, 3)
}
