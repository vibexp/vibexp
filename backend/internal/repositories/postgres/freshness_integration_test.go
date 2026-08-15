//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Behavior-level suite for the Resource Freshness schema foundation (#729)
// against real Postgres. It asserts what only a real database can prove:
// the CHECK constraints, the unique upsert key, ON DELETE CASCADE, array
// round-tripping, index usage for the listing queries, and that the
// last_accessed_at columns exist on all four resource tables and start NULL.
// Query text is never asserted here — that is the sqlmock suite's job.

// resetFreshnessTables clears this suite's chain. The freshness tables hang
// off teams and projects, which the shared resetIntegrationTables does not
// name, so this suite truncates its own.
func resetFreshnessTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, projects, prompts, artifacts, blueprints, memories, "+
			"resource_freshness, freshness_rules, team_freshness_settings, "+
			"resource_freshness_audit CASCADE")
	require.NoError(t, err)
}

// seedFreshnessScope creates a user, a team and a project — the FK targets
// every freshness row needs.
func seedFreshnessScope(t *testing.T) (teamID, projectID string) {
	t.Helper()
	userID := insertTestUser(t)
	teamID = insertTestTeam(t, userID)
	projectID = insertTestProject(t, userID, teamID)
	return teamID, projectID
}

// upsertFreshness writes a fixture row and discards the inserted flag (#771).
// The flag itself is proved against real Postgres by
// TestIntegrationResourceFreshness_UpsertReportsWhetherItInserted; everywhere
// else below only cares that the row landed.
func upsertFreshness(
	ctx context.Context, repo repositories.ResourceFreshnessRepository, f *models.ResourceFreshness,
) error {
	_, err := repo.Upsert(ctx, f)
	return err
}

func newFreshnessState(teamID, projectID, resourceType string, ruleIDs []string) *models.ResourceFreshness {
	return &models.ResourceFreshness{
		TeamID:         teamID,
		ProjectID:      projectID,
		ResourceType:   resourceType,
		ResourceID:     uuid.New().String(),
		Status:         models.FreshnessStatusStale,
		MatchedRuleIDs: ruleIDs,
		Reason:         models.FreshnessReasonRuleRun,
	}
}

// ---------------------------------------------------------------------------
// resource_freshness
// ---------------------------------------------------------------------------

func TestIntegrationResourceFreshness_UpsertRoundTrip(t *testing.T) {
	resetFreshnessTables(t)
	teamID, projectID := seedFreshnessScope(t)
	repo := NewResourceFreshnessRepository(integrationDB)
	ctx := context.Background()

	ruleA, ruleB := uuid.New().String(), uuid.New().String()
	state := newFreshnessState(teamID, projectID, "artifact", []string{ruleA, ruleB})
	require.NoError(t, upsertFreshness(ctx, repo, state))

	require.NotEmpty(t, state.ID)
	assert.False(t, state.Since.IsZero(), "a zero Since must default to the database clock")

	got, err := repo.GetByResource(ctx, "artifact", state.ResourceID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, state.ID, got.ID)
	assert.Equal(t, []string{ruleA, ruleB}, got.MatchedRuleIDs, "uuid[] must round-trip in order")
	assert.Equal(t, models.FreshnessStatusStale, got.Status)
}

func TestIntegrationResourceFreshness_GetByResource_NotStale(t *testing.T) {
	resetFreshnessTables(t)
	repo := NewResourceFreshnessRepository(integrationDB)

	got, err := repo.GetByResource(context.Background(), "artifact", uuid.New().String())

	require.NoError(t, err)
	assert.Nil(t, got, "a resource that is not stale must yield (nil, nil), not an error")
}

