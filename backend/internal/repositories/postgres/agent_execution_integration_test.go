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

// insertTestAgent creates a minimal active agent owned by userID/teamID and
// returns its id. agent_executions.agent_id references it.
func insertTestAgent(t *testing.T, userID, teamID string) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO agents (id, user_id, team_id, name, description) VALUES ($1, $2, $3, $4, $5)",
		id, userID, teamID, "Integration Agent", "for integration tests")
	require.NoError(t, err)
	return id
}

// TestAgentExecutionRepository_Create_SyncsVersion asserts Create writes the
// DB-assigned version back onto the struct (regression for issue #197: a stale
// version 0 made the optimistic-locking Update a silent no-op).
func TestAgentExecutionRepository_Create_SyncsVersion(t *testing.T) {
	resetIntegrationTables(t)
	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	agentID := insertTestAgent(t, userID, teamID)

	repo := NewAgentExecutionRepository(integrationDB)

	execution := &models.AgentExecution{
		AgentID: agentID,
		UserID:  userID,
		Status:  "pending",
		Input:   map[string]interface{}{"text": "hi"},
	}
	require.NoError(t, repo.Create(context.Background(), execution))

	assert.Equal(t, int64(1), execution.Version, "Create must sync the DB-assigned version onto the struct")
}

// TestAgentExecutionRepository_StreamingErrorPersists is the end-to-end DB
// regression for issue #197. It reproduces the streaming-error sequence against
// real Postgres: the version-guarded Update (as handleStreamingError issues it)
// must persist "error", and the subsequent finalize UpdateStatus("success") must
// NOT overwrite it (it is guarded WHERE status = 'pending'). Before the fix,
// Create left version 0, the Update matched no rows, and finalize set "success".
func TestAgentExecutionRepository_StreamingErrorPersists(t *testing.T) {
	resetIntegrationTables(t)
	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	agentID := insertTestAgent(t, userID, teamID)

	repo := NewAgentExecutionRepository(integrationDB)
	ctx := context.Background()

	execution := &models.AgentExecution{
		AgentID: agentID,
		UserID:  userID,
		Status:  "pending",
		Input:   map[string]interface{}{"text": "hi"},
	}
	require.NoError(t, repo.Create(ctx, execution))
	require.Equal(t, int64(1), execution.Version)

	// 1) handleStreamingError: mark the execution "error" via the version-guarded Update.
	endedAt := time.Now()
	durationMs := 5
	errMsg := "connection reset mid-stream"
	execution.Status = "error"
	execution.Error = &errMsg
	execution.EndedAt = &endedAt
	execution.Duration = &durationMs
	require.NoError(t, repo.Update(ctx, execution), "version-guarded Update must persist the error status")

	// 2) finalize: UpdateStatus("success") — must no-op because status is no longer 'pending'.
	require.NoError(t, repo.UpdateStatus(ctx, execution.ID, "success"))

	// The error status must survive.
	got, err := repo.GetByID(ctx, userID, execution.ID)
	require.NoError(t, err)
	assert.Equal(t, "error", got.Status, "a mid-stream error must not be overwritten back to success")
	require.NotNil(t, got.Error)
	assert.Equal(t, errMsg, *got.Error)
}

// TestAgentExecutionRepository_CleanStreamFinalizesSuccess guards against a
// regression in the other direction: a clean stream that ends while still
// 'pending' (finalize's default) must reach "success".
func TestAgentExecutionRepository_CleanStreamFinalizesSuccess(t *testing.T) {
	resetIntegrationTables(t)
	userID := insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	agentID := insertTestAgent(t, userID, teamID)

	repo := NewAgentExecutionRepository(integrationDB)
	ctx := context.Background()

	execution := &models.AgentExecution{
		AgentID: agentID,
		UserID:  userID,
		Status:  "pending",
		Input:   map[string]interface{}{"text": "hi"},
	}
	require.NoError(t, repo.Create(ctx, execution))

	require.NoError(t, repo.UpdateStatus(ctx, execution.ID, "success"))

	got, err := repo.GetByID(ctx, userID, execution.ID)
	require.NoError(t, err)
	assert.Equal(t, "success", got.Status)
}
