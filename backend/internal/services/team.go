package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/logging"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// TeamService implements the TeamServiceInterface
type TeamService struct {
	teamRepo       repositories.TeamRepository
	teamMemberRepo repositories.TeamMemberRepository
	userRepo       repositories.UserRepository
	authz          AuthorizationServiceInterface
	logger         *slog.Logger
}

// NewTeamService creates a new TeamService
func NewTeamService(
	teamRepo repositories.TeamRepository,
	teamMemberRepo repositories.TeamMemberRepository,
	userRepo repositories.UserRepository,
	authz AuthorizationServiceInterface,
	logger *slog.Logger,
) *TeamService {
	if logger == nil {
		logger = logging.New(logging.Config{})
	}
	return &TeamService{
		teamRepo:       teamRepo,
		teamMemberRepo: teamMemberRepo,
		userRepo:       userRepo,
		authz:          authz,
		logger:         logger,
	}
}

// CreateDefaultTeam creates a default team for a user
func (s *TeamService) CreateDefaultTeam(ctx context.Context, userID string) (*models.Team, error) {
	now := time.Now()
	team := &models.Team{
		OwnerID:     userID,
		Name:        "Private Workspace",
		Slug:        "private-workspace",
		Description: "Your personal workspace for individual projects and resources",
		IsPersonal:  true, // Default teams are personal workspaces
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.teamRepo.Create(ctx, team); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-service",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create default team")
		return nil, fmt.Errorf("failed to create default team: %w", err)
	}

	// Add owner to team_members table
	member := &models.TeamMember{
		TeamID:    team.ID,
		UserID:    userID,
		Role:      models.TeamMemberRoleOwner,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.teamMemberRepo.Create(ctx, member); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-service",
			"user_id", userID,
			"team_id", team.ID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create team member")
		return nil, fmt.Errorf("failed to create team member: %w", err)
	}

	// Update user's default_team_id
	if err := s.userRepo.UpdateDefaultTeamID(ctx, userID, team.ID); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-service",
			"user_id", userID,
			"team_id", team.ID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to update user's default team ID")

		// Don't return error here - the team was created successfully
	}

	s.logger.With(
		"service", "vibexp-api",
		"component", "team-service",
		"user_id", userID,
		"team_id", team.ID,
	).
		Info("Default team created successfully")

	return team, nil
}

// GetTeamByOwnerID retrieves the team owned by a user
func (s *TeamService) GetTeamByOwnerID(ctx context.Context, ownerID string) (*models.Team, error) {
	team, err := s.teamRepo.GetByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get team by owner ID: %w", err)
	}
	return team, nil
}

// CreateTeam creates a new team for a user
func (s *TeamService) CreateTeam(
	ctx context.Context, userID string, req *models.CreateTeamRequest,
) (*models.Team, error) {
	// Generate slug from name
	slug := generateSlug(req.Name)

	// Check if slug already exists for this owner
	existingTeam, err := s.teamRepo.GetByOwnerAndSlug(ctx, userID, slug)
	if err == nil && existingTeam != nil {
		// Slug exists, append a suffix to make it unique
		slug = makeSlugUnique(ctx, s.teamRepo, userID, slug)
	}

	now := time.Now()
	team := &models.Team{
		OwnerID:     userID,
		Name:        req.Name,
		Slug:        slug,
		Description: req.Description,
		IsPersonal:  false, // Manually created teams are not personal workspaces
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.teamRepo.Create(ctx, team); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-service",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create team")
		return nil, fmt.Errorf("failed to create team: %w", err)
	}

	// Add owner to team_members table
	member := &models.TeamMember{
		TeamID:    team.ID,
		UserID:    userID,
		Role:      models.TeamMemberRoleOwner,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.teamMemberRepo.Create(ctx, member); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-service",
			"user_id", userID,
			"team_id", team.ID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to create team member")
		return nil, fmt.Errorf("failed to create team member: %w", err)
	}

	s.logger.With(
		"service", "vibexp-api",
		"component", "team-service",
		"user_id", userID,
		"team_id", team.ID,
	).
		Info("Team created successfully")

	return team, nil
}

