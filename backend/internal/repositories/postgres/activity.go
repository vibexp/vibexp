package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Masterminds/squirrel"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// activityRepository implements the ActivityRepository interface using PostgreSQL
type activityRepository struct {
	db *database.DB
}

// NewActivityRepository creates a new PostgreSQL activity repository
func NewActivityRepository(db *database.DB) repositories.ActivityRepository {
	return &activityRepository{db: db}
}

// Create creates a new activity record
func (r *activityRepository) Create(ctx context.Context, activity *models.Activity) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Convert metadata to JSON
	metadataJSON := "{}"
	if activity.Metadata != nil {
		if jsonData, err := json.Marshal(activity.Metadata); err == nil {
			metadataJSON = string(jsonData)
		}
	}

	query := `
		INSERT INTO activities
		(id, user_id, activity_type, entity_type, entity_id, session_id, description, metadata, source_ip, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING created_at`

	err := r.db.QueryRowContext(ctx, query,
		activity.ID,
		activity.UserID,
		activity.ActivityType,
		activity.EntityType,
		activity.EntityID,
		activity.SessionID,
		activity.Description,
		metadataJSON,
		activity.SourceIP,
		activity.UserAgent,
	).Scan(&activity.CreatedAt)

	if err != nil {
		slog.Error("Failed to create activity", "error", err)
		return fmt.Errorf("failed to create activity: %w", err)
	}

	return nil
}

