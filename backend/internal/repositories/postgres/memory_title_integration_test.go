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

// The optional memory title against a real Postgres (issue #911, migration 017).
//
// sqlmock cannot prove any of this: it matches the query by regex and hands back
// canned rows, so it never evaluates the column at all. What needs a real DB is
// (a) that `title` actually round-trips through INSERT/SELECT/UPDATE on every
// read path, and (b) that a row written BEFORE the column existed -- which
// migration 017 deliberately does not backfill -- reads back as nil rather than
// "".

func newMemoryTitleRepo() repositories.MemoryRepository {
	return NewMemoryRepository(integrationDB)
}

func TestMemoryTitle_RoundTripsThroughEveryReadPath(t *testing.T) {
	ctx := context.Background()
	at := seedAccessTeam(t)
	repo := newMemoryTitleRepo()

	title := "Deploy checklist"
	memory := &models.Memory{
		UserID: at.ownerID, TeamID: at.teamID, ProjectID: at.projectID,
		Title: &title, Text: "remember the deploy order",
		Status: models.MemoryStatusActive,
	}
	require.NoError(t, repo.Create(ctx, memory))
	require.NotEmpty(t, memory.ID)

	got, err := repo.GetByID(ctx, at.ownerID, at.teamID, memory.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Title)
	assert.Equal(t, title, *got.Title)

	crossTeam, err := repo.GetByIDCrossTeam(ctx, at.ownerID, memory.ID)
	require.NoError(t, err)
	require.NotNil(t, crossTeam.Title)
	assert.Equal(t, title, *crossTeam.Title)

	listed, _, err := repo.List(ctx, at.ownerID, repositories.MemoryFilters{
		TeamID: at.teamID, ProjectID: &at.projectID, Page: 1, Limit: 10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, listed)
	var found bool
	for _, m := range listed {
		if m.ID == memory.ID {
			found = true
			require.NotNil(t, m.Title, "the list projection must carry the title")
			assert.Equal(t, title, *m.Title)
		}
	}
	assert.True(t, found)
}

func TestMemoryTitle_UpdateSetsAndClearsTheColumn(t *testing.T) {
	ctx := context.Background()
	at := seedAccessTeam(t)
	repo := newMemoryTitleRepo()

	memory := &models.Memory{
		UserID: at.ownerID, TeamID: at.teamID, ProjectID: at.projectID,
		Text: "remember this", Status: models.MemoryStatusActive,
	}
	require.NoError(t, repo.Create(ctx, memory))

	stored, err := repo.GetByID(ctx, at.ownerID, at.teamID, memory.ID)
	require.NoError(t, err)
	assert.Nil(t, stored.Title, "a memory created without a title stores NULL, never ''")

	title := "Deploy checklist"
	stored.Title = &title
	require.NoError(t, repo.Update(ctx, stored))

	reread, err := repo.GetByID(ctx, at.ownerID, at.teamID, memory.ID)
	require.NoError(t, err)
	require.NotNil(t, reread.Title)
	assert.Equal(t, title, *reread.Title)

	reread.Title = nil
	require.NoError(t, repo.Update(ctx, reread))

	cleared, err := repo.GetByID(ctx, at.ownerID, at.teamID, memory.ID)
	require.NoError(t, err)
	assert.Nil(t, cleared.Title, "clearing must write NULL back, not ''")
}

// Migration 017 adds the column with no default and no backfill, so a row that
// predates it is indistinguishable from one inserted without a title: both are
// NULL. Inserting through raw SQL that never mentions `title` is the closest
// reproduction of a pre-migration row.
func TestMemoryTitle_PreExistingRowReadsBackAsNull(t *testing.T) {
	ctx := context.Background()
	at := seedAccessTeam(t)

	id := uuid.New().String()
	_, err := integrationDB.ExecContext(ctx,
		`INSERT INTO memories (id, user_id, text, team_id, project_id) VALUES ($1, $2, $3, $4, $5)`,
		id, at.ownerID, "a memory from before titles existed", at.teamID, at.projectID)
	require.NoError(t, err)

	got, err := newMemoryTitleRepo().GetByID(ctx, at.ownerID, at.teamID, id)
	require.NoError(t, err)
	assert.Nil(t, got.Title)
}
