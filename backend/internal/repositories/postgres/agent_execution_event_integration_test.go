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
	"github.com/vibexp/vibexp/internal/repositories"
)

// resetAgentExecutionData clears everything the agent-execution suites write.
// resetIntegrationTables only truncates users/api_keys/user_preferences, so the
// execution and event tables are named explicitly (users CASCADE reaches
// teams -> agents -> agent_executions -> agent_execution_events anyway, but
// being explicit keeps the isolation independent of the FK graph).
func resetAgentExecutionData(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, agents, agent_executions, agent_execution_events CASCADE")
	require.NoError(t, err)
}

// seedExecutionFixture seeds user -> team -> agent -> execution (execution via
// the real repository Create) and returns the ids the event tests need.
func seedExecutionFixture(t *testing.T) (userID, agentID, executionID string) {
	t.Helper()
	userID = insertTestUser(t)
	teamID := insertTestTeam(t, userID)
	agentID = insertTestAgent(t, userID, teamID)

	execution := &models.AgentExecution{
		AgentID: agentID,
		UserID:  userID,
		Input:   map[string]interface{}{"text": "hi"},
	}
	require.NoError(t, NewAgentExecutionRepository(integrationDB).Create(context.Background(), execution))
	return userID, agentID, execution.ID
}

// mustCreateEvent persists one streaming event via the repository under test.
func mustCreateEvent(
	t *testing.T,
	repo repositories.AgentExecutionEventRepository,
	executionID string,
	seq int,
	data map[string]interface{},
) *models.AgentExecutionEvent {
	t.Helper()
	event := &models.AgentExecutionEvent{
		ExecutionID:    executionID,
		EventType:      "status-update",
		EventData:      data,
		SequenceNumber: seq,
		ReceivedAt:     time.Now().UTC().Truncate(time.Microsecond),
	}
	require.NoError(t, repo.Create(context.Background(), event))
	return event
}

func TestAgentExecutionEventRepository_Integration_CreateGetByIDRoundTrip(t *testing.T) {
	resetAgentExecutionData(t)
	_, _, executionID := seedExecutionFixture(t)
	repo := NewAgentExecutionEventRepository(integrationDB)
	ctx := context.Background()

	// Nested structures exercise JSONB fidelity end to end.
	eventData := map[string]interface{}{
		"message": map[string]interface{}{
			"role":  "agent",
			"parts": []interface{}{map[string]interface{}{"kind": "text", "text": "hello"}},
		},
		"count": float64(3),
		"final": false,
	}
	created := mustCreateEvent(t, repo, executionID, 1, eventData)

	require.NotEmpty(t, created.ID, "Create must assign an ID")
	_, parseErr := uuid.Parse(created.ID)
	require.NoError(t, parseErr, "assigned ID must be a UUID (the column is uuid)")

	got, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, executionID, got.ExecutionID)
	assert.Equal(t, "status-update", got.EventType)
	assert.Equal(t, 1, got.SequenceNumber)
	assert.Equal(t, eventData, got.EventData, "event_data must round-trip through JSONB unchanged")
	assert.WithinDuration(t, created.ReceivedAt, got.ReceivedAt, time.Millisecond)

	// Unknown id maps to the sentinel.
	_, err = repo.GetByID(ctx, uuid.New().String())
	assert.ErrorIs(t, err, repositories.ErrAgentExecutionEventNotFound)
}

