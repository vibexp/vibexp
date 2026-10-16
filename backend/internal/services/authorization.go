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
	"github.com/vibexp/vibexp/internal/services/audit"
)

// AuthorizationServiceInterface is the single enforcement point for team
// permissions. It resolves the caller's role and consults the authz matrix;
// callers never inspect roles themselves.
type AuthorizationServiceInterface interface {
	// Can reports whether the user may perform perm within the team, returning
	// nil when allowed and an ErrPermissionDenied-wrapped error when not.
	Can(ctx context.Context, userID, teamID string, perm authz.Permission) error

	// Authorize is Can, additionally returning the caller's resolved role. Use
	// it when the caller must also echo the role back to the client, so the
	// role lookup happens once per request rather than twice.
	Authorize(ctx context.Context, userID, teamID string, perm authz.Permission) (models.TeamMemberRole, error)

	// IsMember reports whether the user belongs to the team at all, in any
	// role, returning nil when they do and an ErrPermissionDenied-wrapped error
	// when they do not.
	//
	// It is deliberately NOT a permission: membership carries no role dimension
	// and so is not a cell in the authz matrix, and adding a constant for it
	// would widen the published team `permissions` array. Use it only where the
	// operation itself authorizes on membership alone AND no tenancy middleware
	// already covers the team — today that is the SOURCE team of a cross-team
	// custom-types copy (#829), which appears in a request body rather than in
	// the URL and is therefore never seen by teamValidationMiddleware.
	IsMember(ctx context.Context, userID, teamID string) error

	// CanActOnResource is the own-vs-any variant of Can, for actions whose
	// policy depends on whether the caller created the resource (deletes, feed
	// moderation). It checks ownPerm when the caller owns the resource and
	// anyPerm otherwise.
	CanActOnResource(
		ctx context.Context,
		userID, teamID, resourceOwnerID string,
		ownPerm, anyPerm authz.Permission,
	) error

	// Allows reports whether the user may perform perm, WITHOUT recording a
	// denial in the audit log. Use it only where the answer picks a branch and
	// a "no" is not a refused request: UI gating, filtering, "can I show this
	// button". Can, Authorize and CanActOnResource record every denial
	// (authz.denied, #1515), so a probe made through them would fill the log
	// with denials nobody asked for. A database failure is still an error.
	Allows(ctx context.Context, userID, teamID string, perm authz.Permission) (bool, error)
}

// AuthorizationService implements AuthorizationServiceInterface against the
// team_members table, which is the source of truth for a user's role.
type AuthorizationService struct {
	teamMemberRepo repositories.TeamMemberRepository
	logger         *slog.Logger
	// audit records denials (#1515); nil records nothing.
	audit audit.Recorder
}

// AuthorizationServiceOption customizes an AuthorizationService at
// construction.
type AuthorizationServiceOption func(*AuthorizationService)

// WithAuthorizationAuditRecorder makes the service record every denial of Can,
// Authorize and CanActOnResource as authz.denied.
func WithAuthorizationAuditRecorder(recorder audit.Recorder) AuthorizationServiceOption {
	return func(s *AuthorizationService) { s.audit = recorder }
}

