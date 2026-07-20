package services

import (
	"errors"
	"fmt"

	"github.com/vibexp/vibexp/internal/models"
)

// Team Authorization Errors

// ErrTeamNotFound is returned when the requested team does not exist.
var ErrTeamNotFound = errors.New("team not found")

// ErrCannotRemoveTeamOwner is returned when a removal targets the team owner.
// The owner is removable only by deleting the team or transferring ownership
// first; without this an admin could remove the one role above them.
var ErrCannotRemoveTeamOwner = errors.New("cannot remove team owner")

// ErrCannotChangeOwnerRole is returned when a role change targets the team
// owner. Ownership moves only through TransferOwnership.
var ErrCannotChangeOwnerRole = errors.New("cannot change the role of the team owner")

// ErrInvalidMemberRole is returned when a role change requests a role other
// than member or admin — notably owner, which would mint a second owner.
var ErrInvalidMemberRole = errors.New("role must be either member or admin")

// ErrAlreadyTeamOwner is returned when ownership is transferred to the user who
// already owns the team, which would be a no-op demoting the owner to admin.
var ErrAlreadyTeamOwner = errors.New("user already owns this team")

// ErrPermissionDenied is returned when a user's role in a team does not grant
// the permission required for the attempted action. Handlers map it to an RFC
// 9457 FORBIDDEN 403 via errors.NewForbiddenError. Non-membership is reported
// as a denial too, so the error never distinguishes "not a member" from "role
// too low" to a caller.
var ErrPermissionDenied = errors.New("permission denied")

// Relation Errors

// ErrRelationInvalidType is returned when a relation's object (to) type does
// not satisfy the type its relation_type requires (RelationTypeMatrix): e.g. a
// governed-by whose object is not a blueprint, or a supersedes across types.
var ErrRelationInvalidType = errors.New("relation object type not allowed for this relation type")

// ErrRelationSelfLink is returned when a relation's two endpoints are the same
// resource (same type and id); a resource cannot relate to itself.
var ErrRelationSelfLink = errors.New("a resource cannot relate to itself")

// ErrRelationCrossProject is returned when a relation's two endpoints live in
// different projects; relations are within a single project.
var ErrRelationCrossProject = errors.New("relation endpoints must be in the same project")

// ErrRelationResourceNotFound is returned when a relation endpoint does not
// exist in the team (missing or foreign resource).
var ErrRelationResourceNotFound = errors.New("relation resource not found")

// ErrRelationAlreadyConfirmed is returned when confirming a relation that is
// already in the confirmed state.
var ErrRelationAlreadyConfirmed = errors.New("relation is already confirmed")

// Embedding Provider Errors

// ErrProviderNotFound is returned when an embedding provider is not found
var ErrProviderNotFound = errors.New("embedding provider not found")

// ErrProviderAlreadyExists is returned when trying to create a provider that already exists
var ErrProviderAlreadyExists = errors.New("embedding provider already exists")

// ErrLastProviderDelete is returned when trying to delete the last embedding provider
var ErrLastProviderDelete = errors.New("cannot delete the last embedding provider")

// Model Provider Errors

// ErrModelProviderNotFound is returned when a model provider is not found
var ErrModelProviderNotFound = errors.New("model provider not found")

// ErrModelProviderAlreadyExists is returned when trying to create a model provider that already exists
var ErrModelProviderAlreadyExists = errors.New("model provider already exists")

// ErrLastModelProviderDelete is returned when trying to delete the last model provider
var ErrLastModelProviderDelete = errors.New("cannot delete the last model provider")

// DuplicateMembersError represents an error when trying to invite members who are already in the team
type DuplicateMembersError struct {
	DuplicateEmails []string
}

// Error implements the error interface
func (e *DuplicateMembersError) Error() string {
	if len(e.DuplicateEmails) == 1 {
		return fmt.Sprintf("User %s is already in the team", e.DuplicateEmails[0])
	}
	return fmt.Sprintf("Users already in team: %s", fmt.Sprintf("%v", e.DuplicateEmails))
}

