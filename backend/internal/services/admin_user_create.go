package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/pkg/events"
)

// Admin user creation (#462).
//
// # Why this publishes an event instead of creating the team itself
//
// A new user is not just a `users` row. TeamCreationListener consumes
// `user.created` and provisions the "Private Workspace" personal team, the owner
// row in team_members, users.default_team_id, and a default project.
// TeamService.CreateDefaultTeam has NO other production caller — that listener is
// the only route. So an admin create path that writes the user row directly
// produces an account with no workspace and no project: broken from its first
// sign-in and invisible until someone tries to use it.
//
// Reusing the event also keeps the two paths from drifting: the listener stays
// the single definition of "what a new user gets".
//
// # The asynchrony this has to live with
//
// EventManager.Publish is asynchronous — it hands the event to a buffered
// channel and returns, and a consumer goroutine dispatches it. Two consequences:
//
//   - The 201 can be sent before the personal team exists. That matches
//     self-signup exactly, so it is parity rather than a regression, and the
//     operation description says so.
//   - A FULL channel makes Publish drop the event and return an error. AuthService
//     merely logs that, which means sign-up can silently create an unprovisioned
//     user. The admin path deliberately does not inherit that: a publish failure
//     removes the just-created row and fails the request, so an admin never
//     believes they created a working account when they did not. At that instant
//     no team or project exists yet, so there is nothing to cascade.

// ErrAdminUserEmailNotAllowed is returned when an active access allowlist does
// not admit the email: the account could never sign in (#1434).
type ErrAdminUserEmailNotAllowed struct {
	Email string
}

func (e *ErrAdminUserEmailNotAllowed) Error() string {
	return fmt.Sprintf("%s is not permitted by the access allowlist", e.Email)
}

// ErrAdminUserEmailTaken is returned when the email already belongs to a user.
// The handler maps it to 409.
type ErrAdminUserEmailTaken struct {
	Email string
}

func (e *ErrAdminUserEmailTaken) Error() string {
	return fmt.Sprintf("a user with email %s already exists", e.Email)
}

// AdminUserCreateRequest is the validated input for CreateUser.
type AdminUserCreateRequest struct {
	Email       string
	Name        string
	IDPProvider *string
	// Teams are existing, non-personal teams to add the user to (#1436).
	Teams []AdminTeamAssignment
}

// AdminTeamAssignment is one team a created user joins, with a role.
type AdminTeamAssignment struct {
	TeamID string
	Role   models.TeamMemberRole
}

// ErrAdminUserTeamInvalid is returned when a team named in a create request
// does not exist, is a personal workspace, or is named twice, or the role is
// not one a member can be given. Nothing was created. Index is the position of
// the offending entry.
type ErrAdminUserTeamInvalid struct {
	Index  int
	Reason string
}

func (e *ErrAdminUserTeamInvalid) Error() string {
	return fmt.Sprintf("teams[%d]: %s", e.Index, e.Reason)
}

