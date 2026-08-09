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

// Behavior-level suite for the stale-candidate query (#732) against real
// Postgres. sqlmock can only pin the query TEXT; whether GREATEST actually
// ignores NULLs, whether the threshold boundary is inclusive, and whether the
// naive `memories.updated_at` compares correctly against timestamptz columns
// are facts only a real database settles. Query text is never asserted here.

// candidateScope is one seeded team/project plus the ids needed to insert
// resources into it.
type candidateScope struct {
	userID    string
	teamID    string
	projectID string
}

func seedCandidateScope(t *testing.T) candidateScope {
	t.Helper()
	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	return candidateScope{userID: userID, teamID: teamID, projectID: insertTestProject(t, userID, teamID)}
}

// touchResource sets a resource's edit time and one per-medium last-accessed
// column directly, which is what lets a test place a row on either side of a
// threshold without waiting.
//
// `column` empty leaves every last-accessed column NULL -- the "never accessed"
// case the rule engine must treat as eligible.
func touchResource(t *testing.T, table, id, column string, updatedAt time.Time, accessedAt *time.Time) {
	t.Helper()

	// `memories` carries a BEFORE UPDATE trigger that rewrites updated_at to
	// CURRENT_TIMESTAMP unless the update touches only the last-accessed
	// columns (migration 014). Backdating updated_at from a test is exactly the
	// case it overwrites, so the trigger is suspended around this one statement
	// -- without it every memory fixture silently reads as edited just now, and
	// the assertion fails with no hint as to why.
	if table == "memories" {
		toggleMemoriesUpdatedAtTrigger(t, "DISABLE")
		defer toggleMemoriesUpdatedAtTrigger(t, "ENABLE")
	}

	// Table and column come from this test's own literals, never from input.
	query := fmt.Sprintf("UPDATE %s SET updated_at = $1 WHERE id = $2", table)
	_, err := integrationDB.ExecContext(context.Background(), query, updatedAt, id)
	require.NoError(t, err)

	if column == "" || accessedAt == nil {
		return
	}
	query = fmt.Sprintf("UPDATE %s SET %s = $1 WHERE id = $2", table, column)
	_, err = integrationDB.ExecContext(context.Background(), query, *accessedAt, id)
	require.NoError(t, err)
}

// toggleMemoriesUpdatedAtTrigger enables or disables the updated_at trigger on
// `memories`. `action` is a test-owned literal, never caller input.
func toggleMemoriesUpdatedAtTrigger(t *testing.T, action string) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		fmt.Sprintf("ALTER TABLE memories %s TRIGGER update_memories_updated_at", action))
	require.NoError(t, err)
}

func daysAgo(days int) time.Time {
	return time.Now().UTC().AddDate(0, 0, -days)
}

func candidateIDs(candidates []models.FreshnessCandidate) []string {
	ids := make([]string, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, c.ResourceID)
	}
	return ids
}

// A resource untouched for longer than the threshold is stale; one touched
// inside it is not. This is the whole rule, so it is asserted first.
func TestIntegrationFreshnessCandidates_ThresholdSelectsOnlyOldResources(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	repo := NewFreshnessCandidateRepository(integrationDB)

	stale := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "stale", "body", "published")
	fresh := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "fresh", "body", "published")
	accessedLongAgo, accessedRecently := daysAgo(90), daysAgo(2)
	touchResource(t, "prompts", stale, "last_accessed_web_at", daysAgo(120), &accessedLongAgo)
	touchResource(t, "prompts", fresh, "last_accessed_web_at", daysAgo(120), &accessedRecently)

	got, err := repo.ListStaleCandidates(context.Background(), models.FreshnessCandidateQuery{
		TeamID: scope.teamID, ResourceType: "prompt", ThresholdDays: 30,
	})

	require.NoError(t, err)
	assert.Equal(t, []string{stale}, candidateIDs(got))
	require.Len(t, got, 1)
	assert.Equal(t, scope.projectID, got[0].ProjectID, "project_id is denormalized onto the freshness row")
}

