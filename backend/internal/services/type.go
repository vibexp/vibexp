package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/services/audit"
)

// Type validation rules and sentinels. The handler maps these to 400 responses.
var (
	// ErrTypeSlugRequired is returned when a custom type's slug is empty.
	ErrTypeSlugRequired = errors.New("slug is required")
	// ErrTypeSlugInvalid is returned when a slug contains characters outside
	// [a-z0-9-].
	ErrTypeSlugInvalid = errors.New("slug must contain only lowercase letters, numbers, and hyphens")
	// ErrTypeSlugTooLong is returned when a slug exceeds the column length.
	ErrTypeSlugTooLong = errors.New("slug must be at most 255 characters")
	// ErrTypeNameRequired is returned when a custom type's display name is empty.
	ErrTypeNameRequired = errors.New("name is required")
	// ErrTypeNameTooLong is returned when a name exceeds the column length.
	ErrTypeNameTooLong = errors.New("name must be at most 255 characters")
	// ErrTypeResourceTypeUnsupported is returned when resource_type is not one of
	// the resources that support custom types.
	ErrTypeResourceTypeUnsupported = errors.New("resource_type is not supported")
)

const typeSlugMaxLen = 255
const typeNameMaxLen = 255

// slugPattern enforces URL-safe, lowercase, hyphenated slugs. Shared by the
// custom type and custom status (status_management.go) slug rules.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// resourceTypeDefaultSlug maps a supported resource_type to the system-default
// slug that orphaned resources are reassigned to when a custom type is deleted.
// Adding a resource here (plus the repository's reassignment branch) is all
// that is needed for it to adopt custom types.
var resourceTypeDefaultSlug = map[string]string{
	"artifacts": "general",
}

// copyableResourceTypes lists the resources whose custom types a cross-team
// copy carries, in a fixed order so one copy of the same two teams always
// produces the same result and the same audit detail. Derived from
// resourceTypeDefaultSlug — the registry of resources that support custom types
// — rather than duplicated, so adopting a second resource needs no change here.
func copyableResourceTypes() []string {
	out := make([]string, 0, len(resourceTypeDefaultSlug))
	for resourceType := range resourceTypeDefaultSlug {
		out = append(out, resourceType)
	}
	sort.Strings(out)
	return out
}

// TypeService is the concrete TypeServiceInterface implementation.
//
// authorizer and audit exist for CopyFromTeam alone. The single-team
// operations deliberately keep no authorization of their own: they authorize on
// membership, which teamValidationMiddleware already enforces from the URL's
// {team_id} (tightening that pre-existing gap is its own issue, out of scope
// for epic #827). A copy cannot rely on that, because its SOURCE team arrives
// in the request body and no middleware ever sees it.
type TypeService struct {
	repo       repositories.TypeRepository
	authorizer AuthorizationServiceInterface
	audit      TeamSettingsAuditServiceInterface
	logger     *slog.Logger
	teamSettingsAuditor
}

// TypeServiceOption customizes a TypeService.
type TypeServiceOption func(*TypeService)

// WithTypeAuditRecorder records custom type changes in the instance audit log
// (#1520).
func WithTypeAuditRecorder(recorder audit.Recorder) TypeServiceOption {
	return func(s *TypeService) { s.recorder = recorder }
}

// NewTypeService creates a new TypeService.
func NewTypeService(
	repo repositories.TypeRepository,
	authorizer AuthorizationServiceInterface,
	audit TeamSettingsAuditServiceInterface,
	logger *slog.Logger,
	opts ...TypeServiceOption,
) *TypeService {
	s := &TypeService{repo: repo, authorizer: authorizer, audit: audit, logger: logger}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *TypeService) List(ctx context.Context, teamID, resourceType string) ([]models.Type, error) {
	if _, ok := resourceTypeDefaultSlug[resourceType]; !ok {
		return nil, ErrTypeResourceTypeUnsupported
	}
	return s.repo.List(ctx, teamID, resourceType)
}

