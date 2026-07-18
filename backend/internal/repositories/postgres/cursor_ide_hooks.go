package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// countCursorIDESessionsQuery counts a user's distinct Cursor IDE sessions;
// shared by GetSessions, GetOverviewStats, and CountUniqueSessions.
const countCursorIDESessionsQuery = "SELECT COUNT(DISTINCT session_id) FROM cursor_ide_hooks_payload WHERE user_id = $1"

// cursorIDEHooksRepository implements the CursorIDEHooksRepository interface using PostgreSQL
type cursorIDEHooksRepository struct {
	db *database.DB
}

// NewCursorIDEHooksRepository creates a new PostgreSQL Cursor IDE hooks repository
func NewCursorIDEHooksRepository(db *database.DB) repositories.CursorIDEHooksRepository {
	return &cursorIDEHooksRepository{db: db}
}

// Create creates a new Cursor IDE hook payload record
func (r *cursorIDEHooksRepository) Create(ctx context.Context, payload *models.CursorIDEHookPayload) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	query := `
		INSERT INTO cursor_ide_hooks_payload
		(user_id, team_id, session_id, conversation_id, generation_id, hook_event_name, tool_name, workspace_roots,
		configuration, reference, context, input, output, induced_failure, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, query,
		payload.UserID,
		payload.TeamID,
		payload.SessionID,
		payload.ConversationID,
		payload.GenerationID,
		payload.HookEventName,
		payload.ToolName,
		pq.Array(payload.WorkspaceRoots),
		payload.Configuration,
		payload.Reference,
		payload.Context,
		payload.Input,
		payload.Output,
		payload.InducedFailure,
		payload.Payload,
	).Scan(&payload.ID, &payload.CreatedAt, &payload.UpdatedAt)

	if err != nil {
		slog.Error("Failed to create Cursor IDE hook payload", "error", err)
		return fmt.Errorf("failed to create Cursor IDE hook payload: %w", err)
	}

	return nil
}

// GetByID retrieves a specific Cursor IDE hook payload by ID
func (r *cursorIDEHooksRepository) GetByID(
	ctx context.Context, userID string, id int,
) (*models.CursorIDEHookPayload, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT id, user_id, team_id, session_id, conversation_id, generation_id, hook_event_name, tool_name, workspace_roots,
		       configuration, reference, context, input, output, induced_failure, payload, created_at, updated_at
		FROM cursor_ide_hooks_payload
		WHERE id = $1 AND user_id = $2`

	var payload models.CursorIDEHookPayload
	err := r.db.QueryRowContext(ctx, query, id, userID).Scan(
		&payload.ID,
		&payload.UserID,
		&payload.TeamID,
		&payload.SessionID,
		&payload.ConversationID,
		&payload.GenerationID,
		&payload.HookEventName,
		&payload.ToolName,
		pq.Array(&payload.WorkspaceRoots),
		&payload.Configuration,
		&payload.Reference,
		&payload.Context,
		&payload.Input,
		&payload.Output,
		&payload.InducedFailure,
		&payload.Payload,
		&payload.CreatedAt,
		&payload.UpdatedAt,
	)

	if err != nil {
		slog.Error("Failed to get Cursor IDE hook payload by ID", "error", err)
		return nil, fmt.Errorf("failed to get Cursor IDE hook payload: %w", err)
	}

	return &payload, nil
}

// List retrieves Cursor IDE hook payloads with filtering and pagination
func (r *cursorIDEHooksRepository) List(
	ctx context.Context, filters repositories.CursorIDEHooksFilters,
) (*models.CursorIDEHooksPaginatedResponse, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// All filters are optional; when none are set no WHERE clause is emitted
	// (an empty squirrel.And{} would render a spurious "(1=1)").
	conditions := buildCursorIDEHooksConditions(filters)

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

	response := &models.CursorIDEHooksPaginatedResponse{
		Data:       payloads,
		Page:       filters.Page,
		Limit:      filters.Limit,
		Total:      total,
		TotalPages: totalPages,
	}

	return response, nil
}

// countHooks counts Cursor IDE hook payloads matching the shared filter
// conditions used by List, so the count and page queries can never diverge.
func (r *cursorIDEHooksRepository) countHooks(ctx context.Context, conditions squirrel.And) (int, error) {
	builder := psql.Select("COUNT(*)").From("cursor_ide_hooks_payload")
	if len(conditions) > 0 {
		builder = builder.Where(conditions)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build Cursor IDE hook payloads count query: %w", err)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		slog.Error("Failed to count Cursor IDE hook payloads", "error", err)
		return 0, fmt.Errorf("failed to count Cursor IDE hook payloads: %w", err)
	}

	return total, nil
}

