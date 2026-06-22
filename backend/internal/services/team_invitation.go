package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/logging"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// fallbackInviterName is used in the invitation email when the inviter has no
// display name and no usable email address — never expose the raw user ID to
// recipients (it looks like spam/phishing).
const fallbackInviterName = "A teammate"

// TeamInvitationService handles business logic for team invitations
type TeamInvitationService struct {
	invitationRepo repositories.TeamInvitationRepository
	teamRepo       repositories.TeamRepository
	teamMemberRepo repositories.TeamMemberRepository
	userRepo       repositories.UserRepository
	emailService   EmailServiceInterface
	cfg            *config.Config
	logger         *slog.Logger
}

// NewTeamInvitationService creates a new TeamInvitationService
func NewTeamInvitationService(
	invitationRepo repositories.TeamInvitationRepository,
	teamRepo repositories.TeamRepository,
	teamMemberRepo repositories.TeamMemberRepository,
	userRepo repositories.UserRepository,
	emailService EmailServiceInterface,
	cfg *config.Config,
	logger *slog.Logger,
) *TeamInvitationService {
	if logger == nil {
		logger = logging.New(logging.Config{})
	}
	return &TeamInvitationService{
		invitationRepo: invitationRepo,
		teamRepo:       teamRepo,
		teamMemberRepo: teamMemberRepo,
		userRepo:       userRepo,
		emailService:   emailService,
		cfg:            cfg,
		logger:         logger,
	}
}

