package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// auditPurgeLockID is the fixed key of the Postgres advisory lock that lets
// only one replica run the audit retention purge at a time. The value is an
// arbitrary, stable application-chosen constant; it only has to be unique among
// the advisory lock ids this application uses.
const auditPurgeLockID int64 = 0x7662_6175_6470_7267 // "vbaudprg"

// InstanceAuditSettingsRepository implements
// repositories.InstanceAuditSettingsRepository for PostgreSQL.
//
// The table's primary key is `id boolean CHECK (id)`, so it holds at most one
// row and statements carry no WHERE clause and no tenancy or role predicate:
// the scope is the instance itself.
type InstanceAuditSettingsRepository struct {
	db *database.DB
}

// NewInstanceAuditSettingsRepository creates a new
// InstanceAuditSettingsRepository.
func NewInstanceAuditSettingsRepository(db *database.DB) repositories.InstanceAuditSettingsRepository {
	return &InstanceAuditSettingsRepository{db: db}
}

// instanceAuditSettingsSelect reads the singleton row;
// scanInstanceAuditSettings reads its columns in this order.
const instanceAuditSettingsSelect = `SELECT retention_weeks, capture_ip_ua,
		created_at, updated_at, updated_by, version
		FROM instance_audit_settings`

// instanceAuditSettingsUpsert creates or replaces the row in one INSERT ... ON
// CONFLICT (id). id is omitted: its default (true) is the only value the
// singleton CHECK admits.
const instanceAuditSettingsUpsert = `
	INSERT INTO instance_audit_settings (retention_weeks, capture_ip_ua, updated_by)
	VALUES ($1, $2, $3)
		ON CONFLICT (id)
		DO UPDATE SET
			retention_weeks = EXCLUDED.retention_weeks,
			capture_ip_ua = EXCLUDED.capture_ip_ua,
			updated_by = EXCLUDED.updated_by,
			updated_at = CURRENT_TIMESTAMP,
			version = instance_audit_settings.version + 1
		RETURNING created_at, updated_at, version`

func scanInstanceAuditSettings(row *sql.Row) (*models.InstanceAuditSettings, error) {
	var s models.InstanceAuditSettings
	if err := row.Scan(
		&s.RetentionWeeks, &s.CaptureIPUA,
		&s.CreatedAt, &s.UpdatedAt, &s.UpdatedBy, &s.Version,
	); err != nil {
		return nil, err
	}
	return &s, nil
}

// Get retrieves the stored settings, or ErrInstanceAuditSettingsNotFound when
// none are stored.
func (r *InstanceAuditSettingsRepository) Get(ctx context.Context) (*models.InstanceAuditSettings, error) {
	s, err := scanInstanceAuditSettings(r.db.QueryRowContext(ctx, instanceAuditSettingsSelect))
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get instance audit settings: %w", err),
			repositories.ErrInstanceAuditSettingsNotFound,
		)
	}

	return s, nil
}

// instanceAuditSettingsSubject names the table's contents in errors and logs.
const instanceAuditSettingsSubject = "instance audit settings"

// UpsertAudited creates or replaces the stored settings and appends the audit
// entry audit builds, in one transaction. A non-nil expectedVersion is checked
// against the row read under the lock (see checkSingletonVersion).
func (r *InstanceAuditSettingsRepository) UpsertAudited(
	ctx context.Context, s *models.InstanceAuditSettings, expectedVersion *int64,
	audit repositories.InstanceAuditSettingsAuditFunc,
) error {
	return r.inAuditedTx(ctx, "upsert", func(tx *sql.Tx, before *models.InstanceAuditSettings) (bool, error) {
		var storedVersion *int64
		if before != nil {
			storedVersion = &before.Version
		}
		if err := checkSingletonVersion(expectedVersion, storedVersion); err != nil {
			return false, err
		}
		err := tx.QueryRowContext(ctx, instanceAuditSettingsUpsert, s.RetentionWeeks, s.CaptureIPUA, s.UpdatedBy).
			Scan(&s.CreatedAt, &s.UpdatedAt, &s.Version)
		if err != nil {
			return false, fmt.Errorf("failed to upsert instance audit settings: %w", err)
		}
		return true, appendBuiltSingletonAudit[models.InstanceAuditSettings](
			ctx, tx, instanceAuditSettingsSubject, audit, before, s)
	})
}