// "Not accessed in N days" means MORE than N. Asserted a minute either side of
// the boundary so an off-by-one in either direction fails; equality itself is
// not asserted because `now()` advances between the fixture and the query,
// which would make the test race its own clock.
func TestIntegrationFreshnessCandidates_ThresholdBoundaryDiscriminatesEitherSide(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	repo := NewFreshnessCandidateRepository(integrationDB)

	atThreshold := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "at", "body", "published")
	pastThreshold := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "past", "body", "published")
	// A minute inside the boundary, and a minute past it.
	insideEdge := daysAgo(30).Add(time.Minute)
	outsideEdge := daysAgo(30).Add(-time.Minute)
	touchResource(t, "prompts", atThreshold, "last_accessed_web_at", daysAgo(400), &insideEdge)
	touchResource(t, "prompts", pastThreshold, "last_accessed_web_at", daysAgo(400), &outsideEdge)

	got, err := repo.ListStaleCandidates(context.Background(), models.FreshnessCandidateQuery{
		TeamID: scope.teamID, ResourceType: "prompt", ThresholdDays: 30,
	})

	require.NoError(t, err)
	assert.Equal(t, []string{pastThreshold}, candidateIDs(got))
}

// A resource never accessed through any medium falls back to its own
// updated_at, so it is eligible rather than invisible. NULL must not act as
// "recently accessed".
func TestIntegrationFreshnessCandidates_NeverAccessedIsEligible(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	repo := NewFreshnessCandidateRepository(integrationDB)

	neverAccessed := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "never", "body", "published")
	touchResource(t, "prompts", neverAccessed, "", daysAgo(120), nil)

	got, err := repo.ListStaleCandidates(context.Background(), models.FreshnessCandidateQuery{
		TeamID: scope.teamID, ResourceType: "prompt", ThresholdDays: 30,
	})

	require.NoError(t, err)
	assert.Equal(t, []string{neverAccessed}, candidateIDs(got))
}

// Editing a resource keeps it fresh even when nobody has re-read it: the
// evaluation date is the LATER of the edit and the accesses.
func TestIntegrationFreshnessCandidates_RecentEditKeepsResourceFresh(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	repo := NewFreshnessCandidateRepository(integrationDB)

	edited := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "edited", "body", "published")
	accessedLongAgo := daysAgo(200)
	touchResource(t, "prompts", edited, "last_accessed_web_at", daysAgo(1), &accessedLongAgo)

	got, err := repo.ListStaleCandidates(context.Background(), models.FreshnessCandidateQuery{
		TeamID: scope.teamID, ResourceType: "prompt", ThresholdDays: 30,
	})

	require.NoError(t, err)
	assert.Empty(t, got)
}

