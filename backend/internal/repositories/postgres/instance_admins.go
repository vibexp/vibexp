package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// InstanceAdminRepository implements repositories.InstanceAdminRepository for
// PostgreSQL. The scope is the instance, so statements carry no tenancy or role
// predicate.
type InstanceAdminRepository struct {
	db *database.DB
}

// NewInstanceAdminRepository creates a new InstanceAdminRepository.
func NewInstanceAdminRepository(db *database.DB) repositories.InstanceAdminRepository {
	return &InstanceAdminRepository{db: db}
}

// instanceAdminsSubject names the grants in errors and logs.
const instanceAdminsSubject = "instance admin grant"

const instanceAdminList = `SELECT user_id, granted_by, created_at
	FROM instance_admins ORDER BY created_at, user_id`

const instanceAdminByUser = `SELECT user_id, granted_by, created_at
	FROM instance_admins WHERE user_id = $1`

const instanceAdminIsGranted = `SELECT EXISTS (SELECT 1 FROM instance_admins WHERE user_id = $1)`

const instanceAdminInsert = `INSERT INTO instance_admins (user_id, granted_by)
	VALUES ($1, $2)
	RETURNING created_at`

const instanceAdminDelete = `DELETE FROM instance_admins WHERE user_id = $1`

// instanceAdminsWriteLock serializes audited grant writes; see
// runAuditedSingletonTx for why it is a table lock (a row lock on a grant that
// does not exist yet locks nothing).
const instanceAdminsWriteLock = `LOCK TABLE instance_admins IN SHARE ROW EXCLUSIVE MODE`

func scanInstanceAdminGrant(row rowScanner) (*models.InstanceAdminGrant, error) {
	var g models.InstanceAdminGrant
	if err := row.Scan(&g.UserID, &g.GrantedBy, &g.CreatedAt); err != nil {
		return nil, err
	}
	return &g, nil
}

// List returns every grant, oldest first.
func (r *InstanceAdminRepository) List(ctx context.Context) ([]*models.InstanceAdminGrant, error) {
	return queryAdminRows(ctx, r.db, "instance admin grants",
		func(rows *sql.Rows) (*models.InstanceAdminGrant, error) { return scanInstanceAdminGrant(rows) },
		instanceAdminList)
}

// IsGranted reports whether userID holds a DB grant.
func (r *InstanceAdminRepository) IsGranted(ctx context.Context, userID string) (bool, error) {
	var granted bool
	if err := r.db.QueryRowContext(ctx, instanceAdminIsGranted, userID).Scan(&granted); err != nil {
		return false, fmt.Errorf("failed to check instance admin grant: %w", err)
	}
	return granted, nil
}

// Grant makes userID an instance admin; see the interface.
func (r *InstanceAdminRepository) Grant(ctx context.Context, userID string, grantedBy *string) (bool, error) {
	var granted bool
	err := r.inTx(ctx, "grant", userID, func(tx *sql.Tx, before *models.InstanceAdminGrant) (bool, error) {
		if before != nil {
			return false, nil
		}
		after := &models.InstanceAdminGrant{UserID: userID, GrantedBy: grantedBy}
		if err := tx.QueryRowContext(ctx, instanceAdminInsert, userID, grantedBy).Scan(&after.CreatedAt); err != nil {
			return false, fmt.Errorf("failed to grant instance admin: %w", err)
		}
		granted = true
		return true, appendInstanceAdminAudit(ctx, tx, models.InstanceSettingsAuditActionUpsert,
			grantedBy, nil, after)
	})
	if err != nil {
		return false, err
	}
	return granted, nil
}

// Revoke removes userID's grant; see the interface.
func (r *InstanceAdminRepository) Revoke(ctx context.Context, userID string, actorUserID *string) error {
	return r.inTx(ctx, "revoke", userID, func(tx *sql.Tx, before *models.InstanceAdminGrant) (bool, error) {
		if before == nil {
			return false, repositories.ErrInstanceAdminNotFound
		}
		if _, err := tx.ExecContext(ctx, instanceAdminDelete, userID); err != nil {
			return false, fmt.Errorf("failed to revoke instance admin: %w", err)
		}
		return true, appendInstanceAdminAudit(ctx, tx, models.InstanceSettingsAuditActionDelete,
			actorUserID, before, nil)
	})
}

// inTx runs one audited grant write under the table's write lock with userID's
// current grant (nil when none). A grant naming a user that does not exist
// (the grantee or the grantor) is ErrUserNotFound.
func (r *InstanceAdminRepository) inTx(
	ctx context.Context, op, userID string,
	change func(tx *sql.Tx, before *models.InstanceAdminGrant) (bool, error),
) error {
	err := runAuditedSingletonTx(ctx, r.db, instanceAdminsSubject, op, instanceAdminsWriteLock,
		func(tx *sql.Tx) (*models.InstanceAdminGrant, error) {
			return scanInstanceAdminGrant(tx.QueryRowContext(ctx, instanceAdminByUser, userID))
		}, change)
	if isFKViolation(err) {
		return fmt.Errorf("%w: %w", repositories.ErrUserNotFound, err)
	}
	return err
}

// appendInstanceAdminAudit records one grant or revoke.
func appendInstanceAdminAudit(
	ctx context.Context, tx *sql.Tx, action string, actorUserID *string,
	before, after *models.InstanceAdminGrant,
) error {
	beforeDoc, err := optionalAuditSnapshot(before)
	if err != nil {
		return fmt.Errorf("failed to snapshot the instance admin grant: %w", err)
	}
	afterDoc, err := optionalAuditSnapshot(after)
	if err != nil {
		return fmt.Errorf("failed to snapshot the instance admin grant: %w", err)
	}
	return appendAuthSettingsAudit(ctx, tx, models.InstanceSettingInstanceAdmins, action, actorUserID,
		beforeDoc, afterDoc)
}