// DeleteAudited removes the stored settings and appends the audit entry audit
// builds, in one transaction. With no row stored it writes nothing.
func (r *InstanceAuditSettingsRepository) DeleteAudited(
	ctx context.Context, audit repositories.InstanceAuditSettingsAuditFunc,
) (bool, error) {
	var deleted bool
	err := r.inAuditedTx(ctx, "delete", func(tx *sql.Tx, before *models.InstanceAuditSettings) (bool, error) {
		if before == nil {
			return false, nil
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM instance_audit_settings`); err != nil {
			return false, fmt.Errorf("failed to delete instance audit settings: %w", err)
		}
		deleted = true
		return true, appendBuiltSingletonAudit[models.InstanceAuditSettings](
			ctx, tx, instanceAuditSettingsSubject, audit, before, nil)
	})
	return deleted, err
}

// instanceAuditSettingsWriteLock serializes audited writes; see
// runAuditedSingletonTx for why it is a table lock.
const instanceAuditSettingsWriteLock = `LOCK TABLE instance_audit_settings IN SHARE ROW EXCLUSIVE MODE`

// inAuditedTx runs change under the table's write lock with the current row;
// see runAuditedSingletonTx.
func (r *InstanceAuditSettingsRepository) inAuditedTx(
	ctx context.Context, op string,
	change func(tx *sql.Tx, before *models.InstanceAuditSettings) (bool, error),
) error {
	return runAuditedSingletonTx(ctx, r.db, instanceAuditSettingsSubject, op, instanceAuditSettingsWriteLock,
		func(tx *sql.Tx) (*models.InstanceAuditSettings, error) {
			return scanInstanceAuditSettings(tx.QueryRowContext(ctx, instanceAuditSettingsSelect))
		}, change)
}

// TryPurgeLock takes the audit purge's advisory lock without blocking.
//
// A session-level advisory lock belongs to one connection, so the lock and the
// unlock must run on the same one: through the pool they could land on
// different sessions, and the lock would leak until its connection closed
// (#728/#751). The connection is therefore pinned until release, which unlocks
// and closes it. The unlock runs on context.Background() so it still happens
// when the purge's own context was cancelled (shutdown, timeout). When the lock
// is already held elsewhere the connection goes straight back to the pool and
// release is a no-op.
func (r *InstanceAuditSettingsRepository) TryPurgeLock(
	ctx context.Context,
) (acquired bool, release func() error, err error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("failed to acquire connection for the audit purge lock: %w", err)
	}

	var locked bool
	if scanErr := conn.QueryRowContext(ctx,
		`SELECT pg_try_advisory_lock($1)`, auditPurgeLockID).Scan(&locked); scanErr != nil {
		if cerr := conn.Close(); cerr != nil {
			scanErr = errors.Join(scanErr, cerr)
		}
		return false, nil, fmt.Errorf("failed to acquire the audit purge lock: %w", scanErr)
	}

	if !locked {
		if cerr := conn.Close(); cerr != nil {
			return false, nil, fmt.Errorf("failed to release connection after a contended audit purge lock: %w", cerr)
		}
		return false, func() error { return nil }, nil
	}

	release = func() error {
		_, unlockErr := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, auditPurgeLockID)
		if unlockErr != nil {
			unlockErr = fmt.Errorf("failed to release the audit purge lock: %w", unlockErr)
		}
		if cerr := conn.Close(); cerr != nil {
			unlockErr = errors.Join(unlockErr, fmt.Errorf("failed to close the audit purge lock connection: %w", cerr))
		}
		return unlockErr
	}
	return true, release, nil
}
