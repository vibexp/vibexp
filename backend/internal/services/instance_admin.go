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
	"github.com/vibexp/vibexp/internal/services/audit"
)

// Instance admins (#1233, epic #1230, decision 12).
//
// An instance admin is either a ROOT admin, named in config.yaml's
// auth.instance_admins, or a DB-granted admin, a row in instance_admins (#1231).
// Root admins are the trust root: they cannot be suspended, deleted or revoked,
// and only they may grant or revoke DB admins. Because the root set is config
// rather than data, an operator can always recover admin access by editing
// config.
//
// A third source is PROVIDER-SOURCED (#1644): a row in instance_provider_admins
// written by a sign-in provider's role mapping when the user's verified claims
// carry one of its admin values. Only the mapping writes those rows, so a manual
// grant or revoke refuses to touch a provider-sourced admin, and the mapping
// never touches a root admin or a manual grant. A provider-sourced admin holds
// no root powers, and the row grants nothing while its provider is disabled.
//
// The address alone is a RESERVATION, not the power (#1467): an account holds
// root admin power only while its email is in auth.instance_admins AND
// users.email_verified_at is set. Otherwise any path that creates an account
// for an address without proving the mailbox (an open instance trusting an
// unverified provider claim, a bearer link) would hand out instance-admin
// power to whoever got there first.

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

// ErrInstanceAdminTargetIsProviderSourced is returned when a grant or revoke
// targets a user whose admin status comes from a sign-in provider's role
// mapping (#1644), which only the mapping may change.
type ErrInstanceAdminTargetIsProviderSourced struct {
	UserID       string
	ProviderSlug string
}

func (e *ErrInstanceAdminTargetIsProviderSourced) Error() string {
	return fmt.Sprintf("user %s is an instance admin through the role mapping of sign-in provider %q, "+
		"which only that provider's role mapping can change", e.UserID, e.ProviderSlug)
}

// Why a provider-sourced admin was demoted: the audit metadata's reason.
const (
	// ProviderAdminDemotedNoMatch is a sign-in whose claims no longer match.
	ProviderAdminDemotedNoMatch = "no_match"
	// ProviderAdminDemotedMappingCleared is an admin clearing the mapping.
	ProviderAdminDemotedMappingCleared = "mapping_cleared"
	// ProviderAdminDemotedProviderDeleted is an admin deleting the provider.
	ProviderAdminDemotedProviderDeleted = "provider_deleted"
)

// IsUnknownUser reports whether the grant was refused because the target user
// does not exist (as opposed to existing but being suspended).
func (e *ErrInstanceAdminTargetInvalid) IsUnknownUser() bool {
	return e.Reason == instanceAdminTargetUnknown
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
	// It is the email-only RESERVATION check: use it to protect an address
	// before anyone owns it, never to grant or protect power (IsVerifiedRootAdmin).
	IsRootAdmin(email string) bool
	// IsVerifiedRootAdmin reports whether user holds root admin power: its
	// email is in auth.instance_admins AND its email_verified_at is set (#1467).
	// Every check that grants or protects power uses this, not IsRootAdmin.
	IsVerifiedRootAdmin(user *models.User) bool
	// RootAdminEmails returns the root admins' normalized emails, sorted.
	RootAdminEmails() []string
	// IsInstanceAdmin reports whether user is a verified root admin, or a
	// non-suspended user holding a DB grant or a provider-sourced row of an
	// enabled provider. Verified root admins are answered
	// from the user row and config alone, so a database error never locks them
	// out; for anyone else the error is returned and the caller must fail closed.
	IsInstanceAdmin(ctx context.Context, user *models.User) (bool, error)
	// RequireRootAdmin returns ErrInstanceAdminNotRoot unless userID names an
	// active root admin. Grant and revoke apply it themselves; a caller uses it
	// to refuse a non-root actor before doing anything on their behalf.
	RequireRootAdmin(ctx context.Context, userID string) error
	// GrantInstanceAdmin makes targetUserID a DB-granted admin and reports
	// whether it was newly granted: granting an existing admin is a no-op that
	// writes no audit entry. actingUserID must be a root admin.
	GrantInstanceAdmin(ctx context.Context, actingUserID, targetUserID string) (bool, error)
	// RevokeInstanceAdmin removes targetUserID's DB grant. actingUserID must be
	// a root admin.
	RevokeInstanceAdmin(ctx context.Context, actingUserID, targetUserID string) error
	// SyncProviderAdmin applies one evaluation of a provider's role mapping to
	// user: a non-nil matched records them as an admin of the provider, nil
	// removes that. Only a real transition is audited, with user as the actor.
	// It is the one entry point for sign-in and, later, token refresh (#1648).
	SyncProviderAdmin(ctx context.Context, user *models.User, providerID, providerSlug string, matched *string) error
	// RemoveProviderAdmins removes every provider-sourced admin of providerID,
	// auditing one demotion with reason per user; the actor comes from ctx.
	RemoveProviderAdmins(ctx context.Context, providerID, providerSlug, reason string) error
	// RecordProviderAdminDemotions audits one demotion with reason for each of
	// userIDs, whose rows a provider delete already removed by cascade; the
	// actor comes from ctx.
	RecordProviderAdminDemotions(ctx context.Context, providerSlug, reason string, userIDs []string)
}