// queryHooks runs the paginated page query for List using the same shared filter
// conditions as the count query. The returned slice is always non-nil.
func (r *cursorIDEHooksRepository) queryHooks(
	ctx context.Context, conditions squirrel.And, filters repositories.CursorIDEHooksFilters,
) ([]models.CursorIDEHookPayload, error) {
	builder := psql.
		Select(
			"id", "user_id", "team_id", "session_id", "conversation_id", "generation_id",
			"hook_event_name", "tool_name", "workspace_roots", "configuration", "reference",
			"context", "input", "output", "induced_failure", "payload", "created_at", "updated_at",
		).
		From("cursor_ide_hooks_payload")
	if len(conditions) > 0 {
		builder = builder.Where(conditions)
	}

	limit, offset := cursorIDEHooksPaging(filters)
	query, args, err := builder.
		OrderBy("created_at DESC").
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build Cursor IDE hook payloads list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		slog.Error("Failed to query Cursor IDE hook payloads", "error", err)
		return nil, fmt.Errorf("failed to query Cursor IDE hook payloads: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	return scanCursorIDEHookRows(rows), nil
}

// scanCursorIDEHookRows scans the 18-column List projection into a non-nil
// slice. A row that fails to scan is logged and skipped (prior behaviour).
func scanCursorIDEHookRows(rows *sql.Rows) []models.CursorIDEHookPayload {
	payloads := make([]models.CursorIDEHookPayload, 0)
	for rows.Next() {
		var payload models.CursorIDEHookPayload
		err := rows.Scan(
			&payload.ID,
			&payload.UserID,
			&payload.TeamID,
			&payload.SessionID,
			&payload.ConversationID,
			&payload.GenerationID,
			&payload.HookEventName,
			&payload.ToolName,
			pq.Array(&payload.WorkspaceRoots),
			&payload.Configuration,
			&payload.Reference,
			&payload.Context,
			&payload.Input,
			&payload.Output,
			&payload.InducedFailure,
			&payload.Payload,
			&payload.CreatedAt,
			&payload.UpdatedAt,
		)
		if err != nil {
			slog.Error("Failed to scan Cursor IDE hook payload row", "error", err)
			continue
		}
		payloads = append(payloads, payload)
	}

	return payloads
}

// cursorIDEHooksPaging resolves the LIMIT/OFFSET for the List page query,
// preserving the prior contract: offset = (Page-1)*Limit. Negative results are
// clamped to 0 (provably non-wrapping for gosec G115); Postgres rejects negative
// LIMIT/OFFSET regardless.
func cursorIDEHooksPaging(filters repositories.CursorIDEHooksFilters) (limit, offset uint64) {
	if filters.Limit > 0 {
		limit = uint64(filters.Limit)
	}
	if rawOffset := (filters.Page - 1) * filters.Limit; rawOffset > 0 {
		offset = uint64(rawOffset)
	}
	return limit, offset
}

// GetSessions retrieves Cursor IDE sessions with pagination
//
//nolint:funlen,lll // Repository code with necessary complexity
func (r *cursorIDEHooksRepository) GetSessions(ctx context.Context, filters repositories.CursorSessionFilters) (*models.CursorSessionsResponse, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// Ensure UserID is provided for filtering
	if filters.UserID == nil {
		return nil, fmt.Errorf("user_id is required for session queries")
	}

	// Count total unique sessions for the user
	var total int
	err := r.db.QueryRowContext(ctx, countCursorIDESessionsQuery, *filters.UserID).Scan(&total)
	if err != nil {
		slog.Error("Failed to count Cursor IDE sessions", "error", err)
		return nil, fmt.Errorf("failed to count Cursor IDE sessions: %w", err)
	}

	// Calculate offset
	offset := (filters.Page - 1) * filters.Limit

	// Query sessions with pagination
	dataQuery := `
		SELECT
			session_id,
			MIN(created_at) as first_seen,
			MAX(created_at) as last_seen,
			COUNT(*) as hook_count,
			COUNT(DISTINCT tool_name) as unique_tools
		FROM cursor_ide_hooks_payload
		WHERE user_id = $1
		GROUP BY session_id
		ORDER BY MAX(created_at) DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, dataQuery, *filters.UserID, filters.Limit, offset)
	if err != nil {
		slog.Error("Failed to query Cursor IDE sessions", "error", err)
		return nil, fmt.Errorf("failed to query Cursor IDE sessions: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	var sessions []models.CursorSessionSummary
	for rows.Next() {
		var session models.CursorSessionSummary
		err := rows.Scan(
			&session.SessionID,
			&session.FirstSeen,
			&session.LastSeen,
			&session.HookCount,
			&session.UniqueTools,
		)
		if err != nil {
			slog.Error("Failed to scan Cursor IDE session row", "error", err)
			continue
		}
		sessions = append(sessions, session)
	}

	// Ensure we always return an empty array instead of nil
	if sessions == nil {
		sessions = []models.CursorSessionSummary{}
	}

	// Calculate total pages
	totalPages := (total + filters.Limit - 1) / filters.Limit

	// Create response
	response := &models.CursorSessionsResponse{
		Data:       sessions,
		Page:       filters.Page,
		Limit:      filters.Limit,
		Total:      total,
		TotalPages: totalPages,
	}

	return response, nil
}

// GetSessionCounts retrieves session counts by date for the specified range
func (r *cursorIDEHooksRepository) GetSessionCounts(
	ctx context.Context, userID string, days int,
) (*models.CursorSessionCountsResponse, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// Get total session count for the user within the specified date range
	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT session_id)
		FROM cursor_ide_hooks_payload
		WHERE user_id = $1 AND created_at >= CURRENT_DATE - INTERVAL '%d day'`, days)
	var totalSessions int
	err := r.db.QueryRowContext(ctx, countQuery, userID).Scan(&totalSessions)
	if err != nil {
		slog.Error("Failed to count total Cursor IDE sessions", "error", err)
		return nil, fmt.Errorf("failed to count total Cursor IDE sessions: %w", err)
	}

	// Get session counts by date for the specified range
	countsQuery := fmt.Sprintf(`
		SELECT
			DATE(created_at) as date,
			COUNT(DISTINCT session_id) as count
		FROM cursor_ide_hooks_payload
		WHERE user_id = $1 AND created_at >= CURRENT_DATE - INTERVAL '%d day'
		GROUP BY DATE(created_at)
		ORDER BY date DESC`, days)

	rows, err := r.db.QueryContext(ctx, countsQuery, userID)
	if err != nil {
		slog.Error("Failed to query Cursor IDE session counts by date", "error", err)
		return nil, fmt.Errorf("failed to query Cursor IDE session counts by date: %w", err)
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
			slog.Error("Failed to scan Cursor IDE session count row", "error", err)
			continue
		}
		counts = append(counts, count)
	}

	// Ensure we always return an empty array instead of nil
	if counts == nil {
		counts = []models.SessionCountByDate{}
	}

	// Create response
	response := &models.CursorSessionCountsResponse{
		TotalSessions: totalSessions,
		Counts:        counts,
	}

	return response, nil
}

