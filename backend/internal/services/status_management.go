package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/vibexp/vibexp/internal/authz"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// Custom status validation sentinels. The handler maps the first five to 400
// and ErrStatusReserved to 409.
var (
	// ErrStatusSlugInvalid is returned when a slug is empty or is not lowercase
	// letters and numbers in hyphen-separated groups.
	ErrStatusSlugInvalid = errors.New(
		"slug must be lowercase letters and numbers in hyphen-separated groups")
	// ErrStatusSlugTooLong is returned when a slug exceeds the column length.
	ErrStatusSlugTooLong = errors.New("slug must be at most 50 characters")
	// ErrStatusNameRequired is returned when a name is empty once trimmed.
	ErrStatusNameRequired = errors.New("name is required")
	// ErrStatusNameTooLong is returned when a trimmed name exceeds the column
	// length.
	ErrStatusNameTooLong = errors.New("name must be at most 50 characters")
	// ErrStatusLimitReached is returned when the team already has
	// MaxCustomStatusesPerResourceType custom statuses for the resource type.
	ErrStatusLimitReached = errors.New(
		"a team can have at most 20 custom statuses per resource type")
	// ErrStatusReserved is returned when a slug is one of
	// models.ReservedStatusSlugs.
	ErrStatusReserved = errors.New("slug is a built-in status and cannot be used")
)

const (
	// statusSlugMaxLen and statusNameMaxLen are the widths of
	// team_statuses.slug and .name, and of the four resource status columns.
	statusSlugMaxLen = 50
	statusNameMaxLen = 50

	// MaxCustomStatusesPerResourceType caps a team's custom statuses for one
	// resource type. It keeps a status picker usable; it is not a security
	// boundary, so it is checked before the insert rather than inside it, and
	// two concurrent creates can leave a team one over.
	MaxCustomStatusesPerResourceType = 20
)

// The `action` values a custom_statuses audit entry carries in its detail.
const (
	statusAuditActionCreated = "created"
	statusAuditActionRenamed = "renamed"
	statusAuditActionDeleted = "deleted"
)

// ManagedStatus is one status as the management API lists it: a catalog entry
// plus how many of the team's resources carry it.
type ManagedStatus struct {
	CatalogStatus
	ResourceType string
	// UsageCount is the number of the team's resources of ResourceType whose
	// status is Slug -- for a custom status, what a delete would reassign.
	UsageCount int
}

// CreateStatusParams carries the inputs for creating a custom status.
type CreateStatusParams struct {
	TeamID       string
	UserID       string
	ResourceType string
	Slug         string
	Name         string
}

// RenameStatusParams carries the inputs for renaming a custom status.
type RenameStatusParams struct {
	TeamID string
	UserID string
	ID     string
	Name   string
}

// DeleteStatusParams identifies the custom status to delete and who asked.
type DeleteStatusParams struct {
	TeamID string
	UserID string
	ID     string
}

// StatusDeletion is the outcome of deleting a custom status.
type StatusDeletion struct {
	// ReassignedCount is how many resources carried the status.
	ReassignedCount int
	// ReassignedTo is the system status they carry now.
	ReassignedTo string
}

