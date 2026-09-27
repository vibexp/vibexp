package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// InstanceEmailProviderRepository implements
// repositories.InstanceEmailProviderRepository for PostgreSQL.
//
// The table's primary key is `id boolean CHECK (id)`, so it holds at most one
// row and every write is keyed on that singleton: statements carry no WHERE
// clause beyond the implicit "the row", and no tenancy or role predicate, since
// the scope is the instance itself.
type InstanceEmailProviderRepository struct {
	db *database.DB
}

// NewInstanceEmailProviderRepository creates a new InstanceEmailProviderRepository.
func NewInstanceEmailProviderRepository(db *database.DB) repositories.InstanceEmailProviderRepository {
	return &InstanceEmailProviderRepository{db: db}
}

// instanceEmailProviderColumns is the full projection, shared by every read so
// a column added to one query can never be forgotten in another.
const instanceEmailProviderColumns = `provider_type, settings, secret_encrypted,
	from_address, from_name, reply_to, contact_recipient_address, privacy_policy_url,
	last_success_at, last_error, last_error_at, created_at, updated_at, updated_by, version`

// instanceEmailProviderInsert is the column list and values shared by Upsert
// and InsertIfAbsent, in instanceEmailProviderArgs order. id is omitted: its
// default (true) is the only value the singleton CHECK admits.
const instanceEmailProviderInsert = `
	INSERT INTO instance_email_provider
	(provider_type, settings, secret_encrypted, from_address, from_name, reply_to,
	contact_recipient_address, privacy_policy_url, updated_by)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

// scanInstanceEmailProvider reads one row in instanceEmailProviderColumns order.
func scanInstanceEmailProvider(row rowScanner) (*models.InstanceEmailProvider, error) {
	var provider models.InstanceEmailProvider
	err := row.Scan(
		&provider.ProviderType, &provider.Settings, &provider.SecretEncrypted,
		&provider.FromAddress, &provider.FromName, &provider.ReplyTo,
		&provider.ContactRecipientAddress, &provider.PrivacyPolicyURL,
		&provider.LastSuccessAt, &provider.LastError, &provider.LastErrorAt,
		&provider.CreatedAt, &provider.UpdatedAt, &provider.UpdatedBy, &provider.Version,
	)
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// instanceEmailProviderArgs returns the configuration values in
// instanceEmailProviderInsert order, normalising an absent Settings first: the
// column is NOT NULL with a '{}' default, but a default only fires when the
// column is omitted, and these statements name it. SendGrid, whose only
// configuration is its secret, legitimately has no settings.
func instanceEmailProviderArgs(provider *models.InstanceEmailProvider) []any {
	if len(provider.Settings) == 0 {
		provider.Settings = json.RawMessage(`{}`)
	}
	return []any{
		provider.ProviderType, provider.Settings, provider.SecretEncrypted,
		provider.FromAddress, provider.FromName, provider.ReplyTo,
		provider.ContactRecipientAddress, provider.PrivacyPolicyURL, provider.UpdatedBy,
	}
}

// Get retrieves the instance provider, or ErrInstanceEmailProviderNotFound
// when none is stored.
func (r *InstanceEmailProviderRepository) Get(ctx context.Context) (*models.InstanceEmailProvider, error) {
	query := `SELECT ` + instanceEmailProviderColumns + ` FROM instance_email_provider`

	provider, err := scanInstanceEmailProvider(r.db.QueryRowContext(ctx, query))
	if err != nil {
		return nil, mapNoRows(
			fmt.Errorf("failed to get instance email provider: %w", err),
			repositories.ErrInstanceEmailProviderNotFound,
		)
	}

	return provider, nil
}

// Upsert creates or replaces the instance provider.
//
// A single INSERT ... ON CONFLICT (id) rather than a read-then-write: the
// singleton primary key is the arbiter, so two concurrent writers cannot both
// decide the row is absent. The health columns are deliberately left out of the
// DO UPDATE set so reconfiguring keeps the delivery history.
func (r *InstanceEmailProviderRepository) Upsert(
	ctx context.Context, provider *models.InstanceEmailProvider,
) error {
	query := instanceEmailProviderInsert + `
		ON CONFLICT (id)
		DO UPDATE SET
			provider_type = EXCLUDED.provider_type,
			settings = EXCLUDED.settings,
			secret_encrypted = EXCLUDED.secret_encrypted,
			from_address = EXCLUDED.from_address,
			from_name = EXCLUDED.from_name,
			reply_to = EXCLUDED.reply_to,
			contact_recipient_address = EXCLUDED.contact_recipient_address,
			privacy_policy_url = EXCLUDED.privacy_policy_url,
			updated_by = EXCLUDED.updated_by,
			updated_at = CURRENT_TIMESTAMP,
			version = instance_email_provider.version + 1
		RETURNING created_at, updated_at, version`

	err := r.db.QueryRowContext(ctx, query, instanceEmailProviderArgs(provider)...).
		Scan(&provider.CreatedAt, &provider.UpdatedAt, &provider.Version)
	if err != nil {
		return fmt.Errorf("failed to upsert instance email provider: %w", err)
	}

	return nil
}

// InsertIfAbsent stores the provider only when no row exists and reports
// whether it did.
//
// ON CONFLICT DO NOTHING returns no row when one already exists, so
// sql.ErrNoRows is the "already configured" answer rather than a fault. The
// database decides, not a prior read, which is what makes two replicas booting
// at once safe: exactly one of them inserts.
func (r *InstanceEmailProviderRepository) InsertIfAbsent(
	ctx context.Context, provider *models.InstanceEmailProvider,
) (bool, error) {
	query := instanceEmailProviderInsert + `
		ON CONFLICT (id) DO NOTHING
		RETURNING created_at, updated_at, version`

	err := r.db.QueryRowContext(ctx, query, instanceEmailProviderArgs(provider)...).
		Scan(&provider.CreatedAt, &provider.UpdatedAt, &provider.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to insert instance email provider: %w", err)
	}

	return true, nil
}

// Delete removes the instance provider.
func (r *InstanceEmailProviderRepository) Delete(ctx context.Context) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM instance_email_provider`)
	if err != nil {
		return fmt.Errorf("failed to delete instance email provider: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repositories.ErrInstanceEmailProviderNotFound
	}

	return nil
}

// RecordSuccess stamps a successful send on the instance provider.
//
// A targeted UPDATE of the health column only, never a read-modify-write and
// never a version bump, so it cannot clobber a concurrent configuration change.
// A missing row updates nothing and is not an error: the provider can be
// deleted while a send is in flight. last_error is kept for diagnosis; current
// health comes from comparing the timestamps.
func (r *InstanceEmailProviderRepository) RecordSuccess(ctx context.Context, at time.Time) error {
	query := `UPDATE instance_email_provider SET last_success_at = $1`

	if _, err := r.db.ExecContext(ctx, query, at); err != nil {
		return fmt.Errorf("failed to record instance email provider success: %w", err)
	}

	return nil
}

// RecordError stamps a failed send on the instance provider, with the same
// guarantees as RecordSuccess. A nil sendErr means the send did not fail, so it
// is recorded as a success rather than dereferenced.
func (r *InstanceEmailProviderRepository) RecordError(ctx context.Context, sendErr error, at time.Time) error {
	if sendErr == nil {
		return r.RecordSuccess(ctx, at)
	}

	query := `UPDATE instance_email_provider SET last_error = $1, last_error_at = $2`

	if _, err := r.db.ExecContext(ctx, query, sendErr.Error(), at); err != nil {
		return fmt.Errorf("failed to record instance email provider error: %w", err)
	}

	return nil
}
