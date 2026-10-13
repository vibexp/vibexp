package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// InstanceAuthPolicyRepository implements
// repositories.InstanceAuthPolicyRepository for PostgreSQL.
//
// The table's primary key is `id boolean CHECK (id)`, so it holds at most one
// row and statements carry no WHERE clause and no tenancy or role predicate.
type InstanceAuthPolicyRepository struct {
	db *database.DB
}

// NewInstanceAuthPolicyRepository creates a new InstanceAuthPolicyRepository.
func NewInstanceAuthPolicyRepository(db *database.DB) repositories.InstanceAuthPolicyRepository {
	return &InstanceAuthPolicyRepository{db: db}
}

// instanceAuthPolicySubject names the policy in errors and logs.
const instanceAuthPolicySubject = "instance auth policy"

const instanceAuthPolicySelect = `SELECT password_login_enabled, registration_mode,
		created_at, updated_at, updated_by, version
	FROM instance_auth_policy`

// instanceAuthPolicyUpsert creates or replaces the row.
const instanceAuthPolicyUpsert = `INSERT INTO instance_auth_policy
		(password_login_enabled, registration_mode, updated_by)
	VALUES ($1, $2, $3)
	ON CONFLICT (id)
	DO UPDATE SET
		password_login_enabled = EXCLUDED.password_login_enabled,
		registration_mode = EXCLUDED.registration_mode,
		updated_by = EXCLUDED.updated_by,
		updated_at = CURRENT_TIMESTAMP,
		version = instance_auth_policy.version + 1
	RETURNING created_at, updated_at, version`

func scanInstanceAuthPolicy(row rowScanner) (*models.InstanceAuthPolicy, error) {
	var p models.InstanceAuthPolicy
	if err := row.Scan(
		&p.PasswordLoginEnabled, &p.RegistrationMode,
		&p.CreatedAt, &p.UpdatedAt, &p.UpdatedBy, &p.Version,
	); err != nil {
		return nil, err
	}
	return &p, nil
}

// Get returns the stored policy, or ErrInstanceAuthPolicyNotFound.
func (r *InstanceAuthPolicyRepository) Get(ctx context.Context) (*models.InstanceAuthPolicy, error) {
	p, err := scanInstanceAuthPolicy(r.db.QueryRowContext(ctx, instanceAuthPolicySelect))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get instance auth policy: %w", err),
			repositories.ErrInstanceAuthPolicyNotFound)
	}
	return p, nil
}

// UpsertAudited creates or replaces the policy; see the interface. It runs
// under the shared auth settings version lock (runAuthSettingsTx), so the
// write, its audit entry and the version bump land together or not at all, and
// the compare-and-set on the row's own version cannot race another writer.
func (r *InstanceAuthPolicyRepository) UpsertAudited(
	ctx context.Context, p *models.InstanceAuthPolicy, actorUserID *string, expectedVersion *int64,
) error {
	p.UpdatedBy = actorUserID
	return runAuthSettingsTx(ctx, r.db, instanceAuthPolicySubject, "upsert",
		func(tx *sql.Tx) (*models.InstanceAuthPolicy, error) {
			return scanInstanceAuthPolicy(tx.QueryRowContext(ctx, instanceAuthPolicySelect))
		},
		func(tx *sql.Tx, before *models.InstanceAuthPolicy, _ int64) (bool, error) {
			var storedVersion *int64
			if before != nil {
				storedVersion = &before.Version
			}
			if err := checkSingletonVersion(expectedVersion, storedVersion); err != nil {
				return false, err
			}
			err := tx.QueryRowContext(ctx, instanceAuthPolicyUpsert,
				p.PasswordLoginEnabled, p.RegistrationMode, p.UpdatedBy,
			).Scan(&p.CreatedAt, &p.UpdatedAt, &p.Version)
			if err != nil {
				return false, fmt.Errorf("failed to write instance auth policy: %w", err)
			}
			return true, appendInstanceAuthPolicyAudit(ctx, tx, actorUserID, before, p)
		})
}

// appendInstanceAuthPolicyAudit records one policy change. The policy holds no
// credential, so the snapshot is the row itself.
func appendInstanceAuthPolicyAudit(
	ctx context.Context, tx *sql.Tx, actorUserID *string, before, after *models.InstanceAuthPolicy,
) error {
	beforeDoc, err := optionalAuditSnapshot(before)
	if err != nil {
		return fmt.Errorf("failed to snapshot the instance auth policy: %w", err)
	}
	afterDoc, err := optionalAuditSnapshot(after)
	if err != nil {
		return fmt.Errorf("failed to snapshot the instance auth policy: %w", err)
	}
	return appendAuthSettingsAudit(ctx, tx, models.InstanceSettingAuthPolicy,
		models.InstanceSettingsAuditActionUpsert, actorUserID, beforeDoc, afterDoc)
}