// `(xmax = 0)` is a system-column trick, not portable SQL, and sqlmock returns
// whatever the test declares — so only real Postgres can prove the flag means
// what the evaluator now branches on (#771). A row that is inserted, then
// deleted, then written again must report true BOTH times: that second insert
// is exactly the race, a clear having removed the row mid-run.
func TestIntegrationResourceFreshness_UpsertReportsWhetherItInserted(t *testing.T) {
	resetFreshnessTables(t)
	teamID, projectID := seedFreshnessScope(t)
	repo := NewResourceFreshnessRepository(integrationDB)
	ctx := context.Background()

	row := newFreshnessState(teamID, projectID, "prompt", []string{uuid.New().String()})

	inserted, err := repo.Upsert(ctx, row)
	require.NoError(t, err)
	assert.True(t, inserted, "the first write of a resource inserts")

	again := newFreshnessState(teamID, projectID, "prompt", []string{uuid.New().String()})
	again.ResourceID = row.ResourceID
	inserted, err = repo.Upsert(ctx, again)
	require.NoError(t, err)
	assert.False(t, inserted, "a second write of the same resource updates on conflict")

	deleted, err := repo.DeleteByResource(ctx, "prompt", row.ResourceID)
	require.NoError(t, err)
	require.True(t, deleted)

	afterClear := newFreshnessState(teamID, projectID, "prompt", []string{uuid.New().String()})
	afterClear.ResourceID = row.ResourceID
	inserted, err = repo.Upsert(ctx, afterClear)
	require.NoError(t, err)
	assert.True(t, inserted, "a write after a concurrent clear inserts again — the #771 race")
}

// The second upsert of the same resource must update the one row rather than
// insert a second, and must preserve `since` — that column means "first marked
// stale at", so re-evaluation resetting it would misreport every resource's age.
func TestIntegrationResourceFreshness_UpsertPreservesSinceAndDoesNotDuplicate(t *testing.T) {
	resetFreshnessTables(t)
	teamID, projectID := seedFreshnessScope(t)
	repo := NewResourceFreshnessRepository(integrationDB)
	ctx := context.Background()

	original := time.Now().UTC().Add(-30 * 24 * time.Hour).Truncate(time.Microsecond)
	first := newFreshnessState(teamID, projectID, "prompt", []string{uuid.New().String()})
	first.Since = original
	require.NoError(t, upsertFreshness(ctx, repo, first))
	require.True(t, first.Since.Equal(original))

	newRule := uuid.New().String()
	second := newFreshnessState(teamID, projectID, "prompt", []string{newRule})
	second.ResourceID = first.ResourceID
	second.Since = time.Now().UTC() // a later "since" the upsert must ignore
	require.NoError(t, upsertFreshness(ctx, repo, second))

	assert.Equal(t, first.ID, second.ID, "upsert must update the existing row, not insert a second")
	assert.True(t, second.Since.Equal(original),
		"since must survive re-evaluation: got %v, want %v", second.Since, original)
	assert.Equal(t, []string{newRule}, second.MatchedRuleIDs, "matched rules ARE replaced")

	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT count(*) FROM resource_freshness WHERE resource_id = $1", first.ResourceID).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestIntegrationResourceFreshness_ListFilters(t *testing.T) {
	resetFreshnessTables(t)
	teamID, projectID := seedFreshnessScope(t)
	otherTeamID, otherProjectID := seedFreshnessScope(t)
	repo := NewResourceFreshnessRepository(integrationDB)
	ctx := context.Background()

	rule := uuid.New().String()
	require.NoError(t, upsertFreshness(ctx, repo, newFreshnessState(teamID, projectID, "artifact", []string{rule})))
	require.NoError(t, upsertFreshness(ctx, repo, newFreshnessState(teamID, projectID, "prompt", []string{rule})))
	require.NoError(t, upsertFreshness(ctx, repo, newFreshnessState(otherTeamID, otherProjectID, "artifact", []string{rule})))

	tests := []struct {
		name    string
		filters models.ResourceFreshnessFilters
		want    int
	}{
		{
			name:    "team only",
			filters: models.ResourceFreshnessFilters{TeamID: teamID},
			want:    2,
		},
		{
			name:    "team and type",
			filters: models.ResourceFreshnessFilters{TeamID: teamID, ResourceType: "artifact"},
			want:    1,
		},
		{
			name:    "team and project",
			filters: models.ResourceFreshnessFilters{TeamID: teamID, ProjectID: projectID},
			want:    2,
		},
		{
			name:    "another team's project is not visible",
			filters: models.ResourceFreshnessFilters{TeamID: teamID, ProjectID: otherProjectID},
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, total, err := repo.List(ctx, tt.filters)
			require.NoError(t, err)
			assert.Equal(t, tt.want, total)
			assert.Len(t, items, tt.want)
		})
	}
}

// Total must count every match, not just the returned page.
func TestIntegrationResourceFreshness_ListPaginates(t *testing.T) {
	resetFreshnessTables(t)
	teamID, projectID := seedFreshnessScope(t)
	repo := NewResourceFreshnessRepository(integrationDB)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		require.NoError(t, upsertFreshness(ctx, repo,
			newFreshnessState(teamID, projectID, "memory", []string{uuid.New().String()})))
	}

	items, total, err := repo.List(ctx, models.ResourceFreshnessFilters{
		TeamID: teamID, Limit: 2, Offset: 1,
	})

	require.NoError(t, err)
	assert.Equal(t, 5, total, "total counts all matches, ignoring limit/offset")
	assert.Len(t, items, 2)
}