// NewDuplicateMembersError creates a new DuplicateMembersError
func NewDuplicateMembersError(emails []string) *DuplicateMembersError {
	return &DuplicateMembersError{
		DuplicateEmails: emails,
	}
}

// PersonalWorkspaceError is returned when attempting to invite members to a personal workspace
type PersonalWorkspaceError struct {
	TeamID string
}

// Error implements the error interface
func (e *PersonalWorkspaceError) Error() string {
	return "cannot invite members to personal workspace - upgrade to a team plan to enable collaboration"
}

// NewPersonalWorkspaceError creates a new PersonalWorkspaceError
func NewPersonalWorkspaceError(teamID string) *PersonalWorkspaceError {
	return &PersonalWorkspaceError{TeamID: teamID}
}

// ActiveSubscriptionError is returned when attempting to delete a team with an active subscription
type ActiveSubscriptionError struct {
	TeamID           string
	SubscriptionID   string
	SubscriptionTier string
	BillingPortalURL string
	HelpText         string
}

// Error implements the error interface
func (e *ActiveSubscriptionError) Error() string {
	return "cannot delete team with active subscription. Cancel subscription first"
}

// NewActiveSubscriptionError creates a new ActiveSubscriptionError
func NewActiveSubscriptionError(teamID, subscriptionID, tier, portalURL string) *ActiveSubscriptionError {
	return &ActiveSubscriptionError{
		TeamID:           teamID,
		SubscriptionID:   subscriptionID,
		SubscriptionTier: tier,
		BillingPortalURL: portalURL,
		HelpText:         "Visit the billing portal to cancel your subscription, then try deleting again.",
	}
}

// SubscriptionCancelingError is returned when subscription is canceled but period hasn't ended
type SubscriptionCancelingError struct {
	TeamID   string
	CancelAt string
}

// Error implements the error interface
func (e *SubscriptionCancelingError) Error() string {
	return fmt.Sprintf("team subscription is canceling on %s. You can delete the team after this date", e.CancelAt)
}

// NewSubscriptionCancelingError creates a new SubscriptionCancelingError
func NewSubscriptionCancelingError(teamID, cancelAt string) *SubscriptionCancelingError {
	return &SubscriptionCancelingError{
		TeamID:   teamID,
		CancelAt: cancelAt,
	}
}

// TeamHasMembersError is returned when attempting to delete a team with multiple members
type TeamHasMembersError struct {
	TeamID      string
	MemberCount int
}

// Error implements the error interface
func (e *TeamHasMembersError) Error() string {
	return fmt.Sprintf("cannot delete team with active members. Remove all %d members first", e.MemberCount)
}

// NewTeamHasMembersError creates a new TeamHasMembersError
func NewTeamHasMembersError(teamID string, memberCount int) *TeamHasMembersError {
	return &TeamHasMembersError{
		TeamID:      teamID,
		MemberCount: memberCount,
	}
}

// CannotDeletePersonalWorkspaceError is returned when attempting to delete a personal workspace
type CannotDeletePersonalWorkspaceError struct {
	TeamID string
}

// Error implements the error interface
func (e *CannotDeletePersonalWorkspaceError) Error() string {
	return "cannot delete personal workspace"
}

// NewCannotDeletePersonalWorkspaceError creates a new CannotDeletePersonalWorkspaceError
func NewCannotDeletePersonalWorkspaceError(teamID string) *CannotDeletePersonalWorkspaceError {
	return &CannotDeletePersonalWorkspaceError{TeamID: teamID}
}

// NoActiveSubscriptionError is returned when attempting to invite members without an active subscription
type NoActiveSubscriptionError struct {
	TeamID string
}

// Error implements the error interface
func (e *NoActiveSubscriptionError) Error() string {
	return "team requires an active subscription to invite members"
}

