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

// insertCommentArtifact/Prompt/Memory/Blueprint seed a resource of each type in
// the access team so the recent-activity JOIN and ResourceExists can be
// exercised against real rows.
func insertCommentPrompt(t *testing.T, at accessTeam, creatorID, slug, name string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		`INSERT INTO prompts (id, name, slug, body, user_id, team_id, project_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, name, slug, "prompt body", creatorID, at.teamID, at.projectID)
	require.NoError(t, err)
	return id
}

func insertCommentMemory(t *testing.T, at accessTeam, creatorID, text string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		`INSERT INTO memories (id, user_id, text, team_id, project_id)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, creatorID, text, at.teamID, at.projectID)
	require.NoError(t, err)
	return id
}

func newComment(teamID, rtype, resourceID, userID, content string) *models.Comment {
	return &models.Comment{
		TeamID:       teamID,
		ResourceType: rtype,
		ResourceID:   resourceID,
		UserID:       userID,
		Content:      content,
	}
}

func TestIntegrationComment_CRUDAndList(t *testing.T) {
	resetIntegrationTables(t)
	at := seedAccessTeam(t)
	artifactID := insertAccessArtifact(t, at, at.ownerID, "commented-artifact")
	repo := NewCommentRepository(integrationDB)
	ctx := context.Background()

	// Create populates ID + timestamps.
	c := newComment(at.teamID, models.CommentResourceTypeArtifact, artifactID, at.ownerID, "first note")
	require.NoError(t, repo.Create(ctx, c))
	require.NotEmpty(t, c.ID)
	require.False(t, c.CreatedAt.IsZero())
	require.Equal(t, c.CreatedAt, c.UpdatedAt)

	// GetByID round-trips, and is team-scoped.
	got, err := repo.GetByID(ctx, at.teamID, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "first note", got.Content)

	_, err = repo.GetByID(ctx, uuid.New().String(), c.ID)
	assert.ErrorIs(t, err, repositories.ErrCommentNotFound)

	// A second comment, then list is newest-first with a correct total count.
	c2 := newComment(at.teamID, models.CommentResourceTypeArtifact, artifactID, at.memberID, "second note")
	require.NoError(t, repo.Create(ctx, c2))

	list, total, err := repo.ListByResource(ctx, at.teamID, models.CommentResourceTypeArtifact, artifactID, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, list, 2)
	assert.Equal(t, c2.ID, list[0].ID, "newest first")
	assert.Equal(t, c.ID, list[1].ID)

	// Pagination: page 2, limit 1 returns the older row and still the full count.
	page2, total2, err := repo.ListByResource(ctx, at.teamID, models.CommentResourceTypeArtifact, artifactID, 2, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, total2)
	require.Len(t, page2, 1)
	assert.Equal(t, c.ID, page2[0].ID)

	// UpdateContent bumps updated_at past created_at ("edited").
	updated, err := repo.UpdateContent(ctx, at.teamID, c.ID, "first note (edited)")
	require.NoError(t, err)
	assert.Equal(t, "first note (edited)", updated.Content)
	assert.True(t, updated.UpdatedAt.After(updated.CreatedAt))

	// Delete is team-scoped and idempotency-safe (second delete -> not found).
	require.NoError(t, repo.Delete(ctx, at.teamID, c.ID))
	_, err = repo.GetByID(ctx, at.teamID, c.ID)
	assert.ErrorIs(t, err, repositories.ErrCommentNotFound)
	assert.ErrorIs(t, repo.Delete(ctx, at.teamID, c.ID), repositories.ErrCommentNotFound)
}

func TestIntegrationComment_ResourceExists(t *testing.T) {
	resetIntegrationTables(t)
	at := seedAccessTeam(t)
	artifactID := insertAccessArtifact(t, at, at.ownerID, "exists-artifact")
	repo := NewCommentRepository(integrationDB)
	ctx := context.Background()

	ok, err := repo.ResourceExists(ctx, at.teamID, models.CommentResourceTypeArtifact, artifactID)
	require.NoError(t, err)
	assert.True(t, ok)

	// Wrong team -> not found (tenancy), unknown id -> not found.
	ok, err = repo.ResourceExists(ctx, uuid.New().String(), models.CommentResourceTypeArtifact, artifactID)
	require.NoError(t, err)
	assert.False(t, ok)

	ok, err = repo.ResourceExists(ctx, at.teamID, models.CommentResourceTypeArtifact, uuid.New().String())
	require.NoError(t, err)
	assert.False(t, ok)

	_, err = repo.ResourceExists(ctx, at.teamID, "project", artifactID)
	assert.Error(t, err, "unknown resource type is rejected")
}

