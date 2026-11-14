package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
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

// userSessionColumns are the columns scanUserSession reads, in its order.
const userSessionColumns = `id_hash, user_id, provider, auth_method, authenticated_at,
			access_token_enc, refresh_token_enc, access_expires_at, last_refreshed_at,
			created_at, expires_at`

// lockUnexpiredSession reads an unexpired session row and locks it until the
// transaction ends.
const lockUnexpiredSession = `SELECT ` + userSessionColumns + `
		FROM sessions WHERE id_hash = $1 AND expires_at > CURRENT_TIMESTAMP FOR UPDATE`

// GetByIDHash returns an unexpired session or ErrUserSessionNotFound.
func (r *UserSessionRepository) GetByIDHash(ctx context.Context, idHash []byte) (*models.UserSession, error) {
	query := `SELECT ` + userSessionColumns + `
		FROM sessions WHERE id_hash = $1 AND expires_at > CURRENT_TIMESTAMP`
	s, err := scanUserSession(r.db.QueryRowContext(ctx, query, idHash))
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get session: %w", err), repositories.ErrUserSessionNotFound)
	}
	return s, nil
}

// scanUserSession scans one row of userSessionColumns.
func scanUserSession(row *sql.Row) (*models.UserSession, error) {
	var (
		s                            models.UserSession
		provider, access, refresh    sql.NullString
		authenticatedAt, refreshedAt sql.NullTime
	)
	err := row.Scan(
		&s.IDHash, &s.UserID, &provider, &s.AuthMethod, &authenticatedAt,
		&access, &refresh, &s.AccessExpiresAt, &refreshedAt,
		&s.CreatedAt, &s.ExpiresAt,
	)
	if err != nil {
		return nil, err
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

// updateRefreshedSession is the statement UpdateTokens and RefreshLocked store
// rotated tokens with.
const updateRefreshedSession = `
		UPDATE sessions
		SET access_token_enc = $2, refresh_token_enc = $3, access_expires_at = $4,
			expires_at = $5, last_refreshed_at = CURRENT_TIMESTAMP
		WHERE id_hash = $1 AND expires_at > CURRENT_TIMESTAMP`

// UpdateTokens stores rotated tokens and the new expiries of an unexpired
// session, and stamps last_refreshed_at.
func (r *UserSessionRepository) UpdateTokens(ctx context.Context, s *models.UserSession) error {
	res, err := r.db.ExecContext(ctx, updateRefreshedSession,
		s.IDHash, nullableString(s.AccessTokenEnc), nullableString(s.RefreshTokenEnc), s.AccessExpiresAt, s.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update session tokens: %w", err)
	}
	return requireOneSessionRow(res, "updating session tokens")
}

// RefreshLocked locks the unexpired row of idHash, hands it to decide and
// applies the decision before committing. The lock is held while decide runs
// (it may call an identity provider), so a second caller for the same session
// waits and then sees the row the first one stored.
func (r *UserSessionRepository) RefreshLocked(
	ctx context.Context, idHash []byte, decide repositories.UserSessionRefreshFunc,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin the session refresh: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			slog.Error("Failed to roll back the session refresh", "error", rbErr)
		}
	}()

	row, err := scanUserSession(tx.QueryRowContext(ctx, lockUnexpiredSession, idHash))
	if err != nil {
		return mapNoRows(fmt.Errorf("failed to lock session: %w", err), repositories.ErrUserSessionNotFound)
	}

	decision, decideErr := decide(row)
	if decideErr != nil && !decision.Delete && decision.Update == nil {
		return decideErr
	}
	if err = applySessionRefresh(ctx, tx, idHash, decision); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit the session refresh: %w", err)
	}
	return decideErr
}

// applySessionRefresh applies a RefreshLocked decision inside tx.
func applySessionRefresh(ctx context.Context, tx *sql.Tx, idHash []byte, d repositories.UserSessionRefresh) error {
	switch {
	case d.Delete:
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = $1`, idHash); err != nil {
			return fmt.Errorf("failed to delete session: %w", err)
		}
	case d.Update != nil:
		u := d.Update
		res, err := tx.ExecContext(ctx, updateRefreshedSession,
			idHash, nullableString(u.AccessTokenEnc), nullableString(u.RefreshTokenEnc), u.AccessExpiresAt, u.ExpiresAt,
		)
		if err != nil {
			return fmt.Errorf("failed to update session tokens: %w", err)
		}
		return requireOneSessionRow(res, "updating session tokens")
	}
	return nil
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
