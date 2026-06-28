package postgres

import (
	"context"
	"fmt"

	"github.com/lib/pq"

	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// OAuthClientRepository implements repositories.OAuthClientRepository for PostgreSQL.
type OAuthClientRepository struct {
	db *database.DB
}

// NewOAuthClientRepository creates a new OAuthClientRepository.
func NewOAuthClientRepository(db *database.DB) repositories.OAuthClientRepository {
	return &OAuthClientRepository{db: db}
}

// Create persists a dynamically-registered OAuth client.
func (r *OAuthClientRepository) Create(ctx context.Context, client *models.OAuthClient) error {
	query := `
		INSERT INTO oauth_clients
			(id, secret_hash, redirect_uris, grant_types, response_types, scopes,
			 audience, public, token_endpoint_auth_method, client_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := r.db.ExecContext(ctx, query,
		client.ID, client.SecretHash,
		pq.Array(client.RedirectURIs), pq.Array(client.GrantTypes),
		pq.Array(client.ResponseTypes), pq.Array(client.Scopes), pq.Array(client.Audience),
		client.Public, client.TokenEndpointAuthMethod, client.ClientName,
	)
	if err != nil {
		return fmt.Errorf("failed to create oauth client: %w", err)
	}
	return nil
}

// GetByID returns a client by client_id or repositories.ErrOAuthClientNotFound.
func (r *OAuthClientRepository) GetByID(ctx context.Context, clientID string) (*models.OAuthClient, error) {
	query := `
		SELECT id, secret_hash, redirect_uris, grant_types, response_types, scopes,
		       audience, public, token_endpoint_auth_method, client_name, created_at
		FROM oauth_clients WHERE id = $1`

	var c models.OAuthClient
	err := r.db.QueryRowContext(ctx, query, clientID).Scan(
		&c.ID, &c.SecretHash,
		pq.Array(&c.RedirectURIs), pq.Array(&c.GrantTypes),
		pq.Array(&c.ResponseTypes), pq.Array(&c.Scopes), pq.Array(&c.Audience),
		&c.Public, &c.TokenEndpointAuthMethod, &c.ClientName, &c.CreatedAt,
	)
	if err != nil {
		return nil, mapNoRows(fmt.Errorf("failed to get oauth client: %w", err),
			repositories.ErrOAuthClientNotFound)
	}
	return &c, nil
}