func TestIntegrationResourceFreshness_DeleteByResource(t *testing.T) {
	resetFreshnessTables(t)
	teamID, projectID := seedFreshnessScope(t)
	repo := NewResourceFreshnessRepository(integrationDB)
	ctx := context.Background()

	state := newFreshnessState(teamID, projectID, "blueprint", []string{uuid.New().String()})
	require.NoError(t, upsertFreshness(ctx, repo, state))

	deleted, err := repo.DeleteByResource(ctx, "blueprint", state.ResourceID)
	require.NoError(t, err)
	assert.True(t, deleted)

	again, err := repo.DeleteByResource(ctx, "blueprint", state.ResourceID)
	require.NoError(t, err)
	assert.False(t, again, "clearing a resource that is not stale is a no-op, not an error")
}

// The cleanup contract a rule deletion depends on: the id is stripped from
// every row, rows still matching another rule survive, and rows left matching
// nothing are removed.
func TestIntegrationResourceFreshness_RemoveRule(t *testing.T) {
	resetFreshnessTables(t)
	teamID, projectID := seedFreshnessScope(t)
	repo := NewResourceFreshnessRepository(integrationDB)
	ctx := context.Background()

	doomed, survivor := uuid.New().String(), uuid.New().String()
	onlyDoomed := newFreshnessState(teamID, projectID, "artifact", []string{doomed})
	both := newFreshnessState(teamID, projectID, "prompt", []string{doomed, survivor})
	untouched := newFreshnessState(teamID, projectID, "memory", []string{survivor})
	for _, s := range []*models.ResourceFreshness{onlyDoomed, both, untouched} {
		require.NoError(t, upsertFreshness(ctx, repo, s))
	}

	deleted, err := repo.RemoveRule(ctx, doomed)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted, "only the row left matching no rule is deleted")

	gone, err := repo.GetByResource(ctx, "artifact", onlyDoomed.ResourceID)
	require.NoError(t, err)
	assert.Nil(t, gone, "a row matching only the deleted rule must be removed")

	kept, err := repo.GetByResource(ctx, "prompt", both.ResourceID)
	require.NoError(t, err)
	require.NotNil(t, kept)
	assert.Equal(t, []string{survivor}, kept.MatchedRuleIDs, "the deleted rule's id must be stripped")

	stillThere, err := repo.GetByResource(ctx, "memory", untouched.ResourceID)
	require.NoError(t, err)
	require.NotNil(t, stillThere)
	assert.Equal(t, []string{survivor}, stillThere.MatchedRuleIDs)
}

// Regression: the cleanup deletes only the rows THIS call emptied. An earlier
// implementation deleted on `cardinality(matched_rule_ids) = 0`, which also
// matched pre-existing empty-array rows — including another team's — turning a
// rule deletion into silent collateral damage.
func TestIntegrationResourceFreshness_RemoveRuleLeavesUnrelatedEmptyRowsAlone(t *testing.T) {
	resetFreshnessTables(t)
	teamID, projectID := seedFreshnessScope(t)
	otherTeamID, otherProjectID := seedFreshnessScope(t)
	repo := NewResourceFreshnessRepository(integrationDB)
	ctx := context.Background()

	// A row belonging to another team that already matches no rule. Upsert
	// accepts an empty array, so this state is reachable through the API.
	bystander := newFreshnessState(otherTeamID, otherProjectID, "artifact", []string{})
	require.NoError(t, upsertFreshness(ctx, repo, bystander))

	doomed := uuid.New().String()
	target := newFreshnessState(teamID, projectID, "prompt", []string{doomed})
	require.NoError(t, upsertFreshness(ctx, repo, target))

	deleted, err := repo.RemoveRule(ctx, doomed)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted, "only the row this call emptied is deleted")

	survived, err := repo.GetByResource(ctx, "artifact", bystander.ResourceID)
	require.NoError(t, err)
	assert.NotNil(t, survived,
		"an unrelated team's empty-array row must not be collateral damage of a rule deletion")
}

