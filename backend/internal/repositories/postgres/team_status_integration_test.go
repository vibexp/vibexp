//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Integration tests for TeamStatusRepository against the migrated schema: the
// behaviour sqlmock cannot show -- the unique key, the two foreign keys'
// delete rules, and that no method ever reaches another team's rows.

type teamStatusFixture struct {
	repo             repositories.TeamStatusRepository
	userID           string
	teamA, teamOther string
}

func newTeamStatusFixture(t *testing.T) teamStatusFixture {
	t.Helper()
	userID := refScopeInsertUser(t, integrationDB.DB)
	return teamStatusFixture{
		repo:      NewTeamStatusRepository(integrationDB),
		userID:    userID,
		teamA:     refScopeInsertTeam(t, integrationDB.DB, userID),
		teamOther: refScopeInsertTeam(t, integrationDB.DB, userID),
	}
}

func (f teamStatusFixture) create(t *testing.T, teamID, resourceType, slug, name string) *models.TeamStatus {
	t.Helper()
	status := &models.TeamStatus{
		TeamID: teamID, ResourceType: resourceType, Slug: slug, Name: name, CreatedBy: &f.userID,
	}
	require.NoError(t, f.repo.Create(context.Background(), status, testStatusAudit))
	return status
}

func TestIntegrationTeamStatusRepository_CRUD(t *testing.T) {
	f := newTeamStatusFixture(t)
	ctx := context.Background()

	created := f.create(t, f.teamA, models.StatusResourceTypeArtifact, "in-review", "In review")
	require.NotEmpty(t, created.ID)
	assert.False(t, created.CreatedAt.IsZero())
	assert.Equal(t, created.CreatedAt, created.UpdatedAt)

	got, err := f.repo.GetByID(ctx, f.teamA, created.ID)
	require.NoError(t, err)
	assert.Equal(t, f.teamA, got.TeamID)
	assert.Equal(t, models.StatusResourceTypeArtifact, got.ResourceType)
	assert.Equal(t, "in-review", got.Slug)
	assert.Equal(t, "In review", got.Name)
	require.NotNil(t, got.CreatedBy)
	assert.Equal(t, f.userID, *got.CreatedBy)

	renamed, err := f.repo.UpdateName(ctx, f.teamA, created.ID, "Under review", testStatusAudit)
	require.NoError(t, err)
	assert.Equal(t, "Under review", renamed.Name)
	assert.Equal(t, "in-review", renamed.Slug, "a rename never changes the slug")
	assert.False(t, renamed.UpdatedAt.Before(created.UpdatedAt))

	deleted, reassigned, err := f.repo.DeleteAndReassign(
		ctx, f.teamA, created.ID, models.StatusActive, testStatusAudit)
	require.NoError(t, err)
	assert.Equal(t, created.ID, deleted.ID)
	assert.Equal(t, "Under review", deleted.Name, "the deleted row is returned as it was stored")
	assert.Zero(t, reassigned, "no resource carried the status")
	_, err = f.repo.GetByID(ctx, f.teamA, created.ID)
	require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
	_, _, err = f.repo.DeleteAndReassign(ctx, f.teamA, created.ID, models.StatusActive, testStatusAudit)
	require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
	_, err = f.repo.UpdateName(ctx, f.teamA, created.ID, "Gone", testStatusAudit)
	require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
}

func TestIntegrationTeamStatusRepository_CreateWithoutCreator(t *testing.T) {
	f := newTeamStatusFixture(t)

	status := &models.TeamStatus{
		TeamID: f.teamA, ResourceType: models.StatusResourceTypeMemory, Slug: "verified", Name: "Verified",
	}
	require.NoError(t, f.repo.Create(context.Background(), status, testStatusAudit))

	got, err := f.repo.GetByID(context.Background(), f.teamA, status.ID)
	require.NoError(t, err)
	assert.Nil(t, got.CreatedBy)
}

