//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// resetIDEHooksTables clears the IDE-hooks tables plus the users/teams rows
// they hang off. resetIntegrationTables only truncates users, api_keys and
// user_preferences; the hooks tables reference teams, so both families are
// truncated explicitly here.
func resetIDEHooksTables(t *testing.T) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(),
		"TRUNCATE TABLE users, teams, claude_code_hooks_payload, cursor_ide_hooks_payload CASCADE")
	require.NoError(t, err)
}

// seedHooksUserAndTeam creates a user plus a team it owns (the hooks tables'
// team_id FK target) and returns both ids.
func seedHooksUserAndTeam(t *testing.T) (userID, teamID string) {
	t.Helper()
	userID = insertTestUser(t)
	teamID = insertTestTeam(t, userID)
	return userID, teamID
}

// insertClaudeHookRow seeds one claude_code_hooks_payload row with an explicit
// created_at so ordering assertions are deterministic.
func insertClaudeHookRow(
	t *testing.T, userID, teamID, sessionID, hookEvent string, toolName, cwd *string, createdAt time.Time,
) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), `
		INSERT INTO claude_code_hooks_payload
		(user_id, team_id, session_id, hook_event_name, tool_name, cwd, payload, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, '{}'::jsonb, $7, $7)`,
		userID, teamID, sessionID, hookEvent, toolName, cwd, createdAt)
	require.NoError(t, err)
}

// insertCursorHookRow seeds one cursor_ide_hooks_payload row with an explicit
// created_at so ordering assertions are deterministic.
func insertCursorHookRow(
	t *testing.T, userID, teamID, sessionID, hookEvent string, toolName *string, createdAt time.Time,
) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), `
		INSERT INTO cursor_ide_hooks_payload
		(user_id, team_id, session_id, hook_event_name, tool_name, payload, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, '{}'::jsonb, $6, $6)`,
		userID, teamID, sessionID, hookEvent, toolName, createdAt)
	require.NoError(t, err)
}