func (s *TypeService) CreateCustom(ctx context.Context, params CreateTypeParams) (*models.Type, error) {
	if _, ok := resourceTypeDefaultSlug[params.ResourceType]; !ok {
		return nil, ErrTypeResourceTypeUnsupported
	}
	if err := validateTypeSlug(params.Slug); err != nil {
		return nil, err
	}
	if err := validateTypeName(params.Name); err != nil {
		return nil, err
	}

	// Reject collisions against a global default or an existing team row up
	// front (the team partial unique index does not cover global-row slugs);
	// Create itself is the race backstop.
	if _, err := s.repo.GetBySlug(ctx, params.TeamID, params.ResourceType, params.Slug); err == nil {
		return nil, repositories.ErrTypeAlreadyExists
	} else if !errors.Is(err, repositories.ErrTypeNotFound) {
		return nil, err
	}

	t := &models.Type{
		TeamID:       params.TeamID,
		ResourceType: params.ResourceType,
		Slug:         params.Slug,
		Name:         params.Name,
		CreatedBy:    params.UserID,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	s.record(ctx, teamSettingsChange{
		TeamID: params.TeamID, ActorID: params.UserID, Surface: teamSettingsSurfaceCustomTypes,
		Action: teamSettingsActionCreate, After: customTypeAuditValues(t),
		Details: map[string]any{"type_id": t.ID, "type_slug": t.Slug},
	})
	return t, nil
}

func (s *TypeService) Delete(ctx context.Context, teamID, id string) error {
	// "general" is the artifacts default; the repository only applies the
	// fallback to the row's own resource_type, so passing the artifacts default
	// is safe even once other resources adopt types.
	existing := s.customTypeForAudit(ctx, teamID, id)
	if err := s.repo.DeleteCustom(ctx, teamID, id, resourceTypeDefaultSlug["artifacts"]); err != nil {
		return err
	}
	// The method has no user id: the recorder takes the actor from the request.
	change := teamSettingsChange{
		TeamID: teamID, Surface: teamSettingsSurfaceCustomTypes, Action: teamSettingsActionDelete,
		Before: customTypeAuditValues(existing), Details: map[string]any{"type_id": id},
	}
	if existing != nil {
		change.Details["type_slug"] = existing.Slug
	}
	s.record(ctx, change)
	return nil
}

// customTypeForAudit finds the team's custom type id before a delete, for the
// audit event. The repository has no lookup by id, so it scans the team's
// types; nil when nothing is recorded or the type is not found.
func (s *TypeService) customTypeForAudit(ctx context.Context, teamID, id string) *models.Type {
	if !s.enabled() {
		return nil
	}
	for _, resourceType := range copyableResourceTypes() {
		types, err := s.repo.List(ctx, teamID, resourceType)
		if err != nil {
			s.logger.WarnContext(ctx, "reading custom types for the audit log", "team_id", teamID, "error", err)
			return nil
		}
		for i := range types {
			if types[i].ID == id {
				return &types[i]
			}
		}
	}
	return nil
}

// customTypeAuditValues is the audited field allowlist of a custom type, nil
// for no type.
func customTypeAuditValues(t *models.Type) map[string]any {
	if t == nil {
		return nil
	}
	return map[string]any{"resource_type": t.ResourceType, "slug": t.Slug, "name": t.Name}
}

// CopyFromTeam merges another team's custom types into params.TeamID.
func (s *TypeService) CopyFromTeam(ctx context.Context, params CopyTypesParams) (*CopyTypesResult, error) {
	if err := AuthorizeCrossTeamCopy(
		ctx, s.authorizer, params.UserID, params.TeamID, params.SourceTeamID,
		CrossTeamCopyMembershipOnly,
	); err != nil {
		return nil, err
	}

	result := &CopyTypesResult{
		Added:   []models.Type{},
		Skipped: []SkippedType{},
	}
	for _, resourceType := range copyableResourceTypes() {
		if err := s.copyResourceTypes(ctx, params, resourceType, result); err != nil {
			return nil, err
		}
	}

	if err := s.recordCopy(ctx, params, result); err != nil {
		return nil, err
	}

	s.logger.Info("Copied custom types between teams",
		"team_id", params.TeamID, "source_team_id", params.SourceTeamID,
		"added", len(result.Added), "skipped", len(result.Skipped),
	)
	return result, nil
}

// copyResourceTypes copies one resource's custom types, appending to result.
func (s *TypeService) copyResourceTypes(
	ctx context.Context, params CopyTypesParams, resourceType string, result *CopyTypesResult,
) error {
	sourceTypes, err := s.repo.List(ctx, params.SourceTeamID, resourceType)
	if err != nil {
		return fmt.Errorf("failed to list source types for %q: %w", resourceType, err)
	}

	for i := range sourceTypes {
		source := &sourceTypes[i]
		// List unions the global system defaults with the team's own rows. The
		// defaults exist in EVERY team, so copying one would collide at
		// GetBySlug against the destination's copy of the same global row and
		// report the whole system set as skipped.
		if source.IsSystem {
			continue
		}

		skipped, copyErr := s.copyOneType(ctx, params, source)
		if copyErr != nil {
			return copyErr
		}
		if skipped != nil {
			result.Skipped = append(result.Skipped, *skipped)
			continue
		}
		result.Added = append(result.Added, *source)
	}
	return nil
}

// copyOneType creates one type in the destination, reporting a slug that is
// already taken as skipped rather than as an error. On success it rewrites
// source in place with the destination's row (new id, team and timestamps), so
// the caller can return the created resource.
func (s *TypeService) copyOneType(
	ctx context.Context, params CopyTypesParams, source *models.Type,
) (*SkippedType, error) {
	skip := &SkippedType{ResourceType: source.ResourceType, Slug: source.Slug}

	// Pre-check the destination the same way CreateCustom does: the team's
	// partial unique index does not cover a colliding GLOBAL default slug, so
	// the index alone would let one through.
	if _, err := s.repo.GetBySlug(ctx, params.TeamID, source.ResourceType, source.Slug); err == nil {
		return skip, nil
	} else if !errors.Is(err, repositories.ErrTypeNotFound) {
		return nil, fmt.Errorf("failed to check destination type %q: %w", source.Slug, err)
	}

	created := &models.Type{
		TeamID:       params.TeamID,
		ResourceType: source.ResourceType,
		Slug:         source.Slug,
		Name:         source.Name,
		CreatedBy:    params.UserID,
	}
	if err := s.repo.Create(ctx, created); err != nil {
		// The pre-check above is not a lock. A concurrent create of the same
		// slug is the same outcome as an existing one — the destination has the
		// type — so it is a skip, not a failure.
		if errors.Is(err, repositories.ErrTypeAlreadyExists) {
			return skip, nil
		}
		return nil, fmt.Errorf("failed to create type %q: %w", source.Slug, err)
	}

	*source = *created
	return nil, nil
}

// recordCopy writes the single audit entry for the whole action (epic #827,
// decision 8). One copy is one row: the individual ids live in the detail
// payload because one action creates many rows.
//
// A failure here fails the copy. The rows it did create are NOT rolled back —
// they are legitimately the destination's now, and re-running the copy skips
// them — but reporting success for a copy whose compensating control never
// landed would defeat the point of having one.
func (s *TypeService) recordCopy(
	ctx context.Context, params CopyTypesParams, result *CopyTypesResult,
) error {
	addedIDs := make([]string, 0, len(result.Added))
	addedSlugs := make([]string, 0, len(result.Added))
	for i := range result.Added {
		addedIDs = append(addedIDs, result.Added[i].ID)
		addedSlugs = append(addedSlugs, result.Added[i].Slug)
	}
	skippedSlugs := make([]string, 0, len(result.Skipped))
	for i := range result.Skipped {
		skippedSlugs = append(skippedSlugs, result.Skipped[i].Slug)
	}

	if _, err := s.audit.Record(ctx, TeamSettingsAuditRecord{
		TeamID:       params.TeamID,
		ActorUserID:  params.UserID,
		Surface:      models.SettingsAuditSurfaceCustomTypes,
		SourceTeamID: params.SourceTeamID,
		Detail: map[string]interface{}{
			"added_ids":     addedIDs,
			"added_slugs":   addedSlugs,
			"skipped_slugs": skippedSlugs,
		},
	}); err != nil {
		return fmt.Errorf("failed to record custom types copy: %w", err)
	}
	// The team history above is fail-closed; the instance log is best-effort.
	s.record(ctx, teamSettingsChange{
		TeamID: params.TeamID, ActorID: params.UserID, Surface: teamSettingsSurfaceCustomTypes,
		Action: teamSettingsActionCopy,
		Details: map[string]any{
			"source_team_id": params.SourceTeamID,
			"added_slugs":    strings.Join(addedSlugs, ", "),
			"skipped_slugs":  strings.Join(skippedSlugs, ", "),
		},
	})
	return nil
}

func (s *TypeService) ValidateType(
	ctx context.Context, teamID, resourceType, slug string,
) (bool, error) {
	_, err := s.repo.GetBySlug(ctx, teamID, resourceType, slug)
	if err != nil {
		if errors.Is(err, repositories.ErrTypeNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func validateTypeSlug(slug string) error {
	switch {
	case slug == "":
		return ErrTypeSlugRequired
	case len(slug) > typeSlugMaxLen:
		return ErrTypeSlugTooLong
	case !slugPattern.MatchString(slug):
		return ErrTypeSlugInvalid
	default:
		return nil
	}
}

func validateTypeName(name string) error {
	switch {
	case name == "":
		return ErrTypeNameRequired
	case len(name) > typeNameMaxLen:
		return ErrTypeNameTooLong
	default:
		return nil
	}
}