// authorizeTeam loads the team and checks that the caller's role grants perm.
//
// It replaces the former verifyTeamOwnership, which keyed off the
// teams.owner_id column and so admitted only the owner. team_members.role is
// now the sole authority (epic #220): owner_id is referential data, kept in
// sync by TransferOwnership. Callers pass the permission the operation needs,
// which is what lets team.update admit admins while team.delete stays
// owner-only — the matrix decides, not this function.
//
// The returned team carries the caller's Role populated, as before, because
// callers marshal it back to the client.
func (s *TeamService) authorizeTeam(
	ctx context.Context, userID, teamID string, perm authz.Permission,
) (*models.Team, error) {
	team, err := s.teamRepo.GetByID(ctx, teamID)
	if err != nil {
		return nil, ErrTeamNotFound
	}

	// Authorize rather than Can: it hands back the role it already resolved, so
	// populating team.Role below costs no second lookup.
	role, err := s.authz.Authorize(ctx, userID, teamID, perm)
	if err != nil {
		return nil, err
	}
	team.Role = string(role)

	return team, nil
}

// GetTeam retrieves a team by ID, verifying membership (read-only operation)
func (s *TeamService) GetTeam(ctx context.Context, userID, teamID string) (*models.Team, error) {
	team, err := s.teamRepo.GetByID(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("team not found")
	}

	// Verify user is a member of this team
	member, err := s.teamMemberRepo.GetByTeamAndUser(ctx, team.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("team not found")
	}

	// Set user's role in this team
	team.Role = string(member.Role)

	return team, nil
}

// UpdateTeam updates an existing team
func (s *TeamService) UpdateTeam(
	ctx context.Context, userID, teamID string, req *models.UpdateTeamRequest,
) (*models.Team, error) {
	// Load the team and authorize: updating team details is Admin+ (epic #220).
	team, err := s.authorizeTeam(ctx, userID, teamID, authz.TeamUpdate)
	if err != nil {
		return nil, err
	}

	// Update fields if provided
	if req.Name != nil && *req.Name != "" {
		team.Name = *req.Name

		// Regenerate slug if name changed
		newSlug := generateSlug(*req.Name)
		if newSlug != team.Slug {
			// Slugs are unique per OWNER, so uniqueness must be checked against
			// team.OwnerID, not the caller: now that admins may update a team,
			// the caller is often not the owner, and checking their namespace
			// would both miss real collisions and invent phantom ones.
			existingTeam, slugErr := s.teamRepo.GetByOwnerAndSlug(ctx, team.OwnerID, newSlug)
			if slugErr == nil && existingTeam != nil && existingTeam.ID != team.ID {
				newSlug = makeSlugUnique(ctx, s.teamRepo, team.OwnerID, newSlug)
			}
			team.Slug = newSlug
		}
	}

	if req.Description != nil {
		team.Description = *req.Description
	}

	team.UpdatedAt = time.Now()

	if err := s.teamRepo.Update(ctx, team); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-service",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to update team")
		return nil, fmt.Errorf("failed to update team: %w", err)
	}

	s.logger.With(
		"service", "vibexp-api",
		"component", "team-service",
		"user_id", userID,
		"team_id", teamID,
	).
		Info("Team updated successfully")

	return team, nil
}

// DeleteTeam deletes a team, with protection for default and personal teams.
// nolint:funlen // DeleteTeam is a linear sequence of independent guard checks
// (ownership, personal-workspace, default-team, member validation) followed by
// the delete; splitting it would scatter the deletion preconditions and hurt
// readability more than the length costs.
func (s *TeamService) DeleteTeam(ctx context.Context, userID, teamID string) error {
	// 1. Get existing team and authorize: deleting a team stays Owner-only.
	team, err := s.authorizeTeam(ctx, userID, teamID, authz.TeamDelete)
	if err != nil {
		return err
	}

	// 2. Check if this is a personal workspace
	if team.IsPersonal {
		return NewCannotDeletePersonalWorkspaceError(teamID)
	}

	// 3. Check if this is the user's default team
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-service",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get user")
		return fmt.Errorf("failed to verify default team: %w", err)
	}

	if user.DefaultTeamID != nil && *user.DefaultTeamID == team.ID {
		return fmt.Errorf("cannot delete default team")
	}

	// Open-source build: teams have no paid subscription, so there are no
	// billing-related deletion guards. Deletion proceeds straight to member
	// validation.

	// 4. Check for multiple members (must remove all members first except owner)
	members, err := s.teamMemberRepo.GetByTeamID(ctx, teamID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-service",
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Warn("Failed to get team members for deletion validation")
	}

	memberCount := len(members)
	if memberCount > 1 {
		return NewTeamHasMembersError(teamID, memberCount)
	}

	// 7. Delete team members (should only be owner at this point)
	if err == nil {
		for _, member := range members {
			if delErr := s.teamMemberRepo.Delete(ctx, teamID, member.UserID); delErr != nil {
				s.logger.With(
					"service", "vibexp-api",
					"component", "team-service",
					"team_id", teamID,
					"user_id", member.UserID,
					"error", fmt.Sprintf("%+v", delErr),
				).Warn("Failed to delete team member")
			}
		}
	}

	// Delete the team. The repo scopes the delete by owner_id, so pass the
	// team's owner rather than the caller: they are the same user while
	// team.delete is owner-only, but relying on that coincidence would turn a
	// future matrix change into a silent no-op delete.
	if err := s.teamRepo.Delete(ctx, team.OwnerID, teamID); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-service",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to delete team")
		return fmt.Errorf("failed to delete team: %w", err)
	}

	s.logger.With(
		"service", "vibexp-api",
		"component", "team-service",
		"user_id", userID,
		"team_id", teamID,
	).
		Info("Team deleted successfully")

	return nil
}