func TestIntegrationResourceFreshness_TeamCascade(t *testing.T) {
	resetFreshnessTables(t)
	teamID, projectID := seedFreshnessScope(t)
	repo := NewResourceFreshnessRepository(integrationDB)
	ctx := context.Background()

	state := newFreshnessState(teamID, projectID, "artifact", []string{uuid.New().String()})
	require.NoError(t, upsertFreshness(ctx, repo, state))

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", teamID)
	require.NoError(t, err)

	got, err := repo.GetByResource(ctx, "artifact", state.ResourceID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleting a team must cascade its freshness state away")
}

// ---------------------------------------------------------------------------
// freshness_rules
// ---------------------------------------------------------------------------

func TestIntegrationFreshnessRule_CRUDRoundTrip(t *testing.T) {
	resetFreshnessTables(t)
	teamID, projectID := seedFreshnessScope(t)
	repo := NewFreshnessRuleRepository(integrationDB)
	ctx := context.Background()

	rule := &models.FreshnessRule{
		TeamID:        teamID,
		ProjectID:     &projectID,
		ResourceTypes: []string{"artifact", "prompt"},
		Mediums:       []string{"web", "mcp"},
		ThresholdDays: 90,
		Enabled:       true,
	}
	require.NoError(t, repo.Create(ctx, rule))
	require.NotEmpty(t, rule.ID)

	got, err := repo.GetByID(ctx, teamID, rule.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"artifact", "prompt"}, got.ResourceTypes)
	assert.Equal(t, []string{"web", "mcp"}, got.Mediums)
	require.NotNil(t, got.ProjectID)
	assert.Equal(t, projectID, *got.ProjectID)

	got.ThresholdDays = 45
	got.Enabled = false
	got.Mediums = nil
	require.NoError(t, repo.Update(ctx, got))
	assert.Equal(t, 45, got.ThresholdDays)
	assert.False(t, got.Enabled)
	assert.Equal(t, []string{}, got.Mediums, "a nil Mediums persists as {} (any medium), never NULL")

	deleted, err := repo.Delete(ctx, teamID, rule.ID)
	require.NoError(t, err)
	assert.True(t, deleted)

	missing, err := repo.GetByID(ctx, teamID, rule.ID)
	require.NoError(t, err)
	assert.Nil(t, missing)
}

// A NULL project_id is the "any project" scope and must be storable.
func TestIntegrationFreshnessRule_NullProjectMeansAnyProject(t *testing.T) {
	resetFreshnessTables(t)
	teamID, _ := seedFreshnessScope(t)
	repo := NewFreshnessRuleRepository(integrationDB)
	ctx := context.Background()

	rule := &models.FreshnessRule{
		TeamID: teamID, ResourceTypes: []string{"memory"}, ThresholdDays: 30, Enabled: true,
	}
	require.NoError(t, repo.Create(ctx, rule))

	got, err := repo.GetByID(ctx, teamID, rule.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.ProjectID)
	assert.Equal(t, []string{}, got.Mediums)
}

// Tenancy: another team's rule id must be indistinguishable from a
// non-existent one, on both read and write paths.
func TestIntegrationFreshnessRule_ScopedToTeam(t *testing.T) {
	resetFreshnessTables(t)
	teamID, projectID := seedFreshnessScope(t)
	otherTeamID, _ := seedFreshnessScope(t)
	repo := NewFreshnessRuleRepository(integrationDB)
	ctx := context.Background()

	rule := &models.FreshnessRule{
		TeamID: teamID, ProjectID: &projectID,
		ResourceTypes: []string{"artifact"}, ThresholdDays: 10, Enabled: true,
	}
	require.NoError(t, repo.Create(ctx, rule))

	got, err := repo.GetByID(ctx, otherTeamID, rule.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "another team's rule must read as absent")

	deleted, err := repo.Delete(ctx, otherTeamID, rule.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "another team must not be able to delete the rule")

	stolen := &models.FreshnessRule{
		TeamID: otherTeamID, ID: rule.ID,
		ResourceTypes: []string{"memory"}, ThresholdDays: 1, Enabled: false,
	}
	err = repo.Update(ctx, stolen)
	assert.ErrorIs(t, err, repositories.ErrFreshnessRuleNotFound)
}

