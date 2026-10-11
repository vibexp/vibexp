package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// InstanceSiteInfoRepository implements repositories.InstanceSiteInfoRepository
// for PostgreSQL.
//
// The table's primary key is `id boolean CHECK (id)`, so it holds at most one
// row and statements carry no WHERE clause and no tenancy or role predicate:
// the scope is the instance itself.
type InstanceSiteInfoRepository struct {
	db *database.DB
}

// NewInstanceSiteInfoRepository creates a new InstanceSiteInfoRepository.
func NewInstanceSiteInfoRepository(db *database.DB) repositories.InstanceSiteInfoRepository {
	return &InstanceSiteInfoRepository{db: db}
}

// instanceSiteInfoSelect reads the singleton row; scanInstanceSiteInfo reads
// its columns in this order.
const instanceSiteInfoSelect = `SELECT company_name, company_email, website, support_url,
		terms_url, privacy_url, logo_url, logo_dark_url,
		created_at, updated_at, updated_by, version
		FROM instance_site_info`

// instanceSiteInfoUpsert creates or replaces the row in one INSERT ... ON
// CONFLICT (id); its values are in instanceSiteInfoArgs order. id is omitted:
// its default (true) is the only value the singleton CHECK admits.
const instanceSiteInfoUpsert = `
	INSERT INTO instance_site_info
	(company_name, company_email, website, support_url,
	terms_url, privacy_url, logo_url, logo_dark_url, updated_by)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id)
		DO UPDATE SET
			company_name = EXCLUDED.company_name,
			company_email = EXCLUDED.company_email,
			website = EXCLUDED.website,
			support_url = EXCLUDED.support_url,
			terms_url = EXCLUDED.terms_url,
			privacy_url = EXCLUDED.privacy_url,
			logo_url = EXCLUDED.logo_url,
			logo_dark_url = EXCLUDED.logo_dark_url,
			updated_by = EXCLUDED.updated_by,
			updated_at = CURRENT_TIMESTAMP,
			version = instance_site_info.version + 1
		RETURNING created_at, updated_at, version`

func instanceSiteInfoArgs(s *models.InstanceSiteInfo) []any {
	return []any{
		s.CompanyName, s.CompanyEmail, s.Website, s.SupportURL,
		s.TermsURL, s.PrivacyURL, s.LogoURL, s.LogoDarkURL, s.UpdatedBy,
	}
}

func scanInstanceSiteInfo(row *sql.Row) (*models.InstanceSiteInfo, error) {
	var s models.InstanceSiteInfo
	if err := row.Scan(
		&s.CompanyName, &s.CompanyEmail, &s.Website, &s.SupportURL,
		&s.TermsURL, &s.PrivacyURL, &s.LogoURL, &s.LogoDarkURL,
		&s.CreatedAt, &s.UpdatedAt, &s.UpdatedBy, &s.Version,
	); err != nil {
		return nil, err
	}
	return &s, nil
}

// Get retrieves the stored site info, or ErrInstanceSiteInfoNotFound when none
// is stored.
func (r *InstanceSiteInfoRepository) Get(ctx context.Context) (*models.InstanceSiteInfo, error) {
	s, err := scanInstanceSiteInfo(r.db.QueryRowContext(ctx, instanceSiteInfoSelect))
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get instance site info: %w", err),
			repositories.ErrInstanceSiteInfoNotFound,
		)
	}

	return s, nil
}

// instanceSiteInfoSubject names the table's contents in errors and logs.
const instanceSiteInfoSubject = "instance site info"

// UpsertAudited creates or replaces the stored site info and appends the audit
// entry audit builds, in one transaction. A non-nil expectedVersion is checked
// against the row read under the lock (see checkSingletonVersion).
func (r *InstanceSiteInfoRepository) UpsertAudited(
	ctx context.Context, s *models.InstanceSiteInfo, expectedVersion *int64,
	audit repositories.InstanceSiteInfoAuditFunc,
) error {
	return r.inAuditedTx(ctx, "upsert", func(tx *sql.Tx, before *models.InstanceSiteInfo) (bool, error) {
		var storedVersion *int64
		if before != nil {
			storedVersion = &before.Version
		}
		if err := checkSingletonVersion(expectedVersion, storedVersion); err != nil {
			return false, err
		}
		err := tx.QueryRowContext(ctx, instanceSiteInfoUpsert, instanceSiteInfoArgs(s)...).
			Scan(&s.CreatedAt, &s.UpdatedAt, &s.Version)
		if err != nil {
			return false, fmt.Errorf("failed to upsert instance site info: %w", err)
		}
		return true, appendBuiltSingletonAudit[models.InstanceSiteInfo](
			ctx, tx, instanceSiteInfoSubject, audit, before, s)
	})
}

// DeleteAudited removes the stored site info and appends the audit entry audit
// builds, in one transaction. With no row stored it writes nothing.
func (r *InstanceSiteInfoRepository) DeleteAudited(
	ctx context.Context, audit repositories.InstanceSiteInfoAuditFunc,
) (bool, error) {
	var deleted bool
	err := r.inAuditedTx(ctx, "delete", func(tx *sql.Tx, before *models.InstanceSiteInfo) (bool, error) {
		if before == nil {
			return false, nil
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM instance_site_info`); err != nil {
			return false, fmt.Errorf("failed to delete instance site info: %w", err)
		}
		deleted = true
		return true, appendBuiltSingletonAudit[models.InstanceSiteInfo](
			ctx, tx, instanceSiteInfoSubject, audit, before, nil)
	})
	return deleted, err
}

// instanceSiteInfoWriteLock serializes audited writes; see
// runAuditedSingletonTx for why it is a table lock.
const instanceSiteInfoWriteLock = `LOCK TABLE instance_site_info IN SHARE ROW EXCLUSIVE MODE`

// inAuditedTx runs change under the table's write lock with the current row;
// see runAuditedSingletonTx.
func (r *InstanceSiteInfoRepository) inAuditedTx(
	ctx context.Context, op string,
	change func(tx *sql.Tx, before *models.InstanceSiteInfo) (bool, error),
) error {
	return runAuditedSingletonTx(ctx, r.db, instanceSiteInfoSubject, op, instanceSiteInfoWriteLock,
		func(tx *sql.Tx) (*models.InstanceSiteInfo, error) {
			return scanInstanceSiteInfo(tx.QueryRowContext(ctx, instanceSiteInfoSelect))
		}, change)
}
