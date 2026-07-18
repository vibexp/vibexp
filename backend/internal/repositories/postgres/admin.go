package postgres

import (
	"context"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// AdminRepository implements the repositories.AdminRepository interface for
// PostgreSQL.
type AdminRepository struct {
	db *database.DB
}

// NewAdminRepository creates a new AdminRepository.
func NewAdminRepository(db *database.DB) repositories.AdminRepository {
	return &AdminRepository{
		db: db,
	}
}

// instanceCountsQuery gathers every instance-wide total in a single round-trip
// via correlated scalar subqueries. Table names are hardcoded (not user input),
// so there is no injection surface.
const instanceCountsQuery = `
SELECT
	(SELECT COUNT(*) FROM users)     AS users,
	(SELECT COUNT(*) FROM teams)     AS teams,
	(SELECT COUNT(*) FROM prompts)   AS prompts,
	(SELECT COUNT(*) FROM artifacts) AS artifacts,
	(SELECT COUNT(*) FROM memories)  AS memories
`

// GetInstanceCounts returns unscoped totals for the top-level entities.
func (r *AdminRepository) GetInstanceCounts(ctx context.Context) (models.InstanceCounts, error) {
	var counts models.InstanceCounts
	err := r.db.QueryRowContext(ctx, instanceCountsQuery).Scan(
		&counts.Users,
		&counts.Teams,
		&counts.Prompts,
		&counts.Artifacts,
		&counts.Memories,
	)
	if err != nil {
		return models.InstanceCounts{}, fmt.Errorf("failed to query instance counts: %w", err)
	}
	return counts, nil
}
