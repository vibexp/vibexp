package services

import (
	"context"
	"fmt"

	"github.com/vibexp/vibexp/internal/models"
)

// Admin suspend/reactivate (#454).
//
// The endpoints are the easy half of this feature; the enforcement in the auth
// middleware is the security-critical half. What lives here is the two guards
// that stop an instance locking itself out.

// ErrAdminSuspendSelf is returned when an admin targets their own account.
// The handler maps it to 409.
type ErrAdminSuspendSelf struct{}

func (e *ErrAdminSuspendSelf) Error() string {
	return "an instance admin cannot suspend their own account"
}

// ErrAdminSuspendInstanceAdmin is returned when the target's email is in the
// auth.instance_admins config allowlist. The handler maps it to 409.
type ErrAdminSuspendInstanceAdmin struct {
	Email string
}

func (e *ErrAdminSuspendInstanceAdmin) Error() string {
	return fmt.Sprintf("%s is a configured instance admin and cannot be suspended", e.Email)
}

// InstanceAdminPredicate reports whether an email is a config-listed instance
// admin. It is the same predicate instanceAdminMiddleware gates the admin
// surface with (config.Config.IsInstanceAdmin), injected rather than imported so
// the service does not depend on the config package.
type InstanceAdminPredicate func(email string) bool

// SuspendUser blocks an account at every authentication entry point.
//
// Guards, in order:
//  1. unknown id → (nil, nil), which the handler maps to 404;
//  2. self-suspension → *ErrAdminSuspendSelf;
//  3. config-listed instance admin → *ErrAdminSuspendInstanceAdmin.
//
// Because auth.instance_admins is CONFIG rather than data, guard 3 also means an
// operator can always recover from a lockout by editing config — there is no
// state in the database that can permanently remove admin access.
//
// Suspending an already-suspended account is a no-op that still returns the
// current detail (idempotent).
func (s *AdminService) SuspendUser(
	ctx context.Context, actingAdminID, targetID string, isInstanceAdmin InstanceAdminPredicate,
) (*models.AdminUserDetail, error) {
	target, err := s.adminRepo.GetUserDetail(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, nil
	}

	if targetID == actingAdminID {
		return nil, &ErrAdminSuspendSelf{}
	}
	if isInstanceAdmin != nil && isInstanceAdmin(target.Email) {
		return nil, &ErrAdminSuspendInstanceAdmin{Email: target.Email}
	}

	return s.setUserStatus(ctx, targetID, models.UserStatusSuspended)
}

// ReactivateUser restores a suspended account. There is no self/admin guard
// here: granting access back can never lock an instance out, and an admin
// reactivating themselves is exactly the recovery path an operator wants.
// Reactivating an already-active account is a no-op (idempotent).
func (s *AdminService) ReactivateUser(
	ctx context.Context, targetID string,
) (*models.AdminUserDetail, error) {
	target, err := s.adminRepo.GetUserDetail(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, nil
	}

	return s.setUserStatus(ctx, targetID, models.UserStatusActive)
}

// setUserStatus writes the new status and re-reads the detail so the caller
// always returns the persisted state rather than an optimistic local edit.
func (s *AdminService) setUserStatus(
	ctx context.Context, targetID, status string,
) (*models.AdminUserDetail, error) {
	updated, err := s.adminRepo.UpdateUserStatus(ctx, targetID, status)
	if err != nil {
		return nil, err
	}
	if !updated {
		// The row vanished between the read above and this write.
		return nil, nil
	}
	return s.adminRepo.GetUserDetail(ctx, targetID)
}
