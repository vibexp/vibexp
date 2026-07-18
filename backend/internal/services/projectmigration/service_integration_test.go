//go:build integration

package projectmigration

import (
	"context"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// migrationFixture is the common seed for the migration integration tests:
// one user owning one team with a source and a destination project.
type migrationFixture struct {
	userID string
	teamID string
	srcID  string
	dstID  string
}

func newMigrationFixture(t *testing.T) migrationFixture {
	t.Helper()
	resetIntegrationTables(t)
	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	return migrationFixture{
		userID: userID,
		teamID: teamID,
		srcID:  insertTestProject(t, userID, teamID, "Source Project"),
		dstID:  insertTestProject(t, userID, teamID, "Destination Project"),
	}
}

// allResources selects every resource type, mirroring a full-project move.
func allResources() ResourceSelections {
	all := ResourceSelection{All: true}
	return ResourceSelections{Prompts: all, Artifacts: all, Blueprints: all, FeedItems: all}
}

func inventoryIDs(inv ResourceInventory) []string {
	ids := make([]string, 0, len(inv.Items))
	for _, item := range inv.Items {
		ids = append(ids, item.ID)
	}
	return ids
}

func TestIntegrationGetInventory_CountsPerType(t *testing.T) {
	ctx := context.Background()
	fx := newMigrationFixture(t)
	svc := newIntegrationService()

	prompt1 := insertTestPrompt(t, fx.userID, fx.teamID, fx.srcID, "inv-prompt-1")
	prompt2 := insertTestPrompt(t, fx.userID, fx.teamID, fx.srcID, "inv-prompt-2")
	artifact1 := insertTestArtifact(t, fx.userID, fx.teamID, fx.srcID, "inv-artifact-1")
	artifact2 := insertTestArtifact(t, fx.userID, fx.teamID, fx.srcID, "inv-artifact-2")
	blueprint1 := insertTestBlueprint(t, fx.userID, fx.teamID, fx.srcID, "inv-blueprint-1", 1)
	feedID := insertTestFeed(t, fx.teamID, fx.userID)
	feedItem1 := insertTestFeedItem(t, fx.teamID, feedID, fx.srcID, fx.userID, "Feed Item One")

	// Resources in another project of the same team must not leak into the
	// source project's inventory.
	insertTestPrompt(t, fx.userID, fx.teamID, fx.dstID, "other-project-prompt")
	insertTestArtifact(t, fx.userID, fx.teamID, fx.dstID, "other-project-artifact")

	inv, err := svc.GetInventory(ctx, fx.userID, fx.teamID, fx.srcID)
	require.NoError(t, err)

	assert.Equal(t, 2, inv.Prompts.Count)
	assert.ElementsMatch(t, []string{prompt1, prompt2}, inventoryIDs(inv.Prompts))
	assert.Equal(t, 2, inv.Artifacts.Count)
	assert.ElementsMatch(t, []string{artifact1, artifact2}, inventoryIDs(inv.Artifacts))
	assert.Equal(t, 1, inv.Blueprints.Count)
	assert.ElementsMatch(t, []string{blueprint1}, inventoryIDs(inv.Blueprints))
	assert.Equal(t, 1, inv.FeedItems.Count)
	assert.ElementsMatch(t, []string{feedItem1}, inventoryIDs(inv.FeedItems))

	// Item names surface the slug for slugged resources and the title for feed items.
	assert.ElementsMatch(t, []string{"inv-prompt-1", "inv-prompt-2"},
		[]string{inv.Prompts.Items[0].Name, inv.Prompts.Items[1].Name})
	assert.Equal(t, "Feed Item One", inv.FeedItems.Items[0].Name)
}

func TestIntegrationGetInventory_EmptyProject(t *testing.T) {
	ctx := context.Background()
	fx := newMigrationFixture(t)
	svc := newIntegrationService()

	inv, err := svc.GetInventory(ctx, fx.userID, fx.teamID, fx.srcID)
	require.NoError(t, err)

	assert.Equal(t, 0, inv.Prompts.Count)
	assert.Empty(t, inv.Prompts.Items)
	assert.Equal(t, 0, inv.Artifacts.Count)
	assert.Empty(t, inv.Artifacts.Items)
	assert.Equal(t, 0, inv.Blueprints.Count)
	assert.Empty(t, inv.Blueprints.Items)
	assert.Equal(t, 0, inv.FeedItems.Count)
	assert.Empty(t, inv.FeedItems.Items)
}

func TestIntegrationGetInventory_Guards(t *testing.T) {
	ctx := context.Background()
	fx := newMigrationFixture(t)
	otherTeamID := insertTestTeam(t, fx.userID)
	strangerID := insertTestUser(t)
	svc := newIntegrationService()

	t.Run("team mismatch", func(t *testing.T) {
		_, err := svc.GetInventory(ctx, fx.userID, otherTeamID, fx.srcID)
		require.ErrorIs(t, err, ErrTeamMismatch)
	})

	t.Run("nonexistent project", func(t *testing.T) {
		_, err := svc.GetInventory(ctx, fx.userID, fx.teamID, uuid.New().String())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source project not accessible")
	})

	t.Run("user without team access", func(t *testing.T) {
		_, err := svc.GetInventory(ctx, strangerID, fx.teamID, fx.srcID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "source project not accessible")
	})
}

func TestIntegrationMigrate_AllResourcesMoved(t *testing.T) {
	ctx := context.Background()
	fx := newMigrationFixture(t)
	svc := newIntegrationService()

	prompt1 := insertTestPrompt(t, fx.userID, fx.teamID, fx.srcID, "mig-prompt-1")
	prompt2 := insertTestPrompt(t, fx.userID, fx.teamID, fx.srcID, "mig-prompt-2")
	artifact1 := insertTestArtifact(t, fx.userID, fx.teamID, fx.srcID, "mig-artifact-1")
	artifact2 := insertTestArtifact(t, fx.userID, fx.teamID, fx.srcID, "mig-artifact-2")
	blueprint1 := insertTestBlueprint(t, fx.userID, fx.teamID, fx.srcID, "mig-blueprint-1", 1)
	feedID := insertTestFeed(t, fx.teamID, fx.userID)
	feedItem1 := insertTestFeedItem(t, fx.teamID, feedID, fx.srcID, fx.userID, "Migrating Feed Item")

	// A bystander project in the same team must be untouched by the migration.
	bystanderID := insertTestProject(t, fx.userID, fx.teamID, "Bystander Project")
	bystanderPrompt := insertTestPrompt(t, fx.userID, fx.teamID, bystanderID, "bystander-prompt")

	result, err := svc.Migrate(ctx, fx.userID, fx.teamID, fx.srcID, &MigrationRequest{
		DestinationProjectID: fx.dstID,
		Resources:            allResources(),
		ConflictPolicy:       ConflictPolicySkip,
	})
	require.NoError(t, err)

	assert.Equal(t, ResourceMigrationCounts{Prompts: 2, Artifacts: 2, Blueprints: 1, FeedItems: 1}, result.Migrated)
	assert.Equal(t, ResourceMigrationOutcomes{}, result.Skipped)
	assert.Equal(t, ResourceMigrationOutcomes{}, result.Failed)
	assert.Equal(t, "Source Project", result.SourceProjectName)
	assert.Equal(t, "Destination Project", result.DestinationProjectName)

	// Every selected resource is now reparented to the destination project.
	for _, row := range []struct{ table, id string }{
		{"prompts", prompt1}, {"prompts", prompt2},
		{"artifacts", artifact1}, {"artifacts", artifact2},
		{"blueprints", blueprint1},
		{"feed_items", feedItem1},
	} {
		assert.Equal(t, fx.dstID, projectIDOf(t, row.table, row.id), "%s %s should live in the destination", row.table, row.id)
	}

	// The source project is left with no strays of any type.
	for _, table := range []string{"prompts", "artifacts", "blueprints", "feed_items"} {
		assert.Zero(t, countInProject(t, table, fx.srcID), "source project should have no %s left", table)
	}

	// The bystander project is untouched.
	assert.Equal(t, bystanderID, projectIDOf(t, "prompts", bystanderPrompt))

	// Moves are updates in place: the version counter is bumped, proving the
	// row was reparented rather than copied.
	var promptVersion int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT version FROM prompts WHERE id = $1", prompt1).Scan(&promptVersion))
	assert.Equal(t, int64(2), promptVersion)
}

func TestIntegrationMigrate_PartialSelection(t *testing.T) {
	ctx := context.Background()
	fx := newMigrationFixture(t)
	svc := newIntegrationService()

	prompt1 := insertTestPrompt(t, fx.userID, fx.teamID, fx.srcID, "part-prompt-1")
	prompt2 := insertTestPrompt(t, fx.userID, fx.teamID, fx.srcID, "part-prompt-2")
	artifactMoved := insertTestArtifact(t, fx.userID, fx.teamID, fx.srcID, "part-artifact-moved")
	artifactStays := insertTestArtifact(t, fx.userID, fx.teamID, fx.srcID, "part-artifact-stays")
	blueprintStays := insertTestBlueprint(t, fx.userID, fx.teamID, fx.srcID, "part-blueprint-stays", 1)
	feedID := insertTestFeed(t, fx.teamID, fx.userID)
	feedItemStays := insertTestFeedItem(t, fx.teamID, feedID, fx.srcID, fx.userID, "Staying Feed Item")

	// IDs from outside the source project must be silently filtered out by
	// the source-project ownership checks in resolveIDs / querySlugRows.
	foreignArtifact := insertTestArtifact(t, fx.userID, fx.teamID, fx.dstID, "part-artifact-foreign")
	foreignPrompt := insertTestPrompt(t, fx.userID, fx.teamID, fx.dstID, "part-prompt-foreign")

	result, err := svc.Migrate(ctx, fx.userID, fx.teamID, fx.srcID, &MigrationRequest{
		DestinationProjectID: fx.dstID,
		Resources: ResourceSelections{
			Prompts:   ResourceSelection{IDs: []string{prompt1, foreignPrompt}},
			Artifacts: ResourceSelection{IDs: []string{artifactMoved, foreignArtifact}},
			// Blueprints and FeedItems are not selected at all.
		},
		ConflictPolicy: ConflictPolicySkip,
	})
	require.NoError(t, err)

	assert.Equal(t, ResourceMigrationCounts{Prompts: 1, Artifacts: 1, Blueprints: 0, FeedItems: 0}, result.Migrated)

	// Selected resources moved.
	assert.Equal(t, fx.dstID, projectIDOf(t, "prompts", prompt1))
	assert.Equal(t, fx.dstID, projectIDOf(t, "artifacts", artifactMoved))

	// Unselected resources stayed in the source project.
	assert.Equal(t, fx.srcID, projectIDOf(t, "prompts", prompt2))
	assert.Equal(t, fx.srcID, projectIDOf(t, "artifacts", artifactStays))
	assert.Equal(t, fx.srcID, projectIDOf(t, "blueprints", blueprintStays))
	assert.Equal(t, fx.srcID, projectIDOf(t, "feed_items", feedItemStays))

	// The foreign resources never belonged to the source and are untouched.
	assert.Equal(t, fx.dstID, projectIDOf(t, "artifacts", foreignArtifact))
	assert.Equal(t, fx.dstID, projectIDOf(t, "prompts", foreignPrompt))
}

// TestIntegrationMigrate_RollbackOnMidTransactionFailure pins the
// single-transaction guarantee against real Postgres. The natural seam the
// issue suggests — a (project_id, slug) collision in the destination — is
// unreachable with real data: artifacts_slug_team_id_key / blueprints_slug_team_id_key
// make slugs team-unique, so a same-team destination can never already hold a
// colliding slug. Instead the failure is induced with real row data: a
// blueprint seeded with version = math.MaxInt64 makes the service's in-transaction
// "SET version = version + 1" overflow bigint (SQLSTATE 22003). Postgres
// aborts the whole transaction at that point, so the prompts and artifacts
// already reparented earlier in the same transaction must be rolled back.
func TestIntegrationMigrate_RollbackOnMidTransactionFailure(t *testing.T) {
	ctx := context.Background()
	fx := newMigrationFixture(t)
	svc := newIntegrationService()

	prompt1 := insertTestPrompt(t, fx.userID, fx.teamID, fx.srcID, "rb-prompt-1")
	artifact1 := insertTestArtifact(t, fx.userID, fx.teamID, fx.srcID, "rb-artifact-1")
	blueprintOverflow := insertTestBlueprint(t, fx.userID, fx.teamID, fx.srcID, "rb-blueprint-overflow", math.MaxInt64)
	feedID := insertTestFeed(t, fx.teamID, fx.userID)
	feedItem1 := insertTestFeedItem(t, fx.teamID, feedID, fx.srcID, fx.userID, "Rollback Feed Item")

	result, err := svc.Migrate(ctx, fx.userID, fx.teamID, fx.srcID, &MigrationRequest{
		DestinationProjectID: fx.dstID,
		Resources:            allResources(),
		ConflictPolicy:       ConflictPolicySkip,
	})
	require.Error(t, err)
	assert.Nil(t, result)

	// NOTHING moved: prompts and artifacts were updated inside the transaction
	// before the blueprint failure, and must have been rolled back with it.
	assert.Equal(t, fx.srcID, projectIDOf(t, "prompts", prompt1))
	assert.Equal(t, fx.srcID, projectIDOf(t, "artifacts", artifact1))
	assert.Equal(t, fx.srcID, projectIDOf(t, "blueprints", blueprintOverflow))
	assert.Equal(t, fx.srcID, projectIDOf(t, "feed_items", feedItem1))
	for _, table := range []string{"prompts", "artifacts", "blueprints", "feed_items"} {
		assert.Zero(t, countInProject(t, table, fx.dstID), "destination project must stay empty of %s", table)
	}

	// The rolled-back prompt update must not have left a version bump behind.
	var promptVersion int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT version FROM prompts WHERE id = $1", prompt1).Scan(&promptVersion))
	assert.Equal(t, int64(1), promptVersion)
}

// TestIntegrationMigrate_RollbackOnPromptFailure is the first-stage variant:
// the failure hits prompts (the first resource type migrated). The service
// records the per-row failure and keeps going, but Postgres has aborted the
// transaction, so the artifacts-stage query fails and the migration errors
// out with nothing moved.
func TestIntegrationMigrate_RollbackOnPromptFailure(t *testing.T) {
	ctx := context.Background()
	fx := newMigrationFixture(t)
	svc := newIntegrationService()

	promptOverflow := insertTestPromptWithVersion(t, fx.userID, fx.teamID, fx.srcID, "rb-prompt-overflow", math.MaxInt64)
	artifact1 := insertTestArtifact(t, fx.userID, fx.teamID, fx.srcID, "rb-artifact-untouched")

	result, err := svc.Migrate(ctx, fx.userID, fx.teamID, fx.srcID, &MigrationRequest{
		DestinationProjectID: fx.dstID,
		Resources:            allResources(),
		ConflictPolicy:       ConflictPolicySkip,
	})
	require.Error(t, err)
	assert.Nil(t, result)

	assert.Equal(t, fx.srcID, projectIDOf(t, "prompts", promptOverflow))
	assert.Equal(t, fx.srcID, projectIDOf(t, "artifacts", artifact1))
	assert.Zero(t, countInProject(t, "prompts", fx.dstID))
	assert.Zero(t, countInProject(t, "artifacts", fx.dstID))
}

func TestIntegrationMigrate_Guards(t *testing.T) {
	ctx := context.Background()
	fx := newMigrationFixture(t)
	svc := newIntegrationService()

	// A second team owned by the same user, with its own project: accessible,
	// but in a different team than the source.
	otherTeamID := insertTestTeam(t, fx.userID)
	otherTeamProjectID := insertTestProject(t, fx.userID, otherTeamID, "Other Team Project")

	// A team the acting user has no relationship with at all.
	strangerID := insertTestUser(t)
	strangerTeamID := insertTestTeam(t, strangerID)
	strangerProjectID := insertTestProject(t, strangerID, strangerTeamID, "Stranger Project")

	promptID := insertTestPrompt(t, fx.userID, fx.teamID, fx.srcID, "guard-prompt")

	tests := []struct {
		name            string
		teamID          string
		sourceID        string
		destinationID   string
		wantErrIs       error
		wantErrContains string
	}{
		{
			name:          "source team mismatch",
			teamID:        otherTeamID,
			sourceID:      fx.srcID,
			destinationID: fx.dstID,
			wantErrIs:     ErrTeamMismatch,
		},
		{
			name:            "cross-team destination rejected",
			teamID:          fx.teamID,
			sourceID:        fx.srcID,
			destinationID:   otherTeamProjectID,
			wantErrContains: "cross-team migration not supported",
		},
		{
			name:            "nonexistent source project",
			teamID:          fx.teamID,
			sourceID:        uuid.New().String(),
			destinationID:   fx.dstID,
			wantErrContains: "source project not accessible",
		},
		{
			name:            "nonexistent destination project",
			teamID:          fx.teamID,
			sourceID:        fx.srcID,
			destinationID:   uuid.New().String(),
			wantErrContains: "destination project not accessible",
		},
		{
			name:            "source project in an inaccessible team",
			teamID:          strangerTeamID,
			sourceID:        strangerProjectID,
			destinationID:   fx.dstID,
			wantErrContains: "source project not accessible",
		},
		{
			name:            "destination project inaccessible to the user",
			teamID:          fx.teamID,
			sourceID:        fx.srcID,
			destinationID:   strangerProjectID,
			wantErrContains: "destination project not accessible",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := svc.Migrate(ctx, fx.userID, tc.teamID, tc.sourceID, &MigrationRequest{
				DestinationProjectID: tc.destinationID,
				Resources:            allResources(),
				ConflictPolicy:       ConflictPolicySkip,
			})
			require.Error(t, err)
			assert.Nil(t, result)
			if tc.wantErrIs != nil {
				assert.ErrorIs(t, err, tc.wantErrIs)
			}
			if tc.wantErrContains != "" {
				assert.Contains(t, err.Error(), tc.wantErrContains)
			}
			// A rejected migration must never move anything.
			assert.Equal(t, fx.srcID, projectIDOf(t, "prompts", promptID))
		})
	}
}