// NewNoActiveSubscriptionError creates a new NoActiveSubscriptionError
func NewNoActiveSubscriptionError(teamID string) *NoActiveSubscriptionError {
	return &NoActiveSubscriptionError{TeamID: teamID}
}

// SeatLimitExceededError is returned when team has reached its seat limit
type SeatLimitExceededError struct {
	TeamID           string
	CurrentMembers   int
	PendingInvites   int
	PaidSeats        int
	RequestedInvites int
}

// Error implements the error interface
func (e *SeatLimitExceededError) Error() string {
	totalOccupied := e.CurrentMembers + e.PendingInvites
	availableSeats := e.PaidSeats - totalOccupied
	return fmt.Sprintf(
		"team has reached seat limit. You have %d/%d seats used (%d members + %d pending invitations). "+
			"Add %d more seats to invite %d additional members",
		totalOccupied, e.PaidSeats,
		e.CurrentMembers, e.PendingInvites,
		e.RequestedInvites-availableSeats,
		e.RequestedInvites,
	)
}

// NewSeatLimitExceededError creates a new SeatLimitExceededError
func NewSeatLimitExceededError(
	teamID string,
	currentMembers, pendingInvites, paidSeats, requestedInvites int,
) *SeatLimitExceededError {
	return &SeatLimitExceededError{
		TeamID:           teamID,
		CurrentMembers:   currentMembers,
		PendingInvites:   pendingInvites,
		PaidSeats:        paidSeats,
		RequestedInvites: requestedInvites,
	}
}

// InvitationNotFoundError is returned when an invitation cannot be found by token.
type InvitationNotFoundError struct {
	Token string
}

// Error implements the error interface
func (e *InvitationNotFoundError) Error() string {
	return "invitation not found"
}

// NewInvitationNotFoundError creates a new InvitationNotFoundError
func NewInvitationNotFoundError(token string) *InvitationNotFoundError {
	return &InvitationNotFoundError{Token: token}
}

// InvitationExpiredError is returned when an invitation is past its expiry timestamp.
type InvitationExpiredError struct {
	ID string
}

// Error implements the error interface
func (e *InvitationExpiredError) Error() string {
	return "invitation has expired"
}

// NewInvitationExpiredError creates a new InvitationExpiredError
func NewInvitationExpiredError(id string) *InvitationExpiredError {
	return &InvitationExpiredError{ID: id}
}

// InvitationStateError is returned when an invitation is in a non-pending state
// (accepted, rejected, or revoked) and therefore cannot be acted on.
type InvitationStateError struct {
	ID     string
	Status models.InvitationStatus
}

// Error implements the error interface
func (e *InvitationStateError) Error() string {
	switch e.Status {
	case models.InvitationStatusAccepted:
		return "invitation has already been accepted"
	case models.InvitationStatusRejected:
		return "invitation has been rejected"
	case models.InvitationStatusRevoked:
		return "invitation has been revoked"
	default:
		return fmt.Sprintf("invitation is in state %q", string(e.Status))
	}
}

// NewInvitationStateError creates a new InvitationStateError
func NewInvitationStateError(id string, status models.InvitationStatus) *InvitationStateError {
	return &InvitationStateError{ID: id, Status: status}
}

// CannotDeleteLastProjectError is returned when attempting to delete the last project in a team
type CannotDeleteLastProjectError struct {
	TeamID      string
	ProjectSlug string
}

// Error implements the error interface
func (e *CannotDeleteLastProjectError) Error() string {
	return fmt.Sprintf("cannot delete project '%s': teams must have at least one project", e.ProjectSlug)
}

// NewCannotDeleteLastProjectError creates a new CannotDeleteLastProjectError
func NewCannotDeleteLastProjectError(teamID, projectSlug string) *CannotDeleteLastProjectError {
	return &CannotDeleteLastProjectError{
		TeamID:      teamID,
		ProjectSlug: projectSlug,
	}
}