func TestClaudeCodeHooksIntegration_CreateGetByIDRoundTrip(t *testing.T) {
	resetIDEHooksTables(t)
	userID, teamID := seedHooksUserAndTeam(t)
	repo := NewClaudeCodeHooksRepository(integrationDB)
	ctx := context.Background()

	payload := &models.ClaudeCodeHookPayload{
		UserID:         &userID,
		TeamID:         teamID,
		SessionID:      "sess-roundtrip",
		TranscriptPath: strPtr("/tmp/transcript.jsonl"),
		CWD:            strPtr("/repo"),
		HookEventName:  "PostToolUse",
		ToolName:       strPtr("Bash"),
		ToolInput:      &models.JSONBData{"command": "ls -la", "timeout": float64(30)},
		ToolResponse:   &models.JSONBData{"output": "ok"},
		Prompt:         strPtr("run ls"),
		Message:        strPtr("done"),
		Payload:        models.JSONBData{"nested": map[string]interface{}{"k": "v"}},
	}
	require.NoError(t, repo.Create(ctx, payload))
	assert.Positive(t, payload.ID, "Create must write the DB-assigned id back")
	assert.False(t, payload.CreatedAt.IsZero(), "Create must write the DB timestamps back")

	got, err := repo.GetByID(ctx, userID, payload.ID)
	require.NoError(t, err)
	assert.Equal(t, payload.ID, got.ID)
	require.NotNil(t, got.UserID)
	assert.Equal(t, userID, *got.UserID)
	assert.Equal(t, teamID, got.TeamID)
	assert.Equal(t, "sess-roundtrip", got.SessionID)
	assert.Equal(t, payload.TranscriptPath, got.TranscriptPath)
	assert.Equal(t, payload.CWD, got.CWD)
	assert.Equal(t, "PostToolUse", got.HookEventName)
	assert.Equal(t, payload.ToolName, got.ToolName)
	// JSONB fidelity: nested structures and numbers must round-trip.
	require.NotNil(t, got.ToolInput)
	assert.Equal(t, *payload.ToolInput, *got.ToolInput)
	require.NotNil(t, got.ToolResponse)
	assert.Equal(t, *payload.ToolResponse, *got.ToolResponse)
	assert.Equal(t, payload.Prompt, got.Prompt)
	assert.Equal(t, payload.Message, got.Message)
	assert.Equal(t, payload.Payload, got.Payload)
	assert.WithinDuration(t, payload.CreatedAt, got.CreatedAt, time.Second)

	// Tenancy: another user cannot read the row by id.
	otherUser := insertTestUser(t)
	_, err = repo.GetByID(ctx, otherUser, payload.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestClaudeCodeHooksIntegration_ListFiltersPaginationTenancy(t *testing.T) {
	resetIDEHooksTables(t)
	userA, teamA := seedHooksUserAndTeam(t)
	userB, teamB := seedHooksUserAndTeam(t)
	repo := NewClaudeCodeHooksRepository(integrationDB)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	bash, read := strPtr("Bash"), strPtr("Read")
	insertClaudeHookRow(t, userA, teamA, "sess-1", "PreToolUse", bash, nil, base.Add(-5*time.Minute))
	insertClaudeHookRow(t, userA, teamA, "sess-1", "PostToolUse", bash, strPtr("/repo"), base.Add(-4*time.Minute))
	insertClaudeHookRow(t, userA, teamA, "sess-1", "PostToolUse", read, nil, base.Add(-3*time.Minute))
	insertClaudeHookRow(t, userA, teamA, "sess-2", "UserPromptSubmit", nil, nil, base.Add(-2*time.Minute))
	insertClaudeHookRow(t, userA, teamA, "sess-2", "PostToolUse", bash, nil, base.Add(-time.Minute))
	// User B row with a colliding session id: must never leak into A's results.
	insertClaudeHookRow(t, userB, teamB, "sess-1", "PostToolUse", bash, nil, base)

	page1, err := repo.List(ctx, repositories.ClaudeCodeHooksFilters{UserID: &userA, Page: 1, Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, 5, page1.Total)
	assert.Equal(t, 3, page1.TotalPages)
	require.Len(t, page1.Data, 2)
	// Newest first.
	assert.Equal(t, "sess-2", page1.Data[0].SessionID)
	assert.Equal(t, "PostToolUse", page1.Data[0].HookEventName)
	for _, row := range page1.Data {
		require.NotNil(t, row.UserID)
		assert.Equal(t, userA, *row.UserID, "another user's rows must never appear")
	}

	page3, err := repo.List(ctx, repositories.ClaudeCodeHooksFilters{UserID: &userA, Page: 3, Limit: 2})
	require.NoError(t, err)
	require.Len(t, page3.Data, 1, "page 3 of 5 rows with limit 2 holds the single oldest row")
	assert.Equal(t, "PreToolUse", page3.Data[0].HookEventName)

	bySession, err := repo.List(ctx, repositories.ClaudeCodeHooksFilters{
		UserID: &userA, SessionID: strPtr("sess-1"), Page: 1, Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, bySession.Total)
	for _, row := range bySession.Data {
		assert.Equal(t, "sess-1", row.SessionID)
	}

	byEventAndTool, err := repo.List(ctx, repositories.ClaudeCodeHooksFilters{
		UserID: &userA, HookEventName: strPtr("PostToolUse"), ToolName: bash, Page: 1, Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, byEventAndTool.Total, "event+tool filter must combine conjunctively")
}

func TestClaudeCodeHooksIntegration_SessionsExistsAndCounts(t *testing.T) {
	resetIDEHooksTables(t)
	userA, teamA := seedHooksUserAndTeam(t)
	userB, teamB := seedHooksUserAndTeam(t)
	repo := NewClaudeCodeHooksRepository(integrationDB)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	bash, read := strPtr("Bash"), strPtr("Read")
	insertClaudeHookRow(t, userA, teamA, "sess-old", "PreToolUse", bash, strPtr("/old"), base.Add(-3*time.Hour))
	insertClaudeHookRow(t, userA, teamA, "sess-old", "PostToolUse", read, strPtr("/repo/latest"), base.Add(-2*time.Hour))
	insertClaudeHookRow(t, userA, teamA, "sess-new", "PostToolUse", bash, nil, base.Add(-time.Hour))
	insertClaudeHookRow(t, userB, teamB, "sess-b", "PostToolUse", bash, nil, base)

	resp, err := repo.GetSessions(ctx, repositories.SessionFilters{UserID: &userA, Page: 1, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 2, resp.Total, "user B's session must not count for user A")
	require.Len(t, resp.Data, 2)
	// Ordered by last activity, newest session first.
	assert.Equal(t, "sess-new", resp.Data[0].SessionID)
	assert.Equal(t, 1, resp.Data[0].HookCount)
	sessOld := resp.Data[1]
	assert.Equal(t, "sess-old", sessOld.SessionID)
	assert.Equal(t, 2, sessOld.HookCount)
	assert.Equal(t, 2, sessOld.UniqueTools)
	require.NotNil(t, sessOld.LatestCWD)
	assert.Equal(t, "/repo/latest", *sessOld.LatestCWD, "latest_cwd must come from the most recent row with a cwd")
	assert.WithinDuration(t, base.Add(-3*time.Hour), sessOld.FirstSeen, time.Second)
	assert.WithinDuration(t, base.Add(-2*time.Hour), sessOld.LastSeen, time.Second)

	exists, err := repo.SessionExists(ctx, userA, "sess-old")
	require.NoError(t, err)
	assert.True(t, exists)
	exists, err = repo.SessionExists(ctx, userA, "sess-b")
	require.NoError(t, err)
	assert.False(t, exists, "another user's session must not exist for user A")
	exists, err = repo.SessionExists(ctx, userB, "sess-b")
	require.NoError(t, err)
	assert.True(t, exists)

	countA, err := repo.CountUniqueSessions(ctx, userA)
	require.NoError(t, err)
	assert.Equal(t, 2, countA)
	countB, err := repo.CountUniqueSessions(ctx, userB)
	require.NoError(t, err)
	assert.Equal(t, 1, countB)
}

func TestClaudeCodeHooksIntegration_DeleteSessionScopedToUser(t *testing.T) {
	resetIDEHooksTables(t)
	userA, teamA := seedHooksUserAndTeam(t)
	userB, teamB := seedHooksUserAndTeam(t)
	repo := NewClaudeCodeHooksRepository(integrationDB)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	bash := strPtr("Bash")
	insertClaudeHookRow(t, userA, teamA, "sess-shared", "PreToolUse", bash, nil, base.Add(-2*time.Minute))
	insertClaudeHookRow(t, userA, teamA, "sess-shared", "PostToolUse", bash, nil, base.Add(-time.Minute))
	insertClaudeHookRow(t, userA, teamA, "sess-keep", "PostToolUse", bash, nil, base)
	insertClaudeHookRow(t, userB, teamB, "sess-shared", "PostToolUse", bash, nil, base)

	require.NoError(t, repo.DeleteSession(ctx, userA, "sess-shared"))

	exists, err := repo.SessionExists(ctx, userA, "sess-shared")
	require.NoError(t, err)
	assert.False(t, exists, "user A's session must be gone")
	exists, err = repo.SessionExists(ctx, userA, "sess-keep")
	require.NoError(t, err)
	assert.True(t, exists, "user A's other session must survive")
	exists, err = repo.SessionExists(ctx, userB, "sess-shared")
	require.NoError(t, err)
	assert.True(t, exists, "user B's rows in the same-named session must survive")

	var bRows int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM claude_code_hooks_payload WHERE user_id = $1", userB).Scan(&bRows))
	assert.Equal(t, 1, bRows)

	// Re-deleting (or deleting a session you don't own) reports not-found.
	err = repo.DeleteSession(ctx, userA, "sess-shared")
	assert.ErrorIs(t, err, repositories.ErrHookSessionNotFound)
}

func TestCursorIDEHooksIntegration_CreateGetByIDRoundTrip(t *testing.T) {
	resetIDEHooksTables(t)
	userID, teamID := seedHooksUserAndTeam(t)
	repo := NewCursorIDEHooksRepository(integrationDB)
	ctx := context.Background()

	payload := &models.CursorIDEHookPayload{
		UserID:         &userID,
		TeamID:         teamID,
		SessionID:      "conv-roundtrip",
		ConversationID: strPtr("conv-roundtrip"),
		GenerationID:   strPtr("gen-1"),
		HookEventName:  "beforeShellExecution",
		ToolName:       strPtr("Shell"),
		WorkspaceRoots: []string{"/w1", "/w2"},
		Configuration:  &models.JSONBData{"model": "auto"},
		Input:          &models.JSONBData{"command": "ls -la", "timeout": float64(30)},
		Output:         &models.JSONBData{"stdout": "ok"},
		Payload:        models.JSONBData{"nested": map[string]interface{}{"k": "v"}},
	}
	require.NoError(t, repo.Create(ctx, payload))
	assert.Positive(t, payload.ID, "Create must write the DB-assigned id back")
	assert.False(t, payload.CreatedAt.IsZero(), "Create must write the DB timestamps back")

	got, err := repo.GetByID(ctx, userID, payload.ID)
	require.NoError(t, err)
	assert.Equal(t, payload.ID, got.ID)
	require.NotNil(t, got.UserID)
	assert.Equal(t, userID, *got.UserID)
	assert.Equal(t, teamID, got.TeamID)
	assert.Equal(t, "conv-roundtrip", got.SessionID)
	assert.Equal(t, payload.ConversationID, got.ConversationID)
	assert.Equal(t, payload.GenerationID, got.GenerationID)
	assert.Equal(t, "beforeShellExecution", got.HookEventName)
	assert.Equal(t, payload.ToolName, got.ToolName)
	assert.Equal(t, []string{"/w1", "/w2"}, got.WorkspaceRoots, "text[] workspace roots must round-trip")
	// JSONB fidelity: nested structures and numbers must round-trip.
	require.NotNil(t, got.Configuration)
	assert.Equal(t, *payload.Configuration, *got.Configuration)
	require.NotNil(t, got.Input)
	assert.Equal(t, *payload.Input, *got.Input)
	require.NotNil(t, got.Output)
	assert.Equal(t, *payload.Output, *got.Output)
	assert.Nil(t, got.InducedFailure)
	assert.Equal(t, payload.Payload, got.Payload)
	assert.WithinDuration(t, payload.CreatedAt, got.CreatedAt, time.Second)

	// Tenancy: another user cannot read the row by id.
	otherUser := insertTestUser(t)
	_, err = repo.GetByID(ctx, otherUser, payload.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestCursorIDEHooksIntegration_ListFiltersPaginationTenancy(t *testing.T) {
	resetIDEHooksTables(t)
	userA, teamA := seedHooksUserAndTeam(t)
	userB, teamB := seedHooksUserAndTeam(t)
	repo := NewCursorIDEHooksRepository(integrationDB)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	shell, edit := strPtr("Shell"), strPtr("Edit")
	insertCursorHookRow(t, userA, teamA, "conv-1", "beforeShellExecution", shell, base.Add(-5*time.Minute))
	insertCursorHookRow(t, userA, teamA, "conv-1", "afterShellExecution", shell, base.Add(-4*time.Minute))
	insertCursorHookRow(t, userA, teamA, "conv-1", "afterFileEdit", edit, base.Add(-3*time.Minute))
	insertCursorHookRow(t, userA, teamA, "conv-2", "beforeSubmitPrompt", nil, base.Add(-2*time.Minute))
	insertCursorHookRow(t, userA, teamA, "conv-2", "afterShellExecution", shell, base.Add(-time.Minute))
	// User B row with a colliding session id: must never leak into A's results.
	insertCursorHookRow(t, userB, teamB, "conv-1", "afterShellExecution", shell, base)

	page1, err := repo.List(ctx, repositories.CursorIDEHooksFilters{UserID: &userA, Page: 1, Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, 5, page1.Total)
	assert.Equal(t, 3, page1.TotalPages)
	require.Len(t, page1.Data, 2)
	// Newest first.
	assert.Equal(t, "conv-2", page1.Data[0].SessionID)
	assert.Equal(t, "afterShellExecution", page1.Data[0].HookEventName)
	for _, row := range page1.Data {
		require.NotNil(t, row.UserID)
		assert.Equal(t, userA, *row.UserID, "another user's rows must never appear")
	}

	page3, err := repo.List(ctx, repositories.CursorIDEHooksFilters{UserID: &userA, Page: 3, Limit: 2})
	require.NoError(t, err)
	require.Len(t, page3.Data, 1, "page 3 of 5 rows with limit 2 holds the single oldest row")
	assert.Equal(t, "beforeShellExecution", page3.Data[0].HookEventName)

	bySession, err := repo.List(ctx, repositories.CursorIDEHooksFilters{
		UserID: &userA, SessionID: strPtr("conv-1"), Page: 1, Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, bySession.Total)
	for _, row := range bySession.Data {
		assert.Equal(t, "conv-1", row.SessionID)
	}

	byEventAndTool, err := repo.List(ctx, repositories.CursorIDEHooksFilters{
		UserID: &userA, HookEventName: strPtr("afterShellExecution"), ToolName: shell, Page: 1, Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, byEventAndTool.Total, "event+tool filter must combine conjunctively")
}

func TestCursorIDEHooksIntegration_SessionsExistsAndCounts(t *testing.T) {
	resetIDEHooksTables(t)
	userA, teamA := seedHooksUserAndTeam(t)
	userB, teamB := seedHooksUserAndTeam(t)
	repo := NewCursorIDEHooksRepository(integrationDB)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	shell, edit := strPtr("Shell"), strPtr("Edit")
	insertCursorHookRow(t, userA, teamA, "conv-old", "beforeShellExecution", shell, base.Add(-3*time.Hour))
	insertCursorHookRow(t, userA, teamA, "conv-old", "afterFileEdit", edit, base.Add(-2*time.Hour))
	insertCursorHookRow(t, userA, teamA, "conv-new", "afterShellExecution", shell, base.Add(-time.Hour))
	insertCursorHookRow(t, userB, teamB, "conv-b", "afterShellExecution", shell, base)

	resp, err := repo.GetSessions(ctx, repositories.CursorSessionFilters{UserID: &userA, Page: 1, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 2, resp.Total, "user B's session must not count for user A")
	require.Len(t, resp.Data, 2)
	// Ordered by last activity, newest session first.
	assert.Equal(t, "conv-new", resp.Data[0].SessionID)
	assert.Equal(t, 1, resp.Data[0].HookCount)
	sessOld := resp.Data[1]
	assert.Equal(t, "conv-old", sessOld.SessionID)
	assert.Equal(t, 2, sessOld.HookCount)
	assert.Equal(t, 2, sessOld.UniqueTools)
	assert.WithinDuration(t, base.Add(-3*time.Hour), sessOld.FirstSeen, time.Second)
	assert.WithinDuration(t, base.Add(-2*time.Hour), sessOld.LastSeen, time.Second)

	exists, err := repo.SessionExists(ctx, userA, "conv-old")
	require.NoError(t, err)
	assert.True(t, exists)
	exists, err = repo.SessionExists(ctx, userA, "conv-b")
	require.NoError(t, err)
	assert.False(t, exists, "another user's session must not exist for user A")
	exists, err = repo.SessionExists(ctx, userB, "conv-b")
	require.NoError(t, err)
	assert.True(t, exists)

	countA, err := repo.CountUniqueSessions(ctx, userA)
	require.NoError(t, err)
	assert.Equal(t, 2, countA)
	countB, err := repo.CountUniqueSessions(ctx, userB)
	require.NoError(t, err)
	assert.Equal(t, 1, countB)
}

func TestCursorIDEHooksIntegration_DeleteSessionScopedToUser(t *testing.T) {
	resetIDEHooksTables(t)
	userA, teamA := seedHooksUserAndTeam(t)
	userB, teamB := seedHooksUserAndTeam(t)
	repo := NewCursorIDEHooksRepository(integrationDB)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	shell := strPtr("Shell")
	insertCursorHookRow(t, userA, teamA, "conv-shared", "beforeShellExecution", shell, base.Add(-2*time.Minute))
	insertCursorHookRow(t, userA, teamA, "conv-shared", "afterShellExecution", shell, base.Add(-time.Minute))
	insertCursorHookRow(t, userA, teamA, "conv-keep", "afterShellExecution", shell, base)
	insertCursorHookRow(t, userB, teamB, "conv-shared", "afterShellExecution", shell, base)

	require.NoError(t, repo.DeleteSession(ctx, userA, "conv-shared"))

	exists, err := repo.SessionExists(ctx, userA, "conv-shared")
	require.NoError(t, err)
	assert.False(t, exists, "user A's session must be gone")
	exists, err = repo.SessionExists(ctx, userA, "conv-keep")
	require.NoError(t, err)
	assert.True(t, exists, "user A's other session must survive")
	exists, err = repo.SessionExists(ctx, userB, "conv-shared")
	require.NoError(t, err)
	assert.True(t, exists, "user B's rows in the same-named session must survive")

	var bRows int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM cursor_ide_hooks_payload WHERE user_id = $1", userB).Scan(&bRows))
	assert.Equal(t, 1, bRows)

	// Re-deleting (or deleting a session you don't own) reports not-found.
	err = repo.DeleteSession(ctx, userA, "conv-shared")
	assert.ErrorIs(t, err, repositories.ErrHookSessionNotFound)
}