// CreateUser creates a user and triggers the same provisioning self-signup gets.
//
// Returns *ErrAdminUserEmailTaken when the email is taken — both from the
// pre-check and from the users_email_key unique violation, so a race between two
// concurrent creates still yields 409 rather than a 500.
func (s *AdminService) CreateUser(
	ctx context.Context, req AdminUserCreateRequest,
) (*models.AdminUserDetail, error) {
	email := models.NormalizeEmail(req.Email)

	if s.userRepo == nil || s.eventPublisher == nil {
		return nil, errors.New("admin user creation is not wired: user repository or event publisher missing")
	}

	// The allowlist is the hard boundary: an account whose address it rejects
	// could never sign in, so creating it would hand the admin a dead account.
	if err := s.rejectEmailOutsideAllowlist(ctx, email); err != nil {
		return nil, err
	}
	// Teams are checked before anything is written, so a bad one creates
	// nothing.
	if err := s.checkTeamAssignments(ctx, req.Teams); err != nil {
		return nil, err
	}

	existing, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil && !errors.Is(err, repositories.ErrUserNotFound) {
		return nil, fmt.Errorf("failed to check for an existing user: %w", err)
	}
	if existing != nil {
		return nil, &ErrAdminUserEmailTaken{Email: email}
	}

	now := time.Now()
	user := &models.User{
		Email:       email,
		Name:        strings.TrimSpace(req.Name),
		IDPProvider: req.IDPProvider,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	// Status is deliberately NOT set here: UserRepository.Create does not include
	// the column in its INSERT, so assigning it would be silently dropped and read
	// as if it did something. The users.status column defaults to 'active'
	// (migration 011), which is what a newly created account should be.
	//
	// GoogleID and IDPSubject stay nil too: an admin-created account has no IdP
	// subject until its owner first signs in through the configured provider.

	if createErr := s.userRepo.Create(ctx, user); createErr != nil {
		if errors.Is(createErr, repositories.ErrUserEmailTaken) {
			return nil, &ErrAdminUserEmailTaken{Email: email}
		}
		return nil, fmt.Errorf("failed to create user: %w", createErr)
	}

	// Memberships go in before user.created is published: if one fails, the
	// row is removed (its memberships cascade) before anything has been
	// provisioned for it, the same compensation as a failed publish.
	if joinErr := s.joinTeamsOrRollBack(ctx, user.ID, req.Teams); joinErr != nil {
		return nil, joinErr
	}

	if pubErr := s.provisionNewUser(ctx, user); pubErr != nil {
		return nil, pubErr
	}

	return s.adminRepo.GetUserDetail(ctx, user.ID)
}

// checkTeamAssignments refuses, before anything is written, a team that does
// not exist, a personal workspace, a team named twice, or an unknown role.
func (s *AdminService) checkTeamAssignments(ctx context.Context, assignments []AdminTeamAssignment) error {
	if len(assignments) == 0 {
		return nil
	}
	if s.teams == nil || s.members == nil {
		return errors.New("admin user creation cannot assign teams: team repositories are not wired")
	}
	seen := make(map[string]bool, len(assignments))
	for i, a := range assignments {
		if a.Role != models.TeamMemberRoleMember && a.Role != models.TeamMemberRoleAdmin {
			return &ErrAdminUserTeamInvalid{Index: i, Reason: "the role must be member or admin"}
		}
		if seen[a.TeamID] {
			return &ErrAdminUserTeamInvalid{Index: i, Reason: "the team is listed more than once"}
		}
		seen[a.TeamID] = true
		if err := s.checkTeamJoinable(ctx, i, a.TeamID); err != nil {
			return err
		}
	}
	return nil
}

// checkTeamJoinable refuses a team that does not exist or is a personal
// workspace; i is the entry's position, for the error.
func (s *AdminService) checkTeamJoinable(ctx context.Context, i int, teamID string) error {
	team, err := s.teams.GetByID(ctx, teamID)
	if errors.Is(err, repositories.ErrTeamNotFound) || (err == nil && team == nil) {
		return &ErrAdminUserTeamInvalid{Index: i, Reason: "no such team"}
	}
	if err != nil {
		return fmt.Errorf("failed to read team %s: %w", teamID, err)
	}
	if team.IsPersonal {
		return &ErrAdminUserTeamInvalid{Index: i, Reason: "a personal workspace cannot be joined"}
	}
	return nil
}

// joinTeamsOrRollBack adds the new user to its teams and, if any membership
// cannot be added, removes the user row again (its memberships cascade).
func (s *AdminService) joinTeamsOrRollBack(
	ctx context.Context, userID string, assignments []AdminTeamAssignment,
) error {
	joinErr := s.addTeamMemberships(ctx, userID, assignments)
	if joinErr == nil {
		return nil
	}
	if delErr := s.userRepo.DeleteByID(ctx, userID); delErr != nil {
		return fmt.Errorf("failed to add the new user to its teams (%w) and failed to roll back user %s: %w",
			joinErr, userID, delErr)
	}
	return fmt.Errorf("failed to add the new user to its teams: %w", joinErr)
}

// addTeamMemberships adds the user to each team with its role.
func (s *AdminService) addTeamMemberships(ctx context.Context, userID string, assignments []AdminTeamAssignment) error {
	now := time.Now()
	for _, a := range assignments {
		member := &models.TeamMember{TeamID: a.TeamID, UserID: userID, Role: a.Role, CreatedAt: now, UpdatedAt: now}
		if err := s.members.Create(ctx, member); err != nil {
			return fmt.Errorf("add to team %s: %w", a.TeamID, err)
		}
	}
	return nil
}

// rejectEmailOutsideAllowlist returns *ErrAdminUserEmailNotAllowed when an
// ACTIVE access allowlist does not admit email. Root instance admins are exempt,
// as at sign-in. An allowlist that cannot be evaluated fails the request.
func (s *AdminService) rejectEmailOutsideAllowlist(ctx context.Context, email string) error {
	if s.allowlist == nil {
		return nil
	}
	allowed, _, err := s.allowlist.IsEmailAllowed(ctx, email)
	if err != nil {
		return fmt.Errorf("failed to evaluate the access allowlist: %w", err)
	}
	if !allowed {
		return &ErrAdminUserEmailNotAllowed{Email: email}
	}
	return nil
}

// provisionNewUser publishes user.created and, if that fails, removes the row it
// was created for — see the file header on why this does not just log.
func (s *AdminService) provisionNewUser(ctx context.Context, user *models.User) error {
	event := events.NewUserCreatedEvent(user.ID, user.Email, user.Name, user.CreatedAt)
	publishErr := s.eventPublisher.Publish(ctx, event)
	if publishErr == nil {
		return nil
	}

	// Compensate: without the event nothing will ever provision this user's
	// workspace, so leaving the row behind would hand the admin a broken account.
	// Nothing has cascaded yet, so the row deletes cleanly.
	if delErr := s.userRepo.DeleteByID(ctx, user.ID); delErr != nil {
		return fmt.Errorf(
			"failed to publish user.created (%w) and failed to roll back user %s: %w",
			publishErr, user.ID, delErr,
		)
	}
	return fmt.Errorf("failed to provision the new user: %w", publishErr)
}
