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
	"github.com/vibexp/vibexp/internal/repositories"
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

// The analytics aggregations (#734) against real Postgres. sqlmock can pin the
// query text but not what `unnest` does to a resource two rules both match,
// nor whether the grouped totals agree with the distinct count — which is the
// whole reason the by-rule chart carries a separate total.
func TestIntegrationFreshnessMetrics_GroupedCountsAndDistinctTotal(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	otherProject := insertTestProject(t, scope.userID, scope.teamID)
	otherTeam := seedCandidateScope(t)
	ctx := context.Background()
	state := NewResourceFreshnessRepository(integrationDB)

	ruleA, ruleB := uuid.New().String(), uuid.New().String()
	// One prompt matched by BOTH rules, one artifact by rule A only, one
	// memory in another project, and one row in a different team entirely.
	require.NoError(t, upsertFreshness(ctx, state, &models.ResourceFreshness{
		TeamID: scope.teamID, ProjectID: scope.projectID, ResourceType: "prompt",
		ResourceID: uuid.New().String(), Status: models.FreshnessStatusStale,
		MatchedRuleIDs: []string{ruleA, ruleB}, Reason: models.FreshnessReasonRuleRun,
	}))
	require.NoError(t, upsertFreshness(ctx, state, &models.ResourceFreshness{
		TeamID: scope.teamID, ProjectID: scope.projectID, ResourceType: "artifact",
		ResourceID: uuid.New().String(), Status: models.FreshnessStatusStale,
		MatchedRuleIDs: []string{ruleA}, Reason: models.FreshnessReasonRuleRun,
	}))
	require.NoError(t, upsertFreshness(ctx, state, &models.ResourceFreshness{
		TeamID: scope.teamID, ProjectID: otherProject, ResourceType: "memory",
		ResourceID: uuid.New().String(), Status: models.FreshnessStatusStale,
		MatchedRuleIDs: []string{ruleB}, Reason: models.FreshnessReasonRuleRun,
	}))
	require.NoError(t, upsertFreshness(ctx, state, &models.ResourceFreshness{
		TeamID: otherTeam.teamID, ProjectID: otherTeam.projectID, ResourceType: "prompt",
		ResourceID: uuid.New().String(), Status: models.FreshnessStatusStale,
		MatchedRuleIDs: []string{ruleA}, Reason: models.FreshnessReasonRuleRun,
	}))

	byType, err := state.CountStaleByType(ctx, scope.teamID)
	require.NoError(t, err)
	assert.Equal(t, []models.FreshnessBucketCount{
		{Key: "artifact", Count: 1}, {Key: "memory", Count: 1}, {Key: "prompt", Count: 1},
	}, byType, "another team's stale prompt must not appear")

	byProject, err := state.CountStaleByProject(ctx, scope.teamID)
	require.NoError(t, err)
	counts := map[string]int{}
	for _, b := range byProject {
		counts[b.Key] = b.Count
	}
	assert.Equal(t, map[string]int{scope.projectID: 2, otherProject: 1}, counts)

	byRule, err := state.CountStaleByRule(ctx, scope.teamID)
	require.NoError(t, err)
	ruleCounts := map[string]int{}
	for _, b := range byRule {
		ruleCounts[b.Key] = b.Count
	}
	assert.Equal(t, map[string]int{ruleA: 2, ruleB: 2}, ruleCounts,
		"the resource both rules match contributes to each of them")

	total, err := state.CountStale(ctx, scope.teamID)
	require.NoError(t, err)
	assert.Equal(t, 3, total,
		"the distinct total is 3 while the per-rule counts sum to 4 — which is why it is reported separately")
}