func TestIntegrationTeamStatusRepository_ListAndCount(t *testing.T) {
	f := newTeamStatusFixture(t)
	ctx := context.Background()

	// Inserted out of order; two names tie so the slug breaks it.
	f.create(t, f.teamA, models.StatusResourceTypeArtifact, "shipped", "Shipped")
	f.create(t, f.teamA, models.StatusResourceTypeArtifact, "review-b", "In review")
	f.create(t, f.teamA, models.StatusResourceTypeArtifact, "review-a", "In review")
	f.create(t, f.teamA, models.StatusResourceTypeMemory, "verified", "Verified")

	artifacts, err := f.repo.List(ctx, f.teamA, models.StatusResourceTypeArtifact)
	require.NoError(t, err)
	slugs := make([]string, 0, len(artifacts))
	for _, status := range artifacts {
		slugs = append(slugs, status.Slug)
	}
	assert.Equal(t, []string{"review-a", "review-b", "shipped"}, slugs, "name, then slug")

	count, err := f.repo.Count(ctx, f.teamA, models.StatusResourceTypeArtifact)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// A resource type with no rows: empty, not nil, and a zero count.
	prompts, err := f.repo.List(ctx, f.teamA, models.StatusResourceTypePrompt)
	require.NoError(t, err)
	assert.NotNil(t, prompts)
	assert.Empty(t, prompts)
	count, err = f.repo.Count(ctx, f.teamA, models.StatusResourceTypePrompt)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestIntegrationTeamStatusRepository_Uniqueness(t *testing.T) {
	f := newTeamStatusFixture(t)

	f.create(t, f.teamA, models.StatusResourceTypeArtifact, "in-review", "In review")

	// Same team, type and slug: rejected, whatever the name.
	dup := &models.TeamStatus{
		TeamID: f.teamA, ResourceType: models.StatusResourceTypeArtifact, Slug: "in-review", Name: "Other",
	}
	require.ErrorIs(t, f.repo.Create(context.Background(), dup, testStatusAudit), repositories.ErrTeamStatusExists)

	// The key is the full triple: the slug is free on another resource type and
	// in another team.
	f.create(t, f.teamA, models.StatusResourceTypeMemory, "in-review", "In review")
	f.create(t, f.teamOther, models.StatusResourceTypeArtifact, "in-review", "In review")
}

func TestIntegrationTeamStatusRepository_UnknownTeamOrCreator(t *testing.T) {
	f := newTeamStatusFixture(t)
	ghost := uuid.New().String()

	err := f.repo.Create(context.Background(), &models.TeamStatus{
		TeamID: ghost, ResourceType: models.StatusResourceTypeArtifact, Slug: "x", Name: "X",
	}, testStatusAudit)
	require.ErrorContains(t, err, "team or creator not found")

	err = f.repo.Create(context.Background(), &models.TeamStatus{
		TeamID: f.teamA, ResourceType: models.StatusResourceTypeArtifact, Slug: "x", Name: "X",
		CreatedBy: &ghost,
	}, testStatusAudit)
	require.ErrorContains(t, err, "team or creator not found")
}

// Another team's status must be unreachable through every method, and
// indistinguishable from a missing one.
func TestIntegrationTeamStatusRepository_CrossTeamIsolation(t *testing.T) {
	f := newTeamStatusFixture(t)
	ctx := context.Background()

	mine := f.create(t, f.teamA, models.StatusResourceTypeArtifact, "in-review", "In review")
	theirs := f.create(t, f.teamOther, models.StatusResourceTypeArtifact, "secret-stage", "Secret stage")

	listed, err := f.repo.List(ctx, f.teamA, models.StatusResourceTypeArtifact)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, mine.ID, listed[0].ID)

	count, err := f.repo.Count(ctx, f.teamA, models.StatusResourceTypeArtifact)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	_, err = f.repo.GetByID(ctx, f.teamA, theirs.ID)
	require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
	_, err = f.repo.UpdateName(ctx, f.teamA, theirs.ID, "Hijacked", testStatusAudit)
	require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
	_, _, err = f.repo.DeleteAndReassign(ctx, f.teamA, theirs.ID, models.StatusActive, testStatusAudit)
	require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)

	// ...and the failed writes left the other team's row exactly as it was.
	untouched, err := f.repo.GetByID(ctx, f.teamOther, theirs.ID)
	require.NoError(t, err)
	assert.Equal(t, "Secret stage", untouched.Name)
	assert.Equal(t, theirs.UpdatedAt, untouched.UpdatedAt)
}

