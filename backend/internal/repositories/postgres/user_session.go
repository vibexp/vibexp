package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// UserSessionRepository implements repositories.UserSessionRepository for
// PostgreSQL over the sessions table (#1646).
type UserSessionRepository struct {
	db *database.DB
}

// NewUserSessionRepository creates a new UserSessionRepository.
func NewUserSessionRepository(db *database.DB) repositories.UserSessionRepository {
	return &UserSessionRepository{db: db}
}

// Create persists a new session row.
func (r *UserSessionRepository) Create(ctx context.Context, s *models.UserSession) error {
	query := `
		INSERT INTO sessions (
			id_hash, user_id, provider, auth_method, authenticated_at,
			access_token_enc, refresh_token_enc, access_expires_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := r.db.ExecContext(ctx, query,
		s.IDHash, s.UserID, nullableString(s.Provider), s.AuthMethod, nullIfZeroTime(s.AuthenticatedAt),
		nullableString(s.AccessTokenEnc), nullableString(s.RefreshTokenEnc), s.AccessExpiresAt, s.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

// GetByIDHash returns an unexpired session or ErrUserSessionNotFound.
func (r *UserSessionRepository) GetByIDHash(ctx context.Context, idHash []byte) (*models.UserSession, error) {
	query := `
		SELECT id_hash, user_id, provider, auth_method, authenticated_at,
			access_token_enc, refresh_token_enc, access_expires_at, last_refreshed_at,
			created_at, expires_at
		FROM sessions WHERE id_hash = $1 AND expires_at > CURRENT_TIMESTAMP`

	var (
		s                            models.UserSession
		provider, access, refresh    sql.NullString
		authenticatedAt, refreshedAt sql.NullTime
	)
	err := r.db.QueryRowContext(ctx, query, idHash).Scan(
		&s.IDHash, &s.UserID, &provider, &s.AuthMethod, &authenticatedAt,
		&access, &refresh, &s.AccessExpiresAt, &refreshedAt,
		&s.CreatedAt, &s.ExpiresAt,
	)
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get session: %w", err), repositories.ErrUserSessionNotFound)
	}
	s.Provider = provider.String
	s.AccessTokenEnc = access.String
	s.RefreshTokenEnc = refresh.String
	if authenticatedAt.Valid {
		s.AuthenticatedAt = authenticatedAt.Time
	}
	if refreshedAt.Valid {
		t := refreshedAt.Time
		s.LastRefreshedAt = &t
	}
	return &s, nil
}

// UpdateTokens stores rotated tokens and the new expiries of an unexpired
// session, and stamps last_refreshed_at.
func (r *UserSessionRepository) UpdateTokens(ctx context.Context, s *models.UserSession) error {
	query := `
		UPDATE sessions
		SET access_token_enc = $2, refresh_token_enc = $3, access_expires_at = $4,
			expires_at = $5, last_refreshed_at = CURRENT_TIMESTAMP
		WHERE id_hash = $1 AND expires_at > CURRENT_TIMESTAMP`
	res, err := r.db.ExecContext(ctx, query,
		s.IDHash, nullableString(s.AccessTokenEnc), nullableString(s.RefreshTokenEnc), s.AccessExpiresAt, s.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update session tokens: %w", err)
	}
	return requireOneSessionRow(res, "updating session tokens")
}

// UpdateExpiry moves the expiries of an unexpired session.
func (r *UserSessionRepository) UpdateExpiry(
	ctx context.Context, idHash []byte, accessExpiresAt, expiresAt time.Time,
) error {
	query := `
		UPDATE sessions SET access_expires_at = $2, expires_at = $3
		WHERE id_hash = $1 AND expires_at > CURRENT_TIMESTAMP`
	res, err := r.db.ExecContext(ctx, query, idHash, accessExpiresAt, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to update session expiry: %w", err)
	}
	return requireOneSessionRow(res, "updating session expiry")
}

// Delete removes a session row by id hash.
func (r *UserSessionRepository) Delete(ctx context.Context, idHash []byte) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = $1`, idHash); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// DeleteExpired purges sessions past their expiry and returns the count
// removed. It is one idempotent statement, so replicas running it at the same
// time only race to delete the same rows.
func (r *UserSessionRepository) DeleteExpired(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= CURRENT_TIMESTAMP`)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired sessions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read affected rows deleting expired sessions: %w", err)
	}
	return n, nil
}

// requireOneSessionRow maps an update that matched no row to
// ErrUserSessionNotFound.
func requireOneSessionRow(res sql.Result, what string) error {
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read affected rows %s: %w", what, err)
	}
	if affected == 0 {
		return repositories.ErrUserSessionNotFound
	}
	return nil
}

// nullIfZeroTime maps the zero time to SQL NULL.
func nullIfZeroTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: !t.IsZero()}
}