func TestIntegrationFreshnessRule_ListByTeamEnabledOnly(t *testing.T) {
	resetFreshnessTables(t)
	teamID, _ := seedFreshnessScope(t)
	repo := NewFreshnessRuleRepository(integrationDB)
	ctx := context.Background()

	enabled := &models.FreshnessRule{
		TeamID: teamID, ResourceTypes: []string{"artifact"}, ThresholdDays: 10, Enabled: true,
	}
	disabled := &models.FreshnessRule{
		TeamID: teamID, ResourceTypes: []string{"prompt"}, ThresholdDays: 20, Enabled: false,
	}
	require.NoError(t, repo.Create(ctx, enabled))
	require.NoError(t, repo.Create(ctx, disabled))

	all, err := repo.ListByTeam(ctx, teamID, false)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	only, err := repo.ListByTeam(ctx, teamID, true)
	require.NoError(t, err)
	require.Len(t, only, 1)
	assert.Equal(t, enabled.ID, only[0].ID)
}

// The storage invariants the service layer must not be the only guard for.
func TestIntegrationFreshnessRule_ChecksRejectDegenerateInput(t *testing.T) {
	resetFreshnessTables(t)
	teamID, _ := seedFreshnessScope(t)
	repo := NewFreshnessRuleRepository(integrationDB)
	ctx := context.Background()

	tests := []struct {
		name       string
		rule       *models.FreshnessRule
		constraint string
	}{
		{
			name: "zero threshold",
			rule: &models.FreshnessRule{
				TeamID: teamID, ResourceTypes: []string{"artifact"}, ThresholdDays: 0,
			},
			constraint: "freshness_rules_threshold_positive",
		},
		{
			name: "negative threshold",
			rule: &models.FreshnessRule{
				TeamID: teamID, ResourceTypes: []string{"artifact"}, ThresholdDays: -1,
			},
			constraint: "freshness_rules_threshold_positive",
		},
		{
			name: "no resource types",
			rule: &models.FreshnessRule{
				TeamID: teamID, ResourceTypes: []string{}, ThresholdDays: 30,
			},
			constraint: "freshness_rules_resource_types_nonempty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.rule)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.constraint)
		})
	}
}