// The audit aggregation buckets by UTC day and action, and stops at the window.
func TestIntegrationFreshnessMetrics_TransitionsByDay(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	ctx := context.Background()
	audit := NewFreshnessAuditRepository(integrationDB)

	for i := 0; i < 3; i++ {
		require.NoError(t, audit.Create(ctx, &models.ResourceFreshnessAudit{
			TeamID: scope.teamID, ResourceType: "prompt", ResourceID: uuid.New().String(),
			Action: models.FreshnessActionMarked, Reason: models.FreshnessReasonRuleRun,
		}))
	}
	require.NoError(t, audit.Create(ctx, &models.ResourceFreshnessAudit{
		TeamID: scope.teamID, ResourceType: "prompt", ResourceID: uuid.New().String(),
		Action: models.FreshnessActionCleared, Reason: models.FreshnessReasonAccessed,
	}))

	// Backdate one row well outside the window; it must not be counted.
	_, err := integrationDB.ExecContext(ctx,
		`UPDATE resource_freshness_audit SET created_at = now() - interval '90 days'
		 WHERE id = (SELECT id FROM resource_freshness_audit WHERE action = 'marked' LIMIT 1)`)
	require.NoError(t, err)

	got, err := audit.CountTransitionsByDay(ctx, scope.teamID, time.Now().UTC().AddDate(0, 0, -7))
	require.NoError(t, err)

	byAction := map[string]int{}
	today := time.Now().UTC().Format("2006-01-02")
	for _, c := range got {
		assert.Equal(t, today, c.Date, "the bucket must be rendered in the series layout")
		byAction[c.Action] = c.Count
	}
	assert.Equal(t, map[string]int{
		models.FreshnessActionMarked:  2,
		models.FreshnessActionCleared: 1,
	}, byAction, "the backdated row is outside the window")
}

// The audit list must be team-scoped: the log is readable by any member, so a
// leak here exposes another team's resource ids to every member of this one.
func TestIntegrationFreshnessMetrics_AuditIsTeamScoped(t *testing.T) {
	resetFreshnessTables(t)
	mine := seedCandidateScope(t)
	theirs := seedCandidateScope(t)
	ctx := context.Background()
	audit := NewFreshnessAuditRepository(integrationDB)

	require.NoError(t, audit.Create(ctx, &models.ResourceFreshnessAudit{
		TeamID: mine.teamID, ResourceType: "prompt", ResourceID: uuid.New().String(),
		Action: models.FreshnessActionMarked, Reason: models.FreshnessReasonRuleRun,
	}))
	require.NoError(t, audit.Create(ctx, &models.ResourceFreshnessAudit{
		TeamID: theirs.teamID, ResourceType: "prompt", ResourceID: uuid.New().String(),
		Action: models.FreshnessActionMarked, Reason: models.FreshnessReasonRuleRun,
	}))

	entries, total, err := audit.ListByTeam(ctx, mine.teamID, 20, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, mine.teamID, entries[0].TeamID)

	transitions, err := audit.CountTransitionsByDay(ctx, mine.teamID, time.Now().UTC().AddDate(0, 0, -7))
	require.NoError(t, err)
	require.Len(t, transitions, 1)
	assert.Equal(t, 1, transitions[0].Count)
}

// Freshness surfacing (#735) against real Postgres. The filter is a correlated
// EXISTS added to the SHARED where-clause, so the properties that matter are
// that it narrows the page AND the count identically, that it is team-scoped,
// and that it is index-backed rather than scanning.

// markResourceStale writes a freshness row the way the evaluator would.
func markResourceStale(t *testing.T, teamID, projectID, resourceType, resourceID string) {
	t.Helper()
	require.NoError(t, upsertFreshness(context.Background(), NewResourceFreshnessRepository(integrationDB),
		&models.ResourceFreshness{
			TeamID: teamID, ProjectID: projectID, ResourceType: resourceType, ResourceID: resourceID,
			Status: models.FreshnessStatusStale, MatchedRuleIDs: []string{uuid.New().String()},
			Reason: models.FreshnessReasonRuleRun,
		}))
}

// The filter must narrow the page and the TOTAL together. They are produced by
// two separate queries that share only the WHERE clause, so a predicate added
// to one and not the other would return a short page while claiming the
// unfiltered total — the pagination bug an EXISTS in the shared clause avoids.
func TestIntegrationFreshnessFilter_NarrowsPageAndTotalTogether(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	ctx := context.Background()
	repo := NewArtifactRepository(integrationDB)

	stale := insertTestArtifact(t, scope.userID, scope.teamID, scope.projectID, "stale", "body", "active")
	insertTestArtifact(t, scope.userID, scope.teamID, scope.projectID, "fresh-1", "body", "active")
	insertTestArtifact(t, scope.userID, scope.teamID, scope.projectID, "fresh-2", "body", "active")
	markResourceStale(t, scope.teamID, scope.projectID, "artifact", stale)

	unfiltered, unfilteredTotal, err := repo.List(ctx, scope.userID, repositories.ArtifactFilters{
		TeamID: scope.teamID, Page: 1, Limit: 20,
	})
	require.NoError(t, err)
	require.Len(t, unfiltered, 3)
	require.Equal(t, 3, unfilteredTotal)

	staleOnly := FreshnessFilterStale
	filtered, filteredTotal, err := repo.List(ctx, scope.userID, repositories.ArtifactFilters{
		TeamID: scope.teamID, Page: 1, Limit: 20, Freshness: &staleOnly,
	})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, stale, filtered[0].ID)
	assert.Equal(t, 1, filteredTotal, "the count query must see the same predicate as the page query")
}