// NewAuthorizationService creates a new AuthorizationService
func NewAuthorizationService(
	teamMemberRepo repositories.TeamMemberRepository,
	logger *slog.Logger,
	opts ...AuthorizationServiceOption,
) *AuthorizationService {
	if logger == nil {
		logger = logging.New(logging.Config{})
	}
	s := &AuthorizationService{
		teamMemberRepo: teamMemberRepo,
		logger:         logger,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Can returns nil when the user's role in the team grants perm, and an error
// wrapping ErrPermissionDenied (naming the permission) when it does not.
//
// A genuine database failure is propagated (wrapped) and never reported as a
// denial: an unreachable database must surface as a 500, not a 403.
func (s *AuthorizationService) Can(ctx context.Context, userID, teamID string, perm authz.Permission) error {
	_, err := s.Authorize(ctx, userID, teamID, perm)
	return err
}

// Authorize checks perm and returns the caller's role, recording a denial.
func (s *AuthorizationService) Authorize(
	ctx context.Context, userID, teamID string, perm authz.Permission,
) (models.TeamMemberRole, error) {
	role, err := s.decide(ctx, userID, teamID, perm)
	if err != nil {
		if errors.Is(err, ErrPermissionDenied) {
			s.recordDenial(ctx, denial{userID: userID, teamID: teamID, perm: perm, role: role})
		}
		return "", err
	}
	return role, nil
}

// Allows implements AuthorizationServiceInterface: decide, with no recording.
func (s *AuthorizationService) Allows(
	ctx context.Context, userID, teamID string, perm authz.Permission,
) (bool, error) {
	_, err := s.decide(ctx, userID, teamID, perm)
	if errors.Is(err, ErrPermissionDenied) {
		return false, nil
	}
	return err == nil, err
}

// decide carries the logic every check shares and records nothing. On a
// denial it still returns the role that was refused ("" for a non-member), so
// the denial can name it; on success it returns the role.
func (s *AuthorizationService) decide(
	ctx context.Context, userID, teamID string, perm authz.Permission,
) (models.TeamMemberRole, error) {
	role, err := s.roleOf(ctx, userID, teamID)
	if err != nil {
		return "", err
	}

	if !authz.Allowed(role, perm) {
		return role, fmt.Errorf("%w: role %q may not perform %q", ErrPermissionDenied, role, perm)
	}

	return role, nil
}

// denial is one refused check, as authz.denied records it.
type denial struct {
	userID, teamID string
	perm           authz.Permission
	// role is the caller's role, "" when they are not a member of the team.
	role models.TeamMemberRole
	// resourceOwnerID is set for an own-vs-any check (CanActOnResource).
	resourceOwnerID string
}

// auditRoleNonMember is the role a denial names for a caller who is not a
// member of the team.
const auditRoleNonMember = "non_member"

// recordDenial records one denial: a single SafeRecord call on the hot path.
// It does no I/O of its own; the recorder resolves the actor's and the team's
// names off the request path. A database error is never a denial and never
// reaches here.
func (s *AuthorizationService) recordDenial(ctx context.Context, d denial) {
	role := string(d.role)
	if role == "" {
		role = auditRoleNonMember
	}
	metadata := map[string]any{"permission": string(d.perm), "role": role}
	if d.resourceOwnerID != "" {
		metadata["resource_owner_id"] = d.resourceOwnerID
	}
	userID := d.userID
	audit.SafeRecord(ctx, s.audit, audit.Event{
		Operation: models.AuditOpAuthzDenied, Outcome: models.AuditOutcomeDenied,
		Actor: &audit.Actor{UserID: &userID}, TeamID: d.teamID, Metadata: metadata,
	})
}

// IsMember resolves the user's role purely to establish that they have one.
// The role itself is discarded: every role is a member, so the matrix has
// nothing to add.
func (s *AuthorizationService) IsMember(ctx context.Context, userID, teamID string) error {
	_, err := s.roleOf(ctx, userID, teamID)
	return err
}

// CanActOnResource checks ownPerm when userID created the resource and anyPerm
// otherwise. resourceOwnerID is the resource's creator/owner user ID.
//
// It checks exactly one permission, so a denial records one authz.denied,
// naming the resource's owner.
func (s *AuthorizationService) CanActOnResource(
	ctx context.Context,
	userID, teamID, resourceOwnerID string,
	ownPerm, anyPerm authz.Permission,
) error {
	perm := anyPerm
	if userID == resourceOwnerID {
		perm = ownPerm
	}
	role, err := s.decide(ctx, userID, teamID, perm)
	if errors.Is(err, ErrPermissionDenied) {
		s.recordDenial(ctx, denial{
			userID: userID, teamID: teamID, perm: perm, role: role, resourceOwnerID: resourceOwnerID,
		})
	}
	return err
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
