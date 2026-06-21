package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// BackofficeRepository implements the repositories.BackofficeRepository interface for PostgreSQL
type BackofficeRepository struct {
	db *database.DB
}

// NewBackofficeRepository creates a new BackofficeRepository
func NewBackofficeRepository(db *database.DB) repositories.BackofficeRepository {
	return &BackofficeRepository{
		db: db,
	}
}

// GetUsageMetrics retrieves weekly usage metrics with optional date filtering
func (r *BackofficeRepository) GetUsageMetrics(
	ctx context.Context,
	fromDate, toDate *time.Time,
) ([]models.UsageMetricsRow, error) {
	weekStarts, err := r.getWeekStarts(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]models.UsageMetricsRow, 0, len(weekStarts))
	for _, weekStart := range weekStarts {
		weekEnd := weekStart.AddDate(0, 0, 6).Add(23*time.Hour + 59*time.Minute + 59*time.Second)

		// Apply date filter if specified
		if fromDate != nil && toDate != nil {
			if weekEnd.Before(*fromDate) || weekStart.After(*toDate) {
				continue
			}
		}

		row := r.buildUsageMetricsRow(ctx, weekStart, weekEnd)
		results = append(results, row)
	}

	return results, nil
}

// getWeekStarts gets all unique weeks from all tables
func (r *BackofficeRepository) getWeekStarts(ctx context.Context) ([]time.Time, error) {
	timelineQuery := `
		SELECT DISTINCT date_trunc('week', created_at)::date AS week_start
		FROM (
			SELECT created_at FROM users
			UNION ALL
			SELECT created_at FROM artifacts
			UNION ALL
			SELECT created_at FROM memories
			UNION ALL
			SELECT created_at FROM api_keys
			UNION ALL
			SELECT created_at FROM prompts
			UNION ALL
			SELECT created_at FROM agents
			UNION ALL
			SELECT created_at FROM claude_code_hooks_payload
			UNION ALL
			SELECT created_at FROM cursor_ide_hooks_payload
		) AS all_dates
		ORDER BY week_start DESC
	`

	rows, err := r.db.QueryContext(ctx, timelineQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query timeline: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			// Log the close error but don't override the original error
			fmt.Printf("warning: failed to close rows: %v\n", closeErr)
		}
	}()

	var weekStarts []time.Time
	for rows.Next() {
		var ws time.Time
		if err := rows.Scan(&ws); err != nil {
			return nil, fmt.Errorf("failed to scan week start: %w", err)
		}
		weekStarts = append(weekStarts, ws)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating timeline rows: %w", err)
	}

	return weekStarts, nil
}

// buildUsageMetricsRow builds a usage metrics row for a specific week
func (r *BackofficeRepository) buildUsageMetricsRow(
	ctx context.Context,
	weekStart, weekEnd time.Time,
) models.UsageMetricsRow {
	row := models.UsageMetricsRow{WeekStart: weekStart}

	row.NewUsers = r.countByTable(ctx, "users", "created_at", weekStart, weekEnd)
	row.NewArtifacts = r.countByTable(ctx, "artifacts", "created_at", weekStart, weekEnd)
	row.NewMemories = r.countByTable(ctx, "memories", "created_at", weekStart, weekEnd)
	row.NewAPIKeys = r.countByTable(ctx, "api_keys", "created_at", weekStart, weekEnd)
	row.NewPrompts = r.countByTable(ctx, "prompts", "created_at", weekStart, weekEnd)
	row.NewAgents = r.countByTable(ctx, "agents", "created_at", weekStart, weekEnd)
	row.AgentExecutions = r.countByTable(ctx, "agent_executions", "started_at", weekStart, weekEnd)

	row.ClaudeSessions = r.countDistinctSessions(ctx, "claude_code_hooks_payload", weekStart, weekEnd)
	row.CursorSessions = r.countDistinctSessions(ctx, "cursor_ide_hooks_payload", weekStart, weekEnd)
	row.TotalAIToolSessions = row.ClaudeSessions + row.CursorSessions

	return row
}

