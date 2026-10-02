package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// Instance admins (#1233, epic #1230, decision 12).
//
// An instance admin is either a ROOT admin, named in config.yaml's
// auth.instance_admins, or a DB-granted admin, a row in instance_admins (#1231).
// Root admins are the trust root: they cannot be suspended, deleted or revoked,
// and only they may grant or revoke DB admins. Because the root set is config
// rather than data, an operator can always recover admin access by editing
// config.

// ErrInstanceAdminNotRoot is returned when a grant or revoke is attempted by
// anyone other than a root admin (a DB-granted admin, a plain user, or an
// unknown actor).
type ErrInstanceAdminNotRoot struct{}

func (e *ErrInstanceAdminNotRoot) Error() string {
	return "only a root instance admin (auth.instance_admins) may grant or revoke instance admins"
}

// ErrInstanceAdminTargetIsRoot is returned when a grant or revoke targets a
// root admin, whose admin access comes from config and cannot be changed here.
type ErrInstanceAdminTargetIsRoot struct {
	Email string
}

func (e *ErrInstanceAdminTargetIsRoot) Error() string {
	return fmt.Sprintf("%s is a root instance admin (auth.instance_admins) and cannot be granted or revoked", e.Email)
}

// ErrInstanceAdminTargetInvalid is returned when a grant targets a user that
// does not exist or is suspended.
type ErrInstanceAdminTargetInvalid struct {
	UserID string
	Reason string
}

func (e *ErrInstanceAdminTargetInvalid) Error() string {
	return fmt.Sprintf("user %s cannot be granted instance admin: %s", e.UserID, e.Reason)
}

// ErrInstanceAdminNotGranted is returned when a revoke targets a user that
// holds no DB grant.
type ErrInstanceAdminNotGranted struct {
	UserID string
}

func (e *ErrInstanceAdminNotGranted) Error() string {
	return fmt.Sprintf("user %s is not a granted instance admin", e.UserID)
}

const (
	instanceAdminTargetUnknown   = "user not found"
	instanceAdminTargetSuspended = "user is suspended"
)

// InstanceAdminResolver answers who is an instance admin and lets a root admin
// grant or revoke DB admins.
type InstanceAdminResolver interface {
	// IsRootAdmin reports whether email is in auth.instance_admins. Matching is
	// case-insensitive and whitespace-trimmed; a blank email is never root.
	IsRootAdmin(email string) bool
	// RootAdminEmails returns the root admins' normalized emails, sorted.
	RootAdminEmails() []string
	// IsInstanceAdmin reports whether user is a root admin or a non-suspended
	// user holding a DB grant. Root admins are answered from config alone, so a
	// database error never locks them out; for anyone else the error is
	// returned and the caller must fail closed.
	IsInstanceAdmin(ctx context.Context, user *models.User) (bool, error)
	// GrantInstanceAdmin makes targetUserID a DB-granted admin and reports
	// whether it was newly granted: granting an existing admin is a no-op that
	// writes no audit entry. actingUserID must be a root admin.
	GrantInstanceAdmin(ctx context.Context, actingUserID, targetUserID string) (bool, error)
	// RevokeInstanceAdmin removes targetUserID's DB grant. actingUserID must be
	// a root admin.
	RevokeInstanceAdmin(ctx context.Context, actingUserID, targetUserID string) error
}

// InstanceAdminService implements InstanceAdminResolver. Grant lookups are not
// cached, so a revocation takes effect on the admin's next request.
type InstanceAdminService struct {
	rootAdmins map[string]struct{}
	grants     repositories.InstanceAdminRepository
	users      repositories.UserRepository
	logger     *slog.Logger
}

var _ InstanceAdminResolver = (*InstanceAdminService)(nil)

// NewInstanceAdminService creates the resolver. rootAdmins is
// auth.instance_admins; blank entries are ignored.
func NewInstanceAdminService(
	rootAdmins []string,
	grants repositories.InstanceAdminRepository,
	users repositories.UserRepository,
	logger *slog.Logger,
) *InstanceAdminService {
	roots := make(map[string]struct{}, len(rootAdmins))
	for _, email := range rootAdmins {
		if normalized := normalizeAdminEmail(email); normalized != "" {
			roots[normalized] = struct{}{}
		}
	}
	return &InstanceAdminService{rootAdmins: roots, grants: grants, users: users, logger: logger}
}

func normalizeAdminEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// IsRootAdmin implements InstanceAdminResolver.
func (s *InstanceAdminService) IsRootAdmin(email string) bool {
	normalized := normalizeAdminEmail(email)
	if normalized == "" {
		return false
	}
	_, ok := s.rootAdmins[normalized]
	return ok
}

// RootAdminEmails implements InstanceAdminResolver.
func (s *InstanceAdminService) RootAdminEmails() []string {
	emails := make([]string, 0, len(s.rootAdmins))
	for email := range s.rootAdmins {
		emails = append(emails, email)
	}
	slices.Sort(emails)
	return emails
}

// IsInstanceAdmin implements InstanceAdminResolver.
func (s *InstanceAdminService) IsInstanceAdmin(ctx context.Context, user *models.User) (bool, error) {
	if user == nil {
		return false, nil
	}
	if s.IsRootAdmin(user.Email) {
		return true, nil
	}
	if user.IsSuspended() {
		return false, nil
	}
	granted, err := s.grants.IsGranted(ctx, user.ID)
	if err != nil {
		return false, fmt.Errorf("failed to resolve instance admin grant: %w", err)
	}
	return granted, nil
}

// GrantInstanceAdmin implements InstanceAdminResolver.
func (s *InstanceAdminService) GrantInstanceAdmin(
	ctx context.Context, actingUserID, targetUserID string,
) (bool, error) {
	if err := s.requireRootActor(ctx, actingUserID); err != nil {
		return false, err
	}
	target, err := s.lookupUser(ctx, targetUserID)
	if err != nil {
		return false, err
	}
	if target == nil {
		return false, &ErrInstanceAdminTargetInvalid{UserID: targetUserID, Reason: instanceAdminTargetUnknown}
	}
	if s.IsRootAdmin(target.Email) {
		return false, &ErrInstanceAdminTargetIsRoot{Email: target.Email}
	}
	if target.IsSuspended() {
		return false, &ErrInstanceAdminTargetInvalid{UserID: targetUserID, Reason: instanceAdminTargetSuspended}
	}

	granted, err := s.grants.Grant(ctx, targetUserID, &actingUserID)
	if errors.Is(err, repositories.ErrUserNotFound) {
		// The target (or the actor) was deleted between the lookup and the write.
		return false, &ErrInstanceAdminTargetInvalid{UserID: targetUserID, Reason: instanceAdminTargetUnknown}
	}
	if err != nil {
		return false, fmt.Errorf("failed to grant instance admin: %w", err)
	}
	if granted {
		s.logger.Info("Instance admin granted", "user_id", targetUserID, "granted_by", actingUserID)
	}
	return granted, nil
}

// RevokeInstanceAdmin implements InstanceAdminResolver. A suspended DB admin
// can be revoked; an unknown user holds no grant.
func (s *InstanceAdminService) RevokeInstanceAdmin(
	ctx context.Context, actingUserID, targetUserID string,
) error {
	if err := s.requireRootActor(ctx, actingUserID); err != nil {
		return err
	}
	target, err := s.lookupUser(ctx, targetUserID)
	if err != nil {
		return err
	}
	if target == nil {
		return &ErrInstanceAdminNotGranted{UserID: targetUserID}
	}
	if s.IsRootAdmin(target.Email) {
		return &ErrInstanceAdminTargetIsRoot{Email: target.Email}
	}

	err = s.grants.Revoke(ctx, targetUserID, &actingUserID)
	if errors.Is(err, repositories.ErrInstanceAdminNotFound) {
		return &ErrInstanceAdminNotGranted{UserID: targetUserID}
	}
	if err != nil {
		return fmt.Errorf("failed to revoke instance admin: %w", err)
	}
	s.logger.Info("Instance admin revoked", "user_id", targetUserID, "revoked_by", actingUserID)
	return nil
}

// requireRootActor re-resolves the actor from the database by id, so the
// root-only rule never rests on anything the caller supplied beyond its id.
func (s *InstanceAdminService) requireRootActor(ctx context.Context, actingUserID string) error {
	actor, err := s.lookupUser(ctx, actingUserID)
	if err != nil {
		return err
	}
	if actor == nil || actor.IsSuspended() || !s.IsRootAdmin(actor.Email) {
		return &ErrInstanceAdminNotRoot{}
	}
	return nil
}

// lookupUser returns (nil, nil) for an unknown user.
func (s *InstanceAdminService) lookupUser(ctx context.Context, userID string) (*models.User, error) {
	user, err := s.users.GetByID(ctx, userID)
	if errors.Is(err, repositories.ErrUserNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to look up user %s: %w", userID, err)
	}
	return user, nil
}