// TestAgentExecutionEventRepository_Integration_DuplicateSequenceRejected is the
// only real proof of the (execution_id, sequence_number) uniqueness contract:
// the a2atest in-memory EventStore does NOT enforce it.
func TestAgentExecutionEventRepository_Integration_DuplicateSequenceRejected(t *testing.T) {
	resetAgentExecutionData(t)
	userID, agentID, executionID := seedExecutionFixture(t)
	repo := NewAgentExecutionEventRepository(integrationDB)
	ctx := context.Background()

	mustCreateEvent(t, repo, executionID, 1, map[string]interface{}{"n": float64(1)})

	dup := &models.AgentExecutionEvent{
		ExecutionID:    executionID,
		EventType:      "status-update",
		EventData:      map[string]interface{}{"n": float64(1)},
		SequenceNumber: 1,
		ReceivedAt:     time.Now(),
	}
	err := repo.Create(ctx, dup)
	require.Error(t, err, "the real unique constraint must reject a duplicate sequence number")
	assert.Contains(t, err.Error(), "already exists")

	// The duplicate must not have been stored.
	count, err := repo.CountByExecutionID(ctx, executionID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// The constraint is composite: the same sequence number on a DIFFERENT
	// execution is fine.
	otherExecution := &models.AgentExecution{
		AgentID: agentID,
		UserID:  userID,
		Input:   map[string]interface{}{"text": "second"},
	}
	require.NoError(t, NewAgentExecutionRepository(integrationDB).Create(ctx, otherExecution))
	mustCreateEvent(t, repo, otherExecution.ID, 1, map[string]interface{}{"n": float64(1)})
}

func eventSequences(events []models.AgentExecutionEvent) []int {
	seqs := make([]int, 0, len(events))
	for _, e := range events {
		seqs = append(seqs, e.SequenceNumber)
	}
	return seqs
}

func TestAgentExecutionEventRepository_Integration_ListByExecutionID(t *testing.T) {
	resetAgentExecutionData(t)
	userID, agentID, executionID := seedExecutionFixture(t)
	repo := NewAgentExecutionEventRepository(integrationDB)
	ctx := context.Background()

	// Insert out of order: the repository must return sequence order, not
	// insertion order.
	for _, seq := range []int{3, 1, 4, 2, 5} {
		mustCreateEvent(t, repo, executionID, seq, map[string]interface{}{"seq": float64(seq)})
	}

	// A second execution's event must never leak into the list.
	otherExecution := &models.AgentExecution{
		AgentID: agentID, UserID: userID, Input: map[string]interface{}{"text": "other"},
	}
	require.NoError(t, NewAgentExecutionRepository(integrationDB).Create(ctx, otherExecution))
	mustCreateEvent(t, repo, otherExecution.ID, 1, map[string]interface{}{"seq": float64(99)})

	t.Run("defaults limit to 50 and offset to 0, ordered by sequence ASC", func(t *testing.T) {
		events, total, err := repo.ListByExecutionID(ctx, executionID, 0, -1)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Equal(t, []int{1, 2, 3, 4, 5}, eventSequences(events))
	})

	t.Run("limit and offset paginate the ordered stream", func(t *testing.T) {
		events, total, err := repo.ListByExecutionID(ctx, executionID, 2, 1)
		require.NoError(t, err)
		assert.Equal(t, 5, total, "total count must ignore pagination")
		assert.Equal(t, []int{2, 3}, eventSequences(events))
	})

	t.Run("offset past the end yields an empty page with the full total", func(t *testing.T) {
		events, total, err := repo.ListByExecutionID(ctx, executionID, 2, 10)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		require.NotNil(t, events)
		assert.Empty(t, events)
	})
}

func TestAgentExecutionEventRepository_Integration_ListAfterSequence(t *testing.T) {
	resetAgentExecutionData(t)
	_, _, executionID := seedExecutionFixture(t)
	repo := NewAgentExecutionEventRepository(integrationDB)
	ctx := context.Background()

	for _, seq := range []int{1, 2, 3, 4, 5} {
		mustCreateEvent(t, repo, executionID, seq, map[string]interface{}{"seq": float64(seq)})
	}

	t.Run("returns only events strictly after the cursor, ASC", func(t *testing.T) {
		events, err := repo.ListAfterSequence(ctx, executionID, 2)
		require.NoError(t, err)
		assert.Equal(t, []int{3, 4, 5}, eventSequences(events))
	})

	t.Run("cursor at the head returns everything", func(t *testing.T) {
		events, err := repo.ListAfterSequence(ctx, executionID, 0)
		require.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3, 4, 5}, eventSequences(events))
	})

	t.Run("cursor at the tail returns an empty non-nil slice", func(t *testing.T) {
		events, err := repo.ListAfterSequence(ctx, executionID, 5)
		require.NoError(t, err)
		require.NotNil(t, events)
		assert.Empty(t, events)
	})
}

func TestAgentExecutionEventRepository_Integration_LatestAndCount(t *testing.T) {
	resetAgentExecutionData(t)
	userID, agentID, executionID := seedExecutionFixture(t)
	repo := NewAgentExecutionEventRepository(integrationDB)
	ctx := context.Background()

	for _, seq := range []int{2, 5, 1} {
		mustCreateEvent(t, repo, executionID, seq, map[string]interface{}{"seq": float64(seq)})
	}

	latest, err := repo.GetLatestByExecutionID(ctx, executionID)
	require.NoError(t, err)
	assert.Equal(t, 5, latest.SequenceNumber, "latest must be the highest sequence, not the last insert")

	count, err := repo.CountByExecutionID(ctx, executionID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// An execution with no events: not-found sentinel for latest, zero count.
	emptyExecution := &models.AgentExecution{
		AgentID: agentID, UserID: userID, Input: map[string]interface{}{"text": "empty"},
	}
	require.NoError(t, NewAgentExecutionRepository(integrationDB).Create(ctx, emptyExecution))

	_, err = repo.GetLatestByExecutionID(ctx, emptyExecution.ID)
	assert.ErrorIs(t, err, repositories.ErrAgentExecutionEventNotFound)

	count, err = repo.CountByExecutionID(ctx, emptyExecution.ID)
	require.NoError(t, err)
	assert.Zero(t, count)
}