// GetOverviewStats retrieves comprehensive overview statistics
const (
	cursorIDEOverviewThisWeekQuery = `SELECT COUNT(DISTINCT session_id) FROM cursor_ide_hooks_payload
		WHERE user_id = $1 AND created_at >= CURRENT_DATE - INTERVAL '7 day'`

	cursorIDEOverviewLastWeekQuery = `SELECT COUNT(DISTINCT session_id) FROM cursor_ide_hooks_payload
		WHERE user_id = $1 AND created_at >= CURRENT_DATE - INTERVAL '14 day'
		AND created_at < CURRENT_DATE - INTERVAL '7 day'`

	cursorIDEOverviewAvgPromptsQuery = `
		SELECT COALESCE(AVG(prompt_count), 0) as avg_prompts
		FROM (
			SELECT session_id, COUNT(*) as prompt_count
			FROM cursor_ide_hooks_payload
			WHERE user_id = $1 AND hook_event_name = 'tool:start'
			GROUP BY session_id
		) as session_prompts`

	cursorIDEOverviewUniqueToolsQuery = "SELECT COUNT(DISTINCT tool_name) FROM cursor_ide_hooks_payload " +
		"WHERE user_id = $1 AND tool_name IS NOT NULL"

	cursorIDEOverviewTopToolsQuery = `
		SELECT tool_name, COUNT(*) as usage_count
		FROM cursor_ide_hooks_payload
		WHERE user_id = $1 AND tool_name IS NOT NULL
		GROUP BY tool_name
		ORDER BY usage_count DESC
		LIMIT 3`

	cursorIDEOverviewAvgDurationQuery = `
		SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (last_seen - first_seen))/60), 0) as avg_duration_minutes
		FROM (
			SELECT session_id, MIN(created_at) as first_seen, MAX(created_at) as last_seen
			FROM cursor_ide_hooks_payload
			WHERE user_id = $1
			GROUP BY session_id
			HAVING MIN(created_at) != MAX(created_at)
		) as session_durations`
)