// GetByID retrieves a specific activity by ID
func (r *activityRepository) GetByID(ctx context.Context, userID, activityID string) (*models.Activity, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT id, user_id, activity_type, entity_type, entity_id, session_id,
		       description, metadata, source_ip, user_agent, created_at
		FROM activities
		WHERE id = $1 AND user_id = $2`

	var activity models.Activity
	var metadataJSON string
	err := r.db.QueryRowContext(ctx, query, activityID, userID).Scan(
		&activity.ID,
		&activity.UserID,
		&activity.ActivityType,
		&activity.EntityType,
		&activity.EntityID,
		&activity.SessionID,
		&activity.Description,
		&metadataJSON,
		&activity.SourceIP,
		&activity.UserAgent,
		&activity.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repositories.ErrActivityNotFound
		}
		slog.Error("Failed to get activity by ID", "error", err)
		return nil, fmt.Errorf("failed to get activity: %w", err)
	}

	// Parse metadata JSON
	if metadataJSON != "" {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(metadataJSON), &metadata); err == nil {
			activity.Metadata = metadata
		}
	}

	return &activity, nil
}

// List retrieves activities with filtering and pagination
func (r *activityRepository) List(
	ctx context.Context, filters repositories.ActivityFilters,
) (*models.ActivityListResponse, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// All filters are optional, so conditions may be empty — in which case no
	// WHERE clause is emitted at all (matching the prior hand-built behaviour;
	// an empty squirrel.And{} would render a spurious "(1=1)").
	conditions := buildActivityListConditions(filters)

	totalCount, err := r.countActivities(ctx, conditions)
	if err != nil {
		return nil, err
	}

	activities, err := r.queryActivities(ctx, conditions, filters)
	if err != nil {
		return nil, err
	}

	// Calculate pagination with division by zero protection
	perPage := filters.Limit
	if perPage <= 0 {
		perPage = 1 // Default to 1 to prevent division by zero
	}
	totalPages := (totalCount + perPage - 1) / perPage
	page := (filters.Offset / perPage) + 1

	response := &models.ActivityListResponse{
		Activities: activities,
		TotalCount: totalCount,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}

	return response, nil
}

// countActivities counts activities matching the shared filter conditions used
// by List, so the count and page queries can never diverge. An empty conditions
// slice yields a count over the whole table with no WHERE clause.
func (r *activityRepository) countActivities(ctx context.Context, conditions squirrel.And) (int, error) {
	builder := psql.Select("COUNT(*)").From("activities")
	if len(conditions) > 0 {
		builder = builder.Where(conditions)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build activities count query: %w", err)
	}

	var totalCount int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&totalCount); err != nil {
		slog.Error("Failed to count activities", "error", err)
		return 0, fmt.Errorf("failed to count activities: %w", err)
	}

	return totalCount, nil
}

// queryActivities runs the paginated page query for List using the same shared
// filter conditions as the count query, so they can never diverge. The returned
// slice is always non-nil.
func (r *activityRepository) queryActivities(
	ctx context.Context, conditions squirrel.And, filters repositories.ActivityFilters,
) ([]models.Activity, error) {
	builder := psql.
		Select(
			"id", "user_id", "activity_type", "entity_type", "entity_id", "session_id",
			"description", "metadata", "source_ip", "user_agent", "created_at",
		).
		From("activities")
	if len(conditions) > 0 {
		builder = builder.Where(conditions)
	}

	limit, offset := activityListPaging(filters)
	query, args, err := builder.
		OrderBy("created_at DESC").
		Limit(limit).
		Offset(offset).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build activities list query: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		slog.Error("Failed to query activities", "error", err)
		return nil, fmt.Errorf("failed to query activities: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close rows", "error", closeErr)
		}
	}()

	return scanActivityListRows(rows), nil
}

// scanActivityListRows scans the 11-column List projection into a non-nil slice.
// A row that fails to scan is logged and skipped (matching the prior behaviour);
// metadata that fails to parse leaves Metadata nil rather than erroring.
func scanActivityListRows(rows *sql.Rows) []models.Activity {
	activities := make([]models.Activity, 0)
	for rows.Next() {
		var activity models.Activity
		var metadataJSON string
		err := rows.Scan(
			&activity.ID,
			&activity.UserID,
			&activity.ActivityType,
			&activity.EntityType,
			&activity.EntityID,
			&activity.SessionID,
			&activity.Description,
			&metadataJSON,
			&activity.SourceIP,
			&activity.UserAgent,
			&activity.CreatedAt,
		)
		if err != nil {
			slog.Error("Failed to scan activity row", "error", err)
			continue
		}

		if metadataJSON != "" {
			var metadata map[string]interface{}
			if err := json.Unmarshal([]byte(metadataJSON), &metadata); err == nil {
				activity.Metadata = metadata
			}
		}

		activities = append(activities, activity)
	}

	return activities
}

// GetStats retrieves activity statistics
//
//nolint:gocognit,gocyclo,funlen // Repository code with necessary complexity
func (r *activityRepository) GetStats(ctx context.Context, userID string) (*models.ActivityStatsResponse, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	stats := &models.ActivityStatsResponse{}

	// Get total activities
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM activities WHERE user_id = $1", userID).Scan(&stats.TotalActivities)
	if err != nil {
		slog.Error("Failed to get total activities count", "error", err)
		return nil, fmt.Errorf("failed to get activity stats: %w", err)
	}

	// Get activities today
	err = r.db.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM activities WHERE user_id = $1 AND DATE(created_at) = CURRENT_DATE",
		userID,
	).Scan(&stats.ActivitiesToday)
	if err != nil {
		slog.Error("Failed to get today's activities count", "error", err)
		stats.ActivitiesToday = 0
	}

	// Get activities this week
	err = r.db.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM activities WHERE user_id = $1 AND created_at >= DATE_TRUNC('week', CURRENT_DATE)",
		userID,
	).Scan(&stats.ActivitiesThisWeek)
	if err != nil {
		slog.Error("Failed to get this week's activities count", "error", err)
		stats.ActivitiesThisWeek = 0
	}

	// Get top activity types (last 30 days)
	topActivityTypesQuery := `
		SELECT activity_type, COUNT(*) as count
		FROM activities
		WHERE user_id = $1 AND created_at >= CURRENT_DATE - INTERVAL '30 days'
		GROUP BY activity_type
		ORDER BY count DESC
		LIMIT 5`

	rows, err := r.db.QueryContext(ctx, topActivityTypesQuery, userID)
	if err != nil {
		slog.Error("Failed to query top activity types", "error", err)
		stats.TopActivityTypes = []models.ActivityTypeCount{}
	} else {
		defer func() {
			if closeErr := rows.Close(); closeErr != nil {
				slog.Error("Failed to close rows", "error", closeErr)
			}
		}()
		var topActivityTypes []models.ActivityTypeCount
		for rows.Next() {
			var item models.ActivityTypeCount
			if scanErr := rows.Scan(&item.ActivityType, &item.Count); scanErr == nil {
				topActivityTypes = append(topActivityTypes, item)
			}
		}
		if topActivityTypes == nil {
			topActivityTypes = []models.ActivityTypeCount{}
		}
		stats.TopActivityTypes = topActivityTypes
	}

	// Get top entity types (last 30 days)
	topEntityTypesQuery := `
		SELECT entity_type, COUNT(*) as count
		FROM activities
		WHERE user_id = $1 AND created_at >= CURRENT_DATE - INTERVAL '30 days'
		GROUP BY entity_type
		ORDER BY count DESC
		LIMIT 5`

	rows, err = r.db.QueryContext(ctx, topEntityTypesQuery, userID)
	if err != nil {
		slog.Error("Failed to query top entity types", "error", err)
		stats.TopEntityTypes = []models.EntityTypeCount{}
	} else {
		defer func() {
			if closeErr := rows.Close(); closeErr != nil {
				slog.Error("Failed to close rows", "error", closeErr)
			}
		}()
		var topEntityTypes []models.EntityTypeCount
		for rows.Next() {
			var item models.EntityTypeCount
			if scanErr := rows.Scan(&item.EntityType, &item.Count); scanErr == nil {
				topEntityTypes = append(topEntityTypes, item)
			}
		}
		if topEntityTypes == nil {
			topEntityTypes = []models.EntityTypeCount{}
		}
		stats.TopEntityTypes = topEntityTypes
	}

	// Get recent activities (last 10)
	recentActivitiesQuery := `
		SELECT id, user_id, activity_type, entity_type, entity_id, session_id,
		       description, metadata, source_ip, user_agent, created_at
		FROM activities
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 10`

	rows, err = r.db.QueryContext(ctx, recentActivitiesQuery, userID)
	if err != nil {
		slog.Error("Failed to query recent activities", "error", err)
		stats.RecentActivities = []models.Activity{}
	} else {
		defer func() {
			if closeErr := rows.Close(); closeErr != nil {
				slog.Error("Failed to close rows", "error", closeErr)
			}
		}()
		var recentActivities []models.Activity
		for rows.Next() {
			var activity models.Activity
			var metadataJSON string
			scanErr := rows.Scan(
				&activity.ID,
				&activity.UserID,
				&activity.ActivityType,
				&activity.EntityType,
				&activity.EntityID,
				&activity.SessionID,
				&activity.Description,
				&metadataJSON,
				&activity.SourceIP,
				&activity.UserAgent,
				&activity.CreatedAt,
			)
			if scanErr == nil {
				// Parse metadata JSON
				if metadataJSON != "" {
					var metadata map[string]interface{}
					if jsonErr := json.Unmarshal([]byte(metadataJSON), &metadata); jsonErr == nil {
						activity.Metadata = metadata
					}
				}
				recentActivities = append(recentActivities, activity)
			}
		}
		if recentActivities == nil {
			recentActivities = []models.Activity{}
		}
		stats.RecentActivities = recentActivities
	}

	// Get activities by date (last 7 days)
	activitiesByDateQuery := `
		SELECT DATE(created_at) as date, COUNT(*) as count
		FROM activities
		WHERE user_id = $1 AND created_at >= CURRENT_DATE - INTERVAL '7 days'
		GROUP BY DATE(created_at)
		ORDER BY date DESC`

	rows, err = r.db.QueryContext(ctx, activitiesByDateQuery, userID)
	if err != nil {
		slog.Error("Failed to query activities by date", "error", err)
		stats.ActivitiesByDateWeek = []models.ActivityCountByDate{}
	} else {
		defer func() {
			if closeErr := rows.Close(); closeErr != nil {
				slog.Error("Failed to close rows", "error", closeErr)
			}
		}()
		var activitiesByDate []models.ActivityCountByDate
		for rows.Next() {
			var item models.ActivityCountByDate
			if err := rows.Scan(&item.Date, &item.Count); err == nil {
				activitiesByDate = append(activitiesByDate, item)
			}
		}
		if activitiesByDate == nil {
			activitiesByDate = []models.ActivityCountByDate{}
		}
		stats.ActivitiesByDateWeek = activitiesByDate
	}

	return stats, nil
}

// Delete deletes an activity (admin only)
func (r *activityRepository) Delete(ctx context.Context, activityID string) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	query := "DELETE FROM activities WHERE id = $1"
	result, err := r.db.ExecContext(ctx, query, activityID)
	if err != nil {
		slog.Error("Failed to delete activity", "error", err)
		return fmt.Errorf("failed to delete activity: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrActivityNotFound
	}

	return nil
}

// activityRetentionBatchSize is the number of rows deleted per batch in DeleteOlderThan.
// Keeps each DELETE transaction short to reduce WAL pressure and lock contention.
const activityRetentionBatchSize = 10000

// activityRetentionAdvisoryLockID is the Postgres advisory lock key for the retention job.
// Prevents concurrent runs (e.g. Cloud Scheduler at-least-once delivery on multi-instance Cloud Run).
const activityRetentionAdvisoryLockID = 7391028 // arbitrary stable constant for activities retention

// DeleteOlderThan deletes activity rows with created_at before the given cutoff time in batches.
// Uses a Postgres advisory lock to prevent concurrent execution across Cloud Run instances.
// Returns the total number of rows deleted.
func (r *activityRepository) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	if r.db == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	// Acquire advisory lock; bail early if another instance already holds it.
	var locked bool
	if err := r.db.QueryRowContext(ctx,
		`SELECT pg_try_advisory_lock($1)`, activityRetentionAdvisoryLockID,
	).Scan(&locked); err != nil {
		return 0, fmt.Errorf("delete old activities: acquire advisory lock: %w", err)
	}
	if !locked {
		return 0, nil // Another instance is already running the job — skip silently.
	}
	defer func() {
		// Best-effort unlock; the lock auto-releases when the connection closes anyway.
		if _, err := r.db.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, activityRetentionAdvisoryLockID); err != nil {
			return
		}
	}()

	var total int64
	for {
		res, err := r.db.ExecContext(ctx,
			`DELETE FROM activities WHERE id IN (SELECT id FROM activities WHERE created_at < $1 LIMIT $2)`,
			before, activityRetentionBatchSize,
		)
		if err != nil {
			return total, fmt.Errorf("delete old activities: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("delete old activities: get rows affected: %w", err)
		}
		total += n
		if n < activityRetentionBatchSize {
			break // Last batch — no more rows to delete.
		}
	}

	return total, nil
}

// buildActivityListConditions builds the shared, all-optional WHERE conditions
// for the activity List count and page queries, so the two can never diverge.
// Every filter is optional; when none are set the returned slice is empty and
// callers must omit the WHERE clause entirely (an empty squirrel.And{} would
// render a spurious "(1=1)"). Predicate set and trigger conditions match the
// prior hand-built builder exactly, including the nil+non-empty guard on Search.
func buildActivityListConditions(filters repositories.ActivityFilters) squirrel.And {
	conditions := squirrel.And{}

	if filters.UserID != nil {
		conditions = append(conditions, squirrel.Eq{"user_id": *filters.UserID})
	}
	if filters.ActivityType != nil {
		conditions = append(conditions, squirrel.Eq{"activity_type": *filters.ActivityType})
	}
	if filters.EntityType != nil {
		conditions = append(conditions, squirrel.Eq{"entity_type": *filters.EntityType})
	}
	if filters.EntityID != nil {
		conditions = append(conditions, squirrel.Eq{"entity_id": *filters.EntityID})
	}
	if filters.SessionID != nil {
		conditions = append(conditions, squirrel.Eq{"session_id": *filters.SessionID})
	}
	if filters.Search != nil && *filters.Search != "" {
		pattern := "%" + *filters.Search + "%"
		conditions = append(conditions, squirrel.Or{
			squirrel.ILike{"description": pattern},
			squirrel.ILike{"activity_type": pattern},
		})
	}
	if filters.DateFrom != nil {
		conditions = append(conditions, squirrel.GtOrEq{"created_at": *filters.DateFrom})
	}
	if filters.DateTo != nil {
		conditions = append(conditions, squirrel.LtOrEq{"created_at": *filters.DateTo})
	}

	return conditions
}

// activityListPaging resolves the LIMIT/OFFSET for the List page query.
//
// Pass-through contract (NOT defaulting): this repository historically binds the
// caller-supplied Limit and Offset directly. Negative values are clamped to 0 —
// Postgres rejects a negative LIMIT/OFFSET, so a caller passing one already had
// a failing query; the clamp only changes the (invalid) negative case and lets
// the unsigned conversion be provably non-wrapping for gosec (G115).
func activityListPaging(filters repositories.ActivityFilters) (limit, offset uint64) {
	if filters.Limit > 0 {
		limit = uint64(filters.Limit)
	}
	if filters.Offset > 0 {
		offset = uint64(filters.Offset)
	}
	return limit, offset
}
