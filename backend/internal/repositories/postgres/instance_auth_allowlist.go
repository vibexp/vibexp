package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// InstanceAuthAllowlistRepository implements
// repositories.InstanceAuthAllowlistRepository for PostgreSQL.
//
// The table's primary key is `id boolean CHECK (id)`, so it holds at most one
// row and statements carry no WHERE clause and no tenancy or role predicate.
type InstanceAuthAllowlistRepository struct {
	db *database.DB
}

// NewInstanceAuthAllowlistRepository creates a new InstanceAuthAllowlistRepository.
func NewInstanceAuthAllowlistRepository(db *database.DB) repositories.InstanceAuthAllowlistRepository {
	return &InstanceAuthAllowlistRepository{db: db}
}

// instanceAuthAllowlistSubject names the allowlist in errors and logs.
const instanceAuthAllowlistSubject = "instance auth allowlist"

const instanceAuthAllowlistSelect = `SELECT domains, emails, created_at, updated_at, updated_by, version
	FROM instance_auth_allowlist`

// instanceAuthAllowlistUpsert creates or replaces the row. With no row stored
// it is a plain insert, which InsertIfAbsent relies on under the lock.
const instanceAuthAllowlistUpsert = `INSERT INTO instance_auth_allowlist (domains, emails, updated_by)
	VALUES ($1, $2, $3)
	ON CONFLICT (id)
	DO UPDATE SET
		domains = EXCLUDED.domains,
		emails = EXCLUDED.emails,
		updated_by = EXCLUDED.updated_by,
		updated_at = CURRENT_TIMESTAMP,
		version = instance_auth_allowlist.version + 1
	RETURNING created_at, updated_at, version`

const instanceAuthAllowlistDelete = `DELETE FROM instance_auth_allowlist`

func scanInstanceAuthAllowlist(row rowScanner) (*models.InstanceAuthAllowlist, error) {
	var a models.InstanceAuthAllowlist
	if err := row.Scan(
		(*pq.StringArray)(&a.Domains), (*pq.StringArray)(&a.Emails),
		&a.CreatedAt, &a.UpdatedAt, &a.UpdatedBy, &a.Version,
	); err != nil {
		return nil, err
	}
	return &a, nil
}

// nonNilStringArray binds a nil list as an empty array: the columns are NOT NULL.
func nonNilStringArray(values []string) pq.StringArray {
	if values == nil {
		return pq.StringArray{}
	}
	return pq.StringArray(values)
}

// Get returns the stored allowlist, or ErrInstanceAuthAllowlistNotFound.
func (r *InstanceAuthAllowlistRepository) Get(ctx context.Context) (*models.InstanceAuthAllowlist, error) {
	a, err := scanInstanceAuthAllowlist(r.db.QueryRowContext(ctx, instanceAuthAllowlistSelect))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get instance auth allowlist: %w", err),
			repositories.ErrInstanceAuthAllowlistNotFound)
	}
	return a, nil
}

// UpsertAudited creates or replaces the allowlist; see the interface.
func (r *InstanceAuthAllowlistRepository) UpsertAudited(
	ctx context.Context, a *models.InstanceAuthAllowlist, actorUserID *string, expectedVersion *int64,
) error {
	a.UpdatedBy = actorUserID
	return r.inTx(ctx, "upsert", func(tx *sql.Tx, before *models.InstanceAuthAllowlist) (bool, error) {
		var storedVersion *int64
		if before != nil {
			storedVersion = &before.Version
		}
		if err := checkSingletonVersion(expectedVersion, storedVersion); err != nil {
			return false, err
		}
		if err := r.write(ctx, tx, a); err != nil {
			return false, err
		}
		return true, appendInstanceAuthAllowlistAudit(ctx, tx, models.InstanceSettingsAuditActionUpsert,
			actorUserID, before, a)
	})
}

// DeleteAudited removes the allowlist; see the interface.
func (r *InstanceAuthAllowlistRepository) DeleteAudited(ctx context.Context, actorUserID *string) (bool, error) {
	var deleted bool
	err := r.inTx(ctx, "delete", func(tx *sql.Tx, before *models.InstanceAuthAllowlist) (bool, error) {
		if before == nil {
			return false, nil
		}
		if _, err := tx.ExecContext(ctx, instanceAuthAllowlistDelete); err != nil {
			return false, fmt.Errorf("failed to delete instance auth allowlist: %w", err)
		}
		deleted = true
		return true, appendInstanceAuthAllowlistAudit(ctx, tx, models.InstanceSettingsAuditActionDelete,
			actorUserID, before, nil)
	})
	return deleted, err
}

// InsertIfAbsent stores the allowlist for the boot-time import; see the
// interface. Under the lock, no row read means none exists, so the upsert
// statement is a plain insert.
func (r *InstanceAuthAllowlistRepository) InsertIfAbsent(
	ctx context.Context, a *models.InstanceAuthAllowlist,
) (bool, error) {
	a.UpdatedBy = nil
	var inserted bool
	err := r.inTx(ctx, "import", func(tx *sql.Tx, before *models.InstanceAuthAllowlist) (bool, error) {
		if before != nil {
			return false, nil
		}
		if err := r.write(ctx, tx, a); err != nil {
			return false, err
		}
		inserted = true
		return true, appendInstanceAuthAllowlistAudit(ctx, tx, models.InstanceSettingsAuditActionImport,
			nil, nil, a)
	})
	if err != nil {
		return false, err
	}
	return inserted, nil
}

