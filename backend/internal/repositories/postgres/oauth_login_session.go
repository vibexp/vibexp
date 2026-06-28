package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// OAuthLoginSessionRepository implements repositories.OAuthLoginSessionRepository for PostgreSQL.
type OAuthLoginSessionRepository struct {
	db *database.DB
}

// NewOAuthLoginSessionRepository creates a new OAuthLoginSessionRepository.
func NewOAuthLoginSessionRepository(db *database.DB) repositories.OAuthLoginSessionRepository {
	return &OAuthLoginSessionRepository{db: db}
}

// Create persists a federated-login stash.
func (r *OAuthLoginSessionRepository) Create(ctx context.Context, s *models.OAuthLoginSession) error {
	query := `
		INSERT INTO oauth_login_sessions (id, authorize_query, provider, idp_state, expires_at)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := r.db.ExecContext(ctx, query, s.ID, s.AuthorizeQuery, s.Provider, s.IDPState, s.ExpiresAt)
	if err != nil {
		return fmt.Errorf("failed to create oauth login session: %w", err)
	}
	return nil
}

// Get returns a non-expired login session or ErrOAuthLoginSessionNotFound.
func (r *OAuthLoginSessionRepository) Get(ctx context.Context, id string) (*models.OAuthLoginSession, error) {
	query := `
		SELECT id, authorize_query, provider, idp_state, user_id, created_at, expires_at
		FROM oauth_login_sessions WHERE id = $1 AND expires_at > CURRENT_TIMESTAMP`

	var s models.OAuthLoginSession
	var userID sql.NullString
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&s.ID, &s.AuthorizeQuery, &s.Provider, &s.IDPState, &userID, &s.CreatedAt, &s.ExpiresAt,
	)
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get oauth login session: %w", err),
			repositories.ErrOAuthLoginSessionNotFound)
	}
	if userID.Valid {
		s.UserID = &userID.String
	}
	return &s, nil
}

// AttachUser records the resolved user id after the IdP callback succeeds.
func (r *OAuthLoginSessionRepository) AttachUser(ctx context.Context, id, userID string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE oauth_login_sessions SET user_id = $2 WHERE id = $1`, id, userID)
	if err != nil {
		return fmt.Errorf("failed to attach user to oauth login session: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read affected rows attaching user: %w", err)
	}
	if affected == 0 {
		return repositories.ErrOAuthLoginSessionNotFound
	}
	return nil
}

// Delete removes a login session by id.
func (r *OAuthLoginSessionRepository) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM oauth_login_sessions WHERE id = $1`, id); err != nil {
		return fmt.Errorf("failed to delete oauth login session: %w", err)
	}
	return nil
}

// DeleteExpired purges sessions past their expiry and returns the count removed.
func (r *OAuthLoginSessionRepository) DeleteExpired(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM oauth_login_sessions WHERE expires_at <= CURRENT_TIMESTAMP`)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired oauth login sessions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read affected rows deleting expired sessions: %w", err)
	}
	return n, nil
}