func (r *cursorIDEHooksRepository) GetOverviewStats(ctx context.Context, userID string) (*models.CursorOverviewStats, error) { //nolint:lll
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	var stats models.CursorOverviewStats

	// Get total sessions for the user
	err := r.db.QueryRowContext(ctx, countCursorIDESessionsQuery, userID).Scan(&stats.TotalSessions)
	if err != nil {
		slog.Error("Failed to count total sessions", "error", err)
		return nil, fmt.Errorf("failed to count total sessions: %w", err)
	}

	// Get sessions this week (last 7 days)
	scanStatOrZero(ctx, r.db, cursorIDEOverviewThisWeekQuery, userID,
		"Failed to count sessions this week", &stats.SessionsThisWeek)

	// Get sessions last week (8-14 days ago)
	scanStatOrZero(ctx, r.db, cursorIDEOverviewLastWeekQuery, userID,
		"Failed to count sessions last week", &stats.SessionsLastWeek)

	// Calculate weekly trend percentage
	stats.WeeklyTrendPercent = weeklyTrendPercent(stats.SessionsThisWeek, stats.SessionsLastWeek)

	// Get average user prompts per session (assuming tool:start events are prompts)
	scanStatOrZero(ctx, r.db, cursorIDEOverviewAvgPromptsQuery, userID,
		"Failed to calculate average prompts per session", &stats.AvgUserPromptsPerSession)

	// Get total unique tools
	scanStatOrZero(ctx, r.db, cursorIDEOverviewUniqueToolsQuery, userID,
		"Failed to count unique tools", &stats.TotalUniqueTools)

	// Get top 3 tools
	stats.TopTools = queryTopToolUsage(ctx, r.db, cursorIDEOverviewTopToolsQuery, userID)

	// Get average session duration
	scanStatOrZero(ctx, r.db, cursorIDEOverviewAvgDurationQuery, userID,
		"Failed to calculate average session duration", &stats.AvgSessionDurationMinutes)

	return &stats, nil
}