// The filter must never surface another team's resources, and another team's
// freshness row must never mark this team's resource as stale.
func TestIntegrationFreshnessFilter_IsTeamScoped(t *testing.T) {
	resetFreshnessTables(t)
	mine := seedCandidateScope(t)
	theirs := seedCandidateScope(t)
	ctx := context.Background()
	repo := NewArtifactRepository(integrationDB)

	theirStale := insertTestArtifact(t, theirs.userID, theirs.teamID, theirs.projectID, "theirs", "body", "active")
	markResourceStale(t, theirs.teamID, theirs.projectID, "artifact", theirStale)
	insertTestArtifact(t, mine.userID, mine.teamID, mine.projectID, "mine", "body", "active")

	staleOnly := FreshnessFilterStale
	filtered, total, err := repo.List(ctx, mine.userID, repositories.ArtifactFilters{
		TeamID: mine.teamID, Page: 1, Limit: 20, Freshness: &staleOnly,
	})

	require.NoError(t, err)
	assert.Empty(t, filtered, "the other team's stale artifact is outside this team's list entirely")
	assert.Equal(t, 0, total)
}

// The batch lookup that attaches freshness to a page must return only the ids
// asked for, and must not leak another team's state (the caller filters by
// team, and this proves the row carries the team to filter on).
func TestIntegrationFreshnessFilter_ListByResourcesIsExact(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	other := seedCandidateScope(t)
	ctx := context.Background()
	state := NewResourceFreshnessRepository(integrationDB)

	mine := uuid.New().String()
	theirs := uuid.New().String()
	unasked := uuid.New().String()
	markResourceStale(t, scope.teamID, scope.projectID, "artifact", mine)
	markResourceStale(t, other.teamID, other.projectID, "artifact", theirs)
	markResourceStale(t, scope.teamID, scope.projectID, "artifact", unasked)
	// Same id, different type: the lookup must not confuse the two.
	markResourceStale(t, scope.teamID, scope.projectID, "prompt", mine)

	got, err := state.ListByResources(ctx, "artifact", []string{mine, theirs})

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, scope.teamID, got[mine].TeamID)
	assert.Equal(t, other.teamID, got[theirs].TeamID, "the row carries the team the caller filters on")
	assert.NotContains(t, got, unasked)

	empty, err := state.ListByResources(ctx, "artifact", nil)
	require.NoError(t, err)
	assert.Empty(t, empty, "an empty page issues no query and still returns a usable map")
}

// The stale filter sits on list endpoints people browse, so it has to be an
// index seek. Only an execution plan can show that — a correlated EXISTS that
// seq-scans resource_freshness passes every behavioural test above.
func TestIntegrationFreshnessFilter_UsesTheUniqueIndex(t *testing.T) {
	resetFreshnessTables(t)
	scope := seedCandidateScope(t)
	artifactID := insertTestArtifact(t, scope.userID, scope.teamID, scope.projectID, "stale", "body", "active")
	markResourceStale(t, scope.teamID, scope.projectID, "artifact", artifactID)

	// The predicate is copied from applyStaleFilter verbatim; asserting a plan
	// for a query the repository does not issue would prove nothing.
	plan := explainFreshness(t,
		`SELECT a.id FROM artifacts a
		  WHERE a.team_id = $1
		    AND EXISTS (SELECT 1 FROM resource_freshness rf
		                 WHERE rf.resource_type = $2 AND rf.resource_id = a.id)`,
		scope.teamID, "artifact")

	assert.Contains(t, plan, "idx_resource_freshness_resource",
		"the stale filter must seek the unique (resource_type, resource_id) index; plan was:\n"+plan)
}
