package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// InstanceAuthProviderRepository implements
// repositories.InstanceAuthProviderRepository for PostgreSQL.
//
// The scope is the instance, so statements carry no tenancy or role predicate.
// It stores whatever client-secret ciphertext it is handed and never encrypts
// or decrypts.
type InstanceAuthProviderRepository struct {
	db *database.DB
}

// NewInstanceAuthProviderRepository creates a new InstanceAuthProviderRepository.
func NewInstanceAuthProviderRepository(db *database.DB) repositories.InstanceAuthProviderRepository {
	return &InstanceAuthProviderRepository{db: db}
}

// instanceAuthProvidersSubject names the providers in errors and logs.
const instanceAuthProvidersSubject = "instance auth provider"

// The provider SELECTs below read the same columns, in the order
// scanInstanceAuthProvider scans them. Each is one literal (not a shared
// column-list concatenation) so every statement stays a single static string.
const instanceAuthProviderByID = `SELECT id, type, slug, display_name, enabled, sort_order,
		client_id, client_secret_encrypted, issuer_url, created_at, updated_at, updated_by
		FROM instance_auth_providers WHERE id = $1`

const instanceAuthProviderBySlug = `SELECT id, type, slug, display_name, enabled, sort_order,
		client_id, client_secret_encrypted, issuer_url, created_at, updated_at, updated_by
		FROM instance_auth_providers WHERE slug = $1`

const instanceAuthProviderList = `SELECT id, type, slug, display_name, enabled, sort_order,
		client_id, client_secret_encrypted, issuer_url, created_at, updated_at, updated_by
		FROM instance_auth_providers ORDER BY sort_order, slug`

// instanceAuthProviderInsert creates a provider, in instanceAuthProviderArgs
// order.
const instanceAuthProviderInsert = `INSERT INTO instance_auth_providers
	(type, slug, display_name, enabled, sort_order, client_id, client_secret_encrypted, issuer_url, updated_by)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	RETURNING id, created_at, updated_at`

// instanceAuthProviderInsertIfAbsent is instanceAuthProviderInsert that yields
// no row when the slug, or for google/github the type, is already stored. With
// no conflict target it covers both unique indexes.
const instanceAuthProviderInsertIfAbsent = `INSERT INTO instance_auth_providers
	(type, slug, display_name, enabled, sort_order, client_id, client_secret_encrypted, issuer_url, updated_by)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	ON CONFLICT DO NOTHING
	RETURNING id, created_at, updated_at`

// instanceAuthProviderUpdate replaces the mutable columns. Slug and type are
// immutable, so neither is written; both are returned so the caller's struct
// reflects the stored row.
const instanceAuthProviderUpdate = `UPDATE instance_auth_providers
	SET display_name = $2, enabled = $3, sort_order = $4, client_id = $5,
		client_secret_encrypted = $6, issuer_url = $7, updated_by = $8,
		updated_at = CURRENT_TIMESTAMP
	WHERE id = $1
	RETURNING type, slug, created_at, updated_at`

const instanceAuthProviderDelete = `DELETE FROM instance_auth_providers WHERE id = $1`

func instanceAuthProviderArgs(p *models.InstanceAuthProvider) []any {
	return []any{
		p.Type, p.Slug, p.DisplayName, p.Enabled, p.SortOrder,
		p.ClientID, p.ClientSecretEncrypted, p.IssuerURL, p.UpdatedBy,
	}
}

func scanInstanceAuthProvider(row rowScanner) (*models.InstanceAuthProvider, error) {
	var p models.InstanceAuthProvider
	if err := row.Scan(
		&p.ID, &p.Type, &p.Slug, &p.DisplayName, &p.Enabled, &p.SortOrder,
		&p.ClientID, &p.ClientSecretEncrypted, &p.IssuerURL, &p.CreatedAt, &p.UpdatedAt, &p.UpdatedBy,
	); err != nil {
		return nil, err
	}
	return &p, nil
}

// List returns every provider ordered by sort_order, then slug.
func (r *InstanceAuthProviderRepository) List(ctx context.Context) ([]*models.InstanceAuthProvider, error) {
	return queryAdminRows(ctx, r.db, "instance auth providers",
		func(rows *sql.Rows) (*models.InstanceAuthProvider, error) { return scanInstanceAuthProvider(rows) },
		instanceAuthProviderList)
}