// countByTable counts records in a table for a specific time range
func (r *BackofficeRepository) countByTable(
	ctx context.Context,
	table, timestampColumn string,
	weekStart, weekEnd time.Time,
) int {
	query := "SELECT COALESCE(COUNT(*), 0) FROM " + table +
		" WHERE " + timestampColumn + " >= $1 AND " + timestampColumn + " <= $2"

	var count int
	err := r.db.QueryRowContext(ctx, query, weekStart, weekEnd).Scan(&count)
	if err != nil {
		return 0
	}

	return count
}

// countDistinctSessions counts distinct sessions for a specific time range
func (r *BackofficeRepository) countDistinctSessions(
	ctx context.Context,
	table string,
	weekStart, weekEnd time.Time,
) int {
	query := "SELECT COALESCE(COUNT(DISTINCT session_id), 0) FROM " + table +
		" WHERE created_at >= $1 AND created_at <= $2"

	var count int
	err := r.db.QueryRowContext(ctx, query, weekStart, weekEnd).Scan(&count)
	if err != nil {
		return 0
	}

	return count
}

// GetUserActivities retrieves per-user activity summary
func (r *BackofficeRepository) GetUserActivities(ctx context.Context) ([]models.UserActivityRow, error) {
	query := r.buildUserActivityQuery()

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query user activities: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			// Log the close error but don't override the original error
			fmt.Printf("warning: failed to close rows: %v\n", closeErr)
		}
	}()

	results, err := r.scanUserActivityRows(rows)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// buildUserActivityQuery builds the SQL query for user activities
func (r *BackofficeRepository) buildUserActivityQuery() string {
	return `
		SELECT
			u.id AS user_id,
			u.email,
			u.name,
			u.created_at AS user_created_at,
			COALESCE(COUNT(DISTINCT a.id), 0) AS total_artifacts,
			MIN(a.created_at) AS first_artifact_created_at,
			COALESCE(COUNT(DISTINCT m.id), 0) AS total_memories,
			MIN(m.created_at) AS first_memory_created_at,
			COALESCE(COUNT(DISTINCT p.id), 0) AS total_prompts,
			MIN(p.created_at) AS first_prompt_created_at,
			COALESCE(COUNT(DISTINCT ag.id), 0) AS total_agents_created,
			COALESCE(COUNT(DISTINCT ax.id), 0) AS total_agent_executions_run
		FROM users u
		LEFT JOIN artifacts a ON u.id = a.user_id
		LEFT JOIN memories m ON u.id = m.user_id
		LEFT JOIN prompts p ON u.id = p.user_id
		LEFT JOIN agents ag ON u.id = ag.user_id
		LEFT JOIN agent_executions ax ON u.id = ax.user_id
		GROUP BY u.id, u.email, u.name, u.created_at
		ORDER BY u.created_at DESC
	`
}

// scanUserActivityRows scans and processes user activity query results
func (r *BackofficeRepository) scanUserActivityRows(rows *sql.Rows) ([]models.UserActivityRow, error) {
	results := make([]models.UserActivityRow, 0)
	for rows.Next() {
		var row models.UserActivityRow
		var name sql.NullString
		var firstArtifactAt, firstMemoryAt, firstPromptAt sql.NullTime
		err := rows.Scan(
			&row.UserID,
			&row.Email,
			&name,
			&row.UserCreatedAt,
			&row.TotalArtifacts,
			&firstArtifactAt,
			&row.TotalMemories,
			&firstMemoryAt,
			&row.TotalPrompts,
			&firstPromptAt,
			&row.TotalAgentsCreated,
			&row.TotalAgentExecutionsRun,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user activity row: %w", err)
		}
		if name.Valid {
			row.Name = name.String
		}
		// Convert sql.NullTime to *time.Time
		if firstArtifactAt.Valid {
			row.FirstArtifactCreatedAt = &firstArtifactAt.Time
		}
		if firstMemoryAt.Valid {
			row.FirstMemoryCreatedAt = &firstMemoryAt.Time
		}
		if firstPromptAt.Valid {
			row.FirstPromptCreatedAt = &firstPromptAt.Time
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user activity rows: %w", err)
	}

	return results, nil
}