// buildCursorRecentActivitiesConditions builds the shared WHERE conditions for
// GetRecentActivities. The user_id constraint and the tool_name IS NOT NULL guard
// are always present; remaining filters are optional, matching the prior
// hand-built builder exactly. The caller has already verified filters.UserID is
// non-nil.
func buildCursorRecentActivitiesConditions(filters repositories.CursorRecentActivitiesFilters) squirrel.And {
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

// GetRecentActivities retrieves recent Cursor IDE activities with filtering and pagination
func (r *cursorIDEHooksRepository) GetRecentActivities(
	ctx context.Context, filters repositories.CursorRecentActivitiesFilters,
) (*models.CursorRecentActivitiesResponse, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// Ensure UserID is provided for filtering
	if filters.UserID == nil {
		return nil, fmt.Errorf("user_id is required for activities queries")
	}

	conditions := buildCursorRecentActivitiesConditions(filters)

	total, err := r.countRecentActivities(ctx, conditions)
	if err != nil {
		return nil, err
	}

	activities, err := r.queryRecentActivities(ctx, conditions, filters)
	if err != nil {
		return nil, err
	}

	totalPages := (total + filters.Limit - 1) / filters.Limit

	response := &models.CursorRecentActivitiesResponse{
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
func (r *cursorIDEHooksRepository) countRecentActivities(ctx context.Context, conditions squirrel.And) (int, error) {
	query, args, err := psql.
		Select("COUNT(*)").
		From("cursor_ide_hooks_payload").
		Where(conditions).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build Cursor IDE activities count query: %w", err)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		slog.Error("Failed to count Cursor IDE activities", "error", err)
		return 0, fmt.Errorf("failed to count Cursor IDE activities: %w", err)
	}

	return total, nil
}

// queryRecentActivities runs the paginated page query for GetRecentActivities
// using the same shared conditions as the count query. The returned slice is
// always non-nil.
func (r *cursorIDEHooksRepository) queryRecentActivities(
	ctx context.Context, conditions squirrel.And, filters repositories.CursorRecentActivitiesFilters,
) ([]models.CursorRecentActivity, error) {
	limit, offset := cursorRecentActivitiesPaging(filters)
	query, args, err := psql.
		Select("session_id", "tool_name", "input", "hook_event_name", "created_at").
		From("cursor_ide_hooks_payload").
		Where(conditions).
		OrderBy("created_at DESC").
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build Cursor IDE activities list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		slog.Error("Failed to query recent Cursor IDE activities", "error", err)
		return nil, fmt.Errorf("failed to query recent Cursor IDE activities: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	activities := make([]models.CursorRecentActivity, 0)
	for rows.Next() {
		var activity models.CursorRecentActivity
		err := rows.Scan(
			&activity.SessionID,
			&activity.ToolName,
			&activity.Input,
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

// cursorRecentActivitiesPaging resolves the LIMIT/OFFSET for the
// GetRecentActivities page query, preserving the prior offset = (Page-1)*Limit
// contract. Negative results clamp to 0 (provably non-wrapping for gosec G115).
func cursorRecentActivitiesPaging(filters repositories.CursorRecentActivitiesFilters) (limit, offset uint64) {
	if filters.Limit > 0 {
		limit = uint64(filters.Limit)
	}
	if rawOffset := (filters.Page - 1) * filters.Limit; rawOffset > 0 {
		offset = uint64(rawOffset)
	}
	return limit, offset
}

// buildCursorIDEHooksConditions builds the shared, all-optional WHERE conditions
// for the List count and page queries, so the two can never diverge. Predicate
// set and trigger conditions match the prior hand-built builder exactly; when no
// filter is set the slice is empty and callers must omit the WHERE clause.
func buildCursorIDEHooksConditions(filters repositories.CursorIDEHooksFilters) squirrel.And {
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
func (r *cursorIDEHooksRepository) SessionExists(ctx context.Context, userID, sessionID string) (bool, error) {
	if r.db == nil {
		return false, fmt.Errorf("database connection is nil")
	}

	query := "SELECT EXISTS(SELECT 1 FROM cursor_ide_hooks_payload WHERE user_id = $1 AND session_id = $2)"

	var exists bool
	err := r.db.QueryRowContext(ctx, query, userID, sessionID).Scan(&exists)
	if err != nil {
		slog.Error("Failed to check if Cursor IDE session exists", "error", err)
		return false, fmt.Errorf("failed to check if session exists: %w", err)
	}

	return exists, nil
}

// CountUniqueSessions returns the count of unique sessions for a user
func (r *cursorIDEHooksRepository) CountUniqueSessions(ctx context.Context, userID string) (int, error) {
	if r.db == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	var count int
	err := r.db.QueryRowContext(ctx, countCursorIDESessionsQuery, userID).Scan(&count)
	if err != nil {
		slog.Error("Failed to count unique Cursor IDE sessions", "error", err)
		return 0, fmt.Errorf("failed to count unique sessions: %w", err)
	}

	return count, nil
}

// DeleteSession deletes all hook payloads for a specific session
func (r *cursorIDEHooksRepository) DeleteSession(ctx context.Context, userID, sessionID string) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	// First, check if the session exists and belongs to the user
	exists, err := r.SessionExists(ctx, userID, sessionID)
	if err != nil {
		slog.Error("Failed to check if Cursor IDE session exists", "error", err)
		return fmt.Errorf("failed to check if session exists: %w", err)
	}

	if !exists {
		slog.With(
			"user_id", userID,
			"session_id", sessionID,
		).Warn("Attempted to delete non-existent or unauthorized Cursor IDE session")
		return repositories.ErrHookSessionNotFound
	}

	// Delete all records for this session
	query := "DELETE FROM cursor_ide_hooks_payload WHERE user_id = $1 AND session_id = $2"

	result, err := r.db.ExecContext(ctx, query, userID, sessionID)
	if err != nil {
		slog.Error("Failed to delete Cursor IDE session", "error", err)
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
	).Info("Cursor IDE session deleted successfully")

	return nil
}