// IsUserMemberOfTeam checks if a user is a member of a specific team
func (s *TeamService) IsUserMemberOfTeam(ctx context.Context, userID, teamID string) (bool, error) {
	member, err := s.teamMemberRepo.GetByTeamAndUser(ctx, teamID, userID)
	if err != nil {
		// Only return false without error for "not found" case
		if errors.Is(err, repositories.ErrTeamMemberNotFound) {
			return false, nil
		}
		// Propagate actual database errors
		return false, err
	}
	return member != nil, nil
}

// GetTeamMembers retrieves all members of a team with detailed user information
func (s *TeamService) GetTeamMembers(
	ctx context.Context, userID, teamID string, page, pageSize int,
) (*models.TeamMembersListResponse, error) {
	// Verify user has access to the team
	team, err := s.GetTeam(ctx, userID, teamID)
	if err != nil {
		return nil, err
	}

	// Get team members
	members, err := s.teamMemberRepo.GetByTeamID(ctx, teamID)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-service",
			"user_id", userID,
			"team_id", teamID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to get team members")
		return nil, fmt.Errorf("failed to get team members: %w", err)
	}

	// Fetch user details for each member
	memberDetails := s.fetchMemberDetails(ctx, members, team.OwnerID == userID)

	// Apply pagination
	paginatedDetails := s.paginateMembers(memberDetails, page, pageSize)

	return &models.TeamMembersListResponse{
		Members:    paginatedDetails,
		TotalCount: len(memberDetails),
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

// fetchMemberDetails fetches user details for each team member
func (s *TeamService) fetchMemberDetails(
	ctx context.Context, members []models.TeamMember, includeInvitationStatus bool,
) []models.TeamMemberDetail {
	memberDetails := make([]models.TeamMemberDetail, 0, len(members))
	for _, member := range members {
		user, userErr := s.userRepo.GetByID(ctx, member.UserID)
		if userErr != nil {
			s.logger.With(
				"service", "vibexp-api",
				"component", "team-service",
				"user_id", member.UserID,
				"error", fmt.Sprintf("%+v", userErr),
			).Warn("Failed to get user details for team member")
			continue // Skip this member if user not found
		}

		detail := models.TeamMemberDetail{
			UserID:   member.UserID,
			Email:    user.Email,
			Name:     user.Name,
			Role:     string(member.Role),
			JoinedAt: member.CreatedAt.Format(time.RFC3339),
		}

		// If user is owner, include invitation status
		if includeInvitationStatus {
			// For now, we'll mark all as "accepted" since they're in team_members table
			status := "accepted"
			detail.InvitationStatus = &status
		}

		memberDetails = append(memberDetails, detail)
	}
	return memberDetails
}

// paginateMembers applies pagination to member details
func (s *TeamService) paginateMembers(
	memberDetails []models.TeamMemberDetail, page, pageSize int,
) []models.TeamMemberDetail {
	totalCount := len(memberDetails)
	start := (page - 1) * pageSize
	end := start + pageSize

	switch {
	case start > totalCount:
		return []models.TeamMemberDetail{}
	case end > totalCount:
		return memberDetails[start:]
	default:
		return memberDetails[start:end]
	}
}

// UpdateMemberRole changes a member's role between member and admin.
//
// Ownership is deliberately not reachable here: a team has exactly one owner
// and it moves only through TransferOwnership, so newRole is restricted to
// member|admin and the current owner's row is untouchable — otherwise an admin
// could demote the owner and take over the team.
func (s *TeamService) UpdateMemberRole(
	ctx context.Context, userID, teamID, targetUserID string, newRole models.TeamMemberRole,
) (*models.TeamMemberDetail, error) {
	if newRole != models.TeamMemberRoleMember && newRole != models.TeamMemberRoleAdmin {
		return nil, ErrInvalidMemberRole
	}

	team, err := s.authorizeTeam(ctx, userID, teamID, authz.MemberRoleUpdate)
	if err != nil {
		return nil, err
	}

	if team.OwnerID == targetUserID {
		return nil, ErrCannotChangeOwnerRole
	}

	if updateErr := s.teamMemberRepo.UpdateRole(ctx, teamID, targetUserID, newRole); updateErr != nil {
		if errors.Is(updateErr, repositories.ErrTeamMemberNotFound) {
			return nil, updateErr
		}
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-service",
			"user_id", userID,
			"team_id", teamID,
			"member_id", targetUserID,
			"error", fmt.Sprintf("%+v", updateErr),
		).Error("Failed to update team member role")
		return nil, fmt.Errorf("failed to update team member role: %w", updateErr)
	}

	member, err := s.teamMemberRepo.GetByTeamAndUser(ctx, teamID, targetUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to load updated team member: %w", err)
	}

	user, err := s.userRepo.GetByID(ctx, targetUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to load updated team member's user: %w", err)
	}

	s.logger.With(
		"service", "vibexp-api",
		"component", "team-service",
		"user_id", userID,
		"team_id", teamID,
		"member_id", targetUserID,
		"new_role", string(newRole),
	).Info("Team member role updated successfully")

	return &models.TeamMemberDetail{
		UserID:   member.UserID,
		Email:    user.Email,
		Name:     user.Name,
		Role:     string(member.Role),
		JoinedAt: member.CreatedAt.Format(time.RFC3339),
	}, nil
}

