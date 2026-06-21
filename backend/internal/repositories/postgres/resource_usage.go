package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// ResourceUsageRepository handles database operations for resource usage tracking
type ResourceUsageRepository struct {
	db     *sql.DB
	logger *logrus.Logger
}

// NewResourceUsageRepository creates a new resource usage repository
func NewResourceUsageRepository(db *sql.DB, logger *logrus.Logger) *ResourceUsageRepository {
	return &ResourceUsageRepository{
		db:     db,
		logger: logger,
	}
}

// IncrementUsage increments the usage count for a resource type within a period
func (r *ResourceUsageRepository) IncrementUsage(
	ctx context.Context, userID, resourceType string, periodStart, periodEnd time.Time,
) error {
	query := `
		INSERT INTO resource_usage (user_id, resource_type, count, period_start, period_end)
		VALUES ($1, $2, 1, $3, $4)
		ON CONFLICT (user_id, resource_type, period_start, period_end)
		DO UPDATE SET count = resource_usage.count + 1, updated_at = CURRENT_TIMESTAMP
	`

	_, err := r.db.ExecContext(ctx, query, userID, resourceType, periodStart, periodEnd)
	if err != nil {
		r.logger.WithError(err).WithFields(logrus.Fields{
			"user_id":       userID,
			"resource_type": resourceType,
		}).Error("Failed to increment resource usage")
		return fmt.Errorf("failed to increment resource usage: %w", err)
	}

	return nil
}

// DecrementUsage decrements the usage count for a resource type within a period
func (r *ResourceUsageRepository) DecrementUsage(
	ctx context.Context, userID, resourceType string, periodStart, periodEnd time.Time,
) error {
	query := `
		UPDATE resource_usage
		SET count = GREATEST(count - 1, 0), updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1
		  AND resource_type = $2
		  AND period_start = $3
		  AND period_end = $4
	`

	_, err := r.db.ExecContext(ctx, query, userID, resourceType, periodStart, periodEnd)
	if err != nil {
		r.logger.WithError(err).WithFields(logrus.Fields{
			"user_id":       userID,
			"resource_type": resourceType,
		}).Error("Failed to decrement resource usage")
		return fmt.Errorf("failed to decrement resource usage: %w", err)
	}

	return nil
}

// GetUsageCount gets the current usage count for a resource type within a period.
//
// When no usage row exists for the period it returns (0, nil) — a missing row
// means zero usage, never an error.
func (r *ResourceUsageRepository) GetUsageCount(
	ctx context.Context, userID, resourceType string, periodStart, periodEnd time.Time,
) (int, error) {
	query := `
		SELECT count FROM resource_usage
		WHERE user_id = $1
		  AND resource_type = $2
		  AND period_start = $3
		  AND period_end = $4
	`

	var count int
	err := r.db.QueryRowContext(ctx, query, userID, resourceType, periodStart, periodEnd).Scan(&count)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// If no record exists, usage is 0
			return 0, nil
		}
		r.logger.WithError(err).WithFields(logrus.Fields{
			"user_id":       userID,
			"resource_type": resourceType,
		}).Error("Failed to get resource usage count")
		return 0, fmt.Errorf("failed to get resource usage count: %w", err)
	}

	return count, nil
}

// GetResourceCounts gets counts of all resources for a user within a period
func (r *ResourceUsageRepository) GetResourceCounts(
	ctx context.Context, userID string, periodStart, periodEnd time.Time,
) (map[string]int, error) {
	query := `
		SELECT resource_type, count FROM resource_usage
		WHERE user_id = $1
		  AND period_start = $2
		  AND period_end = $3
	`

	rows, err := r.db.QueryContext(ctx, query, userID, periodStart, periodEnd)
	if err != nil {
		r.logger.WithError(err).WithFields(logrus.Fields{
			"user_id": userID,
		}).Error("Failed to get resource counts")
		return nil, fmt.Errorf("failed to get resource counts: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			r.logger.WithError(closeErr).Error("Failed to close rows")
		}
	}()

	counts := make(map[string]int)
	for rows.Next() {
		var resourceType string
		var count int
		if err := rows.Scan(&resourceType, &count); err != nil {
			r.logger.WithError(err).Error("Failed to scan resource usage row")
			return nil, fmt.Errorf("failed to scan resource usage row: %w", err)
		}
		counts[resourceType] = count
	}

	if err := rows.Err(); err != nil {
		r.logger.WithError(err).Error("Error iterating resource usage rows")
		return nil, fmt.Errorf("error iterating resource usage rows: %w", err)
	}

	return counts, nil
}