// InviteMembers invites multiple users to join a team
//
//nolint:gocognit,gocyclo,funlen // Complex business logic with multiple validation checks
func (s *TeamInvitationService) InviteMembers(
	ctx context.Context,
	userID string,
	teamID string,
	emails []string,
	role models.TeamMemberRole,
) ([]*models.TeamInvitation, error) {
	// Verify user has permission to invite (owner/admin role)
	canManage, err := s.canManageTeam(ctx, userID, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to check permissions: %w", err)
	}
	if !canManage {
		return nil, fmt.Errorf("user does not have permission to invite members")
	}

	// Get team details for email
	team, err := s.teamRepo.GetByID(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get team: %w", err)
	}

	// Prevent invitations to personal workspaces
	if team.IsPersonal {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-invitation-service",
			"team_id", teamID,
			"user_id", userID,
		).Warn("Attempted to invite members to personal workspace")
		return nil, NewPersonalWorkspaceError(teamID)
	}

	// Open-source build: team membership is unlimited and requires no paid
	// subscription. Invitations proceed without any subscription or seat-limit
	// gating.

	// Resolve the inviter's display name once — it's the same for every email
	// in this batch, and the invitation email must show a human-readable name
	// (never the raw user UUID). Lookup failures degrade to a fallback string
	// so a transient DB blip does not block onboarding.
	inviterName := s.resolveInviterDisplayName(ctx, userID)

	// Create invitations
	invitations := make([]*models.TeamInvitation, 0, len(emails))
	duplicateEmails := make([]string, 0)
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour) // 7 days

	for _, email := range emails {
		// Check if user is already a member
		// First get the user by email
		user, err := s.userRepo.GetByEmail(ctx, email)
		if err == nil {
			// User exists, check if they're already a team member
			_, memberErr := s.teamMemberRepo.GetByTeamAndUser(ctx, teamID, user.ID)
			if memberErr == nil {
				s.logger.With(
					"service", "vibexp-api",
					"component", "team-invitation-service",
					"team_id", teamID,
					"email", email,
					"user_id", user.ID,
				).Warn("User already a member of team")
				duplicateEmails = append(duplicateEmails, email)
				continue
			}
		} else if !errors.Is(err, repositories.ErrUserNotFound) {
			// Database error (not "user not found"), log and skip this email
			s.logger.With(
				"service", "vibexp-api",
				"component", "team-invitation-service",
				"team_id", teamID,
				"email", email,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to check if user exists, skipping invitation for this email")
			continue
		}
		// If err contains "user not found", user doesn't exist yet, proceed with invitation

		// Check for existing pending invitation
		pendingInvitations, err := s.invitationRepo.GetPendingByEmail(ctx, email)
		if err != nil {
			// Database error when checking pending invitations
			s.logger.With(
				"service", "vibexp-api",
				"component", "team-invitation-service",
				"team_id", teamID,
				"email", email,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to check pending invitations, skipping invitation for this email")
			continue
		}

		// Check if pending invitation already exists for this team
		hasPendingForTeam := false
		for _, pending := range pendingInvitations {
			if pending.TeamID == teamID {
				s.logger.With(
					"service", "vibexp-api",
					"component", "team-invitation-service",
					"team_id", teamID,
					"email", email,
				).Warn("Pending invitation already exists for this email and team")
				hasPendingForTeam = true
				break
			}
		}
		if hasPendingForTeam {
			continue
		}

		// Generate secure token
		token, err := s.generateInvitationToken()
		if err != nil {
			return nil, fmt.Errorf("failed to generate token: %w", err)
		}

		invitation := &models.TeamInvitation{
			TeamID:       teamID,
			InviterID:    userID,
			InviteeEmail: email,
			Role:         role,
			Token:        token,
			Status:       models.InvitationStatusPending,
			ExpiresAt:    expiresAt,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		if err := s.invitationRepo.Create(ctx, invitation); err != nil {
			s.logger.With(
				"service", "vibexp-api",
				"component", "team-invitation-service",
				"team_id", teamID,
				"email", email,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to create invitation")
			return nil, fmt.Errorf("failed to create invitation: %w", err)
		}

		// Send invitation email
		if err := s.emailService.SendTeamInvitation(invitation, team.Name, inviterName); err != nil {
			s.logger.With(
				"service", "vibexp-api",
				"component", "team-invitation-service",
				"team_id", teamID,
				"email", email,
				"error", fmt.Sprintf("%+v", err),
			).Error("Failed to send invitation email")

			// Continue with other invitations even if email fails
		}

		invitations = append(invitations, invitation)

		s.logger.With(
			"service", "vibexp-api",
			"component", "team-invitation-service",
			"team_id", teamID,
			"email", email,
			"invitation_id", invitation.ID,
		).Info("Team invitation created successfully")
	}

	// If there are duplicate emails, return an error
	if len(duplicateEmails) > 0 {
		return invitations, NewDuplicateMembersError(duplicateEmails)
	}

	return invitations, nil
}

// InvitationDetails carries an invitation enriched with the team name and inviter info,
// suitable for rendering an invitee-facing landing page before acceptance.
type InvitationDetails struct {
	Invitation *models.TeamInvitation
	TeamName   string
	InvitedBy  *models.InviterInfo
}

// GetInvitationByToken loads a single invitation by its token and enriches it with the
// team name and inviter info for display.
//
// Effective state mapping:
//   - status == accepted/rejected/revoked  → *InvitationStateError
//   - status == pending and now > expires_at → *InvitationExpiredError
//   - repository GetByToken returns "not found" wrap → *InvitationNotFoundError
//
// Inviter lookup is best-effort: a failure does not abort the call (InvitedBy returns nil).
func (s *TeamInvitationService) GetInvitationByToken(
	ctx context.Context,
	token string,
) (*InvitationDetails, error) {
	invitation, err := s.invitationRepo.GetByToken(ctx, token)
	if err != nil {
		if errors.Is(err, repositories.ErrTeamInvitationNotFound) {
			return nil, NewInvitationNotFoundError(token)
		}
		return nil, fmt.Errorf("failed to get invitation by token: %w", err)
	}

	// Pre-flight: only invitations in pending+unexpired state are returnable.
	// Anything else maps to a typed error so the handler can pick the right
	// HTTP code without inspecting the raw status. Order: terminal states
	// (accepted/rejected/revoked) before expiry, so a fully-accepted
	// invitation never produces a misleading "expired" message.
	if invitation.Status != models.InvitationStatusPending {
		return nil, NewInvitationStateError(invitation.ID, invitation.Status)
	}
	if time.Now().After(invitation.ExpiresAt) {
		return nil, NewInvitationExpiredError(invitation.ID)
	}

	team, err := s.teamRepo.GetByID(ctx, invitation.TeamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get team for invitation %s: %w", invitation.ID, err)
	}

	details := &InvitationDetails{
		Invitation: invitation,
		TeamName:   team.Name,
	}

	inviter, err := s.userRepo.GetByID(ctx, invitation.InviterID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-invitation-service",
			"invitation_id", invitation.ID,
			"inviter_id", invitation.InviterID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to load inviter for invitation, continuing without inviter info")
		return details, nil
	}

	details.InvitedBy = &models.InviterInfo{
		ID:    inviter.ID,
		Name:  inviter.Name,
		Email: inviter.Email,
	}

	return details, nil
}

// GetPendingInvitations retrieves pending invitations for an email address
func (s *TeamInvitationService) GetPendingInvitations(
	ctx context.Context,
	email string,
) ([]*models.TeamInvitation, error) {
	invitations, err := s.invitationRepo.GetPendingByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending invitations: %w", err)
	}

	result := make([]*models.TeamInvitation, len(invitations))
	for i := range invitations {
		result[i] = &invitations[i]
	}

	return result, nil
}

// AcceptInvitation accepts a team invitation and returns the team ID
//
//nolint:funlen // Complex business logic with comprehensive security checks
func (s *TeamInvitationService) AcceptInvitation(
	ctx context.Context,
	token string,
	userID string,
) (string, error) {
	// Get invitation by token
	invitation, err := s.invitationRepo.GetByToken(ctx, token)
	if err != nil {
		return "", fmt.Errorf("invalid invitation token: %w", err)
	}

	// Get the authenticated user's email to verify authorization
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	// Verify the user's email matches the invitation
	if !strings.EqualFold(user.Email, invitation.InviteeEmail) {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-invitation-service",
			"user_email", user.Email,
			"invitee_email", invitation.InviteeEmail,
			"invitation_id", invitation.ID,
		).Warn("User attempted to accept invitation for different email")
		return "", fmt.Errorf("this invitation was sent to a different email address")
	}

	// Verify invitation is pending and not expired
	if invitation.Status != models.InvitationStatusPending {
		return "", fmt.Errorf("invitation is not pending")
	}
	if time.Now().After(invitation.ExpiresAt) {
		return "", fmt.Errorf("invitation has expired")
	}

	// Check if user is already a member
	_, err = s.teamMemberRepo.GetByTeamAndUser(ctx, invitation.TeamID, userID)
	if err == nil {
		return "", fmt.Errorf("user is already a member of this team")
	}

	// Create team member
	now := time.Now()
	member := &models.TeamMember{
		TeamID:    invitation.TeamID,
		UserID:    userID,
		Role:      invitation.Role,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.teamMemberRepo.Create(ctx, member); err != nil {
		return "", fmt.Errorf("failed to create team member: %w", err)
	}

	// Update invitation status
	if err := s.invitationRepo.UpdateStatus(ctx, invitation.ID, models.InvitationStatusAccepted); err != nil {
		return "", fmt.Errorf("failed to update invitation status: %w", err)
	}

	s.logger.With(
		"service", "vibexp-api",
		"component", "team-invitation-service",
		"team_id", invitation.TeamID,
		"user_id", userID,
		"invitation_id", invitation.ID,
	).Info("Team invitation accepted successfully")

	return invitation.TeamID, nil
}

// RejectInvitation rejects a team invitation
func (s *TeamInvitationService) RejectInvitation(ctx context.Context, token string, userID string) error {
	// Get invitation by token
	invitation, err := s.invitationRepo.GetByToken(ctx, token)
	if err != nil {
		return fmt.Errorf("invalid invitation token: %w", err)
	}

	// Get the authenticated user's email to verify authorization
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Verify the user's email matches the invitation
	if !strings.EqualFold(user.Email, invitation.InviteeEmail) {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-invitation-service",
			"user_email", user.Email,
			"invitee_email", invitation.InviteeEmail,
			"invitation_id", invitation.ID,
		).Warn("User attempted to reject invitation for different email")
		return fmt.Errorf("user is not authorized to reject this invitation")
	}

	// Verify invitation is pending
	if invitation.Status != models.InvitationStatusPending {
		return fmt.Errorf("invitation is not pending")
	}

	// Update invitation status
	if err := s.invitationRepo.UpdateStatus(ctx, invitation.ID, models.InvitationStatusRejected); err != nil {
		return fmt.Errorf("failed to update invitation status: %w", err)
	}

	s.logger.With(
		"service", "vibexp-api",
		"component", "team-invitation-service",
		"invitation_id", invitation.ID,
	).
		Info("Team invitation rejected")

	return nil
}

// RevokeInvitation revokes a team invitation
func (s *TeamInvitationService) RevokeInvitation(
	ctx context.Context,
	userID string,
	invitationID string,
) error {
	// Get invitation
	invitation, err := s.invitationRepo.GetByID(ctx, invitationID)
	if err != nil {
		return fmt.Errorf("invitation not found: %w", err)
	}

	// Verify user has permission to revoke
	canManage, err := s.canManageTeam(ctx, userID, invitation.TeamID)
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}
	if !canManage {
		return fmt.Errorf("user does not have permission to revoke invitations")
	}

	// Update invitation status
	if err := s.invitationRepo.UpdateStatus(ctx, invitationID, models.InvitationStatusRevoked); err != nil {
		return fmt.Errorf("failed to revoke invitation: %w", err)
	}

	s.logger.With(
		"service", "vibexp-api",
		"component", "team-invitation-service",
		"team_id", invitation.TeamID,
		"user_id", userID,
		"invitation_id", invitationID,
	).Info("Team invitation revoked")

	return nil
}

