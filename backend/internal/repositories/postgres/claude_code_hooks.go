package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/Masterminds/squirrel"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// claudeCodeHooksRepository implements the ClaudeCodeHooksRepository interface using PostgreSQL
type claudeCodeHooksRepository struct {
	db *database.DB
}

// NewClaudeCodeHooksRepository creates a new PostgreSQL Claude Code hooks repository
func NewClaudeCodeHooksRepository(db *database.DB) repositories.ClaudeCodeHooksRepository {
	return &claudeCodeHooksRepository{db: db}
}

// Create creates a new Claude Code hook payload record
func (r *claudeCodeHooksRepository) Create(ctx context.Context, payload *models.ClaudeCodeHookPayload) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	query := `
		INSERT INTO claude_code_hooks_payload
		(user_id, team_id, session_id, transcript_path, cwd, hook_event_name, tool_name, tool_input,
		tool_response, prompt, message, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, query,
		payload.UserID,
		payload.TeamID,
		payload.SessionID,
		payload.TranscriptPath,
		payload.CWD,
		payload.HookEventName,
		payload.ToolName,
		payload.ToolInput,
		payload.ToolResponse,
		payload.Prompt,
		payload.Message,
		payload.Payload,
	).Scan(&payload.ID, &payload.CreatedAt, &payload.UpdatedAt)

	if err != nil {
		slog.Error("Failed to create Claude Code hook payload", "error", err)
		return fmt.Errorf("failed to create Claude Code hook payload: %w", err)
	}

	return nil
}

// GetByID retrieves a specific Claude Code hook payload by ID
func (r *claudeCodeHooksRepository) GetByID(
	ctx context.Context, userID string, id int,
) (*models.ClaudeCodeHookPayload, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT id, user_id, team_id, session_id, transcript_path, cwd, hook_event_name, tool_name,
		       tool_input, tool_response, prompt, message, payload, created_at, updated_at
		FROM claude_code_hooks_payload
		WHERE id = $1 AND user_id = $2`

	var payload models.ClaudeCodeHookPayload
	err := r.db.QueryRowContext(ctx, query, id, userID).Scan(
		&payload.ID,
		&payload.UserID,
		&payload.TeamID,
		&payload.SessionID,
		&payload.TranscriptPath,
		&payload.CWD,
		&payload.HookEventName,
		&payload.ToolName,
		&payload.ToolInput,
		&payload.ToolResponse,
		&payload.Prompt,
		&payload.Message,
		&payload.Payload,
		&payload.CreatedAt,
		&payload.UpdatedAt,
	)

	if err != nil {
		slog.Error("Failed to get Claude Code hook payload by ID", "error", err)
		return nil, fmt.Errorf("failed to get Claude Code hook payload: %w", err)
	}

	return &payload, nil
}

// List retrieves Claude Code hook payloads with filtering and pagination
func (r *claudeCodeHooksRepository) List(
	ctx context.Context, filters repositories.ClaudeCodeHooksFilters,
) (*models.ClaudeCodeHooksPaginatedResponse, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// All filters are optional; when none are set no WHERE clause is emitted
	// (an empty squirrel.And{} would render a spurious "(1=1)").
	conditions := buildClaudeCodeHooksConditions(filters)

	total, err := r.countHooks(ctx, conditions)
	if err != nil {
		return nil, err
	}

	payloads, err := r.queryHooks(ctx, conditions, filters)
	if err != nil {
		return nil, err
	}

	// Calculate total pages (Limit is a required positive page size per the
	// existing contract — preserved verbatim).
	totalPages := (total + filters.Limit - 1) / filters.Limit

	response := &models.ClaudeCodeHooksPaginatedResponse{
		Data:       payloads,
		Page:       filters.Page,
		Limit:      filters.Limit,
		Total:      total,
		TotalPages: totalPages,
	}

	return response, nil
}

// countHooks counts Claude Code hook payloads matching the shared filter
// conditions used by List, so the count and page queries can never diverge.
func (r *claudeCodeHooksRepository) countHooks(ctx context.Context, conditions squirrel.And) (int, error) {
	builder := psql.Select("COUNT(*)").From("claude_code_hooks_payload")
	if len(conditions) > 0 {
		builder = builder.Where(conditions)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build Claude Code hook payloads count query: %w", err)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		slog.Error("Failed to count Claude Code hook payloads", "error", err)
		return 0, fmt.Errorf("failed to count Claude Code hook payloads: %w", err)
	}

	return total, nil
}