func TestIntegrationComment_Cascades(t *testing.T) {
	resetIntegrationTables(t)
	at := seedAccessTeam(t)
	artifactID := insertAccessArtifact(t, at, at.ownerID, "cascade-artifact")
	repo := NewCommentRepository(integrationDB)
	ctx := context.Background()

	// Two comments by different authors on the same resource.
	require.NoError(t, repo.Create(ctx, newComment(at.teamID, models.CommentResourceTypeArtifact, artifactID, at.ownerID, "a")))
	require.NoError(t, repo.Create(ctx, newComment(at.teamID, models.CommentResourceTypeArtifact, artifactID, at.memberID, "b")))

	// DeleteByUser removes only the member's comment.
	n, err := repo.DeleteByUser(ctx, at.teamID, at.memberID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	_, total, err := repo.ListByResource(ctx, at.teamID, models.CommentResourceTypeArtifact, artifactID, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, total)

	// DeleteByResource removes the rest.
	n, err = repo.DeleteByResource(ctx, at.teamID, models.CommentResourceTypeArtifact, artifactID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	_, total, err = repo.ListByResource(ctx, at.teamID, models.CommentResourceTypeArtifact, artifactID, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
}

func TestIntegrationComment_UserDeleteCascadesViaFK(t *testing.T) {
	resetIntegrationTables(t)
	at := seedAccessTeam(t)
	artifactID := insertAccessArtifact(t, at, at.ownerID, "fk-artifact")
	repo := NewCommentRepository(integrationDB)
	ctx := context.Background()

	stranger := insertTestUser(t)
	_, err := integrationDB.ExecContext(ctx,
		"INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)",
		at.teamID, stranger, models.TeamMemberRoleMember)
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, newComment(at.teamID, models.CommentResourceTypeArtifact, artifactID, stranger, "x")))

	// Deleting the user row cascades to their comments (ON DELETE CASCADE).
	_, err = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id = $1", stranger)
	require.NoError(t, err)

	_, total, err := repo.ListByResource(ctx, at.teamID, models.CommentResourceTypeArtifact, artifactID, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
}

func TestIntegrationComment_ListRecentByTeam(t *testing.T) {
	resetIntegrationTables(t)
	at := seedAccessTeam(t)
	artifactID := insertAccessArtifact(t, at, at.ownerID, "recent-artifact")
	blueprintID := insertAccessBlueprint(t, at, at.ownerID, "recent-blueprint")
	promptID := insertCommentPrompt(t, at, at.ownerID, "recent-prompt", "Recent Prompt")
	memoryID := insertCommentMemory(t, at, at.ownerID, "a memory body used as its title label")
	repo := NewCommentRepository(integrationDB)
	ctx := context.Background()

	// One comment per resource type.
	artComment := newComment(at.teamID, models.CommentResourceTypeArtifact, artifactID, at.ownerID, "on artifact")
	require.NoError(t, repo.Create(ctx, artComment))
	require.NoError(t, repo.Create(ctx, newComment(at.teamID, models.CommentResourceTypeBlueprint, blueprintID, at.ownerID, "on blueprint")))
	require.NoError(t, repo.Create(ctx, newComment(at.teamID, models.CommentResourceTypePrompt, promptID, at.ownerID, "on prompt")))
	require.NoError(t, repo.Create(ctx, newComment(at.teamID, models.CommentResourceTypeMemory, memoryID, at.ownerID, "on memory")))

	// A comment on a resource that then vanishes must be OMITTED (dangling).
	danglingID := insertAccessArtifact(t, at, at.ownerID, "dangling-artifact")
	require.NoError(t, repo.Create(ctx, newComment(at.teamID, models.CommentResourceTypeArtifact, danglingID, at.ownerID, "on dangling")))
	_, err := integrationDB.ExecContext(ctx, "DELETE FROM artifacts WHERE id = $1", danglingID)
	require.NoError(t, err)

	recent, err := repo.ListRecentByTeam(ctx, at.teamID, 20)
	require.NoError(t, err)
	require.Len(t, recent, 4, "the dangling comment is omitted")

	byType := map[string]models.CommentActivity{}
	for _, a := range recent {
		byType[a.ResourceType] = a
	}

	// Title + link fields resolved per type.
	assert.Equal(t, "Artifact Title", byType[models.CommentResourceTypeArtifact].ResourceTitle)
	require.NotNil(t, byType[models.CommentResourceTypeArtifact].Slug)
	assert.Equal(t, "recent-artifact", *byType[models.CommentResourceTypeArtifact].Slug)
	require.NotNil(t, byType[models.CommentResourceTypeArtifact].ProjectID)
	assert.Equal(t, at.projectID, *byType[models.CommentResourceTypeArtifact].ProjectID)

	assert.Equal(t, "Blueprint Title", byType[models.CommentResourceTypeBlueprint].ResourceTitle)
	require.NotNil(t, byType[models.CommentResourceTypeBlueprint].Slug)
	assert.Equal(t, "recent-blueprint", *byType[models.CommentResourceTypeBlueprint].Slug)

	// Prompt resolves its title from `name`, and carries a slug.
	assert.Equal(t, "Recent Prompt", byType[models.CommentResourceTypePrompt].ResourceTitle)
	require.NotNil(t, byType[models.CommentResourceTypePrompt].Slug)
	assert.Equal(t, "recent-prompt", *byType[models.CommentResourceTypePrompt].Slug)

	// Memory has no slug; its title is the text prefix; project id is present.
	assert.Equal(t, "a memory body used as its title label", byType[models.CommentResourceTypeMemory].ResourceTitle)
	assert.Nil(t, byType[models.CommentResourceTypeMemory].Slug)
	require.NotNil(t, byType[models.CommentResourceTypeMemory].ProjectID)

	// Editing the (oldest) artifact comment resurfaces it to the top by latest activity.
	_, err = repo.UpdateContent(ctx, at.teamID, artComment.ID, "on artifact (edited)")
	require.NoError(t, err)
	recent, err = repo.ListRecentByTeam(ctx, at.teamID, 20)
	require.NoError(t, err)
	require.NotEmpty(t, recent)
	assert.Equal(t, artComment.ID, recent[0].ID, "an edited comment sorts first by GREATEST(created_at, updated_at)")

	// The limit is honored.
	limited, err := repo.ListRecentByTeam(ctx, at.teamID, 2)
	require.NoError(t, err)
	assert.Len(t, limited, 2)
}