// GetTeamInvitations retrieves all invitations for a team
func (s *TeamInvitationService) GetTeamInvitations(
	ctx context.Context,
	userID string,
	teamID string,
) ([]models.TeamInvitation, error) {
	// Verify user has permission to view invitations
	canManage, err := s.canManageTeam(ctx, userID, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to check permissions: %w", err)
	}
	if !canManage {
		return nil, fmt.Errorf("user does not have permission to view invitations")
	}

	invitations, err := s.invitationRepo.GetByTeamID(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get team invitations: %w", err)
	}

	return invitations, nil
}

// canManageTeam checks if a user has permission to manage a team (owner or admin)
func (s *TeamInvitationService) canManageTeam(ctx context.Context, userID, teamID string) (bool, error) {
	member, err := s.teamMemberRepo.GetByTeamAndUser(ctx, teamID, userID)
	if err != nil {
		return false, err
	}

	return member.Role == models.TeamMemberRoleOwner || member.Role == models.TeamMemberRoleAdmin, nil
}

// resolveInviterDisplayName looks up the inviter and returns a human-readable
// name suitable for the invitation email. It falls back through:
//  1. user.Name (trimmed) when set
//  2. the local-part of user.Email (the segment before "@")
//  3. fallbackInviterName ("A teammate")
//
// A failed lookup degrades to the constant fallback so a transient DB blip
// never blocks an invitation — and never lets the raw user UUID reach the
// recipient.
func (s *TeamInvitationService) resolveInviterDisplayName(ctx context.Context, userID string) string {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-invitation-service",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to resolve inviter display name; using fallback")
		return fallbackInviterName
	}
	if user == nil {
		return fallbackInviterName
	}

	if name := strings.TrimSpace(user.Name); name != "" {
		return name
	}

	if local, _, ok := strings.Cut(strings.TrimSpace(user.Email), "@"); ok {
		if local = strings.TrimSpace(local); local != "" {
			return local
		}
	}

	return fallbackInviterName
}

// generateInvitationToken generates a secure random token for invitations
func (s *TeamInvitationService) generateInvitationToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