// StatusManagementServiceInterface manages a team's custom statuses (epic
// #1263): the list the settings page renders, and the create, rename and
// delete behind it.
//
// List authorizes nothing: any member may read the catalog, and membership of
// the URL's team is what teamValidationMiddleware establishes. Create, Rename
// and Delete each require authz.TeamSettingsUpdate and return an
// ErrPermissionDenied-wrapped error without it. Each successful write appends
// one team settings audit entry on the custom_statuses surface, in the same
// transaction as the change (#1333): when the entry cannot be built or stored
// the change is rolled back and the error returned, so an error from a write
// always means nothing was stored and the request can be retried as it is.
type StatusManagementServiceInterface interface {
	// List returns the catalog for (teamID, resourceType) with usage counts:
	// system statuses first, then the team's custom ones. It returns
	// ErrStatusResourceTypeUnsupported for an unknown resource type.
	List(ctx context.Context, teamID, resourceType string) ([]ManagedStatus, error)
	// Create validates and stores a custom status. The returned UsageCount is
	// always 0 and costs no query: a status that did not exist a moment ago is
	// on no resource. It returns
	// ErrStatusResourceTypeUnsupported or an ErrStatus* validation sentinel for
	// bad input, ErrStatusReserved for a built-in slug,
	// ErrStatusLimitReached at the cap, and repositories.ErrTeamStatusExists
	// when the team already has the slug for that resource type.
	Create(ctx context.Context, params CreateStatusParams) (*ManagedStatus, error)
	// Rename changes a custom status's display name; the slug is immutable. It
	// returns repositories.ErrTeamStatusNotFound for an id the team does not
	// have. The usage count is read after the rename has committed, so a
	// failure to read it is logged and reported as UsageCount 0 rather than
	// failing a rename that is already stored; List is the exact figure.
	Rename(ctx context.Context, params RenameStatusParams) (*ManagedStatus, error)
	// Delete removes a custom status and moves every resource carrying it to
	// the resource type's default system status, atomically. It returns
	// repositories.ErrTeamStatusNotFound for an id the team does not have, and
	// a *repositories.LockedResourcesError, changing nothing, while a locked
	// prompt, blueprint or artifact carries the status.
	Delete(ctx context.Context, params DeleteStatusParams) (*StatusDeletion, error)
}

// StatusManagementService implements StatusManagementServiceInterface.
type StatusManagementService struct {
	repo    repositories.TeamStatusRepository
	catalog StatusCatalogServiceInterface
	authz   AuthorizationServiceInterface
	audit   TeamSettingsAuditServiceInterface
	logger  *slog.Logger
	teamSettingsAuditor
}

var _ StatusManagementServiceInterface = (*StatusManagementService)(nil)

// StatusManagementServiceOption customizes a StatusManagementService.
type StatusManagementServiceOption func(*StatusManagementService)

// WithStatusManagementAuditRecorder records custom status changes in the
// instance audit log (#1520).
func WithStatusManagementAuditRecorder(recorder audit.Recorder) StatusManagementServiceOption {
	return func(s *StatusManagementService) { s.recorder = recorder }
}

