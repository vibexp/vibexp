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
}

// CreateUser creates a user and triggers the same provisioning self-signup gets.
//
// Returns *ErrAdminUserEmailTaken when the email is taken — both from the
// pre-check and from the users_email_key unique violation, so a race between two
// concurrent creates still yields 409 rather than a 500.
func (s *AdminService) CreateUser(
	ctx context.Context, req AdminUserCreateRequest,
) (*models.AdminUserDetail, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	if s.userRepo == nil || s.eventPublisher == nil {
		return nil, errors.New("admin user creation is not wired: user repository or event publisher missing")
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

	if pubErr := s.provisionNewUser(ctx, user); pubErr != nil {
		return nil, pubErr
	}

	return s.adminRepo.GetUserDetail(ctx, user.ID)
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
