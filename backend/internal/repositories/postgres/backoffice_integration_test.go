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

// resetBackofficeTables clears every table the backoffice aggregation queries
// read, plus the parents (teams/projects) the seeded resources hang off.
// resetIntegrationTables only covers users/api_keys/user_preferences.
func resetBackofficeTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, projects, api_keys, prompts, artifacts, memories, "+
			"agents, agent_executions, claude_code_hooks_payload, cursor_ide_hooks_payload CASCADE")
	require.NoError(t, err)
}

// The seed helpers below control created_at (started_at for executions)
// explicitly so rows land in known ISO weeks.

func insertUserCreatedAt(t *testing.T, createdAt time.Time) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO users (id, email, name, created_at) VALUES ($1, $2, $3, $4)",
		id, id+"@backoffice.test", "Backoffice User "+id[:8], createdAt)
	require.NoError(t, err)
	return id
}

func insertAPIKeyCreatedAt(t *testing.T, userID string, createdAt time.Time) {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix, created_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6)",
		id, userID, "key-"+id[:8], "hash-"+id, "vbx_"+id[:8], createdAt)
	require.NoError(t, err)
}

func insertArtifactCreatedAt(t *testing.T, userID, teamID, projectID string, createdAt time.Time) {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO artifacts (id, user_id, team_id, project_id, title, slug, content, created_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
		id, userID, teamID, projectID, "Artifact "+id[:8], "artifact-"+id[:8], "content", createdAt)
	require.NoError(t, err)
}

func insertMemoryCreatedAt(t *testing.T, userID, teamID, projectID string, createdAt time.Time) {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO memories (id, user_id, team_id, project_id, text, created_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6)",
		id, userID, teamID, projectID, "memory text", createdAt)
	require.NoError(t, err)
}

func insertPromptCreatedAt(t *testing.T, userID, teamID, projectID string, createdAt time.Time) {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO prompts (id, user_id, team_id, project_id, name, slug, body, created_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
		id, userID, teamID, projectID, "Prompt "+id[:8], "prompt-"+id[:8], "body", createdAt)
	require.NoError(t, err)
}

func insertAgentCreatedAt(t *testing.T, userID, teamID string, createdAt time.Time) string {
	t.Helper()
	id := uuid.New().String()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO agents (id, user_id, team_id, name, description, created_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6)",
		id, userID, teamID, "Agent "+id[:8], "backoffice test agent", createdAt)
	require.NoError(t, err)
	return id
}

func insertAgentExecutionStartedAt(t *testing.T, agentID, userID string, startedAt time.Time) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO agent_executions (id, agent_id, user_id, started_at) VALUES ($1, $2, $3, $4)",
		uuid.New().String(), agentID, userID, startedAt)
	require.NoError(t, err)
}

func insertClaudeHookCreatedAt(t *testing.T, teamID, sessionID string, createdAt time.Time) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO claude_code_hooks_payload (session_id, hook_event_name, payload, team_id, created_at) "+
			"VALUES ($1, $2, $3, $4, $5)",
		sessionID, "PostToolUse", "{}", teamID, createdAt)
	require.NoError(t, err)
}

func insertCursorHookCreatedAt(t *testing.T, teamID, sessionID string, createdAt time.Time) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"INSERT INTO cursor_ide_hooks_payload (session_id, hook_event_name, payload, team_id, created_at) "+
			"VALUES ($1, $2, $3, $4, $5)",
		sessionID, "afterShellExecution", "{}", teamID, createdAt)
	require.NoError(t, err)
}

