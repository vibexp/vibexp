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

// OAuthSigningKeyRepository implements repositories.OAuthSigningKeyRepository for PostgreSQL.
type OAuthSigningKeyRepository struct {
	db *database.DB
}

// NewOAuthSigningKeyRepository creates a new OAuthSigningKeyRepository.
func NewOAuthSigningKeyRepository(db *database.DB) repositories.OAuthSigningKeyRepository {
	return &OAuthSigningKeyRepository{db: db}
}

// Create inserts a new (inactive) signing key.
func (r *OAuthSigningKeyRepository) Create(ctx context.Context, key *models.OAuthSigningKey) error {
	query := `
		INSERT INTO oauth_signing_keys (kid, algorithm, private_key_encrypted, public_jwk, active)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := r.db.ExecContext(ctx, query,
		key.KID, key.Algorithm, key.PrivateKeyEncrypted, key.PublicJWK, key.Active)
	if err != nil {
		return fmt.Errorf("failed to create oauth signing key: %w", err)
	}
	return nil
}

// GetActive returns the single active signing key or ErrOAuthSigningKeyNotFound.
func (r *OAuthSigningKeyRepository) GetActive(ctx context.Context) (*models.OAuthSigningKey, error) {
	query := `
		SELECT kid, algorithm, private_key_encrypted, public_jwk, active, created_at, rotated_at
		FROM oauth_signing_keys WHERE active ORDER BY created_at DESC LIMIT 1`
	key, err := scanSigningKey(r.db.QueryRowContext(ctx, query))
	if err != nil {
		return nil, mapNoRows(err, repositories.ErrOAuthSigningKeyNotFound)
	}
	return key, nil
}

// ListAll returns every signing key, newest first, for building the JWKS.
func (r *OAuthSigningKeyRepository) ListAll(ctx context.Context) ([]*models.OAuthSigningKey, error) {
	query := `
		SELECT kid, algorithm, private_key_encrypted, public_jwk, active, created_at, rotated_at
		FROM oauth_signing_keys ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list oauth signing keys: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()

	var keys []*models.OAuthSigningKey
	for rows.Next() {
		key, scanErr := scanSigningKey(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan oauth signing key: %w", scanErr)
		}
		keys = append(keys, key)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("failed to iterate oauth signing keys: %w", rowsErr)
	}
	return keys, err
}

// Activate atomically promotes kid to the sole active key, stamping rotated_at on
// keys it deactivates.
func (r *OAuthSigningKeyRepository) Activate(ctx context.Context, kid string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin tx to activate signing key: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && rbErr != sql.ErrTxDone {
			slog.Error("failed to rollback signing-key activation transaction", "error", rbErr)
		}
	}()

	if _, err = tx.ExecContext(ctx,
		`UPDATE oauth_signing_keys SET active = false, rotated_at = CURRENT_TIMESTAMP
		 WHERE active AND kid <> $1`, kid); err != nil {
		return fmt.Errorf("failed to deactivate prior signing keys: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE oauth_signing_keys SET active = true WHERE kid = $1`, kid)
	if err != nil {
		return fmt.Errorf("failed to activate signing key: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read affected rows activating signing key: %w", err)
	}
	if affected == 0 {
		return repositories.ErrOAuthSigningKeyNotFound
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit signing key activation: %w", err)
	}
	return nil
}

func scanSigningKey(row rowScanner) (*models.OAuthSigningKey, error) {
	var key models.OAuthSigningKey
	var rotatedAt sql.NullTime
	if err := row.Scan(
		&key.KID, &key.Algorithm, &key.PrivateKeyEncrypted, &key.PublicJWK,
		&key.Active, &key.CreatedAt, &rotatedAt,
	); err != nil {
		return nil, err
	}
	if rotatedAt.Valid {
		key.RotatedAt = &rotatedAt.Time
	}
	return &key, nil
}