// Rules are scoped to a project by FK; deleting the project must take its
// rules with it rather than leave them pointing at nothing.
func TestIntegrationFreshnessRule_ProjectCascade(t *testing.T) {
	resetFreshnessTables(t)
	teamID, projectID := seedFreshnessScope(t)
	repo := NewFreshnessRuleRepository(integrationDB)
	ctx := context.Background()

	rule := &models.FreshnessRule{
		TeamID: teamID, ProjectID: &projectID,
		ResourceTypes: []string{"artifact"}, ThresholdDays: 30, Enabled: true,
	}
	require.NoError(t, repo.Create(ctx, rule))

	_, err := integrationDB.ExecContext(ctx, "DELETE FROM projects WHERE id = $1", projectID)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, teamID, rule.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// team_freshness_settings
// ---------------------------------------------------------------------------

func TestIntegrationTeamFreshnessSettings_UpsertBumpsVersion(t *testing.T) {
	resetFreshnessTables(t)
	teamID, _ := seedFreshnessScope(t)
	repo := NewTeamFreshnessSettingsRepository(integrationDB)
	ctx := context.Background()

	missing, err := repo.Get(ctx, teamID)
	require.NoError(t, err)
	assert.Nil(t, missing, "no row means the team inherits the defaults")

	settings := &models.TeamFreshnessSettings{
		TeamID:               teamID,
		IntervalSeconds:      models.DefaultFreshnessIntervalSeconds,
		ReversibilityEnabled: true,
	}
	require.NoError(t, repo.Upsert(ctx, settings))
	assert.Equal(t, int64(1), settings.Version)

	settings.IntervalSeconds = 7200
	settings.ReversibilityEnabled = false
	require.NoError(t, repo.Upsert(ctx, settings))
	assert.Equal(t, int64(2), settings.Version, "a conflicting upsert must bump the version")

	got, err := repo.Get(ctx, teamID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 7200, got.IntervalSeconds)
	assert.False(t, got.ReversibilityEnabled)

	require.NoError(t, repo.Delete(ctx, teamID))
	after, err := repo.Get(ctx, teamID)
	require.NoError(t, err)
	assert.Nil(t, after, "delete is the reset-to-defaults path")

	require.NoError(t, repo.Delete(ctx, teamID), "deleting a missing row is a no-op")
}

// The one-hour floor is a storage invariant, not just service validation.
func TestIntegrationTeamFreshnessSettings_IntervalFloorEnforced(t *testing.T) {
	resetFreshnessTables(t)
	teamID, _ := seedFreshnessScope(t)
	repo := NewTeamFreshnessSettingsRepository(integrationDB)
	ctx := context.Background()

	err := repo.Upsert(ctx, &models.TeamFreshnessSettings{
		TeamID:          teamID,
		IntervalSeconds: models.MinFreshnessIntervalSeconds - 1,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "team_freshness_settings_interval_floor")

	require.NoError(t, repo.Upsert(ctx, &models.TeamFreshnessSettings{
		TeamID:          teamID,
		IntervalSeconds: models.MinFreshnessIntervalSeconds,
	}), "exactly the floor must be accepted")
}

// ---------------------------------------------------------------------------
// resource_freshness_audit
// ---------------------------------------------------------------------------

// Every entry of one rule run shares a created_at (now() is transaction-start
// time), so ordering must fall back to id to stay stable. Writing the batch in
// one transaction is what reproduces the tie.
func TestIntegrationFreshnessAudit_ListOrderingIsStableAcrossTiedTimestamps(t *testing.T) {
	resetFreshnessTables(t)
	teamID, _ := seedFreshnessScope(t)
	repo := NewFreshnessAuditRepository(integrationDB)
	ctx := context.Background()

	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	for i := 0; i < 6; i++ {
		_, err = tx.ExecContext(ctx,
			"INSERT INTO resource_freshness_audit (team_id, resource_type, resource_id, action, reason) "+
				"VALUES ($1, $2, $3, $4, $5)",
			teamID, "artifact", uuid.New().String(),
			models.FreshnessActionMarked, models.FreshnessReasonRuleRun)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())

	var tiedTimestamps int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT count(DISTINCT created_at) FROM resource_freshness_audit WHERE team_id = $1",
		teamID).Scan(&tiedTimestamps))
	require.Equal(t, 1, tiedTimestamps,
		"precondition: one transaction must write one created_at to every row")

	firstPage, total, err := repo.ListByTeam(ctx, teamID, 3, 0)
	require.NoError(t, err)
	assert.Equal(t, 6, total)
	require.Len(t, firstPage, 3)

	secondPage, _, err := repo.ListByTeam(ctx, teamID, 3, 3)
	require.NoError(t, err)
	require.Len(t, secondPage, 3)

	seen := make(map[string]bool, 6)
	for _, entry := range append(append([]*models.ResourceFreshnessAudit{}, firstPage...), secondPage...) {
		assert.False(t, seen[entry.ID], "entry %s appeared on both pages", entry.ID)
		seen[entry.ID] = true
	}
	assert.Len(t, seen, 6, "paging must visit every entry exactly once despite tied timestamps")
}

func TestIntegrationFreshnessAudit_CreateAndTeamScope(t *testing.T) {
	resetFreshnessTables(t)
	teamID, projectID := seedFreshnessScope(t)
	otherTeamID, _ := seedFreshnessScope(t)
	ruleRepo := NewFreshnessRuleRepository(integrationDB)
	repo := NewFreshnessAuditRepository(integrationDB)
	ctx := context.Background()

	rule := &models.FreshnessRule{
		TeamID: teamID, ProjectID: &projectID,
		ResourceTypes: []string{"artifact"}, ThresholdDays: 30, Enabled: true,
	}
	require.NoError(t, ruleRepo.Create(ctx, rule))

	marked := &models.ResourceFreshnessAudit{
		TeamID: teamID, ResourceType: "artifact", ResourceID: uuid.New().String(),
		RuleID: &rule.ID, Action: models.FreshnessActionMarked,
		Reason: models.FreshnessReasonRuleRun,
	}
	require.NoError(t, repo.Create(ctx, marked))
	require.NotEmpty(t, marked.ID)

	cleared := &models.ResourceFreshnessAudit{
		TeamID: teamID, ResourceType: "prompt", ResourceID: uuid.New().String(),
		Action: models.FreshnessActionCleared, Reason: models.FreshnessReasonAccessed,
	}
	require.NoError(t, repo.Create(ctx, cleared), "a clear has no rule and must accept a NULL rule_id")

	entries, total, err := repo.ListByTeam(ctx, teamID, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, entries, 2, "a zero limit means no limit")

	_, otherTotal, err := repo.ListByTeam(ctx, otherTeamID, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, otherTotal, "the log is team-scoped")
}

