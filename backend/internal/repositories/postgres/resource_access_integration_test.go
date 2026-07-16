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

// Access-path integration tests for artifacts and blueprints against real
// Postgres (#258).
//
// The bug these guard: the detail-read and delete paths resolved a team-shared
// artifact/blueprint through an owner-scoped getter (WHERE project_id = $1 AND
// slug = $2 AND user_id = $3), so a team member who did NOT create the resource
// got not-found — even though the list shows it to every member. The correct
// getter (GetByProjectIDAndSlug) scopes by team_id + membership (an EXISTS over
// team_members / teams.owner_id).
//
// This is exactly the seam sqlmock cannot prove: it matches the query by regex
// and returns canned rows, so it never evaluates the membership EXISTS. Every
// handler/service test mocks at that seam too. Only a real DB with creator !=
// caller inside one team distinguishes "owner_id = caller" from team membership.
// Mirrors seedTransferTeam's two-member seeding pattern.

// accessTeam is a team with three distinct members plus a project, all created
// by seedAccessTeam. ownerID is the creator of the seeded resource; memberID
// and adminID are members who did NOT create it (the callers the bug broke).
type accessTeam struct {
	teamID    string
	ownerID   string
	memberID  string
	adminID   string
	projectID string
}

// seedAccessTeam builds a team owned by ownerID with a plain member and an
// admin member, plus a project, and returns their ids. The resource row is
// inserted separately so each suite controls its own slug.
func seedAccessTeam(t *testing.T) accessTeam {
	t.Helper()
	ctx := context.Background()

	ownerID := insertTestUser(t)
	memberID := insertTestUser(t)
	adminID := insertTestUser(t)
	teamID := uuid.New().String()

	_, err := integrationDB.ExecContext(ctx,
		"INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, $3, $4)",
		teamID, ownerID, "Access Team", "access-team-"+teamID[:8])
	require.NoError(t, err)

	for _, m := range []struct {
		userID string
		role   models.TeamMemberRole
	}{
		{ownerID, models.TeamMemberRoleOwner},
		{memberID, models.TeamMemberRoleMember},
		{adminID, models.TeamMemberRoleAdmin},
	} {
		_, err = integrationDB.ExecContext(ctx,
			"INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)",
			teamID, m.userID, m.role)
		require.NoError(t, err)
	}

	projectID := uuid.New().String()
	_, err = integrationDB.ExecContext(ctx,
		"INSERT INTO projects (id, user_id, team_id, name, slug) VALUES ($1, $2, $3, $4, $5)",
		projectID, ownerID, teamID, "Access Project", "access-project-"+projectID[:8])
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", teamID)
	})

	return accessTeam{
		teamID:    teamID,
		ownerID:   ownerID,
		memberID:  memberID,
		adminID:   adminID,
		projectID: projectID,
	}
}

func insertAccessArtifact(t *testing.T, at accessTeam, creatorID, slug string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		`INSERT INTO artifacts (id, slug, user_id, team_id, project_id, content, title)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, slug, creatorID, at.teamID, at.projectID, "artifact-content", "Artifact Title")
	require.NoError(t, err)
	return id
}

func insertAccessBlueprint(t *testing.T, at accessTeam, creatorID, slug string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		`INSERT INTO blueprints (id, slug, user_id, team_id, project_id, content, title)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, slug, creatorID, at.teamID, at.projectID, "blueprint-content", "Blueprint Title")
	require.NoError(t, err)
	return id
}

func artifactRowExists(t *testing.T, id string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM artifacts WHERE id = $1)", id).Scan(&exists))
	return exists
}

func blueprintRowExists(t *testing.T, id string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, integrationDB.QueryRowContext(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM blueprints WHERE id = $1)", id).Scan(&exists))
	return exists
}

// TestIntegrationArtifact_GetByProjectIDAndSlug_TeamMembership is the read-path
// regression: a non-creator member (and an admin) must resolve an artifact they
// did not create, while a caller outside the team must not — proving the fix
// scopes by team membership, not merely dropping the user_id filter.
func TestIntegrationArtifact_GetByProjectIDAndSlug_TeamMembership(t *testing.T) {
	resetIntegrationTables(t)
	at := seedAccessTeam(t)
	const slug = "shared-artifact"
	artifactID := insertAccessArtifact(t, at, at.ownerID, slug)
	repo := NewArtifactRepository(integrationDB)
	ctx := context.Background()

	t.Run("creator resolves it", func(t *testing.T) {
		got, err := repo.GetByProjectIDAndSlug(ctx, at.ownerID, at.teamID, at.projectID, slug)
		require.NoError(t, err)
		assert.Equal(t, artifactID, got.ID)
	})

	t.Run("non-creator member resolves it (the bug)", func(t *testing.T) {
		got, err := repo.GetByProjectIDAndSlug(ctx, at.memberID, at.teamID, at.projectID, slug)
		require.NoError(t, err)
		assert.Equal(t, artifactID, got.ID)
	})

	t.Run("admin member resolves it", func(t *testing.T) {
		got, err := repo.GetByProjectIDAndSlug(ctx, at.adminID, at.teamID, at.projectID, slug)
		require.NoError(t, err)
		assert.Equal(t, artifactID, got.ID)
	})

	t.Run("non-member gets not-found (tenancy preserved)", func(t *testing.T) {
		stranger := insertTestUser(t)
		_, err := repo.GetByProjectIDAndSlug(ctx, stranger, at.teamID, at.projectID, slug)
		assert.ErrorIs(t, err, repositories.ErrArtifactNotFound)
	})

	t.Run("member cannot reach it via a foreign team_id", func(t *testing.T) {
		// A team the caller is not in: team_id must match the artifact's row AND
		// the caller must belong to that team, so a mismatched team_id is not-found
		// even for a real member of the resource's team.
		_, err := repo.GetByProjectIDAndSlug(ctx, at.memberID, uuid.New().String(), at.projectID, slug)
		assert.ErrorIs(t, err, repositories.ErrArtifactNotFound)
	})
}

