//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GetUserKnowledgeLoop against real Postgres (#1508): the first AI-tool read
// and write per user, where only the cli/mcp/api sources count and another
// user's events never leak in.

func insertLoopAccessEvent(t *testing.T, teamID, userID, source string, at time.Time) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), `
		INSERT INTO resource_access_events (team_id, user_id, resource_type, resource_id, source, created_at)
		VALUES ($1, $2, 'memory', $3, $4, $5)`,
		teamID, userID, uuid.New().String(), source, at)
	require.NoError(t, err)
}

func insertLoopWriteEvent(t *testing.T, teamID, userID, source string, at time.Time) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), `
		INSERT INTO resource_write_events (team_id, user_id, resource_type, resource_id, action, source, created_at)
		VALUES ($1, $2, 'memory', $3, 'create', $4, $5)`,
		teamID, userID, uuid.New().String(), source, at)
	require.NoError(t, err)
}

func TestResourceAccessRepository_GetUserKnowledgeLoop_Integration(t *testing.T) {
	ctx := context.Background()
	repo := NewResourceAccessRepository(integrationDB)
	team := seedAccessTeam(t)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	t.Run("a user with no events has not started", func(t *testing.T) {
		loop, err := repo.GetUserKnowledgeLoop(ctx, team.adminID)
		require.NoError(t, err)
		assert.Nil(t, loop.FirstReadAt)
		assert.Nil(t, loop.FirstWriteAt)
	})

	t.Run("web reads and writes do not count", func(t *testing.T) {
		insertLoopAccessEvent(t, team.teamID, team.memberID, "web", base)
		insertLoopWriteEvent(t, team.teamID, team.memberID, "web", base)

		loop, err := repo.GetUserKnowledgeLoop(ctx, team.memberID)
		require.NoError(t, err)
		assert.Nil(t, loop.FirstReadAt)
		assert.Nil(t, loop.FirstWriteAt)
	})

	t.Run("the earliest AI-tool read and write are reported", func(t *testing.T) {
		insertLoopAccessEvent(t, team.teamID, team.ownerID, "web", base)
		insertLoopAccessEvent(t, team.teamID, team.ownerID, "mcp", base.Add(3*time.Hour))
		insertLoopAccessEvent(t, team.teamID, team.ownerID, "cli", base.Add(2*time.Hour))
		insertLoopWriteEvent(t, team.teamID, team.ownerID, "web", base)
		insertLoopWriteEvent(t, team.teamID, team.ownerID, "api", base.Add(5*time.Hour))
		insertLoopWriteEvent(t, team.teamID, team.ownerID, "mcp", base.Add(4*time.Hour))

		loop, err := repo.GetUserKnowledgeLoop(ctx, team.ownerID)
		require.NoError(t, err)
		require.NotNil(t, loop.FirstReadAt)
		require.NotNil(t, loop.FirstWriteAt)
		assert.True(t, loop.FirstReadAt.Equal(base.Add(2*time.Hour)), "first read: %v", loop.FirstReadAt)
		assert.True(t, loop.FirstWriteAt.Equal(base.Add(4*time.Hour)), "first write: %v", loop.FirstWriteAt)
	})

	t.Run("another user's events never count", func(t *testing.T) {
		// The owner now has AI-tool reads and writes; the admin still has none.
		loop, err := repo.GetUserKnowledgeLoop(ctx, team.adminID)
		require.NoError(t, err)
		assert.Nil(t, loop.FirstReadAt)
		assert.Nil(t, loop.FirstWriteAt)
	})

	t.Run("a read alone leaves the write unset", func(t *testing.T) {
		insertLoopAccessEvent(t, team.teamID, team.adminID, "api", base)

		loop, err := repo.GetUserKnowledgeLoop(ctx, team.adminID)
		require.NoError(t, err)
		require.NotNil(t, loop.FirstReadAt)
		assert.True(t, loop.FirstReadAt.Equal(base))
		assert.Nil(t, loop.FirstWriteAt)
	})
}