// rule_id deliberately carries no foreign key: the log has to outlive the rule
// it references, so deleting the rule must leave the entry intact.
func TestIntegrationFreshnessAudit_SurvivesRuleDeletion(t *testing.T) {
	resetFreshnessTables(t)
	teamID, _ := seedFreshnessScope(t)
	ruleRepo := NewFreshnessRuleRepository(integrationDB)
	repo := NewFreshnessAuditRepository(integrationDB)
	ctx := context.Background()

	rule := &models.FreshnessRule{
		TeamID: teamID, ResourceTypes: []string{"artifact"}, ThresholdDays: 30, Enabled: true,
	}
	require.NoError(t, ruleRepo.Create(ctx, rule))

	entry := &models.ResourceFreshnessAudit{
		TeamID: teamID, ResourceType: "artifact", ResourceID: uuid.New().String(),
		RuleID: &rule.ID, Action: models.FreshnessActionMarked,
		Reason: models.FreshnessReasonRuleRun,
	}
	require.NoError(t, repo.Create(ctx, entry))

	deleted, err := ruleRepo.Delete(ctx, teamID, rule.ID)
	require.NoError(t, err)
	require.True(t, deleted)

	entries, total, err := repo.ListByTeam(ctx, teamID, 0, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total, "the audit entry must outlive the rule it names")
	require.NotNil(t, entries[0].RuleID)
	assert.Equal(t, rule.ID, *entries[0].RuleID)
}

// ---------------------------------------------------------------------------
// Index usage
// ---------------------------------------------------------------------------

// explainFreshness returns the plan for query, with seq scans disabled so the
// index preference is visible. Every plan line is joined, because an index can
// appear as a "Bitmap Index Scan" child rather than the top-level node.
func explainFreshness(t *testing.T, query string, args ...interface{}) string {
	t.Helper()
	ctx := context.Background()

	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off")
	require.NoError(t, err)

	rows, err := tx.QueryContext(ctx, "EXPLAIN "+query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var plan strings.Builder
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	require.NoError(t, rows.Err())
	return plan.String()
}

func TestIntegrationFreshness_ListingQueriesUseIndexes(t *testing.T) {
	resetFreshnessTables(t)
	teamID, projectID := seedFreshnessScope(t)
	ctx := context.Background()

	repo := NewResourceFreshnessRepository(integrationDB)
	for i := 0; i < 3; i++ {
		require.NoError(t, upsertFreshness(ctx, repo,
			newFreshnessState(teamID, projectID, "artifact", []string{uuid.New().String()})))
	}

	tests := []struct {
		name      string
		query     string
		args      []interface{}
		wantIndex string
	}{
		{
			name: "stale by team and type",
			query: "SELECT id FROM resource_freshness WHERE team_id = $1 AND resource_type = $2 " +
				"ORDER BY since DESC, id DESC",
			args:      []interface{}{teamID, "artifact"},
			wantIndex: "idx_resource_freshness_team_type",
		},
		{
			name: "stale by team and project",
			query: "SELECT id FROM resource_freshness WHERE team_id = $1 AND project_id = $2 " +
				"ORDER BY since DESC, id DESC",
			args:      []interface{}{teamID, projectID},
			wantIndex: "idx_resource_freshness_project_team",
		},
		{
			// The project-first column order exists for this: an ON DELETE
			// CASCADE from projects would otherwise scan the whole table.
			name:      "cascade from a deleted project",
			query:     "SELECT id FROM resource_freshness WHERE project_id = $1",
			args:      []interface{}{projectID},
			wantIndex: "idx_resource_freshness_project_team",
		},
		{
			name:      "cascade from a deleted project into rules",
			query:     "SELECT id FROM freshness_rules WHERE project_id = $1",
			args:      []interface{}{projectID},
			wantIndex: "idx_freshness_rules_project",
		},
		{
			name:      "state lookup by resource",
			query:     "SELECT id FROM resource_freshness WHERE resource_type = $1 AND resource_id = $2",
			args:      []interface{}{"artifact", uuid.New().String()},
			wantIndex: "idx_resource_freshness_resource",
		},
		{
			// Must mirror RemoveRule's predicate exactly. `= ANY (array)` is
			// NOT an indexable operator -- only containment (`@>`) is served
			// by the GIN array_ops index, and rule deletion would otherwise
			// seq-scan every freshness row.
			name:      "rows referencing a rule",
			query:     "SELECT id FROM resource_freshness WHERE matched_rule_ids @> ARRAY[$1::uuid]",
			args:      []interface{}{uuid.New().String()},
			wantIndex: "idx_resource_freshness_matched_rules",
		},
		{
			name: "audit list newest first",
			query: "SELECT id FROM resource_freshness_audit WHERE team_id = $1 " +
				"ORDER BY created_at DESC, id DESC",
			args:      []interface{}{teamID},
			wantIndex: "idx_resource_freshness_audit_team_created",
		},
		{
			name:      "enabled rules for a team",
			query:     "SELECT id FROM freshness_rules WHERE team_id = $1 AND enabled = true",
			args:      []interface{}{teamID},
			wantIndex: "idx_freshness_rules_team_enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := explainFreshness(t, tt.query, tt.args...)
			assert.Contains(t, plan, tt.wantIndex, "plan was:\n%s", plan)
		})
	}
}