// InstanceAdminService implements InstanceAdminResolver. Grant lookups are not
// cached, so a revocation takes effect on the admin's next request.
type InstanceAdminService struct {
	rootAdmins map[string]struct{}
	grants     repositories.InstanceAdminRepository
	users      repositories.UserRepository
	// providerAdmins holds the provider-sourced admins (#1644); nil means the
	// source is off and SyncProviderAdmin / RemoveProviderAdmins write nothing.
	providerAdmins repositories.InstanceProviderAdminRepository
	// audit records provider-sourced promotions and demotions; nil records
	// nothing.
	audit  audit.Recorder
	logger *slog.Logger
}

var _ InstanceAdminResolver = (*InstanceAdminService)(nil)

// InstanceAdminOption configures an InstanceAdminService.
type InstanceAdminOption func(*InstanceAdminService)

// WithProviderAdmins turns on the provider-sourced admin source (#1644),
// recording its promotions and demotions with recorder.
func WithProviderAdmins(
	repo repositories.InstanceProviderAdminRepository, recorder audit.Recorder,
) InstanceAdminOption {
	return func(s *InstanceAdminService) {
		s.providerAdmins = repo
		s.audit = recorder
	}
}

// NewInstanceAdminService creates the resolver. rootAdmins is
// auth.instance_admins; blank entries are ignored.
func NewInstanceAdminService(
	rootAdmins []string,
	grants repositories.InstanceAdminRepository,
	users repositories.UserRepository,
	logger *slog.Logger,
	opts ...InstanceAdminOption,
) *InstanceAdminService {
	roots := make(map[string]struct{}, len(rootAdmins))
	for _, email := range rootAdmins {
		if normalized := normalizeAdminEmail(email); normalized != "" {
			roots[normalized] = struct{}{}
		}
	}
	s := &InstanceAdminService{rootAdmins: roots, grants: grants, users: users, logger: logger}
	for _, opt := range opts {
		opt(s)
	}
	return s
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

// IsVerifiedRootAdmin implements InstanceAdminResolver.
func (s *InstanceAdminService) IsVerifiedRootAdmin(user *models.User) bool {
	return user != nil && user.EmailVerifiedAt != nil && s.IsRootAdmin(user.Email)
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
	if s.IsVerifiedRootAdmin(user) {
		return true, nil
	}
	if user.IsSuspended() {
		return false, nil
	}
	granted, err := s.grants.IsGranted(ctx, user.ID)
	if err != nil {
		return false, fmt.Errorf("failed to resolve instance admin grant: %w", err)
	}
	if granted || s.providerAdmins == nil {
		return granted, nil
	}
	sourced, err := s.providerAdmins.IsProviderAdmin(ctx, user.ID)
	if err != nil {
		return false, fmt.Errorf("failed to resolve provider-sourced instance admin: %w", err)
	}
	return sourced, nil
}

// providerSourcedRefusal returns ErrInstanceAdminTargetIsProviderSourced when
// userID holds a provider-sourced row of an enabled provider, and nil
// otherwise.
func (s *InstanceAdminService) providerSourcedRefusal(ctx context.Context, userID string) error {
	if s.providerAdmins == nil {
		return nil
	}
	rows, err := s.providerAdmins.ListByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to read provider-sourced instance admin: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	return &ErrInstanceAdminTargetIsProviderSourced{UserID: userID, ProviderSlug: rows[0].ProviderSlug}
}

// GrantInstanceAdmin implements InstanceAdminResolver.
func (s *InstanceAdminService) GrantInstanceAdmin(
	ctx context.Context, actingUserID, targetUserID string,
) (bool, error) {
	if err := s.RequireRootAdmin(ctx, actingUserID); err != nil {
		return false, err
	}
	target, err := s.lookupUser(ctx, targetUserID)
	if err != nil {
		return false, err
	}
	if target == nil {
		return false, &ErrInstanceAdminTargetInvalid{UserID: targetUserID, Reason: instanceAdminTargetUnknown}
	}
	if s.IsVerifiedRootAdmin(target) {
		return false, &ErrInstanceAdminTargetIsRoot{Email: target.Email}
	}
	if target.IsSuspended() {
		return false, &ErrInstanceAdminTargetInvalid{UserID: targetUserID, Reason: instanceAdminTargetSuspended}
	}
	// A provider-sourced admin is already an admin, and a grant beside the
	// mapping would outlive the claim the identity provider controls.
	if err = s.providerSourcedRefusal(ctx, targetUserID); err != nil {
		return false, err
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
	if err := s.RequireRootAdmin(ctx, actingUserID); err != nil {
		return err
	}
	target, err := s.lookupUser(ctx, targetUserID)
	if err != nil {
		return err
	}
	if target == nil {
		return &ErrInstanceAdminNotGranted{UserID: targetUserID}
	}
	if s.IsVerifiedRootAdmin(target) {
		return &ErrInstanceAdminTargetIsRoot{Email: target.Email}
	}
	// A provider-sourced admin with no manual grant has nothing a revoke may
	// remove; with one, the revoke removes just the grant and the mapping keeps
	// them an admin.
	if err = s.refuseProviderSourcedRevoke(ctx, targetUserID); err != nil {
		return err
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

// refuseProviderSourcedRevoke refuses a revoke of a provider-sourced admin who
// holds no manual grant.
func (s *InstanceAdminService) refuseProviderSourcedRevoke(ctx context.Context, userID string) error {
	refusal := s.providerSourcedRefusal(ctx, userID)
	var sourced *ErrInstanceAdminTargetIsProviderSourced
	if !errors.As(refusal, &sourced) {
		return refusal
	}
	granted, err := s.grants.IsGranted(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to resolve instance admin grant: %w", err)
	}
	if granted {
		return nil
	}
	return refusal
}

// SyncProviderAdmin implements InstanceAdminResolver.
func (s *InstanceAdminService) SyncProviderAdmin(
	ctx context.Context, user *models.User, providerID, providerSlug string, matched *string,
) error {
	if s.providerAdmins == nil || user == nil {
		return nil
	}
	actor := &audit.Actor{UserID: &user.ID, Email: user.Email, Name: user.Name}
	if matched != nil {
		inserted, err := s.providerAdmins.Upsert(ctx, user.ID, providerID, *matched)
		if err != nil {
			return err
		}
		if inserted {
			s.logger.Info("Provider-sourced instance admin promoted",
				"user_id", user.ID, "provider", providerSlug, "matched_value", *matched)
			s.recordProviderAdminChange(ctx, models.AuditOpAuthInstanceAdminPromoted, actor, user.ID, user.Email,
				map[string]any{"provider_slug": providerSlug, "matched_value": *matched})
		}
		return nil
	}
	deleted, err := s.providerAdmins.Delete(ctx, user.ID, providerID)
	if err != nil {
		return err
	}
	if deleted {
		s.logger.Info("Provider-sourced instance admin demoted",
			"user_id", user.ID, "provider", providerSlug, "reason", ProviderAdminDemotedNoMatch)
		s.recordProviderAdminChange(ctx, models.AuditOpAuthInstanceAdminDemoted, actor, user.ID, user.Email,
			map[string]any{"provider_slug": providerSlug, "reason": ProviderAdminDemotedNoMatch})
	}
	return nil
}

// RemoveProviderAdmins implements InstanceAdminResolver.
func (s *InstanceAdminService) RemoveProviderAdmins(
	ctx context.Context, providerID, providerSlug, reason string,
) error {
	if s.providerAdmins == nil {
		return nil
	}
	userIDs, err := s.providerAdmins.DeleteByProvider(ctx, providerID)
	if err != nil {
		return err
	}
	s.RecordProviderAdminDemotions(ctx, providerSlug, reason, userIDs)
	return nil
}

// RecordProviderAdminDemotions implements InstanceAdminResolver.
func (s *InstanceAdminService) RecordProviderAdminDemotions(
	ctx context.Context, providerSlug, reason string, userIDs []string,
) {
	for _, userID := range userIDs {
		s.logger.Info("Provider-sourced instance admin demoted",
			"user_id", userID, "provider", providerSlug, "reason", reason)
		s.recordProviderAdminChange(ctx, models.AuditOpAuthInstanceAdminDemoted, nil, userID, "",
			map[string]any{"provider_slug": providerSlug, "reason": reason})
	}
}

// recordProviderAdminChange audits one promotion or demotion of userID. A nil
// actor is read from ctx (the admin whose settings change caused it).
func (s *InstanceAdminService) recordProviderAdminChange(
	ctx context.Context, op models.AuditOperation, actor *audit.Actor, userID, email string, metadata map[string]any,
) {
	audit.SafeRecord(ctx, s.audit, audit.Event{
		Operation: op, Actor: actor, ResourceType: models.AuditResourceTypeUser, ResourceID: userID,
		ResourceName: email, Metadata: metadata,
	})
}

// RequireRootAdmin implements InstanceAdminResolver. It re-resolves the actor
// from the database by id, so the root-only rule never rests on anything the
// caller supplied beyond its id.
func (s *InstanceAdminService) RequireRootAdmin(ctx context.Context, actingUserID string) error {
	actor, err := s.lookupUser(ctx, actingUserID)
	if err != nil {
		return err
	}
	if actor == nil || actor.IsSuspended() || !s.IsVerifiedRootAdmin(actor) {
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

// WarnUnverifiedRootAdmins logs, at boot, a warning for every address in
// auth.instance_admins whose account exists but has no verified email: such an
// account holds no root admin power (#1467) until its owner signs in once with
// a provider that verifies email, or the operator runs the recovery CLI. An
// address with no account yet is not warned about; a look-up error is logged
// and never fatal.
func WarnUnverifiedRootAdmins(
	ctx context.Context, admins InstanceAdminResolver, users repositories.UserRepository, logger *slog.Logger,
) {
	for _, email := range admins.RootAdminEmails() {
		user, err := users.GetByEmail(ctx, email)
		if errors.Is(err, repositories.ErrUserNotFound) {
			continue
		}
		if err != nil {
			logger.Warn("Could not check whether a root instance admin's email is verified",
				"email", email, "error", err)
			continue
		}
		if user == nil || user.EmailVerifiedAt != nil {
			continue
		}
		logger.Warn("Root instance admin account has no verified email and holds no root admin power; "+
			"sign in once with a provider that verifies email, or run "+
			"`vibexp admin auth password set --email <email>`",
			"email", email, "user_id", user.ID)
	}
}
