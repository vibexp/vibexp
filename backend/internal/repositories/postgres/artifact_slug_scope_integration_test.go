//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

// TestIntegrationArtifact_SlugConflict_NamesScope proves the #256 fix against
// real Postgres: a slug that is free in the target project but already used
// elsewhere in the same team collides on the team-wide artifacts_slug_team_id_key
// and must report "in this team" — the exact case the old "for this project"
// message misdirected. A same-project collision names the project.
func TestIntegrationArtifact_SlugConflict_NamesScope(t *testing.T) {
	ctx := context.Background()
	resetIntegrationTables(t)

	ownerID := insertTestUser(t)
	teamID := uuid.New().String()
	_, err := integrationDB.ExecContext(ctx,
		"INSERT INTO teams (id, owner_id, name, slug) VALUES ($1, $2, $3, $4)",
		teamID, ownerID, "Slug Scope Team", "slug-scope-"+teamID[:8])
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM teams WHERE id = $1", teamID)
	})

	projectA := uuid.New().String()
	projectB := uuid.New().String()
	for _, p := range []string{projectA, projectB} {
		_, err = integrationDB.ExecContext(ctx,
			"INSERT INTO projects (id, user_id, team_id, name, slug) VALUES ($1, $2, $3, $4, $5)",
			p, ownerID, teamID, "Project", "proj-"+p[:8])
		require.NoError(t, err)
	}

	repo := NewArtifactRepository(integrationDB)
	newArtifact := func(projectID, slug string) *models.Artifact {
		now := time.Now()
		return &models.Artifact{
			ProjectID: projectID, Slug: slug, UserID: ownerID, TeamID: teamID,
			Title: "Title", Content: "Content", Status: "active", Type: "general",
			CreatedAt: now, UpdatedAt: now,
		}
	}

	// Baseline: the slug is created in project A.
	require.NoError(t, repo.Create(ctx, newArtifact(projectA, "dup")))

	// Cross-project, same team: free in project B per (project_id, slug), but the
	// team-wide key fires — the message must point at the team (#256).
	err = repo.Create(ctx, newArtifact(projectB, "dup"))
	assert.EqualError(t, err, "artifact with slug 'dup' already exists in this team")

	// Same project, same slug: a genuine per-project collision. Both keys are
	// violated, so Postgres may report either; assert only that the message names
	// a concrete scope rather than the old blanket wording.
	err = repo.Create(ctx, newArtifact(projectA, "dup"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists in this")
}
