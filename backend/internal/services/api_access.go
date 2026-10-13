package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// APIAccessServiceInterface revokes a user's non-browser access.
type APIAccessServiceInterface interface {
	// RevokeAPIAccess ends every way into the account that is not a browser
	// session: it deletes the user's API keys and stored MCP/CLI OAuth grants,
	// and makes access tokens that were already issued stop authenticating.
	//
	// It is for the moments an account changes hands or is recovered (an
	// account claim, a password reset with "also revoke API keys and connected
	// tools"). It is all-or-nothing, atomic, and safe to call twice; each call
	// means "everything up to now". Browser sessions are ended separately, with
	// AuthService.RevokeUserSessions.
	RevokeAPIAccess(ctx context.Context, userID string) (*models.APIAccessRevocation, error)
}

// APIAccessService implements APIAccessServiceInterface.
type APIAccessService struct {
	repo   repositories.APIAccessRepository
	logger *slog.Logger
}

var _ APIAccessServiceInterface = (*APIAccessService)(nil)

// NewAPIAccessService creates a new APIAccessService.
func NewAPIAccessService(repo repositories.APIAccessRepository, logger *slog.Logger) *APIAccessService {
	return &APIAccessService{repo: repo, logger: logger}
}

// RevokeAPIAccess implements APIAccessServiceInterface.
func (s *APIAccessService) RevokeAPIAccess(ctx context.Context, userID string) (*models.APIAccessRevocation, error) {
	revoked, err := s.repo.RevokeAll(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to revoke API access of user %q: %w", userID, err)
	}
	s.logger.With(
		"user_id", userID,
		"api_keys", revoked.APIKeys,
		"oauth_grants", revoked.OAuthGrants,
		"tokens_valid_after", revoked.TokensValidAfter,
	).Info("Revoked the user's API access")
	return revoked, nil
}
