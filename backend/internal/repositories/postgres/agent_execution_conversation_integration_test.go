//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// insertTestAgentNamed is insertTestAgent with an explicit name, needed because
// agents carry a UNIQUE (name, team_id) constraint.
func insertTestAgentNamed(t *testing.T, userID, teamID, name string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO agents (id, user_id, team_id, name, description) VALUES ($1, $2, $3, $4, $5)",
		id, userID, teamID, name, "for integration tests")
	require.NoError(t, err)
	return id
}

// seedConversationExecution creates an execution in a conversation via the real
// repository, then pins started_at (Create always stamps time.Now()) so ordering
// assertions are deterministic.
func seedConversationExecution(
	t *testing.T, userID, agentID, conversationID, text string, startedAt time.Time,
) string {
	t.Helper()
	execution := &models.AgentExecution{
		AgentID:        agentID,
		UserID:         userID,
		Input:          map[string]interface{}{"text": text},
		ConversationID: &conversationID,
	}
	require.NoError(t, NewAgentExecutionRepository(integrationDB).Create(context.Background(), execution))
	_, err := integrationDB.ExecContext(context.Background(),
		"UPDATE agent_executions SET started_at = $1 WHERE id = $2", startedAt, execution.ID)
	require.NoError(t, err)
	return execution.ID
}

func conversationInputTexts(executions []models.AgentExecution) []string {
	texts := make([]string, 0, len(executions))
	for _, e := range executions {
		texts = append(texts, e.Input["text"].(string))
	}
	return texts
}

func TestAgentExecutionRepository_Integration_ConversationsFlow(t *testing.T) {
	resetAgentExecutionData(t)
	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	agentA := insertTestAgentNamed(t, userID, teamID, "Conversation Agent A")
	agentB := insertTestAgentNamed(t, userID, teamID, "Conversation Agent B")

	repo := NewAgentExecutionRepository(integrationDB)
	ctx := context.Background()

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	t0, t1, t2 := base, base.Add(time.Minute), base.Add(2*time.Minute)
	t3 := base.Add(3 * time.Minute)

	// conv-1: three messages on agent A; conv-2: one later message on agent B.
	seedConversationExecution(t, userID, agentA, "conv-1", "one", t0)
	seedConversationExecution(t, userID, agentA, "conv-1", "two", t1)
	seedConversationExecution(t, userID, agentA, "conv-1", "three", t2)
	seedConversationExecution(t, userID, agentB, "conv-2", "solo", t3)

	t.Run("GetByConversationID returns chronological order", func(t *testing.T) {
		executions, hasMore, total, err := repo.GetByConversationID(ctx, userID, "conv-1", 10, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"one", "two", "three"}, conversationInputTexts(executions))
		assert.False(t, hasMore)
		assert.Equal(t, 3, total)
	})

	t.Run("GetByConversationID limit keeps the newest page and reports hasMore", func(t *testing.T) {
		executions, hasMore, total, err := repo.GetByConversationID(ctx, userID, "conv-1", 2, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"two", "three"}, conversationInputTexts(executions),
			"the newest executions win the page, returned oldest-first")
		assert.True(t, hasMore)
		assert.Equal(t, 3, total, "totalCount must be the full conversation size")
	})

	t.Run("GetByConversationID before cursor pages backwards", func(t *testing.T) {
		executions, hasMore, total, err := repo.GetByConversationID(ctx, userID, "conv-1", 10, &t2)
		require.NoError(t, err)
		assert.Equal(t, []string{"one", "two"}, conversationInputTexts(executions))
		assert.False(t, hasMore)
		assert.Equal(t, 3, total)
	})

	t.Run("GetFirstExecutionInConversation returns the earliest message", func(t *testing.T) {
		first, err := repo.GetFirstExecutionInConversation(ctx, userID, "conv-1")
		require.NoError(t, err)
		assert.Equal(t, "one", first.Input["text"])
		require.NotNil(t, first.ConversationID)
		assert.Equal(t, "conv-1", *first.ConversationID)

		_, err = repo.GetFirstExecutionInConversation(ctx, userID, "conv-missing")
		assert.ErrorIs(t, err, repositories.ErrConversationNotFound)
	})

	t.Run("ListConversations groups and orders by last activity DESC", func(t *testing.T) {
		conversations, total, err := repo.ListConversations(ctx, userID, "", 1, 10)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		require.Len(t, conversations, 2)

		// conv-2 (t3) is more recent than conv-1 (t2).
		assert.Equal(t, "conv-2", conversations[0].ConversationID)
		assert.Equal(t, agentB, conversations[0].AgentID)
		assert.Equal(t, 1, conversations[0].MessageCount)

		conv1 := conversations[1]
		assert.Equal(t, "conv-1", conv1.ConversationID)
		assert.Equal(t, agentA, conv1.AgentID)
		assert.Equal(t, 3, conv1.MessageCount)
		assert.Equal(t, "one", conv1.FirstMessage)
		assert.Equal(t, "three", conv1.LastMessage)
		assert.WithinDuration(t, t0, conv1.StartedAt, time.Millisecond)
		assert.WithinDuration(t, t2, conv1.LastActivityAt, time.Millisecond)
		assert.Equal(t, "running", conv1.LastStatus, "Create defaults status to running")
	})

	t.Run("ListConversations filters by agent", func(t *testing.T) {
		conversations, total, err := repo.ListConversations(ctx, userID, agentA, 1, 10)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, conversations, 1)
		assert.Equal(t, "conv-1", conversations[0].ConversationID)
	})

	t.Run("ListConversations excludes executions without a conversation", func(t *testing.T) {
		loose := &models.AgentExecution{
			AgentID: agentA, UserID: userID, Input: map[string]interface{}{"text": "no conv"},
		}
		require.NoError(t, repo.Create(ctx, loose))

		conversations, total, err := repo.ListConversations(ctx, userID, "", 1, 10)
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, conversations, 2)
	})
}