// NewStatusManagementService creates a new StatusManagementService.
func NewStatusManagementService(
	repo repositories.TeamStatusRepository,
	catalog StatusCatalogServiceInterface,
	authzService AuthorizationServiceInterface,
	audit TeamSettingsAuditServiceInterface,
	logger *slog.Logger,
	opts ...StatusManagementServiceOption,
) *StatusManagementService {
	s := &StatusManagementService{
		repo: repo, catalog: catalog, authz: authzService, audit: audit, logger: logger,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *StatusManagementService) List(
	ctx context.Context, teamID, resourceType string,
) ([]ManagedStatus, error) {
	catalog, err := s.catalog.List(ctx, teamID, resourceType)
	if err != nil {
		return nil, err
	}
	counts, err := s.repo.UsageCounts(ctx, teamID, resourceType)
	if err != nil {
		return nil, fmt.Errorf("failed to count status usage: %w", err)
	}

	statuses := make([]ManagedStatus, 0, len(catalog))
	for i := range catalog {
		statuses = append(statuses, ManagedStatus{
			CatalogStatus: catalog[i],
			ResourceType:  resourceType,
			UsageCount:    counts[catalog[i].Slug],
		})
	}
	return statuses, nil
}

func (s *StatusManagementService) Create(
	ctx context.Context, params CreateStatusParams,
) (*ManagedStatus, error) {
	if err := s.authorizeWrite(ctx, params.UserID, params.TeamID); err != nil {
		return nil, err
	}
	if !models.IsStatusResourceType(params.ResourceType) {
		return nil, fmt.Errorf("%w: %q", ErrStatusResourceTypeUnsupported, params.ResourceType)
	}
	if err := validateStatusSlug(params.Slug); err != nil {
		return nil, err
	}
	name, err := normalizeStatusName(params.Name)
	if err != nil {
		return nil, err
	}

	count, err := s.repo.Count(ctx, params.TeamID, params.ResourceType)
	if err != nil {
		return nil, fmt.Errorf("failed to count team statuses: %w", err)
	}
	if count >= MaxCustomStatusesPerResourceType {
		return nil, ErrStatusLimitReached
	}

	row := &models.TeamStatus{
		TeamID:       params.TeamID,
		ResourceType: params.ResourceType,
		Slug:         params.Slug,
		Name:         name,
		CreatedBy:    optionalID(params.UserID),
	}
	var change repositories.TeamStatusChange
	build := capturingStatusChange(s.auditBuilder(params.UserID, statusAuditActionCreated, ""), &change)
	if err := s.repo.Create(ctx, row, build); err != nil {
		return nil, err
	}
	s.recordStatusChange(ctx, params.UserID, params.TeamID, statusAuditActionCreated, "", change)
	// A status that did not exist until this insert is on no resource, so the
	// count is known without a query that could fail after the write.
	return managedStatus(row, 0), nil
}

func (s *StatusManagementService) Rename(
	ctx context.Context, params RenameStatusParams,
) (*ManagedStatus, error) {
	if err := s.authorizeWrite(ctx, params.UserID, params.TeamID); err != nil {
		return nil, err
	}
	name, err := normalizeStatusName(params.Name)
	if err != nil {
		return nil, err
	}

	var change repositories.TeamStatusChange
	row, err := s.repo.UpdateName(ctx, params.TeamID, params.ID, name,
		capturingStatusChange(s.auditBuilder(params.UserID, statusAuditActionRenamed, ""), &change))
	if err != nil {
		return nil, err
	}
	s.recordStatusChange(ctx, params.UserID, params.TeamID, statusAuditActionRenamed, "", change)
	return managedStatus(row, s.usageCountOrZero(ctx, row)), nil
}

func (s *StatusManagementService) Delete(
	ctx context.Context, params DeleteStatusParams,
) (*StatusDeletion, error) {
	if err := s.authorizeWrite(ctx, params.UserID, params.TeamID); err != nil {
		return nil, err
	}

	// The default depends on the row's resource type, which only the row
	// knows. It cannot change between this read and the delete, and a row that
	// disappears in between surfaces as not-found from the delete itself.
	existing, err := s.repo.GetByID(ctx, params.TeamID, params.ID)
	if err != nil {
		return nil, err
	}
	reassignTo := models.DefaultStatusFor(existing.ResourceType)
	if reassignTo == "" {
		return nil, fmt.Errorf("%w: %q", ErrStatusResourceTypeUnsupported, existing.ResourceType)
	}

	var change repositories.TeamStatusChange
	_, reassigned, err := s.repo.DeleteAndReassign(ctx, params.TeamID, params.ID, reassignTo,
		capturingStatusChange(s.auditBuilder(params.UserID, statusAuditActionDeleted, reassignTo), &change))
	if err != nil {
		return nil, err
	}
	s.recordStatusChange(ctx, params.UserID, params.TeamID, statusAuditActionDeleted, reassignTo, change)
	return &StatusDeletion{ReassignedCount: reassigned, ReassignedTo: reassignTo}, nil
}

// capturingStatusChange wraps build so the change the repository hands it is
// also kept in dst, for the instance audit event recorded after the commit.
func capturingStatusChange(
	build repositories.TeamStatusAuditBuilder, dst *repositories.TeamStatusChange,
) repositories.TeamStatusAuditBuilder {
	return func(change repositories.TeamStatusChange) (*models.TeamSettingsAudit, error) {
		*dst = change
		return build(change)
	}
}

// recordStatusChange records a committed custom status write in the instance
// audit log, with the per-team history's action names (created / renamed /
// deleted). change is what the repository passed to the builder.
func (s *StatusManagementService) recordStatusChange(
	ctx context.Context, userID, teamID, action, reassignTo string, change repositories.TeamStatusChange,
) {
	row := change.Status
	if row == nil {
		return
	}
	values := map[string]any{"resource_type": row.ResourceType, "slug": row.Slug, "name": row.Name}
	audited := teamSettingsChange{
		TeamID: teamID, ActorID: userID, Surface: teamSettingsSurfaceCustomStatuses, Action: action,
		Details: map[string]any{"status_id": row.ID},
	}
	switch action {
	case statusAuditActionCreated:
		audited.After = values
	case statusAuditActionRenamed:
		audited.After = values
		audited.Before = map[string]any{"resource_type": row.ResourceType, "slug": row.Slug, "name": change.PreviousName}
	case statusAuditActionDeleted:
		audited.Before = values
		audited.Details["reassigned_count"] = change.ReassignedCount
		audited.Details["reassigned_to"] = reassignTo
	}
	s.record(ctx, audited)
}

// authorizeWrite is the one gate on every custom-status write.
func (s *StatusManagementService) authorizeWrite(ctx context.Context, userID, teamID string) error {
	_, err := s.authz.Authorize(ctx, userID, teamID, authz.TeamSettingsUpdate)
	return err
}

// auditBuilder returns the builder the repository calls inside the write's
// transaction to get the custom_statuses entry to store with it. Every entry
// names the status by resource type, slug and name, plus the action and what is
// specific to it: the created row's id, the name a rename replaced, or how many
// resources a delete moved and where to (reassignTo, used by a delete only).
//
// The builder does no I/O -- TeamSettingsAuditServiceInterface.Build is pure --
// which is what makes it safe to run while the transaction is open.
func (s *StatusManagementService) auditBuilder(
	userID, action, reassignTo string,
) repositories.TeamStatusAuditBuilder {
	return func(change repositories.TeamStatusChange) (*models.TeamSettingsAudit, error) {
		row := change.Status
		detail := map[string]interface{}{
			"action":        action,
			"resource_type": row.ResourceType,
			"slug":          row.Slug,
			"name":          row.Name,
		}
		createdID := ""
		switch action {
		case statusAuditActionCreated:
			createdID = row.ID
		case statusAuditActionRenamed:
			detail["previous_name"] = change.PreviousName
		case statusAuditActionDeleted:
			detail["reassigned_count"] = change.ReassignedCount
			detail["reassigned_to"] = reassignTo
		}

		return s.audit.Build(TeamSettingsAuditRecord{
			TeamID:            row.TeamID,
			ActorUserID:       userID,
			Surface:           models.SettingsAuditSurfaceCustomStatuses,
			CreatedResourceID: createdID,
			Detail:            detail,
		})
	}
}

// usageCountOrZero reads how many resources carry a status that has just been
// renamed. The rename and its audit entry are already committed, so a failed
// read is logged and answered with 0 instead of turning a stored write into an
// error: the count is advisory on this response, and List reports it exactly.
func (s *StatusManagementService) usageCountOrZero(ctx context.Context, row *models.TeamStatus) int {
	counts, err := s.repo.UsageCounts(ctx, row.TeamID, row.ResourceType)
	if err != nil {
		s.logger.Error("Custom status was renamed but its usage count could not be read",
			"team_id", row.TeamID, "status_id", row.ID, "resource_type", row.ResourceType, "error", err)
		return 0
	}
	return counts[row.Slug]
}

// managedStatus renders a stored row as the API's view of it.
func managedStatus(row *models.TeamStatus, usageCount int) *ManagedStatus {
	createdAt, updatedAt := row.CreatedAt, row.UpdatedAt
	return &ManagedStatus{
		CatalogStatus: CatalogStatus{
			ID:        &row.ID,
			Slug:      row.Slug,
			Name:      row.Name,
			IsLive:    models.IsLiveStatus(row.ResourceType, row.Slug),
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt,
		},
		ResourceType: row.ResourceType,
		UsageCount:   usageCount,
	}
}

// validateStatusSlug applies the slug rules in order: length, shape, then the
// reserved vocabulary. The slug is taken as
// sent, never trimmed or lowercased, because it is the value resources store.
func validateStatusSlug(slug string) error {
	switch {
	case len(slug) > statusSlugMaxLen:
		return ErrStatusSlugTooLong
	case !slugPattern.MatchString(slug):
		return ErrStatusSlugInvalid
	case slices.Contains(models.ReservedStatusSlugs, slug):
		return ErrStatusReserved
	}
	return nil
}

// normalizeStatusName trims a display name and checks it is 1 to 50
// characters, counted as runes because the column is varchar(50).
func normalizeStatusName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", ErrStatusNameRequired
	case utf8.RuneCountInString(name) > statusNameMaxLen:
		return "", ErrStatusNameTooLong
	}
	return name, nil
}