// TransferOwnership hands the team to another member, demoting the caller to
// admin. Owner-only per the matrix; the repository applies all three writes in
// one transaction.
func (s *TeamService) TransferOwnership(
	ctx context.Context, userID, teamID, newOwnerID string,
) (*models.Team, error) {
	team, err := s.authorizeTeam(ctx, userID, teamID, authz.OwnershipTransfer)
	if err != nil {
		return nil, err
	}

	// A personal workspace has exactly one member by construction, so there is
	// nobody to transfer to and the workspace must stay with its user.
	if team.IsPersonal {
		return nil, NewPersonalWorkspaceError(teamID)
	}

	if newOwnerID == team.OwnerID {
		return nil, ErrAlreadyTeamOwner
	}

	// Verify the target is a member before touching anything: the repository
	// would also catch this, but only after opening a transaction, and this
	// gives the handler a clean 404 instead of a rollback path.
	if _, err := s.teamMemberRepo.GetByTeamAndUser(ctx, teamID, newOwnerID); err != nil {
		if errors.Is(err, repositories.ErrTeamMemberNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to verify new owner membership: %w", err)
	}

	if err := s.teamRepo.TransferOwnership(ctx, teamID, team.OwnerID, newOwnerID); err != nil {
		if errors.Is(err, repositories.ErrTeamNotFound) || errors.Is(err, repositories.ErrTeamMemberNotFound) {
			return nil, err
		}
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-service",
			"user_id", userID,
			"team_id", teamID,
			"new_owner_id", newOwnerID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to transfer team ownership")
		return nil, fmt.Errorf("failed to transfer team ownership: %w", err)
	}

	s.logger.With(
		"service", "vibexp-api",
		"component", "team-service",
		"user_id", userID,
		"team_id", teamID,
		"new_owner_id", newOwnerID,
	).Info("Team ownership transferred successfully")

	// Reflect the post-transfer state: the caller is now an admin.
	team.OwnerID = newOwnerID
	team.Role = string(models.TeamMemberRoleAdmin)

	return team, nil
}

// RemoveTeamMember removes a member from a team
func (s *TeamService) RemoveTeamMember(ctx context.Context, userID, teamID, memberUserID string) error {
	// Load the team and authorize: removing members is Admin+ (epic #220).
	team, err := s.authorizeTeam(ctx, userID, teamID, authz.MemberRemove)
	if err != nil {
		return err
	}

	// Cannot remove the owner — this is what keeps an admin from removing the
	// one role above them.
	if team.OwnerID == memberUserID {
		return ErrCannotRemoveTeamOwner
	}

	// Remove the member
	if err := s.teamMemberRepo.Delete(ctx, teamID, memberUserID); err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-service",
			"user_id", userID,
			"team_id", teamID,
			"member_id", memberUserID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to remove team member")
		return fmt.Errorf("failed to remove team member: %w", err)
	}

	s.logger.With(
		"service", "vibexp-api",
		"component", "team-service",
		"user_id", userID,
		"team_id", teamID,
		"member_id", memberUserID,
	).Info("Team member removed successfully")

	return nil
}

