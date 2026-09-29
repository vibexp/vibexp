package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// InstanceAuthSettingsVersionRepository implements
// repositories.InstanceAuthSettingsVersionRepository for PostgreSQL.
//
// The table is a seeded singleton (migration 023), so statements carry no WHERE
// clause and no tenancy or role predicate.
type InstanceAuthSettingsVersionRepository struct {
	db *database.DB
}

// NewInstanceAuthSettingsVersionRepository creates a new
// InstanceAuthSettingsVersionRepository.
func NewInstanceAuthSettingsVersionRepository(db *database.DB) repositories.InstanceAuthSettingsVersionRepository {
	return &InstanceAuthSettingsVersionRepository{db: db}
}

// instanceAuthSettingsVersionSelect reads the shared auth settings version.
const instanceAuthSettingsVersionSelect = `SELECT version FROM instance_auth_settings_version`

// instanceAuthSettingsVersionLock row-locks the shared version for the rest of
// the transaction. Every provider and allowlist write takes it first, so they
// are serialized against each other: a compare-and-set on the version cannot
// race another writer's bump, and two concurrent creates cannot both pass the
// check. The row is seeded by the migration, so there is always one to lock;
// plain reads are never blocked.
const instanceAuthSettingsVersionLock = `SELECT version FROM instance_auth_settings_version FOR UPDATE`

// instanceAuthSettingsVersionBump increments the shared version.
const instanceAuthSettingsVersionBump = `UPDATE instance_auth_settings_version
	SET version = version + 1, updated_at = CURRENT_TIMESTAMP
	RETURNING version`

// Get returns the current shared auth settings version.
func (r *InstanceAuthSettingsVersionRepository) Get(ctx context.Context) (int64, error) {
	var version int64
	if err := r.db.QueryRowContext(ctx, instanceAuthSettingsVersionSelect).Scan(&version); err != nil {
		return 0, fmt.Errorf("failed to get instance auth settings version: %w", err)
	}
	return version, nil
}

// bumpAuthSettingsVersion increments the shared version inside tx, so the bump
// commits or rolls back with the write it records.
func bumpAuthSettingsVersion(ctx context.Context, tx *sql.Tx) (int64, error) {
	var version int64
	if err := tx.QueryRowContext(ctx, instanceAuthSettingsVersionBump).Scan(&version); err != nil {
		return 0, fmt.Errorf("failed to bump instance auth settings version: %w", err)
	}
	return version, nil
}

// runAuthSettingsTx runs one audited provider or allowlist write: under the
// shared version lock (see instanceAuthSettingsVersionLock) it reads the
// current row with read (nil when none), hands change the row and the shared
// version as locked, and, when change reports it wrote, bumps the shared version
// before committing. The write, its audit entry and the bump land together or
// not at all. An actor naming no user — whether it fails on the row's
// updated_by or on the audit entry's actor_user_id — is ErrUserNotFound.
func runAuthSettingsTx[T any](
	ctx context.Context, db *database.DB, subject, op string,
	read func(tx *sql.Tx) (*T, error),
	change func(tx *sql.Tx, before *T, sharedVersion int64) (bool, error),
) error {
	err := runAuditedSingletonTx(ctx, db, subject, op, instanceAuthSettingsVersionLock, read,
		func(tx *sql.Tx, before *T) (bool, error) {
			var sharedVersion int64
			if err := tx.QueryRowContext(ctx, instanceAuthSettingsVersionSelect).Scan(&sharedVersion); err != nil {
				return false, fmt.Errorf("failed to read instance auth settings version: %w", err)
			}
			wrote, err := change(tx, before, sharedVersion)
			if err != nil || !wrote {
				return false, err
			}
			if _, err := bumpAuthSettingsVersion(ctx, tx); err != nil {
				return false, err
			}
			return true, nil
		})
	if isFKViolation(err) {
		return fmt.Errorf("%w: %w", repositories.ErrUserNotFound, err)
	}
	return err
}

// appendAuthSettingsAudit appends one audit entry for an auth settings change
// inside tx. before and after are already-redacted snapshots (nil when absent).
func appendAuthSettingsAudit(
	ctx context.Context, tx *sql.Tx, setting, action string, actorUserID *string, before, after json.RawMessage,
) error {
	return appendInstanceSettingsAudit(ctx, tx, &models.InstanceSettingsAuditEntry{
		Setting:     setting,
		Action:      action,
		ActorUserID: actorUserID,
		Before:      before,
		After:       after,
	})
}

// auditSnapshot marshals v (a model whose credential fields are json:"-") and
// merges extra keys into the resulting object, e.g. a client_secret
// changed/unchanged marker. A nil v must be handled by the caller.
func auditSnapshot(v any, extra map[string]any) (json.RawMessage, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(extra) == 0 {
		return encoded, nil
	}
	var snapshot map[string]any
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return nil, err
	}
	for k, val := range extra {
		snapshot[k] = val
	}
	return json.Marshal(snapshot)
}

// optionalAuditSnapshot marshals a credential-free row as an audit snapshot;
// nil (no document) for an absent row.
func optionalAuditSnapshot[T any](row *T) (json.RawMessage, error) {
	if row == nil {
		return nil, nil
	}
	return auditSnapshot(row, nil)
}