// TestBackofficeRepositoryIntegration_GetUsageMetrics seeds rows across two
// distinct ISO weeks and asserts the aggregation is correct per week and per
// table, then that the from/to filter excludes out-of-range weeks.
func TestBackofficeRepositoryIntegration_GetUsageMetrics(t *testing.T) {
	resetBackofficeTables(t)
	ctx := context.Background()
	repo := NewBackofficeRepository(integrationDB)

	week1 := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // Monday
	week2 := week1.AddDate(0, 0, 7)                      // next Monday

	// Week 1: 2 users, 1 artifact, 1 memory, 1 api key, 2 prompts, 1 agent,
	// 2 executions, claude sessions {a,a,b} -> 2 distinct, cursor {a,a} -> 1.
	// Week 2: 1 user, 2 artifacts, 1 api key, 1 execution (started_at),
	// cursor {z} -> 1 distinct.
	u1 := insertUserCreatedAt(t, week1.Add(9*time.Hour))
	u2 := insertUserCreatedAt(t, week1.Add(30*time.Hour))
	insertUserCreatedAt(t, week2.Add(9*time.Hour))

	teamID := insertTestTeam(t, u1)
	projectID := insertTestProject(t, u1, teamID)

	insertArtifactCreatedAt(t, u1, teamID, projectID, week1.Add(10*time.Hour))
	insertArtifactCreatedAt(t, u1, teamID, projectID, week2.Add(10*time.Hour))
	insertArtifactCreatedAt(t, u1, teamID, projectID, week2.Add(40*time.Hour))

	insertMemoryCreatedAt(t, u1, teamID, projectID, week1.Add(11*time.Hour))

	insertAPIKeyCreatedAt(t, u1, week1.Add(12*time.Hour))
	insertAPIKeyCreatedAt(t, u2, week2.Add(12*time.Hour))

	insertPromptCreatedAt(t, u1, teamID, projectID, week1.Add(13*time.Hour))
	insertPromptCreatedAt(t, u2, teamID, projectID, week1.Add(50*time.Hour))

	agentID := insertAgentCreatedAt(t, u1, teamID, week1.Add(14*time.Hour))
	insertAgentExecutionStartedAt(t, agentID, u1, week1.Add(15*time.Hour))
	insertAgentExecutionStartedAt(t, agentID, u1, week1.Add(16*time.Hour))
	insertAgentExecutionStartedAt(t, agentID, u1, week2.Add(15*time.Hour))

	insertClaudeHookCreatedAt(t, teamID, "claude-session-a", week1.Add(17*time.Hour))
	insertClaudeHookCreatedAt(t, teamID, "claude-session-a", week1.Add(18*time.Hour))
	insertClaudeHookCreatedAt(t, teamID, "claude-session-b", week1.Add(19*time.Hour))

	insertCursorHookCreatedAt(t, teamID, "cursor-session-a", week1.Add(17*time.Hour))
	insertCursorHookCreatedAt(t, teamID, "cursor-session-a", week1.Add(20*time.Hour))
	insertCursorHookCreatedAt(t, teamID, "cursor-session-z", week2.Add(17*time.Hour))

	all, err := repo.GetUsageMetrics(ctx, nil, nil)
	require.NoError(t, err)
	require.Len(t, all, 2, "exactly the two seeded weeks must appear")

	t.Run("weekly aggregation", func(t *testing.T) {
		// Weeks come back newest first.
		assert.Equal(t, "2026-01-12", all[0].WeekStart.Format("2006-01-02"))
		assert.Equal(t, "2026-01-05", all[1].WeekStart.Format("2006-01-02"))

		assert.Equal(t, models.UsageMetricsRow{
			WeekStart:           all[0].WeekStart,
			NewUsers:            1,
			NewArtifacts:        2,
			NewMemories:         0,
			NewAPIKeys:          1,
			NewPrompts:          0,
			NewAgents:           0,
			AgentExecutions:     1, // counted by started_at, agent itself is week 1
			ClaudeSessions:      0,
			CursorSessions:      1,
			TotalAIToolSessions: 1,
		}, all[0], "week 2 row")

		assert.Equal(t, models.UsageMetricsRow{
			WeekStart:           all[1].WeekStart,
			NewUsers:            2,
			NewArtifacts:        1,
			NewMemories:         1,
			NewAPIKeys:          1,
			NewPrompts:          2,
			NewAgents:           1,
			AgentExecutions:     2,
			ClaudeSessions:      2, // 3 hook rows, 2 distinct session_ids
			CursorSessions:      1, // 2 hook rows, 1 distinct session_id
			TotalAIToolSessions: 3,
		}, all[1], "week 1 row")
	})

	t.Run("date range filter excludes out-of-range weeks", func(t *testing.T) {
		week1End := week1.AddDate(0, 0, 6)
		week2End := week2.AddDate(0, 0, 6)

		onlyWeek2, err := repo.GetUsageMetrics(ctx, &week2, &week2End)
		require.NoError(t, err)
		require.Len(t, onlyWeek2, 1)
		assert.Equal(t, all[0], onlyWeek2[0])

		onlyWeek1, err := repo.GetUsageMetrics(ctx, &week1, &week1End)
		require.NoError(t, err)
		require.Len(t, onlyWeek1, 1)
		assert.Equal(t, all[1], onlyWeek1[0])

		bothWeeks, err := repo.GetUsageMetrics(ctx, &week1, &week2End)
		require.NoError(t, err)
		assert.Equal(t, all, bothWeeks)

		farFuture := week2.AddDate(0, 0, 14)
		farFutureEnd := farFuture.AddDate(0, 0, 6)
		none, err := repo.GetUsageMetrics(ctx, &farFuture, &farFutureEnd)
		require.NoError(t, err)
		assert.Empty(t, none)
	})
}