// ---------------------------------------------------------------------------
// last_accessed_at columns
// ---------------------------------------------------------------------------

// The columns must exist on all four resource tables, be nullable, and start
// NULL — "start clean" is decision #7, and a NULL means "never accessed",
// which the rule engine treats as eligible for staleness.
func TestIntegrationFreshness_LastAccessedColumnsExistAndStartNull(t *testing.T) {
	resetFreshnessTables(t)
	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	projectID := insertTestProject(t, userID, teamID)
	ctx := context.Background()

	promptID := insertTestPrompt(t, userID, teamID, projectID, "Prompt", "body", "published")

	seeded := map[string]string{"prompts": promptID}
	for _, table := range []string{"artifacts", "blueprints", "memories"} {
		id := uuid.New().String()
		var err error
		switch table {
		case "artifacts":
			_, err = integrationDB.ExecContext(ctx,
				"INSERT INTO artifacts (id, user_id, team_id, project_id, slug, title, content) "+
					"VALUES ($1, $2, $3, $4, $5, $6, $7)",
				id, userID, teamID, projectID, "artifact-"+id[:8], "Artifact", "content")
		case "blueprints":
			// `path` is NOT NULL since 007_blueprint_sync.
			_, err = integrationDB.ExecContext(ctx,
				"INSERT INTO blueprints (id, user_id, team_id, project_id, slug, title, content, path) "+
					"VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
				id, userID, teamID, projectID, "blueprint-"+id[:8], "Blueprint", "content",
				".claude/blueprint-"+id[:8]+".md")
		case "memories":
			_, err = integrationDB.ExecContext(ctx,
				"INSERT INTO memories (id, user_id, team_id, project_id, text) VALUES ($1, $2, $3, $4, $5)",
				id, userID, teamID, projectID, "text")
		}
		require.NoError(t, err)
		seeded[table] = id
	}

	for table, id := range seeded {
		t.Run(table, func(t *testing.T) {
			var web, cli, mcp, api *time.Time
			require.NoError(t, integrationDB.QueryRowContext(ctx,
				"SELECT last_accessed_web_at, last_accessed_cli_at, last_accessed_mcp_at, "+
					"last_accessed_api_at FROM "+table+" WHERE id = $1", id).
				Scan(&web, &cli, &mcp, &api))

			assert.Nil(t, web)
			assert.Nil(t, cli)
			assert.Nil(t, mcp)
			assert.Nil(t, api)
		})
	}
}

// All four columns are timestamptz on every resource table, including
// `memories`, whose own created_at/updated_at are naive. The rule engine
// compares these against now(), so a naive column there would silently shift
// by the session time zone.
func TestIntegrationFreshness_LastAccessedColumnsAreTimestamptz(t *testing.T) {
	ctx := context.Background()

	for _, table := range []string{"prompts", "artifacts", "blueprints", "memories"} {
		t.Run(table, func(t *testing.T) {
			rows, err := integrationDB.QueryContext(ctx,
				"SELECT column_name, data_type, is_nullable FROM information_schema.columns "+
					"WHERE table_name = $1 AND column_name LIKE 'last_accessed%' ORDER BY column_name",
				table)
			require.NoError(t, err)
			defer func() { _ = rows.Close() }()

			var found int
			for rows.Next() {
				var name, dataType, nullable string
				require.NoError(t, rows.Scan(&name, &dataType, &nullable))
				assert.Equal(t, "timestamp with time zone", dataType, "column %s", name)
				assert.Equal(t, "YES", nullable, "column %s must stay nullable", name)
				found++
			}
			require.NoError(t, rows.Err())
			assert.Equal(t, 4, found, "expected one column per medium (web/cli/mcp/api)")
		})
	}
}
