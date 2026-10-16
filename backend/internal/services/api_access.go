package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services/audit"
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
	// audit records each revocation (#1514); nil records nothing.
	audit audit.Recorder
}

var _ APIAccessServiceInterface = (*APIAccessService)(nil)

// APIAccessServiceOption customizes an APIAccessService at construction.
type APIAccessServiceOption func(*APIAccessService)

// WithAPIAccessAuditRecorder makes the service record each revocation in the
// audit log.
func WithAPIAccessAuditRecorder(recorder audit.Recorder) APIAccessServiceOption {
	return func(s *APIAccessService) { s.audit = recorder }
}

// NewAPIAccessService creates a new APIAccessService.
func NewAPIAccessService(
	repo repositories.APIAccessRepository, logger *slog.Logger, opts ...APIAccessServiceOption,
) *APIAccessService {
	s := &APIAccessService{repo: repo, logger: logger}
	for _, opt := range opts {
		opt(s)
	}
	return s
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
	// The only grant revocation there is: it stands in for a per-app
	// auth.mcp_grant.revoked until one exists. The actor is the caller from
	// the context; for a reset link or an account claim there is none.
	audit.SafeRecord(ctx, s.audit, audit.Event{
		Operation: models.AuditOpAuthAPIAccessRevoked, ResourceType: models.AuditResourceTypeUser, ResourceID: userID,
		Metadata: map[string]any{"api_keys": revoked.APIKeys, "oauth_grants": revoked.OAuthGrants},
	})
	return revoked, nil
}