func TestBackofficeRepositoryIntegration_GetUsageMetrics_EmptyDatabase(t *testing.T) {
	resetBackofficeTables(t)
	repo := NewBackofficeRepository(integrationDB)

	got, err := repo.GetUsageMetrics(context.Background(), nil, nil)
	require.NoError(t, err)
	assert.Empty(t, got, "an empty database must yield an empty (non-error) result")
}

// TestBackofficeRepositoryIntegration_GetUserActivities asserts per-user
// distinct totals and MIN(created_at) first-timestamps, including that the
// LEFT JOIN cross-product (2 artifacts x 2 executions) does not inflate the
// DISTINCT counts and that a user with no resources comes back with zeros and
// nil first-timestamps.
func TestBackofficeRepositoryIntegration_GetUserActivities(t *testing.T) {
	resetBackofficeTables(t)
	ctx := context.Background()
	repo := NewBackofficeRepository(integrationDB)

	base := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	activeUser := insertUserCreatedAt(t, base)
	idleUser := insertUserCreatedAt(t, base.Add(24*time.Hour)) // newer, sorts first

	teamID := insertTestTeam(t, activeUser)
	projectID := insertTestProject(t, activeUser, teamID)

	firstArtifact := base.Add(2 * time.Hour)
	// Insert the later artifact first to prove MIN() wins over insertion order.
	insertArtifactCreatedAt(t, activeUser, teamID, projectID, firstArtifact.Add(5*time.Hour))
	insertArtifactCreatedAt(t, activeUser, teamID, projectID, firstArtifact)

	firstMemory := base.Add(3 * time.Hour)
	insertMemoryCreatedAt(t, activeUser, teamID, projectID, firstMemory)

	firstPrompt := base.Add(4 * time.Hour)
	insertPromptCreatedAt(t, activeUser, teamID, projectID, firstPrompt)

	agentID := insertAgentCreatedAt(t, activeUser, teamID, base.Add(5*time.Hour))
	insertAgentExecutionStartedAt(t, agentID, activeUser, base.Add(6*time.Hour))
	insertAgentExecutionStartedAt(t, agentID, activeUser, base.Add(7*time.Hour))

	rows, err := repo.GetUserActivities(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	idle := rows[0] // ordered by user created_at DESC
	assert.Equal(t, idleUser, idle.UserID)
	assert.Equal(t, idleUser+"@backoffice.test", idle.Email)
	assert.NotEmpty(t, idle.Name)
	assert.WithinDuration(t, base.Add(24*time.Hour), idle.UserCreatedAt, time.Second)
	assert.Zero(t, idle.TotalArtifacts)
	assert.Zero(t, idle.TotalMemories)
	assert.Zero(t, idle.TotalPrompts)
	assert.Zero(t, idle.TotalAgentsCreated)
	assert.Zero(t, idle.TotalAgentExecutionsRun)
	assert.Nil(t, idle.FirstArtifactCreatedAt, "LEFT JOIN with no artifacts must yield a nil first-timestamp")
	assert.Nil(t, idle.FirstMemoryCreatedAt)
	assert.Nil(t, idle.FirstPromptCreatedAt)

	active := rows[1]
	assert.Equal(t, activeUser, active.UserID)
	assert.Equal(t, activeUser+"@backoffice.test", active.Email)
	assert.WithinDuration(t, base, active.UserCreatedAt, time.Second)
	assert.Equal(t, 2, active.TotalArtifacts, "DISTINCT must collapse the join cross-product")
	assert.Equal(t, 1, active.TotalMemories)
	assert.Equal(t, 1, active.TotalPrompts)
	assert.Equal(t, 1, active.TotalAgentsCreated)
	assert.Equal(t, 2, active.TotalAgentExecutionsRun)
	require.NotNil(t, active.FirstArtifactCreatedAt)
	assert.WithinDuration(t, firstArtifact, *active.FirstArtifactCreatedAt, time.Second,
		"first artifact timestamp must be MIN(created_at), not insertion order")
	require.NotNil(t, active.FirstMemoryCreatedAt)
	assert.WithinDuration(t, firstMemory, *active.FirstMemoryCreatedAt, time.Second)
	require.NotNil(t, active.FirstPromptCreatedAt)
	assert.WithinDuration(t, firstPrompt, *active.FirstPromptCreatedAt, time.Second)
}