func TestAgentRepository_Integration_UpdateExecutionStats(t *testing.T) {
	resetAgentExecutionData(t)
	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	agentID := insertTestAgent(t, userID, teamID)

	repo := NewAgentRepository(integrationDB)
	ctx := context.Background()

	readStats := func() (totalRuns int, successRate float64, lastRun sql.NullTime) {
		t.Helper()
		err := integrationDB.QueryRowContext(ctx,
			"SELECT total_runs, success_rate, last_run FROM agents WHERE id = $1", agentID,
		).Scan(&totalRuns, &successRate, &lastRun)
		require.NoError(t, err)
		return totalRuns, successRate, lastRun
	}

	// Seeded row starts at the column defaults.
	totalRuns, successRate, lastRun := readStats()
	require.Zero(t, totalRuns)
	require.Zero(t, successRate)
	require.False(t, lastRun.Valid)

	// First run succeeds: 1 run, 100% success.
	require.NoError(t, repo.UpdateExecutionStats(ctx, agentID, true, 1200))
	totalRuns, successRate, lastRun = readStats()
	assert.Equal(t, 1, totalRuns)
	assert.InDelta(t, 100.0, successRate, 0.01)
	assert.True(t, lastRun.Valid, "last_run must be stamped")

	// Second run fails: 2 runs, the rate is re-derived from stored values -> 50%.
	require.NoError(t, repo.UpdateExecutionStats(ctx, agentID, false, 800))
	totalRuns, successRate, _ = readStats()
	assert.Equal(t, 2, totalRuns)
	assert.InDelta(t, 50.0, successRate, 0.01)

	// Third run succeeds: 3 runs, 2/3 success.
	require.NoError(t, repo.UpdateExecutionStats(ctx, agentID, true, 900))
	totalRuns, successRate, _ = readStats()
	assert.Equal(t, 3, totalRuns)
	assert.InDelta(t, 66.67, successRate, 0.01)

	// Unknown agent maps to the sentinel.
	err := repo.UpdateExecutionStats(ctx, uuid.New().String(), true, 100)
	assert.ErrorIs(t, err, repositories.ErrAgentNotFound)
}