// instanceAuthAllowlistUsersOutside lists the active users a candidate
// allowlist would not admit, with the total in every row. It mirrors the
// matching of services.AccessAllowlistResolver: the trimmed, lower-cased email
// is compared with the listed emails ($1), and its domain, the part after the
// LAST "@", with the listed domains ($2). An email with no "@" has no domain
// and matches none. $3 holds the exempt (root admin) emails. Suspended users
// cannot authenticate, so they are not counted.
const instanceAuthAllowlistUsersOutside = `SELECT u.email, COUNT(*) OVER () AS total
	FROM (
		SELECT email, lower(btrim(email)) AS normalized
		FROM users
		WHERE status IS DISTINCT FROM 'suspended'
	) u
	WHERE NOT (u.normalized = ANY($1))
		AND NOT COALESCE(substring(u.normalized from '@([^@]*)$') = ANY($2), false)
		AND NOT (u.normalized = ANY($3))
	ORDER BY u.normalized
	LIMIT $4`

// usersOutsideAllowlistRow is one row of instanceAuthAllowlistUsersOutside.
type usersOutsideAllowlistRow struct {
	email string
	total int
}

func scanUsersOutsideAllowlistRow(rows *sql.Rows) (usersOutsideAllowlistRow, error) {
	var row usersOutsideAllowlistRow
	err := rows.Scan(&row.email, &row.total)
	return row, err
}

// CountUsersOutside counts the active users a candidate allowlist would not
// admit; see the interface.
func (r *InstanceAuthAllowlistRepository) CountUsersOutside(
	ctx context.Context, domains, emails, exemptEmails []string, sampleLimit int,
) (int, []string, error) {
	rows, err := queryAdminRows(ctx, r.db, "users outside the instance auth allowlist",
		scanUsersOutsideAllowlistRow, instanceAuthAllowlistUsersOutside,
		nonNilStringArray(emails), nonNilStringArray(domains), nonNilStringArray(exemptEmails),
		max(sampleLimit, 1),
	)
	if err != nil {
		return 0, nil, err
	}
	if len(rows) == 0 {
		return 0, []string{}, nil
	}
	sample := make([]string, 0, len(rows))
	for _, row := range rows {
		sample = append(sample, row.email)
	}
	return rows[0].total, sample, nil
}

// write runs the upsert inside tx and refreshes a from the stored row.
func (r *InstanceAuthAllowlistRepository) write(
	ctx context.Context, tx *sql.Tx, a *models.InstanceAuthAllowlist,
) error {
	a.Domains, a.Emails = nonNilStringArray(a.Domains), nonNilStringArray(a.Emails)
	err := tx.QueryRowContext(ctx, instanceAuthAllowlistUpsert,
		pq.StringArray(a.Domains), pq.StringArray(a.Emails), a.UpdatedBy,
	).Scan(&a.CreatedAt, &a.UpdatedAt, &a.Version)
	if err != nil {
		return fmt.Errorf("failed to write instance auth allowlist: %w", err)
	}
	return nil
}

// inTx runs one audited allowlist write under the shared version lock with the
// current row; see runAuthSettingsTx. The allowlist's compare-and-set is on its
// own version, so the shared version is not consulted here.
func (r *InstanceAuthAllowlistRepository) inTx(
	ctx context.Context, op string,
	change func(tx *sql.Tx, before *models.InstanceAuthAllowlist) (bool, error),
) error {
	return runAuthSettingsTx(ctx, r.db, instanceAuthAllowlistSubject, op,
		func(tx *sql.Tx) (*models.InstanceAuthAllowlist, error) {
			return scanInstanceAuthAllowlist(tx.QueryRowContext(ctx, instanceAuthAllowlistSelect))
		},
		func(tx *sql.Tx, before *models.InstanceAuthAllowlist, _ int64) (bool, error) {
			return change(tx, before)
		})
}

// appendInstanceAuthAllowlistAudit records one allowlist change. The allowlist
// holds no credential, so the snapshot is the row itself.
func appendInstanceAuthAllowlistAudit(
	ctx context.Context, tx *sql.Tx, action string, actorUserID *string,
	before, after *models.InstanceAuthAllowlist,
) error {
	beforeDoc, err := optionalAuditSnapshot(before)
	if err != nil {
		return fmt.Errorf("failed to snapshot the instance auth allowlist: %w", err)
	}
	afterDoc, err := optionalAuditSnapshot(after)
	if err != nil {
		return fmt.Errorf("failed to snapshot the instance auth allowlist: %w", err)
	}
	return appendAuthSettingsAudit(ctx, tx, models.InstanceSettingAuthAllowlist, action, actorUserID,
		beforeDoc, afterDoc)
}
