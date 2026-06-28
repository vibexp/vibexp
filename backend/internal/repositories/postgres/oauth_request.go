package postgres

import (
	"context"
	"fmt"

	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// OAuth request-backing tables. These are internal constants, never user input,
// so interpolating them into the (otherwise fully parameterized) statements is
// safe; the column shape is identical across all four.
const (
	oauthAuthorizationCodesTable = "oauth_authorization_codes"
	oauthAccessTokensTable       = "oauth_access_tokens"  // #nosec G101 -- table name, not a credential
	oauthRefreshTokensTable      = "oauth_refresh_tokens" // #nosec G101 -- table name, not a credential
	oauthPKCESessionsTable       = "oauth_pkce_sessions"
)

// oauthRequestRepository implements repositories.OAuthRequestRepository over one
// of the OAuth request tables (authorization codes, access tokens, refresh
// tokens, or PKCE sessions), which all share the same column layout.
type oauthRequestRepository struct {
	db    *database.DB
	table string
}

// NewOAuthCodeRepository returns the store for authorization codes.
func NewOAuthCodeRepository(db *database.DB) repositories.OAuthRequestRepository {
	return &oauthRequestRepository{db: db, table: oauthAuthorizationCodesTable}
}

// NewOAuthAccessTokenRepository returns the store for access-token sessions.
func NewOAuthAccessTokenRepository(db *database.DB) repositories.OAuthRequestRepository {
	return &oauthRequestRepository{db: db, table: oauthAccessTokensTable}
}

// NewOAuthRefreshTokenRepository returns the store for refresh-token sessions.
func NewOAuthRefreshTokenRepository(db *database.DB) repositories.OAuthRequestRepository {
	return &oauthRequestRepository{db: db, table: oauthRefreshTokensTable}
}

// NewOAuthPKCERepository returns the store for PKCE request sessions.
func NewOAuthPKCERepository(db *database.DB) repositories.OAuthRequestRepository {
	return &oauthRequestRepository{db: db, table: oauthPKCESessionsTable}
}

// Create persists a request session.
func (r *oauthRequestRepository) Create(ctx context.Context, req *models.OAuthRequest) error {
	// #nosec G201 -- r.table is an internal constant (one of the oauth*Table values), not user input.
	query := fmt.Sprintf(`
		INSERT INTO %s
			(signature, request_id, client_id, subject, requested_scope, granted_scope,
			 requested_audience, granted_audience, requested_at, form_data, session_data, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`, r.table)

	_, err := r.db.ExecContext(ctx, query,
		req.Signature, req.RequestID, req.ClientID, req.Subject,
		pq.Array(req.RequestedScope), pq.Array(req.GrantedScope),
		pq.Array(req.RequestedAudience), pq.Array(req.GrantedAudience),
		req.RequestedAt, req.FormData, req.SessionData, req.Active,
	)
	if err != nil {
		return fmt.Errorf("failed to create oauth request in %s: %w", r.table, err)
	}
	return nil
}

// Get returns the request session for a signature, including inactive rows so
// the caller can detect invalidated codes and rotated refresh tokens. A missing
// row yields repositories.ErrOAuthRequestNotFound.
func (r *oauthRequestRepository) Get(ctx context.Context, signature string) (*models.OAuthRequest, error) {
	// #nosec G201 -- r.table is an internal constant, not user input.
	query := fmt.Sprintf(`
		SELECT signature, request_id, client_id, subject, requested_scope, granted_scope,
		       requested_audience, granted_audience, requested_at, form_data, session_data, active
		FROM %s WHERE signature = $1`, r.table)

	var req models.OAuthRequest
	err := r.db.QueryRowContext(ctx, query, signature).Scan(
		&req.Signature, &req.RequestID, &req.ClientID, &req.Subject,
		pq.Array(&req.RequestedScope), pq.Array(&req.GrantedScope),
		pq.Array(&req.RequestedAudience), pq.Array(&req.GrantedAudience),
		&req.RequestedAt, &req.FormData, &req.SessionData, &req.Active,
	)
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get oauth request from %s: %w", r.table, err),
			repositories.ErrOAuthRequestNotFound)
	}
	return &req, nil
}

// Delete removes a request session by signature.
func (r *oauthRequestRepository) Delete(ctx context.Context, signature string) error {
	return r.exec(ctx, "delete", signature,
		// #nosec G201 -- internal constant table name.
		fmt.Sprintf("DELETE FROM %s WHERE signature = $1", r.table))
}

// Deactivate marks a single row inactive (code invalidation / refresh rotation).
func (r *oauthRequestRepository) Deactivate(ctx context.Context, signature string) error {
	return r.exec(ctx, "deactivate", signature,
		// #nosec G201 -- internal constant table name.
		fmt.Sprintf("UPDATE %s SET active = false WHERE signature = $1", r.table))
}

// DeactivateByRequestID marks every row sharing a request id inactive.
func (r *oauthRequestRepository) DeactivateByRequestID(ctx context.Context, requestID string) error {
	return r.exec(ctx, "deactivate by request id", requestID,
		// #nosec G201 -- internal constant table name.
		fmt.Sprintf("UPDATE %s SET active = false WHERE request_id = $1", r.table))
}

// DeleteByRequestID removes every row sharing a request id.
func (r *oauthRequestRepository) DeleteByRequestID(ctx context.Context, requestID string) error {
	return r.exec(ctx, "delete by request id", requestID,
		// #nosec G201 -- internal constant table name.
		fmt.Sprintf("DELETE FROM %s WHERE request_id = $1", r.table))
}

// exec runs a single-argument statement, treating zero affected rows as success
// (fosite invalidation/revocation are idempotent and may target missing rows).
func (r *oauthRequestRepository) exec(ctx context.Context, op, arg, query string) error {
	if _, err := r.db.ExecContext(ctx, query, arg); err != nil {
		return fmt.Errorf("failed to %s oauth request in %s: %w", op, r.table, err)
	}
	return nil
}