// Get returns one provider by id, or ErrInstanceAuthProviderNotFound.
func (r *InstanceAuthProviderRepository) Get(ctx context.Context, id string) (*models.InstanceAuthProvider, error) {
	p, err := scanInstanceAuthProvider(r.db.QueryRowContext(ctx, instanceAuthProviderByID, id))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get instance auth provider: %w", err),
			repositories.ErrInstanceAuthProviderNotFound)
	}
	return p, nil
}

// GetBySlug returns one provider by slug, or ErrInstanceAuthProviderNotFound.
func (r *InstanceAuthProviderRepository) GetBySlug(
	ctx context.Context, slug string,
) (*models.InstanceAuthProvider, error) {
	p, err := scanInstanceAuthProvider(r.db.QueryRowContext(ctx, instanceAuthProviderBySlug, slug))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get instance auth provider by slug: %w", err),
			repositories.ErrInstanceAuthProviderNotFound)
	}
	return p, nil
}

// Create stores a new provider; see the interface.
func (r *InstanceAuthProviderRepository) Create(
	ctx context.Context, p *models.InstanceAuthProvider, actorUserID *string, expectedVersion *int64,
) error {
	p.UpdatedBy = actorUserID
	return r.inTx(ctx, "create", nil,
		func(tx *sql.Tx, _ *models.InstanceAuthProvider, sharedVersion int64) (bool, error) {
			if err := checkSingletonVersion(expectedVersion, &sharedVersion); err != nil {
				return false, err
			}
			err := tx.QueryRowContext(ctx, instanceAuthProviderInsert, instanceAuthProviderArgs(p)...).
				Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
			if err != nil {
				return false, mapInstanceAuthProviderWriteError("create", err)
			}
			return true, appendInstanceAuthProviderAudit(ctx, tx, models.InstanceSettingsAuditActionUpsert,
				actorUserID, nil, p)
		})
}

// Update replaces a provider's mutable fields; see the interface.
func (r *InstanceAuthProviderRepository) Update(
	ctx context.Context, p *models.InstanceAuthProvider, actorUserID *string, expectedVersion *int64,
) error {
	p.UpdatedBy = actorUserID
	return r.inTx(ctx, "update", &p.ID,
		func(tx *sql.Tx, before *models.InstanceAuthProvider, sharedVersion int64) (bool, error) {
			if err := checkSingletonVersion(expectedVersion, &sharedVersion); err != nil {
				return false, err
			}
			if before == nil {
				return false, repositories.ErrInstanceAuthProviderNotFound
			}
			err := tx.QueryRowContext(ctx, instanceAuthProviderUpdate,
				p.ID, p.DisplayName, p.Enabled, p.SortOrder, p.ClientID,
				p.ClientSecretEncrypted, p.IssuerURL, p.UpdatedBy,
			).Scan(&p.Type, &p.Slug, &p.CreatedAt, &p.UpdatedAt)
			if err != nil {
				return false, mapInstanceAuthProviderWriteError("update", err)
			}
			return true, appendInstanceAuthProviderAudit(ctx, tx, models.InstanceSettingsAuditActionUpsert,
				actorUserID, before, p)
		})
}

// Delete removes a provider; see the interface.
func (r *InstanceAuthProviderRepository) Delete(
	ctx context.Context, id string, actorUserID *string, expectedVersion *int64,
) error {
	return r.inTx(ctx, "delete", &id,
		func(tx *sql.Tx, before *models.InstanceAuthProvider, sharedVersion int64) (bool, error) {
			if err := checkSingletonVersion(expectedVersion, &sharedVersion); err != nil {
				return false, err
			}
			if before == nil {
				return false, repositories.ErrInstanceAuthProviderNotFound
			}
			if _, err := tx.ExecContext(ctx, instanceAuthProviderDelete, id); err != nil {
				return false, fmt.Errorf("failed to delete instance auth provider: %w", err)
			}
			return true, appendInstanceAuthProviderAudit(ctx, tx, models.InstanceSettingsAuditActionDelete,
				actorUserID, before, nil)
		})
}