func TestIntegrationTeamStatusRepository_ForeignKeyDeleteRules(t *testing.T) {
	db := integrationDB.DB
	ctx := context.Background()
	repo := NewTeamStatusRepository(integrationDB)

	owner := refScopeInsertUser(t, db)
	creator := refScopeInsertUser(t, db)
	teamID := refScopeInsertTeam(t, db, owner)

	status := &models.TeamStatus{
		TeamID: teamID, ResourceType: models.StatusResourceTypeBlueprint, Slug: "proposed",
		Name: "Proposed", CreatedBy: &creator,
	}
	require.NoError(t, repo.Create(ctx, status, testStatusAudit))

	// Deleting the creator keeps the status and blanks created_by.
	_, err := db.Exec("DELETE FROM users WHERE id = $1", creator)
	require.NoError(t, err)
	got, err := repo.GetByID(ctx, teamID, status.ID)
	require.NoError(t, err)
	assert.Nil(t, got.CreatedBy)

	// Deleting the team takes its statuses with it.
	_, err = db.Exec("DELETE FROM teams WHERE id = $1", teamID)
	require.NoError(t, err)
	assert.Equal(t, 0, countRows(t, db, "SELECT count(*) FROM team_statuses WHERE team_id = $1", teamID))
}

// statusResourceTables maps each status resource type to its table, for the
// tests that must show a method reaches the right one.
var statusResourceTables = map[string]string{
	models.StatusResourceTypePrompt:    "prompts",
	models.StatusResourceTypeArtifact:  "artifacts",
	models.StatusResourceTypeBlueprint: "blueprints",
	models.StatusResourceTypeMemory:    "memories",
}