// queryHooks runs the paginated page query for List using the same shared filter
// conditions as the count query. The returned slice is always non-nil.
func (r *claudeCodeHooksRepository) queryHooks(
	ctx context.Context, conditions squirrel.And, filters repositories.ClaudeCodeHooksFilters,
) ([]models.ClaudeCodeHookPayload, error) {
	builder := psql.
		Select(
			"id", "user_id", "team_id", "session_id", "transcript_path", "cwd", "hook_event_name", "tool_name",
			"tool_input", "tool_response", "prompt", "message", "payload", "created_at", "updated_at",
		).
		From("claude_code_hooks_payload")
	if len(conditions) > 0 {
		builder = builder.Where(conditions)
	}

	limit, offset := claudeCodeHooksPaging(filters)
	query, args, err := builder.
		OrderBy("created_at DESC").
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build Claude Code hook payloads list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		slog.Error("Failed to query Claude Code hook payloads", "error", err)
		return nil, fmt.Errorf("failed to query Claude Code hook payloads: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	return scanClaudeCodeHookRows(rows), nil
}

// scanClaudeCodeHookRows scans the 15-column List projection into a non-nil
// slice. A row that fails to scan is logged and skipped (prior behaviour).
func scanClaudeCodeHookRows(rows *sql.Rows) []models.ClaudeCodeHookPayload {
	payloads := make([]models.ClaudeCodeHookPayload, 0)
	for rows.Next() {
		var payload models.ClaudeCodeHookPayload
		err := rows.Scan(
			&payload.ID,
			&payload.UserID,
			&payload.TeamID,
			&payload.SessionID,
			&payload.TranscriptPath,
			&payload.CWD,
			&payload.HookEventName,
			&payload.ToolName,
			&payload.ToolInput,
			&payload.ToolResponse,
			&payload.Prompt,
			&payload.Message,
			&payload.Payload,
			&payload.CreatedAt,
			&payload.UpdatedAt,
		)
		if err != nil {
			slog.Error("Failed to scan Claude Code hook payload row", "error", err)
			continue
		}
		payloads = append(payloads, payload)
	}

	return payloads
}

// claudeCodeHooksPaging resolves the LIMIT/OFFSET for the List page query,
// preserving the prior contract: offset = (Page-1)*Limit. Negative results are
// clamped to 0 so the unsigned conversion is provably non-wrapping (gosec G115);
// Postgres rejects negative LIMIT/OFFSET regardless.
func claudeCodeHooksPaging(filters repositories.ClaudeCodeHooksFilters) (limit, offset uint64) {
	if filters.Limit > 0 {
		limit = uint64(filters.Limit)
	}
	if rawOffset := (filters.Page - 1) * filters.Limit; rawOffset > 0 {
		offset = uint64(rawOffset)
	}
	return limit, offset
}

// GetSessions retrieves Claude Code sessions with pagination
//
//nolint:funlen,lll // Repository code with necessary complexity
func (r *claudeCodeHooksRepository) GetSessions(ctx context.Context, filters repositories.SessionFilters) (*models.SessionsResponse, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// Ensure UserID is provided for filtering
	if filters.UserID == nil {
		return nil, fmt.Errorf("user_id is required for session queries")
	}

	// Count total unique sessions for the user
	countQuery := "SELECT COUNT(DISTINCT session_id) FROM claude_code_hooks_payload WHERE user_id = $1"
	var total int
	err := r.db.QueryRowContext(ctx, countQuery, *filters.UserID).Scan(&total)
	if err != nil {
		slog.Error("Failed to count Claude Code sessions", "error", err)
		return nil, fmt.Errorf("failed to count Claude Code sessions: %w", err)
	}

	// Calculate offset
	offset := (filters.Page - 1) * filters.Limit

	// Query sessions with pagination
	dataQuery := `
		SELECT
			p1.session_id,
			MIN(p1.created_at) as first_seen,
			MAX(p1.created_at) as last_seen,
			COUNT(*) as hook_count,
			(SELECT p2.cwd FROM claude_code_hooks_payload p2
			 WHERE p2.session_id = p1.session_id
			 AND p2.user_id = $1
			 AND p2.cwd IS NOT NULL
			 ORDER BY p2.created_at DESC
			 LIMIT 1) as latest_cwd,
			COUNT(DISTINCT p1.tool_name) as unique_tools
		FROM claude_code_hooks_payload p1
		WHERE p1.user_id = $1
		GROUP BY p1.session_id
		ORDER BY MAX(p1.created_at) DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, dataQuery, *filters.UserID, filters.Limit, offset)
	if err != nil {
		slog.Error("Failed to query Claude Code sessions", "error", err)
		return nil, fmt.Errorf("failed to query Claude Code sessions: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	var sessions []models.SessionSummary
	for rows.Next() {
		var session models.SessionSummary
		err := rows.Scan(
			&session.SessionID,
			&session.FirstSeen,
			&session.LastSeen,
			&session.HookCount,
			&session.LatestCWD,
			&session.UniqueTools,
		)
		if err != nil {
			slog.Error("Failed to scan Claude Code session row", "error", err)
			continue
		}
		sessions = append(sessions, session)
	}

	// Ensure we always return an empty array instead of nil
	if sessions == nil {
		sessions = []models.SessionSummary{}
	}

	// Calculate total pages
	totalPages := (total + filters.Limit - 1) / filters.Limit

	// Create response
	response := &models.SessionsResponse{
		Data:       sessions,
		Page:       filters.Page,
		Limit:      filters.Limit,
		Total:      total,
		TotalPages: totalPages,
	}

	return response, nil
}

// GetSessionCounts retrieves session counts by date for the specified range
func (r *claudeCodeHooksRepository) GetSessionCounts(
	ctx context.Context, userID string, days int,
) (*models.SessionCountsResponse, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// Get total session count for the user within the specified date range
	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT session_id)
		FROM claude_code_hooks_payload
		WHERE user_id = $1 AND created_at >= CURRENT_DATE - INTERVAL '%d day'`, days)
	var totalSessions int
	err := r.db.QueryRowContext(ctx, countQuery, userID).Scan(&totalSessions)
	if err != nil {
		slog.Error("Failed to count total Claude Code sessions", "error", err)
		return nil, fmt.Errorf("failed to count total Claude Code sessions: %w", err)
	}

	// Get session counts by date for the specified range
	countsQuery := fmt.Sprintf(`
		SELECT
			DATE(created_at) as date,
			COUNT(DISTINCT session_id) as count
		FROM claude_code_hooks_payload
		WHERE user_id = $1 AND created_at >= CURRENT_DATE - INTERVAL '%d day'
		GROUP BY DATE(created_at)
		ORDER BY date DESC`, days)

	rows, err := r.db.QueryContext(ctx, countsQuery, userID)
	if err != nil {
		slog.Error("Failed to query Claude Code session counts by date", "error", err)
		return nil, fmt.Errorf("failed to query Claude Code session counts by date: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	var counts []models.SessionCountByDate
	for rows.Next() {
		var count models.SessionCountByDate
		err := rows.Scan(&count.Date, &count.Count)
		if err != nil {
			slog.Error("Failed to scan Claude Code session count row", "error", err)
			continue
		}
		counts = append(counts, count)
	}

	// Ensure we always return an empty array instead of nil
	if counts == nil {
		counts = []models.SessionCountByDate{}
	}

	// Create response
	response := &models.SessionCountsResponse{
		TotalSessions: totalSessions,
		Counts:        counts,
	}

	return response, nil
}

//nolint:gocognit // Repository code with necessary complexity

// GetOverviewStats retrieves comprehensive overview statistics
//
//nolint:gocognit,gocyclo,funlen
func (r *claudeCodeHooksRepository) GetOverviewStats(ctx context.Context, userID string) (*models.OverviewStats, error) { //nolint:lll
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	var stats models.OverviewStats

	// Get total sessions for the user
	totalQuery := "SELECT COUNT(DISTINCT session_id) FROM claude_code_hooks_payload WHERE user_id = $1"
	err := r.db.QueryRowContext(ctx, totalQuery, userID).Scan(&stats.TotalSessions)
	if err != nil {
		slog.Error("Failed to count total sessions", "error", err)
		return nil, fmt.Errorf("failed to count total sessions: %w", err)
	}

	// Get sessions this week (last 7 days)
	thisWeekQuery := `SELECT COUNT(DISTINCT session_id) FROM claude_code_hooks_payload
		WHERE user_id = $1 AND created_at >= CURRENT_DATE - INTERVAL '7 day'`
	err = r.db.QueryRowContext(ctx, thisWeekQuery, userID).Scan(&stats.SessionsThisWeek)
	if err != nil {
		slog.Error("Failed to count sessions this week", "error", err)
		stats.SessionsThisWeek = 0
	}

	// Get sessions last week (8-14 days ago)
	lastWeekQuery := `SELECT COUNT(DISTINCT session_id) FROM claude_code_hooks_payload
		WHERE user_id = $1 AND created_at >= CURRENT_DATE - INTERVAL '14 day'
		AND created_at < CURRENT_DATE - INTERVAL '7 day'`
	err = r.db.QueryRowContext(ctx, lastWeekQuery, userID).Scan(&stats.SessionsLastWeek)
	if err != nil {
		slog.Error("Failed to count sessions last week", "error", err)
		stats.SessionsLastWeek = 0
	}

	// Calculate weekly trend percentage
	if stats.SessionsLastWeek > 0 {
		stats.WeeklyTrendPercent = float64(stats.SessionsThisWeek-stats.SessionsLastWeek) /
			float64(stats.SessionsLastWeek) * 100
	} else if stats.SessionsThisWeek > 0 {
		stats.WeeklyTrendPercent = 100.0 // 100% increase from 0
	}

	// Get average UserPromptSubmit per session
	avgPromptsQuery := `
		SELECT COALESCE(AVG(prompt_count), 0) as avg_prompts
		FROM (
			SELECT session_id, COUNT(*) as prompt_count
			FROM claude_code_hooks_payload
			WHERE user_id = $1 AND hook_event_name = 'UserPromptSubmit'
			GROUP BY session_id
		) as session_prompts`
	err = r.db.QueryRowContext(ctx, avgPromptsQuery, userID).Scan(&stats.AvgUserPromptsPerSession)
	if err != nil {
		slog.Error("Failed to calculate average prompts per session", "error", err)
		stats.AvgUserPromptsPerSession = 0
	}

	// Get total unique tools
	uniqueToolsQuery := "SELECT COUNT(DISTINCT tool_name) FROM claude_code_hooks_payload " +
		"WHERE user_id = $1 AND tool_name IS NOT NULL"
	err = r.db.QueryRowContext(ctx, uniqueToolsQuery, userID).Scan(&stats.TotalUniqueTools)
	if err != nil {
		slog.Error("Failed to count unique tools", "error", err)
		stats.TotalUniqueTools = 0
	}

	// Get top 3 tools
	topToolsQuery := `
		SELECT tool_name, COUNT(*) as usage_count
		FROM claude_code_hooks_payload
		WHERE user_id = $1 AND tool_name IS NOT NULL
		GROUP BY tool_name
		ORDER BY usage_count DESC
		LIMIT 3`
	topToolsRows, err := r.db.QueryContext(ctx, topToolsQuery, userID)
	if err != nil {
		slog.Error("Failed to query top tools", "error", err)
		stats.TopTools = []models.ToolUsageCount{}
	} else {
		defer func() {
			if closeErr := topToolsRows.Close(); closeErr != nil {
				slog.Error("Failed to close rows", "error", closeErr)
			}
		}()
		var topTools []models.ToolUsageCount
		for topToolsRows.Next() {
			var tool models.ToolUsageCount
			scanErr := topToolsRows.Scan(&tool.ToolName, &tool.Count)
			if scanErr != nil {
				slog.Error("Failed to scan top tool row", "error", scanErr)
				continue
			}
			topTools = append(topTools, tool)
		}
		if topTools == nil {
			topTools = []models.ToolUsageCount{}
		}
		stats.TopTools = topTools
	}

	// Get average session duration
	avgDurationQuery := `
		SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (last_seen - first_seen))/60), 0) as avg_duration_minutes
		FROM (
			SELECT session_id, MIN(created_at) as first_seen, MAX(created_at) as last_seen
			FROM claude_code_hooks_payload
			WHERE user_id = $1
			GROUP BY session_id
			HAVING MIN(created_at) != MAX(created_at)
		) as session_durations`
	err = r.db.QueryRowContext(ctx, avgDurationQuery, userID).Scan(&stats.AvgSessionDurationMinutes)
	if err != nil {
		slog.Error("Failed to calculate average session duration", "error", err)
		stats.AvgSessionDurationMinutes = 0
	}

	// Get total memories count
	memoriesCountQuery := "SELECT COUNT(*) FROM memories WHERE user_id = $1"
	err = r.db.QueryRowContext(ctx, memoriesCountQuery, userID).Scan(&stats.TotalMemories)
	if err != nil {
		slog.Error("Failed to count total memories", "error", err)
		stats.TotalMemories = 0
	}

	return &stats, nil
	//nolint:gocognit // Repository code with necessary complexity
}

// buildClaudeCodeRecentActivitiesConditions builds the shared WHERE conditions
// for GetRecentActivities. The user_id constraint and the tool_name IS NOT NULL
// guard are always present (backward compatibility); remaining filters are
// optional, matching the prior hand-built builder exactly. The caller has
// already verified filters.UserID is non-nil.
func buildClaudeCodeRecentActivitiesConditions(filters repositories.RecentActivitiesFilters) squirrel.And {
	conditions := squirrel.And{
		squirrel.Eq{"user_id": *filters.UserID},
		squirrel.NotEq{"tool_name": nil},
	}

	if filters.SessionID != nil {
		conditions = append(conditions, squirrel.Eq{"session_id": *filters.SessionID})
	}
	if filters.ToolName != nil {
		conditions = append(conditions, squirrel.Eq{"tool_name": *filters.ToolName})
	}
	if filters.HookEventName != nil {
		conditions = append(conditions, squirrel.Eq{"hook_event_name": *filters.HookEventName})
	}
	if filters.DateFrom != nil {
		conditions = append(conditions, squirrel.GtOrEq{"created_at": *filters.DateFrom})
	}
	if filters.DateTo != nil {
		conditions = append(conditions, squirrel.LtOrEq{"created_at": *filters.DateTo})
	}

	return conditions
}

// GetRecentActivities retrieves recent Claude Code activities with filtering and pagination
func (r *claudeCodeHooksRepository) GetRecentActivities(
	ctx context.Context, filters repositories.RecentActivitiesFilters,
) (*models.RecentActivitiesResponse, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// Ensure UserID is provided for filtering
	if filters.UserID == nil {
		return nil, fmt.Errorf("user_id is required for activities queries")
	}

	conditions := buildClaudeCodeRecentActivitiesConditions(filters)

	total, err := r.countRecentActivities(ctx, conditions)
	if err != nil {
		return nil, err
	}

	activities, err := r.queryRecentActivities(ctx, conditions, filters)
	if err != nil {
		return nil, err
	}

	totalPages := (total + filters.Limit - 1) / filters.Limit

	response := &models.RecentActivitiesResponse{
		Activities: activities,
		Page:       filters.Page,
		Limit:      filters.Limit,
		Total:      total,
		TotalPages: totalPages,
	}

	return response, nil
}

// countRecentActivities counts the rows matching the shared GetRecentActivities
// conditions, so the count and page queries can never diverge.
func (r *claudeCodeHooksRepository) countRecentActivities(ctx context.Context, conditions squirrel.And) (int, error) {
	query, args, err := psql.
		Select("COUNT(*)").
		From("claude_code_hooks_payload").
		Where(conditions).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build Claude Code activities count query: %w", err)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		slog.Error("Failed to count Claude Code activities", "error", err)
		return 0, fmt.Errorf("failed to count Claude Code activities: %w", err)
	}

	return total, nil
}

// queryRecentActivities runs the paginated page query for GetRecentActivities
// using the same shared conditions as the count query. The returned slice is
// always non-nil.
func (r *claudeCodeHooksRepository) queryRecentActivities(
	ctx context.Context, conditions squirrel.And, filters repositories.RecentActivitiesFilters,
) ([]models.RecentActivity, error) {
	limit, offset := claudeCodeRecentActivitiesPaging(filters)
	query, args, err := psql.
		Select("session_id", "cwd", "tool_name", "tool_input", "hook_event_name", "created_at").
		From("claude_code_hooks_payload").
		Where(conditions).
		OrderBy("created_at DESC").
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build Claude Code activities list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		slog.Error("Failed to query recent Claude Code activities", "error", err)
		return nil, fmt.Errorf("failed to query recent Claude Code activities: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	activities := make([]models.RecentActivity, 0)
	for rows.Next() {
		var activity models.RecentActivity
		err := rows.Scan(
			&activity.SessionID,
			&activity.CWD,
			&activity.ToolName,
			&activity.ToolInput,
			&activity.HookEventName,
			&activity.CreatedAt,
		)
		if err != nil {
			slog.Error("Failed to scan recent activity row", "error", err)
			continue
		}
		activities = append(activities, activity)
	}

	return activities, nil
}

// claudeCodeRecentActivitiesPaging resolves the LIMIT/OFFSET for the
// GetRecentActivities page query, preserving the prior offset = (Page-1)*Limit
// contract. Negative results clamp to 0 (provably non-wrapping for gosec G115).
func claudeCodeRecentActivitiesPaging(filters repositories.RecentActivitiesFilters) (limit, offset uint64) {
	if filters.Limit > 0 {
		limit = uint64(filters.Limit)
	}
	if rawOffset := (filters.Page - 1) * filters.Limit; rawOffset > 0 {
		offset = uint64(rawOffset)
	}
	return limit, offset
}

// buildClaudeCodeHooksConditions builds the shared, all-optional WHERE conditions
// for the List count and page queries, so the two can never diverge. Predicate
// set and trigger conditions match the prior hand-built builder exactly; when no
// filter is set the slice is empty and callers must omit the WHERE clause.
func buildClaudeCodeHooksConditions(filters repositories.ClaudeCodeHooksFilters) squirrel.And {
	conditions := squirrel.And{}

	// UserID filtering is required for security, but remains optional at this layer.
	if filters.UserID != nil {
		conditions = append(conditions, squirrel.Eq{"user_id": *filters.UserID})
	}
	if filters.SessionID != nil {
		conditions = append(conditions, squirrel.Eq{"session_id": *filters.SessionID})
	}
	if filters.HookEventName != nil {
		conditions = append(conditions, squirrel.Eq{"hook_event_name": *filters.HookEventName})
	}
	if filters.ToolName != nil {
		conditions = append(conditions, squirrel.Eq{"tool_name": *filters.ToolName})
	}

	return conditions
}

// SessionExists checks if a session exists for a user
func (r *claudeCodeHooksRepository) SessionExists(ctx context.Context, userID, sessionID string) (bool, error) {
	if r.db == nil {
		return false, fmt.Errorf("database connection is nil")
	}

	query := "SELECT EXISTS(SELECT 1 FROM claude_code_hooks_payload WHERE user_id = $1 AND session_id = $2)"

	var exists bool
	err := r.db.QueryRowContext(ctx, query, userID, sessionID).Scan(&exists)
	if err != nil {
		slog.Error("Failed to check if Claude Code session exists", "error", err)
		return false, fmt.Errorf("failed to check if session exists: %w", err)
	}

	return exists, nil
}

// CountUniqueSessions returns the count of unique sessions for a user
func (r *claudeCodeHooksRepository) CountUniqueSessions(ctx context.Context, userID string) (int, error) {
	if r.db == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	query := "SELECT COUNT(DISTINCT session_id) FROM claude_code_hooks_payload WHERE user_id = $1"

	var count int
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	if err != nil {
		slog.Error("Failed to count unique Claude Code sessions", "error", err)
		return 0, fmt.Errorf("failed to count unique sessions: %w", err)
	}

	return count, nil
}

// DeleteSession deletes all hook payloads for a specific session
func (r *claudeCodeHooksRepository) DeleteSession(ctx context.Context, userID, sessionID string) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	// First, check if the session exists and belongs to the user
	exists, err := r.SessionExists(ctx, userID, sessionID)
	if err != nil {
		slog.Error("Failed to check if Claude Code session exists", "error", err)
		return fmt.Errorf("failed to check if session exists: %w", err)
	}

	if !exists {
		slog.With(
			"user_id", userID,
			"session_id", sessionID,
		).Warn("Attempted to delete non-existent or unauthorized Claude Code session")
		return repositories.ErrHookSessionNotFound
	}

	// Delete all records for this session
	query := "DELETE FROM claude_code_hooks_payload WHERE user_id = $1 AND session_id = $2"

	result, err := r.db.ExecContext(ctx, query, userID, sessionID)
	if err != nil {
		slog.Error("Failed to delete Claude Code session", "error", err)
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		slog.Error("Failed to get rows affected count", "error", err)
		return fmt.Errorf("failed to verify deletion: %w", err)
	}

	slog.With(
		"user_id", userID,
		"session_id", sessionID,
		"rows_affected", rowsAffected,
	).Info("Claude Code session deleted successfully")

	return nil
}
