//go:build integration

package postgres

import (
	"context"
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
	require.NoError(t, f.repo.Create(context.Background(), status))
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

	renamed, err := f.repo.UpdateName(ctx, f.teamA, created.ID, "Under review")
	require.NoError(t, err)
	assert.Equal(t, "Under review", renamed.Name)
	assert.Equal(t, "in-review", renamed.Slug, "a rename never changes the slug")
	assert.False(t, renamed.UpdatedAt.Before(created.UpdatedAt))

	require.NoError(t, f.repo.Delete(ctx, f.teamA, created.ID))
	_, err = f.repo.GetByID(ctx, f.teamA, created.ID)
	require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
	require.ErrorIs(t, f.repo.Delete(ctx, f.teamA, created.ID), repositories.ErrTeamStatusNotFound)
	_, err = f.repo.UpdateName(ctx, f.teamA, created.ID, "Gone")
	require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
}

func TestIntegrationTeamStatusRepository_CreateWithoutCreator(t *testing.T) {
	f := newTeamStatusFixture(t)

	status := &models.TeamStatus{
		TeamID: f.teamA, ResourceType: models.StatusResourceTypeMemory, Slug: "verified", Name: "Verified",
	}
	require.NoError(t, f.repo.Create(context.Background(), status))

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
	require.ErrorIs(t, f.repo.Create(context.Background(), dup), repositories.ErrTeamStatusExists)

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
	})
	require.ErrorContains(t, err, "team or creator not found")

	err = f.repo.Create(context.Background(), &models.TeamStatus{
		TeamID: f.teamA, ResourceType: models.StatusResourceTypeArtifact, Slug: "x", Name: "X",
		CreatedBy: &ghost,
	})
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
	_, err = f.repo.UpdateName(ctx, f.teamA, theirs.ID, "Hijacked")
	require.ErrorIs(t, err, repositories.ErrTeamStatusNotFound)
	require.ErrorIs(t, f.repo.Delete(ctx, f.teamA, theirs.ID), repositories.ErrTeamStatusNotFound)

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
	require.NoError(t, repo.Create(ctx, status))

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