// InsertIfAbsent stores a provider for the boot-time import; see the interface.
func (r *InstanceAuthProviderRepository) InsertIfAbsent(
	ctx context.Context, p *models.InstanceAuthProvider,
) (bool, error) {
	p.UpdatedBy = nil
	var inserted bool
	err := r.inTx(ctx, "import", nil,
		func(tx *sql.Tx, _ *models.InstanceAuthProvider, _ int64) (bool, error) {
			err := tx.QueryRowContext(ctx, instanceAuthProviderInsertIfAbsent, instanceAuthProviderArgs(p)...).
				Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
			if errors.Is(err, sql.ErrNoRows) {
				return false, nil
			}
			if err != nil {
				return false, mapInstanceAuthProviderWriteError("import", err)
			}
			inserted = true
			return true, appendInstanceAuthProviderAudit(ctx, tx, models.InstanceSettingsAuditActionImport,
				nil, nil, p)
		})
	if err != nil {
		return false, err
	}
	return inserted, nil
}

// inTx runs one audited provider write under the shared version lock. id names
// the provider the write targets, read as before (nil when absent); a nil id
// means a create, with no before.
func (r *InstanceAuthProviderRepository) inTx(
	ctx context.Context, op string, id *string,
	change func(tx *sql.Tx, before *models.InstanceAuthProvider, sharedVersion int64) (bool, error),
) error {
	return runAuthSettingsTx(ctx, r.db, instanceAuthProvidersSubject, op,
		func(tx *sql.Tx) (*models.InstanceAuthProvider, error) {
			if id == nil {
				return nil, nil
			}
			return scanInstanceAuthProvider(tx.QueryRowContext(ctx, instanceAuthProviderByID, *id))
		}, change)
}

// mapInstanceAuthProviderWriteError maps a unique violation (slug taken, or a
// second google/github provider) to ErrInstanceAuthProviderConflict and a CHECK
// violation to ErrInstanceAuthProviderInvalid.
func mapInstanceAuthProviderWriteError(op string, err error) error {
	if uniqueViolation(err) != nil {
		return fmt.Errorf("failed to %s instance auth provider: %w: %w",
			op, repositories.ErrInstanceAuthProviderConflict, err)
	}
	if isCheckViolation(err) {
		return fmt.Errorf("failed to %s instance auth provider: %w: %w",
			op, repositories.ErrInstanceAuthProviderInvalid, err)
	}
	return fmt.Errorf("failed to %s instance auth provider: %w", op, err)
}

// appendInstanceAuthProviderAudit records one provider change. The ciphertext
// never enters a snapshot (the model marshals it as json:"-"); each snapshot
// carries has_client_secret, and the after snapshot a client_secret
// changed/unchanged marker.
func appendInstanceAuthProviderAudit(
	ctx context.Context, tx *sql.Tx, action string, actorUserID *string,
	before, after *models.InstanceAuthProvider,
) error {
	beforeDoc, err := instanceAuthProviderSnapshot(before, "")
	if err != nil {
		return err
	}
	afterDoc, err := instanceAuthProviderSnapshot(after, instanceAuthProviderSecretMarker(before, after))
	if err != nil {
		return err
	}
	return appendAuthSettingsAudit(ctx, tx, models.InstanceSettingAuthProviders, action, actorUserID,
		beforeDoc, afterDoc)
}

// instanceAuthProviderSnapshot is the redacted audit shape of a provider; nil
// for an absent one. secretMarker, when non-empty, is stored as client_secret.
func instanceAuthProviderSnapshot(p *models.InstanceAuthProvider, secretMarker string) (json.RawMessage, error) {
	if p == nil {
		return nil, nil
	}
	extra := map[string]any{"has_client_secret": p.HasClientSecret()}
	if secretMarker != "" {
		extra["client_secret"] = secretMarker
	}
	doc, err := auditSnapshot(p, extra)
	if err != nil {
		return nil, fmt.Errorf("failed to snapshot the instance auth provider: %w", err)
	}
	return doc, nil
}

// instanceAuthProviderSecretMarker says whether a write changed the stored
// ciphertext; "" for a delete (no after snapshot).
func instanceAuthProviderSecretMarker(before, after *models.InstanceAuthProvider) string {
	if after == nil {
		return ""
	}
	var stored *string
	if before != nil {
		stored = before.ClientSecretEncrypted
	}
	if equalOptionalStrings(stored, after.ClientSecretEncrypted) {
		return models.InstanceSettingsAuditSecretUnchanged
	}
	return models.InstanceSettingsAuditSecretChanged
}

func equalOptionalStrings(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
