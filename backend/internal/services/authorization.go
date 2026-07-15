package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/logging"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// AuthorizationServiceInterface is the single enforcement point for team
// permissions. It resolves the caller's role and consults the authz matrix;
// callers never inspect roles themselves.
type AuthorizationServiceInterface interface {
	// Can reports whether the user may perform perm within the team, returning
	// nil when allowed and an ErrPermissionDenied-wrapped error when not.
	Can(ctx context.Context, userID, teamID string, perm authz.Permission) error

	// CanActOnResource is the own-vs-any variant of Can, for actions whose
	// policy depends on whether the caller created the resource (deletes, feed
	// moderation). It checks ownPerm when the caller owns the resource and
	// anyPerm otherwise.
	CanActOnResource(
		ctx context.Context,
		userID, teamID, resourceOwnerID string,
		ownPerm, anyPerm authz.Permission,
	) error
}

// AuthorizationService implements AuthorizationServiceInterface against the
// team_members table, which is the source of truth for a user's role.
type AuthorizationService struct {
	teamMemberRepo repositories.TeamMemberRepository
	logger         *slog.Logger
}

// NewAuthorizationService creates a new AuthorizationService
func NewAuthorizationService(
	teamMemberRepo repositories.TeamMemberRepository,
	logger *slog.Logger,
) *AuthorizationService {
	if logger == nil {
		logger = logging.New(logging.Config{})
	}
	return &AuthorizationService{
		teamMemberRepo: teamMemberRepo,
		logger:         logger,
	}
}

// Can returns nil when the user's role in the team grants perm, and an error
// wrapping ErrPermissionDenied (naming the permission) when it does not.
//
// A genuine database failure is propagated (wrapped) and never reported as a
// denial: an unreachable database must surface as a 500, not a 403.
func (s *AuthorizationService) Can(ctx context.Context, userID, teamID string, perm authz.Permission) error {
	role, err := s.roleOf(ctx, userID, teamID)
	if err != nil {
		return err
	}

	if !authz.Allowed(role, perm) {
		return fmt.Errorf("%w: role %q may not perform %q", ErrPermissionDenied, role, perm)
	}

	return nil
}

// CanActOnResource checks ownPerm when userID created the resource and anyPerm
// otherwise. resourceOwnerID is the resource's creator/owner user ID.
func (s *AuthorizationService) CanActOnResource(
	ctx context.Context,
	userID, teamID, resourceOwnerID string,
	ownPerm, anyPerm authz.Permission,
) error {
	if userID == resourceOwnerID {
		return s.Can(ctx, userID, teamID, ownPerm)
	}
	return s.Can(ctx, userID, teamID, anyPerm)
}

// roleOf resolves the user's role within the team.
//
// A non-member is a denial, not a lookup failure — but a real database error
// must stay a database error, so the two are deliberately distinguished here
// rather than collapsed into a single "false".
func (s *AuthorizationService) roleOf(ctx context.Context, userID, teamID string) (models.TeamMemberRole, error) {
	member, err := s.teamMemberRepo.GetByTeamAndUser(ctx, teamID, userID)
	if err != nil {
		if errors.Is(err, repositories.ErrTeamMemberNotFound) {
			return "", fmt.Errorf("%w: user is not a member of the team", ErrPermissionDenied)
		}

		s.logger.With(
			"service", "vibexp-api",
			"component", "authorization-service",
			"team_id", teamID,
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to resolve team member role")

		return "", fmt.Errorf("failed to resolve team member role: %w", err)
	}

	return member.Role, nil
}