// TestIntegrationArtifact_Delete_TeamMembership proves the delete resolves by
// team membership: an admin (not the creator) can delete another member's
// artifact — the delete.any moderation path the owner-scoped getter used to
// make unreachable — while a non-member cannot and the row survives. (own-vs-any
// role enforcement lives in the service layer and is unit-tested there.)
func TestIntegrationArtifact_Delete_TeamMembership(t *testing.T) {
	resetIntegrationTables(t)
	at := seedAccessTeam(t)
	repo := NewArtifactRepository(integrationDB)
	ctx := context.Background()

	t.Run("non-member cannot delete and the row survives", func(t *testing.T) {
		id := insertAccessArtifact(t, at, at.ownerID, "keep-me")
		stranger := insertTestUser(t)
		err := repo.Delete(ctx, stranger, at.teamID, id)
		assert.ErrorIs(t, err, repositories.ErrArtifactNotFound)
		assert.True(t, artifactRowExists(t, id), "row must survive a non-member delete")
	})

	t.Run("admin deletes another member's artifact", func(t *testing.T) {
		id := insertAccessArtifact(t, at, at.ownerID, "delete-me")
		err := repo.Delete(ctx, at.adminID, at.teamID, id)
		require.NoError(t, err)
		assert.False(t, artifactRowExists(t, id), "row must be gone after an admin delete")
	})
}

// TestIntegrationBlueprint_GetByProjectIDAndSlug_TeamMembership mirrors the
// artifact read-path regression for blueprints.
func TestIntegrationBlueprint_GetByProjectIDAndSlug_TeamMembership(t *testing.T) {
	resetIntegrationTables(t)
	at := seedAccessTeam(t)
	const slug = "shared-blueprint"
	blueprintID := insertAccessBlueprint(t, at, at.ownerID, slug)
	repo := NewBlueprintRepository(integrationDB)
	ctx := context.Background()

	t.Run("creator resolves it", func(t *testing.T) {
		got, err := repo.GetByProjectIDAndSlug(ctx, at.ownerID, at.teamID, at.projectID, slug)
		require.NoError(t, err)
		assert.Equal(t, blueprintID, got.ID)
	})

	t.Run("non-creator member resolves it (the bug)", func(t *testing.T) {
		got, err := repo.GetByProjectIDAndSlug(ctx, at.memberID, at.teamID, at.projectID, slug)
		require.NoError(t, err)
		assert.Equal(t, blueprintID, got.ID)
	})

	t.Run("admin member resolves it", func(t *testing.T) {
		got, err := repo.GetByProjectIDAndSlug(ctx, at.adminID, at.teamID, at.projectID, slug)
		require.NoError(t, err)
		assert.Equal(t, blueprintID, got.ID)
	})

	t.Run("non-member gets not-found (tenancy preserved)", func(t *testing.T) {
		stranger := insertTestUser(t)
		_, err := repo.GetByProjectIDAndSlug(ctx, stranger, at.teamID, at.projectID, slug)
		assert.ErrorIs(t, err, repositories.ErrBlueprintNotFound)
	})

	t.Run("member cannot reach it via a foreign team_id", func(t *testing.T) {
		_, err := repo.GetByProjectIDAndSlug(ctx, at.memberID, uuid.New().String(), at.projectID, slug)
		assert.ErrorIs(t, err, repositories.ErrBlueprintNotFound)
	})
}

// TestIntegrationBlueprint_Delete_TeamMembership mirrors the artifact delete
// regression for blueprints.
func TestIntegrationBlueprint_Delete_TeamMembership(t *testing.T) {
	resetIntegrationTables(t)
	at := seedAccessTeam(t)
	repo := NewBlueprintRepository(integrationDB)
	ctx := context.Background()

	t.Run("non-member cannot delete and the row survives", func(t *testing.T) {
		id := insertAccessBlueprint(t, at, at.ownerID, "keep-me")
		stranger := insertTestUser(t)
		err := repo.Delete(ctx, stranger, at.teamID, id)
		assert.ErrorIs(t, err, repositories.ErrBlueprintNotFound)
		assert.True(t, blueprintRowExists(t, id), "row must survive a non-member delete")
	})

	t.Run("admin deletes another member's blueprint", func(t *testing.T) {
		id := insertAccessBlueprint(t, at, at.ownerID, "delete-me")
		err := repo.Delete(ctx, at.adminID, at.teamID, id)
		require.NoError(t, err)
		assert.False(t, blueprintRowExists(t, id), "row must be gone after an admin delete")
	})
}
