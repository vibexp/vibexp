package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// APIAccessRepository implements repositories.APIAccessRepository for
// PostgreSQL.
type APIAccessRepository struct {
	db *database.DB
}

// NewAPIAccessRepository creates a new APIAccessRepository.
func NewAPIAccessRepository(db *database.DB) repositories.APIAccessRepository {
	return &APIAccessRepository{db: db}
}

// revokeAllAPIAccess is ONE statement so the revocation is atomic without a
// transaction: either the cut-off is set and every row is gone, or nothing
// changed. The user id is passed twice because the OAuth tables store it as
// text (subject) while users and api_keys key on uuid, and a placeholder can
// have only one type.
//
// The cut-off is returned from the UPDATE, so no row means no such user; the
// deletes then matched nothing either, a subject being a user id.
const revokeAllAPIAccess = `
	WITH cutoff AS (
		UPDATE users SET api_tokens_valid_after = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING api_tokens_valid_after
	), keys AS (
		DELETE FROM api_keys WHERE user_id = $1 RETURNING 1
	), access_tokens AS (
		DELETE FROM oauth_access_tokens WHERE subject = $2 RETURNING 1
	), refresh_tokens AS (
		DELETE FROM oauth_refresh_tokens WHERE subject = $2 RETURNING 1
	), codes AS (
		DELETE FROM oauth_authorization_codes WHERE subject = $2 RETURNING 1
	), pkce AS (
		DELETE FROM oauth_pkce_sessions WHERE subject = $2 RETURNING 1
	), login_sessions AS (
		DELETE FROM oauth_login_sessions WHERE user_id = $1 RETURNING 1
	)
	SELECT
		(SELECT api_tokens_valid_after FROM cutoff),
		(SELECT count(*) FROM keys),
		(SELECT count(*) FROM access_tokens) + (SELECT count(*) FROM refresh_tokens)
			+ (SELECT count(*) FROM codes) + (SELECT count(*) FROM pkce)
			+ (SELECT count(*) FROM login_sessions)
`

// RevokeAll implements repositories.APIAccessRepository.
func (r *APIAccessRepository) RevokeAll(ctx context.Context, userID string) (*models.APIAccessRevocation, error) {
	var (
		cutoff sql.NullTime
		result models.APIAccessRevocation
	)
	err := r.db.QueryRowContext(ctx, revokeAllAPIAccess, userID, userID).
		Scan(&cutoff, &result.APIKeys, &result.OAuthGrants)
	if err != nil {
		return nil, fmt.Errorf("failed to revoke API access: %w", err)
	}
	if !cutoff.Valid {
		return nil, repositories.ErrUserNotFound
	}
	result.TokensValidAfter = cutoff.Time
	return &result, nil
}