// ListTeams retrieves all teams where user is owner or member with pagination
func (s *TeamService) ListTeams(
	ctx context.Context, userID string, page, pageSize int,
) (*models.TeamListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	teams, totalCount, err := s.teamRepo.ListByUserID(ctx, userID, pageSize, offset)
	if err != nil {
		s.logger.With(
			"service", "vibexp-api",
			"component", "team-service",
			"user_id", userID,
			"error", fmt.Sprintf("%+v", err),
		).Error("Failed to list teams")
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}

	// Enrich each team with user's role and member count
	for i := range teams {
		// Get user's role in this team
		member, err := s.teamMemberRepo.GetByTeamAndUser(ctx, teams[i].ID, userID)
		if err != nil {
			s.logger.With(
				"service", "vibexp-api",
				"component", "team-service",
				"team_id", teams[i].ID,
				"user_id", userID,
				"error", fmt.Sprintf("%+v", err),
			).Warn("Failed to get team member role, setting to member")
			teams[i].Role = string(models.TeamMemberRoleMember)
		} else {
			teams[i].Role = string(member.Role)
		}

		// Get member count for this team
		members, err := s.teamMemberRepo.GetByTeamID(ctx, teams[i].ID)
		if err != nil {
			s.logger.With(
				"service", "vibexp-api",
				"component", "team-service",
				"team_id", teams[i].ID,
				"error", fmt.Sprintf("%+v", err),
			).Warn("Failed to get team members count, setting to 0")
			teams[i].MemberCount = 0
		} else {
			teams[i].MemberCount = len(members)
		}
	}

	return &models.TeamListResponse{
		Teams:      teams,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

// generateSlug creates a URL-friendly slug from a name
func generateSlug(name string) string {
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "-")
	// Remove special characters, keep only alphanumeric and hyphens
	var result strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	slug = result.String()
	// Remove consecutive hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	// Trim leading and trailing hyphens
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "team"
	}
	return slug
}

// makeSlugUnique appends a numeric suffix to make the slug unique
func makeSlugUnique(
	ctx context.Context, repo repositories.TeamRepository, ownerID, baseSlug string,
) string {
	for i := 1; i <= 100; i++ {
		newSlug := fmt.Sprintf("%s-%d", baseSlug, i)
		_, err := repo.GetByOwnerAndSlug(ctx, ownerID, newSlug)
		if err != nil {
			// Error means slug doesn't exist, we can use it
			return newSlug
		}
	}
	// Fallback: append timestamp
	return fmt.Sprintf("%s-%d", baseSlug, time.Now().Unix())
}

// GetTeamStats returns team-wide resource counts for the team analytics page.
// Team membership is validated by the caller (the handler) before this runs, so
// this is a thin pass-through to the repository.
func (s *TeamService) GetTeamStats(ctx context.Context, teamID string) (*models.TeamStatsResponse, error) {
	stats, err := s.teamRepo.GetTeamStats(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get team stats: %w", err)
	}
	return stats, nil
}

// GetTeamResourceCreationMetrics returns sparse per-day creation counts per
// resource type (prompts, artifacts, blueprints, memories, projects) for the
// team, counting rows created at or after `since`. The handler zero-fills the
// result into a continuous daily series. Team membership is validated by the
// caller before this runs.
func (s *TeamService) GetTeamResourceCreationMetrics(
	ctx context.Context, teamID string, since time.Time,
) ([]models.TeamResourceCreationCount, error) {
	counts, err := s.teamRepo.GetTeamResourceCreationMetrics(ctx, teamID, since)
	if err != nil {
		return nil, fmt.Errorf("failed to get team resource creation metrics: %w", err)
	}
	return counts, nil
}

// GetTeamFeedCreationMetrics returns sparse per-day creation counts for feeds and
// feed_items belonging to the team, counting rows created at or after `since`. The
// handler zero-fills the result into a continuous daily series. Team membership is
// validated by the caller before this runs.
func (s *TeamService) GetTeamFeedCreationMetrics(
	ctx context.Context, teamID string, since time.Time,
) ([]models.TeamFeedCreationCount, error) {
	counts, err := s.teamRepo.GetTeamFeedCreationMetrics(ctx, teamID, since)
	if err != nil {
		return nil, fmt.Errorf("failed to get team feed creation metrics: %w", err)
	}
	return counts, nil
}