func TestIntegrationTeamStatusRepository_UsageCounts(t *testing.T) {
	ctx := context.Background()
	repo := NewTeamStatusRepository(integrationDB)

	for resourceType, table := range statusResourceTables {
		t.Run(resourceType, func(t *testing.T) {
			mine := newTeamStatusesFixture(t, integrationDB.DB)
			other := newTeamStatusesFixture(t, integrationDB.DB)
			defaultStatus := models.DefaultStatusFor(resourceType)

			mine.insert(t, table, defaultStatus)
			mine.insert(t, table, "in-review")
			mine.insert(t, table, "in-review")
			other.insert(t, table, "in-review")

			got, err := repo.UsageCounts(ctx, mine.teamID, resourceType)
			require.NoError(t, err)
			assert.Equal(t, map[string]int{defaultStatus: 1, "in-review": 2}, got,
				"another team's resources are never counted")
		})
	}

	t.Run("a team with no resources gets an empty map", func(t *testing.T) {
		f := newTeamStatusFixture(t)
		got, err := repo.UsageCounts(ctx, f.teamA, models.StatusResourceTypeArtifact)
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
}

func TestIntegrationTeamStatusRepository_DeleteAndReassign(t *testing.T) {
	ctx := context.Background()
	repo := NewTeamStatusRepository(integrationDB)

	create := func(t *testing.T, f teamStatusesFixture, resourceType, slug string) *models.TeamStatus {
		t.Helper()
		status := &models.TeamStatus{
			TeamID: f.teamID, ResourceType: resourceType, Slug: slug, Name: "In review",
		}
		require.NoError(t, repo.Create(ctx, status, testStatusAudit))
		return status
	}

	for resourceType, table := range statusResourceTables {
		t.Run(resourceType+" resources move to the default and keep updated_at", func(t *testing.T) {
			mine := newTeamStatusesFixture(t, integrationDB.DB)
			other := newTeamStatusesFixture(t, integrationDB.DB)
			defaultStatus := models.DefaultStatusFor(resourceType)

			status := create(t, mine, resourceType, "in-review")
			otherStatus := create(t, other, resourceType, "in-review")
			// The same slug on a different resource type of the same team is a
			// different status and must survive.
			siblingType := models.StatusResourceTypeArtifact
			if resourceType == siblingType {
				siblingType = models.StatusResourceTypeMemory
			}
			sibling := create(t, mine, siblingType, "in-review")
			siblingRow := mine.insert(t, statusResourceTables[siblingType], "in-review")

			carrying := []string{mine.insert(t, table, "in-review"), mine.insert(t, table, "in-review")}
			untouched := mine.insert(t, table, "shipped")
			foreign := other.insert(t, table, "in-review")

			before := make(map[string]teamStatusesSnapshot)
			for _, id := range append(carrying, untouched) {
				before[id] = mine.snapshot(t, table, id)
			}
			foreignBefore := other.snapshot(t, table, foreign)
			siblingBefore := mine.snapshot(t, statusResourceTables[siblingType], siblingRow)

			deleted, reassigned, err := repo.DeleteAndReassign(
				ctx, mine.teamID, status.ID, defaultStatus, testStatusAudit)
			require.NoError(t, err)
			assert.Equal(t, 2, reassigned)
			assert.Equal(t, status.ID, deleted.ID)
			assert.Equal(t, "in-review", deleted.Slug)
			assert.Equal(t, resourceType, deleted.ResourceType)

			for _, id := range carrying {
				after := mine.snapshot(t, table, id)
				assert.Equal(t, defaultStatus, after.status)
				assert.Equal(t, before[id].updatedAt, after.updatedAt,
					"a reassigned %s must not look edited", resourceType)
			}
			assert.Equal(t, before[untouched], mine.snapshot(t, table, untouched))
			assert.Equal(t, foreignBefore, other.snapshot(t, table, foreign),
				"another team's resources with the same slug are untouched")
			assert.Equal(t, siblingBefore, mine.snapshot(t, statusResourceTables[siblingType], siblingRow),
				"the same slug on another resource type is untouched")

			_, err = repo.GetByID(ctx, mine.teamID, status.ID)
			require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
			_, err = repo.GetByID(ctx, other.teamID, otherStatus.ID)
			require.NoError(t, err, "another team's status row survives")
			_, err = repo.GetByID(ctx, mine.teamID, sibling.ID)
			require.NoError(t, err, "the sibling resource type's status row survives")
		})
	}

	t.Run("a failed reassignment rolls the delete back", func(t *testing.T) {
		f := newTeamStatusesFixture(t, integrationDB.DB)
		status := create(t, f, models.StatusResourceTypeArtifact, "in-review")
		row := f.insert(t, "artifacts", "in-review")
		before := f.snapshot(t, "artifacts", row)

		// 51 characters cannot be stored in the varchar(50) status column, so
		// the UPDATE fails after the status row has already been deleted.
		_, reassigned, err := repo.DeleteAndReassign(
			ctx, f.teamID, status.ID, strings.Repeat("x", 51), testStatusAudit)
		require.Error(t, err)
		assert.Zero(t, reassigned)

		got, err := repo.GetByID(ctx, f.teamID, status.ID)
		require.NoError(t, err, "the status row must still exist")
		assert.Equal(t, "in-review", got.Slug)
		assert.Equal(t, before, f.snapshot(t, "artifacts", row))
	})

	t.Run("another team's id is not found and changes nothing", func(t *testing.T) {
		mine := newTeamStatusesFixture(t, integrationDB.DB)
		other := newTeamStatusesFixture(t, integrationDB.DB)
		status := create(t, other, models.StatusResourceTypeMemory, "in-review")
		row := other.insert(t, "memories", "in-review")
		before := other.snapshot(t, "memories", row)

		_, _, err := repo.DeleteAndReassign(ctx, mine.teamID, status.ID, models.StatusActive, testStatusAudit)
		require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)

		_, err = repo.GetByID(ctx, other.teamID, status.ID)
		require.NoError(t, err)
		assert.Equal(t, before, other.snapshot(t, "memories", row))
	})

	t.Run("an ordinary memory edit after a delete is still stamped", func(t *testing.T) {
		f := newTeamStatusesFixture(t, integrationDB.DB)
		status := create(t, f, models.StatusResourceTypeMemory, "in-review")
		row := f.insert(t, "memories", "in-review")

		_, _, err := repo.DeleteAndReassign(ctx, f.teamID, status.ID, models.StatusActive, testStatusAudit)
		require.NoError(t, err)
		reassignedAt := f.snapshot(t, "memories", row).updatedAt

		// The setting was transaction-local, so on any pooled connection the
		// trigger fires again.
		for range 5 {
			_, err = integrationDB.Exec("UPDATE memories SET text = text || '.' WHERE id = $1", row)
			require.NoError(t, err)
		}
		assert.True(t, f.snapshot(t, "memories", row).updatedAt.After(reassignedAt))
	})
}
