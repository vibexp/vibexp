package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/vibexp/vibexp/internal/models"
)

// Admin user edit + guarded hard delete (#455).
//
// Delete is the epic's only destructive operation, and the database makes it
// far more destructive than it looks: teams_owner_id_fkey is ON DELETE CASCADE
// and 19 further constraints cascade FROM teams, so removing a user destroys
// every team they own and, for a shared team, every other member's data in it.
// The guards below are what stand between an admin's click and that outcome.

// ErrAdminDeleteSelf is returned when an admin targets their own account.
type ErrAdminDeleteSelf struct{}

func (e *ErrAdminDeleteSelf) Error() string {
	return "an instance admin cannot delete their own account"
}

// ErrAdminDeleteInstanceAdmin is returned when the target is a config-listed
// instance admin.
type ErrAdminDeleteInstanceAdmin struct {
	Email string
}

func (e *ErrAdminDeleteInstanceAdmin) Error() string {
	return fmt.Sprintf("%s is a configured instance admin and cannot be deleted", e.Email)
}

// ErrAdminDeleteBlocked is returned when the user owns shared teams with other
// members. It carries the full blocker list so the client can tell the admin
// exactly which teams need an ownership transfer first. NOTHING was deleted.
type ErrAdminDeleteBlocked struct {
	Blockers []models.AdminDeleteBlocker
}

func (e *ErrAdminDeleteBlocked) Error() string {
	names := make([]string, 0, len(e.Blockers))
	for _, b := range e.Blockers {
		names = append(names, b.TeamName)
	}
	return fmt.Sprintf(
		"user owns shared team(s) with other members (%s); transfer ownership before deleting",
		strings.Join(names, ", "),
	)
}

// UpdateUserName changes the only field an instance admin may edit and returns
// the refreshed detail. (nil, nil) means no such user — the handler 404s.
func (s *AdminService) UpdateUserName(
	ctx context.Context, targetID, name string,
) (*models.AdminUserDetail, error) {
	updated, err := s.adminRepo.UpdateUserName(ctx, targetID, name)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, nil
	}
	return s.adminRepo.GetUserDetail(ctx, targetID)
}

// DeleteUser hard-deletes a user after three guards, in order:
//
//  1. unknown id → (nil, nil), which the handler maps to 404;
//  2. self-deletion → *ErrAdminDeleteSelf;
//  3. config-listed instance admin → *ErrAdminDeleteInstanceAdmin;
//  4. owns shared teams with other members → *ErrAdminDeleteBlocked.
//
// Guard 4 is evaluated by the repository INSIDE the delete transaction rather
// than here, so a member joining one of those teams between the check and the
// delete cannot slip through — that race is exactly the case that would destroy
// someone else's data.
//
// Returns (true, nil) when the user was deleted, (false, nil) when there was no
// such user.
func (s *AdminService) DeleteUser(
	ctx context.Context, actingAdminID, targetID string, isInstanceAdmin InstanceAdminPredicate,
) (bool, error) {
	target, err := s.adminRepo.GetUserDetail(ctx, targetID)
	if err != nil {
		return false, err
	}
	if target == nil {
		return false, nil
	}

	if targetID == actingAdminID {
		return false, &ErrAdminDeleteSelf{}
	}
	if isInstanceAdmin != nil && isInstanceAdmin(target.Email) {
		return false, &ErrAdminDeleteInstanceAdmin{Email: target.Email}
	}

	blockers, found, err := s.adminRepo.DeleteUserIfUnblocked(ctx, targetID)
	if err != nil {
		return false, err
	}
	if len(blockers) > 0 {
		return false, &ErrAdminDeleteBlocked{Blockers: blockers}
	}
	return found, nil
}