// A narrowed rule counts only its own mediums; an "any medium" rule counts all
// of them. Same fixture, two queries, opposite verdicts.
func TestIntegrationFreshnessCandidates_MediumNarrowing(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	repo := NewFreshnessCandidateRepository(integrationDB)

	// Read recently through the CLI only.
	cliOnly := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "cli", "body", "published")
	recent := daysAgo(1)
	touchResource(t, "prompts", cliOnly, "last_accessed_cli_at", daysAgo(200), &recent)

	narrowed, err := repo.ListStaleCandidates(context.Background(), models.FreshnessCandidateQuery{
		TeamID: scope.teamID, ResourceType: "prompt", ThresholdDays: 30, Mediums: []string{"web"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{cliOnly}, candidateIDs(narrowed),
		"a web-only rule must ignore the CLI read")

	anyMedium, err := repo.ListStaleCandidates(context.Background(), models.FreshnessCandidateQuery{
		TeamID: scope.teamID, ResourceType: "prompt", ThresholdDays: 30,
	})
	require.NoError(t, err)
	assert.Empty(t, anyMedium, "an any-medium rule must count the CLI read")
}

// The tenancy predicate and the optional project scope are the two ways a rule
// can be narrowed; neither may leak another team's or project's resources.
func TestIntegrationFreshnessCandidates_ScopesByTeamAndProject(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	otherProject := insertTestProject(t, scope.userID, scope.teamID)
	otherScope := seedCandidateScope(t)
	repo := NewFreshnessCandidateRepository(integrationDB)

	mine := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "mine", "body", "published")
	sameTeamOtherProject := insertTestPrompt(t, scope.userID, scope.teamID, otherProject, "other", "body", "published")
	otherTeam := insertTestPrompt(
		t, otherScope.userID, otherScope.teamID, otherScope.projectID, "foreign", "body", "published")
	for _, id := range []string{mine, sameTeamOtherProject} {
		touchResource(t, "prompts", id, "", daysAgo(120), nil)
	}
	touchResource(t, "prompts", otherTeam, "", daysAgo(120), nil)

	anyProject, err := repo.ListStaleCandidates(context.Background(), models.FreshnessCandidateQuery{
		TeamID: scope.teamID, ResourceType: "prompt", ThresholdDays: 30,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{mine, sameTeamOtherProject}, candidateIDs(anyProject),
		"a rule with no project scope covers the whole team and nothing outside it")

	scoped, err := repo.ListStaleCandidates(context.Background(), models.FreshnessCandidateQuery{
		TeamID: scope.teamID, ResourceType: "prompt", ThresholdDays: 30, ProjectID: &scope.projectID,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{mine}, candidateIDs(scoped))
}

// Every freshness-eligible resource type must be queryable, and `memories` in
// particular: its updated_at is a naive timestamp, so an unconverted GREATEST
// would compare it against timestamptz in the server's timezone and silently
// shift the threshold.
func TestIntegrationFreshnessCandidates_CoversAllFourResourceTypes(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	repo := NewFreshnessCandidateRepository(integrationDB)

	seeded := map[string]string{
		"prompt":    insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "p", "body", "published"),
		"artifact":  insertTestArtifact(t, scope.userID, scope.teamID, scope.projectID, "a", "content", "active"),
		"blueprint": insertCandidateBlueprint(t, scope),
		"memory":    insertCandidateMemory(t, scope),
	}
	tables := map[string]string{
		"prompt": "prompts", "artifact": "artifacts", "blueprint": "blueprints", "memory": "memories",
	}

	for resourceType, id := range seeded {
		touchResource(t, tables[resourceType], id, "", daysAgo(120), nil)
	}

	for resourceType, id := range seeded {
		t.Run(resourceType, func(t *testing.T) {
			got, err := repo.ListStaleCandidates(context.Background(), models.FreshnessCandidateQuery{
				TeamID: scope.teamID, ResourceType: resourceType, ThresholdDays: 30,
			})
			require.NoError(t, err)
			assert.Equal(t, []string{id}, candidateIDs(got))
		})
	}
}

// Paging must return each resource exactly once: the keyset cursor is what
// stops a large rule from re-reading its first page forever.
func TestIntegrationFreshnessCandidates_KeysetPagingWalksEveryRowOnce(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	repo := NewFreshnessCandidateRepository(integrationDB)

	want := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		id := insertTestPrompt(
			t, scope.userID, scope.teamID, scope.projectID, fmt.Sprintf("p%d", i), "body", "published")
		touchResource(t, "prompts", id, "", daysAgo(120), nil)
		want = append(want, id)
	}

	seen := make([]string, 0, len(want))
	query := models.FreshnessCandidateQuery{
		TeamID: scope.teamID, ResourceType: "prompt", ThresholdDays: 30, Limit: 2,
	}
	for {
		page, err := repo.ListStaleCandidates(context.Background(), query)
		require.NoError(t, err)
		seen = append(seen, candidateIDs(page)...)
		if len(page) < query.Limit {
			break
		}
		query.AfterID = page[len(page)-1].ResourceID
	}

	assert.ElementsMatch(t, want, seen)
	assert.Len(t, seen, len(want), "no resource may be returned twice")
}

// insertCandidateBlueprint seeds a blueprint. `path` is NOT NULL since
// migration 007, so it must be supplied explicitly.
func insertCandidateBlueprint(t *testing.T, scope candidateScope) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO blueprints (id, user_id, team_id, project_id, title, slug, content, type, path, status) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)",
		id, scope.userID, scope.teamID, scope.projectID, "Blueprint", "blueprint-"+id[:8],
		"content", "general", ".claude/blueprint-"+id[:8]+".md", "active")
	require.NoError(t, err)
	return id
}

func insertCandidateMemory(t *testing.T, scope candidateScope) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO memories (id, user_id, team_id, project_id, text) VALUES ($1, $2, $3, $4, $5)",
		id, scope.userID, scope.teamID, scope.projectID, "remembered")
	require.NoError(t, err)
	return id
}

// `updated_at` carries a DEFAULT but no NOT NULL on any of the four resource
// tables, so an explicit NULL is accepted. Without the COALESCE, GREATEST over
// all-NULL arguments is NULL, `NULL < x` is NULL, and the row drops out of the
// result -- permanently EXEMPT from staleness, the exact opposite of the
// never-touched rule.
func TestIntegrationFreshnessCandidates_NullUpdatedAtIsMaximallyStale(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	repo := NewFreshnessCandidateRepository(integrationDB)

	orphan := insertTestPrompt(t, scope.userID, scope.teamID, scope.projectID, "null", "body", "published")
	_, err := integrationDB.ExecContext(context.Background(),
		"UPDATE prompts SET updated_at = NULL WHERE id = $1", orphan)
	require.NoError(t, err)

	got, err := repo.ListStaleCandidates(context.Background(), models.FreshnessCandidateQuery{
		TeamID: scope.teamID, ResourceType: "prompt", ThresholdDays: 30,
	})

	require.NoError(t, err)
	assert.Equal(t, []string{orphan}, candidateIDs(got))
}
