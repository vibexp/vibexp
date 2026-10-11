//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// A team's custom status against a real Postgres, on all four resource types
// (#1266, on top of migration 019_consolidated, #1264).
//
// The status columns used to be varchar(20) and, on memories, guarded by a
// CHECK constraint naming the three system values. Migration 019_consolidated
// widened them to varchar(50) and dropped the constraint; these tests are the end-to-end
// proof that a status of the full 50 characters -- the longest slug
// `team_statuses` can hold -- is written, read back and filtered on, which
// sqlmock cannot show because it never evaluates a column type.
//
// The repositories do no validation of their own: whether a status is one the
// team has is the service's question.

// longestCustomStatus is a slug-shaped status of exactly 50 characters.
var longestCustomStatus = "in-review-" + strings.Repeat("x", 40)

func TestCustomStatus_LongestSlugIsFiftyCharacters(t *testing.T) {
	require.Len(t, longestCustomStatus, 50)
}

func TestCustomStatus_Prompt_RoundTripsAndFilters(t *testing.T) {
	ctx := context.Background()
	at := seedAccessTeam(t)
	repo := NewPromptRepository(integrationDB)

	prompt := &models.Prompt{
		Name: "Custom status prompt", Slug: "custom-status-" + at.teamID[:8], Body: "body",
		UserID: at.ownerID, TeamID: at.teamID, ProjectID: at.projectID,
		Status: longestCustomStatus, Labels: []string{},
	}
	require.NoError(t, repo.Create(ctx, prompt))

	got, err := repo.GetByID(ctx, at.ownerID, at.teamID, prompt.ID)
	require.NoError(t, err)
	assert.Equal(t, longestCustomStatus, got.Status)

	page, total, err := repo.List(ctx, at.ownerID, repositories.PromptFilters{
		TeamID: at.teamID, Status: longestCustomStatus, Page: 1, Limit: 20,
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, prompt.ID, page[0].ID)

	_, total, err = repo.List(ctx, at.ownerID, repositories.PromptFilters{
		TeamID: at.teamID, Status: models.PromptStatusPublished, Page: 1, Limit: 20,
	})
	require.NoError(t, err)
	assert.Zero(t, total, "a system status filter must not match the custom-status row")
}

func TestCustomStatus_Artifact_RoundTripsAndFilters(t *testing.T) {
	ctx := context.Background()
	at := seedAccessTeam(t)
	repo := NewArtifactRepository(integrationDB)

	artifact := &models.Artifact{
		ProjectID: at.projectID, Slug: "custom-status-" + at.teamID[:8],
		UserID: at.ownerID, TeamID: at.teamID,
		Title: "Title", Content: "Content", Status: longestCustomStatus, Type: "general",
	}
	require.NoError(t, repo.Create(ctx, artifact))

	got, err := repo.GetByID(ctx, at.ownerID, at.teamID, artifact.ID)
	require.NoError(t, err)
	assert.Equal(t, longestCustomStatus, got.Status)

	status := longestCustomStatus
	page, total, err := repo.List(ctx, at.ownerID, repositories.ArtifactFilters{
		TeamID: at.teamID, Status: &status, Page: 1, Limit: 20,
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, artifact.ID, page[0].ID)

	archived := models.ArtifactStatusArchived
	_, total, err = repo.List(ctx, at.ownerID, repositories.ArtifactFilters{
		TeamID: at.teamID, Status: &archived, Page: 1, Limit: 20,
	})
	require.NoError(t, err)
	assert.Zero(t, total, "a system status filter must not match the custom-status row")
}

func TestCustomStatus_Blueprint_RoundTripsAndFilters(t *testing.T) {
	ctx := context.Background()
	at := seedAccessTeam(t)
	repo := NewBlueprintRepository(integrationDB)

	blueprint := &models.Blueprint{
		ProjectID: at.projectID, Slug: "custom-status-" + at.teamID[:8],
		UserID: at.ownerID, TeamID: at.teamID,
		Title: "Title", Content: "Content", Status: longestCustomStatus, Type: "general",
	}
	require.NoError(t, repo.Create(ctx, blueprint))

	got, err := repo.GetByID(ctx, at.ownerID, at.teamID, blueprint.ID)
	require.NoError(t, err)
	assert.Equal(t, longestCustomStatus, got.Status)

	status := longestCustomStatus
	page, total, err := repo.List(ctx, at.ownerID, repositories.BlueprintFilters{
		TeamID: at.teamID, Status: &status, Page: 1, Limit: 20,
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, blueprint.ID, page[0].ID)

	expired := models.BlueprintStatusExpired
	_, total, err = repo.List(ctx, at.ownerID, repositories.BlueprintFilters{
		TeamID: at.teamID, Status: &expired, Page: 1, Limit: 20,
	})
	require.NoError(t, err)
	assert.Zero(t, total, "a system status filter must not match the custom-status row")
}

func TestCustomStatus_Memory_RoundTripsAndFilters(t *testing.T) {
	ctx := context.Background()
	at := seedAccessTeam(t)
	repo := NewMemoryRepository(integrationDB)

	memory := &models.Memory{
		UserID: at.ownerID, TeamID: at.teamID, ProjectID: at.projectID,
		Text: "remember this", Status: longestCustomStatus,
	}
	require.NoError(t, repo.Create(ctx, memory))

	got, err := repo.GetByID(ctx, at.ownerID, at.teamID, memory.ID)
	require.NoError(t, err)
	assert.Equal(t, longestCustomStatus, got.Status)

	status := longestCustomStatus
	page, total, err := repo.List(ctx, at.ownerID, repositories.MemoryFilters{
		TeamID: at.teamID, Status: &status, Page: 1, Limit: 20,
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, memory.ID, page[0].ID)

	// A custom status can also be set on an existing row: the UPDATE path binds
	// the same column.
	got.Status = models.MemoryStatusActive
	require.NoError(t, repo.Update(ctx, got))
	got.Status = longestCustomStatus
	require.NoError(t, repo.Update(ctx, got))
	reread, err := repo.GetByID(ctx, at.ownerID, at.teamID, memory.ID)
	require.NoError(t, err)
	assert.Equal(t, longestCustomStatus, reread.Status)
}
