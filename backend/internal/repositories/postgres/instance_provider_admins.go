package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// InstanceProviderAdminRepository implements
// repositories.InstanceProviderAdminRepository for PostgreSQL. The scope is the
// instance, so statements carry no tenancy or role predicate.
type InstanceProviderAdminRepository struct {
	db *database.DB
}

// NewInstanceProviderAdminRepository creates a new InstanceProviderAdminRepository.
func NewInstanceProviderAdminRepository(db *database.DB) repositories.InstanceProviderAdminRepository {
	return &InstanceProviderAdminRepository{db: db}
}

// instanceProviderAdminUpsert reports, through xmax, whether the row was
// inserted (xmax = 0) rather than updated by the conflict clause.
const instanceProviderAdminUpsert = `INSERT INTO instance_provider_admins (user_id, provider_id, matched_value)
	VALUES ($1, $2, $3)
	ON CONFLICT (user_id, provider_id) DO UPDATE
		SET matched_value = EXCLUDED.matched_value, evaluated_at = CURRENT_TIMESTAMP
	RETURNING (xmax = 0)`

const instanceProviderAdminDelete = `DELETE FROM instance_provider_admins
	WHERE user_id = $1 AND provider_id = $2`

const instanceProviderAdminDeleteByProvider = `DELETE FROM instance_provider_admins
	WHERE provider_id = $1 RETURNING user_id`

// liveProviderAdmins joins a row to its provider only while that provider is
// enabled and still has a role mapping: a row left behind by a cleared mapping
// (a sign-in that was in flight while it was cleared) grants nothing.
const liveProviderAdmins = `FROM instance_provider_admins a
	JOIN instance_auth_providers p
		ON p.id = a.provider_id AND p.enabled AND p.roles_claim IS NOT NULL`

const instanceProviderAdminIsAdmin = `SELECT EXISTS (SELECT 1 ` + liveProviderAdmins + ` WHERE a.user_id = $1)`

const instanceProviderAdminSelect = `SELECT a.user_id, a.provider_id, p.slug, a.matched_value, a.evaluated_at ` +
	liveProviderAdmins

const instanceProviderAdminList = instanceProviderAdminSelect +
	` ORDER BY p.sort_order, p.slug, a.evaluated_at, a.user_id`

const instanceProviderAdminListByUser = instanceProviderAdminSelect +
	` WHERE a.user_id = $1 ORDER BY p.sort_order, p.slug`

func scanInstanceProviderAdmin(rows *sql.Rows) (*models.InstanceProviderAdmin, error) {
	var a models.InstanceProviderAdmin
	if err := rows.Scan(&a.UserID, &a.ProviderID, &a.ProviderSlug, &a.MatchedValue, &a.EvaluatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

// Upsert records userID as an admin of providerID; see the interface.
func (r *InstanceProviderAdminRepository) Upsert(
	ctx context.Context, userID, providerID, matchedValue string,
) (bool, error) {
	var inserted bool
	err := r.db.QueryRowContext(ctx, instanceProviderAdminUpsert, userID, providerID, matchedValue).Scan(&inserted)
	if err != nil {
		return false, fmt.Errorf("failed to record provider-sourced instance admin: %w", err)
	}
	return inserted, nil
}

// Delete removes userID's row for providerID; see the interface.
func (r *InstanceProviderAdminRepository) Delete(ctx context.Context, userID, providerID string) (bool, error) {
	res, err := r.db.ExecContext(ctx, instanceProviderAdminDelete, userID, providerID)
	if err != nil {
		return false, fmt.Errorf("failed to remove provider-sourced instance admin: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to remove provider-sourced instance admin: %w", err)
	}
	return n > 0, nil
}

// DeleteByProvider removes every row of providerID; see the interface.
func (r *InstanceProviderAdminRepository) DeleteByProvider(ctx context.Context, providerID string) ([]string, error) {
	return queryAdminRows(ctx, r.db, "provider-sourced instance admins",
		func(rows *sql.Rows) (string, error) {
			var userID string
			err := rows.Scan(&userID)
			return userID, err
		},
		instanceProviderAdminDeleteByProvider, providerID)
}

// IsProviderAdmin reports whether userID holds a row of an enabled, mapped
// provider.
func (r *InstanceProviderAdminRepository) IsProviderAdmin(ctx context.Context, userID string) (bool, error) {
	var isAdmin bool
	if err := r.db.QueryRowContext(ctx, instanceProviderAdminIsAdmin, userID).Scan(&isAdmin); err != nil {
		return false, fmt.Errorf("failed to check provider-sourced instance admin: %w", err)
	}
	return isAdmin, nil
}

// List returns the rows of the enabled, mapped providers.
func (r *InstanceProviderAdminRepository) List(ctx context.Context) ([]*models.InstanceProviderAdmin, error) {
	return queryAdminRows(ctx, r.db, "provider-sourced instance admins", scanInstanceProviderAdmin,
		instanceProviderAdminList)
}

// ListByUser returns userID's rows of the enabled, mapped providers.
func (r *InstanceProviderAdminRepository) ListByUser(
	ctx context.Context, userID string,
) ([]*models.InstanceProviderAdmin, error) {
	return queryAdminRows(ctx, r.db, "provider-sourced instance admins", scanInstanceProviderAdmin,
		instanceProviderAdminListByUser, userID)
}
