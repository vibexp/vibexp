package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// auditResourceTypeUser is the audit resource type of an event about a user
// account.
const auditResourceTypeUser = "user"

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

// NewAPIAccessService creates a new APIAccessService. recorder may be nil.
func NewAPIAccessService(
	repo repositories.APIAccessRepository, logger *slog.Logger, recorder ...audit.Recorder,
) *APIAccessService {
	s := &APIAccessService{repo: repo, logger: logger}
	if len(recorder) > 0 {
		s.audit = recorder[0]
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
		Operation: models.AuditOpAuthAPIAccessRevoked, ResourceType: auditResourceTypeUser, ResourceID: userID,
		Metadata: map[string]any{"api_keys": revoked.APIKeys, "oauth_grants": revoked.OAuthGrants},
	})
	return revoked, nil
}
