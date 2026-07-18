package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

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

// adminUserListQuery lists users (newest first) with a team count via a LEFT
// JOIN aggregate over team_members. No role predicate (decision D3): the join is
// on user_id only.
const adminUserListQuery = `
SELECT u.id, u.email, u.name, u.idp_provider, u.created_at, COUNT(tm.team_id) AS team_count
FROM users u
LEFT JOIN team_members tm ON tm.user_id = u.id
GROUP BY u.id, u.email, u.name, u.idp_provider, u.created_at
ORDER BY u.created_at DESC, u.id
LIMIT $1 OFFSET $2
`

// ListUsers returns a page of users with team counts plus the total user count.
func (r *AdminRepository) ListUsers(
	ctx context.Context, page, limit int,
) ([]models.AdminUserListItem, int, error) {
	var totalCount int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	offset := (page - 1) * limit
	rows, err := r.db.QueryContext(ctx, adminUserListQuery, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close admin user rows", "error", closeErr)
		}
	}()

	users := make([]models.AdminUserListItem, 0)
	for rows.Next() {
		var u models.AdminUserListItem
		if scanErr := rows.Scan(&u.ID, &u.Email, &u.Name, &u.IDPProvider, &u.CreatedAt, &u.TeamCount); scanErr != nil {
			return nil, 0, fmt.Errorf("failed to scan admin user: %w", scanErr)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate admin users: %w", err)
	}
	return users, totalCount, nil
}

// adminUserMembershipsQuery returns the teams a user belongs to with the user's
// role as a plain column (no role predicate — decision D3).
const adminUserMembershipsQuery = `
SELECT t.id, t.name, tm.role
FROM team_members tm
JOIN teams t ON t.id = tm.team_id
WHERE tm.user_id = $1
ORDER BY t.name, t.id
`

// GetUserDetail returns one user with their team memberships, or (nil, nil) when
// no user with that id exists.
func (r *AdminRepository) GetUserDetail(
	ctx context.Context, id string,
) (*models.AdminUserDetail, error) {
	var detail models.AdminUserDetail
	err := r.db.QueryRowContext(ctx,
		"SELECT id, email, name, idp_provider, created_at FROM users WHERE id = $1", id,
	).Scan(&detail.ID, &detail.Email, &detail.Name, &detail.IDPProvider, &detail.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query admin user: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, adminUserMembershipsQuery, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query user memberships: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Error("Failed to close membership rows", "error", closeErr)
		}
	}()

	detail.Memberships = make([]models.AdminTeamMembership, 0)
	for rows.Next() {
		var m models.AdminTeamMembership
		if scanErr := rows.Scan(&m.TeamID, &m.TeamName, &m.Role); scanErr != nil {
			return nil, fmt.Errorf("failed to scan membership: %w", scanErr)
		}
		detail.Memberships = append(detail.Memberships, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate memberships: %w", err)
	}
	return &detail, nil
}
